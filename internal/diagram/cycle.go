package diagram

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var cycleKind = core.Kind{
	Name:   "cycle",
	Family: "diagram",
	Use:    "A repeating sequence of 3 to 8 steps drawn round a circle with arrows (the cardiac cycle, a simplified Krebs cycle, a PDSA cycle); for a sequence with an end use flowchart or timeline.",
	Fields: "steps: [{label (" + core.UpTo(cyLabelChars) + "), detail? (" + core.UpTo(cyDetailChars) + "), tone?}] (3 to 8), in order, clockwise from the top. centre?: a label in the middle (" + core.UpTo(cyLabelChars) + ").",
	Example: `{"type":"cycle","title":"The cardiac cycle","alt":"The five phases of the cardiac cycle in order: atrial systole, isovolumetric contraction, ventricular ejection, isovolumetric relaxation and passive ventricular filling, then round again.","spec":{"centre":"Cardiac cycle","steps":[` +
		`{"label":"Atrial systole","detail":"Atria top up the ventricles"},` +
		`{"label":"Isovolumetric contraction","detail":"AV valves close (S1)","tone":"accent"},` +
		`{"label":"Ventricular ejection","detail":"Aortic and pulmonary valves open"},` +
		`{"label":"Isovolumetric relaxation","detail":"Semilunar valves close (S2)","tone":"accent"},` +
		`{"label":"Passive filling","detail":"AV valves open"}]}}`,
	Parse: parseCycle,
}

type cyStep struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type cySpec struct {
	Steps  []cyStep `json:"steps"`
	Centre string   `json:"centre"`
}

type cycle struct {
	steps    []cyStep
	lab, det []textBlock
	centre   textBlock
	w, h     float64
	cx, cy   float64
	rx, ry   float64
	boxes    []box
	H        float64
	arcs     [][2]float64 // each arrow's start and tip, as ellipse angles
}

func parseCycle(raw json.RawMessage) (core.Drawing, error) {
	var s cySpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("steps", len(s.Steps), 3, 8)
	for i := range s.Steps {
		st := &s.Steps[i]
		st.Label = p.Label(core.Path("steps", i, "label"), st.Label, true, cyLabelChars)
		st.Detail = p.Label(core.Path("steps", i, "detail"), st.Detail, false, cyDetailChars)
		p.Tone(core.Path("steps", i, "tone"), st.Tone)
	}
	centre := p.Label("centre", s.Centre, false, cyLabelChars)
	if p.Any() {
		return nil, p.Err()
	}
	c := &cycle{steps: s.Steps}
	c.layout(centre)
	return c, nil
}

// cyPad is the room either side of a step's text, together.
const (
	cyPad         = 12.0
	cyLabelChars  = 40
	cyDetailChars = 70
)

func (c *cycle) pt(a float64) core.Pt {
	return core.Pt{X: c.cx + c.rx*math.Cos(a), Y: c.cy + c.ry*math.Sin(a)}
}

func (c *cycle) layout(centre string) {
	n := len(c.steps)
	w0 := 136.0
	switch {
	case n >= 7:
		w0 = 108
	case n >= 5:
		w0 = 122
	}
	c.cx = core.Width / 2
	if centre != "" {
		c.centre = wrapBlock(centre, 14, 120, true)
	}
	// Try narrower boxes until the ring fits without growing too tall:
	// the boxes clear each other, each arrow has room, and the centre
	// label is clear of every box.
	// No box is narrower than its longest word needs.
	floor := 84.0
	for _, st := range c.steps {
		for _, w := range strings.Fields(st.Label) {
			floor = max(floor, core.TextWidth(w, core.Small, true)+cyPad+2)
		}
	}
	for c.w = max(w0, floor); ; c.w = max(floor, c.w-6) {
		c.size()
		c.rx = (avail - c.w) / 2
		last := c.w <= floor
		for c.ry = max(70, c.rx*0.62); c.ry <= c.rx*1.6 || (last && c.ry < 420); c.ry += 4 {
			if c.fits(centre != "") {
				c.H = c.cy + c.ry + c.h/2 + 4
				return
			}
		}
		if last {
			c.H = c.cy + c.ry + c.h/2 + 4
			return
		}
	}
}

