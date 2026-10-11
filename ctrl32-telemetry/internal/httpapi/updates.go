package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// Update modes and offline updates (ADR-0024, spec 031, 0.76.0), all system.admin:
//
//	GET  /api/v1/server/updates              mode, available release (signed?), next window, history of the agent
//	PUT  /api/v1/server/updates              {mode, weekday, time, tz, include_minor, rollback_minutes, reason}
//	POST /api/v1/server/updates/check        check the distribution point now
//	GET  /api/v1/server/updates/notice       the banner for system administrators (cached, no network)
//	POST /api/v1/server/update               {version, reason, backup, source}: install (source "package": the staged one)
//	POST /api/v1/server/update/package       multipart {reason, password, code, backup, install, file}: offline package
//
// Modes: manual (nothing happens by itself), notify (the default: a daily check, a banner for system administrators
// and an e-mail to the server administrators once per version) and auto (patch versions — minor ones when allowed,
// never major ones — installed in a weekly maintenance window with a backup first and the automatic rollback). The
// licence (updates_until) is respected everywhere: the agent refuses a release dated after it.

type updateSettings struct {
	Mode            string `json:"mode"`             // manual, notify, auto
	Weekday         int    `json:"weekday"`          // 0 = Sunday … 6 = Saturday
	Time            string `json:"time"`             // HH:MM, start of the one-hour window
	TZ              string `json:"tz"`               // IANA time zone; "" = the server's local time
	IncludeMinor    bool   `json:"include_minor"`    // automatic mode: also minor versions
	RollbackMinutes int    `json:"rollback_minutes"` // the new version must answer healthy within this time
	// bookkeeping of the scheduler
	NotifiedVersion string    `json:"notified_version,omitempty"`
	AutoVersion     string    `json:"auto_version,omitempty"` // last version the automatic mode tried
	AutoAt          time.Time `json:"auto_at,omitzero"`
}

func defaultUpdateSettings() updateSettings {
	return updateSettings{Mode: "notify", Weekday: 0, Time: "03:00", RollbackMinutes: 10}
}

