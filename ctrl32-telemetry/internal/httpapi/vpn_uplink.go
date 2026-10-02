package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/uptunnel"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// VPN phase 3 — uplink to a hub (0.69.0, ADR-0021, spec 027).
//
// Hub side (a telemetry server with a public address): a site peer with `uplink: true` is another telemetry server.
// Adding it (or rotating its key) returns an uplink token instead of router texts — once, after the password again —
// with the hub's endpoint, keys, the address in the hub's VPN, the hub's range and the networks the hub routes to it;
// when the hub has an HTTPS address the token also carries the HTTPS transport (a per-uplink secret; the hub keeps its
// SHA-256). The hub's own access rules decide which technicians reach the uplink's address and networks.
//
//	GET  /api/v1/vpn/uplink/tunnel   WebSocket of the HTTPS transport (bearer secret of the uplink; not a user API)
//
// Customer side (this server inside a LAN): system.admin connects the server to a hub with the pasted token; its own
// uplink access rules (default deny) decide what the hub's range may reach — this server's uplink address or hosts of
// the networks it offers (forwarding with masquerade). Access needs a rule on both sides.
//
//	POST /api/v1/vpn/uplink                     {token, transport, reason, password, code}: connect or replace
//	POST /api/v1/vpn/uplink/transport           {transport, reason}
//	POST /api/v1/vpn/uplink/disconnect          {reason}
//	POST /api/v1/vpn/uplink/rules               {name, src_cidr, dest_type, dest_cidr, protocol, ports, enabled, reason}
//	PUT  /api/v1/vpn/uplink/rules/{id}          same
//	POST /api/v1/vpn/uplink/rules/{id}/remove   {reason}

// ---- hub side --------------------------------------------------------------------------------------------------------

// uplinkTunnelURL is the WebSocket URL of the HTTPS transport, or "" when this server has no HTTPS address.
func (s *Server) uplinkTunnelURL() string {
	u, err := url.Parse(strings.TrimSpace(s.cfg.HTTP.BaseURL))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return "https://" + u.Host + vpn.UplinkTunnelPath
}

// uplinkConfig is what the hub shows once for an uplink peer: the token to paste on the remote server.
func (s *Server) uplinkConfig(st *agent.WireGuardState, srv netip.Prefix, p vpn.Peer, priv, psk, tunnelKey string) map[string]any {
	t := vpn.UplinkToken{Name: p.Name, Endpoint: st.Endpoint, HubPublicKey: st.PublicKey, PresharedKey: psk, PrivateKey: priv,
		Address: p.Address.String(), VPNNet: srv.Masked().String(), HubAddress: srv.Addr().String(), Subnets: []string{}}
	for _, sn := range p.Subnets {
		t.Subnets = append(t.Subnets, sn.String())
	}
	if tunnelKey != "" {
		t.TunnelURL, t.TunnelKey = s.uplinkTunnelURL(), tunnelKey
	}
	return map[string]any{"uplink_token": vpn.EncodeUplinkToken(t), "server": srv.Addr().String(), "allowed_ips": vpn.HubAllowedIPs(srv),
		"https_transport": t.TunnelURL != "", "address": t.Address, "subnets": t.Subnets}
}

// newUplinkTunnel is the hub's endpoint of the HTTPS transport: an active uplink peer's secret opens a relay to the
// local WireGuard port.
func (s *Server) newUplinkTunnel() *uptunnel.Handler {
	return &uptunnel.Handler{Hash: vpn.HashTunnelKey, Log: s.log,
		Lookup: func(ctx context.Context, hash string) (string, string, bool) {
			var id, name string
			err := s.pool.QueryRow(ctx, `SELECT id::text, name FROM core.vpn_peer WHERE tunnel_key_hash = $1 AND removed_at IS NULL AND uplink AND enabled
				AND (expires_at IS NULL OR expires_at > now())`, hash).Scan(&id, &name)
			return id, name, err == nil
		},
		Target: func(ctx context.Context) (netip.AddrPort, error) {
			st, err := s.agent.WireGuard(ctx)
			if err != nil {
				return netip.AddrPort{}, err
			}
			if !st.Enabled || st.ListenPort == 0 {
				return netip.AddrPort{}, errors.New("the VPN is not enabled")
			}
			return netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(st.ListenPort)), nil
		}}
}

