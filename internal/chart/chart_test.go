package chart

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

func TestNiceTicks(t *testing.T) {
	labels := func(ts []Tick) string {
		var s []string
		for _, t := range ts {
			s = append(s, t.Label)
		}
		return strings.Join(s, " ")
	}
	cases := []struct {
		lo, hi float64
		want   string
	}{
		{0, 100, "0 20 40 60 80 100"},
		{6, 18, "6 8 10 12 14 16 18"},
		{0, 1, "0 0.2 0.4 0.6 0.8 1"},
		{-30, 30, "−30 −20 −10 0 10 20 30"},
		{0, 70, "0 10 20 30 40 50 60 70"},
		{0, 50000, "0 10,000 20,000 30,000 40,000 50,000"},
		{1995, 2025, "1995 2000 2005 2010 2015 2020 2025"},
		{0, 0.003, "0 0.0005 0.001 0.0015 0.002 0.0025 0.003"},
	}
	for _, c := range cases {
		if got := labels(linearTicks(c.lo, c.hi, 5)); got != c.want {
			t.Errorf("ticks %v..%v = %q, want %q", c.lo, c.hi, got, c.want)
		}
	}
	lo, hi, step := niceRange(2.4, 14.6, 6)
	if lo != 2 || hi != 16 || step != 2 {
		t.Errorf("niceRange(2.4, 14.6) = %v, %v, %v", lo, hi, step)
	}
}

func TestLogTicks(t *testing.T) {
	var major []string
	for _, tk := range logTicks(0.01, 10000) {
		if !tk.Minor {
			major = append(major, tk.Label)
		}
	}
	if got := strings.Join(major, " "); got != "0.01 0.1 1 10 100 1,000 10,000" {
		t.Errorf("decades: %q", got)
	}
	var wide []string
	for _, tk := range logTicks(1e-9, 1e-3) {
		if !tk.Minor {
			wide = append(wide, tk.Label)
		}
	}
	if wide[0] != "10⁻⁹" || wide[len(wide)-1] != "10⁻³" {
		t.Errorf("wide decades: %v", wide)
	}
	// Two decades get 2 and 5 labelled too.
	n := 0
	for _, tk := range logTicks(1, 100) {
		if tk.Label != "" {
			n++
		}
	}
	if n != 7 {
		t.Errorf("1 to 100 has %d labelled ticks, want 7 (1 2 5 10 20 50 100)", n)
	}
}

