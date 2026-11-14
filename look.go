package figures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Look is how a figure is drawn somewhere other than the apps' own light
// and dark pages: in a host's colours (a slide deck's theme), and without
// its title when the host names it already (a slide's heading).
type Look struct {
	// Palette is the host's colours; nil is Light.
	Palette *Palette
	// Untitled leaves the title out of the drawing. The figure keeps it
	// (Figure.Title), for the host's own heading and for screen readers.
	Untitled bool
}

// Palette is a host's colours, each "#rrggbb". Background and Text are
// required; the rest are worked out from them when empty. Every colour the
// figure draws with is derived from these and held to contrast on the
// background: text at 4.5:1, data marks and outlines at 3:1, a host colour
// that falls short darkened (or, on a dark background, lightened) until it
// does. The meanings figures draw by tone (good, warn, bad) and the
// tissue colours a model's own SVG may name stay the library's own, from
// Light or Dark by the background's lightness.
type Palette struct {
	Background string `json:"background"`
	Text       string `json:"text"`
	// Muted is secondary text (axis ticks, captions).
	Muted string `json:"muted,omitempty"`
	// Accent marks what a figure points out, and is the first data colour.
	Accent string `json:"accent,omitempty"`
	// Series are further data colours, after Accent and before the
	// library's own.
	Series []string `json:"series,omitempty"`
	// Surface fills a box, a node or a table cell; SurfaceAlt a table's
	// header or a compartment.
	Surface    string `json:"surface,omitempty"`
	SurfaceAlt string `json:"surface_alt,omitempty"`
}

// theme is the palette as a full theme.
func (p *Palette) theme() (*core.Theme, error) {
	bg, err := parseHex("background", p.Background, true)
	if err != nil {
		return nil, err
	}
	ink, err := parseHex("text", p.Text, true)
	if err != nil {
		return nil, err
	}
	base := core.Light
	if luminance(bg) < 0.4 {
		base = core.Dark
	}
	t := base
	t.Name = base.Name // color-scheme for HTML figures: light or dark
	t.Bg = bg.hex()
	t.Ink = toward(ink, bg, 4.5).hex()
	opt := func(name, v string, fallback rgb) (rgb, error) {
		if v == "" {
			return fallback, nil
		}
		return parseHex(name, v, false)
	}
	muted, err := opt("muted", p.Muted, mix(ink, bg, 0.38))
	if err != nil {
		return nil, err
	}
	t.Muted = toward(muted, bg, 4.5).hex()
	t.Faint = mix(ink, bg, 0.88).hex()
	t.Edge = toward(mix(ink, bg, 0.55), bg, 3).hex()
	accent, err := opt("accent", p.Accent, mustHex(base.Accent))
	if err != nil {
		return nil, err
	}
	accent = toward(accent, bg, 3)
	t.Accent = accent.hex()
	t.AccentSoft = mix(accent, bg, 0.84).hex()
	surface, err := opt("surface", p.Surface, mix(accent, bg, 0.93))
	if err != nil {
		return nil, err
	}
	t.Surface = surface.hex()
	alt, err := opt("surface_alt", p.SurfaceAlt, mix(accent, bg, 0.85))
	if err != nil {
		return nil, err
	}
	t.SurfaceAlt = alt.hex()
	// Data colours: the host's first, then the library's, each held to
	// contrast and none too close to one already taken.
	var series []rgb
	add := func(c rgb) {
		c = toward(c, bg, 3)
		for _, s := range series {
			if distance(s, c) < 60 {
				return
			}
		}
		series = append(series, c)
	}
	add(accent)
	for i, s := range p.Series {
		c, err := parseHex(fmt.Sprintf("series[%d]", i), s, false)
		if err != nil {
			return nil, err
		}
		add(c)
	}
	for _, s := range base.Series {
		add(mustHex(s))
	}
	// Always as many as the library's own (a Mermaid page names eight):
	// close colours are let in again rather than fewer.
	for _, s := range base.Series {
		if len(series) >= len(base.Series) {
			break
		}
		series = append(series, toward(mustHex(s), bg, 3))
	}
	series = series[:len(base.Series)]
	t.Series = make([]string, len(series))
	t.SeriesSoft = make([]string, len(series))
	for i, c := range series {
		t.Series[i] = c.hex()
		t.SeriesSoft[i] = mix(c, bg, 0.82).hex()
	}
	// Soft fills sit under text: kept light enough (or dark enough) that
	// the text on them still reads.
	for _, f := range []*string{&t.Surface, &t.SurfaceAlt, &t.AccentSoft, &t.GoodSoft, &t.WarnSoft, &t.BadSoft} {
		c := mustHex(*f)
		for i := 0; i < 20 && contrast(mustHex(t.Ink), c) < 4.5; i++ {
			c = mix(c, bg, 0.25)
		}
		*f = c.hex()
	}
	for i := range t.SeriesSoft {
		c := mustHex(t.SeriesSoft[i])
		for j := 0; j < 20 && contrast(mustHex(t.Ink), c) < 4.5; j++ {
			c = mix(c, bg, 0.25)
		}
		t.SeriesSoft[i] = c.hex()
	}
	return &t, nil
}

// PaletteError is a palette that could not be used: a fault in the host's
// colours, not in the model's arguments.
type PaletteError struct{ Problem string }

func (e *PaletteError) Error() string { return "figures: palette: " + e.Problem }

