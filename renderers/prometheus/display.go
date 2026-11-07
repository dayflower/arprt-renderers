package prometheus

import (
	_ "embed"
	"fmt"
	"html"
	"math"
	"sort"
	"strings"
	"time"
)

func esc(s string) string { return html.EscapeString(s) }

type ink struct {
	color, dash, fill string
	width             int
}

func seriesInk(style, palette string, index int) ink {
	k := ink{"#000", "", "#000", 3}
	if palette == "spectra6" {
		k.color = map[string]string{"primary": "#0000ff", "secondary": "#ff0000", "tertiary": "#008000", "positive": "#008000", "negative": "#ff0000", "muted": "#aaaa00"}[style]
		if k.color == "" {
			k.color = "#0000ff"
		}
	} else {
		switch style {
		case "secondary":
			k.dash = "8 5"
		case "tertiary":
			k.color = "#555555"
			k.width = 2
		case "muted":
			k.color = "#aaaaaa"
		}
	}
	k.fill = k.color
	if style == "negative" && palette == "gray4" {
		k.fill = "white"
	}
	// Query-returned series can share logical styles; geometry keeps all eight
	// distinguishable even after palette conversion.
	if index > 0 {
		k.dash = []string{"", "8 5", "2 4", "12 4 2 4", "5 3", "14 6", "2 3 7 3", "10 3 2 3 2 3"}[index%8]
	}
	return k
}

// RenderHTML accepts a validated configuration/context and its normalized model.
// Only renderer-owned markup/scripts are emitted; user text is always escaped.
func RenderHTML(c Config, target Context, m Model) ([]byte, error) {
	w, h := target.LogicalSize()
	g := c.Layout.Landscape
	if target.Rotation%180 != 0 {
		g = *c.Layout.Portrait
	}
	cols, _ := tracks(g.Columns, len(cells(g)[0]))
	rows, _ := tracks(g.Rows, len(g.Areas))
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><style>%s html,body{width:%dpx;height:%dpx}.grid{grid-template-columns:%s;grid-template-rows:%s;gap:%dpx;grid-template-areas:`, displayCSS, w, h, strings.Join(cols, " "), strings.Join(rows, " "), g.Gap)
	for _, row := range cells(g) {
		b.WriteString(`"`)
		for _, id := range row {
			if id != "." {
				id = "p-" + id
			}
			b.WriteString(id + " ")
		}
		b.WriteString(`" `)
	}
	b.WriteString(`}</style></head><body><main class="grid">`)
	byID := map[string]PanelModel{}
	for _, p := range m.Panels {
		byID[p.ID] = p
	}
	loc, _ := time.LoadLocation(c.Timezone)
	now := time.Unix(m.Now, 0).In(loc)
	var draw func(Panel, bool) error
	draw = func(p Panel, child bool) error {
		style := "grid-area:p-" + p.ID
		if child {
			basis := "0px"
			style = fmt.Sprintf("flex:%d 1 %s", p.Grow, basis)
		}
		class := "panel"
		if p.Type == "group" {
			class += " group"
			gap := g.Gap
			if p.Gap != nil {
				gap = *p.Gap
			}
			wrap := "nowrap"
			if p.Wrap {
				wrap = "wrap"
			}
			style += fmt.Sprintf(";flex-direction:%s;flex-wrap:%s;gap:%dpx", p.Direction, wrap, gap)
		}
		fmt.Fprintf(&b, `<section class="%s" id="p-%s" style="%s" data-type="%s">`, class, p.ID, style, p.Type)
		pm := byID[p.ID]
		switch p.Type {
		case "group":
			for _, ch := range p.Panels {
				if err := draw(ch, true); err != nil {
					return err
				}
			}
		case "text":
			text, err := FormatText(p.Text, now)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, `<div class="text %s %s" style="text-align:%s;justify-content:%s">`, p.Size, p.Weight, cssAlign(p.Align), cssAlign(p.VerticalAlign))
			for _, line := range strings.Split(text, "\n") {
				fmt.Fprintf(&b, `<div class="ellipsis">%s</div>`, line)
			}
			b.WriteString(`</div>`)
		case "value":
			label, value, stale := p.Title, "—", false
			if pm.Value != nil {
				label = pm.Value.Label
				v := pm.Value.Number
				if p.SignLabels != nil && p.SignLabels.Abs {
					v = math.Abs(v)
				}
				value = displayNumber(v, p.Decimals, p.Prefix, p.Suffix)
				stale = pm.Value.Stale
			}
			fmt.Fprintf(&b, `<h2 class="ellipsis">%s</h2><div class="number%s"><span class="ellipsis">%s</span></div>`, esc(label), map[bool]string{true: " stale"}[stale], esc(value))
			if stale {
				b.WriteString(`<div class="status">! Stale</div>`)
			}
			if pm.Value == nil {
				b.WriteString(`<div class="status">No data</div>`)
			}
		case "line", "bar":
			fmt.Fprintf(&b, `<h2 class="ellipsis">%s</h2>`, esc(p.Title))
			b.WriteString(chartSVG(p, pm, target.Target.Palette, loc))
		}
		b.WriteString(`</section>`)
		return nil
	}
	for _, p := range c.Panels {
		if err := draw(p, false); err != nil {
			return nil, err
		}
	}
	b.WriteString(`</main><script>` + readinessJS + `</script></body></html>`)
	if b.Len() > HTMLLimit {
		return nil, Diagnostic{Code: "html_limit"}
	}
	return []byte(b.String()), nil
}
func cssAlign(s string) string {
	switch s {
	case "center":
		return "center"
	case "end":
		return "end"
	default:
		return "start"
	}
}

