# ADR-0020: VPN on Windows servers — userspace WireGuard hub with the access rules enforced in-process

**Status:** accepted 2 Oct 2026 (owner: VPN phase 2, "Windows server support … keep the same model (peers, routed
subnets, default-deny ACL) … only the official signed wintun DLL may be shipped"). Amends ADR-0018 (decision 11 and
the consequence "Windows server support … is phase 2") and ADR-0012 (the agent manages WireGuard on Windows too).
**Context:** On Linux the VPN is kernel WireGuard (`wg-quick@wg0`) plus the nftables table `inet ctrl32_vpn`, applied
by the privileged agent (ADR-0018). Windows has neither: kernel WireGuard exists only through the WireGuard for
Windows service and its wintun/WireGuardNT drivers, and there is no nftables. The options were:

1. **wintun + Windows routing + Windows Firewall.** A real network adapter (the official signed `wintun.dll` beside
   the binary), wireguard-go on it, routes for site subnets through `winipcfg`, forwarding between peers by enabling IP
   routing on the adapter and filtering with Windows Firewall rules. Windows Firewall cannot express "source prefix →
   destination prefix : port" for *forwarded* traffic (it filters host traffic; routed traffic needs WFP callouts or
   RRAS), so the default-deny model for site access would not be enforceable without writing our own WFP filters;
   it also needs a driver, a shipped DLL, administrator network changes and a reboot-safe cleanup.
2. **wireguard-go on a private userspace TCP/IP stack (gvisor), filter in our process.** No adapter, no driver, no DLL,
   no host routing or firewall changes except the inbound UDP port. Every packet from a peer passes our own filter
   before it reaches anything.

## Decision

1. **Userspace hub in the agent (option 2).** On Windows the agent runs WireGuard itself
   (`golang.zx2c4.com/wireguard`, device + UAPI configuration) on a virtual interface implemented in
   `internal/wghub`. It is not an interface of the operating system. wintun is **not** used and no DLL is shipped (the
   wintun Go binding is compiled in with wireguard-go's `tun` package but is never called, so `wintun.dll` is never
   loaded). Linux keeps kernel WireGuard and nftables unchanged; only the Windows build links the hub
   (`internal/agent/hub_windows.go`; the Linux binary does not contain wireguard-go or gvisor).
2. **The same compiled model, enforced in-process.** The web service sends the same `PUT /vpn` state (`vpn.Desired`:
   peers and compiled rules with chains `input` and `forward`). The hub's filter is the equivalent of the nftables
   table: every packet that arrives from a peer is dropped unless
   - its destination is the server's VPN address and an `input` rule matches (source prefix, protocol, destination
     port), or it is an ICMP echo request to the server (the nft table allows pinging the server as well);
   - its destination is another peer's address or a site's routed subnet and a `forward` rule matches; such packets
     go straight back into WireGuard (towards the destination peer by its allowed IPs) and never touch Windows;
   - it belongs to a tracked flow: connection tracking (5-tuple; idle 10 min TCP, 60 s UDP, 30 s ICMP; at most
     65 536 entries, a full table refuses new flows) lets replies of accepted forwarded connections pass, like
     `ct state established`.
   IPv6 and IP fragments are dropped (the VPN is IPv4; clients use MTU 1420). Rules are swapped atomically; tracked
   flows that the new rules no longer allow are removed, so **taking access away ends existing connections at once**
   (stricter than Linux, where established connections survive until they close). Counters of dropped, delivered
   and forwarded packets are kept.
3. **Server services through a relay.** Packets accepted by the `input` chain enter a private gvisor stack that holds
   only the server's VPN address. Its TCP forwarder accepts a connection on any port that passed the filter and
   relays it to `127.0.0.1:<port>` on the host (at most 1 024 concurrent connections; nothing listening there → the
   peer gets a reset). Consequences: services see `127.0.0.1` as the client address (MQTT and web logs show the
   loopback address, not the peer's VPN address); a service must listen on loopback or all addresses; **UDP services
   of the server are not offered on Windows** (rules towards the server with UDP have no effect there); the broker's
   `mqtt.vpn_listen` is not set on Windows (there is no VPN address on the host).
4. **What Windows does not do (yet).** The server's own outgoing connections into the VPN (e.g. a Modbus connector
   polling a PLC at a site) are not possible: the host has no route into the VPN. That needs an OS adapter (wintun
   with routes) and is left for a later phase; the hub's design keeps the filter in front of any such adapter.
5. **Agent integration.** `WireGuardState.mode` is `kernel` (Linux), `userspace` (Windows hub) or `simulation`.
   `PUT /wireguard` on Windows generates the server key as before, (re)starts the hub on the configured port and
   address and applies the stored peers and rules, and adds an inbound Windows Firewall rule "ctrl32 WireGuard" for
   the UDP port limited to the agent's binary (`netsh advfirewall`, best effort, logged; deleted when the VPN is
   disabled); no packages are installed and no services are touched. `PUT /vpn` validates the state exactly as on
   Linux (`vpn.ValidateDesired`), applies it to the hub (filter first, then peers through UAPI without recreating
   peers that stay, so their sessions survive; on failure the previous filter and peers are restored and the error is
   reported as on Linux) and writes `peers.json`, `vpn.json` and `vpn-rules.txt` (the rules text shown in the UI
   instead of the nftables table). When the agent starts and the VPN is enabled it starts the hub from these files.
   Live values (handshake, endpoint, bytes) come from the UAPI. The agent runs as a Windows service (LocalSystem), so
   keys stay in `%ProgramData%\ctrl32-telemetry\wireguard` as before.

## Consequences

- One model on both platforms: the UI, the API, the licence split and the audit are the same; only the enforcement
  point differs (nftables on Linux, the agent on Windows). The filter and the hub are unit- and integration-tested on
  any platform without administrator rights (two wireguard-go clients against the hub over loopback UDP: relay to an
  allowed service, refused ports, ping, forwarding only with a rule and replies through connection tracking, removal
  of peers, rollback on a bad key).
- Throughput is that of userspace WireGuard and gvisor (enough for technician access and telemetry, not for bulk
  transfers); the Windows binary grows by about 5.5 MB (46.5 → 52.0 MB).
- If the agent stops, the VPN on Windows stops with it (fail closed: no tunnel, no access).
- Not verified: a real Windows server with remote peers over the internet (NAT, the Windows Firewall rule on a real
  installation, long-running stability and throughput), and site routers behind the Windows hub with real PLCs.
- Considered and rejected: wintun with Windows Firewall (no enforceable default deny for forwarded traffic without our
  own WFP filters, a driver and a DLL to ship), WireGuard for Windows (WireGuardNT) driven by its service (no per-port
  filtering of forwarded traffic either, and a second product to install), running the hub in the web service (it
  must keep the keys out of the unprivileged process, ADR-0012/0018).
