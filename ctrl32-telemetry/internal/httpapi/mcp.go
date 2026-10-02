package httpapi

// AI assistants over MCP (spec 022, ADR-0017): the endpoint /mcp speaks the Model Context Protocol and offers the whole
// REST API as tools, generated from the OpenAPI table (apiOps), so every operation added to the API is offered to
// assistants as well. Each area of the API (an OpenAPI tag) is two tools: <area>_read for GET operations and
// <area>_change for the others, so a client can let reading run without asking and ask before changes. A tool call
// names the operation and its parameters and runs through the same handlers as the REST API — permissions, camera
// groups, validation, the reason for every change and the audit trail are those of the API; the audit names the
// assistant ("… via MCP"). Operations that are not requests of a user are excluded with a reason (mcpExcluded); the
// unit test checks that every operation is either offered or excluded.
//
//	POST /mcp                                   Bearer: OAuth access token (oauth.go) or a personal API token
//	GET  /api/v1/mcp/settings                   any user      whether AI access is on, the address to add
//	PUT  /api/v1/mcp/settings                   user.admin    {enabled, reason}
//	GET  /api/v1/mcp/grants[?all=1]             any user      AI clients I approved (all=1: the organization's, user.admin)
//	DELETE /api/v1/mcp/grants/{id}?reason=      owner or user.admin
//
// AI access is off until an administrator of the organization switches it on (SAFETY FIRST). Reading through an
// assistant is free; changes through an assistant need the paid license of the server (the white-labeling license,
// ADR-0016 amended by ADR-0017) — without it every assistant is read-only.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/mcp"
)

const mcpPath = "/mcp"

// mcpAreas names the tools of each OpenAPI tag. A tag without an entry is not offered (all its operations must then be
// excluded by mcpExcluded).
var mcpAreas = map[string]string{
	tagAccount: "account", tagUsers: "users", tagOrgs: "organizations", tagSites: "sites", tagData: "data",
	tagDevices: "devices", tagCommands: "commands", tagConsole: "device_console", tagFota: "firmware", tagAlarms: "alarms",
	tagNotify: "notifications", tagRules: "rules", tagDash: "dashboards", tagHMI: "hmi_library", tagRepl: "replication",
	tagFlows: "flows", tagPublic: "public_dashboards", tagConn: "connectors", tagReports: "reports", tagExports: "exports",
	tagBrand: "branding", tagApps: "apps", tagServer: "server", tagProv: "provisioning", tagSmart: "smart_home",
	tagVideo: "cameras", tagLanding: "front_page", tagDisplays: "displays", tagVPN: "vpn", tagAudit: "audit", tagSearch: "search",
}

// mcpExclusions are single operations that are not offered to assistants, with the reason.
var mcpExclusions = map[string]string{
	"GET /api/v1/openapi.json":                          "offered as the resource ctrl32://openapi.json and through the tool api_reference",
	"POST /api/v1/auth/password":                        "the user changes their own password",
	"POST /api/v1/auth/totp/setup":                      "two-factor enrolment is done by the user",
	"POST /api/v1/auth/totp/confirm":                    "two-factor enrolment is done by the user",
	"POST /api/v1/push/subscriptions":                   "a Web Push subscription belongs to a browser",
	"DELETE /api/v1/push/subscriptions/{id}":            "a Web Push subscription belongs to a browser",
	"POST /api/v1/replication/pair":                     "pairing of a replica server with a one-time code (machine to machine)",
	"POST /api/v1/display/hello":                        "pairing of a display (kiosk) — the display calls it itself",
	"POST /api/v1/flow-hooks/{token}":                   "incoming webhook of other systems (the secret is the address)",
	"GET /api/v1/replication/role":                      "used by the replica servers themselves",
	"GET /api/v1/pwa-apps/{slug}/launch":                "used by the installed app window itself",
	"GET /api/v1/i18n/{locale}":                         "the translation catalog of the web UI (large; the texts are in the pages)",
	"GET /api/v1/branding/icon":                         "browser icon of the web UI",
	"POST /api/v1/video/cameras/{id}/picture-saved":     "records a picture saved by the video player in the browser",
	"GET /api/v1/public/{token}/hmi/objects/{slug}/svg": "graphic of a shared picture, fetched by the public page",
	"POST /api/v1/vpn/peers":                            "issues a private key; needs the user's password (and authenticator code) again in the browser",
	"POST /api/v1/vpn/peers/{id}/rotate":                "issues a private key; needs the user's password (and authenticator code) again in the browser",
	"POST /api/v1/vpn/peers/{id}/grant":                 "opens network access into sites for a person; granted (and approved) by people in the browser",
	"POST /api/v1/reports/{id}/signatures":              "an electronic signature is given by the person, with the password again, in the browser",
	"POST /api/v1/audit/reviews":                        "a review of the audit trail is signed by the reviewer, with the password again, in the browser",
	"POST /api/v1/video/inventory/credentials":          "exports camera passwords; needs the user's password (and authenticator code) again in the browser",
	"GET /api/v1/video/inventory":                       "a file for people (Excel, CSV, PDF); assistants read the same settings with the camera list",
	"GET /api/v1/video/inventory/template":              "an empty file for people to fill in",
	"POST /api/v1/video/inventory/import":               "uploads a file from the user's computer; done in the browser or with the command line, after checking the preview",
	"POST /api/v1/vpn/uplink":                           "connects this server to an outside network with a pasted private key; needs the user's password (and authenticator code) again in the browser",
}

