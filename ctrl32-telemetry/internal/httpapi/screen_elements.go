package httpapi

// Drawing elements of process pictures (kind "mimic"): the vector layer a SCADA editor draws — shapes, pipes, text and
// industrial symbols — with dynamics (properties driven by datastream values) and click events. static/scada.js
// renders them, static/scada-editor.js edits them. The server keeps the document small and inert: every key is
// whitelisted, numbers are bounded, colours are hex, text is plain (the renderer escapes it) and ids are checked,
// so a stored picture can never carry markup, script or a reference outside the organization's own objects.

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

const (
	maxElements      = 1500
	maxElementPoints = 200
	maxDynamics      = 8
	maxEvents        = 4
	maxMapEntries    = 16
)

var (
	colorRe     = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{4}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8}|none)$`)
	symbolIDRe  = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)
	cmdKeyRe    = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,64}$`)
	cmdMethodRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

	elementTypes = map[string]bool{"rect": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "pipe": true, "text": true, "symbol": true, "image": true}
	pointTypes   = map[string]bool{"line": true, "polyline": true, "polygon": true, "pipe": true}
	dynProps     = map[string]string{ // property → kind of the mapped value
		"fill": "color", "stroke": "color", "color": "color", "state": "state", "text": "text",
		"visible": "cond", "blink": "cond", "flow": "cond", "rotate": "cond", "animate": "cond",
		"level": "lin", "opacity": "lin", "angle": "lin", "move_x": "lin", "move_y": "lin",
	}
	dynOps     = map[string]bool{">": true, ">=": true, "<": true, "<=": true, "==": true, "!=": true}
	symStates  = map[string]bool{"run": true, "stop": true, "fault": true, "manual": true, "open": true, "closed": true, "transit": true, "off": true, "warn": true}
	eventActs  = map[string]bool{"faceplate": true, "screen": true, "command": true, "trend": true}
	dashOpts   = map[string]bool{"": true, "dash": true, "dot": true, "dashdot": true}
	alignOpts  = map[string]bool{"start": true, "middle": true, "end": true}
	valignOpts = map[string]bool{"top": true, "middle": true, "bottom": true}
)

// elemErr prefixes an error with the element's position (1-based) for the editor.
func elemErr(i int, f string, a ...any) error {
	return fmt.Errorf("element %d: %s", i+1, fmt.Sprintf(f, a...))
}

func numOf(v any) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func boundedNum(i int, name string, v any, lo, hi float64) (float64, error) {
	f, ok := numOf(v)
	if !ok {
		return 0, elemErr(i, "%s must be a number", name)
	}
	if f < lo || f > hi {
		return 0, elemErr(i, "%s must be %g..%g", name, lo, hi)
	}
	return math.Round(f*100) / 100, nil
}

func plainText(i int, name string, v any, max int) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", elemErr(i, "%s must be text", name)
	}
	if len(s) > max {
		return "", elemErr(i, "%s is longer than %d characters", name, max)
	}
	return s, nil
}

func colorOf(i int, name string, v any) (string, error) {
	s, ok := v.(string)
	if !ok || !colorRe.MatchString(s) {
		return "", elemErr(i, "%s must be a colour like #1f2937 or none", name)
	}
	return strings.ToLower(s), nil
}

