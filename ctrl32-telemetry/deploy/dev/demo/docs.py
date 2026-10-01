#!/usr/bin/env python3
"""Demo for the product documentation: fills the screens the other demo scripts leave empty or thin.

  docs.py setup        cold chain (medicine fridges, 30 days), energy meters (13 months), alarm rules with alarms and
                       incidents, notification channels and escalation, automation rules, flows, smart-home accounts,
                       a display, firmware and a campaign, users and a custom role, an API token, a report template and
                       stored reports, a PWA app, the "overview" and "showcase" starter dashboards
  docs.py live         keeps the cold chain and the energy meters current (a sample every 60 s)
  docs.py mqtt-creds   prints "user pass" of the smart-home MQTT accounts (used by dev.sh to start the simulators)

Idempotent: what is recorded in .dev/docs.json (or found by name) is skipped, so setup can be run again.
Nothing sensitive is switched on (no audio, no unmasked video, AI access stays as it is)."""
import json, math, os, random, subprocess, sys, time, uuid
from common import B, call, login, state_path
import common, urllib.request

STATE = state_path("docs.json")
R = "ukážka pre dokumentáciu"
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
    """Month partitions of core.observation for the history (the server keeps only the current and next months)."""
    q = "SELECT core.ensure_month_partitions('core.observation', (current_date - interval '14 months')::date, 14)"
    url = os.environ.get("CTRL32_DB_ADMIN_URL")
    cmd = ["psql", "-q", url, "-c", q] if url else ["docker", "exec", "ctrl32-pg-dev", "psql", "-q", "-U", "ctrl32", "-d", "ctrl32_dev", "-c", q]
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
    site = next((s for s in sites if s["name"] in ("Demo site", "Lekáreň Nitra – sklad liekov")), None)
    if site:
        call("PATCH", f"/api/v1/sites/{site['id']}", {"name": "Lekáreň Nitra – sklad liekov", "address": "Štefánikova 12, Nitra", "reason": R})
        sid = site["id"]
    else:
        sid = call("POST", "/api/v1/sites", {"name": "Lekáreň Nitra – sklad liekov", "address": "Štefánikova 12, Nitra", "reason": R})["id"]
    assets = {a["name"]: a for a in call("GET", "/api/v1/assets")}
    ren = {"Fridge 1": "Chladnička liekov 1", "Room 1": "Sklad liekov – miestnosť"}
    for old, new in ren.items():
        if old in assets:
            call("PATCH", f"/api/v1/assets/{assets[old]['id']}", {"name": new, "reason": R})
            assets[new] = assets.pop(old)
    dss = {(d.get("asset"), d["key"]): d["id"] for d in call("GET", "/api/v1/datastreams")}
    # the seed device esp32-demo feeds fridge 1 and the room
    esp = next(x for x in call("GET", "/api/v1/devices") if x["external_id"] == "esp32-demo")
    call("PATCH", f"/api/v1/devices/{esp['id']}", {"name": "Záznamník teploty – sklad (ESP32)", "location": {"lat": 48.3076, "lon": 18.0858}, "reason": R})
    esp_tok = call("POST", f"/api/v1/devices/{esp['id']}/token", {"reason": R})["token"]
    f1 = dss.get(("Chladnička liekov 1", "temp")) or dss.get(("Fridge 1", "temp"))
    rt = dss.get(("Sklad liekov – miestnosť", "temp")) or dss.get(("Room 1", "temp"))
    rh = dss.get(("Sklad liekov – miestnosť", "rh")) or dss.get(("Room 1", "rh"))
    a2 = asset(sid, "fridge", "Chladnička liekov 2", {"storage_class": "2-8C", "model": "Liebherr MKv 3910"})
    a3 = asset(sid, "freezer", "Mraznička vakcín", {"storage_class": "-25..-15C"})
    d2, t2 = device("logger-t2", "Záznamník teploty T2", loc=(48.3079, 18.0861))
    d3, t3 = device("logger-t3", "Záznamník teploty T3", loc=(48.3073, 18.0866))
    f2 = stream(a2, d2, "temp", "temperature", "°C", interval="15 minutes")
    fz = stream(a3, d3, "temp", "temperature", "°C", interval="15 minutes")
    call("PUT", f"/api/v1/devices/{d2}/attributes", {"attributes": {"interval_s": 900, "probe": "PT1000 v glykole"}, "reason": R})
    # notification channels and an escalation policy (not attached to the rules: without an SMTP server every alarm
    # would open an "alarm not delivered" incident), then alarm rules before the history so it raises and clears alarms
    ch1 = named("/api/v1/notification-channels", "Lekárnik – e-mail", {"kind": "email", "settings": {"recipients": ["lekarnik@lekaren-nitra.example"]}})
    ch2 = named("/api/v1/notification-channels", "Dispečing – webhook", {"kind": "webhook", "settings": {"url": "https://dispecing.example/hooks/ctrl32", "secret": uuid.uuid4().hex}})
    pol = named("/api/v1/escalation-policies", "Chladový reťazec – eskalácia", {"steps": [{"after_s": 0, "channel_id": ch1}, {"after_s": 900, "channel_id": ch2}]})
    have = {(r["datastream_id"], r["type"]) for r in call("GET", "/api/v1/alarm-rules")}
    for ds, typ, warn, act in [(f2, "high", 7.5, 8), (f2, "low", 2.5, 2), (fz, "high", -18, -15)]:
        if (ds, typ) not in have:
            call("POST", "/api/v1/alarm-rules", {"datastream_id": ds, "type": typ, "warning": warn, "action": act, "hysteresis": 0.2, "delay_seconds": 0, "reason": R})
    if (f2, "comm_loss") not in have:
        call("POST", "/api/v1/alarm-rules", {"datastream_id": f2, "type": "comm_loss", "delay_seconds": 3600, "reason": R})
    # the room temperature gets a logger of its own (the seed device carries the fridge and the room humidity)
    d0, t0 = device("logger-room", "Záznamník – miestnosť skladu")
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


