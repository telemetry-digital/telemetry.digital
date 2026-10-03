// Headless Chrome: the dashboard widget "Period table" (spec 023). Creates a dashboard with a daily log of a
// temperature (min and max per day, limits 3–7) and a table of counters per month, checks the rows, the ◀ ▶ period
// navigation, the export links (Excel, CSV, PDF) and that the Excel download is a workbook, and the widget's options
// in the editor. Screenshots period-*.png go to OUT. Uses the first two datastreams of the organization.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9375;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-period", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text + " " + JSON.stringify(r.result.exceptionDetails.exception) } : r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const until = async (expr, ms = 10000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2500) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await go("/dashboards", 2000);
  // the fridge and meters of the e2e suite when present (they have data), else any temperature and counters
  const ds = await ev(`ctrl32.api('GET','/api/v1/datastreams').then(l => { const t = l.filter(d => d.asset.startsWith('Fridge ') && d.key === 'temp').pop() || l.find(d => d.quantity === 'temperature') || l[0]; let m = [l.filter(d => d.key === 'e_a').pop(), l.filter(d => d.key === 'e_b').pop()].filter(Boolean); if (!m.length) m = l.filter(d => d.kind === 'counter').slice(0, 2); return {t: t.id, m: (m.length ? m : [l[0]]).map(d => d.id)}; })`);
  expect("datastreams to bind", !!(ds && ds.t), true);
  const slug = "period-probe-" + (Date.now() % 100000);
  const spec = { version: 1, kind: "dashboard", layout: { type: "grid", row_height: 96 }, range: "24h", refresh_s: 60, widgets: [
    { id: "log", type: "period_table", geometry: { x: 0, y: 0, w: 6, h: 6 }, bindings: { datastreams: [ds.t] }, params: { title: "Daily temperature log", interval: "day", values: "min,max", period: "last_7d", low: 3, high: 7, total: true, decimals: 1 } },
    { id: "meters", type: "period_table", geometry: { x: 6, y: 0, w: 6, h: 6 }, bindings: { datastreams: ds.m }, params: { title: "Energy consumption", interval: "month", values: "delta", period: "this_year", total: true, decimals: 1, series: [{ label: "Main meter" }, { label: "Heating" }] } }] };
  const made = await ev(`ctrl32.api('POST','/api/v1/dashboards',{name:'${slug}',spec:${JSON.stringify(spec)},reason:'headless probe'}).then(r => r.slug)`);
  expect("dashboard created", typeof made === "string", true);
  await go("/dashboards/" + made, 4000);
  expect("daily log: a row per day of the last 7 days and the total", await until(`(() => { const t = document.querySelector('[data-id="log"] .pt-table, .dw[data-wid="log"] .pt-table') || document.querySelectorAll('.pt-table')[0]; return !!t && t.querySelectorAll('tbody tr').length === 8; })()`), true);
  expect("months of this year in the meter table", await ev(`(() => { const t = document.querySelectorAll('.pt-table')[1]; return t ? t.querySelectorAll('tbody tr').length : 0; })()`), (n) => n === 13);
  expect("export links: Excel, CSV, PDF", await ev(`[...document.querySelectorAll('.pt-bar')[0].querySelectorAll('a')].map(a => a.textContent).join()`), "Excel,CSV,PDF");
  const xl = await ev(`fetch(document.querySelectorAll('.pt-bar')[0].querySelector('a').href, {credentials:'same-origin'}).then(async r => r.status + ' ' + r.headers.get('content-type') + ' ' + new Uint8Array(await r.arrayBuffer()).slice(0, 2).join('.'))`);
  expect("Excel download is a workbook", xl, "200 application/vnd.openxmlformats-officedocument.spreadsheetml.sheet 80.75");
  await shot("period-dashboard.png");
  // ◀ moves to the previous 7 days, ▶ back; ▶ is disabled on the current period
  const label0 = await ev(`document.querySelectorAll('.pt-bar')[0].querySelector('b').textContent`);
  expect("next disabled on the current period", await ev(`document.querySelectorAll('.pt-bar')[0].querySelector('.pt-next').disabled`), true);
  await ev(`document.querySelectorAll('.pt-bar')[0].querySelector('.pt-prev').click()`);
  expect("previous period", await until(`document.querySelectorAll('.pt-bar')[0].querySelector('b').textContent !== ${JSON.stringify(label0)}`), true);
  expect("export follows the shown period", await ev(`document.querySelectorAll('.pt-bar')[0].querySelector('a').href.includes('shift=-1')`), true);
  await ev(`document.querySelectorAll('.pt-bar')[0].querySelector('.pt-now').click()`);
  expect("back to the current period", await until(`document.querySelectorAll('.pt-bar')[0].querySelector('b').textContent === ${JSON.stringify(label0)}`), true);
  // phone
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/dashboards/" + made, 4000);
  await until(`document.querySelectorAll('.pt-table').length === 2`);
  await shot("period-dashboard-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");
  // editor: the options of the widget with readable names
  await go("/dashboards/" + made + "/edit", 3500);
  await ev(`document.querySelector('#eGrid .dash-widget').click()`);
  expect("widget selected in the editor", await until(`!!document.querySelector('select[data-p="period"]')`), true);
  expect("editor offers readable period names", await ev(`[...document.querySelectorAll('select[data-p="period"] option')].map(o => o.textContent).join('|')`), (s) => typeof s === "string" && s.split("|").length === 11 && !/_/.test(s)); // readable names (any language), not the codes
  // every value choice of the catalogue (12 since the sums of object counting, 0.70) shows a readable name, never its code
  expect("editor offers readable values", await ev(`[...document.querySelectorAll('select[data-p="values"] option')].map(o => o.value === o.textContent ? '!' + o.value : o.textContent).join('|')`), (s) => typeof s === "string" && s.split("|").length === 12 && !s.includes("!"));
  await shot("period-editor.png");
  expect("dashboard removed", await ev(`ctrl32.api('DELETE','/api/v1/dashboards/${made}?reason=headless%20probe').then(() => true)`), true);
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