// GET /api/v1/vpn/uplink/tunnel
func (s *Server) uplinkTunnelWS(w http.ResponseWriter, r *http.Request) {
	if !s.agent.Configured() {
		writeErr(w, 503, "no VPN on this server")
		return
	}
	s.uplinkTunnel.ServeHTTP(w, r)
}

// ---- customer side: rules -------------------------------------------------------------------------------------------

type uplinkRuleRow struct {
	vpn.UplinkRule
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
}

func loadUplinkRules(ctx context.Context, q dbQuerier) ([]uplinkRuleRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, coalesce(src_cidr::text, ''), dest_type, coalesce(dest_cidr::text, ''), protocol, ports, enabled, created_at, created_by, updated_at
		FROM core.vpn_uplink_rule WHERE removed_at IS NULL ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uplinkRuleRow
	for rows.Next() {
		var r uplinkRuleRow
		if err := rows.Scan(&r.ID, &r.Name, &r.SrcCIDR, &r.DestType, &r.DestCIDR, &r.Protocol, &r.Ports, &r.Enabled, &r.CreatedAt, &r.CreatedBy, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func uplinkRuleJSON(r uplinkRuleRow) map[string]any {
	return map[string]any{"id": r.ID, "name": r.Name, "src_cidr": r.SrcCIDR, "dest_type": r.DestType, "dest_cidr": r.DestCIDR, "protocol": r.Protocol,
		"ports": r.Ports, "enabled": r.Enabled, "created_at": r.CreatedAt, "created_by": r.CreatedBy, "updated_at": r.UpdatedAt}
}

func plainUplinkRules(rows []uplinkRuleRow) []vpn.UplinkRule {
	out := make([]vpn.UplinkRule, len(rows))
	for i, r := range rows {
		out[i] = r.UplinkRule
	}
	return out
}

// uplinkInfoOf is the connected uplink as the agent reports it (zero when none).
func uplinkInfoOf(u *agent.UplinkState) vpn.UplinkInfo {
	var info vpn.UplinkInfo
	if u == nil || !u.Configured {
		return info
	}
	info.Address, _ = netip.ParseAddr(u.Address)
	info.VPNNet, _ = netip.ParsePrefix(u.VPNNet)
	for _, s := range u.Subnets {
		if p, err := netip.ParsePrefix(s); err == nil {
			info.Subnets = append(info.Subnets, p)
		}
	}
	return info
}

// uplinkServices are the ports of this server a rule may open to the hub (presets of the UI).
func (s *Server) uplinkServices(u *agent.UplinkState) []map[string]any {
	out := []map[string]any{{"key": "mqtt", "name": "MQTT in the tunnel (plain, no TLS)", "protocol": "tcp", "ports": "1883"}}
	web := []string{}
	if host, port, err := net.SplitHostPort(s.cfg.HTTP.Listen); err == nil {
		if ip := net.ParseIP(host); host == "" || (ip != nil && !ip.IsLoopback()) || (ip == nil && host != "localhost") || (u != nil && u.Mode == agent.ModeUserspace) {
			web = append(web, port)
		}
	}
	if bu, err := url.Parse(s.cfg.HTTP.BaseURL); err == nil && bu.Scheme == "https" && bu.Port() == "" {
		web = append(web, "443")
	}
	if len(web) > 0 {
		out = append(out, map[string]any{"key": "web", "name": "Web interface", "protocol": "tcp", "ports": strings.Join(web, ", ")})
	}
	if _, port, err := net.SplitHostPort(s.cfg.MQTT.TLSListen); err == nil && port != "" {
		out = append(out, map[string]any{"key": "mqtt_tls", "name": "MQTT with TLS", "protocol": "tcp", "ports": port})
	}
	if u == nil || u.Mode != agent.ModeUserspace {
		out = append(out, map[string]any{"key": "ssh", "name": "SSH (administration of the host)", "protocol": "tcp", "ports": "22"})
	}
	return out
}

// uplinkView adds the uplink to GET /api/v1/vpn (server administrators only).
func (s *Server) uplinkView(ctx context.Context, st *agent.WireGuardState, out map[string]any) error {
	rules, err := loadUplinkRules(ctx, s.pool)
	if err != nil {
		return err
	}
	rl := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		rl = append(rl, uplinkRuleJSON(r))
	}
	out["uplink_rules"] = rl
	out["uplink_https"] = s.uplinkTunnelURL() != "" // tokens issued here carry the HTTPS transport
	if st != nil && st.Uplink != nil {
		out["uplink"] = st.Uplink
		out["uplink_services"] = s.uplinkServices(st.Uplink)
	}
	return nil
}

// uplinkPush compiles the stored uplink rules (inside q, uncommitted changes included) for the connected uplink and
// has the agent apply them. Without an uplink nothing is sent (the rules are applied when one is connected).
func (s *Server) uplinkPush(ctx context.Context, q dbQuerier, u *agent.UplinkState) (*agent.WireGuardState, error) {
	if u == nil || !u.Configured {
		return nil, nil
	}
	rows, err := loadUplinkRules(ctx, q)
	if err != nil {
		return nil, err
	}
	fw, _ := vpn.CompileUplink(uplinkInfoOf(u), plainUplinkRules(rows))
	return s.agent.UplinkRules(ctx, fw)
}

// uplinkReady returns the agent's uplink state for a change, or answers the request.
func (s *Server) uplinkReady(w http.ResponseWriter, r *http.Request) (*agent.WireGuardState, bool) {
	st, err := s.agent.WireGuard(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return nil, false
	}
	if !st.Supported || st.Uplink == nil {
		writeErr(w, 501, "the agent of this server does not manage an uplink")
		return nil, false
	}
	return st, true
}

type uplinkRuleInput struct {
	Name     string `json:"name"`
	SrcCIDR  string `json:"src_cidr"`
	DestType string `json:"dest_type"`
	DestCIDR string `json:"dest_cidr"`
	Protocol string `json:"protocol"`
	Ports    string `json:"ports"`
	Enabled  *bool  `json:"enabled"`
	Reason   string `json:"reason"`
}

func uplinkRuleAudit(r vpn.UplinkRule) map[string]any {
	return map[string]any{"name": r.Name, "src_cidr": r.SrcCIDR, "dest_type": r.DestType, "dest_cidr": r.DestCIDR, "protocol": r.Protocol, "ports": r.Ports, "enabled": r.Enabled}
}

// uplinkRuleChange runs fn in a transaction, applies the rules through the agent (when an uplink is connected) and
// commits only when they were applied.
func (s *Server) uplinkRuleChange(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, tx pgx.Tx, info vpn.UplinkInfo) error) bool {
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	st, ok := s.uplinkReady(w, r)
	if !ok {
		return false
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "database unavailable")
		return false
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(ctx, tx, uplinkInfoOf(st.Uplink)); err != nil {
		vpnFail(w, err)
		return false
	}
	if _, err := s.uplinkPush(ctx, tx, st.Uplink); err != nil {
		writeErr(w, 502, "the uplink rules were not applied, nothing changed: "+err.Error())
		return false
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "saving failed after the agent applied the change; it is restored within five minutes")
		return false
	}
	return true
}

