package prometheus

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func modelInput(t *testing.T, panel Panel) (Config, Plan) {
	t.Helper()
	c := config(t)
	c.Panels = []Panel{panel}
	c.AllMissing = "display"
	c.Layout.Landscape = Grid{Areas: []string{panel.ID}, Columns: "1fr", Rows: "1fr"}
	c.Layout.Portrait = nil
	p, err := BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:34:56+09:00"))
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}

func normalized(t *testing.T, c Config, p Plan, raw map[string]json.RawMessage) (Model, error) {
	t.Helper()
	responses, err := ParseResponses(p, raw)
	if err != nil {
		return Model{}, err
	}
	return Normalize(c, p, responses)
}

func TestValueFreshnessAndFormatting(t *testing.T) {
	for _, tc := range []struct {
		name, value, fresh    string
		stale, missing, fails bool
	}{
		{"fresh", "-12.25", "now", false, false, false},
		{"boundary", "1", "boundary", false, false, false},
		{"old", "1", "old", true, false, false},
		{"missing freshness", "1", "empty", true, false, false},
		{"nonfinite freshness", "1", "NaN", true, false, false},
		{"infinite freshness", "1", "+Inf", true, false, false},
		{"future", "1", "future", false, false, true},
		{"negative freshness", "1", "-1", false, false, true},
		{"missing value", "empty", "now", false, true, false},
		{"nan value", "NaN", "now", false, true, false},
		{"infinite value", "-Inf", "now", false, true, false},
		{"zero", "-0", "now", false, false, false},
		{"multiple before filtering", "multiple", "now", false, false, true},
		{"multiple freshness", "1", "multiple", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panel := Panel{ID: "v", Type: "value", Query: "synthetic", StaleAfter: "60s", Decimals: 2, Prefix: "[", Suffix: "]", SignLabels: &SignLabels{Positive: "positive", Negative: "negative", Abs: true}}
			c, p := modelInput(t, panel)
			raw := map[string]json.RawMessage{}
			for _, r := range p.Requests {
				v := tc.value
				if r.Role == "freshness" {
					v = tc.fresh
				}
				switch v {
				case "now":
					v = seconds(p.Now)
				case "boundary":
					v = seconds(p.Now - 60)
				case "old":
					v = seconds(p.Now - 61)
				case "future":
					v = seconds(p.Now + 1)
				}
				switch v {
				case "empty":
					raw[r.ID] = apiResponse("vector", []any{})
				case "multiple":
					raw[r.ID] = apiResponse("vector", []any{vectorSample(map[string]string{"a": "1"}, p.Now, "1"), vectorSample(map[string]string{"a": "2"}, p.Now, "NaN")})
				default:
					raw[r.ID] = apiResponse("scalar", []any{r.Time, v})
				}
			}
			m, err := normalized(t, c, p, raw)
			if (err != nil) != tc.fails {
				t.Fatalf("err=%v", err)
			}
			if err != nil {
				return
			}
			pm := m.Panels[0]
			if pm.Missing != tc.missing {
				t.Fatal("missing state")
			}
			if tc.missing {
				if pm.Value != nil {
					t.Fatal("missing has value")
				}
				return
			}
			if pm.Value.Stale != tc.stale {
				t.Fatal("freshness state")
			}
			if tc.name == "fresh" && (pm.Value.Formatted != "[12.25]" || pm.Value.Label != "negative" || pm.Value.Number != -12.25) {
				t.Fatalf("value=%+v", pm.Value)
			}
			if tc.name == "zero" && (pm.Value.Formatted != "[0.00]" || pm.Value.Label != "positive" || math.Signbit(pm.Value.Number)) {
				t.Fatal("negative zero")
			}
		})
	}
	for _, tc := range []struct {
		v      float64
		digits int
		want   string
	}{{-0.001, 2, "0.00"}, {-12.25, 1, "-12.2"}, {1234.5, 2, "1234.50"}, {1, 0, "1"}} {
		if got := FormatNumber(tc.v, tc.digits, "", ""); got != tc.want {
			t.Fatalf("format=%s want=%s", got, tc.want)
		}
	}
}

