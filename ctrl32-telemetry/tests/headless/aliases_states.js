// Headless Chrome: entity aliases, key filters and dashboard states (0.73.0). Creates a store room with three fridges
// (each with its own logger and a max_temp attribute) and a dashboard with two states, then checks: the entities table
// (latest values and attributes as columns, highlighting by a key filter), an overview table with a key filter, the
// entity selector, drill-down from a row to the device detail state (address, breadcrumb, Back, a shared link), the
// editor (state tabs, preview entity, settings tabs for aliases, key filters and states, the alias on the Data tab,
// actions), a public link that drills down too, and a phone. Screenshots states-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9383;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-states", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
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
  const api = (m, p, b) => ev(`ctrl32.api(${JSON.stringify(m)}, ${JSON.stringify(p)}${b ? ", " + JSON.stringify(b) : ""}).then(r => r, e => ({error: e.message}))`);

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await go("/dashboards", 2000);
  // own data: a room with three fridges, a logger each; fridge 2 is above 8 °C, fridge 3 above its own limit
  const suf = String(Date.now() % 100000);
  const site = (await api("POST", "/api/v1/sites", { name: "States store " + suf, reason: "headless probe" })).id;
  const room = (await api("POST", "/api/v1/assets", { site_id: site, type: "room", name: "States room " + suf, reason: "headless probe" })).id;
  const fr = [], dev = [], ds = [];
  const temps = [5.1, 9.2, 7.4], limits = [8, 10, 7];
  for (let i = 0; i < 3; i++) {
    fr.push((await api("POST", "/api/v1/assets", { site_id: site, parent_id: room, type: "fridge", name: `States fridge ${i + 1} ${suf}`, attributes: { max_temp: limits[i] }, reason: "headless probe" })).id);
    ds.push((await api("POST", "/api/v1/datastreams", { asset_id: fr[i], key: "temp", quantity: "temperature", unit: "°C", kind: "gauge", reason: "headless probe" })).id);
    dev.push((await api("POST", "/api/v1/devices", { external_id: `states-${i}-${suf}`, name: `States logger ${i + 1} ${suf}`, transport: "http", reason: "headless probe" })).id);
    await api("POST", `/api/v1/devices/${dev[i]}/datastreams`, { datastream_id: ds[i], reason: "headless probe" });
    const tok = (await api("POST", `/api/v1/devices/${dev[i]}/token`, { reason: "headless probe" })).token;
    const now = Math.floor(Date.now() / 1000); const pts = [6, 5, 4, 3, 2, 1, 0].map((k) => ({ ts: now - k * 600, v: { temp: temps[i] - k * 0.1 } }));
    await ev(`fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: 'Bearer ${tok}'}, body: JSON.stringify(${JSON.stringify(pts)})}).then(r => r.status)`);
  }
  const g = (x, y, w, h) => ({ x, y, w, h });
  const spec = { version: 1, layout: { type: "grid", columns: 12, row_height: 90 }, range: "6h",
    aliases: [{ id: "fridges", name: "Fridges of the store room", kind: "hierarchy", root: { type: "asset", id: room }, entity_type: "asset", asset_type: ["fridge"] },
      { id: "logger", name: "Selected logger", kind: "current", entity_type: "device" }, { id: "temps", name: "Its temperatures", kind: "current", entity_type: "datastream" }],
    key_filters: [{ id: "warm", name: "Above 8 °C", rules: [{ source: "value", key: "temp", op: "gt", value: 8 }] }, { id: "own", name: "Above its own limit", rules: [{ source: "value", key: "temp", op: "gt", dynamic: { from: "self", key: "max_temp" } }] }],
    states: [{ id: "main", name: "Store room" }, { id: "detail", name: "Logger detail" }],
    widgets: [
      { id: "sel", type: "entity_selector", geometry: g(0, 0, 4, 3), alias: { id: "fridges" }, params: { title: "Fridge", style: "list" } },
      { id: "table", type: "entities_table", geometry: g(4, 0, 8, 3), alias: { id: "fridges" }, params: { title: "Fridges", columns: ["name", "t:temp=Temperature", "a:max_temp=Limit", "alarm"], highlight_filter: "own" },
        actions: [{ event: "row", action: "state", target: "detail", pass: "device" }, { event: "row_button", action: "device", label: "Device" }] },
      { id: "warm", type: "overview_table", geometry: g(0, 3, 12, 3), alias: { id: "fridges", filter: "warm", quantity: ["temperature"] }, bindings: { datastreams: [] }, params: { title: "Fridges above 8 °C", columns: ["asset", "value", "age"] },
        actions: [{ event: "row", action: "state", target: "detail", pass: "device" }] },
      { id: "trend", type: "chart", state: "detail", geometry: g(0, 0, 8, 3), alias: { id: "temps" }, bindings: { datastreams: [] }, params: { title: "Temperature" } },
      { id: "devs", type: "devices", state: "detail", geometry: g(8, 0, 4, 3), alias: { id: "logger" }, bindings: { devices: [] }, params: { title: "Logger" } }] };
  const slug = (await api("POST", "/api/v1/dashboards", { name: "States probe " + suf, spec, reason: "headless probe" })).slug;
  expect("dashboard with states created", typeof slug, "string");

  // ---- view: the main state
  await go("/dashboards/" + slug, 4000);
  expect("main state: three widgets, no breadcrumb", await ev(`[document.querySelectorAll('#vGrid .dash-widget').length, document.querySelector('#vStates').hidden]`), [3, true]);
  expect("entities table: the three fridges", await until(`document.querySelectorAll('[data-id="table"] tr[data-ent]').length === 3`), true);
  expect("columns from latest values and attributes", await ev(`[...document.querySelectorAll('[data-id="table"] th')].map(t => t.textContent.replace(/[▲▼]/g, '').trim()).slice(0, 3).join()`), "Name,Temperature,Limit");
  expect("fridge 3 highlighted (above its own limit)", await ev(`[...document.querySelectorAll('[data-id="table"] tr.ot-hl')].map(r => r.textContent.match(/fridge (\\d)/)[1]).join()`), "3");
  expect("value with unit", await ev(`document.querySelector('[data-id="table"] tr[data-ent="asset:${fr[1]}"]').textContent.includes('9.20 °C')`), true);
  expect("overview table with the key filter: fridge 2 only", await until(`document.querySelectorAll('[data-id="warm"] tr[data-ds]').length === 1 && document.querySelector('[data-id="warm"] tr[data-ds]').dataset.ds === '${ds[1]}'`), true);
  expect("entity selector chose the first fridge", await until(`location.search.includes('st=.asset.${fr[0]}') && !!document.querySelector('[data-id="sel"] .es-item.on')`), true);
  await ev(`document.querySelectorAll('[data-id="sel"] .es-item')[1].click()`);
  expect("choosing another fridge changes the current entity", await until(`location.search.includes('${fr[1]}') && document.querySelector('[data-id="sel"] .es-item.on') && document.querySelector('[data-id="sel"] .es-item.on').textContent.includes('fridge 2')`), true);
  await shot("states-main.png");

  // ---- drill-down: a row opens the detail state with the fridge's logger
  await ev(`document.querySelector('[data-id="table"] tr[data-ent="asset:${fr[1]}"] td').click()`);
  expect("detail state in the address", await until(`location.search.includes('st=detail.device.${dev[1]}')`), true);
  expect("detail widgets", await until(`document.querySelectorAll('#vGrid .dash-widget').length === 2 && !!document.querySelector('[data-id="trend"] svg')`), true);
  expect("breadcrumb: main view › detail with the logger", await ev(`[!document.querySelector('#vStates').hidden, document.querySelector('#vStates b').textContent]`), [true, "Logger detail: States logger 2 " + suf]);
  expect("the logger of the detail state", await until(`document.querySelector('[data-id="devs"]').textContent.includes('States logger 2 ${suf}')`), true);
  await shot("states-detail.png");
  const link = await ev(`location.pathname + location.search`);
  await ev(`document.querySelector('#vStates [data-back]').click()`);
  expect("Back returns to the main state", await until(`document.querySelectorAll('#vGrid .dash-widget').length === 3 && document.querySelector('#vStates').hidden`), true);
  await go(link, 4000);
  expect("a link opens the detail state", await until(`!!document.querySelector('[data-id="trend"] svg') && document.querySelector('[data-id="devs"]').textContent.includes('States logger 2')`), true);
  await ev(`history.back(); true`);
  // the overview table drills down too
  await go("/dashboards/" + slug, 4000);
  await until(`document.querySelectorAll('[data-id="warm"] tr[data-ds]').length === 1`);
  await ev(`document.querySelector('[data-id="warm"] tr[data-ds] td').click()`);
  expect("overview-table row opens the detail", await until(`location.search.includes('st=detail.device.${dev[1]}')`), true);

  // ---- editor
  await go("/dashboards/" + slug + "/edit", 5000);
  expect("state tabs and the preview entity", await ev(`[document.querySelectorAll('#eStates [data-est]').length, !!document.querySelector('#ePrev')]`), [2, true]);
  await ev(`document.querySelectorAll('#eStates [data-est]')[1].click()`); await sleep(1200);
  expect("the detail state in the editor", await ev(`document.querySelectorAll('#eGrid .dash-widget').length`), 2);
  await ev(`(() => { const s = document.querySelector('#ePrev'); s.value = 'device:${dev[2]}'; s.dispatchEvent(new Event('change')); })()`);
  expect("preview with a logger", await until(`!!document.querySelector('#eGrid [data-id="trend"] svg')`), true);
  await shot("states-editor.png");
  await ev(`document.querySelectorAll('#eStates [data-est]')[0].click()`); await sleep(1200);
  await ev(`document.querySelector('#eGrid .dash-widget[data-id="table"]').click()`); await sleep(800);
  expect("Data tab: the alias", await ev(`[document.querySelector('[data-al="id"]').value, !!document.querySelector('[data-al="filter"]')]`), ["fridges", true]);
  expect("alias preview", await until(`/Matches 3/.test((document.querySelector('#eProps .src-prev') || {}).textContent || '')`), true);
  await ev(`document.querySelector('#eProps [data-tab="actions"]').click()`); await sleep(300);
  expect("Actions tab: row click and row button", await ev(`[...document.querySelectorAll('#eProps [data-act] [data-ak="event"]')].map(s => s.value).join()`), "row,row_button");
  await shot("states-editor-actions.png");
  await ev(`document.querySelector('#eSettings').click()`); await sleep(500);
  await ev(`document.querySelector('#setTabs [data-st="aliases"]').click()`);
  expect("aliases in the settings", await until(`document.querySelectorAll('#alList [data-ai]').length === 3 && /Matches 3/.test(document.querySelector('#alList .al-prev').textContent)`), true);
  await ev(`(() => { const s = document.querySelector('#alPreset'); s.value = 'current_devices'; s.dispatchEvent(new Event('change')); })()`);
  expect("preset: devices of the selected asset", await ev(`[document.querySelectorAll('#alList [data-ai]').length, document.querySelectorAll('#alList [data-ai]')[3].querySelector('[data-af="kind"]').value]`), [4, "hierarchy"]);
  await shot("states-editor-aliases.png");
  await ev(`document.querySelector('#setTabs [data-st="keyfilters"]').click()`); await sleep(200);
  expect("key filters with a dynamic value", await ev(`[document.querySelectorAll('#kfList [data-ki]').length, document.querySelectorAll('#kfList [data-ki]')[1].querySelector('[data-k="dfrom"]').value]`), [2, "self"]);
  await ev(`(() => { const s = document.querySelector('#kfPreset'); s.value = 'above_viewer'; s.dispatchEvent(new Event('change')); })()`);
  expect("preset with a value filter", await ev(`[document.querySelectorAll('#kfList [data-ki]').length, document.querySelectorAll('#fltList [data-fi]').length]`), [3, 1]);
  await shot("states-editor-keyfilters.png");
  await ev(`document.querySelector('#setTabs [data-st="states"]').click()`); await sleep(200);
  expect("states in the settings", await ev(`document.querySelectorAll('#stList [data-si]').length`), 2);
  await ev(`document.querySelector('#setDlg').close(); true`);

  // ---- public link: the same drill-down within the dashboard's scope
  const share = await api("POST", `/api/v1/dashboards/${slug}/share`, { reason: "headless probe" });
  await go("/p/" + share.token, 4000);
  expect("public: the fridges", await until(`document.querySelectorAll('[data-id="table"] tr[data-ent]').length === 3`), true);
  expect("public: no device page button", await ev(`document.querySelectorAll('[data-id="table"] [data-rb]').length`), 0);
  await ev(`document.querySelector('[data-id="table"] tr[data-ent="asset:${fr[2]}"] td').click()`);
  expect("public: drill-down to the detail", await until(`location.search.includes('st=detail.device.${dev[2]}') && !!document.querySelector('[data-id="trend"] svg')`), true);
  await shot("states-public.png");

  // ---- phone
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/dashboards/" + slug + "?st=detail.device." + dev[0] + ".Logger", 4000);
  expect("phone: no horizontal page scroll", await ev(`document.documentElement.scrollWidth <= window.innerWidth + 1`), true);
  await shot("states-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");

  await api("DELETE", `/api/v1/dashboards/${slug}/share?reason=headless%20probe`);
  expect("dashboard removed", await ev(`ctrl32.api('DELETE','/api/v1/dashboards/${slug}?reason=headless%20probe').then(() => true)`), true);
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "problems: none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
