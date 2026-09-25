// Headless Chrome: My account → Notifications (state without push support in headless http), Settings →
// Notifications channel dialog with the web_push and voice_http kinds; screenshots.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9344;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile12", "--window-size=1440,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; let problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await send("Page.navigate", { url: base + "/account/push" }); await sleep(3000);
  console.log("account push: state:", await ev(`document.querySelector('#pushState').innerText`), "| rows:", await ev(`document.querySelectorAll('#pushTable tbody tr').length`), "| nav link:", await ev(`!![...document.querySelectorAll('#sidebar a')].find(a => a.getAttribute('href') === '/account/push')`), "| err:", await ev(`document.querySelector('#pushErr').textContent`));
  const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/push-account.png", Buffer.from(s.result.data, "base64"));
  await send("Page.navigate", { url: base + "/settings/notifications" }); await sleep(3000);
  const suf = Math.floor(Math.random() * 1e5);
  console.log("channel dialog web_push:", await ev(`(() => { document.querySelector('#newChanBtn').click(); const f = document.querySelector('#chanForm'); f.kind.value = 'web_push'; f.kind.dispatchEvent(new Event('change')); const vis = [...f.querySelectorAll('[data-k]')].filter(el => !el.hidden).map(el => el.dataset.k); f.name.value = 'push-probe-${suf}'; f.users.value = 'all'; f.reason.value = 'probe'; f.requestSubmit(); return JSON.stringify(vis); })()`)); await sleep(2000);
  console.log("channels:", JSON.stringify(await ev(`[...document.querySelectorAll('#chanTable tbody tr')].map(r => r.innerText.replace(/\\t/g, ' | ').slice(0, 90)).filter(t => t.includes('push-probe-${suf}'))`)), "err:", await ev(`document.querySelector('#chanErr').textContent`), "open:", await ev(`document.querySelector('#chanDlg').open`));
  console.log("voice kind fields:", await ev(`(() => { document.querySelector('#newChanBtn').click(); const f = document.querySelector('#chanForm'); f.kind.value = 'voice_http'; f.kind.dispatchEvent(new Event('change')); return JSON.stringify([...f.querySelectorAll('[data-k]')].filter(el => !el.hidden).map(el => el.dataset.k)); })()`));
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/push-channel.png", Buffer.from(s2.result.data, "base64"));
  console.log("cleanup:", await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const cs = await (await fetch('/api/v1/notification-channels',{credentials:'same-origin'})).json(); const c = cs.find(x => x.name === 'push-probe-${suf}'); if(!c) return 'none'; return (await fetch('/api/v1/notification-channels/' + c.id + '?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`));
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
