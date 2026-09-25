// Package gap detects datastreams that stopped delivering data (URS B-05): a period without observations
// longer than 1.5 × expected_interval opens a gap record; ingest closes it when data resumes or is backfilled.
package gap

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Detector runs the periodic check.
type Detector struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	interval time.Duration
}

// New creates a detector that checks every `interval`.
func New(pool *pgxpool.Pool, log *slog.Logger, interval time.Duration) *Detector {
	return &Detector{pool: pool, log: log.With("component", "gap"), interval: interval}
}

// Run blocks until ctx is done.
func (d *Detector) Run(ctx context.Context) {
	t := time.NewTicker(d.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := d.Check(ctx); err != nil {
				d.log.Error("gap check failed", "err", err)
			} else if n > 0 {
				d.log.Warn("gaps opened", "count", n)
			}
		}
	}
}

// Check opens gaps for silent datastreams. Returns the number of gaps opened.
func (d *Detector) Check(ctx context.Context) (int, error) {
	tag, err := d.pool.Exec(ctx, `
		INSERT INTO core.gap (org_id, datastream_id, gap_from)
		SELECT ds.org_id, ds.id, last.max_at + ds.expected_interval
		FROM core.datastream ds
		JOIN LATERAL (SELECT max(o.measured_at) AS max_at FROM core.observation o WHERE o.datastream_id = ds.id) last ON true
		WHERE ds.expected_interval IS NOT NULL
		  AND last.max_at IS NOT NULL
		  AND now() - last.max_at > ds.expected_interval * 1.5
		  AND NOT EXISTS (SELECT 1 FROM core.gap g WHERE g.datastream_id = ds.id AND g.closed_at IS NULL)`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
