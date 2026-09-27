# ctrl32 camera relay

Forwards the cameras on the Home Assistant network to a ctrl32 telemetry server. The relay opens the connection
itself (RTSPS, TLS), so the site needs no open port. The video is forwarded unchanged.

## Setup

1. On the server: *Cameras → Camera management → Add camera*, **Connection: The camera sends its video to the
   server**. After saving, the camera's **id** and **secret** are shown once.
2. Here, *Configuration*: add one entry per camera:

   ```yaml
   server: rtsps://telemetry.digital:8322
   cameras:
     - name: Garage
       id: 01a0dfcb-...            # from the server
       secret: ...                 # shown once on the server
       source: rtsp://user:password@192.168.0.21:554/stream1
       sub_source: rtsp://user:password@192.168.0.21:554/stream2
       audio: true                  # optional: forward the sound (AAC, Opus, G.711 as Tapo sends it)
       onvif: http://192.168.0.21:2020   # optional: PTZ, presets and motion events through the relay
   ```

   TP-Link Tapo: the user and password are the **camera account** (Tapo app → camera → Settings → Advanced
   settings → Camera account), and on newer firmware *Third-Party Compatibility* must be on (Tapo app → Me →
   Tapo Lab).

   **Camera control** (`onvif`): on the server, give the camera its ONVIF address as seen from this network
   (`http://192.168.0.21:2020/onvif/device_service` for Tapo) and the camera account; the relay then carries the
   PTZ and event requests over the server's web port. Only that address is ever reached. `web` (optional) is the
   server's web address when it is not `https://` + the host of `server`.
3. Start the add-on and read the *Log*: every camera is checked first (`ok` / `FAIL`), then the relay runs.

The server needs `[video] push_listen = ":8322"` and TCP 8322 open. Secrets and camera passwords stay in the
add-on's private data and are never logged.
