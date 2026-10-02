package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
)

// Exports (URS J-09, draft Annex 11 12.9 "flat and locked files are not acceptable"):
// every record class can be exported as CSV or JSON Lines, streamed, with a time range and a hard row cap,
// or as a PDF report (format=pdf) rendered from the organization's template and stored as a record (ADR-0010).
//
//	GET /api/v1/export/observations?datastream_id=&from=&to=&format=csv|jsonl|pdf&limit=
//	GET /api/v1/export/alarms?from=&to=&format=
//	GET /api/v1/export/audit?from=&to=&format=          (audit.read)
//	GET /api/v1/export/access-log?from=&to=&format=     (audit.read)
//	GET /api/v1/export/messages?device_id=&from=&to=&format=   (data.export)

const exportMaxRows = 1_000_000

type exportSpec struct {
	name    string
	columns []string
	query   string
	args    []any

	// PDF report data (only used for format=pdf)
	kind   string         // template kind: observations|alarms|audit|generic
	title  string         //
	scope  map[string]any // printed in the title block and stored with the record
	from   time.Time
	to     time.Time
	labels map[string]string // column label overrides
	right  []string          // right-aligned columns
	// enrich may add a summary and assets (e.g. chart.svg) from the collected rows.
	enrich func(rows []map[string]any) (summary map[string]any, assets map[string][]byte, chart string)
}

func (s *Server) exportObservations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dsID := q.Get("datastream_id")
	if dsID == "" {
		writeErr(w, 400, "datastream_id required")
		return
	}
	from, to, ok := timeRange(w, r, 30*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	spec := exportSpec{
		name: "observations_" + safePrefix(dsID),
		// annotation (0.67.0): the comments of the annotations covering the observation, oldest first, " | " between
		columns: []string{"measured_at", "received_at", "value", "value_json", "quality", "time_source", "decoder_version", "calibration_version", "series_version", "raw_id", "annotation"},
		query: `SELECT o.measured_at, o.received_at, o.value::text, o.value_json::text, o.quality, o.time_source, o.decoder_version, o.calibration_version::text, o.series_version::text, o.raw_id::text,
		               (SELECT string_agg(a.text, ' | ' ORDER BY a.ts_from) FROM core.annotation a
		                WHERE a.datastream_id = o.datastream_id AND a.retracts IS NULL AND o.measured_at BETWEEN a.ts_from AND a.ts_to
		                  AND NOT EXISTS (SELECT 1 FROM core.annotation x WHERE x.retracts = a.id))
		        FROM core.observation o JOIN core.datastream ds ON ds.id = o.datastream_id
		        WHERE o.datastream_id = $1::uuid AND ($2 = '' OR ds.org_id = $2::uuid) AND o.measured_at >= $3 AND o.measured_at < $4
		        ORDER BY o.measured_at LIMIT $5`,
		args:  []any{dsID, orgOf(p), from, to, exportMaxRows},
		kind:  "observations",
		from:  from,
		to:    to,
		right: []string{"value"},
	}
	if q.Get("format") == "pdf" {
		// Datastream context and current limits for the title block, chart and summary.
		var key, quantity, unit, asset, kind string
		err := s.pool.QueryRow(r.Context(), `SELECT ds.key, ds.quantity, ds.unit, a.name, ds.kind FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id
			WHERE ds.id = $1::uuid AND ($2 = '' OR ds.org_id = $2::uuid)`, dsID, orgOf(p)).Scan(&key, &quantity, &unit, &asset, &kind)
		if err != nil {
			writeErr(w, 404, "datastream not found")
			return
		}
		var warning, action *float64
		_ = s.pool.QueryRow(r.Context(), `SELECT warning::float8, action::float8 FROM core.alarm_rule WHERE datastream_id = $1::uuid AND enabled AND valid_to IS NULL AND type IN ('high','low') ORDER BY valid_from DESC LIMIT 1`, dsID).Scan(&warning, &action)
		spec.title = fmt.Sprintf("Observations — %s / %s", asset, key)
		spec.scope = map[string]any{"asset": asset, "datastream": key, "quantity": quantity, "unit": unit, "datastream_id": dsID}
		spec.columns = []string{"measured_at", "received_at", "value", "quality", "time_source", "decoder_version", "annotation"}
		spec.query = `SELECT o.measured_at, o.received_at, o.value::text, o.quality, o.time_source, o.decoder_version,
		               (SELECT string_agg(a.text, ' | ' ORDER BY a.ts_from) FROM core.annotation a
		                WHERE a.datastream_id = o.datastream_id AND a.retracts IS NULL AND o.measured_at BETWEEN a.ts_from AND a.ts_to
		                  AND NOT EXISTS (SELECT 1 FROM core.annotation x WHERE x.retracts = a.id))
		        FROM core.observation o JOIN core.datastream ds ON ds.id = o.datastream_id
		        WHERE o.datastream_id = $1::uuid AND ($2 = '' OR ds.org_id = $2::uuid) AND o.measured_at >= $3 AND o.measured_at < $4
		        ORDER BY o.measured_at LIMIT $5`
		spec.args = []any{dsID, orgOf(p), from, to, report.MaxRows + 1}
		spec.enrich = func(rows []map[string]any) (map[string]any, map[string][]byte, string) {
			pts := make([]report.Point, 0, len(rows))
			var mn, mx, sum float64 = math.Inf(1), math.Inf(-1), 0
			n, flagged := 0, 0
			for _, row := range rows {
				v, err := strconv.ParseFloat(fmt.Sprint(row["value"]), 64)
				if err != nil {
					continue
				}
				t, _ := time.Parse("2006-01-02 15:04:05Z", fmt.Sprint(row["measured_at"]))
				q := fmt.Sprint(row["quality"])
				pts = append(pts, report.Point{T: t, V: v, Q: q})
				mn, mx, sum, n = math.Min(mn, v), math.Max(mx, v), sum+v, n+1
				if q != "ok" && q != "" {
					flagged++
				}
			}
			summary := map[string]any{"count": len(rows), "flagged": flagged}
			annotated := 0
			for _, row := range rows {
				if a, ok := row["annotation"].(string); ok && a != "" {
					annotated++
				}
			}
			if annotated > 0 {
				summary["annotated"] = annotated
			}
			if n > 0 {
				summary["min"] = fmtF(mn)
				summary["max"] = fmtF(mx)
				summary["avg"] = fmtF(sum / float64(n))
			}
			if warning != nil {
				summary["warning"] = fmtF(*warning)
			}
			if action != nil {
				summary["action"] = fmtF(*action)
			}
			if kind != "gauge" && kind != "counter" {
				return summary, nil, ""
			}
			return summary, map[string][]byte{"chart.svg": []byte(report.ChartSVG(pts, unit, warning, action))}, "chart.svg"
		}
	}
	s.streamExport(w, r, spec)
}

