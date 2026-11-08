package ical

import (
	"bytes"
	"context"
	"encoding/json"
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
	"testing"
	"time"
)

func TestRendererCLI(t *testing.T) { checkRenderer(t, "") }
func TestProductionCLI(t *testing.T) {
	binary := os.Getenv("ICAL_CLI")
	if binary == "" {
		t.Skip("set ICAL_CLI to exercise production binary")
	}
	if os.Getuid() != 1000 || os.Getgid() != 1000 {
		t.Fatal("incorrect Job identity")
	}
	for _, path := range []string{"/app/forbidden", "/tmp/forbidden", "/arprt/context.json", "/input/context.json"} {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		if err == nil {
			f.Close()
			t.Fatalf("unexpected writable path %s", path)
		}
	}
	checkRenderer(t, binary)
}
func checkRenderer(t *testing.T, binary string) {
	root := t.TempDir()
	cp := filepath.Join(root, "context.json")
	config := filepath.Join(root, "config.json")
	output := filepath.Join(root, "result.png")
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("synthetic-capture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var failure, hold atomic.Bool
	started := make(chan struct{}, 1)
	var captures atomic.Int32
	var expectInsets atomic.Bool
	expectInsets.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar" {
			if r.Header.Get("Authorization") != "" {
				t.Error("capture credentials leaked to calendar")
			}
			data, err := fixtures.ReadFile("fixtures/vendor-properties.ics")
			if err != nil {
				t.Error(err)
				http.Error(w, "fixture unavailable", http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, strings.ReplaceAll(string(data), "20261102", time.Now().In(time.FixedZone("JST", 9*3600)).Format("20060102")))
			return
		}
		captures.Add(1)
		body, _ := io.ReadAll(r.Body)
		if hold.Load() {
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		if failure.Load() {
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, "broken PNG")
			return
		}
		if !bytes.Contains(body, []byte("Synthetic vendor event")) || r.Header.Get("Authorization") != "Bearer synthetic-capture" || r.URL.Query().Get("ready_selector") != "#ical-ready" {
			t.Error("incorrect capture contract")
		}
		if expectInsets.Load() && !bytes.Contains(body, []byte("left:12px;top:4px;")) {
			t.Error("CLI omitted safe area")
		}
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		height, _ := strconv.Atoi(r.URL.Query().Get("height"))
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes(t, width, height))
	}))
	defer server.Close()
	executable, _ := os.Executable()
	t.Setenv("ICAL_TEST_WORKER", "1")
	worker := []string{executable, "-test.run=^TestWorker$"}
	invoke := func(ctx context.Context, args []string) error {
		if binary != "" {
			cmd := exec.CommandContext(ctx, binary, args...)
			b, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %w", b, err)
			}
			return nil
		}
		return Run(ctx, args, worker, io.Discard)
	}
	for _, view := range []string{"month", "agenda"} {
		data, _ := json.Marshal(map[string]any{"version": 1, "view": view, "calendars": []any{map[string]any{"id": "work", "name": "仕事", "url": server.URL + "/calendar"}}})
		if err := os.WriteFile(config, data, 0644); err != nil {
			t.Fatal(err)
		}
		for _, palette := range []string{"gray4", "spectra6"} {
			for _, rotation := range []int{0, 90, 180, 270} {
				if err := os.WriteFile(cp, insetContextBytes(rotation, palette), 0644); err != nil {
					t.Fatal(err)
				}
				args := []string{"--config", config, "--context", cp, "--output", output, "--webcappi-url", server.URL, "--webcappi-token-file", token}
				if err := invoke(context.Background(), args); err != nil {
					t.Fatal(err)
				}
				prior, _ := os.ReadFile(output)
				assertRotated(t, prior, rotation)
				failure.Store(true)
				if err := invoke(context.Background(), args); err == nil {
					t.Fatal("accepted invalid PNG")
				}
				failure.Store(false)
				after, _ := os.ReadFile(output)
				if !bytes.Equal(prior, after) {
					t.Fatal("failed render replaced output")
				}
			}
		}
	}
	args := []string{"--config", config, "--context", cp, "--output", output, "--webcappi-url", server.URL, "--webcappi-token-file", token}
	prior, _ := os.ReadFile(output)
	hold.Store(true)
	if binary != "" {
		cmd := exec.Command(binary, args...)
		var log bytes.Buffer
		cmd.Stdout = &log
		cmd.Stderr = &log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Fatal("capture did not start")
		}
		cmd.Process.Signal(os.Interrupt)
		if cmd.Wait() == nil {
			t.Fatal("signal succeeded")
		}
		if !strings.Contains(log.String(), "capture:") || strings.Contains(log.String(), server.URL) {
			t.Fatal("unsafe signal diagnostic", log.String())
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := invoke(ctx, args); err == nil {
			t.Fatal("cancelled render succeeded")
		}
	}
	hold.Store(false)
	after, _ := os.ReadFile(output)
	if !bytes.Equal(prior, after) {
		t.Fatal("signal changed output")
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".ical-*.tmp")); len(matches) != 0 {
		t.Fatal("temporary file leak")
	}
	// Mounted default Contexts deliberately have no safe-area extension.
	expectInsets.Store(false)
	if binary != "" {
		args = []string{"--config", config, "--webcappi-url", server.URL, "--webcappi-token-file", token}
		if err := invoke(context.Background(), args); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile("/arprt/output.png")
		assertRotated(t, data, 0)
		args = append(args, "--context", "/input/context.json", "--output", output)
		if err := invoke(context.Background(), args); err != nil {
			t.Fatal(err)
		}
	}
	before := captures.Load()
	if err := os.WriteFile(config, []byte(`{"version":1,"calendars":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := invoke(context.Background(), args); err == nil {
		t.Fatal("accepted invalid sources")
	}
	if captures.Load() != before {
		t.Fatal("invalid input reached capture")
	}
}
