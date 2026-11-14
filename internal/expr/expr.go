// Package expr reads and evaluates the expressions a function plot draws:
// arithmetic in one variable, x, with named parameters fixed by the spec.
//
// It is a recursive-descent parser to a small tree, evaluated by a switch;
// nothing is compiled, reflected on or run as code. What it does not know it
// refuses, saying where and what to write instead, since the message goes
// back to a model that will correct its expression and try again.
package expr

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on what is read.
const (
	// MaxLen is the longest expression read, in characters.
	MaxLen = 200
	// MaxDepth is the deepest nesting of brackets, calls, signs and
	// exponents.
	MaxDepth = 32
)

// Error is an expression that cannot be read: where (the character, from
// 1) and what to do about it.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string {
	if e.Pos <= 0 {
		return e.Msg
	}
	return fmt.Sprintf("at character %d: %s", e.Pos, e.Msg)
}

// Expr is a parsed expression in x.
type Expr struct {
	root *node
	src  string
}

type op byte

const (
	opNum op = iota
	opX
	opNeg
	opAdd
	opSub
	opMul
	opDiv
	opPow
	opCall
)

type node struct {
	op   op
	num  float64
	fn   string
	args []*node
}

// funcs are the functions an expression may call, with how many
// arguments each takes (-1 is two or more).
var funcs = map[string]int{
	"sin": 1, "cos": 1, "tan": 1, "asin": 1, "acos": 1, "atan": 1,
	"exp": 1, "ln": 1, "log10": 1, "log2": 1, "sqrt": 1, "abs": 1,
	"floor": 1, "ceil": 1, "min": -1, "max": -1,
}

// constants are the names that stand for a number.
var constants = map[string]float64{"pi": math.Pi, "e": math.E}

// Reserved says whether name is taken by the language (a function, a
// constant or x), so a parameter cannot be called it.
func Reserved(name string) bool {
	_, f := funcs[name]
	_, k := constants[name]
	return f || k || name == "x" || name == "log"
}

// ValidName says whether name can be a parameter: a letter, then letters,
// digits or underscores, at most 16 characters.
func ValidName(name string) bool {
	if name == "" || len(name) > 16 {
		return false
	}
	for i, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

// Parse reads src. params are the named numbers it may use besides x, pi
// and e; their values are fixed into the expression.
func Parse(src string, params map[string]float64) (*Expr, error) {
	if strings.TrimSpace(src) == "" {
		return nil, &Error{Msg: "is empty; write an expression in x, such as 2*x^2 - 3"}
	}
	if n := utf8.RuneCountInString(src); n > MaxLen {
		return nil, &Error{Msg: fmt.Sprintf("is %d characters; at most %d are read", n, MaxLen)}
	}
	return parse(src, params)
}

// parse is Parse without the length limit, which String's output (with
// its spaces and written-out numbers) can pass.
func parse(src string, params map[string]float64) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, params: params}
	root, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.unexpected(t)
	}
	return &Expr{root: root, src: src}, nil
}

// Eval is the expression's value at x. A value outside a function's domain
// (sqrt of a negative, ln of zero) is NaN or an infinity, as in package
// math; a plot breaks its line there.
func (e *Expr) Eval(x float64) float64 { return eval(e.root, x) }

// String is the expression written out with the brackets it needs, which
// Parse reads back to the same expression.
func (e *Expr) String() string {
	var b strings.Builder
	write(&b, e.root)
	return b.String()
}

// UsesX says whether the expression depends on x at all.
func (e *Expr) UsesX() bool { return usesX(e.root) }

func usesX(n *node) bool {
	if n.op == opX {
		return true
	}
	for _, a := range n.args {
		if usesX(a) {
			return true
		}
	}
	return false
}

