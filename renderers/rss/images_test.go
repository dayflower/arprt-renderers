package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"html"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestImageCandidates(t *testing.T) {
	for _, profile := range []string{"generic", "livedoor", "hatena", "google-news"} {
		f, err := parseFeedProfile(rssItems(`<item xmlns:h="http://www.hatena.ne.jp/info/xmlns#"><title>Headline</title><description><![CDATA[<img src="https://bad.example.com/tracker">]]></description><enclosure type="audio/mpeg" url="https://media.example.com/audio"/><enclosure type="image/png" url="file:///bad"/><enclosure type="image/jpeg" url="https://media.example.com/photo"/><h:imageurl>https://media.example.com/hatena</h:imageurl></item>`), profile, "")
		if err != nil {
			t.Fatal(err)
		}
		want := "https://media.example.com/photo"
		if profile == "hatena" {
			want = "https://media.example.com/hatena"
		}
		if profile == "google-news" {
			want = ""
		}
		if f.Articles[0].ImageURL != want {
			t.Fatalf("%s: %q", profile, f.Articles[0].ImageURL)
		}
	}
}
func TestPublicImageDestinations(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1", "192.0.2.1", "198.18.0.1", "224.0.0.1", "240.0.0.1", "::1", "::", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1", "2002:7f00:1::1", "2001::1"} {
		if publicImageIP(netip.MustParseAddr(ip)) {
			t.Errorf("accepted %s", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if !publicImageIP(netip.MustParseAddr(ip)) {
			t.Errorf("rejected %s", ip)
		}
	}
	var lookups, dials int
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		dials++
		if address != "8.8.8.8:443" {
			t.Fatalf("not pinned: %s", address)
		}
		return nil, errors.New("synthetic dial")
	}
	_, _ = imageDialer(lookup, dial)(context.Background(), "tcp", "images.example.com:443")
	if lookups != 1 || dials != 1 {
		t.Fatal("unexpected DNS/dial count")
	}
	lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	}
	_, err := imageDialer(lookup, dial)(context.Background(), "tcp", "mixed.example.com:443")
	if err == nil || dials != 1 {
		t.Fatal("mixed DNS answers dialed")
	}
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer s.Close()
	client := newImageClient(true)
	defer client.CloseIdleConnections()
	if _, err := fetchThumbnail(context.Background(), client, s.URL); err == nil || hits.Load() != 0 {
		t.Fatal("production client reached loopback")
	}
	// The redirect is reached using an injected initial public-to-stub dial. A
	// second dial must still inspect the private redirect destination.
	transport := client.Transport.(*http.Transport)
	transport.DialContext = imageDialer(net.DefaultResolver.LookupNetIP, func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(s.URL, "http://"))
	})
	s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); http.Redirect(w, r, s.URL+"/private", 302) })
	if _, err := fetchThumbnail(context.Background(), client, "http://8.8.8.8/start"); err == nil || hits.Load() != 1 {
		t.Fatal("private redirect was followed")
	}
}

// A synthetic solid red VP8L image. Its only pixel is used to check WebP decoding.
const redWebP = "UklGRhwAAABXRUJQVlA4TA8AAAAvAAAAAAcQ/Y/+ByKi/wEA"

func TestThumbnailFormatsAndBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 192, 144))
	for y := 0; y < 144; y++ {
		for x := 0; x < 192; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var jpg, pngData, gifData bytes.Buffer
	_ = jpeg.Encode(&jpg, src, nil)
	_ = png.Encode(&pngData, src)
	_ = gif.EncodeAll(&gifData, &gif.GIF{Image: []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 192, 144), color.Palette{color.RGBA{255, 0, 0, 255}}), image.NewPaletted(image.Rect(0, 0, 192, 144), color.Palette{color.RGBA{0, 0, 255, 255}})}, Delay: []int{0, 0}})
	webp, _ := base64.StdEncoding.DecodeString(redWebP)
	for name, data := range map[string][]byte{"jpeg": jpg.Bytes(), "png": pngData.Bytes(), "gif": gifData.Bytes(), "webp": webp} {
		t.Run(name, func(t *testing.T) {
			result, err := thumbnail(data)
			if err != nil {
				t.Fatal(err)
			}
			if !validThumbnail(result) {
				t.Fatal("invalid data URL")
			}
			decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(result, "data:image/jpeg;base64,"))
			img, err := jpeg.Decode(bytes.NewReader(decoded))
			if err != nil {
				t.Fatal(err)
			}
			if name != "webp" && img.Bounds() != image.Rect(0, 0, 96, 72) {
				t.Fatal(img.Bounds())
			}
			r, g, b, _ := img.At(0, 0).RGBA()
			if r < 50000 || g > 10000 || b > 10000 {
				t.Fatal("first frame/color was not retained")
			}
		})
	}
	for _, dims := range [][2]int{{1, 20000}, {20000, 1}, {1, 1}} {
		var b bytes.Buffer
		_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, dims[0], dims[1])))
		result, err := thumbnail(b.Bytes())
		if err != nil || !validThumbnail(result) {
			t.Fatalf("ratio %v: %v", dims, err)
		}
		data, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(result, "data:image/jpeg;base64,"))
		img, _ := jpeg.Decode(bytes.NewReader(data))
		r, g, bv, _ := img.At(0, 0).RGBA()
		if r < 65000 || g < 65000 || bv < 65000 {
			t.Fatal("transparency not white")
		}
	}
	huge := bytes.Clone(pngData.Bytes())
	binary.BigEndian.PutUint32(huge[16:20], 4000001)
	// A valid header CRC ensures rejection depends on dimensions, before allocation.
	fixPNGHeaderCRC(huge)
	for _, data := range [][]byte{[]byte("<html>not an image</html>"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), pngData.Bytes()[:40], huge} {
		if _, err := thumbnail(data); err == nil {
			t.Fatal("accepted invalid/oversized image")
		}
	}
	// Wrap the synthetic frame in an animation; a bogus second frame must never
	// be decoded, and the canvas must be bounded independently of the first frame.
	animated := animatedWebP(webp)
	if _, err := thumbnail(animated); err != nil {
		t.Fatal(err)
	}
	oversized := bytes.Clone(animated)
	oversized[24], oversized[25], oversized[26] = 255, 255, 255
	if _, err := thumbnail(oversized); err == nil {
		t.Fatal("accepted oversized animation canvas")
	}
}
func fixPNGHeaderCRC(data []byte) {
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
}
func animatedWebP(frame []byte) []byte {
	var body bytes.Buffer
	body.WriteString("WEBPVP8X")
	_ = binary.Write(&body, binary.LittleEndian, uint32(10))
	body.Write([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	body.WriteString("ANIM")
	_ = binary.Write(&body, binary.LittleEndian, uint32(6))
	body.Write(make([]byte, 6))
	body.WriteString("ANMF")
	_ = binary.Write(&body, binary.LittleEndian, uint32(16+len(frame)-12))
	body.Write(make([]byte, 16))
	body.Write(frame[12:])
	body.WriteString("ANMF")
	_ = binary.Write(&body, binary.LittleEndian, uint32(0))
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}
func TestImageFetchFailuresAndMixedRows(t *testing.T) {
	valid := pngBytes(t, 200, 150)
	var active, peak, hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// Timed-out error responses may still unwind on the server. Measure
		// concurrency only among the successful image requests under test.
		if r.URL.Path == "/ok" {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("authentication leaked to image")
		}
		switch r.URL.Path {
		case "/ok":
			time.Sleep(10 * time.Millisecond)
			w.Write(valid)
		case "/redirect":
			http.Redirect(w, r, "/ok", 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		case "/scheme":
			http.Redirect(w, r, "file:///etc/passwd", 302)
		case "/credentials":
			http.Redirect(w, r, "http://user:pass@example.com/", 302)
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			_, _ = io.WriteString(z, strings.Repeat("x", imageLimit+1))
			z.Close()
		case "/large":
			_, _ = io.WriteString(w, strings.Repeat("x", imageLimit+1))
		case "/fake":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "<html>fake</html>")
		case "/cut":
			w.Header().Set("Content-Length", "100")
			w.Write([]byte("short"))
		case "/timeout":
			<-r.Context().Done()
		default:
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	for _, path := range []string{"/ok", "/redirect"} {
		if _, err := fetchThumbnail(context.Background(), newHTTPClient(), s.URL+path); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/loop", "/scheme", "/credentials", "/gzip", "/large", "/fake", "/cut", "/timeout", "/failure"} {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, err := fetchThumbnail(ctx, newHTTPClient(), s.URL+path)
		cancel()
		if err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	for _, paths := range [][]string{{}, {"/ok", "/ok"}, {"/ok", "/failure"}, {"/fake", "/failure"}} {
		f := feed{Title: "Synthetic images"}
		for i, path := range paths {
			f.Articles = append(f.Articles, article{Title: fmt.Sprintf("Article %d", i), ImageURL: s.URL + path})
		}
		loadImages(context.Background(), newHTTPClient(), &f, 20)
		body, err := renderHTML(f, "", 20, true, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		for i, path := range paths {
			if !bytes.Contains(body, []byte(fmt.Sprintf("Article %d", i))) {
				t.Fatal("text disappeared")
			}
			if (f.Articles[i].ImageData != "") != (path == "/ok") {
				t.Fatal("wrong image fallback")
			}
		}
		if bytes.Contains(body, []byte(s.URL)) {
			t.Fatal("external URL in HTML")
		}
	}
	peak.Store(0)
	f := feed{Articles: make([]article, 20)}
	for i := range f.Articles {
		f.Articles[i].ImageURL = s.URL + "/ok"
	}
	before := hits.Load()
	loadImages(context.Background(), newHTTPClient(), &f, 8)
	if hits.Load()-before != 8 || peak.Load() > 2 {
		t.Fatalf("bounds: %d requests, %d concurrent", hits.Load()-before, peak.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.Articles[0].ImageURL = s.URL + "/timeout"
	f.Articles[0].ImageData = ""
	loadImages(ctx, newHTTPClient(), &f, 1)
	if f.Articles[0].ImageData != "" {
		t.Fatal("cancelled image retained")
	}
}
func TestImageHTMLBudget(t *testing.T) {
	data, err := thumbnail(pngBytes(t, 192, 144))
	if err != nil {
		t.Fatal(err)
	}
	f := feed{Title: "Budget"}
	// Fill the text budget to leave room for some, but not all, thumbnails.
	for i := 0; i < 20; i++ {
		f.Articles = append(f.Articles, article{Title: fmt.Sprintf("Headline %d", i), ImageData: data, Summary: []summaryBlock{{Text: strings.Repeat("&", 10380)}}})
	}
	body, err := renderHTML(f, "", 20, true, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > htmlLimit {
		t.Fatal("HTML exceeded budget")
	}
	for i := 0; i < 20; i++ {
		if !bytes.Contains(body, []byte(fmt.Sprintf("Headline %d", i))) {
			t.Fatal("text lost")
		}
	}
	count := bytes.Count(body, []byte(`<img `))
	if count == 0 || count >= 20 {
		t.Fatalf("budget did not exercise partial images: %d, %d bytes", count, len(body))
	}
	f.Articles[0].ImageData = "https://images.example.com/unsafe"
	body, err = renderHTML(f, "", 20, false, time.UTC)
	if err != nil || bytes.Contains(body, []byte("https://images.example.com")) {
		t.Fatal("unsafe template URL")
	}
}
func TestImageBudgetReduction(t *testing.T) {
	full, err := thumbnail(pngBytes(t, 192, 144))
	if err != nil {
		t.Fatal(err)
	}
	reduced := reduceThumbnail(full)
	if reduced == "" || len(reduced) >= len(full) {
		t.Fatal("thumbnail reduction failed")
	}
	f := feed{Title: "Reduction", Articles: []article{{Title: "Text retained", Summary: []summaryBlock{{Text: "x"}}}}}
	base, err := renderHTML(f, "", 1, true, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	reserve := (len(full)+len(reduced))/2 + 90
	f.Articles[0].Summary[0].Text += strings.Repeat("&", (htmlLimit-len(base)-reserve)/5)
	f.Articles[0].ImageData = full
	body, err := renderHTML(f, "", 1, true, time.UTC)
	if err != nil || len(body) > htmlLimit {
		t.Fatal("HTML budget failed", err)
	}
	decoded := html.UnescapeString(string(body))
	if !strings.Contains(decoded, reduced) || strings.Contains(decoded, full) {
		t.Fatalf("did not use reduced thumbnail: full=%d reduced=%d reserve=%d body=%d images=%d fullPresent=%t", len(full), len(reduced), reserve, len(body), bytes.Count(body, []byte(`<img `)), bytes.Contains(body, []byte(full)))
	}
}

func TestImagesCLI(t *testing.T) { checkImagesCLI(t, "") }
func checkImagesCLI(t *testing.T, binary string) {
	t.Helper()
	root := t.TempDir()
	input := filepath.Join(root, "feed.xml")
	ctxPath := filepath.Join(root, "context.json")
	out := filepath.Join(root, "output.png")
	var imageHits atomic.Int32
	var wantImage atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/image", http.StatusFound)
			return
		}
		if r.URL.Path == "/image" {
			imageHits.Add(1)
			w.Write(pngBytes(t, 96, 72))
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte("Headline survives")) || bytes.Contains(body, []byte(`<img `)) != wantImage.Load() {
			t.Error("image policy did not match capture HTML")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes(t, 800, 480))
	}))
	defer s.Close()
	_ = os.WriteFile(input, rssItems(`<item><title>Headline survives</title><enclosure type="image/png" url="`+s.URL+`/image"/></item>`), 0600)
	_ = os.WriteFile(ctxPath, contextBytes(0, "gray4"), 0600)
	for _, test := range []struct {
		flags []string
		want  bool
		path  string
	}{
		{nil, false, "/image"},
		{[]string{"--public-images-only=true"}, false, "/image"},
		{[]string{"--public-images-only=false"}, true, "/image"},
		{[]string{"--public-images-only=false"}, true, "/redirect"},
		{[]string{"--public-images-only=false", "--images=false"}, false, "/image"},
	} {
		wantImage.Store(test.want)
		before := imageHits.Load()
		if err := os.WriteFile(input, rssItems(`<item><title>Headline survives</title><enclosure type="image/png" url="`+s.URL+test.path+`"/></item>`), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"--context", ctxPath, "--input", input, "--output", out, "--webcappi-url", s.URL}
		args = append(args, test.flags...)
		var err error
		if binary == "" {
			err = run(context.Background(), args, io.Discard, io.Discard)
		} else {
			cmd := exec.Command(binary, args...)
			data, e := cmd.CombinedOutput()
			if e != nil {
				t.Fatalf("CLI: %v %s", e, data)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, data, 0)
		wantHits := int32(0)
		if test.want {
			wantHits = 1
		}
		if imageHits.Load()-before != wantHits {
			t.Fatalf("image requests for %v: got %d want %d", test.flags, imageHits.Load()-before, wantHits)
		}
	}
}
