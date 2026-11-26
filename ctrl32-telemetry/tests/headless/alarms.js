// Headless Chrome: the Alarms page (spec 035) on a server with alarms of every ISA state (tests/headless/alarms_demo.sh makes them
// on a dev server): the current list with class colours and states, acknowledging through the dialog, the shelved list,
// history, statistics and classes, the alarm bar on another page; screenshots to OUT.
//   BASE=http://127.0.0.1:8080 OUT=shots node tests/headless/alarms.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9361;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile-alarms", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "headless probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  await send("Emulation.setDeviceMetricsOverride", { width: 1500, height: 1000, deviceScaleFactor: 1, mobile: false });
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? "EXC " + r.result.exceptionDetails.text : r.result.result.value; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(`${out}/${name}.png`, Buffer.from(s.result.data, "base64")); };
  const go = async (path, ms) => { await send("Page.navigate", { url: base + path }); await sleep(ms || 3000); };
  await go("/login", 1500);
  check("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  if (lang) await ev(`localStorage.setItem('ctrl32.locale', ${JSON.stringify(lang)})`);

  console.log("== current");
  await go("/alarms");
  check("menu entry", await ev(`!![...document.querySelectorAll('a.nav-link')].find(a => a.getAttribute('href') === '/alarms')`), true);
  const states = await ev(`[...document.querySelectorAll('#cur tbody .al-state')].map(e => e.className.split(' ')[1]).join(',')`);
  check("rows of the three states that need attention", ["unack_active", "ack_active", "unack_rtn"].every((s) => states.includes(s)), true);
  check("no shelved alarm in the current list", states.includes("shelved"), false);
  check("ordered by priority (class Alarm first)", await ev(`(() => { const p = [...document.querySelectorAll('#cur tbody tr')].map(tr => getComputedStyle(tr).getPropertyValue('--c').trim()); return p[0] })()`), "#d63939");
  check("unacknowledged states blink", await ev(`[...document.querySelectorAll('#cur .al-state.unack_active, #cur .al-state.unack_rtn')].every(e => e.classList.contains('al-blink'))`), true);
  check("acknowledge all enabled", await ev(`!document.querySelector('#ackAll').disabled`), true);
  check("alarm bar shows the top alarm", await ev(`(() => { const b = document.querySelector('#alarmBar'); return !!b && !b.hidden && b.classList.contains('unack') && /Reactor R1/.test(b.textContent) })()`), true);
  await shot("alarms-current");

  console.log("== acknowledge through the dialog");
  const before = await ev(`document.querySelectorAll('#cur [data-ack]').length`), rtn = await ev(`document.querySelectorAll('#cur tr.rtn').length`);
  await ev(`document.querySelector('#cur tr.rtn [data-ack]').click()`); await sleep(500);
  check("dialog open", await ev(`document.querySelector('#ackDlg').open`), true);
  await shot("alarms-acknowledge");
  await ev(`(() => { const f = document.querySelector('#ackForm'); f.reason.value = 'Seen, no action needed'; f.requestSubmit(); })()`); await sleep(2000);
  check("one alarm less waits", await ev(`document.querySelectorAll('#cur [data-ack]').length`), before - 1);
  check("the alarm that had returned to normal left the list", await ev(`document.querySelectorAll('#cur tr.rtn').length`), rtn - 1);

  console.log("== shelve dialog");
  await ev(`document.querySelector('#cur [data-shelve]').click()`); await sleep(500);
  check("shelve dialog open with durations", await ev(`document.querySelector('#shDlg').open + ' ' + document.querySelector('#shForm').minutes.options.length`), "true 8");
  await shot("alarms-shelve");
  await ev(`document.querySelector('#shDlg').close()`);

  console.log("== shelved");
  await go("/alarms/shelved");
  check("the shelved alarm with who and why", await ev(`(() => { const r = document.querySelector('#shv tbody tr'); return !!r && /admin/.test(r.textContent) && /Flow meter being replaced/.test(r.textContent) && !!r.querySelector('[data-unshelve]') })()`), true);
  await shot("alarms-shelved");

  console.log("== history");
  await go("/alarms/history");
  check("history rows", await ev(`document.querySelectorAll('#his tbody tr').length >= 5`), true);
  check("class filter filled", await ev(`document.querySelector('#hClass').options.length >= 4`), true);
  await ev(`(() => { document.querySelector('#hQ').value = 'temperature'; document.querySelector('#hGo').click(); })()`); await sleep(1500);
  check("search narrows to the source", await ev(`[...document.querySelectorAll('#his tbody tr')].every(tr => /temperature/.test(tr.textContent)) && document.querySelectorAll('#his tbody tr').length >= 1`), true);
  await ev(`(() => { document.querySelector('#hQ').value = ''; document.querySelector('#hGo').click(); })()`); await sleep(1500);
  await shot("alarms-history");

  console.log("== statistics");
  await go("/alarms/statistics");
  check("six numbers", await ev(`document.querySelectorAll('#kpis .al-kpi').length`), 6);
  check("most frequent alarms listed with a share", await ev(`(() => { const r = document.querySelector('#top tbody tr'); return !!r && /%/.test(r.textContent) })()`), true);
  check("chattering alarm found", await ev(`/temperature/.test(document.querySelector('#chat tbody').textContent)`), true);
  check("a bar for every day", await ev(`document.querySelectorAll('#days div').length >= 7`), true);
  await shot("alarms-statistics");

  console.log("== classes");
  await go("/alarms/classes");
  check("three built-in classes", await ev(`document.querySelectorAll('#cls tbody tr').length >= 3`), true);
  await ev(`document.querySelector('#newClass').click()`); await sleep(400);
  check("class dialog", await ev(`document.querySelector('#clsDlg').open`), true);
  await shot("alarms-class-dialog");
  await ev(`document.querySelector('#clsDlg').close()`);
  await shot("alarms-classes");

  console.log("== the bar on another page");
  await go("/devices");
  check("alarm bar on the Devices page", await ev(`(() => { const b = document.querySelector('#alarmBar'); return !!b && !b.hidden && !!b.querySelector('#abSound') && /\\d/.test(b.querySelector('.ab-count').textContent) })()`), true);
  await shot("alarms-bar");
  await ev(`document.querySelector('#abHide').click()`); await sleep(300);
  check("the bar can be hidden", await ev(`document.querySelector('#alarmBar').hidden`), true);

  check("no script errors", problems.length ? JSON.stringify(problems) : "none", "none");
  console.log(`\npassed ${pass}, failed ${fail}`);
  proc.kill(); process.exit(fail ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
