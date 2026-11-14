// Package freeform is the one figure type the model draws itself: an SVG
// it writes, for what no typed figure covers (a mechanism, an anatomy
// sketch the library lacks, an odd diagram). Nothing in it is trusted: it
// is parsed strictly and refused, with the reason, for anything outside a
// small allow-list, then rebuilt element by element inside the figure's
// own frame. Data and clinical charts stay typed, where exact numbers
// matter and the code, not the model, places them.
package freeform

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// Limits on what the model may write.
const (
	MaxBytes    = 40 << 10
	MaxElements = 800
	MaxDepth    = 16
	MaxText     = 4000
	// MinTextSize is the smallest text allowed, in figure units once the
	// drawing is scaled to the figure's width: 10 is about 8.5 px on a
	// phone, the least that can be read.
	MinTextSize = 10.0
)

const pad = 14.0

// Kinds is the free-form type.
func Kinds() []core.Kind {
	return []core.Kind{{
		Name:   "svg",
		Family: "free-form",
		Use: "Your own SVG drawing, for what no other type covers: a mechanism (how a drug blocks a channel), an anatomy sketch, an unusual diagram. " +
			"Never for data, numbers or clinical thresholds: use a chart type, where the app places every value exactly. Prefer a typed diagram when one fits; they are laid out for you.",
		Fields: "svg: one complete <svg> element, at most 40 KB. It must have a viewBox; draw for viewBox=\"0 0 400 H\" (H 150 to 1000) and it is shown at the figure's width. " +
			"Allowed elements: g, path, rect, circle, ellipse, line, polyline, polygon, text, tspan, defs, linearGradient, radialGradient, stop, clipPath, marker, title, desc. " +
			"Not allowed: style elements or style/class attributes (use attributes: fill, stroke, stroke-width, font-size, ...), script, foreignObject, image, use, a, filter, mask, pattern, animation, event attributes, href, external url(). " +
			"url(#id) may point only to an id in your drawing (a marker for arrowheads, a gradient). At most 800 elements, nesting depth 16. " +
			"Text: font-size at least 12 at width 400; keep labels short (" + core.UpTo(MaxText) + " of text in all); text does not wrap, so break long labels into several <text> lines yourself. " +
			"Colours: give fill and stroke as these names so the figure works on light and dark pages: ink (text, outlines), muted, faint, edge, bg, surface, surface_alt, accent, accent_soft, good, good_soft, warn, warn_soft, bad, bad_soft, series1 to series8 and series1_soft to series8_soft, " +
			"and tissues: flesh, muscle, arterial, venous, nerve, bone, fat, mucosa, fluid; or none. Any other colour (#hex, rgb, a CSS name) is allowed but then the figure is shown on a white card in dark mode. An element with no fill is filled with ink.",
		Example: example,
		Parse:   parse,
	}}
}

const example = `{"type":"svg","title":"How a loop diuretic works","alt":"In the thick ascending limb, furosemide blocks the NKCC2 co-transporter on the luminal side, so sodium, potassium and chloride stay in the tubule and water follows them into the urine.","spec":{"svg":"<svg viewBox=\"0 0 400 240\"><defs><marker id=\"a\" viewBox=\"0 0 10 10\" refX=\"9\" refY=\"5\" markerWidth=\"6\" markerHeight=\"6\" orient=\"auto\"><path d=\"M0 0L10 5L0 10z\" fill=\"ink\"/></marker></defs><rect x=\"8\" y=\"30\" width=\"112\" height=\"196\" rx=\"10\" fill=\"fluid\"/><text x=\"64\" y=\"52\" font-size=\"13\" font-weight=\"700\" text-anchor=\"middle\">Tubule lumen</text><rect x=\"130\" y=\"30\" width=\"150\" height=\"196\" rx=\"14\" fill=\"surface\" stroke=\"edge\"/><text x=\"205\" y=\"52\" font-size=\"13\" font-weight=\"700\" text-anchor=\"middle\">TAL cell</text><rect x=\"290\" y=\"30\" width=\"102\" height=\"196\" rx=\"10\" fill=\"bad_soft\"/><text x=\"341\" y=\"52\" font-size=\"13\" font-weight=\"700\" text-anchor=\"middle\">Blood</text><text x=\"64\" y=\"104\" font-size=\"12\" text-anchor=\"middle\">Na⁺ K⁺ 2Cl⁻</text><line x1=\"22\" y1=\"122\" x2=\"106\" y2=\"122\" stroke=\"ink\" stroke-width=\"2\" marker-end=\"url(#a)\"/><circle cx=\"130\" cy=\"122\" r=\"18\" fill=\"series1_soft\" stroke=\"series1\" stroke-width=\"2.5\"/><path d=\"M117 109L143 135M143 109L117 135\" stroke=\"bad\" stroke-width=\"4\" stroke-linecap=\"round\"/><text x=\"130\" y=\"158\" font-size=\"12\" text-anchor=\"middle\">NKCC2</text><line x1=\"152\" y1=\"122\" x2=\"282\" y2=\"122\" stroke=\"muted\" stroke-width=\"2\" stroke-dasharray=\"5 4\" marker-end=\"url(#a)\"/><text x=\"215\" y=\"96\" font-size=\"13\" font-weight=\"700\" fill=\"bad\" text-anchor=\"middle\">Furosemide</text><text x=\"215\" y=\"112\" font-size=\"12\" fill=\"bad\" text-anchor=\"middle\">blocks uptake</text><text x=\"64\" y=\"194\" font-size=\"12\" text-anchor=\"middle\">Na⁺ and water</text><text x=\"64\" y=\"210\" font-size=\"12\" text-anchor=\"middle\">stay in the urine</text></svg>"}}`

