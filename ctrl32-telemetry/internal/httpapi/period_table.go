package httpapi

// Period tables (spec 023): values of datastreams per local period — hour, day, week, month or year in the
// organization's time zone — for overviews such as a daily log of a fridge (minimum and maximum per day) or the
// consumption of electricity meters per month. One computation serves the dashboard widget "Period table", its
// exports (XLSX, CSV, a PDF stored under Reports) and the public dashboard link.
//
//	GET /api/v1/period-table          data.read    the table as JSON
//	GET /api/v1/period-table/export   data.export  ?format=xlsx|csv|pdf, audited (PDF as a report record)
//	GET /api/v1/public/{token}/period-table        datastreams of the shared dashboard only
//
// Values per cell: min, max, avg, first, last, sum (of the values: passages of object counting), delta (consumption of a counter: the sum of increases between readings,
// a meter reset counts the new reading, the reading before the period is the baseline), count (samples) and out
// (samples outside low/high). Periods start at local midnight (and local month starts), so days and months match the
// calendar of the organization, also across daylight-saving changes.

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/i18n"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

var (
	ptIntervals = []string{"hour", "day", "week", "month", "year"}
	ptValues    = []string{"min", "max", "avg", "first", "last", "delta", "sum", "count", "out"}
	ptPeriods   = []string{"today", "yesterday", "this_week", "last_week", "this_month", "last_month", "this_year", "last_year", "last_7d", "last_30d", "last_12m", "custom"}
	// ptValueLabels name the values in column headers of exports
	ptValueLabels = map[string]string{"min": "min", "max": "max", "avg": "average", "first": "start", "last": "end", "delta": "consumption", "sum": "sum", "count": "samples", "out": "out of limits"}
)

const (
	ptMaxDatastreams = 50
	ptMaxPeriods     = 1000
)

// ptQuery is a validated request.
type ptQuery struct {
	IDs         []string
	Labels      []string // optional display names, aligned with IDs
	Interval    string
	Values      []string
	Period      string
	Shift       int
	From, To    time.Time // local start (inclusive) and end (exclusive)
	Low, High   *float64
	Orientation string // periods (a row per period) or datastreams (a row per datastream)
	Total       bool
	Decimals    int
	Title       string
	loc         *time.Location
}

// ptCell is the aggregate of one datastream in one period.
type ptCell struct {
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Avg     *float64 `json:"avg,omitempty"`
	First   *float64 `json:"first,omitempty"`
	Last    *float64 `json:"last,omitempty"`
	Delta   *float64 `json:"delta,omitempty"`
	Sum     *float64 `json:"sum,omitempty"` // sum of the values: passages counted per reading (object counting, spec 029)
	Count   int64    `json:"count"`
	Out     int64    `json:"out"`
	Flagged bool     `json:"flagged,omitempty"` // a sample of the period has a doubtful quality (not ok, not backfilled)
	sum     float64
}

func (c *ptCell) value(v string) *float64 {
	f := func(x float64) *float64 { return &x }
	switch v {
	case "min":
		return c.Min
	case "max":
		return c.Max
	case "avg":
		return c.Avg
	case "first":
		return c.First
	case "last":
		return c.Last
	case "delta":
		return c.Delta
	case "sum":
		return c.Sum
	case "count":
		return f(float64(c.Count))
	case "out":
		return f(float64(c.Out))
	}
	return nil
}

