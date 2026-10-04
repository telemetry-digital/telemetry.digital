package counting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

// Engine records passages and speeds, keeps the occupancy of the areas, resets it daily and runs the camera workers.
type Engine struct {
	pool *pgxpool.Pool
	ing  *ingest.Ingest
	vid  *video.Manager
	cfg  config.AnalyticsConfig // guarded by cfgMu (SetAnalytics)
	log  *slog.Logger

	cfgMu      sync.RWMutex
	installing atomic.Bool // the server agent is installing the detector: no camera workers

	mu     sync.Mutex
	ds     map[string]string // asset id + "/" + key → datastream id
	entr   map[string]*entrance
	inputs map[string][]inputRef // datastream id → sensor inputs reading it
	last   map[string]float64    // total mode: input id + datastream id → previous reading
	cams   map[string]*camWorker
	status map[string]*CameraStatus

	jobs   chan func()
	reload chan struct{}

	checkMu sync.Mutex // detector self-check, cached
	check   *analytics.CheckResult
	checkAt time.Time
}

type entrance struct {
	ID, OrgID, AreaID, AreaAsset, Asset, Name, AreaName string
	Groups                                              []string
}

type inputRef struct {
	ID, Entrance, Group, Mode, Dir string
}

// New creates the engine; Run starts its loops.
func New(pool *pgxpool.Pool, ing *ingest.Ingest, vid *video.Manager, cfg config.AnalyticsConfig, log *slog.Logger) *Engine {
	return &Engine{pool: pool, ing: ing, vid: vid, cfg: cfg, log: log.With("component", "counting"),
		ds: map[string]string{}, entr: map[string]*entrance{}, inputs: map[string][]inputRef{}, last: map[string]float64{},
		cams: map[string]*camWorker{}, status: map[string]*CameraStatus{}, jobs: make(chan func(), 4096), reload: make(chan struct{}, 1)}
}

// Reload drops the caches and reconciles the camera workers (after a configuration change).
func (e *Engine) Reload() {
	e.mu.Lock()
	e.entr = map[string]*entrance{}
	e.mu.Unlock()
	e.loadInputs(context.Background())
	select {
	case e.reload <- struct{}{}:
	default:
	}
}

// Run runs the loops until ctx ends: sensor passages in order, the daily resets, the camera workers.
func (e *Engine) Run(ctx context.Context) {
	e.loadInputs(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-e.jobs:
				job()
			}
		}
	}()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	e.resets(ctx)
	e.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			e.stopAll()
			return
		case <-e.reload:
			e.reconcile(ctx)
		case <-tick.C:
			e.resets(ctx)
			e.reconcile(ctx)
			e.loadInputs(ctx)
		}
	}
}

// Datastream returns (creating when missing) the datastream of an asset for a key.
func (e *Engine) Datastream(ctx context.Context, org, asset, key, kind, quantity, unit string) (string, error) {
	ck := asset + "/" + key
	e.mu.Lock()
	id := e.ds[ck]
	e.mu.Unlock()
	if id != "" {
		return id, nil
	}
	err := e.pool.QueryRow(ctx, `SELECT id::text FROM core.datastream WHERE asset_id = $1::uuid AND key = $2`, asset, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = e.pool.Exec(ctx, `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, attributes)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, '{"source": "object counting"}'::jsonb) ON CONFLICT (asset_id, key) DO NOTHING`, org, asset, key, quantity, unit, kind)
		if err == nil {
			err = e.pool.QueryRow(ctx, `SELECT id::text FROM core.datastream WHERE asset_id = $1::uuid AND key = $2`, asset, key).Scan(&id)
		}
	}
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	e.ds[ck] = id
	e.mu.Unlock()
	return id, nil
}

