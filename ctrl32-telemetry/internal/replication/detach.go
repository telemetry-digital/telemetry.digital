package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
)

// DetachTimeout bounds how long a replica may take to catch up, promote and verify.
var DetachTimeout = 30 * time.Minute

// DetachCommand is what the replica receives (research R6).
type DetachCommand struct {
	Action   string           `json:"action"` // "detach"
	LSN      string           `json:"lsn"`    // wait until replayed
	Seq      int64            `json:"seq"`    // the replica.detach audit entry
	Hash     string           `json:"hash"`
	Snapshot string           `json:"snapshot"` // pg_snapshot of the counts
	Counts   map[string]int64 `json:"counts"`   // append-only record tables, as seen by Snapshot
	Partial  bool             `json:"partial"`
	Reason   string           `json:"reason"`
	At       time.Time        `json:"at"`
}

// DetachReport is the replica's answer.
type DetachReport struct {
	OK        bool             `json:"ok"`
	Promoted  bool             `json:"promoted"`
	NewNode   string           `json:"new_node"`
	ChainOK   bool             `json:"chain_ok"`
	HeadSeq   int64            `json:"head_seq"`
	HeadHash  string           `json:"head_hash"`
	EntryOK   bool             `json:"entry_ok"` // the replica.detach entry exists with the same hash
	Counts    map[string]int64 `json:"counts"`
	Errors    []string         `json:"errors"`
	ReplayLSN string           `json:"replay_lsn"`
}

var detachLocks sync.Map // replica id → *pgxpool.Conn holding the purge lock shared

// Actor identifies who asked for a change.
type Actor struct {
	OrgID, UserID, Name, Type, RequestID string
}

func (a Actor) entry(action, entityID string, old, new any, reason string) audit.Entry {
	t := a.Type
	if t == "" {
		t = "user"
	}
	return audit.Entry{OrgID: a.OrgID, ActorType: t, ActorID: a.UserID, ActorName: a.Name, Action: action, EntityType: "replica", EntityID: entityID,
		Old: old, New: new, Reason: reason, RequestID: a.RequestID}
}

// StartDetach begins a safe detach: counts of the record tables in one snapshot, the replica.detach audit entry,
// then the command for the replica. The primary keeps collecting data throughout.
func (p *Primary) StartDetach(ctx context.Context, id, reason string, acceptPartial bool, by Actor) (*Replica, error) {
	r, err := p.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	switch r.State {
	case "in_sync":
	case "lagging", "disconnected", "copying":
		if !acceptPartial {
			return nil, fmt.Errorf("%w: the replica is %s, not in sync — wait for \"in sync\" or accept a partial copy", ErrConflict, r.State)
		}
	default:
		return nil, fmt.Errorf("%w: a replica that is %s cannot be detached", ErrConflict, r.State)
	}
	partial := r.State != "in_sync"

	// The purge (the only deleter of record rows) waits while the counts are compared.
	lockConn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtext('ctrl32.purge'))`); err != nil {
		lockConn.Release()
		return nil, err
	}
	release := func() { p.releaseDetachLock(id) }
	detachLocks.Store(id, lockConn)

	snap, counts, err := p.snapshotCounts(ctx)
	if err != nil {
		release()
		return nil, err
	}
	cmd := DetachCommand{Action: "detach", Snapshot: snap, Counts: counts, Partial: partial, Reason: reason, At: time.Now().UTC()}
	seq, hash, err := audit.Append(ctx, p.Pool, by.entry("replica.detach", id, map[string]any{"state": r.State},
		map[string]any{"replica": r.Name, "mode": r.Mode, "snapshot": snap, "counts": counts, "partial": partial}, reason))
	if err != nil {
		release()
		return nil, err
	}
	cmd.Seq, cmd.Hash = seq, hash
	if err := p.Pool.QueryRow(ctx, `SELECT pg_current_wal_lsn()::text`).Scan(&cmd.LSN); err != nil {
		release()
		return nil, err
	}
	detach := map[string]any{"started_at": cmd.At, "started_by": by.Name, "reason": reason, "seq": seq, "hash": hash, "lsn": cmd.LSN,
		"snapshot": snap, "counts": counts, "partial": partial}
	if _, err := p.Pool.Exec(ctx, `UPDATE core.replica SET command = $2::jsonb, detach = $3::jsonb, updated_at = now() WHERE id = $1::uuid`,
		id, mustJSON(cmd), mustJSON(detach)); err != nil {
		release()
		return nil, err
	}
	p.setState(ctx, r, "detaching", map[string]any{"seq": seq, "lsn": cmd.LSN, "partial": partial})
	p.event(ctx, id, "command", map[string]any{"action": "detach", "seq": seq, "lsn": cmd.LSN, "partial": partial})
	return p.Get(ctx, id)
}

// snapshotCounts counts every append-only record table in one REPEATABLE READ snapshot and returns the snapshot.
func (p *Primary) snapshotCounts(ctx context.Context) (string, map[string]int64, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)
	var snap string
	if err := tx.QueryRow(ctx, `SELECT pg_current_snapshot()::text`).Scan(&snap); err != nil {
		return "", nil, err
	}
	tables, err := recordTables(ctx, tx)
	if err != nil {
		return "", nil, err
	}
	counts := map[string]int64{}
	for _, t := range tables {
		var n int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM core.`+pgx.Identifier{t}.Sanitize()).Scan(&n); err != nil {
			return "", nil, err
		}
		counts[t] = n
	}
	return snap, counts, tx.Commit(ctx)
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func recordTables(ctx context.Context, q queryer) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT relname FROM core.record_tables ORDER BY relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *Primary) releaseDetachLock(id string) {
	if v, ok := detachLocks.LoadAndDelete(id); ok {
		c := v.(*pgxpool.Conn)
		_, _ = c.Exec(context.Background(), `SELECT pg_advisory_unlock_shared(hashtext('ctrl32.purge'))`)
		c.Release()
	}
}

