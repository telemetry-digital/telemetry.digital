package camerainv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

// Meta is the header of an exported file: what it is, from where, when and by whom.
type Meta struct {
	Org, Server, User, Version string
	ExportedAt                 string // local time of the organization, "2006-01-02 15:04 MST"
	Confidential               bool   // the file holds passwords
}

// Confidential is the marking of a file with passwords (header, footer, sheet name).
const Confidential = "CONFIDENTIAL — contains passwords"

// metaLines are the lines above the table of the Cameras sheet and the CSV.
func metaLines(m Meta, n int) [][2]string {
	out := [][2]string{
		{"Camera inventory", m.Org},
		{"Server", m.Server},
		{"Exported", m.ExportedAt},
		{"Exported by", m.User},
		{"Version", m.Version},
		{"Cameras", strconv.Itoa(n)},
	}
	if m.Confidential {
		out = append([][2]string{{Confidential, "Keep this file like the keys to the building; delete copies you no longer need."}}, out...)
	}
	return out
}

// XLSX writes the inventory workbook: the sheet "Cameras" (or "CONFIDENTIAL cameras") and the sheet "Read me".
func XLSX(m Meta, cams []Camera) ([]byte, error) {
	cols := visible(m.Confidential)
	sheet := xlsx.Sheet{Name: "Cameras", TextColumns: true}
	if m.Confidential {
		sheet.Name = "CONFIDENTIAL cameras"
	}
	for _, l := range metaLines(m, len(cams)) {
		sheet.Rows = append(sheet.Rows, []xlsx.Cell{xlsx.B(l[0]), xlsx.T(l[1])})
	}
	sheet.Rows = append(sheet.Rows, nil)
	hdr := make([]xlsx.Cell, len(cols))
	for i, c := range cols {
		hdr[i] = xlsx.B(c.Key)
		sheet.Widths = append(sheet.Widths, c.Width)
	}
	sheet.Rows = append(sheet.Rows, hdr)
	sheet.FreezeRows = len(sheet.Rows)
	for _, cam := range cams {
		vals := cam.Values(m.Confidential)
		row := make([]xlsx.Cell, len(vals))
		for i, v := range vals {
			if numeric(cols[i].Key) {
				if n, err := strconv.Atoi(v); err == nil {
					row[i] = xlsx.N(float64(n), 0)
					continue
				}
			}
			row[i] = xlsx.T(v)
		}
		sheet.Rows = append(sheet.Rows, row)
	}
	return xlsx.Bytes(sheet, readMe(m.Confidential))
}

// Template is an empty inventory with two example rows (their id starts with #, so they are not imported).
func Template(format string) ([]byte, error) {
	cols := visible(true)
	ex1 := make([]string, len(cols))
	ex2 := make([]string, len(cols))
	for i, c := range cols {
		ex1[i] = c.Example
		switch c.Key {
		case "id":
			ex2[i] = "#example"
		case "name":
			ex2[i] = "Warehouse door"
		case "make":
			ex2[i] = "tapo"
		case "ip":
			ex2[i] = "192.168.1.70"
		case "username":
			ex2[i] = "camera-account"
		case "password":
			ex2[i] = "typed-here-or-empty"
		case "location", "group", "connection", "enabled", "recording", "retention_days":
			ex2[i] = c.Example
		}
		if c.ExportOnly {
			ex1[i], ex2[i] = "", ""
		}
	}
	hdr := Headers(true)
	if format == "csv" {
		var b bytes.Buffer
		b.WriteString("\ufeff")
		w := csv.NewWriter(&b)
		w.Comma = ';'
		w.UseCRLF = true
		_ = w.Write([]string{"# Camera inventory template: fill one row per camera, delete or keep the example rows (their id starts with #). Columns: see the Read me sheet of the Excel template or the documentation."})
		_ = w.Write(hdr)
		_ = w.Write(ex1)
		_ = w.Write(ex2)
		w.Flush()
		return b.Bytes(), w.Error()
	}
	sheet := xlsx.Sheet{Name: "Cameras", TextColumns: true}
	sheet.Rows = append(sheet.Rows, []xlsx.Cell{xlsx.B("Camera inventory template"), xlsx.T("One row per camera. Rows whose id starts with # are examples and are not imported. An empty cell takes the default value.")}, nil)
	h := make([]xlsx.Cell, len(hdr))
	for i, k := range hdr {
		h[i] = xlsx.B(k)
		sheet.Widths = append(sheet.Widths, cols[i].Width)
	}
	sheet.Rows = append(sheet.Rows, h)
	sheet.FreezeRows = len(sheet.Rows)
	for _, ex := range [][]string{ex1, ex2} {
		row := make([]xlsx.Cell, len(ex))
		for i, v := range ex {
			row[i] = xlsx.T(v)
		}
		sheet.Rows = append(sheet.Rows, row)
	}
	return xlsx.Bytes(sheet, readMe(true))
}

