// Headless Chrome: open /api/docs anonymously and signed in, expand an operation, filter, screenshot.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9343;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile11", "--window-size=1440,1100", "about:blank"], { stdio: "ignore" });
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
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/api/docs" }); await sleep(3000);
  console.log("anonymous: ops rendered:", await ev(`document.querySelectorAll('details.op').length`), "nav entries:", await ev(`document.querySelectorAll('#nav a').length`), "schemas:", await ev(`document.querySelectorAll('#schemas h3').length`));
  console.log("expand rules POST:", await ev(`(() => { const d = document.querySelector('#op-postRules'); d.open = true; d.scrollIntoView(); return d.querySelector('.op-body').innerText.slice(0, 400).replace(/\\n+/g, ' / '); })()`));
  await sleep(300);
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/docs.png", Buffer.from(s.result.data, "base64"));
  console.log("filter 'modbus':", await ev(`(() => { const q = document.querySelector('#q'); q.value = 'connector'; q.dispatchEvent(new Event('input')); return [...document.querySelectorAll('details.op:not(.hidden)')].length + ' visible, sections ' + [...document.querySelectorAll('section.tag:not(.hidden):not(.schema)')].length; })()`));
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await send("Page.navigate", { url: base + "/login" }); await sleep(1000); await send("Page.navigate", { url: base + "/api/docs?signed=1#op-getStream" }); await sleep(3000);
  console.log("signed in: nav link present:", await ev(`!![...document.querySelectorAll('#sidebar a')].find(a => a.getAttribute('href') === '/api/docs')`), "deep link opened:", await ev(`document.querySelector('#op-getStream').open`));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/docs-signed.png", Buffer.from(s2.result.data, "base64"));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
