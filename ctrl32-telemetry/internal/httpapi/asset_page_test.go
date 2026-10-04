package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The position of an asset or a site lives in its attributes as lat and lon; setting it keeps every other attribute,
// clearing it removes both, and nothing else changes when neither is asked for.
func TestWithLocation(t *testing.T) {
	attrs := json.RawMessage(`{"floor":"ground","lat":1,"lon":2,"n":3}`)
	if got := withLocation(attrs, nil, false); string(got) != string(attrs) {
		t.Fatalf("untouched: %s", got)
	}
	var m map[string]any
	_ = json.Unmarshal(withLocation(attrs, &geoPoint{Lat: 48.3, Lon: 18.1}, false), &m)
	if m["lat"] != 48.3 || m["lon"] != 18.1 || m["floor"] != "ground" || m["n"] != 3.0 {
		t.Fatalf("set: %v", m)
	}
	m = nil
	_ = json.Unmarshal(withLocation(attrs, nil, true), &m)
	if _, ok := m["lat"]; ok || m["floor"] != "ground" || len(m) != 2 {
		t.Fatalf("clear: %v", m)
	}
	m = nil
	_ = json.Unmarshal(withLocation(json.RawMessage(`null`), &geoPoint{Lat: 0, Lon: 0}, false), &m)
	if m["lat"] != 0.0 || m["lon"] != 0.0 {
		t.Fatalf("from null attributes: %v", m)
	}
	if l := locationOf(json.RawMessage(`{"lat":48.3,"lon":18.1}`)); l == nil || l["lat"] != 48.3 {
		t.Fatalf("locationOf: %v", l)
	}
	for _, s := range []string{`{}`, `{"lat":48.3}`, `{"lat":"48","lon":"18"}`, `null`, ``} {
		if l := locationOf(json.RawMessage(s)); l != nil {
			t.Errorf("locationOf(%s) = %v, want none", s, l)
		}
	}
}

func TestGeoPointProblem(t *testing.T) {
	var none *geoPoint
	for _, g := range []*geoPoint{none, {Lat: 90, Lon: 180}, {Lat: -90, Lon: -180}, {Lat: 48.1, Lon: 17.1}} {
		if msg := g.problem(); msg != "" {
			t.Errorf("%+v: %s", g, msg)
		}
	}
	for _, g := range []*geoPoint{{Lat: 90.01}, {Lat: -91}, {Lon: 180.5}, {Lon: -181}, {Lat: math.NaN()}} {
		if msg := g.problem(); msg == "" {
			t.Errorf("%+v accepted", g)
		}
	}
}

