# Cameras, recording and video walls

ctrl32 telemetry records IP cameras and shows them live, as recordings and on video walls — without ffmpeg or any
other program: the server receives the camera's stream, stores it and forwards it to browsers as it is (no decoding,
no re-encoding). Specification: `specs/010-cameras-nvr`.

## Adding a camera

*Cameras → Camera management → Add camera.*

- **Main stream URL** — `rtsp://…` (H.264, H.265 or MJPEG) or `http://…` (MJPEG, `multipart/x-mixed-replace`).
  Examples: Hikvision `rtsp://IP:554/Streaming/Channels/101`, Dahua `rtsp://IP:554/cam/realmonitor?channel=1&subtype=0`,
  Axis `rtsp://IP/axis-media/media.amp`, many ONVIF cameras `rtsp://IP:554/stream1`.
- **Sub-stream URL** — the camera's smaller stream (Hikvision `…/102`, Dahua `subtype=1`). Tiles and walls use it;
  without it tiles play the main stream, which costs bandwidth and browser power with many tiles.
- User name and password may be pasted in the URL (`rtsp://admin:pw@…`); they are stored separately, the password
  encrypted with `security.secret_key`, and never shown or written to the audit trail.
- **Test connection** connects once and shows codec and picture size, or the error.
- **Transport** — automatic (UDP, falling back to TCP), or force TCP (through firewalls, NAT) or UDP.
- **Recording** — continuous or off; **Keep recordings (days)** per camera.

A camera that does not record connects only while someone watches it (and 30 s after), so an unwatched camera costs
nothing.

## Recording and storage

Recordings are fragmented MP4 files of about one minute (each starts with a key frame) under
`video.storage_dir/<camera id>/<date>/`, indexed in the database. The configuration:

```toml
[video]
storage_dir = "/var/lib/ctrl32-telemetry/video"   # default: next to the reports directory
max_disk_gb = 50                                  # the oldest recordings are deleted beyond this
```

Retention runs every minute: first each camera's days, then the disk limit for all cameras together. Size guide: a
2 Mbit/s camera needs about 21 GB a day. Removing a camera keeps its recordings until its retention ends.

## Camera settings for quality at low load

The server records exactly what the camera sends, so quality and size are set in the camera:

| Stream | Recommended | Why |
|---|---|---|
| Main (recording, full view) | H.265 (or H.264), full resolution, 15–20 fps, VBR 2–4 Mbit/s, **key frame every 1–2 s** (I-frame interval = 1–2 × fps) | H.265 needs about half the space of H.264; short key-frame intervals make files start and seeking land exactly |
| Sub (tiles, walls) | **H.264**, 640 × 360, 10–15 fps, 256–512 kbit/s | the browser decodes every tile; H.264 plays everywhere, small pictures keep a PC with 40 tiles light |

"Smart" codecs (H.265+, H.264+, WiseStream) with key frames every 10+ s work but make recording files and seeking
coarser; turn them off if exact seeking matters more than space.

## Sizing

Storage per day ≈ cameras × Mbit/s × 10.8 GB. Examples for 64 cameras:

| 64 cameras | Network | Disk per day | 14 days |
|---|---|---|---|
| H.265, 2 Mbit/s | 128 Mbit/s (16 MB/s) | ≈ 1.4 TB | ≈ 19 TB |
| H.264, 4 Mbit/s | 256 Mbit/s (32 MB/s) | ≈ 2.8 TB | ≈ 39 TB |

- Put `video.storage_dir` on its own disk or RAID of surveillance hard drives (WD Purple, Seagate SkyHawk: rated for
  continuous writing); keep PostgreSQL on an SSD. SSDs wear out quickly under continuous recording.
- Files are written in fragments of about one second per camera (large sequential writes), the index is updated
  every 10 s per camera, so the database load stays negligible.
- Measured (0.46.0, same VM): **128 cameras, 593 Mbit/s, all recording — 128/128 online, 3 193 frames/s, 72 MB/s to
  disk, ~70 % of one core, 153 MB; with 40 live viewers on top ~110 % of one core (≈ 28 % of the machine), every
  viewer at the full 25 fps.** For 128 cameras plan a 1 Gbit network (a 10 Gbit uplink when the cameras run
  4 Mbit/s or more) and a RAID of surveillance drives that sustains the total write rate.
- Measured (0.45.0, 4-core VM, `tools/camsim -loop`): **64 H.264 cameras at 25 fps, 297 Mbit/s, all recording —
  64/64 online, 1 596 frames/s, 37 MB/s to disk, the server at ~60 % of one core and 91 MB of memory**; with 40 live
  viewers on top (a two-screen wall) ~80 % of one core, every viewer at the full 25 fps. The server never decodes
  video, so CPU scales with packets, not with resolution.

## Remote cameras (relay)

Cameras at another site (a branch, a pumping station, a building without a VPN) do not need to be reachable from the
server. A **relay** at the site reads the local cameras and sends the video to the server over an encrypted
connection it opens itself:

```
camera (192.168.1.64) ──RTSP──▶ camera-relay at the site ══RTSPS (TLS)══▶ server :8322 ──▶ recording, live, walls
```

1. Server: set `[video] push_listen = ":8322"` (and TLS: `push_tls_cert`/`push_tls_key`, or the `http.tls_cert`
   pair is used; profile "internet" refuses to start without TLS) and open TCP 8322 in the firewall.
2. *Camera management → Add camera → Connection: The camera sends its video to the server*. After saving, the
   camera's **secret and a ready `relay.toml` are shown once** (only an encrypted copy stays on the server).
