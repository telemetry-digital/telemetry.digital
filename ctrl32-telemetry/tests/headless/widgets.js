// Headless Chrome: dashboard widgets with rich settings (spec 002). The probe creates its own datastreams and device,
// stores a dashboard with one widget of every catalogue type plus styled / conditional / click-action widgets,
// publishes values and checks: every type renders without an error (T017), the appearance settings reach the card
// (T008), conditional formatting follows a live value and a click opens the trend (T019), the chart applies
// per-series settings with a right axis, full screen opens and closes on Escape, and the editor offers the tabs.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9363;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile23", "--window-size=1500,1000", "about:blank"], { stdio: "ignore" });
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
  const key = async (k, code) => { await send("Input.dispatchKeyEvent", { type: "keyDown", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); await send("Input.dispatchKeyEvent", { type: "keyUp", key: k, code, windowsVirtualKeyCode: code === "Escape" ? 27 : 0 }); };

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  const fx = await ev(`(async () => {
    const me = await (await fetch('/api/v1/auth/me',{credentials:'same-origin'})).json();
    const h = {'Content-Type':'application/json','X-CSRF-Token': me.csrf_token};
    const post = async (u, b) => { const r = await fetch(u,{method:'POST',headers:h,credentials:'same-origin',body:JSON.stringify(Object.assign({reason:'probe'}, b))}); const j = await r.json().catch(()=>({})); if (!r.ok) throw new Error(u + ' ' + r.status + ' ' + JSON.stringify(j)); return j; };
    const suf = Math.floor(Math.random()*1e6);
    const site = await post('/api/v1/sites', {name:'Widget probe ' + suf});
    const asset = await post('/api/v1/assets', {site_id: site.id, type:'machine', name:'Line ' + suf});
    const ds = {};
    for (const [key, unit] of [['temp','C'],['rh','%'],['state',''],['dir','deg']]) ds[key] = (await post('/api/v1/datastreams', {asset_id: asset.id, key, quantity: key === 'temp' ? 'temperature' : 'other', unit, kind: 'gauge'})).id;
    const dev = await post('/api/v1/devices', {external_id:'wprobe-' + suf, name:'widget probe', transport:'http'});
    const tok = (await post('/api/v1/devices/' + dev.id + '/token', {})).token;
    for (const k of Object.keys(ds)) await post('/api/v1/devices/' + dev.id + '/datastreams', {datastream_id: ds[k]});
    const cat = await (await fetch('/api/v1/dashboards/catalog',{credentials:'same-origin'})).json();
    const all = [ds.temp, ds.rh, ds.state];
    // a dashboard holds at most 60 widgets: the object counting widgets (0.71.0) are covered by tests/headless/counting.js,
    // the trend and batch widgets (0.97.0) by tests/headless/batches.js
    const widgets = cat.widgets.filter((d) => !/^counting_|^vehicle_speeds$|^trend$|^batches$/.test(d.type)).map((d, i) => ({id: 't' + i, type: d.type, geometry: {x: (i % 3) * 4, y: 20 + Math.floor(i / 3) * 3, w: 4, h: 3},
      bindings: Object.fromEntries((d.binds || []).map((b) => [b, b === 'devices' ? (['map','devices'].includes(d.type) ? [] : [dev.id]) : (d.type === 'radar' ? all : d.type === 'compass' ? [ds.dir, ds.rh] : [ds.temp])])),
      params: Object.assign({title: 'T ' + d.type}, d.type === 'setpoint' || d.type === 'switch' || d.type === 'button' || d.type === 'knob' || d.type === 'slider' || d.type === 'round_switch' ? {key: 'temp'} : {})}));
    widgets.push(
      {id:'styled', type:'value', geometry:{x:0,y:0,w:3,h:2}, bindings:{datastreams:[ds.temp]}, params:{title:'Styled', show_title:false, bg_color:'#0f172a', text_color:'#f8fafc', border_width:0, radius:14, value_size:40, sparkline:false}},
      {id:'cond', type:'value_card', geometry:{x:3,y:0,w:3,h:2}, bindings:{datastreams:[ds.temp]}, params:{title:'Conditional', ranges:[{op:'<',v:2,target:'background',color:'#2563eb'},{op:'between',v:2,v2:8,target:'background',color:'#16a34a'},{op:'>',v:8,target:'background',color:'#dc2626'}], click:'trend', sparkline:false, trend_arrow:false}},
      {id:'bars2', type:'bars', geometry:{x:0,y:3,w:4,h:2}, bindings:{datastreams:[ds.temp, ds.rh]}, params:{title:'Bars', series:[{label:'Teplota', color:'#123456', decimals:0, unit:'K'},{hidden:true}], warn:100, title_color:'#aa0000', title_align:'center', font:'mono'}},
      {id:'pie2', type:'pie', geometry:{x:4,y:3,w:4,h:3}, bindings:{datastreams:[ds.temp, ds.rh]}, params:{title:'Pie', legend_pos:'none', series:[{color:'#00ff00'},{color:'#0000ff'}]}},
      {id:'gauge2', type:'radial_gauge', geometry:{x:8,y:3,w:4,h:3}, bindings:{datastreams:[ds.temp]}, params:{title:'Gauge', show_label:false, warn:20, action:40, warn_color:'#ffff00', action_color:'#ff00ff', value_color:'#010203'}},
      {id:'chart2', type:'chart', geometry:{x:6,y:0,w:6,h:3}, bindings:{datastreams:[ds.temp, ds.rh]}, params:{title:'Two axes', range:'1h', series:[{label:'Temperature A', color:'#ff00aa', unit:'°C'},{label:'Humidity B', color:'#00aa55', axis:'right', unit:'%'}], legend_values:['min','max','latest'], legend_pos:'bottom', filters:['range','agg','series']}});
    const d = await post('/api/v1/dashboards', {name:'Widget probe ' + suf, spec: {version:1, range:'1h', refresh_s:30, widgets}});
    return {slug: d.slug, tok, dev: dev.id, ds, types: cat.widgets.length, stored: d.spec.widgets.length};
  })()`);
  expect("fixtures: catalogue types ≥ 44", fx && fx.types, (n) => n >= 44);
  if (!fx || !fx.slug) { console.log("problems:", JSON.stringify(problems)); proc.kill(); process.exit(1); }
  const publish = (vals) => ev(`fetch('/api/v1/d/telemetry',{method:'POST',headers:{'Content-Type':'application/json','Authorization':'Bearer ${fx.tok}'},body:JSON.stringify({v:${JSON.stringify(vals)}})}).then(r=>r.status)`);
  expect("publish 1", await publish({ temp: 5.5, rh: 40, state: 1, dir: 225 }), 202);
  await sleep(1100);
  expect("publish 2", await publish({ temp: 6.5, rh: 45, state: 2, dir: 230 }), 202);
  await sleep(1200);

  await send("Page.navigate", { url: base + "/dashboards/" + fx.slug }); await sleep(6000);
  // T017 — every type renders without an error.
  const bad = await ev(`[...document.querySelectorAll('#vGrid .dash-widget')].map((c) => { const b = c.querySelector('.dash-body'); const t = b.textContent; return [c.dataset.id, t]; }).filter(([id, t]) => /unknown widget|loading…|is not |undefined|NaN|Cannot read|Error/.test(t)).map(([id, t]) => id + ': ' + t.slice(0, 80))`);
  expect("every widget type renders", bad, (x) => Array.isArray(x) && x.length === 0);
  expect("cards on the page", await ev(`document.querySelectorAll('#vGrid .dash-widget').length`), fx.stored);
  // Full configurability: series settings, zone colours, label switch, title and font on non-chart widgets.
  const cfg = await ev(`(() => { const q = (id) => document.querySelector('#vGrid [data-id="' + id + '"]'); const b = q('bars2'), pie = q('pie2'), g = q('gauge2'); const t = b.querySelector('.dash-title');
    return { barsRows: b.querySelectorAll('.dash-body > div').length, barsText: b.querySelector('.dash-body').textContent, barColour: [...b.querySelectorAll('.dash-body div[style*="background:#123456"]')].length,
      titleColour: getComputedStyle(t).color, titleJustify: getComputedStyle(t).justifyContent, font: getComputedStyle(b).fontFamily,
      pieFills: [...pie.querySelectorAll('path[fill]')].map(x => x.getAttribute('fill')), pieLegend: pie.querySelectorAll('.dash-body i').length,
      gaugeBands: [...g.querySelectorAll('path[stroke]')].map(x => x.getAttribute('stroke')).filter(c => c.startsWith('#')), gaugeLabel: getComputedStyle(g.querySelector('.dw-lbl')).display, gaugeValue: [...g.querySelectorAll('text')].some(x => x.getAttribute('fill') === '#010203') }; })()`);
  expect("bars: hidden series left out", cfg.barsRows, 1);
  expect("bars: series label, unit and decimals", cfg.barsText, (x) => typeof x === "string" && x.includes("Teplota") && x.includes("7 K"));
  expect("bars: series colour", cfg.barColour, 1);
  expect("title colour", cfg.titleColour, "rgb(170, 0, 0)");
  expect("title centred", cfg.titleJustify, "center");
  expect("mono font", cfg.font, (f) => typeof f === "string" && /mono/i.test(f));
  expect("pie: series colours", cfg.pieFills, (f) => Array.isArray(f) && f.includes("#00ff00") && f.includes("#0000ff"));
  expect("pie: legend off", cfg.pieLegend, 0);
  expect("gauge: zone colours", cfg.gaugeBands, (c) => Array.isArray(c) && c.includes("#ffff00") && c.includes("#ff00ff"));
  expect("gauge: label hidden", cfg.gaugeLabel, "none");
  expect("gauge: value colour", cfg.gaugeValue, true);
  // T008 — appearance.
  const st = await ev(`(() => { const c = document.querySelector('#vGrid [data-id="styled"]'); const cs = getComputedStyle(c); const v = c.querySelector('.dash-body [style*="--dw-value"], .dash-body div[style*="font-size"]'); return {bg: cs.backgroundColor, fg: cs.color, bw: cs.borderTopWidth, radius: cs.borderTopLeftRadius, head: c.querySelector('.dash-head').hidden, value: v ? getComputedStyle(v).fontSize : null}; })()`);
  expect("styled background", st.bg, "rgb(15, 23, 42)");
  expect("styled text colour", st.fg, "rgb(248, 250, 252)");
  expect("styled no border", st.bw, "0px");
  expect("styled radius", st.radius, "14px");
  expect("styled title hidden", st.head, true);
  expect("styled value size", st.value, "40px");
  // T019 — conditional background follows the live value; click opens the trend.
  const bgOf = (wid) => ev(`getComputedStyle(document.querySelector('#vGrid [data-id="${wid}"]')).backgroundColor`);
  expect("range 2–8 → green", await bgOf("cond"), "rgb(22, 163, 74)");
  expect("publish 9.1", await publish({ temp: 9.1 }), 202);
  await sleep(2500);
  expect("range > 8 → red (live)", await bgOf("cond"), "rgb(220, 38, 38)");
  expect("publish 1.0", await publish({ temp: 1.0 }), 202);
  await sleep(2500);
  expect("range < 2 → blue (live)", await bgOf("cond"), "rgb(37, 99, 235)");
  await ev(`document.querySelector('#vGrid [data-id="cond"] .dash-body').click()`); await sleep(1500);
  expect("click opens the trend", await ev(`(() => { const d = document.querySelector('dialog.sc-dlg'); return !!(d && d.open && d.querySelector('.sc-fp-chart')); })()`), true);
  await ev(`document.querySelector('dialog.sc-dlg').close()`);
  // Chart: per-series label/colour, right axis, legend values at the bottom.
  const ch = await ev(`(() => { const c = document.querySelector('#vGrid [data-id="chart2"]'); const leg = c.querySelector('.dw-legend'); const svg = c.querySelector('.chart-box svg'); return {legend: leg ? leg.textContent : '', bottom: !!(leg && leg.previousElementSibling && leg.previousElementSibling.classList.contains('chart-box')), colours: svg ? [...svg.querySelectorAll('path[stroke]')].map(p => p.getAttribute('stroke')).filter(s => s.startsWith('#')) : [], rightLabels: svg ? [...svg.querySelectorAll('text')].filter(t => +t.getAttribute('x') > 900).length : 0}; })()`);
  expect("series labels in legend", ch.legend, (s) => s.includes("Temperature A") && s.includes("Humidity B") && s.includes("latest"));
  expect("legend at the bottom", ch.bottom, true);
  expect("series colours", ch.colours, (c) => c.includes("#ff00aa") && c.includes("#00aa55"));
  expect("right axis labels", ch.rightLabels, (n) => n >= 2);
  // Viewer filters: the author enabled range, aggregation and series; switching a series off hides it.
  expect("filter bar", await ev(`(() => { const b = document.querySelector('#vGrid [data-id="chart2"] .dw-filters'); return b ? [b.querySelectorAll('select').length, b.querySelectorAll('[data-si]').length] : null; })()`), (x) => Array.isArray(x) && x[0] === 2 && x[1] === 2);
  await ev(`document.querySelector('#vGrid [data-id="chart2"] .dw-filters [data-si="1"]').click()`); await sleep(1500);
  expect("series switched off", await ev(`[...document.querySelectorAll('#vGrid [data-id="chart2"] .chart-box svg path[stroke]')].map(p => p.getAttribute('stroke')).includes('#00aa55')`), false);
  await ev(`(() => { const s = document.querySelector('#vGrid [data-id="chart2"] .dw-filters [data-f="range"]'); s.value = '6h'; s.dispatchEvent(new Event('change')); })()`); await sleep(1500);
  expect("range filter applied", await ev(`document.querySelector('#vGrid [data-id="chart2"] .dw-win').textContent`), (t) => typeof t === "string" && (t.startsWith("6h") || t.startsWith("6 h")));
  await ev(`document.querySelector('#vGrid [data-id="chart2"] .dw-filters [data-reset]').click()`); await sleep(1500);
  expect("reset brings the series back", await ev(`[...document.querySelectorAll('#vGrid [data-id="chart2"] .chart-box svg path[stroke]')].map(p => p.getAttribute('stroke')).includes('#00aa55')`), true);
  const s1 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/widgets-view.png", Buffer.from(s1.result.data, "base64"));
  // Full screen: opens with the widget, Escape closes it.
  await ev(`document.querySelector('#vGrid [data-id="chart2"] .dw-fs').click()`); await sleep(2000);
  expect("full screen open", await ev(`(() => { const d = document.querySelector('dialog.dw-full'); return !!(d && d.open && d.querySelector('.chart-box svg')) && d.getBoundingClientRect().width > 1200; })()`), true);
  await key("Escape", "Escape"); await sleep(500);
  expect("Escape closes full screen", await ev(`!document.querySelector('dialog.dw-full')`), true);
  // Editor: tabs and a colour setting applied from the Appearance tab.
  await send("Page.navigate", { url: base + "/dashboards/" + fx.slug + "/edit" }); await sleep(4000);
  const tabs = await ev(`(() => { const i = SPEC.widgets.findIndex(w => w.id === 'chart2'); select(i); return [...document.querySelectorAll('#eProps [data-tab]')].map(t => t.dataset.tab); })()`);
  expect("chart tabs", tabs, (t) => Array.isArray(t) && ["data", "appearance", "series", "time", "actions"].every((k) => t.includes(k)));
  expect("series editor rows", await ev(`document.querySelectorAll('#eProps [data-ser]').length`), 2);
  const applied = await ev(`(() => { document.querySelector('#eProps [data-tab="appearance"]').click(); const inp = document.querySelector('#eProps [data-sec="appearance"] [data-p="bg_color"]'); inp.value = '#112233'; inp.dispatchEvent(new Event('change', {bubbles: true})); return SPEC.widgets.find(w => w.id === 'chart2').params.bg_color; })()`);
  expect("appearance applied to the spec", applied, "#112233");
  await sleep(1500);
  expect("editor preview shows it", await ev(`getComputedStyle(document.querySelector('#eGrid [data-id="chart2"]')).backgroundColor`), "rgb(17, 34, 51)");
  const s2 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/widgets-edit.png", Buffer.from(s2.result.data, "base64"));

  console.log(problems.length ? "problems: " + JSON.stringify(problems) : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
