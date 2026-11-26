// Package tagimport reads the tag exports of Siemens engineering tools and turns them into tags of the S7
// connector: a datastream key, an absolute address and a data type per variable. Nothing is stored and no tag is
// made writable — the result is a proposal the user picks from.
//
// Formats:
//
//	TIA Portal   PLC tag table as Excel (.xlsx, "Export tags") and as XML (Openness, SW.Tags.PlcTagTable)
//	TIA Portal   data block as source (.db, "Generate source from blocks") and as XML (Openness, SW.Blocks.GlobalDB)
//	STEP 7 V5    symbol table (.sdf, .asc, .seq) and data block source (.awl)
//	any tool     a table with the columns name, address, type, comment (.csv, .txt; comma, semicolon or tab)
//
// Data blocks carry no addresses in their sources, so the offsets are calculated by the layout rules of blocks
// with standard access (not optimized): see layout.go.
package tagimport

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/unicode/norm"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
)

// Limits of one import.
const (
	MaxFileBytes  = 8 << 20
	MaxTags       = 20000 // tags proposed from one file
	maxArrayItems = 1024  // elements of one array that become tags; the rest is reported as skipped
	maxDepth      = 16    // nesting of structures, arrays and data types
)

// Tag is one proposed tag.
type Tag struct {
	Key     string `json:"key"`               // datastream key, unique within the result and the existing keys
	Name    string `json:"name"`              // the symbol as in the PLC project: Tank_Level, Motor1.Speed, Temps[3]
	Address string `json:"address"`           // DB5.DBD12, MW10, I0.0 …; "DB{db}.DBD12" when the block number is not in the file
	Type    string `json:"type"`              // data type of the S7 connector
	PLCType string `json:"plc_type"`          // data type as declared in the project
	Comment string `json:"comment,omitempty"` // comment from the project
	Area    string `json:"area"`              // i | q | m | db
	Block   string `json:"block,omitempty"`   // data block the tag belongs to
	NeedsDB bool   `json:"needs_db,omitempty"`
}

// Skipped is a variable of the file that did not become a tag, with the reason.
type Skipped struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	PLCType string `json:"plc_type,omitempty"`
	Reason  string `json:"reason"`
}

// Block describes a data block found in the file.
type Block struct {
	Name    string `json:"name"`
	Number  int    `json:"number"` // 0: not in the file (TIA Portal sources); the caller supplies it
	Tags    int    `json:"tags"`
	Skipped int    `json:"skipped"`
	Size    int    `json:"size"` // bytes, as calculated
	Note    string `json:"note,omitempty"`
}

// Result is what one file yields.
type Result struct {
	Format   string    `json:"format"`
	Tags     []Tag     `json:"tags"`
	Skipped  []Skipped `json:"skipped"`
	Blocks   []Block   `json:"blocks"`
	Warnings []string  `json:"warnings"`
}

// Options of an import.
type Options struct {
	Filename string
	Encoding string   // of text files that are not UTF-8: windows-1250 | windows-1251 | windows-1252 (default) | windows-1253 | windows-1257
	DB       int      // number for data blocks whose source does not carry one; 0 leaves "{db}" in the address
	Prefix   string   // put before every key
	Existing []string // keys already used by the connector
}

var prefixRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,15}$`)

// Parse reads one export file.
func Parse(data []byte, opt Options) (*Result, error) {
	if len(data) == 0 {
		return nil, errors.New("the file is empty")
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("the file is larger than %d MB", MaxFileBytes>>20)
	}
	if opt.Prefix != "" && !prefixRe.MatchString(opt.Prefix) {
		return nil, errors.New("key prefix must match ^[a-z][a-z0-9_]{0,15}$")
	}
	if opt.DB < 0 || opt.DB > 65535 {
		return nil, errors.New("data block number must be 1..65535")
	}
	b := newBuilder(opt)
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(opt.Filename, `\`, "/")))
	var err error
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		b.res.Format = "TIA Portal PLC tag table (Excel)"
		err = parseXLSX(data, b)
	default:
		text, derr := decodeText(data, opt.Encoding)
		if derr != nil {
			return nil, derr
		}
		trim := strings.TrimSpace(text)
		switch {
		case strings.HasPrefix(trim, "<"):
			err = parseXML(text, b)
		case reSource.MatchString(text):
			b.res.Format = "data block source"
			err = parseSource(text, b)
		case ext == ".asc" || reASC.MatchString(trim):
			b.res.Format = "STEP 7 symbol table (ASC)"
			err = parseASC(text, b)
		default:
			b.res.Format = "table of symbols"
			if ext == ".sdf" || ext == ".seq" {
				b.res.Format = "STEP 7 symbol table (" + strings.ToUpper(ext[1:]) + ")"
			}
			err = parseTable(text, b)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(b.res.Tags) == 0 && len(b.res.Skipped) == 0 && len(b.res.Blocks) == 0 {
		return nil, errors.New("no variables found: expected a PLC tag table (.xlsx, .xml), a symbol table (.sdf, .asc, .seq), a data block source (.db, .awl) or a table with name, address, type")
	}
	return b.res, nil
}

var (
	reSource = regexp.MustCompile(`(?im)^\s*(DATA_BLOCK|TYPE)\s+("|UDT|DB|[A-Za-z_])`)
	reASC    = regexp.MustCompile(`^126,`)
)

// ---- text encodings ----------------------------------------------------------------------------------------

func decodeText(data []byte, enc string) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		out, err := xunicode.UTF16(xunicode.LittleEndian, xunicode.UseBOM).NewDecoder().Bytes(data)
		if err != nil {
			return "", errors.New("the file is not valid UTF-16")
		}
		return string(out), nil
	}
	if utf8.Valid(data) {
		return string(data), nil
	}
	var cm encoding.Encoding
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "", "windows-1252", "cp1252", "latin1":
		cm = charmap.Windows1252
	case "windows-1250", "cp1250":
		cm = charmap.Windows1250
	case "windows-1251", "cp1251":
		cm = charmap.Windows1251
	case "windows-1253", "cp1253":
		cm = charmap.Windows1253
	case "windows-1257", "cp1257":
		cm = charmap.Windows1257
	default:
		return "", errors.New("encoding must be windows-1250, windows-1251, windows-1252, windows-1253 or windows-1257")
	}
	out, err := cm.NewDecoder().Bytes(data)
	if err != nil {
		return "", errors.New("the file cannot be decoded with the chosen encoding")
	}
	return string(out), nil
}

// ---- builder: keys, tags, skipped --------------------------------------------------------------------------

type builder struct {
	opt  Options
	res  *Result
	keys map[string]bool
	full bool
}

func newBuilder(opt Options) *builder {
	b := &builder{opt: opt, res: &Result{Tags: []Tag{}, Skipped: []Skipped{}, Blocks: []Block{}, Warnings: []string{}}, keys: map[string]bool{}}
	for _, k := range opt.Existing {
		b.keys[strings.TrimSpace(k)] = true
	}
	return b
}

func (b *builder) warn(format string, a ...any) {
	w := fmt.Sprintf(format, a...)
	for _, x := range b.res.Warnings {
		if x == w {
			return
		}
	}
	if len(b.res.Warnings) < 50 {
		b.res.Warnings = append(b.res.Warnings, w)
	}
}

func (b *builder) skip(name, addr, typ, reason string) {
	if len(b.res.Skipped) < MaxTags {
		b.res.Skipped = append(b.res.Skipped, Skipped{Name: name, Address: addr, PLCType: typ, Reason: reason})
	}
}

// add appends a tag; false when the limit is reached.
func (b *builder) add(t Tag) bool {
	if len(b.res.Tags) >= MaxTags {
		if !b.full {
			b.full = true
			b.warn("the file has more than %d variables; the rest is left out", MaxTags)
		}
		return false
	}
	t.Key = b.key(t.Name)
	b.res.Tags = append(b.res.Tags, t)
	return true
}

var translit = strings.NewReplacer("ß", "ss", "ł", "l", "Ł", "l", "đ", "d", "Đ", "d", "ø", "o", "Ø", "o", "æ", "ae", "Æ", "ae", "œ", "oe", "ı", "i")

