// The dashboard of podkop-exp has no service worker. One that Yacd-meta registered at this
// address before (on https or localhost) serves its own copy from its cache; when the browser
// looks for an update of it, it gets this one: the caches are emptied, the worker goes, and
// the open pages are loaded again from the router.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) await caches.delete(key);
      await self.registration.unregister();
      for (const client of await self.clients.matchAll({ type: 'window' }))
        client.navigate(client.url);
    })(),
  );
});
