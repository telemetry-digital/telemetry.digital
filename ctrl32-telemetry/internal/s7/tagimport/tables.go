package tagimport

import (
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

// ---- tables: TIA Portal Excel export, STEP 7 symbol tables, generic CSV --------------------------------------

// header finds the columns of a header row; ok when it names a name and an address column.
func header(row []string) (name, addr, typ, comment int, ok bool) {
	name, addr, typ, comment = -1, -1, -1, -1
	for i, c := range row {
		switch h := strings.ToLower(strings.TrimSpace(c)); {
		case name < 0 && (h == "name" || h == "symbol" || h == "tag" || h == "názov" || h == "nazov"):
			name = i
		case addr < 0 && (h == "logical address" || h == "address" || h == "adresse" || h == "adresa" || h == "operand"):
			addr = i
		case typ < 0 && (h == "data type" || h == "datatype" || h == "type" || h == "datentyp" || h == "typ" || h == "dátový typ"):
			typ = i
		case comment < 0 && (h == "comment" || h == "kommentar" || h == "komentár" || h == "komentar" || h == "description"):
			comment = i
		}
	}
	return name, addr, typ, comment, name >= 0 && addr >= 0
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func parseXLSX(data []byte, b *builder) error {
	sheets, err := xlsx.Read(data)
	if err != nil {
		return errors.New("the Excel file cannot be read: " + err.Error())
	}
	found := false
	for _, sh := range sheets {
		for r := 0; r < len(sh.Rows) && r < 10; r++ {
			n, a, t, c, ok := header(sh.Rows[r])
			if !ok {
				continue
			}
			found = true
			for _, row := range sh.Rows[r+1:] {
				b.addRow(cell(row, n), cell(row, a), cell(row, t), cell(row, c))
			}
			break
		}
	}
	if !found {
		return errors.New("no sheet with the columns Name and Logical Address found: export the PLC tags from TIA Portal (PLC tags → Export)")
	}
	return nil
}

// parseASC reads the fixed-width symbol table of STEP 7: "126," then symbol (24), address (12), data type (10),
// comment (80).
func parseASC(text string, b *builder) error {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "126,") {
			continue
		}
		r := []rune(line[4:])
		part := func(from, to int) string {
			if from >= len(r) {
				return ""
			}
			if to > len(r) {
				to = len(r)
			}
			return string(r[from:to])
		}
		b.addRow(part(0, 24), part(24, 36), part(36, 46), part(46, 126))
	}
	return nil
}

// parseTable reads a delimited table: the STEP 7 symbol table as .sdf (quoted, comma) or .seq (tab), or any CSV
// with or without a header row.
func parseTable(text string, b *builder) error {
	first := text
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		first = text[:i]
	}
	delim := ','
	switch {
	case strings.Count(first, "\t") >= 1:
		delim = '\t'
	case strings.Count(first, ";") > strings.Count(first, ","):
		delim = ';'
	}
	rd := csv.NewReader(strings.NewReader(text))
	rd.Comma, rd.FieldsPerRecord, rd.LazyQuotes, rd.TrimLeadingSpace = delim, -1, true, true
	n, a, t, c := 0, 1, 2, 3
	for i := 0; ; i++ {
		row, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("the table cannot be read: " + err.Error())
		}
		if i == 0 {
			if hn, ha, ht, hc, ok := header(row); ok {
				n, a, t, c = hn, ha, ht, hc
				continue
			}
			if len(row) < 2 {
				return errors.New("the file is not a table with name and address columns")
			}
		}
		b.addRow(cell(row, n), cell(row, a), cell(row, t), cell(row, c))
	}
	return nil
}

// ---- XML exports of TIA Portal Openness --------------------------------------------------------------------

type node struct {
	Name  string
	Attr  map[string]string
	Text  string
	Kids  []*node
	depth int
}

func (n *node) child(name string) *node {
	for _, k := range n.Kids {
		if k.Name == name {
			return k
		}
	}
	return nil
}

func (n *node) walk(f func(*node) bool) {
	if !f(n) {
		return
	}
	for _, k := range n.Kids {
		k.walk(f)
	}
}

func parseTree(text string) (*node, error) {
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = false
	root := &node{Name: "#root"}
	stack := []*node{root}
	count := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("the XML file cannot be read: " + err.Error())
		}
		switch t := tok.(type) {
		case xml.StartElement:
			count++
			if count > 2_000_000 || len(stack) > 200 {
				return nil, errors.New("the XML file is too large or nested too deep")
			}
			n := &node{Name: t.Name.Local, Attr: map[string]string{}}
			for _, a := range t.Attr {
				n.Attr[a.Name.Local] = a.Value
			}
			top := stack[len(stack)-1]
			top.Kids = append(top.Kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				stack[len(stack)-1].Text += s
			}
		}
	}
	return root, nil
}