func eval(n *node, x float64) float64 {
	switch n.op {
	case opNum:
		return n.num
	case opX:
		return x
	case opNeg:
		return -eval(n.args[0], x)
	case opAdd:
		return eval(n.args[0], x) + eval(n.args[1], x)
	case opSub:
		return eval(n.args[0], x) - eval(n.args[1], x)
	case opMul:
		return eval(n.args[0], x) * eval(n.args[1], x)
	case opDiv:
		return eval(n.args[0], x) / eval(n.args[1], x)
	case opPow:
		return pow(eval(n.args[0], x), eval(n.args[1], x))
	case opCall:
		a := eval(n.args[0], x)
		switch n.fn {
		case "sin":
			return math.Sin(a)
		case "cos":
			return math.Cos(a)
		case "tan":
			return math.Tan(a)
		case "asin":
			return math.Asin(a)
		case "acos":
			return math.Acos(a)
		case "atan":
			return math.Atan(a)
		case "exp":
			return math.Exp(a)
		case "ln":
			return math.Log(a)
		case "log10":
			return math.Log10(a)
		case "log2":
			return math.Log2(a)
		case "sqrt":
			return math.Sqrt(a)
		case "abs":
			return math.Abs(a)
		case "floor":
			return math.Floor(a)
		case "ceil":
			return math.Ceil(a)
		case "min", "max":
			v := a
			for _, arg := range n.args[1:] {
				b := eval(arg, x)
				if math.IsNaN(b) {
					return b
				}
				if (n.fn == "min" && b < v) || (n.fn == "max" && b > v) {
					v = b
				}
			}
			return v
		}
	}
	return math.NaN()
}

// pow is a^b, with the real odd root of a negative number: (-8)^(1/3) is
// -2, as a student expects, where math.Pow gives NaN.
func pow(a, b float64) float64 {
	if a < 0 && b != math.Trunc(b) {
		// 1/b an odd integer (within rounding) is an odd root.
		inv := 1 / b
		if r := math.Round(inv); math.Abs(inv-r) < 1e-9 && int64(r)%2 != 0 {
			return -math.Pow(-a, b)
		}
	}
	return math.Pow(a, b)
}

// prec is how tightly a node binds, for writing it with only the
// brackets it needs.
func prec(n *node) int {
	switch n.op {
	case opAdd, opSub:
		return 1
	case opMul, opDiv:
		return 2
	case opNeg:
		return 3
	case opPow:
		return 4
	case opNum:
		if n.num < 0 || math.Signbit(n.num) {
			return 3
		}
	}
	return 5
}

func write(b *strings.Builder, n *node) {
	sub := func(child *node, bracket bool) {
		if bracket {
			b.WriteString("(")
		}
		write(b, child)
		if bracket {
			b.WriteString(")")
		}
	}
	switch n.op {
	case opNum:
		b.WriteString(strconv.FormatFloat(n.num, 'g', -1, 64))
	case opX:
		b.WriteString("x")
	case opNeg:
		b.WriteString("-")
		sub(n.args[0], prec(n.args[0]) < 3)
	case opAdd, opSub, opMul, opDiv:
		p := prec(n)
		sub(n.args[0], prec(n.args[0]) < p)
		b.WriteString(map[op]string{opAdd: " + ", opSub: " - ", opMul: " * ", opDiv: " / "}[n.op])
		sub(n.args[1], prec(n.args[1]) <= p)
	case opPow:
		sub(n.args[0], prec(n.args[0]) <= 4)
		b.WriteString("^")
		sub(n.args[1], prec(n.args[1]) < 3)
	case opCall:
		b.WriteString(n.fn + "(")
		for i, a := range n.args {
			if i > 0 {
				b.WriteString(", ")
			}
			write(b, a)
		}
		b.WriteString(")")
	}
}

type tkind byte

const (
	tEOF tkind = iota
	tNum
	tIdent
	tOp // + - * / ^
	tLParen
	tRParen
	tComma
)

type token struct {
	kind tkind
	text string
	num  float64
	pos  int // character, from 1
}

