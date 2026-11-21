package diagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

func kind(t *testing.T, name string) core.Kind {
	t.Helper()
	for _, k := range Kinds() {
		if k.Name == name {
			return k
		}
	}
	t.Fatalf("no kind %q", name)
	return core.Kind{}
}

// spec is the spec of a kind's worked example.
func exampleSpec(t *testing.T, k core.Kind) json.RawMessage {
	t.Helper()
	var a struct {
		Type string          `json:"type"`
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal([]byte(k.Example), &a); err != nil {
		t.Fatalf("%s: the example is not JSON: %v", k.Name, err)
	}
	if a.Type != k.Name {
		t.Fatalf("%s: the example is of type %q", k.Name, a.Type)
	}
	return a.Spec
}

func draw(t *testing.T, d core.Drawing, th *core.Theme) *core.Canvas {
	t.Helper()
	c := core.NewCanvas(th)
	if err := d.Draw(c); err != nil {
		t.Fatalf("draw: %v", err)
	}
	return c
}

var numAttr = regexp.MustCompile(` (x|y|x1|y1|x2|y2|cx|cy)="(-?[0-9.]+)"`)
var pointsAttr = regexp.MustCompile(` points="([^"]+)"`)

// inside checks every placed coordinate in a body lies on the canvas.
func inside(t *testing.T, name string, c *core.Canvas) {
	t.Helper()
	if c.H <= 0 || c.H > maxBody {
		t.Fatalf("%s: body height %v", name, c.H)
	}
	check := func(axis string, v float64) {
		lim := core.Width
		if strings.HasPrefix(axis, "y") || axis == "cy" {
			lim = c.H
		}
		if v < -0.5 || v > lim+0.5 {
			t.Errorf("%s: %s=%v is outside the body (0 to %v)", name, axis, v, lim)
		}
	}
	for _, m := range numAttr.FindAllStringSubmatch(c.Body(), -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		check(m[1], v)
	}
	for _, m := range pointsAttr.FindAllStringSubmatch(c.Body(), -1) {
		for _, p := range strings.Fields(m[1]) {
			xy := strings.Split(p, ",")
			x, _ := strconv.ParseFloat(xy[0], 64)
			y, _ := strconv.ParseFloat(xy[1], 64)
			check("x", x)
			check("y", y)
		}
	}
}

// Every example parses and draws, the same each time, inside its body,
// in both themes.
func TestExamples(t *testing.T) {
	for _, k := range Kinds() {
		t.Run(k.Name, func(t *testing.T) {
			if k.Family != "diagram" || k.Use == "" || k.Fields == "" {
				t.Fatal("the kind is not fully described")
			}
			if len(k.Example) > 1300 {
				t.Errorf("the example is %d characters; keep it compact", len(k.Example))
			}
			spec := exampleSpec(t, k)
			d, err := k.Parse(spec)
			if err != nil {
				t.Fatalf("the example is refused: %v", err)
			}
			for _, th := range []*core.Theme{&core.Light, &core.Dark} {
				c := draw(t, d, th)
				inside(t, k.Name+" "+th.Name, c)
				d2, _ := k.Parse(spec)
				if again := draw(t, d2, th); again.Body() != c.Body() || again.H != c.H {
					t.Fatalf("%s: drawn twice, it differs", th.Name)
				}
			}
			light, dark := draw(t, d, &core.Light), draw(t, d, &core.Dark)
			if light.H != dark.H {
				t.Fatal("light and dark are laid out differently")
			}
			if strings.Contains(light.Body(), core.Dark.Ink) || strings.Contains(dark.Body(), core.Light.Ink) {
				t.Fatal("a fixed colour from the other theme is drawn")
			}
		})
	}
}

// refused parses spec with kind and checks the refusal says each of want.
func refused(t *testing.T, kindName, spec string, want ...string) {
	t.Helper()
	_, err := kind(t, kindName).Parse(json.RawMessage(spec))
	var r *core.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("want a refusal, got %v", err)
	}
	msg := err.Error()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("refusal %q does not say %q", msg, w)
		}
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		kind, spec string
		want       []string
	}{
		{"flowchart", `{"nodes":[{"id":"a","text":"A","next":"x"},{"id":"b","text":"B"}]}`,
			[]string{`nodes[0].next: "x" is not a node id; ids are a, b`}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","next":"d"},{"id":"d","text":"Q?","shape":"decision","yes":"a"}]}`,
			[]string{"nodes[1]", "needs at least two", "one per answer"}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","next":"b"},{"id":"b","text":"B","shape":"end"},{"id":"c","text":"C","shape":"end"}]}`,
			[]string{"nodes[2]", "cannot be reached", "link to it"}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","yes":"b"},{"id":"b","text":"B"}]}`,
			[]string{"nodes[0]", `make it shape "decision"`}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","shape":"oval"}]}`,
			[]string{"nodes[0].shape", "start, step, decision, end, note", "at least 2"}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","next":"b"},{"id":"b","text":"B","colour":"red"}]}`,
			[]string{`field "colour"`}},
		{"flowchart", `{"nodes":[{"id":"a","text":"A","next":"n"},{"id":"n","text":"N","shape":"note","next":"a"}]}`,
			[]string{`"n" is a note`, `next: "a" instead`}},
		{"flowchart", `{"nodes":[{"id":"q","text":"Q?","shape":"decision","yes":"a"},{"id":"a","text":"A"}],"edges":[{"from":"q","to":"q"}]}`,
			[]string{"edges[0].label", "needs a label"}},
		{"pathway", `{"nodes":[{"id":"a","text":"A"},{"id":"b","text":"B"}],"links":[{"from":"a","to":"b","effect":"blocks"}]}`,
			[]string{"links[0].effect", "activates, inhibits, converts, releases"}},
		{"pathway", `{"nodes":[{"id":"a","text":"A"},{"id":"b","text":"B"},{"id":"d","text":"Drug","input":true}],"links":[{"from":"a","to":"b","effect":"activates"}]}`,
			[]string{"nodes[2]", "not linked to anything", "one link"}},
		{"timeline", `{"events":[{"when":"Day 1","label":"A"},{"when":"Day 2","label":"B"}],"phases":[{"from":"Day 1","to":"Day 9","label":"P"}]}`,
			[]string{"phases[0].to", `"Day 9" is not the when of an event`, "Day 1, Day 2"}},
		{"timeline", `{"events":[{"when":"Day 1","label":"A"},{"when":"Day 1","label":"B"}]}`,
			[]string{"events[1].when", "distinct"}},
		{"cycle", `{"steps":[{"label":"A"},{"label":"B"}]}`,
			[]string{"steps", "at least 3"}},
		{"cycle", `{"steps":[{"label":"A"},{"label":"B"},{"label":"C","tone":"red"}]}`,
			[]string{"steps[2].tone", "neutral, good, warn, bad, accent"}},
		{"comparison_table", `{"columns":["A","B"],"rows":[{"feature":"F","cells":["x"]}]}`,
			[]string{"rows[0].cells", "has 1 entries", "one per column"}},
		{"comparison_table", `{"columns":["A","B"],"rows":[{"feature":"F","cells":["x",{"mark":"maybe"}]}]}`,
			[]string{"rows[0].cells[1].mark", "yes, no, partial"}},
		{"comparison_table", `{"columns":["A","B"],"rows":[{"feature":"F","cells":["x",{"text":"y","colour":"red"}]}]}`,
			[]string{"rows[0].cells[1]", `field "colour"`}},
		{"concept_map", `{"centre":"C","branches":[{"text":"B","children":[{"text":"1"},{"text":"2"},{"text":"3"},{"text":"4"},{"text":"5"}]}]}`,
			[]string{"branches[0].children", "at most 4"}},
		{"venn", `{"sets":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"regions":[{"sets":["a","c"],"items":["x"]}]}`,
			[]string{"regions[0].sets[1]", `"c" is not a set id; ids are a, b`}},
		{"venn", `{"sets":[{"id":"a","label":"A"},{"id":"b","label":"B"},{"id":"c","label":"C"}],"regions":[{"sets":["a","b","c"],"items":["Fever and rash","Conjunctivitis","Lymphadenopathy","Mucositis"]}]}`,
			[]string{"regions[0]", "do not fit", "fewer or shorter items"}},
		{"schematic", `{"compartments":[{"id":"a","label":"A","shape":"box","col":1,"row":1,"colspan":2},{"id":"b","label":"B","shape":"box","col":2,"row":1}]}`,
			[]string{"compartments[1]", `"b" overlaps "a"`, "col 2, row 1", "move one"}},
		{"schematic", `{"compartments":[{"id":"a","label":"A","shape":"box","col":4,"row":1,"colspan":2}]}`,
			[]string{"compartments[0].col", "outside the grid", "1 to 4"}},
		{"schematic", `{"compartments":[{"id":"a","label":"A","shape":"box","col":1,"row":1},{"id":"b","label":"B","shape":"box","col":2,"row":1},{"id":"c","label":"C","shape":"box","col":3,"row":1}],"flows":[{"from":"a","to":"c"}]}`,
			[]string{"flows[0]", `would cross "b"`, "same row or column"}},
		{"schematic", `{"compartments":[{"id":"o","label":"O","shape":"box","col":1,"row":1},{"id":"i","label":"I","shape":"box","col":2,"row":1,"in":"o"}]}`,
			[]string{"compartments[1]", "not within"}},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("%s-%d", c.kind, i), func(t *testing.T) { refused(t, c.kind, c.spec, c.want...) })
	}
}

