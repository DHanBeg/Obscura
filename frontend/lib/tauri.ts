/**
 * Tauri native API entegrasyonu
 * Tauri ortamında native keychain ve bildirimler kullanılır,
 * web ortamında localStorage fallback
 */

export const isTauri = typeof window !== "undefined" && "__TAURI__" in window;

// ── Token yönetimi ────────────────────────────────────────────────────────────

export async function getToken(): Promise<string | null> {
  if (!isTauri) return localStorage.getItem("obscura_token");
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<string | null>("get_token");
}

export async function setToken(token: string): Promise<void> {
  if (!isTauri) { localStorage.setItem("obscura_token", token); return; }
  const { invoke } = await import("@tauri-apps/api/core");
  await invoke("set_token", { token });
}

export async function deleteToken(): Promise<void> {
  if (!isTauri) { localStorage.removeItem("obscura_token"); return; }
  const { invoke } = await import("@tauri-apps/api/core");
  await invoke("delete_token");
}

// ── Bildirimler ───────────────────────────────────────────────────────────────

export async function showNotification(title: string, body: string): Promise<void> {
  if (!isTauri) {
    if ("Notification" in window && Notification.permission === "granted") {
      new Notification(title, { body, icon: "/favicon.ico" });
    }
    return;
  }
  const { invoke } = await import("@tauri-apps/api/core");
  await invoke("show_notification", { title, body });
}

// ── Event listener (Tauri → frontend) ────────────────────────────────────────

export async function onTauriEvent(event: string, handler: (payload: any) => void): Promise<() => void> {
  if (!isTauri) return () => {};
  const { listen } = await import("@tauri-apps/api/event");
  const unlisten = await listen(event, (e) => handler(e.payload));
  return unlisten;
}

// ── App version ───────────────────────────────────────────────────────────────

export async function getAppVersion(): Promise<string> {
  if (!isTauri) return "web";
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<string>("get_app_version");
}

// ── FCM Push Token (Web) ─────────────────────────────────────────────────────

/**
 * Web Push izni iste ve FCM token al.
 * Tauri native'de kullanılmaz — sadece web için.
 * VAPID key ve Firebase config gerektirir.
 */
export async function requestWebPushPermission(): Promise<string | null> {
  if (isTauri) return null;
  if (!("Notification" in window) || !("serviceWorker" in navigator)) return null;

  const permission = await Notification.requestPermission();
  if (permission !== "granted") return null;

  // Service worker zaten kayıtlıysa token dön
  try {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: process.env.NEXT_PUBLIC_VAPID_KEY,
    });
    return JSON.stringify(sub);
  } catch {
    return null;
  }
}
