package diagram

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// The flowchart and the pathway are laid out the same way, top down in
// rows (a layered, Sugiyama-style layout):
//
//  1. Edges that close a loop (back to a node already on the path) are set
//     aside and later drawn round the side in a lane of their own.
//  2. Each node gets a row by the longest path to it, so every edge points
//     down. A pathway then moves each node as low as its successors allow,
//     so two chains that meet (the intrinsic and extrinsic pathways
//     meeting at factor X) arrive side by side.
//  3. An edge that skips rows passes through a placeholder in each row it
//     crosses, which keeps it clear of the boxes there.
//  4. Each row is ordered by the average position of its neighbours (the
//     barycentre), sweeping down and up, keeping the order with the fewest
//     crossings.
//  5. Boxes are sized for the room their row leaves, the same size across a
//     row, and placed left to right as near as they can be to their
//     neighbours (a least-squares fit under the spacing constraints,
//     solved exactly by pooling adjacent violators).
//  6. Lines are routed orthogonally: down from the bottom of a box, across
//     in the gap between rows on a track of their own, and down into the
//     top of the target; a branch whose target lies to one side leaves from
//     the side. Labels go beside the start of their line, where they are
//     read with the branch.
//
// Some nodes sit beside another in the same row: a decision's short branch
// (the "No" that ends in one step while "Yes" carries on), a note, or a
// drug acting on a step. These attachments are joined to their host by a
// short straight line.

// gnode is a node or a placeholder in the layout.
type gnode struct {
	idx   int // the caller's node index; -1 for a placeholder
	layer int
	pos   int
	// x is the centre, y the top.
	x, y, w, h float64
	// host is the node an attachment sits beside, on side (-1 left, +1
	// right); left and right are a host's attachments, gapL and gapR the
	// space to them.
	host        *gnode
	side        int
	left, right *gnode
	gapL, gapR  float64
	// sideUsed says a left (0) or right (1) port is taken by a line.
	sideUsed [2]bool
}

func (n *gnode) dummy() bool { return n.idx < 0 }

func (n *gnode) bbox() box { return box{n.x - n.w/2, n.y, n.w, n.h} }

// leftExt and rightExt are how far a host and its attachments reach from
// the host's centre.
func (n *gnode) leftExt() float64 {
	e := n.w / 2
	if n.left != nil {
		e += n.gapL + n.left.w
	}
	return e
}

func (n *gnode) rightExt() float64 {
	e := n.w / 2
	if n.right != nil {
		e += n.gapR + n.right.w
	}
	return e
}

// offset is where an attachment's centre is from its host's.
func (n *gnode) offset() float64 {
	if n.host == nil {
		return 0
	}
	if n.side < 0 {
		return -(n.host.w/2 + n.host.gapL + n.w/2)
	}
	return n.host.w/2 + n.host.gapR + n.w/2
}

// item is what is ordered in a row: the host of an attachment, or itself.
func (n *gnode) item() *gnode {
	if n.host != nil {
		return n.host
	}
	return n
}

// gedge is an edge to lay out; the caller sets from, to, label and how it
// is drawn, and the layout sets the rest.
type gedge struct {
	from, to int
	label    string
	// labelBold sets the label in bold (a decision's branch).
	labelBold bool
	// the caller's own data, carried through.
	data any

	back   bool
	attach bool
	chain  []*gnode // from, placeholders, to
	route  []core.Pt
	lab    *label
	side   int // the side a back edge goes round
	exit   int // -1, 0 (bottom) or +1: where it leaves its source
	refs   []rpt
	// straight is the x of a lone line drawn straight, or 0.
	straight float64
	// depart is where a node's only line leaves its bottom when it heads
	// off to a side (0: the bottom's middle).
	depart float64
}

// attachment puts node beside host; side is the side wanted (0: either).
type attachment struct {
	node, host int
	side       int
	edge       int // the edge joining them
}

// graph is a layout's input.
type graph struct {
	n     int
	edges []*gedge
	att   []attachment
	// size is a node's width and height when its text may be at most maxW
	// wide; class groups nodes that share a size within a row.
	size  func(i int, maxW float64) (w, h float64)
	class func(i int) int
	// whole is the width node i needs for every word of its text to stay
	// whole at its own text size (optional): a row gives a node with a
	// long word more of its width before the node's text is made smaller
	// or a word in it broken.
	whole func(i int) float64
	// roots are where the flow starts, for finding loops.
	roots []int
	late  bool
	// what names the figure in messages; name a node for them.
	what string
	name func(i int) string
	// narrow says a node is sized on its own before the rest of its row
	// share what is left.
	narrow func(i int) bool
	// maxW is the widest a node may be; labelMid places labels at the
	// middle of their line rather than at its start.
	maxW, minW float64
	labelMid   bool
	baseGap    float64
}

// layout is a laid-out graph.
type layout struct {
	g      *graph
	nodes  []*gnode // by the caller's index
	layers [][]*gnode
	all    []*gnode // every node and placeholder
	H      float64
	chans  map[int]*channel
}

const (
	gapNodes   = 22.0 // between boxes in a row
	gapDummy   = 12.0 // between a box and a passing line
	gapDummies = 9.0  // between two passing lines
	dummyW     = 2.0
	trackSep   = 9.0
	laneSep    = 10.0
	attachGap  = 30.0
)

// rpt is a route point whose y is known only once rows are placed: a
// fraction of a node's height, or a track.
type rpt struct {
	x    float64
	n    *gnode
	frac float64
	s    *seg
}

