// ctrl32 telemetry — single-binary server.
//
//	ctrl32-telemetry serve   [-config path]           run the server (foreground or under the service manager)
//	ctrl32-telemetry migrate [-config path]           apply embedded schema migrations
//	ctrl32-telemetry audit verify [-config path]      re-walk the audit hash chain
//	ctrl32-telemetry service install|uninstall|start|stop|restart [-config path]
//	ctrl32-telemetry dev seed [-config path]          create demo organization, device and datastreams (prints the device token)
//	ctrl32-telemetry version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/flows"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/incident"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/timecheck"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/alarm"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/console"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/db"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/gap"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/httpapi"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/logging"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/push"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/replication"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/seed"
	svc "github.com/telemetry-digital/ctrl32-telemetry/internal/service"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/smarthome"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

// version is set at build time: -ldflags "-X main.version=1.2.3"
var version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "migrate":
		err = cmdMigrate(args)
	case "audit":
		err = cmdAudit(args)
	case "service":
		err = cmdService(args)
	case "dev":
		err = cmdDev(args)
	case "admin":
		err = cmdAdmin(args)
	case "agent":
		err = cmdAgent(args)
	case "replica":
		err = cmdReplica(args)
	case "version":
		fmt.Println("ctrl32-telemetry", version)
	default:
		usage()
		os.Exit(2)
	}
	if errors.Is(err, replication.ErrRestart) {
		fmt.Fprintln(os.Stderr, "restarting in the new replication role (the service manager starts the server again)")
		os.Exit(3)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ctrl32-telemetry <serve|agent|migrate|audit verify|service <action>|admin bootstrap|admin reset-password|admin seed-dashboard|dev seed|replica attach|status|promote|version> [-config path]")
}

