package diagram

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var schematicKind = core.Kind{
	Name:   "schematic",
	Family: "diagram",
	Use:    "A simple physiology schematic of compartments and what flows between them, placed on a coarse grid (body fluid compartments, fetal circulation shunts, transport across a tubular cell); for a sequence of steps use flowchart.",
	Fields: "compartments: [{id, label (" + core.UpTo(scLabelChars) + "), shape: box|rounded|ellipse|organ|vessel|membrane, col, row (from 1), colspan?, rowspan? (default 1), tone?, in? (the id of the compartment it sits inside)}] (1 to 20). The grid is at most 4 columns and 10 rows; compartments may not overlap, except one inside another that names it in in and lies within its cells. A membrane is a thin bilayer: give it a column (or row) of its own between the compartments it separates. flows: [{from, to, label? (" + core.UpTo(scFlowChars) + "), style?: solid|dashed, effect?: forward|both|inhibits}] (at most 20); a flow is drawn straight between compartments that share a row or column, or with one bend, and may cross only membranes.",
	Example: `{"type":"schematic","title":"Body fluid compartments","alt":"Total body water is two thirds intracellular and one third extracellular; the extracellular fluid is plasma and interstitial fluid. Fluid filters from plasma to the interstitium and returns in lymph; water crosses the cell membrane by osmosis.","spec":{"compartments":[` +
		`{"id":"ecf","label":"Extracellular fluid: 20% of body weight","shape":"rounded","col":1,"row":1,"colspan":2},` +
		`{"id":"pl","label":"Plasma 5%","shape":"vessel","col":1,"row":1,"in":"ecf","tone":"bad"},` +
		`{"id":"isf","label":"Interstitial fluid 15%","shape":"box","col":2,"row":1,"in":"ecf"},` +
		`{"id":"mem","label":"Cell membrane","shape":"membrane","col":1,"row":2,"colspan":2},` +
		`{"id":"icf","label":"Intracellular fluid: 40% of body weight","shape":"rounded","col":1,"row":3,"colspan":2}],"flows":[` +
		`{"from":"pl","to":"isf","label":"filtration"},{"from":"isf","to":"pl","label":"lymph","style":"dashed"},` +
		`{"from":"isf","to":"icf","label":"water","effect":"both"}]}}`,
	Parse: parseSchematic,
}

type scComp struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Shape   string `json:"shape"`
	Col     int    `json:"col"`
	Row     int    `json:"row"`
	Colspan int    `json:"colspan"`
	Rowspan int    `json:"rowspan"`
	Tone    string `json:"tone"`
	In      string `json:"in"`
}

type scFlow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label"`
	Style  string `json:"style"`
	Effect string `json:"effect"`
}

type scSpec struct {
	Compartments []scComp `json:"compartments"`
	Flows        []scFlow `json:"flows"`
}

type schematic struct {
	comps    []scComp
	parent   []int
	depth    []int
	parentOf map[int]bool
	// narrow is the compartments whose label has a word too wide for
	// them even at the smallest size: drawn, it would break inside words.
	narrow []int
	// header is the room a container's label or a membrane's takes at its
	// top.
	header map[int]float64
	boxes  []box
	text   []textBlock
	flows  []scFlow
	routes [][]core.Pt
	labels []*label
	H      float64
}

const (
	scGutX   = 24.0
	scGutY   = 30.0
	scMemW   = 18.0
	scInset  = 9.0
	scHeader = 20.0
	scSize   = 12.0

	// scLabelChars is the longest a compartment's label may be: 50
	// characters until 2026-11-10, when the model wrote labels of 64 and
	// 66 (core.CharsPerWord). A label wraps and its row grows; a word
	// too wide for its compartment is refused (narrow).
	scLabelChars = 70
	scFlowChars  = 30
)