func (l *layout) run(p *core.Problems) bool {
	g := l.g
	l.nodes = make([]*gnode, g.n)
	for i := range l.nodes {
		l.nodes[i] = &gnode{idx: i}
	}
	attached := map[int]attachment{}
	for _, a := range g.att {
		attached[a.node] = a
		g.edges[a.edge].attach = true
	}
	l.findBack(attached)
	if !l.assignLayers(attached) {
		p.Add("", "the %s's links go round in a circle with no way in; make sure the first node starts the flow", g.what)
		return false
	}
	if !l.attach(p) {
		return false
	}
	l.buildLayers()
	l.order()
	if !l.sizeRows(p) {
		return false
	}
	l.placeX()
	l.route()
	if l.H > maxBody {
		tooTall(p, g.what, l.H, fmt.Sprintf("it has %d rows. Merge consecutive steps into one, shorten the text in each box, or split it into two figures", len(l.layers)))
		return false
	}
	return true
}

// findBack marks the edges that close a loop, by a depth-first walk from
// the roots in the order given.
func (l *layout) findBack(attached map[int]attachment) {
	g := l.g
	out := make([][]int, g.n)
	for ei, e := range g.edges {
		if e.attach {
			a := attached[e.to]
			if a.host != e.from || a.node != e.to {
				continue // a note or an input: not part of the flow
			}
		}
		out[e.from] = append(out[e.from], ei)
	}
	state := make([]int, g.n) // 0 new, 1 on the path, 2 done
	var visit func(v int)
	visit = func(v int) {
		state[v] = 1
		for _, ei := range out[v] {
			e := g.edges[ei]
			switch state[e.to] {
			case 0:
				visit(e.to)
			case 1:
				if !e.attach {
					e.back = true
				}
			}
		}
		state[v] = 2
	}
	starts := append([]int{}, g.roots...)
	indeg := make([]int, g.n)
	for _, e := range g.edges {
		indeg[e.to]++
	}
	for i := 0; i < g.n; i++ {
		if indeg[i] == 0 {
			starts = append(starts, i)
		}
	}
	for i := 0; i < g.n; i++ {
		starts = append(starts, i)
	}
	for _, s := range starts {
		if state[s] == 0 {
			if a, ok := attached[s]; ok && a.host != s && g.edges[a.edge].from == s {
				continue // a note or input waits for its host
			}
			visit(s)
		}
	}
}

// assignLayers gives each node its row.
func (l *layout) assignLayers(attached map[int]attachment) bool {
	g := l.g
	type arc struct{ to, len int }
	out := make([][]arc, g.n)
	in := make([]int, g.n)
	side := map[int]bool{} // notes and inputs, placed with their host
	for _, a := range g.att {
		if g.edges[a.edge].from == a.node {
			side[a.node] = true
		}
	}
	for _, e := range g.edges {
		if e.back || side[e.from] {
			continue
		}
		ln := 1
		if e.attach {
			ln = 0
		}
		out[e.from] = append(out[e.from], arc{e.to, ln})
		in[e.to]++
	}
	var topo []int
	queue := []int{}
	for i := 0; i < g.n; i++ {
		if in[i] == 0 && !side[i] {
			queue = append(queue, i)
		}
	}
	layer := make([]int, g.n)
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		topo = append(topo, v)
		for _, a := range out[v] {
			layer[a.to] = max(layer[a.to], layer[v]+a.len)
			in[a.to]--
			if in[a.to] == 0 {
				queue = append(queue, a.to)
			}
		}
	}
	if len(topo)+len(side) < g.n {
		return false
	}
	if g.late {
		for i := len(topo) - 1; i >= 0; i-- {
			v := topo[i]
			if len(out[v]) == 0 {
				continue
			}
			lo := math.MaxInt
			for _, a := range out[v] {
				lo = min(lo, layer[a.to]-a.len)
			}
			if lo > layer[v] {
				layer[v] = lo
			}
		}
	}
	for i := range side {
		layer[i] = layer[attached[i].host]
	}
	for i, n := range l.nodes {
		n.layer = layer[i]
	}
	return true
}

// attach joins attachments to their hosts, each on a free side.
func (l *layout) attach(p *core.Problems) bool {
	g := l.g
	for _, a := range g.att {
		n, h := l.nodes[a.node], l.nodes[a.host]
		if h.host != nil {
			p.Add("", "%s sits beside %s, so nothing more can sit beside it; link it into the flow instead", g.name(a.host), g.name(h.host.idx))
			return false
		}
		sides := []int{1, -1}
		if a.side < 0 {
			sides = []int{-1, 1}
		}
		placed := false
		for _, s := range sides {
			if (s < 0 && h.left == nil) || (s > 0 && h.right == nil) {
				n.host, n.side = h, s
				gap := attachGap
				if lb := g.edges[a.edge].label; lb != "" {
					gap = max(gap, wrapBlock(lb, labelSize, 90, true).w+16)
				}
				if s < 0 {
					h.left, h.gapL = n, gap
				} else {
					h.right, h.gapR = n, gap
				}
				placed = true
				break
			}
		}
		if !placed {
			p.Add("", "%s already has something on both sides; at most two notes, inputs or short branches fit beside one box", g.name(a.host))
			return false
		}
		n.layer = h.layer
	}
	return true
}

// buildLayers makes the rows, with a placeholder wherever an edge passes
// through a row.
func (l *layout) buildLayers() {
	nl := 0
	for _, n := range l.nodes {
		nl = max(nl, n.layer+1)
	}
	l.layers = make([][]*gnode, nl)
	for _, n := range l.nodes {
		l.all = append(l.all, n)
		if n.host == nil {
			l.layers[n.layer] = append(l.layers[n.layer], n)
		}
	}
	for _, e := range l.g.edges {
		a, b := l.nodes[e.from], l.nodes[e.to]
		e.chain = []*gnode{a}
		if !e.back && !e.attach {
			for k := a.layer + 1; k < b.layer; k++ {
				d := &gnode{idx: -1, layer: k, w: dummyW}
				l.layers[k] = append(l.layers[k], d)
				l.all = append(l.all, d)
				e.chain = append(e.chain, d)
			}
		}
		e.chain = append(e.chain, b)
	}
}

// conn is one end of a line between adjacent rows, seen from an item.
type conn struct {
	other         *gnode // the item in the other row
	myOff, othOff float64
	wt            float64
	dir           int // -1: other is above
}