type ptColumn struct {
	ID    string `json:"datastream_id"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	Kind  string `json:"kind"`
}

type ptPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Label string    `json:"label"`
	key   string
}

type ptTable struct {
	TZ       string              `json:"tz"`
	Interval string              `json:"interval"`
	Period   string              `json:"period"`
	From     time.Time           `json:"from"`
	To       time.Time           `json:"to"`
	Label    string              `json:"label"` // e.g. 2026-09 or 2026-09-01 – 2026-09-07
	Values   []string            `json:"values"`
	Low      *float64            `json:"low,omitempty"`
	High     *float64            `json:"high,omitempty"`
	Columns  []ptColumn          `json:"columns"`
	Periods  []ptPeriod          `json:"periods"`
	Cells    map[string][]ptCell `json:"cells"` // datastream id → a cell per period
	Totals   map[string]ptCell   `json:"totals"`
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func optFloat(s string) (*float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("not a number: %s", s)
	}
	return &v, nil
}

// parsePeriodTable validates the query parameters.
func parsePeriodTable(q url.Values, loc *time.Location, now time.Time) (*ptQuery, error) {
	p := &ptQuery{loc: loc, Interval: q.Get("interval"), Period: q.Get("period"), Orientation: q.Get("orientation"), Total: q.Get("total") == "1" || q.Get("total") == "true", Decimals: -1}
	p.IDs = splitList(q.Get("datastreams"))
	if len(p.IDs) == 0 || len(p.IDs) > ptMaxDatastreams {
		return nil, fmt.Errorf("datastreams: 1 to %d ids, comma separated", ptMaxDatastreams)
	}
	seen := map[string]bool{}
	for _, id := range p.IDs {
		if !uuidRe.MatchString(strings.ToLower(id)) || seen[id] {
			return nil, fmt.Errorf("datastreams: invalid or repeated id %q", id)
		}
		seen[id] = true
	}
	p.Labels = q["label"]
	if p.Interval == "" {
		p.Interval = "day"
	}
	if !slices.Contains(ptIntervals, p.Interval) {
		return nil, fmt.Errorf("interval: %s", strings.Join(ptIntervals, ", "))
	}
	p.Values = splitList(q.Get("values"))
	if len(p.Values) == 0 {
		p.Values = []string{"min", "max"}
	}
	for _, v := range p.Values {
		if !slices.Contains(ptValues, v) {
			return nil, fmt.Errorf("values: %s", strings.Join(ptValues, ", "))
		}
	}
	if len(p.Values) > len(ptValues) {
		return nil, fmt.Errorf("values: at most %d", len(ptValues))
	}
	if p.Orientation == "" {
		p.Orientation = "periods"
	}
	if p.Orientation != "periods" && p.Orientation != "datastreams" {
		return nil, fmt.Errorf("orientation: periods or datastreams")
	}
	var err error
	if p.Low, err = optFloat(q.Get("low")); err != nil {
		return nil, fmt.Errorf("low: %v", err)
	}
	if p.High, err = optFloat(q.Get("high")); err != nil {
		return nil, fmt.Errorf("high: %v", err)
	}
	if d := q.Get("decimals"); d != "" {
		if p.Decimals, err = strconv.Atoi(d); err != nil || p.Decimals < 0 || p.Decimals > 6 {
			return nil, fmt.Errorf("decimals: 0 to 6")
		}
	}
	if s := q.Get("shift"); s != "" {
		if p.Shift, err = strconv.Atoi(s); err != nil || p.Shift < -1000 || p.Shift > 1000 {
			return nil, fmt.Errorf("shift: a whole number of periods, -1000 to 1000")
		}
	}
	p.Title = strings.TrimSpace(q.Get("title"))
	if len([]rune(p.Title)) > 120 {
		return nil, fmt.Errorf("title: at most 120 characters")
	}
	if p.Period == "" {
		p.Period = "this_month"
	}
	if !slices.Contains(ptPeriods, p.Period) {
		return nil, fmt.Errorf("period: %s", strings.Join(ptPeriods, ", "))
	}
	if p.From, p.To, err = periodRange(p.Period, p.Shift, q.Get("from"), q.Get("to"), now.In(loc), loc); err != nil {
		return nil, err
	}
	if n := len(ptPeriodsOf(p.From, p.To, p.Interval, loc)); n == 0 || n > ptMaxPeriods {
		return nil, fmt.Errorf("%d periods of %s: choose a longer interval or a shorter period (at most %d)", n, p.Interval, ptMaxPeriods)
	}
	return p, nil
}

// periodRange resolves a named period (moved by shift of its own length) to local [from, to).
func periodRange(period string, shift int, fromS, toS string, now time.Time, loc *time.Location) (time.Time, time.Time, error) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	week := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7)) // Monday
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	year := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	var f, t time.Time
	switch period {
	case "today":
		f, t = day.AddDate(0, 0, shift), day.AddDate(0, 0, shift+1)
	case "yesterday":
		f, t = day.AddDate(0, 0, shift-1), day.AddDate(0, 0, shift)
	case "this_week":
		f, t = week.AddDate(0, 0, 7*shift), week.AddDate(0, 0, 7*shift+7)
	case "last_week":
		f, t = week.AddDate(0, 0, 7*shift-7), week.AddDate(0, 0, 7*shift)
	case "this_month":
		f, t = month.AddDate(0, shift, 0), month.AddDate(0, shift+1, 0)
	case "last_month":
		f, t = month.AddDate(0, shift-1, 0), month.AddDate(0, shift, 0)
	case "this_year":
		f, t = year.AddDate(shift, 0, 0), year.AddDate(shift+1, 0, 0)
	case "last_year":
		f, t = year.AddDate(shift-1, 0, 0), year.AddDate(shift, 0, 0)
	case "last_7d":
		f, t = day.AddDate(0, 0, 7*shift-6), day.AddDate(0, 0, 7*shift+1)
	case "last_30d":
		f, t = day.AddDate(0, 0, 30*shift-29), day.AddDate(0, 0, 30*shift+1)
	case "last_12m":
		f, t = month.AddDate(0, 12*shift-11, 0), month.AddDate(0, 12*shift+1, 0)
	case "custom":
		a, err1 := time.ParseInLocation("2006-01-02", fromS, loc)
		b, err2 := time.ParseInLocation("2006-01-02", toS, loc)
		if err1 != nil || err2 != nil || b.Before(a) {
			return f, t, fmt.Errorf("custom period: from and to as YYYY-MM-DD, to not before from (both days included)")
		}
		days := int(b.Sub(a).Hours()/24+0.5) + 1
		f, t = a.AddDate(0, 0, days*shift), b.AddDate(0, 0, days*shift+1)
	}
	return f, t, nil
}

// ptPeriodsOf lists the local periods of an interval in [from, to).
func ptPeriodsOf(from, to time.Time, interval string, loc *time.Location) []ptPeriod {
	start := truncLocal(from, interval, loc)
	var out []ptPeriod
	for s := start; s.Before(to) && len(out) <= ptMaxPeriods; {
		n := nextLocal(s, interval)
		out = append(out, ptPeriod{Start: s, End: n, Label: ptLabel(s, interval), key: s.Format("2006-01-02 15")})
		s = n
	}
	return out
}

func truncLocal(t time.Time, interval string, loc *time.Location) time.Time {
	t = t.In(loc)
	switch interval {
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
	case "week":
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	case "year":
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, loc)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func nextLocal(t time.Time, interval string) time.Time {
	switch interval {
	case "hour":
		return t.Add(time.Hour)
	case "week":
		return t.AddDate(0, 0, 7)
	case "month":
		return t.AddDate(0, 1, 0)
	case "year":
		return t.AddDate(1, 0, 0)
	}
	return t.AddDate(0, 0, 1)
}

func ptLabel(t time.Time, interval string) string {
	switch interval {
	case "hour":
		return t.Format("2006-01-02 15:00")
	case "week":
		y, w := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d (%s)", y, w, t.Format("2006-01-02"))
	case "month":
		return t.Format("2006-01")
	case "year":
		return t.Format("2006")
	}
	return t.Format("2006-01-02")
}

// computePeriodTable runs the aggregation for the organization of the principal.
func (s *Server) computePeriodTable(ctx context.Context, p *iam.Principal, q *ptQuery) (*ptTable, error) {
	org := orgOf(p)
	// columns: the datastreams of this organization, in the requested order
	rows, err := s.pool.Query(ctx, `SELECT ds.id::text, a.name || ' / ' || ds.key, ds.unit, ds.kind FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id
		WHERE ds.id = ANY($1::uuid[]) AND ($2 = '' OR ds.org_id = $2::uuid)`, q.IDs, org)
	if err != nil {
		return nil, err
	}
	byID := map[string]ptColumn{}
	for rows.Next() {
		var c ptColumn
		if err := rows.Scan(&c.ID, &c.Label, &c.Unit, &c.Kind); err != nil {
			rows.Close()
			return nil, err
		}
		byID[c.ID] = c
	}
	rows.Close()
	t := &ptTable{TZ: q.loc.String(), Interval: q.Interval, Period: q.Period, From: q.From, To: q.To, Values: q.Values, Low: q.Low, High: q.High,
		Cells: map[string][]ptCell{}, Totals: map[string]ptCell{}}
	t.Periods = ptPeriodsOf(q.From, q.To, q.Interval, q.loc)
	t.Label = rangeLabel(q.From, q.To, q.Period)
	index := map[string]int{}
	for i, pr := range t.Periods {
		index[pr.key] = i
	}
	for i, id := range q.IDs {
		c, ok := byID[strings.ToLower(id)]
		if !ok {
			return nil, fmt.Errorf("datastream %s not found", id)
		}
		if i < len(q.Labels) && strings.TrimSpace(q.Labels[i]) != "" {
			c.Label = strings.TrimSpace(q.Labels[i])
			if len([]rune(c.Label)) > 80 {
				c.Label = string([]rune(c.Label)[:80])
			}
		}
		t.Columns = append(t.Columns, c)
		t.Cells[c.ID] = make([]ptCell, len(t.Periods))
	}
	ids := make([]string, 0, len(t.Columns))
	for _, c := range t.Columns {
		ids = append(ids, c.ID)
	}
	rows, err = s.pool.Query(ctx, `
		WITH o AS (
		  SELECT datastream_id, measured_at, value::float8 AS value, quality,
		         lag(value::float8) OVER (PARTITION BY datastream_id ORDER BY measured_at) AS prev
		  FROM core.observation
		  WHERE datastream_id = ANY($1::uuid[]) AND ($6 = '' OR org_id = $6::uuid) AND value IS NOT NULL
		    AND measured_at < $3 AND measured_at >= $2::timestamptz - interval '2 days')
		SELECT datastream_id::text, date_trunc($4, measured_at AT TIME ZONE $5),
		  min(value), max(value), sum(value), count(*),
		  (array_agg(value ORDER BY measured_at))[1], (array_agg(value ORDER BY measured_at DESC))[1],
		  coalesce(sum(CASE WHEN prev IS NULL THEN 0 WHEN value >= prev THEN value - prev WHEN value < prev * 0.5 THEN value ELSE 0 END), 0),
		  count(*) FILTER (WHERE ($7::float8 IS NOT NULL AND value < $7::float8) OR ($8::float8 IS NOT NULL AND value > $8::float8)),
		  bool_or(quality NOT IN ('ok', 'backfilled'))
		FROM o WHERE measured_at >= $2 GROUP BY 1, 2`,
		ids, q.From, q.To, q.Interval, q.loc.String(), org, q.Low, q.High)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var b time.Time
		var mn, mx, sum, first, last, delta float64
		var n, out int64
		var flagged bool
		if err := rows.Scan(&id, &b, &mn, &mx, &sum, &n, &first, &last, &delta, &out, &flagged); err != nil {
			return nil, err
		}
		i, ok := index[b.Format("2006-01-02 15")]
		if !ok || n == 0 {
			continue
		}
		avg := sum / float64(n)
		total := sum
		t.Cells[id][i] = ptCell{Min: &mn, Max: &mx, Avg: &avg, First: &first, Last: &last, Delta: &delta, Sum: &total, Count: n, Out: out, Flagged: flagged, sum: sum}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range t.Columns {
		t.Totals[c.ID] = ptTotal(t.Cells[c.ID])
	}
	return t, nil
}

// ptTotal summarises the periods: min of minima, max of maxima, average of all samples, first and last reading,
// total consumption, samples and samples out of limits.
func ptTotal(cells []ptCell) ptCell {
	var t ptCell
	var sum float64
	for i := range cells {
		c := &cells[i]
		if c.Count == 0 {
			continue
		}
		if t.Min == nil || *c.Min < *t.Min {
			t.Min = c.Min
		}
		if t.Max == nil || *c.Max > *t.Max {
			t.Max = c.Max
		}
		if t.First == nil {
			t.First = c.First
		}
		t.Last = c.Last
		d := *c.Delta
		if t.Delta != nil {
			d += *t.Delta
		}
		t.Delta = &d
		t.Count += c.Count
		t.Out += c.Out
		if c.Sum != nil {
			v := *c.Sum
			if t.Sum != nil {
				v += *t.Sum
			}
			t.Sum = &v
		}
		t.Flagged = t.Flagged || c.Flagged
		sum += c.sum
	}
	if t.Count > 0 {
		avg := sum / float64(t.Count)
		t.Avg = &avg
	}
	return t
}

func rangeLabel(from, to time.Time, period string) string {
	last := to.AddDate(0, 0, -1)
	switch {
	case from.Day() == 1 && to.Day() == 1 && from.AddDate(0, 1, 0).Equal(to):
		return from.Format("2006-01")
	case from.YearDay() == 1 && from.AddDate(1, 0, 0).Equal(to):
		return from.Format("2006")
	case from.AddDate(0, 0, 1).Equal(to):
		return from.Format("2006-01-02")
	}
	return from.Format("2006-01-02") + " – " + last.Format("2006-01-02")
}

// GET /api/v1/period-table
func (s *Server) periodTable(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	loc := s.orgLoc(r.Context(), orgOf(p))
	q, err := parsePeriodTable(r.URL.Query(), loc, time.Now())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t, err := s.computePeriodTable(r.Context(), p, q)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, 404, err.Error())
			return
		}
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, t)
}

// GET /api/v1/public/{token}/period-table
func (s *Server) publicPeriodTable(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	for _, id := range splitList(r.URL.Query().Get("datastreams")) {
		if !si.DS[id] {
			writeErr(w, 403, "datastream is not part of this dashboard")
			return
		}
	}
	s.periodTable(w, s.withPublicPrincipal(r, si))
}

// ptGrid lays the table out as header and rows of cells for an export.
func ptGrid(t *ptTable, q *ptQuery, tr func(string) string) (header []string, numeric []bool, rows [][]xlsx.Cell) {
	dec := func(c ptColumn, v string) int {
		if v == "count" || v == "out" {
			return 0
		}
		if q.Decimals >= 0 {
			return q.Decimals
		}
		return 2
	}
	cell := func(c *ptCell, col ptColumn, v string) xlsx.Cell {
		if c == nil || c.Count == 0 {
			return xlsx.Cell{}
		}
		x := c.value(v)
		if x == nil {
			return xlsx.Cell{}
		}
		return xlsx.N(*x, dec(col, v))
	}
	unit := func(c ptColumn, v string) string {
		if c.Unit == "" || v == "count" || v == "out" {
			return ""
		}
		return " [" + c.Unit + "]"
	}
	name := func(v string) string { return tr(ptValueLabels[v]) }
	if q.Orientation == "datastreams" {
		header = []string{tr("Datastream")}
		numeric = []bool{false}
		for _, pr := range t.Periods {
			for _, v := range q.Values {
				h := pr.Label
				if len(q.Values) > 1 {
					h += " " + name(v)
				}
				header, numeric = append(header, h), append(numeric, true)
			}
		}
		if q.Total {
			for _, v := range q.Values {
				header, numeric = append(header, tr("Total")+" "+name(v)), append(numeric, true)
			}
		}
		for _, c := range t.Columns {
			row := []xlsx.Cell{xlsx.T(c.Label + unit(c, "min"))}
			for i := range t.Periods {
				for _, v := range q.Values {
					row = append(row, cell(&t.Cells[c.ID][i], c, v))
				}
			}
			if q.Total {
				tot := t.Totals[c.ID]
				for _, v := range q.Values {
					x := cell(&tot, c, v)
					x.Bold = true
					row = append(row, x)
				}
			}
			rows = append(rows, row)
		}
		return
	}
	header = []string{tr("Period")}
	numeric = []bool{false}
	for _, c := range t.Columns {
		for _, v := range q.Values {
			h := c.Label
			if len(q.Values) > 1 || len(t.Columns) == 1 {
				h += " " + name(v)
			}
			header, numeric = append(header, h+unit(c, v)), append(numeric, true)
		}
	}
	for i, pr := range t.Periods {
		row := []xlsx.Cell{xlsx.T(pr.Label)}
		for _, c := range t.Columns {
			for _, v := range q.Values {
				row = append(row, cell(&t.Cells[c.ID][i], c, v))
			}
		}
		rows = append(rows, row)
	}
	if q.Total {
		row := []xlsx.Cell{xlsx.B(tr("Total"))}
		for _, c := range t.Columns {
			tot := t.Totals[c.ID]
			for _, v := range q.Values {
				x := cell(&tot, c, v)
				x.Bold = true
				row = append(row, x)
			}
		}
		rows = append(rows, row)
	}
	return
}

func ptCellString(c xlsx.Cell) string {
	if c.IsNum {
		return strconv.FormatFloat(c.Num, 'f', c.Decimals, 64)
	}
	return c.Text
}

// GET /api/v1/period-table/export?format=xlsx|csv|pdf
func (s *Server) periodTableExport(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	loc := s.orgLoc(r.Context(), orgOf(p))
	q, err := parsePeriodTable(r.URL.Query(), loc, time.Now())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "xlsx"
	}
	if format != "xlsx" && format != "csv" && format != "pdf" {
		writeErr(w, 400, "format: xlsx, csv or pdf")
		return
	}
	if format == "pdf" && s.reportsUnavailable(w) {
		return
	}
	t, err := s.computePeriodTable(r.Context(), p, q)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeErr(w, 404, err.Error())
			return
		}
		writeErr(w, 500, "query failed")
		return
	}
	lang := s.exportLanguage(r, p)
	cat := i18n.Builtin(lang)
	tr := func(k string) string {
		if v := cat[k]; v != "" {
			return v
		}
		return k
	}
	title := q.Title
	if title == "" {
		title = tr("Period table")
	}
	header, numeric, rows := ptGrid(t, q, tr)
	stamp := time.Now().In(loc).Format("20060102-1504")
	base := fileSafe(title) + "_" + fileSafe(t.Label)
	subtitle := fmt.Sprintf("%s · %s · %s", t.Label, tr(q.Interval), t.TZ)
	switch format {
	case "csv":
		var b bytes.Buffer
		b.WriteString("\ufeff") // Excel opens UTF-8 with the byte-order mark
		cw := csv.NewWriter(&b)
		_ = cw.Write(header)
		for _, row := range rows {
			rec := make([]string, len(row))
			for i, c := range row {
				rec[i] = ptCellString(c)
			}
			_ = cw.Write(rec)
		}
		cw.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.csv"`, base, stamp))
		_, _ = w.Write(b.Bytes())
	case "xlsx":
		sheet := xlsx.Sheet{Name: title, FreezeRows: 5}
		sheet.Rows = append(sheet.Rows, []xlsx.Cell{xlsx.B(title)}, []xlsx.Cell{xlsx.T(subtitle)},
			[]xlsx.Cell{xlsx.T(fmt.Sprintf("%s · %s %s", s.orgName(r, p.OrgID), time.Now().In(loc).Format("2006-01-02 15:04"), p.Username))}, nil)
		hdr := make([]xlsx.Cell, len(header))
		for i, h := range header {
			hdr[i] = xlsx.B(h)
			w := float64(len([]rune(h))) + 2
			if i == 0 {
				w = 22
			}
			sheet.Widths = append(sheet.Widths, max(10, min(w, 40)))
		}
		sheet.Rows = append(sheet.Rows, hdr)
		sheet.Rows = append(sheet.Rows, rows...)
		b, err := xlsx.Bytes(sheet)
		if err != nil {
			writeErr(w, 500, "xlsx failed")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.xlsx"`, base, stamp))
		_, _ = w.Write(b)
	case "pdf":
		cols := make([]report.Column, len(header))
		for i, h := range header {
			al := "left"
			if numeric[i] {
				al = "right"
			}
			cols[i] = report.Column{Key: fmt.Sprintf("c%d", i), Label: h, Align: al}
		}
		data := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			m := map[string]any{}
			for i, c := range row {
				m[fmt.Sprintf("c%d", i)] = ptCellString(c)
			}
			data = append(data, m)
		}
		d := report.Data{Kind: "generic", Title: title, Subtitle: subtitle, Org: s.orgName(r, p.OrgID), Locale: lang,
			Period:  report.Period{From: t.From.UTC().Format(time.RFC3339), To: t.To.UTC().Format(time.RFC3339)},
			Scope:   map[string]any{"datastreams": q.IDs, "interval": q.Interval, "values": q.Values, "period": t.Label, "tz": t.TZ},
			Columns: cols, Rows: data, Summary: map[string]any{"periods": len(t.Periods), "datastreams": len(t.Columns)},
			System: map[string]any{"version": s.version, "instance": s.cfg.HTTP.BaseURL}}
		assets := map[string][]byte{}
		if logo, name := s.orgLogo(r, p.OrgID); len(logo) > 0 {
			d.Logo = name
			assets[name] = logo
		}
		res, err := s.renderReport(r, p, d, assets)
		if err != nil {
			writeErr(w, 500, "report failed: "+err.Error())
			return
		}
		servePDF(w, res.PDF, base+"_"+stamp+".pdf", res.Record.ID)
		return // renderReport audited it
	}
	s.audit(r, "export.read", "export", "", nil, map[string]any{"export": "period_table", "format": format, "rows": len(rows), "query": r.URL.RawQuery}, "")
}

