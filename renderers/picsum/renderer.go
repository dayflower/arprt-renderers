package main

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"time"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

const downloadLimit = 8_000_000
const pngLimit = 90_000

type renderContext struct {
	Version string `json:"version"`
	Target  struct {
		Width    int             `json:"width"`
		Height   int             `json:"height"`
		Palette  string          `json:"palette"`
		Rotation json.RawMessage `json:"rotation"`
	} `json:"renderTarget"`
	rotation int
}

func parseContext(data []byte, palette string) (renderContext, error) {
	var context renderContext
	if err := json.Unmarshal(data, &context); err != nil {
		return context, err
	}
	if palette != "gray4" && palette != "bw" && palette != "rgb" {
		return context, errors.New("expected output palette gray4, bw or rgb")
	}
	validPalette := context.Target.Palette == "gray4" ||
		(palette == "rgb" && context.Target.Palette == "spectra6")
	if context.Version != "v1" || context.Target.Width != 800 || context.Target.Height != 480 || !validPalette {
		return context, errors.New("expected Render Context v1 for 800 x 480 / gray4 (or spectra6 for rgb)")
	}
	if raw := context.Target.Rotation; len(raw) != 0 {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return context, errors.New("expected rotation 0, 90, 180 or 270")
		}
		if err := json.Unmarshal(raw, &context.rotation); err != nil {
			return context, errors.New("expected rotation 0, 90, 180 or 270")
		}
	}
	if r := context.rotation; r != 0 && r != 90 && r != 180 && r != 270 {
		return context, errors.New("expected rotation 0, 90, 180 or 270")
	}
	return context, nil
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type renderer struct {
	client httpDoer
	sleep  func(time.Duration)
	log    io.Writer
	encode func(*image.Gray) ([]byte, error)
}

func newRenderer(log io.Writer) renderer {
	return renderer{
		client: &http.Client{Timeout: 20 * time.Second},
		sleep:  time.Sleep,
		log:    log,
		encode: encodeGray,
	}
}

func (r renderer) download(width, height int) ([]byte, string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf("https://picsum.photos/%d/%d.jpg?random=%s", width, height, hex.EncodeToString(nonce[:]))
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("User-Agent", "arprt-picsum/1.0")
	for attempt := 0; attempt < 3; attempt++ {
		var response *http.Response
		response, err = r.client.Do(request)
		if err == nil {
			var data []byte
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				data, err = io.ReadAll(io.LimitReader(response.Body, downloadLimit+1))
			} else {
				err = fmt.Errorf("photo download: HTTP %d", response.StatusCode)
			}
			response.Body.Close()
			if err == nil {
				if len(data) > downloadLimit {
					return nil, "", errors.New("downloaded photo exceeds size limit")
				}
				id := response.Header.Get("Picsum-ID")
				if id == "" {
					id = "unknown"
				}
				return data, id, nil
			}
		}
		if attempt < 2 {
			r.sleep(2 * time.Second)
		}
	}
	return nil, "", err
}

func (r renderer) render(context renderContext, source []byte, palette string) ([]byte, error) {
	width, height := 800, 480
	if context.rotation == 90 || context.rotation == 270 {
		width, height = height, width
	}
	photoID := "local"
	if source == nil {
		var err error
		source, photoID, err = r.download(width, height)
		if err != nil {
			return nil, err
		}
	}
	photo, err := imaging.Decode(bytes.NewReader(source), imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}
	// Drop alpha like Pillow's RGB/L conversion, rather than compositing on black.
	opaque := imaging.Clone(photo)
	for i := 0; i < len(opaque.Pix); i += 4 {
		if palette != "rgb" {
			v := uint8((299*int(opaque.Pix[i]) + 587*int(opaque.Pix[i+1]) + 114*int(opaque.Pix[i+2]) + 500) / 1000)
			opaque.Pix[i], opaque.Pix[i+1], opaque.Pix[i+2] = v, v, v
		}
		opaque.Pix[i+3] = 255
	}
	fitted := imaging.Fill(opaque, width, height, imaging.Center, imaging.Lanczos)
	var output []byte
	if palette == "rgb" {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, rotate(fitted, context.rotation)); err != nil {
			return nil, err
		}
		output = buffer.Bytes()
	} else {
		gray := image.NewGray(fitted.Bounds())
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				gray.Pix[y*gray.Stride+x] = fitted.Pix[y*fitted.Stride+4*x]
			}
		}
		autocontrast(gray)
		output, err = r.encode(rotateGray(quantize(gray, palette, true), context.rotation))
		if err != nil {
			return nil, err
		}
		if len(output) >= pngLimit {
			output, err = r.encode(rotateGray(quantize(gray, palette, false), context.rotation))
			if err != nil {
				return nil, err
			}
		}
		if len(output) >= pngLimit {
			return nil, errors.New("PNG exceeds device size limit")
		}
	}
	ref := photoID
	if photoID != "local" && photoID != "unknown" {
		ref += fmt.Sprintf(" (https://picsum.photos/id/%s/%d/%d)", photoID, width, height)
	}
	mode := palette
	if mode == "rgb" {
		mode += " input"
	}
	fmt.Fprintf(r.log, "Rendered Picsum photo %s: 800x480 %s, %d bytes\n", ref, mode, len(output))
	return output, nil
}

