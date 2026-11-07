package prometheus

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Context struct {
	Version string `json:"version"`
	Device  struct {
		Name string `json:"name"`
	} `json:"device"`
	Target struct {
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Palette  string `json:"palette"`
		Rotation int    `json:"rotation"`
	} `json:"renderTarget"`
	Rotation int `json:"-"`
}

func ParseContext(b []byte) (Context, error) {
	var c Context
	if len(b) > ContextLimit {
		return c, errors.New("Context size limit")
	}
	v, e := decodeJSON(b)
	if e != nil {
		return c, e
	}
	raw, _ := json.Marshal(v)
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, errors.New("invalid Context")
	}
	c.Rotation = c.Target.Rotation
	if c.Version != "v1" || strings.TrimSpace(c.Device.Name) == "" || c.Target.Width != 800 || c.Target.Height != 480 || (c.Target.Palette != "gray4" && c.Target.Palette != "spectra6") || (c.Rotation != 0 && c.Rotation != 90 && c.Rotation != 180 && c.Rotation != 270) {
		return c, errors.New("unsupported Context")
	}
	return c, nil
}
func (c Context) LogicalSize() (int, int) {
	if c.Rotation == 90 || c.Rotation == 270 {
		return 480, 800
	}
	return 800, 480
}

var textExpr = regexp.MustCompile(`^now "([^"\r\n]{1,128})"$`)

// FormatText returns escaped text; no user input becomes template source.
func FormatText(s string, now time.Time) (string, error) {
	var b strings.Builder
	for s != "" {
		i := strings.Index(s, "{{")
		j := strings.Index(s, "}}")
		if i < 0 {
			if j >= 0 {
				return "", errors.New("unmatched text expression")
			}
			b.WriteString(s)
			break
		}
		if j >= 0 && j < i {
			return "", errors.New("unmatched text expression")
		}
		b.WriteString(s[:i])
		s = s[i+2:]
		end := strings.Index(s, "}}")
		if end < 0 {
			return "", errors.New("unclosed text expression")
		}
		m := textExpr.FindStringSubmatch(s[:end])
		if m == nil {
			return "", errors.New("unsupported text expression")
		}
		b.WriteString(now.Format(m[1]))
		s = s[end+2:]
	}
	return html.EscapeString(b.String()), nil
}

