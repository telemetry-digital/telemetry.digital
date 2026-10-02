# ADR-0018: Own WireGuard VPN management — hub and spoke, access rules with default deny, nftables

**Amended by ADR-0020** (2 Oct 2026): Windows servers run a userspace WireGuard hub in the agent that enforces the same
access rules in-process.

**Status:** accepted 2 Oct 2026 (owner: "VPN phase 1 … our own WireGuard network management, Netmaker-like but our own
code"; goals: technician access to PLC/HMI at sites, site-to-site through a router or Raspberry Pi, devices sending data
through the tunnel, a general company VPN; "basic is free, more with the licence"; Linux first, Windows phase 2, cloud
hub phase 3; SAFETY FIRST). Amends ADR-0012 (decision 5: WireGuard through the agent) and ADR-0016/ADR-0017 (a further
capability of the same paid license). Research: comparison with Netmaker, Headscale, NetBird, Firezone and others
(2 Oct 2026, summary in spec 024).
**Context:** Since 0.24 the agent ran a WireGuard server (`wg0`) for devices: peers with a name, an address and a key,
nothing else. A peer could reach every service of the host that listens on the VPN address or on all addresses (the web
UI in the intranet profile, Caddy on 443, MQTT/TLS 8883, SSH) and, because Docker switches on IPv4 forwarding for the
whole host, published container ports directly by their bridge address (on Docker before 28 also ports published only on
127.0.0.1, e.g. the bundled PostgreSQL). There were no site networks, no expiry, no rotation, no link to organizations.
For OT customers the expectation (NIS2, IEC 62443) is a controlled conduit: least privilege per device and port, time
limits, audit, immediate revocation — not a flat network.

## Decision

1. **Hub and spoke, the server is the hub.** Peer kinds: `device` (sends data, e.g. MQTT without TLS through the tunnel),
   `user` (a technician's laptop or phone), `site` (a router or Raspberry Pi that connects the LAN behind it: routed
   subnets, site-to-site). No mesh, no NAT traversal (the server needs a reachable UDP port; a cloud hub for servers
   behind NAT is phase 3).
2. **The database is the source of truth; the agent applies the whole state.** Peers (`core.vpn_peer`) and access rules
   (`core.vpn_rule`) live in the database (migration 0044) with organization (optional), device link (optional), routed
   subnets, expiry, enabled flag, key rotation time; rows are never deleted (`removed_at`). Every change runs in a
   transaction, is compiled into the complete state (`internal/vpn`) and sent with `PUT /vpn` to the agent; the
   transaction commits only when the agent applied it. The web service pushes the state again every five minutes and
   whenever a time-limited access expires.
3. **Secrets stay where they are needed.** Peer private keys are generated in Go, shown once (text, QR, router texts)
   and never stored. Preshared keys travel once from the web service to the agent and stay in the agent's root-only
   `peers.json`; the database holds public keys only. The server key never leaves the agent. Showing a configuration
   (adding a peer, rotating a key) needs the caller's password again and, with two-factor sign-in, the authenticator code
   (failed confirmations are limited to five a minute per user); these two operations are not offered to AI assistants.
4. **Access rules, default deny, enforced by nftables.** A rule is *sources → destination : protocol/ports*. Sources:
   all peers, a kind, a peer or an organization (a site as a source includes its routed subnets). Destination: this
   server's VPN address (input; presets for MQTT, the web UI, MQTT/TLS, SSH), a peer's address, or a site's routed
   subnets or a host/subnet inside them (forwarding through the hub). The agent renders one dedicated table
   `inet ctrl32_vpn` with an input and a forward chain (priority filter − 10): traffic that does not come from (or go to)
   `wg0` is not judged; from `wg0` established/related replies pass, ping of the server passes, the rules accept, and
   everything else is dropped (`counter drop`). The table is replaced atomically (`table … / delete table … / table … {…}`
   in one `nft -f` transaction) after `nft -c` checked it. Because a drop in any table is final, the table also closes
   Docker's forwarding path and every host service for VPN traffic, whatever other firewalls (ufw, Docker's iptables)
   do; with an active ufw the agent allows `wg0` in ufw so this table is the single place that decides.