func visible(withCredentials bool) []Column {
	var out []Column
	for _, c := range Columns {
		if c.Credential && !withCredentials {
			continue
		}
		out = append(out, c)
	}
	return out
}

// readMe explains the columns and the import.
func readMe(withCredentials bool) xlsx.Sheet {
	sh := xlsx.Sheet{Name: "Read me", Widths: []float64{20, 90, 40}}
	sh.Rows = [][]xlsx.Cell{
		{xlsx.B("Camera inventory")},
		{xlsx.T("Every setting of the cameras of one organization. Keep it with the documentation of the building; import it to put the cameras back after a crash or on a new server.")},
		{xlsx.T("Import: Cameras → Camera management → Import (Excel or CSV with these columns). The preview shows what will change; nothing changes until you confirm with a reason.")},
		{xlsx.T("An empty cell keeps the value of an existing camera (a new camera gets the default). A dash (-) clears a text such as location or sub_stream_url. Rows whose id starts with # are ignored.")},
		{xlsx.T("Cameras are matched by id, else by name (and ip when several cameras have the name). Import never deletes a camera.")},
		nil,
		{xlsx.B("Column"), xlsx.B("Meaning"), xlsx.B("Values")},
	}
	for _, c := range Columns {
		if c.Credential && !withCredentials {
			continue
		}
		sh.Rows = append(sh.Rows, []xlsx.Cell{xlsx.T(c.Key), xlsx.T(c.Help), xlsx.T(c.Values)})
	}
	if !withCredentials {
		sh.Rows = append(sh.Rows, nil, []xlsx.Cell{xlsx.T("password"), xlsx.T("Passwords are not in this file. A file with them is made only with the permission to export camera passwords, after confirming your password; on import an empty password keeps the stored one."), xlsx.T("")})
	}
	return sh
}

// CSV writes the inventory as CSV: UTF-8 with a byte-order mark and semicolons, which Excel opens in columns in
// languages that use the decimal comma (Slovak, Czech, German, …) and LibreOffice and Google Sheets recognise. The
// header lines start with #.
func CSV(m Meta, cams []Camera) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	w.Comma = ';'
	w.UseCRLF = true
	for _, l := range metaLines(m, len(cams)) {
		_ = w.Write([]string{"# " + l[0] + ": " + l[1]})
	}
	_ = w.Write(Headers(m.Confidential))
	for _, c := range cams {
		_ = w.Write(c.Values(m.Confidential))
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// ---- reading files -------------------------------------------------------------------------------------------------

// File is an imported file.
type File struct {
	Format  string   // xlsx or csv
	Sheet   string   // the sheet read (xlsx)
	Columns []string // recognised columns, in file order
	Ignored []string // headers that are not columns of the inventory (or export only)
	Rows    []Row
}

// Has reports whether the file has the column.
func (f *File) Has(key string) bool {
	for _, c := range f.Columns {
		if c == key {
			return true
		}
	}
	return false
}

// Parse reads an inventory from an Excel workbook or a CSV file (recognised by content) with at most maxRows camera
// rows. The table starts at the first row whose cells include the column name (or id).
func Parse(data []byte, maxRows int) (*File, error) {
	if len(data) == 0 {
		return nil, errors.New("the file is empty")
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		sheets, err := xlsx.Read(data)
		if err != nil {
			return nil, err
		}
		for _, sh := range sheets {
			if f, ok := table(sh.Rows, maxRows); ok {
				f.Format, f.Sheet = "xlsx", sh.Name
				return f.check(maxRows)
			}
		}
		return nil, errors.New("no sheet has a header row with the column name or id (see the template)")
	}
	text, err := decodeText(data)
	if err != nil {
		return nil, err
	}
	rows, err := readCSV(text)
	if err != nil {
		return nil, err
	}
	f, ok := table(rows, maxRows)
	if !ok {
		return nil, errors.New("no header row with the column name or id (see the template)")
	}
	f.Format = "csv"
	return f.check(maxRows)
}

func (f *File) check(maxRows int) (*File, error) {
	if len(f.Rows) > maxRows {
		return nil, fmt.Errorf("the file has %d camera rows; at most %d at once", len(f.Rows), maxRows)
	}
	return f, nil
}

// decodeText returns the text of a CSV file: UTF-8 (with or without a byte-order mark), UTF-16 as Excel's "Unicode
// text", else Windows-1250 — what Excel saves as "CSV" in Central European languages.
func decodeText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:]), nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data)
	case utf8.Valid(data):
		return string(data), nil
	}
	s, err := charmap.Windows1250.NewDecoder().Bytes(data)
	if err != nil {
		return "", errors.New("the file is neither UTF-8 nor Windows-1250 text")
	}
	return string(s), nil
}

