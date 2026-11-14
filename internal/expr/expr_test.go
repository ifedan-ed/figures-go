package expr

import (
	"math"
	"strings"
	"testing"
)

func TestEval(t *testing.T) {
	params := map[string]float64{"a": 2, "k": 0.5, "v_max": 10}
	cases := []struct {
		src  string
		x    float64
		want float64
	}{
		{"1 + 2 * 3", 0, 7},
		{"(1 + 2) * 3", 0, 9},
		{"2^3^2", 0, 512},
		{"-x^2", 3, -9},
		{"(-x)^2", 3, 9},
		{"2^-x", 1, 0.5},
		{"e^-x", 0, 1},
		{"x - -x", 2, 4},
		{"10 / 4 / 5", 0, 0.5},
		{"a*x^2 - 3", 2, 5},
		{"v_max * x / (k + x)", 0.5, 5},
		{"sin(pi/2)", 0, 1},
		{"cos(0) + tan(0)", 0, 1},
		{"asin(1) - pi/2", 0, 0},
		{"acos(1) + atan(0)", 0, 0},
		{"exp(ln(5))", 0, 5},
		{"log10(1000) + log2(8)", 0, 6},
		{"sqrt(16) + abs(-3)", 0, 7},
		{"floor(-1.5) + ceil(1.2)", 0, 0},
		{"min(x, 0, 3)", -2, -2},
		{"max(x, 0)", -2, 0},
		{"1.5e2 + 2E-1", 0, 150.2},
		{".5 + 1.", 0, 1.5},
		{"(-8)^(1/3)", 0, -2},
		{"x^(1/3)", -27, -3},
		{"(-4)^0.5", 0, math.NaN()},
		{"e", 0, math.E},
	}
	for _, c := range cases {
		e, err := Parse(c.src, params)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		got := e.Eval(c.x)
		if math.IsNaN(c.want) {
			if !math.IsNaN(got) {
				t.Errorf("%s at %v = %v, want NaN", c.src, c.x, got)
			}
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s at %v = %v, want %v", c.src, c.x, got, c.want)
		}
		again, err := parse(e.String(), nil)
		if err != nil {
			t.Fatalf("%s written as %s does not read back: %v", c.src, e.String(), err)
		}
		if g := again.Eval(c.x); math.Abs(g-got) > 1e-9 {
			t.Errorf("%s written as %s evaluates to %v, not %v", c.src, e.String(), g, got)
		}
	}
}

func TestDomain(t *testing.T) {
	for src, x := range map[string]float64{"ln(x)": 0, "1/x": 0, "sqrt(x)": -1} {
		e, err := Parse(src, nil)
		if err != nil {
			t.Fatal(err)
		}
		if v := e.Eval(x); !math.IsNaN(v) && !math.IsInf(v, 0) {
			t.Errorf("%s at %v = %v, want NaN or an infinity", src, x, v)
		}
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		src      string
		pos      int
		contains string
	}{
		{"2x", 2, "write * for multiplication"},
		{"x(x+1)", 2, "operator is missing"},
		{"3 sin(x)", 3, "operator is missing"},
		{"x**2", 2, "write ^"},
		{"log(x)", 1, "ln(x)"},
		{"sinx", 1, `"sinx" is not a known name`},
		{"arcsin(x)", 1, "write asin"},
		{"xy", 1, "write x*y"},
		{"y + 1", 1, `"y" is not a known name`},
		{"(x + 1", 7, "expected )"},
		{"x + 1)", 6, "closes no bracket"},
		{"x +", 4, "ends where a value"},
		{"* x", 1, "needs a value before it"},
		{"sin x", 5, "write sin(...)"},
		{"sin(x, 1)", 1, "one argument"},
		{"max(x)", 1, "two or more"},
		{"2 × x", 3, "write * for multiplication"},
		{"x − 1", 3, "ASCII hyphen"},
		{"2π", 2, "write pi"},
		{"|x|", 1, "abs(x)"},
		{"x²", 2, "x^2"},
		{"y = x + 1", 3, "right-hand side"},
		{"1e999", 1, "not a number"},
		{"1..2", 1, "not a number"},
		{"", 0, "is empty"},
		{"1, 2", 2, "comma"},
	}
	for _, c := range cases {
		_, err := Parse(c.src, nil)
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: want an *Error, got %v", c.src, err)
			continue
		}
		if e.Pos != c.pos || !strings.Contains(e.Msg, c.contains) {
			t.Errorf("%q: got position %d %q, want position %d saying %q", c.src, e.Pos, e.Msg, c.pos, c.contains)
		}
	}
}

func TestLimits(t *testing.T) {
	if _, err := Parse(strings.Repeat("x+", 100)+"x", nil); err == nil || !strings.Contains(err.Error(), "at most 200") {
		t.Errorf("a long expression: %v", err)
	}
	deep := strings.Repeat("(", 40) + "x" + strings.Repeat(")", 40)
	if _, err := Parse(deep, nil); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Errorf("a deep expression: %v", err)
	}
	if _, err := Parse(strings.Repeat("-", 60)+"x", nil); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Errorf("many signs: %v", err)
	}
}

func TestNames(t *testing.T) {
	for _, n := range []string{"x", "sin", "pi", "e", "log", "max"} {
		if !Reserved(n) {
			t.Errorf("%s should be reserved", n)
		}
	}
	for n, ok := range map[string]bool{"a": true, "v_max": true, "k2": true, "2k": false, "": false, "é": false, "a-b": false, "abcdefghijklmnopq": false} {
		if ValidName(n) != ok {
			t.Errorf("ValidName(%q) = %v", n, !ok)
		}
	}
}

// Whatever the input, Parse neither panics nor loops, and what it reads it
// writes out in a form it reads back to the same values.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"2*x^2 - 3*x + 1", "sin(x)/x", "max(a, x, -x)", "e^-x^2", "((x))", "1e-3*x", "tan(x)", "floor(x)+ceil(-x)", "x^(1/3)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		e, err := Parse(src, map[string]float64{"a": 1.5})
		if err != nil {
			if _, ok := err.(*Error); !ok {
				t.Fatalf("not an *Error: %v", err)
			}
			return
		}
		again, err := parse(e.String(), nil)
		if err != nil {
			t.Fatalf("%q written as %q does not read back: %v", src, e.String(), err)
		}
		for _, x := range []float64{-2, 0, 0.5, 3} {
			a, b := e.Eval(x), again.Eval(x)
			if !(a == b || (math.IsNaN(a) && math.IsNaN(b))) {
				t.Fatalf("%q at %v: %v, read back %v", src, x, a, b)
			}
		}
	})
}
