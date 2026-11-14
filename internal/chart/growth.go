package chart

//go:generate go run ./refgen -out refdata.go

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// lms is one row of a growth reference: at age (months), the Box-Cox power
// L, median M and coefficient of variation S, from which any centile and
// any measurement's z-score follow (Cole's LMS method).
type lms struct{ age, l, m, s float64 }

// value is the measurement at z-score z.
func (r lms) value(z float64) float64 {
	if r.l == 0 {
		return r.m * math.Exp(r.s*z)
	}
	return r.m * math.Pow(1+r.l*r.s*z, 1/r.l)
}

// z is a measurement's z-score.
func (r lms) z(x float64) float64 {
	if r.l == 0 {
		return math.Log(x/r.m) / r.s
	}
	return (math.Pow(x/r.m, r.l) - 1) / (r.l * r.s)
}

// lmsAt is the reference at age, interpolated linearly between the
// published rows; ok is false outside them.
func lmsAt(rows []lms, age float64) (lms, bool) {
	if len(rows) == 0 || age < rows[0].age-1e-9 || age > rows[len(rows)-1].age+1e-9 {
		return lms{}, false
	}
	for i := 1; i < len(rows); i++ {
		a, b := rows[i-1], rows[i]
		if age <= b.age+1e-9 {
			t := (age - a.age) / (b.age - a.age)
			t = math.Max(0, math.Min(1, t))
			return lms{age, a.l + t*(b.l-a.l), a.m + t*(b.m-a.m), a.s + t*(b.s-a.s)}, true
		}
	}
	return rows[0], true
}

// normCDF is the standard normal distribution function.
func normCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// centileZ are the z-scores of the centiles the charts draw.
var centileZ = map[int]float64{
	3: -1.880794, 5: -1.644854, 10: -1.281552, 25: -0.674490, 50: 0,
	75: 0.674490, 90: 1.281552, 95: 1.644854, 97: 1.880794,
}

// growthRef is one reference chart: a measure from one organisation, for
// one age span.
type growthRef struct {
	org      string // WHO or CDC
	table    *[2][]lms
	minMo    float64
	maxMo    float64
	label    string // Weight
	unit     string // kg
	centiles []int
}

var (
	whoCentiles = []int{3, 10, 25, 50, 75, 90, 97}
	cdcCentiles = []int{5, 10, 25, 50, 75, 90, 95}
)

// growthRefs are the references for each measure: WHO for 0 to 24 months,
// CDC for 2 to 20 years, as the CDC recommends.
var growthRefs = map[string][]growthRef{
	"weight_for_age": {
		{"WHO", &whoWeight, 0, 24, "Weight", "kg", whoCentiles},
		{"CDC", &cdcWeight, 24, 240, "Weight", "kg", cdcCentiles},
	},
	"length_for_age":             {{"WHO", &whoLength, 0, 24, "Length", "cm", whoCentiles}},
	"head_circumference_for_age": {{"WHO", &whoHead, 0, 24, "Head circumference", "cm", whoCentiles}},
	"stature_for_age":            {{"CDC", &cdcStature, 24, 240, "Stature", "cm", cdcCentiles}},
	"bmi_for_age":                {{"CDC", &cdcBMI, 24, 240, "BMI", "kg/m²", cdcCentiles}},
}

var measures = []string{"weight_for_age", "length_for_age", "stature_for_age", "head_circumference_for_age", "bmi_for_age"}

type growthSpec struct {
	Sex      string       `json:"sex"`
	Measure  string       `json:"measure"`
	AgeUnit  string       `json:"age_unit"`
	Points   [][]*float64 `json:"points"`
	AgeRange []float64    `json:"age_range"`
}