func (s *Server) saveUplinkRule(w http.ResponseWriter, r *http.Request, id string) {
	var in uplinkRuleInput
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	pr := PrincipalFrom(r.Context())
	rule := vpn.UplinkRule{ID: id, Name: strings.TrimSpace(in.Name), SrcCIDR: in.SrcCIDR, DestType: in.DestType, DestCIDR: in.DestCIDR, Protocol: in.Protocol,
		Ports: in.Ports, Enabled: in.Enabled == nil || *in.Enabled}
	var before any
	ok := s.uplinkRuleChange(w, r, func(ctx context.Context, tx pgx.Tx, info vpn.UplinkInfo) error {
		if err := vpn.ValidateUplinkRule(&rule, info); err != nil {
			return vpnBad(err.Error())
		}
		if id == "" {
			n := 0
			_ = tx.QueryRow(ctx, `SELECT count(*) FROM core.vpn_uplink_rule WHERE removed_at IS NULL`).Scan(&n)
			if n >= vpn.MaxUplinkRules {
				return vpnBad("too many uplink rules")
			}
			return tx.QueryRow(ctx, `INSERT INTO core.vpn_uplink_rule (name, src_cidr, dest_type, dest_cidr, protocol, ports, enabled, created_by)
				VALUES ($1, $2::cidr, $3, $4::cidr, $5, $6, $7, $8) RETURNING id::text`,
				rule.Name, nullStr(rule.SrcCIDR), rule.DestType, nullStr(rule.DestCIDR), rule.Protocol, rule.Ports, rule.Enabled, pr.Username).Scan(&rule.ID)
		}
		rows, err := loadUplinkRules(ctx, tx)
		if err != nil {
			return err
		}
		for _, x := range rows {
			if x.ID == id {
				before = uplinkRuleAudit(x.UplinkRule)
			}
		}
		if before == nil {
			return &vpnErr{code: 404, msg: "rule not found"}
		}
		_, err = tx.Exec(ctx, `UPDATE core.vpn_uplink_rule SET name = $2, src_cidr = $3::cidr, dest_type = $4, dest_cidr = $5::cidr, protocol = $6, ports = $7, enabled = $8, updated_at = now()
			WHERE id = $1::uuid`, id, rule.Name, nullStr(rule.SrcCIDR), rule.DestType, nullStr(rule.DestCIDR), rule.Protocol, rule.Ports, rule.Enabled)
		return err
	})
	if !ok {
		return
	}
	after := uplinkRuleAudit(rule)
	if id == "" {
		s.audit(r, "vpn.uplink_rule_add", "vpn_uplink_rule", rule.ID, nil, after, in.Reason)
		after["id"] = rule.ID
		writeJSON(w, 201, after)
		return
	}
	s.audit(r, "vpn.uplink_rule_update", "vpn_uplink_rule", id, before, after, in.Reason)
	after["id"] = id
	writeJSON(w, 200, after)
}

