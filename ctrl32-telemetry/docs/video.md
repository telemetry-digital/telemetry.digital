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

## Events, motion and PTZ (ONVIF)

Enter the camera's ONVIF address (`http://camera/onvif/device_service`, the same user name and password as the stream).
**Find cameras in the network** sends a WS-Discovery probe (multicast, and unicast to the addresses you list, IP
addresses only); **Fill in from ONVIF** reads the profiles and fills the main and sub stream addresses.

- **Events without decoding**: the server subscribes to the camera's events (PullPoint, WS-Security digest, the
  camera's clock offset is measured) and stores motion, tamper, line crossing, intrusion and digital inputs with their
  start and end. Motion that starts again within 5 s joins the same event. The server never decodes video, so event
  detection costs nothing per camera — the camera's own analytics does the work; set its sensitivity and zones in the
  camera. Events of a disconnected camera, a changed camera and a restarted server are closed.
- **Recording on events**: record mode *on events* records only around the chosen kinds: the pre-roll (0–60 s, kept
  in memory from the last key frame) and the post-roll (1–600 s) after the event ends. Continuous recording stores
  events too, as marks on the timeline.
- **Marks**: the flag button on the camera page (or a flow) marks the camera with a label; on a camera recording on
  events it also records the chosen number of seconds. Marks are audited (`camera.mark`).
- **PTZ**: hold an arrow or +/− to move, release to stop; presets from the camera; the speed (slow, normal, fast) and whether the controls are shown (the PTZ button in the player; hidden by default on phones) are remembered in the browser. Needs `video.ptz`; moves are
  audited once per 5 minutes per user and camera (`camera.ptz`), preset changes always.
- **Flows**: the trigger *Camera event* (camera, kind, start/end) and the action *Mark on camera* — for example a door
  contact opens → mark "Door opened" and record 30 s.

Active events outline the tile in the camera list and on video walls. `GET /api/v1/video/events` lists events of all
cameras (`video.playback`).

## Motion detection on the server

Cameras without ONVIF events (cheap cameras, MJPEG cameras, older models) get motion events from the server:
*Camera management → camera → Motion detection on the server*. The server never decodes H.264 or H.265, so it looks
at JPEG pictures only:

- a camera with an **MJPEG stream** (main or sub; the sub-stream is tried first): its frames, at most 2.5 a second;
- an **H.264/H.265 camera**: its **snapshot address** (one JPEG picture, e.g. Hikvision
  `http://camera/ISAPI/Streaming/channels/101/picture`, Dahua `http://camera/cgi-bin/snapshot.cgi`, Axis
  `http://camera/axis-cgi/jpg/image.cgi`), polled once a second with the camera's user name and password (basic or
  digest authentication; never put them into the address).

Each picture becomes a 64 × 36 grid of brightness cells compared with a slowly learning background and with the
previous picture. Motion is a share of changed cells above a threshold set by the **sensitivity** (1–100), in three
pictures in a row; it ends 4 s after the last change. An object that stops (a parked car) becomes background; a
change of almost the whole picture (lights, infrared switching, exposure) resets the background instead of raising an
event. **Motion zones** (green rectangles drawn over the picture in the camera form, next to the privacy masks) limit
where motion counts — e.g. the gate, not the road or the trees behind it.

The events have kind `motion` and source `server` and behave like the camera's own: they start recording on events
(with the pre-event time), show on walls, floor plans and displays, and reach flows. The camera list shows the state
(*watching (MJPEG)*, *watching (snapshots)*, or what is missing). Detection costs the server little CPU with a
small MJPEG sub-stream (320 × 180 is enough); the camera's own detection (ONVIF) stays the better choice where it
exists.

## Camera makes (TP-Link Tapo and others)

*Add camera → Camera make* fills in the stream, ONVIF and snapshot addresses from the IP address for TP-Link Tapo,
Hikvision (HiLook), Dahua (Imou, Amcrest), Reolink, Axis, Uniview and EZVIZ; *Test connection* checks them at once.

**TP-Link Tapo** (C100, C110, C200, C210, C220, C225, C310, C320WS, C500, C510W, C520WS, TC60, TC70 …):

1. In the Tapo app: camera → Settings → Advanced settings → **Camera account**: create a user name and password.
   This is the account for RTSP and ONVIF, not the Tapo app account. On newer firmware also turn on
   **Third-Party Compatibility** (Tapo app → Me → Tapo Lab); until then the camera answers ping but keeps ports 554
   and 2020 closed. Check from a PC on the same network that TCP 554 and 2020 are open before adding the camera.
