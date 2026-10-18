package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Device commands (RPC with acknowledgement, protocol §3.4) and shared attributes (protocol §3.2).
//
//	GET  /api/v1/devices/{id}/commands?limit=       command log, newest first                (data.read)
//	POST /api/v1/devices/{id}/commands              {method, args, ttl_seconds, reason}      (device.command)
//	GET  /api/v1/devices/{id}/attributes            {shared, client}                         (data.read)
//	PUT  /api/v1/devices/{id}/attributes            {attributes:{...}, remove:[...], reason} (device.manage)

var methodRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func (s *Server) listCommands(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	// lazy expiry: commands past their deadline without an ack are marked expired
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'expired' WHERE device_id = $1::uuid AND status IN ('pending','queued','sent') AND expires_at IS NOT NULL AND expires_at < now()`, id)
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.method, c.args, c.expires_at, c.status, c.issued_by_name, c.issued_at, c.sent_at, c.acked_at, c.result, c.error, c.reason, c.approved_by_name
		FROM core.device_command c JOIN core.device d ON d.id = c.device_id WHERE c.device_id = $1::uuid AND ($2 = '' OR d.org_id = $2::uuid) ORDER BY c.issued_at DESC LIMIT $3`, id, orgOf(p), limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var cid, method, status, by, reason string
		var args, result json.RawMessage
		var exp, sent, acked *time.Time
		var issued time.Time
		var errText, approvedBy *string
		if err := rows.Scan(&cid, &method, &args, &exp, &status, &by, &issued, &sent, &acked, &result, &errText, &reason, &approvedBy); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": cid, "method": method, "args": args, "expires_at": exp, "status": status, "issued_by": by, "issued_at": issued, "sent_at": sent, "acked_at": acked, "result": result, "error": errText, "reason": reason, "approved_by": approvedBy})
	}
	writeJSON(w, 200, out)
}

