package figures

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Palettes as a deck's themes give them: light paper, warm paper, a dark
// one, and colours that fall short of contrast on their own background.
var testPalettes = map[string]Palette{
	"blue":  {Background: "#FFFFFF", Text: "#1F2937", Muted: "#4B5563", Accent: "#2563EB", Series: []string{"#D97706"}, Surface: "#EFF6FF"},
	"amber": {Background: "#ffffff", Text: "#1c1917", Accent: "#f59e0b", Series: []string{"#fde68a"}},
	"warm":  {Background: "#FFFBF6", Text: "#2E2A3B", Accent: "#E8604E", Series: []string{"#178F80"}},
	"night": {Background: "#0B1733", Text: "#F1F5F9", Muted: "#9FB0C8", Accent: "#F5B544", Series: []string{"#5EEAD4", "#FB8B7A"}, Surface: "#13234A"},
}

var hexRe = regexp.MustCompile(`#[0-9a-f]{6}`)

// Every type's example draws in every palette, as one SVG whose every
// colour comes from that palette's theme, with text and marks held to
// contrast on its background.
func TestPalettesDrawEveryExampleInTheirColours(t *testing.T) {
	for name, p := range testPalettes {
		p := p
		th, err := p.theme()
		if err != nil {
			t.Fatal(name, err)
		}
		bg := mustHex(th.Bg)
		if c := contrast(mustHex(th.Ink), bg); c < 4.5 {
			t.Errorf("%s: text at %.2f:1", name, c)
		}
		if c := contrast(mustHex(th.Muted), bg); c < 4.5 {
			t.Errorf("%s: muted text at %.2f:1", name, c)
		}
		for i, s := range th.Series {
			if c := contrast(mustHex(s), bg); c < 3 {
				t.Errorf("%s: series %d %s at %.2f:1", name, i, s, c)
			}
		}
		for _, soft := range append([]string{th.Surface, th.SurfaceAlt, th.AccentSoft}, th.SeriesSoft...) {
			if c := contrast(mustHex(th.Ink), mustHex(soft)); c < 4.5 {
				t.Errorf("%s: text on %s at %.2f:1", name, soft, c)
			}
		}
		if len(th.Series) != len(core.Light.Series) || len(th.SeriesSoft) != len(th.Series) {
			t.Errorf("%s: %d data colours", name, len(th.Series))
		}
		if th.Series[0] != th.Accent {
			t.Errorf("%s: the first data colour %s is not the accent %s", name, th.Series[0], th.Accent)
		}
		for _, typ := range Types() {
			ex, _ := Example(typ)
			f, err := Render([]byte(ex))
			if err != nil {
				t.Fatal(typ, err)
			}
			svg, err := DrawSVG([]byte(ex), Look{Palette: &p})
			if f.Form == "png" {
				if err == nil {
					t.Errorf("%s: a browser-drawn figure gave an SVG", typ)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s %s: %v", name, typ, err)
			}
			if !strings.Contains(svg, `fill="`+th.Bg+`"`) {
				t.Errorf("%s %s: not on the palette's background", name, typ)
			}
			if svg == f.SVG {
				t.Errorf("%s %s: drawn as Light", name, typ)
			}
		}
	}
}

// The library's own colours stay where they carry meaning: a zone chart's
// treat zone is the base theme's Bad, light or dark by the background.
func TestPaletteKeepsTheMeaningColours(t *testing.T) {
	for name, p := range testPalettes {
		th, err := p.theme()
		if err != nil {
			t.Fatal(err)
		}
		base := core.Light
		if name == "night" {
			base = core.Dark
		}
		if th.Bad != base.Bad || th.Good != base.Good || th.Arterial != base.Arterial {
			t.Errorf("%s: the tones or tissues changed", name)
		}
		if th.Name != base.Name {
			t.Errorf("%s: colour scheme %q", name, th.Name)
		}
	}
}

// Untitled leaves the title out of the drawing: shorter by the title's
// line, the title's words nowhere in its text, the alt naming it.
func TestUntitled(t *testing.T) {
	ex, _ := Example("flowchart")
	titled, err := DrawSVG([]byte(ex), Look{})
	if err != nil {
		t.Fatal(err)
	}
	untitled, err := DrawSVG([]byte(ex), Look{Untitled: true})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := Render([]byte(ex))
	if titled != f.SVG {
		t.Fatal("Look{} is not Light")
	}
	if strings.Contains(untitled, ">"+f.Title+"<") {
		t.Fatal("the title is still drawn")
	}
	if !strings.Contains(untitled, "<title>"+f.Alt) && !strings.Contains(untitled, `aria-label="`) {
		t.Fatal("not named for a screen reader")
	}
	height := func(svg string) float64 {
		m := regexp.MustCompile(`viewBox="0 0 [0-9.]+ ([0-9.]+)"`).FindStringSubmatch(svg)
		h, _ := strconv.ParseFloat(m[1], 64)
		return h
	}
	if height(untitled) >= height(titled)-15 {
		t.Fatalf("not shorter by the title's line: %.0f, titled %.0f", height(untitled), height(titled))
	}
}

// A palette that cannot be used is the host's fault, said as such; the
// model's arguments are still refused as a Refusal.
func TestPaletteRefusals(t *testing.T) {
	ex, _ := Example("bar_chart")
	for _, p := range []Palette{{Text: "#000000"}, {Background: "#fff", Text: "#000000"}, {Background: "#ffffff", Text: "#000000", Series: []string{"blue"}}} {
		p := p
		_, err := DrawSVG([]byte(ex), Look{Palette: &p})
		var pe *PaletteError
		if !errors.As(err, &pe) {
			t.Errorf("%+v: %v", p, err)
		}
	}
	p := testPalettes["blue"]
	_, err := DrawSVG([]byte(`{"type":"bar_chart","title":"","alt":"x","spec":{}}`), Look{Palette: &p})
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("%v", err)
	}
}

// Picture: an SVG figure at the width asked for, a browser-drawn one at
// its page's; each palette a different picture.
func TestPicture(t *testing.T) {
	r := testRasteriser(t)
	ctx := context.Background()
	night := testPalettes["night"]
	for _, typ := range []string{"flowchart", "line_chart", "svg", "mermaid", "html"} {
		ex, _ := Example(typ)
		light, err := r.Picture(ctx, []byte(ex), Look{}, 1600)
		if err != nil {
			t.Fatal(typ, err)
		}
		dark, err := r.Picture(ctx, []byte(ex), Look{Palette: &night, Untitled: true}, 1600)
		if err != nil {
			t.Fatal(typ, err)
		}
		if bytes.Equal(light, dark) {
			t.Fatalf("%s: the palette changed nothing", typ)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(light))
		if err != nil {
			t.Fatal(err)
		}
		want := 1600
		if typ == "mermaid" || typ == "html" {
			want = PNGWidth
		}
		if cfg.Width != want {
			t.Fatalf("%s: %d px wide, want %d", typ, cfg.Width, want)
		}
	}
	if _, err := r.Picture(ctx, []byte(`{}`), Look{}, 100); err == nil {
		t.Fatal("a 100 px picture was drawn")
	}
}

func TestHexColoursInPalettes(t *testing.T) {
	for name, p := range testPalettes {
		th, _ := p.theme()
		for _, c := range append([]string{th.Bg, th.Ink, th.Muted, th.Edge, th.Faint, th.Accent}, th.Series...) {
			if !hexRe.MatchString(c) {
				t.Errorf("%s: %q", name, c)
			}
		}
	}
}
