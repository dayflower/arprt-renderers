package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func mustContext(t *testing.T, rotation int, palette string) renderContext {
	t.Helper()
	c, err := parseContext(contextBytes(rotation, palette))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func slotColors(v view) []string {
	var out []string
	for _, s := range v.Slots {
		if s.Tile == nil {
			out = append(out, "")
		} else {
			out = append(out, s.Tile.Color)
		}
	}
	return out
}

func TestContrast(t *testing.T) {
	for _, tc := range []struct {
		color, text string
		outline     bool
	}{
		{"#000000", "#ffffff", false}, {"#ffffff", "#000000", true}, {"#ffff00", "#000000", true},
		{"#0000ff", "#ffffff", false}, {"#ff0000", "#000000", false}, {"#808080", "#000000", false},
		{"#555555", "#ffffff", false}, {"#cccccc", "#000000", false}, {"#eeeeee", "#000000", true},
		{"#800080", "#ffffff", false}, {"#00ff00", "#000000", false},
	} {
		got := newTile(swatchColor{Color: tc.color})
		if got.Text != tc.text || got.Outline != tc.outline {
			t.Errorf("%s: %+v", tc.color, got)
		}
	}
	if l := relativeLuminance("#ffffff"); math.Abs(l-1) > 1e-9 || relativeLuminance("#000000") != 0 {
		t.Fatal("luminance endpoints")
	}
	// Both black and white reach the same ratio at L = sqrt(1.05*0.05) - 0.05.
	tie := math.Sqrt(1.05*0.05) - 0.05
	if overlayText(tie) != "#000000" || overlayText(tie+1e-6) != "#000000" || overlayText(tie-1e-6) != "#ffffff" {
		t.Fatal("tie must choose black")
	}
}

func TestGridPlacement(t *testing.T) {
	defaults, _ := parseConfig([]byte(`{"version":1}`))
	for _, tc := range []struct {
		palette  string
		rotation int
		first    int // first candidate slot
	}{
		{"spectra6", 0, 6}, {"spectra6", 90, 6}, {"gray4", 0, 6}, {"gray4", 270, 6},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.palette, tc.rotation), func(t *testing.T) {
			v := buildView(defaults, mustContext(t, tc.rotation, tc.palette))
			columns, rows := gridSize(tc.rotation)
			if v.Columns != columns || v.Rows != rows || columns*rows != slotCount || len(v.Slots) != slotCount {
				t.Fatal("grid", v.Columns, v.Rows)
			}
			colors := slotColors(v)
			reference := referenceColors[tc.palette]
			if strings.Join(colors[:len(reference)], ",") != strings.Join(reference, ",") {
				t.Fatal("reference order", colors)
			}
			for i := len(reference); i < tc.first; i++ {
				if colors[i] != "" {
					t.Fatal("reference row not padded", colors)
				}
			}
			if strings.Join(colors[tc.first:tc.first+24], ",") != strings.Join(builtinColors, ",") {
				t.Fatal("candidate order", colors)
			}
		})
	}
	c, _ := parseConfig([]byte(`{"version":1,"reference":false,"colors":[{"color":"#123456"}]}`))
	v := buildView(c, mustContext(t, 0, "spectra6"))
	colors := slotColors(v)
	if colors[0] != "#123456" || strings.Join(colors[1:], "") != "" {
		t.Fatal("reference disabled", colors)
	}
}

func TestLayoutHTML(t *testing.T) {
	c, _ := parseConfig([]byte(`{"version":1,"note":"<script>湾 & \"x\"</script>","colors":[{"color":"#AAAA00","label":"<b>muted</b>"},{"color":"#123456"}]}`))
	body, err := renderHTML(buildView(c, mustContext(t, 90, "spectra6")))
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if strings.Contains(html, "<script>湾") || strings.Contains(html, "<b>muted") || !strings.Contains(html, "&lt;script&gt;湾 &amp;") || !strings.Contains(html, "&lt;b&gt;muted&lt;/b&gt;") {
		t.Fatal("text escaping")
	}
	// Tile fills are the exact configured, normalized values.
	for _, want := range []string{
		`style="background:#aaaa00;color:#000000"`, `style="background:#123456;color:#ffffff"`,
		`style="background:#ffffff;color:#000000"`, `<span class="hex">#aaaa00</span>`,
		`repeat(3,minmax(0,1fr))`, `repeat(10,minmax(0,1fr))`, `<span class="meta">spectra6</span>`, `rotation 90`,
		`class="tile outline" data-color="#ffffff"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if n := strings.Count(html, `class="tile`) + strings.Count(html, `class="slot"`); n != slotCount {
		t.Fatal("slot count", n)
	}
	if strings.Contains(html, "synthetic-display") {
		t.Fatal("device name displayed")
	}
	c, _ = parseConfig([]byte(`{"version":1}`))
	body, _ = renderHTML(buildView(c, mustContext(t, 0, "gray4")))
	if strings.Contains(string(body), `class="note"`) || strings.Contains(string(body), `class="label"`) || !strings.Contains(string(body), `repeat(6,minmax(0,1fr))`) {
		t.Fatal("optional note/label or landscape grid")
	}
}
