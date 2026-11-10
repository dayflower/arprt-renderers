package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real capture endpoint is opt-in; ordinary checks need no browser or network.
// Exporting HTML alone is useful, but does not establish layout acceptance.
func TestBrowserLayouts(t *testing.T) {
	endpoint, directory := os.Getenv("COUNTDOWN_WEBCAPPI"), os.Getenv("COUNTDOWN_LAYOUT_DIR")
	if endpoint == "" && directory == "" {
		t.Skip("set COUNTDOWN_WEBCAPPI and optionally COUNTDOWN_LAYOUT_DIR")
	}
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	for _, tc := range []struct{ name, title, count, status string }{
		{"reference", "", "52", "DAYS TO GO"},
		{"title", "New Year", "52", "DAYS TO GO"},
		{"tomorrow", "", "1", "DAY TO GO"},
		{"today", "", "0", "TODAY IS THE DAY"},
		{"past", "", "0", "TARGET PASSED"},
		{"long-title", strings.Repeat("予定 & <Event> 湾 ", 8), "3652058", "DAYS TO GO"},
	} {
		for _, palette := range []string{"gray4", "spectra6"} {
			for _, rotation := range []int{0, 90, 180, 270} {
				name := fmt.Sprintf("%s-%s-%d", tc.name, palette, rotation)
				t.Run(name, func(t *testing.T) {
					input := insetContextBytes(rotation, palette)
					if tc.name == "reference" {
						input = contextBytes(rotation, palette)
					}
					target, err := parseContext(input)
					if err != nil {
						t.Fatal(err)
					}
					targetDate, today := "2027-01-01", "2026-11-10"
					switch tc.name {
					case "tomorrow":
						today = "2026-12-31"
					case "today":
						today = targetDate
					case "past":
						today = "2027-01-02"
					case "long-title":
						targetDate, today = "9999-12-31", "0001-01-01"
					}
					body, err := renderHTML(countdownView{Title: tc.title, Count: tc.count, Status: tc.status, Target: targetDate, Today: today})
					if err == nil {
						body, err = target.layoutHTML(body)
					}
					if err != nil {
						t.Fatal(err)
					}
					if directory != "" {
						if err := os.MkdirAll(directory, 0o755); err != nil {
							t.Fatal(err)
						}
						put(t, filepath.Join(directory, name+".html"), body)
					}
					if endpoint == "" {
						t.Skip("HTML exported; real capture not requested")
					}
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					w, h := target.logicalSize()
					data, err := capture(ctx, client, endpoint, "", body, w, h)
					if err != nil {
						t.Fatal(err)
					}
					img, err := decodePNG(data, w, h)
					if err != nil {
						t.Fatal(err)
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
					out, err := outputPNG(img, rotation)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := decodePNG(out, 800, 480); err != nil {
						t.Fatal(err)
					}
					if directory != "" {
						put(t, filepath.Join(directory, name+".png"), out)
					}
				})
			}
		}
	}
	if endpoint != "" {
		t.Run("insufficient-safe-area", func(t *testing.T) {
			target, _ := parseContext(safeContext(0, `{"left":700,"top":400}`))
			body, _ := renderHTML(countdownView{Count: "3652058", Status: "DAYS TO GO", Target: "9999-12-31", Today: "0001-01-01"})
			body, _ = target.layoutHTML(body)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := capture(ctx, client, endpoint, "", body, 800, 480); err == nil {
				t.Fatal("impossible layout declared readiness")
			}
		})
	}
}
