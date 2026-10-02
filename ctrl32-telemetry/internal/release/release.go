// Package release knows the release date of this binary. The licence includes updates released until its
// updates_until date (ADR-0016 amendment of 2 Oct 2026), so the server agent compares the release date of a downloaded
// binary with it before installing.
//
// The date is set at build time by deploy/publish.sh:
//
//	go build -ldflags "-X github.com/telemetry-digital/ctrl32-telemetry/internal/release.date=2026-10-02"
//
// Builds without it (development, source builds) fall back to the commit time Go stamps into the binary (vcs.time);
// without either the date is unknown.
//
// Limitation: the update channel publishes VERSION, SHA256SUMS and the binaries without a signature of their own, so
// the release date is only as trustworthy as the downloaded binary itself (HTTPS from the distribution point plus the
// SHA-256 checksum from the same place). There is no separately signed release metadata.
package release

import (
	"regexp"
	"runtime/debug"
	"time"
)

// date is YYYY-MM-DD, set with -ldflags -X (see the package comment).
var date string

// Layout is the format of release dates.
const Layout = "2006-01-02"

// Date returns the release date of this binary and whether it is known.
func Date() (time.Time, bool) {
	if d, err := time.Parse(Layout, date); err == nil {
		return d, true
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.time" {
				if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
					return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
				}
			}
		}
	}
	return time.Time{}, false
}

// String is the release date as YYYY-MM-DD, or "" when it is unknown.
func String() string {
	if d, ok := Date(); ok {
		return d.Format(Layout)
	}
	return ""
}

var releasedRe = regexp.MustCompile(`\breleased (\d{4}-\d{2}-\d{2})\b`)

// FromVersionOutput reads the release date from the output of `ctrl32-telemetry version`
// ("ctrl32-telemetry 0.66.0 released 2026-10-02"). Binaries before 0.66.0 print no date.
func FromVersionOutput(out string) (time.Time, bool) {
	m := releasedRe.FindStringSubmatch(out)
	if m == nil {
		return time.Time{}, false
	}
	d, err := time.Parse(Layout, m[1])
	return d, err == nil
}

// Covered reports whether a release of the given date may be installed under a licence whose updates end on until
// (the whole day included). A zero until means unlimited updates.
func Covered(released, until time.Time) bool {
	if until.IsZero() {
		return true
	}
	return !released.After(until)
}
