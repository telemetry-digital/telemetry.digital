// Settings → Apps (spec 021): installable app profiles — name, home-screen name, address, start page, menu sections,
// colour. Presets only fill the form. Every change needs a reason (audit trail, new version).
let APP_OPTS = null, APPS = [];
async function settingsApps() {
  const $ = (s) => document.querySelector(s), esc = ctrl32.esc, api = ctrl32.api, t = ctrl32.t;
  const f = $('#appForm');
  const secLabel = (k) => { const s = APP_OPTS && APP_OPTS.sections.find((x) => x.key === k); return s ? t(s.label) : k; };
  const pageLabel = (u) => { const p = APP_OPTS && APP_OPTS.start_pages.find((x) => x.url === u); return p ? (p.group === 'Pages' ? t(p.label) : t(p.group) + ': ' + p.label) : u; };
  async function load() {
    $('#appListErr').textContent = '';
    try {
      [APP_OPTS, APPS] = await Promise.all([api('GET', '/api/v1/pwa-apps/options'), api('GET', '/api/v1/pwa-apps')]);
    } catch (x) { $('#appListErr').textContent = x.message; return; }
    $('#appTable tbody').innerHTML = APPS.map((a) => `<tr>
      <td><b class="notranslate">${esc(a.name)}</b><div class="muted notranslate" style="font-size:12px">${esc(a.short_name)} · v${a.version}</div></td>
      <td><a class="notranslate" href="${esc(a.install_url)}" target="_blank" rel="noopener">${esc(a.install_url)}</a></td>
      <td class="notranslate">${esc(pageLabel(a.start_url))}</td>
      <td>${a.sections.map((k) => esc(secLabel(k))).join(', ')}${a.locked ? ` <span class="badge" title="${esc(t('No way into the full application'))}">${esc(t('locked'))}</span>` : ''}${a.page_zoom === false ? ` <span class="badge">${esc(t('no page zoom'))}</span>` : ''}</td>
      <td>${a.enabled ? '<span class="badge ok">' + esc(t('enabled')) + '</span>' : '<span class="badge">' + esc(t('disabled')) + '</span>'}</td>
      <td style="white-space:nowrap"><a class="btn small" href="${esc(a.install_url)}" target="_blank" rel="noopener">${esc(t('Install page'))}</a>
        <button class="btn small" type="button" data-app-edit="${esc(a.slug)}">${esc(t('Edit'))}</button>
        <button class="btn small danger" type="button" data-app-del="${esc(a.slug)}">${esc(t('Delete'))}</button></td></tr>`).join('') ||
      `<tr><td colspan="6" class="muted">${esc(t('No apps yet. Create one, e.g. with the Cameras preset.'))}</td></tr>`;
    for (const b of document.querySelectorAll('[data-app-edit]')) b.onclick = () => open(APPS.find((a) => a.slug === b.dataset.appEdit));
    for (const b of document.querySelectorAll('[data-app-del]')) b.onclick = async () => {
      const reason = ctrl32.ask('Reason for deleting this app:');
      if (!reason) return;
      try { await api('DELETE', '/api/v1/pwa-apps/' + encodeURIComponent(b.dataset.appDel) + '?reason=' + encodeURIComponent(reason)); ctrl32.toast('app deleted', 'ok'); load(); } catch (x) { ctrl32.toast(x.message, 'bad'); }
    };
  }
  function fillStart(url) {
    const groups = {};
    for (const p of APP_OPTS.start_pages) (groups[p.group] = groups[p.group] || []).push(p);
    const known = APP_OPTS.start_pages.some((p) => p.url === url);
    f.start.innerHTML = Object.entries(groups).map(([g, ps]) => `<optgroup label="${esc(t(g))}">${ps.map((p) => `<option value="${esc(p.url)}">${esc(g === 'Pages' ? t(p.label) : p.label)}</option>`).join('')}</optgroup>`).join('') +
      `<option value="custom">${esc(t('Another page (path)…'))}</option>`;
    f.start.value = known ? url : 'custom';
    f.custom.value = known ? '' : (url || '');
    $('#appCustomRow').hidden = f.start.value !== 'custom';
  }
  f.start.onchange = () => { $('#appCustomRow').hidden = f.start.value !== 'custom'; };
  function fillSections(on) {
    $('#appSections').innerHTML = APP_OPTS.sections.map((s) => `<label class="inline"><input type="checkbox" name="sec" value="${esc(s.key)}"${on.includes(s.key) ? ' checked' : ''}><span>${esc(t(s.label))}</span></label>`).join('');
  }
  const slugOf = (s) => s.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
  const PRESETS = {
    // a phone app for the cameras: only the cameras, no page zoom, one camera per row
    cameras: () => ({ name: t('Cameras'), short_name: t('Cameras'), slug: 'cameras', start_url: '/video', sections: ['cameras'], locked: true, page_zoom: false, camera_layout: 'single' }),
    wall: () => { const w = APP_OPTS.start_pages.find((p) => p.group === 'Video walls'); return { name: w ? w.label : t('Video wall'), short_name: t('Video wall'), slug: w ? slugOf(w.label) || 'wall' : 'wall', start_url: w ? w.url : '/video/walls', sections: ['cameras'] }; },
    dashboards: () => ({ name: t('Dashboards'), short_name: t('Dashboards'), slug: 'dashboards', start_url: '/dashboards', sections: ['dashboards'] }),
    smarthome: () => ({ name: t('Smart home'), short_name: t('Home'), slug: 'home', start_url: '/home', sections: ['home', 'energy'] }),
  };
  function put(a) {
    f.name.value = a.name || ''; f.short_name.value = (a.short_name || '').slice(0, 24); f.slug.value = a.slug || '';
    fillStart(a.start_url || '/video'); fillSections(a.sections || []);
    f.locked.checked = !!a.locked; f.page_zoom.checked = a.page_zoom !== false; f.camera_layout.value = a.camera_layout || 'auto';
    f.own_colour.checked = !!a.theme_color; f.theme_color.value = a.theme_color || (ctrl32.brand.theme && ctrl32.brand.theme.primary) || '#e30613';
  }
  function open(a) {
    f.reset(); $('#appErr').textContent = '';
    f.dataset.slug = a ? a.slug : '';
    f.slug.readOnly = !!a;
    $('#appPresets').hidden = !!a;
    $('#appTitle').textContent = t(a ? 'Edit app' : 'New app');
    $('#appSubmit').textContent = t(a ? 'Save as new version' : 'Create app');
    put(a || PRESETS.cameras());
    f.enabled.checked = a ? a.enabled : true;
    $('#appDlg').showModal();
  }
  for (const b of document.querySelectorAll('[data-preset]')) b.onclick = () => { put(PRESETS[b.dataset.preset]()); if (b.dataset.preset === 'wall') f.start.focus(); };
  f.name.oninput = () => { if (!f.dataset.slug && !f.slug.dataset.touched) f.slug.value = slugOf(f.name.value); };
  f.slug.oninput = () => { f.slug.dataset.touched = '1'; };
  $('#newAppBtn').onclick = () => { delete f.slug.dataset.touched; open(null); };
  f.onsubmit = async (e) => {
    e.preventDefault();
    $('#appErr').textContent = '';
    const body = { name: f.name.value, short_name: f.short_name.value, start_url: f.start.value === 'custom' ? f.custom.value.trim() : f.start.value,
      sections: [...f.querySelectorAll('input[name=sec]:checked')].map((c) => c.value), theme_color: f.own_colour.checked ? f.theme_color.value : '',
      locked: f.locked.checked, page_zoom: f.page_zoom.checked, camera_layout: f.camera_layout.value,
      enabled: f.enabled.checked, reason: f.reason.value };
    try {
      let r;
      if (f.dataset.slug) r = await api('PUT', '/api/v1/pwa-apps/' + encodeURIComponent(f.dataset.slug), body);
      else r = await api('POST', '/api/v1/pwa-apps', Object.assign({ slug: f.slug.value.trim() }, body));
      $('#appDlg').close();
      ctrl32.toast((r.warnings && r.warnings.length) ? r.warnings[0] : 'app saved', (r.warnings && r.warnings.length) ? '' : 'ok');
      load();
    } catch (x) { $('#appErr').textContent = x.message; }
  };
  await load();
}
