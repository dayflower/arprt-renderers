package prometheus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type Request struct {
	ID       string `json:"id"`
	Panel    string `json:"panel"`
	Role     string `json:"role"`
	Series   int    `json:"series"`
	Bucket   int    `json:"bucket"` // -1 outside bar panels.
	Endpoint string `json:"endpoint"`
	Query    string `json:"query"`
	Time     int64  `json:"time"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Step     int64  `json:"step"`
}
type Plan struct {
	Version     int                 `json:"version"`
	Now         int64               `json:"now"`
	ConfigHash  string              `json:"configHash"`
	ContextHash string              `json:"contextHash"`
	Requests    []Request           `json:"requests"`
	Buckets     map[string][]Bucket `json:"buckets"`
	PlotWidths  map[string]int      `json:"plotWidths"`
}

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// BuildPlan requires a Config returned by ParseConfig and a validated Context.
// It never reads the wall clock or makes network requests.
func BuildPlan(c Config, ctx Context, now time.Time) (Plan, error) {
	loc, e := time.LoadLocation(c.Timezone)
	if e != nil {
		return Plan{}, errors.New("invalid timezone")
	}
	now = integerTime(now.In(loc))
	if now.Unix() < 0 || now.Year() > 9999 {
		return Plan{}, errors.New("execution time outside bounds")
	}
	day, e := calendarBoundary(now, 0)
	if e != nil || day.Unix() < 0 {
		return Plan{}, errors.New("invalid local day boundary")
	}
	p := Plan{Version: 1, Now: now.Unix(), ConfigHash: hash(c), ContextHash: hash(ctx), Requests: []Request{}, Buckets: map[string][]Bucket{}, PlotWidths: PlotWidths(c, ctx)}
	base := map[string]string{"$__now": seconds(now.Unix()), "$__today_start": seconds(day.Unix()), "$__today_range": durationSeconds(now.Unix() - day.Unix())}
	add := func(panel, role string, s, b int, q string, vars map[string]string, t, start, end, step int64) error {
		expanded, err := expand(q, vars)
		if err != nil {
			return err
		}
		if len(expanded) > 16384 {
			return errors.New("expanded query size limit")
		}
		endpoint := "/api/v1/query"
		if role == "range" {
			endpoint = "/api/v1/query_range"
		}
		r := Request{fmt.Sprintf("%s/%s/%d/%d", panel, role, s, b), panel, role, s, b, endpoint, expanded, t, start, end, step}
		p.Requests = append(p.Requests, r)
		// Reserve one retry per task within the absolute attempt limit.
		if 2*len(p.Requests) > RequestLimit {
			return errors.New("query request budget exceeded")
		}
		return nil
	}
	var visit func([]Panel) error
	visit = func(ps []Panel) error {
		for _, panel := range ps {
			switch panel.Type {
			case "group":
				if e := visit(panel.Panels); e != nil {
					return e
				}
			case "value":
				if e := add(panel.ID, "value", 0, -1, panel.Query, base, p.Now, 0, 0, 0); e != nil {
					return e
				}
				if panel.StaleAfter != "" {
					if e := add(panel.ID, "freshness", 0, -1, "timestamp("+panel.Query+")", base, p.Now, 0, 0, 0); e != nil {
						return e
					}
				}
			case "line":
				rangeSec, _ := period(panel.Range)
				minStep, _ := period(panel.MinStep)
				scrape, _ := period(c.Prometheus.ScrapeInterval)
				intervals := int64(max(1, min(499, p.PlotWidths[panel.ID]-1)))
				step := max(minStep, (rangeSec+intervals-1)/intervals)
				vars := copyVars(base)
				vars["$__range"] = durationSeconds(rangeSec)
				vars["$__step"] = durationSeconds(step)
				vars["$__rate_interval"] = durationSeconds(max(4*scrape, step+scrape))
				if p.Now-rangeSec < 0 {
					return errors.New("query start outside bounds")
				}
				for i, s := range panel.Series {
					if e := add(panel.ID, "range", i, -1, s.Query, vars, 0, p.Now-rangeSec, p.Now, step); e != nil {
						return e
					}
				}
				for i, r := range panel.ReferenceLines {
					if r.Query != "" {
						if e := add(panel.ID, "reference", i, -1, r.Query, base, p.Now, 0, 0, 0); e != nil {
							return e
						}
					}
				}
			case "bar":
				bs, e := Buckets(now, *panel.Buckets)
				if e != nil {
					return e
				}
				p.Buckets[panel.ID] = bs
				for bi, b := range bs {
					vars := copyVars(base)
					vars["$__bucket_start"] = seconds(b.Start)
					vars["$__bucket_end"] = seconds(b.End)
					vars["$__bucket_range"] = durationSeconds(b.End - b.Start)
					for si, s := range panel.Series {
						if e := add(panel.ID, "bucket", si, bi, s.Query, vars, b.End, 0, 0, 0); e != nil {
							return e
						}
					}
				}
			}
		}
		return nil
	}
	if e = visit(c.Panels); e != nil {
		return Plan{}, e
	}
	return p, nil
}
func copyVars(m map[string]string) map[string]string {
	n := map[string]string{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

// Options is shared by the future rendering CLI and offline planning tests.
type Options struct {
	Config        string
	Context       string
	Output        string
	Input         string
	PrometheusURL *string
	Now           time.Time
}

func ParseOptions(args []string, clock func() time.Time) (Options, error) {
	o := Options{}
	f := flag.NewFlagSet("prometheus", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Config, "config", "", "configuration file")
	f.StringVar(&o.Context, "context", "/arprt/context.json", "Render Context")
	f.StringVar(&o.Output, "output", "/arprt/output.png", "output PNG")
	f.StringVar(&o.Input, "input", "", "snapshot directory")
	url := f.String("prometheus-url", "", "service base URL override")
	fixed := f.String("now", "", "RFC3339 execution instant")
	if e := f.Parse(args); e != nil || f.NArg() != 0 || o.Config == "" {
		return o, errors.New("invalid CLI options")
	}
	f.Visit(func(v *flag.Flag) {
		if v.Name == "prometheus-url" {
			o.PrometheusURL = url
		}
	})
	if *fixed != "" {
		t, e := time.Parse(time.RFC3339, *fixed)
		if e != nil || strings.Contains(*fixed, ".") {
			return o, errors.New("now must be RFC3339 in whole seconds")
		}
		o.Now = t
	} else {
		o.Now = integerTime(clock())
	}
	return o, nil
}
