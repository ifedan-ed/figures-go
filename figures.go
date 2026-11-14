// Package figures draws a figure for a model's answer: charts, clinical
// charts, function plots and diagrams from a
// checked spec, as a small, self-contained, sanitised SVG; and, for what
// none of those covers, the model's own SVG, HTML or Mermaid, checked as
// strictly.
//
// For the typed figures the model never draws. It asks for a figure of a
// known type with parameters (the draw_figure tool, Tool); Render checks
// every parameter, refuses with words the model can correct from
// (*Refusal), and trusted code lays out and draws the picture, in Go with
// no sidecar and no picture model: a figure costs the tokens of its spec.
// Data and clinical values only ever go through typed figures, where the
// code places every number.
//
// Every figure is also a PNG (Rasteriser), which is what the apps show:
// SVG through resvg, HTML and Mermaid through a pinned Chromium.
package figures

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/chart"
	"github.com/ifedan-ed/figures-go/internal/core"
	"github.com/ifedan-ed/figures-go/internal/diagram"
	"github.com/ifedan-ed/figures-go/internal/freeform"
	"github.com/ifedan-ed/figures-go/internal/page"
)

// Refusal is a spec that was not drawn and why, for the model to correct.
type Refusal = core.Refusal

// Limits on what a model may send and what is kept.
const (
	// MaxArgsBytes is the largest draw_figure arguments read.
	MaxArgsBytes = 24 << 10
	// MaxSVGBytes is the largest SVG kept (each theme's).
	MaxSVGBytes = 256 << 10
	// MaxTitle, MaxAlt and MaxCaption are characters, told to the model
	// in words (core.UpTo).
	MaxTitle   = 90
	MaxAlt     = 400
	MaxCaption = 240
)

// Figure is a drawn figure, as it is kept with a message and sent to a
// page or an app.
type Figure struct {
	// Type is the kind drawn ("growth_chart").
	Type string `json:"type"`
	// Form is how it is shown: "svg" (SVG and SVGDark are set; pages show
	// them, apps the PNGs) or "png" (an HTML or Mermaid figure, drawn by a
	// browser: pages and apps both show the PNGs).
	Form  string `json:"form"`
	Title string `json:"title"`
	// Alt says what the figure shows, for a screen reader and for the
	// model's later turns.
	Alt string `json:"alt"`
	// Width and Height are the figure's size in its own units (the
	// viewBox); a viewer scales it to fit, keeping the ratio.
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// SVG is drawn for light pages, SVGDark for dark ones; the same
	// figure, each a complete, sanitised SVG document. Empty for form png.
	SVG     string `json:"svg,omitempty"`
	SVGDark string `json:"svg_dark,omitempty"`
	// Spec is the checked arguments it was drawn from, so it can be drawn
	// again (by a newer renderer, or to check a saved figure).
	Spec json.RawMessage `json:"spec"`
}

type args struct {
	Type    string          `json:"type"`
	Title   string          `json:"title"`
	Alt     string          `json:"alt"`
	Caption string          `json:"caption"`
	Spec    json.RawMessage `json:"spec"`
}

var kinds = func() map[string]core.Kind {
	m := map[string]core.Kind{}
	for _, group := range [][]core.Kind{chart.Kinds(), diagram.Kinds(), freeform.Kinds(), page.Kinds()} {
		for _, k := range group {
			if _, dup := m[k.Name]; dup {
				panic("figures: kind " + k.Name + " registered twice")
			}
			m[k.Name] = k
		}
	}
	return m
}()

