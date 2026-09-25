package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
)

// Report templates are versioned content (ADR-0010): built-in defaults ship in the binary, an organization may
// override a kind+locale with its own Typst source. Every save is a new version with a reason; the current
// version is superseded (valid_to), never edited. Generated PDFs are records (core.report).
//
//	GET    /api/v1/report-templates                       list built-in kinds and the org's overrides   (data.export)
//	GET    /api/v1/report-templates/{kind}?locale=        resolved template incl. body                  (data.export)
//	GET    /api/v1/report-templates/{kind}/contract       sample data.json the template receives        (data.export)
//	GET    /api/v1/report-templates/{kind}/preview        PDF of the current template with sample data  (data.export)
//	PUT    /api/v1/report-templates/{kind}                new version {locale,name,body,reason}         (content.write)
//	DELETE /api/v1/report-templates/{kind}?locale=        revert to built-in                            (content.write)
//	POST   /api/v1/report-templates/preview               {kind,locale,body} -> PDF, nothing stored     (content.write)
//	GET    /api/v1/reports?limit=                         stored reports                                (data.export)
//	GET    /api/v1/reports/{id}                           metadata                                      (data.export)
//	GET    /api/v1/reports/{id}/file                      PDF, hash verified before serving             (data.export)

func (s *Server) reportsUnavailable(w http.ResponseWriter) bool {
	if s.rep == nil {
		writeErr(w, 503, "PDF reports unavailable: typst is not installed on this server")
		return true
	}
	return false
}

func kindParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	kind := r.PathValue("kind")
	if len(kind) < 2 || len(kind) > 64 || strings.Trim(kind, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		writeErr(w, 400, "kind must be lowercase [a-z0-9_]")
		return "", false
	}
	return kind, true
}

func localeParam(r *http.Request, fallback string) string {
	l := r.URL.Query().Get("locale")
	if l == "" || len(l) > 8 || strings.Trim(l, "abcdefghijklmnopqrstuvwxyz-_*") != "" {
		return fallback
	}
	return l
}

func (s *Server) listReportTemplates(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	overrides, err := s.rep.ListOverrides(r.Context(), orgOf(p))
	if err != nil {
		s.log.Error("list templates", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"builtin_kinds": report.Kinds(), "overrides": overrides, "engine": "typst", "engine_version": s.rep.Version(), "max_rows": report.MaxRows})
}

func (s *Server) getReportTemplate(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	t, err := s.rep.Resolve(r.Context(), orgOf(p), kind, localeParam(r, "en"))
	if err != nil {
		writeErr(w, 404, "unknown template kind")
		return
	}
	writeJSON(w, 200, map[string]any{"template": t, "layout": report.Layout(), "blocks": report.BlockCatalog()})
}

func (s *Server) reportTemplateContract(w http.ResponseWriter, r *http.Request) {
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, report.SampleData(kind, localeParam(r, "en")))
}

func (s *Server) previewReportTemplate(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	locale := localeParam(r, "en")
	t, err := s.rep.Resolve(r.Context(), orgOf(p), kind, locale)
	if err != nil {
		writeErr(w, 404, "unknown template kind")
		return
	}
	logo, logoName := s.orgLogo(r, orgOf(p))
	pdf, err := s.rep.Preview(r.Context(), t.Body, kind, locale, logo, logoName)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	servePDF(w, pdf, kind+"_preview.pdf", "")
}

