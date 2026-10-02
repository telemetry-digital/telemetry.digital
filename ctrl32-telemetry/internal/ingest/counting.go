package ingest

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
)

// Counted is a value computed by object counting (spec 029): people or vehicles that passed an entrance, the
// occupancy of an area, the speed of a vehicle.
type Counted struct {
	OrgID        string
	DatastreamID string
	Key          string
	At           time.Time
	Value        float64
	JSON         map[string]any // details (class, direction of a speed), stored as value_json
	Quality      string         // ok (default), backfilled, out_of_range (occupancy clamped at zero)
}

// StoreCounted stores computed values as observations (server time source, decoder counting-v1, no raw message) and
// hands them to the sinks (alarm rules, flows, live data) like telemetry.
func (in *Ingest) StoreCounted(ctx context.Context, list []Counted) error {
	if len(list) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	obs := make([]Observation, len(list))
	times := make([]time.Time, 0, len(list))
	for i, c := range list {
		q := c.Quality
		if q == "" {
			q = "ok"
		}
		v := c.Value
		o := Observation{OrgID: c.OrgID, DatastreamID: c.DatastreamID, Key: c.Key, MeasuredAt: c.At.UTC(), Value: &v, Quality: q, Backfilled: q == "backfilled", timeSource: "server"}
		var js *string
		if len(c.JSON) > 0 {
			raw, _ := json.Marshal(c.JSON)
			s := string(raw)
			js = &s
			o.valueJSON = js
		}
		obs[i] = o
		times = append(times, o.MeasuredAt)
		b.Queue(`INSERT INTO core.observation (org_id, datastream_id, measured_at, value, value_json, quality, time_source, decoder_version)
			VALUES ($1::uuid, $2::uuid, $3, $4::numeric, $5::jsonb, $6, 'server', 'counting-v1') RETURNING id::text, received_at`,
			c.OrgID, c.DatastreamID, o.MeasuredAt, strconv.FormatFloat(v, 'g', -1, 64), js, q)
	}
	in.ensureMonths(ctx, "observation", times...)
	res := in.pool.SendBatch(ctx, b)
	for i := range obs {
		if err := res.QueryRow().Scan(&obs[i].ID, &obs[i].ReceivedAt); err != nil {
			_ = res.Close()
			return err
		}
	}
	if err := res.Close(); err != nil {
		return err
	}
	metrics.Observations.Add("", uint64(len(obs)))
	for _, s := range in.sinks {
		s.OnObservations(ctx, obs)
	}
	return nil
}
