package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/db"
)

// admin purge — the one authorised, logged way to remove records (URS B-11): demo data before an instance goes
// live, a site or device that was created by mistake, seeded dashboards, reports rendered from test data. The
// application's database role cannot delete records; this runs with db.admin_url on the server, shows what it
// would delete, and does it only with -yes, in one transaction, writing an audit trail entry per target.
//
//	ctrl32-telemetry admin purge [-config path] -reason "why" [-org slug] [-site NAME] [-device EXTERNAL_ID] [-dashboard SLUG] [-reports] [-yes]
func cmdPurge(args []string) error {
	fs := flag.NewFlagSet("admin purge", flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	orgSlug := fs.String("org", "", "organization slug (default: the first organization)")
	site := fs.String("site", "", "site name: the site, its assets and datastreams with every observation, gap, alarm and rule")
	device := fs.String("device", "", "device external id: the device with its credentials, messages, commands, console and FOTA history")
	dash := fs.String("dashboard", "", "dashboard slug: every version and its share links")
	reports := fs.Bool("reports", false, "every stored report of the organization (rows and PDF files)")
	reason := fs.String("reason", "", "reason recorded in the audit trail (required)")
	yes := fs.Bool("yes", false, "actually delete; without it only the counts are shown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*reason) == "" {
		return errors.New("-reason is required")
	}
	if *site == "" && *device == "" && *dash == "" && !*reports {
		return errors.New("nothing selected: use -site, -device, -dashboard and/or -reports")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DB.AdminURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	var orgID, orgName string
	if *orgSlug != "" {
		err = pool.QueryRow(ctx, `SELECT id::text, name FROM core.organization WHERE slug = $1`, *orgSlug).Scan(&orgID, &orgName)
	} else {
		err = pool.QueryRow(ctx, `SELECT id::text, name FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&orgID, &orgName)
	}
	if err != nil {
		return fmt.Errorf("organization: %w", err)
	}
	fmt.Printf("organization: %s\n", orgName)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// A safe detach of a database replica compares row counts with the primary (spec 003); it holds this lock shared,
	// so a purge waits until the detach has finished.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ctrl32.purge'))`); err != nil {
		return err
	}
	p := &purger{ctx: ctx, tx: tx, pool: pool, org: orgID}
	// Record tables carry append-only triggers (0001_core.sql) that refuse DELETE for everyone. The purge is the one
	// sanctioned exception: the user triggers of exactly these tables are disabled inside this transaction and
	// enabled again before the commit; the audit trail keeps its guard throughout.
	guarded, err := guardedRelations(ctx, pool)
	if err != nil {
		return err
	}
	for _, t := range guarded {
		if _, err := tx.Exec(ctx, `ALTER TABLE `+t+` DISABLE TRIGGER USER`); err != nil {
			return fmt.Errorf("disable guards on %s: %w", t, err)
		}
	}
	// DDL is transactional: a rollback (dry run or error) restores the guards by itself; the commit path re-enables
	// them explicitly below.
	var entries []audit.Entry
	var files []string
	if *site != "" {
		counts, siteID, err := p.site(*site)
		if err != nil {
			return err
		}
		printCounts("site "+*site, counts)
		entries = append(entries, audit.Entry{OrgID: orgID, EntityType: "site", EntityID: siteID, New: counts, Reason: *reason})
	}
	if *device != "" {
		counts, devID, err := p.device(*device)
		if err != nil {
			return err
		}
		printCounts("device "+*device, counts)
		entries = append(entries, audit.Entry{OrgID: orgID, EntityType: "device", EntityID: devID, New: counts, Reason: *reason})
	}
	if *dash != "" {
		counts, err := p.dashboard(*dash)
		if err != nil {
			return err
		}
		printCounts("dashboard "+*dash, counts)
		entries = append(entries, audit.Entry{OrgID: orgID, EntityType: "dashboard", EntityID: "", New: counts, Reason: *reason})
	}
	if *reports {
		counts, paths, err := p.reports()
		if err != nil {
			return err
		}
		files = paths
		printCounts("reports", counts)
		entries = append(entries, audit.Entry{OrgID: orgID, EntityType: "report", EntityID: "", New: counts, Reason: *reason})
	}
	if !*yes {
		fmt.Println("dry run — nothing deleted (add -yes)")
		return nil
	}
	for _, t := range guarded {
		if _, err := tx.Exec(ctx, `ALTER TABLE `+t+` ENABLE TRIGGER USER`); err != nil {
			return fmt.Errorf("enable guards on %s: %w", t, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	removed := 0
	for _, f := range files {
		full := filepath.Join(cfg.Report.StorageDir, filepath.FromSlash(f))
		if err := os.Remove(full); err == nil {
			removed++
		}
	}
	if len(files) > 0 {
		fmt.Printf("report files removed: %d of %d\n", removed, len(files))
	}
	for _, e := range entries {
		e.ActorType, e.ActorName, e.Action = "system", "cli admin purge", "data.purge"
		if _, _, err := audit.Append(ctx, pool, e); err != nil {
			fmt.Fprintln(os.Stderr, "audit entry failed:", err)
		}
	}
	fmt.Println("deleted; audit trail entries written")
	return nil
}

// guardedTables are the append-only record tables the purge deletes from; observation and raw_message are
// partitioned by month and every partition carries its own guard trigger, so the partitions are listed too.
var guardedTables = []string{"alarm_event", "observation", "raw_message", "console_log", "report", "assignment", "alarm_rule"}

func guardedRelations(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	var out []string
	for _, t := range guardedTables {
		out = append(out, "core."+t)
		rows, err := pool.Query(ctx, `SELECT inhrelid::regclass::text FROM pg_inherits WHERE inhparent = ('core.' || $1)::regclass`, t)
		if err != nil {
			return nil, fmt.Errorf("partitions of %s: %w", t, err)
		}
		for rows.Next() {
			var p string
			if rows.Scan(&p) == nil {
				out = append(out, p)
			}
		}
		rows.Close()
	}
	return out, nil
}

type purger struct {
	ctx  context.Context
	tx   pgx.Tx
	pool *pgxpool.Pool
	org  string
}

func (p *purger) del(counts map[string]any, name, sql string, args ...any) error {
	tag, err := p.tx.Exec(p.ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if tag.RowsAffected() > 0 {
		counts[name] = tag.RowsAffected()
	}
	return nil
}

// site removes the site with its assets, datastreams and every record derived from them.
func (p *purger) site(name string) (map[string]any, string, error) {
	var siteID string
	if err := p.tx.QueryRow(p.ctx, `SELECT id::text FROM core.site WHERE org_id = $1::uuid AND name = $2`, p.org, name).Scan(&siteID); err != nil {
		return nil, "", fmt.Errorf("site %q not found in this organization", name)
	}
	counts := map[string]any{"site": name}
	ds := `SELECT ds.id FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id WHERE a.site_id = $1::uuid`
	// Automation rules that name one of these datastreams keep working on nothing; say so (checked outside the
	// transaction so that a failed lookup cannot abort it).
	var rules int64
	if err := p.pool.QueryRow(p.ctx, `SELECT count(*) FROM core.rule r WHERE r.org_id = $2::uuid AND r.valid_to IS NULL AND EXISTS (SELECT 1 FROM (`+ds+`) d WHERE r.spec::text LIKE '%' || d.id::text || '%')`, siteID, p.org).Scan(&rules); err == nil && rules > 0 {
		counts["automation_rules_referencing_warning"] = rules
	}
	steps := []struct{ name, sql string }{
		{"notifications", `DELETE FROM core.notification WHERE alarm_id IN (SELECT id FROM core.alarm WHERE datastream_id IN (` + ds + `))`},
		{"alarm_events", `DELETE FROM core.alarm_event WHERE alarm_id IN (SELECT id FROM core.alarm WHERE datastream_id IN (` + ds + `))`},
		{"alarms", `DELETE FROM core.alarm WHERE datastream_id IN (` + ds + `)`},
		{"observations", `DELETE FROM core.observation WHERE datastream_id IN (` + ds + `)`},
		{"gaps", `DELETE FROM core.gap WHERE datastream_id IN (` + ds + `)`},
		{"alarm_rules", `DELETE FROM core.alarm_rule WHERE datastream_id IN (` + ds + `)`},
		{"assignments", `DELETE FROM core.assignment WHERE to_id IN (` + ds + `) OR from_id IN (` + ds + `)`},
		{"datastreams", `DELETE FROM core.datastream WHERE id IN (` + ds + `)`},
		{"assets", `DELETE FROM core.asset WHERE site_id = $1::uuid`},
		{"sites", `DELETE FROM core.site WHERE id = $1::uuid`},
	}
	for _, s := range steps {
		if err := p.del(counts, s.name, s.sql, siteID); err != nil {
			return nil, "", err
		}
	}
	return counts, siteID, nil
}

// device removes the device and everything that names it.
func (p *purger) device(ext string) (map[string]any, string, error) {
	var devID string
	if err := p.tx.QueryRow(p.ctx, `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2`, p.org, ext).Scan(&devID); err != nil {
		return nil, "", fmt.Errorf("device %q not found in this organization", ext)
	}
	counts := map[string]any{"device": ext}
	steps := []struct{ name, sql string }{
		{"notifications", `DELETE FROM core.notification WHERE alarm_id IN (SELECT id FROM core.alarm WHERE device_id = $1::uuid)`},
		{"alarm_events", `DELETE FROM core.alarm_event WHERE alarm_id IN (SELECT id FROM core.alarm WHERE device_id = $1::uuid)`},
		{"alarms", `DELETE FROM core.alarm WHERE device_id = $1::uuid`},
		{"console_lines", `DELETE FROM core.console_log WHERE device_id = $1::uuid`},
		{"console_sessions", `DELETE FROM core.console_session WHERE device_id = $1::uuid`},
		{"commands", `DELETE FROM core.device_command WHERE device_id = $1::uuid`},
		{"fota_targets", `DELETE FROM core.fota_target WHERE device_id = $1::uuid`},
		{"raw_messages", `DELETE FROM core.raw_message WHERE device_id = $1::uuid`},
		{"assignments", `DELETE FROM core.assignment WHERE from_id = $1::uuid OR to_id = $1::uuid`},
		{"alarm_rules", `DELETE FROM core.alarm_rule WHERE device_id = $1::uuid`},
		{"credentials", `DELETE FROM core.device_credential WHERE device_id = $1::uuid`},
		// Modbus / OPC UA devices are the device side of a connector whose id is the device's external id.
		{"connectors", `DELETE FROM core.connector WHERE org_id = $2::uuid AND id::text = (SELECT external_id FROM core.device WHERE id = $1::uuid)`},
		{"devices", `DELETE FROM core.device WHERE id = $1::uuid`},
	}
	for _, s := range steps {
		args := []any{devID}
		if strings.Contains(s.sql, "$2") {
			args = append(args, p.org)
		}
		if err := p.del(counts, s.name, s.sql, args...); err != nil {
			return nil, "", err
		}
	}
	return counts, devID, nil
}

// dashboard removes every version of a dashboard and its share links.
func (p *purger) dashboard(slug string) (map[string]any, error) {
	counts := map[string]any{"dashboard": slug}
	if err := p.del(counts, "shares", `DELETE FROM core.dashboard_share WHERE org_id = $1::uuid AND slug = $2`, p.org, slug); err != nil {
		return nil, err
	}
	if err := p.del(counts, "versions", `DELETE FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2`, p.org, slug); err != nil {
		return nil, err
	}
	if counts["versions"] == nil {
		return nil, fmt.Errorf("dashboard %q not found in this organization", slug)
	}
	return counts, nil
}

// reports removes the organization's stored reports; the files are deleted after the commit.
func (p *purger) reports() (map[string]any, []string, error) {
	rows, err := p.tx.Query(p.ctx, `SELECT file_path FROM core.report WHERE org_id = $1::uuid`, p.org)
	if err != nil {
		return nil, nil, err
	}
	var files []string
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil && f != "" {
			files = append(files, f)
		}
	}
	rows.Close()
	counts := map[string]any{}
	if err := p.del(counts, "reports", `DELETE FROM core.report WHERE org_id = $1::uuid`, p.org); err != nil {
		return nil, nil, err
	}
	counts["files"] = len(files)
	return counts, files, nil
}

func printCounts(title string, counts map[string]any) {
	fmt.Printf("%s:", title)
	for k, v := range counts {
		fmt.Printf(" %s=%v", k, v)
	}
	fmt.Println()
}
