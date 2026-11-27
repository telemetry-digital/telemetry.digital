// ctrl32 telemetry — dashboard widgets of trends and batches (spec 043): the trend control of the Trends page with the
// bound datastreams as curves, and the batches of a unit. Registered with ctrl32Widgets.register.
(() => {
  const WG = window.ctrl32Widgets, TRD = window.ctrl32Trend; if (!WG || !TRD) return;
  const { esc, ids, api, P } = WG.util;
  const t = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
  const lang = () => document.documentElement.lang || undefined;
  const RANGES = ['5m', '15m', '1h', '6h', '24h', '7d'];

  // ---- trend ----
  // The control lives as long as its card: a refresh of the dashboard must not throw away the window the person moved
  // to, so the widget is built once and only told about new curves.
  WG.register('trend', async (el, p, ctx) => {
    const list = ids(ctx); const sig = list.join(',') + '|' + (p.window || '1h') + '|' + !!p.tools + '|' + !!p.table + '|' + !!p.limits;
    if (el._trend && el._trend.sig === sig && el._trend.root.isConnected) { el._trend.ctl.redraw(); return; }
    if (el._trend) el._trend.ctl.destroy();
    if (!list.length) { el.innerHTML = `<span class="muted">${esc(t('select at least one datastream'))}</span>`; return; }
    const pub = WG.isPublic(); let range = RANGES.includes(p.window) || TRD.RANGE_MS[p.window] ? p.window : '1h';
    el.innerHTML = `<div class="trw">${p.tools === false ? '' : `<div class="trw-bar">${[...new Set([...RANGES, range])].sort((a, b) => TRD.RANGE_MS[a] - TRD.RANGE_MS[b]).map((r) => `<button class="btn notranslate" type="button" data-r="${r}">${r.replace(/(\d+)/, '$1 ')}</button>`).join('')}<button class="btn" type="button" data-live>${esc(t('Live'))}</button><span class="grow"></span>${pub || ctx.edit ? '' : `<a class="btn" data-open href="#">${esc(t('Open in Trends'))}</a>`}</div>`}<div class="trw-chart"></div>${p.table ? '<div class="trw-table"></div>' : ''}</div>`;
    const root = el.querySelector('.trw'), bar = el.querySelector('.trw-bar'), tbl = el.querySelector('.trw-table');
    let all = []; try { all = await WG.datastreams(); } catch (e) { all = []; }
    const onChange = (st) => {
      if (bar) { for (const b of bar.querySelectorAll('[data-r]')) b.classList.toggle('on', st.live && b.dataset.r === range); bar.querySelector('[data-live]').classList.toggle('on', st.live);
        const o = bar.querySelector('[data-open]'); if (o) o.href = '/trends?ds=' + list.slice(0, 16).join(',') + (st.live ? '&range=' + range : '&from=' + encodeURIComponent(new Date(st.t0).toISOString()) + '&to=' + encodeURIComponent(new Date(st.t1).toISOString())); }
      if (tbl) { const at = st.hover != null ? 'cur' : 'a';
        tbl.innerHTML = `<table><thead><tr><th>${esc(t('Curve'))}</th><th class="num">${esc(t(st.hover != null ? 'At the pointer' : 'Ruler A'))}</th><th class="num">${esc(t('Minimum'))}</th><th class="num">${esc(t('Maximum'))}</th><th class="num">${esc(t('Average'))}</th></tr></thead><tbody>${st.rows.map((r) => `<tr><td class="notranslate"><span class="trw-sw" style="background:${esc(r.color)}"></span>${esc(r.name)}${r.unit ? ' [' + esc(r.unit) + ']' : ''}</td><td class="num">${TRD.fmtNum(r[at], r.dec)}</td><td class="num">${r.stats ? TRD.fmtNum(r.stats.min, r.dec) : '—'}</td><td class="num">${r.stats ? TRD.fmtNum(r.stats.max, r.dec) : '—'}</td><td class="num">${r.stats ? TRD.fmtNum(r.stats.avg, r.dec) : '—'}</td></tr>`).join('')}</tbody></table>`; }
    };
    const ctl = TRD.create(el.querySelector('.trw-chart'), { curves: list.slice(0, 16).map((id, i) => ({ datastream_id: id, color: TRD.PALETTE[i % TRD.PALETTE.length] })), range, datastreams: all, limits: !!p.limits && !pub, aux: !pub,
      api: (m, u) => api(m, u), path: P, onChange });
    if (bar) { for (const b of bar.querySelectorAll('[data-r]')) b.onclick = (e) => { e.stopPropagation(); range = b.dataset.r; ctl.setRange(range); };
      bar.querySelector('[data-live]').onclick = (e) => { e.stopPropagation(); ctl.setLive(!ctl.state().live); }; }
    el._trend = { ctl, sig, root };
  }, { label: 'Trend', needsDs: true });

  // ---- batches ----
  const dur = (s) => { s = Math.max(0, Math.round(s)); const h = Math.floor(s / 3600), m = Math.floor(s % 3600 / 60); return (h ? h + ' h ' : '') + m + ' min'; };
  const when = (x) => (x ? new Date(x).toLocaleString(lang(), { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—');
  WG.register('batches', async (el, p, ctx) => {
    if (WG.isPublic()) { el.innerHTML = `<span class="muted">${esc(t('Batches are shown to signed-in users.'))}</span>`; return; }
    const rows = p.rows == null ? 6 : p.rows; let list;
    try { list = await api('GET', '/api/v1/batches?limit=' + Math.max(rows + 5, 10) + (p.batch_unit ? '&unit=' + encodeURIComponent(p.batch_unit) : '')); } catch (e) { el.innerHTML = `<span class="muted">${esc(t(e.message))}</span>`; return; }
    const link = (b, inner) => (ctx.edit ? inner : `<a href="/batches?id=${esc(b.id)}">${inner}</a>`);
    const run = list.filter((b) => b.status === 'running');
    let h = '';
    if (p.running !== false) h += run.length ? run.map((b) => `<div class="btw-now"><span class="notranslate">${esc(b.unit)}</span>${link(b, `<b class="notranslate">${b.number}${b.code ? ' · ' + esc(b.code) : ''}</b>`)}${b.recipe ? `<span class="notranslate">${esc(b.recipe + ' / ' + b.record)}</span>` : ''}<span class="muted">${esc(t('since'))} ${esc(when(b.started_at))} · ${esc(dur(b.seconds))}</span></div>`).join('')
      : `<div class="btw-now idle"><span class="muted">${esc(t('No batch is running.'))}</span></div>`;
    const past = (p.running !== false ? list.filter((b) => b.status !== 'running') : list).slice(0, rows);
    if (rows > 0) h += `<div class="table-wrap"><table><thead><tr>${p.batch_unit ? '' : `<th>${esc(t('Unit'))}</th>`}<th>${esc(t('No.'))}</th><th>${esc(t('Code'))}</th><th>${esc(t('Began'))}</th><th style="text-align:right">${esc(t('Lasted'))}</th></tr></thead><tbody>${past.map((b) => `<tr>${p.batch_unit ? '' : `<td class="notranslate">${esc(b.unit)}</td>`}<td>${link(b, String(b.number))}</td><td class="notranslate">${esc(b.code || (b.recipe ? b.record : ''))}</td><td>${esc(when(b.started_at))}</td><td style="text-align:right">${esc(dur(b.seconds))}</td></tr>`).join('') || `<tr><td colspan="5" class="muted">${esc(t('no batches yet'))}</td></tr>`}</tbody></table></div>`;
    el.innerHTML = h;
  }, { label: 'Batches' });
})();
