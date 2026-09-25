// ctrl32 telemetry — service worker: precaches the static shell so the app installs as a PWA and works offline,
// but serves shell files network-first so a new release is visible on the very next load (cache is the fallback).
// The cache name carries the server version (substituted when /sw.js is served), so every release starts fresh.
// API responses, pages and exports are never cached (records must always come from the server).
const SHELL = 'ctrl32-shell-__VERSION__';
const FILES = ['/app.css', '/app.js', '/logo.svg', '/logo-dark.svg', '/favicon.svg', '/icon.svg', '/icon-192.png', '/icon-512.png', '/manifest.webmanifest'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(SHELL).then((c) => c.addAll(FILES)).then(() => self.skipWaiting()));
});
self.addEventListener('activate', (e) => {
  e.waitUntil(caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener('fetch', (e) => {
  const u = new URL(e.request.url);
  if (e.request.method !== 'GET' || u.origin !== location.origin || !FILES.includes(u.pathname)) return;
  e.respondWith(caches.open(SHELL).then(async (c) => {
    try {
      const res = await fetch(e.request, { cache: 'no-cache' });
      if (res.ok) c.put(e.request, res.clone());
      return res;
    } catch (err) {
      const cached = await c.match(e.request);
      if (cached) return cached;
      throw err;
    }
  }));
});

// Web Push (URS C-06): the server sends {title, body, url, tag}; clicking opens or focuses the app at the URL.
self.addEventListener('push', (e) => {
  let d = {};
  try { d = e.data ? e.data.json() : {}; } catch (err) { d = { body: e.data ? e.data.text() : '' }; }
  const title = d.title || 'ctrl32 telemetry';
  e.waitUntil(self.registration.showNotification(title, { body: d.body || '', tag: d.tag || undefined, icon: '/icon-192.png', badge: '/icon-192.png', data: { url: d.url || '/' }, renotify: !!d.tag }));
});
self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const url = (e.notification.data && e.notification.data.url) || '/';
  e.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
    for (const c of list) { if ('focus' in c) { c.navigate(url); return c.focus(); } }
    return self.clients.openWindow(url);
  }));
});
