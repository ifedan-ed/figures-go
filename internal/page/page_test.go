package page

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

func raw(k, v string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{k: v})
	return b
}

func TestHTMLRefusesWithTheReason(t *testing.T) {
	cases := map[string]struct{ html, want string }{
		"script":       {`<p>x</p><script>alert(1)</script>`, "<script> is not allowed"},
		"img":          {`<p>x</p><img src="http://x/a.png"/>`, "<img> is not allowed"},
		"iframe":       {`<iframe src="http://x"></iframe><p>x</p>`, "<iframe> is not allowed"},
		"link":         {`<a href="http://x">x</a>`, "<a> is not allowed"},
		"svg":          {`<svg><rect/></svg><p>x</p>`, "use type svg"},
		"event":        {`<p onclick="x()">x</p>`, "event attributes"},
		"css url":      {`<p style="background:url(http://x)">x</p>`, "url("},
		"css import":   {`<style>@import "x.css";</style><p>x</p>`, "@import"},
		"css escape":   {`<p style="background:u\72l(x)">x</p>`, `\`},
		"font-face":    {`<style>@font-face{font-family:x}</style><p>x</p>`, "@font-face"},
		"fixed":        {`<p style="position: fixed">x</p>`, "position: fixed"},
		"src attr":     {`<p src="x">x</p>`, "attribute src"},
		"empty":        {``, "is required"},
		"no text":      {`<div></div>`, "no text"},
		"h1":           {`<h1>x</h1>`, "title is drawn for you"},
		"style breaks": {`<style></style><p>x</style><script>x</script></p>`, "could not be read"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseHTML(raw("html", c.html))
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

func TestHTMLIsRewrittenFromTheAllowList(t *testing.T) {
	pg, err := parseHTML(raw("html", `<div class="c" style="color:var(--bad)">A &amp; B &nbsp;<br>next<hr><table><tr><td colspan="2">x</td></tr></table></div>`))
	if err != nil {
		t.Fatal(err)
	}
	page := pg.HTML(&core.Dark, "Title <b>", "Cap")
	for _, want := range []string{`<div class="c" style="color:var(--bad)">A &amp; B `, `<br>next<hr>`, `<td colspan="2">x</td>`, "Title &lt;b&gt;", "--ink:" + core.Dark.Ink, "default-src 'none'"} {
		if !strings.Contains(page, want) {
			t.Fatalf("%q not in %s", want, page)
		}
	}
	if strings.Contains(page, "<script") {
		t.Fatal("a static page has a script")
	}
}

func TestHTMLLiteralColoursKeepTheLightTheme(t *testing.T) {
	pg, err := parseHTML(raw("html", `<p style="color:#ff0000">x</p>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pg.HTML(&core.Dark, "t", ""), "--bg:"+core.Light.Bg) {
		t.Fatal("dark page for a literal-colour figure is not light")
	}
}

func TestMermaidRefuses(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"empty":     {"", "is required"},
		"no type":   {"A --> B", "must start with its diagram type"},
		"directive": {"%%{init: {'securityLevel': 'loose'}}%%\nflowchart TD\nA-->B", "%%{"},
		"click":     {"flowchart TD\nA-->B\nclick A callback", "click"},
		"href":      {"flowchart TD\nA-->B\nclick A href \"http://x\"", "not allowed"},
		"html":      {"flowchart TD\nA[\"<img src=x>\"]-->B", "not allowed"},
		"style":     {"flowchart TD\nA-->B\nstyle A fill:#f00", "leave out style"},
		"classDef":  {"flowchart TD\nA-->B\nclassDef x fill:#f00", "leave out style"},
		"too long":  {"flowchart TD\n" + strings.Repeat("A-->B\n", MaxMermaidLines), "lines"},
		"too big":   {"flowchart TD\n" + strings.Repeat("A", MaxMermaidBytes), "bytes"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseMermaid(raw("mermaid", c.src))
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

func TestMermaidPageCarriesTheSourceAsData(t *testing.T) {
	pg, err := parseMermaid(raw("mermaid", "flowchart TD\n  A[\"Fever ≥ 38 °C\"] --> B[Done] %% </script>"))
	if err == nil {
		t.Fatal("a closing script tag in the source was accepted")
	}
	pg, err = parseMermaid(raw("mermaid", "flowchart TD\n  A[\"Fever ≥ 38 °C\"] --> B[Done]"))
	if err != nil {
		t.Fatal(err)
	}
	page := pg.HTML(&core.Light, "t", "")
	if !strings.Contains(page, "var src = \"flowchart TD\\n  A[\\\"Fever ≥ 38 °C\\\"] --\\u003e B[Done]\";") || !strings.Contains(page, `securityLevel: "strict"`) {
		t.Fatal("the source is not passed as a JSON string under strict security")
	}
}
