package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

// FOTA (protocol §3.6, URS C-*): firmware images are records; a campaign targets devices, the server publishes a
// retained d/{id}/fota/target, the device downloads the blob over HTTPS with its own token (Range supported),
// verifies the SHA-256 and reports progress on fota/status.
//
//	GET    /api/v1/firmware                       (fota.read)     list images
//	POST   /api/v1/firmware                       (fota.manage)   multipart: file, name, version, hw, min_version, signature, notes, reason
//	GET    /api/v1/fota/campaigns                 (fota.read)     campaigns with status counts
//	POST   /api/v1/fota/campaigns                 (fota.manage)   {firmware_id, device_ids, name, reason}
//	GET    /api/v1/fota/campaigns/{id}            (fota.read)     targets
//	DELETE /api/v1/fota/campaigns/{id}?reason=    (fota.manage)   cancel (clears retained targets)
//	GET    /api/v1/d/fota/{fw}/blob               device token (Bearer), only for targeted devices

var versionRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,32}$`)

func (s *Server) listFirmware(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, name, version, hw, size, sha256, signature IS NOT NULL, COALESCE(min_version,''), COALESCE(notes,''), uploaded_by_name, uploaded_at,
		(SELECT count(*) FROM core.fota_campaign c WHERE c.firmware_id = f.id) FROM core.firmware f WHERE org_id = $1::uuid ORDER BY uploaded_at DESC`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, ver, hw, sha, minv, notes, by string
		var size int64
		var signed bool
		var at time.Time
		var campaigns int
		if err := rows.Scan(&id, &name, &ver, &hw, &size, &sha, &signed, &minv, &notes, &by, &at, &campaigns); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "version": ver, "hw": hw, "size": size, "sha256": sha, "signed": signed, "min_version": minv, "notes": notes, "uploaded_by": by, "uploaded_at": at, "campaigns": campaigns})
	}
	writeJSON(w, 200, out)
}

func (s *Server) uploadFirmware(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	maxBytes := int64(s.cfg.FOTA.MaxSizeMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeErr(w, 400, "multipart form required (max "+fmt.Sprint(s.cfg.FOTA.MaxSizeMB)+" MiB)")
		return
	}
	name, ver, hw := strings.TrimSpace(r.FormValue("name")), strings.TrimSpace(r.FormValue("version")), strings.TrimSpace(strings.ToLower(r.FormValue("hw")))
	minv, sig, notes, reason := strings.TrimSpace(r.FormValue("min_version")), strings.TrimSpace(r.FormValue("signature")), strings.TrimSpace(r.FormValue("notes")), strings.TrimSpace(r.FormValue("reason"))
	if name == "" || len(name) > 64 || !versionRe.MatchString(ver) || hw == "" || len(hw) > 32 {
		writeErr(w, 400, "name, version ([A-Za-z0-9._+-]) and hw are required")
		return
	}
	if minv != "" && !versionRe.MatchString(minv) {
		writeErr(w, 400, "invalid min_version")
		return
	}
	if sig != "" {
		if _, err := hex.DecodeString(sig); err != nil || len(sig) != 128 {
			writeErr(w, 400, "signature must be 64 bytes hex (Ed25519)")
			return
		}
	}
	if !reasonOf(w, reason) {
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file field required")
		return
	}
	defer file.Close()
	if hdr.Size > maxBytes || hdr.Size <= 0 {
		writeErr(w, 413, "firmware exceeds the configured maximum size")
		return
	}
	if err := os.MkdirAll(s.cfg.FOTA.StorageDir, 0o750); err != nil {
		writeErr(w, 500, "storage unavailable")
		return
	}
	tmp, err := os.CreateTemp(s.cfg.FOTA.StorageDir, "upload-*.tmp")
	if err != nil {
		writeErr(w, 500, "storage unavailable")
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), file)
	tmp.Close()
	if err != nil || n != hdr.Size {
		os.Remove(tmp.Name())
		writeErr(w, 400, "upload incomplete")
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	var id, path string
	err = s.pool.QueryRow(r.Context(), `WITH n AS (SELECT uuidv7() AS id)
		INSERT INTO core.firmware (id, org_id, name, version, hw, size, sha256, signature, min_version, notes, file_path, uploaded_by, uploaded_by_name, reason)
		SELECT n.id, $1::uuid, $2, $3, $4, $5, $6, NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), n.id::text || '.bin', NULLIF($10,'')::uuid, $11, $12 FROM n RETURNING id::text, file_path`,
		org, name, ver, hw, n, sum, sig, minv, notes, p.UserID, p.Username, reason).Scan(&id, &path)
	if err != nil {
		os.Remove(tmp.Name())
		writeErr(w, 409, "firmware could not be recorded (duplicate name/version/hw?)")
		return
	}
	if err := os.Rename(tmp.Name(), filepath.Join(s.cfg.FOTA.StorageDir, path)); err != nil {
		os.Remove(tmp.Name())
		writeErr(w, 500, "storage failed")
		return
	}
	s.audit(r, "firmware.upload", "firmware", id, nil, map[string]any{"name": name, "version": ver, "hw": hw, "size": n, "sha256": sum, "signed": sig != ""}, reason)
	writeJSON(w, 201, map[string]any{"id": id, "name": name, "version": ver, "hw": hw, "size": n, "sha256": sum, "signed": sig != ""})
}

