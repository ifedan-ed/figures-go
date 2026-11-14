package chart

import (
	"math"
	"sort"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// rect is an area in figure units, x0 < x1 and y0 < y1.
type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) w() float64 { return r.x1 - r.x0 }
func (r rect) h() float64 { return r.y1 - r.y0 }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

func (r rect) contains(p core.Pt) bool {
	return p.X >= r.x0 && p.X <= r.x1 && p.Y >= r.y0 && p.Y <= r.y1
}

func (r rect) inside(o rect) bool {
	return r.x0 >= o.x0-0.01 && r.x1 <= o.x1+0.01 && r.y0 >= o.y0-0.01 && r.y1 <= o.y1+0.01
}

func (r rect) grow(d float64) rect { return rect{r.x0 - d, r.y0 - d, r.x1 + d, r.y1 + d} }

// textRect is the box a one-line text at size takes, its middle at y.
func textRect(x, y float64, s string, size float64, bold bool, anchor core.Anchor) rect {
	w := core.TextWidth(s, size, bold)
	switch anchor {
	case core.Middle:
		x -= w / 2
	case core.End:
		x -= w
	}
	return rect{x, y - size*0.62, x + w, y + size*0.62}
}

// clipSegment is the part of a→b inside r (Liang–Barsky), and whether
// there is any.
func clipSegment(a, b core.Pt, r rect) (core.Pt, core.Pt, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := b.X-a.X, b.Y-a.Y
	for _, e := range [4][2]float64{{-dx, a.X - r.x0}, {dx, r.x1 - a.X}, {-dy, a.Y - r.y0}, {dy, r.y1 - a.Y}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return a, b, false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return a, b, false
			}
			t0 = math.Max(t0, t)
		} else {
			if t < t0 {
				return a, b, false
			}
			t1 = math.Min(t1, t)
		}
	}
	return core.Pt{X: a.X + t0*dx, Y: a.Y + t0*dy}, core.Pt{X: a.X + t1*dx, Y: a.Y + t1*dy}, true
}

// clipPolyline cuts a line to r: the runs of it inside, each drawn on its
// own. A point that is not finite breaks the line too.
func clipPolyline(pts []core.Pt, r rect) [][]core.Pt {
	var out [][]core.Pt
	var run []core.Pt
	flush := func() {
		if len(run) >= 2 {
			out = append(out, run)
		}
		run = nil
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if !finitePt(a) || !finitePt(b) {
			flush()
			continue
		}
		ca, cb, ok := clipSegment(a, b, r)
		if !ok {
			flush()
			continue
		}
		if len(run) == 0 || !samePt(run[len(run)-1], ca) {
			flush()
			run = append(run, ca)
		}
		run = append(run, cb)
		if !samePt(cb, b) {
			flush()
		}
	}
	flush()
	if len(pts) == 1 && finitePt(pts[0]) && r.contains(pts[0]) {
		return nil
	}
	return out
}

func finitePt(p core.Pt) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}

func samePt(a, b core.Pt) bool { return math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) < 1e-6 }

// clipPolygon cuts a closed shape to r (Sutherland–Hodgman), for a fill
// that runs past the plot.
func clipPolygon(pts []core.Pt, r rect) []core.Pt {
	type edge struct {
		in  func(core.Pt) bool
		cut func(a, b core.Pt) core.Pt
	}
	lerpX := func(a, b core.Pt, x float64) core.Pt {
		return core.Pt{X: x, Y: a.Y + (b.Y-a.Y)*(x-a.X)/(b.X-a.X)}
	}
	lerpY := func(a, b core.Pt, y float64) core.Pt {
		return core.Pt{X: a.X + (b.X-a.X)*(y-a.Y)/(b.Y-a.Y), Y: y}
	}
	edges := []edge{
		{func(p core.Pt) bool { return p.X >= r.x0 }, func(a, b core.Pt) core.Pt { return lerpX(a, b, r.x0) }},
		{func(p core.Pt) bool { return p.X <= r.x1 }, func(a, b core.Pt) core.Pt { return lerpX(a, b, r.x1) }},
		{func(p core.Pt) bool { return p.Y >= r.y0 }, func(a, b core.Pt) core.Pt { return lerpY(a, b, r.y0) }},
		{func(p core.Pt) bool { return p.Y <= r.y1 }, func(a, b core.Pt) core.Pt { return lerpY(a, b, r.y1) }},
	}
	out := pts
	for _, e := range edges {
		in := out
		out = nil
		for i := range in {
			cur, prev := in[i], in[(i+len(in)-1)%len(in)]
			switch {
			case e.in(cur) && e.in(prev):
				out = append(out, cur)
			case e.in(cur):
				out = append(out, e.cut(prev, cur), cur)
			case e.in(prev):
				out = append(out, e.cut(prev, cur))
			}
		}
		if len(out) == 0 {
			return nil
		}
	}
	return out
}

// segHitsRect says whether a→b crosses or lies in r.
func segHitsRect(a, b core.Pt, r rect) bool {
	_, _, ok := clipSegment(a, b, r)
	return ok
}

// obstacles are what a label must not cover: the data's lines, points and
// bars, and the labels already placed.
type obstacles struct {
	segs  [][2]core.Pt
	rects []rect
	pts   []core.Pt
}

func (o *obstacles) line(pts []core.Pt) {
	for i := 1; i < len(pts); i++ {
		o.segs = append(o.segs, [2]core.Pt{pts[i-1], pts[i]})
	}
}

func (o *obstacles) rect(r rect)     { o.rects = append(o.rects, r) }
func (o *obstacles) point(p core.Pt) { o.pts = append(o.pts, p) }

// cost is how much of what is drawn r would cover: each line crossed,
// each point and box covered.
func (o *obstacles) cost(r rect) float64 {
	c := 0.0
	for _, s := range o.segs {
		if segHitsRect(s[0], s[1], r) {
			c++
		}
	}
	for _, p := range o.pts {
		if r.grow(3).contains(p) {
			c += 2
		}
	}
	// Labels must never cover each other: a box costs far more than a
	// line crossed.
	for _, q := range o.rects {
		if r.overlaps(q) {
			c += 25
		}
	}
	return c
}

// spread moves labels' centres (want, in any order) apart until each is at
// least gap from the next, as little as it can, keeping them within lo..hi.
// It is the usual way end-of-line labels are set: clusters of labels that
// would touch are centred on their wanted positions together.
func spread(want []float64, gap, lo, hi float64) []float64 {
	n := len(want)
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return want[idx[a]] < want[idx[b]] })
	type cluster struct {
		first, count int // in idx order
		sum          float64
	}
	place := func(c cluster) float64 {
		// The top of the cluster, centred on its members' mean.
		top := c.sum/float64(c.count) - gap*float64(c.count-1)/2
		top = math.Max(lo, math.Min(top, hi-gap*float64(c.count-1)))
		return top
	}
	var cs []cluster
	for k, i := range idx {
		cs = append(cs, cluster{first: k, count: 1, sum: want[i]})
		for len(cs) > 1 {
			a, b := cs[len(cs)-2], cs[len(cs)-1]
			if place(a)+gap*float64(a.count) <= place(b)+1e-9 {
				break
			}
			cs = cs[:len(cs)-2]
			cs = append(cs, cluster{first: a.first, count: a.count + b.count, sum: a.sum + b.sum})
		}
	}
	for _, c := range cs {
		top := place(c)
		for j := 0; j < c.count; j++ {
			out[idx[c.first+j]] = top + gap*float64(j)
		}
	}
	return out
}
