package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Logbook (operations log of a control room, 0.87.0): one chronological list of what people did and what the plant
// reported — commands sent to devices (setpoint writes, start/stop, mode changes: who, when, where, what, value,
// read-back, reason) and alarm events (raised, escalated, acknowledged with who and why, cleared). It reads the
// append-only command and alarm event tables, so it shows exactly what the audit trail holds, scoped to the
// organization and optionally to some devices and key prefixes.
//
//	GET /api/v1/logbook?devices=<id,...>&keys=<key or prefix*,...>&kinds=commands,alarms&limit=100&before=<RFC3339>   (data.read)

type logEntry struct {
	TS       time.Time `json:"ts"`
	Kind     string    `json:"kind"`  // command | alarm
	Event    string    `json:"event"` // command: write/start/...; alarm: raised, escalated, acknowledged, cleared
	User     string    `json:"user,omitempty"`
	DeviceID string    `json:"device_id,omitempty"`
	Device   string    `json:"device,omitempty"`
	Asset    string    `json:"asset,omitempty"`
	Key      string    `json:"key,omitempty"`
	Value    any       `json:"value,omitempty"`
	Readback any       `json:"readback,omitempty"`
	Status   string    `json:"status,omitempty"`
	Severity string    `json:"severity,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func splitIDs(v string, max int) []string {
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		x = strings.TrimSpace(x)
		if x != "" && len(out) < max {
			out = append(out, x)
		}
	}
	return out
}

func (s *Server) logbook(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	devs := splitIDs(q.Get("devices"), 200)
	for _, d := range devs {
		if !uuidRe.MatchString(d) {
			writeErr(w, 400, "devices must be ids")
			return
		}
	}
	keys := splitIDs(q.Get("keys"), 50)
	for _, k := range keys {
		if !cmdKeyRe.MatchString(strings.TrimSuffix(k, "*")) {
			writeErr(w, 400, "keys must be keys or key prefixes ending with *")
			return
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	before := time.Now().Add(time.Minute)
	if b := q.Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			writeErr(w, 400, "before must be an RFC 3339 time")
			return
		}
		before = t
	}
	kinds := q.Get("kinds")
	want := func(k string) bool { return kinds == "" || strings.Contains(kinds, k) }
	keyOK := func(k string) bool {
		if len(keys) == 0 {
			return true
		}
		for _, p := range keys {
			if pre, ok := strings.CutSuffix(p, "*"); ok && strings.HasPrefix(k, pre) || p == k {
				return true
			}
		}
		return false
	}
	var out []logEntry
	ctx := r.Context()
	if want("commands") {
		rows, err := s.pool.Query(ctx, `SELECT c.issued_at, c.method, c.args, c.status, c.issued_by_name, d.id::text, d.name, c.result, coalesce(c.error, ''), coalesce(c.reason, '')
			FROM core.device_command c JOIN core.device d ON d.id = c.device_id
			WHERE c.org_id = $1::uuid AND c.issued_at < $2 AND (cardinality($3::uuid[]) = 0 OR c.device_id = ANY($3::uuid[]))
			ORDER BY c.issued_at DESC LIMIT $4`, org, before, devs, limit*4)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var e logEntry
			var args, res []byte
			if err := rows.Scan(&e.TS, &e.Event, &args, &e.Status, &e.User, &e.DeviceID, &e.Device, &res, &e.Error, &e.Reason); err != nil {
				rows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			var a map[string]any
			_ = json.Unmarshal(args, &a)
			if k, ok := a["key"].(string); ok {
				e.Key = k
			}
			e.Value = a["value"]
			if e.Value == nil && len(a) > 0 && e.Key == "" {
				e.Value = a
			}
			var rr map[string]any
			if json.Unmarshal(res, &rr) == nil {
				e.Readback = rr["readback"]
			}
			if !keyOK(e.Key) {
				continue
			}
			e.Kind = "command"
			out = append(out, e)
		}
		rows.Close()
	}
	if want("alarms") {
		rows, err := s.pool.Query(ctx, `SELECT ev.ts, ev.event, coalesce(ev.actor_name, ''), coalesce(ev.reason, ''), a.severity, a.value::float8,
				coalesce(ds.key, a.detail->>'key', ''), coalesce(ast.name, ''), coalesce(dv.id::text, ''), coalesce(dv.name, '')
			FROM core.alarm_event ev
			JOIN core.alarm a ON a.id = ev.alarm_id
			LEFT JOIN core.datastream ds ON ds.id = a.datastream_id
			LEFT JOIN core.asset ast ON ast.id = ds.asset_id
			LEFT JOIN LATERAL (SELECT g.from_id FROM core.assignment g WHERE g.kind = 'device_datastream' AND g.to_id = a.datastream_id
				AND g.valid_to IS NULL ORDER BY g.valid_from DESC LIMIT 1) asg ON true
			LEFT JOIN core.device dv ON dv.id = coalesce(a.device_id, asg.from_id)
			WHERE a.org_id = $1::uuid AND ev.ts < $2 AND ev.event IN ('raised', 'escalated', 'acknowledged', 'suppressed', 'unsuppressed', 'cleared')
				AND (cardinality($3::uuid[]) = 0 OR coalesce(a.device_id, asg.from_id) = ANY($3::uuid[]))
			ORDER BY ev.ts DESC LIMIT $4`, org, before, devs, limit*4)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var e logEntry
			var v *float64
			if err := rows.Scan(&e.TS, &e.Event, &e.User, &e.Reason, &e.Severity, &v, &e.Key, &e.Asset, &e.DeviceID, &e.Device); err != nil {
				rows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			if v != nil {
				e.Value = *v
			}
			if !keyOK(e.Key) {
				continue
			}
			e.Kind = "alarm"
			out = append(out, e)
		}
		rows.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS.After(out[j].TS) })
	if len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []logEntry{}
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
