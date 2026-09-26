package smarthome

// A safe subset of the Jinja templates Home Assistant discovery configs use (research R7): {{ expressions }} with
// literals, value, value_json and its members, arithmetic, comparisons, and/or/not, inline if, filters and tests, and
// {% if %}…{% elif %}…{% else %}…{% endif %} blocks. No loops, no access to anything but the variables given.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Template limits.
const (
	MaxTemplateLen   = 2000
	MaxTemplateSteps = 10000
)

type undefined struct{}

// Undefined is the value of a missing variable or member.
var Undefined = undefined{}

// Template is a parsed template.
type Template struct {
	src   string
	nodes []tnode
}

type tnode interface{}

type textNode string
type exprNode struct{ e texpr }
type ifNode struct {
	conds  []texpr
	bodies [][]tnode
	els    []tnode
}

// ParseTemplate checks and parses a template.
func ParseTemplate(src string) (*Template, error) {
	if len(src) > MaxTemplateLen {
		return nil, fmt.Errorf("template longer than %d characters", MaxTemplateLen)
	}
	toks, err := splitTemplate(src)
	if err != nil {
		return nil, err
	}
	p := &tparser{toks: toks}
	nodes, end, err := p.block()
	if err != nil {
		return nil, err
	}
	if end != "" {
		return nil, fmt.Errorf("unexpected {%% %s %%}", end)
	}
	return &Template{src: src, nodes: nodes}, nil
}

type ttok struct {
	kind byte // 't' text, 'e' expression, 's' statement
	s    string
}

func splitTemplate(src string) ([]ttok, error) {
	var out []ttok
	for len(src) > 0 {
		i := strings.Index(src, "{")
		for i >= 0 && i+1 < len(src) && src[i+1] != '{' && src[i+1] != '%' && src[i+1] != '#' {
			j := strings.Index(src[i+1:], "{")
			if j < 0 {
				i = -1
				break
			}
			i += 1 + j
		}
		if i < 0 || i+1 >= len(src) {
			out = append(out, ttok{'t', src})
			break
		}
		if i > 0 {
			out = append(out, ttok{'t', src[:i]})
		}
		open := src[i+1]
		closing := map[byte]string{'{': "}}", '%': "%}", '#': "#}"}[open]
		j := strings.Index(src[i+2:], closing)
		if j < 0 {
			return nil, fmt.Errorf("unclosed {%c", open)
		}
		inner := src[i+2 : i+2+j]
		// whitespace control markers {{- … -}} (the text around is not trimmed; payload templates rarely rely on it)
		inner = strings.TrimSuffix(strings.TrimPrefix(inner, "-"), "-")
		inner = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(inner, "+"), "+"))
		switch open {
		case '{':
			out = append(out, ttok{'e', inner})
		case '%':
			out = append(out, ttok{'s', inner})
		}
		src = src[i+2+j+2:]
	}
	return out, nil
}

type tparser struct {
	toks []ttok
	i    int
}

// block parses nodes until an elif/else/endif statement, which it returns (without consuming its body).
func (p *tparser) block() ([]tnode, string, error) {
	var nodes []tnode
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		switch t.kind {
		case 't':
			nodes = append(nodes, textNode(t.s))
			p.i++
		case 'e':
			e, err := parseTExpr(t.s)
			if err != nil {
				return nil, "", err
			}
			nodes = append(nodes, exprNode{e})
			p.i++
		case 's':
			word, rest, _ := strings.Cut(t.s, " ")
			switch word {
			case "if":
				p.i++
				n, err := p.ifBlock(rest)
				if err != nil {
					return nil, "", err
				}
				nodes = append(nodes, n)
			case "elif", "else", "endif":
				return nodes, t.s, nil
			default:
				return nil, "", fmt.Errorf("unsupported statement {%% %s %%}", word)
			}
		}
	}
	return nodes, "", nil
}

func (p *tparser) ifBlock(cond string) (tnode, error) {
	n := &ifNode{}
	for {
		c, err := parseTExpr(cond)
		if err != nil {
			return nil, err
		}
		body, end, err := p.block()
		if err != nil {
			return nil, err
		}
		n.conds = append(n.conds, c)
		n.bodies = append(n.bodies, body)
		if end == "" {
			return nil, errors.New("missing {% endif %}")
		}
		p.i++
		word, rest, _ := strings.Cut(end, " ")
		switch word {
		case "elif":
			cond = rest
			continue
		case "else":
			els, end2, err := p.block()
			if err != nil {
				return nil, err
			}
			if end2 != "endif" {
				return nil, errors.New("missing {% endif %}")
			}
			p.i++
			n.els = els
			return n, nil
		case "endif":
			return n, nil
		}
	}
}

