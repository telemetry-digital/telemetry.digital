# VPN for sites, technicians and devices

*System → VPN* (0.64.0, spec 024, ADR-0018; phase 2 in 0.65.0, spec 025) turns the server into a WireGuard hub with
access rules. On Linux the server agent manages kernel WireGuard and an nftables table; on Windows (0.65.0, ADR-0020)
the agent runs WireGuard itself and enforces the same rules in its own process (see *Windows servers*). An
organization's administrator manages the VPN of the own organization under *Settings → VPN* (see *Organizations*).

| Goal | How |
|---|---|
| A technician reaches a PLC or HMI at a site (TIA Portal, Modbus TCP, web HMI, VNC) | a **technician** peer (laptop or phone with the WireGuard app) and a rule *technician → site host : ports* |
| A site's whole LAN is connected (site-to-site) | a **site** peer: a router (OpenWrt, MikroTik, Teltonika, Linux) or a Raspberry Pi with the LAN as routed subnets |
| Devices send data without TLS | a **device** peer and a rule *devices → this server : MQTT 1883* |
| The server polls a PLC at a site (Modbus/OPC UA connectors) | nothing to add: the server itself may always reach the peers and site networks |

Everything that arrives from the VPN is **dropped unless an access rule allows it** (default deny), also traffic to the
server's own services. A new peer reaches nothing until a rule allows it.

## Free and with the license

- **Free:** up to 5 peers (technicians and devices), all access rules.
- **With the license** (the same license as white labeling, *System → License*): unlimited peers, **sites** with routed
  subnets (site-to-site) and **time-limited access** (an expiry date or an access grant; the peer is disabled
  automatically), including access approved by a second person.
- Without the license existing peers keep working and rules can always be changed; adding a peer beyond the limit, a
  site or an expiry is refused with a message.

## Setting up

1. *System → VPN → Server settings*: enable, UDP port (default 51820), the server's VPN address with the range
   (default `10.66.0.1/24`), the endpoint peers connect to (`host:port`; filled from the domain when empty), optional
   DNS for peers. On enabling the agent installs `wireguard-tools` and `nftables` (Debian/Ubuntu), loads the firewall
   table, starts `wg-quick@wg0` and opens the UDP port in ufw. Open the UDP port in the provider's firewall too.
2. *Add peer*: choose *Technician*, *Device* or *Site (router)*, a name, optionally the organization and a device, for a
   site the routed subnets (the LAN behind the router, e.g. `192.168.10.0/24`), optionally an expiry. Confirm with your
   password (and the authenticator code with two-factor sign-in). The configuration is shown **once**: a QR code for the
   WireGuard app, the `.conf` file, the file for the WireGuard app for Windows, steps for Android and iPhone and, for a
   site, router texts (Linux/Raspberry Pi with wg-quick, OpenWrt, MikroTik RouterOS 7, Teltonika RUT with RutOS,
   Ubiquiti EdgeRouter, pfSense, OPNsense). The private key is not stored on the server; a lost configuration means
   *Rotate key*.
3. *Add rule*: sources (all peers, all technicians/devices/sites, an organization, single peers), destination (this
   server, a site network or a host inside it, a peer), protocol and ports (`102, 502, 8000-8100`; empty = all ports).
   Presets for the server: MQTT inside the VPN (1883), the web interface (when it listens beyond localhost), MQTT/TLS,
   SSH.

Every change needs a reason and is written to the audit trail (`vpn.peer_add`, `vpn.peer_update`, `vpn.peer_rotate`,
`vpn.peer_remove`, `vpn.peer_expire`, `vpn.grant`, `vpn.rule_add`, `vpn.rule_update`, `vpn.rule_remove`, `vpn.apply`,
`vpn.import`, `server.wireguard`, and for approvals `change_request.create`, `change_request.approve`,
`change_request.reject`). *Disable* ends the access at once and keeps the key; *Remove* also removes rules towards the peer.

### Examples

- **PLC programming at a plant:** site *plant-1* with `192.168.10.0/24`; rule *all technicians → plant-1
  192.168.10.20 : tcp 102* (Siemens S7). The technician's configuration routes the VPN range and the routed subnets of
  all sites; the firewall decides what is reachable. Profinet discovery (layer 2 broadcasts) does not cross a routed
  VPN: enter the PLC's address by hand.
- **Contractor for one afternoon:** technician peer with an expiry; it is disabled automatically (audited as
  `vpn.peer_expire`). Enabling it again needs a new expiry or none.
- **Gateway without TLS:** device peer; rule *all devices → this server : tcp 1883*; the gateway publishes to
  `mqtt://10.66.0.1:1883` with its device token.

## Access grants and four eyes (0.65.0)

*Grant access* on a peer enables it — and every rule that names it — for a window: 2 hours, 8 hours, a day or a custom
length (15 minutes to 30 days). When the window ends the peer is disabled again (`vpn.peer_expire`, with the grant). A
new grant replaces the open one; *Disable* ends it at once. The peers table shows the open window ("access until …,
approved by …").

With *Settings → Security policy → Four eyes for VPN access* (off by default, per organization):

- a new **technician** is added disabled (its configuration is still shown once, so it can be installed beforehand);
- *Request access* (a grant) does not enable it but waits under *Settings → Approvals* (kind *VPN access*, e.g.
  "VPN access for technician novak for 8 hours");
- a **different** user with `vpn.manage` or `system.admin` approves or rejects it with a reason; on approval the grant
  is executed under the approver's name and the window starts **then**; both names are in the audit trail and on the
  peer;
- enabling a technician or changing its expiry directly is refused (`approval_required`); disabling stays immediate;
- devices and sites are not covered by the policy. Grants are not offered to AI assistants (MCP).

## Organizations (0.65.0)

The server administrator (`system.admin`, role `server_admin`) manages everything. An organization's administrator
(`org_admin` holds `vpn.manage`) opens *Settings → VPN* and sees only the organization's own peers and rules:

- new peers belong to the organization (they cannot be moved to another organization or to the whole server);
- rule sources are the organization's peers or the whole organization; *all peers* and *all technicians/devices/sites*
  span every organization and are for the server administrator only;
- rule destinations are the organization's peers and sites, or this server with TCP to the offered services (MQTT, the
  web interface, MQTT with TLS) or ping — never SSH of the host;
