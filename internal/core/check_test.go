package core

import (
	"strings"
	"testing"
)

// A limit is told in words, rounded to a whole number of words the model
// can keep to.
func TestWords(t *testing.T) {
	for chars, want := range map[int]int{14: 2, 16: 2, 24: 3, 40: 5, 60: 8, 70: 10, 90: 10, 120: 15, 160: 20, 400: 55, 4000: 570, 3: 1} {
		if got := Words(chars); got != want {
			t.Errorf("Words(%d) = %d, want %d", chars, got, want)
		}
	}
	if UpTo(160) != "at most 20 words" || UpTo(5) != "at most 1 word" {
		t.Fatalf("UpTo: %q, %q", UpTo(160), UpTo(5))
	}
}

// A model that keeps to the words it is told passes the check in
// characters: words of six letters (seven with the space, CharsPerWord)
// fit every limit the figures use.
func TestTheWordsToldFit(t *testing.T) {
	for chars := 10; chars <= 4000; chars++ {
		s := strings.TrimSpace(strings.Repeat("abcdef ", Words(chars)))
		var p Problems
		p.Label("x", s, true, chars)
		if p.Any() {
			t.Fatalf("%d words of six letters are refused at %d characters: %v", Words(chars), chars, p.Err())
		}
	}
}

// A text over its limit is refused in words, with the words to keep to
// and its start quoted; it is never cut.
func TestLabelOverItsLimit(t *testing.T) {
	long := "Give IM adrenaline 10 micrograms/kg into the anterolateral thigh, repeat after five minutes if no improvement, and call for help"
	cases := []struct {
		name, text string
		max        int
		want       []string
	}{
		{"too many words", long, 120, []string{"x: is 20 words; keep it to 15 or fewer", "put the rest in your answer", `"Give IM adrenaline 10 micrograms/kg into…"`}},
		// Few words, but long ones: the words to keep to are fewer than
		// it has, so following the message passes.
		{"long words", "Haemophagocytic lymphohistiocytosis hyperferritinaemia", 40, []string{"is 3 words; keep it to 2 or fewer"}},
		{"one long word", "Pneumonoultramicroscopicsilicovolcanoconiosis", 24, []string{"is one word of 45 characters, and 24 fit; abbreviate it"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p Problems
			got := p.Label("x", "  "+c.text+" ", true, c.max)
			if got != c.text {
				t.Fatalf("the text came back as %q", got)
			}
			msg := p.Err().Error()
			for _, w := range c.want {
				if !strings.Contains(msg, w) {
					t.Errorf("%q does not say %q", msg, w)
				}
			}
			if strings.Contains(msg, "characters;") {
				t.Errorf("%q speaks of characters", msg)
			}
		})
	}
	var p Problems
	if p.Label("x", strings.Repeat("abcdef ", 17), true, 120); p.Any() {
		t.Fatalf("120 characters are refused: %v", p.Err())
	}
}

func TestExcerpt(t *testing.T) {
	if got := Excerpt("one two three"); got != `"one two three"` {
		t.Fatal(got)
	}
	if got := Excerpt("one two three four five six seven"); got != `"one two three four five six…"` {
		t.Fatal(got)
	}
	if got := Excerpt(strings.Repeat("x", 80)); got != `"`+strings.Repeat("x", 60)+`…"` {
		t.Fatal(got)
	}
}

func TestCountSaysHowManyToLeaveOut(t *testing.T) {
	var p Problems
	p.Count("columns", 5, 2, 4)
	if msg := p.Err().Error(); !strings.Contains(msg, "columns: has 5 entries; at most 4") || !strings.Contains(msg, "leave out 1") {
		t.Fatal(msg)
	}
}