// Render evaluates the template with the given variables (typically value and value_json).
func (t *Template) Render(vars map[string]any) (string, error) {
	st := &tstate{vars: vars}
	var b strings.Builder
	if err := st.run(t.nodes, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

// RenderPayload renders a template for an MQTT payload: value is the raw payload, value_json its JSON if it parses.
func (t *Template) RenderPayload(payload []byte) (string, error) {
	return t.Render(PayloadVars(payload))
}

// PayloadVars returns the template variables of a received payload.
func PayloadVars(payload []byte) map[string]any {
	vars := map[string]any{"value": string(payload)}
	if j, ok := parseJSON(payload); ok {
		vars["value_json"] = j
	}
	return vars
}

type tstate struct {
	vars  map[string]any
	steps int
}

func (st *tstate) step() error {
	st.steps++
	if st.steps > MaxTemplateSteps {
		return errors.New("template too complex")
	}
	return nil
}

func (st *tstate) run(nodes []tnode, b *strings.Builder) error {
	for _, n := range nodes {
		if err := st.step(); err != nil {
			return err
		}
		switch n := n.(type) {
		case textNode:
			b.WriteString(string(n))
		case exprNode:
			v, err := st.eval(n.e)
			if err != nil {
				return err
			}
			b.WriteString(ToString(v))
		case *ifNode:
			done := false
			for i, c := range n.conds {
				v, err := st.eval(c)
				if err != nil {
					return err
				}
				if Truthy(v) {
					if err := st.run(n.bodies[i], b); err != nil {
						return err
					}
					done = true
					break
				}
			}
			if !done && n.els != nil {
				if err := st.run(n.els, b); err != nil {
					return err
				}
			}
		}
		if b.Len() > 64<<10 {
			return errors.New("template output too long")
		}
	}
	return nil
}

// ---- expressions -----------------------------------------------------------------------------------------------

type texpr interface{}

type tLit struct{ v any }
type tVar struct{ name string }
type tMember struct {
	x   texpr
	key texpr
}
type tUnary struct {
	op string
	x  texpr
}
type tBinary struct {
	op   string
	a, b texpr
}
type tCond struct{ c, a, b texpr }
type tFilter struct {
	x    texpr
	name string
	args []texpr
}
type tTest struct {
	x    texpr
	name string
	neg  bool
}
type tCall struct {
	name string
	args []texpr
}
type tList struct{ items []texpr }

type xtok struct {
	k string // num str id op
	s string
	n float64
}

func lexT(src string) ([]xtok, error) {
	var out []xtok
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9'):
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.' || src[j] == 'e' || src[j] == 'E' ||
				((src[j] == '-' || src[j] == '+') && j > i && (src[j-1] == 'e' || src[j-1] == 'E'))) {
				j++
			}
			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", src[i:j])
			}
			out = append(out, xtok{k: "num", n: f, s: src[i:j]})
			i = j
		case c == '\'' || c == '"':
			j := i + 1
			var b strings.Builder
			for j < len(src) && src[j] != c {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(src[j])
					}
				} else {
					b.WriteByte(src[j])
				}
				j++
			}
			if j >= len(src) {
				return nil, errors.New("unterminated string")
			}
			out = append(out, xtok{k: "str", s: b.String()})
			i = j + 1
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(src) && (src[j] == '_' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			out = append(out, xtok{k: "id", s: src[i:j]})
			i = j
		default:
			two := ""
			if i+1 < len(src) {
				two = src[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "//", "**":
				out = append(out, xtok{k: "op", s: two})
				i += 2
				continue
			}
			if strings.IndexByte("+-*/%<>()[].,|~:=", c) >= 0 {
				out = append(out, xtok{k: "op", s: string(c)})
				i++
				continue
			}
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}
	return out, nil
}

type xparser struct {
	t []xtok
	i int
	n int
}

func parseTExpr(src string) (texpr, error) {
	toks, err := lexT(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, errors.New("empty expression")
	}
	p := &xparser{t: toks}
	e, err := p.cond()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.t) {
		return nil, fmt.Errorf("unexpected %q", p.t[p.i].s)
	}
	return e, nil
}