func loadConfig(args []string) (*config.Config, string, error) {
	fs := flag.NewFlagSet("ctrl32-telemetry", flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	if env := os.Getenv("CTRL32_CONFIG"); env != "" && !isFlagSet(fs, "config") {
		*path = env
	}
	cfg, err := config.Load(*path)
	return cfg, *path, err
}

func isFlagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func cmdServe(args []string) error {
	cfg, path, err := loadConfig(args)
	if err != nil {
		return err
	}
	logger, closeLog, err := logging.Setup(cfg.Log.Level, cfg.Log.File, cfg.Log.Syslog)
	if err != nil {
		return err
	}
	defer closeLog()
	run := func(ctx context.Context) error { return runServer(ctx, cfg, path, logger) }

	if svc.Interactive() {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		logger.Info("starting in foreground", "config", path, "version", version, "profile", cfg.Profile)
		return run(ctx)
	}
	s, err := svc.New(svc.Config(path), run, logger)
	if err != nil {
		return err
	}
	return s.Run()
}

func runServer(ctx context.Context, cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	// Redundant database (spec 003): a fenced former primary and a replica run restricted; neither writes.
	if f := replication.ReadFence(cfgPath); f != nil {
		return runFenced(ctx, cfg, cfgPath, f, logger)
	}
	if st, err := replication.ReadState(cfgPath); err != nil {
		return err
	} else if st != nil {
		return runReplica(ctx, cfg, cfgPath, st, logger)
	}
	pool, err := db.Connect(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	inc := incident.New(pool, logger)
	repl := &replication.Primary{Pool: pool, AdminURL: cfg.DB.AdminURL, DBURL: cfg.DB.URL, Version: version, Log: logger.With("component", "replication"), Incident: inc}
	defer repl.Close()
	// A hot standby promoted while this server was down: stay out of the way (no two primaries).
	fctx, fcancel := context.WithTimeout(ctx, 20*time.Second)
	if f, err := repl.CheckFence(fctx, cfgPath); err != nil {
		logger.Warn("fencing check", "err", err)
	} else if f != nil {
		fcancel()
		pool.Close()
		return runFenced(ctx, cfg, cfgPath, f, logger)
	}
	fcancel()

	if res, err := audit.Verify(ctx, pool); err != nil {
		logger.Warn("audit chain verification failed at startup", "err", err)
	} else if !res.OK {
		logger.Error("AUDIT CHAIN BROKEN", "security", true, "first_bad_seq", res.FirstBadSeq)
		metrics.AuditChainOK.Set(0)
		inc.AuditChainBroken(ctx, seqOrZero(res.FirstBadSeq), res.Checked)
	} else {
		logger.Info("audit chain verified", "entries", res.Checked)
		metrics.AuditChainOK.Set(1)
	}
	if _, _, err := audit.Append(ctx, pool, audit.Entry{ActorType: "system", ActorName: "ctrl32-telemetry", Action: "service.start", EntityType: "service",
		New: map[string]any{"version": version, "profile": cfg.Profile}}); err != nil {
		logger.Warn("could not record service start in audit trail", "err", err)
	}

	br, err := broker.New(cfg, pool, logger)
	if err != nil {
		return err
	}
	iamSvc, err := newIAM(cfg, pool, logger)
	if err != nil {
		return err
	}
	rep, err := report.New(cfg.Report, pool, logger)
	if err != nil {
		logger.Warn("PDF reports disabled", "err", err)
		rep = nil
	}
	notifier := notify.New(cfg.SMTP, logger)
	pm := push.New(pool, logger, cfg.HTTP.BaseURL)
	notifier.Push = pm
	hub := live.NewHub()
	esc := escalate.New(pool, notifier, logger)
	inc.Email, inc.Push = notifier, pm
	esc.Incidents = inc
	esc.Start(ctx)
	engine := alarm.New(pool, logger, notifier)
	engine.Escalate = esc
	engine.Live = hub
	ing := ingest.New(pool, br, logger, engine, hub)
	br.OnDeviceState = func(d *broker.Device, connected bool) { hub.DeviceState(d.OrgID, d.ID, connected) }
	ing.OnCommand = func(e ingest.CommandEvent) {
		hub.Command(e.OrgID, e.DeviceID, e.CommandID, "", e.Status, map[string]any{"result": e.Result, "error": e.Error})
	}
	if err := ing.Start(); err != nil {
		return err
	}
	rulesEngine := rules.New(pool, notifier, esc, hub, logger)
	rulesEngine.Deriver = ing
	ing.AddSink(rulesEngine)
	rulesEngine.Start(ctx)
	flowEngine := flows.New(pool, nil, cfg.Security.SecretKey, logger)
	smart := smarthome.New(pool, br, ing, hub, cfg.Security.SecretKey, logger)
	smart.Inc = inc
	vids := video.New(pool, cfg.Video.StorageDir, cfg.Video.MaxDiskGB, cfg.Security.SecretKey, logger)
	ing.AddSink(smart) // values of datastreams exported to Home Assistant through bridges
	ing.AddSink(flowEngine)
	hub.Tap(flowEngine.OnEvent)
	detector := gap.New(pool, logger, cfg.Gap.CheckInterval)
	con := console.New(pool, br, logger)
	ing.SetConsole(con)
	opc := opcua.New(pool, ing, logger, filepath.Join(filepath.Dir(cfg.Report.StorageDir), "opcua"))
	opc.Start(ctx)
	mbm := modbus.New(pool, ing, logger)
	mbm.Start(ctx)

	tc := timecheck.New(timecheck.Config{Server: cfg.Time.NTPServer, Interval: cfg.Time.CheckInterval, MaxOffset: cfg.Time.MaxOffset}, logger)
	tc.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.HTTP.Listen,
		Handler:           httpapi.New(cfg, pool, br, iamSvc, rep, ing, con, agent.NewClient(cfg.Agent.Socket), opc, logger, version, hub, mbm, esc, rulesEngine, pm, tc, inc, repl, flowEngine, notifier, smart, vids),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return br.Serve(gctx) })
	g.Go(func() error { detector.Run(gctx); return nil })
	g.Go(func() error { con.Run(gctx); return nil })
	g.Go(func() error { repl.Run(gctx, 10*time.Second); return nil })
	g.Go(func() error { flowEngine.Start(gctx); return nil })
	g.Go(func() error {
		if err := vids.Run(gctx); err != nil {
			logger.Error("cameras not started", "err", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := smart.Start(gctx); err != nil {
			logger.Error("smart home not started", "err", err)
		}
		return nil
	})
	g.Go(func() error {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-gctx.Done():
				return nil
			case <-t.C:
				if f, err := repl.CheckFence(gctx, cfgPath); err == nil && f != nil {
					return replication.ErrRestart // restart in fenced mode
				}
			}
		}
	})
	g.Go(func() error {
		logger.Info("http listening", "addr", cfg.HTTP.Listen, "tls", cfg.HTTP.TLSCert != "")
		var err error
		if cfg.HTTP.TLSCert != "" {
			err = srv.ListenAndServeTLS(cfg.HTTP.TLSCert, cfg.HTTP.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})
	logger.Info("mqtt listening", "tcp", cfg.MQTT.Listen, "tls", cfg.MQTT.TLSListen, "ws", cfg.MQTT.WSListen)
	err = g.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	logger.Info("stopped")
	return err
}

// runReplica serves a replica (research R7): the tunnel for the local standby, the primary's commands, and a status
// page; no broker, ingest or background workers — the database is read-only.
func runReplica(ctx context.Context, cfg *config.Config, cfgPath string, st *replication.State, logger *slog.Logger) error {
	pool, err := replication.ConnectLocal(ctx, st, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	rs := &replication.ReplicaServer{State: st, ConfigPath: cfgPath, Pool: pool, AppURL: cfg.DB.URL, AdminURL: cfg.DB.AdminURL, Version: version,
		Log: logger.With("component", "replica")}
	if ag := agent.NewClient(cfg.Agent.Socket); ag.Configured() {
		rs.Promote = func(ctx context.Context) (string, error) { return ag.ReplicaPromote(ctx) }
	}
	return serveRestricted(ctx, cfg, rs.Handler(), logger, rs.Run)
}

// runFenced serves a former primary whose hot standby was promoted: read-only notice, no writes, no ingest.
func runFenced(ctx context.Context, cfg *config.Config, cfgPath string, f *replication.Fence, logger *slog.Logger) error {
	logger.Error("FENCED: this server is no longer the primary; it accepts no writes until it is re-attached as a replica", "security", true,
		"new_primary", f.ByNode, "standby", f.StandbyURL, "since", f.At)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/replication/role" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"node_id": f.OwnNode, "role": "fenced", "new_primary": f.ByNode, "standby_url": f.StandbyURL, "since": f.At})
			return
		}
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("fenced\n"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "This ctrl32 telemetry server was the primary until its hot standby %s was promoted (%s).\nIt accepts no data and no changes. Use the new primary, or re-attach this machine as its replica (see the replication runbook), then remove %s.\n",
			f.StandbyURL, f.At.Format(time.RFC3339), replication.FencePath(cfgPath))
	})
	return serveRestricted(ctx, cfg, h, logger, nil)
}

