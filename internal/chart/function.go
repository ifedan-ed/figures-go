package chart

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
	"github.com/ifedan-ed/figures-go/internal/expr"
)

type fnSpec struct {
	Expr  string `json:"expr"`
	Label string `json:"label"`
	Dash  bool   `json:"dash"`
}

type fnAxisSpec struct {
	Label string   `json:"label"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Pi    bool     `json:"pi"`
}

type fnPointSpec struct {
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
	On    *int     `json:"on"`
	Label string   `json:"label"`
}

type fnShadeSpec struct {
	Function int     `json:"function"`
	From     float64 `json:"from"`
	To       float64 `json:"to"`
	Label    string  `json:"label"`
}

type functionSpec struct {
	Functions []fnSpec           `json:"functions"`
	Params    map[string]float64 `json:"params"`
	XAxis     *fnAxisSpec        `json:"x_axis"`
	YAxis     *fnAxisSpec        `json:"y_axis"`
	Points    []fnPointSpec      `json:"points"`
	Shade     []fnShadeSpec      `json:"shade"`
}

const (
	fnLabelChars = 40
	fnAxisChars  = 24
	fnPointChars = 28
)

var functionPlot = core.Kind{
	Name:    "function_plot",
	Family:  "maths",
	Use:     "Graphs of functions of x drawn from their expressions (parabolas, trigonometric, exponential and log functions, rational functions with asymptotes), with marked points and shaded areas; for measured data use line_chart or scatter_chart.",
	Fields:  "functions: [{expr, label? (default \"y = expr\"; " + core.UpTo(fnLabelChars) + "), dash?: bool}] 1–6; expr uses x, numbers, + - * / ^ (power), brackets, sin cos tan asin acos atan exp ln log10 log2 sqrt abs floor ceil min max, pi, e and the params; write every product with * (2*x, not 2x); params?: {name: number} ≤8; x_axis: {label? (default x; " + core.UpTo(fnAxisChars) + "), min, max (required: the domain), pi?: bool (ticks at multiples of π/2)}; y_axis?: {label? (default y; " + core.UpTo(fnAxisChars) + "), min?, max?} (default from the functions' values); points?: [{x, y? (or on: the function's index, to compute y), label? (" + core.UpTo(fnPointChars) + ")}] ≤8, such as roots and turning points; shade?: [{function: index, from, to, label? (" + core.UpTo(fnPointChars) + ")}] ≤3, the area between the function and the x axis.",
	Example: `{"type":"function_plot","title":"y = x² − 2x − 3: roots, vertex and the area below the axis","alt":"Parabola opening upward, crossing the x axis at −1 and 3 with its vertex at (1, −4); the region between the curve and the x axis from −1 to 3 is shaded.","spec":{"functions":[{"expr":"x^2 - 2*x - 3"}],"x_axis":{"min":-3,"max":5},"y_axis":{"min":-5,"max":8},"points":[{"x":-1,"on":0,"label":"(−1, 0)"},{"x":3,"on":0,"label":"(3, 0)"},{"x":1,"on":0,"label":"Vertex (1, −4)"}],"shade":[{"function":0,"from":-1,"to":3,"label":"Area = 32/3"}]}}`,
	Parse:   parseFunction,
}

type fnPoint struct {
	x, y  float64
	label string
}

type functionDrawing struct {
	fns    []*expr.Expr
	labels []string
	dash   []bool
	x, y   *axis
	points []fnPoint
	shade  []fnShadeSpec
	pi     bool
}

func parseFunction(raw json.RawMessage) (core.Drawing, error) {
	var s functionSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	d := &functionDrawing{}
	if len(s.Params) > 8 {
		p.Add("params", "has %d parameters; at most 8", len(s.Params))
	}
	for _, name := range sortedKeys(s.Params) {
		switch {
		case !expr.ValidName(name):
			p.Add(core.Path("params", name), "is not a usable name: a letter, then letters, digits or _, at most 16 characters")
		case expr.Reserved(name):
			p.Add(core.Path("params", name), "is a name the expressions already use (x, pi, e or a function): choose another")
		}
	}
	p.Count("functions", len(s.Functions), 1, 6)
	for i, f := range s.Functions {
		path := core.Path("functions", i)
		e, err := expr.Parse(f.Expr, s.Params)
		if err != nil {
			p.Add(core.Path(path, "expr"), "%q: %v", f.Expr, err)
			d.fns = append(d.fns, nil)
		} else {
			d.fns = append(d.fns, e)
		}
		label := p.Label(core.Path(path, "label"), f.Label, false, fnLabelChars)
		if label == "" {
			label = "y = " + prettyExpr(f.Expr)
		}
		d.labels = append(d.labels, label)
		d.dash = append(d.dash, f.Dash)
	}
	if s.XAxis == nil || s.XAxis.Min == nil || s.XAxis.Max == nil {
		p.Add("x_axis", "needs min and max: the domain to draw over, such as {\"min\": -5, \"max\": 5}")
	}
	if p.Any() {
		return nil, p.Err()
	}
	xlo, xhi := *s.XAxis.Min, *s.XAxis.Max
	if xlo >= xhi {
		p.Add("x_axis", "min (%s) must be less than max (%s)", fmtValue(xlo), fmtValue(xhi))
		return nil, p.Err()
	}
	if xhi-xlo < 1e-9*math.Max(1, math.Abs(xlo)) || xhi-xlo > 1e9 {
		p.Add("x_axis", "spans %s; give a range from 1e-9 to 1e9 wide", fmtValue(xhi-xlo))
		return nil, p.Err()
	}
	d.pi = s.XAxis.Pi
	xTitle := p.Label("x_axis.label", s.XAxis.Label, false, fnAxisChars)
	if xTitle == "" {
		xTitle = "x"
	}
	// The points first: their y, given or computed.
	p.Count("points", len(s.Points), 0, 8)
	var extraY []float64
	for i, pt := range s.Points {
		at := core.Path("points", i)
		if pt.X == nil {
			p.Add(core.Path(at, "x"), "is required")
			continue
		}
		fp := fnPoint{x: *pt.X, label: p.Label(core.Path(at, "label"), pt.Label, false, fnPointChars)}
		if fp.x < xlo || fp.x > xhi {
			p.Add(core.Path(at, "x"), "%s is outside x_axis (%s to %s)", fmtValue(fp.x), fmtValue(xlo), fmtValue(xhi))
		}
		switch {
		case pt.Y != nil && pt.On != nil:
			p.Add(at, "gives both y and on: give y, or on (a function's index) to compute y")
			continue
		case pt.Y != nil:
			fp.y = *pt.Y
		case pt.On != nil:
			k := *pt.On
			if k < 0 || k >= len(d.fns) {
				p.Add(core.Path(at, "on"), "is %d; it must be a function's index, 0 to %d", k, len(d.fns)-1)
				continue
			}
			fp.y = d.fns[k].Eval(fp.x)
			if math.IsNaN(fp.y) || math.IsInf(fp.y, 0) {
				p.Add(at, "functions[%d] has no value at x = %s", k, fmtValue(fp.x))
				continue
			}
		default:
			p.Add(at, "needs y, or on (a function's index) to compute y from x")
			continue
		}
		extraY = append(extraY, fp.y)
		d.points = append(d.points, fp)
	}
	p.Count("shade", len(s.Shade), 0, 3)
	for i := range s.Shade {
		sh := &s.Shade[i]
		at := core.Path("shade", i)
		if sh.Function < 0 || sh.Function >= len(d.fns) {
			p.Add(core.Path(at, "function"), "is %d; it must be a function's index, 0 to %d", sh.Function, len(d.fns)-1)
		}
		if sh.From >= sh.To {
			p.Add(at, "from (%s) must be less than to (%s)", fmtValue(sh.From), fmtValue(sh.To))
		}
		if sh.From < xlo || sh.To > xhi {
			p.Add(at, "%s to %s runs outside x_axis (%s to %s)", fmtValue(sh.From), fmtValue(sh.To), fmtValue(xlo), fmtValue(xhi))
		}
		sh.Label = p.Label(core.Path(at, "label"), sh.Label, false, fnPointChars)
	}
	d.shade = s.Shade
	if p.Any() {
		return nil, p.Err()
	}
	// The x axis is the domain as given.
	lo0, hi0 := xlo, xhi
	d.x = resolveAxis(&p, "x_axis", &AxisSpec{Min: &lo0, Max: &hi0}, xlo, xhi, axisOpts{target: 8})
	d.x.title = xTitle
	if d.pi {
		d.x.ticks = piTicks(xlo, xhi)
		if len(d.x.ticks) < 2 {
			p.Add("x_axis.pi", "the domain %s to %s holds fewer than two multiples of π/2: widen it or leave pi out", fmtValue(xlo), fmtValue(xhi))
		}
	}
	// The y axis: given, or from the functions' values, robust to an
	// asymptote's runaway values.
	ys := extraY
	finite := 0
	for _, f := range d.fns {
		for k := 0; k <= 400; k++ {
			v := f.Eval(xlo + (xhi-xlo)*float64(k)/400)
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				ys = append(ys, v)
				finite++
			}
		}
	}
	if finite == 0 {
		p.Add("functions", "none of the functions has a value anywhere from %s to %s: check the domain (ln and sqrt need positive x)", fmtValue(xlo), fmtValue(xhi))
		return nil, p.Err()
	}
	ylo, yhi := robustRange(ys)
	ya := s.YAxis
	if ya == nil {
		ya = &fnAxisSpec{}
	}
	if ya.Pi {
		p.Add("y_axis.pi", "is available on x_axis only")
	}
	yTitle := p.Label("y_axis.label", ya.Label, false, fnAxisChars)
	if yTitle == "" {
		yTitle = "y"
	}
	if ya.Min == nil && ya.Max == nil {
		// Keep zero in view when it is near, so the x axis shows.
		span := yhi - ylo
		if ylo > 0 && ylo < span*0.5 {
			ylo = 0
		}
		if yhi < 0 && -yhi < span*0.5 {
			yhi = 0
		}
		pad := (yhi - ylo) * 0.06
		ylo, yhi = ylo-pad, yhi+pad
	}
	d.y = resolveAxis(&p, "y_axis", &AxisSpec{Min: ya.Min, Max: ya.Max}, ylo, yhi, axisOpts{target: 6})
	d.y.title = yTitle
	if p.Any() {
		return nil, p.Err()
	}
	for i, fp := range d.points {
		if !d.y.in(fp.y) {
			p.Add(core.Path("points", i), "y = %s is outside y_axis (%s to %s): widen y_axis or leave the point out", fmtValue(fp.y), fmtValue(d.y.lo), fmtValue(d.y.hi))
		}
	}
	return d, p.Err()
}

// robustRange is the range of most of vs: the 2nd to 98th percentiles,
// widened to the extremes when those are near, so one asymptote's huge
// values do not flatten everything else.
func robustRange(vs []float64) (float64, float64) {
	s := append([]float64{}, vs...)
	sort.Float64s(s)
	lo, hi := quantile(s, 0.02), quantile(s, 0.98)
	mn, mx := s[0], s[len(s)-1]
	span := math.Max(hi-lo, 1e-9)
	if lo-mn < span*0.5 {
		lo = mn
	}
	if mx-hi < span*0.5 {
		hi = mx
	}
	if hi-lo < 1e-9 {
		lo, hi = lo-1, hi+1
	}
	return lo, hi
}

// piTicks are the multiples of π/2 (or of π across a wide domain) within
// lo..hi, labelled as fractions of π.
func piTicks(lo, hi float64) []Tick {
	step := 0.5
	if (hi-lo)/math.Pi > 6 {
		step = 1
	}
	if (hi-lo)/math.Pi > 14 {
		step = 2
	}
	var out []Tick
	for k := math.Ceil(lo/math.Pi/step - 1e-9); k*step*math.Pi <= hi+1e-9 && len(out) < 30; k++ {
		m := k * step // the multiple of π
		out = append(out, Tick{V: m * math.Pi, Label: piLabel(m)})
	}
	return out
}

func piLabel(m float64) string {
	if m == 0 {
		return "0"
	}
	sign := ""
	if m < 0 {
		sign, m = "−", -m
	}
	if m == math.Trunc(m) {
		if m == 1 {
			return sign + "π"
		}
		return sign + fmtValue(m) + "π"
	}
	n := int(math.Round(m * 2))
	if n == 1 {
		return sign + "π/2"
	}
	return sign + fmtValue(float64(n)) + "π/2"
}

// prettyExpr writes an expression for a label: powers of 2 and 3 as
// superscripts, * as a middle dot, a true minus sign, pi as π.
func prettyExpr(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '^' && i+1 < len(s) && (s[i+1] == '2' || s[i+1] == '3') && (i+2 >= len(s) || !isDigitOrDot(s[i+2])):
			b.WriteString(map[byte]string{'2': "²", '3': "³"}[s[i+1]])
			i++
		case c == '*' && i > 0 && isDigitOrDot(s[i-1]) && i+1 < len(s) && (s[i+1] >= 'a' && s[i+1] <= 'z' || s[i+1] == '('):
			// 2*x reads as 2x.
		case c == '*':
			b.WriteString("·")
		case c == '-':
			b.WriteString("−")
		case c == 'p' && i+1 < len(s) && s[i+1] == 'i' && (i == 0 || !isWord(s[i-1])) && (i+2 >= len(s) || !isWord(s[i+2])):
			b.WriteString("π")
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isDigitOrDot(c byte) bool { return c >= '0' && c <= '9' || c == '.' }
func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// sampleFunction is f across the plot in figure units: adaptively, more
// densely where it bends or steepens, with NaN where the line must break
// (outside f's domain, or at a jump such as an asymptote or a step).
func (d *functionDrawing) sampleFunction(f *expr.Expr, r rect) []core.Pt {
	const n = 240
	clampY := func(v float64) float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return math.NaN()
		}
		py := d.y.px(v)
		return math.Max(r.y0-2e4, math.Min(r.y1+2e4, py))
	}
	at := func(px float64) core.Pt {
		return core.Pt{X: px, Y: clampY(f.Eval(d.x.value(px)))}
	}
	var out []core.Pt
	var refine func(a, b core.Pt, depth int)
	refine = func(a, b core.Pt, depth int) {
		an, bn := math.IsNaN(a.Y), math.IsNaN(b.Y)
		if an && bn {
			return
		}
		m := at((a.X + b.X) / 2)
		need := false
		switch {
		case an || bn:
			// The edge of the domain: find it closely.
			need = depth < 14
		default:
			dy := math.Abs(b.Y - a.Y)
			bend := math.Abs(m.Y - (a.Y+b.Y)/2)
			visible := r.grow(4).contains(a) || r.grow(4).contains(b) || (a.Y < r.y0) != (b.Y < r.y0) || (a.Y > r.y1) != (b.Y > r.y1)
			need = visible && (dy > 2.5 || bend > 0.3 || math.IsNaN(m.Y)) && depth < 10
		}
		if need {
			refine(a, m, depth+1)
			refine(m, b, depth+1)
			return
		}
		switch {
		case an:
			out = append(out, core.Pt{X: b.X, Y: math.NaN()}, b)
		case bn:
			out = append(out, b)
		default:
			// A segment still steep at the finest level is a jump.
			if depth >= 10 && math.Abs(b.Y-a.Y) > 12 {
				out = append(out, core.Pt{X: b.X, Y: math.NaN()})
			}
			out = append(out, b)
		}
	}
	prev := at(r.x0)
	out = append(out, prev)
	for k := 1; k <= n; k++ {
		next := at(r.x0 + r.w()*float64(k)/n)
		refine(prev, next, 0)
		prev = next
	}
	return out
}

// simplify drops points that lie within tol of the line kept through
// them (Douglas–Peucker), keeping the drawing small and its curves true.
func simplify(pts []core.Pt, tol float64) []core.Pt {
	if len(pts) < 3 {
		return pts
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	stack := [][2]int{{0, len(pts) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		a, b := pts[s[0]], pts[s[1]]
		dx, dy := b.X-a.X, b.Y-a.Y
		l := math.Hypot(dx, dy)
		far, at := -1.0, -1
		for i := s[0] + 1; i < s[1]; i++ {
			var dist float64
			if l == 0 {
				dist = math.Hypot(pts[i].X-a.X, pts[i].Y-a.Y)
			} else {
				dist = math.Abs((pts[i].X-a.X)*dy-(pts[i].Y-a.Y)*dx) / l
			}
			if dist > far {
				far, at = dist, i
			}
		}
		if far > tol {
			keep[at] = true
			stack = append(stack, [2]int{s[0], at}, [2]int{at, s[1]})
		}
	}
	var out []core.Pt
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

func (d *functionDrawing) Draw(c *core.Canvas) error {
	t := c.T
	p := newPlot(c, d.x, d.y)
	p.origin = true
	p.alwaysLegend = true
	p.height = 240
	for i, l := range d.labels {
		dash := seriesDash(i, len(d.fns))
		if d.dash[i] {
			dash = "6 4"
		}
		p.legend = append(p.legend, legendItem{label: core.Ellipsize(l, labelSize, 330, false), kind: legLine, colour: t.SeriesColour(i), dash: dash, shape: -1})
	}
	p.layout(0)
	for _, fp := range d.points {
		p.avoid = append(p.avoid, core.Pt{X: d.x.px(fp.x), Y: d.y.px(fp.y)})
	}
	// Shaded areas first, under the axes and the curves.
	for _, sh := range d.shade {
		f := d.fns[sh.Function]
		zero := math.Max(p.r.y0, math.Min(p.r.y1, d.y.px(0)))
		var poly []core.Pt
		x0, x1 := d.x.px(sh.From), d.x.px(sh.To)
		n := 160
		for k := 0; k <= n; k++ {
			px := x0 + (x1-x0)*float64(k)/float64(n)
			v := f.Eval(d.x.value(px))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			poly = append(poly, core.Pt{X: px, Y: math.Max(p.r.y0-10, math.Min(p.r.y1+10, d.y.px(v)))})
		}
		if len(poly) < 2 {
			continue
		}
		poly = append(poly, core.Pt{X: poly[len(poly)-1].X, Y: zero}, core.Pt{X: poly[0].X, Y: zero})
		if cl := clipPolygon(poly, p.r); len(cl) >= 3 {
			c.El("polygon", "points", core.Points(simplify(cl, 0.2)), "fill", t.SeriesColour(sh.Function), "fill-opacity", "0.18")
		}
	}
	p.drawFrame(0)
	for i, f := range d.fns {
		col := t.SeriesColour(i)
		dash := seriesDash(i, len(d.fns))
		if d.dash[i] {
			dash = "6 4"
		}
		pts := d.sampleFunction(f, p.r)
		for _, part := range clipPolyline(pts, p.r) {
			part = simplify(part, 0.15)
			c.Polyline(part, col, 2.4, dash)
			p.obs.line(part)
		}
	}
	for _, sh := range d.shade {
		if sh.Label == "" {
			continue
		}
		f := d.fns[sh.Function]
		mid := (sh.From + sh.To) / 2
		v := f.Eval(mid)
		zero := math.Max(p.r.y0, math.Min(p.r.y1, d.y.px(0)))
		cy := (zero + math.Max(p.r.y0, math.Min(p.r.y1, d.y.px(v)))) / 2
		at := core.Pt{X: d.x.px(mid), Y: cy}
		col := t.SeriesColour(sh.Function)
		r := textRect(at.X, at.Y, sh.Label, labelSize, true, core.Middle)
		if p.obs.cost(r) == 0 && r.inside(p.r) {
			c.Text(at.X, at.Y, sh.Label, core.TextStyle{Size: labelSize, Bold: true, Fill: col, Anchor: core.Middle, Halo: t.Bg})
			p.obs.rect(r)
		} else {
			p.annotate(at, sh.Label, col, false, true)
		}
	}
	for _, fp := range d.points {
		at := core.Pt{X: d.x.px(fp.x), Y: d.y.px(fp.y)}
		c.Circle(at.X, at.Y, 4, t.Ink, t.Bg, 1.5)
		p.obs.point(at)
	}
	for _, fp := range d.points {
		if fp.label == "" {
			continue
		}
		at := core.Pt{X: d.x.px(fp.x), Y: d.y.px(fp.y)}
		p.annotate(at, fp.label, t.Ink, false, true)
	}
	c.H = p.bottom
	return nil
}
