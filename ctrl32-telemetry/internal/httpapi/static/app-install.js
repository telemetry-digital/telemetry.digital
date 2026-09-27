// Install and launch page of an app profile (/app/{slug}, spec 021). Launched from the home screen (display-mode
// standalone, or navigator.standalone on iPhone) or with ?launch=1 ("Open"), it enters the app mode for this window
// and opens the app's start page at once. In a browser tab it explains how to install the app and shows a QR code.
(function () {
  const $ = (s) => document.querySelector(s);
  const card = $('#appCard');
  const slug = card.dataset.slug, start = card.dataset.start;
  const standalone = ['standalone', 'fullscreen', 'minimal-ui'].some((m) => window.matchMedia && window.matchMedia('(display-mode: ' + m + ')').matches) || navigator.standalone === true;
  if (standalone || new URLSearchParams(location.search).has('launch')) {
    $('#launching').hidden = false;
    $('#installBox').hidden = true;
    ctrl32.enterApp(slug, start);
    return;
  }
  const ios = /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  $('#ios').hidden = !ios;
  $('#other').hidden = ios;
  const btn = $('#installBtn');
  if (window.deferredPrompt) btn.hidden = false;
  btn.onclick = async () => {
    if (!window.deferredPrompt) return;
    window.deferredPrompt.prompt();
    const choice = await window.deferredPrompt.userChoice.catch(() => null);
    window.deferredPrompt = null;
    btn.hidden = true;
    if (choice && choice.outcome === 'accepted') $('#installed').hidden = false;
  };
  window.addEventListener('appinstalled', () => { btn.hidden = true; $('#installed').hidden = false; });
  const url = location.origin + '/app/' + slug;
  $('#appUrl').textContent = url;
  const s = document.createElement('script');
  s.src = '/vendor/qrcode.js';
  s.onload = () => {
    window.qrcode.stringToBytes = window.qrcode.stringToBytesFuncs['UTF-8'];
    const q = window.qrcode(0, 'M');
    q.addData(url);
    q.make();
    const n = q.getModuleCount();
    let d = '';
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (q.isDark(r, c)) d += `M${c + 2},${r + 2}h1v1h-1z`;
    $('#qr').innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${n + 4} ${n + 4}" shape-rendering="crispEdges" role="img" aria-label="QR code"><rect width="${n + 4}" height="${n + 4}" fill="#fff"/><path d="${d}" fill="#000"/></svg>`;
  };
  document.head.appendChild(s);
  ctrl32.ready();
})();
