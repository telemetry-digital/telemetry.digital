package httpapi

// Scheduled reports by e-mail (spec 029): Reports → Scheduled reports. A report has content (a summary of object
// counting for an area, a period table of a dashboard, or a report template: observations of a datastream or the
// alarms), a period (the previous day, week or month in the organization's time zone), a cron schedule, recipients
// (e-mail notification channels and addresses) and PDF and/or XLSX (CSV for templates) attachments. The files are made
// by the same code as the exports of the user interface, run as the report's author with the read and export
// permissions; every delivery is recorded and audited.
//
//	GET/POST /api/v1/scheduled-reports, GET/PUT/DELETE /api/v1/scheduled-reports/{id}   config.write (reason on change)
//	POST /api/v1/scheduled-reports/{id}/send      config.write  send now (a test), recorded like a scheduled run
//	GET  /api/v1/scheduled-reports/{id}/runs      config.write  the delivery log
//	GET  /api/v1/scheduled-reports/options        config.write  what can be chosen in the form
//	POST /api/v1/scheduled-reports/preview-cron   config.write  the next runs of an expression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/counting"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/i18n"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/schedule"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

var emailRe = regexp.MustCompile(`^[^@\s<>,;"']{1,64}@[A-Za-z0-9.-]{1,190}\.[A-Za-z]{2,24}$`)

type reportContent struct {
	Type         string `json:"type"`                    // counting | period_table | template
	AreaID       string `json:"area_id,omitempty"`       // counting
	Dashboard    string `json:"dashboard,omitempty"`     // period_table: dashboard slug
	Widget       string `json:"widget,omitempty"`        // period_table: widget id
	Kind         string `json:"kind,omitempty"`          // template: observations | alarms
	DatastreamID string `json:"datastream_id,omitempty"` // template observations
}

type reportRecipients struct {
	Channels  []string `json:"channels"`
	Addresses []string `json:"addresses"`
}

