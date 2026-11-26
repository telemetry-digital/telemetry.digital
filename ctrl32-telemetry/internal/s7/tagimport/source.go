package tagimport

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Data block sources: STEP 7 V5 (.awl, "DATA_BLOCK DB 5 … STRUCT … END_STRUCT ; BEGIN … END_DATA_BLOCK") and
// TIA Portal (.db / .scl / .udt, "DATA_BLOCK "Name" { S7_Optimized_Access := 'FALSE' } … STRUCT|VAR … BEGIN …
// END_DATA_BLOCK", "TYPE "Name" … STRUCT … END_STRUCT; END_TYPE"). Only declarations are read; start values and the
// assignments after BEGIN are skipped.

type token struct {
	s      string // text; for quoted names without the quotes
	quoted bool
	num    bool
	line   int
}

type lexed struct {
	toks     []token
	comments map[int]string // line → // comment
	attrs    []string       // texts of { … } attribute lists, in order, with the index of the token they precede
	attrAt   []int
}

func lex(src string) lexed {
	out := lexed{comments: map[int]string{}}
	line := 1
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < n && src[i+1] == '/':
			j := i + 2
			for j < n && src[j] != '\n' {
				j++
			}
			if _, ok := out.comments[line]; !ok {
				out.comments[line] = strings.TrimSpace(src[i+2 : j])
			}
			i = j
		case c == '(' && i+1 < n && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*)")
			end := n
			if j >= 0 {
				end = i + 2 + j + 2
			}
			line += strings.Count(src[i:end], "\n")
			i = end
		case c == '{':
			j := strings.IndexByte(src[i:], '}')
			end := n
			if j >= 0 {
				end = i + j + 1
			}
			out.attrs = append(out.attrs, src[i:end])
			out.attrAt = append(out.attrAt, len(out.toks))
			line += strings.Count(src[i:end], "\n")
			i = end
		case c == '"':
			j := i + 1
			for j < n && src[j] != '"' && src[j] != '\n' {
				j++
			}
			out.toks = append(out.toks, token{s: src[i+1 : j], quoted: true, line: line})
			i = j + 1
		case c == '\'':
			j := i + 1
			for j < n && src[j] != '\'' && src[j] != '\n' {
				if src[j] == '$' {
					j++
				}
				j++
			}
			out.toks = append(out.toks, token{s: "'", line: line}) // a string literal: only ever skipped
			i = j + 1
		case c >= '0' && c <= '9':
			j := i
			for j < n && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			out.toks = append(out.toks, token{s: src[i:j], num: true, line: line})
			i = j
		case c == '_' || c == '#' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80:
			j := i + 1
			for j < n && (src[j] == '_' || src[j] >= '0' && src[j] <= '9' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 0x80) {
				j++
			}
			out.toks = append(out.toks, token{s: strings.TrimPrefix(src[i:j], "#"), line: line})
			i = j
		case c == '.' && i+1 < n && src[i+1] == '.':
			out.toks = append(out.toks, token{s: "..", line: line})
			i += 2
		case c == ':' && i+1 < n && src[i+1] == '=':
			out.toks = append(out.toks, token{s: ":=", line: line})
			i += 2
		default:
			out.toks = append(out.toks, token{s: string(c), line: line})
			i++
		}
	}
	return out
}

type parser struct {
	lx  lexed
	pos int
}

func (p *parser) peek() *token {
	if p.pos < len(p.lx.toks) {
		return &p.lx.toks[p.pos]
	}
	return nil
}

func (p *parser) is(words ...string) bool {
	t := p.peek()
	if t == nil || t.quoted {
		return false
	}
	for _, w := range words {
		if strings.EqualFold(t.s, w) {
			return true
		}
	}
	return false
}

func (p *parser) skipLine(line int) {
	for t := p.peek(); t != nil && t.line == line; t = p.peek() {
		p.pos++
	}
}

var reOptimized = regexp.MustCompile(`(?i)S7_Optimized_Access\s*:=\s*'TRUE'`)

// blockName reads the name after DATA_BLOCK or TYPE: "Name", DB 5, DB5, UDT 3, Name.
func (p *parser) blockName() (name string, number int) {
	t := p.peek()
	if t == nil {
		return "", 0
	}
	p.pos++
	if t.quoted {
		return t.s, 0
	}
	up := strings.ToUpper(t.s)
	if up == "DB" || up == "UDT" || up == "FB" || up == "SFB" {
		if n := p.peek(); n != nil && n.num && n.line == t.line {
			p.pos++
			v, _ := strconv.Atoi(n.s)
			return up + n.s, v
		}
	}
	if m := regexp.MustCompile(`^(?i)(DB|UDT)(\d+)$`).FindStringSubmatch(t.s); m != nil {
		v, _ := strconv.Atoi(m[2])
		return strings.ToUpper(m[1]) + m[2], v
	}
	return t.s, 0
}