var (
	elements = map[string]bool{
		"g": true, "path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
		"text": true, "tspan": true, "defs": true, "linearGradient": true, "radialGradient": true, "stop": true,
		"clipPath": true, "marker": true, "title": true, "desc": true,
	}
	attrs = map[string]bool{
		"x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true,
		"fx": true, "fy": true, "width": true, "height": true, "d": true, "points": true, "transform": true, "dx": true, "dy": true,
		"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-opacity": true,
		"stroke-dasharray": true, "stroke-linecap": true, "stroke-linejoin": true, "opacity": true,
		"font-size": true, "font-weight": true, "font-style": true, "text-anchor": true, "dominant-baseline": true, "letter-spacing": true,
		"id": true, "offset": true, "stop-color": true, "stop-opacity": true, "gradientUnits": true, "gradientTransform": true,
		"clip-path": true, "clipPathUnits": true, "viewBox": true, "preserveAspectRatio": true,
		"markerWidth": true, "markerHeight": true, "refX": true, "refY": true, "orient": true, "markerUnits": true,
		"marker-start": true, "marker-mid": true, "marker-end": true,
	}
	// hints say why a common element or attribute is refused and what to
	// write instead.
	hints = map[string]string{
		"style":         "set colours and sizes with attributes (fill, stroke, font-size), not CSS",
		"class":         "set colours and sizes with attributes (fill, stroke, font-size), not CSS classes",
		"script":        "scripts never run in a figure",
		"foreignObject": "HTML cannot be embedded; draw with text and shapes",
		"image":         "pictures cannot be embedded; draw it with shapes",
		"use":           "repeat the shape itself instead of <use>",
		"a":             "figures have no links",
		"filter":        "filters (shadows, blur) are not shown in the apps; leave them out",
		"mask":          "masks are not supported; use clipPath or draw the shape directly",
		"pattern":       "patterns are not supported; use a plain fill",
		"symbol":        "draw the shape directly instead of a symbol",
		"href":          "nothing may be linked or referenced by href",
		"font-family":   "the figure's font is set for you; leave font-family out",
	}
	colourAttrs = map[string]bool{"fill": true, "stroke": true, "stop-color": true}
	urlRe       = regexp.MustCompile(`url\(\s*['"]?#([A-Za-z_][\w.-]*)['"]?\s*\)`)
	idRe        = regexp.MustCompile(`^[A-Za-z_][\w.-]{0,40}$`)
)

// tokens are the colour names a drawing may use, resolved per theme.
func tokens(t *core.Theme) map[string]string {
	m := map[string]string{
		"ink": t.Ink, "muted": t.Muted, "faint": t.Faint, "edge": t.Edge, "bg": t.Bg, "surface": t.Surface, "surface_alt": t.SurfaceAlt,
		"accent": t.Accent, "accent_soft": t.AccentSoft, "good": t.Good, "good_soft": t.GoodSoft, "warn": t.Warn, "warn_soft": t.WarnSoft,
		"bad": t.Bad, "bad_soft": t.BadSoft, "flesh": t.Flesh, "muscle": t.Muscle, "arterial": t.Arterial, "venous": t.Venous,
		"nerve": t.Nerve, "bone": t.Bone, "fat": t.Fat, "mucosa": t.Mucosa, "fluid": t.Fluid,
	}
	for i := range t.Series {
		m[fmt.Sprintf("series%d", i+1)] = t.Series[i]
		m[fmt.Sprintf("series%d_soft", i+1)] = t.SeriesSoft[i]
	}
	return m
}

