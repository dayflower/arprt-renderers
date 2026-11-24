package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Synthetic responses exercise rendering without validating operator PromQL.
func syntheticModel(t *testing.T, c Config, target Context) (Plan, Model, map[string]json.RawMessage) {
	t.Helper()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("JST", 9*3600))
	plan, err := BuildPlan(c, target, now)
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]json.RawMessage{}
	for _, r := range plan.Requests {
		var result any
		kind := "vector"
		switch r.Role {
		case "range":
			kind = "matrix"
			series := []any{}
			for si := 0; si < len(c.Panels)+1 && si < 2; si++ {
				values := []any{}
				for ts := r.Start; ts <= r.End; ts += r.Step {
					if ts > r.Start+3600*8 && ts < r.Start+3600*8+1800 {
						continue
					}
					v := 100*math.Sin(float64(ts-r.Start)/7200) + float64(si)*20 + float64(r.Series)*30
					if r.Panel == "remaining-history" {
						v = 80 - 30*float64(ts-r.Start)/float64(r.End-r.Start) + float64(si)*5
					}
					values = append(values, []any{ts, fmt.Sprint(v)})
				}
				series = append(series, map[string]any{"metric": map[string]string{"source": fmt.Sprintf("Source %d", si+1)}, "values": values})
			}
			result = series
		default:
			v := 123.4
			if r.Role == "freshness" {
				v = float64(plan.Now - 7200)
			}
			if r.Role == "bucket" {
				v = float64((r.Bucket + 1) * 10)
				if r.Bucket == 2 {
					result = []any{}
					break
				}
			}
			result = []any{map[string]any{"metric": map[string]string{}, "value": []any{r.Time, fmt.Sprint(v)}}}
			if r.Role == "bucket" {
				result = []any{map[string]any{"metric": map[string]string{"source": "Import"}, "value": []any{r.Time, fmt.Sprint(v)}}, map[string]any{"metric": map[string]string{"source": "Export"}, "value": []any{r.Time, fmt.Sprint(-v / 3)}}}
			}
		}
		b, _ := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"resultType": kind, "result": result}})
		raw[r.ID] = b
	}
	responses, err := ParseResponses(plan, raw)
	if err != nil {
		t.Fatal(err)
	}
	model, err := Normalize(c, plan, responses)
	if err != nil {
		t.Fatal(err)
	}
	return plan, model, raw
}

func TestDisplaySemantics(t *testing.T) {
	c := config(t)
	target := contextFixture(t)
	_, m, _ := syntheticModel(t, c, target)
	for _, palette := range []string{"gray4", "spectra6"} {
		target.Target.Palette = palette
		b, err := RenderHTML(c, target, m)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range []string{`class="segment"`, `* partial`, `! Stale`, `prometheus-ready`, `document.fonts.ready`, `Power`, `No data`} {
			if want == "No data" {
				continue
			}
			if !strings.Contains(s, want) {
				t.Errorf("missing %s", want)
			}
		}
		if strings.Contains(s, "example_power_watts") || strings.Contains(s, "http://prometheus") || strings.Contains(s, "opacity=") {
			t.Fatal("query/URL or translucent rendering leaked")
		}
	}
	c.Panels[0].Text = `<script>alert(1)</script> {{now "15:04 MST"}}`
	c.Panels[2].Title = `<img src="https://invalid.example">`
	b, err := RenderHTML(c, target, m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `<script>alert`) || strings.Contains(string(b), `<img`) || !strings.Contains(string(b), "12:00 JST") {
		t.Fatal("unsafe or nondeterministic text")
	}
	for _, pair := range [][2]float64{{0, 0}, {5, 5}, {-5, -5}, {-9, -1}, {-2, 5}, {-1e308, 1e308}} {
		pm := PanelModel{Series: []SeriesModel{{Points: []Point{{Value: pair[0], Top: pair[0], Present: true, StackKnown: true}, {Value: pair[1], Top: pair[1], Present: true, StackKnown: true}}, Segments: []Segment{{0, 2}}}}}
		svg := chartSVG(Panel{Type: "line"}, pm, "gray4", false, time.UTC)
		if strings.Contains(svg, "NaN") || strings.Contains(svg, "Inf") {
			t.Fatalf("invalid coordinates for %v", pair)
		}
	}
	c.Panels[0].Text = strings.Repeat("&", HTMLLimit)
	if _, err := RenderHTML(c, target, m); err == nil {
		t.Fatal("HTML limit")
	}
}