// A long algorithm that cannot fit a phone is refused, saying how to
// shorten it.
func TestTooTall(t *testing.T) {
	var nodes []string
	for i := 0; i < 30; i++ {
		next := ""
		if i < 29 {
			next = fmt.Sprintf(`,"next":"n%d"`, i+1)
		}
		nodes = append(nodes, fmt.Sprintf(`{"id":"n%d","text":"Step %d: a longer instruction that takes two lines in its box"%s}`, i, i, next))
	}
	refused(t, "flowchart", `{"nodes":[`+strings.Join(nodes, ",")+`]}`, "units tall", "Merge consecutive steps", "split it into two figures")
	var rows []string
	for i := 0; i < 14; i++ {
		rows = append(rows, `{"feature":"A feature with a long name","cells":["`+strings.Repeat("words that wrap ", 3)+`","`+strings.Repeat("more words here ", 3)+`"]}`)
	}
	_, err := kind(t, "comparison_table").Parse(json.RawMessage(`{"columns":["A","B"],"rows":[` + strings.Join(rows, ",") + `]}`))
	if err != nil && !strings.Contains(err.Error(), "units tall") {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

// Hard layouts: a long algorithm with loops, notes and side branches, a
// converging cascade with feedback.
var hard = map[string]string{
	// The first hour of an asthma attack, as a model drew it for the chat:
	// a long word in a narrow row ("Mild-moderate:"), a branch out of a
	// decision's side beside another box, a line round the next row.
	"asthma": "{\"nodes\": [{\"id\": \"s\", \"text\": \"7-year-old in acute asthma attack: ABCDE, sit upright, call for senior help\", \"shape\": \"start\", \"next\": \"o2\"}, {\"id\": \"o2\", \"text\": \"Oxygen by mask to keep SpO2 94-98%\", \"next\": \"as\"}, {\"id\": \"as\", \"text\": \"Assess severity: SpO2, speech, PEF, HR, RR, consciousness, work of breathing\", \"next\": \"lt\"}, {\"id\": \"lt\", \"text\": \"Silent chest, exhaustion, cyanosis or drowsiness?\", \"shape\": \"decision\", \"yes\": \"ltx\", \"no\": \"sv\"}, {\"id\": \"ltx\", \"text\": \"Life-threatening: ICU and anaesthetist, continuous salbutamol plus ipratropium, IV magnesium\", \"tone\": \"bad\", \"next\": \"pred\"}, {\"id\": \"sv\", \"text\": \"Severe? SpO2 under 92%, cannot complete sentences, PEF under 50%\", \"shape\": \"decision\", \"yes\": \"sev\", \"no\": \"mild\"}, {\"id\": \"sev\", \"text\": \"Severe: nebulised salbutamol 2.5-5 mg plus ipratropium 0.5 mg, repeat every 20 min\", \"tone\": \"warn\", \"next\": \"pred\"}, {\"id\": \"mild\", \"text\": \"Mild-moderate: salbutamol 2-10 puffs via spacer, repeat every 20 min\", \"tone\": \"good\", \"next\": \"pred\"}, {\"id\": \"nt\", \"text\": \"One puff at a time, 5 breaths each; nebulise if too breathless\", \"shape\": \"note\", \"next\": \"mild\"}, {\"id\": \"pred\", \"text\": \"Prednisolone 1-2 mg/kg by mouth (max 40 mg) within the first hour\", \"tone\": \"bad\", \"next\": \"re\"}, {\"id\": \"re\", \"text\": \"Reassess at 20-30 min: improving?\", \"shape\": \"decision\", \"yes\": \"cont\", \"no\": \"rep\"}, {\"id\": \"cont\", \"text\": \"Salbutamol every 20-30 min, spacing out as PEF rises\", \"next\": \"adm\"}, {\"id\": \"rep\", \"text\": \"Repeat salbutamol with ipratropium, IV magnesium 40-50 mg/kg over 20 min, senior review\", \"tone\": \"warn\", \"next\": \"re\"}, {\"id\": \"adm\", \"text\": \"Admit or observe; steroid for 3-5 days; discharge when PEF over 75% predicted\", \"shape\": \"end\", \"tone\": \"good\"}]}",
	"dka": `{"nodes":[
{"id":"s","text":"Suspected DKA: glucose > 11 mmol/L, pH < 7.3 or bicarbonate < 15","shape":"start","next":"shock"},
{"id":"shock","text":"Shocked (poor perfusion, hypotension)?","shape":"decision","yes":"bolus","no":"mild"},
{"id":"bolus","text":"0.9% saline 10 mL/kg bolus over 15 min","tone":"bad","next":"reshock"},
{"id":"reshock","text":"Still shocked?","shape":"decision","yes":"bolus","no":"fluids"},
{"id":"mild","text":"Mild or moderate: 10 mL/kg over 60 min if needed","next":"fluids"},
{"id":"fluids","text":"Deficit plus maintenance over 48 h, 0.9% saline with 40 mmol/L KCl","next":"insulin"},
{"id":"insulin","text":"Start insulin 0.05 to 0.1 units/kg/h, 1 to 2 h after fluids","next":"mon"},
{"id":"k","text":"Do not bolus insulin","shape":"note","next":"insulin"},
{"id":"mon","text":"Hourly glucose, neuro obs, fluid balance; 2-hourly gas","next":"neuro"},
{"id":"neuro","text":"Headache, falling GCS, bradycardia?","shape":"decision","yes":"ce","no":"glu"},
{"id":"ce","text":"Cerebral oedema: mannitol or 2.7% saline; call PICU","tone":"bad","shape":"end"},
{"id":"glu","text":"Glucose below 14 mmol/L?","shape":"decision","yes":"dex","no":"ph"},
{"id":"dex","text":"Add 5% glucose to fluids","next":"ph"},
{"id":"ph","text":"pH above 7.3 and ketones below 1 mmol/L?","shape":"decision","yes":"oral","no":"mon"},
{"id":"oral","text":"Tolerating oral fluids?","shape":"decision","yes":"sc","no":"wait"},
{"id":"wait","text":"Continue IV fluids and insulin","next":"oral"},
{"id":"sc","text":"Subcutaneous insulin, stop IV insulin 30 min later","next":"edu"},
{"id":"edu","text":"Diabetes education and follow-up","shape":"end","tone":"good"}]}`,
	"fever": `{"nodes":[
{"id":"s","text":"Well-appearing infant 22 to 28 days, 38.0 °C or more","shape":"start","next":"tests"},
{"id":"tests","text":"Urinalysis, blood culture, CRP, procalcitonin, ANC","next":"im"},
{"id":"im","text":"Any inflammatory marker abnormal?","shape":"decision","yes":"lp","no":"ua"},
{"id":"lp","text":"Lumbar puncture","next":"csf"},
{"id":"csf","text":"CSF pleocytosis or uninterpretable?","shape":"decision","yes":"menin","no":"adm"},
{"id":"menin","text":"Admit; IV antibiotics at meningitic doses","shape":"end","tone":"bad"},
{"id":"adm","text":"Admit or observe at home; parenteral antibiotics","shape":"end","tone":"warn"},
{"id":"ua","text":"Urinalysis positive?","shape":"decision","yes":"uti","no":"lpopt"},
{"id":"uti","text":"Consider LP; treat as UTI","next":"home"},
{"id":"lpopt","text":"LP may be considered","next":"obs"},
{"id":"obs","text":"Observe 24 to 36 h; culture results","next":"cult"},
{"id":"cult","text":"Cultures positive or clinical change?","shape":"decision","yes":"lp","no":"home"},
{"id":"home","text":"Home with follow-up in 24 h","shape":"end","tone":"good"}]}`,
}

var hardPathway = `{"nodes":[
{"id":"xii","text":"XII → XIIa"},{"id":"xi","text":"XI → XIa"},{"id":"ix","text":"IX → IXa"},{"id":"viii","text":"VIIIa","input":true},
{"id":"tf","text":"Tissue factor + VIIa"},{"id":"x","text":"X → Xa","tone":"accent"},{"id":"v","text":"Va","input":true},
{"id":"thr","text":"Prothrombin → thrombin"},{"id":"fib","text":"Fibrinogen → fibrin"},{"id":"xiii","text":"XIIIa cross-links fibrin","tone":"good"},
{"id":"hep","text":"Heparin–antithrombin","input":true}],"links":[
{"from":"xii","to":"xi","effect":"activates"},{"from":"xi","to":"ix","effect":"activates"},{"from":"viii","to":"ix","effect":"activates","label":"cofactor"},
{"from":"ix","to":"x","effect":"activates","label":"intrinsic"},{"from":"tf","to":"x","effect":"activates","label":"extrinsic"},
{"from":"v","to":"x","effect":"activates"},{"from":"x","to":"thr","effect":"converts"},{"from":"thr","to":"fib","effect":"converts"},
{"from":"fib","to":"xiii","effect":"activates"},{"from":"hep","to":"thr","effect":"inhibits"},{"from":"thr","to":"xi","effect":"activates","label":"feedback"}]}`

// checkLayout checks a layered layout: boxes apart and on the canvas,
// every line starting and ending on its nodes' borders and passing
// through no other box, labels clear of boxes and of each other.
func checkLayout(t *testing.T, l *layout) {
	t.Helper()
	var boxes []box
	for _, n := range l.nodes {
		b := box{n.absX() - n.w/2, n.y, n.w, n.h}
		if b.X < 0 || b.R() > core.Width || b.Y < 0 || b.B() > l.H {
			t.Errorf("node %d at %+v is off the canvas (height %v)", n.idx, b, l.H)
		}
		boxes = append(boxes, b)
	}
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			if boxes[i].overlaps(boxes[j], 4) {
				t.Errorf("nodes %d and %d overlap: %+v %+v", i, j, boxes[i], boxes[j])
			}
		}
	}
	onBorder := func(p core.Pt, b box) bool {
		const e = 0.6
		inX := p.X >= b.X-e && p.X <= b.R()+e
		inY := p.Y >= b.Y-e && p.Y <= b.B()+e
		return (inY && (math.Abs(p.X-b.X) < e || math.Abs(p.X-b.R()) < e)) || (inX && (math.Abs(p.Y-b.Y) < e || math.Abs(p.Y-b.B()) < e))
	}
	var labels []box
	for ei, e := range l.g.edges {
		r := e.route
		if len(r) < 2 {
			t.Fatalf("edge %d has no route", ei)
		}
		if !onBorder(r[0], boxes[e.from]) || !onBorder(r[len(r)-1], boxes[e.to]) {
			t.Errorf("edge %d (%d→%d) does not start and end on its nodes: %v", ei, e.from, e.to, r)
		}
		for k := 0; k+1 < len(r); k++ {
			if r[k].X != r[k+1].X && r[k].Y != r[k+1].Y {
				t.Errorf("edge %d has a sloping leg %v→%v", ei, r[k], r[k+1])
			}
			for bi, b := range boxes {
				if segHitsBox(r[k], r[k+1], b) {
					t.Errorf("edge %d (%d→%d) passes through node %d", ei, e.from, e.to, bi)
				}
			}
			for _, p := range r[k : k+2] {
				if p.X < 0 || p.X > core.Width || p.Y < 0 || p.Y > l.H {
					t.Errorf("edge %d leaves the canvas at %v", ei, p)
				}
			}
		}
		if e.label != "" && e.lab == nil {
			t.Errorf("edge %d's label %q is not drawn", ei, e.label)
		}
		if e.lab != nil {
			if lb := e.lab.at; lb.X < 0 || lb.R() > core.Width || lb.Y < 0 || lb.B() > l.H {
				t.Errorf("edge %d's label %q at %+v is off the canvas (height %v)", ei, e.label, lb, l.H)
			}
			// Read with its own line: touching it, or a few units off.
			near := false
			for k := 0; k+1 < len(r); k++ {
				if segHitsBox(r[k], r[k+1], e.lab.at.grow(8)) {
					near = true
				}
			}
			if !near {
				t.Errorf("edge %d's label %q at %+v is not beside its own line %v", ei, e.label, e.lab.at, r)
			}
			for bi, b := range boxes {
				if e.lab.at.overlaps(b, 0) {
					t.Errorf("edge %d's label %q is over node %d", ei, e.label, bi)
				}
			}
			for _, o := range labels {
				if e.lab.at.overlaps(o, 0) {
					t.Errorf("edge %d's label %q is over another label", ei, e.label)
				}
			}
			labels = append(labels, e.lab.at)
		}
	}
}