// validateElements normalizes the drawing layer. Datastream and device ids are returned for the reference check the
// caller performs against the organization (the same rule as widget bindings).
func validateElements(raw []any, width, height int) ([]map[string]any, error) {
	if len(raw) > maxElements {
		return nil, fmt.Errorf("at most %d drawing elements", maxElements)
	}
	lim := float64(max(width, height)) * 2
	ids := map[string]bool{}
	out := make([]map[string]any, 0, len(raw))
	for i, x := range raw {
		src, ok := x.(map[string]any)
		if !ok {
			return nil, elemErr(i, "must be an object")
		}
		typ, _ := src["type"].(string)
		if !elementTypes[typ] {
			return nil, elemErr(i, "unknown type %q", typ)
		}
		id, _ := src["id"].(string)
		if id == "" {
			id = fmt.Sprintf("e%d", i+1)
		}
		if !widgetIDRe.MatchString(id) || ids[id] {
			return nil, elemErr(i, "id must be unique and simple")
		}
		ids[id] = true
		e := map[string]any{"id": id, "type": typ}
		for _, k := range []string{"x", "y"} {
			v, err := boundedNum(i, k, orZero(src[k]), -lim, lim)
			if err != nil {
				return nil, err
			}
			e[k] = v
		}
		for _, k := range []string{"w", "h"} {
			v, err := boundedNum(i, k, orZero(src[k]), 0, lim)
			if err != nil {
				return nil, err
			}
			e[k] = v
		}
		if v, ok := src["r"]; ok && v != nil {
			r, err := boundedNum(i, "r", v, -360, 360)
			if err != nil {
				return nil, err
			}
			if r != 0 {
				e["r"] = r
			}
		}
		if pointTypes[typ] {
			pts, err := elementPoints(i, src["points"], lim)
			if err != nil {
				return nil, err
			}
			if len(pts) < 2 || (typ == "polygon" && len(pts) < 3) {
				return nil, elemErr(i, "%s needs at least %d points", typ, map[bool]int{true: 3, false: 2}[typ == "polygon"])
			}
			e["points"] = pts
		}
		if err := copyStrings(i, src, e, map[string]int{"name": 64, "text": 500, "unit": 16}); err != nil {
			return nil, err
		}
		if typ == "symbol" {
			s, _ := src["symbol"].(string)
			if !symbolIDRe.MatchString(s) {
				return nil, elemErr(i, "symbol must be a symbol id")
			}
			e["symbol"] = s
		}
		if typ == "image" {
			src, _ := src["src"].(string)
			if !dashSlugRe.MatchString(src) {
				return nil, elemErr(i, "src must be the slug of a library object")
			}
			e["src"] = src
		}
		if g, ok := src["group"]; ok && g != nil && g != "" {
			gs, _ := g.(string)
			if !widgetIDRe.MatchString(gs) {
				return nil, elemErr(i, "group must be letters, digits, _ and -")
			}
			e["group"] = gs
		}
		for _, k := range []string{"tag", "device"} {
			if v, ok := src[k]; ok && v != nil && v != "" {
				s, _ := v.(string)
				if !uuidRe.MatchString(s) {
					return nil, elemErr(i, "%s must be an id", k)
				}
				e[k] = s
			}
		}
		if v, ok := src["key"]; ok && v != nil && v != "" {
			s, _ := v.(string)
			if !cmdKeyRe.MatchString(s) {
				return nil, elemErr(i, "key must be letters, digits and _ . : / -")
			}
			e["key"] = s
		}
		if v, ok := src["decimals"]; ok && v != nil {
			d, err := boundedNum(i, "decimals", v, 0, 6)
			if err != nil {
				return nil, err
			}
			e["decimals"] = math.Round(d)
		}
		for _, k := range []string{"locked", "hidden"} {
			if b, ok := src[k].(bool); ok && b {
				e[k] = true
			}
		}
		if v, ok := src["style"]; ok && v != nil {
			st, err := elementStyle(i, v)
			if err != nil {
				return nil, err
			}
			if len(st) > 0 {
				e["style"] = st
			}
		}
		if v, ok := src["dyn"]; ok && v != nil {
			d, err := elementDynamics(i, v)
			if err != nil {
				return nil, err
			}
			if len(d) > 0 {
				e["dyn"] = d
			}
		}
		if v, ok := src["on"]; ok && v != nil {
			ev, err := elementEvents(i, v)
			if err != nil {
				return nil, err
			}
			if len(ev) > 0 {
				e["on"] = ev
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func orZero(v any) any {
	if v == nil {
		return 0.0
	}
	return v
}

func copyStrings(i int, src, dst map[string]any, keys map[string]int) error {
	for k, maxLen := range keys {
		v, ok := src[k]
		if !ok || v == nil || v == "" {
			continue
		}
		s, err := plainText(i, k, v, maxLen)
		if err != nil {
			return err
		}
		dst[k] = s
	}
	return nil
}

func elementPoints(i int, v any, lim float64) ([][2]float64, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, elemErr(i, "points must be a list of [x, y]")
	}
	if len(list) > maxElementPoints {
		return nil, elemErr(i, "at most %d points", maxElementPoints)
	}
	pts := make([][2]float64, 0, len(list))
	for _, p := range list {
		xy, ok := p.([]any)
		if !ok || len(xy) != 2 {
			return nil, elemErr(i, "points must be a list of [x, y]")
		}
		var q [2]float64
		for k := range 2 {
			f, err := boundedNum(i, "point", xy[k], -lim, lim)
			if err != nil {
				return nil, err
			}
			q[k] = f
		}
		pts = append(pts, q)
	}
	return pts, nil
}

func elementStyle(i int, v any) (map[string]any, error) {
	src, ok := v.(map[string]any)
	if !ok {
		return nil, elemErr(i, "style must be an object")
	}
	st := map[string]any{}
	for k, x := range src {
		if x == nil || x == "" {
			continue
		}
		var err error
		switch k {
		case "fill", "stroke", "color", "medium", "bg":
			st[k], err = colorOf(i, "style."+k, x)
		case "sw":
			st[k], err = boundedNum(i, "style.sw", x, 0, 100)
		case "opacity":
			st[k], err = boundedNum(i, "style.opacity", x, 0, 1)
		case "radius":
			st[k], err = boundedNum(i, "style.radius", x, 0, 1000)
		case "font":
			st[k], err = boundedNum(i, "style.font", x, 4, 400)
		case "bold", "italic", "shade", "shadow":
			b, ok := x.(bool)
			if !ok {
				return nil, elemErr(i, "style.%s must be true or false", k)
			}
			if b {
				st[k] = true
			}
		case "dash", "align", "valign":
			s, _ := x.(string)
			opts := map[string]map[string]bool{"dash": dashOpts, "align": alignOpts, "valign": valignOpts}[k]
			if !opts[s] {
				return nil, elemErr(i, "style.%s has an unknown value", k)
			}
			st[k] = s
		default:
			return nil, elemErr(i, "unknown style %q", k)
		}
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func dynCond(i int, src, dst map[string]any) error {
	op, _ := src["op"].(string)
	if !dynOps[op] {
		return elemErr(i, "dynamic condition needs an operator (> >= < <= == !=)")
	}
	v, err := boundedNum(i, "dynamic value", src["v"], -1e12, 1e12)
	if err != nil {
		return err
	}
	dst["op"], dst["v"] = op, v
	return nil
}

func elementDynamics(i int, v any) ([]map[string]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, elemErr(i, "dyn must be a list")
	}
	if len(list) > maxDynamics {
		return nil, elemErr(i, "at most %d dynamics", maxDynamics)
	}
	out := []map[string]any{}
	for _, x := range list {
		src, ok := x.(map[string]any)
		if !ok {
			return nil, elemErr(i, "dynamic must be an object")
		}
		prop, _ := src["prop"].(string)
		kind, ok := dynProps[prop]
		if !ok {
			return nil, elemErr(i, "unknown dynamic property %q", prop)
		}
		d := map[string]any{"prop": prop}
		if t, ok := src["tag"]; ok && t != nil && t != "" {
			s, _ := t.(string)
			if !uuidRe.MatchString(s) {
				return nil, elemErr(i, "dynamic tag must be an id")
			}
			d["tag"] = s
		}
		switch kind {
		case "cond":
			if err := dynCond(i, src, d); err != nil {
				return nil, err
			}
			if c, ok := src["color"]; ok && c != nil && c != "" {
				col, err := colorOf(i, "dynamic colour", c)
				if err != nil {
					return nil, err
				}
				d["color"] = col
			}
			if s, ok := src["speed"]; ok && s != nil {
				f, err := boundedNum(i, "dynamic speed", s, 0.05, 20)
				if err != nil {
					return nil, err
				}
				d["speed"] = f
			}
		case "lin":
			lin, ok := src["lin"].([]any)
			if !ok || len(lin) != 4 {
				return nil, elemErr(i, "dynamic %s needs lin [in0, in1, out0, out1]", prop)
			}
			vals := make([]float64, 4)
			for k := range 4 {
				f, err := boundedNum(i, "lin", lin[k], -1e12, 1e12)
				if err != nil {
					return nil, err
				}
				vals[k] = f
			}
			if vals[0] == vals[1] {
				return nil, elemErr(i, "dynamic %s: input range must not be empty", prop)
			}
			d["lin"] = vals
		default: // color, state, text: an ordered mapping, first match wins
			m, ok := src["map"].([]any)
			if !ok && kind != "text" {
				return nil, elemErr(i, "dynamic %s needs a mapping", prop)
			}
			if len(m) > maxMapEntries {
				return nil, elemErr(i, "at most %d mapping entries", maxMapEntries)
			}
			entries := []map[string]any{}
			for _, y := range m {
				ms, ok := y.(map[string]any)
				if !ok {
					return nil, elemErr(i, "mapping entry must be an object")
				}
				me := map[string]any{}
				if err := dynCond(i, ms, me); err != nil {
					return nil, err
				}
				var to string
				var err error
				switch kind {
				case "color":
					to, err = colorOf(i, "mapping colour", ms["to"])
				case "state":
					to, _ = ms["to"].(string)
					if !symStates[to] {
						err = elemErr(i, "unknown state %q", to)
					}
				default:
					to, err = plainText(i, "mapping text", ms["to"], 64)
				}
				if err != nil {
					return nil, err
				}
				me["to"] = to
				entries = append(entries, me)
			}
			if len(entries) > 0 {
				d["map"] = entries
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func elementEvents(i int, v any) ([]map[string]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, elemErr(i, "on must be a list")
	}
	if len(list) > maxEvents {
		return nil, elemErr(i, "at most %d events", maxEvents)
	}
	out := []map[string]any{}
	for _, x := range list {
		src, ok := x.(map[string]any)
		if !ok {
			return nil, elemErr(i, "event must be an object")
		}
		act, _ := src["act"].(string)
		if !eventActs[act] {
			return nil, elemErr(i, "unknown event action %q", act)
		}
		ev := map[string]any{"act": act}
		switch act {
		case "screen":
			t, _ := src["target"].(string)
			if !dashSlugRe.MatchString(t) {
				return nil, elemErr(i, "screen event needs the target screen's slug")
			}
			ev["target"] = t
		case "command":
			k, _ := src["key"].(string)
			if !cmdKeyRe.MatchString(k) {
				return nil, elemErr(i, "command event needs a key")
			}
			ev["key"] = k
			m, _ := src["method"].(string)
			if m == "" {
				m = "write"
			}
			if !cmdMethodRe.MatchString(m) {
				return nil, elemErr(i, "command method must be a simple name")
			}
			ev["method"] = m
			switch val := src["value"].(type) {
			case bool:
				ev["value"] = val
			case float64:
				f, err := boundedNum(i, "command value", val, -1e12, 1e12)
				if err != nil {
					return nil, err
				}
				ev["value"] = f
			case string:
				if len(val) > 64 {
					return nil, elemErr(i, "command value too long")
				}
				ev["value"] = val
			case nil:
			default:
				return nil, elemErr(i, "command value must be a number, text or true/false")
			}
		}
		if b, ok := src["confirm"].(bool); ok {
			ev["confirm"] = b
		}
		out = append(out, ev)
	}
	return out, nil
}

// elementsOf returns the drawing layer of a validated spec (none for grid dashboards).
func elementsOf(spec map[string]any) []map[string]any {
	e, _ := spec["elements"].([]map[string]any)
	return e
}
