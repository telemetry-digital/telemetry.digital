package httpapi

// Visual automation flows (spec 004): versioned content, validation against the closed node library, deploy with
// reason (four eyes for flows that send commands), versions and restore, export/import, test injection, dry run,
// debug and status; and the server side of the flow host (commands, derived values, notifications, incidents, HTTP
// to allow-listed hosts). The engine lives in internal/flows.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/flows"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/incident"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/smarthome"
)

// ---- references for validation ------------------------------------------------------------------------------------

type flowRefs struct {
	s   *Server
	ctx context.Context
	org string
}

func (f flowRefs) exists(sql, id string) bool {
	var ok bool
	_ = f.s.pool.QueryRow(f.ctx, sql, id, f.org).Scan(&ok)
	return ok
}
func (f flowRefs) Datastream(id string) bool {
	return f.exists(`SELECT EXISTS (SELECT 1 FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id JOIN core.site si ON si.id = a.site_id WHERE d.id = $1::uuid AND si.org_id = $2::uuid)`, id)
}
func (f flowRefs) Device(id string) bool {
	return f.exists(`SELECT EXISTS (SELECT 1 FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, id)
}
func (f flowRefs) Channel(id string) bool {
	return f.exists(`SELECT EXISTS (SELECT 1 FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, id)
}
func (f flowRefs) Entity(id string) bool {
	return f.exists(`SELECT EXISTS (SELECT 1 FROM core.entity WHERE id = $1::uuid AND org_id = $2::uuid AND status <> 'removed')`, id)
}
func (f flowRefs) MQTTSource(id string) bool {
	return f.exists(`SELECT EXISTS (SELECT 1 FROM core.connector WHERE id = $1::uuid AND org_id = $2::uuid AND kind IN ('mqtt_account', 'mqtt_bridge') AND revoked_at IS NULL)`, id)
}
func (f flowRefs) HostAllowed(host string) bool {
	for _, h := range f.s.flowHosts(f.ctx, f.org) {
		if strings.EqualFold(h, host) || (strings.HasPrefix(h, "*.") && strings.HasSuffix(strings.ToLower(host), strings.ToLower(h[1:]))) {
			return true
		}
	}
	return false
}

// flowHosts is the organization's allow-list for HTTP request nodes.
func (s *Server) flowHosts(ctx context.Context, org string) []string {
	var raw []byte
	_ = s.pool.QueryRow(ctx, `SELECT settings->'flows'->'allowed_hosts' FROM core.organization WHERE id = $1::uuid`, org).Scan(&raw)
	var hosts []string
	_ = json.Unmarshal(raw, &hosts)
	return hosts
}

// ---- flow rows ------------------------------------------------------------------------------------------------------

type flowRow struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Version     int             `json:"version"`
	State       string          `json:"state"`
	Spec        json.RawMessage `json:"spec,omitempty"`
	Checksum    string          `json:"checksum"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	Reason      string          `json:"reason"`
	Deployed    *int            `json:"deployed_version"`
	Nodes       int             `json:"nodes"`
	HasCommands bool            `json:"has_commands"`
}

const flowCols = `f.id::text, f.slug, f.name, f.version, f.state, f.spec, f.checksum, coalesce(u.username, ''), f.created_at, f.reason,
	(SELECT d.version FROM core.flow d WHERE d.org_id = f.org_id AND d.slug = f.slug AND d.state = 'deployed' ORDER BY d.version DESC LIMIT 1)`

func scanFlow(row pgx.Row, withSpec bool) (*flowRow, error) {
	var f flowRow
	var spec []byte
	if err := row.Scan(&f.ID, &f.Slug, &f.Name, &f.Version, &f.State, &spec, &f.Checksum, &f.CreatedBy, &f.CreatedAt, &f.Reason, &f.Deployed); err != nil {
		return nil, err
	}
	var sp flows.Spec
	_ = json.Unmarshal(spec, &sp)
	f.Nodes, f.HasCommands = len(sp.Nodes), flows.HasCommands(&sp)
	if withSpec {
		f.Spec = spec
	}
	return &f, nil
}

func (s *Server) currentFlow(ctx context.Context, org, slug string) (*flowRow, error) {
	return scanFlow(s.pool.QueryRow(ctx, `SELECT `+flowCols+` FROM core.flow f LEFT JOIN core.user_account u ON u.id = f.created_by
		WHERE f.org_id = $1::uuid AND f.slug = $2 AND f.valid_to IS NULL`, org, slug), true)
}

func canonicalSpec(sp *flows.Spec) ([]byte, string) {
	if sp.Nodes == nil {
		sp.Nodes = []flows.NodeSpec{}
	}
	if sp.Wires == nil {
		sp.Wires = []flows.Wire{}
	}
	b, _ := json.Marshal(sp)
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:])
}

// GET /api/v1/flows/catalog
func (s *Server) flowCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"nodes": flows.Catalog, "functions": flows.FuncNames(), "limits": map[string]int{"nodes": flows.MaxNodes, "debug": flows.DebugEntries, "queue": flows.QueueSize}})
}

// GET /api/v1/flows
func (s *Server) listFlows(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+flowCols+` FROM core.flow f LEFT JOIN core.user_account u ON u.id = f.created_by
		WHERE f.org_id = $1::uuid AND f.valid_to IS NULL ORDER BY f.name`, org)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		f, err := scanFlow(rows, false)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		st := s.flows.Status(org, f.Slug)
		var errs int64
		for _, n := range st.Nodes {
			errs += n.Errors
		}
		out = append(out, map[string]any{"flow": f, "running": st.Running, "errors": errs, "dropped": st.Dropped})
	}
	writeJSON(w, 200, out)
}

// saveFlowVersion stores a new version (draft) and returns it with the validation problems.
func (s *Server) saveFlowVersion(ctx context.Context, r *http.Request, org, slug, name string, sp *flows.Spec, reason string, isNew bool) (*flowRow, []flows.Problem, error) {
	problems := flows.Validate(sp, flowRefs{s, ctx, org})
	body, sum := canonicalSpec(sp)
	p := PrincipalFrom(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	version := 1
	var oldName string
	if !isNew {
		if err := tx.QueryRow(ctx, `UPDATE core.flow SET valid_to = now() WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL RETURNING version, name`, org, slug).Scan(&version, &oldName); err != nil {
			return nil, nil, errNotFound
		}
		version++
		if name == "" {
			name = oldName
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.flow (org_id, slug, name, version, state, spec, checksum, created_by, reason)
		VALUES ($1::uuid, $2, $3, $4, 'draft', $5::jsonb, $6, NULLIF($7, '')::uuid, $8) RETURNING id::text`,
		org, slug, name, version, string(body), sum, p.UserID, reason).Scan(&id); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	action := "flow.update"
	if isNew {
		action = "flow.create"
	}
	s.audit(r, action, "flow", id, nil, map[string]any{"slug": slug, "name": name, "version": version, "checksum": sum, "nodes": len(sp.Nodes), "problems": len(problems)}, reason)
	f, err := s.currentFlow(ctx, org, slug)
	return f, problems, err
}

