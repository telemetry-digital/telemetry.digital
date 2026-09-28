package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestWorkbook(t *testing.T) {
	b, err := Bytes(Sheet{Name: "Temperatures: fridge [1]/2 and a very long name beyond 31", FreezeRows: 3, Widths: []float64{12, 10},
		Rows: [][]Cell{{B("Title & <more>")}, {T("Period"), T("1. 9. 2026 – 30. 9. 2026")}, {B("Day"), B("Fridge 1 min [°C]")},
			{T("2026-09-01"), N(4.25, 1)}, {T("total"), {Num: 12.5, IsNum: true, Decimals: 2, Bold: true}}, {T("bad \x01 char")}}})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(body)
		// every part is well-formed XML
		dec := xml.NewDecoder(bytes.NewReader(body))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
	}
	for _, p := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		if files[p] == "" {
			t.Errorf("missing %s", p)
		}
	}
	sh := files["xl/worksheets/sheet1.xml"]
	for _, want := range []string{`<c r="B4" s="3"><v>4.25</v>`, `s="11"><v>12.5</v>`, `Title &amp; &lt;more&gt;`, `°C`, `state="frozen"`, `<col min="1" max="1" width="12.0"`, `bad  char`} {
		if !strings.Contains(sh, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if !strings.Contains(files["xl/workbook.xml"], `name="Temperatures- fridge -1--2 and"`) {
		t.Errorf("sheet name: %s", files["xl/workbook.xml"])
	}
	if colName(0) != "A" || colName(25) != "Z" || colName(26) != "AA" || colName(701) != "ZZ" || colName(702) != "AAA" {
		t.Error("column names")
	}
}
