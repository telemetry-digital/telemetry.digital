package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// Modes of a replica (FR-002).
var Modes = []string{"copy", "continuous", "standby"}

// Errors the API maps to status codes.
var (
	ErrBadCode   = errors.New("pairing code wrong, expired or already used")
	ErrVersion   = errors.New("version mismatch")
	ErrInvalid   = errors.New("invalid")
	ErrNotFound  = errors.New("replica not found")
	ErrConflict  = errors.New("conflict")
	ErrMaxReplic = errors.New("at most 3 replicas can be attached")
)

// MaxReplicas follows the spec's assumption (one primary with up to 3 replicas).
const MaxReplicas = 3

// Replica is the API view of core.replica.
type Replica struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Mode          string          `json:"mode"`
	State         string          `json:"state"`
	LagS          *float64        `json:"lag_s"`
	LagBytes      *int64          `json:"lag_bytes"`
	ReplayLSN     string          `json:"replay_lsn,omitempty"`
	LastContact   *time.Time      `json:"last_contact"`
	Copy          *CopyProgress   `json:"copy,omitempty"`
	LagLimitS     int             `json:"lag_limit_s"`
	MaxRetainedMB int             `json:"max_retained_mb"`
	StandbyURL    string          `json:"standby_url,omitempty"`
	HostInfo      map[string]any  `json:"host_info"`
	Detach        json.RawMessage `json:"detach,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	CreatedBy     string          `json:"created_by,omitempty"`
	RoleName      string          `json:"-"`
	SlotName      string          `json:"-"`
}

// CopyProgress of the base copy.
type CopyProgress struct {
	TotalBytes int64    `json:"total_bytes"`
	DoneBytes  int64    `json:"done_bytes"`
	Percent    float64  `json:"percent"`
	ETAS       *float64 `json:"eta_s,omitempty"`
}

// Primary is the primary's side: pairing, tunnel target, monitoring, detach.
type Primary struct {
	Pool     *pgxpool.Pool // application role
	AdminURL string        // schema owner / superuser: roles, slots, pg_hba, replication views
	DBURL    string        // application URL: host and port of the local PostgreSQL for the tunnel
	Version  string        // ctrl32 release
	Log      *slog.Logger
	Incident IncidentSink

	adminOnce sync.Once
	admin     *pgxpool.Pool
	adminErr  error
	progress  sync.Map // replica id → copy sample for the ETA
}

// IncidentSink is implemented by the incident manager.
type IncidentSink interface {
	ReplicaProblem(ctx context.Context, replicaID, name, state, impact string, detail map[string]any)
	ReplicaRecovered(ctx context.Context, replicaID, state string)
}

func (p *Primary) adminPool(ctx context.Context) (*pgxpool.Pool, error) {
	p.adminOnce.Do(func() {
		cfg, err := pgxpool.ParseConfig(p.AdminURL)
		if err != nil {
			p.adminErr = err
			return
		}
		cfg.MaxConns = 3
		p.admin, p.adminErr = pgxpool.NewWithConfig(ctx, cfg)
	})
	return p.admin, p.adminErr
}

// Close releases the administrative pool.
func (p *Primary) Close() {
	if p.admin != nil {
		p.admin.Close()
	}
}

// DialDB connects to the primary's PostgreSQL (the tunnel's far end).
func (p *Primary) DialDB(ctx context.Context) (net.Conn, error) {
	addr, err := dbAddr(p.DBURL)
	if err != nil {
		return nil, err
	}
	return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", addr)
}

func dbAddr(dsn string) (string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", err
	}
	host := cfg.Host
	if host == "" || strings.HasPrefix(host, "/") {
		host = "127.0.0.1" // unix socket configurations: the server also listens on localhost
	}
	return net.JoinHostPort(host, strconv.Itoa(int(cfg.Port))), nil
}

// PGMajor returns the major version of a PostgreSQL server.
func PGMajor(ctx context.Context, q audit.Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT current_setting('server_version_num')::int / 10000`).Scan(&n)
	return n, err
}

// NodeInfo describes this database (core.node) and the server's role.
type NodeInfo struct {
	ID           string  `json:"node_id"`
	Role         string  `json:"role"` // primary | replica | fenced
	Epoch        int     `json:"epoch"`
	PromotedFrom *string `json:"promoted_from"`
}

