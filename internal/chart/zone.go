package chart

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type zoneBoundarySpec struct {
	Label  string      `json:"label"`
	Points [][]float64 `json:"points"`
}

type zoneSpec struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

type zonePointSpec struct {
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
	Label string   `json:"label"`
}

type zoneChartSpec struct {
	XAxis      *AxisSpec          `json:"x_axis"`
	YAxis      *AxisSpec          `json:"y_axis"`
	Boundaries []zoneBoundarySpec `json:"boundaries"`
	Zones      []zoneSpec         `json:"zones"`
	Points     []zonePointSpec    `json:"points"`
}

const (
	zoneBoundChars = 16
	zoneLabelChars = 24
)

var zoneChart = core.Kind{
	Name:    "zone_chart",
	Family:  "clinical chart",
	Use:     "Zones between threshold lines or curves (bilirubin nomogram zones or phototherapy thresholds by hours of age, asthma action plan zones), with a patient's values plotted and the zone of each named. The thresholds come from your sources, never from memory or from this example; cite them in the caption.",
	Fields:  "boundaries: [{label? (" + core.UpTo(zoneBoundChars) + "), points: [[x, y], ...] 2–100 in increasing x}] 1–6, listed lowest to highest, all over the same x range and never crossing; zones: [{label (" + core.UpTo(zoneLabelChars) + "), tone?}] one more than boundaries, lowest first (below the first boundary, between each pair, above the last); points?: [{x, y, label? (" + core.UpTo(zoneBoundChars) + ")}] 0–8, the patient's values in order, each within the boundaries' x range; x_axis, y_axis: " + axisFields + ".",
	Example: `{"type":"zone_chart","title":"Peak flow by asthma action plan zone","alt":"Peak flow as a percentage of personal best over five days: green zone at 80% or more, yellow 50 to 79%, red below 50%; the readings fell into the yellow zone on day 2 and recovered to green by day 5.","caption":"Zones as in the child's asthma action plan.","spec":{"x_axis":{"label":"Day"},"y_axis":{"label":"Peak flow","unit":"% of best","min":0,"max":110},"boundaries":[{"label":"50%","points":[[1,50],[5,50]]},{"label":"80%","points":[[1,80],[5,80]]}],"zones":[{"label":"Red: get help now","tone":"bad"},{"label":"Yellow: use reliever","tone":"warn"},{"label":"Green: doing well","tone":"good"}],"points":[{"x":1,"y":88},{"x":2,"y":64,"label":"Cold"},{"x":3,"y":71},{"x":4,"y":79},{"x":5,"y":86}]}}`,
	Parse:   parseZone,
}

type zoneBoundary struct {
	label string
	xs    []float64
	ys    []float64
}

// at is the boundary's value at x, linear between its points.
func (b zoneBoundary) at(x float64) float64 {
	i := sort.SearchFloat64s(b.xs, x)
	switch {
	case i <= 0:
		return b.ys[0]
	case i >= len(b.xs):
		return b.ys[len(b.ys)-1]
	}
	x0, x1 := b.xs[i-1], b.xs[i]
	t := (x - x0) / (x1 - x0)
	return b.ys[i-1] + t*(b.ys[i]-b.ys[i-1])
}

type zonePoint struct {
	x, y  float64
	label string
	zone  int
}

type zoneDrawing struct {
	x, y     *axis
	bounds   []zoneBoundary
	zones    []zoneSpec
	points   []zonePoint
	xLo, xHi float64
}

