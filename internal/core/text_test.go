package core

import (
	"strings"
	"testing"
)

// widthOf is the width that holds exactly the first n characters of s at
// size: a box made to the measure of a fragment.
func widthOf(s string, size float64, bold bool) float64 { return TextWidth(s, size, bold) + 0.01 }

// A word that must break breaks where a typesetter would: at its own
// hyphen first, adding nothing, never leaving fewer than three letters on a
// side, never adding a hyphen next to its own.
func TestAWordBreaksWhereATypesetterWould(t *testing.T) {
	for _, c := range []struct {
		name, text string
		max        float64
		bold       bool
		want       []string
	}{
		// The two the posters showed: "Mild-moderat-" / "e:" and
		// "Life-threaten-" / "ing".
		{"at its own hyphen", "Mild-moderate: salbutamol", widthOf("Mild-moderat", Body, false), false, []string{"Mild-", "moderate:", "salbutamol"}},
		{"a bold header", "Life-threatening", widthOf("Life-threaten", Small, true), true, []string{"Life-", "threatening"}},
		{"the last hyphen that fits", "anti-inflammatory-drug", widthOf("anti-inflammatory-dr", Body, false), false, []string{"anti-inflammatory-", "drug"}},
		{"at a slash", "nausea/vomiting", widthOf("nausea/vom", Body, false), false, []string{"nausea/", "vomiting"}},
		{"at a dash", "2023–2024", widthOf("2023–20", Body, false), false, []string{"2023–", "2024"}},
		{"inside, three letters a side", "Bronchiolitis", widthOf("Bronchio-", Body, false), false, []string{"Bronchio-", "litis"}},
	} {
		size := Body
		if c.bold {
			size = Small
		}
		got := Wrap(c.text, size, c.max, c.bold)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
		for _, line := range got {
			if strings.HasSuffix(line, "--") || strings.Contains(line, ForcedBreak) {
				t.Errorf("%s: %q", c.name, line)
			}
		}
	}
}

// A hyphen is never added next to the word's own, nor with fewer than
// three letters left: when nothing else is possible the break is marked as
// forced, for the figure to be refused naming the word.
func TestABreakNoRuleAllowsIsMarkedForced(t *testing.T) {
	// "e-mail": its hyphen leaves one letter, and inside "mail" a hyphen
	// would leave fewer than three on a side.
	got := Wrap("e-mail", Body, widthOf("e-ma", Body, false), false)
	if len(got) != 2 || !strings.HasSuffix(got[0], ForcedBreak) || got[0] == "e-" {
		t.Fatalf("%q", got)
	}
	// "Mild-moderate" with room for "Mild-moder" breaks at its hyphen,
	// never as "Mild-moder-" / "ate".
	if got := Wrap("Mild-moderate", Body, widthOf("Mild-moder-", Body, false), false); strings.Join(got, "|") != "Mild-|moderate" {
		t.Errorf("%q", got)
	}
	// Two letters left after an added hyphen is not allowed: "moderat-" /
	// "e:" was the defect.
	for _, line := range Wrap("moderate:", Body, widthOf("moderat-", Body, false), false) {
		if line == "e:" || line == "te:" {
			t.Errorf("a stranded fragment %q", line)
		}
	}
}

// Forced names the word a drawing had to break by force, across a label
// written twice over its halo.
func TestForcedNamesTheWord(t *testing.T) {
	svg := `<g><text stroke="#fff">Supercalifrag` + ForcedBreak + `</text><text>Supercalifrag` + ForcedBreak +
		`</text><text stroke="#fff">ilistic &amp; more</text><text>ilistic &amp; more</text></g>`
	if word, ok := Forced(svg); !ok || word != "Supercalifragilistic" {
		t.Errorf("%q %v", word, ok)
	}
	if _, ok := Forced(`<text>Mild-</text><text>moderate</text>`); ok {
		t.Error("a break at a hyphen was taken for a forced one")
	}
}

// A box's text is made smaller, to the floor, before any word breaks.
func TestFitWordsShrinksBeforeBreaking(t *testing.T) {
	max := widthOf("Life-threatening", 11.5, true)
	if got := FitWords("Life-threatening signs", 13, Small, max, true); got != 11.5 {
		t.Errorf("fitted at %v", got)
	}
	if got := FitWords("Life-threatening", 13, Small, 10, true); got != Small {
		t.Errorf("an impossible fit: %v", got)
	}
}
