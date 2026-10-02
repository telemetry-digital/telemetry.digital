# ADR-0021: VPN phase 3 — uplink of a customer server to a hub, rules on both sides, HTTPS transport

**Status:** accepted 2 Oct 2026 (owner: "most customer servers sit in a customer LAN without a public address, so
technicians cannot reach them through the server's own VPN hub … phase 3 = a central hub in the cloud, e.g. our
portal.telemetry.digital server, or any telemetry server with a public address"; SAFETY FIRST: off by default,
`system.admin` on the customer server, reason and audit, default deny on both sides; the uplink counts as a site and
needs the licence; no prices). Amends ADR-0018 (decision 1: "no NAT traversal … a cloud hub for servers behind NAT is
phase 3") and ADR-0020 (the Windows agent runs the uplink in userspace too).
**Context:** The hub of ADR-0018 needs a reachable UDP port. A telemetry server installed in a customer's LAN usually
has none (no public address, no port forwarding, sometimes even outgoing UDP blocked), so technicians could reach it
only through a separate site router or a Cloudflare Tunnel (web only). Any telemetry server with a public address can
already be a hub; what was missing is a way for another telemetry server to join it as a member and to decide itself
what the hub may reach.

## Decision

1. **The customer server is a site of the hub, marked as an uplink.** On the hub, *Add peer → Server (uplink)* creates
   a `site` peer with `uplink = true` (migration 0047): an address in the hub's VPN and, optionally, the LAN subnets
   behind the customer server (routed through the hub like any site's; an uplink without subnets offers only the
   server itself). Everything of ADR-0018 applies unchanged on the hub: the licence (a site needs it), the access rules
   (*technicians → uplink peer : 443*, *technicians → uplink subnet host : 502*), organizations, grants, rotation,
   removal, audit. Instead of a WireGuard file and router texts the hub shows once — after the password again — an
   **uplink token** (`ctrl32-uplink-v1.` + base64url JSON: hub endpoint, hub public key, preshared key, the uplink's
   private key, its address, the hub's VPN range and address, the routed subnets, and, when the hub has an HTTPS
   address, the HTTPS transport's URL and secret). Its AllowedIPs on the customer side are the hub's VPN range only
   (other sites of the hub are never routed to a customer server).
2. **The customer server applies its own default deny.** *System → VPN → Connect to a hub* (`system.admin` only, the
   password again, a reason, audited `vpn.uplink_connect|replace|transport|disconnect`) hands the token to the agent,
   which validates it again (keys, ranges, endpoint, URL), refuses a hub range that overlaps the host's networks, the
   server's own VPN range or its own sites (and, the other way round, its own VPN may not take the hub's range later)
   and never overwrites a `wg1.conf` it did not write. **Uplink rules** (`core.vpn_uplink_rule`, `system.admin`,
   reason, audit `vpn.uplink_rule_*`) say what arrives from the hub may reach: *from* the hub's range or an address or
   subnet inside it (one technician), *to* this server's uplink address (input) or a host or subnet of the offered
   networks (forwarding), protocol and ports. They are stored even while no uplink is connected and applied when one
   is; parts outside the connected uplink's ranges are left out. **Access therefore needs a rule on both sides**: the
   hub's rule lets the technician towards the uplink, the customer's rule lets the hub's traffic in.
3. **Linux: a second interface and a second table.** The agent writes `/etc/wireguard/wg1.conf` (no `ListenPort`,
   one peer — the hub — with `PersistentKeepalive = 25`) and the nftables table `inet ctrl32_uplink`: input from `wg1`
   only to the uplink address with the rules (ping of the server allowed), forward from `wg1` to the LAN with the rules,
   replies as established, everything else `counter drop`; forwarded traffic is **masqueraded** behind the server's LAN
   address (`postrouting` NAT), so LAN devices — whose gateway is usually not the telemetry server — answer without a
   route back and see the server, not the technician. Forwarding is switched on for `wg1` and, only while a LAN rule
   exists, on the LAN interfaces that lead to the offered subnets (replies arrive there); when the agent had to switch
   one on, the table drops everything else forwarded from it (`iifname <lan> oifname != wg1 drop`), so the host does
   not become a router for its LAN. Hosts that already forward on all interfaces (Docker) are left as they are. PreUp
   loads the table before the interface exists, PostUp/PostDown switch the forwarding (so a reboot restores the same
   state), PostDown removes the table. Apply order and rollback as in ADR-0018 (check with `nft -c`, write, restart
   `wg-quick@wg1` for a new configuration, otherwise reload the table and set the endpoint; any failure restores the
   previous files and state; a failed first connect leaves nothing). The broker gets `mqtt.uplink_listen` on the uplink
   address. Disconnecting stops the interface, removes the table and deletes the keys (the rules stay).
4. **Windows: the same in the agent.** The userspace device of ADR-0020 in uplink mode (`internal/wghub`): one peer
   with an endpoint and keepalive, no listening port of its own; the in-process filter checks every packet from the
   hub against the same compiled rules; TCP to this server is relayed to `127.0.0.1:<port>`, TCP to LAN hosts ends in
   the private stack and the agent opens its own connection from the host (the LAN host sees the server, like the
   masquerade; never to an address of the host itself). UDP and ping to LAN hosts are not offered on Windows.
5. **HTTPS transport for networks that block UDP (implemented).** Options considered: (a) WireGuard over TCP with a
   foreign tool (udp2raw, wstunnel) — another binary to ship and run as root, another configuration; (b) a TLS tunnel
   of our own on a separate port — needs a second open port on the hub and certificates; (c) **a WebSocket on the hub's
   existing HTTPS address, inside our own binary on both ends** — chosen. The customer agent listens on a loopback UDP
   port that becomes WireGuard's endpoint and relays each datagram as one binary WebSocket message to
   `wss://<hub>/api/v1/vpn/uplink/tunnel` through the hub's reverse proxy (Caddy passes WebSockets); the hub's web
   service relays them to its WireGuard port on `127.0.0.1` (`internal/uptunnel`, ~300 lines on gorilla/websocket, a
   dependency already in use). Security: the relay is not trusted — WireGuard authenticates and encrypts every
   datagram end to end; the WebSocket is authenticated with a per-uplink 256-bit secret (only its SHA-256 is stored on
   the hub, rotated with the key, refused for a disabled, expired or removed uplink, closed at once on disable, rotate
   and remove), frames are limited to 4 KiB, 1 000 relays at most, idle relays close after two minutes; the hub's
   WireGuard port is never exposed through the web server to anyone without a valid uplink secret. The transport is
   chosen when connecting and can be switched (`udp`/`https`); while it is on, the agent keeps WireGuard's endpoint on
   the local end (WireGuard roams to the source of authenticated packets and could otherwise move back to the blocked
   UDP path). Cost: TCP inside TCP is slower under packet loss — meant for maintenance access, documented as such.
   Without an HTTPS base URL on the hub the token carries no HTTPS transport (UDP only).
6. **Not in this phase:** a technician connecting directly to a customer server (always through the hub), several
   uplinks per server, the hub's other sites reaching a customer server (only the hub's VPN range is routed), IPv6.

