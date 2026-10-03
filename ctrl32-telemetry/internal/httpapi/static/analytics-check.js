// ctrl32 telemetry — self-check of the object counting detector (0.72.0): is ffmpeg (LGPL build), ONNX Runtime and a
// YOLOX model installed on this server? Shown on Object counting → Cameras and on System → Server. The server runs
// the check in a separate process (`analytics-worker -selfcheck`) and caches it for 5 minutes; "Check again" refreshes.
(() => {
  const esc = (s) => ctrl32.esc(s), t = (s) => ctrl32.t(s);
  const NAME = { ffmpeg: 'ffmpeg', onnxruntime: 'ONNX Runtime', model_nano: 'Detector model nano', model_tiny: 'Detector model tiny', worker: 'Self-check' };
  const STATE = { ok: ['found', 'ok'], warning: ['warning', 'warn'], error: ['error', 'bad'], missing: ['not found', 'bad'], not_configured: ['not configured', ''] };
  const HINT = {
    install: 'Install it with the installer: install.sh --analytics (Linux) or install.ps1 -Analytics (Windows), or set [analytics] in config.toml.',
    gpl_build: 'This ffmpeg is a GPL build. Object counting uses an unmodified LGPL build: run the installer with --analytics (Linux) or -Analytics (Windows).',
    sha_mismatch: 'The model file is not the pinned one (a different or damaged file). Run the installer with --analytics or -Analytics again.',
    platform: 'This server cannot run the detector: a 64-bit Linux, Windows or macOS server is needed.',
    ort_version: 'ONNX Runtime is too old: version 1.17 or newer is needed.',
    model_broken: 'The model does not load or run (a YOLOX model in ONNX format is needed).',
  };

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
      <div class="actions" style="margin-top:8px"><button class="btn small" type="button" data-ac-refresh>Check again</button><span class="grow"></span>${when}</div>`;
  }

  // render(el): fill el (a card) with the self-check; resolves to the result (null when it could not be read)
  async function render(el, refresh) {
    if (!el) return null;
    if (!el.dataset.acInit) { el.innerHTML = `<p class="muted">${esc(t('Checking…'))}</p>`; el.dataset.acInit = '1'; }
    let r = null;
    try { r = await ctrl32.api('GET', '/api/v1/counting/selfcheck' + (refresh ? '?refresh=1' : '')); }
    catch (e) { el.innerHTML = `<p class="muted">${esc(e.message)}</p>`; return null; }
    el.innerHTML = body(r);
    const b = el.querySelector('[data-ac-refresh]');
    if (b) b.onclick = async () => { b.disabled = true; b.textContent = t('Checking…'); await render(el, true); };
    return r;
  }
  window.ctrl32AnalyticsCheck = { render };
})();
