// Headless Chrome: AI assistants over MCP (spec 022). Settings → AI assistants (switch on with a reason, the address and
// how to connect), the consent page of an assistant reached signed out (sign-in with ?next= leads back to it), approval
// with the choice "reading only", the redirect with the code, and My account → AI assistants with the connected
// assistant and disconnecting it. Screenshots mcp-*.png go to OUT.
const { spawn } = require("child_process");
const crypto = require("crypto");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9373;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-mcp", "--window-size=1300,1000", "about:blank"], { stdio: "ignore" });
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
    if (msg.method === "Fetch.requestPaused") // the assistant's redirect address answers like a local callback server
      ws.send(JSON.stringify({ id: 900000 + msg.params.requestId.length + Math.floor(Math.random() * 1000), method: "Fetch.fulfillRequest", params: { requestId: msg.params.requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "text/html" }], body: Buffer.from("<p>Assistant connected.</p>").toString("base64") } }));
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  await send("Fetch.enable", { patterns: [{ urlPattern: "http://127.0.0.1:45871/*" }] });
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 8000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2000) => { await send("Page.navigate", { url: url.startsWith("http") ? url : base + url }); await sleep(wait); };
  const login = `fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`;

  await go("/login", 1500);
  expect("login", await ev(login), 200);
  await go("/settings/ai", 2500);
  expect("Settings → AI assistants", await until(`document.querySelector('section[data-section=ai]').classList.contains('active') && !!document.querySelector('#aiUrl').textContent`), true);
  await ev(`(() => { const f = document.querySelector('#aiForm'); f.enabled.checked = true; f.reason.value = 'headless probe: assistants allowed'; f.requestSubmit(); })()`);
  expect("switched on", await until(`/on/.test(document.querySelector('#aiOn').textContent)`), true);
  expect("address and AI assistant command", await ev(`document.querySelector('#aiCli').textContent.startsWith('assistant mcp add --transport http ctrl32 ') && document.querySelector('#aiUrl').textContent.endsWith('/mcp')`), true);
  await shot("mcp-settings.png");

  // an assistant registers and sends the user to the consent page (signed out first: sign-in leads back)
  const redirect = "http://127.0.0.1:45871/callback";
  const reg = await ev(`fetch('/oauth/register',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({client_name:'assistant',redirect_uris:['${redirect}'],token_endpoint_auth_method:'none'})}).then(r=>r.json())`);
  expect("client registered", !!reg.client_id, true);
  const verifier = crypto.randomBytes(48).toString("base64url");
  const challenge = crypto.createHash("sha256").update(verifier).digest("base64url");
  const q = new URLSearchParams({ response_type: "code", client_id: reg.client_id, redirect_uri: redirect, code_challenge: challenge, code_challenge_method: "S256", state: "probe-state", scope: "mcp" });
  await ev(`fetch('/api/v1/auth/logout',{method:'POST',credentials:'same-origin'}).then(r=>r.status)`);
  await go("/oauth/authorize?" + q, 2500);
  expect("signed out: to sign-in with next", await ev(`location.pathname + ' ' + new URLSearchParams(location.search).get('next').startsWith('/oauth/authorize?')`), "/login true");
  await ev(`(() => { const f = document.querySelector('#f'); f.username.value = 'admin'; f.password.value = 'admin1234'; f.requestSubmit(); })()`);
  expect("back on the consent page after sign-in", await until(`location.pathname === '/oauth/authorize' && !document.querySelector('#ask').hidden`, 10000), true);
  expect("consent names the assistant, user and where the code goes", await ev(`[document.querySelector('#client').textContent, document.querySelector('#user').textContent.includes('admin'), document.querySelector('#host').textContent].join(' | ')`), "assistant | true | 127.0.0.1:45871");
  await shot("mcp-consent.png");
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await sleep(300);
  await shot("mcp-consent-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");
  await ev(`document.querySelector('input[value=read]').click()`);
  await ev(`document.querySelector('#allow').click()`);
  expect("redirect to the assistant with the code", await until(`location.href.startsWith('${redirect}')`, 8000), true);
  const back = new URL(await ev(`location.href`));
  expect("state and issuer", back.searchParams.get("state") + " " + !!back.searchParams.get("iss"), "probe-state true");
  const tok = await new Promise((res) => {
    const body = new URLSearchParams({ grant_type: "authorization_code", code: back.searchParams.get("code"), client_id: reg.client_id, redirect_uri: redirect, code_verifier: verifier }).toString();
    const u = new URL(base + "/oauth/token");
    const r = http.request({ host: u.hostname, port: u.port, path: u.pathname, method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" } }, (rs) => { let d = ""; rs.on("data", (c) => (d += c)); rs.on("end", () => res(JSON.parse(d))); });
    r.end(body);
  });
  expect("code exchanged: reading only", tok.scope, "mcp:read");

  // My account → AI assistants: the connected assistant, disconnect with a reason
  await go("/account/ai", 2500);
  expect("My account lists the assistant", await until(`[...document.querySelectorAll('#aiTable tbody tr')].some(tr => tr.textContent.includes('assistant') && tr.textContent.includes('reading only'))`), true);
  await shot("mcp-account.png");
  await ev(`window.prompt = () => 'headless probe: disconnect'`);
  const before = await ev(`document.querySelectorAll('[data-grant]').length`);
  expect("disconnect button", before, (n) => n >= 1);
  await ev(`document.querySelector('[data-grant]').click()`);
  expect("disconnected", await until(`document.querySelectorAll('[data-grant]').length === ${before} - 1`), true);
  await go("/settings/ai", 2000);
  await ev(`(() => { const f = document.querySelector('#aiForm'); f.enabled.checked = false; f.reason.value = 'headless probe: off again'; f.requestSubmit(); })()`);
  expect("switched off again", await until(`/off/.test(document.querySelector('#aiOn').textContent)`), true);

  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
