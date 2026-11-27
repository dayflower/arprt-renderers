package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func paginationFeed(letters string) string {
	var text strings.Builder
	text.WriteString(`<rss version="2.0"><channel><title>Pagination</title>`)
	for _, letter := range letters {
		fmt.Fprintf(&text, `<item><title>Article %c</title><guid>%c</guid><description>%s</description></item>`,
			letter, letter, strings.Repeat("Synthetic article summary with enough text to wrap. ", 8))
	}
	text.WriteString(`</channel></rss>`)
	return text.String()
}

func withRendererState(t *testing.T, data []byte, state json.RawMessage) []byte {
	t.Helper()
	var input map[string]json.RawMessage
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if len(state) > 0 && string(state) != "null" {
		input["rendererState"] = state
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPaginationCLI(t *testing.T) { checkPaginationCLI(t, "") }

func checkPaginationCLI(t *testing.T, binary string) {
	t.Helper()
	root := t.TempDir()
	contextPath := filepath.Join(root, "context.json")
	feedPath := filepath.Join(root, "feed.xml")
	output := filepath.Join(root, "screen.png")
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
	type reply struct {
		want   string
		counts []string
		paged  bool
	}
	replies := make(chan reply, 1)
	png := pngBytes(t, 800, 480)
	headline := regexp.MustCompile(`<h2>Article ([A-Z])</h2>`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response reply
		select {
		case response = <-replies:
		default:
			t.Error("unexpected capture request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var got strings.Builder
		for _, match := range headline.FindAllSubmatch(data, -1) {
			got.Write(match[1])
		}
		if got.String() != response.want {
			t.Errorf("candidate articles = %q, want %q", got.String(), response.want)
		}
		query := r.URL.Query().Get("capture_metadata")
		if (response.paged && query != "attributeValue:data-count:#rss-ready") || (!response.paged && query != "") {
			t.Errorf("unexpected capture_metadata %q", query)
		}
		w.Header().Set("Content-Type", "image/png")
		for _, count := range response.counts {
			w.Header().Add(captureMetadataHeader, "b64url="+base64.RawURLEncoding.EncodeToString([]byte(count)))
		}
		w.Write(png)
	}))
	defer server.Close()
	invoke := func(extra ...string) error {
		defer func() {
			if len(replies) != 0 {
				t.Fatal("expected a capture request")
			}
		}()
		args := append([]string{"--context", contextPath, "--input", feedPath, "--output", output, "--webcappi-url", server.URL, "--images=false"}, extra...)
		if binary != "" {
			data, err := exec.Command(binary, args...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("CLI failed: %w: %s", err, data)
			}
			return nil
		}
		return run(context.Background(), args, io.Discard, io.Discard)
	}
	ctx := contextBytes(0, "gray4")
	var state json.RawMessage
	for _, step := range []struct {
		feed, candidates string
		count            int
		cleared          bool
	}{
		{"ABCDEFGHIJ", "ABCDEFGH", 3, false},
		{"XABCDEFGHIJ", "DEFGHIJ", 7, true},
		{"XABCDEFGHIJ", "XABCDEFG", 2, false},
		{"BCDEFGHIJ", "BCDEFGHI", 2, false},
	} {
		write(feedPath, []byte(paginationFeed(step.feed)))
		write(contextPath, withRendererState(t, ctx, state))
		replies <- reply{step.candidates, []string{strconv.Itoa(step.count)}, true}
		if err := invoke("--paginate"); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Version   string          `json:"version"`
			NextState json.RawMessage `json:"nextState"`
		}
		if err := json.Unmarshal(read(result), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Version != "v1" || len(envelope.NextState) == 0 || (string(envelope.NextState) == "null") != step.cleared {
			t.Fatalf("unexpected result: %+v", envelope)
		}
		state = envelope.NextState
	}
	assertRotated(t, read(output), 0)

	write(feedPath, []byte(paginationFeed("")))
	write(contextPath, withRendererState(t, ctx, state))
	custom := filepath.Join(root, "custom.json")
	replies <- reply{"", []string{"0"}, true}
	if err := invoke("--paginate", "--result", custom); err != nil {
		t.Fatal(err)
	}
	if string(read(custom)) != `{"version":"v1"}` {
		t.Fatal("empty feed should preserve committed state")
	}

	write(feedPath, []byte(paginationFeed("BCDEFGHIJ")))
	previousPNG, previousState := read(output), read(result)
	for _, counts := range [][]string{nil, {"0"}, {"3"}, {"bad"}, {"-1"}, {"01"}, {"1", "1"}} {
		replies <- reply{"DE", counts, true}
		if err := invoke("--paginate", "--max-items", "2"); err == nil {
			t.Fatalf("invalid capture counts accepted: %v", counts)
		}
		if !bytes.Equal(read(output), previousPNG) || !bytes.Equal(read(result), previousState) {
			t.Fatal("invalid capture replaced an existing output")
		}
	}
	if err := os.Mkdir(filepath.Join(root, "blocked.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(output, []byte("previous image"))
	replies <- reply{"DE", []string{"1"}, true}
	if err := invoke("--paginate", "--max-items", "2", "--result", filepath.Join(root, "blocked.json")); err == nil {
		t.Fatal("result replacement unexpectedly succeeded")
	}
	if string(read(output)) != "previous image" {
		t.Fatal("result write failure replaced the PNG")
	}
	entries, err := filepath.Glob(filepath.Join(root, ".rss-*.tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v (%v)", entries, err)
	}
	for _, path := range []string{output, contextPath, feedPath, filepath.Join(root, "nested", "result.json")} {
		if err := invoke("--paginate", "--result", path); err == nil {
			t.Fatalf("invalid result path accepted: %s", path)
		}
	}
	if err := invoke("--result", result); err == nil {
		t.Fatal("result path accepted with pagination disabled")
	}
	replies <- reply{want: "BCDEFGHI"}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(result), previousState) {
		t.Fatal("pagination disabled but state result replaced")
	}
}

func TestCursorWithoutIDs(t *testing.T) {
	items := []article{{Title: "same"}, {Title: "same"}, {Title: "later"}}
	keys := articleKeys(items)
	if keys[0] == keys[1] || !reflect.DeepEqual(keys, articleKeys(append([]article{{Title: "new"}}, items...))[1:]) {
		t.Fatal("fallback article keys are not distinct and stable across new head articles")
	}
	result, err := pageResult(keys, len(keys), 2)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(result, &envelope); err != nil {
		t.Fatal(err)
	}
	if start := pageStart(keys, envelope["nextState"]); start != 2 {
		t.Fatalf("resumed at %d, want 2", start)
	}
	for _, state := range []string{`{"version":"v2","after":"` + keys[1] + `"}`, `{"version":"v1","after":4}`, `true`, `null`} {
		if start := pageStart(keys, json.RawMessage(state)); start != 0 {
			t.Fatalf("unknown or incompatible cursor resumed at %d", start)
		}
	}
}
