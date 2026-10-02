// Package counting keeps object counting (spec 029): areas (a building, a car park, a zone) with entrances, the
// people and vehicles that pass them — from the analytics workers on camera pictures or from sensors — and the
// occupancy of each area. Every number is an observation of a datastream on an asset (area, entrance, speed section),
// so the rest of the system (dashboards, period tables, alarm rules, flows, exports) works on it unchanged.
package counting

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/schedule"
)

// Groups are what an area counts: people, vehicles (cars, motorcycles, buses and trucks together), and single
// classes. Each group has the datastreams <group>_in and <group>_out on the entrances and the area, and
// <group>_occupancy on the area.
var Groups = []string{"people", "vehicles", "cars", "trucks", "buses", "motorcycles", "bicycles"}

var classGroups = map[string][]string{
	"person":     {"people"},
	"car":        {"vehicles", "cars"},
	"truck":      {"vehicles", "trucks"},
	"bus":        {"vehicles", "buses"},
	"motorcycle": {"vehicles", "motorcycles"},
	"bicycle":    {"bicycles"},
}

// GroupsOf lists the groups a detected class counts for.
func GroupsOf(class string) []string { return classGroups[class] }

// ClassesFor lists the detector classes the groups need.
func ClassesFor(groups []string) []string {
	var out []string
	for _, c := range []string{"person", "bicycle", "car", "motorcycle", "bus", "truck"} {
		for _, g := range classGroups[c] {
			if slices.Contains(groups, g) && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
}

// ValidGroups checks a list of groups.
func ValidGroups(gs []string) error {
	if len(gs) == 0 {
		return fmt.Errorf("groups: at least one of %s", strings.Join(Groups, ", "))
	}
	seen := map[string]bool{}
	for _, g := range gs {
		if !slices.Contains(Groups, g) || seen[g] {
			return fmt.Errorf("groups: unknown or repeated %q (%s)", g, strings.Join(Groups, ", "))
		}
		seen[g] = true
	}
	return nil
}

// Key is the datastream key of a group: in, out or occupancy.
func Key(group, what string) string { return group + "_" + what }

// Unit of a group's datastreams.
func Unit(group string) string { return group }

// Apply adds arrivals and departures to an occupancy. It never goes below zero: more departures than the area
// holds (a person counted out who was not counted in, a reset during the night shift) clamp it at zero and report it.
func Apply(occ, in, out int64) (int64, bool) {
	n := occ + in - out
	if n < 0 {
		return 0, true
	}
	return n, false
}

// ParseHHMM parses a local time of day "HH:MM".
func ParseHHMM(s string) (h, m int, err error) {
	hs, ms, ok := strings.Cut(s, ":")
	if !ok || len(hs) != 2 || len(ms) != 2 {
		return 0, 0, fmt.Errorf("time of day as HH:MM")
	}
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time of day as HH:MM (00:00 to 23:59)")
	}
	return h, m, nil
}

// LastReset is the most recent moment at or before now when the daily reset at local time hhmm was due. A time that
// does not exist on a day (02:30 when the clocks jump from 02:00 to 03:00) falls on the first moment after the jump.
func LastReset(now time.Time, hhmm string, loc *time.Location) (time.Time, error) {
	h, m, err := ParseHHMM(hhmm)
	if err != nil {
		return time.Time{}, err
	}
	l := now.In(loc)
	for d := 0; d < 3; d++ {
		day := l.AddDate(0, 0, -d)
		t := schedule.LocalTime(day.Year(), day.Month(), day.Day(), h, m, loc)
		if !t.After(now) {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("no reset time found")
}

// ResetDue reports whether the occupancy must be reset now, and the scheduled moment of that reset: it is due when
// the last reset happened before the most recent scheduled one (also when the server was down at that moment).
func ResetDue(lastReset, now time.Time, hhmm string, loc *time.Location) (time.Time, bool) {
	if hhmm == "" {
		return time.Time{}, false
	}
	t, err := LastReset(now, hhmm, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, lastReset.Before(t)
}

// Share is the percentage of part in total, one decimal.
func Share(part, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(part/total*1000) / 10
}
