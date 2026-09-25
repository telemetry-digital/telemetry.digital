// apigen writes the TypeScript client and the Bruno collection from the OpenAPI document (URS G-08):
//
//	go run ./tools/apigen            # writes clients/typescript/ctrl32-api.ts and clients/bruno/…
//	go run ./tools/apigen -check     # exits 1 when the files on disk differ from the generated ones
//
// The test in this package runs the generator and compares it with the committed files, so the clients cannot
// fall behind the API description (which in turn cannot fall behind the routes).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/httpapi"
)

type S = map[string]any

func main() {
	out := flag.String("out", "clients", "output directory")
	check := flag.Bool("check", false, "compare with the files on disk instead of writing")
	flag.Parse()
	files := Generate(httpapi.OpenAPIDocument(""))
	if *check {
		bad := 0
		for name, want := range files {
			got, err := os.ReadFile(filepath.Join(*out, filepath.FromSlash(name)))
			if err != nil || !bytes.Equal(got, want) {
				fmt.Println("stale:", name)
				bad++
			}
		}
		if bad > 0 {
			fmt.Printf("%d file(s) differ — run: go run ./tools/apigen\n", bad)
			os.Exit(1)
		}
		fmt.Println("clients are current")
		return
	}
	// remove previously generated Bruno files so renamed operations do not leave stale requests behind
	_ = os.RemoveAll(filepath.Join(*out, "bruno"))
	for name, data := range files {
		p := filepath.Join(*out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d files to %s\n", len(files), *out)
}

// Generate returns the client files keyed by path relative to the clients directory.
func Generate(doc map[string]any) map[string][]byte {
	files := map[string][]byte{}
	files["typescript/ctrl32-api.ts"] = []byte(typescript(doc))
	for name, data := range bruno(doc) {
		files["bruno/"+name] = data
	}
	return files
}

// ---- helpers over the document

func schemas(doc map[string]any) S { return doc["components"].(S)["schemas"].(S) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var methodOrder = []string{"get", "post", "put", "patch", "delete"}

type operation struct {
	method, path, id, tag, summary, desc string
	op                                   S
	pathParams                           []string
	query                                []S
	bodySchema                           S
	multipart                            S
	respSchema                           S
	binary                               bool
}

var pathParamRe = regexp.MustCompile(`\{([a-z]+)\}`)

func operations(doc map[string]any) []operation {
	paths := doc["paths"].(map[string]map[string]any)
	var ops []operation
	for _, path := range sortedKeys(paths) {
		for _, m := range methodOrder {
			raw, ok := paths[path][m]
			if !ok {
				continue
			}
			o := raw.(S)
			op := operation{method: m, path: path, id: o["operationId"].(string), tag: o["tags"].([]string)[0], summary: o["summary"].(string), op: o}
			if d, ok := o["description"].(string); ok {
				op.desc = d
			}
			for _, pm := range pathParamRe.FindAllStringSubmatch(path, -1) {
				op.pathParams = append(op.pathParams, pm[1])
			}
			if ps, ok := o["parameters"].([]any); ok {
				for _, p := range ps {
					pp := p.(S)
					if pp["in"] == "query" {
						op.query = append(op.query, pp)
					}
				}
			}
			if rb, ok := o["requestBody"].(S); ok {
				content := rb["content"].(S)
				if c, ok := content["application/json"].(S); ok {
					op.bodySchema = c["schema"].(S)
				} else if c, ok := content["multipart/form-data"].(S); ok {
					op.multipart = c["schema"].(S)
				}
			}
			responses := o["responses"].(S)
			for _, code := range []string{"200", "201", "202"} {
				r, ok := responses[code].(S)
				if !ok {
					continue
				}
				content := r["content"].(S)
				if c, ok := content["application/json"].(S); ok {
					op.respSchema = c["schema"].(S)
				} else {
					op.binary = true
				}
				break
			}
			ops = append(ops, op)
		}
	}
	return ops
}

// ---- TypeScript

func tsType(s S, indent string) string {
	if s == nil {
		return "unknown"
	}
	if ref, ok := s["$ref"].(string); ok {
		return ref[strings.LastIndex(ref, "/")+1:]
	}
	if one, ok := s["oneOf"].([]any); ok {
		parts := make([]string, 0, len(one))
		for _, x := range one {
			parts = append(parts, tsType(x.(S), indent))
		}
		return strings.Join(parts, " | ")
	}
	nullable := false
	var typ string
	switch t := s["type"].(type) {
	case string:
		typ = t
	case []any:
		for _, x := range t {
			if x == "null" {
				nullable = true
			} else if xs, ok := x.(string); ok {
				typ = xs
			}
		}
	}
	var out string
	if e, ok := s["enum"].([]any); ok {
		parts := make([]string, 0, len(e))
		for _, v := range e {
			parts = append(parts, fmt.Sprintf("%q", fmt.Sprint(v)))
		}
		out = strings.Join(parts, " | ")
	} else {
		switch typ {
		case "string":
			out = "string"
		case "number", "integer":
			out = "number"
		case "boolean":
			out = "boolean"
		case "array":
			items, _ := s["items"].(S)
			it := tsType(items, indent)
			if strings.Contains(it, " | ") || strings.Contains(it, " ") {
				it = "(" + it + ")"
			}
			out = it + "[]"
		case "object":
			if props, ok := s["properties"].(S); ok && len(props) > 0 {
				out = tsObject(props, s, indent)
			} else if ap, ok := s["additionalProperties"].(S); ok {
				out = "Record<string, " + tsType(ap, indent) + ">"
			} else {
				out = "Record<string, unknown>"
			}
		default:
			if props, ok := s["properties"].(S); ok && len(props) > 0 {
				out = tsObject(props, s, indent)
			} else {
				out = "unknown"
			}
		}
	}
	if nullable {
		out += " | null"
	}
	return out
}

func tsObject(props S, s S, indent string) string {
	required := map[string]bool{}
	if r, ok := s["required"].([]string); ok {
		for _, k := range r {
			required[k] = true
		}
	}
	var b strings.Builder
	b.WriteString("{\n")
	for _, k := range sortedKeys(props) {
		ps := props[k].(S)
		if d, _ := ps["description"].(string); d != "" {
			b.WriteString(indent + "  /** " + strings.ReplaceAll(d, "*/", "* /") + " */\n")
		}
		opt := "?"
		if required[k] {
			opt = ""
		}
		b.WriteString(fmt.Sprintf("%s  %s%s: %s;\n", indent, tsKey(k), opt, tsType(ps, indent+"  ")))
	}
	b.WriteString(indent + "}")
	return b.String()
}

var identRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func tsKey(k string) string {
	if identRe.MatchString(k) {
		return k
	}
	return fmt.Sprintf("%q", k)
}

func typescript(doc map[string]any) string {
	var b strings.Builder
	b.WriteString("// ctrl32 telemetry API client — generated from the OpenAPI document by tools/apigen. Do not edit; run: go run ./tools/apigen\n")
	b.WriteString("// Usage: const api = new Ctrl32Client({ baseUrl: 'https://telemetry.example.com', token: 'c32_…' });\n")
	b.WriteString("//        const devices = await api.getDevices();\n")
	b.WriteString("// In a browser signed in with a session, pass { csrfToken } from getAuthMe() instead of a token.\n\n")
	sch := schemas(doc)
	for _, name := range sortedKeys(sch) {
		s := sch[name].(S)
		if d, _ := s["description"].(string); d != "" {
			b.WriteString("/** " + d + " */\n")
		}
		b.WriteString("export type " + name + " = " + tsType(s, "") + ";\n\n")
	}
	b.WriteString(`export interface Ctrl32Options {
  /** Server origin, e.g. https://telemetry.example.com (default: the page's origin) */
  baseUrl?: string;
  /** Personal API token; when absent the browser session cookie is used */
  token?: string;
  /** csrf_token from getAuthMe() — required for mutating calls with a session cookie */
  csrfToken?: string;
  /** Custom fetch (Node < 18, tests) */
  fetch?: typeof fetch;
}

export class Ctrl32Error extends Error {
  constructor(public readonly status: number, message: string, public readonly body?: unknown) {
    super(message);
    this.name = 'Ctrl32Error';
  }
}

export class Ctrl32Client {
  constructor(private readonly opts: Ctrl32Options = {}) {}

  /** Low-level request; returns the parsed JSON body, or the Response when raw is true. */
  async request<T>(method: string, path: string, query?: Record<string, unknown>, body?: unknown, raw = false): Promise<T> {
    const base = this.opts.baseUrl ?? (typeof location !== 'undefined' ? location.origin : 'http://127.0.0.1:8080');
    const url = new URL(path, base);
    if (query) for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== null) url.searchParams.set(k, String(v));
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (this.opts.token) headers['Authorization'] = 'Bearer ' + this.opts.token;
    if (this.opts.csrfToken && method !== 'GET') headers['X-CSRF-Token'] = this.opts.csrfToken;
    let payload: BodyInit | undefined;
    if (typeof FormData !== 'undefined' && body instanceof FormData) payload = body;
    else if (body !== undefined) { headers['Content-Type'] = 'application/json'; payload = JSON.stringify(body); }
    const f = this.opts.fetch ?? fetch;
    const res = await f(url.toString(), { method, headers, body: payload, credentials: this.opts.token ? 'omit' : 'include' });
    if (!res.ok) {
      let message = res.status + ' ' + res.statusText;
      let data: unknown;
      try { data = await res.json(); const e = (data as { error?: string }).error; if (e) message = e; } catch { /* no JSON body */ }
      throw new Ctrl32Error(res.status, message, data);
    }
    if (raw) return res as unknown as T;
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  /** Cursor pagination helper: follows X-Next-Cursor until the server stops sending one. */
  async *pages<T>(path: string, query: Record<string, unknown>): AsyncGenerator<T[]> {
    let cursor: string | undefined;
    for (;;) {
      const res = await this.request<Response>('GET', path, { ...query, cursor }, undefined, true);
      const rows = (await res.json()) as T[];
      yield rows;
      const next = res.headers.get('X-Next-Cursor');
      if (!next) return;
      cursor = next;
    }
  }
`)
	for _, op := range operations(doc) {
		var args []string
		for _, p := range op.pathParams {
			args = append(args, p+": string")
		}
		if op.bodySchema != nil {
			args = append(args, "body: "+tsType(op.bodySchema, "  "))
		} else if op.multipart != nil {
			args = append(args, "body: FormData")
		}
		queryArg := ""
		if len(op.query) > 0 {
			required := false
			var props []string
			for _, q := range op.query {
				opt := "?"
				if req, _ := q["required"].(bool); req {
					opt = ""
					required = true
				}
				typ := "string"
				if st, ok := q["schema"].(S); ok && (st["type"] == "integer" || st["type"] == "number") {
					typ = "number"
				}
				desc, _ := q["description"].(string)
				if desc != "" {
					desc = " /** " + desc + " */"
				}
				props = append(props, fmt.Sprintf("%s%s: %s;%s", tsKey(q["name"].(string)), opt, typ, desc))
			}
			qopt := "?"
			if required {
				qopt = ""
			}
			queryArg = "query" + qopt + ": { " + strings.Join(props, " ") + " }"
			args = append(args, queryArg)
		}
		ret := "unknown"
		raw := "false"
		if op.binary {
			ret = "Response"
			raw = "true"
		} else if op.respSchema != nil {
			ret = tsType(op.respSchema, "  ")
		}
		path := pathParamRe.ReplaceAllStringFunc(op.path, func(m string) string { return "${encodeURIComponent(" + m[1:len(m)-1] + ")}" })
		doc := op.summary
		if op.desc != "" {
			doc += "\n   *\n   * " + strings.ReplaceAll(strings.ReplaceAll(op.desc, "*/", "* /"), "\n", "\n   * ")
		}
		b.WriteString("\n  /** " + doc + " */\n")
		b.WriteString(fmt.Sprintf("  %s(%s): Promise<%s> {\n", op.id, strings.Join(args, ", "), ret))
		body := "undefined"
		if op.bodySchema != nil || op.multipart != nil {
			body = "body"
		}
		query := "undefined"
		if len(op.query) > 0 {
			query = "query"
		}
		b.WriteString(fmt.Sprintf("    return this.request<%s>('%s', `%s`, %s, %s, %s);\n  }\n", ret, strings.ToUpper(op.method), path, query, body, raw))
	}
	b.WriteString("}\n")
	return b.String()
}

// ---- Bruno collection

func exampleOf(sch S, all S, depth int) any {
	if depth > 6 || sch == nil {
		return nil
	}
	if ref, ok := sch["$ref"].(string); ok {
		return exampleOf(all[ref[strings.LastIndex(ref, "/")+1:]].(S), all, depth+1)
	}
	if one, ok := sch["oneOf"].([]any); ok && len(one) > 0 {
		return exampleOf(one[0].(S), all, depth+1)
	}
	if e, ok := sch["enum"].([]any); ok && len(e) > 0 {
		return e[0]
	}
	typ := ""
	switch t := sch["type"].(type) {
	case string:
		typ = t
	case []any:
		for _, x := range t {
			if x != "null" {
				typ, _ = x.(string)
			}
		}
	}
	if props, ok := sch["properties"].(S); ok && (typ == "object" || typ == "") {
		out := map[string]any{}
		for k, v := range props {
			out[k] = exampleOf(v.(S), all, depth+1)
		}
		return out
	}
	switch typ {
	case "object":
		if ap, ok := sch["additionalProperties"].(S); ok {
			return map[string]any{"key": exampleOf(ap, all, depth+1)}
		}
		return map[string]any{}
	case "array":
		items, _ := sch["items"].(S)
		return []any{exampleOf(items, all, depth+1)}
	case "string":
		switch sch["format"] {
		case "uuid":
			return "018f5c2a-1b7e-7c3d-9a41-3f2e6b8d1c00"
		case "date-time":
			return "2026-09-24T12:00:00Z"
		}
		return "string"
	case "number":
		return 21.5
	case "integer":
		return 1
	case "boolean":
		return true
	}
	return nil
}

var unsafeRe = regexp.MustCompile(`[^A-Za-z0-9 _()-]+`)

func fileName(s string) string {
	s = unsafeRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		s = strings.TrimSpace(s[:60])
	}
	return s
}

func bruno(doc map[string]any) map[string][]byte {
	files := map[string][]byte{}
	files["bruno.json"] = []byte("{\n  \"version\": \"1\",\n  \"name\": \"ctrl32 telemetry API\",\n  \"type\": \"collection\",\n  \"ignore\": [\"node_modules\", \".git\"]\n}\n")
	files["collection.bru"] = []byte("auth {\n  mode: bearer\n}\n\nauth:bearer {\n  token: {{token}}\n}\n\ndocs {\n  ctrl32 telemetry API — generated from the OpenAPI document by tools/apigen.\n  Set baseUrl and token in the environment (My account → API tokens). Every mutating request carries a reason.\n}\n")
	files["environments/local.bru"] = []byte("vars {\n  baseUrl: http://127.0.0.1:8080\n  token: \n}\n\nvars:secret [\n  token\n]\n")
	files["environments/production.bru"] = []byte("vars {\n  baseUrl: https://telemetry.example.com\n  token: \n}\n\nvars:secret [\n  token\n]\n")
	all := schemas(doc)
	seq := map[string]int{}
	for _, op := range operations(doc) {
		folder := fileName(op.tag)
		seq[folder]++
		var b strings.Builder
		fmt.Fprintf(&b, "meta {\n  name: %s\n  type: http\n  seq: %d\n}\n\n", fileName(op.summary), seq[folder])
		url := pathParamRe.ReplaceAllStringFunc(op.path, func(m string) string { return ":" + m[1:len(m)-1] })
		bodyKind := "none"
		if op.bodySchema != nil {
			bodyKind = "json"
		} else if op.multipart != nil {
			bodyKind = "multipartForm"
		}
		fmt.Fprintf(&b, "%s {\n  url: {{baseUrl}}%s\n  body: %s\n  auth: inherit\n}\n\n", op.method, url, bodyKind)
		if len(op.pathParams) > 0 {
			b.WriteString("params:path {\n")
			for _, p := range op.pathParams {
				fmt.Fprintf(&b, "  %s: \n", p)
			}
			b.WriteString("}\n\n")
		}
		if len(op.query) > 0 {
			b.WriteString("params:query {\n")
			for _, q := range op.query {
				name := q["name"].(string)
				prefix := "~"
				if req, _ := q["required"].(bool); req {
					prefix = ""
				}
				fmt.Fprintf(&b, "  %s%s: \n", prefix, name)
			}
			b.WriteString("}\n\n")
		}
		if op.bodySchema != nil {
			ex, _ := json.MarshalIndent(exampleOf(op.bodySchema, all, 0), "  ", "  ")
			fmt.Fprintf(&b, "body:json {\n  %s\n}\n\n", string(ex))
		} else if op.multipart != nil {
			b.WriteString("body:multipart-form {\n")
			if props, ok := op.multipart["properties"].(S); ok {
				for _, k := range sortedKeys(props) {
					if k == "file" {
						fmt.Fprintf(&b, "  %s: @file()\n", k)
					} else {
						fmt.Fprintf(&b, "  %s: \n", k)
					}
				}
			}
			b.WriteString("}\n\n")
		}
		docs := op.summary
		if op.desc != "" {
			docs += "\n  \n  " + strings.ReplaceAll(op.desc, "\n", "\n  ")
		}
		fmt.Fprintf(&b, "docs {\n  %s\n}\n", docs)
		files[fmt.Sprintf("%s/%02d %s.bru", folder, seq[folder], fileName(op.summary))] = []byte(b.String())
	}
	return files
}
