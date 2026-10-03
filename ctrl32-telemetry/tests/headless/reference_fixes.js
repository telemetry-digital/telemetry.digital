// Headless Chrome: fixes found while writing the reference documentation (0.63.8).
//   - the video player's "Save picture" burns the privacy masks into the picture (a stand-in WebSocket feeds the
//     player a masks message and a white JPEG; the left half must come out black)
//   - New alarm rule: the escalation policy list is real options, every evaluated type is offered
//     (sensor_fault since 0.67.0); the datastream's "New rule version" keeps the
//     current escalation policy
//   - the dashboard editor gives a Pipe no title setting; Setpoint refuses a typed value outside min–max; Knob and
//     Slider show the datastream's unit when their unit is empty; the Alarms widget has an Acknowledge column
// Screenshots fixes-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9381;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-fixes", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
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
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: true, promptText: "headless probe" });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2500) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);

  // ---- 1. Save picture keeps the privacy masks
  await go("/video", 2500);
  const snap = await ev(`(async () => {
    if (!window.ctrl32Player) await new Promise((r) => { const s = document.createElement('script'); s.src = '/videoplayer.js'; s.onload = r; document.head.appendChild(s); });
    const c = document.createElement('canvas'); c.width = 160; c.height = 90; const g = c.getContext('2d'); g.fillStyle = '#fff'; g.fillRect(0, 0, 160, 90);
    const jpeg = await new Promise((r) => c.toBlob((b) => b.arrayBuffer().then(r), 'image/jpeg', 0.95));
    const Real = window.WebSocket;
    const run = async (masks, unmasked) => {
      window.WebSocket = class { constructor() { this.readyState = 1; setTimeout(() => { const m = (d) => this.onmessage && this.onmessage({ data: d });
        m(JSON.stringify({ type: 'masks', masks, unmasked })); m(JSON.stringify({ type: 'init', codec: 'mjpeg', width: 160, height: 90 })); m(jpeg.slice(0)); }, 50); } close() {} send() {} };
      const host = document.createElement('div'); host.style.cssText = 'width:320px;height:180px'; document.body.appendChild(host);
      const p = window.ctrl32Player(host, { url: '/api/v1/video/cameras/x/live', live: true });
      await new Promise((r) => setTimeout(r, 600));
      const s = p.snapshot(); const px = (x, y) => Array.from(s.getContext('2d').getImageData(x, y, 1, 1).data.slice(0, 3)).join(',');
      const res = { left: px(20, 45), right: px(140, 45), unmasked: p.unmasked };
      p.close(); host.remove(); return res;
    };
    try { return { masked: await run([[[0, 0], [0.5, 0], [0.5, 1], [0, 1]]], false), plain: await run([], true) }; } finally { window.WebSocket = Real; }
  })()`);
  const dark = (s) => typeof s === "string" && s.split(",").every((v) => +v < 30), light = (s) => typeof s === "string" && s.split(",").every((v) => +v > 225);
  expect("saved picture: masked half is black", snap && snap.masked && snap.masked.left, dark);
  expect("saved picture: the rest is the picture", snap && snap.masked && snap.masked.right, light);
  expect("player reports a masked stream", snap && snap.masked && snap.masked.unmasked, false);
  expect("unmasked stream (video.unmask): picture as is, flagged", snap && snap.plain && [light(snap.plain.left), snap.plain.unmasked].join(), "true,true");

  // ---- 2. alarm rule forms
  await go("/assets/rules", 3000);
  await ev(`document.querySelector('#newRuleBtn').click()`);
  expect("new rule dialog open", await until(`document.querySelector('#ruleDlg').open`), true);
  const dlg = await ev(`(() => { const f = document.querySelector('#ruleNew'); return { types: [...f.type.options].map(o => o.value).join(), pol: [...f.policy.options].map(o => o.textContent).join('|'), html: f.innerHTML.includes('\${POLS') }; })()`);
  expect("types offered (the evaluated ones, sensor_fault since 0.67)", dlg && dlg.types, "high,low,low_battery,power_loss,door_open,comm_loss,sensor_fault");
  expect("escalation policy: real options, no template text", dlg && !dlg.html && !dlg.pol.includes("POLS") && dlg.pol.length > 0, true);
  await ev(`(() => { const f = document.querySelector('#ruleNew'); f.type.value = 'door_open'; f.type.dispatchEvent(new Event('change')); })()`);
  expect("door_open: limits off, hint shown", await ev(`(() => { const f = document.querySelector('#ruleNew'); return f.warning.disabled && f.action.disabled && f.querySelector('.rule-hint').textContent.length > 10; })()`), true);
  await shot("fixes-new-alarm-rule.png");
  await ev(`document.querySelector('#ruleDlg').close()`);
  // a rule with a policy; the datastream's "New rule version" keeps it
  const setup = await ev(`(async () => {
    const api = ctrl32.api, sfx = Date.now() % 100000;
    const ds = (await api('GET', '/api/v1/datastreams')).find(d => d.kind === 'gauge' && d.unit) || (await api('GET', '/api/v1/datastreams'))[0];
    const ch = await api('POST', '/api/v1/notification-channels', { name: 'probe-mail-' + sfx, kind: 'email', settings: { recipients: ['ops@example.com'] }, reason: 'headless probe' });
    const pol = await api('POST', '/api/v1/escalation-policies', { name: 'probe-pol-' + sfx, steps: [{ after_s: 0, channel_id: ch.id }], reason: 'headless probe' });
    const rule = await api('POST', '/api/v1/alarm-rules', { datastream_id: ds.id, type: 'high', warning: 1e6, action: 2e6, escalation_policy_id: pol.id, reason: 'headless probe' });
    return { ds: ds.id, unit: ds.unit, ch: ch.id, pol: pol.id, rule: rule.id, dev: ((await api('GET', '/api/v1/devices')).find(d => !d.revoked_at) || {}).id };
  })()`);
  expect("rule with an escalation policy", !!(setup && setup.rule), true);
  await ev(`openDS(${JSON.stringify(setup.ds)})`);
  expect("datastream detail with the rule form", await until(`!!document.querySelector('#ruleForm')`), true);
  const rf = await ev(`(() => { const f = document.querySelector('#ruleForm'); f.type.value = 'high'; f.type.dispatchEvent(new Event('change')); return { pol: f.policy.value, warn: f.warning.value }; })()`);
  expect("New rule version keeps the escalation policy", rf && rf.pol, setup.pol);
  expect("New rule version starts from the current limits", rf && rf.warn, "1000000");
  await shot("fixes-rule-version.png");
  await ev(`document.querySelector('#dsDlg').close()`);

  // ---- 3. widgets
  const spec = { version: 1, kind: "dashboard", layout: { type: "grid", row_height: 96 }, range: "24h", refresh_s: 60, widgets: [
    { id: "sp", type: "setpoint", geometry: { x: 0, y: 0, w: 4, h: 2 }, bindings: { devices: setup.dev ? [setup.dev] : [], datastreams: [] }, params: { title: "Setpoint", key: "sp", min: 0, max: 10, confirm: false } },
    { id: "kn", type: "knob", geometry: { x: 4, y: 0, w: 3, h: 3 }, bindings: { devices: setup.dev ? [setup.dev] : [], datastreams: [setup.ds] }, params: { title: "Knob", key: "k" } },
    { id: "al", type: "alarms", geometry: { x: 7, y: 0, w: 5, h: 3 }, bindings: {}, params: { title: "Alarms" } }] };
  const slug = await ev(`ctrl32.api('POST','/api/v1/dashboards',{name:'fixes-probe-${Date.now() % 100000}',spec:${JSON.stringify(spec)},reason:'headless probe'}).then(r => r.slug)`);
  expect("probe dashboard", typeof slug === "string", true);
  await go("/dashboards/" + slug, 4000);
  if (setup.dev) {
    expect("setpoint rendered", await until(`!!document.querySelector('.ctl-value')`), true);
    await ev(`(() => { const el = document.querySelector('.ctl-value').closest('.dw-body') || document; document.querySelector('.ctl-value').value = '50'; document.querySelector('.ctl-reason').value = 'headless probe'; document.querySelector('.ctl-send').click(); })()`);
    expect("setpoint: 50 outside 0–10 refused", await until(`(document.querySelector('.ctl-out') || {}).textContent.length > 0`), true);
    expect("setpoint message", await ev(`document.querySelector('.ctl-out').textContent`), (s) => typeof s === "string" && s.includes("0 – 10"));
    expect("knob shows the datastream's unit", await until(`(document.querySelector('.dw-kv2') || {}).textContent.trim().endsWith(${JSON.stringify(setup.unit || "")})`), true);
  } else console.log("skip setpoint and knob: no device");
  expect("alarms widget: Acknowledge column", await until(`(() => { const t = [...document.querySelectorAll('table')].find(t => t.querySelector('th') && t.querySelector('th').textContent.length); return !!document.querySelector('.dw-body, body') && [...document.querySelectorAll('table thead tr')].some(r => r.children.length === 6); })()`), true);
  await shot("fixes-widgets.png");
  // the editor: a new Pipe has no title setting (it has none)
  await go("/dashboards/" + slug + "/edit", 4000);
  const pipe = await ev(`(() => { addWidget('pipe'); const w = SPEC.widgets[SPEC.widgets.length - 1]; return { type: w.type, title: 'title' in w.params }; })()`);
  expect("new Pipe without a title setting", pipe && [pipe.type, pipe.title].join(), "pipe,false");
  const inst = await ev(`(() => { addWidget('instrument'); return 'title' in SPEC.widgets[SPEC.widgets.length - 1].params; })()`);
  expect("new Instrument without a title setting", inst, false);
  const val = await ev(`(() => { addWidget('value'); return SPEC.widgets[SPEC.widgets.length - 1].params.title; })()`);
  expect("a widget with a title still gets one", typeof val === "string" && val.length > 0, true);

  // ---- cleanup
  expect("cleanup", await ev(`(async () => { const api = ctrl32.api; await api('DELETE', '/api/v1/dashboards/${slug}?reason=headless%20probe'); await api('DELETE', '/api/v1/alarm-rules/${setup.rule}?reason=headless%20probe'); await api('DELETE', '/api/v1/escalation-policies/${setup.pol}?reason=headless%20probe'); await api('DELETE', '/api/v1/notification-channels/${setup.ch}?reason=headless%20probe'); return true; })()`), true);
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