var growthChart = core.Kind{
	Name:    "growth_chart",
	Family:  "clinical chart",
	Use:     "A child's measurements plotted on the WHO (0–24 months) or CDC (2–20 years) reference centiles, which the app draws from the published tables; the last point is labelled with its percentile and z-score. Give only the child's measurements, never centile values.",
	Fields:  "sex: female|male; measure: weight_for_age (kg; WHO 0–24 mo or CDC 2–20 y, not both on one chart) | length_for_age (cm, WHO 0–24 mo) | stature_for_age (cm, CDC 2–20 y) | head_circumference_for_age (cm, WHO 0–24 mo) | bmi_for_age (kg/m², CDC 2–20 y); age_unit: months|years (for points and age_range); points: [[age, value], ...] 1–30 in increasing age, joined in order; age_range?: [from, to] (default: from the points).",
	Example: `{"type":"growth_chart","title":"Weight-for-age: a girl crossing centiles downward","alt":"WHO weight-for-age chart for a girl, 0 to 24 months: her weight is near the 75th centile at 2 months, then gains little from 6 months, crossing down to the 6th percentile at 15 months (8.0 kg).","caption":"WHO Child Growth Standards.","spec":{"sex":"female","measure":"weight_for_age","age_unit":"months","points":[[2,5.6],[6,7.9],[9,8.2],[12,8.2],[15,8]]}}`,
	Parse:   parseGrowth,
}

type growthDrawing struct {
	ref      growthRef
	rows     []lms
	sex      string
	measure  string
	lo, hi   float64 // months
	years    bool    // the axis in years
	ages     []float64
	values   []float64
	lastZ    float64
	lastNote string
}

// monthsPer converts an age in unit to months.
func monthsPer(unit string) float64 {
	if unit == "years" {
		return 12
	}
	return 1
}

// fmtAge writes an age in months as a reader says it: "15 months",
// "2 years 6 months", "11 years".
func fmtAge(mo float64) string {
	if mo < 24 {
		return fmtValue(math.Round(mo*10)/10) + " months"
	}
	y := math.Floor(mo/12 + 1e-9)
	m := math.Round(mo - y*12)
	if m >= 12 {
		y, m = y+1, 0
	}
	if m == 0 {
		return fmt.Sprintf("%.0f years", y)
	}
	return fmt.Sprintf("%.0f years %.0f months", y, m)
}

