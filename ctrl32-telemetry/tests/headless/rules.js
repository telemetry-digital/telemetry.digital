// Headless Chrome: open Assets → Automation, build a rule through the editor (input, condition, notify + derive
// actions), run "Test now", submit, screenshot the list and the run log. Usage: node cdp-rules.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9342;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile10", "--window-size=1440,1200", "about:blank"], { stdio: "ignore" });
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
  const suf = Math.floor(Math.random() * 1e5);
  // fixtures through the API: a channel and two datastreams on a fresh asset
  console.log("fixtures:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const post = (u, b) => fetch(u,{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify(b)}).then(r=>r.json());
    const ch = await post('/api/v1/notification-channels',{name:'probe-hook-${suf}',kind:'webhook',settings:{url:'http://127.0.0.1:1/x'},reason:'probe'});
    const site = await post('/api/v1/sites',{name:'Probe site ${suf}',reason:'probe'}); const asset = await post('/api/v1/assets',{site_id:site.id,type:'room',name:'Probe room ${suf}',reason:'probe'});
    const a = await post('/api/v1/datastreams',{asset_id:asset.id,key:'t_in',quantity:'temperature',unit:'C',kind:'gauge',reason:'probe'}); const b = await post('/api/v1/datastreams',{asset_id:asset.id,key:'t_diff',quantity:'temperature',unit:'K',kind:'gauge',reason:'probe'});
    window.__P = {ch: ch.id, a: a.id, b: b.id}; return JSON.stringify(window.__P); })()`));
  const P = JSON.parse(await ev(`JSON.stringify(window.__P)`));
  await send("Page.navigate", { url: base + "/assets/automation" }); await sleep(3000);
  console.log("nav has Automation:", await ev(`!![...document.querySelectorAll('#sidebar a')].find(a => a.getAttribute('href') === '/assets/automation')`));
  console.log("open editor:", await ev(`(() => { document.querySelector('#newAutoBtn').click(); return document.querySelector('#autoDlg').open; })()`));
  console.log("build rule:", await ev(`(() => { const f = document.querySelector('#autoForm'); f.name.value = 'probe-rule-${suf}'; f.description.value = 'built in the browser';
    document.querySelector('#autoAddInput').click(); const inSel = document.querySelector('#autoInputs select'); inSel.value = '${P.a}'; inSel.dispatchEvent(new Event('change', {bubbles:true}));
    const nameInp = document.querySelector('#autoInputs input[data-f=name]'); nameInp.value = 't_in'; nameInp.dispatchEvent(new Event('change', {bubbles:true}));
    document.querySelector('#autoAddCond').click(); const val = document.querySelector('#autoConds input[data-f=value]'); val.value = '30'; val.dispatchEvent(new Event('input', {bubbles:true}));
    document.querySelector('#autoAddAct').click(); const chSel = document.querySelector('#autoActs select[data-f=channel_id]'); chSel.value = '${P.ch}'; chSel.dispatchEvent(new Event('change', {bubbles:true}));
    document.querySelector('#autoAddAct').click(); const t = document.querySelectorAll('#autoActs select[data-f=type]')[1]; t.value = 'derive'; t.dispatchEvent(new Event('change', {bubbles:true}));
    const ds = document.querySelector('#autoActs select[data-f=datastream_id]'); ds.value = '${P.b}'; ds.dispatchEvent(new Event('change', {bubbles:true})); const ex = document.querySelector('#autoActs input[data-f=expr]'); ex.value = 't_in * 2'; ex.dispatchEvent(new Event('input', {bubbles:true}));
    f.reason.value = 'probe'; return JSON.stringify(edSpec()); })()`));
  console.log("test now:", await ev(`(async () => { document.querySelector('#autoTestBtn').click(); await new Promise(r => setTimeout(r, 1500)); return document.querySelector('#autoTestOut').textContent + ' | err=' + document.querySelector('#autoErr').textContent; })()`));
  console.log("submit:", await ev(`(async () => { document.querySelector('#autoForm').requestSubmit(); await new Promise(r => setTimeout(r, 2000)); return 'dialog open=' + document.querySelector('#autoDlg').open + ' err=' + document.querySelector('#autoErr').textContent; })()`));
  console.log("rows:", JSON.stringify(await ev(`[...document.querySelectorAll('#autoTable tbody tr')].map(r => r.innerText.replace(/\\t/g, ' | ').replace(/\\n/g, ' / ').slice(0, 160)).filter(t => t.includes('${suf}'))`)));
  console.log("edit reopens with values:", await ev(`(() => { const b = [...document.querySelectorAll('[data-edit-rule]')].find(b => b.closest('tr').innerText.includes('probe-rule-${suf}')); b.click(); const f = document.querySelector('#autoForm'); const r = {name: f.name.value, inputs: document.querySelectorAll('#autoInputs .row').length, conds: document.querySelectorAll('#autoConds .row').length, acts: document.querySelectorAll('#autoActs .row').length, title: document.querySelector('#autoTitle').textContent}; document.querySelector('#autoDlg').close(); return JSON.stringify(r); })()`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/rules.png", Buffer.from(s.result.data, "base64"));
  console.log("open editor for screenshot:", await ev(`(() => { const b = [...document.querySelectorAll('[data-edit-rule]')].find(b => b.closest('tr').innerText.includes('probe-rule-${suf}')); b.click(); return document.querySelector('#autoDlg').open; })()`));
  await sleep(500);
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/rules-editor.png", Buffer.from(s2.result.data, "base64"));
  console.log("cleanup:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const rs = await (await fetch('/api/v1/rules',{credentials:'same-origin'})).json(); const r = rs.find(x => x.name === 'probe-rule-${suf}'); const d = await fetch('/api/v1/rules/' + r.id + '?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'}); const c = await fetch('/api/v1/notification-channels/${P.ch}?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'}); return d.status + ' ' + c.status; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
