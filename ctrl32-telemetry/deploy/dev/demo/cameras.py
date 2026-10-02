#!/usr/bin/env python3
"""Demo cameras: 40 simulated cameras (Gate and Gatehouse with ONVIF events and PTZ) and two video walls.
Needs the camera simulator: go run ./tools/camsim -rtsp :8554 -http :8555 -fps 8 -onvif GATE,GATEHOUSE -motion-every 45s"""
import json, unicodedata
from common import call, login, state_path

login()
NAMES = ["Gate", "Gatehouse", "Car park north", "Car park south", "Warehouse A", "Warehouse B", "Dock 1", "Dock 2", "Hall 1", "Hall 2",
         "Hall 3", "Boiler room", "Compressors", "Switchgear room", "Corridor 1", "Corridor 2", "Reception", "Canteen", "Server room", "Roof",
         "Fence east", "Fence west", "Line 1", "Line 2", "Line 3", "Line 4", "Dispatch", "Goods in", "Quality control", "Laboratory",
         "Workshop", "Chemical store", "Tanks", "Pumps", "Weighbridge", "Truck entrance", "Exit", "Yard", "Offices", "Meeting room"]
def label(n):
    return unicodedata.normalize("NFKD", n).encode("ascii", "ignore").decode().upper().replace(" ", "-")
ids = []
for i, n in enumerate(NAMES):
    lb = label(n)
    body = {"name": n, "location": "Nitra site", "url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/640x352", "sub_url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/256x144",
            "record": "off", "sort": i, "reason": "demo"}
    if i < 4: body["record"] = "continuous"
    if n in ("Gate", "Gatehouse"):
        body.update({"onvif_url": f"http://127.0.0.1:8555/onvif/{lb}/device_service", "username": "admin", "password": "admin", "group": "Gatehouse and gate"})
        if n == "Gatehouse": body.update({"record": "events", "event_pre_s": 5, "event_post_s": 10})
    ids.append(call("POST", "/api/v1/video/cameras", body)["id"])
walls = [
    call("POST", "/api/v1/video/walls", {"name": "Control room – 2 monitors", "reason": "demo", "screens": [
        {"name": "Left monitor", "cols": 5, "rows": 4, "tiles": ids[:20]}, {"name": "Right monitor", "cols": 5, "rows": 4, "tiles": ids[20:40]}]})["id"],
    call("POST", "/api/v1/video/walls", {"name": "Gatehouse", "reason": "demo", "screens": [{"name": "TV", "cols": 2, "rows": 2, "tiles": ids[:4]}]})["id"],
]
json.dump({"cams": dict(zip(NAMES, ids)), "walls": walls}, open(state_path("cameras.json"), "w"), ensure_ascii=False)
print(len(ids), "cameras,", len(walls), "walls")
