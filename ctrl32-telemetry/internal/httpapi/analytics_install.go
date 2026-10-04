package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// Installation of the object counting detector from the user interface (0.75.0, ADR-0022): the button "Install the
// detector" on Object counting → Cameras and System → Server. The privileged server agent downloads the pinned parts
// (internal/analytics/pins.go) and writes [analytics]; this process then uses the new paths without a restart.
//
//	GET    /api/v1/counting/install   system.admin   agent, platform, pins, last installation, running or last job
//	POST   /api/v1/counting/install   system.admin   {reason, password, code, force}: install (force: repair, every part again)
//	DELETE /api/v1/counting/install   system.admin   {reason, password, code}: remove the parts and their paths
//
// The agent downloads and runs programs on the host, so the mutations ask for the password (and the authenticator
// code) again, only from a browser session (never an API token or an AI assistant), and are audited
// (analytics.install, analytics.remove: the request and its result).

// anJobs remembers which finished job this process has applied and who started the running one.
type anJobs struct {
	mu      sync.Mutex
	applied string                 // id of the last finished job applied to the counting engine
	started map[string]audit.Entry // who started a job of this process, for the audit entry of its result
}

func (s *Server) analyticsInstallState(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"agent": map[string]any{"configured": s.agent.Configured(), "connected": false}}
	if !s.agent.Configured() {
		writeJSON(w, 200, out)
		return
	}
	st, err := s.agent.Analytics(r.Context())
	if err != nil {
		out["agent"] = map[string]any{"configured": true, "connected": false, "error": err.Error()}
		writeJSON(w, 200, out)
		return
	}
	s.applyAnalyticsJob(st)
	out["agent"] = map[string]any{"configured": true, "connected": true}
	out["supported"], out["platform"], out["simulated"], out["pins"], out["installed"], out["job"] = st.Supported, st.Platform, st.Simulated, st.Pins, st.Installed, st.Job
	writeJSON(w, 200, out)
}

// applyAnalyticsJob hands the paths of a finished job to the counting engine once (the watcher or a poll, whichever
// sees it first); a failed job only ends the pause of the camera workers.
func (s *Server) applyAnalyticsJob(st *agent.AnalyticsState) {
	if st == nil || st.Job == nil || st.Job.State == "running" || s.counting == nil {
		return
	}
	s.anJobs.mu.Lock()
	defer s.anJobs.mu.Unlock()
	if s.anJobs.applied == st.Job.ID {
		return
	}
	s.anJobs.applied = st.Job.ID
	if e, ok := s.anJobs.started[st.Job.ID]; ok {
		delete(s.anJobs.started, st.Job.ID)
		e.New = map[string]any{"state": st.Job.State, "job": st.Job.ID, "installed": st.Job.Installed, "kept": st.Job.Kept, "error": st.Job.Error}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
			s.log.Error("audit append failed", "security", true, "action", e.Action, "err", err)
		}
		cancel()
	}
	switch {
	case st.Job.State == "done" && st.Job.Action == "install" && st.Job.Config != nil:
		c := st.Job.Config
		s.counting.SetAnalytics(c.FFmpeg, c.ONNXRuntime, c.ModelNano, c.ModelTiny, c.NanoSHA256, c.TinySHA256)
	case st.Job.State == "done" && st.Job.Action == "remove":
		s.counting.SetAnalytics("", "", "", "", "", "")
	default:
		s.counting.SetInstalling(false)
	}
	s.log.Info("detector job applied", "action", st.Job.Action, "state", st.Job.State, "installed", st.Job.Installed, "error", st.Job.Error)
}

