package httpapi

// Dashboard filters and dynamic data sources (0.68.0, screen format 1.1 — additive, older dashboards keep their
// stored form):
//
//   - spec.filters: up to 8 filters shown as a bar on top of the dashboard — site, asset type, device, quantity, an
//     attribute (tag) of assets or datastreams, free-text search and the time range. Each has an optional label, a
//     default, "multi" (several values) and "public" (a viewer of a public link may change it; otherwise the default
//     applies there and cannot be changed).
//   - widget.source: the widget's datastreams are selected by a query instead of a fixed list — sites, asset types,
//     quantities, devices, attributes and a name pattern — so new devices appear on their own. Resolved on the server
//     within the organization and limited (500 rows for tables, 24 for other widgets).
//   - widget.follow: the widget follows the dashboard filters (its datastreams, devices or alarms are narrowed by the
//     selected values). Filters only ever narrow what a widget shows, never widen it.

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	filterKinds = []string{"site", "asset_type", "device", "quantity", "attribute", "search", "range"}
	attrKeyRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)
	// widgets that list many datastreams in one request may resolve up to 500 of them; the others 24 (one request each)
	manyRowWidgets = setOf("table", "entity_table", "overview_table")
)

const (
	maxDashFilters  = 8
	maxSourceRows   = 500
	maxSourceSeries = 24
	maxFilterValues = 50
)

// sourceLimit is the most datastreams a widget of the type may resolve from a query.
func sourceLimit(typ string) int {
	if manyRowWidgets[typ] {
		return maxSourceRows
	}
	return maxSourceSeries
}

// filterID is the name of a filter in URLs (f.<id>=…): the kind, "q" for search, "a.<key>" for an attribute.
func filterID(kind, key string) string {
	switch kind {
	case "search":
		return "q"
	case "attribute":
		return "a." + key
	}
	return kind
}

func stringList(v any, max int, what string) ([]string, error) {
	var in []any
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(x) == "" {
			return nil, nil
		}
		in = []any{x}
	case []any:
		in = x
	case []string:
		for _, s := range x {
			in = append(in, s)
		}
	default:
		return nil, fmt.Errorf("%s must be a text or a list of texts", what)
	}
	if len(in) > max {
		return nil, fmt.Errorf("%s: at most %d values", what, max)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be texts", what)
		}
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		if len(s) > 120 {
			return nil, fmt.Errorf("%s: a value is longer than 120 characters", what)
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}

func uuidList(v any, max int, what string) ([]string, error) {
	list, err := stringList(v, max, what)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		if !uuidRe.MatchString(s) {
			return nil, fmt.Errorf("%s must be ids", what)
		}
	}
	return list, nil
}

