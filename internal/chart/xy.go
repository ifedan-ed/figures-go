package chart

import (
	"encoding/json"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Limits shared by the x–y charts.
const (
	maxSeries      = 8
	maxPoints      = 500
	maxSeriesLabel = 28
)

// seriesSpec is one line, area or set of points.
type seriesSpec struct {
	Label  string       `json:"label"`
	Points [][]*float64 `json:"points"`
	Dash   bool         `json:"dash"`
	Fit    string       `json:"fit"`
}

// series is a checked series: its points, NaN y where a line has a gap.
type series struct {
	label string
	x, y  []float64
	dash  bool
	fit   string
}

// checkSeries checks a chart's series: their count, labels and points.
// gaps allows a null y (a break in a line); fit allows a fitted line.
func checkSeries(p *core.Problems, specs []seriesSpec, gaps, fit bool) []series {
	p.Count("series", len(specs), 1, maxSeries)
	out := make([]series, 0, len(specs))
	seen := map[string]bool{}
	for i, s := range specs {
		path := core.Path("series", i)
		label := p.Label(core.Path(path, "label"), s.Label, len(specs) > 1, maxSeriesLabel)
		if label != "" && seen[label] {
			p.Add(core.Path(path, "label"), "%q is used by two series; each series needs its own label", label)
		}
		seen[label] = true
		if !fit && s.Fit != "" {
			p.Add(core.Path(path, "fit"), "is not available on this chart; it is for scatter_chart")
		} else if fit {
			p.OneOf(core.Path(path, "fit"), s.Fit, true, "linear")
		}
		pp := core.Path(path, "points")
		p.Count(pp, len(s.Points), 1, maxPoints)
		se := series{label: label, dash: s.Dash, fit: s.Fit}
		for j, pt := range s.Points {
			if j >= maxPoints {
				break
			}
			at := core.Path(pp, j)
			if len(pt) != 2 {
				p.Add(at, "must be [x, y], two numbers; it has %d", len(pt))
				continue
			}
			if pt[0] == nil {
				p.Add(at, "x is null; every point needs an x")
				continue
			}
			y := math.NaN()
			if pt[1] == nil {
				if !gaps {
					p.Add(at, "y is null; every point needs a y (leave the point out instead)")
					continue
				}
			} else {
				y = *pt[1]
			}
			se.x = append(se.x, *pt[0])
			se.y = append(se.y, y)
		}
		out = append(out, se)
	}
	return out
}

func allX(ss []series) []float64 {
	var v []float64
	for _, s := range ss {
		v = append(v, s.x...)
	}
	return v
}

func allY(ss []series) []float64 {
	var v []float64
	for _, s := range ss {
		v = append(v, s.y...)
	}
	return v
}

// checkPointsInAxes refuses points outside axes the spec fixed.
func checkPointsInAxes(p *core.Problems, ss []series, x, y *axis) {
	for i, s := range ss {
		for j := range s.x {
			at := core.Path("series", i, "points", j)
			if x.log && s.x[j] <= 0 || !x.in(s.x[j]) {
				x.checkIn(p, at, "x_axis", s.x[j])
			}
			if !math.IsNaN(s.y[j]) && (y.log && s.y[j] <= 0 || !y.in(s.y[j])) {
				y.checkIn(p, at, "y_axis", s.y[j])
			}
		}
	}
}

// xyBase is what the line, area and scatter specs share.
type xyBase struct {
	x, y   *axis
	series []series
	over   overlays
}

// parseXY reads and checks an x–y chart's spec into v (whose fields the
// chart declares) and its shared parts.
func parseXY(raw json.RawMessage, v any, get func() (xa, ya *AxisSpec, ss []seriesSpec, o overlays), gaps, fit bool, xo, yo axisOpts) (*xyBase, *core.Problems, error) {
	if err := core.Decode(raw, v); err != nil {
		return nil, nil, err
	}
	xa, ya, specs, o := get()
	return buildXY(xa, ya, specs, o, gaps, fit, xo, yo, nil)
}

// buildXY checks a decoded x–y spec. extraY are values the y axis must
// show besides the points (a stack's totals).
func buildXY(xa, ya *AxisSpec, specs []seriesSpec, o overlays, gaps, fit bool, xopts, yo axisOpts, extraY []float64) (*xyBase, *core.Problems, error) {
	var p core.Problems
	ss := checkSeries(&p, specs, gaps, fit)
	ox, oy := o.check(&p)
	if p.Any() {
		return nil, &p, p.Err()
	}
	xs := append(allX(ss), ox...)
	ys := append(append(allY(ss), oy...), extraY...)
	xopts.name = "x_axis"
	if xa != nil && xa.Scale == "log" {
		xs = positiveOrAll(xs)
	}
	if ya != nil && ya.Scale == "log" {
		ys = positiveOrAll(ys)
	}
	xlo, xhi := extent(xs)
	ylo, yhi := extent(ys)
	b := &xyBase{series: ss, over: o}
	b.x = resolveAxis(&p, "x_axis", xa, xlo, xhi, xopts)
	yo.name = "y_axis"
	b.y = resolveAxis(&p, "y_axis", ya, ylo, yhi, yo)
	if p.Any() {
		return nil, &p, p.Err()
	}
	checkPointsInAxes(&p, ss, b.x, b.y)
	o.checkInAxes(&p, b.x, b.y)
	return b, &p, p.Err()
}

// positiveOrAll keeps the positive values when there are any, so a log
// axis's range is found from them and the others are refused by name.
func positiveOrAll(vs []float64) []float64 {
	if pos := positive(vs); len(pos) > 0 && len(pos) < len(vs) {
		// Keep one non-positive value so resolveAxis says what is wrong.
		for _, v := range vs {
			if v <= 0 {
				return append(pos, v)
			}
		}
	}
	return vs
}

// pts is a series's points in figure units, NaN where y is.
func (b *xyBase) pts(s series) []core.Pt {
	out := make([]core.Pt, len(s.x))
	for i := range s.x {
		y := math.NaN()
		if !math.IsNaN(s.y[i]) {
			y = b.y.px(s.y[i])
		}
		out[i] = core.Pt{X: b.x.px(s.x[i]), Y: y}
	}
	return out
}

// runs splits points at their gaps (NaN), each run drawn as one line.
func runs(pts []core.Pt) [][]core.Pt {
	var out [][]core.Pt
	var run []core.Pt
	for _, p := range pts {
		if !finitePt(p) {
			if len(run) > 0 {
				out = append(out, run)
			}
			run = nil
			continue
		}
		run = append(run, p)
	}
	if len(run) > 0 {
		out = append(out, run)
	}
	return out
}

// seriesLegend is the legend's items for series drawn as kind.
func seriesLegend(c *core.Canvas, ss []series, kind int, markers bool) []legendItem {
	var items []legendItem
	for i, s := range ss {
		it := legendItem{label: s.label, kind: kind, colour: c.T.SeriesColour(i), fill: c.T.SeriesSoftColour(i), shape: -1}
		it.dash = seriesDash(i, len(ss))
		if s.dash {
			it.dash = "5 4"
		}
		if markers || kind == legPoint {
			it.shape = i
		}
		items = append(items, it)
	}
	return items
}

// endLabelsFit says whether the series' labels are short enough to set at
// the ends of their lines rather than in a legend.
func endLabelsFit(ss []series) bool {
	if len(ss) < 2 {
		return false
	}
	var labels []string
	for _, s := range ss {
		labels = append(labels, s.label)
	}
	return endLabelWidth(labels) <= 92
}
