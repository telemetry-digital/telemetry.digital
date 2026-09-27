#!/usr/bin/env python3
"""Demo cameras: 40 simulated cameras (Brána and Vrátnica with ONVIF events and PTZ) and two video walls.
Needs the camera simulator: go run ./tools/camsim -rtsp :8554 -http :8555 -fps 8 -onvif BRANA,VRATNICA -motion-every 45s"""
import json, unicodedata
from common import call, login, state_path

login()
NAMES = ["Brána", "Vrátnica", "Parkovisko sever", "Parkovisko juh", "Sklad A", "Sklad B", "Rampa 1", "Rampa 2", "Hala 1", "Hala 2",
         "Hala 3", "Kotolňa", "Kompresory", "Rozvodňa", "Chodba 1", "Chodba 2", "Recepcia", "Jedáleň", "Serverovňa", "Strecha",
         "Plot východ", "Plot západ", "Linka 1", "Linka 2", "Linka 3", "Linka 4", "Expedícia", "Príjem", "Kontrola kvality", "Laboratórium",
         "Dielňa", "Sklad chémie", "Nádrže", "Čerpadlá", "Váženie", "Vjazd kamióny", "Výjazd", "Dvor", "Kancelárie", "Zasadačka"]
def label(n):
    return unicodedata.normalize("NFKD", n).encode("ascii", "ignore").decode().upper().replace(" ", "-")
ids = []
for i, n in enumerate(NAMES):
    lb = label(n)
    body = {"name": n, "location": "Areál Nitra", "url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/640x352", "sub_url": f"rtsp://127.0.0.1:8554/{lb}/mjpeg/256x144",
            "record": "off", "sort": i, "reason": "ukážka"}
    if i < 4: body["record"] = "continuous"
    if n in ("Brána", "Vrátnica"):
        body.update({"onvif_url": f"http://127.0.0.1:8555/onvif/{lb}/device_service", "username": "admin", "password": "admin", "group": "Vrátnica a brána"})
        if n == "Vrátnica": body.update({"record": "events", "event_pre_s": 5, "event_post_s": 10})
    ids.append(call("POST", "/api/v1/video/cameras", body)["id"])
walls = [
    call("POST", "/api/v1/video/walls", {"name": "Velín – 2 monitory", "reason": "ukážka", "screens": [
        {"name": "Monitor ľavý", "cols": 5, "rows": 4, "tiles": ids[:20]}, {"name": "Monitor pravý", "cols": 5, "rows": 4, "tiles": ids[20:40]}]})["id"],
    call("POST", "/api/v1/video/walls", {"name": "Vrátnica", "reason": "ukážka", "screens": [{"name": "TV", "cols": 2, "rows": 2, "tiles": ids[:4]}]})["id"],
]
json.dump({"cams": dict(zip(NAMES, ids)), "walls": walls}, open(state_path("cameras.json"), "w"), ensure_ascii=False)
print(len(ids), "cameras,", len(walls), "walls")