func (l *layout) conns() map[*gnode][]conn {
	m := map[*gnode][]conn{}
	for _, e := range l.g.edges {
		if e.back || e.attach {
			continue
		}
		for k := 0; k+1 < len(e.chain); k++ {
			a, b := e.chain[k], e.chain[k+1]
			wt := 1.0
			if a.dummy() && b.dummy() {
				wt = 8
			} else if a.dummy() || b.dummy() {
				wt = 2
			}
			ai, bi := a.item(), b.item()
			m[ai] = append(m[ai], conn{bi, a.offset(), b.offset(), wt, 1})
			m[bi] = append(m[bi], conn{ai, b.offset(), a.offset(), wt, -1})
		}
	}
	return m
}

// order arranges each row to cross as few lines as it can.
func (l *layout) order() {
	// Start from the order the walk from the roots meets things in.
	seen := map[*gnode]int{}
	t := 0
	var visit func(n *gnode)
	out := map[*gnode][]*gedge{}
	for _, e := range l.g.edges {
		if !e.back {
			out[e.chain[0]] = append(out[e.chain[0]], e)
		}
	}
	visit = func(n *gnode) {
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = t
		t++
		for _, e := range out[n] {
			for _, d := range e.chain[1 : len(e.chain)-1] {
				if _, ok := seen[d]; !ok {
					seen[d] = t
					t++
				}
			}
			visit(e.chain[len(e.chain)-1])
		}
	}
	for _, r := range l.g.roots {
		visit(l.nodes[r])
	}
	for _, n := range l.all {
		visit(n)
	}
	for _, row := range l.layers {
		sort.SliceStable(row, func(i, j int) bool { return seen[row[i]] < seen[row[j]] })
		renumber(row)
	}
	cs := l.conns()
	best := l.snapshot()
	bestX := l.crossings(cs)
	for it := 0; it < 24 && bestX > 0; it++ {
		if it%2 == 0 {
			for k := 1; k < len(l.layers); k++ {
				l.sortRow(l.layers[k], cs, -1)
			}
		} else {
			for k := len(l.layers) - 2; k >= 0; k-- {
				l.sortRow(l.layers[k], cs, 1)
			}
		}
		if x := l.crossings(cs); x < bestX {
			bestX, best = x, l.snapshot()
		}
	}
	l.restore(best)
}

func renumber(row []*gnode) {
	for i, n := range row {
		n.pos = i
	}
}

func (l *layout) snapshot() [][]*gnode {
	s := make([][]*gnode, len(l.layers))
	for i, r := range l.layers {
		s[i] = append([]*gnode{}, r...)
	}
	return s
}

func (l *layout) restore(s [][]*gnode) {
	l.layers = s
	for _, r := range l.layers {
		renumber(r)
	}
}

// frac is where an attachment sits in a row's order, near its host.
func frac(off float64) float64 {
	switch {
	case off < 0:
		return -0.35
	case off > 0:
		return 0.35
	}
	return 0
}

func (l *layout) sortRow(row []*gnode, cs map[*gnode][]conn, dir int) {
	bary := map[*gnode]float64{}
	for _, n := range row {
		sum, wt := 0.0, 0.0
		for _, c := range cs[n] {
			if c.dir == dir {
				sum += float64(c.other.pos) + frac(c.othOff) - frac(c.myOff)
				wt++
			}
		}
		if wt == 0 {
			bary[n] = float64(n.pos)
		} else {
			bary[n] = sum / wt
		}
	}
	sort.SliceStable(row, func(i, j int) bool { return bary[row[i]] < bary[row[j]] })
	renumber(row)
}

// crossings counts the lines that cross between adjacent rows.
func (l *layout) crossings(cs map[*gnode][]conn) int {
	total := 0
	for _, row := range l.layers {
		var segs [][2]float64
		for _, n := range row {
			for _, c := range cs[n] {
				if c.dir == 1 {
					segs = append(segs, [2]float64{float64(n.pos) + frac(c.myOff), float64(c.other.pos) + frac(c.othOff)})
				}
			}
		}
		for i := range segs {
			for j := i + 1; j < len(segs); j++ {
				a, b := segs[i], segs[j]
				if (a[0]-b[0])*(a[1]-b[1]) < 0 {
					total++
				}
			}
		}
	}
	return total
}

// sizeRows sizes every node for the room its row leaves.
func (l *layout) sizeRows(p *core.Problems) bool {
	g := l.g
	for k, row := range l.layers {
		var real []*gnode
		seps := 0.0
		for i, n := range row {
			if !n.dummy() {
				real = append(real, n)
				if n.left != nil {
					real = append(real, n.left)
					seps += n.gapL
				}
				if n.right != nil {
					real = append(real, n.right)
					seps += n.gapR
				}
			} else {
				seps += dummyW
			}
			if i > 0 {
				seps += sepGap(row[i-1], n)
			}
		}
		if len(real) == 0 {
			continue
		}
		lr, rr := l.laneRes(k)
		room := avail - lr - rr - seps
		// Narrow nodes (notes) take their own size first.
		var wide []*gnode
		for _, n := range real {
			if g.narrow != nil && g.narrow(n.idx) {
				n.w, n.h = g.size(n.idx, min(g.maxW, room/float64(len(real))))
				room -= n.w
			} else {
				wide = append(wide, n)
			}
		}
		limit := g.maxW
		if len(wide) > 0 {
			limit = min(g.maxW, room/float64(len(wide)))
		}
		if limit < g.minW {
			var names []string
			for _, n := range real {
				names = append(names, g.name(n.idx))
			}
			p.Add("", "row %d of the %s would hold %d boxes side by side (%s), more than fit across a phone; link them in sequence instead of in parallel, or put fewer branches at that point",
				k+1, g.what, len(real), strings.Join(names, ", "))
			return false
		}
		// Size each, then give nodes of one class the same size. A node
		// whose words would not stay whole in an equal share is given
		// the width it needs, when the rest of the row still fits.
		if !l.sizeWide(wide, limit, room) {
			l.sizeEqual(wide, limit)
		}
		l.sameByClass(real)
	}
	return true
}

