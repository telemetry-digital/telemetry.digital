package tagimport

import (
	"fmt"
	"strconv"
	"strings"
)

// Layout of a data block with standard access (STEP 7 V5; TIA Portal with "Optimized block access" off):
//
//   - BOOL takes one bit; consecutive BOOLs fill a byte from bit 0.
//   - BYTE, CHAR, SINT, USINT take one byte and start at the next free byte.
//   - Every type of two bytes or more (WORD, INT, DWORD, DINT, REAL, TIME, DATE, LREAL …) starts at an even byte.
//   - STRING[n] takes n + 2 bytes, WSTRING[n] 2n + 4; ARRAY, STRUCT and user data types start at an even byte and
//     the next variable after them starts at an even byte too.
//   - Elements of an array follow the same rules: BOOL elements are packed into bits, byte elements into bytes;
//     every row of a multi-dimensional array starts at an even byte.
//
// Blocks with optimized access have no fixed offsets and cannot be read by absolute address at all.

type kind int

const (
	kElem kind = iota
	kString
	kWString
	kArray
	kStruct
	kRef
)

// typ is a declared data type.
type typ struct {
	Kind   kind
	Name   string   // elementary type or the referenced data type, as written
	Len    int      // STRING / WSTRING maximum length
	Dims   [][2]int // array bounds
	Of     *typ     // array element
	Fields []decl   // structure members
}

// decl is one declared variable.
type decl struct {
	Name    string
	Comment string
	Type    typ
}

// elemSize is the size in bytes of the elementary types (bool: 0 = one bit).
var elemSize = map[string]int{
	"bool": 0,
	"byte": 1, "char": 1, "sint": 1, "usint": 1,
	"word": 2, "int": 2, "uint": 2, "date": 2, "s5time": 2, "wchar": 2,
	"dword": 4, "dint": 4, "udint": 4, "real": 4, "time": 4, "time_of_day": 4, "tod": 4,
	"lreal": 8, "lint": 8, "ulint": 8, "lword": 8, "ltime": 8, "ltime_of_day": 8, "ltod": 8, "ldt": 8,
	"date_and_time": 8, "dt": 8, "dtl": 12,
}

type blockDef struct {
	Name      string
	Number    int
	Optimized bool
	Body      typ // a structure, or a reference to a data type
	Unknown   string
}

type layouter struct {
	b       *builder
	udts    map[string]typ
	blk     *Block
	addrDB  string // "DB5" or "DB{db}"
	needsDB bool
	byteOff int
	bit     int
	stopped string // why the layout cannot go on
}

func (l *layouter) alignByte() {
	if l.bit > 0 {
		l.byteOff++
		l.bit = 0
	}
}

func (l *layouter) alignWord() {
	l.alignByte()
	if l.byteOff%2 == 1 {
		l.byteOff++
	}
}

func udtKey(name string) string { return strings.ToLower(strings.Join(strings.Fields(name), "")) }

// layoutBlock calculates the addresses of a data block and adds its tags.
func (b *builder) layoutBlock(d blockDef, udts map[string]typ) {
	blk := Block{Name: d.Name, Number: d.Number}
	if blk.Number == 0 {
		blk.Number = b.opt.DB
	}
	switch {
	case d.Optimized:
		blk.Note = "optimized block access: the block has no absolute addresses. Switch \"Optimized block access\" off in the block properties, compile, download and export again."
	case d.Unknown != "":
		blk.Note = d.Unknown
	}
	if blk.Note != "" {
		b.skip(d.Name, "", "DB", blk.Note)
		blk.Skipped = 1
		b.res.Blocks = append(b.res.Blocks, blk)
		return
	}
	l := &layouter{b: b, udts: udts, blk: &blk, addrDB: "DB{db}", needsDB: true}
	if blk.Number > 0 {
		l.addrDB, l.needsDB = "DB"+strconv.Itoa(blk.Number), false
	}
	before, skippedBefore := len(b.res.Tags), len(b.res.Skipped)
	l.place(d.Body, "", "", 0, true)
	l.alignWord()
	blk.Size = l.byteOff
	blk.Tags, blk.Skipped = len(b.res.Tags)-before, len(b.res.Skipped)-skippedBefore
	if l.stopped != "" {
		blk.Note = l.stopped
	}
	if l.needsDB && blk.Tags > 0 {
		b.warn("the source of block %s does not carry its number: enter the data block number", d.Name)
	}
	b.res.Blocks = append(b.res.Blocks, blk)
	if blk.Tags > 0 {
		b.warn("data block addresses are calculated from the declaration: compare a few of them with the Offset column of the block before use")
	}
}

