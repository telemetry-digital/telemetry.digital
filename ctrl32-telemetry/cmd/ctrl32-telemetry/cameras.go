package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/db"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/httpapi"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

// cameras export|import — the camera inventory (spec 028) on the server's command line, for when the web interface is
// not available: after a crash, on a new server, or to script a migration. It runs the same checks as the web import
// and writes the same audit entries (actor "cli cameras … (OS user)"). A running server picks imported cameras up
// within a minute; a stopped one when it starts.
//
//	ctrl32-telemetry cameras export -org SLUG -file cameras.xlsx [-format xlsx|csv|pdf] [-passwords -reason "why"]
//	ctrl32-telemetry cameras import -org SLUG -file cameras.xlsx -reason "why" [-dry-run] [-no-update] [-no-create]
func cmdCameras(args []string) error {
	if len(args) < 1 || (args[0] != "export" && args[0] != "import") {
		return errors.New(`usage: ctrl32-telemetry cameras export -org SLUG -file cameras.xlsx [-format xlsx|csv|pdf] [-passwords -reason "why"]
       ctrl32-telemetry cameras import -org SLUG -file cameras.xlsx -reason "why" [-dry-run] [-no-update] [-no-create]`)
	}
	sub := args[0]
	fs := flag.NewFlagSet("cameras "+sub, flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	orgSlug := fs.String("org", "", "organization slug (default: the only organization)")
	file := fs.String("file", "", "the inventory file (.xlsx or .csv; - = standard input/output)")
	reason := fs.String("reason", "", "reason recorded in the audit trail")
	format := fs.String("format", "", "export: xlsx, csv or pdf (default: from the file name, else xlsx)")
	passwords := fs.Bool("passwords", false, "export: include camera passwords and relay secrets (needs -reason; audited)")
	dry := fs.Bool("dry-run", false, "import: only show what would change")
	noUpdate := fs.Bool("no-update", false, "import: leave existing cameras as they are")
	noCreate := fs.Bool("no-create", false, "import: do not add cameras")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("-file is required")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	var orgID, orgName string
	if *orgSlug != "" {
		err = pool.QueryRow(ctx, `SELECT id::text, name FROM core.organization WHERE slug = $1`, *orgSlug).Scan(&orgID, &orgName)
		if err != nil {
			return fmt.Errorf("organization %q not found", *orgSlug)
		}
	} else {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM core.organization`).Scan(&n)
		if n != 1 {
			return fmt.Errorf("the server has %d organizations: choose one with -org SLUG", n)
		}
		if err := pool.QueryRow(ctx, `SELECT id::text, name FROM core.organization`).Scan(&orgID, &orgName); err != nil {
			return err
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	vm := video.New(pool, cfg.Video.StorageDir, cfg.Video.MaxDiskGB, cfg.Security.SecretKey, logger) // only seals and opens passwords
	actor := "cli cameras " + sub
	if u, err := user.Current(); err == nil {
		actor += " (" + u.Username + ")"
	}
	switch sub {
	case "export":
		f := *format
		if f == "" {
			f = strings.TrimPrefix(strings.ToLower(filepath.Ext(*file)), ".")
			if f != "csv" && f != "pdf" {
				f = "xlsx"
			}
		}
		var rep *report.Engine
		if f == "pdf" {
			if rep, err = report.New(cfg.Report, pool, logger); err != nil {
				return err
			}
		}
		out, err := httpapi.NewInventory(cfg, pool, vm, rep, logger, version).Export(ctx, orgID, f, actor, *reason, *passwords)
		if err != nil {
			return err
		}
		if *file == "-" {
			_, err = os.Stdout.Write(out.Body)
			return err
		}
		mode := os.FileMode(0o644)
		if *passwords {
			mode = 0o600
		}
		if err := os.WriteFile(*file, out.Body, mode); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "%d cameras of %s written to %s", out.Cameras, orgName, *file)
		if *passwords {
			fmt.Fprint(os.Stderr, " — CONFIDENTIAL: the file contains camera passwords; keep it safe and delete copies you do not need")
		}
		fmt.Fprintln(os.Stderr)
		return nil
	default:
		var data []byte
		if *file == "-" {
			data, err = io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
		} else {
			data, err = os.ReadFile(*file)
		}
		if err != nil {
			return err
		}
		if !*dry && strings.TrimSpace(*reason) == "" {
			return errors.New("-reason is required (or -dry-run)")
		}
		opt := httpapi.InventoryOptions{Update: !*noUpdate, Create: !*noCreate}
		plan, err := httpapi.NewInventory(cfg, pool, vm, nil, logger, version).Import(ctx, orgID, data, opt, actor, *reason, !*dry)
		if plan != nil {
			for _, it := range plan.Rows {
				fmt.Println(it.Summary())
			}
			fmt.Printf("\n%s: %d new, %d changed, %d restored, %d unchanged, %d skipped, %d with errors", orgName, plan.Counts["create"], plan.Counts["update"], plan.Counts["restore"],
				plan.Counts["unchanged"], plan.Counts["skip"], plan.Counts["error"])
			if n := plan.Counts["disabled_by_licence"]; n > 0 {
				fmt.Printf("; %d imported disabled (without a licence up to %d enabled cameras)", n, plan.FreeLimit)
			}
			fmt.Println()
		}
		if err != nil {
			return err
		}
		switch {
		case *dry && plan.Errors() > 0:
			return fmt.Errorf("%d rows have errors", plan.Errors())
		case *dry:
			fmt.Println("dry run: nothing changed (run again without -dry-run and with -reason to apply)")
		default:
			fmt.Println("applied; a running server connects the cameras within a minute")
		}
		return nil
	}
}
