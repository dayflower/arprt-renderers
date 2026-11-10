package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func safeContext(rotation int, insets string) []byte {
	extra := ""
	if insets != "" {
		extra = `,"safeAreaInsets":` + insets
	}
	return []byte(fmt.Sprintf(`{"version":"v1","device":{"name":"synthetic"},"renderTarget":{"width":800,"height":480,"palette":"gray4","rotation":%d%s}}`, rotation, extra))
}

func TestSafeAreaContextAndLayout(t *testing.T) {
	page := []byte("<html><head><style>body{padding:8px}</style></head><body><main>content</main></body></html>")
	for _, rotation := range []int{0, 90, 180, 270} {
		for _, insets := range []string{"", `{}`, `{"top":0,"bottom":0,"left":0,"right":0}`, `{"left":12}`, `{"top":4,"bottom":8,"left":12,"right":16,"future":true}`} {
			t.Run(fmt.Sprintf("%d/%s", rotation, insets), func(t *testing.T) {
				c, err := parseContext(safeContext(rotation, insets))
				if err != nil {
					t.Fatal(err)
				}
				w, h := c.logicalSize()
				cw, ch := c.contentSize()
				i := c.Target.SafeAreaInsets
				if cw != w-i.Left-i.Right || ch != h-i.Top-i.Bottom {
					t.Fatal("incorrect content size")
				}
				body, err := c.layoutHTML(page)
				if err != nil {
					t.Fatal(err)
				}
				if i == (safeAreaInsets{}) {
					if !bytes.Equal(body, page) {
						t.Fatal("zero insets changed layout")
					}
				} else {
					want := fmt.Sprintf("left:%dpx;top:%dpx;width:%dpx!important;height:%dpx!important", i.Left, i.Top, cw, ch)
					if !strings.Contains(string(body), want) || !bytes.Contains(body, []byte("padding:8px")) {
						t.Fatal("missing inset placement or design padding")
					}
				}
				// Safe area never changes capture/output dimensions.
				ww, hh := c.logicalSize()
				if ww != w || hh != h {
					t.Fatal("safe area resized canvas")
				}
			})
		}
		w, h := 800, 480
		if rotation%180 != 0 {
			w, h = h, w
		}
		for _, invalid := range []string{`null`, `[]`, `"bad"`, `{"top":null}`, `{"left":-1}`, `{"right":4096}`, `{"top":1.5}`, `{"left":"12"}`, `{"top":true}`, `{"left":9223372036854775808}`, fmt.Sprintf(`{"left":%d}`, w), fmt.Sprintf(`{"top":%d}`, h), fmt.Sprintf(`{"left":%d,"right":1}`, w-1), fmt.Sprintf(`{"top":%d,"bottom":1}`, h-1)} {
			if _, err := parseContext(safeContext(rotation, invalid)); err == nil {
				t.Errorf("rotation %d accepted %s", rotation, invalid)
			}
		}
		if _, err := parseContext(safeContext(rotation, fmt.Sprintf(`{"left":%d,"top":%d}`, w-1, h-1))); err != nil {
			t.Fatal("valid one-pixel safe area rejected", err)
		}
	}
}

func insetContextBytes(rotation int, palette string) []byte {
	return []byte(strings.Replace(string(safeContext(rotation, `{"top":4,"bottom":8,"left":12,"right":16}`)), `"gray4"`, fmt.Sprintf("%q", palette), 1))
}
