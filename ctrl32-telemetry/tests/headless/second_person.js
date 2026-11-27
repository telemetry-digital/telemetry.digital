// Headless Chrome: four eyes everywhere (spec 040, 0.94.0). The switches in Settings → Security policy; a control of a
// dashboard marked "needs a second person" opens the dialog of the second person, who approves on the spot with their
// own name and password; left waiting, the action is in Approvals, where a different user approves it; work with
// cameras asks for an approval that lasts some minutes and then goes through.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/second_person.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9384;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-second", "--window-size=1440,1000", "--lang=" + (lang || "en")], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const json = (url) => new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej));
let failed = 0;
const expect = (label, cond, detail) => { console.log((cond ? "  ok   " : "  FAIL ") + label + (cond ? "" : " — " + JSON.stringify(detail))); if (!cond) failed++; };

// the second person works at another computer: a plain HTTP client with its own cookies
function client() {
  let cookie = "", csrf = "";
  const call = async (method, url, body) => {
    const r = await fetch(base + url, { method, headers: { "Content-Type": "application/json", Cookie: cookie, "X-CSRF-Token": csrf }, body: body === undefined ? undefined : JSON.stringify(body) });
    const set = r.headers.getSetCookie ? r.headers.getSetCookie() : [];
    if (set.length) cookie = set.map((c) => c.split(";")[0]).join("; ");
    const data = await r.json().catch(() => ({}));
    return { status: r.status, data };
  };
  return { call, login: async (username, password) => { const r = await call("POST", "/api/v1/auth/login", { username, password }); csrf = (await call("GET", "/api/v1/auth/me")).data.csrf_token || ""; return r.status; } };
}

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
  const until = async (expr, ms = 8000) => { for (let i = 0; i < ms / 150; i++) { if ((await ev(expr)) === true) return true; await sleep(150); } return false; };
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2200); if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`); };
  const suf = String(Math.floor(Math.random() * 90000) + 10000);
  const second = "marta_" + suf, secondPw = "Second-Person-Passw0rd-" + suf;
  const OFF = { four_eyes_commands: false, four_eyes_alarms: false, four_eyes_attributes: false, four_eyes_cameras: false, four_eyes_displays: false, four_eyes_minutes: 15 };
  const dlgOpen = `!!(document.querySelector('#spDlg') && document.querySelector('#spDlg').open)`;
  const fill = (user, pw, extra) => ev(`(() => { const f = document.querySelector('#spDlg form'); f.spuser.value = ${JSON.stringify(user)}; f.sppass.value = ${JSON.stringify(pw)}; document.querySelector('#spDlg .sp-err').textContent = ''; ${extra || ""} return 1; })()`);
  const submit = () => ev(`(document.querySelector('#spDlg form').requestSubmit(), 1)`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, 1)`);
  await go("/settings/policy");

  // ---- fixtures: a second user, a device, a dashboard with a marked and a plain switch ----
  const fx = await ev(`(async () => {
    await ctrl32.api('PUT', '/api/v1/settings/policy', Object.assign(${JSON.stringify(OFF)}, { reason: 'headless start' }));
    const u = await ctrl32.api('POST', '/api/v1/users', { username: '${second}', full_name: 'Marta Shift Leader', email: '${second}@example.com', roles: ['engineer'], reason: 'headless probe' });
    const dev = await ctrl32.api('POST', '/api/v1/devices', { external_id: 'sp-${suf}', name: 'Motor M1', reason: 'headless probe' });
    const sw = (id, x, title, key, mark) => ({ id, type: 'switch', geometry: { x, y: 0, w: 4, h: 3 }, bindings: { devices: [dev.id] }, params: { title, key, confirm: false, second_person: mark } });
    const d = await ctrl32.api('POST', '/api/v1/dashboards', { name: 'Second person ${suf}', spec: { version: 1, range: '1h', refresh_s: 30, widgets: [sw('marked', 0, 'Motor M1', 'run', true), sw('plain', 4, 'Fan', 'fan', false)] }, reason: 'headless probe' });
    const cat = await ctrl32.api('GET', '/api/v1/dashboards/catalog');
    return { pw: u.initial_password, dev: dev.id, slug: d.slug, marks: cat.widgets.filter(w => (w.params || []).some(p => p.name === 'second_person')).map(w => w.type).sort().join(',') };
  })().catch(e => ({ error: e.message }))`);
  expect("fixtures", fx && fx.slug && fx.pw, fx);
  if (!fx || !fx.slug) { console.log("problems: " + JSON.stringify(problems)); proc.kill(); process.exit(1); }
  expect("every control widget can be marked", fx.marks === "button,knob,round_switch,setpoint,slider,switch", fx.marks);
  const other = client();
  await other.login(second, fx.pw);
  await other.call("POST", "/api/v1/auth/password", { current: fx.pw, new: secondPw });
  expect("the second user signs in at another computer", (await other.login(second, secondPw)) === 200);

  // ---- the switches in the security policy ----
  await go("/settings/policy");
  expect("the policy page has the new switches, all off", await ev(`['alarms','attributes','cameras','displays'].every(k => { const c = document.querySelector('#policyForm [name=four_eyes_' + k + ']'); return c && !c.checked; }) && document.querySelector('#policyForm [name=four_eyes_minutes]').value === '15'`));
  await ev(`(() => { const f = document.querySelector('#policyForm'); f.four_eyes_cameras.click(); f.four_eyes_minutes.value = '10'; f.reason.value = 'headless probe'; f.requestSubmit(); return 1; })()`);
  expect("cameras switched on, 10 minutes", await until(`fetch('/api/v1/settings/policy',{credentials:'same-origin'}).then(r=>r.json()).then(p => p.four_eyes_cameras === true && p.four_eyes_minutes === 10 && p.four_eyes_alarms === false)`));
  await ev(`(document.querySelector('#policyForm [name=four_eyes_alarms]').scrollIntoView({block:'start'}), 1)`); await sleep(300);
  await shot("four-eyes-policy.png");

  // ---- a marked control: approved on the spot ----
  await go("/dashboards/" + fx.slug);
  const card = (wid) => `document.querySelector('[data-id="${wid}"]')`;
  expect("the dashboard shows both switches", await until(`!!(${card("marked")} && ${card("marked")}.querySelector('.ctl-on') && ${card("plain")}.querySelector('.ctl-on'))`));
  await ev(`(() => { const c = ${card("plain")}; c.querySelector('.ctl-reason').value = 'air'; c.querySelector('.ctl-on').click(); return 1; })()`);
  expect("a plain control sends at once, no dialog", (await until(`!!${card("plain")}.querySelector('.ctl-out .badge:not(.warn):not(.bad)')`)) && !(await ev(dlgOpen)), await ev(`${card("plain")}.querySelector('.ctl-out').textContent`));
  await ev(`(() => { const c = ${card("marked")}; c.querySelector('.ctl-reason').value = 'start the motor'; c.querySelector('.ctl-on').click(); return 1; })()`);
  expect("the marked control opens the dialog of the second person", await until(dlgOpen));
  expect("the dialog says what is to be approved", await ev(`/run/.test(document.querySelector('#spDlg .sp-sum').textContent) && document.querySelector('#spDlg .sp-why').hidden`));
  if (lang === "sk") expect("the dialog is translated", await ev(`document.querySelector('#spDlg h2').textContent !== 'A second person is needed' && document.querySelector('#spDlg [data-sp-ok]').textContent !== 'Approve'`), await ev(`document.querySelector('#spDlg h2').textContent`));
  await fill(second, "wrong-password"); await submit();
  expect("a wrong password is refused and the dialog stays", (await until(`document.querySelector('#spDlg .sp-err').textContent.length > 3`)) && (await ev(dlgOpen)), await ev(`document.querySelector('#spDlg .sp-err').textContent`));
  await fill("admin", "admin1234"); await submit();
  expect("one cannot approve one's own action", (await until(`/differ|in|druh/i.test(document.querySelector('#spDlg .sp-err').textContent)`)) && (await ev(dlgOpen)), await ev(`document.querySelector('#spDlg .sp-err').textContent`));
  await fill(second, secondPw, `f.spnote.value = 'I stand next to the motor';`);
  await shot("second-person-dialog.png");
  await submit();
  expect("approved on the spot: the dialog closes", await until(`!${dlgOpen}`));
  expect("the control shows the command as sent, not waiting", await until(`!!${card("marked")}.querySelector('.ctl-out .badge:not(.warn):not(.bad)')`), await ev(`${card("marked")}.querySelector('.ctl-out').textContent`));
  const cmds = await ev(`ctrl32.api('GET', '/api/v1/devices/${fx.dev}/commands')`);
  const c1 = (cmds || []).find((c) => c.reason === "start the motor");
  expect("the command carries both names", c1 && c1.issued_by === "admin" && c1.approved_by === second && c1.status !== "pending", c1);

  // ---- left waiting: it is in Approvals ----
  await ev(`(() => { const c = ${card("marked")}; c.querySelector('.ctl-reason').value = 'start it again'; c.querySelector('.ctl-on').click(); return 1; })()`);
  await until(dlgOpen);
  await ev(`(document.querySelector('#spDlg [data-sp-wait]').click(), 1)`);
  expect("left waiting: the control says it waits", (await until(`!${dlgOpen}`)) && (await until(`!!${card("marked")}.querySelector('.ctl-out .badge.warn')`)), await ev(`${card("marked")}.querySelector('.ctl-out').textContent`));
  // what the second user asks for at the other computer
  const held = await other.call("POST", `/api/v1/devices/${fx.dev}/commands`, { method: "write", args: { key: "valve_v2", value: true }, reason: "open valve V2", second_person: true });
  const asked = await other.call("POST", "/api/v1/second-person/grants", { area: "cameras", reason: "incident at gate 2" });
  expect("the second user asks too", held.status === 202 && asked.status === 201, [held.status, asked.status]);
  await go("/approvals");
  const rows = () => ev(`[...document.querySelectorAll('#apWait tbody tr')].map(r => ({ by: r.cells[1].textContent.trim(), what: r.cells[2].textContent.trim(), reason: r.cells[3].textContent.trim(), ok: !!r.querySelector('[data-ok]'), no: !!r.querySelector('[data-no]') }))`);
  let rw = await rows();
  const mine = rw.find((r) => r.reason === "start it again"), theirs = rw.find((r) => r.reason === "open valve V2"), grant = rw.find((r) => r.reason === "incident at gate 2");
  expect("Approvals lists what waits", !!mine && !!theirs && !!grant, rw);
  expect("my own request cannot be approved by me, theirs can", mine && !mine.ok && theirs && theirs.ok && grant && grant.ok, [mine, theirs, grant]);
  expect("the page says where four eyes are on", await ev(`document.querySelectorAll('#apAreas .ap-area.on').length === 1`));
  await shot("approvals.png");
  await ev(`(window.prompt = () => 'agreed', [...document.querySelectorAll('#apWait tbody tr')].find(r => r.cells[3].textContent.trim() === 'open valve V2').querySelector('[data-ok]').click(), 1)`);
  expect("approved from the list: it leaves the list", await until(`![...document.querySelectorAll('#apWait tbody tr')].some(r => r.cells[3].textContent.trim() === 'open valve V2')`));
  await ev(`([...document.querySelectorAll('#apWait tbody tr')].find(r => r.cells[3].textContent.trim() === 'incident at gate 2').querySelector('[data-ok]').click(), 1)`);
  expect("the approval for cameras runs and can be ended", await until(`!document.querySelector('#apRunCard').hidden && !!document.querySelector('#apRun [data-end]')`));
  await shot("approvals-running.png");
  await ev(`(document.querySelector('#apRun [data-end]').click(), 1)`);
  expect("ended: it moves to the earlier ones", await until(`document.querySelector('#apRunCard').hidden && !document.querySelector('#apHistCard').hidden`));

  // ---- cameras: an approval for a limited time, asked for by the page ----
  const NOCAM = "00000000-0000-7000-8000-000000000000";
  await ev(`(window.__ptz = null, ctrl32.api('POST', '/api/v1/video/cameras/${NOCAM}/ptz', { action: 'stop' }).then(() => { window.__ptz = 'done'; }).catch(e => { window.__ptz = e.status + ' ' + (e.code || '') + ' ' + e.message; }), 1)`);
  expect("a camera move asks for the second person", (await until(dlgOpen)) && (await ev(`!document.querySelector('#spDlg .sp-why').hidden && !document.querySelector('#spDlg .sp-min').hidden && /10/.test(document.querySelector('#spDlg .sp-min').textContent)`)));
  await fill(second, secondPw); await submit();
  expect("without a reason nothing is asked", (await until(`document.querySelector('#spDlg .sp-err').textContent.length > 3`)) && (await ev(dlgOpen)));
  await fill(second, secondPw, `f.why.value = 'incident at gate 2';`);
  await shot("second-person-cameras.png");
  await submit();
  expect("approved: the move is sent again and reaches the camera handler", (await until(`typeof window.__ptz === 'string'`)) && /^404/.test(await ev(`window.__ptz`)), await ev(`window.__ptz`));
  expect("the next move needs no dialog", (await ev(`ctrl32.api('POST', '/api/v1/video/cameras/${NOCAM}/ptz', { action: 'stop' }).then(() => 'done').catch(e => String(e.status))`)) === "404" && !(await ev(dlgOpen)));
  expect("a recording may be opened without asking again", (await ev(`ctrl32.secondEnsure('cameras')`)) === true && !(await ev(dlgOpen)));

  // ---- phone width ----
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 2, mobile: true });
  await go("/dashboards/" + fx.slug);
  await until(`!!(${card("marked")} && ${card("marked")}.querySelector('.ctl-on'))`);
  await ev(`(() => { const c = ${card("marked")}; c.querySelector('.ctl-reason').value = 'from the phone'; c.querySelector('.ctl-on').click(); return 1; })()`);
  await until(dlgOpen);
  expect("the dialog fits a phone", await ev(`(() => { const d = document.querySelector('#spDlg'), r = d.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth + 1 && d.scrollWidth <= d.clientWidth + 1; })()`), await ev(`(() => { const d = document.querySelector('#spDlg'), r = d.getBoundingClientRect(); return [r.left, r.right, innerWidth, d.scrollWidth, d.clientWidth]; })()`));
  await shot("second-person-phone.png");
  await ev(`(document.querySelector('#spDlg [data-sp-wait]').click(), 1)`);
  await go("/approvals");
  expect("Approvals fits a phone", await ev(`document.documentElement.scrollWidth <= innerWidth + 1`), await ev(`[document.documentElement.scrollWidth, innerWidth]`));
  await send("Emulation.clearDeviceMetricsOverride");

  // ---- cleanup ----
  await ev(`(async () => { await ctrl32.api('PUT', '/api/v1/settings/policy', Object.assign(${JSON.stringify(OFF)}, { reason: 'headless end' }));
    for (const g of await ctrl32.api('GET', '/api/v1/second-person/grants')) if (g.status === 'active' || g.status === 'pending') await ctrl32.api('DELETE', '/api/v1/second-person/grants/' + g.id).catch(() => 0);
    return 1; })()`);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
