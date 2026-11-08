package main

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// This opt-in test captures renderer-owned HTML with real Chromium. It verifies
// blank inset pixels and the physical rotation, rather than trusting stub PNGs.
func TestBrowserSafeArea(t *testing.T) {
	endpoint := os.Getenv("SAFE_AREA_WEBCAPPI")
	if endpoint == "" {
		t.Skip("set SAFE_AREA_WEBCAPPI for real-browser safe-area checks")
	}
	for _, view := range []string{"default"} {
		for _, palette := range []string{"gray4", "spectra6"} {
			for _, rotation := range []int{0, 90, 180, 270} {
				t.Run(fmt.Sprintf("%s-%s-%d", view, palette, rotation), func(t *testing.T) {
					target, err := parseContext(insetContextBytes(rotation, palette))
					if err != nil {
						t.Fatal(err)
					}
					f, err := parseFeed(fixture(t, "rss2.xml"))
					if err != nil {
						t.Fatal(err)
					}
					body, err := renderHTML(f, "Synthetic feed", 8, true, time.UTC)
					if err == nil {
						body, err = target.layoutHTML(body)
					}
					if err != nil {
						t.Fatal(err)
					}
					if dir := os.Getenv("SAFE_AREA_LAYOUT_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("rss-%s-%s-%d.html", view, palette, rotation)), body, 0644); err != nil {
							t.Fatal(err)
						}
					}
					w, h := target.logicalSize()
					address := endpoint + "/capture?" + url.Values{"width": {strconv.Itoa(w)}, "height": {strconv.Itoa(h)}, "html_mode": {"file"}, "ready_selector": {"#rss-ready"}}.Encode()
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Content-Type", "text/html; charset=utf-8")
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
					if err != nil || resp.StatusCode != 200 {
						t.Fatalf("capture: %s %s %v", resp.Status, data, err)
					}
					img, err := png.Decode(bytes.NewReader(data))
					if err != nil {
						t.Fatal(err)
					}
					if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
						t.Fatal("capture canvas resized")
					}
					i := target.Target.SafeAreaInsets
					ink := 0
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							r, g, b, a := img.At(x, y).RGBA()
							if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
								if x < i.Left || x >= w-i.Right || y < i.Top || y >= h-i.Bottom {
									t.Fatalf("ink outside safe area at %d,%d", x, y)
								}
								ink++
							}
						}
					}
					if ink == 0 {
						t.Fatal("empty capture")
					}
					output, err := outputPNG(img, rotation)
					if err != nil {
						t.Fatal(err)
					}
					physical, err := png.Decode(bytes.NewReader(output))
					if err != nil {
						t.Fatal(err)
					}
					if physical.Bounds().Dx() != 800 || physical.Bounds().Dy() != 480 {
						t.Fatal("physical output resized")
					}
					// The logical top-left safe-area corner must remain white at its rotated position.
					x, y := i.Left-1, i.Top-1
					switch rotation {
					case 90:
						x, y = h-1-y, x
					case 180:
						x, y = w-1-x, h-1-y
					case 270:
						x, y = y, w-1-x
					}
					r, g, b, _ := physical.At(x, y).RGBA()
					if r != 65535 || g != 65535 || b != 65535 {
						t.Fatal("insets rotated incorrectly")
					}
					if dir := os.Getenv("SAFE_AREA_LAYOUT_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						prefix := filepath.Join(dir, fmt.Sprintf("rss-%s-%s-%d", view, palette, rotation))
						if err := os.WriteFile(prefix+".png", output, 0644); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(prefix+".html", body, 0644); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}