// availInfo is the latest release on the distribution point.
type availInfo struct {
	Version   string    `json:"version"`
	Released  string    `json:"released,omitempty"`
	KeyID     string    `json:"key_id,omitempty"`
	Signed    bool      `json:"signed"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

type updatesRuntime struct {
	mu        sync.Mutex
	avail     availInfo
	nextCheck time.Time
	auditMu   sync.Mutex // one pass over the agent's results at a time
}

func (s *Server) updateURL() string {
	if u := strings.TrimRight(s.cfg.Agent.UpdateURL, "/"); u != "" {
		return u
	}
	return "https://portal.telemetry.digital/dl"
}

func (s *Server) loadUpdateSettings(ctx context.Context) updateSettings {
	st := defaultUpdateSettings()
	if s.pool == nil {
		return st
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT data FROM core.server_setting WHERE key = 'updates'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	if st.RollbackMinutes <= 0 {
		st.RollbackMinutes = 10
	}
	return st
}

func (s *Server) saveUpdateSettings(ctx context.Context, st updateSettings, userID string) error {
	b, _ := json.Marshal(st)
	_, err := s.pool.Exec(ctx, `INSERT INTO core.server_setting (key, data, updated_at, updated_by) VALUES ('updates', $1::jsonb, now(), NULLIF($2,'')::uuid)
		ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data, updated_at = now(), updated_by = COALESCE(EXCLUDED.updated_by, core.server_setting.updated_by)`, string(b), userID)
	return err
}

// validateUpdateSettings checks what a person may set.
func validateUpdateSettings(st updateSettings) error {
	switch st.Mode {
	case "manual", "notify", "auto":
	default:
		return errors.New("mode must be manual, notify or auto")
	}
	if st.Weekday < 0 || st.Weekday > 6 {
		return errors.New("weekday must be 0 (Sunday) to 6 (Saturday)")
	}
	if _, _, err := parseHHMM(st.Time); err != nil {
		return err
	}
	if st.TZ != "" {
		if _, err := time.LoadLocation(st.TZ); err != nil {
			return errors.New("unknown time zone")
		}
	}
	if st.RollbackMinutes < 2 || st.RollbackMinutes > 60 {
		return errors.New("rollback_minutes must be between 2 and 60")
	}
	return nil
}

func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, errors.New("time must be HH:MM")
	}
	return t.Hour(), t.Minute(), nil
}

func (st updateSettings) location() *time.Location {
	if st.TZ != "" {
		if loc, err := time.LoadLocation(st.TZ); err == nil {
			return loc
		}
	}
	return time.Local
}

// windowLength is how long the maintenance window stays open after its start.
const windowLength = time.Hour

// inWindow reports whether now is inside the weekly maintenance window and returns the window's start. A window that
// starts late in the evening continues past midnight into the next day.
func inWindow(now time.Time, st updateSettings) (bool, time.Time) {
	h, m, err := parseHHMM(st.Time)
	if err != nil {
		return false, time.Time{}
	}
	loc := st.location()
	t := now.In(loc)
	for _, d := range []int{0, -1} {
		start := time.Date(t.Year(), t.Month(), t.Day()+d, h, m, 0, 0, loc)
		if int(start.Weekday()) == st.Weekday && !t.Before(start) && t.Before(start.Add(windowLength)) {
			return true, start
		}
	}
	return false, time.Time{}
}

// nextWindow is the start of the next maintenance window (the current one when now is inside it).
func nextWindow(now time.Time, st updateSettings) time.Time {
	if in, start := inWindow(now, st); in {
		return start
	}
	h, m, err := parseHHMM(st.Time)
	if err != nil {
		return time.Time{}
	}
	loc := st.location()
	t := now.In(loc)
	for d := 0; d <= 7; d++ {
		start := time.Date(t.Year(), t.Month(), t.Day()+d, h, m, 0, 0, loc)
		if int(start.Weekday()) == st.Weekday && start.After(t) {
			return start
		}
	}
	return time.Time{}
}

// autoAllowed decides whether the automatic mode may install the step from current to available.
func autoAllowed(current, available string, includeMinor bool) (bool, string) {
	switch release.Change(current, available) {
	case "patch":
		return true, ""
	case "minor":
		if includeMinor {
			return true, ""
		}
		return false, "a minor version: the automatic mode installs patch versions only (allow minor versions to include it)"
	case "major":
		return false, "a major version is installed manually"
	case "same", "older":
		return false, "up to date"
	}
	return false, "unknown version"
}

// licenceCovers reports whether a release of the given date is within the licence's updates (empty = no limit).
func licenceCovers(released, updatesUntil string) bool {
	if released == "" || updatesUntil == "" {
		return true
	}
	r, err1 := time.Parse(release.Layout, released)
	u, err2 := time.Parse(release.Layout, updatesUntil)
	if err1 != nil || err2 != nil {
		return true
	}
	return release.Covered(r, u)
}

// ---- checking the distribution point ------------------------------------------------------------------------------

// checkAvailable reads VERSION and the signed manifest of the distribution point.
func (s *Server) checkAvailable(ctx context.Context) availInfo {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := availInfo{CheckedAt: time.Now().UTC()}
	get := func(name string, max int64) ([]byte, int, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", s.updateURL()+"/"+name, nil)
		req.Header.Set("User-Agent", "ctrl32-telemetry/"+s.version)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(io.LimitReader(resp.Body, max))
		return b, resp.StatusCode, err
	}
	b, code, err := get("VERSION", 64)
	switch {
	case err != nil:
		out.Error = err.Error()
		return out
	case code != 200:
		out.Error = "HTTP " + strconv.Itoa(code)
		return out
	}
	out.Version = strings.TrimSpace(string(b))
	mb, code, err := get(release.ManifestFile, release.MaxManifestBytes)
	if err != nil || code != 200 {
		return out // a release before 0.76.0: no manifest, not signed
	}
	sb, _, _ := get(release.SignatureFile, 64<<10)
	m, keyID, verr := release.Verify(mb, sb)
	if verr != nil {
		out.Error = "signature: " + verr.Error()
		return out
	}
	if m.Version == out.Version {
		out.Released, out.KeyID, out.Signed = m.Released, keyID, true
	}
	return out
}

// available returns the last check, refreshing it when it is older than maxAge.
func (s *Server) available(ctx context.Context, maxAge time.Duration) availInfo {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	if !s.upd.avail.CheckedAt.IsZero() && time.Since(s.upd.avail.CheckedAt) < maxAge {
		return s.upd.avail
	}
	s.upd.avail = s.checkAvailable(ctx)
	return s.upd.avail
}

// availableVersion is the summary of GET /api/v1/server (10-minute cache, as before 0.76.0).
func (s *Server) availableVersion(ctx context.Context) (string, time.Time, string) {
	a := s.available(ctx, 10*time.Minute)
	return a.Version, a.CheckedAt, a.Error
}

// ---- the scheduler ------------------------------------------------------------------------------------------------

func (s *Server) updatesLoop() {
	// first check a few minutes after the start, then once a day with up to an hour of jitter
	s.upd.mu.Lock()
	s.upd.nextCheck = time.Now().Add(time.Duration(60+rand.IntN(540)) * time.Second)
	s.upd.mu.Unlock()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
		s.updateTick(ctx, time.Now())
		cancel()
	}
}

func (s *Server) updateTick(ctx context.Context, now time.Time) {
	s.auditUpdateResults(ctx)
	s.auditBackupResults(ctx)
	st := s.loadUpdateSettings(ctx)
	if st.Mode == "manual" {
		return
	}
	s.upd.mu.Lock()
	due := !now.Before(s.upd.nextCheck)
	if due {
		s.upd.nextCheck = now.Add(24*time.Hour + time.Duration(rand.IntN(3600))*time.Second)
	}
	s.upd.mu.Unlock()
	var av availInfo
	if due {
		s.upd.mu.Lock()
		s.upd.avail = s.checkAvailable(ctx)
		av = s.upd.avail
		s.upd.mu.Unlock()
		s.log.Info("update check", "available", av.Version, "signed", av.Signed, "error", av.Error)
	} else {
		av = s.available(ctx, 25*time.Hour)
	}
	if av.Version == "" || release.Compare(s.version, av.Version) >= 0 {
		return
	}
	if st.NotifiedVersion != av.Version {
		st.NotifiedVersion = av.Version
		if err := s.saveUpdateSettings(ctx, st, ""); err == nil {
			s.mailAdmins(ctx, productName+": version "+av.Version+" is available",
				"Version "+av.Version+" of the server is available (installed: "+s.version+").\n\n"+
					"Install it under System → Updates: "+strings.TrimRight(s.cfg.HTTP.BaseURL, "/")+"/server/updates\n"+
					modeNote(st)+"\n")
		}
	}
	if st.Mode != "auto" {
		return
	}
	if ok, _ := autoAllowed(s.version, av.Version, st.IncludeMinor); !ok || st.AutoVersion == av.Version {
		return // not this kind of step, or already tried (a failed version is not retried automatically)
	}
	if !av.Signed {
		return
	}
	until := s.updatesUntil(ctx)
	if !licenceCovers(av.Released, until) {
		return
	}
	if in, _ := inWindow(now, st); !in {
		return
	}
	st.AutoVersion, st.AutoAt = av.Version, now.UTC()
	if err := s.saveUpdateSettings(ctx, st, ""); err != nil {
		return
	}
	s.log.Info("automatic update in the maintenance window", "from", s.version, "to", av.Version)
	reason := "automatic update in the maintenance window"
	res, err := s.agent.Update(ctx, agent.UpdateRequest{Version: av.Version, UpdatesUntil: until, Mode: "auto", Backup: true, RollbackMinutes: st.RollbackMinutes, Actor: "automatic", Reason: reason})
	e := audit.Entry{OrgID: s.serverOrg(ctx), ActorType: "system", ActorName: "automatic updates", Action: "server.update", EntityType: "server:" + av.Version,
		Old: map[string]any{"version": s.version}, Reason: reason}
	if err != nil {
		e.New = map[string]any{"version": av.Version, "mode": "auto", "error": err.Error()}
		s.mailAdmins(ctx, productName+": the automatic update to "+av.Version+" failed",
			"The automatic update from "+s.version+" to "+av.Version+" failed:\n\n"+err.Error()+"\n\nThe server keeps running "+s.version+". Details: System → Updates.\n")
	} else {
		e.New = map[string]any{"version": av.Version, "mode": "auto", "id": res.ID, "sha256": res.SHA256, "key_id": res.KeyID, "backup": res.Backup}
	}
	if _, _, aerr := audit.Append(ctx, s.pool, e); aerr != nil {
		s.log.Error("audit append failed", "security", true, "action", e.Action, "err", aerr)
	}
}

func modeNote(st updateSettings) string {
	if st.Mode == "auto" {
		return "Automatic updates are on: patch versions" + map[bool]string{true: " and minor versions", false: ""}[st.IncludeMinor] + " are installed in the maintenance window."
	}
	return "This server installs updates when a system administrator starts them (mode: notify)."
}

// auditUpdateResults writes the finished installations of the agent (also those of the CLI and of the guard after a
// restart) into the audit trail once, and mails the server administrators about a rollback or a failure.
func (s *Server) auditUpdateResults(ctx context.Context) {
	s.upd.auditMu.Lock()
	defer s.upd.auditMu.Unlock()
	if s.pool == nil || !s.agent.Configured() {
		return
	}
	st, err := s.agent.UpdateState(ctx)
	if err != nil {
		return
	}
	var ids []string
	for _, h := range st.History {
		if h.Audited || (h.Phase != "done" && h.Phase != "rolled_back" && h.Phase != "failed") {
			continue
		}
		e := audit.Entry{OrgID: s.serverOrg(ctx), ActorType: "system", ActorName: "server agent", Action: "server.update_result", EntityType: "server:" + h.To,
			Old: map[string]any{"version": h.From}, New: h, Reason: "result of the update " + h.ID + " (" + h.Mode + ", " + h.Source + "): " + h.Phase}
		if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
			s.log.Error("audit append failed", "security", true, "action", e.Action, "err", err)
			return
		}
		ids = append(ids, h.ID)
		if h.Phase != "done" {
			s.mailAdmins(ctx, productName+": update to "+h.To+" "+strings.ReplaceAll(h.Phase, "_", " "),
				"The update from "+h.From+" to "+h.To+" ("+h.Mode+") ended: "+h.Phase+".\n\n"+h.Error+"\n\nDetails: System → Updates.\n")
		}
	}
	if len(ids) > 0 {
		_ = s.agent.AckUpdates(ctx, ids)
	}
}

// serverOrg is the first organization of the installation: the audit trail of the server's own actions (automatic
// updates, results of the agent) goes there, where the server administrators read it.
func (s *Server) serverOrg(ctx context.Context) string {
	var id string
	_ = s.pool.QueryRow(ctx, `SELECT id::text FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&id)
	return id
}

