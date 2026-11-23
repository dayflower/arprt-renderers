package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const configLimit = 64 << 10
const maxColors = 24

type swatchColor struct {
	Color string
	Label string
}

type config struct {
	Version   int
	Note      string
	Reference bool
	Colors    []swatchColor
}

// Candidate colors evaluated by default, in display order.
var builtinColors = []string{
	"#ffff00", "#cccc00", "#aaaa00",
	"#cc9900", "#808000", "#996633",
	"#ff8000", "#ffaa00",
	"#00ff00", "#00c000", "#008000", "#66aa00", "#80c080",
	"#800080", "#8000ff", "#aa66cc", "#cc99ff",
	"#ff00ff", "#ff8080",
	"#008080", "#00c0c0",
	"#555555", "#808080", "#aaaaaa",
}

var errInvalidConfig = errors.New("invalid swatch configuration")

// decodeObject walks one JSON object explicitly so duplicate keys and nulls
// cannot hide behind encoding/json's last-key-wins and zero-value behavior.
// The field callback rejects unknown names.
func decodeObject(data []byte, field func(name string, raw json.RawMessage) error) error {
	d := json.NewDecoder(bytes.NewReader(data))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return errInvalidConfig
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return errInvalidConfig
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return errInvalidConfig
		}
		seen[name] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errInvalidConfig
		}
		if err := field(name, raw); err != nil {
			return err
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		return errInvalidConfig
	}
	if _, err := d.Token(); err != io.EOF {
		return errInvalidConfig
	}
	return nil
}

// cleanText rejects control characters, trims and bounds display text.
func cleanText(s string, limit int, name string) (string, error) {
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", errors.New(name + " must not contain control characters")
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > limit {
		return "", errors.New(name + " is too long")
	}
	return s, nil
}

func normalizeColor(s string) (string, bool) {
	if len(s) != 7 || s[0] != '#' {
		return "", false
	}
	for _, c := range s[1:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(s), true
}

func parseColor(data []byte) (swatchColor, error) {
	var c swatchColor
	hasColor := false
	err := decodeObject(data, func(name string, raw json.RawMessage) error {
		var err error
		switch name {
		case "color":
			hasColor = true
			err = json.Unmarshal(raw, &c.Color)
		case "label":
			err = json.Unmarshal(raw, &c.Label)
		default:
			return errInvalidConfig
		}
		if err != nil {
			return errInvalidConfig
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if !hasColor {
		return c, errors.New("each color entry requires color")
	}
	color, ok := normalizeColor(c.Color)
	if !ok {
		return c, errors.New("color must be #rrggbb")
	}
	c.Color = color
	c.Label, err = cleanText(c.Label, 32, "label")
	return c, err
}

func parseConfig(data []byte) (config, error) {
	c := config{Reference: true}
	if len(data) > configLimit || !utf8.Valid(data) {
		return c, errInvalidConfig
	}
	hasVersion, hasColors := false, false
	var colors []json.RawMessage
	err := decodeObject(data, func(name string, raw json.RawMessage) error {
		var err error
		switch name {
		case "version":
			hasVersion = true
			err = json.Unmarshal(raw, &c.Version)
		case "note":
			err = json.Unmarshal(raw, &c.Note)
		case "reference":
			err = json.Unmarshal(raw, &c.Reference)
		case "colors":
			hasColors = true
			err = json.Unmarshal(raw, &colors)
		default:
			return errInvalidConfig
		}
		if err != nil {
			return errInvalidConfig
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if !hasVersion || c.Version != 1 {
		return c, errInvalidConfig
	}
	if c.Note, err = cleanText(c.Note, 128, "note"); err != nil {
		return c, err
	}
	if !hasColors {
		for _, color := range builtinColors {
			c.Colors = append(c.Colors, swatchColor{Color: color})
		}
		return c, nil
	}
	if len(colors) < 1 || len(colors) > maxColors {
		return c, errors.New("colors must have 1 through 24 entries")
	}
	seen := map[string]bool{}
	for _, raw := range colors {
		entry, err := parseColor(raw)
		if err != nil {
			return c, err
		}
		if seen[entry.Color] {
			return c, errors.New("colors must not contain duplicates")
		}
		seen[entry.Color] = true
		c.Colors = append(c.Colors, entry)
	}
	return c, nil
}
