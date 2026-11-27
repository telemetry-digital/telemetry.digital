// Headless Chrome: the batch record (spec 043, 0.97.0). Units are set up (a recipe unit, a tag unit); a person begins
// and ends batches; the list, the record of a batch with its trend, the statistics of its curves and the alarms;
// comparing two batches of a unit; the trend and batch widgets on a dashboard; phone width.
// Needs datastreams with history: deploy/dev/demo/production.py setup.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/batches.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9387;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-batches", "--window-size=1440,1100", "--lang=" + (lang || "en")], { stdio: "ignore" });
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
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2300); if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner, #alarmBar').forEach(e => e.remove())`); };
  const suf = String(Math.floor(Math.random() * 90000) + 10000);
  // a canvas that shows more than its background
  const drawn = (sel) => `(() => { const c = document.querySelector('${sel} canvas'); if (!c || !c.width) return false; const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data; const seen = new Set(); for (let i = 0; i < d.length; i += 4 * 89) seen.add(d[i] + ',' + d[i + 1] + ',' + d[i + 2]); return seen.size > 6; })()`;

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  await go("/batches/units");
  expect("the menu has Batches", await ev(`!!document.querySelector('.side-nav a[href="/batches"]')`));

  // ---- fixtures: the unit of the test and two short batches begun by hand ----
  const fx = await ev(`(async () => {
    for (const u of await ctrl32.api('GET', '/api/v1/batch-units')) if (/^(Press line|Tag unit)/.test(u.name) && !/docs$/.test(u.name)) await ctrl32.api('DELETE', '/api/v1/batch-units/' + u.id + '?reason=headless');
    const all = await ctrl32.api('GET', '/api/v1/datastreams'); const pick = ['temperature', 'pressure', 'level'].map(k => all.find(x => /Line 1/.test(x.asset) && x.key === k)).filter(Boolean);
    const devs = await ctrl32.api('GET', '/api/v1/devices');
    if (pick.length < 3 || !devs.length) return { error: 'no demo data' };
    const u = await ctrl32.api('POST', '/api/v1/batch-units', { name: 'Press line ${suf}', mode: 'recipe', device_id: devs[0].id, prefix: 'm1_', curves: pick.map(d => d.id), reason: 'headless probe' });
    const b1 = await ctrl32.api('POST', '/api/v1/batches', { unit_id: u.id, code: 'LOT-${suf}-1', reason: 'headless probe' });
    await new Promise(r => setTimeout(r, 7000));
    const b2 = await ctrl32.api('POST', '/api/v1/batches', { unit_id: u.id, code: 'LOT-${suf}-2', reason: 'headless probe' });
    const docs = (await ctrl32.api('GET', '/api/v1/batch-units')).find(x => /docs$/.test(x.name));
    return { unit: u.id, b1: b1.id, b2: b2.id, ds: pick.map(d => d.id), tag: pick[0].id, docs: docs ? docs.id : '' };
  })()`);
  if (!fx || !fx.unit) { console.log("fixtures: " + JSON.stringify(fx) + " (run deploy/dev/demo/production.py setup)"); proc.kill(); process.exit(1); }

  // ---- units ----
  await go("/batches/units");
  expect("the unit is listed with the way its batches begin and a running batch", await until(`[...document.querySelectorAll('#uList tbody tr')].some(r => r.cells[0].textContent.trim() === 'Press line ${suf}' && r.cells[3].textContent.trim() === '3' && r.cells[4].textContent.trim() === '2' && !!r.cells[5].querySelector('.badge.ok'))`), await ev(`document.querySelector('#uList tbody').innerText`));
  await ev(`(document.querySelector('#uNew').click(), 1)`);
  expect("a new unit starts as a recipe unit", await until(`document.querySelector('#uDlg').open && !document.querySelector('#uRecipe').hidden && document.querySelector('#uTag').hidden`));
  await ev(`(() => { const f = document.querySelector('#uDlg form'); f.name.value = 'Tag unit ${suf}'; f.mode.value = 'tag'; f.mode.dispatchEvent(new Event('change')); return 1; })()`);
  expect("choosing a tag shows the tag and what it means", await until(`document.querySelector('#uRecipe').hidden && !document.querySelector('#uTag').hidden && document.querySelector('#uHint').textContent.length > 20`));
  if (process.env.DOCS) { await ev(`(() => { const f = document.querySelector('#uDlg form'); f.tag_datastream.value = '${fx.tag}'; f.tag_kind.value = 'number'; for (const id of ${JSON.stringify(fx.ds)}) { const c = document.querySelector('#uCurves input[value="' + id + '"]'); if (c) { c.checked = true; c.dispatchEvent(new Event('change')); } } document.querySelector('#uFilter').value = 'Line 1'; document.querySelector('#uFilter').dispatchEvent(new Event('input')); return 1; })()`); await sleep(300); await shot("batch-unit.png"); }
  await ev(`(() => { const f = document.querySelector('#uDlg form'); f.tag_datastream.value = '${fx.tag}'; f.tag_kind.value = 'number'; f.requestSubmit(); return 1; })()`);
  expect("a unit needs a reason", await ev(`document.querySelector('#uDlg').open`));
  await ev(`(() => { const f = document.querySelector('#uDlg form'); f.reason.value = 'headless probe'; f.requestSubmit(); return 1; })()`);
  expect("the tag unit is saved and waits for its tag", (await until(`!document.querySelector('#uDlg').open`)) && (await until(`[...document.querySelectorAll('#uList tbody tr')].some(r => r.cells[0].textContent.trim() === 'Tag unit ${suf}' && r.cells[2].textContent.includes('temperature'))`)), [await ev(`document.querySelector('#uErr').textContent`), await ev(`document.querySelector('#uList tbody').innerText`)]);
  await ev(`window.scrollTo(0, 0)`); await shot("batch-units.png");
  await ev(`(window.confirm = () => true, [...document.querySelectorAll('#uList tbody tr')].find(r => r.cells[0].textContent.trim() === 'Tag unit ${suf}').click(), 1)`);
  await until(`document.querySelector('#uDlg').open`);
  await ev(`(() => { const f = document.querySelector('#uDlg form'); f.reason.value = 'headless end'; document.querySelector('#uDelete').click(); return 1; })()`);
  expect("a unit is removed with a reason", await until(`![...document.querySelectorAll('#uList tbody tr')].some(r => r.cells[0].textContent.trim() === 'Tag unit ${suf}')`));

  // ---- the list and the record of a batch ----
  await go("/batches?unit=" + fx.unit);
  expect("the list shows both batches, the running one first", await until(`(() => { const r = [...document.querySelectorAll('#btList tbody tr[data-id]')]; return r.length === 2 && r[0].cells[2].textContent.trim() === 'LOT-${suf}-2' && !!r[0].querySelector('.badge.ok') && r[1].cells[2].textContent.trim() === 'LOT-${suf}-1'; })()`), await ev(`document.querySelector('#btList tbody').innerText`));
  await ev(`(document.querySelectorAll('#btList tbody tr[data-id]')[1].click(), 1)`);
  expect("a click opens the record: who began it, how long it ran", await until(`!document.querySelector('#btDetail').hidden && document.querySelector('#dTitle').textContent.includes('LOT-${suf}-1') && document.querySelector('#dHead').textContent.includes('admin') && /id=/.test(location.search)`), await ev(`document.querySelector('#dHead').innerText`));
  expect("its trend is drawn with the rulers at its beginning and end", (await until(drawn("#dChart"))) && (await until(`CTL && CTL.state().A != null && CTL.state().B != null && CTL.state().B > CTL.state().A`)), await ev(`CTL ? [CTL.state().A, CTL.state().B] : 'no control'`));
  expect("its three curves are listed (a batch of seconds may hold no sample yet)", await until(`document.querySelectorAll('#dStats tbody tr').length === 3`), await ev(`document.querySelector('#dStats tbody').innerText`));
  expect("the alarms of its values are listed, or said to be none", await until(`document.querySelectorAll('#dAlarms tbody tr').length >= 1`));
  expect("the link to Trends carries the curves and the time of the batch", await ev(`(() => { const u = new URL(document.querySelector('#dTrends').href); return u.pathname === '/trends' && u.searchParams.get('ds').split(',').length === 3 && Date.parse(u.searchParams.get('to')) > Date.parse(u.searchParams.get('from')); })()`));
  // comparing
  expect("the other batch of the unit is offered for comparison", await until(`document.querySelectorAll('#cSel option').length === 2`), await ev(`document.querySelector('#cSel').innerText`));
  await ev(`(() => { const s = document.querySelector('#cSel'); s.value = s.options[1].value; s.dispatchEvent(new Event('change')); return 1; })()`);
  expect("the comparison draws the second batch over the same length of time, with the difference of the averages", (await until(`!document.querySelector('#cBox').hidden && document.querySelectorAll('#cStats tbody tr').length === 3`)) && (await until(drawn("#cChart"))) && (await until(`CCTL && CTL && Math.abs(CCTL.state().span - CTL.state().span) < 50`)), await ev(`CCTL && CTL ? [CCTL.state().span, CTL.state().span] : 'no control'`));

  // ---- ending and beginning by hand ----
  await ev(`(document.querySelectorAll('#btList tbody tr[data-id]')[0].click(), 1)`);
  expect("the running batch offers End the batch", await until(`document.querySelector('#dTitle').textContent.includes('LOT-${suf}-2') && !document.querySelector('#dEnd').hidden`));
  await ev(`(window.prompt = () => 'lot complete', document.querySelector('#dEnd').click(), 1)`);
  expect("ended with a reason: finished, the reason is in the record", await until(`document.querySelector('#dEnd').hidden && document.querySelector('#dHead').textContent.includes('lot complete') && !document.querySelector('#btList tbody .badge.ok')`), await ev(`document.querySelector('#dHead').innerText`));
  await ev(`(document.querySelector('#btStart').click(), 1)`);
  expect("Begin a batch asks for the unit, a code and a reason", await until(`document.querySelector('#sDlg').open && document.querySelector('#sDlg form').unit_id.value === '${fx.unit}'`));
  await ev(`(() => { const f = document.querySelector('#sDlg form'); f.code.value = 'LOT-${suf}-3'; f.reason.value = 'next lot'; f.requestSubmit(); return 1; })()`);
  expect("the new batch runs and its record opens", (await until(`!document.querySelector('#sDlg').open`)) && (await until(`document.querySelector('#dTitle').textContent.includes('LOT-${suf}-3') && document.querySelectorAll('#btList tbody tr[data-id]').length === 3`)), await ev(`[document.querySelector('#sErr').textContent, document.querySelector('#dTitle').textContent]`));

  // ---- an hour-long batch of the demo (tests/headless/batches_demo.sh): the statistics kept with the record ----
  if (fx.docs) {
    const one = await ev(`ctrl32.api('GET', '/api/v1/batches?unit=${fx.docs}&from=' + encodeURIComponent(new Date(Date.now() - 3 * 86400e3).toISOString())).then(l => l.filter(b => b.status === 'finished')[0].id)`);
    await go("/batches?unit=" + fx.docs + "&id=" + one);
    expect("an hour-long batch: minimum, maximum, average and the number of samples of every curve", await until(`(() => { const r = [...document.querySelectorAll('#dStats tbody tr')]; return r.length === 3 && r.every(x => /\\d/.test(x.cells[4].textContent) && /\\d/.test(x.cells[6].textContent) && +x.cells[7].textContent.replace(/\\D/g, '') >= 10); })()`), await ev(`document.querySelector('#dStats tbody').innerText`));
    expect("its curves are drawn as lines, not lost as holes (a sample a minute in a window of short buckets)", await until(`(() => { const c = document.querySelector('#dChart canvas'); const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data; let blue = 0; for (let i = 0; i < d.length; i += 4) if (d[i] < 80 && d[i + 1] > 70 && d[i + 1] < 130 && d[i + 2] > 200) blue++; return blue > 400; })()`), 'no curve pixels');
    expect("the record names the recipe, the person at the controls and why it ended", await ev(`(() => { const t = document.querySelector('#dHead').textContent; return t.includes('Pressing') && t.includes('station1 / peter') && t.includes('veduci'); })()`), await ev(`document.querySelector('#dHead').innerText`));
  } else console.log("  skip the hour-long batch (run tests/headless/batches_demo.sh)");
  if (process.env.DOCS && fx.docs) {
    const long = await ev(`ctrl32.api('GET', '/api/v1/batches?unit=${fx.docs}&from=' + encodeURIComponent(new Date(Date.now() - 3 * 86400e3).toISOString())).then(l => l.filter(b => b.status === 'finished').map(b => b.id))`);
    if (long && long.length >= 2) { await go("/batches?unit=" + fx.docs + "&id=" + long[0]); await until(drawn("#dChart")); await sleep(1200); await ev(`window.scrollTo(0, 0)`); await shot("batches.png");
      await ev(`(document.querySelector('#btDetail').scrollIntoView({ block: 'start' }), 1)`); await sleep(400); await shot("batch-record.png");
      await ev(`(() => { const s = document.querySelector('#cSel'); s.value = s.options[1].value; s.dispatchEvent(new Event('change')); return 1; })()`);
      await until(drawn("#cChart")); await sleep(1200); await ev(`(document.querySelector('.bt-cmp').scrollIntoView({ block: 'start' }), 1)`); await sleep(400); await shot("batch-compare.png"); }
  } else { await ev(`window.scrollTo(0, 0)`); await shot("batches.png"); }

  // ---- the widgets ----
  const dash = await ev(`ctrl32.api('POST', '/api/v1/dashboards', { name: 'Batch widgets ${suf}', spec: { version: 1, range: '1h', refresh_s: 30, widgets: [
    { id: 'tr', type: 'trend', geometry: { x: 0, y: 0, w: 8, h: 5 }, bindings: { datastreams: ${JSON.stringify(fx.ds)} }, params: { title: 'Line 1', window: '1h', tools: true, table: true } },
    { id: 'bt', type: 'batches', geometry: { x: 8, y: 0, w: 4, h: 5 }, bindings: {}, params: { title: 'Press line', batch_unit: '${fx.unit}', rows: 5, running: true } }] }, reason: 'headless probe' }).then(d => d.slug)`);
  await go("/dashboards/" + dash);
  const card = (wid) => `document.querySelector('[data-id="${wid}"]')`;
  expect("the trend widget draws the bound datastreams", (await until(`!!${card("tr")} && !!${card("tr")}.querySelector('.trw-chart canvas')`)) && (await until(drawn('[data-id="tr"] .trw-chart'))));
  expect("its table lists the three curves with minimum, maximum and average", await until(`(() => { const r = [...${card("tr")}.querySelectorAll('.trw-table tbody tr')]; return r.length === 3 && r.every(x => /\\d/.test(x.cells[2].textContent) && /\\d/.test(x.cells[4].textContent)); })()`), await ev(`(${card("tr")}.querySelector('.trw-table') || { innerText: 'no table' }).innerText`));
  await ev(`([...${card("tr")}.querySelectorAll('.trw-bar [data-r]')].find(b => b.dataset.r === '6h').click(), 1)`);
  expect("its buttons change the time shown", await until(`${card("tr")}.querySelector('.trw-bar [data-r="6h"]').classList.contains('on') && /range=6h/.test(${card("tr")}.querySelector('[data-open]').href)`), await ev(`${card("tr")}.querySelector('[data-open]').href`));
  await ev(`(typeof refreshFn === 'function' ? refreshFn() : Promise.resolve()).then(() => 1)`); await sleep(800);
  expect("a refresh of the dashboard keeps the time the person chose", await ev(`${card("tr")}.querySelector('.trw-bar [data-r="6h"]').classList.contains('on') && !!${card("tr")}.querySelector('.trw-chart canvas')`));
  expect("the batch widget shows the running batch and the last ones, each a link to its record", await until(`(() => { const c = ${card("bt")}; const now = c.querySelector('.btw-now:not(.idle)'); const rows = [...c.querySelectorAll('tbody tr')]; return !!now && now.textContent.includes('LOT-${suf}-3') && rows.length === 2 && /\\/batches\\?id=/.test(rows[0].querySelector('a').getAttribute('href')); })()`), await ev(`${card("bt")}.innerText`));
  await sleep(500); await ev(`window.scrollTo(0, 0)`); await shot("batch-widgets.png");

  // ---- phone width ----
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 2, mobile: true });
  await go("/batches?unit=" + fx.unit + "&id=" + fx.b1);
  expect("on a phone the record fits the screen", (await until(drawn("#dChart"))) && (await ev(`(() => { const c = document.querySelector('#dChart canvas').getBoundingClientRect(); return c.left >= 0 && c.right <= 391 && document.querySelector('#btDetail').getBoundingClientRect().right <= 391; })()`)), await ev(`JSON.stringify(document.querySelector('#dChart canvas').getBoundingClientRect())`));
  await ev(`(document.querySelector('#btDetail').scrollIntoView({ block: 'start' }), 1)`); await sleep(300); await shot("batch-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");

  await ev(`(async () => { await ctrl32.api('DELETE', '/api/v1/batch-units/${fx.unit}?reason=headless'); await ctrl32.api('DELETE', '/api/v1/dashboards/${dash}?reason=headless').catch(() => 0); return 1; })()`);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
