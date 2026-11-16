package chart

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

const (
	maxCategories = 24
	maxCatLabel   = 40
)

type barSeriesSpec struct {
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
	// Errors are symmetric half-widths: the bar shows value ± error.
	Errors []*float64 `json:"errors"`
	// Intervals are each bar's [low, high], for an interval that is not
	// symmetric about its value: a ratio's confidence interval (RR 1.53,
	// 0.97 to 2.42) is even on a log scale, not on the bar's. Drawing such
	// an interval as ± one half-width puts both ends in the wrong place.
	Intervals [][]float64 `json:"intervals"`
}

// valueLineSpec is a reference line on a value axis (bars, boxes).
type valueLineSpec struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
	Tone  string  `json:"tone"`
}

type barSpec struct {
	Categories    []string        `json:"categories"`
	Series        []barSeriesSpec `json:"series"`
	Orientation   string          `json:"orientation"`
	Stacked       bool            `json:"stacked"`
	ValueAxis     *AxisSpec       `json:"value_axis"`
	CategoryLabel string          `json:"category_label"`
	ValueLabels   bool            `json:"value_labels"`
	RefLines      []valueLineSpec `json:"reference_lines"`
}

var barChart = core.Kind{
	Name:    "bar_chart",
	Family:  "chart",
	Use:     "Values compared across named categories (groups, drugs, age bands), grouped or stacked, optionally with error bars; use horizontal for long category names, and line_chart when x is continuous.",
	Fields:  "categories: [names, each " + core.UpTo(maxCatLabel) + "] 1–24; series: [{label (required if >1; " + core.UpTo(maxSeriesLabel) + "), values: [one number per category, null = none], errors?: [± half-widths, one per category], or intervals?: [[low, high] as given (for an uneven interval such as a ratio's 95% CI), one per category]; null = none}] 1–8; orientation?: vertical|horizontal (default vertical); stacked?: bool (no errors or intervals when stacked); value_axis?: " + axisFields + " (linear, from zero); category_label? (" + core.UpTo(catTitleChars) + "); value_labels?: bool, writes each value at its bar; reference_lines?: [{value, label? (" + core.UpTo(maxOverlayLab) + "), tone?}] ≤6 on the value axis.",
	Example: `{"type":"bar_chart","title":"WHO fast-breathing thresholds by age","alt":"Bar chart of the respiratory rate at or above which breathing counts as fast: 60 breaths a minute under 2 months, 50 from 2 to 11 months, and 40 from 1 to 5 years.","caption":"WHO IMCI thresholds for fast breathing.","spec":{"categories":["Under 2 months","2–11 months","1–5 years"],"series":[{"label":"Fast breathing","values":[60,50,40]}],"value_axis":{"label":"Respiratory rate","unit":"breaths/min","min":0,"max":70},"value_labels":true}}`,
	Parse:   parseBar,
}

type barDrawing struct {
	cats   []string
	labels []string
	values [][]float64 // [series][category], NaN = none
	// bars are each bar's error bar as its two ends, [series][category],
	// whichever form the spec gave them in; nil for a series with none,
	// NaN ends for a bar with none.
	bars       [][]errorBar
	horizontal bool
	stacked    bool
	valueLabel bool
	val        *axis
	catTitle   string
	refs       []valueLineSpec
}

