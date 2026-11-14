package chart

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/ifedan-ed/figures-go/internal/core"
)

type boxGroupSpec struct {
	Label    string    `json:"label"`
	Values   []float64 `json:"values"`
	Min      *float64  `json:"min"`
	Q1       *float64  `json:"q1"`
	Median   *float64  `json:"median"`
	Q3       *float64  `json:"q3"`
	Max      *float64  `json:"max"`
	Outliers []float64 `json:"outliers"`
}

type boxSpec struct {
	Groups        []boxGroupSpec  `json:"groups"`
	Orientation   string          `json:"orientation"`
	ValueAxis     *AxisSpec       `json:"value_axis"`
	CategoryLabel string          `json:"category_label"`
	RefLines      []valueLineSpec `json:"reference_lines"`
}

var boxPlot = core.Kind{
	Name:    "box_plot",
	Family:  "chart",
	Use:     "The spread of a measure in several groups: median, quartiles, range and outliers, from five-number summaries or from raw values; for single values per group use bar_chart.",
	Fields:  "groups: [{label (" + core.UpTo(maxCatLabel) + "), then either values: [numbers] 5–500 (quartiles computed; whiskers to the furthest value within 1.5 × IQR, the rest outliers) or min, q1, median, q3, max with outliers?: [numbers beyond min and max]}] 1–24; orientation?: vertical|horizontal (default vertical); value_axis?: " + axisFields + "; category_label? (" + core.UpTo(catTitleChars) + "); reference_lines?: [{value, label? (" + core.UpTo(maxOverlayLab) + "), tone?}] ≤6.",
	Example: `{"type":"box_plot","title":"Length of stay in bronchiolitis by oxygen need","alt":"Box plots of hospital stay: children without oxygen stayed a median of 1.5 days, those on low-flow oxygen 3 days, and those on high-flow 5 days, with wider spread and longer outliers as support increased.","caption":"Illustrative data.","spec":{"value_axis":{"label":"Length of stay","unit":"days","min":0,"max":14},"groups":[{"label":"No oxygen","min":0.5,"q1":1,"median":1.5,"q3":2,"max":3.5,"outliers":[5]},{"label":"Low-flow","min":1,"q1":2,"median":3,"q3":4,"max":6.5,"outliers":[9]},{"label":"High-flow","min":2,"q1":3.5,"median":5,"q3":7,"max":11,"outliers":[13]}]}}`,
	Parse:   parseBox,
}

type boxStats struct {
	label                    string
	min, q1, median, q3, max float64
	outliers                 []float64
}

type boxDrawing struct {
	groups     []boxStats
	val        *axis
	horizontal bool
	catTitle   string
	refs       []valueLineSpec
}

// quantile is the p-quantile of sorted values, linearly interpolated
// between order statistics (R's and Excel's default, type 7).
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	h := (float64(n) - 1) * p
	lo := math.Floor(h)
	i := int(lo)
	if i+1 >= n {
		return sorted[n-1]
	}
	return sorted[i] + (h-lo)*(sorted[i+1]-sorted[i])
}

// summarise is a box from raw values: quartiles, whiskers at the furthest
// values within 1.5 IQR of the box (Tukey), and the outliers past them.
func summarise(label string, values []float64) boxStats {
	v := append([]float64{}, values...)
	sort.Float64s(v)
	b := boxStats{label: label, q1: quantile(v, 0.25), median: quantile(v, 0.5), q3: quantile(v, 0.75)}
	iqr := b.q3 - b.q1
	lo, hi := b.q1-1.5*iqr, b.q3+1.5*iqr
	b.min, b.max = math.Inf(1), math.Inf(-1)
	for _, x := range v {
		if x < lo || x > hi {
			b.outliers = append(b.outliers, x)
			continue
		}
		b.min = math.Min(b.min, x)
		b.max = math.Max(b.max, x)
	}
	return b
}

