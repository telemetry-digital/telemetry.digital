# Implementation Plan: Server administration split, VPN phase 2

**Spec**: `specs/025-vpn-phase2/spec.md` · **Version**: 0.65.0 · **Decisions**: ADR-0019, ADR-0018 amendment, ADR-0020

## Design

| Part | Where | Notes |
|---|---|---|
| Data | `db/migrations/0045_server_admin_vpn2.sql` | role, permission, grants with reason, `vpn_rule.org_id`, `core.vpn_grant`, change request kind |
| IAM | `internal/iam/admin.go`, `service.go` | `ServerPermission`, `ServerAdminRole`, `PermissionsOfRoles`, `ServerAdmins` (last-administrator count), policy `four_eyes_vpn`, MFA defaults |
| User administration | `internal/httpapi/users.go`, `auth.go` | `guardServerAdminTarget`, `checkRoleChange`, role create/update checks, create user check |
| Permissions | `internal/httpapi/middleware.go`, `mcp.go`, `openapi.go` | `permOK` ("a\|b" any of), OpenAPI text "one of" |
| VPN scope and grants | `internal/httpapi/vpn_scope.go`, `vpn.go` | `vpnScope`, `checkOrgRule`, `grantVPNPeer`, `endGrants`, `vpnGrantInfo`; scope checks in every handler; expiry job closes grants |
| Four eyes | `internal/httpapi/changes.go`, `commands_approval.go` | `holdChange` (shared with `fourEyes`), `canApprove("vpn")`, policy field |
| Windows hub | `internal/wghub`, `internal/agent/hub*.go`, `vpn.go` | see ADR-0020 |
| Templates | `internal/vpn/render.go` | `RouterTemplateKeys`, `ClientTemplates`, `ClientTemplateKeys` |
| UI | `static/vpn.html`, `users.html`, `settings.html`, `app.js` | scope, grant dialog, pending/active grants, template tabs, server role, policy switch, approvals kind |
| Tests | `vpn_scope_test.go`, `vpn_test.go`, `templates_test.go`, `wghub/*_test.go`, `agent/hub_test.go`, `tests/e2e/users.sh`, `tests/e2e/vpn2.sh`, `tests/headless/vpn.js`, `tests/headless/users_roles.js`, `tests/run-all.sh` | |