func (p *xparser) peek(k, s string) bool {
	return p.i < len(p.t) && p.t[p.i].k == k && p.t[p.i].s == s
}
func (p *xparser) accept(k, s string) bool {
	if p.peek(k, s) {
		p.i++
		return true
	}
	return false
}
func (p *xparser) node() error {
	p.n++
	if p.n > 500 {
		return errors.New("expression too long")
	}
	return nil
}

func (p *xparser) cond() (texpr, error) {
	a, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.accept("id", "if") {
		c, err := p.or()
		if err != nil {
			return nil, err
		}
		var b texpr = tLit{Undefined}
		if p.accept("id", "else") {
			if b, err = p.cond(); err != nil {
				return nil, err
			}
		}
		return tCond{c, a, b}, nil
	}
	return a, nil
}

func (p *xparser) or() (texpr, error) {
	a, err := p.and()
	for err == nil && p.accept("id", "or") {
		var b texpr
		if b, err = p.and(); err == nil {
			a = tBinary{"or", a, b}
		}
	}
	return a, err
}

func (p *xparser) and() (texpr, error) {
	a, err := p.not()
	for err == nil && p.accept("id", "and") {
		var b texpr
		if b, err = p.not(); err == nil {
			a = tBinary{"and", a, b}
		}
	}
	return a, err
}

func (p *xparser) not() (texpr, error) {
	if p.accept("id", "not") {
		x, err := p.not()
		return tUnary{"not", x}, err
	}
	return p.cmp()
}

func (p *xparser) cmp() (texpr, error) {
	a, err := p.concat()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.i < len(p.t) && p.t[p.i].k == "op" && (p.t[p.i].s == "==" || p.t[p.i].s == "!=" || p.t[p.i].s == "<" || p.t[p.i].s == ">" || p.t[p.i].s == "<=" || p.t[p.i].s == ">="):
			op := p.t[p.i].s
			p.i++
			b, err := p.concat()
			if err != nil {
				return nil, err
			}
			a = tBinary{op, a, b}
		case p.peek("id", "in"):
			p.i++
			b, err := p.concat()
			if err != nil {
				return nil, err
			}
			a = tBinary{"in", a, b}
		case p.peek("id", "not") && p.i+1 < len(p.t) && p.t[p.i+1].s == "in":
			p.i += 2
			b, err := p.concat()
			if err != nil {
				return nil, err
			}
			a = tUnary{"not", tBinary{"in", a, b}}
		case p.peek("id", "is"):
			p.i++
			neg := p.accept("id", "not")
			if p.i >= len(p.t) || p.t[p.i].k != "id" {
				return nil, errors.New("test name expected after is")
			}
			name := p.t[p.i].s
			p.i++
			a = tTest{a, name, neg}
		default:
			return a, nil
		}
	}
}

func (p *xparser) concat() (texpr, error) {
	a, err := p.add()
	for err == nil && p.accept("op", "~") {
		var b texpr
		if b, err = p.add(); err == nil {
			a = tBinary{"~", a, b}
		}
	}
	return a, err
}

func (p *xparser) add() (texpr, error) {
	a, err := p.mul()
	for err == nil && (p.peek("op", "+") || p.peek("op", "-")) {
		op := p.t[p.i].s
		p.i++
		var b texpr
		if b, err = p.mul(); err == nil {
			a = tBinary{op, a, b}
		}
	}
	return a, err
}

func (p *xparser) mul() (texpr, error) {
	a, err := p.unary()
	for err == nil && (p.peek("op", "*") || p.peek("op", "/") || p.peek("op", "//") || p.peek("op", "%") || p.peek("op", "**")) {
		op := p.t[p.i].s
		p.i++
		var b texpr
		if b, err = p.unary(); err == nil {
			a = tBinary{op, a, b}
		}
	}
	return a, err
}

func (p *xparser) unary() (texpr, error) {
	if p.accept("op", "-") {
		x, err := p.unary()
		return tUnary{"-", x}, err
	}
	if p.accept("op", "+") {
		return p.unary()
	}
	return p.filtered()
}