var errNotFound = errors.New("not found")

func decodeSpec(raw json.RawMessage) (*flows.Spec, error) {
	sp := &flows.Spec{}
	if len(raw) == 0 || string(raw) == "null" {
		return sp, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(sp); err != nil {
		return nil, fmt.Errorf("spec: %w", err)
	}
	return sp, nil
}

// POST /api/v1/flows {name, spec, reason}
func (s *Server) createFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name, Slug, Reason string
		Spec               json.RawMessage
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 {
		writeErr(w, 400, "name must be 1–128 characters")
		return
	}
	sp, err := decodeSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Name)
	}
	var exists bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.flow WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL)`, org, slug).Scan(&exists)
	if exists {
		writeErr(w, 409, "a flow with this name already exists")
		return
	}
	f, problems, err := s.saveFlowVersion(r.Context(), r, org, slug, in.Name, sp, in.Reason, true)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"flow": f, "problems": nonNil(problems)})
}

func nonNil(p []flows.Problem) []flows.Problem {
	if p == nil {
		return []flows.Problem{}
	}
	return p
}

// GET /api/v1/flows/{slug}
func (s *Server) getFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	f, err := s.currentFlow(r.Context(), org, slug)
	if err != nil {
		writeErr(w, 404, "flow not found")
		return
	}
	var sp flows.Spec
	_ = json.Unmarshal(f.Spec, &sp)
	problems := flows.Validate(&sp, flowRefs{s, r.Context(), org})
	hooks := map[string]string{}
	for _, n := range sp.Nodes {
		if n.Type == "webhook" {
			hooks[n.ID] = strings.TrimRight(s.cfg.HTTP.BaseURL, "/") + "/api/v1/flow-hooks/" + s.flows.HookToken(org, slug, n.ID)
		}
	}
	writeJSON(w, 200, map[string]any{"flow": f, "problems": nonNil(problems), "status": s.flows.Status(org, slug), "hooks": hooks})
}

// PUT /api/v1/flows/{slug} {name?, spec, reason}
func (s *Server) updateFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name, Reason string
		Spec         json.RawMessage
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	sp, err := decodeSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	f, problems, err := s.saveFlowVersion(r.Context(), r, org, r.PathValue("slug"), strings.TrimSpace(in.Name), sp, in.Reason, false)
	if errors.Is(err, errNotFound) {
		writeErr(w, 404, "flow not found")
		return
	} else if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"flow": f, "problems": nonNil(problems)})
}

// POST /api/v1/flows/validate {spec}
func (s *Server) validateFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Spec json.RawMessage }
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sp, err := decodeSpec(in.Spec)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "problems": []flows.Problem{{Message: err.Error()}}})
		return
	}
	p := flows.Validate(sp, flowRefs{s, r.Context(), org})
	writeJSON(w, 200, map[string]any{"ok": len(p) == 0, "problems": nonNil(p), "has_commands": flows.HasCommands(sp)})
}

// POST /api/v1/flows/{slug}/deploy {reason} — four eyes when the flow sends commands and the policy asks for it.
func (s *Server) deployFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	slug := r.PathValue("slug")
	f, err := s.currentFlow(r.Context(), org, slug)
	if err != nil {
		writeErr(w, 404, "flow not found")
		return
	}
	var sp flows.Spec
	_ = json.Unmarshal(f.Spec, &sp)
	if p := flows.Validate(&sp, flowRefs{s, r.Context(), org}); len(p) > 0 {
		writeJSON(w, 422, map[string]any{"error": "the flow has problems and cannot be deployed", "problems": p})
		return
	}
	if flows.HasCommands(&sp) {
		if pr := PrincipalFrom(r.Context()); pr == nil || !pr.Has("device.command") {
			writeErr(w, 403, "deploying a flow that sends commands needs the device.command permission")
			return
		}
	}
	var n int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.flow WHERE org_id = $1::uuid AND state = 'deployed' AND slug <> $2`, org, slug).Scan(&n)
	if n >= flows.MaxDeployed {
		writeErr(w, 409, fmt.Sprintf("at most %d deployed flows per organization", flows.MaxDeployed))
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE core.flow SET state = 'draft' WHERE org_id = $1::uuid AND slug = $2 AND state IN ('deployed', 'disabled') AND id <> $3::uuid`, org, slug, f.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE core.flow SET state = 'deployed' WHERE id = $1::uuid`, f.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit(r, "flow.deploy", "flow", f.ID, map[string]any{"deployed_version": f.Deployed}, map[string]any{"slug": slug, "version": f.Version, "checksum": f.Checksum, "sends_commands": f.HasCommands}, in.Reason)
	if err := s.flows.Reload(r.Context(), org, slug); err != nil {
		writeErr(w, 500, "deployed, but the flow could not start: "+err.Error())
		return
	}
	nf, _ := s.currentFlow(r.Context(), org, slug)
	writeJSON(w, 200, map[string]any{"flow": nf, "status": s.flows.Status(org, slug)})
}

