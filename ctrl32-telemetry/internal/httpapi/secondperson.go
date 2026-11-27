package httpapi

// Four eyes everywhere (spec 040, 0.94.0, ADR-0026 step 2). Everything here is off until the organization switches it
// on in its security policy, area by area.
//
// Two kinds of work:
//
//   - A single action — a command, a recipe load, an alarm acknowledgement or shelf, device attributes, an alarm limit,
//     an assignment — is held (202, status pending) and executed when a different user approves it.
//   - Continuous work — moving a camera, viewing and exporting recordings, controlling displays — is refused (403,
//     code second_person_required) until a second person approves it once for a limited time on that station
//     (policy four_eyes_minutes); every press is still logged.
//
// Both can be approved in two ways: from the list of what waits, by the second person in their own session, or on the
// spot — the second person types their name and password (and the code of their authenticator app) at the station of
// the person who asked. The server checks the credentials, never creates a session from them, and runs the same
// approval as the list would, so the record carries both names. The person who asked can never approve.
//
//	GET    /api/v1/second-person/waiting                       everything that waits for a second person
//	POST   /api/v1/second-person/approve {kind, id, device_id, username, password, code, reason}   on the spot
//	POST   /api/v1/second-person/grants {area, reason}         ask for a time-limited approval
//	GET    /api/v1/second-person/grants?status=&mine=1
//	POST   /api/v1/second-person/grants/{id}/approve {reason}  (a different user with a permission of the area)
//	POST   /api/v1/second-person/grants/{id}/reject  {reason}
//	DELETE /api/v1/second-person/grants/{id}                   end it early

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// secondAreas: the areas approved for a limited time, and the permissions of which the approver needs one.
var secondAreas = map[string][]string{
	"cameras":  {"video.ptz", "video.playback", "video.evidence", "video.unmask"},
	"displays": {"display.control", "display.manage"},
}

func hasAny(p *iam.Principal, perms []string) bool {
	for _, x := range perms {
		if p.Has(x) {
			return true
		}
	}
	return false
}

// fourEyesAreas is the state of every switch of the organization (for /auth/me and the pages).
func fourEyesAreas(pol iam.Policy) map[string]bool {
	return map[string]bool{"commands": pol.FourEyesCommands, "alarms": pol.FourEyesAlarms, "attributes": pol.FourEyesAttributes, "cameras": pol.FourEyesCameras,
		"displays": pol.FourEyesDisplays, "limits": pol.FourEyesLimits, "assignments": pol.FourEyesAssignments, "vpn": pol.FourEyesVPN}
}

type spotCtxKey struct{}

// spotInfo marks a request that runs an approval given on the spot: the approver's credentials were checked at the
// station of Requester.
type spotInfo struct{ Requester string }

func spotFrom(ctx context.Context) *spotInfo {
	si, _ := ctx.Value(spotCtxKey{}).(*spotInfo)
	return si
}

// station identifies where a person works: the browser session, or the API token.
func station(p *iam.Principal) string {
	if p.SessionID != "" {
		return p.SessionID
	}
	if p.TokenID != "" {
		return "token:" + p.TokenID
	}
	return ""
}