// Types is every figure type, sorted.
func Types() []string {
	out := make([]string, 0, len(kinds))
	for n := range kinds {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Example is a type's worked example, as draw_figure arguments.
func Example(name string) (string, bool) {
	k, ok := kinds[name]
	return k.Example, ok
}

// Render checks draw_figure's arguments and draws the figure. A *Refusal
// is the arguments' fault and goes back to the model; any other error is a
// fault here.
func Render(raw []byte) (*Figure, error) {
	if len(raw) > MaxArgsBytes {
		return nil, &Refusal{Problems: []string{fmt.Sprintf("the arguments are %d bytes; at most %d are read. Draw less, or split it into two figures", len(raw), MaxArgsBytes)}}
	}
	var a args
	if err := core.Decode(raw, &a); err != nil {
		var r *Refusal
		if errors.As(err, &r) {
			for i, p := range r.Problems {
				r.Problems[i] = strings.Replace(strings.Replace(p, "spec has a field", "the arguments have a field", 1), "spec.", "", 1)
			}
		}
		return nil, err
	}
	k, ok := kinds[a.Type]
	if !ok {
		msg := "type is required"
		if a.Type != "" {
			msg = fmt.Sprintf("type %q is not a figure type", a.Type)
		}
		return nil, &Refusal{Problems: []string{msg + "; it must be one of " + strings.Join(Types(), ", ")}}
	}
	var p core.Problems
	title := p.Label("title", a.Title, true, MaxTitle)
	// The alt is never drawn: a long one is cut at a word, not refused.
	alt := p.Label("alt", clipWords(a.Alt, MaxAlt), true, MaxAlt)
	a.Alt = alt
	a.Spec = withoutType(a.Spec, a.Type)
	caption := p.Label("caption", a.Caption, false, MaxCaption)
	var d core.Drawing
	var err error
	if k.PageOf != nil {
		_, err = k.PageOf(a.Spec)
	} else {
		d, err = k.Parse(a.Spec)
	}
	if err != nil {
		var r *Refusal
		if !errors.As(err, &r) {
			return nil, err
		}
		for _, msg := range r.Problems {
			if !strings.HasPrefix(msg, "spec") {
				msg = "spec." + msg
			}
			p.Add("", "%s", msg)
		}
	}
	if err := p.Err(); err != nil {
		err.(*Refusal).Type = a.Type
		return nil, err
	}
	spec, err := canonical(a)
	if err != nil {
		return nil, err
	}
	if k.PageOf != nil {
		// Drawn by a browser: its size is known once the rasteriser has
		// drawn it (Rasteriser.Draw).
		return &Figure{Type: a.Type, Form: "png", Title: title, Alt: alt, Width: core.Width, Spec: spec}, nil
	}
	f := &Figure{Type: a.Type, Form: "svg", Title: title, Alt: alt, Spec: spec}
	for _, t := range []*core.Theme{&core.Light, &core.Dark} {
		svg, h, err := draw(d, t, title, alt, caption)
		if err != nil {
			var r *Refusal
			if errors.As(err, &r) {
				r.Type = a.Type
			}
			return nil, err
		}
		if len(svg) > MaxSVGBytes {
			return nil, &Refusal{Type: a.Type, Problems: []string{"the figure came out too large to keep; draw fewer points or parts"}}
		}
		f.Width, f.Height = core.Width, h
		if t == &core.Light {
			f.SVG = svg
		} else {
			f.SVGDark = svg
		}
	}
	return f, nil
}

// clipWords shortens s to at most max characters, at a word, with an
// ellipsis.
func clipWords(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max-1])
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

// withoutType is the spec without a "type" that repeats the figure's own,
// which models often copy into it; any other "type" is left for the kind
// to refuse.
func withoutType(spec json.RawMessage, typ string) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(spec, &m) != nil {
		return spec
	}
	t, ok := m["type"]
	if !ok {
		return spec
	}
	var s string
	if json.Unmarshal(t, &s) != nil || s != typ {
		return spec
	}
	var out bytes.Buffer
	d := json.NewDecoder(bytes.NewReader(spec))
	// Rewritten in the order sent, without the one key.
	tok, _ := d.Token()
	if tok != json.Delim('{') {
		return spec
	}
	out.WriteByte('{')
	first := true
	for d.More() {
		k, _ := d.Token()
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return spec
		}
		if k == "type" {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		out.Write(kb)
		out.WriteByte(':')
		out.Write(v)
	}
	out.WriteByte('}')
	return out.Bytes()
}