// mcpExcluded gives the reason an operation is not offered to assistants ("" = offered).
func mcpExcluded(op *apiOp) string {
	if r, ok := mcpExclusions[op.M]; ok {
		return r
	}
	switch op.P {
	case "device", "connector", "relay", "display", "replica token", "uplink":
		return "authenticated with a " + op.P + " credential, not by a user"
	}
	switch op.T {
	case tagAuth:
		return "sign-in, sign-out and password recovery of the web UI"
	case tagTokens:
		return "an AI assistant does not handle API tokens (lasting credentials)"
	case tagMCP:
		return "an AI assistant does not manage AI access"
	case tagDevAPI:
		return "the HTTP API of devices (device token)"
	case tagLive:
		return "a server-sent event stream, not a request with an answer"
	}
	if op.CT == "text/event-stream" {
		return "a server-sent event stream, not a request with an answer"
	}
	if strings.Contains(op.S, "(WebSocket)") || strings.Contains(op.S, "(HTTP upgrade)") || strings.HasPrefix(op.D, "WebSocket") || strings.HasPrefix(op.D, "Upgrade to a WebSocket") {
		return "a WebSocket (live or recorded video); a tool call is one request — use the snapshot and export operations"
	}
	if _, ok := mcpAreas[op.T]; !ok {
		return "no MCP area for the tag " + op.T
	}
	return ""
}

// mcpOp is an operation offered to assistants.
type mcpOp struct {
	op           *apiOp
	id           string // operation id, e.g. getVideoCameras
	method, path string
}

func (o *mcpOp) read() bool { return o.method == http.MethodGet }

// mcpTool is one generated tool: an area and a direction.
type mcpTool struct {
	name, area, tag string
	read            bool
	ops             []*mcpOp
}

