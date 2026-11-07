package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Model contains data for leaf panels in configuration order, without queries,
// service URLs, credentials or raw API annotations. Group layout stays in Config.
type Model struct {
	Now    int64
	Panels []PanelModel
}

type PanelModel struct {
	ID, Type   string
	Value      *ValueModel
	Series     []SeriesModel
	References []ReferenceModel
	Buckets    []Bucket
	Missing    bool
}

type ValueModel struct {
	Number           float64
	Formatted, Label string
	Stale            bool
}

type SeriesModel struct {
	QueryIndex    int
	Labels        map[string]string
	Legend, Style string
	Points        []Point
	// Segments index consecutive present points. End is exclusive.
	Segments []Segment
}

type Segment struct{ Start, End int }

type Point struct {
	Time    int64
	Value   float64
	Present bool
	// Base/Top use independent positive and negative accumulators.
	// StackKnown is false if an earlier contribution at this time was missing.
	Base, Top  float64
	StackKnown bool
}

type ReferenceModel struct {
	Value        float64
	Label, Style string
}

// LoadModel is the single acquisition boundary for future rendering. A failure
// returns no model, so a caller must not proceed to capture/output replacement.
// The caller owns the total execution deadline; live fetching additionally caps
// its shared stage at FetchBudget. Snapshots never open credential files.
func LoadModel(ctx context.Context, c Config, p Plan, input string) (Model, FetchStats, error) {
	if ctx.Err() != nil {
		return Model{}, FetchStats{}, Diagnostic{Code: "canceled"}
	}
	if p.ConfigHash != hash(c) {
		return Model{}, FetchStats{}, Diagnostic{Code: "plan_mismatch"}
	}
	var responses map[string]Response
	var stats FetchStats
	if input != "" {
		raw, err := LoadSnapshot(input, p)
		if err != nil {
			return Model{}, stats, Diagnostic{Code: "snapshot_invalid"}
		}
		responses, err = ParseResponses(p, raw)
		if err != nil {
			return Model{}, stats, err
		}
		for _, r := range responses {
			stats.Warnings += r.Warnings
			stats.Infos += r.Infos
		}
	} else {
		client, err := NewClient(c.Prometheus)
		if err != nil {
			return Model{}, stats, err
		}
		responses, stats, err = client.Fetch(ctx, p)
		if err != nil {
			return Model{}, stats, err
		}
	}
	if ctx.Err() != nil {
		return Model{}, stats, Diagnostic{Code: "canceled"}
	}
	m, err := Normalize(c, p, responses)
	if ctx.Err() != nil {
		return Model{}, stats, Diagnostic{Code: "canceled"}
	}
	return m, stats, err
}

func ParseResponses(p Plan, raw map[string]json.RawMessage) (map[string]Response, error) {
	if len(raw) != len(p.Requests) {
		return nil, Diagnostic{Code: "response_count"}
	}
	out := map[string]Response{}
	total := 0
	for _, r := range p.Requests {
		b, ok := raw[r.ID]
		total += len(b)
		if !ok || total > TotalBytesLimit {
			return nil, Diagnostic{r.Panel, "response_limit"}
		}
		v, err := ParseResponse(b, r)
		if err != nil {
			return nil, Diagnostic{r.Panel, err.Error()}
		}
		out[r.ID] = v
	}
	return out, nil
}

func singleValue(r Response) (*float64, error) {
	// Count before filtering NaN and infinities.
	if len(r.Series) > 1 {
		return nil, errors.New("multiple_samples")
	}
	if len(r.Series) == 0 {
		return nil, nil
	}
	if len(r.Series[0].Samples) != 1 {
		return nil, errors.New("invalid_sample_count")
	}
	v := r.Series[0].Samples[0].Value
	if !finite(v) {
		return nil, nil
	}
	return &v, nil
}

// FormatNumber never changes units; prefix/suffix remain literal strings.
func FormatNumber(v float64, decimals int, prefix, suffix string) string {
	if v == 0 || (decimals >= 0 && decimals <= 6 && math.Abs(v) < .5/math.Pow10(decimals)) {
		v = 0
	}
	return prefix + strconv.FormatFloat(v, 'f', decimals, 64) + suffix
}

