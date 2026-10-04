package httpapi

// Entity aliases, key filters, dashboard states and widget actions (0.73.0, screen format 1.2 — additive: a dashboard
// without them keeps its stored form byte for byte, and every 1.1 dashboard is a valid 1.2 dashboard).
//
//   - spec.aliases: named sets of entities (devices, assets, datastreams, sites) that many widgets share. Kinds:
//     single (one entity), list (chosen entities), type (by device profile, asset type or quantity), name (starts with,
//     contains, ends with), hierarchy (everything under an asset or a site, N levels deep, optionally only assets of
//     some types), current (the entity of the dashboard state — drill-down and the entity selector) and query (the
//     query data source of 0.68.0). Resolved on the server within the organization (entities.go).
//   - spec.key_filters: named predicates on the latest values, attributes, quality, staleness and alarm state of
//     entities, combined with all/any and one level of groups; a value can be constant or dynamic (a value filter of
//     the dashboard, an attribute of the entity itself, of the current entity or of the organization).
//   - spec.states: views of the dashboard ("Overview" → "Device detail"); the first is the root. A widget belongs to
//     one state (widget.state, absent = the root).
//   - widget.alias {id, filter, quantity, key}: the widget takes its entities (or their datastreams or devices) from an
//     alias, optionally only those matching a key filter, narrowed to some quantities or keys.
//   - widget.actions: what a row click, a marker click, a chart click, a widget click or a button does — go to a state
//     with the clicked entity as the state's current entity, open the device page, or open another dashboard.

import (
	"errors"
	"fmt"
	"strings"
)

var (
	aliasKinds   = []string{"single", "list", "type", "name", "hierarchy", "current", "query"}
	entityTypes  = setOf("device", "asset", "datastream", "site")
	ruleSources  = setOf("value", "attribute", "quality", "stale", "alarm")
	numOps       = setOf("eq", "ne", "gt", "ge", "lt", "le", "between")
	strOps       = setOf("eq", "ne", "contains", "starts", "ends", "in")
	boolOps      = setOf("eq", "ne")
	dynFroms     = setOf("filter", "self", "current", "org")
	alarmValues  = setOf("any", "warning", "action", "critical")
	actionEvents = setOf("row", "row_button", "marker", "chart", "click", "button")
	actionKinds  = setOf("state", "device", "asset", "dashboard")
	passKinds    = setOf("self", "device", "asset")
	// control widgets send commands to one bound device: they never take entities from an alias
	noAliasGroups = setOf("control")
)

const (
	maxAliases    = 20
	maxKeyFilters = 20
	maxRules      = 12
	maxStates     = 10
	maxActions    = 6
	maxAliasIDs   = 100
	maxTreeDepth  = 10
)

// fieldsOnly refuses settings outside the allowed set.
func fieldsOnly(what string, m map[string]any, allowed ...string) error {
	ok := setOf(allowed...)
	for k := range m {
		if !ok[k] {
			return fmt.Errorf("%s: unknown setting %q", what, k)
		}
	}
	return nil
}

func nameOf(v any, what string, max int) (string, error) {
	if v == nil {
		return "", nil
	}
	s, err := textOf(v, max)
	if err != nil {
		return "", fmt.Errorf("%s %v", what, err)
	}
	return s, nil
}

// entityRefOf validates {type, id}.
func entityRefOf(v any, what string, types map[string]bool) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object {type, id}", what)
	}
	if err := fieldsOnly(what, m, "type", "id"); err != nil {
		return nil, err
	}
	t, _ := m["type"].(string)
	id, _ := m["id"].(string)
	if !types[t] {
		return nil, fmt.Errorf("%s: type must be one of %s", what, strings.Join(sortedKeys(types), ", "))
	}
	if !uuidRe.MatchString(id) {
		return nil, fmt.Errorf("%s: id must be an id", what)
	}
	return map[string]any{"type": t, "id": id}, nil
}

