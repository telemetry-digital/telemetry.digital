// Headless Chrome: overviews (0.68.0). Creates a site with fridges (and the "Overview of all sensors" and "Cold chain
// overview" starters), then checks: the dashboard filter bar with choices from the server, the chosen values in the
// URL and remembered per viewer, widgets that follow the filters, the overview table (search, sorting, column filters,
// grouping, pages, highlighting, export), the editor (data source by query, filters in the dashboard settings), a
// public link with only the public filters, and the global search (Ctrl+K, arrows, Enter). Screenshots overview-*.png
// go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9381;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-overviews", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : JSON.stringify(got) === JSON.stringify(want); console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2500) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };
  const key = (k, mods = 0, code) => send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code: code || k, modifiers: mods, windowsVirtualKeyCode: k === "Enter" ? 13 : k === "ArrowDown" ? 40 : k.length === 1 ? k.toUpperCase().charCodeAt(0) : 0 }).then(() => send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code: code || k, modifiers: mods }));
  const api = (m, p, b) => ev(`ctrl32.api(${JSON.stringify(m)}, ${JSON.stringify(p)}${b ? ", " + JSON.stringify(b) : ""}).then(r => r, e => ({error: e.message}))`);

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await go("/dashboards", 2000);
  // own data: a site with three fridges (one with an alarm-rule limit and a value above it), telemetry over the device API
  const suf = String(Date.now() % 100000);
  const site = (await api("POST", "/api/v1/sites", { name: "Probe store " + suf, reason: "headless probe" })).id;
  const dev = await api("POST", "/api/v1/devices", { external_id: "probe-" + suf, name: "Probe logger " + suf, transport: "http", reason: "headless probe" });
  const tok = (await api("POST", `/api/v1/devices/${dev.id}/token`, { reason: "headless probe" })).token;
  const ds = [];
  for (let i = 1; i <= 7; i++) {
    const a = (await api("POST", "/api/v1/assets", { site_id: site, type: "fridge", name: `Probe fridge ${i} ${suf}`, attributes: { storage_class: i % 2 ? "2-8C" : "15-25C" }, reason: "headless probe" })).id;
    const d = (await api("POST", "/api/v1/datastreams", { asset_id: a, key: "t" + i, quantity: "temperature", unit: "°C", kind: "gauge", reason: "headless probe" })).id;
    await api("POST", `/api/v1/devices/${dev.id}/datastreams`, { datastream_id: d, reason: "headless probe" }); ds.push(d);
  }
  await api("POST", "/api/v1/alarm-rules", { datastream_id: ds[0], type: "high", warning: 7.5, action: 8, reason: "headless probe" });
  const v = {}; for (let i = 1; i <= 7; i++) v["t" + i] = i === 1 ? 9.2 : 3 + i * 0.4;
  expect("telemetry sent", await ev(`fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: 'Bearer ${tok}'}, body: JSON.stringify({ts: Math.floor(Date.now() / 1000), v: ${JSON.stringify(v)}})}).then(r => r.status)`), 202);
  // the cold chain overview, starting on this site; its table pages after 5 rows
  const st = await api("GET", "/api/v1/dashboards/starter?kind=coldchain");
  for (const f of st.spec.filters) if (f.kind === "site") f.default = [site];
  st.spec.widgets.find((w) => w.id === "temps").params.page_size = 5;
  const slug = (await api("POST", "/api/v1/dashboards", { name: "Probe cold chain " + suf, spec: st.spec, reason: "headless probe" })).slug;
  expect("overview dashboard created", typeof slug, "string");

  // ---- view: filter bar and overview table
  await ev("localStorage.clear(); true");
  await go("/dashboards/" + slug, 5000);
  expect("filter bar: site, asset type, storage class, search, range", await ev(`[document.querySelectorAll('#vFilters .df-pick').length, !!document.querySelector('#vFilters .df-q'), !!document.querySelector('#vFilters .df-range')]`), [3, true, true]);
  expect("the Range select is replaced by the range filter", await ev(`document.querySelector('#vRangeL').hidden`), true);
  expect("site filter starts on its default", await ev(`document.querySelector('#vFilters .df-pick').classList.contains('set')`), true);
  expect("table rows of the site (first page of 5)", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 5`), true);
  expect("pages", await ev(`document.querySelector('[data-id="temps"] .ot-pages span').textContent`), "1 / 2");
  expect("rows grouped by site, site column hidden", await ev(`[!!document.querySelector('[data-id="temps"] tr.ot-group'), [...document.querySelectorAll('[data-id="temps"] th')].some(t => t.dataset.k === 'site')]`), [true, false]);
  expect("value above the limit highlighted", await ev(`(() => { const r = document.querySelector('[data-id="temps"] tr[data-ds="${ds[0]}"]'); return r ? r.querySelector('td:nth-child(2)').style.color : 'no row'; })()`), (c) => /bad|220, 38, 38/.test(c));
  expect("limits column", await ev(`document.querySelector('[data-id="temps"] tr[data-ds="${ds[0]}"]').textContent.includes('≤ 8')`), true);
  await shot("overview-view.png");
  // sort by value (descending first), search, column filters
  await ev(`document.querySelector('[data-id="temps"] th[data-k="value"]').click()`); await sleep(300);
  expect("sorted by value, highest first", await ev(`document.querySelector('[data-id="temps"] tr[data-ds]').dataset.ds`), ds[0]);
  await ev(`(() => { const q = document.querySelector('[data-id="temps"] .ot-q'); q.value = 'fridge 3'; q.dispatchEvent(new Event('input')); })()`); await sleep(300);
  expect("search in the table", await ev(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length`), 1);
  await ev(`(() => { const q = document.querySelector('[data-id="temps"] .ot-q'); q.value = ''; q.dispatchEvent(new Event('input')); document.querySelector('[data-id="temps"] .ot-ftoggle').click(); })()`); await sleep(300);
  await ev(`(() => { const s = document.querySelector('[data-id="temps"] .ot-filters [data-f="vmin"]'); s.value = '5'; s.dispatchEvent(new Event('change')); })()`); await sleep(300);
  expect("column filter: value from 5", await ev(`[...document.querySelectorAll('[data-id="temps"] tr[data-ds]')].length`), 4);
  await ev(`(() => { const s = document.querySelector('[data-id="temps"] .ot-filters [data-f="alarm"]'); s.value = 'none'; s.dispatchEvent(new Event('change')); })()`); await sleep(300);
  expect("column filter: no alarm", await ev(`document.querySelector('[data-id="temps"] .ot-count').textContent.trim().split(' ')[0]`), (n) => +n >= 1);
  await shot("overview-column-filters.png");
  await ev(`document.querySelector('[data-id="temps"] [data-freset]').click()`); await sleep(300);
  expect("export buttons", await ev(`[...document.querySelectorAll('[data-id="temps"] [data-x]')].map(b => b.textContent).join()`), "Excel,CSV");
  const exp = await ev(`ctrl32.api('POST', '/api/v1/datastreams/export', {ids: ${JSON.stringify(ds)}, columns: ['asset', 'value', 'limits'], format: 'csv'}).then(async b => (await b.text()).split('\\n').length)`);
  expect("export of the rows", exp, (n) => n >= 8);
  // filters: storage class narrows the table and lands in the URL; a reload keeps it; the values are remembered
  await ev(`(() => { const d = [...document.querySelectorAll('#vFilters .df-pick')][2]; d.open = true; return true; })()`); await sleep(300);
  await ev(`(() => { const i = [...document.querySelectorAll('#vFilters .df-pick')][2].querySelector('input[value="15-25C"]'); i.checked = true; i.dispatchEvent(new Event('change')); })()`);
  expect("filter narrows the table", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 3`), true);
  expect("filter value in the URL", await ev(`location.search`), (s) => s.includes("f.a.storage_class=15-25C"));
  expect("clear filters offered", await ev(`!!document.querySelector('#vFilters [data-reset]')`), true);
  await shot("overview-filtered.png");
  const shared = await ev(`location.pathname + location.search`);
  await go(shared, 5000);
  expect("a link shows the same view", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 3`), true);
  await go("/dashboards/" + slug, 5000);
  expect("remembered for this viewer", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 3`), true);
  await ev(`document.querySelector('#vFilters [data-reset]').click()`);
  expect("cleared back to the defaults", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 5 && !location.search.includes('f.')`), true);
  expect("the alarms widget follows the filters", await ev(`!!document.querySelector('[data-id="alarms"] table')`), true);

  // ---- editor: data source and filters
  await go("/dashboards/" + slug + "/edit", 5000);
  expect("editor filter bar", await ev(`!document.querySelector('#eFilters').hidden`), true);
  await ev(`document.querySelector('#eGrid .dash-widget[data-id="trend"]').click()`); await sleep(800);
  expect("query source shown", await ev(`[document.querySelector('[data-srcmode][value="query"]').checked, !!document.querySelector('select[data-src="quantity"]'), document.querySelector('[data-follow]').checked]`), [true, true, true]);
  expect("matches preview", await until(`/\\d/.test((document.querySelector('.src-prev') || {}).textContent || '')`), true);
  await shot("overview-editor-source.png");
  await ev(`document.querySelector('#eSettings').click()`); await sleep(800);
  expect("filters in the dashboard settings", await ev(`document.querySelectorAll('#fltList [data-fi]').length`), 5);
  await shot("overview-editor-filters.png");
  await ev(`document.querySelector('#setDlg').close(); true`);

  // ---- public link: only public filters; a non-public filter keeps its default
  st.spec.filters.find((f) => f.kind === "asset_type").public = false;
  await api("PUT", `/api/v1/dashboards/${slug}`, { spec: st.spec, reason: "headless probe" });
  const share = await api("POST", `/api/v1/dashboards/${slug}/share`, { reason: "headless probe" });
  await go("/p/" + share.token + "?f.a.storage_class=2-8C", 5000);
  expect("public bar without the non-public filter", await ev(`document.querySelectorAll('#pFilters .df-pick').length`), 2);
  expect("public table filtered by the link", await until(`document.querySelectorAll('[data-id="temps"] tr[data-ds]').length === 4`), true);
  expect("no export on a public link", await ev(`document.querySelectorAll('[data-id="temps"] [data-x]').length`), 0);
  await shot("overview-public.png");

  // ---- global search: Ctrl+K, type, arrows, Enter
  await go("/dashboards/" + slug, 4000);
  await key("k", 2, "KeyK"); await sleep(500);
  expect("Ctrl+K opens the search", await ev(`!!document.querySelector('dialog.gs-dlg[open]')`), true);
  await send("Input.insertText", { text: "probe fridge 4 " + suf });
  expect("results", await until(`[...document.querySelectorAll('.gs-dlg a.gs-item')].length >= 2`), true);
  expect("first result selected", await ev(`document.querySelector('.gs-dlg a.gs-item.on').textContent.includes('Probe fridge 4')`), true);
  await key("ArrowDown"); await sleep(200);
  const target = await ev(`document.querySelector('.gs-dlg a.gs-item.on').getAttribute('href')`);
  expect("arrow moves the selection", target, (h) => typeof h === "string" && h.startsWith("/assets"));
  await shot("overview-search.png");
  await key("Enter"); await sleep(3500);
  expect("Enter opens the object", await ev(`location.pathname + location.search`), target);

  // ---- phone
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/dashboards/" + slug, 5000);
  expect("phone: no horizontal page scroll", await ev(`document.documentElement.scrollWidth <= window.innerWidth + 1`), true);
  await shot("overview-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");

  // clean up
  await api("DELETE", `/api/v1/dashboards/${slug}/share?reason=headless%20probe`);
  expect("dashboard removed", await ev(`ctrl32.api('DELETE','/api/v1/dashboards/${slug}?reason=headless%20probe').then(() => true)`), true);
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
