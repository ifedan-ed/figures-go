package core

import (
	"html"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The figure's text is laid out here, before any viewer has a font: widths
// come from Helvetica's metrics (per 1000 units of the font size) with a
// margin, since the system fonts the figure asks for first (San Francisco,
// Segoe UI, Roboto) run a little wider. A line measured to fit, fits.

const widthMargin = 1.05

var regularWidths = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // space to /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, // 0 to 9
	278, 278, 584, 584, 584, 556, 1015, // : to @
	667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, // A to Z
	278, 278, 278, 469, 556, 333, // [ to `
	556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, // a to z
	334, 260, 334, 584, // { to ~
}

var boldWidths = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
	333, 333, 584, 584, 584, 611, 975,
	722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
	333, 278, 333, 584, 556, 333,
	556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611, 611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500,
	389, 280, 389, 584,
}

func runeWidth(r rune, bold bool) int {
	if r >= 32 && r <= 126 {
		if bold {
			return boldWidths[r-32]
		}
		return regularWidths[r-32]
	}
	switch {
	case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
		return 1000
	case unicode.IsMark(r):
		return 0
	case r == '—' || r == '→' || r == '←' || r == '↑' || r == '↓' || r == '⇒':
		return 1000
	case r == '–' || r == '·' || r == '•':
		return 556
	}
	if bold {
		return 640
	}
	return 600
}

// TextWidth is how wide s is at size, in the figure's units.
func TextWidth(s string, size float64, bold bool) float64 {
	n := 0
	for _, r := range s {
		n += runeWidth(r, bold)
	}
	return float64(n) * size / 1000 * widthMargin
}