//go:embed display.css
var displayCSS string

// Readiness fails closed when a layout cannot fit readable content. Ellipsized
// plain text and labels are intentional; geometric clipping is never readiness.
//
//go:embed display.js
var readinessJS string

func chartDomain(p Panel, m PanelModel) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	add := func(v float64) { lo = math.Min(lo, v); hi = math.Max(hi, v) }
	for _, s := range m.Series {
		for _, q := range s.Points {
			if q.Present && q.StackKnown {
				add(q.Top)
				if p.Stack || p.Type == "bar" {
					add(q.Base)
				}
			}
		}
	}
	for _, r := range m.References {
		add(r.Value)
	}
	if !finite(lo) {
		lo, hi = 0, 1
	}
	if p.YAxis != nil {
		if p.YAxis.Min != nil {
			lo = *p.YAxis.Min
		}
		if p.YAxis.Max != nil {
			hi = *p.YAxis.Max
		}
	}
	if lo >= hi {
		if p.YAxis != nil && p.YAxis.Min != nil {
			hi = lo + math.Max(1, math.Abs(lo)*.1)
		} else if p.YAxis != nil && p.YAxis.Max != nil {
			lo = hi - math.Max(1, math.Abs(hi)*.1)
		} else if lo > 0 {
			lo *= .9
		} else if hi < 0 {
			hi *= .9
		} else {
			lo, hi = -1, 1
		}
	}
	return lo, hi
}

