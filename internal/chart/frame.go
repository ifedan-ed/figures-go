package chart

import (
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// The look every chart shares, in figure units.
const (
	padX      = 14.0 // the figure's side margin, as the title's
	tickSize  = core.Small
	labelSize = 12.0 // axis titles, legends, end labels
	plotH     = 196.0
	rowH      = 17.0 // a legend row
)

// legend item kinds.
const (
	legLine = iota
	legBox
	legPoint
)

type legendItem struct {
	label  string
	kind   int
	colour string
	fill   string
	dash   string
	shape  int
}

// plot lays out and draws a chart's frame: the legend, the axes' titles,
// ticks and grid, around a plot area the chart draws its data in.
type plot struct {
	c    *core.Canvas
	x, y *axis
	// legend is drawn at the top, in rows.
	legend []legendItem
	// rightPad is room right of the plot for labels at the ends of lines.
	rightPad float64
	// height is the plot area's height.
	height float64
	// xGrid draws vertical grid lines too (scatter plots, growth charts).
	xGrid bool
	// origin draws the axes through zero when zero is in view, as in a
	// maths textbook.
	origin bool
	// r is the plot area once laid out.
	r rect
	// obs is what labels placed later must avoid.
	obs obstacles
	// catLines is the rows a category axis's labels need.
	catLines [][]string
	// bottom is the body's end.
	bottom float64
	// alwaysLegend shows the legend for a single item too.
	alwaysLegend bool
	// avoid are marked points a tick label must not cover; such a label
	// is left out.
	avoid []core.Pt
	// titles are the axis names' boxes in a function plot.
	titles []rect
}

func newPlot(c *core.Canvas, x, y *axis) *plot {
	return &plot{c: c, x: x, y: y, height: plotH}
}

// legendRows breaks the legend into rows that fit the width.
func (p *plot) legendRows() [][]legendItem {
	var rows [][]legendItem
	var row []legendItem
	w := 0.0
	avail := core.Width - 2*padX
	for _, it := range p.legend {
		iw := legendItemWidth(it)
		if len(row) > 0 && w+iw > avail {
			rows = append(rows, row)
			row, w = nil, 0
		}
		row = append(row, it)
		w += iw
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func legendItemWidth(it legendItem) float64 {
	return 22 + core.TextWidth(it.label, labelSize, false) + 14
}

// yLabelWidth is the width of the widest y tick label.
func (p *plot) yLabelWidth() float64 {
	if p.y.cats != nil {
		w := 0.0
		for _, lines := range p.catLines {
			w = math.Max(w, core.WidestLine(lines, tickSize, false))
		}
		return w
	}
	w := 0.0
	for _, t := range p.y.ticks {
		w = math.Max(w, core.TextWidth(t.Label, tickSize, false))
	}
	return w
}

// layout places the plot area and sets the axes' coordinates. top is
// where the body starts (below anything drawn above the chart).
func (p *plot) layout(top float64) {
	y := top
	if rows := p.legendRows(); len(rows) > 0 && (len(p.legend) > 1 || p.alwaysLegend) {
		y += float64(len(rows))*rowH + 6
	}
	if p.y.title != "" && !p.origin {
		y += 17
	}
	y += 7
	left := padX + p.yLabelWidth() + 7
	if p.origin && p.x.lo < 0 {
		// The y axis runs inside the plot, its labels beside it.
		left = padX + 4
	}
	right := core.Width - padX - p.rightPad
	// The last x tick label must not run off the figure.
	if p.x.cats == nil && len(p.x.ticks) > 0 && p.rightPad < 4 {
		last := p.x.ticks[len(p.x.ticks)-1]
		if p.x.frac(last.V) > 0.999 {
			right = math.Min(right, core.Width-6-core.TextWidth(last.Label, tickSize, false)/2)
		}
	}
	p.r = rect{left, y, right, y + p.height}
	p.x.a, p.x.b = left, right
	if p.y.cats != nil {
		p.y.a, p.y.b = p.r.y0, p.r.y1
	} else {
		p.y.a, p.y.b = p.r.y1, p.r.y0
	}
	b := p.r.y1 + 6
	if p.x.cats != nil {
		n := 1
		for _, l := range p.catLines {
			n = max(n, len(l))
		}
		b += core.BlockHeight(n, tickSize, 0) + 4
	} else {
		b += 16
	}
	if p.x.title != "" && !p.origin {
		b += 18
	}
	p.bottom = b + 2
}

// drawLegend draws the legend's rows from top.
func (p *plot) drawLegend(top float64) {
	if len(p.legend) < 2 && !(p.alwaysLegend && len(p.legend) == 1) {
		return
	}
	c := p.c
	for i, row := range p.legendRows() {
		y := top + float64(i)*rowH + rowH/2
		x := padX
		for _, it := range row {
			switch it.kind {
			case legLine:
				c.Line(x, y, x+18, y, it.colour, 2.2, it.dash)
				if it.shape >= 0 {
					drawMarker(c, it.shape, x+9, y, 3.2, it.colour)
				}
			case legBox:
				c.Rect(x+3, y-6, 12, 12, 2, it.fill, it.colour, 1)
			case legPoint:
				drawMarker(c, it.shape, x+9, y, 3.6, it.colour)
			}
			c.Text(x+22, y, it.label, core.TextStyle{Size: labelSize, Fill: c.T.Ink})
			x += legendItemWidth(it)
		}
	}
}

// drawFrame draws the legend, the grid, the axes and their titles. It is
// drawn before the data, which goes over the grid.
func (p *plot) drawFrame(top float64) {
	c := p.c
	t := c.T
	p.drawLegend(top)
	if p.y.title != "" && !p.origin {
		ty := p.r.y0 - 7 - 10
		c.Text(padX, ty, core.Ellipsize(p.y.title, labelSize, core.Width-2*padX, false), core.TextStyle{Size: labelSize, Fill: t.Muted})
	}
	if p.origin {
		p.drawOriginAxes()
		return
	}
	// Horizontal grid and y labels.
	if p.y.cats == nil {
		ys := p.visibleTicks(p.y, false)
		for _, tk := range p.y.ticks {
			py := p.y.px(tk.V)
			col, op := t.Faint, ""
			if tk.Minor {
				op = "0.6"
			}
			if math.Abs(py-p.r.y1) < 0.5 {
				continue // the axis line is drawn there
			}
			c.El("line", "x1", core.F(p.r.x0), "y1", core.F(py), "x2", core.F(p.r.x1), "y2", core.F(py),
				"stroke", col, "stroke-width", "1", "stroke-opacity", op)
		}
		for _, tk := range ys {
			c.Text(p.r.x0-7, p.y.px(tk.V), tk.Label, core.TextStyle{Size: tickSize, Fill: t.Muted, Anchor: core.End})
		}
		// Zero, when the axis crosses it, is a stronger line.
		if !p.y.log && p.y.lo < 0 && p.y.hi > 0 {
			zy := p.y.px(0)
			c.Line(p.r.x0, zy, p.r.x1, zy, t.Edge, 1, "")
		}
	} else {
		for i, lines := range p.catLines {
			cy, _ := p.y.band(i)
			c.Lines(p.r.x0-8, cy, lines, core.TextStyle{Size: tickSize, Fill: t.Ink, Anchor: core.End}, 0)
		}
	}
	// Vertical grid and x labels.
	if p.x.cats == nil {
		for _, tk := range p.x.ticks {
			px := p.x.px(tk.V)
			if p.xGrid && math.Abs(px-p.r.x0) > 0.5 {
				op := ""
				if tk.Minor {
					op = "0.6"
				}
				c.El("line", "x1", core.F(px), "y1", core.F(p.r.y0), "x2", core.F(px), "y2", core.F(p.r.y1),
					"stroke", t.Faint, "stroke-width", "1", "stroke-opacity", op)
			}
			if !tk.Minor || tk.Label != "" {
				c.Line(px, p.r.y1, px, p.r.y1+4, t.Edge, 1, "")
			}
		}
		for _, tk := range p.visibleTicks(p.x, true) {
			c.Text(p.x.px(tk.V), p.r.y1+14, tk.Label, core.TextStyle{Size: tickSize, Fill: t.Muted, Anchor: core.Middle})
		}
	} else {
		for i, lines := range p.catLines {
			cx, _ := p.x.band(i)
			h := core.BlockHeight(len(lines), tickSize, 0)
			c.Lines(cx, p.r.y1+7+h/2, lines, core.TextStyle{Size: tickSize, Fill: t.Ink, Anchor: core.Middle}, 0)
		}
	}
	c.Line(p.r.x0, p.r.y1, p.r.x1, p.r.y1, t.Edge, 1, "")
	if p.x.title != "" && !p.origin {
		ty := p.bottom - 2 - 9
		c.Text((p.r.x0+p.r.x1)/2, ty, core.Ellipsize(p.x.title, labelSize, core.Width-2*padX, false), core.TextStyle{Size: labelSize, Fill: t.Muted, Anchor: core.Middle})
	}
}

// visibleTicks are the labelled ticks that can be shown without their
// labels touching: every one if they fit, else every second, third and so
// on, always keeping the first.
func (p *plot) visibleTicks(ax *axis, horizontal bool) []Tick {
	var labelled []Tick
	for _, t := range ax.ticks {
		if t.Label != "" {
			labelled = append(labelled, t)
		}
	}
	for k := 1; k <= len(labelled); k++ {
		var keep []Tick
		for i := 0; i < len(labelled); i += k {
			keep = append(keep, labelled[i])
		}
		ok := true
		for i := 1; i < len(keep); i++ {
			a, b := ax.px(keep[i-1].V), ax.px(keep[i].V)
			if horizontal {
				need := (core.TextWidth(keep[i-1].Label, tickSize, false)+core.TextWidth(keep[i].Label, tickSize, false))/2 + 8
				if math.Abs(b-a) < need {
					ok = false
					break
				}
			} else if math.Abs(b-a) < tickSize+3 {
				ok = false
				break
			}
		}
		if ok {
			return keep
		}
	}
	return labelled[:min(1, len(labelled))]
}

// drawOriginAxes draws a function plot's axes: through zero when zero is in
// view, else along the plot's edge, with ticks labelled beside them.
func (p *plot) drawOriginAxes() {
	c := p.c
	t := c.T
	for _, tk := range p.y.ticks {
		py := p.y.px(tk.V)
		c.Line(p.r.x0, py, p.r.x1, py, t.Faint, 1, "")
	}
	for _, tk := range p.x.ticks {
		px := p.x.px(tk.V)
		c.Line(px, p.r.y0, px, p.r.y1, t.Faint, 1, "")
	}
	// Where the axes run.
	ax := p.r.y1 // the x axis's y
	if p.y.lo < 0 && p.y.hi > 0 {
		ax = p.y.px(0)
	} else if p.y.hi <= 0 {
		ax = p.r.y0
	}
	ay := p.r.x0 // the y axis's x
	if p.x.lo < 0 && p.x.hi > 0 {
		ay = p.x.px(0)
	} else if p.x.hi <= 0 {
		ay = p.r.x1
	}
	c.Line(p.r.x0, ax, p.r.x1, ax, t.Ink, 1.2, "")
	c.Line(ay, p.r.y0, ay, p.r.y1, t.Ink, 1.2, "")
	// Axis names at the positive ends, as a textbook writes x and y; the
	// tick labels after them give way where they would meet.
	if p.x.title != "" {
		st := core.TextStyle{Size: labelSize, Fill: t.Ink, Anchor: core.End, Halo: t.Bg, Italic: p.x.title == "x"}
		y := ax - 10
		if y-8 < p.r.y0 {
			y = ax + 26
		}
		c.Text(p.r.x1-2, y, p.x.title, st)
		tr := textRect(p.r.x1-2, y, p.x.title, labelSize, false, core.End)
		p.obs.rect(tr)
		p.titles = append(p.titles, tr)
	}
	if p.y.title != "" {
		st := core.TextStyle{Size: labelSize, Fill: t.Ink, Halo: t.Bg, Italic: p.y.title == "y"}
		x := ay + 7
		if x+core.TextWidth(p.y.title, labelSize, false) > p.r.x1 {
			st.Anchor = core.End
			x = ay - 7
		}
		c.Text(x, p.r.y0+7, p.y.title, st)
		tr := textRect(x, p.r.y0+7, p.y.title, labelSize, false, st.Anchor)
		p.obs.rect(tr)
		p.titles = append(p.titles, tr)
	}

	halo := core.TextStyle{Size: tickSize, Fill: t.Muted, Halo: t.Bg}
	// x tick labels under the x axis, skipping zero where the axes cross
	// and any label that would cover a marked point or an axis name.
	below := ax+15 <= p.r.y1+16
	for _, tk := range p.visibleTicks(p.x, true) {
		px := p.x.px(tk.V)
		if tk.V == 0 && ay == px {
			continue
		}
		c.Line(px, ax-3, px, ax+3, t.Ink, 1, "")
		st := halo
		st.Anchor = core.Middle
		y := ax + 13
		if !below {
			y = ax - 11
		}
		if p.covers(textRect(px, y, tk.Label, tickSize, false, core.Middle)) {
			continue
		}
		c.Text(px, y, tk.Label, st)
		p.obs.rect(textRect(px, y, tk.Label, tickSize, false, core.Middle))
	}
	for _, tk := range p.visibleTicks(p.y, false) {
		py := p.y.px(tk.V)
		if tk.V == 0 && ax == py {
			continue
		}
		c.Line(ay-3, py, ay+3, py, t.Ink, 1, "")
		st := halo
		st.Anchor = core.End
		x := ay - 6
		if x-core.TextWidth(tk.Label, tickSize, false) < 2 {
			st.Anchor = core.Start
			x = ay + 6
		}
		if p.covers(textRect(x, py, tk.Label, tickSize, false, st.Anchor)) {
			continue
		}
		c.Text(x, py, tk.Label, st)
		p.obs.rect(textRect(x, py, tk.Label, tickSize, false, st.Anchor))
	}
	if ay == p.x.px(0) && ax == p.y.px(0) {
		st := halo
		st.Anchor = core.End
		c.Text(ay-5, ax+12, "0", st)
	}
}

// covers says whether r would cover a point in p.avoid.
func (p *plot) covers(r rect) bool {
	for _, q := range p.avoid {
		if r.grow(4).contains(q) {
			return true
		}
	}
	for _, q := range p.titles {
		if r.grow(2).overlaps(q) {
			return true
		}
	}
	return false
}

// drawMarker draws one of six point shapes, so series stay apart without
// colour.
func drawMarker(c *core.Canvas, shape int, x, y, r float64, colour string) {
	ring := c.T.Bg
	switch shape % 6 {
	case 0:
		c.Circle(x, y, r, colour, ring, 1)
	case 1:
		s := r * 0.9
		c.Rect(x-s, y-s, 2*s, 2*s, 0, colour, ring, 1)
	case 2:
		s := r * 1.25
		c.Polygon([]core.Pt{{X: x, Y: y - s}, {X: x + s*0.95, Y: y + s*0.7}, {X: x - s*0.95, Y: y + s*0.7}}, colour, ring, 1)
	case 3:
		s := r * 1.25
		c.Polygon([]core.Pt{{X: x, Y: y - s}, {X: x + s, Y: y}, {X: x, Y: y + s}, {X: x - s, Y: y}}, colour, ring, 1)
	case 4:
		s := r * 1.25
		c.Polygon([]core.Pt{{X: x, Y: y + s}, {X: x + s*0.95, Y: y - s*0.7}, {X: x - s*0.95, Y: y - s*0.7}}, colour, ring, 1)
	case 5:
		c.Circle(x, y, r, c.T.Bg, colour, 1.8)
	}
}

// seriesDash is the i-th of n series' dash: solid for one or two, a
// second cue past that, so series stay apart for any reader.
func seriesDash(i, n int) string {
	if n <= 2 {
		return ""
	}
	return []string{"", "7 4", "1.5 3.5", "10 3 2 3", "4 3", "1.5 6", "12 4", "6 2 1.5 2"}[i%8]
}
