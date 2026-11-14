// Command preview draws figures into a directory to look at: each type's
// worked example, or the draw_figure arguments in the files given, as
// light and dark SVG and (with -resvg) PNG, with an index.html of them all.
//
//	go run ./cmd/preview -out /tmp/fig [-resvg /path/to/resvg -fonts dir] [args.json ...]
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"

	figures "github.com/ifedan-ed/figures-go"
)

func main() {
	out := flag.String("out", "preview", "directory to write into")
	bin := flag.String("resvg", "", "resvg binary, to write PNGs too")
	fonts := flag.String("fonts", "", "font directory for resvg")
	chromium := flag.String("chromium", "", "Gotenberg 8 address, for html and mermaid figures")
	only := flag.String("type", "", "only this type's example")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	type item struct{ name, args string }
	var items []item
	if flag.NArg() > 0 {
		for _, p := range flag.Args() {
			b, err := os.ReadFile(p)
			if err != nil {
				log.Fatal(err)
			}
			items = append(items, item{strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), string(b)})
		}
	} else {
		for _, t := range figures.Types() {
			if *only != "" && t != *only {
				continue
			}
			ex, _ := figures.Example(t)
			items = append(items, item{t, ex})
		}
	}
	var r *figures.Rasteriser
	if *bin != "" {
		r = &figures.Rasteriser{Bin: *bin, FontDir: *fonts, Chromium: *chromium, Cache: figures.MemoryCache{}}
	}
	var page strings.Builder
	page.WriteString("<!doctype html><meta charset=utf-8><title>Figures</title><style>body{font:14px system-ui;margin:16px;background:#ddd}figure{display:inline-block;margin:8px;vertical-align:top}img{width:400px;display:block}.d{background:#000}</style>")
	for _, it := range items {
		var f *figures.Figure
		var shown *figures.Shown
		var err error
		if r != nil {
			shown, err = r.Draw(context.Background(), []byte(it.args))
			if shown != nil {
				f = &shown.Figure
			}
		} else {
			f, err = figures.Render([]byte(it.args))
		}
		if err != nil {
			fmt.Printf("%s: %v\n", it.name, err)
			page.WriteString("<p><b>" + html.EscapeString(it.name) + "</b>: " + html.EscapeString(err.Error()) + "</p>")
			continue
		}
		srcs := map[string]string{}
		for theme, svg := range map[string]string{"light": f.SVG, "dark": f.SVGDark} {
			base := filepath.Join(*out, it.name+"."+theme)
			if svg != "" {
				if err := os.WriteFile(base+".svg", []byte(svg), 0o644); err != nil {
					log.Fatal(err)
				}
				srcs[theme] = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
			}
			if shown != nil {
				b64 := shown.PNG
				if theme == "dark" {
					b64 = shown.PNGDark
				}
				png, _ := base64.StdEncoding.DecodeString(b64)
				if err := os.WriteFile(base+".png", png, 0o644); err != nil {
					log.Fatal(err)
				}
				if svg == "" {
					srcs[theme] = "data:image/png;base64," + b64
				}
			}
		}
		fmt.Printf("%s: %s %.0f×%.0f, %d bytes\n", it.name, f.Form, f.Width, f.Height, len(f.SVG))
		page.WriteString("<figure><figcaption>" + html.EscapeString(it.name) + "</figcaption><img src=\"" + srcs["light"] + "\"><img class=d src=\"" + srcs["dark"] + "\"></figure>")
	}
	if err := os.WriteFile(filepath.Join(*out, "index.html"), []byte(page.String()), 0o644); err != nil {
		log.Fatal(err)
	}
}