// niceTicks calculates in scaled units so extreme, opposite-sign finite samples
// cannot overflow an intermediate range subtraction.
func niceTicks(lo, hi float64) []float64 {
	scale := math.Max(math.Abs(lo), math.Abs(hi))
	if scale == 0 {
		scale = 1
	}
	a, z := lo/scale, hi/scale
	rawLog := math.Log10(z-a) + math.Log10(scale) - math.Log10(4)
	power := math.Pow(10, math.Floor(rawLog))
	if power == 0 {
		power = math.SmallestNonzeroFloat64
	}
	step := power / scale
	best := math.Inf(1)
	for _, factor := range []float64{.1, .2, .5, 1, 2, 5, 10, 20, 50} {
		candidate := power / scale * factor
		if candidate == 0 || !finite(candidate) {
			continue
		}
		count := math.Floor(z/candidate+1e-9) - math.Ceil(a/candidate-1e-9) + 1
		penalty := math.Abs(count - 4)
		if count < 3 || count > 5 {
			penalty += 10
		}
		if penalty < best {
			best = penalty
			step = candidate
		}
	}
	out := []float64{}
	for n := math.Ceil(a / step); n*step <= z+step*1e-8 && len(out) < 6; n++ {
		v := n * step * scale
		if finite(v) {
			out = append(out, v)
		}
	}
	return out
}

