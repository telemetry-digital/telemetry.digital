package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fake struct{}

func (fake) Tools(context.Context) []Tool {
	return []Tool{{Name: "echo", Description: "echo", InputSchema: map[string]any{"type": "object"}}}
}
func (fake) Call(_ context.Context, name string, args json.RawMessage) (*Result, error) {
	if name != "echo" {
		return nil, InvalidParams("unknown tool " + name)
	}
	return Text(string(args), false), nil
}
func (fake) Resources(context.Context) []Resource { return nil }
func (fake) ReadResource(_ context.Context, uri string) ([]ResourceContent, error) {
	return nil, InvalidParams("unknown resource " + uri)
}

func post(t *testing.T, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Name: "t", Version: "1", Instructions: "use it"}
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r, fake{})
	return w
}

func TestInitializeNegotiates(t *testing.T) {
	w := post(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"protocolVersion":"2025-06-18"`) || !strings.Contains(w.Body.String(), `"instructions":"use it"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = post(t, `{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if !strings.Contains(w.Body.String(), `"protocolVersion":"`+Versions[0]+`"`) || !strings.Contains(w.Body.String(), `"id":"a"`) {
		t.Fatalf("fallback: %s", w.Body)
	}
}

func TestNotificationsAndErrors(t *testing.T) {
	if w := post(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); w.Code != http.StatusAccepted || w.Body.Len() != 0 {
		t.Fatalf("notification: %d %q", w.Code, w.Body)
	}
	if w := post(t, `{"jsonrpc":"2.0","id":2,"method":"nope"}`); !strings.Contains(w.Body.String(), `"code":-32601`) {
		t.Fatalf("unknown method: %s", w.Body)
	}
	if w := post(t, `{not json`); !strings.Contains(w.Body.String(), `"code":-32700`) {
		t.Fatalf("parse error: %s", w.Body)
	}
	if w := post(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"other"}}`); !strings.Contains(w.Body.String(), `"code":-32602`) {
		t.Fatalf("unknown tool: %s", w.Body)
	}
	if w := post(t, `{"jsonrpc":"2.0","id":4,"method":"ping"}`, "MCP-Protocol-Version", "2000-01-01"); w.Code != 400 {
		t.Fatalf("bad protocol header: %d", w.Code)
	}
	s := &Server{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/mcp", nil), fake{})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", w.Code)
	}
}

func TestToolsAndBatch(t *testing.T) {
	w := post(t, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`)
	var resp struct {
		Result Result `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Result.Content[0].Text != `{"x":1}` {
		t.Fatalf("call: %s", w.Body)
	}
	w = post(t, `[{"jsonrpc":"2.0","id":6,"method":"tools/list"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":7,"method":"ping"}]`)
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out) != 2 {
		t.Fatalf("batch: %s", w.Body)
	}
	if w := post(t, `{"jsonrpc":"2.0","id":8,"method":"resources/list"}`); !strings.Contains(w.Body.String(), `"resources":[]`) {
		t.Fatalf("resources: %s", w.Body)
	}
}
