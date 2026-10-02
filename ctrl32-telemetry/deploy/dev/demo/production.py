#!/usr/bin/env python3
"""Demo: production lines (counts, scrap, speed, state) with 8 h of history, a production dashboard, a SCADA screen of
line 1, and live data every 10 s. Usage: production.py setup | production.py live | production.py dash"""
import json, math, random, sys, time
from common import call, login, state_path

STATE = state_path("production.json")

LINES = [("Line 1 – press shop", "L1", 12.0, 4800), ("Line 2 – assembly", "L2", 9.0, 3600)]
STREAMS = [("pieces", "count", "pcs", "gauge"), ("pieces_shift", "count", "pcs", "gauge"), ("scrap", "count", "pcs", "gauge"),
           ("speed", "rate", "pcs/min", "gauge"), ("state", "state", "", "gauge"), ("temperature", "temperature", "°C", "gauge"),
           ("pressure", "pressure", "bar", "gauge"), ("level", "level", "%", "gauge"), ("valve", "state", "", "gauge")]

def model(line, t, rng):
    """Values of a line at unix time t (minute resolution)."""
    _, code, speed, _ = line
    minute = int(t // 60)
    h = (t / 3600) % 24
    # a short stop now and then, a fault once in a while
    phase = (minute * (7 if code == "L1" else 11)) % 97
    state = 2 if phase in (13, 14) else 0 if phase in (40, 41, 42) else 1
    v = speed * (0.9 + 0.2 * math.sin(t / 900)) + rng.uniform(-0.6, 0.6) if state == 1 else 0
    pieces = max(0, round(v))
    scrap = 1 if pieces and rng.random() < 0.08 else 0
    temp = 58 + 6 * math.sin(t / 1800) + (8 if state == 2 else 0) + rng.uniform(-0.4, 0.4)
    press = (6.2 + 0.4 * math.sin(t / 700)) if state == 1 else 1.1
    level = 55 + 30 * math.sin(t / 2400)
    return {"pieces": pieces, "scrap": scrap, "speed": round(v, 1), "state": state, "temperature": round(temp, 1),
            "pressure": round(press, 2), "level": round(level, 1), "valve": 1 if state == 1 else 0}

def setup():
    login()
    site = call("POST", "/api/v1/sites", {"name": "Nitra plant", "reason": "production demo"})["id"]
    st = {"lines": []}
    for name, code, speed, plan in LINES:
        asset = call("POST", "/api/v1/assets", {"site_id": site, "type": "machine", "name": name, "reason": "production demo"})["id"]
        dev = call("POST", "/api/v1/devices", {"external_id": "plc-" + code.lower(), "name": f"PLC {name}", "transport": "http", "reason": "production demo"})["id"]
        tok = call("POST", f"/api/v1/devices/{dev}/token", {"reason": "production demo"})["token"]
        ds = {}
        for key, q, unit, kind in STREAMS:
            d = call("POST", "/api/v1/datastreams", {"asset_id": asset, "key": key, "quantity": q, "unit": unit, "kind": kind, "reason": "production demo"})
            ds[key] = d["id"]
            call("POST", f"/api/v1/devices/{dev}/datastreams", {"datastream_id": d["id"], "reason": "production demo"})
        st["lines"].append({"name": name, "code": code, "plan": plan, "token": tok, "ds": ds})
    # 8 hours of history, one sample a minute
    now = int(time.time()) // 60 * 60
    start = now - 8 * 3600
    for i, line in enumerate(LINES):
        rng = random.Random(i)
        cum, batch = 0, []
        for t in range(start, now, 60):
            v = model(line, t, rng); cum += v["pieces"]
            v["pieces_shift"] = cum
            batch.append({"ts": t, "v": v, "bf": 1})
            if len(batch) == 60:
                call("POST", "/api/v1/d/telemetry", batch, token=st["lines"][i]["token"]); batch = []
        if batch: call("POST", "/api/v1/d/telemetry", batch, token=st["lines"][i]["token"])
        st["lines"][i]["cum"] = cum
    json.dump(st, open(STATE, "w"))
    dashboards(st)

def w(id, typ, x, y, ww, hh, ds, **p):
    return {"id": id, "type": typ, "geometry": {"x": x, "y": y, "w": ww, "h": hh}, "bindings": {"datastreams": ds} if ds else {}, "params": p}

def dashboards(st):
    L1, L2 = st["lines"]
    states = ["0=Stopped:#f59e0b", "1=Running:#16a34a", "2=Fault:#dc2626"]
    widgets = [
        w("clk", "clock", 0, 0, 3, 2, None, title="Shift A · 06:00 – 14:00"),
        w("a1", "agg_card", 3, 0, 3, 2, [L1["ds"]["pieces"]], title="Line 1 – produced in 8 h", agg="sum", range="8h", icon="package", decimals=0),
        w("a2", "agg_card", 6, 0, 3, 2, [L2["ds"]["pieces"]], title="Line 2 – produced in 8 h", agg="sum", range="8h", icon="package", decimals=0),
        w("s", "agg_card", 9, 0, 3, 2, [L1["ds"]["scrap"]], title="Line 1 scrap (8 h)", agg="sum", range="8h", icon="alert-triangle", color="#dc2626", decimals=0),
        w("p", "progress", 0, 2, 6, 2, [L1["ds"]["pieces_shift"], L2["ds"]["pieces_shift"]], title="Shift plan progress (plan 4,800 pcs)", y_min=0, y_max=L1["plan"], decimals=0, series=[{"label": "Line 1"}, {"label": "Line 2"}]),
        w("st", "status", 6, 2, 6, 2, [L1["ds"]["state"], L2["ds"]["state"]], title="Line status", states=states, series=[{"label": "Line 1"}, {"label": "Line 2"}]),
        w("b", "bar_time", 0, 4, 8, 3, [L1["ds"]["pieces"], L2["ds"]["pieces"]], title="Pieces per hour", agg="sum", agg_interval="1h", range="8h", decimals=0, series=[{"label": "Line 1"}, {"label": "Line 2"}]),
        w("tl", "state_timeline", 8, 4, 4, 3, [L1["ds"]["state"], L2["ds"]["state"]], title="Machine run time", states=states, range="8h", series=[{"label": "Line 1"}, {"label": "Line 2"}]),
        w("c", "chart", 0, 7, 8, 3, [L1["ds"]["speed"], L2["ds"]["speed"]], title="Speed (pcs/min)", range="6h", series=[{"label": "Line 1"}, {"label": "Line 2"}]),
        w("g", "radial_gauge", 8, 7, 4, 3, [L1["ds"]["temperature"]], title="Press temperature", min=20, max=100, warn=70, action=80),
    ]
    put = lambda slug, body: call("PUT", f"/api/v1/dashboards/{slug}", body) or {"slug": slug}
    mk = (lambda key: (lambda body: put(st[key], body))) if st.get("dash") else (lambda key: (lambda body: call("POST", "/api/v1/dashboards", body)))
    d = mk("dash")({"name": "Production – line overview", "reason": "production demo",
        "spec": {"version": 1, "kind": "dashboard", "layout": {"type": "grid", "row_height": 90}, "range": "8h", "refresh_s": 10, "widgets": widgets}})
    ds = L1["ds"]
    mimic = [
        w("txt", "text", 40, 30, 900, 80, None, title="Line 1 – press shop · hydraulic circuit", text="Process screen per ISA-101: the process is grey, colour only on a deviation or a fault."),
        w("pcs", "value", 1180, 30, 380, 150, [ds["pieces_shift"]], title="Produced this shift", frame=True, decimals=0),
        w("pt", "instrument", 560, 170, 170, 170, [ds["pressure"]], tag="PT-101", warn=7.5),
        w("t", "tank", 40, 300, 260, 420, [ds["level"]], title="Tank T-101", min=0, max=100, decimals=0),
        w("l1", "pipe", 300, 520, 200, 50, [ds["speed"]], thickness=40),
        w("pu", "pump", 500, 400, 260, 300, [ds["state"]], title="Pump P-101", on_value=1, fault_value=2, on_label="RUNNING", off_label="STOPPED"),
        w("l2", "pipe", 760, 520, 180, 50, [ds["speed"]], thickness=40),
        w("v", "valve", 940, 400, 260, 300, [ds["valve"]], title="Valve V-101"),
        w("l3", "pipe", 1200, 520, 160, 50, [ds["speed"]], thickness=40),
        w("tt", "instrument", 1370, 440, 200, 200, [ds["temperature"]], tag="TT-102", warn=70),
        w("sp", "chart", 40, 740, 1520, 150, [ds["speed"]], title="Line speed (pcs/min)", range="6h"),
    ]
    m = mk("mimic")({"name": "Line 1 – process screen", "reason": "production demo",
        "spec": {"kind": "mimic", "layout": {"type": "canvas", "width": 1600, "height": 900}, "refresh_s": 10, "widgets": mimic}})
    print(d["slug"], m["slug"])
    st["dash"], st["mimic"] = d["slug"], m["slug"]
    json.dump(st, open(STATE, "w"))

def live():
    st = json.load(open(STATE))
    rngs = [random.Random(100 + i) for i in range(len(LINES))]
    while True:
        t = time.time()
        for i, line in enumerate(LINES):
            v = model(line, t, rngs[i])
            # a sample every 10 s carries a sixth of the minute's pieces
            v["pieces"] = round(v["pieces"] / 6 + rngs[i].random() * 0.5)
            st["lines"][i]["cum"] += v["pieces"]; v["pieces_shift"] = st["lines"][i]["cum"]
            try: call("POST", "/api/v1/d/telemetry", {"ts": int(t), "v": v}, token=st["lines"][i]["token"])
            except SystemExit as e: print(e, file=sys.stderr)
        json.dump(st, open(STATE, "w"))
        time.sleep(10)

if __name__ == "__main__":
    {"setup": setup, "live": live, "dash": lambda: (login(), dashboards(json.load(open(STATE))))}[sys.argv[1]]()
