"""Shared helpers of the demo scripts: a signed-in session against the development server.

Environment: CTRL32_URL (default http://127.0.0.1:8080), CTRL32_USER / CTRL32_PASS (default admin / admin1234),
CTRL32_DEV_DIR (state files, default ctrl32-telemetry/.dev)."""
import json, os, urllib.request, urllib.error, http.cookiejar

B = os.environ.get("CTRL32_URL", "http://127.0.0.1:8080").rstrip("/")
DEV = os.environ.get("CTRL32_DEV_DIR", os.path.join(os.path.dirname(__file__), "..", "..", "..", ".dev"))
os.makedirs(DEV, exist_ok=True)
_cj = http.cookiejar.CookieJar()
_op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(_cj))
_csrf = ""


def call(m, path, body=None, token=None):
    h = {"Content-Type": "application/json"}
    if token:
        h["Authorization"] = "Bearer " + token
    elif _csrf:
        h["X-CSRF-Token"] = _csrf
    req = urllib.request.Request(B + path, method=m, headers=h, data=json.dumps(body).encode() if body is not None else None)
    try:
        with (urllib.request.build_opener() if token else _op).open(req) as r:
            t = r.read()
            return json.loads(t) if t else {}
    except urllib.error.HTTPError as e:
        raise SystemExit(f"{m} {path}: {e.code} {e.read()[:300]}")


def login():
    global _csrf
    call("POST", "/api/v1/auth/login", {"username": os.environ.get("CTRL32_USER", "admin"), "password": os.environ.get("CTRL32_PASS", "admin1234")})
    _csrf = call("GET", "/api/v1/auth/me")["csrf_token"]


def state_path(name):
    return os.path.join(DEV, name)