// Normalize requires a validated Config, its BuildPlan result and responses
// validated by ParseResponse (or Client.Fetch).
func Normalize(c Config, p Plan, responses map[string]Response) (Model, error) {
	m := Model{Now: p.Now, Panels: []PanelModel{}}
	if len(responses) != len(p.Requests) {
		return Model{}, Diagnostic{Code: "response_count"}
	}
	byPanel := map[string][]Request{}
	for _, r := range p.Requests {
		if _, ok := responses[r.ID]; !ok {
			return Model{}, Diagnostic{r.Panel, "response_missing"}
		}
		byPanel[r.Panel] = append(byPanel[r.Panel], r)
	}
	dataPanels, available := 0, false
	var visit func([]Panel) error
	visit = func(panels []Panel) error {
		for _, panel := range panels {
			if panel.Type == "group" {
				if err := visit(panel.Panels); err != nil {
					return err
				}
				continue
			}
			pm, err := normalizePanel(panel, p, byPanel[panel.ID], responses)
			if err != nil {
				return Diagnostic{panel.ID, err.Error()}
			}
			m.Panels = append(m.Panels, pm)
			if panel.Type != "text" {
				dataPanels++
				available = available || !pm.Missing
			}
		}
		return nil
	}
	if err := visit(c.Panels); err != nil {
		return Model{}, err
	}
	if dataPanels > 0 && !available && c.AllMissing != "display" {
		return Model{}, Diagnostic{Code: "all_missing"}
	}
	return m, nil
}

func normalizePanel(panel Panel, p Plan, requests []Request, responses map[string]Response) (PanelModel, error) {
	pm := PanelModel{ID: panel.ID, Type: panel.Type, Missing: true, Buckets: p.Buckets[panel.ID]}
	if panel.Type == "text" {
		return pm, nil
	}
	if panel.Type == "value" {
		var value, freshness *float64
		for _, r := range requests {
			v, err := singleValue(responses[r.ID])
			if err != nil {
				return pm, err
			}
			if r.Role == "value" {
				value = v
			} else {
				freshness = v
				if v != nil && (*v < 0 || *v > float64(p.Now)) {
					return pm, errors.New("invalid_freshness")
				}
			}
		}
		if value == nil {
			return pm, nil
		}
		v := *value
		label := panel.Title
		if panel.SignLabels != nil {
			label = panel.SignLabels.Positive
			if v < 0 {
				label = panel.SignLabels.Negative
			}
			if panel.SignLabels.Abs {
				v = math.Abs(v)
			}
		}
		stale := false
		if panel.StaleAfter != "" {
			age, _ := period(panel.StaleAfter)
			stale = freshness == nil || float64(p.Now)-*freshness > float64(age)
		}
		pm.Value = &ValueModel{*value, FormatNumber(v, panel.Decimals, panel.Prefix, panel.Suffix), label, stale}
		pm.Missing = false
		return pm, nil
	}
	for i, ref := range panel.ReferenceLines {
		v := ref.Value
		if ref.Query != "" {
			for _, r := range requests {
				if r.Role == "reference" && r.Series == i {
					var err error
					v, err = singleValue(responses[r.ID])
					if err != nil {
						return pm, err
					}
				}
			}
		}
		if v != nil {
			pm.References = append(pm.References, ReferenceModel{*v, ref.Label, ref.Style})
		}
	}
	type collected struct {
		model   SeriesModel
		samples map[int64]float64
	}
	series := map[string]*collected{}
	var lineRequest Request
	for _, r := range requests {
		if r.Role != "range" && r.Role != "bucket" {
			continue
		}
		if r.Role == "range" {
			lineRequest = r
		}
		for _, s := range responses[r.ID].Series {
			key := fmt.Sprintf("%d/%s", r.Series, hash(s.Labels))
			cs := series[key]
			if cs == nil {
				if len(series) == SeriesLimit {
					return pm, errors.New("series_limit")
				}
				labels := map[string]string{}
				for k, v := range s.Labels {
					labels[k] = v
				}
				cs = &collected{SeriesModel{QueryIndex: r.Series, Labels: labels, Style: panel.Series[r.Series].Style}, map[int64]float64{}}
				series[key] = cs
			}
			for _, sample := range s.Samples {
				t := sample.Time
				if panel.Type == "bar" {
					t = int64(r.Bucket)
				}
				cs.samples[t] = sample.Value
			}
		}
	}
	for _, cs := range series {
		add := func(key, timestamp int64) {
			v, ok := cs.samples[key]
			present := ok && finite(v)
			if !present {
				v = 0
			}
			cs.model.Points = append(cs.model.Points, Point{Time: timestamp, Value: v, Present: present})
			pm.Missing = pm.Missing && !present
		}
		if panel.Type == "bar" {
			for i, b := range pm.Buckets {
				add(int64(i), b.End)
			}
		} else {
			if lineRequest.Step <= 0 {
				return pm, errors.New("invalid_plan")
			}
			for t := lineRequest.Start; t <= lineRequest.End; t += lineRequest.Step {
				add(t, t)
			}
		}
		pm.Series = append(pm.Series, cs.model)
	}
	sort.Slice(pm.Series, func(i, j int) bool {
		cmp := compareLabels(pm.Series[i].Labels, pm.Series[j].Labels)
		if cmp != 0 {
			return cmp < 0
		}
		return pm.Series[i].QueryIndex < pm.Series[j].QueryIndex
	})
	setLegends(pm.Series, panel.Series)
	if err := stackPoints(pm.Series, panel.Stack); err != nil {
		return pm, err
	}
	return pm, nil
}

func labelKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func compareLabels(a, b map[string]string) int {
	ak, bk := labelKeys(a), labelKeys(b)
	for i := 0; i < min(len(ak), len(bk)); i++ {
		if c := strings.Compare(ak[i], bk[i]); c != 0 {
			return c
		}
		if c := strings.Compare(a[ak[i]], b[bk[i]]); c != 0 {
			return c
		}
	}
	return len(ak) - len(bk)
}

var legendToken = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

func setLegends(series []SeriesModel, configured []Series) {
	names := make([]string, len(series))
	counts := map[string]int{}
	for i, s := range series {
		name := configured[s.QueryIndex].Legend
		if name != "" {
			name = legendToken.ReplaceAllStringFunc(name, func(token string) string {
				if v, ok := s.Labels[token[2:len(token)-2]]; ok {
					return v
				}
				return "?"
			})
		} else {
			parts := []string{}
			for _, k := range labelKeys(s.Labels) {
				differs := false
				for _, other := range series {
					v, ok := other.Labels[k]
					if !ok || v != s.Labels[k] {
						differs = true
						break
					}
				}
				if differs {
					parts = append(parts, k+"="+s.Labels[k])
				}
			}
			name = strings.Join(parts, ", ")
		}
		if name == "" {
			name = fmt.Sprintf("Query %d", s.QueryIndex+1)
		}
		names[i] = name
		counts[name]++
	}
	used, sequence := map[string]bool{}, map[string]int{}
	for i, name := range names {
		candidate := name
		if counts[name] > 1 || used[candidate] {
			for {
				sequence[name]++
				candidate = fmt.Sprintf("%s (%d)", name, sequence[name])
				if !used[candidate] && counts[candidate] == 0 {
					break
				}
			}
		}
		series[i].Legend = candidate
		used[candidate] = true
	}
}

func stackPoints(series []SeriesModel, stack bool) error {
	if len(series) == 0 {
		return nil
	}
	for i := range series[0].Points {
		positive, negative, known := 0.0, 0.0, true
		for si := range series {
			point := &series[si].Points[i]
			if !point.Present {
				known = false
				continue
			}
			point.StackKnown = !stack || known
			if stack {
				if point.Value >= 0 {
					point.Base = positive
					positive += point.Value
					point.Top = positive
				} else {
					point.Base = negative
					negative += point.Value
					point.Top = negative
				}
			} else {
				point.Top = point.Value
			}
			if !finite(point.Top) {
				return errors.New("stack_overflow")
			}
		}
	}
	for si := range series {
		s := &series[si]
		for i, p := range s.Points {
			if !p.Present || !p.StackKnown {
				continue
			}
			last := len(s.Segments) - 1
			if last >= 0 && s.Segments[last].End == i {
				s.Segments[last].End++
			} else {
				s.Segments = append(s.Segments, Segment{i, i + 1})
			}
		}
	}
	return nil
}
