// Headless Chrome: the SCADA picture editor (spec 001, US2/US3) driven with real mouse and keyboard input — place
// symbols from the library, draw a pipe, rubber-band select, align, undo/redo, copy/paste, drag, delete, bind a tag
// and add a dynamic through the property panel, save with a reason, reload and compare. Mismatches fail the probe.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9362;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile22", "--window-size=1700,1050", "about:blank"], { stdio: "ignore" });
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
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : JSON.stringify(got) === JSON.stringify(want); console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const mouse = async (type, x, y, extra) => send("Input.dispatchMouseEvent", Object.assign({ type, x, y, button: "left", buttons: type === "mouseReleased" ? 0 : 1, clickCount: 1, pointerType: "mouse" }, extra || {}));
  const click = async (x, y, mods) => { await mouse("mouseMoved", x, y, { buttons: 0, modifiers: mods || 0 }); await mouse("mousePressed", x, y, { modifiers: mods || 0 }); await mouse("mouseReleased", x, y, { modifiers: mods || 0 }); await sleep(150); };
  const dragTo = async (x0, y0, x1, y1, mods) => { await mouse("mouseMoved", x0, y0, { buttons: 0 }); await mouse("mousePressed", x0, y0, { modifiers: mods || 0 }); for (let k = 1; k <= 8; k++) await mouse("mouseMoved", x0 + (x1 - x0) * k / 8, y0 + (y1 - y0) * k / 8, { modifiers: mods || 0 }); await mouse("mouseReleased", x1, y1, { modifiers: mods || 0 }); await sleep(200); };
  const key = async (k, code, mods, vk) => { await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, modifiers: mods || 0, windowsVirtualKeyCode: vk }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, modifiers: mods || 0, windowsVirtualKeyCode: vk }); await sleep(150); };
  const CTRL = 2, SHIFT = 8;
  // Canvas point → screen coordinates.
  const at = async (x, y) => ev(`(() => { const r = document.querySelector('#scadaEd .mimic').getBoundingClientRect(); const z = window.__scadaEditor._debug.Z; return [r.left + ${x} * z, r.top + ${y} * z]; })()`);
  const center = async (sel) => ev(`(() => { const r = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()`);
  const elements = () => ev(`SPEC.elements.map(e => e.id + ':' + e.type + (e.symbol ? '/' + e.symbol : ''))`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  const slug = await ev(`(async () => { const me = await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json(); const h = {'Content-Type':'application/json','X-CSRF-Token': me.csrf_token};
    const spec = {version:1, kind:'mimic', layout:{type:'canvas', width:1200, height:700, bg:'#dfe3e8', grid:10}, widgets:[], elements:[{id:'title', type:'text', x:20, y:20, w:300, h:30, text:'Editor probe', style:{font:18, bold:true, color:'#111827'}}]};
    const r = await (await fetch('/api/v1/dashboards',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({name:'Editor probe ' + Math.floor(Math.random()*1e6), spec, reason:'probe'})})).json(); return r.slug; })()`);
  expect("picture created", typeof slug === "string" && slug.length > 0, true);
  await send("Page.navigate", { url: base + "/dashboards/" + slug + "/edit" }); await sleep(4500);
  expect("editor mounted", await ev(`!!window.__scadaEditor && document.querySelectorAll('#scadaEd .se-sym').length >= 50`), true);
  await ev(`window.__scadaEditor._debug.Z && (document.querySelector('#scadaEd [data-cmd=fit]').click(), true)`); await sleep(300);

  // Place a tank and a pump from the library (click tile, click canvas).
  let [x, y] = await center('#scadaEd .se-sym[data-sym="tank_vertical"]'); await click(x, y);
  [x, y] = await at(300, 300); await click(x, y);
  [x, y] = await center('#scadaEd .se-sym[data-sym="pump_centrifugal"]'); await click(x, y);
  [x, y] = await at(700, 400); await click(x, y);
  expect("two symbols placed", await ev(`SPEC.elements.filter(e => e.type === 'symbol').map(e => e.symbol).sort().join(',')`), "pump_centrifugal,tank_vertical");
  expect("placed snapped to grid", await ev(`SPEC.elements.filter(e => e.type === 'symbol').every(e => e.x % 10 === 0 && e.y % 10 === 0)`), true);

  // Draw an orthogonal pipe with three clicks and Enter.
  [x, y] = await center('#scadaEd .se-tool[data-tool="pipe"]'); await click(x, y);
  for (const [px, py] of [[370, 330], [562, 336], [566, 430]]) { [x, y] = await at(px, py); await click(x, y); }
  await key("Enter", "Enter", 0, 13);
  expect("pipe drawn orthogonally", await ev(`JSON.stringify((SPEC.elements.find(e => e.type === 'pipe') || {}).points)`), "[[370,330],[560,330],[560,430]]");

  // Rubber band around everything except the title, then align top, undo, redo (the band stays clear of the tank's edge at y = 200, whatever the zoom).
  [x, y] = await at(5, 5); await click(x, y); // deselect (empty area)
  const [bx0, by0] = await at(190, 190), [bx1, by1] = await at(1000, 610); await dragTo(bx0, by0, bx1, by1);
  expect("rubber band selects 3", await ev(`window.__scadaEditor.selection().length`), 3);
  [x, y] = await center('#scadaEd [data-cmd="at"]'); await click(x, y);
  const tops = () => ev(`[...new Set(window.__scadaEditor.selection().map(k => Math.round(ctrl32Scada.bbox(SPEC.elements.find(e => 'e:' + e.id === k)).y)))].length`);
  expect("align top → one top edge", await tops(), 1);
  await key("z", "KeyZ", CTRL, 90); expect("undo → edges differ", await tops(), (v) => v > 1);
  await key("y", "KeyY", CTRL, 89); expect("redo → one edge", await tops(), 1);

  // Select the tank, copy, paste, drag the copy by 100 px, delete it.
  const tankId = await ev(`SPEC.elements.find(e => e.symbol === 'tank_vertical').id`);
  [x, y] = await center(`#scadaEd [data-eid="${tankId}"] svg`); await click(x, y);
  expect("click selects the tank", await ev(`window.__scadaEditor.selection()`), ["e:" + tankId]);
  await key("c", "KeyC", CTRL, 67); await key("v", "KeyV", CTRL, 86);
  const copyId = await ev(`window.__scadaEditor.selection()[0].slice(2)`);
  expect("paste creates a new id", copyId !== tankId && (await ev(`SPEC.elements.filter(e => e.symbol === 'tank_vertical').length`)) === 2, true);
  const before = await ev(`SPEC.elements.find(e => e.id === '${copyId}').x`);
  const [cx, cy] = await center(`#scadaEd [data-eid="${copyId}"] svg`); const z = await ev(`window.__scadaEditor._debug.Z`);
  await dragTo(cx, cy, cx + 100 * z, cy);
  expect("drag moves by 100 px", Math.round((await ev(`SPEC.elements.find(e => e.id === '${copyId}').x`)) - before), 100);
  await key("Delete", "Delete", 0, 46);
  expect("delete removes the copy", await ev(`SPEC.elements.filter(e => e.symbol === 'tank_vertical').length`), 1);

  // Bind a tag and add a level dynamic through the property panel.
  [x, y] = await center(`#scadaEd [data-eid="${tankId}"] svg`); await click(x, y);
  expect("binding set through the panel", await ev(`(() => { const s = document.querySelector('#scadaEd .se-props select[data-f="tag"]'); s.value = s.options[1].value; s.dispatchEvent(new Event('change', {bubbles:true})); return !!SPEC.elements.find(e => e.id === '${tankId}').tag; })()`), true);
  expect("dynamic added (state by default for symbols)", await ev(`(() => { document.querySelector('#scadaEd .se-props [data-dadd]').click(); return SPEC.elements.find(e => e.id === '${tankId}').dyn[0].prop; })()`), "state");
  expect("dynamic switched to level", await ev(`(() => { const s = document.querySelector('#scadaEd .se-props select[data-dp="0"]'); s.value = 'level'; s.dispatchEvent(new Event('change', {bubbles:true})); return JSON.stringify(SPEC.elements.find(e => e.id === '${tankId}').dyn[0]); })()`), JSON.stringify({ prop: "level", lin: [0, 100, 0, 1] }));
  // Organization library (US6): store the tank + pump as a composite, import an SVG, place both.
  const libName = "probe-lib-" + Math.floor(Math.random() * 1e6);
  await ev(`window.__scadaEditor.select(SPEC.elements.filter(e => e.type === 'symbol').map(e => 'e:' + e.id))`);
  await send("Runtime.evaluate", { expression: `(() => { const orig = ctrl32.ask; ctrl32.ask = (t) => (t.startsWith('Name') ? '${libName}' : 'probe'); document.querySelector('#scadaEd [data-lib="save"]').click(); setTimeout(() => (ctrl32.ask = orig), 2000); })()` });
  await sleep(1500);
  const comp = await ev(`fetch('/api/v1/hmi/objects/${libName}',{credentials:'same-origin'}).then(r => r.ok ? r.json() : null).then(o => o && (o.kind + ' ' + o.body.elements.length + ' ' + JSON.stringify(o.body.tags)))`);
  expect("selection stored as a composite with a tag placeholder", comp, (v) => typeof v === "string" && v.startsWith("composite 2 [") && v.includes('"'));
  const svgSlug = await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token};
    const r = await (await fetch('/api/v1/hmi/objects/import-svg',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({name:'${libName}-logo', svg:'<svg xmlns="http://www.w3.org/2000/svg" width="200" height="80"><rect width="200" height="80" fill="#1e3a8a"/><script>alert(1)</script></svg>', reason:'probe'})})).json(); return r.slug; })()`);
  await ev(`window.__scadaEditor._debug.loadLibrary()`); await sleep(500);
  expect("library tiles listed", await ev(`document.querySelectorAll('#scadaEd .se-libobj').length >= 2`), true);
  await ev(`window.__scadaEditor._debug.placeLibrary('${svgSlug}', 'svg')`); await sleep(400);
  expect("imported graphic placed as an image", await ev(`(() => { const e = SPEC.elements.find(e => e.type === 'image'); return e ? e.src + ' ' + !!document.querySelector('#scadaEd [data-eid="' + e.id + '"] image') : 'none'; })()`), svgSlug + " true");
  await send("Runtime.evaluate", { expression: `window.__scadaEditor._debug.placeLibrary('${libName}', 'composite')` }); await sleep(800);
  expect("re-binding dialog for the placeholder", await ev(`(() => { const d = document.querySelector('dialog.se-dlg'); if (!d) return 'no dialog'; const s = d.querySelector('select'); s.value = s.options[1].value; d.querySelector('form').requestSubmit(); return 'ok'; })()`), "ok");
  await sleep(500);
  expect("composite placed as one group with the chosen tag", await ev(`(() => { const g = SPEC.elements.filter(e => e.group); const groups = new Set(g.map(e => e.group)); return g.length + ' ' + groups.size + ' ' + g.some(e => e.tag && !e.tag.startsWith('$')); })()`), "2 1 true");
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/scada-editor.png", Buffer.from(s1.result.data, "base64"));

  // Save with a reason, reload and compare.
  const local = await ev(`JSON.stringify(SPEC.elements.map(e => [e.id, e.type, e.symbol || '', Math.round(e.x), Math.round(e.y)]))`);
  await ev(`(() => { document.querySelector('#eSave').click(); const f = document.querySelector('#saveForm'); f.reason.value = 'probe edit'; setTimeout(() => f.requestSubmit(), 100); return true; })()`);
  await sleep(3500);
  const stored = await ev(`fetch('/api/v1/dashboards/${slug}',{credentials:'same-origin'}).then(r=>r.json()).then(j => ({v: j.version, els: JSON.stringify(j.spec.elements.map(e => [e.id, e.type, e.symbol || '', Math.round(e.x), Math.round(e.y)])), dyn: JSON.stringify(j.spec.elements.find(e => e.id === '${tankId}').dyn.map(d => ({prop: d.prop, lin: d.lin})))}))`);
  expect("saved as version 2", stored.v, 2);
  expect("stored elements equal the editor", stored.els, local);
  expect("stored dynamic", stored.dyn, JSON.stringify([{ prop: "level", lin: [0, 100, 0, 1] }]));
  await send("Page.navigate", { url: base + "/dashboards/" + slug + "/edit" }); await sleep(4000);
  expect("reloaded editor shows the same objects", await ev(`JSON.stringify(SPEC.elements.map(e => [e.id, e.type, e.symbol || '', Math.round(e.x), Math.round(e.y)]))`), local);
  expect("rendered layer has every element", await ev(`document.querySelectorAll('#scadaEd .sc-root > [data-eid]').length === SPEC.elements.length`), true);

  const del = await ev(`(async () => { const h = {'X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; for (const s of ['${libName}', '${svgSlug}']) await fetch('/api/v1/hmi/objects/' + s + '?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'}); return (await fetch('/api/v1/dashboards/${slug}?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`);
  console.log("cleanup:", del);
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
