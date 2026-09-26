package main

// ctrl32-telemetry replica attach|status|promote — the second PC's side of the redundant database (spec 003). Run as
// root (Administrator): it stops and starts the local PostgreSQL and replaces its data directory with a copy of the
// primary (the old one is moved aside, never deleted).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/replication"
)

func cmdReplica(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ctrl32-telemetry replica <attach|status|promote> [flags]")
	}
	switch args[0] {
	case "attach":
		return replicaAttach(args[1:])
	case "status":
		return replicaStatus(args[1:])
	case "promote":
		return replicaPromote(args[1:])
	}
	return fmt.Errorf("unknown replica command %q (attach, status, promote)", args[0])
}

type hostFlags struct {
	container, pgdata, pgbin, pguser string
	pgport                          int
	noService                       bool
}

func (h *hostFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&h.container, "container", "ctrl32-telemetry-postgres", "Docker container of the local PostgreSQL (install.sh)")
	fs.StringVar(&h.pgdata, "pgdata", "", "data directory of a cluster managed by hand (with -pgbin, -pgport, -pguser)")
	fs.StringVar(&h.pgbin, "pgbin", "", "directory of pg_basebackup, psql and pg_ctl (with -pgdata)")
	fs.IntVar(&h.pgport, "pgport", 5432, "port of the cluster managed by hand")
	fs.StringVar(&h.pguser, "pguser", "", "superuser of the cluster managed by hand (default: the admin_url user)")
	fs.BoolVar(&h.noService, "no-service", false, "do not stop/start the ctrl32-telemetry service (tests, manual setups)")
}

func replicaAttach(args []string) error {
	fs := flag.NewFlagSet("replica attach", flag.ContinueOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "path to config.toml")
	primary := fs.String("primary", "", "base URL of the primary, e.g. https://telemetry.digital")
	code := fs.String("code", "", "pairing code shown on the primary's Server page")
	name := fs.String("name", "", "name of this replica (default: from the pairing)")
	standbyURL := fs.String("standby-url", "", "hot standby: this machine's public URL (for fencing after a failover)")
	tunnelPort := fs.Int("tunnel-port", 54329, "local port of the replication tunnel")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	var hf hostFlags
	hf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *primary == "" || *code == "" {
		return errors.New("-primary and -code are required")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if st, _ := replication.ReadState(*cfgPath); st != nil {
		return fmt.Errorf("this machine is already a replica of %s (%s)", st.Primary, replication.StatePath(*cfgPath))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	host, err := detectHost(ctx, cfg, hf)
	if err != nil {
		return err
	}
	major, err := host.PGMajor(ctx)
	if err != nil {
		return fmt.Errorf("local PostgreSQL: %w", err)
	}
	fmt.Printf("local PostgreSQL %d (%s), data directory %s\n", major, host.Mode, host.DataDir)
	if !*yes {
		fmt.Printf("This replaces the local database with a copy of %s (the current data directory is kept as %s.before-replica-…).\nType yes to continue: ", *primary, host.DataDir)
		var answer string
		_, _ = fmt.Scanln(&answer)
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			return errors.New("cancelled")
		}
	}
	hostname, _ := os.Hostname()
	res, err := pairWithPrimary(ctx, *primary, replication.PairRequest{Code: *code, Name: *name, Hostname: hostname, OS: runtime.GOOS + "/" + runtime.GOARCH,
		PGMajor: major, Ctrl32Version: version, StandbyURL: *standbyURL})
	if err != nil {
		return err
	}
	fmt.Printf("paired as %q (%s), replica %s\n", res.Name, res.Mode, res.ReplicaID)
	st := &replication.State{Primary: strings.TrimRight(*primary, "/"), ReplicaID: res.ReplicaID, Name: res.Name, Mode: res.Mode, Token: res.Token, Role: res.Role,
		Password: res.Password, Slot: res.Slot, PrimaryNode: res.PrimaryNode, TunnelPort: *tunnelPort, Host: *host, AttachedAt: time.Now().UTC()}

	// The tunnel carries the base copy now and the streaming later (then served by the ctrl32 service).
	ln, err := net.Listen("tcp", net.JoinHostPort(host.TunnelHost, strconv.Itoa(*tunnelPort)))
	if err != nil {
		return fmt.Errorf("tunnel port: %w", err)
	}
	tctx, tcancel := context.WithCancel(ctx)
	go func() { _ = (&replication.TunnelClient{Primary: st.Primary, Token: st.Token}).Serve(tctx, ln) }()
	defer tcancel()

	if !hf.noService {
		serviceCtl(ctx, "stop")
	}
	fmt.Println("stopping the local PostgreSQL …")
	if err := host.Stop(ctx); err != nil {
		return err
	}
	fmt.Println("copying the primary's database (the primary keeps running) …")
	aside, err := host.BaseBackup(ctx, *tunnelPort, res.Role, res.Password, res.Slot, os.Stdout)
	st.Aside = aside
	if err != nil {
		fmt.Fprintln(os.Stderr, "the copy failed; the previous data directory is", aside)
		return err
	}
	if err := replication.SaveState(*cfgPath, st); err != nil {
		return err
	}
	tcancel()
	ln.Close()
	fmt.Println("starting the local PostgreSQL as a standby …")
	if err := host.Start(ctx); err != nil {
		return err
	}
	if !hf.noService {
		serviceCtl(ctx, "start")
		fmt.Println("the ctrl32-telemetry service runs in replica mode now; follow the state on the primary's Server page")
	} else {
		fmt.Println("start the server (ctrl32-telemetry serve) to feed the standby; it runs in replica mode")
	}
	fmt.Println("previous data directory kept as", aside)
	return nil
}

func detectHost(ctx context.Context, cfg *config.Config, hf hostFlags) (*replication.Host, error) {
	pc, err := pgx.ParseConfig(cfg.DB.AdminURL)
	if err != nil {
		return nil, err
	}
	user := hf.pguser
	if user == "" {
		user = pc.User
	}
	return replication.DetectHost(ctx, hf.container, pc.Database, user, hf.pgdata, hf.pgbin, hf.pgport)
}

func pairWithPrimary(ctx context.Context, primary string, in replication.PairRequest) (*replication.PairResult, error) {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(primary, "/")+"/api/v1/replication/pair", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("primary not reachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusCreated {
		var e struct{ Error string }
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(body))
		}
		return nil, fmt.Errorf("pairing refused (%d): %s", resp.StatusCode, e.Error)
	}
	var res replication.PairResult
	return &res, json.Unmarshal(body, &res)
}

