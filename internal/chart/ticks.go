package chart

import (
	"math"
	"strconv"
	"strings"
)

// Tick is a labelled place on an axis. Minor ticks get a fainter grid line
// and, on a log axis of few decades, a label.
type Tick struct {
	V     float64
	Label string
	Minor bool
}

// niceStep is the step of about target intervals across span, from the
// ladder 1, 2, 5 × 10^k that reads as round numbers.
func niceStep(span float64, target int) float64 {
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return 1
	}
	raw := span / float64(max(1, target))
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if raw <= m*mag*1.0000001 {
			return m * mag
		}
	}
	return 10 * mag
}

// niceRange widens lo..hi to whole steps, so the axis starts and ends on a
// labelled tick, and returns the step: of the round steps, the one whose
// widened range has nearest target intervals.
func niceRange(lo, hi float64, target int) (float64, float64, float64) {
	if lo == hi {
		d := math.Abs(lo) * 0.1
		if d == 0 {
			d = 1
		}
		lo, hi = lo-d, hi+d
	}
	mag := math.Pow(10, math.Floor(math.Log10((hi-lo)/float64(max(1, target)))))
	bestLo, bestHi, bestStep, bestScore := lo, hi, niceStep(hi-lo, target), math.Inf(1)
	for _, k := range []float64{0.1, 1, 10} {
		for _, m := range []float64{1, 2, 2.5, 5} {
			step := m * mag * k
			nlo := math.Floor(lo/step+1e-9) * step
			nhi := math.Ceil(hi/step-1e-9) * step
			n := (nhi - nlo) / step
			if n < 2.999 || n > 10.001 {
				continue
			}
			score := math.Abs(n - float64(target))
			if m == 2.5 {
				score += 1.5
			}
			if score < bestScore {
				bestLo, bestHi, bestStep, bestScore = nlo, nhi, step, score
			}
		}
	}
	return bestLo, bestHi, bestStep
}

// linearTicks are the multiples of a nice step inside lo..hi. Of the
// round steps giving three to eight intervals, the one nearest target
// wins, preferring a step the range's ends fall on, so a range the spec
// fixed (6 to 18) is labelled at both ends (6, 8, ..., 18) rather than
// inside them (7.5, 10, ...).
func linearTicks(lo, hi float64, target int) []Tick {
	return stepTicks(lo, hi, chooseStep(lo, hi, target))
}

func chooseStep(lo, hi float64, target int) float64 {
	span := hi - lo
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return 1
	}
	best, bestScore := niceStep(span, target), math.Inf(1)
	mag := math.Pow(10, math.Floor(math.Log10(span/float64(target))))
	for _, k := range []float64{0.1, 1, 10} {
		// 3, 4 and 6 are round too for hours, weeks and months, but only
		// win when the range ends on them (0 to 24 by 4).
		for _, m := range []float64{1, 2, 2.5, 3, 4, 5, 6} {
			step := m * mag * k
			n := span / step
			if n < 2.999 || n > 8.001 {
				continue
			}
			score := math.Abs(n - float64(target))
			ends := onStep(lo, step) && onStep(hi, step)
			if !ends {
				score += 2
			}
			switch m {
			case 2.5:
				score += 1.5
			case 3, 4, 6:
				if !ends {
					continue
				}
				score += 1
			}
			if score < bestScore {
				best, bestScore = step, score
			}
		}
	}
	return best
}

func onStep(v, step float64) bool {
	r := v / step
	return math.Abs(r-math.Round(r)) < 1e-6
}

func stepTicks(lo, hi, step float64) []Tick {
	var out []Tick
	first := math.Ceil(lo/step - 1e-9)
	for i := 0; i < 200; i++ {
		v := (first + float64(i)) * step
		if v > hi+step*1e-9 {
			break
		}
		if math.Abs(v) < step*1e-9 {
			v = 0
		}
		out = append(out, Tick{V: v, Label: formatNum(v, step)})
	}
	return out
}

