// Headless Chrome: Incidents page lists incidents, opens the detail dialog, records a cause through the form and
// changes the status through the workflow button; the navigation shows the entry; screenshots.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9350;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile18", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify((r.result.exceptionDetails.exception || {}).description) } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  const suf = Math.floor(Math.random() * 1e5);
  const incId = await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const r = await fetch('/api/v1/incidents',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({title:'Probe incident ${suf}',severity:'major',impact:'probe impact',reason:'probe'})}); return (await r.json()).id; })()`);
  console.log("created:", incId);
  await send("Page.navigate", { url: base + "/incidents" }); await sleep(3500);
  console.log("nav entry:", await ev(`!![...document.querySelectorAll('nav a, aside a, .nav a')].find(a=>a.getAttribute('href')==='/incidents')`), "| rows:", await ev(`document.querySelectorAll('#list tbody tr[data-id]').length`), "| probe row:", await ev(`[...document.querySelectorAll('#list tbody tr')].some(tr=>tr.textContent.includes('Probe incident ${suf}'))`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/incidents-list.png", Buffer.from(s.result.data, "base64"));
  await ev(`[...document.querySelectorAll('#list tbody tr[data-id]')].find(tr=>tr.textContent.includes('Probe incident ${suf}')).click()`); await sleep(1500);
  console.log("dialog open:", await ev(`document.querySelector('#dlg').open`), "| title:", await ev(`document.querySelector('#dTitle').textContent`), "| actions:", await ev(`[...document.querySelectorAll('#dActions button')].map(b=>b.textContent.trim()).join(' | ')`), "| events:", await ev(`document.querySelectorAll('#dEvents li').length`));
  await ev(`(() => { const f = document.querySelector('#edit'); f.cause.value = 'probe cause'; f.note.value = 'probe note'; f.reason.value = 'probe'; f.requestSubmit(); })()`); await sleep(2500);
  console.log("after save: cause persisted:", await ev(`document.querySelector('#edit').cause.value`), "| events:", await ev(`document.querySelectorAll('#dEvents li').length`));
  await ev(`(() => { const f = document.querySelector('#edit'); f.reason.value = 'probe resolve'; [...document.querySelectorAll('#dActions [data-status]')].find(b=>b.dataset.status==='resolved').click(); })()`); await sleep(2500);
  console.log("status after resolve:", await ev(`document.querySelector('#dMeta').textContent.includes('resolved')`), "| resolve badge:", await ev(`!!document.querySelector('#dMeta .badge.ok')`));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/incidents-detail.png", Buffer.from(s2.result.data, "base64"));
  console.log("api status:", await ev(`fetch('/api/v1/incidents/${incId}',{credentials:'same-origin'}).then(r=>r.json()).then(j=>j.incident.status+' events='+j.events.length)`));
  console.log("cleanup:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; return (await fetch('/api/v1/incidents/${incId}',{method:'PATCH',headers:h,credentials:'same-origin',body:JSON.stringify({capa:'probe capa',status:'closed',reason:'probe cleanup'})})).status; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
