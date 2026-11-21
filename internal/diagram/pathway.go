package diagram

import (
	"encoding/json"
	"fmt"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var pathwayKind = core.Kind{
	Name:   "pathway",
	Family: "diagram",
	Use:    "A cascade, axis or signalling pathway (coagulation, RAAS, the HPA axis): what activates, converts, releases or inhibits what, with drugs acting from the side and feedback loops; for a management algorithm with decisions use flowchart.",
	Fields: "nodes: [{id, text (" + core.UpTo(pwNodeChars) + "), tone?, input?}] (2 to 24). input: true draws a drug or stimulus beside the node it acts on; it should have exactly one link, from it (an input with more links is drawn in the pathway). links: [{from, to, effect: activates|inhibits|converts|releases, label? (" + core.UpTo(maxEdgeLabel) + ", e.g. the enzyme)}] (1 to 40). A link back up the pathway (negative feedback) is drawn round the side. Chains that meet are laid out to converge.",
	Example: `{"type":"pathway","title":"Renin–angiotensin–aldosterone system","alt":"Renin and ACE convert angiotensinogen to angiotensin II, which constricts arterioles and releases aldosterone; both raise blood pressure, which reduces renin. ACE inhibitors block the last step.","spec":{"nodes":[` +
		`{"id":"agt","text":"Angiotensinogen"},{"id":"a1","text":"Angiotensin I"},{"id":"a2","text":"Angiotensin II","tone":"accent"},` +
		`{"id":"acei","text":"ACE inhibitor","input":true,"tone":"bad"},{"id":"vc","text":"Arteriolar vasoconstriction"},{"id":"ald","text":"Aldosterone"},` +
		`{"id":"na","text":"Salt and water kept"},{"id":"bp","text":"Blood pressure rises","tone":"good"}],"links":[` +
		`{"from":"agt","to":"a1","effect":"converts","label":"renin"},{"from":"a1","to":"a2","effect":"converts","label":"ACE (lung)"},` +
		`{"from":"acei","to":"a2","effect":"inhibits"},{"from":"a2","to":"vc","effect":"activates"},{"from":"a2","to":"ald","effect":"releases"},` +
		`{"from":"ald","to":"na","effect":"activates"},{"from":"vc","to":"bp","effect":"activates"},{"from":"na","to":"bp","effect":"activates"},` +
		`{"from":"bp","to":"agt","effect":"inhibits","label":"less renin"}]}}`,
	Parse: parsePathway,
}

type pwSpec struct {
	Nodes []pwNode `json:"nodes"`
	Links []pwLink `json:"links"`
}

type pwNode struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Tone  string `json:"tone"`
	Input bool   `json:"input"`
}

type pwLink struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Effect string `json:"effect"`
	Label  string `json:"label"`
}

// pwNodeChars is the longest a node's text may be. 80 characters until
// 2026-11-10, when the model wrote nodes of 81 to 85 (core.CharsPerWord);
// a node wraps its text, and a pathway too tall is refused.
const pwNodeChars = 100

var effects = []string{"activates", "inhibits", "converts", "releases"}

type pathway struct {
	nodes []pwNode
	text  []string
	lay   *layout
	// fs and pad are each node's text size and padding as its box was
	// sized, and ladder its ways to set them (fitLadder).
	fs, pad []float64
	ladder  [][]fitStep
	used    map[string]bool
	legend  float64
}

