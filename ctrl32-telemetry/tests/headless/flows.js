// Headless Chrome: the flow editor (spec 004) with real mouse events. The probe creates an empty flow, adds an inject,
// a math and a debug node from the palette, draws the wires by dragging from output to input ports, sets the math
// expression in the property panel, saves and deploys with reasons, sends a test message with the node's button and
// reads the result in the debug panel; then checks that a problem (an empty expression) is marked on the node.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9365;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile25", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const key = async (k, code) => { await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); };

  const mouse = async (type, x, y) => send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1, pointerType: "mouse" });
  const dragFromTo = async (a, b) => { await mouse("mouseMoved", a.x, a.y); await mouse("mousePressed", a.x, a.y); for (let i = 1; i <= 8; i++) await mouse("mouseMoved", a.x + (b.x - a.x) * i / 8, a.y + (b.y - a.y) * i / 8); await mouse("mouseReleased", b.x, b.y); await sleep(200); };
  const center = (sel) => ev(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); if (!e) return null; const r = e.getBoundingClientRect(); return {x: r.left + r.width / 2, y: r.top + r.height / 2}; })()`);
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await send("Page.navigate", { url: base + "/flows" }); await sleep(2000);
  const name = "Probe flow " + Math.floor(Math.random() * 1e6);
  await ev(`(() => { document.querySelector('#newBtn').click(); const f = document.querySelector('#newForm'); f.name.value = ${JSON.stringify(name)}; f.reason.value = 'probe'; f.requestSubmit(); })()`); await sleep(3000);
  expect("editor opened", await ev(`location.pathname.startsWith('/flows/') && !!document.querySelector('.fe-svg')`), true);
  // add three nodes by double-clicking the palette
  for (const t of ["inject", "math", "debug"]) { await ev(`(() => { const it = document.querySelector('.fe-pal-item[data-type="${t}"]'); it.dispatchEvent(new MouseEvent('dblclick', {bubbles: true})); })()`); await sleep(200); }
  expect("three nodes", await ev(`window.__flowEditor.spec.nodes.length`), 3);
  // spread them out by dragging the node bodies
  const ids = await ev(`window.__flowEditor.spec.nodes.map(n => n.id)`);
  for (let i = 0; i < 3; i++) { const c = await center(`.fe-node[data-id="${ids[i]}"] .fe-body`); await dragFromTo(c, { x: 560 + i * 230, y: 300 + (i % 2) * 40 }); }
  const xs = await ev(`window.__flowEditor.spec.nodes.map(n => n.x)`);
  expect("nodes moved apart", xs, (a) => Array.isArray(a) && a[1] - a[0] > 120 && a[2] - a[1] > 120);
  // wires: output port of 0 → node 1, output of 1 → node 2
  for (let i = 0; i < 2; i++) { const a = await center(`.fe-node[data-id="${ids[i]}"] [data-out]`), b = await center(`.fe-node[data-id="${ids[i + 1]}"] [data-in]`); await dragFromTo(a, b); }
  expect("two wires drawn with the mouse", await ev(`JSON.stringify(window.__flowEditor.spec.wires.map(w => w.from + '>' + w.to))`), JSON.stringify([ids[0] + ">" + ids[1], ids[1] + ">" + ids[2]]));
  // settings of the math node through the property panel
  await ev(`window.__flowEditor.select(['${ids[1]}'])`); await sleep(200);
  await ev(`(() => { const i = document.querySelector('.fe-props [data-s="expr"]'); i.value = 'value * 2 + 1'; i.dispatchEvent(new Event('change', {bubbles: true})); const n = document.querySelector('.fe-props [data-name]'); n.value = 'Double'; n.dispatchEvent(new Event('change', {bubbles: true})); })()`); await sleep(200);
  expect("expression set", await ev(`window.__flowEditor.spec.nodes[1].settings.expr + ' / ' + window.__flowEditor.spec.nodes[1].name`), "value * 2 + 1 / Double");
  await ev(`window.__flowEditor.select(['${ids[0]}'])`); await sleep(150);
  await ev(`(() => { const i = document.querySelector('.fe-props [data-s="value"]'); i.value = '20'; i.dispatchEvent(new Event('change', {bubbles: true})); })()`);
  expect("unsaved marker", await ev(`!document.querySelector('#fDirty').hidden`), true);
  // undo/redo
  await ev(`window.__flowEditor.undo()`); expect("undo", await ev(`window.__flowEditor.spec.nodes[0].settings.value`), "0");
  await ev(`window.__flowEditor.redo()`); expect("redo", await ev(`window.__flowEditor.spec.nodes[0].settings.value`), "20");
  // deploy (saves the new version first)
  await ev(`document.querySelector('#tDeploy').click()`); await sleep(1200);
  await ev(`(() => { const f = document.querySelector('#reasonForm'); f.reason.value = 'probe deploy'; f.requestSubmit(); })()`); await sleep(2500);
  expect("deployed and running", await ev(`document.querySelector('#fState').textContent`), (t) => typeof t === "string" && t.includes("running v2"));
  // test message with the node's own button (a real click on the ▶)
  await ev(`window.__flowEditor.fit()`); await sleep(300);
  const inj = await center(`.fe-node[data-id="${ids[0]}"] [data-inject] rect`);
  await mouse("mouseMoved", inj.x, inj.y); await mouse("mousePressed", inj.x, inj.y); await mouse("mouseReleased", inj.x, inj.y); await sleep(500);
  expect("test dialog", await ev(`document.querySelector('#injDlg').open`), true);
  await ev(`document.querySelector('#injForm').requestSubmit()`); await sleep(2500);
  expect("debug shows 41", await ev(`[...document.querySelectorAll('#dbgList .fe-dbg.debug .fe-dbg-text')].map(e => e.textContent)`), (a) => Array.isArray(a) && a.includes("41"));
  await sleep(3200);
  expect("node counters on the canvas", await ev(`[...document.querySelectorAll('.fe-badge')].map(e => e.textContent).join('|')`), (t) => typeof t === "string" && t.includes("in 1"));
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/flow-editor-probe.png", Buffer.from(s1.result.data, "base64"));
  // a problem is marked on its node
  await ev(`window.__flowEditor.select(['${ids[1]}'])`); await sleep(150);
  await ev(`(() => { const i = document.querySelector('.fe-props [data-s="expr"]'); i.value = 'value *'; i.dispatchEvent(new Event('change', {bubbles: true})); })()`);
  await ev(`document.querySelector('#tDeploy').click()`); await sleep(1500);
  expect("deploy refused with the problem", await ev(`document.querySelector('#reasonProblems').textContent`), (t) => typeof t === "string" && t.includes("expression"));
  expect("node marked", await ev(`document.querySelector('.fe-node[data-id="${ids[1]}"]').classList.contains('bad')`), true);
  await ev(`document.querySelector('#reasonDlg').close()`);
  // clean up
  await ev(`(async () => { const slug = location.pathname.split('/')[2]; await fetch('/api/v1/flows/' + slug + '?reason=probe', {method: 'DELETE', credentials: 'same-origin', headers: {'X-CSRF-Token': ctrl32.csrf}}); })()`);
  console.log(problems.length ? "problems: " + JSON.stringify(problems) : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
