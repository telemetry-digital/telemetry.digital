// The telemetry panel (0.67.0, shared since 0.74.0): a table of keys with the latest value, quality, age, a 24-hour
// sparkline and statistics over a range, live updates, and for one key a chart, its values (paged), annotations and
// exports. The device page shows every key a device sends; the asset page every datastream of an asset (optionally
// of the assets below it). One panel per page: it renders into the given element and owns the tel* element ids.
//
//   ctrl32Telemetry.mount(el, {
//     base: '/api/v1/devices/<id>/telemetry',   // summary; /series, /values and /export below it
//     summaryQuery: () => '&children=1',        // extra query of the summary
//     keyQuery: (k) => 'source=…&key=…',        // which key the chart, values and export are about
//     rowId: (k) => 'datastream:temp',          // stable row identity
//     columns: ['source'] | ['asset', 'device'],// extra columns after the key
//     tools: '<label>…</label>', onTools(T)    // extra controls in the tool bar
//     streamURL: (keys) => '/api/v1/stream?…',  // live updates; listen(es, T) adds page-specific events
//     side: (k, el, T) => {}                    // the panel next to the values (default: annotations of a datastream)
//     note, empty                               // texts under the table and when there is nothing
//   })
(function () {
  const $ = (s) => document.querySelector(s);
  const esc = (s) => ctrl32.esc(s), fmt = (t) => ctrl32.fmt(t), api = (m, p, b) => ctrl32.api(m, p, b);
  const MS = { '1h': 3600e3, '24h': 86400e3, '7d': 7 * 86400e3, '30d': 30 * 86400e3 };
  const SRC = { datastream: ['primary', 'datastream'], unassigned: ['warn', 'no datastream'], status: ['', 'status'] };
  const lang = () => document.documentElement.lang || undefined;
  const isoOf = (t) => new Date(t).toISOString().replace(/\.\d{3}Z$/, 'Z');
  const localInput = (t) => { const d = new Date(t), p = (n) => String(n).padStart(2, '0'); return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`; };
  function relTime(t) { if (!t) return '—'; const s = (new Date(t).getTime() - Date.now()) / 1000, a = Math.abs(s), rtf = new Intl.RelativeTimeFormat(lang(), { numeric: 'auto' });
    return a < 60 ? rtf.format(Math.round(s), 'second') : a < 3600 ? rtf.format(Math.round(s / 60), 'minute') : a < 86400 ? rtf.format(Math.round(s / 3600), 'hour') : rtf.format(Math.round(s / 86400), 'day'); }
  function numFmt(v) { if (v == null || !isFinite(v)) return '—'; const a = Math.abs(v); return Number(v).toLocaleString(lang(), { maximumFractionDigits: a >= 100 ? 1 : a >= 1 ? 2 : 4 }); }
  function valText(l) { if (!l) return '—'; if (l.value != null) return numFmt(l.value); if (l.value_json != null) return typeof l.value_json === 'string' ? l.value_json : JSON.stringify(l.value_json); return '—'; }
  function qualBadge(q) { if (!q) return ''; if (q === 'ok') return '<span class="badge ok">ok</span>'; if (q === 'sensor_fault') return `<span class="badge bad">${esc(ctrl32.t('sensor fault'))}</span>`; return `<span class="badge warn">${esc(ctrl32.t(q.replace(/_/g, ' ')))}</span>`; }
  const COLS = { source: 'Source', asset: 'Asset', device: 'Device' };

  function mount(root, cfg) {
    const T = { cfg, keys: [], range: '24h', from: null, to: null, sel: null, es: null, liveAt: null, d: null, pages: [''], page: 0, reload: null, tick: null, series: null };
    const cols = cfg.columns || [];
    const ncol = 8 + cols.length;
    root.innerHTML = `<div class="card">
      <div class="row tel-tools">
        <input id="telSearch" type="search" placeholder="Search keys" aria-label="Search keys" autocomplete="off">
        <label>Sort <select id="telSort"><option value="name">Name</option><option value="last">Last update</option></select></label>
        <label>Statistics over <select id="telRange"><option value="1h">1 hour</option><option value="24h" selected>24 hours</option><option value="7d">7 days</option><option value="30d">30 days</option><option value="custom">Custom range</option></select></label>
        <span id="telCustom" class="row" hidden style="gap:6px;align-items:end"><label>From <input type="datetime-local" id="telFrom"></label><label>To <input type="datetime-local" id="telTo"></label><button class="btn small" type="button" id="telApply">Apply</button></span>
        ${cfg.tools || ''}<span class="grow"></span><span class="muted" id="telLive"></span>
      </div>
      <div class="table-wrap" style="margin-top:12px"><table id="telTable"><thead><tr><th>Key</th>${cols.map((c) => `<th>${COLS[c]}</th>`).join('')}<th>Latest value</th><th>Quality</th><th>Last value</th><th>Last 24 h</th><th>Min</th><th>Max</th><th>Average</th></tr></thead><tbody><tr><td colspan="${ncol}" class="muted">…</td></tr></tbody></table></div>
      ${cfg.note ? `<p class="muted" style="margin-bottom:0;font-size:12px">${cfg.note}</p>` : ''}
    </div>
    <div class="card" id="telDetail" hidden style="margin-top:12px"></div>`;
    const span = () => { if (T.range === 'custom' && T.from && T.to) return [T.from, T.to]; const now = Date.now(); return [now - MS[T.range === 'custom' ? '24h' : T.range], now + 60e3]; };
    $('#telSearch').oninput = render; $('#telSort').onchange = render;
    $('#telRange').onchange = () => { T.range = $('#telRange').value; $('#telCustom').hidden = T.range !== 'custom'; if (T.range === 'custom') { const [f, t] = span(); $('#telFrom').value = localInput(f); $('#telTo').value = localInput(t); } else load(); };
    $('#telApply').onclick = () => { const f = new Date($('#telFrom').value).getTime(), t = new Date($('#telTo').value).getTime(); if (!(t > f)) { ctrl32.toast('to must be after from', 'bad'); return; } T.from = f; T.to = t; load(); };
    T.tick = setInterval(() => { for (const el of document.querySelectorAll('#telTable [data-rel]')) el.textContent = relTime(el.dataset.rel); }, 20000);
    if (cfg.onTools) cfg.onTools(T);

    async function load() {
      const [f, t] = span();
      try { const r = await api('GET', `${cfg.base}?from=${isoOf(f)}&to=${isoOf(t)}${cfg.summaryQuery ? cfg.summaryQuery() : ''}`); T.keys = r.keys; T.summary = r; }
      catch (x) { $('#telTable tbody').innerHTML = `<tr><td colspan="${ncol}" class="muted">${esc(x.message)}</td></tr>`; return; }
      render(); live();
      if (T.sel) { const k = T.keys.find((x) => cfg.rowId(x) === cfg.rowId(T.sel)) || T.keys.find((x) => x.key === T.sel.key && x.source === 'datastream'); if (k) T.sel = k; }
    }
    function visible() {
      const q = ($('#telSearch').value || '').trim().toLowerCase();
      let ks = T.keys.filter((k) => !q || [k.key, k.asset, k.quantity, k.source, k.device].some((s) => s && s.toLowerCase().includes(q)));
      if ($('#telSort').value === 'last') ks = ks.slice().sort((a, b) => (b.last ? new Date(b.last.at).getTime() : 0) - (a.last ? new Date(a.last.at).getTime() : 0));
      else ks = ks.slice().sort((a, b) => a.key.localeCompare(b.key) || (a.asset || '').localeCompare(b.asset || '') || a.source.localeCompare(b.source));
      return ks;
    }
    function cell(c, k) {
      if (c === 'source') { const [cls, lbl] = SRC[k.source] || ['', k.source]; return `<td><span class="badge ${cls}">${esc(lbl)}</span></td>`; }
      if (c === 'asset') return `<td>${k.asset_id ? `<a href="/assets/${esc(k.asset_id)}" class="notranslate">${esc(k.asset)}</a>` : esc(k.asset || '')}</td>`;
      if (c === 'device') return `<td>${k.device_id ? `<a href="/devices/${esc(k.device_id)}" class="notranslate">${esc(k.device)}</a>` : `<span class="muted">${esc(ctrl32.t('none'))}</span>`}</td>`;
      return '<td></td>';
    }
    function render() {
      const ks = visible(), U = ctrl32Widgets.util;
      $('#telTable tbody').innerHTML = ks.map((k) => { const l = k.last, unit = k.unit ? ' <span class="muted" style="font-weight:400">' + esc(k.unit) + '</span>' : '';
        const spark = U.sparkline((k.spark || []).map((p) => ({ t: p[0] * 1000, v: p[1] })), k.source === 'status' ? 'var(--muted)' : 'var(--primary)');
        const sub = cols.includes('asset') ? [k.quantity] : [k.asset, k.quantity];
        return `<tr class="clickable${T.sel && cfg.rowId(T.sel) === cfg.rowId(k) ? ' sel' : ''}" data-row="${esc(cfg.rowId(k))}"><td><div class="key">${esc(k.key)}</div><div class="sub">${esc(sub.filter(Boolean).join(' · '))}</div></td>
        ${cols.map((c) => cell(c, k)).join('')}<td class="tel-val">${esc(valText(l))}${unit}</td><td>${l ? qualBadge(l.quality) : ''}</td>
        <td>${l ? `<span data-rel="${esc(l.at)}" title="${esc(fmt(l.at))}">${esc(relTime(l.at))}</span><div class="sub">${esc(fmt(l.at))}</div>` : '<span class="muted">—</span>'}</td>
        <td>${spark || '<span class="muted">—</span>'}</td><td class="num">${numFmt(k.stats.min)}</td><td class="num">${numFmt(k.stats.max)}</td><td class="num">${numFmt(k.stats.avg)}${k.stats.faults ? `<div class="sub" title="sensor fault">⚠ ${k.stats.faults}</div>` : ''}</td></tr>`; }).join('')
        || `<tr><td colspan="${ncol}" class="muted">${esc(ctrl32.t(T.keys.length ? 'no key matches the search' : (cfg.empty || 'nothing yet')))}</td></tr>`;
      for (const tr of document.querySelectorAll('#telTable tr[data-row]')) tr.onclick = (e) => { if (e.target.closest('a')) return; const k = T.keys.find((x) => cfg.rowId(x) === tr.dataset.row); if (k) openKey(k); };
    }
    // live: observations of the datastreams; the page may listen to more (keys without a datastream, status)
    function live() {
      if (T.es) T.es.close();
      const es = new EventSource(cfg.streamURL(T.keys)); T.es = es;
      es.addEventListener('hello', () => { $('#telLive').textContent = ctrl32.t('live'); });
      es.onerror = () => { $('#telLive').textContent = ctrl32.t('reconnecting…'); };
      es.addEventListener('observation', (ev) => { const e = JSON.parse(ev.data); const k = T.keys.find((x) => x.datastream_id === e.datastream_id); if (!k) return;
        if (!k.last || new Date(e.measured_at) >= new Date(k.last.at)) k.last = { at: e.measured_at, value: e.value, quality: e.quality || 'ok' };
        T.mark(); render(); T.touched(k); });
      if (cfg.listen) cfg.listen(es, T);
    }
    T.mark = () => { T.liveAt = new Date(); $('#telLive').textContent = ctrl32.t('live') + ' · ' + T.liveAt.toLocaleTimeString(lang()); };
    T.touched = (k) => { if (!T.sel || cfg.rowId(T.sel) !== cfg.rowId(k) || T.d.range === 'custom') return; clearTimeout(T.reload); T.reload = setTimeout(() => { loadSeries(); if (T.page === 0) loadValues(); }, 3000); };
    // ---- one key: chart, values, annotations
    const detailSpan = () => { const d = T.d; if (d.range === 'custom' && d.from && d.to) return [d.from, d.to]; const now = Date.now(); return [now - MS[d.range], now + 60e3]; };
    function openKey(k) {
      T.sel = k; T.d = { range: T.range === 'custom' ? '24h' : T.range, agg: 'auto', from: null, to: null }; T.pages = ['']; T.page = 0; render();
      const [cls, lbl] = SRC[k.source] || ['', k.source], el = $('#telDetail'), canExp = ctrl32.can('data.export');
      el.hidden = false;
      el.innerHTML = `<div class="row"><h2 style="margin:0"><code>${esc(k.key)}</code></h2>${cols.includes('source') ? `<span class="badge ${cls}">${esc(lbl)}</span>` : ''}${k.asset_id ? `<a class="notranslate" href="/assets/${esc(k.asset_id)}">${esc(k.asset)}</a>` : ''}<span class="muted">${esc([k.asset_id ? '' : k.asset, k.quantity, k.unit].filter(Boolean).join(' · '))}</span>${k.device_id ? `<a class="muted notranslate" href="/devices/${esc(k.device_id)}">${esc(k.device)}</a>` : ''}<span class="grow"></span><button class="btn ghost icon" id="telClose" type="button" aria-label="Close">${ctrl32.ICON.close}</button></div>
        <div class="row" style="margin-top:10px;gap:10px;flex-wrap:wrap;align-items:end"><span class="tel-rng" id="telRng">${['1h', '24h', '7d', '30d'].map((r) => `<button class="btn small${r === T.d.range ? ' active' : ''}" type="button" data-r="${r}">${{ '1h': '1 h', '24h': '24 h', '7d': '7 d', '30d': '30 d' }[r]}</button>`).join('')}<button class="btn small" type="button" data-r="custom">Custom</button></span>
          <span id="telDCustom" class="row" hidden style="gap:6px;align-items:end"><label>From <input type="datetime-local" id="telDFrom"></label><label>To <input type="datetime-local" id="telDTo"></label><button class="btn small" type="button" id="telDApply">Apply</button></span>
          <label>Aggregation <select id="telAgg"><option value="auto">Automatic</option><option value="none">Raw values</option><option value="avg">Average</option><option value="min">Minimum</option><option value="max">Maximum</option></select></label>
          <span class="grow"></span>${canExp ? `<a class="btn small" id="telCsv" download>Export CSV</a><a class="btn small" id="telXlsx" download>Export Excel</a>` : ''}</div>
        <div id="telChart" style="margin-top:10px"><p class="muted">…</p></div><div id="telHover"></div><div class="muted" id="telStats" style="font-size:13px"></div>
        <div class="two" style="margin-top:12px"><div><h3 style="margin:0 0 6px">Values</h3><div class="table-wrap"><table><thead><tr><th>Measured</th><th>Value</th><th>Quality</th><th>Received</th><th>Annotation</th></tr></thead><tbody id="telRows"></tbody></table></div>
          <div class="row" style="margin-top:8px"><button class="btn small" type="button" id="telNewer">Newer</button><button class="btn small" type="button" id="telOlder">Older</button><span class="muted" id="telPage"></span></div></div>
          <div id="telSide"></div></div>`;
      $('#telClose').onclick = () => { el.hidden = true; T.sel = null; render(); };
      for (const b of el.querySelectorAll('#telRng [data-r]')) b.onclick = () => { for (const x of el.querySelectorAll('#telRng [data-r]')) x.classList.toggle('active', x === b); T.d.range = b.dataset.r; $('#telDCustom').hidden = b.dataset.r !== 'custom';
        if (b.dataset.r === 'custom') { const [f, t] = detailSpan(); $('#telDFrom').value = localInput(f); $('#telDTo').value = localInput(t); return; } refreshKey(); };
      $('#telDApply').onclick = () => { const f = new Date($('#telDFrom').value).getTime(), t = new Date($('#telDTo').value).getTime(); if (!(t > f)) { ctrl32.toast('to must be after from', 'bad'); return; } T.d.from = f; T.d.to = t; refreshKey(); };
      $('#telAgg').onchange = () => { T.d.agg = $('#telAgg').value; loadSeries(); };
      $('#telNewer').onclick = () => { if (T.page > 0) { T.page--; loadValues(); } };
      $('#telOlder').onclick = () => { if (T.pages[T.page + 1]) { T.page++; loadValues(); } };
      const side = $('#telSide');
      if (cfg.side && cfg.side(k, side, T) !== false) { /* the page drew it */ } else annotationSide(k, side);
      refreshKey(); el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
    function refreshKey() { T.pages = ['']; T.page = 0; const [f, t] = detailSpan(), q = `${cfg.keyQuery(T.sel)}&from=${isoOf(f)}&to=${isoOf(t)}`;
      if ($('#telCsv')) { $('#telCsv').href = `${cfg.base}/export?${q}&format=csv`; $('#telXlsx').href = `${cfg.base}/export?${q}&format=xlsx`; }
      loadSeries(); loadValues(); }
    async function loadSeries() {
      const [f, t] = detailSpan(), k = T.sel; if (!k) return;
      try { const s = await api('GET', `${cfg.base}/series?${cfg.keyQuery(k)}&from=${isoOf(f)}&to=${isoOf(t)}&agg=${T.d.agg}`); T.series = s; drawChart(s, f, t, k.unit);
        const st = s.stats; $('#telStats').innerHTML = `${ctrl32.t('Values')}: <b>${st.n}</b> · ${ctrl32.t('min')} <b>${numFmt(st.min)}</b> · ${ctrl32.t('max')} <b>${numFmt(st.max)}</b> · ${ctrl32.t('average')} <b>${numFmt(st.avg)}</b>${st.faults ? ` · <span style="color:var(--bad)">${ctrl32.t('sensor fault')}: ${st.faults}</span>` : ''}${s.bucket_s ? ` · ${ctrl32.t('aggregated per')} ${s.bucket_s >= 3600 ? s.bucket_s / 3600 + ' h' : s.bucket_s / 60 + ' min'}` : ''}${s.truncated ? ' · ' + ctrl32.t('only the newest raw messages were read') : ''}`;
        renderAnnotations(s.annotations || []);
      } catch (x) { $('#telChart').innerHTML = `<p class="muted">${esc(x.message)}</p>`; }
    }
    function drawChart(s, f, t, unit) {
      const box = $('#telChart'), W = Math.max(320, box.clientWidth || 800), H = 260, L = 58, R = 12, Tp = 10, B = 26, U = ctrl32Widgets.util;
      const pts = (s.points || []).map((p) => ({ t: new Date(p.t).getTime(), v: p.v, lo: p.min ?? p.v, hi: p.max ?? p.v, q: p.quality, n: p.n }));
      if (!pts.length) { box.innerHTML = `<p class="muted" style="padding:100px 0;text-align:center">${esc(ctrl32.t('no values in this range'))}</p>`; $('#telHover').textContent = ''; return; }
      // values from a faulty sensor do not set the scale (a probe reporting -127 would flatten the chart); they are marked
      let vs = pts.filter((p) => p.q !== 'sensor_fault' || s.bucket_s).flatMap((p) => [p.lo, p.hi, p.v]).filter((v) => v != null && isFinite(v));
      if (!vs.length) vs = pts.flatMap((p) => [p.lo, p.hi, p.v]).filter((v) => v != null && isFinite(v));
      if (!vs.length) vs = [0]; let lo = Math.min(...vs), hi = Math.max(...vs); if (lo === hi) { lo -= 1; hi += 1; } const pad = (hi - lo) * 0.06; lo -= pad; hi += pad;
      const yt = U.niceTicks(lo, hi, 5); lo = Math.min(lo, yt[0]); hi = Math.max(hi, yt[yt.length - 1]);
      const x = (tt) => L + (tt - f) / (t - f) * (W - L - R), y = (v) => Tp + (1 - (v - lo) / (hi - lo)) * (H - Tp - B);
      const tk = U.timeTicks(f, t, Math.max(3, Math.floor((W - L) / 110)));
      let svg = `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="chart">`;
      for (const v of yt) svg += `<line x1="${L}" x2="${W - R}" y1="${y(v).toFixed(1)}" y2="${y(v).toFixed(1)}" stroke="var(--line)"/><text x="${L - 6}" y="${(y(v) + 4).toFixed(1)}" text-anchor="end" font-size="11" fill="var(--muted)">${esc(numFmt(v))}</text>`;
      for (const tt of tk.ticks) svg += `<line x1="${x(tt).toFixed(1)}" x2="${x(tt).toFixed(1)}" y1="${Tp}" y2="${H - B}" stroke="var(--line)" stroke-dasharray="2 3"/><text x="${x(tt).toFixed(1)}" y="${H - 8}" text-anchor="middle" font-size="11" fill="var(--muted)">${esc(tk.fmt(tt))}</text>`;
      for (const a of (s.annotations || [])) { const x1 = Math.max(L, x(new Date(a.from).getTime())), x2 = Math.min(W - R, x(new Date(a.to).getTime())); if (x2 < L || x1 > W - R) continue;
        svg += `<rect x="${x1.toFixed(1)}" y="${Tp}" width="${Math.max(2, x2 - x1).toFixed(1)}" height="${H - Tp - B}" fill="var(--warn)" opacity="0.18"><title>${esc(fmt(a.from) + ' · ' + a.category.replace(/_/g, ' ') + ' · ' + a.text + ' (' + a.created_by + ')')}</title></rect>`; }
      if (s.bucket_s) { const band = pts.filter((p) => p.lo != null && p.hi != null); if (band.length > 1) svg += `<path d="${band.map((p, i) => (i ? 'L' : 'M') + x(p.t).toFixed(1) + ',' + y(p.hi).toFixed(1)).join(' ')} ${band.slice().reverse().map((p) => 'L' + x(p.t).toFixed(1) + ',' + y(p.lo).toFixed(1)).join(' ')} Z" fill="var(--primary)" opacity="0.12"/>`; }
      let d = '', pen = false; for (const p of pts) { if (p.v == null || p.q === 'sensor_fault' && !s.bucket_s) { pen = false; continue; } d += (pen ? 'L' : 'M') + x(p.t).toFixed(1) + ',' + y(p.v).toFixed(1) + ' '; pen = true; }
      svg += `<path d="${d}" fill="none" stroke="var(--primary)" stroke-width="1.8" vector-effect="non-scaling-stroke"/>`;
      if (!s.bucket_s && pts.length <= 300) for (const p of pts) if (p.v != null && p.q !== 'sensor_fault') svg += `<circle cx="${x(p.t).toFixed(1)}" cy="${y(p.v).toFixed(1)}" r="2.2" fill="var(--primary)"/>`;
      for (const p of pts) if (p.q === 'sensor_fault') { const yy = p.v != null && !s.bucket_s ? Math.max(Tp + 5, Math.min(H - B - 5, y(p.v))) : Tp + 6; svg += `<circle cx="${x(p.t).toFixed(1)}" cy="${yy.toFixed(1)}" r="4" fill="var(--bad)"><title>${esc(ctrl32.t('sensor fault') + ' · ' + fmt(p.t))}</title></circle>`; }
      svg += `<rect id="telHit" x="${L}" y="${Tp}" width="${W - L - R}" height="${H - Tp - B}" fill="transparent"/><line id="telCur" x1="0" x2="0" y1="${Tp}" y2="${H - B}" stroke="var(--muted)" visibility="hidden"/></svg>`;
      box.innerHTML = svg;
      const svgEl = box.querySelector('svg'), cur = box.querySelector('#telCur');
      const near = (ev) => { const r = svgEl.getBoundingClientRect(), px = (ev.clientX - r.left) * W / r.width, tt = f + (px - L) / (W - L - R) * (t - f); let best = null; for (const p of pts) if (!best || Math.abs(p.t - tt) < Math.abs(best.t - tt)) best = p; return best; };
      svgEl.onmousemove = (ev) => { const p = near(ev); if (!p) return; cur.setAttribute('x1', x(p.t)); cur.setAttribute('x2', x(p.t)); cur.setAttribute('visibility', 'visible');
        $('#telHover').textContent = `${fmt(p.t)} · ${p.v == null ? '—' : numFmt(p.v)} ${unit || ''}${s.bucket_s && p.lo != null ? ` (${numFmt(p.lo)} … ${numFmt(p.hi)}, n=${p.n})` : ''}${p.q === 'sensor_fault' ? ' · ' + ctrl32.t('sensor fault') : ''}`; };
      svgEl.onmouseleave = () => cur.setAttribute('visibility', 'hidden');
      svgEl.onclick = (ev) => { const p = near(ev), fr = document.getElementById('annForm'); if (p && fr) { fr.from.value = localInput(p.t); fr.to.value = ''; fr.text.focus(); } };
    }
    async function loadValues() {
      const [f, t] = detailSpan();
      try { const r = await api('GET', `${cfg.base}/values?${cfg.keyQuery(T.sel)}&from=${isoOf(f)}&to=${isoOf(t)}&limit=25${T.pages[T.page] ? '&cursor=' + encodeURIComponent(T.pages[T.page]) : ''}`);
        T.pages[T.page + 1] = r.next_cursor || ''; T.pages.length = T.page + 2;
        $('#telRows').innerHTML = r.rows.map((x) => `<tr><td>${fmt(x.at)}</td><td class="num"><b>${esc(valText(x))}</b></td><td>${qualBadge(x.quality)}</td><td>${x.received_at ? fmt(x.received_at) : '<span class="muted">—</span>'}</td><td style="white-space:normal">${esc(x.annotation || '')}</td></tr>`).join('') || '<tr><td colspan="5" class="muted">no values in this range</td></tr>';
        $('#telNewer').disabled = T.page === 0; $('#telOlder').disabled = !r.next_cursor; $('#telPage').textContent = ctrl32.t('Page') + ' ' + (T.page + 1);
      } catch (x) { $('#telRows').innerHTML = `<tr><td colspan="5" class="muted">${esc(x.message)}</td></tr>`; }
    }
    function annotationSide(k, side) {
      if (k.source !== 'datastream') { side.innerHTML = ''; return; }
      side.innerHTML = `<h3 style="margin:0 0 6px">Annotations</h3><div id="annList" class="muted">…</div>${ctrl32.can('data.annotate') ? `<form id="annForm" class="form" style="margin-top:8px"><div class="row"><label>From <input type="datetime-local" name="from" required></label><label>To (optional) <input type="datetime-local" name="to"></label></div>
        <label>Category <select name="category"><option value="note">Note</option><option value="expected_event">Expected event</option><option value="maintenance">Maintenance</option><option value="calibration">Calibration</option><option value="sensor_fault">Sensor fault</option><option value="data_issue">Data issue</option></select></label>
        <label>Comment <input name="text" required maxlength="1000" placeholder="e.g. door open for loading"></label><div class="err" id="annErr"></div><div><button class="btn small primary" type="submit">Add annotation</button></div>
        <p class="muted" style="margin:0;font-size:12px">An annotation never changes the data. It is shown on the chart and written to exports; click the chart to take a time.</p></form>` : ''}`;
      const fr = $('#annForm'); if (fr) { fr.from.value = localInput(Date.now() - 3600e3);
        fr.onsubmit = async (e) => { e.preventDefault(); $('#annErr').textContent = ''; const body = { datastream_id: k.datastream_id, from: new Date(fr.from.value).toISOString(), category: fr.category.value, text: fr.text.value }; if (fr.to.value) body.to = new Date(fr.to.value).toISOString();
          try { await api('POST', '/api/v1/annotations', body); ctrl32.toast('annotation added', 'ok'); fr.text.value = ''; loadSeries(); loadValues(); } catch (x) { $('#annErr').textContent = x.message; } }; }
    }
    function renderAnnotations(list) {
      const el = $('#annList'); if (!el) return;
      el.classList.toggle('muted', !list.length);
      el.innerHTML = list.map((a) => `<div class="tel-ann"><b>${esc(a.text)}</b> <span class="badge">${esc(ctrl32.t(a.category.replace(/_/g, ' ')))}</span><div class="muted" style="font-size:12px">${fmt(a.from)}${a.to !== a.from ? ' – ' + fmt(a.to) : ''} · ${esc(a.created_by)}${ctrl32.can('data.annotate') ? ` · <a href="#" data-retract="${a.id}">${esc(ctrl32.t('Retract'))}</a>` : ''}</div></div>`).join('') || esc(ctrl32.t('No annotations in this range.'));
      for (const b of el.querySelectorAll('[data-retract]')) b.onclick = async (e) => { e.preventDefault(); const reason = ctrl32.ask('Reason for retracting this annotation:'); if (!reason) return;
        try { await api('POST', `/api/v1/annotations/${b.dataset.retract}/retract`, { reason }); ctrl32.toast('annotation retracted', 'ok'); loadSeries(); loadValues(); } catch (x) { ctrl32.toast(x.message, 'bad'); } };
    }
    T.load = load; T.render = render; T.openKey = openKey;
    T.close = () => { if (T.es) T.es.close(); clearInterval(T.tick); };
    load();
    return T;
  }
  window.ctrl32Telemetry = { mount, numFmt, valText, qualBadge, relTime };
})();
