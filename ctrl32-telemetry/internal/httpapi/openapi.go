package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// OpenAPI 3.1 description of /api/v1 (URS G-01, ADR-0010). The document is generated from the table below:
// every route registered in server.go must have an entry here (openapi_test.go enforces it both ways), so the
// description cannot fall behind the code. Served at GET /api/v1/openapi.json (no authentication — it contains
// no secrets) and rendered by the built-in reference page at /api/docs.

// S is a JSON Schema / JSON object literal.
type S = map[string]any

type apiParam struct {
	Name, Desc, Type string
	Required         bool
	Enum             []string
}

type apiOp struct {
	M  string     // "METHOD /api/v1/path"
	T  string     // tag
	S  string     // summary
	D  string     // description (markdown)
	P  string     // permission: "" = any signed-in user, "public" = none, "device"/"connector" = token kinds
	Q  []apiParam // query parameters
	B  S          // request body schema (JSON)
	F  []apiParam // multipart form fields
	R  S          // success response schema (JSON)
	C  int        // success status (default 200)
	CT string     // non-JSON response content type
	RD string     // response description
}

func str(desc string) S     { return S{"type": "string", "description": desc} }
func uuid(desc string) S    { return S{"type": "string", "format": "uuid", "description": desc} }
func ts(desc string) S      { return S{"type": "string", "format": "date-time", "description": desc} }
func num(desc string) S     { return S{"type": "number", "description": desc} }
func integer(desc string) S { return S{"type": "integer", "description": desc} }
func boolean(desc string) S { return S{"type": "boolean", "description": desc} }
func anyObj(desc string) S {
	return S{"type": "object", "additionalProperties": true, "description": desc}
}
func anyVal(desc string) S { return S{"description": desc} }
func ref(name string) S    { return S{"$ref": "#/components/schemas/" + name} }
func arr(items S) S        { return S{"type": "array", "items": items} }
func nullable(s S) S {
	t := S{}
	for k, v := range s {
		t[k] = v
	}
	t["type"] = []any{s["type"], "null"}
	return t
}
func enum(desc string, vals ...string) S {
	e := make([]any, len(vals))
	for i, v := range vals {
		e[i] = v
	}
	return S{"type": "string", "enum": e, "description": desc}
}
func obj(props S, required ...string) S {
	o := S{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}
func q(name, desc string) apiParam  { return apiParam{Name: name, Desc: desc, Type: "string"} }
func qi(name, desc string) apiParam { return apiParam{Name: name, Desc: desc, Type: "integer"} }
func qr(name, desc string) apiParam {
	return apiParam{Name: name, Desc: desc, Type: "string", Required: true}
}

var reason = str("Reason for the change, written to the audit trail (required, 3–200 characters).")
var reasonBody = obj(S{"reason": reason}, "reason")
var okResp = obj(S{"ok": boolean("true on success")})

const (
	tagAuth     = "Authentication"
	tagAccount  = "Account"
	tagUsers    = "Users and roles"
	tagTokens   = "API tokens"
	tagOrgs     = "Organizations"
	tagSites    = "Sites and assets"
	tagData     = "Datastreams and observations"
	tagDevices  = "Devices"
	tagCommands = "Commands"
	tagConsole  = "Device console"
	tagFota     = "Firmware (FOTA)"
	tagAlarms   = "Alarms"
	tagNotify   = "Notifications"
	tagRules    = "Automation rules"
	tagDash     = "Dashboards"
	tagHMI      = "HMI object library"
	tagRepl     = "Replication"
	tagFlows    = "Flows"
	tagPublic   = "Public dashboards"
	tagLive     = "Live stream"
	tagConn     = "Connectors"
	tagReports  = "Reports"
	tagExports  = "Exports"
	tagBrand    = "Branding and languages"
	tagServer   = "Server administration"
	tagProv     = "Provisioning"
	tagSmart    = "Smart home"
	tagDevAPI   = "Device HTTP API"
	tagMeta     = "API"
)

var apiTags = []struct{ Name, Desc string }{
	{tagAuth, "Sign in with username and password (optionally TOTP), sessions, password recovery and self-service registration requests."},
	{tagAccount, "The signed-in user's own profile, password, MFA and language."},
	{tagUsers, "User accounts, built-in and organization roles, the permission catalog and account requests (user.admin)."},
	{tagTokens, "Personal API tokens: scopes are permission names, a subset of the owner's permissions. Send as `Authorization: Bearer <token>`."},
	{tagOrgs, "Organizations on this server (system.admin)."},
	{tagSites, "Sites (locations) and assets (rooms, fridges, machines) that own datastreams."},
	{tagData, "Datastreams (measured quantities of an asset) and their observations; aggregation and latest values."},
	{tagDevices, "Devices (MQTT, HTTP, LoRaWAN, OPC UA, Modbus), their tokens, datastream assignments, messages and shared attributes."},
	{tagCommands, "Commands (RPC) to devices and writes to OPC UA nodes / Modbus registers, with optional four-eyes approval."},
	{tagConsole, "Remote console sessions on a device (line in / lines out), fully logged."},
	{tagFota, "Firmware images and update campaigns."},
	{tagAlarms, "Alarm rules (limits on a datastream), active/cleared alarms and acknowledgement."},
	{tagNotify, "Notification channels (e-mail, webhook, SMS gateway, voice provider, Web Push), escalation policies and the delivery log."},
	{tagRules, "Automation rules \"condition → action\" over datastreams: notify, webhook, device command, alarm, derived datastream."},
	{tagDash, "Dashboards and process pictures (screen format v1: widgets and the drawing layer) and the widget catalog."},
	{tagHMI, "Library of process-picture objects: imported SVG graphics (sanitised) and composites with tag placeholders."},
	{tagFlows, "Visual automation flows (spec 004): a closed library of trigger, logic and action nodes wired into flows, executed on the server; expressions only (no user code). Flows are versioned; a deploy needs a reason and, for flows that send commands, four eyes when the policy is on."},
	{tagRepl, "Redundant database (spec 003): replicas on other machines — one-time pairing, continuous replica, hot standby, safe detach with verification. The replica's own server uses the pair, tunnel and agent endpoints with its token."},
	{tagPublic, "Read-only access to a dashboard shared by token, no sign-in."},
	{tagLive, "Server-Sent Events with observations, device state, alarms and command status."},
	{tagConn, "Inbound and outbound connectors: ChirpStack (LoRaWAN), OPC UA, Modbus TCP."},
	{tagReports, "PDF report templates (Typst) and stored reports."},
	{tagExports, "CSV / JSON Lines / PDF exports of observations, alarms, messages, audit and access logs; organization export."},
	{tagBrand, "Organization branding, theme, map tiles, enabled languages and translation overrides."},
	{tagServer, "Server status, updates, backups, TLS (Caddy) and WireGuard through the local agent (system.admin)."},
	{tagProv, "Device self-provisioning with a claim code or a provisioning profile (ThingsBoard-compatible over MQTT and HTTP), approvals, and the LoRaWAN ingest endpoint used by ChirpStack."},
	{tagSmart, "Smart-home devices that announce themselves with Home Assistant MQTT discovery (zigbee2mqtt, Tasmota, ESPHome, …): MQTT accounts on the ctrl32 broker with their own topic namespace, bridges to foreign brokers, the inbox of discovered devices, entities with state and control, the Home page (spec 005)."},
	{tagDevAPI, "What a device does over MQTT, for devices that only have an HTTP client: telemetry, attributes, status, shared attributes, commands with acknowledgement, FOTA status. Authentication: the device token as `Authorization: Bearer <token>` (the same token is the MQTT password). See docs/integration/device-integration.md."},
	{tagMeta, "This description."},
}

const apiIntro = `ctrl32 telemetry exposes everything the web UI can do as a JSON REST API under ` + "`/api/v1`" + ` (ADR-0010: the UI is one client of this API). Times are RFC 3339 in UTC, identifiers are UUIDs, bodies and responses are JSON unless stated otherwise.

## Authentication
- **API token** — create one under *My account → API tokens* and send ` + "`Authorization: Bearer <token>`" + `. A token carries *scopes*, which are permission names and a subset of its owner's permissions, and an optional expiry. Every call is attributed to the token and its owner in the audit and access logs.
- **Browser session** — ` + "`POST /api/v1/auth/login`" + ` sets the ` + "`ctrl32_session`" + ` cookie; mutating requests must also send the header ` + "`X-CSRF-Token`" + ` with the value of ` + "`csrf_token`" + ` from ` + "`GET /api/v1/auth/me`" + `.
- **Device token** — devices authenticate to MQTT and to the firmware download with their own token (see the device protocol).

## Permissions
Each operation names the permission it needs (` + "`x-permission`" + `): ` + "`data.read`, `data.export`, `data.annotate`, `alarm.ack`, `device.manage`, `device.command`, `device.console`, `fota.read`, `fota.manage`, `config.write`, `content.write`, `user.admin`, `token.manage`, `audit.read`, `audit.review`, `report.sign`, `system.admin`" + `. Roles are sets of permissions.

## Conventions
- Every change carries a ` + "`reason`" + ` (body field, or the ` + "`reason`" + ` query parameter on DELETE) that is written to the tamper-evident audit trail together with the actor, request id, old and new values.
- Errors are ` + "`{\"error\": \"message\"}`" + ` with 400 (validation), 401 (not signed in), 403 (no permission / organization scope), 404, 409 (conflict, e.g. duplicate name or wrong state), 429 (rate limit on sign-in and provisioning) and 5xx.
- Lists are bounded by ` + "`limit`" + ` (default and maximum documented per operation) and ordered newest first unless stated.
- Requests are limited in size; unknown JSON fields are rejected.
- With the organization policy *four eyes for commands* a command — or an automation rule that sends commands — stays ` + "`pending`" + ` until a second user approves it.`

// apiOps describes every route. Path parameters are derived from the pattern.
var apiOps = []apiOp{
	// ---- meta
	{M: "GET /api/v1/openapi.json", T: tagMeta, S: "OpenAPI 3.1 document", D: "This description. The reference page at `/api/docs` renders it.", P: "public", R: anyObj("OpenAPI document")},

	// ---- authentication
	{M: "POST /api/v1/auth/login", T: tagAuth, S: "Sign in", D: "Starts a browser session (cookie `ctrl32_session`). When the user has MFA enrolled the response carries `mfa_required` and a short-lived `pre_auth_token` for `POST /api/v1/auth/mfa`. Rate limited per IP; repeated failures lock the account per the security policy.", P: "public",
		B: obj(S{"username": str("Username"), "password": str("Password")}, "username", "password"), R: ref("LoginResponse")},
	{M: "POST /api/v1/auth/mfa", T: tagAuth, S: "Complete sign-in with a TOTP or recovery code", P: "public",
		B: obj(S{"pre_auth_token": str("From the login response"), "code": str("6-digit TOTP code or a recovery code")}, "pre_auth_token", "code"), R: ref("LoginResponse")},
	{M: "POST /api/v1/auth/logout", T: tagAuth, S: "Sign out", D: "Ends the session; safe to call without one.", P: "public", R: okResp},
	{M: "POST /api/v1/auth/forgot", T: tagAuth, S: "Request a password reset link", D: "Always answers 200 (no account enumeration). Sends an e-mail when SMTP is configured and the login matches an account with an e-mail address.", P: "public",
		B: obj(S{"login": str("Username or e-mail")}, "login"), R: obj(S{"ok": boolean(""), "mail_configured": boolean("whether the server can send e-mail")})},
	{M: "POST /api/v1/auth/reset", T: tagAuth, S: "Set a new password with a reset token", P: "public",
		B: obj(S{"token": str("Token from the reset link"), "password": str("New password (policy: minimum length, mixed classes)")}, "token", "password"), R: okResp},
	{M: "POST /api/v1/auth/register-request", T: tagAuth, S: "Ask for an account", D: "Self-service request handled by an administrator under *Users → Account requests*. Rate limited.", P: "public",
		B: obj(S{"full_name": str(""), "email": str(""), "message": str("optional")}, "full_name", "email"), R: okResp},

	// ---- account
	{M: "GET /api/v1/auth/me", T: tagAccount, S: "Who am I", D: "The signed-in principal: user, organization, roles, effective permissions and the CSRF token for cookie sessions.", R: ref("Me")},
	{M: "POST /api/v1/auth/password", T: tagAccount, S: "Change my password", B: obj(S{"current": str(""), "new": str("")}, "current", "new"), R: okResp},
	{M: "POST /api/v1/auth/totp/setup", T: tagAccount, S: "Start TOTP enrolment", D: "Returns the secret, an otpauth URL and a QR code (data URL). Confirm with `POST /api/v1/auth/totp/confirm`.", R: obj(S{"secret": str(""), "otpauth_url": str(""), "qr": str("data:image/png;base64,…")})},
	{M: "POST /api/v1/auth/totp/confirm", T: tagAccount, S: "Confirm TOTP enrolment", D: "Returns the one-time recovery codes; store them safely.", B: obj(S{"code": str("Current TOTP code")}, "code"), R: obj(S{"ok": boolean(""), "recovery_codes": arr(str(""))})},
	{M: "PUT /api/v1/auth/locale", T: tagAccount, S: "Set my UI language", B: obj(S{"locale": str("Enabled locale code, e.g. sk")}, "locale"), R: obj(S{"locale": str("")})},
	{M: "GET /api/v1/push/key", T: tagAccount, S: "VAPID public key for Web Push", D: "Pass it as `applicationServerKey` to `PushManager.subscribe` in the browser.", R: obj(S{"public_key": str("base64url")})},
	{M: "GET /api/v1/push/subscriptions", T: tagAccount, S: "My browsers subscribed to notifications", D: "Keys are never returned; subscriptions revoked in the last 7 days are listed with `revoked_at`.", R: arr(ref("PushSubscription"))},
	{M: "POST /api/v1/push/subscriptions", T: tagAccount, S: "Register this browser for Web Push", D: "The JSON of the browser's `PushSubscription` (`endpoint`, `keys.p256dh`, `keys.auth`). Registering the same endpoint again refreshes the keys (200). Not allowed with an API token.", C: 201,
		B: obj(S{"endpoint": str("push service URL"), "keys": obj(S{"p256dh": str("base64url"), "auth": str("base64url")}, "p256dh", "auth"), "user_agent": str("optional label of the browser"), "expirationTime": nullable(anyVal("ignored"))}, "endpoint", "keys"), R: obj(S{"id": uuid(""), "created": boolean("")})},
	{M: "DELETE /api/v1/push/subscriptions/{id}", T: tagAccount, S: "Remove one of my browser subscriptions", R: okResp},
	{M: "POST /api/v1/push/test", T: tagAccount, S: "Send a test notification to my browsers", B: anyObj("empty object"), R: obj(S{"sent": integer(""), "failed": integer("")})},
	{M: "GET /api/v1/permissions", T: tagAccount, S: "Permission catalog", R: arr(ref("Permission"))},
	{M: "GET /api/v1/roles", T: tagAccount, S: "Roles of the organization", D: "Built-in roles (immutable) and organization roles with their permission sets and member counts.", R: arr(ref("Role"))},

	// ---- users and roles
	{M: "GET /api/v1/users", T: tagUsers, S: "List users", P: "user.admin", R: arr(ref("User"))},
	{M: "POST /api/v1/users", T: tagUsers, S: "Create a user", D: "Returns a one-time initial password that must be changed at first sign-in.", P: "user.admin", C: 201,
		B: obj(S{"username": str(""), "full_name": str(""), "email": str(""), "roles": arr(str("role name")), "reason": reason}, "username", "roles", "reason"), R: obj(S{"user": ref("User"), "initial_password": str("shown once")})},
	{M: "GET /api/v1/users/{id}", T: tagUsers, S: "User detail", D: "Profile, effective permissions, role history and API tokens.", P: "user.admin", R: obj(S{"user": ref("User"), "permissions": arr(str("")), "role_history": arr(anyObj("grant/revoke rows")), "api_tokens": arr(ref("APIToken"))})},
	{M: "PATCH /api/v1/users/{id}", T: tagUsers, S: "Update a user", D: "Roles, disabled flag, name, e-mail. Every role change is a row in the role history.", P: "user.admin",
		B: obj(S{"roles": arr(str("")), "disabled": boolean(""), "full_name": str(""), "email": str(""), "reason": reason}, "reason"), R: ref("User")},
	{M: "POST /api/v1/users/{id}/reset-password", T: tagUsers, S: "Reset a user's password", D: "Returns a new one-time password.", P: "user.admin", B: reasonBody, R: obj(S{"initial_password": str(""), "by": str("")})},
	{M: "POST /api/v1/users/{id}/mfa-reset", T: tagUsers, S: "Remove a user's MFA enrolment", P: "user.admin", B: reasonBody, R: obj(S{"username": str(""), "by": str("")})},
	{M: "POST /api/v1/users/{id}/sessions/revoke", T: tagUsers, S: "Sign a user out everywhere", P: "user.admin", B: reasonBody, R: obj(S{"username": str(""), "sessions": integer("revoked sessions"), "by": str("")})},
	{M: "POST /api/v1/users/{id}/unlock", T: tagUsers, S: "Unlock a locked-out user", P: "user.admin", B: reasonBody, R: obj(S{"username": str(""), "by": str("")})},
	{M: "POST /api/v1/roles", T: tagUsers, S: "Create an organization role", P: "user.admin", C: 201, B: obj(S{"name": str(""), "permissions": arr(str("permission name")), "reason": reason}, "name", "permissions", "reason"), R: ref("Role")},
	{M: "PUT /api/v1/roles/{id}", T: tagUsers, S: "Replace the permissions of an organization role", D: "Built-in roles cannot be changed.", P: "user.admin", B: obj(S{"permissions": arr(str("")), "reason": reason}, "permissions", "reason"), R: ref("Role")},
	{M: "GET /api/v1/account-requests", T: tagUsers, S: "Open account requests", P: "user.admin", R: arr(ref("AccountRequest"))},
	{M: "POST /api/v1/account-requests/{id}/created", T: tagUsers, S: "Close an account request as fulfilled", P: "user.admin", B: reasonBody, R: okResp},
	{M: "POST /api/v1/account-requests/{id}/dismiss", T: tagUsers, S: "Dismiss an account request", P: "user.admin", B: reasonBody, R: okResp},

	// ---- API tokens
	{M: "GET /api/v1/tokens", T: tagTokens, S: "My API tokens", P: "token.manage", R: arr(ref("APIToken"))},
	{M: "POST /api/v1/tokens", T: tagTokens, S: "Create an API token", D: "The raw token is returned once. Scopes must be a subset of the caller's permissions.", P: "token.manage", C: 201,
		B: obj(S{"name": str(""), "scopes": arr(str("permission name")), "expires_at": nullable(ts("optional expiry"))}, "name", "scopes"), R: obj(S{"token": str("raw token, shown once"), "info": ref("APIToken")})},
	{M: "DELETE /api/v1/tokens/{id}", T: tagTokens, S: "Revoke an API token", P: "token.manage", R: okResp},

	// ---- organizations
	{M: "GET /api/v1/organizations", T: tagOrgs, S: "List organizations", P: "system.admin", R: arr(ref("Organization"))},
	{M: "POST /api/v1/organizations", T: tagOrgs, S: "Create an organization with its first administrator", P: "system.admin", C: 201,
		B: obj(S{"slug": str("URL-safe identifier"), "name": str(""), "tz": str("IANA time zone"), "locale": str(""), "admin_username": str(""), "admin_full_name": str(""), "admin_email": str(""), "reason": reason}, "slug", "name", "admin_username", "reason"),
		R: obj(S{"organization": ref("Organization"), "admin": obj(S{"username": str(""), "initial_password": str("shown once")})})},
	{M: "PATCH /api/v1/organizations/{id}", T: tagOrgs, S: "Update or disable an organization", P: "system.admin", B: obj(S{"name": str(""), "tz": str(""), "locale": str(""), "disabled": boolean(""), "reason": reason}, "reason"), R: ref("Organization")},
	{M: "POST /api/v1/organizations/{id}/reset-admin", T: tagOrgs, S: "Reset the organization administrator's password", P: "system.admin", B: reasonBody, R: obj(S{"username": str(""), "initial_password": str("shown once")})},
	{M: "GET /api/v1/export/organization", T: tagOrgs, S: "Export an organization as a ZIP archive", D: "CSV files of every table of the organization for a period, with a manifest and SHA-256 checksums.", P: "system.admin", Q: []apiParam{q("from", "RFC 3339 start (optional)"), q("to", "RFC 3339 end (optional)")}, CT: "application/zip", RD: "ZIP archive"},

	// ---- sites and assets
	{M: "GET /api/v1/sites", T: tagSites, S: "List sites", P: "data.read", R: arr(ref("Site"))},
	{M: "POST /api/v1/sites", T: tagSites, S: "Create a site", P: "config.write", C: 201, B: obj(S{"name": str(""), "tz": str("IANA time zone, default Europe/Bratislava"), "address": str(""), "reason": reason}, "name", "reason"), R: ref("Site")},
	{M: "PATCH /api/v1/sites/{id}", T: tagSites, S: "Update a site", P: "config.write", B: obj(S{"name": str(""), "tz": str(""), "address": str(""), "reason": reason}, "reason"), R: ref("Site")},
	{M: "GET /api/v1/assets", T: tagSites, S: "List assets", P: "data.read", Q: []apiParam{q("site_id", "only assets of this site")}, R: arr(ref("Asset"))},
	{M: "POST /api/v1/assets", T: tagSites, S: "Create an asset", P: "config.write", C: 201, B: obj(S{"site_id": uuid(""), "parent_id": nullable(uuid("parent asset")), "type": str("fridge, room, meter, machine, …"), "name": str(""), "attributes": anyObj("free JSON"), "reason": reason}, "site_id", "type", "name", "reason"), R: ref("Asset")},
	{M: "PATCH /api/v1/assets/{id}", T: tagSites, S: "Update an asset", P: "config.write", B: obj(S{"name": str(""), "type": str(""), "attributes": anyObj(""), "reason": reason}, "reason"), R: ref("Asset")},

	// ---- datastreams and observations
	{M: "GET /api/v1/datastreams", T: tagData, S: "List datastreams", P: "data.read", R: arr(ref("Datastream"))},
	{M: "POST /api/v1/datastreams", T: tagData, S: "Create a datastream", P: "config.write", C: 201,
		B: obj(S{"asset_id": uuid(""), "key": str("what the device publishes, ^[a-z][a-z0-9_]{0,31}$"), "quantity": str("temperature, humidity, power_active, …"), "unit": str(""), "kind": enum("", "gauge", "counter", "state", "event"), "expected_interval": str("PostgreSQL interval for gap detection, e.g. 5 minutes; empty = none"), "accuracy": nullable(num("required accuracy in the unit")), "reason": reason}, "asset_id", "key", "quantity", "kind", "reason"), R: ref("Datastream")},
	{M: "GET /api/v1/datastreams/{id}", T: tagData, S: "Datastream detail", D: "Datastream, asset, site, source device, last observation and current alarm rules.", P: "data.read", R: obj(S{"datastream": ref("Datastream"), "asset": anyObj(""), "site": anyObj(""), "device": nullable(anyObj("assigned device")), "last_observation": nullable(ref("Observation")), "alarm_rules": arr(ref("AlarmRule")), "rules": arr(obj(S{"id": uuid(""), "name": str(""), "status": str(""), "enabled": boolean(""), "role": enum("automation rules that read (input) or write (derive) this datastream", "input", "derive")}))})},
	{M: "PATCH /api/v1/datastreams/{id}", T: tagData, S: "Update a datastream", P: "config.write", B: obj(S{"quantity": str(""), "unit": str(""), "expected_interval": str(""), "accuracy": nullable(num("")), "reason": reason}, "reason"), R: ref("Datastream")},
	{M: "GET /api/v1/observations", T: tagData, S: "Observations of a datastream", D: "Raw rows ordered by time, or aggregated per bucket (`bucket` = seconds: avg/min/max/count per bucket, at most 20000 buckets). Raw pages are bounded by `limit` (default 1000, max 10000) and 366 days per request; when more rows exist the response carries the header `X-Next-Cursor` (and a `Link: <…>; rel=\"next\"`), pass it as `cursor` to continue.", P: "data.read",
		Q: []apiParam{qr("datastream_id", ""), q("from", "RFC 3339 (default: 24 hours ago)"), q("to", "RFC 3339 (default: now)"), qi("bucket", "aggregation bucket in seconds; omitted = raw rows"), qi("limit", "max rows per page"), q("cursor", "X-Next-Cursor of the previous page")}, R: S{"oneOf": []any{arr(ref("Observation")), ref("ObservationBuckets")}, "description": "raw rows, or buckets when bucket is set"}},
	{M: "GET /api/v1/datastreams/latest", T: tagData, S: "Latest value of several datastreams", P: "data.read", Q: []apiParam{qr("ids", "comma-separated datastream ids")}, R: S{"type": "object", "additionalProperties": ref("LatestValue"), "description": "keyed by datastream id"}},

	// ---- devices
	{M: "GET /api/v1/devices", T: tagDevices, S: "List devices", P: "data.read", R: arr(ref("Device"))},
	{M: "POST /api/v1/devices", T: tagDevices, S: "Register a device", D: "Returns a claim code for self-provisioning (see `POST /api/v1/provision`); a token can also be issued with `POST /api/v1/devices/{id}/token`.", P: "device.manage", C: 201,
		B: obj(S{"external_id": str("MQTT client id / DevEUI / connector id"), "name": str(""), "transport": enum("default mqtt", "mqtt", "http", "lorawan", "opcua", "modbus"), "reason": reason}, "external_id", "name", "reason"), R: obj(S{"id": uuid(""), "external_id": str(""), "name": str(""), "transport": str(""), "claim_code": str(""), "claim_expires_in_hours": integer("")})},
	{M: "GET /api/v1/devices/{id}", T: tagDevices, S: "Device detail", D: "Device, its datastream assignments (current and past) and credentials (no secrets).", P: "data.read", R: obj(S{"device": ref("Device"), "assignments": arr(anyObj("")), "credentials": arr(anyObj(""))})},
	{M: "PATCH /api/v1/devices/{id}", T: tagDevices, S: "Rename a device or set its location", P: "device.manage", B: obj(S{"name": str(""), "location": nullable(obj(S{"lat": num(""), "lon": num("")})), "clear_location": boolean(""), "reason": reason}, "reason"), R: ref("Device")},
	{M: "POST /api/v1/devices/{id}/token", T: tagDevices, S: "Issue or rotate the device token", D: "The previous token stops working at once; the new one is returned once.", P: "device.manage", B: reasonBody, R: obj(S{"external_id": str(""), "token": str("shown once")})},
	{M: "POST /api/v1/devices/{id}/revoke", T: tagDevices, S: "Revoke a device", D: "Disconnects it and rejects its credentials; data stays.", P: "device.manage", B: reasonBody, R: okResp},
	{M: "POST /api/v1/devices/{id}/datastreams", T: tagDevices, S: "Assign a datastream to the device", D: "The device's value with the datastream's key is stored into it; a previous assignment of the datastream is closed. With the policy `four_eyes_assignments` the call answers 202 with a pending change request instead.", P: "device.manage", C: 201, B: obj(S{"datastream_id": uuid(""), "reason": reason}, "datastream_id", "reason"), R: obj(S{"id": uuid("assignment id"), "device_id": uuid(""), "datastream_id": uuid(""), "closed_previous": boolean("")})},
	{M: "DELETE /api/v1/devices/{id}/datastreams/{ds}", T: tagDevices, S: "End a datastream assignment", P: "device.manage", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/devices/{id}/messages", T: tagDevices, S: "Raw messages of a device", D: "Every stored message in both directions with decode status (default 50, max 500).", P: "data.read", Q: []apiParam{qi("limit", "")}, R: arr(ref("Message"))},
	{M: "GET /api/v1/devices/{id}/attributes", T: tagDevices, S: "Shared and client attributes", P: "data.read", R: obj(S{"shared": anyObj("set by the server, pushed to the device"), "client": anyObj("reported by the device")})},
	{M: "PUT /api/v1/devices/{id}/attributes", T: tagDevices, S: "Set or remove shared attributes", D: "Merged into the retained MQTT attributes message.", P: "device.manage", B: obj(S{"attributes": anyObj("keys to set"), "remove": arr(str("keys to remove")), "reason": reason}, "reason"), R: obj(S{"shared": anyObj(""), "published": boolean("")})},

	// ---- commands
	{M: "GET /api/v1/devices/{id}/commands", T: tagCommands, S: "Command log of a device", D: "Newest first (default 50, max 500). Commands past their deadline are marked expired.", P: "data.read", Q: []apiParam{qi("limit", "")}, R: arr(ref("Command"))},
	{M: "POST /api/v1/devices/{id}/commands", T: tagCommands, S: "Send a command or write a value", D: "MQTT devices receive `{id, m, a, exp}` on `d/{external_id}/cmd` and acknowledge on `d/{external_id}/cmd/ack`. OPC UA and Modbus devices accept `method: write` with `args: {key, value}` for a writable node/register (limits checked, value read back). With the four-eyes policy the command is stored as `pending` (202) until another user approves it.", P: "device.command", C: 201,
		B: obj(S{"method": str("^[a-z][a-z0-9_]{0,31}$"), "args": anyObj("method arguments"), "ttl_seconds": integer("deadline for the acknowledgement (default 300)"), "reason": reason}, "method", "reason"), R: ref("CommandResult")},
	{M: "POST /api/v1/devices/{id}/commands/{cid}/approve", T: tagCommands, S: "Approve a pending command (four eyes)", D: "Must be a different user than the issuer; the command is then executed.", P: "device.command", B: reasonBody, R: ref("CommandResult")},
	{M: "POST /api/v1/devices/{id}/commands/{cid}/reject", T: tagCommands, S: "Reject a pending command", P: "device.command", B: reasonBody, R: ref("CommandResult")},
	{M: "GET /api/v1/settings/policy", T: tagCommands, S: "Organization security policy", P: "config.write", R: ref("Policy")},
	{M: "PUT /api/v1/settings/policy", T: tagCommands, S: "Set the four-eyes policies", D: "`four_eyes_commands`: commands wait for approval. `four_eyes_limits`: alarm rule changes become change requests. `four_eyes_assignments`: device ↔ datastream assignments become change requests. At least one field.", P: "user.admin", B: obj(S{"four_eyes_commands": boolean(""), "four_eyes_limits": boolean(""), "four_eyes_assignments": boolean(""), "reason": reason}, "reason"), R: obj(S{"four_eyes_commands": boolean(""), "four_eyes_limits": boolean(""), "four_eyes_assignments": boolean("")})},
	{M: "GET /api/v1/change-requests", T: tagCommands, S: "Change requests waiting for a second person", D: "Alarm rule and assignment changes held by the four-eyes policies (`status=pending`, default) or the full history (`status=all`). Requires config.write or device.manage.", Q: []apiParam{q("status", "pending (default) | approved | rejected | expired | failed | all")}, R: arr(ref("ChangeRequest"))},
	{M: "POST /api/v1/change-requests/{id}/approve", T: tagCommands, S: "Approve a change request", D: "A different user than the requester, holding the permission of the change (config.write for limits, device.manage for assignments). The original call is replayed under the approver's identity; `code` and `result` are its response.", B: reasonBody, R: obj(S{"id": uuid(""), "status": enum("", "approved", "failed"), "code": integer("HTTP status of the replayed call"), "result": anyVal("")})},
	{M: "POST /api/v1/change-requests/{id}/reject", T: tagCommands, S: "Reject a change request", B: reasonBody, R: obj(S{"id": uuid(""), "status": str("")})},

	// ---- console
	{M: "GET /api/v1/devices/{id}/console", T: tagConsole, S: "Console sessions of a device", P: "device.console", R: obj(S{"open": nullable(ref("ConsoleSession")), "sessions": arr(ref("ConsoleSession")), "idle_timeout_minutes": integer("")})},
	{M: "POST /api/v1/devices/{id}/console", T: tagConsole, S: "Open a console session", P: "device.console", C: 201, B: reasonBody, R: obj(S{"session_id": uuid(""), "external_id": str("")})},
	{M: "GET /api/v1/devices/{id}/console/{sid}", T: tagConsole, S: "Read console output", D: "Lines after a sequence number (poll with `after`).", P: "device.console", Q: []apiParam{qi("after", "last sequence number seen"), qi("limit", "")}, R: obj(S{"session": ref("ConsoleSession"), "lines": arr(obj(S{"seq": integer(""), "ts": ts(""), "dir": enum("", "in", "out"), "text": str("")}))})},
	{M: "POST /api/v1/devices/{id}/console/{sid}/in", T: tagConsole, S: "Send a line to the device", P: "device.console", B: obj(S{"line": str("")}, "line"), R: okResp},
	{M: "DELETE /api/v1/devices/{id}/console/{sid}", T: tagConsole, S: "Close a console session", P: "device.console", Q: []apiParam{qr("reason", "")}, R: okResp},

	// ---- firmware
	{M: "GET /api/v1/firmware", T: tagFota, S: "List firmware images", P: "fota.read", R: arr(ref("Firmware"))},
	{M: "POST /api/v1/firmware", T: tagFota, S: "Upload a firmware image", D: "multipart/form-data. The SHA-256 is computed on the server; an Ed25519 signature may be attached for devices that verify it.", P: "fota.manage", C: 201,
		F: []apiParam{qr("file", "binary image"), qr("name", ""), qr("version", ""), qr("hw", "hardware id the image is for"), q("min_version", "minimum device version that may install it"), q("signature", "base64 Ed25519 signature"), q("notes", ""), qr("reason", "")}, R: ref("Firmware")},
	{M: "GET /api/v1/fota/campaigns", T: tagFota, S: "List update campaigns", P: "fota.read", R: arr(ref("Campaign"))},
	{M: "POST /api/v1/fota/campaigns", T: tagFota, S: "Start an update campaign", D: "Publishes the update offer to the selected devices; progress arrives on `d/{id}/fota/status`.", P: "fota.manage", C: 201, B: obj(S{"firmware_id": uuid(""), "device_ids": arr(uuid("")), "name": str(""), "reason": reason}, "firmware_id", "device_ids", "reason"), R: obj(S{"id": uuid(""), "name": str(""), "firmware_id": uuid(""), "version": str(""), "devices": integer(""), "published": integer("")})},
	{M: "GET /api/v1/fota/campaigns/{id}", T: tagFota, S: "Campaign detail with per-device status", P: "fota.read", R: obj(S{"campaign": ref("Campaign"), "devices": arr(obj(S{"device_id": uuid(""), "external_id": str(""), "name": str(""), "status": str(""), "pct": integer(""), "version": str(""), "error": nullable(str("")), "updated_at": ts(""), "device_fw": str("")}))})},
	{M: "DELETE /api/v1/fota/campaigns/{id}", T: tagFota, S: "Cancel a campaign", P: "fota.manage", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/d/fota/{fw}/blob", T: tagFota, S: "Download a firmware image (device)", D: "Called by the device with its device token (`Authorization: Bearer`), supports HTTP range requests.", P: "device", CT: "application/octet-stream", RD: "firmware bytes"},
	{M: "POST /api/v1/d/telemetry", T: tagDevAPI, S: "Send telemetry (device)", D: "One sample or an array. Accepted shapes: ctrl32 `{\"ts\":1700000000,\"v\":{\"temp\":4.2}}`, ThingsBoard `{\"ts\":1700000000000,\"values\":{…}}`, or a flat object `{\"temp\":4.2}` (every non-reserved key is a value). `ts` in seconds or milliseconds, `bf: 1` for backfill, `seq` for deduplication. Keys are normalised (lower case, `-`/`.`/space → `_`) and must match a datastream assigned to the device; unknown keys are reported and stored as a raw message. Up to 64 KiB, 2000 samples.", P: "device", C: 202,
		B: anyVal("Sample or array of samples"), R: obj(S{"accepted": integer("Observations stored"), "status": str("ok, unknown_key or error"), "unknown_keys": arr(str("")), "error": str("")})},
	{M: "POST /api/v1/d/attributes", T: tagDevAPI, S: "Report client attributes (device)", D: "JSON object such as `{\"fw\":\"1.4.2\",\"hw\":\"esp32-s3\",\"model\":\"…\",\"ip\":\"10.0.0.12\"}`; merged into the device's client attributes.", P: "device", C: 202, B: anyObj("Attributes"), R: obj(S{"ok": boolean("")})},
	{M: "POST /api/v1/d/status", T: tagDevAPI, S: "Heartbeat (device)", D: "`{\"online\":true,\"bat\":3.61,\"rssi\":-71,\"up_s\":86400,\"free\":120000,\"fw\":\"1.4.2\",\"buf\":12}`; `lat`/`lon` update the reported position.", P: "device", C: 202, B: anyObj("Status"), R: obj(S{"ok": boolean("")})},
	{M: "GET /api/v1/d/attributes/shared", T: tagDevAPI, S: "Shared attributes (device)", D: "The configuration the server holds for the device (the retained `attributes/shared` document over MQTT).", P: "device", R: anyObj("Shared attributes")},
	{M: "GET /api/v1/d/cmd", T: tagDevAPI, S: "Fetch queued commands (device, long polling)", D: "Returns the queued commands of the device and marks them sent. `wait` (0–60 s) keeps the request open until a command appears. Devices registered with transport `http` receive commands only this way; acknowledge each with `POST /api/v1/d/cmd/{id}/ack`.", P: "device",
		Q: []apiParam{q("wait", "Seconds to wait for a command (0–60)")}, R: arr(obj(S{"id": uuid(""), "m": str("Method"), "a": anyObj("Arguments"), "exp": integer("Unix time after which the command must not be executed")}))},
	{M: "POST /api/v1/d/cmd/{id}/ack", T: tagDevAPI, S: "Acknowledge a command (device)", D: "`{\"ok\":true,\"r\":{…}}` or `{\"ok\":false,\"err\":\"…\"}`; 404 when the command is not open for this device.", P: "device",
		B: obj(S{"ok": boolean(""), "r": anyVal("Result"), "err": str("")}, "ok"), R: obj(S{"id": uuid(""), "status": str("acked or failed")})},
	{M: "POST /api/v1/d/fota/status", T: tagDevAPI, S: "Report firmware update progress (device)", D: "`{\"fw_id\":\"…\",\"st\":\"downloading|verifying|applying|updated|failed|rolled_back\",\"pct\":42,\"ver\":\"1.5.0\",\"err\":\"…\"}`.", P: "device", C: 202, B: anyObj("FOTA status"), R: obj(S{"ok": boolean("")})},

	// ---- alarms
	{M: "GET /api/v1/alarm-rules", T: tagAlarms, S: "Current alarm rules", P: "data.read", Q: []apiParam{q("datastream_id", "only rules of this datastream")}, R: arr(ref("AlarmRule"))},
	{M: "POST /api/v1/alarm-rules", T: tagAlarms, S: "Create an alarm rule (new version)", D: "A rule of the same type on the same datastream is superseded (its `valid_to` is set); the new row carries the next version number. With the policy `four_eyes_limits` the call answers 202 with a pending change request instead.", P: "config.write", C: 201,
		B: obj(S{"datastream_id": uuid(""), "type": enum("", "high", "low", "comm_loss", "sensor_fault", "low_battery", "power_loss", "door_open"), "warning": nullable(num("warning limit")), "action": nullable(num("action limit")), "delay_seconds": num(""), "hysteresis": num(""), "escalation_policy_id": str("optional; empty = global e-mail recipients"), "reason": reason}, "datastream_id", "type", "reason"), R: ref("AlarmRule")},
	{M: "DELETE /api/v1/alarm-rules/{id}", T: tagAlarms, S: "Disable an alarm rule", P: "config.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/alarms", T: tagAlarms, S: "Alarms, newest first (200)", P: "data.read", Q: []apiParam{q("datastream_id", "")}, R: arr(ref("Alarm"))},
	{M: "GET /api/v1/incidents", T: tagAlarms, S: "List incidents", D: "Incident log (URS C-07, I-09): an alarm whose escalation step could not be delivered and a broken audit trail chain open incidents automatically; operators add their own. Each incident carries impact, cause and CAPA (corrective and preventive action). `status` = `open` (default: open + investigating), `all`, or one status. System administrators also see instance-level incidents (no organization).",
		Q: []apiParam{q("status", "open, all, investigating, resolved or closed")}, R: arr(ref("Incident"))},
	{M: "GET /api/v1/incidents/{id}", T: tagAlarms, S: "Incident with its history", R: obj(S{"incident": ref("Incident"), "events": arr(ref("IncidentEvent"))})},
	{M: "POST /api/v1/incidents", T: tagAlarms, S: "Open an incident by hand", P: "alarm.ack", C: 201,
		B: obj(S{"title": str("Up to 200 characters"), "severity": str("minor, major (default) or critical"), "impact": str(""), "cause": str(""), "capa": str(""), "reason": str("Audit trail")}, "title", "reason"), R: ref("Incident")},
	{M: "PATCH /api/v1/incidents/{id}", T: tagAlarms, S: "Update an incident: fields, a note, the status", D: "Transitions: open → investigating, resolved or closed; investigating → open, resolved or closed; resolved → investigating or closed; closed → investigating (reopen). Resolving needs a cause, closing a cause and a CAPA. Every change becomes an incident event and an audit trail entry.", P: "alarm.ack",
		B: obj(S{"status": str("New status (optional)"), "impact": str(""), "cause": str(""), "capa": str(""), "note": str("Free text appended to the history"), "reason": str("Audit trail")}, "reason"), R: ref("Incident")},
	{M: "POST /api/v1/alarms/{id}/ack", T: tagAlarms, S: "Acknowledge an active alarm", D: "Stops the escalation ladder; recorded in the alarm log with the actor and reason.", P: "alarm.ack", B: reasonBody, R: okResp},
	{M: "GET /api/v1/audit/verify", T: tagAlarms, S: "Verify the audit hash chain", P: "data.read", R: obj(S{"ok": boolean(""), "checked": integer("rows"), "first_bad_seq": nullable(integer("")), "head_seq": integer(""), "head_hash": str("")})},

	// ---- notifications
	{M: "GET /api/v1/notification-channels", T: tagNotify, S: "List channels", P: "config.write", R: arr(ref("Channel"))},
	{M: "POST /api/v1/notification-channels", T: tagNotify, S: "Create a channel", P: "config.write", C: 201, B: obj(S{"name": str(""), "kind": enum("", "email", "webhook", "sms_http", "voice_http", "web_push"), "settings": ref("ChannelSettings"), "reason": reason}, "name", "kind", "settings", "reason"), R: ref("Channel")},
	{M: "PUT /api/v1/notification-channels/{id}", T: tagNotify, S: "Update a channel", D: "A blank webhook secret keeps the stored one.", P: "config.write", B: obj(S{"name": str(""), "settings": ref("ChannelSettings"), "enabled": boolean(""), "reason": reason}, "reason"), R: ref("Channel")},
	{M: "DELETE /api/v1/notification-channels/{id}", T: tagNotify, S: "Revoke a channel", P: "config.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "POST /api/v1/notification-channels/{id}/test", T: tagNotify, S: "Send a test message", D: "Logged in the delivery log like any notification.", P: "config.write", B: anyObj("empty object"), R: obj(S{"ok": boolean(""), "channel": str(""), "kind": str("")})},
	{M: "GET /api/v1/escalation-policies", T: tagNotify, S: "List escalation policies", P: "config.write", R: arr(ref("EscalationPolicy"))},
	{M: "POST /api/v1/escalation-policies", T: tagNotify, S: "Create an escalation policy", D: "Step 0 (after_s = 0) is delivered at once; later steps only while the alarm is still active and unacknowledged.", P: "config.write", C: 201, B: obj(S{"name": str(""), "steps": arr(ref("EscalationStep")), "reason": reason}, "name", "steps", "reason"), R: ref("EscalationPolicy")},
	{M: "PUT /api/v1/escalation-policies/{id}", T: tagNotify, S: "Update a policy (new version in place)", P: "config.write", B: obj(S{"name": str(""), "steps": arr(ref("EscalationStep")), "reason": reason}, "steps", "reason"), R: ref("EscalationPolicy")},
	{M: "DELETE /api/v1/escalation-policies/{id}", T: tagNotify, S: "Revoke a policy", D: "Refused (409) while alarm rules use it.", P: "config.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/notifications", T: tagNotify, S: "Delivery log, newest first", P: "config.write", Q: []apiParam{qi("limit", "default 50, max 500")}, R: arr(ref("Notification"))},

	// ---- automation rules
	{M: "GET /api/v1/rules", T: tagRules, S: "List automation rules", D: "With the live state of each loaded rule (`state.active` = condition currently true).", P: "config.write", R: arr(ref("Rule"))},
	{M: "POST /api/v1/rules", T: tagRules, S: "Create an automation rule", D: "Rules that send commands need `device.command`; with the four-eyes policy they are stored as `pending` (202) until a different user approves them.", P: "config.write", C: 201, B: obj(S{"name": str("unique in the organization"), "description": str(""), "spec": ref("RuleSpec"), "reason": reason}, "name", "spec", "reason"), R: obj(S{"id": uuid(""), "name": str(""), "status": enum("", "active", "pending"), "version": integer("")})},
	{M: "PUT /api/v1/rules/{id}", T: tagRules, S: "Update a rule (new version in place when the spec changes)", P: "config.write", B: obj(S{"name": str(""), "description": str(""), "spec": ref("RuleSpec"), "enabled": boolean(""), "reason": reason}, "reason"), R: obj(S{"id": uuid(""), "name": str(""), "status": str(""), "version": integer(""), "enabled": boolean("")})},
	{M: "DELETE /api/v1/rules/{id}", T: tagRules, S: "Revoke a rule", D: "Alarms raised by the rule are cleared.", P: "config.write", Q: []apiParam{qr("reason", "")}, R: obj(S{"id": uuid(""), "revoked": boolean("")})},
	{M: "POST /api/v1/rules/{id}/approve", T: tagRules, S: "Approve a pending rule (four eyes)", P: "device.command", B: reasonBody, R: obj(S{"id": uuid(""), "status": str("")})},
	{M: "POST /api/v1/rules/{id}/reject", T: tagRules, S: "Reject a pending rule", P: "device.command", B: reasonBody, R: obj(S{"id": uuid(""), "status": str("")})},
	{M: "POST /api/v1/rules/test", T: tagRules, S: "Dry run a rule spec", D: "Evaluates the condition against the current values; no action runs.", P: "config.write", B: obj(S{"spec": ref("RuleSpec"), "rule_id": str("optional: include the live state of this rule")}, "spec"), R: ref("RuleEvaluation")},
	{M: "GET /api/v1/rules/{id}/as-flow", T: tagRules, S: "Convert a rule into a flow spec", D: "Returns an equivalent flow (datastream triggers → join → \"true for\" → actions) to create as a draft with `POST /api/v1/flows`; parts without a flow equivalent (staleness, webhook secrets, custom RPC arguments) are listed in `notes` and in a comment node. The rule itself is not changed.", P: "config.write", R: obj(S{"name": str(""), "spec": ref("FlowSpec"), "notes": arr(str("")), "problems": arr(ref("FlowProblem"))})},
	{M: "GET /api/v1/rule-runs", T: tagRules, S: "Rule transitions with their action results", P: "config.write", Q: []apiParam{q("rule_id", ""), qi("limit", "default 50, max 500")}, R: arr(ref("RuleRun"))},

	// ---- dashboards
	{M: "GET /api/v1/dashboards", T: tagDash, S: "List dashboards", P: "data.read", R: arr(ref("DashboardSummary"))},
	{M: "POST /api/v1/dashboards", T: tagDash, S: "Create a dashboard", D: "`spec` is screen format v1 (`GET /api/v1/dashboards/catalog` lists the widget types; `GET /api/v1/dashboards/starter` returns a ready-made spec).", P: "content.write", C: 201, B: obj(S{"slug": str("URL-safe; generated from the name when empty"), "name": str(""), "spec": ref("DashboardSpec"), "is_default": boolean(""), "reason": reason}, "name", "spec", "reason"), R: ref("Dashboard")},
	{M: "GET /api/v1/dashboards/{slug}", T: tagDash, S: "Dashboard with its spec", P: "data.read", R: ref("Dashboard")},
	{M: "PUT /api/v1/dashboards/{slug}", T: tagDash, S: "Save a new version of a dashboard", D: "Widget settings are validated against the catalogue; invalid values are refused, settings the catalogue does not know are dropped and listed in `warnings` of the response.", P: "content.write", B: obj(S{"name": str(""), "spec": ref("DashboardSpec"), "is_default": boolean(""), "reason": reason}, "reason"), R: ref("Dashboard")},
	{M: "DELETE /api/v1/dashboards/{slug}", T: tagDash, S: "Delete a dashboard", P: "content.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/dashboards/catalog", T: tagDash, S: "Widget catalog and time ranges", P: "data.read", R: obj(S{"widgets": arr(anyObj("widget type definition")), "ranges": arr(str(""))})},
	{M: "GET /api/v1/dashboards/starter", T: tagDash, S: "A starter dashboard spec built from the organization's data", P: "data.read", Q: []apiParam{q("kind", "overview (default), showcase, mimic (widget symbols on a canvas) or plant (mixing plant process picture)")}, R: obj(S{"kind": str(""), "name": str(""), "spec": ref("DashboardSpec")})},
	{M: "GET /api/v1/flows/catalog", T: tagFlows, S: "Node library", D: "Every node type with its group, ports and settings schema, and the expression functions.", P: "data.read", R: obj(S{"nodes": arr(anyObj("node type")), "functions": arr(str("")), "limits": anyObj("")})},
	{M: "GET /api/v1/flows", T: tagFlows, S: "Flows", P: "data.read", R: arr(obj(S{"flow": ref("Flow"), "running": boolean(""), "errors": integer(""), "dropped": integer("")}))},
	{M: "POST /api/v1/flows", T: tagFlows, S: "Create a flow (draft)", P: "content.write", C: 201, B: obj(S{"name": str(""), "spec": ref("FlowSpec"), "reason": reason}, "name", "reason"), R: obj(S{"flow": ref("Flow"), "problems": arr(ref("FlowProblem"))})},
	{M: "GET /api/v1/flows/{slug}", T: tagFlows, S: "A flow (current version)", D: "With validation problems, running status per node and the URLs of its webhook nodes.", P: "data.read", R: obj(S{"flow": ref("Flow"), "problems": arr(ref("FlowProblem")), "status": anyObj("running, version, queue, dropped, nodes {in, out, errors, last}"), "hooks": anyObj("node id → URL")})},
	{M: "PUT /api/v1/flows/{slug}", T: tagFlows, S: "Save a new version (draft)", D: "The deployed version keeps running until the new one is deployed.", P: "content.write", B: obj(S{"name": str(""), "spec": ref("FlowSpec"), "reason": reason}, "spec", "reason"), R: obj(S{"flow": ref("Flow"), "problems": arr(ref("FlowProblem"))})},
	{M: "DELETE /api/v1/flows/{slug}", T: tagFlows, S: "Delete a flow", P: "content.write", Q: []apiParam{qr("reason", "")}, R: anyObj("")},
	{M: "POST /api/v1/flows/{slug}/deploy", T: tagFlows, S: "Deploy the current version", D: "Refused (422) with the problems when the flow is not valid. A flow that sends commands needs device.command and, with the four-eyes policy for commands, becomes a change request (202) that another user approves.", P: "content.write", B: obj(S{"reason": reason}, "reason"), R: obj(S{"flow": ref("Flow"), "status": anyObj("")})},
	{M: "POST /api/v1/flows/{slug}/disable", T: tagFlows, S: "Stop a deployed flow", P: "content.write", B: obj(S{"reason": reason}, "reason"), R: obj(S{"flow": ref("Flow")})},
	{M: "GET /api/v1/flows/{slug}/versions", T: tagFlows, S: "Versions of a flow", P: "data.read", R: arr(ref("Flow"))},
	{M: "POST /api/v1/flows/{slug}/restore", T: tagFlows, S: "Restore a version", D: "Saves the old content as a new version (deploy it separately).", P: "content.write", B: obj(S{"version": integer(""), "reason": reason}, "version", "reason"), R: obj(S{"flow": ref("Flow"), "problems": arr(ref("FlowProblem"))})},
	{M: "POST /api/v1/flows/{slug}/inject", T: tagFlows, S: "Send a test message", D: "Into a node of the deployed flow (dry_run: actions are shown, not executed), or with `spec` into the edited flow (always a dry run).", P: "content.write", C: 202,
		B: obj(S{"node": str(""), "value": anyObj("number, text, bool or null"), "topic": str(""), "dry_run": boolean(""), "spec": ref("FlowSpec")}, "node"), R: anyObj("")},
	{M: "GET /api/v1/flows/{slug}/debug", T: tagFlows, S: "Debug panel entries", D: "The last 500 entries: debug nodes, errors, actions, dry-run actions, drops.", P: "data.read", Q: []apiParam{qi("after", "sequence number of the last entry seen")}, R: arr(obj(S{"seq": integer(""), "ts": ts(""), "node": str(""), "kind": str("debug, error, action, dry_action, drop, info"), "text": str(""), "msg": anyObj(""), "dry": boolean("")}))},
	{M: "GET /api/v1/flows/{slug}/status", T: tagFlows, S: "Counters of a running flow", P: "data.read", R: anyObj("running, version, queue, dropped, nodes")},
	{M: "GET /api/v1/flows/{slug}/export", T: tagFlows, S: "Export a flow (JSON)", P: "data.read", R: obj(S{"format": str("ctrl32-flow/1"), "name": str(""), "slug": str(""), "version": integer(""), "checksum": str(""), "spec": ref("FlowSpec")})},
	{M: "POST /api/v1/flows/import", T: tagFlows, S: "Import a flow", P: "content.write", C: 201, B: obj(S{"format": str("ctrl32-flow/1"), "name": str(""), "slug": str(""), "spec": ref("FlowSpec"), "reason": reason}, "format", "name", "spec", "reason"), R: obj(S{"flow": ref("Flow"), "problems": arr(ref("FlowProblem"))})},
	{M: "POST /api/v1/flows/validate", T: tagFlows, S: "Validate a flow", P: "data.read", B: obj(S{"spec": ref("FlowSpec")}, "spec"), R: obj(S{"ok": boolean(""), "problems": arr(ref("FlowProblem")), "has_commands": boolean("")})},
	{M: "GET /api/v1/flows/settings", T: tagFlows, S: "Flow settings", P: "data.read", R: obj(S{"allowed_hosts": arr(str("hosts HTTP request nodes may call; *.example.com allowed"))})},
	{M: "PUT /api/v1/flows/settings", T: tagFlows, S: "Change flow settings", P: "config.write", B: obj(S{"allowed_hosts": arr(str("")), "reason": reason}, "allowed_hosts", "reason"), R: obj(S{"allowed_hosts": arr(str(""))})},
	{M: "POST /api/v1/flow-hooks/{token}", T: tagFlows, S: "Incoming webhook of a flow", D: "The JSON body's value and topic become the message, other fields msg.<name>. With a secret on the node, X-Signature: sha256=<HMAC-SHA256 of the body> is required.", P: "public", C: 202, B: anyObj("JSON object"), R: anyObj("")},
	{M: "GET /api/v1/replication", T: tagRepl, S: "This node and its replicas", D: "Node identity and role (primary, replica, fenced) and every replica with mode, state, lag, copy progress, limits and detachment record.", P: "system.admin", R: obj(S{"node": ref("ReplicationNode"), "replicas": arr(ref("Replica")), "pending": arr(anyObj("pairing codes not used yet: id, name, mode, created_at, expires_at")), "modes": arr(str("")), "max_replicas": integer("")})},
	{M: "POST /api/v1/replication/pairings", T: tagRepl, S: "Create a one-time pairing code", D: "The code (XXXX-XXXX-XXXX) is shown once and is valid 15 minutes; on the second machine run `ctrl32-telemetry replica attach -primary <this URL> -code <code>`.", P: "system.admin", C: 201,
		B: obj(S{"mode": enum("", "copy", "continuous", "standby"), "name": str("≤ 64"), "lag_limit_s": integer("alarm when the replica is further behind (default 10)"), "reason": reason}, "mode", "name", "reason"), R: obj(S{"code": str(""), "expires_at": ts(""), "mode": str(""), "name": str("")})},
	{M: "PATCH /api/v1/replication/replicas/{id}", T: tagRepl, S: "Change a replica's mode or limits", P: "system.admin", B: obj(S{"mode": enum("", "copy", "continuous", "standby"), "lag_limit_s": integer(""), "max_retained_mb": integer("maximum WAL the primary keeps for it (64 MB – 10 TB)"), "standby_url": str("hot standby: its public URL"), "reason": reason}, "reason"), R: ref("Replica")},
	{M: "POST /api/v1/replication/replicas/{id}/detach", T: tagRepl, S: "Detach a replica safely", D: "Counts the record tables in one snapshot, writes the `replica.detach` audit entry, then the replica catches up to it, becomes independent and verifies the audit chain and the counts; the result is in `detach.verification`. Refused (409) when not in sync unless `accept_partial`.", P: "system.admin", C: 202,
		B: obj(S{"reason": reason, "accept_partial": boolean("detach a replica that is not in sync; the copy is marked partial")}, "reason"), R: ref("Replica")},
	{M: "DELETE /api/v1/replication/replicas/{id}", T: tagRepl, S: "Drop a replica", D: "Revokes its token and removes its replication role, slot and pg_hba rule; the copy stays as it is and is no longer fed.", P: "system.admin", Q: []apiParam{qr("reason", "")}, R: ref("Replica")},
	{M: "GET /api/v1/replication/replicas/{id}/events", T: tagRepl, S: "History of a replica", P: "system.admin", Q: []apiParam{qi("limit", "default 100")}, R: arr(obj(S{"ts": ts(""), "kind": str("state, alarm, command, detach, promote, security, config"), "detail": anyObj("")}))},
	{M: "POST /api/v1/replication/pair", T: tagRepl, S: "Pair a replica with a one-time code", D: "Used by `ctrl32-telemetry replica attach`. Rate-limited; a wrong or expired code is a security event (403). Versions of PostgreSQL (major) and ctrl32 must match (400).", P: "public", C: 201,
		B: obj(S{"code": str(""), "name": str(""), "hostname": str(""), "os": str(""), "pg_major": integer(""), "ctrl32_version": str(""), "standby_url": str("")}, "code", "pg_major", "ctrl32_version"),
		R: obj(S{"replica_id": uuid(""), "name": str(""), "mode": str(""), "token": str("bearer token of the replica (shown once)"), "role": str("replication role"), "password": str("its password (shown once)"), "slot": str(""), "primary_node": uuid(""), "pg_major": integer("")})},
	{M: "GET /api/v1/replication/tunnel", T: tagRepl, S: "Replication tunnel (HTTP upgrade)", D: "With `Connection: Upgrade`, `Upgrade: ctrl32-pgrepl` and the replica token the connection becomes a byte stream to the primary's PostgreSQL (101 Switching Protocols).", P: "replica token", C: 101},
	{M: "GET /api/v1/replication/agent", T: tagRepl, S: "Pending command for the replica (long poll)", P: "replica token", R: obj(S{"command": nullable(anyObj("detach command: lsn, seq, hash, snapshot, counts, partial"))})},
	{M: "POST /api/v1/replication/agent", T: tagRepl, S: "Report from the replica", P: "replica token", B: obj(S{"status": anyObj("replica's own view"), "result": anyObj("detach result: promoted, chain_ok, entry_ok, counts, errors")}), R: anyObj("")},
	{M: "GET /api/v1/replication/role", T: tagRepl, S: "Who this node is", D: "No authentication: a returning primary asks its hot standby whether it was promoted (fencing).", P: "public", R: ref("ReplicationNode")},
	{M: "GET /api/v1/hmi/objects", T: tagHMI, S: "Library objects for process pictures (current versions)", P: "data.read", Q: []apiParam{q("kind", "svg or composite (empty = all)")}, R: arr(anyObj("library object summary: slug, kind, name, category, version, width, height, created_at"))},
	{M: "POST /api/v1/hmi/objects", T: tagHMI, S: "Create a library object", D: "kind `composite`: body {elements, tags, width, height} — drawing elements (screen format v1) whose tags may be placeholders `$name` listed in `tags`; kind `svg`: body {svg}, sanitised like an import.", P: "content.write", C: 201, B: obj(S{"name": str(""), "slug": str(""), "kind": str("svg | composite"), "category": str(""), "body": anyObj("object body"), "reason": reason}, "name", "kind", "body", "reason"), R: anyObj("library object")},
	{M: "POST /api/v1/hmi/objects/import-svg", T: tagHMI, S: "Import an SVG graphic", D: "The SVG is sanitised by a whitelist (no script, event handlers, foreignObject, images, links, animations or external references); the response lists what was removed.", P: "content.write", C: 201, B: obj(S{"name": str(""), "slug": str(""), "category": str(""), "svg": str("SVG document, at most 512 KiB"), "reason": reason}, "name", "svg", "reason"), R: anyObj("library object with removed[]")},
	{M: "GET /api/v1/hmi/objects/{slug}", T: tagHMI, S: "Library object with its body", P: "data.read", R: anyObj("library object")},
	{M: "PUT /api/v1/hmi/objects/{slug}", T: tagHMI, S: "Save a new version of a library object", P: "content.write", B: obj(S{"name": str(""), "category": str(""), "body": anyObj("object body"), "reason": reason}, "body", "reason"), R: anyObj("library object")},
	{M: "DELETE /api/v1/hmi/objects/{slug}", T: tagHMI, S: "Retire a library object", P: "content.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/hmi/objects/{slug}/svg", T: tagHMI, S: "The sanitised SVG of a graphic", D: "Served as image/svg+xml with a sandbox Content-Security-Policy.", P: "data.read", CT: "image/svg+xml", RD: "SVG document"},
	{M: "GET /api/v1/dashboards/{slug}/share", T: tagDash, S: "Public share of a dashboard", P: "content.write", R: ref("Share")},
	{M: "POST /api/v1/dashboards/{slug}/share", T: tagDash, S: "Publish a dashboard under a public link", D: "Creates the share token; the response carries the public URL and a QR code.", P: "content.write", C: 201, B: reasonBody, R: ref("Share")},
	{M: "DELETE /api/v1/dashboards/{slug}/share", T: tagDash, S: "Withdraw the public link", P: "content.write", Q: []apiParam{qr("reason", "")}, R: ref("Share")},

	// ---- public dashboards
	{M: "GET /api/v1/public/{token}", T: tagPublic, S: "Shared dashboard", P: "public", R: obj(S{"slug": str(""), "name": str(""), "spec": ref("DashboardSpec")})},
	{M: "GET /api/v1/public/{token}/datastreams", T: tagPublic, S: "Datastreams bound in the shared dashboard", P: "public", R: arr(ref("Datastream"))},
	{M: "GET /api/v1/public/{token}/datastreams/latest", T: tagPublic, S: "Latest values (bound datastreams only)", P: "public", Q: []apiParam{qr("ids", "")}, R: S{"type": "object", "additionalProperties": ref("LatestValue")}},
	{M: "GET /api/v1/public/{token}/observations", T: tagPublic, S: "Observations (bound datastreams only)", D: "Same parameters and cursor pagination as `GET /api/v1/observations`.", P: "public", Q: []apiParam{qr("datastream_id", ""), q("from", ""), q("to", ""), qi("bucket", "seconds"), qi("limit", ""), q("cursor", "")}, R: S{"oneOf": []any{arr(ref("Observation")), ref("ObservationBuckets")}}},
	{M: "GET /api/v1/public/{token}/devices", T: tagPublic, S: "Devices shown in the shared dashboard", P: "public", R: arr(ref("Device"))},
	{M: "GET /api/v1/public/{token}/alarms", T: tagPublic, S: "Alarms of the bound datastreams", P: "public", R: arr(ref("Alarm"))},
	{M: "GET /api/v1/public/{token}/stream", T: tagPublic, S: "Live stream of the shared dashboard (SSE)", P: "public", CT: "text/event-stream", RD: "Server-Sent Events, see Live stream"},
	{M: "GET /api/v1/public/{token}/hmi/objects/{slug}/svg", T: tagPublic, S: "A library graphic shown by the shared picture", D: "Only graphics that the linked process picture references.", P: "public", CT: "image/svg+xml", RD: "SVG document"},

	// ---- live
	{M: "GET /api/v1/stream", T: tagLive, S: "Live events (Server-Sent Events)", D: "Long-lived `text/event-stream`. Events: `hello`, `observation` {datastream_id, key, value, measured_at}, `device` {device_id, connected}, `alarm` {alarm_id, datastream_id, state, severity, value}, `command` {device_id, command_id, method, status, detail}; `ping` every 25 s with the count of dropped events. Filters: `datastreams` (comma-separated ids), `devices` (`all` or ids), `alarms=1`.", P: "data.read",
		Q: []apiParam{q("datastreams", "comma-separated datastream ids"), q("devices", "all, or comma-separated device ids"), q("alarms", "1 to include alarm events")}, CT: "text/event-stream", RD: "event stream"},

	// ---- connectors
	{M: "GET /api/v1/connectors", T: tagConn, S: "List connectors", P: "config.write", R: arr(ref("Connector"))},
	{M: "POST /api/v1/connectors", T: tagConn, S: "Create a connector", D: "`kind` chirpstack: returns the ingest URL and a bearer secret (shown once) for the ChirpStack HTTP integration. `kind` opcua / modbus_tcp: `settings` describe the endpoint and the nodes/registers; values arrive under a virtual device (`settings.device_external_id`).", P: "config.write", C: 201,
		B: obj(S{"kind": enum("", "chirpstack", "opcua", "modbus_tcp"), "name": str(""), "auto_create_devices": boolean("chirpstack: register unknown DevEUIs automatically"), "settings": ref("ConnectorSettings"), "reason": reason}, "kind", "name", "reason"), R: ref("ConnectorCreated")},
	{M: "PUT /api/v1/connectors/{id}", T: tagConn, S: "Update connector settings (OPC UA, Modbus)", P: "config.write", B: obj(S{"settings": ref("ConnectorSettings"), "reason": reason}, "settings", "reason"), R: ref("Connector")},
	{M: "DELETE /api/v1/connectors/{id}", T: tagConn, S: "Revoke a connector", D: "Stops polling and rejects its secret.", P: "config.write", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "POST /api/v1/connectors/test", T: tagConn, S: "Test connector settings without saving", D: "Connects once and reads the configured nodes/registers.", P: "config.write", B: obj(S{"kind": enum("default opcua", "opcua", "modbus_tcp"), "settings": ref("ConnectorSettings"), "id": str("existing connector id (reuse its stored password)")}, "settings"), R: obj(S{"ok": boolean(""), "values": anyObj("key → value"), "bad_nodes": arr(str("")), "security": str("OPC UA: negotiated policy"), "endpoint": str("")})},
	{M: "GET /api/v1/register-profiles", T: tagConn, S: "Register profiles (current versions)", D: "Versioned Modbus register maps shared by connectors of the same device type, with the number of connectors using any version and how many are on an older version.", P: "config.write", R: arr(ref("RegisterProfile"))},
	{M: "POST /api/v1/register-profiles", T: tagConn, S: "Create a register profile", P: "config.write", C: 201, B: obj(S{"name": str("unique among active profiles"), "description": str(""), "spec": ref("RegisterProfileSpec"), "reason": reason}, "name", "spec", "reason"), R: obj(S{"id": uuid(""), "name": str(""), "version": integer(""), "registers": integer("")})},
	{M: "GET /api/v1/register-profiles/{id}", T: tagConn, S: "One profile version with the history of its lineage", P: "config.write", R: ref("RegisterProfileDetail")},
	{M: "PUT /api/v1/register-profiles/{id}", T: tagConn, S: "New version of a profile", D: "The current version is closed (`valid_to`) and a new row with the next version number is created; connectors keep their version until it is applied.", P: "config.write", C: 201, B: obj(S{"description": str(""), "spec": ref("RegisterProfileSpec"), "changelog": str("what changed"), "reason": reason}, "spec", "reason"), R: obj(S{"id": uuid("new version id"), "previous_id": uuid(""), "name": str(""), "version": integer(""), "registers": integer("")})},
	{M: "DELETE /api/v1/register-profiles/{id}", T: tagConn, S: "Retire a profile", D: "Refused (409) while connectors use any version of it.", P: "config.write", Q: []apiParam{qr("reason", "")}, R: obj(S{"id": uuid(""), "retired": boolean("")})},
	{M: "POST /api/v1/register-profiles/{id}/apply", T: tagConn, S: "Apply this version to the connectors on older versions", D: "Each connector's registers and byte order are replaced from the profile (audited as connector.update) and its poller restarts.", P: "config.write", B: reasonBody, R: obj(S{"id": uuid(""), "version": integer(""), "applied": integer(""), "connectors": arr(obj(S{"id": uuid(""), "name": str("")}))})},
	{M: "GET /api/v1/connectors/opcua/certificate", T: tagConn, S: "This server's OPC UA client certificate (PEM)", D: "Trust it on the OPC UA server for signed/encrypted sessions.", P: "config.write", CT: "application/x-pem-file", RD: "PEM certificate"},
	{M: "POST /api/v1/ingest/chirpstack/{id}", T: tagProv, S: "ChirpStack HTTP integration endpoint", D: "Called by ChirpStack with `?event=up` (and others) and the connector's bearer secret. Uplinks are decoded per device profile and stored like MQTT telemetry.", P: "connector", Q: []apiParam{qr("event", "ChirpStack event type, e.g. up")}, B: anyObj("ChirpStack event JSON"), R: obj(S{"accepted": boolean(""), "event": str(""), "device": str(""), "decoded_keys": arr(str("")), "note": str("")})},

	// ---- reports
	{M: "GET /api/v1/report-templates", T: tagReports, S: "Report kinds, overrides and engine info", P: "data.export", R: obj(S{"builtin_kinds": arr(str("")), "overrides": arr(anyObj("")), "engine": str(""), "engine_version": str(""), "max_rows": integer("")})},
	{M: "GET /api/v1/report-templates/{kind}", T: tagReports, S: "Template of a report kind", D: "The effective template (organization override or built-in) as Typst source and, when designed with blocks, its block spec.", P: "data.export", Q: []apiParam{q("locale", "")}, R: obj(S{"template": anyObj(""), "layout": str(""), "blocks": arr(anyObj(""))})},
	{M: "PUT /api/v1/report-templates/{kind}", T: tagReports, S: "Override a report template", D: "Either a block `spec` (compiled to Typst on the server) or a full Typst `body`.", P: "content.write", B: obj(S{"locale": str(""), "name": str(""), "body": str("Typst source"), "spec": anyObj("block designer spec"), "reason": reason}, "reason"), R: obj(S{"id": uuid(""), "version": integer(""), "checksum": str(""), "kind": str(""), "locale": str(""), "name": str("")})},
	{M: "DELETE /api/v1/report-templates/{kind}", T: tagReports, S: "Revert to the built-in template", P: "content.write", Q: []apiParam{q("locale", ""), qr("reason", "")}, R: obj(S{"kind": str(""), "locale": str(""), "reverted": boolean("")})},
	{M: "GET /api/v1/report-templates/{kind}/contract", T: tagReports, S: "Data contract of a report kind", D: "The variables and tables a template of this kind receives.", P: "data.export", R: anyObj("contract")},
	{M: "GET /api/v1/report-templates/{kind}/default-spec", T: tagReports, S: "Default block spec of a report kind", P: "data.export", R: obj(S{"spec": anyObj(""), "blocks": arr(anyObj("block catalog"))})},
	{M: "GET /api/v1/report-templates/{kind}/preview", T: tagReports, S: "Render the effective template with sample data (PDF)", P: "data.export", Q: []apiParam{q("locale", "")}, CT: "application/pdf", RD: "PDF"},
	{M: "POST /api/v1/report-templates/preview", T: tagReports, S: "Render Typst source with sample data (PDF)", P: "content.write", B: obj(S{"kind": str(""), "locale": str(""), "body": str("Typst source")}, "kind", "body"), CT: "application/pdf", RD: "PDF"},
	{M: "POST /api/v1/report-templates/compile", T: tagReports, S: "Compile a block spec to Typst source", P: "content.write", B: obj(S{"spec": anyObj("")}, "spec"), R: obj(S{"body": str("Typst source"), "spec": anyObj("")})},
	{M: "GET /api/v1/reports", T: tagReports, S: "Stored reports, newest first", P: "data.export", Q: []apiParam{qi("limit", "")}, R: arr(ref("Report"))},
	{M: "GET /api/v1/reports/{id}", T: tagReports, S: "Report metadata", P: "data.export", R: ref("Report")},
	{M: "GET /api/v1/reports/{id}/file", T: tagReports, S: "Download the report PDF", P: "data.export", CT: "application/pdf", RD: "PDF"},

	// ---- exports
	{M: "GET /api/v1/export/observations", T: tagExports, S: "Export observations", D: "`format` csv (default), jsonl or pdf (a signed report stored under Reports). Flagged rows (quality other than ok) are marked.", P: "data.export", Q: []apiParam{qr("datastream_id", ""), q("from", "RFC 3339"), q("to", "RFC 3339"), q("format", "csv | jsonl | pdf"), qi("limit", "")}, CT: "text/csv", RD: "CSV, JSON Lines or PDF"},
	{M: "GET /api/v1/export/alarms", T: tagExports, S: "Export the alarm log", P: "data.export", Q: []apiParam{q("from", ""), q("to", ""), q("format", "csv | jsonl"), qi("limit", "")}, CT: "text/csv", RD: "CSV or JSON Lines"},
	{M: "GET /api/v1/export/messages", T: tagExports, S: "Export raw messages", P: "data.export", Q: []apiParam{q("device_id", ""), q("from", ""), q("to", ""), q("format", "csv | jsonl"), qi("limit", "")}, CT: "text/csv", RD: "CSV or JSON Lines"},
	{M: "GET /api/v1/export/audit", T: tagExports, S: "Export the audit trail", D: "Hash-chained rows: seq, hash, prev_hash, actor, action, entity, old/new values, reason, request id.", P: "audit.read", Q: []apiParam{q("from", ""), q("to", ""), q("format", "csv | jsonl"), qi("limit", "")}, CT: "text/csv", RD: "CSV or JSON Lines"},
	{M: "GET /api/v1/export/access-log", T: tagExports, S: "Export the access log", D: "Sign-ins, failures, token use, session events.", P: "audit.read", Q: []apiParam{q("from", ""), q("to", ""), q("format", "csv | jsonl"), qi("limit", "")}, CT: "text/csv", RD: "CSV or JSON Lines"},

	// ---- branding and languages
	{M: "GET /api/v1/branding", T: tagBrand, S: "Branding, theme, map settings and version", D: "Public: the sign-in page uses it.", P: "public", R: ref("Branding")},
	{M: "GET /api/v1/tiles", T: tagBrand, S: "Self-hosted map tiles (PMTiles archive)", D: "Status of the server's local map: a raster PMTiles v3 archive named by `[map] pmtiles` in config.toml. When it is configured and the organization has no tile URL of its own, the branding map points at `/tiles/{z}/{x}/{y}.<ext>` on this server (public, cacheable; 404 outside the archive), so map widgets work without the Internet. Create an archive with `pmtiles convert map.mbtiles map.pmtiles` (go-pmtiles) or try `go run ./tools/pmtilesgen`.", P: "public", R: ref("TilesInfo")},
	{M: "PUT /api/v1/branding", T: tagBrand, S: "Set branding, theme and map tiles", D: "Logo as base64 (PNG/SVG/JPEG, max 256 KiB).", P: "content.write", B: obj(S{"logo": nullable(obj(S{"mime": str(""), "data": str("base64")})), "clear_logo": boolean(""), "theme": ref("Theme"), "map": ref("MapSettings"), "reason": reason}, "reason"), R: okResp},
	{M: "GET /api/v1/branding/logo", T: tagBrand, S: "Organization logo image", P: "public", CT: "image/*", RD: "image"},
	{M: "GET /api/v1/i18n/languages", T: tagBrand, S: "Supported and enabled languages", P: "public", R: obj(S{"supported": arr(obj(S{"code": str(""), "name": str(""), "builtin": integer("built-in translated strings")})), "enabled": arr(str("")), "default": str("")})},
	{M: "PUT /api/v1/i18n/languages", T: tagBrand, S: "Enable languages and set the default", P: "content.write", B: obj(S{"enabled": arr(str("locale codes")), "default": str(""), "reason": reason}, "enabled", "default", "reason"), R: obj(S{"enabled": arr(str("")), "default": str("")})},
	{M: "GET /api/v1/i18n/{locale}", T: tagBrand, S: "Translation catalog of a locale", D: "Built-in strings merged with the organization's overrides; key = English text.", P: "public", R: S{"type": "object", "additionalProperties": str(""), "description": "English → translation"}},
	{M: "GET /api/v1/i18n/{locale}/edit", T: tagBrand, S: "Catalog for editing (built-in and override per key)", P: "content.write", R: obj(S{"locale": str(""), "entries": arr(obj(S{"key": str(""), "builtin": str(""), "override": str("")}))})},
	{M: "PUT /api/v1/i18n/{locale}", T: tagBrand, S: "Set translation overrides", D: "An empty value removes the override.", P: "content.write", B: obj(S{"entries": S{"type": "object", "additionalProperties": str("")}, "reason": reason}, "entries", "reason"), R: obj(S{"locale": str(""), "set": integer(""), "removed": integer("")})},

	// ---- server administration
	{M: "GET /api/v1/server", T: tagServer, S: "Server status and configuration summary", D: "Version, listeners, database, SMTP, storage paths, connected MQTT clients, update availability.", P: "system.admin", R: anyObj("status")},
	{M: "POST /api/v1/server/restart", T: tagServer, S: "Restart the service (through the agent)", P: "system.admin", B: reasonBody, R: obj(S{"unit": str("")})},
	{M: "POST /api/v1/server/update", T: tagServer, S: "Download, verify and install a release", D: "The agent fetches the binary and SHA256SUMS from the update URL, verifies, installs and restarts.", P: "system.admin", B: obj(S{"version": str("release version, e.g. 0.24.0"), "reason": reason}, "version", "reason"), R: obj(S{"version": str(""), "sha256": str("")})},
	{M: "PUT /api/v1/server/caddy", T: tagServer, S: "Configure the domain and TLS (Caddy)", P: "system.admin", B: obj(S{"domain": str(""), "extra_domains": arr(str("")), "email": str("ACME account e-mail"), "dns_provider": str("optional DNS challenge provider"), "token": str("DNS provider token (blank keeps the stored one)"), "reason": reason}, "domain", "reason"), R: obj(S{"domain": str(""), "extra_domains": arr(str("")), "email": str(""), "dns_provider": str(""), "token_changed": boolean("")})},
	{M: "GET /api/v1/server/wireguard", T: tagServer, S: "WireGuard status and peers", P: "system.admin", R: anyObj("status")},
	{M: "PUT /api/v1/server/wireguard", T: tagServer, S: "Enable or configure WireGuard", P: "system.admin", B: obj(S{"enabled": boolean(""), "listen_port": integer(""), "address": str("server address in the tunnel, CIDR"), "endpoint": str("public endpoint host:port"), "reason": reason}, "reason"), R: obj(S{"enabled": boolean(""), "listen_port": integer(""), "address": str(""), "endpoint": str("")})},
	{M: "POST /api/v1/server/wireguard/peers", T: tagServer, S: "Add a WireGuard peer", D: "Returns the peer's client configuration (private key shown once).", P: "system.admin", C: 201, B: obj(S{"name": str(""), "reason": reason}, "name", "reason"), R: obj(S{"peer": anyObj(""), "config": str("client configuration file")})},
	{M: "POST /api/v1/server/wireguard/peers/{id}/remove", T: tagServer, S: "Remove a WireGuard peer", P: "system.admin", B: reasonBody, R: okResp},
	{M: "GET /api/v1/server/backups", T: tagServer, S: "Backups and schedule", P: "system.admin", R: obj(S{"dir": str(""), "running": boolean(""), "files": arr(obj(S{"name": str(""), "size_bytes": integer(""), "created_at": ts(""), "encrypted": boolean("")})), "schedule": anyObj("")})},
	{M: "POST /api/v1/server/backups", T: tagServer, S: "Start a backup now", P: "system.admin", B: anyObj("empty object"), R: obj(S{"encrypted": boolean("")})},
	{M: "PUT /api/v1/server/backups/schedule", T: tagServer, S: "Set the backup schedule", P: "system.admin", B: obj(S{"enabled": boolean(""), "hour": integer(""), "minute": integer(""), "keep": integer("files to keep"), "passphrase": str("encrypt backups (blank keeps the stored one)"), "reason": reason}, "reason"), R: obj(S{"enabled": boolean(""), "hour": integer(""), "minute": integer(""), "keep": integer(""), "passphrase_changed": boolean("")})},
	{M: "GET /api/v1/server/backups/{name}", T: tagServer, S: "Download a backup archive", P: "system.admin", CT: "application/octet-stream", RD: "tar.gz archive"},
	{M: "POST /api/v1/server/backups/{name}/delete", T: tagServer, S: "Delete a backup archive", P: "system.admin", B: reasonBody, R: okResp},

	// ---- provisioning
	{M: "POST /api/v1/provision", T: tagProv, S: "Device self-provisioning with a claim code or a provisioning key", D: "Either the claim code from device registration (response: token and MQTT connection details), or ThingsBoard-compatible `{deviceName, provisionDeviceKey, provisionDeviceSecret}` of a provisioning profile (response: `{status, credentialsType: ACCESS_TOKEN, credentialsValue}`; the same works over MQTT: user name `provision`, publish `/provision/request`, answer on `/provision/response`). Rate limited per IP and key.", P: "public",
		B: obj(S{"claim": str("claim code"), "hw": str("hardware id"), "fw": str("firmware version"), "deviceName": str("provisioning profile variant"), "provisionDeviceKey": str(""), "provisionDeviceSecret": str("")}), R: obj(S{"device_id": uuid(""), "external_id": str(""), "token": str("device token, shown once"), "mqtt": obj(S{"host": str(""), "port": integer(""), "tls": boolean("")}), "protocol": str("")})},
	{M: "GET /api/v1/provision-profiles", T: tagProv, S: "Provisioning profiles", P: "device.manage", R: arr(obj(S{"id": uuid(""), "name": str(""), "key": str("pk_…, identifies the profile"), "strategy": enum("", "create", "preprovisioned"), "approval": boolean("created devices wait for approval"), "site_id": uuid(""), "room_id": uuid(""), "auto_datastreams": boolean(""), "name_pattern": str("regular expression"), "transport": enum("", "mqtt", "http"), "max_devices": integer("0 = unlimited"), "devices": integer("")}))},
	{M: "POST /api/v1/provision-profiles", T: tagProv, S: "Create a provisioning profile", D: "The response carries the `secret` once; devices use key and secret to register themselves.", P: "device.manage", C: 201, B: obj(S{"name": str(""), "strategy": enum("", "create", "preprovisioned"), "approval": boolean(""), "site_id": uuid(""), "room_id": uuid(""), "auto_datastreams": boolean(""), "name_pattern": str(""), "transport": enum("", "mqtt", "http"), "max_devices": integer(""), "reason": reason}, "reason"), R: obj(S{"id": uuid(""), "name": str(""), "key": str("pk_…, identifies the profile"), "strategy": enum("", "create", "preprovisioned"), "approval": boolean("created devices wait for approval"), "site_id": uuid(""), "room_id": uuid(""), "auto_datastreams": boolean(""), "name_pattern": str("regular expression"), "transport": enum("", "mqtt", "http"), "max_devices": integer("0 = unlimited"), "devices": integer("")})},
	{M: "PUT /api/v1/provision-profiles/{id}", T: tagProv, S: "Change a provisioning profile", P: "device.manage", B: obj(S{"name": str(""), "strategy": enum("", "create", "preprovisioned"), "approval": boolean(""), "site_id": uuid(""), "room_id": uuid(""), "auto_datastreams": boolean(""), "name_pattern": str(""), "transport": enum("", "mqtt", "http"), "max_devices": integer(""), "reason": reason}, "reason"), R: obj(S{"id": uuid(""), "name": str(""), "key": str("pk_…, identifies the profile"), "strategy": enum("", "create", "preprovisioned"), "approval": boolean("created devices wait for approval"), "site_id": uuid(""), "room_id": uuid(""), "auto_datastreams": boolean(""), "name_pattern": str("regular expression"), "transport": enum("", "mqtt", "http"), "max_devices": integer("0 = unlimited"), "devices": integer("")})},
	{M: "POST /api/v1/provision-profiles/{id}/rotate", T: tagProv, S: "New secret for a profile", D: "Devices already provisioned keep their tokens.", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "secret": str("shown once")})},
	{M: "DELETE /api/v1/provision-profiles/{id}", T: tagProv, S: "Revoke a provisioning profile", P: "device.manage", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/mqtt-sources", T: tagSmart, S: "MQTT accounts and bridges", P: "device.manage", R: arr(obj(S{"id": uuid(""), "kind": enum("", "account", "bridge"), "name": str(""), "settings": anyObj("host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords"), "pending": integer("devices waiting in the inbox"), "status": obj(S{"connected": boolean(""), "error": str(""), "entities": integer("")})}))},
	{M: "POST /api/v1/mqtt-sources", T: tagSmart, S: "Create an MQTT account or a bridge", D: "An account gets a generated user name and password (returned once) and its own topic namespace: its devices use any topics (zigbee2mqtt/…, cmnd/…, homeassistant/…) without seeing other accounts. A bridge connects ctrl32 to a foreign broker.", P: "device.manage", C: 201, B: obj(S{"kind": enum("", "account", "bridge"), "name": str(""), "host": str("bridge"), "port": integer("bridge"), "tls": boolean("bridge"), "insecure_skip_verify": boolean("bridge, self-signed brokers"), "username": str("bridge"), "password": str("bridge; stored encrypted"), "client_id": str("bridge"), "discovery_prefix": str("default homeassistant"), "adopt": enum("", "auto", "inbox"), "site_id": uuid(""), "publish_datastreams": arr(uuid("bridge: datastreams published to the foreign broker with Home Assistant discovery")), "outage_incident_min": num("minutes disconnected before an incident opens (default 10, 0 = never; accounts only once they have entities)"), "reason": reason}, "reason"), R: obj(S{"id": uuid(""), "kind": enum("", "account", "bridge"), "name": str(""), "settings": anyObj("host, port, tls, username, client_id, discovery_prefix, adopt (auto|inbox), site_id, namespace; never passwords"), "pending": integer("devices waiting in the inbox"), "status": obj(S{"connected": boolean(""), "error": str(""), "entities": integer("")})})},
	{M: "PUT /api/v1/mqtt-sources/{id}", T: tagSmart, S: "Change an MQTT source", P: "device.manage", B: obj(S{"kind": enum("", "account", "bridge"), "name": str(""), "host": str("bridge"), "port": integer("bridge"), "tls": boolean("bridge"), "insecure_skip_verify": boolean("bridge, self-signed brokers"), "username": str("bridge"), "password": str("bridge; stored encrypted"), "client_id": str("bridge"), "discovery_prefix": str("default homeassistant"), "adopt": enum("", "auto", "inbox"), "site_id": uuid(""), "publish_datastreams": arr(uuid("bridge: datastreams published to the foreign broker with Home Assistant discovery")), "outage_incident_min": num("minutes disconnected before an incident opens (default 10, 0 = never; accounts only once they have entities)"), "reason": reason}, "reason"), R: anyObj("source")},
	{M: "POST /api/v1/mqtt-sources/{id}/password", T: tagSmart, S: "New password for an MQTT account", D: "Connected clients of the account are disconnected.", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "username": str(""), "password": str("shown once")})},
	{M: "DELETE /api/v1/mqtt-sources/{id}", T: tagSmart, S: "Delete an MQTT source", D: "Its entities are marked removed; their history stays.", P: "device.manage", Q: []apiParam{qr("reason", "")}, R: okResp},
	{M: "GET /api/v1/discovery", T: tagSmart, S: "Discovered devices (inbox)", P: "device.manage", Q: []apiParam{q("status", "pending | adopted | ignored")}, R: arr(anyObj("discovered device"))},
	{M: "POST /api/v1/discovery/{id}/adopt", T: tagSmart, S: "Adopt a discovered device", D: "Creates the device, its room (from the suggested area) and its entities.", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "device_id": uuid(""), "status": str("")})},
	{M: "POST /api/v1/discovery/{id}/ignore", T: tagSmart, S: "Ignore a discovered device", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "status": str("")})},
	{M: "GET /api/v1/entities", T: tagSmart, S: "Entities with their state", P: "data.read", Q: []apiParam{q("room", ""), q("device", ""), q("component", ""), q("source", "MQTT source id")}, R: arr(obj(S{"id": uuid(""), "component": str("sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, …"), "name": str(""), "state": str(""), "attributes": anyObj(""), "available": boolean(""), "status": enum("", "active", "unsupported", "removed"), "error": str(""), "device_id": uuid(""), "device": str(""), "room_id": uuid(""), "room": str(""), "datastream_id": uuid("history of the state"), "unit": str(""), "services": arr(str("")), "ui": anyObj("control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds}")}))},
	{M: "GET /api/v1/entities/{id}", T: tagSmart, S: "Entity with its discovery config", P: "data.read", R: obj(S{"id": uuid(""), "component": str("sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, …"), "name": str(""), "state": str(""), "attributes": anyObj(""), "available": boolean(""), "status": enum("", "active", "unsupported", "removed"), "error": str(""), "device_id": uuid(""), "device": str(""), "room_id": uuid(""), "room": str(""), "datastream_id": uuid("history of the state"), "unit": str(""), "services": arr(str("")), "ui": anyObj("control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds}")})},
	{M: "PATCH /api/v1/entities/{id}", T: tagSmart, S: "Rename, move to a room, hide", P: "device.manage", B: obj(S{"name": str(""), "room_id": uuid(""), "hidden": boolean(""), "sort": integer(""), "reason": reason}, "reason"), R: okResp},
	{M: "POST /api/v1/entities/{id}/command", T: tagSmart, S: "Control an entity", D: "Becomes a device command (permissions, four eyes, audit): `sent` until the device reports the requested state, then `acked`; optimistic entities and buttons are `acked` at once. The reason defaults to \"Home: <service> <entity>\".", P: "device.command",
		B: obj(S{"service": str("turn_on, turn_off, toggle, open, close, stop, set_position, lock, unlock, set_temperature, set_hvac_mode, set_value, select_option, press, set_percentage"), "value": anyObj("number, text or option"), "brightness": num("0–100 %"), "rgb": arr(num("")), "color_temp": num("mireds"), "position": num("0–100 %"), "temperature": num(""), "mode": str(""), "percentage": num(""), "reason": str("optional")}, "service"), R: anyObj("command")},
	{M: "GET /api/v1/home", T: tagSmart, S: "Rooms with their entities (Home page)", P: "data.read", R: obj(S{"rooms": arr(obj(S{"id": uuid(""), "name": str(""), "entities": arr(obj(S{"id": uuid(""), "component": str("sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, …"), "name": str(""), "state": str(""), "attributes": anyObj(""), "available": boolean(""), "status": enum("", "active", "unsupported", "removed"), "error": str(""), "device_id": uuid(""), "device": str(""), "room_id": uuid(""), "room": str(""), "datastream_id": uuid("history of the state"), "unit": str(""), "services": arr(str("")), "ui": anyObj("control hints: min, max, step, options, modes, …; lights: color {rgb, color_temp, min_mireds, max_mireds}")}))})), "pending_devices": integer(""), "can_control": boolean(""), "can_manage": boolean("")})},
	{M: "GET /api/v1/scenes", T: tagSmart, S: "Scenes", P: "data.read", R: arr(ref("Scene"))},
	{M: "POST /api/v1/scenes", T: tagSmart, S: "Create a scene", D: "Actions explicitly, or capture: entity ids whose current states (on/off, brightness, colour, position, temperature, mode, value) become the actions.", P: "device.manage", C: 201, B: obj(S{"name": str("≤ 80"), "icon": str("icon name"), "room_id": uuid(""), "actions": arr(ref("SceneAction")), "capture": arr(uuid("entity")), "sort": integer(""), "reason": reason}, "name", "reason"), R: ref("Scene")},
	{M: "PUT /api/v1/scenes/{id}", T: tagSmart, S: "Change a scene", P: "device.manage", B: obj(S{"name": str(""), "icon": str(""), "room_id": uuid(""), "actions": arr(ref("SceneAction")), "capture": arr(uuid("entity")), "sort": integer(""), "reason": reason}, "reason"), R: ref("Scene")},
	{M: "DELETE /api/v1/scenes/{id}", T: tagSmart, S: "Delete a scene", P: "device.manage", Q: []apiParam{q("reason", "required")}, R: okResp},
	{M: "POST /api/v1/scenes/{id}/activate", T: tagSmart, S: "Activate a scene", D: "Each action is an entity command of the signed-in user (four eyes when required, audit with reason \"Scene: <name>\"). 409 when every command failed.", P: "device.command", B: obj(S{"reason": str("optional")}), R: obj(S{"scene": uuid(""), "name": str(""), "failed": integer(""), "results": arr(obj(S{"entity_id": uuid(""), "service": str(""), "command_id": uuid(""), "status": str("sent, acked, pending, failed"), "error": str(""), "http_status": integer("")}))})},
	{M: "GET /api/v1/energy", T: tagSmart, S: "Energy overview", D: "Consumption and production of energy meters (entities with device_class energy, datastreams in Wh/kWh/MWh) per hour (day), day (week, month) or month (year) in the organization's time zone, with cost.", P: "data.read", Q: []apiParam{q("period", "day (default), week, month, year"), q("date", "YYYY-MM-DD inside the period (default today)")}, R: obj(S{"period": str(""), "from": ts(""), "to": ts(""), "bucket": str("hour, day, month"), "tz": str(""), "unit": str("kWh"), "currency": str(""), "price_per_kwh": num(""), "feed_in_per_kwh": num(""), "buckets": arr(obj(S{"start": ts(""), "consumption": num(""), "production": num(""), "cost": num("")})), "total": obj(S{"consumption": num(""), "production": num(""), "cost": num("")}), "sources": arr(obj(S{"datastream_id": uuid(""), "entity_id": str(""), "name": str(""), "device": str(""), "unit": str(""), "role": str("consumption, production, off"), "kwh": num(""), "share": num("% of consumption")})), "power_w": num("sum of the current states of power entities")})},
	{M: "GET /api/v1/energy/settings", T: tagSmart, S: "Energy settings and meters", P: "data.read", R: obj(S{"price_per_kwh": num(""), "feed_in_per_kwh": num(""), "currency": str(""), "exclude_others": boolean(""), "meters": arr(obj(S{"datastream_id": uuid(""), "name": str(""), "device": str(""), "unit": str(""), "role": str("")}))})},
	{M: "PUT /api/v1/energy/settings", T: tagSmart, S: "Set energy price and meter roles", D: "Meters without an explicit role count as consumption; a device with several energy sensors counts the one named total.", P: "content.write", B: obj(S{"price_per_kwh": num("0–100"), "feed_in_per_kwh": num("0–100"), "currency": str("≤ 4"), "exclude_others": boolean("count only the listed meters"), "sources": arr(obj(S{"datastream_id": uuid(""), "role": enum("", "consumption", "production", "off")})), "reason": reason}, "reason"), R: okResp},
	{M: "GET /api/v1/onboarding", T: tagSmart, S: "Connection details for devices", P: "device.manage", R: obj(S{"base_url": str(""), "mqtt": obj(S{"host": str(""), "port": integer(""), "tls_port": integer(""), "ws_port": integer("")}), "provision_url": str("")})},
	{M: "GET /api/v1/device-approvals", T: tagProv, S: "Devices waiting for approval", P: "device.manage", R: arr(anyObj("device"))},
	{M: "POST /api/v1/devices/{id}/approve", T: tagProv, S: "Approve a self-provisioned device", D: "Its token works from now on.", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "approval": str("")})},
	{M: "POST /api/v1/devices/{id}/reject", T: tagProv, S: "Reject a self-provisioned device", D: "The device and its token are revoked.", P: "device.manage", B: reasonBody, R: obj(S{"id": uuid(""), "approval": str("")})},
}

