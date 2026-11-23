package main

import (
	"math"
	"strconv"
)

const slotCount = 30

// Palette entries exactly as ARPRT quantizes to them. Spectra 6 uses the EPF
// reference colors from ARPRT's image processing, not pure RGB primaries.
var referenceColors = map[string][]string{
	"spectra6": {"#000000", "#ffffff", "#bf0000", "#438a1c", "#6440ff", "#fff338"},
	"gray4":    {"#000000", "#555555", "#aaaaaa", "#ffffff"},
}

type tile struct {
	Color   string
	Label   string
	Text    string // overlay text color, black or white
	Outline bool
}

type slot struct {
	Tile *tile
}

type view struct {
	Palette  string
	Rotation int
	Note     string
	Columns  int
	Rows     int
	Slots    []slot
}

// relativeLuminance implements the WCAG 2 definition for an sRGB #rrggbb color.
func relativeLuminance(hex string) float64 {
	channel := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(1) + 0.7152*channel(3) + 0.0722*channel(5)
}

func contrastRatio(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

// overlayText chooses the higher-contrast of black and white. Ties, within
// floating-point tolerance, choose black.
func overlayText(luminance float64) string {
	if contrastRatio(1, luminance) > contrastRatio(0, luminance)+1e-9 {
		return "#ffffff"
	}
	return "#000000"
}

func newTile(c swatchColor) *tile {
	l := relativeLuminance(c.Color)
	return &tile{Color: c.Color, Label: c.Label, Text: overlayText(l), Outline: contrastRatio(1, l) < 1.25}
}

func gridSize(rotation int) (int, int) {
	if rotation == 90 || rotation == 270 {
		return 3, 10
	}
	return 6, 5
}

// buildView places reference tiles first and starts candidates on the next
// grid row, keeping a fixed 30-slot grid so tile sizes never depend on counts.
func buildView(c config, target renderContext) view {
	columns, rows := gridSize(target.rotation)
	v := view{Palette: target.Target.Palette, Rotation: target.rotation, Note: c.Note, Columns: columns, Rows: rows, Slots: make([]slot, slotCount)}
	next := 0
	if c.Reference {
		for _, color := range referenceColors[v.Palette] {
			v.Slots[next].Tile = newTile(swatchColor{Color: color})
			next++
		}
		next = (next + columns - 1) / columns * columns
	}
	for _, color := range c.Colors {
		v.Slots[next].Tile = newTile(color)
		next++
	}
	return v
}
