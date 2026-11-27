package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBrowserPagination(t *testing.T) {
	endpoint := os.Getenv("RSS_PAGINATION_WEBCAPPI")
	if endpoint == "" {
		t.Skip("set RSS_PAGINATION_WEBCAPPI to a webcappi 0.2.0 endpoint")
	}
	const letters = "ABCDEFGHIJKLMNOPQRST"
	source := []byte(paginationFeed(letters))
	f, err := parseFeed(source)
	if err != nil {
		t.Fatal(err)
	}
	keys := articleKeys(f.Articles)
	for _, rotation := range []int{0, 90} {
		t.Run(strconv.Itoa(rotation), func(t *testing.T) {
			root := t.TempDir()
			ctxPath := filepath.Join(root, "context.json")
			feedPath := filepath.Join(root, "feed.xml")
			output := filepath.Join(root, "page.png")
			result := filepath.Join(root, "result.json")
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			read := func(path string) []byte {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			invoke := func(extra ...string) {
				t.Helper()
				args := append([]string{"--context", ctxPath, "--input", feedPath, "--output", output, "--webcappi-url", endpoint, "--images=false"}, extra...)
				if err := run(context.Background(), args, io.Discard, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			ctx := insetContextBytes(rotation, "gray4")
			var state json.RawMessage
			start := 0
			for range 2 {
				write(ctxPath, withRendererState(t, ctx, state))
				write(feedPath, source)
				invoke("--paginate")
				page := read(output)
				var envelope struct {
					NextState json.RawMessage `json:"nextState"`
				}
				if err := json.Unmarshal(read(result), &envelope); err != nil {
					t.Fatal(err)
				}
				state = envelope.NextState
				next := pageStart(keys, state)
				if next <= start || next-start >= 8 {
					t.Fatalf("expected the browser to fit 1–7 of 8 candidates, got %d", next-start)
				}
				t.Logf("browser selected articles %s", letters[start:next])
				write(feedPath, []byte(paginationFeed(letters[start:next])))
				invoke()
				if !bytes.Equal(page, read(output)) {
					t.Fatal("captured page differs from a capture of exactly the reported articles")
				}
				start = next
			}
		})
	}
}
