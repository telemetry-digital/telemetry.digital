package httpapi

// Alarm management after ISA-18.2 (spec 035, 0.89.0): the alarm list with the ISA states and classes, acknowledging
// (also of alarms that returned to normal, one or many), shelving, the summary for the alarm bar of every page, the
// performance statistics and the alarm classes.

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/alarmmgmt"
)

const alarmCols = `a.id::text, a.raised_at, a.cleared_at, a.state, a.severity, a.retrospective, a.value::float8, a.detail->>'key', a.datastream_id::text,
	a.acked_at, a.acked_by, a.shelved_until, coalesce(c.code, ''), coalesce(c.name, ''), coalesce(c.color, '#d63939'), coalesce(c.priority, 1)::int,
	coalesce(c.ack_required, true), coalesce(c.sound, 'none'), coalesce(ast.name, ''), coalesce(d.unit, ''), coalesce(a.detail->>'type', ''),
	CASE WHEN a.severity = 'warning' THEN r.warning ELSE coalesce(r.action, r.warning) END::float8, coalesce(a.detail->>'rule', ''),
	coalesce(sh.by_name, ''), coalesce(sh.reason, '')`

const alarmFrom = ` FROM core.alarm a LEFT JOIN core.alarm_rule r ON r.id = a.rule_id` + alarmmgmt.ClassJoin + `
	LEFT JOIN core.datastream d ON d.id = a.datastream_id LEFT JOIN core.asset ast ON ast.id = d.asset_id
	LEFT JOIN core.alarm_shelf sh ON sh.released_at IS NULL AND coalesce(sh.rule_id, sh.automation_rule_id) = coalesce(a.rule_id, a.automation_rule_id) `

// attention: what the Current list and the alarm bar show.
const alarmAttention = ` a.shelved_until IS NULL AND a.state <> 'suppressed' AND (a.cleared_at IS NULL OR ` + alarmmgmt.NeedsAck + `) `

func scanAlarm(rows pgx.Rows) (map[string]any, error) {
	var id, state, sev, ccode, cname, ccolor, csound, asset, unit, typ, rule, shBy, shReason string
	var key, ds, ackedBy *string
	var raised time.Time
	var cleared, acked, shelved *time.Time
	var retro, ackReq bool
	var val, limit *float64
	var prio int
	if err := rows.Scan(&id, &raised, &cleared, &state, &sev, &retro, &val, &key, &ds, &acked, &ackedBy, &shelved, &ccode, &cname, &ccolor, &prio,
		&ackReq, &csound, &asset, &unit, &typ, &limit, &rule, &shBy, &shReason); err != nil {
		return nil, err
	}
	isShelved := state == "suppressed" || shelved != nil
	out := map[string]any{"id": id, "raised_at": raised, "cleared_at": cleared, "state": state, "severity": sev, "retrospective": retro, "value": val,
		"key": key, "datastream_id": ds, "acked_at": acked, "acked_by": ackedBy, "shelved_until": shelved,
		"isa_state": alarmmgmt.State(cleared != nil, acked != nil, ackReq, isShelved),
		"class":     map[string]any{"code": ccode, "name": cname, "color": ccolor, "priority": prio, "ack_required": ackReq, "sound": csound},
		"source":    map[string]any{"asset": asset, "unit": unit, "type": typ, "rule": rule}, "limit": limit}
	if isShelved {
		out["shelf"] = map[string]any{"by": shBy, "reason": shReason}
	}
	return out, nil
}