2. Camera make *TP-Link Tapo*, the camera's IP address (give it a fixed address in the router), that user name and
   password. Streams: `rtsp://IP:554/stream1` (main, 1080p/2K/4K) and `rtsp://IP:554/stream2` (sub, 640×360);
   ONVIF `http://IP:2020/onvif/device_service`.
3. Motion events come over ONVIF (turn on motion detection in the Tapo app); pan/tilt models (C200, C210, C500,
   C510W, C520WS) turn and use presets with PTZ; the sound is G.711 (switch *Sound* on if it is lawful there).
4. Battery cameras and doorbells (C400, C420, C425, D230, D235) have no RTSP/ONVIF and cannot be added.
5. Tapo cameras serve few RTSP connections at a time: do not also watch them from another recorder.

Tapo has no HTTP snapshot address, so motion detection on the server is not available for it; use the camera's
motion events (ONVIF).

## Sound

A camera's sound is **off by default**: recording sound (conversations) is regulated more strictly than pictures. Switch
it on per camera (*Camera management → camera → Sound*) only where it is lawful, and inform people (a sign).

- The camera may send **AAC**, **Opus** or **G.711** (A-law/µ-law — TP-Link Tapo, most Hikvision and Dahua defaults).
  AAC and Opus pass unchanged; G.711 is expanded to 16-bit PCM (a table lookup, no encoder), recorded as an
  uncompressed track (`ipcm`, about 1.4 GB a day per camera at 8 kHz) and played in the browser through WebAudio.
  Other codecs (G.726, MP2) show ⚠ with the advice to choose AAC or G.711. The sound is never re-encoded.
- *Sound recording*: **record it with the picture**, or **live only** — heard live but not written to disk (sound
  per camera and day: G.711 about 1.4 GB, AAC 0.2–0.7 GB; the picture 10–40 GB). Live-only sound is not in
  recordings, playback or exports.
- When recorded, it is the second track of the recording files (the main stream), so exports and evidence packages are
  ordinary MP4 files with sound for VLC or any player.
- The camera page shows a speaker button to people with `video.audio`. The sound is requested only after it is
  switched on (and then with the main stream); each listening is audited (`video.listen`). Walls, widgets and
  displays never play sound. At speeds other than 1× there is no sound.
- Exports (MP4 and evidence packages) contain the sound only for people with `video.audio`.
- In the browser the sound is a separate stream played next to the picture on the same time line, so a gap in the
  sound never stops the picture.
- A relay forwards the sound only with `audio = true` in its `relay.toml`.

## Video settings

**Cameras → Settings** holds the server-wide settings (anyone with `video.manage` sees them, a system administrator
changes them; every change is audited with the old and new values and a reason). Every value has bounds checked by
the server; **Default values** fills in the defaults.

| Setting | Default | Bounds | Meaning |
|---|---|---|---|
| Disk space for recordings | `max_disk_gb` from `config.toml` (50) | 1 GB – 1 PB | the oldest recordings of all cameras are deleted beyond it |
| Length of one recorded file | 60 s | 10–600 s | shorter loses less after a power cut, makes more files |
| Join events within | 5 s | 0–120 s | flapping motion becomes one event (0 = never join) |
| Find cameras: wait for answers | 3 s | 1–15 s | WS-Discovery answer time |
| Audit PTZ moves once per | 5 min | 0–60 min | per user and camera; 0 = every move; presets always |
| Keep an unwatched camera connected | 30 s | 0–600 s | cameras that do not record reconnect instantly within it |
| Timeshift end | 20 s | 5–300 s | a following playback ends after this long without new video |
| Video walls: sub-stream above | 4 tiles | 0–64 | for screens set to *automatic* |
| Relay blocking | 10 attempts / 10 min → 15 min | 3–100 / 1–1440 min / 1–10080 min | failed relay attempts from one address |
| Camera offline or not recording | 5 min | 0–1440 min | incident for a recording camera that is offline or writes nothing (0 = none) |
| Display not showing | 5 min | 0–1440 min | incident for a paired display that stopped reporting |
| Free space of the video disk | 5 % | 0–50 % | incident when the disk of the recordings is nearly full |

