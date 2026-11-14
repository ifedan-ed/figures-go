package diagram

import (
	"encoding/json"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var conceptKind = core.Kind{
	Name:   "concept_map",
	Family: "diagram",
	Use:    "A central concept with labelled links to related concepts, one or two levels deep (a disease's causes, features, tests and treatment); for a sequence of steps use flowchart, for a cascade use pathway.",
	Fields: "centre: the central concept (" + core.UpTo(cmCentreChars) + "). branches: [{text (" + core.UpTo(cmTextChars) + "), link? (the link's label read from the centre, e.g. \"causes\", " + core.UpTo(cmLinkChars) + "), tone?, children?: [{text (" + core.UpTo(cmTextChars) + "), link? (" + core.UpTo(cmLinkChars) + ")}] (at most 4)}] (1 to 8; at most 24 concepts in all). Branches are drawn half above and half below the centre, each with its children stacked outward.",
	Example: `{"type":"concept_map","title":"Iron deficiency anaemia in toddlers","alt":"Concept map of iron deficiency anaemia: caused by excess cow's milk and poor intake, presenting with pallor and fatigue, diagnosed with a low ferritin and microcytosis, treated with oral iron.","spec":{"centre":"Iron deficiency anaemia","branches":[` +
		`{"text":"Causes","link":"caused by","children":[{"text":"Over 500 mL cow's milk a day"},{"text":"Low dietary iron"},{"text":"Blood loss"}]},` +
		`{"text":"Features","link":"presents with","children":[{"text":"Pallor, fatigue"},{"text":"Pica"},{"text":"Poor attention"}]},` +
		`{"text":"Tests","link":"shown by","tone":"accent","children":[{"text":"Low ferritin"},{"text":"Microcytic, hypochromic"}]},` +
		`{"text":"Treatment","link":"treated with","tone":"good","children":[{"text":"Oral iron 3 mg/kg/day"},{"text":"Limit milk","link":"and"}]}]}}`,
	Parse: parseConcept,
}

type cmChild struct {
	Text string `json:"text"`
	Link string `json:"link"`
}

type cmBranch struct {
	Text     string    `json:"text"`
	Link     string    `json:"link"`
	Tone     string    `json:"tone"`
	Children []cmChild `json:"children"`
}

type cmSpec struct {
	Centre   string     `json:"centre"`
	Branches []cmBranch `json:"branches"`
}

// cmNode is a placed concept.
type cmNode struct {
	text textBlock
	b    box
	link textBlock // its link's label, for a child drawn above it
}

type concept struct {
	branches []cmBranch
	centre   cmNode
	nodes    []cmNode   // the branches
	kids     [][]cmNode // each branch's children
	up       []bool     // whether a branch is above the centre
	spine    []float64  // the x of each branch's spine
	labels   []label    // the links' labels from the centre
	H        float64
}

const (
	cmGapX    = 12.0
	cmIndent  = 13.0
	cmKidPad  = 10.0 // a child's text, from side to side of its box
	cmKidGap  = 8.0
	cmRowGap  = 64.0 // between the centre and a row of branches
	cmMaxColW = 210.0

	// cmTextChars is the longest a branch's or child's text may be: 50
	// characters until 2026-11-10, when the model wrote children of 54
	// to 70 (core.CharsPerWord). They wrap, and a map too tall is refused.
	cmTextChars   = 70
	cmCentreChars = 50
	cmLinkChars   = 24
)

func parseConcept(raw json.RawMessage) (core.Drawing, error) {
	var s cmSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	centre := p.Label("centre", s.Centre, true, cmCentreChars)
	p.Count("branches", len(s.Branches), 1, 8)
	total := 1
	for i := range s.Branches {
		b := &s.Branches[i]
		at := core.Path("branches", i)
		b.Text = p.Label(core.Path(at, "text"), b.Text, true, cmTextChars)
		b.Link = p.Label(core.Path(at, "link"), b.Link, false, cmLinkChars)
		p.Tone(core.Path(at, "tone"), b.Tone)
		p.Count(core.Path(at, "children"), len(b.Children), 0, 4)
		total += 1 + len(b.Children)
		for j := range b.Children {
			k := &b.Children[j]
			k.Text = p.Label(core.Path(at, "children", j, "text"), k.Text, true, cmTextChars)
			k.Link = p.Label(core.Path(at, "children", j, "link"), k.Link, false, cmLinkChars)
		}
	}
	if total > 25 {
		p.Add("branches", "the map has %d concepts; at most 25 can be read on a phone, so keep the main branches and fewer children", total)
	}
	if p.Any() {
		return nil, p.Err()
	}
	m := &concept{branches: s.Branches}
	m.layout(centre)
	if m.H > maxBody {
		tooTall(&p, "concept map", m.H, "give fewer children, or shorter texts")
		return nil, p.Err()
	}
	return m, nil
}

func (m *concept) layout(centre string) {
	n := len(m.branches)
	nTop := (n + 1) / 2
	rows := [][]int{}
	var top, bottom []int
	for i := 0; i < n; i++ {
		if i < nTop {
			top = append(top, i)
		} else {
			bottom = append(bottom, i)
		}
	}
	rows = append(rows, top)
	if len(bottom) > 0 {
		rows = append(rows, bottom)
	}
	m.nodes = make([]cmNode, n)
	m.kids = make([][]cmNode, n)
	m.up = make([]bool, n)
	m.spine = make([]float64, n)
	// Size each row's columns.
	type col struct{ x, w float64 }
	cols := map[int]col{}
	rowH := map[int]float64{}   // the branches' common height in a row
	stackH := map[int]float64{} // the tallest stack of children in a row
	for r, row := range rows {
		k := float64(len(row))
		gap := cmGapX
		if k >= 4 {
			gap = 7
		}
		cw := math.Min(cmMaxColW, (avail-(k-1)*gap)/k)
		x0 := core.Width/2 - (k*cw+(k-1)*gap)/2
		for j, i := range row {
			cols[i] = col{x0 + float64(j)*(cw+gap), cw}
			b := m.branches[i]
			tb := wrapBlock(b.Text, fitSize(b.Text, 12.5, cw-16, true), cw-16, true)
			m.nodes[i].text = tb
			rowH[r] = max(rowH[r], tb.h+16)
			sh := 0.0
			for _, kd := range b.Children {
				kw := cw - cmIndent
				node := cmNode{text: wrapBlock(kd.Text, fitSize(kd.Text, 12, kw-cmKidPad, false), kw-cmKidPad, false)}
				if kd.Link != "" {
					node.link = wrapBlock(kd.Link, labelSize, kw, false)
				}
				h := node.text.h + 12
				node.b = box{0, 0, kw, h}
				m.kids[i] = append(m.kids[i], node)
				sh += h + cmKidGap + node.link.h
			}
			stackH[r] = max(stackH[r], sh)
		}
	}
	m.centre.text = wrapBlock(centre, 14, 200, true)
	cw := m.centre.text.w + 36
	ch := m.centre.text.h + 20
	// Top row: children above the branches; then the centre; then the
	// bottom row with children below.
	y := 2.0
	topBranchY := y + stackH[0]
	centreY := topBranchY + rowH[0] + cmRowGap
	m.centre.b = box{core.Width/2 - cw/2, centreY, cw, ch}
	y = centreY + ch
	bottomY := y + cmRowGap
	for r, row := range rows {
		for _, i := range row {
			c := cols[i]
			by := topBranchY
			if r == 1 {
				by = bottomY
			}
			m.up[i] = r == 0
			m.nodes[i].b = box{c.x, by, c.w, rowH[r]}
			m.spine[i] = c.x + 6
			if r == 0 {
				// Children read top down, ending just above the branch.
				yy := by
				for k := len(m.kids[i]) - 1; k >= 0; k-- {
					kd := &m.kids[i][k]
					yy -= cmKidGap + kd.b.H
					kd.b.X, kd.b.Y = c.x+cmIndent, yy
					yy -= kd.link.h
				}
			} else {
				yy := by + rowH[r] + cmKidGap
				for k := range m.kids[i] {
					kd := &m.kids[i][k]
					yy += kd.link.h
					kd.b.X, kd.b.Y = c.x+cmIndent, yy
					yy += kd.b.H + cmKidGap
				}
			}
		}
	}
	m.H = centreY + ch + 2
	if len(rows) > 1 {
		m.H = bottomY + rowH[1] + stackH[1] + 4
	}
	// Labels for the links from the centre, along each line where they
	// clear the boxes and each other.
	var placed []box
	for i, b := range m.branches {
		if b.Link == "" {
			continue
		}
		a, z := m.linkEnds(i)
		tb := wrapBlock(b.Link, labelSize, 90, false)
		var best box
		for _, t := range []float64{0.5, 0.62, 0.38, 0.72, 0.28} {
			p := core.Pt{X: a.X + (z.X-a.X)*t, Y: a.Y + (z.Y-a.Y)*t}
			best = box{p.X - tb.w/2 - 2, p.Y - tb.h/2, tb.w + 4, tb.h}
			ok := !best.overlaps(m.centre.b, 2)
			for _, nd := range m.nodes {
				ok = ok && !best.overlaps(nd.b, 2)
			}
			for _, o := range placed {
				ok = ok && !best.overlaps(o, 2)
			}
			if ok {
				break
			}
		}
		placed = append(placed, best)
		m.labels = append(m.labels, label{text: tb, at: best, italic: true})
	}
}

// linkEnds is where the line from the centre to branch i starts and ends.
func (m *concept) linkEnds(i int) (core.Pt, core.Pt) {
	c, b := m.centre.b, m.nodes[i].b
	if m.up[i] {
		return core.Pt{X: c.CX() + (b.CX()-c.CX())*0.25, Y: c.Y}, core.Pt{X: b.CX(), Y: b.B()}
	}
	return core.Pt{X: c.CX() + (b.CX()-c.CX())*0.25, Y: c.B()}, core.Pt{X: b.CX(), Y: b.Y}
}

func (m *concept) Draw(c *core.Canvas) error {
	t := c.T
	for i := range m.branches {
		a, z := m.linkEnds(i)
		stroke, _ := toneOf(t, m.branches[i].Tone, t.Muted, "")
		c.Arrow(a, z, stroke, 1.5, "")
		// The spine to the children, and a tick into each.
		if len(m.kids[i]) == 0 {
			continue
		}
		b := m.nodes[i].b
		x := m.spine[i]
		last := m.kids[i][len(m.kids[i])-1].b
		y0, y1 := b.B(), last.CY()
		if m.up[i] {
			y0, y1 = b.Y, m.kids[i][0].b.CY()
		}
		c.Line(x, y0, x, y1, t.Edge, 1.3, "")
		for _, kd := range m.kids[i] {
			c.Arrow(core.Pt{X: x, Y: kd.b.CY()}, core.Pt{X: kd.b.X, Y: kd.b.CY()}, t.Edge, 1.3, "")
		}
	}
	// The centre.
	cb := m.centre.b
	c.Rect(cb.X, cb.Y, cb.W, cb.H, cb.H/2, t.AccentSoft, t.Accent, 1.8)
	m.centre.text.draw(c, cb.CX(), cb.CY(), t.Ink, core.Middle, false)
	for i, br := range m.branches {
		nd := m.nodes[i]
		stroke, fill := toneOf(t, br.Tone, t.Series[0], t.SeriesSoft[0])
		c.Rect(nd.b.X, nd.b.Y, nd.b.W, nd.b.H, 8, fill, stroke, 1.4)
		nd.text.draw(c, nd.b.CX(), nd.b.CY(), t.Ink, core.Middle, false)
		for _, kd := range m.kids[i] {
			c.Rect(kd.b.X, kd.b.Y, kd.b.W, kd.b.H, 6, t.Surface, t.Edge, 1)
			kd.text.draw(c, kd.b.X+cmKidPad/2, kd.b.CY(), t.Ink, core.Start, false)
			if len(kd.link.lines) > 0 {
				kd.link.draw(c, kd.b.X+2, kd.b.Y-kd.link.h/2-1, t.Muted, core.Start, true)
			}
		}
	}
	for _, l := range m.labels {
		l.draw(c)
	}
	c.H = m.H
	return nil
}