// flowNeedsFourEyes: a deploy of a flow that sends commands, when the organization requires four eyes for commands.
func (s *Server) flowNeedsFourEyes(r *http.Request, org string) bool {
	if !strings.HasSuffix(r.URL.Path, "/deploy") {
		return false
	}
	f, err := s.currentFlow(r.Context(), org, r.PathValue("slug"))
	return err == nil && f.HasCommands
}

// POST /api/v1/flows/{slug}/disable {reason}
func (s *Server) disableFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	slug := r.PathValue("slug")
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.flow SET state = 'disabled' WHERE org_id = $1::uuid AND slug = $2 AND state = 'deployed'`, org, slug)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 409, "the flow is not deployed")
		return
	}
	s.flows.Stop(org, slug)
	s.audit(r, "flow.disable", "flow", "", nil, map[string]any{"slug": slug}, in.Reason)
	f, _ := s.currentFlow(r.Context(), org, slug)
	writeJSON(w, 200, map[string]any{"flow": f})
}

// GET /api/v1/flows/{slug}/versions
func (s *Server) flowVersions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+flowCols+` FROM core.flow f LEFT JOIN core.user_account u ON u.id = f.created_by
		WHERE f.org_id = $1::uuid AND f.slug = $2 ORDER BY f.version DESC LIMIT 200`, org, r.PathValue("slug"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []*flowRow{}
	for rows.Next() {
		f, err := scanFlow(rows, false)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, f)
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/flows/{slug}/restore {version, reason} → a new version with the old content (deploy separately).
func (s *Server) restoreFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Version int
		Reason  string
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	slug := r.PathValue("slug")
	var raw []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT spec FROM core.flow WHERE org_id = $1::uuid AND slug = $2 AND version = $3`, org, slug, in.Version).Scan(&raw); err != nil {
		writeErr(w, 404, "version not found")
		return
	}
	sp, err := decodeSpec(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	f, problems, err := s.saveFlowVersion(r.Context(), r, org, slug, "", sp, fmt.Sprintf("restore version %d: %s", in.Version, in.Reason), false)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"flow": f, "problems": nonNil(problems)})
}

