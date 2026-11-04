// Headless Chrome, 0.86.9. A display: a click on a wall tile enlarges it with the main stream, another click, Esc or the
// button returns, and it offers no recordings. The camera page: the From and To fields select an exact range across an
// hour, the buttons follow them, dragging fills them and a download gets exactly the range.
//   BASE=http://127.0.0.1:8080 CAMERA="Car park north" [OUT=dir for a picture] node tests/headless/range_and_display_enlarge.js
// CAMERA must record continuously; the default is the first such camera.
const { spawn } = require("child_process");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const port = 9398;
const proc = spawn(chrome, ["--headless=new", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/c32-range-" + Date.now(), "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const json = (url) => new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej));
let pass = 0, fail = 0, cleanup = async () => {}; const problems = [];
const check = (name, got, want) => { if (JSON.stringify(got) === JSON.stringify(want)) { pass++; console.log("  ok   " + name); } else { fail++; console.log(`  FAIL ${name} (got: ${JSON.stringify(got)}, want: ${JSON.stringify(want)})`); } };
(async () => {
  let targets;
  for (let i = 0; i < 120; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {};
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || ""));
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result && r.result.result ? r.result.result.value : undefined; };
  const go = async (path) => { await send("Page.navigate", { url: base + path }); await sleep(3000); };
  const api = (m, p, b) => ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(me=>fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token},body:${b ? JSON.stringify(JSON.stringify(b)) : "undefined"}})).then(async r=>({status:r.status,body:await r.json().catch(()=>null)}))`);
  const key = async (k) => { await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code: k, windowsVirtualKeyCode: 27 }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code: k, windowsVirtualKeyCode: 27 }); };

  await go("/login");
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  const list = (await api("GET", "/api/v1/video/cameras")).body;
  const cam = list.find((c) => (process.env.CAMERA ? c.name === process.env.CAMERA : c.record === "continuous" && c.enabled));
  if (!cam) throw new Error("no continuously recording camera: bash deploy/dev/dev.sh camsim && demo");

  console.log("== the camera page: an exact range");
  await go(`/video/cameras/${cam.id}`);
  const st = () => ev(`JSON.stringify({f:document.querySelector('#selFrom').value,t:document.querySelector('#selTo').value,x:document.querySelector('#export').disabled,e:document.querySelector('#evidence').disabled,h:document.querySelector('#hold').disabled,i:document.querySelector('#selInfo').textContent})`).then(JSON.parse);
  const set = (f, t) => ev(`(()=>{const a=document.querySelector('#selFrom'),b=document.querySelector('#selTo');a.value=${JSON.stringify(f)};b.value=${JSON.stringify(t)};a.dispatchEvent(new Event('change'));})()`);
  let s = await st();
  check("nothing selected: buttons off, fields empty, hint", [s.x, s.e, s.h, s.f, s.t, s.i.length > 10], [true, true, true, "", "", true]);
  await set("2026-11-03T18:35:00", "2026-11-03T19:25:00"); await sleep(500);
  s = await st();
  check("50 minutes across an hour: buttons on", [s.x, s.e, s.h], [false, false, false]);
  check("the fields keep the exact times", [s.f, s.t], ["2026-11-03T18:35", "2026-11-03T19:25"]); // a browser leaves out :00
  check("the selection itself is exact, and the day and hour follow", await ev(`JSON.stringify([new Date(SEL[0]).toTimeString().slice(0,8), new Date(SEL[1]).toTimeString().slice(0,8), new Date(HOUR).getHours(), document.querySelector('#day').value])`), JSON.stringify(["18:35:00", "19:25:00", 18, "2026-11-03"]));
  await set("2026-11-03T18:00:00", "2026-11-03T19:00:01"); await sleep(300);
  s = await st();
  check("more than an hour: downloads off, holding allowed", [s.x, s.e, s.h, s.i.includes("(")], [true, true, false, true]);
  await set("2026-11-03T23:50:00", "2026-11-04T00:10:00"); await sleep(500);
  s = await st();
  check("across midnight", [s.x, s.f, s.t], [false, "2026-11-03T23:50", "2026-11-04T00:10"]);
  await set("2026-11-03T19:00:00", "2026-11-03T18:00:00"); await sleep(300);
  s = await st();
  check("the end before the start: nothing selected, the fields stay", [s.x, s.h, s.f, s.t], [true, true, "2026-11-03T19:00", "2026-11-03T18:00"]);
  await set("2026-11-03T18:35:00", ""); await sleep(300);
  s = await st();
  check("an empty end: one hour after the start", [s.t, s.x], ["2026-11-03T19:35", false]);

  // a real recording: the two ends of the last minutes of today
  await go(`/video/cameras/${cam.id}`);
  const local = (ms) => ev(`localField(${ms})`);
  const now = Math.floor(Date.now() / 1000) * 1000, a = now - 40000, b = now - 20000;
  await set(await local(a), await local(b)); await sleep(500);
  const dl = await ev(`(async()=>{const r=await fetch('/api/v1/video/cameras/${cam.id}/export?from='+new Date(Math.min(...SEL)).toISOString()+'&to='+new Date(Math.max(...SEL)).toISOString());const b=await r.arrayBuffer();return JSON.stringify({s:r.status,n:b.byteLength,t:r.headers.get('content-type')})})()`).then(JSON.parse);
  check("the download of the typed range", [dl.s, dl.t, dl.n > 1000], [200, "video/mp4", true]);

  // dragging fills the fields
  const r = JSON.parse(await ev(`(()=>{const e=document.querySelector('#tl2');e.scrollIntoView({block:'center'});const b=e.getBoundingClientRect();return JSON.stringify({x:b.x,y:b.y,w:b.width,h:b.height})})()`));
  await sleep(300);
  const y = r.y + r.h / 2;
  await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: r.x + r.w * 0.3, y });
  await send("Input.dispatchMouseEvent", { type: "mousePressed", x: r.x + r.w * 0.3, y, button: "left", clickCount: 1 });
  await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: r.x + r.w * 0.4, y, button: "left", buttons: 1 });
  await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: r.x + r.w * 0.4, y, button: "left", clickCount: 1 });
  await sleep(300);
  s = await st();
  check("dragging fills the fields, about 6 minutes", [Math.round((new Date(s.t) - new Date(s.f)) / 60000), s.x], [6, false]);

  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 1, mobile: true });
  await sleep(500);
  check("nothing overflows at phone width", await ev(`document.documentElement.scrollWidth <= innerWidth`), true);
  await send("Emulation.clearDeviceMetricsOverride");

  console.log("== a display: enlarging a tile");
  const suf = Date.now() % 100000;
  const wall = await api("POST", "/api/v1/video/walls", { name: `Enlarge ${suf}`, reason: "probe", screens: [{ cols: 2, rows: 2, tiles: [cam.id], quality: "sub" }] });
  check("a wall", wall.status, 201);
  let DID = null;
  cleanup = async () => { if (DID) await api("DELETE", `/api/v1/displays/${DID}?reason=probe`); await api("DELETE", `/api/v1/video/walls/${wall.body.id}?reason=probe`); cleanup = async () => {}; };
  await go("/display");
  const code = await ev(`new Promise(r => { const t = setInterval(() => { const c = document.querySelector('#code').textContent.trim(); if (/^[A-Z0-9]/.test(c)) { clearInterval(t); r(c); } }, 200); })`);
  const pr = await api("POST", "/api/v1/displays/pair", { code, name: `Enlarge ${suf}`, content: { items: [{ kind: "wall", wall: wall.body.id, screen: 0, seconds: 60 }] }, reason: "probe" });
  check("a paired display", pr.status, 201);
  DID = pr.body && pr.body.id;
  await sleep(8000);
  const cell = `document.querySelector('.wall-grid .cell[data-id]')`;
  const state = () => ev(`JSON.stringify({max:${cell}.classList.contains('max'),players:${cell}.querySelectorAll('.vp, video, canvas').length>0,bar:!!${cell}.querySelector('.wall-max-bar'),rec:/Recordings|Záznamy/.test((${cell}.querySelector('.wall-max-bar')||{}).textContent||''),full:(()=>{const b=${cell}.getBoundingClientRect();return b.width>=innerWidth-1&&b.height>=innerHeight-1})()})`).then(JSON.parse);
  const tile = JSON.parse(await ev(`(()=>{const b=${cell}.getBoundingClientRect();return JSON.stringify({x:b.x+b.width/2,y:b.y+b.height/2})})()`));
  const click = async (x, y) => { await send("Input.dispatchMouseEvent", { type: "mouseMoved", x, y }); await send("Input.dispatchMouseEvent", { type: "mousePressed", x, y, button: "left", clickCount: 1 }); await send("Input.dispatchMouseEvent", { type: "mouseReleased", x, y, button: "left", clickCount: 1 }); await sleep(700); };
  let d = await state();
  check("the tile starts small", [d.max, d.full], [false, false]);
  await click(tile.x, tile.y); d = await state();
  check("a click enlarges it to the whole screen, with a way back", [d.max, d.full, d.bar], [true, true, true]);
  if (process.env.OUT) { await sleep(3000); const p = await send("Page.captureScreenshot", { format: "png" }); require("fs").writeFileSync(process.env.OUT + "/display-enlarged.png", Buffer.from(p.result.data, "base64")); }
  check("a display offers no recordings", d.rec, false);
  await click(700, 400); d = await state();
  check("another click returns", [d.max, d.bar], [false, false]);
  await click(tile.x, tile.y); await key("Escape"); await sleep(400); d = await state();
  check("Esc returns", d.max, false);
  await click(tile.x, tile.y);
  await ev(`${cell}.querySelector('.wall-max-bar button').click()`); await sleep(400); d = await state();
  check("the button returns", d.max, false);
  await click(tile.x, tile.y);
  await ev(`clearTimeout(bigT); enlarge(big.el)`); await sleep(300);
  check("the time limit's call returns too", (await state()).max, false);

  check("no script errors", problems, []);
  await cleanup();
  console.log(`passed ${pass}, failed ${fail}`);
  proc.kill(); process.exit(fail ? 1 : 0);
})().catch(async (e) => { console.log("error", e.message); await cleanup().catch(() => {}); proc.kill(); process.exit(1); });
