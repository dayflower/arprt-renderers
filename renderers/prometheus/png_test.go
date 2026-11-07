package prometheus

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(w-1, 0, color.NRGBA{0, 255, 0, 255})
	img.SetNRGBA(0, h-1, color.NRGBA{0, 0, 255, 255})
	img.SetNRGBA(w-1, h-1, color.NRGBA{0, 0, 0, 0})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func contextBytes(rotation int, palette string) []byte {
	return []byte(fmt.Sprintf(`{"version":"v1","renderTarget":{"width":800,"height":480,"palette":%q,"rotation":%d},"device":{"name":"synthetic-display"},"unknown":true}`, palette, rotation))
}
func TestContextValidation(t *testing.T) {
	for _, p := range []string{"gray4", "spectra6"} {
		for _, r := range []int{0, 90, 180, 270} {
			c, err := ParseContext(contextBytes(r, p))
			if err != nil || c.Rotation != r {
				t.Fatal(c, err)
			}
		}
	}
	base := string(contextBytes(0, "gray4"))
	for _, bad := range []string{
		strings.Replace(base, `"v1"`, `"v2"`, 1), strings.Replace(base, `800`, `480`, 1), strings.Replace(base, `"gray4"`, `"gray16"`, 1),
		strings.Replace(base, `"rotation":0`, `"rotation":null`, 1), strings.Replace(base, `"rotation":0`, `"rotation":"90"`, 1),
		strings.Replace(base, `"rotation":0`, `"rotation":45`, 1), strings.Replace(base, `"rotation":0`, `"rotation":90.0`, 1), strings.Replace(base, `"synthetic-display"`, `""`, 1), `{}`,
	} {
		if _, err := ParseContext([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	c, err := ParseContext(mustContextFixture(t))
	if err != nil || c.Rotation != 0 {
		t.Fatal("omitted rotation rejected")
	}
}
func assertRotated(t *testing.T, data []byte, r int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 800, 480) {
		t.Fatal(img.Bounds())
	}
	// Source top-left is red, top-right green, bottom-left blue and bottom-right
	// transparent (white after compositing). Check positions, not only dimensions.
	corners := [][2]int{{0, 0}, {799, 0}, {0, 479}, {799, 479}}
	expected := map[int][]color.RGBA{
		0:   {{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}},
		90:  {{0, 0, 255, 255}, {255, 0, 0, 255}, {255, 255, 255, 255}, {0, 255, 0, 255}},
		180: {{255, 255, 255, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {255, 0, 0, 255}},
		270: {{0, 255, 0, 255}, {255, 255, 255, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}},
	}
	for i, p := range corners {
		got := color.RGBAModel.Convert(img.At(p[0], p[1])).(color.RGBA)
		if got != expected[r][i] {
			t.Fatalf("rotation %d corner %d: %v want %v", r, i, got, expected[r][i])
		}
	}
	if data[25] != 2 || data[24] != 8 {
		t.Fatal("output is not 8-bit RGB PNG")
	}
}
func TestPNGValidationAndRotation(t *testing.T) {
	for _, r := range []int{0, 90, 180, 270} {
		c, _ := ParseContext(contextBytes(r, "gray4"))
		w, h := c.LogicalSize()
		img, err := decodePNG(pngBytes(t, w, h), w, h)
		if err != nil {
			t.Fatal(err)
		}
		out, err := outputPNG(img, r)
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, out, r)
	}
	valid := pngBytes(t, 800, 480)
	badCRC := bytes.Clone(valid)
	badCRC[29] ^= 1
	for name, b := range map[string][]byte{"json": []byte(`{"error":"failed"}`), "truncated": valid[:len(valid)-2], "trailing": append(bytes.Clone(valid), 'x'), "crc": badCRC, "wrong-size": pngBytes(t, 480, 800)} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodePNG(b, 800, 480); err == nil {
				t.Fatal("accepted bad PNG")
			}
		})
	}
	// Animated input must not silently become a valid capture response.
	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(8))
	chunk.WriteString("acTL")
	chunk.Write(make([]byte, 8))
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()[4:]))
	animated := append(bytes.Clone(valid[:33]), chunk.Bytes()...)
	animated = append(animated, valid[33:]...)
	if _, err := decodePNG(animated, 800, 480); err == nil {
		t.Fatal("accepted animation")
	}
}
func TestAtomicWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "out.png")
	prior := []byte("prior output")
	if err := os.WriteFile(path, prior, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writeOutput(ctx, path, []byte("new")); err == nil {
		t.Fatal("cancelled write succeeded")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, prior) {
		t.Fatal("prior output changed")
	}
	entries, _ := filepath.Glob(filepath.Join(root, ".prometheus-*.tmp"))
	if len(entries) != 0 {
		t.Fatal("temporary files leaked")
	}
	if err := writeOutput(context.Background(), path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() != 0 {
		if err := os.Chmod(root, 0500); err != nil {
			t.Fatal(err)
		}
		err := writeOutput(context.Background(), path, []byte("failed"))
		os.Chmod(root, 0700)
		if err == nil {
			t.Fatal("unwritable parent accepted")
		}
		b, _ := os.ReadFile(path)
		if string(b) != "new" {
			t.Fatal("write failure replaced output")
		}
	}
	// Rename failure must also clean its sibling temporary file.
	if err := writeOutput(context.Background(), root, []byte("new")); err == nil {
		t.Fatal("rename to directory succeeded")
	}
	entries, _ = filepath.Glob(filepath.Join(root, ".prometheus-*.tmp"))
	if len(entries) != 0 {
		t.Fatal("temporary files leaked after rename failure")
	}
}

func mustContextFixture(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("fixtures/render-context-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
