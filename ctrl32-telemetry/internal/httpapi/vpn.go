package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Own WireGuard VPN management (spec 024, ADR-0018). The database holds peers and access rules; every change is made
// in a transaction, compiled into the complete state (internal/vpn) and sent to the privileged agent, which applies
// the interface and the nftables table together. The transaction commits only when the agent applied the state, so
// the database and the host never disagree; on failure both stay as they were and the error is shown. Everything
// needs system.admin, every change carries a reason into the audit trail, and showing a client configuration (it
// holds a private key) needs the password again (and the authenticator code when two-factor sign-in is on).
//
//	GET  /api/v1/vpn                          interface, firewall, licence, peers with live state, rules, services
//	POST /api/v1/vpn/peers                    {name, kind, org_id, device_id, subnets, expires_at, note, reason, password, code} -> config once
//	PATCH /api/v1/vpn/peers/{id}              {name, enabled, subnets, expires_at, org_id, device_id, note, reason}
//	POST /api/v1/vpn/peers/{id}/rotate        {reason, password, code} -> new keys, config once
//	POST /api/v1/vpn/peers/{id}/remove        {reason}
//	POST /api/v1/vpn/rules                    {name, sources, dest_type, dest_peer_id, dest_cidr, protocol, ports, enabled, reason}
//	PUT  /api/v1/vpn/rules/{id}               same
//	POST /api/v1/vpn/rules/{id}/remove        {reason}
//	POST /api/v1/vpn/apply                    {reason}: send the stored state to the agent again
//
// Licence: free are up to vpn.FreePeerLimit peers of kinds device and user; site peers (routed subnets), time-limited
// access and more peers need the server's licence (the same feature as white labeling). Without it existing peers keep
// working and access rules can always be changed (restricting access is never paid).

type vpnPeerRow struct {
	vpn.Peer
	Note         string
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    time.Time
	KeyRotatedAt *time.Time
	OrgName      string
	DeviceName   string
}

type vpnRuleRow struct {
	vpn.Rule
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
}

type dbQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadVPNPeers(ctx context.Context, q dbQuerier) ([]vpnPeerRow, error) {
	rows, err := q.Query(ctx, `SELECT p.id::text, p.name, p.kind, coalesce(p.org_id::text, ''), coalesce(p.device_id::text, ''), host(p.address), p.public_key,
		p.subnets::text[], p.enabled, p.expires_at, p.note, p.created_at, p.created_by, p.updated_at, p.key_rotated_at, coalesce(o.name, ''), coalesce(d.name, '')
		FROM core.vpn_peer p LEFT JOIN core.organization o ON o.id = p.org_id LEFT JOIN core.device d ON d.id = p.device_id
		WHERE p.removed_at IS NULL ORDER BY p.address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vpnPeerRow
	for rows.Next() {
		var p vpnPeerRow
		var addr string
		var subnets []string
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.OrgID, &p.DeviceID, &addr, &p.PublicKey, &subnets, &p.Enabled, &p.ExpiresAt, &p.Note,
			&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.KeyRotatedAt, &p.OrgName, &p.DeviceName); err != nil {
			return nil, err
		}
		p.Address, _ = netip.ParseAddr(addr)
		for _, s := range subnets {
			if pr, err := netip.ParsePrefix(s); err == nil {
				p.Subnets = append(p.Subnets, pr)
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadVPNRules(ctx context.Context, q dbQuerier) ([]vpnRuleRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, sources, dest_type, coalesce(dest_peer_id::text, ''), coalesce(dest_cidr::text, ''), protocol, ports, enabled,
		created_at, created_by, updated_at FROM core.vpn_rule WHERE removed_at IS NULL ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vpnRuleRow
	for rows.Next() {
		var r vpnRuleRow
		var src []byte
		if err := rows.Scan(&r.ID, &r.Name, &src, &r.DestType, &r.DestPeerID, &r.DestCIDR, &r.Protocol, &r.Ports, &r.Enabled, &r.CreatedAt, &r.CreatedBy, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(src, &r.Sources)
		out = append(out, r)
	}
	return out, rows.Err()
}

func plainPeers(rows []vpnPeerRow) []vpn.Peer {
	out := make([]vpn.Peer, len(rows))
	for i, r := range rows {
		out[i] = r.Peer
	}
	return out
}

func plainRules(rows []vpnRuleRow) []vpn.Rule {
	out := make([]vpn.Rule, len(rows))
	for i, r := range rows {
		out[i] = r.Rule
	}
	return out
}

// vpnLicensed: site peers, time-limited access and more than the free peers need the server's licence.
func (s *Server) vpnLicensed(ctx context.Context) bool { return s.whiteLabelLicensed(ctx) }

// ---- reading --------------------------------------------------------------------------------------------------------

func (s *Server) vpnServices(st *agent.WireGuardState) []map[string]any {
	out := []map[string]any{{"key": "mqtt", "name": "MQTT inside the VPN (plain, no TLS)", "protocol": "tcp", "ports": "1883"}}
	web := []string{}
	if host, port, err := net.SplitHostPort(s.cfg.HTTP.Listen); err == nil {
		if ip := net.ParseIP(host); host == "" || (ip != nil && !ip.IsLoopback()) || (ip == nil && host != "localhost") {
			web = append(web, port)
		}
	}
	if u, err := url.Parse(s.cfg.HTTP.BaseURL); err == nil && u.Scheme == "https" && u.Port() == "" {
		web = append(web, "443") // the reverse proxy (Caddy) listens on every address of the host
	}
	if len(web) > 0 {
		out = append(out, map[string]any{"key": "web", "name": "Web interface", "protocol": "tcp", "ports": strings.Join(web, ", ")})
	}
	if _, port, err := net.SplitHostPort(s.cfg.MQTT.TLSListen); err == nil && port != "" {
		out = append(out, map[string]any{"key": "mqtt_tls", "name": "MQTT with TLS", "protocol": "tcp", "ports": port})
	}
	out = append(out, map[string]any{"key": "ssh", "name": "SSH (administration of the host)", "protocol": "tcp", "ports": "22"})
	return out
}

func peerJSON(p vpnPeerRow, live map[string]agent.WireGuardPeer, now time.Time) map[string]any {
	subnets := []string{}
	for _, s := range p.Subnets {
		subnets = append(subnets, s.String())
	}
	m := map[string]any{"id": p.ID, "name": p.Name, "kind": p.Kind, "org_id": p.OrgID, "org_name": p.OrgName, "device_id": p.DeviceID, "device_name": p.DeviceName,
		"address": p.Address.String(), "subnets": subnets, "public_key": p.PublicKey, "enabled": p.Enabled, "expires_at": p.ExpiresAt,
		"expired": p.ExpiresAt != nil && !now.Before(*p.ExpiresAt), "active": p.Active(now), "note": p.Note, "created_at": p.CreatedAt, "created_by": p.CreatedBy,
		"updated_at": p.UpdatedAt, "key_rotated_at": p.KeyRotatedAt, "online": false, "rx_bytes": 0, "tx_bytes": 0}
	if l, ok := live[p.PublicKey]; ok {
		m["online"], m["rx_bytes"], m["tx_bytes"], m["endpoint"] = l.Online, l.RxBytes, l.TxBytes, l.Endpoint
		if !l.LatestHandshake.IsZero() {
			m["latest_handshake"] = l.LatestHandshake
		}
	}
	return m
}

func ruleJSON(r vpnRuleRow) map[string]any {
	src := r.Sources
	if src == nil {
		src = []vpn.Source{}
	}
	return map[string]any{"id": r.ID, "name": r.Name, "sources": src, "dest_type": r.DestType, "dest_peer_id": r.DestPeerID, "dest_cidr": r.DestCIDR,
		"protocol": r.Protocol, "ports": r.Ports, "enabled": r.Enabled, "created_at": r.CreatedAt, "created_by": r.CreatedBy, "updated_at": r.UpdatedAt}
}

// GET /api/v1/vpn
func (s *Server) getVPN(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{"agent": map[string]any{"configured": s.agent.Configured(), "connected": false}, "supported": false,
		"licensed": s.vpnLicensed(ctx), "free_limit": vpn.FreePeerLimit, "buy_url": s.cfg.License.BuyURL}
	var st *agent.WireGuardState
	if s.agent.Configured() {
		var err error
		if st, err = s.agent.WireGuard(ctx); err != nil {
			out["agent"] = map[string]any{"configured": true, "connected": false, "error": err.Error()}
			st = nil
		} else {
			out["agent"] = map[string]any{"configured": true, "connected": true}
			out["supported"], out["simulated"] = st.Supported, st.Simulated
			if st.Supported && s.vpnImportLegacy(ctx, st) {
				st, _ = s.agent.WireGuard(ctx)
			}
		}
	}
	live := map[string]agent.WireGuardPeer{}
	if st != nil {
		for _, p := range st.Peers {
			live[p.PublicKey] = p
		}
		iface := *st
		iface.Peers = nil
		out["interface"] = iface
		out["services"] = s.vpnServices(st)
	}
	peers, err := loadVPNPeers(ctx, s.pool)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	rules, err := loadVPNRules(ctx, s.pool)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	now := time.Now()
	pl := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		pl = append(pl, peerJSON(p, live, now))
	}
	rl := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		rl = append(rl, ruleJSON(r))
	}
	out["peers"], out["rules"], out["peers_used"] = pl, rl, len(peers)
	writeJSON(w, 200, out)
}

// ---- applying -------------------------------------------------------------------------------------------------------

type vpnErr struct {
	code int
	msg  string
	key  string // machine-readable code (licence_required, …)
}

func (e *vpnErr) Error() string { return e.msg }

func vpnBad(msg string) *vpnErr { return &vpnErr{code: 400, msg: msg} }

func vpnFail(w http.ResponseWriter, err error) {
	var ve *vpnErr
	var le *vpn.LicenceError
	switch {
	case errors.As(err, &ve):
		if ve.key != "" {
			writeJSON(w, ve.code, map[string]string{"error": ve.msg, "code": ve.key})
			return
		}
		writeErr(w, ve.code, ve.msg)
	case errors.As(err, &le):
		writeJSON(w, 402, map[string]string{"error": le.Msg, "code": "license_required"})
	default:
		writeErr(w, 500, err.Error())
	}
}

// vpnReady returns the agent's VPN state for a change, or answers the request.
func (s *Server) vpnReady(w http.ResponseWriter, r *http.Request) (*agent.WireGuardState, netip.Prefix, bool) {
	st, err := s.agent.WireGuard(r.Context())
	if err != nil {
		s.agentErr(w, err)
		return nil, netip.Prefix{}, false
	}
	if !st.Supported {
		writeErr(w, 501, "the VPN is managed on Linux servers only (Windows: a later version)")
		return nil, netip.Prefix{}, false
	}
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil || st.PublicKey == "" {
		writeErr(w, 409, "configure the VPN server first (address and port) and enable it")
		return nil, netip.Prefix{}, false
	}
	return st, srv, true
}

// vpnPush compiles the state inside tx (uncommitted changes included) and has the agent apply it.
func (s *Server) vpnPush(ctx context.Context, tx dbQuerier, srv netip.Prefix, psk map[string]string) (*agent.WireGuardState, error) {
	peers, err := loadVPNPeers(ctx, tx)
	if err != nil {
		return nil, err
	}
	rules, err := loadVPNRules(ctx, tx)
	if err != nil {
		return nil, err
	}
	d := vpn.Compile(srv, plainPeers(peers), plainRules(rules), time.Now())
	for i := range d.Peers {
		if k, ok := psk[d.Peers[i].ID]; ok {
			d.Peers[i].PresharedKey = k
		}
	}
	return s.agent.ApplyVPN(ctx, d)
}

// vpnChange runs fn in a transaction, applies the result through the agent and commits only when it was applied.
func (s *Server) vpnChange(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error)) (*agent.WireGuardState, bool) {
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	st, srv, ok := s.vpnReady(w, r)
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "database unavailable")
		return nil, false
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	psk, err := fn(ctx, tx, srv, st)
	if err != nil {
		vpnFail(w, err)
		return nil, false
	}
	applied, err := s.vpnPush(ctx, tx, srv, psk)
	if err != nil {
		if errors.Is(err, agent.ErrNoAgent) {
			s.agentErr(w, err)
			return nil, false
		}
		writeErr(w, 502, "the VPN change was not applied, nothing changed: "+err.Error())
		return nil, false
	}
	if err := tx.Commit(ctx); err != nil {
		s.log.Error("vpn commit after apply failed; the next synchronisation restores the stored state", "err", err)
		writeErr(w, 500, "saving failed after the agent applied the change; it is restored within a minute")
		return nil, false
	}
	return applied, true
}

// ---- step-up --------------------------------------------------------------------------------------------------------

// stepUp re-checks the caller's password (and the authenticator code with two-factor sign-in) before a private key is
// shown. Failed attempts are limited per user.
func (s *Server) stepUp(w http.ResponseWriter, r *http.Request, password, code string) bool {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeErr(w, 401, "authentication required")
		return false
	}
	if !s.stepUpLimiter.peek(p.UserID) {
		writeErr(w, 429, "too many failed confirmations; wait a minute")
		return false
	}
	if password == "" {
		writeJSON(w, 403, map[string]string{"error": "confirm with your password (and the code from your authenticator app) to show a configuration", "code": "step_up_required"})
		return false
	}
	if !s.iam.ReauthPassword(r.Context(), p.UserID, password) || (p.MFAEnabled && !s.iam.ReauthTOTP(r.Context(), p.UserID, strings.TrimSpace(code))) {
		s.log.Warn("vpn step-up failed", "security", true, "user", p.Username, "remote", clientIP(r))
		s.stepUpLimiter.allow(p.UserID) // failures are limited (5 a minute per user)
		msg := "wrong password"
		if p.MFAEnabled {
			msg = "wrong password or authenticator code"
		}
		writeJSON(w, 403, map[string]string{"error": msg, "code": "step_up_failed"})
		return false
	}
	return true
}

// ---- peers ----------------------------------------------------------------------------------------------------------

type peerInput struct {
	Name      *string   `json:"name"`
	Kind      string    `json:"kind"`
	OrgID     *string   `json:"org_id"`
	DeviceID  *string   `json:"device_id"`
	Subnets   *[]string `json:"subnets"`
	ExpiresAt *string   `json:"expires_at"` // RFC 3339; "" clears
	Enabled   *bool     `json:"enabled"`
	Note      *string   `json:"note"`
	Reason    string    `json:"reason"`
	Password  string    `json:"password"`
	Code      string    `json:"code"`
}

func parseExpiry(s string, now time.Time) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return nil, vpnBad("expires_at: RFC 3339 date and time, e.g. 2026-10-31T18:00:00+01:00")
	}
	if !t.After(now.Add(time.Minute)) {
		return nil, vpnBad("expires_at must be in the future")
	}
	return &t, nil
}

// resolveOwner checks the organization and device links; a device implies its organization.
func resolveOwner(ctx context.Context, tx pgx.Tx, orgID, deviceID string) (string, string, error) {
	orgID, deviceID = strings.TrimSpace(orgID), strings.TrimSpace(deviceID)
	if deviceID != "" {
		if !uuidRe.MatchString(deviceID) {
			return "", "", vpnBad("device_id is not an id")
		}
		var devOrg string
		if err := tx.QueryRow(ctx, `SELECT org_id::text FROM core.device WHERE id = $1::uuid AND revoked_at IS NULL`, deviceID).Scan(&devOrg); err != nil {
			return "", "", vpnBad("device not found")
		}
		if orgID == "" {
			orgID = devOrg
		} else if orgID != devOrg {
			return "", "", vpnBad("the device belongs to another organization")
		}
	}
	if orgID != "" {
		if !uuidRe.MatchString(orgID) {
			return "", "", vpnBad("org_id is not an id")
		}
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&one); err != nil {
			return "", "", vpnBad("organization not found")
		}
	}
	return orgID, deviceID, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// clientConfig builds what the peer's owner gets once: the configuration, its QR code and, for a site, router texts.
func (s *Server) clientConfig(st *agent.WireGuardState, srv netip.Prefix, p vpn.Peer, priv, psk string, all []vpn.Peer) map[string]any {
	c := vpn.Client{Name: p.Name, Kind: p.Kind, PrivateKey: priv, Address: p.Address.String(), VPNNet: srv.Masked().String(), VPNBits: srv.Bits(), DNS: st.DNS,
		ServerPublicKey: st.PublicKey, PresharedKey: psk, Endpoint: st.Endpoint, AllowedIPs: vpn.ClientAllowedIPs(srv, p.Kind, p.ID, all)}
	for _, sn := range p.Subnets {
		c.Subnets = append(c.Subnets, sn.String())
	}
	conf := vpn.ClientConf(c)
	out := map[string]any{"config": conf, "allowed_ips": c.AllowedIPs, "server": srv.Addr().String()}
	if png, err := qrcode.Encode(conf, qrcode.Medium, 360); err == nil {
		out["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	if p.Kind == vpn.KindSite {
		out["templates"] = vpn.RouterTemplates(c)
	}
	return out
}

func peerAudit(p vpn.Peer, note string) map[string]any {
	sub := []string{}
	for _, s := range p.Subnets {
		sub = append(sub, s.String())
	}
	return map[string]any{"name": p.Name, "kind": p.Kind, "org_id": p.OrgID, "device_id": p.DeviceID, "address": p.Address.String(), "subnets": sub,
		"enabled": p.Enabled, "expires_at": p.ExpiresAt, "public_key": p.PublicKey, "note": note}
}

// POST /api/v1/vpn/peers
func (s *Server) createVPNPeer(w http.ResponseWriter, r *http.Request) {
	pr := PrincipalFrom(r.Context())
	var in peerInput
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if err := vpn.ValidName(name); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !vpn.ValidKind(in.Kind) {
		writeErr(w, 400, "kind must be device, user or site")
		return
	}
	var subnets []string
	if in.Subnets != nil {
		subnets = *in.Subnets
	}
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	if len([]rune(note)) > 200 {
		writeErr(w, 400, "note: at most 200 characters")
		return
	}
	if !s.stepUp(w, r, in.Password, in.Code) {
		return
	}
	licensed := s.vpnLicensed(r.Context())
	var created vpn.Peer
	var priv, psk string
	var all []vpn.Peer
	st, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		rows, err := loadVPNPeers(ctx, tx)
		if err != nil {
			return nil, err
		}
		exp := ""
		if in.ExpiresAt != nil {
			exp = *in.ExpiresAt
		}
		expires, err := parseExpiry(exp, time.Now())
		if err != nil {
			return nil, err
		}
		if err := vpn.CheckAdd(licensed, len(rows), in.Kind, expires != nil); err != nil {
			return nil, err
		}
		p := vpn.Peer{Name: name, Kind: in.Kind, Enabled: true, ExpiresAt: expires}
		for _, o := range rows {
			if strings.EqualFold(o.Name, name) {
				return nil, vpnBad("a peer with that name exists")
			}
		}
		if in.Kind == vpn.KindSite {
			if p.Subnets, err = vpn.ParseSubnets(subnets, srv); err != nil {
				return nil, vpnBad(err.Error())
			}
			if len(p.Subnets) == 0 {
				return nil, vpnBad("a site needs at least one routed subnet (the LAN behind the router, e.g. 192.168.10.0/24)")
			}
			if err := vpn.CheckSubnetConflicts(p.Subnets, "", plainPeers(rows)); err != nil {
				return nil, vpnBad(err.Error())
			}
		} else if len(subnets) > 0 {
			return nil, vpnBad("only site peers have routed subnets")
		}
		org, dev := "", ""
		if in.OrgID != nil {
			org = *in.OrgID
		}
		if in.DeviceID != nil {
			dev = *in.DeviceID
		}
		if p.OrgID, p.DeviceID, err = resolveOwner(ctx, tx, org, dev); err != nil {
			return nil, err
		}
		used := map[netip.Addr]bool{}
		for _, o := range rows {
			used[o.Address] = true
		}
		if p.Address, err = vpn.NextAddress(srv, used); err != nil {
			return nil, vpnBad(err.Error())
		}
		if priv, p.PublicKey, err = vpn.GenerateKey(); err != nil {
			return nil, err
		}
		if psk, err = vpn.GeneratePSK(); err != nil {
			return nil, err
		}
		sub := make([]string, len(p.Subnets))
		for i, x := range p.Subnets {
			sub[i] = x.String()
		}
		if err := tx.QueryRow(ctx, `INSERT INTO core.vpn_peer (name, kind, org_id, device_id, address, public_key, subnets, expires_at, note, created_by)
			VALUES ($1, $2, $3::uuid, $4::uuid, $5::inet, $6, $7::cidr[], $8, $9, $10) RETURNING id::text`,
			p.Name, p.Kind, nullStr(p.OrgID), nullStr(p.DeviceID), p.Address.String()+"/32", p.PublicKey, sub, p.ExpiresAt, note, pr.Username).Scan(&p.ID); err != nil {
			return nil, err
		}
		created = p
		all = append(plainPeers(rows), p)
		return map[string]string{p.ID: psk}, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.peer_add", "vpn_peer", created.ID, nil, peerAudit(created, note), in.Reason)
	srv, _ := vpn.ServerPrefix(st.Address)
	out := s.clientConfig(st, srv, created, priv, psk, all)
	out["peer"] = peerJSON(vpnPeerRow{Peer: created, Note: note, CreatedAt: time.Now(), CreatedBy: pr.Username, UpdatedAt: time.Now()}, nil, time.Now())
	writeJSON(w, 201, out)
}

func findPeer(rows []vpnPeerRow, id string) (vpnPeerRow, bool) {
	for _, p := range rows {
		if p.ID == id {
			return p, true
		}
	}
	return vpnPeerRow{}, false
}

// PATCH /api/v1/vpn/peers/{id}
func (s *Server) patchVPNPeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid peer id")
		return
	}
	var in peerInput
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	licensed := s.vpnLicensed(r.Context())
	var before, after map[string]any
	_, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		rows, err := loadVPNPeers(ctx, tx)
		if err != nil {
			return nil, err
		}
		cur, found := findPeer(rows, id)
		if !found {
			return nil, &vpnErr{code: 404, msg: "peer not found"}
		}
		before = peerAudit(cur.Peer, cur.Note)
		p, note := cur.Peer, cur.Note
		if in.Name != nil {
			n := strings.TrimSpace(*in.Name)
			if err := vpn.ValidName(n); err != nil {
				return nil, vpnBad(err.Error())
			}
			for _, o := range rows {
				if o.ID != id && strings.EqualFold(o.Name, n) {
					return nil, vpnBad("a peer with that name exists")
				}
			}
			p.Name = n
		}
		if in.Note != nil {
			note = strings.TrimSpace(*in.Note)
			if len([]rune(note)) > 200 {
				return nil, vpnBad("note: at most 200 characters")
			}
		}
		subnetsChanged := false
		if in.Subnets != nil {
			if p.Kind != vpn.KindSite {
				if len(*in.Subnets) > 0 {
					return nil, vpnBad("only site peers have routed subnets")
				}
			} else {
				ns, err := vpn.ParseSubnets(*in.Subnets, srv)
				if err != nil {
					return nil, vpnBad(err.Error())
				}
				if len(ns) == 0 {
					return nil, vpnBad("a site needs at least one routed subnet")
				}
				if err := vpn.CheckSubnetConflicts(ns, id, plainPeers(rows)); err != nil {
					return nil, vpnBad(err.Error())
				}
				subnetsChanged = fmt.Sprint(ns) != fmt.Sprint(p.Subnets)
				p.Subnets = ns
			}
		}
		expirySet := false
		if in.ExpiresAt != nil {
			e, err := parseExpiry(*in.ExpiresAt, time.Now())
			if err != nil {
				return nil, err
			}
			expirySet = e != nil && (p.ExpiresAt == nil || !e.Equal(*p.ExpiresAt))
			p.ExpiresAt = e
		}
		if err := vpn.CheckEdit(licensed, subnetsChanged, expirySet); err != nil {
			return nil, err
		}
		if in.Enabled != nil {
			p.Enabled = *in.Enabled
		}
		if p.Enabled && p.ExpiresAt != nil && !time.Now().Before(*p.ExpiresAt) {
			return nil, vpnBad("the access has expired: set a new expiry date or clear it to enable the peer again")
		}
		if in.OrgID != nil || in.DeviceID != nil {
			org, dev := p.OrgID, p.DeviceID
			if in.OrgID != nil {
				org = *in.OrgID
			}
			if in.DeviceID != nil {
				dev = *in.DeviceID
			}
			if p.OrgID, p.DeviceID, err = resolveOwner(ctx, tx, org, dev); err != nil {
				return nil, err
			}
		}
		sub := make([]string, len(p.Subnets))
		for i, x := range p.Subnets {
			sub[i] = x.String()
		}
		if _, err := tx.Exec(ctx, `UPDATE core.vpn_peer SET name = $2, subnets = $3::cidr[], expires_at = $4, enabled = $5, org_id = $6::uuid, device_id = $7::uuid, note = $8, updated_at = now()
			WHERE id = $1::uuid`, id, p.Name, sub, p.ExpiresAt, p.Enabled, nullStr(p.OrgID), nullStr(p.DeviceID), note); err != nil {
			return nil, err
		}
		after = peerAudit(p, note)
		return nil, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.peer_update", "vpn_peer", id, before, after, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "peer": after})
}

// POST /api/v1/vpn/peers/{id}/rotate
func (s *Server) rotateVPNPeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid peer id")
		return
	}
	var in struct{ Reason, Password, Code string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) || !s.stepUp(w, r, in.Password, in.Code) {
		return
	}
	var p vpn.Peer
	var priv, psk, oldKey string
	var all []vpn.Peer
	st, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		rows, err := loadVPNPeers(ctx, tx)
		if err != nil {
			return nil, err
		}
		cur, found := findPeer(rows, id)
		if !found {
			return nil, &vpnErr{code: 404, msg: "peer not found"}
		}
		p, oldKey = cur.Peer, cur.PublicKey
		if priv, p.PublicKey, err = vpn.GenerateKey(); err != nil {
			return nil, err
		}
		if psk, err = vpn.GeneratePSK(); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE core.vpn_peer SET public_key = $2, key_rotated_at = now(), updated_at = now() WHERE id = $1::uuid`, id, p.PublicKey); err != nil {
			return nil, err
		}
		for _, o := range rows {
			if o.ID != id {
				all = append(all, o.Peer)
			}
		}
		all = append(all, p)
		return map[string]string{id: psk}, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.peer_rotate", "vpn_peer", id, map[string]any{"public_key": oldKey}, map[string]any{"public_key": p.PublicKey, "name": p.Name}, in.Reason)
	srv, _ := vpn.ServerPrefix(st.Address)
	out := s.clientConfig(st, srv, p, priv, psk, all)
	out["peer"] = map[string]any{"id": p.ID, "name": p.Name, "public_key": p.PublicKey, "address": p.Address.String()}
	writeJSON(w, 200, out)
}

