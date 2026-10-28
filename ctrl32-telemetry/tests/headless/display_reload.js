// Headless Chrome: a display reloads its page (0.85.1) — on the operator's Reload even when it is the first command
// the page ever got, never on a command issued before the page loaded, and by itself after a server update (another
// server version in the state), 0–30 s later and at most once in 10 minutes.
//   BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/display_reload.js
const { spawn } = require("child_process");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const port = 9363;
const proc = spawn(chrome, ["--headless=new", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-reload", "--window-size=1280,800", "about:blank"], { stdio: "ignore" });
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
  const go = async (path) => { await send("Page.navigate", { url: base + path }); for (let i = 0; i < 100; i++) { await sleep(150); const v = await ev(`location.pathname + '|' + document.readyState`).catch(() => ""); if (v === path + "|complete") return; } };
  const api = (m, p, b) => ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(me=>fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':me.csrf_token},body:${b ? JSON.stringify(JSON.stringify(b)) : "undefined"}})).then(async r=>({status:r.status, body: await r.json().catch(()=>null)}))`);
  // a marker that a reload removes
  const mark = () => ev(`window.__probe = 1`);
  const reloaded = async (ms) => { const t0 = Date.now(); while (Date.now() - t0 < ms) { await sleep(500); const v = await ev(`window.__probe === 1`).catch(() => false); if (v === false) return true; } return false; };

  await go("/login");
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  const suf = Date.now() % 100000;
  const dash = await api("POST", "/api/v1/dashboards", { slug: `reload-${suf}`, name: `Reload ${suf}`, spec: { version: 1, kind: "dashboard", layout: { type: "grid", columns: 12, row_height: 80 }, widgets: [] }, reason: "probe" });
  let DID = null;
  cleanup = async () => {
    if (DID) await api("DELETE", `/api/v1/displays/${DID}?reason=probe`);
    await api("DELETE", `/api/v1/dashboards/reload-${suf}?reason=probe`);
    cleanup = async () => {};
  };
  check("dashboard for the display", dash.status, 201);
  // a reload command issued before the display page loads must not reload it
  await go("/display");
  const code = await ev(`new Promise(r => { const t = setInterval(() => { const c = document.querySelector('#code').textContent.trim(); if (/^[A-Z0-9]{4}-[A-Z0-9]{4}$/.test(c)) { clearInterval(t); r(c); } }, 300); setTimeout(() => r(''), 15000); })`);
  const pr = await api("POST", "/api/v1/displays/pair", { code, name: `Reload ${suf}`, content: { items: [{ kind: "dashboard", slug: `reload-${suf}`, seconds: 60 }], popup: "none" }, reason: "probe" });
  check("display paired", pr.status, 201);
  DID = pr.body && pr.body.id;
  await sleep(6000);
  await api("POST", `/api/v1/displays/${DID}/command`, { action: "reload" });
  await ev(`sessionStorage.clear()`); // a fresh tab: no command remembered
  await go("/display"); await sleep(1500); await mark();
  check("an old reload command is ignored by a page loaded after it", await reloaded(10000), false);
  // the first command this page gets is Reload: it reloads (0.84.2 dropped it)
  await api("POST", `/api/v1/displays/${DID}/command`, { action: "reload" });
  check("the first command, Reload, reloads the page", await reloaded(15000), true);
  await sleep(2000); await mark();
  check("after the reload the same command is not handled again", await reloaded(10000), false);
  // a server update: the state carries another server version → reload within 30 s
  await ev(`serverVersion = 'older-than-' + serverVersion`);
  check("another server version reloads the display within 30 s", await reloaded(40000), true);
  await sleep(3000); await mark();
  await ev(`serverVersion = 'older-again'`);
  check("at most one automatic reload in 10 minutes", await reloaded(40000), false);

  await cleanup();
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  console.log(`passed ${pass}, failed ${fail}`);
  proc.kill(); process.exit(fail || problems.length ? 1 : 0);
})().catch(async (e) => { console.error(e); console.log("problems:", JSON.stringify(problems)); try { await cleanup(); } catch (x) { /* the browser is gone */ } proc.kill(); process.exit(1); });
