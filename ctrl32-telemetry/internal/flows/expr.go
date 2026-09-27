// Package flows runs visual automation flows (spec 004): a closed library of nodes wired into flows, executed inside
// the server. Computation happens only in this package's expression language — no user code (clarified).
package flows

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Limits of the expression language (research R3).
const (
	MaxExprLen   = 1000
	MaxExprNodes = 300
	MaxExprSteps = 10000
	MaxStringLen = 4096
)

// Value is null (nil), float64, string or bool.
type Value = any

// Env resolves names during evaluation.
type Env interface {
	// Lookup returns the value of an identifier: value, topic, ts, quality, source, msg.<field>, flow.<name>.
	Lookup(name string) (Value, bool)
	Location() *time.Location
	Now() time.Time
}

// Expr is a parsed expression.
type Expr struct {
	src  string
	root node
}

func (e *Expr) String() string { return e.src }

// Names returns the identifiers the expression reads.
func (e *Expr) Names() []string {
	var out []string
	var walk func(n node)
	walk = func(n node) {
		switch x := n.(type) {
		case identNode:
			out = append(out, string(x))
		case unaryNode:
			walk(x.x)
		case binNode:
			walk(x.l)
			walk(x.r)
		case condNode:
			walk(x.c)
			walk(x.a)
			walk(x.b)
		case callNode:
			for _, a := range x.args {
				walk(a)
			}
		}
	}
	walk(e.root)
	return out
}

type node interface{}
type litNode struct{ v Value }
type identNode string
type unaryNode struct {
	op string
	x  node
}
type binNode struct {
	op   string
	l, r node
}
type condNode struct{ c, a, b node }
type callNode struct {
	fn   string
	args []node
}

// funcs: name → (min args, max args)
var funcs = map[string][2]int{
	"abs": {1, 1}, "min": {2, 8}, "max": {2, 8}, "round": {1, 2}, "floor": {1, 1}, "ceil": {1, 1}, "sqrt": {1, 1}, "pow": {2, 2},
	"clamp": {3, 3}, "log": {1, 1}, "log10": {1, 1}, "exp": {1, 1},
	"len": {1, 1}, "lower": {1, 1}, "upper": {1, 1}, "trim": {1, 1}, "contains": {2, 2}, "startsWith": {2, 2}, "endsWith": {2, 2},
	"replace": {3, 3}, "substr": {2, 3}, "str": {1, 1}, "num": {1, 1}, "bool": {1, 1}, "fixed": {2, 2},
	"now": {0, 0}, "hour": {0, 1}, "minute": {0, 1}, "weekday": {0, 1}, "day": {0, 1}, "month": {0, 1}, "year": {0, 1}, "fmtTime": {2, 2},
	"if": {3, 3}, "coalesce": {2, 8}, "isNull": {1, 1},
}

// FuncNames lists the functions (for the editor's help).
func FuncNames() []string {
	out := make([]string, 0, len(funcs))
	for k := range funcs {
		out = append(out, k)
	}
	return out
}

// Parse checks and parses an expression.
func Parse(src string) (*Expr, error) {
	if strings.TrimSpace(src) == "" {
		return nil, errors.New("empty expression")
	}
	if len(src) > MaxExprLen {
		return nil, fmt.Errorf("expression longer than %d characters", MaxExprLen)
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	root, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) {
		return nil, fmt.Errorf("unexpected %q", p.toks[p.pos].s)
	}
	if p.count > MaxExprNodes {
		return nil, fmt.Errorf("expression too complex (more than %d parts)", MaxExprNodes)
	}
	return &Expr{src: src, root: root}, nil
}

type tok struct {
	k string // num str ident op
	s string
	n float64
}

