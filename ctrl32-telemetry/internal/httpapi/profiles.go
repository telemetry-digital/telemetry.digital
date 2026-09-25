package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
)

// Versioned register profiles (URS A-11, 0.29.0), all config.write:
//
//	GET    /api/v1/register-profiles            current versions with connector counts
//	POST   /api/v1/register-profiles            {name, description, spec:{byte_order, registers}, reason} → 201
//	GET    /api/v1/register-profiles/{id}       one version with the history of its lineage
//	PUT    /api/v1/register-profiles/{id}       {description?, spec, changelog, reason} → new version (201)
//	DELETE /api/v1/register-profiles/{id}?reason=  retire (refused while connectors use the profile)
//	POST   /api/v1/register-profiles/{id}/apply {reason}  move every connector on an older version to this one
//
// A Modbus connector whose settings carry profile_id gets its registers and byte order from that profile version
// (snapshot in the settings, so the poller is unchanged); profile_name / profile_version stay for traceability.

type profileSpec struct {
	ByteOrder string            `json:"byte_order,omitempty"`
	Registers []modbus.Register `json:"registers"`
}

func validateProfileSpec(raw json.RawMessage) (profileSpec, error) {
	var sp profileSpec
	if len(raw) == 0 {
		return sp, errors.New("spec required")
	}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return sp, errors.New("spec: invalid JSON")
	}
	switch sp.ByteOrder {
	case "", "big", "big_swap", "little", "little_swap":
	default:
		return sp, errors.New("byte order must be big, big_swap, little or little_swap")
	}
	regs, err := modbus.ValidateRegisters(sp.Registers)
	if err != nil {
		return sp, err
	}
	sp.Registers = regs
	return sp, nil
}

func writableCount(regs []modbus.Register) int {
	n := 0
	for _, r := range regs {
		if r.Writable {
			n++
		}
	}
	return n
}

