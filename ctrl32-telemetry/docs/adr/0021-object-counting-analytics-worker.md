# ADR-0021: Object counting with our own detector in a worker process; ffmpeg as an unmodified external program

**Status:** accepted 2 Oct 2026 (owner: "the system itself does the counting and writes it into telemetry … any RTSP
camera, no vendor analytics … a person detector via ONNX Runtime with a model under a permissive licence (not AGPL) …
decoding via ffmpeg as a separate process (LGPL, unmodified) … CPU-only must work … off by default, no images
stored"). Amends ADR-0009 (licences of third-party components) with one exception, and extends ADR-0012 (processes).

## Context

Counting people and vehicles crossing a line needs decoded pictures, a detector, a tracker and the line logic. The
server so far never decodes H.264/H.265 (recording and live view pass the camera's stream through; motion detection
works on MJPEG/JPEG only, spec 019). The options:

1. **Vendor analytics** (ONVIF line-crossing rules, Hikvision/Dahua/Axis counters): depends on camera models and
   firmware, direction is a rule setting rather than a field of the event, results differ between vendors; rejected
   by the owner ("a secure system without random cameras' analytics").
2. **Third-party counting software** sending results to us: not our product; whoever has it uses the device protocol.
3. **Our own detector on the server** (chosen).

## Decision

1. **A worker process per camera.** `ctrl32-telemetry analytics-worker` (the same binary) analyses one camera. A crash
   or a hang of native code (decoder, inference runtime) cannot take the server down, the CPU use of a camera is
   measurable and limited (`[analytics] max_cameras`, default 4). The server feeds the worker from its own
   subscription to the camera's sub-stream (one camera connection for recording, viewers and counting; relayed
   cameras work the same way) as framed samples on stdin (wall-clock time + Annex B access unit or JPEG); the worker
   answers JSON lines on stdout (crossings, speeds, statistics). No credentials, no network access of the worker.
2. **Decoding by ffmpeg, unmodified, as a separate program (LGPL).** It reads the samples from a pipe and writes
   416 × 416 BGR pictures to a pipe. **Exception to ADR-0009:** ffmpeg is LGPL-licensed; it is **not linked and not
   compiled into** our binary, it is an independent program the operator installs (an LGPL build, e.g. the "lgpl"
   builds of BtbN or the distribution's package; GPL-enabled builds must not be used); we communicate through pipes
   only. If an installer later downloads it, it downloads an unmodified upstream LGPL build with its licence and
   source offer; we never modify or redistribute a changed ffmpeg. Alternatives without LGPL (OpenH264, BSD-2) would
   need cgo or another external program and cover H.264 only.
3. **Inference with ONNX Runtime (MIT)** loaded from its shared library at run time (`syscall` on Windows, `purego`
   (Apache-2.0) on Linux/macOS; the binary stays free of cgo; 32-bit ARM builds report "not supported"). CPU only for
   now (one thread per camera by default); a GPU execution provider can be added later.
4. **Models: YOLOX nano and tiny (Megvii, Apache-2.0, weights in the same repository release 0.1.1rc0)**, trained on
   COCO 2017 (annotations CC BY 4.0; images under Flickr terms — the usual basis of permissively licensed detectors).
   Not used: Ultralytics YOLOv5/v8/v11 (AGPL-3.0), other models with non-commercial terms. The model files are not
   embedded in the binary; the configuration names them and may pin their SHA-256 (the worker refuses a different
   file). Classes used: person, bicycle, car, motorcycle, bus, truck.
5. **Tracker, crossing and speed logic are ours** (no third-party tracker code): constant-velocity prediction,
   overlap or per-axis distance gates with a size-ratio gate, crossings through the drawn segment once per track and
   line, interpolated crossing moments, speed between two lines over a known distance.
6. **No images leave the worker or are stored.** Only numbers become observations; the line editor gets the boxes of
   the last analysed frame (coordinates, classes) and takes the picture itself from the live view in the browser
   (privacy masks burnt in, `video.view` of the camera).
7. **Off by default per camera**, configured with `config.write`, every change audited with a reason; the CPU, the
   analysed frame rate and the inference time are shown per camera.

## Consequences

- A server counts with any camera whose sub-stream it receives; a few cameras per CPU core at 5 pictures per second
  with the nano model (measured on the development PC: see JOURNAL 0.71.0).
- The operator installs ffmpeg, ONNX Runtime and the model files (documented; the installers do not yet download them).
- Accuracy depends on the camera's view (a view from above at an angle is best; a strict top-down view detects people
  worse with COCO-trained models), light, crowding and the frame rate; speed is indicative only, never enforcement.
- The analytics worker is a new process boundary in the threat model (untrusted video parsed by ffmpeg in a separate
  process without network or credentials).
