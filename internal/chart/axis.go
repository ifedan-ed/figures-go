package chart

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// AxisSpec is an axis as a spec gives it. Every field is optional; the
// range and ticks come from the data when not given.
type AxisSpec struct {
	Label string    `json:"label"`
	Unit  string    `json:"unit"`
	Min   *float64  `json:"min"`
	Max   *float64  `json:"max"`
	Scale string    `json:"scale"`
	Ticks []float64 `json:"ticks"`
}

// Text limits shared by the charts, in characters (said in words:
// core.UpTo).
const (
	axisLabelChars = 40
	axisUnitChars  = 16
	catTitleChars  = 40
)

var axisFields = "{label? (" + core.UpTo(axisLabelChars) + "), unit? (" + core.UpTo(axisUnitChars) + "), min?, max?, scale?: linear|log, ticks?: [numbers]}"

// axis is a resolved axis: its range, ticks, title, and where it is drawn.
type axis struct {
	title  string
	log    bool
	lo, hi float64
	ticks  []Tick
	// fixedLo and fixedHi say the spec gave the range's ends, so data
	// outside them is refused rather than shown.
	fixedLo, fixedHi bool
	// a and b are the figure coordinates of lo and hi.
	a, b float64
	// cats makes this a category axis: one band per name.
	cats []string
}

// px is where v is drawn.
func (x *axis) px(v float64) float64 {
	return x.a + (x.b-x.a)*x.frac(v)
}

func (x *axis) frac(v float64) float64 {
	if x.log {
		if v <= 0 {
			return math.Inf(-1)
		}
		return (math.Log10(v) - math.Log10(x.lo)) / (math.Log10(x.hi) - math.Log10(x.lo))
	}
	return (v - x.lo) / (x.hi - x.lo)
}

// value is the data value drawn at figure coordinate p.
func (x *axis) value(p float64) float64 {
	t := (p - x.a) / (x.b - x.a)
	if x.log {
		return math.Pow(10, math.Log10(x.lo)+t*(math.Log10(x.hi)-math.Log10(x.lo)))
	}
	return x.lo + t*(x.hi-x.lo)
}

// band is a category's centre and width.
func (x *axis) band(i int) (centre, width float64) {
	w := (x.b - x.a) / float64(len(x.cats))
	return x.a + w*(float64(i)+0.5), math.Abs(w)
}

// in says whether v is within the axis, with a little slack for rounding.
func (x *axis) in(v float64) bool {
	eps := (x.hi - x.lo) * 1e-9
	if x.log {
		return v >= x.lo*(1-1e-9) && v <= x.hi*(1+1e-9)
	}
	return v >= x.lo-eps && v <= x.hi+eps
}

// axisOpts are what a chart allows of an axis and how its range is found.
type axisOpts struct {
	name string // "x_axis"
	// noLog refuses a log scale (bars start at zero).
	noLog bool
	// logDefault makes log the scale when none is given.
	logDefault bool
	// zero keeps zero in an automatic range (bars, areas).
	zero bool
	// tight uses the data's own extent, not widened to ticks, for an
	// automatic range (a line chart's x).
	tight  bool
	target int
}