// commentOf returns the first non-empty text of the comment of an Openness object.
func commentOf(n *node) string {
	out := ""
	for _, k := range n.Kids {
		if k.Name != "ObjectList" && k.Name != "Comment" {
			continue
		}
		k.walk(func(x *node) bool {
			if out == "" && (x.Name == "Text" || x.Name == "MultiLanguageText") && x.Text != "" {
				out = x.Text
			}
			return out == ""
		})
	}
	return out
}

func parseXML(text string, b *builder) error {
	root, err := parseTree(text)
	if err != nil {
		return err
	}
	tags, blocks := 0, 0
	udts := map[string]typ{}
	var defs []blockDef
	root.walk(func(n *node) bool {
		switch n.Name {
		case "SW.Tags.PlcTag":
			tags++
			al := n.child("AttributeList")
			if al == nil {
				return false
			}
			get := func(k string) string {
				if c := al.child(k); c != nil {
					return c.Text
				}
				return ""
			}
			b.addRow(get("Name"), get("LogicalAddress"), get("DataTypeName"), commentOf(n))
			return false
		case "SW.Types.PlcStruct", "SW.Blocks.GlobalDB", "SW.Blocks.InstanceDB", "SW.Blocks.ArrayDB":
			al := n.child("AttributeList")
			if al == nil {
				return false
			}
			name, number, optimized := "", 0, false
			var members []decl
			for _, k := range al.Kids {
				switch k.Name {
				case "Name":
					name = k.Text
				case "Number":
					number, _ = strconv.Atoi(k.Text)
				case "MemoryLayout":
					optimized = strings.EqualFold(k.Text, "Optimized")
				case "Interface":
					k.walk(func(x *node) bool {
						if x.Name != "Section" {
							return true
						}
						if sn := x.Attr["Name"]; sn == "Static" || sn == "None" || sn == "" {
							members = append(members, xmlMembers(x, 0)...)
						}
						return false
					})
				}
			}
			if n.Name == "SW.Types.PlcStruct" {
				udts[udtKey(name)] = typ{Kind: kStruct, Fields: members}
				return false
			}
			blocks++
			d := blockDef{Name: name, Number: number, Optimized: optimized, Body: typ{Kind: kStruct, Fields: members}}
			if n.Name != "SW.Blocks.GlobalDB" {
				d.Unknown = "only global data blocks are imported (this is an instance or array data block)"
			}
			defs = append(defs, d)
			return false
		}
		return true
	})
	if tags == 0 && blocks == 0 {
		return errors.New("the XML file holds neither a PLC tag table (SW.Tags.PlcTag) nor a global data block (SW.Blocks.GlobalDB)")
	}
	b.res.Format = "TIA Portal PLC tag table (XML)"
	if blocks > 0 {
		b.res.Format = "TIA Portal data block (XML)"
	}
	for _, d := range defs {
		b.layoutBlock(d, udts)
	}
	return nil
}

// xmlMembers reads the Member elements of a section or of a structured member.
func xmlMembers(n *node, depth int) []decl {
	var out []decl
	if depth > maxDepth {
		return out
	}
	for _, m := range n.Kids {
		if m.Name != "Member" {
			continue
		}
		d := decl{Name: m.Attr["Name"]}
		if c := m.child("Comment"); c != nil {
			c.walk(func(x *node) bool {
				if d.Comment == "" && x.Text != "" {
					d.Comment = x.Text
				}
				return d.Comment == ""
			})
		}
		dt := strings.TrimSpace(m.Attr["Datatype"])
		t, err := parseTypeString(reOfStruct.ReplaceAllString(dt, "of anonymousStruct"))
		if err != nil {
			t = typ{Kind: kRef, Name: dt}
		}
		// A structure carries its members as children; a user data type carries them in Sections (V14 and newer),
		// so the type does not have to be exported separately.
		var inner []decl
		if strings.EqualFold(dt, "Struct") {
			inner = xmlMembers(m, depth+1)
			t = typ{Kind: kStruct, Fields: inner}
		} else if sec := m.child("Sections"); sec != nil {
			for _, s := range sec.Kids {
				if s.Name == "Section" {
					inner = append(inner, xmlMembers(s, depth+1)...)
				}
			}
			st := typ{Kind: kStruct, Fields: inner}
			switch {
			case t.Kind == kRef:
				t = st
			case t.Kind == kArray && t.Of != nil && t.Of.Kind == kRef:
				t.Of = &st
			}
		} else if t.Kind == kArray && t.Of != nil && (t.Of.Kind == kRef && t.Of.Name == "anonymousStruct") {
			st := typ{Kind: kStruct, Fields: xmlMembers(m, depth+1)}
			t.Of = &st
		}
		out = append(out, d2(d, t))
	}
	return out
}

func d2(d decl, t typ) decl { d.Type = t; return d }

var reOfStruct = regexp.MustCompile(`(?i)of\s+struct\s*$`)