// sizeEqual sizes every node of a row's wide ones for the same limit.
func (l *layout) sizeEqual(wide []*gnode, limit float64) {
	for _, n := range wide {
		n.w, n.h = l.g.size(n.idx, limit)
	}
}

// sameByClass gives the nodes of one class in a row one size, the largest.
func (l *layout) sameByClass(real []*gnode) {
	g := l.g
	wByClass, hByClass := map[int]float64{}, map[int]float64{}
	for _, n := range real {
		c := g.class(n.idx)
		wByClass[c] = max(wByClass[c], n.w)
		hByClass[c] = max(hByClass[c], n.h)
	}
	for _, n := range real {
		c := g.class(n.idx)
		n.w, n.h = wByClass[c], hByClass[c]
	}
}

// sizeWide sizes a row's wide nodes giving each whose words need more than
// an equal share that width, and the others what is left between them. It
// reports false, sizing nothing, when no node needs more, or when the row
// would no longer fit (the others too narrow, or a class made as wide as
// its widest member running past room): then every node has an equal
// share and its text is made smaller or broken by the usual rules.
func (l *layout) sizeWide(wide []*gnode, limit, room float64) bool {
	g := l.g
	if g.whole == nil || len(wide) < 2 {
		return false
	}
	need := map[*gnode]float64{}
	grown, rest := 0.0, 0
	for _, n := range wide {
		if w := min(g.maxW, g.whole(n.idx)); w > limit {
			need[n] = w
			grown += w
		} else {
			rest++
		}
	}
	if len(need) == 0 {
		return false
	}
	share := g.maxW
	if rest > 0 {
		share = min(g.maxW, (room-grown)/float64(rest))
	}
	if share < g.minW {
		return false
	}
	for _, n := range wide {
		if w, ok := need[n]; ok {
			n.w, n.h = g.size(n.idx, w)
		} else {
			if g.whole(n.idx) > share {
				return false
			}
			n.w, n.h = g.size(n.idx, share)
		}
	}
	// What the row takes once a class is as wide as its widest member.
	byClass := map[int]float64{}
	for _, n := range wide {
		byClass[g.class(n.idx)] = max(byClass[g.class(n.idx)], n.w)
	}
	total := 0.0
	for _, n := range wide {
		total += byClass[g.class(n.idx)]
	}
	return total <= room+0.5
}

func sepGap(a, b *gnode) float64 {
	switch {
	case a.dummy() && b.dummy():
		return gapDummies
	case a.dummy() || b.dummy():
		return gapDummy
	}
	return gapNodes
}

// laneRes is the room row k leaves at its left and right for the loops
// that pass it.
func (l *layout) laneRes(k int) (left, right float64) {
	for _, e := range l.g.edges {
		if !e.back {
			continue
		}
		a, b := l.nodes[e.to].layer, l.nodes[e.from].layer
		if k < a || k > b {
			continue
		}
		if e.side < 0 {
			left += laneSep
		} else {
			right += laneSep
		}
	}
	if left > 0 {
		left += 8
	}
	if right > 0 {
		right += 8
	}
	return
}

// placeX places every row left to right.
func (l *layout) placeX() {
	// Loops go right unless both their ends are at the left.
	for _, e := range l.g.edges {
		if e.back {
			e.side = 1
		}
	}
	l.solveX()
	for _, e := range l.g.edges {
		if !e.back {
			continue
		}
		s, t := l.nodes[e.from], l.nodes[e.to]
		switch {
		case l.extreme(s, 1) && l.extreme(t, 1):
		case l.extreme(s, -1) && l.extreme(t, -1):
			e.side = -1
		case (s.absX()+t.absX())/2 < core.Width/2-20:
			e.side = -1
		}
	}
	l.solveX()
	l.centre()
}

// absX is a node's centre, an attachment's from its host.
func (n *gnode) absX() float64 {
	if n.host != nil {
		return n.host.x + n.offset()
	}
	return n.x
}

func (l *layout) solveX() {
	cs := l.conns()
	bounds := func(k int) (float64, float64) {
		a, b := l.laneRes(k)
		return margin + a, core.Width - margin - b
	}
	for k, row := range l.layers {
		lo, hi := bounds(k)
		w := 0.0
		for i, n := range row {
			if i > 0 {
				w += sepGap(row[i-1], n)
			}
			w += n.leftExt() + n.rightExt()
		}
		x := (lo+hi)/2 - w/2
		for i, n := range row {
			if i > 0 {
				x += sepGap(row[i-1], n)
			}
			n.x = x + n.leftExt()
			x += n.leftExt() + n.rightExt()
		}
	}
	desired := func(n *gnode, dir int) (float64, float64) {
		sum, wt := 0.0, 0.0
		for _, c := range cs[n] {
			if dir == 0 || c.dir == dir {
				sum += c.wt * (c.other.x + c.othOff - c.myOff)
				wt += c.wt
			}
		}
		if wt == 0 {
			return n.x, 0.2
		}
		return sum / wt, wt
	}
	pass := func(k int, dir int, exact bool) {
		row := l.layers[k]
		lo, hi := bounds(k)
		d := make([]float64, len(row))
		wts := make([]float64, len(row))
		seps := make([]float64, max(0, len(row)-1))
		for i, n := range row {
			d[i], wts[i] = desired(n, dir)
			if wts[i] == 0.2 && dir != 0 {
				d[i], wts[i] = desired(n, -dir)
				wts[i] = min(wts[i], 0.5)
			}
			if exact {
				// A node with one neighbour that has only it lines up exactly.
				d[i], wts[i] = n.x, 0.05
				var one []conn
				for _, c := range cs[n] {
					if c.dir == dir {
						one = append(one, c)
					}
				}
				if len(one) == 1 {
					back := 0
					for _, c := range cs[one[0].other] {
						if c.dir == -dir {
							back++
						}
					}
					if back == 1 {
						d[i], wts[i] = one[0].other.x+one[0].othOff-one[0].myOff, 10
					}
				}
			}
			if i > 0 {
				seps[i-1] = row[i-1].rightExt() + sepGap(row[i-1], n) + n.leftExt()
			}
		}
		if len(row) == 0 {
			return
		}
		xs := pava(d, wts, seps, lo+row[0].leftExt(), hi-row[len(row)-1].rightExt())
		for i, n := range row {
			n.x = xs[i]
		}
	}
	for it := 0; it < 14; it++ {
		if it%2 == 0 {
			for k := 1; k < len(l.layers); k++ {
				pass(k, -1, false)
			}
		} else {
			for k := len(l.layers) - 2; k >= 0; k-- {
				pass(k, 1, false)
			}
		}
	}
	for k := range l.layers {
		pass(k, 0, false)
	}
	for k := 1; k < len(l.layers); k++ {
		pass(k, -1, true)
	}
	for k := len(l.layers) - 2; k >= 0; k-- {
		pass(k, 1, true)
	}
	for _, n := range l.all {
		n.x = math.Round(n.x*2) / 2
	}
}

