package main

import (
	"embed"
	"html/template"
	"strings"
)

// Only pinned, repository-owned Lucide assets enter the trusted SVG wrapper.
// No SVG from forecast responses or command-line input is accepted.
//
//go:embed icons/lucide/*.svg
var lucideFiles embed.FS

var lucideArtwork = func() map[string]template.HTML {
	entries, err := lucideFiles.ReadDir("icons/lucide")
	if err != nil {
		panic("cannot read embedded Lucide assets")
	}
	artwork := make(map[string]template.HTML, len(entries))
	for _, entry := range entries {
		data, err := lucideFiles.ReadFile("icons/lucide/" + entry.Name())
		if err != nil {
			panic("cannot read embedded Lucide SVG")
		}
		source := string(data)
		root := strings.Index(source, "<svg")
		if root < 0 {
			panic("invalid embedded Lucide SVG")
		}
		start := strings.Index(source[root:], ">") + root + 1
		end := strings.LastIndex(source, "</svg>")
		if start <= root || end < start {
			panic("invalid embedded Lucide SVG")
		}
		artwork[strings.TrimSuffix(entry.Name(), ".svg")] = template.HTML(source[start:end])
	}
	return artwork
}()

func (i icon) Artwork() template.HTML { return lucideArtwork[i.Kind] }
