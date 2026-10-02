package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// ReadSheet is a sheet read back from a workbook: its name and the texts of its cells by row (empty cells are "").
type ReadSheet struct {
	Name string
	Rows [][]string
}

// Read limits: the uncompressed size of one part and the number of cells, against zip bombs and huge files.
const (
	maxPartBytes = 32 << 20
	maxCells     = 2_000_000
)

// Read returns the sheets of a workbook in their order, with the text of every cell: shared and inline strings as
// they are, numbers as written in the file, booleans as TRUE/FALSE. Formulas give their last calculated value.
func Read(data []byte) ([]ReadSheet, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not an .xlsx file (zip)")
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}
	part := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("%s missing", name)
		}
		if f.UncompressedSize64 > maxPartBytes {
			return nil, fmt.Errorf("%s too large", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, maxPartBytes+1))
	}
	wb, err := part("xl/workbook.xml")
	if err != nil {
		return nil, errors.New("not an .xlsx workbook: " + err.Error())
	}
	var book struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(wb, &book); err != nil {
		return nil, errors.New("workbook.xml: " + err.Error())
	}
	targets := map[string]string{}
	if rb, err := part("xl/_rels/workbook.xml.rels"); err == nil {
		var rels struct {
			Rel []struct {
				ID     string `xml:"Id,attr"`
				Target string `xml:"Target,attr"`
			} `xml:"Relationship"`
		}
		if xml.Unmarshal(rb, &rels) == nil {
			for _, r := range rels.Rel {
				t := r.Target
				if strings.HasPrefix(t, "/") {
					t = strings.TrimPrefix(t, "/")
				} else {
					t = path.Join("xl", t)
				}
				targets[r.ID] = t
			}
		}
	}
	var shared []string
	if sb, err := part("xl/sharedStrings.xml"); err == nil {
		shared, err = readShared(sb)
		if err != nil {
			return nil, errors.New("sharedStrings.xml: " + err.Error())
		}
	}
	cells := 0
	var out []ReadSheet
	for i, sh := range book.Sheets {
		target := targets[sh.RID]
		if target == "" {
			target = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		body, err := part(target)
		if err != nil {
			return nil, err
		}
		rows, n, err := readRows(body, shared, maxCells-cells)
		if err != nil {
			return nil, fmt.Errorf("sheet %q: %w", sh.Name, err)
		}
		cells += n
		out = append(out, ReadSheet{Name: sh.Name, Rows: rows})
	}
	if len(out) == 0 {
		return nil, errors.New("the workbook has no sheet")
	}
	return out, nil
}

// readShared reads the shared string table; a rich text item is the text of all its runs.
func readShared(b []byte) ([]string, error) {
	var out []string
	dec := xml.NewDecoder(bytes.NewReader(b))
	var cur strings.Builder
	in, inT := false, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				in = true
				cur.Reset()
			case "t":
				inT = in
			case "rPh": // phonetic runs are not part of the text
				_ = dec.Skip()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				in = false
				out = append(out, cur.String())
				if len(out) > maxCells {
					return nil, errors.New("too many strings")
				}
			case "t":
				inT = false
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
}

// readRows reads the cells of a worksheet into rows of texts, placed by their references (A1, C7).
func readRows(b []byte, shared []string, limit int) ([][]string, int, error) {
	var rows [][]string
	dec := xml.NewDecoder(bytes.NewReader(b))
	n := 0
	rowIdx := -1
	col := 0
	var typ string
	var val, inline strings.Builder
	inV, inT, inIs := false, false, false
	ref := ""
	set := func(r, c int, s string) {
		for len(rows) <= r {
			rows = append(rows, nil)
		}
		for len(rows[r]) <= c {
			rows[r] = append(rows[r], "")
		}
		rows[r][c] = s
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				rowIdx++
				col = 0
				for _, a := range t.Attr {
					if a.Name.Local == "r" {
						if v, err := strconv.Atoi(a.Value); err == nil && v >= 1 && v <= 1_048_576 {
							rowIdx = v - 1
						}
					}
				}
			case "c":
				typ, ref = "", ""
				val.Reset()
				inline.Reset()
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "t":
						typ = a.Value
					case "r":
						ref = a.Value
					}
				}
			case "v":
				inV = true
			case "is":
				inIs = true
			case "t":
				inT = inIs
			case "rPh":
				_ = dec.Skip()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
			case "is":
				inIs = false
			case "t":
				inT = false
			case "c":
				c := col
				if ref != "" {
					if x, ok := refCol(ref); ok {
						c = x
					}
				}
				col = c + 1
				var s string
				switch typ {
				case "s":
					i, err := strconv.Atoi(strings.TrimSpace(val.String()))
					if err == nil && i >= 0 && i < len(shared) {
						s = shared[i]
					}
				case "inlineStr":
					s = inline.String()
				case "b":
					if strings.TrimSpace(val.String()) == "1" {
						s = "TRUE"
					} else {
						s = "FALSE"
					}
				default: // n, str, e
					s = val.String()
				}
				if s != "" && rowIdx >= 0 && c < 16384 {
					n++
					if n > limit {
						return nil, 0, errors.New("too many cells")
					}
					set(rowIdx, c, s)
				}
			}
		case xml.CharData:
			if inV {
				val.Write(t)
			} else if inT {
				inline.Write(t)
			}
		}
	}
	return rows, n, nil
}

// refCol turns the letters of a cell reference into a column index (A1 → 0, AB3 → 27).
func refCol(ref string) (int, bool) {
	c := 0
	k := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			c = c*26 + int(r-'A'+1)
			k++
			continue
		}
		if r >= 'a' && r <= 'z' {
			c = c*26 + int(r-'a'+1)
			k++
			continue
		}
		break
	}
	if k == 0 || k > 3 {
		return 0, false
	}
	return c - 1, true
}