func serveRestricted(ctx context.Context, cfg *config.Config, h http.Handler, logger *slog.Logger, work func(context.Context) error) error {
	srv := &http.Server{Addr: cfg.HTTP.Listen, Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		if cfg.HTTP.TLSCert != "" {
			err = srv.ListenAndServeTLS(cfg.HTTP.TLSCert, cfg.HTTP.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	g.Go(func() error {
		<-gctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	})
	if work != nil {
		g.Go(func() error { return work(gctx) })
	}
	err := g.Wait()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	return err
}

func cmdMigrate(args []string) error {
	cfg, _, err := loadConfig(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	applied, err := db.Migrate(ctx, cfg.DB.AdminURL, cfg.DB.AppPassword)
	for _, a := range applied {
		fmt.Println("applied", a)
	}
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("schema up to date")
	}
	return nil
}

func cmdAudit(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("usage: ctrl32-telemetry audit verify [-config path]")
	}
	cfg, _, err := loadConfig(args[1:])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	res, err := audit.Verify(ctx, pool)
	if err != nil {
		return err
	}
	if !res.OK {
		fmt.Printf("AUDIT CHAIN BROKEN: first bad seq %v (checked %d)\n", deref(res.FirstBadSeq), res.Checked)
		os.Exit(3)
	}
	fmt.Printf("audit chain OK: %d entries, head seq %v, head hash %s\n", res.Checked, deref(res.HeadSeq), derefS(res.HeadHash))
	return nil
}

func cmdService(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ctrl32-telemetry service <install|uninstall|start|stop|restart>[-agent] [-config path]")
	}
	action := args[0]
	_, path, err := loadConfig(args[1:])
	if err != nil && strings.HasPrefix(action, "install") {
		return err
	}
	def := svc.Config(path)
	if strings.HasSuffix(action, "-agent") {
		def = svc.AgentConfig(path)
		action = strings.TrimSuffix(action, "-agent")
	}
	s, err := svc.New(def, func(ctx context.Context) error { return nil }, slog.Default())
	if err != nil {
		return err
	}
	if err := svc.Control(s, action); err != nil {
		return err
	}
	fmt.Println("service", action, "ok")
	return nil
}

func cmdDev(args []string) error {
	if len(args) == 0 || args[0] != "seed" {
		return errors.New("usage: ctrl32-telemetry dev seed [-config path]")
	}
	cfg, _, err := loadConfig(args[1:])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DB.AdminURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return seed.Demo(ctx, pool, os.Stdout)
}

func newIAM(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) (*iam.Service, error) {
	hasher, err := iam.NewHasher(cfg.Security.Pepper)
	if err != nil {
		return nil, err
	}
	box, err := iam.NewSecretBox(cfg.Security.SecretKey)
	if err != nil {
		return nil, err
	}
	return iam.New(pool, hasher, box, logger, cfg.Profile == "internet"), nil
}

// admin bootstrap: create the organization (if none) and the first org_admin; prints a one-time password.
// admin reset-password: set a new one-time password for a user.
// admin seed-dashboard: create a starter dashboard (overview, showcase, mimic or plant) for an organization without a browser session.
// admin purge: the authorised, logged deletion of a site, device, dashboard or the stored reports (purge.go).
func cmdAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ctrl32-telemetry admin <bootstrap|reset-password|seed-dashboard|purge> [flags] [-config path]")
	}
	sub := args[0]
	if sub == "purge" {
		return cmdPurge(args[1:])
	}
	fs := flag.NewFlagSet("admin "+sub, flag.ContinueOnError)
	path := fs.String("config", config.DefaultPath(), "path to config.toml")
	orgName := fs.String("org-name", "My organization", "organization name (bootstrap)")
	orgSlug := fs.String("org-slug", "main", "organization slug (bootstrap)")
	username := fs.String("username", "admin", "username")
	fullName := fs.String("name", "Administrator", "full name (bootstrap)")
	email := fs.String("email", "", "e-mail (bootstrap)")
	password := fs.String("password", "", "initial password (generated if empty; must be changed at first login)")
	permanent := fs.Bool("permanent", false, "reset-password: do not force a password change at the next login")
	allowWeak := fs.Bool("allow-weak", false, "reset-password: bypass the password policy (demo instances only; recorded in the audit trail)")
	dashKind := fs.String("kind", "showcase", "seed-dashboard: overview, showcase, mimic or plant")
	dashName := fs.String("dashboard-name", "", "seed-dashboard: dashboard name (default: the starter's name)")
	dashSlug := fs.String("slug", "", "seed-dashboard: slug (default: from the name)")
	dashDefault := fs.Bool("default", false, "seed-dashboard: show it on the home page")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if env := os.Getenv("CTRL32_CONFIG"); env != "" && !isFlagSet(fs, "config") {
		*path = env
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := db.Connect(ctx, cfg.DB.AdminURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc, err := newIAM(cfg, pool, slog.Default())
	if err != nil {
		return err
	}
	pw := *password
	if pw == "" {
		if pw, err = iam.RandomPassword(); err != nil {
			return err
		}
	}
	switch sub {
	case "seed-dashboard":
		var orgID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM core.organization WHERE slug = $1`, *orgSlug).Scan(&orgID); err != nil {
			slugs := []string{}
			if rows, err := pool.Query(ctx, `SELECT slug FROM core.organization ORDER BY created_at`); err == nil {
				for rows.Next() {
					var sl string
					if rows.Scan(&sl) == nil {
						slugs = append(slugs, sl)
					}
				}
				rows.Close()
			}
			return fmt.Errorf("organization %q not found (-org-slug); organizations: %v", *orgSlug, slugs)
		}
		name, raw, err := httpapi.StarterSpec(ctx, pool, orgID, *dashKind)
		if err != nil {
			return err
		}
		if *dashName != "" {
			name = *dashName
		}
		res, err := httpapi.CreateDashboard(ctx, pool, orgID, name, *dashSlug, raw, *dashDefault, "", "cli admin seed-dashboard "+*dashKind)
		if err != nil {
			return err
		}
		_, _, _ = audit.Append(ctx, pool, audit.Entry{OrgID: orgID, ActorType: "system", ActorName: "cli admin seed-dashboard", Action: "dashboard.create", EntityType: "dashboard", EntityID: res.ID,
			New: map[string]any{"slug": res.Slug, "name": res.Name, "version": res.Version, "widgets": res.Widgets, "is_default": *dashDefault, "kind": *dashKind}, Reason: "cli admin seed-dashboard"})
		fmt.Printf("dashboard created: %s (/dashboards/%s), %d widgets, default=%v\n", res.Name, res.Slug, res.Widgets, *dashDefault)
		return nil
	case "bootstrap":
		var orgID string
		err := pool.QueryRow(ctx, `SELECT id::text FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&orgID)
		if err != nil {
			if err := pool.QueryRow(ctx, `INSERT INTO core.organization (slug, name, profile) VALUES ($1, $2, $3) RETURNING id::text`, *orgSlug, *orgName, cfg.Profile).Scan(&orgID); err != nil {
				return fmt.Errorf("create organization: %w", err)
			}
			fmt.Println("organization created:", *orgName)
		}
		u, err := svc.CreateUser(ctx, orgID, *username, *fullName, *email, pw, []string{"org_admin"}, "", "bootstrap")
		if err != nil {
			return err
		}
		_, _, _ = audit.Append(ctx, pool, audit.Entry{OrgID: orgID, ActorType: "system", ActorName: "cli admin bootstrap", Action: "user.create", EntityType: "user", EntityID: u.ID,
			New: map[string]any{"username": u.Username, "roles": u.Roles}, Reason: "bootstrap"})
		fmt.Printf("admin user created: %s\none-time password (must be changed at first login): %s\n", u.Username, pw)
		return nil
	case "reset-password":
		var id string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM core.user_account WHERE username = $1`, *username).Scan(&id); err != nil {
			return fmt.Errorf("user %q not found", *username)
		}
		if err := svc.SetPassword(ctx, id, pw, !*permanent, !*allowWeak); err != nil {
			return err
		}
		_, _, _ = audit.Append(ctx, pool, audit.Entry{ActorType: "system", ActorName: "cli admin reset-password", Action: "user.password_reset", EntityType: "user", EntityID: id,
			New: map[string]any{"must_change": !*permanent, "policy_bypassed": *allowWeak}, Reason: "cli reset"})
		if *allowWeak {
			fmt.Fprintln(os.Stderr, "WARNING: password policy bypassed; this account is not suitable for production use")
		}
		if *permanent {
			fmt.Printf("password set for %s (no change required at login)\n", *username)
		} else {
			fmt.Printf("password reset for %s\none-time password (must be changed at first login): %s\n", *username, pw)
		}
		return nil
	default:
		return errors.New("usage: ctrl32-telemetry admin <bootstrap|reset-password>")
	}
}

func deref(p *int64) any {
	if p == nil {
		return "-"
	}
	return *p
}

func derefS(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

// cmdAgent runs the privileged helper (ADR-0012): root, Unix socket, closed list of host operations.
func cmdAgent(args []string) error {
	cfg, path, err := loadConfig(args)
	if err != nil {
		return err
	}
	logger, closeLog, err := logging.Setup(cfg.Log.Level, "", "")
	if err != nil {
		return err
	}
	defer closeLog()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sock := cfg.Agent.Socket
	if sock == "" {
		sock = "/run/ctrl32-telemetry/agent.sock"
		if runtime.GOOS == "windows" {
			pd := os.Getenv("ProgramData")
			if pd == "" {
				pd = `C:\ProgramData`
			}
			sock = filepath.Join(pd, "ctrl32-telemetry", "agent.sock")
		}
	}
	srv := agent.NewServer(sock, path, version, cfg.Agent.UpdateURL, logger.With("component", "agent"))
	srv.SetDBTools(cfg.Agent.PgDump, cfg.Agent.PgContainer)
	if svc.Interactive() {
		return srv.Serve(ctx)
	}
	// under the Windows Service Control Manager the process must answer the SCM: run through the service wrapper
	s, err := svc.New(svc.AgentConfig(path), func(ctx context.Context) error { return srv.Serve(ctx) }, logger)
	if err != nil {
		return err
	}
	return s.Run()
}

func seqOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