func parsePathway(raw json.RawMessage) (core.Drawing, error) {
	var s pwSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("nodes", len(s.Nodes), 2, 24)
	p.Count("links", len(s.Links), 1, 40)
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
	w := &pathway{nodes: s.Nodes, text: make([]string, len(s.Nodes)), used: map[string]bool{}}
	for i, n := range s.Nodes {
		w.text[i] = p.Label(core.Path("nodes", i, "text"), n.Text, true, pwNodeChars)
		p.Tone(core.Path("nodes", i, "tone"), n.Tone)
	}
	g := &graph{n: len(s.Nodes), what: "pathway", late: true, labelMid: true, maxW: 200, minW: 70, baseGap: 30,
		name: func(i int) string { return fmt.Sprintf("%q", ids[i]) }}
	outN, inN := make([]int, len(s.Nodes)), make([]int, len(s.Nodes))
	seen := map[[2]int]bool{}
	for i, l := range s.Links {
		at := core.Path("links", i)
		lb := p.Label(core.Path(at, "label"), l.Label, false, maxEdgeLabel)
		p.OneOf(core.Path(at, "effect"), l.Effect, false, effects...)
		a, okA := index[l.From]
		b, okB := index[l.To]
		if !okA {
			p.Add(core.Path(at, "from"), "%q is not a node id; ids are %s", l.From, idList(ids))
		}
		if !okB {
			p.Add(core.Path(at, "to"), "%q is not a node id; ids are %s", l.To, idList(ids))
		}
		if !okA || !okB {
			continue
		}
		switch {
		case a == b:
			p.Add(at, "links %q to itself", l.From)
			continue
		case seen[[2]int{a, b}]:
			p.Add(at, "links %q to %q a second time; give each pair one link", l.From, l.To)
			continue
		}
		seen[[2]int{a, b}] = true
		outN[a]++
		inN[b]++
		w.used[l.Effect] = true
		g.edges = append(g.edges, &gedge{from: a, to: b, label: lb, data: l.Effect})
	}
	for i, n := range s.Nodes {
		at := core.Path("nodes", i)
		// An input acting on more than one node, or acted on itself (an
		// enzyme that is also a product), cannot sit beside one node: it is
		// drawn in the pathway instead, which shows the same links.
		if n.Input && outN[i]+inN[i] > 0 && (outN[i] != 1 || inN[i] != 0) {
			s.Nodes[i].Input = false
			n.Input = false
		}
		switch {
		case n.Input && outN[i]+inN[i] == 0:
			p.Add(at, "input %q is not linked to anything; give it one link, from it to the node it acts on", n.ID)
		case !n.Input && outN[i]+inN[i] == 0:
			p.Add(at, "%q is not linked to anything; link it or remove it", n.ID)
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	for ei, e := range g.edges {
		if s.Nodes[e.from].Input {
			if s.Nodes[e.to].Input {
				p.Add("", "input %q acts on another input; an input acts on a node in the pathway", ids[e.from])
				continue
			}
			g.att = append(g.att, attachment{node: e.from, host: e.to, edge: ei})
		}
	}
	for i, n := range s.Nodes {
		if !n.Input {
			g.roots = append(g.roots, i)
			break
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	w.fs, w.pad, w.ladder = make([]float64, len(s.Nodes)), make([]float64, len(s.Nodes)), make([][]fitStep, len(s.Nodes))
	for i := range s.Nodes {
		pad, tight, size, _ := w.style(i)
		w.ladder[i] = fitLadder(size, pad, tight)
	}
	g.size = w.size
	g.need = w.need
	g.class = func(i int) int {
		if s.Nodes[i].Input {
			return 1
		}
		return 0
	}
	g.narrow = func(i int) bool { return s.Nodes[i].Input }
	l := &layout{g: g}
	var lp core.Problems
	if !l.run(&lp) {
		return nil, lp.Err()
	}
	w.lay = l
	if len(w.used) > 1 {
		w.legend = 30
		if l.H+w.legend > maxBody {
			tooTall(&p, "pathway", l.H+w.legend, "draw fewer steps, or split it into two figures")
			return nil, p.Err()
		}
	}
	return w, nil
}

// style is node i's padding (full and tight), text size and weight: an
// input is a bold pill, a step a plain box.
func (w *pathway) style(i int) (pad, tight, size float64, bold bool) {
	if w.nodes[i].Input {
		return 10, 8, labelSize + 0.5, true
	}
	return 11, 7, nodeSize, false
}

// need is the narrowest node i can be at step k of its ladder, and false
// past its end.
func (w *pathway) need(i, k int) (float64, bool) {
	if k >= len(w.ladder[i]) {
		return 0, false
	}
	_, _, _, bold := w.style(i)
	return w.ladder[i][k].need(w.text[i], bold), true
}

// size is node i's width and height for a box at most maxW wide, its text
// set at the first step of its ladder that keeps every word whole.
func (w *pathway) size(i int, maxW float64) (float64, float64) {
	_, _, _, bold := w.style(i)
	if w.nodes[i].Input {
		maxW = min(maxW, 150)
	}
	st := pickFit(w.ladder[i], w.text[i], bold, maxW)
	w.fs[i], w.pad[i] = st.size, st.pad
	tb := wrapBlock(w.text[i], st.size, maxW-2*st.pad, bold)
	if w.nodes[i].Input {
		return max(64, tb.w+2*st.pad+2), max(26, tb.h+10)
	}
	return min(maxW, max(96, tb.w+2*st.pad+2)), max(32, tb.h+16)
}

// effectStyle is how a link of an effect is drawn.
func effectStyle(t *core.Theme, effect string) (stroke string, width float64, dashed bool, h head) {
	switch effect {
	case "inhibits":
		return t.Bad, 1.6, false, headBar
	case "converts":
		return t.Ink, 1.6, false, headArrow
	case "releases":
		return t.Muted, 1.4, true, headArrow
	}
	return t.Muted, 1.4, false, headArrow
}

func (w *pathway) Draw(c *core.Canvas) error {
	t := c.T
	for _, e := range w.lay.g.edges {
		st, sw, dash, h := effectStyle(t, e.data.(string))
		drawRoute(c, e.route, st, sw, dash, h)
	}
	for i, n := range w.nodes {
		g := w.lay.nodes[i]
		b := box{g.absX() - g.w/2, g.y, g.w, g.h}
		if n.Input {
			stroke, fill := toneOf(t, n.Tone, t.Muted, t.SurfaceAlt)
			c.Rect(b.X, b.Y, b.W, b.H, b.H/2, fill, stroke, 1.2)
			tb := wrapBlock(w.text[i], w.fs[i], b.W-2*w.pad[i]+2, true)
			tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
			continue
		}
		stroke, fill := toneOf(t, n.Tone, t.Edge, t.Surface)
		c.Rect(b.X, b.Y, b.W, b.H, 8, fill, stroke, 1.3)
		tb := wrapBlock(w.text[i], w.fs[i], b.W-2*w.pad[i]+2, false)
		tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
	}
	for _, e := range w.lay.g.edges {
		if e.lab != nil {
			lb := *e.lab
			lb.italic = e.data.(string) == "converts"
			if e.data.(string) == "inhibits" {
				lb.fill = t.Bad
			}
			lb.draw(c)
		}
	}
	c.H = w.lay.H
	if w.legend > 0 {
		w.drawLegend(c, w.lay.H+8)
		c.H += w.legend
	}
	return nil
}

// drawLegend explains the kinds of line used, in one centred row.
func (w *pathway) drawLegend(c *core.Canvas, y float64) {
	t := c.T
	names := map[string]string{"activates": "activates", "inhibits": "inhibits", "converts": "converted to", "releases": "releases"}
	var items []string
	for _, e := range effects {
		if w.used[e] {
			items = append(items, e)
		}
	}
	const sample = 26.0
	total := 0.0
	for i, e := range items {
		if i > 0 {
			total += 16
		}
		total += sample + 5 + core.TextWidth(names[e], labelSize, false)
	}
	x := core.Width/2 - total/2
	cy := y + 8
	for _, e := range items {
		st, sw, dash, h := effectStyle(t, e)
		drawRoute(c, []core.Pt{{X: x, Y: cy}, {X: x + sample, Y: cy}}, st, sw, dash, h)
		x += sample + 5
		c.Text(x, cy, names[e], core.TextStyle{Size: labelSize, Fill: t.Muted})
		x += core.TextWidth(names[e], labelSize, false) + 16
	}
}