Incidents open in the incident log (administrators are notified by e-mail and Web Push) and resolve themselves when
the problem ends.

Per camera (**Camera management → Edit**): addresses, transport, ONVIF, recording mode, pre- and post-roll, the event
kinds that record, retention, order. Per video wall screen: layout, cameras and stream (automatic, sub-stream or main
stream). Per viewer (remembered in the browser): tile size, playback speed, PTZ speed.

## Privacy masks (GDPR)

*Camera management → Edit → Privacy masks*: drag over the live picture to cover areas that must not be watched (a
neighbour's window, a cash desk, a public street). The server does not decode video, so the masks travel with every
live and recorded stream and every player covers them — the camera page, video walls, camera widgets, displays and
phones; JPEG snapshots are masked on the server. The recording itself stays complete:

- seeing it unmasked (the editor, `?unmask=1`) needs `video.unmask` (administrators) and is audited (`video.unmask`);
- MP4 export and evidence packages of a masked camera need `video.unmask`;
- to keep the areas out of the recording altogether, set the privacy masks in the camera itself (its web page or
  ONVIF) — then they are missing from the recording and every export too.

## Evidence

Recordings needed for the police, an insurer or a court:

- **Hold as evidence** — on the camera's page drag over the hour detail and choose *Hold as evidence* (title, case
  number, reason). Held recordings are never deleted: not after the camera's retention days and not to free disk space
  (when only held recordings remain on a full disk, the disk incident tells administrators). *Release* (with a reason)
  hands them back to the normal retention. *Cameras → Evidence* lists all held recordings.
- **Fingerprints while recording** — every recorded file gets its SHA-256 computed while it is written (no extra disk
  reads), so an export can show that the file on disk is unchanged since it was recorded.