func TestFlowchartLayout(t *testing.T) {
	specs := map[string]string{"example": string(exampleSpec(t, flowchartKind))}
	for k, v := range hard {
		specs[k] = v
	}
	for name, s := range specs {
		t.Run(name, func(t *testing.T) {
			d, err := parseFlowchart(json.RawMessage(s))
			if err != nil {
				t.Fatal(err)
			}
			checkLayout(t, d.(*flowchart).lay)
		})
	}
	// Loops are found and drawn round the side.
	d, _ := parseFlowchart(json.RawMessage(hard["dka"]))
	backs := 0
	for _, e := range d.(*flowchart).lay.g.edges {
		if e.back {
			backs++
		}
	}
	if backs != 3 {
		t.Errorf("found %d loops in the DKA algorithm, want 3", backs)
	}
}

func TestPathwayLayout(t *testing.T) {
	for name, s := range map[string]string{"example": string(exampleSpec(t, pathwayKind)), "coagulation": hardPathway} {
		t.Run(name, func(t *testing.T) {
			d, err := parsePathway(json.RawMessage(s))
			if err != nil {
				t.Fatal(err)
			}
			l := d.(*pathway).lay
			checkLayout(t, l)
			if name == "coagulation" {
				// The intrinsic and extrinsic pathways converge: their last
				// steps share a row, just above factor X.
				ix, tf, x := l.nodes[2], l.nodes[4], l.nodes[5]
				if ix.layer != tf.layer || x.layer != ix.layer+1 {
					t.Errorf("IX in row %d, tissue factor in %d, X in %d: the pathways do not converge", ix.layer, tf.layer, x.layer)
				}
				// Inputs sit beside the node they act on.
				if v := l.nodes[6]; v.host != x {
					t.Error("Va is not beside X")
				}
			}
		})
	}
}

