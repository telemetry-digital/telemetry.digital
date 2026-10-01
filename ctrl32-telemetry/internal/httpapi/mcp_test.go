package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// Every operation of the API is either offered to AI assistants or excluded with a reason; nothing falls through.
func TestMCPEveryOperationOfferedOrExcluded(t *testing.T) {
	offered, excluded := 0, 0
	for i := range apiOps {
		op := &apiOps[i]
		method, path, _ := strings.Cut(op.M, " ")
		o := mcpCatalog.ops[opID(method, path)]
		reason := mcpExcluded(op)
		switch {
		case reason != "" && o != nil:
			t.Errorf("%s is excluded (%s) and offered", op.M, reason)
		case reason == "" && o == nil:
			t.Errorf("%s is neither offered nor excluded", op.M)
		case reason != "":
			excluded++
		default:
			offered++
			if o.op != op {
				t.Errorf("%s: operation id %s taken by %s", op.M, o.id, o.op.M)
			}
		}
	}
	if offered+excluded != len(apiOps) || offered < 280 {
		t.Fatalf("offered %d, excluded %d of %d", offered, excluded, len(apiOps))
	}
	t.Logf("offered %d, excluded %d of %d operations in %d tools", offered, excluded, len(apiOps), len(mcpCatalog.tools)+1)
	// an exclusion names an operation that exists
	for m := range mcpExclusions {
		found := false
		for i := range apiOps {
			found = found || apiOps[i].M == m
		}
		if !found {
			t.Errorf("exclusion of an operation that does not exist: %s", m)
		}
	}
	// every tag with operations a user calls has an area
	for _, tg := range apiTags {
		if _, ok := mcpAreas[tg.Name]; !ok {
			switch tg.Name {
			case tagAuth, tagTokens, tagMCP, tagDevAPI, tagLive, tagMeta:
			default:
				t.Errorf("tag %q has no MCP area", tg.Name)
			}
		}
	}
}

// Tools stay within what clients accept: names, count, one direction per tool, valid JSON schemas.
func TestMCPToolsShape(t *testing.T) {
	nameRe := regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	if n := len(mcpCatalog.tools) + 1; n > 100 {
		t.Fatalf("%d tools: too many for some clients", n)
	}
	admin := &iam.Principal{Permissions: map[string]bool{}}
	for i := range apiOps {
		if apiOps[i].P != "" {
			admin.Permissions[apiOps[i].P] = true
		}
	}
	b := &mcpBackend{p: admin, write: true}
	tools := b.Tools(t.Context())
	seen := map[string]bool{}
	for _, tl := range tools {
		if !nameRe.MatchString(tl.Name) || seen[tl.Name] {
			t.Errorf("tool name %q", tl.Name)
		}
		seen[tl.Name] = true
		if _, err := json.Marshal(tl); err != nil {
			t.Errorf("%s: %v", tl.Name, err)
		}
		if len(tl.Description) > 12000 {
			t.Errorf("%s: description of %d characters", tl.Name, len(tl.Description))
		}
	}
	for _, tl := range mcpCatalog.tools {
		for _, o := range tl.ops {
			if o.read() != tl.read {
				t.Errorf("%s in %s", o.id, tl.name)
			}
		}
	}
	if !seen["cameras_read"] || !seen["cameras_change"] || !seen["api_reference"] || !seen["devices_change"] {
		t.Fatalf("expected tools missing: %v", seen)
	}
	// a read-only approval offers no change tools; a user without permissions sees fewer operations
	ro := &mcpBackend{p: &iam.Principal{Permissions: admin.Permissions, ReadOnly: true}, write: true}
	for _, tl := range ro.Tools(t.Context()) {
		if strings.HasSuffix(tl.Name, "_change") {
			t.Errorf("read-only approval offers %s", tl.Name)
		}
	}
	// without the license for changes through assistants everyone only reads (reading is free)
	unlicensed := &mcpBackend{p: admin}
	for _, tl := range unlicensed.Tools(t.Context()) {
		if strings.HasSuffix(tl.Name, "_change") {
			t.Errorf("unlicensed server offers %s", tl.Name)
		}
	}
	if res, err := unlicensed.Call(t.Context(), "sites_change", []byte(`{"operation":"postSites","body":{"name":"x","reason":"y"}}`)); err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "license") {
		t.Errorf("unlicensed change: %v %+v", err, res)
	}
	none := &mcpBackend{p: &iam.Principal{Permissions: map[string]bool{}}, write: true}
	for _, tl := range none.Tools(t.Context()) {
		if strings.Contains(tl.Description, "[system.admin]") {
			t.Errorf("%s offers system.admin operations to a user without them", tl.Name)
		}
	}
}

