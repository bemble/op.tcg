// Minimal service worker. Its only job is to make the site installable as a PWA
// on Android: Chrome only creates a real WebAPK (which uses the maskable
// manifest icon, full-bleed, no white-circle shortcut) when a service worker
// with a fetch handler is registered. We deliberately do NOT cache anything —
// every request goes straight to the network, so redeploys are picked up
// immediately with no stale-asset headaches.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {
  // Network pass-through: the handler's presence satisfies the install
  // criteria; not calling respondWith() means requests behave exactly as they
  // would without a service worker.
});