func lex(s string) ([]tok, error) {
	var out []tok
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			i += w
		case r >= '0' && r <= '9' || (r == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9'):
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == 'e' || s[j] == 'E' ||
				((s[j] == '+' || s[j] == '-') && j > i && (s[j-1] == 'e' || s[j-1] == 'E'))) {
				j++
			}
			f, err := strconv.ParseFloat(s[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", s[i:j])
			}
			out = append(out, tok{k: "num", s: s[i:j], n: f})
			i = j
		case r == '"' || r == '\'':
			q := byte(r)
			var b strings.Builder
			j := i + 1
			for ; j < len(s) && s[j] != q; j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
					switch s[j] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(s[j])
					}
					continue
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, errors.New("unterminated string")
			}
			out = append(out, tok{k: "str", s: b.String()})
			i = j + 1
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(s) {
				r2, w2 := utf8.DecodeRuneInString(s[j:])
				if unicode.IsLetter(r2) || unicode.IsDigit(r2) || r2 == '_' || (r2 == '.' && j+1 < len(s) && (unicode.IsLetter(rune(s[j+1])) || s[j+1] == '_')) {
					j += w2
					continue
				}
				break
			}
			out = append(out, tok{k: "ident", s: s[i:j]})
			i = j
		default:
			two := ""
			if i+1 < len(s) {
				two = s[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				out = append(out, tok{k: "op", s: two})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/%^<>!?:(),", r) {
				out = append(out, tok{k: "op", s: string(r)})
				i += w
				continue
			}
			return nil, fmt.Errorf("unexpected character %q", r)
		}
	}
	return out, nil
}

type parser struct {
	toks  []tok
	pos   int
	count int
	depth int
}

var prec = map[string]int{"?": 1, "||": 2, "&&": 3, "==": 4, "!=": 4, "<": 5, "<=": 5, ">": 5, ">=": 5, "+": 6, "-": 6, "*": 7, "/": 7, "%": 7, "^": 9}

func (p *parser) peek() *tok {
	if p.pos < len(p.toks) {
		return &p.toks[p.pos]
	}
	return nil
}

func (p *parser) expr(min int) (node, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 60 {
		return nil, errors.New("expression nested too deeply")
	}
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || t.k != "op" {
			return left, nil
		}
		pr, ok := prec[t.s]
		if !ok || pr < min {
			return left, nil
		}
		p.pos++
		p.count++
		if t.s == "?" {
			a, err := p.expr(1)
			if err != nil {
				return nil, err
			}
			if n := p.peek(); n == nil || n.s != ":" {
				return nil, errors.New("missing ':' in ?:")
			}
			p.pos++
			b, err := p.expr(1)
			if err != nil {
				return nil, err
			}
			left = condNode{left, a, b}
			continue
		}
		next := pr + 1
		if t.s == "^" {
			next = pr // right associative
		}
		right, err := p.expr(next)
		if err != nil {
			return nil, err
		}
		left = binNode{t.s, left, right}
	}
}

func (p *parser) unary() (node, error) {
	t := p.peek()
	if t == nil {
		return nil, errors.New("unexpected end of expression")
	}
	p.count++
	if t.k == "op" && (t.s == "-" || t.s == "!" || t.s == "+") {
		p.pos++
		x, err := p.unaryPow()
		if err != nil {
			return nil, err
		}
		if t.s == "+" {
			return x, nil
		}
		return unaryNode{t.s, x}, nil
	}
	return p.primary()
}

// unaryPow lets -2^2 be -(2^2).
func (p *parser) unaryPow() (node, error) {
	x, err := p.unary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t != nil && t.s == "^" {
		p.pos++
		r, err := p.expr(9)
		if err != nil {
			return nil, err
		}
		return binNode{"^", x, r}, nil
	}
	return x, nil
}

