package replication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// State is the replica's file next to the configuration (0640, readable by the service).
type State struct {
	Primary     string    `json:"primary"`
	ReplicaID   string    `json:"replica_id"`
	Name        string    `json:"name"`
	Mode        string    `json:"mode"`
	Token       string    `json:"token"`
	Role        string    `json:"role"`
	Password    string    `json:"password"`
	Slot        string    `json:"slot"`
	PrimaryNode string    `json:"primary_node"`
	TunnelPort  int       `json:"tunnel_port"`
	Host        Host      `json:"host"`
	AttachedAt  time.Time `json:"attached_at"`
	Aside       string    `json:"aside,omitempty"` // the data directory the base copy replaced
}

// StatePath is the replica state file for a configuration file path.
func StatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "replica.json")
}

// ReadState returns the replica state, or nil when this server is not a replica.
func ReadState(configPath string) (*State, error) {
	b, err := os.ReadFile(StatePath(configPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("replica state %s: %w", StatePath(configPath), err)
	}
	return &s, nil
}

// SaveState writes the file with the owner and group of the configuration file.
func SaveState(configPath string, s *State) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	p := StatePath(configPath)
	if err := os.WriteFile(p, b, 0o640); err != nil {
		return err
	}
	chownLike(p, configPath)
	return nil
}

// RetireState renames the state file once the replica is detached or promoted; the server then starts normally.
func RetireState(configPath, why string) error {
	p := StatePath(configPath)
	return os.Rename(p, strings.TrimSuffix(p, ".json")+"-"+why+"-"+time.Now().Format("20060102-150405")+".json")
}

// LocalURL is the replica server's read-only connection: the local cluster with the replication role.
func (s *State) LocalURL(localDBURL string) (string, error) {
	u, err := url.Parse(localDBURL)
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword(s.Role, s.Password)
	return u.String(), nil
}

// ErrRestart asks the process to exit so that the service manager starts it again in its new mode.
var ErrRestart = errors.New("restart required")

// ReplicaServer is the server in replica mode (research R7).
type ReplicaServer struct {
	State      *State
	ConfigPath string
	Pool       *pgxpool.Pool // local copy, read-only role
	AppURL     string        // local configuration, valid after promotion
	AdminURL   string
	Version    string
	Log        *slog.Logger
	// Promote runs Host.Promote as root (through the agent); nil = directly (tests, manual clusters).
	Promote func(ctx context.Context) (string, error)

	mu      sync.Mutex
	last    map[string]any
	contact time.Time
	client  *http.Client
}

func (r *ReplicaServer) http() *http.Client {
	if r.client == nil {
		r.client = &http.Client{Timeout: 40 * time.Second}
	}
	return r.client
}