func (l *layouter) emit(path, comment, plcType, connT string, size int, strLen int) {
	if l.stopped != "" {
		return
	}
	var addr string
	switch {
	case connT == "bool":
		addr = fmt.Sprintf("%s.DBX%d.%d", l.addrDB, l.byteOff, l.bit)
	case connT == "string":
		addr = fmt.Sprintf("%s.DBS%d.%d", l.addrDB, l.byteOff, strLen)
	case connT == "lreal":
		addr = fmt.Sprintf("%s,LREAL%d", l.addrDB, l.byteOff)
	case size == 1:
		addr = fmt.Sprintf("%s.DBB%d", l.addrDB, l.byteOff)
	case size == 2:
		addr = fmt.Sprintf("%s.DBW%d", l.addrDB, l.byteOff)
	default:
		addr = fmt.Sprintf("%s.DBD%d", l.addrDB, l.byteOff)
	}
	if connT == "" {
		l.b.skip(path, addr, plcType, "data type "+plcType+" is not supported by the S7 connector")
		return
	}
	l.b.add(Tag{Name: path, Address: addr, Type: connT, PLCType: plcType, Comment: comment, Area: "db", Block: l.blk.Name, NeedsDB: l.needsDB})
}

// place lays out one variable of type t at the current position. top marks the body of the block itself.
func (l *layouter) place(t typ, path, comment string, depth int, top bool) {
	if l.stopped != "" {
		return
	}
	if depth > maxDepth {
		l.stop(path, "nested too deep")
		return
	}
	switch t.Kind {
	case kElem:
		name := strings.ToLower(t.Name)
		size := elemSize[name]
		ct, note := connType(name)
		if note != "" {
			l.b.warn("%s", note)
		}
		switch {
		case name == "bool":
			l.emit(path, comment, t.Name, "bool", 0, 0)
			l.bit++
			if l.bit == 8 {
				l.byteOff++
				l.bit = 0
			}
		case size == 1:
			l.alignByte()
			l.emit(path, comment, t.Name, ct, 1, 0)
			l.byteOff++
		default:
			l.alignWord()
			l.emit(path, comment, t.Name, ct, size, 0)
			l.byteOff += size
		}
	case kString:
		l.alignWord()
		l.emit(path, comment, fmt.Sprintf("String[%d]", t.Len), "string", t.Len+2, t.Len)
		l.byteOff += t.Len + 2
		if t.Len%2 == 1 {
			l.b.warn("%s is a string of odd length: check the address of the variable that follows it", path)
		}
		l.alignWord()
	case kWString:
		l.alignWord()
		l.b.skip(path, "", fmt.Sprintf("WString[%d]", t.Len), "data type WString is not supported by the S7 connector")
		l.byteOff += 2*t.Len + 4
	case kStruct:
		l.alignWord()
		for _, f := range t.Fields {
			p := f.Name
			if path != "" {
				p = path + "." + f.Name
			}
			l.place(f.Type, p, f.Comment, depth+1, false)
		}
		l.alignWord()
	case kRef:
		u, ok := l.udts[udtKey(t.Name)]
		if !ok {
			if top {
				l.stop(l.blk.Name, "the block is an instance of "+t.Name+", whose declaration is not in the file")
			} else {
				l.stop(path, "data type "+t.Name+" is not declared in the file: export it together with the block; the variables after "+path+" are left out because their addresses depend on it")
			}
			return
		}
		l.place(u, path, comment, depth+1, top)
	case kArray:
		l.alignWord()
		if t.Of == nil || len(t.Dims) == 0 {
			l.stop(path, "array without bounds")
			return
		}
		small := t.Of.Kind == kElem && elemSize[strings.ToLower(t.Of.Name)] <= 1
		if small && len(t.Dims) > 1 {
			l.b.warn("%s is a multi-dimensional array of bits or bytes: check its addresses against the Offset column", path)
		}
		total := 1
		for _, d := range t.Dims {
			n := d[1] - d[0] + 1
			if n < 1 || n > 1<<20 || total*n > 1<<22 {
				l.stop(path, "array bounds are not usable")
				return
			}
			total *= n
		}
		count := 0
		l.placeDims(t, path, comment, depth, 0, "", &count)
		if count > maxArrayItems && l.stopped == "" {
			l.b.skip(fmt.Sprintf("%s (%d more elements)", path, count-maxArrayItems), "", t.Of.Name, fmt.Sprintf("only the first %d elements of an array are proposed", maxArrayItems))
		}
		l.alignWord()
	}
}

// placeDims walks the dimensions of an array; elements beyond maxArrayItems are laid out but not proposed.
func (l *layouter) placeDims(t typ, path, comment string, depth, dim int, idx string, count *int) {
	d := t.Dims[dim]
	for i := d[0]; i <= d[1] && l.stopped == ""; i++ {
		ix := idx
		if ix != "" {
			ix += ","
		}
		ix += strconv.Itoa(i)
		if dim < len(t.Dims)-1 {
			l.alignWord()
			l.placeDims(t, path, comment, depth, dim+1, ix, count)
			l.alignWord()
			continue
		}
		*count++
		if *count > maxArrayItems {
			mute := &layouter{b: newBuilder(Options{}), udts: l.udts, blk: l.blk, addrDB: l.addrDB, byteOff: l.byteOff, bit: l.bit}
			mute.place(*t.Of, path, "", depth+1, false)
			l.byteOff, l.bit = mute.byteOff, mute.bit
			if mute.stopped != "" {
				l.stop(path, mute.stopped)
			}
			continue
		}
		l.place(*t.Of, path+"["+ix+"]", comment, depth+1, false)
	}
}

func (l *layouter) stop(path, reason string) {
	if l.stopped == "" {
		l.stopped = reason
		l.b.skip(path, "", "", reason)
	}
}
