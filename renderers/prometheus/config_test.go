package prometheus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) map[string]any {
	t.Helper()
	b, e := os.ReadFile("fixtures/config.json")
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func parseMap(t *testing.T, m map[string]any) (Config, error) {
	t.Helper()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	checkSchemaOracle(t, b)
	return ParseConfig(bytes.NewReader(b), nil)
}

func checkSchemaOracle(t *testing.T, b []byte) {
	t.Helper()
	python := os.Getenv("PROMETHEUS_SCHEMA_PYTHON")
	if python == "" {
		return
	}
	var s map[string]any
	_ = json.Unmarshal(schemaJSON, &s)
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := d.Decode(&v); e != nil {
		t.Fatal(e)
	}
	want := validateSchema(s, v, s) == nil
	envelope, _ := json.Marshal(map[string]any{"schema": json.RawMessage(schemaJSON), "document": json.RawMessage(b)})
	cmd := exec.Command(python, "-c", `import json,sys,jsonschema
v=json.load(sys.stdin)
jsonschema.Draft202012Validator.check_schema(v['schema'])
print(jsonschema.Draft202012Validator(v['schema']).is_valid(v['document']))`)
	cmd.Stdin = bytes.NewReader(envelope)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("independent schema validator failed: %s", out)
	}
	if (strings.TrimSpace(string(out)) == "True") != want {
		t.Fatal("embedded and independent schema validators disagree")
	}
}
func panel(m map[string]any, i int) map[string]any { return m["panels"].([]any)[i].(map[string]any) }
func grid(m map[string]any) map[string]any {
	return m["layout"].(map[string]any)["landscape"].(map[string]any)
}
func config(t *testing.T) Config {
	t.Helper()
	c, e := parseMap(t, fixture(t))
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func contextFixture(t *testing.T) Context {
	t.Helper()
	b, e := os.ReadFile("fixtures/render-context-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	c, e := ParseContext(b)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestConfigDefaultsAndPortrait(t *testing.T) {
	c := config(t)
	if c.Timezone != "Asia/Tokyo" || c.Prometheus.ScrapeInterval != "60s" || c.AllMissing != "fail" || c.Panels[2].MinStep != "1s" || c.Panels[1].Panels[0].Grow != 1 || !c.Panels[3].Buckets.IncludeCurrent {
		t.Fatalf("missing defaults: %+v", c)
	}
	if !reflect.DeepEqual(c.Layout.Portrait.Areas, []string{"heading", "current", "history", "daily"}) || c.Layout.Portrait.Rows != "auto auto auto 1fr" {
		t.Fatal(c.Layout.Portrait)
	}
	// Parsing must produce the same normalized configuration, independent of map iteration.
	if !reflect.DeepEqual(c, config(t)) {
		t.Fatal("nondeterministic defaults")
	}
}

func TestValueSizeConfig(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		for _, size := range []string{"", "small", "medium", "large", "xlarge"} {
			t.Run(fmt.Sprintf("grouped=%t/size=%s", grouped, size), func(t *testing.T) {
				m := fixture(t)
				p := panel(m, 1)["panels"].([]any)[0].(map[string]any)
				if !grouped {
					p["id"] = "current"
					m["panels"].([]any)[1] = p
				}
				if size != "" {
					p["size"] = size
				}
				c, err := parseMap(t, m)
				if err != nil {
					t.Fatal(err)
				}
				value := c.Panels[1]
				if grouped {
					value = value.Panels[0]
				}
				if value.Size != size {
					t.Fatalf("size=%q, want %q", value.Size, size)
				}
			})
		}
		for _, size := range []any{"", "auto", "64px", 64, nil} {
			t.Run(fmt.Sprintf("grouped=%t/invalid=%v", grouped, size), func(t *testing.T) {
				m := fixture(t)
				p := panel(m, 1)["panels"].([]any)[0].(map[string]any)
				if !grouped {
					p["id"] = "current"
					m["panels"].([]any)[1] = p
				}
				p["size"] = size
				if _, err := parseMap(t, m); err == nil {
					t.Fatal("accepted invalid value size")
				}
			})
		}
	}
}

func TestRejectConfig(t *testing.T) {
	cases := map[string]func(map[string]any){
		"version":         func(m map[string]any) { m["version"] = 2 },
		"string version":  func(m map[string]any) { m["version"] = "1" },
		"unknown":         func(m map[string]any) { m["css"] = "color:red" },
		"wrong attribute": func(m map[string]any) { panel(m, 0)["query"] = "up" },
		"top grow":        func(m map[string]any) { panel(m, 0)["grow"] = 1 },
		"bad id":          func(m map[string]any) { panel(m, 0)["id"] = "a;display:none" },
		"duplicate id":    func(m map[string]any) { panel(m, 1)["panels"].([]any)[0].(map[string]any)["id"] = "daily" },
		"empty group":     func(m map[string]any) { panel(m, 1)["panels"] = []any{} },
		"nested group": func(m map[string]any) {
			panel(m, 1)["panels"] = []any{map[string]any{"id": "nested", "type": "group", "panels": []any{}}}
		},
		"bad grow":        func(m map[string]any) { panel(m, 1)["panels"].([]any)[0].(map[string]any)["grow"] = 0 },
		"group children":  func(m map[string]any) { p := panel(m, 1); p["panels"] = make([]any, 13) },
		"bad track":       func(m map[string]any) { grid(m)["columns"] = "repeat(2,1fr)" },
		"track injection": func(m map[string]any) { grid(m)["columns"] = "1fr 1fr; color:red" },
		"track count":     func(m map[string]any) { grid(m)["columns"] = "1fr" },
		"zero track":      func(m map[string]any) { grid(m)["columns"] = "0fr 1fr" },
		"negative track":  func(m map[string]any) { grid(m)["columns"] = "-1px 1fr" },
		"large track":     func(m map[string]any) { grid(m)["columns"] = "801px 1fr" },
		"ragged":          func(m map[string]any) { grid(m)["areas"] = []string{"heading", "current history", "daily history"} },
		"L shaped": func(m map[string]any) {
			grid(m)["areas"] = []string{"heading heading", "heading history", "current daily"}
		},
		"unknown area": func(m map[string]any) {
			grid(m)["areas"] = []string{"heading heading", "current history", "daily missing"}
		},
		"unused panel": func(m map[string]any) {
			grid(m)["areas"] = []string{"heading heading", "current history", "current history"}
		},
		"invalid portrait": func(m map[string]any) {
			m["layout"].(map[string]any)["portrait"] = map[string]any{"areas": []string{"heading"}}
		},
		"gap":             func(m map[string]any) { grid(m)["gap"] = 33 },
		"unicode title":   func(m map[string]any) { panel(m, 2)["title"] = strings.Repeat("日", 65) },
		"text length":     func(m map[string]any) { panel(m, 0)["text"] = strings.Repeat("日", 513) },
		"text expression": func(m map[string]any) { panel(m, 0)["text"] = "{{query \"up\"}}" },
		"text format":     func(m map[string]any) { panel(m, 0)["format"] = "markdown" },
		"text size":       func(m map[string]any) { panel(m, 0)["size"] = "10px" },
		"line size":       func(m map[string]any) { panel(m, 2)["size"] = "large" },
		"bar size":        func(m map[string]any) { panel(m, 3)["size"] = "large" },
		"group size":      func(m map[string]any) { panel(m, 1)["size"] = "large" },
		"bad zone":        func(m map[string]any) { m["timezone"] = "Local" },
		"unknown zone":    func(m map[string]any) { m["timezone"] = "Nowhere/Example" },
		"duration":        func(m map[string]any) { panel(m, 2)["range"] = "1d" },
		"zero duration":   func(m map[string]any) { panel(m, 2)["range"] = "0s" },
		"large duration":  func(m map[string]any) { panel(m, 2)["range"] = "8785h" },
		"axis":            func(m map[string]any) { panel(m, 2)["yAxis"] = map[string]any{"min": 10, "max": 0} },
		"axis bound":      func(m map[string]any) { panel(m, 2)["yAxis"] = map[string]any{"min": 1e101} },
		"decimals":        func(m map[string]any) { panel(m, 2)["decimals"] = 7 },
		"prefix":          func(m map[string]any) { panel(m, 2)["prefix"] = strings.Repeat("字", 33) },
		"reference xor": func(m map[string]any) {
			panel(m, 2)["referenceLines"] = []any{map[string]any{"value": 1, "query": "up"}}
		},
		"reference missing": func(m map[string]any) { panel(m, 2)["referenceLines"] = []any{map[string]any{"label": "x"}} },
		"style":             func(m map[string]any) { panel(m, 2)["series"].([]any)[0].(map[string]any)["style"] = "red" },
		"legend expression": func(m map[string]any) {
			panel(m, 2)["series"].([]any)[0].(map[string]any)["legend"] = "{{now \"15:04\"}}"
		},
		"count": func(m map[string]any) { panel(m, 3)["buckets"].(map[string]any)["count"] = 32 },
		"aggregate freshness": func(m map[string]any) {
			panel(m, 1)["panels"].([]any)[0].(map[string]any)["query"] = "max(example_power_watts)"
		},
		"arithmetic freshness": func(m map[string]any) {
			panel(m, 1)["panels"].([]any)[0].(map[string]any)["query"] = "example_power_watts * 2"
		},
		"unknown variable": func(m map[string]any) {
			panel(m, 2)["series"].([]any)[0].(map[string]any)["query"] = "up @ $__now_extra"
		},
		"out of scope": func(m map[string]any) { panel(m, 3)["series"].([]any)[0].(map[string]any)["query"] = "up[$__range]" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := fixture(t)
			mutate(m)
			if _, e := parseMap(t, m); e == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}
func TestInputBoundaries(t *testing.T) {
	for _, s := range []string{`{"version":1,"version":1}`, `null`, `{"version":NaN}`, `{} {}`, `{"x":null}`, strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65), string([]byte{0xff})} {
		if _, e := decodeJSON([]byte(s)); e == nil {
			t.Errorf("accepted invalid JSON %q", s)
		}
	}
	if _, e := ParseConfig(strings.NewReader(strings.Repeat(" ", ConfigLimit+1)), nil); e == nil {
		t.Fatal("size limit")
	}
	m := fixture(t)
	panel(m, 0)["text"] = strings.Repeat("日", 512)
	panel(m, 2)["title"] = strings.Repeat("日", 64)
	if _, e := parseMap(t, m); e != nil {
		t.Fatal(e)
	}
}

func TestExactIntegerSyntax(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want bool
	}{{"1", true}, {"1.0", true}, {"10e-1", true}, {"10.0e-1", true}, {"1e10", true}, {"-10.00", true}, {"1.00000000000000000001", false}, {"1e-1000000000", false}, {"0e9999999999999999999", true}, {"1.01e1", false}} {
		if exactInteger(json.Number(tc.s)) != tc.want {
			t.Fatal(tc)
		}
	}
	b, _ := os.ReadFile("fixtures/config.json")
	for _, literal := range []string{"1.00000000000000000001", "1e999999999"} {
		input := bytes.Replace(b, []byte(`"version": 1`), []byte(`"version": `+literal), 1)
		if _, e := ParseConfig(bytes.NewReader(input), nil); e == nil {
			t.Fatal("inexact/oversized version accepted")
		}
	}
}

func TestResourceLimits(t *testing.T) {
	for _, tc := range []struct {
		top, groups, children int
		valid                 bool
	}{{24, 0, 0, true}, {25, 0, 0, false}, {4, 4, 11, true}, {4, 4, 12, false}} {
		m := map[string]any{"version": 1, "prometheus": map[string]any{"url": "http://prometheus.example"}}
		ps := []any{}
		areas := []string{}
		row := []string{}
		for i := 0; i < tc.top; i++ {
			id := fmt.Sprintf("panel-%d", i)
			p := map[string]any{"id": id, "type": "text", "text": "Example"}
			if i < tc.groups {
				p = map[string]any{"id": id, "type": "group"}
				children := []any{}
				for j := 0; j < tc.children; j++ {
					children = append(children, map[string]any{"id": fmt.Sprintf("child-%d-%d", i, j), "type": "text", "text": "Example"})
				}
				p["panels"] = children
			}
			ps = append(ps, p)
			row = append(row, id)
			if len(row) == 5 || i == tc.top-1 {
				areas = append(areas, strings.Join(row, " "))
				row = nil
			}
		}
		// Pad the final row with empty cells to keep structural validity.
		if len(areas) > 1 {
			for len(strings.Fields(areas[len(areas)-1])) < 5 {
				areas[len(areas)-1] += " ."
			}
		}
		m["panels"] = ps
		m["layout"] = map[string]any{"landscape": map[string]any{"areas": areas}}
		_, e := parseMap(t, m)
		if (e == nil) != tc.valid {
			t.Fatalf("limits %+v: %v", tc, e)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"rows", func(m map[string]any) { grid(m)["areas"] = make([]string, 33) }},
		{"columns", func(m map[string]any) {
			grid(m)["areas"] = []string{strings.TrimSpace(strings.Repeat("heading ", 17))}
			delete(grid(m), "columns")
			delete(grid(m), "rows")
		}},
		{"line queries", func(m map[string]any) {
			s := []any{}
			for i := 0; i < 9; i++ {
				s = append(s, map[string]any{"query": "up"})
			}
			panel(m, 2)["series"] = s
		}},
		{"bar queries", func(m map[string]any) {
			s := []any{}
			for i := 0; i < 5; i++ {
				s = append(s, map[string]any{"query": "up"})
			}
			panel(m, 3)["series"] = s
		}},
		{"references", func(m map[string]any) {
			s := []any{}
			for i := 0; i < 9; i++ {
				s = append(s, map[string]any{"value": i})
			}
			panel(m, 2)["referenceLines"] = s
		}},
		{"query length", func(m map[string]any) {
			panel(m, 2)["series"].([]any)[0].(map[string]any)["query"] = strings.Repeat("x", 8193)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fixture(t)
			tc.mutate(m)
			if _, e := parseMap(t, m); e == nil {
				t.Fatal("limit not enforced")
			}
		})
	}
	// JSON Schema integers include integral decimal/exponent representations.
	b, _ := os.ReadFile("fixtures/config.json")
	b = bytes.Replace(b, []byte(`"version": 1`), []byte(`"version": 1e0`), 1)
	checkSchemaOracle(t, b)
	if _, e := ParseConfig(bytes.NewReader(b), nil); e != nil {
		t.Fatal(e)
	}
}
func TestURLsAndOverride(t *testing.T) {
	for _, s := range []string{"", "file:///tmp/input", "http://user:pass@example.test", "http://example.test/?", "http://example.test/#", "http://example.test/?x=1", "http://example.test/../private", "http://example.test/%2e%2e/private", "http://example.test/a%2fb", "http://example.test/a%00b", "http://example.test/a//b"} {
		if _, e := BaseURL(s); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
	u, e := APIURL("https://prometheus.example/prefix/", "/api/v1/query")
	if e != nil || u != "https://prometheus.example/prefix/api/v1/query" {
		t.Fatal(u, e)
	}
	m := fixture(t)
	m["prometheus"].(map[string]any)["url"] = "invalid"
	b, _ := json.Marshal(m)
	override := "http://prometheus.example/base"
	c, e := ParseConfig(bytes.NewReader(b), &override)
	if e != nil || c.Prometheus.URL != override {
		t.Fatal(c, e)
	}
	override = ""
	if _, e = ParseConfig(bytes.NewReader(b), &override); e == nil {
		t.Fatal("explicit empty override")
	}
}
func TestTextAndSelectors(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 34, 0, 0, time.FixedZone("JST", 9*3600))
	s, e := FormatText(`<script>& {{now "2006-01-02 15:04 MST"}}`, now)
	if e != nil || s != "&lt;script&gt;&amp; 2026-11-07 12:34 JST" {
		t.Fatal(s, e)
	}
	for _, s := range []string{"{{now}}", "{{now \"15:04\" }}", "}} {{now \"15:04\"}}", "{{now \"15:04\"}", "{{.Query}}"} {
		if _, e := FormatText(s, now); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"up", `example_power{site="example",source=~"a|b"}`} {
		if !selectorPattern.MatchString(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"sum(up)", "up * 1", "up offset 1h", "up @ 123", "up[5m]", "timestamp(up)"} {
		if selectorPattern.MatchString(s) {
			t.Fatal(s)
		}
	}
}
