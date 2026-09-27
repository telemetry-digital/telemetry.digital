package replication

import (
	"context"
	"fmt"
	"time"
)

// Run monitors the attached replicas every interval until ctx ends (research R9).
func (p *Primary) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := p.MonitorOnce(ctx); err != nil && ctx.Err() == nil {
			p.Log.Warn("replica monitor", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type observed struct {
	slotExists  bool
	walStatus   string
	streaming   bool
	replayLSN   string
	lagS        *float64
	lagBytes    *int64
	copyTotal   *int64
	copyDone    *int64
	copyRunning bool
}

// MonitorOnce reads PostgreSQL's replication views (as the administrative role, which may see other roles'
// walsenders) and updates each replica's state, lag and copy progress.
func (p *Primary) MonitorOnce(ctx context.Context) error {
	p.expireDetaches(ctx)
	list, err := p.list(ctx, `state NOT IN ('detached', 'dropped', 'failed')`)
	if err != nil || len(list) == 0 {
		return err
	}
	adm, err := p.adminPool(ctx)
	if err != nil {
		return err
	}
	for i := range list {
		r := &list[i]
		var o observed
		err := adm.QueryRow(ctx, `
			SELECT s.slot_name IS NOT NULL, coalesce(s.wal_status, ''),
			       coalesce(sr.state = 'streaming', false), coalesce(sr.replay_lsn::text, ''),
			       CASE WHEN sr.pid IS NULL THEN NULL ELSE coalesce(extract(epoch FROM sr.replay_lag)::float8, 0) END,
			       CASE WHEN sr.pid IS NULL THEN NULL ELSE pg_wal_lsn_diff(pg_current_wal_lsn(), coalesce(sr.replay_lsn, sr.flush_lsn, sr.sent_lsn))::bigint END,
			       pb.backup_total, pb.backup_streamed, pb.pid IS NOT NULL
			FROM (SELECT $1::text AS slot, $2::text AS role) q
			LEFT JOIN pg_replication_slots s ON s.slot_name = q.slot
			LEFT JOIN pg_stat_replication sr ON sr.usename = q.role AND sr.state IN ('streaming', 'catchup')
			LEFT JOIN LATERAL (SELECT b.backup_total, b.backup_streamed, b.pid FROM pg_stat_progress_basebackup b
			                   JOIN pg_stat_activity a ON a.pid = b.pid WHERE a.usename = q.role LIMIT 1) pb ON true
			LIMIT 1`, r.SlotName, r.RoleName).
			Scan(&o.slotExists, &o.walStatus, &o.streaming, &o.replayLSN, &o.lagS, &o.lagBytes, &o.copyTotal, &o.copyDone, &o.copyRunning)
		if err != nil {
			p.Log.Warn("replica monitor query", "replica", r.ID, "err", err)
			continue
		}
		p.apply(ctx, r, o)
	}
	return nil
}

func (p *Primary) apply(ctx context.Context, r *Replica, o observed) {
	state := r.State
	switch {
	case r.State == "detaching":
		// the detach owns the state
	case !o.slotExists || o.walStatus == "lost":
		state = "lost"
	case o.copyRunning:
		state = "copying"
	case o.streaming:
		lagging := false
		if o.lagS != nil && *o.lagS > float64(r.LagLimitS) {
			lagging = true
		}
		if lagging {
			state = "lagging"
		} else {
			state = "in_sync"
		}
	case r.State == "pairing" || r.State == "copying":
		// not streaming yet: the base copy is being set up or finished and the standby is starting
		if r.State == "copying" && r.LastContact != nil && time.Since(*r.LastContact) > time.Duration(r.LagLimitS)*time.Second+2*time.Minute {
			state = "disconnected"
		}
	default:
		state = "disconnected"
	}
	if o.copyRunning && o.copyTotal != nil {
		if _, ok := p.progress.Load(r.ID); !ok {
			d := int64(0)
			if o.copyDone != nil {
				d = *o.copyDone
			}
			p.progress.Store(r.ID, copySample{at0: time.Now(), done0: d})
		}
	} else {
		p.progress.Delete(r.ID)
	}
	if _, err := p.Pool.Exec(ctx, `UPDATE core.replica SET lag_s = $2, lag_bytes = $3, replay_lsn = NULLIF($4, ''),
		copy_total_bytes = coalesce($5, copy_total_bytes), copy_done_bytes = CASE WHEN $6::bigint IS NOT NULL THEN $6 WHEN $7 THEN copy_done_bytes ELSE copy_total_bytes END,
		last_contact = CASE WHEN $8 THEN now() ELSE last_contact END WHERE id = $1::uuid`,
		r.ID, o.lagS, o.lagBytes, o.replayLSN, o.copyTotal, o.copyDone, o.copyRunning, o.streaming || o.copyRunning); err != nil {
		p.Log.Warn("replica monitor update", "replica", r.ID, "err", err)
	}
	prev := r.State
	p.setState(ctx, r, state, map[string]any{"lag_s": o.lagS, "lag_bytes": o.lagBytes, "wal_status": o.walStatus})
	if p.Incident == nil {
		return
	}
	switch state {
	case "lost":
		p.Incident.ReplicaProblem(ctx, r.ID, r.Name, state, "The primary no longer keeps the changes this replica needs (maximum retained WAL exceeded or slot removed). The replica must be attached again; the primary kept collecting.",
			map[string]any{"replica": r.ID, "mode": r.Mode, "wal_status": o.walStatus})
	case "disconnected", "lagging":
		// disconnected counts from the last contact: short interruptions (restarts, a network blip) are no incident
		if state == "disconnected" && r.LastContact != nil && time.Since(*r.LastContact) < time.Duration(r.LagLimitS)*time.Second {
			return
		}
		impact := fmt.Sprintf("The replica is not up to date (limit %d s). If the primary fails now, the changes since the replica's last position are not on the replica.", r.LagLimitS)
		p.Incident.ReplicaProblem(ctx, r.ID, r.Name, state, impact, map[string]any{"replica": r.ID, "mode": r.Mode, "lag_s": o.lagS, "lag_bytes": o.lagBytes})
	case "in_sync":
		if prev != state {
			p.Incident.ReplicaRecovered(ctx, r.ID, state)
		}
	}
}
