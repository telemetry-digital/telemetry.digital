// ctrl32 telemetry — shared page runtime: branding/theme, navigation shell (header, left drawer on mobile),
// fixed footer, session helpers, PWA registration. The footer (product name, GitHub link, license link) is part
// of the product and is not configurable. Nothing user-specific is rendered before the session state is known.
(function () {
  const ICON = {
    shield: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a12 12 0 0 0 8.5 3a12 12 0 0 1 -8.5 15a12 12 0 0 1 -8.5 -15a12 12 0 0 0 8.5 -3"/><path d="M9 12l2 2l4 -4"/></svg>',
    display: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="12" rx="1"/><path d="M7 20h10"/><path d="M9 16v4"/><path d="M15 16v4"/></svg>',
    bolt: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 3l0 7l6 0l-8 11l0 -7l-6 0l8 -11"/></svg>',
    home: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12l-2 0l9 -9l9 9l-2 0"/><path d="M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-7"/><path d="M10 12h4v4h-4z"/></svg>',
    flow: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="6" height="5" rx="1"/><rect x="15" y="4" width="6" height="5" rx="1"/><rect x="15" y="15" width="6" height="5" rx="1"/><path d="M9 6.5h6"/><path d="M6 9v6.5a1 1 0 0 0 1 1h8"/></svg>',
    github: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 19c-4.3 1.4 -4.3 -2.5 -6 -3m12 5v-3.5c0 -1 .1 -1.4 -.5 -2c2.8 -.3 5.5 -1.4 5.5 -6a4.6 4.6 0 0 0 -1.3 -3.2a4.2 4.2 0 0 0 -.1 -3.2s-1.1 -.3 -3.5 1.3a12.3 12.3 0 0 0 -6.2 0c-2.4 -1.6 -3.5 -1.3 -3.5 -1.3a4.2 4.2 0 0 0 -.1 3.2a4.6 4.6 0 0 0 -1.3 3.2c0 4.6 2.7 5.7 5.5 6c-.6 .6 -.6 1.2 -.5 2v3.5"/></svg>',
    sun: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M3 12h1m8 -9v1m8 8h1m-9 8v1m-6.4 -15.4l.7 .7m12.1 -.7l-.7 .7m0 11.4l.7 .7m-12.1 -.7l-.7 .7"/></svg>',
    moon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3c.132 0 .263 0 .393 0a7.5 7.5 0 0 0 7.92 12.446a9 9 0 1 1 -8.313 -12.454z"/></svg>',
    auto: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 3v18"/><path d="M12 9l4.65 -4.65"/><path d="M12 14.3l7.37 -7.37"/><path d="M12 19.6l8.85 -8.85"/></svg>',
    menu: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6h16M4 12h16M4 18h16"/></svg>',
    close: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18M6 6l12 12"/></svg>',
    chevL: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6l-6 6l6 6"/></svg>',
    alert: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0z"/><path d="M12 16h.01"/></svg>',
    dashboard: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></svg>',
    users: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="9" cy="7" r="4"/><path d="M3 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/><path d="M21 21v-2a4 4 0 0 0 -3 -3.85"/></svg>',
    settings: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.325 4.317c.426 -1.756 2.924 -1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543 -.94 3.31 .826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756 .426 1.756 2.924 0 3.35a1.724 1.724 0 0 0 -1.066 2.573c.94 1.543 -.826 3.31 -2.37 2.37a1.724 1.724 0 0 0 -2.572 1.065c-.426 1.756 -2.924 1.756 -3.35 0a1.724 1.724 0 0 0 -2.573 -1.066c-1.543 .94 -3.31 -.826 -2.37 -2.37a1.724 1.724 0 0 0 -1.065 -2.572c-1.756 -.426 -1.756 -2.924 0 -3.35a1.724 1.724 0 0 0 1.066 -2.573c-.94 -1.543 .826 -3.31 2.37 -2.37c1 .608 2.296 .07 2.572 -1.065z"/><circle cx="12" cy="12" r="3"/></svg>',
    user: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="7" r="4"/><path d="M6 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2"/></svg>',
    logout: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 8v-2a2 2 0 0 0 -2 -2h-7a2 2 0 0 0 -2 2v12a2 2 0 0 0 2 2h7a2 2 0 0 0 2 -2v-2"/><path d="M9 12h12l-3 -3"/><path d="M18 15l3 -3"/></svg>',
    login: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 8v-2a2 2 0 0 1 2 -2h3a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-3a2 2 0 0 1 -2 -2v-2"/><path d="M3 12h13l-3 -3"/><path d="M13 15l3 -3"/></svg>',
    bug: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 9v-1a3 3 0 0 1 6 0v1"/><path d="M8 9h8a6 6 0 0 1 1 3v3a5 5 0 0 1 -10 0v-3a6 6 0 0 1 1 -3"/><path d="M3 13l4 0"/><path d="M17 13l4 0"/><path d="M12 20l0 -6"/><path d="M4 19l3.35 -2"/><path d="M20 19l-3.35 -2"/><path d="M4 7l3.75 2.4"/><path d="M20 7l-3.75 2.4"/></svg>',
    system: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="6" rx="2"/><rect x="3" y="14" width="18" height="6" rx="2"/><path d="M7 7h.01M7 17h.01"/></svg>',
    device: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M7 20h10"/><path d="M9 16v4"/><path d="M15 16v4"/><path d="M7 8h2"/><path d="M7 12h6"/></svg>',
    assets: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 21h18"/><path d="M5 21v-14l8 -4v18"/><path d="M19 21v-10l-6 -4"/><path d="M9 9v0"/><path d="M9 12v0"/><path d="M9 15v0"/><path d="M9 18v0"/></svg>',
    camera: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 10l4.553 -2.276a1 1 0 0 1 1.447 .894v6.764a1 1 0 0 1 -1.447 .894l-4.553 -2.276v-4z"/><rect x="3" y="6" width="12" height="12" rx="2"/></svg>',
    search: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="10" cy="10" r="7"/><path d="M21 21l-6 -6"/></svg>',
    count: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 12v.01"/><path d="M3 21h18"/><path d="M5 21v-16a2 2 0 0 1 2 -2h6m4 10.5v7.5"/><path d="M21 7h-7m3 -3l-3 3l3 3"/></svg>',
    report: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5h-2a2 2 0 0 0 -2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-12a2 2 0 0 0 -2 -2h-2"/><path d="M9 5a2 2 0 0 1 2 -2h2a2 2 0 0 1 2 2a2 2 0 0 1 -2 2h-2a2 2 0 0 1 -2 -2z"/><path d="M9 12h6"/><path d="M9 16h4"/></svg>',
    file: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21h-10a2 2 0 0 1 -2 -2v-14a2 2 0 0 1 2 -2h7l5 5v11a2 2 0 0 1 -2 2z"/></svg>',
  };
  const $ = (s) => document.querySelector(s);
  const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  let brand = { product: 'ctrl32 telemetry', version: '', org_name: '', logo_url: '/logo.svg', theme: { primary: '#e30613', font: '', mode: 'auto', radius: 'small', font_scale: 'normal' },
    footer: { text: 'ctrl32 · telemetry', github: 'https://github.com/telemetry-digital', license: '/license' } };
  let csrf = null, me = null, meLoaded = false;

  // ---- theme ------------------------------------------------------------------------------------
  function applyTheme(t) {
    const root = document.documentElement;
    root.style.setProperty('--primary', /^#[0-9a-fA-F]{6}$/.test(t.primary) ? t.primary : '#e30613');
    root.style.setProperty('--font', t.font ? '"' + t.font.replace(/["\\]/g, '') + '", system-ui, sans-serif' : 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif');
    root.style.setProperty('--radius', { none: '0', small: '3px', medium: '6px', large: '10px' }[t.radius] || '3px');
    root.style.setProperty('--fs', { small: '14px', normal: '15px', large: '17px' }[t.font_scale] || '15px');
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = t.primary || '#e30613';
  }
  function userMode() { try { return localStorage.getItem('ctrl32.theme'); } catch (e) { return null; } }
  function mode() { return userMode() || brand.theme.mode || 'auto'; }
  // built-in logo follows the theme (telemetry.digital wordmark: grey text on light, white text on dark)
  function isDark() { const m = mode(); return m === 'dark' || (m === 'auto' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches); }
  function logoURL() { return brand.logo_custom ? brand.logo_url : (isDark() ? '/logo-dark.svg' : '/logo.svg'); }
  function applyMode() {
    const m = mode(), root = document.documentElement;
    if (m === 'auto') root.removeAttribute('data-theme'); else root.setAttribute('data-theme', m);
    for (const img of document.querySelectorAll('img[data-logo]')) img.src = logoURL();
    for (const b of document.querySelectorAll('[data-theme-toggle]')) {
      b.innerHTML = (m === 'dark' ? ICON.moon : (m === 'light' ? ICON.sun : ICON.auto)) + (b.dataset.label !== undefined ? '<span>Theme: ' + m + '</span>' : '');
      b.title = 'Theme: ' + m + ' (click to change)';
      b.onclick = toggleMode;
    }
  }
  function toggleMode() {
    const next = { auto: 'light', light: 'dark', dark: 'auto' }[mode()];
    try { if (next === 'auto') localStorage.removeItem('ctrl32.theme'); else localStorage.setItem('ctrl32.theme', next); } catch (e) { /* private mode */ }
    applyMode();
  }

  // ---- session ----------------------------------------------------------------------------------
  async function loadMe() {
    try {
      const r = await fetch('/api/v1/auth/me', { credentials: 'same-origin' });
      me = r.ok ? await r.json() : null;
      csrf = me ? me.csrf_token : null;
    } catch (e) { me = null; }
    meLoaded = true;
    return me;
  }
  // can('a') or can('a|b'): any of the permissions (as the server's "a|b" routes)
  const can = (p) => !!(me && me.permissions && String(p).split('|').some((x) => me.permissions.includes(x)));
  async function api(method, url, body) {
    const h = { 'Content-Type': 'application/json' };
    if (csrf) h['X-CSRF-Token'] = csrf;
    const r = await fetch(url, { method, headers: h, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' });
    const ct = r.headers.get('Content-Type') || '';
    const data = ct.includes('json') ? await r.json().catch(() => ({})) : await r.blob();
    if (r.status === 401 && meLoaded && me) { location.href = '/login'; }
    if (!r.ok) { const e = new Error((data && data.error) || ('HTTP ' + r.status)); e.status = r.status; e.code = data && data.code; throw e; }
    return data;
  }
  async function logout() { try { await api('POST', '/api/v1/auth/logout', {}); } catch (e) { /* already gone */ } location.href = app ? '/login' : '/'; }

  // ---- installable apps (spec 021) ------------------------------------------------------------------
  // /app/{slug} (launched from the home screen, or "Open") remembers the app for this window in sessionStorage; every
  // page opened in the window then shows only the app's menu sections (plus My account) and its name, and the logo
  // leads to the app's start page. The app is a view, not a permission boundary: permissions filter the menu first.
  const APP_KEY = 'ctrl32.app';
  let app = null, pendingFull = false;
  function appStore(slug, profile) {
    try {
      if (slug) { sessionStorage.setItem(APP_KEY, slug); if (profile) sessionStorage.setItem(APP_KEY + '.profile', JSON.stringify(profile)); }
      else { sessionStorage.removeItem(APP_KEY); sessionStorage.removeItem(APP_KEY + '.profile'); }
    } catch (e) { /* storage blocked */ }
  }
  function appSlug() { try { return sessionStorage.getItem(APP_KEY) || ''; } catch (e) { return ''; } }
  // The app also rides in the address (?app=<slug>): browsers that reopen the last page in a fresh session (Firefox on
  // iPhone, a home-screen bookmark) lose sessionStorage, the address keeps the app.
  (function appFromAddress() {
    const s = new URLSearchParams(location.search).get('app');
    if (s && /^[a-z0-9][a-z0-9-]{0,39}$/.test(s) && s !== appSlug()) { appStore(''); appStore(s); }
  })();
  function markAddress() {
    if (!app) return;
    const u = new URL(location.href);
    if (u.searchParams.get('app') === app.slug) return;
    u.searchParams.set('app', app.slug);
    history.replaceState(history.state, '', u.pathname + u.search + u.hash);
  }
  // the remembered profile is known at once, so a locked app is confined before the page renders
  if (appSlug()) { try { app = JSON.parse(sessionStorage.getItem(APP_KEY + '.profile') || 'null'); } catch (e) { app = null; } if (app && app.slug !== appSlug()) app = null; }
  // Locked app (0.61.0): the window stays on the app's sections. Pages of a section (and their sub-pages), as the server's
  // pwaSectionPaths; every locked app also keeps sign-in, password reset, Password and MFA, the licence and its install
  // page, and always its start page. Anything else opens the start page instead. This confines the user interface only;
  // permissions stay the security boundary (the API answers what the role allows, whatever window asks).
  const APP_PATHS = {
    dashboards: ['/', '/dashboards'], home: ['/home', '/smarthome'], energy: ['/energy'], counting: ['/counting'], cameras: ['/video'],
    displays: ['/displays'], flows: ['/flows', '/automation'], devices: ['/devices'], incidents: ['/incidents'], assets: ['/assets'],
    firmware: ['/fota'], users: ['/users'], audit: ['/audit', '/reports/signatures'], settings: ['/settings', '/vpn'], system: ['/organizations', '/server', '/landing/edit'],
    reports: ['/reports'],
  };
  const APP_ALWAYS = ['/login', '/forgot', '/reset', '/account/security', '/license'];
  // View-only app (0.80.0): the pages that configure are left out (for watching, controlling and playing back)
  const APP_CONFIG = ['/video/manage', '/video/settings', '/counting/areas', '/counting/cameras', '/counting/sensors'];
  const pathIn = (p, list) => list.some((x) => p === x || (x !== '/' && p.startsWith(x + '/')));
  function appAllows(path) {
    if (!app) return true;
    path = path.replace(/\/+$/, '') || '/';
    if (path === (app.start_url || '/').split('?')[0] || path === '/app/' + app.slug || pathIn(path, APP_ALWAYS)) return true;
    if (app.view_only && pathIn(path, APP_CONFIG)) return false;
    if (!app.locked) return true;
    return (app.sections || []).some((s) => pathIn(path, APP_PATHS[s] || []));
  }
  // a page outside a locked app (or a settings page of a view-only one): its start page instead (true when leaving)
  function confine() {
    if (!app || appAllows(location.pathname)) return false;
    location.replace(app.start_url || '/');
    return true;
  }
  (function leaveAppByLink() { // "Open the full application" links to ?full=1; a locked app has no way out
    const q = new URLSearchParams(location.search);
    if (!q.has('full')) return;
    q.delete('full'); if (!(app && app.locked)) q.delete('app');
    history.replaceState(null, '', location.pathname + (q.toString() ? '?' + q : '') + location.hash);
    if (app && app.locked) return;
    if (appSlug() && !app) { pendingFull = true; return; } // profile not known yet: loadApp decides
    appStore(''); app = null;
  })();
  // page zoom off (page_zoom false): the viewport forbids zooming, touch-action stops pinch and double-tap zoom, and
  // for iOS Safari (which ignores user-scalable since iOS 10) multi-touch gestures outside the video player are
  // cancelled. The player keeps its own pinch zoom of the picture (videoplayer.js).
  const noPageZoom = () => !!(app && app.page_zoom === false);
  function applyAppMode() {
    const root = document.documentElement;
    root.classList.toggle('app-locked', !!(app && app.locked));
    root.classList.toggle('no-page-zoom', noPageZoom());
    if (document.body) document.body.classList.toggle('app-mode', !!app);
    const vp = document.querySelector('meta[name="viewport"]');
    if (vp) {
      if (vp.dataset.orig === undefined) vp.dataset.orig = vp.getAttribute('content') || '';
      const base = vp.dataset.orig.split(',').map((s) => s.trim()).filter((s) => s && !/^(maximum-scale|minimum-scale|user-scalable)=/.test(s));
      vp.setAttribute('content', noPageZoom() ? base.concat(['maximum-scale=1', 'user-scalable=no']).join(', ') : vp.dataset.orig);
    }
  }
  const inPlayer = (t) => !!(t && t.closest && t.closest('.vp'));
  for (const ev of ['gesturestart', 'gesturechange']) document.addEventListener(ev, (e) => { if (noPageZoom() && !inPlayer(e.target)) e.preventDefault(); }, { passive: false });
  document.addEventListener('touchmove', (e) => { if (noPageZoom() && e.touches.length > 1 && !inPlayer(e.target)) e.preventDefault(); }, { passive: false });
  // links out of a locked app (or to a settings page of a view-only one) go to its start page without loading it first
  document.addEventListener('click', (e) => {
    if (!app || e.defaultPrevented) return;
    const a = e.target.closest && e.target.closest('a[href]');
    if (!a || (a.target && a.target !== '_self')) return;
    let u; try { u = new URL(a.href, location.href); } catch (x) { return; }
    if (u.origin !== location.origin || appAllows(u.pathname)) return;
    e.preventDefault();
    location.href = app.start_url || '/';
  }, true);
  window.addEventListener('pageshow', (e) => { if (e.persisted) confine(); }); // back/forward from the page cache
  applyAppMode();
  confine();
  async function loadApp() {
    const slug = appSlug();
    if (!slug) return null;
    try {
      const r = await fetch('/api/v1/pwa-apps/' + encodeURIComponent(slug) + '/launch', { credentials: 'same-origin' });
      if (r.ok) { app = await r.json(); appStore(slug, app); } else if (r.status === 404) { app = null; appStore(''); }
    } catch (e) { /* offline: keep the remembered profile */ }
    if (pendingFull && app && !app.locked) { app = null; appStore(''); }
    pendingFull = false;
    applyAppMode();
    markAddress();
    if (confine()) return new Promise(() => {}); // leaving for the start page: nothing to render here
    return app;
  }
  // enterApp: called by the install page; remembers the app and opens its start page in this window
  function enterApp(slug, start) { appStore(''); appStore(slug); const u = new URL(start || '/', location.href); u.searchParams.set('app', slug); location.replace(u.pathname + u.search + u.hash); }
  function home() { return app ? app.start_url : '/'; }
  const locked = () => !!(app && app.locked);
  const accountHref = () => (locked() ? '/account/security' : '/account');

  // ---- navigation shell -------------------------------------------------------------------------
  // Items are filtered by the signed-in user's permissions. Nothing user-specific appears before loadMe().
  // Menu: top-level pages with sub-pages (each sub-page is its own screen, nothing is crammed on one page). Every tab of a
  // page's in-page .subnav is a sub-item here too: the desktop hides the .subnav (TestSubnavTabsInMenu checks it).
  // { head } items are the section headings (Monitoring, Devices and automation, Administration; '' is a plain divider);
  // a heading without visible items under it is left out.
  function navItems() {
    const items = menuItems();
    // sub-items the user may not open, and in a locked app the ones outside its sections (they would open its start page)
    for (const i of items) if (i.children) i.children = i.children.filter((c) => (c.show === undefined || c.show) && appAllows(c.href.split('?')[0]));
    // app mode: only the app's sections (after the permission filter) and My account
    const shown = items.filter((i) => i.head !== undefined || (i.show && (!app || !i.sec || app.sections.includes(i.sec))));
    return shown.filter((i, k) => i.head === undefined || (k > 0 && k + 1 < shown.length && shown[k + 1].head === undefined));
  }
  // deniedPages: sub-pages the menu hides for lack of a permission; their page tabs (.subnav) are hidden as well
  function deniedPages() {
    const sub = menuItems().flatMap((i) => i.children || []);
    return new Set(sub.filter((c) => c.show === false && !sub.some((o) => o.href === c.href && o.show !== false)).map((c) => c.href));
  }
  function menuItems() {
    const cw = can('content.write'), cfg = can('config.write');
    return [
      { href: '/', label: 'Overview', icon: ICON.dashboard, show: !!me, sec: 'dashboards' },
      { href: '/dashboards', label: 'Dashboards', icon: ICON.dashboard, show: can('data.read'), sec: 'dashboards' },
      { head: 'Monitoring' },
      { href: '/home', label: 'Home', icon: ICON.home, show: can('data.read'), sec: 'home' },
      { href: '/energy', label: 'Energy', icon: ICON.bolt, show: can('data.read'), sec: 'energy' },
      { href: '/counting', label: 'Object counting', icon: ICON.count, show: can('data.read'), sec: 'counting', children: [
        { href: '/counting', label: 'Overview' }, { href: '/counting/areas', label: 'Areas and entrances' },
        { href: '/counting/cameras', label: 'Cameras' }, { href: '/counting/sensors', label: 'Sensors', show: cfg },
        { href: '/reports/scheduled', label: 'Scheduled reports', show: cw && cfg }] },
      { href: '/video', label: 'Cameras', icon: ICON.camera, show: can('video.view'), sec: 'cameras', children: [
        { href: '/video', label: 'Live view' }, { href: '/video/walls', label: 'Video walls' }, { href: '/video/manage', label: 'Camera management', show: can('video.manage') },
        { href: '/video/evidence', label: 'Evidence', show: can('video.playback') }, { href: '/video/settings', label: 'Settings', show: can('video.manage') },
        { href: '/counting/cameras', label: 'Object counting', show: can('data.read') }] },
      { href: '/displays', label: 'Displays', icon: ICON.display, show: can('display.control'), sec: 'displays' },
      { href: '/incidents', label: 'Incidents', icon: ICON.alert, show: can('data.read'), sec: 'incidents' },
      // every report page in one place (0.75.0); the old addresses under Settings and Audit redirect here
      { href: cw ? '/reports/templates' : '/reports/signatures', base: '/reports', label: 'Reports', icon: ICON.report, show: cw || can('audit.read') || can('report.sign'), sec: 'reports', children: [
        { href: '/reports/templates', label: 'Report templates', show: cw }, { href: '/reports', label: 'Stored reports', show: cw },
        { href: '/reports/scheduled', label: 'Scheduled reports', show: cw && cfg }, { href: '/reports/signatures', label: 'Report signatures', show: can('audit.read') || can('report.sign') }] },
      { head: 'Devices and automation' },
      { href: '/devices', label: 'Devices', icon: ICON.device, show: can('data.read'), sec: 'devices' },
      { href: '/assets', label: 'Assets', icon: ICON.assets, show: can('data.read'), sec: 'assets', children: [
        { href: '/assets/sites', label: 'Sites' }, { href: '/assets', label: 'Assets' }] },
      // flows, alarm rules and automation rules together (0.75.0); /assets/rules and /assets/automation redirect here
      { href: '/flows', base: '/automation', label: 'Automation', icon: ICON.flow, show: can('data.read'), sec: 'flows', children: [
        { href: '/flows', label: 'Flows' }, { href: '/automation/alarm-rules', label: 'Alarm rules' },
        { href: '/automation/rules', label: 'Automation rules', show: cfg }] },
      { href: '/fota', label: 'Firmware', icon: ICON.file, show: can('fota.read'), sec: 'firmware', children: [
        { href: '/fota', label: 'Images' }, { href: '/fota/campaigns', label: 'Campaigns' }] },
      { head: 'Administration' },
      { href: '/users', label: 'Users', icon: ICON.users, show: can('user.admin'), sec: 'users', children: [
        { href: '/users', label: 'Accounts' }, { href: '/users/roles', label: 'Roles and permissions' }] },
      { href: can('audit.read') ? '/audit' : '/reports/signatures', label: 'Audit', icon: ICON.shield, show: can('audit.read') || can('report.sign'), sec: 'audit', children: [
        { href: '/audit', label: 'Audit trail', show: can('audit.read') }, { href: '/audit/reviews', label: 'Reviews', show: can('audit.read') },
        { href: '/reports/signatures', label: 'Report signatures' }] },
      // in the order of the page tabs: look and language, notification channels, connections and apps, rules of the
      // organization; an organization's VPN manager without content.write sees only VPN here
      { href: cw ? '/settings' : '/vpn', label: 'Settings', icon: ICON.settings, show: cw || (can('vpn.manage') && !can('system.admin')), sec: 'settings', children: [
        { href: '/settings/branding', label: 'Branding and theme', show: cw }, { href: '/settings/languages', label: 'Languages', show: cw },
        { href: '/settings/translations', label: 'Translations', show: cw }, { href: '/settings/notifications', label: 'Notification channels', show: cw && cfg },
        { href: '/settings/connectors', label: 'Connectors', show: cw && cfg },
        { href: '/settings/apps', label: 'Apps', show: cw }, { href: '/settings/ai', label: 'AI assistants', show: cw },
        { href: '/settings/approvals', label: 'Approvals', show: cw }, { href: '/settings/policy', label: 'Security policy', show: cw && cfg },
        { href: '/vpn', label: 'VPN', show: can('vpn.manage') && !can('system.admin') }, { href: '/settings/export', label: 'Organization export', show: cw && can('user.admin') }] },
      { href: '/organizations', label: 'System', icon: ICON.system, show: can('system.admin'), sec: 'system', children: [
        { href: '/organizations', label: 'Organizations' }, { href: '/server', label: 'Server' }, { href: '/server/domain', label: 'Domain and TLS' },
        { href: '/server/vpn', label: 'VPN' }, { href: '/server/backups', label: 'Backups' }, { href: '/server/redundancy', label: 'Redundant database' }, { href: '/server/updates', label: 'Updates' },
        { href: '/server/email', label: 'E-mail (SMTP)' },
        { href: '/server/license', label: 'License' }, { href: '/landing/edit', label: 'Front page' }] },
      { head: '' },
      // My account (foot: true) sits once, at the bottom of the sidebar next to Sign out, and at the end of the phone
      // drawer; a locked app keeps only Password and MFA
      { href: accountHref(), label: 'My account', icon: ICON.user, show: !!me, foot: true, children: locked() ? [{ href: '/account/security', label: 'Password and MFA' }] : [
        { href: '/account', label: 'Profile' }, { href: '/account/security', label: 'Password and MFA' }, { href: '/account/tokens', label: 'API tokens' }, { href: '/account/push', label: 'Browser notifications' },
        { href: '/account/ai', label: 'AI assistants' }, { href: '/api/docs', label: 'API documentation', show: !app },
        { href: '/?full=1', label: 'Open the full application', show: !!app }] },
    ];
  }
  // sidebar state: open (default) -> icons only -> hidden (top bar with hamburger); remembered per browser
  function sidebarState() { try { return localStorage.getItem('ctrl32.sidebar') || 'open'; } catch (e) { return 'open'; } }
  function setSidebar(state) { try { localStorage.setItem('ctrl32.sidebar', state); } catch (e) { /* private mode */ } applySidebar(); }
  function applySidebar() {
    const s = sidebarState();
    document.body.classList.toggle('sb-icons', s === 'icons');
    document.body.classList.toggle('sb-hidden', s === 'hidden');
    const t = $('#sideToggle');
    if (t) t.title = s === 'open' ? 'Collapse menu to icons' : 'Hide menu';
    const mb = $('#menuBtn');
    if (mb) mb.classList.toggle('sb-restore', s === 'hidden');
  }
  function isActive(href) { const p = location.pathname; return href === '/' ? p === '/' : p.startsWith(href); }
  function isExact(href) { const p = location.pathname.replace(/\/$/, '') || '/'; return p === href; }
  // section(): which sub-page of the current page is shown, from the last path segment; shows the matching
  // [data-section] panel and marks the in-page sub-navigation. Pages call ctrl32.section('branding', ['branding', ...]).
  // alias maps a last segment to a section where the address differs (/automation/rules → automation of assets.html)
  function section(defaultName, names, alias) {
    let seg = location.pathname.replace(/\/$/, '').split('/').pop();
    if (alias && Object.prototype.hasOwnProperty.call(alias, seg)) seg = alias[seg];
    const name = names.includes(seg) ? seg : defaultName;
    for (const el of document.querySelectorAll('[data-section]')) el.classList.toggle('active', el.dataset.section === name);
    for (const a of document.querySelectorAll('.subnav a')) a.classList.toggle('active', a.dataset.section === name);
    return name;
  }
  // Signed-in layout: left sidebar with the menu on desktop (collapses to icons on narrow desktops), top bar with
  // a hamburger + drawer on mobile. Anonymous pages keep a plain top bar with the logo only.
  // Bug report: opens the mail client with the context filled in (page, version, organization, user, browser).
  function bugMailto() {
    const body = ['Describe the problem (what you did, what you expected, what happened):', '', '', '--- context ---', 'Page: ' + location.href, 'Version: ' + (brand.version || ''), 'Organization: ' + (brand.org_name || ''), 'User: ' + (me ? me.username : 'anonymous'), 'Browser: ' + navigator.userAgent, 'Time: ' + new Date().toISOString()].join('\n');
    return 'mailto:info@telemetry.digital?subject=' + encodeURIComponent('[ctrl32 telemetry ' + (brand.version || '') + '] bug report') + '&body=' + encodeURIComponent(body);
  }
  function bugButton(cls) { return `<a class="btn ghost icon ${cls}" id="bugBtn" href="#" title="Report a bug (opens your mail client, info@telemetry.digital)" aria-label="Report a bug">${ICON.bug}</a>`; }
  function wireBug() { const b = $('#bugBtn'); if (b) b.onclick = (e) => { e.preventDefault(); location.href = bugMailto(); }; }
  function renderShell() {
    const header = $('header.topbar');
    if (!header) return;
    const items = navItems();
    const chev = '<svg class="chev" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6l-6 6"/></svg>';
    // a group is open (and marked) on its own pages and on the pages of its sub-items, e.g. System on /server; a page that
    // is also a sub-item of another group (Object counting under Cameras) opens only the group it belongs to
    // (base: the address prefix of a group whose pages live under it, e.g. Reports under /reports)
    const inGroup = (i) => isActive(i.base || i.href) || i.children.some((c) => isExact(c.href) && !items.some((o) => o !== i && o.children && isActive(o.base || o.href) && o.children.some((x) => x.href === c.href)));
    const opened = new Set(items.filter((i) => i.children && inGroup(i)));
    const on = (i, sub) => (sub ? isExact(i.href) : i.children ? opened.has(i) : isActive(i.href));
    const link = (i, sub) => `<a class="nav-link${on(i, sub) ? ' active' : ''}" href="${i.href}" title="${esc(i.label)}">${i.icon || ''}<span>${esc(i.label)}</span>${!sub && i.children ? chev : ''}</a>`;
    const head = (i) => `<div class="nav-head${i.head ? '' : ' sep'}" role="presentation">${i.head ? `<span>${esc(i.head)}</span>` : ''}</div>`;
    const group = (i) => `<div class="group${opened.has(i) ? ' open' : ''}">${link(i)}<div class="sub">${i.children.map((c) => link(c, true)).join('')}</div></div>`;
    // the sidebar keeps My account (foot) in its footer, so it is listed once; a heading left without items goes
    const side = items.filter((i) => !i.foot).filter((i, k, a) => i.head === undefined || (k + 1 < a.length && a[k + 1].head === undefined));
    const links = side.map((i) => i.head !== undefined ? head(i) : i.children ? group(i) : link(i)).join('');
    const foot = items.filter((i) => i.foot).map((i) => i.children && i.children.length ? group(i) : link(i)).join('');
    // phone drawer: the same headings; a group's sub-items show under it when it is open (tap the group to open it)
    const drawerLinks = items.map((i, k) => i.head !== undefined ? head(i)
      : `<a class="nav-link${on(i) ? ' active' : ''}" href="${i.href}"${i.children ? ` data-grp="${k}" aria-expanded="${opened.has(i)}"` : ''}>${i.icon}<span>${esc(i.label)}</span>${i.children ? chev : ''}</a>`
        + (i.children ? i.children.map((c) => `<a class="nav-link sub-link${isExact(c.href) ? ' active' : ''}" href="${c.href}" data-in="${k}"${opened.has(i) ? '' : ' hidden'}><span>${esc(c.label)}</span></a>`).join('') : '')).join('');
    header.innerHTML = `<div class="container row">
      ${me ? `<button class="btn ghost icon only-mobile" id="menuBtn" type="button" aria-label="Menu" aria-expanded="false">${ICON.menu}</button>` : ''}
      <a class="brand" href="${esc(home())}"><img id="logo" data-logo alt="telemetry.digital" src="${esc(logoURL())}">${me || app ? `<span id="orgname" class="${app ? 'notranslate' : ''}">${esc(app ? app.name : (brand.org_name || brand.product))}</span>` : ''}</a>
      <span class="grow"></span>
      ${me ? '' : (brand.public_read ? '<span class="badge warn" title="http.public_read is enabled: anyone can read data without signing in. Demo instances only.">Read-only demo · sign in to change anything</span>' : '')}
      ${me && !app ? `<button class="btn ghost icon only-mobile" id="gsBtnM" type="button" title="Search (Ctrl+K)" aria-label="Search">${ICON.search}</button>` : ''}
      ${app ? '' : bugButton(me ? 'only-mobile' : '')}
      ${me ? '' : langSwitcher('langTop')}
    </div>`;
    // a locked app shows no in-page links out of it either (sub-navigation of a page, e.g. My account)
    if (app) for (const a of document.querySelectorAll('.subnav a[href]')) if (!appAllows(new URL(a.href, location.href).pathname)) a.hidden = true;
    // page tabs follow the menu: a tab whose sub-page the menu hides for this user is hidden too
    if (me) { const denied = deniedPages(); for (const a of document.querySelectorAll('.subnav:not(#tabs) a[href]')) if (denied.has(a.getAttribute('href'))) a.hidden = true; }
    // Desktop: the bug button lives in its own strip at the top of the page (in flow, sticky), never over page buttons.
    if (me && !app && !$('#bugBtnDesk')) { const host = $('main'); const strip = document.createElement('div'); strip.className = 'top-strip only-desktop'; const b = document.createElement('a'); b.id = 'bugBtnDesk'; b.className = 'btn ghost icon bug-btn'; b.href = '#'; b.title = 'Report a bug (opens your mail client, info@telemetry.digital)'; b.setAttribute('aria-label', 'Report a bug'); b.innerHTML = ICON.bug; b.onclick = (e) => { e.preventDefault(); location.href = bugMailto(); }; const gs = document.createElement('button'); gs.type = 'button'; gs.className = 'gs-field'; gs.id = 'gsBtn'; gs.title = 'Search (Ctrl+K)'; gs.innerHTML = ICON.search + '<span>Search…</span><kbd>Ctrl K</kbd>'; strip.appendChild(gs); const sp = document.createElement('span'); sp.style.flex = '1'; strip.appendChild(sp); strip.appendChild(b); if (host) { host.prepend(strip); document.body.classList.add('has-strip'); } else document.body.appendChild(strip); }
    if (me && !app) { for (const id of ['gsBtn', 'gsBtnM']) { const el = $('#' + id); if (el) el.onclick = () => openSearch(); } }
    wireBug();
    if (me) {
      document.body.classList.add('has-sidebar');
      let layout = $('#appLayout');
      if (!layout) {
        layout = document.createElement('div');
        layout.id = 'appLayout';
        const side = document.createElement('aside');
        side.className = 'sidebar';
        side.id = 'sidebar';
        const content = document.createElement('div');
        content.className = 'content';
        header.after(layout);
        layout.appendChild(side);
        layout.appendChild(content);
        const main = $('main');
        if (main) content.appendChild(main);
        const f = $('.footer');
        if (f) content.appendChild(f);
      }
      $('#sidebar').innerHTML = `<div class="side-top"><a class="brand side-brand" href="${esc(home())}" title="${esc(app ? app.name : (brand.org_name || brand.product))}"><img data-logo alt="" src="${esc(logoURL())}"><img class="mark" alt="" src="/favicon.svg"></a></div>
        <div class="side-org${app ? ' notranslate' : ''}" title="${esc(app ? app.name : (brand.org_name || brand.product))}"><span>${esc(app ? app.name : (brand.org_name || brand.product))}</span></div>
        <nav class="side-nav" aria-label="Main">${links}</nav>
        <div class="side-foot">
          <button class="btn ghost side-toggle" id="sideToggle" type="button" title="Collapse menu (icons, hidden, open)">${ICON.chevL}<span>Collapse menu</span></button>
          <div class="side-user" title="${esc((me.roles || []).join(', '))}">${ICON.user}<div><b>${esc(me.username)}</b><br><span class="muted">${esc((me.roles || []).join(', ') || 'no roles')}</span></div></div>
          <button class="btn ghost" data-theme-toggle data-label type="button"></button>
          ${langSwitcher('langSide')}
          ${foot ? `<nav class="side-nav side-acct" aria-label="My account">${foot}</nav>` : ''}
          <button class="btn ghost" id="signout" type="button" title="Sign out">${ICON.logout}<span>Sign out</span></button>
        </div>`;
      $('#signout').onclick = logout;
      applySidebar();
      $('#sideToggle').onclick = () => setSidebar({ open: 'icons', icons: 'hidden', hidden: 'open' }[sidebarState()]);
      // a group header only expands/collapses its sub-pages; navigation happens through the sub-items. The compact
      // sidebar (icons only, or a desktop narrower than 1100 px) shows no sub-items: there the header opens the group's
      // first page and the page shows its tabs (.subnav).
      const compact = () => document.body.classList.contains('sb-icons') || window.matchMedia('(min-width: 801px) and (max-width: 1100px)').matches;
      for (const g of document.querySelectorAll('#sidebar .group > .nav-link')) {
        g.addEventListener('click', (e) => { if (compact()) return; e.preventDefault(); g.parentElement.classList.toggle('open'); });
      }
      // left drawer (mobile)
      let drawer = $('#drawer');
      if (!drawer) {
        drawer = document.createElement('div');
        drawer.id = 'drawer';
        drawer.innerHTML = `<div class="drawer-backdrop"></div><aside class="drawer-panel" role="dialog" aria-label="Menu">
          <div class="drawer-head"><a class="brand" href="${esc(home())}"><img data-logo alt="" src="${esc(logoURL())}"><span class="${app ? 'notranslate' : ''}">${esc(app ? app.name : (brand.org_name || brand.product))}</span></a><button class="btn ghost icon" id="drawerClose" type="button" aria-label="Close">${ICON.close}</button></div>
          <div class="drawer-user">${ICON.user}<div><b>${esc(me.full_name || me.username)}</b><br><span class="muted">${esc((me.roles || []).join(', ') || 'no roles')}</span></div></div>
          <nav class="drawer-nav">${drawerLinks}</nav>
          <div class="drawer-foot"><button class="btn ghost" data-theme-toggle data-label type="button"></button>${langSwitcher('langDrawer')}<button class="btn ghost" id="drawerSignout" type="button">${ICON.logout}<span>Sign out</span></button></div>
        </aside>`;
        document.body.appendChild(drawer);
        const open = (v) => { drawer.classList.toggle('open', v); document.body.classList.toggle('drawer-open', v); $('#menuBtn').setAttribute('aria-expanded', String(v)); };
        // on a desktop with the sidebar hidden the hamburger brings the sidebar back; on mobile it opens the drawer
        $('#menuBtn').onclick = () => { if (window.innerWidth > 800 && sidebarState() === 'hidden') setSidebar('open'); else open(true); };
        $('#drawerClose').onclick = () => open(false);
        drawer.querySelector('.drawer-backdrop').onclick = () => open(false);
        $('#drawerSignout').onclick = logout;
        for (const g of drawer.querySelectorAll('.drawer-nav a[data-grp]')) g.onclick = (e) => {
          e.preventDefault();
          const v = g.getAttribute('aria-expanded') !== 'true';
          g.setAttribute('aria-expanded', String(v));
          for (const s of drawer.querySelectorAll(`.drawer-nav a[data-in="${g.dataset.grp}"]`)) s.hidden = !v;
        };
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape') open(false); });
      }
    }
    applyMode();
  }
  // ---- global search (0.68.0): the header field and Ctrl+K; GET /api/v1/search answers only what the user may see,
  // the pages of the menu are matched here. Arrow keys move, Enter opens, Esc closes.
  const GS_KIND = { page: 'Pages', device: 'Devices', datastream: 'Datastreams', asset: 'Assets', site: 'Sites', dashboard: 'Dashboards', camera: 'Cameras', flow: 'Flows', incident: 'Incidents', user: 'Users' };
  let gsDlg = null, gsSeq = 0;
  function openSearch(prefill) {
    if (!me || locked()) return;
    if (!gsDlg) {
      gsDlg = document.createElement('dialog'); gsDlg.className = 'gs-dlg'; gsDlg.setAttribute('aria-label', t('Search'));
      gsDlg.innerHTML = `<div class="gs-in">${ICON.search}<input type="search" id="gsQ" placeholder="${esc(t('Search devices, datastreams, assets, dashboards…'))}" autocomplete="off" aria-label="${esc(t('Search'))}"><button class="btn small ghost" type="button" data-gsclose>Esc</button></div><div class="gs-list" id="gsList" role="listbox"></div><div class="gs-foot"><span>↑ ↓ ${esc(t('move'))}</span><span>Enter ${esc(t('go to'))}</span><span>Esc ${esc(t('close'))}</span></div>`;
      document.body.appendChild(gsDlg);
      const q = gsDlg.querySelector('#gsQ'); let timer = null;
      q.oninput = () => { clearTimeout(timer); timer = setTimeout(() => runSearch(q.value.trim()), 200); };
      q.onkeydown = (e) => {
        const items = [...gsDlg.querySelectorAll('a.gs-item')]; let i = items.findIndex((x) => x.classList.contains('on'));
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); if (!items.length) return; i = e.key === 'ArrowDown' ? (i + 1) % items.length : (i <= 0 ? items.length - 1 : i - 1); items.forEach((x, k) => x.classList.toggle('on', k === i)); items[i].scrollIntoView({ block: 'nearest' }); }
        else if (e.key === 'Enter') { e.preventDefault(); const it = items[i >= 0 ? i : 0]; if (it) location.href = it.getAttribute('href'); }
      };
      gsDlg.querySelector('[data-gsclose]').onclick = () => gsDlg.close();
      gsDlg.addEventListener('click', (e) => { if (e.target === gsDlg) gsDlg.close(); });
    }
    const q = gsDlg.querySelector('#gsQ'); q.value = prefill || ''; if (!gsDlg.open) gsDlg.showModal(); q.focus(); runSearch(q.value.trim());
  }
  function pageHits(q) {
    const out = []; const l = q.toLowerCase();
    for (const i of navItems()) for (const x of [i].concat(i.children || [])) if (x.href && t(x.label).toLowerCase().includes(l) && !out.some((o) => o.url === x.href)) out.push({ kind: 'page', title: t(x.label), subtitle: x === i ? '' : t(i.label), url: x.href });
    return out.slice(0, 5);
  }
  async function runSearch(q) {
    const list = gsDlg.querySelector('#gsList'); const seq = ++gsSeq;
    if (!q) { list.innerHTML = `<div class="muted" style="padding:10px 8px">${esc(t('Type to search devices, datastreams, assets, sites, dashboards, cameras, flows, incidents and users.'))}</div>`; return; }
    let res = { results: [], more: [] };
    try { res = await api('GET', '/api/v1/search?limit=6&q=' + encodeURIComponent(q)); } catch (e) { /* show the pages at least */ }
    if (seq !== gsSeq) return;
    const hits = pageHits(q).concat(res.results || []);
    if (!hits.length) { list.innerHTML = `<div class="muted" style="padding:10px 8px">${esc(t('Nothing found.'))}</div>`; return; }
    let h = '', last = '';
    for (const r of hits) {
      if (r.kind !== last) { last = r.kind; h += `<div class="gs-head">${esc(t(GS_KIND[r.kind] || r.kind))}${(res.more || []).includes(r.kind) ? ' · ' + esc(t('more')) + '…' : ''}</div>`; }
      h += `<a class="gs-item" role="option" href="${esc(r.url)}"><span class="notranslate">${esc(r.title)}</span>${r.subtitle ? `<span class="muted notranslate">${esc(r.subtitle)}</span>` : ''}</a>`;
    }
    list.innerHTML = h; const first = list.querySelector('a.gs-item'); if (first) first.classList.add('on');
  }
  document.addEventListener('keydown', (e) => { if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.key === 'k' || e.key === 'K')) { if (!me || app) return; e.preventDefault(); openSearch(); } });

  function footer() {
    if ($('.footer')) return;
    const f = document.createElement('footer');
    f.className = 'footer';
    const ver = '<span class="grow"></span><span>' + (brand.version && brand.footer.show_version !== false ? 'v' + esc(brand.version) : '') + '</span>';
    if (brand.footer.white_label) { // licensed white label (spec 007): the organization's own footer
      f.innerHTML = '<div class="container row">' + (brand.footer.text ? '<span>' + esc(brand.footer.text) + '</span>' : '') +
        (brand.footer.links || []).map((l) => '<a href="' + esc(l.url) + '"' + (/^https?:/.test(l.url) ? ' target="_blank" rel="noopener noreferrer"' : '') + '>' + esc(l.label) + '</a>').join('') + ver + '</div>';
    } else {
      f.innerHTML = '<div class="container row"><span>ctrl32 · telemetry</span>' +
        '<a href="' + brand.footer.github + '" target="_blank" rel="noopener noreferrer" title="Source on GitHub" aria-label="GitHub">' + ICON.github + '</a>' +
        '<a href="' + brand.footer.license + '">License</a>' + ver + '</div>';
    }
    // in an app the bug report lives in the footer (the header has no bug button there)
    if (app) {
      const row = f.querySelector('.row'), b = document.createElement('a');
      b.href = '#'; b.id = 'bugFoot'; b.title = 'Report a bug'; b.setAttribute('aria-label', 'Report a bug'); b.innerHTML = ICON.bug;
      b.onclick = (e) => { e.preventDefault(); location.href = bugMailto(); };
      row.insertBefore(b, row.querySelector('.grow'));
    }
    ($('.content') || document.body).appendChild(f);
  }
  // White label (spec 007): the product name replaces "ctrl32 telemetry" in visible texts, titles and the icon.
  const PRODUCT_RE = /ctrl32 · telemetry|ctrl32 telemetry/g;
  function rebrandNode(root) {
    if (!brand.footer || !brand.footer.white_label || !root) return;
    if (root.nodeType === 3) root = root.parentElement;
    if (!root || !root.nodeType) return;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const n = walker.currentNode;
      if (n.nodeValue.includes('ctrl32') && !(n.parentElement && n.parentElement.closest('script,style,code,pre,textarea,.no-rebrand'))) n.nodeValue = n.nodeValue.replace(PRODUCT_RE, brand.product);
    }
    if (root.querySelectorAll) for (const el of root.querySelectorAll('[placeholder],[title],[alt],[aria-label]')) {
      for (const a of ['placeholder', 'title', 'alt', 'aria-label']) { const v = el.getAttribute(a); if (v && v.includes('ctrl32')) el.setAttribute(a, v.replace(PRODUCT_RE, brand.product)); }
    }
  }
  function applyWhiteLabel() {
    if (!brand.footer || !brand.footer.white_label) return;
    document.title = document.title.replace(PRODUCT_RE, brand.product);
    for (const l of document.querySelectorAll('link[rel~="icon"],link[rel="apple-touch-icon"]')) l.href = brand.icon_url || l.href;
    const intro = document.getElementById('intro');
    if (intro && brand.footer.intro) intro.textContent = brand.footer.intro;
    rebrandNode(document.body);
    new MutationObserver((muts) => { for (const m of muts) for (const n of m.addedNodes) rebrandNode(n); }).observe(document.body, { childList: true, subtree: true });
  }
  async function loadBranding() {
    try {
      const r = await fetch('/api/v1/branding', { credentials: 'same-origin' });
      if (r.ok) brand = Object.assign(brand, await r.json());
    } catch (e) { /* offline: defaults */ }
    applyTheme(brand.theme);
    if (brand.org_name && !document.title.includes(brand.org_name)) document.title = document.title + ' · ' + brand.org_name;
    return brand;
  }
  // ---- i18n: English is the key; the catalog of the chosen language maps English strings to translations.
  // Text nodes, placeholders and titles are translated on load and whenever the page renders new content.
  let catalog = {}, locale = 'en', langs = { enabled: ['en'], default: 'en', supported: [] };
  function storedLocale() { try { return localStorage.getItem('ctrl32.locale'); } catch (e) { return null; } }
  function pickLocale() {
    for (const c of [storedLocale(), me && me.locale, langs.default, 'en']) if (c && langs.enabled.includes(c)) return c;
    return 'en';
  }
  async function loadI18n() {
    try {
      const r = await fetch('/api/v1/i18n/languages', { credentials: 'same-origin' });
      if (r.ok) langs = await r.json();
      locale = pickLocale();
      document.documentElement.lang = locale;
      if (locale !== 'en') { const c = await fetch('/api/v1/i18n/' + locale, { credentials: 'same-origin' }); catalog = c.ok ? await c.json() : {}; }
    } catch (e) { catalog = {}; }
  }
  const t = (s) => (catalog && catalog[s]) || s;
  const t_ = t;
  // prompts and confirmations go through the catalog too (window.prompt / confirm take plain strings)
  function ask(text, def) { return window.prompt(t(text), def === undefined ? '' : def); }
  function confirmAsk(text) { return window.confirm(t(text)); }
  function translateNode(root) {
    if (!root || !catalog || !Object.keys(catalog).length) return;
    if (root.nodeType === 3) { root = root.parentElement; if (!root) return; }
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    for (const n of nodes) {
      const p = n.parentElement;
      if (!p || p.closest('script,style,code,pre,textarea,.notranslate')) continue;
      const raw = n.nodeValue, key = raw.trim();
      if (!key) continue;
      const tr = catalog[key];
      if (tr && tr !== key) n.nodeValue = raw.replace(key, tr);
    }
    if (root.querySelectorAll) for (const el of root.querySelectorAll('[placeholder],[title]')) {
      for (const a of ['placeholder', 'title']) { const v = el.getAttribute(a); if (v && catalog[v]) el.setAttribute(a, catalog[v]); }
    }
  }
  function observeI18n() {
    if (!Object.keys(catalog).length) return;
    new MutationObserver((muts) => { for (const m of muts) for (const n of m.addedNodes) translateNode(n); }).observe(document.body, { childList: true, subtree: true });
  }
  async function setLocale(code) {
    try { localStorage.setItem('ctrl32.locale', code); } catch (e) { /* private mode */ }
    if (me) { try { await api('PUT', '/api/v1/auth/locale', { locale: code }); } catch (e) { /* keep local choice */ } }
    location.reload();
  }
  function langSwitcher(id) {
    if (!langs.enabled || langs.enabled.length < 2) return '';
    const names = Object.fromEntries((langs.supported || []).map((l) => [l.code, l.name]));
    return `<select class="lang-sel" id="${id}" title="Language" aria-label="Language">${langs.enabled.map((c) => `<option value="${c}"${c === locale ? ' selected' : ''}>${esc(names[c] || c)}</option>`).join('')}</select>`;
  }
  function wireLang() { for (const s of document.querySelectorAll('.lang-sel')) s.onchange = () => setLocale(s.value); }

  // a newer version for system administrators (update mode notify or automatic, ADR-0024); hidden per version
  function updateBanner() {
    if (!can('system.admin') || app || location.pathname.startsWith('/server/updates')) return;
    api('GET', '/api/v1/server/updates/notice').then((n) => {
      if (!n || !n.version) return;
      try { if (localStorage.getItem('ctrl32.updateDismissed') === n.version) return; } catch (e) { /* storage blocked */ }
      const m = $('main');
      if (!m) return;
      const d = document.createElement('div');
      d.className = 'card update-banner';
      d.innerHTML = `<span>${esc(t('Version'))} <b>${esc(n.version)}</b> ${esc(t('is available'))}${n.window ? ' — ' + esc(t('installs automatically in the maintenance window')) + ' ' + esc(new Date(n.window).toLocaleString(document.documentElement.lang || undefined)) : ''}.</span>`
        + `<span class="grow"></span><a class="btn small primary" href="/server/updates">${esc(t('Open Updates'))}</a><button class="btn small ghost" type="button">${esc(t('Later'))}</button>`;
      d.querySelector('button').onclick = () => { try { localStorage.setItem('ctrl32.updateDismissed', n.version); } catch (e) { /* storage blocked */ } d.remove(); };
      m.prepend(d);
    }).catch(() => {});
  }

  // init: branding + session + language, then shell + footer. Pages call ctrl32.ready() and get {me, brand}.
  let readyPromise = null;
  function ready(opts) {
    opts = opts || {};
    if (!readyPromise) readyPromise = Promise.all([loadBranding(), loadMe(), loadApp()]).then(loadI18n).then(() => { renderShell(); footer(); wireLang(); translateNode(document.body); if (document.title && catalog[document.title]) document.title = catalog[document.title]; observeI18n(); applyWhiteLabel(); updateBanner(); return { me, brand, app }; });
    return readyPromise.then((r) => {
      if (opts.requireAuth && !me) { location.href = '/login'; return new Promise(() => {}); }
      if (opts.requirePerm && !can(opts.requirePerm)) { const m = $('main'); if (m) m.innerHTML = '<div class="card">Your role has no <code>' + esc(opts.requirePerm) + '</code> permission.</div>'; return new Promise(() => {}); }
      return r;
    });
  }
  // Full screen that works everywhere (0.81.0): the Fullscreen API where the browser has it for any element (Android,
  // computers, iPad), otherwise (iPhone: only a <video> could go full screen natively, without the privacy masks drawn
  // over it) the element fills the window itself. On a phone the picture turns to landscape where the browser allows it.
  const fsCur = () => document.fullscreenElement || document.webkitFullscreenElement || document.querySelector('.pseudo-fs');
  function fullscreen(el) {
    const cur = fsCur();
    if (cur) {
      if (cur.classList.contains('pseudo-fs')) { cur.classList.remove('pseudo-fs'); document.documentElement.classList.remove('has-pseudo-fs'); window.dispatchEvent(new Event('resize')); } else (document.exitFullscreen || document.webkitExitFullscreen).call(document);
      return;
    }
    const pseudo = () => { if (el === document.documentElement) return; el.classList.add('pseudo-fs'); document.documentElement.classList.add('has-pseudo-fs'); window.dispatchEvent(new Event('resize')); };
    const req = el.requestFullscreen || el.webkitRequestFullscreen;
    if (!req) { pseudo(); return; }
    try {
      const r = req.call(el);
      const turn = () => { try { if (window.matchMedia('(pointer: coarse)').matches && screen.orientation && screen.orientation.lock) screen.orientation.lock('landscape').catch(() => {}); } catch (e) { /* not allowed */ } };
      if (r && r.then) r.then(turn, pseudo); else turn();
    } catch (e) { pseudo(); }
  }
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') { const p = document.querySelector('.pseudo-fs'); if (p) fullscreen(p); } });
  if ('serviceWorker' in navigator && (location.protocol === 'https:' || location.hostname === 'localhost' || location.hostname === '127.0.0.1')) {
    window.addEventListener('load', () => { navigator.serviceWorker.register('/sw.js').catch(() => {}); });
  }
  if (window.matchMedia) window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyMode);
  applyMode();
  window.ctrl32 = { ready, loadBranding, loadMe, api, logout, toggleMode, can, esc, section, t, appAllows, fullscreen, isFullscreen: () => !!fsCur(), ask, confirm: confirmAsk, setLocale, get locale() { return locale; }, get langs() { return langs; }, get brand() { return brand; }, get me() { return me; }, get csrf() { return csrf; }, get app() { return app; }, home, enterApp, ICON,
    fmt: (t) => (t ? new Date(t).toLocaleString(document.documentElement.lang || undefined) : '—'), toast, openSearch };
  function toast(msg, kind) {
    let t = $('#toast');
    if (!t) { t = document.createElement('div'); t.id = 'toast'; document.body.appendChild(t); }
    t.textContent = t_(msg); t.className = 'show ' + (kind || '');
    clearTimeout(t._h); t._h = setTimeout(() => { t.className = ''; }, 4000);
  }
})();