// pava places items as near their desired centres d (weights wt) as the
// spacing seps between neighbours and the bounds allow: the least-squares
// isotonic regression of d less the cumulative spacing.
func pava(d, wt, seps []float64, lo, hi float64) []float64 {
	n := len(d)
	off := make([]float64, n)
	for i := 1; i < n; i++ {
		off[i] = off[i-1] + seps[i-1]
	}
	type block struct {
		sum, wt float64
		n       int
	}
	var st []block
	for i := 0; i < n; i++ {
		st = append(st, block{wt[i] * (d[i] - off[i]), wt[i], 1})
		for len(st) > 1 {
			a, b := st[len(st)-2], st[len(st)-1]
			if a.sum/a.wt <= b.sum/b.wt {
				break
			}
			st = st[:len(st)-2]
			st = append(st, block{a.sum + b.sum, a.wt + b.wt, a.n + b.n})
		}
	}
	ylo, yhi := lo, hi-off[n-1]
	if ylo > yhi {
		ylo = (ylo + yhi) / 2
		yhi = ylo
	}
	out := make([]float64, 0, n)
	for _, b := range st {
		y := math.Min(yhi, math.Max(ylo, b.sum/b.wt))
		for j := 0; j < b.n; j++ {
			out = append(out, y)
		}
	}
	for i := range out {
		out[i] += off[i]
	}
	return out
}

// centre moves the drawing to the middle of the width.
func (l *layout) centre() {
	lo, hi := math.Inf(1), math.Inf(-1)
	for k, row := range l.layers {
		a, b := l.laneRes(k)
		for _, n := range row {
			lo = math.Min(lo, n.x-n.leftExt()-a)
			hi = math.Max(hi, n.x+n.rightExt()+b)
		}
	}
	shift := math.Round((core.Width/2-(lo+hi)/2)*2) / 2
	for _, row := range l.layers {
		for _, n := range row {
			n.x += shift
		}
	}
}

// extreme says whether nothing in n's row lies beyond it on side s.
func (l *layout) extreme(n *gnode, s int) bool {
	if n.host != nil {
		if n.side != s {
			return false
		}
		n = n.host
	} else if (s < 0 && n.left != nil) || (s > 0 && n.right != nil) {
		return false
	}
	row := l.layers[n.layer]
	for _, o := range row {
		if o == n || o.dummy() {
			continue
		}
		if (s < 0 && o.pos < n.pos) || (s > 0 && o.pos > n.pos) {
			return false
		}
	}
	return true
}

// seg is a line's run across the gap below row ch, on a track.
type seg struct {
	ch     int
	x1, x2 float64
	net    string
	// ups and downs are where lines leave the run upwards and downwards,
	// to order the tracks so they cross as little as they can.
	ups, downs []float64
	target     *gnode // a run into a node's top: runs into one node may share a track
	track      int
	y          float64
}

type channel struct {
	segs     []*seg
	tracks   int
	topRes   float64
	labelGap float64
	top, h   float64
}

func (l *layout) channel(c int) *channel {
	if l.chans == nil {
		l.chans = map[int]*channel{}
	}
	ch, ok := l.chans[c]
	if !ok {
		ch = &channel{}
		l.chans[c] = ch
	}
	return ch
}

func (l *layout) addSeg(c int, from, to float64, net string, target *gnode, up, down []float64) *seg {
	s := &seg{ch: c, x1: math.Min(from, to), x2: math.Max(from, to), net: net, ups: up, downs: down, target: target}
	ch := l.channel(c)
	// Runs from one exit share a track: they are one fan.
	for _, o := range ch.segs {
		if net != "" && o.net == net {
			o.x1, o.x2 = math.Min(o.x1, s.x1), math.Max(o.x2, s.x2)
			o.ups = append(o.ups, up...)
			o.downs = append(o.downs, down...)
			return o
		}
	}
	ch.segs = append(ch.segs, s)
	return s
}

