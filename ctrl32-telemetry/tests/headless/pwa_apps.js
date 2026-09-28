// Headless Chrome: installable apps (spec 021). Creates a "Cameras" app, shows Settings → Apps with the create dialog,
// opens the install page (QR code, install hints for Android and iPhone), then launches the app the way the home
// screen does (display-mode standalone emulated; ?launch=1 as the fallback) and checks that the window keeps the app
// mode: start page, app name in the header, logo to the start page, menu restricted to Cameras and My account on
// every page, and "Open the full application" brings the whole menu back. Screenshots pwa-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9371;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-pwa-apps", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
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
  const until = async (expr, ms = 8000) => { for (let t = 0; t < ms; t += 250) { if (await ev(expr) === true) return true; await sleep(250); } return false; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const go = async (url, wait = 2000) => { await send("Page.navigate", { url: base + url }); await sleep(wait); };
  const topMenu = `[...document.querySelectorAll('#sidebar .side-nav > a.nav-link, #sidebar .side-nav > .group > a.nav-link')].map(a => a.getAttribute('href'))`;

  await go("/login", 1500);
  expect("login", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`), 200);
  await go("/settings/apps", 2500);
  const slug = "cams-probe-" + (Date.now() % 100000);
  const made = await ev(`ctrl32.api('POST', '/api/v1/pwa-apps', {slug: '${slug}', name: 'Cameras', short_name: 'Cameras', start_url: '/video', sections: ['cameras'], reason: 'headless probe'})`);
  expect("app created", made && made.slug, slug);
  await go("/settings/apps", 2500);
  expect("Settings → Apps lists the app", await until(`[...document.querySelectorAll('#appTable a')].some(a => a.getAttribute('href') === '/app/${slug}')`), true);
  await shot("pwa-settings-apps.png");
  await ev(`document.querySelector('#newAppBtn').click()`);
  await sleep(300);
  await ev(`document.querySelector('[data-preset="smarthome"]').click()`);
  expect("preset fills the form", await ev(`(() => { const f = document.querySelector('#appForm'); return f.start.value + ' ' + [...f.querySelectorAll('input[name=sec]:checked')].map(c => c.value).join(); })()`), "/home home,energy");
  await ev(`document.querySelector('[data-preset="cameras"]').click()`);
  expect("Cameras preset", await ev(`(() => { const f = document.querySelector('#appForm'); return f.start.value + ' ' + [...f.querySelectorAll('input[name=sec]:checked')].map(c => c.value).join(); })()`), "/video cameras");
  expect("Cameras preset: locked, no page zoom, one camera per row", await ev(`(() => { const f = document.querySelector('#appForm'); return [f.locked.checked, f.page_zoom.checked, f.camera_layout.value].join(); })()`), "true,false,single");
  await shot("pwa-settings-new-app.png");
  await ev(`document.querySelector('#appDlg').close()`);

  // install page in a normal browser tab (phone-sized)
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/app/" + slug, 2500);
  expect("install page stays (not launched)", await ev(`location.pathname`), "/app/" + slug);
  expect("app name", await ev(`document.querySelector('#appCard h1').textContent`), "Cameras");
  expect("own manifest linked", await ev(`document.querySelector('link[rel=manifest]').getAttribute('href')`), `/app/${slug}/manifest.webmanifest`);
  expect("QR code drawn", await until(`!!document.querySelector('#qr svg path')`), true);
  expect("QR names the install address", await ev(`document.querySelector('#appUrl').textContent`), base + "/app/" + slug);
  expect("Android/desktop hints", await ev(`!document.querySelector('#other').hidden && document.querySelector('#ios').hidden`), true);
  const mf = await ev(`fetch(document.querySelector('link[rel=manifest]').href).then(r => r.json())`);
  expect("manifest start and scope", mf.start_url + " " + mf.scope + " " + mf.id, `/app/${slug} / /app/${slug}`);
  await shot("pwa-install.png");
  await send("Emulation.setUserAgentOverride", { userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1" });
  await go("/app/" + slug, 2500);
  expect("iPhone: Share → Add to Home Screen", await ev(`!document.querySelector('#ios').hidden && /Add to Home Screen/.test(document.querySelector('#ios').textContent)`), true);
  await shot("pwa-install-iphone.png");
  await send("Emulation.setUserAgentOverride", { userAgent: "" });
  await send("Emulation.clearDeviceMetricsOverride");

  // launched from the home screen: display-mode standalone (fallback ?launch=1 when the browser cannot emulate it)
  // Chrome accepts but ignores an emulated display-mode, so the probe also plays an iPhone home-screen launch
  // (navigator.standalone), which the install page checks as well.
  await send("Emulation.setEmulatedMedia", { features: [{ name: "display-mode", value: "standalone" }] });
  const inj = await send("Page.addScriptToEvaluateOnNewDocument", { source: "Object.defineProperty(navigator, 'standalone', { get: () => true });" });
  await go("/app/" + slug, 3000);
  await send("Page.removeScriptToEvaluateOnNewDocument", { identifier: inj.result.identifier });
  let how = "home-screen launch";
  if ((await ev(`location.pathname`)) !== "/video") { how = "launch=1"; await go(`/app/${slug}?launch=1`, 3000); }
  expect("start page opened (" + how + ")", await ev(`location.pathname`), "/video");
  expect("app name in the header", await until(`(document.querySelector('.side-org span') || {}).textContent === 'Cameras'`), true);
  expect("logo leads to the start page", await ev(`document.querySelector('#sidebar .side-brand').getAttribute('href')`), "/video");
  expect("menu restricted to Cameras and My account", await ev(topMenu + `.join()`), "/video,/account");
  expect("way out in My account", await ev(`[...document.querySelectorAll('#sidebar a')].some(a => a.getAttribute('href') === '/?full=1')`), true);
  await shot("pwa-app-menu.png");
  await go("/home", 2500);
  expect("another page keeps the app mode", await ev(topMenu + `.join()`), "/video,/account");
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/video", 2500);
  await ev(`document.querySelector('#menuBtn').click()`);
  await sleep(500);
  expect("phone drawer restricted too", await ev(`[...document.querySelectorAll('#drawer .drawer-nav > a.nav-link:not(.sub-link)')].map(a => a.getAttribute('href')).join()`), "/video,/account");
  await shot("pwa-app-menu-phone.png");
  await send("Emulation.clearDeviceMetricsOverride");
  await send("Emulation.setEmulatedMedia", { features: [] });
  await go("/?full=1", 2500);
  expect("full application again", await ev(topMenu + `.length`), (n) => n > 5);
  expect("marker removed from the address", await ev(`location.search`), "");

  // a deleted app leaves the app mode of an open window
  await go(`/app/${slug}?launch=1`, 3000);
  expect("app mode again", await ev(topMenu + `.join()`), "/video,/account");
  expect("app deleted", await ev(`ctrl32.api('DELETE', '/api/v1/pwa-apps/${slug}?reason=headless%20probe').then(r => r.ok)`), true);
  await go("/video", 2500);
  expect("deleted app: full menu", await ev(topMenu + `.length`), (n) => n > 5);
  await go("/app/" + slug, 1500);
  expect("deleted app: install page gone", await ev(`document.querySelector('h1').textContent`), "App not found");

  // 0.61.0: a locked phone app for the cameras — no way into the full application, no page zoom, one camera per row
  const slug2 = "phone-probe-" + (Date.now() % 100000);
  const made2 = await ev(`ctrl32.api('POST', '/api/v1/pwa-apps', {slug: '${slug2}', name: 'Phone cameras', short_name: 'Cameras', start_url: '/video', sections: ['cameras'], locked: true, page_zoom: false, camera_layout: 'single', reason: 'headless probe'})`);
  expect("locked app created", made2 && [made2.slug, made2.locked, made2.page_zoom, made2.camera_layout].join(), `${slug2},true,false,single`);
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await go("/app/" + slug2, 2500);
  expect("locked: install page without the way out", await ev(`[...document.querySelectorAll('a')].some(a => a.getAttribute('href') === '/?full=1')`), false);
  await go(`/app/${slug2}?launch=1`, 3000);
  expect("locked: start page", await ev(`location.pathname`), "/video");
  expect("locked: html classes", await ev(`['app-locked', 'no-page-zoom'].map(c => document.documentElement.classList.contains(c)).join()`), "true,true");
  expect("no page zoom: viewport forbids zooming", await ev(`document.querySelector('meta[name=viewport]').content`), (v) => /user-scalable=no/.test(v) && /maximum-scale=1/.test(v));
  expect("one camera per row", await until(`document.querySelector('#liveGrid').classList.contains('cam-list')`), true);
  expect("layout switch shows the list", await ev(`document.querySelector('#camLayout [data-layout=list]').classList.contains('on')`), true);
  await shot("pwa-locked-phone.png");
  await ev(`document.querySelector('#menuBtn').click()`);
  await sleep(500);
  expect("locked: no way out in the menu", await ev(`[...document.querySelectorAll('#drawer a')].some(a => a.getAttribute('href') === '/?full=1')`), false);
  expect("locked: My account leads to Password and MFA", await ev(`[...document.querySelectorAll('#drawer .drawer-nav > a.nav-link:not(.sub-link)')].map(a => a.getAttribute('href')).join()`), "/video,/account/security");
  await shot("pwa-locked-phone-menu.png");
  await go("/devices", 3000);
  expect("locked: a page outside the app opens the start page", await ev(`location.pathname`), "/video");
  await go("/?full=1", 3000);
  expect("locked: ?full=1 does not leave the app", await ev(`location.pathname + ' ' + document.documentElement.classList.contains('app-locked')`), "/video true");
  // the viewer's own choice on this device wins over the app profile
  await ev(`document.querySelector('#camLayout [data-layout=grid]').click()`);
  expect("grid chosen on the device", await ev(`document.querySelector('#liveGrid').classList.contains('cam-list')`), false);
  await ev(`localStorage.removeItem('ctrl32.video.layout'); sessionStorage.clear()`);
  await send("Emulation.clearDeviceMetricsOverride");
  expect("locked app deleted", await ev(`ctrl32.api('DELETE', '/api/v1/pwa-apps/${slug2}?reason=headless%20probe').then(r => r.ok)`), true);

  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
