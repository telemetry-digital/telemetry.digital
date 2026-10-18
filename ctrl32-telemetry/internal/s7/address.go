package s7

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Addresses are accepted in Siemens (STEP 7 / TIA Portal) syntax, in nodes7 syntax and as LOGO! V memory, with
// English or German mnemonics:
//
//	Siemens   DB1.DBX0.0  DB1.DBB2  DB1.DBW4  DB1.DBD6  DB1.DBS10.20 (S7 string, max 20 characters)
//	          I0.0 IB0 IW64 ID0 (German E…), Q0.0 QB0 QW0 QD0 (German A…), M10.0 MB10 MW10 MD10 (MR10 = REAL)
//	nodes7    DB1,X0.0 DB1,B2 DB1,C3 DB1,INT4 DB1,WORD4 DB1,DINT6 DB1,DWORD6 DB1,REAL6 DB1,LREAL10 DB1,S10.20
//	          (also SINT USINT UINT UDINT, short forms BYTE CHAR I W DI DW R LR STRING)
//	LOGO!     V10.3 VB100 VW100 VD100 (V memory is DB1)
//
// The size in the address implies the default data type: X/bit → bool, B → byte, W → int, D → real,
// S → string; nodes7 type tokens set the type directly. An explicit type may override the default when it has the
// same size (B: byte char sint usint; W: int uint word; D: dint udint dword real); anything else is rejected.
// LREAL (8 bytes) has no Siemens mnemonic and is addressed as DB1,LREAL10. Values are big-endian.

// Area is the S7 memory area code of an ANY pointer.
type Area byte

const (
	AreaI  Area = 0x81 // process inputs (German E)
	AreaQ  Area = 0x82 // process outputs (German A)
	AreaM  Area = 0x83 // bit memory (German M)
	AreaDB Area = 0x84 // data blocks
)

// Address is one parsed S7 variable.
type Address struct {
	Area  Area
	DB    int    // data block number (AreaDB only)
	Start int    // byte offset
	Bit   int    // bit 0..7 (bool only)
	Type  string // bool byte char sint usint int uint word dint udint dword real lreal string
	Len   int    // maximum length of a string; 0 for other types
}

const (
	maxDB    = 65535
	maxStart = 1<<21 - 1 // the ANY pointer carries a 24-bit bit address
	maxStrLn = 254
)

// typeSize is the size in bytes of each data type (string: 2 + max length).
var typeSize = map[string]int{
	"bool": 1, "byte": 1, "char": 1, "sint": 1, "usint": 1,
	"int": 2, "uint": 2, "word": 2,
	"dint": 4, "udint": 4, "dword": 4, "real": 4,
	"lreal": 8, "string": 0,
}

// Types lists the supported data types.
var Types = []string{"bool", "byte", "char", "sint", "usint", "int", "uint", "word", "dint", "udint", "dword", "real", "lreal", "string"}

// sizeTypes is the class of types an address size letter allows; the first entry is the default.
var sizeTypes = map[string][]string{
	"X": {"bool"},
	"B": {"byte", "char", "sint", "usint"},
	"W": {"int", "uint", "word"},
	"D": {"real", "dint", "udint", "dword"},
	"L": {"lreal"},
	"S": {"string"},
}

// nodes7 type tokens → (type, size letter).
var nodes7Types = map[string]string{
	"X": "bool", "B": "byte", "BYTE": "byte", "C": "char", "CHAR": "char", "SINT": "sint", "USINT": "usint",
	"I": "int", "INT": "int", "W": "word", "WORD": "word", "UINT": "uint",
	"DI": "dint", "DINT": "dint", "DW": "dword", "DWORD": "dword", "UDINT": "udint", "R": "real", "REAL": "real",
	"LR": "lreal", "LREAL": "lreal", "S": "string", "STRING": "string",
}