var apiSchemas = S{
	"Error":            obj(S{"error": str("human-readable message")}, "error"),
	"LoginResponse":    obj(S{"user": ref("Me"), "mfa_required": boolean("MFA step needed"), "pre_auth_token": str("for POST /api/v1/auth/mfa"), "must_change_password": boolean("")}),
	"Me":               obj(S{"user_id": uuid(""), "org_id": uuid(""), "username": str(""), "full_name": str(""), "locale": str(""), "roles": arr(str("")), "permissions": arr(str("")), "csrf_token": str("send as X-CSRF-Token with cookie sessions"), "must_change_password": boolean(""), "mfa_enabled": boolean(""), "mfa_required": boolean(""), "auth": enum("", "session", "token")}),
	"Permission":       obj(S{"name": str(""), "description": str("")}),
	"Role":             obj(S{"id": uuid(""), "org_id": str("empty for built-in roles"), "name": str(""), "permissions": arr(str("")), "is_admin": boolean(""), "is_qa": boolean(""), "users": integer("members")}),
	"User":             obj(S{"id": uuid(""), "username": str(""), "full_name": str(""), "email": str(""), "roles": arr(str("")), "disabled": boolean(""), "locked_until": nullable(ts("")), "mfa_enabled": boolean(""), "must_change_password": boolean(""), "last_login_at": nullable(ts("")), "created_at": ts("")}),
	"PushSubscription": obj(S{"id": uuid(""), "user_id": uuid(""), "endpoint": str("push service URL"), "user_agent": str(""), "created_at": ts(""), "last_used_at": nullable(ts("")), "failed_at": nullable(ts("")), "revoked_at": nullable(ts(""))}),
	"AccountRequest":   obj(S{"id": uuid(""), "full_name": str(""), "email": str(""), "message": str(""), "created_at": ts("")}),
	"APIToken":         obj(S{"id": uuid(""), "name": str(""), "scopes": arr(str("")), "expires_at": nullable(ts("")), "last_used_at": nullable(ts("")), "created_at": ts(""), "revoked_at": nullable(ts(""))}),
	"Organization":     obj(S{"id": uuid(""), "slug": str(""), "name": str(""), "tz": str(""), "locale": str(""), "profile": str(""), "created_at": ts(""), "disabled_at": nullable(ts("")), "disabled_reason": nullable(str("")), "users": integer(""), "devices": integer(""), "admin_username": str(""), "current": boolean("the caller's organization")}),
	"Site":             obj(S{"id": uuid(""), "name": str(""), "tz": str(""), "address": str(""), "customer_id": nullable(uuid("")), "created_at": ts(""), "assets": integer("")}),
	"Asset":            obj(S{"id": uuid(""), "site_id": uuid(""), "site": str(""), "parent_id": nullable(uuid("")), "type": str(""), "name": str(""), "attributes": anyObj(""), "created_at": ts(""), "datastreams": integer("")}),
	"Datastream":       obj(S{"id": uuid(""), "key": str(""), "quantity": str(""), "unit": str(""), "kind": enum("", "gauge", "counter", "state", "event"), "asset": str("asset name"), "asset_id": uuid(""), "expected_interval": str("")}),
	"Observation":      obj(S{"measured_at": ts("source time"), "received_at": ts("server time"), "value": nullable(num("")), "value_json": anyVal("structured/string value"), "quality": enum("", "ok", "backfilled", "sensor_fault", "out_of_range", "clock_suspect", "stale", "simulated"), "time_source": enum("", "device", "device_relative", "server")}, "measured_at", "received_at", "quality", "time_source"),
	"FlowSpec":         obj(S{"nodes": arr(obj(S{"id": str(""), "type": str("node type from the catalogue"), "name": str(""), "x": num(""), "y": num(""), "settings": anyObj(""), "disabled": boolean("")}, "id", "type")), "wires": arr(obj(S{"from": str(""), "port": integer("output index"), "to": str("")}, "from", "port", "to"))}, "nodes", "wires"),
	"Flow":             obj(S{"id": uuid("this version"), "slug": str(""), "name": str(""), "version": integer(""), "state": enum("", "draft", "deployed", "disabled"), "spec": ref("FlowSpec"), "checksum": str(""), "created_by": str(""), "created_at": ts(""), "reason": str(""), "deployed_version": nullable(integer("")), "nodes": integer(""), "has_commands": boolean("")}, "slug", "name", "version", "state"),
	"FlowProblem":      obj(S{"node": str(""), "message": str("")}, "message"),
	"ReplicationNode":  obj(S{"node_id": uuid(""), "role": enum("", "primary", "replica", "fenced"), "epoch": integer("+1 on every promotion"), "promoted_from": nullable(uuid(""))}, "node_id", "role"),
	"Replica": obj(S{"id": uuid(""), "name": str(""), "mode": enum("", "copy", "continuous", "standby"), "state": enum("", "pairing", "copying", "in_sync", "lagging", "disconnected", "detaching", "detached", "failed", "lost", "dropped"),
		"lag_s": nullable(num("")), "lag_bytes": nullable(integer("")), "replay_lsn": str(""), "last_contact": nullable(ts("")), "copy": anyObj("total_bytes, done_bytes, percent, eta_s"),
		"lag_limit_s": integer(""), "max_retained_mb": integer(""), "standby_url": str(""), "host_info": anyObj(""), "detach": anyObj("detachment record with verification"), "created_at": ts(""), "created_by": str("")}, "id", "name", "mode", "state"),
	"ObservationBuckets":    obj(S{"bucket_s": integer(""), "from": ts(""), "to": ts(""), "points": arr(obj(S{"t": ts("bucket start"), "avg": nullable(num("")), "min": nullable(num("")), "max": nullable(num("")), "n": integer("rows in the bucket"), "flagged": boolean("any row with quality other than ok")}, "t", "n", "flagged"))}, "bucket_s", "from", "to", "points"),
	"LatestValue":           obj(S{"measured_at": ts(""), "value": nullable(num("")), "value_json": anyVal(""), "quality": str(""), "unit": str(""), "key": str(""), "asset": str("")}),
	"Device":                obj(S{"id": uuid(""), "external_id": str(""), "name": str(""), "transport": enum("", "mqtt", "http", "lorawan", "opcua", "modbus"), "status": anyObj("last_seen, fw, battery, rssi, last_values, …"), "last_seen": nullable(ts("")), "revoked_at": nullable(ts("")), "location": nullable(obj(S{"lat": num(""), "lon": num(""), "source": str("manual | reported"), "updated_at": ts("")})), "connected": boolean("MQTT session open")}),
	"Message":               obj(S{"id": uuid(""), "received_at": ts(""), "connector": str(""), "direction": enum("", "in", "out"), "topic": str(""), "payload_text": str(""), "payload_json": anyVal(""), "meta": anyObj(""), "decode_status": str(""), "decode_error": nullable(str(""))}),
	"ChangeRequest":         obj(S{"id": uuid(""), "kind": enum("", "limits", "assignments", "calibration"), "method": str(""), "path": str(""), "body": anyObj("the original request body"), "summary": str("what the change does, in words"), "reason": str(""), "requested_by": str(""), "requested_at": ts(""), "expires_at": ts(""), "status": enum("", "pending", "approved", "rejected", "expired", "failed"), "decided_by": nullable(str("")), "decided_at": nullable(ts("")), "decision_reason": nullable(str("")), "result": anyVal("code and body of the replayed call"), "can_approve": boolean("the caller may approve it")}),
	"Command":               obj(S{"id": uuid(""), "method": str(""), "args": anyObj(""), "expires_at": nullable(ts("")), "status": enum("", "pending", "queued", "sent", "acked", "failed", "expired", "rejected"), "issued_by": str("username or rule:<name>"), "issued_at": ts(""), "sent_at": nullable(ts("")), "acked_at": nullable(ts("")), "result": anyVal(""), "error": nullable(str("")), "reason": str(""), "approved_by": nullable(str(""))}),
	"CommandResult":         obj(S{"id": uuid(""), "method": str(""), "status": str(""), "expires_at": ts(""), "result": anyVal("connector writes: {readback}"), "error": str("")}),
	"Policy":                obj(S{"four_eyes_commands": boolean(""), "four_eyes_limits": boolean(""), "four_eyes_assignments": boolean(""), "mfa_required_all": boolean(""), "mfa_required_roles": arr(str("")), "password_min_length": integer("")}),
	"ConsoleSession":        obj(S{"id": uuid(""), "opened_by": str(""), "opened_at": ts(""), "last_activity_at": ts(""), "closed_at": nullable(ts("")), "close_reason": nullable(str("")), "lines": integer("")}),
	"Firmware":              obj(S{"id": uuid(""), "name": str(""), "version": str(""), "hw": str(""), "size": integer(""), "sha256": str(""), "signed": boolean(""), "min_version": str(""), "notes": str(""), "uploaded_by": str(""), "uploaded_at": ts(""), "campaigns": integer("")}),
	"Campaign":              obj(S{"id": uuid(""), "name": str(""), "firmware_id": uuid(""), "firmware": str(""), "hw": str(""), "created_by": str(""), "created_at": ts(""), "closed_at": nullable(ts("")), "close_reason": nullable(str("")), "counts": anyObj("status → number of devices")}),
	"AlarmRule":             obj(S{"id": uuid(""), "datastream_id": uuid(""), "asset": str(""), "key": str(""), "type": str(""), "warning": nullable(num("")), "action": nullable(num("")), "delay_seconds": num(""), "hysteresis": num(""), "escalation_policy_id": nullable(uuid("")), "version": integer(""), "valid_from": ts(""), "reason": str("")}),
	"Alarm":                 obj(S{"id": uuid(""), "raised_at": ts(""), "cleared_at": nullable(ts("")), "state": enum("", "active", "acknowledged", "suppressed", "cleared"), "severity": enum("", "warning", "action", "critical"), "retrospective": boolean("raised on backfilled data"), "value": nullable(num("")), "key": str("datastream key, or input name for rule alarms")}),
	"ChannelSettings":       obj(S{"recipients": arr(str("email: addresses; sms_http: phone numbers")), "url": str("webhook or SMS gateway URL; {to} and {text} placeholders for sms_http"), "secret": str("webhook: HMAC-SHA256 key (X-Ctrl32-Signature)"), "method": enum("sms_http", "GET", "POST"), "body": str("sms_http POST body template"), "headers": S{"type": "object", "additionalProperties": str("")}}),
	"Channel":               obj(S{"id": uuid(""), "name": str(""), "kind": enum("", "email", "webhook", "sms_http"), "settings": ref("ChannelSettings"), "has_secret": boolean(""), "enabled": boolean(""), "created_at": ts(""), "revoked_at": nullable(ts(""))}),
	"EscalationStep":        obj(S{"after_s": integer("seconds after the alarm"), "channel_id": uuid(""), "channel_name": str("in responses")}, "after_s", "channel_id"),
	"EscalationPolicy":      obj(S{"id": uuid(""), "name": str(""), "steps": arr(ref("EscalationStep")), "version": integer(""), "rules": integer("alarm rules using it"), "revoked_at": nullable(ts(""))}),
	"Notification":          obj(S{"id": uuid(""), "alarm_id": nullable(uuid("")), "channel": nullable(str("")), "kind": nullable(str("")), "step": integer(""), "event": enum("", "raised", "escalated", "cleared", "test", "rule"), "due_at": ts(""), "sent_at": nullable(ts("")), "status": enum("", "scheduled", "sent", "failed", "cancelled"), "attempts": integer(""), "error": nullable(str("")), "subject": str("")}),
	"RuleInput":             obj(S{"name": str("^[a-z][a-z0-9_]{0,31}$, used in conditions and expressions"), "datastream_id": uuid("")}, "name", "datastream_id"),
	"RuleCondition":         obj(S{"input": str("input name"), "op": enum("", ">", ">=", "<", "<=", "==", "!=", "stale"), "value": anyVal("number, or an expression string over the inputs; stale: seconds without data")}, "input", "op", "value"),
	"RuleAction":            obj(S{"type": enum("", "notify", "webhook", "command", "alarm", "derive"), "channel_id": uuid("notify"), "subject": str("notify: template with {rule} {event} {inputs} {<input>}"), "text": str("notify: template"), "url": str("webhook"), "secret": str("webhook: HMAC key"), "device_id": uuid("command"), "method": str("command: default write"), "key": str("command: writable register/node"), "value": anyVal("command: number, boolean or expression string"), "args": anyObj("command: custom RPC arguments instead of {key, value}"), "severity": enum("alarm", "warning", "action", "critical"), "escalation_policy_id": uuid("alarm"), "datastream_id": uuid("derive: target datastream"), "expr": str("derive: expression, e.g. t_in - t_out")}, "type"),
	"RuleSpec":              obj(S{"inputs": arr(ref("RuleInput")), "match": enum("default all", "all", "any"), "conditions": arr(ref("RuleCondition")), "for_s": integer("hold time before firing"), "cooldown_s": integer("minimum seconds between firings"), "actions": arr(ref("RuleAction")), "clear_actions": arr(ref("RuleAction"))}, "inputs"),
	"Rule":                  obj(S{"id": uuid(""), "name": str(""), "description": str(""), "spec": ref("RuleSpec"), "enabled": boolean(""), "status": enum("", "active", "pending", "rejected"), "version": integer(""), "created_by": str(""), "updated_by": str(""), "approved_by": nullable(str("")), "approved_at": nullable(ts("")), "created_at": ts(""), "updated_at": ts(""), "revoked_at": nullable(ts("")), "last_run": nullable(ts("")), "last_event": nullable(str("")), "state": obj(S{"loaded": boolean(""), "active": boolean(""), "pending": boolean("holding / cooldown"), "since": nullable(ts(""))})}),
	"RuleRun":               obj(S{"id": uuid(""), "rule_id": uuid(""), "rule": str(""), "version": integer(""), "fired_at": ts(""), "event": enum("", "enter", "exit"), "inputs": S{"type": "object", "additionalProperties": num("")}, "actions": arr(obj(S{"type": str(""), "status": enum("", "done", "failed"), "detail": str(""), "command_id": str(""), "alarm_id": str("")})), "ok": boolean("")}),
	"RuleEvaluation":        obj(S{"condition": boolean(""), "inputs": S{"type": "object", "additionalProperties": num("")}, "ages_s": S{"type": "object", "additionalProperties": num("")}, "missing": arr(str("inputs without data")), "active": boolean(""), "pending": boolean(""), "errors": arr(str(""))}),
	"DashboardSpec":         obj(S{"format": str("screen-v1"), "layout": anyObj("{type: grid, columns: 12, row_height}"), "widgets": arr(anyObj("{id, type, title, geometry {x,y,w,h}, bindings [{datastream_id, …}], params}"))}),
	"DashboardSummary":      obj(S{"id": uuid(""), "slug": str(""), "name": str(""), "version": integer(""), "is_default": boolean(""), "created_at": ts(""), "widgets": integer("")}),
	"Dashboard":             obj(S{"id": uuid(""), "slug": str(""), "name": str(""), "version": integer(""), "is_default": boolean(""), "spec": ref("DashboardSpec"), "checksum": str(""), "created_at": ts("")}),
	"Share":                 obj(S{"public": boolean(""), "slug": str(""), "token_prefix": str(""), "url": str("public URL"), "qr": str("data URL of the QR code")}),
	"ModbusRegister":        obj(S{"key": str("^[a-z][a-z0-9_]{0,31}$"), "table": enum("", "holding", "input", "coil", "discrete"), "address": integer("0-based"), "type": enum("default int16 (bool for coils)", "bool", "int16", "uint16", "int32", "uint32", "float32", "int64", "uint64", "float64"), "scale": num(""), "offset": num(""), "writable": boolean("holding registers and coils only"), "min": num("limit for writes"), "max": num("limit for writes")}, "key", "table", "address"),
	"RegisterProfileSpec":   obj(S{"byte_order": enum("", "big", "big_swap", "little", "little_swap"), "registers": arr(ref("ModbusRegister"))}, "registers"),
	"RegisterProfile":       obj(S{"id": uuid("current version id"), "lineage": uuid("same across versions"), "name": str(""), "description": str(""), "version": integer(""), "spec": ref("RegisterProfileSpec"), "registers": integer(""), "writable": integer(""), "changelog": str(""), "created_by": str(""), "reason": str(""), "valid_from": ts(""), "retired_at": nullable(ts("")), "connectors": integer("connectors on any version"), "outdated": integer("connectors on an older version")}),
	"RegisterProfileDetail": obj(S{"id": uuid(""), "lineage": uuid(""), "name": str(""), "description": str(""), "version": integer(""), "spec": ref("RegisterProfileSpec"), "changelog": str(""), "created_by": str(""), "reason": str(""), "valid_from": ts(""), "valid_to": nullable(ts("")), "retired_at": nullable(ts("")), "versions": arr(obj(S{"id": uuid(""), "version": integer(""), "changelog": str(""), "created_by": str(""), "reason": str(""), "valid_from": ts(""), "valid_to": nullable(ts("")), "connectors": integer("")}))}),
	"ConnectorSettings":     obj(S{"endpoint": str("opcua: opc.tcp://host:4840"), "security_policy": str("opcua"), "security_mode": str("opcua"), "auth": str("opcua: anonymous | username"), "username": str(""), "password": str(""), "interval_s": integer("polling interval"), "device_external_id": str("virtual device the values arrive under"), "nodes": arr(anyObj("opcua: {key, node_id, writable, min, max}")), "host": str("modbus"), "port": integer("modbus, default 502"), "unit_id": integer("modbus"), "timeout_ms": integer("modbus"), "byte_order": enum("modbus", "big", "big_swap", "little", "little_swap"), "framing": enum("modbus", "tcp", "rtuovertcp"), "registers": arr(ref("ModbusRegister")), "profile_id": str("modbus: register profile version the registers come from"), "profile_name": str("modbus, informational"), "profile_version": integer("modbus, informational")}),
	"Connector":             obj(S{"id": uuid(""), "kind": str(""), "name": str(""), "settings": ref("ConnectorSettings"), "created_at": ts(""), "last_seen_at": nullable(ts("")), "messages": integer(""), "revoked_at": nullable(ts("")), "url": str("chirpstack: ingest URL"), "status": anyObj("opcua/modbus: connection state, last values, errors, writes")}),
	"ConnectorCreated":      obj(S{"id": uuid(""), "kind": str(""), "name": str(""), "settings": ref("ConnectorSettings"), "url": str("chirpstack: ingest URL"), "secret": str("chirpstack: bearer secret, shown once"), "chirpstack": anyObj("chirpstack: integration hints")}),
	"Report":                obj(S{"id": uuid(""), "kind": str(""), "created_at": ts(""), "created_by": str(""), "file_hash": str(""), "size_bytes": integer(""), "params": anyObj(""), "signed": boolean("")}),
	"Theme":                 obj(S{"primary": str("hex colour"), "font": str(""), "mode": enum("", "auto", "light", "dark"), "radius": enum("", "none", "small", "medium", "large"), "font_scale": enum("", "small", "normal", "large")}),
	"MapSettings":           obj(S{"tile_url": str("XYZ tile URL template; empty = the server's PMTiles archive when configured, otherwise built-in OpenStreetMap"), "attribution": str(""), "max_zoom": integer(""), "center_lat": num(""), "center_lon": num(""), "zoom": integer(""), "builtin": boolean("built-in OpenStreetMap source in use (read-only)"), "local": boolean("the server's PMTiles archive in use (read-only)"), "source": str("custom, pmtiles or builtin (read-only)")}),
	"Incident":              obj(S{"id": uuid(""), "org_id": str("Empty for instance-level incidents"), "kind": str("alarm_undelivered, audit_chain, backup_failed, manual, replica, flow or mqtt_source"), "severity": str("minor, major or critical"), "status": str("open, investigating, resolved or closed"), "title": str(""), "detail": anyObj("Source data: alarm id and step, channel, error, sequence number …"), "impact": str(""), "cause": str(""), "capa": str("Corrective and preventive action"), "opened_at": ts(""), "opened_by": str("Username or system"), "updated_at": ts(""), "resolved_at": nullable(ts("")), "closed_at": nullable(ts("")), "closed_by": str(""), "org_name": str("")}),
	"IncidentEvent":         obj(S{"id": uuid(""), "at": ts(""), "actor": str(""), "event": str("opened, fields, note, status or notified"), "detail": anyObj("")}),
	"TilesInfo":             obj(S{"configured": boolean("[map] pmtiles is set in config.toml"), "ok": boolean("the archive opened and holds raster tiles"), "error": str("why the archive is not usable"), "name": str("from the archive metadata"), "attribution": str(""), "tile_url": str("template served by this server"), "tile_type": str("png, jpg, webp or avif"), "min_zoom": integer(""), "max_zoom": integer(""), "bounds": arr(num("")), "center": arr(num("")), "tiles": integer("addressed tiles"), "size_bytes": integer("")}),
	"Scene":                 obj(S{"id": uuid(""), "name": str(""), "icon": str(""), "room_id": nullable(uuid("")), "room": nullable(str("")), "sort": integer(""), "actions": arr(obj(S{"entity_id": uuid(""), "entity": str(""), "component": str(""), "service": str(""), "params": anyObj("brightness, rgb, color_temp, position, temperature, mode, value, percentage")}))}),
	"SceneAction":           obj(S{"entity_id": uuid(""), "service": str("turn_on, turn_off, set_position, …"), "params": anyObj("brightness, rgb, color_temp, position, temperature, mode, value, percentage")}, "entity_id", "service"),
	"Branding":              obj(S{"product": str(""), "version": str(""), "org_name": str(""), "logo_url": str(""), "logo_custom": boolean(""), "public_read": boolean(""), "theme": ref("Theme"), "map": ref("MapSettings"), "footer": anyObj("fixed footer text, licence, github")}),
}

