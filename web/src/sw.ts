/// <reference lib="webworker" />
// Custom service worker (vite-plugin-pwa injectManifest strategy): precaches
// the app shell for fast/offline-tolerant loads, and handles Web Push
// notifications. Account/ledger data is never cached here — it always comes
// live from the API.
import { precacheAndRoute } from "workbox-precaching";

declare let self: ServiceWorkerGlobalScope;

precacheAndRoute(self.__WB_MANIFEST);

self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

interface PushPayload {
  title?: string;
  body?: string;
  type?: string;
  data?: Record<string, unknown>;
}

self.addEventListener("push", (event) => {
  let payload: PushPayload = {};
  try {
    payload = event.data?.json() ?? {};
  } catch {
    payload = { body: event.data?.text() };
  }
  const title = payload.title || "BenefitCoins";
  event.waitUntil(
    self.registration.showNotification(title, {
      body: payload.body,
      icon: "/icons/icon-192.png",
      badge: "/icons/icon-192.png",
      tag: payload.type,
      data: payload.data ?? {},
    }),
  );
});

// Clicking a notification focuses an existing tab if one's open, otherwise
// opens a new one at the app.
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ("focus" in client) return (client as WindowClient).focus();
      }
      return self.clients.openWindow("/app");
    }),
  );
});