// activeGrant returns the second person of a running time-limited approval for the caller's station.
func (s *Server) activeGrant(ctx context.Context, p *iam.Principal, area string) (by string, until time.Time, ok bool) {
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(decided_by_name, ''), valid_until FROM core.second_person_grant
		WHERE org_id = $1::uuid AND user_id = NULLIF($2,'')::uuid AND station = $3 AND area = $4 AND status = 'active' AND valid_until > now()
		ORDER BY valid_until DESC LIMIT 1`, p.OrgID, p.UserID, station(p), area).Scan(&by, &until)
	return by, until, err == nil
}

// second wraps a handler of continuous work: with the area's policy on it runs only under a running approval.
func (s *Server) second(area string, fn http.HandlerFunc) http.HandlerFunc {
	return s.secondIf(area, nil, fn)
}

// secondIf is second for calls that need the second person only sometimes (a live picture without its masks).
func (s *Server) secondIf(area string, when func(*http.Request) bool, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		if p == nil || p.OrgID == "" || s.iam == nil || (when != nil && !when(r)) {
			fn(w, r)
			return
		}
		pol := s.iam.Policy(r.Context(), p.OrgID)
		if !fourEyesAreas(pol)[area] {
			fn(w, r)
			return
		}
		if _, _, ok := s.activeGrant(r.Context(), p, area); ok {
			fn(w, r)
			return
		}
		writeJSON(w, 403, map[string]any{"error": "four eyes: a second person must approve this (" + area + ")", "code": "second_person_required", "area": area, "minutes": pol.SecondPersonMinutes()})
	}
}

// expireGrants closes what ran out, so that lists show the truth.
func (s *Server) expireGrants(ctx context.Context, org string) {
	_, _ = s.pool.Exec(ctx, `UPDATE core.second_person_grant SET status = 'expired' WHERE org_id = $1::uuid AND ((status = 'pending' AND expires_at < now()) OR (status = 'active' AND valid_until < now()))`, org)
}

// POST /api/v1/second-person/grants {area, reason}
func (s *Server) requestGrant(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Area   string `json:"area"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	perms, known := secondAreas[in.Area]
	if !known {
		writeErr(w, 400, "area must be cameras or displays")
		return
	}
	if !hasAny(p, perms) {
		writeErr(w, 403, "you have no permission in this area")
		return
	}
	pol := s.iam.Policy(r.Context(), org)
	if !fourEyesAreas(pol)[in.Area] {
		writeErr(w, 409, "four eyes are not switched on for this area")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	s.expireGrants(r.Context(), org)
	var id, status string
	var until *time.Time
	var expires time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT id::text, status, valid_until, expires_at FROM core.second_person_grant
		WHERE org_id = $1::uuid AND user_id = NULLIF($2,'')::uuid AND station = $3 AND area = $4 AND status IN ('pending', 'active') ORDER BY requested_at DESC LIMIT 1`,
		org, p.UserID, station(p), in.Area).Scan(&id, &status, &until, &expires); err == nil {
		writeJSON(w, 200, map[string]any{"id": id, "area": in.Area, "status": status, "valid_until": until, "expires_at": expires, "minutes": pol.SecondPersonMinutes()})
		return
	}
	minutes := pol.SecondPersonMinutes()
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.second_person_grant (org_id, area, user_id, user_name, station, station_ip, reason, minutes)
		VALUES ($1::uuid, $2, NULLIF($3,'')::uuid, $4, $5, $6, $7, $8) RETURNING id::text, expires_at`, org, in.Area, p.UserID, p.Username, station(p), clientIP(r), strings.TrimSpace(in.Reason), minutes).Scan(&id, &expires); err != nil {
		s.log.Error("second person grant insert", "err", err)
		writeErr(w, 500, "request could not be recorded")
		return
	}
	s.audit(r, "second_person.request", "second_person_grant", id, nil, map[string]any{"area": in.Area, "minutes": minutes}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "area": in.Area, "status": "pending", "expires_at": expires, "minutes": minutes,
		"second_person": map[string]any{"kind": "grant", "id": id, "summary": grantSummary(in.Area, minutes)}})
}

func grantSummary(area string, minutes int) string {
	what := "cameras: movement, recordings, export, evidence, pictures without masks"
	if area == "displays" {
		what = "displays and kiosk PCs: commands"
	}
	return fmt.Sprintf("%s — for %d minutes on this station", what, minutes)
}

