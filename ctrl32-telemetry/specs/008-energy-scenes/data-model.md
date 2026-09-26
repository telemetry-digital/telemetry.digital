# Data model: Energy overview and scenes

`core.scene` (migration 0026): id, org_id, name (1–80), icon (≤ 40), room_id (optional asset), actions jsonb
`[{entity_id, service, params}]` (1–50), sort, created_at/by, updated_at, removed_at (soft delete).

`core.organization.settings.energy`: `price_per_kwh`, `feed_in_per_kwh` (0–100), `currency` (≤ 4), `sources`
`[{datastream_id, role: consumption|production|off}]`, `exclude_others`.
