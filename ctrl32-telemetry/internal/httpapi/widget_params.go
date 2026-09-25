package httpapi

// Rich widget settings (spec 002): colours, icons, date-times, per-series settings, conditional-formatting ranges
// and multi-choice lists; the common appearance and action settings every widget type carries; and the warnings
// for settings a stored dashboard carries but the catalogue no longer knows (they are dropped, never kept silently).

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func fptr(f float64) *float64 { return &f }

var (
	seriesStyles = setOf("", "line", "area", "step", "bar", "scatter")
	seriesAxes   = setOf("", "left", "right")
	rangeOps     = setOf(">", ">=", "<", "<=", "==", "!=", "between")
	rangeTargets = setOf("value", "background", "icon")
)

// commonParams are appended to every widget definition: appearance (card, title, colours) and a click action.
var commonParams = []ScreenParam{
	{Name: "show_title", Label: "Show title", Kind: "bool", Default: true, Group: "appearance"},
	{Name: "title_icon", Label: "Title icon", Kind: "icon", Group: "appearance"},
	{Name: "title_size", Label: "Title size px", Kind: "number", Group: "appearance", Min: fptr(8), Max: fptr(48)},
	{Name: "value_size", Label: "Value size px", Kind: "number", Group: "appearance", Min: fptr(8), Max: fptr(120)},
	{Name: "text_color", Label: "Text colour", Kind: "color", Group: "appearance"},
	{Name: "bg_color", Label: "Background", Kind: "color", Group: "appearance"},
	{Name: "border_color", Label: "Border colour", Kind: "color", Group: "appearance"},
	{Name: "border_width", Label: "Border width px", Kind: "number", Group: "appearance", Min: fptr(0), Max: fptr(8)},
	{Name: "radius", Label: "Corner radius px", Kind: "number", Group: "appearance", Min: fptr(0), Max: fptr(40)},
	{Name: "padding", Label: "Padding px", Kind: "number", Group: "appearance", Min: fptr(0), Max: fptr(40)},
	{Name: "shadow", Label: "Shadow", Kind: "bool", Group: "appearance"},
	{Name: "transparent", Label: "Transparent card", Kind: "bool", Group: "appearance"},
	{Name: "tooltip", Label: "Tooltip", Kind: "text", Group: "appearance"},
	{Name: "fullscreen", Label: "Full-screen button", Kind: "bool", Default: true, Group: "appearance"},
	{Name: "click", Label: "On click", Kind: "select", Options: []string{"", "screen", "trend", "faceplate"}, Group: "actions"},
	{Name: "click_target", Label: "Screen (slug) for On click = screen", Kind: "text", Group: "actions"},
}

// timeParams are added to time-based widgets (trends and time charts).
var timeParams = []ScreenParam{
	{Name: "time_mode", Label: "Time window", Kind: "select", Options: []string{"relative", "fixed"}, Default: "relative", Group: "time"},
	{Name: "from", Label: "From (fixed window)", Kind: "datetime", Group: "time"},
	{Name: "to", Label: "To (fixed window)", Kind: "datetime", Group: "time"},
	{Name: "agg", Label: "Aggregation", Kind: "select", Options: []string{"auto", "none", "avg", "min", "max", "sum", "count"}, Default: "auto", Group: "time"},
	{Name: "agg_interval", Label: "Aggregation interval", Kind: "select", Options: []string{"auto", "1m", "5m", "15m", "1h", "6h", "1d"}, Default: "auto", Group: "time"},
	{Name: "legend_pos", Label: "Legend position", Kind: "select", Options: []string{"top", "bottom", "right", "none"}, Default: "top", Group: "series"},
	{Name: "legend_values", Label: "Legend values", Kind: "multi", Options: []string{"min", "max", "avg", "total", "latest"}, Group: "series"},
	{Name: "series", Label: "Series", Kind: "series", Group: "series"},
	{Name: "filters", Label: "Filters for viewers (shown on the widget)", Kind: "multi", Options: []string{"range", "agg", "interval", "series"}, Group: "time"},
}

// rangesParam is added to value-like widgets for conditional formatting.
var rangesParam = ScreenParam{Name: "ranges", Label: "Conditional formatting", Kind: "ranges", Group: "appearance"}

func textOf(v any, max int) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.New("must be text")
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		return "", fmt.Errorf("longer than %d characters", max)
	}
	return s, nil
}

func paramColor(v any) (string, error) {
	s, ok := v.(string)
	if !ok || !colorRe.MatchString(s) || s == "none" {
		return "", errors.New("must be a colour like #1f2937")
	}
	return strings.ToLower(s), nil
}

func numberOf(v any, lo, hi float64) (float64, error) {
	f, ok := numOf(v)
	if !ok || f < lo || f > hi {
		return 0, fmt.Errorf("must be a number %g..%g", lo, hi)
	}
	return f, nil
}