3. At the site, on any computer that reaches the camera (Linux, Windows, Raspberry Pi):
   ```toml
   # relay.toml — chmod 600
   server = "rtsps://telemetry.example.com:8322"
   # ca_file = "/etc/ctrl32/ca.pem"   # only for a private CA
   [[camera]]
   id = "01a0dfcb-…"                 # from the server
   secret = "…"                      # shown once
   source = "rtsp://admin:pw@192.168.1.64:554/Streaming/Channels/101"
   sub_source = "rtsp://admin:pw@192.168.1.64:554/Streaming/Channels/102"   # optional, for tiles
   ```
   `ctrl32-telemetry camera-relay -config relay.toml` (as a systemd service or Windows service for permanent use).
   One relay forwards any number of cameras; each stream reconnects on its own (1 s … 30 s back-off).

Security: the relay only sends (the push listener refuses reading); each camera has its own 192-bit secret; with
TLS the relay verifies the server certificate (system CAs or `ca_file`); without TLS (intranet only) the secret is
checked with digest authentication and never crosses the network in clear text; wrong secrets are logged as security
events and an address with 10 failures in 10 minutes is blocked for 15 minutes; *New secret* disconnects the old
relay at once; the relay never logs the secret or the camera password. The video is forwarded unchanged (no
decoding), so a Raspberry Pi relays many cameras.

Alternatively connect the site with the server's WireGuard network (*System → WireGuard*) and add the cameras
normally — useful when the site already has a router with WireGuard.

## Camera widget

Dashboards and process pictures can show a live camera anywhere: *Add widget → Other → Camera*, choose the camera,
the stream (sub for small widgets), whether to show the name and whether a click opens the camera page. The widget
keeps its connection across dashboard refreshes and closes it when it leaves the page. It needs `video.view`;
public (shared) dashboards never show cameras.

## Watching

- **Live view** — all cameras as tiles (sub-stream), click for the camera page.
- **Camera page** — one player for live and recorded video with a control bar: previous recording, 60 s / 10 s back
  and forward, pause/play, **LIVE**, speed 0.5–16×, digital zoom (buttons, mouse wheel or pinch at the pointer, drag
  to pan, double click for the whole picture), save picture, full screen. Keyboard: space pause, ← → 10 s, Shift+← →
  1 min, Home previous recording, + − 0 zoom, L live, F full screen. Below: the day's timeline and the detail of one
  hour — click to play from a time, drag in the hour detail to select up to one hour and download it as MP4.
- **Playback never stops recording.** Recording runs on the server for itself; playing, pausing or downloading only
  reads the files. Pausing the live picture and pressing play continues from the paused moment out of the recording
  (timeshift) and can catch up to the present; LIVE returns to the live picture.
- Browsers play H.264 everywhere; H.265 only where the operating system supports it (Safari, Edge/Chrome with
  hardware decoding) — otherwise use an H.264 sub-stream for tiles. MJPEG plays everywhere.

## Video walls

*Cameras → Video walls → New video wall*: a name and 1–8 screens (monitors), each with a layout from 1 × 1 to 8 × 6
(for example 5 × 4 = 20 cameras) and a camera per tile; *Fill with all cameras* places them in order.

*Open on all monitors* opens one window per screen; in Chrome and Edge the browser asks once for permission to place
windows and puts each on its own monitor (Window Management API); a click in each window switches it to full screen.
Other browsers open the windows and you drag each to its monitor and press F11. A screen can also be opened directly:
`/video/walls/<id>?screen=1` (e.g. a PC per monitor, kiosk mode). On the wall a click enlarges a camera to the whole
monitor (main stream), Esc or another click returns; the toolbar hides after 3 s without mouse movement.

## Phone and tablet (PWA)

The web app installs on a phone or tablet (*Add to home screen*; shortcuts Cameras and Video walls) and shows live
video, recordings with the same controls (pinch to zoom) and walls. Live video uses Media Source Extensions, on
iPhone ManagedMediaSource (iOS 17.1+); MJPEG cameras work everywhere. Nothing of the video or the API is stored on
the device (the service worker caches only the app shell). Access from outside goes through the same HTTPS address
as the web interface, with the same sign-in, MFA and permissions.

## Permissions and audit

| Permission | Allows | Built-in roles |
|---|---|---|
| `video.view` | live view, video walls | operator, engineer, org_admin |
| `video.playback` | recordings, playback, export | engineer, org_admin |
| `video.manage` | cameras, walls, storage | engineer, org_admin |

Camera and wall changes need a reason and are audited (`camera.*`, `video_wall.*`). Every playback (`video.playback`)
and export (`video.export`) is written to the audit trail with the camera and the time range — recordings of people
are personal data (GDPR): set the retention to what the purpose needs and mark the recorded area.

## Network and security notes

- The server connects to the camera addresses the administrator enters (like connectors); keep cameras in a separate
  network reachable only from the server.
- Browsers receive video over a WebSocket on the same address as the UI (`/api/v1/video/...`); a reverse proxy must
  pass WebSocket upgrades (Caddy does by default).

## Simulated cameras

`go run ./tools/camsim -rtsp :8554 -http :8555` serves test cameras:
`rtsp://127.0.0.1:8554/<label>/<h264|mjpeg>/<W>x<H>` and `http://127.0.0.1:8555/<label>/<W>x<H>` — a picture with the
label, a clock and moving shapes; H.264 uses raw (I_PCM) macroblocks, so it needs a lot of bandwidth. `-loop 50`
encodes 50 frames once and repeats them, for load tests of many cameras (`-fps 25`).