var mcpCatalog = func() struct {
	tools []*mcpTool
	ops   map[string]*mcpOp
} {
	ops := map[string]*mcpOp{}
	tools := map[string]*mcpTool{}
	for i := range apiOps {
		op := &apiOps[i]
		if mcpExcluded(op) != "" {
			continue
		}
		method, path, _ := strings.Cut(op.M, " ")
		o := &mcpOp{op: op, id: opID(method, path), method: method, path: path}
		ops[o.id] = o
		name := mcpAreas[op.T] + "_change"
		if o.read() {
			name = mcpAreas[op.T] + "_read"
		}
		t := tools[name]
		if t == nil {
			t = &mcpTool{name: name, area: mcpAreas[op.T], tag: op.T, read: o.read()}
			tools[name] = t
		}
		t.ops = append(t.ops, o)
	}
	out := make([]*mcpTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return struct {
		tools []*mcpTool
		ops   map[string]*mcpOp
	}{out, ops}
}()

// mcpNoLicense explains why an assistant may only read on this server.
const mcpNoLicense = "changes through AI assistants need the license of this server (the ctrl32 license for white labeling and AI changes, System → License); reading is free — tell the user, or make the change in the web UI"

const mcpInstructions = `This is ctrl32 telemetry: devices and telemetry, alarms, dashboards and SCADA pictures, automation flows, smart home, energy, IP cameras and video walls, displays and server administration of one organization.

Tools come in pairs per area: <area>_read runs GET operations, <area>_change runs operations that change something. Every tool takes {"operation": id, "path": {...}, "query": {...}, "body": {...}}. The tool description lists its operations; call api_reference with operation ids (or a search text) for their parameters, body schema and answer before the first call.

Rules of this server:
- You act as the signed-in user with exactly their permissions (and only the cameras of their camera groups). A refused operation says which permission is missing.
- Every change needs "reason" (3–200 characters) — in the body, or as the query parameter "reason" for DELETE. It is written to the tamper-evident audit trail with your name as "… via MCP". Write the real reason the user gave, and ask the user before changing, deleting or sending commands to devices.
- Times are RFC 3339 in UTC, identifiers are UUIDs, lists are bounded by "limit".
- Sensitive data (camera sound, unmasked pictures, evidence) needs its own permission, as in the web UI.`

// ---- tools of one caller ------------------------------------------------------------------------------------------

type mcpBackend struct {
	s     *Server
	p     *iam.Principal
	r     *http.Request // the POST /mcp request (context, client address)
	write bool          // the server's license allows changes through assistants
}

// readOnly: the approval is for reading only, or the server has no license for changes through assistants.
func (b *mcpBackend) readOnly() bool { return b.p.ReadOnly || !b.write }

// allowed reports whether the caller may run the operation at all (the handler checks again).
func (b *mcpBackend) allowed(o *mcpOp) bool {
	if b.readOnly() && !o.read() {
		return false
	}
	switch o.op.P {
	case "", "public":
		return true
	}
	return permOK(b.p, o.op.P)
}

func (b *mcpBackend) Tools(ctx context.Context) []mcp.Tool {
	var out []mcp.Tool
	for _, t := range mcpCatalog.tools {
		var ops []*mcpOp
		for _, o := range t.ops {
			if b.allowed(o) {
				ops = append(ops, o)
			}
		}
		if len(ops) == 0 {
			continue
		}
		out = append(out, mcpToolDef(t, ops))
	}
	out = append(out, mcp.Tool{
		Name: "api_reference", Title: "API reference",
		Description: "Parameters, body schema, permission and answer of operations of the other tools. Give operation ids (as listed in the tool descriptions) or a search text (words of the summary, path or area).",
		InputSchema: S{"type": "object", "properties": S{
			"operations": S{"type": "array", "items": S{"type": "string"}, "maxItems": 20, "description": "operation ids, e.g. getVideoCameras"},
			"search":     S{"type": "string", "description": "words to look for in summaries, paths and areas"},
		}, "additionalProperties": false},
		Annotations: &mcp.Annotations{Title: "API reference", ReadOnlyHint: true, IdempotentHint: true},
	})
	return out
}

func mcpToolDef(t *mcpTool, ops []*mcpOp) mcp.Tool {
	title := t.tag
	var desc strings.Builder
	for _, tg := range apiTags {
		if tg.Name == t.tag {
			desc.WriteString(tg.Desc)
		}
	}
	if t.read {
		title += " — read"
		desc.WriteString("\n\nReading operations (GET):\n")
	} else {
		title += " — change"
		desc.WriteString("\n\nOperations that change something (every change needs a reason; ask the user first):\n")
	}
	ids := make([]any, 0, len(ops))
	multipart := false
	for _, o := range ops {
		ids = append(ids, o.id)
		perm := o.op.P
		if perm == "" || perm == "public" {
			perm = "any user"
		}
		fmt.Fprintf(&desc, "- %s: %s %s — %s [%s]\n", o.id, o.method, o.path, o.op.S, perm)
		if len(o.op.F) > 0 {
			multipart = true
		}
	}
	desc.WriteString("Call api_reference with operation ids for parameters and body schemas.")
	props := S{
		"operation": S{"type": "string", "enum": ids, "description": "the operation id"},
		"path":      S{"type": "object", "additionalProperties": S{"type": "string"}, "description": "path parameters by name, e.g. {\"id\": \"…\"}"},
		"query":     S{"type": "object", "additionalProperties": true, "description": "query parameters by name (for DELETE also \"reason\")"},
	}
	if !t.read {
		props["body"] = S{"type": "object", "additionalProperties": true, "description": "JSON body of the operation (see api_reference); changes carry \"reason\""}
	}
	if multipart {
		props["form"] = S{"type": "object", "additionalProperties": S{"type": "string"}, "description": "form fields of an upload operation (multipart)"}
		props["file"] = S{"type": "object", "properties": S{
			"name": S{"type": "string"}, "mime_type": S{"type": "string"}, "content_base64": S{"type": "string"},
		}, "required": []any{"name", "content_base64"}, "additionalProperties": false, "description": "the uploaded file of an upload operation"}
	}
	return mcp.Tool{
		Name: t.name, Title: title, Description: desc.String(),
		InputSchema: S{"type": "object", "properties": props, "required": []any{"operation"}, "additionalProperties": false},
		Annotations: &mcp.Annotations{Title: title, ReadOnlyHint: t.read, DestructiveHint: !t.read, IdempotentHint: t.read, OpenWorldHint: false},
	}
}

type mcpArgs struct {
	Operation string            `json:"operation"`
	Path      map[string]any    `json:"path"`
	Query     map[string]any    `json:"query"`
	Body      json.RawMessage   `json:"body"`
	Form      map[string]string `json:"form"`
	File      *struct {
		Name          string `json:"name"`
		MimeType      string `json:"mime_type"`
		ContentBase64 string `json:"content_base64"`
	} `json:"file"`
	// api_reference
	Operations []string `json:"operations"`
	Search     string   `json:"search"`
}

func (b *mcpBackend) Call(ctx context.Context, name string, raw json.RawMessage) (*mcp.Result, error) {
	var a mcpArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, mcp.InvalidParams("arguments: " + err.Error())
	}
	if name == "api_reference" {
		return b.reference(a), nil
	}
	var tool *mcpTool
	for _, t := range mcpCatalog.tools {
		if t.name == name {
			tool = t
		}
	}
	if tool == nil {
		return nil, mcp.InvalidParams("unknown tool " + name)
	}
	o := mcpCatalog.ops[a.Operation]
	if o == nil {
		return mcp.Text("unknown operation "+strconvQuote(a.Operation)+"; the tool description lists the operations of "+name, true), nil
	}
	if !strings.HasPrefix(name, mcpAreas[o.op.T]+"_") || o.read() != tool.read {
		other := mcpAreas[o.op.T] + "_change"
		if o.read() {
			other = mcpAreas[o.op.T] + "_read"
		}
		return mcp.Text("operation "+a.Operation+" belongs to the tool "+other, true), nil
	}
	if b.p.ReadOnly && !o.read() {
		return mcp.Text("this AI access was approved for reading only", true), nil
	}
	if !b.write && !o.read() {
		return mcp.Text(mcpNoLicense, true), nil
	}
	return b.s.mcpRun(ctx, b.p, b.r, o, &a), nil
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

// mcpRun performs one operation through the API's own handlers and turns the answer into a tool result.
func (s *Server) mcpRun(ctx context.Context, p *iam.Principal, outer *http.Request, o *mcpOp, a *mcpArgs) *mcp.Result {
	path := o.path
	for _, m := range pathParamRe.FindAllStringSubmatch(o.path, -1) {
		v, ok := a.Path[m[1]]
		sv := fmt.Sprint(v)
		if !ok || v == nil || sv == "" {
			return mcp.Text("path parameter "+m[1]+" is required for "+o.id, true)
		}
		path = strings.Replace(path, m[0], url.PathEscape(sv), 1)
	}
	q := url.Values{}
	for k, v := range a.Query {
		switch t := v.(type) {
		case nil:
		case []any:
			parts := make([]string, 0, len(t))
			for _, x := range t {
				parts = append(parts, fmt.Sprint(x))
			}
			q.Set(k, strings.Join(parts, ","))
		case float64:
			q.Set(k, strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", t), "0"), "."))
		default:
			q.Set(k, fmt.Sprint(t))
		}
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var body io.Reader
	contentType := ""
	switch {
	case len(o.op.F) > 0:
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range a.Form {
			_ = mw.WriteField(k, v)
		}
		if a.File != nil {
			data, err := base64.StdEncoding.DecodeString(a.File.ContentBase64)
			if err != nil {
				return mcp.Text("file.content_base64 is not valid base64", true)
			}
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, a.File.Name))
			mt := a.File.MimeType
			if mt == "" {
				mt = "application/octet-stream"
			}
			h.Set("Content-Type", mt)
			fw, _ := mw.CreatePart(h)
			_, _ = fw.Write(data)
		}
		_ = mw.Close()
		body, contentType = &buf, mw.FormDataContentType()
	case len(a.Body) > 0 && string(a.Body) != "null":
		if o.read() {
			return mcp.Text(o.id+" takes no body (use query)", true)
		}
		body, contentType = bytes.NewReader(a.Body), "application/json"
	case o.op.B != nil:
		body, contentType = strings.NewReader("{}"), "application/json"
	}
	ictx := context.WithValue(ctx, principalKey, p)
	ictx = context.WithValue(ictx, viaMCPKey, true)
	req, err := http.NewRequestWithContext(ictx, o.method, target, body)
	if err != nil {
		return mcp.Text("invalid request: "+err.Error(), true)
	}
	req.RemoteAddr = outer.RemoteAddr
	req.Host = outer.Host
	req.TLS = outer.TLS
	for _, h := range []string{"User-Agent", "X-Forwarded-For", "X-Forwarded-Proto", "Accept-Language"} {
		if v := outer.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := newCaptureWriter(mcpMaxAnswer)
	start := time.Now()
	s.mux.ServeHTTP(rec, req)
	s.log.Info("mcp call", "user", p.Username, "client", p.ClientName, "op", o.id, "status", rec.status, "ms", time.Since(start).Milliseconds())
	return mcpAnswer(o, target, rec)
}

const (
	mcpMaxAnswer = 8 << 20   // captured answer
	mcpMaxText   = 200 << 10 // text handed to the model
	mcpMaxImage  = 3 << 20   // image handed to the model
)

// mcpAnswer turns a captured answer into a tool result.
func mcpAnswer(o *mcpOp, target string, rec *captureWriter) *mcp.Result {
	ct := strings.ToLower(rec.Header().Get("Content-Type"))
	body := rec.buf.Bytes()
	failed := rec.status >= 400
	status := fmt.Sprintf("HTTP %d", rec.status)
	switch {
	case rec.status == http.StatusSwitchingProtocols:
		return mcp.Text("this operation needs a WebSocket", true)
	case strings.HasPrefix(ct, "application/json") || (len(body) > 0 && (body[0] == '{' || body[0] == '[') && ct == ""):
		var v any
		if json.Unmarshal(body, &v) == nil {
			if failed {
				if m, ok := v.(map[string]any); ok && m["error"] != nil {
					return mcp.Text(fmt.Sprintf("%s: %v", status, m["error"]), true)
				}
			}
			out, _ := json.Marshal(v)
			return mcp.Text(truncateText(string(out)), failed)
		}
	case strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "image/svg"):
		if !failed && len(body) <= mcpMaxImage && !rec.truncated {
			mt := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
			return &mcp.Result{Content: []mcp.Content{{Type: "image", Data: base64.StdEncoding.EncodeToString(body), MimeType: mt}}}
		}
		return mcp.Text(fmt.Sprintf("%s: an image of %d bytes, too large to show here; download %s with your token", status, rec.size, target), failed)
	case strings.HasPrefix(ct, "text/") || strings.HasPrefix(ct, "image/svg") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "x-pem"):
		txt := string(body)
		if failed {
			return mcp.Text(status+": "+truncateText(txt), true)
		}
		return mcp.Text(truncateText(txt), false)
	}
	if len(body) == 0 {
		if failed {
			return mcp.Text(status, true)
		}
		return mcp.Text(fmt.Sprintf("%s (no content)", status), false)
	}
	if failed {
		return mcp.Text(status+": "+truncateText(string(body)), true)
	}
	// binary answers (PDF, MP4, ZIP, firmware): the model gets the facts and where to fetch it
	name := ""
	if cd := rec.Header().Get("Content-Disposition"); cd != "" {
		name = " (" + cd + ")"
	}
	return mcp.Text(fmt.Sprintf("%s: %s of %d bytes%s. Binary content is not passed to the assistant; it can be downloaded as GET %s with an API token or in the web UI.", status, ct, rec.size, name, target), false)
}

