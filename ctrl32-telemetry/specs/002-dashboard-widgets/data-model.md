# Data Model: Dashboard widgets with rich settings

## Widget setting definition (catalogue, extended)

| Field | Type | Notes |
|---|---|---|
| name | string | param key in `widget.params` |
| label | string | editor label (translated) |
| kind | text, multiline, number, bool, select, list, **color**, **icon**, **series**, **ranges**, **datetime** | |
| group | data, appearance, series, time, actions | editor tab |
| default, options, help | | as today |

## Common appearance params (every widget type)

| name | kind | default | rule |
|---|---|---|---|
| show_title | bool | true | |
| title_icon | icon | "" | Tabler name from the vendored list |
| title_size | number | 14 | 8–48 |
| value_size | number | — | 8–120 (value-like widgets) |
| text_color | color | "" | hex |
| bg_color | color | "" | hex |
| border_color | color | "" | hex |
| border_width | number | 1 | 0–8 |
| radius | number | — | 0–40 |
| padding | number | — | 0–40 |
| shadow | bool | false | |
| transparent | bool | false | |
| tooltip | text | "" | ≤ 200 |
| fullscreen | bool | true | header icon |
| click | select | "" | "", screen, trend, faceplate |
| click_target | text | "" | screen slug when click = screen |

## Series settings (`series` param: list aligned with `bindings.datastreams`)

`{label ≤ 64, color hex, unit ≤ 16, decimals 0–6, style line|area|step|bar|scatter, axis left|right, points bool,
fill bool, hidden bool}` — at most 24 entries.

## Time settings (time-based widgets)

`time_mode` relative|fixed · `range` (existing) · `from`, `to` datetime (fixed) · `agg` none|avg|min|max|sum|count ·
`agg_interval` auto|1m|5m|15m|1h|6h|1d · `legend_pos` top|bottom|right|none · `legend_values` list of
min|max|avg|total|latest.

## Conditional formatting (`ranges` param)

`[{op > >= < <= == != between, v, v2 (between), target value|background|icon, color hex}]` — at most 8, first match wins.

## New widget types (24)

value_card, agg_card, progress, level_card, thermometer, battery, signal, compass, radial_gauge, linear_gauge,
led_grid, state_timeline, bar_time, pie, polar, radar, ts_table, entity_table, alarm_count, clock, markdown, image,
qr, nav_card, knob, slider, round_switch (27 listed; ≥ 24 required).