// DELETE /api/v1/flows/{slug}?reason=
func (s *Server) deleteFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	slug := r.PathValue("slug")
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.flow SET valid_to = now(), state = CASE WHEN state = 'deployed' THEN 'disabled' ELSE state END
		WHERE org_id = $1::uuid AND slug = $2 AND (valid_to IS NULL OR state = 'deployed')`, org, slug)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "flow not found")
		return
	}
	s.flows.Stop(org, slug)
	_, _ = s.pool.Exec(r.Context(), `DELETE FROM core.flow_state WHERE org_id = $1::uuid AND flow_slug = $2`, org, slug)
	s.audit(r, "flow.delete", "flow", "", nil, map[string]any{"slug": slug}, reason)
	writeJSON(w, 200, map[string]any{"deleted": slug})
}

// POST /api/v1/flows/{slug}/inject {node, value, topic, dry_run, spec?}
func (s *Server) injectFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Node   string          `json:"node"`
		Value  any             `json:"value"`
		Topic  string          `json:"topic"`
		DryRun bool            `json:"dry_run"`
		Spec   json.RawMessage `json:"spec"`
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	slug := r.PathValue("slug")
	f, err := s.currentFlow(r.Context(), org, slug)
	if err != nil {
		writeErr(w, 404, "flow not found")
		return
	}
	var sp *flows.Spec
	if len(in.Spec) > 0 && string(in.Spec) != "null" {
		if sp, err = decodeSpec(in.Spec); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if p := flows.Validate(sp, flowRefs{s, r.Context(), org}); len(p) > 0 {
			writeJSON(w, 422, map[string]any{"error": "the edited flow has problems", "problems": p})
			return
		}
	}
	if sp == nil && !in.DryRun {
		// a real test message can make a deployed flow act: only for those allowed to change flows
		if pr := PrincipalFrom(r.Context()); f.HasCommands && (pr == nil || !pr.Has("device.command")) {
			writeErr(w, 403, "a test message that may send commands needs the device.command permission (or use dry run)")
			return
		}
	}
	var value flows.Value
	switch v := in.Value.(type) {
	case nil, float64, string, bool:
		value = v
	default:
		writeErr(w, 400, "value must be a number, text, true/false or null")
		return
	}
	if err := s.flows.Inject(r.Context(), org, slug, in.Node, value, in.Topic, in.DryRun, sp, f.Name); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if !in.DryRun && sp == nil {
		s.audit(r, "flow.inject", "flow", f.ID, nil, map[string]any{"slug": slug, "node": in.Node, "value": in.Value}, "test message")
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

// GET /api/v1/flows/{slug}/debug?after=
func (s *Server) flowDebug(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	writeJSON(w, 200, s.flows.Debug(org, r.PathValue("slug"), after))
}

// GET /api/v1/flows/{slug}/status
func (s *Server) flowStatus(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.flows.Status(org, r.PathValue("slug")))
}

// GET /api/v1/flows/{slug}/export
func (s *Server) exportFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	f, err := s.currentFlow(r.Context(), org, r.PathValue("slug"))
	if err != nil {
		writeErr(w, 404, "flow not found")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="flow-`+f.Slug+`.json"`)
	writeJSON(w, 200, map[string]any{"format": "ctrl32-flow/1", "name": f.Name, "slug": f.Slug, "version": f.Version, "checksum": f.Checksum, "spec": f.Spec})
}

