package prometheus

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Real Chromium verifies computed sizes, automatic shrinking, ellipsis and
// readiness failure for fixed-size values that cannot fit vertically.
func TestBrowserValueSizes(t *testing.T) {
	image := os.Getenv("PROMETHEUS_BROWSER_IMAGE")
	if image == "" {
		t.Skip("set PROMETHEUS_BROWSER_IMAGE to a local image containing /opt/chrome/chrome")
	}
	for _, rotation := range []int{0, 90} {
		for _, tc := range []struct {
			name, size, width, height string
			wantSize                  int
			missing, stale, overflow  bool
		}{
			{name: "auto-wide", width: "1fr", height: "1fr", wantSize: 64},
			{name: "auto-narrow", width: "120px", height: "1fr", wantSize: 40},
			{name: "small", size: "small", width: "120px", height: "1fr", wantSize: 40},
			{name: "medium", size: "medium", width: "120px", height: "1fr", wantSize: 48},
			{name: "large", size: "large", width: "120px", height: "1fr", wantSize: 64},
			{name: "xlarge", size: "xlarge", width: "120px", height: "1fr", wantSize: 80},
			{name: "missing", size: "large", width: "120px", height: "1fr", wantSize: 64, missing: true},
			{name: "stale", size: "xlarge", width: "120px", height: "1fr", wantSize: 80, stale: true},
			{name: "too-short", size: "xlarge", width: "120px", height: "100px", wantSize: 80, overflow: true},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, rotation), func(t *testing.T) {
				c := config(t)
				c.Panels = []Panel{{ID: "value", Type: "value", Title: "Power", Query: "up", Size: tc.size, Prefix: "$", Suffix: " W"}}
				grid := Grid{Areas: []string{"value .", ". ."}, Columns: tc.width + " 1fr", Rows: tc.height + " 1fr"}
				c.Layout.Landscape = grid
				c.Layout.Portrait = &grid
				target := contextFixture(t)
				target.Rotation = rotation
				m := Model{Panels: []PanelModel{{ID: "value", Value: &ValueModel{Number: 12345678, Label: "Power", Stale: tc.stale}}}}
				if tc.name == "auto-wide" {
					m.Panels[0].Value.Number = 123
				}
				if tc.missing {
					m.Panels[0].Value = nil
				}
				body, err := RenderHTML(c, target, m)
				if err != nil {
					t.Fatal(err)
				}
				geometry := browserValueGeometry(t, image, body)
				var result struct {
					Ready    bool
					Error    string
					Size     int
					Clipped  bool
					Ellipsis string
				}
				if err := json.Unmarshal(geometry, &result); err != nil {
					t.Fatal(err)
				}
				if result.Size != tc.wantSize || result.Ready == tc.overflow || (tc.overflow && result.Error != "overflow") {
					t.Fatalf("unexpected geometry: %+v", result)
				}
				if tc.width == "120px" && !tc.missing && !tc.overflow && (!result.Clipped || result.Ellipsis != "ellipsis") {
					t.Fatalf("long value did not use ellipsis: %+v", result)
				}
			})
		}
	}
}

// DevTools pipe messages are NUL-delimited JSON. Waiting for the production
// readiness marker avoids racing requestAnimationFrame with a DOM dump.
func browserValueGeometry(t *testing.T, image string, body []byte) json.RawMessage {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "value.html"), body, 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-i", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", "1000:1000", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/work:rw,mode=1777", "--mount", "type=bind,source="+dir+",target=/input,readonly", "--entrypoint", "/bin/sh", image, "-c", "exec /opt/chrome/chrome --headless --no-sandbox --user-data-dir=/tmp/browser --disable-gpu --disable-dev-shm-usage --no-zygote --remote-debugging-pipe 3<&0 4>&1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = input.Close()
		cancel()
		_ = cmd.Wait()
	}()
	reader := bufio.NewReader(output)
	id := 0
	call := func(method, session string, params map[string]any) json.RawMessage {
		t.Helper()
		id++
		message := map[string]any{"id": id, "method": method, "params": params}
		if session != "" {
			message["sessionId"] = session
		}
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := input.Write(append(data, 0)); err != nil {
			t.Fatal(err)
		}
		for {
			data, err := reader.ReadBytes(0)
			if err != nil {
				cancel()
				_ = cmd.Wait()
				t.Fatalf("browser pipe: %v; %s", err, stderr.String())
			}
			var response struct {
				ID     int
				Result json.RawMessage
				Error  json.RawMessage
			}
			if err := json.Unmarshal(data[:len(data)-1], &response); err != nil {
				t.Fatal(err)
			}
			if response.ID != id {
				continue
			}
			if len(response.Error) != 0 {
				t.Fatalf("DevTools %s: %s", method, response.Error)
			}
			return response.Result
		}
	}
	var target struct{ TargetID string }
	if err := json.Unmarshal(call("Target.createTarget", "", map[string]any{"url": "about:blank"}), &target); err != nil {
		t.Fatal(err)
	}
	var session struct{ SessionID string }
	if err := json.Unmarshal(call("Target.attachToTarget", "", map[string]any{"targetId": target.TargetID, "flatten": true}), &session); err != nil {
		t.Fatal(err)
	}
	call("Page.navigate", session.SessionID, map[string]any{"url": "file:///input/value.html"})
	result := call("Runtime.evaluate", session.SessionID, map[string]any{"awaitPromise": true, "returnByValue": true, "expression": `(async () => {
	  while (!document.querySelector('#prometheus-ready') && !document.body?.dataset.layoutError) {
	    await new Promise(resolve => setTimeout(resolve, 10));
	  }
	  const number = document.querySelector('.number');
	  const span = number.firstElementChild;
	  return {ready: !!document.querySelector('#prometheus-ready'),
	    error: document.body.dataset.layoutError || '',
	    size: parseFloat(getComputedStyle(number).fontSize),
	    clipped: span.scrollWidth > span.clientWidth,
	    ellipsis: getComputedStyle(span).textOverflow};
	})()`})
	var evaluated struct {
		Result struct{ Value json.RawMessage }
	}
	if err := json.Unmarshal(result, &evaluated); err != nil || len(evaluated.Result.Value) == 0 {
		t.Fatalf("browser did not return geometry: %s (%v)", result, err)
	}
	return evaluated.Result.Value
}
