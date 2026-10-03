// Package xlsx writes Office Open XML workbooks (.xlsx) without a third-party library: a zip of the few XML parts
// Excel, LibreOffice and Google Sheets need. Cells are text (inline strings) or numbers; a row can be bold; numbers
// carry a number format with a fixed count of decimals. Read (read.go) takes the cell texts of the sheets of a
// workbook back, for imports (spec 028). Enough for tables exported from the application (spec 023); not a general
// spreadsheet library.
package xlsx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Cell is one cell: Text, or Num when IsNum. Decimals (0–6) formats a number; Bold makes the cell bold.
type Cell struct {
	Text     string
	Num      float64
	IsNum    bool
	Decimals int
	Bold     bool
}

// T is a text cell, N a number cell with decimals, B a bold text cell.
func T(s string) Cell                { return Cell{Text: s} }
func N(v float64, decimals int) Cell { return Cell{Num: v, IsNum: true, Decimals: decimals} }
func B(s string) Cell                { return Cell{Text: s, Bold: true} }

// Sheet is the worksheet: a name (≤ 31 characters), rows of cells, and optional column widths in characters.
type Sheet struct {
	Name   string
	Rows   [][]Cell
	Widths []float64
	// FreezeRows keeps the first rows visible when scrolling (the header).
	FreezeRows int
	// TextColumns formats the columns that have a width as text, so that what people type into them stays as typed
	// (a password 0123 or an address is not turned into a number).
	TextColumns bool
}

// Write writes a workbook with one sheet.
func Write(w io.Writer, sh Sheet) error { return WriteBook(w, sh) }

// WriteBook writes a workbook with the sheets in order (the first one is shown when the file is opened).
func WriteBook(w io.Writer, sheets ...Sheet) error {
	if len(sheets) == 0 {
		sheets = []Sheet{{}}
	}
	zw := zip.NewWriter(w)
	var types, list, rels strings.Builder
	used := map[string]bool{}
	for i, sh := range sheets {
		n := i + 1
		name := sheetName(sh.Name)
		for k := 2; used[strings.ToLower(name)]; k++ {
			name = sheetName(fmt.Sprintf("%s %d", trimRunes(sheetName(sh.Name), 27), k))
		}
		used[strings.ToLower(name)] = true
		fmt.Fprintf(&types, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, n)
		fmt.Fprintf(&list, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, esc(name), n, n)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, n, n)
	}
	fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, len(sheets)+1)
	parts := []struct{ path, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` + types.String() + `<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` + list.String() + `</sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels.String() + `</Relationships>`},
		{"xl/styles.xml", styles},
	}
	for i, sh := range sheets {
		parts = append(parts, struct{ path, body string }{fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), sheetXML(sh)})
	}
	for _, p := range parts {
		f, err := zw.Create(p.path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Bytes returns the workbook (one or more sheets) as bytes.
func Bytes(sheets ...Sheet) ([]byte, error) {
	var b bytes.Buffer
	err := WriteBook(&b, sheets...)
	return b.Bytes(), err
}

// Styles: 0 normal, 1 bold, 2..8 number with 0..6 decimals, 9..15 bold number with 0..6 decimals, 16 text ("@").
const styles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="6"><numFmt numFmtId="164" formatCode="0.0"/><numFmt numFmtId="165" formatCode="0.00"/><numFmt numFmtId="166" formatCode="0.000"/><numFmt numFmtId="167" formatCode="0.0000"/><numFmt numFmtId="168" formatCode="0.00000"/><numFmt numFmtId="169" formatCode="0.000000"/></numFmts><fonts count="2"><font><sz val="11"/><name val="Arial"/></font><font><b/><sz val="11"/><name val="Arial"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="17"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/><xf numFmtId="1" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="167" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="168" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="169" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="1" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="164" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="165" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="166" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="167" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="168" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="169" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/><xf numFmtId="49" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`

func sheetXML(sh Sheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	if sh.FreezeRows > 0 {
		fmt.Fprintf(&b, `<sheetViews><sheetView workbookViewId="0"><pane ySplit="%d" topLeftCell="A%d" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`, sh.FreezeRows, sh.FreezeRows+1)
	}
	if len(sh.Widths) > 0 {
		b.WriteString("<cols>")
		for i, w := range sh.Widths {
			if w <= 0 {
				continue
			}
			style := ""
			if sh.TextColumns {
				style = ` style="16"`
			}
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%s" customWidth="1"%s/>`, i+1, i+1, strconv.FormatFloat(math.Min(w, 120), 'f', 1, 64), style)
		}
		b.WriteString("</cols>")
	}
	b.WriteString("<sheetData>")
	for r, row := range sh.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, cell := range row {
			ref := colName(c) + strconv.Itoa(r+1)
			switch {
			case cell.IsNum && !math.IsNaN(cell.Num) && !math.IsInf(cell.Num, 0):
				d := max(0, min(6, cell.Decimals))
				st := 2 + d
				if cell.Bold {
					st += 7
				}
				fmt.Fprintf(&b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, st, strconv.FormatFloat(cell.Num, 'f', -1, 64))
			case cell.Text != "":
				st := 0
				if cell.Bold {
					st = 1
				}
				fmt.Fprintf(&b, `<c r="%s" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, st, esc(cell.Text))
			}
		}
		b.WriteString("</row>")
	}
	b.WriteString("</sheetData></worksheet>")
	return b.String()
}

// colName turns 0 into A, 25 into Z, 26 into AA.
func colName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// sheetName keeps a name Excel accepts: 1–31 characters without : \ / ? * [ ].
func sheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		s = "Sheet1"
	}
	return strings.TrimSpace(trimRunes(s, 31))
}

// trimRunes keeps at most n characters.
func trimRunes(s string, n int) string {
	for utf8.RuneCountInString(s) > n {
		_, k := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-k]
	}
	return s
}

// esc escapes text for XML and drops characters XML 1.0 does not allow.
func esc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != 0xFFFE && r != 0xFFFF && r != utf8.RuneError):
			b.WriteRune(r)
		}
	}
	return b.String()
}