func TestFormat(t *testing.T) {
	cases := map[string]string{
		formatNum(-3, 1):           "−3",
		formatNum(2.5, 0.5):        "2.5",
		formatNum(0.25, 0.25):      "0.25",
		formatNum(12345, 5):        "12,345",
		formatNum(2e-6, 1e-6):      "2×10⁻⁶",
		formatNum(3e8, 1e8):        "3×10⁸",
		fmtValue(12.345):           "12.3",
		fmtValue(0.012345):         "0.0123",
		fmtValue(1234.6):           "1235",
		fmtSigned(-1.104, 2):       "−1.10",
		fmtSigned(-0.001, 2):       "0.00",
		ordinal(1) + ordinal(2):    "1st2nd",
		ordinal(13):                "13th",
		ordinal(23):                "23rd",
		fmtAge(15):                 "15 months",
		fmtAge(30):                 "2 years 6 months",
		fmtAge(132):                "11 years",
		piLabel(-1.5):              "−3π/2",
		piLabel(1):                 "π",
		piLabel(0.5):               "π/2",
		prettyExpr("2*x^2 - 3*pi"): "2x² − 3π",
		prettyExpr("x*sin(x)"):     "x·sin(x)",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestSpread(t *testing.T) {
	cases := [][]float64{
		{10, 11, 12, 13},
		{50, 10, 52, 51, 200},
		{0, 0, 0, 0, 0, 0, 0},
		{100, 30, 70},
	}
	for _, want := range cases {
		got := spread(want, 12, 0, 200)
		for i := range got {
			if got[i] < -1e-9 || got[i] > 200+1e-9 {
				t.Errorf("%v: %v leaves the bounds", want, got)
			}
			for j := range got {
				if i != j && math.Abs(got[i]-got[j]) < 12-1e-9 {
					t.Errorf("%v: %v has labels %d and %d closer than 12", want, got, i, j)
				}
				// Order is kept.
				if want[i] < want[j] && got[i] > got[j] {
					t.Errorf("%v: %v changes the order", want, got)
				}
			}
		}
	}
	// Labels already apart stay where they are.
	got := spread([]float64{100, 30, 70}, 12, 0, 200)
	if got[0] != 100 || got[1] != 30 || got[2] != 70 {
		t.Errorf("labels apart were moved: %v", got)
	}
}

func TestClip(t *testing.T) {
	r := rect{0, 0, 100, 100}
	parts := clipPolyline([]core.Pt{{X: -50, Y: 50}, {X: 50, Y: 50}, {X: 50, Y: 200}, {X: 60, Y: 50}, {X: 70, Y: math.NaN()}, {X: 80, Y: 10}, {X: 90, Y: 20}}, r)
	if len(parts) != 3 {
		t.Fatalf("parts %v", parts)
	}
	if parts[0][0].X != 0 || parts[0][len(parts[0])-1].Y != 100 {
		t.Errorf("first part %v", parts[0])
	}
	poly := clipPolygon([]core.Pt{{X: -10, Y: -10}, {X: 50, Y: -10}, {X: 50, Y: 50}, {X: -10, Y: 50}}, r)
	for _, p := range poly {
		if !r.contains(p) {
			t.Errorf("clipped polygon point %v outside", p)
		}
	}
}

// The LMS curves reproduce the centiles the CDC publishes beside them.
func TestLMSAgainstPublishedCentiles(t *testing.T) {
	cases := []struct {
		name    string
		table   *[2][]lms
		sex     int // 0 female, 1 male
		age     float64
		centile int
		want    float64
	}{
		{"CDC weight, boy 5 y", &cdcWeight, 1, 60.5, 5, 15.23201937},
		{"CDC weight, boy 5 y", &cdcWeight, 1, 60.5, 95, 23.50832656},
		{"CDC weight, boy 5 y", &cdcWeight, 1, 60.5, 3, 14.85692044},
		{"CDC weight, boy 10 y", &cdcWeight, 1, 120.5, 97, 49.41515104},
		{"CDC weight, boy 15 y", &cdcWeight, 1, 180.5, 10, 45.49646217},
		{"CDC stature, girl 8 y", &cdcStature, 0, 96.5, 5, 118.5443251},
		{"CDC stature, girl 8 y", &cdcStature, 0, 96.5, 95, 137.8974887},
		{"CDC stature, girl 12.5 y", &cdcStature, 0, 150.5, 25, 149.9058698},
		{"CDC BMI, boy 12 y", &cdcBMI, 1, 144.5, 95, 24.22984982},
		{"CDC BMI, boy 12 y", &cdcBMI, 1, 144.5, 90, 22.13290433},
		{"CDC BMI, girl 5 y", &cdcBMI, 0, 60.5, 10, 13.81725538},
		{"WHO weight, girl 12 mo", &whoWeight, 0, 12, 5, 7.342376},
		{"WHO weight, girl 12 mo", &whoWeight, 0, 12, 95, 10.99531},
		{"WHO weight, girl 6 mo", &whoWeight, 0, 6, 25, 6.72212},
		{"WHO length, boy 6 mo", &whoLength, 1, 6, 5, 64.10314},
		{"WHO length, boy 12 mo", &whoLength, 1, 12, 90, 78.79407},
		{"WHO head, girl 12 mo", &whoHead, 0, 12, 95, 47.13188},
	}
	for _, c := range cases {
		r, ok := lmsAt(c.table[c.sex], c.age)
		if !ok {
			t.Fatalf("%s: no row at %v months", c.name, c.age)
		}
		got := r.value(centileZ[c.centile])
		if math.Abs(got-c.want)/c.want > 1e-5 {
			t.Errorf("%s, %s centile: %.6f, published %.6f", c.name, ordinal(c.centile), got, c.want)
		}
		// And back: the published value is at that centile.
		if z := r.z(c.want); math.Abs(z-centileZ[c.centile]) > 1e-3 {
			t.Errorf("%s: z of the published %s centile is %.4f", c.name, ordinal(c.centile), z)
		}
	}
	if pt := percentileText(0); pt != "50th percentile" {
		t.Errorf("z 0 is %q", pt)
	}
	if pt := percentileText(-2.5); pt != "below the 1st percentile" {
		t.Errorf("z −2.5 is %q", pt)
	}
	if pt := percentileText(1.0364); pt != "85th percentile" {
		t.Errorf("z 1.0364 is %q", pt)
	}
}

// The embedded AAP 2017 tables give the guideline's Table 3 screening
// values: the 90th percentile at the 5th height percentile, ages 1 to 12.
func TestBPTablesMatchGuidelineTable3(t *testing.T) {
	table3 := map[string][12][2]float64{
		"male":   {{98, 52}, {100, 55}, {101, 58}, {102, 60}, {103, 63}, {105, 66}, {106, 68}, {107, 69}, {107, 70}, {108, 72}, {110, 74}, {113, 75}},
		"female": {{98, 54}, {101, 58}, {102, 60}, {103, 62}, {104, 64}, {105, 67}, {106, 68}, {107, 69}, {108, 71}, {109, 72}, {111, 74}, {114, 75}},
	}
	for sex, rows := range table3 {
		si := map[string]int{"female": 0, "male": 1}[sex]
		for age := 1; age <= 12; age++ {
			got := flynn2017[si][age-1][1][0]
			if got != rows[age-1] {
				t.Errorf("%s %d y: 90th at 5th height %v, Table 3 %v", sex, age, got, rows[age-1])
			}
		}
	}
	// Every row is ordered: 50th < 90th < 95th, and rises with height.
	for si := 0; si < 2; si++ {
		for a := 0; a < 12; a++ {
			for h := 0; h < 7; h++ {
				r := flynn2017[si][a]
				if !(r[0][h][0] < r[1][h][0] && r[1][h][0] < r[2][h][0] && r[0][h][1] < r[1][h][1] && r[1][h][1] < r[2][h][1]) {
					t.Errorf("sex %d age %d height %d is out of order: %v", si, a+1, h, r)
				}
			}
		}
	}
}

func TestBPStages(t *testing.T) {
	cases := []struct {
		sex      string
		age, hp  float64
		sys, dia float64
		want     string
	}{
		{"male", 13.2, 50, 119, 79, "Normal"},
		{"male", 13.2, 50, 125, 78, "Elevated"},
		{"male", 15, 50, 125, 82, "Stage 1"},
		{"female", 16, 50, 141, 70, "Stage 2"},
		{"female", 14, 50, 118, 90, "Stage 2"},
		// A 10-year-old boy at the 50th height percentile: 90th 112/74,
		// 95th 116/77, so stage 2 from 128/89.
		{"male", 10.5, 50, 111, 60, "Normal"},
		{"male", 10.5, 50, 113, 60, "Elevated"},
		{"male", 10.5, 50, 116, 60, "Stage 1"},
		{"male", 10.5, 50, 127, 60, "Stage 1"},
		{"male", 10.5, 50, 128, 60, "Stage 2"},
		{"male", 10.5, 50, 100, 89, "Stage 2"},
		// The adult caps apply where they are lower: a tall 12-year-old's
		// 95th percentile + 12 passes 140, so 140 is stage 2.
		{"male", 12, 95, 140, 60, "Stage 2"},
	}
	for _, c := range cases {
		l := bpLimitsFor(c.sex, c.age, c.hp)
		st := max(stageOf(c.sys, l.sys), stageOf(c.dia, l.dia))
		if bpStages[st].name != c.want {
			t.Errorf("%s %v y, %v/%v: %s, want %s (limits %v)", c.sex, c.age, c.sys, c.dia, bpStages[st].name, c.want, l)
		}
	}
	if l := bpLimitsFor("male", 10.5, 50); l.sys[0] != 112 || l.sys[1] != 116 || l.dia[2] != 89 {
		t.Errorf("10-year-old boy, 50th height: %v", l)
	}
}

// Each kind refuses what it cannot draw, naming the path and the fix.
func TestRefusals(t *testing.T) {
	cases := []struct {
		kind string
		spec string
		want []string
	}{
		{"line_chart", `{"series":[]}`, []string{"series: needs at least one entry"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2],[3]]}]}`, []string{"series[0].points[1]: must be [x, y]"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2]]},{"label":"a","points":[[1,3]]}]}`, []string{"series[1].label", "each series needs its own label"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,-2],[2,3]]}],"y_axis":{"scale":"log"}}`, []string{"y_axis", "only positive numbers", "use scale linear"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2],[12,3]]}],"x_axis":{"max":10}}`, []string{"series[0].points[1]", "outside x_axis", "widen"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2]]}],"x_axis":{"min":5,"max":1}}`, []string{"x_axis: min (5) must be less than max (1)"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2]]}],"colour":"red"}`, []string{`field "colour"`, "series"}},
		{"line_chart", `{"series":[{"label":"a","points":[[1,2]]}],"bands":[{"axis":"z","from":1,"to":2}]}`, []string{"bands[0].axis", "one of x, y"}},
		{"bar_chart", `{"categories":["A","B"],"series":[{"label":"x","values":[1]}]}`, []string{"series[0].values", "has 1 values but there are 2 categories"}},
		{"bar_chart", `{"categories":["A","B"],"stacked":true,"series":[{"label":"x","values":[1,2],"errors":[1,1]}]}`, []string{"series[0].errors", "stacked"}},
		{"bar_chart", `{"categories":["A","B"],"series":[{"values":[1,2]}],"value_axis":{"scale":"log"}}`, []string{"value_axis.scale", "log is not available"}},
		{"bar_chart", `{"categories":["Paracetamol and ibuprofen alternating","B","C","D","E","F","G","H"],"series":[{"values":[1,2,3,4,5,6,7,8]}]}`, []string{"categories[0]", "orientation horizontal"}},
		{"scatter_chart", `{"series":[{"label":"a","points":[[1,2],[1,3]],"fit":"linear"}]}`, []string{"series[0].fit", "at least three points"}},
		{"area_chart", `{"stacked":true,"series":[{"label":"a","points":[[1,2],[2,3]]},{"label":"b","points":[[1,2],[3,3]]}]}`, []string{"series[1].points[1]", "same x values"}},
		{"area_chart", `{"series":[{"label":"a","points":[[2,2],[1,3]]}]}`, []string{"series[0].points[1]", "increasing x"}},
		{"box_plot", `{"groups":[{"label":"A","min":1,"q1":3,"median":2,"q3":4,"max":5}]}`, []string{"groups[0]", "min ≤ q1 ≤ median ≤ q3 ≤ max"}},
		{"box_plot", `{"groups":[{"label":"A","min":1,"q1":2,"median":3,"q3":4}]}`, []string{"groups[0]", "missing max"}},
		{"box_plot", `{"groups":[{"label":"A","values":[1,2,3],"median":2}]}`, []string{"groups[0]", "either values"}},
		{"growth_chart", `{"sex":"female","measure":"weight_for_age","age_unit":"months","points":[[6,7.5],[12,45]]}`, []string{"points[1]", "SD from the median", "check the unit", "kg"}},
		{"growth_chart", `{"sex":"male","measure":"weight_for_age","age_unit":"months","points":[[6,7.5],[30,13]]}`, []string{"points:", "WHO", "CDC", "two charts"}},
		{"growth_chart", `{"sex":"male","measure":"length_for_age","age_unit":"years","points":[[3,95]]}`, []string{"points:", "stature_for_age"}},
		{"growth_chart", `{"sex":"boy","measure":"weight","age_unit":"weeks","points":[[3,95]]}`, []string{"sex", "female, male", "measure", "age_unit"}},
		{"growth_chart", `{"sex":"male","measure":"stature_for_age","age_unit":"years","points":[[5,110],[4,100]]}`, []string{"points[1]", "increasing age"}},
		{"bp_chart", `{"sex":"male","readings":[{"age":10,"systolic":110,"diastolic":70}]}`, []string{"height_percentile", "required", "growth chart"}},
		{"bp_chart", `{"sex":"male","readings":[{"age":14,"systolic":70,"diastolic":110}]}`, []string{"readings[0]", "swapped"}},
		{"bp_chart", `{"sex":"female","height_percentile":50,"readings":[{"age":0.5,"systolic":90,"diastolic":50}]}`, []string{"readings[0].age", "start at 1 year"}},
		{"zone_chart", `{"boundaries":[{"points":[[0,1],[10,2]]}],"zones":[{"label":"Low"}]}`, []string{"zones", "one more zone than boundaries"}},
		{"zone_chart", `{"boundaries":[{"points":[[0,1],[10,5]]},{"points":[[0,2],[10,4]]}],"zones":[{"label":"a"},{"label":"b"},{"label":"c"}]}`, []string{"boundaries[1]", "crosses boundaries[0]"}},
		{"zone_chart", `{"boundaries":[{"points":[[0,1],[10,5]]},{"points":[[0,2],[8,6]]}],"zones":[{"label":"a"},{"label":"b"},{"label":"c"}]}`, []string{"boundaries[1].points", "same x range"}},
		{"zone_chart", `{"boundaries":[{"points":[[0,3],[10,5]]},{"points":[[0,1],[10,2]]}],"zones":[{"label":"a"},{"label":"b"},{"label":"c"}]}`, []string{"lowest to highest"}},
		{"zone_chart", `{"boundaries":[{"points":[[0,1],[10,5]]}],"zones":[{"label":"a"},{"label":"b"}],"points":[{"x":12,"y":3}]}`, []string{"points[0].x", "outside the boundaries' x range"}},
		{"dose_response", `{"curves":[{"label":"a","emax":100,"ec50":-1}]}`, []string{"curves[0].ec50", "greater than zero"}},
		{"dose_response", `{"curves":[{"label":"a","emax":100,"ec50":1,"hill":9}]}`, []string{"curves[0].hill", "0.2 to 5"}},
		{"dose_response", `{"curves":[{"label":"a","emax":100,"ec50":1}],"x_axis":{"min":10,"max":1000}}`, []string{"curves[0].ec50", "outside x_axis"}},
		{"function_plot", `{"functions":[{"expr":"2x"}],"x_axis":{"min":0,"max":1}}`, []string{"functions[0].expr", "at character 2", "2*x"}},
		{"function_plot", `{"functions":[{"expr":"x"}]}`, []string{"x_axis", "needs min and max"}},
		{"function_plot", `{"functions":[{"expr":"x*k"}],"params":{"sin":2},"x_axis":{"min":0,"max":1}}`, []string{"params.sin", "choose another"}},
		{"function_plot", `{"functions":[{"expr":"ln(x)"}],"x_axis":{"min":-5,"max":-1}}`, []string{"functions", "has a value", "positive x"}},
		{"function_plot", `{"functions":[{"expr":"x"}],"x_axis":{"min":0,"max":1},"points":[{"x":0.5,"on":3}]}`, []string{"points[0].on", "function's index"}},
	}
	byName := map[string]core.Kind{}
	for _, k := range Kinds() {
		byName[k.Name] = k
	}
	for _, c := range cases {
		k, ok := byName[c.kind]
		if !ok {
			t.Fatalf("no kind %s", c.kind)
		}
		_, err := k.Parse(json.RawMessage(c.spec))
		var r *core.Refusal
		if !errors.As(err, &r) {
			t.Errorf("%s %s: want a refusal, got %v", c.kind, c.spec, err)
			continue
		}
		msg := strings.Join(r.Problems, " | ")
		for _, w := range c.want {
			if !strings.Contains(msg, w) {
				t.Errorf("%s %s:\n  refusal %q\n  does not say %q", c.kind, c.spec, msg, w)
			}
		}
	}
}

