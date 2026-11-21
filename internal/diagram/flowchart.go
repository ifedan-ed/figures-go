package diagram

import (
	"encoding/json"
	"fmt"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var flowchartKind = core.Kind{
	Name:   "flowchart",
	Family: "diagram",
	Use:    "A management algorithm or process drawn top down like a clinical guideline: steps, and decisions that branch; for a cascade of activations and inhibitions use pathway, for events in time use timeline, for a sequence or state diagram use mermaid.",
	Fields: "nodes: [{id, text (" + core.UpTo(maxNodeText) + "), shape: start|step|decision|end|note (default step), tone?, next?}] (2 to 30), in reading order, the first where the flow starts. next is the id that follows a start or step; an end has nothing after it; a note is a remark drawn beside the node its next names. edges: [{from, to, label? (" + core.UpTo(maxEdgeLabel) + ")}] are the other links: each branch of a decision is an edge from it, labelled with the answer it stands for. A link back to an earlier node (reassess, repeat) is drawn as a loop round the side.",
	Example: `{"type":"flowchart","title":"Anaphylaxis: first steps","alt":"Algorithm: give IM adrenaline, then call for help, lie flat, oxygen and fluids; reassess after 5 minutes and repeat adrenaline until improving, then observe.","caption":"Doses after the Resuscitation Council UK 2021 guideline.","spec":{"nodes":[` +
		`{"id":"s","text":"Suspected anaphylaxis: airway, breathing or circulation problem","shape":"start","next":"adr"},` +
		`{"id":"adr","text":"IM adrenaline 10 micrograms/kg (1 mg/mL), anterolateral thigh","tone":"bad","next":"sup"},` +
		`{"id":"dose","text":"Maximum 500 micrograms per dose","shape":"note","next":"adr"},` +
		`{"id":"sup","text":"Call for help, lie flat, high-flow oxygen, IV fluid 10 mL/kg","next":"re"},` +
		`{"id":"re","text":"Improving after 5 minutes?","shape":"decision"},` +
		`{"id":"rep","text":"Repeat IM adrenaline every 5 minutes","next":"re"},` +
		`{"id":"obs","text":"Observe at least 6 to 12 hours","shape":"end","tone":"good"}],` +
		`"edges":[{"from":"re","to":"obs","label":"Yes"},{"from":"re","to":"rep","label":"No"}]}}`,
	Parse: parseFlowchart,
}

type fcSpec struct {
	Nodes []fcNode `json:"nodes"`
	Edges []fcEdge `json:"edges"`
}

type fcNode struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Shape string `json:"shape"`
	Tone  string `json:"tone"`
	Next  string `json:"next"`
	// Yes and No are a shorthand for a decision's two edges labelled Yes
	// and No. The description no longer offers them (a decision's branches
	// are edges labelled with their answers), but specs written with them
	// are still drawn.
	Yes string `json:"yes"`
	No  string `json:"no"`
}

type fcEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

const (
	maxFlowNodes = 30
	// maxNodeText was 120 characters until 2026-11-10, when the model
	// wrote boxes of 121 to 175 (core.CharsPerWord); a box wraps its
	// text, and a flowchart too tall is refused (tooTall), so a longer
	// box is only a taller one.
	maxNodeText  = 160
	maxEdgeLabel = 32
)

// flowchart is a checked, laid-out flowchart.
type flowchart struct {
	nodes []fcNode
	text  []string
	// fs is each node's text size as its box was sized (size), so it is
	// drawn at the size it was measured at.
	fs []float64
	// pad is each node's padding as its box was sized: the full padding
	// of its shape, or the tight one where its row is short of room.
	pad []float64
	// ladder is each node's ways to set its text (fitLadder).
	ladder [][]fitStep
	lay    *layout
	// edges carry how each is drawn.
	edges []*gedge
}

type fcEdgeData struct {
	note bool
}

