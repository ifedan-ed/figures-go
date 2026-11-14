package core

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
)

// Width is every figure's width in its own units. Drawn for a phone first:
// at 340 px across, the body text (13) is still 11 px, and a figure is tall
// rather than wide when it has a lot to say. Pages scale it to fit; a tap
// opens it larger.
const Width = 400.0

// Text sizes, in figure units. Nothing is drawn smaller than Small.
const (
	TitleSize = 15.0
	Body      = 13.0
	Small     = 11.0
)

// MaxHeight is the tallest a figure may be: a flowchart longer than this
// is too long to follow on a phone and is refused, not shrunk.
const MaxHeight = 1600.0

// FontFamily is the system's own interface font first, then the common
// fallbacks; react-native-svg takes the first it knows.
const FontFamily = "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"

// Canvas collects one drawing's SVG elements. Coordinates are figure units
// from the top left; the body of a figure starts at y 0 and says how tall
// it is by setting H. Every method writes only elements and attributes
// that Sanitise keeps, which a test checks on every example.
type Canvas struct {
	W, H float64
	T    *Theme
	b    strings.Builder
}

// NewCanvas is an empty body drawn in theme t.
func NewCanvas(t *Theme) *Canvas { return &Canvas{W: Width, T: t} }

// Body is what has been drawn so far.
func (c *Canvas) Body() string { return c.b.String() }

// Raw appends drawing made by another canvas (a part drawn first and
// placed later).
func (c *Canvas) Raw(s string) { c.b.WriteString(s) }

// F formats a coordinate: at most two decimals, no trailing zeros.
func F(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	v = math.Round(v*100) / 100
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// El writes a self-closing element; attrs are name, value pairs.
func (c *Canvas) El(name string, attrs ...string) {
	c.open(name, attrs)
	c.b.WriteString("/>")
}

// Open starts an element that holds others; Close ends it.
func (c *Canvas) Open(name string, attrs ...string) {
	c.open(name, attrs)
	c.b.WriteString(">")
}

// Close ends the element Open started.
func (c *Canvas) Close(name string) { c.b.WriteString("</" + name + ">") }

func (c *Canvas) open(name string, attrs []string) {
	c.b.WriteString("<" + name)
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i+1] == "" {
			continue
		}
		c.b.WriteString(" " + attrs[i] + `="` + Escape(attrs[i+1]) + `"`)
	}
}

// Escape makes s safe as XML text or an attribute value, escaped exactly
// as encoding/xml escapes it, so Sanitise gives Render's output back byte
// for byte. Characters XML cannot hold are dropped.
func Escape(s string) string {
	clean := strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t' && r != '\n') || r == 0xFFFE || r == 0xFFFF || (r >= 0xD800 && r <= 0xDFFF) {
			return -1
		}
		return r
	}, s)
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(clean))
	return b.String()
}

// Rect is a rectangle; r rounds its corners.
func (c *Canvas) Rect(x, y, w, h, r float64, fill, stroke string, strokeW float64) {
	c.El("rect", "x", F(x), "y", F(y), "width", F(math.Max(0, w)), "height", F(math.Max(0, h)),
		"rx", nz(r), "fill", or(fill, "none"), "stroke", stroke, "stroke-width", sw(stroke, strokeW))
}

// Line is a straight stroke; dash is a stroke-dasharray or "".
func (c *Canvas) Line(x1, y1, x2, y2 float64, stroke string, w float64, dash string) {
	c.El("line", "x1", F(x1), "y1", F(y1), "x2", F(x2), "y2", F(y2), "stroke", stroke, "stroke-width", F(w),
		"stroke-dasharray", dash, "stroke-linecap", "round")
}

// Circle is a circle.
func (c *Canvas) Circle(cx, cy, r float64, fill, stroke string, strokeW float64) {
	c.El("circle", "cx", F(cx), "cy", F(cy), "r", F(r), "fill", or(fill, "none"), "stroke", stroke, "stroke-width", sw(stroke, strokeW))
}

// Ellipse is an ellipse.
func (c *Canvas) Ellipse(cx, cy, rx, ry float64, fill, stroke string, strokeW float64) {
	c.El("ellipse", "cx", F(cx), "cy", F(cy), "rx", F(rx), "ry", F(ry), "fill", or(fill, "none"), "stroke", stroke, "stroke-width", sw(stroke, strokeW))
}

// Pt is a point in figure units.
type Pt struct{ X, Y float64 }

// Points is pts as a points attribute.
func Points(pts []Pt) string {
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(F(p.X) + "," + F(p.Y))
	}
	return b.String()
}

// Polyline is an open line through pts.
func (c *Canvas) Polyline(pts []Pt, stroke string, w float64, dash string) {
	c.El("polyline", "points", Points(pts), "fill", "none", "stroke", stroke, "stroke-width", F(w),
		"stroke-dasharray", dash, "stroke-linejoin", "round", "stroke-linecap", "round")
}

// Polygon is a closed shape through pts.
func (c *Canvas) Polygon(pts []Pt, fill, stroke string, strokeW float64) {
	c.El("polygon", "points", Points(pts), "fill", or(fill, "none"), "stroke", stroke, "stroke-width", sw(stroke, strokeW),
		"stroke-linejoin", "round")
}

