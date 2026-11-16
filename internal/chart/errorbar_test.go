package chart

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ifedan-ed/figures-go/internal/core"
)

// An uneven interval is drawn where it lies: the whisker of RR 1.53
// (0.97–2.42) runs from 0.97 to 2.42, not from 1.53 ± one averaged width.
// Each orientation draws its whisker as one line along the value axis,
// whose two ends are the interval's.
func TestAnUnevenIntervalIsDrawnAtItsOwnEnds(t *testing.T) {
	for _, orientation := range []string{"vertical", "horizontal"} {
		spec := `{"categories":["Ward A","Ward B"],"orientation":"` + orientation + `","series":[{"values":[1.53,0.8],"intervals":[[0.97,2.42],null]}],"value_axis":{"min":0,"max":3}}`
		parsed, err := barChart.Parse(json.RawMessage(spec))
		if err != nil {
			t.Fatalf("%s: %v", orientation, err)
		}
		d := parsed.(*barDrawing)
		if got := d.barAt(0, 0); got != (errorBar{0.97, 2.42}) || d.barAt(0, 1).given() {
			t.Fatalf("%s: read as %+v and %+v", orientation, got, d.barAt(0, 1))
		}
		c := core.NewCanvas(&core.Light)
		if err := d.Draw(c); err != nil {
			t.Fatal(err)
		}
		low, high := core.F(d.val.px(0.97)), core.F(d.val.px(2.42))
		var want string
		if orientation == "vertical" {
			want = `y1="` + low + `" x2=`
		} else {
			want = `x1="` + low + `" y1=`
		}
		found := false
		for _, line := range strings.Split(c.Body(), "<line") {
			if strings.Contains(line, want) && (strings.Contains(line, `y2="`+high+`"`) || strings.Contains(line, `x2="`+high+`"`)) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no whisker from %s to %s in\n%s", orientation, low, high, c.Body())
		}
	}
}

// The symmetric form still means value ± error, and reaches the axis's
// range the same way.
func TestASymmetricErrorIsTheValuePlusAndMinusIt(t *testing.T) {
	parsed, err := barChart.Parse(json.RawMessage(`{"categories":["A"],"series":[{"values":[10],"errors":[2]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.(*barDrawing).barAt(0, 0); got != (errorBar{8, 12}) {
		t.Errorf("read as %+v", got)
	}
}

func TestIntervalRefusals(t *testing.T) {
	for spec, says := range map[string][]string{
		`{"categories":["A"],"series":[{"values":[1],"errors":[1],"intervals":[[0,2]]}]}`:               {"series[0].intervals", "not both"},
		`{"categories":["A"],"series":[{"values":[1],"intervals":[[2,0]]}]}`:                            {"series[0].intervals[0]", "low end comes first"},
		`{"categories":["A"],"series":[{"values":[3],"intervals":[[0.97,2.42]]}]}`:                      {"series[0].intervals[0]", "does not hold the bar's value 3"},
		`{"categories":["A"],"series":[{"values":[1],"intervals":[[0.5]]}]}`:                            {"series[0].intervals[0]", "[low, high]"},
		`{"categories":["A","B"],"series":[{"values":[1,2],"intervals":[[0,2]]}]}`:                      {"series[0].intervals", "has 1 values but there are 2 categories"},
		`{"categories":["A"],"series":[{"values":[null],"intervals":[[0,2]]}]}`:                         {"series[0].intervals[0]", "no value"},
		`{"categories":["A","B"],"stacked":true,"series":[{"values":[1,2],"intervals":[[0,2],[1,3]]}]}`: {"series[0].intervals", "stacked"},
		`{"categories":["A"],"series":[{"values":[1],"intervals":[[0,5]]}],"value_axis":{"max":3}}`:     {"value_axis"},
	} {
		_, err := barChart.Parse(json.RawMessage(spec))
		r, ok := err.(*core.Refusal)
		if !ok {
			t.Errorf("%s: want a refusal, got %v", spec, err)
			continue
		}
		msg := strings.Join(r.Problems, " | ")
		for _, w := range says {
			if !strings.Contains(msg, w) {
				t.Errorf("%s:\n  refusal %q\n  does not say %q", spec, msg, w)
			}
		}
	}
}
