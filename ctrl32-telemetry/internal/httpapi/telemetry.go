package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/protocol"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

// The Telemetry tab of the device page (0.67.0): everything a device sends, in one place.
//
//	GET  /api/v1/devices/{id}/telemetry?from=&to=                         every key: latest value, 24 h sparkline, statistics
//	GET  /api/v1/devices/{id}/telemetry/series?source=&key=&from=&to=&agg=&bucket=   chart points
//	GET  /api/v1/devices/{id}/telemetry/values?source=&key=&from=&to=&cursor=&limit= the values, newest first, paged
//	GET  /api/v1/devices/{id}/telemetry/export?source=&key=&from=&to=&format=csv|xlsx   (data.export, audited)
//	POST /api/v1/devices/{id}/telemetry/assign {key, asset_id, quantity, unit, kind, reason}  (device.manage)
//
// A key has one of three sources:
//   - datastream: a key the device publishes into an assigned datastream; values are observations.
//   - unassigned: a key without a datastream. The server keeps such messages as raw messages (decode status
//     unknown_key) and stores no observations, so its values are read from the raw messages of the last 90 days
//     (at most 5000 messages); assigning a datastream does not convert the past ones.
//   - status: a key of the device's status (heartbeat) messages: battery, signal, uptime, firmware, …; the current
//     value is the merged status document, the history comes from the raw status messages.
//
// Every query is bounded: observations by the (datastream_id, measured_at) index and the time range, raw messages by
// the (device_id, received_at) index, a time window and a row limit.

const (
	telRawWindow     = 90 * 24 * time.Hour // how far back unassigned and status keys are looked for
	telRawMessages   = 5000                // raw messages read for the summary
	telSeriesRaw     = 20000               // raw messages read for one chart or table
	telMaxPoints     = 5000                // raw points of a chart before it is aggregated
	telMaxRange      = 366 * 24 * time.Hour
	telExportMaxRows = 100000
	telSparkBucket   = 30 * time.Minute
)

// status keys that are bookkeeping of the server, not values reported by the device
var telStatusSkip = map[string]bool{"last_seen": true, "last_topic": true, "attributes": true, "location": true}

type telValue struct {
	At         time.Time       `json:"at"`
	ReceivedAt *time.Time      `json:"received_at,omitempty"`
	Value      *float64        `json:"value"`
	ValueJSON  json.RawMessage `json:"value_json,omitempty"`
	Quality    string          `json:"quality"`
	// latest value only: older than the expected interval allows (computed, quality.go); Quality then reads "stale"
	// for a good value and StoredQuality keeps the stored one
	Stale         bool   `json:"stale,omitempty"`
	StoredQuality string `json:"stored_quality,omitempty"`
}

type telStats struct {
	Min    *float64 `json:"min"`
	Max    *float64 `json:"max"`
	Avg    *float64 `json:"avg"`
	N      int64    `json:"n"`      // values in the range
	Faults int64    `json:"faults"` // of them with quality sensor_fault (not in min/max/avg)
}

type telKey struct {
	Key          string       `json:"key"`
	Source       string       `json:"source"` // datastream | unassigned | status
	DatastreamID string       `json:"datastream_id,omitempty"`
	Asset        string       `json:"asset,omitempty"`
	Quantity     string       `json:"quantity,omitempty"`
	Unit         string       `json:"unit"`
	Kind         string       `json:"kind,omitempty"`
	Since        *time.Time   `json:"since,omitempty"` // assignment start
	Last         *telValue    `json:"last"`
	Stats        telStats     `json:"stats"`
	Spark        [][2]float64 `json:"spark"` // [unix seconds, average] per 30 minutes of the last 24 hours
}

// telAcc accumulates values of one key read from raw messages.
type telAcc struct {
	k        *telKey
	sum      float64
	spark    map[int64][2]float64 // bucket start -> sum, count
	lastSeen time.Time
}

func (a *telAcc) add(at time.Time, v protocol.Value, faulty bool, from, to, sparkFrom time.Time) {
	q := "ok"
	if faulty {
		q = "sensor_fault"
	}
	if a.k.Last == nil || at.After(a.k.Last.At) {
		tv := &telValue{At: at, Quality: q}
		if f := telNum(v); f != nil {
			tv.Value = f
		} else if raw := jsonOf(v); raw != nil {
			tv.ValueJSON = raw
		}
		a.k.Last = tv
	}
	f := telNum(v)
	if f == nil || at.Before(from) || !at.Before(to) {
		if f != nil && !faulty && !at.Before(sparkFrom) {
			a.sparkAdd(at, *f, sparkFrom)
		}
		return
	}
	a.k.Stats.N++
	if faulty {
		a.k.Stats.Faults++
		return
	}
	x := *f
	if a.k.Stats.Min == nil || x < *a.k.Stats.Min {
		a.k.Stats.Min = &x
	}
	if a.k.Stats.Max == nil || x > *a.k.Stats.Max {
		y := x
		a.k.Stats.Max = &y
	}
	a.sum += x
	if !at.Before(sparkFrom) {
		a.sparkAdd(at, x, sparkFrom)
	}
}

