// ctrl32 telemetry — self-check of the object counting detector (0.72.0): is ffmpeg (LGPL build), ONNX Runtime and a
// YOLOX model installed on this server? Shown on Object counting → Cameras and on System → Server. The server runs
// the check in a separate process (`analytics-worker -selfcheck`) and caches it for 5 minutes; "Check again" refreshes.
// 0.75.0: a server administrator installs, repairs or removes the detector here (the server agent downloads the pinned
// parts; reason, password and authenticator code again; the progress is polled).
(() => {
  const esc = (s) => ctrl32.esc(s), t = (s) => ctrl32.t(s);
  const NAME = { ffmpeg: 'ffmpeg', onnxruntime: 'ONNX Runtime', model_nano: 'Detector model nano', model_tiny: 'Detector model tiny', worker: 'Self-check' };
  const STATE = { ok: ['found', 'ok'], warning: ['warning', 'warn'], error: ['error', 'bad'], missing: ['not found', 'bad'], not_configured: ['not configured', ''] };
  const HINT = {
    install: 'Install it with the button Install the detector (server administrators), with the installer: install.sh --analytics (Linux) or install.ps1 -Analytics (Windows), or set [analytics] in config.toml.',
    gpl_build: 'This ffmpeg is a GPL build. Object counting uses an unmodified LGPL build: install the detector with the button or the installer (--analytics, -Analytics).',
    sha_mismatch: 'The model file is not the pinned one (a different or damaged file). Repair the detector with the button or run the installer with --analytics or -Analytics again.',
    platform: 'This server cannot run the detector: a 64-bit Linux, Windows or macOS server is needed.',
    ort_version: 'ONNX Runtime is too old: version 1.17 or newer is needed.',
    model_broken: 'The model does not load or run (a YOLOX model in ONNX format is needed).',
  };
  const STEP = { prepare: 'Preparing…', unpack: 'Unpacking', verify: 'Checking', swap: 'Putting the parts in place…', done: 'Done' };
  const mb = (n) => (n / 1e6).toFixed(1);

  function body(r) {
    const comps = r.components || [];
    // paths only reach server administrators; shown where they help (a part that is missing or broken), otherwise on hover
    const rows = comps.map((c) => {
      const s = STATE[c.state] || [c.state, ''];
      let ver = esc(c.version || '');
      if (c.licence) ver += ` <span class="badge ${c.licence === 'LGPL' ? '' : 'warn'}">${esc(c.licence)}</span>`;
      if (c.ms) ver += ` <span class="muted small">${esc(t('test inference'))} ${c.ms} ms</span>`;
      const hint = c.hint && HINT[c.hint] ? `<div class="small">${esc(t(HINT[c.hint]))}</div>` : '';
      const detail = c.detail ? `<div class="muted small notranslate">${esc(c.detail)}</div>` : '';
      const path = c.path && c.state !== 'ok' ? `<div class="muted small notranslate" style="word-break:break-all">${esc(c.path)}</div>` : '';
      return `<tr><td${c.path ? ` title="${esc(c.path)}"` : ''}>${esc(t(NAME[c.name] || c.name))}</td><td><span class="badge ${s[1]}">${esc(t(s[0]))}</span>${hint}${detail}${path}</td><td>${ver}</td></tr>`;
    }).join('');
    const head = `<tr><th>${esc(t('Part'))}</th><th>${esc(t('State'))}</th><th>${esc(t('Version'))}</th></tr>`;
    const sum = r.ready
      ? `<span class="badge ok">${esc(t('ready'))}</span> <span>${esc(t('Cameras can count on this server.'))}</span>`
      : `<span class="badge warn">${esc(t('not installed'))}</span> <span>${esc(t('Counting with cameras needs the detector; counting with sensors works without it.'))}</span>`;
    const when = r.checked && !String(r.checked).startsWith('0001') ? `<span class="muted small">${esc(t('Checked'))} ${esc(ctrl32.fmt(r.checked))}${r.platform ? ' · ' + esc(r.platform) : ''}</span>` : '';
    return `<p style="margin-top:0">${sum}</p><div class="table-wrap"><table class="ac-table"><thead>${head}</thead><tbody>${rows}</tbody></table></div>
      <div data-ac-install></div>
      <div class="actions" style="margin-top:8px"><button class="btn small" type="button" data-ac-refresh>${esc(t('Check again'))}</button><span data-ac-buttons></span><span class="grow"></span>${when}</div>`;
  }

  // ---- installation by the server agent (system.admin) ----------------------------------------------------------
  function progressHTML(job) {
    const p = job.progress || {};
    let what;
    if (p.step === 'download') {
      what = `${esc(t('Downloading'))} ${p.item} ${esc(t('of'))} ${p.items}: <span class="notranslate">${esc(p.label || '')}</span>` +
        (p.total ? ` <span class="muted small">(${mb(p.bytes || 0)} ${esc(t('of'))} ${mb(p.total)} MB)</span>` : p.bytes ? ` <span class="muted small">(${mb(p.bytes)} MB)</span>` : '');
    } else if (p.step === 'unpack' || p.step === 'verify') {
      what = `${esc(t(STEP[p.step]))} <span class="notranslate">${esc(p.label || '')}</span>…`;
    } else what = esc(t(STEP[p.step] || 'Preparing…'));
    const frac = p.step === 'download' && p.items ? ((p.item - 1) + (p.total ? (p.bytes || 0) / p.total : 0)) / p.items : p.step === 'swap' || p.step === 'done' ? 1 : null;
    const bar = frac === null ? '<progress></progress>' : `<progress max="100" value="${Math.round(frac * 100)}"></progress>`;
    const title = job.action === 'remove' ? 'Removing the detector…' : 'Installing the detector…';
    return `<div class="ac-progress" role="status"><b>${esc(t(title))}</b> ${bar}<div class="small">${what}</div></div>`;
  }

  function installHTML(inst, ready) {
    if (!inst) return '';
    const ag = inst.agent || {};
    if (!ag.configured) return `<p class="muted small">${esc(t('The server agent is not running on this server, so the detector cannot be installed from here. Use the installer: install.sh --analytics (Linux) or install.ps1 -Analytics (Windows).'))}</p>`;
    if (!ag.connected) return `<p class="muted small">${esc(t('The server agent does not answer'))}: <span class="notranslate">${esc(ag.error || '')}</span></p>`;
    if (!inst.supported) return `<p class="muted small">${esc(t('The detector cannot be installed from here on this platform (Linux amd64 or arm64, Windows amd64 are supported).'))} <span class="notranslate">${esc(inst.platform || '')}</span></p>`;
    const job = inst.job;
    if (job && job.state === 'running') return progressHTML(job);
    let out = '';
    if (job && job.state === 'failed') out += `<div class="err small" style="margin:8px 0">${esc(t(job.action === 'remove' ? 'The removal failed' : 'The installation failed'))}: <span class="notranslate">${esc(job.error || '')}</span>. ${esc(t('Nothing installed was changed.'))}</div>`;
    else if (job && job.state === 'done' && job.action === 'install') out += `<p class="small" style="margin:8px 0"><span class="badge ok">${esc(t('installed'))}</span> ${esc(t('Downloaded'))}: ${job.installed && job.installed.length ? job.installed.length : 0}, ${esc(t('already installed'))}: ${job.kept && job.kept.length ? job.kept.length : 0}${inst.simulated ? ' · ' + esc(t('simulation')) : ''}</p>`;
    return out;
  }

  function buttonsHTML(inst, ready) {
    if (!inst || !inst.agent || !inst.agent.connected || !inst.supported || (inst.job && inst.job.state === 'running')) return '';
    const installed = ready || !!inst.installed;
    return ` <button class="btn small ${installed ? '' : 'primary'}" type="button" data-ac-install>${esc(t(installed ? 'Repair or update' : 'Install the detector'))}</button>` +
      (installed ? ` <button class="btn small ghost" type="button" data-ac-remove>${esc(t('Remove'))}</button>` : '');
  }

  let dlg = null;
  function dialog() {
    if (dlg) return dlg;
    dlg = document.createElement('dialog');
    dlg.id = 'acDlg';
    dlg.innerHTML = `<form class="form" id="acForm" method="dialog">
      <h2 style="margin-top:0" id="acTitle"></h2>
      <p class="muted small" id="acWhat"></p>
      <label class="inline" id="acForceRow"><input type="checkbox" name="force"> <span>${esc(t('Download every part again (repair)'))}</span></label>
      <label>${esc(t('Reason (audit trail)'))} <input name="reason" required maxlength="200"></label>
      <p class="muted small">${esc(t('Installing or removing programs on the server needs your password again.'))}</p>
      <div class="two">
        <label>${esc(t('Your password'))} <input name="password" type="password" required autocomplete="current-password"></label>
        <label class="mfa-row">${esc(t('Authenticator code'))} <input name="code" inputmode="numeric" autocomplete="one-time-code" maxlength="10"></label>
      </div>
      <div class="err" id="acErr"></div>
      <div class="actions"><button class="btn primary" type="submit" id="acSubmit"></button><button class="btn" type="button" data-ac-close>${esc(t('Close'))}</button></div>
    </form>`;
    document.body.appendChild(dlg);
    dlg.querySelector('[data-ac-close]').onclick = () => dlg.close();
    return dlg;
  }

  function open(el, inst, action) {
    const d = dialog(), f = d.querySelector('form'), pins = inst.pins || {};
    f.reset();
    d.querySelector('#acErr').textContent = '';
    for (const r of d.querySelectorAll('.mfa-row')) r.hidden = !(ctrl32.me && ctrl32.me.mfa_enabled);
    d.querySelector('#acForceRow').hidden = action !== 'install' || !inst.installed;
    d.querySelector('#acTitle').textContent = t(action === 'install' ? 'Install the detector' : 'Remove the detector');
    d.querySelector('#acWhat').textContent = action === 'install'
      ? t('The server agent downloads ONNX Runtime, ffmpeg (an unmodified LGPL build) and the YOLOX nano and tiny models from their official releases over HTTPS, checks every file against the SHA-256 pinned in this version and only then puts them in place; a failure changes nothing installed. About 90–180 MB are downloaded. Counting with cameras pauses meanwhile.') +
        ` (ONNX Runtime ${pins.onnxruntime || ''}, ${String(pins.ffmpeg || '').replace(/^ffmpeg-/, 'ffmpeg ')}, YOLOX ${pins.yolox || ''})`
      : t('The server agent removes ffmpeg, ONNX Runtime and the models installed for object counting and their paths in config.toml. Counting with cameras stops; counting with sensors keeps working.');
    const sub = d.querySelector('#acSubmit');
    sub.textContent = t(action === 'install' ? 'Install' : 'Remove');
    sub.disabled = false;
    f.onsubmit = async (e) => {
      e.preventDefault();
      sub.disabled = true;
      const b = { reason: f.reason.value, password: f.password.value, code: f.code.value.trim() };
      if (action === 'install') b.force = f.force.checked;
      try {
        await ctrl32.api(action === 'install' ? 'POST' : 'DELETE', '/api/v1/counting/install', b);
        f.password.value = '';
        d.close();
        render(el, false);
      } catch (x) { d.querySelector('#acErr').textContent = x.message; sub.disabled = false; }
    };
    d.showModal();
  }

  const polls = new WeakMap();
  async function poll(el, jobID) {
    clearTimeout(polls.get(el));
    let inst;
    try { inst = await ctrl32.api('GET', '/api/v1/counting/install'); } catch (e) { polls.set(el, setTimeout(() => poll(el, jobID), 3000)); return; }
    const job = inst.job;
    if (job && job.id === jobID && job.state === 'running') {
      const box = el.querySelector('[data-ac-install]');
      if (box) box.innerHTML = progressHTML(job);
      polls.set(el, setTimeout(() => poll(el, jobID), 1200));
      return;
    }
    if (job && job.id === jobID) ctrl32.toast(t(job.state === 'done' ? (job.action === 'remove' ? 'detector removed' : 'detector installed') : (job.action === 'remove' ? 'the removal failed' : 'the installation failed')), job.state === 'done' ? 'ok' : 'bad');
    render(el, true); // the server applied the new paths: check the installation again
  }

  // render(el): fill el (a card) with the self-check; resolves to the result (null when it could not be read)
  async function render(el, refresh) {
    if (!el) return null;
    if (!el.dataset.acInit) { el.innerHTML = `<p class="muted">${esc(t('Checking…'))}</p>`; el.dataset.acInit = '1'; }
    const admin = ctrl32.can('system.admin');
    let r = null, inst = null;
    try {
      [r, inst] = await Promise.all([ctrl32.api('GET', '/api/v1/counting/selfcheck' + (refresh ? '?refresh=1' : '')),
        admin ? ctrl32.api('GET', '/api/v1/counting/install').catch(() => null) : null]);
    } catch (e) { el.innerHTML = `<p class="muted">${esc(e.message)}</p>`; return null; }
    el.innerHTML = body(r);
    el.querySelector('[data-ac-install]').innerHTML = installHTML(inst, r.ready);
    el.querySelector('[data-ac-buttons]').innerHTML = buttonsHTML(inst, r.ready);
    const b = el.querySelector('[data-ac-refresh]');
    if (b) b.onclick = async () => { b.disabled = true; b.textContent = t('Checking…'); await render(el, true); };
    const ib = el.querySelector('[data-ac-install]') && el.querySelector('[data-ac-buttons] [data-ac-install]');
    if (ib) ib.onclick = () => open(el, inst, 'install');
    const rb = el.querySelector('[data-ac-remove]');
    if (rb) rb.onclick = () => open(el, inst, 'remove');
    if (inst && inst.job && inst.job.state === 'running') polls.set(el, setTimeout(() => poll(el, inst.job.id), 1200));
    return r;
  }
  window.ctrl32AnalyticsCheck = { render };
})();