// size sets each step's text and the boxes' common height for width c.w.
func (c *cycle) size() {
	n := len(c.steps)
	c.lab, c.det = make([]textBlock, n), make([]textBlock, n)
	c.h = 0
	for i, s := range c.steps {
		c.lab[i] = wrapBlock(s.Label, fitSize(s.Label, 12.5, c.w-cyPad, true), c.w-cyPad, true)
		h := c.lab[i].h
		if s.Detail != "" {
			c.det[i] = wrapBlock(s.Detail, labelSize, c.w-cyPad, false)
			h += c.det[i].h + 3
		}
		c.h = max(c.h, h+16)
	}
}

func (c *cycle) fits(centre bool) bool {
	n := len(c.steps)
	c.cy = c.h/2 + c.ry + 2
	c.boxes = c.boxes[:0]
	for i := 0; i < n; i++ {
		p := c.pt(c.angle(i))
		c.boxes = append(c.boxes, box{p.X - c.w/2, p.Y - c.h/2, c.w, c.h})
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if c.boxes[i].overlaps(c.boxes[j], 10) {
				return false
			}
		}
	}
	c.arcs = c.arcs[:0]
	for i := 0; i < n; i++ {
		a0, a1 := c.angle(i), c.angle(i+1)
		s := c.leave(c.boxes[i].grow(5), a0, a1, 1)
		e := c.leave(c.boxes[(i+1)%n].grow(5), a1, a0, -1)
		if dist(c.pt(s), c.pt(e)) < 22 {
			return false
		}
		c.arcs = append(c.arcs, [2]float64{s, e})
	}
	if centre {
		cb := box{c.cx - c.centre.w/2 - 8, c.cy - c.centre.h/2 - 6, c.centre.w + 16, c.centre.h + 12}
		for _, b := range c.boxes {
			if cb.overlaps(b, 4) {
				return false
			}
		}
	}
	return true
}

func (c *cycle) angle(i int) float64 {
	return -math.Pi/2 + 2*math.Pi*float64(i)/float64(len(c.steps))
}

// leave is the angle, moving from a towards b (dir +1 increasing), at
// which the ellipse leaves r.
func (c *cycle) leave(r box, a, b float64, dir float64) float64 {
	if dir > 0 && b < a {
		b += 2 * math.Pi
	}
	if dir < 0 && b > a {
		b -= 2 * math.Pi
	}
	steps := 400
	for k := 0; k <= steps; k++ {
		t := a + (b-a)*float64(k)/float64(steps)
		p := c.pt(t)
		if !(p.X > r.X && p.X < r.R() && p.Y > r.Y && p.Y < r.B()) {
			return t
		}
	}
	return b
}

func (c *cycle) Draw(cv *core.Canvas) error {
	t := cv.T
	for _, a := range c.arcs {
		s, e := a[0], a[1]
		for e < s {
			e += 2 * math.Pi
		}
		tip := c.pt(e)
		// The head's base is a head-length back along the ellipse.
		base := e
		for base > s && dist(c.pt(base), tip) < 9 {
			base -= 0.002
		}
		p0, p1 := c.pt(s), c.pt(base)
		large := "0"
		if base-s > math.Pi {
			large = "1"
		}
		d := "M" + core.F(p0.X) + " " + core.F(p0.Y) + "A" + core.F(c.rx) + " " + core.F(c.ry) + " 0 " + large + " 1 " + core.F(p1.X) + " " + core.F(p1.Y)
		cv.Path(d, "", t.Muted, 2)
		cv.Polygon(core.ArrowHead(c.pt(base-0.05), tip, 10), t.Muted, "", 0)
	}
	if len(c.centre.lines) > 0 {
		c.centre.draw(cv, c.cx, c.cy, t.Ink, core.Middle, false)
	}
	for i, s := range c.steps {
		b := c.boxes[i]
		stroke, fill := toneOf(t, s.Tone, t.Edge, t.Surface)
		cv.Rect(b.X, b.Y, b.W, b.H, 8, fill, stroke, 1.3)
		th := c.lab[i].h
		if s.Detail != "" {
			th += c.det[i].h + 3
		}
		top := b.CY() - th/2
		c.lab[i].draw(cv, b.CX(), top+c.lab[i].h/2, t.Ink, core.Middle, false)
		if s.Detail != "" {
			c.det[i].draw(cv, b.CX(), top+c.lab[i].h+3+c.det[i].h/2, t.Muted, core.Middle, false)
		}
	}
	cv.H = c.H
	return nil
}
