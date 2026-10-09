import { api } from "../http/apiClient";

export type DeviceNotificationState = "unknown" | "enabled" | "denied" | "unsupported" | "error";

function urlBase64ToUint8Array(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4);
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(base64);
  return Uint8Array.from(raw, (character) => character.charCodeAt(0));
}

function usesApplicationServerKey(subscription: PushSubscription, expected: Uint8Array) {
  const current = subscription.options.applicationServerKey;
  if (!current) return true;
  const bytes = new Uint8Array(current);
  return bytes.length === expected.length && bytes.every((value, index) => value === expected[index]);
}

export function currentDeviceNotificationState(): DeviceNotificationState {
  if (!("Notification" in window) || !("serviceWorker" in navigator) || !("PushManager" in window)) return "unsupported";
  return Notification.permission === "granted" ? "enabled" : Notification.permission === "denied" ? "denied" : "unknown";
}

export async function unregisterDeviceNotifications() {
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) return;
  try {
    const registration = await navigator.serviceWorker.ready;
    const subscription = await registration.pushManager.getSubscription();
    if (!subscription) return;
    try { await api.deletePushSubscription(subscription.endpoint); } catch { /* logout must continue */ }
    await subscription.unsubscribe();
  } catch {
    // The server session will still be closed even if the browser cannot remove its local subscription.
  }
}

export async function registerDeviceNotifications(
  requestPermission = true,
  config?: { configured: boolean; publicKey: string },
): Promise<DeviceNotificationState> {
  try {
    if (currentDeviceNotificationState() === "unsupported") return "unsupported";
    if (config && (!config.configured || !config.publicKey)) return "unsupported";
    const permission = requestPermission ? await Notification.requestPermission() : Notification.permission;
    if (permission !== "granted") return permission === "denied" ? "denied" : "unknown";
    const resolvedConfig = config ?? await api.notificationConfig();
    if (!resolvedConfig.configured || !resolvedConfig.publicKey) return "unsupported";
    const registration = await navigator.serviceWorker.ready;
    const applicationServerKey = urlBase64ToUint8Array(resolvedConfig.publicKey);
    let subscription = await registration.pushManager.getSubscription();
    if (subscription && !usesApplicationServerKey(subscription, applicationServerKey)) {
      try { await api.deletePushSubscription(subscription.endpoint); } catch { /* stale server entry expires on delivery */ }
      await subscription.unsubscribe();
      subscription = null;
    }
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey,
      });
    }
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys.auth) return "error";
    await api.savePushSubscription({ endpoint: json.endpoint, p256dh: json.keys.p256dh, auth: json.keys.auth });
    return "enabled";
  } catch {
    return "error";
  }
}
