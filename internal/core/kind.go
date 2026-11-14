package core

import "encoding/json"

// Kind is one type of figure the model may ask for.
type Kind struct {
	// Name is the spec's type ("line_chart").
	Name string
	// Family groups kinds in the tool's description: "chart", "clinical
	// chart", "maths", "diagram" or "free-form".
	Family string
	// Use is one sentence: what this kind is for, said to the model.
	Use string
	// Fields is the spec's fields in brief, as the model reads them:
	// "series: [{label, points: [[x, y], ...]}] (1 to 8)", and so on.
	Fields string
	// Example is a complete, good set of draw_figure arguments for this
	// kind ({"type", "title", "alt", "spec"}), shown in the tool's
	// description and drawn by the tests.
	Example string
	// Parse checks a spec and returns something to draw, or a *Refusal.
	Parse func(spec json.RawMessage) (Drawing, error)
	// PageOf is Parse for a kind a browser draws (Page); a kind has one
	// or the other.
	PageOf func(spec json.RawMessage) (Page, error)
}

// Drawing is a checked spec, ready to be drawn. Draw is called once per
// theme and must draw the same figure each time; it sets c.H to the body's
// height. A layout that cannot fit (a flowchart too tall to read) is a
// *Refusal, as a bad spec is.
type Drawing interface {
	Draw(c *Canvas) error
}

// Page is a figure drawn by a browser rather than by this module: the
// model's HTML, or its Mermaid. Kind.PageOf checks the spec and returns one;
// the figure is then a PNG of the page, made by the rasteriser's Chromium.
type Page interface {
	// HTML is the complete, self-contained page for theme t: no network,
	// a Content-Security-Policy that loads nothing, and, when Scripted,
	// only the module's own pinned script.
	HTML(t *Theme, title, caption string) string
	// Scripted says the page runs a script (Mermaid's), which sets
	// window.figureReady when drawn and throws on a model's mistake.
	Scripted() bool
}
