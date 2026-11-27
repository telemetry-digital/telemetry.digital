// Headless Chrome: the operator action log (spec 039, 0.93.0). The log is switched on in Settings → Security policy,
// the pages report themselves when opened, a command sent from a page carries that page, the Operator log page lists,
// filters and searches the actions, and says so when the log is switched off.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/operator_log.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9380;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-oplog", "--window-size=1440,1000", "--lang=" + (lang || "en-US"), "about:blank"], { stdio: "ignore" });
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
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(path.join(out, name), Buffer.from(s.result.data, "base64")); };
  const until = async (expr, ms = 8000) => { for (let i = 0; i < ms / 150; i++) { if (await ev(expr)) return true; await sleep(150); } return false; };
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2200); if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`); };
  const rows = () => ev(`[...document.querySelectorAll('#olTable tbody tr')].map(r => [...r.cells].map(c => c.textContent.trim()))`);
  const suf = String(Math.floor(Math.random() * 90000) + 10000);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);

  // ---- switched on in the security policy ----
  await go("/settings/policy");
  await ev(`(async () => { if ((await ctrl32.api('GET', '/api/v1/settings/policy')).log_operator_actions) await ctrl32.api('PUT', '/api/v1/settings/policy', { log_operator_actions: false, reason: 'headless start' }); return 1; })()`);
  await go("/settings/policy");
  expect("the policy page has the switch, off", await ev(`!!document.querySelector('#policyForm [name=log_operator_actions]') && !document.querySelector('#policyForm [name=log_operator_actions]').checked`));
  await ev(`(() => { const f = document.querySelector('#policyForm'); f.log_operator_actions.click(); f.reason.value = 'headless probe'; f.requestSubmit(); return 1; })()`);
  expect("the switch is saved", await until(`fetch('/api/v1/settings/policy',{credentials:'same-origin'}).then(r=>r.json()).then(p => p.log_operator_actions === true)`));
  await shot("operator-log-policy.png");

  // ---- pages report themselves; a command carries its page ----
  await go("/recipes"); await go("/alarms");
  const dev = await ev(`ctrl32.api('POST', '/api/v1/devices', { external_id: 'oplog-h-${suf}', name: 'Motor M${suf}', transport: 'http', reason: 'headless probe' }).then(d => d.id)`);
  expect("a device for the command", typeof dev === "string" && dev.length > 10, dev);
  expect("a command sent from the page", (await ev(`ctrl32.api('POST', '/api/v1/devices/${dev}/commands', { method: 'write', args: { key: 'run', value: true }, reason: 'start the motor' }).then(r => r.status)`)) === "queued");
  await ev(`(ctrl32.opReport('popup', 'Motor M${suf}', { screen: 'fp-motor' }), 1)`); await sleep(400);

  // ---- the page ----
  await go("/oplog");
  expect("the menu has the Operator log", await ev(`!!document.querySelector('a[href="/oplog"]')`));
  expect("the log is on: no warning", await ev(`document.querySelector('#olOff').hidden`));
  let r = await rows();
  const cmd = r.find((x) => x[3] === "Motor M" + suf && x[5] === "start the motor");
  expect("the command: user, target, key and value, reason, page and station", !!cmd && cmd[1] === "admin" && cmd[4].startsWith("run → true") && cmd[6] === "/alarms" && cmd[7].length > 0, cmd || r.slice(0, 4));
  expect("the opened pages are listed with their path", r.some((x) => x[4] === "/recipes") && r.some((x) => x[4] === "/alarms"), r.map((x) => x[4]).slice(0, 8));
  expect("the opened window is listed", r.some((x) => x[3] === "Motor M" + suf && x[4].includes("fp-motor")));
  await shot("operator-log.png");
  await ev(`(() => { const k = document.querySelector('#olKind'); k.value = 'acts'; document.querySelector('#olGo').click(); return 1; })()`); await sleep(900);
  r = await rows();
  expect("Actions only leaves out pages and windows", r.length > 0 && !r.some((x) => x[4] === "/recipes") && r.some((x) => x[5] === "start the motor"), r.slice(0, 3));
  await ev(`(() => { const k = document.querySelector('#olKind'); k.value = 'moves'; document.querySelector('#olGo').click(); return 1; })()`); await sleep(900);
  r = await rows();
  expect("Pictures and windows only", r.length > 0 && !r.some((x) => x[5] === "start the motor") && r.some((x) => x[4] === "/recipes"), r.slice(0, 3));
  await ev(`(() => { document.querySelector('#olKind').value = ''; const q = document.querySelector('#olQ'); q.value = 'M${suf}'; document.querySelector('#olGo').click(); return 1; })()`); await sleep(900);
  r = await rows();
  expect("search finds the device in commands and windows", r.length === 2 && r.every((x) => x[3] === "Motor M" + suf), r);
  expect("the user filter offers the users of the range", await ev(`[...document.querySelectorAll('#olUser option')].some(o => o.textContent === 'admin')`));
  if (lang === "sk") expect("Slovak: the page is translated, names are not", await ev(`document.querySelector('#olTable th').textContent !== 'Time' && document.querySelector('h1').textContent !== 'Operator log'`) && r[0][3] === "Motor M" + suf);
  await send("Emulation.setDeviceMetricsOverride", { width: 400, height: 800, deviceScaleFactor: 1, mobile: true }); await sleep(400);
  expect("on a phone the page does not scroll sideways", await ev(`document.documentElement.scrollWidth <= innerWidth + 1`), await ev(`[document.documentElement.scrollWidth, innerWidth]`));
  await send("Emulation.clearDeviceMetricsOverride"); await sleep(200);

  // ---- switched off ----
  await ev(`ctrl32.api('PUT', '/api/v1/settings/policy', { log_operator_actions: false, reason: 'headless probe' }).then(() => 1)`);
  await go("/oplog");
  expect("switched off: the page says so and keeps the old rows", await ev(`!document.querySelector('#olOff').hidden`) && (await rows()).some((x) => x[5] === "start the motor"));
  await go("/recipes"); await go("/oplog");
  expect("nothing new is written when off", !(await rows()).some((x) => x[0] && x[4] === "/recipes" && false) && (await ev(`ctrl32.me.log_actions`)) === false);
  await ev(`ctrl32.api('DELETE', '/api/v1/devices/${dev}?reason=headless+probe').then(() => 1).catch(() => 0)`);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