// validateRichParam validates the kinds introduced by spec 002.
func validateRichParam(d ScreenParam, v any) (any, error) {
	switch d.Kind {
	case "color":
		return paramColor(v)
	case "icon":
		s, _ := v.(string)
		if !tablerIcons[s] {
			return nil, errors.New("must be one of the icon names of the catalogue")
		}
		return s, nil
	case "datetime":
		s, _ := v.(string)
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return nil, errors.New("must be a date-time like 2026-09-25T08:00:00Z")
		}
		return t.UTC().Format(time.RFC3339), nil
	case "multi":
		list, ok := v.([]any)
		if !ok {
			return nil, errors.New("must be a list")
		}
		allowed := setOf(d.Options...)
		out := []string{}
		for _, x := range list {
			s, _ := x.(string)
			if !allowed[s] {
				return nil, fmt.Errorf("must contain only %s", strings.Join(d.Options, ", "))
			}
			out = append(out, s)
		}
		return out, nil
	case "series":
		list, ok := v.([]any)
		if !ok || len(list) > 24 {
			return nil, errors.New("must be a list of at most 24 series")
		}
		out := []map[string]any{}
		for _, x := range list {
			src, ok := x.(map[string]any)
			if !ok {
				return nil, errors.New("entries must be objects")
			}
			e := map[string]any{}
			for k, val := range src {
				if val == nil || val == "" {
					continue
				}
				var err error
				switch k {
				case "label":
					e[k], err = textOf(val, 64)
				case "unit":
					e[k], err = textOf(val, 16)
				case "color":
					e[k], err = paramColor(val)
				case "decimals":
					e[k], err = numberOf(val, 0, 6)
				case "style", "axis":
					s, _ := val.(string)
					if (k == "style" && !seriesStyles[s]) || (k == "axis" && !seriesAxes[s]) {
						err = fmt.Errorf("%s has an unknown value", k)
					}
					e[k] = s
				case "points", "fill", "hidden":
					b, ok := val.(bool)
					if !ok {
						err = fmt.Errorf("%s must be true or false", k)
					}
					if b {
						e[k] = true
					}
				default:
					err = fmt.Errorf("unknown series setting %q", k)
				}
				if err != nil {
					return nil, err
				}
			}
			out = append(out, e)
		}
		return out, nil
	case "ranges":
		list, ok := v.([]any)
		if !ok || len(list) > 8 {
			return nil, errors.New("must be a list of at most 8 ranges")
		}
		out := []map[string]any{}
		for _, x := range list {
			src, ok := x.(map[string]any)
			if !ok {
				return nil, errors.New("entries must be objects")
			}
			op, _ := src["op"].(string)
			if !rangeOps[op] {
				return nil, errors.New("op must be > >= < <= == != or between")
			}
			e := map[string]any{"op": op}
			var err error
			if e["v"], err = numberOf(src["v"], -1e12, 1e12); err != nil {
				return nil, errors.New("v " + err.Error())
			}
			if op == "between" {
				if e["v2"], err = numberOf(src["v2"], -1e12, 1e12); err != nil {
					return nil, errors.New("v2 " + err.Error())
				}
			}
			target, _ := src["target"].(string)
			if target == "" {
				target = "value"
			}
			if !rangeTargets[target] {
				return nil, errors.New("target must be value, background or icon")
			}
			e["target"] = target
			if e["color"], err = paramColor(src["color"]); err != nil {
				return nil, errors.New("color " + err.Error())
			}
			out = append(out, e)
		}
		return out, nil
	}
	return nil, errors.New("unknown kind")
}

var timeWidgets = setOf("chart", "bar_time", "state_timeline", "ts_table")
var valueWidgets = setOf("value", "gauge", "bars", "table", "value_card", "agg_card", "progress", "level_card", "thermometer", "radial_gauge", "linear_gauge", "entity_table", "battery")

// withCommon completes the catalogue once at start-up: every type gets the common settings, time-based types the
// time/series settings, value-like types conditional formatting. Parameters a type already defines win.
func withCommon(cat []WidgetDef) []WidgetDef {
	for i := range cat {
		have := map[string]bool{}
		for _, p := range cat[i].Params {
			have[p.Name] = true
		}
		add := func(ps ...ScreenParam) {
			for _, p := range ps {
				if !have[p.Name] {
					cat[i].Params = append(cat[i].Params, p)
					have[p.Name] = true
				}
			}
		}
		if timeWidgets[cat[i].Type] {
			add(timeParams...)
		}
		if valueWidgets[cat[i].Type] {
			add(rangesParam)
		}
		add(commonParams...)
	}
	return cat
}

func init() { widgetCatalog = withCommon(append(widgetCatalog, moreWidgets...)) }

// unknownParams lists the params of a widget that its definition does not know (reported as warnings).
func unknownParams(i int, def *WidgetDef, src map[string]any) []string {
	known := map[string]bool{}
	for _, p := range def.Params {
		known[p.Name] = true
	}
	var out []string
	for k := range src {
		if !known[k] {
			out = append(out, fmt.Sprintf("widget %d: unknown setting %s dropped", i+1, k))
		}
	}
	sort.Strings(out)
	return out
}