func (s *Server) alarms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	org := orgOf(PrincipalFrom(r.Context()))
	_ = alarmmgmt.EnsureClasses(r.Context(), s.pool, org)
	where, order, lim := `($2 = '' OR a.datastream_id = NULLIF($2,'')::uuid)`, `a.raised_at DESC`, 200
	args := []any{org, q.Get("datastream_id")}
	switch q.Get("view") {
	case "":
	case "current":
		where += ` AND` + alarmAttention
		order, lim = `c.priority NULLS LAST, `+alarmmgmt.NeedsAck+` DESC, a.raised_at DESC`, 500
	case "shelved":
		where += ` AND (a.state = 'suppressed' OR a.shelved_until IS NOT NULL)`
		lim = 500
	case "history":
		to := time.Now()
		from := to.Add(-24 * time.Hour)
		var err error
		if v := q.Get("from"); v != "" {
			if from, err = time.Parse(time.RFC3339, v); err != nil {
				writeErr(w, 400, "from must be RFC 3339")
				return
			}
		}
		if v := q.Get("to"); v != "" {
			if to, err = time.Parse(time.RFC3339, v); err != nil {
				writeErr(w, 400, "to must be RFC 3339")
				return
			}
		}
		args = append(args, from, to, q.Get("class"), strings.TrimSpace(q.Get("q")))
		where += ` AND a.raised_at >= $3 AND a.raised_at < $4 AND ($5 = '' OR c.code = $5)
			AND ($6 = '' OR a.detail->>'key' ILIKE '%' || $6 || '%' OR ast.name ILIKE '%' || $6 || '%' OR a.detail->>'rule' ILIKE '%' || $6 || '%')`
		lim = 1000
	default:
		writeErr(w, 400, "view must be current, shelved or history")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+alarmCols+alarmFrom+` WHERE ($1 = '' OR a.org_id = $1::uuid) AND `+where+` ORDER BY `+order+` LIMIT `+strconv.Itoa(lim), args...)
	if err != nil {
		s.log.Error("alarms query", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		a, err := scanAlarm(rows)
		if err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, a)
	}
	writeJSON(w, 200, out)
}

// alarmSummary feeds the alarm bar: how many alarms wait for an acknowledgement, how many are active, how many are
// shelved, and the most important unacknowledged one.
func (s *Server) alarmSummary(w http.ResponseWriter, r *http.Request) {
	org := orgOf(PrincipalFrom(r.Context()))
	_ = alarmmgmt.EnsureClasses(r.Context(), s.pool, org)
	var unack, active, shelved int
	if err := s.pool.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE `+alarmAttention+` AND `+alarmmgmt.NeedsAck+`),
		count(*) FILTER (WHERE `+alarmAttention+` AND a.cleared_at IS NULL), count(*) FILTER (WHERE a.state = 'suppressed')
		FROM core.alarm a LEFT JOIN core.alarm_rule r ON r.id = a.rule_id`+alarmmgmt.ClassJoin+`
		WHERE ($1 = '' OR a.org_id = $1::uuid) AND (a.cleared_at IS NULL OR a.acked_at IS NULL)`, org).Scan(&unack, &active, &shelved); err != nil {
		s.log.Error("alarm summary", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	out := map[string]any{"unack": unack, "active": active, "shelved": shelved, "top": nil}
	if unack > 0 {
		rows, err := s.pool.Query(r.Context(), `SELECT `+alarmCols+alarmFrom+` WHERE ($1 = '' OR a.org_id = $1::uuid) AND`+alarmAttention+` AND `+alarmmgmt.NeedsAck+`
			ORDER BY c.priority NULLS LAST, a.raised_at DESC LIMIT 1`, org)
		if err == nil {
			if rows.Next() {
				if a, err := scanAlarm(rows); err == nil {
					out["top"] = a
				}
			}
			rows.Close()
		}
	}
	writeJSON(w, 200, out)
}

// ackAlarms marks alarms acknowledged (active ones and ones that returned to normal) and writes the alarm log, the
// audit trail and the live events. Returns the number acknowledged.
func (s *Server) ackAlarms(r *http.Request, ids []string, reason string) (int, error) {
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `UPDATE core.alarm SET acked_at = now(), acked_by = $3, state = CASE WHEN state = 'active' THEN 'acknowledged' ELSE state END
		WHERE id = ANY($1::uuid[]) AND org_id = $2::uuid AND acked_at IS NULL
		RETURNING id::text, coalesce(datastream_id::text, ''), severity, value::float8, cleared_at IS NOT NULL`, ids, p.OrgID, p.Username)
	if err != nil {
		return 0, err
	}
	type acked struct {
		id, ds, sev string
		val         *float64
		cleared     bool
	}
	var done []acked
	for rows.Next() {
		var a acked
		if err := rows.Scan(&a.id, &a.ds, &a.sev, &a.val, &a.cleared); err != nil {
			rows.Close()
			return 0, err
		}
		done = append(done, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, a := range done {
		if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.alarm_event (alarm_id, event, actor_id, actor_name, reason) VALUES ($1::uuid, 'acknowledged', $2::uuid, $3, $4)`,
			a.id, p.UserID, p.Username, reason); err != nil {
			return 0, err
		}
		old := "active"
		if a.cleared {
			old = "cleared"
		}
		s.audit(r, "alarm.acknowledge", "alarm", a.id, map[string]any{"state": old}, map[string]any{"state": "acknowledged"}, reason)
		// live pages and the flows' Alarm trigger ("acknowledged") learn about it at once
		if s.live != nil {
			s.live.Alarm(p.OrgID, a.id, a.ds, "acknowledged", a.sev, a.val)
		}
	}
	return len(done), nil
}

