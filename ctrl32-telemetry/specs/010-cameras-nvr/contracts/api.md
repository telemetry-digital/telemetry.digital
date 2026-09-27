# API: cameras and video walls

See `/api/docs` (tag "Cameras") for the full schema.

| Method | Path | Permission |
|---|---|---|
| GET/POST | /api/v1/video/cameras | video.view / video.manage |
| POST | /api/v1/video/cameras/test | video.manage |
| GET/PUT/DELETE | /api/v1/video/cameras/{id} | video.view / video.manage |
| GET (WebSocket) | /api/v1/video/cameras/{id}/live?quality=sub\|main | video.view |
| GET | /api/v1/video/cameras/{id}/snapshot | video.view |
| GET | /api/v1/video/cameras/{id}/recordings?from&to | video.playback |
| GET (WebSocket) | /api/v1/video/cameras/{id}/playback?from&speed | video.playback |
| GET | /api/v1/video/cameras/{id}/export?from&to | video.playback |
| GET | /api/v1/video/storage | video.manage |
| GET/POST, GET/PUT/DELETE | /api/v1/video/walls[/{id}] | video.view / video.manage |

WebSocket messages: text `{type:"init", codec, mime, width, height}`, `{type:"time", t, pos}`, `{type:"status", state,
error}`, `{type:"end"}`; binary: MP4 initialization and fragments (H.264/H.265) or JPEG pictures (MJPEG).
