package httpapi

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("no tz database:", err)
	}
	return loc
}

// Periods follow the local calendar, also across daylight-saving changes (23 h and 25 h days).
func TestPeriodRangesLocal(t *testing.T) {
	loc := mustLoc(t, "Europe/Bratislava")
	now := time.Date(2026, 10, 25, 15, 30, 0, 0, loc) // the day the clocks go back
	for _, c := range []struct {
		period         string
		shift          int
		from, to, want string
	}{
		{"today", 0, "", "", "2026-10-25 00:00 +0200 – 2026-10-26 00:00 +0100"},
		{"yesterday", 0, "", "", "2026-10-24 00:00 +0200 – 2026-10-25 00:00 +0200"},
		{"this_week", 0, "", "", "2026-10-19 00:00 +0200 – 2026-10-26 00:00 +0100"},
		{"last_month", 0, "", "", "2026-09-01 00:00 +0200 – 2026-10-01 00:00 +0200"},
		{"this_month", -1, "", "", "2026-09-01 00:00 +0200 – 2026-10-01 00:00 +0200"},
		{"this_year", 0, "", "", "2026-01-01 00:00 +0100 – 2027-01-01 00:00 +0100"},
		{"last_7d", 0, "", "", "2026-10-19 00:00 +0200 – 2026-10-26 00:00 +0100"},
		{"last_12m", 0, "", "", "2025-11-01 00:00 +0100 – 2026-11-01 00:00 +0100"},
		{"custom", 0, "2026-03-28", "2026-03-30", "2026-03-28 00:00 +0100 – 2026-03-31 00:00 +0200"},
		{"custom", -1, "2026-03-28", "2026-03-30", "2026-03-25 00:00 +0100 – 2026-03-28 00:00 +0100"},
	} {
		f, to, err := periodRange(c.period, c.shift, c.from, c.to, now, loc)
		if err != nil {
			t.Fatalf("%s: %v", c.period, err)
		}
		if got := f.Format("2006-01-02 15:04 -0700") + " – " + to.Format("2006-01-02 15:04 -0700"); got != c.want {
			t.Errorf("%s shift %d: %s, want %s", c.period, c.shift, got, c.want)
		}
	}
	if _, _, err := periodRange("custom", 0, "2026-03-30", "2026-03-28", now, loc); err == nil {
		t.Error("custom with to before from accepted")
	}
	// days across the change: 24 local days in October from the 8th, one of them 25 h long
	f, to, _ := periodRange("custom", 0, "2026-10-08", "2026-10-31", now, loc)
	ps := ptPeriodsOf(f, to, "day", loc)
	if len(ps) != 24 || ps[17].Label != "2026-10-25" || ps[17].End.Sub(ps[17].Start) != 25*time.Hour || ps[23].Start.Hour() != 0 {
		t.Fatalf("days: %d %v %v", len(ps), ps[17], ps[23].Start)
	}
	// hours of the spring-forward day: 23
	f, to, _ = periodRange("custom", 0, "2026-03-29", "2026-03-29", now, loc)
	if n := len(ptPeriodsOf(f, to, "hour", loc)); n != 23 {
		t.Errorf("hours on 29 March: %d", n)
	}
	// months of a year, weeks starting on Monday
	f, to, _ = periodRange("this_year", 0, "", "", now, loc)
	if ps := ptPeriodsOf(f, to, "month", loc); len(ps) != 12 || ps[2].Label != "2026-03" {
		t.Errorf("months: %v", ps)
	}
	f, to, _ = periodRange("this_month", 0, "", "", now, loc)
	ws := ptPeriodsOf(f, to, "week", loc)
	if ws[0].Start.Weekday() != time.Monday || !strings.HasPrefix(ws[0].Label, "2026-W40") {
		t.Errorf("weeks: %v", ws[0])
	}
	if rangeLabel(time.Date(2026, 9, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 1, 0, 0, 0, 0, loc), "") != "2026-09" ||
		rangeLabel(time.Date(2026, 9, 1, 0, 0, 0, 0, loc), time.Date(2026, 9, 8, 0, 0, 0, 0, loc), "") != "2026-09-01 – 2026-09-07" {
		t.Error("range labels")
	}
}

