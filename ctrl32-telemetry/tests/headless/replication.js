// Headless Chrome: the Redundant database section of the Server page (spec 003, US4). The probe creates a pairing
// code through the dialog, sees the waiting replica in the list, opens its details, checks that a detach of a replica
// that never connected is refused with a message, and drops it with a reason.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9364;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile24", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
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
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const key = async (k, code) => { await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); };

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await send("Page.navigate", { url: base + "/server/redundancy" }); await sleep(3000);
  expect("section shown", await ev(`!!document.querySelector('[data-section="redundancy"].active')`), true);
  expect("node role", await ev(`document.querySelector('#kvNode').textContent`), (t) => typeof t === "string" && t.includes("primary"));
  expect("nav entry", await ev(`[...document.querySelectorAll('a')].some(a => a.getAttribute('href') === '/server/redundancy')`), true);
  const name = "Probe replica " + Math.floor(Math.random() * 1e6);
  await ev(`(() => { document.querySelector('#replAttachBtn').click(); const f = document.querySelector('#replForm'); f.mode.value = 'continuous'; f.name.value = ${JSON.stringify(name)}; f.reason.value = 'headless probe'; f.requestSubmit(); })()`); await sleep(2000);
  expect("pairing code shown", await ev(`document.querySelector('#replCodeVal').textContent`), (c) => /^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$/.test(c || ""));
  expect("attach command", await ev(`document.querySelector('#replCmd').textContent`), (c) => typeof c === "string" && c.includes("replica attach -primary") && c.includes("-code"));
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/replication-code.png", Buffer.from(s1.result.data, "base64"));
  await ev(`document.querySelector('#replDlg').close()`); await sleep(1500);
  expect("code waiting in the list", await ev(`[...document.querySelectorAll('#replicas tbody tr')].some(t => t.textContent.includes(${JSON.stringify(name)}) && t.textContent.includes('waiting for the second PC'))`), true);
  // The second PC pairs (what `replica attach` does first): the replica appears, not connected yet.
  const code = await ev(`document.querySelector('#replCodeVal').textContent`);
  const paired = await ev(`(async () => { const v = (await (await fetch('/healthz')).json()).version; const pair = (m) => fetch('/api/v1/replication/pair', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({code: ${JSON.stringify(code)}, pg_major: m, ctrl32_version: v, hostname: 'probe-pc'})}); /* the refusal names the primary's PostgreSQL version (pairing is rate-limited: no guessing) */ const r0 = await pair(1); const m = /PostgreSQL (\\d+)/.exec((await r0.json()).error || ''); if (!m) return r0.status; return (await pair(+m[1])).status; })()`);
  expect("paired", paired, 201);
  await ev(`renderRedundancy()`); await sleep(1500);
  const row = await ev(`(() => { const tr = [...document.querySelectorAll('#replicas tbody tr')].find(t => t.textContent.includes(${JSON.stringify(name)})); return tr ? tr.textContent : null; })()`);
  expect("replica listed as waiting", row, (t) => typeof t === "string" && t.includes("waiting for the replica"));
  await ev(`[...document.querySelectorAll('#replicas tbody tr')].find(t => t.textContent.includes(${JSON.stringify(name)})).querySelector('td').click()`); await sleep(1500);
  expect("details with history", await ev(`(() => { const d = document.querySelector('#replDetail'); return !d.hidden && d.textContent.includes('History') && d.textContent.includes('pairing'); })()`), true);
  await ev(`[...document.querySelectorAll('#replicas tbody tr')].find(t => t.textContent.includes(${JSON.stringify(name)})).querySelector('[data-rdetach]').click()`); await sleep(500);
  expect("partial option offered when not in sync", await ev(`!document.querySelector('#replPartialRow').hidden`), true);
  await ev(`(() => { const f = document.querySelector('#replDetachForm'); f.reason.value = 'probe'; f.requestSubmit(); })()`); await sleep(1200);
  expect("detach of a never-connected replica refused", await ev(`document.querySelector('#replDetachErr').textContent`), (t) => typeof t === "string" && t.includes("cannot be detached"));
  await ev(`document.querySelector('#replDetachDlg').close()`);
  await ev(`[...document.querySelectorAll('#replicas tbody tr')].find(t => t.textContent.includes(${JSON.stringify(name)})).querySelector('[data-rdrop]').click()`); await sleep(500);
  await ev(`(() => { const f = document.querySelector('#reasonForm'); f.reason.value = 'probe cleanup'; f.requestSubmit(); })()`); await sleep(2500);
  expect("replica dropped", await ev(`[...document.querySelectorAll('#replicas tbody tr')].find(t => t.textContent.includes(${JSON.stringify(name)})).textContent`), (t) => typeof t === "string" && t.includes("dropped"));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/replication-list.png", Buffer.from(s2.result.data, "base64"));
  console.log(problems.length ? "problems: " + JSON.stringify(problems) : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
