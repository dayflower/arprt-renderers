package main

import (
	"bytes"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func contextJSON(rotation int, palette string) []byte {
	return fmt.Appendf(nil, `{"version":"v1","renderTarget":{"width":800,"height":480,"palette":%q,"rotation":%d},"device":{"name":"example","battery":{"voltage":3.91}}}`, palette, rotation)
}

func contextFor(t *testing.T, rotation int, palette string) renderContext {
	t.Helper()
	context, err := parseContext(contextJSON(rotation, "gray4"), palette)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

var grayCorners = []color.NRGBA{{0, 0, 0, 255}, {85, 85, 85, 255}, {170, 170, 170, 255}, {255, 255, 255, 255}}
var rgbCorners = []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}}

func photo(t *testing.T, colors []color.NRGBA) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 800, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 800; x++ {
			src.SetNRGBA(x, y, colors[(y/240*2+x/400)%len(colors)])
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, src); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertPNG(t *testing.T, data []byte, palette string) image.Image {
	t.Helper()
	depth, kind := byte(2), byte(0)
	if palette == "rgb" {
		depth, kind = 8, 2
	} else if len(data) >= pngLimit {
		t.Fatalf("PNG exceeds exclusive size limit: %d", len(data))
	}
	if len(data) < 33 || string(data[:8]) != "\x89PNG\r\n\x1a\n" ||
		binary.BigEndian.Uint32(data[16:20]) != 800 || binary.BigEndian.Uint32(data[20:24]) != 480 ||
		!bytes.Equal(data[24:29], []byte{depth, kind, 0, 0, 0}) {
		t.Fatal("unexpected PNG header")
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if palette != "rgb" {
		for y := 0; y < 480; y++ {
			for x := 0; x < 800; x++ {
				v := color.GrayModel.Convert(decoded.At(x, y)).(color.Gray).Y
				if v%85 != 0 || (palette == "bw" && v != 0 && v != 255) {
					t.Fatalf("unexpected shade %d", v)
				}
			}
		}
	}
	return decoded
}

func TestPalettesAndClockwiseRotation(t *testing.T) {
	orders := [][]int{{0, 1, 2, 3}, {2, 0, 3, 1}, {3, 2, 1, 0}, {1, 3, 0, 2}}
	points := []image.Point{{100, 60}, {700, 60}, {100, 420}, {700, 420}}
	for _, palette := range []string{"gray4", "bw", "rgb"} {
		colors := grayCorners
		if palette == "rgb" {
			colors = rgbCorners
		}
		source := photo(t, colors)
		for index, order := range orders {
			t.Run(fmt.Sprintf("%s/%d", palette, index*90), func(t *testing.T) {
				data, err := newRenderer(io.Discard).render(contextFor(t, index*90, palette), source, palette)
				if err != nil {
					t.Fatal(err)
				}
				decoded := assertPNG(t, data, palette)
				for i, p := range points {
					want := colors[order[i]]
					if palette != "bw" {
						if got := color.NRGBAModel.Convert(decoded.At(p.X, p.Y)); got != want {
							t.Fatalf("corner %d: got %v, want %v", i, got, want)
						}
						continue
					}
					// Dithered regions should retain their average brightness.
					sum := 0
					for y := p.Y - 20; y < p.Y+20; y++ {
						for x := p.X - 20; x < p.X+20; x++ {
							sum += int(color.GrayModel.Convert(decoded.At(x, y)).(color.Gray).Y)
						}
					}
					if mean := sum / 1600; mean < int(want.R)-5 || mean > int(want.R)+5 {
						t.Fatalf("corner brightness: got %d, want %d", mean, want.R)
					}
				}
			})
		}
	}
}

func TestContextValidation(t *testing.T) {
	for _, fixture := range []struct{ name, palette string }{
		{"render-context-v1.json", "gray4"}, {"render-context-spectra6.json", "rgb"},
	} {
		data, err := os.ReadFile(filepath.Join("fixtures", fixture.name))
		if err != nil {
			t.Fatal(err)
		}
		context, err := parseContext(data, fixture.palette)
		if err != nil {
			t.Fatal(err)
		}
		png, err := newRenderer(io.Discard).render(context, photo(t, rgbCorners), fixture.palette)
		if err != nil {
			t.Fatal(err)
		}
		assertPNG(t, png, fixture.palette)
	}
	for _, palette := range []string{"gray4", "bw", "rgb"} {
		for _, field := range []struct {
			key   string
			value any
		}{
			{"version", "v2"}, {"width", 480}, {"height", 800}, {"palette", "rgb"}, {"palette", "gray16"},
			{"rotation", 45}, {"rotation", false}, {"rotation", nil}, {"rotation", "90"}, {"rotation", 90.5},
		} {
			var object map[string]any
			json.Unmarshal(contextJSON(0, "gray4"), &object)
			if field.key == "version" {
				object[field.key] = field.value
			} else {
				object["renderTarget"].(map[string]any)[field.key] = field.value
			}
			data, _ := json.Marshal(object)
			if _, err := parseContext(data, palette); err == nil {
				t.Fatalf("accepted %s=%v for %s", field.key, field.value, palette)
			}
		}
		_, err := parseContext(contextJSON(0, "spectra6"), palette)
		if (err == nil) != (palette == "rgb") {
			t.Fatalf("unexpected spectra6 validation: %s: %v", palette, err)
		}
	}
	if _, err := parseContext(contextJSON(0, "gray4"), "invalid"); err == nil {
		t.Fatal("accepted invalid CLI palette")
	}
	if _, err := parseContext([]byte(`{"version":"v1","futureField":true,"renderTarget":{"width":800,"height":480,"palette":"gray4","futureField":{}}}`), "gray4"); err != nil {
		t.Fatalf("omitted rotation or unknown fields: %v", err)
	}
}

func TestExclusiveBudgetFallbackAndFailure(t *testing.T) {
	source := photo(t, []color.NRGBA{{100, 100, 100, 255}})
	for _, palette := range []string{"gray4", "bw"} {
		for _, rotation := range []int{0, 90, 180, 270} {
			r := newRenderer(io.Discard)
			calls := 0
			r.encode = func(gray *image.Gray) ([]byte, error) {
				calls++
				if calls == 1 {
					return make([]byte, pngLimit), nil
				}
				want := byte(85)
				if palette == "bw" {
					want = 0
				}
				for _, v := range gray.Pix {
					if v != want {
						t.Fatal("fallback must use direct, undithered quantization")
					}
				}
				return encodeGray(gray)
			}
			data, err := r.render(contextFor(t, rotation, palette), source, palette)
			if err != nil || calls != 2 {
				t.Fatalf("fallback: %d calls: %v", calls, err)
			}
			assertPNG(t, data, palette)
		}
	}
	r := newRenderer(io.Discard)
	r.encode = func(*image.Gray) ([]byte, error) { return make([]byte, pngLimit), nil }
	if _, err := r.render(contextFor(t, 0, "gray4"), source, "gray4"); err == nil || !strings.Contains(err.Error(), "PNG exceeds") {
		t.Fatalf("expected size failure, got %v", err)
	}
	r.encode = func(*image.Gray) ([]byte, error) { return nil, errors.New("encoder failed") }
	if _, err := r.render(contextFor(t, 0, "gray4"), source, "gray4"); err == nil {
		t.Fatal("ignored encoder failure")
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

type trackedBody struct {
	io.Reader
	closed bool
	read   int
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestDownloadBoundaryAndPhotoLogging(t *testing.T) {
	var log bytes.Buffer
	r := newRenderer(&log)
	if r.client.(*http.Client).Timeout != 20*time.Second {
		t.Fatal("missing request timeout")
	}
	previous := ""
	for _, id := range []string{"123", "", "unknown"} {
		body := &trackedBody{Reader: bytes.NewReader(photo(t, grayCorners))}
		r.client = doerFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != "GET" || req.Body != nil || req.URL.Scheme != "https" || req.URL.Host != "picsum.photos" || req.URL.Path != "/480/800.jpg" || req.Header.Get("User-Agent") != "arprt-picsum/1.0" {
				t.Fatalf("unexpected request: %v", req)
			}
			query := req.URL.Query()
			if len(query) != 1 || len(query.Get("random")) != 32 || query.Get("random") == previous {
				t.Fatal("request must contain only a fresh random value")
			}
			previous = query.Get("random")
			return &http.Response{StatusCode: 200, Header: http.Header{"Picsum-Id": {id}}, Body: body}, nil
		})
		log.Reset()
		data, err := r.render(contextFor(t, 90, "gray4"), nil, "gray4")
		if err != nil || !body.closed {
			t.Fatalf("download: %v, closed=%v", err, body.closed)
		}
		assertPNG(t, data, "gray4")
		if got := strings.Contains(log.String(), "https://picsum.photos/id/123/480/800"); got != (id == "123") {
			t.Fatalf("unexpected photo URL: %s", log.String())
		}
	}
	log.Reset()
	if _, err := r.render(contextFor(t, 0, "gray4"), photo(t, grayCorners), "gray4"); err != nil || strings.Contains(log.String(), "https://") {
		t.Fatalf("local photo logging: %s, %v", log.String(), err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDownloadRetriesAndSizeLimit(t *testing.T) {
	for _, mode := range []string{"transport", "status", "body", "oversize", "success-after-failure"} {
		t.Run(mode, func(t *testing.T) {
			r := newRenderer(io.Discard)
			calls, sleeps := 0, 0
			var bodies []*trackedBody
			r.sleep = func(d time.Duration) {
				if d != 2*time.Second {
					t.Fatal("unexpected retry delay")
				}
				sleeps++
			}
			r.client = doerFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if mode == "transport" || (mode == "success-after-failure" && calls == 1) {
					return nil, errors.New("offline")
				}
				status := 200
				var reader io.Reader = strings.NewReader("ok")
				switch mode {
				case "status":
					status = 503
				case "body":
					reader = errorReader{}
				case "oversize":
					reader = io.LimitReader(zeroReader{}, downloadLimit*2)
				}
				body := &trackedBody{Reader: reader}
				bodies = append(bodies, body)
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}, nil
			})
			_, _, err := r.download(800, 480)
			wantCalls := 3
			if mode == "oversize" {
				wantCalls = 1
			}
			if mode == "success-after-failure" {
				wantCalls = 2
			}
			if (err == nil) != (mode == "success-after-failure") || calls != wantCalls || sleeps != wantCalls-1 {
				t.Fatalf("calls=%d sleeps=%d err=%v", calls, sleeps, err)
			}
			for _, body := range bodies {
				if !body.closed || body.read > downloadLimit+1 {
					t.Fatalf("unbounded/unclosed response: %+v", body)
				}
			}
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestCLIAndAtomicOutput(t *testing.T) {
	dir := t.TempDir()
	context, source, output := filepath.Join(dir, "context.json"), filepath.Join(dir, "photo.png"), filepath.Join(dir, "output.png")
	os.WriteFile(source, photo(t, rgbCorners), 0o644)
	for _, palette := range []string{"gray4", "bw", "rgb"} {
		os.WriteFile(context, contextJSON(90, "gray4"), 0o644)
		args := []string{"--context", context, "--input", source, "--output", output, "--palette", palette}
		if err := run(args, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		assertPNG(t, data, palette)
		os.WriteFile(source, []byte("invalid image"), 0o644)
		if err := run(args, io.Discard, io.Discard); err == nil {
			t.Fatal("accepted corrupt input")
		}
		after, _ := os.ReadFile(output)
		if !bytes.Equal(data, after) {
			t.Fatal("failed render replaced prior output")
		}
		os.WriteFile(source, photo(t, rgbCorners), 0o644)
	}
	// Failed rename must remove the temporary file and preserve its destination.
	if err := writeOutput(dir, []byte("unpublished")); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatal("leaked temporary output")
		}
	}
	if err := writeOutput(filepath.Join(dir, "missing", "output.png"), nil); err == nil {
		t.Fatal("created missing output parent")
	}
	for _, args := range [][]string{{"--palette", "invalid", "--context", context}, {"unexpected"}, {"--unknown"}} {
		if err := run(args, io.Discard, io.Discard); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}

func TestEXIFOrientation(t *testing.T) {
	src, _, err := image.Decode(bytes.NewReader(photo(t, rgbCorners)))
	if err != nil {
		t.Fatal(err)
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, src, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	// Synthetic little-endian TIFF IFD containing only the EXIF orientation tag.
	orders := [][]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	for orientation, order := range orders {
		exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(orientation + 1), 0, 0, 0, 0, 0, 0, 0}
		data := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
		data = append(data, jpegData.Bytes()[2:]...)
		output, err := newRenderer(io.Discard).render(contextFor(t, 0, "rgb"), data, "rgb")
		if err != nil {
			t.Fatal(err)
		}
		decoded := assertPNG(t, output, "rgb")
		for i, point := range []image.Point{{100, 60}, {700, 60}, {100, 420}, {700, 420}} {
			got := color.NRGBAModel.Convert(decoded.At(point.X, point.Y)).(color.NRGBA)
			want := rgbCorners[order[i]]
			for channel, v := range []uint8{got.R, got.G, got.B} {
				w := []uint8{want.R, want.G, want.B}[channel]
				if int(v)-int(w) > 3 || int(w)-int(v) > 3 {
					t.Fatalf("EXIF %d corner %d: got %v want %v", orientation+1, i, got, want)
				}
			}
		}
	}
}

func TestAutocontrast(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 100, 2))
	for i := range gray.Pix {
		gray.Pix[i] = 100
		if i >= 100 {
			gray.Pix[i] = 200
		}
	}
	gray.Pix[0], gray.Pix[199] = 0, 255
	autocontrast(gray)
	if gray.Pix[0] != 0 || gray.Pix[1] != 0 || gray.Pix[100] != 255 || gray.Pix[199] != 255 {
		t.Fatal("autocontrast did not discard 0.5 percent at each end")
	}
}

func TestJobEnvironment(t *testing.T) {
	if os.Getenv("PICSUM_JOB_CHECK") != "1" {
		t.Skip("only runs inside the restricted production image")
	}
	if os.Getuid() != 1000 || os.Getgid() != 1000 {
		t.Fatal("unexpected runtime UID/GID")
	}
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); !os.IsNotExist(err) {
		t.Fatal("unexpected ServiceAccount token")
	}
	for _, path := range []string{"/tmp/probe", "/app/probe", "/fixtures/probe", "/arprt/context.json"} {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err == nil {
			file.Close()
			t.Fatalf("unexpected writable path: %s", path)
		}
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(status, []byte("NoNewPrivs:\t1\n")) || !bytes.Contains(status, []byte("CapEff:\t0000000000000000\n")) {
		t.Fatal("missing Job security restrictions")
	}
	pool, err := x509.SystemCertPool()
	if err != nil || len(pool.Subjects()) == 0 {
		t.Fatal("missing HTTPS root certificates")
	}
}