// resolveProfile fills registers and byte order of Modbus connector settings from settings.profile_id.
func (s *Server) resolveProfile(ctx context.Context, org string, raw json.RawMessage) (json.RawMessage, error) {
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return raw, nil
	}
	pid, _ := m["profile_id"].(string)
	if pid == "" {
		delete(m, "profile_id")
		delete(m, "profile_name")
		delete(m, "profile_version")
		out, _ := json.Marshal(m)
		return out, nil
	}
	var name, spec string
	var version int
	var retired *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT name, version, spec::text, retired_at FROM core.register_profile WHERE id = $1::uuid AND org_id = $2::uuid AND kind = 'modbus'`, pid, org).Scan(&name, &version, &spec, &retired); err != nil {
		return nil, errors.New("register profile not found")
	}
	if retired != nil {
		return nil, errors.New("register profile is retired")
	}
	var sp profileSpec
	_ = json.Unmarshal([]byte(spec), &sp)
	m["registers"] = sp.Registers
	if sp.ByteOrder != "" {
		m["byte_order"] = sp.ByteOrder
	}
	m["profile_name"] = name
	m["profile_version"] = version
	out, _ := json.Marshal(m)
	return out, nil
}

const profileConnectorsSQL = `(SELECT count(*) FROM core.connector c WHERE c.org_id = p.org_id AND c.revoked_at IS NULL AND c.kind = 'modbus_tcp'
	AND (c.settings->>'profile_id') IN (SELECT q.id::text FROM core.register_profile q WHERE q.lineage = p.lineage %s))`

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT p.id::text, p.lineage::text, p.name, p.description, p.version, p.spec, p.changelog, p.created_by_name, p.reason, p.valid_from, p.retired_at,
		`+strings.Replace(profileConnectorsSQL, "%s", "", 1)+`, `+strings.Replace(profileConnectorsSQL, "%s", "AND q.id <> p.id", 1)+`
		FROM core.register_profile p WHERE p.org_id = $1::uuid AND p.valid_to IS NULL ORDER BY p.retired_at NULLS FIRST, p.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, lineage, name, desc, changelog, by, reason string
		var version int
		var spec json.RawMessage
		var from time.Time
		var retired *time.Time
		var conns, outdated int
		if rows.Scan(&id, &lineage, &name, &desc, &version, &spec, &changelog, &by, &reason, &from, &retired, &conns, &outdated) != nil {
			continue
		}
		var sp profileSpec
		_ = json.Unmarshal(spec, &sp)
		out = append(out, map[string]any{"id": id, "lineage": lineage, "name": name, "description": desc, "version": version, "spec": sp, "registers": len(sp.Registers), "writable": writableCount(sp.Registers),
			"changelog": changelog, "created_by": by, "reason": reason, "valid_from": from, "retired_at": retired, "connectors": conns, "outdated": outdated})
	}
	writeJSON(w, 200, out)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var lineage, name, desc, changelog, by, reason string
	var version int
	var spec json.RawMessage
	var from time.Time
	var to, retired *time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT lineage::text, name, description, version, spec, changelog, created_by_name, reason, valid_from, valid_to, retired_at FROM core.register_profile WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).
		Scan(&lineage, &name, &desc, &version, &spec, &changelog, &by, &reason, &from, &to, &retired); err != nil {
		writeErr(w, 404, "register profile not found")
		return
	}
	var sp profileSpec
	_ = json.Unmarshal(spec, &sp)
	rows, err := s.pool.Query(r.Context(), `SELECT p.id::text, p.version, p.changelog, p.created_by_name, p.reason, p.valid_from, p.valid_to,
		(SELECT count(*) FROM core.connector c WHERE c.org_id = p.org_id AND c.revoked_at IS NULL AND c.kind = 'modbus_tcp' AND c.settings->>'profile_id' = p.id::text)
		FROM core.register_profile p WHERE p.lineage = $1::uuid ORDER BY p.version DESC`, lineage)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	versions := []map[string]any{}
	for rows.Next() {
		var vid, vlog, vby, vreason string
		var v, conns int
		var vfrom time.Time
		var vto *time.Time
		if rows.Scan(&vid, &v, &vlog, &vby, &vreason, &vfrom, &vto, &conns) == nil {
			versions = append(versions, map[string]any{"id": vid, "version": v, "changelog": vlog, "created_by": vby, "reason": vreason, "valid_from": vfrom, "valid_to": vto, "connectors": conns})
		}
	}
	writeJSON(w, 200, map[string]any{"id": id, "lineage": lineage, "name": name, "description": desc, "version": version, "spec": sp, "changelog": changelog, "created_by": by, "reason": reason, "valid_from": from, "valid_to": to, "retired_at": retired, "versions": versions})
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Spec        json.RawMessage `json:"spec"`
		Reason      string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		writeErr(w, 400, "name required (max 64)")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	sp, err := validateProfileSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	spec, _ := json.Marshal(sp)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.register_profile (org_id, name, description, spec, created_by_name, reason) VALUES ($1::uuid, $2, $3, $4::jsonb, $5, $6) RETURNING id::text`,
		org, in.Name, in.Description, string(spec), p.Username, in.Reason).Scan(&id); err != nil {
		writeErr(w, 409, "profile could not be created (duplicate name?)")
		return
	}
	s.audit(r, "register_profile.create", "register_profile", id, nil, map[string]any{"name": in.Name, "registers": len(sp.Registers), "writable": writableCount(sp.Registers), "byte_order": sp.ByteOrder}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "version": 1, "registers": len(sp.Registers)})
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Description *string         `json:"description"`
		Spec        json.RawMessage `json:"spec"`
		Changelog   string          `json:"changelog"`
		Reason      string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	sp, err := validateProfileSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var lineage, name, desc string
	var version int
	var oldSpec json.RawMessage
	var to, retired *time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT lineage::text, name, description, version, spec, valid_to, retired_at FROM core.register_profile WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&lineage, &name, &desc, &version, &oldSpec, &to, &retired); err != nil {
		writeErr(w, 404, "register profile not found")
		return
	}
	if to != nil || retired != nil {
		writeErr(w, 409, "only the current version of an active profile can be changed")
		return
	}
	if in.Description != nil {
		desc = *in.Description
	}
	spec, _ := json.Marshal(sp)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "transaction failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE core.register_profile SET valid_to = now() WHERE id = $1::uuid`, id); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	var newID string
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.register_profile (org_id, lineage, name, description, version, spec, changelog, created_by_name, reason) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::jsonb, $7, $8, $9) RETURNING id::text`,
		org, lineage, name, desc, version+1, string(spec), strings.TrimSpace(in.Changelog), p.Username, in.Reason).Scan(&newID); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	var old profileSpec
	_ = json.Unmarshal(oldSpec, &old)
	s.audit(r, "register_profile.update", "register_profile", newID, map[string]any{"id": id, "version": version, "registers": len(old.Registers)}, map[string]any{"version": version + 1, "registers": len(sp.Registers), "writable": writableCount(sp.Registers), "changelog": in.Changelog}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": newID, "previous_id": id, "name": name, "version": version + 1, "registers": len(sp.Registers)})
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var lineage, name string
	var to, retired *time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT lineage::text, name, valid_to, retired_at FROM core.register_profile WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&lineage, &name, &to, &retired); err != nil {
		writeErr(w, 404, "register profile not found")
		return
	}
	if to != nil || retired != nil {
		writeErr(w, 409, "not the current version of an active profile")
		return
	}
	var used int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.connector c WHERE c.org_id = $1::uuid AND c.revoked_at IS NULL AND c.kind = 'modbus_tcp' AND (c.settings->>'profile_id') IN (SELECT q.id::text FROM core.register_profile q WHERE q.lineage = $2::uuid)`, org, lineage).Scan(&used)
	if used > 0 {
		writeErr(w, 409, "profile is used by connectors; change them first")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.register_profile SET retired_at = now() WHERE id = $1::uuid`, id); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "register_profile.retire", "register_profile", id, map[string]any{"name": name}, nil, reason)
	writeJSON(w, 200, map[string]any{"id": id, "retired": true})
}

func (s *Server) applyProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var lineage, name string
	var version int
	var to, retired *time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT lineage::text, name, version, valid_to, retired_at FROM core.register_profile WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&lineage, &name, &version, &to, &retired); err != nil {
		writeErr(w, 404, "register profile not found")
		return
	}
	if to != nil || retired != nil {
		writeErr(w, 409, "only the current version of an active profile can be applied")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.name, c.settings::text FROM core.connector c WHERE c.org_id = $1::uuid AND c.revoked_at IS NULL AND c.kind = 'modbus_tcp'
		AND (c.settings->>'profile_id') IN (SELECT q.id::text FROM core.register_profile q WHERE q.lineage = $2::uuid AND q.id <> $3::uuid)`, org, lineage, id)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	type conn struct{ id, name, settings string }
	var conns []conn
	for rows.Next() {
		var c conn
		if rows.Scan(&c.id, &c.name, &c.settings) == nil {
			conns = append(conns, c)
		}
	}
	rows.Close()
	applied := []map[string]any{}
	for _, c := range conns {
		var m map[string]any
		_ = json.Unmarshal([]byte(c.settings), &m)
		m["profile_id"] = id
		raw, _ := json.Marshal(m)
		resolved, err := s.resolveProfile(r.Context(), org, raw)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		st, err := modbus.ValidateSettings(resolved)
		if err != nil {
			writeErr(w, 400, "connector "+c.name+": "+err.Error())
			return
		}
		settings, _ := json.Marshal(st)
		if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET settings = $3::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, c.id, org, string(settings)); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
		var old modbus.Settings
		_ = json.Unmarshal([]byte(c.settings), &old)
		s.audit(r, "connector.update", "connector", c.id, map[string]any{"profile": old.ProfileName, "profile_version": old.ProfileVersion, "registers": len(old.Registers)}, map[string]any{"profile": name, "profile_version": version, "registers": len(st.Registers)}, "profile "+name+" v"+itoa(version)+" applied: "+in.Reason)
		applied = append(applied, map[string]any{"id": c.id, "name": c.name})
	}
	if len(applied) > 0 && s.mb != nil {
		s.mb.Resync()
	}
	s.audit(r, "register_profile.apply", "register_profile", id, nil, map[string]any{"name": name, "version": version, "connectors": len(applied)}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "version": version, "applied": len(applied), "connectors": applied})
}
