// ctrl32 telemetry — dashboard filter bar (0.68.0, screen format 1.1). A dashboard defines filters (spec.filters:
// site, asset type, device, quantity, an attribute, free-text search, time range); the bar shows them on top of the
// dashboard and widgets that follow the filters are narrowed by the chosen values. Values live in the URL
// (f.site=…&f.q=…&f.range=…&f.a.<key>=…, so a link shows the same view) and are remembered per viewer in this browser;
// the URL wins over the remembered values, those over the dashboard's defaults. On a public link only the filters the
// owner marked public are shown; the server applies the defaults of the others.
// 0.73.0: a "value" filter is a value the viewer types (a limit such as 8 °C) that key filters compare with, and
// ctrl32DashStates keeps the dashboard state (drill-down view and its current entity) in the address.
(function () {
  'use strict';
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const T = (s) => (window.ctrl32 && ctrl32.t ? ctrl32.t(s) : s);
  const WG = () => window.ctrl32Widgets;
  const RANGES = ['1h', '6h', '8h', '12h', 'today', '24h', '7d', '30d', '90d'];
  const LABEL = { site: 'Site', asset_type: 'Asset type', device: 'Device', quantity: 'Quantity', search: 'Search', range: 'Range' };
  const fid = (d) => (d.kind === 'search' ? 'q' : d.kind === 'attribute' ? 'a.' + d.key : d.kind === 'value' ? 'v.' + d.key : d.kind);
  const facetKind = (d) => (d.kind === 'attribute' ? 'attr:' + d.key : d.kind);
  const labelOf = (d) => d.label || (d.kind === 'attribute' || d.kind === 'value' ? String(d.key).replace(/[_.-]+/g, ' ') : T(LABEL[d.kind] || d.kind));
  const SINGLE = new Set(['search', 'range', 'value']);
  const PICK = new Set(['site', 'asset_type', 'device', 'quantity', 'attribute']);
  const getV = (vals, d) => (d.kind === 'search' ? vals.q : d.kind === 'range' ? vals.range : d.kind === 'value' ? ((vals.values || {})[d.key] || '') : d.kind === 'attribute' ? (vals.attr[d.key] || []) : (vals[d.kind] || []));
  function setV(vals, d, v) { if (d.kind === 'search') vals.q = v; else if (d.kind === 'range') vals.range = v; else if (d.kind === 'value') { vals.values = vals.values || {}; vals.values[d.key] = v; } else if (d.kind === 'attribute') vals.attr[d.key] = v; else vals[d.kind] = v; }
  const clone = (v) => JSON.parse(JSON.stringify(v));
  const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
  function storeGet(key) { try { return JSON.parse(localStorage.getItem('ctrl32.df.' + key) || 'null'); } catch (e) { return null; } }
  function storePut(key, v) { try { localStorage.setItem('ctrl32.df.' + key, JSON.stringify(v)); } catch (e) { /* private mode */ } }

  // initial values: URL > remembered > defaults (only for the filters the dashboard has)
  function initial(spec, key, usable) {
    const vals = WG().defaultFilters(spec); const url = new URLSearchParams(location.search); const mem = key ? storeGet(key) : null;
    for (const d of usable) {
      const id = 'f.' + fid(d);
      if (url.has(id)) { const all = url.getAll(id).filter((x) => x !== ''); setV(vals, d, SINGLE.has(d.kind) ? (all[0] || '') : (d.multi ? all : all.slice(0, 1))); continue; }
      if (mem && mem[fid(d)] !== undefined) setV(vals, d, mem[fid(d)]);
    }
    if (vals.range && !RANGES.includes(vals.range)) vals.range = '';
    return vals;
  }
  function writeURL(spec, vals, usable) {
    const url = new URLSearchParams(location.search); const defs = WG().defaultFilters(spec);
    for (const k of [...url.keys()]) if (k.startsWith('f.')) url.delete(k);
    for (const d of usable) { const v = getV(vals, d); if (same(v, getV(defs, d))) continue; for (const x of [].concat(v)) url.append('f.' + fid(d), x); if (Array.isArray(v) && !v.length) url.append('f.' + fid(d), ''); }
    const qs = url.toString(); history.replaceState(null, '', location.pathname + (qs ? '?' + qs : '') + location.hash);
  }

  // mount(host, spec, {key, public, onChange(values), editor}) → {values, refresh()}
  function mount(host, spec, opts) {
    opts = opts || {}; const pub = !!opts.public;
    const usable = (spec.filters || []).filter((d) => !pub || d.public);
    const ctl = { values: initial(spec, opts.key, usable) };
    if (!usable.length) { host.hidden = true; host.innerHTML = ''; return ctl; }
    host.hidden = false; host.classList.add('df-bar');
    let facets = {}; let qTimer = null;
    const changed = (redraw) => { if (!opts.editor) writeURL(spec, ctl.values, usable); if (opts.key) { const m = {}; for (const d of usable) m[fid(d)] = getV(ctl.values, d); storePut(opts.key, m); } if (redraw !== false) load(); if (opts.onChange) opts.onChange(ctl.values); };
    async function load() {
      const kinds = usable.filter((d) => PICK.has(d.kind)).map(facetKind);
      if (kinds.length) {
        const q = new URLSearchParams({ kinds: kinds.join(',') }); const v = ctl.values;
        for (const k of ['site', 'asset_type', 'quantity', 'device']) if ((v[k] || []).length) q.set('f_' + k, v[k].join(','));
        for (const [k, list] of Object.entries(v.attr || {})) for (const x of list || []) q.append('f_attr', k + '=' + x);
        if (v.q) q.set('f_q', v.q);
        try { facets = await ctrl32.api('GET', WG().apiPath('/api/v1/datastreams/facets') + '?' + q); } catch (e) { facets = {}; }
        // asset types and quantities are codes: shown as words in the user's language, the value (URL) stays the code
        for (const k of ['asset_type', 'quantity']) for (const o of facets[k] || []) o.label = WG().codeLabel(o.value);
      }
      draw();
    }
    function summary(d, v) {
      if (!v.length) return T('All');
      const opts2 = facets[facetKind(d)] || [];
      const names = v.map((x) => { const o = opts2.find((o) => o.value === x); return o ? o.label : (d.kind === 'site' || d.kind === 'device' ? '…' : d.kind === 'asset_type' || d.kind === 'quantity' ? WG().codeLabel(x) : x); });
      return names.length > 2 ? names.length + ' ' + T('selected') : names.join(', ');
    }
    function draw() {
      const v = ctl.values; const defs = WG().defaultFilters(spec);
      let h = '';
      for (const d of usable) {
        const i = usable.indexOf(d);
        if (PICK.has(d.kind)) {
          const cur = getV(v, d); const list = facets[facetKind(d)] || [];
          // values chosen but no longer offered (narrowed away) stay visible so they can be unticked
          const shown = list.concat(cur.filter((x) => !list.some((o) => o.value === x)).map((x) => ({ value: x, label: x, n: 0 })));
          h += `<details class="df-pick${cur.length ? ' set' : ''}" data-i="${i}"><summary><span class="df-label">${esc(labelOf(d))}:</span> <b class="notranslate">${esc(summary(d, cur))}</b> ▾</summary><div class="df-menu">${shown.length > 8 ? `<input type="search" placeholder="${esc(T('Filter…'))}" data-find>` : ''}${d.multi ? '' : `<label><input type="radio" name="df${i}" value="" ${cur.length ? '' : 'checked'}> ${esc(T('All'))}</label>`}${shown.map((o) => `<label class="notranslate" data-txt="${esc(String(o.label).toLowerCase())}"><input type="${d.multi ? 'checkbox' : 'radio'}" name="df${i}" value="${esc(o.value)}" ${cur.includes(o.value) ? 'checked' : ''}> ${esc(o.label)} <span class="muted">${o.n || ''}</span></label>`).join('') || `<span class="muted">${esc(T('nothing to choose'))}</span>`}</div></details>`;
        } else if (d.kind === 'search') {
          h += `<input type="search" class="df-q" data-i="${i}" placeholder="${esc(d.label || T('Search…'))}" value="${esc(v.q || '')}" aria-label="${esc(labelOf(d))}">`;
        } else if (d.kind === 'value') {
          h += `<label class="df-val">${esc(labelOf(d))} <input type="text" inputmode="decimal" class="df-v" data-i="${i}" value="${esc(getV(v, d))}" maxlength="80" aria-label="${esc(labelOf(d))}"></label>`;
        } else if (d.kind === 'range') {
          h += `<label class="df-label" style="display:inline-flex;flex-direction:row;align-items:center;gap:4px">${esc(labelOf(d))} <select class="df-range" data-i="${i}">${RANGES.map((r) => `<option value="${r}" ${r === (v.range || spec.range) ? 'selected' : ''}>${esc(WG().rangeLabel(r))}</option>`).join('')}</select></label>`;
        }
      }
      const dirty = usable.some((d) => !same(getV(v, d), getV(defs, d)));
      h += '<span style="flex:1"></span>';
      if (dirty) h += `<button type="button" class="btn small ghost" data-reset>${esc(T('Clear filters'))}</button>`;
      if (!opts.editor && navigator.clipboard) h += `<button type="button" class="btn small ghost" data-copy title="${esc(T('Copy a link to this view'))}">${esc(T('Copy link'))}</button>`;
      const open = [...host.querySelectorAll('details[open]')].map((x) => x.dataset.i);
      host.innerHTML = h;
      for (const x of host.querySelectorAll('details')) if (open.includes(x.dataset.i)) x.open = true;
      for (const det of host.querySelectorAll('details.df-pick')) {
        const d = usable[+det.dataset.i];
        det.addEventListener('toggle', () => { if (det.open) for (const o of host.querySelectorAll('details[open]')) if (o !== det) o.open = false; const f = det.querySelector('[data-find]'); if (det.open && f) f.focus(); });
        for (const inp of det.querySelectorAll('input[name]')) inp.onchange = () => { const val = d.multi ? [...det.querySelectorAll('input[name]:checked')].map((x) => x.value) : (inp.value ? [inp.value] : []); setV(ctl.values, d, val); if (!d.multi) det.open = false; changed(); };
        const f = det.querySelector('[data-find]'); if (f) f.oninput = () => { const t = f.value.toLowerCase(); for (const l of det.querySelectorAll('label[data-txt]')) l.hidden = t && !l.dataset.txt.includes(t); };
      }
      const q = host.querySelector('.df-q'); if (q) q.oninput = () => { clearTimeout(qTimer); qTimer = setTimeout(() => { setV(ctl.values, usable[+q.dataset.i], q.value.trim()); changed(false); }, 400); };
      for (const vi of host.querySelectorAll('.df-v')) vi.oninput = () => { clearTimeout(qTimer); qTimer = setTimeout(() => { setV(ctl.values, usable[+vi.dataset.i], vi.value.trim()); changed(false); }, 500); };
      const r = host.querySelector('.df-range'); if (r) r.onchange = () => { ctl.values.range = r.value; changed(false); };
      const rs = host.querySelector('[data-reset]'); if (rs) rs.onclick = () => { const d0 = WG().defaultFilters(spec); for (const d of usable) setV(ctl.values, d, clone(getV(d0, d))); changed(); };
      const cp = host.querySelector('[data-copy]'); if (cp) cp.onclick = async () => { try { await navigator.clipboard.writeText(location.href); if (ctrl32.toast) ctrl32.toast('link copied', 'ok'); } catch (e) { /* no clipboard */ } };
    }
    document.addEventListener('click', (e) => { if (!host.contains(e.target)) for (const o of host.querySelectorAll('details[open]')) o.open = false; });
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') for (const o of host.querySelectorAll('details[open]')) o.open = false; });
    ctl.refresh = load;
    draw(); load();
    return ctl;
  }
  window.ctrl32DashFilters = { mount, fid, labelOf, RANGES };

  // ---- dashboard states (0.73.0) -----------------------------------------------------------------------------------
  // A dashboard with states shows one state (view) at a time; widget actions open another state with an entity as its
  // "current entity", and the entity selector changes the current entity of the state shown. The stack of states
  // lives in the address — st=<state>.<type>.<id>.<name> per level, an empty state for the root — so a link opens the
  // same view; Back (the button or the browser's) goes up one level. The breadcrumb shows the levels.
  // mountStates(host, spec, {onChange, editor}) → nav {state, current, stack, go(state, entity), setEntity(entity),
  // back(level), setState(state)}; the editor keeps nothing in the address.
  function mountStates(host, spec, opts) {
    opts = opts || {}; const states = spec.states || []; const root = states.length ? states[0].id : '';
    const has = (id) => (states.length ? states.some((s) => s.id === id) : id === '');
    const nameOf = (id) => ((states.find((s) => s.id === id) || {}).name) || '';
    const parse = () => {
      const stack = [{ state: root, entity: null }];
      for (const t of new URLSearchParams(location.search).getAll('st')) {
        const parts = t.split('.'); const sid = parts[0] || root; if (!has(sid)) continue;
        const ent = parts[1] && /^(device|asset|datastream|site)$/.test(parts[1]) && /^[0-9a-f-]{36}$/.test(parts[2] || '') ? { type: parts[1], id: parts[2], name: parts.slice(3).join('.') } : null;
        if (sid === root) stack[0] = { state: root, entity: ent }; else stack.push({ state: sid, entity: ent });
      }
      return stack;
    };
    const changed = (push) => {
      if (!opts.editor) {
        const u = new URLSearchParams(location.search); u.delete('st');
        nav.stack.forEach((x, i) => { if (i === 0 && !x.entity) return; u.append('st', (i === 0 ? '' : x.state) + (x.entity ? '.' + x.entity.type + '.' + x.entity.id + '.' + (x.entity.name || '') : '')); });
        const url = location.pathname + (u.toString() ? '?' + u : '') + location.hash;
        if (push) history.pushState({ dst: 1 }, '', url); else history.replaceState(history.state, '', url);
      }
      draw(); if (opts.onChange) opts.onChange(nav);
    };
    const nav = {
      stack: opts.editor ? [{ state: root, entity: null }] : parse(), states, root,
      get state() { return this.stack[this.stack.length - 1].state; },
      get current() { return this.stack[this.stack.length - 1].entity; },
      go(state, ent) {
        if (!has(state)) return;
        if (opts.editor) { this.stack = [{ state, entity: ent || null }]; changed(false); return; }
        const top = this.stack[this.stack.length - 1];
        if (state === root) this.stack = [{ state: root, entity: ent || this.stack[0].entity }];
        else if (top.state === state) top.entity = ent || null;
        else { const i = this.stack.findIndex((x) => x.state === state); if (i > 0) this.stack = this.stack.slice(0, i); this.stack.push({ state, entity: ent || null }); }
        changed(true);
      },
      setEntity(ent) { const top = this.stack[this.stack.length - 1]; const k = (e) => (e ? e.type + ':' + e.id : ''); if (k(top.entity) === k(ent)) return; top.entity = ent || null; changed(false); },
      back(i) { if (i == null) i = this.stack.length - 2; if (i < 0) return; this.stack = this.stack.slice(0, i + 1); changed(true); },
      setState(state) { if (!has(state)) return; this.stack = [{ state, entity: this.current }]; changed(false); },
    };
    function draw() {
      if (!host) return;
      const show = !opts.editor && nav.stack.length > 1; host.hidden = !show; if (!show) { host.innerHTML = ''; return; }
      host.classList.add('ds-crumbs');
      host.innerHTML = `<button type="button" class="btn small ghost" data-back>← ${esc(T('Back'))}</button>` + nav.stack.map((x, i) => {
        const label = (nameOf(x.state) || T('Overview')) + (x.entity && x.entity.name ? ': ' + x.entity.name : '');
        return (i ? '<span class="sep">›</span>' : '') + (i < nav.stack.length - 1 ? `<a class="notranslate" data-lv="${i}" tabindex="0">${esc(label)}</a>` : `<b class="notranslate">${esc(label)}</b>`);
      }).join('');
      host.querySelector('[data-back]').onclick = () => nav.back();
      for (const a of host.querySelectorAll('[data-lv]')) { a.onclick = () => nav.back(+a.dataset.lv); a.onkeydown = (e) => { if (e.key === 'Enter') nav.back(+a.dataset.lv); }; }
    }
    if (!opts.editor) window.addEventListener('popstate', () => { nav.stack = parse(); draw(); if (opts.onChange) opts.onChange(nav); });
    draw();
    return nav;
  }
  window.ctrl32DashStates = { mount: mountStates };
})();