func (s *Server) sendCommand(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Method     string          `json:"method"`
		Args       json.RawMessage `json:"args"`
		TTLSeconds int             `json:"ttl_seconds"`
		Reason     string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Method = strings.TrimSpace(in.Method)
	if !methodRe.MatchString(in.Method) {
		writeErr(w, 400, "method must match ^[a-z][a-z0-9_]{0,31}$")
		return
	}
	if len(in.Args) == 0 {
		in.Args = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Args) || len(in.Args) > 8192 {
		writeErr(w, 400, "args must be JSON (max 8 KiB)")
		return
	}
	if in.TTLSeconds <= 0 {
		in.TTLSeconds = 300
	}
	if in.TTLSeconds > 86400*7 {
		writeErr(w, 400, "ttl_seconds too large")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var ext string
	var transport string
	if err := s.pool.QueryRow(r.Context(), `SELECT external_id, transport FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&ext, &transport); err != nil {
		writeErr(w, 404, "device not found")
		return
	}
	// Connector devices: the write is validated against the register map / node list before anything is recorded.
	op, code, err := s.prepareConnector(r.Context(), id, transport, in.Method, in.Args)
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	// Four eyes (URS H-08): the command is recorded as pending and executed only after a different user approves it.
	fourEyes := s.iam != nil && s.iam.Policy(r.Context(), org).FourEyesCommands
	initial := "queued"
	if fourEyes {
		initial = "pending"
	}
	exp := time.Now().Add(time.Duration(in.TTLSeconds) * time.Second).UTC()
	var cid string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.device_command (org_id, device_id, method, args, expires_at, issued_by, issued_by_name, reason, status)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5, NULLIF($6,'')::uuid, $7, $8, $9) RETURNING id::text`, org, id, in.Method, string(in.Args), exp, p.UserID, p.Username, in.Reason, initial).Scan(&cid); err != nil {
		s.log.Error("command insert", "err", err)
		writeErr(w, 500, "insert failed")
		return
	}
	status, result, cmdErr := initial, any(nil), ""
	if fourEyes {
		if s.live != nil {
			s.live.Command(org, id, cid, in.Method, "pending", nil)
		}
	} else {
		status, result, cmdErr = s.dispatchCommand(r.Context(), cmdExec{org: org, deviceID: id, ext: ext, transport: transport, cid: cid, method: in.Method, args: in.Args, exp: exp, op: op})
	}
	msg, _ := json.Marshal(map[string]any{"id": cid, "m": in.Method, "a": in.Args, "exp": exp.Unix()})
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, decode_status) VALUES ($1::uuid, $2::uuid, 'mqtt', 'out', $3, $4, $4::jsonb, 'ok')`, org, id, "d/"+ext+"/cmd", msg)
	s.audit(r, "device.command", "device", id, nil, map[string]any{"command_id": cid, "method": in.Method, "args": in.Args, "ttl_seconds": in.TTLSeconds, "status": status}, in.Reason)
	code = 201
	if fourEyes {
		code = 202
	}
	writeJSON(w, code, map[string]any{"id": cid, "method": in.Method, "status": status, "expires_at": exp, "result": result, "error": cmdErr})
}

func (s *Server) getAttributes(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var shared, status json.RawMessage
	if err := s.pool.QueryRow(r.Context(), `SELECT shared_attributes, status FROM core.device WHERE id = $1::uuid AND ($2 = '' OR org_id = $2::uuid)`, id, orgOf(p)).Scan(&shared, &status); err != nil {
		writeErr(w, 404, "device not found")
		return
	}
	var st map[string]json.RawMessage
	_ = json.Unmarshal(status, &st)
	client := json.RawMessage(`{}`)
	if v, ok := st["attributes"]; ok {
		client = v
	}
	writeJSON(w, 200, map[string]any{"shared": shared, "client": client})
}

// PUT merges attributes into the device's shared attributes, removes listed keys and republishes the retained document.
func (s *Server) putAttributes(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Attributes map[string]json.RawMessage `json:"attributes"`
		Remove     []string                   `json:"remove"`
		Reason     string                     `json:"reason"`
	}
	if err := decodeJSON(r, &in, 32768); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	for k := range in.Attributes {
		if k == "" || len(k) > 64 {
			writeErr(w, 400, "attribute keys must be 1–64 characters")
			return
		}
	}
	old, doc, published, code, err := s.setSharedAttributes(r.Context(), org, id, in.Attributes, in.Remove)
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	s.audit(r, "device.attributes", "device", id, old, doc, in.Reason)
	writeJSON(w, 200, map[string]any{"shared": doc, "published": published})
}

// setSharedAttributes merges shared attributes of a device, stores them and publishes them to the device (retained).
func (s *Server) setSharedAttributes(ctx context.Context, org, id string, set map[string]json.RawMessage, remove []string) (old, doc json.RawMessage, published bool, code int, err error) {
	var ext string
	if err := s.pool.QueryRow(ctx, `SELECT external_id, shared_attributes FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&ext, &old); err != nil {
		return nil, nil, false, 404, errors.New("device not found")
	}
	merged := map[string]json.RawMessage{}
	_ = json.Unmarshal(old, &merged)
	for k, v := range set {
		merged[k] = v
	}
	for _, k := range remove {
		delete(merged, k)
	}
	doc, _ = json.Marshal(merged)
	if len(doc) > 16384 {
		return nil, nil, false, 413, errors.New("shared attributes exceed 16 KiB")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.device SET shared_attributes = $2::jsonb WHERE id = $1::uuid`, id, string(doc)); err != nil {
		return nil, nil, false, 500, errors.New("update failed")
	}
	if s.br != nil {
		_ = s.br.Publish("d/"+ext+"/tb/attributes", doc, false) // ThingsBoard devices: v1/devices/me/attributes
		if err := s.br.Publish("d/"+ext+"/attributes/shared", doc, true); err == nil {
			published = true
			_, _ = s.pool.Exec(ctx, `INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, decode_status) VALUES ($1::uuid, $2::uuid, 'mqtt', 'out', $3, $4, $4::jsonb, 'ok')`, org, id, "d/"+ext+"/attributes/shared", doc)
		}
	}
	return old, doc, published, 200, nil
}