type fotaTarget struct {
	FwID     string `json:"fw_id"`
	Ver      string `json:"ver"`
	HW       string `json:"hw"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	URL      string `json:"url"`
	MinVer   string `json:"min_ver,omitempty"`
	Sig      string `json:"sig,omitempty"`
	Campaign string `json:"campaign"`
}

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.name, c.firmware_id::text, f.name, f.version, f.hw, c.created_by_name, c.created_at, c.closed_at, c.close_reason,
		(SELECT jsonb_object_agg(status, n) FROM (SELECT status, count(*) n FROM core.fota_target t WHERE t.campaign_id = c.id GROUP BY status) x)
		FROM core.fota_campaign c JOIN core.firmware f ON f.id = c.firmware_id WHERE c.org_id = $1::uuid ORDER BY c.created_at DESC LIMIT 200`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, fwID, fwName, ver, hw, by string
		var created time.Time
		var closed *time.Time
		var closeReason *string
		var counts json.RawMessage
		if err := rows.Scan(&id, &name, &fwID, &fwName, &ver, &hw, &by, &created, &closed, &closeReason, &counts); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		if counts == nil {
			counts = json.RawMessage(`{}`)
		}
		out = append(out, map[string]any{"id": id, "name": name, "firmware_id": fwID, "firmware": fwName + " " + ver, "hw": hw, "created_by": by, "created_at": created, "closed_at": closed, "close_reason": closeReason, "counts": counts})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		FirmwareID string   `json:"firmware_id"`
		DeviceIDs  []string `json:"device_ids"`
		Name       string   `json:"name"`
		Reason     string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 65536); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(in.DeviceIDs) == 0 || len(in.DeviceIDs) > 1000 {
		writeErr(w, 400, "device_ids required (1–1000)")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var fw fotaTarget
	var fwName string
	if err := s.pool.QueryRow(r.Context(), `SELECT id::text, name, version, hw, size, sha256, COALESCE(min_version,''), COALESCE(signature,'') FROM core.firmware WHERE id = $1::uuid AND org_id = $2::uuid`, in.FirmwareID, org).
		Scan(&fw.FwID, &fwName, &fw.Ver, &fw.HW, &fw.Size, &fw.SHA256, &fw.MinVer, &fw.Sig); err != nil {
		writeErr(w, 404, "firmware not found")
		return
	}
	if in.Name == "" {
		in.Name = fwName + " " + fw.Ver + " " + time.Now().UTC().Format("2006-01-02")
	}
	fw.URL = s.cfg.HTTP.BaseURL + "/api/v1/d/fota/" + fw.FwID + "/blob"
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	var cid string
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.fota_campaign (org_id, firmware_id, name, created_by, created_by_name, reason) VALUES ($1::uuid, $2::uuid, $3, NULLIF($4,'')::uuid, $5, $6) RETURNING id::text`,
		org, fw.FwID, in.Name, p.UserID, p.Username, in.Reason).Scan(&cid); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	fw.Campaign = cid
	type dev struct{ id, ext, transport string }
	var devs []dev
	for _, id := range in.DeviceIDs {
		var d dev
		if err := tx.QueryRow(r.Context(), `SELECT id::text, external_id, transport FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&d.id, &d.ext, &d.transport); err != nil {
			writeErr(w, 404, "device not found: "+id)
			return
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO core.fota_target (campaign_id, device_id) VALUES ($1::uuid, $2::uuid) ON CONFLICT DO NOTHING`, cid, d.id); err != nil {
			writeErr(w, 500, "target insert failed")
			return
		}
		devs = append(devs, d)
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	msg, _ := json.Marshal(fw)
	published := 0
	for _, d := range devs {
		if d.transport == "mqtt" && s.br != nil {
			if err := s.br.Publish("d/"+d.ext+"/fota/target", msg, true); err == nil {
				published++
			}
		}
	}
	s.audit(r, "fota.campaign_create", "fota_campaign", cid, nil, map[string]any{"firmware_id": fw.FwID, "version": fw.Ver, "devices": len(devs), "published": published, "name": in.Name}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": cid, "name": in.Name, "devices": len(devs), "published": published})
}

func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var c struct {
		ID, Name, FwID, FwName, Ver, HW, By string
		Created                             time.Time
		Closed                              *time.Time
		CloseReason                         *string
	}
	if err := s.pool.QueryRow(r.Context(), `SELECT c.id::text, c.name, c.firmware_id::text, f.name, f.version, f.hw, c.created_by_name, c.created_at, c.closed_at, c.close_reason
		FROM core.fota_campaign c JOIN core.firmware f ON f.id = c.firmware_id WHERE c.id = $1::uuid AND c.org_id = $2::uuid`, id, org).
		Scan(&c.ID, &c.Name, &c.FwID, &c.FwName, &c.Ver, &c.HW, &c.By, &c.Created, &c.Closed, &c.CloseReason); err != nil {
		writeErr(w, 404, "campaign not found")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT t.device_id::text, d.external_id, d.name, t.status, t.pct, t.version, t.error, t.updated_at, d.status->>'fw'
		FROM core.fota_target t JOIN core.device d ON d.id = t.device_id WHERE t.campaign_id = $1::uuid ORDER BY d.external_id`, id)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	targets := []map[string]any{}
	for rows.Next() {
		var devID, ext, name, status string
		var pct *int
		var ver, errText, fw *string
		var at time.Time
		if err := rows.Scan(&devID, &ext, &name, &status, &pct, &ver, &errText, &at, &fw); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		targets = append(targets, map[string]any{"device_id": devID, "external_id": ext, "name": name, "status": status, "pct": pct, "version": ver, "error": errText, "updated_at": at, "device_fw": fw})
	}
	writeJSON(w, 200, map[string]any{"campaign": map[string]any{"id": c.ID, "name": c.Name, "firmware_id": c.FwID, "firmware": c.FwName + " " + c.Ver, "hw": c.HW, "created_by": c.By, "created_at": c.Created, "closed_at": c.Closed, "close_reason": c.CloseReason}, "targets": targets})
}