// resolveAxis checks s and finds the axis for data spanning dlo..dhi
// (NaN when there is none).
func resolveAxis(p *core.Problems, path string, s *AxisSpec, dlo, dhi float64, o axisOpts) *axis {
	if s == nil {
		s = &AxisSpec{}
	}
	if o.target == 0 {
		o.target = 5
	}
	x := &axis{}
	label := p.Label(core.Path(path, "label"), s.Label, false, axisLabelChars)
	unit := p.Label(core.Path(path, "unit"), s.Unit, false, axisUnitChars)
	x.title = label
	if unit != "" {
		if label == "" {
			x.title = unit
		} else {
			x.title = label + " (" + unit + ")"
		}
	}
	p.OneOf(core.Path(path, "scale"), s.Scale, true, "linear", "log")
	x.log = s.Scale == "log" || (s.Scale == "" && o.logDefault)
	if x.log && o.noLog {
		p.Add(core.Path(path, "scale"), "log is not available here: bars and boxes are measured from zero on a linear axis")
		x.log = false
	}
	if s.Min != nil {
		p.Finite(core.Path(path, "min"), *s.Min)
	}
	if s.Max != nil {
		p.Finite(core.Path(path, "max"), *s.Max)
	}
	if s.Min != nil && s.Max != nil && *s.Min >= *s.Max {
		p.Add(path, "min (%s) must be less than max (%s)", fmtValue(*s.Min), fmtValue(*s.Max))
		return fallbackAxis(x)
	}
	if x.log {
		if s.Min != nil && *s.Min <= 0 {
			p.Add(core.Path(path, "min"), "is %s, but a log axis holds only positive numbers: give a positive min or use scale linear", fmtValue(*s.Min))
			return fallbackAxis(x)
		}
		if s.Max != nil && *s.Max <= 0 {
			p.Add(core.Path(path, "max"), "is %s, but a log axis holds only positive numbers", fmtValue(*s.Max))
			return fallbackAxis(x)
		}
		if !math.IsNaN(dlo) && dlo <= 0 {
			p.Add(path, "is a log axis, but the data reach %s; a log axis holds only positive numbers: use scale linear, or leave out the values at or below zero", fmtValue(dlo))
			return fallbackAxis(x)
		}
	}
	if math.IsNaN(dlo) {
		dlo, dhi = 0, 1
		if x.log {
			dlo, dhi = 1, 10
		}
	}
	if o.zero && !x.log {
		dlo, dhi = math.Min(dlo, 0), math.Max(dhi, 0)
	}
	if s.Min != nil {
		dlo, x.fixedLo = *s.Min, true
		dhi = math.Max(dhi, dlo)
	}
	if s.Max != nil {
		dhi, x.fixedHi = *s.Max, true
		dlo = math.Min(dlo, dhi)
	}
	if x.log {
		x.lo, x.hi = dlo, dhi
		if !x.fixedLo {
			x.lo = math.Pow(10, math.Floor(math.Log10(dlo)+1e-9))
		}
		if !x.fixedHi {
			x.hi = math.Pow(10, math.Ceil(math.Log10(dhi)-1e-9))
		}
		if x.hi <= x.lo {
			x.hi = x.lo * 10
		}
		if math.Log10(x.hi)-math.Log10(x.lo) > 12 {
			p.Add(path, "spans %.0f powers of ten; at most 12 can be read: narrow the range", math.Log10(x.hi)-math.Log10(x.lo))
		}
	} else {
		lo, hi := dlo, dhi
		if hi-lo < 1e-12*math.Max(1, math.Abs(lo)) {
			d := math.Max(math.Abs(lo)*0.1, 1e-9)
			if lo == 0 {
				d = 1
			}
			if !x.fixedLo {
				lo -= d
			}
			if !x.fixedHi {
				hi += d
			}
			if hi <= lo {
				hi = lo + d
			}
		}
		nlo, nhi, _ := niceRange(lo, hi, o.target)
		if o.tight {
			nlo, nhi = lo, hi
		}
		x.lo, x.hi = lo, hi
		if !x.fixedLo {
			x.lo = nlo
		}
		if !x.fixedHi {
			x.hi = nhi
		}
	}
	// Ticks: given, or found.
	if len(s.Ticks) > 0 {
		tp := core.Path(path, "ticks")
		if len(s.Ticks) > 12 {
			p.Add(tp, "has %d ticks; at most 12 can be labelled legibly", len(s.Ticks))
		}
		for i, v := range s.Ticks {
			p.Finite(core.Path(tp, i), v)
			if !x.in(v) {
				p.Add(core.Path(tp, i), "%s is outside the axis's range %s to %s", fmtValue(v), fmtValue(x.lo), fmtValue(x.hi))
			}
			if x.log && v <= 0 {
				p.Add(core.Path(tp, i), "must be positive on a log axis")
			}
			if i > 0 && v <= s.Ticks[i-1] {
				p.Add(tp, "must be in increasing order")
				break
			}
		}
		step := 0.0
		for i := 1; i < len(s.Ticks); i++ {
			d := s.Ticks[i] - s.Ticks[i-1]
			if step == 0 || d < step {
				step = d
			}
		}
		if step == 0 {
			step = math.Abs(s.Ticks[0])
		}
		for _, v := range s.Ticks {
			lab := formatNum(v, step)
			if x.log {
				lab = fmtValue(v)
			}
			x.ticks = append(x.ticks, Tick{V: v, Label: lab})
		}
	} else if x.log {
		x.ticks = logTicks(x.lo, x.hi)
	} else {
		x.ticks = linearTicks(x.lo, x.hi, o.target)
	}
	return x
}

func fallbackAxis(x *axis) *axis {
	x.log = false
	x.lo, x.hi = 0, 1
	x.ticks = linearTicks(0, 1, 5)
	return x
}

// checkIn refuses a value outside an axis the spec fixed.
func (x *axis) checkIn(p *core.Problems, path, axisName string, v float64) {
	if x.log && v <= 0 {
		p.Add(path, "is %s, but %s is a log axis, which holds only positive numbers", fmtValue(v), axisName)
		return
	}
	if !x.in(v) {
		p.Add(path, "%s is outside %s, which runs from %s to %s: widen its min and max or leave the value out", fmtValue(v), axisName, fmtValue(x.lo), fmtValue(x.hi))
	}
}

// extent is the smallest and largest of vs that are finite (NaN, NaN when
// none are).
func extent(vs ...[]float64) (float64, float64) {
	lo, hi := math.NaN(), math.NaN()
	for _, list := range vs {
		for _, v := range list {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			if math.IsNaN(lo) || v < lo {
				lo = v
			}
			if math.IsNaN(hi) || v > hi {
				hi = v
			}
		}
	}
	return lo, hi
}

// positive keeps the positive values, for a log axis's range.
func positive(vs []float64) []float64 {
	var out []float64
	for _, v := range vs {
		if v > 0 {
			out = append(out, v)
		}
	}
	return out
}

// fmtList is "a, b and c".
func fmtList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// ordinal is 1st, 2nd, 3rd, 4th, 11th, 22nd.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
