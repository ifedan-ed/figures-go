package chart

import (
	"encoding/json"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type doseCurveSpec struct {
	Label string   `json:"label"`
	Emax  *float64 `json:"emax"`
	EC50  *float64 `json:"ec50"`
	Hill  *float64 `json:"hill"`
	E0    *float64 `json:"e0"`
	Dash  bool     `json:"dash"`
}

type doseWindowSpec struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Label string  `json:"label"`
}

type doseSpec struct {
	Curves   []doseCurveSpec `json:"curves"`
	XAxis    *AxisSpec       `json:"x_axis"`
	YAxis    *AxisSpec       `json:"y_axis"`
	Window   *doseWindowSpec `json:"window"`
	MarkEC50 *bool           `json:"mark_ec50"`
}

var doseResponse = core.Kind{
	Name:    "dose_response",
	Family:  "clinical chart",
	Use:     "Concentration–response curves computed from the Hill equation (E = E0 + Emax·Cⁿ/(EC50ⁿ + Cⁿ)): potency and efficacy, full versus partial agonists, the right shift of a competitive antagonist; give each curve's parameters, never its points.",
	Fields:  "curves: [{label (required if >1; " + core.UpTo(maxSeriesLabel) + "), emax (the maximum effect above e0; negative for inhibition), ec50 (> 0, in the x unit), hill?: 0.2–5 (default 1), e0?: baseline (default 0), dash?: bool}] 1–6; x_axis?: " + axisFields + " (log by default; range defaults to two powers of ten either side of the EC50s); y_axis?: same, linear; window?: {from, to, label? (" + core.UpTo(maxOverlayLab) + ")}, a therapeutic window shaded on x; mark_ec50?: bool (default true), marks each EC50 at half its maximum.",
	Example: `{"type":"dose_response","title":"Agonist, partial agonist and a competitive antagonist","alt":"Three sigmoid curves on a log concentration axis: the full agonist reaches 100% with an EC50 of 10 nM; the partial agonist plateaus at 45% whatever the dose; with a competitive antagonist the full agonist's curve shifts ten-fold right to an EC50 of 100 nM but still reaches 100%.","spec":{"x_axis":{"label":"Agonist concentration","unit":"nM"},"y_axis":{"label":"Response","unit":"% of maximum","min":0,"max":100},"curves":[{"label":"Full agonist","emax":100,"ec50":10},{"label":"Partial agonist","emax":45,"ec50":8},{"label":"+ antagonist","emax":100,"ec50":100,"dash":true}]}}`,
	Parse:   parseDose,
}

type doseCurve struct {
	label                string
	emax, ec50, hill, e0 float64
	dash                 bool
}

func (c doseCurve) at(x float64) float64 {
	if x <= 0 {
		return c.e0
	}
	r := math.Pow(x/c.ec50, c.hill)
	return c.e0 + c.emax*r/(1+r)
}

type doseDrawing struct {
	curves []doseCurve
	x, y   *axis
	window *doseWindowSpec
	mark   bool
}

func parseDose(raw json.RawMessage) (core.Drawing, error) {
	var s doseSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("curves", len(s.Curves), 1, 6)
	d := &doseDrawing{mark: s.MarkEC50 == nil || *s.MarkEC50}
	var ec, ys []float64
	seen := map[string]bool{}
	for i, cs := range s.Curves {
		path := core.Path("curves", i)
		cv := doseCurve{label: p.Label(core.Path(path, "label"), cs.Label, len(s.Curves) > 1, maxSeriesLabel), hill: 1, dash: cs.Dash}
		if seen[cv.label] && cv.label != "" {
			p.Add(core.Path(path, "label"), "%q is used twice; each curve needs its own label", cv.label)
		}
		seen[cv.label] = true
		switch {
		case cs.Emax == nil:
			p.Add(core.Path(path, "emax"), "is required: the maximum effect above the baseline")
		case *cs.Emax == 0:
			p.Add(core.Path(path, "emax"), "is 0, which draws a flat line; give the maximum effect (negative for an inhibitor)")
		default:
			cv.emax = *cs.Emax
		}
		switch {
		case cs.EC50 == nil:
			p.Add(core.Path(path, "ec50"), "is required: the concentration giving half the maximum effect, in the x axis's unit")
		case *cs.EC50 <= 0:
			p.Add(core.Path(path, "ec50"), "is %s; an EC50 is a concentration, greater than zero", fmtValue(*cs.EC50))
		default:
			cv.ec50 = *cs.EC50
			ec = append(ec, cv.ec50)
		}
		if cs.Hill != nil {
			if *cs.Hill < 0.2 || *cs.Hill > 5 {
				p.Add(core.Path(path, "hill"), "is %s; a Hill coefficient from 0.2 to 5 is drawn (1 for simple binding)", fmtValue(*cs.Hill))
			}
			cv.hill = *cs.Hill
		}
		if cs.E0 != nil {
			cv.e0 = *cs.E0
		}
		ys = append(ys, cv.e0, cv.e0+cv.emax)
		d.curves = append(d.curves, cv)
	}
	if p.Any() {
		return nil, p.Err()
	}
	lo, hi := extent(ec)
	xa := s.XAxis
	if xa == nil {
		xa = &AxisSpec{}
	}
	xs := *xa
	if xs.Label == "" && xs.Unit == "" {
		xs.Label = "Concentration"
	}
	log := xs.Scale != "linear"
	dlo, dhi := lo/100, hi*100
	if !log {
		dlo, dhi = 0, hi*4
	}
	if s.Window != nil {
		if s.Window.From >= s.Window.To {
			p.Add("window", "from (%s) must be less than to (%s)", fmtValue(s.Window.From), fmtValue(s.Window.To))
		}
		if log && s.Window.From <= 0 {
			p.Add("window.from", "is %s; on a log axis the window must start above zero", fmtValue(s.Window.From))
		}
		s.Window.Label = p.Label("window.label", s.Window.Label, false, maxOverlayLab)
		dlo, dhi = math.Min(dlo, s.Window.From), math.Max(dhi, s.Window.To)
		if log {
			dlo = math.Max(dlo, s.Window.From/10)
		}
	}
	d.window = s.Window
	d.x = resolveAxis(&p, "x_axis", &xs, dlo, dhi, axisOpts{logDefault: true})
	ya := s.YAxis
	if ya == nil {
		ya = &AxisSpec{}
	}
	yss := *ya
	if yss.Label == "" && yss.Unit == "" {
		yss.Label = "Response"
	}
	ylo, yhi := extent(append(ys, 0))
	d.y = resolveAxis(&p, "y_axis", &yss, ylo, yhi, axisOpts{noLog: true})
	if p.Any() {
		return nil, p.Err()
	}
	for i, cv := range d.curves {
		if !d.x.in(cv.ec50) {
			p.Add(core.Path("curves", i, "ec50"), "%s is outside x_axis (%s to %s): widen the axis so the curve's middle shows", fmtValue(cv.ec50), fmtValue(d.x.lo), fmtValue(d.x.hi))
		}
		for _, v := range []float64{cv.e0, cv.e0 + cv.emax} {
			if !d.y.in(v) {
				p.Add(core.Path("curves", i), "reaches %s, outside y_axis (%s to %s): widen the axis", fmtValue(v), fmtValue(d.y.lo), fmtValue(d.y.hi))
				break
			}
		}
	}
	return d, p.Err()
}

