package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
)

// Server administration from the UI (ADR-0012, URS K-06..K-09). Read-only facts come from this process and the
// database; anything that touches the host goes through the privileged agent. Everything needs system.admin and
// every mutation carries a reason into the audit trail.
//
//	GET  /api/v1/server                         process, config summary (no secrets), database, broker, agent status, update check
//	POST /api/v1/server/restart                 {unit: app|agent|caddy|wireguard, reason}
//	PUT  /api/v1/server/caddy                   {domain, extra_domains, email, dns_provider, cloudflare_token, reason}
//	GET  /api/v1/server/wireguard
//	PUT  /api/v1/server/wireguard               {enabled, listen_port, address, endpoint, dns, reason}
//	POST /api/v1/server/wireguard/peers         {name, reason} -> client config (once) + QR
//	POST /api/v1/server/wireguard/peers/{id}/remove {reason}
//	POST /api/v1/server/update                  {version, reason}

var updateCheck struct {
	sync.Mutex
	version string
	at      time.Time
	err     string
}

func (s *Server) updateURL() string {
	if u := strings.TrimRight(s.cfg.Agent.UpdateURL, "/"); u != "" {
		return u
	}
	return "https://telemetry.digital/dl"
}

func (s *Server) availableVersion(ctx context.Context) (string, time.Time, string) {
	updateCheck.Lock()
	defer updateCheck.Unlock()
	if time.Since(updateCheck.at) < 10*time.Minute {
		return updateCheck.version, updateCheck.at, updateCheck.err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	updateCheck.at, updateCheck.err = time.Now(), ""
	req, _ := http.NewRequestWithContext(ctx, "GET", s.updateURL()+"/VERSION", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		updateCheck.err = err.Error()
		return updateCheck.version, updateCheck.at, updateCheck.err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if resp.StatusCode == 200 {
		updateCheck.version = strings.TrimSpace(string(b))
	} else {
		updateCheck.err = "HTTP " + resp.Status
	}
	return updateCheck.version, updateCheck.at, updateCheck.err
}

func hostOf(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return u.Host + u.Path
}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.cfg
	out := map[string]any{
		"version": s.version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "pid": os.Getpid(),
		"started_at": s.started, "goroutines": runtime.NumGoroutine(),
		"config": map[string]any{
			"profile": cfg.Profile, "http_listen": cfg.HTTP.Listen, "base_url": cfg.HTTP.BaseURL, "public_read": cfg.HTTP.PublicRead,
			"http_tls": cfg.HTTP.TLSCert != "", "mqtt_listen": cfg.MQTT.Listen, "mqtt_tls_listen": cfg.MQTT.TLSListen, "mqtt_ws_listen": cfg.MQTT.WSListen,
			"mqtt_vpn_listen": cfg.MQTT.VPNListen, "db": hostOf(cfg.DB.URL), "smtp_host": cfg.SMTP.Host, "smtp_port": cfg.SMTP.Port,
			"log_file": cfg.Log.File, "log_level": cfg.Log.Level, "report_storage": cfg.Report.StorageDir, "typst_path": cfg.Report.TypstPath,
			"fota_storage": cfg.FOTA.StorageDir, "fota_max_mb": cfg.FOTA.MaxSizeMB, "gap_check": cfg.Gap.CheckInterval.String(),
			"agent_socket": cfg.Agent.Socket, "update_url": s.updateURL(),
		},
	}
	db := map[string]any{}
	var sizeMB float64
	var orgs, users, devices, sites, reports int64
	_ = s.pool.QueryRow(ctx, `SELECT pg_database_size(current_database())/1048576.0,
		(SELECT count(*) FROM core.organization), (SELECT count(*) FROM core.user_account WHERE disabled_at IS NULL),
		(SELECT count(*) FROM core.device WHERE revoked_at IS NULL), (SELECT count(*) FROM core.site), (SELECT count(*) FROM core.report)`).
		Scan(&sizeMB, &orgs, &users, &devices, &sites, &reports)
	db["size_mb"], db["organizations"], db["users"], db["devices"], db["sites"], db["reports"] = sizeMB, orgs, users, devices, sites, reports
	var pgv string
	_ = s.pool.QueryRow(ctx, `SHOW server_version`).Scan(&pgv)
	db["server_version"] = pgv
	var pending int64
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.device_command WHERE status IN ('queued','sent')`).Scan(&pending)
	db["pending_commands"] = pending
	out["database"] = db
	br := map[string]any{"connected_clients": 0}
	if s.br != nil && s.br.Server != nil {
		br["connected_clients"] = s.br.Server.Clients.Len()
	}
	out["broker"] = br
	ag := map[string]any{"configured": s.agent.Configured(), "connected": false}
	if s.agent.Configured() {
		if st, err := s.agent.Status(ctx); err != nil {
			ag["error"] = err.Error()
		} else {
			ag["connected"] = true
			ag["status"] = st
		}
	}
	out["agent"] = ag
	v, at, err := s.availableVersion(ctx)
	out["update"] = map[string]any{"current": s.version, "available": v, "checked_at": at, "error": err, "url": s.updateURL()}
	out["time"] = s.tc.State()
	out["links"] = map[string]any{"metrics": "/metrics", "security_txt": "/.well-known/security.txt"}
	writeJSON(w, 200, out)
}

func (s *Server) agentErr(w http.ResponseWriter, err error) {
	if errors.Is(err, agent.ErrNoAgent) {
		writeErr(w, 503, "the server agent is not configured on this host (Linux installs run it as ctrl32-telemetry-agent)")
		return
	}
	writeErr(w, 502, err.Error())
}

func (s *Server) serverRestart(w http.ResponseWriter, r *http.Request) {
	var in struct{ Unit, Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	switch in.Unit {
	case "app", "agent", "caddy", "wireguard":
	default:
		writeErr(w, 400, "unit must be app, agent, caddy or wireguard")
		return
	}
	res, err := s.agent.Restart(r.Context(), in.Unit)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.restart", "server", in.Unit, nil, map[string]any{"unit": in.Unit}, in.Reason)
	writeJSON(w, 200, res)
}

func (s *Server) serverCaddy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		agent.CaddyRequest
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	res, err := s.agent.SetCaddy(r.Context(), in.CaddyRequest)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.caddy", "server", "caddy", nil, map[string]any{"domain": in.Domain, "extra_domains": in.ExtraDomains, "email": in.Email, "dns_provider": in.DNSProvider, "token_changed": in.CloudflareToken != ""}, in.Reason)
	writeJSON(w, 200, res)
}

func (s *Server) serverWireGuard(w http.ResponseWriter, r *http.Request) {
	st, err := s.agent.WireGuard(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) serverSetWireGuard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		agent.WireGuardRequest
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	st, err := s.agent.SetWireGuard(r.Context(), in.WireGuardRequest)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.wireguard", "server", "wireguard", nil, map[string]any{"enabled": in.Enabled, "listen_port": in.ListenPort, "address": in.Address, "endpoint": in.Endpoint}, in.Reason)
	writeJSON(w, 200, st)
}

func (s *Server) serverAddPeer(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct{ Name, Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" || strings.TrimSpace(in.Name) == "" {
		writeErr(w, 400, "name and reason are required")
		return
	}
	res, err := s.agent.AddPeer(r.Context(), agent.PeerRequest{Name: in.Name, CreatedBy: p.Username})
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.wireguard_peer_add", "wireguard_peer", res.Peer.ID, nil, map[string]any{"name": res.Peer.Name, "address": res.Peer.Address, "public_key": res.Peer.PublicKey}, in.Reason)
	out := map[string]any{"peer": res.Peer, "config": res.Config}
	if png, err := qrcode.Encode(res.Config, qrcode.Medium, 360); err == nil {
		out["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	writeJSON(w, 200, out)
}

func (s *Server) serverRemovePeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	if strings.ContainsAny(id, "/\\ ") || id == "" {
		writeErr(w, 400, "invalid peer id")
		return
	}
	res, err := s.agent.RemovePeer(r.Context(), id)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.wireguard_peer_remove", "wireguard_peer", id, nil, nil, in.Reason)
	writeJSON(w, 200, res)
}

func (s *Server) serverUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct{ Version, Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Version = strings.TrimSpace(in.Version)
	if strings.TrimSpace(in.Reason) == "" || in.Version == "" {
		writeErr(w, 400, "version and reason are required")
		return
	}
	if strings.ContainsAny(in.Version, " /\\") {
		writeErr(w, 400, "invalid version")
		return
	}
	res, err := s.agent.Update(r.Context(), in.Version)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.update", "server", in.Version, map[string]any{"version": s.version}, map[string]any{"version": in.Version, "sha256": res.SHA256}, in.Reason)
	writeJSON(w, 200, res)
}

// ---- backups (URS K-14) ---------------------------------------------------------------------------

var backupNameRe = regexp.MustCompile(`^ctrl32-\d{8}-\d{6}\.tar\.gz(\.enc)?$`)

func (s *Server) serverBackups(w http.ResponseWriter, r *http.Request) {
	st, err := s.agent.Backups(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) serverBackupCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ Passphrase, Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	res, err := s.agent.CreateBackup(r.Context(), in.Passphrase)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_create", "server", "backup", nil, map[string]any{"encrypted": in.Passphrase != ""}, in.Reason)
	writeJSON(w, 202, res)
}

func (s *Server) serverBackupDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	if !backupNameRe.MatchString(name) {
		writeErr(w, 400, "invalid backup name")
		return
	}
	res, err := s.agent.DeleteBackup(r.Context(), name)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_delete", "server", name, nil, nil, in.Reason)
	writeJSON(w, 200, res)
}

func (s *Server) serverBackupSchedule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		agent.ScheduleRequest
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	st, err := s.agent.SetBackupSchedule(r.Context(), in.ScheduleRequest)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_schedule", "server", "backup", nil, map[string]any{"enabled": in.Enabled, "hour": in.Hour, "minute": in.Minute, "keep": in.Keep, "passphrase_changed": in.Passphrase != "" || in.ClearPassphrase}, in.Reason)
	writeJSON(w, 200, st)
}

// serverBackupDownload streams a bundle; the file is root:ctrl32 0640 so this process can read it.
func (s *Server) serverBackupDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !backupNameRe.MatchString(name) {
		writeErr(w, 400, "invalid backup name")
		return
	}
	if !s.agent.Configured() {
		s.agentErr(w, agent.ErrNoAgent)
		return
	}
	f, err := os.Open(filepath.Join(agent.BackupDir, name))
	if err != nil {
		writeErr(w, 404, "backup not found")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	s.audit(r, "server.backup_download", "server", name, nil, map[string]any{"bytes": st.Size()}, "download")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, st.ModTime(), f)
}