var tokenNames = tokens(&core.Light)

type spec struct {
	SVG string `json:"svg"`
}

// node is one element of the model's drawing, checked.
type node struct {
	name  string
	attrs []xml.Attr
	kids  []any // *node or string (text)
}

type drawing struct {
	root          *node
	minX, minY    float64
	vbW, vbH      float64
	literalColour bool
}

func parse(raw json.RawMessage) (core.Drawing, error) {
	var s spec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	src := strings.TrimSpace(s.SVG)
	switch {
	case src == "":
		p.Add("svg", "is required: one complete <svg viewBox=\"0 0 400 H\"> element")
	case len(src) > MaxBytes:
		p.Add("svg", "is %d bytes; at most %d. Draw less detail, or use a typed figure", len(src), MaxBytes)
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	d, err := read(src, &p)
	if err != nil {
		p.Add("svg", "%s", err.Error())
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

// read parses and checks the drawing, recording every problem.
func read(src string, p *core.Problems) (*drawing, error) {
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = true
	dec.Entity = map[string]string{}
	dec.CharsetReader = func(label string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "utf-8") {
			return r, nil
		}
		return nil, fmt.Errorf("encoding %q is not allowed; write UTF-8", label)
	}
	var stack []*node
	var root *node
	ids := map[string]bool{}
	var refs []string
	count, textChars := 0, 0
	d := &drawing{}
	// sizes is the font-size in force at each depth, for the legibility
	// check; SVG's default is 16.
	sizes := []float64{16}
	type smallText struct{ size float64 }
	var small []smallText
	refused := map[string]bool{}
	refuse := func(what, msg string) {
		if !refused[what] {
			refused[what] = true
			p.Add("svg", "%s", msg)
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("is not well-formed XML: %s", strings.TrimPrefix(err.Error(), "XML syntax error on "))
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				return nil, fmt.Errorf("has the element <%s:%s> from another namespace; only SVG elements are allowed", t.Name.Space, name)
			}
			if root != nil && len(stack) == 0 {
				return nil, errors.New("has more than one root element; give exactly one <svg>")
			}
			if len(stack) >= MaxDepth {
				return nil, fmt.Errorf("nests elements more than %d deep", MaxDepth)
			}
			count++
			if count > MaxElements {
				return nil, fmt.Errorf("has more than %d elements; simplify the drawing", MaxElements)
			}
			n := &node{name: name}
			if root == nil {
				if name != "svg" {
					return nil, fmt.Errorf("must start with <svg>, not <%s>", name)
				}
				root = n
				for _, a := range t.Attr {
					if a.Name.Local == "viewBox" && a.Name.Space == "" {
						vb, ok := viewBox(a.Value)
						if !ok {
							p.Add("svg", "viewBox %q must be four numbers, min-x min-y width height, with width and height above 0", a.Value)
						} else {
							d.minX, d.minY, d.vbW, d.vbH = vb[0], vb[1], vb[2], vb[3]
						}
					}
				}
				if d.vbW == 0 {
					if !hasAttr(t.Attr, "viewBox") {
						p.Add("svg", "the <svg> needs a viewBox, such as viewBox=\"0 0 400 300\"")
					}
				} else if r := d.vbH / d.vbW; r < 0.3 || r > 3 {
					p.Add("svg", "the viewBox is %s×%s; keep height between 0.3 and 3 times the width so it reads on a phone", core.F(d.vbW), core.F(d.vbH))
				}
			} else {
				if !elements[name] {
					hint := hints[name]
					if hint == "" {
						hint = "allowed elements are g, path, rect, circle, ellipse, line, polyline, polygon, text, tspan, defs, linearGradient, radialGradient, stop, clipPath, marker, title, desc"
					}
					refuse("el:"+name, fmt.Sprintf("<%s> is not allowed: %s", name, hint))
				}
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			}
			size := sizes[len(sizes)-1]
			for _, a := range t.Attr {
				local := a.Name.Local
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && local == "xmlns") {
					continue // namespace declarations; the output declares its own
				}
				if n == root {
					// The root's own size and placement are the figure's.
					if local == "viewBox" || local == "width" || local == "height" || local == "version" || local == "preserveAspectRatio" {
						continue
					}
				}
				if a.Name.Space != "" {
					refuse("attr:"+local, fmt.Sprintf("the attribute %s:%s is not allowed; %s", a.Name.Space, local, or(hints[local], "use plain SVG attributes")))
					continue
				}
				if strings.HasPrefix(strings.ToLower(local), "on") {
					refuse("on", "event attributes ("+local+") are not allowed; figures run nothing")
					continue
				}
				if !attrs[local] {
					refuse("attr:"+local, fmt.Sprintf("the attribute %s is not allowed: %s", local, or(hints[local], "use geometry and presentation attributes only")))
					continue
				}
				v := a.Value
				low := strings.ToLower(v)
				if strings.Contains(low, "javascript:") || strings.ContainsAny(v, `\<`) || strings.Contains(low, "expression(") {
					refuse("val:"+local, fmt.Sprintf("the attribute %s has a value that is not allowed (%q)", local, clip(v)))
					continue
				}
				if strings.Contains(low, "url(") {
					all := strings.Count(low, "url(")
					m := urlRe.FindAllStringSubmatch(v, -1)
					if len(m) != all {
						refuse("url", "url() may point only to an id inside the drawing, as url(#arrow)")
						continue
					}
					for _, mm := range m {
						refs = append(refs, mm[1])
					}
				}
				if local == "id" {
					if !idRe.MatchString(v) {
						p.Add("svg", "id %q must be a letter followed by letters, digits, - or _ (at most 40)", clip(v))
					} else if ids[v] {
						p.Add("svg", "id %q is used twice", v)
					}
					ids[v] = true
				}
				if colourAttrs[local] {
					if _, ok := tokenNames[v]; !ok && !strings.HasPrefix(low, "url(") && low != "none" && low != "transparent" {
						if low == "currentcolor" || low == "inherit" {
							v = "ink"
						} else if !literalColour(low) {
							refuse("col:"+v, fmt.Sprintf("the colour %q is not one this figure knows; use a name such as ink, muted, accent, series1 or bad", clip(v)))
							continue
						} else {
							d.literalColour = true
						}
					}
				}
				if local == "font-size" {
					if f, ok := number(v); ok && f > 0 {
						size = f
					} else {
						p.Add("svg", "font-size %q must be a plain number (user units, no px or em)", clip(v))
					}
				}
				n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: local}, Value: v})
			}
			sizes = append(sizes, size)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			sizes = sizes[:len(sizes)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if len(bytes.TrimSpace(t)) > 0 {
					return nil, errors.New("has text outside the <svg> element")
				}
				continue
			}
			top := stack[len(stack)-1]
			if top.name != "text" && top.name != "tspan" && top.name != "title" && top.name != "desc" {
				continue
			}
			text := string(t)
			if strings.TrimSpace(text) == "" && top.name != "tspan" && top.name != "text" {
				continue
			}
			textChars += len([]rune(text))
			top.kids = append(top.kids, text)
			if (top.name == "text" || top.name == "tspan") && strings.TrimSpace(text) != "" {
				small = append(small, smallText{sizes[len(sizes)-1]})
			}
		case xml.Directive:
			return nil, errors.New("has a <!DOCTYPE> or other declaration; give only the <svg> element")
		}
	}
	if root == nil {
		return nil, errors.New("has no <svg> element")
	}
	if len(stack) != 0 {
		return nil, errors.New("is not complete: an element is not closed")
	}
	if textChars > MaxText {
		p.Add("svg", "has about %d words of text; keep it to %d or fewer. Labels should be short; explain in the answer", textChars/core.CharsPerWord, core.Words(MaxText))
	}
	for _, r := range refs {
		if !ids[r] {
			p.Add("svg", "url(#%s) points to an id the drawing does not define", r)
		}
	}
	if count < 3 {
		p.Add("svg", "draws almost nothing; a figure needs shapes and labels")
	}
	if d.vbW > 0 {
		scale := (core.Width - 2*pad) / d.vbW
		least := math.Inf(1)
		for _, s := range small {
			least = math.Min(least, s.size)
		}
		if least*scale < MinTextSize {
			need := math.Ceil(MinTextSize / scale)
			p.Add("svg", "text at font-size %s is too small to read on a phone at this viewBox width (%s); use font-size %s or more, or a narrower viewBox",
				core.F(least), core.F(d.vbW), core.F(need))
		}
		if d.vbH*scale > core.MaxHeight-120 {
			p.Add("svg", "is too tall to follow on a phone once scaled to the figure's width; keep the viewBox height under %s for width %s",
				core.F((core.MaxHeight-120)/scale), core.F(d.vbW))
		}
	}
	d.root = root
	return d, nil
}