func (p *parser) primary() (node, error) {
	t := p.peek()
	p.pos++
	switch t.k {
	case "num":
		return litNode{t.n}, nil
	case "str":
		return litNode{t.s}, nil
	case "ident":
		switch t.s {
		case "true":
			return litNode{true}, nil
		case "false":
			return litNode{false}, nil
		case "null":
			return litNode{nil}, nil
		}
		if n := p.peek(); n != nil && n.s == "(" {
			ar, ok := funcs[t.s]
			if !ok {
				return nil, fmt.Errorf("unknown function %s()", t.s)
			}
			p.pos++
			var args []node
			if n := p.peek(); n != nil && n.s == ")" {
				p.pos++
			} else {
				for {
					a, err := p.expr(1)
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					n := p.peek()
					if n == nil {
						return nil, errors.New("missing ')'")
					}
					p.pos++
					if n.s == ")" {
						break
					}
					if n.s != "," {
						return nil, fmt.Errorf("unexpected %q in arguments", n.s)
					}
				}
			}
			if len(args) < ar[0] || len(args) > ar[1] {
				return nil, fmt.Errorf("%s() takes %s arguments", t.s, arity(ar))
			}
			return callNode{t.s, args}, nil
		}
		if !validIdent(t.s) {
			return nil, fmt.Errorf("unknown name %s (use value, topic, ts, quality, source, msg.<field> or flow.<name>)", t.s)
		}
		return identNode(t.s), nil
	case "op":
		if t.s == "(" {
			x, err := p.expr(1)
			if err != nil {
				return nil, err
			}
			if n := p.peek(); n == nil || n.s != ")" {
				return nil, errors.New("missing ')'")
			}
			p.pos++
			return x, nil
		}
	}
	return nil, fmt.Errorf("unexpected %q", t.s)
}

func arity(a [2]int) string {
	if a[0] == a[1] {
		return strconv.Itoa(a[0])
	}
	return fmt.Sprintf("%d–%d", a[0], a[1])
}

func validIdent(s string) bool {
	switch s {
	case "value", "topic", "ts", "quality", "source":
		return true
	}
	for _, pre := range []string{"msg.", "flow."} {
		if strings.HasPrefix(s, pre) && len(s) > len(pre) && !strings.Contains(s[len(pre):], ".") {
			return true
		}
	}
	return false
}

// ErrSteps is returned when an evaluation exceeds MaxExprSteps.
var ErrSteps = errors.New("expression evaluation limit reached")

type evaluator struct {
	env   Env
	steps int
}

// Eval evaluates the expression.
func (e *Expr) Eval(env Env) (Value, error) {
	ev := &evaluator{env: env}
	return ev.eval(e.root)
}

// EvalBool evaluates and converts to a boolean (null/0/""/false are false).
func (e *Expr) EvalBool(env Env) (bool, error) {
	v, err := e.Eval(env)
	if err != nil {
		return false, err
	}
	return Truthy(v), nil
}

// Truthy is the boolean meaning of a value.
func Truthy(v Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	}
	return false
}

// Num converts to a number (ok false when impossible).
func Num(v Value) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x)
	case int: // catalogue defaults
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