func parseBox(raw json.RawMessage) (core.Drawing, error) {
	var s boxSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	d := &boxDrawing{}
	p.Count("groups", len(s.Groups), 1, maxCategories)
	p.OneOf("orientation", s.Orientation, true, "vertical", "horizontal")
	d.horizontal = s.Orientation == "horizontal"
	d.catTitle = p.Label("category_label", s.CategoryLabel, false, catTitleChars)
	var all []float64
	for i, g := range s.Groups {
		path := core.Path("groups", i)
		label := p.Label(core.Path(path, "label"), g.Label, true, maxCatLabel)
		summary := g.Min != nil || g.Q1 != nil || g.Median != nil || g.Q3 != nil || g.Max != nil
		switch {
		case len(g.Values) > 0 && summary:
			p.Add(path, "gives both values and a summary: give either values (the summary is computed) or min, q1, median, q3 and max")
		case len(g.Values) > 0:
			p.Count(core.Path(path, "values"), len(g.Values), 5, maxPoints)
			if len(g.Outliers) > 0 {
				p.Add(core.Path(path, "outliers"), "are found from the values; leave outliers out when giving values")
			}
			b := summarise(label, g.Values)
			d.groups = append(d.groups, b)
			all = append(all, g.Values...)
		case summary:
			var missing []string
			for _, f := range []struct {
				n string
				v *float64
			}{{"min", g.Min}, {"q1", g.Q1}, {"median", g.Median}, {"q3", g.Q3}, {"max", g.Max}} {
				if f.v == nil {
					missing = append(missing, f.n)
				}
			}
			if len(missing) > 0 {
				p.Add(path, "is missing %s: a summary needs all of min, q1, median, q3 and max", fmtList(missing))
				continue
			}
			b := boxStats{label: label, min: *g.Min, q1: *g.Q1, median: *g.Median, q3: *g.Q3, max: *g.Max}
			if !(b.min <= b.q1 && b.q1 <= b.median && b.median <= b.q3 && b.q3 <= b.max) {
				p.Add(path, "must have min ≤ q1 ≤ median ≤ q3 ≤ max; it has %s, %s, %s, %s, %s", fmtValue(b.min), fmtValue(b.q1), fmtValue(b.median), fmtValue(b.q3), fmtValue(b.max))
			}
			for j, o := range g.Outliers {
				if o >= b.min && o <= b.max {
					p.Add(core.Path(path, "outliers", j), "%s is within min to max (%s to %s); an outlier lies beyond the whiskers", fmtValue(o), fmtValue(b.min), fmtValue(b.max))
				}
			}
			p.Count(core.Path(path, "outliers"), len(g.Outliers), 0, 50)
			b.outliers = g.Outliers
			d.groups = append(d.groups, b)
			all = append(all, b.min, b.max)
			all = append(all, g.Outliers...)
		default:
			p.Add(path, "needs values, or min, q1, median, q3 and max")
		}
	}
	p.Count("reference_lines", len(s.RefLines), 0, maxOverlay)
	for i := range s.RefLines {
		r := &s.RefLines[i]
		r.Label = p.Label(core.Path("reference_lines", i, "label"), r.Label, false, maxOverlayLab)
		p.Tone(core.Path("reference_lines", i, "tone"), r.Tone)
		all = append(all, r.Value)
	}
	d.refs = s.RefLines
	if p.Any() {
		return nil, p.Err()
	}
	lo, hi := extent(all)
	if s.ValueAxis != nil && s.ValueAxis.Scale == "log" {
		all = positiveOrAll(all)
		lo, hi = extent(all)
	}
	d.val = resolveAxis(&p, "value_axis", s.ValueAxis, lo, hi, axisOpts{})
	if p.Any() {
		return nil, p.Err()
	}
	for i, g := range d.groups {
		for _, v := range append([]float64{g.min, g.max}, g.outliers...) {
			if !d.val.in(v) || (d.val.log && v <= 0) {
				d.val.checkIn(&p, core.Path("groups", i), "value_axis", v)
				break
			}
		}
	}
	for i, r := range d.refs {
		d.val.checkIn(&p, core.Path("reference_lines", i, "value"), "value_axis", r.Value)
	}
	if !d.horizontal && !p.Any() {
		slot := (core.Width - 2*padX - 40) / float64(len(d.groups))
		for i, g := range d.groups {
			if lines := core.Wrap(g.label, tickSize, slot-4, false); len(lines) > 3 || brokeWord(lines) {
				p.Add(core.Path("groups", i, "label"), "%q is too long to set under a vertical box: set orientation horizontal, or shorten the labels", g.label)
				break
			}
		}
	}
	return d, p.Err()
}

