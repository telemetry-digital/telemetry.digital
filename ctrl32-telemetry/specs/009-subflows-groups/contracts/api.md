# API contract

| Method | Path | Change |
|---|---|---|
| GET | /api/v1/flows, /api/v1/flows/{slug} | `kind` (flow, subflow), `outputs` (subflows) |
| POST/PUT | /api/v1/flows[/{slug}] | spec `kind`, `groups`; subflow nodes pinned on save |
| POST | /api/v1/flows/{slug}/deploy | 400 for subflows; 422 when a subflow cannot be expanded |
| GET | /api/v1/flows/catalog | nodes `split`, `subflow`, `sub_in`, `sub_out` (group "subflow") |
