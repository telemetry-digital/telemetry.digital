# Implementation Plan: Own WireGuard VPN (phase 1)

**Spec**: `specs/024-vpn/spec.md` · **Version**: 0.64.0 · **Decision**: ADR-0018

## Design

| Part | Where | Notes |
|---|---|---|
| Model and rules (pure) | `internal/vpn/vpn.go` | `Peer`, `Rule`, `Source`; `ParseSubnet(s)`, `CheckSubnetConflicts`, `ParsePorts`, `ValidateRule`, `CheckAdd`/`CheckEdit` (license), `NextAddress`, `Compile` → `Desired` (peers with enabled flag, `FwRule` input/forward), `ClientAllowedIPs` |
| Rendering and agent-side checks (pure) | `internal/vpn/render.go` | `ValidateDesired` (ids, names, keys, addresses, subnets incl. host networks, rule CIDRs and ports), `LocalNets`, `RenderNft` (table `inet ctrl32_vpn`, atomic replace), `ServerConf` (PreUp/PostUp/PostDown), `ClientConf`, `RouterTemplates` |
| Keys | `internal/vpn/keys.go` | Curve25519 in Go (x/crypto), PSK, key format check |
| Agent | `internal/agent/vpn.go` | `PUT /vpn` (`hApplyVPN` → `applyLive`: `nft -c`, files, `nft -f`, `wg syncconf`, routes, forwarding, ufw; `restore` on failure), `hSetWireGuard` (installs `wireguard-tools` and `nftables`, writes the table before starting wg-quick), `wgState` (+ firewall state, ruleset), simulation (`SetSimulation`, restricted `routes()`); host access through `a.exec`, `a.localNets`, `a.setForwarding` (fakes in tests) |
| Agent client | `internal/agent/client.go` | `ApplyVPN`; peer add/remove operations removed |
| Data | `db/migrations/0044_vpn.sql` | `core.vpn_peer`, `core.vpn_rule`, partial unique indexes, grants |
| API | `internal/httpapi/vpn.go` | `vpnChange` (lock, agent state, transaction, compile, `ApplyVPN`, commit), handlers, `stepUp` (password/TOTP, failures limited with `limiter.peek`), `vpnLoop`/`vpnTick` (expiry, periodic push), `vpnImportLegacy` |
| Routes and pages | `internal/httpapi/server.go` | 9 operations, `/server/vpn` → `vpn.html`, `/server/wireguard` → 301 |
| OpenAPI and MCP | `openapi.go`, `mcp.go` | tag *VPN*, schemas `VPNPeer`, `VPNSource`, `VPNRule`, `VPNRuleInput`; MCP area `vpn`, peer creation and rotation excluded (private key, step-up); `go run ./tools/apigen` |
| UI | `static/vpn.html`, `static/server.html` | new page; the old WireGuard section removed from the server page, subnav link *VPN* |
| Config | `internal/config`, `cmd/ctrl32-telemetry` | `[agent] vpn_simulate` |
| Tests | `internal/vpn/vpn_test.go`, `internal/agent/vpn_test.go`, `tests/e2e/vpn.sh`, `tests/headless/vpn.js`, `tests/run-all.sh` (VPN suites against the simulation) | |
| Docs | ADR-0018, `docs/vpn.md`, threat model, README, JOURNAL, STAV | |
