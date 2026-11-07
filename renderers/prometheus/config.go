// Package prometheus implements bounded metric acquisition and panel data models.
package prometheus

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

const (
	ConfigLimit     = 256 << 10
	ContextLimit    = 1 << 20
	ResponseLimit   = 4 << 20
	TotalBytesLimit = 32 << 20
	ManifestLimit   = 1 << 20
	HTMLLimit       = 1 << 20
	RequestLimit    = 256 // Includes retry attempts and auxiliary queries.
	Concurrency     = 4
	SeriesLimit     = 8
	PanelLimit      = 48
	MaxPeriod       = 366 * 24 * time.Hour
	ExecutionBudget = 60 * time.Second
	FetchBudget     = 20 * time.Second
	OutputReserve   = 5 * time.Second
)

//go:embed config.schema.json
var schemaJSON []byte

type Config struct {
	Version    int        `json:"version"`
	Timezone   string     `json:"timezone"`
	Prometheus Prometheus `json:"prometheus"`
	Layout     Layout     `json:"layout"`
	Panels     []Panel    `json:"panels"`
	AllMissing string     `json:"allMissing"`
}
type Prometheus struct {
	URL            string `json:"url"`
	Auth           *Auth  `json:"auth,omitempty"`
	ScrapeInterval string `json:"scrapeInterval"`
}
type Auth struct {
	Type            string `json:"type"`
	CredentialsFile string `json:"credentialsFile"`
}
type Layout struct {
	Landscape Grid  `json:"landscape"`
	Portrait  *Grid `json:"portrait,omitempty"`
}
type Grid struct {
	Areas   []string `json:"areas"`
	Columns string   `json:"columns"`
	Rows    string   `json:"rows"`
	Gap     int      `json:"gap"`
}
type Panel struct {
	ID             string        `json:"id"`
	Type           string        `json:"type"`
	Title          string        `json:"title,omitempty"`
	Grow           int           `json:"grow"`
	Prefix         string        `json:"prefix,omitempty"`
	Suffix         string        `json:"suffix,omitempty"`
	Decimals       int           `json:"decimals"`
	Query          string        `json:"query,omitempty"`
	SignLabels     *SignLabels   `json:"signLabels,omitempty"`
	StaleAfter     string        `json:"staleAfter,omitempty"`
	Range          string        `json:"range,omitempty"`
	MinStep        string        `json:"minStep,omitempty"`
	Series         []Series      `json:"series,omitempty"`
	YAxis          *Axis         `json:"yAxis,omitempty"`
	ReferenceLines []Reference   `json:"referenceLines,omitempty"`
	Stack          bool          `json:"stack"`
	Buckets        *BucketConfig `json:"buckets,omitempty"`
	Text           string        `json:"text,omitempty"`
	Format         string        `json:"format,omitempty"`
	Size           string        `json:"size,omitempty"`
	Weight         string        `json:"weight,omitempty"`
	Align          string        `json:"align,omitempty"`
	VerticalAlign  string        `json:"verticalAlign,omitempty"`
	Direction      string        `json:"direction,omitempty"`
	Wrap           bool          `json:"wrap"`
	Gap            *int          `json:"gap,omitempty"`
	Panels         []Panel       `json:"panels,omitempty"`
}
type SignLabels struct {
	Positive string `json:"positive"`
	Negative string `json:"negative"`
	Abs      bool   `json:"abs"`
}
type Series struct {
	Query  string `json:"query"`
	Legend string `json:"legend,omitempty"`
	Style  string `json:"style"`
}
type Axis struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}
type Reference struct {
	Value *float64 `json:"value,omitempty"`
	Query string   `json:"query,omitempty"`
	Label string   `json:"label,omitempty"`
	Style string   `json:"style"`
}
type BucketConfig struct {
	Unit           string `json:"unit"`
	Count          int    `json:"count"`
	IncludeCurrent bool   `json:"includeCurrent"`
}

func bounded(r io.Reader, limit int) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil || len(b) > limit {
		return nil, errors.New("input size or read failure")
	}
	return b, nil
}