func TestValueSizeHTML(t *testing.T) {
	c := config(t)
	target := contextFixture(t)
	_, m, _ := syntheticModel(t, c, target)
	for _, size := range []string{"", "small", "medium", "large", "xlarge"} {
		c.Panels[1].Panels[0].Size = size
		class := size
		if size == "" {
			class = "auto-size"
		}
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("size=%s/missing=%t", size, missing), func(t *testing.T) {
				model := m
				model.Panels = append([]PanelModel(nil), m.Panels...)
				if missing {
					for i := range model.Panels {
						if model.Panels[i].ID == "power" {
							model.Panels[i].Value = nil
						}
					}
				}
				body, err := RenderHTML(c, target, model)
				if err != nil {
					t.Fatal(err)
				}
				want := `<div class="number ` + class + ` stale"><span class="ellipsis">123 W</span></div>`
				if missing {
					want = `<div class="number ` + class + `"><span class="ellipsis">—</span></div>`
				}
				if !strings.Contains(string(body), want) {
					t.Fatalf("missing value markup %s", want)
				}
			})
		}
	}
}

// Optional export supplies reproducible snapshots and completed HTML for manual
// real-capture/visual inspection. It never reads a live service or credentials.
func TestExportSyntheticExamples(t *testing.T) {
	out := os.Getenv("PROMETHEUS_EXAMPLE_OUTPUT")
	if out == "" {
		t.Skip("set PROMETHEUS_EXAMPLE_OUTPUT to export synthetic examples")
	}
	for _, name := range []string{"summary", "history", "daily"} {
		b, err := os.ReadFile(filepath.Join("examples", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		c, err := ParseConfig(strings.NewReader(string(b)), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, rotation := range []int{0, 90} {
			for _, palette := range []string{"gray4", "spectra6"} {
				target := contextFixture(t)
				target.Rotation = rotation
				target.Target.Rotation = rotation
				target.Target.Palette = palette
				plan, m, raw := syntheticModel(t, c, target)
				dir := filepath.Join(out, fmt.Sprintf("%s-%s-%d", name, palette, rotation))
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				manifest := ExpectedManifest(plan)
				for i, e := range manifest.Entries {
					if err := os.WriteFile(filepath.Join(dir, e.File), raw[plan.Requests[i].ID], 0644); err != nil {
						t.Fatal(err)
					}
				}
				mb, _ := json.MarshalIndent(manifest, "", "  ")
				cb, _ := json.MarshalIndent(target, "", "  ")
				cfg := b
				reparsed, err := ParseConfig(strings.NewReader(string(cfg)), nil)
				if err != nil {
					t.Fatal(err)
				}
				if hash(reparsed) != hash(c) {
					t.Fatal("exported configuration changed plan identity")
				}
				html, err := RenderHTML(c, target, m)
				if err != nil {
					t.Fatal(err)
				}
				for file, data := range map[string][]byte{"manifest.json": mb, "context.json": cb, "config.json": cfg, "dashboard.html": html} {
					if err := os.WriteFile(filepath.Join(dir, file), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				if _, _, err := LoadModel(context.Background(), c, plan, dir); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestChartMeaning(t *testing.T) {
	for _, bounds := range [][2]float64{{0, 70}, {-90, -10}, {-100, 120}, {5, 5.01}, {-1e308, 1e308}} {
		ticks := niceTicks(bounds[0], bounds[1])
		if len(ticks) < 3 || len(ticks) > 5 {
			t.Fatalf("ticks %v: %v", bounds, ticks)
		}
		if len(ticks) > 1 {
			step := ticks[1] - ticks[0]
			normalized := step / math.Pow(10, math.Floor(math.Log10(step)))
			if math.Abs(normalized-1) > 1e-6 && math.Abs(normalized-2) > 1e-6 && math.Abs(normalized-5) > 1e-6 && math.Abs(normalized-10) > 1e-6 {
				t.Fatalf("not 1/2/5: %g", step)
			}
		}
	}
	if got := displayNumber(-12345.125, 2, "$", " kWh"); got != "$-12,345.12 kWh" {
		t.Fatal(got)
	}
	for _, palette := range []string{"gray4", "spectra6"} {
		for _, dithered := range []bool{false, true} {
			styles := map[string]bool{}
			for i := 0; i < 8; i++ {
				k := seriesInk("primary", palette, dithered, i)
				key := fmt.Sprint(k, i%2, 5+i%3)
				if styles[key] {
					t.Fatal("indistinguishable series")
				}
				styles[key] = true
			}
			neg := seriesInk("negative", palette, dithered, 0)
			if palette == "gray4" && neg.fill != "white" {
				t.Fatal("negative bar must be outlined")
			}
		}
	}
	p := Panel{Type: "line", Decimals: 1, Prefix: "<", Suffix: "&"}
	m := PanelModel{Series: []SeriesModel{{Legend: "<unsafe>", Style: "primary", Points: []Point{{Time: 0, Top: 2, Present: true, StackKnown: true}, {Time: 60}, {Time: 120, Top: 3, Present: true, StackKnown: true}}, Segments: []Segment{{0, 1}, {2, 3}}}}, References: []ReferenceModel{{Value: 1, Label: "<limit>", Style: "secondary"}}}
	s := chartSVG(p, m, "gray4", false, time.UTC)
	if strings.Count(s, `class="segment"`) != 2 || strings.Count(s, `class="marker"`) != 2 || strings.Contains(s, "<unsafe>") || strings.Contains(s, "<limit>") {
		t.Fatal("gaps/isolated points/escaping")
	}
	m = PanelModel{Missing: true}
	s = chartSVG(p, m, "gray4", false, time.UTC)
	if !strings.Contains(s, "No data") {
		t.Fatal("all missing")
	}
	p.Type = "bar"
	p.Buckets = &BucketConfig{Unit: "day"}
	p.Stack = true
	m = PanelModel{Buckets: []Bucket{{Start: 0, End: 1, Partial: true}}, Series: []SeriesModel{{Legend: "Negative", Style: "negative", Points: []Point{{Value: -2, Base: 0, Top: -2, Present: true, StackKnown: true}}}, {Legend: "Unknown", Style: "positive", Points: []Point{{Value: 2, Base: 0, Top: 2, Present: true, StackKnown: false}}}}}
	s = chartSVG(p, m, "gray4", false, time.UTC)
	if strings.Count(s, `class="bar"`) != 1 || strings.Contains(s, `class="partial"`) || !strings.Contains(s, `01/01*</text>`) || !strings.Contains(s, `* partial`) || !strings.Contains(s, `fill="white"`) || !strings.Contains(s, `class="zero"`) {
		t.Fatal("negative/unknown/partial stack")
	}
	// A positive-only line must not introduce a visible zero baseline.
	p.Type = "line"
	p.Stack = false
	m.Buckets = nil
	m.Series = m.Series[:1]
	m.Series[0].Points[0].Top = 5
	if strings.Contains(chartSVG(p, m, "gray4", false, time.UTC), `class="zero"`) {
		t.Fatal("misleading zero baseline")
	}
}

func TestSpectraInks(t *testing.T) {
	styles := []string{"primary", "secondary", "tertiary", "positive", "negative", "muted"}
	solid := map[string]bool{"#0000ff": true, "#ff0000": true, "#008000": true, "#000000": true}
	for _, style := range styles {
		k := seriesInk(style, "spectra6", false, 0)
		if !solid[k.color] || k.text != k.color {
			t.Fatalf("undithered %s must use a native non-yellow ink: %+v", style, k)
		}
		if d := seriesInk(style, "spectra6", true, 0); d.color == "#ffff00" || d.color == "#aaaa00" {
			t.Fatalf("dithered %s must avoid yellow: %+v", style, d)
		}
	}
	if k := seriesInk("muted", "spectra6", false, 0); k.dash == "" || k.width != 2 {
		t.Fatalf("undithered muted must be thin and dotted: %+v", k)
	}
	if k := seriesInk("muted", "spectra6", true, 0); k.color != "#cc9900" || k.text != "#000" || k.width != 4 {
		t.Fatalf("dithered muted must use gold with black labels: %+v", k)
	}
	if seriesInk("tertiary", "spectra6", true, 0).color == seriesInk("positive", "spectra6", true, 0).color {
		t.Fatal("dithered tertiary must differ from positive")
	}

	series := make([]SeriesModel, 8)
	for i := range series {
		series[i].Style = "primary"
	}
	series[6].Style, series[7].Style = "positive", "positive"
	inks := seriesInks(series, "spectra6", true)
	want := []string{"#0000ff", "#ff8000", "#cc99ff", "#800080", "#cc9900", "#008000", "#008000", "#008000"}
	for i, k := range inks {
		if k.color != want[i] {
			t.Fatalf("series %d: got %s, want %s", i, k.color, want[i])
		}
	}
	for _, k := range seriesInks(series, "spectra6", false) {
		if spectraMixed[k.color] {
			t.Fatalf("undithered series must not use mixed inks: %+v", k)
		}
	}
	m := PanelModel{Series: []SeriesModel{
		{Legend: "a", Style: "primary", Points: []Point{{Time: 0, Top: 1, Present: true, StackKnown: true}, {Time: 60, Top: 2, Present: true, StackKnown: true}}, Segments: []Segment{{0, 2}}},
		{Legend: "b", Style: "primary", Points: []Point{{Time: 0, Top: 2, Present: true, StackKnown: true}, {Time: 60, Top: 3, Present: true, StackKnown: true}}, Segments: []Segment{{0, 2}}},
	}}
	svg := chartSVG(Panel{Type: "line"}, m, "spectra6", true, time.UTC)
	if !strings.Contains(svg, `stroke="#ff8000" stroke-width="4"`) || !strings.Contains(svg, `class="direct-label" fill="#000"`) {
		t.Fatal("dithered repeated series must use a spare hue with a legible label")
	}
}

func TestNativeInkGrays(t *testing.T) {
	c := config(t)
	target := contextFixture(t)
	_, m, _ := syntheticModel(t, c, target)
	for _, tc := range []struct {
		palette  string
		dithered bool
		native   bool
	}{{"gray4", false, false}, {"gray4", true, false}, {"spectra6", false, true}, {"spectra6", true, false}} {
		target.Target.Palette, c.Dithered = tc.palette, tc.dithered
		b, err := RenderHTML(c, target, m)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, `<body class="native-inks">`) != tc.native {
			t.Fatalf("%s dithered=%v: native-inks mismatch", tc.palette, tc.dithered)
		}
		if !strings.Contains(s, `class="gridline"`) {
			t.Fatal("missing gridlines")
		}
		// Undithered spectra6 converts light gray to white, hiding it.
		if tc.native && strings.Contains(s, `stroke="#aaa"`) {
			t.Fatalf("undithered spectra6 must not draw light gray gridlines")
		}
		if !tc.native && !strings.Contains(s, `stroke="#aaa"`) {
			t.Fatalf("%s dithered=%v: gridlines must stay light gray", tc.palette, tc.dithered)
		}
	}
	if !strings.Contains(displayCSS, ".native-inks .stale{color:#ff0000}") {
		t.Fatal("undithered spectra6 stale values must use a native ink")
	}
}
