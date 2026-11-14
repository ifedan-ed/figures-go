package page

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Limits on the model's HTML.
const (
	MaxHTMLBytes    = 30 << 10
	MaxHTMLElements = 600
	MaxHTMLDepth    = 20
)

var htmlKind = core.Kind{
	Name:   "html",
	Family: "free-form",
	Use: "A small HTML and CSS layout, for what reads best as styled boxes and text: a labelled card grid, a staged summary, a dosing-free visual checklist, a side-by-side layout a comparison_table cannot express. " +
		"No scripts and nothing loaded. Never for data or numbers that need a chart, and prefer a typed figure when one fits.",
	Fields: "html: the figure's body, at most 30 KB of well-formed HTML (close every tag; <br/> is fine), laid out for a 372 px wide box (it is shown at a phone's width). " +
		"Elements: div, span, p, h2 to h4, ul, ol, li, dl, dt, dd, table, caption, thead, tbody, tfoot, tr, th, td, colgroup, col, b, strong, i, em, u, s, small, sub, sup, mark, br, hr, blockquote, code, pre, figure, figcaption, section, header, footer, article, aside, abbr, kbd, q, cite, time, style. " +
		"Attributes: class, id, style, colspan, rowspan, scope, title, role, lang, dir, aria-*. CSS in style attributes or one <style> element; no url(), @import, @font-face or position: fixed. " +
		"Colours: write var(--ink), var(--muted), var(--faint), var(--edge), var(--bg), var(--surface), var(--surface-alt), var(--accent), var(--accent-soft), var(--good), var(--good-soft), var(--warn), var(--warn-soft), var(--bad), var(--bad-soft), var(--series1) to var(--series8) and var(--series1-soft) to var(--series8-soft), " +
		"so it shows well on light and dark pages; any other colour is allowed but the figure is then shown on a white card in dark mode. Fonts: sizes 11 px or more; the font is set for you. " +
		"Not allowed: script, style with url(), img, svg (use type svg), iframe, object, embed, link, meta, a, form, input, button, video, audio, event attributes.",
	Example: htmlExample,
	PageOf:  parseHTML,
}

const htmlExample = `{"type":"html","title":"Dehydration: how sick is this child?","alt":"Three cards from minimal to severe dehydration, each listing its signs: severe has lethargy, sunken eyes, very slow skin pinch and inability to drink.","spec":{"html":"<style>.g{display:grid;gap:8px}.c{border-radius:10px;padding:10px 12px;border:1px solid var(--edge)}.c h3{margin:0 0 4px;font-size:14px}.c ul{margin:0;padding-left:18px}</style><div class=\"g\"><div class=\"c\" style=\"background:var(--good-soft)\"><h3 style=\"color:var(--good)\">Minimal (&lt;3%)</h3><ul><li>Alert, drinks normally</li><li>Moist mucous membranes</li></ul></div><div class=\"c\" style=\"background:var(--warn-soft)\"><h3 style=\"color:var(--warn)\">Some (3–9%)</h3><ul><li>Restless, thirsty</li><li>Sunken eyes, dry mouth</li><li>Skin pinch goes back slowly</li></ul></div><div class=\"c\" style=\"background:var(--bad-soft)\"><h3 style=\"color:var(--bad)\">Severe (&gt;9%)</h3><ul><li>Lethargic or unconscious</li><li>Drinks poorly or not at all</li><li>Skin pinch very slow (&gt;2 s)</li></ul></div></div>"}}`