// Freshness is deliberately restricted to bare named vector selectors. PromQL
// aggregations and arithmetic set evaluation timestamps, not source timestamps.
var selectorPattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[a-zA-Z_][a-zA-Z0-9_]*\s*(=|!=|=~|!~)\s*"([^"\\]|\\.)*"(\s*,\s*[a-zA-Z_][a-zA-Z0-9_]*\s*(=|!=|=~|!~)\s*"([^"\\]|\\.)*")*\})?$`)
var variablePattern = regexp.MustCompile(`\$[a-zA-Z_][a-zA-Z0-9_]*`)
var legendExpr = regexp.MustCompile(`\{\{[a-zA-Z_][a-zA-Z0-9_]*\}\}`)

func validateLegend(s string) error {
	r := legendExpr.ReplaceAllString(s, "")
	if strings.Contains(r, "{{") || strings.Contains(r, "}}") {
		return errors.New("invalid legend template")
	}
	return nil
}
func allowedVariables(kind string) map[string]bool {
	m := map[string]bool{"$__now": true, "$__today_start": true, "$__today_range": true}
	if kind == "line" {
		for _, k := range []string{"$__range", "$__step", "$__rate_interval"} {
			m[k] = true
		}
	}
	if kind == "bar" {
		for _, k := range []string{"$__bucket_start", "$__bucket_end", "$__bucket_range"} {
			m[k] = true
		}
	}
	return m
}
func validateVariables(q, kind string) error {
	m := allowedVariables(kind)
	for _, v := range variablePattern.FindAllString(q, -1) {
		if !m[v] {
			return errors.New("unknown or out-of-scope variable")
		}
	}
	if strings.Contains(variablePattern.ReplaceAllString(q, ""), "$") {
		return errors.New("invalid variable token")
	}
	return nil
}
func expand(q string, vars map[string]string) (string, error) {
	var err error
	s := variablePattern.ReplaceAllStringFunc(q, func(v string) string {
		x, ok := vars[v]
		if !ok {
			err = errors.New("variable unavailable")
		}
		if x == "0s" {
			err = errors.New("zero-length calendar range")
		}
		return x
	})
	return s, err
}

type Bucket struct {
	Start       int64  `json:"start"`
	End         int64  `json:"end"` // Evaluation end, clipped to now.
	BoundaryEnd int64  `json:"boundaryEnd"`
	Partial     bool   `json:"partial"`
	Label       string `json:"label"`
}

func calendarBoundary(t time.Time, n int) (time.Time, error) {
	y, m, d := t.Date()
	date := time.Date(y, m, d+n, 0, 0, 0, 0, time.UTC)
	y, m, d = date.Date()
	b := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	by, bm, bd := b.Date()
	if by != y || bm != m || bd != d || b.Hour() != 0 || b.Minute() != 0 || b.Second() != 0 {
		return time.Time{}, errors.New("nonexistent local midnight")
	}
	// Choose the earliest occurrence if midnight itself is repeated.
	start, end := b.ZoneBounds()
	for _, probe := range []time.Time{start.Add(-time.Second), end} {
		if probe.IsZero() {
			continue
		}
		_, offset := probe.Zone()
		candidate := date.Add(-time.Duration(offset) * time.Second).In(t.Location())
		cy, cm, cd := candidate.Date()
		if cy == y && cm == m && cd == d && candidate.Hour() == 0 && candidate.Minute() == 0 && candidate.Second() == 0 && candidate.Before(b) {
			b = candidate
		}
	}
	return b, nil
}

// Hour buckets are consecutive elapsed hours anchored at local midnight.
// Spring's skipped hour is absent; fall's repeated hours retain distinct offsets.
func Buckets(now time.Time, b BucketConfig) ([]Bucket, error) {
	if (b.Unit != "hour" && b.Unit != "day") || b.Count < 1 || b.Count > 31 {
		return nil, errors.New("invalid bucket configuration")
	}
	now = integerTime(now)
	day, e := calendarBoundary(now, 0)
	if e != nil {
		return nil, e
	}
	start := day
	if b.Unit == "hour" {
		start = day.Add(time.Duration((now.Unix()-day.Unix())/3600) * time.Hour)
	}
	shift := func(n int) (time.Time, error) {
		if b.Unit == "day" {
			return calendarBoundary(start, n)
		}
		return start.Add(time.Duration(n) * time.Hour), nil
	}
	offset := 0
	if !b.IncludeCurrent {
		offset = -1
	}
	out := make([]Bucket, 0, b.Count)
	for i := 0; i < b.Count; i++ {
		s, err := shift(offset + 1 - b.Count + i)
		if err != nil {
			return nil, err
		}
		end, err := shift(offset + 2 - b.Count + i)
		if err != nil {
			return nil, err
		}
		e := end
		if e.After(now) {
			e = now
		}
		if e.Before(s) || s.Unix() < 0 {
			return nil, errors.New("invalid calendar boundary")
		}
		label := s.Format("2006-01-02")
		if b.Unit == "hour" {
			label = s.Format("01-02 15:04 -0700")
		}
		out = append(out, Bucket{s.Unix(), e.Unix(), end.Unix(), e.Before(end), label})
	}
	return out, nil
}
func integerTime(t time.Time) time.Time { return time.Unix(t.Unix(), 0).In(t.Location()) }
func seconds(v int64) string            { return strconv.FormatInt(v, 10) }
func durationSeconds(v int64) string    { return seconds(v) + "s" }
