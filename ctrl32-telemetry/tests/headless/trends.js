// Headless Chrome: the trend page (spec 042, 0.96.0). Curves are added from the list of datastreams and drawn on axes
// by unit; ranges, live, the wheel, dragging and the zoom box change the window; a click places ruler A, with "Ruler B"
// the second one, and the table shows the values under them and the statistics of the window; alarm limits; a view is
// saved for oneself and shared with the organization; a link carries the curves and the time; phone width.
// Needs datastreams with history: bash deploy/dev/dev.sh demo (or deploy/dev/demo/production.py setup).
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/trends.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9386;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-trends", "--window-size=1440,1000", "--lang=" + (lang || "en")], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const json = (url) => new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej));
let failed = 0;
const expect = (label, cond, detail) => { console.log((cond ? "  ok   " : "  FAIL ") + label + (cond ? "" : " — " + JSON.stringify(detail))); if (!cond) failed++; };

(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push(msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n")[0]);
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + ((r.result.exceptionDetails.exception || {}).description || "") } : r.result.result.value; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(path.join(out, name), Buffer.from(s.result.data, "base64")); };
  const until = async (expr, ms = 10000) => { for (let i = 0; i < ms / 150; i++) { if ((await ev(expr)) === true) return true; await sleep(150); } return false; };
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2200); if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`); };
  const mouse = (type, x, y, extra) => send("Input.dispatchMouseEvent", Object.assign({ type, x, y, button: type === "mouseMoved" ? "none" : "left", buttons: type === "mouseReleased" || type === "mouseMoved" && !(extra && extra.drag) ? 0 : 1, clickCount: type === "mouseMoved" ? 0 : 1 }, extra && extra.raw));
  // a point inside the plot: fx 0..1 from its left edge, in page coordinates
  const at = (fx, fy = 0.5) => ev(`(() => { const c = document.querySelector('.tr-canvas'); c.scrollIntoView({ block: 'center' }); const r = c.getBoundingClientRect(); return { x: r.left + 70 + (r.width - 150) * ${fx}, y: r.top + 30 + (r.height - 80) * ${fy} }; })()`);
  const st = (expr) => ev(`(() => { const s = CTL.state(); return ${expr}; })()`);
  const suf = String(Math.floor(Math.random() * 90000) + 10000);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  await go("/trends");
  expect("the page opens empty and says how to start", await until(`(typeof CTL !== 'undefined' && !!CTL) && document.querySelectorAll('#trTable tbody tr[data-i]').length === 0 && document.querySelector('#trSave').disabled`));
  expect("the menu has Trends", await ev(`!!document.querySelector('#sidebar a[href="/trends"], .side-nav a[href="/trends"]')`));

  // ---- datastreams with history, and views left by an earlier run removed ----
  const fx = await ev(`(async () => {
    for (const v of await ctrl32.api('GET', '/api/v1/trends')) if (/^(Press shop|My press)/.test(v.name)) await ctrl32.api('DELETE', '/api/v1/trends/' + v.id + '?reason=headless');
    const all = await ctrl32.api('GET', '/api/v1/datastreams'); const want = ['temperature', 'pressure', 'level']; const pick = [];
    for (const k of want) { const d = all.find(x => /Line 1/.test(x.asset) && x.key === k); if (d) pick.push(d); }
    const from = new Date(Date.now() - 3600e3).toISOString(); const counts = [];
    for (const d of pick) counts.push((await ctrl32.api('GET', '/api/v1/observations?datastream_id=' + d.id + '&from=' + encodeURIComponent(from) + '&limit=2000')).length);
    return { ids: pick.map(d => d.id), names: pick.map(d => d.asset + ' / ' + d.key), counts };
  })()`);
  if (!fx || !fx.ids || fx.ids.length < 3 || fx.counts.some((n) => n < 5)) { console.log("no datastreams with history (run deploy/dev/demo/production.py setup): " + JSON.stringify(fx)); proc.kill(); process.exit(1); }

  // ---- adding curves ----
  await ev(`(document.querySelector('#trAdd').click(), 1)`);
  expect("the dialog lists the datastreams", await until(`document.querySelector('#addDlg').open && document.querySelectorAll('#addList input').length > 10`));
  await ev(`(() => { const f = document.querySelector('#addFilter'); f.value = 'Line 1'; f.dispatchEvent(new Event('input')); return 1; })()`);
  expect("the search narrows the list", await until(`[...document.querySelectorAll('#addList label')].every(l => /Line 1/.test(l.textContent)) && document.querySelectorAll('#addList label').length >= 3`));
  await ev(`(() => { for (const id of ${JSON.stringify(fx.ids.slice(0, 2))}) document.querySelector('#addList input[value="' + id + '"]').checked = true; document.querySelector('#addDlg form').requestSubmit(); return 1; })()`);
  expect("two curves are in the table with their units", await until(`document.querySelectorAll('#trTable tbody tr[data-i]').length === 2`));
  expect("their data arrive and the statistics are filled", await until(`CTL.state().rows.every(r => r.stats && r.stats.n > 3 && r.stats.max >= r.stats.min)`), await st(`s.rows.map(r => r.stats)`));
  expect("two units make two axes; the view is live for one hour", await st(`s.live && Math.round(s.span / 60000) === 60 && new Set(s.rows.map(r => r.unit)).size === 2`));
  expect("the table shows minimum, maximum and average as numbers", await ev(`[...document.querySelectorAll('#trTable tbody tr[data-i]')].every(r => [6, 7, 8].every(i => /\\d/.test(r.cells[i].textContent)))`), await ev(`document.querySelector('#trTable tbody').innerText`));

  // ---- pointer, rulers ----
  let p = await at(0.5); await mouse("mouseMoved", p.x, p.y);
  expect("moving the pointer shows the time and the values under it", (await until(`CTL.state().hover != null && CTL.state().rows.every(r => r.cur != null)`)) && (await ev(`document.querySelector('#trRead').textContent.length > 8 && /\\d/.test(document.querySelector('#trTable tbody tr').cells[2].textContent)`)), await st(`[s.hover, s.rows.map(r => r.cur)]`));
  p = await at(0.3); await mouse("mousePressed", p.x, p.y); await mouse("mouseReleased", p.x, p.y);
  expect("a click places ruler A", await until(`CTL.state().A != null && CTL.state().rows.every(r => r.a != null)`), await st(`[s.A, s.rows.map(r => r.a)]`));
  await ev(`(document.querySelector('#trSecond').click(), 1)`);
  p = await at(0.7); await mouse("mousePressed", p.x, p.y); await mouse("mouseReleased", p.x, p.y);
  expect("with Ruler B on, a click places the second ruler; the table shows the difference", (await until(`(() => { const s = CTL.state(); return s.B != null && s.B > s.A && s.rows.every(r => r.b != null); })()`)) && (await ev(`/\\d/.test(document.querySelector('#trTable tbody tr').cells[5].textContent) && /B − A/.test(document.querySelector('#trRead').textContent)`)), await ev(`document.querySelector('#trRead').textContent`));
  const dT = await st(`(s.B - s.A) / s.span`);
  expect("the rulers stand where they were clicked (0.3 and 0.7 of the window)", Math.abs(dT - 0.4) < 0.04, dT);
  await ev(`(document.querySelector('#trSecond').click(), 1)`);
  await sleep(300); await ev(`window.scrollTo(0, 0)`); await shot("trends.png");

  // ---- time: ranges, wheel, drag, zoom box ----
  await ev(`([...document.querySelectorAll('#trRanges [data-r]')].find(b => b.dataset.r === '6h').click(), 1)`);
  expect("a range button changes the window and stays live", await until(`(() => { const s = CTL.state(); return s.live && Math.round(s.span / 3600e3) === 6 && document.querySelector('#trRanges [data-r="6h"]').classList.contains('on'); })()`));
  expect("a longer window is drawn from averages and says so", await until(`CTL.state().rows.every(r => r.bucket >= 5) && document.querySelector('.tr-note').textContent.length > 10`), [await st(`s.rows.map(r => r.bucket)`), await ev(`document.querySelector('.tr-note').textContent`)]);
  await ev(`([...document.querySelectorAll('#trRanges [data-r]')].find(b => b.dataset.r === '1h').click(), 1)`); await until(`Math.round(CTL.state().span / 60000) === 60`);
  p = await at(0.5); const span0 = await st(`s.span`);
  await send("Input.dispatchMouseEvent", { type: "mouseWheel", x: p.x, y: p.y, deltaX: 0, deltaY: -120 });
  expect("the wheel zooms in around the pointer and ends the live mode", await until(`CTL.state().span < ${span0} * 0.9 && !CTL.state().live`), await st(`[s.span, s.live]`));
  const t1a = await st(`s.t1`); p = await at(0.3); const q = await at(0.6);
  await mouse("mousePressed", p.x, p.y); await mouse("mouseMoved", (p.x + q.x) / 2, p.y, { drag: true }); await mouse("mouseMoved", q.x, q.y, { drag: true }); await mouse("mouseReleased", q.x, q.y);
  expect("dragging moves back in time", await until(`CTL.state().t1 < ${t1a} - 1000`), [t1a, await st(`s.t1`)]);
  expect("the fields From and To follow", await ev(`Math.abs(Date.parse(document.querySelector('#trTo').value) - CTL.state().t1) < 2000`));
  await ev(`(document.querySelector('#trBox').click(), 1)`); const span1 = await st(`s.span`); p = await at(0.4); const q2 = await at(0.6);
  await mouse("mousePressed", p.x, p.y); await mouse("mouseMoved", (p.x + q2.x) / 2, p.y, { drag: true }); await mouse("mouseMoved", q2.x, q2.y, { drag: true }); await mouse("mouseReleased", q2.x, q2.y);
  expect("the zoom box shows the selected fifth of the window", await until(`Math.abs(CTL.state().span / ${span1} - 0.2) < 0.04`), [span1, await st(`s.span`)]);
  await ev(`(document.querySelector('#trBox').click(), document.querySelector('#trLive').click(), 1)`);
  expect("Live returns to the clock", await until(`CTL.state().live && Date.now() - CTL.state().t1 < 5000`));
  await ev(`([...document.querySelectorAll('#trBar [data-act]')].find(b => b.dataset.act === 'back').click(), 1)`);
  expect("the arrow moves half a window back", await until(`!CTL.state().live && Date.now() - CTL.state().t1 > CTL.state().span * 0.4`));

  // ---- a curve's settings, alarm limits ----
  await ev(`(document.querySelector('#trTable tbody tr[data-i="1"] [data-edit]').click(), 1)`);
  expect("the settings of a curve open", await until(`document.querySelector('#curveDlg').open`));
  await ev(`(() => { const f = document.querySelector('#curveDlg form'); f.label.value = 'Pressure P1'; f.style.value = 'step'; f.min.value = '5'; f.max.value = '2'; f.requestSubmit(); return 1; })()`);
  expect("an axis that ends below its start is refused", await until(`document.querySelector('#curveErr').textContent.length > 3`));
  await ev(`(() => { const f = document.querySelector('#curveDlg form'); f.min.value = '0'; f.max.value = '20'; f.requestSubmit(); return 1; })()`);
  expect("label, steps and a fixed axis are applied", await until(`(() => { const r = CTL.state().rows[1]; return r.name === 'Pressure P1' && r.style === 'step' && r.curve.min === 0 && r.curve.max === 20; })()`), await st(`[s.rows[1].name, s.rows[1].style]`));
  await ev(`(document.querySelector('#trTable tbody tr[data-i="0"] [data-vis]').click(), 1)`);
  expect("a click on the colour hides the curve", await until(`CTL.state().rows[0].curve.hidden === true && document.querySelector('#trTable tbody tr[data-i="0"]').classList.contains('off')`));
  await ev(`(document.querySelector('#trTable tbody tr[data-i="0"] [data-vis]').click(), document.querySelector('#trLimits').click(), document.querySelector('#trAlarms').click(), 1)`);
  expect("alarm limits and alarms are switched on", await until(`CTL.state().limits && CTL.state().alarms && document.querySelector('#trLimits').classList.contains('on')`));

  // ---- saved views ----
  await ev(`(document.querySelector('#trLive').click(), document.querySelector('#trSaveAs').click(), 1)`);
  expect("Save as asks for a name", await until(`document.querySelector('#saveDlg').open`));
  await ev(`(() => { const f = document.querySelector('#saveDlg form'); f.name.value = 'My press ${suf}'; f.requestSubmit(); return 1; })()`);
  expect("a personal view is saved without a reason", (await until(`!document.querySelector('#saveDlg').open`)) && (await until(`(() => { const o = document.querySelector('#trView').selectedOptions[0]; return !!o && o.textContent === 'My press ${suf}' && /view=/.test(location.search); })()`, 4000)), [await ev(`document.querySelector('#saveErr').textContent`), await ev(`location.search`)]);
  const viewURL = await ev(`location.pathname + location.search`);
  await ev(`(document.querySelector('#trSaveAs').click(), 1)`); await until(`document.querySelector('#saveDlg').open`);
  await ev(`(() => { const f = document.querySelector('#saveDlg form'); f.name.value = 'Press shop ${suf}'; f.shared.click(); f.requestSubmit(); return 1; })()`);
  expect("a view shared with the organization needs a reason", (await until(`document.querySelector('#saveErr').textContent.length > 3`)) && (await ev(`document.querySelector('#saveDlg').open`)));
  await ev(`(() => { const f = document.querySelector('#saveDlg form'); f.reason.value = 'the shift watches these together'; f.requestSubmit(); return 1; })()`);
  expect("with a reason it is saved and listed as shared", (await until(`!document.querySelector('#saveDlg').open`)) && (await until(`[...document.querySelectorAll('#trView optgroup')].length === 2 && document.querySelector('#trView').selectedOptions[0].textContent === 'Press shop ${suf}'`)), await ev(`document.querySelector('#trView').innerText`));
  const audit = await ev(`ctrl32.api('GET', '/api/v1/audit?limit=10').then(a => a.entries.some(e => e.action === 'trend_view.create' && e.reason === 'the shift watches these together'))`);
  expect("the shared view is in the audit trail", audit === true);

  await go(viewURL);
  expect("the link opens the personal view: its curves, label, limits", await until(`(typeof CTL !== 'undefined' && !!CTL) && (() => { const s = CTL.state(); return s.rows.length === 2 && s.rows[1].name === 'Pressure P1' && s.limits && s.alarms && s.live && s.rows.every(r => r.stats); })()`), await st(`[s.rows.map(r => r.name), s.limits, s.live]`));
  await go("/trends?ds=" + fx.ids.join(",") + "&from=" + encodeURIComponent(new Date(Date.now() - 40 * 60000).toISOString()) + "&to=" + encodeURIComponent(new Date(Date.now() - 10 * 60000).toISOString()));
  expect("a link with datastreams and a time opens exactly that", await until(`(typeof CTL !== 'undefined' && !!CTL) && (() => { const s = CTL.state(); return s.rows.length === 3 && !s.live && Math.round(s.span / 60000) === 30 && s.rows.every(r => r.stats); })()`), await st(`[s.rows.length, s.live, s.span]`));
  const tb = await ev(`(() => { const t = CTL.table(); return { cols: t.cols.length, rows: t.rows.length, ordered: t.rows.every((r, i) => !i || r[0] > t.rows[i - 1][0]), inWin: t.rows.every(r => r[0] >= CTL.state().t0 && r[0] <= CTL.state().t1) }; })()`);
  expect("the export holds the window: three columns, rows in time order", tb.cols === 3 && tb.rows > 5 && tb.ordered && tb.inWin, tb);
  expect("the CSV button is there for data.export; the picture is a PNG", (await ev(`!document.querySelector('#trCsv').hidden`)) && (await ev(`CTL.png().then(b => b.type === 'image/png' && b.size > 5000)`)));
  expect("the canvas is not blank", await ev(`(() => { const c = document.querySelector('.tr-canvas'), d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data; const seen = new Set(); for (let i = 0; i < d.length; i += 4 * 97) seen.add(d[i] + ',' + d[i + 1] + ',' + d[i + 2]); return seen.size > 6; })()`));
  if (process.env.DOCS) { await go("/trends?ds=" + fx.ids.join(",") + "&range=6h"); await until(`(typeof CTL !== 'undefined' && !!CTL) && CTL.state().rows.every(r => r.stats)`); await ev(`(document.querySelector('#trLimits').click(), 1)`); p = await at(0.35); await mouse("mousePressed", p.x, p.y); await mouse("mouseReleased", p.x, p.y); await ev(`(document.querySelector('#trSecond').click(), 1)`); p = await at(0.8); await mouse("mousePressed", p.x, p.y); await mouse("mouseReleased", p.x, p.y); await sleep(700); await ev(`window.scrollTo(0, 0)`); await shot("trends-6h.png");
    await ev(`(document.querySelector('#trAdd').click(), 1)`); await sleep(400); await shot("trends-add.png"); await ev(`(document.querySelector('#addDlg [data-close]').click(), 1)`);
    await ev(`(document.querySelector('#trTable tbody tr[data-i="1"] [data-edit]').click(), 1)`); await sleep(400); await shot("trends-curve.png"); await ev(`(document.querySelector('#curveDlg [data-close]').click(), 1)`); }

  // ---- another user: sees the shared view, not the personal one ----
  const other = await ev(`(async () => { const u = await ctrl32.api('POST', '/api/v1/users', { username: 'trend_${suf}', full_name: 'Trend Viewer', email: 'trend${suf}@example.com', roles: ['viewer'], reason: 'headless probe' }); return u.initial_password; })()`);
  const seen = await (async () => { let cookie = "", csrf = ""; const call = async (m, u, b) => { const r = await fetch(base + u, { method: m, headers: { "Content-Type": "application/json", Cookie: cookie, "X-CSRF-Token": csrf }, body: b ? JSON.stringify(b) : undefined }); const s = r.headers.getSetCookie ? r.headers.getSetCookie() : []; if (s.length) cookie = s.map((c) => c.split(";")[0]).join("; "); return { status: r.status, data: await r.json().catch(() => ({})) }; };
    const pw = "Trend-Viewer-Passw0rd-" + suf; await call("POST", "/api/v1/auth/login", { username: "trend_" + suf, password: other }); csrf = (await call("GET", "/api/v1/auth/me")).data.csrf_token || "";
    await call("POST", "/api/v1/auth/password", { current: other, new: pw }); const l = await call("POST", "/api/v1/auth/login", { username: "trend_" + suf, password: pw }); csrf = (await call("GET", "/api/v1/auth/me")).data.csrf_token || ""; const v = await call("GET", "/api/v1/trends"); return { login: l.status, status: v.status, names: Array.isArray(v.data) ? v.data.map((x) => x.name + ":" + x.can_edit) : v.data }; })();
  expect("another user sees the shared view only, and cannot change it", seen.status === 200 && seen.names.includes("Press shop " + suf + ":false") && !seen.names.some((n) => n.startsWith("My press " + suf)), seen);

  // ---- phone width ----
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 2, mobile: true });
  await go("/trends?ds=" + fx.ids.slice(0, 2).join(","));
  expect("on a phone the chart fits the screen", (await until(`(typeof CTL !== 'undefined' && !!CTL) && CTL.state().rows.every(r => r.stats)`)) && (await ev(`(() => { const c = document.querySelector('.tr-canvas').getBoundingClientRect(); return c.left >= 0 && c.right <= 391 && c.width > 280; })()`)), await ev(`JSON.stringify(document.querySelector('.tr-canvas').getBoundingClientRect())`));
  await shot("trends-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");

  await ev(`(async () => { for (const v of await ctrl32.api('GET', '/api/v1/trends')) if (/${suf}$/.test(v.name)) await ctrl32.api('DELETE', '/api/v1/trends/' + v.id + '?reason=headless'); return 1; })()`);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
