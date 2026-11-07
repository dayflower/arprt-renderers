package prometheus

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var trackPattern = regexp.MustCompile(`^(auto|([0-9]+(\.[0-9]+)?)fr|[0-9]+px)$`)

func tracks(raw string, n int) ([]string, error) {
	if raw == "" {
		return strings.Fields(strings.Repeat("1fr ", n)), nil
	}
	t := strings.Fields(raw)
	if len(t) != n {
		return nil, errors.New("track count mismatch")
	}
	for _, x := range t {
		if !trackPattern.MatchString(x) {
			return nil, errors.New("invalid track grammar")
		}
		if x != "auto" {
			f, e := strconv.ParseFloat(x[:len(x)-2], 64)
			if e != nil || f <= 0 || f > 800 || math.IsInf(f, 0) {
				return nil, errors.New("track outside bounds")
			}
		}
	}
	return t, nil
}

func cells(g Grid) [][]string {
	r := make([][]string, len(g.Areas))
	for i, s := range g.Areas {
		r[i] = strings.Fields(s)
	}
	return r
}
func validateGrid(g Grid, ps []Panel) error {
	a := cells(g)
	if len(a) == 0 || len(a) > 32 || len(a[0]) == 0 || len(a[0]) > 16 {
		return errors.New("invalid grid dimensions")
	}
	if _, e := tracks(g.Columns, len(a[0])); e != nil {
		return e
	}
	if _, e := tracks(g.Rows, len(a)); e != nil {
		return e
	}
	top := map[string]bool{}
	for _, p := range ps {
		top[p.ID] = true
	}
	type box struct{ r0, r1, c0, c1, n int }
	boxes := map[string]*box{}
	for r, row := range a {
		if len(row) != len(a[0]) {
			return errors.New("ragged grid")
		}
		for c, id := range row {
			if id == "." {
				continue
			}
			if !top[id] {
				return errors.New("unknown grid area")
			}
			b := boxes[id]
			if b == nil {
				b = &box{r, r, c, c, 0}
				boxes[id] = b
			}
			b.r1 = r
			b.c0 = min(b.c0, c)
			b.c1 = max(b.c1, c)
			b.n++
		}
	}
	if len(boxes) != len(top) {
		return errors.New("unused panel")
	}
	for _, b := range boxes {
		if b.n != (b.r1-b.r0+1)*(b.c1-b.c0+1) {
			return errors.New("nonrectangular grid area")
		}
	}
	return nil
}
func portrait(g Grid) Grid {
	seen := map[string]bool{}
	out := Grid{Columns: "1fr", Gap: g.Gap}
	for _, row := range cells(g) {
		for _, id := range row {
			if id != "." && !seen[id] {
				seen[id] = true
				out.Areas = append(out.Areas, id)
			}
		}
	}
	out.Rows = strings.TrimSpace(strings.Repeat("auto ", len(out.Areas)-1) + "1fr")
	return out
}

// PlotWidths reserves 96 px for axes/direct labels. Auto columns and wrapping
// groups use a single sample interval, avoiding guesses about text measurement.
// The future browser layout uses min-width:0 and a fixed 120 px wrap basis.
func PlotWidths(c Config, ctx Context) map[string]int {
	w, _ := ctx.LogicalSize()
	g := c.Layout.Landscape
	if ctx.Rotation == 90 || ctx.Rotation == 270 {
		g = *c.Layout.Portrait
	}
	a := cells(g)
	ts, _ := tracks(g.Columns, len(a[0]))
	widths := make([]float64, len(ts))
	free := float64(w - (len(ts)-1)*g.Gap)
	total := 0.0
	auto := false
	for i, t := range ts {
		if t == "auto" {
			auto = true
			continue
		}
		v, _ := strconv.ParseFloat(t[:len(t)-2], 64)
		if strings.HasSuffix(t, "px") {
			widths[i] = v
			free -= v
		} else {
			total += v
		}
	}
	if !auto && total > 0 {
		for i, t := range ts {
			if strings.HasSuffix(t, "fr") {
				v, _ := strconv.ParseFloat(t[:len(t)-2], 64)
				widths[i] = math.Max(0, free) * v / total
			}
		}
	}
	out := map[string]int{}
	var assign func(Panel, float64)
	assign = func(p Panel, width float64) {
		if p.Type != "group" {
			out[p.ID] = max(1, int(math.Floor(width))-96)
			return
		}
		gap := g.Gap
		if p.Gap != nil {
			gap = *p.Gap
		}
		sum := 0
		for _, ch := range p.Panels {
			sum += ch.Grow
		}
		for _, ch := range p.Panels {
			cw := width
			if p.Wrap {
				cw = 0
			} else if p.Direction == "row" {
				cw = math.Max(0, width-float64(gap*(len(p.Panels)-1))) * float64(ch.Grow) / float64(sum)
			}
			assign(ch, cw)
		}
	}
	for _, p := range c.Panels {
		lo, hi := len(ts), -1
		for _, row := range a {
			for i, id := range row {
				if id == p.ID {
					lo = min(lo, i)
					hi = max(hi, i)
				}
			}
		}
		width := 0.0
		if !auto {
			width = float64((hi - lo) * g.Gap)
			for i := lo; i <= hi; i++ {
				width += widths[i]
			}
		}
		assign(p, width)
	}
	return out
}
