package page

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// mermaidJS is Mermaid 11.16.1 (MIT, mermaid.LICENSE), dist/mermaid.min.js
// from the npm package, whose sha1 shasum is
// 57ae2342f6c45b967113b04c9258430bdd057ee8. Pinned here and sent inline
// with each page; nothing is fetched.
//
//go:embed mermaid.min.js
var mermaidJS string

// MermaidVersion is the Mermaid drawing the diagrams.
const MermaidVersion = "11.16.1"

// Limits on the model's Mermaid.
const (
	MaxMermaidBytes = 8 << 10
	MaxMermaidLines = 150
)

var mermaidKind = core.Kind{
	Name:   "mermaid",
	Family: "free-form",
	Use: "A Mermaid diagram, for diagram shapes the typed ones lack: a sequence diagram (who signals whom), a state diagram, a mind map, a class or entity diagram, a Gantt chart of a treatment course. " +
		"Prefer flowchart, pathway or timeline (typed) when they fit: they are laid out for phones. Never for numeric data.",
	Fields: "mermaid: the diagram's Mermaid source (Mermaid " + MermaidVersion + "), at most 8 KB and 150 lines, starting with its type: " +
		"flowchart TD (or graph TD), sequenceDiagram, stateDiagram-v2, classDiagram, erDiagram, gantt, timeline, mindmap, journey, quadrantChart or pie. " +
		"Lay flowcharts out top-down (TD), not LR: the figure is phone-width, and a diagram much wider than tall is refused. " +
		"Not allowed: %%{init}%% or other directives, click, href, links, HTML in labels, styles with url(). Colours and fonts are set for you; leave out style and classDef lines. " +
		"Quote labels with punctuation: A[\"Fever ≥ 38 °C?\"]. If Mermaid cannot read it, the error comes back; correct it and call again.",
	Example: mermaidExample,
	PageOf:  parseMermaid,
}

const mermaidExample = `{"type":"mermaid","title":"Insulin, glucose and potassium in DKA","alt":"A sequence: insulin drives glucose and potassium into cells, so serum potassium falls; potassium is replaced once urine output is confirmed.","spec":{"mermaid":"sequenceDiagram\n  participant I as IV insulin\n  participant C as Cells\n  participant S as Serum\n  I->>C: Glucose uptake\n  I->>C: K⁺ shifts in (Na⁺/K⁺-ATPase)\n  C-->>S: Serum K⁺ falls\n  Note over S: Add K⁺ to fluids once passing urine\n  Note over C,S: Glucose falls, ketones clear"}}`

var (
	mermaidTypes = regexp.MustCompile(`^(flowchart|graph)\s+(TD|TB|BT|LR|RL)\b|^(sequenceDiagram|stateDiagram-v2|stateDiagram|classDiagram|erDiagram|gantt|timeline|mindmap|journey|quadrantChart|pie)\b`)
	badMermaid   = regexp.MustCompile(`(?i)%%\{|\bclick\b|\bhref\b|\bcallback\b|javascript:|<\s*/?\s*(script|img|iframe|a|style|div|span|br|svg|foreignobject)\b|url\s*\(|@import|\blinkStyle\b`)
	styleLine    = regexp.MustCompile(`(?m)^\s*(style|classDef|class)\s`)
)

type mermaidSpec struct {
	Mermaid string `json:"mermaid"`
}

type mermaidPage struct{ src string }

func parseMermaid(raw json.RawMessage) (core.Page, error) {
	var s mermaidSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	src := strings.TrimSpace(strings.ReplaceAll(s.Mermaid, "\r\n", "\n"))
	// Comments (%% …) are allowed; a directive (%%{ … }%%) is not.
	first := ""
	for _, l := range strings.Split(src, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "%%") {
			first = t
			break
		}
	}
	switch {
	case src == "":
		p.Add("mermaid", "is required: the diagram's Mermaid source")
	case len(src) > MaxMermaidBytes:
		p.Add("mermaid", "is %d bytes; at most %d", len(src), MaxMermaidBytes)
	case strings.Count(src, "\n")+1 > MaxMermaidLines:
		p.Add("mermaid", "has %d lines; at most %d", strings.Count(src, "\n")+1, MaxMermaidLines)
	case !mermaidTypes.MatchString(first):
		p.Add("mermaid", "must start with its diagram type, such as \"flowchart TD\" or \"sequenceDiagram\"; it starts %q", clip(first))
	}
	if m := badMermaid.FindString(src); m != "" {
		p.Add("mermaid", "%q is not allowed: no directives, links, clicks, HTML or loaded styles", m)
	}
	if m := styleLine.FindString(src); m != "" {
		p.Add("mermaid", "leave out style, classDef and class lines; colours are set for you")
	}
	if strings.Contains(src, "</") {
		p.Add("mermaid", "has markup; labels are plain text")
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	return &mermaidPage{src: src}, nil
}

