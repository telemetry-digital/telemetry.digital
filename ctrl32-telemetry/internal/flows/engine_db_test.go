package flows

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A draft saved after deploying does not stop the flow after a restart: the deployed version is started. Needs a
// database with the schema (CTRL32_TEST_DB = a URL that may insert and delete core.flow rows, e.g. the superuser URL
// of a development or test database); skipped otherwise.
func TestDeployedVersionStartsDespiteNewerDraftDB(t *testing.T) {
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
	var org string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&org); err != nil {
		t.Skip("no organization in the database: ", err)
	}
	slug := fmt.Sprintf("restart-probe-%d", time.Now().UnixNano()%1000000)
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM core.flow_state WHERE org_id = $1::uuid AND flow_slug = $2`, org, slug)
		_, _ = pool.Exec(ctx, `DELETE FROM core.flow WHERE org_id = $1::uuid AND slug = $2`, org, slug)
	}()
	spec := func(label string) string {
		return `{"nodes":[{"id":"t","type":"inject","x":0,"y":0,"settings":{"value":"1"}},{"id":"d","type":"debug","name":"` + label + `","x":200,"y":0,"settings":{}}],"wires":[{"from":"t","port":0,"to":"d"}]}`
	}
	// version 1 deployed, then version 2 saved as a draft (it supersedes version 1 as the current version)
	if _, err := pool.Exec(ctx, `INSERT INTO core.flow (org_id, slug, name, version, state, spec, checksum, reason, valid_to)
		VALUES ($1::uuid, $2, 'Restart probe', 1, 'deployed', $3::jsonb, 'x', 'test', now())`, org, slug, spec("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO core.flow (org_id, slug, name, version, state, spec, checksum, reason)
		VALUES ($1::uuid, $2, 'Restart probe', 2, 'draft', $3::jsonb, 'y', 'test')`, org, slug, spec("v2")); err != nil {
		t.Fatal(err)
	}
	e := New(pool, &fakeHost{}, "0123456789abcdef-secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	list, err := e.deployedFlows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range list {
		found = found || (l[0] == org && l[1] == slug)
	}
	if !found {
		t.Fatalf("the deployed flow is not among the flows started with the server")
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.mu.Lock()
	e.ctx = rctx
	e.mu.Unlock()
	if err := e.Reload(rctx, org, slug); err != nil {
		t.Fatal(err)
	}
	st := e.Status(org, slug)
	if !st.Running || st.Version != 1 {
		t.Fatalf("running %v version %d, want the deployed version 1 running", st.Running, st.Version)
	}
	e.Stop(org, slug)
	// disabled: nothing runs
	if _, err := pool.Exec(ctx, `UPDATE core.flow SET state = 'disabled' WHERE org_id = $1::uuid AND slug = $2 AND state = 'deployed'`, org, slug); err != nil {
		t.Fatal(err)
	}
	if err := e.Reload(rctx, org, slug); err != nil {
		t.Fatal(err)
	}
	if e.Status(org, slug).Running {
		t.Fatal("a disabled flow runs")
	}
}