func parseGrowth(raw json.RawMessage) (core.Drawing, error) {
	var s growthSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.OneOf("sex", s.Sex, false, "female", "male")
	p.OneOf("measure", s.Measure, false, measures...)
	p.OneOf("age_unit", s.AgeUnit, false, "months", "years")
	p.Count("points", len(s.Points), 1, 30)
	if p.Any() {
		return nil, p.Err()
	}
	per := monthsPer(s.AgeUnit)
	d := &growthDrawing{sex: s.Sex, measure: s.Measure}
	for i, pt := range s.Points {
		at := core.Path("points", i)
		if len(pt) != 2 || pt[0] == nil || pt[1] == nil {
			p.Add(at, "must be [age, value], two numbers")
			continue
		}
		age, v := *pt[0]*per, *pt[1]
		if age < 0 {
			p.Add(at, "age is negative")
		}
		if v <= 0 {
			p.Add(at, "value %s must be positive", fmtValue(v))
		}
		if i > 0 && len(d.ages) > 0 && age <= d.ages[len(d.ages)-1] {
			p.Add(at, "age %s %s is not after the point before it: give the points in increasing age", fmtValue(*pt[0]), s.AgeUnit)
		}
		d.ages = append(d.ages, age)
		d.values = append(d.values, v)
	}
	if p.Any() {
		return nil, p.Err()
	}
	refs := growthRefs[s.Measure]
	lo, hi := d.ages[0], d.ages[len(d.ages)-1]
	if len(s.AgeRange) > 0 {
		if len(s.AgeRange) != 2 || s.AgeRange[0] >= s.AgeRange[1] {
			p.Add("age_range", "must be [from, to] with from less than to, in %s", s.AgeUnit)
			return nil, p.Err()
		}
		lo, hi = math.Min(lo, s.AgeRange[0]*per), math.Max(hi, s.AgeRange[1]*per)
		if s.AgeRange[0]*per > d.ages[0] || s.AgeRange[1]*per < d.ages[len(d.ages)-1] {
			p.Add("age_range", "must include every point's age (%s to %s)", fmtAge(d.ages[0]), fmtAge(d.ages[len(d.ages)-1]))
		}
	}
	// Which reference: the one whose span holds every age.
	found := false
	for _, r := range refs {
		if lo >= r.minMo-1e-9 && hi <= r.maxMo+1e-9 {
			d.ref, found = r, true
			break
		}
	}
	if !found {
		var spans []string
		for _, r := range refs {
			spans = append(spans, fmt.Sprintf("%s %s to %s", r.org, fmtAge(r.minMo), fmtAge(r.maxMo)))
		}
		hint := "give only ages within it"
		switch {
		case len(refs) == 2:
			hint = "draw two charts, one for each reference, splitting the points at 24 months"
		case s.Measure == "length_for_age":
			hint = "from 2 years use stature_for_age (CDC)"
		case s.Measure == "stature_for_age":
			hint = "under 2 years use length_for_age (WHO)"
		case s.Measure == "bmi_for_age":
			hint = "BMI-for-age starts at 2 years; under 2, plot weight_for_age and length_for_age"
		case s.Measure == "head_circumference_for_age":
			hint = "head circumference is charted to 24 months"
		}
		where := "the points run"
		if len(s.AgeRange) > 0 {
			where = "the chart would run"
		}
		p.Add("points", "%s from %s to %s, but %s is charted on %s: %s", where, fmtAge(lo), fmtAge(hi), strings.ReplaceAll(s.Measure, "_", "-"), strings.Join(spans, " and "), hint)
		return nil, p.Err()
	}
	d.rows = d.ref.table[0]
	if s.Sex == "male" {
		d.rows = d.ref.table[1]
	}
	// Each point's z-score; an implausible one is a wrong unit or age.
	for i := range d.ages {
		r, _ := lmsAt(d.rows, d.ages[i])
		z := r.z(d.values[i])
		if math.Abs(z) > 5 || math.IsNaN(z) {
			p.Add(core.Path("points", i), "%s %s at %s is %.1f SD from the median for a %s: check the unit (%s is in %s) and that age_unit is right (%s)",
				fmtValue(d.values[i]), d.ref.unit, fmtAge(d.ages[i]), math.Abs(z), map[string]string{"female": "girl", "male": "boy"}[s.Sex],
				strings.ToLower(d.ref.label), d.ref.unit, s.AgeUnit)
		}
		if i == len(d.ages)-1 {
			d.lastZ = z
		}
	}
	if p.Any() {
		return nil, p.Err()
	}
	// The chart's span: given, or the points' with room around them.
	if len(s.AgeRange) == 0 {
		if d.ref.org == "WHO" {
			lo, hi = 0, 24
			if d.ages[len(d.ages)-1] <= 10 {
				hi = 12
			}
		} else {
			y0 := math.Max(2, math.Floor(d.ages[0]/12)-1)
			y1 := math.Min(20, math.Ceil(d.ages[len(d.ages)-1]/12)+2)
			for y1-y0 < 4 {
				if y0 > 2 {
					y0--
				} else {
					y1++
				}
			}
			// A span labelled every two years starts and ends on a label.
			if y1-y0 > 10 {
				y0 = math.Max(2, 2*math.Floor(y0/2))
				y1 = math.Min(20, 2*math.Ceil(y1/2))
			}
			lo, hi = y0*12, y1*12
		}
	}
	d.lo, d.hi = lo, hi
	d.years = d.ref.org == "CDC"
	d.lastNote = d.percentileNote()
	return d, nil
}

// percentileNote is the last point's percentile and z-score, as the label
// says it. For BMI at or above the 95th centile the CDC advises against
// LMS percentiles (they flatten at the extreme) and expresses BMI as a
// percentage of the 95th centile instead.
func (d *growthDrawing) percentileNote() string {
	z := d.lastZ
	age := d.ages[len(d.ages)-1]
	if d.measure == "bmi_for_age" {
		r, _ := lmsAt(d.rows, age)
		p95 := r.value(centileZ[95])
		if v := d.values[len(d.values)-1]; v >= p95 {
			return fmt.Sprintf("%.0f%% of the 95th percentile", math.Round(v/p95*100))
		}
	}
	return percentileText(z) + " · z " + fmtSigned(z, 2)
}

// percentileText is a z-score as a percentile a reader expects: whole
// percentiles, and "below the 1st" or "above the 99th" at the extremes.
func percentileText(z float64) string {
	pc := normCDF(z) * 100
	switch {
	case pc < 1:
		return "below the 1st percentile"
	case pc > 99:
		return "above the 99th percentile"
	}
	return ordinal(int(math.Round(pc))) + " percentile"
}

