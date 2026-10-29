package httpapi

// The operating system of the server (ADR-0025, spec 034): pending package upgrades, the upgrade, a delayed reboot and
// the logs of a closed list of units, through the privileged agent. Every operation needs system.admin and a reason
// and is audited when it is asked for; the finished jobs go into the audit trail once (auditOSResults, like the
// update results). MeshCentral stays the emergency path: its address is a server setting and opening it is audited.
//
//	GET  /api/v1/server/os                      state (?refresh=1 lists the upgrades again)
//	POST /api/v1/server/os/check                {reason}            apt-get update, then the list (202, a job)
//	POST /api/v1/server/os/upgrade              {reason}            apt-get full-upgrade in its own unit (202)
//	POST /api/v1/server/os/reboot               {reason, delay_s}   reboot in 60 s (202)
//	POST /api/v1/server/os/reboot/cancel        {reason}
//	GET  /api/v1/server/logs?unit=&lines=       the journal of app, agent, caddy, postgresql, wireguard, cloudflared, kernel
//	GET  /api/v1/server/remote                  MeshCentral address and the node of this server
//	PUT  /api/v1/server/remote                  {mesh_url, mesh_node, reason}
//	POST /api/v1/server/remote/open             {reason} → {url}   emergency remote desktop of this server (audited)

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// agentRefusal passes a coded refusal of the agent on with its status (busy 409, unsupported 501, …).
func (s *Server) agentRefusal(w http.ResponseWriter, err error) {
	var ce *agent.CodedError
	if errors.As(err, &ce) && ce.Status >= 400 && ce.Status <= 599 {
		writeJSON(w, ce.Status, map[string]string{"error": ce.Msg, "code": ce.Code})
		return
	}
	s.agentErr(w, err)
}

func (s *Server) serverOS(w http.ResponseWriter, r *http.Request) {
	st, err := s.agent.OS(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	s.auditOSResults(r.Context(), st)
	writeJSON(w, 200, st)
}

func (s *Server) serverOSCheck(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	j, err := s.agent.OSCheck(r.Context(), agent.OSRequest{Actor: p.Username, Reason: in.Reason})
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	s.audit(r, "server.os_check", "server", "os", nil, map[string]any{"job": j.ID}, in.Reason)
	writeJSON(w, 202, j)
}

func (s *Server) serverOSUpgrade(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	before, _ := s.agent.OS(r.Context(), false)
	j, err := s.agent.OSUpgrade(r.Context(), agent.OSRequest{Actor: p.Username, Reason: in.Reason})
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	n := map[string]any{"job": j.ID}
	if before != nil {
		n["pending"], n["security"] = before.UpgradeCount, before.SecurityCount
	}
	s.audit(r, "server.os_upgrade", "server", "os", nil, n, in.Reason)
	writeJSON(w, 202, j)
}

func (s *Server) serverOSReboot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
		Delay  int    `json:"delay_s"`
	}
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	rb, err := s.agent.OSReboot(r.Context(), agent.OSRebootRequest{DelaySeconds: in.Delay, Actor: p.Username, Reason: in.Reason})
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	s.audit(r, "server.reboot", "server", "os", nil, map[string]any{"at": rb.At, "id": rb.ID}, in.Reason)
	writeJSON(w, 202, rb)
}

func (s *Server) serverOSRebootCancel(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	rb, err := s.agent.OSRebootCancel(r.Context(), agent.OSRequest{Actor: p.Username, Reason: in.Reason})
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	s.audit(r, "server.reboot_cancel", "server", "os", map[string]any{"at": rb.At, "id": rb.ID, "by": rb.Actor}, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) serverLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	unit := q.Get("unit")
	if !slices.Contains(agent.LogKeys, unit) {
		writeErr(w, 400, "unit must be one of "+strings.Join(agent.LogKeys, ", "))
		return
	}
	lines, _ := strconv.Atoi(q.Get("lines"))
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	lr, err := s.agent.Logs(r.Context(), unit, lines)
	if err != nil {
		s.agentRefusal(w, err)
		return
	}
	writeJSON(w, 200, lr)
}

// auditOSResults writes the finished jobs of the agent into the audit trail once and mails the server administrators
// about a failed upgrade.
func (s *Server) auditOSResults(ctx context.Context, st *agent.OSState) {
	if s.pool == nil || !s.agent.Configured() {
		return
	}
	s.upd.auditMu.Lock()
	defer s.upd.auditMu.Unlock()
	if st == nil {
		var err error
		if st, err = s.agent.OSJobs(ctx); err != nil || !st.Supported {
			return
		}
	}
	var ids []string
	for _, j := range st.History {
		if j.Audited || j.Phase == "running" {
			continue
		}
		e := audit.Entry{OrgID: s.serverOrg(ctx), ActorType: "system", ActorName: "server agent", Action: "server.os_job_result", EntityType: "server:os",
			New: j, Reason: "result of the operating system job " + j.ID + " (" + j.Kind + ", by " + j.Actor + "): " + j.Phase}
		if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
			s.log.Error("audit append failed", "security", true, "action", e.Action, "err", err)
			return
		}
		ids = append(ids, j.ID)
		if j.Kind == "upgrade" && j.Phase == "failed" {
			s.mailAdmins(ctx, productName+": the upgrade of the operating system packages failed",
				"The upgrade "+j.ID+" started by "+j.Actor+" failed:\n\n"+j.Error+"\n\nThe log is under System → Server → Operating system.\n")
		}
	}
	if len(ids) > 0 {
		_ = s.agent.AckOS(ctx, ids)
	}
}

