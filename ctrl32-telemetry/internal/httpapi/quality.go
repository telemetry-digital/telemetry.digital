package httpapi

import "time"

// Qualities of observations (core.observation.quality, migration 0001):
//
//	ok            a normal value
//	backfilled    sent later from the device's buffer; good data
//	sensor_fault  reported faulty by the device or outside the sensor's physical range (0.67.0)
//	out_of_range  stored by object counting when departures would make an occupancy negative: the value is kept at
//	              zero and flagged (internal/counting, docs/object-counting.md)
//	clock_suspect the device's time could not be trusted
//	stale         never stored: computed for the latest value of a datastream when its age exceeds the datastream's
//	              expected interval (staleFactor ×, the tolerance of gap detection in internal/gap); see liveQuality
//	simulated     produced by a simulator or a demo
//
// Stored observations are never rewritten; "stale" exists only in answers that show the latest value (latest values
// of dashboards and public links, the datastream query of overviews, the Telemetry tab of a device).

// staleFactor is the tolerance on the expected interval before the latest value counts as stale; the same factor
// opens a gap record (internal/gap).
const staleFactor = 1.5

// isStale reports whether a latest value measured at `at` is older than staleFactor × the expected interval
// (seconds; 0 = no expected interval, never stale).
func isStale(at time.Time, expectedS float64, now time.Time) bool {
	return expectedS > 0 && now.Sub(at).Seconds() > expectedS*staleFactor
}

// liveQuality is the quality shown for a latest value: a good value (ok, backfilled) that is stale shows "stale";
// any other stored quality (e.g. sensor_fault) is more telling and stays. stale is reported either way.
func liveQuality(stored string, at time.Time, expectedS float64, now time.Time) (quality string, stale bool) {
	stale = isStale(at, expectedS, now)
	if stale && (stored == "ok" || stored == "backfilled" || stored == "") {
		return "stale", true
	}
	return stored, stale
}

// applyLive sets quality, stale and stored_quality of a latest-value object.
func applyLive(l map[string]any, stored string, at time.Time, expectedS float64, now time.Time) {
	q, st := liveQuality(stored, at, expectedS, now)
	l["quality"] = q
	if st {
		l["stale"] = true
	}
	if q != stored {
		l["stored_quality"] = stored
	}
}