func parseFlowchart(raw json.RawMessage) (core.Drawing, error) {
	var s fcSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("nodes", len(s.Nodes), 2, maxFlowNodes)
	ids := make([]string, len(s.Nodes))
	for i, n := range s.Nodes {
		ids[i] = n.ID
	}
	p.Unique("nodes", ids)
	index := map[string]int{}
	for i, id := range ids {
		if _, ok := index[id]; !ok && id != "" {
			index[id] = i
		}
	}
	idsText := idList(ids)
	f := &flowchart{nodes: s.Nodes, text: make([]string, len(s.Nodes)), fs: make([]float64, len(s.Nodes)), pad: make([]float64, len(s.Nodes))}
	type link struct {
		from, to int
		label    string
		bold     bool
		path     string
	}
	var links []link
	target := func(path, id string) int {
		j, ok := index[id]
		if !ok {
			p.Add(path, "%q is not a node id; ids are %s", id, idsText)
			return -1
		}
		return j
	}
	for i := range s.Nodes {
		n := &s.Nodes[i]
		at := core.Path("nodes", i)
		f.text[i] = p.Label(core.Path(at, "text"), n.Text, true, maxNodeText)
		if n.Shape == "" {
			n.Shape = "step"
		}
		p.OneOf(core.Path(at, "shape"), n.Shape, false, "start", "step", "decision", "end", "note")
		p.Tone(core.Path(at, "tone"), n.Tone)
		switch n.Shape {
		case "decision":
			if n.Next != "" {
				p.Add(core.Path(at, "next"), "a decision branches by edges labelled with their answers, not next; give it an edge to each branch")
			}
		default:
			if n.Yes != "" || n.No != "" {
				p.Add(core.Path(at), "only a decision has yes and no; use next, or make it shape \"decision\" with an edge to each branch")
			}
		}
		if n.Shape == "end" && n.Next != "" {
			p.Add(core.Path(at, "next"), "an end has nothing after it; remove next, or make it a step")
		}
		if n.Shape == "note" && n.Next == "" {
			p.Add(core.Path(at, "next"), "a note needs next: the id of the node it explains, which it is drawn beside")
		}
		for _, br := range []struct{ field, id, label string }{{"next", n.Next, ""}, {"yes", n.Yes, "Yes"}, {"no", n.No, "No"}} {
			if br.id == "" {
				continue
			}
			if j := target(core.Path(at, br.field), br.id); j >= 0 {
				links = append(links, link{i, j, br.label, br.label != "", core.Path(at, br.field)})
			}
		}
	}
	for i, e := range s.Edges {
		at := core.Path("edges", i)
		lb := p.Label(core.Path(at, "label"), e.Label, false, maxEdgeLabel)
		a := target(core.Path(at, "from"), e.From)
		b := target(core.Path(at, "to"), e.To)
		if a < 0 || b < 0 {
			continue
		}
		if s.Nodes[a].Shape == "note" {
			p.Add(core.Path(at, "from"), "%q is a note; a note's next names the node it explains, and it links to nothing else", e.From)
			continue
		}
		if s.Nodes[a].Shape == "end" {
			p.Add(core.Path(at, "from"), "%q is an end, which has nothing after it", e.From)
			continue
		}
		if s.Nodes[a].Shape == "decision" && lb == "" {
			p.Add(core.Path(at, "label"), "a branch from decision %q needs a label: the answer it stands for", e.From)
		}
		links = append(links, link{a, b, lb, s.Nodes[a].Shape == "decision", at})
	}
	if p.Any() {
		return nil, p.Err()
	}
	f.ladder = make([][]fitStep, len(s.Nodes))
	for i := range s.Nodes {
		pad, tight, size, _, _ := f.style(i)
		f.ladder[i] = fitLadder(size, pad, tight)
	}
	// Links checked as a whole.
	seen := map[[2]int]bool{}
	out := make([]int, len(s.Nodes))
	in := make([]int, len(s.Nodes))
	for _, l := range links {
		switch {
		case l.from == l.to:
			p.Add(l.path, "links %q to itself; a repeated step is a link back to an earlier node", ids[l.from])
		case seen[[2]int{l.from, l.to}]:
			p.Add(l.path, "links %q to %q a second time; give each pair one link", ids[l.from], ids[l.to])
		case s.Nodes[l.to].Shape == "note" && s.Nodes[l.from].Shape != "note":
			p.Add(l.path, "%q is a note, which nothing links to; give the note next: %q instead, and it is drawn beside that node", ids[l.to], ids[l.from])
		case s.Nodes[l.from].Shape == "note" && s.Nodes[l.to].Shape == "note":
			p.Add(l.path, "a note explains a step, not another note")
		}
		seen[[2]int{l.from, l.to}] = true
		out[l.from]++
		in[l.to]++
	}
	for i, n := range s.Nodes {
		at := core.Path("nodes", i)
		if n.Shape == "decision" && out[i] < 2 {
			p.Add(at, "decision %q has %d branch; a decision needs at least two edges, one per answer, or make it a step with next", n.ID, out[i])
		}
		if out[i] > 4 {
			p.Add(at, "%q has %d links out; at most 4 fit below one box, so split the choice into two decisions", n.ID, out[i])
		}
		if n.Shape == "start" && in[i] > 0 && i != 0 {
			p.Add(at, "a start begins the flow, so nothing links to it; make it a step")
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	// Everything but notes must be reachable from the start.
	adj := make([][]int, len(s.Nodes))
	for _, l := range links {
		if s.Nodes[l.from].Shape != "note" {
			adj[l.from] = append(adj[l.from], l.to)
		}
	}
	roots := []int{0}
	for i, n := range s.Nodes {
		if n.Shape == "start" && i != 0 {
			roots = append(roots, i)
		}
	}
	reach := make([]bool, len(s.Nodes))
	var walk func(int)
	walk = func(v int) {
		if reach[v] {
			return
		}
		reach[v] = true
		for _, w := range adj[v] {
			walk(w)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	for i, n := range s.Nodes {
		if !reach[i] && n.Shape != "note" {
			p.Add(core.Path("nodes", i), "%q cannot be reached from the start (%q); link to it with next, yes or no from an earlier node, or remove it", n.ID, ids[0])
		}
	}
	if s.Nodes[0].Shape == "note" {
		p.Add("nodes[0]", "the first node starts the flow, so it cannot be a note")
	}
	if p.Any() {
		return nil, p.Err()
	}

	// Side branches: a decision's short branch that ends soon sits beside
	// it while the longer branch carries on down.
	reachFrom := func(v int) map[int]bool {
		m := map[int]bool{}
		var w func(int)
		w = func(x int) {
			if m[x] {
				return
			}
			m[x] = true
			for _, y := range adj[x] {
				w(y)
			}
		}
		w(v)
		return m
	}
	build := func(sides bool) (*layout, *core.Problems) {
		g := &graph{n: len(s.Nodes), roots: roots, what: "flowchart", maxW: 236, minW: 76, baseGap: 30,
			name: func(i int) string { return fmt.Sprintf("%q", ids[i]) }}
		g.size = f.size
		g.need = f.need
		g.narrow = func(i int) bool { return s.Nodes[i].Shape == "note" }
		// Lines leave a decision's flat bottom, not its slanted points,
		// and a rounded end's straight part.
		g.inset = func(i int) float64 {
			switch s.Nodes[i].Shape {
			case "decision":
				return fcPoint + 6
			case "start", "end":
				return 16
			}
			return 8
		}
		g.class = func(i int) int {
			switch s.Nodes[i].Shape {
			case "decision":
				return 1
			case "start", "end":
				return 2
			case "note":
				return 3
			}
			return 0
		}
		byFrom := map[int][]int{}
		for _, l := range links {
			e := &gedge{from: l.from, to: l.to, label: l.label, labelBold: l.bold}
			if s.Nodes[l.from].Shape == "note" {
				e.data = fcEdgeData{note: true}
				g.att = append(g.att, attachment{node: l.from, host: l.to, side: 1, edge: len(g.edges)})
			}
			byFrom[l.from] = append(byFrom[l.from], len(g.edges))
			g.edges = append(g.edges, e)
		}
		if sides {
			for i, n := range s.Nodes {
				es := byFrom[i]
				if n.Shape != "decision" || len(es) != 2 {
					continue
				}
				best, bestN := -1, 1<<30
				for k, ei := range es {
					t := g.edges[ei].to
					other := g.edges[es[1-k]].to
					rt, ro := reachFrom(t), reachFrom(other)
					if in[t] != 1 || rt[i] || len(rt) > 3 || len(rt) >= len(ro) || s.Nodes[t].Shape == "decision" {
						continue
					}
					if len(rt) <= bestN {
						best, bestN = ei, len(rt)
					}
				}
				if best >= 0 {
					// Branches sit right; notes take the other side.
					g.att = append([]attachment{{node: g.edges[best].to, host: i, side: 1, edge: best}}, g.att...)
				}
			}
		}
		l := &layout{g: g}
		var lp core.Problems
		l.run(&lp)
		return l, &lp
	}
	l, lp := build(true)
	if lp.Any() {
		l, lp = build(false)
	}
	if lp.Any() {
		return nil, lp.Err()
	}
	f.lay = l
	f.edges = l.g.edges
	return f, nil
}

const (
	fcPadX  = 12.0
	fcPadY  = 9.0
	fcPoint = 13.0 // how far a decision's points reach out
)

// style is node i's padding (full, and tight for a row short of room),
// text size, weight and narrowest width, by its shape. A decision's
// padding includes its points, which text must keep clear of.
func (f *flowchart) style(i int) (pad, tight, size float64, bold bool, minW float64) {
	switch f.nodes[i].Shape {
	case "decision":
		return fcPadX + fcPoint, 6 + fcPoint, nodeSize, true, 130
	case "start", "end":
		return 16, 10, nodeSize, true, 110
	case "note":
		return 9, 6, labelSize + 0.5, false, 80
	}
	return fcPadX, 8, nodeSize, false, 116
}

// need is the narrowest node i can be at step k of its ladder, and false
// past its end.
func (f *flowchart) need(i, k int) (float64, bool) {
	if k >= len(f.ladder[i]) {
		return 0, false
	}
	_, _, _, bold, _ := f.style(i)
	return f.ladder[i][k].need(f.text[i], bold), true
}

// size is node i's width and height for a box at most maxW wide. Its text
// is set at the first step of its ladder that keeps every word whole in
// maxW, and only when none does is a word broken (core.Wrap).
func (f *flowchart) size(i int, maxW float64) (float64, float64) {
	_, _, _, bold, minW := f.style(i)
	if f.nodes[i].Shape == "note" {
		maxW = min(maxW, 136)
	}
	st := pickFit(f.ladder[i], f.text[i], bold, maxW)
	f.fs[i], f.pad[i] = st.size, st.pad
	tb := wrapBlock(f.text[i], st.size, maxW-2*st.pad, bold)
	w := min(maxW, max(minW, tb.w+2*st.pad+2))
	h := max(34, tb.h+2*fcPadY)
	if f.nodes[i].Shape == "note" {
		h = max(28, tb.h+12)
	}
	return w, h
}

func (f *flowchart) Draw(c *core.Canvas) error {
	t := c.T
	for _, e := range f.edges {
		d, _ := e.data.(fcEdgeData)
		if d.note {
			drawRoute(c, e.route, t.Edge, 1.2, true, headNone)
			continue
		}
		drawRoute(c, e.route, t.Muted, 1.4, false, headArrow)
	}
	for i, n := range f.nodes {
		g := f.lay.nodes[i]
		b := box{g.absX() - g.w/2, g.y, g.w, g.h}
		f.drawNode(c, n, f.text[i], f.fs[i], f.pad[i], b)
	}
	for _, e := range f.edges {
		if e.lab != nil {
			lb := *e.lab
			lb.fill = t.Ink
			lb.draw(c)
		}
	}
	c.H = f.lay.H
	return nil
}

// drawNode draws a node's shape and its text, wrapped within pad of each
// side as it was measured (size).
func (f *flowchart) drawNode(c *core.Canvas, n fcNode, text string, size, pad float64, b box) {
	t := c.T
	switch n.Shape {
	case "decision":
		stroke, fill := toneOf(t, n.Tone, t.Series[0], t.SeriesSoft[0])
		pts := []core.Pt{{X: b.X, Y: b.CY()}, {X: b.X + fcPoint, Y: b.Y}, {X: b.R() - fcPoint, Y: b.Y}, {X: b.R(), Y: b.CY()},
			{X: b.R() - fcPoint, Y: b.B()}, {X: b.X + fcPoint, Y: b.B()}}
		c.Polygon(pts, fill, stroke, 1.4)
		tb := wrapBlock(text, size, b.W-2*pad+2, true)
		tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
	case "start", "end":
		stroke, fill := toneOf(t, n.Tone, t.Ink, t.SurfaceAlt)
		if n.Shape == "end" && n.Tone == "" {
			stroke = t.Edge
		}
		c.Rect(b.X, b.Y, b.W, b.H, min(b.H/2, 17), fill, stroke, 1.4)
		tb := wrapBlock(text, size, b.W-2*pad+2, true)
		tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
	case "note":
		c.El("rect", "x", core.F(b.X), "y", core.F(b.Y), "width", core.F(b.W), "height", core.F(b.H), "rx", "4",
			"fill", t.Bg, "stroke", t.Edge, "stroke-width", "1", "stroke-dasharray", "3 2")
		tb := wrapBlock(text, size, b.W-2*pad+2, false)
		tb.draw(c, b.CX(), b.CY(), t.Muted, core.Middle, true)
	default:
		stroke, fill := toneOf(t, n.Tone, t.Edge, t.Surface)
		c.Rect(b.X, b.Y, b.W, b.H, 6, fill, stroke, 1.2)
		tb := wrapBlock(text, size, b.W-2*pad+2, false)
		tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
	}
}