func entityTypeOf(v any, what string, types map[string]bool, def string) (string, error) {
	s, _ := v.(string)
	if v == nil || s == "" {
		if def == "" {
			return "", fmt.Errorf("%s: entity_type is required", what)
		}
		return def, nil
	}
	if !types[s] {
		return "", fmt.Errorf("%s: entity_type must be one of %s", what, strings.Join(sortedKeys(types), ", "))
	}
	return s, nil
}

// validateAlias checks one alias and returns its canonical form.
func validateAlias(i int, a map[string]any) (map[string]any, error) {
	what := fmt.Sprintf("alias %d", i+1)
	id, _ := a["id"].(string)
	if !widgetIDRe.MatchString(id) {
		return nil, fmt.Errorf("%s: id must be letters, digits, _ or - (at most 32)", what)
	}
	name, err := nameOf(a["name"], what+": name", 60)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = id
	}
	kind, _ := a["kind"].(string)
	out := map[string]any{"id": id, "name": name, "kind": kind}
	common := []string{"id", "name", "kind"}
	switch kind {
	case "single":
		if err := fieldsOnly(what, a, append(common, "entity")...); err != nil {
			return nil, err
		}
		ref, err := entityRefOf(a["entity"], what+": entity", entityTypes)
		if err != nil {
			return nil, err
		}
		out["entity"] = ref
	case "list":
		if err := fieldsOnly(what, a, append(common, "entity_type", "ids")...); err != nil {
			return nil, err
		}
		et, err := entityTypeOf(a["entity_type"], what, entityTypes, "")
		if err != nil {
			return nil, err
		}
		ids, err := uuidList(a["ids"], maxAliasIDs, what+": ids")
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("%s: choose at least one entity", what)
		}
		out["entity_type"], out["ids"] = et, ids
	case "type":
		if err := fieldsOnly(what, a, append(common, "entity_type", "types")...); err != nil {
			return nil, err
		}
		et, err := entityTypeOf(a["entity_type"], what, setOf("device", "asset", "datastream"), "")
		if err != nil {
			return nil, err
		}
		types, err := stringList(a["types"], maxFilterValues, what+": types")
		if err != nil {
			return nil, err
		}
		if len(types) == 0 {
			return nil, fmt.Errorf("%s: name at least one type", what)
		}
		out["entity_type"], out["types"] = et, types
	case "name":
		if err := fieldsOnly(what, a, append(common, "entity_type", "match", "text")...); err != nil {
			return nil, err
		}
		et, err := entityTypeOf(a["entity_type"], what, entityTypes, "")
		if err != nil {
			return nil, err
		}
		m, _ := a["match"].(string)
		if m == "" {
			m = "contains"
		}
		if m != "starts" && m != "contains" && m != "ends" {
			return nil, fmt.Errorf("%s: match must be starts, contains or ends", what)
		}
		txt, err := nameOf(a["text"], what+": text", 80)
		if err != nil {
			return nil, err
		}
		if txt == "" {
			return nil, fmt.Errorf("%s: the text to match is required", what)
		}
		out["entity_type"], out["match"], out["text"] = et, m, txt
	case "hierarchy":
		if err := fieldsOnly(what, a, append(common, "root", "root_current", "depth", "entity_type", "asset_type")...); err != nil {
			return nil, err
		}
		cur, _ := a["root_current"].(bool)
		if a["root_current"] != nil && a["root_current"] != true && a["root_current"] != false {
			return nil, fmt.Errorf("%s: root_current must be true or false", what)
		}
		switch {
		case cur && a["root"] != nil:
			return nil, fmt.Errorf("%s: either a root or the current entity, not both", what)
		case cur:
			out["root_current"] = true
		default:
			ref, err := entityRefOf(a["root"], what+": root", setOf("asset", "site"))
			if err != nil {
				return nil, err
			}
			out["root"] = ref
		}
		depth := maxTreeDepth
		if a["depth"] != nil {
			f, err := numberOf(a["depth"], 1, maxTreeDepth)
			if err != nil {
				return nil, fmt.Errorf("%s: depth %v", what, err)
			}
			depth = int(f)
		}
		out["depth"] = depth
		et, err := entityTypeOf(a["entity_type"], what, setOf("device", "asset", "datastream"), "datastream")
		if err != nil {
			return nil, err
		}
		out["entity_type"] = et
		types, err := stringList(a["asset_type"], maxFilterValues, what+": asset_type")
		if err != nil {
			return nil, err
		}
		if len(types) > 0 {
			out["asset_type"] = types
		}
	case "current":
		if err := fieldsOnly(what, a, append(common, "entity_type", "default")...); err != nil {
			return nil, err
		}
		if s, _ := a["entity_type"].(string); s != "" {
			if !entityTypes[s] {
				return nil, fmt.Errorf("%s: entity_type must be one of %s", what, strings.Join(sortedKeys(entityTypes), ", "))
			}
			out["entity_type"] = s
		}
		if a["default"] != nil {
			ref, err := entityRefOf(a["default"], what+": default", entityTypes)
			if err != nil {
				return nil, err
			}
			out["default"] = ref
		}
	case "query":
		if err := fieldsOnly(what, a, append(common, "source")...); err != nil {
			return nil, err
		}
		src, _ := a["source"].(map[string]any)
		if src == nil {
			src = map[string]any{}
		}
		c, err := validateSource(i, "overview_table", src)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", what, strings.TrimPrefix(err.Error(), fmt.Sprintf("widget %d: ", i+1)))
		}
		delete(c, "limit")
		out["source"] = c
	default:
		return nil, fmt.Errorf("%s: kind must be one of %s", what, strings.Join(aliasKinds, ", "))
	}
	return out, nil
}

