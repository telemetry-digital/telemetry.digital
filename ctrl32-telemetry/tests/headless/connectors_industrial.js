// Headless Chrome: Settings → Connectors (0.83.0) — OPC UA endpoint discovery, browsing the server tree and adding
// nodes, a subscription connector feeding two devices; a Siemens S7 connector with a test read (CPU, PDU, values);
// the connector list with both running. Starts the OPC UA and S7 simulators (bin/opcuasim[.exe], bin/s7sim[.exe] —
// build them with `go build -o bin/ ./tools/opcuasim ./tools/s7sim`), writes screenshots to OUT and revokes the
// probe connectors. The simulators are built when missing.
const { spawn, spawnSync } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9361;
const exe = (n) => path.join("bin", n + (process.platform === "win32" ? ".exe" : ""));
const OPC = 4862, S7 = 10122;
if (!fs.existsSync(exe("opcuasim")) || !fs.existsSync(exe("s7sim"))) {
  const b = spawnSync("go", ["build", "-o", "bin/", "./tools/opcuasim", "./tools/s7sim"], { stdio: "inherit" });
  if (b.status !== 0) { console.error("simulator build failed"); process.exit(1); }
}
const sims = [spawn(exe("opcuasim"), ["-port", String(OPC)], { stdio: "ignore" }), spawn(exe("s7sim"), ["-port", String(S7)], { stdio: "ignore" })];
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-ind", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const stop = () => { proc.kill(); for (const s of sims) s.kill(); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
let failed = 0;
const expect = (label, cond, detail) => { console.log((cond ? "  ok   " : "  FAIL ") + label + (cond ? "" : " — " + JSON.stringify(detail))); if (!cond) failed++; };
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  // DOCS=1: crop to the dialog (or the connector card) for the documentation; otherwise the whole window.
  const shot = async (name, sel) => {
    let clip;
    if (process.env.DOCS && sel) { const r = await ev(`(() => { const e = document.querySelector('${sel}'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: b.x, y: b.y, width: b.width, height: Math.min(b.height, innerHeight - b.y) }; })()`); if (r) clip = { ...r, scale: 1 }; }
    const s = await send("Page.captureScreenshot", clip ? { format: "png", clip } : { format: "png" }); fs.writeFileSync(path.join(out, name), Buffer.from(s.result.data, "base64")); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr)) return true; await sleep(250); } return false; };
  await sleep(1500); // simulators
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await ev(`localStorage.setItem('ctrl32.updateDismissed', '9.9.9')`);
  await send("Page.navigate", { url: base + "/settings/connectors" }); await sleep(3000);
  if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`);
  const suf = Math.floor(Math.random() * 1e5);

  // OPC UA: discover endpoints
  await ev(`(() => { document.querySelector('#newConnBtn').click(); const f = document.querySelector('#newConn'); f.kind.value = 'opcua'; f.kind.dispatchEvent(new Event('change')); f.name.value = 'packaging-${suf}'; f.endpoint.value = 'opc.tcp://127.0.0.1:${OPC}'; document.querySelector('#opcEndpoints').click(); })()`);
  expect("endpoints listed", await until(`document.querySelectorAll('#opcEpOut [data-ep]').length > 0`), await ev(`document.querySelector('#opcEpOut').textContent`));
  await ev(`document.querySelector('#opcEpOut [data-ep]').click()`);
  expect("endpoint applied (None/None)", (await ev(`document.querySelector('#newConn').security_policy.value + '/' + document.querySelector('#newConn').security_mode.value`)) === "None/None");
  await shot("connector-opcua-endpoints.png", "#connDlg");

  // OPC UA: browse Objects → Simulation → Plant → Line1, pick everything
  await ev(`document.querySelector('#opcBrowse').click()`);
  expect("browse dialog lists Objects", await until(`[...document.querySelectorAll('#brTable [data-open]')].some(a => a.textContent === 'Simulation')`), await ev(`document.querySelector('#brErr').textContent`));
  await ev(`[...document.querySelectorAll('#brTable [data-open]')].find(a => a.textContent === 'Simulation').click()`);
  expect("Simulation opened", await until(`[...document.querySelectorAll('#brTable [data-open]')].some(a => a.textContent === 'Plant')`));
  await ev(`[...document.querySelectorAll('#brTable [data-open]')].find(a => a.textContent === 'Plant').click()`);
  expect("Plant opened", await until(`[...document.querySelectorAll('#brTable [data-open]')].some(a => a.textContent === 'Line1')`));
  await ev(`[...document.querySelectorAll('#brTable [data-open]')].find(a => a.textContent === 'Line1').click()`);
  expect("Line1 variables with values", await until(`document.querySelectorAll('#brTable [data-pick]').length === 4`), await ev(`document.querySelector('#brTable').innerText`));
  await ev(`(() => { for (const c of document.querySelectorAll('#brTable [data-pick]')) { c.checked = true; c.dispatchEvent(new Event('change')); } document.querySelector('#brDevice').value = 'line1-${suf}'; })()`);
  expect("breadcrumb Objects › Simulation › Plant › Line1", (await ev(`document.querySelector('#brPath').innerText`)).replace(/\s+/g, " ").includes("Objects › Simulation › Plant › Line1"), await ev(`document.querySelector('#brPath').innerText`));
  await shot("connector-opcua-browse.png", "#browseDlg");
  await ev(`document.querySelector('#brAdd').click()`);
  const nodes = await ev(`document.querySelector('#newConn').nodes.value`);
  expect("4 nodes added with keys and device", nodes.split("\n").length === 4 && nodes.includes("| speed |") && nodes.includes(`line1-${suf}`), nodes);
  // Line 2 by hand, subscription mode
  await ev(`(() => { const f = document.querySelector('#newConn'); const ns = f.nodes.value.match(/^(ns=[0-9]+);/)[1]; f.nodes.value += '\\n' + ns + ';s=Line2.Speed | speed | | | | | | line2-${suf}\\n' + ns + ';s=Line2.Count | count | | | | | | line2-${suf}'; f.mode.value = 'subscribe'; f.mode.dispatchEvent(new Event('change')); f.publishing_ms.value = 500; f.reason.value = 'packaging line 1 and 2'; })()`);
  expect("polling interval hidden in subscription mode", await ev(`document.querySelector('#newConn').interval_s.closest('label').hidden && !document.querySelector('#newConn').publishing_ms.closest('label').hidden`));
  await ev(`document.querySelector('#connTest').click()`);
  expect("test connection reads all nodes", await until(`document.querySelector('#connTestOut').textContent.startsWith('OK') && !document.querySelector('#connTestOut').textContent.includes('not readable')`), await ev(`document.querySelector('#connTestOut').textContent`));
  await ev(`document.querySelector('#connTest').scrollIntoView({block: 'center'})`); await shot("connector-opcua-form.png", "#connDlg");
  await ev(`document.querySelector('#connSubmit').click()`);
  expect("OPC UA connector created", await until(`!document.querySelector('#opcResult').hidden`), await ev(`document.querySelector('#connErr').textContent`));
  await ev(`document.querySelector('#connDlg').close()`);

  // Siemens S7
  await ev(`(() => { document.querySelector('#newConnBtn').click(); const f = document.querySelector('#newConn'); f.kind.value = 's7'; f.kind.dispatchEvent(new Event('change')); f.name.value = 'press-${suf}'; f.s7_host.value = '127.0.0.1'; f.s7_port.value = ${S7}; f.plc.value = 's7-1200'; f.s7_interval_s.value = 2;
    f.tags.value = 'temp | DB1.DBD0\\ncounter | DB1.DBW4\\npump | DB1.DBX6.0 | | | | rw\\nsetpoint | DB1.DBD8 | real | | | rw | 0 | 100\\nrecipe | DB1.DBS20.20 | | | | rw\\nlevel | MW10 | int | 0.1'; f.reason.value = 'press line PLC'; document.querySelector('#s7Test').click(); })()`);
  expect("S7 test identifies the CPU and reads the tags", await until(`/PDU \\d+/.test(document.querySelector('#s7TestOut').textContent) && document.querySelector('#s7TestOut').textContent.includes('level = 123.4')`), await ev(`document.querySelector('#s7TestOut').textContent`));
  await ev(`document.querySelector('#s7Test').scrollIntoView({block: 'center'})`); await shot("connector-s7-form.png", "#connDlg");
  await ev(`document.querySelector('#connSubmit').click()`);
  expect("S7 connector created", await until(`!document.querySelector('#opcResult').hidden`), await ev(`document.querySelector('#connErr').textContent`));
  await ev(`document.querySelector('#connDlg').close()`);

  // the list with both running
  await sleep(4000);
  await ev(`connectors()`); await sleep(1500);
  const rows = await ev(`[...document.querySelectorAll('#connectors tbody tr')].map(r => r.innerText.replace(/\\s+/g, ' ')).filter(t => t.includes('${suf}'))`);
  expect("both connectors connected", rows.length === 2 && rows.every((r) => r.includes("connected")), rows);
  expect("S7 row shows host, PLC and CPU", rows.some((r) => r.includes("S7-1200 rack 0 slot 1") && r.includes("CPU 1214C")), rows);
  expect("OPC UA row shows subscription and notifications", rows.some((r) => r.includes("subscription") && r.includes("notifications")), rows);
  if (process.env.DOCS) await ev(`document.querySelectorAll('#connectors tbody tr').forEach(r => { if (!r.innerText.includes('${suf}')) r.remove(); })`);
  await ev(`window.scrollTo(0, 0)`);
  await shot("connectors-industrial.png", "#connectors");

  // edit round trip keeps the S7 settings
  const editOK = await ev(`(() => { const c = CONNS.find(c => c.name === 'press-${suf}'); openConn(c); const f = document.querySelector('#newConn'); const ok = f.s7_host.value === '127.0.0.1' && f.slot.value === '1' && f.tags.value.split('\\n').length === 6 && /setpoint [|] DB1.DBD8 [|] real( [|] *)+rw [|] 0 [|] 100/.test(f.tags.value); document.querySelector('#connDlg').close(); return ok; })()`);
  expect("edit dialog restores S7 settings", editOK === true, editOK);

  console.log("cleanup:", await ev(`(async () => { const h = {'X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const cs = await (await fetch('/api/v1/connectors',{credentials:'same-origin'})).json(); const st = []; for (const c of cs.filter(c => !c.revoked_at && c.name.endsWith('-${suf}'))) st.push((await fetch('/api/v1/connectors/' + c.id + '?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status); return st; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  if (problems.length) failed++;
  console.log(failed ? `FAILED ${failed}` : "all ok");
  stop(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); stop(); process.exit(1); });