var (
	reDBSiemens = regexp.MustCompile(`^DB(\d+)\.DB([XBWDS])(\d+)(?:\.(\d+))?$`)
	reDBNodes7  = regexp.MustCompile(`^DB(\d+),([A-Z]+)(\d+)(?:\.(\d+))?$`)
	reArea      = regexp.MustCompile(`^([IEQAM])([XBWDR]?)(\d+)(?:\.(\d+))?$`)
	reV         = regexp.MustCompile(`^V([XBWD]?)(\d+)(?:\.(\d+))?$`)
)

// sizeLetter returns the address size letter of a type.
func sizeLetter(typ string) string {
	for l, ts := range sizeTypes {
		for _, t := range ts {
			if t == typ {
				return l
			}
		}
	}
	return ""
}

// ParseAddress parses an S7 address; typ is an optional data type overriding the default implied by the address.
func ParseAddress(addr, typ string) (Address, error) {
	s := strings.ToUpper(strings.Join(strings.Fields(addr), ""))
	typ = strings.ToLower(strings.TrimSpace(typ))
	if s == "" {
		return Address{}, errors.New("address required")
	}
	if typ != "" {
		if _, ok := typeSize[typ]; !ok {
			return Address{}, fmt.Errorf("unknown data type %q (supported: %s)", typ, strings.Join(Types, ", "))
		}
	}
	var a Address
	var size, num, sub string
	switch {
	case reDBSiemens.MatchString(s):
		m := reDBSiemens.FindStringSubmatch(s)
		a.Area = AreaDB
		a.DB = atoi(m[1])
		size, num, sub = m[2], m[3], m[4]
	case reDBNodes7.MatchString(s):
		m := reDBNodes7.FindStringSubmatch(s)
		t, ok := nodes7Types[m[2]]
		if !ok {
			return Address{}, fmt.Errorf("%s: unknown type %q", addr, m[2])
		}
		a.Area = AreaDB
		a.DB = atoi(m[1])
		size, num, sub = sizeLetter(t), m[3], m[4]
		if typ == "" {
			typ = t
		} else if sizeLetter(typ) != size {
			return Address{}, fmt.Errorf("%s: type %s does not match %s", addr, typ, t)
		}
		if sub != "" && size != "X" && size != "S" {
			return Address{}, fmt.Errorf("%s: arrays are not supported", addr)
		}
	case reV.MatchString(s):
		m := reV.FindStringSubmatch(s)
		a.Area, a.DB = AreaDB, 1
		size, num, sub = m[1], m[2], m[3]
		if size == "" {
			size = "X"
		}
	case reArea.MatchString(s):
		m := reArea.FindStringSubmatch(s)
		switch m[1] {
		case "I", "E":
			a.Area = AreaI
		case "Q", "A":
			a.Area = AreaQ
		default:
			a.Area = AreaM
		}
		size, num, sub = m[2], m[3], m[4]
		if size == "" {
			size = "X"
		}
		if size == "R" {
			size = "D"
			if typ == "" {
				typ = "real"
			}
		}
	default:
		return Address{}, fmt.Errorf("invalid S7 address %q (examples: DB1.DBW4, DB1,REAL6, MW10, I0.0, VW100)", addr)
	}
	if a.Area == AreaDB && (a.DB < 1 || a.DB > maxDB) {
		return Address{}, fmt.Errorf("%s: data block number must be 1..%d", addr, maxDB)
	}
	a.Start = atoi(num)
	if a.Start < 0 || a.Start > maxStart {
		return Address{}, fmt.Errorf("%s: byte offset out of range", addr)
	}
	switch size {
	case "X":
		if sub == "" {
			return Address{}, fmt.Errorf("%s: bit address needs a bit number (e.g. %s.0)", addr, s)
		}
		a.Bit = atoi(sub)
		if a.Bit < 0 || a.Bit > 7 {
			return Address{}, fmt.Errorf("%s: bit number must be 0..7", addr)
		}
	case "S":
		if sub == "" {
			return Address{}, fmt.Errorf("%s: string address needs a maximum length (e.g. DB1.DBS10.20)", addr)
		}
		a.Len = atoi(sub)
		if a.Len < 1 || a.Len > maxStrLn {
			return Address{}, fmt.Errorf("%s: string length must be 1..%d", addr, maxStrLn)
		}
	default:
		if sub != "" {
			return Address{}, fmt.Errorf("%s: only bit and string addresses have a .n suffix", addr)
		}
	}
	allowed := sizeTypes[size]
	if typ == "" {
		typ = allowed[0]
	}
	ok := false
	for _, t := range allowed {
		ok = ok || t == typ
	}
	if !ok {
		return Address{}, fmt.Errorf("%s: type %s needs %s, the address allows %s", addr, typ, sizeHint(typ), strings.Join(allowed, ", "))
	}
	a.Type = typ
	if a.Start+a.Size() > maxStart+1 {
		return Address{}, fmt.Errorf("%s: byte offset out of range", addr)
	}
	return a, nil
}