func (s *Server) previewReportSource(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	var in struct {
		Kind   string `json:"kind"`
		Locale string `json:"locale"`
		Body   string `json:"body"`
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Kind == "" {
		in.Kind = "generic"
	}
	if in.Locale == "" {
		in.Locale = "en"
	}
	if len(in.Body) > 512*1024 {
		writeErr(w, 413, "body too large")
		return
	}
	p := PrincipalFrom(r.Context())
	logo, logoName := s.orgLogo(r, orgOf(p))
	pdf, err := s.rep.Preview(r.Context(), in.Body, in.Kind, in.Locale, logo, logoName)
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	servePDF(w, pdf, in.Kind+"_preview.pdf", "")
}

func (s *Server) putReportTemplate(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if orgOf(p) == "" {
		writeErr(w, 403, "templates are stored per organization")
		return
	}
	var in struct {
		Locale string          `json:"locale"`
		Name   string          `json:"name"`
		Body   string          `json:"body"`
		Spec   json.RawMessage `json:"spec"` // optional: block designer; the body is generated from it
		Reason string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Locale == "" {
		in.Locale = "en"
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	specJSON := ""
	if len(in.Spec) > 0 && string(in.Spec) != "null" {
		sp, err := report.ParseSpec(in.Spec)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		in.Body = report.Compile(sp)
		specJSON = report.SpecJSON(sp)
	}
	prev, _ := s.rep.Resolve(r.Context(), p.OrgID, kind, in.Locale)
	t, err := s.rep.SaveVersion(r.Context(), p.OrgID, kind, in.Locale, in.Name, in.Body, specJSON, p.UserID, in.Reason)
	if err != nil {
		if strings.Contains(err.Error(), "does not compile") || strings.Contains(err.Error(), "body must") {
			writeErr(w, 422, err.Error())
			return
		}
		s.log.Error("save template", "err", err)
		writeErr(w, 500, "save failed")
		return
	}
	var old any
	if prev != nil && !prev.BuiltIn {
		old = map[string]any{"id": prev.ID, "version": prev.Version, "checksum": prev.Checksum}
	}
	s.audit(r, "report_template.update", "report_template", t.ID, old, map[string]any{"kind": t.Kind, "locale": t.Locale, "version": t.Version, "checksum": t.Checksum, "name": t.Name}, in.Reason)
	t.Body = ""
	writeJSON(w, 201, t)
}

func (s *Server) deleteReportTemplate(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if orgOf(p) == "" {
		writeErr(w, 403, "templates are stored per organization")
		return
	}
	locale := localeParam(r, "en")
	n, err := s.rep.Revert(r.Context(), p.OrgID, kind, locale)
	if err != nil {
		writeErr(w, 500, "revert failed")
		return
	}
	if n == 0 {
		writeErr(w, 404, "no override for this kind and locale")
		return
	}
	s.audit(r, "report_template.revert", "report_template", "", map[string]any{"kind": kind, "locale": locale}, nil, r.URL.Query().Get("reason"))
	writeJSON(w, 200, map[string]any{"reverted": n, "kind": kind, "locale": locale})
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	p := PrincipalFrom(r.Context())
	out, err := s.rep.List(r.Context(), orgOf(p), limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	rec, err := s.rep.Get(r.Context(), orgOf(p), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 404, "report not found")
			return
		}
		writeErr(w, 400, "invalid id")
		return
	}
	writeJSON(w, 200, rec)
}

func (s *Server) getReportFile(w http.ResponseWriter, r *http.Request) {
	if s.reportsUnavailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	rec, err := s.rep.Get(r.Context(), orgOf(p), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "report not found")
		return
	}
	pdf, err := s.rep.ReadFile(rec)
	if err != nil {
		if errors.Is(err, report.ErrTampered) {
			s.audit(r, "report.integrity_failure", "report", rec.ID, nil, map[string]any{"file_hash": rec.FileHash}, "")
			writeErr(w, 409, "stored file does not match its recorded hash")
			return
		}
		writeErr(w, 404, "report file missing")
		return
	}
	s.audit(r, "report.download", "report", rec.ID, nil, map[string]any{"kind": rec.Kind, "file_hash": rec.FileHash}, "")
	servePDF(w, pdf, fmt.Sprintf("%s_%s.pdf", rec.Kind, rec.GeneratedAt.UTC().Format("20060102T150405Z")), rec.ID)
}

func servePDF(w http.ResponseWriter, pdf []byte, filename, reportID string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	if reportID != "" {
		w.Header().Set("X-Report-Id", reportID)
	}
	w.WriteHeader(200)
	_, _ = w.Write(pdf)
}

// POST /api/v1/report-templates/compile {spec} -> {body}: the Typst source the block designer would save.
func (s *Server) compileReportSpec(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sp, err := report.ParseSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"body": report.Compile(sp), "spec": sp})
}

// GET /api/v1/report-templates/{kind}/default-spec: the block representation of the built-in template.
func (s *Server) defaultReportSpec(w http.ResponseWriter, r *http.Request) {
	kind, ok := kindParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"spec": report.DefaultSpec(kind), "blocks": report.BlockCatalog()})
}