func truncateText(s string) string {
	if len(s) <= mcpMaxText {
		return s
	}
	cut := s[:mcpMaxText]
	for len(cut) > 0 && !utf8.ValidString(cut) { // do not split the last character
		cut = cut[:len(cut)-1]
	}
	return cut + fmt.Sprintf("\n… (answer cut at %d of %d bytes: narrow it with limit, from/to or filters)", len(cut), len(s))
}

// captureWriter records an answer of a handler run for a tool call.
type captureWriter struct {
	h         http.Header
	status    int
	buf       bytes.Buffer
	size      int
	limit     int
	truncated bool
	wrote     bool
}

func newCaptureWriter(limit int) *captureWriter {
	return &captureWriter{h: http.Header{}, status: 200, limit: limit}
}

func (c *captureWriter) Header() http.Header { return c.h }
func (c *captureWriter) WriteHeader(code int) {
	if !c.wrote {
		c.status, c.wrote = code, true
	}
}
func (c *captureWriter) Write(p []byte) (int, error) {
	c.wrote = true
	c.size += len(p)
	if room := c.limit - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else {
		c.truncated = true
	}
	return len(p), nil
}
func (c *captureWriter) Flush() {}

// reference answers api_reference: the documentation of operations the caller may run.
func (b *mcpBackend) reference(a mcpArgs) *mcp.Result {
	var pick []*mcpOp
	seen := map[string]bool{}
	for _, id := range a.Operations {
		if o := mcpCatalog.ops[strings.TrimSpace(id)]; o != nil && !seen[o.id] {
			pick, seen[o.id] = append(pick, o), true
		}
	}
	if words := strings.Fields(strings.ToLower(a.Search)); len(words) > 0 {
		ids := make([]string, 0, len(mcpCatalog.ops))
		for id := range mcpCatalog.ops {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			o := mcpCatalog.ops[id]
			hay := strings.ToLower(o.id + " " + o.op.M + " " + o.op.S + " " + o.op.T + " " + mcpAreas[o.op.T])
			all := true
			for _, w := range words {
				if !strings.Contains(hay, w) {
					all = false
					break
				}
			}
			if all && !seen[o.id] && len(pick) < 25 {
				pick, seen[o.id] = append(pick, o), true
			}
		}
	}
	if len(pick) == 0 {
		return mcp.Text("no operation found: give operation ids from the tool descriptions, or other search words", true)
	}
	out := make([]any, 0, len(pick))
	for _, o := range pick {
		out = append(out, mcpDescribe(o, b.allowed(o)))
	}
	j, _ := json.MarshalIndent(out, "", " ")
	return mcp.Text(truncateText(string(j)), false)
}