// Path is a path; d is its data (PathData builds one).
func (c *Canvas) Path(d, fill, stroke string, strokeW float64, extra ...string) {
	attrs := append([]string{"d", d, "fill", or(fill, "none"), "stroke", stroke, "stroke-width", sw(stroke, strokeW),
		"stroke-linejoin", "round", "stroke-linecap", "round"}, extra...)
	c.El("path", attrs...)
}

// Anchor is where a text's x is: its start, middle or end.
type Anchor string

const (
	Start  Anchor = "start"
	Middle Anchor = "middle"
	End    Anchor = "end"
)

// TextStyle is how a text is set.
type TextStyle struct {
	Size   float64
	Bold   bool
	Italic bool
	Fill   string
	Anchor Anchor
	// Halo draws the text over a stroke of the background, so a label
	// stays legible where it crosses a line or a fill.
	Halo string
}

// Text writes one line with its vertical middle at y. The baseline is
// placed by arithmetic, not dominant-baseline, which react-native-svg does
// not honour everywhere.
func (c *Canvas) Text(x, y float64, s string, st TextStyle) {
	if s == "" {
		return
	}
	size := st.Size
	if size == 0 {
		size = Body
	}
	base := y + size*0.35
	attrs := []string{"x", F(x), "y", F(base), "font-size", F(size), "fill", or(st.Fill, c.T.Ink)}
	if st.Anchor != "" && st.Anchor != Start {
		attrs = append(attrs, "text-anchor", string(st.Anchor))
	}
	if st.Bold {
		attrs = append(attrs, "font-weight", "700")
	}
	if st.Italic {
		attrs = append(attrs, "font-style", "italic")
	}
	if st.Halo != "" {
		halo := append(append([]string{}, attrs...), "stroke", st.Halo, "stroke-width", F(size*0.28), "stroke-linejoin", "round")
		halo[7] = st.Halo // fill
		c.open("text", halo)
		c.b.WriteString(">" + Escape(s) + "</text>")
	}
	c.open("text", attrs)
	c.b.WriteString(">" + Escape(s) + "</text>")
}

// Lines writes lines one under another, centred as a block on y; lead is
// the distance between baselines (0 is 1.25 × size). It returns the
// block's height.
func (c *Canvas) Lines(x, y float64, lines []string, st TextStyle, lead float64) float64 {
	size := st.Size
	if size == 0 {
		size = Body
	}
	if lead == 0 {
		lead = size * 1.25
	}
	h := lead * float64(len(lines))
	top := y - h/2 + lead/2
	for i, l := range lines {
		c.Text(x, top+float64(i)*lead, l, st)
	}
	return h
}

// BlockHeight is how tall n lines at size are, with lead 0 as in Lines.
func BlockHeight(n int, size, lead float64) float64 {
	if lead == 0 {
		lead = size * 1.25
	}
	return lead * float64(n)
}

// Arrow draws a line from a to b ending in a filled head at b, drawn as a
// polygon rather than an SVG marker so every viewer shows it the same.
func (c *Canvas) Arrow(a, b Pt, stroke string, w float64, dash string) {
	c.ArrowPath([]Pt{a, b}, stroke, w, dash, true, false)
}

// ArrowPath draws a polyline through pts with a head at its end (and at
// its start when both).
func (c *Canvas) ArrowPath(pts []Pt, stroke string, w float64, dash string, end, start bool) {
	if len(pts) < 2 {
		return
	}
	head := 5 + w*1.6
	line := append([]Pt{}, pts...)
	n := len(line)
	if end {
		line[n-1] = back(line[n-2], line[n-1], head*0.8)
	}
	if start {
		line[0] = back(line[1], line[0], head*0.8)
	}
	c.Polyline(line, stroke, w, dash)
	if end {
		c.Polygon(ArrowHead(pts[n-2], pts[n-1], head), stroke, "", 0)
	}
	if start {
		c.Polygon(ArrowHead(pts[1], pts[0], head), stroke, "", 0)
	}
}

// BarHead draws the flat end of an inhibition (⊣) at b, across the line
// from a.
func (c *Canvas) BarHead(a, b Pt, stroke string, w float64) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*7, dx/l*7
	c.Line(b.X+nx, b.Y+ny, b.X-nx, b.Y-ny, stroke, w*1.6, "")
}

// ArrowHead is the triangle of a head of length size pointing from a to b,
// its tip at b.
func ArrowHead(a, b Pt, size float64) []Pt {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	ux, uy := dx/l, dy/l
	bx, by := b.X-ux*size, b.Y-uy*size
	hw := size * 0.5
	return []Pt{{b.X, b.Y}, {bx - uy*hw, by + ux*hw}, {bx + uy*hw, by - ux*hw}}
}

// back is the point d short of b along a→b.
func back(a, b Pt, d float64) Pt {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l <= d {
		return a
	}
	return Pt{b.X - dx/l*d, b.Y - dy/l*d}
}

// Group wraps what fn draws in a <g> moved by (dx, dy).
func (c *Canvas) Group(dx, dy float64, fn func()) {
	c.Open("g", "transform", "translate("+F(dx)+" "+F(dy)+")")
	fn()
	c.Close("g")
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nz(v float64) string {
	if v == 0 {
		return ""
	}
	return F(v)
}

func sw(stroke string, w float64) string {
	if stroke == "" || stroke == "none" {
		return ""
	}
	if w == 0 {
		w = 1
	}
	return F(w)
}
