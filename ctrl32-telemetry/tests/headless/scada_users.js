// Headless Chrome: SCADA users (spec 041, 0.95.0). The administration page; the switches in the security policy; the
// groups of a control in the dashboard editor; a control that names a group opens the sign-in at the controls, refuses
// a person of another group and works for a member; the person at the controls is shown in the header; a shift leader
// approves a marked control with a PIN; the operator view; phone width.
//   BASE=http://127.0.0.1:8080 OUT=.dev/shots LANG_UI=sk DOCS=1 node tests/headless/scada_users.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const path = require("path");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const lang = process.env.LANG_UI || "";
const port = 9385;
fs.mkdirSync(out, { recursive: true });
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-scada-users", "--window-size=1440,1000", "--lang=" + (lang || "en")], { stdio: "ignore" });
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
  const until = async (expr, ms = 8000) => { for (let i = 0; i < ms / 150; i++) { if ((await ev(expr)) === true) return true; await sleep(150); } return false; };
  const go = async (url) => { await send("Page.navigate", { url: base + url }); await sleep(2200); if (process.env.DOCS) await ev(`document.querySelectorAll('.update-banner').forEach(e => e.remove())`); };
  const suf = String(Math.floor(Math.random() * 90000) + 10000);
  const OFF = { four_eyes_commands: false, scada_users_required: false, scada_idle_minutes: 10, operator_log_days: 0, log_operator_actions: true };
  const card = (wid) => `document.querySelector('[data-id="${wid}"]')`;
  const scOpen = `!!(document.querySelector('#scadaDlg') && document.querySelector('#scadaDlg').open)`;
  const spOpen = `!!(document.querySelector('#spDlg') && document.querySelector('#spDlg').open)`;
  const chip = `(document.querySelector('#scadaChip') || { textContent: '' }).textContent.trim()`;
  const chipSeen = `(() => { const c = document.querySelector('#scadaChip'); if (!c) return false; const r = c.getBoundingClientRect(); return r.width > 20 && r.height > 10 && r.left >= 0 && r.right <= innerWidth + 1; })()`;
  const press = (wid, reason) => ev(`(() => { const c = ${card(wid)}; c.querySelector('.ctl-reason').value = ${JSON.stringify(reason)}; c.querySelector('.ctl-on').click(); return 1; })()`);
  const sent = (wid) => `!!${card(wid)}.querySelector('.ctl-out .badge:not(.warn):not(.bad)')`;
  // type a PIN on the pad of the dialog, as on a touch screen
  const pad = (digits) => ev(`(() => { for (const k of ${JSON.stringify(digits)}) document.querySelector('#scadaDlg .sc-pad [data-k="' + k + '"]').click(); return document.querySelector('#scadaDlg form').scsecret.value; })()`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", (await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`)) === 200);
  await ev(`(${lang ? `localStorage.setItem('ctrl32.locale','${lang}')` : `localStorage.removeItem('ctrl32.locale')`}, localStorage.removeItem('ctrl32.scadaNames'), 1)`);
  await go("/users/scada");

  // ---- fixtures: SCADA users of three groups, a device, a dashboard with three switches ----
  const fx = await ev(`(async () => {
    await ctrl32.api('PUT', '/api/v1/settings/policy', Object.assign(${JSON.stringify(OFF)}, { reason: 'headless start' }));
    await ctrl32.api('DELETE', '/api/v1/scada/session').catch(() => 0);
    for (const u of await ctrl32.api('GET', '/api/v1/scada/users')) if (['jana', 'peter', 'veduci'].includes(u.name)) await ctrl32.api('DELETE', '/api/v1/scada/users/' + u.id + '?reason=headless');
    await ctrl32.api('POST', '/api/v1/scada/users', { name: 'peter', full_name: 'Peter Kováč', secret_kind: 'password', secret: 'Peter-heslo-1', groups: ['technologist'], reason: 'headless probe' });
    await ctrl32.api('POST', '/api/v1/scada/users', { name: 'veduci', full_name: 'Milan Horváth', secret_kind: 'pin', secret: '4321', groups: ['shift_leader', 'operator'], reason: 'headless probe' });
    const dev = await ctrl32.api('POST', '/api/v1/devices', { external_id: 'scu-${suf}', name: 'Mixer M1', reason: 'headless probe' });
    const sw = (id, x, title, key, extra) => ({ id, type: 'switch', geometry: { x, y: 0, w: 4, h: 3 }, bindings: { devices: [dev.id] }, params: Object.assign({ title, key, confirm: false }, extra) });
    const d = await ctrl32.api('POST', '/api/v1/dashboards', { name: 'Mixer M1 ${suf}', spec: { version: 1, range: '1h', refresh_s: 30, widgets: [sw('plain', 0, 'Lighting', 'light', {}), sw('tech', 4, 'Dosing pump', 'dosing', { scada_groups: ['technologist'] }), sw('two', 8, 'Main motor', 'motor', { second_person: true }), { id: 'who', type: 'scada_user', geometry: { x: 0, y: 3, w: 4, h: 3 }, bindings: {}, params: { title: 'Person at the controls' } }] }, reason: 'headless probe' });
    const pic = await ctrl32.api('POST', '/api/v1/dashboards', { name: 'Valve picture ${suf}', spec: { kind: 'mimic', widgets: [], elements: [{ id: 'v1', type: 'rect', name: 'Drain valve', x: 100, y: 100, w: 200, h: 120, device: dev.id, key: 'drain', scada_groups: ['maintenance'], on: [{ act: 'command', key: 'drain', value: 1 }] }] }, reason: 'headless probe' });
    const cat = await ctrl32.api('GET', '/api/v1/dashboards/catalog');
    return { dev: dev.id, slug: d.slug, pic: pic.slug, groups: cat.widgets.filter(w => (w.params || []).some(p => p.name === 'scada_groups')).map(w => w.type).sort().join(',') };
  })()`);
  if (!fx || !fx.slug) { console.log("fixtures: " + JSON.stringify(fx) + " problems: " + JSON.stringify(problems)); proc.kill(); process.exit(1); }
  expect("every control widget can name groups", fx.groups === "button,knob,round_switch,setpoint,slider,switch", fx.groups);

  // ---- administration ----
  await go("/users/scada");
  expect("the page lists the four default groups", await until(`['operator','shift_leader','technologist','maintenance'].every(k => [...document.querySelectorAll('#suGroups tbody code')].some(c => c.textContent === k))`));
  expect("and the people", await until(`[...document.querySelectorAll('#suUsers tbody b')].map(b => b.textContent).join(',').includes('peter')`));
  await ev(`(document.querySelector('#newUser').click(), 1)`);
  expect("the dialog of a new SCADA user", await until(`document.querySelector('#userDlg').open`));
  await ev(`(() => { const f = document.querySelector('#userDlg form'); f.name.value = 'jana'; f.full_name.value = 'Jana Nováková'; f.secret_kind.value = 'pin'; f.secret.value = '12'; f.querySelector('input[name=grp][value=operator]').checked = true; f.reason.value = 'new operator'; f.requestSubmit(); return 1; })()`);
  expect("a PIN of two digits is refused in the dialog", await until(`document.querySelector('#userErr').textContent.length > 3`), await ev(`document.querySelector('#userErr').textContent`));
  await ev(`(() => { const f = document.querySelector('#userDlg form'); f.secret.value = '1234'; f.requestSubmit(); return 1; })()`);
  expect("saved: Jana is in the list with her group", await until(`[...document.querySelectorAll('#suUsers tbody tr')].some(r => r.cells[0].textContent.trim() === 'jana' && r.cells[2].textContent.trim() === 'PIN' && r.cells[3].textContent.trim().length > 3)`), await ev(`document.querySelector('#suUsers tbody').innerText`));
  await ev(`window.scrollTo(0, 0)`); await shot("scada-users.png");
  await ev(`([...document.querySelectorAll('#suUsers tbody tr')].find(r => r.cells[0].textContent.trim() === 'jana').click(), 1)`);
  expect("a click opens the person; the name cannot be changed", await until(`document.querySelector('#userDlg').open && document.querySelector('#userDlg form').name.readOnly && !document.querySelector('#userDelete').hidden`));
  await shot("scada-user-dialog.png");
  await ev(`(document.querySelector('#userDlg [data-close]').click(), 1)`);

  // ---- the security policy ----
  await go("/settings/policy");
  expect("the policy has the switch, the time and the retention, all at their defaults", await until(`(() => { const f = document.querySelector('#policyForm'); return !!f && f.scada_users_required && !f.scada_users_required.checked && f.scada_idle_minutes.value === '10' && f.operator_log_days.value === '0'; })()`));
  await ev(`(document.querySelector('#policyForm [name=scada_users_required]').scrollIntoView({ block: 'center' }), 1)`); await sleep(300); await shot("scada-policy.png");

  // ---- the editor: the groups of a control ----
  await go("/dashboards/" + fx.slug + "/edit");
  await until(`!!document.querySelector('#eGrid [data-id="tech"]')`);
  await ev(`(select(SPEC.widgets.findIndex(w => w.id === 'tech')), 1)`);
  expect("the editor offers the groups; Technologist is ticked", await until(`[...document.querySelectorAll('[data-sg]')].length >= 4 && [...document.querySelectorAll('[data-sg]:checked')].map(x => x.value).join(',') === 'technologist'`), await ev(`[...document.querySelectorAll('[data-sg]')].map(x => x.value + ':' + x.checked).join(' ')`));
  await ev(`(document.querySelector('[data-sg]').closest('fieldset').scrollIntoView({ block: 'center' }), 1)`); await sleep(300); await shot("scada-control-groups.png");

  // ---- at the controls ----
  await go("/dashboards/" + fx.slug);
  expect("the dashboard shows the three switches", await until(`!!(${card("plain")} && ${card("plain")}.querySelector('.ctl-on') && ${card("tech")}.querySelector('.ctl-on') && ${card("two")}.querySelector('.ctl-on'))`));
  expect("the page offers the sign-in at the controls, where it is seen", (await until(`!!document.querySelector('#scadaChip [data-scada-in]')`)) && (await until(chipSeen)), await ev(chip));
  await press("plain", "light on");
  expect("a control without groups sends at once", (await until(sent("plain"))) && !(await ev(scOpen)), await ev(`${card("plain")}.querySelector('.ctl-out').textContent`));
  await press("tech", "start dosing");
  expect("a control with a group opens the sign-in", await until(scOpen));
  expect("it says whom the control is for", await until(`/Technologist|Technológ/.test(document.querySelector('#scadaDlg .sc-why').textContent)`), await ev(`document.querySelector('#scadaDlg .sc-why').textContent`));
  await ev(`(document.querySelector('#scadaDlg form').scname.value = 'jana', 1)`);
  expect("the PIN pad types and deletes", (await pad(["1", "2", "9", "⌫", "3", "4"])) === "1234");
  await shot("scada-sign-in.png");
  await ev(`(document.querySelector('#scadaDlg form').requestSubmit(), 1)`);
  expect("Jana is signed in, but her group may not: the dialog asks for somebody else", await until(`${scOpen} && ${chip}.includes('jana') && document.querySelector('#scadaDlg .sc-why').textContent.length > 20 && document.querySelector('#scadaDlg form').scsecret.value === ''`, 10000), await ev(`[${chip}, document.querySelector('#scadaDlg .sc-why').textContent]`));
  await ev(`(() => { const f = document.querySelector('#scadaDlg form'); f.scname.value = 'peter'; f.scsecret.value = 'wrong-one'; f.requestSubmit(); return 1; })()`);
  expect("a wrong password stays in the dialog", (await until(`document.querySelector('#scadaDlg .sp-err').textContent.length > 3`)) && (await ev(scOpen)));
  await ev(`(() => { const f = document.querySelector('#scadaDlg form'); f.scname.value = 'peter'; f.scsecret.value = 'Peter-heslo-1'; f.requestSubmit(); return 1; })()`);
  expect("the technologist signs in and the command goes", (await until(`!${scOpen}`)) && (await until(sent("tech"))), await ev(`${card("tech")}.querySelector('.ctl-out').textContent`));
  expect("the header shows who is at the controls", await until(`${chip}.includes('peter') && !!document.querySelector('#scadaChip [data-scada-out]')`), await ev(chip));
  let cmds = await ev(`ctrl32.api('GET', '/api/v1/devices/${fx.dev}/commands')`);
  const c1 = (cmds || []).find((c) => c.reason === "start dosing");
  expect("the command names the station and the person", !!c1 && c1.issued_by === "admin" && c1.operator === "peter", c1);

  // ---- four eyes with a PIN ----
  await press("two", "start the motor");
  expect("a marked control opens the dialog of the second person", await until(spOpen));
  expect("which offers the PIN of the controls", await ev(`!document.querySelector('#spDlg .sp-scada').hidden`));
  await ev(`(() => { const f = document.querySelector('#spDlg form'); f.spscada.click(); f.spuser.value = 'veduci'; f.sppass.value = '4321'; return 1; })()`);
  await shot("scada-second-person.png");
  await ev(`(document.querySelector('#spDlg form').requestSubmit(), 1)`);
  expect("the shift leader approves with his PIN and the command goes", (await until(`!${spOpen}`)) && (await until(sent("two"))), await ev(`[document.querySelector('#spDlg .sp-err').textContent, ${card("two")}.querySelector('.ctl-out').textContent]`));
  cmds = await ev(`ctrl32.api('GET', '/api/v1/devices/${fx.dev}/commands')`);
  const c2 = (cmds || []).find((c) => c.reason === "start the motor");
  expect("the record carries both people", !!c2 && c2.operator === "peter" && c2.approved_by === "admin / veduci" && c2.status !== "pending", c2);
  await sleep(600); await ev(`window.scrollTo(0, 0)`); await shot("scada-at-the-controls.png");

  // ---- an element of a process picture with a group ----
  await go("/dashboards/" + fx.pic);
  expect("the picture draws the element", await until(`!!document.querySelector('[data-eid="v1"]')`));
  await ev(`(window.confirm = () => true, document.querySelector('[data-eid="v1"]').dispatchEvent(new MouseEvent('click', { bubbles: true })), 1)`);
  expect("a click opens its command", await until(`!!document.querySelector('dialog.sc-dlg[open] form')`), await ev(`(document.querySelector('dialog.sc-dlg') || { outerHTML: 'no dialog' }).outerHTML.slice(0, 200)`));
  await ev(`(() => { const f = document.querySelector('dialog.sc-dlg[open] form'); f.reason.value = 'drain the tank'; f.requestSubmit(); return 1; })()`);
  expect("the technologist is not in the group of the element: the sign-in asks for Maintenance", (await until(scOpen)) && (await until(`/Maintenance|Údržba/.test(document.querySelector('#scadaDlg .sc-why').textContent)`)), await ev(`document.querySelector('#scadaDlg') && document.querySelector('#scadaDlg .sc-why').textContent`));
  await ev(`(document.querySelector('#scadaDlg [data-sc-cancel]').click(), 1)`);
  expect("cancelled: the window says the group may not", await until(`!${scOpen} && !!document.querySelector('dialog.sc-dlg[open] .sc-out .badge.bad')`), await ev(`document.querySelector('dialog.sc-dlg[open] .sc-out') && document.querySelector('dialog.sc-dlg[open] .sc-out').textContent`));
  await go("/dashboards/" + fx.slug);
  await until(`!!document.querySelector('#scadaChip [data-scada-out]')`);

  // ---- the organization requires a person at the controls ----
  await ev(`(document.querySelector('#scadaChip [data-scada-out]').click(), 1)`);
  expect("signing out in the header", await until(`!!document.querySelector('#scadaChip [data-scada-in]')`), await ev(chip));
  await ev(`ctrl32.api('PUT', '/api/v1/settings/policy', { scada_users_required: true, reason: 'headless probe' }).then(() => 1)`);
  await press("plain", "light off");
  expect("required: also a control without groups asks who is there", await until(scOpen));
  await ev(`(document.querySelector('#scadaDlg [data-sc-cancel]').click(), 1)`);
  expect("cancelled: nothing is sent and the control says why", (await until(`!${scOpen}`)) && (await until(`!!${card("plain")}.querySelector('.ctl-out .badge.bad')`)), await ev(`${card("plain")}.querySelector('.ctl-out').textContent`));

  // ---- the operator view ----
  await go("/operator/" + fx.slug);
  expect("the operator view shows the sign-in in its frame", await until(`!!document.querySelector('.op-head #scadaChip [data-scada-in]')`), await ev(`document.querySelector('.op-head') ? document.querySelector('.op-head').innerText : 'no frame'`));
  await ev(`(document.querySelector('#scadaChip [data-scada-in]').click(), 1)`);
  expect("the chip opens the sign-in; names used before are offered", (await until(scOpen)) && (await ev(`[...document.querySelectorAll('#scadaDlg .sc-names [data-n]')].map(b => b.dataset.n).join(',')`)).includes("peter"));
  await ev(`(() => { const f = document.querySelector('#scadaDlg form'); f.scname.value = 'veduci'; return 1; })()`); await pad(["4", "3", "2", "1"]);
  await ev(`(document.querySelector('#scadaDlg form').requestSubmit(), 1)`);
  expect("signed in from the frame", (await until(`!${scOpen}`)) && (await until(`${chip}.includes('veduci')`)) && (await ev(chipSeen)), await ev(chip));
  await until(`!!(${card("plain")} && ${card("plain")}.querySelector('.ctl-on'))`);
  await press("plain", "light on again");
  expect("and the control works", await until(sent("plain")), await ev(`${card("plain")}.querySelector('.ctl-out').textContent`));
  await sleep(500); await shot("scada-operator-view.png");

  // ---- the operator log ----
  await go("/oplog");
  expect("the operator log names the people at the controls", await until(`(() => { const t = document.querySelector('#olTable tbody').innerText; return t.includes('peter (admin)') && t.includes('veduci (admin)'); })()`), (await ev(`document.querySelector('#olTable tbody').innerText`) || "").slice(0, 300));
  await shot("scada-operator-log.png");

  // ---- the widget Person at the controls (spec 044) ----
  await ev(`ctrl32.api('DELETE', '/api/v1/scada/session').then(() => 1)`);
  await go("/dashboards/" + fx.slug);
  expect("the widget offers the sign-in when nobody is at the controls", await until(`!!(${card("who")} && ${card("who")}.querySelector('[data-in]'))`));
  expect("with the names used on this computer", (await ev(`[...${card("who")}.querySelectorAll("[data-n]")].map(b => b.dataset.n).sort().join(",")`)).includes("peter,veduci"), await ev(`${card("who")}.innerText`));
  await shot("scada-widget-out.png");
  await ev(`(${card("who")}.querySelector('[data-n="veduci"]').click(), 1)`);
  expect("a name opens the sign-in with the name filled in", (await until(scOpen)) && (await ev(`document.querySelector('#scadaDlg form').scname.value`)) === "veduci");
  await pad("4321"); await ev(`(document.querySelector('#scadaDlg .sc-pad [data-k="OK"]').click(), 1)`);
  expect("the widget shows who signed in", await until(`!!${card("who")}.querySelector('[data-out]') && ${card("who")}.querySelector('.scw-who b').textContent === 'Milan Horváth'`), await ev(`${card("who")}.innerText`));
  expect("the groups of the person", (await ev(`${card("who")}.querySelectorAll('.scw-groups .badge').length`)) === 2 && (await until(`![...${card("who")}.querySelectorAll('.scw-groups .badge')].some(b => b.textContent.includes('_'))`)), await ev(`${card("who")}.querySelector('.scw-groups').innerText`));
  expect("and the time left, counting down", /^\d+:\d\d$/.test(await ev(`${card("who")}.querySelector('[data-left]').textContent`)) && (await (async () => { const a = await ev(`${card("who")}.querySelector('[data-left]').textContent`); await sleep(2300); return a !== (await ev(`${card("who")}.querySelector('[data-left]').textContent`)); })()));
  expect("the header shows the same person", (await ev(chip)).includes("veduci"));
  await shot("scada-widget-in.png");
  await ev(`(${card("who")}.querySelector('[data-out]').click(), 1)`);
  expect("the widget signs the person out", (await until(`!!${card("who")}.querySelector('[data-in]')`)) && (await ev(`ctrl32.api('GET', '/api/v1/scada/session').then(s => !s.user)`)) === true);
  await ev(`(${card("who")}.querySelector('[data-in]').click(), 1)`);
  expect("the large button opens the sign-in", await until(scOpen));
  await ev(`(document.querySelector('#scadaDlg [data-sc-cancel]').click(), 1)`);

  // ---- phone width ----
  await ev(`ctrl32.api('DELETE', '/api/v1/scada/session').then(() => 1)`);
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 800, deviceScaleFactor: 2, mobile: true });
  await go("/dashboards/" + fx.slug);
  await until(`!!(${card("tech")} && ${card("tech")}.querySelector('.ctl-on'))`);
  await press("tech", "from the phone");
  expect("on a phone the sign-in fits the screen", (await until(scOpen)) && (await ev(`(() => { const r = document.querySelector('#scadaDlg').getBoundingClientRect(); return r.left >= 0 && r.right <= 391 && document.documentElement.scrollWidth <= 391; })()`)), await ev(`JSON.stringify(document.querySelector('#scadaDlg').getBoundingClientRect())`));
  await shot("scada-sign-in-phone.png");
  await ev(`(document.querySelector('#scadaDlg [data-sc-cancel]').click(), 1)`);
  expect("on a phone the sign-in is offered in the header", await until(chipSeen), await ev(`JSON.stringify(document.querySelector('#scadaChip') && document.querySelector('#scadaChip').getBoundingClientRect())`));
  await send("Emulation.clearDeviceMetricsOverride");

  await ev(`(async () => { await ctrl32.api('DELETE', '/api/v1/scada/session').catch(() => 0); await ctrl32.api('PUT', '/api/v1/settings/policy', Object.assign(${JSON.stringify(OFF)}, { log_operator_actions: false, reason: 'headless end' })); return 1; })()`);
  expect("no script errors", problems.length === 0, problems);
  console.log("problems: " + (problems.length ? JSON.stringify(problems) : "none"));
  console.log(failed ? `FAILED: ${failed}` : "ALL OK");
  proc.kill(); process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