func sizeHint(typ string) string {
	switch sizeLetter(typ) {
	case "X":
		return "a bit address (X)"
	case "B":
		return "a byte address (B)"
	case "W":
		return "a word address (W)"
	case "D":
		return "a double word address (D)"
	case "L":
		return "an LREAL address (DBn,LREALm)"
	}
	return "a string address (S)"
}

func atoi(s string) int {
	if len(s) > 9 {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// Size is the number of bytes the variable occupies (1 for a bool).
func (a Address) Size() int {
	if a.Type == "string" {
		return 2 + a.Len
	}
	return typeSize[a.Type]
}

// String returns the canonical address in Siemens syntax (the data type is not part of it, except LREAL, which
// is written in nodes7 syntax).
func (a Address) String() string {
	l := sizeLetter(a.Type)
	if a.Area == AreaDB {
		switch l {
		case "X":
			return fmt.Sprintf("DB%d.DBX%d.%d", a.DB, a.Start, a.Bit)
		case "S":
			return fmt.Sprintf("DB%d.DBS%d.%d", a.DB, a.Start, a.Len)
		case "L":
			return fmt.Sprintf("DB%d,LREAL%d", a.DB, a.Start)
		}
		return fmt.Sprintf("DB%d.DB%s%d", a.DB, l, a.Start)
	}
	p := map[Area]string{AreaI: "I", AreaQ: "Q", AreaM: "M"}[a.Area]
	if l == "X" {
		return fmt.Sprintf("%s%d.%d", p, a.Start, a.Bit)
	}
	return fmt.Sprintf("%s%s%d", p, l, a.Start)
}

// Decode converts the raw bytes of the variable (Size bytes; for a bool the byte holding the bit in bit 0) to a Go
// value: bool, float64 for numbers, string for char and string.
func (a Address) Decode(b []byte) (any, error) {
	if len(b) < a.Size() {
		return nil, errors.New("short response")
	}
	switch a.Type {
	case "bool":
		return b[0]&1 == 1, nil
	case "byte", "usint":
		return float64(b[0]), nil
	case "sint":
		return float64(int8(b[0])), nil
	case "char":
		return latin1(b[:1]), nil
	case "int":
		return float64(int16(binary.BigEndian.Uint16(b))), nil
	case "uint", "word":
		return float64(binary.BigEndian.Uint16(b)), nil
	case "dint":
		return float64(int32(binary.BigEndian.Uint32(b))), nil
	case "udint", "dword":
		return float64(binary.BigEndian.Uint32(b)), nil
	case "real":
		f := math.Float32frombits(binary.BigEndian.Uint32(b))
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return nil, errors.New("not a number")
		}
		// The shortest decimal that is the same REAL (21.5, not 21.500000953674316).
		v, _ := strconv.ParseFloat(strconv.FormatFloat(float64(f), 'g', -1, 32), 64)
		return v, nil
	case "lreal":
		v := math.Float64frombits(binary.BigEndian.Uint64(b))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("not a number")
		}
		return v, nil
	case "string":
		n := int(b[1])
		if n > a.Len {
			n = a.Len
		}
		return latin1(b[2 : 2+n]), nil
	}
	return nil, fmt.Errorf("unsupported type %s", a.Type)
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// toLatin1 encodes a string as ISO 8859-1 (the S7 STRING character set).
func toLatin1(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF || r == utf8.RuneError {
			return nil, fmt.Errorf("character %q cannot be written to an S7 string", r)
		}
		out = append(out, byte(r))
	}
	return out, nil
}

