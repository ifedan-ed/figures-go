package figures

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Sanitise keeps only drawing: an allow-list of elements and attributes,
// rebuilt from scratch. Scripts, event handlers, foreign objects, CSS, images, links
// and anything pointing outside the picture are dropped. Render's own
// output passes through unchanged, which the tests check for every
// example; it is here so that an SVG kept with a message is safe however
// it is shown later, and so a saved one can be checked again.
func Sanitise(in []byte) ([]byte, error) {
	if len(in) > MaxSVGBytes {
		return nil, fmt.Errorf("svg: %d bytes, more than %d", len(in), MaxSVGBytes)
	}
	d := xml.NewDecoder(bytes.NewReader(in))
	d.Strict = true
	// No DTDs or entities: an entity is how a "billion laughs" grows.
	d.Entity = map[string]string{}
	d.CharsetReader = func(label string, r io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "utf-8") {
			return r, nil
		}
		return nil, fmt.Errorf("svg: encoding %q is not allowed", label)
	}
	var out bytes.Buffer
	var open []string // the kept elements not yet closed
	depth, skip := 0, 0
	sawRoot, closedRoot := false, false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > 40 {
				return nil, errors.New("svg: nested too deep")
			}
			if closedRoot {
				return nil, errors.New("svg: more than one root element")
			}
			if skip > 0 {
				skip++
				continue
			}
			name := t.Name.Local
			if depth == 1 {
				if name != "svg" {
					return nil, errors.New("svg: the root is not <svg>")
				}
				sawRoot = true
			}
			if !svgElements[name] || (t.Name.Space != "" && t.Name.Space != svgNS) {
				skip = 1
				continue
			}
			out.WriteString("<" + name)
			if depth == 1 {
				out.WriteString(` xmlns="` + svgNS + `"`)
			}
			for _, a := range t.Attr {
				if v, ok := cleanAttr(a); ok {
					out.WriteString(" " + a.Name.Local + `="`)
					_ = xml.EscapeText(&out, []byte(v))
					out.WriteString(`"`)
				}
			}
			out.WriteString(">")
			open = append(open, name)
		case xml.EndElement:
			depth--
			if skip > 0 {
				skip--
				continue
			}
			out.WriteString("</" + t.Name.Local + ">")
			open = open[:len(open)-1]
			if depth == 0 {
				closedRoot = true
			}
		case xml.CharData:
			if skip == 0 && depth > 0 {
				if len(open) == 0 || !textElements[open[len(open)-1]] {
					// Whitespace between elements carries nothing.
					continue
				}
				_ = xml.EscapeText(&out, t)
			}
		}
		// Comments, processing instructions and directives are dropped.
	}
	if !sawRoot {
		return nil, errors.New("svg: no <svg> element")
	}
	return compact(out.Bytes()), nil
}

// tagName is the name in a start tag ("<rect x=..." is rect).
func tagName(tag []byte) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	end := bytes.IndexAny(tag[1:], " >/")
	if end < 0 {
		return ""
	}
	return string(tag[1 : 1+end])
}

// compact turns <x ...></x> into <x .../>, which is how Render writes an
// empty element, so its output passes through byte for byte.
func compact(b []byte) []byte {
	var out bytes.Buffer
	for len(b) > 0 {
		i := bytes.IndexByte(b, '>')
		if i < 0 {
			out.Write(b)
			break
		}
		tag := b[:i+1]
		rest := b[i+1:]
		if len(tag) > 1 && tag[1] != '/' {
			name := tagName(tag)
			closing := []byte("</" + name + ">")
			if name != "" && bytes.HasPrefix(rest, closing) {
				out.Write(tag[:len(tag)-1])
				out.WriteString("/>")
				b = rest[len(closing):]
				continue
			}
		}
		out.Write(tag)
		b = rest
	}
	return out.Bytes()
}

const svgNS = "http://www.w3.org/2000/svg"

var svgElements = map[string]bool{
	"svg": true, "g": true, "path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true, "defs": true,
	"title": true, "desc": true, "linearGradient": true, "radialGradient": true, "stop": true, "clipPath": true,
	"marker": true,
}

var textElements = map[string]bool{"text": true, "tspan": true, "title": true, "desc": true}

var svgAttrs = map[string]bool{
	"viewBox": true, "width": true, "height": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true,
	"cx": true, "cy": true, "r": true, "rx": true, "ry": true, "d": true, "points": true, "transform": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-opacity": true,
	"stroke-dasharray": true, "stroke-linecap": true, "stroke-linejoin": true, "opacity": true,
	"font-family": true, "font-size": true, "font-weight": true, "font-style": true, "text-anchor": true,
	"dx": true, "dy": true, "id": true, "offset": true, "stop-color": true, "stop-opacity": true,
	"clip-path": true, "preserveAspectRatio": true, "role": true, "aria-label": true, "gradientTransform": true,
	"gradientUnits": true, "clipPathUnits": true, "letter-spacing": true, "fx": true, "fy": true,
	"markerWidth": true, "markerHeight": true, "refX": true, "refY": true, "orient": true, "markerUnits": true,
	"marker-start": true, "marker-mid": true, "marker-end": true, "dominant-baseline": true,
}

// cleanAttr keeps an attribute only if it draws and points nowhere outside
// the picture.
func cleanAttr(a xml.Attr) (string, bool) {
	if a.Name.Space != "" || !svgAttrs[a.Name.Local] {
		return "", false
	}
	v := a.Value
	low := strings.ToLower(v)
	// A backslash is a CSS escape, how "u\72l(" would slip a url( past the
	// check below; nothing Render draws needs one.
	if strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") || strings.Contains(low, "@import") ||
		strings.ContainsAny(low, `<\`) {
		return "", false
	}
	// url(...) only to something inside this picture (a gradient, a clip).
	for rest := low; ; {
		i := strings.Index(rest, "url(")
		if i < 0 {
			break
		}
		rest = strings.TrimLeft(rest[i+4:], ` "'`)
		if !strings.HasPrefix(rest, "#") {
			return "", false
		}
	}
	return v, true
}
