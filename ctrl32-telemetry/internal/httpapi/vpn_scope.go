package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// VPN phase 2 (0.65.0, ADR-0018 amendment, ADR-0019):
//
//   - Management per organization. A server administrator (system.admin) manages every peer and rule and the
//     server's VPN settings. An organization's VPN manager (vpn.manage, part of org_admin) sees and manages only the
//     peers of the own organization (vpn_peer.org_id) and the organization's own rules (vpn_rule.org_id): sources are
//     the organization's peers or the organization as a whole, destinations its own peers and sites or the services of
//     this server that are offered to the VPN (never host administration such as SSH). Peers and rules of other
//     organizations and of the whole server are invisible to it (404, not 403, so their existence is not revealed).
//   - Time-limited access grants. POST /api/v1/vpn/peers/{id}/grant {minutes, reason} enables a peer — and with it
//     every rule that names it — for a window; when the window ends the peer is disabled again (audited). With the
//     organization policy four_eyes_vpn a grant for a technician (user peer) is not executed but held as a change
//     request (Settings → Approvals) until a second person approves it; the window starts at the approval. A new
//     technician peer is then created disabled, and enabling it or changing its expiry directly is refused.
//   - Licence: grants (time-limited access) need the licence, like an expiry date in phase 1.

// vpnScope is what the caller may manage.
type vpnScope struct {
	server bool   // system.admin: everything, including the server's VPN settings
	org    string // vpn.manage only: the own organization
}

func vpnScopeOf(p *iam.Principal) vpnScope {
	if p.Has(iam.ServerPermission) {
		return vpnScope{server: true}
	}
	return vpnScope{org: p.OrgID}
}

// owns reports whether an object of organization orgID ("" = the whole server) is within the scope.
func (sc vpnScope) owns(orgID string) bool { return sc.server || (orgID != "" && orgID == sc.org) }

func vpnPeerNotFound() error { return &vpnErr{code: 404, msg: "peer not found"} }

// orgServicePorts are the TCP ports of this server an organization's rule may open: the offered services without
// host administration (SSH stays with the server administrator).
func (s *Server) orgServicePorts() []vpn.PortRange {
	var out []vpn.PortRange
	for _, svc := range s.vpnServices(nil) {
		if so, _ := svc["server_only"].(bool); so {
			continue
		}
		if ps, err := vpn.ParsePorts(fmt.Sprint(svc["ports"])); err == nil {
			out = append(out, ps...)
		}
	}
	return out
}

// checkOrgRule limits a rule of an organization's VPN manager to the organization (ValidateRule ran before).
func (s *Server) checkOrgRule(rule *vpn.Rule, org string, peers map[string]vpnPeerRow) error {
	for _, src := range rule.Sources {
		switch src.Type {
		case vpn.SrcPeer:
			if p, ok := peers[src.ID]; !ok || p.OrgID != org {
				return &vpnErr{code: 404, msg: "source peer not found"}
			}
		case vpn.SrcOrg:
			if src.ID != org {
				return &vpnErr{code: 403, msg: "an organization's rule may use its own organization as a source only"}
			}
		default:
			return &vpnErr{code: 403, msg: "an organization's rule may use its own peers or its own organization as sources (all peers or a kind span every organization: server administrators only)"}
		}
	}
	switch rule.DestType {
	case vpn.DstPeer, vpn.DstSite:
		if p, ok := peers[rule.DestPeerID]; !ok || p.OrgID != org {
			return &vpnErr{code: 404, msg: "destination not found"}
		}
	case vpn.DstServer:
		switch rule.Protocol {
		case "icmp":
			return nil
		case "tcp":
			if strings.TrimSpace(rule.Ports) == "" {
				return &vpnErr{code: 403, msg: "an organization's rule to this server needs ports of the offered services"}
			}
			ps, err := vpn.ParsePorts(rule.Ports)
			if err != nil {
				return vpnBad(err.Error())
			}
			allowed := s.orgServicePorts()
			for _, p := range ps {
				in := false
				for _, a := range allowed {
					if p.Lo >= a.Lo && p.Hi <= a.Hi {
						in = true
					}
				}
				if !in {
					return &vpnErr{code: 403, msg: fmt.Sprintf("port %s of this server is not offered to organizations (allowed: %s)", p, vpn.FormatPorts(allowed))}
				}
			}
			return nil
		default:
			return &vpnErr{code: 403, msg: "an organization's rule to this server allows tcp to the offered services or ping (icmp) only"}
		}
	}
	return nil
}

// ---- access grants ----------------------------------------------------------------------------------------------------

// Grant windows: at least 15 minutes, at most 30 days.
const (
	grantMinMinutes = 15
	grantMaxMinutes = 30 * 24 * 60
)

func grantDuration(min int) string {
	switch {
	case min%(24*60) == 0:
		if min == 24*60 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", min/(24*60))
	case min%60 == 0:
		if min == 60 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", min/60)
	}
	return fmt.Sprintf("%d minutes", min)
}

// vpnFourEyes reports whether the caller's organization requires a second person for technicians' VPN access.
func (s *Server) vpnFourEyes(r *http.Request) bool {
	p := PrincipalFrom(r.Context())
	if p == nil || p.OrgID == "" || s.iam == nil {
		return false
	}
	return s.iam.Policy(r.Context(), p.OrgID).FourEyesVPN
}

