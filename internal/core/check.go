package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Refusal is a spec that cannot be drawn, in words the model can correct
// from: every problem found, each with where it is. It goes back to the
// model as the tool's result; it is never an error the person reading the
// answer sees.
type Refusal struct {
	Type     string
	Problems []string
}

func (r *Refusal) Error() string {
	var b strings.Builder
	b.WriteString("The figure was not drawn")
	if r.Type != "" {
		b.WriteString(" (" + r.Type + ")")
	}
	b.WriteString(". ")
	if len(r.Problems) == 1 {
		b.WriteString("Problem: " + r.Problems[0])
	} else {
		b.WriteString("Problems:")
		for i, p := range r.Problems {
			fmt.Fprintf(&b, "\n%d. %s", i+1, p)
		}
	}
	return b.String()
}

// Problems collects what is wrong with a spec, so one refusal names all of
// it rather than one problem per round trip.
type Problems struct {
	list []string
}

// maxProblems is how many a refusal lists; past that the model has enough
// to fix first.
const maxProblems = 12

// Add records a problem at path ("series[1].points[3].y"; "" is the spec
// itself).
func (p *Problems) Add(path, format string, args ...any) {
	if len(p.list) >= maxProblems {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if path != "" {
		msg = path + ": " + msg
	}
	p.list = append(p.list, msg)
}

// Any says whether a problem was recorded.
func (p *Problems) Any() bool { return len(p.list) > 0 }

// Err is the refusal for the recorded problems, or nil.
func (p *Problems) Err() error {
	if len(p.list) == 0 {
		return nil
	}
	return &Refusal{Problems: append([]string{}, p.list...)}
}

// Path joins a path and an index or a field: Path("series", 2) is
// "series[2]", Path("series[2]", "points") is "series[2].points".
func Path(base string, more ...any) string {
	for _, m := range more {
		switch v := m.(type) {
		case int:
			base += fmt.Sprintf("[%d]", v)
		case string:
			if base == "" {
				base = v
			} else {
				base += "." + v
			}
		}
	}
	return base
}

// Decode reads raw into v strictly: a field v does not have is refused
// (naming the fields it does have), as is a value of the wrong JSON type.
// Checks of the values themselves are the kind's own.
func Decode(raw []byte, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return &Refusal{Problems: []string{"spec is missing: give the figure's content as the spec object"}}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return &Refusal{Problems: []string{decodeProblem(err, v)}}
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return &Refusal{Problems: []string{"spec has something after its closing brace"}}
	}
	return nil
}

func decodeProblem(err error, v any) string {
	var ute *json.UnmarshalTypeError
	var se *json.SyntaxError
	switch {
	case errors.As(err, &ute):
		where := "spec"
		if ute.Field != "" {
			where = "spec." + ute.Field
		}
		return fmt.Sprintf("%s: expected %s, got a JSON %s", where, jsonKind(ute.Type), ute.Value)
	case errors.As(err, &se):
		return "spec is not valid JSON: " + se.Error()
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "spec is not valid JSON: it ends before it is complete"
	}
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
		// encoding/json does not say which object held it: name the
		// top level's fields, and say the field may be deeper.
		return fmt.Sprintf("spec has a field %s that this figure type does not take, at the top level or inside one of its entries; the top-level fields are %s, and each entry's fields are listed in the type's description",
			name, fieldList(reflect.TypeOf(v)))
	}
	return "spec could not be read: " + strings.TrimPrefix(msg, "json: ")
}

func jsonKind(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Float64, reflect.Float32:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Struct, reflect.Map:
		return "an object"
	}
	return t.String()
}

// fieldList is the JSON names of a struct's fields, for "its fields are".
func fieldList(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "(none)"
	}
	var names []string
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			names = append(names, tag)
		}
	}
	return strings.Join(names, ", ")
}

// CharsPerWord is how many characters a word of a figure's text is
// counted at, its space included: a text limit is told to the model in
// words (Words), which it can keep to, and checked in characters, which
// it cannot count. Measured 2026-11-10 (61 questions, Together's DeepSeek
// V4.1 Flash): 8 to 19 draw_figure calls a run were refused for a text a
// few characters over a limit given only in characters (a flowchart box of
// 121 for 120).
const CharsPerWord = 7