func (s *Server) ackAlarm(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil || len(strings.TrimSpace(in.Reason)) < 3 {
		writeErr(w, 400, "reason required")
		return
	}
	id := r.PathValue("id")
	if !uuidRe.MatchString(strings.ToLower(id)) {
		writeErr(w, 404, "alarm not found")
		return
	}
	n, err := s.ackAlarms(r, []string{id}, strings.TrimSpace(in.Reason))
	if err != nil {
		writeErr(w, 500, "alarm log write failed")
		return
	}
	if n == 0 {
		writeErr(w, 404, "unacknowledged alarm not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ackAlarmsBulk acknowledges up to 500 alarms with one reason.
func (s *Server) ackAlarmsBulk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Reason string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil || len(strings.TrimSpace(in.Reason)) < 3 {
		writeErr(w, 400, "reason required")
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 500 {
		writeErr(w, 400, "ids: 1 to 500 alarms")
		return
	}
	for _, id := range in.IDs {
		if !uuidRe.MatchString(strings.ToLower(id)) {
			writeErr(w, 400, "ids must be alarm ids")
			return
		}
	}
	n, err := s.ackAlarms(r, in.IDs, strings.TrimSpace(in.Reason))
	if err != nil {
		writeErr(w, 500, "alarm log write failed")
		return
	}
	writeJSON(w, 200, map[string]any{"acknowledged": n})
}

func shelfErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, alarmmgmt.ErrNotFound):
		writeErr(w, 404, err.Error())
	case errors.Is(err, alarmmgmt.ErrAlreadyShelved), errors.Is(err, alarmmgmt.ErrNotShelved):
		writeErr(w, 409, err.Error())
	case errors.Is(err, alarmmgmt.ErrDuration), errors.Is(err, alarmmgmt.ErrReason):
		writeErr(w, 400, err.Error())
	default:
		writeErr(w, 500, "shelving failed")
	}
}

func (s *Server) liveChanged(ch []alarmmgmt.Changed) {
	if s.live == nil {
		return
	}
	for _, c := range ch {
		s.live.Alarm(c.OrgID, c.AlarmID, c.DatastreamID, c.State, c.Severity, c.Value)
	}
}

func (s *Server) shelveAlarm(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Minutes int    `json:"minutes"`
		Reason  string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 2048); err != nil || !uuidRe.MatchString(strings.ToLower(id)) {
		writeErr(w, 400, "minutes and reason required")
		return
	}
	until, ch, err := alarmmgmt.Shelve(r.Context(), s.pool, p.OrgID, id, p.UserID, p.Username, in.Reason, time.Duration(in.Minutes)*time.Minute)
	if err != nil {
		shelfErr(w, err)
		return
	}
	s.audit(r, "alarm.suppress", "alarm", id, nil, map[string]any{"shelved_until": until, "alarms": len(ch)}, strings.TrimSpace(in.Reason))
	s.liveChanged(ch)
	writeJSON(w, 200, map[string]any{"shelved_until": until, "alarms": len(ch)})
}