// Wrap breaks s into lines no wider than max, at spaces. A word wider
// than max on its own is broken the way a typesetter breaks it:
//
//  1. at a hyphen, a dash or a slash it already has ("Life-" / "threatening",
//     "Mild-" / "moderate:"), adding nothing;
//  2. otherwise inside the word, with a hyphen added, leaving at least
//     MinBreakLetters letters on each side and never adding a hyphen next
//     to one it has;
//  3. otherwise by force, the line ending in ForcedBreak, which no figure
//     is drawn with: the drawing is refused naming the word (Forced).
//
// Callers keep words whole before it comes to this: a smaller size down to
// the floor (FitWords), a wider box where the layout allows one. An
// explicit newline in s starts a new line.
func Wrap(s string, size, max float64, bold bool) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		line := ""
		for _, w := range words {
			try := w
			if line != "" {
				try = line + " " + w
			}
			if TextWidth(try, size, bold) <= max {
				line = try
				continue
			}
			if line != "" {
				out = append(out, line)
				line = ""
			}
			for TextWidth(w, size, bold) > max {
				head, rest := breakWord(w, size, max, bold)
				out = append(out, head)
				w = rest
			}
			line = w
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// MinBreakLetters is the fewest letters (or digits) left on either side
// of a break inside a word: "e:" alone on a line, or "Mild-moderat-", is
// worse than any other choice.
const MinBreakLetters = 3

// ForcedBreak ends a line where a word could be broken by none of the
// rules. It is U+2010 HYPHEN, never written by the layout otherwise, so a
// drawing that holds one is found and refused (Forced) rather than shown.
const ForcedBreak = "\u2010"

// breaks are where a word already breaks: after these, nothing is added.
func isBreakChar(r rune) bool { return r == '-' || r == '–' || r == '—' || r == '/' }

// letters counts the letters and digits in s: punctuation does not keep a
// fragment company ("e:" is one letter).
func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// breakWord splits a word too wide for max into the head that ends a line
// and the rest, by the rules Wrap lists.
func breakWord(w string, size, max float64, bold bool) (head, rest string) {
	// 1. After a break the word already has: the last one that fits.
	best := -1
	for i, r := range w {
		end := i + utf8.RuneLen(r)
		if !isBreakChar(r) || end >= len(w) {
			continue
		}
		if TextWidth(w[:end], size, bold) <= max && letters(w[:end]) >= MinBreakLetters && letters(w[end:]) >= MinBreakLetters {
			best = end
		}
	}
	if best > 0 {
		return w[:best], w[best:]
	}
	// 2. Inside the word, with a hyphen: the longest head that fits with
	// it, leaving enough on both sides, and not next to a hyphen or dash
	// of the word's own.
	hyphen := TextWidth("-", size, bold)
	best = -1
	prev := rune(0)
	for i, r := range w {
		if i > 0 && TextWidth(w[:i], size, bold)+hyphen > max {
			break
		}
		if i > 0 && !isBreakChar(prev) && !isBreakChar(r) && letters(w[:i]) >= MinBreakLetters && letters(w[i:]) >= MinBreakLetters &&
			(unicode.IsLetter(prev) || unicode.IsDigit(prev)) {
			best = i
		}
		prev = r
	}
	if best > 0 {
		return w[:best] + "-", w[best:]
	}
	// 3. By force: as much as fits, marked so the figure is refused.
	cut := fitRunes(w, size, max-TextWidth(ForcedBreak, size, bold), bold)
	return w[:cut] + ForcedBreak, w[cut:]
}

// FitWords is the largest size from size down to floor at which every
// word of s fits in max whole, or floor when none does: a box's text is
// made smaller before any word in it is broken.
func FitWords(s string, size, floor, max float64, bold bool) float64 {
	for ; size > floor; size -= 0.5 {
		if WidestWord(s, size, bold) <= max {
			return size
		}
	}
	return floor
}

// WidestWord is the width of the widest word of s at size.
func WidestWord(s string, size float64, bold bool) float64 {
	w := 0.0
	for _, word := range strings.Fields(s) {
		w = math.Max(w, TextWidth(word, size, bold))
	}
	return w
}

// Forced is the first word of a drawing that had to be broken by force
// (ForcedBreak), as written, and whether there is one. The drawing is
// refused naming it: a figure is never shown with a word cut where no
// reader would cut it.
func Forced(svg string) (string, bool) {
	at := strings.Index(svg, ForcedBreak)
	if at < 0 {
		return "", false
	}
	// The head is the end of the text before the mark, the rest the start
	// of the next text element's content.
	before := svg[:at]
	head := before[strings.LastIndexAny(before, "> ")+1:]
	word := head
	// A label over a halo is written twice, the halo (stroked) first: the
	// halo copies are passed over, and when the mark was found in one, so
	// is its plain twin that follows.
	isHalo := func(tag string) bool { return strings.Contains(tag, "stroke=") }
	skipTwin := isHalo(before[strings.LastIndex(before, "<text"):])
	after := svg[at+len(ForcedBreak):]
	for {
		open := strings.Index(after, "<text")
		if open < 0 {
			break
		}
		after = after[open:]
		gt := strings.IndexByte(after, '>')
		end := strings.Index(after, "</text>")
		if gt < 0 || end < gt {
			break
		}
		tag, content := after[:gt], after[gt+1:end]
		after = after[end:]
		if isHalo(tag) {
			continue
		}
		if skipTwin {
			skipTwin = false
			continue
		}
		if piece, more := strings.CutSuffix(content, ForcedBreak); more {
			word += piece
			continue
		}
		rest, _, _ := strings.Cut(content, " ")
		word += rest
		break
	}
	return html.UnescapeString(word), true
}

// fitRunes is the byte length of the longest prefix of w (at least one
// rune) that fits in max.
func fitRunes(w string, size, max float64, bold bool) int {
	end, width := 0, 0.0
	for i, r := range w {
		width += float64(runeWidth(r, bold)) * size / 1000 * widthMargin
		if width > max && i > 0 {
			return i
		}
		end = i + utf8.RuneLen(r)
	}
	return end
}

// WidestLine is the width of the widest of lines.
func WidestLine(lines []string, size float64, bold bool) float64 {
	w := 0.0
	for _, l := range lines {
		w = max(w, TextWidth(l, size, bold))
	}
	return w
}

// Ellipsize shortens s with an ellipsis until it fits in max.
func Ellipsize(s string, size, max float64, bold bool) string {
	if TextWidth(s, size, bold) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		t := strings.TrimRight(string(r), " ") + "…"
		if TextWidth(t, size, bold) <= max {
			return t
		}
	}
	return "…"
}