type scheduledReport struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Content    reportContent    `json:"content"`
	Period     string           `json:"period"`
	Cron       string           `json:"cron"`
	TZ         string           `json:"tz"`
	Recipients reportRecipients `json:"recipients"`
	AttachPDF  bool             `json:"attach_pdf"`
	AttachXLSX bool             `json:"attach_xlsx"`
	Enabled    bool             `json:"enabled"`
	NextRun    *time.Time       `json:"next_run_at"`
	LastRun    *time.Time       `json:"last_run_at"`
	LastStatus string           `json:"last_status"`
	CreatedBy  string           `json:"created_by"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

func (s *Server) loadScheduledReports(ctx context.Context, org, id string) ([]scheduledReport, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text, r.name, r.content::text, r.period, r.cron, r.tz, r.recipients::text, r.attach_pdf, r.attach_xlsx, r.enabled,
		r.next_run_at, r.last_run_at, coalesce(r.last_status, ''), coalesce(u.username, ''), r.updated_at
		FROM core.scheduled_report r LEFT JOIN core.user_account u ON u.id = r.created_by
		WHERE r.org_id = $1::uuid AND r.archived_at IS NULL AND ($2 = '' OR r.id = $2::uuid) ORDER BY r.name`, org, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []scheduledReport{}
	for rows.Next() {
		var r scheduledReport
		var content, rec string
		if err := rows.Scan(&r.ID, &r.Name, &content, &r.Period, &r.Cron, &r.TZ, &rec, &r.AttachPDF, &r.AttachXLSX, &r.Enabled, &r.NextRun, &r.LastRun, &r.LastStatus, &r.CreatedBy, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(content), &r.Content)
		_ = json.Unmarshal([]byte(rec), &r.Recipients)
		if r.Recipients.Channels == nil {
			r.Recipients.Channels = []string{}
		}
		if r.Recipients.Addresses == nil {
			r.Recipients.Addresses = []string{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GET /api/v1/scheduled-reports
func (s *Server) listScheduledReports(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	list, err := s.loadScheduledReports(r.Context(), org, "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"reports": list, "smtp": s.notifier != nil && s.notifier.Available()})
}

// GET /api/v1/scheduled-reports/{id}
func (s *Server) getScheduledReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "report not found")
		return
	}
	list, err := s.loadScheduledReports(r.Context(), org, id)
	if err != nil || len(list) == 0 {
		writeErr(w, 404, "report not found")
		return
	}
	writeJSON(w, 200, list[0])
}

type reportBody struct {
	Name       string           `json:"name"`
	Content    reportContent    `json:"content"`
	Period     string           `json:"period"`
	Cron       string           `json:"cron"`
	Recipients reportRecipients `json:"recipients"`
	AttachPDF  *bool            `json:"attach_pdf"`
	AttachXLSX *bool            `json:"attach_xlsx"`
	Enabled    *bool            `json:"enabled"`
	Reason     string           `json:"reason"`
}

// validReport checks a report against the organization's data; returns the parsed cron.
func (s *Server) validReport(ctx context.Context, org string, in *reportBody) (*schedule.Cron, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		return nil, errors.New("name: 1 to 80 characters")
	}
	if !slices.Contains([]string{"previous_day", "previous_week", "previous_month"}, in.Period) {
		return nil, errors.New("period: previous_day, previous_week or previous_month")
	}
	if p, ok := schedule.Presets[in.Cron]; ok {
		in.Cron = p
	}
	c, err := schedule.Parse(in.Cron)
	if err != nil {
		return nil, err
	}
	in.Cron = c.Expr
	n := 0
	ct := &in.Content
	switch ct.Type {
	case "counting":
		if !uuidRe.MatchString(ct.AreaID) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.counting_area WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, ct.AreaID, org).Scan(&n) != nil || n == 0 {
			return nil, errors.New("content: counting area not found")
		}
		*ct = reportContent{Type: "counting", AreaID: ct.AreaID}
	case "period_table":
		if _, _, err := s.periodTableWidget(ctx, org, ct.Dashboard, ct.Widget); err != nil {
			return nil, err
		}
		*ct = reportContent{Type: "period_table", Dashboard: ct.Dashboard, Widget: ct.Widget}
	case "template":
		switch ct.Kind {
		case "observations":
			if !uuidRe.MatchString(ct.DatastreamID) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, ct.DatastreamID, org).Scan(&n) != nil || n == 0 {
				return nil, errors.New("content: datastream not found")
			}
			*ct = reportContent{Type: "template", Kind: "observations", DatastreamID: ct.DatastreamID}
		case "alarms":
			*ct = reportContent{Type: "template", Kind: "alarms"}
		default:
			return nil, errors.New("content: template kind observations or alarms")
		}
	default:
		return nil, errors.New("content type: counting, period_table or template")
	}
	if len(in.Recipients.Channels)+len(in.Recipients.Addresses) == 0 {
		return nil, errors.New("recipients: an e-mail channel or an address")
	}
	if len(in.Recipients.Channels) > 10 || len(in.Recipients.Addresses) > 50 {
		return nil, errors.New("recipients: at most 10 channels and 50 addresses")
	}
	for _, ch := range in.Recipients.Channels {
		if !uuidRe.MatchString(ch) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND kind = 'email' AND revoked_at IS NULL`, ch, org).Scan(&n) != nil || n == 0 {
			return nil, errors.New("recipients: e-mail channel not found")
		}
	}
	for i, a := range in.Recipients.Addresses {
		a = strings.TrimSpace(a)
		if !emailRe.MatchString(a) {
			return nil, fmt.Errorf("recipients: not an e-mail address: %q", a)
		}
		in.Recipients.Addresses[i] = a
	}
	if in.Recipients.Channels == nil {
		in.Recipients.Channels = []string{}
	}
	if in.Recipients.Addresses == nil {
		in.Recipients.Addresses = []string{}
	}
	pdf, x := in.AttachPDF == nil || *in.AttachPDF, in.AttachXLSX == nil || *in.AttachXLSX
	if !pdf && !x {
		return nil, errors.New("attachments: PDF, XLSX or both")
	}
	return c, nil
}

// periodTableWidget finds a period table widget of a dashboard.
func (s *Server) periodTableWidget(ctx context.Context, org, slug, widget string) (map[string]any, []string, error) {
	var spec []byte
	if err := s.pool.QueryRow(ctx, `SELECT spec FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, slug).Scan(&spec); err != nil {
		return nil, nil, errors.New("content: dashboard not found")
	}
	var d struct {
		Widgets []struct {
			ID       string              `json:"id"`
			Type     string              `json:"type"`
			Bindings map[string][]string `json:"bindings"`
			Params   map[string]any      `json:"params"`
		} `json:"widgets"`
	}
	_ = json.Unmarshal(spec, &d)
	for _, wd := range d.Widgets {
		if wd.ID == widget && wd.Type == "period_table" && len(wd.Bindings["datastreams"]) > 0 {
			if wd.Params == nil {
				wd.Params = map[string]any{}
			}
			return wd.Params, wd.Bindings["datastreams"], nil
		}
	}
	return nil, nil, errors.New("content: the dashboard has no period table widget " + widget)
}

