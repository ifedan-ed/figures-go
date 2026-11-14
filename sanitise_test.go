package figures

import (
	"strings"
	"testing"
)

func TestSanitiseDropsWhatCouldRunOrLoad(t *testing.T) {
	cases := map[string]struct {
		in      string
		mustNot []string
		must    []string
	}{
		"script":            {`<svg><script>alert(1)</script><rect width="1"/></svg>`, []string{"script", "alert"}, []string{"<rect"}},
		"handler":           {`<svg><rect onload="alert(1)" width="2"/></svg>`, []string{"onload", "alert"}, []string{`width="2"`}},
		"foreign":           {`<svg><foreignObject><div>hi</div></foreignObject><circle r="1"/></svg>`, []string{"foreignObject", "div", "hi"}, []string{"<circle"}},
		"style element":     {`<svg><style>@import url(http://x)</style><rect/></svg>`, []string{"style", "import"}, []string{"<rect"}},
		"style attribute":   {`<svg><rect style="fill:url(http://x)"/></svg>`, []string{"style", "http://x"}, nil},
		"external url":      {`<svg><rect fill="url(http://evil/x.svg#a)"/></svg>`, []string{"evil"}, nil},
		"internal url":      {`<svg><rect fill="url(#g1)"/></svg>`, nil, []string{`fill="url(#g1)"`}},
		"css escape":        {`<svg><rect fill="u\72l(http://x)"/></svg>`, []string{"72l"}, nil},
		"image":             {`<svg><image href="http://x/a.png"/></svg>`, []string{"image", "a.png"}, nil},
		"link":              {`<svg><a href="javascript:alert(1)"><text>x</text></a></svg>`, []string{"<a", "javascript"}, nil},
		"use":               {`<svg><use href="http://x#a"/></svg>`, []string{"use", "http://x"}, nil},
		"xlink":             {`<svg xmlns:xlink="http://www.w3.org/1999/xlink"><rect xlink:href="http://x"/></svg>`, []string{"xlink", "http://x"}, []string{"<rect"}},
		"animate":           {`<svg><rect><set attributeName="fill" to="red"/><animate/></rect></svg>`, []string{"set", "animate"}, []string{"<rect"}},
		"comment and pi":    {`<?xml version="1.0"?><!-- hi --><svg><?php x ?><rect/></svg>`, []string{"hi", "php", "<?"}, []string{"<rect"}},
		"text kept escaped": {`<svg><text>a &amp; b &lt;c&gt;</text></svg>`, nil, []string{"a &amp; b &lt;c&gt;"}},
		"mixed text":        {`<svg><text>a<tspan>b</tspan> c</text></svg>`, nil, []string{"<text>a<tspan>b</tspan> c</text>"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Sanitise([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			for _, bad := range c.mustNot {
				if strings.Contains(s, bad) {
					t.Fatalf("kept %q: %s", bad, s)
				}
			}
			for _, good := range c.must {
				if !strings.Contains(s, good) {
					t.Fatalf("lost %q: %s", good, s)
				}
			}
			if !strings.HasPrefix(s, `<svg xmlns="http://www.w3.org/2000/svg"`) {
				t.Fatalf("the root is not a namespaced svg: %s", s)
			}
		})
	}
}

func TestSanitiseRefuses(t *testing.T) {
	cases := map[string]string{
		"not svg root": `<html><svg/></html>`,
		"entity":       `<!DOCTYPE svg [<!ENTITY a "aaaa">]><svg><text>&a;</text></svg>`,
		"not xml":      `<svg><rect></svg>`,
		"latin1":       `<?xml version="1.0" encoding="ISO-8859-1"?><svg/>`,
		"two roots":    `<svg/><svg/>`,
		"empty":        ``,
		"too deep":     `<svg>` + strings.Repeat("<g>", 50) + strings.Repeat("</g>", 50) + `</svg>`,
		"too large":    `<svg>` + strings.Repeat(" ", MaxSVGBytes) + `</svg>`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if out, err := Sanitise([]byte(in)); err == nil {
				t.Fatalf("accepted: %s", out)
			}
		})
	}
}

func FuzzSanitise(f *testing.F) {
	f.Add([]byte(`<svg><rect onload="x" fill="url(#a)"/><text>hi</text></svg>`))
	f.Add([]byte(`<svg><g><script>x</script></g></svg>`))
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := Sanitise(in)
		if err != nil {
			return
		}
		// What comes out is itself clean: sanitising again changes nothing.
		again, err := Sanitise(out)
		if err != nil || string(again) != string(out) {
			t.Fatalf("not stable: %v\n%s\n%s", err, out, again)
		}
		low := strings.ToLower(string(out))
		for _, bad := range []string{"<script", "javascript:", "<foreignobject", "<style", "<image", "<use", " on"} {
			if strings.Contains(low, bad) && !strings.Contains(low, ">"+bad[1:]) {
				if bad == " on" && !strings.Contains(low, " on") {
					continue
				}
				if bad == " on" {
					// an attribute starting "on" would be " onload="; text may say " on".
					if !hasOnAttr(low) {
						continue
					}
				}
				t.Fatalf("kept %q: %s", bad, out)
			}
		}
	})
}

// hasOnAttr says whether an element in s carries an on… attribute.
func hasOnAttr(s string) bool {
	for _, part := range strings.Split(s, "<") {
		tag, _, _ := strings.Cut(part, ">")
		for _, f := range strings.Fields(tag) {
			if strings.HasPrefix(f, "on") && strings.Contains(f, "=") {
				return true
			}
		}
	}
	return false
}