func chartSVG(p Panel, m PanelModel, palette string, loc *time.Location) string {
	var b strings.Builder
	lo, hi := chartDomain(p, m)
	scale := math.Max(math.Abs(lo), math.Abs(hi))
	if scale == 0 {
		scale = 1
	}
	y := func(v float64) float64 { return 1000 * (1 - (v/scale-lo/scale)/(hi/scale-lo/scale)) }
	// Fixed-pixel labels surround a nested normalized plot. No text is stretched
	// with its data; the actual plot width is exactly the sampling reserve of 96px.
	b.WriteString(`<svg class="chart" xmlns="http://www.w3.org/2000/svg" role="img"><title>` + esc(p.Title) + `</title><svg class="plot" x="48" y="18" width="calc(100% - 96px)" height="calc(100% - 60px)" viewBox="0 0 1000 1000" preserveAspectRatio="none">`)
	for _, v := range niceTicks(lo, hi) {
		fmt.Fprintf(&b, `<path d="M0 %.4f H1000" stroke="#aaa" fill="none" vector-effect="non-scaling-stroke"/>`, y(v))
	}
	if lo <= 0 && hi >= 0 {
		fmt.Fprintf(&b, `<path class="zero" d="M0 %.4f H1000" stroke="#000" fill="none" vector-effect="non-scaling-stroke"/>`, y(0))
	}
	for _, r := range m.References {
		k := seriesInk(r.Style, palette, 0)
		fmt.Fprintf(&b, `<path class="reference" d="M0 %.4f H1000" stroke="%s" stroke-width="2" stroke-dasharray="6 4" vector-effect="non-scaling-stroke"><title>%s</title></path>`, y(r.Value), k.color, esc(r.Label))
	}
	n := len(m.Buckets)
	for _, s := range m.Series {
		n = max(n, len(s.Points))
	}
	x := func(i int) float64 {
		if p.Type == "bar" {
			return (float64(i) + .5) * 1000 / float64(max(1, n))
		}
		return float64(i) * 1000 / float64(max(1, n-1))
	}
	for si, s := range m.Series {
		k := seriesInk(s.Style, palette, si)
		if p.Type == "bar" && palette == "gray4" && s.Style != "positive" && s.Style != "negative" {
			k.fill = []string{"#000", "white", "#555555", "#aaaaaa"}[si%4]
		}
		fmt.Fprintf(&b, `<g class="series" data-series="%d" stroke="%s" stroke-width="%d" stroke-dasharray="%s" fill="none">`, si, k.color, k.width, k.dash)
		if p.Type == "line" {
			for _, seg := range s.Segments {
				var path strings.Builder
				for i := seg.Start; i < seg.End; i++ {
					cmd := "L"
					if i == seg.Start {
						cmd = "M"
					}
					fmt.Fprintf(&path, "%s%.4f %.4f ", cmd, x(i), y(s.Points[i].Top))
				}
				fmt.Fprintf(&b, `<path class="segment" d="%s" vector-effect="non-scaling-stroke"/>`, path.String())
				if p.Stack && seg.End-seg.Start > 1 {
					for i := seg.End - 1; i >= seg.Start; i-- {
						fmt.Fprintf(&path, "L%.4f %.4f ", x(i), y(s.Points[i].Base))
					}
					fmt.Fprintf(&b, `<path class="stack-area" d="%sZ" fill="%s" stroke="%s" vector-effect="non-scaling-stroke"/>`, path.String(), k.fill, k.color)
				}
			}
			// Marker coordinates use non-scaling strokes, keeping isolated points visible.
			for i, q := range s.Points {
				if q.Present && q.StackKnown && (len(s.Points) == 1 || ((si > 0 || s.Style == "tertiary") && i%max(1, len(s.Points)/8) == 0) || isolated(s.Points, i)) {
					fmt.Fprintf(&b, `<path class="marker" d="M%.4f %.4f h.01" stroke-width="%d" stroke-linecap="%s" vector-effect="non-scaling-stroke"/>`, x(i), y(q.Top), 5+si%3, []string{"round", "square"}[si%2])
				}
			}
		} else {
			bw := 800 / float64(max(1, n))
			if !p.Stack {
				bw /= float64(max(1, len(m.Series)))
			}
			for i, q := range s.Points {
				if !q.Present || !q.StackKnown {
					continue
				}
				bx := x(i) - 400/float64(max(1, n))
				if !p.Stack {
					bx += float64(si) * bw
				}
				top, base := y(q.Top), y(q.Base)
				width := bw
				if !p.Stack {
					width *= .85
				}
				fmt.Fprintf(&b, `<rect class="bar" x="%.4f" y="%.4f" width="%.4f" height="%.4f" fill="%s" vector-effect="non-scaling-stroke"/>`, bx, math.Min(top, base), width, math.Max(.5, math.Abs(top-base)), k.fill)
				if si >= 4 && math.Abs(top-base) > 20 {
					marker := "#000"
					if k.fill == "#000" || k.fill == "#555555" {
						marker = "white"
					}
					fmt.Fprintf(&b, `<path class="bar-marker" d="M%.4f %.4f h.01" stroke="%s" stroke-width="%d" stroke-linecap="%s" stroke-dasharray="none" vector-effect="non-scaling-stroke"/>`, bx+width/2, (top+base)/2, marker, 4+si%2, []string{"round", "square"}[si%2])
				}
			}
		}
		b.WriteString(`</g>`)
	}
	for i, bucket := range m.Buckets {
		if bucket.Partial {
			fmt.Fprintf(&b, `<rect class="partial" x="%.4f" y="0" width="%.4f" height="1000" fill="none" stroke="#000" stroke-width="2" vector-effect="non-scaling-stroke"/>`, float64(i)*1000/float64(max(1, n)), 1000/float64(max(1, n)))
		}
	}
	b.WriteString(`</svg>`)
	for i := range m.Buckets {
		available := false
		for _, series := range m.Series {
			if i < len(series.Points) && series.Points[i].Present && series.Points[i].StackKnown {
				available = true
			}
		}
		if !available {
			fmt.Fprintf(&b, `<text class="missing" x="0" data-xfactor="%.6f" y="32" text-anchor="middle">—</text>`, (float64(i)+.5)/float64(max(1, len(m.Buckets))))
		}
	}
	// Outer label y positions are expressed using the same plot height in CSS.
	label := func(class, text string, xpos string, yp float64, maxWidth int, anchor string) {
		fmt.Fprintf(&b, `<text class="%s" x="%s" y="0" data-yfactor="%.6f" dominant-baseline="middle" text-anchor="%s" data-max="%d">%s</text>`, class, xpos, yp, anchor, maxWidth, esc(text))
	}
	for _, v := range niceTicks(lo, hi) {
		label("axis-label", displayNumber(v, p.Decimals, p.Prefix, p.Suffix), "44", y(v)/1000, 43, "end")
	}
	type direct struct {
		value float64
		name  string
		ink   ink
	}
	labels := []direct{}
	for si, s := range m.Series {
		for i := len(s.Points) - 1; i >= 0; i-- {
			q := s.Points[i]
			if q.Present && q.StackKnown {
				labels = append(labels, direct{math.Max(0, math.Min(1, y(q.Top)/1000)), fmt.Sprintf("%d %s", si+1, s.Legend), seriesInk(s.Style, palette, si)})
				break
			}
		}
	}
	for _, r := range m.References {
		if r.Value >= lo && r.Value <= hi && r.Label != "" {
			labels = append(labels, direct{y(r.Value) / 1000, r.Label, seriesInk(r.Style, palette, 0)})
		}
	}
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].value < labels[j].value })
	// Store desired positions; readiness resolves collisions using the measured
	// plot height and rejects a chart too short for its direct labels.
	for _, d := range labels {
		fmt.Fprintf(&b, `<text class="direct-label" fill="%s" x="calc(100%% - 44px)" y="0" data-y="%.6f" data-max="43">%s</text>`, d.ink.color, d.value, esc(d.name))
	}
	if m.Missing {
		b.WriteString(`<text class="missing" x="50%" y="50%" text-anchor="middle">No data</text>`)
	}
	if p.Type == "bar" {
		for i, bucket := range m.Buckets {
			text := bucket.Label
			if p.Buckets.Unit == "day" {
				text = time.Unix(bucket.Start, 0).In(loc).Format("01/02")
			}
			if p.Buckets.Unit == "hour" {
				text = time.Unix(bucket.Start, 0).In(loc).Format("15:04 -0700")
			}
			if bucket.Partial {
				text += "*"
			}
			fmt.Fprintf(&b, `<text class="x-label" x="0" data-xfactor="%.6f" y="0" data-bottom="24" text-anchor="middle" data-bucket="%d">%s</text>`, (float64(i)+.5)/float64(max(1, len(m.Buckets))), i, esc(text))
		}
		b.WriteString(`<text class="x-label" x="48" y="0" data-bottom="5">` + esc(loc.String()) + ` · * partial</text>`)
	} else if n > 0 {
		start, end := m.Series[0].Points[0].Time, m.Series[0].Points[n-1].Time
		interval := int64(3 * 3600)
		if end-start > 24*3600 {
			interval = 24 * 3600
		} else if end-start > 12*3600 {
			interval = 6 * 3600
		}
		times := []int64{}
		first := start - start%60
		if first < start {
			first += 60
		}
		for t := first; t <= end; t += 60 {
			local := time.Unix(t, 0).In(loc)
			if local.Minute() == 0 && local.Second() == 0 && (int64(local.Hour())*3600)%interval == 0 {
				times = append(times, t)
			}
		}
		if len(times) == 0 {
			times = []int64{start}
		}
		for _, t := range times {
			fraction := float64(t-start) / float64(max(int64(1), end-start))
			text := time.Unix(t, 0).In(loc).Format("15:04")
			fmt.Fprintf(&b, `<text class="x-label" x="0" data-xfactor="%.6f" y="0" data-bottom="24" text-anchor="middle">%s</text>`, fraction, esc(text))
		}
		b.WriteString(`<text class="x-label" x="48" y="0" data-bottom="5">` + esc(time.Unix(end, 0).In(loc).Format("01/02 MST -0700")) + `</text>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
func isolated(points []Point, i int) bool {
	return (i == 0 || !points[i-1].Present || !points[i-1].StackKnown) && (i == len(points)-1 || !points[i+1].Present || !points[i+1].StackKnown)
}

func displayNumber(v float64, decimals int, prefix, suffix string) string {
	raw := FormatNumber(v, decimals, "", "")
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = raw[1:]
	}
	parts := strings.SplitN(raw, ".", 2)
	digits := parts[0]
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(sign)
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if len(parts) == 2 {
		b.WriteByte('.')
		b.WriteString(parts[1])
	}
	b.WriteString(suffix)
	return b.String()
}
