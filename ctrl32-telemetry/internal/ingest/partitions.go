package ingest

import (
	"context"
	"sync"
	"time"
)

// Month partitions of the record tables (migration 0046). core.observation is partitioned by measured_at, so a
// backdated reading needs the partition of its month; core.raw_message is partitioned by received_at (now). The
// ingest makes sure the month of every reading exists before inserting it (cached, one database call per new
// month), and a daily job keeps the current month and three ahead, for every writer of the two tables.

var partitionCache sync.Map // "observation 2026-09" -> true

const partitionUpkeepEvery = 12 * time.Hour

func monthKey(tbl string, t time.Time) string { return tbl + " " + t.UTC().Format("2006-01") }

// ensureMonths creates the missing month partitions of tbl for the given times. Failures are only logged: the
// insert then reports what is wrong.
func (in *Ingest) ensureMonths(ctx context.Context, tbl string, times ...time.Time) {
	if in.pool == nil {
		return
	}
	for _, t := range times {
		k := monthKey(tbl, t)
		if _, ok := partitionCache.Load(k); ok {
			continue
		}
		var ok bool
		if err := in.pool.QueryRow(ctx, `SELECT core.ensure_record_month($1, $2)`, tbl, t.UTC()).Scan(&ok); err != nil {
			in.log.Error("month partition not created", "table", tbl, "month", t.UTC().Format("2006-01"), "err", err)
			continue
		}
		if ok {
			partitionCache.Store(k, true)
		} else {
			in.log.Warn("no partition for an implausible time (more than 20 years back or a year ahead)", "table", tbl, "at", t.UTC())
		}
	}
}

// partitionUpkeep keeps the partitions of the current month and three ahead (at start and twice a day).
func (in *Ingest) partitionUpkeep() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		var n int
		if err := in.pool.QueryRow(ctx, `SELECT core.ensure_record_partitions(0, 3)`).Scan(&n); err != nil {
			in.log.Error("record partitions not checked", "err", err)
		} else if n > 0 {
			in.log.Info("record partitions created", "count", n)
		}
		cancel()
		time.Sleep(partitionUpkeepEvery)
	}
}