func (d *growthDrawing) Draw(c *core.Canvas) error {
	t := c.T
	per := 1.0
	unitName := "months"
	if d.years {
		per, unitName = 12, "years"
	}
	// The y range holds the outer centiles across the span and the points.
	var ys []float64
	steps := 96
	type curve struct {
		centile int
		pts     []float64
	}
	var curves []curve
	ages := make([]float64, steps+1)
	for i := range ages {
		ages[i] = d.lo + (d.hi-d.lo)*float64(i)/float64(steps)
	}
	for _, cen := range d.ref.centiles {
		cv := curve{centile: cen}
		for _, a := range ages {
			r, _ := lmsAt(d.rows, a)
			v := r.value(centileZ[cen])
			cv.pts = append(cv.pts, v)
			ys = append(ys, v)
		}
		curves = append(curves, cv)
	}
	ys = append(ys, d.values...)
	ylo, yhi := extent(ys)
	var pr core.Problems
	yax := resolveAxis(&pr, "", &AxisSpec{Label: d.ref.label, Unit: d.ref.unit}, ylo, yhi, axisOpts{target: 6})
	xspec := &AxisSpec{Label: "Age", Unit: unitName}
	lo, hi := d.lo/per, d.hi/per
	xspec.Min, xspec.Max = &lo, &hi
	if !d.years && hi-lo >= 12 {
		var ticks []float64
		for m := math.Ceil(lo/3) * 3; m <= hi+1e-9; m += 3 {
			ticks = append(ticks, m)
		}
		xspec.Ticks = ticks
	}
	if d.years {
		var ticks []float64
		step := 1.0
		if hi-lo > 10 {
			step = 2
		}
		for y := math.Ceil(lo/step) * step; y <= hi+1e-9; y += step {
			ticks = append(ticks, y)
		}
		xspec.Ticks = ticks
	}
	xax := resolveAxis(&pr, "", xspec, lo, hi, axisOpts{})
	p := newPlot(c, xax, yax)
	p.xGrid = true
	p.height = 236
	p.rightPad = core.TextWidth("97th", tickSize, false) + 12
	p.layout(0)
	// Bands between centile pairs, outermost first, then the grid over
	// them faintly, then the curves.
	band := func(a, b int, op string) {
		var lower, upper []float64
		for _, cv := range curves {
			if cv.centile == a {
				lower = cv.pts
			}
			if cv.centile == b {
				upper = cv.pts
			}
		}
		var poly []core.Pt
		for i, ag := range ages {
			poly = append(poly, core.Pt{X: xax.px(ag / per), Y: yax.px(upper[i])})
		}
		for i := len(ages) - 1; i >= 0; i-- {
			poly = append(poly, core.Pt{X: xax.px(ages[i] / per), Y: yax.px(lower[i])})
		}
		if cl := clipPolygon(poly, p.r); len(cl) >= 3 {
			c.El("polygon", "points", core.Points(cl), "fill", t.SeriesSoftColour(0), "fill-opacity", op)
		}
	}
	cs := d.ref.centiles
	band(cs[0], cs[6], "0.45")
	band(cs[1], cs[5], "0.6")
	band(cs[2], cs[4], "0.85")
	p.drawFrame(0)
	var ends []endLabel
	for _, cv := range curves {
		var pts []core.Pt
		for i, ag := range ages {
			pts = append(pts, core.Pt{X: xax.px(ag / per), Y: yax.px(cv.pts[i])})
		}
		w, col := 1.0, t.SeriesColour(0)
		if cv.centile == 50 {
			w = 2
		}
		for _, part := range clipPolyline(pts, p.r.grow(0.5)) {
			c.El("polyline", "points", core.Points(part), "fill", "none", "stroke", col, "stroke-width", core.F(w),
				"stroke-opacity", map[bool]string{true: "1", false: "0.7"}[cv.centile == 50], "stroke-linejoin", "round")
			p.obs.line(part)
		}
		end := pts[len(pts)-1]
		if p.r.contains(end) {
			ends = append(ends, endLabel{text: ordinal(cv.centile), y: end.Y, colour: t.Muted})
		}
	}
	// End labels in muted text, not bold: they name reference lines.
	want := make([]float64, len(ends))
	for i, e := range ends {
		want[i] = e.y
	}
	got := spread(want, tickSize+1.5, p.r.y0-2, p.r.y1+2)
	for i, e := range ends {
		if math.Abs(got[i]-e.y) > 1.5 {
			c.Line(p.r.x1+1.5, e.y, p.r.x1+4.5, got[i], t.Edge, 0.8, "")
		}
		c.Text(p.r.x1+6, got[i], e.text, core.TextStyle{Size: tickSize, Fill: t.Muted})
	}
	// The child's points, joined in order.
	var pts []core.Pt
	for i := range d.ages {
		pts = append(pts, core.Pt{X: xax.px(d.ages[i] / per), Y: yax.px(d.values[i])})
	}
	if len(pts) > 1 {
		c.Polyline(pts, t.Accent, 2.2, "")
	}
	p.obs.line(pts)
	for _, pt := range pts {
		c.Circle(pt.X, pt.Y, 3.8, t.Accent, t.Bg, 1.4)
		p.obs.point(pt)
	}
	last := pts[len(pts)-1]
	label := fmt.Sprintf("%.1f %s at %s", d.values[len(d.values)-1], d.ref.unit, fmtAge(d.ages[len(d.ages)-1]))
	p.callout(last, []string{label, d.lastNote}, t.Accent)
	c.H = p.bottom
	return nil
}

