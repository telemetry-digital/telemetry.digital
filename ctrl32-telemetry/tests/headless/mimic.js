// Headless Chrome: create a mimic from the starter, view it (canvas scaled, symbols rendered as SVG), edit it
// (select a symbol, change x through the properties, drag another with pointer events, save a new version), screenshot.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9347;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile15", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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
  const slug = await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const st = await (await fetch('/api/v1/dashboards/starter?kind=mimic',{credentials:'same-origin'})).json(); const r = await (await fetch('/api/v1/dashboards',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({name:'Probe mimic ${suf}', spec: st.spec, reason:'probe'})})).json(); return r.slug; })()`);
  console.log("created:", slug);
  await send("Page.navigate", { url: base + "/dashboards/" + slug }); await sleep(3500);
  console.log("view: canvas:", await ev(`!!document.querySelector('#vGrid .mimic')`), "widgets:", await ev(`document.querySelectorAll('#vGrid .mimic-widget').length`), "svg symbols:", await ev(`document.querySelectorAll('#vGrid .mimic-widget svg').length`), "scale:", await ev(`document.querySelector('#vGrid .mimic').style.transform`), "flow:", await ev(`document.querySelectorAll('.mimic-flow').length`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/mimic-view.png", Buffer.from(s.result.data, "base64"));
  await send("Page.navigate", { url: base + "/dashboards/" + slug + "/edit" }); await sleep(3500);
  console.log("edit: hint canvas:", await ev(`!document.querySelector('#eHintCanvas').hidden`), "cards editing:", await ev(`document.querySelectorAll('#eGrid .mimic-widget.editing').length`));
  console.log("props x → 500:", await ev(`(() => { const cards = [...document.querySelectorAll('#eGrid .mimic-widget')]; const i = cards.findIndex(c => SPEC.widgets[+c.dataset.i].type === 'tank'); cards[i].click(); const inp = document.querySelector('#eProps [data-g=x]'); inp.value = 500; inp.dispatchEvent(new Event('change', {bubbles:true})); return SPEC.widgets[i].geometry.x + ' left=' + [...document.querySelectorAll('#eGrid .mimic-widget')][i].style.left; })()`));
  await sleep(2000);
  console.log("drag pump by pointer:", await ev(`(() => { const cards = [...document.querySelectorAll('#eGrid .mimic-widget')]; const i = cards.findIndex(c => SPEC.widgets[+c.dataset.i].type === 'pump'); const card = cards[i]; const g = SPEC.widgets[i].geometry; const before = g.x + ',' + g.y; const r = card.getBoundingClientRect(); const down = new PointerEvent('pointerdown', {clientX: r.left + 20, clientY: r.top + 20, bubbles: true, pointerId: 1}); card.dispatchEvent(down); card.dispatchEvent(new PointerEvent('pointermove', {clientX: r.left + 120, clientY: r.top + 60, bubbles: true, pointerId: 1})); card.dispatchEvent(new PointerEvent('pointerup', {clientX: r.left + 120, clientY: r.top + 60, bubbles: true, pointerId: 1})); return before + ' -> ' + g.x + ',' + g.y; })()`));
  await sleep(800);
  console.log("save dialog:", await ev(`(() => { document.querySelector('#eSave').click(); const f = document.querySelector('#saveForm'); f.reason.value = 'probe move'; setTimeout(() => f.requestSubmit(), 100); return document.querySelector('#saveDlg').open; })()`));
  await sleep(4000);
  console.log("version after save:", await ev(`fetch('/api/v1/dashboards/${slug}',{credentials:'same-origin'}).then(r=>r.json()).then(j=>j.version + ' tank.x=' + j.spec.widgets.find(w=>w.type==='tank').geometry.x)`));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/mimic-edit.png", Buffer.from(s2.result.data, "base64"));
  console.log("cleanup:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; return (await fetch('/api/v1/dashboards/${slug}?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