// POST /api/v1/vpn/uplink/rules
func (s *Server) createUplinkRule(w http.ResponseWriter, r *http.Request) { s.saveUplinkRule(w, r, "") }

// PUT /api/v1/vpn/uplink/rules/{id}
func (s *Server) putUplinkRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid rule id")
		return
	}
	s.saveUplinkRule(w, r, id)
}

// POST /api/v1/vpn/uplink/rules/{id}/remove
func (s *Server) removeUplinkRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid rule id")
		return
	}
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if !reasonOf(w, in.Reason) {
		return
	}
	pr := PrincipalFrom(r.Context())
	var name string
	ok := s.uplinkRuleChange(w, r, func(ctx context.Context, tx pgx.Tx, _ vpn.UplinkInfo) error {
		if err := tx.QueryRow(ctx, `UPDATE core.vpn_uplink_rule SET removed_at = now(), removed_by = $2 WHERE id = $1::uuid AND removed_at IS NULL RETURNING name`,
			id, pr.Username).Scan(&name); err != nil {
			return &vpnErr{code: 404, msg: "rule not found"}
		}
		return nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.uplink_rule_remove", "vpn_uplink_rule", id, map[string]any{"name": name}, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- customer side: connecting --------------------------------------------------------------------------------------

func uplinkAudit(u *agent.UplinkState) map[string]any {
	if u == nil || !u.Configured {
		return nil
	}
	return map[string]any{"hub": u.HubEndpoint, "hub_public_key": u.HubPublicKey, "name": u.Name, "address": u.Address, "vpn_net": u.VPNNet,
		"subnets": u.Subnets, "transport": u.Transport, "public_key": u.PublicKey}
}

// POST /api/v1/vpn/uplink
func (s *Server) connectUplink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token     string `json:"token"`
		Transport string `json:"transport"`
		Reason    string `json:"reason"`
		Password  string `json:"password"`
		Code      string `json:"code"`
	}
	if err := decodeJSON(r, &in, 32<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	tok, err := vpn.ParseUplinkToken(in.Token)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Transport == "" {
		in.Transport = vpn.TransportUDP
	}
	// connecting this server to an outside network is a step like issuing a key: the password again
	if !s.stepUp(w, r, in.Password, in.Code) {
		return
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	st, ok := s.uplinkReady(w, r)
	if !ok {
		return
	}
	before := uplinkAudit(st.Uplink)
	ctx := r.Context()
	rows, err := loadUplinkRules(ctx, s.pool)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	info := vpn.UplinkInfo{VPNNet: netip.MustParsePrefix(tok.VPNNet)}
	info.Address, _ = netip.ParseAddr(tok.Address)
	for _, sn := range tok.Subnets {
		info.Subnets = append(info.Subnets, netip.MustParsePrefix(sn))
	}
	fw, _ := vpn.CompileUplink(info, plainUplinkRules(rows))
	res, err := s.agent.ConnectUplink(ctx, agent.UplinkRequest{Token: in.Token, Transport: in.Transport, Rules: fw})
	if err != nil {
		if errors.Is(err, agent.ErrNoAgent) {
			s.agentErr(w, err)
			return
		}
		writeErr(w, 409, "the uplink was not connected: "+err.Error())
		return
	}
	action := "vpn.uplink_connect"
	if before != nil {
		action = "vpn.uplink_replace"
	}
	s.audit(r, action, "server", "vpn_uplink", before, uplinkAudit(res.Uplink), in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "uplink": res.Uplink, "rules_applied": len(fw)})
}

// POST /api/v1/vpn/uplink/transport
func (s *Server) uplinkTransport(w http.ResponseWriter, r *http.Request) {
	var in struct{ Transport, Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Transport != vpn.TransportUDP && in.Transport != vpn.TransportHTTPS {
		writeErr(w, 400, "transport must be udp or https")
		return
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	st, ok := s.uplinkReady(w, r)
	if !ok {
		return
	}
	if !st.Uplink.Configured {
		writeErr(w, 409, "this server is not connected to a hub")
		return
	}
	res, err := s.agent.UplinkTransport(r.Context(), in.Transport)
	if err != nil {
		writeErr(w, 409, "the transport was not changed: "+err.Error())
		return
	}
	s.audit(r, "vpn.uplink_transport", "server", "vpn_uplink", map[string]any{"transport": st.Uplink.Transport}, map[string]any{"transport": in.Transport}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "uplink": res.Uplink})
}

// POST /api/v1/vpn/uplink/disconnect
func (s *Server) disconnectUplink(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if !reasonOf(w, in.Reason) {
		return
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	st, ok := s.uplinkReady(w, r)
	if !ok {
		return
	}
	if !st.Uplink.Configured {
		writeErr(w, 409, "this server is not connected to a hub")
		return
	}
	res, err := s.agent.DisconnectUplink(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return
	}
	s.audit(r, "vpn.uplink_disconnect", "server", "vpn_uplink", uplinkAudit(st.Uplink), nil, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "uplink": res.Uplink})
}

// uplinkTick pushes the stored uplink rules again (the periodic synchronisation heals an agent that lost them).
func (s *Server) uplinkTick(ctx context.Context, st *agent.WireGuardState) {
	if st == nil || st.Uplink == nil || !st.Uplink.Configured {
		return
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	if _, err := s.uplinkPush(ctx, s.pool, st.Uplink); err != nil {
		s.log.Warn("vpn uplink synchronisation failed", "err", err)
	}
}