# ---------------------------------------------------------------------------------------------- energy
METERS = [("L1", "Elektromer – Linka 1", 46, "line"), ("L2", "Elektromer – Linka 2", 31, "line"),
          ("KOMP", "Elektromer – kompresorovňa", 17, "base"), ("ADM", "Elektromer – administratíva a osvetlenie", 9, "office"),
          ("FVE", "Fotovoltika – strecha haly", 80, "solar")]


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
    site = next(s for s in call("GET", "/api/v1/sites") if s["name"] == "Závod Nitra")["id"]
    sources, ids = [], {}
    start = (NOW - 400 * 86400) // 3600 * 3600
    for i, (code, name, peak, kind) in enumerate(METERS):
        a = asset(site, "meter", name)
        d, tok = device("em-" + code.lower(), name, loc=(48.2905 + i * 0.0006, 18.1168 + i * 0.0009))
        ds = stream(a, d, "energia", "energy", "kWh", kind="counter")
        rng, cum, samples = random.Random(100 + i), 10000.0 * (i + 1), []
        for t in range(start, NOW, 3600):
            samples.append({"ts": t, "v": {"energia": round(cum, 2)}})
            cum += meter_kwh(kind, peak, t, rng)
        samples.append({"ts": NOW, "v": {"energia": round(cum, 2)}})
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
        w("v1", "value", 0, 0, 3, 2, [c["f1"]], title="Chladnička liekov 1", decimals=1, sparkline=True),
        w("v2", "value", 3, 0, 3, 2, [c["f2"]], title="Chladnička liekov 2", decimals=1, sparkline=True),
        w("v3", "value", 6, 0, 3, 2, [c["fz"]], title="Mraznička vakcín", decimals=1, sparkline=True),
        w("al", "alarms", 9, 0, 3, 4, None, title="Alarmy", limit=10),
        w("ch", "chart", 0, 2, 9, 3, [c["f1"], c["f2"]], title="Teplota chladničiek (7 dní)", range="7d", band=True, legend=True, series=[{"label": "Chladnička 1"}, {"label": "Chladnička 2"}]),
        w("pt", "period_table", 0, 5, 8, 5, [c["f1"], c["f2"]], title="Denný záznam teplôt (min / max, limity 2–8 °C)", interval="day", values="min,max", period="last_30d", low=2, high=8, total=True, decimals=1, series=[{"label": "Chladnička 1"}, {"label": "Chladnička 2"}]),
        w("rm", "chart", 8, 5, 4, 3, [c["rt"], c["rh"]], title="Sklad – teplota a vlhkosť", range="7d", series=[{"label": "°C"}, {"label": "% RH"}]),
        w("hm", "heatmap", 8, 8, 4, 2, [c["f2"]], title="Chladnička 2 – hodina × deň", range="7d"),
    ]
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "7d", "refresh_s": 60, "widgets": cold}
    out["cold"] = dash("Chladový reťazec – sklad liekov", spec)
    cons = [e[k]["ds"] for k in ("L1", "L2", "KOMP", "ADM")]
    lbl = [{"label": "Linka 1"}, {"label": "Linka 2"}, {"label": "Kompresorovňa"}, {"label": "Administratíva"}]
    en = [
        w("a1", "agg_card", 0, 0, 3, 2, [e["L1"]["ds"]], title="Linka 1 – spotreba za 30 dní", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a2", "agg_card", 3, 0, 3, 2, [e["L2"]["ds"]], title="Linka 2 – spotreba za 30 dní", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a3", "agg_card", 6, 0, 3, 2, [e["KOMP"]["ds"]], title="Kompresorovňa za 30 dní", agg="delta", range="30d", icon="bolt", decimals=0, unit="kWh"),
        w("a4", "agg_card", 9, 0, 3, 2, [e["FVE"]["ds"]], title="Fotovoltika – výroba za 30 dní", agg="delta", range="30d", icon="sun", decimals=0, unit="kWh", color="#16a34a"),
        w("pd", "period_table", 0, 2, 12, 5, cons + [e["FVE"]["ds"]], title="Denná spotreba – posledných 7 dní (kWh)", interval="day", values="delta", period="last_7d", total=True, decimals=0, series=lbl + [{"label": "Fotovoltika"}]),
        w("pt", "period_table", 0, 7, 12, 7, cons + [e["FVE"]["ds"]], title="Mesačná spotreba a výroba (kWh)", interval="month", values="delta", period="last_12m", total=True, decimals=0, series=lbl + [{"label": "Fotovoltika"}]),
    ]
    spec = {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "30d", "refresh_s": 300, "widgets": en}
    out["energy"] = dash("Energie – mesačná spotreba", spec)
    for kind, name in (("overview", "Prehľad"), ("showcase", "Ukážka všetkých widgetov")):
        s = call("GET", f"/api/v1/dashboards/starter?kind={kind}")
        out[kind] = dash(name, s["spec"])
    return out


