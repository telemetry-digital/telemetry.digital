// Headless Chrome: Cameras → Camera management, the camera list (spec 028). The export buttons are there; "With
// passwords…" opens the confidential dialog, refuses a wrong password and downloads with the right one; "Import…"
// checks a file chosen in the dialog and shows the preview (new, update, error rows and counts), keeps the Import
// button disabled while a row has an error, and applies a correct file with a reason; the camera note in the camera
// form; Slovak translation; no script errors. Two throwaway cameras are created and removed again.
//   BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/camera_inventory.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const pass = process.env.PASS || "admin1234";
const port = 9373;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-caminv", "--window-size=1400,1000", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text + " " + ((msg.params.exceptionDetails.exception || {}).description || "").split("\n").slice(0, 2).join(" / "));
    if (msg.method === "Page.javascriptDialogOpening") send("Page.handleJavaScriptDialog", { accept: false });
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); console.log("screenshot:", name); };
  const apiCall = (method, path, body) => ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); const r=await fetch(${JSON.stringify(path)},{method:${JSON.stringify(method)},credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":me.csrf_token},body:${body ? JSON.stringify(JSON.stringify(body)) : "undefined"}}); return {status:r.status, body: await r.json().catch(()=>null)}})()`);
  const expect = (what, ok, detail) => { console.log((ok ? "ok   " : "FAIL ") + what + (detail !== undefined ? " — " + JSON.stringify(detail) : "")); if (!ok) problems.push(what); };
  // choose a file in the import dialog as a person would (DataTransfer into the file input)
  const choose = (csv) => ev(`(()=>{const i=document.querySelector('#impForm').file; const dt=new DataTransfer(); dt.items.add(new File([${JSON.stringify(csv)}], 'cameras.csv', {type:'text/csv'})); i.files=dt.files; i.dispatchEvent(new Event('change')); return i.files.length})()`);

  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:${JSON.stringify(pass)}})}).then(r=>r.status)`));
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  const suf = String(Date.now() % 100000);
  const a = (await apiCall("POST", "/api/v1/video/cameras", { name: "HL gate " + suf, url: "rtsp://192.168.88.10:554/Streaming/Channels/101", username: "admin", password: "pw-" + suf, enabled: false, reason: "headless" })).body;
  const b = (await apiCall("POST", "/api/v1/video/cameras", { name: "HL dock " + suf, url: "rtsp://192.168.88.11/live", enabled: false, reason: "headless" })).body;
  if (!a || !b || !a.id || !b.id) { console.log("cameras not created", a, b); proc.kill(); process.exit(1); }

  await send("Page.navigate", { url: base + "/video/manage" }); await sleep(3000);
  const bar = await ev(`(()=>{const b=document.querySelector('#invBar'); return b && b.offsetParent ? [...b.querySelectorAll('a,button')].filter(e=>!e.hidden).map(e=>e.textContent.trim()) : null})()`);
  expect("camera list buttons", JSON.stringify(bar) === JSON.stringify(["Excel", "CSV", "PDF", "With passwords…", "Import…"]), bar);
  const href = await ev(`document.querySelector('#invCsv').getAttribute('href')`);
  const csv = await ev(`fetch(${JSON.stringify(href)}).then(r=>r.text())`);
  expect("the CSV link downloads the list", csv.includes("HL gate " + suf) && !csv.includes("pw-" + suf), csv.length);

  // the confidential export: a wrong password is refused, the right one downloads (the download itself is a blob)
  await ev(`document.querySelector('#invCred').click()`); await sleep(500);
  expect("confidential dialog open", await ev(`document.querySelector('#credDlg').open`));
  await ev(`(()=>{const f=document.querySelector('#credForm'); f.format.value='csv'; f.reason.value='headless handover'; f.password.value='wrong-password'; f.requestSubmit();})()`); await sleep(1500);
  const err = await ev(`document.querySelector('#credErr').textContent`);
  expect("wrong password refused in the dialog", /wrong password/.test(err), err);
  await shot("caminv-credentials.png");
  await ev(`(()=>{window.__saved=null; const o=URL.createObjectURL; URL.createObjectURL=(b)=>{window.__saved=b; return o.call(URL,b)}; const f=document.querySelector('#credForm'); f.password.value=${JSON.stringify(pass)}; f.requestSubmit();})()`); await sleep(2000);
  if (/too many/.test(await ev(`document.querySelector('#credErr').textContent`))) { // exports of an earlier run: one a minute
    console.log("rate limited by an earlier run: waiting a minute"); await sleep(61000);
    await ev(`(()=>{const f=document.querySelector('#credForm'); f.password.value=${JSON.stringify(pass)}; f.requestSubmit();})()`); await sleep(2000);
  }
  const saved = await ev(`window.__saved ? window.__saved.text() : null`);
  expect("export with passwords downloaded", !!saved && saved.includes("pw-" + suf) && saved.includes("CONFIDENTIAL"), saved && saved.length);
  expect("dialog closed after the export", !(await ev(`document.querySelector('#credDlg').open`)));

  // the import: a file with an update (ip move), a new camera, an unchanged row and an error
  await ev(`document.querySelector('#invImport').click()`); await sleep(500);
  const bad = `id;name;ip;make;retention_days\n${a.id};HL gate ${suf};10.88.0.10;;\n;HL new ${suf};10.88.0.20;hikvision;\n;HL dock ${suf};;;\n;HL broken ${suf};;;0\n`;
  console.log("file chosen:", await choose(bad)); await sleep(2500);
  const prev = await ev(`({rows:[...document.querySelectorAll('#impRows tbody tr')].map(r=>r.children[1].textContent.trim().split(' ')[0]), counts: document.querySelector('#impCounts').textContent.replace(/\\s+/g,' ').trim(), disabled: document.querySelector('#impApply').disabled, btn: document.querySelector('#impApply').textContent, info: document.querySelector('#impInfo').textContent})`);
  expect("preview rows", JSON.stringify(prev.rows) === JSON.stringify(["update", "new", "unchanged", "error"]), prev.rows);
  expect("counts shown", /new: 1/.test(prev.counts) && /update: 1/.test(prev.counts) && /error: 1/.test(prev.counts), prev.counts);
  expect("Import disabled while a row has an error", prev.disabled && /Correct the rows/.test(prev.btn), prev.btn);
  await shot("caminv-preview-error.png");
  await choose(bad.split("\n").slice(0, 4).join("\n") + "\n"); await sleep(2500);
  const ok = await ev(`({disabled: document.querySelector('#impApply').disabled, btn: document.querySelector('#impApply').textContent, changes: document.querySelector('#impRows tbody tr td:last-child').textContent})`);
  expect("Import (2) enabled", !ok.disabled && /Import \(2\)/.test(ok.btn), ok.btn);
  expect("changed settings listed", /url/.test(ok.changes) && /sub_url|onvif_url|url/.test(ok.changes), ok.changes);
  await ev(`(()=>{const f=document.querySelector('#impForm'); f.reason.value='headless import'; f.requestSubmit();})()`); await sleep(3000);
  const after = (await apiCall("GET", `/api/v1/video/cameras/${a.id}`)).body;
  expect("import applied (ip moved)", after && after.url === "rtsp://10.88.0.10:554/Streaming/Channels/101", after && after.url);
  const list = (await apiCall("GET", "/api/v1/video/cameras")).body || [];
  const created = list.find((c) => c.name === "HL new " + suf);
  expect("new camera with the make's addresses", created && created.url === "rtsp://10.88.0.20:554/Streaming/Channels/101", created && created.url);
  await ev(`document.querySelector('#impDlg').close()`);

  // the camera note in the form
  await ev(`document.querySelector('[data-cedit="${a.id}"]') && document.querySelector('[data-cedit="${a.id}"]').click()`); await sleep(800);
  expect("note field in the camera form", await ev(`!!document.querySelector('#camForm').note`));
  await ev(`document.querySelector('#camDlg').close()`);

  // Slovak
  const langs0 = await ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); window.__csrf=me.csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  const putLangs = (enabled, dflt) => ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify(enabled)},default:${JSON.stringify(dflt)},reason:"headless camera inventory"})}).then(r=>r.status)`);
  if (!langs0.enabled.includes("sk")) console.log("enable sk:", await putLangs([...langs0.enabled, "sk"], langs0.default));
  await ev(`localStorage.setItem("ctrl32.locale", "sk")`); await send("Page.navigate", { url: base + "/video/manage" }); await sleep(3500);
  await ev(`document.querySelector('#invImport').click()`); await sleep(600);
  const sk = await ev(`document.querySelector('main').textContent.replace(/\\s+/g,' ')`);
  expect("Slovak", /Zoznam kamier/.test(sk) && /S heslami/.test(sk) && /Import kamier/.test(sk) && /Pridať chýbajúce kamery/.test(sk));
  await shot("caminv-sk.png");
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  if (!langs0.enabled.includes("sk")) { await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`); console.log("restore languages:", await putLangs(langs0.enabled, langs0.default)); }

  for (const c of [a, b, created].filter(Boolean)) await apiCall("DELETE", `/api/v1/video/cameras/${c.id}?reason=headless%20cleanup`);
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
