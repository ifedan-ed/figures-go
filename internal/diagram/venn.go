package diagram

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var vennKind = core.Kind{
	Name:   "venn",
	Family: "diagram",
	Use:    "What two or three conditions or sets share and what is unique to each (features of Kawasaki disease, MIS-C and toxic shock); for many features across several options use comparison_table.",
	Fields: "sets: [{id, label (" + core.UpTo(vnSetChars) + "), tone?}] (2 or 3). regions: [{sets: [the ids the region is inside, e.g. [\"a\"] or [\"a\",\"b\"]], items?: [short texts, each " + core.UpTo(vnItemChars) + "] (at most 5), count?: a number}] (1 to 7), each combination at most once; give items or a count. Keep items to a few words: the overlap regions are small.",
	Example: `{"type":"venn","title":"Kawasaki disease, MIS-C and toxic shock","alt":"All three cause fever and rash. Kawasaki disease and MIS-C share conjunctivitis and mucositis; MIS-C and toxic shock share shock and GI symptoms. Coronary aneurysms are typical of Kawasaki disease, recent SARS-CoV-2 of MIS-C, and a toxin-producing infection of toxic shock.","spec":{"sets":[{"id":"kd","label":"Kawasaki disease"},{"id":"mis","label":"MIS-C"},{"id":"tss","label":"Toxic shock"}],"regions":[` +
		`{"sets":["kd"],"items":["Coronary aneurysms","Under 5 years"]},{"sets":["mis"],"items":["Recent SARS-CoV-2","Older children"]},` +
		`{"sets":["tss"],"items":["Staph or strep toxin"]},{"sets":["kd","mis"],"items":["Conjunctivitis","Mucositis"]},` +
		`{"sets":["mis","tss"],"items":["Shock","GI upset"]},{"sets":["kd","tss"],"items":["Peeling"]},{"sets":["kd","mis","tss"],"items":["Fever","Rash"]}]}}`,
	Parse: parseVenn,
}

type vnSet struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

type vnRegion struct {
	Sets  []string `json:"sets"`
	Items []string `json:"items"`
	Count *float64 `json:"count"`
}

type vnSpec struct {
	Sets    []vnSet    `json:"sets"`
	Regions []vnRegion `json:"regions"`
}

type venn struct {
	sets    []vnSet
	centres []core.Pt
	r       float64
	labels  []textBlock
	labelAt []core.Pt
	texts   []vnText
	H       float64
}

// vnText is a region's content placed in it.
type vnText struct {
	lines []textBlock
	count string
	at    box
}

const (
	vnItemSize = 11.5
	// vnGap parts one item from the next, more than the lines of one.
	vnGap = 4.0

	vnSetChars  = 30
	vnItemChars = 28
)

