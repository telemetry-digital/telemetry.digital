package httpapi

// Energy overview (spec 008): consumption and production per hour, day or month from energy meters — smart-home
// entities with device_class energy and any datastream measured in Wh, kWh or MWh — with cost at the organization's
// price. Meters are cumulative: consumption is the sum of increases between consecutive readings; a drop below half the
// previous reading is a meter reset and counts the new reading, a smaller drop (a device reporting a slightly older
// total after a restart) counts nothing.
//
//	GET /api/v1/energy?period=day|week|month|year&date=YYYY-MM-DD   data.read
//	GET /api/v1/energy/settings                                     data.read
//	PUT /api/v1/energy/settings                                     content.write {price_per_kwh, currency, sources, reason}

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// EnergySource is a meter taken into the overview.
type EnergySource struct {
	DatastreamID string `json:"datastream_id"`
	Role         string `json:"role"` // consumption | production | off
}

// EnergySettings live in organization.settings.energy.
type EnergySettings struct {
	PricePerKWh   float64        `json:"price_per_kwh"`
	FeedInPerKWh  float64        `json:"feed_in_per_kwh"` // price paid for produced energy (0 = not counted)
	Currency      string         `json:"currency"`
	Sources       []EnergySource `json:"sources"` // explicit roles; meters not listed count as consumption
	ExcludeOthers bool           `json:"exclude_others"`
}

type energyMeter struct {
	ID, Name, Device, Unit, Role, EntityID string
	factor                                 float64 // to kWh
}

var currencyRe = regexp.MustCompile(`^[A-Za-z€$£¥]{1,4}$`)

func unitFactor(unit string) float64 {
	switch strings.TrimSpace(unit) {
	case "Wh":
		return 0.001
	case "kWh":
		return 1
	case "MWh":
		return 1000
	}
	return 0
}

func (s *Server) energySettings(ctx context.Context, org string) EnergySettings {
	st := EnergySettings{Currency: "€"}
	var raw []byte
	if s.pool.QueryRow(ctx, `SELECT coalesce(settings->'energy', 'null') FROM core.organization WHERE id = $1::uuid`, org).Scan(&raw) == nil {
		_ = json.Unmarshal(raw, &st)
	}
	if st.Currency == "" {
		st.Currency = "€"
	}
	return st
}