# ---------------------------------------------------------------------------------------------- alarms and incidents
@once("incidents")
def incidents():
    time.sleep(2)
    alarms = call("GET", "/api/v1/alarms?state=active") or []
    if isinstance(alarms, dict): alarms = alarms.get("items", [])
    acked = None
    for a in alarms:
        if a.get("state") == "active" and a.get("severity") == "warning":
            call("POST", f"/api/v1/alarms/{a['id']}/ack", {"reason": "Mraznička otvorená pri vyskladnení, sledujem"}); acked = a["id"]; break
    # incidents the server opened for alarms it could not deliver (no SMTP here): close all but the newest
    auto = [i for i in call("GET", "/api/v1/incidents") if i["kind"] == "alarm_undelivered" and i["status"] == "open"]
    for i in auto[1:]:
        call("PATCH", f"/api/v1/incidents/{i['id']}", {"status": "closed", "cause": "Ukážkový server nemá nastavený SMTP server.", "capa": "SMTP nastaviť v config.toml pred ostrou prevádzkou.", "reason": R})
    i1 = call("POST", "/api/v1/incidents", {"title": "Chladnička liekov 2 – teplota nad 8 °C", "severity": "critical", "impact": "Inzulíny a vakcíny v chladničke 2 mimo 2–8 °C, karanténa do posúdenia.", "reason": R})["id"]
    i2 = call("POST", "/api/v1/incidents", {"title": "Linka 1 – opakované zastavenia lisu", "severity": "major", "impact": "Strata cca 6 % výkonu zmeny.", "reason": R})["id"]
    call("PATCH", f"/api/v1/incidents/{i2}", {"status": "investigating", "note": "Údržba kontroluje snímač polohy.", "reason": R})
    i3 = call("POST", "/api/v1/incidents", {"title": "Výpadok záznamníka T3 (batéria)", "severity": "minor", "reason": R})["id"]
    call("PATCH", f"/api/v1/incidents/{i3}", {"status": "closed", "cause": "Vybitá batéria záznamníka.", "capa": "Výmena batérií raz ročne zaradená do plánu údržby.", "reason": R})
    return {"acked_alarm": acked, "ids": [i1, i2, i3]}