// aliasEntityType is the kind of entity an alias yields ("" = whatever the current entity is).
func aliasEntityType(a map[string]any) string {
	switch a["kind"] {
	case "single":
		ref, _ := a["entity"].(map[string]any)
		t, _ := ref["type"].(string)
		return t
	case "query":
		return "datastream"
	}
	t, _ := a["entity_type"].(string)
	return t
}

// ---- key filters ---------------------------------------------------------------------------------------------------

func scalarOf(v any, typ, what string) (any, error) {
	switch typ {
	case "number":
		f, ok := numOf(v)
		if !ok || f < -1e12 || f > 1e12 {
			return nil, fmt.Errorf("%s must be a number", what)
		}
		return f, nil
	case "boolean":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s must be true or false", what)
		}
		return b, nil
	}
	s, err := textOf(v, 120)
	if err != nil {
		return nil, fmt.Errorf("%s %v", what, err)
	}
	return s, nil
}

// validateRule checks one predicate (or, with "rules", a group) of a key filter.
func validateRule(what string, r map[string]any, nested bool) (map[string]any, error) {
	if _, ok := r["rules"]; ok {
		if nested {
			return nil, fmt.Errorf("%s: groups cannot contain groups", what)
		}
		if err := fieldsOnly(what, r, "match", "rules"); err != nil {
			return nil, err
		}
		return validateRuleList(what, r["match"], r["rules"], true)
	}
	if err := fieldsOnly(what, r, "source", "key", "type", "op", "value", "value2", "dynamic"); err != nil {
		return nil, err
	}
	src, _ := r["source"].(string)
	if !ruleSources[src] {
		return nil, fmt.Errorf("%s: source must be value, attribute, quality, stale or alarm", what)
	}
	out := map[string]any{"source": src}
	key, _ := r["key"].(string)
	key = strings.TrimSpace(key)
	switch {
	case src == "attribute":
		if !attrKeyRe.MatchString(key) {
			return nil, fmt.Errorf("%s: an attribute needs its name (letters, digits, . _ -)", what)
		}
		out["key"] = key
	case src == "alarm":
		if key != "" {
			return nil, fmt.Errorf("%s: the alarm state takes no key", what)
		}
	case key != "":
		if !dsKeyRe.MatchString(key) {
			return nil, fmt.Errorf("%s: key must be a datastream key such as temperature", what)
		}
		out["key"] = key
	}
	op, _ := r["op"].(string)
	typ, _ := r["type"].(string)
	switch src {
	case "value", "attribute":
		if typ == "" {
			typ = "number"
		}
		ops := map[string]map[string]bool{"number": numOps, "string": strOps, "boolean": boolOps}[typ]
		if ops == nil {
			return nil, fmt.Errorf("%s: type must be number, string or boolean", what)
		}
		if !ops[op] {
			return nil, fmt.Errorf("%s: a %s comparison must be one of %s", what, typ, strings.Join(sortedKeys(ops), ", "))
		}
		out["type"], out["op"] = typ, op
		if d, ok := r["dynamic"]; ok && d != nil {
			m, ok := d.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: dynamic must be an object {from, key}", what)
			}
			if err := fieldsOnly(what+": dynamic", m, "from", "key"); err != nil {
				return nil, err
			}
			from, _ := m["from"].(string)
			dk, _ := m["key"].(string)
			if !dynFroms[from] {
				return nil, fmt.Errorf("%s: dynamic.from must be filter, self, current or org", what)
			}
			if !attrKeyRe.MatchString(dk) {
				return nil, fmt.Errorf("%s: dynamic.key must be a name of letters, digits, . _ -", what)
			}
			if op == "between" || op == "in" {
				return nil, fmt.Errorf("%s: %s takes constant values", what, op)
			}
			out["dynamic"] = map[string]any{"from": from, "key": dk}
		}
		switch {
		case op == "in":
			l, err := stringList(r["value"], maxFilterValues, what+": value")
			if err != nil {
				return nil, err
			}
			if len(l) == 0 {
				return nil, fmt.Errorf("%s: name at least one value", what)
			}
			out["value"] = l
		case r["value"] != nil && r["value"] != "":
			v, err := scalarOf(r["value"], typ, what+": value")
			if err != nil {
				return nil, err
			}
			out["value"] = v
		case out["dynamic"] == nil:
			return nil, fmt.Errorf("%s: a value (or a dynamic value) is required", what)
		}
		if op == "between" {
			v2, err := scalarOf(r["value2"], "number", what+": value2")
			if err != nil {
				return nil, err
			}
			out["value2"] = v2
		} else if r["value2"] != nil {
			return nil, fmt.Errorf("%s: value2 is only for between", what)
		}
	case "quality":
		if !boolOps[op] {
			return nil, fmt.Errorf("%s: quality compares with eq or ne", what)
		}
		v, err := textOf(r["value"], 40)
		if err != nil || v == "" {
			return nil, fmt.Errorf("%s: quality value must be good, doubtful or a quality code", what)
		}
		out["op"], out["value"] = op, v
	case "stale", "alarm":
		if op == "" {
			op = "eq"
		}
		if !boolOps[op] {
			return nil, fmt.Errorf("%s: %s compares with eq or ne", what, src)
		}
		out["op"] = op
		if src == "stale" {
			b := true
			if r["value"] != nil {
				x, ok := r["value"].(bool)
				if !ok {
					return nil, fmt.Errorf("%s: stale value must be true or false", what)
				}
				b = x
			}
			out["value"] = b
		} else {
			v, _ := r["value"].(string)
			if v == "" {
				v = "any"
			}
			if !alarmValues[v] {
				return nil, fmt.Errorf("%s: alarm value must be any, warning, action or critical", what)
			}
			out["value"] = v
		}
	}
	if (src == "quality" || src == "stale" || src == "alarm") && r["dynamic"] != nil {
		return nil, fmt.Errorf("%s: only values and attributes take a dynamic value", what)
	}
	return out, nil
}

