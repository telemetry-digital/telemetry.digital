// Headless Chrome: object counting (spec 029) and scheduled reports. Creates an area with two entrances and a sensor
// through the API, sends passages through the device protocol, then checks the page Object counting (tiles, entrances
// with shares, the hour chart, the export dialog, the correction dialog), the areas and sensors sections, the camera
// line editor (a line drawn with two clicks, saved and read back), the four counting widgets on a dashboard, and
// Settings → Scheduled reports (a preset fills the cron and shows the next runs, a report is saved and sent now, the
// delivery log has the run). Screenshots counting-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = +(process.env.CDP_PORT || 9385);
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-counting", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2500) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };
  const suf = Date.now() % 100000;

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await go("/counting", 2500);
  // an area with two entrances and a light barrier through the device protocol
  const setup = await ev(`(async () => {
    const api = ctrl32.api, R = 'headless probe';
    const site = await api('POST', '/api/v1/sites', {name: 'Probe site ${suf}', reason: R});
    const area = await api('POST', '/api/v1/counting/areas', {site_id: site.id, name: 'Probe building ${suf}', limits: {people: 20}, reset_time: '03:00', entrances: ['North door', 'South door'], reason: R});
    const areas = (await api('GET', '/api/v1/counting/areas')).areas; const a = areas.find(x => x.id === area.id);
    const sensor = await api('POST', '/api/v1/assets', {site_id: site.id, type: 'sensor', name: 'Probe counters ${suf}', reason: R});
    const din = await api('POST', '/api/v1/datastreams', {asset_id: sensor.id, key: 'n_in', quantity: 'count', unit: '', kind: 'gauge', reason: R});
    const dout = await api('POST', '/api/v1/datastreams', {asset_id: sensor.id, key: 'n_out', quantity: 'count', unit: '', kind: 'gauge', reason: R});
    const sin = await api('POST', '/api/v1/datastreams', {asset_id: sensor.id, key: 's_in', quantity: 'count', unit: '', kind: 'gauge', reason: R});
    const dev = await api('POST', '/api/v1/devices', {external_id: 'probe-${suf}', name: 'Probe ${suf}', reason: R});
    for (const d of [din, dout, sin]) await api('POST', '/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: d.id, reason: R});
    const tok = (await api('POST', '/api/v1/devices/' + dev.id + '/token', {reason: R})).token;
    await api('POST', '/api/v1/counting/inputs', {entrance_id: a.entrances[0].id, name: 'North barrier', in_datastream_id: din.id, out_datastream_id: dout.id, reason: R});
    await api('POST', '/api/v1/counting/inputs', {entrance_id: a.entrances[1].id, name: 'South barrier', in_datastream_id: sin.id, reason: R});
    const post = (v) => fetch('/api/v1/d/telemetry', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: 'Bearer ' + tok}, body: JSON.stringify(v)});
    await post({n_in: 6}); await post({s_in: 2}); await post({n_out: 3});
    const cam = await api('POST', '/api/v1/video/cameras', {name: 'Probe camera ${suf}', url: 'rtsp://127.0.0.1:9/probe', record: 'off', reason: R});
    return {area: area.id, cam: cam.id, occ: (await api('GET', '/api/v1/datastreams')).find(d => d.asset_id === a.asset_id && d.key === 'people_occupancy').id};
  })()`);
  expect("area, sensors and passages set up", !!(setup && setup.area && setup.cam), true);
  if (!setup || !setup.area) { console.log("problems: " + problems.join(" | ")); proc.kill(); process.exit(1); }
  await until(`ctrl32.api('GET', '/api/v1/counting/summary?area=${setup.area}').then(s => s.totals.people && s.totals.people.out === 3)`, 20000);

  // ---- overview
  await go(`/counting?area=${setup.area}`, 3500);
  expect("tiles: occupancy 5, in 8, out 3", await ev(`['#tOcc', '#tIn', '#tOut'].map(s => document.querySelector(s).textContent).join()`), "5,8,3");
  expect("capacity bar 25 %", await ev(`document.querySelector('#tCap').hidden ? 'hidden' : document.querySelector('#tCap span').style.width`), "25%");
  expect("entrances with shares of arrivals", await ev(`[...document.querySelectorAll('#entTable tbody tr')].map(r => r.cells[0].textContent + ' ' + r.cells[1].textContent + ' ' + r.cells[3].textContent.trim()).join(' | ')`), "North door 6 75 % | South door 2 25 %");
  expect("hour chart with bars", await ev(`document.querySelectorAll('#hourChart rect.oc-in').length`), (n) => n >= 23);
  expect("day comparison: 7 days", await ev(`document.querySelectorAll('#dayChart rect.oc-in').length`), 7);
  await shot("counting-overview.png");
  await ev(`document.querySelector('#correctBtn').click(); true`); await sleep(500);
  expect("correction dialog with the current occupancy", await ev(`document.querySelector('#corrDlg').open && document.querySelector('#corrForm').value.value`), "5");
  await ev(`(() => { const f = document.querySelector('#corrForm'); f.value.value = 4; f.reason.value = 'headless probe count by hand'; f.requestSubmit(); return true; })()`); await sleep(2000);
  expect("corrected to 4 and logged", await ev(`document.querySelector('#tOcc').textContent + ' ' + [...document.querySelectorAll('#logTable tbody tr')].some(r => r.textContent.includes('headless probe count by hand'))`), "4 true");
  await ev(`document.querySelector('#exportBtn').click(); true`); await sleep(400);
  expect("export dialog: Excel, CSV, PDF", await ev(`[...document.querySelectorAll('#exportForm [data-f]')].map(b => b.dataset.f).join()`), "xlsx,csv,pdf");
  await ev(`document.querySelector('#exportDlg').close(); true`);

  // ---- areas and sensors
  await go("/counting/areas", 2500);
  expect("areas list", await ev(`[...document.querySelectorAll('#areaTable tbody tr')].some(r => r.textContent.includes('Probe building ${suf}') && r.textContent.includes('North door, South door'))`), true);
  await ev(`document.querySelector('[data-edit="${setup.area}"]').click(); true`); await sleep(1500);
  expect("area dialog: entrances editable, reset time", await ev(`document.querySelectorAll('#entList [data-ent]').length + ' ' + document.querySelector('#areaForm').reset_time.value`), "2 03:00");
  await ev(`document.querySelector('#areaDlg').close(); true`);
  await go("/counting/sensors", 2500);
  expect("sensors list", await ev(`[...document.querySelectorAll('#inputTable tbody tr')].filter(r => r.textContent.includes('Probe building ${suf}')).length`), 2);

  // ---- camera line editor
  await go(`/counting/cameras?camera=${setup.cam}`, 4000);
  expect("editor open", await ev(`!document.querySelector('#editorCard').hidden && !!document.querySelector('#ocSvg')`), true);
  await ev(`document.querySelector('#ocAdd').click(); true`);
  const clickAt = async (fx, fy) => ev(`(() => { const s = document.querySelector('#ocSvg'), r = s.getBoundingClientRect(); s.dispatchEvent(new PointerEvent('pointerdown', {bubbles: true, clientX: r.left + r.width * ${fx}, clientY: r.top + r.height * ${fy}, pointerId: 1})); return true; })()`);
  await clickAt(0.1, 0.6); await sleep(200); await clickAt(0.9, 0.6); await sleep(400);
  expect("a line drawn with two clicks", await ev(`document.querySelectorAll('#ocSvg line.oc-line').length + ' ' + document.querySelectorAll('#ocLines tr').length`), "1 1");
  await ev(`(() => { const s = document.querySelector('#ocLines select[data-k="entrance_id"]'); s.selectedIndex = 1; s.dispatchEvent(new Event('change')); const f = document.querySelector('#ocForm'); f.reason.value = 'headless probe line'; f.requestSubmit(); return true; })()`); await sleep(2000);
  expect("line saved for the entrance", await ev(`ctrl32.api('GET', '/api/v1/counting/cameras/${setup.cam}').then(c => c.config.lines.length + ' ' + (c.config.lines[0].entrance_id.length) + ' ' + Math.round(c.config.lines[0].a.x * 10) + ' ' + c.config.lines[0].in)`), "1 36 1 right");
  await shot("counting-editor.png");

  // ---- widgets
  const dash = await ev(`ctrl32.api('POST', '/api/v1/dashboards', {name: 'Counting probe ${suf}', reason: 'headless probe', spec: {version: 1, kind: 'dashboard', layout: {type: 'grid', row_height: 96}, widgets: [
    {id: 'o', type: 'counting_occupancy', geometry: {x: 0, y: 0, w: 3, h: 2}, bindings: {datastreams: ['${setup.occ}']}, params: {title: 'Occupancy', group: 'people'}},
    {id: 'e', type: 'counting_entrances', geometry: {x: 3, y: 0, w: 5, h: 3}, bindings: {datastreams: ['${setup.occ}']}, params: {title: 'Entrances', group: 'people'}},
    {id: 's', type: 'counting_share', geometry: {x: 8, y: 0, w: 4, h: 3}, bindings: {datastreams: ['${setup.occ}']}, params: {title: 'Share', group: 'people', direction: 'in'}},
    {id: 'v', type: 'vehicle_speeds', geometry: {x: 0, y: 3, w: 6, h: 3}, bindings: {datastreams: ['${setup.occ}']}, params: {title: 'Speeds'}}]}}).then(r => r.slug)`);
  expect("dashboard with the counting widgets", typeof dash === "string", true);
  await go("/dashboards/" + dash, 5000);
  const txt = await ev(`document.body.innerText`);
  expect("occupancy widget: 4 of 20", /4\s*\/\s*20/.test(txt), true);
  expect("entrances widget lists both doors", txt.includes("North door") && txt.includes("South door"), true);
  expect("share widget: 75 %", txt.includes("75 %"), true);
  expect("speed widget without a section explains what to bind", txt.includes("vehicle_speed"), true);
  await shot("counting-widgets.png");

  // ---- scheduled reports
  await go("/settings/scheduled-reports", 3000);
  await ev(`document.querySelector('#newBtn').click(); true`); await sleep(1500);
  await ev(`document.querySelector('[data-p="0 7 * * 1"]').click(); true`); await sleep(1500);
  expect("preset fills the cron and the next runs", await ev(`document.querySelector('#srForm').cron.value + ' | ' + /\\d/.test(document.querySelector('#next').textContent)`), "0 7 * * 1 | true");
  await ev(`(() => { const f = document.querySelector('#srForm'); f.name.value = 'Probe weekly ${suf}'; f.type.value = 'counting'; f.type.dispatchEvent(new Event('change')); f.area_id.value = '${setup.area}'; f.period.value = 'previous_week'; f.addresses.value = 'probe-${suf}@example.com'; f.attach_pdf.checked = false; f.reason.value = 'headless probe'; f.requestSubmit(); return true; })()`); await sleep(2500);
  expect("report listed", await ev(`[...document.querySelectorAll('#list tbody tr')].some(r => r.textContent.includes('Probe weekly ${suf}') && r.textContent.includes('0 7 * * 1'))`), true);
  const rid = await ev(`ctrl32.api('GET', '/api/v1/scheduled-reports').then(r => r.reports.find(x => x.name === 'Probe weekly ${suf}').id)`);
  await ev(`document.querySelector('[data-send="${rid}"]').click(); true`); await sleep(5000);
  expect("sent now: a run in the delivery log with an Excel attachment", await ev(`!document.querySelector('#runsCard').hidden && [...document.querySelectorAll('#runs tbody tr')].length >= 1 && document.querySelector('#runs tbody tr').textContent.includes('.xlsx')`), true);
  await shot("counting-scheduled-reports.png");

  // clean up what is not data: the dashboard, the report, the camera
  await ev(`(async () => { const api = ctrl32.api; await api('DELETE', '/api/v1/dashboards/${dash}?reason=headless'); await api('DELETE', '/api/v1/scheduled-reports/${rid}?reason=headless'); await api('DELETE', '/api/v1/video/cameras/${setup.cam}?reason=headless'); return true; })()`);
  console.log("problems: " + (problems.length ? problems.join(" | ") : "none"));
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
