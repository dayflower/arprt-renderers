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
	"testing"
	"time"
)

func TestCLI(t *testing.T) { checkCLI(t, "") }
func TestProductionCLI(t *testing.T) {
	binary := os.Getenv("WEATHER_CLI")
	if binary == "" {
		t.Skip("set WEATHER_CLI to test the shipped executable")
	}
	if os.Getuid() != 1000 || os.Getgid() != 1000 {
		t.Fatal("wrong Job identity")
	}
	if e := os.WriteFile("/app/forbidden", []byte("x"), 0o600); e == nil {
		t.Fatal("writable runtime root")
	}
	for _, p := range []string{"/arprt/context.json", "/input/context.json"} {
		f, e := os.OpenFile(p, os.O_WRONLY, 0)
		if e == nil {
			f.Close()
			t.Fatal("writable Context")
		}
	}
	checkCLI(t, binary)
}
func checkCLI(t *testing.T, binary string) {
	t.Helper()
	root := t.TempDir()
	contextPath := filepath.Join(root, "context.json")
	output := filepath.Join(root, "result.png")
	input := filepath.Join(root, "forecast.json")
	token := filepath.Join(root, "token")
	if e := os.WriteFile(input, encoded(t, syntheticForecast(time.Now(), "UTC", "°C")), 0o644); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(token, []byte("synthetic-token\n"), 0o600); e != nil {
		t.Fatal(e)
	}
	var failed atomic.Bool
	var captured atomic.Value
	var expectInsets atomic.Bool
	expectInsets.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			w.WriteHeader(500)
			return
		}
		if r.URL.Path != "/capture" || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.URL.Query().Get("ready_selector") != "#weather-ready" {
			t.Error("incorrect capture contract")
		}
		b, _ := io.ReadAll(r.Body)
		if expectInsets.Load() && !bytes.Contains(b, []byte("left:12px;top:4px;")) {
			t.Error("CLI omitted safe area")
		}
		captured.Store(string(b))
		if !bytes.Contains(b, []byte("7-Day Forecast")) {
			t.Error("missing weather HTML")
		}
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		height, _ := strconv.Atoi(r.URL.Query().Get("height"))
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes(t, width, height))
	}))
	defer server.Close()
	invoke := func(args []string) error {
		if binary != "" {
			cmd := exec.Command(binary, args...)
			b, e := cmd.CombinedOutput()
			if e != nil {
				return fmt.Errorf("CLI: %s: %w", b, e)
			}
			return nil
		}
		return run(context.Background(), args, io.Discard, io.Discard)
	}
	base := []string{"--latitude", "0", "--longitude", "0", "--input", input, "--webcappi-url", server.URL, "--webcappi-token-file", token}
	for _, palette := range []string{"gray4", "spectra6"} {
		for _, rotation := range []int{0, 90, 180, 270} {
			if e := os.WriteFile(contextPath, insetContextBytes(rotation, palette), 0o644); e != nil {
				t.Fatal(e)
			}
			args := append(append([]string{}, base...), "--context", contextPath, "--output", output)
			if rotation == 0 {
				args = append(args, "--location-name", "  Example <Harbor> & 湾  ")
			}
			if e := invoke(args); e != nil {
				t.Fatal(e)
			}

			html := captured.Load().(string)
			if !strings.Contains(html, `data-palette="`+palette+`"`) || strings.Contains(html, "--warm:") != (palette == "spectra6") {
				t.Fatal("capture HTML does not match the target palette")
			}
			if strings.Contains(html, "UTC") || strings.Contains(html, `class="location"`) != (rotation == 0) {
				t.Fatal("unexpected timezone or optional location display")
			}
			if rotation == 0 && (!strings.Contains(html, ">Example &lt;Harbor&gt; &amp; 湾</div>") || strings.Contains(html, "<Harbor>")) {
				t.Fatal("location must be trimmed and HTML escaped")
			}
			before := mustRead(t, output)
			assertRotated(t, before, rotation)
			failed.Store(true)
			if e := invoke(args); e == nil {
				t.Fatal("capture failure accepted")
			}
			failed.Store(false)
			if !bytes.Equal(before, mustRead(t, output)) {
				t.Fatal("prior output replaced")
			}
		}
	}
	for _, name := range []string{"   ", strings.Repeat("湾", 128)} {
		args := append(append([]string{}, base...), "--context", contextPath, "--output", output, "--location-name", name)
		if e := invoke(args); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(captured.Load().(string), `class="location"`) != (strings.TrimSpace(name) != "") {
			t.Fatal("incorrect optional location")
		}
	}
	// Mounted default Contexts deliberately have no safe-area extension.
	expectInsets.Store(false)
	if binary != "" {
		if e := invoke(base); e != nil {
			t.Fatal(e)
		}
		assertRotated(t, mustRead(t, "/arprt/output.png"), 0)
		args := append(append([]string{}, base...), "--context", "/input/context.json", "--output", output)
		if e := invoke(args); e != nil {
			t.Fatal(e)
		}
		assertRotated(t, mustRead(t, output), 0)
	}
	for _, extra := range [][]string{{"--latitude", "NaN"}, {"--latitude", "Inf"}, {"--latitude", "91"}, {"--longitude", "181"}, {"--model", "a,b"}, {"--model", "x y"}, {"--temperature-unit", "kelvin"}, {"--location-name", strings.Repeat("湾", 129)}, {"--location-name", "Example\nCity"}, {"--location-name", string([]byte{0xff})}, {"unexpected"}} {
		args := append(append([]string{}, base...), "--context", contextPath, "--output", output)
		args = append(args, extra...)
		if e := invoke(args); e == nil {
			t.Fatalf("accepted %v", extra)
		}
	}
	for _, args := range [][]string{{"--webcappi-url", server.URL}, {"--latitude", "0", "--webcappi-url", server.URL}, {"--longitude", "0", "--webcappi-url", server.URL}} {
		if e := invoke(args); e == nil {
			t.Fatal("missing coordinates accepted")
		}
	}
	// Invalid input and cancellation cannot replace a successful image.
	before := mustRead(t, output)
	for _, data := range []string{`{}`, `[]`, `{"timezone":"UTC","current":null}`, strings.Repeat("x", forecastLimit+1)} {
		if e := os.WriteFile(input, []byte(data), 0o644); e != nil {
			t.Fatal(e)
		}
		args := append(append([]string{}, base...), "--context", contextPath, "--output", output)
		if e := invoke(args); e == nil {
			t.Fatal("malformed input accepted")
		}
		if !bytes.Equal(before, mustRead(t, output)) {
			t.Fatal("input failure replaced output")
		}
	}
	paths, _ := filepath.Glob(filepath.Join(root, ".weather-*.tmp"))
	if len(paths) != 0 {
		t.Fatal("temporary file leak")
	}
}
