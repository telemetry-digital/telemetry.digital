// Headless Chrome: a live picture that stands while its stream stays connected opens the stream again by itself
// (0.87.5; a tile of a camera wall stood until someone restarted the browser). The probe drops the pictures of a
// live player's connection (it stays open), checks that the player reopens its WebSocket after about 20 s and plays
// again once pictures arrive; a picture that moves is never reopened. Starts the camera simulator itself (built
// with Go from the repository root) unless SIM names one.
//   BASE=http://127.0.0.1:8080 node tests/headless/video_stall.js
const { spawn, execFileSync } = require("child_process");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
let sim = process.env.SIM, simProc = null;
if (!sim) {
  const bin = (process.env.TEMP || "/tmp") + "/camsim-stall" + (process.platform === "win32" ? ".exe" : "");
  execFileSync("go", ["build", "-o", bin, "./tools/camsim"], { stdio: "inherit" });
  simProc = spawn(bin, ["-rtsp", "127.0.0.1:18564", "-http", "127.0.0.1:18565", "-fps", "10"], { stdio: "ignore" });
  sim = "rtsp://127.0.0.1:18564";
}
const port = 9364;
const proc = spawn(chrome, ["--headless=new", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-stall", "--window-size=1280,800", "--autoplay-policy=no-user-gesture-required", "about:blank"], { stdio: "ignore" });
const stop = () => { proc.kill(); if (simProc) simProc.kill(); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
let pass = 0, fail = 0, cleanup = async () => {}; const problems = [];
const check = (name, got, want) => { if (JSON.stringify(got) === JSON.stringify(want)) { pass++; console.log("  ok   " + name); } else { fail++; console.log(`  FAIL ${name} (got: ${JSON.stringify(got)}, want: ${JSON.stringify(want)})`); } };
(async () => {
  let targets;
  for (let i = 0; i < 120; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {};
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); if (!r.result) return { exception: (r.error && r.error.message) || "no result" }; return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const waitFor = (expr, ms) => ev(`new Promise(r => { const t0 = Date.now(), t = setInterval(() => { let v = false; try { v = !!(${expr}); } catch (e) { /* not yet */ } if (v || Date.now() - t0 > ${ms}) { clearInterval(t); r(v); } }, 250); })`);
  const api = (m, p, b) => ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(me=>fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token},body:${b ? JSON.stringify(JSON.stringify(b)) : "undefined"}})).then(async r=>({status:r.status, body: await r.json().catch(()=>null)}))`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  const suf = Date.now() % 100000;
  const cam = await api("POST", "/api/v1/video/cameras", { name: `Stall ${suf}`, url: `${sim}/stall/h264/320x180`, sub_url: `${sim}/stall/h264/160x96`, retention_days: 1, record: "off", reason: "probe" });
  check("camera created", cam.status, 201);
  const CID = cam.body && cam.body.id;
  cleanup = async () => { if (CID) await api("DELETE", `/api/v1/video/cameras/${CID}?reason=probe`); cleanup = async () => {}; };

  // a page with the player script; the probe's own player in an empty box, every WebSocket counted
  await send("Page.navigate", { url: base + "/video" }); await sleep(2500);
  await ev(`(() => {
    window.__ws = 0; const W = window.WebSocket;
    // while __freeze is set the probe's sockets drop the pictures (binary messages): connected, but nothing moves
    window.__freeze = false;
    window.WebSocket = function (u, p) {
      const s = p ? new W(u, p) : new W(u);
      if (!String(u).includes(${JSON.stringify(CID)})) return s;
      window.__ws++;
      let h = null;
      Object.defineProperty(s, 'onmessage', { get: () => h, set: (f) => { h = f; s.addEventListener('message', (e) => { if (h === f && !(window.__freeze && typeof e.data !== 'string')) f.call(s, e); }); } });
      return s;
    };
    window.WebSocket.prototype = W.prototype; Object.assign(window.WebSocket, { OPEN: W.OPEN, CLOSED: W.CLOSED, CONNECTING: W.CONNECTING, CLOSING: W.CLOSING });
    const box = document.createElement('div'); box.style.cssText = 'position:fixed;left:0;top:0;width:320px;height:180px;z-index:9999'; document.body.append(box);
    window.__stalls = 0;
    window.__p = ctrl32Player(box, {url: '/api/v1/video/cameras/${CID}/live?quality=sub', live: true, onStall: () => window.__stalls++});
    return true; })()`);
  check("the live picture plays", await waitFor(`__p.health === 'playing'`, 30000), true);
  check("one connection", await ev(`__ws`), 1);
  // a moving picture is left alone
  await sleep(25000);
  check("a playing picture is not reopened", await ev(`[__stalls, __ws, __p.health]`), [0, 1, "playing"]);
  // freeze the picture: the stream stays connected, no picture arrives
  await ev(`__freeze = true`);
  check("the frozen picture is reported as stalled", await waitFor(`__p.health === 'stalled'`, 15000), true);
  check("the stalled stream is opened again within 30 s", await waitFor(`__stalls >= 1 && __ws >= 2`, 30000), true);
  // pictures arrive again: the reopened stream plays
  await ev(`__freeze = false`);
  // (a stream opened while the probe dropped pictures lost its start too and is opened once more)
  check("after reopening the picture plays again", await waitFor(`__p.health === 'playing' && __p.video.currentTime > 2`, 60000), true);
  const after = await ev(`__stalls`);
  if (process.env.TRACE) for (let i = 0; i < 25; i++) { await sleep(1000); console.log("    t+" + i, await ev(`[__stalls, __ws, __p.health, __p.el.dataset.state, __p.video.currentTime.toFixed(2), __p.video.paused, __p.video.buffered.length ? __p.video.buffered.end(0).toFixed(2) : -1]`)); }
  else await sleep(25000);
  check("no further reopening while it plays", await ev(`[__stalls - ${after}, __p.health]`), [0, "playing"]);
  await ev(`__p.close()`);

  await cleanup();
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  console.log(`passed ${pass}, failed ${fail}`);
  stop(); process.exit(fail || problems.length ? 1 : 0);
})().catch(async (e) => { console.error(e); console.log("problems:", JSON.stringify(problems)); try { await cleanup(); } catch (x) { /* the browser is gone */ } stop(); process.exit(1); });
