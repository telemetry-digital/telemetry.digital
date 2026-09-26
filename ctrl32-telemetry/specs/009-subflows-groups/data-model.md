# Data model: Subflows, Split and groups

No migration. Flow spec (`core.flow.spec`):

| Field | Meaning |
|---|---|
| `kind` | "" (flow) or "subflow" |
| `groups[]` | `{id, name ≤ 64, color #rrggbb, x, y, w, h}`, at most 50 |
| node `subflow` | settings `flow` (slug), `version` (pinned), derived `outputs`, `commands` |
| node `sub_out` | setting `port` 1–4 |
| node `split` | settings `list` (expression), `separator`, `max` 1–1000 |
