package schedule

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("no time zone data: " + name)
	}
	return loc
}

func TestParse(t *testing.T) {
	good := []string{"0 7 * * *", "0 7 * * 1", "0 7 1 * *", "*/15 * * * *", "0 6-18/2 * * MON-FRI", "30 2 * JAN,JUL 0", "0 0 * * 7", "5,35 8 1-7 * 1"}
	for _, e := range good {
		if _, err := Parse(e); err != nil {
			t.Errorf("%q: %v", e, err)
		}
	}
	bad := []string{"", "0 7 * *", "60 7 * * *", "0 24 * * *", "0 7 0 * *", "0 7 * 13 *", "0 7 * * 8", "*/0 * * * *", "a 7 * * *", "0 7 5-1 * *", "0 7 * * * *", "0,,1 7 * * *"}
	for _, e := range bad {
		if _, err := Parse(e); err == nil {
			t.Errorf("%q accepted", e)
		}
	}
}

func next(t *testing.T, expr, after string, loc *time.Location) string {
	c, err := Parse(expr)
	if err != nil {
		t.Fatal(err)
	}
	a, err := time.ParseInLocation("2006-01-02 15:04", after, loc)
	if err != nil {
		t.Fatal(err)
	}
	n := c.Next(a, loc)
	if n.IsZero() {
		return "never"
	}
	return n.In(loc).Format("2006-01-02 15:04 MST")
}

func TestNext(t *testing.T) {
	loc := mustLoc(t, "Europe/Bratislava")
	cases := []struct{ expr, after, want string }{
		{"0 7 * * *", "2026-10-02 06:59", "2026-10-02 07:00 CEST"},
		{"0 7 * * *", "2026-10-02 07:00", "2026-10-03 07:00 CEST"}, // strictly after
		{"0 7 * * 1", "2026-10-02 08:00", "2026-10-05 07:00 CEST"}, // Friday → Monday
		{"0 7 1 * *", "2026-10-02 08:00", "2026-11-01 07:00 CET"},  // across the change to winter time
		{"*/15 * * * *", "2026-10-02 10:07", "2026-10-02 10:15 CEST"},
		{"0 6-18/4 * * MON-FRI", "2026-10-02 18:01", "2026-10-05 06:00 CEST"},
		{"0 0 * * 7", "2026-10-02 08:00", "2026-10-04 00:00 CEST"},
		{"0 0 31 2 *", "2026-10-02 08:00", "never"},
		// both days restricted: the 1st of the month or a Monday, whichever comes first
		{"0 9 1 * 1", "2026-10-02 08:00", "2026-10-05 09:00 CEST"},
		{"0 9 29 2 *", "2026-10-02 08:00", "2028-02-29 09:00 CET"}, // leap day
	}
	for _, c := range cases {
		if got := next(t, c.expr, c.after, loc); got != c.want {
			t.Errorf("%s after %s: %s, want %s", c.expr, c.after, got, c.want)
		}
	}
}

func TestNextAcrossDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Bratislava")
	// spring forward 2027-03-28: 02:00 → 03:00, a daily 02:30 runs at 03:00 that day
	if got := next(t, "30 2 * * *", "2027-03-27 03:00", loc); got != "2027-03-28 03:00 CEST" {
		t.Fatalf("skipped time: %s", got)
	}
	// and the day after at 02:30 again
	if got := next(t, "30 2 * * *", "2027-03-28 03:00", loc); got != "2027-03-29 02:30 CEST" {
		t.Fatalf("after the jump: %s", got)
	}
	// fall back 2026-10-25: 03:00 → 02:00, 02:30 happens twice; it runs once, the first time (CEST)
	c, _ := Parse("30 2 * * *")
	first := c.Next(time.Date(2026, 10, 24, 23, 0, 0, 0, time.UTC), loc)
	if first.UTC().Format("2006-01-02 15:04") != "2026-10-25 00:30" {
		t.Fatalf("repeated time: %v", first.UTC())
	}
	second := c.Next(first, loc)
	if second.In(loc).Format("2006-01-02 15:04") != "2026-10-26 02:30" {
		t.Fatalf("ran twice in the repeated hour: %v", second.In(loc))
	}
	// a weekly run at 07:00 keeps the local wall clock across the change (UTC offset changes)
	w, _ := Parse("0 7 * * 1")
	a := w.Next(time.Date(2026, 10, 20, 0, 0, 0, 0, loc), loc) // Mon 26 Oct, after the change
	if a.In(loc).Format("15:04 MST") != "07:00 CET" || a.UTC().Hour() != 6 {
		t.Fatalf("weekly after the change: %v", a.In(loc))
	}
	// another zone: New York (DST changes on other days)
	ny := mustLoc(t, "America/New_York")
	if got := next(t, "0 7 * * *", "2026-11-01 06:59", ny); got != "2026-11-01 07:00 EST" {
		t.Fatalf("New York: %s", got)
	}
}

func TestPeriod(t *testing.T) {
	loc := mustLoc(t, "Europe/Bratislava")
	run := time.Date(2026, 10, 5, 5, 0, 0, 0, time.UTC) // Monday 07:00 CEST
	f, to, _ := Period("previous_day", run, loc)
	if f.In(loc).Format("2006-01-02 15:04") != "2026-10-04 00:00" || to.In(loc).Format("2006-01-02") != "2026-10-05" {
		t.Fatal(f, to)
	}
	f, to, _ = Period("previous_week", run, loc)
	if f.In(loc).Format("2006-01-02") != "2026-09-28" || to.In(loc).Format("2006-01-02") != "2026-10-05" {
		t.Fatal(f, to)
	}
	f, to, _ = Period("previous_month", time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC), loc)
	if f.In(loc).Format("2006-01-02 15:04") != "2026-10-01 00:00" || to.In(loc).Format("2006-01-02 15:04") != "2026-11-01 00:00" {
		t.Fatal(f.In(loc), to.In(loc))
	}
	// October 2026 has 31 days and 745 hours (the clocks go back)
	if h := to.Sub(f).Hours(); h != 745 {
		t.Fatalf("hours in October: %v", h)
	}
	if _, _, err := Period("yesterday", run, loc); err == nil {
		t.Fatal("unknown period accepted")
	}
}
