// ctrl32 telemetry — flow editor (spec 004): palette, SVG canvas with nodes, ports and wires, selection, pan/zoom,
// copy/paste, undo/redo, property panel generated from the node catalogue, debug panel, test messages.
// Plain JS (ADR-0013/0015); every user text is escaped; the server validates everything again.
(() => {
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const NW = 170, NH = 38, PORT = 6, GRID = 10;
  const icon = (n, sz) => (window.ctrl32Widgets && ctrl32Widgets.icon(n, sz)) || '';
  const snap = (v) => Math.round(v / GRID) * GRID;
  const GROUPS = [['trigger', 'Triggers'], ['subflow', 'Subflow'], ['logic', 'Logic'], ['action', 'Actions'], ['other', 'Other']];
  const GCOLORS = ['#94a3b8', '#60a5fa', '#34d399', '#fbbf24', '#f87171', '#c084fc'];

  function create(o) {
    const host = o.host, cat = o.catalog, defs = Object.fromEntries(cat.nodes.map((d) => [d.type, d]));
    let spec = JSON.parse(JSON.stringify(o.spec || { nodes: [], wires: [] }));
    spec.nodes = spec.nodes || []; spec.wires = spec.wires || []; spec.groups = spec.groups || [];
    const isSub = spec.kind === 'subflow';
    let selGroup = null;
    let sel = new Set(), selWire = -1, view = { x: 0, y: 0, k: 1 }, problems = {}, status = null, dirty = false;
    const undo = [], redo = [];
    const lists = o.lists || { datastreams: [], devices: [], channels: [] };

    host.innerHTML = `<div class="fe">
      <aside class="fe-pal"><input class="fe-search" type="search" placeholder="Filter nodes…">${GROUPS.filter(([g]) => isSub ? g !== 'trigger' : g !== 'subflow').map(([g, l]) => `<details open><summary>${esc(l)}</summary><div class="fe-pal-list">${cat.nodes.filter((d) => d.group === g).map((d) => `<div class="fe-pal-item" draggable="true" data-type="${d.type}" title="${esc(d.help)}"><i style="background:${d.color}">${icon(d.icon, 14)}</i><span>${esc(d.label)}</span></div>`).join('')}</div></details>`).join('')}</aside>
      <div class="fe-stage"><svg class="fe-svg" tabindex="0"><defs><pattern id="fe-grid" width="${GRID * 2}" height="${GRID * 2}" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r="1" fill="var(--line)"/></pattern></defs>
        <rect class="fe-bg" x="-5000" y="-5000" width="10000" height="10000" fill="url(#fe-grid)"/><g class="fe-world"><g class="fe-groups"></g><g class="fe-wires"></g><g class="fe-nodes"></g><path class="fe-drag-wire" fill="none" stroke="var(--primary)" stroke-width="2" stroke-dasharray="5 4" visibility="hidden"/><rect class="fe-band" fill="color-mix(in srgb, var(--primary) 10%, transparent)" stroke="var(--primary)" stroke-dasharray="4 3" visibility="hidden"/></g></svg>
        <div class="fe-hint muted">Drag nodes from the palette; drag from an output port (right) to an input port (left) to wire; Delete removes; Ctrl+C/V/Z/Y; wheel zooms, drag the background to pan, Shift+drag selects.</div></div>
      <aside class="fe-props"></aside></div>`;
    const svg = host.querySelector('.fe-svg'), world = host.querySelector('.fe-world'), gN = host.querySelector('.fe-nodes'), gW = host.querySelector('.fe-wires'), gG = host.querySelector('.fe-groups');
    const dragWire = host.querySelector('.fe-drag-wire'), band = host.querySelector('.fe-band'), props = host.querySelector('.fe-props');

    const subOf = (slug) => (lists.subflows || []).find((x) => x.id === slug) || {};
    const outs = (n) => n.type === 'switch' ? ((n.settings.rules || []).length + 1)
      : n.type === 'subflow' ? (n.settings.outputs != null ? n.settings.outputs : (subOf(n.settings.flow).outputs || 0)) : (defs[n.type] || {}).outputs || 0;
    const ins = (n) => (defs[n.type] || {}).inputs || 0;
    const byId = (id) => spec.nodes.find((n) => n.id === id);
    const newId = (t) => { let i = 1; const base = t.replace(/_/g, ''); while (byId(base + i)) i++; return base + i; };
    const outPos = (n, p) => { const k = outs(n); return [n.x + NW, n.y + NH / 2 + (k > 1 ? (p - (k - 1) / 2) * 14 : 0)]; };
    const inPos = (n) => [n.x, n.y + NH / 2];
    const curve = (a, b) => { const dx = Math.max(40, Math.abs(b[0] - a[0]) / 2); return `M${a[0]},${a[1]} C${a[0] + dx},${a[1]} ${b[0] - dx},${b[1]} ${b[0]},${b[1]}`; };

    function commit() { undo.push(JSON.stringify(spec)); if (undo.length > 100) undo.shift(); redo.length = 0; dirty = true; o.onChange && o.onChange(); }
    function restore(s) { spec = JSON.parse(s); sel = new Set([...sel].filter((id) => byId(id))); selWire = -1; draw(); renderProps(); dirty = true; o.onChange && o.onChange(); }
    function applyView() { world.setAttribute('transform', `translate(${view.x},${view.y}) scale(${view.k})`); }
    function toWorld(ev) { const r = svg.getBoundingClientRect(); return [(ev.clientX - r.left - view.x) / view.k, (ev.clientY - r.top - view.y) / view.k]; }

    const tr = (x) => (window.ctrl32 && ctrl32.t ? ctrl32.t(x) : x);
    const clip1 = (x, max) => (x.length > max ? x.slice(0, max - 1) + '…' : x);
    function label(n) { return n.name || tr((defs[n.type] || {}).label || n.type); }
    function sub(n) {
      const s = n.settings || {}, d = defs[n.type];
      if (!d) return '';
      const dsName = (id) => ((lists.datastreams.find((x) => x.id === id) || {}).label) || '';
      switch (n.type) {
        case 'ds_value': case 'derive': return dsName(s.datastream);
        case 'filter': case 'stable_for': return (s.condition || '') + (n.type === 'stable_for' ? ` · ${s.seconds ?? ''} s` : '');
        case 'math': return s.expr || '';
        case 'delay': case 'debounce': return (s.seconds ?? '') + ' s';
        case 'schedule': return s.mode === 'daily' ? (s.times || '') : `every ${s.every_s ?? ''} s`;
        case 'command': return ((lists.devices.find((x) => x.id === s.device) || {}).label || '') + (s.key ? ' · ' + s.key : '');
        case 'entity': return ((lists.entities || []).find((x) => x.id === s.entity) || {}).label || '' ;
        case 'entity_action': return (s.service || '') + ' · ' + (((lists.entities || []).find((x) => x.id === s.entity) || {}).label || '');
        case 'mqtt_in': return (s.topic || '') + ' · ' + (((lists.sources || []).find((x) => x.id === s.source) || {}).label || '');
        case 'mqtt_out': return (s.topic || '') + ' · ' + (((lists.sources || []).find((x) => x.id === s.source) || {}).label || '');
        case 'scene': return (((lists.scenes || []).find((x) => x.id === s.scene) || {}).label || '');
        case 'notify': return ((lists.channels.find((x) => x.id === s.channel) || {}).label || '');
        case 'aggregate': return `${s.fn || 'avg'} / ${s.window_s ?? ''} s`;
        case 'deadband': return `± ${s.band ?? ''}${s.percent ? ' %' : ''}`;
        case 'http_out': return (s.method || '') + ' ' + (s.url || '');
        case 'comment': return (s.text || '').split('\n')[0];
        case 'subflow': return (subOf(s.flow).label || s.flow || '') + (s.version ? ' · v' + s.version : '');
        case 'sub_out': return tr('output') + ' ' + (s.port || 1);
        case 'split': return s.separator ? `"${s.separator}"` : (s.list || 'value');
      }
      return '';
    }

    function draw() {
      applyView();
      gG.innerHTML = spec.groups.map((g) => `<g class="fe-group${selGroup === g.id ? ' sel' : ''}" data-group="${esc(g.id)}"><rect class="fe-group-box" x="${g.x}" y="${g.y}" width="${g.w}" height="${g.h}" rx="8" style="fill:${esc(g.color || GCOLORS[0])};stroke:${esc(g.color || GCOLORS[0])}"/>
        <text class="fe-group-lbl notranslate" x="${g.x + 10}" y="${g.y + 17}">${esc(g.name || '')}</text><rect class="fe-group-grip" data-grip="${esc(g.id)}" x="${g.x + g.w - 12}" y="${g.y + g.h - 12}" width="12" height="12"/></g>`).join('');
      gW.innerHTML = spec.wires.map((w, i) => { const a = byId(w.from), b = byId(w.to); if (!a || !b) return ''; return `<g class="fe-wire${i === selWire ? ' sel' : ''}" data-w="${i}"><path class="fe-wire-hit" d="${curve(outPos(a, w.port), inPos(b))}"/><path class="fe-wire-line" d="${curve(outPos(a, w.port), inPos(b))}"/></g>`; }).join('');
      gN.innerHTML = spec.nodes.map((n) => {
        const d = defs[n.type] || { color: '#999', label: n.type, icon: '' }; const k = outs(n); const st = status && status.nodes && status.nodes[n.id]; const pr = problems[n.id];
        const isComment = n.type === 'comment';
        let ports = ins(n) ? `<circle class="fe-port in" data-in="${n.id}" cx="0" cy="${NH / 2}" r="${PORT}"/>` : '';
        for (let p = 0; p < k; p++) { const [, y] = outPos(n, p); const nm = tr((d.out_names || [])[p] || '') || (n.type === 'switch' ? (p < k - 1 ? String(p + 1) : 'else') : ''); ports += `<circle class="fe-port out" data-out="${n.id}" data-port="${p}" cx="${NW}" cy="${(y - n.y).toFixed(1)}" r="${PORT}"><title>${esc(nm || 'output')}</title></circle>${nm && k > 1 ? `<text class="fe-port-lbl notranslate" x="${NW + 9}" y="${(y - n.y - 3).toFixed(1)}">${esc(nm)}</text>` : ''}`; }
        const trig = d.group === 'trigger' ? `<g class="fe-inject" data-inject="${n.id}" transform="translate(-26,${NH / 2 - 10})"><rect width="20" height="20" rx="4"/><path d="M7,5 L15,10 L7,15 Z"/><title>Send a test message</title></g>` : '';
        const badge = st && (st.in || st.errors) ? `<text class="fe-badge${st.errors ? ' err' : ''}" x="4" y="${NH + 13}">in ${st.in} · out ${st.out}${st.errors ? ' · errors ' + st.errors : ''}</text>` : '';
        const txt = sub(n);
        return `<g class="fe-node${sel.has(n.id) ? ' sel' : ''}${pr ? ' bad' : ''}${n.disabled ? ' off' : ''}${isComment ? ' comment' : ''}" data-id="${n.id}" transform="translate(${n.x},${n.y})">
          ${trig}<rect class="fe-body" width="${NW}" height="${NH}" rx="6"/>${isComment ? '' : `<rect class="fe-bar" width="26" height="${NH}" rx="6" fill="${d.color}"/><rect x="20" width="6" height="${NH}" fill="${d.color}"/>`}
          <g transform="translate(5,${NH / 2 - 8})" class="fe-ico">${isComment ? '' : icon(d.icon, 16)}</g>
          <text class="fe-lbl notranslate" x="${isComment ? 8 : 32}" y="${txt ? 16 : NH / 2 + 4}">${esc(clip1(label(n), isComment ? 26 : 19))}<title>${esc(label(n))}</title></text>${txt ? `<text class="fe-sub notranslate" x="${isComment ? 8 : 32}" y="30">${esc(clip1(txt, isComment ? 30 : 24))}</text>` : ''}
          ${ports}${badge}${pr ? `<title>${esc(pr.join('\n'))}</title>` : ''}</g>`;
      }).join('');
    }

    // ---- palette: drag or click to add
    for (const it of host.querySelectorAll('.fe-pal-item')) {
      it.addEventListener('dragstart', (e) => { e.dataTransfer.setData('text/plain', it.dataset.type); });
      it.addEventListener('dblclick', () => { const r = svg.getBoundingClientRect(); addNode(it.dataset.type, (r.width / 2 - view.x) / view.k - NW / 2, (r.height / 2 - view.y) / view.k); });
    }
    host.querySelector('.fe-search').oninput = (e) => { const q = e.target.value.toLowerCase(); for (const it of host.querySelectorAll('.fe-pal-item')) it.hidden = q && !it.textContent.toLowerCase().includes(q) && !it.dataset.type.includes(q); };
    svg.addEventListener('dragover', (e) => e.preventDefault());
    svg.addEventListener('drop', (e) => { e.preventDefault(); const t = e.dataTransfer.getData('text/plain'); if (!defs[t]) return; const [x, y] = toWorld(e); addNode(t, x - NW / 2, y - NH / 2); });
    function defaults(d) { const s = {}; for (const p of d.settings) if (p.default !== undefined) s[p.name] = JSON.parse(JSON.stringify(p.default)); return s; }
    function addNode(type, x, y) {
      commit(); const d = defs[type]; x = snap(x); y = snap(y);
      while (spec.nodes.some((m) => Math.abs(m.x - x) < 40 && Math.abs(m.y - y) < 30)) { x += 30; y += 60; } // never stack new nodes
      const n = { id: newId(type), type, x, y, settings: defaults(d) }; spec.nodes.push(n); sel = new Set([n.id]); selWire = -1; draw(); renderProps(); return n; }

    // ---- pointer interaction
    let drag = null;
    svg.addEventListener('pointerdown', (e) => {
      svg.focus();
      const [x, y] = toWorld(e);
      const grip = e.target.closest('[data-grip]');
      if (grip) { const g = spec.groups.find((x) => x.id === grip.dataset.grip); commit(); drag = { kind: 'resize', g, x, y, w: g.w, h: g.h }; selGroup = g.id; sel = new Set(); svg.setPointerCapture(e.pointerId); draw(); renderProps(); return; }
      const inj = e.target.closest('[data-inject]');
      if (inj) { o.onInject && o.onInject(inj.dataset.inject); return; }
      const out = e.target.closest('[data-out]');
      if (out) { drag = { kind: 'wire', from: out.dataset.out, port: +out.dataset.port }; svg.setPointerCapture(e.pointerId); return; }
      const g = e.target.closest('.fe-node');
      if (g) {
        const id = g.dataset.id;
        if (e.shiftKey || e.ctrlKey || e.metaKey) { sel.has(id) ? sel.delete(id) : sel.add(id); } else if (!sel.has(id)) sel = new Set([id]);
        selWire = -1; selGroup = null;
        drag = { kind: 'move', x, y, start: [...sel].map((i) => { const n = byId(i); return [i, n.x, n.y]; }), moved: false };
        svg.setPointerCapture(e.pointerId); draw(); renderProps(); return;
      }
      const w = e.target.closest('[data-w]');
      if (w) { selWire = +w.dataset.w; sel = new Set(); selGroup = null; draw(); renderProps(); return; }
      const grp = e.target.closest('[data-group]');
      if (grp && !e.shiftKey) { // drag a group with the nodes inside it
        const g = spec.groups.find((x) => x.id === grp.dataset.group);
        const inside = spec.nodes.filter((n) => n.x >= g.x && n.y >= g.y && n.x + NW <= g.x + g.w + 1 && n.y + NH <= g.y + g.h + 1);
        selGroup = g.id; sel = new Set(); selWire = -1;
        drag = { kind: 'group', g, x, y, gx: g.x, gy: g.y, start: inside.map((n) => [n.id, n.x, n.y]), moved: false };
        svg.setPointerCapture(e.pointerId); draw(); renderProps(); return;
      }
      selGroup = null;
      if (e.shiftKey) { drag = { kind: 'band', x, y }; } else { drag = { kind: 'pan', cx: e.clientX, cy: e.clientY, vx: view.x, vy: view.y }; }
      sel = new Set(); selWire = -1; svg.setPointerCapture(e.pointerId); draw(); renderProps();
    });
    svg.addEventListener('pointermove', (e) => {
      if (!drag) return;
      const [x, y] = toWorld(e);
      if (drag.kind === 'move') {
        const dx = snap(x - drag.x), dy = snap(y - drag.y);
        if (!drag.moved && (dx || dy)) { commit(); drag.moved = true; }
        for (const [id, x0, y0] of drag.start) { const n = byId(id); n.x = x0 + dx; n.y = y0 + dy; }
        draw();
      } else if (drag.kind === 'group') {
        const dx = snap(x - drag.x), dy = snap(y - drag.y);
        if (!drag.moved && (dx || dy)) { commit(); drag.moved = true; }
        drag.g.x = drag.gx + dx; drag.g.y = drag.gy + dy;
        for (const [id, x0, y0] of drag.start) { const n = byId(id); n.x = x0 + dx; n.y = y0 + dy; }
        draw();
      } else if (drag.kind === 'resize') {
        drag.g.w = Math.max(120, snap(drag.w + x - drag.x)); drag.g.h = Math.max(60, snap(drag.h + y - drag.y)); draw();
      } else if (drag.kind === 'wire') {
        const a = byId(drag.from); dragWire.setAttribute('d', curve(outPos(a, drag.port), [x, y])); dragWire.setAttribute('visibility', 'visible');
      } else if (drag.kind === 'pan') {
        view.x = drag.vx + e.clientX - drag.cx; view.y = drag.vy + e.clientY - drag.cy; applyView();
      } else if (drag.kind === 'band') {
        band.setAttribute('x', Math.min(x, drag.x)); band.setAttribute('y', Math.min(y, drag.y)); band.setAttribute('width', Math.abs(x - drag.x)); band.setAttribute('height', Math.abs(y - drag.y)); band.setAttribute('visibility', 'visible');
        drag.x2 = x; drag.y2 = y;
      }
    });
    svg.addEventListener('pointerup', (e) => {
      if (!drag) return;
      if (drag.kind === 'wire') {
        dragWire.setAttribute('visibility', 'hidden');
        const t = document.elementFromPoint(e.clientX, e.clientY); const inp = t && (t.closest('[data-in]') || t.closest('.fe-node'));
        const to = inp && (inp.dataset.in || inp.dataset.id);
        const target = to && byId(to);
        if (target && to !== drag.from && ins(target) && !spec.wires.some((w) => w.from === drag.from && w.port === drag.port && w.to === to)) { commit(); spec.wires.push({ from: drag.from, port: drag.port, to }); draw(); }
      } else if (drag.kind === 'band' && drag.x2 != null) {
        const x1 = Math.min(drag.x, drag.x2), x2 = Math.max(drag.x, drag.x2), y1 = Math.min(drag.y, drag.y2), y2 = Math.max(drag.y, drag.y2);
        sel = new Set(spec.nodes.filter((n) => n.x + NW > x1 && n.x < x2 && n.y + NH > y1 && n.y < y2).map((n) => n.id));
        band.setAttribute('visibility', 'hidden'); draw(); renderProps();
      }
      drag = null;
    });
    svg.addEventListener('wheel', (e) => {
      e.preventDefault(); const r = svg.getBoundingClientRect(); const mx = e.clientX - r.left, my = e.clientY - r.top;
      const k = Math.max(0.25, Math.min(2.5, view.k * (e.deltaY < 0 ? 1.1 : 1 / 1.1)));
      view.x = mx - (mx - view.x) * k / view.k; view.y = my - (my - view.y) * k / view.k; view.k = k; applyView(); o.onZoom && o.onZoom(view.k);
    }, { passive: false });

    // ---- keyboard
    let clip = null;
    svg.addEventListener('keydown', (e) => {
      const mod = e.ctrlKey || e.metaKey;
      if ((e.key === 'Delete' || e.key === 'Backspace') && selGroup && !sel.size) { e.preventDefault(); commit(); spec.groups = spec.groups.filter((g) => g.id !== selGroup); selGroup = null; draw(); renderProps(); }
      else if ((e.key === 'Delete' || e.key === 'Backspace') && (sel.size || selWire >= 0)) { e.preventDefault(); removeSelection(); }
      else if (mod && e.key.toLowerCase() === 'z') { e.preventDefault(); e.shiftKey ? doRedo() : doUndo(); }
      else if (mod && e.key.toLowerCase() === 'y') { e.preventDefault(); doRedo(); }
      else if (mod && e.key.toLowerCase() === 'c' && sel.size) { e.preventDefault(); copy(); }
      else if (mod && e.key.toLowerCase() === 'v' && clip) { e.preventDefault(); paste(); }
      else if (mod && e.key.toLowerCase() === 'a') { e.preventDefault(); sel = new Set(spec.nodes.map((n) => n.id)); draw(); renderProps(); }
      else if (e.key === 'Escape') { sel = new Set(); selWire = -1; draw(); renderProps(); }
    });
    function removeSelection() {
      commit();
      if (selWire >= 0) spec.wires.splice(selWire, 1);
      spec.nodes = spec.nodes.filter((n) => !sel.has(n.id)); spec.wires = spec.wires.filter((w) => !sel.has(w.from) && !sel.has(w.to));
      sel = new Set(); selWire = -1; draw(); renderProps();
    }
    function copy() { const ids = sel; clip = { nodes: spec.nodes.filter((n) => ids.has(n.id)), wires: spec.wires.filter((w) => ids.has(w.from) && ids.has(w.to)) }; clip = JSON.parse(JSON.stringify(clip)); }
    function paste() {
      commit(); const map = {};
      const added = clip.nodes.map((n) => { const c = JSON.parse(JSON.stringify(n)); c.id = newId(n.type); map[n.id] = c.id; c.x += 40; c.y += 40; spec.nodes.push(c); return c; });
      for (const w of clip.wires) spec.wires.push({ from: map[w.from], port: w.port, to: map[w.to] });
      clip = { nodes: added.map((n) => JSON.parse(JSON.stringify(n))), wires: clip.wires.map((w) => ({ from: map[w.from], port: w.port, to: map[w.to] })) };
      sel = new Set(added.map((n) => n.id)); draw(); renderProps();
    }
    function doUndo() { if (!undo.length) return; redo.push(JSON.stringify(spec)); restore(undo.pop()); }
    function doRedo() { if (!redo.length) return; undo.push(JSON.stringify(spec)); restore(redo.pop()); }

    // ---- properties
    function field(p, v) {
      const lab = `${esc(p.label)}${p.required ? ' *' : ''}`; const help = p.help ? `<span class="muted" style="font-size:11px">${esc(p.help)}</span>` : '';
      const opt = (list, cur) => list.map((x) => `<option value="${esc(x.id)}" ${x.id === cur ? 'selected' : ''}>${esc(x.label)}</option>`).join('');
      switch (p.kind) {
        case 'bool': return `<label class="inline"><input type="checkbox" data-s="${p.name}" ${v ? 'checked' : ''}><span>${lab}</span></label>`;
        case 'select': return `<label>${lab} <select data-s="${p.name}">${p.options.map((x) => `<option value="${esc(x)}" ${x === v ? 'selected' : ''}>${x === '' ? '(any)' : esc(x)}</option>`).join('')}</select>${help}</label>`;
        case 'multi': return `<fieldset class="fe-multi"><span>${lab}</span><div>${p.options.map((x) => `<label class="inline"><input type="checkbox" data-m="${p.name}" value="${esc(x)}" ${(v || []).includes(x) ? 'checked' : ''}><span>${esc(x)}</span></label>`).join('')}</div></fieldset>`;
        case 'number': return `<label>${lab} <input type="number" step="any" data-s="${p.name}" value="${v ?? ''}"${p.min != null ? ` min="${p.min}"` : ''}${p.max != null ? ` max="${p.max}"` : ''}>${help}</label>`;
        case 'multiline': case 'template': return `<label>${lab} <textarea rows="${p.kind === 'template' ? 2 : 4}" data-s="${p.name}" class="${p.kind === 'template' ? 'fe-mono' : ''}">${esc(v ?? '')}</textarea>${help}</label>`;
        case 'expr': return `<label>${lab} <input data-s="${p.name}" class="fe-mono" value="${esc(v ?? '')}" spellcheck="false">${help}</label>`;
        case 'time': return `<label>${lab} <input type="time" data-s="${p.name}" value="${esc(v ?? '')}"></label>`;
        case 'datastream': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.datastreams, v)}</select>${help}</label>`;
        case 'device': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.devices, v)}</select>${help}</label>`;
        case 'entity': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.entities || [], v)}</select>${help}</label>`;
        case 'scene': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.scenes || [], v)}</select>${help}</label>`;
        case 'mqtt_source': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.sources || [], v)}</select>${help}</label>`;
        case 'channel': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.channels, v)}</select>${help}</label>`;
        case 'subflow': return `<label>${lab} <select data-s="${p.name}"><option value="">—</option>${opt(lists.subflows || [], v)}</select>${help}</label>`;
        case 'rules': return `<label>${lab} <textarea rows="3" data-rules="${p.name}" class="fe-mono">${esc((v || []).join('\n'))}</textarea><span class="muted" style="font-size:11px">one condition per line; an extra output takes the rest</span></label>`;
        case 'fields': return `<div class="fe-fields" data-fields="${p.name}"><span>${lab}</span>${(v || []).map((c, i) => `<div class="fe-field-row" data-i="${i}"><select data-f="op">${['set', 'delete', 'rename'].map((x) => `<option ${x === c.op ? 'selected' : ''}>${x}</option>`).join('')}</select><input data-f="name" value="${esc(c.name || '')}" placeholder="value / topic / msg.x" class="fe-mono"><input data-f="${c.op === 'rename' ? 'to' : 'expr'}" value="${esc(c.op === 'rename' ? (c.to || '') : (c.expr || ''))}" placeholder="${c.op === 'rename' ? 'new name' : 'expression'}" class="fe-mono" ${c.op === 'delete' ? 'hidden' : ''}><button type="button" class="btn small ghost" data-fdel="${i}">✕</button></div>`).join('')}<button type="button" class="btn small" data-fadd>Add change</button></div>`;
      }
      return `<label>${lab} <input data-s="${p.name}" value="${esc(v ?? '')}">${help}</label>`;
    }
    function groupAround(ids) {
      const ns = spec.nodes.filter((n) => ids.has(n.id)); if (!ns.length) return;
      commit(); let i = 1; while (spec.groups.some((g) => g.id === 'g' + i)) i++;
      const x0 = Math.min(...ns.map((n) => n.x)) - 20, y0 = Math.min(...ns.map((n) => n.y)) - 36, x1 = Math.max(...ns.map((n) => n.x)) + NW + 20, y1 = Math.max(...ns.map((n) => n.y)) + NH + 30;
      const g = { id: 'g' + i, name: tr('Group') + ' ' + i, color: GCOLORS[(i - 1) % GCOLORS.length], x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
      spec.groups.push(g); selGroup = g.id; sel = new Set(); draw(); renderProps();
    }
    function renderProps() {
      if (selGroup && !sel.size) {
        const g = spec.groups.find((x) => x.id === selGroup);
        if (g) {
          props.innerHTML = `<h3>${esc(tr('Group'))}</h3><div class="fe-form"><label>${esc(tr('Name'))} <input data-gname maxlength="64" value="${esc(g.name || '')}"></label>
            <div class="fe-gcolors">${GCOLORS.map((c) => `<button type="button" class="fe-gcol${c === g.color ? ' on' : ''}" data-gcol="${c}" style="background:${c}" aria-label="${c}"></button>`).join('')}</div>
            <p class="muted" style="font-size:12px">${esc(tr('Drag the group to move it with its nodes; drag the corner to resize.'))}</p>
            <button class="btn small danger" type="button" data-gdel>${esc(tr('Delete group'))}</button></div>`;
          props.querySelector('[data-gname]').onchange = (e) => { commit(); g.name = e.target.value.trim(); draw(); };
          for (const b of props.querySelectorAll('[data-gcol]')) b.onclick = () => { commit(); g.color = b.dataset.gcol; draw(); renderProps(); };
          props.querySelector('[data-gdel]').onclick = () => { commit(); spec.groups = spec.groups.filter((x) => x.id !== g.id); selGroup = null; draw(); renderProps(); };
          return;
        }
      }
      if (selWire >= 0) { props.innerHTML = `<h3>Wire</h3><p class="muted">${esc(spec.wires[selWire].from)} → ${esc(spec.wires[selWire].to)}</p><button class="btn small danger" type="button" data-del>Delete wire</button>`; props.querySelector('[data-del]').onclick = removeSelection; return; }
      if (sel.size !== 1) {
        props.innerHTML = sel.size ? `<h3>${sel.size} nodes</h3><button class="btn small danger" type="button" data-del>Delete</button> <button class="btn small" type="button" data-copy>Copy</button> <button class="btn small" type="button" data-group-make>${esc(tr('Group'))}</button>` : `<h3>${esc(tr(isSub ? 'Subflow' : 'Flow'))}</h3><p class="muted">Select a node to see its settings.</p>${o.flowInfo ? o.flowInfo() : ''}`;
        const d = props.querySelector('[data-del]'); if (d) d.onclick = removeSelection; const c = props.querySelector('[data-copy]'); if (c) c.onclick = copy;
        const gm = props.querySelector('[data-group-make]'); if (gm) gm.onclick = () => groupAround(sel);
        return;
      }
      const n = byId([...sel][0]); const d = defs[n.type]; const pr = problems[n.id];
      props.innerHTML = `<div class="fe-props-head"><i style="background:${d.color}">${icon(d.icon, 16)}</i><b>${esc(d.label)}</b><span class="muted notranslate">${esc(n.id)}</span></div>
        <p class="muted" style="font-size:12px;margin:4px 0 8px">${esc(d.help)}</p>${pr ? `<div class="fe-problems">${pr.map((x) => `<div>${esc(x)}</div>`).join('')}</div>` : ''}
        <div class="fe-form"><label>Name <input data-name value="${esc(n.name || '')}" maxlength="64" placeholder="${esc(d.label)}"></label>
        ${d.settings.map((p) => field(p, (n.settings || {})[p.name])).join('')}
        <label class="inline"><input type="checkbox" data-disabled ${n.disabled ? 'checked' : ''}><span>Disabled</span></label>
        ${o.nodeExtra ? o.nodeExtra(n) : ''}</div>`;
      const prevFlow = (n.settings || {}).flow;
      const change = () => {
        commit();
        n.name = props.querySelector('[data-name]').value.trim() || undefined; n.disabled = props.querySelector('[data-disabled]').checked || undefined;
        n.settings = n.settings || {};
        for (const el of props.querySelectorAll('[data-s]')) { const p = d.settings.find((x) => x.name === el.dataset.s); let v = el.type === 'checkbox' ? el.checked : el.value; if (p.kind === 'number') v = el.value === '' ? undefined : parseFloat(el.value); if (v === '' || v === undefined) delete n.settings[p.name]; else n.settings[p.name] = v; }
        for (const p of d.settings.filter((x) => x.kind === 'multi')) n.settings[p.name] = [...props.querySelectorAll(`[data-m="${p.name}"]:checked`)].map((x) => x.value);
        for (const el of props.querySelectorAll('[data-rules]')) n.settings[el.dataset.rules] = el.value.split('\n').map((x) => x.trim()).filter(Boolean);
        for (const box of props.querySelectorAll('[data-fields]')) n.settings[box.dataset.fields] = [...box.querySelectorAll('.fe-field-row')].map((r) => { const op = r.querySelector('[data-f="op"]').value; const c = { op, name: r.querySelector('[data-f="name"]').value.trim() }; const x = r.querySelector('[data-f="expr"],[data-f="to"]'); if (op === 'set') c.expr = x ? x.value : ''; if (op === 'rename') c.to = x ? x.value.trim() : ''; return c; });
        if (n.type === 'subflow' && n.settings.flow !== prevFlow) { delete n.settings.version; n.settings.outputs = subOf(n.settings.flow).outputs || 0; delete n.settings.commands; }
        if (n.type === 'switch' || n.type === 'subflow') { const k = outs(n); spec.wires = spec.wires.filter((w) => w.from !== n.id || w.port < k); }
        draw();
      };
      props.onchange = (e) => { change(); if (e.target.matches('[data-f="op"]') || e.target.dataset.s === 'flow') renderProps(); };
      for (const b of props.querySelectorAll('[data-fdel]')) b.onclick = () => { const box = b.closest('[data-fields]'); b.closest('.fe-field-row').remove(); change(); renderProps(); void box; };
      for (const b of props.querySelectorAll('[data-fadd]')) b.onclick = () => { commit(); const k = b.closest('[data-fields]').dataset.fields; (n.settings[k] = n.settings[k] || []).push({ op: 'set', name: 'value', expr: 'value' }); renderProps(); };
    }

    function fit() {
      if (!spec.nodes.length) { view = { x: 40, y: 40, k: 1 }; applyView(); return; }
      const r = svg.getBoundingClientRect(); const xs = spec.nodes.map((n) => n.x), ys = spec.nodes.map((n) => n.y);
      const x0 = Math.min(...xs) - 50, y0 = Math.min(...ys) - 30, x1 = Math.max(...xs) + NW + 60, y1 = Math.max(...ys) + NH + 40;
      const k = Math.max(0.3, Math.min(1.4, Math.min(r.width / (x1 - x0), r.height / (y1 - y0))));
      view = { k, x: (r.width - (x1 - x0) * k) / 2 - x0 * k, y: (r.height - (y1 - y0) * k) / 2 - y0 * k }; applyView(); o.onZoom && o.onZoom(k);
    }

    draw(); renderProps(); requestAnimationFrame(fit);
    return {
      get spec() { return JSON.parse(JSON.stringify(spec)); },
      get dirty() { return dirty; }, set dirty(v) { dirty = v; },
      setProblems(list) { problems = {}; for (const p of list || []) if (p.node) (problems[p.node] = problems[p.node] || []).push(p.message); draw(); renderProps(); },
      setStatus(s) { status = s; draw(); },
      undo: doUndo, redo: doRedo, fit, removeSelection, copy, paste, addNode,
      zoom(f) { const r = svg.getBoundingClientRect(); const k = Math.max(0.25, Math.min(2.5, view.k * f)); view.x = r.width / 2 - (r.width / 2 - view.x) * k / view.k; view.y = r.height / 2 - (r.height / 2 - view.y) * k / view.k; view.k = k; applyView(); o.onZoom && o.onZoom(k); },
      select(ids) { sel = new Set(ids); selWire = -1; draw(); renderProps(); },
      connect(from, port, to) { commit(); spec.wires.push({ from, port, to }); draw(); },
      nodeScreenPos(id) { const n = byId(id), r = svg.getBoundingClientRect(); return n ? { x: r.left + view.x + n.x * view.k, y: r.top + view.y + n.y * view.k, w: NW * view.k, h: NH * view.k } : null; },
      canUndo: () => undo.length > 0, canRedo: () => redo.length > 0,
    };
  }
  window.ctrl32FlowEditor = { create };
})();