func parseBar(raw json.RawMessage) (core.Drawing, error) {
	var s barSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	d := &barDrawing{stacked: s.Stacked, valueLabel: s.ValueLabels}
	p.Count("categories", len(s.Categories), 1, maxCategories)
	for i, c := range s.Categories {
		d.cats = append(d.cats, p.Label(core.Path("categories", i), c, true, maxCatLabel))
	}
	p.OneOf("orientation", s.Orientation, true, "vertical", "horizontal")
	d.horizontal = s.Orientation == "horizontal"
	d.catTitle = p.Label("category_label", s.CategoryLabel, false, catTitleChars)
	p.Count("series", len(s.Series), 1, maxSeries)
	var all []float64
	seen := map[string]bool{}
	n := len(s.Categories)
	for i, se := range s.Series {
		path := core.Path("series", i)
		lab := p.Label(core.Path(path, "label"), se.Label, len(s.Series) > 1, maxSeriesLabel)
		if lab != "" && seen[lab] {
			p.Add(core.Path(path, "label"), "%q is used by two series; each needs its own", lab)
		}
		seen[lab] = true
		d.labels = append(d.labels, lab)
		if len(se.Values) != n {
			p.Add(core.Path(path, "values"), "has %d values but there are %d categories: give one per category, null where there is none", len(se.Values), n)
		}
		vals := make([]float64, n)
		for j := range vals {
			vals[j] = math.NaN()
			if j < len(se.Values) && se.Values[j] != nil {
				vals[j] = *se.Values[j]
				all = append(all, vals[j])
			}
		}
		d.values = append(d.values, vals)
		d.bars = append(d.bars, parseErrorBars(&p, path, se, vals, s.Stacked, &all))
	}
	if s.Stacked && !p.Any() {
		all = nil
		for j := 0; j < n; j++ {
			pos, neg := 0.0, 0.0
			for i := range d.values {
				if v := d.values[i][j]; !math.IsNaN(v) {
					if v >= 0 {
						pos += v
					} else {
						neg += v
					}
				}
			}
			all = append(all, pos, neg)
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
	d.val = resolveAxis(&p, "value_axis", s.ValueAxis, lo, hi, axisOpts{noLog: true, zero: true})
	if p.Any() {
		return nil, p.Err()
	}
	if !d.val.in(0) {
		p.Add("value_axis", "runs from %s to %s, but bars are measured from zero: include zero in the range", fmtValue(d.val.lo), fmtValue(d.val.hi))
	}
	for i, vals := range d.values {
		for j, v := range vals {
			if math.IsNaN(v) {
				continue
			}
			if !d.val.in(v) {
				d.val.checkIn(&p, core.Path("series", i, "values", j), "value_axis", v)
			}
		}
	}
	// An error bar's ends must lie on the axis too: one past it would be
	// drawn out of the plot, over the axis labels.
	for i, bars := range d.bars {
		field := "errors"
		if s.Series[i].Intervals != nil {
			field = "intervals"
		}
		for j, bar := range bars {
			if bar.given() && (!d.val.in(bar.low) || !d.val.in(bar.high)) {
				end := bar.high
				if !d.val.in(bar.low) {
					end = bar.low
				}
				d.val.checkIn(&p, core.Path("series", i, field, j), "value_axis", end)
			}
		}
	}
	for i, r := range d.refs {
		d.val.checkIn(&p, core.Path("reference_lines", i, "value"), "value_axis", r.Value)
	}
	// Vertical bars' names must fit under their bars.
	if !d.horizontal && !p.Any() {
		slot := (core.Width - 2*padX - 40) / float64(n)
		for i, c := range d.cats {
			lines := core.Wrap(c, tickSize, slot-4, false)
			if len(lines) > 3 || brokeWord(lines) {
				p.Add(core.Path("categories", i), "%q is too long to set under a vertical bar (%d categories leave %.0f units each): set orientation horizontal, or shorten the names", c, n, slot)
				break
			}
		}
	}
	if !p.Any() && d.horizontal {
		for i, c := range d.cats {
			if lines := core.Wrap(c, tickSize, 120, false); len(lines) > 2 || brokeWord(lines) {
				p.Add(core.Path("categories", i), "%q is too long for a category label: keep it to %s, in common words", c, core.UpTo(maxCatLabel))
			}
		}
	}
	return d, p.Err()
}

// brokeWord says whether Wrap had to break a word to fit.
func brokeWord(lines []string) bool {
	for _, l := range lines {
		if (strings.HasSuffix(l, "-") || strings.HasSuffix(l, core.ForcedBreak)) && len(lines) > 1 {
			return true
		}
	}
	return false
}

func (d *barDrawing) Draw(c *core.Canvas) error {
	cat := &axis{cats: d.cats, title: d.catTitle}
	var p *plot
	nCat := len(d.cats)
	nSer := len(d.values)
	if d.horizontal {
		p = newPlot(c, d.val, cat)
		p.xGrid = true
		per := 26.0
		if !d.stacked {
			per = math.Max(24, float64(nSer)*13+10)
		}
		p.height = math.Max(120, per*float64(nCat))
		for _, s := range d.cats {
			p.catLines = append(p.catLines, core.Wrap(s, tickSize, 120, false))
		}
		if d.valueLabel {
			p.rightPad = 30
		}
		// The category axis's title would sit over the labels; the value
		// axis's is under the plot.
		cat.title = ""
	} else {
		p = newPlot(c, cat, d.val)
		slot := (core.Width - 2*padX - 40) / float64(nCat)
		for _, s := range d.cats {
			p.catLines = append(p.catLines, core.Wrap(s, tickSize, slot-4, false))
		}
	}
	if nSer > 1 {
		for i, l := range d.labels {
			p.legend = append(p.legend, legendItem{label: l, kind: legBox, colour: c.T.SeriesColour(i), fill: c.T.SeriesColour(i)})
		}
	}
	p.layout(0)
	p.drawFrame(0)
	// The value axis's zero, the bars' base.
	if d.horizontal {
		x0 := d.val.px(0)
		c.Line(x0, p.r.y0, x0, p.r.y1, c.T.Edge, 1, "")
	} else if d.val.lo < 0 {
		y0 := d.val.px(0)
		c.Line(p.r.x0, y0, p.r.x1, y0, c.T.Edge, 1, "")
	}
	for j := 0; j < nCat; j++ {
		centre, width := cat.band(j)
		group := width * 0.72
		if nCat == 1 {
			group = math.Min(group, 120)
		}
		if nSer == 1 || d.stacked {
			group = math.Min(group, 64)
		}
		pos, neg := 0.0, 0.0
		for i := 0; i < nSer; i++ {
			v := d.values[i][j]
			if math.IsNaN(v) {
				continue
			}
			var from, to, bw, off float64
			if d.stacked {
				bw, off = group, -group/2
				if v >= 0 {
					from, to = pos, pos+v
					pos += v
				} else {
					from, to = neg, neg+v
					neg += v
				}
			} else {
				bw = group / float64(nSer)
				off = -group/2 + bw*float64(i)
				from, to = 0, v
			}
			gapW := 0.0
			if !d.stacked && nSer > 1 {
				gapW = math.Min(2, bw*0.12)
			}
			a, b := d.val.px(from), d.val.px(to)
			lo, hi := math.Min(a, b), math.Max(a, b)
			col := c.T.SeriesColour(i)
			var r rect
			if d.horizontal {
				r = rect{lo, centre + off + gapW/2, hi, centre + off + bw - gapW/2}
			} else {
				r = rect{centre + off + gapW/2, lo, centre + off + bw - gapW/2, hi}
			}
			stroke := ""
			if d.stacked {
				stroke = c.T.Bg
			}
			c.Rect(r.x0, r.y0, r.w(), r.h(), 0, col, stroke, 0.8)
			p.obs.rect(r)
			// Error bar.
			if bar := d.barAt(i, j); bar.drawn() {
				e0, e1 := d.val.px(bar.low), d.val.px(bar.high)
				cap := math.Min(5, bw*0.3)
				if d.horizontal {
					cy := (r.y0 + r.y1) / 2
					c.Line(e0, cy, e1, cy, c.T.Ink, 1.2, "")
					c.Line(e0, cy-cap, e0, cy+cap, c.T.Ink, 1.2, "")
					c.Line(e1, cy-cap, e1, cy+cap, c.T.Ink, 1.2, "")
				} else {
					cx := (r.x0 + r.x1) / 2
					c.Line(cx, e0, cx, e1, c.T.Ink, 1.2, "")
					c.Line(cx-cap, e0, cx+cap, e0, c.T.Ink, 1.2, "")
					c.Line(cx-cap, e1, cx+cap, e1, c.T.Ink, 1.2, "")
				}
			}
			if d.valueLabel {
				d.drawValueLabel(c, p, r, v, i, j)
			}
		}
	}
	for _, rl := range d.refs {
		col := p.refColour(rl.Tone)
		v := d.val.px(rl.Value)
		if d.horizontal {
			c.Line(v, p.r.y0, v, p.r.y1, col, 1.3, "5 4")
			if rl.Label != "" {
				c.Text(v+4, p.r.y0-0, rl.Label, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Halo: c.T.Bg})
			}
		} else {
			c.Line(p.r.x0, v, p.r.x1, v, col, 1.3, "5 4")
			if rl.Label != "" {
				x, a := p.r.x1-2, core.End
				r := textRect(x, v-8, rl.Label, tickSize, true, a)
				if p.obs.cost(r) > p.obs.cost(textRect(p.r.x0+2, v-8, rl.Label, tickSize, true, core.Start)) {
					x, a = p.r.x0+2, core.Start
				}
				c.Text(x, v-8, rl.Label, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Anchor: a, Halo: c.T.Bg})
			}
		}
	}
	c.H = p.bottom
	return nil
}

