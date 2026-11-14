// Package page holds the figure types a browser draws: the model's own
// HTML and its Mermaid. Both are checked here as strictly as the free-form
// SVG (an allow-list, size caps, refusals the model can correct from), set
// in a page that can load nothing, and turned into a PNG by the
// rasteriser's Chromium. Every viewer, page or app, then shows that PNG.
package page

import (
	"fmt"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Kinds is the browser-drawn types.
func Kinds() []core.Kind { return []core.Kind{htmlKind, mermaidKind} }

// CSP lets a page load nothing at all: no fetches, images, fonts, frames
// or forms. A scripted page may run its own inline script and nothing else.
const (
	cspStatic   = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'"
	cspScripted = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'"
)

// Zoom is how much larger than the figure's units the page is drawn:
// 400 units across become the PNG's 1000 pixels, as the SVG figures do.
const Zoom = 2.5

// vars is the theme as CSS custom properties, the colour names the model
// writes (var(--ink), var(--series1)), so one HTML figure works on light
// and dark pages.
func vars(t *core.Theme) string {
	var b strings.Builder
	set := func(name, v string) { fmt.Fprintf(&b, "--%s:%s;", name, v) }
	set("ink", t.Ink)
	set("muted", t.Muted)
	set("faint", t.Faint)
	set("edge", t.Edge)
	set("bg", t.Bg)
	set("surface", t.Surface)
	set("surface-alt", t.SurfaceAlt)
	set("accent", t.Accent)
	set("accent-soft", t.AccentSoft)
	set("good", t.Good)
	set("good-soft", t.GoodSoft)
	set("warn", t.Warn)
	set("warn-soft", t.WarnSoft)
	set("bad", t.Bad)
	set("bad-soft", t.BadSoft)
	for i := range t.Series {
		set(fmt.Sprintf("series%d", i+1), t.Series[i])
		set(fmt.Sprintf("series%d-soft", i+1), t.SeriesSoft[i])
	}
	for _, x := range [][2]string{{"flesh", t.Flesh}, {"muscle", t.Muscle}, {"arterial", t.Arterial}, {"venous", t.Venous},
		{"nerve", t.Nerve}, {"bone", t.Bone}, {"fat", t.Fat}, {"mucosa", t.Mucosa}, {"fluid", t.Fluid}} {
		set(x[0], x[1])
	}
	return b.String()
}

// shell is the page around a figure's body: the theme, the figure's
// frame (title above, caption below, the same sizes as an SVG figure's),
// and its width.
func shell(t *core.Theme, csp, title, caption, style, body, script string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="` + csp + `">`)
	b.WriteString(`<style>:root{` + vars(t) + `color-scheme:` + t.Name + `}`)
	// A scripted page is laid out at 1 and zoomed once drawn: Mermaid
	// measures its text, and measures it wrongly in a zoomed page.
	if script == "" {
		fmt.Fprintf(&b, `html{zoom:%g;background:%s}`, Zoom, t.Bg)
	} else {
		fmt.Fprintf(&b, `html{background:%s}`, t.Bg)
	}
	b.WriteString(`body{margin:0;width:400px;background:var(--bg);color:var(--ink);font-family:` + core.FontFamily +
		`;font-size:13px;line-height:1.35;-webkit-font-smoothing:antialiased}` +
		`#figure{padding:14px;box-sizing:border-box;width:400px;overflow:hidden}` +
		`#figure-title{font-size:15px;font-weight:700;margin:0 0 8px;line-height:1.25}` +
		`#figure-caption{font-size:11px;color:var(--muted);margin-top:8px}` +
		`#figure-body{width:372px;overflow:hidden}` +
		`#figure-body>*{box-sizing:border-box;max-width:100%}`)
	b.WriteString(`</style>`)
	if style != "" {
		b.WriteString("<style>" + style + "</style>")
	}
	b.WriteString(`</head><body><div id="figure">`)
	// An empty title is a figure its host names: no title line.
	if title != "" {
		b.WriteString(`<h1 id="figure-title">` + core.Escape(title) + `</h1>`)
	}
	b.WriteString(`<div id="figure-body">`)
	b.WriteString(body)
	b.WriteString(`</div>`)
	if caption != "" {
		b.WriteString(`<div id="figure-caption">` + core.Escape(caption) + `</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(script)
	b.WriteString(`</body></html>`)
	return b.String()
}
