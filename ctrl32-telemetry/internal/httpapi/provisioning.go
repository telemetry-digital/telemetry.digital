package httpapi

// Provisioning profiles (spec 005 US1): devices register themselves with a key and a secret over MQTT or HTTP; the
// organization decides the strategy (create new devices or only pre-registered ones), approval, placement and
// automatic datastreams. Devices waiting for approval cannot connect until a user approves them.

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/provision"
)

type provProfileIn struct {
	Name            *string `json:"name"`
	Strategy        *string `json:"strategy"`
	Approval        *bool   `json:"approval"`
	SiteID          *string `json:"site_id"`
	RoomID          *string `json:"room_id"`
	AutoDatastreams *bool   `json:"auto_datastreams"`
	NamePattern     *string `json:"name_pattern"`
	Transport       *string `json:"transport"`
	MaxDevices      *int    `json:"max_devices"`
	Reason          string  `json:"reason"`
}

const provProfileCols = `pp.id::text, pp.name, pp.key, pp.strategy, pp.approval, pp.site_id::text, pp.room_id::text, pp.auto_datastreams, pp.name_pattern,
	pp.transport, pp.max_devices, pp.created_at, (SELECT count(*) FROM core.device d WHERE d.provision_profile_id = pp.id AND d.revoked_at IS NULL)`

func scanProvProfile(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, name, key, strategy, pattern, transport string
	var site, room *string
	var approval, auto bool
	var max, devices int
	var created time.Time
	if err := row.Scan(&id, &name, &key, &strategy, &approval, &site, &room, &auto, &pattern, &transport, &max, &created, &devices); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "name": name, "key": key, "strategy": strategy, "approval": approval, "site_id": site, "room_id": room,
		"auto_datastreams": auto, "name_pattern": pattern, "transport": transport, "max_devices": max, "created_at": created, "devices": devices}, nil
}