func validateRuleList(what string, match any, rules any, nested bool) (map[string]any, error) {
	m, _ := match.(string)
	if m == "" {
		m = "all"
	}
	if m != "all" && m != "any" {
		return nil, fmt.Errorf("%s: match must be all or any", what)
	}
	list, ok := rules.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("%s: at least one condition", what)
	}
	if len(list) > maxRules {
		return nil, fmt.Errorf("%s: at most %d conditions", what, maxRules)
	}
	out := []map[string]any{}
	for j, x := range list {
		r, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: condition %d must be an object", what, j+1)
		}
		c, err := validateRule(fmt.Sprintf("%s, condition %d", what, j+1), r, nested)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return map[string]any{"match": m, "rules": out}, nil
}

// validateKeyFilter checks one key filter {id, name, match, rules}.
func validateKeyFilter(i int, f map[string]any) (map[string]any, error) {
	what := fmt.Sprintf("key filter %d", i+1)
	if err := fieldsOnly(what, f, "id", "name", "match", "rules"); err != nil {
		return nil, err
	}
	id, _ := f["id"].(string)
	if !widgetIDRe.MatchString(id) {
		return nil, fmt.Errorf("%s: id must be letters, digits, _ or - (at most 32)", what)
	}
	name, err := nameOf(f["name"], what+": name", 60)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = id
	}
	body, err := validateRuleList(what, f["match"], f["rules"], false)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "name": name, "match": body["match"], "rules": body["rules"]}, nil
}