// ---- MeshCentral: the emergency remote desktop --------------------------------------------------------------------

type remoteSettings struct {
	MeshURL  string `json:"mesh_url"`  // https://mesh.example.com
	MeshNode string `json:"mesh_node"` // the node of this server in MeshCentral (optional)
}

// meshNodeRe: a MeshCentral node id — base64 of 48 bytes with $ and @ for + and /, optionally with its node/<domain>/
// prefix (copied from the address bar or the device's details).
var meshNodeRe = regexp.MustCompile(`^(?:node/[A-Za-z0-9._-]*/)?[A-Za-z0-9$@+/=_-]{20,96}$`)

func (s *Server) loadRemoteSettings(ctx context.Context) remoteSettings {
	var st remoteSettings
	if s.pool == nil {
		return st
	}
	var raw []byte
	if s.pool.QueryRow(ctx, `SELECT data FROM core.server_setting WHERE key = 'remote_management'`).Scan(&raw) == nil {
		_ = json.Unmarshal(raw, &st)
	}
	return st
}

// validMeshURL accepts an https address of a MeshCentral server without credentials, query or fragment.
func validMeshURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 200 {
		return "", false
	}
	return strings.TrimRight(u.String(), "/"), true
}

// normalizeMeshNode removes the node/<domain>/ prefix MeshCentral shows in its own ids.
func normalizeMeshNode(n string) (string, bool) {
	n = strings.TrimSpace(n)
	if n == "" {
		return "", true
	}
	if !meshNodeRe.MatchString(n) {
		return "", false
	}
	if strings.HasPrefix(n, "node/") {
		n = n[strings.LastIndex(n, "/")+1:]
	}
	return n, true
}

// meshLink is the address of a device's remote desktop in MeshCentral (viewmode 11 = desktop). MeshCentral builds the
// same link with the id as it is ($ and @ unescaped); the node id was validated, only +, / and = need escaping.
func meshLink(base, node string) string {
	return base + "/?gotonode=" + strings.NewReplacer("+", "%2B", "/", "%2F", "=", "%3D").Replace(node) + "&viewmode=11&hide=16"
}

func (s *Server) serverRemote(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.loadRemoteSettings(r.Context()))
}

func (s *Server) serverSetRemote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MeshURL  string `json:"mesh_url"`
		MeshNode string `json:"mesh_node"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	u, ok := validMeshURL(in.MeshURL)
	if !ok {
		writeErr(w, 400, "mesh_url: the https address of the MeshCentral server, e.g. https://mesh.example.com")
		return
	}
	node, ok := normalizeMeshNode(in.MeshNode)
	if !ok {
		writeErr(w, 400, "mesh_node: the node id of this server in MeshCentral (its details, or gotonode= in the address bar)")
		return
	}
	old := s.loadRemoteSettings(r.Context())
	st := remoteSettings{MeshURL: u, MeshNode: node}
	b, _ := json.Marshal(st)
	p := PrincipalFrom(r.Context())
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.server_setting (key, data, updated_at, updated_by) VALUES ('remote_management', $1::jsonb, now(), NULLIF($2,'')::uuid)
		ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data, updated_at = now(), updated_by = COALESCE(EXCLUDED.updated_by, core.server_setting.updated_by)`, string(b), p.UserID); err != nil {
		s.log.Error("remote management settings not saved", "err", err)
		writeErr(w, 500, "could not save the remote management settings")
		return
	}
	s.audit(r, "server.remote_settings", "server", "remote", old, st, in.Reason)
	writeJSON(w, 200, st)
}

// serverRemoteOpen hands out the link to this server's remote desktop in MeshCentral and records who opened it and
// why. MeshCentral asks for its own sign-in (with two factors); the link carries no credential.
func (s *Server) serverRemoteOpen(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	st := s.loadRemoteSettings(r.Context())
	if st.MeshURL == "" || st.MeshNode == "" {
		writeJSON(w, 409, map[string]string{"error": "set the MeshCentral address and the node of this server first (System → Server → Remote management)", "code": "not_configured"})
		return
	}
	link := meshLink(st.MeshURL, st.MeshNode)
	s.audit(r, "remote.mesh_open", "server", "remote", nil, map[string]any{"target": "server", "url": link}, in.Reason)
	writeJSON(w, 200, map[string]string{"url": link})
}