// EnsureDatastreams creates the datastreams of an area and its entrances for the area's groups.
func (e *Engine) EnsureDatastreams(ctx context.Context, areaID string) error {
	var org, asset string
	var groups []string
	if err := e.pool.QueryRow(ctx, `SELECT org_id::text, asset_id::text, groups FROM core.counting_area WHERE id = $1::uuid`, areaID).Scan(&org, &asset, &groups); err != nil {
		return err
	}
	assets := []string{asset}
	rows, err := e.pool.Query(ctx, `SELECT asset_id::text FROM core.counting_entrance WHERE area_id = $1::uuid AND archived_at IS NULL`, areaID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a string
		if rows.Scan(&a) == nil {
			assets = append(assets, a)
		}
	}
	rows.Close()
	for i, a := range assets {
		for _, g := range groups {
			for _, dir := range []string{"in", "out"} {
				if _, err := e.Datastream(ctx, org, a, Key(g, dir), "event", Key(g, dir), Unit(g)); err != nil {
					return err
				}
			}
			if i == 0 {
				if _, err := e.Datastream(ctx, org, a, Key(g, "occupancy"), "gauge", "occupancy", Unit(g)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *Engine) entranceOf(ctx context.Context, id string) (*entrance, error) {
	e.mu.Lock()
	en := e.entr[id]
	e.mu.Unlock()
	if en != nil {
		return en, nil
	}
	en = &entrance{ID: id}
	err := e.pool.QueryRow(ctx, `SELECT en.org_id::text, a.id::text, a.asset_id::text, en.asset_id::text, en.name, a.name, a.groups
		FROM core.counting_entrance en JOIN core.counting_area a ON a.id = en.area_id
		WHERE en.id = $1::uuid AND en.archived_at IS NULL AND a.archived_at IS NULL`, id).Scan(&en.OrgID, &en.AreaID, &en.AreaAsset, &en.Asset, &en.Name, &en.AreaName, &en.Groups)
	if err != nil {
		return nil, fmt.Errorf("entrance %s: %w", id, err)
	}
	e.mu.Lock()
	e.entr[id] = en
	e.mu.Unlock()
	return en, nil
}

// Record adds n passages of the groups through an entrance in a direction (in | out) at a moment: the entrance's and
// the area's <group>_<dir> datastreams get n, the area's <group>_occupancy the new occupancy (clamped at zero).
func (e *Engine) Record(ctx context.Context, entranceID string, groups []string, dir string, n int64, at time.Time) error {
	if n <= 0 || (dir != "in" && dir != "out") {
		return nil
	}
	en, err := e.entranceOf(ctx, entranceID)
	if err != nil {
		return err
	}
	var gs []string
	for _, g := range groups {
		if slices.Contains(en.Groups, g) {
			gs = append(gs, g)
		}
	}
	if len(gs) == 0 {
		return nil // the area does not count this
	}
	var list []ingest.Counted
	for _, g := range gs {
		dsE, err := e.Datastream(ctx, en.OrgID, en.Asset, Key(g, dir), "event", Key(g, dir), Unit(g))
		if err != nil {
			return err
		}
		dsA, err := e.Datastream(ctx, en.OrgID, en.AreaAsset, Key(g, dir), "event", Key(g, dir), Unit(g))
		if err != nil {
			return err
		}
		dsO, err := e.Datastream(ctx, en.OrgID, en.AreaAsset, Key(g, "occupancy"), "gauge", "occupancy", Unit(g))
		if err != nil {
			return err
		}
		in, out := int64(0), int64(0)
		if dir == "in" {
			in = n
		} else {
			out = n
		}
		occ, clamped, err := e.adjust(ctx, en, g, in, out)
		if err != nil {
			return err
		}
		q := ""
		if clamped {
			q = "out_of_range" // more departures than the area held: the occupancy stays at zero, the value is flagged
		}
		list = append(list,
			ingest.Counted{OrgID: en.OrgID, DatastreamID: dsE, Key: Key(g, dir), At: at, Value: float64(n)},
			ingest.Counted{OrgID: en.OrgID, DatastreamID: dsA, Key: Key(g, dir), At: at, Value: float64(n)},
			ingest.Counted{OrgID: en.OrgID, DatastreamID: dsO, Key: Key(g, "occupancy"), At: at, Value: float64(occ), Quality: q})
	}
	return e.ing.StoreCounted(ctx, list)
}

func (e *Engine) adjust(ctx context.Context, en *entrance, g string, in, out int64) (int64, bool, error) {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO core.counting_occupancy (area_id, grp, last_reset_at) VALUES ($1::uuid, $2, now()) ON CONFLICT DO NOTHING`, en.AreaID, g); err != nil {
		return 0, false, err
	}
	var occ int64
	if err := tx.QueryRow(ctx, `SELECT occupancy FROM core.counting_occupancy WHERE area_id = $1::uuid AND grp = $2 FOR UPDATE`, en.AreaID, g).Scan(&occ); err != nil {
		return 0, false, err
	}
	n, clamped := Apply(occ, in, out)
	add := 0
	if clamped {
		add = 1
		if _, err := tx.Exec(ctx, `INSERT INTO core.counting_log (org_id, area_id, grp, kind, before, after, reason, actor) VALUES ($1::uuid, $2::uuid, $3, 'clamp', $4, 0, $5, 'system')`,
			en.OrgID, en.AreaID, g, occ+in-out, "more departures than the area held at "+en.Name); err != nil {
			return 0, false, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE core.counting_occupancy SET occupancy = $3, clamped = clamped + $4, updated_at = now() WHERE area_id = $1::uuid AND grp = $2`, en.AreaID, g, n, add); err != nil {
		return 0, false, err
	}
	return n, clamped, tx.Commit(ctx)
}

// Correct sets the occupancy of a group of an area (a count by hand); the reason goes to the counting log (the
// caller audits the change).
func (e *Engine) Correct(ctx context.Context, areaID, grp string, value int64, reason, actor string) (before int64, err error) {
	var org, asset string
	if err := e.pool.QueryRow(ctx, `SELECT org_id::text, asset_id::text FROM core.counting_area WHERE id = $1::uuid`, areaID).Scan(&org, &asset); err != nil {
		return 0, err
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO core.counting_occupancy (area_id, grp, last_reset_at) VALUES ($1::uuid, $2, now()) ON CONFLICT DO NOTHING`, areaID, grp); err != nil {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `SELECT occupancy FROM core.counting_occupancy WHERE area_id = $1::uuid AND grp = $2 FOR UPDATE`, areaID, grp).Scan(&before); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.counting_occupancy SET occupancy = $3, updated_at = now() WHERE area_id = $1::uuid AND grp = $2`, areaID, grp, value); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.counting_log (org_id, area_id, grp, kind, before, after, reason, actor) VALUES ($1::uuid, $2::uuid, $3, 'correction', $4, $5, $6, $7)`,
		org, areaID, grp, before, value, reason, actor); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	ds, err := e.Datastream(ctx, org, asset, Key(grp, "occupancy"), "gauge", "occupancy", Unit(grp))
	if err != nil {
		return before, err
	}
	return before, e.ing.StoreCounted(ctx, []ingest.Counted{{OrgID: org, DatastreamID: ds, Key: Key(grp, "occupancy"), At: time.Now(), Value: float64(value), JSON: map[string]any{"correction": true}}})
}

// resets sets the occupancy to zero at the daily reset time of each area (also when the server was down then).
func (e *Engine) resets(ctx context.Context) {
	rows, err := e.pool.Query(ctx, `SELECT a.id::text, a.org_id::text, a.asset_id::text, a.reset_time, a.groups, o.tz
		FROM core.counting_area a JOIN core.organization o ON o.id = a.org_id WHERE a.reset_time IS NOT NULL AND a.archived_at IS NULL`)
	if err != nil {
		return
	}
	type area struct {
		id, org, asset, at, tz string
		groups                 []string
	}
	var list []area
	for rows.Next() {
		var a area
		if rows.Scan(&a.id, &a.org, &a.asset, &a.at, &a.groups, &a.tz) == nil {
			list = append(list, a)
		}
	}
	rows.Close()
	now := time.Now()
	for _, a := range list {
		loc, err := time.LoadLocation(a.tz)
		if err != nil {
			loc = time.UTC
		}
		for _, g := range a.groups {
			if err := e.resetOne(ctx, a.id, a.org, a.asset, g, a.at, loc, now); err != nil {
				e.log.Warn("occupancy reset failed", "area", a.id, "group", g, "err", err)
			}
		}
	}
}

func (e *Engine) resetOne(ctx context.Context, areaID, org, asset, g, at string, loc *time.Location, now time.Time) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO core.counting_occupancy (area_id, grp, last_reset_at) VALUES ($1::uuid, $2, now()) ON CONFLICT DO NOTHING`, areaID, g); err != nil {
		return err
	}
	var occ int64
	var last *time.Time
	if err := tx.QueryRow(ctx, `SELECT occupancy, last_reset_at FROM core.counting_occupancy WHERE area_id = $1::uuid AND grp = $2 FOR UPDATE`, areaID, g).Scan(&occ, &last); err != nil {
		return err
	}
	var lr time.Time
	if last != nil {
		lr = *last
	}
	when, due := ResetDue(lr, now, at, loc)
	if !due {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE core.counting_occupancy SET occupancy = 0, clamped = 0, last_reset_at = $3, updated_at = now() WHERE area_id = $1::uuid AND grp = $2`, areaID, g, when); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.counting_log (org_id, area_id, grp, at, kind, before, after, reason, actor) VALUES ($1::uuid, $2::uuid, $3, $4, 'reset', $5, 0, $6, 'system')`,
		org, areaID, g, when, occ, "daily reset at "+at); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	ds, err := e.Datastream(ctx, org, asset, Key(g, "occupancy"), "gauge", "occupancy", Unit(g))
	if err != nil {
		return err
	}
	return e.ing.StoreCounted(ctx, []ingest.Counted{{OrgID: org, DatastreamID: ds, Key: Key(g, "occupancy"), At: when, Value: 0, JSON: map[string]any{"reset": true}}})
}

// RecordSpeed stores the speed of a vehicle through a speed section (datastream vehicle_speed on the section's asset,
// with class and direction) and, above the section's limit, a passage on over_limit.
func (e *Engine) RecordSpeed(ctx context.Context, org, asset string, kmh float64, class, dir string, at time.Time, limit float64) error {
	ds, err := e.Datastream(ctx, org, asset, "vehicle_speed", "gauge", "speed", "km/h")
	if err != nil {
		return err
	}
	list := []ingest.Counted{{OrgID: org, DatastreamID: ds, Key: "vehicle_speed", At: at, Value: math.Round(kmh*10) / 10, JSON: map[string]any{"class": class, "direction": dir}}}
	if limit > 0 && kmh > limit {
		ol, err := e.Datastream(ctx, org, asset, "over_limit", "event", "over_limit", "vehicles")
		if err != nil {
			return err
		}
		list = append(list, ingest.Counted{OrgID: org, DatastreamID: ol, Key: "over_limit", At: at, Value: 1, JSON: map[string]any{"kmh": math.Round(kmh*10) / 10, "class": class}})
	}
	return e.ing.StoreCounted(ctx, list)
}

// ---- sensors ---------------------------------------------------------------------------------------------------

// InputGroups are the groups a sensor of a group counts for (a car counter also counts vehicles).
func InputGroups(g string) []string {
	switch g {
	case "cars", "trucks", "buses", "motorcycles":
		return []string{"vehicles", g}
	}
	return []string{g}
}

func (e *Engine) loadInputs(ctx context.Context) {
	rows, err := e.pool.Query(ctx, `SELECT i.id::text, i.entrance_id::text, i.grp, i.mode, coalesce(i.in_datastream_id::text, ''), coalesce(i.out_datastream_id::text, ''), i.last_values::text
		FROM core.counting_input i JOIN core.counting_entrance en ON en.id = i.entrance_id
		WHERE i.enabled AND i.archived_at IS NULL AND en.archived_at IS NULL`)
	if err != nil {
		return
	}
	defer rows.Close()
	m := map[string][]inputRef{}
	last := map[string]float64{}
	for rows.Next() {
		var id, en, g, mode, in, out, lv string
		if rows.Scan(&id, &en, &g, &mode, &in, &out, &lv) != nil {
			continue
		}
		var vals map[string]float64
		_ = json.Unmarshal([]byte(lv), &vals)
		for ds, v := range vals {
			last[id+"/"+ds] = v
		}
		if in != "" {
			m[in] = append(m[in], inputRef{ID: id, Entrance: en, Group: g, Mode: mode, Dir: "in"})
		}
		if out != "" {
			m[out] = append(m[out], inputRef{ID: id, Entrance: en, Group: g, Mode: mode, Dir: "out"})
		}
	}
	e.mu.Lock()
	e.inputs = m
	for k, v := range last {
		if _, ok := e.last[k]; !ok {
			e.last[k] = v
		}
	}
	e.mu.Unlock()
}

// OnObservations receives every stored observation (ingest sink): readings of sensor datastreams become passages.
func (e *Engine) OnObservations(ctx context.Context, obs []ingest.Observation) {
	e.mu.Lock()
	if len(e.inputs) == 0 {
		e.mu.Unlock()
		return
	}
	type pass struct {
		ref inputRef
		n   int64
		at  time.Time
		ds  string
		v   float64
	}
	var todo []pass
	for _, o := range obs {
		refs := e.inputs[o.DatastreamID]
		if len(refs) == 0 || o.Value == nil || math.IsNaN(*o.Value) {
			continue
		}
		v := *o.Value
		for _, r := range refs {
			var n float64
			if r.Mode == "total" {
				k := r.ID + "/" + o.DatastreamID
				prev, ok := e.last[k]
				e.last[k] = v
				switch {
				case !ok:
					n = 0 // the first reading is the baseline
				case v >= prev:
					n = v - prev
				default:
					n = v // the counter restarted from zero
				}
			} else {
				n = v
			}
			if n <= 0 || n > 100000 {
				continue
			}
			todo = append(todo, pass{r, int64(math.Round(n)), o.MeasuredAt, o.DatastreamID, v})
		}
	}
	e.mu.Unlock()
	for _, p := range todo {
		// the sink waits when the queue is full (a device sending its history): slower, but nothing is lost
		select {
		case <-time.After(2 * time.Minute):
			e.log.Warn("sensor passages queue stuck: a passage was dropped", "input", p.ref.ID)
		case e.jobs <- func() {
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := e.Record(c, p.ref.Entrance, InputGroups(p.ref.Group), p.ref.Dir, p.n, p.at); err != nil {
				e.log.Warn("sensor passage not recorded", "input", p.ref.ID, "err", err)
			}
			if p.ref.Mode == "total" {
				_, _ = e.pool.Exec(c, `UPDATE core.counting_input SET last_values = last_values || jsonb_build_object($2::text, $3::float8) WHERE id = $1::uuid`, p.ref.ID, p.ds, p.v)
			}
		}:
		}
	}
}

// IsCountingAsset reports whether a datastream belongs to object counting (it cannot be a sensor input).
func (e *Engine) IsCountingAsset(ctx context.Context, datastreamID string) bool {
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND attributes->>'source' = 'object counting'`, datastreamID).Scan(&n)
	return n > 0
}

func nonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