// MakeKey turns a symbol into a datastream key (^[a-z][a-z0-9_]{0,31}$): diacritics removed, other characters
// become "_". The result is not yet unique.
func MakeKey(prefix, name string) string {
	var sb strings.Builder
	last := byte('_')
	for _, r := range norm.NFD.String(translit.Replace(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			last = byte(unicode.ToLower(r))
			sb.WriteByte(last)
		default:
			if last != '_' {
				last = '_'
				sb.WriteByte('_')
			}
		}
	}
	k := strings.Trim(sb.String(), "_")
	if k == "" {
		k = "tag"
	}
	if prefix == "" && (k[0] < 'a' || k[0] > 'z') {
		k = "t_" + k
	}
	k = prefix + k
	if len(k) > 32 { // keep the beginning (the machine) and the end (the signal)
		k = strings.TrimRight(k[:15], "_") + "_" + strings.TrimLeft(k[len(k)-16:], "_")
	}
	return k
}

func (b *builder) key(name string) string {
	k := MakeKey(b.opt.Prefix, name)
	if !b.keys[k] {
		b.keys[k] = true
		return k
	}
	for n := 2; ; n++ {
		suf := fmt.Sprintf("_%d", n)
		c := k
		if len(c)+len(suf) > 32 {
			c = strings.TrimRight(c[:32-len(suf)], "_")
		}
		c += suf
		if !b.keys[c] {
			b.keys[c] = true
			return c
		}
	}
}

// ---- data types --------------------------------------------------------------------------------------------

// connType maps a PLC data type to the type of the S7 connector; "" when the connector cannot read it.
func connType(plc string) (typ, note string) {
	switch strings.ToLower(strings.TrimSpace(plc)) {
	case "bool":
		return "bool", ""
	case "byte":
		return "byte", ""
	case "char":
		return "char", ""
	case "sint":
		return "sint", ""
	case "usint":
		return "usint", ""
	case "int":
		return "int", ""
	case "uint":
		return "uint", ""
	case "word":
		return "word", ""
	case "dint":
		return "dint", ""
	case "udint":
		return "udint", ""
	case "dword":
		return "dword", ""
	case "real":
		return "real", ""
	case "lreal":
		return "lreal", ""
	case "string":
		return "string", ""
	case "time":
		return "dint", "TIME is read as milliseconds"
	case "time_of_day", "tod":
		return "udint", "TIME_OF_DAY is read as milliseconds since midnight"
	case "date":
		return "uint", "DATE is read as days since 1990-01-01"
	}
	return "", ""
}

var (
	reBlockSym = regexp.MustCompile(`^(DB|FB|FC|OB|SFB|SFC|UDT|VAT|T|Z|C)\d+$`)
	rePeriph   = regexp.MustCompile(`^P[EIAQ]`)
)

// addRow adds a variable that has an absolute address in the file (tag table, symbol table).
func (b *builder) addRow(name, addr, plcType, comment string) {
	name, comment = strings.TrimSpace(name), strings.TrimSpace(comment)
	raw := strings.TrimSpace(addr)
	a := strings.ToUpper(strings.Join(strings.Fields(strings.TrimPrefix(raw, "%")), ""))
	plcType = strings.TrimSpace(plcType)
	if name == "" && a == "" {
		return
	}
	if name == "" {
		name = a
	}
	switch {
	case a == "":
		b.skip(name, raw, plcType, "no address")
		return
	case strings.HasSuffix(a, ":P") || rePeriph.MatchString(a):
		b.skip(name, raw, plcType, "peripheral addresses are not read; use the process image address (I, Q)")
		return
	case reBlockSym.MatchString(a):
		b.skip(name, raw, plcType, "a block, timer or counter, not a variable")
		return
	}
	typ, note := connType(plcType)
	if plcType != "" && typ == "" {
		b.skip(name, raw, plcType, "data type "+plcType+" is not supported by the S7 connector")
		return
	}
	if typ == "string" { // a string needs its length, which symbol tables do not carry
		b.skip(name, raw, plcType, "strings outside data blocks are not imported")
		return
	}
	pa, err := s7.ParseAddress(a, typ)
	if err != nil {
		b.skip(name, raw, plcType, err.Error())
		return
	}
	if note != "" {
		b.warn("%s", note)
	}
	area := map[s7.Area]string{s7.AreaI: "i", s7.AreaQ: "q", s7.AreaM: "m", s7.AreaDB: "db"}[pa.Area]
	b.add(Tag{Name: name, Address: pa.String(), Type: pa.Type, PLCType: plcType, Comment: comment, Area: area})
}
