package diagram

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// The diagrams share a few measures: the side margin, the size of the text
// inside a shape and of the labels on lines, and how tall a body may be.

const (
	// margin is the clear space at each side of a body.
	margin = 14.0
	// avail is the width a body lays out in.
	avail = core.Width - 2*margin
	// nodeSize is the text inside a node; labelSize the text on a line.
	nodeSize  = 12.5
	labelSize = core.Small
	// maxBody is the tallest a body may be, leaving room for a two-line
	// title and a caption within core.MaxHeight.
	maxBody = core.MaxHeight - 140
)

// box is a rectangle in figure units: X, Y its top left.
type box struct{ X, Y, W, H float64 }

func (b box) R() float64  { return b.X + b.W }
func (b box) B() float64  { return b.Y + b.H }
func (b box) CX() float64 { return b.X + b.W/2 }
func (b box) CY() float64 { return b.Y + b.H/2 }

// overlaps says whether b and o share more than a hairline, with pad
// clear space wanted between them.
func (b box) overlaps(o box, pad float64) bool {
	return b.X < o.R()+pad && o.X < b.R()+pad && b.Y < o.B()+pad && o.Y < b.B()+pad
}

// contains says whether o lies inside b.
func (b box) contains(o box) bool {
	return o.X >= b.X-0.01 && o.Y >= b.Y-0.01 && o.R() <= b.R()+0.01 && o.B() <= b.B()+0.01
}

// grow is b with d added on every side.
func (b box) grow(d float64) box { return box{b.X - d, b.Y - d, b.W + 2*d, b.H + 2*d} }

// segHitsBox says whether the axis-aligned or sloping segment a–b passes
// through the inside of r (its border excluded by shrinking it a little).
func segHitsBox(a, b core.Pt, r box) bool {
	r = r.grow(-1.5)
	if r.W <= 0 || r.H <= 0 {
		return false
	}
	// Liang–Barsky clipping of the segment against the rectangle.
	t0, t1 := 0.0, 1.0
	dx, dy := b.X-a.X, b.Y-a.Y
	for _, e := range [][2]float64{{-dx, a.X - r.X}, {dx, r.R() - a.X}, {-dy, a.Y - r.Y}, {dy, r.B() - a.Y}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return false
			}
			continue
		}
		t := q / p
		if p < 0 {
			t0 = math.Max(t0, t)
		} else {
			t1 = math.Min(t1, t)
		}
		if t0 > t1 {
			return false
		}
	}
	return t1-t0 > 1e-6
}

// textBlock is wrapped text and the size it takes.
type textBlock struct {
	lines []string
	size  float64
	bold  bool
	w, h  float64
}

func wrapBlock(s string, size, max float64, bold bool) textBlock {
	lines := balanced(s, size, max, bold)
	return textBlock{lines: lines, size: size, bold: bold, w: core.WidestLine(lines, size, bold), h: core.BlockHeight(len(lines), size, 0)}
}

// fitSize is the largest text size from size down to core.Small at
// which every word of s fits in max, so a word is broken only when it
// must be.
func fitSize(s string, size, max float64, bold bool) float64 {
	for ; size > core.Small; size -= 0.5 {
		ok := true
		for _, w := range strings.Fields(s) {
			if core.TextWidth(w, size, bold) > max {
				ok = false
				break
			}
		}
		if ok {
			return size
		}
	}
	return core.Small
}

// wrapPara is wrapBlock for running text, filled line by line.
func wrapPara(s string, size, max float64, bold bool) textBlock {
	lines := core.Wrap(s, size, max, bold)
	return textBlock{lines: lines, size: size, bold: bold, w: core.WidestLine(lines, size, bold), h: core.BlockHeight(len(lines), size, 0)}
}

