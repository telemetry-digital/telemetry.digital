package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
)

// Offsite backups, retention and the restore test (0.77.0, URS K-01..K-04, spec 032). The agent does the work and
// records every run with its steps; this file adds the permission checks, the reasons and the audit trail, writes
// finished runs into the audit trail once, and opens an incident when a backup, an offsite copy or a restore test
// fails or no backup succeeded for more than a day (K-03).

// backupsMutation checks a request that sends the server's data elsewhere (the offsite target): a person in the
// browser, a reason, the password (and authenticator code) again.
func (s *Server) offsiteMutation(w http.ResponseWriter, r *http.Request, reason, password, code string) bool {
	p := PrincipalFrom(r.Context())
	if p == nil || p.SessionID == "" || p.TokenID != "" || p.GrantID != "" || r.Context().Value(viaMCPKey) != nil {
		writeJSON(w, 403, map[string]string{"error": "the offsite target is set by a person in the browser, not with an API token or an AI assistant", "code": "session_required"})
		return false
	}
	if !reasonOf(w, reason) || !s.stepUp(w, r, password, code) {
		return false
	}
	if !s.agent.Configured() {
		s.agentErr(w, agent.ErrNoAgent)
		return false
	}
	return true
}

func actorOf(r *http.Request) string {
	if p := PrincipalFrom(r.Context()); p != nil {
		return p.Username
	}
	return ""
}

// PUT /api/v1/server/backups/offsite {config, reason, password, code}; config.kind "" removes the target.
func (s *Server) serverBackupOffsite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config                 offsite.Config `json:"config"`
		Reason, Password, Code string
	}
	if err := decodeJSON(r, &in, 32*1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.offsiteMutation(w, r, in.Reason, in.Password, in.Code) {
		return
	}
	before, _ := s.agent.Backups(r.Context())
	st, err := s.agent.SetOffsite(r.Context(), in.Config)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	var old any
	if before != nil {
		old = before.Offsite.Config
	}
	s.audit(r, "server.backup_offsite", "server", "backup", old, st.Offsite.Config, strings.TrimSpace(in.Reason))
	writeJSON(w, 200, st)
}

// POST /api/v1/server/backups/offsite/test {config}: write, read back and delete a probe file (nothing is stored).
func (s *Server) serverBackupOffsiteTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config offsite.Config `json:"config"`
	}
	if err := decodeJSON(r, &in, 32*1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := s.agent.TestOffsite(r.Context(), in.Config)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_offsite_test", "server", "backup", nil, map[string]any{"target": in.Config.Describe(), "ok": res.OK, "detail": res.Detail}, "connection test")
	writeJSON(w, 200, res)
}

// POST /api/v1/server/backups/offsite/sync {reason}
func (s *Server) serverBackupOffsiteSync(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	st, err := s.agent.SyncOffsite(r.Context(), actorOf(r))
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.backup_offsite_sync", "server", "backup", nil, nil, strings.TrimSpace(in.Reason))
	writeJSON(w, 202, st)
}

// PUT /api/v1/server/backups/restore-test {enabled, day, hour, minute, source, reason}
func (s *Server) serverRestoreTestSchedule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		agent.RestoreTestRequest
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	st, err := s.agent.SetRestoreTest(r.Context(), in.RestoreTestRequest)
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.restore_test_schedule", "server", "backup", nil, in.RestoreTestRequest, strings.TrimSpace(in.Reason))
	writeJSON(w, 200, st)
}