func parseSchematic(raw json.RawMessage) (core.Drawing, error) {
	var s scSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("compartments", len(s.Compartments), 1, 20)
	p.Count("flows", len(s.Flows), 0, 20)
	n := len(s.Compartments)
	ids := make([]string, n)
	for i := range s.Compartments {
		ids[i] = s.Compartments[i].ID
	}
	p.Unique("compartments", ids)
	index := map[string]int{}
	for i, id := range ids {
		index[id] = i
	}
	sc := &schematic{comps: s.Compartments, parent: make([]int, n), depth: make([]int, n), parentOf: map[int]bool{}, header: map[int]float64{}}
	for i := range s.Compartments {
		c := &s.Compartments[i]
		at := core.Path("compartments", i)
		c.Label = p.Label(core.Path(at, "label"), c.Label, c.Shape != "membrane", scLabelChars)
		p.OneOf(core.Path(at, "shape"), c.Shape, false, "box", "rounded", "ellipse", "organ", "vessel", "membrane")
		p.Tone(core.Path(at, "tone"), c.Tone)
		if c.Colspan == 0 {
			c.Colspan = 1
		}
		if c.Rowspan == 0 {
			c.Rowspan = 1
		}
		switch {
		case c.Col < 1 || c.Col+c.Colspan-1 > 4 || c.Colspan < 1:
			p.Add(core.Path(at, "col"), "col %d with colspan %d runs outside the grid; columns are 1 to 4", c.Col, c.Colspan)
		case c.Row < 1 || c.Row+c.Rowspan-1 > 10 || c.Rowspan < 1:
			p.Add(core.Path(at, "row"), "row %d with rowspan %d runs outside the grid; rows are 1 to 10", c.Row, c.Rowspan)
		}
		sc.parent[i] = -1
		if c.In != "" {
			j, ok := index[c.In]
			switch {
			case !ok:
				p.Add(core.Path(at, "in"), "%q is not a compartment id; ids are %s", c.In, idList(ids))
			case j == i:
				p.Add(core.Path(at, "in"), "a compartment cannot sit inside itself")
			default:
				sc.parent[i] = j
				sc.parentOf[j] = true
			}
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	// Nesting: no loops, and a child lies within its parent's cells.
	for i := range s.Compartments {
		d, j := 0, sc.parent[i]
		for j >= 0 && d <= n {
			d++
			j = sc.parent[j]
		}
		if d > n {
			p.Add(core.Path("compartments", i, "in"), "the compartments sit inside each other in a circle")
			return nil, p.Err()
		}
		sc.depth[i] = d
		if q := sc.parent[i]; q >= 0 {
			a, b := s.Compartments[i], s.Compartments[q]
			if a.Col < b.Col || a.Col+a.Colspan > b.Col+b.Colspan || a.Row < b.Row || a.Row+a.Rowspan > b.Row+b.Rowspan {
				p.Add(core.Path("compartments", i), "%q is in %q but its cells (col %d–%d, row %d–%d) are not within %q's (col %d–%d, row %d–%d)",
					a.ID, b.ID, a.Col, a.Col+a.Colspan-1, a.Row, a.Row+a.Rowspan-1, b.ID, b.Col, b.Col+b.Colspan-1, b.Row, b.Row+b.Rowspan-1)
			}
			if b.Shape == "membrane" || b.Shape == "vessel" {
				p.Add(core.Path("compartments", i, "in"), "%q is a %s, which cannot hold another compartment", b.ID, b.Shape)
			}
		}
		if sc.depth[i] > 2 {
			p.Add(core.Path("compartments", i, "in"), "compartments nest at most two deep")
		}
	}
	ancestor := func(a, b int) bool { // a is b or holds it
		for j := b; j >= 0; j = sc.parent[j] {
			if j == a {
				return true
			}
		}
		return false
	}
	for i, a := range s.Compartments {
		for j := i + 1; j < n; j++ {
			b := s.Compartments[j]
			if a.Col < b.Col+b.Colspan && b.Col < a.Col+a.Colspan && a.Row < b.Row+b.Rowspan && b.Row < a.Row+a.Rowspan &&
				!ancestor(i, j) && !ancestor(j, i) {
				p.Add(core.Path("compartments", j), "%q overlaps %q on the grid (both cover col %d, row %d); move one, or put it inside the other with in",
					b.ID, a.ID, max(a.Col, b.Col), max(a.Row, b.Row))
			}
		}
	}
	type fl struct{ a, b int }
	var fls []fl
	for i := range s.Flows {
		f := &s.Flows[i]
		at := core.Path("flows", i)
		f.Label = p.Label(core.Path(at, "label"), f.Label, false, scFlowChars)
		p.OneOf(core.Path(at, "style"), f.Style, true, "solid", "dashed")
		p.OneOf(core.Path(at, "effect"), f.Effect, true, "forward", "both", "inhibits")
		a, okA := index[f.From]
		b, okB := index[f.To]
		if !okA {
			p.Add(core.Path(at, "from"), "%q is not a compartment id; ids are %s", f.From, idList(ids))
		}
		if !okB {
			p.Add(core.Path(at, "to"), "%q is not a compartment id; ids are %s", f.To, idList(ids))
		}
		if !okA || !okB {
			continue
		}
		switch {
		case a == b:
			p.Add(at, "flows from %q to itself", f.From)
		case ancestor(a, b) || ancestor(b, a):
			p.Add(at, "%q and %q are one inside the other; a flow joins two compartments side by side", f.From, f.To)
		case s.Compartments[a].Shape == "membrane" || s.Compartments[b].Shape == "membrane":
			p.Add(at, "a flow joins the compartments on either side of a membrane and crosses it, rather than ending at it")
		}
		fls = append(fls, fl{a, b})
	}
	if p.Any() {
		return nil, p.Err()
	}
	sc.flows = s.Flows
	sc.layout()
	for _, i := range sc.narrow {
		p.Add(core.Path("compartments", i, "label"), "%q has a word wider than its %s, so it would break inside words; give it colspan 2, a box or rounded shape, or a shorter label (a vessel is narrow: use few columns)",
			s.Compartments[i].Label, s.Compartments[i].Shape)
	}
	if p.Any() {
		return nil, p.Err()
	}
	for i, f := range fls {
		pts, why := sc.route(f.a, f.b, i)
		if why != "" {
			p.Add(core.Path("flows", i), "%s", why)
			continue
		}
		sc.routes = append(sc.routes, pts)
	}
	if p.Any() {
		return nil, p.Err()
	}
	sc.placeLabels()
	if sc.H > maxBody {
		tooTall(&p, "schematic", sc.H, "use fewer rows")
		return nil, p.Err()
	}
	return sc, nil
}

// layout sizes the grid and places every compartment.
func (sc *schematic) layout() {
	cols, rows := 0, 0
	for _, c := range sc.comps {
		cols = max(cols, c.Col+c.Colspan-1)
		rows = max(rows, c.Row+c.Rowspan-1)
	}
	// A column or row that holds only membranes is thin.
	thinCol := make([]bool, cols+1)
	thinRow := make([]bool, rows+1)
	for k := 1; k <= cols; k++ {
		thinCol[k] = true
	}
	for k := 1; k <= rows; k++ {
		thinRow[k] = true
	}
	for _, c := range sc.comps {
		mem := c.Shape == "membrane"
		for k := c.Col; k < c.Col+c.Colspan; k++ {
			if !(mem && c.Colspan == 1 && c.Rowspan >= c.Colspan) {
				thinCol[k] = false
			}
		}
		for k := c.Row; k < c.Row+c.Rowspan; k++ {
			if !(mem && c.Rowspan == 1 && c.Colspan > c.Rowspan) {
				thinRow[k] = false
			}
		}
	}
	gutX := make([]float64, cols+1) // after column k
	for k := 1; k < cols; k++ {
		gutX[k] = scGutX
	}
	// Widen the gutters a labelled flow runs through, so its label fits.
	for _, f := range sc.flows {
		if f.Label == "" {
			continue
		}
		a, b := sc.idx(f.From), sc.idx(f.To)
		ca, cb := sc.comps[a], sc.comps[b]
		if ca.Row < cb.Row+cb.Rowspan && cb.Row < ca.Row+ca.Rowspan {
			lo, hi := ca.Col+ca.Colspan-1, cb.Col
			if cb.Col+cb.Colspan-1 < ca.Col {
				lo, hi = cb.Col+cb.Colspan-1, ca.Col
			}
			if hi <= lo {
				continue
			}
			need := math.Min(110, core.TextWidth(f.Label, labelSize, false)) + 18
			have := 0.0
			for k := lo; k < hi; k++ {
				have += gutX[k]
				if k > lo && thinCol[k] {
					have += scMemW
				}
			}
			if have < need {
				g := (need - have) / float64(hi-lo)
				for k := lo; k < hi; k++ {
					gutX[k] += g
				}
			}
		}
	}
	fixed, nWide := 0.0, 0
	for k := 1; k <= cols; k++ {
		fixed += gutX[k]
		if thinCol[k] {
			fixed += scMemW
		} else {
			nWide++
		}
	}
	wide := (avail - fixed) / float64(max(1, nWide))
	colX := make([]float64, cols+2)
	colW := make([]float64, cols+1)
	x := margin
	for k := 1; k <= cols; k++ {
		colX[k] = x
		colW[k] = wide
		if thinCol[k] {
			colW[k] = scMemW
		}
		x += colW[k] + gutX[k]
	}
	// Text first, for the row heights.
	n := len(sc.comps)
	sc.text = make([]textBlock, n)
	cellW := func(c scComp) float64 {
		return colX[c.Col+c.Colspan-1] + colW[c.Col+c.Colspan-1] - colX[c.Col]
	}
	headers := map[int]float64{} // header room at the top of a row
	for i, c := range sc.comps {
		if c.Shape == "membrane" {
			if c.Rowspan >= c.Colspan {
				sc.text[i] = wrapBlock(c.Label, labelSize, scMemW+2*scGutX-6, true)
				if c.Label != "" {
					headers[c.Row] = math.Max(headers[c.Row], sc.text[i].h+6)
				}
			} else {
				sc.text[i] = wrapBlock(c.Label, labelSize, 400, true)
			}
			continue
		}
		w := cellW(c) - 2*scInset*float64(sc.depth[i]) - 16
		size := scSize
		bold := sc.parentOf[i]
		if c.Shape == "ellipse" || c.Shape == "organ" {
			w *= 0.74
		}
		fs := fitSize(c.Label, size, w, bold)
		for _, word := range strings.Fields(c.Label) {
			if core.TextWidth(word, fs, bold) > w {
				sc.narrow = append(sc.narrow, i)
				break
			}
		}
		sc.text[i] = wrapBlock(c.Label, fs, w, bold)
		if sc.parentOf[i] {
			headers[c.Row] = math.Max(headers[c.Row], sc.text[i].h+10)
			sc.header[i] = sc.text[i].h + 10
		}
	}
	rowH := make([]float64, rows+1)
	for k := 1; k <= rows; k++ {
		rowH[k] = 50
		if thinRow[k] {
			rowH[k] = scMemW
		}
	}
	for i, c := range sc.comps {
		if c.Rowspan != 1 || sc.parentOf[i] || c.Shape == "membrane" {
			continue
		}
		h := sc.text[i].h + 22 + 2*scInset*float64(sc.depth[i])
		if c.Shape == "ellipse" || c.Shape == "organ" {
			h = sc.text[i].h*1.5 + 26 + 2*scInset*float64(sc.depth[i])
		}
		if sc.depth[i] > 0 && sc.comps[sc.parent[i]].Row == c.Row {
			h += headers[c.Row]
		}
		rowH[c.Row] = math.Max(rowH[c.Row], h)
	}
	// Room for flows that leave a row: two flows between the same pair
	// sit one above the other.
	pairs := map[[2]int]int{}
	for _, f := range sc.flows {
		a, b := sc.idx(f.From), sc.idx(f.To)
		pairs[[2]int{min(a, b), max(a, b)}]++
	}
	for pr, k := range pairs {
		ca, cb := sc.comps[pr[0]], sc.comps[pr[1]]
		if ca.Rowspan == 1 && cb.Rowspan == 1 && ca.Row == cb.Row {
			rowH[ca.Row] = math.Max(rowH[ca.Row], float64(k)*30+headers[ca.Row]+10)
		}
	}
	rowY := make([]float64, rows+2)
	y := 2.0
	for k := 1; k <= rows; k++ {
		rowY[k] = y
		y += rowH[k] + scGutY
	}
	sc.H = y - scGutY + 2
	sc.boxes = make([]box, n)
	for i, c := range sc.comps {
		x0 := colX[c.Col]
		x1 := colX[c.Col+c.Colspan-1] + colW[c.Col+c.Colspan-1]
		y0 := rowY[c.Row]
		y1 := rowY[c.Row+c.Rowspan-1] + rowH[c.Row+c.Rowspan-1]
		in := scInset * float64(sc.depth[i])
		top := in
		if sc.depth[i] > 0 && sc.comps[sc.parent[i]].Row == c.Row {
			top += headers[c.Row]
		}
		if c.Shape == "membrane" {
			if c.Rowspan >= c.Colspan && c.Label != "" {
				sc.header[i] = sc.text[i].h + 6
			}
			sc.boxes[i] = box{x0, y0 + top, x1 - x0, y1 - y0 - top - in}
			continue
		}
		sc.boxes[i] = box{x0 + in, y0 + top, x1 - x0 - 2*in, y1 - y0 - top - in}
	}
}

func (sc *schematic) idx(id string) int {
	for i, c := range sc.comps {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// blocks says whether compartment k is in the way of a flow from a to b:
// it is drawn solid, and holds neither end.
func (sc *schematic) blocks(k, a, b int) bool {
	if k == a || k == b || sc.comps[k].Shape == "membrane" {
		return false
	}
	for _, e := range []int{a, b} {
		for j := sc.parent[e]; j >= 0; j = sc.parent[j] {
			if j == k {
				return false
			}
		}
	}
	return true
}

func (sc *schematic) clear(pts []core.Pt, a, b int) string {
	for k, bx := range sc.boxes {
		if !sc.blocks(k, a, b) {
			continue
		}
		for i := 0; i+1 < len(pts); i++ {
			if segHitsBox(pts[i], pts[i+1], bx) {
				return sc.comps[k].ID
			}
		}
	}
	return ""
}

// route is a flow's line: straight where the two share a row or column,
// else with one bend; why says what is in the way when neither is clear.
func (sc *schematic) route(a, b, fi int) ([]core.Pt, string) {
	A, B := sc.boxes[a], sc.boxes[b]
	// Flows between the same pair are spread across their overlap.
	k, n := 0, 0
	for i, f := range sc.flows {
		x, y := sc.idx(f.From), sc.idx(f.To)
		if min(x, y) == min(a, b) && max(x, y) == max(a, b) {
			if i < fi {
				k++
			}
			n++
		}
	}
	at := func(lo, hi float64) float64 {
		return lo + (hi-lo)*float64(k+1)/float64(n+1)
	}
	var tries [][]core.Pt
	if lo, hi := math.Max(A.Y, B.Y)+6, math.Min(A.B(), B.B())-6; hi-lo >= 6 {
		x0, x1 := math.Min(A.R(), B.R()), math.Max(A.X, B.X)
		for m, mb := range sc.boxes {
			if h := sc.header[m]; h > 0 && sc.comps[m].Shape == "membrane" && mb.CX() > x0 && mb.CX() < x1 {
				lo = math.Max(lo, mb.Y+h+6)
			}
		}
		y := at(lo, hi)
		if B.X >= A.R() {
			tries = append(tries, []core.Pt{{X: A.R(), Y: y}, {X: B.X, Y: y}})
		} else {
			tries = append(tries, []core.Pt{{X: A.X, Y: y}, {X: B.R(), Y: y}})
		}
	} else if lo, hi := math.Max(A.X, B.X)+6, math.Min(A.R(), B.R())-6; hi-lo >= 6 {
		x := at(lo, hi)
		if B.Y >= A.B() {
			tries = append(tries, []core.Pt{{X: x, Y: A.B()}, {X: x, Y: B.Y}})
		} else {
			tries = append(tries, []core.Pt{{X: x, Y: A.Y}, {X: x, Y: B.B()}})
		}
	} else {
		// Across then down (or up), or down then across.
		sx := A.R()
		if B.CX() < A.CX() {
			sx = A.X
		}
		ey := B.Y
		if B.CY() < A.CY() {
			ey = B.B()
		}
		tries = append(tries, []core.Pt{{X: sx, Y: A.CY()}, {X: B.CX(), Y: A.CY()}, {X: B.CX(), Y: ey}})
		sy := A.B()
		if B.CY() < A.CY() {
			sy = A.Y
		}
		ex := B.X
		if B.CX() < A.CX() {
			ex = B.R()
		}
		tries = append(tries, []core.Pt{{X: A.CX(), Y: sy}, {X: A.CX(), Y: B.CY()}, {X: ex, Y: B.CY()}})
	}
	var in string
	for _, t := range tries {
		if dist(t[0], t[len(t)-1]) < 12 && len(t) == 2 {
			return nil, fmt.Sprintf("%q and %q touch, leaving no room for the arrow; leave a column or row between them, or a membrane", sc.comps[a].ID, sc.comps[b].ID)
		}
		if in = sc.clear(t, a, b); in == "" {
			return t, ""
		}
	}
	return nil, fmt.Sprintf("the flow from %q to %q would cross %q; place %q and %q in the same row or column with nothing solid between them, or move %q",
		sc.comps[a].ID, sc.comps[b].ID, in, sc.comps[a].ID, sc.comps[b].ID, in)
}

func (sc *schematic) placeLabels() {
	var taken []box
	for i, bx := range sc.boxes {
		if !sc.parentOf[i] && sc.comps[i].Shape != "membrane" {
			taken = append(taken, bx)
		}
	}
	sc.labels = make([]*label, len(sc.routes))
	for i, r := range sc.routes {
		f := sc.flows[i]
		if f.Label == "" {
			continue
		}
		tb := wrapBlock(f.Label, labelSize, 110, false)
		w, h := tb.w+4, tb.h
		// The middle of the longest leg.
		best, bl := 0, -1.0
		for k := 0; k+1 < len(r); k++ {
			if d := dist(r[k], r[k+1]); d > bl {
				best, bl = k, d
			}
		}
		a, b := r[best], r[best+1]
		mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
		var cands []box
		if math.Abs(a.Y-b.Y) < 0.5 {
			cands = []box{{mx - w/2, my - h - 3, w, h}, {mx - w/2, my + 3, w, h}}
		} else {
			cands = []box{{mx + 5, my - h/2, w, h}, {mx - 5 - w, my - h/2, w, h}}
		}
		pick, score := cands[0], math.MaxInt
		for _, cb := range cands {
			s := 0
			for _, t := range taken {
				if cb.overlaps(t, 1) {
					s++
				}
			}
			if cb.X < 1 || cb.R() > core.Width-1 {
				s += 2
			}
			if s < score {
				pick, score = cb, s
			}
		}
		taken = append(taken, pick)
		sc.labels[i] = &label{text: tb, at: pick}
	}
}

func (sc *schematic) Draw(c *core.Canvas) error {
	t := c.T
	order := make([]int, len(sc.comps))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return sc.depth[order[i]] < sc.depth[order[j]] })
	for _, i := range order {
		sc.drawComp(c, i)
	}
	for i, r := range sc.routes {
		f := sc.flows[i]
		stroke, w := t.Ink, 1.6
		h := headArrow
		if f.Effect == "inhibits" {
			stroke, h = t.Bad, headBar
		}
		pts := r
		if f.Effect == "both" {
			// Two heads: draw the line, then a head at each end.
			drawRoute(c, pts, stroke, w, f.Style == "dashed", headArrow)
			rev := make([]core.Pt, len(pts))
			for k := range pts {
				rev[k] = pts[len(pts)-1-k]
			}
			c.Polygon(core.ArrowHead(toward(rev[len(rev)-1], rev[len(rev)-2], 10), rev[len(rev)-1], arrowLen(w)), stroke, "", 0)
			continue
		}
		drawRoute(c, pts, stroke, w, f.Style == "dashed", h)
	}
	for _, l := range sc.labels {
		if l != nil {
			lb := *l
			lb.fill = t.Ink
			c.Rect(lb.at.X-2, lb.at.Y+1, lb.at.W+4, lb.at.H-2, 3, t.Bg, "", 0)
			lb.draw(c)
		}
	}
	c.H = sc.H
	return nil
}

func (sc *schematic) drawComp(c *core.Canvas, i int) {
	t := c.T
	cp := sc.comps[i]
	b := sc.boxes[i]
	tb := sc.text[i]
	switch cp.Shape {
	case "membrane":
		stroke, _ := toneOf(t, cp.Tone, t.Warn, "")
		vertical := b.H >= b.W
		if vertical {
			x := b.CX()
			if h := sc.header[i]; h > 0 {
				tb.draw(c, x, b.Y+tb.h/2, t.Muted, core.Middle, false)
				b.Y += h
				b.H -= h
			}
			c.Rect(x-5, b.Y, 10, b.H, 0, t.Bg, "", 0)
			c.Line(x-2, b.Y, x-2, b.B(), t.Edge, 1, "")
			c.Line(x+2, b.Y, x+2, b.B(), t.Edge, 1, "")
			for y := b.Y + 3; y <= b.B()-2; y += 6 {
				c.Circle(x-5, y, 2.4, stroke, "", 0)
				c.Circle(x+5, y, 2.4, stroke, "", 0)
			}
			return
		}
		y := b.CY()
		c.Rect(b.X, y-5, b.W, 10, 0, t.Bg, "", 0)
		c.Line(b.X, y-2, b.R(), y-2, t.Edge, 1, "")
		c.Line(b.X, y+2, b.R(), y+2, t.Edge, 1, "")
		for x := b.X + 3; x <= b.R()-2; x += 6 {
			c.Circle(x, y-5, 2.4, stroke, "", 0)
			c.Circle(x, y+5, 2.4, stroke, "", 0)
		}
		if cp.Label != "" {
			lo, hi := sc.freeStretch(b, false)
			l := core.Ellipsize(cp.Label, labelSize, hi-lo-8, true)
			lw := core.TextWidth(l, labelSize, true)
			c.Rect((lo+hi)/2-lw/2-4, y-8, lw+8, 16, 3, t.Bg, "", 0)
			c.Text((lo+hi)/2, y, l, core.TextStyle{Size: labelSize, Bold: true, Fill: t.Ink, Anchor: core.Middle, Halo: t.Bg})
		}
		return
	case "ellipse":
		stroke, fill := toneOf(t, cp.Tone, t.Edge, t.Surface)
		c.Ellipse(b.CX(), b.CY(), b.W/2, b.H/2, fill, stroke, 1.4)
	case "organ":
		stroke, fill := toneOf(t, cp.Tone, t.Muscle, t.Flesh)
		c.Path(organPath(b), fill, stroke, 1.6)
	case "vessel":
		stroke, fill := toneOf(t, cp.Tone, t.Series[0], t.SeriesSoft[0])
		r := math.Min(b.W, b.H) / 2
		c.Rect(b.X, b.Y, b.W, b.H, r, fill, stroke, 2.4)
	default:
		r := 3.0
		if cp.Shape == "rounded" {
			r = 12
		}
		stroke, fill := toneOf(t, cp.Tone, t.Edge, t.Surface)
		if sc.parentOf[i] && cp.Tone == "" {
			fill = t.Bg
			if sc.depth[i] > 0 {
				fill = t.Surface
			}
		}
		c.Rect(b.X, b.Y, b.W, b.H, r, fill, stroke, 1.3)
	}
	if sc.parentOf[i] {
		tb.draw(c, b.X+10, b.Y+5+tb.h/2, t.Ink, core.Start, false)
		return
	}
	tb.draw(c, b.CX(), b.CY(), t.Ink, core.Middle, false)
}

// freeStretch is the longest stretch along a membrane that no flow or
// flow label crosses, where its own label goes.
func (sc *schematic) freeStretch(b box, vertical bool) (float64, float64) {
	lo, hi := b.Y, b.B()
	if !vertical {
		lo, hi = b.X, b.R()
	}
	var cuts [][2]float64
	for i, r := range sc.routes {
		for k := 0; k+1 < len(r); k++ {
			if segHitsBox(r[k], r[k+1], b.grow(2)) {
				if vertical {
					cuts = append(cuts, [2]float64{math.Min(r[k].Y, r[k+1].Y) - 8, math.Max(r[k].Y, r[k+1].Y) + 8})
				} else {
					cuts = append(cuts, [2]float64{math.Min(r[k].X, r[k+1].X) - 8, math.Max(r[k].X, r[k+1].X) + 8})
				}
			}
		}
		if l := sc.labels[i]; l != nil && l.at.overlaps(b, 0) {
			if vertical {
				cuts = append(cuts, [2]float64{l.at.Y - 3, l.at.B() + 3})
			} else {
				cuts = append(cuts, [2]float64{l.at.X - 3, l.at.R() + 3})
			}
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i][0] < cuts[j][0] })
	bestLo, bestHi, at := lo, lo, lo
	for _, c := range cuts {
		if c[0]-at > bestHi-bestLo {
			bestLo, bestHi = at, c[0]
		}
		at = math.Max(at, c[1])
	}
	if hi-at > bestHi-bestLo {
		bestLo, bestHi = at, hi
	}
	return bestLo, bestHi
}

// organPath is a soft, slightly irregular rounded outline within b.
func organPath(b box) string {
	f := core.F
	x0, y0, x1, y1 := b.X, b.Y, b.R(), b.B()
	cx, cy := b.CX(), b.CY()
	k := 0.55
	return "M" + f(cx) + " " + f(y0) +
		"C" + f(cx+(x1-cx)*k) + " " + f(y0-2) + " " + f(x1+2) + " " + f(cy-(cy-y0)*k) + " " + f(x1) + " " + f(cy) +
		"C" + f(x1-2) + " " + f(cy+(y1-cy)*k) + " " + f(cx+(x1-cx)*k) + " " + f(y1+1) + " " + f(cx) + " " + f(y1) +
		"C" + f(cx-(cx-x0)*k) + " " + f(y1-1) + " " + f(x0-1) + " " + f(cy+(y1-cy)*k) + " " + f(x0) + " " + f(cy) +
		"C" + f(x0+2) + " " + f(cy-(cy-y0)*k) + " " + f(cx-(cx-x0)*k) + " " + f(y0+2) + " " + f(cx) + " " + f(y0) + "Z"
}