- peers and rules of other organizations and of the whole server are invisible (404); the server settings, *Apply
  again* and the generated firewall rules stay with the server administrator.

The free limit of 5 peers counts every peer of the server.

### Site routers

The router (or a Raspberry Pi) connects out to the server; its LAN devices reach technicians' traffic through it. When
it is the LAN's default gateway nothing else is needed. A Raspberry Pi *beside* the gateway must masquerade VPN traffic
into the LAN (the wg-quick template has the two lines commented out), or the gateway needs a route to the VPN range via
the Raspberry Pi. The templates are text only — check them before use. The server does not NAT: VPN addresses stay
visible in the site's logs.

## How it is enforced

- The agent renders `/etc/wireguard/wg0.conf` and `/etc/ctrl32-telemetry/wireguard/vpn.nft` (nftables table
  `inet ctrl32_vpn`, chains `input` and `forward`, priority filter − 10). The table is loaded before the interface
  comes up (`PreUp`) and replaced atomically on every change; the UI shows it under *Firewall rules*.
- Order of a change: the web service validates and writes in a transaction, the agent validates again, checks the table
  with `nft -c`, loads it, applies the peers with `wg syncconf` (no tunnel drops), reconciles the routes of site subnets
  and switches forwarding on `wg0` (only while a rule needs it). Any failure restores the previous state and the
  transaction is rolled back; the error is shown.
- The server pushes the stored state again every five minutes and when an access expires; *Apply again* does it now.
- Check on the host: `nft list table inet ctrl32_vpn`, `wg show wg0`, `ip route show dev wg0`,
  `sysctl net.ipv4.conf.wg0.forwarding`, `journalctl -u ctrl32-telemetry-agent`.

## What a peer can reach (review of 2 Oct 2026)

Review of a production-like install (`deploy/install.sh`) before access rules existed:

| Listener | Binding | Reachable from the VPN before 0.64.0 | Now |
|---|---|---|---|
| PostgreSQL (Docker) | `127.0.0.1:5432` published | not on the VPN address; **but** its container address (`172.x`) through Docker's forwarding when a peer routes it — on Docker before 28 also for ports published on 127.0.0.1 | dropped (forward chain) |
| PostgreSQL (apt) | `localhost` | no | no |
| Web UI | intranet profile `:8080` (all addresses); internet profile `127.0.0.1:8080` behind Caddy (host network, `:80`/`:443` on all addresses) | yes (8080 or 80/443) | only with a rule |
| MQTT | `127.0.0.1:1883`, WebSocket `127.0.0.1:1884`; `<VPN address>:1883` when the VPN is on | 1883 on the VPN address | only with a rule |
| MQTT/TLS | `:8883` with a domain | yes | only with a rule |
| Camera push relay | `push_listen` (e.g. `:8322`) when configured | yes | only with a rule |
| SSH (host) | usually `:22` | yes | only with a rule |
| Caddy admin API | `localhost:2019` | no | no |
| Server agent | Unix socket `/run/ctrl32-telemetry/agent.sock` (0660 root:ctrl32) | no | no |
| Other peers | through the hub when the host forwards (Docker turns forwarding on) | depended on Docker's FORWARD policy | only with a rule |

Ping of the server's VPN address is always allowed (diagnostics). Peers' AllowedIPs on the server are their own address
(and a site's subnets), so a peer cannot use another peer's or a site's address as its source.

## Windows servers (0.65.0)

On Windows the agent runs WireGuard itself (wireguard-go on a private userspace network stack, ADR-0020): no driver,
no extra DLL, no change to Windows routing. Every packet from a peer passes the agent's filter, which is compiled from
the same access rules (default deny, input and forward, replies of allowed connections). The page shows *Runs: Windows:
inside the server agent* and *Access rules enforced by the server agent* instead of the nftables table. Differences:

- services of this server are reached through `127.0.0.1` (they see that address, not the peer's); they must listen on
  loopback or all addresses; UDP services of the server are not offered; SSH is not a preset;
- forwarding between peers and to site subnets happens inside the agent;
- the server itself cannot open connections into sites or to peers (connectors polling a PLC at a site need a Linux
  server for now);
- the agent opens the UDP port in Windows Firewall (rule *ctrl32 WireGuard*, removed when the VPN is disabled); open it
  in the provider's firewall too;
- if the agent stops, the VPN stops (fail closed).

Not yet verified on a real Windows server with remote peers.

## Upgrade from earlier versions

Peers created before 0.64.0 (kept by the agent) are imported once as device peers of the whole server, with the rule
*MQTT for devices* (tcp 1883 to the server). Everything else from the VPN is denied from then on: a peer that used the
web UI or SSH over the tunnel needs a rule. The import is audited (`vpn.import`).

## Limits and later phases

- Linux servers with nftables, or Windows with the userspace hub; IPv4 only; the server needs a reachable UDP port (no
  NAT traversal, no mesh).
- Phase 3: a cloud hub for servers behind NAT (e.g. behind a Cloudflare Tunnel); on Windows, connections from the server
  into sites.