func (d *doseDrawing) Draw(c *core.Canvas) error {
	t := c.T
	p := newPlot(c, d.x, d.y)
	p.xGrid = true
	var labels []string
	for _, cv := range d.curves {
		labels = append(labels, cv.label)
	}
	ss := make([]series, len(d.curves))
	for i, cv := range d.curves {
		ss[i] = series{label: cv.label, dash: cv.dash}
	}
	useEnd := len(d.curves) > 1 && endLabelWidth(labels) <= 92
	if useEnd {
		p.rightPad = endLabelWidth(labels) + 12
	} else if len(d.curves) > 1 {
		p.legend = seriesLegend(c, ss, legLine, false)
	}
	p.layout(0)
	if d.window != nil {
		x0, x1 := bandSpan(d.x, d.window.From, d.window.To)
		_, fill := t.Tone("good")
		c.El("rect", "x", core.F(x0), "y", core.F(p.r.y0), "width", core.F(x1-x0), "height", core.F(p.r.h()), "fill", fill, "fill-opacity", "0.75")
	}
	p.drawFrame(0)
	var ends []endLabel
	type mark struct {
		at  core.Pt
		col string
		cv  doseCurve
	}
	var marks []mark
	for i, cv := range d.curves {
		col := t.SeriesColour(i)
		var pts []core.Pt
		n := 160
		for k := 0; k <= n; k++ {
			px := p.r.x0 + p.r.w()*float64(k)/float64(n)
			x := d.x.value(px)
			pts = append(pts, core.Pt{X: px, Y: d.y.px(cv.at(x))})
		}
		dash := seriesDash(i, len(d.curves))
		if cv.dash {
			dash = "6 4"
		}
		for _, part := range clipPolyline(pts, p.r.grow(1)) {
			c.Polyline(part, col, 2.4, dash)
			p.obs.line(part)
		}
		if useEnd {
			ends = append(ends, endLabel{text: cv.label, y: math.Max(p.r.y0, math.Min(p.r.y1, pts[n].Y)), colour: col})
		}
		if d.mark {
			marks = append(marks, mark{core.Pt{X: d.x.px(cv.ec50), Y: d.y.px(cv.e0 + cv.emax/2)}, col, cv})
		}
	}
	// Each EC50: a dotted drop to the axis, a ring at half the maximum,
	// and its value where there is room.
	base := p.r.y1
	for _, m := range marks {
		c.Line(m.at.X, m.at.Y+4, m.at.X, base, m.col, 1.1, "1.5 3")
		c.Circle(m.at.X, m.at.Y, 4, t.Bg, m.col, 2)
		p.obs.point(m.at)
		p.obs.line([]core.Pt{m.at, {X: m.at.X, Y: base}})
	}
	p.drawEndLabels(ends, labelSize)
	if len(marks) <= 3 {
		unit := unitOf(d.x.title)
		for _, m := range marks {
			text := "EC50 " + fmtValue(m.cv.ec50)
			if unit != "" {
				text += " " + unit
			}
			p.annotate(m.at, text, m.col, false, false)
		}
	}
	if d.window != nil && d.window.Label != "" {
		x0, x1 := bandSpan(d.x, d.window.From, d.window.To)
		stroke, _ := t.Tone("good")
		x, a := (x0+x1)/2, core.Middle
		r := textRect(x, p.r.y0+9, d.window.Label, tickSize, true, a)
		if r.x0 < p.r.x0+2 {
			x, a = x0+3, core.Start
		} else if r.x1 > p.r.x1-2 {
			x, a = x1-3, core.End
		}
		c.Text(x, p.r.y0+9, d.window.Label, core.TextStyle{Size: tickSize, Bold: true, Fill: stroke, Anchor: a, Halo: t.Bg})
	}
	c.H = p.bottom
	return nil
}