var (
	htmlElements = map[string]bool{}
	htmlAttrs    = map[string]bool{"class": true, "id": true, "style": true, "colspan": true, "rowspan": true, "scope": true,
		"title": true, "role": true, "lang": true, "dir": true, "span": true}
	htmlHints = map[string]string{
		"script": "scripts never run in a figure", "img": "pictures cannot be loaded; draw it with type svg", "svg": "use type svg for a drawing",
		"iframe": "nothing can be embedded", "object": "nothing can be embedded", "embed": "nothing can be embedded", "link": "nothing can be loaded",
		"meta": "the page's head is set for you", "a": "figures have no links", "form": "figures take no input", "input": "figures take no input",
		"button": "figures take no input", "video": "use a still figure", "audio": "use a still figure", "canvas": "canvas needs a script; use type svg",
		"h1": "the title is drawn for you; use h2 to h4 inside", "html": "give only the body's content", "body": "give only the body's content", "head": "give only the body's content",
	}
	voidElements = map[string]bool{"br": true, "hr": true, "col": true}
	badCSS       = regexp.MustCompile(`(?i)url\s*\(|@import|@font-face|@namespace|expression\s*\(|javascript:|behavior\s*:|-moz-binding|position\s*:\s*fixed|\\|</`)
	literalCSS   = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b|\b(rgb|rgba|hsl|hsla|oklch|lab|lch|color)\s*\(|\b(black|white|red|green|blue|yellow|orange|purple|pink|brown|gray|grey|navy|teal|maroon|olive|crimson|gold|silver|cyan|magenta)\b`)
)

func init() {
	for _, n := range strings.Fields("div span p h2 h3 h4 ul ol li dl dt dd table caption thead tbody tfoot tr th td colgroup col b strong i em u s small sub sup mark br hr blockquote code pre figure figcaption section header footer article aside abbr kbd q cite time style") {
		htmlElements[n] = true
	}
}

type htmlSpec struct {
	HTML string `json:"html"`
}

type htmlPage struct {
	body, style string
	literal     bool
}

func parseHTML(raw json.RawMessage) (core.Page, error) {
	var s htmlSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	src := strings.TrimSpace(s.HTML)
	switch {
	case src == "":
		p.Add("html", "is required: the figure's body as HTML")
	case len(src) > MaxHTMLBytes:
		p.Add("html", "is %d bytes; at most %d. Show less, or split it", len(src), MaxHTMLBytes)
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	pg, err := cleanHTML(src, &p)
	if err != nil {
		p.Add("html", "%s", err.Error())
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	return pg, nil
}

// cleanHTML parses the model's HTML as (forgiving) XML with HTML's
// entities and void elements, and writes it again from the allow-list.
// Anything outside it is refused, not dropped, so the model knows.
func cleanHTML(src string, p *core.Problems) (*htmlPage, error) {
	d := xml.NewDecoder(strings.NewReader("<root>" + src + "</root>"))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	d.CharsetReader = func(label string, r io.Reader) (io.Reader, error) { return nil, errors.New("declare no encoding") }
	pg := &htmlPage{}
	var out, style strings.Builder
	var stack []string
	count := 0
	refused := map[string]bool{}
	refuse := func(key, msg string, args ...any) {
		if !refused[key] {
			refused[key] = true
			p.Add("html", msg, args...)
		}
	}
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("could not be read: %s; write well-formed HTML, closing every tag", strings.TrimPrefix(err.Error(), "XML syntax error on "))
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if name == "root" && len(stack) == 0 {
				stack = append(stack, name)
				continue
			}
			if t.Name.Space != "" {
				refuse("ns:"+name, "<%s:%s> is not allowed; use plain HTML elements", t.Name.Space, name)
			}
			count++
			if count > MaxHTMLElements {
				return nil, fmt.Errorf("has more than %d elements; show less", MaxHTMLElements)
			}
			if len(stack) > MaxHTMLDepth {
				return nil, fmt.Errorf("nests elements more than %d deep", MaxHTMLDepth)
			}
			stack = append(stack, name)
			if !htmlElements[name] {
				hint := htmlHints[name]
				if hint == "" {
					hint = "see the allowed elements"
				}
				refuse("el:"+name, "<%s> is not allowed: %s", name, hint)
				continue
			}
			if name == "style" {
				continue
			}
			out.WriteString("<" + name)
			for _, a := range t.Attr {
				an := strings.ToLower(a.Name.Local)
				if a.Name.Space != "" && a.Name.Space != "xmlns" {
					refuse("attr:"+an, "the attribute %s:%s is not allowed", a.Name.Space, an)
					continue
				}
				if strings.HasPrefix(an, "on") {
					refuse("on", "event attributes (%s) are not allowed; figures run nothing", an)
					continue
				}
				if !htmlAttrs[an] && !strings.HasPrefix(an, "aria-") {
					refuse("attr:"+an, "the attribute %s is not allowed (allowed: class, id, style, colspan, rowspan, scope, title, role, lang, dir, aria-*)", an)
					continue
				}
				if an == "style" {
					if m := badCSS.FindString(a.Value); m != "" {
						refuse("css:"+m, "the CSS %q is not allowed: figures load nothing and use no escapes", m)
						continue
					}
					if literalCSS.MatchString(a.Value) {
						pg.literal = true
					}
				}
				out.WriteString(" " + an + `="` + core.Escape(a.Value) + `"`)
			}
			out.WriteString(">")
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if len(stack) == 0 {
				continue
			}
			stack = stack[:len(stack)-1]
			if name == "root" && len(stack) == 0 {
				continue
			}
			if htmlElements[name] && name != "style" && !voidElements[name] {
				out.WriteString("</" + name + ">")
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "style" {
				css := string(t)
				if m := badCSS.FindString(css); m != "" {
					refuse("css:"+m, "the CSS %q is not allowed: figures load nothing and use no escapes", m)
					continue
				}
				if literalCSS.MatchString(css) {
					pg.literal = true
				}
				style.WriteString(css)
				continue
			}
			if len(stack) > 0 && htmlElements[stack[len(stack)-1]] || len(stack) == 1 {
				out.WriteString(core.Escape(string(t)))
			}
		case xml.Directive:
			refuse("doctype", "give only the body's content, with no <!DOCTYPE>")
		}
	}
	if strings.TrimSpace(stripTags(out.String())) == "" {
		p.Add("html", "shows no text; a figure needs content")
	}
	pg.body, pg.style = out.String(), style.String()
	return pg, nil
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

func (h *htmlPage) Scripted() bool { return false }

// HTML is the page. A figure with literal colours keeps the light theme in
// the dark one, on its light card, since its colours were chosen for it.
func (h *htmlPage) HTML(t *core.Theme, title, caption string) string {
	if h.literal {
		t = &core.Light
	}
	return shell(t, cspStatic, title, caption, h.style, h.body, "")
}