// route draws every line, then places the rows and the labels.
func (l *layout) route() {
	g := l.g
	// Lanes for loops, innermost first.
	type lane struct {
		e      *gedge
		lo, hi int
	}
	var backs []lane
	for _, e := range g.edges {
		if e.back {
			a, b := l.nodes[e.to].layer, l.nodes[e.from].layer
			backs = append(backs, lane{e, a, b})
		}
	}
	sort.SliceStable(backs, func(i, j int) bool { return backs[i].hi-backs[i].lo < backs[j].hi-backs[j].lo })
	laneX := map[*gedge]float64{}
	var placed []lane
	for _, b := range backs {
		s := b.e.side
		ext := 0.0
		if s < 0 {
			ext = core.Width
		}
		for k := b.lo; k <= b.hi; k++ {
			for _, n := range l.layers[k] {
				if s > 0 {
					ext = math.Max(ext, n.x+n.rightExt())
				} else {
					ext = math.Min(ext, n.x-n.leftExt())
				}
			}
		}
		x := ext + float64(s)*16
		for _, o := range placed {
			if o.e.side == s && o.lo <= b.hi && b.lo <= o.hi {
				if s > 0 {
					x = math.Max(x, laneX[o.e]+laneSep)
				} else {
					x = math.Min(x, laneX[o.e]-laneSep)
				}
			}
		}
		laneX[b.e] = x
		placed = append(placed, b)
	}
	// Exits: a node's branches leave from its bottom, except one whose
	// target lies well to a side, which leaves from that side.
	outs := map[*gnode][]*gedge{}
	for _, e := range g.edges {
		if !e.back && !e.attach {
			outs[e.chain[0]] = append(outs[e.chain[0]], e)
		}
	}
	for _, e := range g.edges {
		if e.attach {
			a, h := l.nodes[e.from], l.nodes[e.to]
			if a.host == nil {
				a, h = h, a
			}
			// a is the attachment; its host's port on that side is taken.
			h.sideUsed[side01(a.side)] = true
			a.sideUsed[side01(-a.side)] = true
		}
	}
	for n, es := range outs {
		if len(es) < 2 {
			continue
		}
		x := n.absX()
		for _, s := range []int{-1, 1} {
			if n.sideUsed[side01(s)] {
				continue
			}
			var far *gedge
			farD := n.w/2 + 6
			for _, e := range es {
				d := float64(s) * (e.chain[1].absX() - x)
				if d > farD && e.exit == 0 {
					far, farD = e, d
				}
			}
			if far != nil {
				far.exit = s
				n.sideUsed[side01(s)] = true
			}
		}
		// Keep one branch at the bottom if all went to the sides.
		bottom := false
		for _, e := range es {
			if e.exit == 0 {
				bottom = true
			}
		}
		if !bottom && len(es) == 2 {
			e := es[0]
			if math.Abs(es[1].chain[1].absX()-x) < math.Abs(e.chain[1].absX()-x) {
				e = es[1]
			}
			n.sideUsed[side01(e.exit)] = false
			e.exit = 0
		}
	}
	// A lone line between two boxes that nearly line up runs straight,
	// leaving and entering a little off centre, rather than with a jog.
	ins := map[*gnode]int{}
	for _, e := range g.edges {
		if !e.attach {
			ins[e.chain[len(e.chain)-1]]++
		}
	}
	for n, es := range outs {
		if len(es) != 1 || es[0].exit != 0 || len(es[0].chain) != 2 {
			continue
		}
		v := es[0].chain[1]
		if ins[v] != 1 {
			continue
		}
		nx, vx := n.absX(), v.absX()
		lo := math.Max(nx-n.w/2, vx-v.w/2) + 18
		hi := math.Min(nx+n.w/2, vx+v.w/2) - 18
		if lo > hi || math.Abs(nx-vx) < 0.5 {
			continue
		}
		x := vx
		if n.w < v.w {
			x = nx
		}
		es[0].straight = math.Min(hi, math.Max(lo, x))
	}
	// A node's only line that heads off to one side (round the other
	// boxes of the next row, to a row further down) leaves from the end
	// of its bottom on that side, not its middle: from the middle its
	// first run across would pass under the box and meet the lines that
	// leave the boxes beside it.
	for n, es := range outs {
		if len(es) != 1 || es[0].exit != 0 || es[0].straight != 0 || len(es[0].chain) < 3 {
			continue
		}
		next := es[0].chain[1]
		if !next.dummy() {
			continue
		}
		nx, x := n.absX(), next.absX()
		lo, hi := nx-n.w/2+18, nx+n.w/2-18
		if x > lo && x < hi {
			continue
		}
		es[0].depart = math.Min(hi, math.Max(lo, x))
	}
	for _, e := range g.edges {
		switch {
		case e.attach:
			l.routeAttach(e)
		case e.back:
			l.routeBack(e, laneX[e])
		default:
			l.routeForward(e)
		}
	}
	l.placeY()
	for _, e := range g.edges {
		pts := make([]core.Pt, len(e.refs))
		for i, r := range e.refs {
			y := 0.0
			if r.s != nil {
				y = r.s.y
			} else {
				y = r.n.y + r.frac*r.n.h
			}
			pts[i] = core.Pt{X: r.x, Y: y}
		}
		e.route = simplify(pts)
	}
	l.placeLabels()
}

func side01(s int) int {
	if s < 0 {
		return 0
	}
	return 1
}

func (l *layout) routeAttach(e *gedge) {
	a, b := e.chain[0], e.chain[len(e.chain)-1]
	// the line runs between the facing sides at the host's middle.
	s := 1
	if b.absX() < a.absX() {
		s = -1
	}
	e.refs = []rpt{{x: a.absX() + float64(s)*a.w/2, n: a, frac: 0.5}, {x: b.absX() - float64(s)*b.w/2, n: a, frac: 0.5}}
	e.exit = s
}

func (l *layout) routeForward(e *gedge) {
	u := e.chain[0]
	v := e.chain[len(e.chain)-1]
	cols := make([]float64, len(e.chain))
	for k, n := range e.chain {
		cols[k] = n.absX()
	}
	var refs []rpt
	if e.straight != 0 {
		e.refs = []rpt{{x: e.straight, n: u, frac: 1}, {x: e.straight, n: v, frac: 0}}
		return
	}
	cur := cols[0]
	start := 0
	net := fmt.Sprintf("out:%p", u)
	if e.exit != 0 {
		s := float64(e.exit)
		vx := u.absX() + s*u.w/2
		refs = append(refs, rpt{x: vx, n: u, frac: 0.5})
		if l.clearAcross(u, vx, cols[1]) {
			refs = append(refs, rpt{x: cols[1], n: u, frac: 0.5})
			cur = cols[1]
		} else {
			cur = vx + s*12
			refs = append(refs, rpt{x: cur, n: u, frac: 0.5})
		}
		net = fmt.Sprintf("side:%p:%d", u, e.exit)
	} else {
		if e.depart != 0 {
			cur = e.depart
		}
		refs = append(refs, rpt{x: cur, n: u, frac: 1})
	}
	for k := start; k+1 < len(e.chain); k++ {
		nx := cols[k+1]
		if math.Abs(nx-cur) > 0.5 {
			n := ""
			if k == 0 {
				n = net
			}
			var tgt *gnode
			if k+2 == len(e.chain) {
				tgt = v
			}
			s := l.addSeg(e.chain[k].layer, cur, nx, n, tgt, []float64{cur}, []float64{nx})
			refs = append(refs, rpt{x: cur, s: s}, rpt{x: nx, s: s})
			cur = nx
		}
	}
	refs = append(refs, rpt{x: cur, n: v, frac: 0})
	e.refs = refs
}