// FinishDetach takes the replica's report: compares, records the result in the audit trail, and removes the
// replica's access (it is an independent database now, whatever the verification says).
func (p *Primary) FinishDetach(ctx context.Context, r *Replica, rep DetachReport) (*Replica, error) {
	defer p.releaseDetachLock(r.ID)
	if r.State != "detaching" {
		return nil, fmt.Errorf("%w: the replica is %s", ErrConflict, r.State)
	}
	var cmd DetachCommand
	var detach map[string]any
	var cmdRaw, detRaw []byte
	if err := p.Pool.QueryRow(ctx, `SELECT command, detach FROM core.replica WHERE id = $1::uuid`, r.ID).Scan(&cmdRaw, &detRaw); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cmdRaw, &cmd)
	_ = json.Unmarshal(detRaw, &detach)
	if detach == nil {
		detach = map[string]any{}
	}
	diffs := compareCounts(cmd.Counts, rep.Counts, cmd.Partial)
	problems := append([]string{}, rep.Errors...)
	if !rep.Promoted {
		problems = append(problems, "the replica was not promoted")
	}
	if !rep.ChainOK {
		problems = append(problems, "the audit chain on the replica does not verify")
	}
	if !cmd.Partial && !rep.EntryOK {
		problems = append(problems, fmt.Sprintf("audit entry %d with the detach hash is missing on the replica", cmd.Seq))
	}
	problems = append(problems, diffs...)
	ok := len(problems) == 0
	detach["finished_at"] = time.Now().UTC()
	detach["verification"] = map[string]any{"ok": ok, "problems": problems, "replica": rep}
	state, action := "detached", "replica.detach_done"
	if !ok {
		state, action = "failed", "replica.detach_failed"
	}
	if _, err := p.Pool.Exec(ctx, `UPDATE core.replica SET command = NULL, detach = $2::jsonb, updated_at = now() WHERE id = $1::uuid`, r.ID, mustJSON(detach)); err != nil {
		return nil, err
	}
	if _, _, err := audit.Append(ctx, p.Pool, audit.Entry{ActorType: "system", ActorName: "ctrl32-telemetry", Action: action, EntityType: "replica", EntityID: r.ID,
		New: map[string]any{"replica": r.Name, "seq": cmd.Seq, "hash": cmd.Hash, "partial": cmd.Partial, "verified": ok, "problems": problems,
			"replica_head_seq": rep.HeadSeq, "replica_head_hash": rep.HeadHash, "new_node": rep.NewNode}}); err != nil {
		p.Log.Error("audit append failed", "security", true, "action", action, "err", err)
	}
	p.setState(ctx, r, state, map[string]any{"verified": ok, "problems": problems})
	p.event(ctx, r.ID, "detach", map[string]any{"verified": ok, "problems": problems, "seq": cmd.Seq})
	if adm, err := p.adminPool(ctx); err == nil {
		p.dropReplicationObjects(ctx, adm, r.RoleName, r.SlotName)
		if err := p.applyRetention(ctx, p.Pool, adm); err != nil {
			p.Log.Warn("could not set max_slot_wal_keep_size", "err", err)
		}
	}
	if p.Incident != nil {
		if ok {
			p.Incident.ReplicaRecovered(ctx, r.ID, state)
		} else {
			p.Incident.ReplicaProblem(ctx, r.ID, r.Name, "failed", "The detached copy did not pass verification; do not rely on it as a complete copy until the differences are understood.",
				map[string]any{"replica": r.ID, "problems": problems})
		}
	}
	return p.Get(ctx, r.ID)
}

func compareCounts(want, got map[string]int64, partial bool) []string {
	if partial {
		return nil
	}
	var out []string
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g, ok := got[k]
		if !ok {
			out = append(out, fmt.Sprintf("%s: not counted on the replica", k))
		} else if g != want[k] {
			out = append(out, fmt.Sprintf("%s: primary %d, replica %d", k, want[k], g))
		}
	}
	return out
}

// expireDetaches fails detaches the replica never answered.
func (p *Primary) expireDetaches(ctx context.Context) {
	list, err := p.list(ctx, `state = 'detaching' AND updated_at < now() - make_interval(secs => $1)`, DetachTimeout.Seconds())
	if err != nil {
		return
	}
	for i := range list {
		r := &list[i]
		rep := DetachReport{Errors: []string{fmt.Sprintf("the replica did not report within %s", DetachTimeout)}}
		if _, err := p.FinishDetach(ctx, r, rep); err != nil {
			p.Log.Warn("expire detach", "replica", r.ID, "err", err)
		}
	}
}