- **Evidence package (ZIP)** — the selection (at most one hour) as `video.mp4` (remuxed, never re-encoded: the pictures
  are the camera's own bytes), `manifest.json` (camera, range, who exported it and why, the SHA-256 of the video and
  of each recorded file then and now, the events in the range), `manifest.sig` (Ed25519 signature by the server's key),
  `public_key.txt` and `README.txt`. The audit entry carries the video's hash.
- **Verification** — `ctrl32-telemetry verify-evidence [-key <public key>] package.zip` checks the signature and the
  video hash; the key fingerprint must equal the one under *Cameras → Evidence* (hand it over through another
  channel). Any changed byte of the video or of the manifest is detected.

Permission `video.evidence` (engineers, administrators). The server key is created on first use and stored encrypted
with `security.secret_key`; keep that secret in the backup, otherwise old packages can still be verified with the
published public key but new ones get a new key.

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
3. At the site, on any computer that reaches the camera (Linux, Raspberry Pi with a 64-bit OS, Windows): save the
   configuration as `relay.toml` (*Download relay.toml* in the dialog), fill in the local address of the camera and
   run the installer next to it:
   ```sh
   # Linux / Raspberry Pi
   curl -fsSL https://telemetry.digital/install-relay.sh -o install-relay.sh
   sudo bash install-relay.sh --config relay.toml
   ```
   ```powershell
   # Windows, PowerShell as administrator
   irm https://telemetry.digital/install-relay.ps1 -OutFile install-relay.ps1; .\install-relay.ps1 -Config .\relay.toml
   ```
   The installer downloads the program and checks it against `SHA256SUMS`, stores the configuration where only the
   service can read it (Linux `/etc/ctrl32-camera-relay/relay.toml`, root:ctrl32relay 0640; Windows
   `C:\ProgramData\ctrl32-camera-relay`, SYSTEM and Administrators), **tests it** and only then installs the service
   `ctrl32-camera-relay` (systemd unit under its own user in a sandbox; Windows service with automatic start, both
   restart on failure). Delete the copied file afterwards. Re-run with a new `relay.toml` to add cameras,
   `--update`/`-Update` for a new program version, `--check`/`-Check` to test again, `--uninstall`/`-Uninstall`
   to remove it.

   The configuration file:
   ```toml
   # relay.toml — keep it private
   server = "rtsps://telemetry.example.com:8322"
   # ca_file = "/etc/ctrl32/ca.pem"   # only for a private CA
   [[camera]]
   id = "01a0dfcb-…"                 # from the server
   secret = "…"                      # shown once
   source = "rtsp://admin:pw@192.168.1.64:554/Streaming/Channels/101"
   sub_source = "rtsp://admin:pw@192.168.1.64:554/Streaming/Channels/102"   # optional, for tiles
   ```
   The test alone: `ctrl32-telemetry camera-relay -check -config relay.toml` — the server is reachable and its
   certificate trusted, every camera answers with H.264, H.265 or MJPEG, the server accepts each secret; passwords
   and secrets are never printed. Without the installer: `ctrl32-telemetry camera-relay -config relay.toml`, or
   `ctrl32-telemetry service install-relay -config relay.toml` to register the service yourself.
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

## Cameras on a floor plan

A floor plan or a site map is a process picture (*Dashboards → New → Process picture*) with the plan as an image
from the library (*Library → Import SVG*, e.g. exported from a CAD drawing) and one *Camera on a plan* widget per
camera (*Add widget → Process → Camera on a plan*). Each pin shows the camera, its view cone (direction 0° = up,
clockwise; field of view; reach) and its name. The pin is green when the camera is online, red when it is offline and
orange, pulsing, while an event (motion or another ONVIF event) is in progress. A click opens the live picture in a
window (privacy masks apply) with a link to the camera page. Pins of cameras outside the viewer's camera groups are
not drawn; a user limited to groups cannot place other cameras. On a display (kiosk) the plan works the same, the
window closes by itself after a minute; shared public dashboards show the pins without state.

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

## Displays at a wall

For walls that run day and night without anyone signed in, pair the wall computer as a display
(`docs/displays.md`): it shows wall screens and dashboards in turn, enlarges cameras with events and follows
operators' commands.

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
| `video.manage` | cameras, walls, storage, discovery | engineer, org_admin |
| `video.ptz` | PTZ control and presets | operator, engineer, org_admin |
| `video.evidence` | hold recordings as evidence, signed evidence packages | engineer, org_admin |
| `video.unmask` | see and export pictures without privacy masks | org_admin |
| `video.audio` | hear camera sound live and recorded, export it | org_admin |
| `display.control` | send cameras and commands to displays | operator, engineer, org_admin |
| `display.manage` | pair, change and disconnect displays | engineer, org_admin |

Users can be limited to camera groups (see `docs/displays.md`).

Camera and wall changes need a reason and are audited (`camera.*`, `video_wall.*`). Every playback (`video.playback`)
and export (`video.export`) is written to the audit trail with the camera and the time range — recordings of people
are personal data (GDPR): set the retention to what the purpose needs and mark the recorded area.

## Network and security notes

- RTP over UDP: the server asks for a 4 MB receive buffer when Linux allows it (`net.core.rmem_max`); with the
  common default of 208 KB it keeps the system default, which can drop packets of large key frames. For many or
  high-resolution cameras over UDP set `sysctl -w net.core.rmem_max=4194304` (and in `/etc/sysctl.d/`), or use
  transport TCP.

- The server connects to the camera addresses the administrator enters (like connectors); keep cameras in a separate
  network reachable only from the server.
- Browsers receive video over a WebSocket on the same address as the UI (`/api/v1/video/...`); a reverse proxy must
  pass WebSocket upgrades (Caddy does by default).

## Simulated cameras

`go run ./tools/camsim -rtsp :8554 -http :8555` serves test cameras:
`rtsp://127.0.0.1:8554/<label>/<h264|mjpeg>/<W>x<H>` and `http://127.0.0.1:8555/<label>/<W>x<H>` — a picture with the
label, a clock and moving shapes; H.264 uses raw (I_PCM) macroblocks, so it needs a lot of bandwidth. `-loop 50`
encodes 50 frames once and repeats them, for load tests of many cameras (`-fps 25`). `-onvif BRANA,VRATNICA` adds
ONVIF devices at `http://127.0.0.1:8555/onvif/<label>/device_service` (user `admin`, password `admin`) with PTZ that
moves the picture, `-motion-every 45s` reports motion periodically, `/sim/motion/<label>?on=1|0` switches it by hand,
and `-discovery 127.0.0.1:3702` answers WS-Discovery. A path ending in `/opus` adds silent Opus sound, `/g711`
announces G.711 (for the unsupported-codec message). Cameras labelled `STILL…` show a standing scene until
motion is switched on (for motion detection on the server), and `http://127.0.0.1:8555/snapshot/<label>/<W>x<H>`
returns one JPEG picture like the snapshot address of an IP camera.
