package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/incident"
)

// Incident log (URS C-07, I-09): system-opened incidents (undelivered alarm, broken audit chain) and manual ones,
// each with impact, cause and CAPA and a small workflow. Organization users see their organization's incidents;
// server administrators (system.admin) also the instance-level ones (the server's disk, backups, the audit chain).

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "open"
	}
	if status != "all" && status != "open" && !contains(incident.Statuses, status) {
		writeErr(w, 400, "status must be open, all, open, investigating, resolved or closed")
		return
	}
	list, err := s.incidents.List(r.Context(), orgOf(p), status, 0, p.Has(iam.ServerPermission))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	in, events, err := s.incidents.Get(r.Context(), orgOf(p), r.PathValue("id"), orgOf(p) == "" || p.Has(iam.ServerPermission))
	if err != nil {
		writeErr(w, 404, "incident not found")
		return
	}
	writeJSON(w, 200, map[string]any{"incident": in, "events": events})
}

func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if orgOf(p) == "" {
		writeErr(w, 403, "an incident belongs to an organization; sign in as a member of one")
		return
	}
	var in struct {
		Title    string `json:"title"`
		Severity string `json:"severity"`
		Impact   string `json:"impact"`
		Cause    string `json:"cause"`
		CAPA     string `json:"capa"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	if in.Severity == "" {
		in.Severity = "major"
	}
	if len(in.Title) > 200 || strings.TrimSpace(in.Title) == "" {
		writeErr(w, 400, "title is required (up to 200 characters)")
		return
	}
	inc, _, err := s.incidents.Open(r.Context(), incident.Incident{OrgID: p.OrgID, Kind: "manual", Severity: in.Severity, Title: in.Title,
		Impact: strings.TrimSpace(in.Impact), Cause: strings.TrimSpace(in.Cause), CAPA: strings.TrimSpace(in.CAPA), OpenedBy: p.Username,
		Detail: map[string]any{"reason": in.Reason}})
	if err != nil {
		if errors.Is(err, incident.ErrInvalid) {
			writeErr(w, 400, err.Error())
			return
		}
		writeErr(w, 500, "create failed")
		return
	}
	writeJSON(w, 201, inc)
}

func (s *Server) updateIncident(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	inc, _, err := s.incidents.Get(r.Context(), orgOf(p), r.PathValue("id"), orgOf(p) == "" || p.Has(iam.ServerPermission))
	if err != nil {
		writeErr(w, 404, "incident not found")
		return
	}
	var in struct {
		Status *string `json:"status"`
		Impact *string `json:"impact"`
		Cause  *string `json:"cause"`
		CAPA   *string `json:"capa"`
		Note   *string `json:"note"`
		Reason string  `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	before := map[string]any{"status": inc.Status, "impact": inc.Impact, "cause": inc.Cause, "capa": inc.CAPA}
	changed, err := s.incidents.Apply(r.Context(), inc, incident.Update{Status: in.Status, Impact: in.Impact, Cause: in.Cause, CAPA: in.CAPA, Note: in.Note}, p.Username)
	if err != nil {
		if errors.Is(err, incident.ErrInvalid) {
			writeErr(w, 400, err.Error())
			return
		}
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "incident.update", "incident", inc.ID, before, changed, in.Reason)
	writeJSON(w, 200, inc)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
