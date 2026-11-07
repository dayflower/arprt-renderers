package prometheus

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Response contains only validated float samples and annotation counts.
// API annotation text and error payloads are deliberately discarded.
type Response struct {
	Series   []SampleSeries
	Warnings int
	Infos    int
}

type SampleSeries struct {
	Labels  map[string]string
	Samples []Sample
}

type Sample struct {
	Time  int64
	Value float64 // NaN and infinities remain missing, never observed zero.
}

var floatSample = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// ParseResponse accepts scalar/vector instant results and float-only matrices.
// Evaluation timestamps must match the requested instant or planned range grid.
func ParseResponse(raw []byte, r Request) (Response, error) {
	out, err := parseResponse(raw, r)
	if err != nil {
		return Response{}, err
	}
	if r.Role == "value" || r.Role == "freshness" || r.Role == "reference" {
		v, err := singleValue(out)
		if err != nil {
			return Response{}, err
		}
		if r.Role == "freshness" && v != nil && (*v < 0 || *v > float64(r.Time)) {
			return Response{}, errors.New("invalid_freshness")
		}
	}
	return out, nil
}

func parseResponse(raw []byte, r Request) (Response, error) {
	bad := errors.New("invalid_response")
	if len(raw) > ResponseLimit {
		return Response{}, errors.New("response_limit")
	}
	v, err := decodeJSON(raw)
	if err != nil {
		return Response{}, bad
	}
	root, ok := v.(map[string]any)
	if !ok {
		return Response{}, bad
	}
	out := Response{Series: []SampleSeries{}}
	for key, dst := range map[string]*int{"warnings": &out.Warnings, "infos": &out.Infos} {
		if a, exists := root[key]; exists {
			items, ok := a.([]any)
			if !ok {
				return Response{}, bad
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return Response{}, bad
				}
			}
			*dst = len(items)
		}
	}
	if root["status"] != "success" {
		return Response{}, errors.New("api_error")
	}
	if _, ok := root["error"]; ok {
		return Response{}, bad
	}
	if _, ok := root["errorType"]; ok {
		return Response{}, bad
	}
	data, ok := root["data"].(map[string]any)
	if !ok {
		return Response{}, bad
	}
	kind, _ := data["resultType"].(string)
	if (r.Role == "range" && kind != "matrix") || (r.Role != "range" && kind != "scalar" && kind != "vector") {
		return Response{}, errors.New("unsupported_result")
	}
	if kind == "scalar" {
		s, err := parseSample(data["result"], r)
		if err != nil {
			return Response{}, bad
		}
		out.Series = append(out.Series, SampleSeries{map[string]string{}, []Sample{s}})
		return out, nil
	}
	items, ok := data["result"].([]any)
	if !ok || len(items) > SeriesLimit {
		return Response{}, bad
	}
	seen := map[string]bool{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || len(m) != 2 {
			return Response{}, bad
		}
		metric, ok := m["metric"].(map[string]any)
		if !ok {
			return Response{}, bad
		}
		s := SampleSeries{Labels: map[string]string{}, Samples: []Sample{}}
		for k, v := range metric {
			str, ok := v.(string)
			if !ok || k == "" {
				return Response{}, bad
			}
			s.Labels[k] = str
		}
		key := hash(s.Labels)
		if seen[key] {
			return Response{}, bad
		}
		seen[key] = true
		values := []any{m["value"]}
		if kind == "matrix" {
			values, ok = m["values"].([]any)
			if !ok || len(values) > 500 {
				return Response{}, bad
			}
		}
		for i, value := range values {
			sample, err := parseSample(value, r)
			if err != nil || (i > 0 && sample.Time <= s.Samples[i-1].Time) {
				return Response{}, bad
			}
			s.Samples = append(s.Samples, sample)
		}
		out.Series = append(out.Series, s)
	}
	return out, nil
}

func parseSample(v any, r Request) (Sample, error) {
	bad := errors.New("invalid_sample")
	pair, ok := v.([]any)
	if !ok || len(pair) != 2 {
		return Sample{}, bad
	}
	n, ok := pair[0].(json.Number)
	if !ok {
		return Sample{}, bad
	}
	// Avoid float rounding accepting a fractional or out-of-bounds timestamp.
	// Bound the token first so adversarial exponents cannot allocate huge integers.
	if len(n.String()) > 32 {
		return Sample{}, bad
	}
	if i := strings.IndexAny(n.String(), "eE"); i >= 0 {
		if _, err := strconv.ParseInt(n.String()[i+1:], 10, 8); err != nil {
			return Sample{}, bad
		}
	}
	t, ok := new(big.Rat).SetString(n.String())
	if !ok || !t.IsInt() || !t.Num().IsInt64() {
		return Sample{}, bad
	}
	ts := t.Num().Int64()
	if ts < 0 || ts > 253402300799 {
		return Sample{}, bad
	}
	if r.Role == "range" {
		if r.Step <= 0 || ts < r.Start || ts > r.End || (ts-r.Start)%r.Step != 0 {
			return Sample{}, bad
		}
	} else if ts != r.Time {
		return Sample{}, bad
	}
	str, ok := pair[1].(string)
	if !ok || (!floatSample.MatchString(str) && str != "NaN" && str != "+Inf" && str != "-Inf" && str != "Inf") {
		return Sample{}, bad
	}
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return Sample{}, bad
	}
	if f == 0 {
		f = 0 // Canonicalize negative zero.
	}
	return Sample{ts, f}, nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
