package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

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
//	(peers and access rules of the VPN: vpn.go, /api/v1/vpn)
//	(updates: updates.go; e-mail (SMTP): smtp_settings.go)

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
			"mqtt_vpn_listen": cfg.MQTT.VPNListen, "db": hostOf(cfg.DB.URL), "smtp_host": s.mailer().SMTP().Host, "smtp_port": s.mailer().SMTP().Port,
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
	upd := map[string]any{"current": s.version, "available": v, "checked_at": at, "error": err, "url": s.updateURL()}
	for k, x := range s.updatesJSON(ctx) {
		upd[k] = x
	}
	out["update"] = upd
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

// ---- backups (URS K-14) ---------------------------------------------------------------------------

var backupNameRe = regexp.MustCompile(`^ctrl32-\d{8}-\d{6}\.tar\.gz(\.enc)?$`)

func (s *Server) serverBackups(w http.ResponseWriter, r *http.Request) {
	s.auditBackupResults(r.Context()) // results finished since the last tick are in the trail before they are shown
	st, err := s.agent.Backups(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) serverBackupCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Passphrase string `json:"passphrase"`
		UseStored  bool   `json:"use_stored"` // encrypt with the passphrase of the schedule
		Reason     string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	res, err := s.agent.CreateBackup(r.Context(), in.Passphrase, in.UseStored, actorOf(r))
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_create", "server", "backup", nil, map[string]any{"encrypted": in.Passphrase != "" || in.UseStored && res.Schedule.HasPassphrase}, in.Reason)
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
	s.audit(r, "server.backup_schedule", "server", "backup", nil, map[string]any{"enabled": in.Enabled, "hour": in.Hour, "minute": in.Minute, "keep": in.Keep, "keep_weekly": in.KeepWeekly, "keep_monthly": in.KeepMonthly, "passphrase_changed": in.Passphrase != "" || in.ClearPassphrase}, in.Reason)
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
