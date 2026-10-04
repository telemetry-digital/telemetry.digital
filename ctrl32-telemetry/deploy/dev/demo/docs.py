#!/usr/bin/env python3
"""Demo for the product documentation: fills the screens the other demo scripts leave empty or thin.

  docs.py setup        cold chain (medicine fridges, 30 days), energy meters (13 months), alarm rules with alarms and
                       incidents, notification channels and escalation, automation rules, flows, smart-home accounts,
                       a display, firmware and a campaign, users and a custom role, an API token, a report template and
                       stored reports, a PWA app, the "overview" and "showcase" starter dashboards, the overviews with
                       filters (all sensors, energy, cold chain; 0.68.0), a cold chain drill-down with entity aliases,
                       key filters and dashboard states (0.73.0), the asset tree of the pharmacy with places on the map
                       (asset and site pages, 0.74.0), and the VPN (a site,
                       a technician, a gateway and access rules; needs the agent's VPN simulation and a licence)
                       ; 0.67.0: keys without a datastream and status of a logger, a sensor fault, annotations, a
                       signed report and a signed audit review (records)
  docs.py counting     only object counting: the main building of the Nitra plant (three entrances, 30 days of people
                       counts), the car park (cars, trucks, a speed section on the plant road), a dashboard, scheduled
                       e-mail reports and cameras with counting lines (CTRL32_DEMO_DETECTOR=1 switches the detector on)
  docs.py vpn          only the VPN part (phases 1 to 3: an organization administrator olivia.grant, four eyes for
                       technicians' access, an approved and a pending access grant)
  docs.py live         keeps the cold chain and the energy meters current (a sample every 60 s)
  docs.py mqtt-creds   prints "user pass" of the smart-home MQTT accounts (used by dev.sh to start the simulators)

Idempotent: what is recorded in .dev/docs.json (or found by name) is skipped, so setup can be run again.
Nothing sensitive is switched on (no audio, no unmasked video, AI access stays as it is)."""
import json, math, os, random, subprocess, sys, time, uuid
from common import B, call, login, state_path
import common, urllib.request

STATE = state_path("docs.json")
R = "documentation demo"
NOW = int(time.time())
st = json.load(open(STATE, encoding="utf-8")) if os.path.exists(STATE) else {}


def save():
    json.dump(st, open(STATE, "w", encoding="utf-8"), ensure_ascii=False, indent=1)


def once(key):
    """Decorator: runs a setup step only when its key is not in the state yet."""
    def deco(fn):
        def run(*a):
            if key in st:
                print("skip", key); return st[key]
            st[key] = fn(*a); save(); print("done", key); return st[key]
        return run
    return deco


def raw(m, path, data=None, ctype=None):
    """A request with a body other than JSON (multipart) or a response other than JSON (PDF)."""
    h = {"X-CSRF-Token": common._csrf}
    if ctype: h["Content-Type"] = ctype
    req = urllib.request.Request(B + path, method=m, headers=h, data=data)
    with common._op.open(req) as r:
        return r.read()


def by_name(path, name, key="name"):
    return next((x for x in call("GET", path) or [] if x.get(key) == name), None)


def send(token, samples):
    for i in range(0, len(samples), 1000):
        call("POST", "/api/v1/d/telemetry", samples[i:i + 1000], token=token)


def device(ext, name, transport="http", loc=None):
    d = next((x for x in call("GET", "/api/v1/devices") if x["external_id"] == ext), None)
    if not d:
        d = call("POST", "/api/v1/devices", {"external_id": ext, "name": name, "transport": transport, "reason": R})
    if loc:
        call("PATCH", f"/api/v1/devices/{d['id']}", {"location": {"lat": loc[0], "lon": loc[1]}, "reason": R})
    tok = call("POST", f"/api/v1/devices/{d['id']}/token", {"reason": R})["token"]
    return d["id"], tok


def asset(site, typ, name, attrs=None):
    a = next((x for x in call("GET", "/api/v1/assets") if x["name"] == name), None)
    return a["id"] if a else call("POST", "/api/v1/assets", {"site_id": site, "type": typ, "name": name, "attributes": attrs or {}, "reason": R})["id"]


def named(path, name, body):
    x = by_name(path, name)
    return x["id"] if x else call("POST", path, dict(body, name=name, reason=R))["id"]


def partitions():
    """Month partitions of core.observation for the history. From 0.66.0 the server creates the month of every reading
    itself (migration 0046), so this runs only for an older server: with CTRL32_DB_ADMIN_URL, or CTRL32_DB_NAME on the
    development container."""
    q = "SELECT core.ensure_month_partitions('core.observation', (current_date - interval '14 months')::date, 14)"
    url, name = os.environ.get("CTRL32_DB_ADMIN_URL"), os.environ.get("CTRL32_DB_NAME")
    if not url and not name:
        print("partitions: created by the server (0.66.0+)"); return
    cmd = ["psql", "-q", url, "-c", q] if url else ["docker", "exec", "ctrl32-pg-dev", "psql", "-q", "-U", "ctrl32", "-d", name, "-c", q]
    try: subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL)
    except Exception as x: raise SystemExit(f"cannot create month partitions for the history ({x}); set CTRL32_DB_ADMIN_URL")


def stream(asset, dev, key, quantity, unit, kind="gauge", interval=""):
    old = next((d for d in call("GET", "/api/v1/datastreams") if d["asset_id"] == asset and d["key"] == key), None)
    if old: return old["id"]
    body = {"asset_id": asset, "key": key, "quantity": quantity, "unit": unit, "kind": kind, "reason": R}
    if interval: body["expected_interval"] = interval
    ds = call("POST", "/api/v1/datastreams", body)["id"]
    call("POST", f"/api/v1/devices/{dev}/datastreams", {"datastream_id": ds, "reason": R})
    return ds