// POST /api/v1/vpn/peers/{id}/grant {minutes, reason}
func (s *Server) grantVPNPeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid peer id")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeErr(w, 400, "body too large")
		return
	}
	var in struct {
		Minutes int    `json:"minutes"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Minutes < grantMinMinutes || in.Minutes > grantMaxMinutes {
		writeErr(w, 400, fmt.Sprintf("minutes: %d to %d (15 minutes to 30 days)", grantMinMinutes, grantMaxMinutes))
		return
	}
	ctx := r.Context()
	p := PrincipalFrom(ctx)
	sc := vpnScopeOf(p)
	if !s.vpnLicensed(ctx) {
		writeJSON(w, 402, map[string]string{"error": "Time-limited access grants require a license.", "code": "license_required"})
		return
	}
	var kind, org, name string
	if err := s.pool.QueryRow(ctx, `SELECT kind, coalesce(org_id::text, ''), name FROM core.vpn_peer WHERE id = $1::uuid AND removed_at IS NULL`, id).Scan(&kind, &org, &name); err != nil || !sc.owns(org) {
		writeErr(w, 404, "peer not found")
		return
	}
	ci := changeFrom(ctx)
	if ci == nil && kind == vpn.KindUser && s.vpnFourEyes(r) {
		s.holdChange(w, r, "vpn", body, fmt.Sprintf("VPN access for technician %s for %s", name, grantDuration(in.Minutes)), strings.TrimSpace(in.Reason))
		return
	}
	requester, approver, crID := p.Username, "", ""
	if ci != nil {
		requester, approver, crID = ci.RequestedBy, p.Username, ci.ID
	}
	now := time.Now()
	ends := now.Add(time.Duration(in.Minutes) * time.Minute).Truncate(time.Second)
	var grantID string
	_, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		tag, err := tx.Exec(ctx, `UPDATE core.vpn_peer SET enabled = true, expires_at = $2, updated_at = now() WHERE id = $1::uuid AND removed_at IS NULL`, id, ends)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, vpnPeerNotFound()
		}
		if _, err := tx.Exec(ctx, `UPDATE core.vpn_grant SET ended_at = now(), ended_by = $2, end_reason = 'replaced by a new grant' WHERE peer_id = $1::uuid AND ended_at IS NULL`, id, p.Username); err != nil {
			return nil, err
		}
		return nil, tx.QueryRow(ctx, `INSERT INTO core.vpn_grant (peer_id, org_id, starts_at, ends_at, reason, requested_by, approved_by, change_request_id)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8::uuid) RETURNING id::text`,
			id, nullStr(org), now, ends, strings.TrimSpace(in.Reason), requester, nullStr(approver), nullStr(crID)).Scan(&grantID)
	})
	if !ok {
		return
	}
	g := map[string]any{"id": grantID, "peer_id": id, "peer": name, "starts_at": now, "ends_at": ends, "minutes": in.Minutes, "requested_by": requester}
	if approver != "" {
		g["approved_by"] = approver
	}
	s.audit(r, "vpn.grant", "vpn_peer", id, nil, g, in.Reason)
	writeJSON(w, 201, map[string]any{"ok": true, "grant": g})
}

// endGrants closes the open grants of a peer (disabled, removed) inside a VPN transaction.
func endGrants(ctx context.Context, tx pgx.Tx, peerID, by, why string) error {
	_, err := tx.Exec(ctx, `UPDATE core.vpn_grant SET ended_at = now(), ended_by = $2, end_reason = $3 WHERE peer_id = $1::uuid AND ended_at IS NULL`, peerID, by, why)
	return err
}

// vpnGrantInfo returns the open grant of every peer and the pending (four-eyes) grant requests by peer id.
func (s *Server) vpnGrantInfo(ctx context.Context) (map[string]map[string]any, map[string]map[string]any) {
	open, pending := map[string]map[string]any{}, map[string]map[string]any{}
	if rows, err := s.pool.Query(ctx, `SELECT peer_id::text, starts_at, ends_at, requested_by, coalesce(approved_by, '') FROM core.vpn_grant WHERE ended_at IS NULL AND ends_at > now()`); err == nil {
		for rows.Next() {
			var pid, req, appr string
			var st, en time.Time
			if rows.Scan(&pid, &st, &en, &req, &appr) == nil {
				open[pid] = map[string]any{"starts_at": st, "ends_at": en, "requested_by": req, "approved_by": appr}
			}
		}
		rows.Close()
	}
	if rows, err := s.pool.Query(ctx, `SELECT id::text, path, requested_by_name, requested_at, summary FROM core.change_request WHERE kind = 'vpn' AND status = 'pending' AND expires_at > now() ORDER BY requested_at`); err == nil {
		for rows.Next() {
			var cid, path, by, summary string
			var at time.Time
			if rows.Scan(&cid, &path, &by, &at, &summary) == nil {
				parts := strings.Split(strings.Trim(strings.SplitN(path, "?", 2)[0], "/"), "/")
				if len(parts) == 6 && parts[3] == "peers" && parts[5] == "grant" {
					pending[parts[4]] = map[string]any{"change_request_id": cid, "requested_by": by, "requested_at": at, "summary": summary}
				}
			}
		}
		rows.Close()
	}
	return open, pending
}