// literalColour says whether v is a colour value a browser and resvg both
// read: #rgb, #rrggbb, rgb()/rgba() of numbers, or a plain CSS name.
func literalColour(v string) bool {
	switch {
	case regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$`).MatchString(v):
		return true
	case regexp.MustCompile(`^rgba?\(\s*[\d.%]+\s*,\s*[\d.%]+\s*,\s*[\d.%]+\s*(,\s*[\d.%]+\s*)?\)$`).MatchString(v):
		return true
	case regexp.MustCompile(`^[a-z]{3,20}$`).MatchString(v):
		return cssNames[v]
	}
	return false
}

var cssNames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields("black white red green blue yellow orange purple pink brown gray grey silver gold navy teal maroon olive lime aqua cyan magenta fuchsia indigo violet crimson coral salmon tomato khaki beige ivory lavender plum orchid tan chocolate sienna peru darkred darkgreen darkblue darkgray darkgrey lightgray lightgrey lightblue lightgreen lightyellow lightpink skyblue steelblue royalblue slategray slategrey dimgray dimgrey firebrick forestgreen seagreen turquoise wheat linen mistyrose honeydew aliceblue whitesmoke gainsboro") {
		m[n] = true
	}
	return m
}()

func viewBox(v string) ([4]float64, bool) {
	var out [4]float64
	f := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' })
	if len(f) != 4 {
		return out, false
	}
	for i, s := range f {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e6 {
			return out, false
		}
		out[i] = n
	}
	return out, out[2] > 0 && out[3] > 0
}

func number(v string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func hasAttr(a []xml.Attr, name string) bool {
	for _, x := range a {
		if x.Name.Local == name {
			return true
		}
	}
	return false
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func clip(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// Draw rebuilds the checked drawing inside the figure: scaled to the
// figure's width, colour names resolved for the theme, ids prefixed so
// they cannot meet any other figure's on the same page, and clipped to its
// viewBox. A drawing with literal colours is shown on a light card in the
// dark theme, since its colours were chosen for a light page.
func (d *drawing) Draw(c *core.Canvas) error {
	scale := (core.Width - 2*pad) / d.vbW
	h := d.vbH * scale
	t := c.T
	if d.literalColour && t.Name == "dark" {
		t = &core.Light
		c.Rect(pad-6, -4, core.Width-2*pad+12, h+8, 8, core.Light.Bg, "", 0)
	}
	names := tokens(t)
	clip := "ff-" + t.Name + "-clip"
	c.Open("defs")
	c.Open("clipPath", "id", clip)
	c.El("rect", "x", core.F(d.minX), "y", core.F(d.minY), "width", core.F(d.vbW), "height", core.F(d.vbH))
	c.Close("clipPath")
	c.Close("defs")
	c.Open("g", "transform", "translate("+core.F(pad)+" 0) scale("+strconv.FormatFloat(scale, 'f', 5, 64)+") translate("+core.F(-d.minX)+" "+core.F(-d.minY)+")",
		"clip-path", "url(#"+clip+")", "fill", t.Ink)
	for _, k := range d.root.kids {
		write(c, k, names)
	}
	c.Close("g")
	c.H = h
	return nil
}

func write(c *core.Canvas, k any, names map[string]string) {
	switch n := k.(type) {
	case string:
		c.Raw(core.Escape(n))
	case *node:
		attrs := make([]string, 0, 2*len(n.attrs))
		for _, a := range n.attrs {
			v := a.Value
			if colourAttrs[a.Name.Local] {
				if col, ok := names[v]; ok {
					v = col
				}
			}
			if a.Name.Local == "id" {
				v = "ff-" + v
			}
			if strings.Contains(v, "url(") {
				v = urlRe.ReplaceAllString(v, "url(#ff-$1)")
			}
			attrs = append(attrs, a.Name.Local, v)
		}
		if len(n.kids) == 0 && n.name != "text" && n.name != "tspan" {
			c.El(n.name, attrs...)
			return
		}
		c.Open(n.name, attrs...)
		for _, kid := range n.kids {
			write(c, kid, names)
		}
		c.Close(n.name)
	}
}