// decodeJSON rejects duplicate keys, trailing documents, null and invalid UTF-8.
func decodeJSON(b []byte) (any, error) {
	if !utf8.Valid(b) {
		return nil, errors.New("invalid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	depth := 0
	var read func() (any, error)
	read = func() (any, error) {
		depth++
		defer func() { depth-- }()
		if depth > 64 {
			return nil, errors.New("JSON nesting limit")
		}
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, errors.New("null is unsupported")
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				key := k.(string)
				if _, ok := m[key]; ok {
					return nil, errors.New("duplicate key")
				}
				v, e := read()
				if e != nil {
					return nil, e
				}
				m[key] = v
			}
			_, err = d.Token()
			return m, err
		case json.Delim('['):
			a := []any{}
			for d.More() {
				v, e := read()
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, err = d.Token()
			return a, err
		default:
			return t, nil
		}
	}
	v, err := read()
	if err != nil {
		return nil, errors.New("invalid JSON")
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return v, nil
}

// ParseConfig applies schema defaults once, then checks cross-field semantics.
// An explicitly supplied URL override is validated instead of the configured URL.
func ParseConfig(r io.Reader, override *string) (Config, error) {
	var c Config
	b, err := bounded(r, ConfigLimit)
	if err != nil {
		return c, err
	}
	v, err := decodeJSON(b)
	if err != nil {
		return c, err
	}
	var schema map[string]any
	if err = json.Unmarshal(schemaJSON, &schema); err != nil {
		return c, errors.New("invalid embedded schema")
	}
	if err = validateSchema(schema, v, schema); err != nil {
		return c, errors.New("configuration does not match schema")
	}
	applyDefaults(schema, v, schema)
	b, _ = json.Marshal(normalizeNumbers(v))
	if err = json.Unmarshal(b, &c); err != nil {
		return c, errors.New("invalid configuration")
	}
	if override != nil {
		c.Prometheus.URL = *override
	}
	if _, err = BaseURL(c.Prometheus.URL); err != nil {
		return c, err
	}
	if _, err = time.LoadLocation(c.Timezone); err != nil || c.Timezone == "Local" {
		return c, errors.New("invalid IANA timezone")
	}
	if _, err = period(c.Prometheus.ScrapeInterval); err != nil {
		return c, err
	}
	ids := map[string]bool{}
	count := 0
	var check func([]Panel, bool) error
	check = func(ps []Panel, child bool) error {
		for _, p := range ps {
			count++
			if count > PanelLimit || ids[p.ID] {
				return errors.New("panel count or duplicate ID")
			}
			ids[p.ID] = true
			if p.Type == "group" {
				if child {
					return errors.New("nested group")
				}
				if err := check(p.Panels, true); err != nil {
					return err
				}
			}
			if p.YAxis != nil && p.YAxis.Min != nil && p.YAxis.Max != nil && *p.YAxis.Min >= *p.YAxis.Max {
				return errors.New("inverted axis")
			}
			for _, s := range []string{p.StaleAfter, p.Range, p.MinStep} {
				if s != "" {
					if _, err := period(s); err != nil {
						return err
					}
				}
			}
			if p.Type == "text" {
				if _, err := FormatText(p.Text, time.Unix(0, 0)); err != nil {
					return err
				}
			}
			if p.StaleAfter != "" && !selectorPattern.MatchString(p.Query) {
				return errors.New("staleAfter requires a bare instant selector")
			}
			if p.Query != "" {
				if err := validateVariables(p.Query, "value"); err != nil {
					return err
				}
			}
			for _, s := range p.Series {
				if err := validateVariables(s.Query, p.Type); err != nil {
					return err
				}
				if err := validateLegend(s.Legend); err != nil {
					return err
				}
			}
			for _, s := range p.ReferenceLines {
				if s.Query != "" {
					if err := validateVariables(s.Query, "value"); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err = check(c.Panels, false); err != nil {
		return c, err
	}
	if err = validateGrid(c.Layout.Landscape, c.Panels); err != nil {
		return c, err
	}
	if c.Layout.Portrait == nil {
		g := portrait(c.Layout.Landscape)
		c.Layout.Portrait = &g
	}
	if err = validateGrid(*c.Layout.Portrait, c.Panels); err != nil {
		return c, err
	}
	return c, nil
}

func BaseURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 8192 || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Opaque != "" || strings.Contains(u.Path, "\\") {
		return nil, errors.New("invalid service base URL")
	}
	if strings.Contains(strings.ToLower(u.RawPath), "%2f") || strings.Contains(strings.ToLower(u.RawPath), "%5c") || strings.Contains(u.Path, "//") {
		return nil, errors.New("invalid escaped base path")
	}
	for _, r := range u.Path {
		if r < 32 || r == 127 {
			return nil, errors.New("invalid base path character")
		}
	}
	for _, s := range strings.Split(u.Path, "/") {
		if s == "." || s == ".." {
			return nil, errors.New("invalid base path")
		}
	}
	return u, nil
}

// APIURL appends an API route without discarding the configured base prefix.
func APIURL(base, endpoint string) (string, error) {
	u, e := BaseURL(base)
	if e != nil {
		return "", e
	}
	if endpoint != "/api/v1/query" && endpoint != "/api/v1/query_range" {
		return "", errors.New("invalid API route")
	}
	u.Path = strings.TrimRight(u.Path, "/") + endpoint
	u.RawPath = ""
	return u.String(), nil
}

var durationPattern = regexp.MustCompile(`^[1-9][0-9]*(s|m|h)$`)

func period(s string) (int64, error) {
	d, e := time.ParseDuration(s)
	if !durationPattern.MatchString(s) || e != nil || d < time.Second || d > MaxPeriod {
		return 0, errors.New("invalid bounded duration")
	}
	return int64(d / time.Second), nil
}

// The embedded schema uses only this small, closed JSON Schema vocabulary.
// Semantic checks (rectangles, timezone, variables) are intentionally separate.
func resolve(s, root map[string]any) map[string]any {
	if ref, ok := s["$ref"].(string); ok {
		return root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	}
	return s
}
func validateSchema(s map[string]any, v any, root map[string]any) error {
	s = resolve(s, root)
	bad := errors.New("schema violation")
	if opts, ok := s["oneOf"].([]any); ok {
		n := 0
		for _, o := range opts {
			if validateSchema(o.(map[string]any), v, root) == nil {
				n++
			}
		}
		if n != 1 {
			return bad
		}
	}
	if x, ok := s["const"]; ok {
		if n, ok := v.(json.Number); ok {
			f, e := n.Float64()
			expected, isNumber := x.(float64)
			if e != nil || !isNumber || f != expected || !exactInteger(n) {
				return bad
			}
		} else {
			a, _ := json.Marshal(v)
			b, _ := json.Marshal(x)
			if !bytes.Equal(a, b) {
				return bad
			}
		}
	}
	if opts, ok := s["enum"].([]any); ok {
		found := false
		for _, o := range opts {
			if v == o {
				found = true
			}
		}
		if !found {
			return bad
		}
	}
	if typ, ok := s["type"].(string); ok {
		switch typ {
		case "object":
			if _, ok := v.(map[string]any); !ok {
				return bad
			}
		case "array":
			if _, ok := v.([]any); !ok {
				return bad
			}
		case "string":
			if _, ok := v.(string); !ok {
				return bad
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return bad
			}
		case "integer", "number":
			n, ok := v.(json.Number)
			if !ok {
				return bad
			}
			f, e := n.Float64()
			if e != nil || math.IsInf(f, 0) || math.IsNaN(f) {
				return bad
			}
			if typ == "integer" {
				if !exactInteger(n) {
					return bad
				}
			}
			if min, ok := s["minimum"].(float64); ok && f < min {
				return bad
			}
			if max, ok := s["maximum"].(float64); ok && f > max {
				return bad
			}
		}
	}
	if m, ok := v.(map[string]any); ok {
		props, _ := s["properties"].(map[string]any)
		if req, ok := s["required"].([]any); ok {
			for _, k := range req {
				if _, ok := m[k.(string)]; !ok {
					return bad
				}
			}
		}
		for k, x := range m {
			if p, ok := props[k]; ok {
				if e := validateSchema(p.(map[string]any), x, root); e != nil {
					return e
				}
			} else if s["additionalProperties"] == false {
				return bad
			}
		}
	}
	if a, ok := v.([]any); ok {
		if min, ok := s["minItems"].(float64); ok && len(a) < int(min) {
			return bad
		}
		if max, ok := s["maxItems"].(float64); ok && len(a) > int(max) {
			return bad
		}
		if item, ok := s["items"].(map[string]any); ok {
			for _, x := range a {
				if e := validateSchema(item, x, root); e != nil {
					return e
				}
			}
		}
	}
	if t, ok := v.(string); ok {
		if min, ok := s["minLength"].(float64); ok && utf8.RuneCountInString(t) < int(min) {
			return bad
		}
		if max, ok := s["maxLength"].(float64); ok && utf8.RuneCountInString(t) > int(max) {
			return bad
		}
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(t) {
			return bad
		}
	}
	return nil
}

// exactInteger checks decimal syntax without float rounding or allocating large
// powers of ten for hostile exponents. The JSON decoder already checked grammar.
func exactInteger(n json.Number) bool {
	s := string(n)
	mantissa := s
	exponent := "0"
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa = s[:i]
		exponent = s[i+1:]
	}
	if strings.Trim(mantissa, "-0.") == "" {
		return true
	}
	e, err := strconv.ParseInt(exponent, 10, 32)
	if err != nil {
		return false
	}
	fraction := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fraction = len(mantissa) - i - 1
	}
	scale := e - int64(fraction)
	if scale >= 0 {
		return true
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	zeroes := len(digits) - len(strings.TrimRight(digits, "0"))
	return int64(zeroes) >= -scale
}

func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, n := range x {
			x[k] = normalizeNumbers(n)
		}
	case []any:
		for i, n := range x {
			x[i] = normalizeNumbers(n)
		}
	}
	return v
}
func applyDefaults(s map[string]any, v any, root map[string]any) {
	s = resolve(s, root)
	if opts, ok := s["oneOf"].([]any); ok {
		for _, o := range opts {
			b := o.(map[string]any)
			if validateSchema(b, v, root) == nil {
				applyDefaults(b, v, root)
				return
			}
		}
	}
	if m, ok := v.(map[string]any); ok {
		props, _ := s["properties"].(map[string]any)
		for k, p := range props {
			ps := resolve(p.(map[string]any), root)
			if _, ok := m[k]; !ok {
				if d, ok := ps["default"]; ok {
					m[k] = d
				}
			}
			if x, ok := m[k]; ok {
				applyDefaults(ps, x, root)
			}
		}
	}
	if a, ok := v.([]any); ok {
		if item, ok := s["items"].(map[string]any); ok {
			for _, x := range a {
				applyDefaults(item, x, root)
			}
		}
	}
}
