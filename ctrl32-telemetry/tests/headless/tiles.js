// Headless Chrome: a dashboard with a map widget loads its tiles from the server's PMTiles archive (/tiles/…),
// the attribution comes from config, Settings → Branding shows the "self-hosted archive active" hint; screenshots.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const slug = process.env.SLUG || "showcase-2906";
const port = 9348;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile16", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; let problems = []; const tiles = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Network.responseReceived" && msg.params.response.url.includes("/tiles/")) tiles.push({ url: msg.params.response.url.replace(base, ""), status: msg.params.response.status, type: msg.params.response.mimeType, cache: (msg.params.response.headers["Cache-Control"] || msg.params.response.headers["cache-control"] || "") });
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable"); await send("Network.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception && r.result.exceptionDetails.exception.description) } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await send("Page.navigate", { url: base + "/dashboards/" + slug }); await sleep(5000);
  console.log("brand map:", await ev(`JSON.stringify(window.ctrl32.brand.map)`));
  console.log("leaflet tiles in DOM:", await ev(`document.querySelectorAll('img.leaflet-tile').length`), "from /tiles:", await ev(`[...document.querySelectorAll('img.leaflet-tile')].filter(i=>i.src.includes('/tiles/')).length`), "loaded 256px:", await ev(`[...document.querySelectorAll('img.leaflet-tile')].filter(i=>i.complete && i.naturalWidth===256).length`));
  console.log("attribution:", await ev(`(document.querySelector('.leaflet-control-attribution')||{}).textContent`));
  console.log("tile responses:", tiles.length, "statuses:", JSON.stringify([...new Set(tiles.map(t=>t.status+" "+t.type+" "+t.cache))]), "sample:", tiles.slice(0, 2).map(t=>t.url).join(" "));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/tiles-map.png", Buffer.from(s.result.data, "base64"));
  await send("Page.navigate", { url: base + "/settings#branding" }); await sleep(3000);
  console.log("settings hint visible:", await ev(`!document.querySelector('#mapLocal').hidden`), "text:", await ev(`document.querySelector('#mapLocal').textContent.replace(/\\s+/g,' ').trim()`));
  console.log("error hint hidden:", await ev(`document.querySelector('#mapLocalErr').hidden`), "tile url field empty:", await ev(`document.querySelector('#brand [name=map_tile_url]').value===''`));
  console.log("sk translation:", await ev(`fetch('/api/v1/i18n/sk',{credentials:'same-origin'}).then(r=>r.json()).then(j=>(j.entries||j)['Self-hosted map archive is active'])`));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/tiles-settings.png", Buffer.from(s2.result.data, "base64"));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