// POST /api/v1/server/backups/restore-test {source, reason}
func (s *Server) serverRestoreTestRun(w http.ResponseWriter, r *http.Request) {
	var in struct{ Source, Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	st, err := s.agent.RunRestoreTest(r.Context(), in.Source, actorOf(r))
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.restore_test_start", "server", "backup", nil, map[string]any{"source": in.Source}, strings.TrimSpace(in.Reason))
	writeJSON(w, 202, st)
}

// ---- results: audit trail and incidents (URS K-03) --------------------------------------------------------------

const (
	incBackupFailed  = "backup:failed"
	incBackupMissing = "backup:missing"
	incRestoreTest   = "backup:restore_test"
)

var runAction = map[string]string{"backup": "server.backup_result", "offsite_sync": "server.backup_offsite_sync_result", "restore_test": "server.restore_test_result"}

// auditBackupResults writes the finished runs of the agent into the audit trail once and opens or resolves the
// incidents; it runs with the update tick (every minute).
// backupAuditMu: the tick and a page load must not write the same run twice.
var backupAuditMu sync.Mutex

func (s *Server) auditBackupResults(ctx context.Context) {
	backupAuditMu.Lock()
	defer backupAuditMu.Unlock()
	if s.pool == nil || !s.agent.Configured() {
		return
	}
	st, err := s.agent.Backups(ctx)
	if err != nil {
		return
	}
	org := s.serverOrg(ctx)
	var ids []string
	// oldest first, so the newest result decides an incident's state
	for i := len(st.History) - 1; i >= 0; i-- {
		h := st.History[i]
		if h.Audited || h.Running {
			continue
		}
		result := map[bool]string{true: "ok", false: "failed"}[h.OK]
		actor := "server agent"
		if h.Actor != "" {
			actor = h.Actor + " (server agent)"
		}
		e := audit.Entry{OrgID: org, ActorType: "system", ActorName: actor, Action: runAction[h.Kind], EntityType: "server:backup " + h.File,
			New: h, Reason: h.Kind + " (" + h.Trigger + "): " + result}
		if e.Action == "" {
			e.Action = "server.backup_result"
		}
		if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
			s.log.Error("audit append failed", "security", true, "action", e.Action, "err", err)
			return
		}
		ids = append(ids, h.ID)
		s.backupIncident(ctx, org, h)
	}
	if len(ids) > 0 {
		_ = s.agent.AckBackups(ctx, ids)
	}
	// no completed backup for more than a day while the schedule is on (the agent stopped, the disk is full …)
	sc := st.Schedule
	if sc.Enabled && !sc.LastOK.IsZero() && time.Since(sc.LastOK) > 26*time.Hour && !st.Running {
		if !s.incidents.IsOpen(ctx, incBackupMissing) {
			s.log.Warn("no completed backup for more than a day", "security", true, "last_ok", sc.LastOK)
			_ = s.incidents.Raise(ctx, org, "backup_failed", incBackupMissing, "major", "No completed backup since "+sc.LastOK.Format("2006-01-02 15:04"),
				"If the server is lost now, the data since then is lost too.", map[string]any{"last_ok": sc.LastOK, "last_error": sc.LastError})
		}
	} else if sc.Enabled && !sc.LastOK.IsZero() && time.Since(sc.LastOK) <= 26*time.Hour {
		s.incidents.Resolve(ctx, incBackupMissing, "A backup completed again.")
	}
}

// backupIncident opens or resolves the incident of a run in the first organization of the installation, where the
// server administrators read the server's own audit trail (serverOrg).
func (s *Server) backupIncident(ctx context.Context, org string, h agent.BackupRun) {
	key := incBackupFailed
	if h.Kind == "restore_test" {
		key = incRestoreTest
	}
	if h.OK {
		s.incidents.Resolve(ctx, key, "The next "+strings.ReplaceAll(h.Kind, "_", " ")+" succeeded ("+h.ID+").")
		return
	}
	failed := ""
	for _, st := range h.Steps {
		if !st.OK {
			failed = st.Name
		}
	}
	title, impact := "Backup failed", "The server has no current backup."
	switch {
	case h.Kind == "restore_test":
		title, impact = "Restore test failed", "It is not proven that the backups can be restored."
	case failed == "upload" || failed == "verify" || failed == "sync" || failed == "prune_offsite" || h.Kind == "offsite_sync":
		title, impact = "Offsite copy of the backup failed", "The backup exists only on this server; losing the server loses it too."
	}
	if failed != "" {
		title += " (" + failed + ")"
	}
	s.log.Error(strings.ToLower(title), "security", true, "run", h.ID, "err", h.Error)
	_ = s.incidents.Raise(ctx, org, "backup_failed", key, "major", title, impact,
		map[string]any{"run": h.ID, "kind": h.Kind, "trigger": h.Trigger, "file": h.File, "target": h.Target, "error": h.Error, "step": failed})
}