// The asset and site pages never cross organizations, and a loop written directly into the database cannot make the
// hierarchy walks run away: two organizations, each with a room, a fridge in the room and a shelf in the fridge.
// Needs a database with the schema (CTRL32_TEST_DB); skipped otherwise.
func TestAssetPageIsolationDB(t *testing.T) {
	url := os.Getenv("CTRL32_TEST_DB")
	if url == "" {
		t.Skip("CTRL32_TEST_DB not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suf := fmt.Sprintf("%d", time.Now().UnixNano()%100000000)
	type tenant struct{ org, site, room, fridge, shelf, ds, dev string }
	mk := func(name string) tenant {
		var x tenant
		q := func(sql string, dst *string, args ...any) {
			if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
				t.Fatal(sql, err)
			}
		}
		q(`INSERT INTO core.organization (slug, name) VALUES ($1, $2) RETURNING id::text`, &x.org, "ap-"+name+"-"+suf, "Asset page "+name)
		q(`INSERT INTO core.site (org_id, name, attributes) VALUES ($1::uuid, $2, '{"lat": 48.1, "lon": 17.1}') RETURNING id::text`, &x.site, x.org, "Site "+name+" "+suf)
		q(`INSERT INTO core.asset (org_id, site_id, type, name) VALUES ($1::uuid, $2::uuid, 'room', $3) RETURNING id::text`, &x.room, x.org, x.site, "Room "+name)
		q(`INSERT INTO core.asset (org_id, site_id, parent_id, type, name) VALUES ($1::uuid, $2::uuid, $3::uuid, 'fridge', $4) RETURNING id::text`, &x.fridge, x.org, x.site, x.room, "Fridge "+name)
		q(`INSERT INTO core.asset (org_id, site_id, parent_id, type, name) VALUES ($1::uuid, $2::uuid, $3::uuid, 'shelf', $4) RETURNING id::text`, &x.shelf, x.org, x.site, x.fridge, "Shelf "+name)
		q(`INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind) VALUES ($1::uuid, $2::uuid, 'temp', 'temperature', 'C', 'gauge') RETURNING id::text`, &x.ds, x.org, x.fridge)
		q(`INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $2, 'http') RETURNING id::text`, &x.dev, x.org, "ap-dev-"+name+"-"+suf)
		var asg string
		q(`INSERT INTO core.assignment (org_id, kind, from_id, to_id, reason) VALUES ($1::uuid, 'device_datastream', $2::uuid, $3::uuid, 'test') RETURNING id::text`, &asg, x.org, x.dev, x.ds)
		return x
	}
	a, b := mk("a"), mk("b")
	defer func() {
		for _, x := range []tenant{a, b} {
			_, _ = pool.Exec(ctx, `DELETE FROM core.assignment WHERE to_id = $1::uuid`, x.ds)
			_, _ = pool.Exec(ctx, `DELETE FROM core.device WHERE id = $1::uuid`, x.dev)
			_, _ = pool.Exec(ctx, `DELETE FROM core.datastream WHERE id = $1::uuid`, x.ds)
			_, _ = pool.Exec(ctx, `UPDATE core.asset SET parent_id = NULL WHERE site_id = $1::uuid`, x.site)
			_, _ = pool.Exec(ctx, `DELETE FROM core.asset WHERE site_id = $1::uuid`, x.site)
			_, _ = pool.Exec(ctx, `DELETE FROM core.site WHERE id = $1::uuid`, x.site)
			_, _ = pool.Exec(ctx, `DELETE FROM core.organization WHERE id = $1::uuid`, x.org)
		}
	}()
	s := &Server{pool: pool}
	rows, err := s.assetRows(ctx, a.org, a.site, "", "")
	if err != nil || len(rows) != 3 {
		t.Fatalf("assets of a: %d %v", len(rows), err)
	}
	by := map[string]assetRow{}
	for _, r := range rows {
		by[r.ID] = r
	}
	if r := by[a.fridge]; r.ParentID != a.room || r.Datastreams != 1 || r.Devices != 1 || r.Children != 1 || r.Alarm != nil {
		t.Errorf("fridge row: %+v", r)
	}
	if r := by[a.room]; r.Children != 1 || r.Datastreams != 0 || r.Devices != 0 {
		t.Errorf("room row: %+v", r)
	}
	// the other organization's site, asset and datastreams give nothing
	if rows, _ := s.assetRows(ctx, a.org, b.site, "", ""); len(rows) != 0 {
		t.Errorf("a sees %d assets of b's site", len(rows))
	}
	if rows, _ := s.assetRows(ctx, a.org, "", "", b.room); len(rows) != 0 {
		t.Errorf("a sees b's room")
	}
	if ids, _ := s.assetIDs(ctx, a.org, b.room, true); len(ids) != 0 {
		t.Errorf("a walks b's tree: %v", ids)
	}
	if d := s.feedingDevices(ctx, a.org, []string{b.ds}); len(d) != 0 {
		t.Errorf("a sees b's devices: %v", d)
	}
	if dss, _ := s.assetDatastreams(ctx, a.org, []string{b.fridge}); len(dss) != 0 {
		t.Errorf("a sees b's datastreams: %v", dss)
	}
	ids, err := s.assetIDs(ctx, a.org, a.room, true)
	sort.Strings(ids)
	want := []string{a.room, a.fridge, a.shelf}
	sort.Strings(want)
	if err != nil || strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("tree of a's room: %v %v", ids, err)
	}
	if ids, _ := s.assetIDs(ctx, a.org, a.room, false); len(ids) != 1 {
		t.Errorf("without children: %v", ids)
	}
	dss, err := s.assetDatastreams(ctx, a.org, []string{a.fridge})
	if err != nil || len(dss) != 1 || dss[0].DeviceID != a.dev || dss[0].AssetID != a.fridge {
		t.Fatalf("datastreams of a's fridge: %+v %v", dss, err)
	}
	// a loop written directly into the database: the walk still ends, every asset once
	if _, err := pool.Exec(ctx, `UPDATE core.asset SET parent_id = $1::uuid WHERE id = $2::uuid`, a.shelf, a.room); err != nil {
		t.Fatal(err)
	}
	ids, err = s.assetIDs(ctx, a.org, a.fridge, true)
	if err != nil || len(ids) != 3 {
		t.Fatalf("walk over a loop: %v %v", ids, err)
	}
}
