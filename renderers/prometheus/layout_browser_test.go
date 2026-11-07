package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This is a geometry experiment, not the eventual renderer HTML or a screenshot
// acceptance test. It compares the Go sampling width with browser track sizing.
func TestBrowserPlotWidths(t *testing.T) {
	image := os.Getenv("PROMETHEUS_BROWSER_IMAGE")
	if image == "" {
		t.Skip("set PROMETHEUS_BROWSER_IMAGE to a local image containing /opt/chrome/chrome")
	}
	for _, name := range []string{"landscape", "portrait", "explicit-portrait", "auto", "wrap", "grow", "pixels"} {
		t.Run(name, func(t *testing.T) {
			c := config(t)
			ctx := contextFixture(t)
			switch name {
			case "portrait":
				ctx.Rotation = 90
			case "explicit-portrait":
				ctx.Rotation = 270
				c.Layout.Portrait = &Grid{Areas: []string{"heading heading", "current history", "daily history"}, Columns: "1fr 1fr", Rows: "40px 1fr 1fr", Gap: 8}
			case "auto":
				c.Layout.Landscape.Columns = "auto 1fr"
			case "wrap":
				c.Panels[1].Wrap = true
			case "grow":
				c.Panels[1].Panels = append(c.Panels[1].Panels, Panel{ID: "extra", Type: "text", Grow: 2, Text: "Example"})
			case "pixels":
				c.Layout.Landscape.Columns = "264px 528px"
			}
			g := c.Layout.Landscape
			if ctx.Rotation%180 != 0 {
				g = *c.Layout.Portrait
			}
			w, h := ctx.LogicalSize()
			cols, _ := tracks(g.Columns, len(cells(g)[0]))
			rows, _ := tracks(g.Rows, len(g.Areas))
			var page strings.Builder
			fmt.Fprintf(&page, `<!doctype html><meta charset="utf-8"><style>*{box-sizing:border-box}html,body{margin:0;width:%dpx;height:%dpx}.grid{display:grid;width:100%%;height:100%%;grid-template-columns:%s;grid-template-rows:%s;gap:%dpx;grid-template-areas:`, w, h, strings.Join(cols, " "), strings.Join(rows, " "), g.Gap)
			for _, row := range cells(g) {
				page.WriteString(`"`)
				for _, id := range row {
					if id != "." {
						id = "p-" + id
					}
					page.WriteString(id + " ")
				}
				page.WriteString(`" `)
			}
			page.WriteString(`}.panel{min-width:0;min-height:0}.plot{width:calc(100% - 96px);min-width:1px;height:20px}.group{display:flex;min-width:0;min-height:0}.child{min-width:0;min-height:0}</style><div class="grid">`)
			for _, p := range c.Panels {
				fmt.Fprintf(&page, `<div class="panel" style="grid-area:p-%s">`, p.ID)
				if p.Type == "group" {
					gap := g.Gap
					if p.Gap != nil {
						gap = *p.Gap
					}
					wrap := "nowrap"
					basis := "0px"
					if p.Wrap {
						wrap = "wrap"
						basis = "120px"
					}
					fmt.Fprintf(&page, `<div class="group" style="width:100%%;height:100%%;flex-direction:%s;flex-wrap:%s;gap:%dpx">`, p.Direction, wrap, gap)
					for _, ch := range p.Panels {
						fmt.Fprintf(&page, `<div class="child" style="flex:%d 1 %s"><div class="plot" id="%s"></div></div>`, ch.Grow, basis, ch.ID)
					}
					page.WriteString(`</div>`)
				} else {
					fmt.Fprintf(&page, `<div class="plot" id="%s"></div>`, p.ID)
				}
				page.WriteString(`</div>`)
			}
			page.WriteString(`</div><pre id="result"></pre><script>const r={};document.querySelectorAll('.plot').forEach(e=>r[e.id]=e.getBoundingClientRect().width);document.querySelector('#result').textContent=JSON.stringify(r);</script>`)
			dir := t.TempDir()
			path := filepath.Join(dir, "layout.html")
			if e := os.WriteFile(path, []byte(page.String()), 0644); e != nil {
				t.Fatal(e)
			}
			timeout, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(timeout, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", "1000:1000", "--tmpfs", "/tmp:rw,mode=1777", "--tmpfs", "/work:rw,mode=1777", "--mount", "type=bind,source="+dir+",target=/input,readonly", "--entrypoint", "/opt/chrome/chrome", image, "--headless", "--no-sandbox", "--user-data-dir=/tmp/browser", "--disable-gpu", "--disable-dev-shm-usage", "--no-zygote", "--dump-dom", "--window-size="+strconv.Itoa(w)+","+strconv.Itoa(h), "file:///input/layout.html")
			output, e := cmd.Output()
			if e != nil {
				if ee, ok := e.(*exec.ExitError); ok {
					t.Fatalf("browser execution failed: %s", ee.Stderr)
				}
				t.Fatal(e)
			}
			match := regexp.MustCompile(`<pre id="result">([^<]+)</pre>`).FindSubmatch(output)
			if match == nil {
				t.Fatal("browser did not return geometry")
			}
			var actual map[string]float64
			if e = json.Unmarshal(match[1], &actual); e != nil {
				t.Fatal(e)
			}
			for id, want := range PlotWidths(c, ctx) {
				got, ok := actual[id]
				if !ok || float64(want) > got+1 {
					t.Errorf("%s planned=%d browser=%.3f", id, want, got)
				}
			}
			t.Logf("sampling widths=%v; browser widths=%v", PlotWidths(c, ctx), actual)
		})
	}
}
