// Headless Chrome: a SCADA process picture (spec 001, US1/US5). The probe creates its own site, asset, datastreams
// and device, stores a picture with drawing elements (tank level, pump state, pipe flow, text value, blink) and a
// trend widget, publishes values through the device HTTP API and checks what the page draws — first values, then a
// live update without reload. Any mismatch is a problem and the probe exits 1.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9361;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile21", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  // Fixtures through the API (helper kept on window for the later steps).
  const fx = await ev(`(async () => {
    const me = await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json();
    const h = {'Content-Type':'application/json','X-CSRF-Token': me.csrf_token};
    const post = async (u, b) => { const r = await fetch(u,{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify(Object.assign({reason:'probe'}, b))}); const j = await r.json().catch(()=>({})); if (!r.ok) throw new Error(u + ' ' + r.status + ' ' + JSON.stringify(j)); return j; };
    const suf = Math.floor(Math.random()*1e6);
    const site = await post('/api/v1/sites', {name:'SCADA probe ' + suf});
    const asset = await post('/api/v1/assets', {site_id: site.id, type:'machine', name:'Mixer ' + suf});
    const ds = {};
    for (const [key, unit] of [['level','%'],['pump','' ],['flow','m3/h'],['temp','C']]) ds[key] = (await post('/api/v1/datastreams', {asset_id: asset.id, key, quantity: key === 'temp' ? 'temperature' : 'other', unit, kind: 'gauge'})).id;
    const dev = await post('/api/v1/devices', {external_id:'scada-' + suf, name:'scada probe', transport:'http'});
    const tok = (await post('/api/v1/devices/' + dev.id + '/token', {})).token;
    for (const k of Object.keys(ds)) await post('/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: ds[k]});
    const spec = {version:1, kind:'mimic', layout:{type:'canvas', width:1200, height:700, bg:'#dfe3e8'}, scheme:'classic', range:'1h', refresh_s:30,
      widgets:[{id:'tr', type:'chart', geometry:{x:800,y:40,w:380,h:260}, bindings:{datastreams:[ds.temp]}, params:{title:'Temperature'}}],
      elements:[
        {id:'tank', type:'symbol', symbol:'tank_vertical', x:100, y:100, w:140, h:260, tag: ds.level, name:'T-101', dyn:[{prop:'level', lin:[0,100,0,1]}]},
        {id:'pump', type:'symbol', symbol:'pump_centrifugal', x:400, y:300, w:90, h:84, tag: ds.pump, name:'P-101', device: dev.id, key:'pump', dyn:[{prop:'state', map:[{op:'==',v:2,to:'fault'},{op:'==',v:1,to:'run'},{op:'==',v:0,to:'stop'}]}]},
        {id:'pipe', type:'pipe', points:[[240,340],[400,340]], style:{sw:12, medium:'#0891b2'}, dyn:[{prop:'flow', tag: ds.flow, op:'>', v:0.5}]},
        {id:'txt', type:'text', x:100, y:380, w:140, h:28, text:'{value} {unit}', tag: ds.temp, decimals:1, style:{font:16, align:'middle', bg:'#ffffff'}},
        {id:'hot', type:'rect', x:600, y:100, w:80, h:40, tag: ds.temp, style:{fill:'#94a3b8'}, dyn:[{prop:'fill', map:[{op:'>=',v:80,to:'#dc2626'},{op:'<',v:80,to:'#16a34a'}]},{prop:'blink', op:'>=', v:80, color:'#dc2626'}]},
        {id:'nodata', type:'text', x:100, y:420, w:140, h:28, text:'{value}', tag:'00000000-0000-7000-8000-000000000000', style:{font:14}}
      ]};
    const d = await post('/api/v1/dashboards', {name:'SCADA probe ' + suf, spec});
    window.__fx = {tok, slug: d.slug, ds, dev: dev.id};
    return {slug: d.slug, tok, dev: dev.id, ds, elements: (d.spec.elements || []).length};
  })()`);
  expect("fixtures", fx && fx.elements, 6);
  if (!fx || !fx.slug) { console.log("problems:", JSON.stringify(problems)); proc.kill(); process.exit(1); }
  const publish = (vals) => ev(`fetch('/api/v1/d/telemetry',{method:'POST',headers:{'Content-Type':'application/json','Authorization':'Bearer ${fx.tok}'},body:JSON.stringify({v:${JSON.stringify(vals)}})}).then(r=>r.status)`);
  expect("publish 1", await publish({ level: 25, pump: 1, flow: 3.2, temp: 21.456 }), 202);
  await sleep(1200);

  await send("Page.navigate", { url: base + "/dashboards/" + fx.slug }); await sleep(4000);
  const q = (eid, expr) => ev(`(() => { const g = document.querySelector('#vGrid [data-eid="${eid}"]'); if (!g) return 'missing'; ${expr} })()`);
  expect("layer below widgets", await ev(`!!document.querySelector('#vGrid .mimic > .sc-host svg.sc-layer') && document.querySelectorAll('#vGrid .mimic-widget').length`), 1);
  expect("tank level ≈ 25 %", await q("tank", `const r = g.querySelector('clipPath + g rect'); return r ? Math.round(100 - (+r.getAttribute('y') - 3) / 94 * 100) : null;`), (v) => v >= 24 && v <= 26);
  expect("pump running colour", await q("pump", `return g.querySelector('circle').getAttribute('fill');`), "#22c55e");
  expect("pipe flow animated", await q("pipe", `return g.querySelectorAll('.sc-flow').length;`), 1);
  expect("text value", await q("txt", `return g.querySelector('text').textContent;`), "21.5 C");
  expect("colour mapping < 80", await q("hot", `return g.querySelector('rect').getAttribute('fill');`), "#16a34a");
  expect("no blink < 80", await q("hot", `return g.querySelectorAll('.sc-blink').length;`), 0);
  expect("no value → dash", await q("nodata", `return g.querySelector('text').textContent;`), "—");
  expect("tank viewport keeps its size (CSS 140 × 260 px)", await ev(`(() => { const cs = getComputedStyle(document.querySelector('#vGrid [data-eid="tank"] svg')); return cs.width + " x " + cs.height; })()`), "140px x 260px");
  expect("clickable pump", await q("pump", `return g.classList.contains('sc-click');`), true);
  // Aeration symbol: rising (animated) bubbles while running, none when stopped, round in a stretched box.
  expect("aeration bubbles run / stop", await ev(`(() => { const S = window.ctrl32Scada; const env = (v) => ({ values: { t: { value: v } }, ds: {}, scheme: 'classic', uid: 'a', edit: false }); const e = { id: 'air', type: 'symbol', symbol: 'aeration', x: 0, y: 0, w: 200, h: 50, tag: 't', dyn: [{ prop: 'state', map: [{ op: '==', v: 1, to: 'run' }, { op: '==', v: 0, to: 'stop' }] }] }; const on = S.renderEl(e, env(1)), off = S.renderEl(e, env(0)); const rx = +(on.match(/rx="([0-9.]+)"/) || [])[1], ry = +(on.match(/ry="([0-9.]+)"/) || [])[1]; return (on.match(/sc-bub/g) || []).length + ' ' + (off.match(/<ellipse/g) || []).length + ' ' + (Math.abs(rx * 4 - ry) < 0.05); })()`), "22 0 true");
  // Library graphics: effects (?anim=1) always without an "animate" dynamic, otherwise only while it is true.
  expect("image animation follows the animate dynamic", await ev(`(() => { const S = window.ctrl32Scada; const env = (v) => ({ values: { t: { value: v } }, ds: {}, scheme: 'classic', uid: 'i', edit: false }); const href = (h) => (h.match(/href="([^"]+)"/) || [])[1]; const e = { id: 'img', type: 'image', src: 'fan', x: 0, y: 0, w: 50, h: 50, tag: 't', dyn: [{ prop: 'animate', op: '==', v: 1, speed: 1.5 }] }; return [href(S.renderEl(e, env(1))), href(S.renderEl(e, env(0))), href(S.renderEl({ ...e, dyn: [] }, env(0)))].map((u) => u.replace(/^.*\\/svg/, '')).join(' | '); })()`), "?anim=1&amp;speed=1.5 |  | ?anim=1&amp;speed=1");
  // Pictures with more than 50 tags: the latest values are fetched in chunks of 50 (the endpoint limit).
  expect("latest values for > 50 tags", await ev(`(async () => { const all = await window.ctrl32Widgets.datastreams(); const ids = all.map((d) => d.id); const pad = Array.from({ length: Math.max(0, 120 - ids.length) }, (_, i) => '00000000-0000-7000-8000-' + String(i).padStart(12, '0')); const r = await window.ctrl32Widgets.latest(ids.concat(pad)); return Object.keys(r).length > 0 && ids.concat(pad).length > 50; })()`), true);
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/scada-view.png", Buffer.from(s1.result.data, "base64"));

  // Live update without reload.
  expect("publish 2", await publish({ level: 80, pump: 2, flow: 0, temp: 85 }), 202);
  await sleep(3500);
  expect("live: tank level ≈ 80 %", await q("tank", `const r = g.querySelector('clipPath + g rect'); return r ? Math.round(100 - (+r.getAttribute('y') - 3) / 94 * 100) : null;`), (v) => v >= 79 && v <= 81);
  expect("live: pump fault colour", await q("pump", `return g.querySelector('circle').getAttribute('fill');`), "#ef4444");
  expect("live: flow stopped", await q("pipe", `return g.querySelectorAll('.sc-flow').length;`), 0);
  expect("live: alarm colour + blink frame", await q("hot", `return g.querySelector('rect').getAttribute('fill') + ' ' + g.querySelectorAll('.sc-blink').length;`), "#dc2626 1");

  // Faceplate on click.
  expect("faceplate opens", await ev(`(() => { document.querySelector('#vGrid [data-eid="pump"]').dispatchEvent(new MouseEvent('click', {bubbles:true})); const d = document.querySelector('dialog.sc-dlg'); return !!(d && d.open && d.querySelector('.sc-fp-big')); })()`), true);
  await sleep(1500);
  expect("faceplate value + trend", await ev(`(() => { const d = document.querySelector('dialog.sc-dlg'); return d.querySelector('.sc-fp-big').textContent + ' ' + !!d.querySelector('.sc-fp-chart svg, .sc-fp-chart .muted'); })()`), "2.0 true");
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/scada-faceplate.png", Buffer.from(s2.result.data, "base64"));
  // Operation from the faceplate: a reason is required, the command is audited with it (US4, T024).
  expect("start without reason is refused", await ev(`(() => { const d = document.querySelector('dialog.sc-dlg'); d.querySelector('[data-v="true"]').click(); return d.querySelector('.sc-out').textContent; })()`), "reason is required");
  expect("start with a reason", await ev(`(() => { const d = document.querySelector('dialog.sc-dlg'); d.querySelector('.sc-reason').value = 'probe start P-101'; d.querySelector('[data-v="true"]').click(); return true; })()`), true);
  await sleep(1500);
  expect("command stored with key, value and reason", await ev(`fetch('/api/v1/devices/${fx.dev}/commands',{credentials:'same-origin'}).then(r=>r.json()).then(l => { const c = (l.items || l).find(c => c.reason === 'probe start P-101'); return c ? c.method + ' ' + JSON.stringify(c.args) : 'none'; })`), 'write {"key":"pump","value":true}');
  expect("faceplate shows the command status", await ev(`document.querySelector('dialog.sc-dlg .sc-out').textContent.trim().length > 0`), true);
  await ev(`document.querySelector('dialog.sc-dlg').close()`);
  // Public link: values yes, operation no.
  const ptok = await ev(`(async () => { const h = {'Content-Type':'application/json','X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; const r = await (await fetch('/api/v1/dashboards/${fx.slug}/share',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({reason:'probe'})})).json(); return r.url.split('/').pop(); })()`);
  await send("Page.navigate", { url: base + "/p/" + ptok }); await sleep(4000);
  expect("public picture renders the drawing", await ev(`document.querySelectorAll('.sc-root > [data-eid]').length`), 6);
  expect("public faceplate is read-only", await ev(`(() => { document.querySelector('[data-eid="pump"]').dispatchEvent(new MouseEvent('click', {bubbles:true})); const d = document.querySelector('dialog.sc-dlg'); return !!d && d.open && !d.querySelector('[data-v]') && d.textContent.includes('read-only'); })()`), true);
  await send("Page.navigate", { url: base + "/dashboards" }); await sleep(1500);

  // Performance (SC-002/SC-003, T036): 1500 objects bound to tags render in under 3 s; the editor opens and a nudge of
  // everything redraws in well under a second.
  const big = await ev(`(async () => { const me = await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json(); const h = {'Content-Type':'application/json','X-CSRF-Token': me.csrf_token};
    const ds = ${JSON.stringify(fx.ds)}; const els = [];
    for (let i = 0; i < 500; i++) { const x = 20 + (i % 25) * 150, y = 20 + Math.floor(i / 25) * 140;
      els.push({type:'symbol', symbol: ['tank_vertical','pump_centrifugal','valve_ball','motor','heat_exchanger'][i % 5], x, y, w:60, h:80, tag: ds.level, dyn:[{prop:'level', lin:[0,100,0,1]}, {prop:'state', tag: ds.pump, map:[{op:'==',v:1,to:'run'}]}]});
      els.push({type:'pipe', points:[[x + 60, y + 40], [x + 140, y + 40]], style:{sw:8}, dyn:[{prop:'flow', tag: ds.flow, op:'>', v:0.5}]});
      els.push({type:'text', x, y: y + 84, w:100, h:20, text:'{value} {unit}', tag: ds.temp, style:{font:11}}); }
    const r = await (await fetch('/api/v1/dashboards',{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify({name:'Perf probe ' + Math.floor(Math.random()*1e6), reason:'probe', spec:{kind:'mimic', layout:{type:'canvas', width:4000, height:3000}, widgets:[], elements: els}})})).json(); return r.slug; })()`);
  let t0 = Date.now(); await send("Page.navigate", { url: base + "/dashboards/" + big });
  let n = 0; while (Date.now() - t0 < 15000) { n = await ev(`document.querySelectorAll('#vGrid .sc-root > [data-eid]').length`).catch(() => 0); if (n === 1500) break; await sleep(100); }
  const viewMs = Date.now() - t0;
  expect("1500 objects rendered in under 3 s (" + viewMs + " ms)", n === 1500 && viewMs < 3000, true);
  t0 = Date.now(); await send("Page.navigate", { url: base + "/dashboards/" + big + "/edit" });
  while (Date.now() - t0 < 20000) { if (await ev(`!!(window.__scadaEditor && document.querySelectorAll('#scadaEd .sc-root > [data-eid]').length === 1500)`).catch(() => false)) break; await sleep(100); }
  const editMs = Date.now() - t0;
  expect("editor with 1500 objects ready in under 5 s (" + editMs + " ms)", editMs < 5000, true);
  const nudge = await ev(`(() => { __scadaEditor.select(SPEC.elements.map(e => 'e:' + e.id)); const t = performance.now(); document.dispatchEvent(new KeyboardEvent('keydown', {key: 'ArrowRight', bubbles: true})); return Math.round(performance.now() - t); })()`);
  expect("moving all 1500 objects redraws in under 1 s (" + nudge + " ms)", nudge < 1000, true);
  await ev(`(DIRTY = false, true)`);
  await ev(`(async () => { const h = {'X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; return (await fetch('/api/v1/dashboards/${big}?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`);

  const del = await ev(`(async () => { const h = {'X-CSRF-Token': (await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json()).csrf_token}; return (await fetch('/api/v1/dashboards/${fx.slug}?reason=probe',{method:'DELETE',headers:h,credentials:'same-origin'})).status; })()`);
  console.log("cleanup:", del);
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
