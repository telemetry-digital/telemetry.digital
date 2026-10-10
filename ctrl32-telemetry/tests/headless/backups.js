// Headless Chrome: System → Server → Backups (0.77.0, spec 032) shows the state with the offsite copy and the last
// restore test, the schedule with the daily/weekly/monthly retention, the offsite target form (only the fields of the
// chosen kind; secrets never filled in), the restore test, the stored backups with checksum and offsite columns and
// the protocol, whose rows open the steps of a run. No script errors; the Slovak translation; screenshots in English
// and Slovak. Runs against the agent simulation (run-all.sh starts the server with config-vpn.toml for this probe);
// it makes two backups and a restore test with a folder target of its own when the page has none.
const { spawn } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9368;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || os.tmpdir()) + "/cdp-profile-backups", "--window-size=1300,1600", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const shot = async (name, full) => { const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: !!full }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`);
  const api = (m, p, body) => ev(`fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:${body ? JSON.stringify(JSON.stringify(body)) : "undefined"}}).then(async r=>({status:r.status, body:await r.json().catch(()=>null)}))`);

  // ---- data: a folder target, two backups, a restore test (only when the page has none)
  const st0 = (await api("GET", "/api/v1/server/backups")).body;
  if (!st0.offsite.config.enabled) {
    const nas = fs.mkdtempSync(path.join(os.tmpdir(), "ctrl32-nas-")).replace(/\\/g, "/");
    console.log("schedule:", (await api("PUT", "/api/v1/server/backups/schedule", { enabled: true, hour: 3, minute: 15, keep: 14, keep_weekly: 4, keep_monthly: 12, passphrase: "headless passphrase", reason: "headless backups probe" })).status);
    console.log("target:", (await api("PUT", "/api/v1/server/backups/offsite", { config: { enabled: true, kind: "folder", path: nas }, reason: "headless backups probe", password: "admin1234" })).status);
    const idle = async () => { for (let i = 0; i < 30; i++) { const b = (await api("GET", "/api/v1/server/backups")).body; if (!b.running && !b.restore_running && !b.history.some((r) => r.running)) return; await sleep(700); } };
    for (let i = 0; i < 2; i++) { await api("POST", "/api/v1/server/backups", { use_stored: true, reason: "headless backups probe" }); await sleep(1200); await idle(); }
    await api("POST", "/api/v1/server/backups/restore-test", { reason: "headless backups probe" }); await sleep(800); await idle();
  }

  // ---- the page
  await send("Page.navigate", { url: base + "/server/backups" }); await sleep(3500);
  const b = await ev(`({kv: document.querySelector('#kvBk').textContent.replace(/\\s+/g,' '),
    keep: ['keep','keep_weekly','keep_monthly'].map(n => document.querySelector('#bkForm [name='+n+']').value).join('/'),
    kind: document.querySelector('#offForm [name=kind]').value,
    folderShown: !document.querySelector('#offForm [data-kind=folder]').hidden, sftpHidden: document.querySelector('#offForm [data-kind=sftp]').hidden,
    s3Hidden: document.querySelector('#offForm [data-kind=s3]').hidden, enabled: document.querySelector('#offForm [name=enabled]').checked,
    rt: document.querySelector('#kvRt').textContent.replace(/\\s+/g,' '), rtDay: document.querySelector('#rtForm [name=day]').value,
    files: [...document.querySelectorAll('#bkFiles tbody tr')].map(tr => tr.textContent.replace(/\\s+/g,' ')),
    runs: document.querySelectorAll('#bkRuns tbody tr[data-run]').length, sync: !document.querySelector('#offSyncBtn').hidden})`);
  console.log("page:", JSON.stringify({ ...b, kv: b.kv.slice(0, 200), files: b.files.length }));
  if (!/Offsite copy\s*folder /.test(b.kv) || !/Last restore test/.test(b.kv)) problems.push("state without the offsite copy or the restore test: " + b.kv);
  if (b.keep !== "14/4/12") problems.push("retention fields " + b.keep);
  if (b.kind !== "folder" || !b.folderShown || !b.sftpHidden || !b.s3Hidden || !b.enabled) problems.push("target form: " + JSON.stringify(b));
  if (!/monthly on day 1/.test(b.rt) || b.rtDay !== "1") problems.push("restore test card: " + b.rt);
  if (b.files.length < 2 || !b.files.every((f) => /yes/.test(f) && /…/.test(f))) problems.push("stored backups without checksum or offsite: " + JSON.stringify(b.files));
  if (b.runs < 3) problems.push("protocol rows " + b.runs);
  if (!b.sync) problems.push("no 'send missing backups' button with an enabled target");
  await shot("backups-en.png", true);

  // a run of the protocol opens with its steps
  await ev(`document.querySelector('#bkRuns tbody tr[data-run]').click()`); await sleep(400);
  const run = await ev(`({open: document.querySelector('#runDlg').open, title: document.querySelector('#runTitle').textContent, steps: [...document.querySelectorAll('#runSteps tbody tr')].map(tr => tr.cells[0].textContent)})`);
  console.log("run:", JSON.stringify(run));
  if (!run.open || run.steps.length < 3) problems.push("run dialog: " + JSON.stringify(run));
  await shot("backups-run-en.png");
  await ev(`document.querySelector('#runDlg').close()`);

  // the SFTP and S3 fields; secrets are never filled in
  await ev(`(f => { f.kind.value = 'sftp'; f.kind.dispatchEvent(new Event('change')); })(document.querySelector('#offForm'))`); await sleep(200);
  const sf = await ev(`({sftp: !document.querySelector('#offForm [data-kind=sftp]').hidden, folder: document.querySelector('#offForm [data-kind=folder]').hidden, pw: document.querySelector('#offForm [name=password_sftp]').type, pwv: document.querySelector('#offForm [name=password_sftp]').value, key: document.querySelector('#offForm [name=host_key]').placeholder})`);
  if (!sf.sftp || !sf.folder || sf.pw !== "password" || sf.pwv !== "" || !/SHA256/.test(sf.key)) problems.push("sftp fields: " + JSON.stringify(sf));
  await ev(`(f => { f.kind.value = 's3'; f.kind.dispatchEvent(new Event('change')); })(document.querySelector('#offForm'))`); await sleep(200);
  const s3 = await ev(`({s3: !document.querySelector('#offForm [data-kind=s3]').hidden, sk: document.querySelector('#offForm [name=secret_key]').type})`);
  if (!s3.s3 || s3.sk !== "password") problems.push("s3 fields: " + JSON.stringify(s3));
  await shot("backups-s3-en.png");
  // the save asks for the password
  await ev(`document.querySelector('#offForm').requestSubmit()`); await sleep(300);
  const dlg = await ev(`({open: document.querySelector('#offDlg').open, pw: !!document.querySelector('#offSave [name=password]')})`);
  if (!dlg.open || !dlg.pw) problems.push("saving the target does not ask for the password");
  await ev(`document.querySelector('#offDlg').close()`);

  // ---- Slovak
  const langs0 = await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  const putLangs = (enabled, dflt) => ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify(enabled)},default:${JSON.stringify(dflt)},reason:"headless backups probe"})}).then(r=>r.status)`);
  if (!langs0.enabled.includes("sk")) console.log("enable sk:", await putLangs([...langs0.enabled, "sk"], langs0.default));
  await ev(`localStorage.setItem("ctrl32.locale", "sk")`);
  await send("Page.navigate", { url: base + "/server/backups" }); await sleep(3500);
  const sk = await ev(`document.querySelector('section[data-section=backups]').textContent.replace(/\\s+/g,' ')`);
  const skOK = /Kópia mimo servera/.test(sk) && /Test obnovy/.test(sk) && /Protokol/.test(sk) && /Mesačný test obnovy/.test(sk);
  console.log("sk backups:", skOK ? "translated" : "NOT translated");
  if (!skOK) problems.push("backups not translated to Slovak");
  await shot("backups-sk.png", true);
  await ev(`localStorage.setItem("ctrl32.locale", "en")`);
  if (!langs0.enabled.includes("sk")) { await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`); console.log("restore languages:", await putLangs(langs0.enabled, langs0.default)); }
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
