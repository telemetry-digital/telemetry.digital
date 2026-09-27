# Data Model: flows (migration 0023)

## core.flow — versioned content
| column | type | notes |
|---|---|---|
| id | uuid PK | one row per version |
| org_id | uuid | |
| slug | text | identity across versions |
| name | text | 1–128 |
| version | int | 1, 2, … |
| state | text | `draft` · `deployed` · `disabled` |
| spec | jsonb | `{nodes:[{id,type,x,y,name,settings}], wires:[{from,port,to}]}` validated |
| checksum | text | SHA-256 of the canonical spec |
| created_by, created_at, reason | | |
| valid_to | timestamptz | set when superseded |

Unique current version per (org, slug).

## core.flow_state — node memory
`(flow_slug, org_id, node_id) → state jsonb, updated_at`.

## core.flow_hook — incoming webhook tokens
`id, org_id, flow_slug, node_id, token_hash, secret (encrypted, HMAC), created_at`.

## In memory
Ring buffer of debug entries per flow (500), counters per node, queue length.