// mcpDescribe documents one operation with its schemas resolved.
func mcpDescribe(o *mcpOp, allowed bool) S {
	tool := mcpAreas[o.op.T] + "_change"
	if o.read() {
		tool = mcpAreas[o.op.T] + "_read"
	}
	d := S{"operation": o.id, "tool": tool, "method": o.method, "path": o.path, "summary": o.op.S}
	if o.op.D != "" {
		d["description"] = o.op.D
	}
	switch o.op.P {
	case "", "public":
		d["permission"] = "any signed-in user"
	default:
		d["permission"] = o.op.P
	}
	if !allowed {
		d["you_may_call"] = false
	}
	var pp []any
	for _, m := range pathParamRe.FindAllStringSubmatch(o.path, -1) {
		desc := pathParamDesc[m[1]]
		pp = append(pp, S{"name": m[1], "description": desc})
	}
	if len(pp) > 0 {
		d["path_parameters"] = pp
	}
	var qp []any
	for _, p := range o.op.Q {
		x := S{"name": p.Name, "type": p.Type, "description": p.Desc}
		if p.Required {
			x["required"] = true
		}
		if len(p.Enum) > 0 {
			x["enum"] = p.Enum
		}
		qp = append(qp, x)
	}
	if len(qp) > 0 {
		d["query_parameters"] = qp
	}
	if o.op.B != nil {
		d["body"] = resolveSchema(o.op.B, 0, map[string]bool{})
	}
	if len(o.op.F) > 0 {
		var ff []any
		for _, f := range o.op.F {
			ff = append(ff, S{"name": f.Name, "description": f.Desc, "required": f.Required})
		}
		d["form_fields"] = ff
		d["upload"] = "give the form fields as \"form\" and the file as \"file\" {name, mime_type, content_base64}"
	}
	switch {
	case o.op.CT != "":
		d["answer"] = o.op.CT + " — " + o.op.RD
	case o.op.R != nil:
		d["answer"] = resolveSchema(o.op.R, 0, map[string]bool{})
	}
	return d
}

