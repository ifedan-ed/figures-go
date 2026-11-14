package freeform

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

func svgSpec(svg string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"svg": svg})
	return b
}

const ok = `<svg viewBox="0 0 400 200"><rect x="10" y="10" width="100" height="50" fill="surface" stroke="edge"/><text x="20" y="40" font-size="13">Label</text><circle cx="200" cy="100" r="20" fill="accent"/></svg>`

func TestRefusesWithTheReason(t *testing.T) {
	cases := map[string]struct{ svg, want string }{
		"empty":           {``, "is required"},
		"no viewBox":      {`<svg><rect/><rect/><rect/></svg>`, "needs a viewBox"},
		"bad viewBox":     {`<svg viewBox="0 0 0 10"><rect/><rect/></svg>`, "four numbers"},
		"script":          {strings.Replace(ok, "<circle", "<script>alert(1)</script><circle", 1), "<script> is not allowed"},
		"style element":   {strings.Replace(ok, "<circle", "<style>rect{fill:red}</style><circle", 1), "not CSS"},
		"style attribute": {strings.Replace(ok, `fill="accent"`, `style="fill:red"`, 1), "attribute style"},
		"class":           {strings.Replace(ok, `fill="accent"`, `class="x"`, 1), "classes"},
		"event":           {strings.Replace(ok, `fill="accent"`, `onclick="x()"`, 1), "event attributes"},
		"foreignObject":   {strings.Replace(ok, "<circle", "<foreignObject><div>x</div></foreignObject><circle", 1), "HTML cannot be embedded"},
		"image":           {strings.Replace(ok, "<circle", `<image href="http://x/a.png"/><circle`, 1), "pictures cannot be embedded"},
		"use":             {strings.Replace(ok, "<circle", `<use href="#a"/><circle`, 1), "<use> is not allowed"},
		"xlink":           {strings.Replace(ok, `<svg `, `<svg xmlns:xlink="http://www.w3.org/1999/xlink" `, 1) + "", ""},
		"external url":    {strings.Replace(ok, `fill="accent"`, `fill="url(http://evil/x#a)"`, 1), "only to an id inside"},
		"missing id":      {strings.Replace(ok, `fill="accent"`, `fill="url(#nope)"`, 1), "url(#nope)"},
		"unknown colour":  {strings.Replace(ok, `fill="accent"`, `fill="brand-blue"`, 1), "not one this figure knows"},
		"tiny text":       {strings.Replace(ok, `font-size="13"`, `font-size="4"`, 1), "too small"},
		"px font":         {strings.Replace(ok, `font-size="13"`, `font-size="13px"`, 1), "plain number"},
		"not xml":         {`<svg viewBox="0 0 400 200"><rect>`, "not well-formed"},
		"doctype":         {`<!DOCTYPE svg><svg viewBox="0 0 400 200"><rect/><rect/></svg>`, "DOCTYPE"},
		"entity":          {`<svg viewBox="0 0 400 200"><text>&xx;</text><rect/></svg>`, "not well-formed"},
		"two roots":       {ok + ok, "more than one root"},
		"too tall":        {`<svg viewBox="0 0 400 1500"><rect/><rect/><rect/></svg>`, "between 0.3 and 3"},
		"too deep":        {`<svg viewBox="0 0 400 200">` + strings.Repeat("<g>", 20) + strings.Repeat("</g>", 20) + `</svg>`, "deep"},
		"too many":        {`<svg viewBox="0 0 400 200">` + strings.Repeat("<rect/>", MaxElements+1) + `</svg>`, "more than 800 elements"},
		"html root":       {`<html><svg/></html>`, "must start with <svg>"},
		"javascript":      {strings.Replace(ok, `fill="accent"`, `fill="javascript:alert(1)"`, 1), "not allowed"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(svgSpec(c.svg))
			if c.want == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var r *core.Refusal
			if !errors.As(err, &r) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%q does not say %q", err.Error(), c.want)
			}
		})
	}
}

func TestDrawsInsideTheFrameWithThemeColours(t *testing.T) {
	d, err := parse(svgSpec(`<svg viewBox="100 50 800 400"><defs><marker id="m" viewBox="0 0 10 10" refX="5" refY="5"><path d="M0 0L10 5L0 10z" fill="ink"/></marker></defs><line x1="0" y1="0" x2="10" y2="10" stroke="series2" marker-end="url(#m)"/><text x="120" y="100" font-size="28">A &amp; B</text></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range []*core.Theme{&core.Light, &core.Dark} {
		c := core.NewCanvas(th)
		if err := d.Draw(c); err != nil {
			t.Fatal(err)
		}
		body := c.Body()
		for _, want := range []string{`id="ff-m"`, `url(#ff-m)`, `stroke="` + th.Series[1] + `"`, `fill="` + th.Ink + `"`, "A &amp; B", "scale(0.465", "translate(-100 -50)"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: %q not in %s", th.Name, want, body)
			}
		}
		if c.H != 400*0.465 {
			t.Fatalf("height %v", c.H)
		}
	}
}

func TestLiteralColoursShowOnALightCardInDark(t *testing.T) {
	d, err := parse(svgSpec(strings.Replace(ok, `fill="accent"`, `fill="#ff0000"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	c := core.NewCanvas(&core.Dark)
	_ = d.Draw(c)
	if !strings.Contains(c.Body(), `fill="`+core.Light.Bg+`"`) || !strings.Contains(c.Body(), `fill="`+core.Light.Surface+`"`) {
		t.Fatal("a literal-colour drawing is not on a light card in the dark theme")
	}
}
