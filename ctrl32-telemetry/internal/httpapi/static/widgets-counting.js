// ctrl32 telemetry — dashboard widgets of object counting (spec 029): occupancy of an area, in and out per entrance,
// share per entrance, vehicle speeds. Each widget binds one datastream of the area (or of the speed section) and reads
// the area's summary; registered with ctrl32Widgets.register like the other widget types.
(() => {
  const WG = window.ctrl32Widgets; if (!WG) return;
  const u = WG.util, { esc, ids, api, P } = u;
  const t = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
  const nf = (v, d = 0) => new Intl.NumberFormat(document.documentElement.lang || undefined, { maximumFractionDigits: d }).format(v || 0);
  const COLORS = ['#e30613', '#2563eb', '#16a34a', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#65a30d'];
  const cache = new Map(); // datastream → {at, data}: several widgets of one area share one request
  const ymd = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  async function summary(ctx, p) {
    const id = ids(ctx)[0];
    if (!id) throw new Error('bind a datastream of a counting area');
    const date = p && p.day === 'yesterday' ? ymd(new Date(Date.now() - 864e5)) : '';
    const key = id + '|' + date, c = cache.get(key);
    if (c && Date.now() - c.at < 15000) return c.data;
    const data = await api('GET', P(`/api/v1/counting/summary?datastream=${encodeURIComponent(id)}&days=1${date ? '&date=' + date : ''}`));
    cache.set(key, { at: Date.now(), data });
    return data;
  }
  const none = (el, s) => { el.innerHTML = `<div class="dw-center muted">${esc(t(s))}</div>`; };
  const group = (p, d) => (d.area.groups.includes(p.group) ? p.group : d.area.groups[0]);

  WG.register('counting_occupancy', async (el, p, ctx) => {
    const d = await summary(ctx, null).catch((e) => { none(el, e.message); return null; }); if (!d) return;
    const g = group(p, d), occ = d.area.occupancy[g] || 0, cap = d.area.limits[g] || 0, tot = d.totals[g] || { in: 0, out: 0 };
    const pct = cap ? Math.min(100, occ / cap * 100) : 0, col = pct >= 100 ? 'var(--bad)' : pct >= 80 ? 'var(--warn)' : 'var(--ok)';
    el.innerHTML = `<div style="display:flex;flex-direction:column;justify-content:center;flex:1;gap:4px;min-height:0">
      <div class="muted notranslate" style="font-size:12px">${esc(d.area.name)}</div>
      <div class="dw-big">${nf(occ)}${cap ? ` <span style="font-size:.4em;color:var(--muted)">/ ${nf(cap)}</span>` : ''}</div>
      ${cap ? `<div style="height:6px;border-radius:3px;background:var(--line);overflow:hidden"><span style="display:block;height:100%;width:${pct}%;background:${col}"></span></div>` : ''}
      ${p.show_today !== false ? `<div class="muted" style="font-size:12px;display:flex;gap:12px"><span>${esc(t('in'))} <b>${nf(tot.in)}</b></span><span>${esc(t('out'))} <b>${nf(tot.out)}</b></span><span>${esc(t('peak'))} <b>${nf(d.peak[g] || 0)}</b></span></div>` : ''}</div>`;
  }, { needsDs: true, label: 'Occupancy' });

  WG.register('counting_entrances', async (el, p, ctx) => {
    const d = await summary(ctx, p).catch((e) => { none(el, e.message); return null; }); if (!d) return;
    const g = group(p, d), list = d.entrances.map((e) => ({ name: e.name, a: (e.counts[g] || {}).in || 0, b: (e.counts[g] || {}).out || 0 }));
    if (!list.length) return none(el, 'No entrances');
    const max = Math.max(1, ...list.map((x) => Math.max(x.a, x.b)));
    el.innerHTML = `<div style="display:flex;flex-direction:column;gap:6px;overflow:auto;flex:1">${list.map((x) => `<div><div class="notranslate" style="font-size:12px">${esc(x.name)}</div>
      <div style="display:flex;align-items:center;gap:6px"><span style="height:10px;border-radius:3px;background:var(--primary);width:${x.a / max * 80}%"></span><span style="font-size:12px">${nf(x.a)} ${esc(t('in'))}</span></div>
      <div style="display:flex;align-items:center;gap:6px"><span style="height:10px;border-radius:3px;background:#64748b;width:${x.b / max * 80}%"></span><span style="font-size:12px">${nf(x.b)} ${esc(t('out'))}</span></div></div>`).join('')}</div>`;
  }, { needsDs: true, label: 'In and out per entrance' });

  WG.register('counting_share', async (el, p, ctx) => {
    const d = await summary(ctx, p).catch((e) => { none(el, e.message); return null; }); if (!d) return;
    const g = group(p, d), dir = p.direction === 'out' ? 'out' : 'in';
    const list = d.entrances.map((e, i) => ({ name: e.name, v: (e.counts[g] || {})[dir] || 0, c: COLORS[i % COLORS.length] }));
    const sum = list.reduce((a, x) => a + x.v, 0);
    if (!sum) return none(el, 'nothing yet');
    let a0 = -Math.PI / 2, arcs = '';
    for (const x of list) {
      if (!x.v) continue;
      const a1 = a0 + 2 * Math.PI * x.v / sum, big = a1 - a0 > Math.PI ? 1 : 0;
      const p0 = [50 + 40 * Math.cos(a0), 50 + 40 * Math.sin(a0)], p1 = [50 + 40 * Math.cos(a1), 50 + 40 * Math.sin(a1)];
      arcs += x.v === sum ? `<circle cx="50" cy="50" r="40" fill="none" stroke="${x.c}" stroke-width="18"/>` : `<path d="M${p0} A40 40 0 ${big} 1 ${p1}" fill="none" stroke="${x.c}" stroke-width="18"><title>${esc(x.name)}</title></path>`;
      a0 = a1;
    }
    el.innerHTML = `<div style="display:flex;gap:10px;align-items:center;flex:1;min-height:0"><svg viewBox="0 0 100 100" style="height:100%;max-height:180px;flex:0 0 auto">${arcs}<text x="50" y="54" text-anchor="middle" font-size="13" fill="currentColor">${nf(sum)}</text></svg>
      <div style="font-size:12px;display:flex;flex-direction:column;gap:3px">${list.map((x) => `<div><span style="display:inline-block;width:9px;height:9px;border-radius:2px;background:${x.c}"></span> <span class="notranslate">${esc(x.name)}</span> <b>${nf(x.v / sum * 100, 1)} %</b></div>`).join('')}</div></div>`;
  }, { needsDs: true, label: 'Share per entrance' });

  WG.register('vehicle_speeds', async (el, p, ctx) => {
    const d = await summary(ctx, p).catch((e) => { none(el, e.message); return null; }); if (!d) return;
    const id = ids(ctx)[0], s = (d.speeds || []).find((x) => x.speed_datastream === id) || (d.speeds || [])[0];
    if (!s) return none(el, 'Bind the vehicle_speed datastream of a speed section');
    const rows = Math.max(1, Math.min(10, p.rows || 6)), max = Math.max(1, ...s.histogram);
    const hist = p.histogram === false ? '' : `<svg viewBox="0 0 280 60" style="width:100%;height:60px">${s.histogram.map((n, i) => `<rect x="${i * 20 + 1}" y="${50 - 46 * n / max}" width="18" height="${46 * n / max}" fill="${s.limit_kmh && i * 10 >= s.limit_kmh ? '#dc2626' : 'var(--primary)'}"><title>${i * 10}–${i * 10 + 10} km/h: ${n}</title></rect>`).join('')}<text x="0" y="59" font-size="7" fill="currentColor">0</text><text x="270" y="59" font-size="7" fill="currentColor">130+</text></svg>`;
    el.innerHTML = `<div style="display:flex;flex-direction:column;gap:6px;flex:1;min-height:0;overflow:auto">
      <div style="display:flex;gap:14px;flex-wrap:wrap;font-size:12px"><span>${esc(t('Vehicles'))} <b>${nf(s.count)}</b></span><span>${esc(t('average'))} <b>${s.avg_kmh != null ? nf(s.avg_kmh, 1) : '—'}</b> km/h</span><span>${esc(t('max'))} <b>${s.max_kmh != null ? nf(s.max_kmh, 1) : '—'}</b> km/h</span>${s.limit_kmh ? `<span>${esc(t('over the limit'))} <b style="color:${s.over_limit ? '#dc2626' : 'inherit'}">${nf(s.over_limit)}</b> (&gt; ${nf(s.limit_kmh)} km/h)</span>` : ''}</div>
      ${hist}
      <table style="font-size:12px;width:100%"><tbody>${s.last.slice(0, rows).map((v) => `<tr><td>${esc(new Date(v.at).toLocaleTimeString(document.documentElement.lang || undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }))}</td><td>${esc(t(v.class))}</td><td>${v.direction === 'ab' ? 'A → B' : 'B → A'}</td><td style="text-align:right;${v.over ? 'color:#dc2626;font-weight:600' : ''}">${nf(v.kmh, 1)} km/h</td></tr>`).join('')}</tbody></table></div>`;
  }, { needsDs: true, label: 'Vehicle speeds' });
})();