func (s *Server) exportAlarms(w http.ResponseWriter, r *http.Request) {
	from, to, ok := timeRange(w, r, 365*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	s.streamExport(w, r, exportSpec{
		name:    "alarms",
		columns: []string{"alarm_id", "raised_at", "cleared_at", "state", "severity", "retrospective", "value", "datastream_id", "rule_id", "event_ts", "event", "actor", "reason", "detail"},
		query: `SELECT a.id::text, a.raised_at, a.cleared_at, a.state, a.severity, a.retrospective::text, a.value::text, a.datastream_id::text, a.rule_id::text,
		               e.ts, e.event, e.actor_name, e.reason, e.detail::text
		        FROM core.alarm a LEFT JOIN core.alarm_event e ON e.alarm_id = a.id
		        WHERE ($1 = '' OR a.org_id = $1::uuid) AND a.raised_at >= $2 AND a.raised_at < $3
		        ORDER BY a.raised_at, e.ts LIMIT $4`,
		args:  []any{orgOf(p), from, to, exportMaxRows},
		kind:  "alarms",
		title: "Alarm log",
		from:  from, to: to,
		right: []string{"value"},
	})
}

func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	from, to, ok := timeRange(w, r, 365*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	// Names instead of ids where possible (draft Annex 11 12.9: intelligible form); hashes included for verification.
	s.streamExport(w, r, exportSpec{
		name:    "audit_trail",
		columns: []string{"seq", "ts", "actor_type", "actor_name", "action", "entity_type", "entity_id", "old_value", "new_value", "reason", "request_id", "prev_hash", "hash"},
		query: `SELECT seq::text, ts, actor_type, actor_name, action, entity_type, entity_id::text, old_value::text, new_value::text, reason, request_id, prev_hash, hash
		        FROM core.audit_trail WHERE (org_id IS NULL OR $1 = '' OR org_id = $1::uuid) AND ts >= $2 AND ts < $3 ORDER BY seq LIMIT $4`,
		args:  []any{orgOf(p), from, to, exportMaxRows},
		kind:  "audit",
		title: "Audit trail",
		from:  from, to: to,
		right: []string{"seq"},
	})
}

func (s *Server) exportAccessLog(w http.ResponseWriter, r *http.Request) {
	from, to, ok := timeRange(w, r, 365*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	s.streamExport(w, r, exportSpec{
		name:    "access_log",
		columns: []string{"ts", "username", "event", "success", "ip", "user_agent", "detail"},
		query: `SELECT ts, username, event, success::text, host(ip), user_agent, detail::text FROM core.access_log
		        WHERE ($1 = '' OR org_id = $1::uuid OR org_id IS NULL) AND ts >= $2 AND ts < $3 ORDER BY ts LIMIT $4`,
		args:  []any{orgOf(p), from, to, exportMaxRows},
		kind:  "generic",
		title: "Access log",
		from:  from, to: to,
	})
}

func (s *Server) exportMessages(w http.ResponseWriter, r *http.Request) {
	devID := r.URL.Query().Get("device_id")
	if devID == "" {
		writeErr(w, 400, "device_id required")
		return
	}
	from, to, ok := timeRange(w, r, 7*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	s.streamExport(w, r, exportSpec{
		name:    "messages_" + safePrefix(devID),
		columns: []string{"received_at", "connector", "direction", "topic", "payload_text", "payload_json", "meta", "decode_status", "decode_error"},
		query: `SELECT m.received_at, m.connector, m.direction, m.topic, encode(m.payload,'escape'), m.payload_json::text, m.meta::text, m.decode_status, m.decode_error
		        FROM core.raw_message m JOIN core.device d ON d.id = m.device_id
		        WHERE m.device_id = $1::uuid AND ($2 = '' OR d.org_id = $2::uuid) AND m.received_at >= $3 AND m.received_at < $4
		        ORDER BY m.received_at LIMIT $5`,
		args:  []any{devID, orgOf(p), from, to, exportMaxRows},
		kind:  "generic",
		title: "Device messages",
		scope: map[string]any{"device_id": devID},
		from:  from, to: to,
	})
}

// streamExport runs the query and streams rows as CSV (default) or JSON Lines, or renders a PDF report, with an audit entry.
func (s *Server) streamExport(w http.ResponseWriter, r *http.Request, spec exportSpec) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format == "pdf" {
		s.exportPDF(w, r, spec)
		return
	}
	rows, err := s.pool.Query(r.Context(), spec.query, spec.args...)
	if err != nil {
		s.log.Error("export query failed", "export", spec.name, "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()

	stamp := time.Now().UTC().Format("20060102T150405Z")
	var count int64
	switch format {
	case "jsonl":
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.jsonl"`, spec.name, stamp))
		enc := json.NewEncoder(w)
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return
			}
			rec := make(map[string]any, len(spec.columns))
			for i, c := range spec.columns {
				rec[c] = vals[i]
			}
			if err := enc.Encode(rec); err != nil {
				return
			}
			count++
		}
	default:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.csv"`, spec.name, stamp))
		cw := csv.NewWriter(w)
		_ = cw.Write(spec.columns)
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return
			}
			rec := make([]string, len(vals))
			for i, v := range vals {
				rec[i] = cell(v)
			}
			if err := cw.Write(rec); err != nil {
				return
			}
			count++
		}
		cw.Flush()
	}
	if p := PrincipalFrom(r.Context()); p != nil {
		s.audit(r, "export.read", "export", "", nil, map[string]any{"export": spec.name, "format": format, "rows": count, "query": r.URL.RawQuery}, "")
	}
}