// logTicks are the decades inside lo..hi (both positive), with 2 and 5 as
// labelled minor ticks when there are few decades, and 2 to 9 unlabelled
// when there are several.
func logTicks(lo, hi float64) []Tick {
	d0 := int(math.Floor(math.Log10(lo) + 1e-9))
	d1 := int(math.Ceil(math.Log10(hi) - 1e-9))
	decades := d1 - d0
	plain := d0 >= -3 && d1 <= 5
	var out []Tick
	for d := d0; d <= d1; d++ {
		base := math.Pow(10, float64(d))
		for m := 1; m <= 9; m++ {
			v := float64(m) * base
			if v < lo*(1-1e-9) || v > hi*(1+1e-9) {
				continue
			}
			switch {
			case m == 1:
				out = append(out, Tick{V: v, Label: decadeLabel(d, plain)})
			case decades <= 2 && (m == 2 || m == 5):
				lab := ""
				if plain {
					lab = formatNumG(v, v, true)
				}
				out = append(out, Tick{V: v, Label: lab, Minor: true})
			case decades <= 4:
				out = append(out, Tick{V: v, Minor: true})
			}
		}
	}
	// A range inside one decade has too few ticks to read: use linear
	// steps between its ends instead.
	labelled := 0
	for _, t := range out {
		if t.Label != "" {
			labelled++
		}
	}
	if labelled < 2 {
		return linearTicks(lo, hi, 4)
	}
	return out
}

// decadeLabel is 10^d as a number (0.01, 1000) or, past those, as a power
// of ten with a superscript exponent.
func decadeLabel(d int, plain bool) string {
	if plain {
		return formatNumG(math.Pow(10, float64(d)), math.Pow(10, float64(d)), true)
	}
	return "10" + superscript(d)
}

func superscript(n int) string {
	digits := []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")
	s := strconv.Itoa(n)
	var b strings.Builder
	for _, r := range s {
		if r == '-' {
			b.WriteRune('⁻')
			continue
		}
		b.WriteRune(digits[r-'0'])
	}
	return b.String()
}

// formatNum writes v with as many decimals as step needs (a step of 0.25
// needs two), a true minus sign, and thousands separated by commas from
// 10,000 up. Very large and very small numbers are written as a power of
// ten.
func formatNum(v, step float64) string { return formatNumG(v, step, false) }

// formatNumG is formatNum; grouped separates thousands from 1,000 up, as a
// log axis's decades are written (1,000 and 10,000 side by side).
func formatNumG(v, step float64, grouped bool) string {
	if v == 0 {
		return "0"
	}
	a := math.Abs(v)
	if a >= 1e7 || a < 1e-4 {
		return sciNum(v)
	}
	dec := decimalsFor(math.Abs(step))
	s := strconv.FormatFloat(a, 'f', dec, 64)
	if dec > 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if a >= 10000 || (a >= 1000 && grouped) {
		s = group(s)
	}
	if v < 0 && s != "0" {
		s = "−" + s
	}
	return s
}

// decimalsFor is the decimals a multiple of step needs: none for 5 or 10,
// one for 0.5, two for 0.25.
func decimalsFor(step float64) int {
	if step == 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return 2
	}
	for d := 0; d <= 8; d++ {
		scaled := step * math.Pow(10, float64(d))
		if math.Abs(scaled-math.Round(scaled)) < 1e-6*math.Max(1, scaled) {
			return d
		}
	}
	return 8
}

func group(s string) string {
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}

// sciNum is v as m×10ⁿ, with m to two significant figures and left out
// when it is 1.
func sciNum(v float64) string {
	e := int(math.Floor(math.Log10(math.Abs(v))))
	m := v / math.Pow(10, float64(e))
	ms := strconv.FormatFloat(math.Abs(m), 'g', 2, 64)
	sign := ""
	if v < 0 {
		sign = "−"
	}
	if ms == "1" {
		return sign + "10" + superscript(e)
	}
	return sign + ms + "×10" + superscript(e)
}

// fmtValue writes a data value (a point's y, a percentile's value) to
// about three significant figures, for labels rather than ticks.
func fmtValue(v float64) string {
	if v == 0 {
		return "0"
	}
	a := math.Abs(v)
	if a >= 1e7 || a < 1e-4 {
		return sciNum(v)
	}
	dec := max(0, 2-int(math.Floor(math.Log10(a))))
	if a >= 100 {
		dec = 0
	}
	return formatNum(v, math.Pow(10, -float64(dec)))
}
