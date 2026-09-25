// ctrl32 telemetry — SCADA picture editor (spec 001, US2/US3/US4). Edits the drawing layer (`spec.elements`) and the
// widgets of a canvas screen in place: tools (select, rect, ellipse, line, polyline, polygon, pipe, text, symbols),
// selection (click, shift, rubber band, groups), move, resize, rotate, point editing, snap, zoom, undo/redo,
// clipboard, align/distribute, z-order, lock/hide, layers, and a property panel with bindings, dynamics and events.
// The model is the screen spec itself; rendering goes through widgets.js/scada.js, the editor only adds an overlay.
(function () {
  'use strict';
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const SC = () => window.ctrl32Scada, WG = () => window.ctrl32Widgets;
  const clone = (o) => JSON.parse(JSON.stringify(o));
  const r2 = (v) => Math.round(v * 100) / 100;
  const colRe = /^(#[0-9a-fA-F]{3,8}|none)$/;
  const newId = (prefix, taken) => { let id; do { id = prefix + Math.random().toString(36).slice(2, 7); } while (taken.has(id)); return id; };
  const POINTS = new Set(['line', 'polyline', 'polygon', 'pipe']);
  // Tabler-style outline icons (24×24, stroke 2), drawn for this editor.
  const I = (d) => `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`;
  const ICON = {
    select: I('<path d="M7 7l10 3.5-4.5 1.5-1.5 4.5z"/><path d="M13 13l5 5"/>'), rect: I('<rect x="4" y="6" width="16" height="12" rx="1"/>'), ellipse: I('<ellipse cx="12" cy="12" rx="8" ry="6"/>'),
    line: I('<path d="M5 19L19 5"/>'), polyline: I('<path d="M4 18l5-8 5 5 6-10"/>'), polygon: I('<path d="M12 4l8 6-3 9H7L4 10z"/>'), pipe: I('<path d="M3 8h9v10h9" stroke-width="4"/>'), text: I('<path d="M6 6h12M12 6v13M9 19h6"/>'),
    undo: I('<path d="M9 14L4 9l5-5"/><path d="M4 9h11a5 5 0 010 10h-1"/>'), redo: I('<path d="M15 14l5-5-5-5"/><path d="M20 9H9a5 5 0 000 10h1"/>'),
    copy: I('<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 00-2-2H6a2 2 0 00-2 2v8a2 2 0 002 2h2"/>'), paste: I('<path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="1"/>'),
    dup: I('<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M14 11v6M11 14h6"/><path d="M16 8V6a2 2 0 00-2-2H6a2 2 0 00-2 2v8a2 2 0 002 2h2"/>'), del: I('<path d="M4 7h16M10 11v6M14 11v6M5 7l1 12a2 2 0 002 2h8a2 2 0 002-2l1-12M9 7V4h6v3"/>'),
    al: I('<path d="M4 4v16M8 8h12M8 16h7"/>'), ac: I('<path d="M12 4v16M6 8h12M8 16h8"/>'), ar: I('<path d="M20 4v16M4 8h12M9 16h7"/>'), at: I('<path d="M4 4h16M8 8v12M16 8v7"/>'), am: I('<path d="M4 12h16M8 6v12M16 8v8"/>'), ab: I('<path d="M4 20h16M8 4v12M16 9v7"/>'),
    dh: I('<path d="M4 4v16M20 4v16M10 8v8M14 8v8"/>'), dv: I('<path d="M4 4h16M4 20h16M8 10h8M8 14h8"/>'), size: I('<rect x="4" y="4" width="7" height="7"/><rect x="13" y="13" width="7" height="7"/>'),
    front: I('<rect x="8" y="8" width="12" height="12" rx="1" fill="currentColor" fill-opacity=".25"/><path d="M4 16V5a1 1 0 011-1h11"/>'), back: I('<rect x="4" y="4" width="12" height="12" rx="1" fill="currentColor" fill-opacity=".25"/><path d="M20 8v11a1 1 0 01-1 1H8"/>'),
    fwd: I('<path d="M12 5v14M7 10l5-5 5 5"/>'), bwd: I('<path d="M12 5v14M7 14l5 5 5-5"/>'), group: I('<rect x="3" y="3" width="8" height="8"/><rect x="13" y="13" width="8" height="8"/><path d="M11 7h4v4M13 17H9v-4"/>'), ungroup: I('<rect x="3" y="3" width="8" height="8"/><rect x="13" y="13" width="8" height="8"/>'),
    lock: I('<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 018 0v4"/>'), eye: I('<circle cx="12" cy="12" r="2"/><path d="M22 12c-2.7 4.7-6 7-10 7s-7.3-2.3-10-7c2.7-4.7 6-7 10-7s7.3 2.3 10 7"/>'),
    grid: I('<path d="M4 4h16v16H4zM4 12h16M12 4v16"/>'), snap: I('<path d="M4 4h4M4 4v4M20 4h-4M20 4v4M4 20h4M4 20v-4M20 20h-4M20 20v-4"/><circle cx="12" cy="12" r="2"/>'),
    zin: I('<circle cx="10" cy="10" r="7"/><path d="M21 21l-6-6M7 10h6M10 7v6"/>'), zout: I('<circle cx="10" cy="10" r="7"/><path d="M21 21l-6-6M7 10h6"/>'), fit: I('<path d="M4 8V4h4M20 8V4h-4M4 16v4h4M20 16v4h-4"/>'),
  };
  const DYN = [
    ['fill', 'Fill colour', 'color'], ['stroke', 'Line colour', 'color'], ['color', 'Text colour', 'color'], ['state', 'State', 'state'], ['text', 'Text', 'text'],
    ['visible', 'Visible when', 'cond'], ['blink', 'Blink when', 'cond'], ['flow', 'Flow when', 'cond'], ['rotate', 'Rotate when', 'cond'],
    ['level', 'Level (fill 0–1)', 'lin'], ['opacity', 'Opacity (0–1)', 'lin'], ['angle', 'Angle (°)', 'lin'], ['move_x', 'Move X (px)', 'lin'], ['move_y', 'Move Y (px)', 'lin'],
  ];
  const DYNKIND = Object.fromEntries(DYN.map((d) => [d[0], d[2]]));
  const OPS = ['>', '>=', '<', '<=', '==', '!='];
  const STATE_NAMES = { run: 'running', stop: 'stopped', fault: 'fault', manual: 'manual', open: 'open', closed: 'closed', transit: 'in transit', off: 'off', warn: 'warning' };

  function create(o) {
    const host = o.host, spec = o.spec; spec.elements = spec.elements || []; spec.widgets = spec.widgets || [];
    let Z = 1, tool = 'select', symbolTool = null, SEL = [], drag = null, drawing = null, showGrid = true, snapOn = true, clip = null, pasteN = 0;
    const W = () => spec.layout.width || 1600, H = () => spec.layout.height || 900, GRID = () => spec.layout.grid || 10;
    const hist = { stack: [], idx: -1 };
    host.innerHTML = `<div class="se">
      <div class="se-top" role="toolbar" aria-label="Editor tools">
        ${btn('undo', 'Undo (Ctrl+Z)')}${btn('redo', 'Redo (Ctrl+Y)')}<span class="se-sep"></span>
        ${btn('copy', 'Copy (Ctrl+C)')}${btn('paste', 'Paste (Ctrl+V)')}${btn('dup', 'Duplicate (Ctrl+D)')}${btn('del', 'Delete (Del)')}<span class="se-sep"></span>
        ${btn('al', 'Align left')}${btn('ac', 'Align centre')}${btn('ar', 'Align right')}${btn('at', 'Align top')}${btn('am', 'Align middle')}${btn('ab', 'Align bottom')}${btn('dh', 'Distribute horizontally')}${btn('dv', 'Distribute vertically')}${btn('size', 'Same size as the first selected')}<span class="se-sep"></span>
        ${btn('front', 'Bring to front')}${btn('fwd', 'Bring forward')}${btn('bwd', 'Send backward')}${btn('back', 'Send to back')}<span class="se-sep"></span>
        ${btn('group', 'Group (Ctrl+G)')}${btn('ungroup', 'Ungroup (Ctrl+Shift+G)')}${btn('lock', 'Lock / unlock')}${btn('eye', 'Hide in operation / show')}<span class="se-sep"></span>
        ${btn('grid', 'Show grid', true)}${btn('snap', 'Snap to grid', true)}<span class="se-sep"></span>
        ${btn('zout', 'Zoom out (-)')}<span class="se-zoom" title="Zoom">100%</span>${btn('zin', 'Zoom in (+)')}${btn('fit', 'Fit (0)')}
      </div>
      <div class="se-main">
        <aside class="se-lib">
          <div class="se-tools">${['select', 'rect', 'ellipse', 'line', 'polyline', 'polygon', 'pipe', 'text'].map((t) => `<button type="button" class="se-tool${t === 'select' ? ' on' : ''}" data-tool="${t}" title="${esc({ select: 'Select (V)', rect: 'Rectangle (R)', ellipse: 'Ellipse (E)', line: 'Line (L)', polyline: 'Polyline (P)', polygon: 'Polygon (G)', pipe: 'Pipe (I)', text: 'Text (T)' }[t])}">${ICON[t]}</button>`).join('')}</div>
          <input class="se-search" type="search" placeholder="Search symbols…">
          <div class="se-syms"></div>
        </aside>
        <div class="se-view"><div class="se-stage"></div></div>
        <aside class="se-side">
          <div class="se-tabs"><button type="button" class="on" data-tab="props">Properties</button><button type="button" data-tab="layers">Layers</button></div>
          <div class="se-props props"></div><div class="se-layers" hidden></div>
        </aside>
      </div>
      <div class="se-status"><span class="se-xy">0, 0</span><span class="se-info"></span><span class="grow"></span><span class="se-hint muted"></span></div>
    </div>`;
    function btn(k, title, toggle) { return `<button type="button" class="se-btn${toggle ? ' on' : ''}" data-cmd="${k}" title="${esc(title)}" aria-label="${esc(title)}">${ICON[k]}</button>`; }
    const $ = (s) => host.querySelector(s), stage = $('.se-stage'), view = $('.se-view'), props = $('.se-props'), layers = $('.se-layers');
    let mimic = null, ov = null, selG = null, layer = null;

    // ---- rendering ---------------------------------------------------------------------------------------------
    async function render() {
      stage._fit = applyZoom; // keeps widgets.js from installing its own fit-to-width handler
      await WG().render(stage, spec, { range: spec.range, edit: true, live: true });
      mimic = stage.querySelector('.mimic'); layer = stage._scada;
      for (const c of stage.querySelectorAll('.mimic-widget')) c.classList.add('editing');
      ov = document.createElementNS('http://www.w3.org/2000/svg', 'svg'); ov.setAttribute('class', 'se-ov');
      ov.innerHTML = `<defs><pattern id="se-grid" width="${GRID()}" height="${GRID()}" patternUnits="userSpaceOnUse"><path d="M${GRID()} 0H0V${GRID()}" fill="none" stroke="rgba(15,23,42,.13)" stroke-width="${1 / Z}"/></pattern></defs><rect class="se-gridrect" width="${W()}" height="${H()}" fill="url(#se-grid)"/><g class="se-sel"></g>`;
      ov.style.cssText = `position:absolute;left:0;top:0;width:${W()}px;height:${H()}px;max-width:none;z-index:5;pointer-events:none;overflow:visible`;
      mimic.appendChild(ov); selG = ov.querySelector('.se-sel');
      applyZoom(); bindCanvas(); drawOverlay(); renderLayers();
    }
    function applyZoom() {
      if (!mimic) return; const wrap = stage.querySelector('.mimic-wrap');
      mimic.style.transform = `scale(${Z})`; wrap.style.width = W() * Z + 'px'; wrap.style.height = H() * Z + 'px';
      $('.se-zoom').textContent = Math.round(Z * 100) + '%';
      const g = ov && ov.querySelector('.se-gridrect'); if (g) { g.style.display = showGrid ? '' : 'none'; ov.querySelector('#se-grid path').setAttribute('stroke-width', String(1 / Z)); }
      drawOverlay();
    }
    function zoomTo(z, keepCenter) {
      const cx = (view.scrollLeft + view.clientWidth / 2) / Z, cy = (view.scrollTop + view.clientHeight / 2) / Z;
      Z = Math.max(0.25, Math.min(4, z)); applyZoom();
      if (keepCenter !== false) { view.scrollLeft = cx * Z - view.clientWidth / 2; view.scrollTop = cy * Z - view.clientHeight / 2; }
    }
    const fit = () => zoomTo(Math.min((view.clientWidth - 24) / W(), (view.clientHeight - 24) / H()), false);
    function drawLayer() { if (layer) { layer.setElements(spec.elements); layer.setSpec(spec); layer.draw(); } }
    let refreshTimer = null;
    const refreshValues = () => { clearTimeout(refreshTimer); refreshTimer = setTimeout(() => layer && layer.refresh(), 300); };
    function placeWidgets() { for (const c of stage.querySelectorAll('.mimic-widget')) { const w = spec.widgets[+c.dataset.i]; if (w) WG().placeCanvas(c, w.geometry); } }

    // ---- model helpers -------------------------------------------------------------------------------------------
    const el = (id) => spec.elements.find((e) => e.id === id);
    const keyOf = (k) => k.split(':'); // ['e', id] | ['w', index]
    function bbOf(k) { const [t, v] = keyOf(k); if (t === 'w') { const g = spec.widgets[+v].geometry; return { x: g.x, y: g.y, w: g.w, h: g.h }; } const e = el(v); return e ? SC().bbox(e) : { x: 0, y: 0, w: 0, h: 0 }; }
    function union(keys) { const bs = keys.map(bbOf); const x = Math.min(...bs.map((b) => b.x)), y = Math.min(...bs.map((b) => b.y)); return { x, y, w: Math.max(...bs.map((b) => b.x + b.w)) - x, h: Math.max(...bs.map((b) => b.y + b.h)) - y }; }
    function syncBox(e) { if (e.points && e.points.length) { const b = SC().bbox(e); e.x = r2(b.x); e.y = r2(b.y); e.w = r2(b.w); e.h = r2(b.h); } }
    function setBB(k, nb, ob) {
      const [t, v] = keyOf(k);
      if (t === 'w') { const g = spec.widgets[+v].geometry; g.x = Math.max(0, Math.round(nb.x)); g.y = Math.max(0, Math.round(nb.y)); g.w = Math.max(20, Math.round(nb.w)); g.h = Math.max(20, Math.round(nb.h)); if (g.x + g.w > W()) g.x = Math.max(0, W() - g.w); if (g.y + g.h > H()) g.y = Math.max(0, H() - g.h); return; }
      const e = el(v); if (!e) return;
      if (e.points && e.points.length && ob) { const sx = ob.w ? nb.w / ob.w : 1, sy = ob.h ? nb.h / ob.h : 1; e.points = e.points.map((p) => [r2(nb.x + (p[0] - ob.x) * sx), r2(nb.y + (p[1] - ob.y) * sy)]); syncBox(e); return; }
      e.x = r2(nb.x); e.y = r2(nb.y); e.w = r2(Math.max(1, nb.w)); e.h = r2(Math.max(1, nb.h));
    }
    const snap = (v, force) => (snapOn || force) ? Math.round(v / GRID()) * GRID() : r2(v);
    function expandGroups(keys) { const out = new Set(keys); for (const k of keys) { const [t, v] = keyOf(k); if (t !== 'e') continue; const e = el(v); if (e && e.group) spec.elements.forEach((x) => { if (x.group === e.group) out.add('e:' + x.id); }); } return [...out]; }
    const selEls = () => SEL.filter((k) => k[0] === 'e').map((k) => el(k.slice(2))).filter(Boolean);
    const takenIds = () => new Set(spec.elements.map((e) => e.id).concat(spec.widgets.map((w) => w.id)));

    // ---- history -------------------------------------------------------------------------------------------------
    const snapshot = () => JSON.stringify({ e: spec.elements, w: spec.widgets, l: spec.layout, s: spec.scheme || '' });
    function commit(initial) {
      const s = snapshot(); if (hist.stack[hist.idx] === s) return;
      hist.stack = hist.stack.slice(0, hist.idx + 1); hist.stack.push(s); if (hist.stack.length > 101) hist.stack.shift(); hist.idx = hist.stack.length - 1;
      if (!initial) o.onDirty && o.onDirty(); toolbarState(); renderLayers();
    }
    async function restore(s) {
      const d = JSON.parse(s); const widgetsChanged = JSON.stringify(spec.widgets) !== JSON.stringify(d.w) || JSON.stringify(spec.layout) !== JSON.stringify(d.l);
      spec.elements = d.e; spec.widgets = d.w; spec.layout = d.l; if (d.s) spec.scheme = d.s; else delete spec.scheme;
      SEL = SEL.filter((k) => (k[0] === 'e' ? !!el(k.slice(2)) : +k.slice(2) < spec.widgets.length));
      if (widgetsChanged) await render(); else { drawLayer(); drawOverlay(); renderLayers(); }
      renderProps(); toolbarState(); o.onDirty && o.onDirty();
    }
    const undo = () => { if (hist.idx > 0) restore(hist.stack[--hist.idx]); };
    const redo = () => { if (hist.idx < hist.stack.length - 1) restore(hist.stack[++hist.idx]); };

    // ---- overlay: selection frames and handles --------------------------------------------------------------------
    const HS = () => 8 / Z;
    function handleRect(x, y, name, cursor) { const s = HS(); return `<rect data-seh="${name}" x="${r2(x - s / 2)}" y="${r2(y - s / 2)}" width="${r2(s)}" height="${r2(s)}" fill="#fff" stroke="#2563eb" stroke-width="${r2(1.2 / Z)}" style="pointer-events:all;cursor:${cursor}"/>`; }
    function drawOverlay() {
      if (!selG) return; let s = ''; const sw = r2(1.2 / Z), dash = `${r2(4 / Z)} ${r2(3 / Z)}`;
      for (const k of SEL) {
        const b = bbOf(k); const [t, v] = keyOf(k); const e = t === 'e' ? el(v) : null; const rot = e && e.r ? ` transform="rotate(${e.r} ${r2(b.x + b.w / 2)} ${r2(b.y + b.h / 2)})"` : '';
        s += `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(Math.max(b.w, 1))}" height="${r2(Math.max(b.h, 1))}" fill="none" stroke="${e && e.locked ? '#9ca3af' : '#2563eb'}" stroke-width="${sw}" stroke-dasharray="${dash}"${rot}/>`;
      }
      const movable = SEL.length && !selEls().some((e) => e.locked);
      if (movable && !drawing) {
        const b = SEL.length === 1 ? bbOf(SEL[0]) : union(SEL); const single = SEL.length === 1 ? keyOf(SEL[0]) : null; const e = single && single[0] === 'e' ? el(single[1]) : null;
        const cx = b.x + b.w / 2, cy = b.y + b.h / 2; const rot = e && e.r ? ` transform="rotate(${e.r} ${r2(cx)} ${r2(cy)})"` : '';
        if (SEL.length > 1) s += `<rect x="${r2(b.x)}" y="${r2(b.y)}" width="${r2(b.w)}" height="${r2(b.h)}" fill="none" stroke="#2563eb" stroke-width="${sw}"/>`;
        let hs = '';
        const H8 = [['nw', b.x, b.y, 'nwse-resize'], ['n', cx, b.y, 'ns-resize'], ['ne', b.x + b.w, b.y, 'nesw-resize'], ['e', b.x + b.w, cy, 'ew-resize'], ['se', b.x + b.w, b.y + b.h, 'nwse-resize'], ['s', cx, b.y + b.h, 'ns-resize'], ['sw', b.x, b.y + b.h, 'nesw-resize'], ['w', b.x, cy, 'ew-resize']];
        if (!(e && e.type === 'line')) for (const [n, x, y, c] of H8) hs += handleRect(x, y, n, c);
        if (e && e.type !== 'line') hs += `<line x1="${r2(cx)}" y1="${r2(b.y)}" x2="${r2(cx)}" y2="${r2(b.y - 22 / Z)}" stroke="#2563eb" stroke-width="${sw}"/><circle data-seh="rot" cx="${r2(cx)}" cy="${r2(b.y - 22 / Z)}" r="${r2(5 / Z)}" fill="#fff" stroke="#2563eb" stroke-width="${sw}" style="pointer-events:all;cursor:grab"/>`;
        s += `<g${rot}>${hs}</g>`;
        if (e && e.points) s += e.points.map((p, i) => `<circle data-seh="p:${i}" cx="${r2(p[0])}" cy="${r2(p[1])}" r="${r2(4.5 / Z)}" fill="#2563eb" stroke="#fff" stroke-width="${r2(1.5 / Z)}" style="pointer-events:all;cursor:move"/>`).join('');
      }
      if (drag && drag.kind === 'rubber') { const r = drag.rect; s += `<rect x="${r2(r.x)}" y="${r2(r.y)}" width="${r2(r.w)}" height="${r2(r.h)}" fill="rgba(37,99,235,.08)" stroke="#2563eb" stroke-width="${sw}" stroke-dasharray="${dash}"/>`; }
      if (drawing) s += drawing.preview || '';
      selG.innerHTML = s;
      const info = SEL.length === 1 ? describe(SEL[0]) : SEL.length ? `${SEL.length} objects` : '';
      $('.se-info').textContent = info;
    }
    function describe(k) { const [t, v] = keyOf(k); if (t === 'w') { const w = spec.widgets[+v]; return `widget ${w.type} ${w.geometry.w}×${w.geometry.h} at ${w.geometry.x},${w.geometry.y}`; } const e = el(v); if (!e) return ''; const b = SC().bbox(e); return `${e.name || e.id} · ${e.type === 'symbol' ? (SC().symbols[e.symbol] || {}).name || e.symbol : e.type} · ${Math.round(b.w)}×${Math.round(b.h)} at ${Math.round(b.x)},${Math.round(b.y)}${e.r ? ' · ' + e.r + '°' : ''}`; }
    function select(keys) { SEL = keys; drawOverlay(); renderProps(); renderLayers(); toolbarState(); }

    // ---- pointer interaction --------------------------------------------------------------------------------------
    function pt(ev) { const r = mimic.getBoundingClientRect(); return { x: (ev.clientX - r.left) / Z, y: (ev.clientY - r.top) / Z }; }
    function hitKey(target) {
      const card = target.closest('.mimic-widget'); if (card && card.closest('.mimic') === mimic) return 'w:' + card.dataset.i;
      const g = target.closest('.sc-root [data-eid]'); if (g) { const e = el(g.dataset.eid); if (e && !e.locked) return 'e:' + e.id; }
      return null;
    }
    function bindCanvas() {
      mimic.addEventListener('pointerdown', onDown, true);
      mimic.addEventListener('dblclick', onDbl, true);
      mimic.addEventListener('pointermove', (ev) => { const p = pt(ev); $('.se-xy').textContent = Math.round(p.x) + ', ' + Math.round(p.y); if (drawing && drawing.poly) { drawing.cursor = constrain(drawing.pts[drawing.pts.length - 1], p, ev); updatePolyPreview(); } });
      mimic.addEventListener('dragover', (ev) => { if (ev.dataTransfer.types.includes('text/x-ctrl32-symbol')) ev.preventDefault(); });
      mimic.addEventListener('drop', (ev) => { const id = ev.dataTransfer.getData('text/x-ctrl32-symbol'); if (!id) return; ev.preventDefault(); placeSymbol(id, pt(ev)); });
    }
    function onDown(ev) {
      if (ev.button !== 0) return; ev.preventDefault(); ev.stopPropagation(); host.focus({ preventScroll: true });
      const p = pt(ev);
      if (tool !== 'select') { drawDown(p, ev); return; }
      const h = ev.target.closest('[data-seh]'); if (h) { startHandle(h.dataset.seh, p, ev); return; }
      const k = hitKey(ev.target);
      if (k) {
        const keys = ev.altKey ? [k] : expandGroups([k]);
        if (ev.shiftKey || ev.ctrlKey || ev.metaKey) { const has = keys.every((x) => SEL.includes(x)); select(has ? SEL.filter((x) => !keys.includes(x)) : [...new Set(SEL.concat(keys))]); return; }
        const wasSelected = SEL.includes(k); if (!wasSelected) select(keys);
        startMove(p, ev); if (drag && wasSelected && SEL.length > keys.length) drag.narrowTo = keys; return;
      }
      if (!ev.shiftKey) select([]);
      drag = { kind: 'rubber', p0: p, rect: { x: p.x, y: p.y, w: 0, h: 0 }, add: ev.shiftKey ? SEL.slice() : [] };
      follow(ev);
    }
    function follow(ev) {
      try { mimic.setPointerCapture(ev.pointerId); } catch (_) { }
      const move = (e2) => onMove(e2), up = (e2) => { mimic.removeEventListener('pointermove', move); mimic.removeEventListener('pointerup', up); mimic.removeEventListener('pointercancel', up); onUp(e2); };
      mimic.addEventListener('pointermove', move); mimic.addEventListener('pointerup', up); mimic.addEventListener('pointercancel', up);
    }
    function startMove(p, ev) {
      if (selEls().some((e) => e.locked)) return;
      drag = { kind: 'move', p0: p, orig: SEL.map((k) => [k, bbOf(k), clone(k[0] === 'e' ? el(k.slice(2)) : spec.widgets[+k.slice(2)].geometry)]), ub: union(SEL), moved: false };
      follow(ev);
    }
    function startHandle(h, p, ev) {
      if (h === 'rot') { const k = SEL[0]; const b = bbOf(k); drag = { kind: 'rot', k, c: { x: b.x + b.w / 2, y: b.y + b.h / 2 } }; }
      else if (h.startsWith('p:')) { drag = { kind: 'point', k: SEL[0], i: +h.slice(2) }; if (ev.altKey) { const e = el(SEL[0].slice(2)); const min = e.type === 'polygon' ? 3 : 2; if (e.points.length > min) { e.points.splice(drag.i, 1); syncBox(e); drawLayer(); drawOverlay(); commit(); renderProps(); } drag = null; return; } }
      else drag = { kind: 'resize', h, ub: SEL.length === 1 ? bbOf(SEL[0]) : union(SEL), orig: SEL.map((k) => [k, bbOf(k)]) };
      follow(ev);
    }
    function onMove(ev) {
      if (!drag) { if (drawing && !drawing.poly) drawMove(pt(ev), ev); return; }
      const p = pt(ev);
      if (drag.kind === 'move') {
        let dx = p.x - drag.p0.x, dy = p.y - drag.p0.y; if (ev.shiftKey) { if (Math.abs(dx) > Math.abs(dy)) dy = 0; else dx = 0; }
        dx = snap(drag.ub.x + dx) - drag.ub.x; dy = snap(drag.ub.y + dy) - drag.ub.y;
        if (dx || dy) drag.moved = true;
        for (const [k, , g] of drag.orig) {
          if (k[0] === 'w') { const w = spec.widgets[+k.slice(2)].geometry; w.x = Math.max(0, Math.min(W() - w.w, Math.round(g.x + dx))); w.y = Math.max(0, Math.min(H() - w.h, Math.round(g.y + dy))); continue; }
          const e = el(k.slice(2)); if (g.points) { e.points = g.points.map((q) => [r2(q[0] + dx), r2(q[1] + dy)]); syncBox(e); } else { e.x = r2(g.x + dx); e.y = r2(g.y + dy); }
        }
        drawLayer(); placeWidgets(); drawOverlay();
      } else if (drag.kind === 'resize') {
        const u = drag.ub; let x0 = u.x, y0 = u.y, x1 = u.x + u.w, y1 = u.y + u.h; const h = drag.h; const sx = ev.altKey ? r2 : (v) => snap(v);
        if (h.includes('w')) x0 = Math.min(sx(p.x), x1 - 2); if (h.includes('e')) x1 = Math.max(sx(p.x), x0 + 2);
        if (h.includes('n')) y0 = Math.min(sx(p.y), y1 - 2); if (h.includes('s')) y1 = Math.max(sx(p.y), y0 + 2);
        if (ev.shiftKey && u.w && u.h) { const ratio = u.w / u.h; if (h === 'n' || h === 's') { const w = (y1 - y0) * ratio; x0 = u.x + (u.w - w) / 2; x1 = x0 + w; } else { const hh = (x1 - x0) / ratio; if (h.includes('n')) y0 = y1 - hh; else y1 = y0 + hh; } }
        const nb = { x: x0, y: y0, w: x1 - x0, h: y1 - y0 }, fx = u.w ? nb.w / u.w : 1, fy = u.h ? nb.h / u.h : 1;
        for (const [k, b] of drag.orig) setBB(k, { x: nb.x + (b.x - u.x) * fx, y: nb.y + (b.y - u.y) * fy, w: b.w * fx, h: b.h * fy }, bbOf(k));
        drawLayer(); placeWidgets(); drawOverlay();
      } else if (drag.kind === 'rot') {
        const e = el(drag.k.slice(2)); let a = Math.atan2(p.y - drag.c.y, p.x - drag.c.x) * 180 / Math.PI + 90; if (a > 180) a -= 360;
        a = ev.shiftKey ? Math.round(a) : Math.round(a / 15) * 15; if (a === -180) a = 180; if (a) e.r = a; else delete e.r; drawLayer(); drawOverlay();
      } else if (drag.kind === 'point') {
        const e = el(drag.k.slice(2)); const q = ev.altKey ? { x: r2(p.x), y: r2(p.y) } : { x: snap(p.x), y: snap(p.y) }; e.points[drag.i] = [q.x, q.y]; syncBox(e); drawLayer(); drawOverlay();
      } else if (drag.kind === 'rubber') {
        const a = drag.p0; drag.rect = { x: Math.min(a.x, p.x), y: Math.min(a.y, p.y), w: Math.abs(p.x - a.x), h: Math.abs(p.y - a.y) }; drawOverlay();
      }
    }
    function onUp(ev) {
      if (!drag) { if (drawing && !drawing.poly) drawUp(pt(ev), ev); return; }
      const d = drag; drag = null;
      if (d.kind === 'rubber') {
        const r = d.rect; if (r.w < 2 && r.h < 2) { drawOverlay(); return; }
        const inside = (b) => b.x >= r.x && b.y >= r.y && b.x + b.w <= r.x + r.w && b.y + b.h <= r.y + r.h;
        const keys = spec.elements.filter((e) => !e.locked && inside(SC().bbox(e))).map((e) => 'e:' + e.id).concat(spec.widgets.map((w, i) => 'w:' + i).filter((k) => inside(bbOf(k))));
        select([...new Set(d.add.concat(expandGroups(keys)))]); return;
      }
      if (d.kind === 'move' && !d.moved) { if (d.narrowTo) select(d.narrowTo); else drawOverlay(); return; } // a click without movement picks one object out of a multi-selection
      commit(); renderProps(); drawOverlay();
    }
    function onDbl(ev) {
      ev.preventDefault(); ev.stopPropagation();
      if (drawing && drawing.poly) { finishPoly(); return; }
      if (tool !== 'select' || SEL.length !== 1 || SEL[0][0] !== 'e') return;
      const e = el(SEL[0].slice(2)); if (!e || !e.points || e.locked) return;
      // Add a point on the nearest segment.
      const p = pt(ev); let best = -1, bd = Infinity;
      for (let i = 0; i < e.points.length - 1 + (e.type === 'polygon' ? 1 : 0); i++) { const a = e.points[i], b = e.points[(i + 1) % e.points.length]; const d = segDist(p, a, b); if (d < bd) { bd = d; best = i; } }
      if (best >= 0 && bd < 12 / Z && e.points.length < 200) { e.points.splice(best + 1, 0, [snap(p.x), snap(p.y)]); syncBox(e); drawLayer(); drawOverlay(); commit(); renderProps(); }
    }
    function segDist(p, a, b) { const dx = b[0] - a[0], dy = b[1] - a[1]; const l = dx * dx + dy * dy; let t = l ? ((p.x - a[0]) * dx + (p.y - a[1]) * dy) / l : 0; t = Math.max(0, Math.min(1, t)); return Math.hypot(p.x - (a[0] + t * dx), p.y - (a[1] + t * dy)); }

    // ---- drawing tools --------------------------------------------------------------------------------------------
    const DEF_STYLE = { rect: { fill: '#e2e8f0', stroke: '#334155', sw: 1 }, ellipse: { fill: '#e2e8f0', stroke: '#334155', sw: 1 }, line: { stroke: '#334155', sw: 2 }, polyline: { stroke: '#334155', sw: 2 }, polygon: { fill: '#e2e8f0', stroke: '#334155', sw: 1 }, pipe: { sw: 12, medium: '#94a3b8' }, text: { font: 16, color: '#111827' } };
    function setTool(t, sym) {
      if (drawing) cancelDraw(); tool = t; symbolTool = sym || null;
      host.querySelectorAll('.se-tool').forEach((b) => b.classList.toggle('on', b.dataset.tool === t));
      host.querySelectorAll('.se-sym').forEach((b) => b.classList.toggle('on', t === 'symbol' && b.dataset.sym === sym));
      if (mimic) mimic.style.cursor = t === 'select' ? '' : 'crosshair';
      $('.se-hint').textContent = { select: 'Click to select, Shift to add, drag an empty area for a rubber band, Alt ignores groups.', rect: 'Drag to draw a rectangle.', ellipse: 'Drag to draw an ellipse.', line: 'Drag to draw a line.', polyline: 'Click the points, double-click or Enter to finish, Esc to cancel. Shift: horizontal/vertical.', polygon: 'Click the corners, double-click or Enter to finish, Esc to cancel.', pipe: 'Click the corners (horizontal/vertical, Alt for free angles), double-click or Enter to finish.', text: 'Drag the text box.', symbol: 'Click on the canvas to place the symbol.' }[t] || '';
    }
    const addEl = (e) => { e.id = e.id || newId('e', takenIds()); spec.elements.push(e); return e; };
    function placeSymbol(id, p) {
      const d = SC().symbols[id]; if (!d) return;
      const e = addEl({ type: 'symbol', symbol: id, x: snap(p.x - d.w / 2), y: snap(p.y - d.h / 2), w: d.w, h: d.h, name: '', style: { shade: true } });
      drawLayer(); commit(); select(['e:' + e.id]); if (!window.event || !window.event.shiftKey) setTool('select');
    }
    function constrain(from, p, ev) { const ortho = drawing && drawing.type === 'pipe' ? !ev.altKey : ev.shiftKey; let x = snap(p.x), y = snap(p.y); if (ortho && from) { if (Math.abs(x - from[0]) > Math.abs(y - from[1])) y = from[1]; else x = from[0]; } return [x, y]; }
    function drawDown(p, ev) {
      if (tool === 'symbol') { placeSymbol(symbolTool, p); return; }
      if (['polyline', 'polygon', 'pipe'].includes(tool)) {
        if (!drawing) drawing = { poly: true, type: tool, pts: [[snap(p.x), snap(p.y)]], cursor: null };
        else { const q = constrain(drawing.pts[drawing.pts.length - 1], p, ev); const last = drawing.pts[drawing.pts.length - 1]; if (q[0] !== last[0] || q[1] !== last[1]) drawing.pts.push(q); }
        updatePolyPreview(); return;
      }
      drawing = { type: tool, p0: { x: snap(p.x), y: snap(p.y) }, p1: { x: snap(p.x), y: snap(p.y) } }; follow(ev);
    }
    function drawMove(p, ev) { drawing.p1 = tool === 'line' ? (() => { const q = constrain([drawing.p0.x, drawing.p0.y], p, ev); return { x: q[0], y: q[1] }; })() : { x: snap(p.x), y: snap(p.y) }; const a = drawing.p0, b = drawing.p1, sw = r2(1.2 / Z);
      drawing.preview = drawing.type === 'line' ? `<line x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}" stroke="#2563eb" stroke-width="${sw * 2}"/>` : drawing.type === 'ellipse' ? `<ellipse cx="${(a.x + b.x) / 2}" cy="${(a.y + b.y) / 2}" rx="${Math.abs(b.x - a.x) / 2}" ry="${Math.abs(b.y - a.y) / 2}" fill="rgba(37,99,235,.1)" stroke="#2563eb" stroke-width="${sw}"/>` : `<rect x="${Math.min(a.x, b.x)}" y="${Math.min(a.y, b.y)}" width="${Math.abs(b.x - a.x)}" height="${Math.abs(b.y - a.y)}" fill="rgba(37,99,235,.1)" stroke="#2563eb" stroke-width="${sw}"/>`; drawOverlay(); }
    function drawUp(p, ev) {
      const d = drawing; drawing = null; if (!d) return; const a = d.p0, b = d.p1; let e;
      if (d.type === 'line') { const b2 = (a.x === b.x && a.y === b.y) ? { x: a.x + 100, y: a.y } : b; e = addEl({ type: 'line', points: [[a.x, a.y], [b2.x, b2.y]], style: clone(DEF_STYLE.line) }); syncBox(e); }
      else { let x = Math.min(a.x, b.x), y = Math.min(a.y, b.y), w = Math.abs(b.x - a.x), h = Math.abs(b.y - a.y); if (w < 4 && h < 4) { w = d.type === 'text' ? 200 : 120; h = d.type === 'text' ? 30 : 60; }
        e = addEl({ type: d.type, x, y, w, h, style: clone(DEF_STYLE[d.type]) }); if (d.type === 'text') e.text = 'Text'; }
      drawLayer(); commit(); if (!ev.shiftKey) setTool('select'); select(['e:' + e.id]);
    }
    function updatePolyPreview() { const d = drawing; const pts = d.pts.concat(d.cursor ? [d.cursor] : []); const sw = r2(1.2 / Z); d.preview = `<polyline points="${pts.map((q) => q.join(',')).join(' ')}" fill="${d.type === 'polygon' ? 'rgba(37,99,235,.1)' : 'none'}" stroke="#2563eb" stroke-width="${d.type === 'pipe' ? 12 : sw * 2}" stroke-opacity="${d.type === 'pipe' ? 0.35 : 1}" stroke-linejoin="round"/>` + d.pts.map((q) => `<circle cx="${q[0]}" cy="${q[1]}" r="${r2(3 / Z)}" fill="#2563eb"/>`).join(''); drawOverlay(); }
    function finishPoly() {
      const d = drawing; drawing = null; const min = d.type === 'polygon' ? 3 : 2;
      if (d.pts.length < min) { drawOverlay(); return; }
      const e = addEl({ type: d.type, points: d.pts, style: clone(DEF_STYLE[d.type]) }); syncBox(e); drawLayer(); commit(); setTool('select'); select(['e:' + e.id]);
    }
    function cancelDraw() { drawing = null; drawOverlay(); }

    // ---- commands -------------------------------------------------------------------------------------------------
    function del() { if (!SEL.length) return; const ids = new Set(SEL.filter((k) => k[0] === 'e').map((k) => k.slice(2))); const wi = new Set(SEL.filter((k) => k[0] === 'w').map((k) => +k.slice(2)));
      spec.elements = spec.elements.filter((e) => !ids.has(e.id)); const hadW = wi.size; spec.widgets = spec.widgets.filter((w, i) => !wi.has(i)); SEL = []; commit(); if (hadW) render().then(renderProps); else { drawLayer(); drawOverlay(); renderProps(); } }
    function copy() { if (!SEL.length) return; clip = { e: selEls().map(clone), w: SEL.filter((k) => k[0] === 'w').map((k) => clone(spec.widgets[+k.slice(2)])) }; pasteN = 0; try { localStorage.setItem('ctrl32.scada.clip', JSON.stringify(clip)); } catch (_) { } }
    async function paste() {
      let c = clip; if (!c) { try { c = JSON.parse(localStorage.getItem('ctrl32.scada.clip') || 'null'); } catch (_) { c = null; } } if (!c) return;
      pasteN++; const off = GRID() * pasteN, taken = takenIds(), groups = {}; const keys = [];
      for (const src of c.e) { const e = clone(src); e.id = newId('e', taken); taken.add(e.id); if (e.group) e.group = groups[e.group] || (groups[e.group] = newId('g', taken)); if (e.points) e.points = e.points.map((q) => [q[0] + off, q[1] + off]); e.x = (e.x || 0) + off; e.y = (e.y || 0) + off; spec.elements.push(e); keys.push('e:' + e.id); }
      for (const src of c.w || []) { const w = clone(src); w.id = newId('w', taken); taken.add(w.id); w.geometry.x = Math.min(W() - w.geometry.w, w.geometry.x + off); w.geometry.y = Math.min(H() - w.geometry.h, w.geometry.y + off); spec.widgets.push(w); keys.push('w:' + (spec.widgets.length - 1)); }
      commit(); if ((c.w || []).length) await render(); else drawLayer(); select(keys);
    }
    const dup = () => { copy(); paste(); };
    function align(kind) {
      if (SEL.length < 2) return; const u = union(SEL);
      for (const k of SEL) { const b = bbOf(k); const nb = { ...b };
        if (kind === 'al') nb.x = u.x; if (kind === 'ar') nb.x = u.x + u.w - b.w; if (kind === 'ac') nb.x = u.x + (u.w - b.w) / 2;
        if (kind === 'at') nb.y = u.y; if (kind === 'ab') nb.y = u.y + u.h - b.h; if (kind === 'am') nb.y = u.y + (u.h - b.h) / 2; setBB(k, nb, b); }
      after();
    }
    function distribute(axis) {
      if (SEL.length < 3) return; const items = SEL.map((k) => [k, bbOf(k)]).sort((a, b) => axis === 'h' ? a[1].x - b[1].x : a[1].y - b[1].y);
      const first = items[0][1], last = items[items.length - 1][1]; const total = items.reduce((s, [, b]) => s + (axis === 'h' ? b.w : b.h), 0);
      const gap = ((axis === 'h' ? last.x + last.w - first.x : last.y + last.h - first.y) - total) / (items.length - 1); let pos = axis === 'h' ? first.x : first.y;
      for (const [k, b] of items) { setBB(k, axis === 'h' ? { ...b, x: pos } : { ...b, y: pos }, b); pos += (axis === 'h' ? b.w : b.h) + gap; }
      after();
    }
    function sameSize() { if (SEL.length < 2) return; const r = bbOf(SEL[0]); for (const k of SEL.slice(1)) { const b = bbOf(k); setBB(k, { x: b.x, y: b.y, w: r.w, h: r.h }, b); } after(); }
    function zorder(kind) {
      const ids = new Set(selEls().map((e) => e.id)); if (!ids.size) return; let list = spec.elements;
      if (kind === 'front') list = list.filter((e) => !ids.has(e.id)).concat(list.filter((e) => ids.has(e.id)));
      else if (kind === 'back') list = list.filter((e) => ids.has(e.id)).concat(list.filter((e) => !ids.has(e.id)));
      else { list = list.slice(); const dir = kind === 'fwd' ? 1 : -1; const idx = list.map((e, i) => (ids.has(e.id) ? i : -1)).filter((i) => i >= 0); if (dir > 0) idx.reverse();
        for (const i of idx) { const j = i + dir; if (j < 0 || j >= list.length || ids.has(list[j].id)) continue; [list[i], list[j]] = [list[j], list[i]]; } }
      spec.elements = list; after();
    }
    function group(on) { const es = selEls(); if (on) { if (es.length < 2) return; const g = newId('g', takenIds()); es.forEach((e) => (e.group = g)); } else es.forEach((e) => delete e.group); commit(); renderProps(); renderLayers(); }
    function toggle(flag) { const es = selEls(); if (!es.length) return; const v = !es.every((e) => e[flag]); es.forEach((e) => { if (v) e[flag] = true; else delete e[flag]; }); drawLayer(); drawOverlay(); commit(); renderProps(); }
    function after() { drawLayer(); placeWidgets(); drawOverlay(); commit(); renderProps(); }
    function nudge(dx, dy) { if (!SEL.length || selEls().some((e) => e.locked)) return; for (const k of SEL) { const b = bbOf(k); setBB(k, { ...b, x: b.x + dx, y: b.y + dy }, b); } after(); }
    function toolbarState() {
      const has = SEL.length > 0, many = SEL.length > 1, els = selEls().length;
      const en = { undo: hist.idx > 0, redo: hist.idx < hist.stack.length - 1, copy: has, dup: has, del: has, paste: true, al: many, ac: many, ar: many, at: many, am: many, ab: many, dh: SEL.length > 2, dv: SEL.length > 2, size: many, front: els > 0, back: els > 0, fwd: els > 0, bwd: els > 0, group: els > 1, ungroup: selEls().some((e) => e.group), lock: els > 0, eye: els > 0 };
      host.querySelectorAll('[data-cmd]').forEach((b) => { if (b.dataset.cmd in en) b.disabled = !en[b.dataset.cmd]; });
    }
    const CMD = { undo, redo, copy, paste, dup, del, al: () => align('al'), ac: () => align('ac'), ar: () => align('ar'), at: () => align('at'), am: () => align('am'), ab: () => align('ab'), dh: () => distribute('h'), dv: () => distribute('v'), size: sameSize,
      front: () => zorder('front'), back: () => zorder('back'), fwd: () => zorder('fwd'), bwd: () => zorder('bwd'), group: () => group(true), ungroup: () => group(false), lock: () => toggle('locked'), eye: () => toggle('hidden'),
      grid: () => { showGrid = !showGrid; $('[data-cmd=grid]').classList.toggle('on', showGrid); applyZoom(); }, snap: () => { snapOn = !snapOn; $('[data-cmd=snap]').classList.toggle('on', snapOn); },
      zin: () => zoomTo(Z * 1.25), zout: () => zoomTo(Z / 1.25), fit };
    // Toolbar buttons hand the keyboard back to the canvas, so Enter/Delete/arrows act on the picture, not on a button.
    const refocus = () => host.focus({ preventScroll: true });
    host.querySelector('.se-top').addEventListener('click', (ev) => { const b = ev.target.closest('[data-cmd]'); if (b && !b.disabled) { CMD[b.dataset.cmd](); refocus(); } });
    host.querySelector('.se-tools').addEventListener('click', (ev) => { const b = ev.target.closest('[data-tool]'); if (b) { setTool(b.dataset.tool); refocus(); } });
    view.addEventListener('wheel', (ev) => { if (!ev.ctrlKey) return; ev.preventDefault(); zoomTo(ev.deltaY < 0 ? Z * 1.1 : Z / 1.1); }, { passive: false });
    host.tabIndex = -1;
    function onKey(ev) {
      if (!host.isConnected || host.closest('[hidden]')) return;
      const t = ev.target; if (t && (t.closest('input,textarea,select,[contenteditable]') || t.closest('dialog'))) return;
      const k = ev.key, mod = ev.ctrlKey || ev.metaKey;
      const run = (f) => { ev.preventDefault(); f(); };
      if (mod && (k === 'z' || k === 'Z') && !ev.shiftKey) return run(undo);
      if (mod && (k === 'y' || ((k === 'z' || k === 'Z') && ev.shiftKey))) return run(redo);
      if (mod && k === 'c') return run(copy);
      if (mod && k === 'x') return run(() => { copy(); del(); });
      if (mod && k === 'v') return run(paste);
      if (mod && k === 'd') return run(dup);
      if (mod && k === 'a') return run(() => select(spec.elements.filter((e) => !e.locked).map((e) => 'e:' + e.id).concat(spec.widgets.map((w, i) => 'w:' + i))));
      if (mod && (k === 'g' || k === 'G')) return run(() => group(!ev.shiftKey));
      if (k === 'Delete' || k === 'Backspace') return run(del);
      if (k === 'Escape') return run(() => { if (drawing) cancelDraw(); else if (tool !== 'select') setTool('select'); else select([]); });
      if (k === 'Enter' && drawing && drawing.poly) return run(finishPoly);
      const step = ev.shiftKey ? GRID() : 1;
      if (k === 'ArrowLeft') return run(() => nudge(-step, 0)); if (k === 'ArrowRight') return run(() => nudge(step, 0)); if (k === 'ArrowUp') return run(() => nudge(0, -step)); if (k === 'ArrowDown') return run(() => nudge(0, step));
      if (mod) return;
      if (k === '+' || k === '=') return run(() => zoomTo(Z * 1.25)); if (k === '-') return run(() => zoomTo(Z / 1.25)); if (k === '0') return run(fit);
      const tk = { v: 'select', r: 'rect', e: 'ellipse', l: 'line', p: 'polyline', g: 'polygon', i: 'pipe', t: 'text' }[k.toLowerCase()]; if (tk) return run(() => setTool(tk));
    }
    document.addEventListener('keydown', onKey);

    // ---- symbol library panel --------------------------------------------------------------------------------------
    function renderLibrary() {
      const q = $('.se-search').value.trim().toLowerCase(); const S = SC(); const byCat = {};
      for (const id of S.SYMBOL_IDS) { const d = S.symbols[id]; if (q && !(d.name.toLowerCase().includes(q) || id.includes(q))) continue; (byCat[d.cat] = byCat[d.cat] || []).push(d); }
      $('.se-syms').innerHTML = Object.keys(S.CATS).filter((c) => byCat[c]).map((c) => `<details open><summary>${esc(S.CATS[c])}</summary><div class="se-symgrid">${byCat[c].map((d) => `<button type="button" class="se-sym" draggable="true" data-sym="${d.id}" title="${esc(d.name)}">${S.symbolSVG(d.id, { level: 0.6, label: 'TT', value: '21.4' })}<span>${esc(d.name)}</span></button>`).join('')}</div></details>`).join('') || '<p class="muted">No symbol matches.</p>';
      host.querySelectorAll('.se-sym').forEach((b) => { b.onclick = () => { setTool('symbol', b.dataset.sym); host.focus({ preventScroll: true }); }; b.ondragstart = (ev) => { ev.dataTransfer.setData('text/x-ctrl32-symbol', b.dataset.sym); ev.dataTransfer.effectAllowed = 'copy'; }; });
    }
    $('.se-search').oninput = renderLibrary;

    // ---- layers panel ----------------------------------------------------------------------------------------------
    host.querySelector('.se-tabs').addEventListener('click', (ev) => { const b = ev.target.closest('[data-tab]'); if (!b) return; host.querySelectorAll('.se-tabs button').forEach((x) => x.classList.toggle('on', x === b)); props.hidden = b.dataset.tab !== 'props'; layers.hidden = b.dataset.tab !== 'layers'; renderLayers(); });
    function renderLayers() {
      if (layers.hidden) return; const S = SC();
      const rows = spec.widgets.map((w, i) => ['w:' + i, `${(w.params && w.params.title) || WG().labelOf(w.type)}`, 'widget', false, false]).reverse()
        .concat(spec.elements.slice().reverse().map((e) => ['e:' + e.id, e.name || (e.type === 'symbol' ? (S.symbols[e.symbol] || {}).name || e.symbol : e.type === 'text' ? String(e.text || '').slice(0, 24) : e.type), e.type + (e.group ? ' · ' + e.group : ''), !!e.locked, !!e.hidden]));
      layers.innerHTML = `<div class="se-layerlist">${rows.map(([k, name, type, locked, hidden]) => `<div class="se-lrow${SEL.includes(k) ? ' on' : ''}" data-k="${esc(k)}"><span class="se-lname">${esc(name)}</span><span class="muted">${esc(type)}</span>${k[0] === 'e' ? `<button type="button" class="se-lb${locked ? ' on' : ''}" data-lf="locked" title="Lock">${ICON.lock}</button><button type="button" class="se-lb${hidden ? ' on' : ''}" data-lf="hidden" title="Hide in operation">${ICON.eye}</button>` : ''}</div>`).join('') || '<p class="muted">Empty picture.</p>'}</div>`;
      layers.querySelectorAll('.se-lrow').forEach((r) => r.onclick = (ev) => { const k = r.dataset.k; const lf = ev.target.closest('[data-lf]');
        if (lf) { const e = el(k.slice(2)); if (e[lf.dataset.lf]) delete e[lf.dataset.lf]; else e[lf.dataset.lf] = true; drawLayer(); drawOverlay(); commit(); renderLayers(); return; }
        select(ev.shiftKey ? [...new Set(SEL.concat([k]))] : [k]); });
    }

    // ---- property panel --------------------------------------------------------------------------------------------
    const dsOptions = (chosen, emptyLabel) => `<option value="">${esc(emptyLabel)}</option>` + (o.datastreams || []).map((d) => `<option value="${d.id}"${d.id === chosen ? ' selected' : ''}>${esc(d.asset + ' / ' + d.key)}${d.unit ? ' [' + esc(d.unit) + ']' : ''}</option>`).join('');
    const colorIn = (path, v, allowNone) => { const hex = /^#[0-9a-fA-F]{6}$/.test(v || '') ? v : (/^#[0-9a-fA-F]{3}$/.test(v || '') ? '#' + v.slice(1).split('').map((c) => c + c).join('') : '#000000'); return `<span class="se-col"><input type="color" data-colpick="${path}" value="${hex}"${v && v !== 'none' ? '' : ' class="se-unset"'}><input data-f="${path}" data-kind="color" value="${esc(v || '')}" placeholder="${allowNone ? 'auto / none' : 'auto'}" maxlength="9"></span>`; };
    const num = (path, v, label, extra) => `<label>${esc(label)} <input type="number" step="any" data-f="${path}" data-kind="number" value="${v == null ? '' : v}" ${extra || ''}></label>`;
    const txt = (path, v, label, extra) => `<label>${esc(label)} <input data-f="${path}" data-kind="text" value="${esc(v ?? '')}" ${extra || ''}></label>`;
    const chk = (path, v, label) => `<label class="inline"><input type="checkbox" data-f="${path}" data-kind="bool" ${v ? 'checked' : ''}><span>${esc(label)}</span></label>`;
    const sel = (path, v, label, opts) => `<label>${esc(label)} <select data-f="${path}" data-kind="text">${opts.map(([val, lab]) => `<option value="${esc(val)}"${String(v ?? '') === val ? ' selected' : ''}>${esc(lab)}</option>`).join('')}</select></label>`;
    function renderProps() {
      if (SEL.length === 1 && SEL[0][0] === 'w') { props.innerHTML = ''; o.widgetProps && o.widgetProps(+SEL[0].slice(2), props); return; }
      const es = selEls();
      if (!es.length) { renderPictureProps(); return; }
      const e = es[0], one = es.length === 1, st = e.style || {}, t = e.type, S = SC();
      let h = `<div class="row" style="align-items:center"><b>${one ? esc(t === 'symbol' ? (S.symbols[e.symbol] || {}).name || e.symbol : t) : es.length + ' objects'}</b><span class="grow"></span>${one ? `<span class="muted" style="font-size:12px">${esc(e.id)}</span>` : ''}</div>`;
      if (one) h += `<details open><summary>Object</summary>${txt('name', e.name, 'Name (tooltip, faceplate)', 'maxlength="64"')}<div class="row">${chk('locked', e.locked, 'Locked')}${chk('hidden', e.hidden, 'Hidden in operation')}</div>${e.group ? `<div class="muted" style="font-size:12px">Group ${esc(e.group)}</div>` : ''}</details>`;
      if (one) { const b = S.bbox(e); h += `<details open><summary>Geometry</summary><div class="row se-geo">${num('@x', r2(b.x), 'X')}${num('@y', r2(b.y), 'Y')}${num('@w', r2(b.w), 'W', 'min="0"')}${num('@h', r2(b.h), 'H', 'min="0"')}${t !== 'line' ? num('r', e.r || 0, 'Rotation °', 'min="-360" max="360"') : ''}</div>${e.points ? `<div class="muted" style="font-size:12px">${e.points.length} points · drag a point, double-click a segment to add one, Alt-click a point to remove it</div>` : ''}</details>`; }
      if (one && t === 'symbol') h += `<details open><summary>Symbol</summary><label>Symbol <select data-f="symbol" data-kind="text">${Object.keys(S.CATS).map((c) => `<optgroup label="${esc(S.CATS[c])}">${S.SYMBOL_IDS.filter((id) => S.symbols[id].cat === c).map((id) => `<option value="${id}"${id === e.symbol ? ' selected' : ''}>${esc(S.symbols[id].name)}</option>`).join('')}</optgroup>`).join('')}</select></label>${txt('text', e.text, 'Label (instruments, buttons)', 'maxlength="500"')}</details>`;
      if (one && t === 'text') h += `<details open><summary>Text</summary><label>Text <textarea rows="2" data-f="text" data-kind="text" maxlength="500">${esc(e.text || '')}</textarea></label><div class="muted" style="font-size:11px">Placeholders: {value} {unit} {name} {time}</div></details>`;
      h += `<details open><summary>Appearance</summary>`;
      if (t !== 'line' && t !== 'polyline') h += `<label>${t === 'pipe' ? 'Medium colour' : t === 'text' ? 'Background' : 'Fill'} ${colorIn(t === 'pipe' ? 'style.medium' : t === 'text' ? 'style.bg' : 'style.fill', t === 'pipe' ? st.medium : t === 'text' ? st.bg : st.fill, true)}</label>`;
      h += `<label>${t === 'pipe' ? 'Outline' : 'Line'} ${colorIn('style.stroke', st.stroke, true)}</label><div class="row">${num('style.sw', st.sw, t === 'pipe' ? 'Diameter px' : 'Line width', 'min="0" max="100"')}${num('style.opacity', st.opacity, 'Opacity', 'min="0" max="1" step="0.05"')}</div>`;
      if (t !== 'pipe' && t !== 'symbol') h += sel('style.dash', st.dash, 'Line style', [['', 'solid'], ['dash', 'dashed'], ['dot', 'dotted'], ['dashdot', 'dash-dot']]);
      if (t === 'rect' || t === 'text') h += num('style.radius', st.radius, 'Corner radius', 'min="0"');
      if (t === 'symbol' || t === 'rect') h += `<label>Liquid / level colour ${colorIn('style.medium', st.medium, false)}</label>`;
      if (t === 'text') h += `<label>Text colour ${colorIn('style.color', st.color, false)}</label><div class="row">${num('style.font', st.font, 'Font size', 'min="4" max="400"')}${sel('style.align', st.align, 'Align', [['', 'left'], ['middle', 'centre'], ['end', 'right']])}${sel('style.valign', st.valign, 'Vertical', [['', 'middle'], ['top', 'top'], ['bottom', 'bottom']])}</div><div class="row">${chk('style.bold', st.bold, 'Bold')}${chk('style.italic', st.italic, 'Italic')}</div>`;
      h += `<div class="row">${t === 'symbol' || t === 'rect' || t === 'ellipse' || t === 'pipe' ? chk('style.shade', t === 'pipe' ? st.shade !== false : st.shade, t === 'pipe' ? '3D highlight' : 'Metallic shading') : ''}${chk('style.shadow', st.shadow, 'Shadow')}</div></details>`;
      if (one) {
        h += `<details open><summary>Binding</summary><label>Tag (datastream) <input class="se-dsfilter" placeholder="filter…"><select data-f="tag" data-kind="text">${dsOptions(e.tag, '— none —')}</select></label><div class="row">${txt('unit', e.unit, 'Unit (empty = datastream)', 'maxlength="16"')}${num('decimals', e.decimals, 'Decimals', 'min="0" max="6" step="1"')}</div>`;
        h += `<label>Device for operation <select data-f="device" data-kind="text"><option value="">— none —</option>${(o.devices || []).map((d) => `<option value="${d.id}"${d.id === e.device ? ' selected' : ''}>${esc(d.name || d.external_id)}</option>`).join('')}</select></label>${txt('key', e.key, 'Command key (register / datastream key)', 'maxlength="64"')}</details>`;
        h += dynEditor(e) + eventEditor(e);
      }
      props.innerHTML = h; wireProps(es);
    }
    function renderPictureProps() {
      const l = spec.layout;
      props.innerHTML = `<b>Picture</b><div class="muted" style="font-size:12px;margin:4px 0 8px">Nothing selected. Pick a tool or a symbol on the left, or click an object.</div>
        <div class="row">${num('#width', l.width || 1600, 'Width px', 'min="600" max="4000" step="10"')}${num('#height', l.height || 900, 'Height px', 'min="300" max="3000" step="10"')}</div>
        <label>Background ${colorIn('#bg', l.bg, true)}</label>
        <div class="row">${num('#grid', l.grid || 10, 'Grid px', 'min="1" max="100" step="1"')}${sel('#scheme', spec.scheme || '', 'State colours', [['', 'classic (WinCC-like)'], ['isa101', 'ISA-101 high performance']])}</div>
        <div class="muted" style="font-size:12px">${spec.elements.length} objects · ${spec.widgets.length} widgets</div>`;
      wireProps([]);
    }
    function dynEditor(e) {
      const rows = (e.dyn || []).map((d, i) => {
        const kind = DYNKIND[d.prop] || 'cond';
        let body = '';
        if (kind === 'color' || kind === 'state' || kind === 'text') body = `<div class="se-map">${(d.map || []).map((m, j) => `<div class="row se-mrow"><select data-dm="${i}.${j}.op">${OPS.map((op) => `<option${op === m.op ? ' selected' : ''}>${esc(op)}</option>`).join('')}</select><input type="number" step="any" data-dm="${i}.${j}.v" value="${m.v}" style="width:80px">${kind === 'color' ? `<input type="color" data-dm="${i}.${j}.to" value="${/^#[0-9a-f]{6}$/i.test(m.to) ? m.to : '#dc2626'}">` : kind === 'state' ? `<select data-dm="${i}.${j}.to">${Object.keys(STATE_NAMES).map((s) => `<option value="${s}"${s === m.to ? ' selected' : ''}>${esc(STATE_NAMES[s])}</option>`).join('')}</select>` : `<input data-dm="${i}.${j}.to" value="${esc(m.to)}" maxlength="64" style="width:110px">`}<button type="button" class="btn small ghost" data-dmdel="${i}.${j}" title="Remove row">✕</button></div>`).join('')}<button type="button" class="btn small" data-dmadd="${i}">+ row</button>${kind === 'text' ? '<div class="muted" style="font-size:11px">Without rows the text shows the formatted value.</div>' : ''}</div>`;
        else if (kind === 'cond') body = `<div class="row"><select data-dc="${i}.op">${OPS.map((op) => `<option${op === d.op ? ' selected' : ''}>${esc(op)}</option>`).join('')}</select><input type="number" step="any" data-dc="${i}.v" value="${d.v ?? 0}" style="width:90px">${d.prop === 'blink' ? `<label class="inline">frame <input type="color" data-dc="${i}.color" value="${/^#[0-9a-f]{6}$/i.test(d.color || '') ? d.color : '#dc2626'}"></label>` : ''}${d.prop === 'flow' || d.prop === 'rotate' ? `<label class="inline">speed <input type="number" step="0.1" min="0.05" max="20" data-dc="${i}.speed" value="${d.speed ?? 1}" style="width:64px"></label>` : ''}</div>`;
        else body = `<div class="row se-lin">${['value from', 'value to', 'result from', 'result to'].map((lab, k) => `<label>${esc(lab)} <input type="number" step="any" data-dl="${i}.${k}" value="${(d.lin || [0, 100, 0, 1])[k]}"></label>`).join('')}</div>`;
        return `<div class="se-dyn"><div class="row" style="align-items:center"><select data-dp="${i}">${DYN.map(([p, lab]) => `<option value="${p}"${p === d.prop ? ' selected' : ''}>${esc(lab)}</option>`).join('')}</select><span class="grow"></span><button type="button" class="btn small ghost" data-ddel="${i}" title="Remove dynamic">✕</button></div><label>Tag <select data-dt="${i}">${dsOptions(d.tag, '(object tag)')}</select></label>${body}</div>`;
      }).join('');
      return `<details open><summary>Dynamics (${(e.dyn || []).length}/8)</summary>${rows}${(e.dyn || []).length < 8 ? '<button type="button" class="btn small" data-dadd>+ Add dynamic</button>' : ''}</details>`;
    }
    function eventEditor(e) {
      const acts = [['faceplate', 'Open faceplate'], ['trend', 'Open trend'], ['screen', 'Go to screen'], ['command', 'Send command']];
      const rows = (e.on || []).map((a, i) => `<div class="se-dyn"><div class="row" style="align-items:center"><select data-ea="${i}.act">${acts.map(([v, l]) => `<option value="${v}"${v === a.act ? ' selected' : ''}>${esc(l)}</option>`).join('')}</select><span class="grow"></span><button type="button" class="btn small ghost" data-edel="${i}" title="Remove">✕</button></div>
        ${a.act === 'screen' ? `<label>Screen <select data-ea="${i}.target">${(o.screens || []).map((s) => `<option value="${esc(s.slug)}"${s.slug === a.target ? ' selected' : ''}>${esc(s.name)}</option>`).join('')}</select></label>` : ''}
        ${a.act === 'command' ? `<div class="row"><label>Key <input data-ea="${i}.key" value="${esc(a.key || '')}" maxlength="64"></label><label>Value <input data-ea="${i}.value" value="${esc(a.value ?? '')}" maxlength="64" placeholder="true / false / number / text"></label><label>Method <input data-ea="${i}.method" value="${esc(a.method || 'write')}" maxlength="32"></label></div><label class="inline"><input type="checkbox" data-ea="${i}.confirm" ${a.confirm !== false ? 'checked' : ''}><span>Ask for confirmation</span></label><div class="muted" style="font-size:11px">Needs the device in Binding; a reason is always asked.</div>` : ''}</div>`).join('');
      return `<details open><summary>Click actions (${(e.on || []).length}/4)</summary>${rows || '<div class="muted" style="font-size:12px">None — with a tag bound, a click opens the faceplate.</div>'}${(e.on || []).length < 4 ? '<button type="button" class="btn small" data-eadd>+ Add action</button>' : ''}</details>`;
    }
    function setPath(obj, path, v) { const ps = path.split('.'); let t = obj; for (let i = 0; i < ps.length - 1; i++) t = t[ps[i]] = t[ps[i]] || {}; const k = ps[ps.length - 1]; if (v === undefined || v === '' || v === null || (typeof v === 'number' && isNaN(v))) delete t[k]; else t[k] = v; if (ps[0] === 'style' && obj.style && !Object.keys(obj.style).length) delete obj.style; }
    function wireProps(es) {
      const changed = (needValues) => { drawLayer(); drawOverlay(); commit(); renderLayers(); if (needValues) refreshValues(); };
      const flt = props.querySelector('.se-dsfilter'); if (flt) flt.oninput = () => { const q = flt.value.toLowerCase(); for (const op of flt.nextElementSibling.options) op.hidden = q && op.value && !op.textContent.toLowerCase().includes(q); };
      props.querySelectorAll('[data-colpick]').forEach((c) => c.oninput = () => { const t = props.querySelector(`[data-f="${c.dataset.colpick}"]`); t.value = c.value; c.classList.remove('se-unset'); t.dispatchEvent(new Event('change', { bubbles: true })); });
      props.onchange = async (ev) => {
        const f = ev.target.closest('[data-f]');
        if (f) {
          const path = f.dataset.f, kind = f.dataset.kind; let v = kind === 'bool' ? f.checked : kind === 'number' ? (f.value === '' ? undefined : parseFloat(f.value)) : f.value.trim();
          if (kind === 'color' && v && !colRe.test(v)) { f.setCustomValidity('#rrggbb or none'); f.reportValidity(); return; } f.setCustomValidity && f.setCustomValidity('');
          if (path[0] === '#') { // picture settings
            const k = path.slice(1); if (k === 'scheme') { if (v) spec.scheme = v; else delete spec.scheme; } else if (k === 'bg') { if (v) spec.layout.bg = v; else delete spec.layout.bg; } else if (v !== undefined) spec.layout[k] = Math.round(v);
            commit(); if (k === 'width' || k === 'height' || k === 'grid') { await render(); } else drawLayer(); return;
          }
          if (path[0] === '@') { const e = es[0]; const b = SC().bbox(e); const nb = { ...b, [path[1]]: v ?? 0 }; setBB('e:' + e.id, nb, b); changed(false); renderProps(); return; }
          if (path === 'style.shade' && es[0].type === 'pipe') v = v ? undefined : false;
          for (const e of es) setPath(e, path, v);
          changed(path === 'tag'); if (path === 'symbol') { const d = SC().symbols[v]; if (d && es.length === 1) { /* keep size */ } }
          if (['locked', 'tag', 'symbol', 'name'].includes(path)) renderProps();
          return;
        }
        const e = es[0]; if (!e) return; e.dyn = e.dyn || []; e.on = e.on || [];
        const t = ev.target, ds = t.dataset;
        if (ds.dp !== undefined) { const d = e.dyn[+ds.dp]; const kind = DYNKIND[t.value]; const nd = { prop: t.value }; if (d.tag) nd.tag = d.tag; if (kind === 'cond') { nd.op = '>'; nd.v = 0; } if (kind === 'lin') nd.lin = t.value === 'level' ? [0, 100, 0, 1] : t.value === 'opacity' ? [0, 1, 0.2, 1] : t.value === 'angle' ? [0, 100, 0, 90] : [0, 100, 0, 100]; if (kind === 'color' || kind === 'state' || kind === 'text') nd.map = kind === 'state' ? [{ op: '==', v: 1, to: 'run' }, { op: '==', v: 0, to: 'stop' }] : kind === 'color' ? [{ op: '>=', v: 80, to: '#dc2626' }] : []; e.dyn[+ds.dp] = nd; }
        else if (ds.dt !== undefined) setPath(e.dyn[+ds.dt], 'tag', t.value);
        else if (ds.dc !== undefined) { const [i, k] = ds.dc.split('.'); e.dyn[+i][k] = k === 'op' || k === 'color' ? t.value : parseFloat(t.value); }
        else if (ds.dl !== undefined) { const [i, k] = ds.dl.split('.').map(Number); e.dyn[i].lin[k] = parseFloat(t.value) || 0; }
        else if (ds.dm !== undefined) { const [i, j, k] = ds.dm.split('.'); e.dyn[+i].map[+j][k] = k === 'v' ? parseFloat(t.value) || 0 : t.value; }
        else if (ds.ea !== undefined) { const [i, k] = ds.ea.split('.'); const a = e.on[+i]; if (k === 'confirm') a.confirm = t.checked; else if (k === 'value') { const s = t.value.trim(); a.value = s === 'true' ? true : s === 'false' ? false : s !== '' && !isNaN(+s) ? +s : s; } else a[k] = t.value; if (k === 'act') { for (const x of ['target', 'key', 'value', 'method', 'confirm']) delete a[x]; if (t.value === 'screen') a.target = ((o.screens || [])[0] || {}).slug || ''; if (t.value === 'command') { a.key = e.key || ''; a.value = true; } } }
        else return;
        if (!e.dyn.length) delete e.dyn; if (!e.on.length) delete e.on;
        changed(!!(ds.dt !== undefined)); if (ds.dp !== undefined || ds.ea !== undefined && ds.ea.endsWith('.act')) renderProps();
      };
      props.onclick = (ev) => {
        const e = es[0]; if (!e) return; const t = ev.target.closest('button'); if (!t) return; const ds = t.dataset;
        e.dyn = e.dyn || []; e.on = e.on || [];
        if (ds.dadd !== undefined) e.dyn.push(e.type === 'pipe' ? { prop: 'flow', op: '>', v: 0 } : e.type === 'text' ? { prop: 'text' } : e.type === 'symbol' ? { prop: 'state', map: [{ op: '==', v: 1, to: 'run' }, { op: '==', v: 0, to: 'stop' }] } : { prop: 'fill', map: [{ op: '>=', v: 80, to: '#dc2626' }] });
        else if (ds.ddel !== undefined) e.dyn.splice(+ds.ddel, 1);
        else if (ds.dmadd !== undefined) { const d = e.dyn[+ds.dmadd]; d.map = d.map || []; d.map.push({ op: '==', v: 0, to: DYNKIND[d.prop] === 'state' ? 'stop' : DYNKIND[d.prop] === 'color' ? '#16a34a' : 'text' }); }
        else if (ds.dmdel !== undefined) { const [i, j] = ds.dmdel.split('.').map(Number); e.dyn[i].map.splice(j, 1); }
        else if (ds.eadd !== undefined) e.on.push({ act: 'faceplate' });
        else if (ds.edel !== undefined) e.on.splice(+ds.edel, 1);
        else { if (!e.dyn.length) delete e.dyn; if (!e.on.length) delete e.on; return; }
        if (!e.dyn.length) delete e.dyn; if (!e.on.length) delete e.on;
        changed(false); renderProps();
      };
    }

    // ---- lifecycle -------------------------------------------------------------------------------------------------
    renderLibrary(); setTool('select');
    const ready = render().then(() => { commit(true); fit(); renderProps(); toolbarState(); });
    return {
      ready,
      reload: async () => { await render(); renderProps(); },
      commit: () => commit(), select: (keys) => select(keys), selection: () => SEL.slice(),
      showError(msg) { const m = /^element (\d+):/.exec(msg || ''); if (m) { const e = spec.elements[+m[1] - 1]; if (e) { select(['e:' + e.id]); } } },
      destroy() { document.removeEventListener('keydown', onKey); host.innerHTML = ''; },
      _debug: { get Z() { return Z; }, get tool() { return tool; }, get drawing() { return drawing; }, get drag() { return drag; }, setTool, placeSymbol, undo, redo, align, copy, paste, del, commit, hist },
    };
  }
  window.ctrl32ScadaEditor = { create };
})();