// POST /api/v1/scheduled-reports
func (s *Server) createScheduledReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in reportBody
	if err := decodeJSON(r, &in, 32768); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	c, err := s.validReport(r.Context(), org, &in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	loc := s.orgLoc(r.Context(), org)
	enabled := in.Enabled == nil || *in.Enabled
	var next *time.Time
	if enabled {
		if n := c.Next(time.Now(), loc); !n.IsZero() {
			next = &n
		}
	}
	content, _ := json.Marshal(in.Content)
	rec, _ := json.Marshal(in.Recipients)
	p := PrincipalFrom(r.Context())
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.scheduled_report (org_id, name, content, period, cron, tz, recipients, attach_pdf, attach_xlsx, enabled, next_run_at, created_by)
		VALUES ($1::uuid, $2, $3::jsonb, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, NULLIF($12, '')::uuid) RETURNING id::text`,
		org, in.Name, string(content), in.Period, in.Cron, loc.String(), string(rec), in.AttachPDF == nil || *in.AttachPDF, in.AttachXLSX == nil || *in.AttachXLSX, enabled, next, p.UserID).Scan(&id); err != nil {
		writeErr(w, 500, "save failed")
		return
	}
	s.audit(r, "scheduled_report.create", "scheduled_report", id, nil, in, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "next_run_at": next, "tz": loc.String()})
}

// PUT /api/v1/scheduled-reports/{id}
func (s *Server) updateScheduledReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	old, err := s.loadScheduledReports(r.Context(), org, id)
	if err != nil || len(old) == 0 || !uuidRe.MatchString(id) {
		writeErr(w, 404, "report not found")
		return
	}
	var in reportBody
	if err := decodeJSON(r, &in, 32768); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	c, err := s.validReport(r.Context(), org, &in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	loc := s.orgLoc(r.Context(), org)
	enabled := in.Enabled == nil || *in.Enabled
	var next *time.Time
	if enabled {
		if n := c.Next(time.Now(), loc); !n.IsZero() {
			next = &n
		}
	}
	content, _ := json.Marshal(in.Content)
	rec, _ := json.Marshal(in.Recipients)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.scheduled_report SET name = $3, content = $4::jsonb, period = $5, cron = $6, tz = $7, recipients = $8::jsonb, attach_pdf = $9, attach_xlsx = $10,
		enabled = $11, next_run_at = $12, updated_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`,
		id, org, in.Name, string(content), in.Period, in.Cron, loc.String(), string(rec), in.AttachPDF == nil || *in.AttachPDF, in.AttachXLSX == nil || *in.AttachXLSX, enabled, next); err != nil {
		writeErr(w, 500, "save failed")
		return
	}
	s.audit(r, "scheduled_report.update", "scheduled_report", id, old[0], in, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "next_run_at": next})
}

// DELETE /api/v1/scheduled-reports/{id}?reason=
func (s *Server) deleteScheduledReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id, reason := r.PathValue("id"), r.URL.Query().Get("reason")
	if !reasonOf(w, reason) || !uuidRe.MatchString(id) {
		return
	}
	ct, err := s.pool.Exec(r.Context(), `UPDATE core.scheduled_report SET archived_at = now(), enabled = false, next_run_at = NULL WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, id, org)
	if err != nil || ct.RowsAffected() == 0 {
		writeErr(w, 404, "report not found")
		return
	}
	s.audit(r, "scheduled_report.delete", "scheduled_report", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/v1/scheduled-reports/{id}/runs
func (s *Server) scheduledReportRuns(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "report not found")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, started_at, finished_at, trigger, status, period_from, period_to, recipients, attachments::text, coalesce(error, ''), actor
		FROM core.scheduled_report_run WHERE report_id = $1::uuid AND org_id = $2::uuid ORDER BY started_at DESC LIMIT 100`, id, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var rid, trig, st, att, e, actor string
		var start, fin time.Time
		var pf, pt *time.Time
		var n int
		if rows.Scan(&rid, &start, &fin, &trig, &st, &pf, &pt, &n, &att, &e, &actor) == nil {
			out = append(out, map[string]any{"id": rid, "started_at": start, "finished_at": fin, "trigger": trig, "status": st, "period_from": pf, "period_to": pt,
				"recipients": n, "attachments": json.RawMessage(att), "error": e, "actor": actor})
		}
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/scheduled-reports/preview-cron {cron, count}
func (s *Server) previewCron(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Cron  string `json:"cron"`
		Count int    `json:"count"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if p, ok := schedule.Presets[in.Cron]; ok {
		in.Cron = p
	}
	c, err := schedule.Parse(in.Cron)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Count <= 0 || in.Count > 20 {
		in.Count = 5
	}
	loc := s.orgLoc(r.Context(), org)
	runs := []string{}
	t := time.Now()
	for i := 0; i < in.Count; i++ {
		t = c.Next(t, loc)
		if t.IsZero() {
			break
		}
		runs = append(runs, t.In(loc).Format(time.RFC3339))
	}
	writeJSON(w, 200, map[string]any{"cron": c.Expr, "tz": loc.String(), "next": runs})
}

// GET /api/v1/scheduled-reports/options
func (s *Server) scheduledReportOptions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	areas := []map[string]any{}
	if rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM core.counting_area WHERE org_id = $1::uuid AND archived_at IS NULL ORDER BY name`, org); err == nil {
		for rows.Next() {
			var id, n string
			if rows.Scan(&id, &n) == nil {
				areas = append(areas, map[string]any{"id": id, "name": n})
			}
		}
		rows.Close()
	}
	tables := []map[string]any{}
	if rows, err := s.pool.Query(ctx, `SELECT slug, name, spec FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY name`, org); err == nil {
		for rows.Next() {
			var slug, name string
			var spec []byte
			if rows.Scan(&slug, &name, &spec) != nil {
				continue
			}
			var d struct {
				Widgets []struct {
					ID     string         `json:"id"`
					Type   string         `json:"type"`
					Params map[string]any `json:"params"`
				} `json:"widgets"`
			}
			_ = json.Unmarshal(spec, &d)
			for _, wd := range d.Widgets {
				if wd.Type == "period_table" {
					title, _ := wd.Params["title"].(string)
					tables = append(tables, map[string]any{"dashboard": slug, "dashboard_name": name, "widget": wd.ID, "title": title})
				}
			}
		}
		rows.Close()
	}
	channels := []map[string]any{}
	if rows, err := s.pool.Query(ctx, `SELECT id::text, name, settings FROM core.notification_channel WHERE org_id = $1::uuid AND kind = 'email' AND revoked_at IS NULL ORDER BY name`, org); err == nil {
		for rows.Next() {
			var id, n string
			var st []byte
			if rows.Scan(&id, &n, &st) == nil {
				var cs notify.ChannelSettings
				_ = json.Unmarshal(st, &cs)
				channels = append(channels, map[string]any{"id": id, "name": n, "recipients": len(cs.Recipients)})
			}
		}
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{"areas": areas, "period_tables": tables, "channels": channels, "templates": []string{"observations", "alarms"},
		"presets": schedule.Presets, "periods": []string{"previous_day", "previous_week", "previous_month"}, "tz": s.orgLoc(ctx, org).String(),
		"smtp": s.notifier != nil && s.notifier.Available(), "pdf": s.rep != nil})
}

