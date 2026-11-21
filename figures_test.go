package figures

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden SVGs in testdata/golden")

// Every type's worked example draws, in both themes, into an SVG that the
// sanitiser gives back unchanged and that matches its golden file. A
// change to a drawing shows as a golden diff to look at (go test -update,
// then view testdata/golden).
func TestExamplesDraw(t *testing.T) {
	if len(Types()) == 0 {
		t.Fatal("no figure types are registered")
	}
	for _, name := range Types() {
		t.Run(name, func(t *testing.T) {
			ex, _ := Example(name)
			f, err := Render([]byte(ex))
			if err != nil {
				t.Fatalf("the worked example does not draw: %v", err)
			}
			if f.Type != name {
				t.Fatalf("type %q, want %q", f.Type, name)
			}
			if f.Form == "png" {
				// Drawn by a browser: TestRasteriser draws it.
				if f.SVG != "" || f.SVGDark != "" {
					t.Fatal("a browser-drawn figure has an SVG")
				}
				return
			}
			for theme, svg := range map[string]string{"light": f.SVG, "dark": f.SVGDark} {
				clean, err := Sanitise([]byte(svg))
				if err != nil {
					t.Fatalf("%s: the sanitiser refuses it: %v", theme, err)
				}
				if string(clean) != svg {
					t.Fatalf("%s: the sanitiser changed it, so Render wrote something it does not allow:\n%s", theme, firstDiff(svg, string(clean)))
				}
				golden(t, filepath.Join("testdata", "golden", name+"."+theme+".svg"), svg)
			}
			if f.Height <= 0 || f.Height > 1600 {
				t.Fatalf("height %v", f.Height)
			}
			again, err := Render(f.Spec)
			if err != nil || again.SVG != f.SVG {
				t.Fatalf("the kept spec does not draw the same figure again: %v", err)
			}
		})
	}
}

// Every example is in the tool's description, and is the type it says.
func TestDescriptionCarriesEveryExample(t *testing.T) {
	d := Description()
	for _, name := range Types() {
		ex, _ := Example(name)
		if !strings.Contains(d, ex) {
			t.Fatalf("%s's example is not in the description", name)
		}
		var a struct{ Type string }
		if err := json.Unmarshal([]byte(ex), &a); err != nil || a.Type != name {
			t.Fatalf("%s's example is of type %q (%v)", name, a.Type, err)
		}
	}
	tool := Tool()
	if _, err := json.Marshal(tool); err != nil {
		t.Fatal(err)
	}
}

// Every text limit is told to the model in words, in the tool's
// description and its schema; never in characters, which it cannot count
// (core.CharsPerWord). Only code is measured otherwise (KB, lines).
func TestLimitsAreToldInWords(t *testing.T) {
	d := Description()
	if strings.Contains(d, "character") {
		i := strings.Index(d, "character")
		t.Fatalf("the description speaks of characters: …%s…", d[max(0, i-120):i+20])
	}
	for _, want := range []string{
		"- title: what the figure shows, in at most 10 words.",
		"text (at most 20 words), shape: start|step|decision|end|note",                        // flowchart
		"label? (at most 4 words)}] are the other",                                            // flowchart edges
		"A cell is text (at most 15 words in a table of two columns, 10 in three, 8 in four)", // comparison table
		"nodes: [{id, text (at most 10 words), tone?, input?}]",                               // pathway
		"children?: [{text (at most 10 words)",                                                // concept map
		"compartments: [{id, label (at most 10 words)",                                        // schematic
		"series: [{label (required if >1; at most 4 words)",                                   // charts
		"unit? (at most 2 words)",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("the description does not say %q", want)
		}
	}
	if n := strings.Count(d, " words"); n < 40 {
		t.Errorf("the description gives %d limits in words; every label field should have one", n)
	}
	props := Tool()["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)
	for field, want := range map[string]string{"title": "at most 10 words", "alt": "at most 55 words", "caption": "at most 30 words", "spec": "words its field allows"} {
		if desc := props[field].(map[string]any)["description"].(string); !strings.Contains(desc, want) {
			t.Errorf("the schema's %s says %q, not %q", field, desc, want)
		}
	}
}

