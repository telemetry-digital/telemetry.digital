package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Block designer (ADR-0010 phase 2, URS J-18): a template can be described as an ordered list of blocks with
// parameters instead of raw Typst. The spec is stored next to the generated source in core.report_template.spec;
// the generated source is what renders, so a spec-based template behaves exactly like a hand-written one and the
// source can always be inspected. Editing the source by hand drops the spec (the designer cannot round-trip it).

const SpecVersion = 1

type Spec struct {
	Version int     `json:"version"`
	Page    Page    `json:"page"`
	Blocks  []Block `json:"blocks"`
}

type Page struct {
	Paper     string  `json:"paper"`     // a4, a3, us-letter
	Landscape bool    `json:"landscape"` //
	Font      string  `json:"font"`      // Typst font family name
	FontSize  float64 `json:"font_size"` // pt
}

type Block struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

// ParamDef describes one parameter for the designer UI.
type ParamDef struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // text, multiline, number, bool, select, list
	Default any      `json:"default"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
}

type BlockDef struct {
	Type   string     `json:"type"`
	Label  string     `json:"label"`
	Help   string     `json:"help"`
	Params []ParamDef `json:"params"`
}

var blockCatalog = []BlockDef{
	{Type: "title", Label: "Title", Help: "Report title, subtitle, period and scope. Tokens such as {{title}}, {{org}}, {{period.from}}, {{scope.asset}} are replaced from the data.", Params: []ParamDef{
		{Name: "title", Label: "Title", Kind: "text", Default: "{{title}}"},
		{Name: "subtitle", Label: "Subtitle", Kind: "text", Default: "{{subtitle}}"},
		{Name: "show_period", Label: "Show period", Kind: "bool", Default: true},
		{Name: "show_scope", Label: "Show scope (asset, datastream, unit…)", Kind: "bool", Default: true},
	}},
	{Type: "summary", Label: "Summary facts", Help: "Boxes with the summary values (count, min, max, avg, gaps, alarms…).", Params: []ParamDef{
		{Name: "keys", Label: "Only these keys (empty = all)", Kind: "list", Default: []string{}},
	}},
	{Type: "chart", Label: "Chart", Help: "Line chart of the period (only kinds that provide one).", Params: []ParamDef{
		{Name: "caption", Label: "Caption", Kind: "text", Default: ""},
	}},
	{Type: "text", Label: "Text", Help: "Free text with tokens; one paragraph per line.", Params: []ParamDef{
		{Name: "text", Label: "Text", Kind: "multiline", Default: "Generated for {{org}} for the period {{period.from}} – {{period.to}}."},
		{Name: "style", Label: "Style", Kind: "select", Default: "normal", Options: []string{"normal", "note", "warning"}},
	}},
	{Type: "table", Label: "Data table", Help: "The rows of the report (observations, alarms, audit…).", Params: []ParamDef{
		{Name: "columns", Label: "Only these columns (empty = all)", Kind: "list", Default: []string{}},
		{Name: "max_rows", Label: "Max rows", Kind: "number", Default: 3000},
		{Name: "font_size", Label: "Font size (pt)", Kind: "number", Default: 8},
	}},
	{Type: "daily", Label: "Daily statistics", Help: "Per-day samples, min, max and average computed from the rows (observations).", Params: []ParamDef{
		{Name: "value_key", Label: "Value column", Kind: "text", Default: "value"},
		{Name: "time_key", Label: "Time column", Kind: "text", Default: "measured_at"},
		{Name: "decimals", Label: "Decimals", Kind: "number", Default: 2},
	}},
	{Type: "signatures", Label: "Signatures", Help: "Signature lines for review and approval (GxP).", Params: []ParamDef{
		{Name: "roles", Label: "Roles (one per line or comma separated)", Kind: "list", Default: []string{"Prepared by", "Reviewed by", "Approved by"}},
		{Name: "date", Label: "Date line", Kind: "bool", Default: true},
	}},
	{Type: "spacer", Label: "Space", Help: "Vertical space.", Params: []ParamDef{{Name: "height", Label: "Height (pt)", Kind: "number", Default: 8}}},
	{Type: "page_break", Label: "Page break", Help: "Continue on a new page."},
	{Type: "end", Label: "End marker", Help: "The “end of report” line."},
}

// BlockCatalog lists the block types and their parameters for the designer.
func BlockCatalog() []BlockDef { return blockCatalog }

func blockDef(t string) *BlockDef {
	for i := range blockCatalog {
		if blockCatalog[i].Type == t {
			return &blockCatalog[i]
		}
	}
	return nil
}

// DefaultSpec is the block representation of the built-in template of a kind.
func DefaultSpec(kind string) Spec {
	page := Page{Paper: "a4", Font: "Libertinus Serif", FontSize: 9.5}
	b := func(t string, p map[string]any) Block {
		if p == nil {
			p = map[string]any{}
		}
		return Block{Type: t, Params: p}
	}
	switch kind {
	case "alarms":
		return Spec{Version: SpecVersion, Page: page, Blocks: []Block{b("title", nil), b("summary", nil), b("table", nil), b("end", nil)}}
	case "audit":
		return Spec{Version: SpecVersion, Page: page, Blocks: []Block{b("title", nil),
			b("text", map[string]any{"text": "Each row carries the SHA-256 chain hash of the audit entry. Verify with ctrl32-telemetry audit verify or GET /api/v1/audit/verify.", "style": "note"}),
			b("table", nil), b("end", nil)}}
	case "observations":
		return Spec{Version: SpecVersion, Page: page, Blocks: []Block{b("title", nil), b("summary", nil), b("chart", nil), b("table", nil), b("end", nil)}}
	default:
		return Spec{Version: SpecVersion, Page: page, Blocks: []Block{b("title", nil), b("summary", nil), b("table", nil), b("end", nil)}}
	}
}

var (
	identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	tokenRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
)

// ParseSpec validates a spec and fills defaults; the returned spec is canonical.
func ParseSpec(raw json.RawMessage) (Spec, error) {
	var sp Spec
	if len(raw) == 0 {
		return sp, errors.New("spec required")
	}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return sp, fmt.Errorf("spec: %w", err)
	}
	if sp.Version == 0 {
		sp.Version = SpecVersion
	}
	if sp.Version != SpecVersion {
		return sp, fmt.Errorf("unsupported spec version %d", sp.Version)
	}
	switch sp.Page.Paper {
	case "":
		sp.Page.Paper = "a4"
	case "a4", "a3", "a5", "us-letter", "us-legal":
	default:
		return sp, errors.New("page.paper must be a4, a3, a5, us-letter or us-legal")
	}
	if sp.Page.Font == "" {
		sp.Page.Font = "Libertinus Serif"
	}
	if len(sp.Page.Font) > 64 || strings.ContainsAny(sp.Page.Font, "\"\\\n") {
		return sp, errors.New("page.font invalid")
	}
	if sp.Page.FontSize == 0 {
		sp.Page.FontSize = 9.5
	}
	if sp.Page.FontSize < 6 || sp.Page.FontSize > 16 {
		return sp, errors.New("page.font_size must be 6..16")
	}
	if len(sp.Blocks) == 0 {
		return sp, errors.New("at least one block is required")
	}
	if len(sp.Blocks) > 60 {
		return sp, errors.New("at most 60 blocks")
	}
	for i := range sp.Blocks {
		bl := &sp.Blocks[i]
		def := blockDef(bl.Type)
		if def == nil {
			return sp, fmt.Errorf("block %d: unknown type %q", i+1, bl.Type)
		}
		clean := map[string]any{}
		for _, p := range def.Params {
			v, ok := bl.Params[p.Name]
			if !ok || v == nil {
				v = p.Default
			}
			switch p.Kind {
			case "text", "multiline":
				s, ok := v.(string)
				if !ok {
					return sp, fmt.Errorf("block %d: %s must be text", i+1, p.Name)
				}
				if len(s) > 4000 {
					return sp, fmt.Errorf("block %d: %s too long", i+1, p.Name)
				}
				clean[p.Name] = s
			case "number":
				f, ok := toFloat(v)
				if !ok || f < 0 || f > 100000 {
					return sp, fmt.Errorf("block %d: %s must be a number", i+1, p.Name)
				}
				clean[p.Name] = f
			case "bool":
				b, ok := v.(bool)
				if !ok {
					return sp, fmt.Errorf("block %d: %s must be true or false", i+1, p.Name)
				}
				clean[p.Name] = b
			case "select":
				s, _ := v.(string)
				found := false
				for _, o := range p.Options {
					if o == s {
						found = true
					}
				}
				if !found {
					return sp, fmt.Errorf("block %d: %s must be one of %s", i+1, p.Name, strings.Join(p.Options, ", "))
				}
				clean[p.Name] = s
			case "list":
				list, err := toStringList(v)
				if err != nil || len(list) > 40 {
					return sp, fmt.Errorf("block %d: %s must be a list of up to 40 strings", i+1, p.Name)
				}
				if p.Name == "keys" || p.Name == "columns" {
					for _, k := range list {
						if !identRe.MatchString(k) {
							return sp, fmt.Errorf("block %d: %s contains an invalid key %q", i+1, p.Name, k)
						}
					}
				}
				for _, s := range list {
					if len(s) > 120 {
						return sp, fmt.Errorf("block %d: %s entry too long", i+1, p.Name)
					}
				}
				clean[p.Name] = list
			}
		}
		bl.Params = clean
	}
	return sp, nil
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func toStringList(v any) ([]string, error) {
	out := []string{}
	switch x := v.(type) {
	case []string:
		out = append(out, x...)
	case []any:
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, errors.New("not a string")
			}
			out = append(out, s)
		}
	case string:
		for _, part := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == '\n' }) {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
	default:
		return nil, errors.New("not a list")
	}
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out, nil
}

// ---- compile to Typst ------------------------------------------------------------------------------

func typstStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

// typstText turns "Hello {{org}}" into `"Hello " + tok(d, "org")`.
func typstText(s string) string {
	if s == "" {
		return `""`
	}
	var parts []string
	last := 0
	for _, m := range tokenRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			parts = append(parts, typstStr(s[last:m[0]]))
		}
		parts = append(parts, `tok(d, "`+s[m[2]:m[3]]+`")`)
		last = m[1]
	}
	if last < len(s) {
		parts = append(parts, typstStr(s[last:]))
	}
	return strings.Join(parts, " + ")
}

func typstList(list []string) string {
	if len(list) == 0 {
		return "()"
	}
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = typstStr(s)
	}
	if len(q) == 1 {
		return "(" + q[0] + ",)"
	}
	return "(" + strings.Join(q, ", ") + ")"
}

func num(v any) string {
	f, _ := toFloat(v)
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func list(v any) []string {
	l, _ := toStringList(v)
	return l
}

func boolStr(v any) string {
	if b, ok := v.(bool); ok && b {
		return "true"
	}
	return "false"
}

// Compile renders a validated spec to Typst source that uses layout.typ helpers only.
func Compile(sp Spec) string {
	var b strings.Builder
	b.WriteString("// Generated by the block designer (spec v" + strconv.Itoa(SpecVersion) + "). Edit the blocks in Settings → Report templates,\n// or switch to source mode — a hand-edited source no longer carries the blocks.\n")
	b.WriteString("#import \"layout.typ\": *\n#let d = load()\n")
	fmt.Fprintf(&b, "#show: page-setup.with(d, paper: %s, landscape: %s, font: %s, size: %spt)\n", typstStr(sp.Page.Paper), boolStr(sp.Page.Landscape), typstStr(sp.Page.Font), num(sp.Page.FontSize))
	for _, bl := range sp.Blocks {
		p := bl.Params
		switch bl.Type {
		case "title":
			fmt.Fprintf(&b, "#title-block(d, title: %s, subtitle: %s, show-period: %s, show-scope: %s)\n", typstText(str(p["title"])), typstText(str(p["subtitle"])), boolStr(p["show_period"]), boolStr(p["show_scope"]))
		case "summary":
			fmt.Fprintf(&b, "#summary-block(d, keys: %s)\n", typstList(list(p["keys"])))
		case "chart":
			if c := str(p["caption"]); c != "" {
				fmt.Fprintf(&b, "#chart-block(d, caption: %s)\n", typstText(c))
			} else {
				b.WriteString("#chart-block(d)\n")
			}
		case "text":
			fmt.Fprintf(&b, "#text-block(%s, style: %s)\n", typstText(str(p["text"])), typstStr(str(p["style"])))
		case "table":
			fmt.Fprintf(&b, "#data-table(d, max-rows: %s, columns: %s, font-size: %spt)\n", num(p["max_rows"]), typstList(list(p["columns"])), num(p["font_size"]))
		case "daily":
			fmt.Fprintf(&b, "#daily-table(d, value-key: %s, time-key: %s, decimals: %s)\n", typstStr(str(p["value_key"])), typstStr(str(p["time_key"])), num(p["decimals"]))
		case "signatures":
			fmt.Fprintf(&b, "#signatures-block(roles: %s, date: %s)\n", typstList(list(p["roles"])), boolStr(p["date"]))
		case "spacer":
			fmt.Fprintf(&b, "#v(%spt)\n", num(p["height"]))
		case "page_break":
			b.WriteString("#pagebreak()\n")
		case "end":
			b.WriteString("#end-marker()\n")
		}
	}
	return b.String()
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// SpecJSON returns the canonical JSON of a spec.
func SpecJSON(sp Spec) string {
	b, _ := json.Marshal(sp)
	return string(b)
}
