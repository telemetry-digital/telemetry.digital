// Headless Chrome: Settings → Connectors → Siemens S7 → Import tags from TIA Portal / STEP 7 (spec 037, 0.91.0).
// A data block source that mirrors DB1 of the bundled simulator and a STEP 7 symbol table are uploaded through the
// real file input; the proposed tags are filtered, selected and added to the tag list, the test read against the
// simulator shows the values at the calculated addresses, and the connector is created and revoked again.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/s7_import.js
const { spawn, spawnSync } = require("child_process");
const fs = require("fs");
const http = require("http");
const os = require("os");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9378, S7 = 10132;
const exe = path.join("bin", "s7sim" + (process.platform === "win32" ? ".exe" : ""));
if (!fs.existsSync(exe)) { const b = spawnSync("go", ["build", "-o", "bin/", "./tools/s7sim"], { stdio: "inherit" }); if (b.status !== 0) { console.error("simulator build failed"); process.exit(1); } }
fs.mkdirSync(out, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "s7imp-"));
const dbFile = path.join(tmp, "DB_Pumps.db"), symFile = path.join(tmp, "symbols.sdf");
fs.writeFileSync(dbFile, `TYPE "udtSpare"
VERSION : 0.1
   STRUCT
      A : Int;
      B : Int;
   END_STRUCT;
END_TYPE

DATA_BLOCK "DB_Pumps"
{ S7_Optimized_Access := 'FALSE' }
VERSION : 0.1
NON_RETAIN
   STRUCT
      Flow : Real;   // m3/h
      Starts : Int;   // number of starts
      Running : Bool;   // pump P1 runs
      Setpoint : Real;   // bar
      Spare : Array[0..1] of "udtSpare";
      Recipe : String[20];   // name of the recipe
      Stamp : DTL;   // last start
   END_STRUCT;

BEGIN
   Setpoint := 21.5;

END_DATA_BLOCK
`);
fs.writeFileSync(symFile, Buffer.from('"Hladina n\xe1dr\x9ee          ","MW     10","INT       ","hladina v mm"\r\n"Start                   ","E       0.0","BOOL      ","tla\xe8idlo \x9atart"\r\n"Analog                  ","PEW   256","WORD      ",""\r\n', "latin1"));
const sim = spawn(exe, ["-port", String(S7)], { stdio: "ignore" });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-s7imp", "--window-size=1440,1000", "--lang=" + (lang || "en-US"), "about:blank"], { stdio: "ignore" });
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
  const setFile = async (file) => { const doc = await send("DOM.getDocument", {}); const n = await send("DOM.querySelector", { nodeId: doc.result.root.nodeId, selector: "#impFile" }); await send("DOM.setFileInputFiles", { nodeId: n.result.nodeId, files: [file] }); };
  const read = async () => { await ev(`(document.querySelector('#impRead').click(), 1)`); for (let i = 0; i < 40; i++) { await sleep(150); if (await ev(`!!document.querySelector('#impInfo').textContent || !!document.querySelector('#impErr').textContent`)) break; } await sleep(150); };
  const rows = () => ev(`[...document.querySelectorAll('#impTable tbody tr')].map(r => [...r.cells].slice(1).map(c => c.innerText.trim().replace(/\\s+/g, ' ')))`);
  const suf = String(Math.floor(Math.random() * 90000) + 10000);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  await send("Page.navigate", { url: base + "/settings/connectors" }); await sleep(3000);
  if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`);
  await ev(`(() => { document.querySelector('#newConnBtn').click(); const f = document.querySelector('#newConn'); f.kind.value = 's7'; f.kind.dispatchEvent(new Event('change')); f.name.value = 'pumps-${suf}'; f.s7_host.value = '127.0.0.1'; f.s7_port.value = ${S7}; f.plc.value = 's7-1200'; f.s7_device_external_id.value = 's7-pumps-${suf}'; f.tags.value = 'setpoint | DB1.DBD8'; return 1; })()`);
  expect("the S7 form has the import button", await ev(`!document.querySelector('#s7Fields').hidden && !!document.querySelector('#s7Import')`));
  await ev(`(document.querySelector('#s7Import').click(), 1)`); await sleep(300);
  expect("the import dialog opens", await ev(`document.querySelector('#impDlg').open && document.querySelector('#impAdd').disabled`));
  if (lang === "sk") expect("Slovak: Central Europe is preselected", (await ev(`document.querySelector('#impEnc').value`)) === "windows-1250");

  // ---- a data block source without its number ----
  await ev(`(document.querySelector('#impRead').click(), 1)`); await sleep(200);
  expect("reading without a file is refused", await ev(`document.querySelector('#impErr').textContent.length > 3`));
  await setFile(dbFile); await read();
  let r = await rows();
  expect("the block is read: 9 variables, 1 left out", r.length === 9 && await ev(`document.querySelector('#impInfo').textContent.includes('DB_Pumps') && /\\(1\\)/.test(document.querySelector('#impSkipped summary').textContent)`), { rows: r.length, info: await ev(`document.querySelector('#impInfo').textContent`) });
  expect("addresses carry the placeholder until the block number is given", r[0] && r[0][2] === "DB{db}.DBD0", r[0]);
  await ev(`(document.querySelector('#impAll').click(), document.querySelector('#impAdd').click(), 1)`); await sleep(200);
  expect("adding without the block number is refused", await ev(`document.querySelector('#impDlg').open && document.querySelector('#impErr').textContent.length > 10`));
  await ev(`(document.querySelector('#impDB').value = '1', document.querySelector('#impPrefix').value = 'p1_', 1)`); await read();
  r = await rows();
  const want = { p1_flow: "DB1.DBD0", p1_starts: "DB1.DBW4", p1_running: "DB1.DBX6.0", p1_setpoint: "DB1.DBD8", p1_spare_0_a: "DB1.DBW12", p1_spare_1_b: "DB1.DBW18", p1_recipe: "DB1.DBS20.20" };
  const got = Object.fromEntries(r.map((x) => [x[0], x[2]]));
  expect("keys with the prefix and the calculated addresses", Object.entries(want).every(([k, a]) => got[k] === a), got);
  expect("the comment and the PLC type are shown", r[0][4].includes("m3/h") && r[2][3].toLowerCase().includes("bool"), [r[0], r[2]]);
  expect("the warning to compare the offsets is shown", await ev(`document.querySelectorAll('#impWarn li').length >= 1`));
  expect("the unsupported DTL is listed with its reason", await ev(`document.querySelector('#impSkipped tbody').textContent.includes('DTL')`));
  await ev(`(() => { const f = document.querySelector('#impFilter'); f.value = 'spare'; f.dispatchEvent(new Event('input')); return 1; })()`); await sleep(150);
  expect("the filter narrows the list to the four spare words", (await rows()).length === 4);
  await ev(`(() => { const f = document.querySelector('#impFilter'); f.value = ''; f.dispatchEvent(new Event('input')); return 1; })()`); await sleep(150);
  // pick five variables with real clicks on their check boxes
  for (const k of ["p1_flow", "p1_starts", "p1_running", "p1_setpoint", "p1_recipe"]) await ev(`(() => { const tr = [...document.querySelectorAll('#impTable tbody tr')].find(r => r.cells[1].innerText.trim() === '${k}'); tr.querySelector('input').click(); return 1; })()`);
  await sleep(200);
  expect("five variables are selected", await ev(`document.querySelectorAll('#impTable tbody input:checked').length === 5 && !document.querySelector('#impAdd').disabled && /5/.test(document.querySelector('#impCount').textContent)`), await ev(`document.querySelector('#impCount').textContent`));
  await shot("s7-import.png", process.env.DOCS ? "#impDlg" : null);
  await ev(`(document.querySelector('#impAdd').click(), 1)`); await sleep(300);
  let tags = await ev(`document.querySelector('#newConn').tags.value`);
  expect("the dialog closes and the tags are in the list", await ev(`!document.querySelector('#impDlg').open`) && tags.includes("p1_flow | DB1.DBD0 | real") && tags.includes("p1_recipe | DB1.DBS20.20 | string"), tags);
  expect("the address that was already in the list is not added twice", tags.split("\n").filter((l) => l.includes("DB1.DBD8")).length === 1 && tags.startsWith("setpoint | DB1.DBD8"), tags);
  expect("no imported tag is writable", !/\brw\b/.test(tags), tags);

  // ---- a STEP 7 symbol table in Windows-1250 ----
  await ev(`(document.querySelector('#s7Import').click(), 1)`); await sleep(300);
  await ev(`(document.querySelector('#impEnc').value = 'windows-1250', document.querySelector('#impDB').value = '', document.querySelector('#impPrefix').value = '', 1)`);
  await setFile(symFile); await read();
  r = await rows();
  expect("the symbol table: two variables, diacritics kept in the symbol and removed from the key", r.length === 2 && r[0][0] === "hladina_nadrze" && r[0][1] === "Hladina nádrže" && r[0][2] === "MW10" && r[1][2] === "I0.0" && r[1][4] === "tlačidlo štart", r);
  await ev(`(() => { const a = document.querySelector('#impArea'); a.value = 'm'; a.dispatchEvent(new Event('change')); return 1; })()`); await sleep(150);
  expect("the area filter shows bit memory only", (await rows()).length === 1);
  await ev(`(document.querySelector('#impAll').click(), 1)`); await sleep(150);
  await shot("s7-import-symbols.png", process.env.DOCS ? "#impDlg" : null);
  await ev(`(document.querySelector('#impAdd').click(), 1)`); await sleep(300);
  tags = await ev(`document.querySelector('#newConn').tags.value`);
  expect("the memory word is added", tags.includes("hladina_nadrze | MW10 | int") && !tags.includes("I0.0"), tags);

  // ---- the imported tags read real values ----
  await ev(`(document.querySelector('#s7Test').click(), 1)`);
  let testOut = ""; for (let i = 0; i < 40; i++) { await sleep(250); testOut = await ev(`document.querySelector('#s7TestOut').textContent`); if (/^(OK|failed)/.test(testOut)) break; }
  expect("the test read answers OK", testOut.startsWith("OK"), testOut);
  expect("values at the calculated addresses: set point 21.5, recipe hello, level 1234", /^setpoint = 21\.5/m.test(testOut) && /p1_recipe = hello/.test(testOut) && /hladina_nadrze = 1234/.test(testOut) && /p1_running = (true|false)/.test(testOut), testOut);
  await shot("s7-import-form.png", process.env.DOCS ? "#connDlg" : null);
  await ev(`(() => { const f = document.querySelector('#newConn'); f.reason.value = 'headless probe'; document.querySelector('#connSubmit').click(); return 1; })()`); await sleep(2500);
  const conn = await ev(`fetch('/api/v1/connectors',{credentials:'same-origin'}).then(r=>r.json()).then(l => l.find(c => c.name === 'pumps-${suf}' && !c.revoked_at) || null)`);
  expect("the connector is created with the six tags, none writable", !!conn && conn.settings.tags.length === 6 && conn.settings.tags.every((t) => !t.writable), conn && conn.settings.tags);
  if (conn) { const csrf = await ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(j=>j.csrf_token)`);
    expect("cleanup: connector revoked", (await ev(`fetch('/api/v1/connectors/${conn.id}?reason=headless+probe',{method:'DELETE',credentials:'same-origin',headers:{'X-CSRF-Token':'${csrf}'}}).then(r=>r.status)`)) < 300); }
  expect("no script errors", problems.length === 0, problems);
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  stop(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); stop(); process.exit(1); });