// energyMeters lists the organization's energy meters with their roles.
func (s *Server) energyMeters(ctx context.Context, org string, st EnergySettings) ([]energyMeter, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.id::text, coalesce(e.name, a.name || ' / ' || d.key), coalesce(dev.name, a.name), d.unit, coalesce(e.id::text, ''),
		  coalesce(e.config->>'unit_of_measurement', '')
		FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id
		LEFT JOIN core.entity e ON e.datastream_id = d.id AND e.status <> 'removed'
		LEFT JOIN core.device dev ON dev.id = e.device_id
		WHERE d.org_id = $1::uuid AND (d.unit IN ('Wh', 'kWh', 'MWh') OR e.config->>'device_class' = 'energy')
		ORDER BY 2`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := map[string]string{}
	for _, src := range st.Sources {
		roles[src.DatastreamID] = src.Role
	}
	var out []energyMeter
	seen := map[string]bool{}
	var devs []string // device of each meter (smart-home entities), to count one meter per device by default
	for rows.Next() {
		var m energyMeter
		var entUnit string
		if err := rows.Scan(&m.ID, &m.Name, &m.Device, &m.Unit, &m.EntityID, &entUnit); err != nil {
			return nil, err
		}
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		if m.Unit == "" {
			m.Unit = entUnit
		}
		m.factor = unitFactor(m.Unit)
		if m.factor == 0 {
			continue
		}
		m.Role = roles[m.ID]
		dev := ""
		if m.EntityID != "" {
			dev = m.Device
		}
		out = append(out, m)
		devs = append(devs, dev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Defaults for meters without an explicit role: a device with several energy sensors (Tasmota: Total, Today,
	// Yesterday) counts one of them — the one named "total", else the first — so energy is not counted twice.
	chosen := map[string]int{}
	for i, m := range out {
		if devs[i] == "" || roles[m.ID] != "" {
			continue
		}
		j, ok := chosen[devs[i]]
		if !ok || (!strings.Contains(strings.ToLower(out[j].Name), "total") && strings.Contains(strings.ToLower(m.Name), "total")) {
			chosen[devs[i]] = i
		}
	}
	for i := range out {
		if out[i].Role != "" {
			continue
		}
		out[i].Role = "consumption"
		if j, ok := chosen[devs[i]]; (ok && j != i) || st.ExcludeOthers {
			out[i].Role = "off"
		}
	}
	return out, nil
}

// energyRange returns the period [from, to) around date and the bucket size.
func energyRange(period string, date time.Time, loc *time.Location) (from, to time.Time, bucket string) {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	switch period {
	case "week":
		wd := (int(d.Weekday()) + 6) % 7 // Monday first
		from = d.AddDate(0, 0, -wd)
		return from, from.AddDate(0, 0, 7), "day"
	case "month":
		from = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(0, 1, 0), "day"
	case "year":
		from = time.Date(d.Year(), 1, 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(1, 0, 0), "month"
	}
	return d, d.AddDate(0, 0, 1), "hour"
}

func (s *Server) orgLoc(ctx context.Context, org string) *time.Location {
	var tz string
	_ = s.pool.QueryRow(ctx, `SELECT tz FROM core.organization WHERE id = $1::uuid`, org).Scan(&tz)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

type energyBucket struct {
	Start       time.Time `json:"start"`
	Consumption float64   `json:"consumption"`
	Production  float64   `json:"production"`
	Cost        float64   `json:"cost"`
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func (s *Server) energy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	switch period {
	case "":
		period = "day"
	case "day", "week", "month", "year":
	default:
		writeErr(w, 400, "period must be day, week, month or year")
		return
	}
	loc := s.orgLoc(r.Context(), org)
	date := time.Now().In(loc)
	if v := r.URL.Query().Get("date"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, loc)
		if err != nil {
			writeErr(w, 400, "date must be YYYY-MM-DD")
			return
		}
		date = t
	}
	from, to, bucket := energyRange(period, date, loc)
	st := s.energySettings(r.Context(), org)
	meters, err := s.energyMeters(r.Context(), org, st)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	byID := map[string]*energyMeter{}
	var ids []string
	for i := range meters {
		if meters[i].Role != "off" {
			byID[meters[i].ID] = &meters[i]
			ids = append(ids, meters[i].ID)
		}
	}
	// buckets
	var starts []time.Time
	for t := from; t.Before(to); {
		starts = append(starts, t)
		switch bucket {
		case "hour":
			t = t.Add(time.Hour)
		case "day":
			t = t.AddDate(0, 0, 1)
		default:
			t = t.AddDate(0, 1, 0)
		}
	}
	buckets := make([]energyBucket, len(starts))
	for i, t := range starts {
		buckets[i].Start = t
	}
	idx := func(t time.Time) int {
		i := sort.Search(len(starts), func(i int) bool { return starts[i].After(t) }) - 1
		return i
	}
	totals := map[string]float64{}
	if len(ids) > 0 {
		// increases between consecutive readings, summed per bucket in the organization's time zone; the reading
		// before the period is the baseline
		rows, err := s.pool.Query(r.Context(), `WITH o AS (
			  SELECT datastream_id, measured_at, value, lag(value) OVER (PARTITION BY datastream_id ORDER BY measured_at) AS prev
			  FROM core.observation
			  WHERE datastream_id = ANY($1::uuid[]) AND value IS NOT NULL AND measured_at < $3
			    AND measured_at >= $2::timestamptz - interval '2 days')
			SELECT datastream_id::text, date_trunc($4, measured_at AT TIME ZONE $5),
			  sum(CASE WHEN value >= prev THEN value - prev WHEN value < prev * 0.5 THEN value ELSE 0 END)::float8
			FROM o WHERE measured_at >= $2 AND prev IS NOT NULL GROUP BY 1, 2`, ids, from, to, bucket, loc.String())
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for rows.Next() {
			var id string
			var at time.Time
			var d float64
			if rows.Scan(&id, &at, &d) != nil {
				continue
			}
			m := byID[id]
			local := time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), 0, 0, 0, loc)
			i := idx(local)
			if m == nil || i < 0 || d < 0 {
				continue
			}
			kwh := d * m.factor
			totals[id] += kwh
			if m.Role == "production" {
				buckets[i].Production += kwh
			} else {
				buckets[i].Consumption += kwh
			}
		}
		rows.Close()
	}
	var sum energyBucket
	for i := range buckets {
		b := &buckets[i]
		b.Cost = round3(b.Consumption*st.PricePerKWh - b.Production*st.FeedInPerKWh)
		b.Consumption, b.Production = round3(b.Consumption), round3(b.Production)
		sum.Consumption += b.Consumption
		sum.Production += b.Production
		sum.Cost += b.Cost
	}
	sources := []map[string]any{}
	for _, m := range meters {
		sh := 0.0
		if m.Role == "consumption" && sum.Consumption > 0 {
			sh = math.Round(totals[m.ID]/sum.Consumption*1000) / 10
		}
		sources = append(sources, map[string]any{"datastream_id": m.ID, "entity_id": m.EntityID, "name": m.Name, "device": m.Device, "unit": m.Unit,
			"role": m.Role, "kwh": round3(totals[m.ID]), "share": sh})
	}
	sort.SliceStable(sources, func(i, j int) bool { return sources[i]["kwh"].(float64) > sources[j]["kwh"].(float64) })
	// current power: latest state of power entities (W, kW)
	var powerW float64
	prows, err := s.pool.Query(r.Context(), `SELECT state, coalesce(config->>'unit_of_measurement', '') FROM core.entity
		WHERE org_id = $1::uuid AND status = 'active' AND available AND config->>'device_class' = 'power'`, org)
	if err == nil {
		for prows.Next() {
			var state *string
			var unit string
			if prows.Scan(&state, &unit) == nil && state != nil {
				if f, ok := toFloat(*state); ok {
					if unit == "kW" {
						f *= 1000
					}
					powerW += f
				}
			}
		}
		prows.Close()
	}
	writeJSON(w, 200, map[string]any{"period": period, "from": from, "to": to, "bucket": bucket, "tz": loc.String(), "unit": "kWh",
		"currency": st.Currency, "price_per_kwh": st.PricePerKWh, "feed_in_per_kwh": st.FeedInPerKWh, "buckets": buckets,
		"total":   map[string]any{"consumption": round3(sum.Consumption), "production": round3(sum.Production), "cost": round3(sum.Cost)},
		"sources": sources, "power_w": math.Round(powerW*10) / 10})
}

func toFloat(s string) (float64, bool) {
	var f float64
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func (s *Server) getEnergySettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	st := s.energySettings(r.Context(), org)
	meters, err := s.energyMeters(r.Context(), org, st)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	list := []map[string]any{}
	for _, m := range meters {
		list = append(list, map[string]any{"datastream_id": m.ID, "name": m.Name, "device": m.Device, "unit": m.Unit, "role": m.Role})
	}
	writeJSON(w, 200, map[string]any{"price_per_kwh": st.PricePerKWh, "feed_in_per_kwh": st.FeedInPerKWh, "currency": st.Currency,
		"exclude_others": st.ExcludeOthers, "meters": list})
}

func (s *Server) putEnergySettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		EnergySettings
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st := in.EnergySettings
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	if st.PricePerKWh < 0 || st.PricePerKWh > 100 || st.FeedInPerKWh < 0 || st.FeedInPerKWh > 100 {
		writeErr(w, 400, "prices must be between 0 and 100 per kWh")
		return
	}
	st.Currency = strings.TrimSpace(st.Currency)
	if st.Currency == "" {
		st.Currency = "€"
	}
	if !currencyRe.MatchString(st.Currency) {
		writeErr(w, 400, "currency: 1 to 4 letters or a currency sign")
		return
	}
	if len(st.Sources) > 500 {
		writeErr(w, 400, "at most 500 sources")
		return
	}
	for _, src := range st.Sources {
		if src.Role != "consumption" && src.Role != "production" && src.Role != "off" {
			writeErr(w, 400, "source role must be consumption, production or off")
			return
		}
		var n int
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, src.DatastreamID, org).Scan(&n)
		if n == 0 {
			writeErr(w, 400, "datastream "+src.DatastreamID+" not found")
			return
		}
	}
	old := s.energySettings(r.Context(), org)
	raw, _ := json.Marshal(st)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(coalesce(settings, '{}'), '{energy}', $2::jsonb) WHERE id = $1::uuid`, org, string(raw)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "energy.settings", "organization", org, old, st, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}
