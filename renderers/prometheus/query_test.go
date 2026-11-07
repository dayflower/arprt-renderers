package prometheus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func instant(t *testing.T, s string) time.Time {
	t.Helper()
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func plan(t *testing.T) Plan {
	t.Helper()
	p, e := BuildPlan(config(t), contextFixture(t), instant(t, "2026-11-07T12:34:56+09:00"))
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestCalendarBuckets(t *testing.T) {
	for _, tc := range []struct {
		name, zone, now string
		want            int64
	}{
		{"Tokyo", "Asia/Tokyo", "2026-01-01T15:00:00+09:00", 86400},
		{"spring", "America/New_York", "2026-03-09T12:00:00-04:00", 23 * 3600},
		{"fall", "America/New_York", "2026-11-02T12:00:00-05:00", 25 * 3600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, e := time.LoadLocation(tc.zone)
			if e != nil {
				t.Fatal(e)
			}
			now := instant(t, tc.now).In(loc)
			b, e := Buckets(now, BucketConfig{"day", 2, false})
			if e != nil {
				t.Fatal(e)
			}
			last := b[1]
			if last.End-last.Start != tc.want || last.Partial || time.Unix(last.Start, 0).In(loc).Hour() != 0 {
				t.Fatal(b)
			}
			if b[0].BoundaryEnd != b[1].Start {
				t.Fatal("nonadjacent buckets")
			}
		})
	}
	loc, _ := time.LoadLocation("America/New_York")
	for _, tc := range []struct {
		now    string
		labels []string
	}{
		{"2026-03-08T04:30:00-04:00", []string{"03-08 00:00 -0500", "03-08 01:00 -0500", "03-08 03:00 -0400", "03-08 04:00 -0400"}},
		{"2026-11-01T02:30:00-05:00", []string{"11-01 00:00 -0400", "11-01 01:00 -0400", "11-01 01:00 -0500", "11-01 02:00 -0500"}},
	} {
		b, e := Buckets(instant(t, tc.now).In(loc), BucketConfig{"hour", 4, true})
		if e != nil {
			t.Fatal(e)
		}
		for i, x := range b {
			if x.Label != tc.labels[i] || x.BoundaryEnd-x.Start != 3600 {
				t.Fatal(b)
			}
			if i == 3 && (!x.Partial || x.End-x.Start != 1800) {
				t.Fatal(x)
			}
		}
	}
	loc, _ = time.LoadLocation("Asia/Tokyo")
	b, e := Buckets(instant(t, "2027-01-01T00:00:00+09:00").In(loc), BucketConfig{"day", 2, true})
	if e != nil || b[0].Label != "2026-12-31" || b[1].End != b[1].Start || !b[1].Partial {
		t.Fatal(b, e)
	}
}

func TestCalendarBoundaryExceptions(t *testing.T) {
	loc, e := time.LoadLocation("Australia/Lord_Howe")
	if e != nil {
		t.Fatal(e)
	}
	b, e := Buckets(instant(t, "2026-10-04T03:15:00+11:00").In(loc), BucketConfig{"hour", 2, true})
	if e != nil || !strings.Contains(b[1].Label, "02:30") || b[1].End-b[1].Start != 45*60 {
		t.Fatal(b, e)
	}
	loc, e = time.LoadLocation("Pacific/Apia")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Buckets(instant(t, "2011-12-31T12:00:00+14:00").In(loc), BucketConfig{"day", 2, true}); e == nil {
		t.Fatal("skipped calendar date accepted")
	}
	loc, e = time.LoadLocation("America/Sao_Paulo")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Buckets(instant(t, "2018-11-04T12:00:00-02:00").In(loc), BucketConfig{"day", 1, true}); e == nil {
		t.Fatal("nonexistent midnight accepted")
	}
}

func TestPlanDeterminismAndSteps(t *testing.T) {
	p := plan(t)
	if !reflect.DeepEqual(p, plan(t)) {
		t.Fatal("nondeterministic plan")
	}
	if len(p.Requests) != 10 || p.Requests[0].Role != "value" || p.Requests[1].Role != "freshness" || p.Requests[2].Role != "range" {
		t.Fatal(p.Requests)
	}
	for _, r := range p.Requests {
		if strings.Contains(r.Query, "$") {
			t.Fatal(r)
		}
		if r.Role == "range" {
			if (r.End-r.Start)/r.Step+1 > 500 || r.End != p.Now || r.Start != p.Now-86400 {
				t.Fatal(r)
			}
			scrape := int64(60)
			want := durationSeconds(max(4*scrape, r.Step+scrape))
			if !strings.Contains(r.Query, "["+want+"]") {
				t.Fatal(r)
			}
		} else if r.Role != "bucket" && r.Time != p.Now {
			t.Fatal(r)
		}
	}
	bs := p.Buckets["daily"]
	if len(bs) != 7 || !bs[6].Partial || bs[6].End != p.Now || bs[6].End-bs[6].Start != 12*3600+34*60+56 {
		t.Fatal(bs)
	}
	for _, r := range p.Requests {
		if r.Role == "bucket" {
			b := bs[r.Bucket]
			if r.Time != b.End || !strings.Contains(r.Query, "["+durationSeconds(b.End-b.Start)+"]") {
				t.Fatal(r)
			}
		}
	}
	c := config(t)
	c.Panels[2].MinStep = "1h"
	p, e := BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:00:00Z"))
	if e != nil || p.Requests[2].Step != 3600 {
		t.Fatal(p, e)
	}
}
func TestMidnightAndVariableBoundaries(t *testing.T) {
	vars := map[string]string{"$__now": "123", "$__range": "60s"}
	if _, e := expand("up @ $__now_extra", vars); e == nil {
		t.Fatal("partial variable expansion")
	}
	for _, s := range []string{"$__step_extra", "${__now}", "$bad", "$__bucket_start"} {
		if e := validateVariables(s, "line"); e == nil {
			t.Fatal(s)
		}
	}
	c := config(t)
	ctx := contextFixture(t)
	now := instant(t, "2026-11-07T00:00:00+09:00")
	if _, e := BuildPlan(c, ctx, now); e == nil {
		t.Fatal("zero bucket range accepted")
	}
	c.Panels[3].Buckets.IncludeCurrent = false
	if _, e := BuildPlan(c, ctx, now); e != nil {
		t.Fatal(e)
	}
	c.Panels[1].Panels[0].StaleAfter = ""
	c.Panels[1].Panels[0].Query = "increase(example_total[$__today_range])"
	if _, e := BuildPlan(c, ctx, now); e == nil {
		t.Fatal("zero today range accepted")
	}
	c.Panels[1].Panels[0].Query = "example_total @ $__today_start"
	p, e := BuildPlan(c, ctx, now)
	if e != nil || !strings.Contains(p.Requests[0].Query, seconds(now.Unix())) {
		t.Fatal(p, e)
	}
}
func TestRequestBudget(t *testing.T) {
	c := config(t)
	c.Panels[3].Buckets.Count = 31
	c.Panels[3].Series = make([]Series, 4)
	for i := range c.Panels[3].Series {
		c.Panels[3].Series[i] = Series{Query: "up", Style: "primary"}
	}
	p, e := BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:00:00Z"))
	if e != nil || len(p.Requests) != 127 {
		t.Fatal(len(p.Requests), e)
	}
	c.Panels[2].ReferenceLines = []Reference{{Query: "up"}}
	if _, e = BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:00:00Z")); e != nil {
		t.Fatal(e)
	}
	c.Panels[2].ReferenceLines = append(c.Panels[2].ReferenceLines, Reference{Query: "up"})
	if _, e = BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:00:00Z")); e == nil {
		t.Fatal("request budget not enforced")
	}
}
func TestContextAndClock(t *testing.T) {
	b, e := os.ReadFile("fixtures/render-context-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, rotation := range []int{0, 90, 180, 270} {
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["renderTarget"].(map[string]any)["rotation"] = rotation
		raw, _ := json.Marshal(m)
		c, e := ParseContext(raw)
		if e != nil {
			t.Fatal(e)
		}
		w, h := c.LogicalSize()
		if rotation%180 == 0 && (w != 800 || h != 480) || rotation%180 != 0 && (w != 480 || h != 800) {
			t.Fatal(w, h)
		}
	}
	for _, bad := range []string{`"rotation": 0`, `"width": 800`, `"palette": "gray4"`} {
		replacement := `"rotation": 45`
		if strings.Contains(bad, "width") {
			replacement = `"width": 480`
		}
		if strings.Contains(bad, "palette") {
			replacement = `"palette": "gray16"`
		}
		if _, e := ParseContext([]byte(strings.Replace(string(b), bad, replacement, 1))); e == nil {
			t.Fatal(bad)
		}
	}
	count := 0
	clock := func() time.Time { count++; return time.Unix(1794010000, 123) }
	o, e := ParseOptions([]string{"--config", "example.json", "--prometheus-url", "http://prometheus.example", "--input", "synthetic"}, clock)
	if e != nil || count != 1 || o.Now.Nanosecond() != 0 || o.PrometheusURL == nil {
		t.Fatal(o, e, count)
	}
	o, e = ParseOptions([]string{"--config", "example.json", "--now", "2026-11-07T12:00:00Z"}, clock)
	if e != nil || count != 1 || o.Context != "/arprt/context.json" || o.Output != "/arprt/output.png" {
		t.Fatal(o, e, count)
	}
	for _, args := range [][]string{{"--config", "x", "--now", "2026-11-07T12:00:00.5Z"}, {"--config", "x", "extra"}, {"--config", "x", "--now", "bad"}, {"--unknown"}} {
		if _, e := ParseOptions(args, clock); e == nil {
			t.Fatal(args)
		}
	}
}

