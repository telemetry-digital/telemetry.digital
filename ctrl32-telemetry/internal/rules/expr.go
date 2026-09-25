package rules

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// A small arithmetic expression language for rule values and derived datastreams:
// numbers, input names, + - * / % ^, unary minus, parentheses and the functions abs, min, max, round, floor, ceil,
// sqrt, clamp(x, lo, hi). No side effects, no strings, no user-defined functions.

type exprNode interface {
	eval(vars map[string]float64) (float64, error)
}

type numNode float64
type varNode string
type unaryNode struct {
	op string
	x  exprNode
}
type binNode struct {
	op   string
	l, r exprNode
}
type callNode struct {
	fn   string
	args []exprNode
}

func (n numNode) eval(map[string]float64) (float64, error) { return float64(n), nil }
func (n varNode) eval(vars map[string]float64) (float64, error) {
	v, ok := vars[string(n)]
	if !ok {
		return 0, fmt.Errorf("input %q has no value", string(n))
	}
	return v, nil
}
func (n unaryNode) eval(vars map[string]float64) (float64, error) {
	v, err := n.x.eval(vars)
	if err != nil {
		return 0, err
	}
	return -v, nil
}
func (n binNode) eval(vars map[string]float64) (float64, error) {
	l, err := n.l.eval(vars)
	if err != nil {
		return 0, err
	}
	r, err := n.r.eval(vars)
	if err != nil {
		return 0, err
	}
	switch n.op {
	case "+":
		return l + r, nil
	case "-":
		return l - r, nil
	case "*":
		return l * r, nil
	case "/":
		if r == 0 {
			return 0, errors.New("division by zero")
		}
		return l / r, nil
	case "%":
		if r == 0 {
			return 0, errors.New("division by zero")
		}
		return math.Mod(l, r), nil
	case "^":
		return math.Pow(l, r), nil
	}
	return 0, fmt.Errorf("unknown operator %q", n.op)
}

var exprFuncs = map[string]int{"abs": 1, "min": 2, "max": 2, "round": 1, "floor": 1, "ceil": 1, "sqrt": 1, "clamp": 3}

func (n callNode) eval(vars map[string]float64) (float64, error) {
	a := make([]float64, len(n.args))
	for i, x := range n.args {
		v, err := x.eval(vars)
		if err != nil {
			return 0, err
		}
		a[i] = v
	}
	switch n.fn {
	case "abs":
		return math.Abs(a[0]), nil
	case "min":
		return math.Min(a[0], a[1]), nil
	case "max":
		return math.Max(a[0], a[1]), nil
	case "round":
		return math.Round(a[0]), nil
	case "floor":
		return math.Floor(a[0]), nil
	case "ceil":
		return math.Ceil(a[0]), nil
	case "sqrt":
		if a[0] < 0 {
			return 0, errors.New("sqrt of a negative number")
		}
		return math.Sqrt(a[0]), nil
	case "clamp":
		return math.Max(a[1], math.Min(a[2], a[0])), nil
	}
	return 0, fmt.Errorf("unknown function %q", n.fn)
}

// Expr is a parsed expression.
type Expr struct {
	root exprNode
	vars []string
	src  string
}

// Vars lists the input names the expression references.
func (e *Expr) Vars() []string { return e.vars }

// String returns the source text.
func (e *Expr) String() string { return e.src }

// Eval evaluates with the given input values; a referenced input without a value is an error.
func (e *Expr) Eval(vars map[string]float64) (float64, error) {
	v, err := e.root.eval(vars)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("result is not a finite number")
	}
	return v, nil
}

type exprParser struct {
	toks []string
	pos  int
	vars map[string]bool
}

// ParseExpr parses an expression (at most 512 characters).
func ParseExpr(src string) (*Expr, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, errors.New("expression is empty")
	}
	if len(src) > 512 {
		return nil, errors.New("expression is too long (max 512 characters)")
	}
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &exprParser{toks: toks, vars: map[string]bool{}}
	root, err := p.parseSum()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("unexpected %q", p.toks[p.pos])
	}
	vars := make([]string, 0, len(p.vars))
	for v := range p.vars {
		vars = append(vars, v)
	}
	return &Expr{root: root, vars: vars, src: src}, nil
}

func tokenize(src string) ([]string, error) {
	var toks []string
	rs := []rune(src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case strings.ContainsRune("+-*/%^(),", c):
			toks = append(toks, string(c))
			i++
		case unicode.IsDigit(c) || c == '.':
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'e' || rs[j] == 'E' || ((rs[j] == '+' || rs[j] == '-') && (rs[j-1] == 'e' || rs[j-1] == 'E'))) {
				j++
			}
			toks = append(toks, string(rs[i:j]))
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			toks = append(toks, string(rs[i:j]))
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	return toks, nil
}

func (p *exprParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *exprParser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *exprParser) parseSum() (exprNode, error) {
	l, err := p.parseProduct()
	if err != nil {
		return nil, err
	}
	for p.peek() == "+" || p.peek() == "-" {
		op := p.next()
		r, err := p.parseProduct()
		if err != nil {
			return nil, err
		}
		l = binNode{op: op, l: l, r: r}
	}
	return l, nil
}

func (p *exprParser) parseProduct() (exprNode, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "*" || p.peek() == "/" || p.peek() == "%" {
		op := p.next()
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l = binNode{op: op, l: l, r: r}
	}
	return l, nil
}

func (p *exprParser) parseUnary() (exprNode, error) {
	if p.peek() == "-" {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: "-", x: x}, nil
	}
	if p.peek() == "+" {
		p.next()
		return p.parseUnary()
	}
	return p.parsePower()
}

func (p *exprParser) parsePower() (exprNode, error) {
	base, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	if p.peek() == "^" {
		p.next()
		exp, err := p.parseUnary() // right-associative
		if err != nil {
			return nil, err
		}
		return binNode{op: "^", l: base, r: exp}, nil
	}
	return base, nil
}

func (p *exprParser) parseAtom() (exprNode, error) {
	t := p.next()
	switch {
	case t == "":
		return nil, errors.New("unexpected end of expression")
	case t == "(":
		x, err := p.parseSum()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, errors.New("missing )")
		}
		return x, nil
	case unicode.IsDigit(rune(t[0])) || t[0] == '.':
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", t)
		}
		return numNode(v), nil
	case unicode.IsLetter(rune(t[0])) || t[0] == '_':
		if p.peek() == "(" {
			n, ok := exprFuncs[t]
			if !ok {
				return nil, fmt.Errorf("unknown function %q", t)
			}
			p.next()
			var args []exprNode
			for {
				a, err := p.parseSum()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if p.peek() == "," {
					p.next()
					continue
				}
				break
			}
			if p.next() != ")" {
				return nil, errors.New("missing ) after function arguments")
			}
			if len(args) != n {
				return nil, fmt.Errorf("%s takes %d argument(s)", t, n)
			}
			return callNode{fn: t, args: args}, nil
		}
		p.vars[t] = true
		return varNode(t), nil
	}
	return nil, fmt.Errorf("unexpected %q", t)
}
