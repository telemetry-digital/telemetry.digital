# Feature Specification: VPN phase 3 — uplink of a customer server to a hub

**Feature Branch**: `feature/0.69-uplink`

**Created**: 2026-10-02

**Status**: Implemented (0.69.0), not merged, not deployed

**Input**: The owner (2 Oct 2026): "most customer servers sit in a customer LAN without a public address, so
technicians cannot reach them through the server's own VPN hub. Phase 3 = a central hub in the cloud (e.g. our
portal.telemetry.digital server, or any telemetry server with a public address)." On the hub a remote telemetry
server is registered as a site-like peer with its routed LAN subnets and gets an uplink configuration once; the
customer server pastes it, brings up a second, outbound-only interface and applies its own default-deny firewall to
traffic from the hub; technicians reach the customer server and its LAN as the rules on both sides allow. If UDP is
blocked, evaluate WireGuard over TCP/443 and implement it only if clean and secure. Licence: the uplink counts as a
site. SAFETY FIRST: off by default, `system.admin` on the customer server, reason and audit, default deny both sides.

**Related**: ADR-0021 (decision), ADR-0018 (VPN model), ADR-0020 (Windows userspace), spec 024, spec 025,
`docs/vpn.md`, threat model.

## Clarifications (decisions and defaults)

- The uplink is a `site` peer with a flag (`uplink`) on the hub — licence, rules, grants, organizations as for sites;
  routed subnets optional (an uplink may offer only the server itself).
- The configuration is a single pasteable token (`ctrl32-uplink-v1.…`), shown once after the password again; the hub
  keeps no private key. Rotation on the hub issues a new token (the old one, including its HTTPS secret, stops at once);
  the customer replaces its configuration with it (rules stay).
- The customer side: `system.admin` only (not `vpn.manage`), the password again to connect or replace, a reason on
  every change. Uplink rules are a separate list (from the hub's range or a part of it, to this server or the LAN).
- LAN access from the hub is masqueraded behind the customer server's LAN address (LAN devices need no route back).
- HTTPS transport: implemented as WireGuard datagrams in a WebSocket through the hub's HTTPS address (both ends in our
  binary); offered only when the hub has an HTTPS base URL.
- No licence check on the customer side (the hub's site needs it).

## User Scenarios

1. **Cloud hub, customer A.** The integrator's cloud server (`10.99.0.1/16`, licence) adds *Server (uplink)*
   "Customer A" with `192.168.50.0/24` and copies the token. On customer A's server (a Windows or Linux box in the
   plant LAN, no public address) the administrator opens *System → VPN → Connect to a hub*, pastes it, enters a reason
   and the password: the uplink shows *connected*. Uplink rules: *the whole hub → this server : 443*, *technician
   10.99.0.21 → PLC 192.168.50.20 : 502*. On the hub: *technicians → Customer A : 443*, *technician → 192.168.50.20 :
   502*. The technician opens the customer's web interface and programs the PLC; another technician reaches the web
   interface but not the PLC; another LAN host is not reachable.
2. **UDP blocked.** The plant firewall allows only HTTPS out. The administrator switches the transport to HTTPS; the
   uplink connects through the hub's web address.
3. **Rotation.** The hub's administrator suspects the token leaked and rotates the key; the uplink stops at once; the
   customer's administrator pastes the new token (*Replace configuration*).
4. **The customer ends it.** *Disconnect*: the interface and the firewall table are removed and the keys deleted; the
   uplink rules stay for later.

## Requirements

- FR-001 Migration 0047 (applied migrations untouched): `vpn_peer.uplink` (only for sites), `vpn_peer.tunnel_key_hash`
  (SHA-256, unique among active peers), `core.vpn_uplink_rule`.
- FR-002 Hub: `POST /api/v1/vpn/peers` with `uplink: true` (kind `site`, subnets optional, the hub's endpoint must be
  set) and rotation return `uplink_token` instead of `config`/QR/router texts; the token holds the HTTPS transport when
  the base URL is https; uplink peers route only the hub's VPN range on the customer side; `uplink` and `via_https` in
  the peer list; disabling, rotating or removing an uplink closes its HTTPS relay.
- FR-003 Hub: `GET /api/v1/vpn/uplink/tunnel` — WebSocket relay to the local WireGuard port, bearer secret of an
  enabled, unexpired uplink peer, 4 KiB frames, 1 000 relays, idle close, one relay per uplink.
- FR-004 Customer: `POST /api/v1/vpn/uplink` (token, transport, step-up, reason), `…/transport`, `…/disconnect`,
  `…/rules` (create, change, remove); `GET /api/v1/vpn` shows `uplink`, `uplink_rules`, `uplink_services`,
  `uplink_https` to server administrators only; every change audited; the rules are pushed again every five minutes.
- FR-005 Agent: `PUT /uplink`, `PUT /uplink/rules`, `PUT /uplink/transport`, `DELETE /uplink`; validation of the token
  and the rules again; refusal of overlapping ranges both ways and of a foreign `wg1.conf`; Linux: `wg1` +
  `inet ctrl32_uplink` (input, forward, masquerade, LAN guard), forwarding only as needed, rollback, ufw, broker
  `uplink_listen`; Windows: userspace uplink (relay to 127.0.0.1 and to LAN hosts, TCP); HTTPS transport client with
  reconnect and an endpoint watch; state with handshake, traffic, transport, tunnel, firewall.
- FR-006 UI (*System → VPN*): *Server (uplink)* as a fourth kind with the token result; the card *Uplink to a hub*
  (state, hub, transport, address, ranges, offered networks, handshake, traffic, firewall, forwarding) with *Connect*,
  *Firewall rules*, *Transport*, *Replace configuration*, *Disconnect*; the uplink rules table and dialog with presets.
- FR-007 Translations of every new string in 19 catalogs; OpenAPI and clients regenerated; MCP: connecting excluded
  (private key, step-up), the tunnel excluded (not a user API).

## Success Criteria

- SC-001 Unit: `vpn` (token round trip and refusals, local overlap, rule validation, compilation, rendering, allowed
  IPs), `agent` (kernel connect/rules/transport/disconnect order and files, refusals, rollback, router host, userspace
  with a fake device, simulation, endpoint watch), `wghub` (uplink filter; end-to-end with real WireGuard: rules on both
  sides, HTTPS transport with reconnect), `httpapi` (routes and permissions, OpenAPI, MCP).
- SC-002 e2e `vpn3` against the agent's simulation; `vpn`, `vpn2`, `openapi`, `mcp` stay green.
- SC-003 Linux: real kernel WireGuard and nftables in privileged containers (hub, customer, technicians, PLCs).
- SC-004 Not verifiable here: a real cloud hub behind Caddy over the internet; the Windows uplink on a real server.