func parseSource(text string, b *builder) error {
	p := &parser{lx: lex(text)}
	udts := map[string]typ{}
	var blocks []blockDef
	for p.peek() != nil {
		switch {
		case p.is("TYPE"):
			p.pos++
			name, _ := p.blockName()
			for p.peek() != nil && !p.is("STRUCT", "END_TYPE") {
				p.pos++
			}
			if p.is("STRUCT") {
				p.pos++
				fields, err := p.decls(0, "END_STRUCT")
				if err != nil {
					return errors.New("data type " + name + ": " + err.Error())
				}
				udts[udtKey(name)] = typ{Kind: kStruct, Fields: fields}
			}
			for p.peek() != nil && !p.is("END_TYPE") {
				p.pos++
			}
			p.pos++
		case p.is("DATA_BLOCK"):
			start := p.pos
			p.pos++
			d := blockDef{}
			d.Name, d.Number = p.blockName()
			if !strings.HasPrefix(strings.ToUpper(d.Name), "DB") || d.Number == 0 {
				d.Number = 0
			}
			var fields []decl
			body := false
		header:
			for p.peek() != nil {
				t := p.peek()
				switch {
				case p.is("END_DATA_BLOCK", "BEGIN"):
					break header
				case p.is("STRUCT"):
					p.pos++
					f, err := p.decls(0, "END_STRUCT")
					if err != nil {
						return errors.New("block " + d.Name + ": " + err.Error())
					}
					fields, body = append(fields, f...), true
				case p.is("VAR"):
					p.skipLine(t.line) // VAR, VAR RETAIN, VAR NON_RETAIN …
					f, err := p.decls(0, "END_VAR")
					if err != nil {
						return errors.New("block " + d.Name + ": " + err.Error())
					}
					fields, body = append(fields, f...), true
				case p.is("TITLE", "VERSION", "AUTHOR", "FAMILY", "NAME"):
					p.skipLine(t.line)
				case p.is("NON_RETAIN", "RETAIN", "KNOW_HOW_PROTECT", "READ_ONLY", "UNLINKED"), t.s == ";":
					p.pos++
				case !body && (t.quoted || p.is("UDT", "FB", "SFB") || regexp.MustCompile(`^(?i)(UDT|FB|SFB)\d+$`).MatchString(t.s)):
					ref, _ := p.blockName()
					if strings.HasPrefix(strings.ToUpper(ref), "FB") || strings.HasPrefix(strings.ToUpper(ref), "SFB") {
						d.Unknown = "instance data block of " + ref + ": the interface of the function block is not in the file"
					}
					d.Body, body = typ{Kind: kRef, Name: ref}, true
				default:
					p.pos++
				}
			}
			if len(fields) > 0 {
				d.Body = typ{Kind: kStruct, Fields: fields}
			} else if d.Body.Kind != kRef {
				d.Body = typ{Kind: kStruct}
			}
			for i, at := range p.lx.attrAt { // attribute lists between DATA_BLOCK and the end of the header
				if at >= start && at <= p.pos && reOptimized.MatchString(p.lx.attrs[i]) {
					d.Optimized = true
				}
			}
			for p.peek() != nil && !p.is("END_DATA_BLOCK") {
				p.pos++
			}
			p.pos++
			blocks = append(blocks, d)
		default:
			p.pos++
		}
	}
	if len(blocks) == 0 {
		if len(udts) > 0 {
			return errors.New("the file declares data types only; export the data block that uses them into the same file")
		}
		return errors.New("no DATA_BLOCK found in the source")
	}
	for _, d := range blocks {
		b.layoutBlock(d, udts)
	}
	return nil
}

