package httpapi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Overview queries, filter choices and the global search never cross organizations: two organizations with a
// fridge each, queried by the ids, attributes and names of the other. Needs a database with the schema
// (CTRL32_TEST_DB, the superuser URL of a development or test database); skipped otherwise.
func TestOverviewIsolationDB(t *testing.T) {
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
	type tenant struct{ org, site, asset, ds string }
	mk := func(name string) tenant {
		var x tenant
		if err := pool.QueryRow(ctx, `INSERT INTO core.organization (slug, name) VALUES ($1, $2) RETURNING id::text`, "ovq-"+name+"-"+suf, "Overview "+name).Scan(&x.org); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO core.site (org_id, name) VALUES ($1::uuid, $2) RETURNING id::text`, x.org, "Store "+name+" "+suf).Scan(&x.site); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, 'fridge', $3, '{"storage_class": "2-8C", "owner": "`+name+`"}') RETURNING id::text`, x.org, x.site, "Ovfridge "+name+" "+suf).Scan(&x.asset); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind) VALUES ($1::uuid, $2::uuid, 'temp', 'temperature', '°C', 'gauge') RETURNING id::text`, x.org, x.asset).Scan(&x.ds); err != nil {
			t.Fatal(err)
		}
		return x
	}
	a, b := mk("a"), mk("b")
	defer func() {
		for _, x := range []tenant{a, b} {
			_, _ = pool.Exec(ctx, `DELETE FROM core.datastream WHERE id = $1::uuid`, x.ds)
			_, _ = pool.Exec(ctx, `DELETE FROM core.asset WHERE id = $1::uuid`, x.asset)
			_, _ = pool.Exec(ctx, `DELETE FROM core.site WHERE id = $1::uuid`, x.site)
			_, _ = pool.Exec(ctx, `DELETE FROM core.organization WHERE id = $1::uuid`, x.org)
		}
	}()
	s := &Server{pool: pool}
	// the other organization's id, site and attribute value select nothing
	for _, sel := range []dsSel{{IDs: []string{b.ds}}, {Sites: []string{b.site}}, {Attrs: map[string][]string{"owner": {"b"}}}, {Name: "Ovfridge b*"}, {Text: "Store b"}} {
		items, _, total, err := s.queryDS(ctx, a.org, []dsSel{sel}, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 || total != 0 {
			t.Errorf("organization a sees %d datastreams of b with %+v", len(items), sel)
		}
	}
	items, ids, total, err := s.queryDS(ctx, a.org, []dsSel{{Quantities: []string{"temperature"}, Attrs: map[string][]string{"storage_class": {"2-8C"}}}}, 500)
	if err != nil || total != 1 || len(ids) != 1 || ids[0] != a.ds || items[0]["site"] != "Store a "+suf {
		t.Fatalf("own datastream: %v %v %d %v", items, ids, total, err)
	}
	if err := s.enrichDS(ctx, a.org, items, ids, true, &dsWindow{From: time.Now().Add(-time.Hour), To: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f, err := s.facets(ctx, a.org, []string{"site", "asset_type", "attr:owner", "attr_keys"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := fmt.Sprint(f)
	if strings.Contains(all, "Store b") || strings.Contains(all, " b ") || !strings.Contains(all, "Store a "+suf) {
		t.Errorf("facets of a: %s", all)
	}
	for _, k := range searchKinds {
		if k.kind == "user" || k.kind == "camera" {
			continue
		}
		hits, _, err := s.searchKind(ctx, k, a.org, likeContains(suf), likeRepl.Replace(suf)+"%", 20)
		if err != nil {
			t.Fatal(k.kind, err)
		}
		for _, h := range hits {
			if strings.Contains(h.Title, " b ") || strings.Contains(h.Subtitle, " b ") || h.ID == b.ds || h.ID == b.asset || h.ID == b.site {
				t.Errorf("search of a finds %s %s of b", h.Kind, h.Title)
			}
		}
	}
}