func TestCycleLayout(t *testing.T) {
	for n := 3; n <= 8; n++ {
		var steps []string
		for i := 0; i < n; i++ {
			steps = append(steps, fmt.Sprintf(`{"label":"Isovolumetric step %d","detail":"with a detail line"}`, i+1))
		}
		d, err := parseCycle(json.RawMessage(`{"centre":"Centre label","steps":[` + strings.Join(steps, ",") + `]}`))
		if err != nil {
			t.Fatal(err)
		}
		c := d.(*cycle)
		for i, a := range c.boxes {
			if a.X < 0 || a.R() > core.Width || a.Y < 0 || a.B() > c.H {
				t.Errorf("%d steps: box %d is off the canvas", n, i)
			}
			for j := i + 1; j < len(c.boxes); j++ {
				if a.overlaps(c.boxes[j], 0) {
					t.Errorf("%d steps: boxes %d and %d overlap", n, i, j)
				}
			}
		}
		if c.H > 2*core.Width {
			t.Errorf("%d steps: %v tall, too stretched", n, c.H)
		}
	}
}

func TestVennRegions(t *testing.T) {
	d, err := parseVenn(exampleSpec(t, vennKind))
	if err != nil {
		t.Fatal(err)
	}
	v := d.(*venn)
	masks := []int{1, 2, 4, 3, 6, 5, 7} // the example's regions, in order
	for i, tx := range v.texts {
		if !v.boxIn(tx.at, masks[i]) {
			t.Errorf("region %d's text is not wholly inside its region", i)
		}
		for j := i + 1; j < len(v.texts); j++ {
			if tx.at.overlaps(v.texts[j].at, 0) {
				t.Errorf("regions %d and %d's texts overlap", i, j)
			}
		}
	}
}