// GET /api/v1/second-person/grants?status=pending|active|all&mine=1
func (s *Server) listGrants(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	s.expireGrants(r.Context(), org)
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	mine := r.URL.Query().Get("mine") == "1"
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, area, COALESCE(user_id::text, ''), user_name, station, station_ip, reason, minutes, requested_at, expires_at, status, on_spot,
		decided_by_name, decided_at, decision_reason, valid_until, ended_at, ended_by_name
		FROM core.second_person_grant WHERE org_id = $1::uuid AND ($2 = 'all' OR status = $2) AND (NOT $3 OR (user_id = NULLIF($4,'')::uuid AND station = $5))
		ORDER BY requested_at DESC LIMIT 200`, org, status, mine, p.UserID, station(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, area, uid, uname, st, ip, reason, status string
		var minutes int
		var requested, expires time.Time
		var onSpot bool
		var decidedBy, decisionReason, endedBy *string
		var decided, until, ended *time.Time
		if rows.Scan(&id, &area, &uid, &uname, &st, &ip, &reason, &minutes, &requested, &expires, &status, &onSpot, &decidedBy, &decided, &decisionReason, &until, &ended, &endedBy) != nil {
			continue
		}
		own := uid == p.UserID
		if !own && !hasAny(p, secondAreas[area]) && !p.Has("audit.read") {
			continue
		}
		out = append(out, map[string]any{"id": id, "area": area, "user": uname, "station": ip, "this_station": own && st == station(p), "reason": reason, "minutes": minutes, "requested_at": requested, "expires_at": expires,
			"status": status, "on_spot": onSpot, "decided_by": decidedBy, "decided_at": decided, "decision_reason": decisionReason, "valid_until": until, "ended_at": ended, "ended_by": endedBy,
			"summary": grantSummary(area, minutes), "can_approve": status == "pending" && !own && hasAny(p, secondAreas[area]), "can_end": status == "active" && (own || hasAny(p, secondAreas[area]))})
	}
	writeJSON(w, 200, out)
}

func (s *Server) decideGrant(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := s.mustOrg(w, r)
		if !ok {
			return
		}
		p := PrincipalFrom(r.Context())
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
		var area, uid, uname, status string
		var minutes int
		var expires time.Time
		if err := s.pool.QueryRow(r.Context(), `SELECT area, COALESCE(user_id::text, ''), user_name, status, minutes, expires_at FROM core.second_person_grant WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).
			Scan(&area, &uid, &uname, &status, &minutes, &expires); err != nil {
			writeErr(w, 404, "request not found")
			return
		}
		if status != "pending" || expires.Before(time.Now()) {
			s.expireGrants(r.Context(), org)
			writeErr(w, 409, "the request is not waiting any more")
			return
		}
		if !hasAny(p, secondAreas[area]) {
			writeErr(w, 403, "a permission of this area is required")
			return
		}
		if !approve {
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.second_person_grant SET status = 'rejected', decided_by = NULLIF($2,'')::uuid, decided_by_name = $3, decided_at = now(), decision_reason = $4 WHERE id = $1::uuid AND status = 'pending'`, id, p.UserID, p.Username, in.Reason)
			s.audit(r, "second_person.reject", "second_person_grant", id, map[string]any{"status": "pending", "area": area, "requested_by": uname}, map[string]any{"status": "rejected", "area": area, "rejected_by": p.Username}, in.Reason)
			writeJSON(w, 200, map[string]any{"id": id, "status": "rejected"})
			return
		}
		if (uid != "" && uid == p.UserID) || (uid == "" && uname == p.Username) {
			writeErr(w, 403, "four eyes: a different user must approve this")
			return
		}
		var until time.Time
		onSpot := spotFrom(r.Context()) != nil
		if err := s.pool.QueryRow(r.Context(), `UPDATE core.second_person_grant SET status = 'active', on_spot = $5, decided_by = NULLIF($2,'')::uuid, decided_by_name = $3, decided_at = now(), decision_reason = $4,
			valid_until = now() + make_interval(mins => minutes) WHERE id = $1::uuid AND status = 'pending' RETURNING valid_until`, id, p.UserID, p.Username, in.Reason, onSpot).Scan(&until); err != nil {
			writeErr(w, 409, "the request is not waiting any more")
			return
		}
		s.audit(r, "second_person.approve", "second_person_grant", id, map[string]any{"status": "pending", "area": area, "requested_by": uname},
			map[string]any{"status": "active", "area": area, "approved_by": p.Username, "requested_by": uname, "minutes": minutes, "valid_until": until, "on_spot": onSpot}, in.Reason)
		writeJSON(w, 200, map[string]any{"id": id, "status": "active", "area": area, "valid_until": until, "minutes": minutes})
	}
}

// DELETE /api/v1/second-person/grants/{id}: the person who works under it, or anyone who could approve it, ends it.
func (s *Server) endGrant(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var area, uid, uname, status string
	if err := s.pool.QueryRow(r.Context(), `SELECT area, COALESCE(user_id::text, ''), user_name, status FROM core.second_person_grant WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&area, &uid, &uname, &status); err != nil {
		writeErr(w, 404, "request not found")
		return
	}
	if uid != p.UserID && !hasAny(p, secondAreas[area]) {
		writeErr(w, 403, "a permission of this area is required")
		return
	}
	if status != "active" && status != "pending" {
		writeErr(w, 409, "it has ended already")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.second_person_grant SET status = 'ended', ended_at = now(), ended_by_name = $2 WHERE id = $1::uuid AND status IN ('active', 'pending')`, id, p.Username)
	s.audit(r, "second_person.end", "second_person_grant", id, map[string]any{"status": status, "area": area, "requested_by": uname}, map[string]any{"status": "ended", "area": area, "ended_by": p.Username}, strings.TrimSpace(r.URL.Query().Get("reason")))
	writeJSON(w, 200, map[string]any{"id": id, "status": "ended"})
}

// GET /api/v1/second-person/waiting: everything in the organization that waits for a second person, oldest first.
func (s *Server) listWaiting(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	ctx := r.Context()
	s.expireGrants(ctx, org)
	_, _ = s.pool.Exec(ctx, `UPDATE core.change_request SET status = 'expired', decided_at = now() WHERE org_id = $1::uuid AND status = 'pending' AND expires_at < now()`, org)
	out := []map[string]any{}
	add := func(kind, id, device, summary, reason, by, byID string, at time.Time, exp *time.Time, can bool, base string) {
		own := (byID != "" && byID == p.UserID) || (byID == "" && by == p.Username)
		out = append(out, map[string]any{"kind": kind, "id": id, "device_id": device, "summary": summary, "reason": reason, "requested_by": by, "requested_at": at, "expires_at": exp,
			"own": own, "can_approve": can && !own, "can_reject": can, "approve": base + "/approve", "reject": base + "/reject"})
	}
	if p.Has("device.command") || p.Has("data.read") {
		rows, err := s.pool.Query(ctx, `SELECT c.id::text, c.device_id::text, d.name, c.method, c.args, c.reason, c.issued_by_name, COALESCE(c.issued_by::text, ''), c.issued_at, c.expires_at
			FROM core.device_command c JOIN core.device d ON d.id = c.device_id
			WHERE c.org_id = $1::uuid AND c.status = 'pending' AND (c.expires_at IS NULL OR c.expires_at > now()) ORDER BY c.issued_at LIMIT 200`, org)
		if err == nil {
			for rows.Next() {
				var id, dev, name, method, reason, by, byID string
				var args json.RawMessage
				var at time.Time
				var exp *time.Time
				if rows.Scan(&id, &dev, &name, &method, &args, &reason, &by, &byID, &at, &exp) == nil {
					add("command", id, dev, cmdSummary(name, method, args), reason, by, byID, at, exp, p.Has("device.command"), "/api/v1/devices/"+dev+"/commands/"+id)
				}
			}
			rows.Close()
		}
		rows, err = s.pool.Query(ctx, `SELECT id::text, device_id::text, recipe_name, record_name, target_name, reason, requested_by_name, COALESCE(requested_by::text, ''), requested_at
			FROM core.recipe_download WHERE org_id = $1::uuid AND status = 'pending' AND requested_at > now() - interval '24 hours' ORDER BY requested_at LIMIT 200`, org)
		if err == nil {
			for rows.Next() {
				var id, dev, recipe, record, target, reason, by, byID string
				var at time.Time
				if rows.Scan(&id, &dev, &recipe, &record, &target, &reason, &by, &byID, &at) == nil {
					add("recipe", id, dev, "recipe "+recipe+" / "+record+" → "+target, reason, by, byID, at, nil, p.Has("device.command"), "/api/v1/recipe-downloads/"+id)
				}
			}
			rows.Close()
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, kind, summary, reason, requested_by_name, COALESCE(requested_by::text, ''), requested_at, expires_at FROM core.change_request
		WHERE org_id = $1::uuid AND status = 'pending' ORDER BY requested_at LIMIT 200`, org)
	if err == nil {
		for rows.Next() {
			var id, kind, summary, reason, by, byID string
			var at, exp time.Time
			if rows.Scan(&id, &kind, &summary, &reason, &by, &byID, &at, &exp) == nil && (s.canApprove(p, kind) || byID == p.UserID) {
				add("change", id, "", summary, reason, by, byID, at, &exp, s.canApprove(p, kind), "/api/v1/change-requests/"+id)
				out[len(out)-1]["area"] = kind
			}
		}
		rows.Close()
	}
	rows, err = s.pool.Query(ctx, `SELECT id::text, area, minutes, reason, user_name, COALESCE(user_id::text, ''), requested_at, expires_at FROM core.second_person_grant
		WHERE org_id = $1::uuid AND status = 'pending' ORDER BY requested_at LIMIT 200`, org)
	if err == nil {
		for rows.Next() {
			var id, area, reason, by, byID string
			var minutes int
			var at, exp time.Time
			if rows.Scan(&id, &area, &minutes, &reason, &by, &byID, &at, &exp) == nil && (hasAny(p, secondAreas[area]) || byID == p.UserID) {
				add("grant", id, "", grantSummary(area, minutes), reason, by, byID, at, &exp, hasAny(p, secondAreas[area]), "/api/v1/second-person/grants/"+id)
				out[len(out)-1]["area"] = area
				out[len(out)-1]["minutes"] = minutes
			}
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{"waiting": out})
}

