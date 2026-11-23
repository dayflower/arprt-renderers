package main

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func rgbOf(hex string) color.RGBA {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// A real capture endpoint is opt-in; ordinary checks need no browser or network.
// Exporting HTML alone is useful, but does not establish layout acceptance.
func TestBrowserLayouts(t *testing.T) {
	endpoint, directory := os.Getenv("SWATCH_WEBCAPPI"), os.Getenv("SWATCH_LAYOUT_DIR")
	if endpoint == "" && directory == "" {
		t.Skip("set SWATCH_WEBCAPPI and optionally SWATCH_LAYOUT_DIR")
	}
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	var labelled []string
	for i, c := range builtinColors {
		labelled = append(labelled, fmt.Sprintf(`{"color":%q,"label":"候補 %d <Long> label text & more"}`, c, i+1))
	}
	cases := []struct{ name, config string }{
		{"defaults", `{"version":1}`},
		{"labels", `{"version":1,"note":"` + strings.Repeat("dithering: none 湾 ", 7) + `","colors":[` + strings.Join(labelled, ",") + `]}`},
		{"single", `{"version":1,"reference":false,"colors":[{"color":"#aaaa00","label":"muted"}]}`},
	}
	for _, tc := range cases {
		for _, palette := range []string{"gray4", "spectra6"} {
			for _, rotation := range []int{0, 90, 180, 270} {
				name := fmt.Sprintf("%s-%s-%d", tc.name, palette, rotation)
				t.Run(name, func(t *testing.T) {
					input := insetContextBytes(rotation, palette)
					if tc.name == "defaults" {
						input = contextBytes(rotation, palette)
					}
					target, err := parseContext(input)
					if err != nil {
						t.Fatal(err)
					}
					cfg, err := parseConfig([]byte(tc.config))
					if err != nil {
						t.Fatal(err)
					}
					v := buildView(cfg, target)
					body, err := renderHTML(v)
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
					counts := map[color.RGBA]int{}
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
							counts[got]++
							if got != (color.RGBA{255, 255, 255, 255}) && (x < i.Left || x >= w-i.Right || y < i.Top || y >= h-i.Bottom) {
								t.Fatalf("ink outside safe area at %d,%d", x, y)
							}
						}
					}
					// Tile interiors must keep their exact configured RGB values.
					for _, s := range v.Slots {
						if s.Tile != nil && counts[rgbOf(s.Tile.Color)] < 2000 {
							t.Fatalf("tile %s has %d exact pixels", s.Tile.Color, counts[rgbOf(s.Tile.Color)])
						}
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
			target, _ := parseContext(safeContext(0, `{"left":400,"top":200}`))
			cfg, _ := parseConfig([]byte(`{"version":1}`))
			body, _ := renderHTML(buildView(cfg, target))
			body, _ = target.layoutHTML(body)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := capture(ctx, client, endpoint, "", body, 800, 480); err == nil {
				t.Fatal("impossible layout declared readiness")
			}
		})
	}
}