// analyticsMutation checks a request that makes the agent install or remove programs: a person in the browser, a
// reason, the password (and authenticator code) again.
func (s *Server) analyticsMutation(w http.ResponseWriter, r *http.Request) (reason string, force bool, ok bool) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.SessionID == "" || p.TokenID != "" || p.GrantID != "" || r.Context().Value(viaMCPKey) != nil {
		writeJSON(w, 403, map[string]string{"error": "the detector is installed by a person in the browser, not with an API token or an AI assistant", "code": "session_required"})
		return "", false, false
	}
	var in struct {
		Reason, Password, Code string
		Force                  bool
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return "", false, false
	}
	if !reasonOf(w, in.Reason) || !s.stepUp(w, r, in.Password, in.Code) {
		return "", false, false
	}
	if !s.agent.Configured() {
		s.agentErr(w, agent.ErrNoAgent)
		return "", false, false
	}
	return strings.TrimSpace(in.Reason), in.Force, true
}

func (s *Server) analyticsInstall(w http.ResponseWriter, r *http.Request) {
	reason, force, ok := s.analyticsMutation(w, r)
	if !ok {
		return
	}
	s.startAnalyticsJob(w, r, "install", reason, force)
}

func (s *Server) analyticsRemove(w http.ResponseWriter, r *http.Request) {
	reason, _, ok := s.analyticsMutation(w, r)
	if !ok {
		return
	}
	s.startAnalyticsJob(w, r, "remove", reason, false)
}

func (s *Server) startAnalyticsJob(w http.ResponseWriter, r *http.Request, action, reason string, force bool) {
	if prev, err := s.agent.Analytics(r.Context()); err == nil {
		s.applyAnalyticsJob(prev) // a finished job nobody has seen yet is applied (and its result audited) first
	}
	if s.counting != nil {
		s.counting.SetInstalling(true) // no camera worker holds the old parts while they are replaced
	}
	var st *agent.AnalyticsState
	var err error
	if action == "install" {
		st, err = s.agent.InstallAnalytics(r.Context(), force)
	} else {
		st, err = s.agent.RemoveAnalytics(r.Context())
	}
	if err != nil {
		var ce *agent.CodedError
		if errors.As(err, &ce) && ce.Code == "busy" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": ce.Msg, "code": ce.Code}) // the running job keeps the pause
			return
		}
		if s.counting != nil {
			s.counting.SetInstalling(false)
		}
		s.agentErr(w, err)
		return
	}
	jobID := ""
	if st.Job != nil {
		jobID = st.Job.ID
	}
	s.audit(r, "analytics."+action, "server", "analytics", nil, map[string]any{"state": "started", "job": jobID, "force": force, "platform": st.Platform,
		"pins": st.Pins, "simulated": st.Simulated}, reason)
	// the result is audited when the job is applied: by the watcher (even when nobody keeps the page open) or by a poll
	p := PrincipalFrom(r.Context())
	s.anJobs.mu.Lock()
	if s.anJobs.started == nil {
		s.anJobs.started = map[string]audit.Entry{}
	}
	s.anJobs.started[jobID] = audit.Entry{OrgID: p.OrgID, ActorID: p.UserID, ActorName: p.Username, ActorType: "user", RequestID: RequestID(r.Context()),
		Action: "analytics." + action, EntityType: "server:analytics", Reason: reason + " (result)"}
	s.anJobs.mu.Unlock()
	go s.watchAnalyticsJob(jobID)
	writeJSON(w, 202, map[string]any{"agent": map[string]any{"configured": true, "connected": true}, "supported": st.Supported, "platform": st.Platform,
		"simulated": st.Simulated, "pins": st.Pins, "installed": st.Installed, "job": st.Job})
}

// watchAnalyticsJob follows a job until it ends (or a newer one replaced it) and applies it.
func (s *Server) watchAnalyticsJob(jobID string) {
	deadline := time.Now().Add(50 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		st, err := s.agent.Analytics(ctx)
		cancel()
		if err != nil || st.Job == nil {
			continue
		}
		if st.Job.ID != jobID || st.Job.State != "running" {
			s.applyAnalyticsJob(st)
			return
		}
	}
	if s.counting != nil {
		s.counting.SetInstalling(false)
	}
}