# ---------------------------------------------------------------------------------------------- automation
@once("rules")
def rules():
    c = st["cold"]
    prod = json.load(open(state_path("production.json"), encoding="utf-8")) if os.path.exists(state_path("production.json")) else None
    out = []
    spec = {"inputs": [{"name": "t", "datastream_id": c["f2"]}, {"name": "room", "datastream_id": c["rt"]}], "match": "all",
            "conditions": [{"input": "t", "op": ">", "value": 7.5}, {"input": "room", "op": "<", "value": 26}],
            "actions": [{"type": "notify", "channel_id": c["ch1"], "subject": "Chladnička 2: {t} °C – skontrolujte dvere"}, {"type": "alarm", "severity": "warning"}],
            "clear_actions": [{"type": "notify", "channel_id": c["ch1"], "subject": "Chladnička 2 je opäť v limite"}]}
    out.append(call("POST", "/api/v1/rules", {"name": "Chladnička 2 – otvorené dvere", "description": "Teplota stúpa, hoci miestnosť nie je prehriata", "spec": spec, "reason": R})["id"])
    if prod:
        ds = prod["lines"][0]["ds"]
        spec = {"inputs": [{"name": "teplota", "datastream_id": ds["teplota"]}, {"name": "stav", "datastream_id": ds["stav"]}], "match": "all",
                "conditions": [{"input": "teplota", "op": ">=", "value": 70}, {"input": "stav", "op": "==", "value": 1}],
                "actions": [{"type": "alarm", "severity": "action"}, {"type": "notify", "channel_id": c["ch2"], "subject": "Lis linky 1 sa prehrieva: {teplota} °C"}]}
        out.append(call("POST", "/api/v1/rules", {"name": "Linka 1 – prehriatie lisu", "description": "Teplota lisu nad 70 °C počas behu", "spec": spec, "reason": R})["id"])
    return out


def flow(name, spec):
    old = next((f["flow"] for f in call("GET", "/api/v1/flows") if f["flow"]["name"] == name), None)
    if old:
        call("PUT", f"/api/v1/flows/{old['slug']}", {"spec": spec, "reason": "prehľadnejšie rozloženie uzlov"}); return old["slug"]
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
        {"id": "mail", "type": "notify", "x": 560, "y": 10, "settings": {"channel": c["ch1"], "subject": "Chladnička 2: {{ fixed(value, 1) }} °C dlhšie ako 10 min"}},
        {"id": "inc", "type": "incident", "x": 560, "y": 110, "settings": {"action": "open", "severity": "major", "title": "Chladnička 2 mimo 2–8 °C"}},
        {"id": "dbg", "type": "debug", "x": 740, "y": 10, "settings": {"expr": "'odoslané ' + str(value)"}},
        {"id": "note", "type": "comment", "x": 20, "y": 180, "settings": {"text": "Teplota nad 8 °C dlhšie ako 10 minút → e-mail lekárnikovi a incident (najviac raz za hodinu)."}}],
        "wires": [{"from": "t", "port": 0, "to": "hot"}, {"from": "hot", "port": 0, "to": "rl"}, {"from": "rl", "port": 0, "to": "mail"},
                  {"from": "rl", "port": 0, "to": "inc"}, {"from": "mail", "port": 0, "to": "dbg"}]}
    out.append(flow("Chladnička 2 – upozornenie lekárnikovi", spec))
    spec = {"nodes": [
        {"id": "s", "type": "schedule", "x": 40, "y": 60, "settings": {"mode": "daily", "times": "06:00", "weekdays": ["mon", "tue", "wed", "thu", "fri"]}},
        {"id": "m", "type": "template", "x": 220, "y": 60, "settings": {"text": "Ranný prehľad energie: {{ fmtTime(now(), '2006-01-02') }}", "target": "text"}},
        {"id": "n", "type": "notify", "x": 420, "y": 60, "settings": {"channel": c["ch2"], "subject": "Ranný prehľad", "text": "{{ msg.text }}"}},
        {"id": "p", "type": "ds_value", "x": 40, "y": 180, "settings": {"datastream": e["FVE"]["ds"], "min_interval_s": 900}},
        {"id": "d", "type": "deadband", "x": 220, "y": 180, "settings": {"band": 5}},
        {"id": "g", "type": "debug", "x": 420, "y": 180, "settings": {"expr": "'FVE ' + fixed(value, 0) + ' kWh'"}}],
        "wires": [{"from": "s", "port": 0, "to": "m"}, {"from": "m", "port": 0, "to": "n"}, {"from": "p", "port": 0, "to": "d"}, {"from": "d", "port": 0, "to": "g"}]}
    out.append(flow("Energie – ranný prehľad", spec))
    for slug in out:
        try: call("POST", f"/api/v1/flows/{slug}/deploy", {"reason": R})
        except SystemExit as x: print("deploy", slug, x)
    return out


