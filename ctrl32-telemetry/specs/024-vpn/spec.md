# Feature Specification: Own WireGuard VPN for sites, technicians and devices (phase 1)

**Feature Branch**: `feature/vpn`

**Created**: 2026-10-02

**Status**: Implemented (0.64.0), not yet deployed

**Input**: The owner (2 Oct 2026) after a research of Netmaker and alternatives: "VPN phase 1 … our own WireGuard network
management (Netmaker-like, but our own code)". Goals: technician access to PLC/HMI at sites; site-to-site (a router or
Raspberry Pi at a site connects its whole LAN); devices sending data over the VPN (MQTT through the tunnel); a general
company VPN. "Basic is free, more with the licence." Linux first; Windows phase 2; cloud hub phase 3. SAFETY FIRST: off
by default, behind permission, every change with a reason and audited, access rules default deny.

**Related**: ADR-0018 (decision), ADR-0012 (server agent), ADR-0016/0017 (license), `docs/vpn.md`, threat model.

## Clarifications (owner's decisions and defaults)

- Free: the VPN with up to 5 peers of kinds device and user. With the license (the existing license feature, no prices
  anywhere): unlimited peers, site peers with routed subnets, time-limited technician access. Without the license
  existing peers keep working; adding beyond the limit is refused with a clear message.
- Hub and spoke; no mesh, no NAT traversal, IPv4 only.
- Access rules are free (restricting access is never paid).
- A configuration with a private key is shown once, after the password (and the authenticator code with two-factor
  sign-in) is confirmed again; it is never stored.
- Management needs `system.admin`. Organizations are an attribute of a peer and a rule source; org-scoped management is
  left for later (ADR-0018 decision 9).
- Phase 0 first: what listens on the VPN address and on all addresses of a production-like install, and close what
  peers must not reach (`docs/vpn.md`, *What a peer can reach*).

## User Scenarios

1. **Technician to a PLC.** The administrator adds the site *plant-1* (a MikroTik router, LAN `192.168.10.0/24`),
   configures the router from the RouterOS text, adds the technician *novak* (QR code on the phone or the `.conf` on the
   laptop) and the rule *all technicians → plant-1 192.168.10.20 : tcp 102*. The technician opens TIA Portal and reaches
   the PLC — nothing else at the site, nothing on the server.
2. **Contractor for a day.** A technician peer with an expiry at 18:00; at 18:00 it is disabled automatically and the
   audit trail says so.
3. **Gateway without TLS.** A device peer for a gateway; rule *all devices → this server : MQTT 1883*.
4. **Revocation.** *Disable* ends the access at once and keeps the key; *Rotate key* issues a new configuration and the
   old key stops working; *Remove* removes the peer and the rules towards it.
5. **Free limit.** On a server without the license the sixth peer, a site and an expiry are refused with a message
   pointing to *System → License*; existing peers keep working.

## Requirements

- FR-001 Data model (migration 0044, applied migrations untouched): `core.vpn_peer` (name, kind device|user|site,
  organization nullable, device nullable, address /32, public key, routed subnets `cidr[]` for sites only, enabled,
  expiry, note, created/updated, key rotation time, removal) and `core.vpn_rule` (name, sources JSON, destination
  server|peer|site with peer and optional CIDR, protocol tcp|udp|icmp|any, ports, enabled, removal); unique address,
  name and key among peers not removed; rows never deleted.
- FR-002 Validation: names; subnets IPv4, canonical network address, /8–/32, not reserved, not overlapping the VPN range,
  each other or other sites; ports 1–65535 and ranges, no overlaps, at most 20 entries; destination CIDR inside the
  site's subnets; expiry in the future.
- FR-003 License: free ≤ 5 peers of kinds device and user without an expiry; site peers, expiry, more peers and new
  routed subnets need the license (402 `license_required`); renaming, disabling, enabling, clearing an expiry, rotating
  and removing never do.
- FR-004 Access rules, default deny, compiled into an nftables table `inet ctrl32_vpn` (input: towards the server's VPN
  address; forward: towards peers and site subnets); only active peers (enabled, not expired) appear; a site as a source
  includes its subnets; forwarding on `wg0` only while a forward rule exists; routes for site subnets through `wg0`.
- FR-005 Agent operation `PUT /vpn` with the complete state; the agent validates again (including overlaps with the
  host's networks), checks the table with `nft -c`, applies firewall, peers, routes and forwarding, and restores the
  previous state on any failure. Preshared keys stay in the agent; keys are generated in Go.
- FR-006 API (`system.admin`, reason on every change, audit): `GET /api/v1/vpn`, `POST /api/v1/vpn/peers`,
  `PATCH /api/v1/vpn/peers/{id}`, `POST /api/v1/vpn/peers/{id}/rotate`, `POST /api/v1/vpn/peers/{id}/remove`,
  `POST /api/v1/vpn/rules`, `PUT /api/v1/vpn/rules/{id}`, `POST /api/v1/vpn/rules/{id}/remove`, `POST /api/v1/vpn/apply`;
  `PUT /api/v1/server/wireguard` stays for the interface; the old `/api/v1/server/wireguard/peers…` operations are
  replaced. A change commits only when the agent applied it.
- FR-007 Step-up: creating a peer and rotating a key need the password again (and the TOTP code with two-factor
  sign-in); five failed confirmations a minute per user; both operations are not offered to AI assistants (MCP).
- FR-008 Expiry: a background job disables expired peers (audited `vpn.peer_expire`, actor *system*) and pushes the
  state; the full state is pushed every five minutes.
- FR-009 UI *System → VPN* (`/server/vpn`; `/server/wireguard` redirects): server state and firewall, license usage,
  peers table (kind, organization, address and subnets, expiry, state, handshake, traffic; edit, disable/enable, rotate,
  remove), add-peer dialog per kind with the configuration and QR shown once, router templates for sites (wg-quick,
  OpenWrt, MikroTik RouterOS 7, text only), rules editor with server service presets, generated firewall rules, server
  settings; translations in 19 catalogs.
- FR-010 Upgrade: peers of earlier versions are imported once as device peers with the rule *MQTT for devices*.
- FR-011 Development: `[agent] vpn_simulate` renders files without applying anything and serves only the VPN operations.

## Success Criteria

- SC-001 Unit (`internal/vpn`): keys (RFC 7748 vector), subnets, ports, licence, address allocation, rule validation,
  compilation, desired-state validation (injection, outside ranges, host networks), nftables and configuration
  rendering, router templates. Unit (`internal/agent`, fake runner): apply order, preshared key kept, forwarding,
  rollback on a failed `wg syncconf`, refused `nft -c`, validation before touching the host, files only while disabled,
  simulation.
- SC-002 e2e `vpn`: pages and access, validation, step-up, free limit and licensed capabilities, rules in the generated
  table, disable/enable/rotate, removal with rule clean-up, audit; skips gracefully without an agent.
- SC-003 Headless `vpn`: add a technician through the dialog (config and QR), a rule with a preset, firewall rules,
  router templates (licensed), Slovak; screenshots.
- SC-004 Not verifiable on the development PC: real nftables, `wg syncconf`, routes and forwarding on Linux — to be
  checked on a Linux server before production (`docs/vpn.md`, *How it is enforced*).
