#!/usr/bin/env python3
"""Demo: a floor plan of the site with ten cameras on it (spec 017). Run cameras.py first."""
import json
from common import call, login, state_path

login()
cams = json.load(open(state_path("cameras.json")))["cams"]  # made by cameras.py
W, H = 1600, 900
rooms = [(60, 60, 520, 360, "Hall 1"), (580, 60, 480, 360, "Hall 2"), (1060, 60, 480, 360, "Warehouse A"),
         (60, 420, 360, 300, "Reception"), (420, 420, 380, 300, "Offices"), (800, 420, 740, 300, "Dispatch")]
svg = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">',
       f'<rect width="{W}" height="{H}" fill="#f4f5f7"/>',
       '<rect x="30" y="30" width="1540" height="720" fill="#fff" stroke="#9aa3ad" stroke-width="10"/>']
for x, y, w, h, n in rooms:
    svg.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="none" stroke="#b8c0c8" stroke-width="5"/>')
    svg.append(f'<text x="{x+18}" y="{y+40}" font-family="sans-serif" font-size="26" fill="#8a939c">{n}</text>')
for x in (200, 360, 700, 900, 1200, 1380):
    svg.append(f'<rect x="{x}" y="160" width="90" height="180" fill="#e3e7eb" stroke="#c4cbd2"/>')
svg.append('<rect x="700" y="750" width="200" height="16" fill="#f4f5f7"/><text x="720" y="820" font-family="sans-serif" font-size="26" fill="#8a939c">Entrance / Gate</text>')
svg.append('<rect x="60" y="770" width="400" height="110" fill="#e9ecef" stroke="#c4cbd2"/><text x="80" y="835" font-family="sans-serif" font-size="24" fill="#8a939c">Car park</text></svg>')
try: call("POST", "/api/v1/hmi/objects/import-svg", {"name": "Site floor plan", "slug": "site-floor-plan", "category": "Floor plans", "svg": "".join(svg), "reason": "demo"})
except SystemExit: pass
pins = [("Hall 1", 540, 90, 135), ("Hall 2", 620, 90, 45), ("Warehouse A", 1500, 100, 135), ("Reception", 80, 450, 45),
        ("Offices", 770, 450, 135), ("Dispatch", 1500, 700, 225), ("Gate", 800, 720, 180), ("Gatehouse", 680, 700, 300),
        ("Car park north", 470, 790, 250), ("Dock 1", 1100, 700, 270)]
widgets = [{"id": "plan", "type": "image", "geometry": {"x": 0, "y": 0, "w": W, "h": H}, "bindings": {}, "params": {"src": "site-floor-plan", "fit": "contain", "frame": False}}]
for i, (n, x, y, d) in enumerate(pins):
    widgets.append({"id": f"c{i}", "type": "camera_pin", "geometry": {"x": x - 60, "y": y - 60, "w": 120, "h": 120}, "bindings": {},
                    "params": {"camera": cams[n], "direction": d, "fov": 70, "reach": 90, "show_name": True}})
r = call("POST", "/api/v1/dashboards", {"name": "Site – cameras on the floor plan", "reason": "demo",
         "spec": {"version": 1, "kind": "mimic", "layout": {"type": "canvas", "width": W, "height": H}, "refresh_s": 10, "widgets": widgets}})
print(r["slug"])
