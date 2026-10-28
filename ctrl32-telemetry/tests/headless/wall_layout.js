// Headless Chrome: video wall tiles with their own place and size (0.85.0). The editor shows the tiles on the grid,
// a tile is resized by dragging its corner and moved by dragging it (never onto another tile), an arrangement preset
// sets the whole screen; the wall page and a paired display place the tiles where the rects say; a click on a tile
// enlarges it (main stream) on every wall, Esc returns, and with playback allowed the enlarged camera offers its
// recordings. Starts the camera simulator itself (built with Go from the repository root) unless SIM names one;
// screenshots in OUT.
//   BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/wall_layout.js
const { spawn, execFileSync } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
let sim = process.env.SIM, simProc = null;
if (!sim) {
  const bin = (process.env.TEMP || "/tmp") + "/camsim-wall" + (process.platform === "win32" ? ".exe" : "");
  execFileSync("go", ["build", "-o", bin, "./tools/camsim"], { stdio: "inherit" });
  simProc = spawn(bin, ["-rtsp", "127.0.0.1:18554", "-http", "127.0.0.1:18555", "-fps", "10"], { stdio: "ignore" });
  sim = "rtsp://127.0.0.1:18554";
}
const stop = () => { proc.kill(); if (simProc) simProc.kill(); };
const out = process.env.OUT || ".";
const port = 9361;
const proc = spawn(chrome, ["--headless=new", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-wall", "--window-size=1600,1000", "--autoplay-policy=no-user-gesture-required", "about:blank"], { stdio: "ignore" });
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
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception && r.result.exceptionDetails.exception.description) } : r.result.result.value; };
  const waitFor = (expr, ms = 15000) => ev(`new Promise(r => { const t0 = Date.now(), t = setInterval(() => { let v = false; try { v = !!(${expr}); } catch (e) { /* not yet */ } if (v || Date.now() - t0 > ${ms}) { clearInterval(t); r(v); } }, 200); })`);
  // navigates and waits until the new page has loaded (an evaluate during the switch sees the old page or nothing)
  const go = async (path) => { await send("Page.navigate", { url: base + path }); for (let i = 0; i < 100; i++) { await sleep(150); const v = await ev(`location.pathname + location.search + '|' + document.readyState`).catch(() => ""); if (v === path + "|complete") return; } };
  // a screenshot of the page, or of one element (sel)
  const shot = async (name, sel) => {
    const clip = sel ? await ev(`(() => { const r = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect(); return {x: r.left, y: r.top, width: r.width, height: Math.min(r.height, innerHeight - r.top), scale: 1}; })()`) : undefined;
    const s = await send("Page.captureScreenshot", clip ? { format: "png", clip } : { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64"));
  };
  const api = (m, p, b) => ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(me=>fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token},body:${b ? JSON.stringify(JSON.stringify(b)) : "undefined"}})).then(async r=>({status:r.status, body: await r.json().catch(()=>null)}))`);
  const mouse = async (type, x, y) => send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1, pointerType: "mouse" });
  const center = (sel) => ev(`(() => { const r = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2, r.width, r.height]; })()`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  const suf = Date.now() % 100000, cams = [];
  // whatever happens below, the probe's cameras, wall and display go away again (the camera limit without a licence)
  let WID = null, DID = null;
  cleanup = async () => {
    if (DID) await api("DELETE", `/api/v1/displays/${DID}?reason=probe`);
    if (WID) await api("DELETE", `/api/v1/video/walls/${WID}?reason=probe`);
    for (const cid of cams) if (cid) await api("DELETE", `/api/v1/video/cameras/${cid}?reason=probe`);
    cleanup = async () => {};
  };
  for (const n of ["Gate", "Yard", "Door", "Hall"]) {
    const r = await api("POST", "/api/v1/video/cameras", { name: `${n} ${suf}`, url: `${sim}/${n.toLowerCase()}/h264/320x180`, sub_url: `${sim}/${n.toLowerCase()}/h264/160x96`, retention_days: 1, record: "off", reason: "probe" });
    cams.push(r.body && r.body.id);
  }
  check("cameras created", cams.every(Boolean), true);
  // a 3×2 tile, a tall 1×2 beside it, two single cells; the rest of the 6×4 grid stays free
  const rects = [{ x: 0, y: 0, w: 3, h: 2 }, { x: 3, y: 0, w: 1, h: 2 }, { x: 4, y: 0, w: 1, h: 1 }, { x: 0, y: 2, w: 1, h: 1 }];
  const w = await api("POST", "/api/v1/video/walls", { name: `Layout ${suf}`, playback_on_click: true, screens: [{ name: "Main", cols: 6, rows: 4, tiles: cams, rects, quality: "sub" }], reason: "probe" });
  check("wall with rects created", w.status, 201);
  WID = w.body.id;

  // ---- the editor
  await go("/video/walls");
  await waitFor(`document.querySelector('[data-edit="${WID}"]')`);
  await ev(`document.querySelector('[data-edit="${WID}"]').click()`); await waitFor(`document.querySelector('#screens [data-ti]')`);
  check("editor: tiles and free cells", await ev(`[document.querySelectorAll('#screens [data-ti]').length, document.querySelectorAll('#screens [data-free]').length]`), [4, 24 - 6 - 2 - 1 - 1]);
  check("editor: sizes shown", await ev(`[...document.querySelectorAll('#screens [data-ti] .wall-size')].map(e => e.textContent)`), ["3 × 2", "1 × 2", "1 × 1", "1 × 1"]);
  check("editor: presets offered on 6×4", await ev(`[...document.querySelectorAll('#screens [data-preset]')].map(b => b.dataset.preset)`), ["equal", "one", "two", "four"]);
  // resize the single tile at (4,0) to 2×1 by its corner: drag the handle one cell to the right
  const cell = await ev(`(() => { const g = document.querySelector('#screens [data-grid]').getBoundingClientRect(); return [g.width / 6, g.height / 4]; })()`);
  const h = await center(`#screens [data-ti="2"] [data-handle]`);
  await mouse("mousePressed", h[0], h[1]); await mouse("mouseMoved", h[0] + cell[0] * 0.6, h[1]); await mouse("mouseMoved", h[0] + cell[0], h[1]); await mouse("mouseReleased", h[0] + cell[0], h[1]); await sleep(300);
  check("resized by the corner to 2×1", await ev(`JSON.stringify(WSCREENS[0].rects[2])`), JSON.stringify({ x: 4, y: 0, w: 2, h: 1 }));
  // move the single tile at (0,2) onto the large tile: refused (stays); then one cell to the right: moved
  let c = await center(`#screens [data-ti="3"]`);
  await mouse("mousePressed", c[0], c[1]); await mouse("mouseMoved", c[0], c[1] - cell[1]); await mouse("mouseReleased", c[0], c[1] - cell[1]); await sleep(300);
  check("never onto another tile", await ev(`(r => [r.x, r.y, r.w, r.h])(WSCREENS[0].rects[3])`), [0, 2, 1, 1]);
  c = await center(`#screens [data-ti="3"]`);
  await mouse("mousePressed", c[0], c[1]); await mouse("mouseMoved", c[0] + cell[0], c[1]); await mouse("mouseReleased", c[0] + cell[0], c[1]); await sleep(300);
  check("moved one cell right", await ev(`JSON.stringify(WSCREENS[0].rects[3])`), JSON.stringify({ x: 1, y: 2, w: 1, h: 1 }));
  check("the moved tile is selected with its fields", await ev(`[document.querySelector('#screens [data-g="x"]').value, document.querySelector('#screens [data-g="w"]').value]`), ["2", "1"]);
  // typing a size that would cover another tile is refused
  await ev(`(() => { const i = document.querySelector('#screens [data-g="h"]'); i.value = '9'; i.dispatchEvent(new Event('change')); })()`); await sleep(200);
  check("a size off the grid is refused", await ev(`WSCREENS[0].rects[3].h`), 1);
  // a free cell adds a tile there
  await ev(`document.querySelector('#screens [data-free="5,3"]').click()`); await sleep(200);
  check("a click on a free cell adds a tile", await ev(`JSON.stringify(WSCREENS[0].rects[WSCREENS[0].rects.length - 1])`), JSON.stringify({ x: 5, y: 3, w: 1, h: 1 }));
  await shot("wall-editor.png", "#wallDlg");
  // the preset: two large stacked on the left, the cameras of the largest tiles get them
  await ev(`document.querySelector('#screens [data-preset="two"]').click()`); await sleep(300);
  check("preset 2 large + small", await ev(`JSON.stringify(WSCREENS[0].rects.filter(r => r.w > 1))`), JSON.stringify([{ x: 0, y: 0, w: 2, h: 2 }, { x: 0, y: 2, w: 2, h: 2 }]));
  check("the large tile keeps the camera of the largest tile", await ev(`WSCREENS[0].tiles[0]`), cams[0]);
  await shot("wall-editor-preset.png", "#wallDlg");
  // the option of the enlarged camera at the bottom of the dialog
  await ev(`document.querySelector('#wallDlg').scrollTop = 1e5`); await sleep(200);
  const pc = await ev(`(() => { const d = document.querySelector('#wallDlg').getBoundingClientRect(), l = document.querySelector('#wallForm [name=playback_on_click]').closest('label').getBoundingClientRect(); return {x: d.left, y: l.top - 16, width: d.width, height: d.bottom - l.top + 16, scale: 1}; })()`);
  const ps = await send("Page.captureScreenshot", { format: "png", clip: pc }); fs.writeFileSync(out + "/wall-editor-playback.png", Buffer.from(ps.result.data, "base64"));
  // back to the custom layout and save it
  await ev(`(() => { WSCREENS[0].rects = ${JSON.stringify(rects)}; WSCREENS[0].tiles = ${JSON.stringify(cams)}; renderScreens(); const f = document.querySelector('#wallForm'); f.reason.value = 'probe'; f.requestSubmit(); })()`); await sleep(1200);
  const saved = await api("GET", "/api/v1/video/walls/" + WID);
  check("saved with rects", saved.body.screens[0].rects.map(r => [r.x, r.y, r.w, r.h]), rects.map(r => [r.x, r.y, r.w, r.h]));
  check("list preview places the tiles", await ev(`[...document.querySelectorAll('.wall-preview')].some(p => p.querySelector('i[style*="span 3"]'))`), true);

  // ---- the wall page
  await go("/video/walls/" + WID);
  await waitFor(`document.querySelector('#grid .cell[data-id]')`); await sleep(500);
  const sizes = await ev(`[...document.querySelectorAll('#grid .cell[data-id]')].map(e => { const r = e.getBoundingClientRect(); return [Math.round(r.width), Math.round(r.height)]; })`);
  check("wall: 4 camera tiles and 14 empty cells", await ev(`[document.querySelectorAll('#grid .cell[data-id]').length, document.querySelectorAll('#grid .cell:not([data-id])').length]`), [4, 14]);
  check("wall: the large tile is 3 cells wide and 2 high", [Math.abs(sizes[0][0] / sizes[2][0] - 3) < 0.1, Math.abs(sizes[0][1] / sizes[2][1] - 2) < 0.1], [true, true]);
  check("wall: the tall tile is 2 cells high", Math.abs(sizes[1][1] / sizes[2][1] - 2) < 0.1, true);
  const playing = (sel, n) => ev(`new Promise(r => { let k = 0; const t = setInterval(() => { const c = [...document.querySelectorAll(${JSON.stringify(sel)})].filter(v => v.readyState >= 2).length; if (c >= ${n} || ++k > 40) { clearInterval(t); r(c); } }, 500); })`);
  check("wall: video plays", await playing('#grid .cell[data-id] video', 4), 4);
  await shot("wall-tiles.png");
  await ev(`document.querySelector('#grid .cell[data-id]:nth-child(3)').click()`); await playing('.cell.max video', 2);
  check("click enlarges with the main stream", await ev(`[!!document.querySelector('#grid .cell.max'), !!document.querySelector('.cell.max .wall-max-bar'), [...document.querySelectorAll('.cell.max video')].some(v => v.videoWidth === 320)]`), [true, true, true]);
  check("playback allowed: Recordings offered", await ev(`!!document.querySelector('.cell.max [data-rec]')`), true);
  await shot("wall-enlarged.png");
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Escape", code: "Escape", windowsVirtualKeyCode: 27 }); await sleep(500);
  check("Esc returns to the wall", await ev(`!!document.querySelector('#grid .cell.max')`), false);
  await ev(`document.querySelector('#grid .cell[data-id]').click()`); await sleep(800);
  await ev(`document.querySelector('.cell.max [data-rec]').click()`); await sleep(1500);
  check("Recordings opens the camera's player over the wall", await ev(`!!document.querySelector('#play iframe')`), true);
  await ev(`document.querySelector('#playClose').click()`); await sleep(800);
  // without playback the enlarged camera has only the way back
  await api("PUT", "/api/v1/video/walls/" + WID, { playback_on_click: false, reason: "probe" });
  await send("Page.reload"); await sleep(4000);
  await ev(`document.querySelector('#grid .cell[data-id]').click()`); await sleep(800);
  check("without playback: enlarges, no Recordings", await ev(`[!!document.querySelector('#grid .cell.max'), !!document.querySelector('.cell.max [data-rec]')]`), [true, false]);

  // ---- a paired display shows the same layout
  await go("/display");
  const code = await ev(`new Promise(r => { const t = setInterval(() => { const c = document.querySelector('#code').textContent.trim(); if (/^[A-Z0-9]{4}-[A-Z0-9]{4}$/.test(c)) { clearInterval(t); r(c); } }, 300); setTimeout(() => r(''), 15000); })`);
  const pr = await api("POST", "/api/v1/displays/pair", { code, name: `Probe ${suf}`, content: { items: [{ kind: "wall", wall: WID, screen: 0, seconds: 60 }], popup: "none" }, reason: "probe" });
  check("display paired", pr.status, 201);
  await sleep(8000);
  const dsz = await ev(`[...document.querySelectorAll('.wall-grid .cell[data-id]')].map(e => { const r = e.getBoundingClientRect(); return [Math.round(r.width), Math.round(r.height)]; })`);
  check("display: 4 camera tiles", dsz.length, 4);
  check("display: the large tile spans 3×2", dsz.length === 4 && Math.abs(dsz[0][0] / dsz[2][0] - 3) < 0.1 && Math.abs(dsz[0][1] / dsz[2][1] - 2) < 0.1, true);
  await shot("wall-display.png");

  DID = pr.body && pr.body.id;
  await cleanup();
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  console.log(`passed ${pass}, failed ${fail}`);
  stop(); process.exit(fail || problems.length ? 1 : 0);
})().catch(async (e) => { console.error(e); console.log("problems:", JSON.stringify(problems)); try { await cleanup(); } catch (x) { /* the browser is gone */ } stop(); process.exit(1); });
