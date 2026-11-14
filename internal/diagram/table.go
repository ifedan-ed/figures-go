package diagram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifedan-ed/figures-go/internal/core"
)

var tableKind = core.Kind{
	Name:   "comparison_table",
	Family: "diagram",
	Use:    "Two to four options compared feature by feature (nephritic vs nephrotic syndrome, types of shock), with ticks, crosses and short text; for overlapping features of conditions use venn.",
	Fields: "columns: [option names] (2 to 4, each " + core.UpTo(tbColumnChars) + "). corner?: the first column's heading (" + core.UpTo(tbCornerChars) + "). rows: [{feature (" + core.UpTo(tbFeatureChars) + "), cells: [one per column]}] (1 to 14). A cell is text (" + core.UpTo(tbCellChars[2]) + " in a table of two columns, " + fmt.Sprint(core.Words(tbCellChars[3])) + " in three, " + fmt.Sprint(core.Words(tbCellChars[4])) + " in four) or {text?, mark?: yes|no|partial, tone?}; marks are drawn as a tick, a cross or a wave.",
	Example: `{"type":"comparison_table","title":"Nephritic vs nephrotic syndrome","alt":"Nephritic syndrome has haematuria, hypertension and mild proteinuria; nephrotic syndrome has heavy proteinuria, low albumin and marked oedema.","spec":{"corner":"Feature","columns":["Nephritic","Nephrotic"],"rows":[` +
		`{"feature":"Haematuria","cells":[{"mark":"yes","text":"Red cell casts"},{"mark":"partial","text":"Microscopic, at most"}]},` +
		`{"feature":"Proteinuria","cells":["Mild to moderate",{"text":"Over 40 mg/m²/h","tone":"bad"}]},` +
		`{"feature":"Oedema","cells":["Mild, periorbital",{"text":"Marked, generalised","tone":"bad"}]},` +
		`{"feature":"Hypertension","cells":[{"mark":"yes"},{"mark":"no","text":"Usually normal"}]},` +
		`{"feature":"Serum albumin","cells":["Normal or slightly low",{"text":"Below 25 g/L","tone":"bad"}]},` +
		`{"feature":"Typical cause","cells":["Post-streptococcal GN","Minimal change disease"]}]}}`,
	Parse: parseTable,
}

type tbSpec struct {
	Corner  string   `json:"corner"`
	Columns []string `json:"columns"`
	Rows    []tbRow  `json:"rows"`
}

type tbRow struct {
	Feature string            `json:"feature"`
	Cells   []json.RawMessage `json:"cells"`
}

type tbCell struct {
	Text string `json:"text"`
	Mark string `json:"mark"`
	Tone string `json:"tone"`
}

type table struct {
	corner   textBlock
	heads    []textBlock
	features []textBlock
	cells    [][]tbCell
	texts    [][]textBlock
	colX     []float64 // left edge of each column, the feature column first
	colW     []float64
	rowY     []float64 // top of each row, the header first
	rowH     []float64
	H        float64
}

const (
	tbPadX   = 8.0
	tbPadY   = 8.0
	tbMark   = 8.0 // a mark's radius
	tbSize   = 12.0
	tbHeadSz = 12.5

	tbColumnChars  = 30
	tbCornerChars  = 24
	tbFeatureChars = 40
)

// tbCellChars is the longest a cell's text may be, by the table's columns:
// each wraps in its column, so a cell of a narrower column takes fewer.
// Until 2026-11-10 every cell took 60 characters, which the model, writing
// cells of 61 to 136, passed in a third of its tables; a two-column cell
// is 100 units wide and three-column one 66, so they hold 120 and 90 in
// about as many lines as a four-column cell holds 60.
var tbCellChars = map[int]int{2: 120, 3: 90, 4: 60}