// Node reads core.node.
func Node(ctx context.Context, q audit.Querier) (NodeInfo, error) {
	var n NodeInfo
	err := q.QueryRow(ctx, `SELECT id::text, epoch, promoted_from::text, CASE WHEN pg_is_in_recovery() THEN 'replica' ELSE 'primary' END FROM core.node`).
		Scan(&n.ID, &n.Epoch, &n.PromotedFrom, &n.Role)
	return n, err
}

// CreatePairing stores a one-time code for mode and name, valid 15 minutes.
func (p *Primary) CreatePairing(ctx context.Context, userID, mode, name string, lagLimit int) (string, time.Time, error) {
	if !has(Modes, mode) {
		return "", time.Time{}, fmt.Errorf("%w: mode must be copy, continuous or standby", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", time.Time{}, fmt.Errorf("%w: name must be 1–64 characters", ErrInvalid)
	}
	if lagLimit == 0 {
		lagLimit = 10
	}
	if lagLimit < 1 || lagLimit > 86400 {
		return "", time.Time{}, fmt.Errorf("%w: lag limit must be 1–86400 s", ErrInvalid)
	}
	var n int
	if err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM core.replica WHERE state NOT IN ('detached', 'dropped', 'failed')`).Scan(&n); err != nil {
		return "", time.Time{}, err
	}
	if n >= MaxReplicas {
		return "", time.Time{}, ErrMaxReplic
	}
	code, err := NewCode()
	if err != nil {
		return "", time.Time{}, err
	}
	norm, _ := NormalizeCode(code)
	var exp time.Time
	err = p.Pool.QueryRow(ctx, `INSERT INTO core.replica_pairing (code_hash, mode, name, lag_limit_s, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5::uuid, now() + interval '15 minutes') RETURNING expires_at`, Hash(norm), mode, name, lagLimit, userID).Scan(&exp)
	return code, exp, err
}

// PairRequest is what a replica sends with its code.
type PairRequest struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	PGMajor       int    `json:"pg_major"`
	Ctrl32Version string `json:"ctrl32_version"`
	StandbyURL    string `json:"standby_url"`
}

// PairResult is returned once to the replica (over TLS).
type PairResult struct {
	ReplicaID   string `json:"replica_id"`
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Token       string `json:"token"`
	Role        string `json:"role"`
	Password    string `json:"password"`
	Slot        string `json:"slot"`
	PrimaryNode string `json:"primary_node"`
	PGMajor     int    `json:"pg_major"`
}

// Pair consumes a code: checks versions, creates the replication role, the pg_hba rule and the slot, stores the
// replica and returns its credentials.
func (p *Primary) Pair(ctx context.Context, in PairRequest) (*PairResult, error) {
	norm, ok := NormalizeCode(in.Code)
	if !ok {
		return nil, ErrBadCode
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var pairID, mode, name, createdBy string
	var lagLimit int
	err = tx.QueryRow(ctx, `SELECT id::text, mode, name, lag_limit_s, created_by::text FROM core.replica_pairing
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`, Hash(norm)).Scan(&pairID, &mode, &name, &lagLimit, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBadCode
	} else if err != nil {
		return nil, err
	}
	// Versions are checked only for a valid code (an unauthenticated caller learns nothing).
	adm, err := p.adminPool(ctx)
	if err != nil {
		return nil, err
	}
	major, err := PGMajor(ctx, adm)
	if err != nil {
		return nil, err
	}
	if in.PGMajor != major {
		return nil, fmt.Errorf("%w: the primary runs PostgreSQL %d, this machine %d — install the same major version", ErrVersion, major, in.PGMajor)
	}
	if !sameRelease(in.Ctrl32Version, p.Version) {
		return nil, fmt.Errorf("%w: the primary runs ctrl32 telemetry %s, this machine %s — update both to the same release", ErrVersion, p.Version, in.Ctrl32Version)
	}
	if in.StandbyURL != "" {
		if u, err := url.Parse(in.StandbyURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("%w: standby URL must be http(s)://host", ErrInvalid)
		}
	}
	if strings.TrimSpace(in.Name) != "" && len(in.Name) <= 64 {
		name = strings.TrimSpace(in.Name)
	}
	if mode == "standby" && in.StandbyURL == "" {
		return nil, fmt.Errorf("%w: a hot standby needs its public URL (-standby-url) so that a returning primary can check who is primary", ErrInvalid)
	}
	short := shortID()
	res := &PairResult{Name: name, Mode: mode, Token: NewToken(), Role: "ctrl32_repl_" + short, Password: NewPassword(), Slot: "ctrl32_replica_" + short, PGMajor: major}
	host := map[string]any{"hostname": in.Hostname, "os": in.OS, "pg_major": in.PGMajor, "ctrl32_version": in.Ctrl32Version}
	hostJSON, _ := json.Marshal(host)
	var standby *string
	if in.StandbyURL != "" {
		u := strings.TrimRight(in.StandbyURL, "/")
		standby = &u
	}
	if err := tx.QueryRow(ctx, `INSERT INTO core.replica (name, mode, token_hash, role_name, slot_name, standby_url, host_info, lag_limit_s, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9::uuid) RETURNING id::text`,
		name, mode, Hash(res.Token), res.Role, res.Slot, standby, string(hostJSON), lagLimit, createdBy).Scan(&res.ReplicaID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.replica_pairing SET used_at = now(), replica_id = $2::uuid WHERE id = $1::uuid`, pairID, res.ReplicaID); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM core.node`).Scan(&res.PrimaryNode); err != nil {
		return nil, err
	}
	// Database objects first; if anything fails they are removed again and the code stays unused (rollback).
	if err := p.createReplicationObjects(ctx, adm, res.Role, res.Password, res.Slot); err != nil {
		p.dropReplicationObjects(context.WithoutCancel(ctx), adm, res.Role, res.Slot)
		return nil, err
	}
	if err := p.applyRetention(ctx, tx, adm); err != nil {
		p.Log.Warn("could not set max_slot_wal_keep_size", "err", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.replica_event (replica_id, kind, detail) VALUES ($1::uuid, 'state', $2::jsonb)`,
		res.ReplicaID, mustJSON(map[string]any{"state": "pairing", "host": host})); err != nil {
		return nil, err
	}
	if _, _, err := audit.Append(ctx, tx, audit.Entry{ActorType: "system", ActorName: "replica " + name, Action: "replica.pair", EntityType: "replica", EntityID: res.ReplicaID,
		New: map[string]any{"name": name, "mode": mode, "role": res.Role, "slot": res.Slot, "host": host, "standby_url": in.StandbyURL, "paired_by_code_of": createdBy}}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		p.dropReplicationObjects(context.WithoutCancel(ctx), adm, res.Role, res.Slot)
		return nil, err
	}
	p.Log.Info("replica paired", "security", true, "replica", res.ReplicaID, "name", name, "mode", mode, "host", in.Hostname)
	return res, nil
}