func (s *Server) unshelveAlarm(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 2048); err != nil || !uuidRe.MatchString(strings.ToLower(id)) {
		writeErr(w, 400, "reason required")
		return
	}
	ch, err := alarmmgmt.Unshelve(r.Context(), s.pool, p.OrgID, id, p.Username, in.Reason)
	if err != nil {
		shelfErr(w, err)
		return
	}
	s.audit(r, "alarm.unsuppress", "alarm", id, nil, map[string]any{"alarms": len(ch)}, strings.TrimSpace(in.Reason))
	s.liveChanged(ch)
	writeJSON(w, 200, map[string]any{"alarms": len(ch)})
}

// alarmStats: the ISA-18.2 performance numbers of a range (default the last 7 days, at most 92).
func (s *Server) alarmStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	org := orgOf(PrincipalFrom(r.Context()))
	to := time.Now()
	from := to.Add(-7 * 24 * time.Hour)
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			writeErr(w, 400, "from must be RFC 3339")
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			writeErr(w, 400, "to must be RFC 3339")
			return
		}
	}
	if !to.After(from) || to.Sub(from) > 92*24*time.Hour {
		writeErr(w, 400, "the range must be 1 second to 92 days")
		return
	}
	tz := q.Get("tz")
	if tz == "" || !tzRe.MatchString(tz) {
		tz = "UTC"
	} else if _, err := time.LoadLocation(tz); err != nil {
		tz = "UTC"
	}
	ctx := r.Context()
	fail := func(what string, err error) {
		s.log.Error("alarm statistics", "part", what, "err", err)
		writeErr(w, 500, "query failed")
	}
	const base = ` FROM core.alarm a WHERE ($1 = '' OR a.org_id = $1::uuid) AND a.raised_at >= $2 AND a.raised_at < $3 AND a.state <> 'suppressed' AND a.shelved_until IS NULL `
	var total, peakHour int
	var ackSec *float64
	if err := s.pool.QueryRow(ctx, `SELECT count(*), (SELECT coalesce(max(n), 0) FROM (SELECT count(*) n`+base+` GROUP BY date_trunc('hour', a.raised_at)) h),
		(SELECT avg(EXTRACT(EPOCH FROM (a.acked_at - a.raised_at)))::float8`+base+` AND a.acked_by IS NOT NULL)`+base, org, from, to).Scan(&total, &peakHour, &ackSec); err != nil {
		fail("totals", err)
		return
	}
	hours := to.Sub(from).Hours()
	out := map[string]any{"from": from, "to": to, "total": total, "per_hour": float64(total) / hours, "per_day": float64(total) / hours * 24,
		"peak_hour": peakHour, "mean_ack_s": ackSec, "target_per_hour": 6}

	list := func(key, sql string, scan func(pgx.Rows) (map[string]any, error), args ...any) bool {
		rows, err := s.pool.Query(ctx, sql, args...)
		if err != nil {
			fail(key, err)
			return false
		}
		defer rows.Close()
		l := []map[string]any{}
		for rows.Next() {
			m, err := scan(rows)
			if err != nil {
				fail(key, err)
				return false
			}
			l = append(l, m)
		}
		out[key] = l
		return true
	}
	if !list("days", `SELECT to_char(date_trunc('day', a.raised_at AT TIME ZONE $4), 'YYYY-MM-DD'), count(*)`+base+` GROUP BY 1 ORDER BY 1`, func(rows pgx.Rows) (map[string]any, error) {
		var d string
		var n int
		err := rows.Scan(&d, &n)
		return map[string]any{"day": d, "count": n}, err
	}, org, from, to, tz) {
		return
	}
	named := func(rows pgx.Rows) (map[string]any, error) {
		var key, asset, rule string
		var n int
		var ds *string
		err := rows.Scan(&key, &asset, &rule, &ds, &n)
		m := map[string]any{"key": key, "asset": asset, "rule": rule, "datastream_id": ds, "count": n}
		if total > 0 {
			m["share"] = float64(n) / float64(total)
		}
		return m, err
	}
	const nameCols = `coalesce(max(a.detail->>'key'), ''), coalesce(max(ast.name), ''), coalesce(max(a.detail->>'rule'), ''), max(a.datastream_id::text)`
	const nameJoin = ` FROM core.alarm a LEFT JOIN core.datastream d ON d.id = a.datastream_id LEFT JOIN core.asset ast ON ast.id = d.asset_id
		WHERE ($1 = '' OR a.org_id = $1::uuid) `
	if !list("top", `SELECT `+nameCols+`, count(*)`+nameJoin+` AND a.raised_at >= $2 AND a.raised_at < $3 AND a.state <> 'suppressed' AND a.shelved_until IS NULL
		GROUP BY coalesce(a.rule_id, a.automation_rule_id) ORDER BY count(*) DESC LIMIT 10`, named, org, from, to) {
		return
	}
	// chattering: three or more alarms of one rule within a minute
	if !list("chattering", `SELECT `+nameCols+`, count(*) FROM (SELECT a.*, a.raised_at - lag(a.raised_at, 2) OVER (PARTITION BY coalesce(a.rule_id, a.automation_rule_id) ORDER BY a.raised_at) AS span
			FROM core.alarm a WHERE ($1 = '' OR a.org_id = $1::uuid) AND a.raised_at >= $2 AND a.raised_at < $3) a
		LEFT JOIN core.datastream d ON d.id = a.datastream_id LEFT JOIN core.asset ast ON ast.id = d.asset_id
		WHERE a.span <= interval '1 minute' GROUP BY coalesce(a.rule_id, a.automation_rule_id) ORDER BY count(*) DESC LIMIT 10`, named, org, from, to) {
		return
	}
	// standing: active for longer than 24 hours now
	if !list("standing", `SELECT `+nameCols+`, (EXTRACT(EPOCH FROM (now() - min(a.raised_at))) / 3600)::int`+nameJoin+` AND a.cleared_at IS NULL AND a.raised_at < now() - interval '24 hours'
		GROUP BY a.id ORDER BY min(a.raised_at) LIMIT 20`, func(rows pgx.Rows) (map[string]any, error) {
		m, err := named(rows)
		if err == nil {
			m["hours"] = m["count"]
			delete(m, "count")
			delete(m, "share")
		}
		return m, err
	}, org) {
		return
	}
	if !list("classes", `SELECT coalesce(c.code, ''), coalesce(c.name, ''), coalesce(c.color, '#d63939'), count(*)
		FROM core.alarm a LEFT JOIN core.alarm_rule r ON r.id = a.rule_id`+alarmmgmt.ClassJoin+`
		WHERE ($1 = '' OR a.org_id = $1::uuid) AND a.raised_at >= $2 AND a.raised_at < $3 GROUP BY 1, 2, 3, c.priority ORDER BY c.priority NULLS LAST`, func(rows pgx.Rows) (map[string]any, error) {
		var code, name, color string
		var n int
		err := rows.Scan(&code, &name, &color, &n)
		return map[string]any{"code": code, "name": name, "color": color, "count": n}, err
	}, org, from, to) {
		return
	}
	writeJSON(w, 200, out)
}

