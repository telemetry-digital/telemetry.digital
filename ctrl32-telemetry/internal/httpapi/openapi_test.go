package httpapi

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every route registered in server.go must be described in openapi.go and vice versa (URS G-01).
func TestOpenAPICoversRoutes(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.Handle(?:Func)?\("([A-Z]+ /api/v1/[^"]+)"`)
	registered := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		registered[m[1]] = true
	}
	if len(registered) < 150 {
		t.Fatalf("only %d routes found in server.go — regexp broken?", len(registered))
	}
	described := map[string]bool{}
	for _, r := range apiRoutes() {
		if described[r] {
			t.Errorf("route described twice: %s", r)
		}
		described[r] = true
	}
	var missing, extra []string
	for r := range registered {
		if !described[r] {
			missing = append(missing, r)
		}
	}
	for r := range described {
		if !registered[r] {
			extra = append(extra, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("routes without OpenAPI description:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("OpenAPI operations without a route:\n  %s", strings.Join(extra, "\n  "))
	}
}

func TestOpenAPIDocument(t *testing.T) {
	doc := buildOpenAPI("test")
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"openapi":"3.1.0"`) {
		t.Error("openapi version")
	}
	// every $ref resolves, every operation has a summary, tag and operationId; operationIds are unique
	refRe := regexp.MustCompile(`"\$ref":"#/components/schemas/([A-Za-z]+)"`)
	schemas := doc["components"].(S)["schemas"].(S)
	for _, m := range refRe.FindAllStringSubmatch(string(b), -1) {
		if _, ok := schemas[m[1]]; !ok {
			t.Errorf("dangling $ref %s", m[1])
		}
	}
	tags := map[string]bool{}
	for _, tg := range doc["tags"].([]any) {
		tags[tg.(S)["name"].(string)] = true
	}
	ids := map[string]string{}
	for path, item := range doc["paths"].(map[string]map[string]any) {
		for method, raw := range item {
			o := raw.(S)
			if o["summary"] == "" {
				t.Errorf("%s %s: no summary", method, path)
			}
			tg := o["tags"].([]string)[0]
			if !tags[tg] {
				t.Errorf("%s %s: unknown tag %q", method, path, tg)
			}
			id := o["operationId"].(string)
			if prev, dup := ids[id]; dup {
				t.Errorf("operationId %s used by %s and %s %s", id, prev, method, path)
			}
			ids[id] = method + " " + path
			if _, ok := o["responses"].(S)["400"]; !ok {
				t.Errorf("%s %s: no 400 response", method, path)
			}
		}
	}
	if opID("GET", "/api/v1/devices/{id}/commands") != "getDevicesIdCommands" {
		t.Errorf("opID = %s", opID("GET", "/api/v1/devices/{id}/commands"))
	}
}