// Str converts to text.
func Str(v Value) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func (ev *evaluator) eval(n node) (Value, error) {
	ev.steps++
	if ev.steps > MaxExprSteps {
		return nil, ErrSteps
	}
	switch x := n.(type) {
	case litNode:
		return x.v, nil
	case identNode:
		v, _ := ev.env.Lookup(string(x))
		return v, nil
	case unaryNode:
		v, err := ev.eval(x.x)
		if err != nil {
			return nil, err
		}
		if x.op == "!" {
			return !Truthy(v), nil
		}
		f, ok := Num(v)
		if !ok {
			return nil, fmt.Errorf("-%s: not a number", Str(v))
		}
		return -f, nil
	case condNode:
		c, err := ev.eval(x.c)
		if err != nil {
			return nil, err
		}
		if Truthy(c) {
			return ev.eval(x.a)
		}
		return ev.eval(x.b)
	case binNode:
		if x.op == "&&" || x.op == "||" {
			l, err := ev.eval(x.l)
			if err != nil {
				return nil, err
			}
			if x.op == "&&" && !Truthy(l) {
				return false, nil
			}
			if x.op == "||" && Truthy(l) {
				return true, nil
			}
			r, err := ev.eval(x.r)
			if err != nil {
				return nil, err
			}
			return Truthy(r), nil
		}
		l, err := ev.eval(x.l)
		if err != nil {
			return nil, err
		}
		r, err := ev.eval(x.r)
		if err != nil {
			return nil, err
		}
		return binary(x.op, l, r)
	case callNode:
		if x.fn == "if" { // lazy
			c, err := ev.eval(x.args[0])
			if err != nil {
				return nil, err
			}
			if Truthy(c) {
				return ev.eval(x.args[1])
			}
			return ev.eval(x.args[2])
		}
		args := make([]Value, len(x.args))
		for i, a := range x.args {
			v, err := ev.eval(a)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return ev.call(x.fn, args)
	}
	return nil, errors.New("bad expression")
}

func binary(op string, l, r Value) (Value, error) {
	switch op {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	case "+":
		if ls, ok := l.(string); ok {
			return limitStr(ls + Str(r))
		}
		if rs, ok := r.(string); ok {
			return limitStr(Str(l) + rs)
		}
	}
	if op == "<" || op == "<=" || op == ">" || op == ">=" {
		if ls, ok := l.(string); ok {
			if rs, ok := r.(string); ok {
				c := strings.Compare(ls, rs)
				return cmp(op, float64(c), 0), nil
			}
		}
		lf, ok1 := Num(l)
		rf, ok2 := Num(r)
		if !ok1 || !ok2 {
			return false, nil // comparisons with null or text are false
		}
		return cmp(op, lf, rf), nil
	}
	lf, ok1 := Num(l)
	rf, ok2 := Num(r)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%s %s %s: not numbers", Str(l), op, Str(r))
	}
	switch op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, errors.New("division by zero")
		}
		return lf / rf, nil
	case "%":
		if rf == 0 {
			return nil, errors.New("division by zero")
		}
		return math.Mod(lf, rf), nil
	case "^":
		return math.Pow(lf, rf), nil
	}
	return nil, fmt.Errorf("unknown operator %s", op)
}

func cmp(op string, a, b float64) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	}
	return a >= b
}

func equal(l, r Value) bool {
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	if lf, ok := l.(float64); ok {
		if rf, ok := Num(r); ok {
			return lf == rf
		}
		return false
	}
	if rf, ok := r.(float64); ok {
		if lf, ok := Num(l); ok {
			return lf == rf
		}
		return false
	}
	return Str(l) == Str(r)
}

func limitStr(s string) (Value, error) {
	if len(s) > MaxStringLen {
		return nil, fmt.Errorf("text longer than %d bytes", MaxStringLen)
	}
	return s, nil
}

func (ev *evaluator) tm(args []Value) time.Time {
	loc := ev.env.Location()
	if loc == nil {
		loc = time.UTC
	}
	if len(args) > 0 {
		if f, ok := Num(args[0]); ok {
			return time.UnixMilli(int64(f * 1000)).In(loc)
		}
	}
	return ev.env.Now().In(loc)
}

