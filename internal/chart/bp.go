package chart

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Blood pressure categories by the AAP 2017 guideline (Flynn et al.,
// Pediatrics 2017, Table 3). From 13 years the thresholds are fixed, as
// for adults; from 1 to 12 they are percentiles for the child's sex, age
// and height (the guideline's Tables 4 and 5, embedded in refdata.go),
// each capped at the adult threshold, whichever is lower:
//
//	elevated  ≥ 90th percentile, or 120/80
//	stage 1   ≥ 95th percentile, or 130/80
//	stage 2   ≥ 95th percentile + 12 mmHg, or 140/90
//
// A reading's category is the higher of its systolic and diastolic ones.
// The guideline diagnoses only from repeated readings; the chart names each
// reading's category and leaves the diagnosis to the text.

var bpStages = []struct{ name, tone string }{
	{"Normal", "good"}, {"Elevated", "warn"}, {"Stage 1", "bad"}, {"Stage 2", "bad"},
}

// bpHeightCols are the height percentiles the tables are given at.
var bpHeightCols = []float64{5, 10, 25, 50, 75, 90, 95}

// bpLimits are the thresholds at one age: the lowest systolic and diastolic
// values of elevated, stage 1 and stage 2.
type bpLimits struct {
	sys, dia [3]float64
}

// bpLimitsFor are the thresholds for a child of sex at ageYears with height
// at heightPct. The tables are read as a clinician reads them: the row for
// the completed year of age and the nearest tabulated height percentile.
func bpLimitsFor(sex string, ageYears, heightPct float64) bpLimits {
	if ageYears >= 13 {
		return bpLimits{sys: [3]float64{120, 130, 140}, dia: [3]float64{80, 80, 90}}
	}
	si := 0
	if sex == "male" {
		si = 1
	}
	age := int(math.Floor(ageYears + 1e-9))
	age = max(1, min(12, age))
	hc := 0
	for i, h := range bpHeightCols {
		if math.Abs(h-heightPct) < math.Abs(bpHeightCols[hc]-heightPct) {
			hc = i
		}
	}
	row := flynn2017[si][age-1]
	p90, p95 := row[1][hc], row[2][hc]
	return bpLimits{
		sys: [3]float64{math.Min(p90[0], 120), math.Min(p95[0], 130), math.Min(p95[0]+12, 140)},
		dia: [3]float64{math.Min(p90[1], 80), math.Min(p95[1], 80), math.Min(p95[1]+12, 90)},
	}
}

// stageOf is the category (0 normal to 3 stage 2) of v against limits.
func stageOf(v float64, lim [3]float64) int {
	s := 0
	for i, l := range lim {
		if v >= l {
			s = i + 1
		}
	}
	return s
}

type bpReadingSpec struct {
	Age       *float64 `json:"age"`
	Systolic  *float64 `json:"systolic"`
	Diastolic *float64 `json:"diastolic"`
	Label     string   `json:"label"`
}

type bpSpec struct {
	Sex              string          `json:"sex"`
	HeightPercentile *float64        `json:"height_percentile"`
	Readings         []bpReadingSpec `json:"readings"`
}

const bpLabelChars = 14

var bpChart = core.Kind{
	Name:    "bp_chart",
	Family:  "clinical chart",
	Use:     "A child's blood pressure readings over time against the AAP 2017 categories (normal, elevated, stage 1, stage 2), which the app computes from the guideline's tables for the child's sex, age and height; each reading's category is stated. Give only the readings.",
	Fields:  "sex: female|male; height_percentile: number 1–99 (the child's height percentile; required when any reading is under 13 years); readings: [{age (years, decimals allowed, 1 to under 18), systolic, diastolic (mmHg), label? (" + core.UpTo(bpLabelChars) + ": \"Clinic\", \"ABPM\")}] 1–10 in order of age.",
	Example: `{"type":"bp_chart","title":"Blood pressure readings over three years in a boy","alt":"Systolic and diastolic readings for a boy from age 10 to 13 against the AAP 2017 categories: his readings rise from normal at 10 through elevated to the stage 1 range at 13, where the fixed adolescent threshold of 130/80 applies.","caption":"Categories from the AAP 2017 guideline; a diagnosis needs repeated readings.","spec":{"sex":"male","height_percentile":75,"readings":[{"age":10,"systolic":104,"diastolic":64},{"age":11,"systolic":112,"diastolic":70},{"age":12,"systolic":117,"diastolic":74},{"age":12.5,"systolic":121,"diastolic":76},{"age":13.2,"systolic":132,"diastolic":79}]}}`,
	Parse:   parseBP,
}

type bpReading struct {
	age, sys, dia float64
	label         string
	stage         int
	lim           bpLimits
}

type bpDrawing struct {
	sex      string
	hp       float64
	readings []bpReading
	lo, hi   float64
}

func parseBP(raw json.RawMessage) (core.Drawing, error) {
	var s bpSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.OneOf("sex", s.Sex, false, "female", "male")
	p.Count("readings", len(s.Readings), 1, 10)
	d := &bpDrawing{sex: s.Sex}
	young := false
	for i, r := range s.Readings {
		path := core.Path("readings", i)
		if r.Age == nil || r.Systolic == nil || r.Diastolic == nil {
			p.Add(path, "needs age, systolic and diastolic")
			continue
		}
		rd := bpReading{age: *r.Age, sys: *r.Systolic, dia: *r.Diastolic}
		rd.label = p.Label(core.Path(path, "label"), r.Label, false, bpLabelChars)
		switch {
		case rd.age < 1:
			p.Add(core.Path(path, "age"), "is %s years; the AAP 2017 categories start at 1 year (age is in years)", fmtValue(rd.age))
		case rd.age >= 18:
			p.Add(core.Path(path, "age"), "is %s years; the AAP 2017 guideline covers 1 to 17 years: from 18 use the adult guideline", fmtValue(rd.age))
		}
		if rd.age < 13 {
			young = true
		}
		if rd.sys < 50 || rd.sys > 250 {
			p.Add(core.Path(path, "systolic"), "%s mmHg is not a plausible systolic pressure", fmtValue(rd.sys))
		}
		if rd.dia < 20 || rd.dia > 150 {
			p.Add(core.Path(path, "diastolic"), "%s mmHg is not a plausible diastolic pressure", fmtValue(rd.dia))
		}
		if rd.dia >= rd.sys {
			p.Add(path, "diastolic (%s) must be below systolic (%s): check they are not swapped", fmtValue(rd.dia), fmtValue(rd.sys))
		}
		if len(d.readings) > 0 && rd.age < d.readings[len(d.readings)-1].age {
			p.Add(core.Path(path, "age"), "%s comes after a reading at %s: give the readings in order of age", fmtValue(rd.age), fmtValue(d.readings[len(d.readings)-1].age))
		}
		d.readings = append(d.readings, rd)
	}
	if s.HeightPercentile != nil {
		d.hp = *s.HeightPercentile
		if d.hp < 1 || d.hp > 99 {
			p.Add("height_percentile", "is %s; give the child's height percentile, 1 to 99", fmtValue(d.hp))
		}
	} else if young {
		p.Add("height_percentile", "is required: under 13 years the thresholds depend on height; give the child's height percentile from the growth chart (1 to 99)")
	}
	if p.Any() {
		return nil, p.Err()
	}
	for i := range d.readings {
		r := &d.readings[i]
		r.lim = bpLimitsFor(d.sex, r.age, d.hp)
		r.stage = max(stageOf(r.sys, r.lim.sys), stageOf(r.dia, r.lim.dia))
	}
	first, last := d.readings[0].age, d.readings[len(d.readings)-1].age
	d.lo = math.Max(1, math.Floor(first))
	d.hi = math.Min(18, math.Max(math.Ceil(last+0.01), d.lo+2))
	if d.hi-d.lo < 2 {
		d.lo = math.Max(1, d.hi-2)
	}
	return d, nil
}

func (d *bpDrawing) Draw(c *core.Canvas) error {
	t := c.T
	top := 0.0
	panels := []struct {
		name string
		sys  bool
	}{{"Systolic", true}, {"Diastolic", false}}
	// Where the thresholds change: each birthday below 13.
	breaks := []float64{d.lo}
	for y := math.Floor(d.lo) + 1; y < d.hi; y++ {
		if y <= 13 {
			breaks = append(breaks, y)
		}
	}
	breaks = append(breaks, d.hi)
	limitsAt := func(a float64) bpLimits { return bpLimitsFor(d.sex, a, d.hp) }
	for pi, pn := range panels {
		get := func(l bpLimits) [3]float64 {
			if pn.sys {
				return l.sys
			}
			return l.dia
		}
		var vals []float64
		for _, r := range d.readings {
			if pn.sys {
				vals = append(vals, r.sys)
			} else {
				vals = append(vals, r.dia)
			}
		}
		for _, b := range breaks {
			l := get(limitsAt(math.Min(b, d.hi-1e-6)))
			vals = append(vals, l[0], l[2])
		}
		lo, hi := extent(vals)
		lo, hi = lo-12, hi+10
		var pr core.Problems
		lo0, hi0 := d.lo, d.hi
		xs := &AxisSpec{Label: "Age", Unit: "years", Min: &lo0, Max: &hi0}
		if pi == 0 {
			xs.Label, xs.Unit = "", ""
		}
		xax := resolveAxis(&pr, "", xs, d.lo, d.hi, axisOpts{})
		yax := resolveAxis(&pr, "", &AxisSpec{Label: pn.name, Unit: "mmHg"}, lo, hi, axisOpts{target: 4})
		p := newPlot(c, xax, yax)
		p.height = 118
		p.rightPad = core.TextWidth("Elevated", tickSize, true) + 12
		p.layout(top)
		// The zones, as steps between the breaks.
		for i := 1; i < len(breaks); i++ {
			a, b := breaks[i-1], breaks[i]
			l := get(limitsAt(a + 1e-6))
			edges := []float64{yax.lo, l[0], l[1], l[2], yax.hi}
			for z := 0; z < 4; z++ {
				y0, y1 := math.Max(edges[z], yax.lo), math.Min(edges[z+1], yax.hi)
				if y1 <= y0 {
					continue
				}
				_, fill := t.Tone(bpStages[z].tone)
				op := "0.8"
				if z == 2 {
					op = "0.45"
				}
				x0, x1 := xax.px(a), xax.px(b)
				c.El("rect", "x", core.F(x0), "y", core.F(yax.px(y1)), "width", core.F(x1-x0), "height", core.F(yax.px(y0)-yax.px(y1)),
					"fill", fill, "fill-opacity", op)
			}
		}
		p.drawFrame(top)
		// Each threshold as a thin line in its zone's colour, so the steps
		// read even where two fills are close in tone.
		for i := 1; i < len(breaks); i++ {
			a, b := breaks[i-1], breaks[i]
			l := get(limitsAt(a + 1e-6))
			for z := 0; z < 3; z++ {
				if l[z] <= yax.lo || l[z] >= yax.hi || (z > 0 && l[z] == l[z-1]) {
					continue
				}
				stroke, _ := t.Tone(bpStages[z+1].tone)
				y := yax.px(l[z])
				c.El("line", "x1", core.F(xax.px(a)), "y1", core.F(y), "x2", core.F(xax.px(b)), "y2", core.F(y),
					"stroke", stroke, "stroke-width", "1", "stroke-opacity", "0.55")
			}
		}
		// Zone names at the right, level with the middle of each zone at
		// the last age.
		l := get(limitsAt(d.hi - 1e-6))
		edges := []float64{yax.lo, l[0], l[1], l[2], yax.hi}
		var labels []endLabel
		for z := 0; z < 4; z++ {
			y0, y1 := math.Max(edges[z], yax.lo), math.Min(edges[z+1], yax.hi)
			if y1-y0 <= 0.01 {
				continue
			}
			stroke, _ := t.Tone(bpStages[z].tone)
			labels = append(labels, endLabel{text: bpStages[z].name, y: yax.px((y0 + y1) / 2), colour: stroke})
		}
		p.drawEndLabels(labels, tickSize)
		// The readings.
		var pts []core.Pt
		var vs []float64
		for _, r := range d.readings {
			v := r.dia
			if pn.sys {
				v = r.sys
			}
			vs = append(vs, v)
			pts = append(pts, core.Pt{X: xax.px(r.age), Y: yax.px(v)})
		}
		if len(pts) > 1 {
			c.Polyline(pts, t.Ink, 1.8, "")
		}
		for i, pt := range pts {
			st := stageOf(vs[i], get(d.readings[i].lim))
			stroke, _ := t.Tone(bpStages[st].tone)
			c.Circle(pt.X, pt.Y, 4, stroke, t.Ink, 1.6)
		}
		top = p.bottom + 6
	}
	// Each reading and its category.
	top += 4
	c.Line(padX, top, core.Width-padX, top, t.Faint, 1, "")
	top += 4
	for _, r := range d.readings {
		y := top + 9
		stroke, _ := t.Tone(bpStages[r.stage].tone)
		age := fmtAge(r.age * 12)
		if r.label != "" {
			age += " · " + r.label
		}
		c.Text(padX, y, age, core.TextStyle{Size: labelSize, Fill: t.Muted})
		c.Text(232, y, fmt.Sprintf("%s/%s", fmtValue(r.sys), fmtValue(r.dia)), core.TextStyle{Size: labelSize, Fill: t.Ink, Bold: true, Anchor: core.End})
		c.Circle(250, y, 4, stroke, "", 0)
		c.Text(260, y, bpStages[r.stage].name, core.TextStyle{Size: labelSize, Fill: stroke, Bold: true})
		top += 18
	}
	c.H = top + 2
	return nil
}
