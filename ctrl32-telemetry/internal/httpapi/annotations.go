package httpapi

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Annotations of observations (data.annotate, URS C-11, P-03; 0.67.0): a comment on one observation (from = to) or
// on a period of a datastream — "door opened for loading", "probe replaced", "expected event". The data never
// changes. Annotations are records (core.annotation, append-only): a wrong one is retracted by a new row that names
// the reason. They are shown on the charts of the device page and written to the observation exports.
//
//	GET  /api/v1/annotations?datastream_id=&from=&to=[&include_retracted=1]   (data.read)
//	POST /api/v1/annotations {datastream_id, from, to, category, text}         (data.annotate)
//	POST /api/v1/annotations/{id}/retract {reason}                            (data.annotate)

var annotationCategories = map[string]bool{"note": true, "expected_event": true, "maintenance": true, "calibration": true, "sensor_fault": true, "data_issue": true}

type annotation struct {
	ID            string     `json:"id"`
	DatastreamID  string     `json:"datastream_id"`
	From          time.Time  `json:"from"`
	To            time.Time  `json:"to"`
	Category      string     `json:"category"`
	Text          string     `json:"text"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	RetractedAt   *time.Time `json:"retracted_at,omitempty"`
	RetractedBy   string     `json:"retracted_by,omitempty"`
	RetractReason string     `json:"retract_reason,omitempty"`
}

// annotationsFor returns the annotations of a datastream that overlap the range (without retracted ones).
func (s *Server) annotationsFor(r *http.Request, org, dsID string, from, to time.Time) []annotation {
	out, _ := s.queryAnnotations(r, org, dsID, from, to, false)
	return out
}

func (s *Server) queryAnnotations(r *http.Request, org, dsID string, from, to time.Time, withRetracted bool) ([]annotation, error) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.id::text, a.datastream_id::text, a.ts_from, a.ts_to, a.category, a.text, a.created_by_name, a.created_at,
		       x.created_at, x.created_by_name, x.text
		FROM core.annotation a LEFT JOIN core.annotation x ON x.retracts = a.id
		WHERE a.datastream_id = $1::uuid AND a.org_id = $2::uuid AND a.retracts IS NULL
		  AND a.ts_from < $4 AND a.ts_to >= $3 AND ($5 OR x.id IS NULL)
		ORDER BY a.ts_from LIMIT 1000`, dsID, org, from, to, withRetracted)
	if err != nil {
		return []annotation{}, err
	}
	defer rows.Close()
	out := []annotation{}
	for rows.Next() {
		var a annotation
		var rBy, rText *string
		if err := rows.Scan(&a.ID, &a.DatastreamID, &a.From, &a.To, &a.Category, &a.Text, &a.CreatedBy, &a.CreatedAt, &a.RetractedAt, &rBy, &rText); err != nil {
			return out, err
		}
		if rBy != nil {
			a.RetractedBy = *rBy
		}
		if rText != nil {
			a.RetractReason = *rText
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GET /api/v1/annotations
func (s *Server) listAnnotations(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	dsID := q.Get("datastream_id")
	if !uuidRe.MatchString(dsID) {
		writeErr(w, 400, "datastream_id required")
		return
	}
	from, to, ok := timeRange(w, r, 30*24*time.Hour)
	if !ok {
		return
	}
	out, err := s.queryAnnotations(r, org, dsID, from, to, q.Get("include_retracted") == "1")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/annotations
func (s *Server) createAnnotation(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		DatastreamID string     `json:"datastream_id"`
		From         time.Time  `json:"from"`
		To           *time.Time `json:"to"`
		Category     string     `json:"category"`
		Text         string     `json:"text"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" || utf8.RuneCountInString(in.Text) > 1000 {
		writeErr(w, 400, "text required (at most 1000 characters)")
		return
	}
	if in.Category == "" {
		in.Category = "note"
	}
	if !annotationCategories[in.Category] {
		writeErr(w, 400, "category must be note, expected_event, maintenance, calibration, sensor_fault or data_issue")
		return
	}
	if in.From.IsZero() {
		writeErr(w, 400, "from required (RFC 3339)")
		return
	}
	to := in.From
	if in.To != nil {
		to = *in.To
	}
	if to.Before(in.From) || to.Sub(in.From) > 366*24*time.Hour {
		writeErr(w, 400, "to must be at or after from, at most 366 days later")
		return
	}
	if in.From.After(time.Now().Add(24 * time.Hour)) {
		writeErr(w, 400, "an annotation cannot start in the future")
		return
	}
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.annotation (org_id, datastream_id, ts_from, ts_to, category, text, created_by, created_by_name)
		SELECT $1::uuid, d.id, $3, $4, $5, $6, NULLIF($7,'')::uuid, $8 FROM core.datastream d WHERE d.id = $2::uuid AND d.org_id = $1::uuid
		RETURNING id::text`, org, in.DatastreamID, in.From.UTC(), to.UTC(), in.Category, in.Text, p.UserID, actorName(r)).Scan(&id)
	if err != nil {
		writeErr(w, 404, "datastream not found")
		return
	}
	s.audit(r, "annotation.create", "annotation", id, nil, map[string]any{"datastream_id": in.DatastreamID, "from": in.From.UTC(), "to": to.UTC(), "category": in.Category, "text": in.Text}, in.Text)
	writeJSON(w, 201, annotation{ID: id, DatastreamID: in.DatastreamID, From: in.From.UTC(), To: to.UTC(), Category: in.Category, Text: in.Text, CreatedBy: actorName(r), CreatedAt: time.Now().UTC()})
}

// POST /api/v1/annotations/{id}/retract
func (s *Server) retractAnnotation(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 3 || utf8.RuneCountInString(in.Reason) > 1000 {
		writeErr(w, 400, "reason required (3 to 1000 characters)")
		return
	}
	var dsID string
	var from, to time.Time
	var cat, text string
	var retracted bool
	if err := s.pool.QueryRow(r.Context(), `SELECT a.datastream_id::text, a.ts_from, a.ts_to, a.category, a.text, EXISTS (SELECT 1 FROM core.annotation x WHERE x.retracts = a.id)
		FROM core.annotation a WHERE a.id = $1::uuid AND a.org_id = $2::uuid AND a.retracts IS NULL`, id, org).Scan(&dsID, &from, &to, &cat, &text, &retracted); err != nil {
		writeErr(w, 404, "annotation not found")
		return
	}
	if retracted {
		writeErr(w, 409, "annotation already retracted")
		return
	}
	var rid string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.annotation (org_id, datastream_id, ts_from, ts_to, category, text, retracts, created_by, created_by_name)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::uuid, NULLIF($8,'')::uuid, $9) RETURNING id::text`,
		org, dsID, from, to, cat, in.Reason, id, p.UserID, actorName(r)).Scan(&rid); err != nil {
		writeErr(w, 409, "annotation already retracted")
		return
	}
	s.audit(r, "annotation.retract", "annotation", id, map[string]any{"datastream_id": dsID, "category": cat, "text": text}, map[string]any{"retraction": rid}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "retraction_id": rid})
}

// actorName is the name written next to a record: the user, marked when an AI assistant acted for them.
func actorName(r *http.Request) string {
	p := PrincipalFrom(r.Context())
	if p == nil {
		return "anonymous"
	}
	switch {
	case p.GrantID != "":
		return p.Username + " (AI assistant " + p.ClientName + " via MCP)"
	case r.Context().Value(viaMCPKey) != nil:
		return p.Username + " via MCP"
	case p.TokenID != "":
		return p.Username + " (token " + p.TokenID[:8] + ")"
	}
	return p.Username
}