func (ev *evaluator) call(fn string, a []Value) (Value, error) {
	nums := func() ([]float64, error) {
		out := make([]float64, len(a))
		for i, v := range a {
			f, ok := Num(v)
			if !ok {
				return nil, fmt.Errorf("%s(): argument %d is not a number", fn, i+1)
			}
			out[i] = f
		}
		return out, nil
	}
	switch fn {
	case "abs", "floor", "ceil", "sqrt", "log", "log10", "exp", "pow", "clamp", "min", "max":
		n, err := nums()
		if err != nil {
			return nil, err
		}
		switch fn {
		case "abs":
			return math.Abs(n[0]), nil
		case "floor":
			return math.Floor(n[0]), nil
		case "ceil":
			return math.Ceil(n[0]), nil
		case "sqrt":
			return math.Sqrt(n[0]), nil
		case "log":
			return math.Log(n[0]), nil
		case "log10":
			return math.Log10(n[0]), nil
		case "exp":
			return math.Exp(n[0]), nil
		case "pow":
			return math.Pow(n[0], n[1]), nil
		case "clamp":
			return math.Max(n[1], math.Min(n[2], n[0])), nil
		case "min", "max":
			m := n[0]
			for _, x := range n[1:] {
				if (fn == "min" && x < m) || (fn == "max" && x > m) {
					m = x
				}
			}
			return m, nil
		}
	case "round":
		n, err := nums()
		if err != nil {
			return nil, err
		}
		d := 0.0
		if len(n) > 1 {
			d = math.Max(0, math.Min(9, n[1]))
		}
		p := math.Pow(10, d)
		return math.Round(n[0]*p) / p, nil
	case "fixed":
		f, ok := Num(a[0])
		d, ok2 := Num(a[1])
		if !ok || !ok2 {
			return nil, errors.New("fixed(): numbers expected")
		}
		dd := int(math.Max(0, math.Min(9, d)))
		p := math.Pow(10, float64(dd))
		return strconv.FormatFloat(math.Round(f*p)/p, 'f', dd, 64), nil // half away from zero, as people expect
	case "len":
		if s, ok := a[0].(string); ok {
			return float64(utf8.RuneCountInString(s)), nil
		}
		return float64(len(Str(a[0]))), nil
	case "lower":
		return strings.ToLower(Str(a[0])), nil
	case "upper":
		return strings.ToUpper(Str(a[0])), nil
	case "trim":
		return strings.TrimSpace(Str(a[0])), nil
	case "contains":
		return strings.Contains(Str(a[0]), Str(a[1])), nil
	case "startsWith":
		return strings.HasPrefix(Str(a[0]), Str(a[1])), nil
	case "endsWith":
		return strings.HasSuffix(Str(a[0]), Str(a[1])), nil
	case "replace":
		return limitStr(strings.ReplaceAll(Str(a[0]), Str(a[1]), Str(a[2])))
	case "substr":
		r := []rune(Str(a[0]))
		from, _ := Num(a[1])
		start := int(math.Max(0, math.Min(float64(len(r)), from)))
		end := len(r)
		if len(a) > 2 {
			n, _ := Num(a[2])
			end = int(math.Max(float64(start), math.Min(float64(len(r)), float64(start)+n)))
		}
		return string(r[start:end]), nil
	case "str":
		return Str(a[0]), nil
	case "num":
		if f, ok := Num(a[0]); ok {
			return f, nil
		}
		return nil, nil
	case "bool":
		return Truthy(a[0]), nil
	case "now":
		return float64(ev.env.Now().UnixMilli()) / 1000, nil
	case "hour":
		return float64(ev.tm(a).Hour()), nil
	case "minute":
		return float64(ev.tm(a).Minute()), nil
	case "weekday": // 1 = Monday … 7 = Sunday
		wd := int(ev.tm(a).Weekday())
		if wd == 0 {
			wd = 7
		}
		return float64(wd), nil
	case "day":
		return float64(ev.tm(a).Day()), nil
	case "month":
		return float64(ev.tm(a).Month()), nil
	case "year":
		return float64(ev.tm(a).Year()), nil
	case "fmtTime":
		layout := Str(a[1])
		for _, r := range [][2]string{{"YYYY", "2006"}, {"MM", "01"}, {"DD", "02"}, {"HH", "15"}, {"mm", "04"}, {"ss", "05"}} {
			layout = strings.ReplaceAll(layout, r[0], r[1])
		}
		return ev.tm(a[:1]).Format(layout), nil
	case "coalesce":
		for _, v := range a {
			if v != nil {
				return v, nil
			}
		}
		return nil, nil
	case "isNull":
		return a[0] == nil, nil
	}
	return nil, fmt.Errorf("unknown function %s()", fn)
}
