package httpapi

// Ready-made overview dashboards (0.68.0), offered in the New dashboard dialog: they bind nothing fixed — their
// widgets select datastreams by query (source) and follow the dashboard filters, so they work for any organization
// and pick up new devices on their own.
//
//	sensors    Overview of all sensors: every datastream with latest value, age, quality and alarm state,
//	           filters by site, asset type, quantity, search and range; alarms and devices below
//	energy     Energy overview: electricity meters (quantity energy*) by site with the consumption in the range and
//	           totals per site, and the monthly consumption of each meter
//	coldchain  Cold chain overview: every temperature with the limits of its alarm rules, minimum and maximum in the
//	           range, alarm state; filters by site, asset type and storage class; a trend and the alarms

func overviewStarter(kind string) (string, map[string]any) {
	wd := func(id, typ string, x, y, w, h int, params map[string]any) map[string]any {
		return map[string]any{"id": id, "type": typ, "geometry": map[string]int{"x": x, "y": y, "w": w, "h": h}, "bindings": map[string][]string{}, "params": params, "follow": true}
	}
	src := func(w map[string]any, s map[string]any) map[string]any {
		w["source"] = s
		w["bindings"] = map[string][]string{"datastreams": {}}
		return w
	}
	spec := func(rng string, filters []map[string]any, widgets ...map[string]any) map[string]any {
		return map[string]any{"version": 1, "kind": "dashboard", "layout": map[string]any{"type": "grid", "columns": 12, "row_height": 90}, "range": rng, "refresh_s": 60, "filters": filters, "widgets": widgets}
	}
	devs := func(w map[string]any) map[string]any { w["bindings"] = map[string][]string{"devices": {}}; return w }
	switch kind {
	case "energy":
		meters := map[string]any{"quantity": []string{"energy*"}}
		return "Energy overview", spec("30d",
			[]map[string]any{{"kind": "site", "multi": true, "public": true}, {"kind": "search", "label": "Meter", "public": true}, {"kind": "range", "default": "30d", "public": true}},
			src(wd("meters", "overview_table", 0, 0, 12, 5, map[string]any{"title": "Meters by site — consumption in the range", "columns": []string{"asset", "device", "value", "change", "age", "alarm"}, "group_by": "site", "group_totals": true, "page_size": 50, "decimals": 1}), meters),
			src(wd("monthly", "period_table", 0, 5, 12, 7, map[string]any{"title": "Monthly consumption per meter", "interval": "month", "values": "delta", "period": "last_12m", "total": true, "decimals": 0}), map[string]any{"quantity": []string{"energy*"}, "limit": 24}),
		)
	case "coldchain":
		temps := map[string]any{"quantity": []string{"temperature"}}
		return "Cold chain overview", spec("24h",
			[]map[string]any{{"kind": "site", "multi": true, "public": true}, {"kind": "asset_type", "multi": true, "public": true}, {"kind": "attribute", "key": "storage_class", "label": "Storage class", "multi": true, "public": true}, {"kind": "search", "public": true}, {"kind": "range", "default": "24h", "public": true}},
			src(wd("temps", "overview_table", 0, 0, 12, 6, map[string]any{"title": "Temperatures with limits", "columns": []string{"site", "asset", "value", "limits", "min", "max", "age", "alarm"}, "group_by": "site", "highlight": "both", "page_size": 50, "decimals": 1}), temps),
			src(wd("trend", "chart", 0, 6, 8, 4, map[string]any{"title": "Temperatures", "legend_pos": "bottom", "decimals": 1}), map[string]any{"quantity": []string{"temperature"}, "limit": 12}),
			wd("alarms", "alarms", 8, 6, 4, 4, map[string]any{"title": "Alarms", "limit": 10}),
		)
	}
	return "Overview of all sensors", spec("24h",
		[]map[string]any{{"kind": "site", "multi": true, "public": true}, {"kind": "asset_type", "multi": true, "public": true}, {"kind": "quantity", "multi": true, "public": true}, {"kind": "search", "public": true}, {"kind": "range", "default": "24h", "public": true}},
		src(wd("sensors", "overview_table", 0, 0, 12, 7, map[string]any{"title": "All sensors", "columns": []string{"site", "asset", "device", "datastream", "quantity", "value", "age", "quality", "alarm"}, "page_size": 25}), map[string]any{}),
		wd("alarms", "alarms", 0, 7, 6, 3, map[string]any{"title": "Alarms", "limit": 10}),
		devs(wd("devices", "devices", 6, 7, 6, 3, map[string]any{"title": "Devices", "limit": 20})),
	)
}
