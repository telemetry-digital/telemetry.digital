// Demo plant for the operator view (spec 036) on a dev server: a waterworks with a picture tree, a faceplate type for the
// pumps, and alarms in two branches of the tree. Writes what it made to OUT (JSON) for tests/headless/operator.js.
//   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 OUT=.dev/operator-demo.json node tests/headless/operator_demo.js
const { execFileSync } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");
const base = process.env.BASE || "http://127.0.0.1:8080";
const mqtt = process.env.MQTT || "127.0.0.1:1883";
const out = process.env.OUT || ".dev/operator-demo.json";
const suf = String(Math.floor(Math.random() * 90000) + 10000);
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "opdemo-"));
const devfill = path.join(tmp, "devfill" + (process.platform === "win32" ? ".exe" : ""));
execFileSync("go", ["build", "-o", devfill, "./tools/devfill"], { stdio: "inherit" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let cookie = "", csrf = "";
async function req(method, p, body) {
  const r = await fetch(base + p, { method, headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf, Cookie: cookie }, body: body ? JSON.stringify(body) : undefined });
  const set = r.headers.getSetCookie ? r.headers.getSetCookie() : [];
  if (set.length) cookie = set.map((c) => c.split(";")[0]).join("; ");
  const text = await r.text(); let j = null; try { j = JSON.parse(text); } catch (e) { /* not JSON */ }
  if (r.status >= 400) throw new Error(`${method} ${p}: ${r.status} ${text.slice(0, 200)}`);
  return j;
}
(async () => {
  await req("POST", "/api/v1/auth/login", { username: "admin", password: "admin1234" });
  csrf = (await req("GET", "/api/v1/auth/me")).csrf_token;
  const R = "demo";
  // one demo plant at a time: the pictures of an earlier run would share the top level of the tree
  for (const d of await req("GET", "/api/v1/dashboards")) if (/^op-(plant|pumps|aeration|reservoir|p1|pump)-\d{5}$/.test(d.slug)) await req("DELETE", `/api/v1/dashboards/${d.slug}?reason=${R}`);
  const site = (await req("POST", "/api/v1/sites", { name: "Waterworks " + suf, reason: R })).id;
  const asset = async (name) => (await req("POST", "/api/v1/assets", { site_id: site, type: "machine", name, reason: R })).id;
  const ds = async (a, key, quantity, unit) => (await req("POST", "/api/v1/datastreams", { asset_id: a, key, quantity, unit, kind: "gauge", reason: R })).id;
  const A = { res: await asset("Reservoir"), p1: await asset("Pump P1"), p2: await asset("Pump P2"), b1: await asset("Blower B1") };
  const D = { level: await ds(A.res, "level", "level", "m"), pressure: await ds(A.b1, "pressure", "pressure", "kPa") };
  for (const p of ["p1", "p2"]) { D[p + "run"] = await ds(A[p], "run", "state", ""); D[p + "speed"] = await ds(A[p], "speed", "speed", "rpm"); D[p + "current"] = await ds(A[p], "current", "current", "A"); }
  const dev = {}, tok = {};
  for (const [k, list] of [["a", [D.level, D.pressure, D.p1run, D.p1speed, D.p1current]], ["b", [D.p2run, D.p2speed, D.p2current]]]) {
    dev[k] = (await req("POST", "/api/v1/devices", { external_id: `opdemo-${k}-${suf}`, name: "PLC " + k.toUpperCase(), reason: R })).id;
    tok[k] = (await req("POST", `/api/v1/devices/${dev[k]}/token`, { reason: R })).token;
    for (const d of list) await req("POST", `/api/v1/devices/${dev[k]}/datastreams`, { datastream_id: d, reason: R });
  }
  const pub = (k, key, v) => execFileSync(devfill, ["-addr", mqtt, "-user", `opdemo-${k}-${suf}`, "-token", tok[k], "-key", key, "-start", String(v), "-step", "0", "-n", "1", "-every", "1"], { stdio: "ignore" });
  await req("POST", "/api/v1/alarm-rules", { datastream_id: D.p1current, type: "high", action: 12, hysteresis: 0.5, reason: R });
  await req("POST", "/api/v1/alarm-rules", { datastream_id: D.pressure, type: "high", warning: 60, hysteresis: 1, reason: R });
  await req("POST", "/api/v1/alarm-rules", { datastream_id: D.level, type: "low", action: 1, hysteresis: 0.1, reason: R });

  // ---- pictures ------------------------------------------------------------------------------------------
  const S = { plant: "op-plant-" + suf, pumps: "op-pumps-" + suf, aeration: "op-aeration-" + suf, reservoir: "op-reservoir-" + suf, p1: "op-p1-" + suf, pump: "op-pump-" + suf };
  const text = (id, x, y, w, t, extra) => Object.assign({ id, type: "text", x, y, w, h: 34, text: t, style: { font: 20, color: "#1f2937" } }, extra || {});
  const title = (t) => text("title", 40, 24, 700, t, { style: { font: 34, bold: true, color: "#111827" } });
  const state = { prop: "state", map: [{ op: "==", v: 1, to: "run" }, { op: "==", v: 0, to: "stop" }] };
  const pump = (id, x, y, p, name) => [
    { id, type: "symbol", symbol: "pump_centrifugal", x, y, w: 150, h: 130, tag: D[p + "run"], name, dyn: [state], on: [{ act: "popup", target: S.pump }] },
    text(id + "n", x, y + 140, 150, name, { style: { font: 20, bold: true, align: "middle" } }),
    text(id + "c", x - 20, y + 172, 190, "{value} {unit}", { tag: D[p + "current"], decimals: 1, style: { font: 20, align: "middle" } }),
  ];
  const tank = (x, y) => [
    { id: "tank", type: "symbol", symbol: "tank_vertical", x, y, w: 220, h: 320, tag: D.level, name: "Reservoir", dyn: [{ prop: "level", lin: [0, 5, 0, 1] }] },
    text("tankv", x, y + 330, 220, "{value} {unit}", { tag: D.level, decimals: 2, style: { font: 22, bold: true, align: "middle" } }),
  ];
  const blower = (x, y) => [
    { id: "blower", type: "symbol", symbol: "blower", x, y, w: 160, h: 130, tag: D.pressure, name: "Blower B1" },
    text("blowern", x, y + 140, 160, "Blower B1", { style: { font: 20, bold: true, align: "middle" } }),
    text("blowerv", x - 20, y + 172, 200, "{value} {unit}", { tag: D.pressure, decimals: 1, style: { font: 20, align: "middle" } }),
  ];
  const pipe = (id, pts) => ({ id, type: "pipe", x: 0, y: 0, w: 0, h: 0, points: pts, style: { medium: "#3b82f6", sw: 10 } });
  const spec = (elements, more, w, h) => Object.assign({ version: 1, kind: "mimic", layout: { type: "canvas", width: w || 1600, height: h || 800, bg: "#e5e7eb" }, range: "6h", refresh_s: 10, widgets: [], elements }, more || {});
  const save = (slug, name, sp) => req("POST", "/api/v1/dashboards", { slug, name, spec: sp, reason: R });
  await save(S.plant, "Waterworks", spec([title("Waterworks — overview"), ...tank(120, 200), pipe("pi1", [[340, 440], [620, 440]]), ...pump("p1", 620, 370, "p1", "Pump P1"), pipe("pi2", [[340, 480], [560, 480], [560, 640], [620, 640]]), ...pump("p2", 620, 570, "p2", "Pump P2").map((e) => (e.y -= 0, e)),
    pipe("pi3", [[770, 440], [1100, 440]]), ...blower(1100, 370),
    text("toair", 1090, 300, 300, "Go to Aeration ›", { style: { font: 20, bold: true, color: "#1d4ed8" }, on: [{ act: "screen", target: S.aeration }] })], { nav: { order: 0 } }));
  await save(S.pumps, "Pumping station", spec([title("Pumping station"), ...pump("p1", 300, 260, "p1", "Pump P1"), text("p1s", 280, 470, 190, "{value} {unit}", { tag: D.p1speed, decimals: 0, style: { font: 20, align: "middle" } }),
    ...pump("p2", 760, 260, "p2", "Pump P2"), text("p2s", 740, 470, 190, "{value} {unit}", { tag: D.p2speed, decimals: 0, style: { font: 20, align: "middle" } })], { nav: { parent: S.plant, order: 10, label: "Pumps" } }));
  await save(S.aeration, "Aeration", spec([title("Aeration"), ...blower(400, 280)], { nav: { parent: S.plant, order: 20 } }));
  await save(S.reservoir, "Reservoir", spec([title("Reservoir"), ...tank(400, 180)], { nav: { parent: S.plant, order: 30 } }));
  await save(S.p1, "Pump P1 — detail", spec([title("Pump P1 — detail"), ...pump("p1", 300, 260, "p1", "Pump P1")], { nav: { parent: S.pumps, order: 1, label: "Pump P1" } }));
  // the faceplate type: drawn for Pump P1, opened for any pump
  const row = (id, y, label, tag, dec) => [text(id + "l", 200, y, 150, label, { style: { font: 18, color: "#6b7280" } }), text(id, 350, y, 150, "{value} {unit}", { tag, decimals: dec, style: { font: 20, bold: true, align: "end" } })];
  const button = (id, x, label, color, value) => [
    { id, type: "rect", x, y: 216, w: 130, h: 44, style: { fill: color, radius: 6 }, device: dev.a, key: "run", on: [{ act: "command", key: "run", value, confirm: true }] },
    text(id + "t", x, 221, 130, label, { style: { font: 20, bold: true, color: "#ffffff", align: "middle" }, device: dev.a, key: "run", on: [{ act: "command", key: "run", value, confirm: true }] }),
  ];
  await save(S.pump, "Pump", spec([
    { id: "sym", type: "symbol", symbol: "pump_centrifugal", x: 30, y: 40, w: 140, h: 120, tag: D.p1run, dyn: [state], on: [{ act: "trend" }] },
    ...row("speed", 40, "Speed", D.p1speed, 0), ...row("current", 84, "Current", D.p1current, 1), ...row("run", 128, "Running", D.p1run, 0),
    ...button("start", 200, "Start", "#16a34a", 1), ...button("stop", 350, "Stop", "#dc2626", 0),
  ], { faceplate: { asset: A.p1 } }, 600, 300));

  // ---- values and alarms: Pump P1 over current (unacknowledged), blower pressure high (acknowledged, still active) ----
  pub("a", "level", 3.2); pub("a", "pressure", 48); pub("a", "run", 1); pub("a", "speed", 1465); pub("a", "current", 9.4);
  pub("b", "run", 0); pub("b", "speed", 0); pub("b", "current", 0.2);
  await sleep(1200);
  pub("a", "pressure", 65.5); await sleep(1200);
  const pa = (await req("GET", "/api/v1/alarms?datastream_id=" + D.pressure))[0];
  await req("POST", `/api/v1/alarms/${pa.id}/ack`, { reason: "Seen, filter is being cleaned" });
  pub("a", "current", 13.8); await sleep(1200);
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, JSON.stringify({ suf, site, slugs: S, assets: A, datastreams: D, devices: dev }, null, 1));
  fs.rmSync(tmp, { recursive: true, force: true });
  console.log("operator demo ready: " + base + "/operator/" + S.plant);
})().catch((e) => { console.error(e.message); process.exit(1); });