// callout labels a point with a boxed note of one or two lines, set where
// it covers least of the child's line, joined to the point by a leader.
// Over reference curves it sits on its own background, as a callout does.
func (p *plot) callout(pt core.Pt, lines []string, noteColour string) {
	c := p.c
	t := c.T
	size := tickSize + 0.5
	w := 0.0
	for i, l := range lines {
		w = math.Max(w, core.TextWidth(l, size, i == 0))
	}
	w += 12
	h := core.BlockHeight(len(lines), size, 0) + 8
	bounds := p.r.grow(-2)
	best, bestR := math.Inf(1), rect{}
	for _, dist := range []float64{22, 34, 48, 64, 84} {
		for k := 0; k < 16; k++ {
			ang := -math.Pi/2 - float64(k)*math.Pi/8
			cx := pt.X + math.Cos(ang)*(dist+w/3)
			cy := pt.Y + math.Sin(ang)*dist
			r := rect{cx - w/2, cy - h/2, cx + w/2, cy + h/2}
			// The child's line and points must stay clear; the reference
			// curves matter less, so only they are counted lightly.
			cost := dist / 20
			for _, q := range p.obs.pts {
				if r.grow(3).contains(q) {
					cost += 50
				}
			}
			for _, s := range p.obs.segs {
				if segHitsRect(s[0], s[1], r) {
					cost += 0.3
				}
			}
			for _, q := range p.obs.rects {
				if r.grow(2).overlaps(q) {
					cost += 20
				}
			}
			if !r.inside(bounds) {
				cost += 1000
			}
			if cost < best {
				best, bestR = cost, r
			}
		}
	}
	r := bestR
	end := core.Pt{X: math.Max(r.x0, math.Min(pt.X, r.x1)), Y: math.Max(r.y0, math.Min(pt.Y, r.y1))}
	if l := math.Hypot(end.X-pt.X, end.Y-pt.Y); l > 6 {
		dx, dy := (end.X-pt.X)/l, (end.Y-pt.Y)/l
		c.Line(pt.X+dx*5, pt.Y+dy*5, end.X, end.Y, t.Accent, 1.2, "")
	}
	c.Rect(r.x0, r.y0, r.w(), r.h(), 4, t.Bg, t.Accent, 1.2)
	lead := size * 1.25
	top := r.y0 + 4 + lead/2
	for i, l := range lines {
		st := core.TextStyle{Size: size, Fill: t.Ink, Bold: i == 0}
		if i > 0 {
			st.Fill = noteColour
			st.Bold = true
		}
		c.Text(r.x0+6, top+float64(i)*lead, l, st)
	}
	p.obs.rect(r)
}
