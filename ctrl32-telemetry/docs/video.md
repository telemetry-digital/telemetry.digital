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
- Measured (0.45.0, 4-core VM, `tools/camsim -loop`): **64 H.264 cameras at 25 fps, 297 Mbit/s, all recording —
  64/64 online, 1 596 frames/s, 37 MB/s to disk, the server at ~60 % of one core and 91 MB of memory**; with 40 live
  viewers on top (a two-screen wall) ~80 % of one core, every viewer at the full 25 fps. The server never decodes
  video, so CPU scales with packets, not with resolution.

## Watching

- **Live view** — all cameras as tiles (sub-stream), click for the camera page.
- **Camera page** — live picture (main or sub-stream), save picture, full screen; **Recordings**: the day's timeline,
  the detail of one hour, click to play from a time (speed 0.5–16×), drag in the hour detail to select up to one hour
  and download it as MP4 (plays in VLC and browsers).
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
