// ctrl32 telemetry — camera video player (spec 010). One WebSocket per player: H.264/H.265 arrive as fragmented MP4 and
// play through Media Source Extensions; MJPEG arrives as single JPEG pictures drawn on a canvas. Text messages:
// {type:"init", codec, mime, width, height}, {type:"time", t (ms), pos (s in the player's timeline)},
// {type:"status", state, error}, {type:"end"}.
//
//   const p = ctrl32Player(element, {url, live: true, onTime(ms), onState(state, error), onEnd()});
//   p.close();
(function () {
  function player(host, opts) {
    const box = document.createElement('div');
    box.className = 'vp';
    box.innerHTML = '<video muted playsinline autoplay disablepictureinpicture></video><canvas hidden></canvas><div class="vp-msg" hidden></div>';
    host.appendChild(box);
    const video = box.querySelector('video'), canvas = box.querySelector('canvas'), msg = box.querySelector('.vp-msg');
    let ws = null, ms = null, sb = null, queue = [], closed = false, retry = null, mode = '', lastT = null, drawing = false, pendingJpeg = null, stallTimer = null;
    const T = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
    function say(text) { msg.textContent = text ? T(text) : ''; msg.hidden = !text; }
    function state(s, e) { box.dataset.state = s; if (opts.onState) opts.onState(s, e || ''); }

    function resetMedia() {
      if (ms) { try { if (ms.readyState === 'open') ms.endOfStream(); } catch (e) { /* closing */ } }
      if (video.src) { URL.revokeObjectURL(video.src); video.removeAttribute('src'); video.load(); }
      ms = null; sb = null; queue = [];
    }
    function pump() {
      if (!sb || sb.updating || !queue.length) return;
      try { sb.appendBuffer(queue.shift()); } catch (e) {
        if (e.name === 'QuotaExceededError') trim(true); else { say('Video cannot be played'); }
      }
    }
    // keep the buffer short: live a few seconds behind the edge, playback half a minute
    function trim(force) {
      if (!sb || sb.updating || !video.buffered.length) return;
      const start = video.buffered.start(0), keep = opts.live ? 6 : 30;
      if (force || video.currentTime - start > keep * 2) { try { sb.remove(start, Math.max(start, video.currentTime - keep)); } catch (e) { /* busy */ } }
    }
    function chase() {
      if (!opts.live || !video.buffered.length) return;
      const end = video.buffered.end(video.buffered.length - 1);
      if (end - video.currentTime > 1.5) video.currentTime = end - 0.2;
      if (video.paused) video.play().catch(() => {});
    }
    function initMSE(m) {
      mode = 'mse';
      resetMedia();
      canvas.hidden = true; video.hidden = false;
      if (!window.MediaSource || !MediaSource.isTypeSupported(m.mime)) {
        say(m.codec === 'h265' ? 'This browser cannot play H.265 video; use a sub-stream in H.264 or another browser' : 'This browser cannot play this video');
        state('unsupported');
        return;
      }
      ms = new MediaSource();
      video.src = URL.createObjectURL(ms);
      ms.addEventListener('sourceopen', () => {
        try { sb = ms.addSourceBuffer(m.mime); } catch (e) { say('This browser cannot play this video'); return; }
        sb.mode = 'segments';
        sb.addEventListener('updateend', () => { trim(false); chase(); pump(); });
        pump();
      }, { once: true });
      video.play().catch(() => {});
    }
    function initJPEG() {
      mode = 'jpeg';
      resetMedia();
      video.hidden = true; canvas.hidden = false;
    }
    async function drawJPEG(data) {
      if (drawing) { pendingJpeg = data; return; } // draw the newest picture, skip ones that arrive while drawing
      drawing = true;
      try {
        const bmp = await createImageBitmap(new Blob([data], { type: 'image/jpeg' }));
        if (canvas.width !== bmp.width || canvas.height !== bmp.height) { canvas.width = bmp.width; canvas.height = bmp.height; }
        canvas.getContext('2d').drawImage(bmp, 0, 0);
        bmp.close();
      } catch (e) { /* a broken picture */ }
      drawing = false;
      if (pendingJpeg) { const d = pendingJpeg; pendingJpeg = null; drawJPEG(d); }
    }
    function connect() {
      if (closed) return;
      const u = new URL(opts.url, location.href);
      u.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
      ws = new WebSocket(u);
      ws.binaryType = 'arraybuffer';
      state('connecting');
      ws.onmessage = (e) => {
        if (typeof e.data === 'string') {
          const m = JSON.parse(e.data);
          if (m.type === 'init') { say(''); state('online'); if (m.codec === 'mjpeg') initJPEG(); else initMSE(m); box.dataset.codec = m.codec; box.dataset.size = m.width + 'x' + m.height; }
          else if (m.type === 'time') { lastT = { t: m.t, pos: m.pos, at: performance.now() }; if (mode === 'jpeg' && opts.onTime) opts.onTime(m.t); }
          else if (m.type === 'status') { if (m.state !== 'online') { say(m.state === 'offline' ? 'Camera offline' : 'Connecting…'); } state(m.state, m.error); }
          else if (m.type === 'end') { state('end'); if (opts.onEnd) opts.onEnd(); }
          return;
        }
        if (mode === 'jpeg') drawJPEG(e.data);
        else if (mode === 'mse') { queue.push(e.data); if (queue.length > 200) queue.splice(0, queue.length - 200); pump(); }
      };
      ws.onclose = () => {
        ws = null;
        if (closed || box.dataset.state === 'end') return;
        state('reconnecting');
        retry = setTimeout(connect, opts.live ? 1500 : 3000);
      };
    }
    // the recording time of the picture on screen
    function now() {
      if (!lastT) return null;
      if (mode === 'mse') return lastT.t + (video.currentTime - lastT.pos) * 1000;
      return lastT.t;
    }
    video.addEventListener('timeupdate', () => { const t = now(); if (t && opts.onTime) opts.onTime(t); });
    // a stalled live picture (e.g. the tab was in the background) jumps to the live edge
    stallTimer = setInterval(() => { if (!document.hidden) chase(); }, 2000);
    say('Connecting…');
    connect();
    return {
      el: box, video, canvas,
      now,
      snapshot() {
        const c = document.createElement('canvas');
        const src = mode === 'jpeg' ? canvas : video;
        c.width = mode === 'jpeg' ? canvas.width : video.videoWidth; c.height = mode === 'jpeg' ? canvas.height : video.videoHeight;
        if (!c.width) return null;
        c.getContext('2d').drawImage(src, 0, 0);
        return c;
      },
      close() {
        closed = true; clearTimeout(retry); clearInterval(stallTimer);
        if (ws) { ws.onclose = null; ws.close(); }
        resetMedia(); box.remove();
      },
    };
  }
  window.ctrl32Player = player;
})();
