// ctrl32 telemetry — additional dashboard widget types (spec 002, US3). Every renderer is registered with
// ctrl32Widgets.register(type, fn(el, params, ctx), opts) and gets the common appearance, conditional formatting
// (ctx.cond(value) → {value, icon, bg} colours), full-screen view and click actions from widgets.js.
// All user text is escaped; colours are validated by the server (hex only).
(() => {
  const WG = window.ctrl32Widgets; if (!WG) return;
  const u = WG.util, { esc, num, color, dsInfo, dsLabel, ids, level, lvlColor, api, P, fmt } = u; const sInfo = u.sInfo;
  const reg = (t, fn, o) => WG.register(t, fn, o);
  const icon = (n, sz) => WG.icon(n, sz);
  const clamp = (x, a, b) => Math.max(a, Math.min(b, x));
  const numVal = (x) => (x && typeof x.value === 'number' && !isNaN(x.value)) ? x.value : (x && typeof x.value_json === 'boolean' ? (x.value_json ? 1 : 0) : null);
  const lohi = (p, dlo, dhi) => { const lo = p.y_min ?? dlo, hi = p.y_max ?? dhi; return hi > lo ? [lo, hi] : [lo, lo + 1]; };
  const noData = (el, txt) => { el.innerHTML = `<div class="dw-center muted">${esc(txt || 'no value yet')}</div>`; };
  async function first(ctx, p) { const all = await WG.datastreams(); const id = ids(ctx)[0]; const d = id ? sInfo(all, id, 0, p) : { key: '', unit: '', asset: '', dec: p && p.decimals }; const lv = id ? await WG.latest([id]).catch(() => ({})) : {}; return { id, d, x: id ? lv[id] : null, v: id ? numVal(lv[id]) : null }; }
  async function many(ctx, p) { const all = await WG.datastreams(); const list = ids(ctx); const lv = await WG.latest(list).catch(() => ({})); return list.map((id, i) => { const d = sInfo(all, id, i, p); return { id, i, d, x: lv[id], v: numVal(lv[id]), c: d.color }; }).filter((r) => !r.d.hidden); }
  const unitOf = (p, d) => (p.unit != null && p.unit !== '') ? p.unit : (d.unit || '');
  const colFor = (ctx, p, v, fb) => { const cf = ctx.cond ? ctx.cond(v) : {}; return { c: cf.value || lvlColor(level(v, p), p.value_color || fb, p), ic: cf.icon }; };
  const svg = (vb, body, style) => `<svg viewBox="${vb}" preserveAspectRatio="xMidYMid meet" style="width:100%;height:100%;display:block;${style || ''}">${body}</svg>`;
  const parseStates = (list) => (list || []).map((s) => { const m = /^\s*([^=]+)=([^:]*)(?::(#[0-9a-fA-F]{3,8}))?\s*$/.exec(s); return m ? { v: m[1].trim(), label: m[2].trim(), color: m[3] || '#6b7280' } : null; }).filter(Boolean);
  const stateOf = (states, v) => states.find((s) => String(v) === s.v || (v != null && !isNaN(parseFloat(s.v)) && parseFloat(s.v) === Number(v)));
  // Raw buckets for a window (avg/min/max/n) — aggregate cards and bars over time.
  async function buckets(id, t0, t1, want) { const span = (t1 - t0) / 1000; const b = Math.max(want || 60, Math.ceil(span / 2000 / 60) * 60); const r = await api('GET', P(`/api/v1/observations?datastream_id=${id}&from=${encodeURIComponent(new Date(t0).toISOString())}&to=${encodeURIComponent(new Date(t1).toISOString())}&bucket=${b}`)); return { bucket: b, points: r.points.filter((x) => x.n > 0).map((x) => ({ t: new Date(x.t).getTime(), avg: x.avg, min: x.min, max: x.max, n: x.n })) }; }

  // ---- values ---------------------------------------------------------------------------------------------------
  reg('value_card', async (el, p, ctx) => {
    const { id, d, x, v } = await first(ctx, p); const { c, ic } = colFor(ctx, p, v, 'inherit');
    let arrow = '', spark = '';
    if (id && (p.trend_arrow !== false || p.sparkline !== false)) {
      const r = await u.seriesWin(id, Date.now() - (WG.RANGE_MS[p.spark_range] || 6 * 3600e3), Date.now(), 'auto').catch(() => ({ points: [] })); const pts = r.points;
      if (p.trend_arrow !== false && v != null && pts.length) { const t = Date.now() - 3600e3; let best = pts[0]; for (const q of pts) if (Math.abs(q.t - t) < Math.abs(best.t - t)) best = q; const dv = v - best.v; arrow = Math.abs(dv) < 1e-9 ? '<span class="muted">→</span>' : `<span title="vs 1 h ago" class="dw-trend" style="color:${dv > 0 ? '#16a34a' : '#dc2626'}">${icon(dv > 0 ? 'trending-up' : 'trending-down', 18) || (dv > 0 ? '▲' : '▼')} ${num(Math.abs(dv), d.dec)}</span>`; }
      if (p.sparkline !== false && ctx.w >= 3) spark = u.sparkline(pts, c === 'inherit' ? color(0) : c);
    }
    el.innerHTML = `<div style="display:flex;align-items:center;gap:12px;flex:1;min-height:0">${p.icon ? `<span style="color:${ic || c};opacity:.9">${icon(p.icon, 36)}</span>` : ''}<div style="min-width:0;flex:1"><div class="dw-big" style="color:${c}">${v == null ? '—' : num(v, d.dec)} <span style="font-size:.45em;font-weight:400;color:var(--muted)">${esc(unitOf(p, d))}</span></div><div class="muted" style="font-size:12px;display:flex;gap:10px;flex-wrap:wrap"><span class="notranslate dw-lbl">${esc(id ? dsLabel(d) : '')}</span>${arrow}</div><div class="muted" style="font-size:11px">${x ? fmt(x.measured_at) : 'no value yet'}</div></div>${spark}</div>`;
  }, { needsDs: true, label: 'Value card' });

  reg('agg_card', async (el, p, ctx) => {
    const all = await WG.datastreams(); const id = ids(ctx)[0]; const d = sInfo(all, id, 0, p); const win = u.winOf(p, ctx); const f = p.agg || 'avg';
    const r = await buckets(id, win.t0, win.t1); const pts = r.points; let v = null;
    if (pts.length) { const n = pts.reduce((a, q) => a + q.n, 0), sum = pts.reduce((a, q) => a + q.avg * q.n, 0);
      v = f === 'min' ? Math.min(...pts.map((q) => q.min)) : f === 'max' ? Math.max(...pts.map((q) => q.max)) : f === 'sum' ? sum : f === 'count' ? n : f === 'delta' ? pts[pts.length - 1].avg - pts[0].avg : sum / n; }
    const { c, ic } = colFor(ctx, p, v, 'inherit'); const names = { avg: WG.aggLabel('avg'), min: WG.aggLabel('min'), max: WG.aggLabel('max'), sum: WG.aggLabel('sum'), count: WG.aggLabel('count'), delta: WG.aggLabel('delta') };
    el.innerHTML = `<div class="dw-center">${p.icon ? `<span style="color:${ic || c}">${icon(p.icon, 28)}</span>` : ''}<div class="dw-big" style="color:${c}">${v == null ? '—' : num(v, f === 'count' ? 0 : d.dec)} <span style="font-size:.45em;font-weight:400;color:var(--muted)">${f === 'count' ? '' : esc(unitOf(p, d))}</span></div><div class="muted" style="font-size:12px">${esc(names[f] || f)} · ${esc(win.label)}</div><div class="muted notranslate dw-lbl" style="font-size:11px">${esc(dsLabel(d))}</div></div>`;
  }, { needsDs: true, label: 'Aggregated value' });

  reg('progress', async (el, p, ctx) => {
    const rows = await many(ctx, p); const [lo, hi] = lohi(p, 0, 100); const vert = p.orientation === 'vertical';
    el.innerHTML = `<div style="display:flex;flex-direction:${vert ? 'row' : 'column'};gap:10px;flex:1;min-height:0;justify-content:center">${rows.map((r) => { const f = r.v == null ? 0 : clamp((r.v - lo) / (hi - lo), 0, 1); const { c } = colFor(ctx, p, r.v, p.bar_color || r.c); const val = p.show_value !== false ? `<b style="font-size:var(--dw-value,13px)">${r.v == null ? '—' : num(r.v, r.d.dec)} ${esc(r.d.unit)}</b>` : '';
      return vert ? `<div style="display:flex;flex-direction:column;align-items:center;gap:4px;flex:1;min-height:0">${val}<div style="flex:1;width:22px;min-height:40px;background:var(--line);border-radius:4px;position:relative;overflow:hidden"><div style="position:absolute;bottom:0;left:0;right:0;height:${(f * 100).toFixed(1)}%;background:${c}"></div></div><span class="muted notranslate" style="font-size:11px;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(r.d.key)}</span></div>`
        : `<div><div class="dw-kv"><span class="notranslate">${esc(dsLabel(r.d))}</span>${val}</div><div style="height:12px;background:var(--line);border-radius:6px;overflow:hidden"><div style="width:${(f * 100).toFixed(1)}%;height:100%;background:${c};border-radius:6px"></div></div><div class="dw-kv muted" style="font-size:10px"><span>${lo}</span><span>${(f * 100).toFixed(0)} %</span><span>${hi}</span></div></div>`; }).join('')}</div>`;
  }, { needsDs: true, label: 'Progress bar' });

  const VESSEL = {
    tank_vertical: { vb: '0 0 100 140', clip: 'M10,14 Q10,4 50,4 Q90,4 90,14 L90,126 Q90,136 50,136 Q10,136 10,126 Z', y0: 4, y1: 136 },
    tank_horizontal: { vb: '0 0 160 100', clip: 'M20,10 L140,10 Q155,10 155,50 Q155,90 140,90 L20,90 Q5,90 5,50 Q5,10 20,10 Z', y0: 10, y1: 90 },
    silo: { vb: '0 0 100 150', clip: 'M10,20 L50,4 L90,20 L90,100 L55,140 L45,140 L10,100 Z', y0: 4, y1: 140 },
    hopper: { vb: '0 0 100 140', clip: 'M5,6 L95,6 L95,70 L60,134 L40,134 L5,70 Z', y0: 6, y1: 134 },
    basin: { vb: '0 0 160 90', clip: 'M5,8 L155,8 L145,84 L15,84 Z', y0: 8, y1: 84 },
    tank_spherical: { vb: '0 0 120 120', clip: 'M60,6 A54,54 0 1 1 59.9,6 Z', y0: 6, y1: 114 },
  };
  let clipSeq = 0;
  reg('level_card', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); const [lo, hi] = lohi(p, 0, 100); const f = v == null ? 0 : clamp((v - lo) / (hi - lo), 0, 1); const sh = VESSEL[p.shape] || VESSEL.tank_vertical; const vb = sh.vb.split(' ').map(Number);
    const { c } = colFor(ctx, p, v, p.liquid || '#3b82f6'); const y = sh.y1 - f * (sh.y1 - sh.y0); const cid = 'dwlv' + (++clipSeq);
    const vol = p.capacity ? `<div class="muted" style="font-size:12px">${num(f * p.capacity, 0)} / ${num(p.capacity, 0)} ${esc(p.capacity_unit || '')}</div>` : '';
    el.innerHTML = `<div style="display:flex;gap:12px;align-items:center;flex:1;min-height:0"><div style="flex:1;height:100%;min-width:60px">${svg(sh.vb, `<defs><clipPath id="${cid}"><path d="${sh.clip}"/></clipPath></defs><path d="${sh.clip}" fill="var(--bg2)"/><g clip-path="url(#${cid})"><rect x="0" y="${y.toFixed(1)}" width="${vb[2]}" height="${vb[3]}" fill="${c}" opacity=".85"/><path d="M0,${y.toFixed(1)} q${vb[2] / 8},-3 ${vb[2] / 4},0 t${vb[2] / 4},0 t${vb[2] / 4},0 t${vb[2] / 4},0" fill="none" stroke="#fff" stroke-opacity=".5"/></g><path d="${sh.clip}" fill="none" stroke="var(--muted)" stroke-width="2"/>`)}</div><div style="min-width:70px"><div class="dw-big">${v == null ? '—' : (f * 100).toFixed(0) + ' %'}</div><div class="muted" style="font-size:12px">${v == null ? '' : num(v, d.dec) + ' ' + esc(d.unit)}</div>${vol}</div></div>`;
  }, { needsDs: true, label: 'Liquid level' });

  reg('thermometer', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); const [lo, hi] = lohi(p, -20, 50); const f = v == null ? 0 : clamp((v - lo) / (hi - lo), 0, 1); const { c } = colFor(ctx, p, v, '#e30613');
    const top = 10, bot = 150, yv = bot - f * (bot - top); const ticks = u.niceTicks(lo, hi, 5);
    const tk = ticks.map((t) => { const yy = bot - (t - lo) / (hi - lo) * (bot - top); return `<line x1="46" x2="52" y1="${yy.toFixed(1)}" y2="${yy.toFixed(1)}" stroke="var(--muted)"/><text x="56" y="${(yy + 3.5).toFixed(1)}" font-size="10" fill="var(--muted)">${num(t, 0)}</text>`; }).join('');
    const zone = (val, col) => val == null ? '' : `<line x1="20" x2="44" y1="${(bot - clamp((val - lo) / (hi - lo), 0, 1) * (bot - top)).toFixed(1)}" y2="${(bot - clamp((val - lo) / (hi - lo), 0, 1) * (bot - top)).toFixed(1)}" stroke="${col}" stroke-width="2" stroke-dasharray="3 2"/>`;
    el.innerHTML = `<div style="display:flex;gap:10px;align-items:center;flex:1;min-height:0"><div style="height:100%;flex:1;min-width:60px">${svg('0 0 90 185', `<rect x="24" y="4" width="16" height="152" rx="8" fill="var(--bg2)" stroke="var(--muted)"/><circle cx="32" cy="164" r="16" fill="${c}" stroke="var(--muted)"/><rect x="28" y="${yv.toFixed(1)}" width="8" height="${(160 - yv).toFixed(1)}" fill="${c}"/>${tk}${zone(p.warn, p.warn_color || '#d97706')}${zone(p.action, p.action_color || '#dc2626')}${zone(p.low_warn, p.warn_color || '#d97706')}${zone(p.low_action, p.action_color || '#dc2626')}`)}</div><div><div class="dw-big" style="color:${c}">${v == null ? '—' : num(v, d.dec)}</div><div class="muted">${esc(d.unit)}</div></div></div>`;
  }, { needsDs: true, label: 'Thermometer' });

  reg('battery', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); const [lo, hi] = lohi(p, 0, 100); const f = v == null ? 0 : clamp((v - lo) / (hi - lo), 0, 1); const pct = f * 100;
    const base = pct < (p.critical ?? 15) ? '#dc2626' : pct < (p.low ?? 30) ? '#d97706' : '#16a34a'; const { c } = colFor(ctx, p, v, base);
    el.innerHTML = `<div style="flex:1;min-height:0;display:flex;align-items:center;gap:12px;flex-wrap:wrap;justify-content:center"><div style="height:100%;max-height:70px;min-height:24px;aspect-ratio:2/1">${svg('0 0 120 60', `<rect x="3" y="6" width="104" height="48" rx="7" fill="none" stroke="currentColor" stroke-width="3"/><rect x="109" y="20" width="8" height="20" rx="2" fill="currentColor"/><rect x="8" y="11" width="${(94 * f).toFixed(1)}" height="38" rx="4" fill="${c}"/>`)}</div><div><div class="dw-big" style="color:${c}">${v == null ? '—' : pct.toFixed(0) + ' %'}</div><div class="muted notranslate dw-lbl" style="font-size:11px">${esc(dsLabel(d))}</div></div></div>`;
  }, { needsDs: true, label: 'Battery level' });

  reg('signal', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); let bars = 0;
    if (v != null) bars = p.scale === 'percent' ? clamp(Math.ceil(v / 20), 0, 5) : (v >= -55 ? 5 : v >= -65 ? 4 : v >= -75 ? 3 : v >= -85 ? 2 : v >= -95 ? 1 : 0);
    const { c } = colFor(ctx, p, v, bars >= 3 ? '#16a34a' : bars === 2 ? '#d97706' : '#dc2626');
    el.innerHTML = `<div class="dw-center"><div style="width:100%;max-width:140px;flex:1;min-height:28px">${svg('0 0 100 60', [0, 1, 2, 3, 4].map((k) => `<rect x="${4 + k * 20}" y="${50 - k * 11}" width="14" height="${10 + k * 11}" rx="2" fill="${k < bars ? c : 'var(--line)'}"/>`).join(''))}</div>${p.show_value !== false ? `<div class="dw-big" style="font-size:var(--dw-value,1.4em)">${v == null ? '—' : num(v, 0)} <span class="muted" style="font-size:.6em">${esc(d.unit || (p.scale === 'percent' ? '%' : 'dBm'))}</span></div>` : ''}<div class="muted notranslate dw-lbl" style="font-size:11px">${esc(dsLabel(d))}</div></div>`;
  }, { needsDs: true, label: 'Signal strength' });

  reg('compass', async (el, p, ctx) => {
    const rows = await many(ctx, p); const dir = rows[0] ? rows[0].v : null, spd = rows[1] || null; const { c } = colFor(ctx, p, dir, '#e30613');
    const card = ['N', 'E', 'S', 'W'].map((t, k) => { const a = k * Math.PI / 2; return `<text x="${(60 + 44 * Math.sin(a)).toFixed(1)}" y="${(64 - 44 * Math.cos(a)).toFixed(1)}" text-anchor="middle" font-size="11" font-weight="700" fill="var(--fg)">${t}</text>`; }).join('');
    const ticks = Array.from({ length: 36 }, (_, k) => { const a = k * Math.PI / 18, r1 = k % 9 ? 52 : 49; return `<line x1="${(60 + r1 * Math.sin(a)).toFixed(1)}" y1="${(60 - r1 * Math.cos(a)).toFixed(1)}" x2="${(60 + 56 * Math.sin(a)).toFixed(1)}" y2="${(60 - 56 * Math.cos(a)).toFixed(1)}" stroke="var(--muted)"/>`; }).join('');
    const needle = dir == null ? '' : `<g transform="rotate(${dir.toFixed(1)} 60 60)"><path d="M60,20 L66,60 L60,54 L54,60 Z" fill="${c}"/><path d="M60,100 L66,60 L60,66 L54,60 Z" fill="var(--muted)" opacity=".5"/></g>`;
    el.innerHTML = `<div style="display:flex;gap:10px;align-items:center;flex:1;min-height:0"><div style="flex:1;height:100%;min-width:80px">${svg('0 0 120 120', `<circle cx="60" cy="60" r="57" fill="var(--bg2)" stroke="var(--line)"/>${ticks}${card}${needle}<circle cx="60" cy="60" r="3" fill="var(--fg)"/>`)}</div><div><div class="dw-big">${dir == null ? '—' : num(dir, 0) + '°'}</div>${spd ? `<div class="muted">${num(spd.v, p.decimals)} ${esc(spd.d.unit)}</div>` : ''}</div></div>`;
  }, { needsDs: true, label: 'Compass / wind' });

  reg('radial_gauge', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); const [lo, hi] = lohi(p, 0, 100); const sweep = (+p.arc || 270) * Math.PI / 180, a0 = Math.PI / 2 + (2 * Math.PI - sweep) / 2; const ang = (x) => a0 + clamp((x - lo) / (hi - lo), 0, 1) * sweep;
    const pt = (a, r) => `${(70 + r * Math.cos(a)).toFixed(2)},${(70 + r * Math.sin(a)).toFixed(2)}`; const arc = (a, b, r) => `M${pt(a, r)} A${r},${r} 0 ${b - a > Math.PI ? 1 : 0} 1 ${pt(b, r)}`;
    const n = clamp(Math.round(p.ticks || 10), 2, 20); let ticks = '';
    for (let k = 0; k <= n; k++) { const a = a0 + k / n * sweep, val = lo + k / n * (hi - lo); ticks += `<line x1="${pt(a, 52).split(',')[0]}" y1="${pt(a, 52).split(',')[1]}" x2="${pt(a, 60).split(',')[0]}" y2="${pt(a, 60).split(',')[1]}" stroke="var(--muted)" stroke-width="1.4"/><text x="${pt(a, 43).split(',')[0]}" y="${(+pt(a, 43).split(',')[1] + 3).toFixed(1)}" font-size="7" text-anchor="middle" fill="var(--muted)">${num(val, Math.abs(hi - lo) < 10 ? 1 : 0)}</text>`;
      if (k < n) for (let m = 1; m < 5; m++) { const b = a + m / 5 * sweep / n; ticks += `<line x1="${pt(b, 56).split(',')[0]}" y1="${pt(b, 56).split(',')[1]}" x2="${pt(b, 60).split(',')[0]}" y2="${pt(b, 60).split(',')[1]}" stroke="var(--muted)" stroke-width=".6"/>`; } }
    const band = (from, to, col) => (from == null) ? '' : `<path d="${arc(ang(from), ang(to), 63)}" fill="none" stroke="${col}" stroke-width="5"/>`;
    const { c } = colFor(ctx, p, v, 'var(--fg)'); const na = v == null ? a0 : ang(v);
    el.innerHTML = `<div class="dw-center"><div style="width:100%;flex:1;min-height:60px">${svg('0 0 140 130', `<path d="${arc(a0, a0 + sweep, 63)}" fill="none" stroke="var(--line)" stroke-width="5"/>${band(p.warn, p.action ?? hi, p.warn_color || '#d97706')}${band(p.action, hi, p.action_color || '#dc2626')}${ticks}<line x1="70" y1="70" x2="${pt(na, 50).split(',')[0]}" y2="${pt(na, 50).split(',')[1]}" stroke="${v == null ? 'var(--line)' : '#e30613'}" stroke-width="2.5" stroke-linecap="round"/><circle cx="70" cy="70" r="5" fill="var(--fg)"/><text x="70" y="100" text-anchor="middle" font-size="15" font-weight="700" fill="${c}">${v == null ? '—' : num(v, d.dec)}</text><text x="70" y="112" text-anchor="middle" font-size="8" fill="var(--muted)">${esc(d.unit)}</text>`)}</div><div class="dw-gauge-lbl notranslate dw-lbl">${esc(dsLabel(d))}</div></div>`;
  }, { needsDs: true, label: 'Radial gauge' });

  reg('linear_gauge', async (el, p, ctx) => {
    const { d, v } = await first(ctx, p); const [lo, hi] = lohi(p, 0, 100); const vert = p.orientation === 'vertical'; const f = (x) => clamp((x - lo) / (hi - lo), 0, 1); const { c } = colFor(ctx, p, v, '#e30613');
    const L = vert ? 200 : 420, ticks = u.niceTicks(lo, hi, 5);
    const zone = (a, b, col) => a == null ? '' : (vert ? `<rect x="22" y="${(10 + (1 - f(b)) * L).toFixed(1)}" width="12" height="${((f(b) - f(a)) * L).toFixed(1)}" fill="${col}" opacity=".7"/>` : `<rect x="${(10 + f(a) * L).toFixed(1)}" y="22" width="${((f(b) - f(a)) * L).toFixed(1)}" height="12" fill="${col}" opacity=".7"/>`);
    const body = vert
      ? `<rect x="22" y="10" width="12" height="${L}" fill="var(--line)"/>${zone(p.warn, p.action ?? hi, p.warn_color || '#d97706')}${zone(p.action, hi, p.action_color || '#dc2626')}${ticks.map((t) => `<line x1="36" x2="42" y1="${(10 + (1 - f(t)) * L).toFixed(1)}" y2="${(10 + (1 - f(t)) * L).toFixed(1)}" stroke="var(--muted)"/><text x="45" y="${(13 + (1 - f(t)) * L).toFixed(1)}" font-size="9" fill="var(--muted)">${num(t, 0)}</text>`).join('')}${v == null ? '' : `<path d="M8,${(10 + (1 - f(v)) * L).toFixed(1)} l12,-6 v12 z" fill="${c}"/>`}`
      : `<rect x="10" y="22" width="${L}" height="12" fill="var(--line)"/>${zone(p.warn, p.action ?? hi, p.warn_color || '#d97706')}${zone(p.action, hi, p.action_color || '#dc2626')}${ticks.map((t) => `<line y1="36" y2="42" x1="${(10 + f(t) * L).toFixed(1)}" x2="${(10 + f(t) * L).toFixed(1)}" stroke="var(--muted)"/><text y="52" x="${(10 + f(t) * L).toFixed(1)}" font-size="9" text-anchor="middle" fill="var(--muted)">${num(t, 0)}</text>`).join('')}${v == null ? '' : `<path d="M${(10 + f(v) * L).toFixed(1)},20 l-6,-12 h12 z" fill="${c}"/>`}`;
    el.innerHTML = `<div style="display:flex;flex-direction:row;gap:8px;flex:1;min-height:0;align-items:center"><div style="${vert ? 'height:100%;width:80px' : 'flex:1;height:100%;min-height:24px;min-width:0'}">${svg(vert ? '0 0 70 220' : '0 0 440 56', body)}</div><div style="text-align:center"><b class="dw-big" style="font-size:var(--dw-value,1.4em);color:${c}">${v == null ? '—' : num(v, d.dec)}</b> <span class="muted">${esc(d.unit)}</span></div></div>`;
  }, { needsDs: true, label: 'Linear gauge' });

  reg('led_grid', async (el, p, ctx) => {
    const rows = await many(ctx, p); const cols = clamp(Math.round(p.columns || 4), 1, 12); const onV = p.on_value ?? 1;
    el.innerHTML = `<div style="display:grid;grid-template-columns:repeat(${cols},minmax(0,1fr));gap:8px;align-content:center;flex:1">${rows.map((r) => { const on = r.v != null && r.v === onV; const col = on ? (p.on_color || '#16a34a') : (p.off_color || '#cbd5e1');
      return `<div style="display:flex;flex-direction:column;align-items:center;gap:3px;min-width:0" title="${esc(dsLabel(r.d))}: ${r.v == null ? '—' : esc(String(r.v))}"><span style="width:18px;height:18px;border-radius:50%;background:${col};box-shadow:${on ? `0 0 8px ${col}` : 'inset 0 1px 2px rgba(0,0,0,.25)'}"></span><span class="muted notranslate" style="font-size:10px;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(r.d.key)}</span></div>`; }).join('')}</div>`;
  }, { needsDs: true, label: 'LED indicators' });

  reg('alarm_count', async (el, p, ctx) => {
    const al = WG.inScope(await api('GET', P('/api/v1/alarms')), ctx); const act = al.filter((a) => !a.cleared_at && (!p.severity || a.severity === p.severity)); const n = act.length;
    const base = n === 0 ? '#16a34a' : act.some((a) => a.severity === 'action') ? '#dc2626' : '#d97706'; const { c } = colFor(ctx, p, n, base);
    el.innerHTML = `<div class="dw-center"><span style="color:${c}">${icon(p.icon || 'bell', 30)}</span><div class="dw-big" style="color:${c}">${n}</div><div class="muted" style="font-size:12px">${p.severity ? esc(p.severity) + ' · ' : ''}active alarms</div></div>`;
  }, { alarms: true, label: 'Alarm count' });

  // ---- charts ---------------------------------------------------------------------------------------------------
  reg('state_timeline', async (el, p, ctx) => {
    const all = await WG.datastreams(); const win = u.winOf(p, ctx); const states = parseStates(p.states); const span = win.t1 - win.t0;
    const rows = await Promise.all(ids(ctx).map(async (id, i) => ({ d: sInfo(all, id, i, p), pts: (await u.seriesWin(id, win.t0, win.t1, 'none').catch(() => ({ points: [] }))).points })));
    const pct = (t) => ((t - win.t0) / span * 100);
    const band = (r) => r.pts.map((q, i) => { const t1 = i + 1 < r.pts.length ? r.pts[i + 1].t : win.t1; const s0 = stateOf(states, q.v); const x0 = pct(Math.max(q.t, win.t0)), x1 = pct(t1); if (x1 <= x0) return ''; return `<rect x="${x0.toFixed(3)}" y="0" width="${(x1 - x0).toFixed(3)}" height="1" fill="${s0 ? s0.color : '#6b7280'}"><title>${esc(s0 ? s0.label : String(q.v))} · ${esc(new Date(q.t).toLocaleString(document.documentElement.lang || undefined))}</title></rect>`; }).join('');
    const xt = u.timeTicks(win.t0, win.t1, 6);
    el.innerHTML = `<div class="muted" style="font-size:12px;display:flex;gap:12px;flex-wrap:wrap">${states.map((s0) => `<span><i style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${s0.color}"></i> ${esc(s0.label)}</span>`).join('')}<span class="grow"></span><span>${esc(win.label)}</span></div>
      <div style="flex:1;min-height:0;display:flex;flex-direction:column;justify-content:center;gap:6px">${rows.map((r) => `<div style="display:flex;align-items:center;gap:8px"><span class="notranslate" style="width:110px;flex:none;text-align:right;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(dsLabel(r.d))}">${esc(r.d.label || r.d.key)}</span><svg viewBox="0 0 100 1" preserveAspectRatio="none" style="flex:1;height:22px;background:var(--bg2);border-radius:3px;display:block">${band(r)}</svg></div>`).join('')}
      <div style="position:relative;height:14px;margin-left:118px">${xt.ticks.map((t) => `<span class="muted" style="position:absolute;left:${pct(t).toFixed(2)}%;transform:translateX(-50%);font-size:11px;white-space:nowrap">${xt.fmt(t)}</span>`).join('')}</div></div>`;
  }, { needsDs: true, label: 'State timeline' });

  reg('bar_time', async (el, p, ctx) => {
    const all = await WG.datastreams(); const win = u.winOf(p, ctx); const cfg = Array.isArray(p.series) ? p.series : [];
    const want = { '1m': 60, '5m': 300, '15m': 900, '1h': 3600, '6h': 21600, '1d': 86400 }[p.agg_interval] || Math.max(60, Math.ceil((win.t1 - win.t0) / 1000 / 40 / 60) * 60);
    const f = p.agg && p.agg !== 'auto' && p.agg !== 'none' ? p.agg : 'avg'; const pick = (q) => f === 'min' ? q.min : f === 'max' ? q.max : f === 'sum' ? q.avg * q.n : f === 'count' ? q.n : q.avg;
    const list = await Promise.all(ids(ctx).map(async (id, i) => { const d = dsInfo(all, id), c = cfg[i] || {}; const r = await buckets(id, win.t0, win.t1, want); return { label: c.label || dsLabel(d), unit: c.unit || d.unit, color: c.color || color(i), hidden: !!c.hidden, bucket: r.bucket, map: new Map(r.points.map((q) => [q.t, pick(q)])) }; }));
    const vis = list.filter((s) => !s.hidden); if (!vis.length || !vis.some((s) => s.map.size)) { noData(el, 'no data in this range'); return; }
    const times = [...new Set(vis.flatMap((s) => [...s.map.keys()]))].sort((a, b) => a - b); const stacked = !!p.stacked;
    const tot = times.map((t) => stacked ? vis.reduce((a, s) => a + Math.max(0, s.map.get(t) || 0), 0) : Math.max(...vis.map((s) => s.map.get(t) || 0))); const mn = stacked ? 0 : Math.min(0, ...vis.flatMap((s) => [...s.map.values()]));
    const hi = Math.max(...tot, 1e-9) * 1.08, lo = mn * 1.08; const Wd = 960, H = Math.max(160, ctx.px_h - 70), L = 50, R = 10, T = 10, B = 26; const py = (v) => T + (1 - (v - lo) / ((hi - lo) || 1)) * (H - T - B);
    const slot = (Wd - L - R) / times.length, bw = stacked ? slot * 0.75 : slot * 0.8 / vis.length; const yt = u.niceTicks(lo, hi, 5);
    let g = yt.map((v) => `<line x1="${L}" x2="${Wd - R}" y1="${py(v).toFixed(1)}" y2="${py(v).toFixed(1)}" stroke="var(--line)"/><text x="${L - 5}" y="${(py(v) + 4).toFixed(1)}" font-size="11" text-anchor="end" fill="var(--muted)">${num(v, p.decimals)}</text>`).join('');
    times.forEach((t, k) => { let acc = 0; vis.forEach((s, i) => { const v = s.map.get(t); if (v == null) return; const x = L + k * slot + (stacked ? slot * 0.125 : slot * 0.1 + i * bw); if (stacked) { const y1 = py(acc + Math.max(0, v)), y0 = py(acc); acc += Math.max(0, v); g += `<rect x="${x.toFixed(1)}" y="${y1.toFixed(1)}" width="${bw.toFixed(1)}" height="${Math.max(0.5, y0 - y1).toFixed(1)}" fill="${s.color}"><title>${esc(s.label)} ${num(v, p.decimals)}</title></rect>`; } else { const y = py(v), y0 = py(0); g += `<rect x="${x.toFixed(1)}" y="${Math.min(y, y0).toFixed(1)}" width="${bw.toFixed(1)}" height="${Math.max(0.5, Math.abs(y0 - y)).toFixed(1)}" fill="${s.color}"><title>${esc(s.label)} ${num(v, p.decimals)}</title></rect>`; } }); });
    const xt = u.timeTicks(times[0], times[times.length - 1] + 1, 8); const px = (t) => L + (times.indexOf(t) + 0.5) * slot; g += times.filter((t, k) => k % Math.max(1, Math.ceil(times.length / 10)) === 0).map((t) => `<text x="${px(t).toFixed(1)}" y="${H - 8}" font-size="11" text-anchor="middle" fill="var(--muted)">${xt.fmt(t)}</text>`).join('');
    el.innerHTML = `<div class="muted" style="font-size:12px;display:flex;gap:12px;flex-wrap:wrap">${vis.map((s) => `<span><i style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${s.color}"></i> ${esc(s.label)}</span>`).join('')}<span class="grow"></span><span>${esc(win.label)} · ${esc(WG.aggLabel(f))}/${list[0].bucket >= 3600 ? list[0].bucket / 3600 + ' h' : list[0].bucket / 60 + ' min'}</span></div><div style="flex:1;min-height:0"><svg viewBox="0 0 ${Wd} ${H}" preserveAspectRatio="none" style="width:100%;height:100%;display:block">${g}</svg></div>`;
  }, { needsDs: true, label: 'Bars over time' });

  const legendList = (rows, p, sum) => p.legend_pos === 'none' ? '' : `<div style="font-size:12px;min-width:0">${rows.map((r) => `<div style="display:flex;align-items:center;gap:6px;white-space:nowrap"><i style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${r.c};flex:none"></i><span class="notranslate" style="overflow:hidden;text-overflow:ellipsis">${esc(dsLabel(r.d))}</span> <b>${num(r.v, r.d.dec)}</b>${sum ? ` <span class="muted">${((r.v || 0) / sum * 100).toFixed(0)}%</span>` : ''}</div>`).join('')}</div>`;
  reg('pie', async (el, p, ctx) => {
    const rows = (await many(ctx, p)).map((r) => ({ ...r, v: Math.max(0, r.v || 0) })); const sum = rows.reduce((a, r) => a + r.v, 0) || 1; let a = -Math.PI / 2; const R = 50, ri = p.doughnut === false ? 0 : 28;
    const seg = rows.map((r) => { const f = r.v / sum; if (f <= 0) return ''; if (f >= 0.9999) return `<circle cx="60" cy="60" r="${(R + ri) / 2}" fill="none" stroke="${r.c}" stroke-width="${R - ri}"/>`; const a1 = a + f * 2 * Math.PI, big = f > 0.5 ? 1 : 0; const P1 = (ang, rr) => `${(60 + rr * Math.cos(ang)).toFixed(2)},${(60 + rr * Math.sin(ang)).toFixed(2)}`;
      const d = ri ? `M${P1(a, R)} A${R},${R} 0 ${big} 1 ${P1(a1, R)} L${P1(a1, ri)} A${ri},${ri} 0 ${big} 0 ${P1(a, ri)} Z` : `M60,60 L${P1(a, R)} A${R},${R} 0 ${big} 1 ${P1(a1, R)} Z`; const mid = (a + a1) / 2; a = a1;
      return `<path d="${d}" fill="${r.c}" stroke="var(--card)" stroke-width="1"><title>${esc(dsLabel(r.d))}: ${num(r.v, r.d.dec)}</title></path>${p.show_percent !== false && f > 0.06 ? `<text x="${P1(mid, (R + ri) / 2).split(',')[0]}" y="${(+P1(mid, (R + ri) / 2).split(',')[1] + 3).toFixed(1)}" font-size="8" text-anchor="middle" fill="#fff">${(f * 100).toFixed(0)}%</text>` : ''}`; }).join('');
    el.innerHTML = `<div style="display:flex;gap:14px;align-items:center;flex:1;min-height:0;flex-wrap:wrap;flex-direction:${p.legend_pos === 'bottom' ? 'column' : 'row'}"><div style="height:100%;min-height:90px;flex:1;min-width:90px;max-width:260px">${svg('0 0 120 120', seg)}</div>${legendList(rows, p, sum)}</div>`;
  }, { needsDs: true, label: 'Pie / doughnut' });

  reg('polar', async (el, p, ctx) => {
    const rows = await many(ctx, p); const mx = Math.max(1e-9, ...rows.map((r) => Math.abs(r.v || 0))); const n = rows.length || 1; const step = 2 * Math.PI / n;
    const P1 = (ang, rr) => `${(60 + rr * Math.cos(ang)).toFixed(2)},${(60 + rr * Math.sin(ang)).toFixed(2)}`;
    const seg = rows.map((r, k) => { const rr = Math.abs(r.v || 0) / mx * 52, a0 = -Math.PI / 2 + k * step, a1 = a0 + step; return rr <= 0 ? '' : `<path d="M60,60 L${P1(a0, rr)} A${rr},${rr} 0 ${step > Math.PI ? 1 : 0} 1 ${P1(a1, rr)} Z" fill="${r.c}" fill-opacity=".7" stroke="${r.c}"><title>${esc(dsLabel(r.d))}: ${num(r.v, r.d.dec)}</title></path>`; }).join('');
    el.innerHTML = `<div style="display:flex;gap:14px;align-items:center;flex:1;min-height:0;flex-wrap:wrap;flex-direction:${p.legend_pos === 'bottom' ? 'column' : 'row'}"><div style="height:100%;min-height:90px;flex:1;min-width:90px;max-width:260px">${svg('0 0 120 120', `${[13, 26, 39, 52].map((r) => `<circle cx="60" cy="60" r="${r}" fill="none" stroke="var(--line)"/>`).join('')}${seg}`)}</div>${legendList(rows, p)}</div>`;
  }, { needsDs: true, label: 'Polar area' });

  reg('radar', async (el, p, ctx) => {
    const rows = await many(ctx, p); const n = rows.length; if (n < 3) { noData(el, 'bind at least 3 datastreams'); return; } const mx = p.y_max || Math.max(1e-9, ...rows.map((r) => r.v || 0)) * 1.1;
    const P1 = (k, f) => { const a = -Math.PI / 2 + k * 2 * Math.PI / n; return [(80 + f * 55 * Math.cos(a)).toFixed(1), (72 + f * 55 * Math.sin(a)).toFixed(1)]; };
    const grid = [0.25, 0.5, 0.75, 1].map((f) => `<polygon points="${rows.map((_, k) => P1(k, f).join(',')).join(' ')}" fill="none" stroke="var(--line)"/>`).join('') + rows.map((_, k) => `<line x1="80" y1="72" x2="${P1(k, 1)[0]}" y2="${P1(k, 1)[1]}" stroke="var(--line)"/>`).join('');
    const poly = rows.map((r, k) => P1(k, clamp((r.v || 0) / mx, 0, 1)).join(',')).join(' ');
    const lbl = rows.map((r, k) => { const [x, y] = P1(k, 1.18); return `<text x="${x}" y="${y}" font-size="8" text-anchor="middle" fill="var(--muted)">${esc(r.d.key)} ${num(r.v, r.d.dec)}</text>`; }).join('');
    el.innerHTML = `<div style="flex:1;min-height:0">${svg('0 0 160 150', `${grid}<polygon points="${poly}" fill="${color(0)}" fill-opacity=".25" stroke="${color(0)}" stroke-width="1.5"/>${lbl}`)}</div>`;
  }, { needsDs: true, label: 'Radar' });

  // ---- lists ----------------------------------------------------------------------------------------------------
  reg('ts_table', async (el, p, ctx) => {
    const all = await WG.datastreams(); const list = ids(ctx); const win = u.winOf(p, ctx); const lim = clamp(Math.round(p.limit || 50), 1, 1000);
    const cols = (await Promise.all(list.map(async (id, i) => { const r = await u.seriesWin(id, win.t0, win.t1, p.agg || 'none', p.agg_interval).catch(() => ({ points: [] })); return { d: sInfo(all, id, i, p), m: new Map(r.points.map((q) => [Math.round(q.t / 1000) * 1000, q.v])) }; }))).filter((c) => !c.d.hidden);
    const ts = [...new Set(cols.flatMap((c) => [...c.m.keys()]))].sort((a, b) => b - a).slice(0, lim);
    el.innerHTML = `<div class="table-wrap"><table><thead><tr><th>Time</th>${cols.map((c) => `<th style="text-align:right" class="notranslate">${esc(c.d.label || c.d.key)}${c.d.unit ? ' [' + esc(c.d.unit) + ']' : ''}</th>`).join('')}</tr></thead><tbody>${ts.map((t) => `<tr><td>${esc(new Date(t).toLocaleString(document.documentElement.lang || undefined))}</td>${cols.map((c) => { const v = c.m.get(t); const cf = WG.condFormat(null, p, v); return `<td style="text-align:right${cf.value ? ';color:' + cf.value : ''}">${v == null ? '' : num(v, c.d.dec)}</td>`; }).join('')}</tr>`).join('') || `<tr><td colspan="${cols.length + 1}" class="muted">no data in this range</td></tr>`}</tbody></table></div>`;
  }, { needsDs: true, label: 'Time-series table' });

  // Period table (spec 023): a row per local day/week/month… (or per datastream), values min/max/avg/first/last/
  // consumption/count/out computed by the server in the organization's time zone; ◀ ▶ move the period; the export
  // buttons download the same table as Excel, CSV or a PDF report (data.export, not on public links).
  const PT_VALUE = { min: 'min', max: 'max', avg: 'average', first: 'start', last: 'end', delta: 'consumption', sum: 'sum', count: 'samples', out: 'out of limits' };
  const tr = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
  function ptQuery(ctx, p, all, shift) {
    const q = new URLSearchParams(); const list = ids(ctx);
    q.set('datastreams', list.join(',')); q.set('interval', p.interval || 'day'); q.set('values', p.values || 'min,max'); q.set('period', p.period || 'this_month');
    q.set('orientation', p.orientation || 'periods'); if (p.total) q.set('total', '1'); if (shift) q.set('shift', String(shift));
    if (p.low != null && p.low !== '') q.set('low', String(p.low)); if (p.high != null && p.high !== '') q.set('high', String(p.high));
    if (p.decimals != null && p.decimals !== '') q.set('decimals', String(p.decimals));
    list.forEach((id, i) => { const d = sInfo(all, id, i, p); q.append('label', d.label || ''); });
    return q;
  }
  function ptPeriodLabel(pr, interval) {
    const d = new Date(pr.start), lang = document.documentElement.lang || undefined;
    try {
      if (interval === 'day') return d.toLocaleDateString(lang, { weekday: 'short', day: 'numeric', month: 'numeric', year: 'numeric' });
      if (interval === 'month') return d.toLocaleDateString(lang, { month: 'long', year: 'numeric' });
      if (interval === 'hour') return d.toLocaleString(lang, { day: 'numeric', month: 'numeric', hour: '2-digit', minute: '2-digit' });
    } catch (e) { /* old browser */ }
    return pr.label;
  }
  const ptRender = async (el, p, ctx) => {
    const all = await WG.datastreams(); const shift = el._shift || 0; const q = ptQuery(ctx, p, all, shift);
    let t; try { t = await api('GET', P('/api/v1/period-table?' + q.toString())); } catch (x) { noData(el, x.message); return; }
    const vals = t.values; const dec = (v, c) => (v === 'count' || v === 'out') ? 0 : (p.decimals ?? sInfo(all, c.datastream_id, 0, {}).dec ?? 2);
    const out = (c, cell) => cell && cell.count && ((p.low != null && p.low !== '' && cell.min < p.low) || (p.high != null && p.high !== '' && cell.max > p.high));
    const td = (c, cell, v, bold) => { const x = cell && cell.count ? (v === 'count' ? cell.count : v === 'out' ? cell.out : cell[v]) : null;
      const bad = (v === 'min' && cell && cell.count && p.low != null && p.low !== '' && cell.min < p.low) || (v === 'max' && cell && cell.count && p.high != null && p.high !== '' && cell.max > p.high) || (v === 'out' && x > 0) || ((v === 'avg' || v === 'last' || v === 'first') && out(c, cell));
      return `<td style="text-align:right;font-variant-numeric:tabular-nums${bold ? ';font-weight:700' : ''}${bad ? ';color:var(--bad,#dc2626);font-weight:700' : ''}"${cell && cell.flagged ? ` title="${esc(tr('contains samples of doubtful quality'))}"` : ''}>${x == null ? '' : num(x, dec(v, c))}${cell && cell.flagged ? ' <span class="muted">*</span>' : ''}</td>`; };
    const vname = (v) => esc(tr(PT_VALUE[v] || v)); const multi = vals.length > 1;
    const unitOfCol = (c, v) => (v === 'count' || v === 'out' || !c.unit) ? '' : ` [${esc(c.unit)}]`;
    let head, body;
    if ((p.orientation || 'periods') === 'datastreams') {
      head = `<tr><th>${esc(tr('Datastream'))}</th>${t.periods.map((pr) => `<th colspan="${vals.length}" style="text-align:center">${esc(ptPeriodLabel(pr, t.interval))}</th>`).join('')}${p.total ? `<th colspan="${vals.length}" style="text-align:center">${esc(tr('Total'))}</th>` : ''}</tr>`;
      if (multi) head += `<tr><th></th>${t.periods.map(() => vals.map((v) => `<th style="text-align:right">${vname(v)}</th>`).join('')).join('')}${p.total ? vals.map((v) => `<th style="text-align:right">${vname(v)}</th>`).join('') : ''}</tr>`;
      body = t.columns.map((c) => `<tr><td class="notranslate">${esc(c.label)}${c.unit ? ` <span class="muted">[${esc(c.unit)}]</span>` : ''}</td>${t.periods.map((_, i) => vals.map((v) => td(c, t.cells[c.datastream_id][i], v)).join('')).join('')}${p.total ? vals.map((v) => td(c, t.totals[c.datastream_id], v, true)).join('') : ''}</tr>`).join('');
    } else {
      head = `<tr><th rowspan="${multi ? 2 : 1}">${esc(tr('Period'))}</th>${t.columns.map((c) => `<th colspan="${vals.length}" style="text-align:${multi ? 'center' : 'right'}" class="notranslate">${esc(c.label)}${multi ? '' : unitOfCol(c, vals[0]) + ' ' + `<span class="muted" style="font-weight:400">${vname(vals[0])}</span>`}</th>`).join('')}</tr>`;
      if (multi) head += `<tr>${t.columns.map((c) => vals.map((v) => `<th style="text-align:right">${vname(v)}${unitOfCol(c, v)}</th>`).join('')).join('')}</tr>`;
      body = t.periods.map((pr, i) => `<tr><td style="white-space:nowrap">${esc(ptPeriodLabel(pr, t.interval))}</td>${t.columns.map((c) => vals.map((v) => td(c, t.cells[c.datastream_id][i], v)).join('')).join('')}</tr>`).join('');
      if (p.total) body += `<tr><td style="font-weight:700">${esc(tr('Total'))}</td>${t.columns.map((c) => vals.map((v) => td(c, t.totals[c.datastream_id], v, true)).join('')).join('')}</tr>`;
    }
    const pub = WG.isPublic();
    const canExport = !pub && p.export !== false && !(ctx && ctx.edit) && (!window.ctrl32 || !ctrl32.can || ctrl32.can('data.export'));
    const xq = new URLSearchParams(q); xq.set('title', p.title || tr('Period table'));
    const btn = (f, lbl) => { xq.set('format', f); return `<a class="btn small ghost" href="/api/v1/period-table/export?${esc(xq.toString())}" download>${lbl}</a>`; };
    el.innerHTML = `<div class="pt-bar" style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:4px"><button class="btn small ghost pt-prev" type="button" title="${esc(tr('Previous'))}">◀</button><b class="notranslate" style="font-variant-numeric:tabular-nums">${esc(t.label)}</b><button class="btn small ghost pt-next" type="button" title="${esc(tr('Next'))}"${shift >= 0 ? ' disabled' : ''}>▶</button>${shift ? `<button class="btn small ghost pt-now" type="button">${esc(tr('Current'))}</button>` : ''}<span class="grow" style="flex:1"></span>${canExport ? btn('xlsx', 'Excel') + btn('csv', 'CSV') + btn('pdf', 'PDF') : ''}</div><div class="table-wrap" style="flex:1;min-height:0;overflow:auto"><table class="pt-table"><thead>${head}</thead><tbody>${body || `<tr><td class="muted">${esc(tr('no data in this range'))}</td></tr>`}</tbody></table></div>`;
    for (const [cls, d] of [['.pt-prev', -1], ['.pt-next', 1], ['.pt-now', 0]]) { const b = el.querySelector(cls); if (b) b.onclick = async (e) => { e.stopPropagation(); el._shift = d === 0 ? 0 : Math.min(0, (el._shift || 0) + d); await ptRender(el, p, ctx); }; }
    for (const a of el.querySelectorAll('.pt-bar a')) a.onclick = (e) => e.stopPropagation();
  };
  reg('period_table', ptRender, { needsDs: true, label: 'Period table' });

  reg('entity_table', async (el, p, ctx) => {
    const rows = await many(ctx, p); let sortK = el._sortK || 'name', dir = el._dir || 1, q = el._q || '';
    const draw = () => {
      const key = (r) => sortK === 'value' ? (r.v ?? -Infinity) : sortK === 'time' ? (r.x ? Date.parse(r.x.measured_at) : 0) : dsLabel(r.d).toLowerCase();
      const vis = rows.filter((r) => !q || dsLabel(r.d).toLowerCase().includes(q.toLowerCase())).sort((a, b) => (key(a) > key(b) ? 1 : key(a) < key(b) ? -1 : 0) * dir);
      const th = (k, t, al) => `<th data-k="${k}" style="cursor:pointer${al ? ';text-align:right' : ''}">${t}${sortK === k ? (dir > 0 ? ' ▲' : ' ▼') : ''}</th>`;
      el.innerHTML = `${p.search !== false ? `<input class="dw-search" type="search" placeholder="Search…" value="${esc(q)}" style="margin-bottom:4px">` : ''}<div class="table-wrap"><table><thead><tr>${th('name', 'Datastream')}${th('value', 'Value', 1)}<th>Unit</th>${p.show_time !== false ? th('time', 'Measured') : ''}${p.show_quality !== false ? '<th>Quality</th>' : ''}</tr></thead><tbody>${vis.map((r) => { const cf = WG.condFormat(null, p, r.v); const c = cf.value || lvlColor(level(r.v, p), p.value_color || 'inherit', p); return `<tr><td class="notranslate">${esc(dsLabel(r.d))}</td><td style="text-align:right;font-weight:600;color:${c}">${r.v == null ? '—' : num(r.v, r.d.dec)}</td><td>${esc(r.d.unit)}</td>${p.show_time !== false ? `<td>${r.x ? fmt(r.x.measured_at) : '—'}</td>` : ''}${p.show_quality !== false ? `<td>${r.x ? `<span class="badge ${r.x.quality === 'ok' ? 'ok' : 'warn'}">${esc(r.x.quality)}</span>` : ''}</td>` : ''}</tr>`; }).join('') || '<tr><td colspan="5" class="muted">nothing found</td></tr>'}</tbody></table></div>`;
      for (const h of el.querySelectorAll('th[data-k]')) h.onclick = (e) => { e.stopPropagation(); if (sortK === h.dataset.k) dir = -dir; else { sortK = h.dataset.k; dir = 1; } el._sortK = sortK; el._dir = dir; draw(); };
      const s = el.querySelector('.dw-search'); if (s) s.oninput = () => { q = s.value; el._q = q; const pos = s.selectionStart; draw(); const s2 = el.querySelector('.dw-search'); s2.focus(); s2.setSelectionRange(pos, pos); };
    };
    draw();
  }, { needsDs: true, label: 'Entity table' });

  // Overview table (0.68.0): many datastreams, usually selected by a query and following the dashboard filters, with
  // search, sorting, column filters (value range, quality, stale/fresh, alarm), grouping, pages, highlighting and the
  // export of the filtered rows (Excel, CSV; data.export, never on a public link). The server answers one query with
  // the latest values, active alarms, the limits of the alarm rules and minimum/maximum/change in the range.
  const OT_LBL = { site: 'Site', asset: 'Asset', asset_type: 'Asset type', device: 'Device', datastream: 'Datastream', quantity: 'Quantity', value: 'Value', measured: 'Measured', age: 'Age', quality: 'Quality', alarm: 'Alarm', limits: 'Limits', min: 'Minimum', max: 'Maximum', change: 'Change' };
  const OT_NUM = new Set(['value', 'age', 'min', 'max', 'change']);
  const STALE = { '15m': 900e3, '1h': 3600e3, '6h': 6 * 3600e3, '24h': 86400e3 };
  const SEV_RANK = { warning: 1, action: 2, critical: 3 };
  const ageOf = (r) => (r.latest ? Date.now() - Date.parse(r.latest.measured_at) : null);
  const ageTxt = (ms) => { if (ms == null) return '—'; const s = ms / 1000; return s < 60 ? Math.round(s) + ' s' : s < 3600 ? Math.round(s / 60) + ' min' : s < 172800 ? Math.round(s / 3600) + ' h' : Math.round(s / 86400) + ' d'; };
  const staleOf = (r, p) => { const a = ageOf(r); if (a == null) return true; const lim = STALE[p.stale_after] || (r.interval_s ? Math.max(2 * r.interval_s * 1000, 120e3) : 3600e3); return a > lim; };
  const rowVal = (r) => (r.latest ? numVal(r.latest) : null);
  const outOfLimits = (r, v) => { const l = r.limits || {}; if (v == null) return ''; if ((l.high_action != null && v >= l.high_action) || (l.low_action != null && v <= l.low_action)) return 'bad'; if ((l.high_warn != null && v >= l.high_warn) || (l.low_warn != null && v <= l.low_warn)) return 'warn'; return ''; };
  const limTxt = (l) => { if (!l) return '—'; const lo = l.low_action ?? l.low_warn, hi = l.high_action ?? l.high_warn; return lo == null && hi == null ? '—' : (lo != null && hi != null ? `${lo} … ${hi}` : lo != null ? `≥ ${lo}` : `≤ ${hi}`); };
  function otKey(r, k) {
    switch (k) {
      case 'value': return rowVal(r); case 'age': return ageOf(r); case 'measured': return r.latest ? Date.parse(r.latest.measured_at) : null;
      case 'quality': return r.latest ? r.latest.quality : ''; case 'alarm': return r.alarm ? SEV_RANK[r.alarm.severity] || 1 : 0;
      case 'limits': return limTxt(r.limits); case 'min': case 'max': case 'change': return r.window ? r.window[k] : null;
      case 'datastream': return (r.label || r.key || '').toLowerCase(); default: return String(r[k] || '').toLowerCase();
    }
  }
  const otGroup = (r, g) => (g === 'device' ? r.device || '—' : r[g] || '—');
  reg('overview_table', async (el, p, ctx) => {
    // the column a table is grouped by is shown in the group rows instead
    const cols = (Array.isArray(p.columns) && p.columns.length ? p.columns : ['site', 'asset', 'datastream', 'value', 'age', 'quality', 'alarm']).filter((c) => OT_LBL[c] && c !== p.group_by);
    const win = u.winOf(p, ctx); const needWin = cols.some((c) => c === 'min' || c === 'max' || c === 'change');
    const extra = { latest: '1' }; if (needWin) { extra.from = new Date(win.t0).toISOString(); extra.to = new Date(win.t1).toISOString(); }
    let res; try { res = await ctx.query(extra); } catch (x) { noData(el, x.message); return; }
    const rows = res.items; const st = el._ot || (el._ot = { q: '', sort: '', dir: 1, page: 0, vmin: '', vmax: '', quality: '', fresh: '', alarm: '', open: false });
    const dec = (r) => (p.decimals != null && p.decimals !== '' ? p.decimals : 2); const hl = p.highlight || 'both';
    const pageSize = clamp(Math.round(p.page_size || 25), 5, 200);
    const filtered = () => rows.filter((r) => {
      if (st.q) { const t = st.q.toLowerCase(); if (![r.site, r.asset, r.asset_type, r.device, r.key, r.label, r.quantity].some((x) => x && String(x).toLowerCase().includes(t))) return false; }
      const v = rowVal(r);
      if (st.vmin !== '' && !(v != null && v >= +st.vmin)) return false;
      if (st.vmax !== '' && !(v != null && v <= +st.vmax)) return false;
      if (st.quality === 'ok' && !(r.latest && (r.latest.quality === 'ok' || r.latest.quality === 'backfilled'))) return false;
      if (st.quality === 'doubtful' && !(r.latest && r.latest.quality !== 'ok' && r.latest.quality !== 'backfilled')) return false;
      if (st.fresh === 'fresh' && staleOf(r, p)) return false;
      if (st.fresh === 'stale' && !staleOf(r, p)) return false;
      if (st.alarm === 'alarm' && !r.alarm) return false;
      if (st.alarm === 'none' && r.alarm) return false;
      return true;
    });
    const sorted = (list) => { if (!st.sort && !p.group_by) return list; const g = p.group_by; return list.slice().sort((a, b) => { if (g) { const ga = otGroup(a, g).toLowerCase(), gb = otGroup(b, g).toLowerCase(); if (ga !== gb) return ga > gb ? 1 : -1; } if (!st.sort) return 0; const ka = otKey(a, st.sort), kb = otKey(b, st.sort); if (ka == null && kb == null) return 0; if (ka == null) return 1; if (kb == null) return -1; return (ka > kb ? 1 : ka < kb ? -1 : 0) * st.dir; }); };
    const cell = (r, c) => {
      const v = rowVal(r);
      switch (c) {
        case 'value': { const lim = hl === 'both' || hl === 'limits' ? outOfLimits(r, v) : ''; const cf = WG.condFormat(null, p, v); const col = cf.value || (lim === 'bad' ? 'var(--bad,#dc2626)' : lim === 'warn' ? 'var(--warn,#d97706)' : (p.value_color || 'inherit'));
          return `<td style="text-align:right;font-weight:600;white-space:nowrap;color:${col}">${v == null ? (r.latest && r.latest.value_json != null ? esc(JSON.stringify(r.latest.value_json)) : '—') : num(v, dec(r))} <span class="muted" style="font-weight:400">${esc(r.unit || '')}</span></td>`; }
        case 'age': { const s = staleOf(r, p); return `<td style="white-space:nowrap">${ageTxt(ageOf(r))}${s && r.latest ? ' <span class="badge warn">' + esc(tr('stale')) + '</span>' : ''}</td>`; }
        case 'measured': return `<td style="white-space:nowrap">${r.latest ? fmt(r.latest.measured_at) : '—'}</td>`;
        case 'quality': return `<td>${r.latest ? `<span class="badge ${r.latest.quality === 'ok' || r.latest.quality === 'backfilled' ? 'ok' : 'warn'}">${esc(r.latest.quality)}</span>` : ''}</td>`;
        case 'alarm': return `<td>${r.alarm ? `<span class="badge ${r.alarm.severity === 'warning' ? 'warn' : 'bad'}">${esc(r.alarm.severity)}${r.alarm.acknowledged ? ' ✓' : ''}</span>` : '<span class="muted">—</span>'}</td>`;
        case 'limits': return `<td style="white-space:nowrap" class="muted">${esc(limTxt(r.limits))}</td>`;
        case 'min': case 'max': case 'change': { const x = r.window ? r.window[c] : null; const lim = c !== 'change' && (hl === 'both' || hl === 'limits') ? outOfLimits(r, x) : ''; return `<td style="text-align:right;white-space:nowrap;font-variant-numeric:tabular-nums${lim ? ';color:var(--' + lim + ')' : ''}">${x == null ? '—' : num(x, dec(r)) + (r.unit ? ` <span class="muted">${esc(r.unit)}</span>` : '')}</td>`; }
        case 'device': return `<td class="notranslate">${r.device ? (WG.isPublic() ? esc(r.device) : `<a href="/devices/${esc(r.device_id)}">${esc(r.device)}</a>`) : '—'}</td>`;
        case 'datastream': return `<td class="notranslate" title="${esc(r.quantity)}">${esc(r.label || r.key)}</td>`;
        default: return `<td class="notranslate">${esc(r[c] || '—')}</td>`;
      }
    };
    const draw = () => {
      const list = sorted(filtered()); const pages = Math.max(1, Math.ceil(list.length / pageSize)); if (st.page >= pages) st.page = pages - 1;
      const shown = list.slice(st.page * pageSize, (st.page + 1) * pageSize);
      const th = (c) => `<th data-k="${c}" style="cursor:pointer;white-space:nowrap${OT_NUM.has(c) ? ';text-align:right' : ''}">${esc(tr(OT_LBL[c]))}${st.sort === c ? (st.dir > 0 ? ' ▲' : ' ▼') : ''}</th>`;
      const totals = {}; if (p.group_by && p.group_totals) for (const r of list) { const g = otGroup(r, p.group_by); const x = r.window ? r.window.change : null; if (x != null) totals[g] = (totals[g] || 0) + x; }
      const counts = {}; if (p.group_by) for (const r of list) { const g = otGroup(r, p.group_by); counts[g] = (counts[g] || 0) + 1; }
      let body = '', last = null;
      for (const r of shown) {
        if (p.group_by) { const g = otGroup(r, p.group_by); if (g !== last) { last = g; body += `<tr class="ot-group"><td colspan="${cols.length}"><b class="notranslate">${esc(g)}</b> <span class="muted">· ${counts[g]}${totals[g] != null ? ' · ' + esc(tr('change')) + ' ' + num(totals[g], dec(r)) + (r.unit ? ' ' + esc(r.unit) : '') : ''}</span></td></tr>`; } }
        const sev = r.alarm && (hl === 'both' || hl === 'alarms') ? (r.alarm.severity === 'warning' ? 'warn' : 'bad') : '';
        body += `<tr class="${sev ? 'ot-' + sev : ''}" data-ds="${esc(r.id)}">${cols.map((c) => cell(r, c)).join('')}</tr>`;
      }
      const canExport = !WG.isPublic() && p.export !== false && !ctx.edit && (!window.ctrl32 || !ctrl32.can || ctrl32.can('data.export'));
      const sel = (k, opts) => `<select data-f="${k}">${opts.map(([v, l]) => `<option value="${v}" ${st[k] === v ? 'selected' : ''}>${esc(tr(l))}</option>`).join('')}</select>`;
      const active = st.vmin !== '' || st.vmax !== '' || st.quality || st.fresh || st.alarm;
      el.innerHTML = `<div class="ot-bar">${p.search !== false ? `<input class="dw-search ot-q" type="search" placeholder="${esc(tr('Search…'))}" value="${esc(st.q)}">` : ''}${p.column_filters !== false ? `<button type="button" class="btn small ghost ot-ftoggle${active ? ' on' : ''}" aria-expanded="${st.open}">${esc(tr('Column filters'))}${active ? ' •' : ''}</button>` : ''}<span class="muted ot-count">${list.length === rows.length ? rows.length : list.length + ' / ' + rows.length}${res.truncated ? ' · ' + esc(tr('first')) + ' ' + rows.length + ' / ' + res.total : ''}</span><span style="flex:1"></span>${canExport ? `<button type="button" class="btn small ghost" data-x="xlsx">Excel</button><button type="button" class="btn small ghost" data-x="csv">CSV</button>` : ''}</div>`
        + (p.column_filters !== false && st.open ? `<div class="ot-filters"><label>${esc(tr('Value'))} <input type="number" step="any" data-f="vmin" value="${esc(st.vmin)}" placeholder="${esc(tr('from'))}"> – <input type="number" step="any" data-f="vmax" value="${esc(st.vmax)}" placeholder="${esc(tr('to'))}"></label><label>${esc(tr('Quality'))} ${sel('quality', [['', 'all'], ['ok', 'good'], ['doubtful', 'doubtful']])}</label><label>${esc(tr('Age'))} ${sel('fresh', [['', 'all'], ['fresh', 'fresh'], ['stale', 'stale']])}</label><label>${esc(tr('Alarm'))} ${sel('alarm', [['', 'all'], ['alarm', 'in alarm'], ['none', 'no alarm']])}</label>${active ? `<button type="button" class="btn small ghost" data-freset>${esc(tr('Clear'))}</button>` : ''}</div>` : '')
        + `<div class="table-wrap ot-wrap"><table class="ot-table"><thead><tr>${cols.map(th).join('')}</tr></thead><tbody>${body || `<tr><td colspan="${cols.length}" class="muted">${esc(tr(rows.length ? 'nothing found' : 'No datastreams match the query or the filters.'))}</td></tr>`}</tbody></table></div>`
        + (pages > 1 ? `<div class="ot-pages"><button type="button" class="btn small ghost" data-pg="-1" ${st.page ? '' : 'disabled'}>◀</button><span class="muted">${st.page + 1} / ${pages}</span><button type="button" class="btn small ghost" data-pg="1" ${st.page < pages - 1 ? '' : 'disabled'}>▶</button></div>` : '');
      for (const h of el.querySelectorAll('th[data-k]')) h.onclick = (e) => { e.stopPropagation(); if (st.sort === h.dataset.k) st.dir = -st.dir; else { st.sort = h.dataset.k; st.dir = OT_NUM.has(h.dataset.k) || h.dataset.k === 'alarm' ? -1 : 1; } draw(); };
      const q = el.querySelector('.ot-q'); if (q) q.oninput = () => { st.q = q.value; st.page = 0; const pos = q.selectionStart; draw(); const q2 = el.querySelector('.ot-q'); q2.focus(); q2.setSelectionRange(pos, pos); };
      const ft = el.querySelector('.ot-ftoggle'); if (ft) ft.onclick = (e) => { e.stopPropagation(); st.open = !st.open; draw(); };
      for (const f of el.querySelectorAll('.ot-filters [data-f]')) f.onchange = () => { st[f.dataset.f] = f.value; st.page = 0; draw(); };
      const fr = el.querySelector('[data-freset]'); if (fr) fr.onclick = (e) => { e.stopPropagation(); Object.assign(st, { vmin: '', vmax: '', quality: '', fresh: '', alarm: '', page: 0 }); draw(); };
      for (const b of el.querySelectorAll('[data-pg]')) b.onclick = (e) => { e.stopPropagation(); st.page += +b.dataset.pg; draw(); };
      for (const b of el.querySelectorAll('[data-x]')) b.onclick = async (e) => { e.stopPropagation(); b.disabled = true;
        try { const body = { ids: list.map((r) => r.id), columns: cols, format: b.dataset.x, title: p.title || tr('Overview'), decimals: p.decimals != null && p.decimals !== '' ? p.decimals : 2 }; if (needWin) { body.from = extra.from; body.to = extra.to; }
          const blob = await api('POST', '/api/v1/datastreams/export', body); const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = (p.title || 'overview').replace(/[^\w\- ]+/g, '').trim().replace(/\s+/g, '_') + '.' + b.dataset.x; document.body.appendChild(a); a.click(); setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
        } catch (x) { if (window.ctrl32 && ctrl32.toast) ctrl32.toast(x.message, 'bad'); } b.disabled = false; };
      for (const tr0 of el.querySelectorAll('tr[data-ds]')) tr0.ondblclick = () => { if (window.ctrl32Scada && ctrl32Scada.openTag) ctrl32Scada.openTag(tr0.dataset.ds, { trendOnly: true }); };
    };
    draw();
  }, { needsDs: true, label: 'Overview table' });

  // ---- other ----------------------------------------------------------------------------------------------------
  const clockFmt = (el) => { const tz = el.dataset.tz || undefined, h12 = el.dataset.f === '12h'; const now = new Date(); let t, d; try { t = now.toLocaleTimeString(document.documentElement.lang || [], { timeZone: tz, hour12: h12, hour: '2-digit', minute: '2-digit', second: '2-digit' }); d = now.toLocaleDateString(document.documentElement.lang || [], { timeZone: tz, weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }); } catch (e) { t = now.toLocaleTimeString(document.documentElement.lang || undefined); d = now.toLocaleDateString(document.documentElement.lang || undefined); } el.querySelector('.dw-time').textContent = t; const de = el.querySelector('.dw-date'); if (de) de.textContent = d; };
  setInterval(() => document.querySelectorAll('.dw-clock').forEach(clockFmt), 1000);
  reg('clock', async (el, p) => {
    el.innerHTML = `<div class="dw-center dw-clock notranslate" data-tz="${esc(p.tz || '')}" data-f="${esc(p.format || '24h')}"><div class="dw-time dw-big" style="font-variant-numeric:tabular-nums"></div>${p.show_date !== false ? '<div class="dw-date muted"></div>' : ''}${p.tz ? `<div class="muted" style="font-size:11px">${esc(p.tz)}</div>` : ''}</div>`;
    clockFmt(el.querySelector('.dw-clock'));
  }, { label: 'Clock' });

  // Markdown subset: # headings, **bold**, *italic*, `code`, - lists, [text](screen:slug | https://…); {value:N}.
  function markdown(src, vals) {
    const inline = (s) => esc(s).replace(/\{value:(\d+)\}/g, (_, n) => esc(vals[+n - 1] ?? '—')).replace(/`([^`]+)`/g, '<code>$1</code>').replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>').replace(/\*([^*]+)\*/g, '<i>$1</i>')
      .replace(/\[([^\]]+)\]\((screen:[a-z0-9-]+|https:\/\/[^\s)"']+)\)/g, (_, t, h) => `<a href="${h.startsWith('screen:') ? '/dashboards/' + h.slice(7) : h}"${h.startsWith('https:') ? ' target="_blank" rel="noopener"' : ''}>${t}</a>`);
    let out = '', list = false;
    for (const line of String(src || '').split('\n')) {
      const m = /^(#{1,3})\s+(.*)$/.exec(line), li = /^\s*[-*]\s+(.*)$/.exec(line);
      if (li) { if (!list) { out += '<ul>'; list = true; } out += `<li>${inline(li[1])}</li>`; continue; }
      if (list) { out += '</ul>'; list = false; }
      if (m) out += `<h${m[1].length}>${inline(m[2])}</h${m[1].length}>`; else if (line.trim()) out += `<p>${inline(line)}</p>`;
    }
    return out + (list ? '</ul>' : '');
  }
  reg('markdown', async (el, p, ctx) => {
    const rows = ids(ctx).length ? await many(ctx, p) : []; const vals = rows.map((r) => r.v == null ? '—' : num(r.v, r.d.dec) + (r.d.unit ? ' ' + r.d.unit : ''));
    el.innerHTML = `<div class="dw-md notranslate" style="overflow:auto;flex:1;min-height:0">${markdown(p.text, vals) || '<span class="muted">empty</span>'}</div>`;
    if (WG.isPublic()) el.querySelectorAll('a[href^="/dashboards/"]').forEach((a) => a.removeAttribute('href'));
  }, { label: 'Markdown card' });

  reg('image', async (el, p) => {
    if (!p.src) { noData(el, 'choose a graphic from the library'); return; }
    el.innerHTML = `<div style="flex:1;min-height:0;display:flex"><img alt="${esc(p.title || p.src)}" src="${esc(WG.apiPath('/api/v1/hmi/objects/') + encodeURIComponent(p.src) + '/svg')}" style="width:100%;height:100%;object-fit:${p.fit === 'cover' ? 'cover' : 'contain'}"></div>`;
  }, { label: 'Image' });

  let qrReady = null;
  const loadQR = () => { if (window.qrcode) return Promise.resolve(); if (!qrReady) qrReady = new Promise((ok, bad) => { const s = document.createElement('script'); s.src = '/vendor/qrcode.js'; s.onload = ok; s.onerror = () => bad(new Error('QR library could not be loaded')); document.head.appendChild(s); }); return qrReady; };
  reg('qr', async (el, p) => {
    await loadQR(); const text = p.text || location.href; window.qrcode.stringToBytes = window.qrcode.stringToBytesFuncs['UTF-8'];
    const q = window.qrcode(0, 'M'); q.addData(text); q.make(); const n = q.getModuleCount(); let d = '';
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (q.isDark(r, c)) d += `M${c + 2},${r + 2}h1v1h-1z`;
    el.innerHTML = `<div class="dw-center"><div style="flex:1;min-height:0;width:100%">${svg(`0 0 ${n + 4} ${n + 4}`, `<rect width="${n + 4}" height="${n + 4}" fill="#fff"/><path d="${d}" fill="#000"/>`, 'shape-rendering:crispEdges')}</div><div class="muted notranslate" style="font-size:10px;max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(text)}</div></div>`;
  }, { label: 'QR code' });

  reg('nav_card', async (el, p) => {
    const inner = `${p.icon ? `<span style="color:var(--primary)">${icon(p.icon, 32)}</span>` : ''}<span style="min-width:0"><b>${esc(p.title || p.target || '')}</b>${p.description ? `<br><span class="muted" style="font-size:12px">${esc(p.description)}</span>` : ''}</span><span class="grow"></span><span class="muted">›</span>`;
    el.innerHTML = p.target && !WG.isPublic() ? `<a class="dw-nav" href="/dashboards/${encodeURIComponent(p.target)}">${inner}</a>` : `<div class="dw-nav">${inner}</div>`;
  }, { label: 'Navigation card' });

  // ---- camera (spec 010): live picture; the player is kept across refreshes and closed when the widget goes away --
  function loadPlayer() {
    if (window.ctrl32Player) return Promise.resolve();
    if (!loadPlayer.p) loadPlayer.p = new Promise((res, rej) => { const s = document.createElement('script'); s.src = '/videoplayer.js'; s.onload = res; s.onerror = rej; document.head.appendChild(s); });
    return loadPlayer.p;
  }
  reg('camera', async (el, p) => {
    // public links never show cameras; a paired display plays the cameras of its dashboards (spec 014)
    const display = WG.isDisplay && WG.isDisplay();
    if (WG.isPublic() && !display) { el.innerHTML = '<div class="dw-center muted">Cameras are not shown on public dashboards.</div>'; return; }
    if (!p.camera) { el.innerHTML = '<div class="dw-center muted">Choose a camera in the widget properties.</div>'; return; }
    const key = p.camera + '|' + (p.quality || 'sub') + '|' + (p.show_name !== false) + '|' + (p.open_on_click !== false) + '|' + !!p.show_stats;
    if (el._cam && el._cam.key === key && el._cam.player.el.isConnected) return; // live already; refreshes do not reconnect
    if (el._cam) el._cam.player.close();
    await loadPlayer();
    let name = '';
    if (display) name = p.show_name !== false ? ((window.ctrl32DisplayCameras || {})[p.camera] || '') : '';
    else if (p.show_name !== false) { try { name = (await api('GET', `/api/v1/video/cameras/${encodeURIComponent(p.camera)}`)).name; } catch (e) { el.innerHTML = `<div class="dw-center muted">${esc(e.status === 403 ? 'No permission to watch cameras.' : e.message)}</div>`; el._cam = null; return; } }
    el.innerHTML = `<div class="cam-widget${p.open_on_click !== false ? ' click' : ''}">${name ? `<div class="cap"><span class="cam-dot"></span><b class="notranslate">${esc(name)}</b></div>` : ''}</div>`;
    const box = el.firstElementChild, dot = box.querySelector('.cam-dot');
    const liveBase = display ? '/api/v1/display/cameras/' : '/api/v1/video/cameras/';
    const player = window.ctrl32Player(box, { url: `${liveBase}${encodeURIComponent(p.camera)}/live?quality=${p.quality === 'main' ? 'main' : 'sub'}`, live: true, stats: !!p.show_stats,
      onState: (s) => { if (dot) dot.className = 'cam-dot ' + (s === 'online' ? 'online' : s === 'offline' ? 'offline' : ''); } });
    box.prepend(player.el);
    if (p.open_on_click !== false && !display) box.onclick = () => { location.href = '/video/cameras/' + encodeURIComponent(p.camera); };
    el._cam = { key, player };
  }, { label: 'Camera' });

  // ---- camera on a plan (spec 017): field of view, state, events; a click shows the live picture ----------------
  const pinCache = new Map(); // camera id → {at, cam} for signed-in users (one request per camera per 10 s)
  async function pinInfo(id) {
    if (WG.isDisplay && WG.isDisplay()) {
      const a = (window.ctrl32DisplayActive || {})[id];
      return { name: (window.ctrl32DisplayCameras || {})[id] || '', online: true, active: a ? a.kinds : [] };
    }
    const c = pinCache.get(id);
    if (c && Date.now() - c.at < 10000) return c.info;
    const cam = await api('GET', `/api/v1/video/cameras/${encodeURIComponent(id)}`);
    const info = { name: cam.name, online: cam.status.state === 'online' || cam.status.state === 'idle', active: cam.status.active || [] };
    pinCache.set(id, { at: Date.now(), info });
    return info;
  }
  function pinPopup(id, name) {
    const display = WG.isDisplay && WG.isDisplay();
    const ov = document.createElement('div'); ov.className = 'pin-pop';
    ov.innerHTML = `<div class="pin-pop-box"><div class="pin-pop-head"><b class="notranslate">${esc(name)}</b><span class="grow"></span>${display ? '' : `<a class="btn small" href="/video/cameras/${encodeURIComponent(id)}">${esc(WG.tr('Open camera'))}</a>`}<button class="btn small" type="button" data-x>✕</button></div><div class="pin-pop-vid"></div></div>`;
    document.body.appendChild(ov);
    loadPlayer().then(() => {
      const base = display ? '/api/v1/display/cameras/' : '/api/v1/video/cameras/';
      const pl = window.ctrl32Player(ov.querySelector('.pin-pop-vid'), { url: `${base}${encodeURIComponent(id)}/live?quality=main`, live: true });
      const close = () => { pl.close(); ov.remove(); document.removeEventListener('keydown', esc_); };
      const esc_ = (e) => { if (e.key === 'Escape') close(); };
      ov.querySelector('[data-x]').onclick = close; ov.onclick = (e) => { if (e.target === ov) close(); }; document.addEventListener('keydown', esc_);
      if (display) setTimeout(close, 60000); // a display returns to its content by itself
    });
  }
  reg('camera_pin', async (el, p) => {
    const pub = WG.isPublic() && !(WG.isDisplay && WG.isDisplay());
    if (!p.camera) { el.innerHTML = '<div class="dw-center muted">Choose a camera in the widget properties.</div>'; return; }
    let info = { name: '', online: true, active: [] };
    if (!pub) { try { info = await pinInfo(p.camera); } catch (e) { info = { name: '', online: false, active: [], error: e.status === 403 || e.status === 404 }; } }
    if (info.error) { el.innerHTML = ''; return; } // a camera the user may not see is not on their plan
    const dir = (+(p.direction ?? 90)) % 360, fov = Math.max(10, Math.min(180, +(p.fov ?? 70))), reach = Math.max(0, Math.min(100, +(p.reach ?? 80))) / 100 * 46;
    const rad = (a) => (a - 90) * Math.PI / 180, pt = (a) => [50 + reach * Math.cos(rad(a)), 50 + reach * Math.sin(rad(a))];
    const [x1, y1] = pt(dir - fov / 2), [x2, y2] = pt(dir + fov / 2);
    const alarm = info.active.length > 0, off = !info.online;
    const cone = reach > 0 ? `<path d="M50,50 L${x1.toFixed(2)},${y1.toFixed(2)} A${reach.toFixed(2)},${reach.toFixed(2)} 0 ${fov > 180 ? 1 : 0} 1 ${x2.toFixed(2)},${y2.toFixed(2)} Z" class="pin-cone${alarm ? ' alarm' : ''}"/>` : '';
    const label = p.show_name !== false && info.name ? `<text x="50" y="97" text-anchor="middle" class="pin-label">${esc(p.title || info.name)}</text>` : '';
    el.innerHTML = `<svg class="cam-pin${alarm ? ' alarm' : ''}${off ? ' off' : ''}${pub ? '' : ' click'}" viewBox="0 0 100 100" preserveAspectRatio="xMidYMid meet" role="img" aria-label="${esc(info.name || 'camera')}">
      <title>${esc((info.name || '') + (alarm ? ' · ' + info.active.map((k) => WG.tr(k)).join(', ') : off ? ' · ' + WG.tr('offline') : ''))}</title>
      ${cone}<g transform="rotate(${dir} 50 50)"><rect x="41" y="44" width="18" height="12" rx="2" class="pin-body"/><rect x="47" y="38" width="6" height="7" rx="1" class="pin-body"/></g>
      <circle cx="50" cy="50" r="3" class="pin-dot"/>${label}</svg>`;
    if (!pub) el.firstElementChild.onclick = (e) => { e.stopPropagation(); pinPopup(p.camera, info.name || p.title || ''); };
  }, { label: 'Camera on a plan' });

  // ---- controls: commands with reason, confirmation, permission and audit (FR-009) --------------------------------
  const ctlFoot = () => `<div class="row" style="gap:6px;align-items:center;flex-wrap:wrap">${u.reasonInput()}<button class="btn small primary ctl-send" type="button">Send</button></div><div class="ctl-out" style="font-size:12px;min-height:16px"></div>`;
  const snap = (v, p) => { const lo = p.min ?? 0, hi = p.max ?? 100, st = p.step > 0 ? p.step : 1; return clamp(Math.round((v - lo) / st) * st + lo, lo, hi); };
  // "Unit (empty = datastream)": the unit of the bound datastream when the setting is empty
  const ctlUnit = async (ctx, p) => { if (p.unit != null && p.unit !== '') return p.unit; const id = ((ctx.bindings || {}).datastreams || [])[0]; if (!id) return ''; const d = (await WG.datastreams().catch(() => [])).find((x) => x.id === id); return unitOf(p, d || {}); };
  reg('slider', async (el, p, ctx) => {
    const h = u.ctlHead(ctx, p); const cur = await u.ctlCurrent(ctx, p); const un = await ctlUnit(ctx, p); const lo = p.min ?? 0, hi = p.max ?? 100; const v0 = cur && cur.value != null ? snap(cur.value, p) : lo;
    const ro = !!h.html; const dec = String(p.step || 1).includes('.') ? String(p.step).split('.')[1].length : 0;
    el.innerHTML = `<div style="display:flex;align-items:center;gap:10px"><span class="muted">${lo}</span><input class="dw-slider" type="range" min="${lo}" max="${hi}" step="${p.step > 0 ? p.step : 1}" value="${v0}" style="flex:1"${ro ? ' disabled' : ''}><span class="muted">${hi}</span><b class="dw-sv" style="min-width:60px;text-align:right">${num(v0, dec)} ${esc(un)}</b></div>${cur ? `<div class="muted" style="font-size:11px">current ${num(cur.value, dec)} · ${fmt(cur.measured_at)}</div>` : ''}${ro ? h.html : ctlFoot()}`;
    const s = el.querySelector('.dw-slider'); s.oninput = () => { el.querySelector('.dw-sv').textContent = num(+s.value, dec) + ' ' + un; };
    if (!ro) el.querySelector('.ctl-send').onclick = () => u.ctlSend(el, ctx, p, snap(+s.value, p));
  }, { label: 'Slider' });

  reg('knob', async (el, p, ctx) => {
    const h = u.ctlHead(ctx, p); const cur = await u.ctlCurrent(ctx, p); const un = await ctlUnit(ctx, p); const lo = p.min ?? 0, hi = p.max ?? 100; let v = cur && cur.value != null ? snap(cur.value, p) : lo; const ro = !!h.html;
    const dec = String(p.step || 1).includes('.') ? String(p.step).split('.')[1].length : 0; const A0 = -135, SW = 270;
    const draw = () => { const f = (v - lo) / ((hi - lo) || 1), a = (A0 + f * SW) * Math.PI / 180; const kn = el.querySelector('.dw-knob'); kn.querySelector('.dw-kp').setAttribute('x2', (60 + 34 * Math.sin(a)).toFixed(1)); kn.querySelector('.dw-kp').setAttribute('y2', (60 - 34 * Math.cos(a)).toFixed(1)); const e = (A0 + f * SW) * Math.PI / 180, s0 = A0 * Math.PI / 180; kn.querySelector('.dw-ka').setAttribute('d', `M${(60 + 50 * Math.sin(s0)).toFixed(1)},${(60 - 50 * Math.cos(s0)).toFixed(1)} A50,50 0 ${f * SW > 180 ? 1 : 0} 1 ${(60 + 50 * Math.sin(e)).toFixed(1)},${(60 - 50 * Math.cos(e)).toFixed(1)}`); el.querySelector('.dw-kv2').textContent = num(v, dec) + ' ' + un; };
    el.innerHTML = `<div class="dw-center"><div style="flex:1;min-height:70px;width:100%">${svg('0 0 120 120', `<g class="dw-knob${ro ? '' : ' dw-ctl-knob'}"><path d="M${(60 + 50 * Math.sin(A0 * Math.PI / 180)).toFixed(1)},${(60 - 50 * Math.cos(A0 * Math.PI / 180)).toFixed(1)} A50,50 0 1 1 ${(60 + 50 * Math.sin(135 * Math.PI / 180)).toFixed(1)},${(60 - 50 * Math.cos(135 * Math.PI / 180)).toFixed(1)}" fill="none" stroke="var(--line)" stroke-width="6" stroke-linecap="round"/><path class="dw-ka" fill="none" stroke="var(--primary)" stroke-width="6" stroke-linecap="round"/><circle cx="60" cy="60" r="40" fill="var(--bg2)" stroke="var(--muted)"/><line class="dw-kp" x1="60" y1="60" x2="60" y2="26" stroke="var(--fg)" stroke-width="4" stroke-linecap="round"/></g>`)}</div><b class="dw-kv2"></b>${cur ? `<span class="muted" style="font-size:11px">current ${num(cur.value, dec)}</span>` : ''}</div>${ro ? h.html : ctlFoot()}`;
    draw(); if (ro) return;
    const kn = el.querySelector('.dw-knob'), sv = kn.ownerSVGElement;
    const fromEvt = (e) => { const r = sv.getBoundingClientRect(); const x = (e.clientX - r.left) / r.width * 120 - 60, y = (e.clientY - r.top) / r.height * 120 - 60; let a = Math.atan2(x, -y) * 180 / Math.PI; a = clamp(a, A0, A0 + SW); v = snap(lo + (a - A0) / SW * (hi - lo), p); draw(); };
    kn.addEventListener('pointerdown', (e) => { e.preventDefault(); kn.setPointerCapture(e.pointerId); fromEvt(e); kn.onpointermove = fromEvt; });
    kn.addEventListener('pointerup', () => { kn.onpointermove = null; });
    sv.addEventListener('wheel', (e) => { e.preventDefault(); v = snap(v + (e.deltaY < 0 ? 1 : -1) * (p.step > 0 ? p.step : 1), p); draw(); }, { passive: false });
    el.querySelector('.ctl-send').onclick = () => u.ctlSend(el, ctx, p, v);
  }, { label: 'Knob' });

  reg('round_switch', async (el, p, ctx) => {
    const h = u.ctlHead(ctx, p); const cur = await u.ctlCurrent(ctx, p); const on = cur ? (cur.value === 1 || cur.value === true || cur.value_json === true) : false; const ro = !!h.html;
    el.innerHTML = `<div class="dw-center"><button class="dw-rsw${on ? ' on' : ''}" type="button"${ro ? ' disabled' : ''} title="${esc(on ? (p.on_label || 'ON') : (p.off_label || 'OFF'))}">${icon('power', 26) || '⏻'}<br><span style="font-size:11px">${esc(on ? (p.on_label || 'ON') : (p.off_label || 'OFF'))}</span></button>${cur ? `<span class="muted" style="font-size:11px">${fmt(cur.measured_at)}</span>` : ''}</div>${ro ? h.html : `<div class="row" style="gap:6px">${u.reasonInput()}</div><div class="ctl-out" style="font-size:12px;min-height:16px"></div>`}`;
    if (!ro) el.querySelector('.dw-rsw').onclick = () => u.ctlSend(el, ctx, p, !on);
  }, { label: 'Round switch' });
})();