func parseZone(raw json.RawMessage) (core.Drawing, error) {
	var s zoneChartSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	d := &zoneDrawing{}
	p.Count("boundaries", len(s.Boundaries), 1, 6)
	var xs, ys []float64
	for i, b := range s.Boundaries {
		path := core.Path("boundaries", i)
		zb := zoneBoundary{label: p.Label(core.Path(path, "label"), b.Label, false, zoneBoundChars)}
		p.Count(core.Path(path, "points"), len(b.Points), 2, 100)
		for j, pt := range b.Points {
			at := core.Path(path, "points", j)
			if len(pt) != 2 {
				p.Add(at, "must be [x, y]")
				continue
			}
			if len(zb.xs) > 0 && pt[0] <= zb.xs[len(zb.xs)-1] {
				p.Add(at, "x %s is not after %s: give a boundary's points in increasing x", fmtValue(pt[0]), fmtValue(zb.xs[len(zb.xs)-1]))
				break
			}
			zb.xs = append(zb.xs, pt[0])
			zb.ys = append(zb.ys, pt[1])
		}
		xs = append(xs, zb.xs...)
		ys = append(ys, zb.ys...)
		d.bounds = append(d.bounds, zb)
	}
	if len(s.Zones) != len(s.Boundaries)+1 {
		p.Add("zones", "has %d zones for %d boundaries; give one more zone than boundaries, lowest first (below the first boundary, between each pair, above the last)", len(s.Zones), len(s.Boundaries))
	}
	for i := range s.Zones {
		s.Zones[i].Label = p.Label(core.Path("zones", i, "label"), s.Zones[i].Label, true, zoneLabelChars)
		p.Tone(core.Path("zones", i, "tone"), s.Zones[i].Tone)
	}
	d.zones = s.Zones
	p.Count("points", len(s.Points), 0, 8)
	if p.Any() {
		return nil, p.Err()
	}
	// The boundaries share one x range and keep their order.
	d.xLo, d.xHi = d.bounds[0].xs[0], d.bounds[0].xs[len(d.bounds[0].xs)-1]
	span := d.xHi - d.xLo
	for i, b := range d.bounds[1:] {
		lo, hi := b.xs[0], b.xs[len(b.xs)-1]
		if math.Abs(lo-d.xLo) > span*1e-6 || math.Abs(hi-d.xHi) > span*1e-6 {
			p.Add(core.Path("boundaries", i+1, "points"), "run from x %s to %s but boundaries[0] runs from %s to %s: give every boundary over the same x range", fmtValue(lo), fmtValue(hi), fmtValue(d.xLo), fmtValue(d.xHi))
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	all := append([]float64{}, xs...)
	sort.Float64s(all)
	for i := 1; i < len(d.bounds); i++ {
		a, b := d.bounds[i-1], d.bounds[i]
		above, below := 0, 0
		cross := math.NaN()
		for _, x := range all {
			switch va, vb := a.at(x), b.at(x); {
			case va > vb+1e-9:
				below++
				if math.IsNaN(cross) {
					cross = x
				}
			case vb > va+1e-9:
				above++
			}
		}
		switch {
		case below > 0 && above == 0:
			p.Add("boundaries", "boundaries[%d] lies above boundaries[%d]: list the boundaries from lowest to highest", i-1, i)
		case below > 0:
			p.Add(core.Path("boundaries", i), "crosses boundaries[%d] near x %s; boundaries must not cross: check the values there", i-1, fmtValue(cross))
		}
	}
	for i, pt := range s.Points {
		at := core.Path("points", i)
		if pt.X == nil || pt.Y == nil {
			p.Add(at, "needs x and y")
			continue
		}
		zp := zonePoint{x: *pt.X, y: *pt.Y, label: p.Label(core.Path(at, "label"), pt.Label, false, zoneBoundChars)}
		if zp.x < d.xLo-span*1e-9 || zp.x > d.xHi+span*1e-9 {
			p.Add(core.Path(at, "x"), "%s is outside the boundaries' x range (%s to %s), where no zone is defined", fmtValue(zp.x), fmtValue(d.xLo), fmtValue(d.xHi))
		}
		if len(d.points) > 0 && zp.x < d.points[len(d.points)-1].x {
			p.Add(core.Path(at, "x"), "%s comes before the previous point's %s: give the points in order of x", fmtValue(zp.x), fmtValue(d.points[len(d.points)-1].x))
		}
		for _, b := range d.bounds {
			if zp.y >= b.at(zp.x)-1e-9 {
				zp.zone++
			}
		}
		d.points = append(d.points, zp)
		xs = append(xs, zp.x)
		ys = append(ys, zp.y)
	}
	if p.Any() {
		return nil, p.Err()
	}
	xlo, xhi := extent(xs)
	xa := s.XAxis
	if xa == nil {
		xa = &AxisSpec{}
	}
	if xa.Min == nil && xa.Max == nil {
		// The boundaries' own range, unless the spec sets one.
		cp := *xa
		cp.Min, cp.Max = &d.xLo, &d.xHi
		xa = &cp
	}
	d.x = resolveAxis(&p, "x_axis", xa, xlo, xhi, axisOpts{})
	ylo, yhi := extent(ys)
	d.y = resolveAxis(&p, "y_axis", s.YAxis, ylo, yhi, axisOpts{})
	if p.Any() {
		return nil, p.Err()
	}
	for i, zp := range d.points {
		d.x.checkIn(&p, core.Path("points", i, "x"), "x_axis", zp.x)
		d.y.checkIn(&p, core.Path("points", i, "y"), "y_axis", zp.y)
	}
	return d, p.Err()
}

func (d *zoneDrawing) Draw(c *core.Canvas) error {
	t := c.T
	p := newPlot(c, d.x, d.y)
	var labels []string
	for _, b := range d.bounds {
		labels = append(labels, b.label)
	}
	if w := endLabelWidth(labels); w > 0 {
		p.rightPad = w + 12
	}
	p.layout(0)
	// Sample the boundaries across their range in figure units.
	n := 120
	xs := make([]float64, n+1)
	for i := range xs {
		xs[i] = d.xLo + (d.xHi-d.xLo)*float64(i)/float64(n)
	}
	curve := func(k int) []core.Pt {
		var pts []core.Pt
		for _, x := range xs {
			v := d.y.lo
			if k == len(d.bounds) {
				v = d.y.hi
			} else if k >= 0 {
				v = d.bounds[k].at(x)
			}
			pts = append(pts, core.Pt{X: d.x.px(x), Y: d.y.px(v)})
		}
		return pts
	}
	// Zone fills, between the curve under each and the curve over it.
	for z := range d.zones {
		lower, upper := curve(z-1), curve(z)
		poly := append([]core.Pt{}, upper...)
		for i := len(lower) - 1; i >= 0; i-- {
			poly = append(poly, lower[i])
		}
		_, fill := t.Tone(d.zones[z].Tone)
		if cl := clipPolygon(poly, p.r); len(cl) >= 3 {
			c.El("polygon", "points", core.Points(cl), "fill", fill, "fill-opacity", "0.85")
		}
	}
	p.drawFrame(0)
	var ends []endLabel
	for k, b := range d.bounds {
		pts := curve(k)
		for _, part := range clipPolyline(pts, p.r.grow(0.5)) {
			c.Polyline(part, t.Ink, 1.4, "")
			p.obs.line(part)
		}
		if b.label != "" {
			last := pts[len(pts)-1]
			ends = append(ends, endLabel{text: b.label, y: math.Max(p.r.y0, math.Min(p.r.y1, last.Y)), colour: t.Muted})
		}
	}
	want := make([]float64, len(ends))
	for i, e := range ends {
		want[i] = e.y
	}
	got := spread(want, tickSize+2, p.r.y0, p.r.y1)
	for i, e := range ends {
		if math.Abs(got[i]-e.y) > 1.5 {
			c.Line(p.r.x1+1.5, e.y, p.r.x1+4.5, got[i], t.Edge, 0.8, "")
		}
		c.Text(p.r.x1+6, got[i], e.text, core.TextStyle{Size: tickSize, Fill: e.colour})
	}
	// The patient's points.
	var pts []core.Pt
	for _, zp := range d.points {
		pts = append(pts, core.Pt{X: d.x.px(zp.x), Y: d.y.px(zp.y)})
	}
	if len(pts) > 1 {
		c.Polyline(pts, t.Accent, 2, "")
	}
	for _, pt := range pts {
		c.Circle(pt.X, pt.Y, 4, t.Accent, t.Bg, 1.4)
		p.obs.point(pt)
	}
	p.obs.line(pts)
	// Zone names, each where its zone leaves the most room for it across
	// the label's whole width, clear of the points.
	for z, zn := range d.zones {
		w := core.TextWidth(zn.Label, tickSize, true)
		best, bestAt := math.Inf(-1), core.Pt{}
		for k := 0; k <= 24; k++ {
			cx := p.r.x0 + w/2 + 5 + (p.r.w()-w-10)*float64(k)/24
			// The zone's open window over the label's span.
			top, bot := p.r.y0, p.r.y1
			for j := 0; j <= 8; j++ {
				x := d.x.value(cx - w/2 - 3 + (w+6)*float64(j)/8)
				if z > 0 {
					bot = math.Min(bot, d.y.px(d.bounds[z-1].at(x)))
				}
				if z < len(d.bounds) {
					top = math.Max(top, d.y.px(d.bounds[z].at(x)))
				}
			}
			room := bot - top
			cy := (top + bot) / 2
			r := textRect(cx, cy, zn.Label, tickSize, true, core.Middle)
			score := room
			for _, q := range p.obs.pts {
				if r.grow(5).contains(q) {
					score -= 100
				}
			}
			for _, q := range p.obs.rects {
				if r.overlaps(q) {
					score -= 100
				}
			}
			if score > best {
				best, bestAt = score, core.Pt{X: cx, Y: cy}
			}
		}
		stroke, _ := t.Tone(zn.Tone)
		if zn.Tone == "" || zn.Tone == "neutral" {
			stroke = t.Muted
		}
		if best >= tickSize {
			c.Text(bestAt.X, bestAt.Y, zn.Label, core.TextStyle{Size: tickSize, Bold: true, Fill: stroke, Anchor: core.Middle, Halo: t.Bg})
			p.obs.rect(textRect(bestAt.X, bestAt.Y, zn.Label, tickSize, true, core.Middle))
		}
	}
	if len(d.points) > 0 {
		last := d.points[len(d.points)-1]
		head := fmtValue(last.y) + " at " + fmtValue(last.x)
		if xu := unitOf(d.x.title); xu != "" {
			head += " " + xu
		}
		if last.label != "" {
			head = last.label + ": " + head
		}
		note, _ := t.Tone(d.zones[last.zone].Tone)
		if d.zones[last.zone].Tone == "" || d.zones[last.zone].Tone == "neutral" {
			note = t.Muted
		}
		p.callout(pts[len(pts)-1], []string{head, d.zones[last.zone].Label}, note)
	}
	bottom := p.bottom
	// With more than one point, each point's zone in a list.
	if len(d.points) > 1 {
		bottom += 4
		c.Line(padX, bottom, core.Width-padX, bottom, t.Faint, 1, "")
		bottom += 4
		for _, zp := range d.points {
			y := bottom + 9
			at := fmtValue(zp.x)
			if xu := unitOf(d.x.title); xu != "" {
				at += " " + xu
			}
			name := at
			if zp.label != "" {
				name = zp.label + " · " + at
			}
			stroke, _ := t.Tone(d.zones[zp.zone].Tone)
			if d.zones[zp.zone].Tone == "" || d.zones[zp.zone].Tone == "neutral" {
				stroke = t.Muted
			}
			c.Text(padX, y, name, core.TextStyle{Size: labelSize, Fill: t.Muted})
			c.Text(210, y, fmtValue(zp.y), core.TextStyle{Size: labelSize, Fill: t.Ink, Bold: true, Anchor: core.End})
			c.Circle(226, y, 4, stroke, "", 0)
			c.Text(236, y, d.zones[zp.zone].Label, core.TextStyle{Size: labelSize, Fill: stroke, Bold: true})
			bottom += 18
		}
		bottom += 2
	}
	c.H = bottom
	return nil
}

// unitOf is the unit in an axis title "Age (hours)", or "".
func unitOf(title string) string {
	i := len(title) - 1
	if i < 0 || title[i] != ')' {
		return ""
	}
	for j := i; j >= 0; j-- {
		if title[j] == '(' {
			return title[j+1 : i]
		}
	}
	return ""
}