var pathParamDesc = map[string]string{
	"id": "Object id (UUID)", "slug": "Dashboard slug", "cid": "Command id (UUID)", "ds": "Datastream id (UUID)", "sid": "Console session id (UUID)",
	"kind": "Report kind (e.g. temperature_period)", "locale": "Locale code (e.g. sk)", "token": "Public share token", "name": "Backup file name", "fw": "Firmware id (UUID)",
}

var pathParamRe = regexp.MustCompile(`\{([a-z]+)\}`)

// OpenAPIDocument returns the description (used by tools/apigen to generate the clients).
func OpenAPIDocument(version string) map[string]any { return buildOpenAPI(version) }

// buildOpenAPI assembles the document.
func buildOpenAPI(version string) map[string]any {
	paths := map[string]map[string]any{}
	for _, op := range apiOps {
		method, path, _ := strings.Cut(op.M, " ")
		o := S{"tags": []string{op.T}, "summary": op.S, "operationId": opID(method, path)}
		desc := op.D
		switch op.P {
		case "public":
			o["security"] = []any{}
		case "device":
			o["security"] = []any{S{"deviceToken": []any{}}}
		case "connector":
			o["security"] = []any{S{"connectorSecret": []any{}}}
		case "":
			desc = strings.TrimSpace(desc + "\n\nRequires a signed-in user or an API token (any permission).")
		default:
			o["x-permission"] = op.P
			desc = strings.TrimSpace(desc + "\n\nRequires permission `" + op.P + "`.")
		}
		if desc != "" {
			o["description"] = desc
		}
		var params []any
		for _, m := range pathParamRe.FindAllStringSubmatch(path, -1) {
			d := pathParamDesc[m[1]]
			if d == "" {
				d = m[1]
			}
			params = append(params, S{"name": m[1], "in": "path", "required": true, "schema": S{"type": "string"}, "description": d})
		}
		for _, p := range op.Q {
			params = append(params, S{"name": p.Name, "in": "query", "required": p.Required, "schema": S{"type": p.Type}, "description": p.Desc})
		}
		if len(params) > 0 {
			o["parameters"] = params
		}
		if op.B != nil {
			o["requestBody"] = S{"required": true, "content": S{"application/json": S{"schema": op.B}}}
		} else if len(op.F) > 0 {
			props, req := S{}, []string{}
			for _, f := range op.F {
				if f.Name == "file" {
					props[f.Name] = S{"type": "string", "format": "binary", "description": f.Desc}
				} else {
					props[f.Name] = S{"type": "string", "description": f.Desc}
				}
				if f.Required {
					req = append(req, f.Name)
				}
			}
			o["requestBody"] = S{"required": true, "content": S{"multipart/form-data": S{"schema": S{"type": "object", "properties": props, "required": req}}}}
		}
		code := op.C
		if code == 0 {
			code = 200
		}
		responses := S{}
		if op.CT != "" {
			responses[itoa(code)] = S{"description": op.RD, "content": S{op.CT: S{"schema": S{"type": "string", "format": "binary"}}}}
		} else {
			r := op.R
			if r == nil {
				r = anyObj("")
			}
			responses[itoa(code)] = S{"description": "OK", "content": S{"application/json": S{"schema": r}}}
		}
		responses["400"] = S{"description": "Validation error", "content": S{"application/json": S{"schema": ref("Error")}}}
		if op.P != "public" {
			responses["401"] = S{"description": "Not signed in / invalid token", "content": S{"application/json": S{"schema": ref("Error")}}}
			responses["403"] = S{"description": "Missing permission or organization scope", "content": S{"application/json": S{"schema": ref("Error")}}}
		}
		if strings.Contains(path, "{") {
			responses["404"] = S{"description": "Not found", "content": S{"application/json": S{"schema": ref("Error")}}}
		}
		if method != "GET" {
			responses["409"] = S{"description": "Conflict (duplicate name, wrong state)", "content": S{"application/json": S{"schema": ref("Error")}}}
		}
		o["responses"] = responses
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		paths[path][strings.ToLower(method)] = o
	}
	tags := make([]any, 0, len(apiTags))
	for _, t := range apiTags {
		tags = append(tags, S{"name": t.Name, "description": t.Desc})
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": S{"title": "ctrl32 telemetry API", "version": version, "description": apiIntro,
			"license": S{"name": "Apache-2.0", "identifier": "Apache-2.0"}, "contact": S{"name": "telemetry.digital", "url": "https://telemetry.digital/"}},
		"servers": []any{S{"url": "/", "description": "this server"}},
		"tags":    tags,
		"paths":   paths,
		"components": S{
			"schemas": apiSchemas,
			"securitySchemes": S{
				"bearerToken":     S{"type": "http", "scheme": "bearer", "description": "Personal API token (My account → API tokens). Scopes are permission names."},
				"sessionCookie":   S{"type": "apiKey", "in": "cookie", "name": sessionCookie, "description": "Browser session from POST /api/v1/auth/login; mutating requests also send X-CSRF-Token."},
				"deviceToken":     S{"type": "http", "scheme": "bearer", "description": "Device token (issued at registration or by POST /api/v1/devices/{id}/token)."},
				"connectorSecret": S{"type": "http", "scheme": "bearer", "description": "Connector secret returned when a ChirpStack connector is created."},
			},
		},
		"security": []any{S{"bearerToken": []any{}}, S{"sessionCookie": []any{}}},
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func opID(method, path string) string {
	p := strings.TrimPrefix(path, "/api/v1/")
	p = strings.NewReplacer("{", "", "}", "").Replace(p)
	parts := nonWord.Split(p, -1)
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, s := range parts {
		if s == "" {
			continue
		}
		b.WriteString(strings.ToUpper(s[:1]) + s[1:])
	}
	return b.String()
}

// apiRoutes returns the "METHOD path" of every described operation, sorted.
func apiRoutes() []string {
	out := make([]string, 0, len(apiOps))
	for _, op := range apiOps {
		out = append(out, op.M)
	}
	sort.Strings(out)
	return out
}

var (
	openapiOnce sync.Once
	openapiBody []byte
)

// GET /api/v1/openapi.json
func (s *Server) openapiJSON(w http.ResponseWriter, r *http.Request) {
	openapiOnce.Do(func() {
		openapiBody, _ = json.MarshalIndent(buildOpenAPI(s.version), "", " ")
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(200)
	_, _ = w.Write(openapiBody)
}
