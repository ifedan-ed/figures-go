package core

// Theme is the colours a figure is drawn in. Every figure is drawn twice,
// once in Light and once in Dark, so a page or an app shows the one that
// matches its own theme; nothing in the picture depends on CSS or on the
// viewer's media queries, which an <img> and react-native-svg treat
// differently.
type Theme struct {
	Name string
	// Bg is the figure's own background, filled edge to edge so the figure
	// reads the same on any page.
	Bg string
	// Surface is a box, a node or a table cell on Bg; SurfaceAlt a second
	// tone (a table's header, a compartment inside another).
	Surface, SurfaceAlt string
	// Ink is text and the main strokes; Muted secondary text (ticks,
	// captions); Faint the lightest marks (grid lines).
	Ink, Muted, Faint string
	// Edge is an outline: a box's border, an axis.
	Edge string
	// Series are the data colours, in order: distinct for the common kinds
	// of colour blindness (after Okabe and Ito) and legible on Bg.
	Series []string
	// SeriesSoft are the same hues, light enough to fill an area or a band
	// under text.
	SeriesSoft []string
	// Accent marks what a figure points out (a highlighted part, a
	// patient's point); AccentSoft fills behind it.
	Accent, AccentSoft string
	// Good, Warn and Bad are zones and outcomes (normal, borderline,
	// treat); their Soft forms fill bands.
	Good, Warn, Bad             string
	GoodSoft, WarnSoft, BadSoft string
	// Tissue colours, by name for the model's own SVG and for schematics:
	// flesh, muscle, oxygenated and deoxygenated blood, nerve, bone, fat,
	// mucosa, fluid.
	Flesh, Muscle, Arterial, Venous, Nerve, Bone, Fat, Mucosa, Fluid string
}

// Light is for white and near-white pages.
var Light = Theme{
	Name: "light",
	Bg:   "#ffffff", Surface: "#f4f6f9", SurfaceAlt: "#e6ebf2",
	Ink: "#1d232b", Muted: "#5b6573", Faint: "#e3e7ed", Edge: "#9aa5b4",
	Series:     []string{"#1f6fb2", "#d55e00", "#008a63", "#c2387d", "#7b5ea8", "#b38600", "#3d8fbf", "#6b6b6b"},
	SeriesSoft: []string{"#d6e6f5", "#fbe0cc", "#cdeee2", "#f6d7e6", "#e4dcf0", "#f5e9c2", "#d8ebf6", "#e6e6e6"},
	Accent:     "#d1432f", AccentSoft: "#fbe1dc",
	Good: "#1d8a4e", Warn: "#b87900", Bad: "#c0392b",
	GoodSoft: "#d9f0e2", WarnSoft: "#fbeecb", BadSoft: "#f8d9d5",
	Flesh: "#f3d3c3", Muscle: "#d9796b", Arterial: "#d0453a", Venous: "#4f6fb8",
	Nerve: "#e8c547", Bone: "#efe6d2", Fat: "#f7e7a6", Mucosa: "#eaa6a0", Fluid: "#cfe6f5",
}

// Dark is for dark pages: lighter strokes, the same hues raised in
// lightness, fills kept low so text on them stays legible.
var Dark = Theme{
	Name: "dark",
	Bg:   "#16191e", Surface: "#22272f", SurfaceAlt: "#2d333d",
	Ink: "#e8ecf1", Muted: "#a3adba", Faint: "#2c323b", Edge: "#6b7684",
	Series:     []string{"#5aa9e6", "#f08a4b", "#3cc597", "#e573a8", "#a98bd6", "#e0b43a", "#7cc0e4", "#a8a8a8"},
	SeriesSoft: []string{"#1e3447", "#47301f", "#173d31", "#45263a", "#322a45", "#40361a", "#213a47", "#363636"},
	Accent:     "#ff7a63", AccentSoft: "#4a2620",
	Good: "#4cc47f", Warn: "#e7ad3c", Bad: "#f0705f",
	GoodSoft: "#173a26", WarnSoft: "#3f3215", BadSoft: "#47231f",
	Flesh: "#7a5a4d", Muscle: "#a1564b", Arterial: "#d9574b", Venous: "#5f80c8",
	Nerve: "#c9a73a", Bone: "#8f8674", Fat: "#8c7c3f", Mucosa: "#9c5f5a", Fluid: "#2f4f66",
}

// SeriesColour is the i-th data colour, repeating past the end.
func (t *Theme) SeriesColour(i int) string { return t.Series[i%len(t.Series)] }

// SeriesSoftColour is the i-th fill colour.
func (t *Theme) SeriesSoftColour(i int) string { return t.SeriesSoft[i%len(t.SeriesSoft)] }

// Tone names a colour a spec may ask for by meaning, never by value: a
// model chooses "bad" for a treatment zone, and the theme says what that
// looks like in light and in dark.
func (t *Theme) Tone(name string) (stroke, fill string) {
	switch name {
	case "good":
		return t.Good, t.GoodSoft
	case "warn":
		return t.Warn, t.WarnSoft
	case "bad":
		return t.Bad, t.BadSoft
	case "accent":
		return t.Accent, t.AccentSoft
	case "neutral", "":
		return t.Edge, t.SurfaceAlt
	}
	return t.Edge, t.SurfaceAlt
}

// Tones are the names Tone accepts, for validation.
var Tones = []string{"neutral", "good", "warn", "bad", "accent"}