## Consequences

- A cloud hub (e.g. `portal.telemetry.digital`) becomes a gateway into many customer networks; the customer keeps the
  last word with its own default-deny rules and can disconnect at once. A compromised hub reaches only what the
  customers' uplink rules allow. The threat model lists the risks.
- Customer servers need nothing open inbound (outbound UDP to the hub's port, or only HTTPS with the HTTPS transport).
- The hub's VPN range must not collide with any customer's networks or own VPN range: a cloud hub should use a range
  of its own (e.g. `10.99.0.1/16`), not the default `10.66.0.1/24` most customer servers use.
- Verified: unit tests (token, rules, rendering, agent with a fake host and a fake userspace device, the endpoint
  watch), an in-process test with real WireGuard (hub, customer in uplink mode, technicians over loopback, rules on
  both sides, HTTPS transport with a reconnect), the e2e suite `vpn3` against the agent's simulation, and **real Linux
  kernel WireGuard and nftables in privileged containers** (hub, customer, two technicians, two PLCs: only what both
  sides allow is reachable, masquerade, the LAN guard, rules changed without a restart, the interface restarted, the
  HTTPS transport with outgoing UDP blocked, disconnect). Not verified: a real cloud hub behind Caddy over the
  internet, the Windows uplink on a real Windows server.