// clearAcross says whether a line can run straight across from a side of
// u at x0 to x1 and drop there, clear of every other box in u's row.
func (l *layout) clearAcross(u *gnode, x0, x1 float64) bool {
	a, b := math.Min(x0, x1)-3, math.Max(x0, x1)+3
	for _, o := range l.rowNodes(u.layer) {
		if o == u {
			continue
		}
		bx := o.absX()
		if bx+o.w/2 > a && bx-o.w/2 < b {
			return false
		}
	}
	return true
}

// rowNodes is every real node in row k, attachments too.
func (l *layout) rowNodes(k int) []*gnode {
	var out []*gnode
	for _, n := range l.layers[k] {
		if n.dummy() {
			continue
		}
		out = append(out, n)
		if n.left != nil {
			out = append(out, n.left)
		}
		if n.right != nil {
			out = append(out, n.right)
		}
	}
	return out
}

func (l *layout) routeBack(e *gedge, lane float64) {
	s, t := l.nodes[e.from], l.nodes[e.to]
	sd := e.side
	i := side01(sd)
	var refs []rpt
	if l.extreme(s, sd) && !s.sideUsed[i] {
		refs = append(refs, rpt{x: s.absX() + float64(sd)*s.w/2, n: s, frac: 0.5}, rpt{x: lane, n: s, frac: 0.5})
		s.sideUsed[i] = true
		e.exit = sd
	} else {
		sg := l.addSeg(s.layer, s.absX(), lane, fmt.Sprintf("out:%p", s), nil, []float64{s.absX(), lane}, nil)
		refs = append(refs, rpt{x: s.absX(), n: s, frac: 1}, rpt{x: s.absX(), s: sg}, rpt{x: lane, s: sg})
	}
	if l.extreme(t, sd) && !t.sideUsed[i] {
		refs = append(refs, rpt{x: lane, n: t, frac: 0.5}, rpt{x: t.absX() + float64(sd)*t.w/2, n: t, frac: 0.5})
		t.sideUsed[i] = true
	} else {
		sg := l.addSeg(t.layer-1, lane, t.absX(), "", t, []float64{lane}, []float64{t.absX()})
		refs = append(refs, rpt{x: lane, s: sg}, rpt{x: t.absX(), s: sg}, rpt{x: t.absX(), n: t, frac: 0})
	}
	e.refs = refs
}

// placeY gives each channel its tracks and each row its height and place.
func (l *layout) placeY() {
	g := l.g
	// Channels where a label sits under a box need room at the top.
	lab := map[int]float64{}
	for _, e := range g.edges {
		if e.label == "" || e.attach {
			continue
		}
		h := wrapBlock(e.label, labelSize, 110, e.labelBold).h
		if g.labelMid {
			for k := 0; k+1 < len(e.chain); k++ {
				lab[e.chain[k].layer] = max(lab[e.chain[k].layer], h+14)
			}
		} else if e.exit == 0 {
			k := l.nodes[e.from].layer
			lab[k] = max(lab[k], h+7)
		}
	}
	for c, ch := range l.chans {
		ch.topRes = max(12, lab[c])
		l.assignTracks(ch)
	}
	bandH := make([]float64, len(l.layers))
	for k := range l.layers {
		for _, n := range l.rowNodes(k) {
			bandH[k] = max(bandH[k], n.h)
		}
	}
	y := 2.0
	if ch, ok := l.chans[-1]; ok && ch.tracks > 0 {
		ch.topRes = 8
		ch.top, ch.h = y, ch.topRes+float64(ch.tracks-1)*trackSep+18
		y += ch.h
		l.setTracks(ch)
	}
	for k := range l.layers {
		for _, n := range l.rowNodes(k) {
			n.y = y + (bandH[k]-n.h)/2
		}
		for _, n := range l.layers[k] {
			if n.dummy() {
				n.y, n.h = y, bandH[k]
			}
		}
		y += bandH[k]
		ch := l.chans[k]
		if ch == nil {
			ch = &channel{topRes: max(12, lab[k])}
		}
		ch.top = y
		if k == len(l.layers)-1 {
			if ch.tracks > 0 {
				ch.h = ch.topRes + float64(ch.tracks-1)*trackSep + 6
			}
		} else {
			ch.h = max(g.baseGap, lab[k]+16, ch.topRes+float64(max(0, ch.tracks-1))*trackSep+16)
		}
		l.setTracks(ch)
		y += ch.h
	}
	l.H = y + 2
}

func (l *layout) setTracks(ch *channel) {
	if ch.tracks == 0 {
		return
	}
	span := float64(ch.tracks-1) * trackSep
	lo := ch.top + ch.topRes
	hi := ch.top + ch.h - 16 - span
	start := math.Max(lo, (lo+hi)/2)
	if ch.top+ch.h-16-span < lo {
		start = lo
	}
	for _, s := range ch.segs {
		s.y = start + float64(s.track)*trackSep
	}
}