func TestLineGapsLegendsReferencesAndStableOrder(t *testing.T) {
	constant := 12.0
	panel := Panel{ID: "l", Type: "line", Range: "20s", MinStep: "5s", Series: []Series{{Query: "a", Legend: "{{name}}", Style: "primary"}, {Query: "b", Legend: "{{missing}}", Style: "secondary"}}, ReferenceLines: []Reference{{Value: &constant, Label: "constant"}, {Query: "ref"}, {Query: "absent"}}}
	c, p := modelInput(t, panel)
	raw := map[string]json.RawMessage{}
	for _, r := range p.Requests {
		if r.Role == "reference" {
			if r.Series == 1 {
				raw[r.ID] = apiResponse("scalar", []any{p.Now, "3"})
			} else {
				raw[r.ID] = apiResponse("vector", []any{})
			}
			continue
		}
		values := []any{[]any{r.Start, "1"}, []any{r.Start + r.Step, "NaN"}, []any{r.Start + 3*r.Step, "4"}, []any{r.Start + 4*r.Step, "5"}}
		labels := []map[string]string{{"name": "same", "instance": "b"}, {"name": "same", "instance": "a"}}
		if r.Series == 1 {
			labels = []map[string]string{{"instance": "c"}}
		}
		items := []any{}
		for _, label := range labels {
			items = append(items, map[string]any{"metric": label, "values": values})
		}
		raw[r.ID] = apiResponse("matrix", items)
	}
	m, err := normalized(t, c, p, raw)
	if err != nil {
		t.Fatal(err)
	}
	pm := m.Panels[0]
	if len(pm.Series) != 3 || len(pm.References) != 2 || pm.References[0].Value != 12 || pm.References[1].Value != 3 {
		t.Fatalf("model=%+v", pm)
	}
	if pm.Series[0].Legend != "same (1)" || pm.Series[1].Legend != "same (2)" || pm.Series[2].Legend != "?" || pm.Series[0].Labels["instance"] != "a" {
		t.Fatalf("legends=%+v", pm.Series)
	}
	for _, s := range pm.Series {
		if len(s.Points) != 5 || s.Points[1].Present || s.Points[2].Present || !reflect.DeepEqual(s.Segments, []Segment{{0, 1}, {3, 5}}) {
			t.Fatalf("gaps=%+v", s)
		}
	}
	// Reversing API series order must not change the model or legend suffixes.
	for _, r := range p.Requests {
		if r.Role == "range" && r.Series == 0 {
			var envelope map[string]any
			_ = json.Unmarshal(raw[r.ID], &envelope)
			items := envelope["data"].(map[string]any)["result"].([]any)
			items[0], items[1] = items[1], items[0]
			raw[r.ID], _ = json.Marshal(envelope)
		}
	}
	other, err := normalized(t, c, p, raw)
	if err != nil || !reflect.DeepEqual(m, other) {
		t.Fatal("nondeterministic ordering")
	}
	b, _ := json.Marshal(m)
	if strings.Contains(string(b), "absent") {
		t.Fatal("query in model")
	}
}