func TestConceptAndSchematicApart(t *testing.T) {
	d, err := parseConcept(exampleSpec(t, conceptKind))
	if err != nil {
		t.Fatal(err)
	}
	m := d.(*concept)
	all := []box{m.centre.b}
	for i, n := range m.nodes {
		all = append(all, n.b)
		for _, k := range m.kids[i] {
			all = append(all, k.b)
		}
	}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[i].overlaps(all[j], 2) {
				t.Errorf("concept map boxes %d and %d overlap", i, j)
			}
		}
	}
	for _, l := range m.labels {
		for i, b := range all {
			if l.at.overlaps(b, 0) {
				t.Errorf("link label %q is over box %d", l.text.lines, i)
			}
		}
	}
	s, err := parseSchematic(exampleSpec(t, schematicKind))
	if err != nil {
		t.Fatal(err)
	}
	sc := s.(*schematic)
	for i, a := range sc.boxes {
		for j := i + 1; j < len(sc.boxes); j++ {
			nested := sc.parent[i] == j || sc.parent[j] == i
			if nested && !sc.boxes[j].contains(a) && !a.contains(sc.boxes[j]) {
				t.Errorf("compartment %d is not drawn inside %d", i, j)
			}
			if !nested && a.overlaps(sc.boxes[j], 0) {
				t.Errorf("compartments %d and %d overlap", i, j)
			}
		}
	}
}