func (p *xparser) filtered() (texpr, error) {
	x, err := p.postfix()
	for err == nil && p.accept("op", "|") {
		if p.i >= len(p.t) || p.t[p.i].k != "id" {
			return nil, errors.New("filter name expected after |")
		}
		f := tFilter{x: x, name: p.t[p.i].s}
		p.i++
		if p.accept("op", "(") {
			if f.args, err = p.args(); err != nil {
				return nil, err
			}
		}
		if _, ok := filters[f.name]; !ok {
			return nil, fmt.Errorf("unsupported filter %q", f.name)
		}
		x = f
	}
	return x, err
}

func (p *xparser) args() ([]texpr, error) {
	var out []texpr
	if p.accept("op", ")") {
		return out, nil
	}
	for {
		// keyword arguments (e.g. round(1, 'floor') or default(0, true)) are accepted positionally
		if p.i+1 < len(p.t) && p.t[p.i].k == "id" && p.t[p.i+1].s == "=" {
			p.i += 2
		}
		a, err := p.cond()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
		if p.accept("op", ")") {
			return out, nil
		}
		if !p.accept("op", ",") {
			return nil, errors.New("expected , or )")
		}
	}
}

func (p *xparser) postfix() (texpr, error) {
	x, err := p.primary()
	for err == nil {
		switch {
		case p.accept("op", "."):
			if p.i >= len(p.t) || (p.t[p.i].k != "id" && p.t[p.i].k != "num") {
				return nil, errors.New("member name expected after .")
			}
			x = tMember{x, tLit{p.t[p.i].s}}
			p.i++
		case p.accept("op", "["):
			k, err := p.cond()
			if err != nil {
				return nil, err
			}
			if !p.accept("op", "]") {
				return nil, errors.New("expected ]")
			}
			x = tMember{x, k}
		default:
			return x, nil
		}
		if err = p.node(); err != nil {
			return nil, err
		}
	}
	return x, err
}

var constants = map[string]any{"true": true, "True": true, "false": false, "False": false, "none": nil, "None": nil}

func (p *xparser) primary() (texpr, error) {
	if err := p.node(); err != nil {
		return nil, err
	}
	if p.i >= len(p.t) {
		return nil, errors.New("unexpected end of expression")
	}
	t := p.t[p.i]
	p.i++
	switch t.k {
	case "num":
		return tLit{t.n}, nil
	case "str":
		return tLit{t.s}, nil
	case "id":
		if v, ok := constants[t.s]; ok {
			return tLit{v}, nil
		}
		if p.accept("op", "(") {
			if _, ok := filters[t.s]; !ok {
				return nil, fmt.Errorf("unsupported function %q", t.s)
			}
			args, err := p.args()
			return tCall{t.s, args}, err
		}
		return tVar{t.s}, nil
	case "op":
		switch t.s {
		case "(":
			e, err := p.cond()
			if err != nil {
				return nil, err
			}
			if !p.accept("op", ")") {
				return nil, errors.New("expected )")
			}
			return e, nil
		case "[":
			var items []texpr
			if !p.accept("op", "]") {
				for {
					e, err := p.cond()
					if err != nil {
						return nil, err
					}
					items = append(items, e)
					if p.accept("op", "]") {
						break
					}
					if !p.accept("op", ",") {
						return nil, errors.New("expected , or ]")
					}
				}
			}
			return tList{items}, nil
		}
	}
	return nil, fmt.Errorf("unexpected %q", t.s)
}