func parseVenn(raw json.RawMessage) (core.Drawing, error) {
	var s vnSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("sets", len(s.Sets), 2, 3)
	p.Count("regions", len(s.Regions), 1, 7)
	ids := make([]string, len(s.Sets))
	for i := range s.Sets {
		ids[i] = s.Sets[i].ID
		s.Sets[i].Label = p.Label(core.Path("sets", i, "label"), s.Sets[i].Label, true, vnSetChars)
		p.Tone(core.Path("sets", i, "tone"), s.Sets[i].Tone)
	}
	p.Unique("sets", ids)
	index := map[string]int{}
	for i, id := range ids {
		index[id] = i
	}
	masks := make([]int, len(s.Regions))
	seen := map[int]int{}
	for i, r := range s.Regions {
		at := core.Path("regions", i)
		if len(r.Sets) == 0 {
			p.Add(core.Path(at, "sets"), "is required: the ids of the sets this region is inside, from %s", idList(ids))
		}
		m := 0
		for j, id := range r.Sets {
			k, ok := index[id]
			if !ok {
				p.Add(core.Path(at, "sets", j), "%q is not a set id; ids are %s", id, idList(ids))
				continue
			}
			if m&(1<<k) != 0 {
				p.Add(core.Path(at, "sets", j), "%q is listed twice", id)
			}
			m |= 1 << k
		}
		masks[i] = m
		if prev, dup := seen[m]; dup && m != 0 {
			p.Add(core.Path(at, "sets"), "the same region as regions[%d]; put all its items in one region", prev)
		}
		seen[m] = i
		p.Count(core.Path(at, "items"), len(r.Items), 0, 5)
		for j := range r.Items {
			s.Regions[i].Items[j] = p.Label(core.Path(at, "items", j), r.Items[j], true, vnItemChars)
		}
		switch {
		case r.Count != nil && len(r.Items) > 0:
			p.Add(at, "has both items and a count; give one")
		case r.Count == nil && len(r.Items) == 0:
			p.Add(at, "is empty; give items or a count, or leave the region out")
		case r.Count != nil:
			p.Finite(core.Path(at, "count"), *r.Count)
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	v := &venn{sets: s.Sets}
	v.geometry()
	for i, r := range s.Regions {
		t, ok := v.place(masks[i], r)
		if !ok {
			var names []string
			for _, id := range r.Sets {
				names = append(names, id)
			}
			p.Add(core.Path("regions", i), "its items do not fit in the region inside %s only; use fewer or shorter items (a word or two each), or give a count", strings.Join(names, " and "))
			continue
		}
		v.texts = append(v.texts, t)
	}
	if p.Any() {
		return nil, p.Err()
	}
	return v, nil
}

func (v *venn) geometry() {
	n := len(v.sets)
	top := 26.0
	if n == 2 {
		v.r = 112
		d := 68.0
		cy := top + v.r
		v.centres = []core.Pt{{X: core.Width/2 - d, Y: cy}, {X: core.Width/2 + d, Y: cy}}
		v.H = cy + v.r + 6
	} else {
		v.r = 114
		d := 66.0
		cy := top + v.r
		v.centres = []core.Pt{{X: core.Width/2 - d, Y: cy}, {X: core.Width/2 + d, Y: cy}, {X: core.Width / 2, Y: cy + d*math.Sqrt(3)}}
		v.H = cy + d*math.Sqrt(3) + v.r + 28
	}
	for i, s := range v.sets {
		tb := wrapBlock(s.Label, 13, 170, true)
		tb.lines = tb.lines[:1]
		tb.lines[0] = core.Ellipsize(tb.lines[0], 13, 186, true)
		v.labels = append(v.labels, tb)
		c := v.centres[i]
		switch {
		case i == 2:
			v.labelAt = append(v.labelAt, core.Pt{X: c.X, Y: c.Y + v.r + 14})
		default:
			v.labelAt = append(v.labelAt, core.Pt{X: c.X, Y: 11})
		}
	}
}

// in is the set of circles p lies inside, -1 when p is too near an edge
// for text.
func (v *venn) in(p core.Pt, clear float64) int {
	m := 0
	for i, c := range v.centres {
		d := dist(p, c)
		if math.Abs(d-v.r) < clear {
			return -1
		}
		if d < v.r {
			m |= 1 << i
		}
	}
	return m
}

// place finds where a region's text fits, as wide as it can be, nearest
// the middle of the region.
func (v *venn) place(mask int, r vnRegion) (vnText, bool) {
	// The region's middle, by sampling.
	sx, sy, n := 0.0, 0.0, 0.0
	for x := 0.0; x <= core.Width; x += 3 {
		for y := 0.0; y <= v.H; y += 3 {
			if v.in(core.Pt{X: x, Y: y}, 0) == mask {
				sx, sy, n = sx+x, sy+y, n+1
			}
		}
	}
	if n == 0 {
		return vnText{}, false
	}
	mid := core.Pt{X: sx / n, Y: sy / n}
	for _, size := range []float64{vnItemSize, labelSize} {
		for w := 120.0; w >= 40; w -= 8 {
			t := vnText{}
			tw, th := 0.0, 0.0
			if r.Count != nil {
				t.count = strconv.FormatFloat(*r.Count, 'f', -1, 64)
				tw, th = core.TextWidth(t.count, 18, true), 22
			} else {
				for _, it := range r.Items {
					b := wrapBlock(it, size, w, false)
					t.lines = append(t.lines, b)
					tw, th = max(tw, b.w), th+b.h+vnGap
				}
				th -= vnGap
			}
			bw, bh := tw+4, th+2
			type cand struct {
				p core.Pt
				d float64
			}
			var cs []cand
			for x := bw/2 + 2; x <= core.Width-bw/2-2; x += 3 {
				for y := bh/2 + 2; y <= v.H-bh/2; y += 3 {
					if v.in(core.Pt{X: x, Y: y}, 3) != mask {
						continue
					}
					cs = append(cs, cand{core.Pt{X: x, Y: y}, dist(core.Pt{X: x, Y: y}, mid)})
				}
			}
			sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
			for _, c := range cs {
				b := box{c.p.X - bw/2, c.p.Y - bh/2, bw, bh}
				if v.boxIn(b, mask) {
					t.at = b
					return t, true
				}
			}
		}
	}
	return vnText{}, false
}

// boxIn says whether b lies wholly in the region mask, clear of its edges.
func (v *venn) boxIn(b box, mask int) bool {
	const step = 4.0
	for x := b.X; ; x += step {
		x = math.Min(x, b.R())
		for y := b.Y; ; y += step {
			y = math.Min(y, b.B())
			if v.in(core.Pt{X: x, Y: y}, 3) != mask {
				return false
			}
			if y >= b.B() {
				break
			}
		}
		if x >= b.R() {
			break
		}
	}
	// Clear of the set labels too.
	for i, lb := range v.labels {
		a := v.labelAt[i]
		if b.overlaps(box{a.X - lb.w/2, a.Y - 9, lb.w, 18}, 2) {
			return false
		}
	}
	return true
}

func (v *venn) colour(t *core.Theme, i int) string {
	if tone := v.sets[i].Tone; tone != "" {
		s, _ := t.Tone(tone)
		return s
	}
	return t.SeriesColour(i)
}

func (v *venn) Draw(c *core.Canvas) error {
	t := c.T
	op := "0.13"
	if t.Name == "dark" {
		op = "0.2"
	}
	for i, ct := range v.centres {
		col := v.colour(t, i)
		c.El("circle", "cx", core.F(ct.X), "cy", core.F(ct.Y), "r", core.F(v.r), "fill", col, "fill-opacity", op)
	}
	for i, ct := range v.centres {
		c.Circle(ct.X, ct.Y, v.r, "", v.colour(t, i), 1.8)
	}
	for i, lb := range v.labels {
		a := v.labelAt[i]
		lb.draw(c, a.X, a.Y, v.colour(t, i), core.Middle, false)
	}
	for _, tx := range v.texts {
		cx := tx.at.CX()
		if tx.count != "" {
			c.Text(cx, tx.at.CY(), tx.count, core.TextStyle{Size: 18, Bold: true, Fill: t.Ink, Anchor: core.Middle})
			continue
		}
		y := tx.at.Y + 1
		for _, l := range tx.lines {
			l.draw(c, cx, y+l.h/2, t.Ink, core.Middle, false)
			y += l.h + vnGap
		}
	}
	c.H = v.H
	return nil
}