// mailAdmins sends a notice of the server to every server administrator with an e-mail address (when SMTP works) and
// records each delivery in the delivery log.
func (s *Server) mailAdmins(ctx context.Context, subject, body string) {
	if s.notifier == nil || !s.notifier.Available() || s.iam == nil {
		return
	}
	list, err := s.iam.ServerAdminContacts(ctx)
	if err != nil {
		return
	}
	for _, c := range list {
		err := s.notifier.SendTo(ctx, c.Email, subject, body)
		status, errText := "sent", ""
		if err != nil {
			status, errText = "failed", err.Error()
		}
		payload, _ := json.Marshal(map[string]any{"subject": subject, "to": c.Email, "kind": "server_notice"})
		_, _ = s.pool.Exec(ctx, `INSERT INTO core.notification (org_id, event, status, sent_at, attempts, error, payload) VALUES ($1::uuid, 'system', $2, now(), 1, NULLIF($3,''), $4::jsonb)`,
			c.OrgID, status, errText, string(payload))
	}
}

// ---- handlers -----------------------------------------------------------------------------------------------------

func (s *Server) updatesState(ctx context.Context, refresh bool) map[string]any {
	st := s.loadUpdateSettings(ctx)
	maxAge := 10 * time.Minute
	if refresh {
		maxAge = 0
	}
	av := s.available(ctx, maxAge)
	until := s.updatesUntil(ctx)
	out := map[string]any{"current": s.version, "release_date": release.String(), "url": s.updateURL(), "available": av,
		"settings":  map[string]any{"mode": st.Mode, "weekday": st.Weekday, "time": st.Time, "tz": st.TZ, "include_minor": st.IncludeMinor, "rollback_minutes": st.RollbackMinutes},
		"server_tz": time.Local.String()}
	for k, v := range s.updatesJSON(ctx) {
		out[k] = v
	}
	if av.Version != "" {
		ch := release.Change(s.version, av.Version)
		ok, why := autoAllowed(s.version, av.Version, st.IncludeMinor)
		out["change"], out["auto_allowed"], out["auto_note"], out["licence_ok"] = ch, ok, why, licenceCovers(av.Released, until)
		out["newer"] = release.Compare(s.version, av.Version) < 0
	}
	if st.Mode == "auto" {
		w := nextWindow(time.Now(), st)
		out["next_window"] = w
		if st.AutoVersion != "" {
			out["auto_last"] = map[string]any{"version": st.AutoVersion, "at": st.AutoAt}
		}
	}
	s.upd.mu.Lock()
	if !s.upd.nextCheck.IsZero() && st.Mode != "manual" {
		out["next_check"] = s.upd.nextCheck
	}
	s.upd.mu.Unlock()
	ag := map[string]any{"configured": s.agent.Configured(), "connected": false}
	if s.agent.Configured() {
		if as, err := s.agent.UpdateState(ctx); err != nil {
			ag["error"] = err.Error()
		} else {
			ag["connected"] = true
			out["state"] = as
		}
	}
	out["agent"] = ag
	if s.pool != nil {
		var org string
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(tz, '') FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&org)
		out["org_tz"] = org
	}
	return out
}