func (s *Server) listProvProfiles(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+provProfileCols+` FROM core.provision_profile pp WHERE pp.org_id = $1::uuid AND pp.revoked_at IS NULL ORDER BY pp.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if m, err := scanProvProfile(rows); err == nil {
			out = append(out, m)
		}
	}
	writeJSON(w, 200, out)
}

// validateProvProfile checks the settings against the organization; it returns an error message or "".
func (s *Server) validateProvProfile(r *http.Request, org string, in *provProfileIn) string {
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 128) {
		return "name required (max 128 characters)"
	}
	if in.Strategy != nil && *in.Strategy != "create" && *in.Strategy != "preprovisioned" {
		return "strategy must be create or preprovisioned"
	}
	if in.Transport != nil && *in.Transport != "mqtt" && *in.Transport != "http" {
		return "transport must be mqtt or http"
	}
	if in.MaxDevices != nil && (*in.MaxDevices < 0 || *in.MaxDevices > 1000000) {
		return "max_devices out of range"
	}
	if in.NamePattern != nil && *in.NamePattern != "" {
		if len(*in.NamePattern) > 200 {
			return "name_pattern too long"
		}
		if _, err := regexp.Compile(*in.NamePattern); err != nil {
			return "name_pattern is not a valid regular expression"
		}
	}
	if in.SiteID != nil && *in.SiteID != "" {
		var n int
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, *in.SiteID, org).Scan(&n)
		if n == 0 {
			return "site not found"
		}
	}
	if in.RoomID != nil && *in.RoomID != "" {
		var n int
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, *in.RoomID, org).Scan(&n)
		if n == 0 {
			return "room not found"
		}
	}
	return ""
}

func nullID(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

func (s *Server) createProvProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in provProfileIn
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Name == nil {
		writeErr(w, 400, "name required")
		return
	}
	if msg := s.validateProvProfile(r, org, &in); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	def := func(p *string, d string) string {
		if p == nil || *p == "" {
			return d
		}
		return *p
	}
	approval, auto := false, true
	if in.Approval != nil {
		approval = *in.Approval
	}
	if in.AutoDatastreams != nil {
		auto = *in.AutoDatastreams
	}
	max := 0
	if in.MaxDevices != nil {
		max = *in.MaxDevices
	}
	key, secret, err := provision.NewKey()
	if err != nil {
		writeErr(w, 500, "key generation failed")
		return
	}
	var id string
	err = s.pool.QueryRow(r.Context(), `INSERT INTO core.provision_profile (org_id, name, key, secret_hash, strategy, approval, site_id, room_id, auto_datastreams, name_pattern, transport, max_devices)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, $8::uuid, $9, $10, $11, $12) RETURNING id::text`,
		org, strings.TrimSpace(*in.Name), key, provision.Hash(secret), def(in.Strategy, "create"), approval, nullID(in.SiteID), nullID(in.RoomID), auto,
		def(in.NamePattern, ""), def(in.Transport, "mqtt"), max).Scan(&id)
	if err != nil {
		writeErr(w, 409, "profile could not be created (name already used?)")
		return
	}
	m, _ := scanProvProfile(s.pool.QueryRow(r.Context(), `SELECT `+provProfileCols+` FROM core.provision_profile pp WHERE pp.id = $1::uuid`, id))
	s.audit(r, "provision_profile.create", "provision_profile", id, nil, m, in.Reason)
	m["secret"] = secret
	writeJSON(w, 201, m)
}

func (s *Server) updateProvProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in provProfileIn
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if msg := s.validateProvProfile(r, org, &in); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	old, err := scanProvProfile(s.pool.QueryRow(r.Context(), `SELECT `+provProfileCols+` FROM core.provision_profile pp WHERE pp.id = $1::uuid AND pp.org_id = $2::uuid AND pp.revoked_at IS NULL`, id, org))
	if err != nil {
		writeErr(w, 404, "profile not found")
		return
	}
	site, room := in.SiteID, in.RoomID
	_, err = s.pool.Exec(r.Context(), `UPDATE core.provision_profile SET
		name = coalesce($2, name), strategy = coalesce($3, strategy), approval = coalesce($4, approval),
		site_id = CASE WHEN $5::boolean THEN $6::uuid ELSE site_id END, room_id = CASE WHEN $7::boolean THEN $8::uuid ELSE room_id END,
		auto_datastreams = coalesce($9, auto_datastreams), name_pattern = coalesce($10, name_pattern), transport = coalesce($11, transport),
		max_devices = coalesce($12, max_devices) WHERE id = $1::uuid`,
		id, in.Name, in.Strategy, in.Approval, site != nil, nullID(site), room != nil, nullID(room), in.AutoDatastreams, in.NamePattern, in.Transport, in.MaxDevices)
	if err != nil {
		writeErr(w, 409, "profile could not be updated (name already used?)")
		return
	}
	m, _ := scanProvProfile(s.pool.QueryRow(r.Context(), `SELECT `+provProfileCols+` FROM core.provision_profile pp WHERE pp.id = $1::uuid`, id))
	s.audit(r, "provision_profile.update", "provision_profile", id, old, m, in.Reason)
	writeJSON(w, 200, m)
}

func (s *Server) rotateProvProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil || !reasonOf(w, in.Reason) {
		if err != nil {
			writeErr(w, 400, err.Error())
		}
		return
	}
	secret, err := func() (string, error) { _, s, err := provision.NewKey(); return s, err }()
	if err != nil {
		writeErr(w, 500, "secret generation failed")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.provision_profile SET secret_hash = $3 WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, r.PathValue("id"), org, provision.Hash(secret))
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "profile not found")
		return
	}
	s.audit(r, "provision_profile.rotate", "provision_profile", r.PathValue("id"), nil, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "secret": secret})
}

func (s *Server) revokeProvProfile(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.provision_profile SET revoked_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, r.PathValue("id"), org)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "profile not found")
		return
	}
	s.audit(r, "provision_profile.revoke", "provision_profile", r.PathValue("id"), nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- approvals ---------------------------------------------------------------------------------------------------

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT d.id::text, d.external_id, d.name, d.transport, d.created_at, coalesce(pp.name, ''), d.status
		FROM core.device d LEFT JOIN core.provision_profile pp ON pp.id = d.provision_profile_id
		WHERE d.org_id = $1::uuid AND d.approval = 'pending' AND d.revoked_at IS NULL ORDER BY d.created_at`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, ext, name, transport, profile string
		var created time.Time
		var status json.RawMessage
		if rows.Scan(&id, &ext, &name, &transport, &created, &profile, &status) == nil {
			out = append(out, map[string]any{"id": id, "external_id": ext, "name": name, "transport": transport, "created_at": created, "profile": profile, "status": status})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) decideDevice(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := s.mustOrg(w, r)
		if !ok {
			return
		}
		var in struct{ Reason string }
		if err := decodeJSON(r, &in, 2048); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if !reasonOf(w, in.Reason) {
			return
		}
		id := r.PathValue("id")
		state := "approved"
		if !approve {
			state = "rejected"
		}
		tag, err := s.pool.Exec(r.Context(), `UPDATE core.device SET approval = $3 WHERE id = $1::uuid AND org_id = $2::uuid AND approval = 'pending' AND revoked_at IS NULL`, id, org, state)
		if err != nil || tag.RowsAffected() == 0 {
			writeErr(w, 404, "no device waiting for approval")
			return
		}
		if !approve {
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.device SET revoked_at = now() WHERE id = $1::uuid`, id)
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_credential SET revoked_at = now() WHERE device_id = $1::uuid AND revoked_at IS NULL`, id)
			if s.br != nil {
				s.br.DisconnectDevice(id)
			}
		}
		s.audit(r, "device."+map[bool]string{true: "approve", false: "reject"}[approve], "device", id, map[string]any{"approval": "pending"}, map[string]any{"approval": state}, in.Reason)
		writeJSON(w, 200, map[string]any{"id": id, "approval": state})
	}
}

// provisionByKey is the ThingsBoard-compatible HTTP provisioning (body with provisionDeviceKey).
func (s *Server) provisionByKey(w http.ResponseWriter, r *http.Request, body []byte) {
	var req provision.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, 400, "request is not JSON")
		return
	}
	resp := s.prov.Provision(r.Context(), req, clientIP(r), "http")
	code := 200
	if resp.Status != "SUCCESS" {
		code = 400
	}
	writeJSON(w, code, resp)
}

// readProvisionBody reads the provisioning request once, so both variants can inspect it.
func readProvisionBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeErr(w, 400, "request too large")
		return nil, false
	}
	return body, true
}