// sameRelease compares major.minor.patch; development builds pair with each other only.
func sameRelease(a, b string) bool {
	a, b = strings.TrimPrefix(strings.TrimSpace(a), "v"), strings.TrimPrefix(strings.TrimSpace(b), "v")
	return a == b && a != ""
}

func (p *Primary) createReplicationObjects(ctx context.Context, adm *pgxpool.Pool, role, password, slot string) error {
	if _, err := adm.Exec(ctx, fmt.Sprintf(`CREATE ROLE %s LOGIN REPLICATION PASSWORD %s CONNECTION LIMIT 4`, pgx.Identifier{role}.Sanitize(), quoteLiteral(password))); err != nil {
		return fmt.Errorf("create replication role: %w", err)
	}
	// Read-only rights for the replica's own server, which reads its local copy with this role (the copy carries
	// the primary's other roles and passwords, which never leave the primary).
	if _, err := adm.Exec(ctx, fmt.Sprintf(`GRANT pg_read_all_data, pg_read_all_stats TO %s`, pgx.Identifier{role}.Sanitize())); err != nil {
		return fmt.Errorf("grant read access to the replication role: %w", err)
	}
	if err := editHBA(ctx, adm, role, true); err != nil {
		return fmt.Errorf("allow replication in pg_hba.conf: %w", err)
	}
	if _, err := adm.Exec(ctx, `SELECT pg_create_physical_replication_slot($1, true)`, slot); err != nil {
		return fmt.Errorf("create replication slot: %w", err)
	}
	return nil
}

