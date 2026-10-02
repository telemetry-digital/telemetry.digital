// Headless Chrome: System → License shows the activation with the order number (form, buy link, the stored order when
// there is one), the offline key form, no script errors, Slovak translation; screenshots in English and Slovak.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const port = 9361;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + process.env.TEMP + "/cdp-profile-license", "--window-size=1300,1500", "about:blank"], { stdio: "ignore" });
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
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.exceptionDetails ? { exception: r.result.exceptionDetails.text } : r.result.result.value; };
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:'admin1234'})}).then(r=>r.status)`));
  await send("Page.navigate", { url: base + "/server/license" }); await sleep(3500);
  const st = await ev(`({form: !!document.querySelector('#licActForm input[name=order]'), key: !!document.querySelector('#licForm textarea[name=token]'),
    buy: document.querySelector('#licBuy').getAttribute('href'), host: document.querySelector('#licActHost').textContent,
    kv: document.querySelector('#kvLic').textContent.replace(/\\s+/g,' ')})`);
  console.log("activation form:", st.form, "| offline key form:", st.key, "| buy link:", st.buy, "|", st.host);
  console.log("status:", st.kv.slice(0, 300));
  if (!st.form || !st.key || !/^https?:\/\//.test(st.buy || "")) problems.push("license page incomplete");
  // 0.66.0: the camera count (of 4 without a licence, or no limit) and the included updates
  if (!/Cameras\s*(\d+ of 4 without a licence|\d+ · no limit with the licence)/.test(st.kv)) problems.push("no camera count on the license page");
  if (/Status\s*valid/.test(st.kv) && !/Updates\s*(Updates included until|Updates ended on|all versions)/.test(st.kv)) problems.push("no updates row for a valid license");
  const intro = await ev(`document.querySelector('section[data-section=license]').textContent.replace(/\\s+/g,' ')`);
  if (!/including up to 4 cameras/.test(intro) || !/keeps running/.test(intro)) problems.push("licence model text missing");
  if (/perpetual|forever|bought once/i.test(intro)) problems.push("old licence wording on the page");
  const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true }); fs.writeFileSync(out + "/license-en.png", Buffer.from(s.result.data, "base64"));
  await send("Page.navigate", { url: base + "/video/manage" }); await sleep(3500);
  const cl = await ev(`({hidden: document.querySelector('#camLimit').hidden, text: document.querySelector('#camLimit').textContent})`);
  console.log("camera management:", cl.hidden ? "no limit shown (licensed)" : cl.text);
  if (!cl.hidden && !/Cameras: \d+ of 4 without a licence/.test(cl.text)) problems.push("camera limit text: " + cl.text);
  const s3 = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/license-cameras-en.png", Buffer.from(s3.result.data, "base64"));
  // Slovak must be enabled for the organization; enable it for the probe and restore the setting afterwards
  const langs0 = await ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); window.__csrf=me.csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  const putLangs = (enabled, dflt) => ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify(enabled)},default:${JSON.stringify(dflt)},reason:"headless license probe"})}).then(r=>r.status)`);
  if (!langs0.enabled.includes("sk")) console.log("enable sk:", await putLangs([...langs0.enabled, "sk"], langs0.default));
  await ev(`localStorage.setItem("ctrl32.locale", "sk")`); await send("Page.navigate", { url: base + "/server/license" }); await sleep(3500);
  const sk = await ev(`document.querySelector('section[data-section=license]').textContent.replace(/\\s+/g,' ')`);
  console.log("sk:", /Aktivácia číslom objednávky/.test(sk) && /Kúpiť licenciu/.test(sk) ? "translated" : "NOT translated");
  if (!/Aktivácia číslom objednávky/.test(sk)) problems.push("not translated to Slovak");
  if (!/vrátane najviac 4 kamier/.test(sk) || !/Licencia zahŕňa aktualizácie/.test(sk) || !/(zo 4 bez licencie|bez obmedzenia s licenciou)/.test(sk)) problems.push("licence model not translated to Slovak");
  const s2 = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true }); fs.writeFileSync(out + "/license-sk.png", Buffer.from(s2.result.data, "base64"));
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  if (!langs0.enabled.includes("sk")) { await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`); console.log("restore languages:", await putLangs(langs0.enabled, langs0.default)); }
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