func (d *boxDrawing) Draw(c *core.Canvas) error {
	var names []string
	for _, g := range d.groups {
		names = append(names, g.label)
	}
	cat := &axis{cats: names, title: d.catTitle}
	var p *plot
	if d.horizontal {
		p = newPlot(c, d.val, cat)
		p.xGrid = true
		p.height = math.Max(120, 30*float64(len(names)))
		for _, s := range names {
			p.catLines = append(p.catLines, core.Wrap(s, tickSize, 120, false))
		}
		cat.title = ""
	} else {
		p = newPlot(c, cat, d.val)
		slot := (core.Width - 2*padX - 40) / float64(len(names))
		for _, s := range names {
			p.catLines = append(p.catLines, core.Wrap(s, tickSize, slot-4, false))
		}
	}
	p.layout(0)
	p.drawFrame(0)
	for _, rl := range d.refs {
		col := p.refColour(rl.Tone)
		v := d.val.px(rl.Value)
		if d.horizontal {
			c.Line(v, p.r.y0, v, p.r.y1, col, 1.3, "5 4")
			if rl.Label != "" {
				c.Text(v+4, p.r.y0-1, rl.Label, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Halo: c.T.Bg})
			}
		} else {
			c.Line(p.r.x0, v, p.r.x1, v, col, 1.3, "5 4")
			if rl.Label != "" {
				c.Text(p.r.x1-2, v-8, rl.Label, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Anchor: core.End, Halo: c.T.Bg})
			}
		}
	}
	col := c.T.SeriesColour(0)
	soft := c.T.SeriesSoftColour(0)
	for i, g := range d.groups {
		centre, width := cat.band(i)
		bw := math.Min(width*0.5, 46)
		// at maps a (position across, value) pair to figure units.
		at := func(across, v float64) core.Pt {
			if d.horizontal {
				return core.Pt{X: d.val.px(v), Y: across}
			}
			return core.Pt{X: across, Y: d.val.px(v)}
		}
		seg := func(a, b core.Pt, w float64, colour string) { c.Line(a.X, a.Y, b.X, b.Y, colour, w, "") }
		// Whiskers and their caps.
		seg(at(centre, g.min), at(centre, g.q1), 1.4, c.T.Ink)
		seg(at(centre, g.q3), at(centre, g.max), 1.4, c.T.Ink)
		seg(at(centre-bw*0.25, g.min), at(centre+bw*0.25, g.min), 1.4, c.T.Ink)
		seg(at(centre-bw*0.25, g.max), at(centre+bw*0.25, g.max), 1.4, c.T.Ink)
		// The box, and the median across it.
		a, b := at(centre-bw/2, g.q1), at(centre+bw/2, g.q3)
		r := rect{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Max(a.X, b.X), math.Max(a.Y, b.Y)}
		c.Rect(r.x0, r.y0, r.w(), r.h(), 2, soft, col, 1.4)
		seg(at(centre-bw/2, g.median), at(centre+bw/2, g.median), 2.6, col)
		for _, o := range g.outliers {
			pt := at(centre, o)
			c.Circle(pt.X, pt.Y, 3, c.T.Bg, c.T.Ink, 1.3)
		}
	}
	c.H = p.bottom
	return nil
}
