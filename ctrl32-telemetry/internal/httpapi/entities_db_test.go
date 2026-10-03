package httpapi

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Aliases never cross organizations: entities of another organization named by id (one entity, a list, the root of
// a hierarchy, the current entity of a state) resolve to nothing, and a hierarchy goes no deeper than asked. Needs a
// database with the schema (CTRL32_TEST_DB); skipped otherwise.
func TestEntitiesIsolationDB(t *testing.T) {
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
	type tenant struct{ org, site, room, fridge, ds string }
	var made []tenant
	mk := func(name string) tenant {
		var x tenant
		q := func(sql string, dst *string, args ...any) {
			if err := pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
				t.Fatal(err)
			}
		}
		q(`INSERT INTO core.organization (slug, name) VALUES ($1, $2) RETURNING id::text`, &x.org, "ent-"+name+"-"+suf, "Entities "+name)
		q(`INSERT INTO core.site (org_id, name) VALUES ($1::uuid, $2) RETURNING id::text`, &x.site, x.org, "Ent store "+name+" "+suf)
		q(`INSERT INTO core.asset (org_id, site_id, type, name) VALUES ($1::uuid, $2::uuid, 'room', $3) RETURNING id::text`, &x.room, x.org, x.site, "Ent room "+name)
		q(`INSERT INTO core.asset (org_id, site_id, parent_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, $3::uuid, 'fridge', $4, '{"max_temp": 8}') RETURNING id::text`, &x.fridge, x.org, x.site, x.room, "Ent fridge "+name)
		q(`INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind) VALUES ($1::uuid, $2::uuid, 'temp', 'temperature', '°C', 'gauge') RETURNING id::text`, &x.ds, x.org, x.fridge)
		made = append(made, x)
		return x
	}
	a, b := mk("a"), mk("b")
	defer func() {
		for _, x := range made {
			_, _ = pool.Exec(ctx, `DELETE FROM core.datastream WHERE id = $1::uuid`, x.ds)
			_, _ = pool.Exec(ctx, `DELETE FROM core.asset WHERE id = $1::uuid`, x.fridge)
			_, _ = pool.Exec(ctx, `DELETE FROM core.asset WHERE id = $1::uuid`, x.room)
			_, _ = pool.Exec(ctx, `DELETE FROM core.site WHERE id = $1::uuid`, x.site)
			_, _ = pool.Exec(ctx, `DELETE FROM core.organization WHERE id = $1::uuid`, x.org)
		}
	}()
	s := &Server{pool: pool}
	res := func(org string, al map[string]any, cur *entRef, want string) []map[string]any {
		out, err := s.resolve(ctx, org, &resolveReq{Alias: al, Current: cur, Want: want, Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		items, _ := out["items"].([]map[string]any)
		return items
	}
	foreign := []map[string]any{
		{"kind": "single", "entity": map[string]any{"type": "asset", "id": b.fridge}},
		{"kind": "list", "entity_type": "datastream", "ids": []any{b.ds}},
		{"kind": "hierarchy", "root": map[string]any{"type": "site", "id": b.site}, "entity_type": "datastream", "depth": 10},
		{"kind": "hierarchy", "root": map[string]any{"type": "asset", "id": b.room}, "entity_type": "asset", "depth": 10},
	}
	for _, al := range foreign {
		for _, want := range []string{"entities", "datastreams", "devices"} {
			if items := res(a.org, al, nil, want); len(items) != 0 {
				t.Errorf("organization a resolves %v of b (%s): %v", al, want, items)
			}
		}
	}
	if items := res(a.org, map[string]any{"kind": "current", "entity_type": "datastream"}, &entRef{"asset", b.fridge}, "entities"); len(items) != 0 {
		t.Errorf("a current entity of b: %v", items)
	}
	if items := res(a.org, map[string]any{"kind": "hierarchy", "root_current": true, "entity_type": "datastream"}, &entRef{"asset", b.room}, "entities"); len(items) != 0 {
		t.Errorf("a hierarchy under an entity of b: %v", items)
	}
	// own entities: the hierarchy finds the fridge's datastream two levels down from the site, not one
	if items := res(a.org, map[string]any{"kind": "hierarchy", "root": map[string]any{"type": "site", "id": a.site}, "entity_type": "datastream", "depth": 2.0}, nil, "entities"); len(items) != 1 || items[0]["id"] != a.ds {
		t.Errorf("own hierarchy: %v", items)
	}
	if items := res(a.org, map[string]any{"kind": "hierarchy", "root": map[string]any{"type": "site", "id": a.site}, "entity_type": "datastream", "depth": 1}, nil, "entities"); len(items) != 0 {
		t.Errorf("depth 1 reaches the fridge: %v", items)
	}
	// the type alias "*" (all assets) stays within the organization
	for _, it := range res(a.org, map[string]any{"kind": "type", "entity_type": "asset", "types": []any{"*"}}, nil, "entities") {
		if it["id"] == b.fridge || it["id"] == b.room {
			t.Errorf("all assets of a include %v", it)
		}
	}
}
