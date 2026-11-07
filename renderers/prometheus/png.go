package prometheus

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const pngLimit = 32 << 20

func decodePNG(data []byte, width, height int) (image.Image, error) {
	if len(data) > pngLimit || len(data) < 8 || !bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("invalid PNG signature or size")
	}
	// Go's decoder stops at IEND. Check every chunk CRC and reject trailing bytes,
	// animation, and extra IEND payload before handing it to the decoder.
	ended := false
	for offset := 8; offset < len(data); {
		if len(data)-offset < 12 {
			return nil, errors.New("truncated PNG chunk")
		}
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length > len(data)-offset-12 {
			return nil, errors.New("truncated PNG data")
		}
		chunk := data[offset+4 : offset+8+length]
		kind := string(chunk[:4])
		if crc32.ChecksumIEEE(chunk) != binary.BigEndian.Uint32(data[offset+8+length:offset+12+length]) {
			return nil, errors.New("invalid PNG checksum")
		}
		if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
			return nil, errors.New("capture must be a static PNG")
		}
		offset += length + 12
		if kind == "IEND" {
			if length != 0 || offset != len(data) {
				return nil, errors.New("invalid PNG ending")
			}
			ended = true
		}
	}
	if !ended {
		return nil, errors.New("missing PNG ending")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != width || config.Height != height {
		return nil, errors.New("capture PNG dimensions do not match logical canvas")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid capture PNG")
	}
	return img, nil
}
func outputPNG(src image.Image, rotation int) ([]byte, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if rotation == 90 || rotation == 270 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch rotation {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			case 270:
				dx, dy = y, w-1-x
			}
			r, g, b, a := src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y).RGBA()
			// RGBA returns premultiplied channels; composite transparency onto white.
			dst.SetRGBA(dx, dy, color.RGBA{uint8((r + 65535 - a) >> 8), uint8((g + 65535 - a) >> 8), uint8((b + 65535 - a) >> 8), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, dst); err != nil {
		return nil, err
	}
	if b.Len() > pngLimit {
		return nil, errors.New("output PNG exceeds controller input limit")
	}
	return b.Bytes(), nil
}
func writeOutput(ctx context.Context, path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".prometheus-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