// balanced wraps s in as few lines as max allows, with the lines as even
// as they can be, so a box does not end on one stranded word.
func balanced(s string, size, max float64, bold bool) []string {
	lines := core.Wrap(s, size, max, bold)
	if len(lines) < 2 {
		return lines
	}
	lo, hi := max/float64(len(lines)+1), max
	for _, w := range strings.Fields(s) {
		lo = math.Max(lo, math.Min(max, core.TextWidth(w, size, bold)))
	}
	if len(core.Wrap(s, size, lo, bold)) <= len(lines) {
		return core.Wrap(s, size, lo, bold)
	}
	for i := 0; i < 14; i++ {
		mid := (lo + hi) / 2
		if len(core.Wrap(s, size, mid, bold)) > len(lines) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return core.Wrap(s, size, hi, bold)
}

// draw writes the block centred on (x, y), or starting at x when anchor
// is core.Start.
func (t textBlock) draw(c *core.Canvas, x, y float64, fill string, anchor core.Anchor, italic bool) {
	c.Lines(x, y, t.lines, core.TextStyle{Size: t.size, Bold: t.bold, Italic: italic, Fill: fill, Anchor: anchor}, 0)
}

// label is a short text placed on or beside a line, drawn over a halo of
// the background so it reads where it crosses a stroke.
type label struct {
	text   textBlock
	at     box // where the text sits
	italic bool
	fill   string // "" is the theme's muted ink
}

func (l label) draw(c *core.Canvas) {
	fill := l.fill
	if fill == "" {
		fill = c.T.Muted
	}
	c.Lines(l.at.CX(), l.at.CY(), l.text.lines, core.TextStyle{Size: l.text.size, Bold: l.text.bold, Italic: l.italic,
		Fill: fill, Anchor: core.Middle, Halo: c.T.Bg}, 0)
}

// roundedPath is an orthogonal polyline as path data with its corners
// rounded to radius r (less where a leg is short).
func roundedPath(pts []core.Pt, r float64) string {
	pts = simplify(pts)
	if len(pts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("M" + core.F(pts[0].X) + " " + core.F(pts[0].Y))
	for i := 1; i < len(pts); i++ {
		p := pts[i]
		if i == len(pts)-1 {
			b.WriteString("L" + core.F(p.X) + " " + core.F(p.Y))
			break
		}
		prev, next := pts[i-1], pts[i+1]
		l1, l2 := dist(prev, p), dist(p, next)
		rr := math.Min(r, math.Min(l1/2, l2/2))
		a := toward(p, prev, rr)
		z := toward(p, next, rr)
		b.WriteString("L" + core.F(a.X) + " " + core.F(a.Y) + "Q" + core.F(p.X) + " " + core.F(p.Y) + " " + core.F(z.X) + " " + core.F(z.Y))
	}
	return b.String()
}

// simplify drops repeated points and the middle of three in a line.
func simplify(pts []core.Pt) []core.Pt {
	var out []core.Pt
	for _, p := range pts {
		if n := len(out); n > 0 && math.Abs(out[n-1].X-p.X) < 0.01 && math.Abs(out[n-1].Y-p.Y) < 0.01 {
			continue
		}
		if n := len(out); n >= 2 {
			a, b := out[n-2], out[n-1]
			if (math.Abs(a.X-b.X) < 0.01 && math.Abs(b.X-p.X) < 0.01) || (math.Abs(a.Y-b.Y) < 0.01 && math.Abs(b.Y-p.Y) < 0.01) {
				out[n-1] = p
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func dist(a, b core.Pt) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }

// toward is the point d from a in the direction of b.
func toward(a, b core.Pt, d float64) core.Pt {
	l := dist(a, b)
	if l == 0 {
		return a
	}
	return core.Pt{X: a.X + (b.X-a.X)/l*d, Y: a.Y + (b.Y-a.Y)/l*d}
}

// head says how a line ends at its target.
type head int

const (
	headArrow head = iota
	headBar
	headNone
)

// arrowLen is how far an arrowhead of a line of width w reaches back.
func arrowLen(w float64) float64 { return 5 + w*1.6 + 1.5 }

// drawRoute draws an orthogonal route with rounded corners ending in h.
func drawRoute(c *core.Canvas, pts []core.Pt, stroke string, w float64, dashed bool, h head) {
	pts = simplify(pts)
	if len(pts) < 2 {
		return
	}
	n := len(pts)
	line := append([]core.Pt{}, pts...)
	end, before := pts[n-1], pts[n-2]
	switch h {
	case headArrow:
		line[n-1] = toward(end, before, arrowLen(w)*0.8)
	case headBar:
		line[n-1] = toward(end, before, 1.5)
	}
	var extra []string
	if dashed {
		extra = []string{"stroke-dasharray", "4 3"}
	}
	c.Path(roundedPath(line, 7), "", stroke, w, extra...)
	switch h {
	case headArrow:
		c.Polygon(core.ArrowHead(toward(end, before, 10), end, arrowLen(w)), stroke, "", 0)
	case headBar:
		c.BarHead(before, toward(end, before, 1.5), stroke, w)
	}
}

// toneOf is a node's stroke and fill: its tone's, or def's when it has
// none.
func toneOf(t *core.Theme, tone string, defStroke, defFill string) (string, string) {
	if tone == "" {
		return defStroke, defFill
	}
	return t.Tone(tone)
}

// decodeAt decodes raw strictly as core.Decode does, with its problems
// placed at path rather than at the spec.
func decodeAt(p *core.Problems, path string, raw json.RawMessage, v any) bool {
	if err := core.Decode(raw, v); err != nil {
		r := err.(*core.Refusal)
		for _, msg := range r.Problems {
			msg = strings.Replace(msg, "spec has a field", "has a field", 1)
			msg = strings.TrimPrefix(msg, "spec.")
			msg = strings.TrimPrefix(msg, "spec")
			msg = strings.TrimPrefix(msg, ": ")
			msg = strings.TrimPrefix(msg, " ")
			p.Add(path, "%s", msg)
		}
		return false
	}
	return true
}

// idList is ids quoted for a message: "a, b, c", shortened past twelve.
func idList(ids []string) string {
	if len(ids) > 12 {
		return strings.Join(ids[:12], ", ") + fmt.Sprintf(" (and %d more)", len(ids)-12)
	}
	return strings.Join(ids, ", ")
}

// tooTall is the refusal for a body that does not fit core.MaxHeight.
func tooTall(p *core.Problems, what string, h float64, how string) {
	p.Add("", "the %s would be %.0f units tall, more than the %.0f that fit on a phone; %s", what, h, maxBody, how)
}