// The model's most frequent refusal (measured 2026-11-10: a flowchart box
// a few characters over 120) now draws, and a box past the new limit is
// refused in words with the words to keep to.
func TestFlowchartBoxesInWords(t *testing.T) {
	box := func(text string) string {
		return `{"type":"flowchart","title":"Sepsis","alt":"Two steps.","spec":{"nodes":[{"id":"a","text":"` + text +
			`","shape":"start","next":"b"},{"id":"b","text":"Reassess","shape":"end"}]}}`
	}
	// 147 characters of clinical prose: refused before, at 120.
	ok := "Give IV ceftriaxone 80 mg/kg after blood cultures, then a 10 mL/kg saline bolus if capillary refill is longer than three seconds or lactate is high"
	if _, err := Render([]byte(box(ok))); err != nil {
		t.Fatalf("a box of %d characters is refused: %v", len(ok), err)
	}
	_, err := Render([]byte(box(ok + " and repeat it twice, watching for crackles and an enlarging liver")))
	if err == nil || !strings.Contains(err.Error(), "spec.nodes[0].text: is 37 words; keep it to 20 or fewer") || !strings.Contains(err.Error(), `"Give IV ceftriaxone 80 mg/kg after…"`) {
		t.Fatalf("a box of 37 words: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	cases := map[string]struct{ args, want string }{
		"not JSON":         {`{"type":`, "not valid JSON"},
		"unknown type":     {`{"type":"pie","title":"t","alt":"a","spec":{}}`, `"pie" is not a figure type`},
		"no title":         {`{"type":"` + Types()[0] + `","alt":"a","spec":{}}`, "title: is required"},
		"extra field":      {`{"type":"` + Types()[0] + `","title":"t","alt":"a","spec":{},"colour":"red"}`, `field "colour"`},
		"title too long":   {`{"type":"` + Types()[0] + `","title":"` + strings.Repeat("word ", 19) + `","alt":"a","spec":{}}`, "title: is 19 words; keep it to 10 or fewer"},
		"spec not object":  {`{"type":"` + Types()[0] + `","title":"t","alt":"a","spec":3}`, "spec"},
		"oversized":        {`{"type":"x","title":"` + strings.Repeat("x", MaxArgsBytes) + `"}`, "bytes"},
		"markup for words": {`{"type":"` + Types()[0] + `","title":"<b>x</b>","alt":"a","spec":{}}`, "markup"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Render([]byte(c.args))
			var r *Refusal
			if !errors.As(err, &r) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refusal %q does not say %q", err.Error(), c.want)
			}
			if !strings.Contains(RefusalForModel(err), "call draw_figure once more") {
				t.Fatal("the model is not told what to do next")
			}
		})
	}
}

// The parser and every kind's checks never panic and never draw something
// the sanitiser would change, whatever the model sends.
func FuzzRender(f *testing.F) {
	for _, name := range Types() {
		ex, _ := Example(name)
		f.Add([]byte(ex))
	}
	f.Add([]byte(`{"type":"line_chart","title":"t","alt":"a","spec":{"series":[{"label":"a","points":[[1e308,1e308],[-1e308,0]]}]}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		fig, err := Render(raw)
		if err != nil || fig.Form == "png" {
			return
		}
		for _, svg := range []string{fig.SVG, fig.SVGDark} {
			clean, err := Sanitise([]byte(svg))
			if err != nil || string(clean) != svg {
				t.Fatalf("drew an SVG the sanitiser changes or refuses: %v", err)
			}
		}
	})
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no golden file (go test -run TestExamplesDraw -update): %v", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from what is drawn now (view it, then go test -update if the change is right):\n%s", path, firstDiff(string(want), got))
	}
}

func firstDiff(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := max(0, i-80)
	return "want …" + a[lo:min(len(a), i+80)] + "…\n got …" + b[lo:min(len(b), i+80)] + "…"
}

// Two mistakes models often make are taken in stride: a long alt is cut
// (it is never drawn), and a spec that repeats the figure's type is read
// without it.
func TestForgivenMistakes(t *testing.T) {
	ex, _ := Example("line_chart")
	var a map[string]any
	if err := json.Unmarshal([]byte(ex), &a); err != nil {
		t.Fatal(err)
	}
	a["alt"] = strings.Repeat("word ", 120)
	a["spec"].(map[string]any)["type"] = "line_chart"
	b, _ := json.Marshal(a)
	f, err := Render(b)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(f.Alt)); n > MaxAlt || !strings.HasSuffix(f.Alt, "…") {
		t.Fatalf("alt %d characters: %q", n, f.Alt)
	}
	if strings.Contains(string(f.Spec), `"spec":{"type"`) {
		t.Fatalf("the kept spec still has its type: %s", f.Spec)
	}
	a["spec"].(map[string]any)["type"] = "bar_chart"
	b, _ = json.Marshal(a)
	if _, err := Render(b); err == nil {
		t.Fatal("a spec naming another type was drawn")
	}
}