func (s *Server) serverUpdates(w http.ResponseWriter, r *http.Request) {
	s.auditUpdateResults(r.Context()) // results finished since the last tick are in the trail before they are shown
	writeJSON(w, 200, s.updatesState(r.Context(), false))
}

func (s *Server) serverUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.updatesState(r.Context(), true))
}

func (s *Server) serverUpdatesSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode            string `json:"mode"`
		Weekday         *int   `json:"weekday"`
		Time            string `json:"time"`
		TZ              string `json:"tz"`
		IncludeMinor    bool   `json:"include_minor"`
		RollbackMinutes int    `json:"rollback_minutes"`
		Reason          string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	old := s.loadUpdateSettings(r.Context())
	st := old
	st.Mode, st.TZ, st.IncludeMinor = in.Mode, strings.TrimSpace(in.TZ), in.IncludeMinor
	if in.Weekday != nil {
		st.Weekday = *in.Weekday
	}
	if in.Time != "" {
		st.Time = in.Time
	}
	if in.RollbackMinutes != 0 {
		st.RollbackMinutes = in.RollbackMinutes
	}
	if err := validateUpdateSettings(st); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p := PrincipalFrom(r.Context())
	if err := s.saveUpdateSettings(r.Context(), st, p.UserID); err != nil {
		writeErr(w, 500, "could not save the update mode")
		return
	}
	view := func(x updateSettings) map[string]any {
		return map[string]any{"mode": x.Mode, "weekday": x.Weekday, "time": x.Time, "tz": x.TZ, "include_minor": x.IncludeMinor, "rollback_minutes": x.RollbackMinutes}
	}
	s.audit(r, "server.update_settings", "server", "updates", view(old), view(st), in.Reason)
	writeJSON(w, 200, s.updatesState(r.Context(), false))
}

