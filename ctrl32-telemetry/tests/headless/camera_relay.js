// Headless Chrome: a camera behind a relay (0.58.0) — the camera page with the stream information (main and
// sub-stream, the overlay on the picture) and the PTZ control reached through the relay, the camera management table
// and the camera form of a relayed camera; screenshots.
//   BASE=http://127.0.0.1:8080 CAM=<camera id> OUT=/tmp node tests/headless/camera_relay.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const cam = process.env.CAM;
const port = 9351;
if (!cam) { console.error("CAM=<camera id> is required"); process.exit(2); }
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--autoplay-policy=no-user-gesture-required", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile-relay", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 120; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
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
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); console.log("screenshot:", out + "/" + name); };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await ev(`localStorage.setItem('ctrl32.video.stats','1'); localStorage.setItem('ctrl32.ptz.shown','1')`);

  // the camera page: stream information and PTZ through the relay
  await send("Page.navigate", { url: base + "/video/cameras/" + cam }); await sleep(9000);
  console.log("streams:", await ev(`document.querySelector('#streams').textContent`));
  console.log("overlay:", await ev(`(() => { const e = document.querySelector('.vp-stats'); return e && !e.hidden ? e.textContent : 'hidden'; })()`));
  console.log("relay note hidden:", await ev(`document.querySelector('#relayNote').hidden`), "| PTZ button:", await ev(`!document.querySelector('#bPtz').hidden`), "| PTZ pad:", await ev(`!!document.querySelector('.ptz') && !document.querySelector('.ptz').hidden`));
  console.log("presets:", await ev(`[...document.querySelectorAll('#presets option')].map(o => o.textContent).join(', ')`));
  console.log("preset through the relay:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; return (await fetch('/api/v1/video/cameras/${cam}/ptz',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({action:'preset',preset:'3'})})).status; })()`));
  await sleep(2500);
  await shot("camera-relay-page.png");

  // live tiles with the stream information
  await send("Page.navigate", { url: base + "/video" }); await sleep(7000);
  console.log("tile overlays:", JSON.stringify(await ev(`[...document.querySelectorAll('.cam-tile .vp-stats')].map(e => e.hidden ? 'hidden' : e.textContent)`)));
  await shot("camera-relay-tiles.png");

  // management: main and sub-stream, the camera form of a relayed camera
  await send("Page.navigate", { url: base + "/video/manage" }); await sleep(4000);
  console.log("table:", JSON.stringify(await ev(`[...document.querySelectorAll('#cams tbody tr')].map(r => r.cells[3].innerText.replace(/\\n/g, ' / '))`)));
  await shot("camera-relay-manage.png");
  console.log("form:", await ev(`(() => { document.querySelector('[data-cedit="${cam}"]').click(); const f = document.querySelector('#camForm'); const vis = (el) => !!el && el.offsetParent !== null; return JSON.stringify({onvif: f.onvif_url.value, user: f.username.value, onvifVisible: vis(f.onvif_url), urlVisible: vis(f.url), hint: vis(document.querySelector('#camForm .push-only.muted[style]')) }); })()`));
  await sleep(1500);
  console.log("fill in from ONVIF (through the relay):", await ev(`(async () => { document.querySelector('#profBtn').click(); await new Promise(r => setTimeout(r, 4000)); return document.querySelector('#onvifOut').textContent; })()`));
  await ev(`document.querySelector('#camForm [name=onvif_url]').scrollIntoView({block: 'center'})`); await sleep(500);
  await shot("camera-relay-form.png");
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
