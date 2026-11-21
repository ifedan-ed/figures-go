# figures-go

    go get github.com/ifedan-ed/figures-go

Figures for a model's answers, drawn by trusted code: the model asks for a
figure with `draw_figure` (`figures.Tool()`), `figures.Render` checks the
arguments and refuses in words the model can correct from, and the figure is
drawn as a sanitised SVG in a light and a dark version. `Rasteriser` makes
the PNGs the apps show.

- Typed figures (drawn here): line, bar, scatter, area and box charts;
  growth (WHO/CDC LMS), blood pressure (AAP 2017), zone and dose-response
  charts; function plots; flowcharts, pathways, timelines, cycles,
  comparison tables, concept maps, Venn diagrams and schematics.
- Bar charts take error bars as symmetric half-widths (`errors`) or as each
  bar's own low and high (`intervals`), so an uneven interval such as a
  ratio's 95% CI is drawn where it lies.
- Free-form, written by the model and checked as strictly: `svg`, `html`,
  `mermaid`. Anatomy is drawn this way (or as a diagram): the built-in
  illustration library (heart, nephron, airway, brain lobes, gut, skin, cell,
  ECG complex) was removed on 2026-11-09. A figure already kept with an
  answer (type `illustration`, form `svg`) still shows: `Rasteriser.Show`
  draws a kept figure from its stored SVG, not from its spec.

Text limits are checked in characters and told to the model in words
(`core.Words`, seven characters a word, rounded down to a multiple of five
from ten), in the tool's description and schema: a model cannot count
characters, and was refused for a box of 121 where 120 fit. A text over its
limit is refused with its length and the words to keep to, and its start
quoted; it is never cut.

A word that does not fit its box is kept whole first. A flowchart or
pathway row shares its width by what each box's text needs, taking room
from boxes that can spare it; then, step by step, boxes of a kind may
differ in width, the gaps and padding close up, and the text is set
smaller, down to the floor. A table column is given the width its longest
word needs from the columns that can spare it, at a smaller size or split
at its own hyphens if it must. A note beside a crowded row gives up width
before a box's word is cut. Only then is a word
broken, the way a typesetter breaks it: at a hyphen, dash or slash it
already has ("Life-" / "threatening"), else inside it with a hyphen, never
leaving fewer than three letters on a side or adding a hyphen next to its
own. A word none of that can place is refused, named.

A flowchart decision branches by edges labelled with their answers (Yes
and No, Low / Intermediate / High, "> 38 °C"); `yes` and `no` on a
decision node are still read as a shorthand for two such edges. Three or
more branches fan out from the decision's bottom, each line on its own
track into its own box with its label just above that box.

Outside the apps' light and dark pages (a slide deck, a Word file),
`Rasteriser.Picture` draws one PNG in a `Look`: a host's `Palette`
(background and text required; accent, data colours and fills derived and
held to contrast, the tone and tissue colours kept) and `Untitled` when the
host names the figure itself, as a slide's heading does. `DrawSVG` is the
same as one SVG.

PNG: SVG through resvg 0.48.1; HTML and Mermaid through a pinned
Gotenberg 8.38.1 (Chromium) from a page that can load nothing. Fonts:
`fonts/` (see fonts/README). Mermaid 11.16.1 is embedded (internal/page).

    go test ./...
    FIGURES_TEST_RESVG=/path/resvg FIGURES_TEST_FONTS=$PWD/fonts \
      FIGURES_TEST_CHROMIUM=http://127.0.0.1:18930 go test .   # PNG tests
    go run ./cmd/preview -out /tmp/fig -resvg /path/resvg -fonts fonts -chromium http://127.0.0.1:18930

## Licence

Apache License 2.0 (LICENSE). The fonts and the embedded Mermaid keep their
own licences; see NOTICE.