// validateFilters checks spec.filters and returns the canonical list.
func validateFilters(raw []map[string]any) ([]map[string]any, error) {
	if len(raw) > maxDashFilters {
		return nil, fmt.Errorf("filters: at most %d", maxDashFilters)
	}
	out := []map[string]any{}
	ids := map[string]bool{}
	for i, f := range raw {
		kind, _ := f["kind"].(string)
		known := false
		for _, k := range filterKinds {
			known = known || k == kind
		}
		if !known {
			return nil, fmt.Errorf("filter %d: kind must be one of %s", i+1, strings.Join(filterKinds, ", "))
		}
		for k := range f {
			switch k {
			case "kind", "label", "key", "default", "multi", "public":
			default:
				return nil, fmt.Errorf("filter %d: unknown setting %q", i+1, k)
			}
		}
		e := map[string]any{"kind": kind}
		key, _ := f["key"].(string)
		key = strings.TrimSpace(key)
		if kind == "attribute" {
			if !attrKeyRe.MatchString(key) {
				return nil, fmt.Errorf("filter %d: an attribute filter needs a key of letters, digits, . _ -", i+1)
			}
			e["key"] = key
		} else if key != "" {
			return nil, fmt.Errorf("filter %d: only an attribute filter has a key", i+1)
		}
		id := filterID(kind, key)
		if ids[id] {
			return nil, fmt.Errorf("filter %d: the dashboard has this filter already", i+1)
		}
		ids[id] = true
		if v, ok := f["label"]; ok && v != nil && v != "" {
			s, err := textOf(v, 40)
			if err != nil {
				return nil, fmt.Errorf("filter %d: label %v", i+1, err)
			}
			if s != "" {
				e["label"] = s
			}
		}
		for _, b := range []string{"multi", "public"} {
			if v, ok := f[b]; ok && v != nil {
				x, ok := v.(bool)
				if !ok {
					return nil, fmt.Errorf("filter %d: %s must be true or false", i+1, b)
				}
				if x {
					e[b] = true
				}
			}
		}
		if e["multi"] == true && (kind == "search" || kind == "range") {
			return nil, fmt.Errorf("filter %d: %s takes one value", i+1, kind)
		}
		if d, ok := f["default"]; ok && d != nil && d != "" {
			switch kind {
			case "search":
				s, err := textOf(d, 80)
				if err != nil {
					return nil, fmt.Errorf("filter %d: default %v", i+1, err)
				}
				if s != "" {
					e["default"] = s
				}
			case "range":
				s, _ := d.(string)
				if !ranges[s] {
					return nil, fmt.Errorf("filter %d: the default range must be one of the dashboard ranges", i+1)
				}
				e["default"] = s
			default:
				var list []string
				var err error
				if kind == "site" || kind == "device" {
					list, err = uuidList(d, maxFilterValues, fmt.Sprintf("filter %d: default", i+1))
				} else {
					list, err = stringList(d, maxFilterValues, fmt.Sprintf("filter %d: default", i+1))
				}
				if err != nil {
					return nil, err
				}
				if len(list) > 1 && e["multi"] != true {
					return nil, fmt.Errorf("filter %d: several default values need multi", i+1)
				}
				if len(list) > 0 {
					e["default"] = list
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// validateSource checks widget.source and returns the canonical query.
func validateSource(i int, typ string, src map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range src {
		var err error
		switch k {
		case "site", "device":
			var l []string
			if l, err = uuidList(v, maxFilterValues, k); err == nil && len(l) > 0 {
				out[k] = l
			}
		case "asset_type", "quantity":
			var l []string
			if l, err = stringList(v, maxFilterValues, k); err == nil && len(l) > 0 {
				out[k] = l
			}
		case "attr":
			m, ok := v.(map[string]any)
			if !ok {
				err = errors.New("attr must be an object of attribute = value")
				break
			}
			if len(m) > 8 {
				err = errors.New("attr: at most 8 attributes")
				break
			}
			a := map[string]any{}
			for ak, av := range m {
				if !attrKeyRe.MatchString(ak) {
					err = fmt.Errorf("attr: %q is not a valid attribute name", ak)
					break
				}
				var l []string
				if l, err = stringList(av, maxFilterValues, "attr "+ak); err != nil {
					break
				}
				if len(l) > 0 {
					a[ak] = l
				}
			}
			if err == nil && len(a) > 0 {
				out[k] = a
			}
		case "name":
			var s string
			if s, err = textOf(v, 80); err == nil && s != "" {
				out[k] = s
			}
		case "limit":
			var f float64
			if f, err = numberOf(v, 1, float64(sourceLimit(typ))); err == nil {
				out[k] = int(f)
			}
		default:
			err = fmt.Errorf("unknown setting %q", k)
		}
		if err != nil {
			return nil, fmt.Errorf("widget %d: source %v", i+1, err)
		}
	}
	if _, ok := out["limit"]; !ok {
		out["limit"] = sourceLimit(typ)
		if !manyRowWidgets[typ] {
			out["limit"] = 12
		}
	}
	return out, nil
}

// specFilters returns the filter definitions of a stored or validated spec.
func specFilters(spec map[string]any) []map[string]any {
	var out []map[string]any
	switch fs := spec["filters"].(type) {
	case []map[string]any:
		out = fs
	case []any:
		for _, x := range fs {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// sortedKeys returns the keys of a map in order (stable SQL and output).
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