func (p *Primary) dropReplicationObjects(ctx context.Context, adm *pgxpool.Pool, role, slot string) {
	// A walsender still using the slot is ended first; the role cannot log in again once dropped.
	_, _ = adm.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = $1`, role)
	time.Sleep(300 * time.Millisecond)
	if _, err := adm.Exec(ctx, `SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = $1`, slot); err != nil {
		p.Log.Warn("drop replication slot", "slot", slot, "err", err)
	}
	if err := editHBA(ctx, adm, role, false); err != nil {
		p.Log.Warn("remove pg_hba rule", "role", role, "err", err)
	}
	if _, err := adm.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, pgx.Identifier{role}.Sanitize())); err != nil {
		p.Log.Warn("drop replication role", "role", role, "err", err)
	}
}

// applyRetention sets max_slot_wal_keep_size to the largest limit of the attached replicas (FR-009): a replica that
// falls further behind loses its slot instead of filling the primary's disk.
func (p *Primary) applyRetention(ctx context.Context, q audit.Querier, adm *pgxpool.Pool) error {
	var mb *int
	if err := q.QueryRow(ctx, `SELECT max(max_retained_mb) FROM core.replica WHERE state NOT IN ('detached', 'dropped', 'failed')`).Scan(&mb); err != nil {
		return err
	}
	val := "-1"
	if mb != nil {
		val = strconv.Itoa(*mb) + "MB"
	}
	if _, err := adm.Exec(ctx, `ALTER SYSTEM SET max_slot_wal_keep_size = `+quoteLiteral(val)); err != nil {
		return err
	}
	_, err := adm.Exec(ctx, `SELECT pg_reload_conf()`)
	return err
}

// editHBA adds or removes the line allowing the replication role (research R3). The file is rewritten by the
// database superuser itself (pg_read_file / COPY TO), so the same code works for docker and native clusters; a
// reload that reports errors restores the previous file.
func editHBA(ctx context.Context, adm *pgxpool.Pool, role string, add bool) error {
	conn, err := adm.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var file, content string
	if err := conn.QueryRow(ctx, `SELECT current_setting('hba_file')`).Scan(&file); err != nil {
		return err
	}
	if err := conn.QueryRow(ctx, `SELECT pg_read_file($1)`, file).Scan(&content); err != nil {
		return err
	}
	marker := "# ctrl32-replica " + role
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if !strings.Contains(l, marker) {
			lines = append(lines, l)
		}
	}
	if add {
		rules := []string{fmt.Sprintf("host replication %s all scram-sha-256 %s", role, marker), fmt.Sprintf("host all %s all scram-sha-256 %s", role, marker)}
		at := 0
		for at < len(lines) && (strings.HasPrefix(strings.TrimSpace(lines[at]), "#") || strings.TrimSpace(lines[at]) == "") {
			at++
		}
		lines = append(lines[:at], append(rules, lines[at:]...)...)
	}
	write := func(ls []string) error {
		if _, err := conn.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS ctrl32_hba (n int, line text)`); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, `TRUNCATE ctrl32_hba`); err != nil {
			return err
		}
		out := make([]string, len(ls))
		for i, l := range ls {
			if l == "" {
				l = "#" // CSV writes an empty string as ""
			}
			out[i] = l
		}
		if _, err := conn.Exec(ctx, `INSERT INTO ctrl32_hba SELECT n, line FROM unnest($1::text[]) WITH ORDINALITY u(line, n)`, out); err != nil {
			return err
		}
		_, err := conn.Exec(ctx, `COPY (SELECT line FROM ctrl32_hba ORDER BY n) TO `+quoteLiteral(file)+` WITH (FORMAT csv, DELIMITER E'\x01', QUOTE E'\x02')`)
		return err
	}
	if err := write(lines); err != nil {
		return err
	}
	var bad int
	if _, err := conn.Exec(ctx, `SELECT pg_reload_conf()`); err != nil {
		return err
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL`).Scan(&bad); err != nil || bad > 0 {
		orig := strings.Split(strings.TrimRight(content, "\n"), "\n")
		_ = write(orig)
		_, _ = conn.Exec(ctx, `SELECT pg_reload_conf()`)
		if err != nil {
			return err
		}
		return fmt.Errorf("pg_hba.conf would not load (%d errors); restored the previous file", bad)
	}
	return nil
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ReplicaByToken authenticates a replica; detached and dropped replicas are refused.
func (p *Primary) ReplicaByToken(ctx context.Context, token string) (*Replica, error) {
	if !strings.HasPrefix(token, "c32r_") {
		return nil, ErrNotFound
	}
	list, err := p.list(ctx, `token_hash = $1 AND state NOT IN ('detached', 'dropped', 'failed')`, Hash(token))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Touch records contact from the replica.
func (p *Primary) Touch(ctx context.Context, id string) {
	_, _ = p.Pool.Exec(ctx, `UPDATE core.replica SET last_contact = now() WHERE id = $1::uuid`, id)
}

const replicaCols = `id::text, name, mode, state, lag_s::float8, lag_bytes, coalesce(replay_lsn, ''), last_contact, copy_total_bytes, copy_done_bytes,
	lag_limit_s, max_retained_mb, coalesce(standby_url, ''), host_info, detach, created_at, coalesce(created_by::text, ''), role_name, slot_name`

func (p *Primary) list(ctx context.Context, where string, args ...any) ([]Replica, error) {
	rows, err := p.Pool.Query(ctx, `SELECT `+replicaCols+` FROM core.replica WHERE `+where+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Replica{}
	for rows.Next() {
		var r Replica
		var total, done *int64
		var detach []byte
		if err := rows.Scan(&r.ID, &r.Name, &r.Mode, &r.State, &r.LagS, &r.LagBytes, &r.ReplayLSN, &r.LastContact, &total, &done,
			&r.LagLimitS, &r.MaxRetainedMB, &r.StandbyURL, &r.HostInfo, &detach, &r.CreatedAt, &r.CreatedBy, &r.RoleName, &r.SlotName); err != nil {
			return nil, err
		}
		if len(detach) > 0 {
			r.Detach = detach
		}
		if total != nil && *total > 0 {
			d := int64(0)
			if done != nil {
				d = *done
			}
			r.Copy = &CopyProgress{TotalBytes: *total, DoneBytes: d, Percent: float64(d) * 100 / float64(*total)}
			if v, ok := p.progress.Load(r.ID); ok && r.State == "copying" {
				s := v.(copySample)
				if rate := float64(d-s.done0) / time.Since(s.at0).Seconds(); rate > 0 {
					eta := float64(*total-d) / rate
					r.Copy.ETAS = &eta
				}
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type copySample struct {
	at0   time.Time
	done0 int64
}

// List returns every replica that was ever attached (newest last).
func (p *Primary) List(ctx context.Context) ([]Replica, error) { return p.list(ctx, `true`) }

// Get returns one replica.
func (p *Primary) Get(ctx context.Context, id string) (*Replica, error) {
	list, err := p.list(ctx, `id = $1::uuid`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// Settings a replica's administrator may change (US4).
type Settings struct {
	Mode          *string `json:"mode"`
	LagLimitS     *int    `json:"lag_limit_s"`
	MaxRetainedMB *int    `json:"max_retained_mb"`
	StandbyURL    *string `json:"standby_url"`
}

// Update changes mode and limits; returns old and new values for the audit trail.
func (p *Primary) Update(ctx context.Context, id string, s Settings) (old, new map[string]any, err error) {
	r, err := p.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if r.State == "detached" || r.State == "dropped" || r.State == "failed" {
		return nil, nil, fmt.Errorf("%w: the replica is %s", ErrConflict, r.State)
	}
	old, new = map[string]any{}, map[string]any{}
	mode, lag, ret, url := r.Mode, r.LagLimitS, r.MaxRetainedMB, r.StandbyURL
	if s.Mode != nil && *s.Mode != mode {
		if !has(Modes, *s.Mode) {
			return nil, nil, fmt.Errorf("%w: mode must be copy, continuous or standby", ErrInvalid)
		}
		old["mode"], new["mode"], mode = mode, *s.Mode, *s.Mode
	}
	if s.LagLimitS != nil && *s.LagLimitS != lag {
		if *s.LagLimitS < 1 || *s.LagLimitS > 86400 {
			return nil, nil, fmt.Errorf("%w: lag limit must be 1–86400 s", ErrInvalid)
		}
		old["lag_limit_s"], new["lag_limit_s"], lag = lag, *s.LagLimitS, *s.LagLimitS
	}
	if s.MaxRetainedMB != nil && *s.MaxRetainedMB != ret {
		if *s.MaxRetainedMB < 64 || *s.MaxRetainedMB > 10485760 {
			return nil, nil, fmt.Errorf("%w: maximum retained WAL must be 64 MB – 10 TB", ErrInvalid)
		}
		old["max_retained_mb"], new["max_retained_mb"], ret = ret, *s.MaxRetainedMB, *s.MaxRetainedMB
	}
	if s.StandbyURL != nil && strings.TrimRight(*s.StandbyURL, "/") != url {
		v := strings.TrimRight(strings.TrimSpace(*s.StandbyURL), "/")
		if v != "" {
			if u, err := neturlParse(v); err != nil || u == "" {
				return nil, nil, fmt.Errorf("%w: standby URL must be http(s)://host", ErrInvalid)
			}
		}
		old["standby_url"], new["standby_url"], url = url, v, v
	}
	if mode == "standby" && url == "" {
		return nil, nil, fmt.Errorf("%w: a hot standby needs its public URL", ErrInvalid)
	}
	if len(new) == 0 {
		return nil, nil, fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	var urlArg *string
	if url != "" {
		urlArg = &url
	}
	if _, err := p.Pool.Exec(ctx, `UPDATE core.replica SET mode = $2, lag_limit_s = $3, max_retained_mb = $4, standby_url = $5, updated_at = now() WHERE id = $1::uuid`,
		id, mode, lag, ret, urlArg); err != nil {
		return nil, nil, err
	}
	p.event(ctx, id, "config", map[string]any{"old": old, "new": new})
	if _, ok := new["max_retained_mb"]; ok {
		if adm, err := p.adminPool(ctx); err == nil {
			if err := p.applyRetention(ctx, p.Pool, adm); err != nil {
				p.Log.Warn("could not set max_slot_wal_keep_size", "err", err)
			}
		}
	}
	return old, new, nil
}

func neturlParse(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("bad url")
	}
	return u.String(), nil
}

// Drop removes a replica's access (token, role, slot, pg_hba rule) without a detach; its copy stays behind as it
// is and is no longer fed.
func (p *Primary) Drop(ctx context.Context, id string) (*Replica, error) {
	r, err := p.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.State == "dropped" || r.State == "detached" {
		return nil, fmt.Errorf("%w: the replica is already %s", ErrConflict, r.State)
	}
	adm, err := p.adminPool(ctx)
	if err != nil {
		return nil, err
	}
	p.setState(ctx, r, "dropped", nil)
	p.dropReplicationObjects(ctx, adm, r.RoleName, r.SlotName)
	if err := p.applyRetention(ctx, p.Pool, adm); err != nil {
		p.Log.Warn("could not set max_slot_wal_keep_size", "err", err)
	}
	if p.Incident != nil {
		p.Incident.ReplicaRecovered(ctx, r.ID, "dropped")
	}
	return p.Get(ctx, id)
}

// Events returns a replica's history, newest first.
func (p *Primary) Events(ctx context.Context, id string, limit int) ([]map[string]any, error) {
	rows, err := p.Pool.Query(ctx, `SELECT ts, kind, detail FROM core.replica_event WHERE replica_id = $1::uuid ORDER BY ts DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ts time.Time
		var kind string
		var detail map[string]any
		if err := rows.Scan(&ts, &kind, &detail); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"ts": ts, "kind": kind, "detail": detail})
	}
	return out, rows.Err()
}

