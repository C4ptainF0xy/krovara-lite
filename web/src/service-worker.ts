/// <reference types="@sveltejs/kit" />
import { build, files, version } from '$service-worker';

const CACHE = `krovara-cache-${version}`;
const ASSETS = [...build, ...files];

const sw = self as unknown as ServiceWorkerGlobalScope;

sw.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(ASSETS)).then(() => sw.skipWaiting()));
});

sw.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => sw.clients.claim())
  );
});

sw.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  if (url.origin !== location.origin || url.pathname.startsWith('/api')) return;

  if (ASSETS.includes(url.pathname)) {
    event.respondWith(caches.match(req).then((hit) => hit ?? fetch(req)));
    return;
  }

  if (req.mode === 'navigate') {
    event.respondWith(
      fetch(req).catch(async () => {
        const cached = (await caches.match(req)) ?? (await caches.match('/'));
        return cached ?? new Response('Hors ligne', { status: 503, headers: { 'Content-Type': 'text/plain' } });
      })
    );
  }
});
