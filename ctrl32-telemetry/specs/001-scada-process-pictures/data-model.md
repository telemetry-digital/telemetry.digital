# Data Model: SCADA process pictures and editor

## Screen (existing `core.dashboard`, spec document extended)

| Field | Type | Rules |
|---|---|---|
| `kind` | `dashboard` \| `mimic` | mimic ⇒ canvas layout |
| `layout` | `{type:"canvas", width, height, bg?, grid?}` | width 600–4000, height 300–3000, bg hex colour, grid 1–100 |
| `scheme` | `classic` \| `isa101` | optional, default classic |
| `widgets` | list (existing) | ≤ 60 |
| `elements` | list of Element | ≤ 1500; only with canvas layout; widgets **or** elements must be non-empty |

Versioning: unchanged (new row per save, `valid_to`, reason, checksum, audit `dashboard.update` with widget and
element counts).

## Element

| Field | Type | Rules |
|---|---|---|
| `id` | string | `^[A-Za-z0-9_-]{1,32}$`, unique in the picture (default `e<n>`) |
| `type` | enum | rect, ellipse, line, polyline, polygon, pipe, text, symbol, image |
| `x,y,w,h` | number | within ±2× canvas size; w,h ≥ 0 |
| `r` | number | −360…360, rotation around the centre |
| `points` | `[[x,y],…]` | line/polyline/polygon/pipe only; 2–200 points (polygon ≥ 3) |
| `name` | text ≤ 64 | shown in tooltip/faceplate |
| `text` | text ≤ 500 | placeholders `{value}` `{unit}` `{name}` `{time}`; label for instruments/buttons |
| `unit` | text ≤ 16 | overrides the datastream unit |
| `decimals` | 0–6 | value formatting |
| `symbol` | `^[a-z0-9_]{1,40}$` | symbol elements; unknown ids render as placeholder |
| `src` | library slug | image elements |
| `group` | id-like string | elements with the same group move/select together |
| `tag` | datastream uuid | primary binding |
| `device`, `key` | uuid, `^[A-Za-z0-9_.:/-]{1,64}$` | faceplate controls |
| `locked`, `hidden` | bool | editor lock; hidden in operation |
| `style` | object | fill, stroke, color, medium, bg (hex/none); sw 0–100; opacity 0–1; radius 0–1000; font 4–400; bold, italic, shade, shadow; dash `''|dash|dot|dashdot`; align `start|middle|end`; valign `top|middle|bottom` |
| `dyn` | list ≤ 8 of Dynamic | |
| `on` | list ≤ 4 of Action | first one runs on click |

## Dynamic

| `prop` | Form | Result |
|---|---|---|
| fill, stroke, color | `map:[{op,v,to:colour}]` (≤ 16) | first match colours the part |
| state | `map:[{op,v,to:state}]` | run, stop, fault, manual, open, closed, transit, off, warn |
| text | `map:[{op,v,to:text}]` optional | mapped text or formatted value |
| visible, blink, flow, rotate | `op, v` (+ `color` for blink, `speed` 0.05–20 for flow/rotate) | condition true ⇒ effect |
| level, opacity, angle, move_x, move_y | `lin:[in0,in1,out0,out1]`, in0 ≠ in1 | clamped linear mapping |

`tag` optional (uuid) — otherwise the element's tag. Operators: `> >= < <= == !=`.

## Action

| `act` | Fields |
|---|---|
| faceplate | — |
| trend | — |
| screen | `target` screen slug |
| command | `key`, `method` (default write), `value` (number, text ≤ 64, bool), `confirm` |

## HMI object (new `core.hmi_object`)

| Column | Type | Rules |
|---|---|---|
| id | uuid pk | uuidv7 |
| org_id | uuid | organization |
| slug | text | `^[a-z0-9][a-z0-9-]{0,62}$`, identity across versions |
| kind | text | `svg` \| `composite` |
| name | text | ≤ 128 |
| category | text | ≤ 64 (palette group) |
| version | int | 1… |
| body | jsonb | svg: `{svg, width, height}`; composite: `{elements:[…], tags:[placeholder names], width, height}` |
| checksum | text | SHA-256 of canonical body |
| created_by, created_at, valid_to, reason | | ADR-0010 pattern |

State: current = row without `valid_to`; retire = set `valid_to` (reason). Audit actions `hmi_object.create`,
`hmi_object.update`, `hmi_object.retire`.
