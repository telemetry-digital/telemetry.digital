// Headless Chrome: System → VPN (spec 024). Adds a technician through the dialog (password check, configuration with
// QR code shown once), adds an access rule with a service preset, opens the generated firewall rules and, with the
// license, a site with router templates; no script errors; Slovak translation. Without a server agent that manages
// the VPN (Windows without the simulation) it checks the notice and stops. Screenshots: vpn-*.png in OUT.
//   BASE=http://127.0.0.1:8080 OUT=/tmp node tests/headless/vpn.js
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const pass = process.env.PASS || "admin1234";
const port = 9367;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + (process.env.TEMP || "/tmp") + "/cdp-profile-vpn", "--window-size=1400,1700", "about:blank"], { stdio: "ignore" });
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
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); console.log("screenshot:", name); };
  const apiCall = (method, path, body) => ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); const r=await fetch(${JSON.stringify(path)},{method:${JSON.stringify(method)},credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":me.csrf_token},body:${body ? JSON.stringify(JSON.stringify(body)) : "undefined"}}); return {status:r.status, body: await r.json().catch(()=>null)}})()`);
  await send("Page.navigate", { url: base + "/login" }); await sleep(1500);
  console.log("login:", await ev(`fetch('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},credentials:'same-origin',body:JSON.stringify({username:'admin',password:${JSON.stringify(pass)}})}).then(r=>r.status)`));
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  let v = (await apiCall("GET", "/api/v1/vpn")).body;
  if (!v || !v.agent.connected || !v.supported) {
    await send("Page.navigate", { url: base + "/server/vpn" }); await sleep(3000);
    const note = await ev(`!document.querySelector('#note').hidden && document.querySelector('#note').textContent`);
    console.log("no agent with VPN support; notice:", note);
    if (!note) problems.push("no notice without the agent");
    await shot("vpn-noagent.png");
    console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
    proc.kill(); process.exit(0);
  }
  // a configured server and a clean list for the screenshots
  console.log("configure:", (await apiCall("PUT", "/api/v1/server/wireguard", { enabled: true, listen_port: 51820, address: "10.66.0.1/24", endpoint: "vpn.example.com:51820", dns: "", reason: "headless vpn probe" })).status);
  v = (await apiCall("GET", "/api/v1/vpn")).body;
  for (const r of v.rules) await apiCall("POST", `/api/v1/vpn/rules/${r.id}/remove`, { reason: "headless cleanup" });
  for (const p of v.peers) await apiCall("POST", `/api/v1/vpn/peers/${p.id}/remove`, { reason: "headless cleanup" });
  const licensed = v.licensed;
  console.log("gateway:", (await apiCall("POST", "/api/v1/vpn/peers", { name: "gateway-hall-2", kind: "device", reason: "headless", password: pass, note: "MQTT without TLS" })).status);
  if (licensed) console.log("site:", (await apiCall("POST", "/api/v1/vpn/peers", { name: "plant-trencin", kind: "site", subnets: ["192.168.10.0/24"], reason: "headless", password: pass })).status);

  await send("Page.navigate", { url: base + "/server/vpn" }); await sleep(3500);
  // add a technician through the dialog
  await ev(`document.querySelector('#addPeerBtn').click()`); await sleep(400);
  await ev(`(()=>{const f=document.querySelector('#peerForm'); document.querySelector('#kindSeg [data-kind=user]').click(); f.name.value='technician-novak'; f.note.value='service laptop'; f.reason.value='PLC service at plant 1'; f.password.value=${JSON.stringify(pass)}; f.requestSubmit();})()`);
  await sleep(2500);
  const res = await ev(`({shown: !document.querySelector('#peerResult').hidden, qr: !!document.querySelector('#peerResult img.qr'), conf: (document.querySelector('#confText')||{}).textContent || '', err: document.querySelector('#peerErr').textContent})`);
  console.log("technician dialog: config shown", res.shown, "| QR", res.qr, "| error", JSON.stringify(res.err));
  if (!res.shown || !res.qr || !/PrivateKey = /.test(res.conf)) problems.push("configuration not shown after adding a peer");
  await shot("vpn-peer-config.png");
  await ev(`document.querySelector('#peerDlg').close()`); await sleep(1500);
  // an access rule with the MQTT preset for devices
  await ev(`document.querySelector('#addRuleBtn').click()`); await sleep(500);
  await ev(`(()=>{const f=document.querySelector('#ruleForm'); document.querySelector('#srcList input[data-st=kind][data-sv=device]').click(); f.dest_type.value='server'; f.dest_type.dispatchEvent(new Event('change')); document.querySelector('[data-preset="0"]').click(); f.reason.value='devices send MQTT'; })()`);
  await shot("vpn-rule.png");
  await ev(`document.querySelector('#ruleForm').requestSubmit()`); await sleep(2000);
  if (licensed) {
    await ev(`document.querySelector('#addRuleBtn').click()`); await sleep(500);
    await ev(`(()=>{const f=document.querySelector('#ruleForm'); f.name.value='PLC programming plant Trenčín'; document.querySelector('#srcList input[data-st=kind][data-sv=user]').click(); f.dest_type.value='site'; f.dest_type.dispatchEvent(new Event('change')); f.dest_cidr.value='192.168.10.20'; f.protocol.value='tcp'; f.ports.value='102, 502'; f.reason.value='TIA Portal access'; f.requestSubmit(); })()`);
    await sleep(2000);
  }
  const st = await ev(`({peers: document.querySelectorAll('#peers tbody tr').length, rules: document.querySelectorAll('#rules tbody tr').length, server: document.querySelector('#kvServer').textContent.replace(/\\s+/g,' ')})`);
  console.log("peers rows:", st.peers, "| rules rows:", st.rules, "|", st.server.slice(0, 200));
  if (st.rules < 1 || !/default deny active/.test(st.server)) problems.push("rule or firewall state missing");
  await shot("vpn-en.png");
  await ev(`document.querySelector('#rulesetBtn').click()`); await sleep(500);
  const fw = await ev(`document.querySelector('#fwText').textContent`);
  if (!/table inet ctrl32_vpn/.test(fw) || !/tcp dport 1883/.test(fw)) problems.push("firewall rules not shown");
  await shot("vpn-firewall.png");
  await ev(`document.querySelector('#fwDlg').close()`);
  if (licensed) {
    v = (await apiCall("GET", "/api/v1/vpn")).body;
    const site = v.peers.find((p) => p.kind === "site");
    await ev(`(()=>{document.querySelector('[data-rotate="${site.id}"]').click(); const f=document.querySelector('#rotForm'); f.reason.value='router replaced'; f.password.value=${JSON.stringify(pass)}; f.requestSubmit();})()`);
    await sleep(2500);
    await ev(`(document.querySelector('[data-tpl=mikrotik]')||{click(){}}).click()`); await sleep(300);
    const tpl = await ev(`(document.querySelector('#rotResult #tplText')||{}).textContent || ''`);
    if (!/\/interface wireguard add/.test(tpl)) problems.push("router templates missing");
    await shot("vpn-site-templates.png");
    await ev(`document.querySelector('#rotDlg').close()`);
  } else console.log("site and templates: skipped (no license)");
  // Slovak (enabled for the probe, restored afterwards)
  const langs0 = await ev(`(async()=>{const me=await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json(); window.__csrf=me.csrf_token; return await (await fetch("/api/v1/i18n/languages",{credentials:"same-origin"})).json()})()`);
  const putLangs = (enabled, dflt) => ev(`fetch("/api/v1/i18n/languages",{method:"PUT",credentials:"same-origin",headers:{"Content-Type":"application/json","X-CSRF-Token":window.__csrf},body:JSON.stringify({enabled:${JSON.stringify(enabled)},default:${JSON.stringify(dflt)},reason:"headless vpn probe"})}).then(r=>r.status)`);
  if (!langs0.enabled.includes("sk")) console.log("enable sk:", await putLangs([...langs0.enabled, "sk"], langs0.default));
  await ev(`localStorage.setItem("ctrl32.locale", "sk")`); await send("Page.navigate", { url: base + "/server/vpn" }); await sleep(3500);
  const sk = await ev(`document.querySelector('main').textContent.replace(/\\s+/g,' ')`);
  const skOK = /Pravidlá prístupu/.test(sk) && /Pridať peera|Pridať účastníka/.test(sk);
  console.log("sk:", skOK ? "translated" : "NOT translated");
  if (!skOK) problems.push("not translated to Slovak");
  await shot("vpn-sk.png");
  await ev(`localStorage.removeItem("ctrl32.locale")`);
  if (!langs0.enabled.includes("sk")) { await ev(`(async()=>{window.__csrf=(await (await fetch("/api/v1/auth/me",{credentials:"same-origin"})).json()).csrf_token})()`); console.log("restore languages:", await putLangs(langs0.enabled, langs0.default)); }
  console.log("problems:", problems.length ? JSON.stringify(problems) : "none");
  proc.kill(); process.exit(0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
