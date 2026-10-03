// Headless Chrome: 0.72.0 loose ends of dashboards. Creates a cold room with a fridge, a datastream with an expected
// interval and two annotations, then checks: annotation markers with their text in the dashboard Chart widget
// (signed in), none on the public link until the owner enables them and then without the author; a stale latest
// value (computed from the expected interval) in the Values table and the overview table; asset types, quantities and
// qualities shown as words in the user's language (Slovak) in the filter bar and the overview table, while the URL
// keeps the codes. Screenshots chart-annotations*.png and overview-codes-sk.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9384;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-chart-annotations", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : JSON.stringify(got) === JSON.stringify(want); console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2500) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };
  const api = (m, p, b) => ev(`ctrl32.api(${JSON.stringify(m)}, ${JSON.stringify(p)}${b ? ", " + JSON.stringify(b) : ""}).then(r => r, e => ({error: e.message}))`);

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await ev(`localStorage.removeItem("ctrl32.locale"); true`);
  await go("/dashboards", 2000);
  // own data: a cold room with a fridge; temperature every 10 minutes for 3 hours, the last one 40 minutes ago
  const suf = String(Date.now() % 100000);
  const site = (await api("POST", "/api/v1/sites", { name: "Annotation store " + suf, reason: "headless probe" })).id;
  const dev = await api("POST", "/api/v1/devices", { external_id: "ann-" + suf, name: "Annotation logger " + suf, transport: "http", reason: "headless probe" });
  const tok = (await api("POST", `/api/v1/devices/${dev.id}/token`, { reason: "headless probe" })).token;
  const room = (await api("POST", "/api/v1/assets", { site_id: site, type: "cold_room", name: "Cold room " + suf, reason: "headless probe" })).id;
  const fr = (await api("POST", "/api/v1/assets", { site_id: site, type: "fridge", name: "Vaccine fridge " + suf, reason: "headless probe" })).id;
  const dsT = (await api("POST", "/api/v1/datastreams", { asset_id: fr, key: "temp", quantity: "temperature", unit: "°C", kind: "gauge", expected_interval: "10 minutes", reason: "headless probe" })).id;
  const dsH = (await api("POST", "/api/v1/datastreams", { asset_id: room, key: "rh", quantity: "humidity", unit: "%", kind: "gauge", reason: "headless probe" })).id;
  for (const d of [dsT, dsH]) await api("POST", `/api/v1/devices/${dev.id}/datastreams`, { datastream_id: d, reason: "headless probe" });
  const now = Math.floor(Date.now() / 1000); const samples = [];
  for (let i = 18; i >= 4; i--) samples.push({ ts: now - i * 600, v: { temp: +(4.2 + Math.sin(i / 2) * 0.8 + (i === 10 ? 2.4 : 0)).toFixed(2), rh: 60 + (i % 5) } });
  expect("telemetry sent", await ev(`fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: 'Bearer ${tok}'}, body: JSON.stringify(${JSON.stringify(samples)})}).then(r => r.status)`), 202);
  const iso = (t) => new Date(t * 1000).toISOString().replace(/\.\d+Z$/, "Z");
  await api("POST", "/api/v1/annotations", { datastream_id: dsT, from: iso(now - 6300), to: iso(now - 5700), category: "expected_event", text: "Door open for loading" });
  await api("POST", "/api/v1/annotations", { datastream_id: dsT, from: iso(now - 3600), category: "maintenance", text: "Probe checked" });
  const spec = { version: 1, kind: "dashboard", layout: { type: "grid", columns: 12, row_height: 90 }, range: "6h", refresh_s: 60,
    filters: [{ kind: "asset_type", multi: true, public: true }, { kind: "quantity", multi: true, public: true }],
    widgets: [
      { id: "trend", type: "chart", geometry: { x: 0, y: 0, w: 8, h: 4 }, bindings: { datastreams: [dsT] }, params: { title: "Vaccine fridge", range: "6h", legend_pos: "bottom" } },
      { id: "vals", type: "table", geometry: { x: 8, y: 0, w: 4, h: 4 }, bindings: { datastreams: [dsT, dsH] }, params: { title: "Latest values" } },
      { id: "ov", type: "overview_table", geometry: { x: 0, y: 4, w: 12, h: 4 }, source: { site: [site] }, follow: true, bindings: { datastreams: [] }, params: { title: "Overview", group_by: "asset_type", columns: ["asset", "datastream", "quantity", "value", "age", "quality"] } },
    ] };
  const dash = await api("POST", "/api/v1/dashboards", { name: "Annotations " + suf, spec, reason: "headless probe" });
  expect("dashboard created", typeof dash.slug, "string");
  const slug = dash.slug;

  // ---- signed in: markers and their text
  await ev("localStorage.clear(); true");
  await go("/dashboards/" + slug, 5000);
  expect("chart: two annotation markers", await until(`document.querySelectorAll('[data-id="trend"] svg .ann').length === 2`), true);
  expect("marker tooltip has the text", await ev(`[...document.querySelectorAll('[data-id="trend"] svg .ann title')].map(t => t.textContent).join(' | ')`), (s) => typeof s === "string" && s.includes("Door open for loading") && s.includes("Probe checked"));
  // hover over the period: the tip line names the annotation
  expect("hover shows the annotation", await ev(`(() => { const svg = document.querySelector('[data-id="trend"] .chart-box svg'); const r = svg.querySelector('rect.ann').getBoundingClientRect(); svg.dispatchEvent(new MouseEvent('mousemove', {clientX: r.left + r.width / 2, clientY: r.top + 20, bubbles: true})); return svg.parentElement.querySelector('.chart-tip').textContent; })()`), (s) => typeof s === "string" && s.includes("Door open for loading"));
  await shot("chart-annotations.png");
  expect("values table: the old temperature is stale", await ev(`[...document.querySelectorAll('[data-id="vals"] tbody tr')].map(r => r.lastElementChild.textContent).join(',')`), (s) => typeof s === "string" && s.startsWith("stale"));
  expect("overview: stale in quality", await until(`[...document.querySelectorAll('[data-id="ov"] tr[data-ds="${dsT}"] .badge')].some(b => b.textContent === 'stale')`), true);
  // chart setting off: no markers
  const noAnn = JSON.parse(JSON.stringify(spec)); noAnn.widgets[0].params.annotations = false;
  await api("PUT", `/api/v1/dashboards/${slug}`, { spec: noAnn, reason: "headless probe" });
  await go("/dashboards/" + slug, 4000);
  expect("chart setting off: no markers", await until(`!!document.querySelector('[data-id="trend"] svg') && document.querySelectorAll('[data-id="trend"] svg .ann').length === 0`), true);
  await api("PUT", `/api/v1/dashboards/${slug}`, { spec, reason: "headless probe" });

  // ---- public link: nothing until the owner enables it, then without the author
  const share = await api("POST", `/api/v1/dashboards/${slug}/share`, { reason: "headless probe" });
  await go("/p/" + share.token, 5000);
  expect("public link: chart drawn", await until(`!!document.querySelector('[data-id="trend"] svg path')`), true);
  expect("public link: no annotations by default", await ev(`document.querySelectorAll('[data-id="trend"] svg .ann').length`), 0);
  await go("/dashboards", 1500);
  await api("PUT", `/api/v1/dashboards/${slug}`, { spec: Object.assign({}, spec, { public_annotations: true }), reason: "headless probe" });
  await go("/p/" + share.token, 5000);
  expect("public link: markers once enabled", await until(`document.querySelectorAll('[data-id="trend"] svg .ann').length === 2`), true);
  expect("public link: text without author", await ev(`[...document.querySelectorAll('[data-id="trend"] svg .ann title')].map(t => t.textContent).join(' | ')`), (s) => typeof s === "string" && s.includes("Probe checked") && !s.includes("admin"));
  await shot("chart-annotations-public.png");

  // ---- editor: the setting in the dashboard settings dialog
  await go("/dashboards", 1500);
  await go(`/dashboards/${slug}/edit`, 5000);
  const opened = await ev(`(() => { const b = document.querySelector('#eSettings'); if (!b) return false; b.click(); return true; })()`);
  if (opened) { await sleep(800); expect("settings: public annotations checked", await ev(`document.querySelector('#setForm [name="public_annotations"]').checked`), true); await ev(`document.querySelector('#setDlg').close(); true`); }
  else console.log("skip editor check (no #eSettings on this page)");

  // ---- Slovak: asset types, quantities and qualities as words, codes stay in the URL
  const me = await ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); window.__csrf=me.csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  if (me && me.enabled && !me.enabled.includes("sk")) await ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify([...(me.enabled || []), "sk"])},default:${JSON.stringify(me.default)},reason:"headless probe"})}).then(r=>r.status)`);
  await ev(`localStorage.setItem("ctrl32.locale", "sk"); true`);
  await go(`/dashboards/${slug}?f.asset_type=fridge`, 5000);
  expect("filter bar: chosen asset type as a word", await ev(`[...document.querySelectorAll('#vFilters .df-pick summary b')].map(b => b.textContent).join('|')`), (s) => typeof s === "string" && s.startsWith("chladnička"));
  await ev(`document.querySelector('#vFilters .df-pick').open = true; true`); await sleep(300);
  expect("filter bar: choices as words", await ev(`[...document.querySelectorAll('#vFilters .df-pick')[0].querySelectorAll('label[data-txt]')].map(l => l.textContent.trim()).join('|')`), (s) => typeof s === "string" && /chladiareň/.test(s) && /chladnička/.test(s) && !/_/.test(s));
  expect("URL keeps the code", await ev(`new URLSearchParams(location.search).get('f.asset_type')`), "fridge");
  await ev(`document.querySelector('#vFilters .df-pick').open = false; true`);
  expect("overview: group and quantity as words", await until(`(() => { const t = document.querySelector('[data-id="ov"] table'); return !!t && t.textContent.includes('chladnička') && t.textContent.includes('teplota') && !t.textContent.includes('cold_room'); })()`), true);
  expect("overview: quality stale in Slovak", await ev(`[...document.querySelectorAll('[data-id="ov"] .badge')].map(b => b.textContent).join(',')`), (s) => typeof s === "string" && s.includes("neaktuálne"));
  await shot("overview-codes-sk.png");
  await ev(`localStorage.removeItem("ctrl32.locale"); true`);

  // clean up
  await go("/dashboards", 1500);
  await api("DELETE", `/api/v1/dashboards/${slug}/share?reason=headless%20probe`);
  expect("dashboard removed", await ev(`ctrl32.api('DELETE','/api/v1/dashboards/${slug}?reason=headless%20probe').then(() => true)`), true);
  console.log(problems.length ? "problems:\n" + problems.join("\n") : "problems: none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