// api_reference resolves schemas and finds operations by words.
func TestMCPReference(t *testing.T) {
	b := &mcpBackend{p: &iam.Principal{Permissions: map[string]bool{"video.view": true}}}
	res := b.reference(mcpArgs{Operations: []string{"postVideoCameras"}})
	if res.IsError || !strings.Contains(res.Content[0].Text, `"reason"`) || strings.Contains(res.Content[0].Text, "$ref") {
		t.Fatalf("reference: %s", res.Content[0].Text)
	}
	res = b.reference(mcpArgs{Search: "snapshot"})
	if res.IsError || !strings.Contains(res.Content[0].Text, "Snapshot") {
		t.Fatalf("search: %s", res.Content[0].Text)
	}
	if b.reference(mcpArgs{Search: "zzzz-nothing"}).IsError != true {
		t.Fatal("nothing found must be an error")
	}
}

// Answers of the handlers become tool results: JSON, errors, images, binaries, long texts.
func TestMCPAnswer(t *testing.T) {
	o := &mcpOp{id: "getX", method: "GET", path: "/api/v1/x", op: &apiOp{}}
	mk := func(status int, ct string, body []byte) *captureWriter {
		c := newCaptureWriter(mcpMaxAnswer)
		if ct != "" {
			c.Header().Set("Content-Type", ct)
		}
		c.WriteHeader(status)
		_, _ = c.Write(body)
		return c
	}
	if r := mcpAnswer(o, "/api/v1/x", mk(200, "application/json", []byte(`{"a": 1}`))); r.IsError || r.Content[0].Text != `{"a":1}` {
		t.Fatalf("json: %+v", r)
	}
	if r := mcpAnswer(o, "/api/v1/x", mk(403, "application/json", []byte(`{"error":"permission denied: video.ptz"}`))); !r.IsError || r.Content[0].Text != "HTTP 403: permission denied: video.ptz" {
		t.Fatalf("error: %+v", r)
	}
	if r := mcpAnswer(o, "/api/v1/x", mk(200, "image/jpeg", []byte{0xff, 0xd8, 0xff})); r.Content[0].Type != "image" || r.Content[0].MimeType != "image/jpeg" {
		t.Fatalf("image: %+v", r)
	}
	if r := mcpAnswer(o, "/api/v1/x", mk(200, "application/pdf", []byte("%PDF-1.7"))); r.IsError || !strings.Contains(r.Content[0].Text, "not passed") {
		t.Fatalf("binary: %+v", r)
	}
	long := strings.Repeat("ž", mcpMaxText)
	if r := mcpAnswer(o, "/api/v1/x", mk(200, "text/csv", []byte(long))); !strings.Contains(r.Content[0].Text, "answer cut") || !utf8ok(r.Content[0].Text) {
		t.Fatal("long text not cut cleanly")
	}
	if r := mcpAnswer(o, "/api/v1/x", mk(http.StatusNoContent, "", nil)); r.IsError {
		t.Fatalf("no content: %+v", r)
	}
}

func utf8ok(s string) bool { return strings.ToValidUTF8(s, "?") == s }

func TestOAuthRedirects(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://assistant.example/oauth/callback": true,
		"http://localhost:33418/callback":          true,
		"http://127.0.0.1:5000/cb":                 true,
		"http://[::1]:5000/cb":                     true,
		"cursor://anysphere.cursor-mcp/oauth":      true,
		"http://example.com/cb":                    false,
		"javascript:alert(1)":                      false,
		"https://x.example/cb#frag":                false,
		"https://user:pw@x.example/cb":             false,
		"data:text/html,x":                         false,
		"https:///nohost":                          false,
	} {
		if validRedirect(u) != ok {
			t.Errorf("validRedirect(%q) = %v", u, !ok)
		}
	}
	if !redirectMatches("http://127.0.0.1:1/cb", "http://127.0.0.1:5555/cb") || redirectMatches("http://127.0.0.1:1/cb", "http://127.0.0.1:5555/other") {
		t.Error("loopback port")
	}
	if redirectMatches("https://a.example/cb", "https://a.example/cb?x=1") || !redirectMatches("https://a.example/cb", "https://a.example/cb") {
		t.Error("exact match")
	}
}