// drawValueLabel writes a bar's value: past its end for side-by-side bars,
// inside the segment for stacked ones (when it fits).
func (d *barDrawing) drawValueLabel(c *core.Canvas, p *plot, r rect, v float64, i, j int) {
	text := fmtValue(v)
	w := core.TextWidth(text, tickSize, true)
	if d.stacked {
		if (d.horizontal && r.w() < w+6) || (!d.horizontal && r.h() < tickSize+4) || r.w() < w+2 {
			return
		}
		c.Text((r.x0+r.x1)/2, (r.y0+r.y1)/2, text, core.TextStyle{Size: tickSize, Bold: true, Fill: c.T.Bg, Anchor: core.Middle})
		return
	}
	col := c.T.Ink
	// Past the error bar, when there is one.
	if bar := d.barAt(i, j); bar.given() {
		if v >= 0 {
			if d.horizontal {
				r.x1 = math.Max(r.x1, d.val.px(bar.high))
			} else {
				r.y0 = math.Min(r.y0, d.val.px(bar.high))
			}
		} else if d.horizontal {
			r.x0 = math.Min(r.x0, d.val.px(bar.low))
		} else {
			r.y1 = math.Max(r.y1, d.val.px(bar.low))
		}
	}
	if d.horizontal {
		if v >= 0 {
			c.Text(r.x1+4, (r.y0+r.y1)/2, text, core.TextStyle{Size: tickSize, Fill: col, Bold: true})
		} else {
			c.Text(r.x0-4, (r.y0+r.y1)/2, text, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Anchor: core.End})
		}
		return
	}
	if r.w() < w-6 {
		return
	}
	y := r.y0 - 8
	if v < 0 {
		y = r.y1 + 8
	}
	c.Text((r.x0+r.x1)/2, y, text, core.TextStyle{Size: tickSize, Fill: col, Bold: true, Anchor: core.Middle, Halo: c.T.Bg})
}