func lex(src string) ([]token, error) {
	var toks []token
	pos := 0 // characters read
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		pos++
		start := pos
		switch {
		case r == ' ' || r == '\t':
			i += size
		case r >= '0' && r <= '9' || r == '.':
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			// An exponent: e or E, an optional sign, digits. "2e" alone is
			// two times e, which is refused below as a missing operator.
			if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < len(src) && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < len(src) && src[k] >= '0' && src[k] <= '9' {
					for k < len(src) && src[k] >= '0' && src[k] <= '9' {
						k++
					}
					j = k
				}
			}
			text := src[i:j]
			v, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsInf(v, 0) {
				return nil, &Error{Pos: start, Msg: fmt.Sprintf("%q is not a number", text)}
			}
			toks = append(toks, token{kind: tNum, text: text, num: v, pos: start})
			pos += j - i - 1
			i = j
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || r == '_'):
			j := i
			for j < len(src) && (isIdent(src[j])) {
				j++
			}
			toks = append(toks, token{kind: tIdent, text: src[i:j], pos: start})
			pos += j - i - 1
			i = j
		case strings.ContainsRune("+-*/^", r):
			if r == '*' && i+1 < len(src) && src[i+1] == '*' {
				return nil, &Error{Pos: start, Msg: "** is not a power here: write ^, as in x^2"}
			}
			toks = append(toks, token{kind: tOp, text: string(r), pos: start})
			i += size
		case r == '(':
			toks = append(toks, token{kind: tLParen, text: "(", pos: start})
			i += size
		case r == ')':
			toks = append(toks, token{kind: tRParen, text: ")", pos: start})
			i += size
		case r == ',':
			toks = append(toks, token{kind: tComma, text: ",", pos: start})
			i += size
		default:
			return nil, &Error{Pos: start, Msg: badChar(r)}
		}
	}
	return append(toks, token{kind: tEOF, pos: pos + 1}), nil
}

func isIdent(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// badChar says what to write instead of a character the language lacks.
func badChar(r rune) string {
	switch r {
	case '×', '·', '⋅':
		return fmt.Sprintf("%q is not an operator here: write * for multiplication", r)
	case '÷':
		return "÷ is not an operator here: write / for division"
	case '−', '–', '—':
		return fmt.Sprintf("%q is not a minus sign here: write the ASCII hyphen -", r)
	case 'π':
		return "π is not read here: write pi"
	case '|':
		return "| is not read here: write abs(x) for an absolute value"
	case '[', ']', '{', '}':
		return fmt.Sprintf("%q is not read here: use round brackets ( )", r)
	case '²', '³':
		return fmt.Sprintf("%q is not read here: write ^, as in x^2", r)
	case '√':
		return "√ is not read here: write sqrt(x)"
	case '=', '<', '>':
		return fmt.Sprintf("%q is not read here: give only the right-hand side, an expression in x such as 2*x + 1", r)
	case '!':
		return "! (factorial) is not available"
	}
	return fmt.Sprintf("%q is not read here; expressions use numbers, x, parameters, + - * / ^, brackets and the listed functions", r)
}

type parser struct {
	toks   []token
	i      int
	params map[string]float64
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func (p *parser) deep(depth int) error {
	if depth > MaxDepth {
		return &Error{Pos: p.peek().pos, Msg: fmt.Sprintf("is nested more than %d deep; simplify it", MaxDepth)}
	}
	return nil
}

// expr is a sum: term (+|- term)*.
func (p *parser) expr(depth int) (*node, error) {
	left, err := p.term(depth)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tOp || (t.text != "+" && t.text != "-") {
			return left, nil
		}
		p.next()
		right, err := p.term(depth)
		if err != nil {
			return nil, err
		}
		o := opAdd
		if t.text == "-" {
			o = opSub
		}
		left = &node{op: o, args: []*node{left, right}}
	}
}

// term is a product: unary (*|/ unary)*.
func (p *parser) term(depth int) (*node, error) {
	left, err := p.unary(depth)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tOp || (t.text != "*" && t.text != "/") {
			return left, nil
		}
		p.next()
		right, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		o := opMul
		if t.text == "/" {
			o = opDiv
		}
		left = &node{op: o, args: []*node{left, right}}
	}
}

// unary is a signed power: -x^2 is -(x^2), as in algebra.
func (p *parser) unary(depth int) (*node, error) {
	t := p.peek()
	if t.kind == tOp && (t.text == "-" || t.text == "+") {
		if err := p.deep(depth + 1); err != nil {
			return nil, err
		}
		p.next()
		inner, err := p.unary(depth + 1)
		if err != nil {
			return nil, err
		}
		if t.text == "+" {
			return inner, nil
		}
		return &node{op: opNeg, args: []*node{inner}}, nil
	}
	return p.power(depth)
}

// power is primary (^ unary)?, right-associative: 2^3^2 is 2^9, and an
// exponent may be signed (e^-x).
func (p *parser) power(depth int) (*node, error) {
	base, err := p.primary(depth)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tOp && t.text == "^" {
		p.next()
		if err := p.deep(depth + 1); err != nil {
			return nil, err
		}
		exp, err := p.unary(depth + 1)
		if err != nil {
			return nil, err
		}
		return &node{op: opPow, args: []*node{base, exp}}, nil
	}
	// Something that can start a value straight after one is a missing
	// operator: 2x, x(x+1), 3 sin(x).
	if t := p.peek(); t.kind == tNum || t.kind == tIdent || t.kind == tLParen {
		return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("an operator is missing before %q: write * for multiplication, as in 2*x", t.text)}
	}
	return base, nil
}

