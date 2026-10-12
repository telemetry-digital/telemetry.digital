// ctrl32 telemetry — object counting on cameras (spec 029): the list of cameras with their counting state and the
// editor of counting lines and speed sections drawn on a picture of the camera. A line runs from A to B; the arrow
// points to the side where the area lies (crossing towards it counts in). A speed section is two lines and the
// distance between them on the ground.
(() => {
  const $ = (s) => document.querySelector(s), esc = ctrl32.esc, api = ctrl32.api, t = ctrl32.t;
  const nf = (v, d = 0) => new Intl.NumberFormat(document.documentElement.lang || undefined, { maximumFractionDigits: d }).format(v || 0);
  const CLASS_LABEL = { person: 'person', bicycle: 'bicycle', car: 'car', motorcycle: 'motorcycle', bus: 'bus', truck: 'truck' };
  const STATE = { off: ['off', ''], not_installed: ['not installed', 'warn'], waiting: ['waiting for the picture', 'warn'], starting: ['starting', 'warn'], running: ['counting', 'ok'], error: ['error', 'bad'], limit: ['over the camera limit', 'warn'] };
  const VEHICLES = ['car', 'motorcycle', 'bus', 'truck'];
  let CTX = null, CAMS = [], CAM = null, CFG = null, SEL = -1, ADDING = null, POLL = null, RATIO = 16 / 9, SHOW_BOXES = true, BOXES = [];
  let PLAYER = null, HAVE_PIC = false, FLASH = {}, LAST = 0;
  const W = 1000, H = () => W / RATIO;
  const store = (k, v) => { try { localStorage.setItem('ctrl32.counting.' + k, v); } catch (e) { /* private mode */ } };
  const stored = (k) => { try { return localStorage.getItem('ctrl32.counting.' + k); } catch (e) { return null; } };
  const liveOn = () => stored('editorLive') !== '0'; // the live picture is the default, a still picture on request
  // the picture fits the window height by default (a tall 4:3 camera otherwise needs scrolling); vh, 0 = full width
  const SIZES = { small: 35, fit: 55, large: 0 };
  const size = () => (SIZES[stored('editorSize')] != null ? stored('editorSize') : 'fit');
  function sizeEd() { const v = SIZES[size()], ed = $('#ocEd'); if (ed) ed.style.maxWidth = v ? `calc(${v}vh * ${RATIO})` : ''; }
  const badge = (st) => { const s = STATE[st.state] || [st.state, '']; return `<span class="badge ${s[1]}">${esc(t(s[0]))}</span>`; };
  const uid = () => Math.random().toString(36).slice(2, 8);

  async function list() {
    if (window.ctrl32AnalyticsCheck) window.ctrl32AnalyticsCheck.render($('#acBody')); // ffmpeg, ONNX Runtime, models
    CAMS = await api('GET', '/api/v1/counting/cameras');
    $('#camTable tbody').innerHTML = CAMS.map((c) => {
      const st = c.status || { state: 'off' }, cfg = c.config || {};
      return `<tr><td class="notranslate">${esc(c.camera)}${c.camera_enabled ? '' : ` <span class="badge">${esc(t('camera off'))}</span>`}</td><td>${c.enabled ? '<span class="badge ok">on</span>' : '<span class="badge">off</span>'}</td><td>${(cfg.lines || []).length}</td><td>${(cfg.sections || []).length}</td><td>${badge(st)}${st.error ? `<div class="muted small">${esc(st.error)}</div>` : ''}</td><td class="num">${c.enabled ? nf(c.fps, 1) : ''}</td><td class="num" data-cpu="${c.camera_id}"></td><td><button class="btn small" type="button" data-cam="${c.camera_id}">${esc(t(CTX.can('config.write') ? 'Configure' : 'Show'))}</button></td></tr>`;
    }).join('') || `<tr><td colspan="8" class="muted">${esc(t('No cameras. Add cameras under Cameras → Camera management.'))}</td></tr>`;
    for (const b of document.querySelectorAll('[data-cam]')) b.onclick = () => open(b.dataset.cam);
    const want = new URLSearchParams(location.search).get('camera');
    if (want && !CAM) open(want);
  }

  async function open(id) {
    const c = await api('GET', `/api/v1/counting/cameras/${id}`);
    CAM = c; CFG = JSON.parse(JSON.stringify(c.config || { lines: [] }));
    CFG.lines = CFG.lines || []; CFG.sections = CFG.sections || [];
    SEL = CFG.lines.length ? 0 : -1; ADDING = null;
    stopLive();
    const card = $('#editorCard'); card.hidden = false;
    const editable = CTX.can('config.write');
    card.innerHTML = `<div class="page-head"><h2 style="margin:0"><span>Counting on</span> <span class="notranslate">${esc(c.camera)}</span></h2><span class="grow"></span>${badge(c.status)}</div>
      <div class="oc-ed-grid">
        <div>
          <div class="oc-editor" id="ocEd"><img id="ocPic" alt="" hidden><div class="oc-live" id="ocLive"></div><svg id="ocSvg" viewBox="0 0 ${W} ${H()}" preserveAspectRatio="none"></svg><div class="oc-hint" id="ocHint"></div></div>
          <div class="actions" style="margin-top:8px">
            <label class="oc-check"><input type="checkbox" id="ocLiveOn" ${liveOn() ? 'checked' : ''}> Live video</label>
            <button class="btn small" type="button" id="ocPicBtn"${liveOn() ? ' hidden' : ''}>Refresh picture</button>
            ${editable ? '<button class="btn small" type="button" id="ocAdd">Add line</button><button class="btn small" type="button" id="ocAddSpeed">Add speed section</button>' : ''}
            <label class="oc-check"><input type="checkbox" id="ocBoxes" ${SHOW_BOXES ? 'checked' : ''}> Show detected objects</label>
            <label class="oc-check">Size <select id="ocSize"><option value="small">small</option><option value="fit">fit to the screen</option><option value="large">full width</option></select></label>
          </div>
        </div>
        <div class="kv" id="ocStatus"></div>
      </div>
        <form class="form" id="ocForm" style="margin-top:12px">
          <label class="oc-check"><input type="checkbox" name="enabled" ${c.enabled ? 'checked' : ''}> Count on this camera</label>
          <div class="row2"><label>Analysed pictures per second <input name="fps" type="number" min="1" max="15" step="1" value="${c.fps}"></label>
            <label>Detector <select name="model"><option value="nano">fast (nano)</option><option value="tiny">more accurate (tiny, about 3× the CPU)</option></select></label></div>
          <div class="row2"><label>Detection threshold <input name="min_score" type="number" min="0.1" max="0.95" step="0.05" value="${c.min_score}"></label>
            <label>Point of an object that crosses <select name="anchor"><option value="bottom">where it touches the ground</option><option value="centre">its centre (camera above, looking down)</option></select></label></div>
          <h3>Counting lines</h3>
          <div class="table-wrap"><table class="oc-lines"><thead><tr><th>Line</th><th>Entrance</th><th>Counts</th><th></th></tr></thead><tbody id="ocLines"></tbody></table></div>
          <h3>Speed sections</h3>
          <div class="table-wrap"><table><thead><tr><th>Section</th><th>From line</th><th>To line</th><th>Distance (m)</th><th>Limit (km/h)</th><th>Area</th><th></th></tr></thead><tbody id="ocSections"></tbody></table></div>
          <p class="muted small">Speed = distance between the two lines on the ground ÷ time between crossing them. At 10 pictures per second over 10 m at 50 km/h one picture is 0.1 s ≈ ±14 %; the crossing moments are interpolated between pictures, so steady traffic is usually within a few per cent. Indicative only, not certified speed enforcement; no number plates or persons are recorded.</p>
          ${editable ? '<label>Reason (audit trail) <input name="reason" required maxlength="200"></label><div class="err" id="ocErr"></div><div class="actions"><button class="btn primary" type="submit">Save</button><button class="btn" type="button" id="ocClose">Close</button></div>' : '<div class="actions"><button class="btn" type="button" id="ocClose">Close</button></div>'}
        </form>`;
    const f = $('#ocForm'); f.model.value = c.model; f.anchor.value = CFG.anchor || 'bottom';
    if (!editable) for (const el of f.elements) el.disabled = el.id !== 'ocClose';
    $('#ocClose').onclick = () => { card.hidden = true; CAM = null; clearInterval(POLL); stopLive(); history.replaceState(null, '', '/counting/cameras'); };
    $('#ocBoxes').onchange = (e) => { SHOW_BOXES = e.target.checked; draw(); };
    $('#ocSize').value = size();
    $('#ocSize').onchange = (e) => { store('editorSize', e.target.value); sizeEd(); };
    $('#ocPicBtn').onclick = grab;
    $('#ocLiveOn').onchange = (e) => { store('editorLive', e.target.checked ? '1' : '0'); $('#ocPicBtn').hidden = e.target.checked; picture(); };
    if (editable) {
      $('#ocAdd').onclick = () => { ADDING = { pts: [] }; hint(t('Click the start (A) of the line on the picture')); };
      $('#ocAddSpeed').onclick = () => {
        if (CFG.lines.length < 2) { ctrl32.toast(t('Draw two lines across the road first'), 'bad'); return; }
        const area = (CTX.areas.find((a) => a.kind === 'car_park') || CTX.areas[0] || {}).id || '';
        CFG.sections.push({ id: 's' + uid(), name: t('Speed section') + ' ' + (CFG.sections.length + 1), line_a: CFG.lines[0].id, line_b: CFG.lines[1].id, distance_m: 10, limit_kmh: 0, area_id: area });
        tables();
      };
      f.onsubmit = save;
    }
    history.replaceState(null, '', '/counting/cameras?camera=' + id);
    FLASH = {}; LAST = Date.now(); HAVE_PIC = false;
    sizeEd(); tables(); draw(); status(c.status); picture();
    clearInterval(POLL);
    // with the live video the detected objects and the crossings follow it closely; with a still picture less often
    let n = 0;
    POLL = setInterval(async () => {
      if (document.hidden || !CAM || (!PLAYER && n++ % 5)) return;
      try { const s = await api('GET', `/api/v1/counting/cameras/${CAM.camera_id}`); if (CAM && s.camera_id === CAM.camera_id) status(s.status); } catch (e) { /* keep */ }
    }, 1000);
    card.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  function status(st) {
    const s = st.stats;
    BOXES = (s && s.boxes) || [];
    for (const p of st.recent || []) { // crossings since the editor was opened light up their line
      const at = new Date(p.t).getTime();
      if (at <= LAST) continue;
      LAST = at; FLASH[p.line] = { dir: p.dir === 'in' ? 'in' : 'out', at: Date.now() };
      setTimeout(draw, 1600);
    }
    const counts = s && s.counts ? Object.entries(s.counts).map(([k, v]) => `${t(CLASS_LABEL[k] || k)} ${nf(v.in || 0)}↓ ${nf(v.out || 0)}↑`).join(', ') : '';
    $('#ocStatus').innerHTML = `<b>State</b><span>${badge(st)} ${st.error ? `<span class="muted">${esc(st.error)}</span>` : ''}</span>` +
      (st.stream ? `<b>Stream analysed</b><span class="notranslate">${esc(st.stream)}</span>` : '') +
      (st.runtime ? `<b>Detector</b><span class="notranslate">${esc((st.model || '') + ' · ' + st.runtime)}</span>` : '') +
      (s ? `<b>Pictures per second</b><span>${nf(s.fps, 1)} <span class="muted">(${nf(s.frames_in)} ${esc(t('decoded'))})</span></span><b>Inference</b><span>${nf(s.infer_ms, 1)} ms</span><b>CPU</b><span>${nf(s.cpu_percent, 1)} % <span class="muted">(${esc(t('of one core'))}: ${esc(t('detector'))} ${nf(s.worker_cpu, 1)} %, ${esc(t('decoder'))} ${nf(s.decoder_cpu, 1)} %)</span></span><b>Objects followed</b><span>${nf(s.tracks)}</span>` + (counts ? `<b>Counted since start</b><span>${esc(counts)}</span>` : '') : '');
    const cpu = document.querySelector(`[data-cpu="${st && CAM ? CAM.camera_id : ''}"]`);
    if (cpu && s) cpu.textContent = nf(s.cpu_percent, 0) + ' %';
    draw();
  }

  function hint(s) { $('#ocHint').textContent = s || ''; $('#ocHint').hidden = !s; }

  function picture() { if (liveOn()) live(); else { stopLive(); HAVE_PIC = !$('#ocPic').hidden; draw(); grab(); } }

  // the lines are fractions of the analysed sub-stream: the live video is shown in its own proportions, filled, so
  // they match; the lines are drawn and edited on top of it
  function live() {
    stopLive();
    $('#ocPic').hidden = true; HAVE_PIC = false;
    const host = $('#ocLive'), id = CAM.camera_id;
    const fit = () => {
      const vp = host.querySelector('.vp'), m = vp && /^(\d+)x(\d+)$/.exec(vp.dataset.size || '');
      if (!CAM || CAM.camera_id !== id) return;
      if (m && +m[2]) {
        if (!HAVE_PIC) hint('');
        HAVE_PIC = true; RATIO = +m[1] / +m[2];
        $('#ocEd').style.aspectRatio = `${m[1]} / ${m[2]}`; sizeEd();
        $('#ocSvg').setAttribute('viewBox', `0 0 ${W} ${H()}`);
        draw();
      }
    };
    hint(t('Loading the picture…'));
    PLAYER = ctrl32Player(host, { url: `/api/v1/video/cameras/${id}/live?quality=sub`, live: true, onState: () => setTimeout(fit, 0) });
  }
  function stopLive() {
    if (PLAYER) { try { PLAYER.close(); } catch (e) { /* closed */ } PLAYER = null; }
    const host = $('#ocLive'); if (host) host.innerHTML = '';
  }

  // a still picture from the live sub-stream (masks of privacy zones are burnt in like in the player)
  function grab() {
    const host = document.createElement('div');
    host.style.cssText = 'position:absolute;left:-9999px;width:640px;height:360px';
    document.body.appendChild(host);
    hint(t('Loading the picture…'));
    let done = false;
    const p = ctrl32Player(host, { url: `/api/v1/video/cameras/${CAM.camera_id}/live?quality=sub`, live: true });
    const finish = (ok) => { if (done) return; done = true; try { p.close(); } catch (e) { /* closed */ } host.remove(); if (!ok) hint(t('No picture from the camera (lines can still be drawn on the grid)')); };
    let tries = 0;
    const tick = setInterval(() => {
      tries++;
      const c = p.snapshot && p.snapshot();
      if (PLAYER) { clearInterval(tick); finish(true); return; } // switched to the live video meanwhile
      if (c && c.width) {
        clearInterval(tick); HAVE_PIC = true;
        const img = $('#ocPic');
        img.src = c.toDataURL('image/jpeg', 0.85); img.hidden = false;
        RATIO = c.width / c.height || 16 / 9;
        $('#ocEd').style.aspectRatio = `${c.width} / ${c.height}`; sizeEd();
        $('#ocSvg').setAttribute('viewBox', `0 0 ${W} ${H()}`);
        hint(''); finish(true); draw();
      } else if (tries > 40) { clearInterval(tick); finish(false); }
    }, 250);
  }

  function pt(e) {
    const r = $('#ocSvg').getBoundingClientRect();
    return { x: Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)), y: Math.max(0, Math.min(1, (e.clientY - r.top) / r.height)) };
  }
  const R3 = (v) => Math.round(v * 1000) / 1000;

  function draw() {
    const svg = $('#ocSvg'); if (!svg) return;
    const h = H();
    let g = '';
    const now = Date.now();
    if (!HAVE_PIC) for (let i = 1; i < 10; i++) g += `<line x1="${i * W / 10}" y1="0" x2="${i * W / 10}" y2="${h}" stroke="#333"/><line x1="0" y1="${i * h / 10}" x2="${W}" y2="${i * h / 10}" stroke="#333"/>`;
    if (SHOW_BOXES) for (const b of BOXES) g += `<rect x="${b[1] * W}" y="${b[2] * h}" width="${(b[3] - b[1]) * W}" height="${(b[4] - b[2]) * h}" class="oc-box${VEHICLES.includes(b[0]) ? ' veh' : ''}"><title>${esc(t(CLASS_LABEL[b[0]] || b[0]))}</title></rect>`;
    CFG.lines.forEach((l, i) => {
      const ax = l.a.x * W, ay = l.a.y * h, bx = l.b.x * W, by = l.b.y * h, mx = (ax + bx) / 2, my = (ay + by) / 2;
      const dx = bx - ax, dy = by - ay, n = Math.hypot(dx, dy) || 1, s = l.in === 'right' ? 1 : -1;
      const nx = -dy / n * s, ny = dx / n * s; // towards the area (the "in" side)
      const tip = [mx + nx * 34, my + ny * 34], b1 = [mx + nx * 12 - dx / n * 10, my + ny * 12 - dy / n * 10], b2 = [mx + nx * 12 + dx / n * 10, my + ny * 12 + dy / n * 10];
      const sel = i === SEL ? ' sel' : '', f = FLASH[l.id], lit = f && now - f.at < 1500 ? ' ' + f.dir : '';
      g += `<line x1="${ax}" y1="${ay}" x2="${bx}" y2="${by}" class="oc-line${sel}${lit}" data-line="${i}" style="cursor:pointer"/>`;
      if (lit) g += `<text x="${mx - nx * 40}" y="${my - ny * 40}" text-anchor="middle" class="oc-flash${lit}">${f.dir === 'in' ? '+1 ' + esc(t('in')) : '−1 ' + esc(t('out'))}</text>`;
      g += `<polygon points="${tip.join(',')} ${b1.join(',')} ${b2.join(',')}" class="oc-arrow${sel}"/>`;
      g += `<text x="${ax + 6}" y="${ay - 8}" class="oc-lbl">A · ${esc(l.name)}</text><text x="${bx + 6}" y="${by - 8}" class="oc-lbl">B</text><text x="${tip[0] + 4}" y="${tip[1] + 4}" class="oc-lbl">${esc(t('in'))}</text>`;
      g += `<circle cx="${ax}" cy="${ay}" r="8" class="oc-end" data-end="${i}:a"/><circle cx="${bx}" cy="${by}" r="8" class="oc-end" data-end="${i}:b"/>`;
    });
    if (ADDING && ADDING.pts.length === 1) g += `<circle cx="${ADDING.pts[0].x * W}" cy="${ADDING.pts[0].y * h}" r="7" fill="#22d3ee"/>`;
    svg.innerHTML = g;
    if (!CTX.can('config.write')) return;
    svg.onpointerdown = (e) => {
      const p = pt(e), end = e.target.dataset && e.target.dataset.end, ln = e.target.dataset && e.target.dataset.line;
      if (ADDING) {
        ADDING.pts.push(p);
        if (ADDING.pts.length === 1) { hint(t('Click the end (B) of the line')); draw(); return; }
        const classes = CTX.classes.filter((c) => c === 'person');
        CFG.lines.push({ id: 'l' + uid(), name: t('Line') + ' ' + (CFG.lines.length + 1), a: { x: R3(ADDING.pts[0].x), y: R3(ADDING.pts[0].y) }, b: { x: R3(ADDING.pts[1].x), y: R3(ADDING.pts[1].y) }, in: 'right', classes, entrance_id: '' });
        SEL = CFG.lines.length - 1; ADDING = null; hint(''); tables(); draw(); return;
      }
      if (end) {
        const [i, which] = end.split(':'); SEL = +i;
        svg.setPointerCapture(e.pointerId);
        svg.onpointermove = (m) => { const q = pt(m); CFG.lines[i][which] = { x: R3(q.x), y: R3(q.y) }; draw(); };
        svg.onpointerup = () => { svg.onpointermove = null; svg.onpointerup = null; tables(); };
        return;
      }
      if (ln != null) { SEL = +ln; tables(); draw(); }
    };
  }

  function entranceOptions(sel) {
    return `<option value="">${esc(t('— speed line only'))}</option>` + CTX.areas.flatMap((a) => a.entrances.map((e) => `<option value="${e.id}"${e.id === sel ? ' selected' : ''}>${esc(a.name)} · ${esc(e.name)}</option>`)).join('');
  }
  function tables() {
    const ed = CTX.can('config.write') ? '' : ' disabled';
    $('#ocLines').innerHTML = CFG.lines.map((l, i) => `<tr class="${i === SEL ? 'sel' : ''}"><td><input type="text" value="${esc(l.name)}" data-k="name" data-i="${i}" maxlength="40" class="notranslate"${ed}></td>
      <td><select data-k="entrance_id" data-i="${i}" class="notranslate"${ed}>${entranceOptions(l.entrance_id)}</select></td>
      <td class="oc-cls">${CTX.classes.map((c) => `<label><input type="checkbox" data-cls="${c}" data-i="${i}"${(l.classes || []).includes(c) ? ' checked' : ''}${ed}> ${esc(t(CLASS_LABEL[c]))}</label>`).join('')}</td>
      <td><button class="btn small" type="button" data-swap="${i}" title="${esc(t('Swap the in and out side'))}"${ed}>⇅ in/out</button> <button class="btn small danger" type="button" data-rm="${i}"${ed}>✕</button></td></tr>`).join('') || `<tr><td colspan="4" class="muted">${esc(t('No lines yet: Add line, then click the two ends on the picture.'))}</td></tr>`;
    const lineOpts = (sel) => CFG.lines.map((l) => `<option value="${l.id}"${l.id === sel ? ' selected' : ''}>${esc(l.name)}</option>`).join('');
    $('#ocSections').innerHTML = CFG.sections.map((s, i) => `<tr><td><input type="text" value="${esc(s.name)}" data-sk="name" data-i="${i}" maxlength="60" class="notranslate"${ed}></td>
      <td><select data-sk="line_a" data-i="${i}" class="notranslate"${ed}>${lineOpts(s.line_a)}</select></td><td><select data-sk="line_b" data-i="${i}" class="notranslate"${ed}>${lineOpts(s.line_b)}</select></td>
      <td><input type="number" min="1" max="500" step="0.1" value="${s.distance_m}" data-sk="distance_m" data-i="${i}" style="width:6em"${ed}></td>
      <td><input type="number" min="0" max="300" step="1" value="${s.limit_kmh || 0}" data-sk="limit_kmh" data-i="${i}" style="width:5em"${ed}></td>
      <td><select data-sk="area_id" data-i="${i}" class="notranslate"${ed}>${CTX.areas.map((a) => `<option value="${a.id}"${a.id === s.area_id ? ' selected' : ''}>${esc(a.name)}</option>`).join('')}</select></td>
      <td><button class="btn small danger" type="button" data-srm="${i}"${ed}>✕</button></td></tr>`).join('') || `<tr><td colspan="7" class="muted">${esc(t('None'))}</td></tr>`;
    for (const el of document.querySelectorAll('#ocLines [data-k]')) el.onchange = () => { CFG.lines[el.dataset.i][el.dataset.k] = el.value; draw(); };
    for (const el of document.querySelectorAll('#ocLines [data-cls]')) el.onchange = () => {
      const l = CFG.lines[el.dataset.i]; l.classes = l.classes || [];
      if (el.checked) { if (!l.classes.includes(el.dataset.cls)) l.classes.push(el.dataset.cls); } else l.classes = l.classes.filter((c) => c !== el.dataset.cls);
    };
    for (const b of document.querySelectorAll('[data-swap]')) b.onclick = () => { const l = CFG.lines[b.dataset.swap]; l.in = l.in === 'right' ? 'left' : 'right'; SEL = +b.dataset.swap; draw(); };
    for (const b of document.querySelectorAll('[data-rm]')) b.onclick = () => { const id = CFG.lines[b.dataset.rm].id; CFG.lines.splice(+b.dataset.rm, 1); CFG.sections = CFG.sections.filter((s) => s.line_a !== id && s.line_b !== id); SEL = -1; tables(); draw(); };
    for (const el of document.querySelectorAll('#ocSections [data-sk]')) el.onchange = () => { const k = el.dataset.sk; CFG.sections[el.dataset.i][k] = ['distance_m', 'limit_kmh'].includes(k) ? +el.value : el.value; };
    for (const b of document.querySelectorAll('[data-srm]')) b.onclick = () => { CFG.sections.splice(+b.dataset.srm, 1); tables(); };
  }

  async function save(e) {
    e.preventDefault(); const f = e.target;
    CFG.anchor = f.anchor.value;
    const body = { enabled: f.enabled.checked, fps: +f.fps.value, model: f.model.value, min_score: +f.min_score.value, config: CFG, reason: f.reason.value };
    try {
      const r = await api('PUT', `/api/v1/counting/cameras/${CAM.camera_id}`, body);
      CFG = r.config; CFG.sections = CFG.sections || [];
      ctrl32.toast(t('saved'), 'ok'); $('#ocErr').textContent = ''; f.reason.value = '';
      list();
    } catch (x) { $('#ocErr').textContent = x.message; }
  }

  window.ctrl32Counting = { cameras(ctx) { CTX = ctx; list(); } };
})();
