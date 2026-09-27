# Implementation Plan: Cameras — recording and video walls

**Spec**: `spec.md` · **Paths**: relative to `ctrl32-telemetry/`

## Technical Context
- Go: `github.com/bluenviron/gortsplib/v5` (RTSP client and, for the simulator, server; MIT) and
  `github.com/bluenviron/mediacommon/v2` (H.264/H.265 parameter sets, DTS extraction, fragmented MP4; MIT);
  `github.com/gorilla/websocket` (BSD) for the video WebSocket. No cgo, no ffmpeg.
- Storage: files under `video.storage_dir` (default next to the reports directory), index `core.video_segment`.
- Browser: Media Source Extensions for H.264/H.265, canvas + createImageBitmap for MJPEG; CSP gains `media-src blob:`.

## Structure
- `internal/video`: `track.go` (codec, MIME for MSE), `source.go` (connection, reconnect, fan-out, GOP cache, status),
  `rtsp.go`, `mjpeg.go`, `muxer.go` (fMP4 with one-frame look-ahead), `recorder.go`, `playback.go` (reads files,
  continuous timestamps), `manager.go` (cameras, retention, repair), `testsrc` (I_PCM H.264 encoder), `camsim`
  (simulated cameras, also `tools/camsim`).
- `internal/httpapi/video.go`: cameras, stream test, live/playback WebSocket, recordings, export, storage, walls.
- UI: `video.html` (live grid, walls, management), `camera.html` (live, timeline, export), `wall.html`, `videoplayer.js`.
- Migration `0027_video.sql`: camera, video_segment, video_wall, permissions.

## Constitution Check
- I audit: camera/wall changes with reason; playback and export audited. II single binary: pure Go. VII tests: unit
  (RTSP H.264/MJPEG, HTTP MJPEG, muxer, redaction) and e2e.
