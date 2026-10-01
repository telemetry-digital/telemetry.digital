// Headless Chrome: the front page for visitors. Without a front page of its own (Front page editor not published) the
// address / leads to the sign-in page, which shows a short overview of the system next to the form (under it on a
// phone). Screenshots front-*.png go to OUT.
const { spawn } = require("child_process");
const fs = require("fs");
const http = require("http");
const chrome = process.env.CHROME || "C:/Program Files/Google/Chrome/Application/chrome.exe";
const base = process.env.BASE || "http://127.0.0.1:8080";
const out = process.env.OUT || ".";
const tmp = process.env.TEMP || require("os").tmpdir();
const port = 9379;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--no-first-run", "--remote-debugging-port=" + port, "--user-data-dir=" + tmp + "/cdp-profile-front", "--window-size=1400,900", "about:blank"], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function json(url) { return new Promise((res, rej) => http.get(url, (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(JSON.parse(d))); }).on("error", rej)); }
(async () => {
  let targets;
  for (let i = 0; i < 40; i++) { try { targets = await json(`http://127.0.0.1:${port}/json`); break; } catch (e) { await sleep(250); } }
  const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
  await new Promise((r) => (ws.onopen = r));
  let id = 0; const pending = {}; const problems = [];
  ws.onmessage = (m) => { const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.exceptionThrown") problems.push("exception: " + msg.params.exceptionDetails.text);
    if (msg.id && pending[msg.id]) { pending[msg.id](msg); delete pending[msg.id]; } };
  const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending[i] = r; ws.send(JSON.stringify({ id: i, method, params })); });
  await send("Page.enable"); await send("Runtime.enable");
  const ev = async (expr) => { const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true }); return r.result.result.value; };
  const expect = (name, got, want) => { const ok = typeof want === "function" ? want(got) : got === want; console.log((ok ? "ok   " : "FAIL ") + name + ": " + JSON.stringify(got)); if (!ok) problems.push(name + " = " + JSON.stringify(got)); };
  const shot = async (name) => { const s = await send("Page.captureScreenshot", { format: "png" }); fs.writeFileSync(out + "/" + name, Buffer.from(s.result.data, "base64")); };
  const published = await new Promise((res) => (base.startsWith("https") ? require("https") : http).get(base + "/api/v1/landing", (r) => { let d = ""; r.on("data", (c) => (d += c)); r.on("end", () => res(!!JSON.parse(d).spec)); }));
  await send("Page.navigate", { url: base + "/" }); await sleep(3000);
  if (published) {
    console.log("skip: this server publishes a front page of its own");
  } else {
    expect("visitor at / is on the sign-in page", await ev(`location.pathname`), "/login");
    expect("a short overview of the system", await ev(`document.querySelectorAll('#intro li').length`), 5);
    expect("icons drawn", await ev(`[...document.querySelectorAll('#intro .ic svg')].length`), 5);
    expect("the sign-in form is there", await ev(`!!document.querySelector('#f input[name=username]')`), true);
    await shot("front-desktop.png");
    await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
    await send("Page.navigate", { url: base + "/" }); await sleep(2500);
    expect("phone: the form comes first", await ev(`document.querySelector('#f').getBoundingClientRect().top < document.querySelector('#intro').getBoundingClientRect().top`), true);
    await shot("front-phone.png");
  }
  console.log(problems.length ? "PROBLEMS:\n" + problems.join("\n") : "no problems");
  proc.kill(); process.exit(problems.length ? 1 : 0);
})().catch((e) => { console.error(e); proc.kill(); process.exit(1); });