func (a *telAcc) sparkAdd(at time.Time, x float64, sparkFrom time.Time) {
	if a.spark == nil {
		a.spark = map[int64][2]float64{}
	}
	b := sparkFrom.Add(at.Sub(sparkFrom).Truncate(telSparkBucket)).Unix()
	s := a.spark[b]
	a.spark[b] = [2]float64{s[0] + x, s[1] + 1}
}

func (a *telAcc) finish() {
	if n := a.k.Stats.N - a.k.Stats.Faults; n > 0 {
		avg := a.sum / float64(n)
		a.k.Stats.Avg = &avg
	}
	a.k.Spark = sparkOf(a.spark)
}

func sparkOf(m map[int64][2]float64) [][2]float64 {
	out := make([][2]float64, 0, len(m))
	for t, s := range m {
		out = append(out, [2]float64{float64(t), s[0] / s[1]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func telNum(v protocol.Value) *float64 {
	if v.Num != nil {
		x := *v.Num
		return &x
	}
	if v.Bool != nil {
		x := 0.0
		if *v.Bool {
			x = 1
		}
		return &x
	}
	return nil
}

func jsonOf(v protocol.Value) json.RawMessage {
	if v.Str != nil {
		b, _ := json.Marshal(*v.Str)
		return b
	}
	return v.JSON
}

// statusValue turns one value of a status document into a protocol value (numbers, booleans, text, objects).
func statusValue(raw json.RawMessage) (protocol.Value, bool) {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return protocol.Value{}, false
	}
	switch t[0] {
	case '{', '[':
		return protocol.Value{JSON: raw}, true
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return protocol.Value{}, false
		}
		return protocol.Value{Str: &s}, true
	case 't', 'f':
		b := t == "true"
		return protocol.Value{Bool: &b}, true
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return protocol.Value{}, false
	}
	return protocol.Value{Num: &f}, true
}

// telDevice checks the device belongs to the caller's organization; returns its status document.
func (s *Server) telDevice(w http.ResponseWriter, r *http.Request) (org, id string, status json.RawMessage, ok bool) {
	org, ok = s.mustOrg(w, r)
	if !ok {
		return
	}
	id = r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "device not found")
		return "", "", nil, false
	}
	if err := s.pool.QueryRow(r.Context(), `SELECT status FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&status); err != nil {
		writeErr(w, 404, "device not found")
		return "", "", nil, false
	}
	return org, id, status, true
}

// telRange reads from/to (RFC 3339; default the last 24 hours) and bounds the span.
func telRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	from, to, ok := timeRange(w, r, 24*time.Hour)
	if !ok {
		return from, to, false
	}
	if !to.After(from) {
		writeErr(w, 400, "to must be after from")
		return from, to, false
	}
	if to.Sub(from) > telMaxRange {
		writeErr(w, 400, "range too long: at most 366 days")
		return from, to, false
	}
	return from, to, true
}

// assignedKeys returns the current datastream assignments of a device: telemetry key -> datastream (the datastream
// key and, when it was normalized, the original key the device publishes).
type telDS struct {
	ID, Key, SourceKey, Quantity, Unit, Kind, Asset string
	Since                                           time.Time
}

func (s *Server) assignedDatastreams(r *http.Request, org, devID string) ([]telDS, error) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT d.id::text, d.key, coalesce(d.attributes->>'source_key', ''), d.quantity, d.unit, d.kind, ast.name, a.valid_from
		FROM core.assignment a JOIN core.datastream d ON d.id = a.to_id JOIN core.asset ast ON ast.id = d.asset_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.org_id = $2::uuid AND a.valid_to IS NULL
		ORDER BY d.key LIMIT 1000`, devID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []telDS
	for rows.Next() {
		var d telDS
		if err := rows.Scan(&d.ID, &d.Key, &d.SourceKey, &d.Quantity, &d.Unit, &d.Kind, &d.Asset, &d.Since); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GET /api/v1/devices/{id}/telemetry
func (s *Server) deviceTelemetry(w http.ResponseWriter, r *http.Request) {
	org, devID, status, ok := s.telDevice(w, r)
	if !ok {
		return
	}
	from, to, ok := telRange(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	sparkFrom := now.Add(-24 * time.Hour).Truncate(telSparkBucket)
	dss, err := s.assignedDatastreams(r, org, devID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	keys := []*telKey{}
	byDS := map[string]*telKey{}
	assigned := map[string]bool{}
	ids := make([]string, 0, len(dss))
	for _, d := range dss {
		since := d.Since
		k := &telKey{Key: d.Key, Source: "datastream", DatastreamID: d.ID, Asset: d.Asset, Quantity: d.Quantity, Unit: d.Unit, Kind: d.Kind, Since: &since, Spark: [][2]float64{}}
		keys = append(keys, k)
		byDS[d.ID] = k
		assigned[d.Key] = true
		if d.SourceKey != "" {
			assigned[d.SourceKey] = true
		}
		ids = append(ids, d.ID)
	}
	if len(ids) > 0 {
		// latest value of each datastream: one index probe per datastream
		rows, err := s.pool.Query(ctx, `
			SELECT d.id::text, l.measured_at, l.received_at, l.value::float8, l.value_json, l.quality,
			       COALESCE((SELECT EXTRACT(EPOCH FROM ds.expected_interval) FROM core.datastream ds WHERE ds.id = d.id), 0)::float8
			FROM unnest($1::uuid[]) AS d(id)
			JOIN LATERAL (SELECT measured_at, received_at, value, value_json, quality FROM core.observation o
			              WHERE o.datastream_id = d.id ORDER BY o.measured_at DESC LIMIT 1) l ON true`, ids)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var id string
			var v telValue
			var rAt time.Time
			var expected float64
			if err := rows.Scan(&id, &v.At, &rAt, &v.Value, &v.ValueJSON, &v.Quality, &expected); err != nil {
				rows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			v.ReceivedAt = &rAt
			if q, st := liveQuality(v.Quality, v.At, expected, time.Now()); st {
				v.Stale = true
				if q != v.Quality {
					v.StoredQuality, v.Quality = v.Quality, q
				}
			}
			if k := byDS[id]; k != nil {
				k.Last = &v
			}
		}
		rows.Close()
		// statistics over the chosen range; values from a faulty sensor are counted, not averaged
		rows, err = s.pool.Query(ctx, `
			SELECT datastream_id::text,
			       min(value) FILTER (WHERE quality <> 'sensor_fault')::float8, max(value) FILTER (WHERE quality <> 'sensor_fault')::float8,
			       avg(value) FILTER (WHERE quality <> 'sensor_fault')::float8, count(*), count(*) FILTER (WHERE quality = 'sensor_fault')
			FROM core.observation WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3 GROUP BY 1`, ids, from, to)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var id string
			var st telStats
			if err := rows.Scan(&id, &st.Min, &st.Max, &st.Avg, &st.N, &st.Faults); err != nil {
				rows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			if k := byDS[id]; k != nil {
				k.Stats = st
			}
		}
		rows.Close()
		// sparkline: averages per 30 minutes of the last 24 hours
		rows, err = s.pool.Query(ctx, `
			SELECT datastream_id::text, extract(epoch FROM date_bin('30 minutes', measured_at, $2::timestamptz))::float8, avg(value)::float8
			FROM core.observation WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3
			  AND value IS NOT NULL AND quality <> 'sensor_fault' GROUP BY 1, 2 ORDER BY 1, 2`, ids, sparkFrom, now.Add(time.Minute))
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var id string
			var t, v float64
			if err := rows.Scan(&id, &t, &v); err != nil {
				rows.Close()
				writeErr(w, 500, "scan failed")
				return
			}
			if k := byDS[id]; k != nil {
				k.Spark = append(k.Spark, [2]float64{t, v})
			}
		}
		rows.Close()
	}

	// keys without a datastream, from the raw messages that carried them
	acc := map[string]*telAcc{}
	rawFrom := now.Add(-telRawWindow)
	if from.Before(rawFrom) {
		rawFrom = from
	}
	if err := s.scanRawTelemetry(r, org, devID, rawFrom, now.Add(time.Minute), telRawMessages, "", func(at, _ time.Time, sm protocol.Sample) {
		for k, v := range sm.Values {
			if assigned[k] {
				continue
			}
			a := acc[k]
			if a == nil {
				a = &telAcc{k: &telKey{Key: k, Source: "unassigned", Spark: [][2]float64{}}}
				acc[k] = a
			}
			a.add(at, v, sm.Faulty(k), from, to, sparkFrom)
		}
	}); err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	unknown := make([]*telKey, 0, len(acc))
	for _, a := range acc {
		a.finish()
		unknown = append(unknown, a.k)
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i].Key < unknown[j].Key })
	keys = append(keys, unknown...)

	// status keys: the current document, the history from the status messages
	keys = append(keys, s.statusKeys(r, org, devID, status, from, to, sparkFrom, now)...)
	writeJSON(w, 200, map[string]any{"device_id": devID, "from": from, "to": to, "generated_at": now, "keys": keys,
		"raw_window_days": int(telRawWindow.Hours() / 24)})
}

// scanRawTelemetry walks the telemetry raw messages of a device with keys without a datastream (decode status
// unknown_key), newest first, and calls fn for every sample. The sample time is its own time or, without one, the
// time the message was received.
func (s *Server) scanRawTelemetry(r *http.Request, org, devID string, from, to time.Time, limit int, key string, fn func(at, received time.Time, sm protocol.Sample)) error {
	q := `SELECT received_at, payload_json::text FROM core.raw_message
		WHERE device_id = $1::uuid AND org_id = $2::uuid AND direction = 'in' AND decode_status = 'unknown_key'
		  AND received_at >= $3 AND received_at < $4 AND payload_json IS NOT NULL`
	args := []any{devID, org, from, to, limit}
	if key != "" {
		// cheap pre-filter on the decode error ("unknown keys: a,b"); the payload is parsed anyway
		q += ` AND position($6 in coalesce(decode_error, '')) > 0`
		args = append(args, key)
	}
	q += ` ORDER BY received_at DESC LIMIT $5`
	rows, err := s.pool.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var at time.Time
		var payload string
		if err := rows.Scan(&at, &payload); err != nil {
			return err
		}
		samples, err := protocol.ParseTelemetry([]byte(payload))
		if err != nil {
			continue
		}
		for _, sm := range samples {
			t := at
			switch {
			case sm.MeasuredAt != nil:
				t = *sm.MeasuredAt
			case sm.Relative != nil:
				t = at.Add(time.Duration(*sm.Relative) * time.Second)
			}
			fn(t.UTC(), at.UTC(), sm)
		}
	}
	return rows.Err()
}

// scanRawStatus walks the status messages of a device, newest first.
func (s *Server) scanRawStatus(r *http.Request, org, devID string, from, to time.Time, limit int, fn func(time.Time, map[string]json.RawMessage)) error {
	rows, err := s.pool.Query(r.Context(), `SELECT received_at, payload_json::text FROM core.raw_message
		WHERE device_id = $1::uuid AND org_id = $2::uuid AND direction = 'in' AND decode_status = 'ok'
		  AND received_at >= $3 AND received_at < $4 AND payload_json IS NOT NULL
		  AND topic LIKE '%/status'
		ORDER BY received_at DESC LIMIT $5`, devID, org, from, to, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var at time.Time
		var payload string
		if err := rows.Scan(&at, &payload); err != nil {
			return err
		}
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(payload), &m) != nil {
			continue
		}
		fn(at.UTC(), m)
	}
	return rows.Err()
}