# ---------------------------------------------------------------------------------------------- cold chain
def fridge_temp(name, t, rng, excursion_until=0):
    """Temperature of a fridge (or freezer) at unix time t: compressor cycle, defrost every 6 h, door openings."""
    if name == "freezer":
        v = -20 + 0.8 * math.sin(t / 1500) + rng.uniform(-0.3, 0.3)
        if (t // 86400) % 9 == 4 and 9 <= (t / 3600) % 24 < 10: v = -14.2 + rng.uniform(-0.4, 0.4)  # long door opening
        if t > NOW - 3 * 3600 and t < excursion_until: v = -17.4 + rng.uniform(-0.2, 0.2)  # warning band now
        return round(v, 2)
    base = (4.8 if name == "f1" else 5.2) + 0.35 * math.sin(t // 86400 * 1.7 + (0 if name == "f1" else 2))
    v = base + 0.6 * math.sin(t / 1200) + rng.uniform(-0.15, 0.15)
    if (t / 3600) % 6 < 0.33: v += 1.3  # defrost
    if 7 <= (t / 3600) % 24 < 17 and rng.random() < 0.05: v += rng.uniform(0.5, 1.6)  # door
    day = (t // 86400) % 30
    if name == "f2" and day == 11 and 13 <= (t / 3600) % 24 < 14: v = 9.1 + rng.uniform(-0.3, 0.3)  # excursion
    if name == "f2" and day == 23 and 3 <= (t / 3600) % 24 < 4: v = 1.6 + rng.uniform(-0.2, 0.2)  # too cold
    if name == "f2" and NOW - 2 * 3600 < t < excursion_until: v = 8.4 + rng.uniform(-0.15, 0.15)  # door left open now
    return round(v, 2)


def room(t, rng):
    h = (t / 3600) % 24
    return round(21 + 1.6 * math.sin((h - 9) / 24 * 2 * math.pi) + rng.uniform(-0.2, 0.2), 2), round(46 + 7 * math.sin(t / 30000) + rng.uniform(-1, 1), 1)


@once("cold")
def cold_chain():
    sites = call("GET", "/api/v1/sites")
    site = next((s for s in sites if s["name"] in ("Demo site", "Nitra pharmacy – medicine store")), None)
    if site:
        call("PATCH", f"/api/v1/sites/{site['id']}", {"name": "Nitra pharmacy – medicine store", "address": "12 Station Road, Nitra", "reason": R})
        sid = site["id"]
    else:
        sid = call("POST", "/api/v1/sites", {"name": "Nitra pharmacy – medicine store", "address": "12 Station Road, Nitra", "reason": R})["id"]
    assets = {a["name"]: a for a in call("GET", "/api/v1/assets")}
    ren = {"Fridge 1": "Medicine fridge 1", "Room 1": "Medicine store – room"}
    for old, new in ren.items():
        if old in assets:
            call("PATCH", f"/api/v1/assets/{assets[old]['id']}", {"name": new, "reason": R})
            assets[new] = assets.pop(old)
    dss = {(d.get("asset"), d["key"]): d["id"] for d in call("GET", "/api/v1/datastreams")}
    # the seed device esp32-demo feeds fridge 1 and the room
    esp = next(x for x in call("GET", "/api/v1/devices") if x["external_id"] == "esp32-demo")
    call("PATCH", f"/api/v1/devices/{esp['id']}", {"name": "Temperature logger – store (ESP32)", "location": {"lat": 48.3076, "lon": 18.0858}, "reason": R})
    esp_tok = call("POST", f"/api/v1/devices/{esp['id']}/token", {"reason": R})["token"]
    f1 = dss.get(("Medicine fridge 1", "temp")) or dss.get(("Fridge 1", "temp"))
    rt = dss.get(("Medicine store – room", "temp")) or dss.get(("Room 1", "temp"))
    rh = dss.get(("Medicine store – room", "rh")) or dss.get(("Room 1", "rh"))
    a2 = asset(sid, "fridge", "Medicine fridge 2", {"storage_class": "2-8C", "model": "Liebherr MKv 3910"})
    a3 = asset(sid, "freezer", "Vaccine freezer", {"storage_class": "-25..-15C"})
    d2, t2 = device("logger-t2", "Temperature logger T2", loc=(48.3079, 18.0861))
    d3, t3 = device("logger-t3", "Temperature logger T3", loc=(48.3073, 18.0866))
    f2 = stream(a2, d2, "temp", "temperature", "°C", interval="15 minutes")
    fz = stream(a3, d3, "temp", "temperature", "°C", interval="15 minutes")
    call("PUT", f"/api/v1/devices/{d2}/attributes", {"attributes": {"interval_s": 900, "probe": "PT1000 in glycol"}, "reason": R})
    # notification channels and an escalation policy (not attached to the rules: without an SMTP server every alarm
    # would open an "alarm not delivered" incident), then alarm rules before the history so it raises and clears alarms
    ch1 = named("/api/v1/notification-channels", "Pharmacist – e-mail", {"kind": "email", "settings": {"recipients": ["pharmacist@nitra-pharmacy.example"]}})
    ch2 = named("/api/v1/notification-channels", "Dispatch centre – webhook", {"kind": "webhook", "settings": {"url": "https://dispatch.example/hooks/ctrl32", "secret": uuid.uuid4().hex}})
    pol = named("/api/v1/escalation-policies", "Cold chain – escalation", {"steps": [{"after_s": 0, "channel_id": ch1}, {"after_s": 900, "channel_id": ch2}]})
    have = {(r["datastream_id"], r["type"]) for r in call("GET", "/api/v1/alarm-rules")}
    for ds, typ, warn, act in [(f2, "high", 7.5, 8), (f2, "low", 2.5, 2), (fz, "high", -18, -15)]:
        if (ds, typ) not in have:
            call("POST", "/api/v1/alarm-rules", {"datastream_id": ds, "type": typ, "warning": warn, "action": act, "hysteresis": 0.2, "delay_seconds": 0, "reason": R})
    if (f2, "comm_loss") not in have:
        call("POST", "/api/v1/alarm-rules", {"datastream_id": f2, "type": "comm_loss", "delay_seconds": 3600, "reason": R})
    # the room temperature gets a logger of its own (the seed device carries the fridge and the room humidity)
    d0, t0 = device("logger-room", "Logger – store room")
    if not any(x["datastream_id"] == rt for x in call("GET", f"/api/v1/devices/{d0}")["datastreams"]):
        call("POST", f"/api/v1/devices/{d0}/datastreams", {"datastream_id": rt, "reason": R})
    # 30 days, a sample every 15 minutes, sent as the loggers' own time-stamped readings (quality ok; with "bf": 1
    # they would be stored as backfilled and every point of a chart would be marked)
    until = NOW + 2 * 3600
    start = (NOW - 30 * 86400) // 900 * 900
    rng = random.Random(7)
    s0, s1, s2, s3 = [], [], [], []
    for t in range(start, NOW, 900):
        a, b = room(t, rng)
        s0.append({"ts": t, "v": {"temp": a}})
        s1.append({"ts": t, "v": {"temp": fridge_temp("f1", t, rng), "rh": b}})
        s2.append({"ts": t, "v": {"temp": fridge_temp("f2", t, rng, until)}})
        s3.append({"ts": t, "v": {"temp": fridge_temp("freezer", t, rng, until)}})
    send(t0, s0)
    send(esp_tok, s1)
    send(t2, s2)
    send(t3, s3)
    return {"site": sid, "f1": f1, "f2": f2, "fz": fz, "rt": rt, "rh": rh, "esp_token": esp_tok, "t0": t0, "t2": t2, "t3": t3,
            "dev2": d2, "esp": esp["id"], "ch1": ch1, "ch2": ch2, "policy": pol, "excursion_until": until}


@once("tree")
def asset_tree():
    """Asset and site pages (0.74.0): the fridges and the freezer stand in the store room (child assets of it), the
    pharmacy, the Nitra plant and the store room have a place on the map, the room has attributes."""
    c = st["cold"]
    assets = {a["name"]: a for a in call("GET", "/api/v1/assets")}
    call("PATCH", f"/api/v1/sites/{c['site']}", {"location": {"lat": 48.3076, "lon": 18.0858}, "reason": R})
    plant = next((s for s in call("GET", "/api/v1/sites") if s["name"] == "Nitra plant"), None)
    if plant:
        call("PATCH", f"/api/v1/sites/{plant['id']}", {"location": {"lat": 48.3311, "lon": 18.1189}, "reason": R})
    room = assets.get("Medicine store – room")
    if not room:
        return {"room": ""}
    attrs = dict(room.get("attributes") or {}, floor="ground floor", area_m2=24, responsible="pharmacist on duty")
    call("PATCH", f"/api/v1/assets/{room['id']}", {"attributes": attrs, "location": {"lat": 48.30765, "lon": 18.08585}, "reason": R})
    for n in ("Medicine fridge 1", "Medicine fridge 2", "Vaccine freezer"):
        if n in assets and not assets[n].get("parent_id"):
            call("PATCH", f"/api/v1/assets/{assets[n]['id']}", {"parent_id": room["id"], "reason": "the fridges stand in the store room"})
    return {"room": room["id"]}


# ---------------------------------------------------------------------------------------------- energy
METERS = [("L1", "Energy meter – Line 1", 46, "line"), ("L2", "Energy meter – Line 2", 31, "line"),
          ("COMP", "Energy meter – compressor room", 17, "base"), ("ADM", "Energy meter – offices and lighting", 9, "office"),
          ("PV", "Solar PV – hall roof", 80, "solar")]


def meter_kwh(kind, peak, t, rng):
    """Energy in the hour starting at t (kWh)."""
    lt = time.localtime(t)
    h, wd, yd = lt.tm_hour, lt.tm_wday, lt.tm_yday
    season = math.cos((yd - 15) / 365 * 2 * math.pi)  # 1 in January, -1 in July
    if kind == "line":
        on = wd < 5 and 6 <= h < 22
        return max(0, (peak if on else peak * 0.08) * rng.uniform(0.85, 1.1))
    if kind == "base":
        return peak * (0.7 if wd >= 5 else 1) * rng.uniform(0.8, 1.15)
    if kind == "office":
        on = wd < 5 and 7 <= h < 17
        return peak * (1 if on else 0.25) * (1 + 0.35 * season) * rng.uniform(0.9, 1.1)
    # solar: day length and height follow the season, clouds per day
    noon, half = 13, 4.5 - 2.5 * season
    if abs(h + 0.5 - noon) > half: return 0
    clouds = random.Random(t // 86400).uniform(0.25, 1)
    return peak * (0.6 - 0.4 * season) * math.cos((h + 0.5 - noon) / half * math.pi / 2) * clouds


@once("energy")
def energy():
    site = next(s for s in call("GET", "/api/v1/sites") if s["name"] == "Nitra plant")["id"]
    sources, ids = [], {}
    start = (NOW - 400 * 86400) // 3600 * 3600
    for i, (code, name, peak, kind) in enumerate(METERS):
        a = asset(site, "meter", name)
        d, tok = device("em-" + code.lower(), name, loc=(48.2905 + i * 0.0006, 18.1168 + i * 0.0009))
        ds = stream(a, d, "energy", "energy", "kWh", kind="counter")
        rng, cum, samples = random.Random(100 + i), 10000.0 * (i + 1), []
        for t in range(start, NOW, 3600):
            samples.append({"ts": t, "v": {"energy": round(cum, 2)}})
            cum += meter_kwh(kind, peak, t, rng)
        samples.append({"ts": NOW, "v": {"energy": round(cum, 2)}})
        send(tok, samples)
        sources.append({"datastream_id": ds, "role": "production" if kind == "solar" else "consumption"})
        ids[code] = {"ds": ds, "token": tok, "cum": cum, "kind": kind, "peak": peak}
    call("PUT", "/api/v1/energy/settings", {"price_per_kwh": 0.168, "feed_in_per_kwh": 0.052, "currency": "€", "exclude_others": True, "sources": sources, "reason": R})
    return ids


# ---------------------------------------------------------------------------------------------- dashboards
def w(id, typ, x, y, ww, hh, ds, **p):
    return {"id": id, "type": typ, "geometry": {"x": x, "y": y, "w": ww, "h": hh}, "bindings": {"datastreams": ds} if ds else {}, "params": p}


def dash(name, spec):
    """Creates a dashboard, or saves a new version of the one with this name."""
    old = by_name("/api/v1/dashboards", name)
    if old:
        call("PUT", f"/api/v1/dashboards/{old['slug']}", {"name": name, "spec": spec, "reason": R}); return old["slug"]
    return call("POST", "/api/v1/dashboards", {"name": name, "spec": spec, "reason": R})["slug"]


@once("dashboards")
def dashboards():
    c, e = st["cold"], st["energy"]
    out = {}
    cold = [
        w("v1", "value", 0, 0, 3, 2, [c["f1"]], title="Medicine fridge 1", decimals=1, sparkline=True),
        w("v2", "value", 3, 0, 3, 2, [c["f2"]], title="Medicine fridge 2", decimals=1, sparkline=True),
        w("v3", "value", 6, 0, 3, 2, [c["fz"]], title="Vaccine freezer", decimals=1, sparkline=True),
        w("al", "alarms", 9, 0, 3, 4, None, title="Alarms", limit=10),
        w("ch", "chart", 0, 2, 9, 3, [c["f1"], c["f2"]], title="Fridge temperature (7 days)", range="7d", band=True, legend=True, series=[{"label": "Fridge 1"}, {"label": "Fridge 2"}]),
        w("pt", "period_table", 0, 5, 8, 5, [c["f1"], c["f2"]], title="Daily temperature log (min / max, limits 2–8 °C)", interval="day", values="min,max", period="last_30d", low=2, high=8, total=True, decimals=1, series=[{"label": "Fridge 1"}, {"label": "Fridge 2"}]),
        w("rm", "chart", 8, 5, 4, 3, [c["rt"], c["rh"]], title="Store – temperature and humidity", range="7d", series=[{"label": "°C"}, {"label": "% RH"}]),
        w("hm", "heatmap", 8, 8, 4, 2, [c["f2"]], title="Fridge 2 – hour × day", range="7d"),
    ]
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "7d", "refresh_s": 60, "widgets": cold}
    out["cold"] = dash("Cold chain – medicine store", spec)
    cons = [e[k]["ds"] for k in ("L1", "L2", "COMP", "ADM")]
    lbl = [{"label": "Line 1"}, {"label": "Line 2"}, {"label": "Compressor room"}, {"label": "Offices"}]
    en = [
        w("a1", "agg_card", 0, 0, 3, 2, [e["L1"]["ds"]], title="Line 1 – consumption, 30 days", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a2", "agg_card", 3, 0, 3, 2, [e["L2"]["ds"]], title="Line 2 – consumption, 30 days", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a3", "agg_card", 6, 0, 3, 2, [e["COMP"]["ds"]], title="Compressor room, 30 days", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a4", "agg_card", 9, 0, 3, 2, [e["PV"]["ds"]], title="Solar PV – generation, 30 days", agg="delta", range="30d", icon="sun", decimals=0, unit="kWh", color="#16a34a"),
        w("pd", "period_table", 0, 2, 12, 5, cons + [e["PV"]["ds"]], title="Daily consumption – last 7 days (kWh)", interval="day", values="delta", period="last_7d", total=True, decimals=0, series=lbl + [{"label": "Solar PV"}]),
        w("pt", "period_table", 0, 7, 12, 7, cons + [e["PV"]["ds"]], title="Monthly consumption and generation (kWh)", interval="month", values="delta", period="last_12m", total=True, decimals=0, series=lbl + [{"label": "Solar PV"}]),
    ]
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "30d", "refresh_s": 300, "widgets": en}
    out["energy"] = dash("Energy – monthly consumption", spec)
    for kind, name in (("overview", "Overview"), ("showcase", "Showcase – all widgets")):
        s = call("GET", f"/api/v1/dashboards/starter?kind={kind}")
        out[kind] = dash(name, s["spec"])
    return out


@once("overviews")
def overviews():
    """The ready-made overviews (0.68.0): widgets select datastreams by query and follow the dashboard filters. The cold
    chain overview starts filtered to the pharmacy (a default of its site filter), so the pictures show a filter in use;
    the overview of all sensors gets a public link."""
    out = {}
    for kind in ("sensors", "energy", "coldchain"):
        s = call("GET", f"/api/v1/dashboards/starter?kind={kind}")
        spec = s["spec"]
        if kind == "coldchain":
            for f in spec.get("filters", []):
                if f["kind"] == "site":
                    f["default"] = [st["cold"]["site"]]
        out[kind] = dash(s["name"], spec)
    call("POST", f"/api/v1/dashboards/{out['sensors']}/share", {"reason": R})
    return out


@once("drilldown")
def drilldown():
    """Entity aliases, key filters and dashboard states (0.73.0): the cold chain with drill-down. Every fridge and the
    freezer gets its own limit (attribute max_temp); the dashboard has an entities table of the fridges with their
    temperature, limit and storage class (rows above their own limit highlighted), an overview table of the fridges
    above 8 °C (a value filter "Limit" the viewer may change), an entity selector and a map of the loggers; a row,
    a marker or the selector opens the state "Fridge detail" with the fridge's trend, daily log, logger and alarms. The
    dashboard gets a public link (drill-down works there too)."""
    c = st["cold"]
    limits = {"Medicine fridge 1": 8, "Medicine fridge 2": 8, "Vaccine freezer": -15}
    for a in call("GET", "/api/v1/assets"):
        if a["name"] in limits:
            attrs = dict(a.get("attributes") or {}, max_temp=limits[a["name"]])
            call("PATCH", f"/api/v1/assets/{a['id']}", {"attributes": attrs, "reason": R})
    call("PUT", "/api/v1/organization/attributes", {"attributes": {"max_temp_default": 8}, "reason": R})
    g = lambda x, y, ww, hh: {"x": x, "y": y, "w": ww, "h": hh}
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "24h", "refresh_s": 60,
        "filters": [{"kind": "value", "key": "limit", "label": "Limit °C", "default": "8", "public": True}],
        "aliases": [
            {"id": "cold", "name": "Fridges and freezers of the pharmacy", "kind": "hierarchy", "root": {"type": "site", "id": c["site"]}, "depth": 10, "entity_type": "asset", "asset_type": ["fridge", "freezer"]},
            {"id": "temps", "name": "Temperatures of the selected fridge", "kind": "current", "entity_type": "datastream"},
            {"id": "loggers", "name": "Loggers of the selected fridge", "kind": "current", "entity_type": "device"}],
        "key_filters": [
            {"id": "above_limit", "name": "Above the limit (8 °C)", "match": "all", "rules": [{"source": "value", "key": "temp", "type": "number", "op": "gt", "dynamic": {"from": "filter", "key": "limit"}, "value": 8}]},
            {"id": "own_limit", "name": "Above its own limit", "match": "all", "rules": [{"source": "value", "key": "temp", "type": "number", "op": "gt", "dynamic": {"from": "self", "key": "max_temp"}}]}],
        "states": [{"id": "overview", "name": "Cold chain"}, {"id": "fridge", "name": "Fridge detail"}],
        "widgets": [
            {"id": "pick", "type": "entity_selector", "geometry": g(0, 0, 3, 3), "alias": {"id": "cold"}, "params": {"title": "Fridge", "style": "list"}},
            {"id": "now", "type": "value", "geometry": g(3, 0, 3, 3), "alias": {"id": "temps", "quantity": ["temperature"]}, "bindings": {"datastreams": []}, "params": {"title": "Selected fridge", "decimals": 1, "sparkline": True},
             "actions": [{"event": "button", "action": "state", "target": "fridge", "pass": "asset", "label": "Details"}]},
            {"id": "table", "type": "entities_table", "geometry": g(6, 0, 6, 3), "alias": {"id": "cold"},
             "params": {"title": "Fridges and freezers", "columns": ["name", "t:temp=Temperature", "a:max_temp=Limit", "a:storage_class=Storage class", "alarm"], "highlight_filter": "own_limit", "decimals": 1, "page_size": 10},
             "actions": [{"event": "row", "action": "state", "target": "fridge"}, {"event": "row_button", "action": "device", "pass": "device", "label": "Logger"}]},
            {"id": "warm", "type": "overview_table", "geometry": g(0, 3, 7, 3), "alias": {"id": "cold", "filter": "above_limit", "quantity": ["temperature"]}, "bindings": {"datastreams": []},
             "params": {"title": "Fridges above the limit", "columns": ["asset", "device", "value", "limits", "age", "alarm"], "decimals": 1, "export": False},
             "actions": [{"event": "row", "action": "state", "target": "fridge", "pass": "asset"}]},
            {"id": "map", "type": "map", "geometry": g(7, 3, 5, 3), "alias": {"id": "cold"}, "bindings": {"devices": []}, "params": {"title": "Loggers"},
             "actions": [{"event": "marker", "action": "state", "target": "fridge"}]},
            {"id": "trend", "type": "chart", "state": "fridge", "geometry": g(0, 0, 8, 3), "alias": {"id": "temps", "quantity": ["temperature"]}, "bindings": {"datastreams": []},
             "params": {"title": "Temperature", "range": "24h", "decimals": 1, "warn": 7.5, "action": 8}},
            {"id": "logger", "type": "devices", "state": "fridge", "geometry": g(8, 0, 4, 2), "alias": {"id": "loggers"}, "bindings": {"devices": []}, "params": {"title": "Logger"},
             "actions": [{"event": "row", "action": "device"}]},
            {"id": "alarms", "type": "alarms", "state": "fridge", "geometry": g(8, 2, 4, 4), "alias": {"id": "temps"}, "params": {"title": "Alarms", "limit": 10}},
            {"id": "log", "type": "period_table", "state": "fridge", "geometry": g(0, 3, 8, 5), "alias": {"id": "temps", "quantity": ["temperature"]}, "bindings": {"datastreams": []},
             "params": {"title": "Daily log (min / max)", "interval": "day", "values": "min,max", "period": "last_7d", "low": 2, "high": 8, "decimals": 1, "export": False}}]}
    slug = dash("Cold chain – drill-down", spec)
    tok = call("POST", f"/api/v1/dashboards/{slug}/share", {"reason": R})["token"]
    return {"slug": slug, "token": tok}


# ---------------------------------------------------------------------------------------------- alarms and incidents
@once("incidents")
def incidents():
    time.sleep(2)
    alarms = call("GET", "/api/v1/alarms?state=active") or []
    if isinstance(alarms, dict): alarms = alarms.get("items", [])
    acked = None
    for a in alarms:
        if a.get("state") == "active" and a.get("severity") == "warning":
            call("POST", f"/api/v1/alarms/{a['id']}/ack", {"reason": "Freezer opened while picking an order, monitoring"}); acked = a["id"]; break
    # incidents the server opened for alarms it could not deliver (no SMTP here): close all but the newest
    auto = [i for i in call("GET", "/api/v1/incidents") if i["kind"] == "alarm_undelivered" and i["status"] == "open"]
    for i in auto[1:]:
        call("PATCH", f"/api/v1/incidents/{i['id']}", {"status": "closed", "cause": "The demo server has no SMTP server configured.", "capa": "Configure SMTP in config.toml before going live.", "reason": R})
    i1 = call("POST", "/api/v1/incidents", {"title": "Medicine fridge 2 – temperature above 8 °C", "severity": "critical", "impact": "Insulins and vaccines in fridge 2 outside 2–8 °C, quarantined until assessed.", "reason": R})["id"]
    i2 = call("POST", "/api/v1/incidents", {"title": "Line 1 – repeated press stops", "severity": "major", "impact": "About 6 % of the shift output lost.", "reason": R})["id"]
    call("PATCH", f"/api/v1/incidents/{i2}", {"status": "investigating", "note": "Maintenance is checking the position sensor.", "reason": R})
    i3 = call("POST", "/api/v1/incidents", {"title": "Logger T3 outage (battery)", "severity": "minor", "reason": R})["id"]
    call("PATCH", f"/api/v1/incidents/{i3}", {"status": "closed", "cause": "The logger battery was flat.", "capa": "Yearly battery replacement added to the maintenance plan.", "reason": R})
    return {"acked_alarm": acked, "ids": [i1, i2, i3]}


# ---------------------------------------------------------------------------------------------- automation
@once("rules")
def rules():
    c = st["cold"]
    prod = json.load(open(state_path("production.json"), encoding="utf-8")) if os.path.exists(state_path("production.json")) else None
    out = []
    spec = {"inputs": [{"name": "t", "datastream_id": c["f2"]}, {"name": "room", "datastream_id": c["rt"]}], "match": "all",
            "conditions": [{"input": "t", "op": ">", "value": 7.5}, {"input": "room", "op": "<", "value": 26}],
            "actions": [{"type": "notify", "channel_id": c["ch1"], "subject": "Fridge 2: {t} °C – check the door"}, {"type": "alarm", "severity": "warning"}],
            "clear_actions": [{"type": "notify", "channel_id": c["ch1"], "subject": "Fridge 2 is back within limits"}]}
    out.append(call("POST", "/api/v1/rules", {"name": "Fridge 2 – door left open", "description": "The temperature rises although the room is not overheated", "spec": spec, "reason": R})["id"])
    if prod:
        ds = prod["lines"][0]["ds"]
        spec = {"inputs": [{"name": "temperature", "datastream_id": ds["temperature"]}, {"name": "state", "datastream_id": ds["state"]}], "match": "all",
                "conditions": [{"input": "temperature", "op": ">=", "value": 70}, {"input": "state", "op": "==", "value": 1}],
                "actions": [{"type": "alarm", "severity": "action"}, {"type": "notify", "channel_id": c["ch2"], "subject": "Line 1 press overheating: {temperature} °C"}]}
        out.append(call("POST", "/api/v1/rules", {"name": "Line 1 – press overheating", "description": "Press temperature above 70 °C while running", "spec": spec, "reason": R})["id"])
    return out


def flow(name, spec):
    old = next((f["flow"] for f in call("GET", "/api/v1/flows") if f["flow"]["name"] == name), None)
    if old:
        call("PUT", f"/api/v1/flows/{old['slug']}", {"spec": spec, "reason": "clearer node layout"}); return old["slug"]
    r = call("POST", "/api/v1/flows", {"name": name, "spec": spec, "reason": R})
    return r.get("slug") or r["flow"]["slug"]


@once("flows")
def flows():
    c, e = st["cold"], st["energy"]
    out = []
    spec = {"nodes": [
        {"id": "t", "type": "ds_value", "x": 20, "y": 60, "settings": {"datastream": c["f2"]}},
        {"id": "hot", "type": "stable_for", "x": 200, "y": 60, "settings": {"condition": "value > 8", "seconds": 600}},
        {"id": "rl", "type": "rate_limit", "x": 380, "y": 60, "settings": {"count": 1, "period_s": 3600}},
        {"id": "mail", "type": "notify", "x": 560, "y": 10, "settings": {"channel": c["ch1"], "subject": "Fridge 2: {{ fixed(value, 1) }} °C for more than 10 min"}},
        {"id": "inc", "type": "incident", "x": 560, "y": 110, "settings": {"action": "open", "severity": "major", "title": "Fridge 2 outside 2–8 °C", "key": "fridge-2"}},
        {"id": "dbg", "type": "debug", "x": 740, "y": 10, "settings": {"expr": "'sent ' + str(value)"}},
        {"id": "note", "type": "comment", "x": 20, "y": 180, "settings": {"text": "Temperature above 8 °C for more than 10 minutes → an e-mail to the pharmacist and an incident (at most once an hour)."}}],
        "wires": [{"from": "t", "port": 0, "to": "hot"}, {"from": "hot", "port": 0, "to": "rl"}, {"from": "rl", "port": 0, "to": "mail"},
                  {"from": "rl", "port": 0, "to": "inc"}, {"from": "mail", "port": 0, "to": "dbg"}]}
    out.append(flow("Fridge 2 – alert to the pharmacist", spec))
    spec = {"nodes": [
        {"id": "s", "type": "schedule", "x": 40, "y": 60, "settings": {"mode": "daily", "times": "06:00", "weekdays": ["mon", "tue", "wed", "thu", "fri"]}},
        {"id": "m", "type": "template", "x": 220, "y": 60, "settings": {"text": "Morning energy summary: {{ fmtTime(now(), '2006-01-02') }}", "target": "text"}},
        {"id": "n", "type": "notify", "x": 420, "y": 60, "settings": {"channel": c["ch2"], "subject": "Morning summary", "text": "{{ msg.text }}"}},
        {"id": "p", "type": "ds_value", "x": 40, "y": 180, "settings": {"datastream": e["PV"]["ds"], "min_interval_s": 900}},
        {"id": "d", "type": "deadband", "x": 220, "y": 180, "settings": {"band": 5}},
        {"id": "g", "type": "debug", "x": 420, "y": 180, "settings": {"expr": "'PV ' + fixed(value, 0) + ' kWh'"}}],
        "wires": [{"from": "s", "port": 0, "to": "m"}, {"from": "m", "port": 0, "to": "n"}, {"from": "p", "port": 0, "to": "d"}, {"from": "d", "port": 0, "to": "g"}]}
    out.append(flow("Energy – morning summary", spec))
    for slug in out:
        try: call("POST", f"/api/v1/flows/{slug}/deploy", {"reason": R})
        except SystemExit as x: print("deploy", slug, x)
    return out


# ---------------------------------------------------------------------------------------------- smart home
@once("mqtt")
def mqtt_accounts():
    a = call("POST", "/api/v1/mqtt-sources", {"kind": "account", "name": "Family house – Zigbee and Tasmota", "adopt": "auto", "reason": R})
    b = call("POST", "/api/v1/mqtt-sources", {"kind": "account", "name": "Office – new devices", "adopt": "inbox", "reason": R})
    p = call("POST", "/api/v1/provision-profiles", {"name": "Temperature sensors – store", "strategy": "create", "transport": "http", "approval": True, "auto_datastreams": True, "max_devices": 50, "name_pattern": "Sensor {serial}", "site_id": st["cold"]["site"], "reason": R})
    return {"auto": {"id": a["id"], "user": a["username"], "pass": a["password"]}, "inbox": {"id": b["id"], "user": b["username"], "pass": b["password"]}, "profile": p.get("id")}


def simulators(wait=True):
    """Builds tools/mqttsim and starts the zigbee2mqtt, Tasmota and Shelly simulators (run for a day)."""
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
    exe = os.path.join(root, ".dev", "mqttsim.exe" if os.name == "nt" else "mqttsim")
    subprocess.run(["go", "build", "-o", exe, "./tools/mqttsim"], cwd=root, check=True)
    m = st["mqtt"]
    procs = []
    for args in (["z2m", "-user", m["auto"]["user"], "-pass", m["auto"]["pass"]],
                 ["tasmota", "-user", m["auto"]["user"], "-pass", m["auto"]["pass"], "-id", "GARAGE"],
                 ["shelly", "-user", m["inbox"]["user"], "-pass", m["inbox"]["pass"], "-id", "OFFICE1"]):
        log = open(os.path.join(root, ".dev", "homesim-" + args[0] + ".log"), "w")
        procs.append(subprocess.Popen([exe] + args + ["-addr", os.environ.get("CTRL32_MQTT", "127.0.0.1:1883"), "-wait", "86400"], stdout=log, stderr=subprocess.STDOUT))
    return procs


@once("home")
def home():
    procs = simulators()
    ents = []
    for _ in range(40):
        time.sleep(0.5)
        ents = call("GET", f"/api/v1/entities?source={st['mqtt']['auto']['id']}")
        if len(ents) >= 6: break
    time.sleep(2)
    ents = call("GET", f"/api/v1/entities?source={st['mqtt']['auto']['id']}")
    for p in procs: p.terminate()
    lights = [x["id"] for x in ents if x.get("component") in ("light", "switch")]
    sc = []
    if lights:
        sc.append(call("POST", "/api/v1/scenes", {"name": "Evening", "icon": "moon", "capture": lights, "reason": R})["id"])
        sc.append(call("POST", "/api/v1/scenes", {"name": "Leaving home", "icon": "door", "actions": [{"entity_id": i, "service": "turn_off"} for i in lights], "reason": R})["id"])
    return {"entities": len(ents), "scenes": sc}


# ---------------------------------------------------------------------------------------------- the rest
def video_walls():
    """Ids of the demo video walls: the control room first, then the gatehouse (empty without cameras.py)."""
    ws = sorted(call("GET", "/api/v1/video/walls"), key=lambda x: 0 if x["name"].startswith("Control room") else 1)
    return [x["id"] for x in ws]


@once("display")
def display():
    hello = json.loads(urllib.request.urlopen(urllib.request.Request(B + "/api/v1/display/hello", method="POST", data=b"{}", headers={"Content-Type": "application/json"})).read())
    walls = video_walls()
    items = [{"kind": "dashboard", "slug": st["dashboards"]["cold"]}]
    if walls: items.insert(0, {"kind": "wall", "wall": walls[1], "screen": 0})
    d = call("POST", "/api/v1/displays/pair", {"code": hello["code"], "name": "Gatehouse – TV", "location": "Nitra site, gatehouse", "offline_alert": False, "content": {"items": items}, "reason": R})
    return d.get("id")


@once("firmware")
def firmware():
    blob = os.urandom(4096) + b"ctrl32-demo-firmware"
    bnd = uuid.uuid4().hex
    fields = {"name": "Temperature logger", "version": "1.5.0", "hw": "esp32-logger", "notes": "Shorter interval when a limit is exceeded, battery reporting fixed.", "reason": R}
    body = b"".join(f"--{bnd}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode() for k, v in fields.items())
    body += f"--{bnd}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"logger-1.5.0.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n".encode() + blob + f"\r\n--{bnd}--\r\n".encode()
    fw = json.loads(raw("POST", "/api/v1/firmware", body, "multipart/form-data; boundary=" + bnd))
    camp = call("POST", "/api/v1/fota/campaigns", {"name": "Loggers – autumn 2026", "firmware_id": fw["id"], "device_ids": [st["cold"]["esp"]], "reason": R})
    call("POST", f"/api/v1/devices/{st['cold']['esp']}/commands", {"method": "reboot", "args": {"delay_s": 5}, "ttl_seconds": 600, "reason": "restart after the interval change"})
    return {"firmware": fw["id"], "campaign": camp.get("id")}


@once("people")
def people():
    role = by_name("/api/v1/roles", "refrigeration_technician")
    role = role["id"] if role else call("POST", "/api/v1/roles", {"name": "refrigeration_technician", "permissions": ["data.read", "alarm.ack", "data.export", "device.manage"], "reason": R})["id"]
    have = {u["username"] for u in call("GET", "/api/v1/users")}
    users = []
    for u, n, r in (("jane.cooper", "Jane Cooper", ["operator"]), ("peter.hughes", "Peter Hughes", ["engineer"]),
                    ("emma.walsh", "Emma Walsh", ["qa"]), ("martin.reed", "Martin Reed", ["refrigeration_technician"])):
        if u not in have:
            call("POST", "/api/v1/users", {"username": u, "full_name": n, "email": u + "@nitra-plant.example", "roles": r, "reason": R})
        users.append(u)
    if not by_name("/api/v1/tokens", "Power BI – data export"):
        call("POST", "/api/v1/tokens", {"name": "Power BI – data export", "scopes": ["data.read", "data.export"]})
    return {"role": role, "users": users}


@once("settings")
def settings():
    c = st["cold"]
    out = {}
    con = by_name("/api/v1/connectors", "LoRaWAN – ChirpStack site") or call("POST", "/api/v1/connectors", {"kind": "chirpstack_http", "name": "LoRaWAN – ChirpStack site", "auto_create_devices": True, "reason": R})
    out["connector"] = con.get("id")
    walls = video_walls()
    app = {"slug": "control-room", "name": "Control room – cameras", "short_name": "Control room", "start_url": f"/video/walls/{walls[0]}" if walls else "/video",
           "sections": ["cameras", "dashboards", "incidents"], "theme_color": "#0e7490", "camera_layout": "grid", "reason": R}
    apps = {a["slug"] for a in call("GET", "/api/v1/pwa-apps")}
    if "control-room" not in apps: call("POST", "/api/v1/pwa-apps", app)
    out["app"] = "control-room"
    if "medicines" not in apps: call("POST", "/api/v1/pwa-apps", {"slug": "medicines", "name": "Medicine store", "short_name": "Medicines", "start_url": f"/dashboards/{st['dashboards']['cold']}", "sections": ["dashboards", "incidents"], "locked": True, "reason": R})
    # report template, then two stored reports
    spec = call("GET", "/api/v1/report-templates/observations/default-spec")["spec"]
    call("PUT", "/api/v1/report-templates/observations", {"name": "Temperature log – medicine store", "locale": "en", "spec": spec, "reason": R})
    to = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(NOW)); fr = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(NOW - 7 * 86400))
    try:
        raw("GET", f"/api/v1/export/observations?datastream_id={c['f2']}&from={fr}&to={to}&format=pdf")
        raw("GET", f"/api/v1/period-table/export?datastreams={c['f1']},{c['f2']}&interval=day&values=min,max&period=last_month&low=2&high=8&total=1&format=pdf&title=Fridge%20temperatures")
    except Exception as x: print("reports:", x)
    # a pending change request for the approvals screen: four-eyes on limits, one rule change, policy off again
    call("PUT", "/api/v1/settings/policy", {"four_eyes_limits": True, "reason": R})
    try: call("POST", "/api/v1/alarm-rules", {"datastream_id": c["f1"], "type": "high", "warning": 7.0, "action": 8, "hysteresis": 0.2, "reason": "tighter warning per SOP-12"})
    finally: call("PUT", "/api/v1/settings/policy", {"four_eyes_limits": False, "reason": R})
    return out


# ---------------------------------------------------------------------------------------------- VPN
def vpn():
    """System → VPN: a site router, a technician with an expiry, a gateway and two access rules (found by name, so it
    can run again). Needs the server agent (on a PC its VPN simulation: [agent] socket and vpn_simulate in
    .dev/config.toml, the agent started with `c32 agent`) and, for the site and the expiry, a licence; otherwise it
    says so and skips. The private keys of the configurations are not kept."""
    v = call("GET", "/api/v1/vpn")
    if not v.get("supported"):
        print("skip vpn: no server agent with the VPN (set [agent] socket and vpn_simulate, start the agent)"); return None
    if not v.get("licensed"):
        print("skip vpn: the site and the time-limited technician need a licence"); return None
    i = v.get("interface") or {}
    if not i.get("enabled") or not i.get("public_key"):
        call("PUT", "/api/v1/server/wireguard", {"enabled": True, "listen_port": 51820, "address": "10.66.0.1/24", "endpoint": "vpn.nitra-plant.example:51820", "dns": "", "reason": R})
    pw = os.environ.get("CTRL32_PASS", "admin1234")
    org = (call("GET", "/api/v1/organizations") or [{}])[0].get("id", "")
    peers = {p["name"]: p["id"] for p in call("GET", "/api/v1/vpn")["peers"]}
    week = time.strftime("%Y-%m-%dT17:00:00Z", time.gmtime(NOW + 7 * 86400))
    for name, kind, extra in (("Nitra plant - line router", "site", {"subnets": ["192.168.10.0/24"], "org_id": org, "note": "MikroTik hEX in the line 1 control cabinet"}),
                              ("Service laptop - J. Cooper", "user", {"expires_at": week, "note": "Commissioning of line 1"}),
                              ("Gateway hall 2", "device", {"org_id": org, "note": "MQTT without TLS through the tunnel"})):
        if name not in peers:
            peers[name] = call("POST", "/api/v1/vpn/peers", dict(extra, name=name, kind=kind, reason=R, password=pw))["peer"]["id"]
    rules = {r["name"] for r in call("GET", "/api/v1/vpn")["rules"]}
    for body in ({"name": "Technicians → PLC line 1 (S7 102, Modbus 502)", "sources": [{"type": "kind", "kind": "user"}], "dest_type": "site",
                  "dest_peer_id": peers["Nitra plant - line router"], "dest_cidr": "192.168.10.20", "protocol": "tcp", "ports": "102, 502"},
                 {"name": "Devices → MQTT", "sources": [{"type": "kind", "kind": "device"}], "dest_type": "server", "protocol": "tcp", "ports": "1883"}):
        if body["name"] not in rules:
            call("POST", "/api/v1/vpn/rules", dict(body, enabled=True, reason=R))
    return {"peers": peers}


ORG_ADMIN, ORG_ADMIN_PW = "olivia.grant", "Olivia-docs-2026!"


class Session:
    """A second signed-in user (the organization's administrator of the VPN pictures)."""
    def __init__(self):
        import http.cookiejar
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.csrf = ""

    def call(self, m, path, body=None):
        h = {"Content-Type": "application/json", "X-CSRF-Token": self.csrf}
        req = urllib.request.Request(B + path, method=m, headers=h, data=json.dumps(body).encode() if body is not None else None)
        try:
            with self.op.open(req) as r:
                t = r.read()
                return json.loads(t) if t else {}
        except urllib.error.HTTPError as e:
            raise SystemExit(f"{m} {path}: {e.code} {e.read()[:300]}")

    def login(self, user, pw):
        self.call("POST", "/api/v1/auth/login", {"username": user, "password": pw})
        self.csrf = self.call("GET", "/api/v1/auth/me")["csrf_token"]
        return self


def vpn2():
    """VPN phase 2 (0.65.0): an administrator of the organization (org_admin only, no server administration) who
    manages the organization's VPN, four eyes for technicians' access, one approved and one pending access grant, a
    rule of the organization. Needs what vpn() needs; runs again without duplicating anything. The administrator's
    password is set to ORG_ADMIN_PW so docshots.js can sign in as her."""
    v = call("GET", "/api/v1/vpn")
    if not v.get("supported") or not v.get("licensed"):
        print("skip vpn2: needs the server agent with the VPN and a licence"); return None
    u = by_name("/api/v1/users", ORG_ADMIN, key="username")
    if not u:
        u = call("POST", "/api/v1/users", {"username": ORG_ADMIN, "full_name": "Olivia Grant", "email": ORG_ADMIN + "@nitra-plant.example", "roles": ["org_admin"], "reason": "administrator of the Nitra plant organization"})["user"]
    olivia = Session()
    try:
        olivia.login(ORG_ADMIN, ORG_ADMIN_PW)
    except SystemExit:
        once_pw = call("POST", f"/api/v1/users/{u['id']}/reset-password", {"reason": R})["initial_password"]
        olivia.login(ORG_ADMIN, once_pw)
        olivia.call("POST", "/api/v1/auth/password", {"current": once_pw, "new": ORG_ADMIN_PW})
        olivia = Session().login(ORG_ADMIN, ORG_ADMIN_PW)
    call("PUT", "/api/v1/settings/policy", {"four_eyes_vpn": True, "reason": "technicians' VPN access is approved by a second person (SOP-21)"})
    mine = {p["name"]: p for p in olivia.call("GET", "/api/v1/vpn")["peers"]}
    for name, note in (("Service laptop - P. Hughes", "Line 1 PLC service"), ("Tablet - E. Walsh", "HMI checks on night shifts")):
        if name not in mine:
            mine[name] = olivia.call("POST", "/api/v1/vpn/peers", {"name": name, "kind": "user", "note": note, "reason": R, "password": ORG_ADMIN_PW})["peer"]
    site = next((p for p in mine.values() if p["kind"] == "site"), None)
    rules = {r["name"] for r in olivia.call("GET", "/api/v1/vpn")["rules"]}
    if site and "Nitra technicians → HMI line 1 (VNC 5900)" not in rules:
        olivia.call("POST", "/api/v1/vpn/rules", {"name": "Nitra technicians → HMI line 1 (VNC 5900)", "sources": [{"type": "peer", "id": mine["Service laptop - P. Hughes"]["id"]}, {"type": "peer", "id": mine["Tablet - E. Walsh"]["id"]}],
                                                 "dest_type": "site", "dest_peer_id": site["id"], "dest_cidr": "192.168.10.30", "protocol": "tcp", "ports": "5900", "enabled": True, "reason": R})
    state = {p["id"]: p for p in olivia.call("GET", "/api/v1/vpn")["peers"]}
    tablet, laptop = state[mine["Tablet - E. Walsh"]["id"]], state[mine["Service laptop - P. Hughes"]["id"]]
    # the tablet: requested by Olivia, approved by the server administrator — access for 8 hours
    if not tablet.get("grant") and not tablet.get("pending_grant"):
        r = olivia.call("POST", f"/api/v1/vpn/peers/{tablet['id']}/grant", {"minutes": 480, "reason": "night shift 2 Oct, HMI alarm checks"})
        call("POST", f"/api/v1/change-requests/{r['change_request']['id']}/approve", {"reason": "night shift confirmed by the plant manager"})
    # the laptop: a request waiting in Settings → Approvals
    if not laptop.get("grant") and not laptop.get("pending_grant"):
        olivia.call("POST", f"/api/v1/vpn/peers/{laptop['id']}/grant", {"minutes": 120, "reason": "service call 2026-117, PLC program update"})
    return {"org_admin": ORG_ADMIN}


# ---------------------------------------------------------------------------------------------- records (0.67.0)
def sign_in(user, password):
    call("POST", "/api/v1/auth/login", {"username": user, "password": password})
    common._csrf = call("GET", "/api/v1/auth/me")["csrf_token"]


@once("records")
def records():
    """The Telemetry tab, sensor faults, annotations, signed reports and a signed audit review:
    - logger T2 also sends keys without a datastream (door contact, probe resistance) and status messages;
    - fridge 2 gets a physical range and a sensor_fault rule, and one faulty reading (flagged by the logger) an hour ago;
    - two annotations on fridge 2 (the excursion of day 11 explained, a probe check);
    - the quality reviewer emma.walsh signs the stored observation report and reviews the audit trail of the last day."""
    c = st["cold"]
    call("PATCH", f"/api/v1/datastreams/{c['f2']}", {"range": {"min": -40, "max": 85}, "reason": "PT1000 probe: measuring range -40…85 °C"})
    have = {(r["datastream_id"], r["type"]) for r in call("GET", "/api/v1/alarm-rules")}
    if (c["f2"], "sensor_fault") not in have:
        call("POST", "/api/v1/alarm-rules", {"datastream_id": c["f2"], "type": "sensor_fault", "delay_seconds": 0, "reason": "SOP-12: probe faults are reported"})
    time.sleep(31)  # the alarm engine reads the rules of a datastream again after 30 s
    rng = random.Random(11)
    samples = []
    for t in range((NOW - 86400) // 900 * 900, NOW, 900):
        door = 1 if 7 <= (t / 3600) % 24 < 17 and rng.random() < 0.12 else 0
        samples.append({"ts": t, "v": {"door": door, "probe_ohm": round(1000 + 3.85 * fridge_temp("f2", t, rng) + rng.uniform(-0.4, 0.4), 1)}})
    send(c["t2"], samples)
    t = NOW - 3600
    send(c["t2"], [{"ts": t, "v": {"temp": -73.6}, "fault": ["temp"]}, {"ts": t + 300, "v": {"temp": fridge_temp("f2", t + 300, rng)}}])
    for i, (bat, rssi) in enumerate([(3.66, -68), (3.64, -71), (3.63, -70)]):
        call("POST", "/api/v1/d/status", {"bat": bat, "bat_pct": 81 - i, "rssi": rssi, "up_s": 86400 * 12 + i * 900, "fw": "2.3.1", "buf": 0}, token=c["t2"])
        time.sleep(1)
    day = (NOW // 86400 - ((NOW // 86400) % 30) + 11) * 86400  # day 11 of the 30-day cycle (the excursion)
    if day > NOW: day -= 30 * 86400
    iso = lambda x: time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(x))
    call("POST", "/api/v1/annotations", {"datastream_id": c["f2"], "from": iso(day + 13 * 3600), "to": iso(day + 14 * 3600), "category": "expected_event", "text": "Delivery: door kept open while unloading vaccines"})
    call("POST", "/api/v1/annotations", {"datastream_id": c["f2"], "from": iso(NOW - 3600), "category": "maintenance", "text": "Probe connector reseated after the fault alarm"})
    # the quality reviewer signs a stored report and reviews the audit trail
    emma = next(u for u in call("GET", "/api/v1/users") if u["username"] == "emma.walsh")
    one = call("POST", f"/api/v1/users/{emma['id']}/reset-password", {"reason": R})["initial_password"]
    pw = "Emma-Review-2026!"
    sign_in("emma.walsh", one)
    call("POST", "/api/v1/auth/password", {"current": one, "new": pw})
    sign_in("emma.walsh", pw)
    rep = next((r for r in call("GET", "/api/v1/reports?limit=50") if r["kind"] == "observations"), None)
    if rep:
        call("POST", f"/api/v1/reports/{rep['id']}/signatures", {"meaning": "reviewed", "comment": "Temperatures within 2–8 °C except the documented delivery", "password": pw})
    call("POST", "/api/v1/audit/reviews", {"from": iso(NOW - 86400), "to": iso(int(time.time()) - 5), "outcome": "no_findings", "comment": "Daily review of changes", "password": pw})
    login()
    return {"emma_password": pw, "report": rep and rep["id"]}

def _x25519_pub(priv):
    """X25519 public key of a 32-byte private key (RFC 7748; the demo needs a valid key pair without extra modules)."""
    p, a24 = 2 ** 255 - 19, 121665
    k = bytearray(priv); k[0] &= 248; k[31] = (k[31] & 127) | 64
    k = int.from_bytes(k, "little")
    x1, x2, z2, x3, z3, swap = 9, 1, 0, 9, 1, 0
    for t in reversed(range(255)):
        kt = (k >> t) & 1
        swap ^= kt
        if swap: x2, x3, z2, z3 = x3, x2, z3, z2
        swap = kt
        A, B_ = (x2 + z2) % p, (x2 - z2) % p
        AA, BB = A * A % p, B_ * B_ % p
        E, C, D = (AA - BB) % p, (x3 + z3) % p, (x3 - z3) % p
        DA, CB = D * A % p, C * B_ % p
        x3, z3 = (DA + CB) ** 2 % p, x1 * (DA - CB) ** 2 % p
        x2, z2 = AA * BB % p, E * (AA + a24 * E) % p
    if swap: x2, z2 = x3, z3
    return (x2 * pow(z2, p - 2, p) % p).to_bytes(32, "little")


def _wg_keys():
    import base64
    priv = bytearray(os.urandom(32)); priv[0] &= 248; priv[31] = (priv[31] & 127) | 64
    return base64.b64encode(bytes(priv)).decode(), base64.b64encode(_x25519_pub(bytes(priv))).decode()


def vpn3():
    """VPN phase 3 (0.69.0): this server as a hub with a customer server connected through an uplink (a "Server
    (uplink)" peer with its LAN and two access rules), and — for the customer side of the pictures — this server's own
    uplink to another hub (a cloud hub in 10.99.0.0/16; its token is made here with fresh keys, so the uplink waits for
    a hub that does not exist) with three uplink rules. Needs what vpn() needs; runs again without duplicating."""
    import base64
    v = call("GET", "/api/v1/vpn")
    if not v.get("supported") or not v.get("licensed"):
        print("skip vpn3: needs the server agent with the VPN and a licence"); return None
    pw = os.environ.get("CTRL32_PASS", "admin1234")
    peers = {p["name"]: p for p in v["peers"]}
    name = "Customer A - Brno server"
    if name not in peers:
        call("POST", "/api/v1/vpn/peers", {"name": name, "kind": "site", "uplink": True, "subnets": ["192.168.50.0/24"], "note": "telemetry server in the Brno plant LAN, no public address", "reason": R, "password": pw})
        peers = {p["name"]: p for p in call("GET", "/api/v1/vpn")["peers"]}
    up = peers[name]["id"]
    rules = {r["name"] for r in call("GET", "/api/v1/vpn")["rules"]}
    for body in ({"name": "Technicians → Customer A web interface", "sources": [{"type": "kind", "kind": "user"}], "dest_type": "peer", "dest_peer_id": up, "protocol": "tcp", "ports": "443"},
                 {"name": "Technicians → Customer A PLC (Modbus 502)", "sources": [{"type": "kind", "kind": "user"}], "dest_type": "site", "dest_peer_id": up, "dest_cidr": "192.168.50.20", "protocol": "tcp", "ports": "502"}):
        if body["name"] not in rules:
            call("POST", "/api/v1/vpn/rules", dict(body, enabled=True, reason=R))
    have = {r["name"] for r in call("GET", "/api/v1/vpn").get("uplink_rules", [])}
    for body in ({"name": "Hub technicians → web interface", "dest_type": "server", "protocol": "tcp", "ports": "443"},
                 {"name": "Service technician → packing line PLC", "src_cidr": "10.99.0.21", "dest_type": "lan", "dest_cidr": "192.168.20.15", "protocol": "tcp", "ports": "102, 502"},
                 {"name": "Hub → ping the LAN", "dest_type": "lan", "protocol": "icmp"}):
        if body["name"] not in have:
            call("POST", "/api/v1/vpn/uplink/rules", dict(body, enabled=True, reason=R))
    if not (v.get("uplink") or {}).get("configured"):
        priv, _ = _wg_keys(); _, hub = _wg_keys()
        psk = base64.b64encode(os.urandom(32)).decode()
        tok = {"name": "Nitra plant server", "endpoint": "hub.telemetry-cloud.example:51820", "hub_public_key": hub, "preshared_key": psk, "private_key": priv,
               "address": "10.99.0.12", "vpn_net": "10.99.0.0/16", "hub_address": "10.99.0.1", "subnets": ["192.168.20.0/24"],
               "tunnel_url": "https://hub.telemetry-cloud.example/api/v1/vpn/uplink/tunnel", "tunnel_key": base64.urlsafe_b64encode(os.urandom(32)).decode().rstrip("=")}
        token = "ctrl32-uplink-v1." + base64.urlsafe_b64encode(json.dumps(tok).encode()).decode().rstrip("=")
        call("POST", "/api/v1/vpn/uplink", {"token": token, "transport": "udp", "reason": "remote service through the cloud hub (contract 2026-31)", "password": pw})
    return {"uplink_peer": up}


# ---------------------------------------------------------------------------------------------- object counting
CAMSIM = os.environ.get("CTRL32_CAMSIM", "rtsp://127.0.0.1:8554").rstrip("/")


def shift_profile(h, wd):
    """Passages of one hour of a working day (wd 0–4), Saturday (5) or Sunday (6) at the three entrances of the
    main building and the gate of the car park: shifts at 06:00 and 14:00 through the staff entrance, office and
    visitors through the main entrance, drivers at the loading dock."""
    k = 1.0 if wd < 5 else (0.25 if wd == 5 else 0.06)
    main_in = {7: 38, 8: 30, 9: 12, 10: 9, 11: 8, 12: 10, 13: 11, 14: 7, 15: 5, 16: 3}.get(h, 0)
    main_out = {9: 4, 10: 6, 11: 8, 12: 14, 13: 9, 14: 8, 15: 26, 16: 38, 17: 22, 18: 6}.get(h, 0)
    staff_in = {5: 74, 13: 68, 21: 4}.get(h, 0)
    staff_out = {14: 70, 15: 4, 22: 66}.get(h, 0)
    dock = {6: 3, 7: 4, 8: 5, 9: 4, 10: 5, 11: 4, 12: 2, 13: 4, 14: 5, 15: 3, 16: 2, 17: 1}.get(h, 0)
    cars_in = {5: 52, 6: 8, 7: 30, 8: 18, 9: 7, 10: 6, 11: 5, 12: 6, 13: 48, 14: 6, 15: 3, 21: 3}.get(h, 0)
    cars_out = {9: 3, 10: 4, 11: 5, 12: 8, 13: 6, 14: 50, 15: 22, 16: 30, 17: 16, 18: 5, 22: 46}.get(h, 0)
    trucks = {6: 2, 7: 3, 8: 4, 9: 3, 10: 4, 11: 3, 12: 2, 13: 3, 14: 4, 15: 2, 16: 2, 17: 1}.get(h, 0)
    return k, dict(main_in=main_in, main_out=main_out, staff_in=staff_in, staff_out=staff_out, dock_in=dock, dock_out=dock,
                   cars_in=cars_in, cars_out=cars_out, trucks_in=trucks, trucks_out=trucks)


@once("counting")
def counting():
    """Object counting (spec 029): the main building of the Nitra plant with three entrances and the car park with a
    gate and a speed section on the plant road; 30 days of hourly passages from people counters (light barriers and
    the gate loop, through the device protocol), vehicle speeds, a dashboard, scheduled e-mail reports and a camera
    with counting lines. CTRL32_CAMSIM: the camera simulator (default rtsp://127.0.0.1:8554); CTRL32_DEMO_DETECTOR=1
    switches the detector on for the entrance camera (camsim -clip with a recording of people)."""
    sites = call("GET", "/api/v1/sites")
    site = next((s for s in sites if s["name"] == "Nitra plant"), None)
    sid = site["id"] if site else call("POST", "/api/v1/sites", {"name": "Nitra plant", "address": "Priemyselná 5, Nitra", "reason": R})["id"]
    areas = {a["name"]: a for a in call("GET", "/api/v1/counting/areas")["areas"]}
    if "Nitra plant – main building" not in areas:
        call("POST", "/api/v1/counting/areas", {"site_id": sid, "name": "Nitra plant – main building", "kind": "building", "groups": ["people"], "reset_time": "03:00",
                                                "limits": {"people": 300}, "entrances": ["Main entrance", "Staff entrance", "Loading dock"], "reason": "people counting of the main building (fire safety: at most 300 people)"})
    if "Nitra plant – car park" not in areas:
        call("POST", "/api/v1/counting/areas", {"site_id": sid, "name": "Nitra plant – car park", "kind": "car_park", "groups": ["vehicles", "cars", "trucks"], "reset_time": "03:00",
                                                "limits": {"vehicles": 160}, "entrances": ["Gate"], "reason": "free spaces of the car park (160 spaces)"})
    areas = {a["name"]: a for a in call("GET", "/api/v1/counting/areas")["areas"]}
    bld, park = areas["Nitra plant – main building"], areas["Nitra plant – car park"]
    ent = {e["name"]: e["id"] for e in bld["entrances"]}
    gate = park["entrances"][0]["id"]
    # the counters: one device with a datastream per direction and entrance (light barriers, the gate loop)
    a = asset(sid, "sensor", "Entrance counters")
    dev, tok = device("people-counters-nitra", "People counters – Nitra plant")
    keys = ["main_in", "main_out", "staff_in", "staff_out", "dock_in", "dock_out", "cars_in", "cars_out", "trucks_in", "trucks_out"]
    ds = {k: stream(a, dev, k, "count", "", "gauge") for k in keys}
    inputs = {i["name"] for i in call("GET", "/api/v1/counting/inputs")}
    for name, en, k, g in (("Light barrier – main entrance", ent["Main entrance"], "main", "people"), ("Turnstile – staff entrance", ent["Staff entrance"], "staff", "people"),
                           ("Light barrier – loading dock", ent["Loading dock"], "dock", "people"), ("Gate loop – cars", gate, "cars", "cars"), ("Gate loop – trucks", gate, "trucks", "trucks")):
        if name not in inputs:
            call("POST", "/api/v1/counting/inputs", {"entrance_id": en, "name": name, "group": g, "in_datastream_id": ds[k + "_in"], "out_datastream_id": ds[k + "_out"], "mode": "increment", "reason": R})
    # cameras: the entrance camera counts people crossing the door line; the gate camera measures speed between two lines
    cams = {c["name"]: c["id"] for c in call("GET", "/api/v1/video/cameras")}
    if "Main entrance camera" not in cams:
        cams["Main entrance camera"] = call("POST", "/api/v1/video/cameras", {"name": "Main entrance camera", "location": "Nitra plant – main building", "url": CAMSIM + "/clip/h264/640x360", "record": "off", "reason": R})["id"]
    if "Plant road camera" not in cams:
        cams["Plant road camera"] = call("POST", "/api/v1/video/cameras", {"name": "Plant road camera", "location": "Nitra plant – car park", "url": CAMSIM + "/gate/h264/640x360", "record": "off", "reason": R})["id"]
    detector = os.environ.get("CTRL32_DEMO_DETECTOR") == "1"
    call("PUT", f"/api/v1/counting/cameras/{cams['Main entrance camera']}", {"enabled": detector, "fps": 8, "model": "nano", "config": {"anchor": "bottom", "lines": [
        {"id": "door", "name": "Door line", "a": {"x": 0.02, "y": 0.62}, "b": {"x": 0.98, "y": 0.62}, "in": "right", "classes": ["person"], "entrance_id": ent["Main entrance"]}]},
        "reason": "count visitors at the main entrance with the camera"})
    road = call("PUT", f"/api/v1/counting/cameras/{cams['Plant road camera']}", {"enabled": False, "fps": 10, "model": "nano", "config": {"anchor": "bottom", "lines": [
        {"id": "ra", "name": "Road A", "a": {"x": 0.05, "y": 0.42}, "b": {"x": 0.95, "y": 0.42}, "in": "right", "classes": ["car", "truck", "bus", "motorcycle"], "entrance_id": ""},
        {"id": "rb", "name": "Road B", "a": {"x": 0.05, "y": 0.78}, "b": {"x": 0.95, "y": 0.78}, "in": "right", "classes": ["car", "truck", "bus", "motorcycle"], "entrance_id": ""}],
        "sections": [{"id": "road", "name": "Plant road", "line_a": "ra", "line_b": "rb", "distance_m": 18, "limit_kmh": 30, "area_id": park["id"]}]},
        "reason": "speed on the plant road (limit 30 km/h)"})
    sec_asset = road["config"]["sections"][0]["asset_id"]
    sp = next(d["id"] for d in call("GET", "/api/v1/datastreams") if d["asset_id"] == sec_asset and d["key"] == "vehicle_speed")
    ol = next(d["id"] for d in call("GET", "/api/v1/datastreams") if d["asset_id"] == sec_asset and d["key"] == "over_limit")
    sdev, stok = device("plant-road-history", "Plant road – speed history import")
    for d in (sp, ol):
        call("POST", f"/api/v1/devices/{sdev}/datastreams", {"datastream_id": d, "reason": "history of the speed section (demo)"})
    # 30 days of history, an hour at a time: arrivals at :20, departures at :40 (the occupancy never needs clamping)
    rng = random.Random(29)
    start = (NOW // 3600 - 30 * 24) * 3600
    ins, outs, speeds = [], [], []
    for t in range(start, NOW - 3600, 3600):
        lt = time.localtime(t)
        k, p = shift_profile(lt.tm_hour, lt.tm_wday)
        noisy = lambda v: max(0, int(round(v * k * rng.uniform(0.8, 1.2)))) if v else 0
        vi = {key: noisy(p[key]) for key in keys if key.endswith("_in")}
        vo = {key: noisy(p[key]) for key in keys if key.endswith("_out")}
        if lt.tm_hour == 22:  # everybody leaves by the end of the second shift
            vo["staff_out"] = max(vo["staff_out"], 0)
        ins.append({"ts": t + 1200, "v": {kk: v for kk, v in vi.items() if v}})
        outs.append({"ts": t + 2400, "v": {kk: v for kk, v in vo.items() if v}})
        for _ in range(vi["cars_in"] + vi["trucks_in"]):
            s = max(8, rng.gauss(24, 5.5) + (6 if rng.random() < 0.06 else 0))
            at = t + rng.randint(0, 3599)
            speeds.append({"ts": at, "v": {"vehicle_speed": round(s, 1)}})
            if s > 30:
                speeds.append({"ts": at, "v": {"over_limit": 1}})
    # balance each day: what is left in the building or the car park at 22:40 leaves at 22:50
    occ = {"people": 0, "cars": 0, "trucks": 0}
    batch = []
    for i in range(len(ins)):
        for m, sign in ((ins[i], 1), (outs[i], -1)):
            v = m["v"]
            occ["people"] += sign * (v.get("main_in", v.get("main_out", 0)) + v.get("staff_in", v.get("staff_out", 0)) + v.get("dock_in", v.get("dock_out", 0)))
            occ["cars"] += sign * v.get("cars_in", v.get("cars_out", 0)); occ["trucks"] += sign * v.get("trucks_in", v.get("trucks_out", 0))
            for g in occ:  # no more departures than people (vehicles) inside
                if occ[g] < 0:
                    key = {"people": "staff_out", "cars": "cars_out", "trucks": "trucks_out"}[g]
                    v[key] = max(0, v.get(key, 0) + occ[g]); occ[g] = 0
            if m["v"]: batch.append(m)
        if time.localtime(ins[i]["ts"]).tm_hour == 22:
            rest = {"staff_out": occ["people"], "cars_out": occ["cars"], "trucks_out": occ["trucks"]}
            rest = {kk: v for kk, v in rest.items() if v > 0}
            if rest: batch.append({"ts": ins[i]["ts"] + 1800, "v": rest})
            occ = {"people": 0, "cars": 0, "trucks": 0}
    for j in range(0, len(batch), 100):
        call("POST", "/api/v1/d/telemetry", batch[j:j + 100], token=tok)
    speeds.sort(key=lambda s: s["ts"])
    for j in range(0, len(speeds), 500):
        call("POST", "/api/v1/d/telemetry", speeds[j:j + 500], token=stok)
    # the capacity alarm of the building (the existing alarm rules on the occupancy datastream)
    occ_ds = next(d["id"] for d in call("GET", "/api/v1/datastreams") if d["asset_id"] == bld["asset_id"] and d["key"] == "people_occupancy")
    rules = call("GET", f"/api/v1/alarm-rules?datastream_id={occ_ds}") or []
    if not rules:
        call("POST", "/api/v1/alarm-rules", {"datastream_id": occ_ds, "type": "high", "warning": 270, "action": 300, "reason": "fire safety: at most 300 people in the main building"})
    return {"site": sid, "building": bld["id"], "car_park": park["id"], "entrance_camera": cams["Main entrance camera"], "road_camera": cams["Plant road camera"], "token": tok, "speed": sp}


def counting_views():
    """The dashboard "Visitors and vehicles" and the scheduled e-mail reports of the counting demo (run again: a new
    version of the dashboard, existing reports kept)."""
    c = st["counting"]
    areas = {a["id"]: a for a in call("GET", "/api/v1/counting/areas")["areas"]}
    bld, park, sp = areas[c["building"]], areas[c["car_park"]], c["speed"]
    occ_ds = next(d["id"] for d in call("GET", "/api/v1/datastreams") if d["asset_id"] == bld["asset_id"] and d["key"] == "people_occupancy")
    # a dashboard of visitors and vehicles
    dsl = {(d["asset_id"], d["key"]): d["id"] for d in call("GET", "/api/v1/datastreams")}
    pin, pout = dsl[(bld["asset_id"], "people_in")], dsl[(bld["asset_id"], "people_out")]
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 96}, "range": "24h", "refresh_s": 60, "widgets": [
        w("occ", "counting_occupancy", 0, 0, 3, 2, [occ_ds], title="Main building now", group="people"),
        w("park", "counting_occupancy", 3, 0, 3, 2, [dsl[(park["asset_id"], "vehicles_occupancy")]], title="Car park", group="vehicles"),
        w("gauge", "radial_gauge", 6, 0, 3, 2, [occ_ds], title="Occupancy", y_min=0, y_max=320, warn=270, action=300, decimals=0),
        w("share", "counting_share", 9, 0, 3, 2, [occ_ds], title="Arrivals by entrance (yesterday)", group="people", day="yesterday"),
        w("ent", "counting_entrances", 0, 2, 6, 3, [occ_ds], title="Entrances yesterday", group="people", day="yesterday"),
        w("trend", "chart", 6, 2, 6, 3, [pin, pout], title="Arrivals and departures per day", agg="sum", agg_interval="1d", chart="bar", range="7d", series=[{"label": "In"}, {"label": "Out"}]),
        w("speed", "vehicle_speeds", 0, 5, 6, 4, [sp], title="Plant road yesterday (limit 30 km/h)", day="yesterday"),
        w("days", "period_table", 6, 5, 6, 4, [pin, pout], title="People per day", interval="day", values="sum", period="last_7d", total=True, decimals=0, series=[{"label": "In"}, {"label": "Out"}])]}
    slug = dash("Visitors and vehicles", spec)
    st["counting"]["dashboard"] = slug; save()
    # scheduled e-mail reports
    reps = {r["name"] for r in call("GET", "/api/v1/scheduled-reports")["reports"]}
    if "Main building – daily visitors" not in reps:
        r1 = call("POST", "/api/v1/scheduled-reports", {"name": "Main building – daily visitors", "content": {"type": "counting", "area_id": bld["id"]}, "period": "previous_day", "cron": "0 7 * * *",
                                                        "recipients": {"addresses": ["facility.manager@nitra-plant.example", "security@nitra-plant.example"]}, "attach_pdf": True, "attach_xlsx": True, "reason": "daily overview for the facility manager"})["id"]
        call("POST", f"/api/v1/scheduled-reports/{r1}/send")
    if "Car park – weekly" not in reps:
        call("POST", "/api/v1/scheduled-reports", {"name": "Car park – weekly", "content": {"type": "counting", "area_id": park["id"]}, "period": "previous_week", "cron": "0 7 * * 1",
                                                   "recipients": {"addresses": ["logistics@nitra-plant.example"]}, "attach_pdf": True, "attach_xlsx": True, "reason": "weekly overview of the car park and the plant road"})
    if "People per day – monthly" not in reps:
        call("POST", "/api/v1/scheduled-reports", {"name": "People per day – monthly", "content": {"type": "period_table", "dashboard": slug, "widget": "days"}, "period": "previous_month", "cron": "0 7 1 * *",
                                                   "recipients": {"addresses": ["facility.manager@nitra-plant.example"]}, "attach_pdf": False, "attach_xlsx": True, "reason": "monthly table for the management report"})
    return slug


def setup():
    login()
    partitions()
    cold_chain(); energy(); dashboards(); overviews(); drilldown(); asset_tree(); rules(); flows(); mqtt_accounts(); home(); display(); firmware(); people(); settings(); incidents(); records()
    counting(); counting_views()
    st["vpn"] = vpn(); st["vpn2"] = vpn2(); st["vpn3"] = vpn3(); save()
    print("docs demo ready:", ", ".join(st))


def live():
    """A sample every 60 s for the cold chain and the energy meters."""
    rng = random.Random()
    c, e = st["cold"], st["energy"]
    while True:
        t = int(time.time())
        try:
            a, b = room(t, rng)
            call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"temp": a}}, token=c["t0"])
            call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"temp": fridge_temp("f1", t, rng), "rh": b}}, token=c["esp_token"])
            call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"temp": fridge_temp("f2", t, rng, c["excursion_until"])}}, token=c["t2"])
            call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"temp": fridge_temp("freezer", t, rng, c["excursion_until"])}}, token=c["t3"])
            for m in e.values():
                m["cum"] += meter_kwh(m["kind"], m["peak"], t, rng) / 60
                call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"energy": round(m["cum"], 3)}}, token=m["token"])
            # keep the meter readings; everything else in the state file belongs to setup
            cur = json.load(open(STATE, encoding="utf-8")); cur["energy"] = e
            json.dump(cur, open(STATE, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
        except SystemExit as x: print(x, file=sys.stderr)
        time.sleep(60)


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else "setup"
    if cmd == "mqtt-creds":
        m = st["mqtt"]; print(m["auto"]["user"], m["auto"]["pass"], m["inbox"]["user"], m["inbox"]["pass"])
    else:
        {"setup": setup, "live": live, "vpn": lambda: (login(), st.update(vpn=vpn(), vpn2=vpn2(), vpn3=vpn3()), save()), "counting": lambda: (login(), counting(), counting_views())}[cmd]()
