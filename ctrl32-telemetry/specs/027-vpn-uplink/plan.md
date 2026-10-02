# Implementation Plan: VPN phase 3 — uplink to a hub

**Spec**: `specs/027-vpn-uplink/spec.md` · **Version**: 0.69.0 · **Decision**: ADR-0021

## Design

| Part | Where | Notes |
|---|---|---|
| Data | `db/migrations/0048_vpn_uplink.sql` | `vpn_peer.uplink`, `vpn_peer.tunnel_key_hash`, `core.vpn_uplink_rule` |
| Model | `internal/vpn/uplink.go` | token (encode, parse, validate), local overlap, uplink rules (validate, compile, agent check), `RenderUplinkNft`, `UplinkConf`, `HubAllowedIPs`; `Peer.Uplink` in `vpn.go` |
| HTTPS transport | `internal/uptunnel` | `Handler` (hub: bearer secret → relay to the WireGuard port), `Client` (customer: loopback UDP ⇄ WebSocket, reconnect) |
| Windows | `internal/wghub/uplink.go`, `hub.go`, `filter.go` | uplink mode: one peer with endpoint, filter for input and TCP to the LAN, relay from the host |
| Agent | `internal/agent/uplink.go`, `types.go`, `client.go`, `server.go`, `hub_*.go`, `vpn.go` | `PUT/DELETE /uplink`, rules, transport, kernel (wg1, nft, forwarding, ufw, broker), userspace, simulation, endpoint watch, overlap checks both ways |
| Broker | `internal/broker/broker.go`, `internal/config` | `mqtt.uplink_listen` |
| Web | `internal/httpapi/vpn_uplink.go`, `vpn.go`, `server.go`, `openapi.go`, `mcp.go` | uplink peer and token, tunnel endpoint, customer endpoints, rules with transaction + push, view, synchronisation |
| UI | `static/vpn.html` | fourth kind, token result, uplink card, dialogs |
| Tests | `vpn/uplink_test.go`, `agent/uplink_test.go`, `wghub/uplink_test.go`, `httpapi/vpn_test.go`, `tests/e2e/vpn3.sh`, `tests/run-all.sh` | Linux containers outside the repository (report in JOURNAL) |
| Docs | ADR-0021, ADR-0018 note, `docs/vpn.md`, threat model, public documentation (`vpn/uplink-hub.md`), `deploy/dev/demo/docs.py` (vpn3), `tests/headless/docshots.js` | |