# ---------------------------------------------------------------------------------------------- smart home
@once("mqtt")
def mqtt_accounts():
    a = call("POST", "/api/v1/mqtt-sources", {"kind": "account", "name": "Rodinný dom – Zigbee a Tasmota", "adopt": "auto", "reason": R})
    b = call("POST", "/api/v1/mqtt-sources", {"kind": "account", "name": "Kancelária – nové zariadenia", "adopt": "inbox", "reason": R})
    p = call("POST", "/api/v1/provision-profiles", {"name": "Senzory teploty – sklad", "strategy": "create", "transport": "http", "approval": True, "auto_datastreams": True, "max_devices": 50, "name_pattern": "Senzor {serial}", "site_id": st["cold"]["site"], "reason": R})
    return {"auto": {"id": a["id"], "user": a["username"], "pass": a["password"]}, "inbox": {"id": b["id"], "user": b["username"], "pass": b["password"]}, "profile": p.get("id")}


def simulators(wait=True):
    """Builds tools/mqttsim and starts the zigbee2mqtt, Tasmota and Shelly simulators (run for a day)."""
    root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
    exe = os.path.join(root, ".dev", "mqttsim.exe" if os.name == "nt" else "mqttsim")
    subprocess.run(["go", "build", "-o", exe, "./tools/mqttsim"], cwd=root, check=True)
    m = st["mqtt"]
    procs = []
    for args in (["z2m", "-user", m["auto"]["user"], "-pass", m["auto"]["pass"]],
                 ["tasmota", "-user", m["auto"]["user"], "-pass", m["auto"]["pass"], "-id", "KUCHYNA"],
                 ["shelly", "-user", m["inbox"]["user"], "-pass", m["inbox"]["pass"], "-id", "KANCELARIA1"]):
        log = open(os.path.join(root, ".dev", "homesim-" + args[0] + ".log"), "w")
        procs.append(subprocess.Popen([exe] + args + ["-wait", "86400"], stdout=log, stderr=subprocess.STDOUT))
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
        sc.append(call("POST", "/api/v1/scenes", {"name": "Večer", "icon": "moon", "capture": lights, "reason": R})["id"])
        sc.append(call("POST", "/api/v1/scenes", {"name": "Odchod z domu", "icon": "door", "actions": [{"entity_id": i, "service": "turn_off"} for i in lights], "reason": R})["id"])
    return {"entities": len(ents), "scenes": sc}


# ---------------------------------------------------------------------------------------------- the rest
def video_walls():
    """Ids of the demo video walls: the control room first, then the gatehouse (empty without cameras.py)."""
    ws = sorted(call("GET", "/api/v1/video/walls"), key=lambda x: 0 if x["name"].startswith("Vel") else 1)
    return [x["id"] for x in ws]


@once("display")
def display():
    hello = json.loads(urllib.request.urlopen(urllib.request.Request(B + "/api/v1/display/hello", method="POST", data=b"{}", headers={"Content-Type": "application/json"})).read())
    walls = video_walls()
    items = [{"kind": "dashboard", "slug": st["dashboards"]["cold"]}]
    if walls: items.insert(0, {"kind": "wall", "wall": walls[1], "screen": 0})
    d = call("POST", "/api/v1/displays/pair", {"code": hello["code"], "name": "Vrátnica – TV", "location": "Areál Nitra, vrátnica", "offline_alert": False, "content": {"items": items}, "reason": R})
    return d.get("id")