// Run serves the tunnel for the local standby and follows the primary's commands until ctx ends; it returns
// ErrRestart after a detach.
func (r *ReplicaServer) Run(ctx context.Context) error {
	addr := net.JoinHostPort(r.State.Host.TunnelHost, strconv.Itoa(r.State.TunnelPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("replication tunnel listener %s: %w", addr, err)
	}
	tc := &TunnelClient{Primary: r.State.Primary, Token: r.State.Token, Log: r.Log, OnContact: func() { r.mu.Lock(); r.contact = time.Now(); r.mu.Unlock() }}
	go func() {
		if err := tc.Serve(ctx, ln); err != nil {
			r.Log.Error("replication tunnel stopped", "err", err)
		}
	}()
	r.Log.Info("replica mode", "primary", r.State.Primary, "replica", r.State.ReplicaID, "mode", r.State.Mode, "tunnel", addr)
	report := time.NewTicker(10 * time.Second)
	defer report.Stop()
	for {
		cmd, err := r.poll(ctx)
		if err != nil && ctx.Err() == nil {
			r.Log.Warn("replica: primary not reachable", "err", err)
		}
		if cmd != nil && cmd.Action == "detach" {
			rep := r.detach(ctx, cmd)
			if err := r.post(ctx, map[string]any{"result": rep}); err != nil {
				r.Log.Error("replica: could not report the detach", "err", err)
				_ = os.WriteFile(filepath.Join(filepath.Dir(r.ConfigPath), "replica-detach-report.json"), []byte(mustJSON(rep)), 0o640)
			}
			if rep.Promoted {
				_ = RetireState(r.ConfigPath, "detached")
				return ErrRestart
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-report.C:
			r.reportStatus(ctx)
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *ReplicaServer) req(ctx context.Context, method string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.State.Primary, "/")+"/api/v1/replication/agent", rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.State.Token)
	req.Header.Set("Content-Type", "application/json")
	return r.http().Do(req)
}

func (r *ReplicaServer) poll(ctx context.Context) (*DetachCommand, error) {
	resp, err := r.req(ctx, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("%d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	r.mu.Lock()
	r.contact = time.Now()
	r.mu.Unlock()
	var out struct {
		Command *DetachCommand `json:"command"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Command, nil
}

func (r *ReplicaServer) post(ctx context.Context, body any) error {
	resp, err := r.req(ctx, http.MethodPost, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Status is the replica's own view.
func (r *ReplicaServer) Status(ctx context.Context) map[string]any {
	st := map[string]any{"role": "replica", "mode": r.State.Mode, "name": r.State.Name, "primary": r.State.Primary, "replica_id": r.State.ReplicaID,
		"attached_at": r.State.AttachedAt, "version": r.Version}
	var replay, receive *string
	var lastXact *time.Time
	var inRecovery bool
	err := r.Pool.QueryRow(ctx, `SELECT pg_is_in_recovery(), pg_last_wal_replay_lsn()::text, pg_last_wal_receive_lsn()::text, pg_last_xact_replay_timestamp()`).
		Scan(&inRecovery, &replay, &receive, &lastXact)
	if err != nil {
		st["db_error"] = err.Error()
	} else {
		st["in_recovery"], st["replay_lsn"], st["receive_lsn"], st["last_replayed_xact"] = inRecovery, replay, receive, lastXact
	}
	var wr string
	if err := r.Pool.QueryRow(ctx, `SELECT coalesce((SELECT status FROM pg_stat_wal_receiver), 'stopped')`).Scan(&wr); err == nil {
		st["wal_receiver"] = wr
	}
	r.mu.Lock()
	if !r.contact.IsZero() {
		st["last_contact"] = r.contact
	}
	r.mu.Unlock()
	return st
}

func (r *ReplicaServer) reportStatus(ctx context.Context) {
	st := r.Status(ctx)
	r.mu.Lock()
	r.last = st
	r.mu.Unlock()
	_ = r.post(ctx, map[string]any{"status": st})
}

// detach executes the primary's detach command: catch up, promote, verify, record (research R6).
func (r *ReplicaServer) detach(ctx context.Context, cmd *DetachCommand) DetachReport {
	rep := DetachReport{Errors: []string{}}
	if !cmd.Partial {
		deadline := time.Now().Add(DetachTimeout - 2*time.Minute)
		for {
			var reached bool
			err := r.Pool.QueryRow(ctx, `SELECT coalesce(pg_last_wal_replay_lsn() >= $1::pg_lsn, false)`, cmd.LSN).Scan(&reached)
			if err == nil && reached {
				break
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				rep.Errors = append(rep.Errors, "the replica did not reach the detach position "+cmd.LSN)
				return rep
			}
			time.Sleep(time.Second)
		}
	}
	_ = r.Pool.QueryRow(ctx, `SELECT coalesce(pg_last_wal_replay_lsn()::text, '')`).Scan(&rep.ReplayLSN)
	promote := r.Promote
	if promote == nil {
		promote = func(ctx context.Context) (string, error) { return r.State.Host.Promote(ctx, r.AppURL, r.AdminURL) }
	}
	node, err := promote(ctx)
	if err != nil {
		rep.Errors = append(rep.Errors, "promotion failed: "+err.Error())
		return rep
	}
	rep.Promoted, rep.NewNode = true, node
	// From here on the copy is an independent, writable database with the local server's own credentials.
	pool, err := pgxpool.New(ctx, r.AppURL)
	if err != nil {
		rep.Errors = append(rep.Errors, "connect after promotion: "+err.Error())
		return rep
	}
	defer pool.Close()
	verify(ctx, pool, cmd, &rep)
	rep.OK = len(rep.Errors) == 0 && rep.ChainOK && (cmd.Partial || rep.EntryOK)
	if _, _, err := audit.Append(ctx, pool, audit.Entry{ActorType: "system", ActorName: "ctrl32-telemetry", Action: "replica.detached", EntityType: "replica", EntityID: r.State.ReplicaID,
		New: map[string]any{"primary": r.State.Primary, "primary_node": r.State.PrimaryNode, "new_node": node, "seq": cmd.Seq, "hash": cmd.Hash,
			"partial": cmd.Partial, "verified": rep.OK, "counts": rep.Counts, "errors": rep.Errors, "replay_lsn": rep.ReplayLSN}, Reason: cmd.Reason}); err != nil {
		rep.Errors = append(rep.Errors, "record the detachment on the copy: "+err.Error())
		rep.OK = false
	}
	return rep
}

// verify checks the audit chain, the detach entry and the record counts visible in the primary's snapshot.
func verify(ctx context.Context, pool *pgxpool.Pool, cmd *DetachCommand, rep *DetachReport) {
	res, err := audit.Verify(ctx, pool)
	if err != nil {
		rep.Errors = append(rep.Errors, "audit verification: "+err.Error())
	} else {
		rep.ChainOK = res.OK
		if res.HeadSeq != nil {
			rep.HeadSeq = *res.HeadSeq
		}
		if res.HeadHash != nil {
			rep.HeadHash = *res.HeadHash
		}
	}
	if !cmd.Partial {
		var h string
		if err := pool.QueryRow(ctx, `SELECT hash FROM core.audit_trail WHERE seq = $1`, cmd.Seq).Scan(&h); err == nil {
			rep.EntryOK = h == cmd.Hash
		}
	}
	rep.Counts = map[string]int64{}
	for t := range cmd.Counts {
		var n int64
		if err := pool.QueryRow(ctx, `SELECT core.count_in_snapshot(('core.' || quote_ident($1))::regclass, $2::pg_snapshot)`, t, cmd.Snapshot).Scan(&n); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("count %s: %v", t, err))
			continue
		}
		rep.Counts[t] = n
	}
}

// Handler serves the replica's own pages: its status (no data), the role for fencing checks and health.
func (r *ReplicaServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok (replica)\n")) })
	mux.HandleFunc("GET /api/v1/replication/role", func(w http.ResponseWriter, req *http.Request) {
		n, err := Node(req.Context(), r.Pool)
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		n.Role = "replica"
		writeJSON(w, n)
	})
	mux.HandleFunc("GET /api/v1/replication/status", func(w http.ResponseWriter, req *http.Request) { writeJSON(w, r.Status(req.Context())) })
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"this server is a read-only database replica; changes are made on the primary"}` + "\n"))
			return
		}
		if strings.HasPrefix(req.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"this server is a read-only database replica"}` + "\n"))
			return
		}
		st := r.Status(req.Context())
		var rows strings.Builder
		for _, k := range []string{"name", "mode", "primary", "replica_id", "attached_at", "wal_receiver", "replay_lsn", "last_replayed_xact", "last_contact", "version"} {
			if v, ok := st[k]; ok && v != nil {
				fmt.Fprintf(&rows, "<tr><th>%s</th><td>%s</td></tr>", html.EscapeString(strings.ReplaceAll(k, "_", " ")), html.EscapeString(fmtVal(v)))
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="10"><title>Database replica</title>
<style>body{font-family:system-ui,sans-serif;margin:40px;color:#1b1f23}table{border-collapse:collapse}th,td{padding:4px 12px;text-align:left;border-bottom:1px solid #e5e7eb}th{color:#6b7280;font-weight:500}</style></head>
<body><h1>ctrl32 telemetry — database replica</h1><p>This server keeps a read-only copy of the primary's database. Operate the system on the primary: <a href="%s">%s</a>.</p><table>%s</table>
<p style="color:#6b7280">Promote (hot standby): <code>ctrl32-telemetry replica promote -reason "…"</code> on this machine.</p></body></html>`,
			html.EscapeString(r.State.Primary), html.EscapeString(r.State.Primary), rows.String())
	})
	return mux
}

func fmtVal(v any) string {
	switch x := v.(type) {
	case *string:
		if x == nil {
			return "—"
		}
		return *x
	case *time.Time:
		if x == nil {
			return "—"
		}
		return x.Format(time.RFC3339)
	case time.Time:
		return x.Format(time.RFC3339)
	}
	return fmt.Sprint(v)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ConnectLocal opens the replica's read-only pool.
func ConnectLocal(ctx context.Context, s *State, localDBURL string) (*pgxpool.Pool, error) {
	u, err := s.LocalURL(localDBURL)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(u)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	return pgxpool.NewWithConfig(ctx, cfg)
}
