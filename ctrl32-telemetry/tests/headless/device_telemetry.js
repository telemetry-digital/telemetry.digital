// Headless Chrome: the Telemetry tab of the device page (0.67.0) and the Audit pages. Creates its own device (HTTP
// transport) with a datastream, keys without a datastream and status messages, then checks: every tab of the device
// is visible, the table lists the three sources, a key opens its chart, values arrive live (a new value and a new
// key without reloading), the sensor-fault quality is shown; the audit trail, reviews and report signatures render.
//   BASE=http://127.0.0.1:8080 OUT=. node tests/headless/device_telemetry.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9367;
const tmp = process.env.TEMP || require("os").tmpdir();
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-devtel", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
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
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("sign in", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  // test data, created through the API from the page
  const suf = Math.floor(Math.random() * 1e6);
  const setup = await ev(`(async () => {
    const me = await (await fetch('/api/v1/auth/me')).json(), H = {'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf_token};
    const post = async (p, b) => (await fetch(p, {method: 'POST', headers: H, body: JSON.stringify(b)})).json();
    const site = await post('/api/v1/sites', {name: 'Headless site ${suf}', reason: 'headless'});
    const asset = await post('/api/v1/assets', {site_id: site.id, type: 'fridge', name: 'Headless fridge ${suf}', reason: 'headless'});
    const ds = await post('/api/v1/datastreams', {asset_id: asset.id, key: 'temp', quantity: 'temperature', unit: '°C', range: {min: -55, max: 125}, reason: 'headless'});
    const dev = await post('/api/v1/devices', {external_id: 'hl-${suf}', name: 'headless', transport: 'http', reason: 'headless'});
    const tok = (await post('/api/v1/devices/' + dev.id + '/token', {reason: 'headless'})).token;
    await post('/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: ds.id, reason: 'headless'});
    const now = Math.floor(Date.now() / 1000), D = {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + tok};
    const samples = []; for (let i = 12; i >= 1; i--) samples.push({ts: now - i * 600, v: {temp: 4 + (i % 3) * 0.5, rh: 50 + i}});
    samples.push({ts: now - 60, v: {temp: -127}});
    await fetch('/api/v1/d/telemetry', {method: 'POST', headers: D, body: JSON.stringify(samples)});
    await fetch('/api/v1/d/status', {method: 'POST', headers: D, body: JSON.stringify({bat: 3.6, rssi: -70})});
    return JSON.stringify({id: dev.id, tok});
  })()`);
  const { id: devId, tok } = JSON.parse(setup);
  await send("Page.navigate", { url: base + "/devices/" + devId + "/telemetry" }); await sleep(3500);
  expect("all seven tabs visible", await ev(`[...document.querySelectorAll('#tabs a')].filter(a => a.offsetParent).length`), 7);
  expect("Telemetry tab active", await ev(`document.querySelector('#tabs a.active').dataset.section`), "telemetry");
  expect("rows by source", await ev(`[...document.querySelectorAll('#telTable tr[data-row]')].map(r => r.dataset.row).join(',')`), "status:bat,unassigned:rh,status:rssi,datastream:temp");
  expect("latest temp is a sensor fault", await ev(`document.querySelector('#telTable tr[data-row="datastream:temp"]').textContent.includes('sensor fault')`), true);
  expect("sparklines drawn", await ev(`document.querySelectorAll('#telTable svg').length >= 2`), true);
  await ev(`(() => { const s = document.querySelector('#telSearch'); s.value = 'rh'; s.dispatchEvent(new Event('input')); return true; })()`);
  expect("search filters keys", await ev(`document.querySelectorAll('#telTable tr[data-row]').length`), 1);
  await ev(`(() => { const s = document.querySelector('#telSearch'); s.value = ''; s.dispatchEvent(new Event('input')); return true; })()`);
  await shot("device-telemetry-table");
  await ev(`document.querySelector('#telTable tr[data-row="datastream:temp"]').click(), true`); await sleep(2500);
  expect("chart of temp", await ev(`!!document.querySelector('#telChart svg path')`), true);
  expect("fault marked on the chart", await ev(`document.querySelectorAll('#telChart circle[fill="var(--bad)"]').length`), 1);
  expect("values table", await ev(`document.querySelectorAll('#telRows tr').length`), 13);
  expect("export links", await ev(`document.querySelector('#telXlsx').href.includes('format=xlsx')`), true);
  await shot("device-telemetry-chart");
  // live: a new value of rh and a new key without a datastream arrive without reloading
  await ev(`fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', 'Authorization': 'Bearer ${tok}'}, body: JSON.stringify({v: {rh: 77.7, co2: 640}})}).then(r => r.status)`);
  await sleep(4000);
  expect("live value", await ev(`document.querySelector('#telTable tr[data-row="unassigned:rh"]').textContent.includes('77.7')`), true);
  expect("live new key", await ev(`!!document.querySelector('#telTable tr[data-row="unassigned:co2"]')`), true);
  expect("live indicator", await ev(`document.querySelector('#telLive').textContent.length > 0`), true);
  await ev(`document.querySelector('#telTable tr[data-row="unassigned:rh"]').click(), true`); await sleep(2500);
  expect("create-datastream form for a key without one", await ev(`!!document.querySelector('#mkForm') && document.querySelector('#mkForm').asset_id.options.length > 1`), true);
  for (const p of ["/audit", "/audit/reviews", "/audit/reports"]) {
    await send("Page.navigate", { url: base + p }); await sleep(2500);
    expect("page " + p, await ev(`!!document.querySelector('section.active table tbody tr')`), true);
  }
  await send("Page.navigate", { url: base + "/audit" }); await sleep(2500);
  expect("audit trail entries", await ev(`document.querySelectorAll('#trail tr[data-i]').length > 0`), true);
  await ev(`document.querySelector('#trail tr[data-i]').click(), true`); await sleep(300);
  expect("entry details open", await ev(`!!document.querySelector('#trail tr.detail pre')`), true);
  await shot("audit-trail");
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