@once("firmware")
def firmware():
    blob = os.urandom(4096) + b"ctrl32-demo-firmware"
    bnd = uuid.uuid4().hex
    fields = {"name": "Záznamník teploty", "version": "1.5.0", "hw": "esp32-logger", "notes": "Kratší interval pri prekročení limitu, opravené hlásenie batérie.", "reason": R}
    body = b"".join(f"--{bnd}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode() for k, v in fields.items())
    body += f"--{bnd}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"logger-1.5.0.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n".encode() + blob + f"\r\n--{bnd}--\r\n".encode()
    fw = json.loads(raw("POST", "/api/v1/firmware", body, "multipart/form-data; boundary=" + bnd))
    camp = call("POST", "/api/v1/fota/campaigns", {"name": "Záznamníky – jeseň 2026", "firmware_id": fw["id"], "device_ids": [st["cold"]["esp"]], "reason": R})
    call("POST", f"/api/v1/devices/{st['cold']['esp']}/commands", {"method": "reboot", "args": {"delay_s": 5}, "ttl_seconds": 600, "reason": "reštart po zmene intervalu"})
    return {"firmware": fw["id"], "campaign": camp.get("id")}


@once("people")
def people():
    role = by_name("/api/v1/roles", "technik_chladenia")
    role = role["id"] if role else call("POST", "/api/v1/roles", {"name": "technik_chladenia", "permissions": ["data.read", "alarm.ack", "data.export", "device.manage"], "reason": R})["id"]
    have = {u["username"] for u in call("GET", "/api/v1/users")}
    users = []
    for u, n, r in (("jana.kovacova", "Jana Kováčová", ["operator"]), ("peter.horvath", "Peter Horváth", ["engineer"]),
                    ("eva.mrazova", "Eva Mrázová", ["qa"]), ("martin.balaz", "Martin Baláž", ["technik_chladenia"])):
        if u not in have:
            call("POST", "/api/v1/users", {"username": u, "full_name": n, "email": u + "@zavod-nitra.example", "roles": r, "reason": R})
        users.append(u)
    if not by_name("/api/v1/tokens", "Power BI – export dát"):
        call("POST", "/api/v1/tokens", {"name": "Power BI – export dát", "scopes": ["data.read", "data.export"]})
    return {"role": role, "users": users}


@once("settings")
def settings():
    c = st["cold"]
    out = {}
    con = by_name("/api/v1/connectors", "LoRaWAN – ChirpStack areál") or call("POST", "/api/v1/connectors", {"kind": "chirpstack_http", "name": "LoRaWAN – ChirpStack areál", "auto_create_devices": True, "reason": R})
    out["connector"] = con.get("id")
    walls = video_walls()
    app = {"slug": "velin", "name": "Velín – kamery", "short_name": "Velín", "start_url": f"/video/walls/{walls[0]}" if walls else "/video",
           "sections": ["cameras", "dashboards", "incidents"], "theme_color": "#0e7490", "camera_layout": "grid", "reason": R}
    apps = {a["slug"] for a in call("GET", "/api/v1/pwa-apps")}
    if "velin" not in apps: call("POST", "/api/v1/pwa-apps", app)
    out["app"] = "velin"
    if "lieky" not in apps: call("POST", "/api/v1/pwa-apps", {"slug": "lieky", "name": "Sklad liekov", "short_name": "Lieky", "start_url": f"/dashboards/{st['dashboards']['cold']}", "sections": ["dashboards", "incidents"], "locked": True, "reason": R})
    # report template, then two stored reports
    spec = call("GET", "/api/v1/report-templates/observations/default-spec")["spec"]
    call("PUT", "/api/v1/report-templates/observations", {"name": "Protokol teplôt – sklad liekov", "locale": "sk", "spec": spec, "reason": R})
    to = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(NOW)); fr = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(NOW - 7 * 86400))
    try:
        raw("GET", f"/api/v1/export/observations?datastream_id={c['f2']}&from={fr}&to={to}&format=pdf")
        raw("GET", f"/api/v1/period-table/export?datastreams={c['f1']},{c['f2']}&interval=day&values=min,max&period=last_month&low=2&high=8&total=1&format=pdf&title=Teploty%20chladni%C4%8Diek")
    except Exception as x: print("reports:", x)
    # a pending change request for the approvals screen: four-eyes on limits, one rule change, policy off again
    call("PUT", "/api/v1/settings/policy", {"four_eyes_limits": True, "reason": R})
    try: call("POST", "/api/v1/alarm-rules", {"datastream_id": c["f1"], "type": "high", "warning": 7.0, "action": 8, "hysteresis": 0.2, "reason": "sprísnenie varovania podľa SOP-12"})
    finally: call("PUT", "/api/v1/settings/policy", {"four_eyes_limits": False, "reason": R})
    return out


def setup():
    login()
    partitions()
    cold_chain(); energy(); dashboards(); rules(); flows(); mqtt_accounts(); home(); display(); firmware(); people(); settings(); incidents()
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
                call("POST", "/api/v1/d/telemetry", {"ts": t, "v": {"energia": round(m["cum"], 3)}}, token=m["token"])
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
        {"setup": setup, "live": live}[cmd]()