// POST /api/v1/flows/import {json, reason}
func (s *Server) importFlow(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Format, Name, Slug, Reason, Checksum string
		Version                              int
		Spec                                 json.RawMessage
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Format != "ctrl32-flow/1" {
		writeErr(w, 400, "not a ctrl32 flow export (format ctrl32-flow/1)")
		return
	}
	b, _ := json.Marshal(map[string]any{"name": in.Name, "slug": in.Slug, "spec": in.Spec, "reason": in.Reason})
	r.Body = io.NopCloser(bytes.NewReader(b))
	s.createFlow(w, r)
}

// GET/PUT /api/v1/flows/settings {allowed_hosts, reason}
func (s *Server) flowSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"allowed_hosts": nonNilStrings(s.flowHosts(r.Context(), org))})
		return
	}
	var in struct {
		AllowedHosts []string `json:"allowed_hosts"`
		Reason       string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	hosts := []string{}
	for _, h := range in.AllowedHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if len(h) > 253 || strings.ContainsAny(h, "/:@ ") {
			writeErr(w, 400, "host names only (example.com or *.example.com): "+h)
			return
		}
		hosts = append(hosts, h)
	}
	if len(hosts) > 100 {
		writeErr(w, 400, "at most 100 hosts")
		return
	}
	old := s.flowHosts(r.Context(), org)
	b, _ := json.Marshal(map[string]any{"allowed_hosts": hosts})
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{flows}', coalesce(settings->'flows', '{}'::jsonb) || $2::jsonb) WHERE id = $1::uuid`, org, string(b)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.audit(r, "flow_settings.update", "organization", org, map[string]any{"allowed_hosts": old}, map[string]any{"allowed_hosts": hosts}, in.Reason)
	writeJSON(w, 200, map[string]any{"allowed_hosts": hosts})
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// POST /api/v1/flow-hooks/{token} — incoming webhook (public; optional HMAC-SHA256 of the body in X-Signature).
func (s *Server) flowHook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeErr(w, 413, "body too large")
		return
	}
	var in map[string]any
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			writeErr(w, 400, "JSON object expected")
			return
		}
	}
	sig := strings.TrimPrefix(r.Header.Get("X-Signature"), "sha256=")
	verify := func(secret string) bool {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(want), []byte(strings.ToLower(sig)))
	}
	if err := s.flows.Webhook(r.PathValue("token"), in, verify); err != nil {
		if errors.Is(err, flows.ErrUnknownHook) {
			writeErr(w, 404, "unknown webhook")
			return
		}
		s.log.Warn("flow webhook refused", "security", true, "ip", clientIP(r), "err", err)
		writeErr(w, 403, err.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"accepted": true})
}

// ---- the server as the flows' host (flows.Host) ---------------------------------------------------------------------

type flowHost struct {
	s     *Server
	mu    sync.Mutex
	names map[string][2]string
	devs  map[string]string
	locs  map[string]*time.Location
	at    time.Time
}

func newFlowHost(s *Server) *flowHost {
	return &flowHost{s: s, names: map[string][2]string{}, devs: map[string]string{}, locs: map[string]*time.Location{}, at: time.Now()}
}

func (h *flowHost) reset() {
	if time.Since(h.at) > 5*time.Minute {
		h.names, h.devs, h.locs, h.at = map[string][2]string{}, map[string]string{}, map[string]*time.Location{}, time.Now()
	}
}

func (h *flowHost) DatastreamName(ctx context.Context, org, id string) (string, string) {
	h.mu.Lock()
	h.reset()
	v, ok := h.names[id]
	h.mu.Unlock()
	if ok {
		return v[0], v[1]
	}
	var asset, key string
	if err := h.s.pool.QueryRow(ctx, `SELECT a.name, d.key FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id WHERE d.id = $1::uuid`, id).Scan(&asset, &key); err != nil {
		return id, ""
	}
	h.mu.Lock()
	h.names[id] = [2]string{asset + " / " + key, key}
	h.mu.Unlock()
	return asset + " / " + key, key
}