func (p *parser) primary(depth int) (*node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return &node{op: opNum, num: t.num}, nil
	case tLParen:
		if err := p.deep(depth + 1); err != nil {
			return nil, err
		}
		inner, err := p.expr(depth + 1)
		if err != nil {
			return nil, err
		}
		if c := p.next(); c.kind != tRParen {
			return nil, &Error{Pos: c.pos, Msg: fmt.Sprintf("expected ) to close the bracket opened at character %d", t.pos)}
		}
		return inner, nil
	case tIdent:
		return p.ident(t, depth)
	case tEOF:
		return nil, &Error{Pos: t.pos, Msg: "the expression ends where a value was expected"}
	}
	return nil, p.unexpected(t)
}

func (p *parser) ident(t token, depth int) (*node, error) {
	name := t.text
	if name == "log" {
		return nil, &Error{Pos: t.pos, Msg: "log is ambiguous: write ln(x) for the natural logarithm, log10(x) or log2(x)"}
	}
	if arity, ok := funcs[name]; ok {
		open := p.next()
		if open.kind != tLParen {
			return nil, &Error{Pos: open.pos, Msg: fmt.Sprintf("%s is a function: write %s(...), with its argument in brackets", name, name)}
		}
		if err := p.deep(depth + 1); err != nil {
			return nil, err
		}
		var args []*node
		for {
			a, err := p.expr(depth + 1)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			c := p.next()
			if c.kind == tRParen {
				break
			}
			if c.kind != tComma {
				return nil, &Error{Pos: c.pos, Msg: fmt.Sprintf("expected , or ) in the call to %s opened at character %d", name, t.pos)}
			}
		}
		switch {
		case arity == 1 && len(args) != 1:
			return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("%s takes one argument, given %d", name, len(args))}
		case arity == -1 && len(args) < 2:
			return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("%s takes two or more arguments, as in %s(x, 0)", name, name)}
		case len(args) > 8:
			return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("%s takes at most 8 arguments", name)}
		}
		return &node{op: opCall, fn: name, args: args}, nil
	}
	if name == "x" {
		return &node{op: opX}, nil
	}
	if v, ok := constants[name]; ok {
		return &node{op: opNum, num: v}, nil
	}
	if v, ok := p.params[name]; ok {
		return &node{op: opNum, num: v}, nil
	}
	known := "x, pi, e"
	if len(p.params) > 0 {
		names := make([]string, 0, len(p.params))
		for n := range p.params {
			names = append(names, n)
		}
		sort.Strings(names)
		known += " and the parameters " + strings.Join(names, ", ")
	}
	hint := ""
	switch strings.ToLower(name) {
	case "arcsin", "arccos", "arctan":
		hint = fmt.Sprintf(" (write a%s)", strings.ToLower(name)[3:])
	case "sqr", "root":
		hint = " (write sqrt)"
	case "log_e", "loge":
		hint = " (write ln)"
	case "sec", "csc", "cot", "sinh", "cosh", "tanh":
		hint = " (write it from sin, cos, tan and exp)"
	}
	if strings.HasPrefix(name, "x") && len(name) > 1 {
		hint = fmt.Sprintf(" (to multiply, write x*%s)", name[1:])
	}
	return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("%q is not a known name%s; the names available are %s, and the functions %s", name, hint, known, funcList())}
}

func (p *parser) unexpected(t token) error {
	switch t.kind {
	case tRParen:
		return &Error{Pos: t.pos, Msg: "this ) closes no bracket"}
	case tComma:
		return &Error{Pos: t.pos, Msg: "a comma belongs only between a function's arguments, as in max(x, 0)"}
	case tOp:
		return &Error{Pos: t.pos, Msg: fmt.Sprintf("%q needs a value before it", t.text)}
	}
	return &Error{Pos: t.pos, Msg: fmt.Sprintf("%q was not expected here", t.text)}
}

func funcList() string {
	names := make([]string, 0, len(funcs))
	for n := range funcs {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
