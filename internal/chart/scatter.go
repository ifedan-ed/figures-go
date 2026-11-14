package chart

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type scatterSpec struct {
	XAxis       *AxisSpec        `json:"x_axis"`
	YAxis       *AxisSpec        `json:"y_axis"`
	Series      []seriesSpec     `json:"series"`
	Bands       []BandSpec       `json:"bands"`
	RefLines    []RefLineSpec    `json:"reference_lines"`
	Annotations []AnnotationSpec `json:"annotations"`
}

func (s *scatterSpec) parts() (*AxisSpec, *AxisSpec, []seriesSpec, overlays) {
	return s.XAxis, s.YAxis, s.Series, overlays{s.Bands, s.RefLines, s.Annotations}
}

var scatterChart = core.Kind{
	Name:    "scatter_chart",
	Family:  "chart",
	Use:     "Individual observations of two measures, to show a relationship or clusters, optionally with a least-squares line; for a trend over time use line_chart.",
	Fields:  "series: [{label (required if >1; " + core.UpTo(maxSeriesLabel) + "), points: [[x, y], ...] 1–500, fit?: \"linear\" (a least-squares line, its r shown)}] 1–8; x_axis, y_axis: " + axisFields + "; " + overlayFields + ".",
	Example: `{"type":"scatter_chart","title":"Systolic blood pressure and BMI in adolescents","alt":"Scatter of systolic blood pressure against BMI for twelve adolescents, rising with BMI; the fitted line climbs about 1.5 mmHg per unit of BMI, and the readings above 130 are all at a BMI over 30.","caption":"Illustrative clinic data.","spec":{"x_axis":{"label":"BMI","unit":"kg/m²","min":15,"max":40},"y_axis":{"label":"Systolic BP","unit":"mmHg","min":90,"max":150},"series":[{"label":"Clinic visits","fit":"linear","points":[[18,104],[19.5,110],[21,106],[22,114],[23.5,112],[25,118],[26,113],[28,124],[30,121],[32,133],[34,131],[37,142]]}],"reference_lines":[{"axis":"y","value":130,"label":"Stage 1 (130)","tone":"warn"}]}}`,
	Parse:   parseScatter,
}

type scatterDrawing struct{ *xyBase }

func parseScatter(raw json.RawMessage) (core.Drawing, error) {
	var s scatterSpec
	b, p, err := parseXY(raw, &s, s.parts, false, true, axisOpts{}, axisOpts{})
	if err != nil {
		return nil, err
	}
	for i, se := range b.series {
		if se.fit == "" {
			continue
		}
		if b.x.log || b.y.log {
			p.Add(core.Path("series", i, "fit"), "a linear fit is drawn only on linear axes")
		}
		if _, _, _, ok := leastSquares(se.x, se.y); !ok {
			p.Add(core.Path("series", i, "fit"), "needs at least three points with different x values")
		}
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	return scatterDrawing{b}, nil
}

// leastSquares is the ordinary least-squares line y = a + bx through the
// points, and Pearson's r.
func leastSquares(xs, ys []float64) (a, b, r float64, ok bool) {
	n := float64(len(xs))
	if len(xs) < 3 {
		return 0, 0, 0, false
	}
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= n
	my /= n
	var sxx, sxy, syy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		sxx += dx * dx
		sxy += dx * dy
		syy += dy * dy
	}
	if sxx == 0 {
		return 0, 0, 0, false
	}
	b = sxy / sxx
	a = my - b*mx
	if syy > 0 {
		r = sxy / math.Sqrt(sxx*syy)
	}
	return a, b, r, true
}

func (d scatterDrawing) Draw(c *core.Canvas) error {
	p := newPlot(c, d.x, d.y)
	p.xGrid = true
	p.legend = seriesLegend(c, d.series, legPoint, true)
	// A fitted line's r goes in its series' legend entry; with one series
	// (no legend) it is set beside the line.
	for i, s := range d.series {
		if s.fit == "linear" && len(d.series) > 1 {
			_, _, rr, _ := leastSquares(s.x, s.y)
			p.legend[i].label += " (r = " + fmtSigned(rr, 2) + ")"
		}
	}
	p.layout(0)
	p.drawBands(d.over.Bands)
	p.drawFrame(0)
	p.drawRefLines(d.over.RefLines)
	total := 0
	for _, s := range d.series {
		total += len(s.x)
	}
	r, op := 3.6, ""
	if total > 80 {
		r, op = 2.8, "0.8"
	}
	if total > 250 {
		r, op = 2.2, "0.7"
	}
	type fitLabel struct {
		at   core.Pt
		text string
		col  string
	}
	var fits []fitLabel
	for i, s := range d.series {
		col := c.T.SeriesColour(i)
		if s.fit == "linear" {
			a, b, rr, _ := leastSquares(s.x, s.y)
			x0, x1 := extent(s.x)
			line := []core.Pt{{X: d.x.px(x0), Y: d.y.px(a + b*x0)}, {X: d.x.px(x1), Y: d.y.px(a + b*x1)}}
			for _, part := range clipPolyline(line, p.r) {
				c.Polyline(part, col, 1.8, "6 4")
				p.obs.line(part)
				if len(d.series) == 1 {
					fits = append(fits, fitLabel{part[len(part)-1], fmt.Sprintf("r = %s", fmtSigned(rr, 2)), col})
				}
			}
		}
	}
	for i, s := range d.series {
		col := c.T.SeriesColour(i)
		if op != "" {
			c.Open("g", "fill-opacity", op)
		}
		for _, pt := range d.pts(s) {
			drawMarker(c, i, pt.X, pt.Y, r, col)
			p.obs.point(pt)
		}
		if op != "" {
			c.Close("g")
		}
	}
	for _, f := range fits {
		p.annotate(f.at, f.text, f.col, false, false)
	}
	p.labelOverlays(d.over)
	c.H = p.bottom
	return nil
}

// fmtSigned writes v to dec decimals with a true minus sign.
func fmtSigned(v float64, dec int) string {
	s := fmt.Sprintf("%.*f", dec, math.Abs(v))
	if v < 0 && s != fmt.Sprintf("%.*f", dec, 0.0) {
		return "−" + s
	}
	return s
}
