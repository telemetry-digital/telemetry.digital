#!/usr/bin/env python3
"""Demo cameras: 40 simulated cameras (Gate and Gatehouse with ONVIF events and PTZ) and two video walls.
Needs the camera simulator: go run ./tools/camsim -rtsp :8554 -http :8555 -fps 8 -onvif GATE,GATEHOUSE -motion-every 45s

Without a licence a server runs up to 4 cameras. dev.sh passes a development licence (signed with the test key of
tests/keys, trusted by the development build) in CTRL32_DEV_LICENSE and it is installed here; without it only the first
4 cameras are added. Idempotent: cameras and walls that exist (by name) are kept, so it can run again after a licence
was installed."""
import json, os, unicodedata
from common import call, login, state_path

FREE_CAMERAS = 4
login()
NAMES = ["Gate", "Gatehouse", "Car park north", "Car park south", "Warehouse A", "Warehouse B", "Dock 1", "Dock 2", "Hall 1", "Hall 2",
         "Hall 3", "Boiler room", "Compressors", "Switchgear room", "Corridor 1", "Corridor 2", "Reception", "Canteen", "Server room", "Roof",
         "Fence east", "Fence west", "Line 1", "Line 2", "Line 3", "Line 4", "Dispatch", "Goods in", "Quality control", "Laboratory",
         "Workshop", "Chemical store", "Tanks", "Pumps", "Weighbridge", "Truck entrance", "Exit", "Yard", "Offices", "Meeting room"]
lic = call("GET", "/api/v1/server/license")
if not lic.get("valid") and os.environ.get("CTRL32_DEV_LICENSE"):
    lic = call("PUT", "/api/v1/server/license", {"token": os.environ["CTRL32_DEV_LICENSE"], "reason": "development demo (test licence key)"})
    print("development licence installed")
licensed = bool(lic.get("valid"))
existing = {c["name"]: c["id"] for c in call("GET", "/api/v1/video/cameras")}
want = NAMES if licensed else NAMES[:FREE_CAMERAS]
if not licensed:
    print(f"no licence: only the first {FREE_CAMERAS} cameras (set CTRL32_DEV_LICENSE or install a licence, then run again)")


def label(n):
    return unicodedata.normalize("NFKD", n).encode("ascii", "ignore").decode().upper().replace(" ", "-")


ids = []
for i, n in enumerate(want):
    if n in existing:
        ids.append(existing[n]); continue
    lb = label(n)
    body = {"name": n, "location": "Nitra site", "url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/640x352", "sub_url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/256x144",
            "record": "off", "sort": i, "reason": "demo"}
    if i < 4: body["record"] = "continuous"
    if n in ("Gate", "Gatehouse"):
        body.update({"onvif_url": f"http://127.0.0.1:8555/onvif/{lb}/device_service", "username": "admin", "password": "admin", "group": "Gatehouse and gate"})
        if n == "Gatehouse": body.update({"record": "events", "event_pre_s": 5, "event_post_s": 10})
    ids.append(call("POST", "/api/v1/video/cameras", body)["id"])
walls_by_name = {w["name"]: w["id"] for w in call("GET", "/api/v1/video/walls")}
walls = []
if len(ids) >= 40:
    n = "Control room – 2 monitors"
    walls.append(walls_by_name.get(n) or call("POST", "/api/v1/video/walls", {"name": n, "reason": "demo", "screens": [
        {"name": "Left monitor", "cols": 5, "rows": 4, "tiles": ids[:20]}, {"name": "Right monitor", "cols": 5, "rows": 4, "tiles": ids[20:40]}]})["id"])
n = "Gatehouse"
walls.append(walls_by_name.get(n) or call("POST", "/api/v1/video/walls", {"name": n, "reason": "demo", "screens": [{"name": "TV", "cols": 2, "rows": 2, "tiles": ids[:4]}]})["id"])
json.dump({"cams": dict(zip(want, ids)), "walls": walls}, open(state_path("cameras.json"), "w"), ensure_ascii=False)
print(len(ids), "cameras,", len(walls), "walls")
