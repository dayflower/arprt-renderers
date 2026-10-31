package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const imageLimit = 2 << 20
const imagePixels = 4_000_000

// Deny special-use destinations as well as private networks. Only public
// unicast addresses may be supplied by untrusted feed image URLs.
var imageDenied = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

func publicImageIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range imageDenied {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

type imageLookup func(context.Context, string, string) ([]netip.Addr, error)
type imageDial func(context.Context, string, string) (net.Conn, error)

func imageDialer(lookup imageLookup, dial imageDial) imageDial {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid image address")
		}
		ips, err := lookup(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("image DNS failed")
		}
		// Reject mixed public/private answers before attempting any connection.
		for _, ip := range ips {
			if !publicImageIP(ip) {
				return nil, errors.New("image destination rejected")
			}
		}
		for _, ip := range ips {
			// Dial the validated IP itself, preventing a second DNS lookup/rebinding.
			conn, e := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
		}
		return nil, errors.New("image connection failed")
	}
}
func newImageClient(publicOnly bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dial := (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	transport.DialContext = dial
	if publicOnly {
		transport.DialContext = imageDialer(net.DefaultResolver.LookupNetIP, dial)
	}
	transport.MaxConnsPerHost = 2
	client := newHTTPClient()
	client.Transport = transport
	return client
}

// An injected client permits loopback stubs in tests; production always uses
// newImageClient, independently of the operator-configured feed/capture client.
func loadImages(parent context.Context, client *http.Client, f *feed, maxItems int) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	jobs := make(chan int)
	for range 2 {
		wg.Go(func() {
			for i := range jobs {
				data, err := fetchThumbnail(ctx, client, f.Articles[i].ImageURL)
				if err == nil {
					f.Articles[i].ImageData = data
				}
			}
		})
	}
	for i := 0; i < len(f.Articles) && i < maxItems; i++ {
		if f.Articles[i].ImageURL == "" {
			continue
		}
		select {
		case jobs <- i:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
}
func fetchThumbnail(parent context.Context, client *http.Client, address string) (string, error) {
	if httpURL(address) == "" {
		return "", errors.New("invalid image URL")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", errors.New("invalid image request")
	}
	req.Header.Set("Accept", "image/png, image/jpeg, image/gif, image/webp")
	req.Header.Set("User-Agent", "arprt-rss/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("image request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("image HTTP failure")
	}
	data, err := readBounded(resp.Body, imageLimit)
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	result, err := thumbnail(data)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return result, err
}
func validImageSize(w, h int) bool { return w > 0 && h > 0 && int64(w)*int64(h) <= imagePixels }

// Convert only the first WebP animation frame to a standalone WebP. Bound the
// canvas and frame before decoding, including animations with small first frames.
func firstWebPFrame(data []byte) ([]byte, image.Point, image.Point, color.NRGBA, bool, error) {
	var canvas, offset image.Point
	var background color.NRGBA
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return data, canvas, offset, background, false, nil
	}
	invalid := errors.New("invalid WebP animation")
	end := int64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if end > int64(len(data)) || end < 12 {
		return nil, canvas, offset, background, false, invalid
	}
	u24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
	animated := false
	for pos := 12; int64(pos)+8 <= end; {
		size := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		next := int64(pos) + 8 + size
		if next > end {
			return nil, canvas, offset, background, false, invalid
		}
		chunk := data[pos+8 : int(next)]
		switch string(data[pos : pos+4]) {
		case "VP8X":
			if len(chunk) != 10 {
				return nil, canvas, offset, background, false, invalid
			}
			animated = chunk[0]&2 != 0
			canvas = image.Pt(u24(chunk[4:7])+1, u24(chunk[7:10])+1)
			if !validImageSize(canvas.X, canvas.Y) {
				return nil, canvas, offset, background, false, invalid
			}
		case "ANIM":
			if len(chunk) != 6 {
				return nil, canvas, offset, background, false, invalid
			}
			background = color.NRGBA{chunk[2], chunk[1], chunk[0], chunk[3]}
		case "ANMF":
			if !animated || len(chunk) < 16 {
				return nil, canvas, offset, background, false, invalid
			}
			offset = image.Pt(2*u24(chunk[:3]), 2*u24(chunk[3:6]))
			w, h := u24(chunk[6:9])+1, u24(chunk[9:12])+1
			if !validImageSize(w, h) || offset.X+w > canvas.X || offset.Y+h > canvas.Y {
				return nil, canvas, offset, background, false, invalid
			}
			// A VP8X header supplies dimensions for an optional ALPH chunk.
			header := make([]byte, 18)
			copy(header, "VP8X")
			binary.LittleEndian.PutUint32(header[4:8], 10)
			if len(chunk) >= 20 && string(chunk[16:20]) == "ALPH" {
				header[8] = 16
			}
			copy(header[12:15], chunk[6:9])
			copy(header[15:18], chunk[9:12])
			out := append([]byte("RIFF\x00\x00\x00\x00WEBP"), header...)
			out = append(out, chunk[16:]...)
			binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
			return out, canvas, offset, background, chunk[15]&2 == 0, nil
		}
		pos = int(next + size%2)
	}
	if animated {
		return nil, canvas, offset, background, false, invalid
	}
	return data, image.Point{}, offset, background, false, nil
}
func thumbnail(data []byte) (string, error) {
	data, canvas, offset, background, blend, err := firstWebPFrame(data)
	if err != nil {
		return "", err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !validImageSize(config.Width, config.Height) {
		return "", errors.New("invalid image dimensions or format")
	}
	switch format {
	case "jpeg", "png", "gif", "webp":
	default:
		return "", errors.New("unsupported image format")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("invalid image pixels")
	}
	if src.Bounds().Dx() != config.Width || src.Bounds().Dy() != config.Height {
		return "", errors.New("image dimensions changed")
	}
	if canvas.X != 0 {
		frame := image.NewNRGBA(image.Rectangle{Max: canvas})
		draw.Draw(frame, frame.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
		op := draw.Src
		if blend {
			op = draw.Over
		}
		draw.Draw(frame, src.Bounds().Add(offset), src, src.Bounds().Min, op)
		src = frame
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	// Fit within 96 x 72 without enlargement, preserving even extreme ratios.
	tw, th := w, h
	if tw > 96 {
		th = max(1, h*96/w)
		tw = 96
	}
	if th > 72 {
		tw = max(1, tw*72/th)
		th = 72
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, dst, &jpeg.Options{Quality: 75}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// Only locally re-encoded JPEGs become trusted template URLs.
func validThumbnail(data string) bool {
	const prefix = "data:image/jpeg;base64,"
	if len(data) < len(prefix) || data[:len(prefix)] != prefix || len(data) > 32<<10 {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(data[len(prefix):])
	if err != nil {
		return false
	}
	c, err := jpeg.DecodeConfig(bytes.NewReader(b))
	return err == nil && c.Width <= 96 && c.Height <= 72 && validImageSize(c.Width, c.Height)
}

// A smaller, lower-quality retry gives text priority near the HTML body limit.
func reduceThumbnail(data string) string {
	encoded, err := base64.StdEncoding.DecodeString(data[len("data:image/jpeg;base64,"):])
	if err != nil {
		return ""
	}
	src, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		return ""
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(1, src.Bounds().Dx()/2), max(1, src.Bounds().Dy()/2)))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var b bytes.Buffer
	if jpeg.Encode(&b, dst, &jpeg.Options{Quality: 45}) != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