func (s *Server) statusKeys(r *http.Request, org, devID string, status json.RawMessage, from, to, sparkFrom, now time.Time) []*telKey {
	var doc map[string]json.RawMessage
	_ = json.Unmarshal(status, &doc)
	acc := map[string]*telAcc{}
	lastSeen := time.Time{}
	if raw, ok := doc["last_seen"]; ok {
		_ = json.Unmarshal(raw, &lastSeen)
	}
	rawFrom := now.Add(-telRawWindow)
	if from.Before(rawFrom) {
		rawFrom = from
	}
	_ = s.scanRawStatus(r, org, devID, rawFrom, now.Add(time.Minute), telRawMessages, func(at time.Time, m map[string]json.RawMessage) {
		for k, raw := range m {
			if telStatusSkip[k] {
				continue
			}
			v, ok := statusValue(raw)
			if !ok {
				continue
			}
			a := acc[k]
			if a == nil {
				a = &telAcc{k: &telKey{Key: k, Source: "status", Spark: [][2]float64{}}}
				acc[k] = a
			}
			a.add(at, v, false, from, to, sparkFrom)
		}
	})
	// the current document wins for the latest value (it may hold keys from older messages or other sources)
	for k, raw := range doc {
		if telStatusSkip[k] {
			continue
		}
		v, ok := statusValue(raw)
		if !ok {
			continue
		}
		a := acc[k]
		if a == nil {
			a = &telAcc{k: &telKey{Key: k, Source: "status", Spark: [][2]float64{}}}
			acc[k] = a
		}
		at := lastSeen
		if a.k.Last != nil {
			at = a.k.Last.At
		}
		tv := &telValue{At: at, Quality: "ok"}
		if f := telNum(v); f != nil {
			tv.Value = f
		} else {
			tv.ValueJSON = jsonOf(v)
		}
		a.k.Last = tv
	}
	out := make([]*telKey, 0, len(acc))
	for _, a := range acc {
		a.finish()
		out = append(out, a.k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// telSource resolves the source and key of a request; for a datastream key the current assignment of the device.
type telSrc struct {
	Source, Key string
	DS          *telDS
}

func (s *Server) telSource(w http.ResponseWriter, r *http.Request, org, devID string) (telSrc, bool) {
	q := r.URL.Query()
	src := telSrc{Source: q.Get("source"), Key: q.Get("key")}
	if !dsKeyRe.MatchString(src.Key) && !(src.Source == "status" && statusKeyRe(src.Key)) {
		writeErr(w, 400, "key required")
		return src, false
	}
	switch src.Source {
	case "datastream", "":
		src.Source = "datastream"
		dss, err := s.assignedDatastreams(r, org, devID)
		if err != nil {
			writeErr(w, 500, "query failed")
			return src, false
		}
		for i := range dss {
			if dss[i].Key == src.Key || dss[i].SourceKey == src.Key {
				src.DS = &dss[i]
			}
		}
		if src.DS == nil {
			writeErr(w, 404, "no datastream with this key is assigned to the device")
			return src, false
		}
	case "unassigned", "status":
	default:
		writeErr(w, 400, "source must be datastream, unassigned or status")
		return src, false
	}
	return src, true
}

// status documents are free-form; their keys are shown as they are, but only plain names are accepted back
func statusKeyRe(k string) bool {
	if k == "" || len(k) > 64 {
		return false
	}
	for _, c := range k {
		if !(c == '_' || c == '-' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// telPoint is a chart point: a value, or an aggregate of a bucket.
type telPoint struct {
	T       time.Time `json:"t"`
	V       *float64  `json:"v"`
	Min     *float64  `json:"min,omitempty"`
	Max     *float64  `json:"max,omitempty"`
	N       int64     `json:"n,omitempty"`
	Quality string    `json:"quality,omitempty"` // raw points; buckets: "sensor_fault" when one was in it
}

// telBucket chooses a bucket for an aggregated chart: about 300 points over the range, rounded to a sensible step.
func telBucket(from, to time.Time, req int) time.Duration {
	if req > 0 {
		return time.Duration(req) * time.Second
	}
	span := to.Sub(from)
	for _, b := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour} {
		if span/b <= 300 {
			return b
		}
	}
	return 7 * 24 * time.Hour
}

// GET /api/v1/devices/{id}/telemetry/series
func (s *Server) deviceTelemetrySeries(w http.ResponseWriter, r *http.Request) {
	org, devID, _, ok := s.telDevice(w, r)
	if !ok {
		return
	}
	src, ok := s.telSource(w, r, org, devID)
	if !ok {
		return
	}
	from, to, ok := telRange(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	agg := q.Get("agg")
	switch agg {
	case "", "auto", "avg", "min", "max", "none":
	default:
		writeErr(w, 400, "agg must be auto, avg, min, max or none")
		return
	}
	reqBucket := 0
	if v := q.Get("bucket"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 31*86400 {
			writeErr(w, 400, "bucket must be seconds (0 = automatic)")
			return
		}
		reqBucket = n
	}
	ctx := r.Context()
	out := map[string]any{"source": src.Source, "key": src.Key, "from": from, "to": to}
	var points []telPoint
	var st telStats
	bucket := time.Duration(0)
	if src.DS != nil {
		out["datastream_id"], out["unit"] = src.DS.ID, src.DS.Unit
		if err := s.pool.QueryRow(ctx, `SELECT min(value) FILTER (WHERE quality <> 'sensor_fault')::float8, max(value) FILTER (WHERE quality <> 'sensor_fault')::float8,
			avg(value) FILTER (WHERE quality <> 'sensor_fault')::float8, count(*), count(*) FILTER (WHERE quality = 'sensor_fault')
			FROM core.observation WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3`, src.DS.ID, from, to).
			Scan(&st.Min, &st.Max, &st.Avg, &st.N, &st.Faults); err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		raw := agg == "none" || (agg == "" || agg == "auto") && st.N <= telMaxPoints
		if agg == "none" && st.N > telMaxPoints {
			raw = false // too many for one chart: aggregate anyway
		}
		if raw {
			rows, err := s.pool.Query(ctx, `SELECT measured_at, value::float8, quality FROM core.observation
				WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3 AND value IS NOT NULL ORDER BY measured_at LIMIT $4`, src.DS.ID, from, to, telMaxPoints)
			if err != nil {
				writeErr(w, 500, "query failed")
				return
			}
			for rows.Next() {
				var p telPoint
				if err := rows.Scan(&p.T, &p.V, &p.Quality); err != nil {
					rows.Close()
					writeErr(w, 500, "scan failed")
					return
				}
				points = append(points, p)
			}
			rows.Close()
		} else {
			bucket = telBucket(from, to, reqBucket)
			if to.Sub(from)/bucket > maxBuckets {
				writeErr(w, 400, "range too long for this bucket: at most 20000 buckets")
				return
			}
			rows, err := s.pool.Query(ctx, `
				SELECT date_bin($4::interval, measured_at, $2::timestamptz),
				       avg(value) FILTER (WHERE quality <> 'sensor_fault')::float8, min(value) FILTER (WHERE quality <> 'sensor_fault')::float8,
				       max(value) FILTER (WHERE quality <> 'sensor_fault')::float8, count(*), bool_or(quality = 'sensor_fault')
				FROM core.observation WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3 AND value IS NOT NULL
				GROUP BY 1 ORDER BY 1`, src.DS.ID, from, to, fmt.Sprintf("%d seconds", int(bucket.Seconds())))
			if err != nil {
				writeErr(w, 500, "query failed")
				return
			}
			for rows.Next() {
				var p telPoint
				var fault bool
				if err := rows.Scan(&p.T, &p.V, &p.Min, &p.Max, &p.N, &fault); err != nil {
					rows.Close()
					writeErr(w, 500, "scan failed")
					return
				}
				if fault {
					p.Quality = "sensor_fault"
				}
				points = append(points, pickAgg(p, agg))
			}
			rows.Close()
		}
		out["annotations"] = s.annotationsFor(r, org, src.DS.ID, from, to)
	} else {
		vals, truncated, err := s.rawValues(r, org, devID, src, from, to, telSeriesRaw)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		out["truncated"] = truncated
		sort.Slice(vals, func(i, j int) bool { return vals[i].At.Before(vals[j].At) })
		var sum float64
		for _, v := range vals {
			if v.Value == nil {
				continue
			}
			st.N++
			if v.Quality == "sensor_fault" {
				st.Faults++
				continue
			}
			x := *v.Value
			if st.Min == nil || x < *st.Min {
				st.Min = &x
			}
			if st.Max == nil || x > *st.Max {
				y := x
				st.Max = &y
			}
			sum += x
		}
		if n := st.N - st.Faults; n > 0 {
			a := sum / float64(n)
			st.Avg = &a
		}
		if agg == "none" || ((agg == "" || agg == "auto") && st.N <= telMaxPoints) {
			for _, v := range vals {
				if v.Value != nil {
					points = append(points, telPoint{T: v.At, V: v.Value, Quality: v.Quality})
				}
			}
			if len(points) > telMaxPoints {
				points = points[len(points)-telMaxPoints:]
			}
		} else {
			bucket = telBucket(from, to, reqBucket)
			points = bucketValues(vals, from, bucket, agg)
		}
		out["annotations"] = []annotation{}
	}
	if bucket > 0 {
		out["bucket_s"] = int(bucket.Seconds())
	} else {
		out["bucket_s"] = 0
	}
	if points == nil {
		points = []telPoint{}
	}
	out["points"], out["stats"] = points, st
	writeJSON(w, 200, out)
}

// pickAgg shows the chosen aggregate as the point's value (the band min…max stays).
func pickAgg(p telPoint, agg string) telPoint {
	switch agg {
	case "min":
		p.V = p.Min
	case "max":
		p.V = p.Max
	}
	return p
}

func bucketValues(vals []telValue, from time.Time, bucket time.Duration, agg string) []telPoint {
	type b struct {
		sum, n float64
		min    *float64
		max    *float64
		cnt    int64
		fault  bool
	}
	m := map[int64]*b{}
	var order []int64
	for _, v := range vals {
		if v.Value == nil {
			continue
		}
		k := from.Add(v.At.Sub(from).Truncate(bucket)).Unix()
		x := m[k]
		if x == nil {
			x = &b{}
			m[k] = x
			order = append(order, k)
		}
		x.cnt++
		if v.Quality == "sensor_fault" {
			x.fault = true
			continue
		}
		f := *v.Value
		x.sum += f
		x.n++
		if x.min == nil || f < *x.min {
			x.min = &f
		}
		if x.max == nil || f > *x.max {
			g := f
			x.max = &g
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	out := make([]telPoint, 0, len(order))
	for _, k := range order {
		x := m[k]
		p := telPoint{T: time.Unix(k, 0).UTC(), Min: x.min, Max: x.max, N: x.cnt}
		if x.n > 0 {
			a := x.sum / x.n
			p.V = &a
		}
		if x.fault {
			p.Quality = "sensor_fault"
		}
		out = append(out, pickAgg(p, agg))
	}
	return out
}

// rawValues reads the values of an unassigned or status key in a range from the raw messages (newest first).
func (s *Server) rawValues(r *http.Request, org, devID string, src telSrc, from, to time.Time, limit int) ([]telValue, bool, error) {
	var out []telValue
	n := 0
	var err error
	if src.Source == "status" {
		err = s.scanRawStatus(r, org, devID, from, to, limit, func(at time.Time, m map[string]json.RawMessage) {
			n++
			raw, ok := m[src.Key]
			if !ok {
				return
			}
			v, ok := statusValue(raw)
			if !ok {
				return
			}
			rAt := at
			out = append(out, telValueOf(at, &rAt, v, false))
		})
	} else {
		err = s.scanRawTelemetry(r, org, devID, from.Add(-time.Hour), to.Add(time.Hour), limit, src.Key, func(at, rAt time.Time, sm protocol.Sample) {
			n++
			v, ok := sm.Values[src.Key]
			if !ok || at.Before(from) || !at.Before(to) {
				return
			}
			out = append(out, telValueOf(at, &rAt, v, sm.Faulty(src.Key)))
		})
	}
	return out, n >= limit, err
}

func telValueOf(at time.Time, rAt *time.Time, v protocol.Value, faulty bool) telValue {
	tv := telValue{At: at, ReceivedAt: rAt, Quality: "ok"}
	if faulty {
		tv.Quality = "sensor_fault"
	}
	if f := telNum(v); f != nil {
		tv.Value = f
	} else {
		tv.ValueJSON = jsonOf(v)
	}
	return tv
}

// GET /api/v1/devices/{id}/telemetry/values: newest first. Datastream keys page by a cursor over (measured_at, id);
// unassigned and status keys page by time (cursor = the time of the last row shown).
func (s *Server) deviceTelemetryValues(w http.ResponseWriter, r *http.Request) {
	org, devID, _, ok := s.telDevice(w, r)
	if !ok {
		return
	}
	src, ok := s.telSource(w, r, org, devID)
	if !ok {
		return
	}
	from, to, ok := telRange(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, next, err := s.telRows(r, org, devID, src, from, to, q.Get("cursor"), limit)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"rows": rows, "next_cursor": next})
}

// telRow is one value in the table and the exports.
type telRow struct {
	telValue
	TimeSource string `json:"time_source,omitempty"`
	Annotation string `json:"annotation,omitempty"`
}

func (s *Server) telRows(r *http.Request, org, devID string, src telSrc, from, to time.Time, cursor string, limit int) ([]telRow, string, error) {
	out := []telRow{}
	if src.DS != nil {
		cAt, cID, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		args := []any{src.DS.ID, from, to, limit + 1}
		cond := ""
		if cAt != nil {
			cond = ` AND (o.measured_at, o.id) < ($5, $6::uuid)`
			args = append(args, *cAt, cID)
		}
		rows, err := s.pool.Query(r.Context(), `SELECT o.id::text, o.measured_at, o.received_at, o.value::float8, o.value_json, o.quality, o.time_source,
			(SELECT string_agg(a.text, ' | ' ORDER BY a.ts_from) FROM core.annotation a
			 WHERE a.datastream_id = o.datastream_id AND a.retracts IS NULL AND o.measured_at BETWEEN a.ts_from AND a.ts_to
			   AND NOT EXISTS (SELECT 1 FROM core.annotation x WHERE x.retracts = a.id))
			FROM core.observation o WHERE o.datastream_id = $1::uuid AND o.measured_at >= $2 AND o.measured_at < $3`+cond+`
			ORDER BY o.measured_at DESC, o.id DESC LIMIT $4`, args...)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		lastAt, lastID := time.Time{}, ""
		for rows.Next() {
			var id string
			var row telRow
			var rAt time.Time
			var ann *string
			if err := rows.Scan(&id, &row.At, &rAt, &row.Value, &row.ValueJSON, &row.Quality, &row.TimeSource, &ann); err != nil {
				return nil, "", err
			}
			if len(out) == limit {
				return out, encodeCursor(lastAt, lastID), nil
			}
			row.ReceivedAt = &rAt
			if ann != nil {
				row.Annotation = *ann
			}
			out = append(out, row)
			lastAt, lastID = row.At, id
		}
		return out, "", rows.Err()
	}
	if cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor")
		}
		if t.Before(to) {
			to = t
		}
	}
	vals, _, err := s.rawValues(r, org, devID, src, from, to, telSeriesRaw)
	if err != nil {
		return nil, "", err
	}
	sort.SliceStable(vals, func(i, j int) bool { return vals[i].At.After(vals[j].At) })
	next := ""
	if len(vals) > limit {
		// the next page starts below the last time shown; values sharing that time stay together on this page
		cut := limit
		for cut < len(vals) && vals[cut].At.Equal(vals[limit-1].At) {
			cut++
		}
		if cut < len(vals) {
			next = vals[cut-1].At.UTC().Format(time.RFC3339Nano)
		}
		vals = vals[:cut]
	}
	for _, v := range vals {
		ts := "device"
		if src.Source == "status" {
			ts = "server"
		}
		out = append(out, telRow{telValue: v, TimeSource: ts})
	}
	return out, next, nil
}

// GET /api/v1/devices/{id}/telemetry/export?format=csv|xlsx
func (s *Server) deviceTelemetryExport(w http.ResponseWriter, r *http.Request) {
	org, devID, _, ok := s.telDevice(w, r)
	if !ok {
		return
	}
	src, ok := s.telSource(w, r, org, devID)
	if !ok {
		return
	}
	from, to, ok := telRange(w, r)
	if !ok {
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" {
		writeErr(w, 400, "format must be csv or xlsx")
		return
	}
	var ext string
	_ = s.pool.QueryRow(r.Context(), `SELECT external_id FROM core.device WHERE id = $1::uuid`, devID).Scan(&ext)
	rows, _, err := s.telRows(r, org, devID, src, from, to, "", telExportMaxRows)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	// oldest first in a file
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	unit := ""
	if src.DS != nil {
		unit = src.DS.Unit
	}
	header := []string{"measured_at", "received_at", "value", "value_json", "unit", "quality", "time_source", "annotation"}
	name := fmt.Sprintf("%s_%s_%s_%s", safeFile(ext), src.Source, src.Key, time.Now().UTC().Format("20060102T150405Z"))
	cellsOf := func(row telRow) []string {
		rAt := ""
		if row.ReceivedAt != nil {
			rAt = row.ReceivedAt.UTC().Format(time.RFC3339Nano)
		}
		val := ""
		if row.Value != nil {
			val = strconv.FormatFloat(*row.Value, 'g', -1, 64)
		}
		return []string{row.At.UTC().Format(time.RFC3339Nano), rAt, val, string(row.ValueJSON), unit, row.Quality, row.TimeSource, row.Annotation}
	}
	if format == "xlsx" {
		sh := xlsx.Sheet{Name: src.Key, FreezeRows: 1, Widths: []float64{26, 26, 14, 24, 8, 14, 12, 40}}
		hdr := make([]xlsx.Cell, len(header))
		for i, h := range header {
			hdr[i] = xlsx.B(h)
		}
		sh.Rows = append(sh.Rows, hdr)
		for _, row := range rows {
			c := cellsOf(row)
			line := make([]xlsx.Cell, len(c))
			for i, v := range c {
				line[i] = xlsx.T(v)
			}
			if row.Value != nil && !math.IsNaN(*row.Value) && !math.IsInf(*row.Value, 0) {
				line[2] = xlsx.N(*row.Value, decimalsOf(*row.Value))
			}
			sh.Rows = append(sh.Rows, line)
		}
		b, err := xlsx.Bytes(sh)
		if err != nil {
			writeErr(w, 500, "export failed")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, name))
		w.WriteHeader(200)
		_, _ = w.Write(b)
	} else {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
		cw := csv.NewWriter(w)
		_ = cw.Write(header)
		for _, row := range rows {
			_ = cw.Write(cellsOf(row))
		}
		cw.Flush()
	}
	s.audit(r, "export.read", "device", devID, nil, map[string]any{"export": "device_telemetry", "format": format, "rows": len(rows), "source": src.Source, "key": src.Key, "from": from, "to": to}, "")
}

// decimalsOf picks up to 6 decimals so a number keeps its digits in a spreadsheet.
func decimalsOf(v float64) int {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if d := len(s) - i - 1; d < 6 {
			return d
		}
		return 6
	}
	return 0
}

func safeFile(s string) string {
	b := []byte(s)
	for i, c := range b {
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			b[i] = '_'
		}
	}
	if len(b) > 48 {
		b = b[:48]
	}
	return string(b)
}

// POST /api/v1/devices/{id}/telemetry/assign: a datastream for a key the device publishes without one, assigned at
// once. Its earlier values stay in the raw messages only (the observations start with the next message).
func (s *Server) deviceTelemetryAssign(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	devID := r.PathValue("id")
	var in struct {
		Key      string   `json:"key"`
		AssetID  string   `json:"asset_id"`
		Quantity string   `json:"quantity"`
		Unit     string   `json:"unit"`
		Kind     string   `json:"kind"`
		Range    *dsRange `json:"range"`
		Reason   string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Key = strings.TrimSpace(in.Key)
	if !dsKeyRe.MatchString(in.Key) {
		writeErr(w, 400, "key must match ^[a-z][a-z0-9_]{0,31}$")
		return
	}
	in.Quantity = strings.TrimSpace(strings.ToLower(in.Quantity))
	if in.Quantity == "" {
		in.Quantity = in.Key
	}
	if len(in.Quantity) > 64 || len(in.Unit) > 16 {
		writeErr(w, 400, "quantity max 64 characters, unit max 16")
		return
	}
	switch in.Kind {
	case "gauge", "counter", "state", "event":
	case "":
		in.Kind = "gauge"
	default:
		writeErr(w, 400, "kind must be gauge, counter, state or event")
		return
	}
	if msg := in.Range.problem(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if in.Range == nil {
		in.Range = &dsRange{}
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(ctx)
	var okDev bool
	_ = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, devID, org).Scan(&okDev)
	if !okDev {
		writeErr(w, 404, "device not found")
		return
	}
	var taken bool
	_ = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.assignment a JOIN core.datastream d ON d.id = a.to_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL AND (d.key = $2 OR d.attributes->>'source_key' = $2))`, devID, in.Key).Scan(&taken)
	if taken {
		writeErr(w, 409, "the device already feeds a datastream with this key")
		return
	}
	var dsID string
	if err := tx.QueryRow(ctx, `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, attributes)
		SELECT $1::uuid, a.id, $3, $4, $5, $6, jsonb_strip_nulls(jsonb_build_object('range_min', $7::float8, 'range_max', $8::float8))
		FROM core.asset a WHERE a.id = $2::uuid AND a.org_id = $1::uuid RETURNING id::text`,
		org, in.AssetID, in.Key, in.Quantity, in.Unit, in.Kind, in.Range.Min, in.Range.Max).Scan(&dsID); err != nil {
		writeErr(w, 409, "datastream could not be created (asset not found, or the asset already has a datastream with this key)")
		return
	}
	var asgID string
	if err := tx.QueryRow(ctx, `INSERT INTO core.assignment (org_id, kind, from_id, to_id, changed_by, reason) VALUES ($1::uuid, 'device_datastream', $2::uuid, $3::uuid, NULLIF($4,'')::uuid, $5) RETURNING id::text`,
		org, devID, dsID, p.UserID, in.Reason).Scan(&asgID); err != nil {
		writeErr(w, 500, "assignment failed")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	if s.ing != nil {
		s.ing.InvalidateDevice(devID)
	}
	s.audit(r, "datastream.create", "datastream", dsID, nil, map[string]any{"asset_id": in.AssetID, "key": in.Key, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind, "range": in.Range, "from_device": devID}, in.Reason)
	s.audit(r, "assignment.create", "assignment", asgID, map[string]any{"closed_previous": 0}, map[string]any{"device_id": devID, "datastream_id": dsID}, in.Reason)
	writeJSON(w, 201, map[string]any{"datastream_id": dsID, "assignment_id": asgID, "key": in.Key, "asset_id": in.AssetID, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind})
}
