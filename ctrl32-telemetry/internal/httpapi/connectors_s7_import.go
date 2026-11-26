package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7/tagimport"
)

// s7ImportTags reads a tag export of TIA Portal or STEP 7 and proposes tags for an S7 connector (0.91.0):
//
//	POST /api/v1/connectors/s7/import   multipart: file, encoding, db, prefix, existing   (config.write)
//
// Nothing is stored and nothing is written to a PLC: the answer is a list the user picks from. Proposed tags are
// never writable (SAFETY FIRST: making a tag writable stays a deliberate edit of the connector).
func (s *Server) s7ImportTags(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustOrg(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, tagimport.MaxFileBytes+256<<10)
	if err := r.ParseMultipartForm(tagimport.MaxFileBytes); err != nil {
		writeErr(w, 400, "multipart form with the file (at most 8 MB)")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file is required: a PLC tag table (.xlsx, .xml), a symbol table (.sdf, .asc, .seq), a data block source (.db, .awl) or a table (.csv)")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, tagimport.MaxFileBytes+1))
	if err != nil || len(data) > tagimport.MaxFileBytes {
		writeErr(w, 400, "the file is larger than 8 MB")
		return
	}
	opt := tagimport.Options{Filename: hdr.Filename, Encoding: r.FormValue("encoding"), Prefix: strings.TrimSpace(r.FormValue("prefix"))}
	if v := strings.TrimSpace(r.FormValue("db")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			writeErr(w, 400, "data block number must be 1..65535")
			return
		}
		opt.DB = n
	}
	for _, k := range strings.FieldsFunc(r.FormValue("existing"), func(c rune) bool { return c == '\n' || c == ',' || c == ' ' || c == '\r' }) {
		opt.Existing = append(opt.Existing, k)
	}
	res, err := tagimport.Parse(data, opt)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"file": hdr.Filename, "format": res.Format, "tags": res.Tags, "skipped": res.Skipped, "blocks": res.Blocks, "warnings": res.Warnings,
		"max_tags": s7.MaxTags})
}
