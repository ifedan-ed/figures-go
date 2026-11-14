package figures

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// The two breaks the posters showed (2026-11-14) are gone from the drawn
// figures: "Mild-moderat-" / "e:" in a flowchart box, "Life-threaten-" /
// "ing" in a table heading. Both words are drawn whole, in light and dark.
func TestWordsAreDrawnWhole(t *testing.T) {
	for file, whole := range map[string]string{
		"testdata/words/asthma_flowchart.json": ">Mild-moderate:",
		"testdata/words/severity_table.json":   ">Life-threatening<",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Render(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, svg := range []string{f.SVG, f.SVGDark} {
			if !strings.Contains(svg, whole) {
				t.Errorf("%s: %q is not drawn whole", file, whole)
			}
			for _, broken := range []string{"moderat-", ">e:", "threaten-", ">ing<", "--"} {
				if strings.Contains(svg, broken) {
					t.Errorf("%s: drawn with %q", file, broken)
				}
			}
		}
	}
}

// A long word with no hyphen of its own breaks inside, with a hyphen and
// at least three letters each side; a "word" no rule can break (no letters
// to leave on either side) is refused, named, for the model to shorten:
// never drawn cut.
func TestAWordNoRuleCanBreakIsRefused(t *testing.T) {
	// The same text in every cell, so no column can widen for it.
	table := func(text string) []byte {
		cell := `{"text":"` + text + `"}`
		return []byte(`{"type":"comparison_table","title":"Dust","alt":"A word too long for its cell.","spec":{"columns":["A","B","C","D"],"rows":[{"feature":"Lung","cells":[` +
			cell + `,` + cell + `,` + cell + `,` + cell + `]}]}}`)
	}
	word := "Pneumonoultramicroscopicsilicovolcanoconiosis"
	f, err := Render(table(word))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.SVG, word) || !strings.Contains(f.SVG, ">Pneumon-") || strings.Contains(f.SVG, core.ForcedBreak) {
		t.Errorf("the long word was not broken inside it: %.200s", f.SVG)
	}
	arrows := strings.Repeat("→", 14)
	_, err = Render(table(arrows))
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), `the word "`+arrows+`" is too long for its box`) {
		t.Fatalf("an unbreakable word: %v", err)
	}
}
