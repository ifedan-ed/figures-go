package figures

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// ToolName is the tool the model calls.
const ToolName = "draw_figure"

// Guidance is a line for a system prompt that offers the tool: when to
// draw. A figure the person asks for is always drawn; otherwise one is
// drawn when it makes the answer clearer, never as decoration.
const Guidance = "You can draw a figure with the draw_figure tool: a chart, a clinical chart, a function plot, a diagram, or your own SVG, HTML or Mermaid when none of those fits. " +
	"When the person asks for a drawing, diagram, chart, graph, table, flowchart, illustration, picture or image, draw it. " +
	"Otherwise draw when a figure makes your answer clearer: numbers or a trend worth seeing, a process or algorithm with steps and branches, a pathway or mechanism, anatomy, a comparison of several things on several points. " +
	"Do not draw for decoration or to repeat a short list. Draw at most one or two per answer. Your written answer must stand on its own; the figure supports it."

var toolIntro = fmt.Sprintf(`Draw a figure for the person, shown beside your answer, on the website and in the apps. Most types take content and the app draws them; three free-form types (svg, html, mermaid) take a drawing you write yourself. Never write image prompts, and never put image markup or links in your answer.

When to draw: whenever the person asks for a drawing, diagram, chart, graph, table, flowchart, illustration, picture or image. Otherwise when a figure makes the answer clearer: data or a trend (a growth curve, a dose-response, a lab value over time), a process or management algorithm with decisions, a cascade or pathway, a mechanism, anatomy, or a comparison of several options on several features. Do not draw for decoration, for a simple fact, or for something a short list says as well.

Rules for every figure:
- title: what the figure shows, in %s.
- alt: one or two plain sentences saying what the figure shows and its main point, for someone who cannot see it.
- caption (optional): a short note under the figure, such as the source of the numbers ("Thresholds from the AAP 2022 guideline").
- spec: the type's content, below. Use only the fields listed for that type; numbers are plain JSON numbers, no units inside them (units go in axis labels).
- Use only values you are confident of from the sources or the conversation. Never invent data to make a figure look complete; leave it out.
- Labels are short: a few words. Each text field's limit is given in words below; keep to it, as a text over its limit is refused. Explanations belong in your answer, not inside the figure.
- spec holds only the type's fields: never repeat "type" or "title" inside it.
- Colours are chosen by meaning where a field says "tone": neutral, good, warn, bad or accent. Never give colour values to typed figures.
- If the result says the figure was not drawn, read the problems, correct the arguments and call again once, or answer without a figure; when the person asked for the figure, say in one short sentence that it could not be drawn.

Which type to use:
- Numbers, measurements, thresholds, curves: a chart or clinical chart, always. The app places every value exactly; never draw data in svg, html or mermaid.
- A process, algorithm, cascade, cycle, timeline, comparison or concept map: the matching diagram type; they are laid out for phones.
- Only when nothing above fits: mermaid for other diagram shapes (sequence, state, mind map, Gantt), svg for a mechanism or a sketch you must draw yourself, html for a styled layout of boxes and text.

The types, each with its fields and a worked example:`, core.UpTo(MaxTitle))

// familyOrder is the order families appear in the description.
var familyOrder = []string{"chart", "clinical chart", "maths", "diagram", "free-form"}

var familyTitle = map[string]string{
	"chart": "Charts", "clinical chart": "Clinical charts (reference data built in; give only the patient's values)", "maths": "Maths",
	"diagram":   "Diagrams (laid out for you)",
	"free-form": "Free-form (you write the drawing; only for what no type above covers)",
}

// Description is the tool's whole description: when to draw, the rules,
// and every type's fields and worked example.
func Description() string {
	var b strings.Builder
	b.WriteString(toolIntro)
	byFamily := map[string][]string{}
	for name, k := range kinds {
		byFamily[k.Family] = append(byFamily[k.Family], name)
	}
	for _, fam := range familyOrder {
		names := byFamily[fam]
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		b.WriteString("\n\n## " + familyTitle[fam])
		for _, n := range names {
			k := kinds[n]
			b.WriteString("\n\n### " + n + "\n" + k.Use + "\nspec fields: " + k.Fields + "\nExample: " + k.Example)
		}
	}
	return b.String()
}

// Tool is draw_figure as an OpenAI-style function tool, for the chat
// completions request's tools list.
func Tool() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        ToolName,
		"description": Description(),
		"parameters": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"type", "title", "alt", "spec"},
			"properties": map[string]any{
				"type":    map[string]any{"type": "string", "enum": anyStrings(Types()), "description": "The figure type."},
				"title":   map[string]any{"type": "string", "minLength": 1, "maxLength": MaxTitle, "description": "What the figure shows, " + core.UpTo(MaxTitle) + "."},
				"alt":     map[string]any{"type": "string", "minLength": 1, "maxLength": MaxAlt, "description": "What the figure shows and its main point, in plain sentences; " + core.UpTo(MaxAlt) + "."},
				"caption": map[string]any{"type": "string", "maxLength": MaxCaption, "description": "Optional note under the figure, such as where its numbers come from; " + core.UpTo(MaxCaption) + "."},
				"spec":    map[string]any{"type": "object", "description": "The type's content: only the fields the description lists for this type, each text within the words its field allows."},
			},
		},
	}}
}

func anyStrings(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
