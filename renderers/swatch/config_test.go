package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	c, err := parseConfig([]byte(`{"version":1}`))
	if err != nil || !c.Reference || c.Note != "" || len(c.Colors) != 24 {
		t.Fatal(c, err)
	}
	for i, color := range c.Colors {
		if color.Color != builtinColors[i] || color.Label != "" {
			t.Fatal("built-in order", i, color)
		}
	}
	seen := map[string]bool{}
	for _, color := range builtinColors {
		if _, ok := normalizeColor(color); !ok || seen[color] {
			t.Fatal("invalid built-in color", color)
		}
		seen[color] = true
	}
}

func TestConfigColors(t *testing.T) {
	c, err := parseConfig([]byte(`{"version":1,"note":"  dithering: none  ","reference":false,"colors":[{"color":"#AAaa00","label":" muted 湾 "},{"color":"#000000"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Reference || c.Note != "dithering: none" || !slices.Equal(c.Colors, []swatchColor{{"#aaaa00", "muted 湾"}, {"#000000", ""}}) {
		t.Fatal(c)
	}
	var entries []string
	for i := 0; i < maxColors; i++ {
		entries = append(entries, fmt.Sprintf(`{"color":"#0000%02x"}`, i))
	}
	if c, err := parseConfig([]byte(`{"version":1,"colors":[` + strings.Join(entries, ",") + `]}`)); err != nil || len(c.Colors) != maxColors {
		t.Fatal("maximum colors rejected", err)
	}
	tooMany := `{"version":1,"colors":[` + strings.Join(append(entries, `{"color":"#ffffff"}`), ",") + `]}`
	if _, err := parseConfig([]byte(tooMany)); err == nil {
		t.Fatal("accepted more than 24 colors")
	}
}

func TestConfigRejects(t *testing.T) {
	base := `{"version":1}`
	for _, input := range []string{
		`{}`, `[]`, `null`, `""`, base + `{}`, `{"version":1.0}`, `{"version":2}`, `{"version":"1"}`,
		`{"version":1,"version":1}`, `{"version":1,"extra":true}`, `{"version":null}`,
		`{"version":1,"note":null}`, `{"version":1,"note":1}`, `{"version":1,"note":"a\nb"}`, `{"version":1,"note":"\ta"}`,
		`{"version":1,"note":"` + strings.Repeat("湾", 129) + `"}`,
		`{"version":1,"reference":null}`, `{"version":1,"reference":"true"}`, `{"version":1,"reference":1}`,
		`{"version":1,"colors":null}`, `{"version":1,"colors":[]}`, `{"version":1,"colors":{}}`, `{"version":1,"colors":[null]}`,
		`{"version":1,"colors":["#ffffff"]}`, `{"version":1,"colors":[{}]}`, `{"version":1,"colors":[{"label":"x"}]}`,
		`{"version":1,"colors":[{"color":"#ffffff","extra":1}]}`, `{"version":1,"colors":[{"color":"#ffffff","color":"#000000"}]}`,
		`{"version":1,"colors":[{"color":"#ffffff","label":null}]}`, `{"version":1,"colors":[{"color":"#ffffff","label":"a\nb"}]}`,
		`{"version":1,"colors":[{"color":"#ffffff","label":"` + strings.Repeat("x", 33) + `"}]}`,
		`{"version":1,"colors":[{"color":"#ffffff"},{"color":"#FFFFFF"}]}`,
		`{"version":1,"note":"` + string([]byte{0xff}) + `"}`,
		strings.Repeat(" ", configLimit) + base,
	} {
		if _, err := parseConfig([]byte(input)); err == nil {
			t.Errorf("accepted configuration %q", input[:min(len(input), 180)])
		}
	}
	for _, color := range []string{"", "#fff", "#ffffffff", "ffffff", "#gggggg", " #ffffff", "#ffffff ", "red", "rgb(0,0,0)", "#ｆｆｆｆｆｆ"} {
		if _, err := parseConfig([]byte(fmt.Sprintf(`{"version":1,"colors":[{"color":%q}]}`, color))); err == nil {
			t.Errorf("accepted color %q", color)
		}
	}
	for _, input := range []string{
		`{"version":1,"note":"   "}`, `{"version":1,"colors":[{"color":"#ffffff","label":"   "}]}`,
		`{"version":1,"note":"` + strings.Repeat("湾", 128) + `"}`, `{"version":1,"colors":[{"color":"#ffffff","label":"` + strings.Repeat("湾", 32) + `"}]}`,
	} {
		if _, err := parseConfig([]byte(input)); err != nil {
			t.Errorf("rejected %q: %v", input[:min(len(input), 80)], err)
		}
	}
}
