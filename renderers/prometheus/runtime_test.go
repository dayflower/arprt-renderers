package prometheus

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

func TestRuntimeCLI(t *testing.T) {
	root := t.TempDir()
	binary := os.Getenv("PROMETHEUS_CLI")
	if binary == "" {
		binary = filepath.Join(root, "renderer")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/renderer")
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("build: %s %v", b, e)
		}
	}
	cfg, contextPath, output := filepath.Join(root, "config.json"), filepath.Join(root, "context.json"), filepath.Join(root, "output.png")
	promToken, captureToken := filepath.Join(root, "prom-token"), filepath.Join(root, "capture-token")
	for path, value := range map[string]string{promToken: "prometheus-only", captureToken: "capture-only"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mode atomic.Int32
	var promCalls, captureCalls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer prometheus-only" || r.Method != "POST" || r.URL.Path != "/prefix/api/v1/query" {
			t.Error("Prometheus contract/auth")
		}
		if mode.Load() == 6 {
			w.WriteHeader(400)
			return
		}
		r.ParseForm()
		ts, _ := strconv.ParseInt(r.Form.Get("time"), 10, 64)
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"scalar","result":[%d,"123"]}}`, ts)
	}))
	defer api.Close()
	var expectInsets atomic.Bool
	expectInsets.Store(true)
	cap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captureCalls.Add(1)
		if r.URL.Path != "/capture-prefix/capture" || r.Header.Get("Authorization") != "Bearer capture-only" || r.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Error("capture contract/auth")
		}
		q := r.URL.Query()
		if len(q) != 4 || q.Get("html_mode") != "file" || q.Get("ready_selector") != "#prometheus-ready" {
			t.Error("capture parameters")
		}
		b, _ := io.ReadAll(r.Body)
		if !bytes.Contains(b, []byte(">123</span>")) || bytes.Contains(b, []byte("prometheus-only")) || bytes.Contains(b, []byte(api.URL)) {
			t.Error("unsafe/wrong HTML")
		}
		if expectInsets.Load() && !bytes.Contains(b, []byte("left:12px;top:4px;")) {
			t.Error("CLI omitted safe area")
		}
		w.Header().Set("Content-Type", "image/png")
		switch mode.Load() {
		case 1:
			w.WriteHeader(500)
			return
		case 2:
			w.Header().Set("Content-Type", "text/plain")
		case 3:
			w.Write([]byte("not PNG"))
			return
		case 4:
			w.Write(pngBytes(t, 1, 1))
			return
		case 7:
			<-r.Context().Done()
			return
		case 5:
			http.Redirect(w, r, api.URL, 307)
			return
		}
		width, _ := strconv.Atoi(q.Get("width"))
		height, _ := strconv.Atoi(q.Get("height"))
		w.Write(pngBytes(t, width, height))
	}))
	defer cap.Close()
	raw := fmt.Sprintf(`{"version":1,"prometheus":{"url":%q,"auth":{"type":"bearer","credentialsFile":%q}},"layout":{"landscape":{"areas":["value"]}},"panels":[{"id":"value","type":"value","query":"example_power_watts"}]}`, api.URL+"/prefix", promToken)
	if err := os.WriteFile(cfg, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--config", cfg, "--context", contextPath, "--output", output, "--now", "2026-11-07T12:00:00+09:00", "--webcappi-url", cap.URL + "/capture-prefix", "--webcappi-token-file", captureToken}
	invoke := func(args []string) error {
		cmd := exec.Command(binary, args...)
		b, e := cmd.CombinedOutput()
		if e != nil {
			return fmt.Errorf("%s", b)
		}
		return nil
	}
	for _, palette := range []string{"gray4", "spectra6"} {
		for _, rotation := range []int{0, 90, 180, 270} {
			if err := os.WriteFile(contextPath, insetContextBytes(rotation, palette), 0644); err != nil {
				t.Fatal(err)
			}
			if err := invoke(base); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			assertRotated(t, data, rotation)
			for _, failure := range []int32{1, 2, 3, 4, 5, 6} {
				mode.Store(failure)
				pc := promCalls.Load()
				cc := captureCalls.Load()
				if err := invoke(base); err == nil {
					t.Fatalf("accepted failure %d", failure)
				}
				if failure == 6 && captureCalls.Load() != cc {
					t.Fatal("acquisition failure reached capture")
				}
				if failure == 5 && promCalls.Load() != pc+1 {
					t.Fatal("capture followed redirect")
				}
				after, _ := os.ReadFile(output)
				if !bytes.Equal(after, data) {
					t.Fatal("prior output changed")
				}
			}
			mode.Store(0)
		}
	}
	// Exact snapshots replace API acquisition while still capturing the dashboard.
	c, err := LoadConfig(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := ParseContext(insetContextBytes(270, "spectra6"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(c, target, time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("JST", 9*3600)))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "snapshot")
	os.Mkdir(snapshot, 0755)
	manifest := ExpectedManifest(plan)
	b, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(snapshot, "manifest.json"), b, 0644)
	for _, e := range manifest.Entries {
		os.WriteFile(filepath.Join(snapshot, e.File), []byte(fmt.Sprintf(`{"status":"success","data":{"resultType":"scalar","result":[%d,"123"]}}`, plan.Now)), 0644)
	}
	count := promCalls.Load()
	args := append(append([]string{}, base...), "--input", snapshot)
	if err := invoke(args); err != nil {
		t.Fatal(err)
	}
	if promCalls.Load() != count {
		t.Fatal("snapshot contacted API")
	}
	// The optional restricted-container harness supplies read-only Context files
	// at the protocol defaults and at an independent custom path.
	// Mounted default Contexts deliberately have no safe-area extension.
	expectInsets.Store(false)
	if os.Getenv("PROMETHEUS_DEFAULT_PATHS") == "1" {
		defaultArgs := []string{"--config", cfg, "--now", "2026-11-07T12:00:00+09:00", "--webcappi-url", cap.URL + "/capture-prefix", "--webcappi-token-file", captureToken}
		if err := invoke(defaultArgs); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile("/arprt/output.png")
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, b, 0)
		defaultArgs = append(defaultArgs, "--context", "/input/context.json", "--output", output)
		if err := invoke(defaultArgs); err != nil {
			t.Fatal(err)
		}
		b, err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		assertRotated(t, b, 0)
	}
	prior, _ := os.ReadFile(output)
	mode.Store(7)
	timed, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	if err := Run(timed, base, io.Discard); err == nil {
		t.Fatal("capture cancellation accepted")
	}
	stop()
	mode.Store(0)
	if b, _ := os.ReadFile(output); !bytes.Equal(prior, b) {
		t.Fatal("capture cancellation replaced output")
	}
	badOutput := append(append([]string{}, base...), "--output", root)
	if err := invoke(badOutput); err == nil {
		t.Fatal("output failure accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, base, io.Discard); err == nil {
		t.Fatal("cancellation accepted")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(prior, after) {
		t.Fatal("cancellation replaced output")
	}
	paths, _ := filepath.Glob(filepath.Join(root, ".prometheus-*.tmp"))
	if len(paths) > 0 {
		t.Fatal("temporary leak")
	}
	if captureCalls.Load() == 0 {
		t.Fatal("capture not tested")
	}
}

func TestCaptureRejectURLs(t *testing.T) {
	for _, base := range []string{"file:///tmp/x", "https://user:pass@example.com", "https://example.com?x=1", "https://example.com/#fragment", "http://example.com/a/../b"} {
		if _, err := captureURL(base, 800, 480); err == nil {
			t.Fatal("unsafe capture URL")
		}
	}
	// Default paths are independent of configuration/input/capture paths.
	o, err := ParseOptions([]string{"--config", "synthetic.json"}, time.Now)
	if err != nil || o.Context != "/arprt/context.json" || o.Output != "/arprt/output.png" {
		t.Fatal("defaults")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer server.Close()
	address, _ := captureURL(server.URL, 800, 480)
	if _, err := capture(ctx, address, "", []byte("<html>")); err == nil || strings.Contains(err.Error(), server.URL) {
		t.Fatal("capture cancellation/redaction")
	}
}