// --- alarm classes ---

type alarmClassIn struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Color       string `json:"color"`
	AckRequired *bool  `json:"ack_required"`
	Sound       string `json:"sound"`
	Reason      string `json:"reason"`
}

var (
	classCodeRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,30}$`)
	classColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func (in *alarmClassIn) check(needCode bool) string {
	in.Name, in.Reason = strings.TrimSpace(in.Name), strings.TrimSpace(in.Reason)
	if in.Sound == "" {
		in.Sound = "none"
	}
	switch {
	case len(in.Reason) < 3:
		return "reason required"
	case needCode && !classCodeRe.MatchString(in.Code):
		return "code: 2 to 31 lower-case letters, digits or _, starting with a letter"
	case in.Name == "" || len(in.Name) > 60:
		return "name: 1 to 60 characters"
	case in.Priority < 1 || in.Priority > 4:
		return "priority: 1 (highest) to 4"
	case !classColorRe.MatchString(in.Color):
		return "color: #rrggbb"
	case in.Sound != "none" && in.Sound != "beep" && in.Sound != "siren":
		return "sound: none, beep or siren"
	}
	return ""
}

func (s *Server) listAlarmClasses(w http.ResponseWriter, r *http.Request) {
	org := orgOf(PrincipalFrom(r.Context()))
	if err := alarmmgmt.EnsureClasses(r.Context(), s.pool, org); err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.code, c.name, c.priority::int, c.color, c.ack_required, c.sound, c.builtin,
		(SELECT count(*) FROM core.alarm_rule r WHERE r.class_id = c.id AND r.valid_to IS NULL)::int
		FROM core.alarm_class c WHERE c.org_id = $1::uuid ORDER BY c.priority, c.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var c alarmmgmt.Class
		var rules int
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Priority, &c.Color, &c.AckRequired, &c.Sound, &c.Builtin, &rules); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": c.ID, "code": c.Code, "name": c.Name, "priority": c.Priority, "color": c.Color, "ack_required": c.AckRequired,
			"sound": c.Sound, "builtin": c.Builtin, "rules": rules})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createAlarmClass(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in alarmClassIn
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if msg := in.check(true); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	_ = alarmmgmt.EnsureClasses(r.Context(), s.pool, p.OrgID)
	ack := in.AckRequired == nil || *in.AckRequired
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.alarm_class (org_id, code, name, priority, color, ack_required, sound) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (org_id, code) DO NOTHING RETURNING id::text`, p.OrgID, in.Code, in.Name, in.Priority, in.Color, ack, in.Sound).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 409, "a class with this code exists")
		return
	}
	if err != nil {
		writeErr(w, 500, "class not saved")
		return
	}
	s.audit(r, "alarm_class.create", "alarm_class", id, nil, map[string]any{"code": in.Code, "name": in.Name, "priority": in.Priority, "color": in.Color, "ack_required": ack, "sound": in.Sound}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) updateAlarmClass(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in alarmClassIn
	if err := decodeJSON(r, &in, 4096); err != nil || !uuidRe.MatchString(strings.ToLower(id)) {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if msg := in.check(false); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	var old alarmmgmt.Class
	if err := s.pool.QueryRow(r.Context(), `SELECT code, name, priority::int, color, ack_required, sound FROM core.alarm_class WHERE id = $1::uuid AND org_id = $2::uuid`, id, p.OrgID).
		Scan(&old.Code, &old.Name, &old.Priority, &old.Color, &old.AckRequired, &old.Sound); err != nil {
		writeErr(w, 404, "class not found")
		return
	}
	ack := old.AckRequired
	if in.AckRequired != nil {
		ack = *in.AckRequired
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.alarm_class SET name = $3, priority = $4, color = $5, ack_required = $6, sound = $7 WHERE id = $1::uuid AND org_id = $2::uuid`,
		id, p.OrgID, in.Name, in.Priority, in.Color, ack, in.Sound); err != nil {
		writeErr(w, 500, "class not saved")
		return
	}
	s.audit(r, "alarm_class.update", "alarm_class", id, map[string]any{"name": old.Name, "priority": old.Priority, "color": old.Color, "ack_required": old.AckRequired, "sound": old.Sound},
		map[string]any{"name": in.Name, "priority": in.Priority, "color": in.Color, "ack_required": ack, "sound": in.Sound}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteAlarmClass(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if len(reason) < 3 || !uuidRe.MatchString(strings.ToLower(id)) {
		writeErr(w, 400, "reason required")
		return
	}
	var code string
	var builtin bool
	var used int
	if err := s.pool.QueryRow(r.Context(), `SELECT c.code, c.builtin, (SELECT count(*) FROM core.alarm_rule r WHERE r.class_id = c.id)::int
		FROM core.alarm_class c WHERE c.id = $1::uuid AND c.org_id = $2::uuid`, id, p.OrgID).Scan(&code, &builtin, &used); err != nil {
		writeErr(w, 404, "class not found")
		return
	}
	if builtin {
		writeErr(w, 409, "a built-in class cannot be deleted")
		return
	}
	if used > 0 {
		writeErr(w, 409, "the class is used by alarm rules (also earlier versions keep it for their alarms)")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM core.alarm_class WHERE id = $1::uuid AND org_id = $2::uuid`, id, p.OrgID); err != nil {
		writeErr(w, 500, "class not deleted")
		return
	}
	s.audit(r, "alarm_class.delete", "alarm_class", id, map[string]any{"code": code}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}