// ---- running ----------------------------------------------------------------------------------------------

// POST /api/v1/scheduled-reports/{id}/send — runs now for the period before now, records the delivery.
func (s *Server) sendScheduledReport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	list, err := s.loadScheduledReports(r.Context(), org, id)
	if err != nil || len(list) == 0 || !uuidRe.MatchString(id) {
		writeErr(w, 404, "report not found")
		return
	}
	p := PrincipalFrom(r.Context())
	res := s.runScheduledReport(r.Context(), org, list[0], time.Now(), "manual", p.Username)
	s.audit(r, "scheduled_report.send", "scheduled_report", id, nil, res, "")
	writeJSON(w, 200, res)
}

type runResult struct {
	Status      string           `json:"status"`
	Recipients  int              `json:"recipients"`
	Attachments []map[string]any `json:"attachments"`
	Error       string           `json:"error,omitempty"`
	From        time.Time        `json:"period_from"`
	To          time.Time        `json:"period_to"`
}

// scheduledReportsLoop runs the reports that are due, every 30 s.
func (s *Server) scheduledReportsLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		s.runDueReports(context.Background())
	}
}

func (s *Server) runDueReports(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, org_id::text, next_run_at FROM core.scheduled_report WHERE enabled AND archived_at IS NULL AND next_run_at <= now() ORDER BY next_run_at LIMIT 20`)
	if err != nil {
		return
	}
	type due struct {
		id, org string
		at      time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.id, &d.org, &d.at) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		reps, err := s.loadScheduledReports(ctx, d.org, d.id)
		if err != nil || len(reps) == 0 {
			continue
		}
		rp := reps[0]
		loc, err := time.LoadLocation(rp.TZ)
		if err != nil {
			loc = s.orgLoc(ctx, d.org)
		}
		var next *time.Time
		if c, err := schedule.Parse(rp.Cron); err == nil {
			// the next run after now: runs missed while the server was down are not repeated
			if n := c.Next(time.Now(), loc); !n.IsZero() {
				next = &n
			}
		}
		// claim the run (a second server process, e.g. right after a restart, does not send it again)
		ct, err := s.pool.Exec(ctx, `UPDATE core.scheduled_report SET next_run_at = $2 WHERE id = $1::uuid AND next_run_at = $3`, d.id, next, d.at)
		if err != nil || ct.RowsAffected() == 0 {
			continue
		}
		s.runScheduledReport(ctx, d.org, rp, d.at, "schedule", "scheduler")
	}
}

// runScheduledReport makes the files for the period before `at` and mails them; the run is recorded.
func (s *Server) runScheduledReport(ctx context.Context, org string, rp scheduledReport, at time.Time, trigger, actor string) runResult {
	started := time.Now()
	loc, err := time.LoadLocation(rp.TZ)
	if err != nil {
		loc = s.orgLoc(ctx, org)
	}
	res := runResult{Status: "failed", Attachments: []map[string]any{}}
	from, to, err := schedule.Period(rp.Period, at, loc)
	res.From, res.To = from, to
	var files []notify.Attachment
	var summary string
	if err == nil {
		files, summary, err = s.reportFiles(ctx, org, rp, from, to, loc)
	}
	var recipients []string
	if err == nil {
		recipients = s.reportRecipients(ctx, org, rp.Recipients)
		if len(recipients) == 0 {
			err = errors.New("no recipients (the e-mail channels have no addresses)")
		}
	}
	if err == nil && (s.notifier == nil || !s.notifier.Available()) {
		err = errors.New("e-mail (SMTP) is not configured on this server")
	}
	for _, f := range files {
		sum := sha256.Sum256(f.Data)
		res.Attachments = append(res.Attachments, map[string]any{"name": f.Name, "bytes": len(f.Data), "sha256": hex.EncodeToString(sum[:])})
	}
	if err == nil {
		label := periodLabel(rp.Period, from, to)
		subject := rp.Name + " — " + label
		body := fmt.Sprintf("%s\n%s (%s)\n\n%s\n\n%s", rp.Name, label, loc.String(), summary, "Sent automatically by the scheduled report \""+rp.Name+"\". Attachments: "+attachmentNames(files)+".")
		var errs []string
		sent := 0
		for _, to := range recipients {
			c, cancel := context.WithTimeout(ctx, 60*time.Second)
			if e := s.notifier.SendWithAttachments(c, to, subject, body, files); e != nil {
				errs = append(errs, to+": "+e.Error())
			} else {
				sent++
			}
			cancel()
		}
		res.Recipients = sent
		switch {
		case len(errs) == 0:
			res.Status = "sent"
		case sent > 0:
			res.Status, res.Error = "partial", strings.Join(errs, "; ")
		default:
			res.Error = strings.Join(errs, "; ")
		}
	} else {
		res.Error = err.Error()
	}
	att, _ := json.Marshal(res.Attachments)
	if _, e := s.pool.Exec(ctx, `INSERT INTO core.scheduled_report_run (org_id, report_id, started_at, trigger, status, period_from, period_to, recipients, attachments, error, actor)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9::jsonb, NULLIF($10, ''), $11)`, org, rp.ID, started, trigger, res.Status, from, to, res.Recipients, string(att), res.Error, actor); e != nil {
		s.log.Warn("scheduled report run not recorded", "err", e)
	}
	_, _ = s.pool.Exec(ctx, `UPDATE core.scheduled_report SET last_run_at = $2, last_status = $3 WHERE id = $1::uuid`, rp.ID, started, res.Status)
	if res.Status != "sent" {
		s.log.Warn("scheduled report", "report", rp.Name, "status", res.Status, "err", res.Error)
	}
	return res
}

func attachmentNames(files []notify.Attachment) string {
	n := make([]string, len(files))
	for i, f := range files {
		n[i] = f.Name
	}
	return strings.Join(n, ", ")
}

func periodLabel(kind string, from, to time.Time) string {
	switch kind {
	case "previous_day":
		return from.Format("2006-01-02")
	case "previous_month":
		return from.Format("2006-01")
	}
	return from.Format("2006-01-02") + " – " + to.AddDate(0, 0, -1).Format("2006-01-02")
}

// reportRecipients collects the addresses of the e-mail channels and the extra addresses, without duplicates.
func (s *Server) reportRecipients(ctx context.Context, org string, rc reportRecipients) []string {
	var out []string
	add := func(a string) {
		a = strings.TrimSpace(a)
		if a != "" && !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, a) }) {
			out = append(out, a)
		}
	}
	for _, ch := range rc.Channels {
		var st []byte
		if s.pool.QueryRow(ctx, `SELECT settings FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND kind = 'email' AND revoked_at IS NULL`, ch, org).Scan(&st) == nil {
			var cs notify.ChannelSettings
			_ = json.Unmarshal(st, &cs)
			for _, a := range cs.Recipients {
				add(a)
			}
		}
	}
	for _, a := range rc.Addresses {
		add(a)
	}
	return out
}

// reportPrincipal is who a scheduled report runs as: its author's organization with read and export permissions.
func (s *Server) reportPrincipal(ctx context.Context, org string, rp scheduledReport) *iam.Principal {
	name := "scheduled report " + rp.Name
	if rp.CreatedBy != "" {
		name += " (" + rp.CreatedBy + ")"
	}
	p := &iam.Principal{OrgID: org, Username: name, Permissions: map[string]bool{"data.read": true, "data.export": true}}
	_ = s.pool.QueryRow(ctx, `SELECT id::text FROM core.user_account WHERE org_id = $1::uuid AND username = $2`, org, rp.CreatedBy).Scan(&p.UserID)
	return p
}

// internalGet runs a read handler of this server for the report's principal and returns the file it answers.
func (s *Server) internalGet(ctx context.Context, p *iam.Principal, h http.HandlerFunc, path string, q url.Values) ([]byte, string, error) {
	req := httptest.NewRequest(http.MethodGet, path+"?"+q.Encode(), nil).WithContext(context.WithValue(ctx, principalKey, p))
	rec := httptest.NewRecorder()
	h(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	if rec.Code != 200 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return nil, "", fmt.Errorf("%s: %d %s", path, rec.Code, e.Error)
	}
	name := "report"
	if cd := rec.Header().Get("Content-Disposition"); strings.Contains(cd, `filename="`) {
		name = strings.TrimSuffix(cd[strings.Index(cd, `filename="`)+10:], `"`)
	}
	return body, name, nil
}