// An input acting on two nodes (antithrombin on thrombin and Xa) is drawn
// in the pathway rather than refused.
func TestPathwayInputWithTwoLinksIsDrawnInline(t *testing.T) {
	_, err := parsePathway([]byte(`{"nodes":[{"id":"x","text":"Xa"},{"id":"t","text":"Thrombin"},{"id":"at","text":"Antithrombin","input":true}],
		"links":[{"from":"x","to":"t","effect":"activates"},{"from":"at","to":"x","effect":"inhibits"},{"from":"at","to":"t","effect":"inhibits"}]}`))
	if err != nil {
		t.Fatal(err)
	}
}

// A label whose words cannot fit across its compartment is refused rather
// than broken letter by letter.
func TestSchematicRefusesLabelsTooWideForTheirShape(t *testing.T) {
	cols := ""
	for i := 1; i <= 4; i++ {
		cols += fmt.Sprintf(`{"id":"v%d","label":"Interventricular","shape":"vessel","col":%d,"row":1},`, i, i)
	}
	_, err := parseSchematic([]byte(`{"compartments":[` + strings.TrimSuffix(cols, ",") + `]}`))
	if err == nil || !strings.Contains(err.Error(), "wider than its vessel") {
		t.Fatalf("got %v", err)
	}
}

// prose is n characters of clinical prose, for a text at a limit.
func prose(n int) string {
	words := strings.Fields("Give intravenous ceftriaxone after blood cultures and reassess perfusion within the first hour with repeated fluid boluses of saline if capillary refill remains prolonged")
	var b strings.Builder
	for i := 0; b.Len() < n+20; i++ {
		b.WriteString(words[i%len(words)] + " ")
	}
	s := b.String()[:n]
	if s[n-1] == ' ' {
		s = s[:n-1] + "s"
	}
	return s
}

// The limits relaxed on 2026-11-10, where the model's texts ran a little
// over and the layout wraps them: each draws at its new limit, laid out
// as it is drawn (Draw on a canvas), and one character more is refused in
// words.
func TestRelaxedLimits(t *testing.T) {
	cases := []struct {
		kind  string
		spec  func(text string) string
		limit int
		path  string
	}{
		{"flowchart", func(s string) string {
			return fmt.Sprintf(`{"nodes":[{"id":"a","text":%q,"shape":"start","next":"q"},{"id":"q","text":%q,"shape":"decision","yes":"b","no":"c"},{"id":"b","text":%q,"shape":"end"},{"id":"c","text":%q,"next":"d"},{"id":"n","text":%q,"shape":"note","next":"c"},{"id":"d","text":%q,"shape":"end"}]}`, s, s, s, s, s, s)
		}, 160, "nodes[0].text"},
		{"comparison_table", func(s string) string {
			return fmt.Sprintf(`{"columns":["Nephritic","Nephrotic"],"rows":[{"feature":"Urine","cells":[%q,%q]},{"feature":"Oedema","cells":[%q,%q]}]}`, s, s, s, s)
		}, 120, "rows[0].cells[0].text"},
		{"comparison_table", func(s string) string {
			return fmt.Sprintf(`{"columns":["Septic","Cardiogenic","Hypovolaemic"],"rows":[{"feature":"Heart rate","cells":[%q,%q,%q]}]}`, s, s, s)
		}, 90, "rows[0].cells[0].text"},
		{"comparison_table", func(s string) string {
			return fmt.Sprintf(`{"columns":["A","B","C","D"],"rows":[{"feature":"Heart rate","cells":[%q,%q,%q,%q]}]}`, s, s, s, s)
		}, 60, "rows[0].cells[0].text"},
		{"pathway", func(s string) string {
			return fmt.Sprintf(`{"nodes":[{"id":"a","text":%q},{"id":"b","text":%q},{"id":"c","text":%q}],"links":[{"from":"a","to":"b","effect":"activates","label":"renin from the macula densa"},{"from":"a","to":"c","effect":"inhibits"}]}`, s, s, s)
		}, 100, "nodes[0].text"},
		{"concept_map", func(s string) string {
			return fmt.Sprintf(`{"centre":"Iron deficiency","branches":[{"text":%q,"children":[{"text":%q},{"text":%q}]},{"text":%q},{"text":%q,"children":[{"text":%q}]},{"text":%q}]}`, s, s, s, s, s, s, s)
		}, 70, "branches[0].text"},
		{"schematic", func(s string) string {
			return fmt.Sprintf(`{"compartments":[{"id":"a","label":%q,"shape":"box","col":1,"row":1},{"id":"b","label":%q,"shape":"rounded","col":2,"row":1},{"id":"c","label":%q,"shape":"box","col":3,"row":1}],"flows":[{"from":"a","to":"b"},{"from":"b","to":"c"}]}`, s, s, s)
		}, 70, "compartments[0].label"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s-%d", c.kind, c.limit), func(t *testing.T) {
			d, err := kind(t, c.kind).Parse(json.RawMessage(c.spec(prose(c.limit))))
			if err != nil {
				t.Fatalf("texts of %d characters are refused: %v", c.limit, err)
			}
			cv := core.NewCanvas(&core.Light)
			if err := d.Draw(cv); err != nil || cv.H <= 0 {
				t.Fatalf("not drawn: %v (height %v)", err, cv.H)
			}
			over := prose(c.limit + 1)
			refused(t, c.kind, c.spec(over), c.path+": is ", fmt.Sprintf("keep it to %d or fewer", min(core.Words(c.limit), c.limit*len(strings.Fields(over))/len(over))))
		})
	}
}