// Words is a limit of chars characters as the model is told it: the
// characters at CharsPerWord a word, rounded down to a multiple of five
// from ten up, so a text of that many words of the
// model's prose fits; at least one.
func Words(chars int) int {
	w := chars / CharsPerWord
	if w >= 10 {
		w -= w % 5
	}
	return max(1, w)
}

// UpTo is a limit of chars characters in words, for a field's
// description: "at most 17 words" ("at most 1 word").
func UpTo(chars int) string {
	if w := Words(chars); w != 1 {
		return fmt.Sprintf("at most %d words", w)
	}
	return "at most 1 word"
}

// Label checks a short text shown in the figure: required or not, and at
// most max characters. It returns the text trimmed. A text over the limit
// is refused (never cut: the model's words are drawn as written), saying
// in words how long it is and how long it may be, and quoting its start
// so the model finds it.
func (p *Problems) Label(path, s string, required bool, max int) string {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	switch {
	case s == "" && required:
		p.Add(path, "is required")
	case n > max:
		// The words to keep to: the limit told, or fewer when these
		// words are long ones (the characters fitting at their length).
		words := len(strings.Fields(s))
		keep := min(Words(max), max*words/n)
		if keep < 1 {
			p.Add(path, "is one word of %d characters, and %d fit; abbreviate it: %s", n, max, Excerpt(s))
			break
		}
		p.Add(path, "is %d words; keep it to %d or fewer, and put the rest in your answer: %s", words, keep, Excerpt(s))
	case strings.ContainsAny(s, "<>") && strings.Contains(s, "</"):
		p.Add(path, "looks like markup; write plain words, the figure is drawn for you")
	}
	return s
}

// Excerpt is the start of a text, quoted, for a message that names it:
// its first six words, "…" when there is more.
func Excerpt(s string) string {
	words := strings.Fields(s)
	more := ""
	if len(words) > 6 {
		words, more = words[:6], "…"
	}
	out := strings.Join(words, " ")
	if r := []rune(out); len(r) > 60 {
		out, more = string(r[:60]), "…"
	}
	return fmt.Sprintf("%q", out+more)
}

// Count checks a list's length.
func (p *Problems) Count(path string, n, min, max int) {
	switch {
	case n < min && min == 1:
		p.Add(path, "needs at least one entry")
	case n < min:
		p.Add(path, "needs at least %d entries, has %d", min, n)
	case n > max:
		p.Add(path, "has %d entries; at most %d can be drawn legibly, so leave out %d or split it into two figures", n, max, n-max)
	}
}

// Finite checks a number is an ordinary one.
func (p *Problems) Finite(path string, v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		p.Add(path, "must be a finite number")
	}
}

// OneOf checks v is one of allowed; "" passes when optional.
func (p *Problems) OneOf(path, v string, optional bool, allowed ...string) {
	if v == "" && optional {
		return
	}
	for _, a := range allowed {
		if v == a {
			return
		}
	}
	if v == "" {
		p.Add(path, "is required: one of %s", strings.Join(allowed, ", "))
		return
	}
	p.Add(path, "is %q; it must be one of %s", v, strings.Join(allowed, ", "))
}

// Tone checks a colour named by meaning (Theme.Tone).
func (p *Problems) Tone(path, v string) { p.OneOf(path, v, true, Tones...) }

// Unique checks ids are distinct and not empty, returning them as a set.
func (p *Problems) Unique(path string, ids []string) map[string]bool {
	seen := map[string]bool{}
	for i, id := range ids {
		switch {
		case strings.TrimSpace(id) == "":
			p.Add(Path(path, i, "id"), "is required")
		case seen[id]:
			p.Add(Path(path, i, "id"), "%q is used twice; ids must be distinct", id)
		case len(id) > 40:
			p.Add(Path(path, i, "id"), "%s is too long for an id; use a short one, a word or two", Excerpt(id))
		}
		seen[id] = true
	}
	return seen
}
