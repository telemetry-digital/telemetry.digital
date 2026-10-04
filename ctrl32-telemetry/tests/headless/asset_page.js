// Headless Chrome: the assets list as a table and the asset and site pages (0.74.0). Creates its own site with a
// room, a fridge in the room (child asset), datastreams fed by an HTTP device, an alarm rule that raises an alarm,
// then checks: the list (rows, hierarchy, site and type filters, search, sorting, a row opens the asset page), the
// asset page (breadcrumb, tabs, attributes, child assets, devices, active alarms; telemetry with chart and child
// assets; datastreams with the dialog; alarm rules; history), the site page, the sites list and the links from the
// device page.
//   BASE=http://127.0.0.1:8080 OUT=. node tests/headless/asset_page.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = +(process.env.CDP_PORT || 9368);
const tmp = process.env.TEMP || require("os").tmpdir();
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-assetpage", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n")[0]);
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: false });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? "EXC " + r.result.exceptionDetails.text : r.result.result.value; };
  const expect = (what, got, want) => { const ok = JSON.stringify(got) === JSON.stringify(want); console.log((ok ? "ok   " : "FAIL ") + what + (ok ? "" : ` (got ${JSON.stringify(got)}, want ${JSON.stringify(want)})`)); if (!ok) problems.push(what); };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name + ".png", Buffer.from(s.result.data, "base64")); };
  const go = async (path, wait = 3000) => { await send("Page.navigate", { url: base + path }); await sleep(wait); };
  await go("/login", 1500);
  expect("sign in", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  const suf = Math.floor(Math.random() * 1e6);
  const setup = JSON.parse(await ev(`(async () => {
    const me = await (await fetch('/api/v1/auth/me')).json(), H = {'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf_token};
    const call = async (m, p, b) => { const r = await fetch(p, {method: m, headers: H, body: b ? JSON.stringify(b) : undefined}); return r.json(); };
    const post = (p, b) => call('POST', p, b);
    const site = await post('/api/v1/sites', {name: 'AP site ${suf}', address: '1 Asset Street', reason: 'headless'});
    await call('PATCH', '/api/v1/sites/' + site.id, {location: {lat: 48.15, lon: 17.11}, reason: 'headless'});
    const room = await post('/api/v1/assets', {site_id: site.id, type: 'room', name: 'AP room ${suf}', attributes: {floor: 'ground', area_m2: 24}, reason: 'headless'});
    const fridge = await post('/api/v1/assets', {site_id: site.id, parent_id: room.id, type: 'fridge', name: 'AP fridge ${suf}', reason: 'headless'});
    const other = await post('/api/v1/assets', {site_id: site.id, type: 'meter', name: 'AP meter ${suf}', reason: 'headless'});
    const rt = await post('/api/v1/datastreams', {asset_id: room.id, key: 'temp', quantity: 'temperature', unit: '°C', expected_interval: '10 minutes', reason: 'headless'});
    const ft = await post('/api/v1/datastreams', {asset_id: fridge.id, key: 'ftemp', quantity: 'temperature', unit: '°C', reason: 'headless'});
    const dev = await post('/api/v1/devices', {external_id: 'ap-${suf}', name: 'AP logger ${suf}', transport: 'http', reason: 'headless'});
    const tok = (await post('/api/v1/devices/' + dev.id + '/token', {reason: 'headless'})).token;
    await post('/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: rt.id, reason: 'headless'});
    await post('/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: ft.id, reason: 'headless'});
    await post('/api/v1/alarm-rules', {datastream_id: ft.id, type: 'high', warning: 7, action: 8, reason: 'headless'});
    const now = Math.floor(Date.now() / 1000), D = {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + tok};
    const samples = []; for (let i = 12; i >= 1; i--) samples.push({ts: now - i * 600, v: {temp: 21 + (i % 3) * 0.5, ftemp: 4 + (i % 4) * 0.3}});
    samples.push({ts: now - 30, v: {temp: 21.4, ftemp: 9.2}});
    await fetch('/api/v1/d/telemetry', {method: 'POST', headers: D, body: JSON.stringify(samples)});
    return JSON.stringify({site: site.id, room: room.id, fridge: fridge.id, other: other.id, rt: rt.id, ft: ft.id, dev: dev.id, tok});
  })()`));
  await sleep(1500);
  // ---- the assets list
  await go("/assets", 3500);
  expect("list is a table, not cards", await ev(`!!document.querySelector('#assetTable') && !document.querySelector('#assets.grid')`), true);
  await ev(`(() => { const s = document.querySelector('#aSite'); s.value = '${setup.site}'; s.dispatchEvent(new Event('change')); return true; })()`); await sleep(300);
  expect("site filter: the three assets of the site", await ev(`document.querySelectorAll('#assetTable tbody tr[data-id]').length`), 3);
  expect("hierarchy: the fridge under the room", await ev(`(() => { const ids = [...document.querySelectorAll('#assetTable tbody tr[data-id]')].map(r => r.dataset.id); return ids.indexOf('${setup.fridge}') === ids.indexOf('${setup.room}') + 1; })()`), true);
  expect("child indented", await ev(`!!document.querySelector('#assetTable tr[data-id="${setup.fridge}"] .tree')`), true);
  expect("parent column links the room", await ev(`!!document.querySelector('#assetTable tr[data-id="${setup.fridge}"] a[href="/assets/${setup.room}"]')`), true);
  expect("counts of the room (datastreams, devices)", await ev(`[...document.querySelectorAll('#assetTable tr[data-id="${setup.room}"] td.num')].map(t => t.textContent).join()`), "1,1");
  expect("alarm of the fridge", await ev(`document.querySelector('#assetTable tr[data-id="${setup.fridge}"]').textContent.includes('action')`), true);
  expect("URL keeps the filter", await ev(`location.search.includes('site=${setup.site}')`), true);
  await ev(`(() => { const s = document.querySelector('#aType'); s.value = 'fridge'; s.dispatchEvent(new Event('change')); return true; })()`); await sleep(300);
  expect("type filter", await ev(`document.querySelectorAll('#assetTable tbody tr[data-id]').length`), 1);
  await ev(`(() => { const s = document.querySelector('#aType'); s.value = ''; s.dispatchEvent(new Event('change')); const q = document.querySelector('#aSearch'); q.value = 'AP meter'; q.dispatchEvent(new Event('input')); return true; })()`); await sleep(300);
  expect("search", await ev(`[...document.querySelectorAll('#assetTable tbody tr[data-id]')].map(r => r.dataset.id).join()`), setup.other);
  await ev(`(() => { const q = document.querySelector('#aSearch'); q.value = ''; q.dispatchEvent(new Event('input')); document.querySelector('#assetTable th[data-k="name"]').click(); return true; })()`); await sleep(300);
  expect("sorted by name: flat", await ev(`[...document.querySelectorAll('#assetTable tbody tr[data-id]')].map(r => r.querySelector('b').textContent.split(' ')[1]).join()`), "fridge,meter,room");
  await ev(`document.querySelector('#assetTable th[data-k="name"]').click(), true`); await sleep(300);
  expect("sorted by name: reversed", await ev(`document.querySelector('#assetTable tbody tr[data-id] b').textContent.startsWith('AP room')`), true);
  await shot("assets-list");
  await ev(`document.querySelector('#assetTable tr[data-id="${setup.room}"] td:nth-child(2)').click(), true`); await sleep(3500);
  expect("a row opens the asset page", await ev(`location.pathname`), "/assets/" + setup.room);
  // ---- the asset page: overview
  expect("five tabs", await ev(`[...document.querySelectorAll('#tabs a')].map(a => a.dataset.section).join()`), "overview,telemetry,datastreams,rules,history");
  expect("breadcrumb: assets › site", await ev(`[...document.querySelectorAll('#aCrumbs a')].map(a => a.getAttribute('href')).join()`), "/assets,/assets/sites/" + setup.site);
  expect("title", await ev(`document.querySelector('#aTitle').textContent`), "AP room " + suf);
  expect("attributes shown", await ev(`document.querySelector('#aAttrs').textContent.includes('area_m2')`), true);
  expect("attribute editor (config.write)", await ev(`document.querySelectorAll('#attrRows .attr-row').length`), 2);
  expect("child asset row", await ev(`!!document.querySelector('#childTable tr[data-href="/assets/${setup.fridge}"]')`), true);
  expect("device feeding it", await ev(`!!document.querySelector('#devTable a[href="/devices/${setup.dev}"]')`), true);
  expect("position of the site", await ev(`document.querySelector('#aLocInfo').textContent.includes('48.15000')`), true);
  await shot("asset-overview");
  // attributes: add one with a reason
  await ev(`document.querySelector('#attrEdit').click(), true`); await sleep(200);
  expect("attribute form opens", await ev(`!document.querySelector('#attrForm').hidden`), true);
  await ev(`(() => { document.querySelector('#attrAdd').click(); const ins = document.querySelectorAll('#attrRows .attr-row:last-child input'); ins[0].value = 'capacity_l'; ins[0].dispatchEvent(new Event('input')); ins[1].value = '350'; ins[1].dispatchEvent(new Event('input')); const f = document.querySelector('#attrForm'); f.reason.value = 'headless'; f.requestSubmit(); return true; })()`); await sleep(2000);
  expect("attribute saved as a number", await ev(`fetch('/api/v1/assets/${setup.room}').then(r => r.json()).then(j => j.asset.attributes.capacity_l)`), 350);
  // ---- the fridge: breadcrumb with the parent, alarms
  await go("/assets/" + setup.fridge);
  expect("breadcrumb: assets › site › room", await ev(`[...document.querySelectorAll('#aCrumbs a')].map(a => a.getAttribute('href')).join()`), `/assets,/assets/sites/${setup.site},/assets/${setup.room}`);
  expect("active alarm of the fridge", await ev(`document.querySelectorAll('#alarmTable tbody tr').length === 1 && document.querySelector('#alarmTable').textContent.includes('ftemp')`), true);
  // ---- telemetry
  await go(`/assets/${setup.room}/telemetry`, 4000);
  expect("telemetry: the room's datastream", await ev(`[...document.querySelectorAll('#telTable tr[data-row]')].map(r => r.dataset.row).join()`), setup.rt);
  expect("device column links the device", await ev(`!!document.querySelector('#telTable a[href="/devices/${setup.dev}"]')`), true);
  expect("sparkline", await ev(`document.querySelectorAll('#telTable svg').length`), 1);
  await ev(`(() => { const c = document.querySelector('#telKids'); c.checked = true; c.dispatchEvent(new Event('change')); return true; })()`); await sleep(2500);
  expect("with child assets: the fridge's too", await ev(`document.querySelectorAll('#telTable tr[data-row]').length`), 2);
  expect("asset column links the fridge", await ev(`!!document.querySelector('#telTable a[href="/assets/${setup.fridge}"]')`), true);
  await ev(`document.querySelector('#telTable tr[data-row="${setup.ft}"] td:nth-child(4)').click(), true`); await sleep(3000);
  expect("chart of the fridge", await ev(`!!document.querySelector('#telChart svg path')`), true);
  expect("values", await ev(`document.querySelectorAll('#telRows tr').length`), 13);
  expect("export links of the asset", await ev(`document.querySelector('#telXlsx').getAttribute('href').startsWith('/api/v1/assets/${setup.room}/telemetry/export?datastream_id=${setup.ft}')`), true);
  expect("annotations panel", await ev(`!!document.querySelector('#annForm')`), true);
  await shot("asset-telemetry");
  // live: a new value without reloading
  await ev(`fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', 'Authorization': 'Bearer ${setup.tok}'}, body: JSON.stringify({v: {temp: 23.75}})}).then(r => r.status)`);
  await sleep(3500);
  expect("live value", await ev(`document.querySelector('#telTable tr[data-row="${setup.rt}"]').textContent.includes('23.75')`), true);
  // ---- datastreams
  await go(`/assets/${setup.fridge}/datastreams?ds=${setup.ft}`, 3500);
  expect("datastreams table", await ev(`document.querySelectorAll('#dsTable tr[data-ds]').length`), 1);
  expect("?ds= opens the datastream", await ev(`document.querySelector('#dsDlg').open && document.querySelector('#dsDlg').textContent.includes('ftemp')`), true);
  expect("source device form", await ev(`document.querySelector('#dsAssign').device_id.value`), setup.dev);
  expect("edit form", await ev(`!!document.querySelector('#dsEdit')`), true);
  await shot("asset-datastream-dialog");
  await ev(`document.querySelector('#dsClose').click(), true`); await sleep(1500);
  // a new datastream through the dialog
  await ev(`(() => { document.querySelector('#newDsBtn').click(); const f = document.querySelector('#dsForm'); f.key.value = 'door'; f.quantity.value = 'state'; f.reason.value = 'headless'; f.requestSubmit(); return true; })()`); await sleep(2500);
  expect("new datastream listed", await ev(`document.querySelectorAll('#dsTable tr[data-ds]').length`), 2);
  // ---- alarm rules
  await go(`/assets/${setup.fridge}/rules`, 3000);
  expect("alarm rule of the fridge", await ev(`document.querySelectorAll('#ruleTable tr[data-ds]').length`), 1);
  // ---- history
  await go(`/assets/${setup.fridge}/history`, 3000);
  expect("history entries (asset, datastreams, rule, assignment)", await ev(`(() => { const a = [...document.querySelectorAll('#histTable tr[data-seq] code')].map(c => c.textContent); return ['asset.create', 'datastream.create', 'alarm_rule.create', 'assignment.create'].every(x => a.includes(x)); })()`), true);
  await ev(`document.querySelector('#histTable tr[data-seq]').click(), true`); await sleep(300);
  expect("entry details", await ev(`!!document.querySelector('#histTable tr.detail:not([hidden]) pre')`), true);
  // ---- edit: move the meter under the room
  await go("/assets/" + setup.other);
  await ev(`document.querySelector('#aEditBtn').click(), true`); await sleep(1500);
  expect("parent choices exclude the asset itself", await ev(`[...document.querySelector('#editForm').parent_id.options].some(o => o.value === '${setup.other}')`), false);
  await ev(`(() => { const f = document.querySelector('#editForm'); f.parent_id.value = '${setup.room}'; f.reason.value = 'headless move'; f.requestSubmit(); return true; })()`); await sleep(2500);
  expect("moved under the room", await ev(`[...document.querySelectorAll('#aCrumbs a')].pop().getAttribute('href')`), "/assets/" + setup.room);
  // ---- the site page and the sites list
  await go("/assets/sites/" + setup.site, 3500);
  expect("site page title", await ev(`document.querySelector('#sTitle').textContent`), "AP site " + suf);
  expect("assets of the site as a tree", await ev(`document.querySelectorAll('#sAssetTable tr[data-href]').length === 3 && document.querySelectorAll('#sAssetTable .tree').length === 2`), true);
  expect("devices of the site", await ev(`!!document.querySelector('#sDevTable a[href="/devices/${setup.dev}"]')`), true);
  expect("alarms of the site link the asset", await ev(`!!document.querySelector('#sAlarmTable a[href="/assets/${setup.fridge}"]')`), true);
  expect("site position", await ev(`document.querySelector('#sLocInfo').textContent.includes('17.11000')`), true);
  await shot("site-page");
  await go("/assets/sites", 3000);
  expect("sites list row", await ev(`!!document.querySelector('#siteTable tr[data-id="${setup.site}"]')`), true);
  await ev(`document.querySelector('#siteTable tr[data-id="${setup.site}"] td:nth-child(2)').click(), true`); await sleep(2500);
  expect("a site row opens the site page", await ev(`location.pathname`), "/assets/sites/" + setup.site);
  // ---- links: the device page, old links of the assets screen, the alarm rules screen
  await go(`/devices/${setup.dev}/datastreams`, 3000);
  expect("device page links the asset", await ev(`!!document.querySelector('#assignRows a[href="/assets/${setup.fridge}"]')`), true);
  await go(`/assets?site=${setup.site}&asset=${setup.fridge}`, 3500);
  expect("old asset link opens the asset page", await ev(`location.pathname`), "/assets/" + setup.fridge);
  await go(`/assets?ds=${setup.ft}`, 4000);
  expect("old datastream link opens it on the asset page", await ev(`location.pathname + ' ' + document.querySelector('#dsDlg').open`), `/assets/${setup.fridge}/datastreams true`);
  await go("/assets/rules", 3000);
  expect("alarm rules screen links the asset page", await ev(`!!document.querySelector('#rules a[href="/assets/${setup.fridge}"]')`), true);
  // ---- phone: the tables scroll inside their cards, the page does not
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  for (const p of ["/assets", `/assets/${setup.room}`, `/assets/${setup.room}/telemetry`, `/assets/sites/${setup.site}`]) {
    await go(p, 3500);
    expect("phone: no horizontal page scroll on " + p.replace(/[0-9a-f-]{36}/g, "…"), await ev(`document.documentElement.scrollWidth <= innerWidth + 1`), true);
  }
  await shot("asset-phone");
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
