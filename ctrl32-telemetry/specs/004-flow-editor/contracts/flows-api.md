# Contract: flows API

- `GET /api/v1/flows/catalog` → node types `{type, group (trigger|logic|action|other), label, help, inputs, outputs,
  settings:[{name, label, kind, default, options, min, max, help}]}` (data.read)
- `GET /api/v1/flows` → list with state, version, node count, status summary (data.read)
- `POST /api/v1/flows` `{name, spec, reason}` → 201 draft (content.write)
- `GET /api/v1/flows/{slug}` → current version + status
- `PUT /api/v1/flows/{slug}` `{name?, spec, reason}` → new version (draft) (content.write)
- `POST /api/v1/flows/{slug}/deploy` `{reason}` → deployed; four-eyes change request when the flow sends commands and
  the policy is on (202 pending)
- `POST /api/v1/flows/{slug}/disable` `{reason}`
- `GET /api/v1/flows/{slug}/versions`, `POST /api/v1/flows/{slug}/restore` `{version, reason}`
- `DELETE /api/v1/flows/{slug}?reason=` → retire
- `POST /api/v1/flows/validate` `{spec}` → `{ok, problems:[{node, message}]}`
- `POST /api/v1/flows/{slug}/inject` `{node, message?, spec?, dry_run}` → runs a test message (edited spec optional)
- `GET /api/v1/flows/{slug}/debug?after=<seq>` → `[{seq, ts, node, kind: debug|error|action|dry_action, text, msg}]`
- `GET /api/v1/flows/{slug}/status` → per node `{in, out, errors, last}` and queue
- `GET /api/v1/flows/{slug}/export` → JSON; `POST /api/v1/flows/import` `{json, reason}`
- `GET /api/v1/rules/{id}/as-flow` → a flow spec equivalent to an automation rule
- `POST /api/v1/flow-hooks/{token}` (public) → webhook trigger
