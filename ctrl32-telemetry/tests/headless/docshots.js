// Headless Chrome: screenshots of every screen for the product documentation. Not a test: it signs in, walks every
// page, opens the important dialogs and writes <section>-<what>.png to OUT. Run against a server with the full demo
// (bash deploy/dev/dev.sh up && camsim && demo; the demo includes deploy/dev/demo/docs.py). The VPN pictures need the
// server agent with [agent] vpn_simulate and a licence (see docs.py); without them they are reported and skipped.
//
//   BASE=http://127.0.0.1:8080 OUT=/path/to/shots [USER=admin PASS=admin1234] [UI_LANG=en] [ONLY=video] node tests/headless/docshots.js
//
// Desktop 1440×900 in the light theme, a few phone shots (390×844) and two in the dark theme. Native browser dialogs
// (alert, confirm, prompt) are disabled so nothing is changed by accident. Each shot is checked for an empty page or a
// visible error; problems are listed at the end (exit code 1).
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || "./docshots";
const user = process.env.USER_NAME || process.env.USER_ || "admin";
const pass = process.env.PASS || "admin1234";
const lang = process.env.UI_LANG || "en";
const only = process.env.ONLY || "";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = +(process.env.CDP_PORT || 9391); // another port when several runs share the PC
fs.mkdirSync(out, { recursive: true });
const profile = tmp + "/cdp-profile-docshots";
fs.rmSync(profile, { recursive: true, force: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--hide-scrollbars", "--autoplay-policy=no-user-gesture-required", "--remote-debugging-port=" + port, "--user-data-dir=" + profile, "--window-size=1440,900", "--lang=en-US", "--accept-lang=en-US", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }

(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = []; let exceptions = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") exceptions.push(msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n")[0]);
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: false });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  // an English browser: native widgets (file inputs, date pickers) and Accept-Language follow it
  await send("Emulation.setLocaleOverride", { locale: "en-US" });
  // no native dialogs: a stray click on a delete button cannot confirm anything
  await send("Page.addScriptToEvaluateOnNewDocument", { source: "window.alert = () => {}; window.confirm = () => false; window.prompt = () => null;" });
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.result ? r.result.result.value : undefined; };
  let W = 1440, H = 900, mobile = false;
  const viewport = async (w, h, mob) => { W = w; H = h; mobile = !!mob;
    await send("Emulation.setDeviceMetricsOverride", { width: w, height: h, deviceScaleFactor: mob ? 2 : 1, mobile: !!mob });
    await send("Emulation.setTouchEmulationEnabled", { enabled: !!mob }); };
  await viewport(1440, 900, false);

  const wanted = (name) => !only || only.split(",").some((s) => name.startsWith(s));
  const go = async (path, wait = 2500) => { exceptions = []; await send("Page.navigate", { url: base + path }); await sleep(wait); };
  const click = async (sel, wait = 1200) => { const ok = await ev(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); if (!e) return false; e.scrollIntoView({block: 'center'}); e.click(); return true; })()`); if (!ok) problems.push("no element " + sel + " on " + (await ev("location.pathname"))); await sleep(wait); return ok; };
  const done = [];
  // check: something rendered, no error box, no exception; then capture (optionally the whole page height or a clip)
  const shot = async (name, opt = {}) => {
    if (opt.top) { await ev("scrollTo(0, 0); true"); await sleep(500); }
    const info = await ev(`(() => {
      const txt = (document.body && document.body.innerText || '').trim();
      const errs = [...document.querySelectorAll('.error, .bad, [role=alert]')].filter(e => e.offsetParent && e.textContent.trim().length > 3 && !e.closest('dialog:not([open])')).map(e => e.textContent.trim().slice(0, 80));
      return { len: txt.length, errs, h: Math.max(document.documentElement.scrollHeight, document.body.scrollHeight), path: location.pathname };
    })()`);
    if (!info || info.len < 20) problems.push(name + ": page looks empty");
    if (info && info.errs.length && !opt.allowErrors) console.log("  note " + name + ": " + info.errs.slice(0, 2).join(" | "));
    if (exceptions.length) console.log("  note " + name + " exceptions: " + exceptions.slice(0, 2).join(" | "));
    let params = { format: "png" };
    if (opt.clip) {
      const pick = opt.clip.startsWith("js:") ? opt.clip.slice(3) : `document.querySelector(${JSON.stringify(opt.clip)})`;
      const r = await ev(`(() => { const e = ${pick}; if (!e) return null; e.scrollIntoView({block: 'center'}); const b = e.getBoundingClientRect(); return {x: b.x + scrollX, y: b.y + scrollY, width: b.width, height: b.height}; })()`);
      await sleep(400);
      if (r) { const p = opt.pad || 8; params = { format: "png", captureBeyondViewport: true, clip: { x: Math.max(0, r.x - p), y: Math.max(0, r.y - p), width: r.width + 2 * p, height: r.height + 2 * p, scale: 1 } }; }
      else { problems.push(name + ": no element " + opt.clip.slice(0, 80)); return; } // keep an earlier picture
    }
    const full = opt.full && info && info.h > H + 40;
    if (full) { await ev("scrollTo(0, 0); true"); await send("Emulation.setDeviceMetricsOverride", { width: W, height: Math.min(info.h, opt.maxH || 4200), deviceScaleFactor: mobile ? 2 : 1, mobile }); await sleep(opt.fullWait || 1500); }
    const s = await send("Page.captureScreenshot", params);
    if (full) await viewport(W, H, mobile);
    const file = out + "/" + name + ".png";
    fs.writeFileSync(file, Buffer.from(s.result.data, "base64"));
    done.push(name); console.log("shot " + name + (full ? " (full " + info.h + ")" : ""));
  };
  const page = async (name, path, opt = {}) => { if (!wanted(name)) return; await go(path, opt.wait || 2500); if (opt.before) { await ev(opt.before); await sleep(opt.afterWait || 600); } if (opt.click) for (const c of [].concat(opt.click)) await click(c, opt.clickWait || 1200); await shot(name, opt); };
  const setLocal = async (theme) => ev(`(() => { localStorage.setItem('ctrl32.locale', ${JSON.stringify(lang)}); ${theme ? `localStorage.setItem('ctrl32.theme', ${JSON.stringify(theme)});` : "localStorage.removeItem('ctrl32.theme');"} localStorage.setItem('ctrl32.sidebar', 'open'); return true; })()`);

  // ---- signed out
  await go("/login", 1500); await setLocal("light");
  await page("start-sign-in", "/login");
  await page("start-front-page", "/", { wait: 3000 });
  await page("admin-license-page", "/license");
  await page("start-forgot-password", "/forgot");
  await page("start-register", "/register");
  if (wanted("start-phone-sign-in")) { await viewport(390, 844, true); await page("start-phone-sign-in", "/login"); await viewport(1440, 900, false); }

  // ---- sign in
  const login = await ev(`fetch('/api/v1/auth/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({username: ${JSON.stringify(user)}, password: ${JSON.stringify(pass)}})}).then(r => r.status)`);
  if (login !== 200) { console.error("sign-in failed: " + login); proc.kill(); process.exit(1); }
  const api = (p) => ev(`fetch(${JSON.stringify(p)}, {credentials: 'same-origin'}).then(r => r.ok ? r.json() : null)`);
  const dashes = (await api("/api/v1/dashboards")) || [];
  const walls = (await api("/api/v1/video/walls")) || [];
  const cams = (await api("/api/v1/video/cameras")) || [];
  const devices = (await api("/api/v1/devices")) || [];
  const flows = (await api("/api/v1/flows")) || [];
  const short = { "site-cameras-on-the-floor-plan": "floor-plan", "line-1-process-screen": "scada-line-1", "production-line-overview": "production-lines",
    "cold-chain-medicine-store": "cold-chain", "energy-monthly-consumption": "energy-monthly", "overview": "overview", "showcase-all-widgets": "showcase",
    "overview-of-all-sensors": "sensors-overview", "energy-overview": "energy-overview", "cold-chain-overview": "cold-chain-overview" };
  const slugName = (s) => short[s] || s;
  const prodDash = () => dashes.find((d) => d.slug === "production-line-overview") || dashes[0];
  const cam = (n) => (cams.find((c) => c.name === n) || cams[0] || {}).id;
  const wallCR = (walls.find((w) => w.name.startsWith("Control room")) || walls[0] || {}).id;
  const wallGate = (walls.find((w) => w.name === "Gatehouse") || walls[1] || {}).id;
  const dev = (ext) => (devices.find((d) => d.external_id === ext) || devices[0] || {}).id;

  // ---- start
  await page("start-home", "/", { wait: 5000 });
  await page("start-sidebar-icons", "/", { wait: 1500, before: "localStorage.setItem('ctrl32.sidebar', 'icons'); location.reload(); true", afterWait: 4000 });
  await ev("localStorage.setItem('ctrl32.sidebar', 'open'); true");
  await page("admin-api-docs", "/api/docs", { wait: 3000 });

  // ---- dashboards
  await page("dashboards-list", "/dashboards");
  await page("dashboards-new-dialog", "/dashboards", { click: "#newBtn" });
  for (const d of dashes) {
    const n = slugName(d.slug);
    await page(`dashboards-${n}`, `/dashboards/${d.slug}`, { wait: 6000, full: ["showcase", "cold-chain", "energy-monthly", "sensors-overview", "energy-overview", "cold-chain-overview"].includes(n), maxH: 3200 });
    await page(`dashboards-${n}-edit`, `/dashboards/${d.slug}/edit`, { wait: 5000 });
  }
  const cold = dashes.find((d) => d.slug === "cold-chain-medicine-store");
  if (cold) {
    await page("dashboards-edit-add-widget", `/dashboards/${cold.slug}/edit`, { wait: 4000, click: "#eAdd" });
    await page("dashboards-edit-widget-properties", `/dashboards/${cold.slug}/edit`, { wait: 4000, before: "(() => { const r = document.querySelectorAll('.dash-resize')[1]; if (r) r.parentElement.click(); return !!r; })()", afterWait: 1500 });
    await page("dashboards-edit-settings", `/dashboards/${cold.slug}/edit`, { wait: 4000, click: "#eSettings" });
    // annotations in a chart widget (0.72.0; docs.py records: fridge 2): the pointer over the annotated period
    if (wanted("dashboards-chart-annotations")) {
      await go(`/dashboards/${cold.slug}`, 6000);
      const hov = await ev(`(() => { const a = document.querySelector('[data-id="ch"] svg .ann'); if (!a) return false; const svg = a.closest('svg'); const r = a.getBoundingClientRect(); svg.dispatchEvent(new MouseEvent('mousemove', {clientX: r.left + Math.max(2, r.width / 2), clientY: r.top + 30, bubbles: true})); return true; })()`);
      if (hov) await shot("dashboards-chart-annotations", { clip: '[data-id="ch"]' }); else problems.push("dashboards-chart-annotations: no annotation in the chart (docs.py records)");
    }
    await page("dashboards-period-table-widget", `/dashboards/${cold.slug}`, { wait: 6000, clip: "js:document.querySelector('.pt-bar').parentElement.parentElement" });
    // the toolbar alone: the period, ◀ ▶ and the Excel / CSV / PDF export buttons
    if (wanted("dashboards-period-table-toolbar")) { await go(`/dashboards/${cold.slug}`, 6000); await shot("dashboards-period-table-toolbar", { clip: ".pt-bar", pad: 12 }); }
  }
  // overviews (0.68.0): the filter bar with a choice open, the overview table with its column filters, the totals of a
  // grouped table, the data source of a widget and the filters of the dashboard settings in the editor, the global search
  const ccOv = dashes.find((d) => d.slug === "cold-chain-overview"), sensOv = dashes.find((d) => d.slug === "overview-of-all-sensors"), enOv = dashes.find((d) => d.slug === "energy-overview");
  if (ccOv) {
    if (wanted("dashboards-filter-bar")) { await go(`/dashboards/${ccOv.slug}`, 6000); await ev("(() => { const d = document.querySelectorAll('#vFilters .df-pick')[2]; if (d) d.open = true; return !!d; })()"); await sleep(800);
      await shot("dashboards-filter-bar", { clip: "js:(() => { const b = document.querySelector('#vFilters'); const m = document.querySelector('#vFilters details[open] .df-menu'); if (!b) return null; return { getBoundingClientRect: () => { const r = b.getBoundingClientRect(), q = m ? m.getBoundingClientRect() : r; return { x: r.x, y: r.y, width: r.width, height: Math.max(r.bottom, q.bottom) - r.y + 4 }; }, scrollIntoView: () => {} }; })()" }); }
    await page("dashboards-edit-data-source", `/dashboards/${ccOv.slug}/edit`, { wait: 5000, before: "(() => { const w = document.querySelector('#eGrid .dash-widget[data-id=\"trend\"]'); if (w) w.click(); return !!w; })()", afterWait: 2500, clip: "#eProps", pad: 6 });
    await page("dashboards-edit-filters", `/dashboards/${ccOv.slug}/edit`, { wait: 5000, click: "#eSettings", clip: "#setDlg", pad: -2 });
  }
  if (sensOv && wanted("dashboards-overview-table")) {
    await go(`/dashboards/${sensOv.slug}?f.quantity=temperature`, 6000);
    await ev("(() => { const b = document.querySelector('[data-id=\"sensors\"] .ot-ftoggle'); if (b) b.click(); return !!b; })()"); await sleep(600);
    await ev("(() => { const s = document.querySelector('[data-id=\"sensors\"] .ot-filters [data-f=\"alarm\"]'); if (s) { s.value = 'alarm'; s.dispatchEvent(new Event('change')); } return !!s; })()"); await sleep(600);
    await shot("dashboards-overview-table", { clip: "[data-id=\"sensors\"]", pad: 4 });
  }
  if (enOv && wanted("dashboards-overview-group-totals")) { await go(`/dashboards/${enOv.slug}`, 7000); await shot("dashboards-overview-group-totals", { clip: "[data-id=\"meters\"]", pad: 4 }); }
  if (wanted("start-global-search")) {
    await go(ccOv ? `/dashboards/${ccOv.slug}` : "/", 5000);
    await ev("ctrl32.openSearch('fridge'); true"); await sleep(2500);
    await shot("start-global-search");
  }
  if (sensOv && wanted("dashboards-public-filters")) {
    // the public link as a visitor sees it: signed out, only the filters marked public
    const sh = await api(`/api/v1/dashboards/${sensOv.slug}/share`);
    if (sh && sh.token) {
      await ev("ctrl32.api('POST', '/api/v1/auth/logout', {}).then(() => true, () => true)"); await sleep(500);
      await page("dashboards-public-filters", `/p/${sh.token}?f.quantity=temperature`, { wait: 7000 });
      const relog = await ev(`fetch('/api/v1/auth/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({username: ${JSON.stringify(user)}, password: ${JSON.stringify(pass)}})}).then(r => r.status)`);
      if (relog !== 200) problems.push("sign-in again after the public link failed: " + relog);
    } else problems.push("dashboards-public-filters: the overview has no public link");
  }

  // ---- devices
  await page("devices-list", "/devices", { wait: 3000 });
  await page("devices-new-dialog", "/devices", { wait: 3000, click: "#newBtn" });
  const d2 = dev("esp32-demo"); // an MQTT device: every tab (an HTTP device has the overview only)
  for (const tab of ["overview", "datastreams", "commands", "attributes", "console", "messages"]) await page(`devices-device-${tab}`, `/devices/${d2}/${tab}`, { wait: 3500 });
  // the Telemetry tab (0.67.0) of logger T2: a datastream, keys without a datastream (docs.py records) and status keys
  const t2 = dev("logger-t2");
  const telRow = (row) => `(() => { const tr = document.querySelector('#telTable tr[data-row="${row}"]'); if (tr) tr.click(); return !!tr; })()`;
  await page("devices-device-telemetry", `/devices/${t2}/telemetry`, { wait: 4000 });
  await page("devices-device-telemetry-chart", `/devices/${t2}/telemetry`, { wait: 4000, before: telRow("datastream:temp"), afterWait: 3000, clip: "#telDetail", pad: 4 });
  await page("devices-device-telemetry-unassigned", `/devices/${t2}/telemetry`, { wait: 4000, before: telRow("unassigned:door"), afterWait: 3000, clip: "#telDetail", pad: 4 });
  await page("devices-sites", "/assets/sites");
  await page("devices-assets", "/assets", { wait: 3000 });
  await page("devices-new-asset-dialog", "/assets", { wait: 3000, click: "#newAssetBtn" });
  await page("devices-home", "/home", { wait: 4000 });
  for (const s of ["add", "inbox", "provisioning", "sources"]) await page(`devices-smarthome-${s}`, `/smarthome#${s}`, { wait: 3000 });
  await page("devices-firmware", "/fota");
  await page("devices-firmware-upload-dialog", "/fota", { click: "#uploadBtn" });
  await page("devices-firmware-campaigns", "/fota/campaigns");
  await page("devices-displays", "/displays", { wait: 3000 });
  await page("devices-displays-pair-dialog", "/displays", { wait: 3000, click: "#pairBtn" });

  // ---- automation, alarms, incidents
  await page("automation-alarm-rules", "/assets/rules");
  // the dialog alone, for a datastream of the cold chain (test runs against the server may leave other assets behind)
  await page("automation-alarm-rule-editor", "/assets/rules", { click: "#newRuleBtn", clip: "#ruleDlg", pad: -4,
    before: "setTimeout(() => { const f = document.querySelector('#ruleNew'); const o = f && [...f.datastream_id.options].find((x) => x.textContent.startsWith('Medicine fridge 2 / temp')); if (o) f.datastream_id.value = o.value; f.warning.value = '7.5'; f.action.value = '8'; f.hysteresis.value = '0.2'; f.reason.value = 'Limits per SOP-12'; }, 1000); true" });
  // the datastream's "New rule version" form, filled from the current version (medicine fridge 2)
  if (wanted("automation-rule-version")) {
    const dss = (await api("/api/v1/datastreams")) || [];
    const f2 = dss.find((d) => /fridge 2/i.test(d.asset || "") && d.key === "temp") || dss.find((d) => d.unit) || dss[0];
    await go("/assets/rules", 3000);
    if (f2) { await ev(`openDS(${JSON.stringify(f2.id)})`); await sleep(3000); await ev("(() => { const f = document.querySelector('#ruleForm'); if (!f) return false; f.type.value = 'high'; f.type.dispatchEvent(new Event('change')); return true; })()"); await sleep(500); }
    await shot("automation-rule-version", { clip: "#ruleForm", pad: 12 });
  }
  // the Alarms widget with the Acknowledge button (the cold chain dashboard; needs an active alarm)
  if (wanted("automation-alarms-acknowledge")) {
    const cd = dashes.find((d) => d.slug === "cold-chain-medicine-store");
    if (cd) {
      await go(`/dashboards/${cd.slug}`, 6000);
      // the widget opened in its full view (the dashboard column is narrow): head, table header and the active rows
      // (with the button); older rows are left out
      await ev("(() => { const a = document.querySelector('[data-ack]'); const w = a && a.closest('.dash-widget'); const b = w && w.querySelector('.dw-fs'); if (b) b.click(); return !!b; })()"); await sleep(2500);
      await shot("automation-alarms-acknowledge", { clip: "js:(() => { const d = document.querySelector('dialog.dw-full'); const b = d ? [...d.querySelectorAll('[data-ack]')] : []; if (!b.length) return null; const last = b[b.length - 1].closest('tr').getBoundingClientRect(); return { getBoundingClientRect: () => { const r = d.getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: last.bottom - r.top + 8 }; }, scrollIntoView: () => {} }; })()", pad: 0 });
    }
  }
  await page("automation-rules", "/assets/automation", { wait: 3000 });
  await page("automation-rule-editor", "/assets/automation", { wait: 3000, click: "#newAutoBtn" });
  await page("automation-incidents", "/incidents", { wait: 3000 });
  await page("automation-incident-new-dialog", "/incidents", { wait: 3000, click: "#newBtn" });
  await page("automation-incident-detail", "/incidents", { wait: 3000, before: "(() => { const r = [...document.querySelectorAll('#list tbody tr[data-id]')].find(t => /temperature above 8/.test(t.textContent)); if (r) r.click(); return !!r; })()", afterWait: 1500 });
  await page("automation-flows", "/flows", { wait: 3000 });
  await page("automation-flows-new-dialog", "/flows", { click: "#newBtn" });
  // the demo server has no SMTP: delivery errors of the flows' e-mail nodes are left out of the debug panel
  const quietDebug = "(() => { const l = document.querySelector('#dbgList'); if (!l) return false; [...l.children].forEach((r) => { if (r.querySelector('.badge.bad')) r.remove(); }); const i = document.querySelector('#dbgInfo'); if (i) i.textContent = i.textContent.replace(/\\d+/, l.children.length); return true; })()";
  const flow = (flows.find((f) => f.flow.slug.startsWith("fridge")) || flows[0] || {}).flow;
  for (const f of flows) {
    const n = f.flow.slug.startsWith("fridge") ? "cold-chain" : f.flow.slug.startsWith("energ") ? "energy" : f.flow.slug;
    await page(`automation-flow-editor-${n}`, `/flows/${f.flow.slug}`, { wait: 4000, before: quietDebug });
  }
  if (flow) {
    await page("automation-flow-change-reason", `/flows/${flow.slug}`, { wait: 4000, before: quietDebug, click: "#tSave" });
    await page("automation-flow-versions", `/flows/${flow.slug}`, { wait: 4000, before: quietDebug, click: "#tVersions" });
    if (wanted("automation-flow-node-settings")) {
      await go(`/flows/${flow.slug}`, 4000);
      const r = await ev(`(() => { const n = document.querySelector('.fe-node[data-id="hot"]') || document.querySelector('.fe-node'); if (!n) return null; const b = n.getBoundingClientRect(); return {x: b.x + b.width / 2, y: b.y + b.height / 2}; })()`);
      if (r) { await send("Input.dispatchMouseEvent", { type: "mousePressed", x: r.x, y: r.y, button: "left", clickCount: 1 }); await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: r.x, y: r.y, button: "left", clickCount: 1 }); await sleep(1200); }
      await ev(quietDebug);
      await shot("automation-flow-node-settings");
    }
    // the Incident node with its incident key
    if (wanted("automation-flow-incident-node")) {
      await go(`/flows/${flow.slug}`, 4000);
      const r = await ev(`(() => { const n = document.querySelector('.fe-node[data-id="inc"]'); if (!n) return null; const b = n.getBoundingClientRect(); return {x: b.x + b.width / 2, y: b.y + b.height / 2}; })()`);
      if (r) { await send("Input.dispatchMouseEvent", { type: "mousePressed", x: r.x, y: r.y, button: "left", clickCount: 1 }); await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: r.x, y: r.y, button: "left", clickCount: 1 }); await sleep(1200); }
      else problems.push("automation-flow-incident-node: no Incident node in " + flow.slug);
      await ev(quietDebug);
      await shot("automation-flow-incident-node");
    }
  }

  // ---- energy
  await page("energy-day", "/energy", { wait: 4000 });
  for (const p of ["week", "month", "year"]) await page(`energy-${p}`, "/energy", { wait: 3500, click: `#period [data-p=${p}]`, clickWait: 3000 });
  await page("energy-settings-dialog", "/energy", { wait: 3500, click: "#setBtn" });

  // ---- video
  await page("video-live", "/video", { wait: 9000 });
  await page("video-walls", "/video/walls", { wait: 3000 });
  if (wallCR) await page("video-wall-control-room", `/video/walls/${wallCR}`, { wait: 10000 });
  if (wallGate) await page("video-wall-gatehouse", `/video/walls/${wallGate}`, { wait: 9000 });
  await page("video-manage", "/video/manage", { wait: 3000, full: true, maxH: 3600 });
  await page("video-add-camera-dialog", "/video/manage", { wait: 3000, click: "#addCam" });
  await page("video-evidence", "/video/evidence", { wait: 3000 });
  await page("video-settings", "/video/settings", { wait: 3000 });
  const gate = cam("Gate"), rec = cam("Car park north");
  if (gate) {
    await page("video-camera-live", `/video/cameras/${cam("Hall 1")}`, { wait: 8000 });
    // a camera with PTZ: the pad and zoom buttons are on the picture (shown by default on wide screens)
    await page("video-camera-ptz", `/video/cameras/${gate}`, { wait: 8000 });
  }
  if (rec && wanted("video-camera-playback")) {
    await go(`/video/cameras/${rec}`, 8000);
    await click("#bBack60", 6000);
    await shot("video-camera-playback", { full: true, maxH: 1800 });
    // select a part of the hour on the detail timeline for download
    const r = await ev(`(() => { const e = document.querySelector('#tl2'); if (!e) return null; e.scrollIntoView({block: 'center'}); const b = e.getBoundingClientRect(); return {x: b.x, y: b.y + b.height / 2, w: b.width}; })()`);
    if (r) {
      await sleep(400);
      const x1 = r.x + r.w * 0.4, x2 = r.x + r.w * 0.55;
      await send("Input.dispatchMouseEvent", { type: "mousePressed", x: x1, y: r.y, button: "left", clickCount: 1 });
      for (let i = 1; i <= 8; i++) await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: x1 + ((x2 - x1) * i) / 8, y: r.y, button: "left", buttons: 1 });
      await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: x2, y: r.y, button: "left", clickCount: 1 });
      await sleep(2500);
      await shot("video-camera-export-selection", { clip: "#recPanel" });
    }
  }

  // ---- administration
  await page("admin-users", "/users", { wait: 3000 });
  await page("admin-new-user-dialog", "/users", { wait: 3000, click: "#newUserBtn" });
  await page("admin-roles", "/users/roles", { wait: 3000, full: true, maxH: 3600 });
  // audit trail, reviews and report signatures (0.67.0); the administrator reads them, the dialogs of the quality
  // reviewer are opened by script (the administrator has neither audit.review nor report.sign)
  await page("admin-audit-trail", "/audit", { wait: 3500 });
  await page("admin-audit-reviews", "/audit/reviews", { wait: 3000 });
  await page("admin-audit-review-dialog", "/audit", { wait: 3500, before: "(() => { document.querySelector('#reviewPeriodBtn').hidden = false; document.querySelector('#reviewPeriodBtn').click(); return true; })()", afterWait: 800, clip: "#reviewDlg", pad: -4 });
  await page("admin-audit-report-signatures", "/audit/reports", { wait: 3000 });
  await page("admin-audit-sign-dialog", "/audit/reports", { wait: 3000, before: "(() => { const f = document.querySelector('#signForm'); f.dataset.id = 'x'; document.querySelector('#signWhat').textContent = 'observations · ' + new Date().toLocaleString() + ' · SHA-256 3f9a1c0e5b7d2a64…'; document.querySelector('#signDlg').showModal(); return true; })()", afterWait: 800, clip: "#signDlg", pad: -4 });
  await page("admin-audit-signatures-dialog", "/audit/reports", { wait: 3000, before: "(() => { const b = [...document.querySelectorAll('[data-sigs]')].find((x) => x.closest('tr').textContent.includes('Reviewed')) || document.querySelector('[data-sigs]'); if (b) b.click(); return !!b; })()", afterWait: 1500, clip: "#sigDlg", pad: -4 });
  await page("admin-new-role-dialog", "/users/roles", { wait: 3000, click: "#newRoleBtn" });
  // the server administrator's own account: server_admin and org_admin, both locked for oneself (0.65.0)
  if (wanted("admin-user-server-admin")) {
    await go("/users", 3000);
    await ev(`(() => { const r = [...document.querySelectorAll('#users tbody tr')].find((t) => t.querySelector('b') && t.querySelector('b').textContent === ${JSON.stringify(user)}); if (r) r.click(); return !!r; })()`); await sleep(1800);
    await shot("admin-user-server-admin", { clip: "#userDlg", pad: -6 });
  }
  for (const s of ["branding", "templates", "reports", "connectors", "languages", "translations", "apps", "export", "notifications", "approvals", "policy"]) {
    await page(`admin-settings-${s}`, `/settings/${s}`, { wait: 3500, full: s === "branding" || s === "templates", maxH: 3200 });
  }
  await page("admin-settings-new-channel-dialog", "/settings/notifications", { wait: 3000, click: "#newChanBtn" });
  await page("admin-settings-new-app-dialog", "/settings/apps", { wait: 3000, click: "#newAppBtn" });
  await page("admin-settings-new-connector-dialog", "/settings/connectors", { wait: 3000, click: "#newConnBtn" });
  await page("admin-organizations", "/organizations", { wait: 3000 });
  await page("admin-server", "/server", { wait: 4000 });
  for (const s of ["domain", "backups", "redundancy", "updates", "license"]) await page(`admin-server-${s}`, `/server/${s}`, { wait: 3500 });
  // the licence (0.66.0): the status card (updates included until, cameras), the activation card, the Updates card
  await page("licence-status", "/server/license", { wait: 3500, clip: "js:document.querySelector('#kvLic').closest('.card')", pad: -1 });
  // the activation form only (the documentation shows no prices, so not the buy button above it)
  await page("licence-activate", "/server/license", { wait: 3500, clip: "#licActForm", pad: 10 });
  // the rows Installed (with the release date) and Updates (included until) of System → Updates
  await page("admin-updates", "/server/updates", { wait: 3500, pad: 10, clip: "js:(() => { const k = document.querySelector('#kvUpd'); if (!k || k.children.length < 4) return null; const a = k.children[0], b = k.children[3]; return { scrollIntoView() { k.scrollIntoView({block: 'center'}); }, getBoundingClientRect() { const r = k.getBoundingClientRect(), r1 = a.getBoundingClientRect(), r2 = b.getBoundingClientRect(); return { x: r.x, y: r1.y, width: r.width, height: r2.bottom - r1.y }; } }; })()" });
  // Camera management on a server without a licence: the page head, the camera count of 4 and the first rows
  // (run with ONLY=video-manage-limit while no licence is installed; with a licence the count is not shown)
  await page("video-manage-limit", "/video/manage", { wait: 3500, clip: "js:(() => { const a = document.querySelector('.page-head'), b = document.querySelector('#cams'); if (!a || !b || document.querySelector('#camLimit').hidden) return null; return { scrollIntoView() {}, getBoundingClientRect() { const r1 = a.getBoundingClientRect(), r2 = b.getBoundingClientRect(); return { x: r1.x, y: r1.y, width: r2.right - r1.x, height: r2.bottom - r1.y }; } }; })()", pad: 8 });

  // ---- camera list: export and import (spec 028). Nothing is imported: the preview only checks the file.
  // the buttons above the camera table with the page head and the first rows
  await page("video-camera-list", "/video/manage", { wait: 3500, pad: 8, clip: "js:(() => { const a = document.querySelector('.page-head'), b = document.querySelector('#cams tbody tr:nth-child(5)'); if (!a || !b) return null; return { scrollIntoView() {}, getBoundingClientRect() { const r1 = a.getBoundingClientRect(), r2 = b.getBoundingClientRect(), t = document.querySelector('#cams').closest('.table-wrap').getBoundingClientRect(); return { x: r1.x, y: r1.y, width: t.right - r1.x, height: r2.bottom - r1.y }; } }; })()" });
  // the export with passwords: the dialog with the warning (the password field stays empty)
  await page("video-camera-list-passwords", "/video/manage", { wait: 3000, click: "#invCred", clip: "#credDlg", pad: 0, allowErrors: true });
  // an import file as people make it from an export: a new ip, a longer retention, a new camera by make and ip, one
  // unchanged row; and the same with a row that has an error
  const camId = (n) => (cams.find((c) => c.name === n) || {}).id || "";
  const impCSV = (withError) => ["id;name;ip;make;username;retention_days;location",
    `${camId("Hall 1")};Hall 1;10.20.0.31;;;;`, `${camId("Gate")};Gate;;;;30;`, `${camId("Gatehouse")};Gatehouse;;;;;`,
    ";Warehouse door;192.168.1.70;tapo;camera-account;14;Hall B"].concat(withError ? [";Loading bay;;;;0;Dock"] : []).join("\n") + "\n";
  const chooseFile = (csv) => `(() => { document.querySelector('#invImport').click(); const i = document.querySelector('#impForm').file; const dt = new DataTransfer(); dt.items.add(new File([${JSON.stringify(csv)}], 'cameras.csv', {type: 'text/csv'})); i.files = dt.files; i.dispatchEvent(new Event('change')); return true; })()`;
  if (wanted("video-camera-import")) {
    await go("/video/manage", 3000); await ev(chooseFile(impCSV(false))); await sleep(3000);
    await ev("document.querySelector('#impForm').reason.value = 'cameras restored after the server was replaced'; true");
    await shot("video-camera-import-preview", { clip: "#impDlg", pad: 0 });
    await go("/video/manage", 3000); await ev(chooseFile(impCSV(true))); await sleep(3000);
    await shot("video-camera-import-errors", { clip: "#impDlg", pad: 0, allowErrors: true });
  }
  // the exported Excel file, drawn as a spreadsheet grid from the same export (CSV has the same cells): the header
  // block, the column names and the first cameras of the sheet Cameras
  if (wanted("video-camera-list-excel")) {
    await go("/video/manage", 2000);
    const text = (await ev("fetch('/api/v1/video/inventory?format=csv').then(r => r.text())")) || "";
    const split = (l) => { const o = []; let cur = "", q = false; for (const ch of l) { if (ch === '"') { q = !q; continue; } if (ch === ";" && !q) { o.push(cur); cur = ""; } else cur += ch; } o.push(cur); return o; };
    const lines = text.replace(/^﻿/, "").split(/\r?\n/).filter(Boolean);
    const meta = lines.filter((l) => /^"?#/.test(l)).map((l) => { const t = l.replace(/^"?# /, "").replace(/"$/, ""); const i = t.indexOf(": "); return [t.slice(0, i), t.slice(i + 2)]; });
    const rows = lines.filter((l) => !/^"?#/.test(l)).map(split);
    const widths = [270, 130, 120, 110, 64, 84, 80, 90, 300, 300, 70];
    const grid = [...meta, [], ...rows.slice(0, 14)];
    const head = meta.length + 2;
    const esc = (v) => String(v || "").replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]);
    let t = '<table><colgroup><col style="width:42px">' + widths.map((w) => `<col style="width:${w}px">`).join("") + "</colgroup>";
    t += '<tr class="hd"><th></th>' + widths.map((w, i) => `<th>${"ABCDEFGHIJK"[i]}</th>`).join("") + "</tr>";
    grid.forEach((r, i) => { t += `<tr${i + 1 === head ? ' class="cols"' : ""}><th>${i + 1}</th>` + widths.map((w, c) => { const v = r[c] || ""; const b = (i < meta.length && c === 0) || i + 1 === head; const n = /^\d+$/.test(v) && i + 1 > head; return `<td class="${b ? "b" : ""}${n ? " n" : ""}">${esc(v)}</td>`; }).join("") + "</tr>"; });
    t += "</table>";
    const css = "body{margin:0;font:13px Calibri,Carlito,'Segoe UI',Arial,sans-serif;background:#fff;color:#000}" +
      ".bar{display:flex;gap:10px;align-items:center;padding:7px 12px;border-bottom:1px solid #d4d4d4;background:#f3f3f3}.bar b{font-weight:600}" +
      ".fx{padding:5px 12px;border-bottom:1px solid #d4d4d4;color:#333}.fx span{display:inline-block;width:60px;color:#666;border-right:1px solid #d4d4d4;margin-right:10px}" +
      "table{border-collapse:collapse;table-layout:fixed}td,th{border:1px solid #e1e1e1;height:21px;padding:0 5px;white-space:nowrap;overflow:hidden;text-align:left}" +
      "th{background:#f3f3f3;color:#555;font-weight:400;text-align:center}td.b{font-weight:700}td.n{text-align:right}tr.cols td{border-bottom:2px solid #9a9a9a}" +
      ".tabs{position:fixed;bottom:0;left:0;right:0;display:flex;gap:2px;padding:0 12px;border-top:1px solid #d4d4d4;background:#f3f3f3}.tabs span{padding:6px 16px;color:#444}" +
      ".tabs span.on{background:#fff;font-weight:600;border-bottom:2px solid #217346;color:#217346}";
    const html = `<!doctype html><html><head><meta charset="utf-8"><title>cameras.xlsx</title><style>${css}</style></head><body>` +
      `<div class="bar"><b>cameras_demo.xlsx</b><span>— the sheet Cameras of the exported file</span></div><div class="fx"><span>A${head}</span>id</div>${t}` +
      '<div class="tabs"><span class="on">Cameras</span><span>Read me</span></div></body></html>';
    await send("Page.navigate", { url: "data:text/html;charset=utf-8," + encodeURIComponent(html) }); await sleep(1000);
    await shot("video-camera-list-excel");
  }

  // ---- VPN (System → VPN). Needs the server agent; on a PC its VPN simulation ([agent] vpn_simulate) and, for the
  // site, a licence; the demo content comes from deploy/dev/demo/docs.py. The simulation's note and badge are hidden
  // on the pictures (they describe the development PC, not the product). Two peers are created through the dialog to
  // show their configuration once, and removed again afterwards.
  const wantVpn = !only || only.split(",").some((s) => s.startsWith("vpn") || "vpn-".startsWith(s));
  const vpnState = wantVpn ? await api("/api/v1/vpn") : null;
  if (vpnState && vpnState.supported) {
    const noSim = "(() => { const fix = () => { const n = document.querySelector('#note'); if (n && !n.hidden && /Simulation/.test(n.textContent)) n.hidden = true; document.querySelectorAll('#kvServer .badge').forEach((b) => { if (/simulation/i.test(b.textContent)) b.remove(); }); document.querySelectorAll('#kvServer dd, #kvUplink dd').forEach((d) => { if (d.textContent.trim() === 'simulation' && d.previousElementSibling) { d.previousElementSibling.remove(); d.remove(); } }); }; fix(); if (!window.__noSim) { window.__noSim = new MutationObserver(fix); window.__noSim.observe(document.body, { subtree: true, childList: true }); } return true; })()";
    const vpnGo = async () => { await go("/server/vpn", 3500); await ev(noSim); await sleep(300); };
    const created = [];
    if (wanted("vpn-page")) { await vpnGo(); await shot("vpn-page", { full: true, maxH: 2400 }); }
    if (wanted("vpn-server-settings")) { await vpnGo(); await click("#settingsBtn"); await shot("vpn-server-settings"); }
    // the add-peer dialog per kind, filled as an administrator would
    const fillPeer = (kind, name, extra) => `(() => { document.querySelector('#addPeerBtn').click(); document.querySelector('#kindSeg [data-kind=${kind}]').click(); const f = document.querySelector('#peerForm');
      f.name.value = ${JSON.stringify(name)}; ${extra || ""} f.reason.value = 'New peer for the documentation'; f.password.value = 'admin1234'; return true; })()`;
    const peerKinds = [["user", "Service tablet - M. Reed", "f.note.value = 'Refrigeration service';"],
      ["device", "Energy meter gateway", "f.note.value = 'Modbus gateway in the switch room';"],
      ["site", "Warehouse Senec - router", "f.subnets.value = '192.168.20.0/24'; f.note.value = 'Teltonika RUT241';"]];
    for (const [kind, name, extra] of peerKinds) {
      if (!wanted(`vpn-add-${kind}`)) continue;
      await vpnGo(); await ev(fillPeer(kind, name, extra)); await sleep(800);
      await shot(`vpn-add-${kind}`);
    }
    // the configuration shown once: a technician (QR code and .conf), a site (router configuration in three tabs)
    const submitPeer = async () => { await ev("document.querySelector('#peerSubmit').click(); true"); await sleep(3000); await ev(noSim); const n = await ev("(() => { const t = document.querySelector('#confText'); return t ? t.textContent.match(/^# (.*)/m) : null; })()"); return n; };
    if (wanted("vpn-peer-config")) {
      await vpnGo(); await ev(fillPeer("user", "Service tablet - M. Reed", "f.note.value = 'Refrigeration service';")); await sleep(500);
      await submitPeer(); created.push("Service tablet - M. Reed");
      await ev("document.querySelector('#peerResult').scrollIntoView({block: 'start'}); true"); await sleep(400);
      await shot("vpn-peer-config", { clip: "#peerDlg", pad: -6 });
      // the app files of a technician (0.65.0): Windows and phones
      for (const tpl of ["windows", "mobile"]) {
        await ev(`(() => { const b = document.querySelector('[data-tpl=${tpl}]'); if (!b) return false; b.click(); document.querySelector('#tplText').style.maxHeight = 'none'; document.querySelector('#tplTabs').scrollIntoView({block: 'start'}); return true; })()`); await sleep(500);
        await shot(`vpn-client-${tpl}`, { clip: "js:(() => { const a = document.querySelector('#tplTabs').previousElementSibling, b = document.querySelector('#tplText').nextElementSibling; const r1 = a.getBoundingClientRect(), r2 = b.getBoundingClientRect(); return { getBoundingClientRect: () => ({ x: r1.x, y: r1.y, width: Math.max(r1.width, r2.width), height: r2.bottom - r1.top }), scrollIntoView: () => {} }; })()", pad: 8 });
      }
    }
    if (wanted("vpn-router")) {
      await vpnGo(); await ev(fillPeer("site", "Warehouse Senec - router", "f.subnets.value = '192.168.20.0/24'; f.note.value = 'Teltonika RUT241';")); await sleep(500);
      await submitPeer(); created.push("Warehouse Senec - router");
      for (const tpl of ["wg_quick", "openwrt", "mikrotik", "rutos", "edgerouter", "pfsense", "opnsense"]) {
        await ev(`(() => { document.querySelector('[data-tpl=${tpl}]').click(); document.querySelector('#tplText').style.maxHeight = 'none'; document.querySelector('#tplTabs').scrollIntoView({block: 'start'}); return true; })()`); await sleep(500);
        await shot(`vpn-router-${tpl.replace("_", "-")}`, { clip: "js:(() => { const a = document.querySelector('#tplTabs').previousElementSibling, b = document.querySelector('#tplText').nextElementSibling; const r1 = a.getBoundingClientRect(), r2 = b.getBoundingClientRect(); return { getBoundingClientRect: () => ({ x: r1.x, y: r1.y, width: Math.max(r1.width, r2.width), height: r2.bottom - r1.top }), scrollIntoView: () => {} }; })()", pad: 8 });
      }
    }
    // remove the peers created for the pictures (with a reason, audited like any removal)
    if (created.length) {
      const st2 = await api("/api/v1/vpn");
      for (const n of created) {
        const p = ((st2 && st2.peers) || []).find((x) => x.name === n);
        if (p) await ev(`ctrl32.api('POST', '/api/v1/vpn/peers/${p.id}/remove', {reason: 'peer created for the documentation pictures'}).then(() => true)`);
      }
    }
    // access rules: the PLC rule of the demo, and a new rule towards this server with the service presets
    if (wanted("vpn-rule-plc")) {
      await vpnGo();
      await ev("(() => { const r = [...document.querySelectorAll('#rules tbody tr')].find((t) => /PLC/.test(t.textContent)); if (r) r.querySelector('[data-redit]').click(); return !!r; })()"); await sleep(800);
      await ev("(() => { const f = document.querySelector('#ruleForm'); f.reason.value = 'Commissioning of line 1'; return true; })()");
      await shot("vpn-rule-plc", { clip: "#ruleDlg", pad: -6 });
    }
    if (wanted("vpn-rule-server")) {
      await vpnGo(); await click("#addRuleBtn", 800);
      await ev("(() => { const f = document.querySelector('#ruleForm'); const c = document.querySelector('#srcList input[data-st=kind][data-sv=user]'); if (c) c.checked = true; const p = document.querySelector('[data-preset]'); if (p) p.click(); return true; })()"); await sleep(400);
      await shot("vpn-rule-server", { clip: "#ruleDlg", pad: -6 });
    }
    if (wanted("vpn-firewall")) { await vpnGo(); await click("#rulesetBtn"); await ev("(() => { const t = document.querySelector('#fwText'); t.style.maxHeight = 'none'; return true; })()"); await sleep(300); await shot("vpn-firewall", { clip: "#fwDlg", pad: -6 }); }
    // access grant for a technician (0.65.0): the dialog with the presets; with four eyes it is a request
    if (wanted("vpn-grant")) {
      await vpnGo();
      await ev("(() => { const r = [...document.querySelectorAll('#peers tbody tr')].find((t) => /J. Cooper/.test(t.textContent)); const b = r && r.querySelector('[data-grant]'); if (b) b.click(); return !!b; })()"); await sleep(600);
      await ev("(() => { const b = document.querySelector('#grantPresets [data-min=\"480\"]'); if (b) b.click(); document.querySelector('#grantForm').reason.value = 'Night shift 2 Oct, line 1 alarms'; return true; })()"); await sleep(400);
      await shot("vpn-grant", { clip: "#grantDlg", pad: -6 });
    }
    // VPN phase 3 (0.69.0, docs.py vpn3): a customer server added as "Server (uplink)" and its uplink token shown once,
    // and this server's own uplink to a cloud hub — the card, the connect dialog, an uplink rule, the transport, the
    // generated table. The demo's hub does not exist, so the uplink waits for it (shown as such).
    if (wanted("vpn-add-uplink") || wanted("vpn-uplink-token")) {
      const fillUp = fillPeer("uplink", "Customer B - Graz server", "f.subnets.value = '192.168.60.0/24'; f.note.value = 'telemetry server in the Graz plant, no public address';");
      if (wanted("vpn-add-uplink")) { await vpnGo(); await ev(fillUp); await sleep(800); await shot("vpn-add-uplink"); }
      if (wanted("vpn-uplink-token")) {
        await vpnGo(); await ev(fillUp); await sleep(500); await submitPeer();
        await ev("document.querySelector('#peerResult').scrollIntoView({block: 'start'}); true"); await sleep(400);
        await shot("vpn-uplink-token", { clip: "#peerDlg", pad: -6 });
        const st4 = await api("/api/v1/vpn"); const p = ((st4 && st4.peers) || []).find((x) => x.name === "Customer B - Graz server");
        if (p) await ev(`ctrl32.api('POST', '/api/v1/vpn/peers/${p.id}/remove', {reason: 'peer created for the documentation pictures'}).then(() => true)`);
      }
    }
    if (vpnState.uplink && vpnState.uplink.configured) {
      if (wanted("vpn-uplink-card")) { await vpnGo(); await shot("vpn-uplink-card", { clip: "#uplinkCard", pad: 8 }); }
      if (wanted("vpn-uplink-connect")) {
        await vpnGo();
        await ev("(() => { document.querySelector('#uplinkConnectBtn').click(); const f = document.querySelector('#uplinkForm'); f.token.value = 'ctrl32-uplink-v1.eyJuYW1lIjoiTml0cmEgcGxhbnQgc2VydmVyIiwiZW5kcG9pbnQiOiJodWIudGVsZW1ldHJ5LWNsb3VkLmV4YW1wbGU6NTE4MjAiLCJodWJfcHVibGljX2tleSI6Ik9MNHg0TmxabjJ4M1ZKa25HWG41N3J0ZDBhWnpLdjFsYXBEMmtCSDl5QW89IiwicHJlc2hhcmVkX2tleSI6IuKApiJ9'; f.reason.value = 'Remote service through the cloud hub (contract 2026-31)'; f.password.value = 'admin1234'; return true; })()"); await sleep(600);
        await shot("vpn-uplink-connect", { clip: "#uplinkDlg", pad: -6 });
      }
      if (wanted("vpn-uplink-rule")) {
        await vpnGo();
        await ev("(() => { const r = [...document.querySelectorAll('#uplinkRules tbody tr')].find((t) => /PLC/.test(t.textContent)); if (r) r.querySelector('[data-uredit]').click(); return !!r; })()"); await sleep(800);
        await ev("(() => { document.querySelector('#uplinkRuleForm').reason.value = 'Service contract 2026-31, packing line'; return true; })()");
        await shot("vpn-uplink-rule", { clip: "#uplinkRuleDlg", pad: -6 });
      }
      if (wanted("vpn-uplink-transport")) {
        await vpnGo(); await click("#uplinkTransportBtn", 800);
        await ev("(() => { const f = document.querySelector('#uplinkTrForm'); f.transport.value = 'https'; f.reason.value = 'UDP blocked by the plant firewall'; return true; })()"); await sleep(300);
        await shot("vpn-uplink-transport", { clip: "#uplinkTrDlg", pad: -6 });
      }
      if (wanted("vpn-uplink-firewall")) { await vpnGo(); await click("#uplinkFwBtn"); await ev("(() => { const t = document.querySelector('#fwText'); t.style.maxHeight = 'none'; return true; })()"); await sleep(300); await shot("vpn-uplink-firewall", { clip: "#fwDlg", pad: -6 }); }
    } else if (["vpn-uplink-card", "vpn-uplink-connect", "vpn-uplink-rule", "vpn-uplink-transport", "vpn-uplink-firewall"].some(wanted)) problems.push("vpn-uplink: this server has no uplink (run deploy/dev/demo/docs.py vpn)");
    // the organization's administrator (org_admin, no server administration; docs.py vpn2): Settings → VPN
    const orgAdmin = process.env.ORG_ADMIN || "olivia.grant", orgPass = process.env.ORG_ADMIN_PASS || "Olivia-docs-2026!";
    if (["vpn-organization", "vpn-organization-rule", "vpn-organization-approval"].some(wanted)) {
      const as = async (u, pw) => { await ev("fetch('/api/v1/auth/logout', {method: 'POST'}).then(() => true)"); return ev(`fetch('/api/v1/auth/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({username: ${JSON.stringify(u)}, password: ${JSON.stringify(pw)}})}).then(r => r.status)`); };
      const st = await as(orgAdmin, orgPass);
      if (st !== 200) problems.push("vpn-organization: sign-in as " + orgAdmin + " failed (" + st + "; run deploy/dev/demo/docs.py vpn)");
      else {
        const orgGo = async () => { await go("/vpn", 3500); await ev(noSim); await sleep(300); };
        if (wanted("vpn-organization")) { await orgGo(); await shot("vpn-organization", { full: true, maxH: 2400 }); }
        if (wanted("vpn-organization-rule")) {
          await orgGo(); await click("#addRuleBtn", 800);
          await ev("(() => { const f = document.querySelector('#ruleForm'); f.name.value = 'Nitra technicians → PLC line 1 (S7 102)'; [...document.querySelectorAll('#srcList input[data-st=peer]')].forEach((c) => { c.checked = true; }); f.dest_type.value = 'site'; f.dest_type.dispatchEvent(new Event('change')); f.dest_cidr.value = '192.168.10.20'; f.protocol.value = 'tcp'; f.ports.value = '102'; f.reason.value = 'Commissioning of line 1'; return true; })()"); await sleep(400);
          await shot("vpn-organization-rule", { clip: "#ruleDlg", pad: -6 });
        }
        if (wanted("vpn-organization-approval")) {
          await orgGo();
          await ev("(() => { document.querySelector('#addPeerBtn').click(); document.querySelector('#kindSeg [data-kind=user]').click(); const f = document.querySelector('#peerForm'); f.name.value = 'Service laptop - R. Novak'; f.note.value = 'Contractor, PLC service'; f.reason.value = 'New peer for the documentation'; f.password.value = " + JSON.stringify(orgPass) + "; return true; })()"); await sleep(500);
          await ev("document.querySelector('#peerSubmit').click(); true"); await sleep(3000); await ev(noSim);
          await ev("document.querySelector('#peerResult').scrollIntoView({block: 'start'}); true"); await sleep(400);
          await shot("vpn-organization-approval", { clip: "#peerDlg", pad: -6 });
          const st3 = await api("/api/v1/vpn"); const p = ((st3 && st3.peers) || []).find((x) => x.name === "Service laptop - R. Novak");
          if (p) await ev(`ctrl32.api('POST', '/api/v1/vpn/peers/${p.id}/remove', {reason: 'peer created for the documentation pictures'}).then(() => true)`);
        }
      }
      if ((await as(user, pass)) !== 200) { console.error("sign-in as " + user + " failed again"); proc.kill(); process.exit(1); }
    }
  } else if (wantVpn) problems.push("vpn: the server agent with the VPN is not available (set [agent] vpn_simulate, start the agent)");
  await page("admin-front-page-editor", "/landing/edit", { wait: 4000 });

  // ---- AI assistants
  await page("ai-settings", "/settings/ai", { wait: 3000 });
  await page("ai-account", "/account/ai", { wait: 3000 });
  if (wanted("ai-consent")) {
    // a registered assistant asks for access (the page says so when AI access is switched off)
    const reg = await ev(`fetch('/oauth/register', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({client_name: 'AI assistant (documentation)', redirect_uris: ['http://127.0.0.1:33418/callback'], token_endpoint_auth_method: 'none'})}).then(r => r.json())`);
    if (reg && reg.client_id) {
      const q = new URLSearchParams({ response_type: "code", client_id: reg.client_id, redirect_uri: "http://127.0.0.1:33418/callback", code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", code_challenge_method: "S256", state: "docs", scope: "mcp", resource: base + "/mcp" });
      await page("ai-consent", "/oauth/authorize?" + q.toString(), { wait: 3000 });
    } else problems.push("ai-consent: client registration failed");
  }

  // ---- object counting (spec 029; the demo: docs.py counting)
  const ca = ((await api("/api/v1/counting/areas")) || { areas: [] }).areas;
  const bld = ca.find((a) => a.name === "Nitra plant – main building") || ca[0];
  const park = ca.find((a) => a.kind === "car_park");
  const ccams = (await api("/api/v1/counting/cameras")) || [];
  const entCam = ccams.find((c) => c.camera === "Main entrance camera") || ccams.find((c) => c.enabled);
  const roadCam = ccams.find((c) => c.camera === "Plant road camera");
  if (bld) {
    const yday = (() => { const d = new Date(Date.now() - 864e5); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`; })();
    const day = async () => { await ev(`(() => { localStorage.setItem('ctrl32.counting.area', ${JSON.stringify(bld.id)}); return true; })()`); };
    if (wanted("counting-overview")) { await go("/counting", 1000); await day(); await go(`/counting?area=${bld.id}`, 4000); await shot("counting-overview", { full: true, maxH: 3200 }); }
    if (wanted("counting-yesterday")) { await go(`/counting?area=${bld.id}`, 3000); await click("#prev", 3000); await shot("counting-yesterday", { full: true, maxH: 3200 }); }
    if (wanted("counting-hours")) { await go(`/counting?area=${bld.id}`, 3000); await click("#prev", 3000); await shot("counting-hours", { clip: "js:document.querySelector('#hourChart').closest('.card')" }); }
    if (wanted("counting-entrances")) { await go(`/counting?area=${bld.id}`, 3000); await click("#prev", 3000); await shot("counting-entrances", { clip: "js:document.querySelector('#entTable').closest('.oc-two')" }); }
    await page("counting-correct-dialog", `/counting?area=${bld.id}`, { wait: 3500, click: "#correctBtn" });
    await page("counting-alarm-dialog", `/counting?area=${bld.id}`, { wait: 3500, click: "#alarmBtn" });
    await page("counting-export-dialog", `/counting?area=${bld.id}`, { wait: 3500, click: "#exportBtn" });
    if (park && wanted("counting-car-park")) { await go(`/counting?area=${park.id}`, 3000); await click("#prev", 3500); await shot("counting-car-park", { full: true, maxH: 3600 }); }
    if (park && wanted("counting-speeds")) { await go(`/counting?area=${park.id}`, 3000); await click("#prev", 3500); await shot("counting-speeds", { clip: "#speeds" }); }
    await page("counting-areas", "/counting/areas", { wait: 3000 });
    await page("counting-area-dialog", "/counting/areas", { wait: 3000, click: `[data-edit="${bld.id}"]`, clickWait: 1500 });
    await page("counting-new-area-dialog", "/counting/areas", { wait: 3000, click: "#newArea", clickWait: 1500 });
    await page("counting-sensors", "/counting/sensors", { wait: 3000 });
    await page("counting-sensor-dialog", "/counting/sensors", { wait: 3000, click: "[data-in]", clickWait: 2000 });
    await page("counting-cameras", "/counting/cameras", { wait: 3500 });
    // the self-check of the detector installation (0.72.0; ready when [analytics] points at an installed detector)
    await page("counting-detector-check", "/counting/cameras", { wait: 6000, clip: "#acCard" });
    if (entCam && wanted("counting-camera-editor")) { await go(`/counting/cameras?camera=${entCam.camera_id}`, 16000); await shot("counting-camera-editor", { full: true, maxH: 2600 }); }
    if (entCam && wanted("counting-camera-picture")) { await go(`/counting/cameras?camera=${entCam.camera_id}`, 16000); await shot("counting-camera-picture", { clip: "#ocEd" }); }
    if (roadCam && wanted("counting-speed-editor")) { await go(`/counting/cameras?camera=${roadCam.camera_id}`, 14000); await shot("counting-speed-editor", { full: true, maxH: 2600 }); }
    const vd = dashes.find((d) => d.name === "Visitors and vehicles");
    if (vd) await page("counting-dashboard", `/dashboards/${vd.slug}`, { wait: 7000 });
    await page("admin-scheduled-reports", "/settings/scheduled-reports", { wait: 3000 });
    const reps = ((await api("/api/v1/scheduled-reports")) || { reports: [] }).reports;
    const r1 = reps.find((r) => r.name === "Main building – daily visitors") || reps[0];
    if (r1) {
      await page("admin-scheduled-report-dialog", "/settings/scheduled-reports", { wait: 3000, click: `[data-edit="${r1.id}"]`, clickWait: 2000 });
      await page("admin-scheduled-report-log", "/settings/scheduled-reports", { wait: 3000, click: `[data-runs="${r1.id}"]`, clickWait: 2000, full: true });
    }
    if (wanted("counting-phone")) { await viewport(390, 844, true); await go(`/counting?area=${bld.id}`, 4000); await click("#prev", 3000); await shot("counting-phone", { full: true, maxH: 2600 }); await viewport(1440, 900, false); }
  }

  // ---- account
  await page("account-profile", "/account");
  await page("account-security", "/account/security");
  await page("account-tokens", "/account/tokens");
  await page("account-new-token-dialog", "/account/tokens", { click: "#newTokBtn" });
  await page("account-notifications", "/account/push");

  // ---- dark theme
  await ev("localStorage.setItem('ctrl32.theme', 'dark'); true");
  if (cold) await page("dashboards-cold-chain-dark", `/dashboards/${cold.slug}`, { wait: 6000 });
  if (prodDash()) await page("dashboards-production-lines-dark", `/dashboards/${prodDash().slug}`, { wait: 6000 });
  if (wallCR && wanted("video-wall-dark")) { await go(`/video/walls/${wallCR}`, 10000); await shot("video-wall-dark"); }
  await ev("localStorage.setItem('ctrl32.theme', 'light'); true");

  // ---- phone
  await viewport(390, 844, true);
  const prod = prodDash();
  if (prod) await page("dashboards-phone", `/dashboards/${prod.slug}`, { wait: 6000 });
  if (cold) await page("dashboards-phone-cold-chain", `/dashboards/${cold.slug}`, { wait: 6000 });
  const ccOvPhone = dashes.find((d) => d.slug === "cold-chain-overview");
  if (ccOvPhone) await page("dashboards-phone-overview", `/dashboards/${ccOvPhone.slug}`, { wait: 6000 });
  await page("video-phone-live", "/video", { wait: 9000 });
  if (gate) await page("video-phone-camera", `/video/cameras/${gate}`, { wait: 8000 });
  await page("energy-phone", "/energy", { wait: 4000 });
  await page("start-phone-menu", "/", { wait: 4000, click: "#menuBtn", clickWait: 1500 });
  await page("automation-phone-incidents", "/incidents", { wait: 3000 });
  await viewport(1440, 900, false);

  console.log(done.length + " screenshots in " + out);
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