// resolveSchema inlines $ref of the component schemas (to a depth, without cycles), so an assistant gets the whole shape.
func resolveSchema(v any, depth int, stack map[string]bool) any {
	switch t := v.(type) {
	case S:
		if r, ok := t["$ref"].(string); ok {
			name := strings.TrimPrefix(r, "#/components/schemas/")
			sc, ok := apiSchemas[name].(S)
			if !ok || depth > 6 || stack[name] {
				return S{"schema": name}
			}
			stack[name] = true
			out := resolveSchema(sc, depth+1, stack)
			delete(stack, name)
			return out
		}
		out := S{}
		for k, x := range t {
			out[k] = resolveSchema(x, depth, stack)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = resolveSchema(x, depth, stack)
		}
		return out
	}
	return v
}

func (b *mcpBackend) Resources(ctx context.Context) []mcp.Resource {
	return []mcp.Resource{
		{URI: "ctrl32://me", Name: "me", Title: "Who I act as", Description: "The user, organization and permissions this assistant acts with.", MimeType: "application/json"},
		{URI: "ctrl32://guide", Name: "guide", Title: "How to use this server", Description: "Tools, rules for changes and conventions.", MimeType: "text/markdown"},
		{URI: "ctrl32://openapi.json", Name: "openapi", Title: "OpenAPI document", Description: "The whole REST API (OpenAPI 3.1).", MimeType: "application/json"},
	}
}