func (st *tstate) eval(e texpr) (any, error) {
	if err := st.step(); err != nil {
		return nil, err
	}
	switch e := e.(type) {
	case tLit:
		return e.v, nil
	case tVar:
		if v, ok := st.vars[e.name]; ok {
			return v, nil
		}
		return Undefined, nil
	case tList:
		out := make([]any, len(e.items))
		for i, it := range e.items {
			v, err := st.eval(it)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case tMember:
		x, err := st.eval(e.x)
		if err != nil {
			return nil, err
		}
		k, err := st.eval(e.key)
		if err != nil {
			return nil, err
		}
		switch x := x.(type) {
		case map[string]any:
			if v, ok := x[ToString(k)]; ok {
				return v, nil
			}
		case []any:
			if f, ok := toNum(k); ok {
				i := int(f)
				if i < 0 {
					i += len(x)
				}
				if i >= 0 && i < len(x) {
					return x[i], nil
				}
			}
		}
		return Undefined, nil
	case tUnary:
		x, err := st.eval(e.x)
		if err != nil {
			return nil, err
		}
		if e.op == "not" {
			return !Truthy(x), nil
		}
		f, ok := toNum(x)
		if !ok {
			return nil, fmt.Errorf("cannot negate %s", ToString(x))
		}
		return -f, nil
	case tCond:
		c, err := st.eval(e.c)
		if err != nil {
			return nil, err
		}
		if Truthy(c) {
			return st.eval(e.a)
		}
		return st.eval(e.b)
	case tBinary:
		return st.binary(e)
	case tTest:
		x, err := st.eval(e.x)
		if err != nil {
			return nil, err
		}
		var r bool
		switch e.name {
		case "defined":
			r = x != Undefined
		case "undefined":
			r = x == Undefined
		case "none":
			r = x == nil
		case "number":
			_, r = x.(float64)
		case "string":
			_, r = x.(string)
		case "mapping":
			_, r = x.(map[string]any)
		default:
			return nil, fmt.Errorf("unsupported test %q", e.name)
		}
		return r != e.neg, nil
	case tFilter:
		x, err := st.eval(e.x)
		if err != nil {
			return nil, err
		}
		args, err := st.evalAll(e.args)
		if err != nil {
			return nil, err
		}
		return filters[e.name](x, args)
	case tCall:
		args, err := st.evalAll(e.args)
		if err != nil {
			return nil, err
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("%s() needs an argument", e.name)
		}
		return filters[e.name](args[0], args[1:])
	}
	return nil, errors.New("bad expression")
}

func (st *tstate) evalAll(es []texpr) ([]any, error) {
	out := make([]any, len(es))
	for i, a := range es {
		v, err := st.eval(a)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (st *tstate) binary(e tBinary) (any, error) {
	a, err := st.eval(e.a)
	if err != nil {
		return nil, err
	}
	switch e.op {
	case "and":
		if !Truthy(a) {
			return a, nil
		}
		return st.eval(e.b)
	case "or":
		if Truthy(a) {
			return a, nil
		}
		return st.eval(e.b)
	}
	b, err := st.eval(e.b)
	if err != nil {
		return nil, err
	}
	switch e.op {
	case "~":
		return ToString(a) + ToString(b), nil
	case "==":
		return equal(a, b), nil
	case "!=":
		return !equal(a, b), nil
	case "in":
		switch b := b.(type) {
		case string:
			return strings.Contains(b, ToString(a)), nil
		case []any:
			for _, x := range b {
				if equal(a, x) {
					return true, nil
				}
			}
			return false, nil
		case map[string]any:
			_, ok := b[ToString(a)]
			return ok, nil
		}
		return false, nil
	}
	if e.op == "+" {
		if as, ok := a.(string); ok {
			if bs, ok := b.(string); ok {
				return as + bs, nil
			}
		}
	}
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if !ok1 || !ok2 {
		if e.op == "<" || e.op == ">" || e.op == "<=" || e.op == ">=" {
			as, ok1 := a.(string)
			bs, ok2 := b.(string)
			if ok1 && ok2 {
				c := strings.Compare(as, bs)
				return map[string]bool{"<": c < 0, ">": c > 0, "<=": c <= 0, ">=": c >= 0}[e.op], nil
			}
		}
		return nil, fmt.Errorf("%s needs numbers, got %s and %s", e.op, ToString(a), ToString(b))
	}
	switch e.op {
	case "+":
		return x + y, nil
	case "-":
		return x - y, nil
	case "*":
		return x * y, nil
	case "/":
		if y == 0 {
			return nil, errors.New("division by zero")
		}
		return x / y, nil
	case "//":
		if y == 0 {
			return nil, errors.New("division by zero")
		}
		return math.Floor(x / y), nil
	case "%":
		if y == 0 {
			return nil, errors.New("division by zero")
		}
		return math.Mod(x, y), nil
	case "**":
		return math.Pow(x, y), nil
	case "<":
		return x < y, nil
	case ">":
		return x > y, nil
	case "<=":
		return x <= y, nil
	case ">=":
		return x >= y, nil
	}
	return nil, fmt.Errorf("unknown operator %s", e.op)
}

func equal(a, b any) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x == y
		}
		return false
	}
	switch a.(type) {
	case map[string]any, []any:
		return false
	}
	switch b.(type) {
	case map[string]any, []any:
		return false
	}
	return a == b
}

type filterFn func(x any, args []any) (any, error)

var filters map[string]filterFn