// exportPDF collects up to report.MaxRows+1 rows, renders the organization's template and stores the PDF as a record.
func (s *Server) exportPDF(w http.ResponseWriter, r *http.Request, spec exportSpec) {
	if s.reportsUnavailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeErr(w, 401, "authentication required")
		return
	}
	args := spec.args
	if n := len(args); n > 0 {
		if _, isInt := args[n-1].(int); isInt {
			args = append(append([]any{}, args[:n-1]...), report.MaxRows+1)
		}
	}
	rows, err := s.pool.Query(r.Context(), spec.query, args...)
	if err != nil {
		s.log.Error("export query failed", "export", spec.name, "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	data := []map[string]any{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			rows.Close()
			writeErr(w, 500, "scan failed")
			return
		}
		rec := make(map[string]any, len(spec.columns))
		for i, c := range spec.columns {
			rec[c] = pdfCell(vals[i])
		}
		data = append(data, rec)
	}
	rows.Close()

	cols := make([]report.Column, 0, len(spec.columns))
	for _, c := range spec.columns {
		label := spec.labels[c]
		if label == "" {
			label = strings.ReplaceAll(c, "_", " ")
		}
		align := "left"
		for _, rc := range spec.right {
			if rc == c {
				align = "right"
			}
		}
		cols = append(cols, report.Column{Key: c, Label: label, Align: align})
	}
	d := report.Data{
		Kind: spec.kind, Title: spec.title, Subtitle: "Export of records — " + spec.name, Org: s.orgName(r, p.OrgID), Locale: localeParam(r, "en"),
		Period:  report.Period{From: spec.from.UTC().Format(time.RFC3339), To: spec.to.UTC().Format(time.RFC3339)},
		Scope:   spec.scope,
		Columns: cols, Rows: data, Summary: map[string]any{"count": len(data)},
		System: map[string]any{"version": s.version, "instance": s.cfg.HTTP.BaseURL},
	}
	if d.Scope == nil {
		d.Scope = map[string]any{}
	}
	d.Scope["from"], d.Scope["to"] = d.Period.From, d.Period.To
	assets := map[string][]byte{}
	if spec.enrich != nil {
		summary, extra, chart := spec.enrich(data)
		if summary != nil {
			d.Summary = summary
		}
		d.Chart = chart
		for k, v := range extra {
			assets[k] = v
		}
	}
	if logo, name := s.orgLogo(r, p.OrgID); len(logo) > 0 {
		assets[name] = logo
		d.Logo = name
	}
	res, err := s.renderReport(r, p, d, assets)
	if err != nil {
		writeErr(w, 500, "report rendering failed")
		return
	}
	servePDF(w, res.PDF, fmt.Sprintf("%s_%s.pdf", spec.name, res.Record.GeneratedAt.UTC().Format("20060102T150405Z")), res.Record.ID)
}

