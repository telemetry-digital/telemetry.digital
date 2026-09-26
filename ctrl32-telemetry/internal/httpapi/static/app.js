// ctrl32 telemetry — shared page runtime: branding/theme, navigation shell (header, left drawer on mobile),
// fixed footer, session helpers, PWA registration. The footer (product name, GitHub link, license link) is part
// of the product and is not configurable. Nothing user-specific is rendered before the session state is known.
(function () {
  const ICON = {
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
    file: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21h-10a2 2 0 0 1 -2 -2v-14a2 2 0 0 1 2 -2h7l5 5v11a2 2 0 0 1 -2 2z"/></svg>',
  };
  const $ = (s) => document.querySelector(s);
  const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  let brand = { product: 'ctrl32 telemetry', version: '', org_name: '', logo_url: '/logo.svg', theme: { primary: '#e30613', font: '', mode: 'auto', radius: 'small', font_scale: 'normal' },
    footer: { text: 'ctrl32 · telemetry', github: 'https://github.com/telemetry-digital/ctrl32-telemetry', license: '/license' } };
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
  const can = (p) => !!(me && me.permissions && me.permissions.includes(p));
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
  async function logout() { try { await api('POST', '/api/v1/auth/logout', {}); } catch (e) { /* already gone */ } location.href = '/'; }

  // ---- navigation shell -------------------------------------------------------------------------
  // Items are filtered by the signed-in user's permissions. Nothing user-specific appears before loadMe().
  // Menu: top-level pages with sub-pages (each sub-page is its own screen, nothing is crammed on one page).
  function navItems() {
    const items = [
      { href: '/', label: 'Dashboard', icon: ICON.dashboard, show: !!me },
      { href: '/dashboards', label: 'Dashboards', icon: ICON.dashboard, show: can('data.read') },
      { href: '/devices', label: 'Devices', icon: ICON.device, show: can('data.read') },
      { href: '/incidents', label: 'Incidents', icon: ICON.alert, show: can('data.read') },
      { href: '/assets', label: 'Assets', icon: ICON.assets, show: can('data.read'), children: [
        { href: '/assets/sites', label: 'Sites' }, { href: '/assets', label: 'Assets' }, { href: '/assets/rules', label: 'Alarm rules' },
        { href: '/assets/automation', label: 'Automation', show: can('config.write') }] },
      { href: '/fota', label: 'Firmware', icon: ICON.file, show: can('fota.read'), children: [
        { href: '/fota', label: 'Images' }, { href: '/fota/campaigns', label: 'Campaigns' }] },
      { href: '/users', label: 'Users', icon: ICON.users, show: can('user.admin'), children: [
        { href: '/users', label: 'Accounts' }, { href: '/users/roles', label: 'Roles and permissions' }] },
      { href: '/settings', label: 'Settings', icon: ICON.settings, show: can('content.write'), children: [
        { href: '/settings/branding', label: 'Branding and theme' }, { href: '/settings/templates', label: 'Report templates' },
        { href: '/settings/reports', label: 'Stored reports' }, { href: '/settings/connectors', label: 'Connectors' },
        { href: '/settings/languages', label: 'Languages' }, { href: '/settings/translations', label: 'Translations' },
        { href: '/settings/export', label: 'Organization export' }, { href: '/settings/notifications', label: 'Notifications' }, { href: '/settings/approvals', label: 'Approvals' }, { href: '/settings/policy', label: 'Security policy' }] },
      { href: '/organizations', label: 'System', icon: ICON.system, show: can('system.admin'), children: [
        { href: '/organizations', label: 'Organizations' }, { href: '/server', label: 'Server' }, { href: '/server/domain', label: 'Domain and TLS' },
        { href: '/server/wireguard', label: 'WireGuard' }, { href: '/server/backups', label: 'Backups' }, { href: '/server/redundancy', label: 'Redundant database' }, { href: '/server/updates', label: 'Updates' }] },
      { href: '/account', label: 'My account', icon: ICON.user, show: !!me, children: [
        { href: '/account', label: 'Profile' }, { href: '/account/security', label: 'Password and MFA' }, { href: '/account/tokens', label: 'API tokens' }, { href: '/account/push', label: 'Notifications' }, { href: '/api/docs', label: 'API documentation' }] },
    ];
    for (const i of items) if (i.children) i.children = i.children.filter((c) => c.show === undefined || c.show);
    return items.filter((i) => i.show);
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
  function section(defaultName, names) {
    const seg = location.pathname.replace(/\/$/, '').split('/').pop();
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
    return 'mailto:bugs@ctrl32.com?subject=' + encodeURIComponent('[ctrl32 telemetry ' + (brand.version || '') + '] bug report') + '&body=' + encodeURIComponent(body);
  }
  function bugButton(cls) { return `<a class="btn ghost icon ${cls}" id="bugBtn" href="#" title="Report a bug (opens your mail client, bugs@ctrl32.com)" aria-label="Report a bug">${ICON.bug}</a>`; }
  function wireBug() { const b = $('#bugBtn'); if (b) b.onclick = (e) => { e.preventDefault(); location.href = bugMailto(); }; }
  function renderShell() {
    const header = $('header.topbar');
    if (!header) return;
    const items = navItems();
    const chev = '<svg class="chev" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6l-6 6"/></svg>';
    const link = (i, sub) => `<a class="nav-link${(sub ? isExact(i.href) : isActive(i.href)) ? ' active' : ''}" href="${i.href}" title="${esc(i.label)}">${i.icon || ''}<span>${esc(i.label)}</span>${!sub && i.children ? chev : ''}</a>`;
    const links = items.map((i) => i.children
      ? `<div class="group${isActive(i.href) ? ' open' : ''}">${link(i)}<div class="sub">${i.children.map((c) => link(c, true)).join('')}</div></div>`
      : link(i)).join('');
    header.innerHTML = `<div class="container row">
      ${me ? `<button class="btn ghost icon only-mobile" id="menuBtn" type="button" aria-label="Menu" aria-expanded="false">${ICON.menu}</button>` : ''}
      <a class="brand" href="/"><img id="logo" data-logo alt="telemetry.digital" src="${esc(logoURL())}">${me ? `<span id="orgname">${esc(brand.org_name || brand.product)}</span>` : ''}</a>
      <span class="grow"></span>
      ${me ? '' : (brand.public_read ? '<span class="badge warn" title="http.public_read is enabled: anyone can read data without signing in. Demo instances only.">Read-only demo · sign in to change anything</span>' : '')}
      ${bugButton(me ? 'only-mobile' : '')}
      ${me ? '' : langSwitcher('langTop')}
    </div>`;
    // Desktop: the bug button lives in its own strip at the top of the page (in flow, sticky), never over page buttons.
    if (me && !$('#bugBtnDesk')) { const host = $('main'); const strip = document.createElement('div'); strip.className = 'top-strip only-desktop'; const b = document.createElement('a'); b.id = 'bugBtnDesk'; b.className = 'btn ghost icon bug-btn'; b.href = '#'; b.title = 'Report a bug (opens your mail client, bugs@ctrl32.com)'; b.setAttribute('aria-label', 'Report a bug'); b.innerHTML = ICON.bug; b.onclick = (e) => { e.preventDefault(); location.href = bugMailto(); }; strip.appendChild(b); if (host) { host.prepend(strip); document.body.classList.add('has-strip'); } else document.body.appendChild(strip); }
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
      $('#sidebar').innerHTML = `<div class="side-top"><a class="brand side-brand" href="/" title="${esc(brand.org_name || brand.product)}"><img data-logo alt="" src="${esc(logoURL())}"><img class="mark" alt="" src="/favicon.svg"></a></div>
        <div class="side-org" title="${esc(brand.org_name || brand.product)}"><span>${esc(brand.org_name || brand.product)}</span></div>
        <nav class="side-nav" aria-label="Main">${links}</nav>
        <div class="side-foot">
          <button class="btn ghost side-toggle" id="sideToggle" type="button" title="Collapse menu (icons, hidden, open)">${ICON.chevL}<span>Collapse menu</span></button>
          <div class="side-user" title="${esc((me.roles || []).join(', '))}">${ICON.user}<div><b>${esc(me.username)}</b><br><span class="muted">${esc((me.roles || []).join(', ') || 'no roles')}</span></div></div>
          <button class="btn ghost" data-theme-toggle data-label type="button"></button>
          ${langSwitcher('langSide')}
          <a class="btn ghost" href="/account" title="My account">${ICON.user}<span>My account</span></a>
          <button class="btn ghost" id="signout" type="button" title="Sign out">${ICON.logout}<span>Sign out</span></button>
        </div>`;
      $('#signout').onclick = logout;
      applySidebar();
      $('#sideToggle').onclick = () => setSidebar({ open: 'icons', icons: 'hidden', hidden: 'open' }[sidebarState()]);
      // a group header only expands/collapses its sub-pages; navigation happens through the sub-items
      for (const g of document.querySelectorAll('#sidebar .group > .nav-link')) {
        g.addEventListener('click', (e) => { e.preventDefault(); g.parentElement.classList.toggle('open'); });
      }
      // left drawer (mobile)
      let drawer = $('#drawer');
      if (!drawer) {
        drawer = document.createElement('div');
        drawer.id = 'drawer';
        drawer.innerHTML = `<div class="drawer-backdrop"></div><aside class="drawer-panel" role="dialog" aria-label="Menu">
          <div class="drawer-head"><a class="brand" href="/"><img data-logo alt="" src="${esc(logoURL())}"><span>${esc(brand.org_name || brand.product)}</span></a><button class="btn ghost icon" id="drawerClose" type="button" aria-label="Close">${ICON.close}</button></div>
          <div class="drawer-user">${ICON.user}<div><b>${esc(me.full_name || me.username)}</b><br><span class="muted">${esc((me.roles || []).join(', ') || 'no roles')}</span></div></div>
          <nav class="drawer-nav">${items.map((i) => `<a class="nav-link${isActive(i.href) ? ' active' : ''}" href="${i.href}">${i.icon}<span>${esc(i.label)}</span></a>` + (i.children ? i.children.map((c) => `<a class="nav-link sub-link${isExact(c.href) ? ' active' : ''}" href="${c.href}"><span style="padding-left:30px">${esc(c.label)}</span></a>`).join('') : '')).join('')}</nav>
          <div class="drawer-foot"><button class="btn ghost" data-theme-toggle data-label type="button"></button>${langSwitcher('langDrawer')}<button class="btn ghost" id="drawerSignout" type="button">${ICON.logout}<span>Sign out</span></button></div>
        </aside>`;
        document.body.appendChild(drawer);
        const open = (v) => { drawer.classList.toggle('open', v); document.body.classList.toggle('drawer-open', v); $('#menuBtn').setAttribute('aria-expanded', String(v)); };
        // on a desktop with the sidebar hidden the hamburger brings the sidebar back; on mobile it opens the drawer
        $('#menuBtn').onclick = () => { if (window.innerWidth > 800 && sidebarState() === 'hidden') setSidebar('open'); else open(true); };
        $('#drawerClose').onclick = () => open(false);
        drawer.querySelector('.drawer-backdrop').onclick = () => open(false);
        $('#drawerSignout').onclick = logout;
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape') open(false); });
      }
    }
    applyMode();
  }
  function footer() {
    if ($('.footer')) return;
    const f = document.createElement('footer');
    f.className = 'footer';
    f.innerHTML = '<div class="container row"><span>ctrl32 · telemetry</span>' +
      '<a href="' + brand.footer.github + '" target="_blank" rel="noopener noreferrer" title="Source on GitHub" aria-label="GitHub">' + ICON.github + '</a>' +
      '<a href="' + brand.footer.license + '">License</a><span class="grow"></span><span>' + (brand.version ? 'v' + brand.version : '') + '</span></div>';
    ($('.content') || document.body).appendChild(f);
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

  // init: branding + session + language, then shell + footer. Pages call ctrl32.ready() and get {me, brand}.
  let readyPromise = null;
  function ready(opts) {
    opts = opts || {};
    if (!readyPromise) readyPromise = Promise.all([loadBranding(), loadMe()]).then(loadI18n).then(() => { renderShell(); footer(); wireLang(); translateNode(document.body); if (document.title && catalog[document.title]) document.title = catalog[document.title]; observeI18n(); return { me, brand }; });
    return readyPromise.then((r) => {
      if (opts.requireAuth && !me) { location.href = '/login'; return new Promise(() => {}); }
      if (opts.requirePerm && !can(opts.requirePerm)) { const m = $('main'); if (m) m.innerHTML = '<div class="card">Your role has no <code>' + esc(opts.requirePerm) + '</code> permission.</div>'; return new Promise(() => {}); }
      return r;
    });
  }
  if ('serviceWorker' in navigator && (location.protocol === 'https:' || location.hostname === 'localhost' || location.hostname === '127.0.0.1')) {
    window.addEventListener('load', () => { navigator.serviceWorker.register('/sw.js').catch(() => {}); });
  }
  if (window.matchMedia) window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyMode);
  applyMode();
  window.ctrl32 = { ready, loadBranding, loadMe, api, logout, toggleMode, can, esc, section, t, ask, confirm: confirmAsk, setLocale, get locale() { return locale; }, get langs() { return langs; }, get brand() { return brand; }, get me() { return me; }, get csrf() { return csrf; }, ICON,
    fmt: (t) => (t ? new Date(t).toLocaleString() : '—'), toast };
  function toast(msg, kind) {
    let t = $('#toast');
    if (!t) { t = document.createElement('div'); t.id = 'toast'; document.body.appendChild(t); }
    t.textContent = t_(msg); t.className = 'show ' + (kind || '');
    clearTimeout(t._h); t._h = setTimeout(() => { t.className = ''; }, 4000);
  }
})();