// reportFiles makes the attachments of a report for a period [from, to).
func (s *Server) reportFiles(ctx context.Context, org string, rp scheduledReport, from, to time.Time, loc *time.Location) ([]notify.Attachment, string, error) {
	p := s.reportPrincipal(ctx, org, rp)
	ct := rp.Content
	var files []notify.Attachment
	want := func(pdf bool) bool { return (pdf && rp.AttachPDF) || (!pdf && rp.AttachXLSX) }
	switch ct.Type {
	case "counting":
		return s.countingReportFiles(ctx, p, org, rp, from, to, loc)
	case "period_table":
		params, ids, err := s.periodTableWidget(ctx, org, ct.Dashboard, ct.Widget)
		if err != nil {
			return nil, "", err
		}
		q := url.Values{}
		q.Set("datastreams", strings.Join(ids, ","))
		q.Set("period", "custom")
		q.Set("from", from.Format("2006-01-02"))
		q.Set("to", to.AddDate(0, 0, -1).Format("2006-01-02"))
		for _, k := range []string{"interval", "values", "orientation", "low", "high", "decimals", "title"} {
			if v, ok := params[k]; ok && fmt.Sprint(v) != "" {
				q.Set(k, fmt.Sprint(v))
			}
		}
		if q.Get("title") == "" {
			q.Set("title", rp.Name)
		}
		if b, _ := params["total"].(bool); b {
			q.Set("total", "1")
		}
		for _, f := range []struct {
			pdf    bool
			format string
			ct     string
		}{{false, "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}, {true, "pdf", "application/pdf"}} {
			if !want(f.pdf) {
				continue
			}
			q.Set("format", f.format)
			b, name, err := s.internalGet(ctx, p, s.periodTableExport, "/api/v1/period-table/export", q)
			if err != nil {
				return nil, "", err
			}
			files = append(files, notify.Attachment{Name: name, ContentType: f.ct, Data: b})
		}
		return files, fmt.Sprintf("Period table %q of the dashboard %s.", q.Get("title"), ct.Dashboard), nil
	case "template":
		q := url.Values{}
		q.Set("from", from.UTC().Format(time.RFC3339))
		q.Set("to", to.UTC().Format(time.RFC3339))
		h, path := s.exportAlarms, "/api/v1/export/alarms"
		if ct.Kind == "observations" {
			h, path = s.exportObservations, "/api/v1/export/observations"
			q.Set("datastream_id", ct.DatastreamID)
		}
		for _, f := range []struct {
			pdf    bool
			format string
			ct     string
		}{{false, "csv", "text/csv"}, {true, "pdf", "application/pdf"}} {
			if !want(f.pdf) {
				continue
			}
			q.Set("format", f.format)
			b, name, err := s.internalGet(ctx, p, h, path, q)
			if err != nil {
				return nil, "", err
			}
			files = append(files, notify.Attachment{Name: name, ContentType: f.ct, Data: b})
		}
		return files, "Report from the template " + ct.Kind + ".", nil
	}
	return nil, "", errors.New("unknown content")
}

// countingReportFiles: a row per hour (previous day) or per day (week, month) with in and out of every group, the peak
// occupancy, then the entrances with their totals and shares, and the speed sections.
func (s *Server) countingReportFiles(ctx context.Context, p *iam.Principal, org string, rp scheduledReport, from, to time.Time, loc *time.Location) ([]notify.Attachment, string, error) {
	areas, err := s.countingAreas(ctx, org, rp.Content.AreaID)
	if err != nil || len(areas) == 0 {
		return nil, "", errors.New("counting area not found")
	}
	a := areas[0]
	lang := "en"
	_ = s.pool.QueryRow(ctx, `SELECT coalesce(locale, 'en') FROM core.user_account WHERE id = NULLIF($1, '')::uuid`, p.UserID).Scan(&lang)
	if !i18n.IsSupported(lang) {
		lang = "en"
	}
	cat := i18n.Builtin(lang)
	tr := func(k string) string {
		if v := cat[k]; v != "" {
			return v
		}
		return k
	}
	bucket, bfmt := "day", "2006-01-02"
	if rp.Period == "previous_day" {
		bucket, bfmt = "hour", "2006-01-02 15:00"
	}
	// area datastreams and entrance datastreams
	type col struct {
		id, label, key string
	}
	var cols []col
	rows, err := s.pool.Query(ctx, `SELECT id::text, key FROM core.datastream WHERE asset_id = $1::uuid ORDER BY key`, a.AssetID)
	if err != nil {
		return nil, "", err
	}
	byKey := map[string]string{}
	for rows.Next() {
		var id, k string
		if rows.Scan(&id, &k) == nil {
			byKey[k] = id
		}
	}
	rows.Close()
	header := []string{tr("Period")}
	for _, g := range a.Groups {
		for _, what := range []string{"in", "out", "occupancy"} {
			if id := byKey[counting.Key(g, what)]; id != "" {
				label := tr(g) + " " + tr(what)
				if what == "occupancy" {
					label = tr(g) + " " + tr("peak occupancy")
				}
				cols = append(cols, col{id, label, counting.Key(g, what)})
				header = append(header, label)
			}
		}
	}
	ids := make([]string, len(cols))
	for i, c := range cols {
		ids[i] = c.id
	}
	vals := map[string]map[string]float64{} // bucket label → datastream → value
	var order []string
	for t := from; t.Before(to); {
		l := t.Format(bfmt)
		vals[l] = map[string]float64{}
		order = append(order, l)
		if bucket == "hour" {
			t = t.Add(time.Hour)
		} else {
			t = t.AddDate(0, 0, 1)
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, date_trunc($4, measured_at AT TIME ZONE $5), coalesce(sum(value), 0)::float8, max(value)::float8
		FROM core.observation WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3 GROUP BY 1, 2`, ids, from, to, bucket, loc.String())
	if err != nil {
		return nil, "", err
	}
	isOcc := map[string]bool{}
	for _, c := range cols {
		isOcc[c.id] = strings.HasSuffix(c.key, "_occupancy")
	}
	for rows.Next() {
		var id string
		var b time.Time
		var sum, mx float64
		if rows.Scan(&id, &b, &sum, &mx) != nil {
			continue
		}
		l := time.Date(b.Year(), b.Month(), b.Day(), b.Hour(), 0, 0, 0, loc).Format(bfmt)
		if m, ok := vals[l]; ok {
			if isOcc[id] {
				m[id] = mx
			} else {
				m[id] = sum
			}
		}
	}
	rows.Close()
	totals := make([]float64, len(cols))
	var grid [][]xlsx.Cell
	for _, l := range order {
		row := []xlsx.Cell{xlsx.T(l)}
		for i, c := range cols {
			v := vals[l][c.id]
			if isOcc[c.id] {
				totals[i] = max(totals[i], v)
			} else {
				totals[i] += v
			}
			row = append(row, xlsx.N(v, 0))
		}
		grid = append(grid, row)
	}
	tot := []xlsx.Cell{xlsx.B(tr("Total"))}
	for _, v := range totals {
		c := xlsx.N(v, 0)
		c.Bold = true
		tot = append(tot, c)
	}
	grid = append(grid, tot)
	// entrances of the period
	summ, _ := s.countingPeriodEntrances(ctx, a, from, to)
	var lines []string
	for i, c := range cols {
		lines = append(lines, fmt.Sprintf("%s: %.0f", c.label, totals[i]))
	}
	grid = append(grid, nil, []xlsx.Cell{xlsx.B(tr("Entrance"))})
	eh := []xlsx.Cell{xlsx.B(tr("Entrance"))}
	for _, g := range a.Groups {
		eh = append(eh, xlsx.B(tr(g)+" "+tr("in")), xlsx.B(tr(g)+" "+tr("out")), xlsx.B(tr(g)+" "+tr("share")+" %"))
	}
	grid[len(grid)-1] = eh
	for _, e := range summ {
		row := []xlsx.Cell{xlsx.T(e.name)}
		for _, g := range a.Groups {
			row = append(row, xlsx.N(e.counts[g].In, 0), xlsx.N(e.counts[g].Out, 0), xlsx.N(e.share[g], 1))
		}
		grid = append(grid, row)
		lines = append(lines, fmt.Sprintf("%s: %s", e.name, e.text(a.Groups)))
	}
	for _, sp := range s.countingSpeeds(ctx, org, a.ID, from, to) {
		l := fmt.Sprintf("%s (%s): %d %s", sp.Name, sp.Camera, sp.Count, tr("vehicles"))
		if sp.Avg != nil {
			l += fmt.Sprintf(", %s %.1f km/h, %s %.1f km/h", tr("average"), *sp.Avg, tr("max"), *sp.Max)
		}
		if sp.LimitKmh > 0 {
			l += fmt.Sprintf(", %d %s %.0f km/h", sp.Over, tr("over the limit of"), sp.LimitKmh)
		}
		lines = append(lines, l)
		grid = append(grid, []xlsx.Cell{xlsx.T(l)})
	}
	title := rp.Name
	subtitle := fmt.Sprintf("%s · %s · %s", a.Name, periodLabel(rp.Period, from, to), loc.String())
	stamp := time.Now().In(loc).Format("20060102-1504")
	base := fileSafe(title) + "_" + fileSafe(periodLabel(rp.Period, from, to))
	var files []notify.Attachment
	if rp.AttachXLSX {
		sheet := xlsx.Sheet{Name: title, FreezeRows: 5}
		sheet.Rows = append(sheet.Rows, []xlsx.Cell{xlsx.B(title)}, []xlsx.Cell{xlsx.T(subtitle)}, []xlsx.Cell{xlsx.T(s.orgNameCtx(ctx, org) + " · " + time.Now().In(loc).Format("2006-01-02 15:04"))}, nil)
		hdr := make([]xlsx.Cell, len(header))
		for i, h := range header {
			hdr[i] = xlsx.B(h)
			sheet.Widths = append(sheet.Widths, max(12, min(float64(len([]rune(h)))+2, 30)))
		}
		sheet.Widths[0] = 20
		sheet.Rows = append(sheet.Rows, hdr)
		sheet.Rows = append(sheet.Rows, grid...)
		b, err := xlsx.Bytes(sheet)
		if err != nil {
			return nil, "", err
		}
		files = append(files, notify.Attachment{Name: base + "_" + stamp + ".xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: b})
	}
	if rp.AttachPDF {
		if s.rep == nil {
			return nil, "", errors.New("PDF reports unavailable: typst is not installed on this server")
		}
		repCols := make([]report.Column, len(header))
		for i, h := range header {
			al := "right"
			if i == 0 {
				al = "left"
			}
			repCols[i] = report.Column{Key: fmt.Sprintf("c%d", i), Label: h, Align: al}
		}
		data := []map[string]any{}
		for _, row := range grid {
			if len(row) == 0 {
				continue
			}
			m := map[string]any{}
			for i, c := range row {
				m[fmt.Sprintf("c%d", i)] = ptCellString(c)
			}
			data = append(data, m)
		}
		d := report.Data{Kind: "generic", Title: title, Subtitle: subtitle, Org: s.orgNameCtx(ctx, org), Locale: lang,
			Period:  report.Period{From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339)},
			Scope:   map[string]any{"area": a.Name, "groups": a.Groups, "tz": loc.String(), "scheduled_report": rp.Name},
			Columns: repCols, Rows: data, Summary: map[string]any{"rows": len(order)}, System: map[string]any{"version": s.version, "instance": s.cfg.HTTP.BaseURL}}
		req := httptest.NewRequest(http.MethodGet, "/scheduled-report", nil).WithContext(context.WithValue(ctx, principalKey, p))
		res, err := s.renderReport(req, p, d, nil)
		if err != nil {
			return nil, "", err
		}
		files = append(files, notify.Attachment{Name: base + "_" + stamp + ".pdf", ContentType: "application/pdf", Data: res.PDF})
	}
	return files, strings.Join(lines, "\n"), nil
}

type entranceTotals struct {
	name   string
	counts map[string]inOut
	share  map[string]float64 // share of the area's passages (in + out)
}

func (e entranceTotals) text(groups []string) string {
	var parts []string
	for _, g := range groups {
		c := e.counts[g]
		parts = append(parts, fmt.Sprintf("%s %.0f in / %.0f out (%.1f %%)", g, c.In, c.Out, e.share[g]))
	}
	return strings.Join(parts, ", ")
}

// countingPeriodEntrances sums the passages of every entrance of an area over a period.
func (s *Server) countingPeriodEntrances(ctx context.Context, a countingAreaJSON, from, to time.Time) ([]entranceTotals, error) {
	out := make([]entranceTotals, len(a.Entrances))
	idx := map[string]int{}
	assets := []string{}
	for i, e := range a.Entrances {
		out[i] = entranceTotals{name: e.Name, counts: map[string]inOut{}, share: map[string]float64{}}
		idx[e.AssetID] = i
		assets = append(assets, e.AssetID)
	}
	rows, err := s.pool.Query(ctx, `SELECT ds.asset_id::text, ds.key, coalesce(sum(o.value), 0)::float8 FROM core.datastream ds
		JOIN core.observation o ON o.datastream_id = ds.id AND o.measured_at >= $2 AND o.measured_at < $3
		WHERE ds.asset_id = ANY($1::uuid[]) GROUP BY 1, 2`, assets, from, to)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var as, key string
		var v float64
		if rows.Scan(&as, &key, &v) != nil {
			continue
		}
		g, dir := splitKey(key)
		i, ok := idx[as]
		if !ok {
			continue
		}
		c := out[i].counts[g]
		if dir == "in" {
			c.In = v
		} else if dir == "out" {
			c.Out = v
		}
		out[i].counts[g] = c
	}
	rows.Close()
	for _, g := range a.Groups {
		var total float64
		for _, e := range out {
			total += e.counts[g].In + e.counts[g].Out
		}
		for i := range out {
			out[i].share[g] = counting.Share(out[i].counts[g].In+out[i].counts[g].Out, total)
		}
	}
	return out, nil
}

func (s *Server) orgNameCtx(ctx context.Context, org string) string {
	var n string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM core.organization WHERE id = $1::uuid`, org).Scan(&n); err != nil {
		return "ctrl32 telemetry"
	}
	return n
}