func TestParsePeriodTable(t *testing.T) {
	loc := mustLoc(t, "Europe/Bratislava")
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, loc)
	id := "01a0e90b-bf8f-7d6e-b71a-2d97dd0426b7"
	ok := url.Values{"datastreams": {id}, "interval": {"day"}, "values": {"min,max,out"}, "low": {"2"}, "high": {"8"}}
	q, err := parsePeriodTable(ok, loc, now)
	if err != nil || q.Period != "this_month" || len(q.Values) != 3 || *q.Low != 2 || q.Orientation != "periods" {
		t.Fatalf("%+v %v", q, err)
	}
	for name, bad := range map[string]url.Values{
		"no datastreams":  {},
		"bad id":          {"datastreams": {"x"}},
		"repeated id":     {"datastreams": {id + "," + id}},
		"interval":        {"datastreams": {id}, "interval": {"minute"}},
		"value":           {"datastreams": {id}, "values": {"median"}},
		"period":          {"datastreams": {id}, "period": {"forever"}},
		"low":             {"datastreams": {id}, "low": {"cold"}},
		"orientation":     {"datastreams": {id}, "orientation": {"diagonal"}},
		"too many hours":  {"datastreams": {id}, "interval": {"hour"}, "period": {"this_year"}},
		"decimals":        {"datastreams": {id}, "decimals": {"9"}},
		"custom no dates": {"datastreams": {id}, "period": {"custom"}},
	} {
		if _, err := parsePeriodTable(bad, loc, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func fp(v float64) *float64 { return &v }

// Totals: extremes, the average of all samples, first and last reading, the sum of consumption.
func TestPeriodTotalsAndGrid(t *testing.T) {
	cells := []ptCell{
		{Min: fp(2), Max: fp(5), Avg: fp(3), First: fp(100), Last: fp(110), Delta: fp(10), Count: 2, Out: 0, sum: 6},
		{},
		{Min: fp(1), Max: fp(9), Avg: fp(6), First: fp(110), Last: fp(125), Delta: fp(15), Count: 4, Out: 2, sum: 24, Flagged: true},
	}
	tot := ptTotal(cells)
	if *tot.Min != 1 || *tot.Max != 9 || *tot.Avg != 5 || *tot.First != 100 || *tot.Last != 125 || *tot.Delta != 25 || tot.Count != 6 || tot.Out != 2 || !tot.Flagged {
		t.Fatalf("total %+v avg %v", tot, *tot.Avg)
	}
	tb := &ptTable{Columns: []ptColumn{{ID: "a", Label: "Fridge / t", Unit: "°C"}}, Periods: []ptPeriod{{Label: "2026-09-01"}, {Label: "2026-09-02"}, {Label: "2026-09-03"}},
		Cells: map[string][]ptCell{"a": cells}, Totals: map[string]ptCell{"a": tot}}
	q := &ptQuery{Values: []string{"min", "max"}, Orientation: "periods", Total: true, Decimals: 1}
	id := func(s string) string { return s }
	h, num, rows := ptGrid(tb, q, id)
	if strings.Join(h, "|") != "Period|Fridge / t min [°C]|Fridge / t max [°C]" || num[0] || !num[1] {
		t.Fatalf("header %q", h)
	}
	if len(rows) != 4 || rows[1][1].IsNum || ptCellString(rows[2][2]) != "9.0" || rows[3][0].Text != "Total" || !rows[3][1].Bold {
		t.Fatalf("rows %+v", rows)
	}
	q = &ptQuery{Values: []string{"delta"}, Orientation: "datastreams", Total: true, Decimals: -1}
	h, _, rows = ptGrid(tb, q, id)
	if strings.Join(h, "|") != "Datastream|2026-09-01|2026-09-02|2026-09-03|Total consumption" || len(rows) != 1 || ptCellString(rows[0][4]) != "25.00" {
		t.Fatalf("meters: %q %+v", h, rows)
	}
	if fileSafe("Teploty: lieky/lekáreň 1.") != "Teploty-lieky-lekaren-1" {
		t.Errorf("file name %q", fileSafe("Teploty: lieky/lekáreň 1."))
	}
}
