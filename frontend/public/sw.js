/* identity workspace service worker: Web Push reminders without offline data caching. */
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("push", (event) => {
  let payload = {};
  try {
    payload = event.data ? event.data.json() : {};
  } catch {
    payload = { body: event.data ? event.data.text() : "Пора выполнить задачу" };
  }
  const title = typeof payload.title === "string" && payload.title ? payload.title : "identity workspace";
  const body = typeof payload.body === "string" ? payload.body : "Пора выполнить задачу";
  const url = typeof payload.url === "string" && payload.url.startsWith("/") ? payload.url : "/?view=tasks";
  const tag = typeof payload.tag === "string" && payload.tag ? payload.tag : "identity-workspace-reminder";
  event.waitUntil(self.registration.showNotification(title, {
    body,
    icon: "/identity-workspace-icon-192-v2.png",
    badge: "/identity-workspace-icon-192-v2.png",
    tag,
    renotify: false,
    data: { url },
  }));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(event.notification.data?.url || "/?view=tasks", self.location.origin).href;
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const client of windows) {
      if ("navigate" in client) await client.navigate(target);
      if ("focus" in client) return client.focus();
    }
    return self.clients.openWindow ? self.clients.openWindow(target) : undefined;
  })());
});