5. **Forwarding only when needed.** `net.ipv4.conf.wg0.forwarding` is switched on only while a rule needs forwarding
   (towards peers or site subnets) and off otherwise (the host's other interfaces keep their setting). Site subnets are
   routed through `wg0` (the agent reconciles the routes; `wg syncconf` does not) and are peers' AllowedIPs on the server.
   **No NAT on the server:** replies from a site come back because the site router routes the VPN range into its tunnel;
   only a site device that is not the LAN's default gateway (a Raspberry Pi beside the router) must masquerade, and its
   template says how.
6. **Apply order and rollback in the agent.** Validate everything again (addresses inside the VPN range, unique keys,
   names, subnets canonical and non-overlapping, never a default route, never a host network — a routed subnet that
   overlaps a network of the host's other interfaces is refused because it would cut the server off; rules only between
   VPN addresses and declared subnets; ports). Then: check the table, write the files, load the table, `wg syncconf`,
   routes, forwarding. Any failure restores the previous files, table, peers, routes and forwarding and is reported (the
   web service rolls its transaction back). `wg0.conf` loads the table with `PreUp` (before the interface exists) and
   removes it with `PostDown`; if loading fails, wg-quick does not bring the interface up (fail closed).
7. **wg CLI kept, wgctrl not adopted.** `golang.zx2c4.com/wireguard/wgctrl` was considered for applying peers and
   reading statistics. It would add five modules (netlink, genetlink, socket, …) to the root process for structured
   statistics only; `wg syncconf` is already atomic and proven in production, and `wg show dump` parsing is tested. Keys
   are generated in Go (`golang.org/x/crypto/curve25519`, already a dependency), so `wg genkey`/`pubkey`/`genpsk` are no
   longer run. Revisit with the userspace hub of phase 2.
8. **Licence split** (same license as white labeling, ADR-0016/0017; no prices in code, UI or documentation): free are
   up to 5 peers of kinds device and user without an expiry; the license adds unlimited peers, site peers with routed
   subnets and time-limited access. Without the license existing peers keep working, access rules can always be changed
   (restricting access is never paid), and adding beyond the limit is refused with a clear message (402
   `license_required`).
9. **Permissions: system.admin only, organizations as an attribute.** A peer may belong to an organization (or the whole
   server) and rules may use an organization as a source, but managing the VPN needs `system.admin`. Org-scoped
   management by `org_admin` is **not** implemented: the built-in `org_admin` role of every organization currently
   contains `system.admin` (migration 0002), so a separate `vpn.manage` scope would first need a split of server and
   organization administration (an addendum to ADR-0012). Left for a later phase and recorded in the threat model.
10. **Upgrade of existing installations.** Peers kept only by the agent (earlier versions) are imported once as device
   peers of the whole server, with one access rule *MQTT for devices* (tcp 1883 to the server) that keeps the documented
   use working; everything else from the VPN is denied from then on (audited as `vpn.import`).
11. **Development and tests.** `[agent] vpn_simulate = "<dir>"` makes the agent a simulation: it renders `wg0.conf` and
   the table into the directory, runs nothing and serves only the VPN operations (everything else 501). The e2e and
   headless suites run against it on any platform; on Windows without it the VPN section stays hidden (501 from the
   agent), as before.

## Consequences

- The server becomes a gateway into customers' OT networks: its compromise or a wrong rule means access to PLCs. The
  threat model lists the risks and mitigations; an external penetration test is needed before customers use site
  access in production.
- Hosts need nftables (the agent installs `nftables` with `wireguard-tools` on enabling, Debian/Ubuntu). Hosts with
  only legacy iptables (no nf_tables kernel support) cannot enable the VPN.
- Every peer reaches nothing until a rule allows it; after the upgrade, VPN peers that used anything except MQTT to the
  server (e.g. the web UI over the tunnel) need a rule.
- Windows server support (userspace WireGuard with the same rules), approval of technician access by a second person
  and router templates for more brands are phase 2; a cloud hub for servers behind NAT is phase 3.