// serverUpdateNotice is the banner for system administrators: a newer version when the mode is not manual.
func (s *Server) serverUpdateNotice(w http.ResponseWriter, r *http.Request) {
	st := s.loadUpdateSettings(r.Context())
	s.upd.mu.Lock()
	av := s.upd.avail
	s.upd.mu.Unlock()
	if st.Mode == "manual" || av.Version == "" || release.Compare(s.version, av.Version) >= 0 {
		writeJSON(w, 200, map[string]any{})
		return
	}
	out := map[string]any{"version": av.Version, "current": s.version, "mode": st.Mode, "signed": av.Signed}
	if st.Mode == "auto" {
		if ok, _ := autoAllowed(s.version, av.Version, st.IncludeMinor); ok {
			out["window"] = nextWindow(time.Now(), st)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) serverUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version, Reason, Source string
		Backup                  bool
	}
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Version = strings.TrimSpace(in.Version)
	if strings.TrimSpace(in.Reason) == "" || (in.Version == "" && in.Source != "package") {
		writeErr(w, 400, "version and reason are required")
		return
	}
	if in.Version != "" && !release.ValidVersion(in.Version) || in.Source != "" && in.Source != "package" {
		writeErr(w, 400, "invalid version or source")
		return
	}
	// a backup takes minutes: the answer may come long after the default write timeout
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(60 * time.Minute))
	// a licensed server installs only the versions released while its licence includes updates; the agent checks the
	// signed release date (servers without a licence always update)
	until := s.updatesUntil(r.Context())
	st := s.loadUpdateSettings(r.Context())
	p := PrincipalFrom(r.Context())
	res, err := s.agent.Update(r.Context(), agent.UpdateRequest{Version: in.Version, UpdatesUntil: until, Mode: "manual", Source: in.Source, Backup: in.Backup,
		RollbackMinutes: st.RollbackMinutes, Actor: p.Username, Reason: in.Reason})
	if err != nil {
		var ce *agent.CodedError
		if errors.As(err, &ce) {
			s.audit(r, "server.update", "server", in.Version, map[string]any{"version": s.version}, map[string]any{"version": in.Version, "source": in.Source, "refused": ce.Msg, "code": ce.Code}, in.Reason)
			status := ce.Status
			if status < 400 || status > 599 || status == 500 && ce.Code == "" {
				status = 502
			}
			writeJSON(w, status, map[string]string{"error": ce.Msg, "code": ce.Code})
			return
		}
		if !errors.Is(err, agent.ErrNoAgent) {
			s.audit(r, "server.update", "server", in.Version, map[string]any{"version": s.version}, map[string]any{"version": in.Version, "source": in.Source, "refused": err.Error()}, in.Reason)
		}
		s.agentErr(w, err)
		return
	}
	s.audit(r, "server.update", "server", res.Version, map[string]any{"version": s.version},
		map[string]any{"version": res.Version, "sha256": res.SHA256, "key_id": res.KeyID, "backup": res.Backup, "id": res.ID, "source": in.Source}, in.Reason)
	writeJSON(w, 200, res)
}