func clip(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

func (m *mermaidPage) Scripted() bool { return true }

// HTML is Mermaid's page: strict security (no HTML in labels, no clicks),
// the theme's colours, drawn into the figure's width. A diagram Mermaid
// cannot read, or one far wider than the phone, throws, and the
// rasteriser sends Mermaid's own words back to the model.
func (m *mermaidPage) HTML(t *core.Theme, title, caption string) string {
	src, _ := json.Marshal(m.src)
	vars := map[string]any{
		"darkMode": t.Name == "dark", "background": t.Bg, "fontFamily": core.FontFamily, "fontSize": "13px",
		"primaryColor": t.Surface, "primaryTextColor": t.Ink, "primaryBorderColor": t.Edge, "lineColor": t.Muted,
		"secondaryColor": t.SurfaceAlt, "tertiaryColor": t.Bg, "textColor": t.Ink, "noteBkgColor": t.WarnSoft, "noteTextColor": t.Ink,
		"noteBorderColor": t.Warn, "actorBkg": t.Surface, "actorBorder": t.Edge, "actorTextColor": t.Ink, "signalColor": t.Ink,
		"signalTextColor": t.Ink, "labelBoxBkgColor": t.Surface, "labelTextColor": t.Ink, "edgeLabelBackground": t.Bg,
		"clusterBkg": t.SurfaceAlt, "clusterBorder": t.Edge, "titleColor": t.Ink, "pie1": t.Series[0], "pie2": t.Series[1],
		"pie3": t.Series[2], "pie4": t.Series[3], "pie5": t.Series[4], "pie6": t.Series[5], "pie7": t.Series[6], "pie8": t.Series[7],
		"cScale0": t.SeriesSoft[0], "cScale1": t.SeriesSoft[1], "cScale2": t.SeriesSoft[2], "cScale3": t.SeriesSoft[3],
		"cScale4": t.SeriesSoft[4], "cScale5": t.SeriesSoft[5], "cScale6": t.SeriesSoft[6], "cScale7": t.SeriesSoft[7],
	}
	tv, _ := json.Marshal(vars)
	zoom := strconv.FormatFloat(Zoom, 'g', -1, 64)
	ff, _ := json.Marshal(core.FontFamily)
	script := `<script>` + mermaidJS + `</script><script>
(function () {
  function fail(msg) {
    window.figureReady = true;
    setTimeout(function () { throw new Error("FIGURE: " + msg); });
  }
  try {
    mermaid.initialize({startOnLoad: false, securityLevel: "strict", theme: "base", themeVariables: ` + string(tv) + `,
      htmlLabels: true, flowchart: {htmlLabels: true, useMaxWidth: true, nodeSpacing: 30, rankSpacing: 36, padding: 8, wrappingWidth: 150},
      fontFamily: ` + string(ff) + `, sequence: {actorFontFamily: ` + string(ff) + `, noteFontFamily: ` + string(ff) + `, messageFontFamily: ` + string(ff) + `, useMaxWidth: true, width: 110, actorMargin: 24, boxMargin: 6, boxTextMargin: 4, noteMargin: 6, messageMargin: 32, wrap: true, mirrorActors: false},
      state: {useMaxWidth: true}, class: {useMaxWidth: true}, er: {useMaxWidth: true}, gantt: {useMaxWidth: true, barHeight: 18, fontSize: 12, leftPadding: 60},
      mindmap: {useMaxWidth: true, padding: 8}, timeline: {useMaxWidth: true}, journey: {useMaxWidth: true}, pie: {useMaxWidth: true}, deterministicIds: true, maxTextSize: 20000});
  } catch (e) { fail("Mermaid could not start: " + e.message); return; }
  var src = ` + string(src) + `;
  mermaid.render("figure-mermaid", src).then(function (r) {
    var box = document.getElementById("figure-body");
    box.innerHTML = r.svg;
    var svg = box.querySelector("svg");
    if (!svg) { fail("Mermaid drew nothing"); return; }
    var vb = svg.viewBox && svg.viewBox.baseVal;
    var w = vb && vb.width ? vb.width : svg.getBBox().width, h = vb && vb.height ? vb.height : svg.getBBox().height;
    if (w > 372 * 1.7) { fail("the diagram is " + Math.round(w) + " wide, too wide to read on a phone; lay it out top-down (flowchart TD), shorten labels or show fewer side-by-side parts"); return; }
    svg.removeAttribute("height");
    svg.style.width = Math.min(372, w) + "px";
    svg.style.maxWidth = "372px";
    svg.style.height = "auto";
    svg.style.display = "block";
    svg.style.margin = "0 auto";
    document.documentElement.style.zoom = "` + zoom + `";
    window.figureReady = true;
  }).catch(function (e) { fail("Mermaid could not read the diagram: " + (e && (e.message || e.str) || e)); });
})();
</script>`
	return shell(t, cspScripted, title, caption, "", "", script)
}