// assignTracks gives each run in a channel a track, runs that would
// overlap on different ones, ordered so their vertical legs cross as few
// other runs as they can.
func (l *layout) assignTracks(ch *channel) {
	segs := ch.segs
	in := func(x float64, s *seg) bool { return x > s.x1+0.5 && x < s.x2-0.5 }
	// cost of a above b.
	cost := func(a, b *seg) int {
		n := 0
		for _, x := range a.downs {
			if in(x, b) {
				n++
			}
		}
		for _, x := range b.ups {
			if in(x, a) {
				n++
			}
		}
		return n
	}
	conflict := func(a, b *seg) bool {
		if a.x1 > b.x2+0.5 || b.x1 > a.x2+0.5 {
			return false
		}
		// Runs into one node may share a track: they read as lines
		// merging on their way in.
		return !(a.target != nil && a.target == b.target)
	}
	score := make([]int, len(segs))
	for i, a := range segs {
		for j, b := range segs {
			if i != j && conflict(a, b) {
				score[i] += cost(a, b) - cost(b, a)
			}
		}
	}
	idx := make([]int, len(segs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		if score[idx[i]] != score[idx[j]] {
			return score[idx[i]] < score[idx[j]]
		}
		return segs[idx[i]].x1 < segs[idx[j]].x1
	})
	var done []*seg
	ch.tracks = 0
	for _, i := range idx {
		s := segs[i]
		t := 0
		for _, o := range done {
			if conflict(s, o) {
				t = max(t, o.track+1)
			}
		}
		s.track = t
		ch.tracks = max(ch.tracks, t+1)
		done = append(done, s)
	}
}

// placeLabels puts each line's label where it collides with nothing.
func (l *layout) placeLabels() {
	var boxes []box
	for _, n := range l.all {
		if !n.dummy() {
			boxes = append(boxes, n.bbox())
		}
	}
	var segs [][2]core.Pt
	for _, e := range l.g.edges {
		for i := 0; i+1 < len(e.route); i++ {
			segs = append(segs, [2]core.Pt{e.route[i], e.route[i+1]})
		}
	}
	for _, e := range l.g.edges {
		if e.label == "" || len(e.route) < 2 {
			continue
		}
		tb := wrapBlock(e.label, labelSize, 110, e.labelBold)
		w, h := tb.w+4, tb.h
		var cands []box
		add := func(x, y float64) { cands = append(cands, box{x, y, w, h}) }
		r := e.route
		if e.attach {
			a, b := r[0], r[len(r)-1]
			mx := (a.X + b.X) / 2
			add(mx-w/2, a.Y-h-2)
			add(mx-w/2, a.Y+2)
		} else if l.g.labelMid {
			// Beside the middle of the longest leg.
			best, bl := 0, -1.0
			for i := 0; i+1 < len(r); i++ {
				if d := dist(r[i], r[i+1]); d > bl {
					best, bl = i, d
				}
			}
			a, b := r[best], r[best+1]
			mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
			if math.Abs(a.X-b.X) < 0.5 {
				add(mx+5, my-h/2)
				add(mx-5-w, my-h/2)
			} else {
				add(mx-w/2, my-h-2)
				add(mx-w/2, my+2)
			}
			for i := 0; i+1 < len(r); i++ {
				a, b := r[i], r[i+1]
				if math.Abs(a.X-b.X) < 0.5 && i != best {
					my := (a.Y + b.Y) / 2
					add(a.X+5, my-h/2)
					add(a.X-5-w, my-h/2)
				}
			}
		} else {
			a, b := r[0], r[1]
			if math.Abs(a.X-b.X) < 0.5 { // leaves downwards
				// Branches that leave one point carry their labels on their
				// own legs, so each is read with its branch.
				shared, otherDir := false, 0.0
				for _, o := range l.g.edges {
					if o != e && !o.attach && len(o.route) > 1 && dist(o.route[0], a) < 0.5 {
						shared = true
						if len(o.route) > 2 {
							otherDir += o.route[2].X - o.route[1].X
						}
					}
				}
				switch {
				case shared && len(r) > 2:
					c := r[2]
					if c.X > b.X {
						add(b.X+8, b.Y-h-1)
						add(b.X+8, b.Y+2)
					} else {
						add(b.X-8-w, b.Y-h-1)
						add(b.X-8-w, b.Y+2)
					}
				case shared && otherDir > 0:
					add(a.X-5-w, a.Y+3)
					add(a.X+5, a.Y+3)
				default:
					add(a.X+5, a.Y+3)
					add(a.X-5-w, a.Y+3)
				}
				if !shared && len(r) > 2 {
					c := r[2]
					if c.X > b.X {
						add(b.X+8, b.Y-h-1)
					} else {
						add(b.X-8-w, b.Y-h-1)
					}
				}
			} else { // leaves sideways
				if b.X > a.X {
					add(a.X+5, a.Y-h-2)
					add(a.X+5, a.Y+2)
				} else {
					add(a.X-5-w, a.Y-h-2)
					add(a.X-5-w, a.Y+2)
				}
				// When the run out of the side is short (a box close
				// beside it), the label is read against that box; beside
				// the line where it turns down, under its own box, it is
				// read with its branch.
				if len(r) > 2 && math.Abs(b.X-r[2].X) < 0.5 && r[2].Y > b.Y {
					u := e.chain[0]
					y := math.Max(b.Y, u.y+u.h) + 3
					if y+h < r[2].Y {
						if b.X > a.X {
							add(b.X-5-w, y)
						} else {
							add(b.X+5, y)
						}
					}
				}
			}
		}
		bestScore, best := math.MaxInt, cands[0]
		for _, cb := range cands {
			s := 0
			if cb.X < 2 || cb.R() > core.Width-2 || cb.Y < 0 || cb.B() > l.H {
				s += 1000
			}
			for _, b := range boxes {
				if cb.overlaps(b, 1) {
					s += 1000
				}
			}
			for _, sg := range segs {
				if segHitsBox(sg[0], sg[1], cb.grow(1)) {
					s++
				}
			}
			if s < bestScore {
				bestScore, best = s, cb
			}
		}
		lb := label{text: tb, at: best}
		e.lab = &lb
		boxes = append(boxes, best)
	}
}
