package xlsx

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestWorkbookRoundTrip(t *testing.T) {
	b, err := Bytes(
		Sheet{Name: "Cameras", TextColumns: true, Widths: []float64{10, 10, 10}, Rows: [][]Cell{{B("id"), B("name"), B("retention")}, {T(""), T("Gate & <yard>"), N(7, 0)}, nil, {T("x"), T("0123"), N(2.5, 1)}}},
		Sheet{Name: "Read me", Rows: [][]Cell{{T("help")}}},
		Sheet{Name: "Cameras"}, // a duplicate name gets a number
	)
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 3 || sheets[0].Name != "Cameras" || sheets[1].Name != "Read me" || sheets[2].Name != "Cameras 2" {
		t.Fatalf("sheets: %+v", sheets)
	}
	rows := sheets[0].Rows
	if len(rows) != 4 || rows[1][1] != "Gate & <yard>" || rows[1][2] != "7" || rows[1][0] != "" || len(rows[2]) != 0 || rows[3][1] != "0123" || rows[3][2] != "2.5" {
		t.Fatalf("rows: %q", rows)
	}
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			if !strings.Contains(buf.String(), `style="16"`) {
				t.Error("text columns not formatted as text")
			}
		}
	}
}

// A workbook as Excel saves it: shared strings (one of them rich text), booleans, cells without references.
func TestReadExcelStyle(t *testing.T) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	put := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	put("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Kamery" sheetId="1" r:id="rId3"/></sheets></workbook>`)
	put("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="x" Target="worksheets/data.xml"/></Relationships>`)
	put("xl/sharedStrings.xml", `<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>name</t></si><si><r><t>Brá</t></r><r><t>na</t></r><rPh><t>x</t></rPh></si></sst>`)
	put("xl/worksheets/data.xml", `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="b"><v>1</v></c></row>
<row r="3"><c t="s"><v>1</v></c><c><v>192.5</v></c><c t="str"><f>A1</f><v>calc</v></c></row></sheetData></worksheet>`)
	_ = zw.Close()
	sheets, err := Read(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	r := sheets[0].Rows
	if sheets[0].Name != "Kamery" || len(r) != 3 || r[0][0] != "name" || r[0][1] != "" || r[0][2] != "TRUE" || r[2][0] != "Brána" || r[2][1] != "192.5" || r[2][2] != "calc" {
		t.Fatalf("rows: %q", r)
	}
	if _, err := Read([]byte("id;name\n")); err == nil {
		t.Error("a CSV is not a workbook")
	}
}