func parseTable(raw json.RawMessage) (core.Drawing, error) {
	var s tbSpec
	if err := core.Decode(raw, &s); err != nil {
		return nil, err
	}
	var p core.Problems
	p.Count("columns", len(s.Columns), 2, 4)
	p.Count("rows", len(s.Rows), 1, 14)
	for i := range s.Columns {
		s.Columns[i] = p.Label(core.Path("columns", i), s.Columns[i], true, tbColumnChars)
	}
	s.Corner = p.Label("corner", s.Corner, false, tbCornerChars)
	// A table of too few or too many columns is refused above; its cells
	// are checked meanwhile at the nearest count's limit.
	cellChars := tbCellChars[max(2, min(4, len(s.Columns)))]
	t := &table{}
	for i, r := range s.Rows {
		at := core.Path("rows", i)
		r.Feature = p.Label(core.Path(at, "feature"), r.Feature, true, tbFeatureChars)
		s.Rows[i].Feature = r.Feature
		if len(r.Cells) != len(s.Columns) {
			p.Add(core.Path(at, "cells"), "has %d entries; there are %d columns, so give one per column (\"\" for an empty cell)", len(r.Cells), len(s.Columns))
		}
		row := make([]tbCell, len(r.Cells))
		for j, rc := range r.Cells {
			cp := core.Path(at, "cells", j)
			trim := bytes.TrimSpace(rc)
			switch {
			case len(trim) > 0 && trim[0] == '"':
				var str string
				_ = json.Unmarshal(trim, &str)
				row[j].Text = str
			case len(trim) > 0 && trim[0] == '{':
				if !decodeAt(&p, cp, rc, &row[j]) {
					continue
				}
			default:
				p.Add(cp, "must be text or {text, mark, tone}, got %s", string(trim))
				continue
			}
			row[j].Text = p.Label(core.Path(cp, "text"), row[j].Text, false, cellChars)
			p.OneOf(core.Path(cp, "mark"), row[j].Mark, true, "yes", "no", "partial")
			p.Tone(core.Path(cp, "tone"), row[j].Tone)
			if m := strings.ToLower(row[j].Text); row[j].Mark == "" && (m == "yes" || m == "no" || m == "partial") {
				row[j].Mark, row[j].Text = m, ""
			}
		}
		t.cells = append(t.cells, row)
	}
	if p.Any() {
		return nil, p.Err()
	}
	t.layout(s)
	if t.H > maxBody {
		tooTall(&p, "table", t.H, "shorten the cells, or compare fewer features")
		return nil, p.Err()
	}
	return t, nil
}

func (t *table) layout(s tbSpec) {
	n := len(s.Columns)
	first := map[int]float64{2: 112, 3: 98, 4: 84}[n]
	widths := t.columnWidths(s, avail-first)
	t.colX, t.colW = []float64{margin}, []float64{first}
	x := margin + first
	for i := 0; i < n; i++ {
		t.colX = append(t.colX, x)
		t.colW = append(t.colW, widths[i])
		x += widths[i]
	}
	inner := func(c int) float64 { return t.colW[c] - 2*tbPadX }
	t.corner = wrapBlock(s.Corner, labelSize, inner(0), true)
	hh := t.corner.h
	for i, c := range s.Columns {
		b := wrapBlock(c, fitSize(c, tbHeadSz, inner(i+1), true), inner(i+1), true)
		t.heads = append(t.heads, b)
		hh = max(hh, b.h)
	}
	y := 0.0
	t.rowY = append(t.rowY, y)
	t.rowH = append(t.rowH, hh+2*tbPadY+2)
	y += hh + 2*tbPadY + 2
	for i, r := range s.Rows {
		f := wrapBlock(r.Feature, fitSize(r.Feature, tbSize, inner(0), true), inner(0), true)
		t.features = append(t.features, f)
		h := f.h
		var texts []textBlock
		for j, c := range t.cells[i] {
			tb := wrapBlock(c.Text, fitSize(c.Text, tbSize, inner(j+1), false), inner(j+1), false)
			texts = append(texts, tb)
			ch := tb.h
			if c.Mark != "" {
				ch += 2 * tbMark
				if c.Text != "" {
					ch += 4
				}
			}
			h = max(h, ch)
		}
		t.texts = append(t.texts, texts)
		t.rowY = append(t.rowY, y)
		t.rowH = append(t.rowH, h+2*tbPadY)
		y += h + 2*tbPadY
	}
	t.H = y + 2
}

// roundTop is a rectangle with its top corners (or bottom, when !top)
// rounded to r.
func roundRect(x, y, w, h, r float64, tl, tr, br, bl bool) string {
	rad := func(on bool) float64 {
		if on {
			return r
		}
		return 0
	}
	a, b, c, d := rad(tl), rad(tr), rad(br), rad(bl)
	f := core.F
	return "M" + f(x+a) + " " + f(y) + "H" + f(x+w-b) + "Q" + f(x+w) + " " + f(y) + " " + f(x+w) + " " + f(y+b) +
		"V" + f(y+h-c) + "Q" + f(x+w) + " " + f(y+h) + " " + f(x+w-c) + " " + f(y+h) +
		"H" + f(x+d) + "Q" + f(x) + " " + f(y+h) + " " + f(x) + " " + f(y+h-d) +
		"V" + f(y+a) + "Q" + f(x) + " " + f(y) + " " + f(x+a) + " " + f(y) + "Z"
}