func writeSnapshot(t *testing.T, dir string, p Plan, m Manifest) {
	t.Helper()
	b, _ := json.Marshal(m)
	if e := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	for _, entry := range ExpectedManifest(p).Entries {
		if e := os.WriteFile(filepath.Join(dir, entry.File), []byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSnapshots(t *testing.T) {
	p := plan(t)
	m := ExpectedManifest(p)
	dir := t.TempDir()
	writeSnapshot(t, dir, p, m)
	r, e := LoadSnapshot(dir, p)
	if e != nil || len(r) != len(p.Requests) {
		t.Fatal(len(r), e)
	}
	for _, tc := range []string{"version", "hash", "missing", "request", "escape", "duplicate", "unknown", "large", "symlink"} {
		t.Run(tc, func(t *testing.T) {
			m := ExpectedManifest(p)
			dir := t.TempDir()
			writeSnapshot(t, dir, p, m)
			switch tc {
			case "version":
				m.Version = 2
			case "hash":
				m.PlanHash = "wrong"
			case "missing":
				m.Entries = m.Entries[1:]
			case "request":
				m.Entries[0].Request.Time++
			case "escape":
				m.Entries[0].File = "../response-000.json"
			case "duplicate":
				m.Entries[1].File = m.Entries[0].File
			case "unknown":
				b, _ := json.Marshal(m)
				b = append([]byte(`{"unknown":true,`), b[1:]...)
				_ = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600)
			case "large":
				_ = os.WriteFile(filepath.Join(dir, m.Entries[0].File), make([]byte, ResponseLimit+1), 0600)
			case "symlink":
				_ = os.Remove(filepath.Join(dir, m.Entries[0].File))
				_ = os.Symlink(filepath.Join(t.TempDir(), "outside.json"), filepath.Join(dir, m.Entries[0].File))
			}
			if tc != "unknown" && tc != "large" && tc != "symlink" {
				b, _ := json.Marshal(m)
				_ = os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600)
			}
			if _, e := LoadSnapshot(dir, p); e == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
	p2 := p
	p2.Now++
	if _, e := LoadSnapshot(dir, p2); e == nil {
		t.Fatal("time mismatch accepted")
	}
	// Nine individually valid files exceed the cumulative decompressed-byte budget.
	for _, entry := range m.Entries {
		if e := os.WriteFile(filepath.Join(dir, entry.File), make([]byte, ResponseLimit), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := LoadSnapshot(dir, p); e == nil {
		t.Fatal("total size limit")
	}
}

func TestPlotWidths(t *testing.T) {
	c := config(t)
	ctx := contextFixture(t)
	w := PlotWidths(c, ctx)
	if w["history"] != 432 || w["power"] != 168 {
		t.Fatal(w)
	}
	c.Layout.Landscape.Columns = "auto 1fr"
	if w = PlotWidths(c, ctx); w["history"] != 1 || w["power"] != 1 {
		t.Fatal(w)
	}
	c = config(t)
	c.Panels[1].Wrap = true
	if w = PlotWidths(c, ctx); w["power"] != 1 {
		t.Fatal(w)
	}
	ctx.Rotation = 90
	if w = PlotWidths(c, ctx); w["history"] != 384 {
		t.Fatal(w)
	}
}
