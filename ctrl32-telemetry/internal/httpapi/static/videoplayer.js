// ctrl32 telemetry — camera video player (spec 010). One WebSocket per player: H.264/H.265 arrive as fragmented MP4 and
// play through Media Source Extensions; MJPEG arrives as single JPEG pictures drawn on a canvas. Text messages:
// {type:"init", codec, mime, width, height}, {type:"time", t (ms), pos (s in the player's timeline)},
// {type:"status", state, error}, {type:"end"}, {type:"audio", mime} (spec 020).
// The sound comes only when the URL asks for it (audio=1) and the user may hear it: binary messages starting with 'A'
// carry it as a separate fragmented MP4 stream, played by its own <audio> element that follows the picture's clock, so
// a gap in the sound never stalls the picture. It starts muted; p.sound(true) switches it on (a user gesture).
//
//   const p = ctrl32Player(element, {url, live: true, zoom: true, onTime(ms), onState(state, error), onEnd()});
//   p.open(url, live);   // another source in the same player (keeps the zoom)
//   p.pause(); p.resume(); p.zoomBy(1.25); p.zoomReset(); p.close();
// Digital zoom (opts.zoom): mouse wheel or pinch zooms at the pointer, dragging pans, a double click resets.
(function () {
  function player(host, opts) {
    const box = document.createElement('div');
    box.className = 'vp';
    box.innerHTML = '<div class="vp-stage"><video muted playsinline autoplay disablepictureinpicture></video><canvas hidden></canvas><svg class="vp-mask" hidden aria-hidden="true"></svg></div><div class="vp-msg" hidden></div><div class="vp-zoom" hidden></div>';
    host.appendChild(box);
    const video = box.querySelector('video'), canvas = box.querySelector('canvas'), msg = box.querySelector('.vp-msg'), stage = box.querySelector('.vp-stage'), zlabel = box.querySelector('.vp-zoom');
    let ws = null, ms = null, sb = null, queue = [], closed = false, retry = null, mode = '', lastT = null, drawing = false, pendingJpeg = null, stallTimer = null;
    // the sound (spec 020)
    let audio = null, ams = null, asb = null, aqueue = [], soundOn = false;
    function resetAudio() {
      if (ams) { try { if (ams.readyState === 'open') ams.endOfStream(); } catch (e) { /* closing */ } }
      if (audio) { if (audio.src) URL.revokeObjectURL(audio.src); audio.removeAttribute('src'); audio.load(); }
      ams = null; asb = null; aqueue = []; box.dataset.audio = '';
    }
    function apump() {
      if (!asb || asb.updating || !aqueue.length) return;
      try { asb.appendBuffer(aqueue.shift()); } catch (e) {
        if (e.name === 'QuotaExceededError' && audio.buffered.length) { try { asb.remove(audio.buffered.start(0), Math.max(audio.buffered.start(0), audio.currentTime - 5)); } catch (x) { /* busy */ } }
      }
    }
    function initAudio(m) {
      resetAudio();
      const MSImpl = window.MediaSource || window.ManagedMediaSource;
      if (!MSImpl || !MSImpl.isTypeSupported(m.mime)) { box.dataset.audio = 'unsupported'; if (opts.onAudio) opts.onAudio('unsupported'); return; }
      if (!audio) { audio = document.createElement('audio'); audio.hidden = true; box.appendChild(audio); }
      audio.muted = !soundOn;
      ams = new MSImpl();
      if (!window.MediaSource) audio.disableRemotePlayback = true;
      audio.src = URL.createObjectURL(ams);
      ams.addEventListener('sourceopen', () => {
        try { asb = ams.addSourceBuffer(m.mime); } catch (e) { box.dataset.audio = 'unsupported'; return; }
        asb.mode = 'segments';
        asb.addEventListener('updateend', () => { if (audio.buffered.length && audio.currentTime - audio.buffered.start(0) > 20) { try { asb.remove(audio.buffered.start(0), audio.currentTime - 10); } catch (x) { /* busy */ } } apump(); });
        apump();
      }, { once: true });
      box.dataset.audio = '1';
      if (opts.onAudio) opts.onAudio('on');
    }
    // the sound follows the picture: same time line (the server starts both at the first picture)
    function syncAudio() {
      if (!audio || !asb || !audio.buffered.length) return;
      const end = audio.buffered.end(audio.buffered.length - 1), start = audio.buffered.start(0);
      let target = null;
      if (mode === 'mse' && video.buffered.length) target = video.currentTime;
      else if (opts.live) target = end - 0.3;
      if (target !== null && target >= start && target <= end && Math.abs(audio.currentTime - target) > 0.25) audio.currentTime = target;
      const runs = mode === 'jpeg' ? box.dataset.state === 'online' : !video.paused; // MJPEG is drawn, not played
      if (soundOn && audio.paused && runs) audio.play().catch(() => {});
      if (!soundOn && !audio.paused) audio.pause();
    }
    const T = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
    // privacy masks (spec 016): the server sends them before the first frame; they cover the picture area, which
    // "contain" letterboxes inside the stage, and zoom with it
    const maskEl = box.querySelector('.vp-mask'); let MASKS = [];
    function placeMasks() {
      if (!MASKS.length) { maskEl.hidden = true; return; }
      const W = mode === 'jpeg' ? canvas.width : video.videoWidth, H = mode === 'jpeg' ? canvas.height : video.videoHeight;
      const sw = stage.clientWidth, sh = stage.clientHeight;
      if (!W || !H || !sw || !sh) { maskEl.hidden = false; maskEl.setAttribute('style', 'left:0;top:0;width:100%;height:100%'); } else {
        const k = Math.min(sw / W, sh / H), w = W * k, h = H * k;
        maskEl.setAttribute('style', `left:${(sw - w) / 2}px;top:${(sh - h) / 2}px;width:${w}px;height:${h}px`); maskEl.hidden = false;
      }
      maskEl.setAttribute('viewBox', '0 0 1 1'); maskEl.setAttribute('preserveAspectRatio', 'none');
      maskEl.innerHTML = MASKS.map((p) => `<polygon points="${p.map((q) => (+q[0]).toFixed(4) + ',' + (+q[1]).toFixed(4)).join(' ')}"/>`).join('');
    }
    if (window.ResizeObserver) new ResizeObserver(placeMasks).observe(stage);
    video.addEventListener('loadedmetadata', placeMasks); video.addEventListener('resize', placeMasks);
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
      const MSImpl = window.MediaSource || window.ManagedMediaSource; // iPhone Safari has only ManagedMediaSource
      if (!MSImpl || !MSImpl.isTypeSupported(m.mime)) {
        say(m.codec === 'h265' ? 'This browser cannot play H.265 video; use a sub-stream in H.264 or another browser' : 'This browser cannot play this video');
        state('unsupported');
        return;
      }
      ms = new MSImpl();
      if (!window.MediaSource) video.disableRemotePlayback = true; // required by ManagedMediaSource
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
        if (canvas.width !== bmp.width || canvas.height !== bmp.height) { canvas.width = bmp.width; canvas.height = bmp.height; placeMasks(); }
        canvas.getContext('2d').drawImage(bmp, 0, 0);
        bmp.close();
      } catch (e) { /* a broken picture */ }
      drawing = false;
      if (pendingJpeg) { const d = pendingJpeg; pendingJpeg = null; drawJPEG(d); }
    }
    let gen = 0;
    function connect() {
      if (closed) return;
      const u = new URL(opts.url, location.href);
      u.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const my = ++gen;
      ws = new WebSocket(u);
      ws.binaryType = 'arraybuffer';
      state('connecting');
      ws.onmessage = (e) => {
        if (my !== gen) return;
        if (typeof e.data === 'string') {
          const m = JSON.parse(e.data);
          if (m.type === 'masks') { MASKS = Array.isArray(m.masks) ? m.masks : []; box.dataset.masked = MASKS.length ? '1' : ''; box.dataset.unmasked = m.unmasked ? '1' : ''; placeMasks(); if (opts.onMasks) opts.onMasks(m); }
          else if (m.type === 'init') { say(''); state('online'); resetAudio(); if (m.codec === 'mjpeg') initJPEG(); else initMSE(m); box.dataset.codec = m.codec; box.dataset.size = m.width + 'x' + m.height; }
          else if (m.type === 'audio') initAudio(m);
          else if (m.type === 'time') { lastT = { t: m.t, pos: m.pos, at: performance.now() }; if (mode === 'jpeg' && opts.onTime) opts.onTime(m.t); }
          else if (m.type === 'status') { if (m.state !== 'online') { say(m.state === 'offline' ? 'Camera offline' : 'Connecting…'); } state(m.state, m.error); }
          else if (m.type === 'end') { state('end'); if (opts.onEnd) opts.onEnd(); }
          return;
        }
        if (new Uint8Array(e.data, 0, 1)[0] === 0x41) { if (ams) { aqueue.push(e.data.slice(1)); if (aqueue.length > 100) aqueue.splice(0, aqueue.length - 100); apump(); } return; }
        if (mode === 'jpeg') drawJPEG(e.data);
        else if (mode === 'mse') { queue.push(e.data); if (queue.length > 200) queue.splice(0, queue.length - 200); pump(); }
      };
      ws.onclose = () => {
        if (my !== gen) return;
        ws = null;
        if (closed || box.dataset.state === 'end' || box.dataset.state === 'paused') return;
        state('reconnecting');
        retry = setTimeout(connect, opts.live ? 1500 : 3000);
      };
    }
    // the recording time of the picture on screen
    function stop() { gen++; clearTimeout(retry); if (ws) { ws.onclose = null; ws.close(); ws = null; } }
    // ---- digital zoom
    let z = 1, zx = 0, zy = 0;
    function applyZoom() {
      const w = box.clientWidth, h = box.clientHeight;
      zx = Math.min(0, Math.max(w - w * z, zx)); zy = Math.min(0, Math.max(h - h * z, zy));
      if (z <= 1.001) { z = 1; zx = 0; zy = 0; }
      stage.style.transform = z === 1 ? '' : `translate(${zx}px, ${zy}px) scale(${z})`;
      box.classList.toggle('zoomed', z > 1);
      zlabel.hidden = z === 1; zlabel.textContent = z.toFixed(1) + '×';
      if (opts.onZoom) opts.onZoom(z);
    }
    function zoomAt(f, cx, cy) {
      const nz = Math.min(8, Math.max(1, z * f));
      if (cx === undefined) { cx = box.clientWidth / 2; cy = box.clientHeight / 2; }
      zx = cx - (cx - zx) * nz / z; zy = cy - (cy - zy) * nz / z; z = nz; applyZoom();
    }
    if (opts.zoom) {
      box.addEventListener('wheel', (e) => { e.preventDefault(); const r = box.getBoundingClientRect(); zoomAt(e.deltaY < 0 ? 1.2 : 1 / 1.2, e.clientX - r.left, e.clientY - r.top); }, { passive: false });
      let drag = null; const pts = new Map();
      box.addEventListener('pointerdown', (e) => { pts.set(e.pointerId, e); if (z > 1) { drag = { x: e.clientX, y: e.clientY, zx, zy, moved: false }; try { box.setPointerCapture(e.pointerId); } catch (x) { /* synthetic */ } } });
      box.addEventListener('pointermove', (e) => {
        if (pts.size === 2 && pts.has(e.pointerId)) { // pinch
          const [a, b] = [...pts.values()], d0 = Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
          pts.set(e.pointerId, e); const [c, d] = [...pts.values()], d1 = Math.hypot(c.clientX - d.clientX, c.clientY - d.clientY);
          const r = box.getBoundingClientRect(); if (d0 > 0) zoomAt(d1 / d0, (c.clientX + d.clientX) / 2 - r.left, (c.clientY + d.clientY) / 2 - r.top); return;
        }
        if (!drag) return; drag.moved = true; zx = drag.zx + e.clientX - drag.x; zy = drag.zy + e.clientY - drag.y; applyZoom();
      });
      const up = (e) => { pts.delete(e.pointerId); if (drag && drag.moved) box._dragged = Date.now(); drag = null; };
      box.addEventListener('pointerup', up); box.addEventListener('pointercancel', up);
      box.addEventListener('dblclick', (e) => { e.stopPropagation(); z = 1; applyZoom(); });
      // a click after panning is not a click on the picture
      box.addEventListener('click', (e) => { if (box._dragged && Date.now() - box._dragged < 300) e.stopPropagation(); }, true);
    }
    function now() {
      if (!lastT) return null;
      if (mode === 'mse') return lastT.t + (video.currentTime - lastT.pos) * 1000;
      return lastT.t;
    }
    video.addEventListener('timeupdate', () => { const t = now(); if (t && opts.onTime) opts.onTime(t); });
    // a stalled live picture (e.g. the tab was in the background) jumps to the live edge
    stallTimer = setInterval(() => { if (!document.hidden) chase(); }, 2000);
    const syncTimer = setInterval(syncAudio, 1000);
    video.addEventListener('pause', () => { if (audio && mode === 'mse') audio.pause(); });
    video.addEventListener('play', syncAudio);
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
      get zoom() { return z; },
      zoomBy(f) { zoomAt(f); },
      zoomReset() { z = 1; applyZoom(); },
      // another source in the same player: the picture stays until the new one starts
      open(url, live) { stop(); opts.url = url; opts.live = !!live; lastT = null; box.dataset.state = ''; connect(); },
      pause() { stop(); video.pause(); if (audio) audio.pause(); state('paused'); },
      // switch the sound on or off (on needs a user gesture in browsers); true when there is sound to hear
      sound(on) { soundOn = !!on; if (audio) { audio.muted = !soundOn; if (soundOn) { syncAudio(); audio.play().catch(() => {}); } else audio.pause(); } return box.dataset.audio === '1'; },
      get hasSound() { return box.dataset.audio === '1'; },
      close() {
        closed = true; stop(); clearInterval(stallTimer); clearInterval(syncTimer);
        resetMedia(); resetAudio(); box.remove();
      },
    };
  }
  // Players whose element left the page (a re-rendered dashboard, a closed dialog) are closed, so no WebSocket stays open.
  const all = new Set();
  setInterval(() => { for (const p of all) if (!p.el.isConnected) { if ((p._gone = (p._gone || 0) + 1) >= 2) { p.close(); all.delete(p); } } else p._gone = 0; }, 2000);
  window.ctrl32Player = (host, opts) => { const p = player(host, opts); all.add(p); const c = p.close; p.close = () => { all.delete(p); c(); }; return p; };
})();