func (t *table) Draw(c *core.Canvas) error {
	th := c.T
	const r = 8.0
	last := len(t.rowY) - 1
	x0, w := margin, avail
	bottom := t.rowY[last] + t.rowH[last]
	c.Rect(x0, 0, w, bottom, r, th.Bg, "", 0)
	// The header row and the feature column.
	c.Path(roundRect(x0, 0, w, t.rowH[0], r, true, true, false, false), th.SurfaceAlt, "", 0)
	c.Path(roundRect(x0, t.rowH[0], t.colW[0], bottom-t.rowH[0], r, false, false, false, true), th.Surface, "", 0)
	// Toned cells.
	for i, row := range t.cells {
		for j, cell := range row {
			if cell.Tone == "" {
				continue
			}
			_, fill := th.Tone(cell.Tone)
			c.Rect(t.colX[j+1]+2, t.rowY[i+1]+2, t.colW[j+1]-4, t.rowH[i+1]-4, 4, fill, "", 0)
		}
	}
	for i := 1; i < len(t.rowY); i++ {
		c.Line(x0, t.rowY[i], x0+w, t.rowY[i], th.Edge, 0.8, "")
	}
	for j := 1; j < len(t.colX); j++ {
		c.Line(t.colX[j], 0, t.colX[j], bottom, th.Edge, 0.8, "")
	}
	c.Rect(x0, 0, w, bottom, r, "", th.Edge, 1.2)
	// Text.
	t.corner.draw(c, t.colX[0]+tbPadX, t.rowH[0]/2, th.Muted, core.Start, false)
	for j, h := range t.heads {
		h.draw(c, t.colX[j+1]+t.colW[j+1]/2, t.rowH[0]/2, th.Ink, core.Middle, false)
	}
	for i, f := range t.features {
		y := t.rowY[i+1] + t.rowH[i+1]/2
		f.draw(c, t.colX[0]+tbPadX, y, th.Ink, core.Start, false)
		for j, cell := range t.cells[i] {
			cx := t.colX[j+1] + t.colW[j+1]/2
			tb := t.texts[i][j]
			ch := tb.h
			if cell.Mark != "" {
				ch += 2 * tbMark
				if cell.Text != "" {
					ch += 4
				}
			}
			top := y - ch/2
			if cell.Mark != "" {
				drawMark(c, cx, top+tbMark, cell.Mark)
				top += 2*tbMark + 4
			}
			if cell.Text != "" {
				tb.draw(c, cx, top+tb.h/2, th.Ink, core.Middle, false)
			}
		}
	}
	c.H = t.H
	return nil
}

// drawMark draws a tick, a cross or a wave in a tinted disc at (x, y).
func drawMark(c *core.Canvas, x, y float64, mark string) {
	th := c.T
	const r = tbMark
	switch mark {
	case "yes":
		c.Circle(x, y, r, th.GoodSoft, th.Good, 1)
		c.Polyline([]core.Pt{{X: x - 3.8, Y: y + 0.2}, {X: x - 1, Y: y + 3}, {X: x + 4, Y: y - 3}}, th.Good, 1.9, "")
	case "no":
		c.Circle(x, y, r, th.BadSoft, th.Bad, 1)
		c.Line(x-3, y-3, x+3, y+3, th.Bad, 1.9, "")
		c.Line(x-3, y+3, x+3, y-3, th.Bad, 1.9, "")
	case "partial":
		c.Circle(x, y, r, th.WarnSoft, th.Warn, 1)
		f := core.F
		c.Path("M"+f(x-4.2)+" "+f(y+1)+"C"+f(x-2.5)+" "+f(y-2.6)+" "+f(x-0.5)+" "+f(y-1.4)+" "+f(x)+" "+f(y)+
			"C"+f(x+0.5)+" "+f(y+1.4)+" "+f(x+2.5)+" "+f(y+2.6)+" "+f(x+4.2)+" "+f(y-1), "", th.Warn, 1.9)
	}
}

// columnWidths shares room among the columns: equally, unless a column's
// longest word (its heading's, or a cell's) would not stay whole in an
// equal share. That column is given the width it needs, and the others
// share what is left, as long as each of them still keeps its own words
// whole; otherwise the shares stay equal and the text is made smaller, or
// broken by the usual rules (core.Wrap).
func (t *table) columnWidths(s tbSpec, room float64) []float64 {
	n := len(s.Columns)
	equal := make([]float64, n)
	for i := range equal {
		equal[i] = room / float64(n)
	}
	need := make([]float64, n)
	for j, c := range s.Columns {
		need[j] = core.WidestWord(c, tbHeadSz, true)
		for i := range t.cells {
			need[j] = max(need[j], core.WidestWord(t.cells[i][j].Text, tbSize, false))
		}
		need[j] += 2*tbPadX + 2
	}
	grown, left, rest := map[int]bool{}, room, n
	for j := range need {
		if need[j] > equal[j] {
			grown[j] = true
			left -= need[j]
			rest--
		}
	}
	if len(grown) == 0 || rest == 0 {
		return equal
	}
	share := left / float64(rest)
	out := make([]float64, n)
	for j := range out {
		if grown[j] {
			out[j] = need[j]
			continue
		}
		if need[j] > share {
			return equal
		}
		out[j] = share
	}
	return out
}
