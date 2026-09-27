// Headless Chrome: the Home page (spec 005). The probe creates an MQTT account, runs the zigbee2mqtt simulator
// (tools/mqttsim) against it, opens /home and checks the rooms and cards, switches the plug with a real mouse click,
// dims the light, sets its colour (spec 006), follows the live states reported back by the device, opens the history dialog and filters by room.
const { spawn, execFileSync } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const mqttAddr = process.env.MQTT || "127.0.0.1:1883";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9366;
const sim = tmp + "/mqttsim-home-probe" + (process.platform === "win32" ? ".exe" : "");
execFileSync("go", ["build", "-o", sim, "./tools/mqttsim"], { stdio: "inherit" });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile26", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
let simProc = null, simLog = "";
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
const done = (code) => { if (simProc) simProc.kill(); proc.kill(); process.exit(code); };
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const mouse = async (type, x, y) => send("Input.dispatchMouseEvent", { type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1, pointerType: "mouse" });
  const click = async (sel) => { const c = await ev(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); if (!e) return null; e.scrollIntoView({block: 'center'}); const r = e.getBoundingClientRect(); return {x: r.left + r.width / 2, y: r.top + r.height / 2}; })()`); if (!c) return false; await mouse("mouseMoved", c.x, c.y); await mouse("mousePressed", c.x, c.y); await mouse("mouseReleased", c.x, c.y); return true; };
  const until = async (expr, ms = 8000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await send("Page.navigate", { url: base + "/home" }); await sleep(2000);
  const acct = await ev(`ctrl32.api('POST', '/api/v1/mqtt-sources', {kind: 'account', name: 'Probe home ' + Date.now(), reason: 'probe'})`);
  expect("account created", !!(acct && acct.username), true);
  simProc = spawn(sim, ["z2m", "-addr", mqttAddr, "-user", acct.username, "-pass", acct.password, "-base", "probe" + (Date.now() % 100000), "-wait", "60"]);
  simProc.stdout.on("data", (d) => (simLog += d));
  await sleep(4000);
  await send("Page.navigate", { url: base + "/home" }); await sleep(2500);
  const ents = await ev(`ctrl32.api('GET', '/api/v1/entities?source=${acct.id}')`);
  const byDev = (d) => (ents.find((e) => e.device === d) || {}).id;
  const plug = byDev("Kitchen plug"), light = byDev("Hall light"), temp = (ents.find((e) => e.name === "Temperature") || {}).id;
  expect("entities of the account", ents.length, 5);
  expect("rooms on the page", await ev(`[...document.querySelectorAll('.hm-room h2')].map(h => h.textContent).filter(t => ['Hall', 'Kitchen', 'Living room'].includes(t)).length`), 3);
  expect("temperature card shows a value with unit", await ev(`document.querySelector('.hm-card[data-id="${temp}"] .hm-state').textContent`), (t) => typeof t === "string" && /°C$/.test(t));
  expect("sparkline drawn", await until(`!!document.querySelector('.hm-card[data-id="${temp}"] .hm-spark path')`), true);
  // the plug with a real click on its switch
  expect("plug off", await ev(`document.querySelector('.hm-card[data-id="${plug}"]').classList.contains('on')`), false);
  await click(`.hm-card[data-id="${plug}"] .hm-switch span`);
  expect("plug on after the device reports", await until(`document.querySelector('.hm-card[data-id="${plug}"]').classList.contains('on')`), true);
  expect("device received ON", /Kitchen plug\/set ON/.test(simLog), true);
  const plugDev = (ents.find((e) => e.id === plug) || {}).device_id;
  expect("command acknowledged", await until(`ctrl32.api('GET', '/api/v1/devices/${plugDev}/commands').then(l => l[0] && l[0].status === 'acked')`), true);
  // dim the light
  await ev(`(() => { const s = document.querySelector('.hm-card[data-id="${light}"] [data-bright]'); s.value = 40; s.dispatchEvent(new Event('change', {bubbles: true})); })()`);
  expect("light on at 40 %", await until(`/40 %/.test(document.querySelector('.hm-card[data-id="${light}"] .hm-state').textContent)`), true);
  expect("light payload", /Hall light\/set \{"brightness":102,"state":"ON"\}/.test(simLog), true);
  // colour picker and colour temperature (spec 006)
  expect("colour controls on the light card", await ev(`!!document.querySelector('.hm-card[data-id="${light}"] input[type=color][data-rgb]') && !!document.querySelector('.hm-card[data-id="${light}"] [data-ct]')`), true);
  await ev(`(() => { const c = document.querySelector('.hm-card[data-id="${light}"] [data-rgb]'); c.value = '#00ff00'; c.dispatchEvent(new Event('change', {bubbles: true})); })()`);
  for (let i = 0; i < 40 && !/"color":\{"b":0,"g":255,"r":0\}/.test(simLog); i++) await sleep(250);
  expect("colour payload", /"color":\{"b":0,"g":255,"r":0\}/.test(simLog), true);
  expect("colour shown after the device reports it", await until(`document.querySelector('.hm-card[data-id="${light}"] [data-rgb]').value === '#00ff00'`), true);
  // history dialog
  await click(`.hm-card[data-id="${temp}"] [data-hist]`);
  expect("history chart", await until(`document.querySelector('#histDlg').open && !!document.querySelector('#histBody svg path.hm-line')`), true);
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/home-history-probe.png", Buffer.from(s1.result.data, "base64"));
  await ev(`document.querySelector('#histDlg').close()`);
  // room filter
  await click(`#roomTabs button:nth-child(3)`);
  expect("room filter shows one room", await ev(`document.querySelectorAll('.hm-room').length`), 1);
  await click(`#roomTabs button:first-child`);
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/home-probe.png", Buffer.from(s2.result.data, "base64"));
  await ev(`ctrl32.api('DELETE', '/api/v1/mqtt-sources/${acct.id}?reason=probe')`);
  console.log(problems.length ? "problems: " + JSON.stringify(problems) : "no problems");
  done(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); done(1); });