// decls reads declarations up to the closing keyword.
func (p *parser) decls(depth int, end string) ([]decl, error) {
	if depth > maxDepth {
		return nil, errors.New("structures are nested too deep")
	}
	var out []decl
	for {
		t := p.peek()
		if t == nil {
			return nil, errors.New(end + " is missing")
		}
		if p.is(end) {
			p.pos++
			if n := p.peek(); n != nil && n.s == ";" {
				p.pos++
			}
			return out, nil
		}
		if t.s == ";" {
			p.pos++
			continue
		}
		if t.num || (!t.quoted && len(t.s) == 1 && !isIdentByte(t.s[0])) {
			return nil, errors.New("line " + strconv.Itoa(t.line) + ": a variable name is expected, found " + strconv.Quote(t.s))
		}
		d := decl{Name: t.s}
		line := t.line
		p.pos++
		if p.is("AT") { // an overlay shares the memory of another variable: no space of its own
			for n := p.peek(); n != nil && n.s != ";"; n = p.peek() {
				p.pos++
			}
			continue
		}
		if n := p.peek(); n == nil || n.s != ":" {
			return nil, errors.New("line " + strconv.Itoa(line) + ": \":\" is expected after " + d.Name)
		}
		p.pos++
		ty, err := p.typ(depth)
		if err != nil {
			return nil, errors.New("line " + strconv.Itoa(line) + " (" + d.Name + "): " + err.Error())
		}
		d.Type = ty
		last := line
		afterStruct := p.pos > 0 && strings.EqualFold(p.lx.toks[p.pos-1].s, "END_STRUCT") && !p.lx.toks[p.pos-1].quoted
		for n := p.peek(); n != nil && n.s != ";" && !afterStruct; n = p.peek() { // start value
			if p.is("END_STRUCT", "END_VAR", "BEGIN", "END_DATA_BLOCK", "END_TYPE") {
				break
			}
			last = n.line
			p.pos++
		}
		if n := p.peek(); n != nil && n.s == ";" {
			last = n.line
			p.pos++
		}
		if c, ok := p.lx.comments[last]; ok {
			d.Comment = c
		} else if c, ok := p.lx.comments[line]; ok && ty.Kind != kStruct {
			d.Comment = c
		}
		out = append(out, d)
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80
}

func (p *parser) number() (int, bool) {
	neg := false
	if t := p.peek(); t != nil && t.s == "-" {
		neg = true
		p.pos++
	}
	t := p.peek()
	if t == nil || !t.num {
		return 0, false
	}
	p.pos++
	v, err := strconv.Atoi(t.s)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// typ reads a data type.
func (p *parser) typ(depth int) (typ, error) {
	t := p.peek()
	if t == nil {
		return typ{}, errors.New("a data type is expected")
	}
	if t.quoted {
		p.pos++
		return typ{Kind: kRef, Name: t.s}, nil
	}
	up := strings.ToUpper(t.s)
	switch up {
	case "STRUCT":
		p.pos++
		f, err := p.decls(depth+1, "END_STRUCT")
		if err != nil {
			return typ{}, err
		}
		p.pos-- // decls consumed the ";" that ends the member; give it back
		if tk := p.lx.toks[p.pos]; tk.s != ";" {
			p.pos++
		}
		return typ{Kind: kStruct, Fields: f}, nil
	case "ARRAY":
		p.pos++
		if n := p.peek(); n == nil || n.s != "[" {
			return typ{}, errors.New("\"[\" is expected after ARRAY")
		}
		p.pos++
		var dims [][2]int
		for {
			lo, ok1 := p.number()
			if n := p.peek(); !ok1 || n == nil || n.s != ".." {
				return typ{}, errors.New("array bounds must be numbers (lo..hi); bounds given by constants cannot be read")
			}
			p.pos++
			hi, ok2 := p.number()
			if !ok2 || hi < lo {
				return typ{}, errors.New("array bounds must be numbers (lo..hi); bounds given by constants cannot be read")
			}
			dims = append(dims, [2]int{lo, hi})
			n := p.peek()
			if n != nil && n.s == "," {
				p.pos++
				continue
			}
			if n == nil || n.s != "]" {
				return typ{}, errors.New("\"]\" is expected")
			}
			p.pos++
			break
		}
		if !p.is("OF") {
			return typ{}, errors.New("OF is expected after the array bounds")
		}
		p.pos++
		of, err := p.typ(depth + 1)
		if err != nil {
			return typ{}, err
		}
		return typ{Kind: kArray, Dims: dims, Of: &of}, nil
	case "STRING", "WSTRING":
		p.pos++
		n := 254
		if b := p.peek(); b != nil && b.s == "[" {
			p.pos++
			v, ok := p.number()
			if c := p.peek(); !ok || c == nil || c.s != "]" || v < 1 || v > 16382 {
				return typ{}, errors.New("the length of a string must be a number")
			}
			p.pos++
			n = v
		}
		if up == "WSTRING" {
			return typ{Kind: kWString, Len: n}, nil
		}
		if n > 254 {
			return typ{}, errors.New("a STRING holds at most 254 characters")
		}
		return typ{Kind: kString, Len: n}, nil
	case "UDT", "FB", "SFB":
		name, _ := p.blockName()
		return typ{Kind: kRef, Name: name}, nil
	}
	p.pos++
	if _, ok := elemSize[strings.ToLower(t.s)]; ok {
		return typ{Kind: kElem, Name: t.s}, nil
	}
	if t.num || !isIdentByte(t.s[0]) {
		return typ{}, errors.New("a data type is expected, found " + strconv.Quote(t.s))
	}
	return typ{Kind: kRef, Name: t.s}, nil
}

// parseTypeString reads a data type written as text ("Array[0..3] of Int", "String[20]", "\"udtMotor\"").
func parseTypeString(s string) (typ, error) {
	p := &parser{lx: lex(s)}
	return p.typ(0)
}
