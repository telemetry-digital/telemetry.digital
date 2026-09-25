# Contract: widget settings in screen format v1

Unchanged routes: `GET /api/v1/dashboards/catalog` (now with `group` and the new kinds per parameter) and
`POST/PUT /api/v1/dashboards` (params validated against the catalogue). Example widget:

```json
{"id": "t1", "type": "chart", "geometry": {"x": 0, "y": 0, "w": 8, "h": 4},
 "bindings": {"datastreams": ["<temp>", "<rh>"]},
 "params": {"title": "Room climate", "title_icon": "temperature", "bg_color": "#0f172a", "text_color": "#f8fafc",
   "border_width": 0, "radius": 12, "shadow": true,
   "series": [{"label": "Temperature", "color": "#ef4444", "unit": "°C", "decimals": 1, "axis": "left"},
              {"label": "Humidity", "color": "#3b82f6", "unit": "%", "axis": "right", "style": "area"}],
   "time_mode": "relative", "range": "7d", "agg": "max", "agg_interval": "1h",
   "legend_pos": "bottom", "legend_values": ["min", "max", "avg", "latest"],
   "click": "trend"}}
```

Errors (400) name the widget and parameter: `widget 3: bg_color must be a colour like #1f2937`.
Unknown parameter names are dropped and listed in `warnings: ["widget 3: unknown setting foo"]` of the save
response (FR-006); invalid values of known settings are refused.
