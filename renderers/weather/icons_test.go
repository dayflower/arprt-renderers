package main

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestLucideAssetsAndMappings(t *testing.T) {
	// Vendored assets become trusted template HTML, so keep them self-contained.
	allowed := map[string]bool{"svg": true, "path": true, "circle": true, "line": true, "polyline": true, "polygon": true, "rect": true, "ellipse": true}
	for name, artwork := range lucideArtwork {
		if strings.TrimSpace(string(artwork)) == "" {
			t.Fatal("empty Lucide drawing", name)
		}
		source, err := lucideFiles.ReadFile("icons/lucide/" + name + ".svg")
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(strings.NewReader(string(source)))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(name, err)
			}
			if element, ok := token.(xml.StartElement); ok {
				if !allowed[element.Name.Local] {
					t.Fatal("unexpected SVG element", name, element.Name)
				}
				for _, attribute := range element.Attr {
					key := strings.ToLower(attribute.Name.Local)
					if strings.HasPrefix(key, "on") || key == "href" || key == "style" || strings.Contains(strings.ToLower(attribute.Value), "url(") {
						t.Fatal("external or executable SVG content", name, key)
					}
				}
			}
		}
	}
	for _, code := range []int{0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 97, 99} {
		n := float64(code)
		v := weatherIcon(&n)
		if v.Artwork() == "" || v.Name == "Unknown" {
			t.Fatal("unmapped weather code", code)
		}
	}
	for code, want := range map[int]string{61: "cloud-hail", 63: "cloud-rain", 65: "cloud-rain-wind", 80: "cloud-hail", 82: "cloud-rain-wind", 96: "regex", 99: "regex", 71: "snowflake", 77: "snowflake", 85: "snowflake", 97: "cloud-lightning"} {
		n := float64(code)
		if got := weatherIcon(&n).Kind; got != want {
			t.Fatalf("code %d: %s, want %s", code, got, want)
		}
	}
	n := 999.0
	if weatherIcon(nil).Artwork() == "" || weatherIcon(&n).Kind != "circle-question-mark" {
		t.Fatal("missing unknown icon")
	}
}