func serviceCtl(ctx context.Context, action string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "sc", action, "ctrl32-telemetry")
	} else if _, err := exec.LookPath("systemctl"); err == nil {
		cmd = exec.CommandContext(ctx, "systemctl", action, "ctrl32-telemetry")
	} else {
		return
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "note: %s ctrl32-telemetry: %v %s\n", action, err, strings.TrimSpace(string(out)))
	}
}

func replicaStatus(args []string) error {
	fs := flag.NewFlagSet("replica status", flag.ContinueOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	st, err := replication.ReadState(*cfgPath)
	if err != nil {
		return err
	}
	if st == nil {
		fmt.Println("this server is not a replica")
		return nil
	}
	ctx := context.Background()
	pool, err := replication.ConnectLocal(ctx, st, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	rs := &replication.ReplicaServer{State: st, Pool: pool, Version: version}
	b, _ := json.MarshalIndent(rs.Status(ctx), "", "  ")
	fmt.Println(string(b))
	return nil
}

// replicaPromote makes a hot standby the primary after the primary failed (manual, with a reason — clarified).
func replicaPromote(args []string) error {
	fs := flag.NewFlagSet("replica promote", flag.ContinueOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "path to config.toml")
	reason := fs.String("reason", "", "why (audit trail)")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	var hf hostFlags
	hf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(strings.TrimSpace(*reason)) < 3 {
		return errors.New("-reason is required")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	st, err := replication.ReadState(*cfgPath)
	if err != nil {
		return err
	}
	if st == nil {
		return errors.New("this server is not a replica")
	}
	if !*yes {
		fmt.Printf("Promote this replica of %s to primary? The former primary will refuse writes when it returns.\nType yes to continue: ", st.Primary)
		var answer string
		_, _ = fmt.Scanln(&answer)
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			return errors.New("cancelled")
		}
	}
	ctx := context.Background()
	if n, err := replication.RoleOf(ctx, st.Primary); err == nil && n.Role == "primary" {
		fmt.Fprintf(os.Stderr, "warning: the primary %s still answers as primary; make sure it is really down or isolated (two primaries must be avoided)\n", st.Primary)
	}
	node, err := st.Host.Promote(ctx, cfg.DB.URL, cfg.DB.AdminURL)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, _, err := audit.Append(ctx, pool, audit.Entry{ActorType: "system", ActorName: "ctrl32-telemetry replica promote", Action: "replica.promoted", EntityType: "replica", EntityID: st.ReplicaID,
		New: map[string]any{"former_primary": st.Primary, "former_primary_node": st.PrimaryNode, "new_node": node, "mode": st.Mode}, Reason: *reason}); err != nil {
		return fmt.Errorf("record the promotion: %w", err)
	}
	if err := replication.RetireState(*cfgPath, "promoted"); err != nil {
		return err
	}
	if !hf.noService {
		serviceCtl(ctx, "restart")
	}
	fmt.Printf("promoted: this server is the primary now (node %s). Point devices and users at it (shared name or address).\n", node)
	return nil
}
