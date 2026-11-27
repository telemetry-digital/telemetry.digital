// ctrl32 telemetry — the trend control (spec 042): several curves on one time axis, an axis per unit (or per curve),
// live or a fixed window, panning and zooming, two rulers with the values under them, statistics of the window, alarm
// limits and alarms. Drawn on a canvas; the data come from GET /api/v1/observations, bucketed to the width of the
// picture so that a month is as quick as an hour.
(function () {
  'use strict';
  const T = (s) => (window.ctrl32 && typeof ctrl32.t === 'function' ? ctrl32.t(s) : s);
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const PALETTE = ['#2563eb', '#dc2626', '#059669', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d', '#ea580c', '#4f46e5', '#0d9488', '#b91c1c', '#a16207', '#9333ea', '#0369a1', '#be185d'];
  const RANGE_MS = { '5m': 300e3, '15m': 900e3, '1h': 3600e3, '6h': 21600e3, '8h': 28800e3, '12h': 43200e3, '24h': 86400e3, '7d': 7 * 86400e3, '30d': 30 * 86400e3, '90d': 90 * 86400e3 };
  const MIN_SPAN = 10e3, MAX_SPAN = 5 * 366 * 86400e3;
  const lang = () => document.documentElement.lang || undefined;
  const css = (name, fallback) => (getComputedStyle(document.documentElement).getPropertyValue(name) || '').trim() || fallback;
  const iso = (t) => encodeURIComponent(new Date(t).toISOString());
  const suspect = (q) => q !== 'ok' && q !== 'backfilled';

  function niceTicks(lo, hi, n) {
    if (!(hi > lo)) { hi = lo + 1; lo = lo - 1; }
    const raw = (hi - lo) / n, mag = Math.pow(10, Math.floor(Math.log10(raw))), norm = raw / mag;
    const step = (norm < 1.5 ? 1 : norm < 3 ? 2 : norm < 7 ? 5 : 10) * mag; const t = [];
    for (let v = Math.ceil(lo / step) * step; v <= hi + step * 1e-6; v += step) t.push(+v.toFixed(10));
    return t;
  }
  function timeTicks(t0, t1, n) {
    const span = t1 - t0, steps = [1e3, 5e3, 15e3, 30e3, 60e3, 300e3, 900e3, 1800e3, 3600e3, 7200e3, 6 * 3600e3, 12 * 3600e3, 86400e3, 2 * 86400e3, 7 * 86400e3, 30 * 86400e3, 90 * 86400e3, 365 * 86400e3];
    const step = steps.find((s) => span / s <= n) || steps[steps.length - 1]; const out = [];
    const off = new Date(t0).getTimezoneOffset() * 60e3; // ticks on local hours and days
    for (let t = Math.ceil((t0 - off) / step) * step + off; t <= t1; t += step) out.push(t);
    const f = step < 60e3 ? { hour: '2-digit', minute: '2-digit', second: '2-digit' } : step < 86400e3 ? { hour: '2-digit', minute: '2-digit' } : { day: '2-digit', month: '2-digit' };
    const day = { day: '2-digit', month: '2-digit' };
    return { ticks: out, fmt: (t) => (step < 86400e3 ? new Date(t).toLocaleTimeString(lang(), f) : new Date(t).toLocaleDateString(lang(), f)), day: (t) => new Date(t).toLocaleDateString(lang(), day), sub: step < 86400e3 };
  }
  const fmtNum = (v, d) => (v == null || !isFinite(v) ? '—' : Number(v).toLocaleString(lang(), { minimumFractionDigits: d, maximumFractionDigits: d }));
  const decOf = (lo, hi) => { const s = Math.abs(hi - lo); return s >= 100 ? 1 : s >= 10 ? 2 : s >= 1 ? 2 : s >= 0.1 ? 3 : 4; };
  const fmtTime = (t) => new Date(t).toLocaleString(lang(), { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  function fmtSpan(ms) {
    const s = Math.abs(ms) / 1000; if (s < 120) return s.toFixed(s < 10 ? 1 : 0) + ' s'; if (s < 7200) return (s / 60).toFixed(1) + ' min'; if (s < 172800) return (s / 3600).toFixed(1) + ' h'; return (s / 86400).toFixed(1) + ' d';
  }

  // create(host, opts) → controller. opts: {curves, range | from+to, limits, alarms, onChange(state), onData()}
  function create(host, opts) {
    const api = ctrl32.api;
    host.classList.add('tr-host');
    host.innerHTML = '<canvas class="tr-canvas" tabindex="0"></canvas><div class="tr-note muted"></div>';
    const cv = host.querySelector('canvas'), note = host.querySelector('.tr-note'), g = cv.getContext('2d');
    const S = {
      curves: [], meta: {}, data: {}, rules: {}, alarms: {},
      live: true, span: RANGE_MS['1h'], t1: Date.now(), limits: !!opts.limits, showAlarms: !!opts.alarms,
      hover: null, A: null, B: null, second: false, box: false, sel: null, loading: 0, err: '',
    };
    let W = 0, H = 0, plot = { x: 0, y: 0, w: 0, h: 0 }, axes = [], timer = null, seq = 0, destroyed = false, loadT = null;
    const t0 = () => S.t1 - S.span;
    const colorOf = (c, i) => c.color || PALETTE[i % PALETTE.length];
    const metaOf = (c) => S.meta[c.datastream_id] || {};
    const nameOf = (c) => c.label || (metaOf(c).asset ? metaOf(c).asset + ' / ' + metaOf(c).key : metaOf(c).key || c.datastream_id.slice(0, 8));
    const unitOf = (c) => metaOf(c).unit || '';
    const styleOf = (c) => c.style || (metaOf(c).kind === 'state' ? 'step' : 'line');

    // ---- data ----------------------------------------------------------------------------------------
    async function fetchCurve(c, a, b) {
      const span = (b - a) / 1000; let bucket = Math.ceil(span / 1500);
      const q = `/api/v1/observations?datastream_id=${c.datastream_id}&from=${iso(a)}&to=${iso(b)}`;
      if (bucket < 5) {
        const rows = await api('GET', q + '&limit=10000');
        if (rows.length < 10000) return { bucket: 0, a, b, pts: rows.filter((r) => r.value != null).map((r) => ({ t: Date.parse(r.measured_at), v: r.value, n: 1, flagged: suspect(r.quality) })) };
        bucket = 5;
      }
      const r = await api('GET', q + '&bucket=' + bucket);
      return { bucket, a, b, pts: (r.points || []).filter((p) => p.n > 0 && p.avg != null).map((p) => ({ t: Date.parse(p.t) + bucket * 500, v: p.avg, min: p.min, max: p.max, n: p.n, flagged: !!p.flagged })) };
    }
    // the window is loaded with half a window on each side, so that a small pan needs no new request
    function stale(d) { if (!d) return true; const want = Math.ceil(S.span * 2 / 1000 / 1500); return t0() < d.a || S.t1 > d.b || (d.bucket >= 5 && want < d.bucket / 2.2) || (want >= 5 && d.bucket < want / 2.2 && !(d.bucket === 0 && want < 5)); }
    async function load(force) {
      const my = ++seq, a = t0() - S.span / 2, b = Math.min(S.t1 + S.span / 2, Date.now() + 60e3);
      const todo = S.curves.filter((c) => force || stale(S.data[c.datastream_id]));
      if (!todo.length) { draw(); return; }
      S.loading++; draw();
      try {
        const res = await Promise.all(todo.map((c) => fetchCurve(c, a, b).catch((e) => ({ error: e.message, pts: [], a, b, bucket: 0 }))));
        if (my !== seq && !force) { return; }
        todo.forEach((c, i) => { S.data[c.datastream_id] = res[i]; });
        S.err = (res.find((r) => r.error) || {}).error || '';
        if (S.showAlarms) loadAlarms(a, b);
      } finally { S.loading--; draw(); if (opts.onData) opts.onData(); }
    }
    const later = () => { clearTimeout(loadT); loadT = setTimeout(() => load(false), 220); };
    async function loadMeta() {
      const need = S.curves.filter((c) => !S.meta[c.datastream_id]);
      if (!need.length) return;
      try { const all = opts.datastreams || await api('GET', '/api/v1/datastreams'); for (const d of all) S.meta[d.id] = d; } catch (e) { S.err = e.message; }
    }
    async function loadRules() {
      for (const c of S.curves) if (!(c.datastream_id in S.rules)) {
        S.rules[c.datastream_id] = [];
        api('GET', '/api/v1/alarm-rules?datastream_id=' + c.datastream_id).then((r) => { S.rules[c.datastream_id] = Array.isArray(r) ? r : []; draw(); }).catch(() => {});
      }
    }
    async function loadAlarms(a, b) {
      for (const c of S.curves) api('GET', `/api/v1/alarms?datastream_id=${c.datastream_id}`).then((r) => { S.alarms[c.datastream_id] = (Array.isArray(r) ? r : []).filter((x) => Date.parse(x.raised_at) < b && (!x.cleared_at || Date.parse(x.cleared_at) > a)); draw(); }).catch(() => {});
    }

    // ---- values --------------------------------------------------------------------------------------
    function valueAt(c, t) {
      const d = S.data[c.datastream_id]; if (!d || !d.pts.length) return null; const p = d.pts;
      let lo = 0, hi = p.length - 1; if (t < p[0].t || t > p[hi].t + (d.bucket ? d.bucket * 1000 : 0)) return t > p[hi].t && styleOf(c) === 'step' ? p[hi].v : (t < p[0].t ? null : (t - p[hi].t < gapOf(d) ? p[hi].v : null));
      while (hi - lo > 1) { const m = (lo + hi) >> 1; if (p[m].t <= t) lo = m; else hi = m; }
      if (styleOf(c) !== 'line' || hi === lo) return p[lo].v;
      if (p[hi].t - p[lo].t > gapOf(d)) return t - p[lo].t < p[hi].t - t ? p[lo].v : p[hi].v;
      return p[lo].v + (p[hi].v - p[lo].v) * (t - p[lo].t) / (p[hi].t - p[lo].t || 1);
    }
    // a hole in the data: more than a few usual steps between two points
    function gapOf(d) {
      if (d._gap) return d._gap; const p = d.pts; let g = d.bucket ? d.bucket * 3000 : 0;
      if (!g && p.length > 2) { const ds = []; for (let i = 1; i < p.length && i < 400; i++) ds.push(p[i].t - p[i - 1].t); ds.sort((x, y) => x - y); g = Math.max(ds[ds.length >> 1] * 5, 5000); }
      return (d._gap = g || Infinity);
    }
    function statsOf(c, a, b) {
      const d = S.data[c.datastream_id]; if (!d) return null; let n = 0, sum = 0, mn = Infinity, mx = -Infinity, first = null, last = null;
      for (const p of d.pts) { if (p.t < a || p.t > b) continue; const k = p.n || 1; n += k; sum += p.v * k; mn = Math.min(mn, p.min ?? p.v); mx = Math.max(mx, p.max ?? p.v); if (first == null) first = p.v; last = p.v; }
      return n ? { n, min: mn, max: mx, avg: sum / n, first, last } : null;
    }

    // ---- drawing -------------------------------------------------------------------------------------
    function layout() {
      const r = host.getBoundingClientRect(), dpr = window.devicePixelRatio || 1;
      W = Math.max(280, Math.floor(r.width)); H = Math.max(180, Math.floor(opts.height || (r.height ? r.height - 18 : 420)));
      if (cv.width !== W * dpr || cv.height !== H * dpr) { cv.width = W * dpr; cv.height = H * dpr; cv.style.width = W + 'px'; cv.style.height = H + 'px'; }
      g.setTransform(dpr, 0, 0, dpr, 0, 0);
      // axes: one per unit, or one of its own for a curve that asks for it
      const by = new Map();
      S.curves.forEach((c, i) => { if (c.hidden) return; const k = c.axis ? 'a:' + c.axis : 'u:' + unitOf(c); if (!by.has(k)) by.set(k, { key: k, label: c.axis || unitOf(c), curves: [], color: colorOf(c, i) }); by.get(k).curves.push(c); });
      axes = [...by.values()];
      const narrow = W < 560, shown = axes.slice(0, narrow ? 2 : 6), AW = narrow ? 44 : 54;
      let left = 0, right = 0;
      shown.forEach((ax, i) => { ax.side = i % 2 === 0 ? 'l' : 'r'; if (ax.side === 'l') ax.off = left++; else ax.off = right++; ax.w = AW; });
      axes.slice(shown.length).forEach((ax) => { ax.side = ''; });
      plot = { x: Math.max(1, left) * AW + 4, y: 22, w: 0, h: H - 22 - 34 };
      plot.w = W - plot.x - Math.max(right * AW + 4, 14);
      for (const ax of axes) {
        let lo = null, hi = null;
        for (const c of ax.curves) { if (c.min != null && lo == null) lo = c.min; if (c.max != null && hi == null) hi = c.max; }
        if (lo == null || hi == null) {
          let mn = Infinity, mx = -Infinity; const a = t0(), b = S.t1;
          for (const c of ax.curves) { const d = S.data[c.datastream_id]; if (!d) continue; for (const p of d.pts) { if (p.t < a || p.t > b) continue; mn = Math.min(mn, p.min ?? p.v); mx = Math.max(mx, p.max ?? p.v); } }
          if (S.limits) for (const c of ax.curves) for (const r of S.rules[c.datastream_id] || []) for (const v of [r.warning, r.action]) if (v != null && isFinite(mn) && Math.abs(v - (mn + mx) / 2) < (mx - mn) * 3 + Math.abs(v) * 0.5) { mn = Math.min(mn, v); mx = Math.max(mx, v); }
          if (!isFinite(mn)) { mn = 0; mx = 1; }
          const pad = (mx - mn) * 0.08 || Math.abs(mx) * 0.05 || 1;
          if (lo == null) lo = mn - pad; if (hi == null) hi = mx + pad;
          if (!(hi > lo)) hi = lo + 1;
        }
        ax.lo = lo; ax.hi = hi; ax.dec = decOf(lo, hi);
        for (const c of ax.curves) c._ax = ax;
      }
    }
    const X = (t) => plot.x + (t - t0()) / S.span * plot.w;
    const TX = (x) => t0() + (x - plot.x) / plot.w * S.span;
    const Y = (ax, v) => plot.y + plot.h - (v - ax.lo) / (ax.hi - ax.lo) * plot.h;

    function draw() {
      if (destroyed) return;
      layout();
      const ink = css('--fg', '#111827'), muted = css('--muted', '#6b7280'), line = css('--line', '#e5e7eb'), bg = css('--card', '#ffffff');
      g.clearRect(0, 0, W, H); g.fillStyle = bg; g.fillRect(0, 0, W, H);
      g.font = '11px system-ui, sans-serif'; g.textBaseline = 'middle'; g.lineWidth = 1;
      // time grid
      const tt = timeTicks(t0(), S.t1, Math.max(3, Math.floor(plot.w / 110)));
      g.strokeStyle = line; g.fillStyle = muted; g.textAlign = 'center';
      let lastDay = '';
      for (const t of tt.ticks) { const x = Math.round(X(t)) + 0.5; g.beginPath(); g.moveTo(x, plot.y); g.lineTo(x, plot.y + plot.h); g.stroke(); g.fillText(tt.fmt(t), x, plot.y + plot.h + 11);
        if (tt.sub) { const d = tt.day(t); if (d !== lastDay) { g.fillText(d, x, plot.y + plot.h + 24); lastDay = d; } } }
      // value grid of the first axis, labels of every axis shown
      const shown = axes.filter((a) => a.side);
      shown.forEach((ax, i) => {
        const ticks = niceTicks(ax.lo, ax.hi, Math.max(3, Math.floor(plot.h / 46))); const x = ax.side === 'l' ? plot.x - ax.off * ax.w : plot.x + plot.w + ax.off * ax.w;
        g.fillStyle = ax.color; g.textAlign = ax.side === 'l' ? 'right' : 'left';
        for (const v of ticks) { const y = Math.round(Y(ax, v)) + 0.5; if (y < plot.y - 1 || y > plot.y + plot.h + 1) continue;
          if (i === 0) { g.strokeStyle = line; g.beginPath(); g.moveTo(plot.x, y); g.lineTo(plot.x + plot.w, y); g.stroke(); }
          g.fillText(fmtNum(v, Math.max(0, ax.dec - 1)), ax.side === 'l' ? x - 5 : x + 5, y); }
        g.fillText(ax.label || '', ax.side === 'l' ? x - 5 : x + 5, 9);
        g.strokeStyle = ax.color; g.beginPath(); g.moveTo(Math.round(x) + 0.5, plot.y); g.lineTo(Math.round(x) + 0.5, plot.y + plot.h); g.stroke();
      });
      g.save(); g.beginPath(); g.rect(plot.x, plot.y, plot.w, plot.h); g.clip();
      // alarm limits
      if (S.limits) S.curves.forEach((c, i) => { if (c.hidden || !c._ax) return; for (const r of S.rules[c.datastream_id] || []) for (const [v, col] of [[r.warning, '#d97706'], [r.action, '#dc2626']]) { if (v == null) continue; const y = Math.round(Y(c._ax, v)) + 0.5; g.strokeStyle = col; g.setLineDash([6, 4]); g.beginPath(); g.moveTo(plot.x, y); g.lineTo(plot.x + plot.w, y); g.stroke(); g.setLineDash([]);
        g.fillStyle = col; g.textAlign = 'left'; g.fillText(fmtNum(v, c._ax.dec) + ' ' + nameOf(c), plot.x + 6, y - 7); } });
      // curves
      S.curves.forEach((c, i) => {
        const d = S.data[c.datastream_id]; if (c.hidden || !d || !d.pts.length || !c._ax) return; const ax = c._ax, col = colorOf(c, i), st = styleOf(c), gap = gapOf(d), p = d.pts;
        let a = 0; while (a < p.length - 1 && p[a + 1].t < t0()) a++;
        let b = p.length - 1; while (b > 0 && p[b - 1].t > S.t1) b--;
        if (d.bucket && st === 'line') { // the band between the lowest and the highest value of each bucket
          g.fillStyle = col + '26'; let open = false, seg = [];
          const flush = () => { if (seg.length > 1) { g.beginPath(); seg.forEach((q, k) => (k ? g.lineTo(X(q.t), Y(ax, q.max)) : g.moveTo(X(q.t), Y(ax, q.max)))); for (let k = seg.length - 1; k >= 0; k--) g.lineTo(X(seg[k].t), Y(ax, seg[k].min)); g.closePath(); g.fill(); } seg = []; };
          for (let k = a; k <= b; k++) { if (p[k].min == null) continue; if (open && p[k].t - seg[seg.length - 1].t > gap) flush(); seg.push(p[k]); open = true; } flush();
        }
        g.strokeStyle = col; g.fillStyle = col; g.lineWidth = c.width || 1.6; g.lineJoin = 'round';
        if (st === 'points') { for (let k = a; k <= b; k++) { g.beginPath(); g.arc(X(p[k].t), Y(ax, p[k].v), 2, 0, 6.3); g.fill(); } }
        else {
          g.beginPath(); let pen = false;
          for (let k = a; k <= b; k++) { const x = X(p[k].t), y = Y(ax, p[k].v);
            if (pen && p[k].t - p[k - 1].t > gap) { if (st === 'step') g.lineTo(X(p[k - 1].t + Math.min(gap, 1e9)), Y(ax, p[k - 1].v)); pen = false; }
            if (!pen) { g.moveTo(x, y); pen = true; } else if (st === 'step') { g.lineTo(x, Y(ax, p[k - 1].v)); g.lineTo(x, y); } else g.lineTo(x, y); }
          if (pen && st === 'step' && S.live && b === p.length - 1) g.lineTo(X(Math.min(S.t1, Date.now())), Y(ax, p[b].v));
          g.stroke();
          if (b - a < 2 || (b - a) < plot.w / 14) for (let k = a; k <= b; k++) { g.beginPath(); g.arc(X(p[k].t), Y(ax, p[k].v), 2.2, 0, 6.3); g.fill(); }
        }
        g.lineWidth = 1;
        for (let k = a; k <= b; k++) if (p[k].flagged && (b - a) < plot.w / 3) { g.strokeStyle = '#d97706'; g.beginPath(); g.arc(X(p[k].t), Y(ax, p[k].v), 4, 0, 6.3); g.stroke(); }
      });
      // alarms: a bar along the top from the moment an alarm was raised until it cleared
      if (S.showAlarms) { let row = 0; S.curves.forEach((c, i) => { const list = S.alarms[c.datastream_id] || []; if (c.hidden || !list.length) return;
        for (const al of list) { const xa = Math.max(plot.x, X(Date.parse(al.raised_at))), xb = Math.min(plot.x + plot.w, X(al.cleared_at ? Date.parse(al.cleared_at) : Date.now())); if (xb < plot.x || xa > plot.x + plot.w) continue;
          g.fillStyle = al.severity === 'warning' ? '#d97706' : '#dc2626'; g.globalAlpha = 0.85; g.fillRect(xa, plot.y + 2 + row * 6, Math.max(3, xb - xa), 4); g.globalAlpha = 1; }
        row++; }); }
      // the zoom box
      if (S.sel) { g.fillStyle = 'rgba(37,99,235,.14)'; g.fillRect(Math.min(S.sel.a, S.sel.b), plot.y, Math.abs(S.sel.b - S.sel.a), plot.h); }
      // rulers
      const ruler = (t, col, tag) => { if (t == null || t < t0() || t > S.t1) return; const x = Math.round(X(t)) + 0.5; g.strokeStyle = col; g.setLineDash(tag ? [] : [3, 3]); g.beginPath(); g.moveTo(x, plot.y); g.lineTo(x, plot.y + plot.h); g.stroke(); g.setLineDash([]);
        S.curves.forEach((c, i) => { if (c.hidden || !c._ax) return; const v = valueAt(c, t); if (v == null) return; g.fillStyle = bg; g.strokeStyle = colorOf(c, i); g.lineWidth = 2; g.beginPath(); g.arc(x, Y(c._ax, v), 3.5, 0, 6.3); g.fill(); g.stroke(); g.lineWidth = 1; });
        if (tag) { g.fillStyle = col; g.fillRect(x - 8, plot.y, 16, 14); g.fillStyle = '#fff'; g.textAlign = 'center'; g.fillText(tag, x, plot.y + 7.5); } };
      ruler(S.hover, muted, ''); ruler(S.A, ink, 'A'); ruler(S.B, '#2563eb', 'B');
      g.restore();
      g.strokeStyle = line; g.strokeRect(plot.x + 0.5, plot.y + 0.5, plot.w, plot.h);
      const empty = S.curves.length && !S.loading && S.curves.every((c) => c.hidden || !(S.data[c.datastream_id] || { pts: [] }).pts.some((p) => p.t >= t0() && p.t <= S.t1));
      g.fillStyle = muted; g.textAlign = 'center';
      if (!S.curves.length) g.fillText(T('Add a curve to start.'), plot.x + plot.w / 2, plot.y + plot.h / 2);
      else if (empty) g.fillText(T('no data in this range'), plot.x + plot.w / 2, plot.y + plot.h / 2);
      const b0 = S.curves.map((c) => (S.data[c.datastream_id] || {}).bucket || 0).reduce((x, y) => Math.max(x, y), 0);
      note.textContent = S.err ? T(S.err) : (S.loading ? T('loading…') : (b0 ? T('Each point is the average of') + ' ' + fmtSpan(b0 * 1000) + '; ' + T('the band shows the lowest and the highest value.') : ''));
      if (opts.onChange) opts.onChange(state());
    }

    // ---- interaction ---------------------------------------------------------------------------------
    const inPlot = (x, y) => x >= plot.x && x <= plot.x + plot.w && y >= plot.y && y <= plot.y + plot.h;
    const pos = (e) => { const r = cv.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top }; };
    function setWindow(a, b, keepLive) {
      let span = Math.min(MAX_SPAN, Math.max(MIN_SPAN, b - a)); const now = Date.now();
      S.span = span; S.t1 = Math.min(b, now + span * 0.02 + 1000);
      if (!keepLive) S.live = false;
      schedule(); draw(); later();
    }
    function zoom(f, at) { const c = at == null ? S.t1 - S.span / 2 : at, a = c - (c - t0()) * f, b = c + (S.t1 - c) * f; setWindow(a, b, S.live && at == null && f < 1 ? false : S.live && b >= Date.now() - 1000); }
    let drag = null;
    cv.addEventListener('pointerdown', (e) => { const p = pos(e); if (!inPlot(p.x, p.y)) return; try { cv.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer */ } drag = { x: p.x, t1: S.t1, moved: false, box: S.box || e.shiftKey }; });
    cv.addEventListener('pointermove', (e) => { const p = pos(e);
      if (drag) { const dx = p.x - drag.x; if (Math.abs(dx) > 4) drag.moved = true;
        if (drag.moved && drag.box) { S.sel = { a: drag.x, b: Math.max(plot.x, Math.min(plot.x + plot.w, p.x)) }; draw(); }
        else if (drag.moved) { S.live = false; S.t1 = Math.min(drag.t1 - dx / plot.w * S.span, Date.now() + S.span * 0.02); S.hover = null; draw(); later(); }
        return; }
      const h = inPlot(p.x, p.y) ? TX(p.x) : null; if (h !== S.hover) { S.hover = h; draw(); } });
    cv.addEventListener('pointerup', (e) => { if (!drag) return; const p = pos(e), d = drag; drag = null;
      if (d.moved && d.box && S.sel) { const a = TX(Math.min(S.sel.a, S.sel.b)), b = TX(Math.max(S.sel.a, S.sel.b)); S.sel = null; if (b - a >= MIN_SPAN / 2) setWindow(a, b); else draw(); return; }
      if (!d.moved) { const t = TX(p.x); if (S.second || e.ctrlKey || e.altKey) S.B = t; else S.A = t; draw(); } else { schedule(); } });
    cv.addEventListener('pointerleave', () => { if (S.hover != null && !drag) { S.hover = null; draw(); } });
    cv.addEventListener('wheel', (e) => { const p = pos(e); if (!inPlot(p.x, p.y)) return; e.preventDefault(); zoom(e.deltaY > 0 ? 1.25 : 0.8, TX(p.x)); }, { passive: false });
    cv.addEventListener('keydown', (e) => { const k = e.key; if (k === 'ArrowLeft') pan(-0.2); else if (k === 'ArrowRight') pan(0.2); else if (k === '+' || k === '=') zoom(0.67); else if (k === '-') zoom(1.5); else if (k === 'Escape') { S.A = S.B = null; draw(); } else return; e.preventDefault(); });
    function pan(f) { setWindow(t0() + S.span * f, S.t1 + S.span * f); }
    const ro = new ResizeObserver(() => draw()); ro.observe(host);

    // live: the window follows the clock; long windows are refreshed less often
    function schedule() { clearInterval(timer); timer = null; if (!S.live) return; const every = S.span <= 900e3 ? 2000 : S.span <= 6 * 3600e3 ? 5000 : 30000;
      timer = setInterval(() => { if (document.hidden || drag) return; S.t1 = Date.now(); load(true); }, every); }

    function state() {
      return { live: S.live, t0: t0(), t1: S.t1, span: S.span, hover: S.hover, A: S.A, B: S.B, second: S.second, box: S.box, limits: S.limits, alarms: S.showAlarms,
        rows: S.curves.map((c, i) => { const s = statsOf(c, t0(), S.t1), d = S.data[c.datastream_id] || {}; const dec = c._ax ? c._ax.dec : 2;
          return { curve: c, i, name: nameOf(c), unit: unitOf(c), color: colorOf(c, i), style: styleOf(c), dec, bucket: d.bucket || 0, error: d.error || '',
            cur: S.hover != null ? valueAt(c, S.hover) : null, a: S.A != null ? valueAt(c, S.A) : null, b: S.B != null ? valueAt(c, S.B) : null, stats: s }; }) };
    }
    const ctl = {
      state,
      setCurves(list) { S.curves = list.map((c) => ({ ...c })); for (const k of Object.keys(S.data)) if (!S.curves.some((c) => c.datastream_id === k)) delete S.data[k]; return loadMeta().then(() => { if (S.limits) loadRules(); return load(false); }); },
      curves: () => S.curves.map((c) => { const o = { ...c }; delete o._ax; return o; }),
      setRange(r) { S.live = true; S.span = RANGE_MS[r] || RANGE_MS['1h']; S.t1 = Date.now(); schedule(); draw(); return load(false); },
      setWindow(a, b) { setWindow(a, b); },
      setLive(on) { S.live = !!on; if (S.live) S.t1 = Date.now(); schedule(); draw(); return load(S.live); },
      pan, zoom: (f) => zoom(f),
      toggle(k, on) { if (k === 'limits') { S.limits = on; if (on) loadRules(); } else if (k === 'alarms') { S.showAlarms = on; if (on) loadAlarms(t0() - S.span / 2, S.t1 + S.span / 2); } else if (k === 'second') S.second = on; else if (k === 'box') S.box = on; draw(); },
      clearRulers() { S.A = S.B = null; draw(); },
      setRuler(which, t) { S[which] = t; draw(); },
      // the loaded points of the window, one row per time, for the export
      table() { const a = t0(), b = S.t1, times = new Set(); const cols = S.curves.filter((c) => !c.hidden);
        const maps = cols.map((c) => { const m = new Map(); for (const p of (S.data[c.datastream_id] || { pts: [] }).pts) if (p.t >= a && p.t <= b) { m.set(p.t, p.v); times.add(p.t); } return m; });
        return { cols: cols.map((c) => ({ name: nameOf(c), unit: unitOf(c), bucket: (S.data[c.datastream_id] || {}).bucket || 0 })), rows: [...times].sort((x, y) => x - y).map((t) => [t, ...maps.map((m) => (m.has(t) ? m.get(t) : null))]) }; },
      png: () => new Promise((res) => cv.toBlob(res, 'image/png')),
      redraw: draw, reload: () => load(true),
      destroy() { destroyed = true; clearInterval(timer); clearTimeout(loadT); ro.disconnect(); host.innerHTML = ''; },
    };
    if (opts.from && opts.to) { S.live = false; S.t1 = +new Date(opts.to); S.span = Math.max(MIN_SPAN, S.t1 - +new Date(opts.from)); } else { S.span = RANGE_MS[opts.range] || RANGE_MS['1h']; }
    schedule();
    ctl.setCurves(opts.curves || []);
    return ctl;
  }
  window.ctrl32Trend = { create, PALETTE, RANGE_MS, fmtNum, fmtTime, fmtSpan, esc };
})();
