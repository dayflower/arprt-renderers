package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCLI(t *testing.T) { checkCLI(t, "") }

func TestProductionCLI(t *testing.T) {
	binary := os.Getenv("COUNTDOWN_CLI")
	if binary == "" {
		t.Skip("set COUNTDOWN_CLI to test the shipped executable")
	}
	if os.Getuid() != 1000 || os.Getgid() != 1000 {
		t.Fatal("wrong Job identity")
	}
	if err := os.WriteFile("/app/forbidden", []byte("x"), 0o600); err == nil {
		t.Fatal("writable runtime root")
	}
	for _, path := range []string{"/arprt/context.json", "/input/context.json"} {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			t.Fatal("writable Context")
		}
	}
	checkCLI(t, binary)
}

func checkCLI(t *testing.T, binary string) {
	t.Helper()
	root := t.TempDir()
	configPath, contextPath := filepath.Join(root, "config.json"), filepath.Join(root, "context.json")
	output, token := filepath.Join(root, "result.png"), filepath.Join(root, "token")
	configuration := []byte(`{"version":1,"targetDate":"2027-01-01","timezone":"Asia/Tokyo","title":"Example <Harbor> & 湾"}`)
	put(t, configPath, configuration)
	put(t, token, []byte("synthetic-token\n"))
	// Projected volumes expose configuration through symlinks.
	link := filepath.Join(root, "projected.json")
	if err := os.Symlink(configPath, link); err != nil {
		t.Fatal(err)
	}
	var mode atomic.Int32
	var captures atomic.Int32
	var captured atomic.Value
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captures.Add(1)
		b, _ := io.ReadAll(r.Body)
		if mode.Load() == 2 {
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		if mode.Load() == 1 {
			w.WriteHeader(500)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/capture" || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.URL.Query().Get("ready_selector") != "#countdown-ready" || r.URL.Query().Get("html_mode") != "file" {
			t.Error("incorrect capture contract")
		}
		captured.Store(string(b))
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		height, _ := strconv.Atoi(r.URL.Query().Get("height"))
		w.Header().Set("Content-Type", "image/png")
		if mode.Load() == 3 {
			w.Write([]byte("not a PNG"))
		} else {
			w.Write(pngBytes(t, width, height))
		}
	}))
	defer server.Close()
	clockCalls := 0
	invoke := func(args []string) error {
		if binary != "" {
			cmd := exec.Command(binary, args...)
			b, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("CLI: %s: %w", b, err)
			}
			return nil
		}
		return runWithClock(context.Background(), args, io.Discard, io.Discard, func() time.Time {
			clockCalls++
			return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		})
	}
	base := []string{"--config", link, "--webcappi-url", server.URL, "--webcappi-token-file", token}
	args := append(append([]string{}, base...), "--context", contextPath, "--output", output)
	for _, palette := range []string{"gray4", "spectra6"} {
		for _, rotation := range []int{0, 90, 180, 270} {
			put(t, contextPath, insetContextBytes(rotation, palette))
			beforeCalls := clockCalls
			if err := invoke(args); err != nil {
				t.Fatal(err)
			}
			html := captured.Load().(string)
			if !strings.Contains(html, `left:12px;top:4px;`) || !strings.Contains(html, `Example &lt;Harbor&gt; &amp; 湾`) {
				t.Fatal("safe area or escaped title missing")
			}
			if binary == "" && (clockCalls != beforeCalls+1 || !strings.Contains(html, `>52</div>`)) {
				t.Fatal("clock must be sampled once; incorrect reference count")
			}
			prior := mustRead(t, output)
			assertRotated(t, prior, rotation)
			for _, failure := range []int32{1, 3} {
				mode.Store(failure)
				if err := invoke(args); err == nil || !bytes.Equal(prior, mustRead(t, output)) {
					t.Fatal("capture failure replaced prior output")
				}
			}
			mode.Store(0)
		}
	}
	if binary != "" {
		if err := invoke(base); err != nil {
			t.Fatal(err)
		}
		assertRotated(t, mustRead(t, "/arprt/output.png"), 0)
		custom := append(append([]string{}, base...), "--context", "/input/context.json", "--output", output)
		if err := invoke(custom); err != nil {
			t.Fatal(err)
		}
	}
	prior := mustRead(t, output)
	for _, bad := range []string{`{}`, `{"version":1,"targetDate":"2027-02-29"}`, strings.Repeat("x", configLimit+1)} {
		put(t, configPath, []byte(bad))
		before := captures.Load()
		if err := invoke(args); err == nil || captures.Load() != before || !bytes.Equal(prior, mustRead(t, output)) {
			t.Fatal("invalid configuration reached capture or replaced output")
		}
	}
	put(t, configPath, configuration)
	for _, path := range []string{contextPath, token} {
		original := mustRead(t, path)
		for _, bad := range []string{"bad token", strings.Repeat("x", (1<<20)+1)} {
			put(t, path, []byte(bad))
			before := captures.Load()
			if err := invoke(args); err == nil || captures.Load() != before {
				t.Fatal("invalid input reached capture")
			}
		}
		put(t, path, original)
	}
	for _, bad := range [][]string{{}, {"--config", configPath}, {"--webcappi-url", server.URL}, append(args, "unexpected")} {
		if err := invoke(bad); err == nil {
			t.Fatal("accepted invalid CLI arguments")
		}
	}
	// Cancellation and the shipped CLI's SIGTERM path must interrupt capture
	// and leave the previous image intact.
	mode.Store(2)
	done := make(chan error, 1)
	if binary != "" {
		cmd := exec.Command(binary, args...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait() }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
			t.Fatal("CLI did not start capture")
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { done <- run(ctx, args, io.Discard, io.Discard) }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("capture did not start")
		}
		cancel()
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled execution succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capture was not cancelled promptly")
	}
	if !bytes.Equal(prior, mustRead(t, output)) {
		t.Fatal("failure replaced prior output")
	}
	paths, _ := filepath.Glob(filepath.Join(root, ".countdown-*.tmp"))
	if len(paths) != 0 {
		t.Fatal("temporary file leak")
	}
}