// canonical is the arguments as kept: compact, with the spec as sent.
func canonical(a args) (json.RawMessage, error) {
	var spec bytes.Buffer
	if err := json.Compact(&spec, a.Spec); err != nil {
		return nil, err
	}
	out := struct {
		Type    string          `json:"type"`
		Title   string          `json:"title"`
		Alt     string          `json:"alt"`
		Caption string          `json:"caption,omitempty"`
		Spec    json.RawMessage `json:"spec"`
	}{a.Type, strings.TrimSpace(a.Title), strings.TrimSpace(a.Alt), strings.TrimSpace(a.Caption), spec.Bytes()}
	return json.Marshal(out)
}

const pad = 14.0

// draw lays out one theme's figure: the title, the body, the caption, on
// the figure's own background.
func draw(d core.Drawing, t *core.Theme, title, alt, caption string) (string, float64, error) {
	body := core.NewCanvas(t)
	if err := d.Draw(body); err != nil {
		return "", 0, err
	}
	capLines := core.Wrap(caption, core.Small, core.Width-2*pad, false)
	y := pad
	outer := core.NewCanvas(t)
	// An empty title is a figure its host names (Look.Untitled): no
	// title line, and the body starts at the top.
	if title != "" {
		titleLines := core.Wrap(title, core.TitleSize, core.Width-2*pad, true)
		th := core.BlockHeight(len(titleLines), core.TitleSize, 0)
		outer.Lines(pad, y+th/2, titleLines, core.TextStyle{Size: core.TitleSize, Bold: true, Fill: t.Ink}, 0)
		y += th + 8
	}
	outer.Group(0, y, func() { outer.Raw(body.Body()) })
	y += body.H
	if len(capLines) > 0 {
		ch := core.BlockHeight(len(capLines), core.Small, 0)
		y += 8
		outer.Lines(pad, y+ch/2, capLines, core.TextStyle{Size: core.Small, Fill: t.Muted}, 0)
		y += ch
	}
	h := y + pad
	if word, forced := core.Forced(outer.Body()); forced {
		// A word no rule could break where it stands (core.Wrap): the
		// model shortens it, rather than a reader meeting it cut.
		return "", 0, &Refusal{Problems: []string{fmt.Sprintf("the word %q is too long for its box even at the smallest text size, and has no hyphen, dash or slash to break at; abbreviate it, or split it with a hyphen where a reader would", word)}}
	}
	if h > core.MaxHeight {
		return "", 0, &Refusal{Problems: []string{fmt.Sprintf("the figure would be %.0f units tall, more than %.0f that can be followed on a phone; draw less, or split it into two figures", h, core.MaxHeight)}}
	}
	named := title
	if named == "" {
		named = alt
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="` + svgNS + `" viewBox="0 0 ` + core.F(core.Width) + " " + core.F(h) + `" width="` + core.F(core.Width) +
		`" height="` + core.F(h) + `" role="img" aria-label="` + core.Escape(named) + `" font-family="` + core.Escape(core.FontFamily) + `">`)
	b.WriteString("<title>" + core.Escape(named) + "</title><desc>" + core.Escape(alt) + "</desc>")
	b.WriteString(`<rect x="0" y="0" width="` + core.F(core.Width) + `" height="` + core.F(h) + `" fill="` + t.Bg + `"/>`)
	b.WriteString(outer.Body())
	b.WriteString("</svg>")
	return b.String(), h, nil
}

// Summary is how a figure is recalled to the model in later turns: what
// was shown, never the picture.
func (f *Figure) Summary() string {
	return "[figure shown: " + f.Type + ", \"" + f.Title + "\": " + f.Alt + "]"
}

// ForModel is the tool's result for a figure that was drawn.
func (f *Figure) ForModel() string {
	b, _ := json.Marshal(map[string]any{
		"shown_to_user": true, "type": f.Type, "title": f.Title,
		"note": "The figure is shown with your answer. Refer to it in words where it helps (\"the chart shows ...\"); do not describe its drawing, write image markup, or link to it.",
	})
	return string(b)
}

// RefusalForModel is the tool's result for arguments that were refused.
func RefusalForModel(err error) string {
	b, _ := json.Marshal(map[string]any{
		"shown_to_user": false, "error": err.Error(),
		"next": "Correct these and call draw_figure once more, or answer without a figure if one would not help.",
	})
	return string(b)
}
