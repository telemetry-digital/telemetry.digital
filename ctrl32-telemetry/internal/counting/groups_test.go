package counting

import (
	"slices"
	"testing"
	"time"
)

func TestGroups(t *testing.T) {
	if g := GroupsOf("truck"); !slices.Equal(g, []string{"vehicles", "trucks"}) {
		t.Fatal(g)
	}
	if g := GroupsOf("person"); !slices.Equal(g, []string{"people"}) {
		t.Fatal(g)
	}
	if g := GroupsOf("dog"); g != nil {
		t.Fatal(g)
	}
	if c := ClassesFor([]string{"people", "vehicles"}); !slices.Equal(c, []string{"person", "car", "motorcycle", "bus", "truck"}) {
		t.Fatal(c)
	}
	if c := ClassesFor([]string{"cars", "bicycles"}); !slices.Equal(c, []string{"bicycle", "car"}) {
		t.Fatal(c)
	}
	if err := ValidGroups([]string{"people", "cars"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{nil, {"dogs"}, {"people", "people"}} {
		if ValidGroups(bad) == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if Key("people", "in") != "people_in" || Key("vehicles", "occupancy") != "vehicles_occupancy" {
		t.Fatal("keys")
	}
}

func TestApplyClamps(t *testing.T) {
	cases := []struct {
		occ, in, out, want int64
		clamped            bool
	}{
		{10, 3, 2, 11, false},
		{0, 0, 1, 0, true},
		{2, 0, 5, 0, true},
		{2, 1, 3, 0, false},
		{0, 1, 0, 1, false},
	}
	for _, c := range cases {
		got, cl := Apply(c.occ, c.in, c.out)
		if got != c.want || cl != c.clamped {
			t.Errorf("Apply(%d,+%d,-%d) = %d %v, want %d %v", c.occ, c.in, c.out, got, cl, c.want, c.clamped)
		}
	}
}

func TestResetDue(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Skip("no time zone data")
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// last reset yesterday at 03:00, now 02:59: not yet; 03:00: due, scheduled today 03:00
	if _, due := ResetDue(at("2026-10-01 03:00"), at("2026-10-02 02:59"), "03:00", loc); due {
		t.Fatal("due before the time")
	}
	when, due := ResetDue(at("2026-10-01 03:00"), at("2026-10-02 03:00"), "03:00", loc)
	if !due || !when.Equal(at("2026-10-02 03:00")) {
		t.Fatalf("%v %v", when, due)
	}
	// the server was down at 03:00: at 09:00 the reset is still due, dated 03:00
	when, due = ResetDue(at("2026-10-01 03:00"), at("2026-10-02 09:00"), "03:00", loc)
	if !due || !when.Equal(at("2026-10-02 03:00")) {
		t.Fatalf("missed reset: %v %v", when, due)
	}
	// already reset today
	if _, due := ResetDue(at("2026-10-02 03:00"), at("2026-10-02 23:00"), "03:00", loc); due {
		t.Fatal("reset twice a day")
	}
	// never reset
	if _, due := ResetDue(time.Time{}, at("2026-10-02 23:00"), "", loc); due {
		t.Fatal("no reset time, but due")
	}
	// spring forward: 2027-03-28 02:30 does not exist in Europe/Bratislava (02:00 → 03:00)
	when, due = ResetDue(at("2027-03-27 02:30"), time.Date(2027, 3, 28, 1, 0, 0, 0, time.UTC), "02:30", loc)
	if !due || when.UTC().Format("15:04") != "01:00" { // 03:00 CEST, the first moment after the jump
		t.Fatalf("spring forward: %v (UTC %v) %v", when, when.UTC(), due)
	}
	// fall back: 2026-10-25 02:30 happens twice; the reset is due once, at the first one (CEST, 00:30 UTC)
	when, due = ResetDue(at("2026-10-24 02:30"), time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC), "02:30", loc)
	if !due || when.UTC().Format("15:04") != "00:30" {
		t.Fatalf("fall back: %v %v", when.UTC(), due)
	}
	if _, due := ResetDue(when, time.Date(2026, 10, 25, 1, 45, 0, 0, time.UTC), "02:30", loc); due {
		t.Fatal("reset again in the repeated hour")
	}
	if _, err := LastReset(time.Now(), "25:00", loc); err == nil {
		t.Fatal("bad time accepted")
	}
}

func TestShare(t *testing.T) {
	if Share(1, 3) != 33.3 || Share(5, 0) != 0 || Share(2, 2) != 100 {
		t.Fatal(Share(1, 3), Share(5, 0))
	}
}