// POST /api/v1/vpn/peers/{id}/remove: the peer and every rule that names it as the destination are removed; it is
// dropped from the sources of other rules (a rule left without sources is removed too).
func (s *Server) removeVPNPeer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid peer id")
		return
	}
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if !reasonOf(w, in.Reason) {
		return
	}
	pr := PrincipalFrom(r.Context())
	var name string
	var rulesRemoved, rulesChanged int
	_, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		if err := tx.QueryRow(ctx, `UPDATE core.vpn_peer SET removed_at = now(), removed_by = $2, enabled = false WHERE id = $1::uuid AND removed_at IS NULL RETURNING name`, id, pr.Username).Scan(&name); err != nil {
			return nil, &vpnErr{code: 404, msg: "peer not found"}
		}
		rules, err := loadVPNRules(ctx, tx)
		if err != nil {
			return nil, err
		}
		for _, ru := range rules {
			kept := []vpn.Source{}
			for _, src := range ru.Sources {
				if !(src.Type == vpn.SrcPeer && src.ID == id) {
					kept = append(kept, src)
				}
			}
			switch {
			case ru.DestPeerID == id || len(kept) == 0:
				if _, err := tx.Exec(ctx, `UPDATE core.vpn_rule SET removed_at = now(), removed_by = $2 WHERE id = $1::uuid`, ru.ID, pr.Username); err != nil {
					return nil, err
				}
				rulesRemoved++
			case len(kept) != len(ru.Sources):
				b, _ := json.Marshal(kept)
				if _, err := tx.Exec(ctx, `UPDATE core.vpn_rule SET sources = $2::jsonb, updated_at = now() WHERE id = $1::uuid`, ru.ID, string(b)); err != nil {
					return nil, err
				}
				rulesChanged++
			}
		}
		return nil, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.peer_remove", "vpn_peer", id, map[string]any{"name": name}, map[string]any{"rules_removed": rulesRemoved, "rules_changed": rulesChanged}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "rules_removed": rulesRemoved, "rules_changed": rulesChanged})
}