func (h *flowHost) DeviceName(ctx context.Context, org, id string) string {
	h.mu.Lock()
	h.reset()
	v, ok := h.devs[id]
	h.mu.Unlock()
	if ok {
		return v
	}
	var name, ext string
	if err := h.s.pool.QueryRow(ctx, `SELECT coalesce(nullif(name, ''), external_id), external_id FROM core.device WHERE id = $1::uuid`, id).Scan(&name, &ext); err != nil {
		return id
	}
	h.mu.Lock()
	h.devs[id] = name
	h.mu.Unlock()
	return name
}

func (h *flowHost) StoreDerived(ctx context.Context, org, ds, key string, v float64, at time.Time) error {
	if h.s.ing == nil {
		return errors.New("ingest not available")
	}
	return h.s.ing.StoreDerived(ctx, org, ds, key, v, at)
}

func (h *flowHost) RunCommand(ctx context.Context, org, dev, method string, args json.RawMessage, flowID, issuer, reason string) (string, string, string, error) {
	if method == "" {
		method = "write"
	}
	res, err := h.s.RunCommand(ctx, rules.CommandRequest{OrgID: org, DeviceID: dev, Method: method, Args: args, FlowID: flowID, Issuer: issuer, Reason: reason})
	if err != nil {
		return "", "", "", err
	}
	return res.ID, res.Status, res.Error, nil
}

func (h *flowHost) EntityName(ctx context.Context, org, id string) string {
	var name string
	if err := h.s.pool.QueryRow(ctx, `SELECT name FROM core.entity WHERE id = $1::uuid`, id).Scan(&name); err != nil {
		return id
	}
	return name
}

func (h *flowHost) EntityCommand(ctx context.Context, org, entityID, service string, params map[string]any, flowID, issuer, reason string) (string, string, string, error) {
	if h.s.smart == nil {
		return "", "", "", errors.New("smart home is not available")
	}
	b, _ := json.Marshal(params)
	var p smarthome.Params
	if err := json.Unmarshal(b, &p); err != nil {
		return "", "", "", err
	}
	dev, err := h.s.smart.Check(entityID, service, p)
	if err != nil {
		return "", "", "", err
	}
	args, _ := json.Marshal(smarthome.CommandArgs{Entity: entityID, Params: p})
	res, err := h.s.RunCommand(ctx, rules.CommandRequest{OrgID: org, DeviceID: dev, Method: service, Args: args, FlowID: flowID, Issuer: issuer, Reason: reason})
	if err != nil {
		return "", "", "", err
	}
	return res.ID, res.Status, res.Error, nil
}

func (h *flowHost) MQTTSubscribe(org, source, filter string, fn func(topic string, payload []byte)) (func(), error) {
	if h.s.smart == nil {
		return nil, errors.New("smart home is not available")
	}
	return h.s.smart.Subscribe(org, source, filter, fn)
}

func (h *flowHost) MQTTPublish(ctx context.Context, org, source, topic string, payload []byte, retain bool) error {
	if h.s.smart == nil {
		return errors.New("smart home is not available")
	}
	return h.s.smart.PublishRaw(ctx, org, source, topic, payload, retain)
}

