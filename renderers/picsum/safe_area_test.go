package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSafeAreaKeepsFullBleedPhoto(t *testing.T) {
	source := photo(t, rgbCorners)
	for _, rotation := range []int{0, 90, 180, 270} {
		raw := contextJSON(rotation, "spectra6")
		plain, err := parseContext(raw, "rgb")
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `"renderTarget":{`, `"renderTarget":{"safeAreaInsets":{"top":4,"bottom":8,"left":12,"right":16},`, 1))
		inset, err := parseContext(raw, "rgb")
		if err != nil {
			t.Fatal(err)
		}
		r := newRenderer(io.Discard)
		want, err := r.render(plain, source, "rgb")
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.render(inset, source, "rgb")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("safe area changed photograph at rotation %d", rotation)
		}
		assertPNG(t, got, "rgb")
	}
}