// serverUpdatePackage receives an offline package from the browser and streams it to the agent. The form fields come
// before the file: the reason and the password (and the authenticator code) are checked before a byte of the file is
// passed on. Only a person in the browser may do it (not an API token, not an AI assistant).
func (s *Server) serverUpdatePackage(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.SessionID == "" || p.TokenID != "" || p.GrantID != "" || r.Context().Value(viaMCPKey) != nil {
		writeJSON(w, 403, map[string]string{"error": "an update package is uploaded by a person in the browser, not with an API token or an AI assistant", "code": "session_required"})
		return
	}
	if !s.agent.Configured() {
		s.agentErr(w, agent.ErrNoAgent)
		return
	}
	limit := int64(s.cfg.Agent.MaxPackageMB)
	if limit <= 0 {
		limit = 2048
	}
	limit <<= 20
	if r.ContentLength > limit+(1<<20) {
		writeJSON(w, 413, map[string]string{"error": fmt.Sprintf("the package is larger than %d MB (agent.max_package_mb)", limit>>20), "code": "too_large"})
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(90 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(120 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "multipart/form-data expected")
		return
	}
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err != nil {
			writeErr(w, 400, "the form has no file")
			return
		}
		if part.FormName() != "file" {
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			fields[part.FormName()] = string(b)
			continue
		}
		reason := strings.TrimSpace(fields["reason"])
		if !reasonOf(w, reason) || !s.stepUp(w, r, fields["password"], fields["code"]) {
			return
		}
		q := url.Values{"mode": {"manual"}, "actor": {p.Username}, "reason": {reason}}
		if fields["install"] != "0" {
			q.Set("install", "1")
		}
		if fields["backup"] == "1" || fields["backup"] == "true" || fields["backup"] == "on" {
			q.Set("backup", "1")
		}
		st := s.loadUpdateSettings(r.Context())
		q.Set("rollback_minutes", strconv.Itoa(st.RollbackMinutes))
		if u := s.updatesUntil(r.Context()); u != "" {
			q.Set("updates_until", u)
		}
		res, err := s.agent.UploadPackage(r.Context(), part, -1, q)
		if err != nil {
			var ce *agent.CodedError
			msg, code, status := err.Error(), "", 502
			if errors.As(err, &ce) {
				msg, code, status = ce.Msg, ce.Code, ce.Status
			}
			s.audit(r, "server.update_package", "server", "package", nil, map[string]any{"file": part.FileName(), "refused": msg, "code": code}, reason)
			writeJSON(w, status, map[string]string{"error": msg, "code": code})
			return
		}
		n := map[string]any{"file": part.FileName(), "version": res.Package.Version, "released": res.Package.Released, "key_id": res.Package.KeyID,
			"files": res.Package.Files, "bytes": res.Package.Bytes, "has_binary": res.Package.HasBinary, "detector": res.Package.Detector}
		if res.Update != nil {
			n["installed"] = map[string]any{"version": res.Update.Version, "sha256": res.Update.SHA256, "id": res.Update.ID, "backup": res.Update.Backup}
		}
		s.audit(r, "server.update_package", "server", "package", map[string]any{"version": s.version}, n, reason)
		writeJSON(w, 200, res)
		return
	}
}
