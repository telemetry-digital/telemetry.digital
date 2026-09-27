// ctrl32 telemetry — screen renderer and widget library (screen format v1, ADR-0008 / URS J-02).
// A screen is {version, kind, layout:{type:"grid", columns:12, row_height} | {type:"canvas", width, height}, range,
// refresh_s, widgets:[{id, type, geometry:{x,y,w,h}, bindings:{datastreams:[], devices:[]}, params:{}}]}. Kind "mimic"
// uses the canvas layout (pixels) and process symbols. Widgets read through the public API only.
// No dependencies except Leaflet (vendored, BSD-2) for the map widget, loaded on demand.
(function () {
  'use strict';
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const api = (m, u, b) => window.ctrl32.api(m, u, b);
  const fmt = (t) => window.ctrl32.fmt(t);
  // Public mode: a dashboard published at /p/<token> reads its data from /api/v1/public/<token>/… (no session).
  let PUBLIC = '';
  const P = (path) => PUBLIC ? path.replace('/api/v1/', '/api/v1/public/' + encodeURIComponent(PUBLIC) + '/') : path;
  function setPublic(token) { PUBLIC = token || ''; }
  // "today" runs from local midnight (production screens: output of the day)
  const RANGE_MS = { '1h': 3600e3, '6h': 6 * 3600e3, '8h': 8 * 3600e3, '12h': 12 * 3600e3, '24h': 24 * 3600e3, '7d': 7 * 86400e3, '30d': 30 * 86400e3, '90d': 90 * 86400e3,
    get today() { const d = new Date(); d.setHours(0, 0, 0, 0); return Math.max(60e3, Date.now() - d.getTime()); } };
  const COLORS = ['#e30613', '#2563eb', '#059669', '#d97706', '#7c3aed', '#db2777', '#0891b2', '#65a30d', '#ea580c', '#4b5563', '#0d9488', '#9333ea'];
  const num = (v, d) => (v == null || isNaN(v)) ? '—' : Number(v).toFixed(d == null ? 2 : d);
  const color = (i) => COLORS[i % COLORS.length];

  // ---- data ---------------------------------------------------------------------------------------
  let dsCache = null, dsAt = 0;
  async function datastreams() { if (!dsCache || Date.now() - dsAt > 60e3) { dsCache = await api('GET', P('/api/v1/datastreams')); dsAt = Date.now(); } return dsCache; }
  function dsInfo(all, id) { return all.find((d) => d.id === id) || { key: id.slice(0, 8), unit: '', asset: '?' }; }
  const dsLabel = (d) => d.label || `${d.asset} / ${d.key}`;
  // sInfo: datastream info with the widget's per-series settings applied (label, colour, unit, decimals, hidden).
  function sInfo(all, id, i, p) { const d = dsInfo(all, id), c = ((p && p.series) || [])[i] || {}; return Object.assign({}, d, { label: c.label || '', unit: c.unit != null && c.unit !== '' ? c.unit : d.unit, color: c.color || color(i), dec: c.decimals ?? (p ? p.decimals : undefined), hidden: !!c.hidden }); }
  function bucketFor(range) { const ms = RANGE_MS[range] || RANGE_MS['24h']; const s = Math.round(ms / 600 / 1000); return s >= 60 ? Math.ceil(s / 60) * 60 : 0; }
  async function series(id, range) {
    const ms = RANGE_MS[range] || RANGE_MS['24h']; const from = new Date(Date.now() - ms).toISOString(); const b = bucketFor(range);
    if (b > 0) { const r = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${encodeURIComponent(from)}&bucket=${b}`)); return r.points.map((p) => ({ t: new Date(p.t).getTime(), v: p.avg, min: p.min, max: p.max, flagged: p.flagged })); }
    const rows = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${encodeURIComponent(from)}&limit=10000`));
    return rows.filter((r) => r.value != null).map((r) => ({ t: new Date(r.measured_at).getTime(), v: r.value, flagged: r.quality !== 'ok' }));
  }
  async function hourly(id, range) {
    const ms = RANGE_MS[range] || RANGE_MS['7d']; const from = new Date(Date.now() - ms).toISOString();
    const r = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${encodeURIComponent(from)}&bucket=3600`));
    return r.points.map((p) => ({ t: new Date(p.t).getTime(), v: p.avg }));
  }
  async function latest(ids) { if (!ids.length) return {}; return api('GET', P('/api/v1/datastreams/latest?ids=' + ids.join(','))); }
  async function devices(ids) { const all = await api('GET', P('/api/v1/devices')); return ids && ids.length ? all.filter((d) => ids.includes(d.id)) : all; }
  const devState = (d) => { if (d.connected) return ['online', 'ok', '#059669']; const ls = d.last_seen ? new Date(d.last_seen).getTime() : 0; if (!ls) return ['never', '', '#6b7280']; return Date.now() - ls < 3600e3 ? ['recent', 'warn', '#d97706'] : ['offline', 'bad', '#dc2626']; };
  function level(v, p) { if (v == null) return ''; if ((p.action != null && v >= p.action) || (p.low_action != null && v <= p.low_action)) return 'bad'; if ((p.warn != null && v >= p.warn) || (p.low_warn != null && v <= p.low_warn)) return 'warn'; return 'ok'; }
  const lvlColor = (lvl, fallback, p) => lvl === 'bad' ? ((p && p.action_color) || '#dc2626') : (lvl === 'warn' ? ((p && p.warn_color) || '#d97706') : fallback);

  // ---- SVG chart ----------------------------------------------------------------------------------
  function niceTicks(lo, hi, n) { if (!(hi > lo)) { hi = lo + 1; lo = lo - 1; } const raw = (hi - lo) / n, mag = Math.pow(10, Math.floor(Math.log10(raw))), norm = raw / mag; const step = (norm < 1.5 ? 1 : norm < 3 ? 2 : norm < 7 ? 5 : 10) * mag; const t = []; for (let v = Math.ceil(lo / step) * step; v <= hi + 1e-9; v += step) t.push(+v.toFixed(10)); return t; }
  function timeTicks(t0, t1, n) { const span = t1 - t0, steps = [60e3, 300e3, 900e3, 1800e3, 3600e3, 7200e3, 6 * 3600e3, 12 * 3600e3, 86400e3, 2 * 86400e3, 7 * 86400e3, 30 * 86400e3]; const step = steps.find((s) => span / s <= n) || steps[steps.length - 1]; const out = []; for (let t = Math.ceil(t0 / step) * step; t <= t1; t += step) out.push(t); return { ticks: out, fmt: span <= 2 * 86400e3 ? (t) => new Date(t).toLocaleTimeString(document.documentElement.lang || [], { hour: '2-digit', minute: '2-digit' }) : (t) => new Date(t).toLocaleDateString(document.documentElement.lang || [], { day: '2-digit', month: '2-digit' }) }; }
  // Series with an explicit window and aggregation (spec 002): buckets carry avg/min/max/n, so sum = avg·n and count = n.
  const AGG_S = { '1m': 60, '5m': 300, '15m': 900, '1h': 3600, '6h': 21600, '1d': 86400 };
  function aggPlan(t0, t1, agg, interval) {
    const span = (t1 - t0) / 1000, floor = Math.ceil(span / 2000); // at most ~2000 points
    if (agg === 'none' && span <= 366 * 86400) return { bucket: 0, raised: false };
    let b = AGG_S[interval] || 0; if (!b) { const s = Math.round(span / 600); b = s >= 60 ? Math.ceil(s / 60) * 60 : (agg && agg !== 'auto' && agg !== 'none' ? 60 : 0); }
    if (b && b < floor) return { bucket: Math.ceil(floor / 60) * 60, raised: true };
    return { bucket: b, raised: false };
  }
  async function seriesWin(id, t0, t1, agg, interval) {
    const plan = aggPlan(t0, t1, agg, interval); const from = encodeURIComponent(new Date(t0).toISOString()), to = encodeURIComponent(new Date(t1).toISOString());
    if (!plan.bucket) { const rows = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${from}&to=${to}&limit=10000`)); return { plan, points: rows.filter((r) => r.value != null).map((r) => ({ t: new Date(r.measured_at).getTime(), v: r.value, flagged: r.quality !== 'ok' })) }; }
    const r = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${from}&to=${to}&bucket=${plan.bucket}`)); const f = agg || 'auto';
    const pick = (p) => f === 'min' ? p.min : f === 'max' ? p.max : f === 'sum' ? (p.avg == null ? null : p.avg * p.n) : f === 'count' ? p.n : p.avg;
    const band = f === 'auto' || f === 'avg';
    return { plan, points: r.points.filter((p) => p.n > 0).map((p) => ({ t: new Date(p.t).getTime(), v: pick(p), min: band ? p.min : null, max: band ? p.max : null, flagged: p.flagged })) };
  }
  // short texts inside widgets follow the user's (or the display's) language
  const TR = (s) => (window.ctrl32 && typeof ctrl32.t === 'function' ? ctrl32.t(s) : s);
  const rangeLabel = (r) => r === 'today' ? TR('today') : String(r || '').replace(/^(\d+)([hd])$/, '$1 $2');
  const aggLabel = (f) => TR({ avg: 'average', min: 'minimum', max: 'maximum', sum: 'sum', count: 'count', delta: 'change', none: 'raw' }[f] || f);
  function winOf(p, ctx) {
    if (p.time_mode === 'fixed' && p.from && p.to) { const t0 = Date.parse(p.from), t1 = Date.parse(p.to); if (t1 > t0) return { t0, t1, label: new Date(t0).toLocaleString(document.documentElement.lang || undefined) + ' – ' + new Date(t1).toLocaleString(document.documentElement.lang || undefined) }; }
    const range = p.range || ctx.range; const t1 = Date.now(); return { t0: t1 - (RANGE_MS[range] || RANGE_MS['24h']), t1, label: rangeLabel(range || '24h'), range };
  }
  const statsOf = (pts) => { const vs = pts.map((p) => p.v).filter((v) => v != null); if (!vs.length) return {}; const total = vs.reduce((a, b) => a + b, 0); return { min: Math.min(...vs), max: Math.max(...vs), avg: total / vs.length, total, latest: vs[vs.length - 1] }; };
  function renderChart(el, list, o) {
    const W = 960, H = Math.max(160, o.height || 240), T = 12, B = 30; const vis = list.filter((s) => !s.hidden); const all = vis.flatMap((s) => s.points);
    if (!all.length) { el.innerHTML = '<div class="muted" style="padding:24px 0;text-align:center">no data in this range</div>'; return; }
    const hasR = vis.some((s) => s.axis === 'right'), L = 54, R = hasR ? 54 : 14;
    const t0 = o.t0, t1 = o.t1;
    const scale = (ss, yMin, yMax) => { let lo = yMin, hi = yMax; const pts = ss.flatMap((s) => s.points); if (lo == null || hi == null) { const vals = pts.flatMap((p) => [p.v, p.min, p.max]).filter((v) => v != null); if (!vals.length) vals.push(0, 1); const mn = Math.min(...vals), mx = Math.max(...vals), pad = (mx - mn) * 0.08 || 1; const bars = ss.some((s) => (s.style || o.style) === 'bar'); if (lo == null) lo = bars ? Math.min(0, mn) : mn - pad; if (hi == null) hi = mx + pad; } return { lo, hi }; };
    const sl = scale(vis.filter((s) => s.axis !== 'right'), o.yMin, o.yMax), sr = hasR ? scale(vis.filter((s) => s.axis === 'right')) : null;
    const px = (t) => L + (t - t0) / (t1 - t0) * (W - L - R), pyOf = (sc) => (v) => T + (1 - (v - sc.lo) / (sc.hi - sc.lo)) * (H - T - B), py = pyOf(sl);
    const yt = niceTicks(sl.lo, sl.hi, 5), xt = timeTicks(t0, t1, 8);
    let g = `<g stroke="var(--line)" stroke-width="1">${yt.map((v) => `<line x1="${L}" x2="${W - R}" y1="${py(v).toFixed(1)}" y2="${py(v).toFixed(1)}"/>`).join('')}</g>`;
    g += `<g fill="var(--muted)" font-size="11">${yt.map((v) => `<text x="${L - 6}" y="${(py(v) + 4).toFixed(1)}" text-anchor="end">${num(v, o.decimals)}</text>`).join('')}${xt.ticks.map((t) => `<text x="${px(t).toFixed(1)}" y="${H - 9}" text-anchor="middle">${xt.fmt(t)}</text>`).join('')}</g>`;
    if (sr) { const pr = pyOf(sr), rs = vis.filter((s) => s.axis === 'right'); g += `<g fill="var(--muted)" font-size="11">${niceTicks(sr.lo, sr.hi, 5).map((v) => `<text x="${W - R + 6}" y="${(pr(v) + 4).toFixed(1)}">${num(v, o.decimals)}</text>`).join('')}<text x="${W - 4}" y="${T + 2}" text-anchor="end" font-size="10">${esc(rs.map((s) => s.unit || s.label).filter((x, i, a) => a.indexOf(x) === i).join(', '))}</text></g>`; }
    if (o.warn != null) g += `<line x1="${L}" x2="${W - R}" y1="${py(o.warn).toFixed(1)}" y2="${py(o.warn).toFixed(1)}" stroke="#d97706" stroke-dasharray="4 4"/>`;
    if (o.action != null) g += `<line x1="${L}" x2="${W - R}" y1="${py(o.action).toFixed(1)}" y2="${py(o.action).toFixed(1)}" stroke="#dc2626" stroke-dasharray="4 4"/>`;
    const nBars = vis.filter((s) => (s.style || o.style) === 'bar').length || 1; let barI = 0;
    list.forEach((s, i) => {
      if (s.hidden) return; const c = s.color || color(i), pts = s.points; if (!pts.length) return; const style = s.style || o.style || 'line'; const yp = s.axis === 'right' && sr ? pyOf(sr) : py; const sc = s.axis === 'right' && sr ? sr : sl; const y0 = yp(Math.max(sc.lo, Math.min(sc.hi, 0)));
      if (o.band !== false && style !== 'bar' && style !== 'scatter') { const band = pts.filter((p) => p.min != null && p.max != null); if (band.length > 1) g += `<path d="${band.map((p, k) => (k ? 'L' : 'M') + px(p.t).toFixed(1) + ',' + yp(p.max).toFixed(1)).join(' ')} ${band.slice().reverse().map((p) => 'L' + px(p.t).toFixed(1) + ',' + yp(p.min).toFixed(1)).join(' ')} Z" fill="${c}" opacity="0.12"/>`; }
      if (style === 'scatter') { g += pts.map((p) => `<circle cx="${px(p.t).toFixed(1)}" cy="${yp(p.v).toFixed(1)}" r="2.6" fill="${c}" opacity="0.85"/>`).join(''); return; }
      if (style === 'bar') { const bw = Math.max(1.5, (W - L - R) / Math.max(pts.length, 1) * 0.8 / nBars), bi = barI++; g += pts.map((p) => { const x = px(p.t) - bw * nBars / 2 + bi * bw, y = yp(p.v); return `<rect x="${x.toFixed(1)}" y="${Math.min(y, y0).toFixed(1)}" width="${bw.toFixed(1)}" height="${Math.max(0.5, Math.abs(y0 - y)).toFixed(1)}" fill="${c}" opacity="0.85"/>`; }).join(''); return; }
      let d = '';
      if (style === 'step') pts.forEach((p, k) => { const x = px(p.t).toFixed(1), y = yp(p.v).toFixed(1); d += k ? ` H${x} V${y}` : `M${x},${y}`; });
      else d = pts.map((p, k) => (k ? 'L' : 'M') + px(p.t).toFixed(1) + ',' + yp(p.v).toFixed(1)).join(' ');
      if (style === 'area' || s.fill) g += `<path d="${d} L${px(pts[pts.length - 1].t).toFixed(1)},${y0.toFixed(1)} L${px(pts[0].t).toFixed(1)},${y0.toFixed(1)} Z" fill="${c}" opacity="0.18"/>`;
      g += `<path d="${d}" fill="none" stroke="${c}" stroke-width="1.6" vector-effect="non-scaling-stroke"/>`;
      if (s.showPoints) g += pts.map((p) => `<circle cx="${px(p.t).toFixed(1)}" cy="${yp(p.v).toFixed(1)}" r="2.2" fill="${c}"/>`).join('');
      g += pts.filter((p) => p.flagged).map((p) => `<circle cx="${px(p.t).toFixed(1)}" cy="${yp(p.v).toFixed(1)}" r="2.5" fill="#d97706"/>`).join('');
    });
    g += `<line class="cursor" x1="0" x2="0" y1="${T}" y2="${H - B}" stroke="var(--muted)" stroke-dasharray="2 3" visibility="hidden"/>`;
    el.style.display = 'flex'; el.style.flexDirection = 'column'; el.innerHTML = `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" style="width:100%;flex:1;min-height:60px;display:block">${g}</svg><div class="chart-tip muted" style="font-size:12px;height:16px;line-height:16px;flex:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis"></div>`;
    const svg = el.querySelector('svg'), tip = el.querySelector('.chart-tip'), cursor = svg.querySelector('.cursor');
    svg.onmousemove = (e) => { const rect = svg.getBoundingClientRect(); const x = (e.clientX - rect.left) / rect.width * W; const t = t0 + (x - L) / (W - L - R) * (t1 - t0); cursor.setAttribute('x1', x.toFixed(1)); cursor.setAttribute('x2', x.toFixed(1)); cursor.setAttribute('visibility', 'visible');
      tip.textContent = new Date(t).toLocaleString(document.documentElement.lang || undefined) + ' · ' + vis.map((s) => { const dd = s.decimals ?? o.decimals; let best = null; for (const p of s.points) if (!best || Math.abs(p.t - t) < Math.abs(best.t - t)) best = p; return best ? `${s.label}: ${num(best.v, dd)}${s.unit ? ' ' + s.unit : ''}${best.min != null ? ` (${num(best.min, dd)}–${num(best.max, dd)})` : ''}` : s.label + ': —'; }).join(' · '); };
    svg.onmouseleave = () => { cursor.setAttribute('visibility', 'hidden'); tip.textContent = ''; };
  }
  function sparkline(points, c) { if (points.length < 2) return ''; const t0 = points[0].t, t1 = points[points.length - 1].t, vs = points.map((p) => p.v), lo = Math.min(...vs), hi = Math.max(...vs); return `<svg viewBox="0 0 120 30" style="width:120px;height:30px;display:block;flex:none"><path d="${points.map((p, k) => (k ? 'L' : 'M') + ((p.t - t0) / ((t1 - t0) || 1) * 120).toFixed(1) + ',' + (28 - ((p.v - lo) / ((hi - lo) || 1)) * 26).toFixed(1)).join(' ')}" fill="none" stroke="${c}" stroke-width="1.5" vector-effect="non-scaling-stroke"/></svg>`; }

  // ---- widgets: (el, params, ctx {range, bindings, w, h}) -----------------------------------------
  const W = {};
  const ids = (ctx) => (ctx.bindings && ctx.bindings.datastreams) || [];
  W.chart = async (el, p, ctx) => {
    const all = await datastreams(); const win = winOf(p, ctx); const cfg = Array.isArray(p.series) ? p.series : []; let raised = false, bucket = 0;
    const list = await Promise.all(ids(ctx).map(async (id, i) => { const d = dsInfo(all, id), c = cfg[i] || {}; const r = await seriesWin(id, win.t0, win.t1, p.agg, p.agg_interval); raised = raised || r.plan.raised; bucket = r.plan.bucket;
      return { label: c.label || dsLabel(d), unit: c.unit != null && c.unit !== '' ? c.unit : d.unit, color: c.color || color(i), style: c.style || '', axis: c.axis || 'left', showPoints: !!c.points, fill: !!c.fill, hidden: !!c.hidden, decimals: c.decimals, points: r.points }; }));
    const pos = p.legend === false ? 'none' : (p.legend_pos || 'top'); const lv = Array.isArray(p.legend_values) ? p.legend_values : [];
    const aggTxt = aggLabel(p.agg && p.agg !== 'auto' ? p.agg : 'avg') + (bucket ? '/' + (bucket >= 3600 ? bucket / 3600 + ' h' : Math.round(bucket / 60) + ' min') : '');
    const info = `<span class="dw-win">${esc(win.label)}${bucket ? ' · ' + esc(aggTxt) : ''}${raised ? ' · <span title="interval raised to keep at most 2000 points">interval raised</span>' : ''}</span>`;
    const draw = () => {
      const legend = pos === 'none' ? '' : list.map((s, i) => { const st = statsOf(s.points); const vals = lv.map((k) => `<span class="muted"> ${k} <b>${num(st[k], s.decimals ?? p.decimals)}</b></span>`).join(''); return `<span class="dw-leg${s.hidden ? ' off' : ''}" data-s="${i}" style="display:inline-flex;align-items:center;gap:4px;margin-right:12px;cursor:pointer${s.hidden ? ';opacity:.45' : ''}"><i style="display:inline-block;width:12px;height:3px;background:${s.color}"></i>${esc(s.label)}${s.unit ? ' [' + esc(s.unit) + ']' : ''}${s.axis === 'right' ? ' ▸' : ''}${vals}</span>`; }).join('');
      const side = pos === 'right';
      el.innerHTML = `<div style="display:flex;flex-direction:${side ? 'row' : 'column'};flex:1;min-height:0;gap:6px">${pos === 'bottom' || side ? '' : `<div class="muted dw-legend" style="font-size:12px;margin-bottom:2px;display:flex;justify-content:space-between;flex-wrap:wrap"><span>${legend}</span>${info}</div>`}<div class="chart-box" style="flex:1;min-height:0;min-width:0"></div>${pos === 'bottom' ? `<div class="muted dw-legend" style="font-size:12px;display:flex;justify-content:space-between;flex-wrap:wrap"><span>${legend}</span>${info}</div>` : ''}${side ? `<div class="muted dw-legend" style="font-size:12px;display:flex;flex-direction:column;gap:4px;max-width:40%;overflow:auto">${legend}${info}</div>` : ''}${pos === 'none' ? `<div class="muted" style="font-size:11px;text-align:right">${info}</div>` : ''}</div>`;
      el.style.display = 'flex'; el.style.flexDirection = 'column';
      renderChart(el.querySelector('.chart-box'), list, { t0: win.t0, t1: win.t1, yMin: p.y_min, yMax: p.y_max, decimals: p.decimals, warn: p.warn, action: p.action, style: p.chart, band: p.band, height: Math.max(160, ctx.px_h - 60) });
      for (const x of el.querySelectorAll('.dw-leg')) x.onclick = (e) => { e.stopPropagation(); const s = list[+x.dataset.s]; s.hidden = !s.hidden; draw(); };
    };
    draw();
  };
  W.bars = async (el, p, ctx) => {
    const all = await datastreams(); const lv = await latest(ids(ctx)); const rows = ids(ctx).map((id, i) => ({ d: sInfo(all, id, i, p), v: lv[id] ? lv[id].value : null, })).filter((r) => !r.d.hidden).map((r) => Object.assign(r, { c: r.d.color }));
    const vals = rows.map((r) => r.v).filter((v) => v != null); const lo = p.y_min != null ? p.y_min : Math.min(0, ...vals), hi = p.y_max != null ? p.y_max : Math.max(1, ...vals) * 1.05;
    el.innerHTML = rows.map((r) => { const frac = r.v == null ? 0 : Math.max(0, Math.min(1, (r.v - lo) / ((hi - lo) || 1))); const lvl = level(r.v, p); return `<div style="margin:4px 0"><div style="display:flex;justify-content:space-between;font-size:12px"><span>${esc(dsLabel(r.d))}</span><b style="color:${lvlColor(lvl, 'inherit', p)}">${num(r.v, r.d.dec)} ${esc(r.d.unit)}</b></div><div style="height:10px;background:var(--line);border-radius:3px;overflow:hidden"><div style="width:${(frac * 100).toFixed(1)}%;height:100%;background:${lvlColor(lvl, r.c, p)}"></div></div></div>`; }).join('');
  };
  W.donut = async (el, p, ctx) => {
    const all = await datastreams(); const lv = await latest(ids(ctx)); const rows = ids(ctx).map((id, i) => ({ d: sInfo(all, id, i, p), v: Math.max(0, (lv[id] && lv[id].value) || 0), })).filter((r) => !r.d.hidden).map((r) => Object.assign(r, { c: r.d.color })); const sum = rows.reduce((a, r) => a + r.v, 0) || 1;
    let a = -Math.PI / 2; const arcs = rows.map((r) => { const f = r.v / sum, a1 = a + f * 2 * Math.PI; const big = f > 0.5 ? 1 : 0; const x0 = 60 + 50 * Math.cos(a), y0 = 60 + 50 * Math.sin(a), x1 = 60 + 50 * Math.cos(a1), y1 = 60 + 50 * Math.sin(a1); const path = f >= 0.9999 ? `<circle cx="60" cy="60" r="50" fill="none" stroke="${r.c}" stroke-width="18"/>` : (f > 0 ? `<path d="M${x0.toFixed(2)},${y0.toFixed(2)} A50,50 0 ${big} 1 ${x1.toFixed(2)},${y1.toFixed(2)}" fill="none" stroke="${r.c}" stroke-width="18"/>` : ''); a = a1; return path; }).join('');
    el.innerHTML = `<div style="display:flex;gap:14px;align-items:center;flex-wrap:wrap"><svg viewBox="0 0 120 120" style="width:120px;height:120px;flex:none"><circle cx="60" cy="60" r="50" fill="none" stroke="var(--line)" stroke-width="18"/>${arcs}</svg><div style="font-size:12px">${rows.map((r) => `<div style="display:flex;align-items:center;gap:6px"><i style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${r.c}"></i>${esc(dsLabel(r.d))} <b>${num(r.v, r.d.dec)}</b> <span class="muted">${(r.v / sum * 100).toFixed(0)}%</span></div>`).join('')}</div></div>`;
  };
  W.heatmap = async (el, p, ctx) => {
    const all = await datastreams(); const id = ids(ctx)[0]; const d = sInfo(all, id, 0, p); const pts = await hourly(id, p.range || '7d');
    if (!pts.length) { el.innerHTML = '<div class="muted">no data</div>'; return; }
    const days = [...new Set(pts.map((x) => new Date(x.t).toDateString()))]; const vals = pts.map((x) => x.v).filter((v) => v != null); const lo = p.y_min != null ? p.y_min : Math.min(...vals), hi = p.y_max != null ? p.y_max : Math.max(...vals);
    const cell = new Map(pts.map((x) => [new Date(x.t).toDateString() + '|' + new Date(x.t).getHours(), x.v]));
    const shade = (v) => { if (v == null) return 'var(--line)'; const f = Math.max(0, Math.min(1, (v - lo) / ((hi - lo) || 1))); return `hsl(${Math.round(210 - f * 210)}, 80%, ${Math.round(88 - f * 40)}%)`; };
    el.innerHTML = `<div class="muted" style="font-size:12px;margin-bottom:4px">${esc(dsLabel(d))} [${esc(d.unit)}] · hourly average · ${num(lo, p.decimals)} → ${num(hi, p.decimals)}</div><div style="flex:1;min-height:0;overflow:auto"><div style="display:grid;grid-template-columns:auto repeat(24, minmax(0, 1fr));gap:2px;font-size:10px;align-items:center"><div></div>${Array.from({ length: 24 }, (_, h) => `<div style="text-align:center;color:var(--muted);overflow:hidden">${h % 3 ? '' : h}</div>`).join('')}${days.map((day) => `<div style="white-space:nowrap;color:var(--muted);padding-right:4px">${esc(new Date(day).toLocaleDateString(document.documentElement.lang || [], { day: '2-digit', month: '2-digit' }))}</div>${Array.from({ length: 24 }, (_, h) => { const v = cell.get(day + '|' + h); return `<div title="${esc(day)} ${h}:00 — ${num(v, d.dec)}" style="height:14px;background:${shade(v)};border-radius:2px"></div>`; }).join('')}`).join('')}</div></div>`;
  };
  W.value = async (el, p, ctx) => {
    const all = await datastreams(); const list = ids(ctx); const lv = await latest(list);
    const cards = await Promise.all(list.map(async (id, i) => { const d = sInfo(all, id, i, p), v = lv[id]; if (d.hidden) return ''; const pts = (p.sparkline === false || ctx.w < 3) ? [] : await series(id, '6h').catch(() => []); const cf = i === 0 && ctx.cond ? ctx.cond(v && v.value) : condFormat(null, p, v && v.value); const c = cf.value || lvlColor(level(v && v.value, p), p.value_color || p.text_color || d.color, p);
      return `<div style="display:flex;justify-content:space-between;align-items:center;gap:10px;padding:2px 0;${i ? 'border-top:1px solid var(--line)' : ''}"><div style="min-width:0;flex:1"><div style="font-size:var(--dw-value,1.6em);font-weight:600;color:${c};line-height:1.1;white-space:nowrap">${v ? num(v.value, d.dec) : '—'} <span style="font-size:0.5em;font-weight:400;color:var(--muted)">${esc(d.unit)}</span></div><div class="muted dw-lbl" style="font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis" title="${esc(dsLabel(d))}">${esc(dsLabel(d))}${v ? ' · ' + fmt(v.measured_at) + (v.quality !== 'ok' ? ' · ' + esc(v.quality) : '') : ' · no value yet'}</div></div>${sparkline(pts, c)}</div>`; }));
    el.style.justifyContent = list.length === 1 ? 'center' : 'flex-start'; el.innerHTML = cards.join('');
  };
  W.gauge = async (el, p, ctx) => {
    const all = await datastreams(); const list = ids(ctx); const lv = await latest(list);
    const arc = (a, b, r) => `M${(70 + r * Math.cos(a)).toFixed(1)},${(70 + r * Math.sin(a)).toFixed(1)} A${r},${r} 0 ${b - a > Math.PI ? 1 : 0} 1 ${(70 + r * Math.cos(b)).toFixed(1)},${(70 + r * Math.sin(b)).toFixed(1)}`;
    el.innerHTML = `<div style="display:flex;flex-wrap:wrap;gap:8px;justify-content:space-around">` + list.map((id, i) => { const d = sInfo(all, id, i, p), v = lv[id] && lv[id].value; if (d.hidden) return ''; const lo = p.y_min ?? 0, hi = p.y_max ?? 100; const frac = v == null ? 0 : Math.max(0, Math.min(1, (v - lo) / ((hi - lo) || 1))); const cf = i === 0 && ctx.cond ? ctx.cond(v) : condFormat(null, p, v); const c = cf.value || lvlColor(level(v, p), p.value_color || p.text_color || d.color, p);
      const zone = (val, col) => (val == null || val < lo || val > hi) ? '' : `<path d="${arc(Math.PI + (val - lo) / (hi - lo) * Math.PI, 2 * Math.PI, 58)}" fill="none" stroke="${col}" stroke-width="4" opacity="0.6"/>`;
      return `<div style="text-align:center"><svg viewBox="0 0 140 84" style="width:150px;height:90px"><path d="${arc(Math.PI, 2 * Math.PI, 58)}" fill="none" stroke="var(--line)" stroke-width="10"/>${zone(p.warn, '#d97706')}${zone(p.action, '#dc2626')}${v == null ? '' : `<path d="${arc(Math.PI, Math.PI + Math.max(frac, 0.002) * Math.PI, 58)}" fill="none" stroke="${c}" stroke-width="10" stroke-linecap="round"/>`}<text x="70" y="66" text-anchor="middle" font-size="18" font-weight="600" fill="${c}">${v == null ? '—' : num(v, d.dec)}</text><text x="70" y="80" text-anchor="middle" font-size="9" fill="var(--muted)">${esc(d.unit)}</text><text x="12" y="82" font-size="8" fill="var(--muted)">${lo}</text><text x="128" y="82" font-size="8" fill="var(--muted)" text-anchor="end">${hi}</text></svg><div class="muted dw-lbl" style="font-size:11px">${esc(dsLabel(d))}</div></div>`; }).join('') + '</div>';
  };
  W.status = async (el, p, ctx) => {
    const all = await datastreams(); const list = ids(ctx); const lv = await latest(list);
    const states = (p.states || []).map((s) => { const m = /^\s*([^=]+)=([^:]*)(?::(#[0-9a-fA-F]{3,8}|[a-z]+))?\s*$/.exec(s); return m ? { v: m[1].trim(), label: m[2].trim(), color: m[3] || '#6b7280' } : null; }).filter(Boolean);
    el.innerHTML = `<div style="display:flex;flex-wrap:wrap;gap:10px">` + list.map((id, i) => { const d = sInfo(all, id, i, p); if (d.hidden) return ''; const v = lv[id] ? lv[id].value : null; const raw = lv[id] && lv[id].value_json != null && lv[id].value == null ? JSON.stringify(lv[id].value_json) : (v == null ? null : String(Number.isInteger(v) ? v : v.toFixed(2))); const st = states.find((s) => s.v === raw || (v != null && !isNaN(parseFloat(s.v)) && parseFloat(s.v) === v)) || { label: raw == null ? 'no value' : raw, color: '#6b7280' };
      return `<div style="display:flex;align-items:center;gap:8px;padding:6px 8px;border:1px solid var(--line);border-radius:var(--radius);flex:1 1 140px;min-width:0"><span style="width:14px;height:14px;border-radius:50%;background:${esc(st.color)};box-shadow:0 0 0 3px ${esc(st.color)}33;flex:none"></span><div style="min-width:0"><div style="font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${esc(st.label)}</div><div class="muted" style="font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${esc(dsLabel(d))}${lv[id] ? ' · ' + fmt(lv[id].measured_at) : ''}</div></div></div>`; }).join('') + '</div>';
  };
  W.table = async (el, p, ctx) => {
    const all = await datastreams(); const list = ids(ctx); const lv = await latest(list);
    el.innerHTML = `<div class="table-wrap"><table><thead><tr><th>Asset</th><th>Datastream</th><th style="text-align:right">Value</th><th>Unit</th><th>Measured</th><th>Quality</th></tr></thead><tbody>${list.map((id, i) => { const d = sInfo(all, id, i, p), v = lv[id]; if (d.hidden) return ''; const lvl = level(v && v.value, p); return `<tr><td>${esc(d.asset)}</td><td>${esc(d.label || d.key)}</td><td style="text-align:right;font-weight:600;color:${lvlColor(lvl, 'inherit', p)}">${v ? num(v.value, d.dec) : '—'}</td><td>${esc(d.unit)}</td><td>${v ? fmt(v.measured_at) : '—'}</td><td>${v ? (v.quality !== 'ok' ? `<span class="badge warn">${esc(v.quality)}</span>` : '<span class="badge ok">ok</span>') : ''}</td></tr>`; }).join('')}</tbody></table></div>`;
  };
  W.alarms = async (el, p) => {
    const al = await api('GET', P('/api/v1/alarms')); const lim = p.limit || 10; const active = al.filter((a) => !a.cleared_at); const rows = active.concat(al.filter((a) => a.cleared_at)).slice(0, lim);
    el.innerHTML = `<div class="muted" style="font-size:12px;margin-bottom:4px">${active.length} active</div><div class="table-wrap"><table><thead><tr><th>Raised</th><th>Severity</th><th>State</th><th>Key</th><th style="text-align:right">Value</th></tr></thead><tbody>${rows.map((a) => `<tr><td>${fmt(a.raised_at)}</td><td><span class="badge ${a.severity === 'action' ? 'bad' : 'warn'}">${esc(a.severity)}</span></td><td>${esc(a.state)}${a.cleared_at ? ' · cleared' : ''}</td><td>${esc(a.key || '')}</td><td style="text-align:right">${num(a.value, 2)}</td></tr>`).join('') || '<tr><td colspan="5" class="muted">no alarms</td></tr>'}</tbody></table></div>`;
  };
  W.devices = async (el, p, ctx) => {
    const ds = await devices(ctx.bindings && ctx.bindings.devices); const lim = p.limit || 20;
    el.innerHTML = `<div class="table-wrap"><table><thead><tr><th>Device</th><th>Transport</th><th>State</th><th>Last seen</th></tr></thead><tbody>${ds.filter((d) => !d.revoked_at).slice(0, lim).map((d) => { const [s, b] = devState(d); return `<tr><td>${PUBLIC ? esc(d.name || d.external_id) : '<a href="/devices/' + d.id + '">' + esc(d.name || d.external_id) + '</a>'}</td><td>${esc(d.transport)}</td><td><span class="badge ${b}">${s}</span></td><td>${d.last_seen ? fmt(d.last_seen) : '—'}</td></tr>`; }).join('') || '<tr><td colspan="4" class="muted">no devices</td></tr>'}</tbody></table></div>`;
  };
  // ---- control widgets: every send is a command with a reason; disabled in public mode and without device.command ----
  const canCmd = () => !PUBLIC && window.ctrl32 && typeof ctrl32.can === 'function' && ctrl32.can('device.command');
  function ctlHead(ctx, p) { const dev = ((ctx.bindings || {}).devices || [])[0]; if (!dev) return { dev: null, html: '<span class="muted">select a device</span>' }; if (!p.key) return { dev, html: '<span class="muted">set the key in the widget properties</span>' }; if (!canCmd()) return { dev, html: '<span class="muted">' + (PUBLIC ? 'read-only' : 'no permission to send commands') + '</span>' }; return { dev, html: '' }; }
  async function ctlSend(el, ctx, p, value) {
    const dev = ((ctx.bindings || {}).devices || [])[0]; const out = el.querySelector('.ctl-out'); const reasonEl = el.querySelector('.ctl-reason'); const reason = (reasonEl && reasonEl.value || '').trim();
    if (!reason) { out.innerHTML = '<span style="color:#d97706">reason is required</span>'; if (reasonEl) reasonEl.focus(); return; }
    if (p.confirm !== false && !window.confirm((p.title || p.key) + ' → ' + value + '?')) return;
    out.innerHTML = '<span class="muted">sending…</span>';
    try { const r = await api('POST', '/api/v1/devices/' + dev + '/commands', { method: p.method || 'write', args: { key: p.key, value }, reason });
      showCmd(out, r.status, { readback: r.result && r.result.readback, error: r.error });
      if (r.status === 'queued' || r.status === 'sent' || r.status === 'pending') PENDING.set(r.id, out);
      if (reasonEl) reasonEl.value = '';
    } catch (e) { out.innerHTML = '<span class="badge bad">' + esc(e.message) + '</span>'; }
  }
  async function ctlCurrent(ctx, p) { const id = ((ctx.bindings || {}).datastreams || [])[0]; if (!id) return null; const lv = await latest([id]).catch(() => ({})); return lv[id] || null; }
  const PENDING = new Map(); // command id -> output element waiting for an ack from the live stream
  function showCmd(out, st, r) { if (st === 'acked') out.innerHTML = '<span class="badge ok">acked</span> ' + (r && r.readback !== undefined ? '<span class="muted">read back ' + esc(String(r.readback)) + '</span>' : ''); else if (st === 'failed' || st === 'expired' || st === 'rejected') out.innerHTML = '<span class="badge bad">' + esc(st) + '</span> <span class="muted">' + esc((r && r.error) || '') + '</span>'; else if (st === 'pending') out.innerHTML = '<span class="badge warn">pending</span> <span class="muted">waiting for approval by another user</span>'; else out.innerHTML = '<span class="badge">' + esc(st) + '</span> <span class="muted">waiting for ack</span>'; }
  const reasonInput = () => '<input class="ctl-reason" placeholder="Reason (audit trail)" maxlength="200" style="flex:1;min-width:120px">';
  W.setpoint = async (el, p, ctx) => {
    const h = ctlHead(ctx, p); const cur = await ctlCurrent(ctx, p);
    const curHtml = cur ? '<div class="muted" style="font-size:12px">current <b>' + num(cur.value, p.decimals) + '</b> ' + esc(p.unit || '') + ' · ' + fmt(cur.measured_at) + '</div>' : '';
    if (h.html) { el.innerHTML = curHtml + h.html; return; }
    const attrs = (p.min != null ? ' min="' + p.min + '"' : '') + (p.max != null ? ' max="' + p.max + '"' : '') + ' step="' + (p.step || 'any') + '"';
    el.innerHTML = curHtml + '<div class="row" style="align-items:center;gap:6px;flex-wrap:wrap"><input class="ctl-value" type="number"' + attrs + ' value="' + (cur && cur.value != null ? cur.value : '') + '" style="width:110px"><span class="muted">' + esc(p.unit || '') + '</span>' + reasonInput() + '<button class="btn small primary ctl-send" type="button">Send</button></div><div class="ctl-out" style="font-size:12px;min-height:16px"></div>';
    el.querySelector('.ctl-send').onclick = () => { const v = parseFloat(el.querySelector('.ctl-value').value); if (isNaN(v)) { el.querySelector('.ctl-out').innerHTML = '<span style="color:#d97706">enter a number</span>'; return; } ctlSend(el, ctx, p, v); };
  };
  W.switch = async (el, p, ctx) => {
    const h = ctlHead(ctx, p); const cur = await ctlCurrent(ctx, p);
    const on = cur ? (cur.value === 1 || cur.value === true || (cur.value_json === true)) : null;
    const state = cur ? '<span class="badge ' + (on ? 'ok' : '') + '">' + esc(on ? (p.on_label || 'On') : (p.off_label || 'Off')) + '</span> <span class="muted" style="font-size:12px">' + fmt(cur.measured_at) + '</span>' : '';
    if (h.html) { el.innerHTML = state + ' ' + h.html; return; }
    el.innerHTML = '<div class="row" style="align-items:center;gap:6px;flex-wrap:wrap">' + state + '<button class="btn small ctl-on" type="button">' + esc(p.on_label || 'On') + '</button><button class="btn small ctl-off" type="button">' + esc(p.off_label || 'Off') + '</button>' + reasonInput() + '</div><div class="ctl-out" style="font-size:12px;min-height:16px"></div>';
    el.querySelector('.ctl-on').onclick = () => ctlSend(el, ctx, p, true); el.querySelector('.ctl-off').onclick = () => ctlSend(el, ctx, p, false);
  };
  W.button = async (el, p, ctx) => {
    const h = ctlHead(ctx, p); if (h.html) { el.innerHTML = h.html; return; }
    let v = p.value; if (v === 'true') v = true; else if (v === 'false') v = false; else if (v !== undefined && v !== '' && !isNaN(parseFloat(v))) v = parseFloat(v);
    el.innerHTML = '<div class="row" style="align-items:center;gap:6px;flex-wrap:wrap"><button class="btn small primary ctl-go" type="button">' + esc(p.label || 'Send') + '</button>' + reasonInput() + '</div><div class="ctl-out" style="font-size:12px;min-height:16px"></div>';
    el.querySelector('.ctl-go').onclick = () => ctlSend(el, ctx, p, v);
  };
  W.text = async (el, p) => { const paras = String(p.text || '').split('\n').filter((l) => l.trim()); const st = p.style || 'normal'; const box = st === 'note' ? 'background:var(--bg2);border:1px solid var(--line);padding:8px 10px;border-radius:var(--radius)' : (st === 'warning' ? 'background:#fff4e5;border:1px solid #e0a800;padding:8px 10px;border-radius:var(--radius);color:#7a4a00' : ''); el.innerHTML = `<div style="${box}">${paras.map((l) => st === 'heading' ? `<h2 style="margin:4px 0">${esc(l)}</h2>` : `<p style="margin:4px 0">${esc(l)}</p>`).join('') || '<span class="muted">empty</span>'}</div>`; };

  // Map: Leaflet (vendored) with the organization's tile source; devices with a position as coloured markers.
  let leafletReady = null;
  function loadLeaflet() { if (window.L) return Promise.resolve(); if (leafletReady) return leafletReady; leafletReady = new Promise((resolve, reject) => { const css = document.createElement('link'); css.rel = 'stylesheet'; css.href = '/vendor/leaflet.css'; document.head.appendChild(css); const s = document.createElement('script'); s.src = '/vendor/leaflet.js'; s.onload = resolve; s.onerror = () => reject(new Error('Leaflet could not be loaded')); document.head.appendChild(s); }); return leafletReady; }
  function mapConfig() { return (window.ctrl32.brand && window.ctrl32.brand.map) || {}; }
  function makeMap(box, cfg) { const map = L.map(box, { zoomControl: true, attributionControl: true }); L.tileLayer(cfg.tile_url, { maxZoom: cfg.max_zoom || 19, attribution: cfg.attribution || '' }).addTo(map); return map; }
  W.map = async (el, p, ctx) => {
    const cfg = mapConfig();
    if (!cfg.tile_url) { el.innerHTML = '<div class="muted" style="padding:24px;text-align:center">No map tiles configured. Set the tile URL under Settings → Branding (own tile server, a provider with a key, or OpenStreetMap for light use).</div>'; return; }
    await loadLeaflet();
    const devs = (await devices(ctx.bindings && ctx.bindings.devices)).filter((d) => d.location && d.location.lat != null && d.location.lon != null && !d.revoked_at);
    el.innerHTML = `<div class="map-box" style="height:100%;min-height:200px;border-radius:var(--radius);overflow:hidden"></div>`;
    const map = makeMap(el.querySelector('.map-box'), cfg); const pts = [];
    for (const d of devs) { const [s, , c] = devState(d); const ll = [d.location.lat, d.location.lon]; pts.push(ll); L.circleMarker(ll, { radius: 8, color: '#fff', weight: 2, fillColor: c, fillOpacity: 0.95 }).addTo(map).bindPopup(`<b>${esc(d.name || d.external_id)}</b><br>${s} · ${d.last_seen ? esc(fmt(d.last_seen)) : 'never'}<br><span style="font-size:11px">${d.location.source === 'fixed' ? 'fixed position' : 'reported ' + (d.location.updated_at ? esc(fmt(d.location.updated_at)) : '')}</span><br><a href="/devices/${d.id}">open device</a>`); }
    if (pts.length) map.fitBounds(pts, { padding: [24, 24], maxZoom: 15 }); else map.setView([cfg.center_lat || 48.7, cfg.center_lon || 19.5], p.zoom || cfg.zoom || 7);
    setTimeout(() => map.invalidateSize(), 50);
  };


  // ---- process symbols for mimics (ISA-101: grey process, colour only for abnormal states, values always visible) ----
  const SYM = { line: 'var(--fg)', body: '#cbd5e1', fill: '#64748b', on: '#334155', warn: '#d97706', bad: '#dc2626' };
  const symLabel = (p, d, fb) => esc((p && p.title) || (d && d.key) || fb);
  // a long symbol title shrinks to fit the 100-unit symbol instead of being cut off
  const symFont = (p, d, fb) => { const n = String((p && p.title) || (d && d.key) || fb).length; return Math.max(6, Math.min(11, 96 / (0.58 * n))).toFixed(1); };
  // state words (RUNNING, OPEN, …) follow the user's language; own labels stay as written
  const stT = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
  const svgBox = (vb, ar) => `<svg viewBox="${vb}" preserveAspectRatio="${ar || 'xMidYMid meet'}" style="width:100%;height:100%;display:block">`;
  async function symData(ctx) { const all = await datastreams(); const id = ids(ctx)[0]; const d = id ? dsInfo(all, id) : null; const lv = id ? await latest([id]) : {}; return { id, d, v: id && lv[id] ? lv[id].value : null }; }
  W.tank = async (el, p, ctx) => {
    const { d, v } = await symData(ctx); const lo = p.y_min ?? 0, hi = p.y_max ?? 100; const frac = v == null ? 0 : Math.max(0, Math.min(1, (v - lo) / ((hi - lo) || 1))); const lvl = level(v, p); const c = lvl === 'bad' ? SYM.bad : lvl === 'warn' ? SYM.warn : SYM.fill; const cid = 'c' + Math.random().toString(36).slice(2, 8); const top = 30 + 90 * (1 - frac);
    el.innerHTML = svgBox('0 0 100 140') + `<text x="50" y="14" text-anchor="middle" font-size="${symFont(p, d, 'Tank')}" fill="var(--muted)">${symLabel(p, d, 'Tank')}</text><defs><clipPath id="${cid}"><rect x="20" y="30" width="60" height="90" rx="8"/></clipPath></defs><rect x="20" y="${top.toFixed(1)}" width="60" height="${(120 - top).toFixed(1)}" fill="${c}" opacity="0.85" clip-path="url(#${cid})"/><rect x="20" y="30" width="60" height="90" rx="8" fill="none" stroke="${SYM.line}" stroke-width="2"/>${[0.25, 0.5, 0.75].map((f) => `<line x1="20" x2="27" y1="${(30 + 90 * (1 - f)).toFixed(1)}" y2="${(30 + 90 * (1 - f)).toFixed(1)}" stroke="${SYM.line}" stroke-width="1"/>`).join('')}<text x="50" y="136" text-anchor="middle" font-size="12" font-weight="600" fill="${lvl ? c : 'var(--fg)'}">${v == null ? '—' : num(v, p.decimals ?? 1)} ${esc((d && d.unit) || '')}</text></svg>`;
  };
  W.pump = async (el, p, ctx) => {
    const { d, v } = await symData(ctx); const on = v != null && v == (p.on_value ?? 1); const fault = p.fault_value != null && p.fault_value !== '' && v != null && v == p.fault_value; const stroke = fault ? SYM.bad : SYM.line; const state = stT(fault ? 'FAULT' : v == null ? '—' : on ? (p.on_label || 'RUNNING') : (p.off_label || 'STOPPED'));
    el.innerHTML = svgBox('0 0 100 120') + `<text x="50" y="14" text-anchor="middle" font-size="${symFont(p, d, 'Pump')}" fill="var(--muted)">${symLabel(p, d, 'Pump')}</text><circle cx="50" cy="62" r="32" fill="${on ? SYM.on : 'none'}" stroke="${stroke}" stroke-width="2.5"/><path d="M38,44 L74,62 L38,80 Z" fill="${on ? '#f8fafc' : 'none'}" stroke="${on ? '#f8fafc' : stroke}" stroke-width="2"/><text x="50" y="113" text-anchor="middle" font-size="11" font-weight="600" fill="${fault ? SYM.bad : 'var(--fg)'}">${esc(state)}</text></svg>`;
  };
  W.valve = async (el, p, ctx) => {
    const { d, v } = await symData(ctx); const open = v != null && v == (p.open_value ?? 1); const fault = p.fault_value != null && p.fault_value !== '' && v != null && v == p.fault_value; const stroke = fault ? SYM.bad : SYM.line; const fill = open ? SYM.on : 'none'; const rot = p.orientation === 'vertical' ? 'rotate(90 50 62)' : '';
    el.innerHTML = svgBox('0 0 100 120') + `<text x="50" y="14" text-anchor="middle" font-size="${symFont(p, d, 'Valve')}" fill="var(--muted)">${symLabel(p, d, 'Valve')}</text><g transform="${rot}"><path d="M20,44 L50,62 L20,80 Z" fill="${fill}" stroke="${stroke}" stroke-width="2.5"/><path d="M80,44 L50,62 L80,80 Z" fill="${fill}" stroke="${stroke}" stroke-width="2.5"/><line x1="50" y1="62" x2="50" y2="36" stroke="${stroke}" stroke-width="2.5"/><rect x="36" y="30" width="28" height="6" rx="1" fill="${stroke}"/></g><text x="50" y="113" text-anchor="middle" font-size="11" font-weight="600" fill="${fault ? SYM.bad : 'var(--fg)'}">${esc(stT(fault ? 'FAULT' : v == null ? '—' : open ? (p.open_label || 'OPEN') : (p.closed_label || 'CLOSED')))}</text></svg>`;
  };
  W.pipe = async (el, p, ctx) => {
    const { v } = await symData(ctx); const flowing = v != null && v > (p.flow_min ?? 0); const vert = p.orientation === 'vertical'; const t = Math.max(5, Math.min(100, p.thickness || 40));
    const body = vert ? `x="${50 - t / 2}" y="0" width="${t}" height="100"` : `x="0" y="${50 - t / 2}" width="100" height="${t}"`;
    el.innerHTML = svgBox('0 0 100 100', 'none') + `<rect ${body} fill="${SYM.body}" stroke="${SYM.line}" stroke-width="1" vector-effect="non-scaling-stroke"/>${flowing ? `<line x1="${vert ? 50 : 0}" y1="${vert ? 0 : 50}" x2="${vert ? 50 : 100}" y2="${vert ? 100 : 50}" stroke="${SYM.on}" stroke-width="${Math.max(2, t / 4)}" stroke-dasharray="8 8" class="mimic-flow" vector-effect="non-scaling-stroke"/>` : ''}</svg>`;
  };
  W.instrument = async (el, p, ctx) => {
    const { d, v } = await symData(ctx); const lvl = level(v, p); const c = lvl === 'bad' ? SYM.bad : lvl === 'warn' ? SYM.warn : SYM.line;
    el.innerHTML = svgBox('0 0 100 100') + `<circle cx="50" cy="50" r="44" fill="var(--card)" stroke="${c}" stroke-width="${lvl ? 4 : 2}"/><line x1="8" y1="50" x2="92" y2="50" stroke="${c}" stroke-width="1.5"/><text x="50" y="42" text-anchor="middle" font-size="13" font-weight="600" fill="var(--fg)">${esc(p.tag || (d && d.key) || '')}</text><text x="50" y="70" text-anchor="middle" font-size="15" font-weight="700" fill="${lvl ? c : 'var(--fg)'}">${v == null ? '—' : num(v, p.decimals ?? 1)}</text><text x="50" y="84" text-anchor="middle" font-size="9" fill="var(--muted)">${esc((d && d.unit) || '')}</text></svg>`;
  };
  // ---- screen renderer ----------------------------------------------------------------------------
  const LABELS = { setpoint: 'Setpoint', switch: 'Switch', button: 'Button', chart: 'Trend', bars: 'Compare', donut: 'Share', heatmap: 'Heatmap', value: 'Latest value', gauge: 'Gauge', status: 'State', table: 'Values', alarms: 'Alarms', devices: 'Devices', map: 'Map', text: 'Text', tank: 'Tank', pump: 'Pump', valve: 'Valve', pipe: 'Pipe', instrument: 'Instrument' };
  const labelOf = (t) => LABELS[t] || t;
  const NEEDS_DS = new Set(['chart', 'bars', 'donut', 'heatmap', 'value', 'gauge', 'status', 'table', 'tank', 'pump', 'valve', 'instrument']);
  const MIMIC_TYPES = new Set(['tank', 'pump', 'valve', 'pipe', 'instrument', 'camera_pin']);
  const isCanvas = (spec) => !!(spec.layout && spec.layout.type === 'canvas');
  const framed = (w) => MIMIC_TYPES.has(w.type) ? !!(w.params && w.params.frame) : !(w.params && w.params.frame === false);
  // ---- appearance, conditional formatting, click actions, full screen (spec 002) -------------------------------
  const ICON = (n, sz) => { const d = window.ctrl32Icons && window.ctrl32Icons[n]; return d ? `<svg class="dw-icon" viewBox="0 0 24 24" width="${sz || 16}" height="${sz || 16}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="flex:none;vertical-align:-3px">${d}</svg>` : ''; };
  function appearance(el, p) {
    const st = el.style;
    if (p.bg_color) st.background = p.bg_color;
    if (p.text_color) st.color = p.text_color;
    if (p.border_color) { st.borderStyle = 'solid'; st.borderColor = p.border_color; }
    if (p.border_width != null) { st.borderStyle = p.border_width ? 'solid' : 'none'; st.borderWidth = p.border_width + 'px'; }
    if (p.radius != null) st.borderRadius = p.radius + 'px';
    if (p.padding != null) st.padding = p.padding + 'px';
    if (p.shadow) st.boxShadow = '0 4px 14px rgba(0,0,0,.18)';
    if (p.transparent) { st.background = 'transparent'; st.border = 'none'; st.boxShadow = 'none'; }
    if (p.value_size) st.setProperty('--dw-value', p.value_size + 'px');
    el.dataset.bg = st.background || '';
    if (p.bg_color || p.text_color || (Array.isArray(p.ranges) && p.ranges.some((r) => r.target === 'background'))) el.classList.add('dw-tinted');
  }
  function decorate(card, w, opts, ctxOf) {
    const p = w.params || {}; appearance(card, p); if (p.tooltip) card.title = p.tooltip;
    const head = card.querySelector('.dash-head'), title = card.querySelector('.dash-title');
    const FONTS = { mono: 'var(--mono)', serif: 'Georgia, "Times New Roman", serif', condensed: '"Arial Narrow", "Roboto Condensed", sans-serif' };
    if (FONTS[p.font]) card.style.fontFamily = FONTS[p.font];
    if (p.show_label === false) card.classList.add('dw-nolabel');
    if (title) { if (p.title_color) title.style.color = p.title_color; if (p.title_bold === false) title.style.fontWeight = '400'; if (p.title_align === 'center' || p.title_align === 'right') { title.style.flex = '1'; title.style.justifyContent = p.title_align === 'center' ? 'center' : 'flex-end'; const g = head && head.querySelector('.grow'); if (g) g.style.flex = '0'; }
      if (p.title_size) title.style.fontSize = p.title_size + 'px'; if (p.title_icon) title.insertAdjacentHTML('afterbegin', ICON(p.title_icon, 16) + ' '); }
    if (p.show_title === false) { if (title) title.hidden = true; if (head && !opts.header) head.hidden = true; }
    if (opts.header) return; // editing: no full screen, no click actions
    if (p.fullscreen !== false && framed(w)) {
      const b = document.createElement('button'); b.type = 'button'; b.className = 'btn small ghost dw-fs'; b.title = 'Full screen'; b.innerHTML = ICON('maximize', 14) || '⤢';
      b.onclick = (e) => { e.stopPropagation(); fullView(w, ctxOf()); };
      if (head && !head.hidden) head.appendChild(b); else { b.classList.add('floating'); card.appendChild(b); }
    }
    if (p.click) { card.classList.add('dw-click'); card.addEventListener('click', (e) => { if (e.target.closest('button,input,select,textarea,a,.dw-leg,.leaflet-container')) return; clickAction(w); }); }
  }
  function clickAction(w) {
    const p = w.params || {}, tag = ((w.bindings || {}).datastreams || [])[0], dev = ((w.bindings || {}).devices || [])[0];
    if (p.click === 'screen') { if (p.click_target && !PUBLIC) location.href = '/dashboards/' + encodeURIComponent(p.click_target); return; }
    if (!tag || !window.ctrl32Scada || !window.ctrl32Scada.openTag) return;
    window.ctrl32Scada.openTag(tag, { trendOnly: p.click === 'trend', name: p.title, device: dev, key: p.key, decimals: p.decimals });
  }
  async function fullView(w, ctx) {
    const p = w.params || {}; const d = document.createElement('dialog'); d.className = 'dw-full';
    d.innerHTML = `<div class="dash-head"><b class="dash-title">${ICON(p.title_icon, 16)} ${esc(p.title || labelOf(w.type))}</b><span class="grow"></span><button class="btn small ghost" type="button" data-close title="Close (Esc)">✕</button></div><div class="dash-body"><span class="muted">loading…</span></div>`;
    appearance(d, p); if (!p.bg_color && !p.transparent) d.style.background = 'var(--bg)'; if (p.transparent) d.style.background = 'var(--bg)';
    document.body.appendChild(d); d.addEventListener('close', () => d.remove()); d.querySelector('[data-close]').onclick = () => d.close(); d.showModal();
    const body = d.querySelector('.dash-body'); const full = Object.assign({}, ctx, { w: 12, h: 8, px_h: Math.max(240, window.innerHeight - 110), cond: (v) => condFormat(d, p, v) });
    try { await (W[w.type] || (async () => {}))(body, p, full); } catch (e) { body.innerHTML = `<span class="muted">${esc(e.message)}</span>`; }
  }
  const matches = (r, v) => { switch (r.op) { case '>': return v > r.v; case '>=': return v >= r.v; case '<': return v < r.v; case '<=': return v <= r.v; case '==': return v === r.v; case '!=': return v !== r.v; case 'between': return v >= Math.min(r.v, r.v2) && v <= Math.max(r.v, r.v2); } return false; };
  // condFormat applies the first matching range: background → the card, value/icon → returned for the renderer.
  function condFormat(card, p, v) {
    const out = { value: null, icon: null, bg: null }; if (card) card.style.background = card.dataset.bg || '';
    if (v == null || isNaN(v) || !Array.isArray(p.ranges)) return out;
    const r = p.ranges.find((x) => matches(x, Number(v))); if (!r) return out;
    out[r.target === 'background' ? 'bg' : r.target] = r.color;
    if (r.target === 'background' && card) card.style.background = r.color;
    return out;
  }
  // Viewer filters (spec 002): the author enables them per widget (params.filters); the viewer switches time range,
  // aggregation, interval and series on the widget itself. Choices are remembered per widget in this browser.
  const FILTER_OPTS = { range: ['', '1h', '6h', '8h', '12h', 'today', '24h', '7d', '30d', '90d'], agg: ['auto', 'none', 'avg', 'min', 'max', 'sum', 'count'], agg_interval: ['auto', '1m', '5m', '15m', '1h', '6h', '1d'] };
  const FILTER_LBL = { range: 'Range', agg: 'Aggregation', agg_interval: 'Interval' };
  const fKey = (w) => 'ctrl32.wf.' + location.pathname + '.' + (w.id || '');
  function filterBar(card, w, redraw) {
    const p = w.params || {}; const f = Array.isArray(p.filters) ? p.filters : []; if (!f.length) return;
    let ovr = {}; try { ovr = JSON.parse(localStorage.getItem(fKey(w)) || '{}') || {}; } catch (e) { ovr = {}; }
    card._ovr = ovr; const bar = document.createElement('div'); bar.className = 'dw-filters';
    const save = () => { try { localStorage.setItem(fKey(w), JSON.stringify(card._ovr)); } catch (e) { /* private mode */ } redraw(); };
    const draw = () => {
      const sel = (k) => `<label>${FILTER_LBL[k]} <select data-f="${k}">${FILTER_OPTS[k].map((o) => `<option value="${o}" ${(ovr[k] ?? p[k] ?? '') === o ? 'selected' : ''}>${o === '' ? '(dashboard)' : esc(o)}</option>`).join('')}</select></label>`;
      let h = (f.includes('range') ? sel('range') : '') + (f.includes('agg') ? sel('agg') : '') + (f.includes('interval') ? sel('agg_interval') : '');
      if (f.includes('series')) { const list = (w.bindings || {}).datastreams || []; const cfg = ovr.series || p.series || []; h += list.map((id, i) => { const c = cfg[i] || {}; const on = !c.hidden; const d = dsCache ? dsInfo(dsCache, id) : null; return `<button type="button" class="dw-chip${on ? ' on' : ''}" data-si="${i}" aria-pressed="${on}" title="${esc(d ? dsLabel(d) : '')}"><i style="background:${esc(c.color || color(i))}"></i><span class="notranslate">${esc(c.label || (d ? d.key : '#' + (i + 1)))}</span></button>`; }).join(''); }
      if (Object.keys(ovr).length) h += '<button type="button" class="dw-chip" data-reset title="Back to the widget settings">↺</button>';
      bar.innerHTML = h;
      for (const s of bar.querySelectorAll('[data-f]')) s.onchange = () => { ovr[s.dataset.f] = s.value; draw(); save(); };
      for (const b of bar.querySelectorAll('[data-si]')) b.onclick = (e) => { e.stopPropagation(); const n = ((w.bindings || {}).datastreams || []).length; const base = ovr.series || p.series || []; ovr.series = Array.from({ length: n }, (_, i) => Object.assign({}, base[i] || {})); const s = ovr.series[+b.dataset.si]; if (s.hidden) delete s.hidden; else s.hidden = true; draw(); save(); };
      const r = bar.querySelector('[data-reset]'); if (r) r.onclick = (e) => { e.stopPropagation(); for (const k of Object.keys(ovr)) delete ovr[k]; draw(); save(); };
    };
    draw(); card.querySelector('.dash-body').before(bar); if (f.includes('series') && !dsCache) datastreams().then(draw).catch(() => {});
  }
  const ALARM_TYPES = new Set(['alarms', 'alarm_count']);
  function register(type, fn, o) { o = o || {}; W[type] = fn; if (o.label) LABELS[type] = o.label; if (o.needsDs) NEEDS_DS.add(type); if (o.alarms) ALARM_TYPES.add(type); }
  // Live updates (SSE): one stream per rendered dashboard; an event re-renders only the widgets bound to it, debounced.
  let ES = null, liveTimer = null;
  function connectLive(container, spec, refreshOne, layer) {
    if (ES) { ES.close(); ES = null; }
    const ds = new Set(), devs = new Set(); let allDevs = false, alarms = false;
    const layerTags = layer ? layer.tags() : new Set(); layerTags.forEach((id) => ds.add(id)); let layerDue = false;
    for (const w of spec.widgets || []) { const b = w.bindings || {}; (b.datastreams || []).forEach((id) => ds.add(id)); if (Array.isArray(b.devices)) { if (!b.devices.length && (w.type === 'map' || w.type === 'devices')) allDevs = true; b.devices.forEach((id) => devs.add(id)); } if (ALARM_TYPES.has(w.type)) alarms = true; }
    if (!ds.size && !devs.size && !allDevs && !alarms) return;
    const qs = 'datastreams=' + [...ds].join(',') + '&devices=' + (allDevs ? 'all' : [...devs].join(',')) + '&alarms=' + (alarms ? 1 : 0);
    const pending = new Set();
    const flush = () => { liveTimer = null; const cards = [...container.querySelectorAll('.dash-widget')]; for (const i of pending) { if (cards[i]) refreshOne(cards[i]); } pending.clear(); if (layerDue && layer) { layerDue = false; layer.refresh(); } };
    const mark = (pred) => { (spec.widgets || []).forEach((w, i) => { if (pred(w)) pending.add(i); }); if ((pending.size || layerDue) && !liveTimer) liveTimer = setTimeout(flush, 400); };
    const es = new EventSource(P('/api/v1/stream') + '?' + qs, { withCredentials: true }); ES = es;
    const state = (connected) => window.dispatchEvent(new CustomEvent('ctrl32:live', { detail: { connected } }));
    es.onopen = () => state(true); es.onerror = () => state(false);
    es.addEventListener('observation', (e) => { const d = JSON.parse(e.data); if (layerTags.has(d.datastream_id)) layerDue = true; mark((w) => ((w.bindings || {}).datastreams || []).includes(d.datastream_id)); });
    es.addEventListener('device', (e) => { const d = JSON.parse(e.data); mark((w) => (w.type === 'map' || w.type === 'devices') && (!((w.bindings || {}).devices || []).length || w.bindings.devices.includes(d.device_id))); });
    es.addEventListener('alarm', () => mark((w) => ALARM_TYPES.has(w.type)));
    es.addEventListener('command', (e) => { const d = JSON.parse(e.data); const out = PENDING.get(d.command_id); if (!out) return; const det = d.detail || {}; const res = det.result && typeof det.result === 'object' ? det.result : {}; showCmd(out, d.status, { readback: res.readback, error: det.error }); if (d.status !== 'queued' && d.status !== 'sent' && d.status !== 'pending') PENDING.delete(d.command_id); });
    window.addEventListener('pagehide', () => es.close(), { once: true });
  }
  window.addEventListener('ctrl32:live', (e) => { const b = document.querySelector('#liveBadge'); if (b) b.hidden = !e.detail.connected; });
  function place(card, g) { card.style.gridColumn = `${g.x + 1} / span ${g.w}`; card.style.gridRow = `${g.y + 1} / span ${g.h}`; }
  // Canvas (mimic) layout: widgets sit at absolute pixel positions on a fixed-size canvas that is scaled to the container width.
  function placeCanvas(card, g) { card.style.left = g.x + 'px'; card.style.top = g.y + 'px'; card.style.width = g.w + 'px'; card.style.height = g.h + 'px'; }
  function fitMimic(container) { const wrap = container.querySelector('.mimic-wrap'), m = container.querySelector('.mimic'); if (!wrap || !m) return 1; const W0 = +m.dataset.w || 1600, H0 = +m.dataset.h || 900; const s = Math.max(0.1, Math.min(1.5, (wrap.clientWidth || W0) / W0)); m.style.transform = `scale(${s})`; wrap.style.height = Math.round(H0 * s) + 'px'; return s; }
  // render(container, spec, {range, header(widget, index) -> html, onReady}) → refresh()
  async function render(container, spec, opts) {
    opts = opts || {}; const range = opts.range || spec.range || '24h'; const rowH = (spec.layout && spec.layout.row_height) || 96; const canvas = isCanvas(spec);
    if (canvas) {
      const W0 = spec.layout.width || 1600, H0 = spec.layout.height || 900;
      container.innerHTML = `<div class="mimic-wrap"><div class="mimic" data-w="${W0}" data-h="${H0}" style="width:${W0}px;height:${H0}px"><div class="sc-host"></div>${(spec.widgets || []).map((w, i) => { const f = framed(w); return `<div class="dash-widget mimic-widget${f ? ' card' : ''}" data-i="${i}" data-id="${esc(w.id || '')}">${f || opts.header ? `<div class="dash-head${f ? '' : ' mimic-tools'}">${f ? `<b class="dash-title">${esc((w.params && w.params.title) || labelOf(w.type))}</b>` : ''}<span class="grow"></span>${opts.header ? opts.header(w, i) : ''}</div>` : ''}<div class="dash-body"><span class="muted">loading…</span></div></div>`; }).join('')}</div></div>`;
      for (const card of container.querySelectorAll('.dash-widget')) placeCanvas(card, spec.widgets[+card.dataset.i].geometry || { x: 0, y: 0, w: 200, h: 200 });
      // Drawing layer (spec 001): vector objects with dynamics below the widgets.
      container._scada = (window.ctrl32Scada && (opts.edit || (spec.elements || []).length || (spec.layout && spec.layout.bg))) ? window.ctrl32Scada.mount(container.querySelector('.sc-host'), spec, { edit: !!opts.edit }) : null;
      fitMimic(container); if (!container._fit) { container._fit = () => fitMimic(container); window.addEventListener('resize', container._fit); }
    } else {
      container.innerHTML = `<div class="dash-grid" style="--row-h:${rowH}px">${(spec.widgets || []).map((w, i) => `<div class="card dash-widget" data-i="${i}" data-id="${esc(w.id || '')}"><div class="dash-head"><b class="dash-title">${esc((w.params && w.params.title) || labelOf(w.type))}</b><span class="grow"></span>${opts.header ? opts.header(w, i) : ''}</div><div class="dash-body"><span class="muted">loading…</span></div></div>`).join('')}</div>`;
      for (const card of container.querySelectorAll('.dash-widget')) place(card, spec.widgets[+card.dataset.i].geometry || { x: 0, y: 0, w: 6, h: 3 });
    }
    const ctxOf = (w, card) => { const g = w.geometry || { w: 6, h: 3 }; const c = canvas ? { range, bindings: w.bindings || {}, w: Math.max(1, Math.round(g.w / 130)), h: Math.max(1, Math.round(g.h / 96)), px_h: g.h } : { range, bindings: w.bindings || {}, w: g.w, h: g.h, px_h: g.h * rowH }; c.cond = (v) => condFormat(card, w.params || {}, v); c.edit = !!opts.header; return c; };
    for (const card of container.querySelectorAll('.dash-widget')) { const w = spec.widgets[+card.dataset.i]; decorate(card, w, opts, () => ctxOf(w, null)); }
    for (const card of container.querySelectorAll('.dash-widget')) filterBar(card, spec.widgets[+card.dataset.i], () => refreshOne(card));
    const refreshOne = async (card) => {
      const w0 = spec.widgets[+card.dataset.i]; const w = card._ovr ? Object.assign({}, w0, { params: Object.assign({}, w0.params, card._ovr) }) : w0; const body = card.querySelector('.dash-body');
      if (NEEDS_DS.has(w.type) && !((w.bindings || {}).datastreams || []).length) { body.innerHTML = '<span class="muted">Select datastreams in the widget properties.</span>'; return; }
      const ctx = ctxOf(w, card);
      try { await (W[w.type] || (async (el) => { el.innerHTML = '<span class="muted">unknown widget</span>'; }))(body, w.params || {}, ctx); }
      catch (e) { body.innerHTML = `<span class="muted">${esc(e.message)}</span>`; }
    };
    const layer = canvas ? container._scada : null;
    const refresh = async () => Promise.all([...container.querySelectorAll('.dash-widget')].map(refreshOne).concat(layer ? [layer.refresh()] : []));
    await refresh();
    if (opts.live !== false && window.EventSource) connectLive(container, spec, refreshOne, layer);
    return refresh;
  }
  window.ctrl32Widgets = { tr: TR, aggLabel, rangeLabel, isPublic: () => !!PUBLIC, isDisplay: () => PUBLIC.startsWith('~'), apiPath: P, render, renderChart, setPublic, series, datastreams, latest, devices, loadLeaflet, makeMap, mapConfig, RANGE_MS, labelOf, isCanvas, fitMimic, placeCanvas, MIMIC_TYPES, register, condFormat, icon: ICON, get types() { return Object.keys(W); },
    // helpers for renderers registered from other files (widgets-more.js)
    util: { esc, api, fmt, num, color, COLORS, dsInfo, sInfo, dsLabel, ids, level, lvlColor, niceTicks, timeTicks, sparkline, seriesWin, winOf, statsOf, hourly, ctlHead, ctlSend, ctlCurrent, reasonInput, canCmd, P } };
})();