func init() {
	num := func(x any, args []any, round bool) (any, error) {
		if f, ok := toNum(x); ok {
			if round {
				return math.Trunc(f), nil
			}
			return f, nil
		}
		if len(args) > 0 {
			return args[0], nil
		}
		return nil, fmt.Errorf("%q is not a number", ToString(x))
	}
	filters = map[string]filterFn{
		"float": func(x any, a []any) (any, error) { return num(x, a, false) },
		"int":   func(x any, a []any) (any, error) { return num(x, a, true) },
		"round": func(x any, a []any) (any, error) {
			f, ok := toNum(x)
			if !ok {
				return nil, fmt.Errorf("%q is not a number", ToString(x))
			}
			p := 0.0
			if len(a) > 0 {
				p, _ = toNum(a[0])
			}
			m := math.Pow(10, p)
			method := "common"
			if len(a) > 1 {
				method = ToString(a[1])
			}
			switch method {
			case "floor":
				return math.Floor(f*m) / m, nil
			case "ceil":
				return math.Ceil(f*m) / m, nil
			}
			return math.Round(f*m) / m, nil
		},
		"abs": func(x any, a []any) (any, error) {
			f, ok := toNum(x)
			if !ok {
				return nil, fmt.Errorf("%q is not a number", ToString(x))
			}
			return math.Abs(f), nil
		},
		"default": func(x any, a []any) (any, error) {
			if x == Undefined || (len(a) > 1 && Truthy(a[1]) && !Truthy(x)) {
				if len(a) > 0 {
					return a[0], nil
				}
				return "", nil
			}
			return x, nil
		},
		"lower":  func(x any, a []any) (any, error) { return strings.ToLower(ToString(x)), nil },
		"upper":  func(x any, a []any) (any, error) { return strings.ToUpper(ToString(x)), nil },
		"string": func(x any, a []any) (any, error) { return ToString(x), nil },
		"trim":   func(x any, a []any) (any, error) { return strings.TrimSpace(ToString(x)), nil },
		"bool":   func(x any, a []any) (any, error) { return Truthy(x), nil },
		"length": func(x any, a []any) (any, error) {
			switch x := x.(type) {
			case string:
				return float64(len([]rune(x))), nil
			case []any:
				return float64(len(x)), nil
			case map[string]any:
				return float64(len(x)), nil
			}
			return 0.0, nil
		},
		"replace": func(x any, a []any) (any, error) {
			if len(a) < 2 {
				return nil, errors.New("replace needs two arguments")
			}
			return strings.ReplaceAll(ToString(x), ToString(a[0]), ToString(a[1])), nil
		},
		"min": func(x any, a []any) (any, error) { return minmax(x, a, -1) },
		"max": func(x any, a []any) (any, error) { return minmax(x, a, 1) },
		"tojson": func(x any, a []any) (any, error) {
			return toJSON(x), nil
		},
		"iif": func(x any, a []any) (any, error) {
			if Truthy(x) {
				if len(a) > 0 {
					return a[0], nil
				}
				return true, nil
			}
			if len(a) > 1 {
				return a[1], nil
			}
			return false, nil
		},
	}
	filters["count"] = filters["length"]
	filters["d"] = filters["default"]
	filters["to_json"] = filters["tojson"]
}

func minmax(x any, a []any, sign float64) (any, error) {
	var list []any
	if l, ok := x.([]any); ok && len(a) == 0 {
		list = l
	} else {
		list = append([]any{x}, a...)
	}
	best, found := 0.0, false
	for _, v := range list {
		f, ok := toNum(v)
		if !ok {
			return nil, fmt.Errorf("%q is not a number", ToString(v))
		}
		if !found || (f-best)*sign > 0 {
			best, found = f, true
		}
	}
	if !found {
		return Undefined, nil
	}
	return best, nil
}

// Truthy is Jinja truthiness.
func Truthy(v any) bool {
	switch v := v.(type) {
	case nil, undefined:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

func toNum(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil && !math.IsNaN(f)
	}
	return 0, false
}

// ToString renders a value like Jinja/Python would print it.
func ToString(v any) string {
	switch v := v.(type) {
	case nil:
		return "None"
	case undefined:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "True"
		}
		return "False"
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any, map[string]any:
		return toJSON(v)
	}
	return fmt.Sprint(v)
}

func toJSON(v any) string {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = strconv.Quote(k) + ": " + toJSON(v[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = toJSON(x)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case string:
		return strconv.Quote(v)
	case nil, undefined:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ToString(v)
}