func (b *mcpBackend) ReadResource(ctx context.Context, uri string) ([]mcp.ResourceContent, error) {
	switch uri {
	case "ctrl32://me":
		perms := make([]string, 0, len(b.p.Permissions))
		for k, v := range b.p.Permissions {
			if v {
				perms = append(perms, k)
			}
		}
		sort.Strings(perms)
		var orgName string
		_ = b.s.pool.QueryRow(ctx, `SELECT name FROM core.organization WHERE id = $1::uuid`, b.p.OrgID).Scan(&orgName)
		j, _ := json.MarshalIndent(S{"user": b.p.Username, "full_name": b.p.FullName, "organization": orgName, "permissions": perms,
			"read_only": b.readOnly(), "changes_licensed": b.write, "ai_client": b.p.ClientName, "camera_groups": cameraGroupsOf(b.p), "server_version": b.s.version}, "", " ")
		return []mcp.ResourceContent{{URI: uri, MimeType: "application/json", Text: string(j)}}, nil
	case "ctrl32://guide":
		return []mcp.ResourceContent{{URI: uri, MimeType: "text/markdown", Text: mcpInstructions}}, nil
	case "ctrl32://openapi.json":
		j, _ := json.Marshal(buildOpenAPI(b.s.version))
		return []mcp.ResourceContent{{URI: uri, MimeType: "application/json", Text: string(j)}}, nil
	}
	return nil, mcp.InvalidParams("unknown resource " + uri)
}

func cameraGroupsOf(p *iam.Principal) any {
	if !p.CameraLimited {
		return "all cameras"
	}
	return p.CameraGroups
}

// ---- the endpoint -------------------------------------------------------------------------------------------------

// mcpEndpoint serves POST /mcp.
func (s *Server) mcpEndpoint(w http.ResponseWriter, r *http.Request) {
	mcpCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	p := PrincipalFrom(r.Context())
	if p == nil || p.SessionID != "" {
		// a browser session is not accepted here (no CSRF protection for a JSON-RPC endpoint): OAuth or an API token
		challenge := `Bearer resource_metadata="` + s.publicBase(r) + `/.well-known/oauth-protected-resource/mcp"`
		if bad, _ := r.Context().Value(badCredentialKey).(bool); bad {
			challenge += `, error="invalid_token"`
		}
		w.Header().Set("WWW-Authenticate", challenge)
		writeErr(w, http.StatusUnauthorized, "sign in: add this server as a connector in your AI assistant (OAuth), or send an API token as Authorization: Bearer")
		return
	}
	if !s.mcpEnabled(r.Context(), p.OrgID) {
		writeErr(w, http.StatusForbidden, "AI access (MCP) is switched off in this organization; an administrator switches it on under Settings → AI assistants")
		return
	}
	if !s.mcpLimiter.allow(p.UserID) {
		w.Header().Set("Retry-After", "10")
		writeErr(w, http.StatusTooManyRequests, "too many requests from this assistant; wait a few seconds")
		return
	}
	if p.GrantID != "" {
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.oauth_grant SET last_used_at = now() WHERE id = $1::uuid AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, p.GrantID)
	}
	write := s.mcpWriteLicensed(r.Context())
	instr := mcpInstructions
	if !write {
		instr += "\n- This server has no license for changes through AI assistants: only reading tools are offered. If the user asks for a change, say that " + mcpNoLicense + "."
	}
	srv := mcp.Server{Name: "ctrl32-telemetry", Title: s.productName(r, p.OrgID), Version: s.version, Instructions: instr}
	srv.ServeHTTP(w, r, &mcpBackend{s: s, p: p, r: r, write: write})
}

func mcpCORS(w http.ResponseWriter) {
	// bearer tokens only, no cookies: browser-based MCP clients (inspectors) may call from any origin
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, MCP-Protocol-Version, Mcp-Session-Id, Last-Event-ID")
	h.Set("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Session-Id")
	h.Set("Access-Control-Max-Age", "600")
}

// mcpWriteLicensed reports whether changes through assistants are licensed on this server: the same paid license as
// white labeling (one purchase unlocks both; ADR-0017).
func (s *Server) mcpWriteLicensed(ctx context.Context) bool { return s.whiteLabelLicensed(ctx) }

// mcpEnabled reports whether the organization allows AI access.
func (s *Server) mcpEnabled(ctx context.Context, org string) bool {
	var on bool
	_ = s.pool.QueryRow(ctx, `SELECT coalesce((settings->'mcp'->>'enabled')::boolean, false) FROM core.organization WHERE id = $1::uuid AND disabled_at IS NULL`, org).Scan(&on)
	return on
}

