// Headless Chrome: Settings → Connectors — create a register profile through the dialog, open the connector
// dialog, pick the profile (registers fill in read-only), screenshot; then retire the probe profile.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9346;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile14", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; let problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await send("Page.navigate", { url: base + "/settings/connectors" }); await sleep(3000);
  const suf = Math.floor(Math.random() * 1e5);
  console.log("profile dialog:", await ev(`(() => { document.querySelector('#newProfBtn').click(); const f = document.querySelector('#profForm'); f.name.value = 'probe-erc-${suf}'; f.description.value = 'cold room controller'; f.registers.value = 'temp | holding | 0 | int16 | 0.1\\nsetpoint | holding | 1 | int16 | 0.1 | 0 | rw | 10 | 40'; f.reason.value = 'probe'; f.requestSubmit(); return document.querySelector('#profDlg').open; })()`)); await sleep(2000);
  console.log("profiles:", JSON.stringify(await ev(`[...document.querySelectorAll('#profTable tbody tr')].map(r => r.innerText.replace(/\\t/g, ' | ').replace(/\\n/g, ' / ').slice(0, 120)).filter(t => t.includes('probe-erc-${suf}'))`)), "err:", await ev(`document.querySelector('#profErr').textContent`));
  console.log("connector form with profile:", await ev(`(() => { document.querySelector('#newConnBtn').click(); const f = document.querySelector('#newConn'); f.kind.value = 'modbus_tcp'; f.kind.dispatchEvent(new Event('change')); const opt = [...f.profile.options].find(o => o.textContent.startsWith('probe-erc-${suf}')); f.profile.value = opt.value; f.profile.onchange(); return 'readonly=' + f.registers.readOnly + ' lines=' + f.registers.value.split('\\n').length + ' settings.profile_id=' + (mbSettings(f).profile_id === opt.value); })()`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/profiles.png", Buffer.from(s.result.data, "base64"));
  console.log("cleanup:", await ev(`(async () => { document.querySelector('#connDlg').close(); const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const ps = await (await fetch('/api/v1/register-profiles',{credentials:'same-origin'})).json(); const p = ps.find(x => x.name === 'probe-erc-${suf}'); return (await fetch('/api/v1/register-profiles/' + p.id + '?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
