// Headless Chrome: the screens of spec 034 (0.86.0) — System → Operating system (updates, the upgrade with its log,
// health), System → Logs, the Remote management card, and Displays with the tiles a display reports and the Kiosk PCs
// section (a PC registered and reporting through its agent's API, the setup dialog, a screenshot with one picture per
// monitor as the agent of 0.86.5 sends them). Against the agent's simulation.
// Screenshots go to OUT (remote-*.png); LANG=sk takes them in Slovak.
//   BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/remote_screens.js
const { spawn } = require("child_process");
const http = require("http");
const fs = require("fs");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "en";
const port = 9367;
const proc = spawn(chrome, ["--headless=new", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-remote", "--window-size=1366,900", "about:blank"], { stdio: "ignore" });
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
  const go = async (p) => { await send("Page.navigate", { url: base + p }); for (let i = 0; i < 100; i++) { await sleep(150); const v = await ev(`location.pathname + '|' + document.readyState`).catch(() => ""); if (v === p.split("?")[0] + "|complete") return; } };
  const api = (m, p, b, hdr) => ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(me=>fetch(${JSON.stringify(p)},{method:${JSON.stringify(m)},credentials:'same-origin',headers:Object.assign({'Content-Type':'application/json','X-CSRF-Token':me.csrf_token},${JSON.stringify(hdr || {})}),body:${b ? JSON.stringify(JSON.stringify(b)) : "undefined"}})).then(async r=>({status:r.status, body: await r.json().catch(()=>null)}))`);
  const shot = async (name) => { const r = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true }); fs.writeFileSync(path.join(out, `remote-${name}${lang === "en" ? "" : "-" + lang}.png`), Buffer.from(r.result.data, "base64")); };
  const waitFor = async (expr, ms = 15000) => { const t0 = Date.now(); while (Date.now() - t0 < ms) { if (await ev(expr) === true) return true; await sleep(300); } return false; };

  await go("/login");
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  if (lang !== "en") { await api("PUT", "/api/v1/i18n/languages", { enabled: ["en", lang], default: "en", reason: "probe" }); await ev(`localStorage.setItem('ctrl32.locale', ${JSON.stringify(lang)})`); }
  const suf = Date.now() % 100000;
  let KID = null, DID = null, CAMS = [], WALL = null;
  cleanup = async () => {
    if (KID) await api("DELETE", `/api/v1/kiosks/${KID}?reason=probe`);
    if (DID) await api("DELETE", `/api/v1/displays/${DID}?reason=probe`);
    if (WALL) await api("DELETE", `/api/v1/video/walls/${WALL}?reason=probe`);
    for (const c of CAMS) await api("DELETE", `/api/v1/video/cameras/${c}?reason=probe`);
    await api("PUT", "/api/v1/server/remote", { mesh_url: "", mesh_node: "", reason: "probe cleanup" });
    cleanup = async () => {};
  };

  // ---- the server
  await api("PUT", "/api/v1/server/remote", { mesh_url: "https://mesh.example.com", mesh_node: "h3l$p4ZnbBa43Fd82gTIlqKsfGNo02d1gXFhiM78EM5DSwiOzo3jLsRJ3sNgrlnO", reason: "probe" });
  await go("/server");
  check("overview: health of the host", await waitFor(`/RAID md0/.test(document.querySelector('#kvHost').textContent)`), true);
  check("overview: remote management with the emergency button", await waitFor(`!document.querySelector('#remoteOpen').hidden`), true);
  await ev(`document.querySelector('#remoteForm').closest('.card').scrollIntoView()`); await sleep(400);
  await shot("server-overview");
  await go("/server/os");
  check("operating system: updates listed", await waitFor(`document.querySelectorAll('#osPkgs tbody tr').length >= 1 && /[0-9]/.test(document.querySelector('#kvOS').textContent)`), true);
  await shot("os-updates");
  const up = await api("POST", "/api/v1/server/os/upgrade", { reason: "probe: monthly updates" });
  check("upgrade started", up.status, 202);
  await go("/server/os");
  check("the job's log is shown", await waitFor(`/apt-get/.test(document.querySelector('#osLog').textContent)`), true);
  check("…and the reboot required at the end", await waitFor(`/libssl3t64/.test(document.querySelector('#kvOS').textContent)`, 20000), true);
  await shot("os-after-upgrade");
  await waitFor(`!document.querySelector('#osRebootBtn').disabled`); await ev(`document.querySelector('#osRebootBtn').click()`); await sleep(500);
  check("reboot asks for a reason and a confirmation", await ev(`document.querySelector('#rebootDlg').open && !!document.querySelector('#rebootForm input[name=sure]')`), true);
  await shot("os-reboot-dialog");
  await ev(`document.querySelector('#rebootDlg').close()`);
  await go("/server/logs");
  check("logs of the service", await waitFor(`document.querySelector('#logText').textContent.length > 10`), true);
  await shot("logs");

  // ---- a display with tiles, a kiosk PC
  for (const n of ["Benzinka", "Restauracia", "Kruhac"]) { const r = await api("POST", "/api/v1/video/cameras", { name: `${n} ${suf}`, url: "rtsp://127.0.0.1:1/x", record: "off", enabled: false, reason: "probe" }); CAMS.push(r.body.id); }
  const w = await api("POST", "/api/v1/video/walls", { name: `Stena ${suf}`, reason: "probe", screens: [{ cols: 3, rows: 1, tiles: CAMS }] }); WALL = w.body.id;
  const k = await api("POST", "/api/v1/kiosks", { name: `Wall PC ${suf}`, monitors: 2, server_url: "http://10.48.8.30:8080", reason: "probe" });
  check("kiosk PC registered", k.status, 201);
  KID = k.body.kiosk.id; const TOK = k.body.token;
  await go("/display");
  const code = await ev(`new Promise(r => { const t = setInterval(() => { const c = document.querySelector('#code').textContent.trim(); if (/^[A-Z0-9]{4}-[A-Z0-9]{4}$/.test(c)) { clearInterval(t); r(c); } }, 300); setTimeout(() => r(''), 15000); })`);
  const pr = await api("POST", "/api/v1/displays/pair", { code, name: `Horny monitor ${suf}`, content: { items: [{ kind: "wall", wall: WALL, screen: 0 }], popup: "none" }, reason: "probe" });
  check("display paired", pr.status, 201); DID = pr.body && pr.body.id;
  await sleep(4000);
  const h = await ev(`fetch('/api/v1/display/health',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({page_version:'0.86.0',item:'Stena',screen:{w:1920,h:1080,vw:1920,vh:1080,dpr:1},kiosk:${JSON.stringify(KID)},monitor:0,tiles:${JSON.stringify(CAMS.map((c, i) => ({ camera: c, state: i === 1 ? "stalled" : "playing", since_s: i === 1 ? 35 : 600 })))}})}).then(r=>r.status)`);
  check("the display reports its tiles", h, 200);
  const report = { hostname: "DESKTOP-MHKQKCH", windows: "Microsoft Windows 11 Pro 10.0.26200", model: "HP EliteDesk 800 G4 DM", uptime_s: 93600, cpu_pct: 19, gpu_decode_pct: 43, ram_total_mb: 7900, ram_free_mb: 4200, disk_free_gb: 182.4, browser: "141.0.7390.123", browser_windows: 2, windows_updates: 0, reboot_pending: false, user: "DESKTOP-MHKQKCH\\stena", ip: ["10.48.8.20"] };
  const sc = await ev(`fetch('/api/v1/kiosk/scripts',{headers:{'X-Kiosk-Token':${JSON.stringify(TOK)}}}).then(r=>r.json())`);
  report.scripts_version = sc.version;
  const rr = await ev(`fetch('/api/v1/kiosk/report',{method:'POST',headers:{'Content-Type':'application/json','X-Kiosk-Token':${JSON.stringify(TOK)}},body:JSON.stringify({report:${JSON.stringify(report)},result:null})}).then(r=>r.status)`);
  check("the kiosk agent reports", rr, 200);
  await api("PUT", `/api/v1/kiosks/${KID}`, { displays: [{ id: DID, monitor: 0 }], reason: "probe" });
  await go("/displays");
  check("displays: 2/3 playing and the camera that is not", await waitFor(`/2\\/3/.test(document.querySelector('#list').textContent) && document.querySelector('#list').textContent.includes('Restauracia ${suf}')`), true);
  check("kiosk PC listed online with its report", await waitFor(`document.querySelector('#kiosks').textContent.includes('DESKTOP-MHKQKCH') && !!document.querySelector('#kiosks .badge.ok')`), true);
  await shot("displays");
  await ev(`document.querySelector('[data-kedit="${KID}"]').click()`); await sleep(600);
  check("edit: the monitors with their displays", await ev(`document.querySelectorAll('#kioskMonitors select').length`), 2);
  await shot("kiosk-edit");
  await ev(`document.querySelector('#kioskDlg').close()`);
  await ev(`setupDialog(KIOSKS.find(k => k.id === ${JSON.stringify(KID)}), ${JSON.stringify(TOK)})`); await sleep(400);
  await shot("kiosk-setup");
  await ev(`document.querySelector('#kioskSetupDlg').close()`);

  // ---- a screenshot: one picture per monitor, taken by the kiosk browsers (0.86.5); this page stands in for them
  await api("PUT", `/api/v1/kiosks/${KID}`, { screenshots: true, reason: "probe" });
  const jpg = (await send("Page.captureScreenshot", { format: "jpeg", quality: 60 })).result.data;
  const cmd = await api("POST", `/api/v1/kiosks/${KID}/command`, { action: "screenshot", reason: "probe" });
  check("screenshot asked", cmd.status, 200);
  await ev(`void screenshot(KIOSKS.find(k => k.id === ${JSON.stringify(KID)}))`);
  for (const m of [0, 1]) {
    check(`monitor ${m + 1} sends its picture`, await ev(`fetch('/api/v1/kiosk/screenshot?seq=${cmd.body.seq}&monitor=${m}',{method:'POST',headers:{'Content-Type':'image/jpeg','X-Kiosk-Token':${JSON.stringify(TOK)}},body:Uint8Array.from(atob(${JSON.stringify(jpg)}), c => c.charCodeAt(0))}).then(r=>r.status)`), 200);
  }
  check("the dialog shows one picture per monitor", await waitFor(`document.querySelectorAll('#shotImgs img').length === 2 && [...document.querySelectorAll('#shotImgs img')].every(i => i.complete && i.naturalWidth > 0)`), true);
  check("…named by monitor and display", await ev(`document.querySelector('#shotImgs figcaption').textContent.includes('Horny monitor ${suf}')`), true);
  await shot("kiosk-screenshot");
  await ev(`document.querySelector('#shotDlg').close()`);

  await cleanup();
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  console.log(`passed ${pass}, failed ${fail}`);
  proc.kill(); process.exit(fail || problems.length ? 1 : 0);
})().catch(async (e) => { console.error(e); console.log("problems:", JSON.stringify(problems)); try { await cleanup(); } catch (x) { /* the browser is gone */ } proc.kill(); process.exit(1); });