func TestBarLabelChangesStackAndSeriesLimit(t *testing.T) {
	panel := Panel{ID: "b", Type: "bar", Buckets: &BucketConfig{Unit: "day", Count: 3, IncludeCurrent: true}, Stack: true, Series: []Series{{Query: "a", Style: "primary"}, {Query: "b", Style: "negative"}}}
	c, p := modelInput(t, panel)
	raw := map[string]json.RawMessage{}
	for _, r := range p.Requests {
		items := []any{}
		if r.Bucket != 1 || r.Series != 0 {
			value := "2"
			if r.Series == 1 {
				value = "-3"
			}
			items = append(items, vectorSample(map[string]string{"label": "same"}, r.Time, value))
		}
		raw[r.ID] = apiResponse("vector", items)
	}
	m, err := normalized(t, c, p, raw)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Panels[0].Series
	if len(s) != 2 || s[0].Points[0].Base != 0 || s[0].Points[0].Top != 2 || s[1].Points[0].Base != 0 || s[1].Points[0].Top != -3 {
		t.Fatalf("positive/negative stack=%+v", s)
	}
	if s[0].Points[1].Present || !s[1].Points[1].Present || s[1].Points[1].StackKnown {
		t.Fatal("missing contribution treated as zero")
	}
	if !m.Panels[0].Buckets[2].Partial {
		t.Fatal("partial metadata lost")
	}
	for _, r := range p.Requests {
		items := []any{}
		for i := 0; i < 2; i++ {
			items = append(items, vectorSample(map[string]string{"label": r.ID + seconds(int64(i))}, r.Time, "NaN"))
		}
		raw[r.ID] = apiResponse("vector", items)
	}
	if _, err := normalized(t, c, p, raw); err == nil || !strings.Contains(err.Error(), "series_limit") {
		t.Fatalf("label union limit: %v", err)
	}

	series := []SeriesModel{{Points: []Point{{Value: 2, Present: true}}}, {Points: []Point{{Value: 3, Present: true}}}, {Points: []Point{{Value: -4, Present: true}}}, {Points: []Point{{Value: -1, Present: true}}}}
	if err := stackPoints(series, true); err != nil {
		t.Fatal(err)
	}
	if series[1].Points[0].Base != 2 || series[1].Points[0].Top != 5 || series[3].Points[0].Base != -4 || series[3].Points[0].Top != -5 {
		t.Fatal("stack accumulators")
	}
	series[0].Points[0].Value = math.MaxFloat64
	series[1].Points[0].Value = math.MaxFloat64
	if err := stackPoints(series, true); err == nil {
		t.Fatal("stack overflow accepted")
	}
}

func TestAllMissingPolicy(t *testing.T) {
	constant := 1.0
	c, p := modelInput(t, Panel{ID: "l", Type: "line", Range: "10s", MinStep: "1s", Series: []Series{{Query: "a"}}, ReferenceLines: []Reference{{Value: &constant}}})
	raw := map[string]json.RawMessage{}
	for _, r := range p.Requests {
		raw[r.ID] = apiResponse("matrix", []any{})
	}
	c.AllMissing = "fail"
	if m, err := normalized(t, c, p, raw); err == nil || len(m.Panels) != 0 {
		t.Fatal("references made missing screen available")
	}
	c.AllMissing = "display"
	if m, err := normalized(t, c, p, raw); err != nil || !m.Panels[0].Missing {
		t.Fatal("display policy")
	}
	c, p = modelInput(t, Panel{ID: "text", Type: "text", Text: "synthetic"})
	c.AllMissing = "fail"
	if _, err := normalized(t, c, p, map[string]json.RawMessage{}); err != nil {
		t.Fatal("text-only screen failed")
	}
}

func TestDefaultAndCollidingLegends(t *testing.T) {
	series := []SeriesModel{{QueryIndex: 0, Labels: map[string]string{"common": "x", "host": "a"}}, {QueryIndex: 0, Labels: map[string]string{"common": "x", "host": "b"}}}
	setLegends(series, []Series{{}})
	if series[0].Legend != "host=a" || series[1].Legend != "host=b" {
		t.Fatal("default differing labels")
	}
	series = []SeriesModel{{QueryIndex: 0}, {QueryIndex: 1}, {QueryIndex: 2}}
	setLegends(series, []Series{{Legend: "same"}, {Legend: "same"}, {Legend: "same (1)"}})
	if series[0].Legend != "same (2)" || series[1].Legend != "same (3)" || series[2].Legend != "same (1)" {
		t.Fatal("legend collision")
	}
}
