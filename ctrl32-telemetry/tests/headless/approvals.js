// Headless Chrome: Settings → Security policy (three checkboxes) and Settings → Approvals with a pending change
// created through the API; screenshot. The admin sees its own request without Approve buttons.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9345;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile13", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
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
  console.log("policy on + request:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const j = (u, m, b) => fetch(u,{method:m,headers:h,credentials:'same-origin',body:JSON.stringify(b)}).then(r=>r.json());
    await j('/api/v1/settings/policy','PUT',{four_eyes_limits:true,reason:'probe'}); const ds = (await (await fetch('/api/v1/datastreams',{credentials:'same-origin'})).json())[0]; const r = await j('/api/v1/alarm-rules','POST',{datastream_id: ds.id, type:'high', action: 40, reason:'probe change'}); window.__CR = r.change_request && r.change_request.id; return r.status + ' ' + (r.change_request||{}).summary; })()`));
  await send("Page.navigate", { url: base + "/settings/policy" }); await sleep(2500);
  console.log("policy checkboxes:", await ev(`[...document.querySelectorAll('#policyForm input[type=checkbox]')].map(i => i.name + '=' + i.checked).join(' ')`));
  await send("Page.navigate", { url: base + "/settings/approvals" }); await sleep(2500);
  console.log("approvals rows:", JSON.stringify(await ev(`[...document.querySelectorAll('#crTable tbody tr')].map(r => r.innerText.replace(/\\t/g, ' | ').replace(/\\n/g, ' / ').slice(0, 140))`)), "| nav:", await ev(`!![...document.querySelectorAll('#sidebar a')].find(a => a.getAttribute('href') === '/settings/approvals')`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/approvals.png", Buffer.from(s.result.data, "base64"));
  console.log("cleanup:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const j = (u, m, b) => fetch(u,{method:m,headers:h,credentials:'same-origin',body:JSON.stringify(b)}).then(r=>r.status); const a = await j('/api/v1/change-requests/' + window.__CR + '/reject','POST',{reason:'probe done'}); const b = await j('/api/v1/settings/policy','PUT',{four_eyes_limits:false,reason:'probe'}); return a + ' ' + b; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