// exportLanguage is the language of an export: ?locale=, else the user's language, else English.
func (s *Server) exportLanguage(r *http.Request, p *iam.Principal) string {
	if l := localeParam(r, ""); l != "" && i18n.IsSupported(l) {
		return l
	}
	var l string
	if p != nil && p.UserID != "" {
		_ = s.pool.QueryRow(r.Context(), `SELECT coalesce(locale, '') FROM core.user_account WHERE id = $1::uuid`, p.UserID).Scan(&l)
	}
	if l != "" && i18n.IsSupported(l) {
		return l
	}
	return "en"
}

// fileSafe keeps letters (without diacritics), digits, dash and underscore for a file name.
func fileSafe(s string) string {
	var b strings.Builder
	for _, r := range fileLetters.Replace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '/' || r == '.' || r == ':':
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if out == "" {
		out = "table"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// fileLetters drops the diacritics of Central and Western European letters.
var fileLetters = strings.NewReplacer(
	"á", "a", "ä", "a", "à", "a", "â", "a", "ã", "a", "å", "a", "ą", "a", "č", "c", "ć", "c", "ç", "c", "ď", "d", "é", "e", "ě", "e", "ë", "e", "è", "e", "ê", "e", "ę", "e",
	"í", "i", "ï", "i", "ì", "i", "î", "i", "ĺ", "l", "ľ", "l", "ł", "l", "ň", "n", "ń", "n", "ñ", "n", "ó", "o", "ô", "o", "ö", "o", "ő", "o", "ò", "o", "õ", "o", "ø", "o",
	"ŕ", "r", "ř", "r", "š", "s", "ś", "s", "ş", "s", "ș", "s", "ť", "t", "ț", "t", "ú", "u", "ů", "u", "ü", "u", "ű", "u", "ù", "u", "û", "u", "ý", "y", "ÿ", "y", "ž", "z", "ź", "z", "ż", "z",
	"Á", "A", "Ä", "A", "Č", "C", "Ď", "D", "É", "E", "Ě", "E", "Í", "I", "Ĺ", "L", "Ľ", "L", "Ł", "L", "Ň", "N", "Ó", "O", "Ô", "O", "Ö", "O", "Ŕ", "R", "Ř", "R",
	"Š", "S", "Ś", "S", "Ť", "T", "Ú", "U", "Ů", "U", "Ü", "U", "Ý", "Y", "Ž", "Z", "Ź", "Z", "Ż", "Z", "ß", "ss")
