// Headless Chrome: open dashboards (logged in on a base, or public links) and screenshot each; count widgets and errors.
// usage: BASE=http://127.0.0.1:8080 LOGIN=1 SLUGS=wall-prehlad,p4-zony OUT=dir node cdp-dash.js
//        URLS=https://telemetry.digital/p/<token> OUT=dir node cdp-dash.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const slugs = (process.env.SLUGS || "").split(",").filter(Boolean);
const urls = (process.env.URLS || "").split(",").filter(Boolean);
const port = 9351;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile19", "--window-size=1600,1100", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  if (process.env.LOGIN) {
    await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
    console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  }
  const list = slugs.length ? slugs.map((s) => [s, base + "/dashboards/" + s]) : urls.map((u, i) => ["public-" + (i + 1), u]);
  for (const [name, url] of list) {
    problems = [];
    await send("Page.navigate", { url }); await sleep(5000);
    const widgets = await ev(`document.querySelectorAll('#vGrid .widget, #vGrid .mimic-widget, .grid .widget, .mimic-widget').length`);
    const errs = await ev(`[...document.querySelectorAll('.widget .err, .widget .error, .widget-err')].map(e=>e.textContent.trim()).filter(Boolean).slice(0,5)`);
    const title = await ev(`document.title`);
    await send("Emulation.setDeviceMetricsOverride", { width: 1600, height: 1100, deviceScaleFactor: 1, mobile: false });
    const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true });
    fs.writeFileSync(out + "/dash-" + name + ".png", Buffer.from(s.result.data, "base64"));
    console.log(name + ": title=" + JSON.stringify(title) + " widgets=" + widgets + " errors=" + JSON.stringify(errs) + " problems=" + (problems.length ? JSON.stringify(problems) : "none"));
  }
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