// ---- states, widget alias bindings, actions -----------------------------------------------------------------------

func validateStates(raw []map[string]any) ([]map[string]any, error) {
	if len(raw) > maxStates {
		return nil, fmt.Errorf("states: at most %d", maxStates)
	}
	out := []map[string]any{}
	ids := map[string]bool{}
	for i, s := range raw {
		what := fmt.Sprintf("state %d", i+1)
		if err := fieldsOnly(what, s, "id", "name"); err != nil {
			return nil, err
		}
		id, _ := s["id"].(string)
		if !widgetIDRe.MatchString(id) || ids[id] {
			return nil, fmt.Errorf("%s: id must be unique letters, digits, _ or -", what)
		}
		ids[id] = true
		name, err := nameOf(s["name"], what+": name", 60)
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = id
		}
		out = append(out, map[string]any{"id": id, "name": name})
	}
	return out, nil
}

// validateWidgetAlias checks widget.alias {id, filter, quantity, key}.
func validateWidgetAlias(i int, def *WidgetDef, raw map[string]any, aliases, filters map[string]bool) (map[string]any, error) {
	what := fmt.Sprintf("widget %d: alias", i+1)
	if noAliasGroups[def.Group] {
		return nil, fmt.Errorf("widget %d: a control widget sends commands to its own device and takes no alias", i+1)
	}
	if !def.Entities && len(def.Binds) == 0 && def.Type != "alarms" && def.Type != "alarm_count" {
		return nil, fmt.Errorf("widget %d: %s shows no entities", i+1, def.Type)
	}
	if err := fieldsOnly(what, raw, "id", "filter", "quantity", "key"); err != nil {
		return nil, err
	}
	id, _ := raw["id"].(string)
	if !aliases[id] {
		return nil, fmt.Errorf("%s: the dashboard has no alias %q", what, id)
	}
	out := map[string]any{"id": id}
	if f, _ := raw["filter"].(string); f != "" {
		if !filters[f] {
			return nil, fmt.Errorf("%s: the dashboard has no key filter %q", what, f)
		}
		out["filter"] = f
	}
	q, err := stringList(raw["quantity"], maxFilterValues, what+": quantity")
	if err != nil {
		return nil, err
	}
	if len(q) > 0 {
		out["quantity"] = q
	}
	k, err := stringList(raw["key"], maxFilterValues, what+": key")
	if err != nil {
		return nil, err
	}
	for _, x := range k {
		if !dsKeyRe.MatchString(x) {
			return nil, fmt.Errorf("%s: key %q is not a datastream key", what, x)
		}
	}
	if len(k) > 0 {
		out["key"] = k
	}
	return out, nil
}

