// Package mcp speaks the server side of the Model Context Protocol over Streamable HTTP (spec 022, ADR-0017): JSON-RPC
// 2.0 messages posted to one endpoint and answered with application/json. The server keeps no stream and no session
// (both are optional in the protocol), so every POST stands on its own and any instance behind a proxy can answer it.
// Methods: initialize, ping, tools/list, tools/call, resources/list, resources/read, resources/templates/list and
// prompts/list. What the tools and resources are is the Backend's business; this package only speaks the protocol.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
)

// Versions are the protocol revisions this server speaks, newest first. A client asking for another one gets the
// newest (the client then decides whether it can continue).
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// MaxRequest is the largest message accepted.
const MaxRequest = 4 << 20

// Tool is one tool as listed to the client.
type Tool struct {
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description"`
	InputSchema any          `json:"inputSchema"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Annotations are hints for the client (for example: read-only tools may run without asking).
type Annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// Content is one piece of a tool result: text, or an image as base64.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// Result is the result of a tool call. IsError marks a failure the model should see (a refused or failed operation);
// protocol errors (unknown tool, malformed arguments) are returned as errors of the call instead.
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Text makes a text result.
func Text(s string, isError bool) *Result {
	return &Result{Content: []Content{{Type: "text", Text: s}}, IsError: isError}
}

// Resource is a readable document offered to the client.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent is the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text"`
}

// Backend provides the tools and resources of one caller (they may depend on who calls).
type Backend interface {
	Tools(ctx context.Context) []Tool
	// Call runs a tool. An error of type *Error is a protocol error (for example an unknown tool); any result,
	// including a failed operation, is returned as *Result.
	Call(ctx context.Context, name string, args json.RawMessage) (*Result, error)
	Resources(ctx context.Context) []Resource
	ReadResource(ctx context.Context, uri string) ([]ResourceContent, error)
}

// Error is a JSON-RPC error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// JSON-RPC error codes.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// InvalidParams is a protocol error for arguments the server cannot use.
func InvalidParams(msg string) *Error { return &Error{Code: CodeInvalidParams, Message: msg} }

// Server answers the protocol for a Backend.
type Server struct {
	Name, Title, Version string
	// Instructions tell the model how to use this server (returned by initialize).
	Instructions string
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// ServeHTTP handles one POST of the Streamable HTTP transport. GET (a server stream) and DELETE (ending a session)
// are not offered: 405, as the protocol allows.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request, b Backend) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed: this MCP server answers POST only (no server-sent stream)", http.StatusMethodNotAllowed)
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !slices.Contains(Versions, v) {
		http.Error(w, "unsupported MCP-Protocol-Version "+v, http.StatusBadRequest)
		return
	}
	if ct := strings.ToLower(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequest+1))
	if err != nil || len(body) > MaxRequest {
		writeJSON(w, http.StatusRequestEntityTooLarge, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{Code: CodeInvalidRequest, Message: "message too large or unreadable"}})
		return
	}
	body = bytes.TrimSpace(body)
	// batches (protocol 2025-03-26) are still answered, one response per request
	if len(body) > 0 && body[0] == '[' {
		var msgs []json.RawMessage
		if err := json.Unmarshal(body, &msgs); err != nil || len(msgs) == 0 || len(msgs) > 50 {
			writeJSON(w, http.StatusBadRequest, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{Code: CodeParse, Message: "invalid batch"}})
			return
		}
		var out []response
		for _, m := range msgs {
			if resp := s.handle(r.Context(), m, b); resp != nil {
				out = append(out, *resp)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	resp := s.handle(r.Context(), body, b)
	if resp == nil { // a notification or a response of the client
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handle answers one message; nil for notifications and responses.
func (s *Server) handle(ctx context.Context, raw []byte, b Backend) *response {
	var m message
	if err := json.Unmarshal(raw, &m); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{Code: CodeParse, Message: "invalid JSON"}}
	}
	if m.Method == "" {
		if m.Result != nil || m.Error != nil {
			return nil // a response to a server request (the server never sends any)
		}
		return &response{JSONRPC: "2.0", ID: idOrNull(m.ID), Error: &Error{Code: CodeInvalidRequest, Message: "method required"}}
	}
	if m.JSONRPC != "2.0" {
		return &response{JSONRPC: "2.0", ID: idOrNull(m.ID), Error: &Error{Code: CodeInvalidRequest, Message: `jsonrpc must be "2.0"`}}
	}
	if len(m.ID) == 0 || string(m.ID) == "null" {
		return nil // notifications/initialized, notifications/cancelled, …: nothing to answer
	}
	result, err := s.dispatch(ctx, m.Method, m.Params, b)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = &Error{Code: CodeInternal, Message: err.Error()}
		}
		return &response{JSONRPC: "2.0", ID: m.ID, Error: e}
	}
	return &response{JSONRPC: "2.0", ID: m.ID, Result: result}
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage, b Backend) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		v := Versions[0]
		if slices.Contains(Versions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
				"prompts":   map[string]any{"listChanged": false},
			},
			"serverInfo":   map[string]any{"name": s.Name, "title": s.Title, "version": s.Version},
			"instructions": s.Instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		t := b.Tools(ctx)
		if t == nil {
			t = []Tool{}
		}
		return map[string]any{"tools": t}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
			return nil, InvalidParams("name required")
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		res, err := b.Call(ctx, p.Name, p.Arguments)
		if err != nil {
			return nil, err
		}
		if res.Content == nil {
			res.Content = []Content{}
		}
		return res, nil
	case "resources/list":
		rs := b.Resources(ctx)
		if rs == nil {
			rs = []Resource{}
		}
		return map[string]any{"resources": rs}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
			return nil, InvalidParams("uri required")
		}
		c, err := b.ReadResource(ctx, p.URI)
		if err != nil {
			return nil, err
		}
		return map[string]any{"contents": c}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	case "logging/setLevel", "completion/complete":
		return map[string]any{}, nil
	}
	return nil, &Error{Code: CodeMethodNotFound, Message: "method not found: " + method}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