// In the asthma flowchart (hard["asthma"]): the line out of the
// life-threatening box leaves from the end of its bottom towards the lane
// it runs down, not from its middle across the line of the decision beside
// it; and that decision's "Yes", whose run out of its side is short, sits
// under its own box beside its line, not against the box next to it.
func TestFlowchartBranchesBesideABox(t *testing.T) {
	d, err := parseFlowchart(json.RawMessage(hard["asthma"]))
	if err != nil {
		t.Fatal(err)
	}
	f := d.(*flowchart)
	byID := map[string]int{}
	for i, n := range f.nodes {
		byID[n.ID] = i
	}
	l := f.lay
	bb := func(id string) box { n := l.nodes[byID[id]]; return box{n.absX() - n.w/2, n.y, n.w, n.h} }
	for _, e := range l.g.edges {
		switch {
		case e.from == byID["ltx"] && e.to == byID["pred"]:
			if start := e.route[0]; start.X > bb("ltx").X+18.5 {
				t.Errorf("the life-threatening line leaves its box at x %v, not at its left end (%v)", start.X, bb("ltx").X+18)
			}
		case e.from == byID["sv"] && e.to == byID["sev"]:
			if e.lab == nil {
				t.Fatal("the Yes branch has no label")
			}
			if lb := e.lab.at; lb.Y < bb("sv").B() || lb.overlaps(bb("ltx"), 2) {
				t.Errorf("the Yes label at %+v is not under its decision %+v, clear of %+v", lb, bb("sv"), bb("ltx"))
			}
		}
	}
	// Every word of every box is whole: the mild-moderate box is given the
	// width its words need, or its text set smaller.
	for i, text := range f.text {
		for _, line := range wrapBlock(text, f.fs[i], l.nodes[i].w-2*f.pad[i]+2, false).lines {
			if strings.HasSuffix(line, "-") && !strings.Contains(text, line) {
				t.Errorf("node %q breaks a word: %q", f.nodes[i].ID, line)
			}
		}
	}
}

// Three narrow branches of one decision (a figure drawn with words cut
// mid-word, 2026-11-21): the row takes room from the boxes that can spare
// it, closer gaps and padding, and smaller text before any word is cut,
// so every word of every box is drawn whole, a hyphenated one included.
func TestThreeBranchesKeepTheirWordsWhole(t *testing.T) {
	spec := `{"nodes":[
{"id":"s","text":"Child with suspected neuroblastoma","shape":"start","next":"st"},
{"id":"st","text":"Imaging, MIBG scan, biopsy with MYCN status, bone marrow","next":"rg"},
{"id":"rg","text":"Which risk group?","shape":"decision"},
{"id":"low","text":"Observation, or surgery alone; chemotherapy only for organ-threatening symptoms","tone":"good","shape":"end"},
{"id":"int","text":"Surgery plus moderate-dose chemotherapy","tone":"warn","shape":"end"},
{"id":"high","text":"Induction chemotherapy, surgery, myeloablative therapy with stem cell rescue, radiotherapy, immunotherapy","tone":"bad","shape":"end"}],
"edges":[{"from":"rg","to":"low","label":"Low"},{"from":"rg","to":"int","label":"Intermediate"},{"from":"rg","to":"high","label":"High"}]}`
	d, err := parseFlowchart(json.RawMessage(spec))
	if err != nil {
		t.Fatal(err)
	}
	f := d.(*flowchart)
	right := 0.0
	for i, text := range f.text {
		n := f.lay.nodes[i]
		right = max(right, n.absX()+n.w/2)
		words := map[string]bool{}
		for _, w := range strings.Fields(text) {
			words[w] = true
		}
		_, _, _, bold, _ := f.style(i)
		for _, line := range wrapBlock(text, f.fs[i], n.w-2*f.pad[i]+2, bold).lines {
			for _, w := range strings.Fields(line) {
				if !words[w] {
					t.Errorf("node %q cuts a word: line %q", f.nodes[i].ID, line)
				}
			}
		}
	}
	if right > core.Width-margin+0.5 {
		t.Errorf("the row runs to %v, past the figure's width", right)
	}
}

