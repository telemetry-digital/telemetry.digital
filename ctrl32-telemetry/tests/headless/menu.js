// Headless Chrome: the menu (0.73.4; 0.75.0: Overview, the Reports and Automation groups, My account once in the
// footer, the old report and rule addresses redirect). Checks the section headings (Monitoring, Devices and automation, Administration),
// that every page tab (.subnav) a user sees is also in the sidebar, that the group of a sub-page opens on it (System on
// /server/license), that the compact sidebar (icons only, narrow desktop) opens a group's first page and keeps the page
// tabs, the phone drawer (only the current group open, a tap opens another one) and what a viewer sees (no
// Administration). Screenshots menu-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9378;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-menu", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
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
  const shot = async (name, sel) => {
    let clip;
    if (sel) { const r = await ev(`(() => { const e = document.querySelector('${sel}'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: b.x, y: b.y, width: b.width, height: b.height }; })()`); if (r) clip = { ...r, scale: 1 }; }
    const s = await send("Page.captureScreenshot", { format: "png", ...(clip ? { clip } : {}) });
    fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64"));
  };
  const go = async (url, wait = 1800) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };
  const login = (u, p) => ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:${JSON.stringify(u)},password:${JSON.stringify(p)}})}).then(r=>r.json())`);
  const desktop = (h = 1000, w = 1400) => send("Emulation.setDeviceMetricsOverride", { width: w, height: h, deviceScaleFactor: 1, mobile: false });
  const phone = () => send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  const heads = `[...document.querySelectorAll('#sidebar .nav-head')].map(h => h.textContent || '—').join('|')`;
  const top = `[...document.querySelectorAll('#sidebar .side-nav > a.nav-link, #sidebar .side-nav > .group > a.nav-link')].map(a => a.getAttribute('href')).join()`;
  // every visible page tab is a link of the sidebar
  const tabsInMenu = `(() => { const side = new Set([...document.querySelectorAll('#sidebar a.nav-link')].map(a => a.getAttribute('href'))); return [...document.querySelectorAll('.subnav:not(#tabs) a[href]')].filter(a => !a.hidden).map(a => a.getAttribute('href')).filter(h => !side.has(h)).join() || 'all'; })()`;

  await go("/login", 1200);
  await ev(`localStorage.removeItem('ctrl32.sidebar')`);
  expect("admin signed in", (await login("admin", "admin1234")).user !== undefined, true);
  await desktop(1500);
  await go("/settings/branding");
  expect("headings", await ev(heads), "Monitoring|Devices and automation|Administration");
  expect("top-level order", await ev(top), "/,/dashboards,/operator,/trends,/recipes,/batches,/home,/energy,/counting,/video,/displays,/alarms,/oplog,/approvals,/incidents,/reports/templates,/devices,/assets,/flows,/fota,/users,/audit,/settings,/organizations,/account");
  // 0.75.0: clearer names, Reports and Automation groups, My account once (in the footer of the sidebar)
  expect("Overview and Dashboards", await ev(`[...document.querySelectorAll('#sidebar a.nav-link[href="/"], #sidebar a.nav-link[href="/dashboards"]')].map(a => a.textContent).join()`), "Overview,Dashboards");
  expect("My account once", await ev(`[...document.querySelectorAll('#sidebar a, #sidebar button')].filter(a => a.textContent.trim() === 'My account').length + ' ' + !!document.querySelector('#sidebar .side-foot .side-acct a[href="/account"]')`), "1 true");
  expect("Settings without the report pages", await ev(`[...document.querySelectorAll('#sidebar .group.open .sub a')].map(a => a.textContent).filter(x => /report|Notification/i.test(x)).join()`), "Notification channels");
  expect("Object counting has its own icon", await ev(`(() => { const a = document.querySelector('#sidebar a[href="/counting"]'), u = document.querySelector('#sidebar a[href="/users"]'); return a.querySelector('svg').outerHTML !== u.querySelector('svg').outerHTML; })()`), true);
  expect("Settings sub-pages in the order of the page tabs", await ev(`[...document.querySelectorAll('#sidebar .group.open .sub a')].map(a => a.getAttribute('href')).join() === [...document.querySelectorAll('nav.subnav:not([hidden]) a')].filter(a => !a.hidden).map(a => a.getAttribute('href')).join()`), true);
  expect("page tabs hidden next to the full sidebar", await ev(`getComputedStyle(document.querySelector('.subnav')).display`), "none");
  for (const p of ["/settings/branding", "/reports/scheduled", "/reports", "/reports/templates", "/reports/signatures", "/automation/rules", "/flows", "/account/push", "/server/license", "/server/vpn", "/video", "/account/ai", "/counting", "/audit", "/users", "/fota", "/assets"]) {
    await go(p, 1500);
    expect("tabs of " + p + " in the sidebar", await ev(tabsInMenu), "all");
  }
  await go("/server/license");
  expect("System open on /server/license", await ev(`[...document.querySelectorAll('#sidebar .group.open > a.nav-link')].map(a => a.textContent).join() + ' ' + (document.querySelector('#sidebar .sub a.active') || {}).textContent`), "System License");
  await go("/counting/cameras");
  expect("only Object counting open on its Cameras tab", await ev(`[...document.querySelectorAll('#sidebar .group.open > a.nav-link')].map(a => a.getAttribute('href')).join()`), "/counting");
  expect("counting has Scheduled reports", await ev(`!!document.querySelector('#sidebar .group.open .sub a[href="/reports/scheduled"]')`), true);
  const openGroups = `[...document.querySelectorAll('#sidebar .group.open > a.nav-link')].map(a => a.textContent).join()`;
  await go("/reports/scheduled");
  expect("only Reports open on Scheduled reports", await ev(openGroups), "Reports");
  expect("Reports: four pages", await ev(`[...document.querySelectorAll('#sidebar .group.open .sub a')].map(a => a.textContent).join()`), "Report templates,Stored reports,Scheduled reports,Report signatures");
  await go("/automation/rules");
  expect("only Automation open on Automation rules", await ev(openGroups + ` + ' ' + document.querySelector('#sidebar .sub a.active').textContent`), "Automation Automation rules");
  await go("/account/push");
  expect("My account (footer) open on Browser notifications", await ev(openGroups + ` + ' ' + document.querySelector('#sidebar .sub a.active').textContent`), "My account Browser notifications");
  for (const [from, to] of [["/settings/scheduled-reports", "/reports/scheduled"], ["/settings/templates", "/reports/templates"], ["/settings/reports", "/reports"], ["/audit/reports", "/reports/signatures"], ["/assets/rules", "/automation/alarm-rules"], ["/assets/automation", "/automation/rules"]]) {
    await go(from, 1200);
    expect("old address " + from, await ev(`location.pathname`), to);
  }
  await go("/video/evidence");
  expect("Cameras: Evidence and Settings", await ev(`[...document.querySelectorAll('#sidebar .group.open .sub a')].map(a => a.textContent).join()`), "Live view,Video walls,Camera management,Evidence,Settings,Object counting");
  await go("/settings/branding");
  await shot("menu-admin.png", "#sidebar");

  // compact sidebar: icons only
  await ev(`localStorage.setItem('ctrl32.sidebar', 'icons')`);
  await go("/settings/branding");
  expect("icons: page tabs shown", await ev(`getComputedStyle(document.querySelector('.subnav')).display`), "flex");
  expect("icons: headings are lines", await ev(`[...document.querySelectorAll('#sidebar .nav-head span')].every(s => getComputedStyle(s).display === 'none')`), true);
  await shot("menu-admin-icons.png");
  await ev(`document.querySelector('#sidebar .group > a[href="/video"]').click()`); await sleep(1800);
  expect("icons: a group opens its first page", await ev(`location.pathname`), "/video");
  await ev(`localStorage.removeItem('ctrl32.sidebar')`);
  // narrow desktop (compact by width)
  await desktop(900, 1000);
  await go("/server");
  expect("narrow desktop: page tabs shown", await ev(`getComputedStyle(document.querySelector('.subnav')).display`), "flex");

  // phone drawer
  await phone();
  await go("/counting/areas");
  await ev(`document.querySelector('#menuBtn').click()`); await sleep(500);
  expect("drawer open", await ev(`document.querySelector('#drawer').classList.contains('open')`), true);
  expect("drawer headings", await ev(`[...document.querySelectorAll('#drawer .nav-head')].map(h => h.textContent || '—').join('|')`), "Monitoring|Devices and automation|Administration|—");
  const subs = `[...document.querySelectorAll('#drawer .sub-link')].filter(a => !a.hidden).map(a => a.getAttribute('href')).join()`;
  expect("drawer: only the current group open", await ev(subs), "/counting,/counting/areas,/counting/cameras,/counting/sensors,/reports/scheduled");
  await shot("menu-phone-admin.png");
  await ev(`document.querySelector('#drawer a[data-grp][href="/video"]').click()`); await sleep(300);
  expect("drawer: a tap opens a group without leaving the page", await ev(`location.pathname + ' ' + [...document.querySelectorAll('#drawer .sub-link')].filter(a => !a.hidden).some(a => a.getAttribute('href') === '/video/walls')`), "/counting/areas true");
  await ev(`document.querySelector('#drawer a[data-grp][href="/video"]').click()`); await sleep(300);
  expect("drawer: a second tap closes it", await ev(`[...document.querySelectorAll('#drawer .sub-link')].filter(a => !a.hidden).some(a => a.getAttribute('href') === '/video/walls')`), false);

  // a viewer (data.read only)
  await desktop(1000);
  const uname = "menu-viewer-" + (Date.now() % 100000);
  const made = await ev(`ctrl32.api('POST', '/api/v1/users', {username: '${uname}', full_name: 'Menu Viewer', email: '${uname}@example.com', roles: ['viewer'], reason: 'headless probe'})`);
  expect("viewer created", !!(made && made.initial_password), true);
  await ev(`ctrl32.api('POST', '/api/v1/auth/logout', {}).catch(() => null)`);
  await go("/login", 1200);
  const lr = await login(uname, made.initial_password);
  expect("viewer signed in", !!(lr && lr.user), true);
  if (lr && lr.must_change_password) expect("viewer: initial password changed", await ev(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json()).then(m => fetch('/api/v1/auth/password',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':m.csrf_token},credentials:'same-origin',body:JSON.stringify({current:${JSON.stringify(made.initial_password)},new:'Menu-probe-2026-viewer!'})}).then(r=>r.status))`), (s) => s < 300);
  await login(uname, "Menu-probe-2026-viewer!"); // a new password ends the other sessions
  await go("/");
  expect("viewer: start page", await ev(`location.pathname`), "/");
  expect("viewer: headings", await ev(heads), "Monitoring|Devices and automation");
  expect("viewer: no administration", await ev(`['/users', '/audit', '/settings', '/organizations'].filter(h => document.querySelector('#sidebar a[href="' + h + '"]')).join() || 'none'`), "none");
  expect("viewer: counting without its settings", await ev(`[...document.querySelectorAll('#sidebar a[href^="/counting"], #sidebar a[href="/reports/scheduled"]')].map(a => a.getAttribute('href')).join()`), "/counting,/counting,/counting/areas,/counting/cameras");
  await shot("menu-viewer.png", "#sidebar");
  await phone();
  await go("/counting");
  await ev(`document.querySelector('#menuBtn').click()`); await sleep(500);
  await shot("menu-phone-viewer.png");
  await send("Emulation.clearDeviceMetricsOverride");

  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