// ---- settings and grants API ----------------------------------------------------------------------------------------

// GET /api/v1/mcp/settings
func (s *Server) getMCPSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	base := s.publicBase(r)
	writeJSON(w, 200, S{"enabled": s.mcpEnabled(r.Context(), org), "can_edit": p.Has("user.admin"), "url": base + mcpPath,
		"authorization_server": base + "/.well-known/oauth-authorization-server", "tools": len(mcpCatalog.tools) + 1, "operations": len(mcpCatalog.ops),
		"write_licensed": s.mcpWriteLicensed(r.Context()), "buy_url": s.cfg.License.BuyURL})
}

// PUT /api/v1/mcp/settings
func (s *Server) putMCPSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled *bool  `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Enabled == nil {
		writeErr(w, 400, "enabled required")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	old := s.mcpEnabled(r.Context(), org)
	b, _ := json.Marshal(S{"enabled": *in.Enabled})
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{mcp}', coalesce(settings->'mcp', '{}'::jsonb) || $2::jsonb) WHERE id = $1::uuid`, org, string(b)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "mcp_settings.update", "organization", org, S{"enabled": old}, S{"enabled": *in.Enabled}, in.Reason)
	s.getMCPSettings(w, r)
}

// GET /api/v1/mcp/grants
func (s *Server) listMCPGrants(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	all := r.URL.Query().Get("all") == "1"
	if all && !p.Has("user.admin") {
		writeErr(w, 403, "permission denied: user.admin")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT g.id::text, u.username, c.name, c.client_uri, c.redirect_uris[1], g.read_only, g.created_at, g.last_used_at, g.revoked_at, COALESCE(g.revoked_reason, '')
		FROM core.oauth_grant g JOIN core.oauth_client c ON c.id = g.client_id JOIN core.user_account u ON u.id = g.user_id
		WHERE g.org_id = $1::uuid AND ($2 OR g.user_id = $3::uuid) AND (g.revoked_at IS NULL OR g.revoked_at > now() - interval '30 days')
		ORDER BY g.revoked_at IS NOT NULL, g.created_at DESC LIMIT 500`, org, all, p.UserID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []S{}
	for rows.Next() {
		var id, user, name, curi, redirect, reason string
		var ro bool
		var created time.Time
		var used, revoked *time.Time
		if err := rows.Scan(&id, &user, &name, &curi, &redirect, &ro, &created, &used, &revoked, &reason); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		host := ""
		if u, err := url.Parse(redirect); err == nil {
			host = u.Host
			if host == "" {
				host = u.Scheme + ":"
			}
		}
		out = append(out, S{"id": id, "user": user, "client_name": name, "client_uri": curi, "redirect_host": host, "read_only": ro,
			"created_at": created, "last_used_at": used, "revoked_at": revoked, "revoked_reason": reason})
	}
	writeJSON(w, 200, S{"grants": out})
}

// DELETE /api/v1/mcp/grants/{id}?reason=
func (s *Server) revokeMCPGrant(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	if len(reason) > 200 {
		writeErr(w, 400, "reason: at most 200 characters")
		return
	}
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "grant not found")
		return
	}
	var owner, client string
	var revoked *time.Time
	err := s.pool.QueryRow(r.Context(), `SELECT g.user_id::text, c.name, g.revoked_at FROM core.oauth_grant g JOIN core.oauth_client c ON c.id = g.client_id WHERE g.id = $1::uuid AND g.org_id = $2::uuid`, id, org).Scan(&owner, &client, &revoked)
	if err != nil {
		writeErr(w, 404, "grant not found")
		return
	}
	if owner != p.UserID && !p.Has("user.admin") {
		writeErr(w, 403, "permission denied: user.admin")
		return
	}
	if revoked != nil {
		writeErr(w, 409, "already revoked")
		return
	}
	if err := s.revokeGrant(r.Context(), id, p.UserID, reason); err != nil {
		writeErr(w, 500, "revoke failed")
		return
	}
	s.audit(r, "mcp_grant.revoke", "oauth_grant", id, S{"client": client}, nil, reason)
	writeJSON(w, 200, S{"ok": true})
}

// revokeGrant ends a grant and every access token issued under it.
func (s *Server) revokeGrant(ctx context.Context, id, by, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE core.oauth_grant SET revoked_at = now(), revoked_by = NULLIF($2,'')::uuid, revoked_reason = $3, refresh_hash = NULL WHERE id = $1::uuid AND revoked_at IS NULL`, id, by, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.api_token SET revoked_at = now() WHERE grant_id = $1::uuid AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