// integer ranges per type
var intRange = map[string][2]float64{
	"byte": {0, 255}, "usint": {0, 255}, "sint": {-128, 127},
	"int": {-32768, 32767}, "uint": {0, 65535}, "word": {0, 65535},
	"dint": {math.MinInt32, math.MaxInt32}, "udint": {0, math.MaxUint32}, "dword": {0, math.MaxUint32},
}

// Encode converts a value (bool, number, numeric string or string) to the bytes written to the PLC. A bool yields
// one byte (0 or 1); a string yields the actual-length byte followed by the characters (the maximum-length byte at
// Start is left untouched, so the data starts at Start+1).
func (a Address) Encode(v any) ([]byte, error) {
	switch a.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: a string value is required", a)
		}
		b, err := toLatin1(s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a, err)
		}
		if len(b) > a.Len {
			return nil, fmt.Errorf("%s: string of %d characters exceeds the maximum length %d", a, len(b), a.Len)
		}
		return append([]byte{byte(len(b))}, b...), nil
	case "char":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: a one-character string is required", a)
		}
		b, err := toLatin1(s)
		if err != nil || len(b) != 1 {
			return nil, fmt.Errorf("%s: a single Latin-1 character is required", a)
		}
		return b, nil
	case "bool":
		switch x := v.(type) {
		case bool:
			if x {
				return []byte{1}, nil
			}
			return []byte{0}, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "true", "1", "on":
				return []byte{1}, nil
			case "false", "0", "off":
				return []byte{0}, nil
			}
			return nil, fmt.Errorf("%s: %q is not a bool", a, x)
		}
		f, err := toFloat(v)
		if err != nil || (f != 0 && f != 1) {
			return nil, fmt.Errorf("%s: a bool (or 0/1) is required", a)
		}
		return []byte{byte(f)}, nil
	}
	f, err := toFloat(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a, err)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%s: value is not a finite number", a)
	}
	switch a.Type {
	case "real":
		if math.Abs(f) > math.MaxFloat32 {
			return nil, fmt.Errorf("%s: value out of REAL range", a)
		}
		return binary.BigEndian.AppendUint32(nil, math.Float32bits(float32(f))), nil
	case "lreal":
		return binary.BigEndian.AppendUint64(nil, math.Float64bits(f)), nil
	}
	rg, ok := intRange[a.Type]
	if !ok {
		return nil, fmt.Errorf("%s: unsupported type %s", a, a.Type)
	}
	i := math.Round(f)
	if i < rg[0] || i > rg[1] {
		return nil, fmt.Errorf("%s: value %v out of %s range %v..%v", a, f, strings.ToUpper(a.Type), rg[0], rg[1])
	}
	n := int64(i)
	switch typeSize[a.Type] {
	case 1:
		return []byte{byte(n)}, nil
	case 2:
		return binary.BigEndian.AppendUint16(nil, uint16(n)), nil
	}
	return binary.BigEndian.AppendUint32(nil, uint32(n)), nil
}

func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int8:
		return float64(x), nil
	case int16:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint:
		return float64(x), nil
	case uint8:
		return float64(x), nil
	case uint16:
		return float64(x), nil
	case uint32:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", x)
		}
		return f, nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("unsupported value type %T", v)
}