// DrawSVG checks draw_figure's arguments and draws the figure in look, as
// one SVG document. A browser-drawn figure (html, mermaid) has no SVG:
// draw it with Rasteriser.Picture.
func DrawSVG(raw []byte, look Look) (string, error) {
	t, err := look.theme()
	if err != nil {
		return "", err
	}
	f, d, a, err := parse(raw)
	if err != nil {
		return "", err
	}
	if f.Form != "svg" {
		return "", fmt.Errorf("figures: a %s figure is drawn by a browser; it has no SVG", f.Type)
	}
	title := f.Title
	if look.Untitled {
		title = ""
	}
	svg, _, err := draw(d, t, title, f.Alt, a.Caption)
	if err != nil {
		var r *Refusal
		if errors.As(err, &r) {
			r.Type = f.Type
		}
		return "", err
	}
	if len(svg) > MaxSVGBytes {
		return "", &Refusal{Type: f.Type, Problems: []string{"the figure came out too large to keep; draw fewer points or parts"}}
	}
	return svg, nil
}

func (l Look) theme() (*core.Theme, error) {
	if l.Palette == nil {
		return &core.Light, nil
	}
	return l.Palette.theme()
}

// MaxPictureWidth is the widest PNG Picture draws.
const MaxPictureWidth = 3000

// Picture is a figure as one PNG in look, width pixels wide (an SVG
// figure; PNGWidth for 0), for a document or a slide: from draw_figure's
// arguments as kept (Figure.Spec) or as sent. A browser-drawn figure
// (html, mermaid) is always PNGWidth wide, the width its page is laid out
// at. It is cached as Show's PNGs are.
func (r *Rasteriser) Picture(ctx context.Context, raw []byte, look Look, width int) ([]byte, error) {
	if width == 0 {
		width = PNGWidth
	}
	if width < 200 || width > MaxPictureWidth {
		return nil, fmt.Errorf("figures: a picture %d pixels wide; it must be 200 to %d", width, MaxPictureWidth)
	}
	t, err := look.theme()
	if err != nil {
		return nil, err
	}
	f, d, a, err := parse(raw)
	if err != nil {
		return nil, err
	}
	title := f.Title
	if look.Untitled {
		title = ""
	}
	if f.Form == "png" {
		pg, err := kinds[f.Type].PageOf(a.Spec)
		if err != nil {
			return nil, err
		}
		img, _, err := r.pagePNG(ctx, pg.HTML(t, title, strings.TrimSpace(a.Caption)), pg.Scripted())
		if err != nil {
			var ref *Refusal
			if errors.As(err, &ref) {
				ref.Type = f.Type
			}
		}
		return img, err
	}
	svg, _, err := draw(d, t, title, f.Alt, a.Caption)
	if err != nil {
		var ref *Refusal
		if errors.As(err, &ref) {
			ref.Type = f.Type
		}
		return nil, err
	}
	return r.svgPNGAt(ctx, svg, width)
}

// parse is Render's checking without the drawing: the figure (no SVGs),
// its drawing for a typed or SVG figure, and the arguments as read.
func parse(raw []byte) (*Figure, core.Drawing, args, error) {
	f, err := Render(raw)
	if err != nil {
		return nil, nil, args{}, err
	}
	var a args
	if err := json.Unmarshal(f.Spec, &a); err != nil {
		return nil, nil, args{}, fmt.Errorf("figures: the kept spec: %w", err)
	}
	k := kinds[a.Type]
	if k.PageOf != nil {
		return f, nil, a, nil
	}
	d, err := k.Parse(a.Spec)
	if err != nil {
		return nil, nil, args{}, err
	}
	return f, d, a, nil
}

// ── Colour arithmetic (sRGB, WCAG 2 contrast) ──────────────────────────

type rgb struct{ r, g, b float64 }

func (c rgb) hex() string {
	b := func(v float64) int { return int(math.Round(math.Max(0, math.Min(255, v)))) }
	return fmt.Sprintf("#%02x%02x%02x", b(c.r), b(c.g), b(c.b))
}

func parseHex(name, s string, required bool) (rgb, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			return rgb{}, &PaletteError{name + " is required"}
		}
		return rgb{}, &PaletteError{name + " is empty"}
	}
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 {
		return rgb{}, &PaletteError{fmt.Sprintf("%s %q is not #rrggbb", name, s)}
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{}, &PaletteError{fmt.Sprintf("%s %q is not #rrggbb", name, s)}
	}
	return rgb{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}, nil
}

func mustHex(s string) rgb {
	c, err := parseHex("colour", s, true)
	if err != nil {
		panic(err)
	}
	return c
}

// mix is a at weight 1-w and b at w.
func mix(a, b rgb, w float64) rgb {
	return rgb{a.r + (b.r-a.r)*w, a.g + (b.g-a.g)*w, a.b + (b.b-a.b)*w}
}

func luminance(c rgb) float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrast(a, b rgb) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// toward is c moved toward black (on a light background) or white (on a
// dark one) until it holds ratio against bg.
func toward(c, bg rgb, ratio float64) rgb {
	end := rgb{0, 0, 0}
	if luminance(bg) < 0.4 {
		end = rgb{255, 255, 255}
	}
	for i := 0; i < 40 && contrast(c, bg) < ratio; i++ {
		c = mix(c, end, 0.08)
	}
	return c
}

func distance(a, b rgb) float64 {
	return math.Sqrt((a.r-b.r)*(a.r-b.r) + (a.g-b.g)*(a.g-b.g) + (a.b-b.b)*(a.b-b.b))
}