var (
	textRe   = regexp.MustCompile(`<text x="([-\d.]+)" y="([-\d.]+)" font-size="([\d.]+)"([^>]*)>([^<]*)</text>`)
	anchorRe = regexp.MustCompile(`text-anchor="(\w+)"`)
)

type drawnText struct {
	x, y, size float64
	anchor     string
	s          string
}

func texts(svg string) []drawnText {
	var out []drawnText
	for _, m := range textRe.FindAllStringSubmatch(svg, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		size, _ := strconv.ParseFloat(m[3], 64)
		anchor := ""
		if a := anchorRe.FindStringSubmatch(m[4]); a != nil {
			anchor = a[1]
		}
		out = append(out, drawnText{x, y, size, anchor, m[5]})
	}
	return out
}

func drawExample(t *testing.T, k core.Kind, theme *core.Theme) *core.Canvas {
	t.Helper()
	var a struct{ Spec json.RawMessage }
	if err := json.Unmarshal([]byte(k.Example), &a); err != nil {
		t.Fatal(err)
	}
	d, err := k.Parse(a.Spec)
	if err != nil {
		t.Fatalf("%s: %v", k.Name, err)
	}
	c := core.NewCanvas(theme)
	if err := d.Draw(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// Every example's text stays on the figure, and nothing is smaller than
// the smallest size allowed.
func TestExamplesLayout(t *testing.T) {
	for _, k := range Kinds() {
		for _, th := range []*core.Theme{&core.Light, &core.Dark} {
			c := drawExample(t, k, th)
			if c.H <= 0 || c.H > 900 {
				t.Errorf("%s: height %v", k.Name, c.H)
			}
			for _, tx := range texts(c.Body()) {
				if tx.size < core.Small {
					t.Errorf("%s: %q is set at %v, below %v", k.Name, tx.s, tx.size, core.Small)
				}
				w := core.TextWidth(tx.s, tx.size, false)
				x0 := tx.x
				switch tx.anchor {
				case "middle":
					x0 -= w / 2
				case "end":
					x0 -= w
				}
				if x0 < 2 || x0+w > core.Width-2 || tx.y < 0 || tx.y > c.H+1 {
					t.Errorf("%s (%s): %q at %.1f..%.1f, y %.1f runs off the figure (height %.0f)", k.Name, th.Name, tx.s, x0, x0+w, tx.y, c.H)
				}
			}
		}
	}
}

// The labels at the right ends of the growth chart's centile curves never
// touch, however close the curves run.
func TestGrowthEndLabelsApart(t *testing.T) {
	specs := []string{
		`{"sex":"female","measure":"weight_for_age","age_unit":"months","points":[[2,5.6],[15,8]]}`,
		`{"sex":"male","measure":"head_circumference_for_age","age_unit":"months","points":[[0,35],[6,45.8]]}`,
		`{"sex":"female","measure":"stature_for_age","age_unit":"years","points":[[16,160],[19.5,163]]}`,
		`{"sex":"male","measure":"bmi_for_age","age_unit":"years","points":[[2,16.5],[3,16.2]]}`,
	}
	centile := regexp.MustCompile(`^\d+(st|nd|rd|th)$`)
	for _, s := range specs {
		d, err := growthChart.Parse(json.RawMessage(s))
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		c := core.NewCanvas(&core.Light)
		if err := d.Draw(c); err != nil {
			t.Fatal(err)
		}
		var ys []float64
		for _, tx := range texts(c.Body()) {
			if centile.MatchString(tx.s) && tx.x > 300 {
				ys = append(ys, tx.y)
			}
		}
		if len(ys) != 7 {
			t.Errorf("%s: %d centile labels, want 7", s, len(ys))
		}
		for i := range ys {
			for j := i + 1; j < len(ys); j++ {
				if math.Abs(ys[i]-ys[j]) < tickSize {
					t.Errorf("%s: centile labels at %.1f and %.1f overlap", s, ys[i], ys[j])
				}
			}
		}
	}
}

// A function plot breaks its line at an asymptote and at a jump rather
// than joining across it.
func TestFunctionBreaks(t *testing.T) {
	for _, e := range []string{"tan(x)", "floor(x)", "1/(x-1)", "sqrt(x)", "ln(x)"} {
		spec := `{"functions":[{"expr":"` + e + `"}],"x_axis":{"min":-6,"max":6},"y_axis":{"min":-5,"max":5}}`
		d, err := functionPlot.Parse(json.RawMessage(spec))
		if err != nil {
			t.Fatalf("%s: %v", e, err)
		}
		fd := d.(*functionDrawing)
		c := core.NewCanvas(&core.Light)
		p := newPlot(c, fd.x, fd.y)
		p.height = 240
		p.layout(0)
		for _, part := range clipPolyline(fd.sampleFunction(fd.fns[0], p.r), p.r) {
			for i := 1; i < len(part); i++ {
				if dy := math.Abs(part[i].Y - part[i-1].Y); dy > 40 {
					t.Errorf("%s: a segment jumps %.0f units at x %.1f: the line is joined across a break", e, dy, fd.x.value(part[i].X))
				}
			}
		}
	}
}

func TestQuartiles(t *testing.T) {
	b := summarise("x", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 40})
	if b.q1 != 3.25 || b.median != 5.5 || b.q3 != 7.75 {
		t.Errorf("quartiles %v %v %v", b.q1, b.median, b.q3)
	}
	if b.max != 9 || len(b.outliers) != 1 || b.outliers[0] != 40 {
		t.Errorf("whisker %v, outliers %v", b.max, b.outliers)
	}
	a, slope, r, ok := leastSquares([]float64{1, 2, 3, 4}, []float64{3, 5, 7, 9})
	if !ok || a != 1 || slope != 2 || math.Abs(r-1) > 1e-12 {
		t.Errorf("least squares %v %v %v", a, slope, r)
	}
}
