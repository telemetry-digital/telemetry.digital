package httpapi

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"time"
)

// Full organization export (URS J-09: "plný export organizácie jedným kliknutím", draft Annex 11 12.9):
// one ZIP with the configuration as JSON, every record class as CSV for the requested period, and a manifest
// with SHA-256 of each file. Streamed; no temporary files. Requires system.admin.
//
//	GET /api/v1/export/organization?from=&to=   (default: last 365 days)

func (s *Server) exportOrganization(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	from, to, ok := timeRange(w, r, 365*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	stamp := time.Now().UTC().Format("20060102T150405Z")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ctrl32-organization-%s.zip"`, stamp))
	zw := zip.NewWriter(w)
	manifest := map[string]any{"exported_at": time.Now().UTC(), "exported_by": p.Username, "version": s.version, "period": map[string]any{"from": from, "to": to}, "files": map[string]any{}}
	files := manifest["files"].(map[string]any)
	addJSON := func(name, query string, args ...any) {
		rows, err := s.pool.Query(r.Context(), query, args...)
		if err != nil {
			files[name] = map[string]any{"error": "query failed"}
			return
		}
		defer rows.Close()
		var out []map[string]any
		cols := rows.FieldDescriptions()
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				break
			}
			rec := map[string]any{}
			for i, c := range cols {
				rec[string(c.Name)] = vals[i]
			}
			out = append(out, rec)
		}
		if out == nil {
			out = []map[string]any{}
		}
		fw, h := s.zipEntry(zw, name)
		enc := json.NewEncoder(io.MultiWriter(fw, h))
		enc.SetIndent("", " ")
		_ = enc.Encode(out)
		files[name] = map[string]any{"rows": len(out), "sha256": hex.EncodeToString(h.Sum(nil))}
	}
	addCSV := func(name string, columns []string, query string, args ...any) {
		rows, err := s.pool.Query(r.Context(), query, args...)
		if err != nil {
			files[name] = map[string]any{"error": "query failed"}
			return
		}
		defer rows.Close()
		fw, h := s.zipEntry(zw, name)
		cw := csv.NewWriter(io.MultiWriter(fw, h))
		_ = cw.Write(columns)
		n := 0
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				break
			}
			rec := make([]string, len(vals))
			for i, v := range vals {
				rec[i] = cell(v)
			}
			_ = cw.Write(rec)
			n++
		}
		cw.Flush()
		files[name] = map[string]any{"rows": n, "sha256": hex.EncodeToString(h.Sum(nil))}
	}

	addJSON("organization.json", `SELECT id::text, slug, name, locale, tz, profile, settings, branding - 'logo' AS branding, created_at FROM core.organization WHERE id = $1::uuid`, org)
	addJSON("sites.json", `SELECT id::text, customer_id::text, name, tz, address, attributes, created_at FROM core.site WHERE org_id = $1::uuid ORDER BY name`, org)
	addJSON("assets.json", `SELECT id::text, site_id::text, parent_id::text, type, name, attributes, retention_policy_id::text, created_at FROM core.asset WHERE org_id = $1::uuid ORDER BY name`, org)
	addJSON("datastreams.json", `SELECT id::text, asset_id::text, key, quantity, unit, kind, expected_interval::text, accuracy, attributes, created_at FROM core.datastream WHERE org_id = $1::uuid ORDER BY asset_id, key`, org)
	addJSON("devices.json", `SELECT id::text, external_id, name, transport, status, shared_attributes, created_at, revoked_at FROM core.device WHERE org_id = $1::uuid ORDER BY external_id`, org)
	addJSON("assignments.json", `SELECT id::text, kind, from_id::text, to_id::text, valid_from, valid_to, changed_by::text, reason FROM core.assignment WHERE org_id = $1::uuid ORDER BY valid_from`, org)
	addJSON("alarm_rules.json", `SELECT id::text, datastream_id::text, asset_id::text, device_id::text, type, warning, action, delay::text, hysteresis, enabled, version, valid_from, valid_to, changed_by::text, reason FROM core.alarm_rule WHERE org_id = $1::uuid ORDER BY valid_from`, org)
	addJSON("roles.json", `SELECT id::text, name, permissions, is_admin, is_qa, (org_id IS NULL) AS builtin FROM core.role WHERE org_id IS NULL OR org_id = $1::uuid ORDER BY org_id NULLS FIRST, name`, org)
	addJSON("users.json", `SELECT id::text, username, email, full_name, mfa_enabled, disabled_at, last_login_at, created_at FROM core.user_account WHERE org_id = $1::uuid ORDER BY username`, org)
	addJSON("user_roles.json", `SELECT h.ts, h.user_id::text, r.name AS role, h.event, h.actor_id::text, h.reason FROM core.user_role_history h JOIN core.role r ON r.id = h.role_id JOIN core.user_account u ON u.id = h.user_id WHERE u.org_id = $1::uuid ORDER BY h.ts`, org)
	addJSON("report_templates.json", `SELECT id::text, kind, name, version, locale, engine, body, checksum, created_by::text, created_at, valid_to, reason FROM core.report_template WHERE org_id = $1::uuid ORDER BY kind, locale, version`, org)
	addJSON("reports.json", `SELECT id::text, kind, template_id::text, template_ver, scope, data_hash, file_hash, bytes, generated_at, generated_by, audit_seq FROM core.report WHERE org_id = $1::uuid AND generated_at >= $2 AND generated_at < $3 ORDER BY generated_at`, org, from, to)
	addJSON("connectors.json", `SELECT id::text, kind, name, settings, created_at, last_seen_at, messages, revoked_at FROM core.connector WHERE org_id = $1::uuid ORDER BY name`, org)
	addJSON("device_commands.json", `SELECT id::text, device_id::text, method, args, expires_at, status, issued_by_name, issued_at, sent_at, acked_at, result, error, reason FROM core.device_command WHERE org_id = $1::uuid AND issued_at >= $2 AND issued_at < $3 ORDER BY issued_at`, org, from, to)

	addCSV("observations.csv", []string{"datastream_id", "measured_at", "received_at", "value", "value_json", "quality", "time_source", "decoder_version", "calibration_version", "series_version", "raw_id"},
		`SELECT datastream_id::text, measured_at, received_at, value::text, value_json::text, quality, time_source, decoder_version, calibration_version::text, series_version::text, raw_id::text
		 FROM core.observation WHERE org_id = $1::uuid AND measured_at >= $2 AND measured_at < $3 ORDER BY datastream_id, measured_at LIMIT 5000000`, org, from, to)
	addCSV("gaps.csv", []string{"id", "datastream_id", "gap_from", "gap_to", "detected_at", "closed_at", "closed_by"},
		`SELECT id::text, datastream_id::text, gap_from, gap_to, detected_at, closed_at, closed_by FROM core.gap WHERE org_id = $1::uuid AND gap_from >= $2 AND gap_from < $3 ORDER BY gap_from`, org, from, to)
	addCSV("alarms.csv", []string{"alarm_id", "raised_at", "cleared_at", "state", "severity", "retrospective", "value", "datastream_id", "rule_id", "event_ts", "event", "actor", "reason", "detail"},
		`SELECT a.id::text, a.raised_at, a.cleared_at, a.state, a.severity, a.retrospective::text, a.value::text, a.datastream_id::text, a.rule_id::text, e.ts, e.event, e.actor_name, e.reason, e.detail::text
		 FROM core.alarm a LEFT JOIN core.alarm_event e ON e.alarm_id = a.id WHERE a.org_id = $1::uuid AND a.raised_at >= $2 AND a.raised_at < $3 ORDER BY a.raised_at, e.ts`, org, from, to)
	addCSV("raw_messages.csv", []string{"id", "received_at", "device_id", "connector", "direction", "topic", "payload_text", "meta", "decode_status", "decode_error"},
		`SELECT id::text, received_at, device_id::text, connector, direction, topic, encode(payload,'escape'), meta::text, decode_status, decode_error FROM core.raw_message WHERE org_id = $1::uuid AND received_at >= $2 AND received_at < $3 ORDER BY received_at LIMIT 2000000`, org, from, to)
	addCSV("audit_trail.csv", []string{"seq", "ts", "actor_type", "actor_name", "action", "entity_type", "entity_id", "old_value", "new_value", "reason", "request_id", "prev_hash", "hash"},
		`SELECT seq::text, ts, actor_type, actor_name, action, entity_type, entity_id::text, old_value::text, new_value::text, reason, request_id, prev_hash, hash FROM core.audit_trail WHERE (org_id IS NULL OR org_id = $1::uuid) AND ts >= $2 AND ts < $3 ORDER BY seq`, org, from, to)
	addCSV("access_log.csv", []string{"ts", "username", "event", "success", "ip", "user_agent", "detail"},
		`SELECT ts, username, event, success::text, host(ip), user_agent, detail::text FROM core.access_log WHERE (org_id = $1::uuid OR org_id IS NULL) AND ts >= $2 AND ts < $3 ORDER BY ts`, org, from, to)

	var head struct {
		Seq  int64
		Hash string
	}
	_ = s.pool.QueryRow(r.Context(), `SELECT seq, hash FROM core.audit_trail ORDER BY seq DESC LIMIT 1`).Scan(&head.Seq, &head.Hash)
	manifest["audit_head"] = map[string]any{"seq": head.Seq, "hash": head.Hash}
	fw, _ := s.zipEntry(zw, "MANIFEST.json")
	enc := json.NewEncoder(fw)
	enc.SetIndent("", " ")
	_ = enc.Encode(manifest)
	_ = zw.Close()
	s.audit(r, "export.organization", "organization", org, nil, map[string]any{"files": len(files), "from": from, "to": to}, "")
}

func (s *Server) zipEntry(zw *zip.Writer, name string) (io.Writer, hash.Hash) {
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now().UTC()})
	if err != nil {
		return io.Discard, sha256.New()
	}
	return fw, sha256.New()
}
