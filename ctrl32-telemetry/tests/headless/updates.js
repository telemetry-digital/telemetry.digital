// Headless Chrome: System → Updates (0.76.0, ADR-0024) shows the status with the signature and the trusted keys, the
// update mode (the window fields only in the automatic mode), the online installation with the backup option, the
// offline package upload with the password, and the history; System → Server → E-mail (SMTP) shows the settings with
// their source, the write-only password, the warning of an unencrypted connection and the test e-mail with the SMTP
// conversation. No script errors; the Slovak translation; screenshots in English and Slovak.
// Runs against the agent simulation (run-all.sh starts the server with config-vpn.toml for this probe).
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9367;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile-updates", "--window-size=1300,1600", "about:blank"], { stdio: "ignore" });
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
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const shot = async (name, full) => { const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: !!full }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));

  // ---- Updates
  await send("Page.navigate", { url: base + "/server/updates" }); await sleep(3500);
  const u = await ev(`({kv: document.querySelector('#kvUpd').textContent.replace(/\\s+/g,' '),
    modes: [...document.querySelectorAll('#updModeForm input[name=mode]')].map(r => r.value + (r.checked ? '*' : '')).join(','),
    autoHidden: document.querySelector('#updAutoFields').hidden, weekdays: document.querySelector('#updWeekday').options.length,
    backup: document.querySelector('#updForm input[name=backup]').checked, pkg: !!document.querySelector('#pkgForm input[type=file]'),
    pw: !!document.querySelector('#pkgForm input[name=password]'), hist: document.querySelectorAll('#updHist tbody tr').length,
    menu: [...document.querySelectorAll('.subnav a')].map(a => a.textContent.trim()).join('|')})`);
  console.log("updates:", JSON.stringify(u).slice(0, 600));
  if (!/Trusted release keys/.test(u.kv) || !/release-2026/.test(u.kv)) problems.push("trusted keys not shown");
  if (u.modes !== "manual,notify*,auto") problems.push("update modes: " + u.modes);
  if (!u.autoHidden) problems.push("window fields visible outside the automatic mode");
  if (u.weekdays !== 7) problems.push("weekdays " + u.weekdays);
  if (!u.backup || !u.pkg || !u.pw) problems.push("install or package form incomplete");
  if (!/E-mail \(SMTP\)/.test(u.menu)) problems.push("no E-mail (SMTP) in the server menu");
  await ev(`(()=>{ const r = document.querySelector('#updModeForm input[value=auto]'); r.checked = true; r.dispatchEvent(new Event('change', {bubbles: true})); })()`);
  await sleep(300);
  if (await ev(`document.querySelector('#updAutoFields').hidden`)) problems.push("window fields not shown in the automatic mode");
  await shot("updates-en.png", true);
  await ev(`(()=>{ const r = document.querySelector('#updModeForm input[value=notify]'); r.checked = true; r.dispatchEvent(new Event('change', {bubbles: true})); })()`);

  // ---- E-mail (SMTP)
  await send("Page.navigate", { url: base + "/server/email" }); await sleep(3000);
  const e = await ev(`({note: document.querySelector('#smtpNote').textContent, pwType: document.querySelector('#smtpForm input[name=password]').type,
    pwValue: document.querySelector('#smtpForm input[name=password]').value, sec: [...document.querySelector('#smtpForm select[name=security]').options].map(o=>o.value).join(','),
    title: document.title})`);
  console.log("email:", JSON.stringify(e));
  if (e.pwType !== "password" || e.pwValue !== "") problems.push("password field not write-only");
  if (e.sec !== "starttls,tls,none") problems.push("security options " + e.sec);
  if (!e.note) problems.push("no source note");
  await ev(`(()=>{ const f = document.querySelector('#smtpForm'); f.host.value = '127.0.0.1'; f.port.value = '2599'; f.security.value = 'none'; f.security.dispatchEvent(new Event('change')); f.from.value = 'telemetry@example.com'; f.from_name.value = 'Telemetry'; })()`);
  if (await ev(`document.querySelector('#smtpNoneWarn').hidden`)) problems.push("no warning for an unencrypted connection");
  await ev(`(()=>{ const f = document.querySelector('#smtpTestForm'); f.to.value = 'ops@example.com'; f.requestSubmit(); })()`);
  await sleep(4000);
  const t = await ev(`({shown: !document.querySelector('#smtpTranscript').hidden, text: document.querySelector('#smtpTranscript').textContent, err: document.querySelector('#smtpTestErr').textContent})`);
  console.log("test e-mail (port without a server):", t.err.slice(0, 120), "| transcript lines:", t.text.split("\n").length);
  if (!t.shown || !/connecting to 127\.0\.0\.1 port 2599/.test(t.text) || !t.err) problems.push("no SMTP conversation shown for a failing test");
  await shot("email-en.png", true);

  // ---- Slovak
  const langs0 = await ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); window.__csrf=me.csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  const putLangs = (enabled, dflt) => ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify(enabled)},default:${JSON.stringify(dflt)},reason:"headless updates probe"})}).then(r=>r.status)`);
  if (!langs0.enabled.includes("sk")) console.log("enable sk:", await putLangs([...langs0.enabled, "sk"], langs0.default));
  await ev(`localStorage.setItem("ctrl32.locale", "sk")`);
  await send("Page.navigate", { url: base + "/server/updates" }); await sleep(3500);
  const sk = await ev(`document.querySelector('section[data-section=updates]').textContent.replace(/\\s+/g,' ')`);
  console.log("sk updates:", /Režim aktualizácií/.test(sk) && /Nahrať balík/.test(sk) && /Dôveryhodné kľúče vydaní/.test(sk) ? "translated" : "NOT translated");
  if (!/Režim aktualizácií/.test(sk) || !/Nahrať balík/.test(sk) || !/Dôveryhodné kľúče vydaní/.test(sk)) problems.push("updates not translated to Slovak");
  await shot("updates-sk.png", true);
  await send("Page.navigate", { url: base + "/server/email" }); await sleep(3000);
  const ske = await ev(`document.querySelector('section[data-section=email]').textContent.replace(/\\s+/g,' ')`);
  if (!/Odchádzajúci e-mail/.test(ske) || !/Poslať skúšobný e-mail/.test(ske)) problems.push("e-mail page not translated to Slovak");
  await shot("email-sk.png", true);
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  if (!langs0.enabled.includes("sk")) { await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`); console.log("restore languages:", await putLangs(langs0.enabled, langs0.default)); }
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