// A table column whose heading would break ("Life-threatening") is given
// the width its words need from the columns that can spare it.
func TestATableColumnWidensForAWholeWord(t *testing.T) {
	d, err := parseTable(json.RawMessage("{\"corner\": \"Feature\", \"columns\": [\"Moderate\", \"Severe\", \"Life-threatening\"], \"rows\": [{\"feature\": \"Oxygen saturation (SpO2)\", \"cells\": [{\"text\": \"92% or more\", \"mark\": \"yes\"}, {\"text\": \"Under 92%\", \"mark\": \"partial\"}, {\"text\": \"Under 92%\", \"mark\": \"no\"}]}, {\"feature\": \"Talking\", \"cells\": [{\"text\": \"Able to talk in sentences\"}, {\"text\": \"Cannot complete sentences\"}, {\"text\": \"Exhausted or confused\"}]}, {\"feature\": \"Peak flow (% of best)\", \"cells\": [{\"text\": \"Over 50%\"}, {\"text\": \"33\\u201350%\"}, {\"text\": \"Under 33%\"}]}]}"))
	if err != nil {
		t.Fatal(err)
	}
	tb := d.(*table)
	if got := strings.Join(tb.heads[2].lines, "|"); got != "Life-threatening" {
		t.Errorf("the heading is set as %q", got)
	}
	if tb.colW[3] <= tb.colW[2] {
		t.Errorf("the columns are %v", tb.colW)
	}
}

// The feature column gives way like the others: a long feature word
// ("Immunotherapy", bold) takes width from the columns with short cells
// rather than being cut.
func TestATableFeatureColumnWidensForAWholeWord(t *testing.T) {
	d, err := parseTable(json.RawMessage(`{"columns":["Low","Intermediate","High"],"rows":[{"feature":"Immunotherapy","cells":["no","no","yes"]},{"feature":"Chemotherapy","cells":["Rarely","Moderate","Intensive"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	tb := d.(*table)
	for _, f := range tb.features {
		for _, line := range f.lines {
			if strings.HasSuffix(line, "-") {
				t.Errorf("a feature is cut: %q", line)
			}
		}
	}
	total := 0.0
	for _, w := range tb.colW {
		total += w
	}
	if total > avail+0.5 {
		t.Errorf("the columns take %v, more than %v", total, avail)
	}
}

// Decisions with labelled branches as a model drew them (2026-11-21),
// three and four of them out of one decision among them: every label is
// drawn, on the canvas, beside its own line and clear of every box, note
// and other label; and the branches out of one decision cross no other.
func TestBranchLabelsAreReadable(t *testing.T) {
	files, err := filepath.Glob("../../testdata/branches/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no specs: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var args struct {
				Spec json.RawMessage `json:"spec"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				t.Fatal(err)
			}
			d, err := parseFlowchart(args.Spec)
			if err != nil {
				t.Fatal(err)
			}
			l := d.(*flowchart).lay
			checkLayout(t, l)
			byFrom := map[int][]*gedge{}
			for _, e := range l.g.edges {
				if !e.back && !e.attach {
					byFrom[e.from] = append(byFrom[e.from], e)
				}
			}
			for _, es := range byFrom {
				for i := range es {
					for j := i + 1; j < len(es); j++ {
						if routesCross(es[i].route, es[j].route) {
							t.Errorf("branches %q and %q out of one node cross", es[i].label, es[j].label)
						}
					}
				}
			}
		})
	}
}

// routesCross says whether two orthogonal routes cross: a horizontal leg
// of one passes through a vertical leg of the other strictly inside both
// (meeting at a shared start is not a crossing).
func routesCross(a, b []core.Pt) bool {
	legs := func(r []core.Pt) (h, v [][2]core.Pt) {
		for k := 0; k+1 < len(r); k++ {
			switch {
			case math.Abs(r[k].Y-r[k+1].Y) < 0.01 && math.Abs(r[k].X-r[k+1].X) > 0.01:
				h = append(h, [2]core.Pt{r[k], r[k+1]})
			case math.Abs(r[k].X-r[k+1].X) < 0.01 && math.Abs(r[k].Y-r[k+1].Y) > 0.01:
				v = append(v, [2]core.Pt{r[k], r[k+1]})
			}
		}
		return
	}
	cross := func(h, v [2]core.Pt) bool {
		x0, x1 := math.Min(h[0].X, h[1].X), math.Max(h[0].X, h[1].X)
		y0, y1 := math.Min(v[0].Y, v[1].Y), math.Max(v[0].Y, v[1].Y)
		return v[0].X > x0+0.5 && v[0].X < x1-0.5 && h[0].Y > y0+0.5 && h[0].Y < y1-0.5
	}
	ah, av := legs(a)
	bh, bv := legs(b)
	for _, h := range ah {
		for _, v := range bv {
			if cross(h, v) {
				return true
			}
		}
	}
	for _, h := range bh {
		for _, v := range av {
			if cross(h, v) {
				return true
			}
		}
	}
	return false
}