// renderReport anchors the audit head, renders, stores and audits.
func (s *Server) renderReport(r *http.Request, p *iam.Principal, d report.Data, assets map[string][]byte) (*report.Result, error) {
	_ = s.pool.QueryRow(r.Context(), `SELECT seq, hash FROM core.audit_trail ORDER BY seq DESC LIMIT 1`).Scan(&d.Audit.Seq, &d.Audit.Hash)
	res, err := s.rep.Render(r.Context(), p.OrgID, d, assets, p.Username)
	if err != nil {
		s.log.Error("report render failed", "kind", d.Kind, "err", err)
		return nil, err
	}
	s.audit(r, "export.read", "report", res.Record.ID, nil, map[string]any{"export": d.Kind, "format": "pdf", "rows": len(d.Rows), "file_hash": res.Record.FileHash, "data_hash": res.Record.DataHash, "template_version": res.Template.Version, "query": r.URL.RawQuery}, "")
	return res, nil
}

func (s *Server) orgName(r *http.Request, orgID string) string {
	if orgID == "" {
		return "ctrl32 telemetry"
	}
	var name string
	if err := s.pool.QueryRow(r.Context(), `SELECT name FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&name); err != nil {
		return "ctrl32 telemetry"
	}
	return name
}

func pdfCell(v any) any {
	switch t := v.(type) {
	case nil:
		return ""
	case time.Time:
		return t.UTC().Format("2006-01-02 15:04:05Z")
	default:
		return cell(v)
	}
}

func fmtF(v float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 3, 64), "0"), ".")
}

func safePrefix(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func cell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(t)
	case pgx.Rows:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func timeRange(w http.ResponseWriter, r *http.Request, defaultSpan time.Duration) (time.Time, time.Time, bool) {
	q := r.URL.Query()
	to := time.Now().UTC()
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, 400, "to must be RFC3339")
			return to, to, false
		}
		to = t
	}
	from := to.Add(-defaultSpan)
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, 400, "from must be RFC3339")
			return to, to, false
		}
		from = t
	}
	if v := q.Get("limit"); v != "" {
		if _, err := strconv.Atoi(v); err != nil {
			writeErr(w, 400, "limit must be an integer")
			return to, to, false
		}
	}
	return from, to, true
}

// orgOf returns the organization scope of a principal ("" = unscoped, i.e. anonymous public_read).
func orgOf(p *iam.Principal) string {
	if p == nil {
		return ""
	}
	return p.OrgID
}