func (p *Primary) event(ctx context.Context, id, kind string, detail map[string]any) {
	if _, err := p.Pool.Exec(ctx, `INSERT INTO core.replica_event (replica_id, kind, detail) VALUES ($1::uuid, $2, $3::jsonb)`, id, kind, mustJSON(detail)); err != nil {
		p.Log.Warn("replica event", "replica", id, "err", err)
	}
}

// setState changes the state and records the change (no-op when unchanged).
func (p *Primary) setState(ctx context.Context, r *Replica, state string, detail map[string]any) {
	if r.State == state {
		return
	}
	if _, err := p.Pool.Exec(ctx, `UPDATE core.replica SET state = $2, updated_at = now() WHERE id = $1::uuid`, r.ID, state); err != nil {
		p.Log.Warn("replica state", "replica", r.ID, "err", err)
		return
	}
	d := map[string]any{"from": r.State, "state": state}
	for k, v := range detail {
		d[k] = v
	}
	p.event(ctx, r.ID, "state", d)
	p.Log.Info("replica state", "replica", r.ID, "name", r.Name, "from", r.State, "to", state)
	r.State = state
}

// Command returns the pending command for the replica (nil when none).
func (p *Primary) Command(ctx context.Context, id string) (map[string]any, error) {
	var cmd map[string]any
	err := p.Pool.QueryRow(ctx, `SELECT command FROM core.replica WHERE id = $1::uuid`, id).Scan(&cmd)
	return cmd, err
}
