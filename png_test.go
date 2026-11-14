package figures

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"strings"
	"testing"
)

// The rasteriser needs resvg, fonts and a Gotenberg 8; these tests run
// when they are given (FIGURES_TEST_RESVG, FIGURES_TEST_FONTS,
// FIGURES_TEST_CHROMIUM) and say so when they are not.
func testRasteriser(t *testing.T) *Rasteriser {
	t.Helper()
	r := &Rasteriser{Bin: os.Getenv("FIGURES_TEST_RESVG"), FontDir: os.Getenv("FIGURES_TEST_FONTS"),
		Chromium: os.Getenv("FIGURES_TEST_CHROMIUM"), Cache: MemoryCache{}}
	if r.Bin == "" || r.FontDir == "" || r.Chromium == "" {
		t.Skip("set FIGURES_TEST_RESVG, FIGURES_TEST_FONTS and FIGURES_TEST_CHROMIUM to draw PNGs")
	}
	if err := r.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

// Every example becomes two PNGs 1000 px wide, whose height matches the
// figure's, light and dark different.
func TestRasteriserDrawsEveryExample(t *testing.T) {
	r := testRasteriser(t)
	for _, name := range Types() {
		t.Run(name, func(t *testing.T) {
			ex, _ := Example(name)
			s, err := r.Draw(context.Background(), []byte(ex))
			if err != nil {
				t.Fatal(err)
			}
			if s.PNG == s.PNGDark {
				t.Fatal("light and dark are the same picture")
			}
			for _, b := range []string{s.PNG, s.PNGDark} {
				raw, err := base64.StdEncoding.DecodeString(b)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := png.DecodeConfig(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Width != PNGWidth {
					t.Fatalf("width %d", cfg.Width)
				}
				want := s.Height * PNGWidth / s.Width
				if d := float64(cfg.Height) - want; d > 3 || d < -3 {
					t.Fatalf("height %d px, the figure says %.0f", cfg.Height, want)
				}
			}
			// Shown again, it comes from the cache, the same.
			again, err := r.Show(context.Background(), &s.Figure)
			if err != nil || again.PNG != s.PNG {
				t.Fatalf("not the same when shown again: %v", err)
			}
		})
	}
}

// Mermaid's own complaint goes back to the model as a refusal.
func TestRasteriserRefusesWhatMermaidCannotRead(t *testing.T) {
	r := testRasteriser(t)
	cases := map[string]string{
		"syntax":   `flowchart TD\n  A[Start] --> B{Fever?\n  B -->|Yes| C[[Bad`,
		"too wide": `flowchart LR\n  A[Assess airway] --> B[Assess breathing] --> C[Assess circulation] --> D[Assess disability] --> E[Expose] --> F[Reassess everything]`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := r.Draw(context.Background(), []byte(`{"type":"mermaid","title":"t","alt":"a","spec":{"mermaid":"`+src+`"}}`))
			var ref *Refusal
			if !errors.As(err, &ref) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if ref.Type != "mermaid" || len(ref.Problems) != 1 || ref.Problems[0] == "" {
				t.Fatalf("refusal %+v", ref)
			}
		})
	}
}

func TestRasteriserCheckRefusesWhatIsMissing(t *testing.T) {
	for name, r := range map[string]*Rasteriser{
		"nothing":   {},
		"no cache":  {Bin: "/bin/true", FontDir: ".", Chromium: "http://127.0.0.1:1"},
		"no resvg":  {Bin: "/nonexistent/resvg", FontDir: "fonts", Chromium: "http://127.0.0.1:1", Cache: MemoryCache{}},
		"no fonts":  {Bin: "/bin/true", FontDir: t.TempDir(), Chromium: "http://127.0.0.1:1", Cache: MemoryCache{}},
		"no server": {Bin: "/bin/true", FontDir: "fonts", Chromium: "http://127.0.0.1:1", Cache: MemoryCache{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := r.Check(context.Background()); err == nil {
				t.Fatal("passed")
			}
		})
	}
}

func TestCropKeepsTheFigureAndItsMargin(t *testing.T) {
	r := testRasteriser(t)
	s, err := r.Draw(context.Background(), []byte(`{"type":"html","title":"Short","alt":"a","spec":{"html":"<p>One line</p>"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Height < 50 || s.Height > 120 {
		t.Fatalf("a one-line figure is %.0f units tall", s.Height)
	}
	if !strings.HasPrefix(string(s.Spec), `{"type":"html"`) {
		t.Fatalf("spec %s", s.Spec)
	}
}

// A figure kept with an answer before the illustration library was removed
// (2026-11-09) still shows: Show draws a kept figure from its stored SVG.
// Asked for afresh, the type is refused.
func TestKeptIllustrationStillShows(t *testing.T) {
	raw, err := os.ReadFile("testdata/kept/illustration.json")
	if err != nil {
		t.Fatal(err)
	}
	var f Figure
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if _, err := Render([]byte(`{"type":"illustration","title":"Heart","alt":"The heart.","spec":{"drawing":"heart"}}`)); err == nil {
		t.Fatal("illustration is still drawn")
	}
	r := testRasteriser(t)
	shown, err := r.Show(context.Background(), &f)
	if err != nil {
		t.Fatal(err)
	}
	if shown.PNG == "" || shown.PNGDark == "" || shown.PNG == shown.PNGDark {
		t.Fatal("the kept illustration was not drawn light and dark")
	}
}
