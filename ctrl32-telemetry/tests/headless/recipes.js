// Headless Chrome: Recipes (spec 038, 0.92.0) against the bundled S7 simulator. A recipe with two mixers behind one
// controller is created in the dialog, data records are written, the values in the unit are shown and taken over, a
// record is loaded (check, values before and after, read back), a record the connector refuses cannot be loaded, a
// load waits for four eyes and is decided in the load log, records are imported from CSV, and a record that no longer
// fits the changed recipe is marked.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/recipes.js
const { spawn, spawnSync } = require("child_process");
const fs = require("fs");
const http = require("http");
const os = require("os");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9379, S7 = 10142;
const exe = path.join("bin", "s7sim" + (process.platform === "win32" ? ".exe" : ""));
if (!fs.existsSync(exe)) { const b = spawnSync("go", ["build", "-o", "bin/", "./tools/s7sim"], { stdio: "inherit" }); if (b.status !== 0) { console.error("simulator build failed"); process.exit(1); } }
fs.mkdirSync(out, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "recipes-"));
const csvFile = path.join(tmp, "records.csv");
fs.writeFileSync(csvFile, "number;name;note;speed;temp;heat;product\r\n1;Cream 30 %;imported;1250;44,5;true;cream\r\n8;Lotion;;600;35;false;lotion\r\n");
const sim = spawn(exe, ["-port", String(S7)], { stdio: "ignore" });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-recipes", "--window-size=1440,1000", "--lang=" + (lang || "en-US"), "about:blank"], { stdio: "ignore" });
const stop = () => { proc.kill(); sim.kill(); fs.rmSync(tmp, { recursive: true, force: true }); };
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
  await send("Page.enable"); await send("Runtime.enable"); await send("DOM.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const shot = async (name, sel) => { let clip;
    if (sel) { const r = await ev(`(() => { const e = document.querySelector('${sel}'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: b.x, y: b.y, width: b.width, height: Math.min(b.height, innerHeight - b.y) }; })()`); if (r) clip = { ...r, scale: 1 }; }
    const s = await send("Page.captureScreenshot", clip ? { format: "png", clip } : { format: "png" }); fs.writeFileSync(path.join(out, name), Buffer.from(s.result.data, "base64")); };
  const until = async (expr, ms = 8000) => { for (let i = 0; i < ms / 150; i++) { if (await ev(expr)) return true; await sleep(150); } return false; };
  // window.prompt asks for the reasons; the headless browser answers with a fixed one
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2200); await ev(`(window.prompt = () => 'headless probe', window.confirm = () => true, 1)`);
    if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`); };
  const call = (m, p, body) => ev(`(async () => { const me = await fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()); const r = await fetch('${p}', { method: '${m}', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf_token }${body ? `, body: JSON.stringify(${JSON.stringify(body)})` : ""} }); let j = null; try { j = await r.json(); } catch (e) {} return { status: r.status, body: j }; })()`);
  const setVal = (key, v) => ev(`(() => { const i = document.querySelector('#valTable [data-el="${key}"]'); if (i.type === 'checkbox') { if (i.checked !== ${v === true}) i.click(); } else { i.value = ${JSON.stringify(String(v))}; i.dispatchEvent(new Event('input', { bubbles: true })); } return 1; })()`);
  const cur = () => ev(`Object.fromEntries([...document.querySelectorAll('#valTable [data-cur]')].map(td => [td.dataset.cur, td.textContent.trim() + (td.classList.contains('diff') ? ' *' : '')]))`);
  // the rows of the load log that belong to this run's recipe
  const mine = (sel) => `[...document.querySelectorAll('#ldTable tbody tr')].filter(r => r.cells[1].textContent === ${JSON.stringify("Mixing ")} + '${suf}')${sel ? `.filter(r => r.querySelector('${sel}'))` : ""}`;
  const suf = String(Math.floor(Math.random() * 90000) + 10000), name = "Mixing " + suf;

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  const unit = (p, db) => [{ key: p + "speed", address: `DB${db}.DBW0`, type: "int", writable: true, min: 0, max: 1500 }, { key: p + "temp", address: `DB${db}.DBD2`, writable: true }, { key: p + "heat", address: `DB${db}.DBX6.0`, writable: true }, { key: p + "product", address: `DB${db}.DBS8.10`, writable: true }];
  const conn = await call("POST", "/api/v1/connectors", { kind: "s7", name: "mixers-" + suf, settings: { host: "127.0.0.1", port: S7, plc: "s7-1200", interval_s: 1, device_external_id: "s7-mixers-" + suf, tags: [...unit("m1_", 2), ...unit("m2_", 3)] }, reason: "headless probe" });
  expect("connector of the simulator", conn.status === 201, conn);
  let dev = null;
  for (let i = 0; i < 30 && !dev; i++) { await sleep(700); dev = await ev(`fetch('/api/v1/devices',{credentials:'same-origin'}).then(r=>r.json()).then(l => (l.find(d => d.external_id === 's7-mixers-${suf}') || {}).id || null)`); }
  expect("its device", !!dev);

  // ---- a new recipe in the dialog ----
  await go("/recipes");
  expect("the menu has Recipes", await ev(`!!document.querySelector('a[href="/recipes"]')`));
  expect("the button for a new recipe is shown", await ev(`!document.querySelector('#rcNew').hidden`));
  await ev(`(document.querySelector('#rcNew').click(), 1)`); await until(`document.querySelector('#rcDlg').open && document.querySelectorAll('#tgTable select option').length > 1`);
  expect("the dialog offers the device of the connector and its writable keys", await ev(`[...document.querySelectorAll('#tgTable select option')].some(o => o.value === '${dev}') && [...document.querySelectorAll('#elKeys option')].some(o => o.value === 'm1_speed')`));
  await ev(`(() => { const f = document.querySelector('#rcForm'); f.name.value = ${JSON.stringify(name)}; f.description.value = 'Speed, temperature and heating of a mixer';
    for (let i = 0; i < 3; i++) document.querySelector('#elAdd').click(); document.querySelector('#tgAdd').click();
    const set = (tr, o) => { for (const k in o) { const e = tr.querySelector('[data-f=' + k + ']'); e.value = o[k]; e.dispatchEvent(new Event('change')); } };
    const el = document.querySelectorAll('#elTable tbody tr');
    set(el[0], { key: 'speed', label: 'Speed', unit: 'rpm', min: 0, max: 3000 }); set(el[1], { key: 'temp', label: 'Temperature', unit: '°C', min: 10, max: 90, decimals: 1 });
    set(el[2], { key: 'heat', label: 'Heating', type: 'bool' }); set(el[3], { key: 'product', label: 'Product', type: 'text' });
    const tg = document.querySelectorAll('#tgTable tbody tr'); set(tg[0], { name: 'Mixer 1', device_id: '${dev}', prefix: 'm1_' }); set(tg[1], { name: 'Mixer 2', device_id: '${dev}', prefix: 'm2_' });
    f.reason.value = 'headless probe'; return 1; })()`);
  expect("limits are switched off for an on/off element", await ev(`document.querySelectorAll('#elTable tbody tr')[2].querySelector('[data-f=min]').disabled && !document.querySelectorAll('#elTable tbody tr')[0].querySelector('[data-f=min]').disabled`));
  await shot("recipe-edit.png", process.env.DOCS ? "#rcDlg" : null);
  await ev(`(document.querySelector('#rcForm').requestSubmit(), 1)`);
  expect("the recipe is created and opened", await until(`!document.querySelector('#rcDlg').open && document.querySelector('#rcSel').selectedOptions[0] && document.querySelector('#rcSel').selectedOptions[0].textContent === ${JSON.stringify(name)}`), await ev(`document.querySelector('#rcErr').textContent`));
  expect("no record yet, both units offered", await ev(`!document.querySelector('#recNone').hidden && document.querySelectorAll('#unitSel option').length === 2`));

  // ---- the first record ----
  await ev(`(document.querySelector('#recNew').click(), 1)`); await sleep(300);
  await ev(`(() => { const f = document.querySelector('#recForm'); f.name.value = 'Cream 30 %'; f.note.value = 'standard batch'; return 1; })()`);
  await setVal("speed", 1200); await setVal("temp", 45.5); await setVal("heat", true); await setVal("product", "cream");
  expect("an unsaved record cannot be loaded", await ev(`document.querySelector('#loadBtn').hidden || document.querySelector('#loadBtn').disabled`));
  await ev(`(document.querySelector('#recForm').requestSubmit(), 1)`);
  expect("the record is saved with number 1", await until(`document.querySelectorAll('#recList button').length === 1 && document.querySelector('#recList button b').textContent === '1' && document.querySelector('#recForm').number.value === '1'`), await ev(`document.querySelector('#recErr').textContent`));
  expect("the values in the unit are shown and differ from the record", await until(`[...document.querySelectorAll('#valTable [data-cur]')].filter(td => td.classList.contains('diff')).length === 4 && /^0 rpm/.test(document.querySelector('#valTable [data-cur="speed"]').textContent)`), await cur());
  expect("names typed by the user are not translated (Speed is also a text of the application)", await ev(`document.querySelector('#valTable tbody tr td').textContent === 'Speed' && document.querySelector('#recList button span').textContent === 'Cream 30 %'`), await ev(`document.querySelector('#valTable tbody tr td').textContent`));
  expect("the unit line names the device and the time of the read", await ev(`/mixers-${suf}/.test(document.querySelector('#unitInfo').textContent) && /\\d/.test(document.querySelector('#unitInfo').textContent.split('·')[1] || '')`), await ev(`document.querySelector('#unitInfo').textContent`));

  // ---- the load ----
  await ev(`(document.querySelector('#loadBtn').click(), 1)`);
  expect("the load dialog checks the record and enables Load", await until(`document.querySelector('#ldDlg').open && !document.querySelector('#ldGo').disabled`), await ev(`document.querySelector('#ldErr').textContent + document.querySelector('#ldProblems').textContent`));
  expect("it shows what is in the unit and what will be written", await ev(`(() => { const r = [...document.querySelectorAll('#ldValues tbody tr')].map(t => [...t.cells].map(c => c.textContent.trim())); return r.length === 4 && r[0][1] === '0 rpm' && r[0][2] === '1200 rpm' && r[1][2] === '45.5 °C' && document.querySelectorAll('#ldValues td.diff').length === 4; })()`), await ev(`[...document.querySelectorAll('#ldValues tbody tr')].map(t => [...t.cells].map(c => c.textContent.trim()))`));
  await ev(`(document.querySelector('#ldForm').requestSubmit(), 1)`); await sleep(300);
  expect("a load without a reason is not sent", await ev(`document.querySelector('#ldDlg').open && !document.querySelector('#ldGo').hidden`));
  await ev(`(document.querySelector('#ldForm').reason.value = 'batch 2026-118', document.querySelector('#ldForm').requestSubmit(), 1)`);
  expect("every value is written and read back", await until(`document.querySelectorAll('#ldValues .rc-st.written').length === 4 && document.querySelector('#ldGo').hidden`), await ev(`document.querySelector('#ldErr').textContent + ' | ' + document.querySelector('#ldValues').textContent`));
  await shot("recipe-load.png", process.env.DOCS ? "#ldDlg" : null);
  await ev(`(document.querySelector('#ldClose').click(), 1)`);
  expect("the unit now holds the record", await until(`[...document.querySelectorAll('#valTable [data-cur]')].every(td => !td.classList.contains('diff')) && /^1200 rpm/.test(document.querySelector('#valTable [data-cur="speed"]').textContent)`, 10000), await cur());
  await shot("recipes.png");
  await ev(`(() => { const s = document.querySelector('#unitSel'); s.value = s.options[1].value; s.dispatchEvent(new Event('change')); return 1; })()`);
  expect("the other unit was not touched", await until(`/^0 rpm/.test(document.querySelector('#valTable [data-cur="speed"]').textContent) && document.querySelector('#valTable [data-cur="speed"]').classList.contains('diff')`), await cur());
  await ev(`(() => { const s = document.querySelector('#unitSel'); s.value = s.options[0].value; s.dispatchEvent(new Event('change')); return 1; })()`); await until(`/^1200 rpm/.test(document.querySelector('#valTable [data-cur="speed"]').textContent)`);

  // ---- editing marks the difference and blocks the load; taking the values over ----
  await setVal("speed", 900);
  expect("a changed value differs from the unit and the load waits for saving", await ev(`document.querySelector('#valTable [data-cur="speed"]').classList.contains('diff') && document.querySelector('#loadBtn').disabled`));
  await ev(`(document.querySelector('#takeBtn').click(), 1)`); await sleep(200);
  expect("Take the values from the unit fills the form", await ev(`document.querySelector('#valTable [data-el="speed"]').value === '1200' && document.querySelector('#valTable [data-el="product"]').value === 'cream' && document.querySelector('#valTable [data-el="heat"]').checked`));
  await ev(`(document.querySelector('#recForm').name.value = 'Too fast', 1)`); await setVal("speed", 2000);
  await ev(`(document.querySelector('#recCopy').click(), 1)`);
  expect("Save as a new record adds number 2", await until(`document.querySelectorAll('#recList button').length === 2 && document.querySelector('#recList button.on b').textContent === '2'`), await ev(`document.querySelector('#recErr').textContent`));

  // ---- a value the connector refuses ----
  await ev(`(document.querySelector('#loadBtn').click(), 1)`);
  expect("the check names the value the connector refuses and Load stays off", await until(`document.querySelector('#ldDlg').open && !document.querySelector('#ldProblems').hidden`) && await ev(`document.querySelector('#ldGo').disabled && document.querySelectorAll('#ldProblems li').length === 1 && /1500/.test(document.querySelector('#ldProblems').textContent)`), await ev(`document.querySelector('#ldProblems').textContent`));
  await shot("recipe-load-refused.png", process.env.DOCS ? "#ldDlg" : null);
  await ev(`(document.querySelector('#ldClose').click(), 1)`);

  // ---- four eyes ----
  expect("four eyes on", (await call("PUT", "/api/v1/settings/policy", { four_eyes_commands: true, reason: "headless probe" })).status === 200);
  await ev(`(document.querySelector('#recList button').click(), 1)`); await until(`document.querySelector('#recList button.on b').textContent === '1' && !document.querySelector('#loadBtn').disabled`);
  await ev(`(document.querySelector('#loadBtn').click(), 1)`);
  expect("the load dialog announces four eyes", await until(`document.querySelector('#ldDlg').open && !document.querySelector('#ldGo').disabled && !document.querySelector('#ldFour').hidden`));
  await ev(`(document.querySelector('#ldForm').reason.value = 'batch 2026-119', document.querySelector('#ldForm').requestSubmit(), 1)`);
  // four eyes everywhere (spec 040): the dialog of the second person opens over the load; here it is left waiting
  expect("the second person is asked for", await until(`!!(document.querySelector('#spDlg') && document.querySelector('#spDlg').open) && /Line A|→/.test(document.querySelector('#spDlg .sp-sum').textContent)`));
  await ev(`(document.querySelector('#spDlg [data-sp-wait]').click(), 1)`);
  expect("the load waits, nothing is written", await until(`document.querySelector('#ldGo').hidden`) && await ev(`document.querySelectorAll('#ldValues .rc-st').length === 0`));
  await go("/recipes/loads");
  expect("the load log shows the waiting load with Approve and Reject", await until(`${mine()}.length === 2`) && await ev(`${mine("[data-approve]")}.length === 1 && ${mine(".rc-st.pending")}.length === 1 && ${mine(".rc-st.done")}.length === 1`), await ev(`${mine()}.map(r => r.textContent)`));
  await shot("recipe-loads.png");
  await ev(`(${mine("[data-approve]")}[0].querySelector('[data-approve]').click(), 1)`);
  expect("the user who asked cannot approve", await until(`[...document.querySelectorAll('#toast.show.bad')].some(e => e.textContent.trim().length > 5)`) && await ev(`${mine(".rc-st.pending")}.length === 1`), await ev(`[...document.querySelectorAll('#toast.show.bad')].map(e => e.textContent)`));
  await ev(`(${mine("[data-reject]")}[0].querySelector('[data-reject]').click(), 1)`);
  expect("rejected", await until(`${mine(".rc-st.pending")}.length === 0 && ${mine(".rc-st.rejected")}.length === 1`));
  expect("four eyes off", (await call("PUT", "/api/v1/settings/policy", { four_eyes_commands: false, reason: "headless probe" })).status === 200);
  await ev(`(${mine(".rc-st.done")}[0].querySelector('[data-detail]').click(), 1)`);
  expect("the detail of a load shows key, written value and read back", await until(`document.querySelector('#dtDlg').open`) && await ev(`(() => { const r = [...document.querySelectorAll('#dtValues tbody tr')].map(t => [...t.cells].map(c => c.textContent.trim())); return r.length === 4 && r[0][1] === 'm1_speed' && r[0][2] === '1200 rpm' && r[0][3] === '1200' && r[3][3] === 'cream' && /batch 2026-118/.test(document.querySelector('#dtWhat').textContent); })()`), await ev(`document.querySelector('#dtValues').textContent`));
  await shot("recipe-load-detail.png", process.env.DOCS ? "#dtDlg" : null);

  // ---- CSV import through the real file input ----
  await go("/recipes");
  await until(`document.querySelectorAll('#recList button').length === 2`);
  expect("CSV buttons", await ev(`!document.querySelector('#rcCsvOut').hidden && !document.querySelector('#rcCsvIn').hidden`));
  { const doc = await send("DOM.getDocument", {}); const n = await send("DOM.querySelector", { nodeId: doc.result.root.nodeId, selector: "#rcCsvFile" }); await send("DOM.setFileInputFiles", { nodeId: n.result.nodeId, files: [csvFile] }); }
  expect("the import changes record 1 and adds record 8", await until(`document.querySelectorAll('#recList button').length === 3`) && await ev(`[...document.querySelectorAll('#recList button')].map(b => b.textContent.trim()).join('|')`) === "1Cream 30 %|2Too fast|8Lotion", await ev(`[...document.querySelectorAll('#recList button')].map(b => b.textContent.trim()).join('|')`));
  expect("the decimal comma of the spreadsheet is read", await until(`document.querySelector('#valTable [data-el="temp"]').value === '44.5' && document.querySelector('#valTable [data-el="speed"]').value === '1250'`), await ev(`document.querySelector('#valTable [data-el="temp"]').value`));

  // ---- a record that no longer fits ----
  await ev(`(document.querySelector('#rcEdit').click(), 1)`); await until(`document.querySelector('#rcDlg').open && document.querySelectorAll('#elTable tbody tr').length === 4`);
  expect("the dialog shows the stored recipe", await ev(`document.querySelectorAll('#elTable tbody tr')[1].querySelector('[data-f=decimals]').value === '1' && document.querySelectorAll('#tgTable tbody tr')[1].querySelector('[data-f=prefix]').value === 'm2_' && document.querySelectorAll('#tgTable tbody tr')[1].dataset.id === 'u2'`));
  await ev(`(() => { const e = document.querySelectorAll('#elTable tbody tr')[0].querySelector('[data-f=max]'); e.value = 1000; document.querySelector('#rcForm').reason.value = 'lower limit'; document.querySelector('#rcForm').requestSubmit(); return 1; })()`);
  expect("records above the new limit are marked and cannot be loaded", await until(`!document.querySelector('#rcDlg').open && document.querySelectorAll('#recList button i').length === 2`) && await until(`!document.querySelector('#recProblem').hidden && document.querySelector('#loadBtn').disabled`), await ev(`document.querySelector('#recList').textContent`));

  // ---- a phone ----
  await send("Emulation.setDeviceMetricsOverride", { width: 400, height: 800, deviceScaleFactor: 1, mobile: true }); await sleep(500);
  expect("on a phone the page does not scroll sideways", await ev(`document.documentElement.scrollWidth <= innerWidth + 1 && ['#recList', '#recForm', '#valTable'].every(s => document.querySelector(s).closest('.table-wrap, #recList, #recForm').getBoundingClientRect().right <= innerWidth)`), await ev(`[document.documentElement.scrollWidth, innerWidth, document.querySelector('#recForm').getBoundingClientRect().right]`));
  await shot("recipes-phone.png");
  await send("Emulation.clearDeviceMetricsOverride"); await sleep(300);

  // ---- deleting ----
  await ev(`(document.querySelector('#recDel').click(), 1)`);
  expect("the record is deleted", await until(`document.querySelectorAll('#recList button').length === 2`));
  await ev(`(document.querySelector('#rcDel').click(), 1)`);
  expect("the recipe is deleted", await until(`![...document.querySelectorAll('#rcSel option')].some(o => o.textContent === ${JSON.stringify(name)})`));
  expect("cleanup: connector revoked", (await call("DELETE", `/api/v1/connectors/${conn.body.id}?reason=headless+probe`)).status < 300);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  stop(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); stop(); process.exit(1); });