func decodeUTF16(data []byte) (string, error) {
	le := data[0] == 0xFF
	data = data[2:]
	if len(data)%2 != 0 {
		return "", errors.New("broken UTF-16 text")
	}
	u := make([]uint16, len(data)/2)
	for i := range u {
		if le {
			u[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
		} else {
			u[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
		}
	}
	var b strings.Builder
	for i := 0; i < len(u); i++ {
		c := rune(u[i])
		if c >= 0xD800 && c < 0xDC00 && i+1 < len(u) {
			c = (c-0xD800)<<10 + (rune(u[i+1]) - 0xDC00) + 0x10000
			i++
		}
		b.WriteRune(c)
	}
	return b.String(), nil
}

// readCSV splits CSV text with the separator of its header line (semicolon, comma or tab).
func readCSV(text string) ([][]string, error) {
	sep := ';'
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, `"#`) {
			continue
		}
		best := 0
		for _, c := range []rune{';', ',', '\t'} {
			if n := strings.Count(l, string(c)); n > best {
				best, sep = n, c
			}
		}
		break
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = false
	var rows [][]string
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, fmt.Errorf("CSV line %d: %v", pe.Line, pe.Err)
			}
			return nil, err
		}
		rows = append(rows, rec)
		if len(rows) > 100_000 {
			return nil, errors.New("the file has too many lines")
		}
	}
	return rows, nil
}

// normKey makes a header comparable: lower case, spaces and dashes as underscores.
func normKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")))
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)
	return s
}

// table finds the header row and reads the camera rows below it.
func table(rows [][]string, maxRows int) (*File, bool) {
	known := map[string]Column{}
	for _, c := range Columns {
		known[c.Key] = c
	}
	for hi, row := range rows {
		idx := map[int]string{}
		hasName := false
		f := &File{}
		for i, cell := range row {
			k := normKey(cell)
			if k == "" {
				continue
			}
			if c, ok := known[k]; ok && !c.ExportOnly {
				idx[i] = k
				f.Columns = append(f.Columns, k)
				if k == "name" || k == "id" {
					hasName = true
				}
			} else {
				f.Ignored = append(f.Ignored, strings.TrimSpace(cell))
			}
		}
		if !hasName || len(idx) < 2 {
			continue
		}
		for ri := hi + 1; ri < len(rows); ri++ {
			rec := rows[ri]
			r := Row{Line: ri + 1}
			skip := false
			for i, cell := range rec {
				k, ok := idx[i]
				if !ok {
					continue
				}
				if k == "id" && strings.HasPrefix(strings.TrimSpace(cell), "#") {
					skip = true
					break
				}
				r.set(k, cell)
			}
			if skip || r.Empty() {
				continue
			}
			// a comment line of a CSV (# …) in the first column
			if len(rec) > 0 && strings.HasPrefix(strings.TrimSpace(rec[0]), "#") && idx[0] != "id" {
				continue
			}
			f.Rows = append(f.Rows, r)
			if len(f.Rows) > maxRows {
				return f, true
			}
		}
		return f, true
	}
	return nil, false
}
