# Contract: drawing layer of screen format v1

Extends the screen document stored by `POST/PUT /api/v1/dashboards` (unchanged routes). Full field rules in
[data-model.md](../data-model.md). The server returns the canonical document; unknown keys in `style` are an error,
unknown top-level element keys are dropped.

```json
{
  "version": 1, "kind": "mimic",
  "layout": {"type": "canvas", "width": 1600, "height": 900, "bg": "#dfe3e8", "grid": 10},
  "scheme": "classic", "range": "24h", "refresh_s": 30,
  "widgets": [],
  "elements": [
    {"id": "t101", "type": "symbol", "symbol": "tank_vertical", "x": 200, "y": 150, "w": 140, "h": 260,
     "name": "T-101 Mixer", "tag": "<datastream uuid>", "unit": "%", "decimals": 1,
     "style": {"shade": true, "medium": "#0891b2"},
     "dyn": [{"prop": "level", "lin": [0, 100, 0, 1]},
             {"prop": "blink", "op": ">", "v": 90, "color": "#dc2626"}],
     "on": [{"act": "faceplate"}]},
    {"id": "p1", "type": "pipe", "points": [[340, 380], [520, 380], [520, 470]],
     "style": {"sw": 14, "medium": "#0891b2"},
     "dyn": [{"prop": "flow", "tag": "<flow uuid>", "op": ">", "v": 0.5, "speed": 1.5}]},
    {"id": "lbl", "type": "text", "x": 200, "y": 420, "w": 140, "h": 26,
     "text": "{value} {unit}", "tag": "<datastream uuid>",
     "style": {"font": 16, "bold": true, "align": "middle", "bg": "#ffffff", "stroke": "#334155"}},
    {"id": "logo", "type": "image", "src": "customer-logo", "x": 20, "y": 20, "w": 160, "h": 60}
  ]
}
```

Errors (400) name the element: `element 3: style.fill must be a colour like #1f2937 or none`.

Public links: the tags of elements and dynamics are readable through the link; images referenced by `src` are served
through the public SVG route; actions `screen` and `command` are disabled.
