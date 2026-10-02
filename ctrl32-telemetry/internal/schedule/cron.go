// Package schedule parses cron expressions and finds their next run in a time zone, across daylight-saving changes
// (scheduled reports, spec 029).
//
// Five fields: minute (0–59), hour (0–23), day of month (1–31), month (1–12 or JAN–DEC), day of week (0–7 or SUN–SAT,
// 0 and 7 are Sunday). Each field is *, a value, a range a-b, a list a,b,c and a step */n or a-b/n. When both the day
// of month and the day of week are restricted, a day matching either runs (as in classic cron). A wall-clock time that
// does not exist because the clocks jump forward runs at the first moment after the jump; a time that happens twice
// when the clocks go back runs once, the first time.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed expression.
type Cron struct {
	Expr                     string
	min, hour, dom, mon, dow uint64 // bit sets
	domStar, dowStar         bool
}

var monthNames = map[string]int{"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6, "JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12}
var dayNames = map[string]int{"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6}

// Presets offered in the user interface.
var Presets = map[string]string{
	"daily_0700":   "0 7 * * *",
	"monday_0700":  "0 7 * * 1",
	"monthly_0700": "0 7 1 * *",
}

// Parse parses a five-field cron expression.
func Parse(expr string) (*Cron, error) {
	f := strings.Fields(strings.ToUpper(strings.TrimSpace(expr)))
	if len(f) != 5 {
		return nil, fmt.Errorf("cron: five fields expected (minute hour day-of-month month day-of-week), e.g. 0 7 * * 1")
	}
	c := &Cron{Expr: strings.Join(f, " ")}
	var err error
	if c.min, err = field(f[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if c.hour, err = field(f[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if c.dom, err = field(f[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("cron day of month: %w", err)
	}
	if c.mon, err = field(f[3], 1, 12, monthNames); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	if c.dow, err = field(f[4], 0, 7, dayNames); err != nil {
		return nil, fmt.Errorf("cron day of week: %w", err)
	}
	if c.dow&(1<<7) != 0 {
		c.dow |= 1 // 7 is Sunday too
	}
	c.domStar, c.dowStar = f[2] == "*", f[4] == "*"
	return c, nil
}

func field(s string, lo, hi int, names map[string]int) (uint64, error) {
	var set uint64
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return 0, fmt.Errorf("empty item in %q", s)
		}
		rng, stepS, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepS)
			if err != nil || n < 1 || n > hi {
				return 0, fmt.Errorf("bad step in %q", part)
			}
			step = n
		}
		a, b := lo, hi
		if rng != "*" {
			x, y, isRange := strings.Cut(rng, "-")
			var err error
			if a, err = value(x, names); err != nil {
				return 0, err
			}
			b = a
			if isRange {
				if b, err = value(y, names); err != nil {
					return 0, err
				}
			} else if hasStep {
				b = hi // a/n: from a to the end
			}
		}
		if a < lo || b > hi || a > b {
			return 0, fmt.Errorf("%q outside %d–%d", part, lo, hi)
		}
		for v := a; v <= b; v += step {
			set |= 1 << uint(v)
		}
	}
	return set, nil
}

func value(s string, names map[string]int) (int, error) {
	if v, ok := names[s]; ok {
		return v, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q", s)
	}
	return v, nil
}

func (c *Cron) dayMatches(t time.Time) bool {
	if c.mon&(1<<uint(t.Month())) == 0 {
		return false
	}
	dom := c.dom&(1<<uint(t.Day())) != 0
	dow := c.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dow
	case c.dowStar:
		return dom
	}
	return dom || dow
}

// Next is the first run strictly after `after`, in the time zone loc. The zero time when there is none within 5 years
// (e.g. 30 February).
func (c *Cron) Next(after time.Time, loc *time.Location) time.Time {
	l := after.In(loc)
	day := time.Date(l.Year(), l.Month(), l.Day(), 12, 0, 0, 0, loc)
	for i := 0; i < 366*5; i++ {
		d := day.AddDate(0, 0, i)
		if !c.dayMatches(d) {
			continue
		}
		for h := 0; h < 24; h++ {
			if c.hour&(1<<uint(h)) == 0 {
				continue
			}
			for m := 0; m < 60; m++ {
				if c.min&(1<<uint(m)) == 0 {
					continue
				}
				t := LocalTime(d.Year(), d.Month(), d.Day(), h, m, loc)
				if t.After(after) {
					return t
				}
			}
		}
	}
	return time.Time{}
}

// LocalTime is the instant of a local wall-clock time. A time skipped by a daylight-saving jump becomes the first
// instant after the jump (02:30 on a day the clocks go from 02:00 to 03:00 is 03:00); a time that happens twice is the
// first of the two.
func LocalTime(y int, mo time.Month, d, h, m int, loc *time.Location) time.Time {
	t := time.Date(y, mo, d, h, m, 0, 0, loc)
	if l := t.In(loc); l.Hour() == h && l.Minute() == m {
		if e := t.Add(-time.Hour); e.In(loc).Hour() == h && e.In(loc).Minute() == m && e.In(loc).Day() == d {
			return e
		}
		return t
	}
	wall := func(x time.Time) int { l := x.In(loc); return l.Hour()*60 + l.Minute() }
	for i := 0; i < 180 && wall(t.Add(-time.Minute)) > h*60+m; i++ {
		t = t.Add(-time.Minute)
	}
	return t
}

// Period is the reporting period before a run: the previous local day, week (Monday to Sunday) or month.
func Period(kind string, run time.Time, loc *time.Location) (from, to time.Time, err error) {
	l := run.In(loc)
	today := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	switch kind {
	case "previous_day":
		return today.AddDate(0, 0, -1), today, nil
	case "previous_week":
		monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
		return monday.AddDate(0, 0, -7), monday, nil
	case "previous_month":
		first := time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, loc)
		return first.AddDate(0, -1, 0), first, nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("period: previous_day, previous_week or previous_month")
}
