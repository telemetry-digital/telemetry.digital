// Headless Chrome: the picture editor in the whole window (0.90.1) at the size of a 1366×768 notebook — the editor
// covers the application menu, the library and the properties panel can be hidden (buttons, Tab) and the choice is
// remembered, the picture is re-fitted, Ctrl+wheel zooms at the pointer, Space+drag and the middle button pan.
// Uses a picture of the operator demo (tests/headless/operator_demo.js) or SLUG; writes screenshots to OUT.
//   BASE=http://127.0.0.1:8080 DEMO=.dev/operator-demo.json OUT=.dev/shots node tests/headless/scada_editor_layout.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const slug = process.env.SLUG || JSON.parse(fs.readFileSync(process.env.DEMO || ".dev/operator-demo.json", "utf8")).slugs.plant;
const port = 9377;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-edlayout", "--window-size=1366,768", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const json = (url) => new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej));
let failed = 0;
const expect = (label, cond, detail) => { console.log((cond ? "  ok   " : "  FAIL ") + label + (cond ? "" : " — " + JSON.stringify(detail))); if (!cond) failed++; };
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push(msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n")[0]);
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const size = (w, h) => send("Emulation.setDeviceMetricsOverride", { width: w, height: h, deviceScaleFactor: 1, mobile: false });
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name + ".png", Buffer.from(s.result.data, "base64")); };
  const mouse = (type, x, y, extra) => send("Input.dispatchMouseEvent", Object.assign({ type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1, pointerType: "mouse" }, extra || {}));
  const key = (type, k, code, vk) => send("Input.dispatchKeyEvent", { type, key: k, code, windowsVirtualKeyCode: vk, nativeVirtualKeyCode: vk });
  const press = async (k, code, vk) => { await key("rawKeyDown", k, code, vk); await key("keyUp", k, code, vk); await sleep(350); };
  const M = () => ev(`(() => { const v = document.querySelector('.se-view'), r = v.getBoundingClientRect(), m = v.querySelector('.mimic').getBoundingClientRect(), d = __scadaEditor._debug; return { vw: Math.round(r.width), vh: Math.round(r.height), vl: r.left, vt: r.top, ml: m.left, mt: m.top, mw: m.width, mh: m.height, z: d.Z, lib: d.panels.lib, side: d.panels.side, sl: v.scrollLeft, st: v.scrollTop, pageScroll: document.documentElement.scrollHeight > innerHeight + 1 || document.documentElement.scrollWidth > innerWidth + 1 }; })()`);
  const open = async () => { await send("Page.navigate", { url: base + "/dashboards/" + slug + "/edit" }); await sleep(4500); };

  await size(1366, 650); // a 1366×768 notebook minus the browser's own bars
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(localStorage.removeItem('ctrl32.scada.panels'), ${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  await open();
  let m = await M();
  expect("the editor is mounted with both panels", m.lib === true && m.side === true, m);
  expect("the editor covers the application menu", await ev(`!!document.elementFromPoint(12, 320).closest('#edit') && !!document.elementFromPoint(680, 12).closest('#edit')`));
  expect("the page itself does not scroll", m.pageScroll === false, m);
  expect("with both panels the canvas is at least 740 px wide and 480 px high", m.vw >= 740 && m.vh >= 480, m);
  expect("the picture is fitted into the canvas", m.mw <= m.vw && m.mh <= m.vh && m.mw > m.vw - 60, m);
  expect("the toolbar is one row", await ev(`document.querySelector('.se-top').getBoundingClientRect().height < 50`));
  expect("the drawing tools stay outside the library", await ev(`!document.querySelector('.se-lib .se-tools') && document.querySelectorAll('.se-main > .se-tools .se-tool').length === 8`));
  await ev(`(__scadaEditor.select(['e:p1']), 1)`); await sleep(300); await shot("scada-editor-notebook");
  const both = m;

  // ---- hide the library, then the properties: the picture grows -------------------------------------------------
  await ev(`(document.querySelector('.se-top [data-cmd=plib]').click(), 1)`); await sleep(500); m = await M();
  expect("the library button hides the library", m.lib === false && await ev(`getComputedStyle(document.querySelector('.se-lib')).display === 'none'`), m);
  expect("the canvas takes the freed width and the picture is re-fitted", m.vw > both.vw + 150 && m.z > both.z, { before: both.vw, after: m.vw, z: [both.z, m.z] });
  expect("the drawing tools are still there", await ev(`document.querySelector('.se-main > .se-tools').getBoundingClientRect().width > 30`));
  await ev(`(document.querySelector('.se-top [data-cmd=pside]').click(), 1)`); await sleep(500); m = await M();
  expect("the properties button hides the properties", m.side === false && m.vw > 1250, m);
  await shot("scada-editor-notebook-wide");
  await open(); m = await M();
  expect("the hidden panels are remembered after a reload", m.lib === false && m.side === false && m.vw > 1250, m);
  await ev(`(document.querySelector('#scadaEd').focus(), 1)`);
  await press("Tab", "Tab", 9); m = await M();
  expect("Tab shows both panels", m.lib === true && m.side === true, m);
  await press("Tab", "Tab", 9); m = await M();
  expect("Tab hides both panels", m.lib === false && m.side === false, m);
  await press("Tab", "Tab", 9);

  // ---- zoom at the pointer, pan ---------------------------------------------------------------------------------
  // (a picture smaller than the canvas is centred and cannot scroll, so the check starts once it is larger)
  for (let i = 0; i < 4; i++) await press("+", "Equal", 187);
  m = await M();
  expect("+ zooms in until the picture is larger than the canvas", m.mw > m.vw && m.mh > m.vh, m);
  const px = m.vl + m.vw * 0.7, py = m.vt + m.vh * 0.3, fx = (px - m.ml) / m.mw, fy = (py - m.mt) / m.mh; // a point of the picture under the pointer
  await mouse("mouseMoved", px, py, { buttons: 0 });
  for (let i = 0; i < 4; i++) { await send("Input.dispatchMouseEvent", { type: "mouseWheel", x: px, y: py, deltaX: 0, deltaY: -100, modifiers: 2 }); await sleep(60); }
  await sleep(300); let z = await M();
  expect("Ctrl+wheel zooms in", z.z > m.z * 1.4, { before: m.z, after: z.z });
  expect("the point under the pointer stays in place", Math.abs(z.ml + z.mw * fx - px) < 3 && Math.abs(z.mt + z.mh * fy - py) < 3, { want: [px, py], got: [z.ml + z.mw * fx, z.mt + z.mh * fy] });
  const cx = z.vl + z.vw / 2, cy = z.vt + z.vh / 2;
  await key("rawKeyDown", " ", "Space", 32); await sleep(100);
  expect("Space shows the hand", await ev(`document.querySelector('.se-view').classList.contains('pan')`));
  await mouse("mouseMoved", cx, cy, { buttons: 0 }); await mouse("mousePressed", cx, cy);
  for (let k = 1; k <= 6; k++) await mouse("mouseMoved", cx - 20 * k, cy - 10 * k);
  await mouse("mouseReleased", cx - 120, cy - 60); await key("keyUp", " ", "Space", 32); await sleep(200);
  let p = await M();
  expect("Space+drag pans the picture", Math.abs(p.sl - z.sl - 120) < 3 && Math.abs(p.st - z.st - 60) < 3, { before: [z.sl, z.st], after: [p.sl, p.st] });
  expect("panning selects and moves nothing", await ev(`__scadaEditor.selection().length === 0 || JSON.stringify(__scadaEditor.selection()) === '["e:p1"]'`) && await ev(`!__scadaEditor._debug.drag`));
  const mid = { button: "middle", buttons: 4 };
  await mouse("mouseMoved", cx, cy, { buttons: 0 }); await mouse("mousePressed", cx, cy, mid);
  for (let k = 1; k <= 5; k++) await mouse("mouseMoved", cx + 16 * k, cy + 8 * k, mid);
  await mouse("mouseReleased", cx + 80, cy + 40, { button: "middle", buttons: 0 }); await sleep(200);
  const q = await M();
  expect("the middle button pans the picture", Math.abs(q.sl - p.sl + 80) < 3 && Math.abs(q.st - p.st + 40) < 3, { before: [p.sl, p.st], after: [q.sl, q.st] });
  await ev(`(document.querySelector('#scadaEd').focus(), 1)`); await press("0", "Digit0", 48); m = await M();
  expect("0 fits the picture again", m.mw <= m.vw && m.mh <= m.vh && m.sl === 0 && m.st === 0, m);
  await ev(`(localStorage.removeItem('ctrl32.scada.panels'), 1)`);

  // ---- other sizes ----------------------------------------------------------------------------------------------
  await size(1920, 950); await open(); m = await M();
  expect("1920 px: both panels, canvas over 1200 px", m.lib && m.side && m.vw > 1200 && !m.pageScroll, m);
  await size(820, 1000); await open(); m = await M();
  expect("820 px: panels start hidden, the canvas takes the width", !m.lib && !m.side && m.vw > 700 && !m.pageScroll, m);
  await ev(`(document.querySelector('.se-top [data-cmd=plib]').click(), 1)`); await sleep(400);
  const n = await M();
  expect("820 px: the library opens over the canvas without shrinking it", n.lib === true && n.vw === m.vw && await ev(`getComputedStyle(document.querySelector('.se-lib')).position === 'absolute'`), n);
  await ev(`(localStorage.removeItem('ctrl32.scada.panels'), 1)`);
  expect("no script errors", problems.length === 0, problems);
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})();