func (s *Server) cancelCampaign(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.fota_campaign SET closed_at = now(), close_reason = $3 WHERE id = $1::uuid AND org_id = $2::uuid AND closed_at IS NULL`, id, org, reason)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "open campaign not found")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.fota_target SET status = 'cancelled', updated_at = now() WHERE campaign_id = $1::uuid AND status IN ('pending', 'downloading', 'verifying', 'applying')`, id)
	// clear the retained target on every device of the campaign
	rows, err := s.pool.Query(r.Context(), `SELECT d.external_id FROM core.fota_target t JOIN core.device d ON d.id = t.device_id WHERE t.campaign_id = $1::uuid AND d.transport = 'mqtt'`, id)
	if err == nil {
		var exts []string
		for rows.Next() {
			var e string
			if rows.Scan(&e) == nil {
				exts = append(exts, e)
			}
		}
		rows.Close()
		for _, e := range exts {
			_ = s.br.Publish("d/"+e+"/fota/target", []byte{}, true)
		}
	}
	s.audit(r, "fota.campaign_cancel", "fota_campaign", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/v1/d/fota/{fw}/blob — device endpoint: Bearer <device token>; only devices targeted by an open campaign.
func (s *Server) firmwareBlob(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ctrl32 device"`)
		writeErr(w, 401, "device token required")
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	fwID := r.PathValue("fw")
	var devID, ext, path, sum string
	var size int64
	err := s.pool.QueryRow(r.Context(), `SELECT d.id::text, d.external_id, f.file_path, f.sha256, f.size
		FROM core.device d JOIN core.device_credential c ON c.device_id = d.id AND c.kind = 'token' AND c.secret_hash = $1 AND c.revoked_at IS NULL AND (c.valid_to IS NULL OR c.valid_to > now())
		JOIN core.fota_target t ON t.device_id = d.id JOIN core.fota_campaign cp ON cp.id = t.campaign_id AND cp.closed_at IS NULL
		JOIN core.firmware f ON f.id = cp.firmware_id
		WHERE d.revoked_at IS NULL AND f.id = $2::uuid AND f.org_id = d.org_id LIMIT 1`, broker.TokenHash(token), fwID).Scan(&devID, &ext, &path, &sum, &size)
	if err != nil {
		s.log.Warn("firmware blob denied", "security", true, "remote", clientIP(r), "fw", fwID)
		writeErr(w, 401, "not authorized for this firmware")
		return
	}
	f, err := os.Open(filepath.Join(s.cfg.FOTA.StorageDir, filepath.Base(path)))
	if err != nil {
		writeErr(w, 404, "firmware file missing")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.fota_target t SET status = 'downloading', updated_at = now() FROM core.fota_campaign c WHERE t.campaign_id = c.id AND t.device_id = $1::uuid AND c.firmware_id = $2::uuid AND c.closed_at IS NULL AND t.status = 'pending'`, devID, fwID)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Checksum-SHA256", sum)
	w.Header().Set("Cache-Control", "private, no-store")
	s.log.Info("firmware download", "device", ext, "fw", fwID, "range", r.Header.Get("Range"))
	http.ServeContent(w, r, "firmware.bin", st.ModTime(), f)
}
