# API clients

Generated from the OpenAPI description of the server (`GET /api/v1/openapi.json`, rendered at `/api/docs`).
Regenerate after any API change with `go run ./tools/apigen`; the test in `tools/apigen` fails while the committed
files are stale.

## TypeScript — `typescript/ctrl32-api.ts`

One file, no dependencies, works in browsers, Node 18+, Deno and Bun (uses `fetch`).

```ts
import { Ctrl32Client, Ctrl32Error } from './ctrl32-api.ts';

const api = new Ctrl32Client({ baseUrl: 'https://telemetry.example.com', token: 'c32_…' });
const devices = await api.getDevices();
const cmd = await api.postDevicesIdCommands(devices[0].id, { method: 'write', args: { key: 'setpoint', value: 21 }, reason: 'night setback' });

// cursor pagination (X-Next-Cursor) over a long range
for await (const page of api.pages<Observation>('/api/v1/observations', { datastream_id: ds, from, to, limit: 5000 })) {
  console.log(page.length);
}

try { await api.getRules(); } catch (e) { if (e instanceof Ctrl32Error) console.log(e.status, e.message); }
```

- Methods are named after the operation ids (`getDevices`, `postDevicesIdCommands`, `putRulesId`, …); path parameters come first, then the request body, then a typed `query` object.
- With an API token the client sends `Authorization: Bearer`; without one it relies on the browser session cookie and needs `csrfToken` (from `getAuthMe()`) for mutating calls.
- Binary responses (CSV, PDF, SSE, firmware) return the `Response` object.

## Bruno — `bruno/`

Open the folder in [Bruno](https://www.usebruno.com/) (open-source API client, collections are plain files). Pick
the `local` or `production` environment and set `baseUrl` and `token`. Requests are grouped by area, carry example
bodies, path parameters (`:id`) and the documentation of each operation.