func (h *flowHost) SetAttribute(ctx context.Context, org, dev, key string, value any, issuer, reason string) error {
	if key == "" || len(key) > 64 {
		return errors.New("attribute keys must be 1–64 characters")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	old, doc, _, _, err := h.s.setSharedAttributes(ctx, org, dev, map[string]json.RawMessage{key: raw}, nil)
	if err != nil {
		return err
	}
	_, _, err = audit.Append(ctx, h.s.pool, audit.Entry{OrgID: org, ActorType: "system", ActorName: issuer, Action: "device.attributes", EntityType: "device", EntityID: dev, Old: old, New: doc, Reason: reason})
	return err
}

func (h *flowHost) Notify(ctx context.Context, org, channelID, flowID, subject, text string, payload map[string]any) error {
	var ch notify.Channel
	var raw []byte
	if err := h.s.pool.QueryRow(ctx, `SELECT id::text, org_id::text, name, kind, settings FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND enabled AND revoked_at IS NULL`, channelID, org).
		Scan(&ch.ID, &ch.OrgID, &ch.Name, &ch.Kind, &raw); err != nil {
		return errors.New("channel not found or disabled")
	}
	_ = json.Unmarshal(raw, &ch.Settings)
	msg := notify.Message{Event: "flow", Subject: subject, Text: text, Rule: payload, SentAt: time.Now().UTC()}
	body, _ := json.Marshal(msg)
	var nid string
	_ = h.s.pool.QueryRow(ctx, `INSERT INTO core.notification (org_id, channel_id, flow_id, event, payload) VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, 'flow', $4::jsonb) RETURNING id::text`,
		org, ch.ID, flowID, string(body)).Scan(&nid)
	if h.s.notifier == nil {
		return errors.New("notifications are not available")
	}
	err := h.s.notifier.Deliver(ctx, ch, msg)
	status, errText := "sent", ""
	if err != nil {
		status, errText = "failed", err.Error()
	}
	if nid != "" {
		_, _ = h.s.pool.Exec(ctx, `UPDATE core.notification SET status = $2, sent_at = now(), attempts = 1, error = NULLIF($3, '') WHERE id = $1::uuid`, nid, status, errText)
	}
	return err
}

func (h *flowHost) Incident(ctx context.Context, org, action, severity, title, dedup string, detail map[string]any) error {
	if h.s.incidents == nil {
		return errors.New("incidents are not available")
	}
	if action == "resolve" {
		var id string
		if err := h.s.pool.QueryRow(ctx, `SELECT id::text FROM core.incident WHERE dedup_key = $1 AND status IN ('open', 'investigating')`, dedup).Scan(&id); err != nil {
			return nil // nothing open
		}
		in, _, err := h.s.incidents.Get(ctx, org, id, false)
		if err != nil {
			return err
		}
		st, cause := "resolved", in.Cause
		if cause == "" {
			cause = "Resolved by the flow: " + title
		}
		_, err = h.s.incidents.Apply(ctx, in, incident.Update{Status: &st, Cause: &cause}, "flow")
		return err
	}
	_, _, err := h.s.incidents.Open(ctx, incident.Incident{OrgID: org, Kind: "flow", Severity: severity, Title: title, Detail: detail, DedupKey: dedup, OpenedBy: "flow"})
	return err
}

func (h *flowHost) HTTP(ctx context.Context, org, method, rawURL string, body []byte) (int, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return 0, "", errors.New("bad URL")
	}
	if !(flowRefs{h.s, ctx, org}).HostAllowed(u.Hostname()) {
		return 0, "", fmt.Errorf("host %s is not on the allow-list", u.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ctrl32-telemetry-flow")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !(flowRefs{h.s, ctx, org}).HostAllowed(r.URL.Hostname()) {
			return errors.New("redirect to a host not on the allow-list")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, "", errors.New("timeout")
		}
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(b), nil
}

func (h *flowHost) Location(ctx context.Context, org string) *time.Location {
	h.mu.Lock()
	h.reset()
	l, ok := h.locs[org]
	h.mu.Unlock()
	if ok {
		return l
	}
	var tz string
	_ = h.s.pool.QueryRow(ctx, `SELECT tz FROM core.organization WHERE id = $1::uuid`, org).Scan(&tz)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	h.mu.Lock()
	h.locs[org] = loc
	h.mu.Unlock()
	return loc
}

// ruleAsFlow converts an automation rule into a flow spec (FR-010); the client creates it as a draft.
func (s *Server) ruleAsFlow(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var name string
	var raw json.RawMessage
	err := s.pool.QueryRow(r.Context(), `SELECT name, spec FROM core.rule WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, r.PathValue("id"), org).Scan(&name, &raw)
	if err != nil {
		writeErr(w, 404, "rule not found")
		return
	}
	var rs rules.Spec
	if json.Unmarshal(raw, &rs) != nil {
		writeErr(w, 500, "stored rule spec is unreadable")
		return
	}
	sp, notes := flows.FromRule(name, rs)
	if notes == nil {
		notes = []string{}
	}
	problems := flows.Validate(&sp, flowRefs{s, r.Context(), org})
	if problems == nil {
		problems = []flows.Problem{}
	}
	writeJSON(w, 200, map[string]any{"name": name, "spec": sp, "notes": notes, "problems": problems})
}
