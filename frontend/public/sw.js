// Obscura Service Worker — Web Push Bildirimleri
// Next.js public/ klasöründen servis edilir
'use strict';

const CACHE_NAME = 'obscura-v1';

// ─── Install ──────────────────────────────────────────────────────────────────
self.addEventListener('install', (event) => {
  self.skipWaiting();
});

// ─── Activate ─────────────────────────────────────────────────────────────────
self.addEventListener('activate', (event) => {
  event.waitUntil(clients.claim());
});

// ─── Push Bildirimi ───────────────────────────────────────────────────────────
self.addEventListener('push', (event) => {
  if (!event.data) return;

  let payload;
  try {
    payload = event.data.json();
  } catch {
    payload = { title: 'Obscura', body: event.data.text() };
  }

  const title = payload.notification?.title || payload.title || 'Obscura';
  const options = {
    body: payload.notification?.body || payload.body || 'Yeni mesajınız var',
    icon: '/icons/icon-192.png',
    badge: '/icons/badge-72.png',
    tag: payload.data?.conv_id || 'obscura-msg',
    renotify: true,
    data: payload.data || {},
    actions: [
      { action: 'open', title: 'Aç' },
      { action: 'dismiss', title: 'Kapat' },
    ],
    vibrate: [200, 100, 200],
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

// ─── Bildirime Tıklama ────────────────────────────────────────────────────────
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const convId = event.notification.data?.conv_id;
  const url = convId ? `/chats/${convId}` : '/chats';

  if (event.action === 'dismiss') return;

  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
      // Açık sekme varsa oraya git
      for (const client of windowClients) {
        if (client.url.includes(self.location.origin)) {
          client.focus();
          client.navigate(url);
          return;
        }
      }
      // Yeni sekme aç
      return clients.openWindow(url);
    })
  );
});

// ─── Push Subscription Değişikliği ────────────────────────────────────────────
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    self.registration.pushManager.subscribe(event.oldSubscription.options)
      .then((subscription) => {
        // Yeni token'ı backend'e gönder
        return fetch('/api/push/resubscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ subscription }),
        });
      })
  );
});
