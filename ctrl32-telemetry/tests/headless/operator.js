// Headless Chrome: the operator view (spec 036) on the demo plant of tests/headless/operator_demo.js: the frame, the
// button bar with group alarms, changing the picture without a reload (buttons, a screen action, Back, Up, Home),
// faceplate windows for two pumps, acknowledging from the alarm line, the editor settings; screenshots to OUT.
//   BASE=http://127.0.0.1:8080 DEMO=.dev/operator-demo.json OUT=shots node tests/headless/operator.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const demo = JSON.parse(fs.readFileSync(process.env.DEMO || ".dev/operator-demo.json", "utf8"));
const S = demo.slugs;
const port = 9362;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile-operator", "--window-size=1600,900", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
let pass = 0, fail = 0;
const check = (name, got, want) => { if (String(got) === String(want)) { pass++; console.log("  ok   " + name); } else { fail++; console.log(`  FAIL ${name} (got: ${got}, want: ${want})`); } };
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
  await send("Emulation.setDeviceMetricsOverride", { width: 1600, height: 900, deviceScaleFactor: 1, mobile: false });
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? "EXC " + r.result.exceptionDetails.text : r.result.result.value; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(`${out}/${name}.png`, Buffer.from(s.result.data, "base64")); };
  const go = async (path, ms) => { await send("Page.navigate", { url: base + path }); await sleep(ms || 3000); };
  const click = (sel) => ev(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); if (!e) return 'missing'; e.dispatchEvent(new MouseEvent('click', {bubbles: true})); return 'ok' })()`);
  const btn = (slug) => `#opNav [data-go="${slug}"]`;
  const cls = (slug) => ev(`(document.querySelector('${btn(slug)}') || {className: 'missing'}).className.replace('notranslate', '').replace('op-btn', '').trim().replace(/\\s+/g, ' ')`);
  await go("/login", 1500);
  check("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await ev(lang ? `localStorage.setItem('ctrl32.locale', ${JSON.stringify(lang)})` : `localStorage.removeItem('ctrl32.locale')`); // the browser profile is kept between runs

  console.log("== menu");
  await go("/dashboards", 2500);
  check("menu entry", await ev(`!![...document.querySelectorAll('a.nav-link')].find(a => a.getAttribute('href') === '/operator')`), true);

  console.log("== the frame");
  await go("/operator/" + S.plant, 4000);
  await ev(`window.__mark = 'same page'`);
  check("no application menu, no alarm bar of the application", await ev(`!document.querySelector('#sidebar') && !document.querySelector('#alarmBar') && !document.querySelector('header.topbar')`), true);
  check("title and path", await ev(`document.querySelector('#opTitle').textContent + ' | ' + document.querySelector('#opPath').textContent`), "Waterworks | Waterworks");
  check("clock runs", await ev(`/\\d/.test(document.querySelector('#opTime').textContent)`), true);
  check("the picture fits the work area", await ev(`(() => { const w = document.querySelector('#opWork').getBoundingClientRect(), m = document.querySelector('#opWork .mimic').getBoundingClientRect(); return m.width > 600 && m.bottom <= w.bottom + 1 && m.right <= w.right + 1 && m.top >= w.top - 1 })()`), true);
  check("live values in the picture", await ev(`/13[.,]8/.test(document.querySelector('#opWork .sc-root').textContent)`), true);
  check("alarm line shows the unacknowledged alarm", await ev(`(() => { const a = document.querySelector('#opAlarm'); return a.classList.contains('unack') && /Pump P1/.test(a.textContent) && !!a.querySelector('#opAckBtn') })()`), true);

  console.log("== button bar with group alarms");
  check("children of the picture, in order", await ev(`[...document.querySelectorAll('#opNav [data-go]')].map(b => b.querySelector('span').textContent).join(',')`), "Pumps,Aeration,Reservoir");
  check("Pumps: an alarm waits below it (blinking)", await cls(S.pumps), "unack");
  check("its count", await ev(`document.querySelector('${btn(S.pumps)} .op-n').textContent`), 1);
  check("Aeration: acknowledged and active (steady)", await cls(S.aeration), "active");
  check("Reservoir: quiet", await cls(S.reservoir), "");
  check("class colours", await ev(`document.querySelector('${btn(S.pumps)}').style.getPropertyValue('--c') + ' ' + document.querySelector('${btn(S.aeration)}').style.getPropertyValue('--c')`), "#d63939 #f59f00");
  check("Home, Back and Up are off on the first picture", await ev(`['opHome', 'opBack', 'opUp'].map(i => document.getElementById(i).disabled).join(',')`), "true,true,true");
  await shot("operator-view");

  console.log("== changing the picture without a reload");
  await click(btn(S.pumps)); await sleep(2500);
  check("address and title follow", await ev(`location.pathname + ' | ' + document.querySelector('#opTitle').textContent + ' | ' + document.querySelector('#opPath').textContent`), `/operator/${S.pumps} | Pumping station | Waterworks › Pumps`);
  check("the page was not reloaded", await ev(`window.__mark`), "same page");
  check("buttons of the pictures below", await ev(`[...document.querySelectorAll('#opNav [data-go]')].map(b => b.querySelector('span').textContent).join(',')`), "Pump P1");
  check("Home, Back and Up are on", await ev(`['opHome', 'opBack', 'opUp'].map(i => document.getElementById(i).disabled).join(',')`), "false,false,false");
  await shot("operator-pumps");
  await click(btn(S.p1)); await sleep(2500);
  check("a picture without children shows its siblings, itself marked", await ev(`[...document.querySelectorAll('#opNav [data-go]')].map(b => b.querySelector('span').textContent + (b.classList.contains('cur') ? '*' : '')).join(',')`), "Pump P1*");
  await click("#opUp"); await sleep(2500);
  check("Up goes to the parent", await ev(`location.pathname`), "/operator/" + S.pumps);
  await click("#opBack"); await sleep(2500);
  check("Back returns to the previous picture", await ev(`location.pathname`), "/operator/" + S.p1);
  await click("#opHome"); await sleep(2500);
  check("Home", await ev(`location.pathname + ' ' + window.__mark`), `/operator/${S.plant} same page`);
  await click('#opWork [data-eid="toair"]'); await sleep(2500);
  check("a screen action of the picture changes it in place", await ev(`location.pathname + ' ' + window.__mark + ' ' + document.querySelector('#opTitle').textContent`), `/operator/${S.aeration} same page Aeration`);
  await click("#opUp"); await sleep(2500);

  console.log("== faceplate windows");
  await click('#opWork [data-eid="p2"]'); await sleep(2500);
  check("a window opens for Pump P2", await ev(`(() => { const w = document.querySelectorAll('.sc-win'); return w.length + ' ' + w[0].querySelector('.sc-pop-title').textContent })()`), "1 Pump — Pump P2");
  check("bound to the tags of P2", await ev(`document.querySelector('.sc-win').dataset.asset`), demo.assets.p2);
  check("with the values of P2 (current 0.2 A)", await ev(`/0[.,]2\\s*A/.test(document.querySelector('.sc-win .sc-root').textContent)`), true);
  await click('#opWork [data-eid="p1"]'); await sleep(2500);
  check("a second window for Pump P1", await ev(`[...document.querySelectorAll('.sc-win .sc-pop-title')].map(e => e.textContent).join(' + ')`), "Pump — Pump P2 + Pump — Pump P1");
  check("with the values of P1 (speed 1465 rpm)", await ev(`/1465\\s*rpm/.test(document.querySelectorAll('.sc-win')[1].querySelector('.sc-root').textContent)`), true);
  check("the windows do not cover each other exactly", await ev(`(() => { const w = [...document.querySelectorAll('.sc-win')].map(x => x.getBoundingClientRect()); return w[0].left !== w[1].left && w[0].top !== w[1].top })()`), true);
  await click('#opWork [data-eid="p1"]'); await sleep(600);
  check("the same faceplate is not opened twice", await ev(`document.querySelectorAll('.sc-win').length`), 2);
  check("a window can be dragged", await ev(`(() => { const w = document.querySelectorAll('.sc-win')[1], h = w.querySelector('.sc-fp-head'), r = w.getBoundingClientRect();
    const p = (t, x, y) => h.dispatchEvent(new PointerEvent(t, {bubbles: true, pointerId: 1, clientX: x, clientY: y}));
    h.setPointerCapture = () => {}; p('pointerdown', r.left + 60, r.top + 14); p('pointermove', r.left + 660, r.top + 214); p('pointerup', r.left + 660, r.top + 214);
    const n = w.getBoundingClientRect(); return Math.round(n.left - r.left) + ',' + Math.round(n.top - r.top) })()`), "600,200");
  await shot("operator-faceplates");
  await click('.sc-win [data-eid="stop"]'); await sleep(800);
  check("Stop asks for a reason before the command", await ev(`(() => { const d = document.querySelector('dialog.sc-dlg'); return !!d && d.open })()`), true);
  await shot("operator-command");
  await ev(`document.querySelector('dialog.sc-dlg').close()`);
  await ev(`document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape'}))`); await sleep(300);
  check("Escape closes the window on top", await ev(`[...document.querySelectorAll('.sc-win .sc-pop-title')].map(e => e.textContent).join(' + ')`), "Pump — Pump P2");
  await click(btn(S.reservoir)); await sleep(2500);
  check("changing the picture closes the windows", await ev(`document.querySelectorAll('.sc-win').length`), 0);
  await click("#opUp"); await sleep(2500);

  console.log("== acknowledging from the alarm line");
  await click("#opAckBtn"); await sleep(500);
  check("dialog names the alarm", await ev(`document.querySelector('#opAck').open && /Pump P1/.test(document.querySelector('#opAckWhat').textContent)`), true);
  await shot("operator-acknowledge");
  await ev(`(() => { const f = document.querySelector('#opAckForm'); f.reason.value = 'Seen, pump is being checked'; f.requestSubmit(); })()`); await sleep(2500);
  check("the alarm line is quiet", await ev(`(() => { const a = document.querySelector('#opAlarm'); return !a.classList.contains('unack') && !a.querySelector('#opAckBtn') })()`), true);
  check("Pumps is steady now: acknowledged, still active", await cls(S.pumps), "active");

  console.log("== editor");
  await go(`/dashboards/${S.pumps}/edit`, 5000);
  check("place in the operator view", await ev(`(() => { const p = document.querySelector('[data-nv="parent"]'), o = document.querySelector('[data-nv="order"]'), l = document.querySelector('[data-nv="label"]'); return !!p && p.value + ' ' + o.value + ' ' + l.value })()`), `${S.plant} 10 Pumps`);
  await shot("operator-editor");
  await go(`/dashboards/${S.pump}/edit`, 5000);
  check("faceplate type names its asset", await ev(`(() => { const a = document.querySelector('[data-fp="asset"]'); return !!a && a.value === '${demo.assets.p1}' && !!document.querySelector('[data-fp="prefix"]') })()`), true);
  await shot("operator-faceplate-editor");

  check("no script errors", problems.length ? JSON.stringify(problems) : "none", "none");
  console.log(`\npassed ${pass}, failed ${fail}`);
  proc.kill(); process.exit(fail ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