// POST /api/v1/second-person/approve: the second person approves on the spot, at the station of the person who asked.
// Only the approvals named by kind can be run this way — never a path chosen by the caller.
func (s *Server) approveOnSpot(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		DeviceID string `json:"device_id"`
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.ID, in.DeviceID = strings.ToLower(strings.TrimSpace(in.ID)), strings.ToLower(strings.TrimSpace(in.DeviceID))
	if !uuidRe.MatchString(in.ID) || (in.Kind == "command" && !uuidRe.MatchString(in.DeviceID)) {
		writeErr(w, 400, "id (and device_id for a command) must be a uuid")
		return
	}
	var path string
	switch in.Kind {
	case "command":
		path = "/api/v1/devices/" + in.DeviceID + "/commands/" + in.ID + "/approve"
	case "recipe":
		path = "/api/v1/recipe-downloads/" + in.ID + "/approve"
	case "change":
		path = "/api/v1/change-requests/" + in.ID + "/approve"
	case "grant":
		path = "/api/v1/second-person/grants/" + in.ID + "/approve"
	default:
		writeErr(w, 400, "kind must be command, recipe, change or grant")
		return
	}
	if strings.TrimSpace(in.Username) == "" || in.Password == "" {
		writeErr(w, 400, "the name and the password of the second person are required")
		return
	}
	if !s.stepUpLimiter.peek("second:" + p.UserID) {
		writeErr(w, 429, "too many failed approvals; wait a minute")
		return
	}
	ap, err := s.iam.Witness(r.Context(), org, in.Username, in.Password, in.Code, clientIP(r), r.UserAgent())
	if err != nil {
		s.stepUpLimiter.allow("second:" + p.UserID)
		s.log.Warn("second person not accepted", "security", true, "station_user", p.Username, "second", in.Username, "err", err, "remote", clientIP(r))
		switch {
		case errors.Is(err, iam.ErrMFACode):
			writeJSON(w, 403, map[string]string{"error": "the code from the second person's authenticator app is required", "code": "second_person_code"})
		case errors.Is(err, iam.ErrLocked):
			writeJSON(w, 403, map[string]string{"error": "the account of the second person is locked", "code": "second_person_failed"})
		default:
			writeJSON(w, 403, map[string]string{"error": "wrong name, password or code of the second person", "code": "second_person_failed"})
		}
		return
	}
	if ap.UserID == p.UserID {
		writeErr(w, 403, "four eyes: a different person must approve")
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "approved on the spot"
	}
	body, _ := json.Marshal(map[string]string{"reason": reason})
	ctx := context.WithValue(r.Context(), principalKey, ap)
	ctx = context.WithValue(ctx, spotCtxKey{}, &spotInfo{Requester: p.Username})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		writeErr(w, 500, "approval failed")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if v := r.Header.Get("X-Ctrl32-Screen"); v != "" {
		req.Header.Set("X-Ctrl32-Screen", v)
	}
	req.RemoteAddr = r.RemoteAddr
	req.Header.Set("X-Forwarded-For", r.Header.Get("X-Forwarded-For"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var result any
	if rec.Body.Len() > 0 && json.Valid(rec.Body.Bytes()) {
		result = json.RawMessage(rec.Body.Bytes())
	} else {
		result = rec.Body.String()
	}
	if rec.Code < 200 || rec.Code >= 300 {
		msg := "the approval was refused"
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &e) == nil && e.Error != "" {
			msg = e.Error
		}
		writeJSON(w, rec.Code, map[string]any{"error": msg, "approved_by": ap.Username})
		return
	}
	writeJSON(w, 200, map[string]any{"kind": in.Kind, "id": in.ID, "approved_by": ap.Username, "result": result})
}

// wantsUnmasked: a live picture asked for without its privacy masks.
func wantsUnmasked(r *http.Request) bool { return r.URL.Query().Get("unmask") == "1" }

// cmdSummary says in words what a held command does: "Motor M1: run → true" for a write of one key.
func cmdSummary(device, method string, args json.RawMessage) string {
	var a struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	out := method + " " + string(args)
	if json.Unmarshal(args, &a) == nil && a.Key != "" && len(a.Value) > 0 {
		out = a.Key + " → " + strings.Trim(string(a.Value), `"`)
		if method != "write" {
			out = method + " " + out
		}
	}
	if len(out) > 300 {
		out = out[:300] + "…"
	}
	if device != "" {
		out = device + ": " + out
	}
	return out
}
