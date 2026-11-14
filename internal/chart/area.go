package chart

import (
	"encoding/json"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type areaSpec struct {
	XAxis       *AxisSpec        `json:"x_axis"`
	YAxis       *AxisSpec        `json:"y_axis"`
	Series      []seriesSpec     `json:"series"`
	Stacked     bool             `json:"stacked"`
	Bands       []BandSpec       `json:"bands"`
	RefLines    []RefLineSpec    `json:"reference_lines"`
	Annotations []AnnotationSpec `json:"annotations"`
}

func (s *areaSpec) parts() (*AxisSpec, *AxisSpec, []seriesSpec, overlays) {
	return s.XAxis, s.YAxis, s.Series, overlays{s.Bands, s.RefLines, s.Annotations}
}

var areaChart = core.Kind{
	Name:    "area_chart",
	Family:  "chart",
	Use:     "Amounts over a continuous x, filled to zero; stacked to show how parts make up a whole over time (such as the share of feeds by type). For several trends to compare, line_chart reads better.",
	Fields:  "series: [{label (required if >1; " + core.UpTo(maxSeriesLabel) + "), points: [[x, y], ...] 1–500}] 1–8, drawn in order (stacked: the first at the bottom); stacked?: bool (then every series has the same x values, and no negative y); x_axis, y_axis: " + axisFields + "; " + overlayFields + ".",
	Example: `{"type":"area_chart","title":"Where an infant's daily energy comes from, 6 to 12 months","alt":"Stacked areas of daily energy intake from 6 to 12 months of age: milk falls from about 500 to 400 kcal while complementary food rises from about 200 to 550 kcal, so the total grows and food overtakes milk at around 10 to 11 months.","caption":"Approximate averages for a breastfed infant; individual needs vary.","spec":{"stacked":true,"x_axis":{"label":"Age","unit":"months","min":6,"max":12},"y_axis":{"label":"Energy","unit":"kcal/day","min":0,"max":1000},"series":[{"label":"Milk","points":[[6,500],[7,480],[8,460],[9,440],[10,420],[11,410],[12,400]]},{"label":"Food","points":[[6,200],[7,260],[8,320],[9,380],[10,440],[11,500],[12,550]]}]}}`,
	Parse:   parseArea,
}

type areaDrawing struct {
	*xyBase
	stacked bool
	// tops are each series' upper edge, stacked or not; bases its lower.
	tops, bases [][]float64
}

func parseArea(raw json.RawMessage) (core.Drawing, error) {
	var s areaSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	// A stack's y axis must show its totals: find them first, from the
	// series as given (any problem with those is reported below).
	var totals []float64
	if s.Stacked {
		var scratch core.Problems
		ss := checkSeries(&scratch, s.Series, false, false)
		if !scratch.Any() && sameXs(ss) {
			totals = make([]float64, len(ss[0].x))
			for _, se := range ss {
				for j, y := range se.y {
					totals[j] += y
				}
			}
		}
	}
	xa, ya, specs, o := s.parts()
	b, p, err := buildXY(xa, ya, specs, o, false, false, axisOpts{tight: true}, axisOpts{zero: true}, totals)
	if err != nil {
		return nil, err
	}
	ss := b.series
	for i, se := range ss {
		for j := 1; j < len(se.x); j++ {
			if se.x[j] <= se.x[j-1] {
				p.Add(core.Path("series", i, "points", j), "x is %s after %s; an area's points must be in increasing x", fmtValue(se.x[j]), fmtValue(se.x[j-1]))
				break
			}
		}
	}
	if b.y.log {
		p.Add("y_axis.scale", "an area is filled from zero, which a log axis cannot show: use linear")
	}
	if s.Stacked {
		for i := 1; i < len(ss); i++ {
			if len(ss[i].x) != len(ss[0].x) {
				p.Add(core.Path("series", i, "points"), "has %d points but series[0] has %d: stacked areas need the same x values in every series", len(ss[i].x), len(ss[0].x))
				continue
			}
			for j := range ss[i].x {
				if ss[i].x[j] != ss[0].x[j] {
					p.Add(core.Path("series", i, "points", j), "x is %s but series[0] has %s there: stacked areas need the same x values in every series", fmtValue(ss[i].x[j]), fmtValue(ss[0].x[j]))
					break
				}
			}
		}
		for i, se := range ss {
			for j, y := range se.y {
				if y < 0 {
					p.Add(core.Path("series", i, "points", j), "y is %s; stacked areas cannot be negative: set stacked false", fmtValue(y))
				}
			}
		}
		for j, t := range totals {
			if !b.y.in(t) {
				p.Add("y_axis", "the stack reaches %s at x %s, above its max %s: raise max or leave it out", fmtValue(t), fmtValue(ss[0].x[j]), fmtValue(b.y.hi))
				break
			}
		}
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	d := &areaDrawing{xyBase: b, stacked: s.Stacked}
	base := math.Max(0, b.y.lo)
	if b.y.hi < 0 {
		base = b.y.hi
	}
	var run []float64
	for i, se := range ss {
		bs := make([]float64, len(se.y))
		ts := make([]float64, len(se.y))
		for j, y := range se.y {
			if s.Stacked {
				if i == 0 {
					bs[j] = base
				} else {
					bs[j] = run[j]
				}
				ts[j] = bs[j] + y
			} else {
				bs[j], ts[j] = base, y
			}
		}
		run = ts
		d.bases = append(d.bases, bs)
		d.tops = append(d.tops, ts)
	}
	return d, nil
}

func sameXs(ss []series) bool {
	for _, s := range ss[1:] {
		if len(s.x) != len(ss[0].x) {
			return false
		}
		for j := range s.x {
			if s.x[j] != ss[0].x[j] {
				return false
			}
		}
	}
	return true
}

func (d *areaDrawing) Draw(c *core.Canvas) error {
	p := newPlot(c, d.x, d.y)
	useEnd := len(d.series) > 1
	var labels []string
	for _, s := range d.series {
		labels = append(labels, s.label)
	}
	if useEnd && endLabelWidth(labels) <= 92 {
		p.rightPad = endLabelWidth(labels) + 12
	} else if useEnd {
		useEnd = false
		p.legend = seriesLegend(c, d.series, legBox, false)
	}
	p.layout(0)
	p.drawBands(d.over.Bands)
	p.drawFrame(0)
	var ends []endLabel
	tops := make([][]core.Pt, len(d.series))
	// Every fill first, then every line, so no fill covers a line.
	for i, s := range d.series {
		col := c.T.SeriesColour(i)
		top := make([]core.Pt, len(s.x))
		poly := make([]core.Pt, 0, 2*len(s.x))
		for j := range s.x {
			top[j] = core.Pt{X: d.x.px(s.x[j]), Y: d.y.px(d.tops[i][j])}
			poly = append(poly, top[j])
		}
		for j := len(s.x) - 1; j >= 0; j-- {
			poly = append(poly, core.Pt{X: d.x.px(s.x[j]), Y: d.y.px(d.bases[i][j])})
		}
		tops[i] = top
		fill, op := c.T.SeriesSoftColour(i), "0.92"
		if !d.stacked {
			fill, op = col, "0.16"
		}
		if clipped := clipPolygon(poly, p.r); len(clipped) >= 3 {
			c.El("polygon", "points", core.Points(clipped), "fill", fill, "fill-opacity", op)
		}
	}
	for i, s := range d.series {
		col := c.T.SeriesColour(i)
		top := tops[i]
		for _, part := range clipPolyline(top, p.r.grow(1)) {
			c.Polyline(part, col, 2, seriesDash(i, len(d.series)))
			p.obs.line(part)
		}
		if useEnd {
			n := len(s.x) - 1
			y := top[n].Y
			if d.stacked {
				y = (top[n].Y + d.y.px(d.bases[i][n])) / 2
			}
			ends = append(ends, endLabel{text: s.label, y: math.Max(p.r.y0, math.Min(p.r.y1, y)), colour: col})
		}
	}
	p.drawRefLines(d.over.RefLines)
	p.drawEndLabels(ends, labelSize)
	p.labelOverlays(d.over)
	c.H = p.bottom
	return nil
}
