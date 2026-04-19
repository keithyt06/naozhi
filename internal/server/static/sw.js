// Minimal service worker — required for PWA installability.
// No caching strategy; just satisfies the browser's SW requirement
// so that "Add to Home Screen" works and permissions persist.
//
// CACHE_VERSION is bumped whenever the asset pipeline changes so any
// pre-Phase-1 caches in the wild are evicted on next SW install.
const CACHE_VERSION = 'naozhi-phase1-v1';
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => {
  e.waitUntil((async () => {
    const keys = await caches.keys();
    await Promise.all(keys.filter(k => k !== CACHE_VERSION).map(k => caches.delete(k)));
    await self.clients.claim();
  })());
});
self.addEventListener('fetch', () => {});