func validateActions(i int, raw []map[string]any, states map[string]bool) ([]map[string]any, error) {
	if len(raw) > maxActions {
		return nil, fmt.Errorf("widget %d: at most %d actions", i+1, maxActions)
	}
	out := []map[string]any{}
	for j, a := range raw {
		what := fmt.Sprintf("widget %d, action %d", i+1, j+1)
		if err := fieldsOnly(what, a, "event", "action", "target", "pass", "label"); err != nil {
			return nil, err
		}
		ev, _ := a["event"].(string)
		if !actionEvents[ev] {
			return nil, fmt.Errorf("%s: event must be one of %s", what, strings.Join(sortedKeys(actionEvents), ", "))
		}
		act, _ := a["action"].(string)
		if !actionKinds[act] {
			return nil, fmt.Errorf("%s: action must be state, device, asset or dashboard", what)
		}
		e := map[string]any{"event": ev, "action": act}
		tgt, _ := a["target"].(string)
		switch act {
		case "state":
			if !states[tgt] {
				return nil, fmt.Errorf("%s: the dashboard has no state %q", what, tgt)
			}
			e["target"] = tgt
		case "dashboard":
			if !dashSlugRe.MatchString(tgt) {
				return nil, fmt.Errorf("%s: target must be the slug of a dashboard", what)
			}
			e["target"] = tgt
		default:
			if tgt != "" {
				return nil, fmt.Errorf("%s: opening the device or asset page takes no target", what)
			}
		}
		if p, _ := a["pass"].(string); p != "" && p != "self" {
			if !passKinds[p] {
				return nil, fmt.Errorf("%s: pass must be self, device or asset", what)
			}
			e["pass"] = p
		}
		lbl, err := nameOf(a["label"], what+": label", 40)
		if err != nil {
			return nil, err
		}
		if lbl != "" {
			e["label"] = lbl
		} else if ev == "button" || ev == "row_button" {
			return nil, fmt.Errorf("%s: a button needs a label", what)
		}
		out = append(out, e)
	}
	return out, nil
}

// specIDs returns the ids of a list of objects in a stored or validated spec (aliases, key_filters, states).
func specList(spec map[string]any, key string) []map[string]any {
	var out []map[string]any
	switch xs := spec[key].(type) {
	case []map[string]any:
		out = xs
	case []any:
		for _, x := range xs {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func specByID(spec map[string]any, key, id string) map[string]any {
	if id == "" {
		return nil
	}
	for _, m := range specList(spec, key) {
		if m["id"] == id {
			return m
		}
	}
	return nil
}

// validateDashModel checks the aliases, key filters and states of a spec; it returns their canonical forms and the
// sets of ids the widgets may refer to.
func validateDashModel(rawAliases, rawFilters, rawStates []map[string]any) (aliases, filters, states []map[string]any, aIDs, fIDs, sIDs map[string]bool, err error) {
	aIDs, fIDs, sIDs = map[string]bool{}, map[string]bool{}, map[string]bool{}
	if len(rawAliases) > maxAliases {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("aliases: at most %d", maxAliases)
	}
	for i, a := range rawAliases {
		c, err := validateAlias(i, a)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if aIDs[c["id"].(string)] {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("alias %d: the id is used twice", i+1)
		}
		aIDs[c["id"].(string)] = true
		aliases = append(aliases, c)
	}
	if len(rawFilters) > maxKeyFilters {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("key_filters: at most %d", maxKeyFilters)
	}
	for i, f := range rawFilters {
		c, err := validateKeyFilter(i, f)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		if fIDs[c["id"].(string)] {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("key filter %d: the id is used twice", i+1)
		}
		fIDs[c["id"].(string)] = true
		filters = append(filters, c)
	}
	if states, err = validateStates(rawStates); err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	for _, s := range states {
		sIDs[s["id"].(string)] = true
	}
	return aliases, filters, states, aIDs, fIDs, sIDs, nil
}

var errNoStates = errors.New("states and drill-down need the grid layout (dashboards, not process pictures)")
