// ctrl32 telemetry — object counting (spec 029): the overview of an area (occupancy now, arrivals and departures by
// hour, entrances with their shares, the days before, vehicle speeds, resets and corrections) and the configuration:
// areas and entrances, counting lines and speed sections drawn on a camera picture, sensors.
(() => {
  const $ = (s) => document.querySelector(s), esc = ctrl32.esc, api = ctrl32.api, t = ctrl32.t;
  const nf = (v, d = 0) => new Intl.NumberFormat(document.documentElement.lang || undefined, { maximumFractionDigits: d, minimumFractionDigits: 0 }).format(v || 0);
  const GROUP_LABEL = { people: 'people', vehicles: 'vehicles', cars: 'cars', trucks: 'trucks', buses: 'buses', motorcycles: 'motorcycles', bicycles: 'bicycles' };
  const CLASS_LABEL = { person: 'person', bicycle: 'bicycle', car: 'car', motorcycle: 'motorcycle', bus: 'bus', truck: 'truck' };
  const KIND_LABEL = { building: 'Building', car_park: 'Car park', zone: 'Zone' };
  let ME = null, AREAS = [], GROUPS = [], CLASSES = [], AREA = null, GROUP = 'people', DATE = null, DATA = null, INSTALLED = false, INSTALL_NOTE = '';
  const can = (p) => !!(ME && (ME.permissions || []).includes(p));
  const iso = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const store = (k, v) => { try { localStorage.setItem('ctrl32.counting.' + k, v); } catch (e) { /* private mode */ } };
  const stored = (k) => { try { return localStorage.getItem('ctrl32.counting.' + k); } catch (e) { return null; } };
  const fmtTz = (d, o) => new Date(d).toLocaleString(document.documentElement.lang || undefined, Object.assign({ timeZone: (DATA && DATA.tz) || undefined }, o));

  // ---- overview ------------------------------------------------------------------------------------------------
  async function loadAreas() {
    const r = await api('GET', '/api/v1/counting/areas');
    AREAS = r.areas; GROUPS = r.groups; CLASSES = r.classes; INSTALLED = r.cameras_installed; INSTALL_NOTE = r.cameras_note;
    return r;
  }
  function renderSites() {
    const bySite = {};
    for (const a of AREAS) {
      const s = bySite[a.site] || (bySite[a.site] = { occ: {}, n: 0 });
      s.n++;
      for (const [g, v] of Object.entries(a.occupancy)) s.occ[g] = (s.occ[g] || 0) + v;
    }
    $('#sites').innerHTML = Object.entries(bySite).filter(([, s]) => s.n > 1).map(([name, s]) => `<span class="badge"><span>Now on the site</span> <span class="notranslate">${esc(name)}</span>: ${Object.entries(s.occ).sort((x, y) => GROUPS.indexOf(x[0]) - GROUPS.indexOf(y[0])).map(([g, v]) => `${nf(v)} <span>${esc(t(GROUP_LABEL[g] || g))}</span>`).join(', ')}</span>`).join('');
  }
  function fillAreaSelect() {
    const sel = $('#area');
    sel.innerHTML = AREAS.map((a) => `<option value="${a.id}">${esc(a.name)} · ${esc(a.site)}</option>`).join('');
    const want = new URLSearchParams(location.search).get('area') || stored('area');
    AREA = AREAS.find((a) => a.id === want) || AREAS[0] || null;
    if (AREA) sel.value = AREA.id;
    $('#empty').hidden = !!AREA; $('#view').hidden = !AREA;
  }
  function renderGroups() {
    if (!AREA) return;
    if (!AREA.groups.includes(GROUP)) GROUP = AREA.groups[0];
    $('#groups').innerHTML = AREA.groups.map((g) => `<button type="button" data-g="${g}" class="${g === GROUP ? 'on' : ''}">${esc(t(GROUP_LABEL[g] || g))}</button>`).join('');
    for (const b of document.querySelectorAll('#groups button')) b.onclick = () => { GROUP = b.dataset.g; store('group', GROUP); render(); };
  }
  async function loadSummary() {
    if (!AREA) return;
    const q = `/api/v1/counting/summary?area=${AREA.id}${DATE ? '&date=' + DATE : ''}&days=7`;
    try { DATA = await api('GET', q); AREA = DATA.area; render(); } catch (x) { ctrl32.toast(x.message, 'bad'); }
    try {
      const log = await api('GET', `/api/v1/counting/log?area=${AREA.id}&limit=20`);
      $('#logTable tbody').innerHTML = log.length ? log.map((e) => `<tr><td>${esc(ctrl32.fmt(e.at))}</td><td>${esc(t(GROUP_LABEL[e.group] || e.group))}</td><td>${esc(t({ reset: 'daily reset', correction: 'correction', clamp: 'clamped at zero' }[e.kind] || e.kind))}</td><td class="num">${nf(e.before)}</td><td class="num">${nf(e.after)}</td><td class="notranslate">${esc(e.reason)}</td><td class="notranslate">${esc(e.actor)}</td></tr>`).join('') : `<tr><td colspan="7" class="muted">${esc(t('nothing yet'))}</td></tr>`;
    } catch (x) { /* the overview stays */ }
  }
  function render() {
    const d = DATA; if (!d) return;
    renderGroups();
    const a = d.area, g = GROUP, tot = d.totals[g] || { in: 0, out: 0 };
    const isToday = d.date === iso(new Date());
    $('#dayLabel').textContent = fmtTz(d.from, { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' });
    const occ = a.occupancy[g] || 0, cap = a.limits[g] || 0;
    $('#tOcc').textContent = isToday ? nf(occ) : '—';
    $('#tIn').textContent = nf(tot.in); $('#tOut').textContent = nf(tot.out); $('#tPeak').textContent = nf(d.peak[g] || 0);
    const capEl = $('#tCap');
    capEl.hidden = !cap || !isToday;
    if (cap) {
      const pct = Math.min(100, occ / cap * 100);
      capEl.className = 'oc-cap' + (pct >= 100 ? ' bad' : pct >= 80 ? ' warn' : '');
      capEl.firstElementChild.style.width = pct + '%';
      $('#tCapText').textContent = isToday ? `${nf(pct, 0)} % · ${t('capacity')} ${nf(cap)}` : '';
    } else $('#tCapText').textContent = '';
    $('#clampNote').hidden = !isToday || !((a.clamped || {})[g] > 0);
    hourChart(); dayChart(); entrances(); speeds(); liveCheck(isToday);
    $('#correctBtn').hidden = !can('config.write') || !isToday;
    $('#alarmBtn').hidden = !can('config.write') || !d.datastreams[g + '_occupancy'];
  }
  function bars(el, items, opts) {
    // items: [{label, a, b, line}] — two bars per slot (in, out) and an optional line (occupancy)
    const W = Math.max(320, Math.round(el.clientWidth || 960)), H = 240, L = 40, B = 24, T = 10, n = items.length || 1;
    const max = Math.max(1, ...items.map((i) => Math.max(i.a, i.b, i.line || 0)));
    const step = (W - L) / n, bw = Math.max(2, step * 0.36);
    const y = (v) => T + (H - T - B) * (1 - v / max);
    let g = '';
    for (let i = 0; i <= 4; i++) { const v = max * i / 4, yy = y(v); g += `<line x1="${L}" x2="${W}" y1="${yy}" y2="${yy}" class="oc-grid"/><text x="${L - 6}" y="${yy + 4}" text-anchor="end" class="oc-ax">${nf(v)}</text>`; }
    let path = '';
    items.forEach((it, i) => {
      const x = L + i * step + (step - 2 * bw) / 2;
      const tip = `${it.label}: ${t('in')} ${nf(it.a)}, ${t('out')} ${nf(it.b)}${it.line != null ? ', ' + t('occupancy') + ' ' + nf(it.line) : ''}`;
      g += `<rect x="${x}" y="${y(it.a)}" width="${bw}" height="${Math.max(0, H - B - y(it.a))}" class="oc-in" rx="2"><title>${esc(tip)}</title></rect>`;
      g += `<rect x="${x + bw}" y="${y(it.b)}" width="${bw}" height="${Math.max(0, H - B - y(it.b))}" class="oc-out" rx="2"><title>${esc(tip)}</title></rect>`;
      if (it.line != null) path += `${path ? 'L' : 'M'}${L + i * step + step / 2},${y(it.line)}`;
      const every = n > 24 ? 3 : n > 12 ? 2 : 1;
      if (i % every === 0) g += `<text x="${L + i * step + step / 2}" y="${H - 6}" text-anchor="middle" class="oc-ax">${esc(it.tick)}</text>`;
    });
    if (path) g += `<path d="${path}" class="oc-occ"/>`;
    el.innerHTML = `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="${esc(t(opts.label))}">${g}</svg>`;
  }
  function hourChart() {
    const g = GROUP, now = Date.now();
    let last = null;
    const items = DATA.hours.map((h) => {
      const st = new Date(h.start).getTime();
      if (h.occupancy[g] != null) last = h.occupancy[g];
      const line = st > now ? null : last;
      return { label: fmtTz(h.start, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }), tick: fmtTz(h.start, { hour: 'numeric', hourCycle: 'h23' }), a: h.in[g] || 0, b: h.out[g] || 0, line };
    });
    bars($('#hourChart'), items, { label: 'By hour' });
  }
  function dayChart() {
    const g = GROUP;
    const items = DATA.days.map((d) => ({ label: d.date, tick: new Date(d.date + 'T12:00:00').toLocaleDateString(document.documentElement.lang || undefined, { weekday: 'short', day: 'numeric' }), a: d.in[g] || 0, b: d.out[g] || 0, line: d.peak[g] }));
    bars($('#dayChart'), items, { label: 'Comparison of days' });
  }
  function entrances() {
    const g = GROUP;
    $('#entTable tbody').innerHTML = DATA.entrances.map((e) => {
      const c = e.counts[g] || { in: 0, out: 0 }, s = e.share[g] || { in: 0, out: 0 };
      return `<tr><td class="notranslate">${esc(e.name)}</td><td class="num">${nf(c.in)}</td><td class="num">${nf(c.out)}</td><td><div class="oc-share"><span style="width:${s.in}%"></span></div> ${nf(s.in, 1)} %</td><td><div class="oc-share"><span style="width:${s.out}%"></span></div> ${nf(s.out, 1)} %</td></tr>`;
    }).join('') || `<tr><td colspan="5" class="muted">${esc(t('No entrances'))}</td></tr>`;
  }
  function speeds() {
    const host = $('#speeds');
    host.innerHTML = (DATA.speeds || []).map((s) => {
      const max = Math.max(1, ...s.histogram);
      const W = 420, H = 120, bw = W / s.histogram.length;
      const hist = s.histogram.map((n, i) => `<rect x="${i * bw + 2}" y="${H - 18 - (H - 28) * n / max}" width="${bw - 4}" height="${(H - 28) * n / max}" class="oc-hist${s.limit_kmh && i * 10 >= s.limit_kmh ? ' over' : ''}"><title>${i * 10}–${i * 10 + 10} km/h: ${n}</title></rect><text x="${i * bw + bw / 2}" y="${H - 4}" text-anchor="middle" class="oc-ax">${i * 10}</text>`).join('');
      return `<div class="card"><h2 style="margin-top:0"><span>Vehicle speed</span> · <span class="notranslate">${esc(s.name)}</span> <span class="muted notranslate" style="font-size:13px">${esc(s.camera)}</span></h2>
        <div class="oc-tiles"><div class="card oc-tile"><div class="muted">Vehicles</div><b>${nf(s.count)}</b></div>
          <div class="card oc-tile"><div class="muted">Average speed</div><b>${s.avg_kmh != null ? nf(s.avg_kmh, 1) + ' km/h' : '—'}</b></div>
          <div class="card oc-tile"><div class="muted">Highest speed</div><b>${s.max_kmh != null ? nf(s.max_kmh, 1) + ' km/h' : '—'}</b></div>
          <div class="card oc-tile"><div class="muted">Over the limit</div><b class="${s.over_limit ? 'oc-over' : ''}">${nf(s.over_limit)}</b><div class="muted small">${s.limit_kmh ? esc(t('limit')) + ' ' + nf(s.limit_kmh) + ' km/h' : esc(t('no limit set'))}</div></div></div>
        <div class="oc-two"><div><div class="muted">Vehicles per speed (km/h)</div><svg viewBox="0 0 ${W} ${H}" style="width:100%;height:140px">${hist}</svg></div>
        <div class="table-wrap"><table><thead><tr><th>Time</th><th>Vehicle</th><th>Direction</th><th class="num">km/h</th></tr></thead><tbody>${s.last.map((v) => `<tr><td>${esc(fmtTz(v.at, { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }))}</td><td>${v.class ? esc(t(CLASS_LABEL[v.class] || v.class)) : '—'}</td><td>${esc(v.direction === 'ab' ? 'A → B' : 'B → A')}</td><td class="num ${v.over ? 'oc-over' : ''}">${nf(v.kmh, 1)}</td></tr>`).join('') || `<tr><td colspan="4" class="muted">${esc(t('nothing yet'))}</td></tr>`}</tbody></table></div></div>
        <p class="muted small">An indicative measurement from the camera picture (accuracy depends on the frame rate, the camera angle and the measured distance), not a certified speed measurement; no number plates or persons are recorded.</p></div>`;
    }).join('');
  }

  // ---- live check: the live picture of the area's counting cameras with their lines; every crossing (from the
  // camera's counting status, polled) flashes its line and goes to the list, so a person can check the counts by eye
  const LIVE = { key: '', cams: [], players: [], poll: null, last: {}, opened: 0 };
  function stopLive() {
    clearInterval(LIVE.poll); LIVE.poll = null;
    for (const p of LIVE.players) { try { p.close(); } catch (e) { /* closed */ } }
    LIVE.players = []; LIVE.cams = []; LIVE.key = '';
    $('#liveGrid').innerHTML = '';
  }
  async function liveCheck(isToday) {
    const card = $('#liveCard');
    const a = AREAS.find((x) => x.id === (AREA && AREA.id));
    if (!a || !isToday || !can('video.view')) { card.hidden = true; stopLive(); return; }
    let cams = [];
    try {
      const ents = new Set((a.entrances || []).map((e) => e.id));
      cams = (await api('GET', '/api/v1/counting/cameras')).filter((c) => c.enabled && c.camera_enabled && ((c.config || {}).lines || []).some((l) => ents.has(l.entrance_id)));
    } catch (x) { cams = []; }
    card.hidden = !cams.length;
    const on = $('#liveOn').checked = stored('live') === '1';
    $('#liveOn').onchange = (e) => { store('live', e.target.checked ? '1' : '0'); stopLive(); liveCheck(true); };
    const key = a.id + '|' + cams.map((c) => c.camera_id + JSON.stringify(c.config)).join('|');
    if (!on || !cams.length) { stopLive(); return; }
    if (LIVE.key === key) return; // already showing these cameras
    stopLive();
    LIVE.key = key; LIVE.cams = cams; LIVE.last = {}; LIVE.opened = Date.now();
    $('#liveGrid').innerHTML = cams.map((c) => `<div data-live="${c.camera_id}"><div class="muted notranslate" style="margin-bottom:4px">${esc(c.camera)}</div>
      <div class="oc-lc-stage"><div class="oc-lc-video"></div><svg viewBox="0 0 1000 562.5" preserveAspectRatio="none"></svg></div>
      <div class="oc-lc-counts"></div><ul class="oc-lc-list"><li class="muted">${esc(t('nothing yet'))}</li></ul></div>`).join('');
    for (const c of cams) {
      const stage = document.querySelector(`[data-live="${c.camera_id}"] .oc-lc-stage`);
      c.ratio = 16 / 9; c.flash = {}; c.counts = {};
      // the lines are fractions of the analysed sub-stream: show it in its own proportions, filled, so they match
      const fit = () => {
        const vp = stage.querySelector('.vp'), m = vp && /^(\d+)x(\d+)$/.exec(vp.dataset.size || '');
        if (m && +m[2]) { c.ratio = +m[1] / +m[2]; stage.style.aspectRatio = `${m[1]} / ${m[2]}`; stage.style.maxWidth = `calc(55vh * ${c.ratio})`; drawLive(c); }
      };
      LIVE.players.push(ctrl32Player(stage.querySelector('.oc-lc-video'), { url: `/api/v1/video/cameras/${c.camera_id}/live?quality=sub`, live: true, onState: () => setTimeout(fit, 0) }));
      drawLive(c);
    }
    pollLive();
    LIVE.poll = setInterval(() => { if (!document.hidden) pollLive(); }, 1500);
  }
  function drawLive(c) {
    const el = document.querySelector(`[data-live="${c.camera_id}"]`); if (!el) return;
    const W = 1000, h = W / c.ratio, now = Date.now(), svg = el.querySelector('.oc-lc-stage > svg');
    svg.setAttribute('viewBox', `0 0 ${W} ${h}`);
    let g = '';
    for (const l of c.config.lines || []) {
      const ax = l.a.x * W, ay = l.a.y * h, bx = l.b.x * W, by = l.b.y * h, mx = (ax + bx) / 2, my = (ay + by) / 2;
      const dx = bx - ax, dy = by - ay, n = Math.hypot(dx, dy) || 1, s = l.in === 'right' ? 1 : -1;
      const nx = -dy / n * s, ny = dx / n * s; // towards the area (the "in" side)
      const tip = [mx + nx * 34, my + ny * 34], b1 = [mx + nx * 12 - dx / n * 10, my + ny * 12 - dy / n * 10], b2 = [mx + nx * 12 + dx / n * 10, my + ny * 12 + dy / n * 10];
      const f = c.flash[l.id], lit = f && now - f.at < 1500 ? ' ' + f.dir : '';
      g += `<line x1="${ax}" y1="${ay}" x2="${bx}" y2="${by}" class="oc-line${lit}"/><polygon points="${tip.join(',')} ${b1.join(',')} ${b2.join(',')}" class="oc-arrow"/>`;
      g += `<text x="${ax + 6}" y="${ay - 8}" class="oc-lbl">${esc(l.name)}</text><text x="${tip[0] + 4}" y="${tip[1] + 4}" class="oc-lbl">${esc(t('in'))}</text>`;
      if (lit) g += `<text x="${mx - nx * 40}" y="${my - ny * 40}" text-anchor="middle" class="oc-flash${lit}">${f.dir === 'in' ? '+1 ' + esc(t('in')) : '−1 ' + esc(t('out'))}</text>`;
    }
    svg.innerHTML = g;
    el.querySelector('.oc-lc-counts').innerHTML = (c.config.lines || []).map((l) => {
      const k = c.counts[l.id] || { in: 0, out: 0 };
      return `<span><span class="notranslate">${esc(l.name)}</span>: <b>${nf(k.in)}</b> ${esc(t('in'))} · <b>${nf(k.out)}</b> ${esc(t('out'))}</span>`;
    }).join('') + `<span class="muted small">${esc(t('since the page was opened'))}</span>`;
  }
  async function pollLive() {
    for (const c of LIVE.cams) {
      let st;
      try { st = (await api('GET', `/api/v1/counting/cameras/${c.camera_id}`)).status || {}; } catch (x) { continue; }
      const el = document.querySelector(`[data-live="${c.camera_id}"]`); if (!el) return; // stopped meanwhile
      const recent = st.recent || [], names = Object.fromEntries((c.config.lines || []).map((l) => [l.id, l.name]));
      const last = LIVE.last[c.camera_id];
      for (const p of recent) {
        const at = new Date(p.t).getTime();
        if (last == null ? at < LIVE.opened : at <= last) continue;
        const dir = p.dir === 'in' ? 'in' : 'out';
        (c.counts[p.line] = c.counts[p.line] || { in: 0, out: 0 })[dir]++;
        c.flash[p.line] = { dir, at: Date.now() };
        setTimeout(() => drawLive(c), 1600);
      }
      LIVE.last[c.camera_id] = Math.max(last || LIVE.opened, ...recent.map((p) => new Date(p.t).getTime()));
      el.querySelector('.oc-lc-list').innerHTML = recent.slice().reverse().slice(0, 20).map((p) => `<li>${esc(fmtTz(p.t, { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }))} · <span class="notranslate">${esc(names[p.line] || p.line)}</span> · <span class="${p.dir === 'in' ? 'in' : 'out'}">${esc(t(p.dir === 'in' ? 'in' : 'out'))}</span> · ${esc(t(CLASS_LABEL[p.cls] || p.cls))}</li>`).join('') || `<li class="muted">${esc(t(st.state === 'running' ? 'nothing yet' : 'not counting now'))}</li>`;
      drawLive(c);
    }
  }

  function shiftDay(n) {
    const d = DATE ? new Date(DATE + 'T12:00:00') : new Date();
    d.setDate(d.getDate() + n);
    DATE = iso(d) === iso(new Date()) ? null : iso(d);
    loadSummary();
  }
  function exportQuery(form) {
    const ids = DATA.datastreams, g = GROUP, list = [], labels = [];
    for (const [k, l] of [[g + '_in', t('in')], [g + '_out', t('out')]]) if (ids[k]) { list.push(ids[k]); labels.push(`${t(GROUP_LABEL[g])} ${l}`); }
    return { list, labels };
  }
  function wireOverview() {
    $('#area').onchange = () => { AREA = AREAS.find((a) => a.id === $('#area').value); store('area', AREA.id); DATE = null; loadSummary(); };
    $('#prev').onclick = () => shiftDay(-1); $('#next').onclick = () => shiftDay(1); $('#today').onclick = () => { DATE = null; loadSummary(); };
    $('#correctBtn').onclick = () => {
      const f = $('#corrForm'); f.reset(); $('#corrErr').textContent = '';
      f.group.innerHTML = AREA.groups.map((g) => `<option value="${g}"${g === GROUP ? ' selected' : ''}>${esc(t(GROUP_LABEL[g]))}</option>`).join('');
      f.value.value = AREA.occupancy[GROUP] || 0;
      $('#corrDlg').showModal();
    };
    $('#corrForm').onsubmit = async (e) => {
      e.preventDefault(); const f = e.target;
      try { await api('POST', `/api/v1/counting/areas/${AREA.id}/occupancy`, { group: f.group.value, value: +f.value.value, reason: f.reason.value }); $('#corrDlg').close(); ctrl32.toast(t('occupancy corrected'), 'ok'); loadSummary(); } catch (x) { $('#corrErr').textContent = x.message; }
    };
    $('#alarmBtn').onclick = () => {
      const f = $('#alarmForm'); f.reset(); $('#alarmErr').textContent = '';
      const cap = AREA.limits[GROUP]; if (cap) { f.action.value = cap; f.warning.value = Math.round(cap * 0.9); }
      $('#alarmDlg').showModal();
    };
    $('#alarmForm').onsubmit = async (e) => {
      e.preventDefault(); const f = e.target;
      const body = { datastream_id: DATA.datastreams[GROUP + '_occupancy'], type: 'high', action: +f.action.value, reason: f.reason.value };
      if (f.warning.value !== '') body.warning = +f.warning.value;
      try { await api('POST', '/api/v1/alarm-rules', body); $('#alarmDlg').close(); ctrl32.toast(t('alarm rule created'), 'ok'); } catch (x) { $('#alarmErr').textContent = x.message; }
    };
    $('#exportBtn').onclick = () => $('#exportDlg').showModal();
    for (const b of document.querySelectorAll('#exportForm [data-f]')) b.onclick = () => {
      const f = $('#exportForm'), ids = DATA.datastreams, g = GROUP;
      const counts = [ids[g + '_in'], ids[g + '_out']].filter(Boolean);
      const q = new URLSearchParams({ datastreams: counts.join(','), interval: f.interval.value, values: 'sum', period: f.period.value, total: '1', format: b.dataset.f, title: `${AREA.name} — ${t(GROUP_LABEL[g])}` });
      q.append('label', `${t(GROUP_LABEL[g])} ${t('in')}`); q.append('label', `${t(GROUP_LABEL[g])} ${t('out')}`);
      location.href = '/api/v1/period-table/export?' + q;
    };
  }

  // ---- areas ---------------------------------------------------------------------------------------------------
  let SITES = [], EDIT = null;
  function renderAreas() {
    $('#newArea').hidden = !can('config.write');
    $('#areaTable tbody').innerHTML = AREAS.map((a) => `<tr><td class="notranslate"><a href="/counting?area=${a.id}">${esc(a.name)}</a></td><td class="notranslate">${esc(a.site)}</td><td>${esc(t(KIND_LABEL[a.kind]))}</td><td>${a.groups.map((g) => `<span class="badge">${esc(t(GROUP_LABEL[g]))}</span>`).join(' ')}</td><td class="notranslate">${esc(a.entrances.map((e) => e.name).join(', '))}</td><td>${esc(a.reset_time || '—')}</td><td>${can('config.write') ? `<button class="btn small" type="button" data-edit="${a.id}">Edit</button>` : ''}</td></tr>`).join('') || `<tr><td colspan="7" class="muted">${esc(t('No areas yet'))}</td></tr>`;
    for (const b of document.querySelectorAll('[data-edit]')) b.onclick = () => openArea(AREAS.find((a) => a.id === b.dataset.edit));
  }
  async function openArea(a) {
    EDIT = a || null;
    if (!SITES.length) SITES = await api('GET', '/api/v1/sites');
    const f = $('#areaForm'); f.reset(); $('#areaErr').textContent = '';
    $('#areaDlgTitle').textContent = t(a ? 'Edit area' : 'New area');
    f.site_id.innerHTML = SITES.map((s) => `<option value="${s.id}">${esc(s.name)}</option>`).join('');
    f.site_id.disabled = !!a;
    const groups = a ? a.groups : ['people'];
    $('#groupChecks').innerHTML = GROUPS.map((g) => `<label><input type="checkbox" value="${g}"${groups.includes(g) ? ' checked' : ''}> ${esc(t(GROUP_LABEL[g]))}</label>`).join('');
    if (a) { f.name.value = a.name; f.site_id.value = a.site_id; f.kind.value = a.kind; f.reset_time.value = a.reset_time; f.capacity.value = a.limits[a.groups[0]] || ''; }
    else f.reset_time.value = '03:00';
    $('#entNewLbl').hidden = !!a; $('#entEdit').hidden = !a; $('#areaArchive').hidden = !a;
    if (a) renderEntrances(a);
    $('#areaDlg').showModal();
  }
  function renderEntrances(a) {
    $('#entList').innerHTML = a.entrances.map((e) => `<div class="actions" style="margin:4px 0"><input value="${esc(e.name)}" data-ent="${e.id}" maxlength="80" class="notranslate"><button class="btn small" type="button" data-ren="${e.id}">Rename</button><button class="btn small danger" type="button" data-del="${e.id}">Archive</button></div>`).join('');
    const reason = () => $('#areaForm').reason.value || prompt(t('Reason (audit trail)'));
    for (const b of document.querySelectorAll('[data-ren]')) b.onclick = async () => { const r = reason(); if (!r) return; try { await api('PUT', `/api/v1/counting/entrances/${b.dataset.ren}`, { name: document.querySelector(`[data-ent="${b.dataset.ren}"]`).value, reason: r }); ctrl32.toast(t('saved'), 'ok'); await refresh(); } catch (x) { $('#areaErr').textContent = x.message; } };
    for (const b of document.querySelectorAll('[data-del]')) b.onclick = async () => { const r = reason(); if (!r) return; try { await api('DELETE', `/api/v1/counting/entrances/${b.dataset.del}?reason=${encodeURIComponent(r)}`); await refresh(); renderEntrances(AREAS.find((x) => x.id === EDIT.id)); } catch (x) { $('#areaErr').textContent = x.message; } };
  }
  function wireAreas() {
    $('#newArea').onclick = () => openArea(null);
    $('#entAdd').onclick = async () => {
      const name = $('#entNewName').value.trim(), r = $('#areaForm').reason.value;
      if (!name) return;
      if (!r) { $('#areaErr').textContent = t('reason required'); return; }
      try { await api('POST', `/api/v1/counting/areas/${EDIT.id}/entrances`, { name, reason: r }); $('#entNewName').value = ''; await refresh(); renderEntrances(AREAS.find((x) => x.id === EDIT.id)); } catch (x) { $('#areaErr').textContent = x.message; }
    };
    $('#areaArchive').onclick = async () => {
      const r = $('#areaForm').reason.value; if (!r) { $('#areaErr').textContent = t('reason required'); return; }
      if (!(await ctrl32.confirm(t('Archive this area? Its counts stay, counting stops.')))) return;
      try { await api('DELETE', `/api/v1/counting/areas/${EDIT.id}?reason=${encodeURIComponent(r)}`); $('#areaDlg').close(); await refresh(); } catch (x) { $('#areaErr').textContent = x.message; }
    };
    $('#areaForm').onsubmit = async (e) => {
      e.preventDefault(); const f = e.target;
      const groups = [...document.querySelectorAll('#groupChecks input:checked')].map((c) => c.value);
      const limits = {}; if (f.capacity.value !== '' && groups.length) limits[groups[0]] = +f.capacity.value;
      const body = { name: f.name.value, kind: f.kind.value, groups, reset_time: f.reset_time.value, limits, reason: f.reason.value };
      try {
        if (EDIT) await api('PUT', `/api/v1/counting/areas/${EDIT.id}`, body);
        else await api('POST', '/api/v1/counting/areas', Object.assign(body, { site_id: f.site_id.value, entrances: f.entrances.value.split('\n').map((s) => s.trim()).filter(Boolean) }));
        $('#areaDlg').close(); ctrl32.toast(t('saved'), 'ok'); await refresh();
      } catch (x) { $('#areaErr').textContent = x.message; }
    };
  }

  // ---- sensors -------------------------------------------------------------------------------------------------
  let INPUTS = [], DS = [], EDIT_IN = null;
  async function loadInputs() {
    INPUTS = await api('GET', '/api/v1/counting/inputs');
    $('#newInput').hidden = !can('config.write');
    $('#inputTable tbody').innerHTML = INPUTS.map((i) => `<tr><td class="notranslate">${esc(i.name)}</td><td class="notranslate">${esc(i.area)}</td><td class="notranslate">${esc(i.entrance)}</td><td>${esc(t(GROUP_LABEL[i.group]))}</td><td>${esc(t(i.mode === 'total' ? 'running total' : 'number of passages since the last message'))}</td><td>${i.enabled ? '<span class="badge ok">on</span>' : '<span class="badge">off</span>'}</td><td>${can('config.write') ? `<button class="btn small" type="button" data-in="${i.id}">Edit</button>` : ''}</td></tr>`).join('') || `<tr><td colspan="7" class="muted">${esc(t('No sensors yet'))}</td></tr>`;
    for (const b of document.querySelectorAll('[data-in]')) b.onclick = () => openInput(INPUTS.find((i) => i.id === b.dataset.in));
  }
  async function openInput(i) {
    EDIT_IN = i || null;
    if (!DS.length) DS = (await api('GET', '/api/v1/datastreams')).filter((d) => !['counting_area', 'entrance', 'speed_section'].includes(d.asset_type));
    const f = $('#inputForm'); f.reset(); $('#inputErr').textContent = '';
    f.entrance_id.innerHTML = AREAS.flatMap((a) => a.entrances.map((e) => `<option value="${e.id}">${esc(a.name)} · ${esc(e.name)}</option>`)).join('');
    f.group.innerHTML = GROUPS.map((g) => `<option value="${g}">${esc(t(GROUP_LABEL[g]))}</option>`).join('');
    const opts = '<option value="">—</option>' + DS.map((d) => `<option value="${d.id}">${esc(d.asset)} / ${esc(d.key)}</option>`).join('');
    f.in_datastream_id.innerHTML = opts; f.out_datastream_id.innerHTML = opts;
    if (i) { f.name.value = i.name; f.entrance_id.value = i.entrance_id; f.group.value = i.group; f.in_datastream_id.value = i.in_datastream_id; f.out_datastream_id.value = i.out_datastream_id; f.mode.value = i.mode; f.enabled.checked = i.enabled; }
    $('#inputDelete').hidden = !i;
    $('#inputDlg').showModal();
  }
  function wireInputs() {
    $('#newInput').onclick = () => openInput(null);
    $('#inputForm').onsubmit = async (e) => {
      e.preventDefault(); const f = e.target;
      const body = { name: f.name.value, entrance_id: f.entrance_id.value, group: f.group.value, in_datastream_id: f.in_datastream_id.value, out_datastream_id: f.out_datastream_id.value, mode: f.mode.value, enabled: f.enabled.checked, reason: f.reason.value };
      try { if (EDIT_IN) await api('PUT', `/api/v1/counting/inputs/${EDIT_IN.id}`, body); else await api('POST', '/api/v1/counting/inputs', body); $('#inputDlg').close(); ctrl32.toast(t('saved'), 'ok'); loadInputs(); } catch (x) { $('#inputErr').textContent = x.message; }
    };
    $('#inputDelete').onclick = async () => {
      const r = $('#inputForm').reason.value; if (!r) { $('#inputErr').textContent = t('reason required'); return; }
      try { await api('DELETE', `/api/v1/counting/inputs/${EDIT_IN.id}?reason=${encodeURIComponent(r)}`); $('#inputDlg').close(); loadInputs(); } catch (x) { $('#inputErr').textContent = x.message; }
    };
  }

  async function refresh() {
    await loadAreas();
    const sec = ctrl32.section('overview', ['overview', 'areas', 'cameras', 'sensors']);
    if (sec === 'overview') { renderSites(); fillAreaSelect(); if (AREA) await loadSummary(); }
    if (sec !== 'overview') stopLive();
    if (sec === 'areas') renderAreas();
    if (sec === 'sensors') loadInputs();
    if (sec === 'cameras' && window.ctrl32Counting) window.ctrl32Counting.cameras({ areas: AREAS, classes: CLASSES, installed: INSTALLED, note: INSTALL_NOTE, can });
  }
  ctrl32.ready({ requireAuth: true, requirePerm: 'data.read' }).then(async ({ me }) => {
    ME = me; GROUP = stored('group') || 'people';
    for (const a of document.querySelectorAll('.subnav a')) if (a.dataset.section !== 'overview' && a.dataset.section !== 'cameras' && !can('config.write') && a.dataset.section !== 'areas') a.hidden = true;
    wireOverview(); wireAreas(); wireInputs();
    await refresh();
    setInterval(() => { if (!document.hidden && ctrl32.section('overview', ['overview', 'areas', 'cameras', 'sensors']) === 'overview' && !DATE) loadSummary(); }, 30000);
  });
})();
