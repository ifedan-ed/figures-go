package chart

import (
	"encoding/json"
	"regexp"
	"strconv"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var rectRe = regexp.MustCompile(`<rect x="([-\d.]+)" y="([-\d.]+)" width="([\d.]+)" height="([\d.]+)"[^>]*fill="([^"]+)"`)

// filledRects are the rects drawn in one fill: a chart's bars or boxes.
func filledRects(svg, fill string) []rect {
	var out []rect
	for _, m := range rectRe.FindAllStringSubmatch(svg, -1) {
		if m[5] != fill {
			continue
		}
		v := make([]float64, 4)
		for i := range v {
			v[i], _ = strconv.ParseFloat(m[i+1], 64)
		}
		out = append(out, rect{v[0], v[1], v[0] + v[2], v[1] + v[3]})
	}
	return out
}

// drawnRect is the box a drawn text takes, from its SVG baseline.
func drawnRect(tx drawnText, bold bool) rect {
	a := core.Start
	switch tx.anchor {
	case "middle":
		a = core.Middle
	case "end":
		a = core.End
	}
	return textRect(tx.x, tx.y-tx.size*0.35, tx.s, tx.size, bold, a)
}

// A reference line's label never covers a bar, a box or another label,
// wherever the data crosses the line: at both ends, in either orientation.
// Before, a vertical chart tried only the line's two ends and a horizontal
// one put the label at the top whatever was there, so "No difference" sat
// on Regimen A's bar.
func TestReferenceLabelsKeepClearOfTheData(t *testing.T) {
	cases := map[string]struct {
		kind core.Kind
		spec string
		fill func(*core.Theme) string
	}{
		"horizontal bars crossing the line at the top": {barChart,
			`{"categories":["Regimen A","Regimen B","Regimen C"],"orientation":"horizontal","series":[{"values":[1.53,0.82,1.1],"intervals":[[0.97,2.42],[0.61,1.1],null]}],"value_axis":{"min":0,"max":3},"reference_lines":[{"value":1,"label":"No difference"}]}`,
			func(th *core.Theme) string { return th.SeriesColour(0) }},
		"vertical bars over the line at both ends": {barChart,
			`{"categories":["Jan","Feb","Mar","Apr","May","Jun"],"series":[{"values":[90,40,35,30,45,95]}],"value_axis":{"min":0,"max":100},"reference_lines":[{"value":60,"label":"Target for the year"}]}`,
			func(th *core.Theme) string { return th.SeriesColour(0) }},
		"vertical bars with value labels and two close lines": {barChart,
			`{"categories":["A","B","C","D"],"value_labels":true,"series":[{"values":[80,20,25,85]}],"value_axis":{"min":0,"max":100},"reference_lines":[{"value":50,"label":"Median"},{"value":55,"label":"Goal"}]}`,
			func(th *core.Theme) string { return th.SeriesColour(0) }},
		"boxes over the line": {boxPlot,
			`{"groups":[{"label":"A","min":60,"q1":70,"median":75,"q3":90,"max":95},{"label":"B","min":62,"q1":72,"median":78,"q3":88,"max":96}],"value_axis":{"min":0,"max":100},"reference_lines":[{"value":80,"label":"Cut-off"}]}`,
			func(th *core.Theme) string { return th.SeriesSoftColour(0) }},
	}
	for name, c := range cases {
		d, err := c.kind.Parse(json.RawMessage(c.spec))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		canvas := core.NewCanvas(&core.Light)
		if err := d.Draw(canvas); err != nil {
			t.Fatal(err)
		}
		svg := canvas.Body()
		data := filledRects(svg, c.fill(&core.Light))
		if len(data) == 0 {
			t.Fatalf("%s: no bars or boxes found in\n%s", name, svg)
		}
		// A label with a halo is drawn twice, its halo under it: one box.
		var labels []rect
		seen := map[rect]bool{}
		for _, tx := range texts(svg) {
			switch tx.s {
			case "No difference", "Target for the year", "Median", "Goal", "Cut-off":
				if r := drawnRect(tx, true); !seen[r] {
					seen[r] = true
					labels = append(labels, r)
				}
			}
		}
		if len(labels) == 0 {
			t.Fatalf("%s: no reference label drawn", name)
		}
		for i, l := range labels {
			for _, b := range data {
				if l.overlaps(b) {
					t.Errorf("%s: label %d at %+v covers %+v", name, i, l, b)
				}
			}
			for _, o := range labels[i+1:] {
				if l.overlaps(o) {
					t.Errorf("%s: labels %+v and %+v overlap", name, l, o)
				}
			}
		}
	}
}
