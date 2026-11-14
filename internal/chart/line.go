package chart

import (
	"encoding/json"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type lineSpec struct {
	XAxis       *AxisSpec        `json:"x_axis"`
	YAxis       *AxisSpec        `json:"y_axis"`
	Series      []seriesSpec     `json:"series"`
	Bands       []BandSpec       `json:"bands"`
	RefLines    []RefLineSpec    `json:"reference_lines"`
	Annotations []AnnotationSpec `json:"annotations"`
}

func (s *lineSpec) parts() (*AxisSpec, *AxisSpec, []seriesSpec, overlays) {
	return s.XAxis, s.YAxis, s.Series, overlays{s.Bands, s.RefLines, s.Annotations}
}

var lineChart = core.Kind{
	Name:    "line_chart",
	Family:  "chart",
	Use:     "A trend over a continuous x such as time or age, one to eight lines; for a child's measurements against reference centiles use growth_chart, for separate categories bar_chart.",
	Fields:  "series: [{label (required if >1; " + core.UpTo(maxSeriesLabel) + "), points: [[x, y], ...] 1–500 (y null = a gap), dash?: bool}] 1–8; x_axis, y_axis: " + axisFields + "; " + overlayFields + ". Lines are labelled at their right ends (a legend when labels are long); markers show when a series has ≤24 points.",
	Example: `{"type":"line_chart","title":"Haemoglobin in the first months: term and preterm infants","alt":"Two lines of haemoglobin against age. Both fall after birth; the preterm line falls earlier and lower, to about 8 g/dL at 6 to 8 weeks, while the term line reaches about 11 g/dL at 8 to 12 weeks, before both recover.","caption":"Typical values; individual infants vary widely.","spec":{"x_axis":{"label":"Age","unit":"weeks","min":0,"max":24},"y_axis":{"label":"Haemoglobin","unit":"g/dL","min":6,"max":18},"series":[{"label":"Term","points":[[0,17],[2,16],[4,14],[8,11.5],[12,11],[16,11.5],[20,11.8],[24,12]]},{"label":"Preterm","points":[[0,16.5],[2,13.5],[4,10.5],[6,8.5],[8,8],[12,9.3],[16,10.3],[20,11],[24,11.4]]}],"annotations":[{"x":7,"y":8.2,"label":"Earlier, deeper nadir"}]}}`,
	Parse:   parseLine,
}

type lineDrawing struct{ *xyBase }

func parseLine(raw json.RawMessage) (core.Drawing, error) {
	var s lineSpec
	b, _, err := parseXY(raw, &s, s.parts, true, false, axisOpts{tight: true}, axisOpts{})
	if err != nil {
		return nil, err
	}
	return lineDrawing{b}, nil
}

func (d lineDrawing) Draw(c *core.Canvas) error {
	p := newPlot(c, d.x, d.y)
	markers := true
	for _, s := range d.series {
		if len(s.x) > 24 {
			markers = false
		}
	}
	useEnd := endLabelsFit(d.series)
	if useEnd {
		var labels []string
		for _, s := range d.series {
			labels = append(labels, s.label)
		}
		p.rightPad = endLabelWidth(labels) + 12
	} else {
		p.legend = seriesLegend(c, d.series, legLine, markers)
	}
	p.layout(0)
	p.drawBands(d.over.Bands)
	p.drawFrame(0)
	p.drawRefLines(d.over.RefLines)
	var ends []endLabel
	for i, s := range d.series {
		col := c.T.SeriesColour(i)
		dash := seriesDash(i, len(d.series))
		if s.dash {
			dash = "5 4"
		}
		pts := d.pts(s)
		for _, run := range runs(pts) {
			for _, part := range clipPolyline(run, p.r.grow(1)) {
				c.Polyline(part, col, 2.2, dash)
				p.obs.line(part)
			}
			if len(run) == 1 && p.r.grow(1).contains(run[0]) {
				p.obs.point(run[0])
			}
		}
		if markers {
			for _, pt := range pts {
				if finitePt(pt) && p.r.grow(1).contains(pt) {
					drawMarker(c, i, pt.X, pt.Y, 3.2, col)
					p.obs.point(pt)
				}
			}
		}
		if useEnd {
			for j := len(pts) - 1; j >= 0; j-- {
				if finitePt(pts[j]) {
					ends = append(ends, endLabel{text: s.label, y: math.Max(p.r.y0, math.Min(p.r.y1, pts[j].Y)), colour: col})
					break
				}
			}
		}
	}
	p.drawEndLabels(ends, labelSize)
	p.labelOverlays(d.over)
	c.H = p.bottom
	return nil
}
