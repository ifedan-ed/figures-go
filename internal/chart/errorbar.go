package chart

import (
	"math"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// errorBar is one bar's error bar as its two ends on the value axis. Both
// spec forms end here: a symmetric half-width (errors) as value ± error, and
// an interval (intervals) as the low and high it was given, so an uneven
// interval is drawn where it lies rather than averaged into ± one width.
type errorBar struct{ low, high float64 }

// none is a bar with no error bar.
var none = errorBar{math.NaN(), math.NaN()}

// given says the bar has an error bar.
func (e errorBar) given() bool { return !math.IsNaN(e.low) && !math.IsNaN(e.high) }

// drawn says the error bar has a length to draw: one of zero width is a
// point and draws nothing.
func (e errorBar) drawn() bool { return e.given() && e.high > e.low }

// barAt is series i's error bar at category j.
func (d *barDrawing) barAt(i, j int) errorBar {
	if d.bars[i] == nil {
		return none
	}
	return d.bars[i][j]
}

// parseErrorBars reads one series' errors or intervals (at most one of the
// two) into error bars, one per category, and adds their ends to all, the
// values the value axis must hold. nil for a series with neither.
func parseErrorBars(p *core.Problems, path string, se barSeriesSpec, vals []float64, stacked bool, all *[]float64) []errorBar {
	if se.Errors == nil && se.Intervals == nil {
		return nil
	}
	n := len(vals)
	field := "errors"
	if se.Intervals != nil {
		field = "intervals"
	}
	if se.Errors != nil && se.Intervals != nil {
		p.Add(core.Path(path, "intervals"), "give errors (± half-widths) or intervals ([low, high]), not both")
	}
	if stacked {
		p.Add(core.Path(path, field), "error bars cannot be drawn on stacked bars: leave %s out or set stacked false", field)
	}
	given := len(se.Errors)
	if field == "intervals" {
		given = len(se.Intervals)
	}
	if given != n {
		p.Add(core.Path(path, field), "has %d values but there are %d categories: give one per category, null where there is none", given, n)
	}
	bars := make([]errorBar, n)
	for j := range bars {
		bars[j] = none
		var bar errorBar
		switch {
		case field == "errors" && j < len(se.Errors) && se.Errors[j] != nil:
			e := *se.Errors[j]
			if e < 0 {
				p.Add(core.Path(path, "errors", j), "is %s; an error is a half-width, zero or more (the bar shows value ± error)", fmtValue(e))
			}
			if math.IsNaN(vals[j]) {
				continue
			}
			bar = errorBar{vals[j] - e, vals[j] + e}
		case field == "intervals" && j < len(se.Intervals) && se.Intervals[j] != nil:
			pair := se.Intervals[j]
			if len(pair) != 2 {
				p.Add(core.Path(path, "intervals", j), "has %d numbers; an interval is [low, high]", len(pair))
				continue
			}
			bar = errorBar{pair[0], pair[1]}
			if bar.low > bar.high {
				p.Add(core.Path(path, "intervals", j), "is [%s, %s]: the low end comes first", fmtValue(bar.low), fmtValue(bar.high))
				continue
			}
			if math.IsNaN(vals[j]) {
				p.Add(core.Path(path, "intervals", j), "is given for a bar with no value: give the value, or null here")
				continue
			}
			if vals[j] < bar.low || vals[j] > bar.high {
				p.Add(core.Path(path, "intervals", j), "is [%s, %s], which does not hold the bar's value %s: an interval is around its estimate", fmtValue(bar.low), fmtValue(bar.high), fmtValue(vals[j]))
				continue
			}
		default:
			continue
		}
		bars[j] = bar
		*all = append(*all, bar.low, bar.high)
	}
	return bars
}
