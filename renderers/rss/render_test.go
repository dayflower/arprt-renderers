package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(w-1, 0, color.NRGBA{0, 255, 0, 255})
	img.SetNRGBA(0, h-1, color.NRGBA{0, 0, 255, 255})
	img.SetNRGBA(w-1, h-1, color.NRGBA{0, 0, 0, 0})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func contextBytes(rotation int, palette string) []byte {
	return []byte(fmt.Sprintf(`{"version":"v1","renderTarget":{"width":800,"height":480,"palette":%q,"rotation":%d},"device":{"name":"synthetic-display"},"unknown":true}`, palette, rotation))
}
func TestContextValidation(t *testing.T) {
	for _, p := range []string{"gray4", "spectra6"} {
		for _, r := range []int{0, 90, 180, 270} {
			c, err := parseContext(contextBytes(r, p))
			if err != nil || c.rotation != r {
				t.Fatal(c, err)
			}
		}
	}
	base := string(contextBytes(0, "gray4"))
	for _, bad := range []string{
		strings.Replace(base, `"v1"`, `"v2"`, 1), strings.Replace(base, `800`, `480`, 1), strings.Replace(base, `"gray4"`, `"gray16"`, 1),
		strings.Replace(base, `"rotation":0`, `"rotation":null`, 1), strings.Replace(base, `"rotation":0`, `"rotation":"90"`, 1),
		strings.Replace(base, `"rotation":0`, `"rotation":45`, 1), strings.Replace(base, `"rotation":0`, `"rotation":90.0`, 1), strings.Replace(base, `"synthetic-display"`, `""`, 1), `{}`,
	} {
		if _, err := parseContext([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	c, err := parseContext(fixture(t, "render-context-v1.json"))
	if err != nil || c.rotation != 0 {
		t.Fatal("omitted rotation rejected")
	}
}
func assertRotated(t *testing.T, data []byte, r int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 800, 480) {
		t.Fatal(img.Bounds())
	}
	// Source top-left is red, top-right green, bottom-left blue and bottom-right
	// transparent (white after compositing). Check positions, not only dimensions.
	corners := [][2]int{{0, 0}, {799, 0}, {0, 479}, {799, 479}}
	expected := map[int][]color.RGBA{
		0:   {{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}},
		90:  {{0, 0, 255, 255}, {255, 0, 0, 255}, {255, 255, 255, 255}, {0, 255, 0, 255}},
		180: {{255, 255, 255, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {255, 0, 0, 255}},
		270: {{0, 255, 0, 255}, {255, 255, 255, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}},
	}
	for i, p := range corners {
		got := color.RGBAModel.Convert(img.At(p[0], p[1])).(color.RGBA)
		if got != expected[r][i] {
			t.Fatalf("rotation %d corner %d: %v want %v", r, i, got, expected[r][i])
		}
	}
	if data[25] != 2 || data[24] != 8 {
		t.Fatal("output is not 8-bit RGB PNG")
	}
}
func TestPNGValidationAndRotation(t *testing.T) {
	for _, r := range []int{0, 90, 180, 270} {
		c, _ := parseContext(contextBytes(r, "gray4"))
		w, h := c.logicalSize()
		img, err := decodePNG(pngBytes(t, w, h), w, h)
		if err != nil {
			t.Fatal(err)
		}
		out, err := outputPNG(img, r)
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, out, r)
	}
	valid := pngBytes(t, 800, 480)
	badCRC := bytes.Clone(valid)
	badCRC[29] ^= 1
	for name, b := range map[string][]byte{"json": []byte(`{"error":"failed"}`), "truncated": valid[:len(valid)-2], "trailing": append(bytes.Clone(valid), 'x'), "crc": badCRC, "wrong-size": pngBytes(t, 480, 800)} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodePNG(b, 800, 480); err == nil {
				t.Fatal("accepted bad PNG")
			}
		})
	}
	// Animated input must not silently become a valid capture response.
	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(8))
	chunk.WriteString("acTL")
	chunk.Write(make([]byte, 8))
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()[4:]))
	animated := append(bytes.Clone(valid[:33]), chunk.Bytes()...)
	animated = append(animated, valid[33:]...)
	if _, err := decodePNG(animated, 800, 480); err == nil {
		t.Fatal("accepted animation")
	}
}
func TestFeedHTTPBoundaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write(fixture(t, "rss2.xml"))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			_, _ = io.WriteString(z, strings.Repeat("x", feedLimit+1))
			z.Close()
		case "/large":
			_, _ = io.WriteString(w, strings.Repeat("x", feedLimit+1))
		case "/cut":
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, "short")
		case "/timeout":
			<-r.Context().Done()
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	for _, p := range []string{"/ok", "/redirect"} {
		if _, err := fetchFeed(context.Background(), newHTTPClient(), server.URL+p); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"/gzip", "/large", "/cut", "/loop", "/failure", "/timeout"} {
		t.Run(p, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if _, err := fetchFeed(ctx, newHTTPClient(), server.URL+p); err == nil {
				t.Fatal("accepted failing feed")
			}
		})
	}
}
func TestCaptureHTTP(t *testing.T) {
	success := pngBytes(t, 800, 480)
	var attempts atomic.Int32
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); w.Write(success) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != "<html>合成</html>" || r.Header.Get("Content-Type") != "text/html; charset=utf-8" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("incorrect capture request")
		}
		q := r.URL.Query()
		if q.Get("width") != "800" || q.Get("height") != "480" || q.Get("html_mode") != "file" || q.Get("ready_selector") != "#rss-ready" || q.Has("output_width") || q.Has("full_page") || q.Has("profile") {
			t.Error("incorrect capture query")
		}
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/ok/capture":
			w.Write(success)
		case "/retry/capture":
			if attempts.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(503)
			} else {
				w.Write(success)
			}
		case "/retry504/capture":
			if attempts.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(504)
			} else {
				w.Write(success)
			}
		case "/always/capture":
			attempts.Add(1)
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(503)
		case "/bad/capture":
			attempts.Add(1)
			w.WriteHeader(400)
		case "/redirect/capture":
			http.Redirect(w, r, destination.URL, 307)
		case "/json/capture":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"error":"failed"}`))
		case "/fake/capture":
			w.Write([]byte(`{"error":"failed"}`))
		case "/size/capture":
			w.Write(pngBytes(t, 480, 800))
		case "/timeout/capture":
			<-r.Context().Done()
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	for _, p := range []string{"/ok", "/retry", "/retry504"} {
		attempts.Store(0)
		if _, err := capture(context.Background(), newHTTPClient(), server.URL+p, "synthetic-token", []byte("<html>合成</html>"), 800, 480); err != nil {
			t.Fatal(err)
		}
		if p != "/ok" && attempts.Load() != 2 {
			t.Fatal("retry count")
		}
	}
	for _, p := range []string{"/always", "/bad", "/redirect", "/json", "/fake", "/size", "/timeout"} {
		t.Run(p, func(t *testing.T) {
			attempts.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if _, err := capture(ctx, newHTTPClient(), server.URL+p, "synthetic-token", []byte("<html>合成</html>"), 800, 480); err == nil {
				t.Fatal("accepted capture error")
			}
			if p == "/always" && attempts.Load() != 2 || p == "/bad" && attempts.Load() != 1 {
				t.Fatal("incorrect retries")
			}
		})
	}
	if forwarded.Load() != 0 {
		t.Fatal("capture followed redirect")
	}
	for _, base := range []string{"file:///tmp/a", "https://user:pass@example.com", "https://example.com?x=1", "https://example.com/#fragment"} {
		if _, err := captureURL(base, 800, 480); err == nil {
			t.Fatalf("accepted URL %s", base)
		}
	}
}
func TestAtomicWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "out.png")
	prior := []byte("prior output")
	if err := os.WriteFile(path, prior, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeOutput(ctx, path, []byte("new")); err == nil {
		t.Fatal("cancelled write succeeded")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, prior) {
		t.Fatal("prior output changed")
	}
	entries, _ := filepath.Glob(filepath.Join(root, ".rss-*.tmp"))
	if len(entries) != 0 {
		t.Fatal("temporary files leaked")
	}
	if err := writeOutput(context.Background(), path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	// Rename failure must also clean its sibling temporary file.
	if err := writeOutput(context.Background(), root, []byte("new")); err == nil {
		t.Fatal("rename to directory succeeded")
	}
	entries, _ = filepath.Glob(filepath.Join(root, ".rss-*.tmp"))
	if len(entries) != 0 {
		t.Fatal("temporary files leaked after rename failure")
	}
}
func TestCLI(t *testing.T) { checkCLI(t, "") }
func TestProductionCLI(t *testing.T) {
	binary := os.Getenv("RSS_CLI")
	if binary == "" {
		t.Skip("set RSS_CLI to test the shipped executable")
	}
	if os.Getuid() != 1000 || os.Getgid() != 1000 {
		t.Fatal("Job identity is not UID/GID 1000")
	}
	if err := os.WriteFile("/app/forbidden", []byte("x"), 0o600); err == nil {
		t.Fatal("runtime root is writable")
	}
	for _, path := range []string{"/arprt/context.json", "/input/context.json"} {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			t.Fatalf("Context mount is writable: %s", path)
		}
	}
	checkCLI(t, binary)
}
func checkCLI(t *testing.T, binary string) {
	t.Helper()
	root := t.TempDir()
	contextPath := filepath.Join(root, "context.json")
	output := filepath.Join(root, "result.png")
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("synthetic-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var failed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			w.Write(fixture(t, "rss2.xml"))
			return
		}
		if r.URL.Path != "/capture" {
			t.Error("wrong capture path")
		}
		if failed.Load() {
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("Bearer token missing")
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte("合成ニュース")) && !bytes.Contains(body, []byte("合成RDF")) && !bytes.Contains(body, []byte("Empty feed")) {
			t.Error("HTML does not include feed title")
		}
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		height, _ := strconv.Atoi(r.URL.Query().Get("height"))
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes(t, width, height))
	}))
	defer server.Close()
	invoke := func(args []string) error {
		if binary != "" {
			cmd := exec.Command(binary, args...)
			b, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("CLI failed: %s: %w", b, err)
			}
			return nil
		}
		return run(context.Background(), args, io.Discard, io.Discard)
	}
	for _, palette := range []string{"gray4", "spectra6"} {
		for _, r := range []int{0, 90, 180, 270} {
			if err := os.WriteFile(contextPath, contextBytes(r, palette), 0o644); err != nil {
				t.Fatal(err)
			}
			args := []string{"--context", contextPath, "--output", output, "--input", "fixtures/rss2.xml", "--webcappi-url", server.URL, "--webcappi-token-file", token}
			if r == 180 {
				args = append(args[:4], "--feed-url", server.URL+"/feed", "--webcappi-url", server.URL, "--webcappi-token-file", token)
			}
			if err := invoke(args); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			assertRotated(t, data, r)
			failed.Store(true)
			if err := invoke(args); err == nil {
				t.Fatal("capture failure succeeded")
			}
			failed.Store(false)
			after, _ := os.ReadFile(output)
			if !bytes.Equal(data, after) {
				t.Fatal("capture failure replaced previous output")
			}
			entries, _ := filepath.Glob(filepath.Join(root, ".rss-*.tmp"))
			if len(entries) != 0 {
				t.Fatal("CLI temporary file leak")
			}
		}
	}
	if binary != "" {
		// The checker mounts the Context at the controller's default read-only path.
		args := []string{"--input", "fixtures/rss2.xml", "--webcappi-url", server.URL, "--webcappi-token-file", token}
		if err := invoke(args); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile("/arprt/output.png")
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, data, 0)
		// Exercise a read-only Context mounted outside the output parent.
		args = append(args, "--context", "/input/context.json", "--output", output)
		if err := invoke(args); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, data, 0)
	}
	// A valid empty feed still renders; malformed feeds never reach capture.
	for _, name := range []string{"rss1.xml", "empty.xml"} {
		if err := invoke([]string{"--context", contextPath, "--output", output, "--input", "fixtures/" + name, "--webcappi-url", server.URL, "--webcappi-token-file", token}); err != nil {
			t.Fatal(err)
		}
	}
	for _, extra := range [][]string{{"--max-items", "0"}, {"--profile", "auto"}, {"--timezone", "Unknown/Zone"}, {"--feed-url", server.URL + "/feed"}} {
		args := []string{"--context", contextPath, "--output", output, "--input", "fixtures/rss2.xml", "--webcappi-url", server.URL}
		args = append(args, extra...)
		if err := invoke(args); err == nil {
			t.Fatalf("accepted invalid CLI options %v", extra)
		}
	}
}
