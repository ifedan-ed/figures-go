package chart

import (
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// BandSpec is a shaded range on one axis, drawn under the data: a normal
// range, a target, a window.
type BandSpec struct {
	Axis  string  `json:"axis"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Label string  `json:"label"`
	Tone  string  `json:"tone"`
}

// RefLineSpec is a dashed line across the plot at one value: a threshold,
// a target, a time.
type RefLineSpec struct {
	Axis  string  `json:"axis"`
	Value float64 `json:"value"`
	Label string  `json:"label"`
	Tone  string  `json:"tone"`
}

// AnnotationSpec is a point the figure points out, with a short label set
// clear of the data and joined to the point by a leader line.
type AnnotationSpec struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label"`
}

const (
	maxOverlay    = 6
	maxOverlayLab = 32
)

var overlayFields = "bands?: [{axis: x|y, from, to, label?, tone?}] ≤6, drawn under the data; reference_lines?: [{axis: x|y, value, label?, tone?}] ≤6; annotations?: [{x, y, label}] ≤6, a labelled point with a leader line; each of their labels " + core.UpTo(maxOverlayLab)

// overlays are a chart's bands, reference lines and annotations.
type overlays struct {
	Bands       []BandSpec       `json:"bands"`
	RefLines    []RefLineSpec    `json:"reference_lines"`
	Annotations []AnnotationSpec `json:"annotations"`
}

// check checks the overlays and returns the values they need in view on
// each axis.
func (o *overlays) check(p *core.Problems) (xs, ys []float64) {
	p.Count("bands", len(o.Bands), 0, maxOverlay)
	p.Count("reference_lines", len(o.RefLines), 0, maxOverlay)
	p.Count("annotations", len(o.Annotations), 0, maxOverlay)
	add := func(axisName string, vs ...float64) {
		if axisName == "x" {
			xs = append(xs, vs...)
		} else {
			ys = append(ys, vs...)
		}
	}
	for i := range o.Bands {
		b := &o.Bands[i]
		path := core.Path("bands", i)
		p.OneOf(core.Path(path, "axis"), b.Axis, false, "x", "y")
		p.Finite(core.Path(path, "from"), b.From)
		p.Finite(core.Path(path, "to"), b.To)
		if b.From >= b.To {
			p.Add(path, "from (%s) must be less than to (%s)", fmtValue(b.From), fmtValue(b.To))
		}
		b.Label = p.Label(core.Path(path, "label"), b.Label, false, maxOverlayLab)
		p.Tone(core.Path(path, "tone"), b.Tone)
		add(b.Axis, b.From, b.To)
	}
	for i := range o.RefLines {
		r := &o.RefLines[i]
		path := core.Path("reference_lines", i)
		p.OneOf(core.Path(path, "axis"), r.Axis, false, "x", "y")
		p.Finite(core.Path(path, "value"), r.Value)
		r.Label = p.Label(core.Path(path, "label"), r.Label, false, maxOverlayLab)
		p.Tone(core.Path(path, "tone"), r.Tone)
		add(r.Axis, r.Value)
	}
	for i := range o.Annotations {
		a := &o.Annotations[i]
		path := core.Path("annotations", i)
		p.Finite(core.Path(path, "x"), a.X)
		p.Finite(core.Path(path, "y"), a.Y)
		a.Label = p.Label(core.Path(path, "label"), a.Label, true, maxOverlayLab)
		xs = append(xs, a.X)
		ys = append(ys, a.Y)
	}
	return xs, ys
}

// checkInAxes refuses overlays outside axes the spec fixed.
func (o *overlays) checkInAxes(p *core.Problems, x, y *axis) {
	ax := func(name string) (*axis, string) {
		if name == "x" {
			return x, "x_axis"
		}
		return y, "y_axis"
	}
	for i, b := range o.Bands {
		a, n := ax(b.Axis)
		// A band may run past the axis (a range open at one end); it must
		// show at least in part.
		if a.log && b.From <= 0 {
			p.Add(core.Path("bands", i, "from"), "is %s, but %s is a log axis, which holds only positive numbers", fmtValue(b.From), n)
		} else if b.To < a.lo || b.From > a.hi {
			p.Add(core.Path("bands", i), "%s to %s is outside %s (%s to %s)", fmtValue(b.From), fmtValue(b.To), n, fmtValue(a.lo), fmtValue(a.hi))
		}
	}
	for i, r := range o.RefLines {
		a, n := ax(r.Axis)
		a.checkIn(p, core.Path("reference_lines", i, "value"), n, r.Value)
	}
	for i, an := range o.Annotations {
		x.checkIn(p, core.Path("annotations", i, "x"), "x_axis", an.X)
		y.checkIn(p, core.Path("annotations", i, "y"), "y_axis", an.Y)
	}
}

// bandSpan is the band's ends clipped to the axis, in figure units.
func bandSpan(a *axis, from, to float64) (float64, float64) {
	if a.log {
		from = math.Max(from, a.lo)
	}
	from, to = math.Max(from, a.lo), math.Min(to, a.hi)
	p0, p1 := a.px(from), a.px(to)
	return math.Min(p0, p1), math.Max(p0, p1)
}

// drawBands fills the bands; their labels are set later, once the data's
// place is known.
func (p *plot) drawBands(bands []BandSpec) {
	for _, b := range bands {
		_, fill := p.c.T.Tone(b.Tone)
		r := p.bandRect(b)
		p.c.El("rect", "x", core.F(r.x0), "y", core.F(r.y0), "width", core.F(r.w()), "height", core.F(r.h()),
			"fill", fill, "fill-opacity", "0.75")
	}
}

func (p *plot) bandRect(b BandSpec) rect {
	if b.Axis == "x" {
		x0, x1 := bandSpan(p.x, b.From, b.To)
		return rect{x0, p.r.y0, x1, p.r.y1}
	}
	y0, y1 := bandSpan(p.y, b.From, b.To)
	return rect{p.r.x0, y0, p.r.x1, y1}
}

// drawRefLines draws the reference lines (their labels come later).
func (p *plot) drawRefLines(lines []RefLineSpec) {
	for _, l := range lines {
		col := p.refColour(l.Tone)
		if l.Axis == "x" {
			x := p.x.px(l.Value)
			p.c.Line(x, p.r.y0, x, p.r.y1, col, 1.3, "5 4")
			p.obs.line([]core.Pt{{X: x, Y: p.r.y0}, {X: x, Y: p.r.y1}})
		} else {
			y := p.y.px(l.Value)
			p.c.Line(p.r.x0, y, p.r.x1, y, col, 1.3, "5 4")
		}
	}
}

func (p *plot) refColour(tone string) string {
	if tone == "" || tone == "neutral" {
		return p.c.T.Muted
	}
	s, _ := p.c.T.Tone(tone)
	return s
}

// labelOverlays sets the bands' and reference lines' labels where they
// cover least, then the annotations. Call it after the data is drawn and
// its lines added to p.obs.
func (p *plot) labelOverlays(o overlays) {
	c := p.c
	inner := p.r.grow(-3)
	place := func(cands []struct {
		x, y float64
		a    core.Anchor
	}, s string, size float64) (float64, float64, core.Anchor) {
		best, bi := math.Inf(1), 0
		for i, cd := range cands {
			r := textRect(cd.x, cd.y, s, size, false, cd.a)
			cost := p.obs.cost(r) + float64(i)*0.01
			if !r.inside(inner) {
				cost += 100
			}
			if cost < best {
				best, bi = cost, i
			}
		}
		return cands[bi].x, cands[bi].y, cands[bi].a
	}
	type cand = struct {
		x, y float64
		a    core.Anchor
	}
	for _, b := range o.Bands {
		if b.Label == "" {
			continue
		}
		stroke, _ := c.T.Tone(b.Tone)
		if b.Tone == "" || b.Tone == "neutral" {
			stroke = c.T.Muted
		}
		r := p.bandRect(b)
		var cands []cand
		if b.Axis == "x" {
			cands = []cand{{r.x0 + 4, r.y0 + 9, core.Start}, {r.x1 - 4, r.y0 + 9, core.End}, {(r.x0 + r.x1) / 2, r.y0 + 9, core.Middle},
				{r.x0 + 4, r.y1 - 9, core.Start}, {r.x1 - 4, r.y1 - 9, core.End}}
		} else {
			cands = []cand{{r.x1 - 4, r.y0 + 9, core.End}, {r.x0 + 4, r.y0 + 9, core.Start}, {r.x1 - 4, r.y1 - 9, core.End}, {r.x0 + 4, r.y1 - 9, core.Start}}
			if r.h() < 20 {
				cands = []cand{{r.x1 - 4, (r.y0 + r.y1) / 2, core.End}, {r.x0 + 4, (r.y0 + r.y1) / 2, core.Start}}
			}
		}
		x, y, a := place(cands, b.Label, tickSize)
		c.Text(x, y, b.Label, core.TextStyle{Size: tickSize, Fill: stroke, Anchor: a, Halo: c.T.Bg, Bold: true})
		p.obs.rect(textRect(x, y, b.Label, tickSize, true, a))
	}
	for _, l := range o.RefLines {
		if l.Label == "" {
			continue
		}
		col := p.refColour(l.Tone)
		var cands []cand
		if l.Axis == "x" {
			x := p.x.px(l.Value)
			cands = []cand{{x + 4, p.r.y0 + 9, core.Start}, {x - 4, p.r.y0 + 9, core.End}, {x + 4, p.r.y1 - 9, core.Start}, {x - 4, p.r.y1 - 9, core.End}}
		} else {
			y := p.y.px(l.Value)
			cands = []cand{{p.r.x1 - 3, y - 8, core.End}, {p.r.x0 + 3, y - 8, core.Start}, {p.r.x1 - 3, y + 8, core.End}, {p.r.x0 + 3, y + 8, core.Start}}
		}
		x, y, a := place(cands, l.Label, tickSize)
		c.Text(x, y, l.Label, core.TextStyle{Size: tickSize, Fill: col, Anchor: a, Halo: c.T.Bg, Bold: true})
		p.obs.rect(textRect(x, y, l.Label, tickSize, true, a))
	}
	for _, an := range o.Annotations {
		p.annotate(core.Pt{X: p.x.px(an.X), Y: p.y.px(an.Y)}, an.Label, c.T.Ink, true, true)
	}
}

// annotate labels the point at pt: the label goes where it covers least of
// the data and of other labels, near the point, joined to it by a leader
// line. mark draws a ring at the point.
func (p *plot) annotate(pt core.Pt, label string, colour string, mark, leader bool) rect {
	c := p.c
	size := labelSize
	lines := core.Wrap(label, size, 130, false)
	w := core.WidestLine(lines, size, false)
	h := core.BlockHeight(len(lines), size, 0)
	bounds := rect{p.r.x0 + 2, p.r.y0 + 2, p.r.x1 - 2, p.r.y1 - 2}
	if p.rightPad > 0 {
		bounds.x1 = core.Width - 4
	}
	best, bestR := math.Inf(1), rect{}
	dists := []float64{26, 40, 56, 74}
	if !leader {
		dists = []float64{w/2 + 8, w/2 + 16}
	}
	for _, dist := range dists {
		for k := 0; k < 16; k++ {
			ang := float64(k) * math.Pi / 8
			// Prefer above and to the right: try those first.
			ang = -math.Pi/4 - ang
			cx := pt.X + math.Cos(ang)*dist
			cy := pt.Y + math.Sin(ang)*dist
			r := rect{cx - w/2, cy - h/2, cx + w/2, cy + h/2}
			cost := p.obs.cost(r.grow(2))*4 + dist/30 + float64(k)*0.02
			if !r.inside(bounds) {
				cost += 1000
			}
			if r.grow(4).contains(pt) {
				cost += 1000
			}
			// Nor may its leader cross another label.
			if leader {
				end := core.Pt{X: math.Max(r.x0, math.Min(pt.X, r.x1)), Y: math.Max(r.y0, math.Min(pt.Y, r.y1))}
				for _, q := range p.obs.rects {
					if segHitsRect(pt, end, q) {
						cost += 25
					}
				}
			}
			if cost < best {
				best, bestR = cost, r
			}
		}
	}
	r := bestR
	// The leader runs from the point to the label's nearest edge.
	end := core.Pt{X: math.Max(r.x0, math.Min(pt.X, r.x1)), Y: math.Max(r.y0, math.Min(pt.Y, r.y1))}
	dx, dy := end.X-pt.X, end.Y-pt.Y
	if l := math.Hypot(dx, dy); l > 8 && leader {
		start := core.Pt{X: pt.X + dx/l*5, Y: pt.Y + dy/l*5}
		stop := core.Pt{X: end.X - dx/l*2, Y: end.Y - dy/l*2}
		c.Line(start.X, start.Y, stop.X, stop.Y, c.T.Muted, 1, "")
	}
	if mark {
		c.Circle(pt.X, pt.Y, 3.6, c.T.Bg, colour, 1.8)
	}
	anchor, x := core.Middle, (r.x0+r.x1)/2
	if end.X <= r.x0+0.01 {
		anchor, x = core.Start, r.x0
	} else if end.X >= r.x1-0.01 {
		anchor, x = core.End, r.x1
	}
	c.Lines(x, (r.y0+r.y1)/2, lines, core.TextStyle{Size: size, Fill: colour, Anchor: anchor, Halo: c.T.Bg}, 0)
	p.obs.rect(r)
	p.obs.point(pt)
	return r
}

// endLabel is a label set at the right end of a line, in the line's colour.
type endLabel struct {
	text   string
	y      float64 // the line's end
	colour string
}

// endLabelWidth is the widest end label.
func endLabelWidth(labels []string) float64 {
	w := 0.0
	for _, l := range labels {
		w = math.Max(w, core.TextWidth(l, labelSize, true))
	}
	return w
}

// drawEndLabels sets labels right of the plot, each level with its line's
// end, nudged apart where they would touch, with a short leader where a
// label has moved off its line.
func (p *plot) drawEndLabels(labels []endLabel, size float64) {
	if len(labels) == 0 {
		return
	}
	want := make([]float64, len(labels))
	for i, l := range labels {
		want[i] = l.y
	}
	got := spread(want, size+2, p.r.y0-4, p.r.y1+4)
	for i, l := range labels {
		x := p.r.x1 + 6
		if math.Abs(got[i]-l.y) > 1.5 {
			p.c.Polyline([]core.Pt{{X: p.r.x1 + 1, Y: l.y}, {X: p.r.x1 + 4, Y: got[i]}}, l.colour, 1, "")
		}
		p.c.Text(x, got[i], l.text, core.TextStyle{Size: size, Fill: l.colour, Bold: true})
	}
}

// labelValueLine sets the label of a reference line on a bar or box
// chart's value axis where it covers least of what is drawn: the bars,
// boxes, whiskers, value labels and other reference labels already in
// p.obs. along says the value axis runs along x (horizontal bars), so the
// line is upright at at; otherwise the line runs across the plot at at.
//
// It tries the usual places first (beside the line's top or right end,
// then its other end) and then walks the line in small steps, so a label
// finds the gap between bars when the data crosses the line at both ends.
// Before, a horizontal chart put the label at the top whatever was there,
// and a vertical one tried only the two ends, and a label could sit on a
// bar. The label it sets is added to p.obs, so the next one keeps clear.
func (p *plot) labelValueLine(along bool, at float64, label, colour string) {
	if label == "" {
		return
	}
	c := p.c
	inner := p.r.grow(-3)
	w := core.TextWidth(label, tickSize, true)
	type cand = struct {
		x, y float64
		a    core.Anchor
	}
	var cands []cand
	const step = 6.0
	if along {
		// Beside an upright line: right of it, then left, from the top down.
		top, bottom := p.r.y0+9, p.r.y1-9
		cands = append(cands, cand{at + 4, top, core.Start}, cand{at - 4, top, core.End})
		for y := top + step; y <= bottom; y += step {
			cands = append(cands, cand{at + 4, y, core.Start}, cand{at - 4, y, core.End})
		}
	} else {
		// Just above a line across the plot, then just below it, from the
		// right end leftwards.
		right, left := p.r.x1-3, p.r.x0+3
		for _, dy := range []float64{-8, 8} {
			cands = append(cands, cand{right, at + dy, core.End}, cand{left, at + dy, core.Start})
		}
		for _, dy := range []float64{-8, 8} {
			for x := right - step; x-w >= left; x -= step {
				cands = append(cands, cand{x, at + dy, core.End})
			}
		}
	}
	best, bi := math.Inf(1), 0
	for i, cd := range cands {
		r := textRect(cd.x, cd.y, label, tickSize, true, cd.a)
		// The order is the preference: a later place wins only by covering
		// less.
		cost := p.obs.cost(r) + float64(i)*0.001
		if !r.inside(inner) {
			cost += 100
		}
		if cost < best {
			best, bi = cost, i
		}
	}
	cd := cands[bi]
	c.Text(cd.x, cd.y, label, core.TextStyle{Size: tickSize, Fill: colour, Bold: true, Anchor: cd.a, Halo: c.T.Bg})
	p.obs.rect(textRect(cd.x, cd.y, label, tickSize, true, cd.a))
}