// ---- rules ----------------------------------------------------------------------------------------------------------

type ruleInput struct {
	Name       string       `json:"name"`
	Sources    []vpn.Source `json:"sources"`
	DestType   string       `json:"dest_type"`
	DestPeerID string       `json:"dest_peer_id"`
	DestCIDR   string       `json:"dest_cidr"`
	Protocol   string       `json:"protocol"`
	Ports      string       `json:"ports"`
	Enabled    *bool        `json:"enabled"`
	Reason     string       `json:"reason"`
}

func (s *Server) saveVPNRule(w http.ResponseWriter, r *http.Request, id string) {
	var in ruleInput
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	pr := PrincipalFrom(r.Context())
	rule := vpn.Rule{ID: id, Name: strings.TrimSpace(in.Name), Sources: in.Sources, DestType: in.DestType, DestPeerID: strings.TrimSpace(in.DestPeerID),
		DestCIDR: strings.TrimSpace(in.DestCIDR), Protocol: in.Protocol, Ports: in.Ports, Enabled: in.Enabled == nil || *in.Enabled}
	var before any
	_, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		rows, err := loadVPNPeers(ctx, tx)
		if err != nil {
			return nil, err
		}
		peers := map[string]vpn.Peer{}
		for _, p := range rows {
			peers[p.ID] = p.Peer
		}
		if err := vpn.ValidateRule(&rule, peers); err != nil {
			return nil, vpnBad(err.Error())
		}
		for _, src := range rule.Sources {
			if src.Type == vpn.SrcOrg {
				var one int
				if !uuidRe.MatchString(src.ID) || tx.QueryRow(ctx, `SELECT 1 FROM core.organization WHERE id = $1::uuid`, src.ID).Scan(&one) != nil {
					return nil, vpnBad("source organization not found")
				}
			}
		}
		src, _ := json.Marshal(rule.Sources)
		if id == "" {
			n := 0
			_ = tx.QueryRow(ctx, `SELECT count(*) FROM core.vpn_rule WHERE removed_at IS NULL`).Scan(&n)
			if n >= vpn.MaxRules {
				return nil, vpnBad(fmt.Sprintf("at most %d access rules", vpn.MaxRules))
			}
			return nil, tx.QueryRow(ctx, `INSERT INTO core.vpn_rule (name, sources, dest_type, dest_peer_id, dest_cidr, protocol, ports, enabled, created_by)
				VALUES ($1, $2::jsonb, $3, $4::uuid, $5::cidr, $6, $7, $8, $9) RETURNING id::text`,
				rule.Name, string(src), rule.DestType, nullStr(rule.DestPeerID), nullStr(rule.DestCIDR), rule.Protocol, rule.Ports, rule.Enabled, pr.Username).Scan(&rule.ID)
		}
		rules, err := loadVPNRules(ctx, tx)
		if err != nil {
			return nil, err
		}
		found := false
		for _, ru := range rules {
			if ru.ID == id {
				found, before = true, ruleJSON(ru)
			}
		}
		if !found {
			return nil, &vpnErr{code: 404, msg: "rule not found"}
		}
		_, err = tx.Exec(ctx, `UPDATE core.vpn_rule SET name = $2, sources = $3::jsonb, dest_type = $4, dest_peer_id = $5::uuid, dest_cidr = $6::cidr, protocol = $7, ports = $8, enabled = $9, updated_at = now()
			WHERE id = $1::uuid`, id, rule.Name, string(src), rule.DestType, nullStr(rule.DestPeerID), nullStr(rule.DestCIDR), rule.Protocol, rule.Ports, rule.Enabled)
		return nil, err
	})
	if !ok {
		return
	}
	after := map[string]any{"name": rule.Name, "sources": rule.Sources, "dest_type": rule.DestType, "dest_peer_id": rule.DestPeerID, "dest_cidr": rule.DestCIDR,
		"protocol": rule.Protocol, "ports": rule.Ports, "enabled": rule.Enabled}
	if id == "" {
		s.audit(r, "vpn.rule_add", "vpn_rule", rule.ID, nil, after, in.Reason)
		after["id"] = rule.ID
		writeJSON(w, 201, after)
		return
	}
	s.audit(r, "vpn.rule_update", "vpn_rule", id, before, after, in.Reason)
	after["id"] = id
	writeJSON(w, 200, after)
}