func rotate(src image.Image, clockwise int) image.Image {
	switch clockwise {
	case 90:
		return imaging.Rotate270(src)
	case 180:
		return imaging.Rotate180(src)
	case 270:
		return imaging.Rotate90(src)
	default:
		return src
	}
}

func rotateGray(src *image.Gray, clockwise int) *image.Gray {
	if clockwise == 0 {
		return src
	}
	width, height := src.Rect.Dx(), src.Rect.Dy()
	bounds := image.Rect(0, 0, width, height)
	if clockwise != 180 {
		bounds = image.Rect(0, 0, height, width)
	}
	dst := image.NewGray(bounds)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx, dy := width-1-x, height-1-y
			if clockwise == 90 {
				dx, dy = height-1-y, x
			} else if clockwise == 270 {
				dx, dy = y, width-1-x
			}
			dst.Pix[dy*dst.Stride+dx] = src.Pix[y*src.Stride+x]
		}
	}
	return dst
}

func autocontrast(gray *image.Gray) {
	var histogram [256]int
	for _, v := range gray.Pix {
		histogram[v]++
	}
	// Discard 0.5 percent from each end, matching the previous cutoff.
	cutoff := len(gray.Pix) / 200
	low, high := 0, 255
	for count := cutoff; low < 255 && count >= histogram[low]; low++ {
		count -= histogram[low]
	}
	for count := cutoff; high > 0 && count >= histogram[high]; high-- {
		count -= histogram[high]
	}
	if high <= low {
		return
	}
	for i, v := range gray.Pix {
		gray.Pix[i] = uint8(min(255, max(0, (int(v)-low)*255/(high-low))))
	}
}

func quantize(gray *image.Gray, palette string, dither bool) *image.Gray {
	step := 85
	if palette == "bw" {
		step = 255
	}
	dst := image.NewGray(gray.Bounds())
	width, height := gray.Rect.Dx(), gray.Rect.Dy()
	current, next := make([]int, width+2), make([]int, width+2)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := int(gray.Pix[y*gray.Stride+x])
			if !dither {
				dst.Pix[y*dst.Stride+x] = uint8(min(255, (v+step/2)/step*step))
				continue
			}
			// Keep fractional error in sixteenths for Floyd-Steinberg diffusion.
			scaled := min(255*16, max(0, v*16+current[x+1]))
			shade := min(255, (scaled+step*8)/(step*16)*step)
			dst.Pix[y*dst.Stride+x] = uint8(shade)
			err := scaled - shade*16
			current[x+2] += err * 7 / 16
			next[x] += err * 3 / 16
			next[x+1] += err * 5 / 16
			next[x+2] += err / 16
		}
		current, next = next, current
		clear(next)
	}
	return dst
}

func writeChunk(output *bytes.Buffer, kind string, data []byte) {
	binary.Write(output, binary.BigEndian, uint32(len(data)))
	output.WriteString(kind)
	output.Write(data)
	checksum := crc32.NewIEEE()
	io.WriteString(checksum, kind)
	checksum.Write(data)
	binary.Write(output, binary.BigEndian, checksum.Sum32())
}

func encodeGray(gray *image.Gray) ([]byte, error) {
	width, height := gray.Rect.Dx(), gray.Rect.Dy()
	// The standard PNG encoder writes Gray as 8-bit; the contract requires 2-bit.
	stride := (width+3)/4 + 1
	rows := make([]byte, stride*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rows[y*stride+1+x/4] |= (gray.Pix[y*gray.Stride+x] / 85) << (6 - 2*(x%4))
		}
	}
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(rows); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString("\x89PNG\r\n\x1a\n")
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, uint32(width))
	binary.BigEndian.PutUint32(header[4:], uint32(height))
	header[8] = 2
	writeChunk(&output, "IHDR", header)
	writeChunk(&output, "IDAT", compressed.Bytes())
	writeChunk(&output, "IEND", nil)
	return output.Bytes(), nil
}