// POST /api/v1/vpn/rules
func (s *Server) createVPNRule(w http.ResponseWriter, r *http.Request) { s.saveVPNRule(w, r, "") }

// PUT /api/v1/vpn/rules/{id}
func (s *Server) putVPNRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 400, "invalid rule id")
		return
	}
	s.saveVPNRule(w, r, id)
}

// POST /api/v1/vpn/rules/{id}/remove
func (s *Server) removeVPNRule(w http.ResponseWriter, r *http.Request) {
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
	_, ok := s.vpnChange(w, r, func(ctx context.Context, tx pgx.Tx, srv netip.Prefix, st *agent.WireGuardState) (map[string]string, error) {
		if err := tx.QueryRow(ctx, `UPDATE core.vpn_rule SET removed_at = now(), removed_by = $2 WHERE id = $1::uuid AND removed_at IS NULL RETURNING name`, id, pr.Username).Scan(&name); err != nil {
			return nil, &vpnErr{code: 404, msg: "rule not found"}
		}
		return nil, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.rule_remove", "vpn_rule", id, map[string]any{"name": name}, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/v1/vpn/apply
func (s *Server) applyVPN(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if !reasonOf(w, in.Reason) {
		return
	}
	st, ok := s.vpnChange(w, r, func(context.Context, pgx.Tx, netip.Prefix, *agent.WireGuardState) (map[string]string, error) {
		return nil, nil
	})
	if !ok {
		return
	}
	s.audit(r, "vpn.apply", "server", "vpn", nil, map[string]any{"peers": len(st.Peers), "rules": st.Firewall.Rules, "forward": st.Firewall.Forward}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "firewall": st.Firewall})
}

// ---- background: expiry, synchronisation, import --------------------------------------------------------------------

func (s *Server) vpnLoop() {
	time.Sleep(15 * time.Second)
	for i := 0; ; i++ {
		s.vpnTick(i%10 == 0) // the full state every five minutes heals an agent that lost it
		time.Sleep(30 * time.Second)
	}
}

// vpnTick disables peers whose access expired (audited) and pushes the state when something changed or force is set.
func (s *Server) vpnTick(force bool) {
	if !s.agent.Configured() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	st, err := s.agent.WireGuard(ctx)
	if err != nil || !st.Supported || st.PublicKey == "" {
		return
	}
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return
	}
	if s.vpnImportLegacy(ctx, st) {
		return
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	rows, err := s.pool.Query(ctx, `UPDATE core.vpn_peer SET enabled = false, updated_at = now()
		WHERE removed_at IS NULL AND enabled AND expires_at IS NOT NULL AND expires_at <= now() RETURNING id::text, name, coalesce(org_id::text, '')`)
	if err != nil {
		return
	}
	type exp struct{ id, name, org string }
	var expired []exp
	for rows.Next() {
		var e exp
		if rows.Scan(&e.id, &e.name, &e.org) == nil {
			expired = append(expired, e)
		}
	}
	rows.Close()
	for _, e := range expired {
		_, _, _ = audit.Append(ctx, s.pool, audit.Entry{OrgID: e.org, ActorType: "system", ActorName: "vpn expiry", Action: "vpn.peer_expire", EntityType: "vpn_peer", EntityID: e.id,
			New: map[string]any{"name": e.name, "enabled": false}, Reason: "time-limited access expired"})
		s.log.Info("vpn access expired", "peer", e.name)
	}
	if len(expired) == 0 && !force {
		return
	}
	if _, err := s.vpnPush(ctx, s.pool, srv, nil); err != nil {
		s.log.Warn("vpn synchronisation failed", "err", err)
	}
}

// vpnImportLegacy adopts the peers of an earlier version (kept by the agent only) once: they become device peers of
// the whole server, and one access rule keeps what they did before — MQTT to the server. Everything else from the
// VPN is denied from now on. Returns whether it imported.
func (s *Server) vpnImportLegacy(ctx context.Context, st *agent.WireGuardState) bool {
	if len(st.Peers) == 0 {
		return false
	}
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return false
	}
	s.vpnMu.Lock()
	defer s.vpnMu.Unlock()
	var n int
	if s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM core.vpn_peer) + (SELECT count(*) FROM core.vpn_rule)`).Scan(&n) != nil || n > 0 {
		return false
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	names := []string{}
	for _, p := range st.Peers {
		addr := strings.TrimSuffix(p.Address, "/32")
		a, err := netip.ParseAddr(addr)
		if err != nil || !vpn.ValidKey(p.PublicKey) || vpn.ValidName(p.Name) != nil || !srv.Masked().Contains(a) {
			s.log.Warn("vpn import: peer skipped", "name", p.Name)
			continue
		}
		created := p.CreatedAt
		if created.IsZero() {
			created = time.Now()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO core.vpn_peer (name, kind, address, public_key, created_at, created_by, note) VALUES ($1, 'device', $2::inet, $3, $4, $5, 'imported from the earlier WireGuard setup')`,
			p.Name, addr+"/32", p.PublicKey, created, p.CreatedBy); err != nil {
			s.log.Warn("vpn import failed", "err", err)
			return false
		}
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		return false
	}
	if _, err := tx.Exec(ctx, `INSERT INTO core.vpn_rule (name, sources, dest_type, protocol, ports, created_by) VALUES ('MQTT for devices', '[{"type":"kind","kind":"device"}]', 'server', 'tcp', '1883', 'import')`); err != nil {
		return false
	}
	if _, err := s.vpnPush(ctx, tx, srv, nil); err != nil {
		s.log.Warn("vpn import: the agent did not apply the imported state", "err", err)
		return false
	}
	if tx.Commit(ctx) != nil {
		return false
	}
	_, _, _ = audit.Append(ctx, s.pool, audit.Entry{ActorType: "system", ActorName: "vpn import", Action: "vpn.import", EntityType: "server:vpn",
		New: map[string]any{"peers": names, "rule": "MQTT for devices (tcp 1883 to the server)"}, Reason: "upgrade to VPN management with access rules (default deny)"})
	s.log.Info("vpn: imported peers of the earlier WireGuard setup", "peers", len(names))
	return true
}
