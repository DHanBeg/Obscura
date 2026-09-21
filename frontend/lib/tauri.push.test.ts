import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// requestWebPushPermission REGRESYONU: eskiden `await Notification.requestPermission()`
// (kullanıcı yanıtlamazsa) ve `await navigator.serviceWorker.ready` (kayıtlı SW yoksa
// ASLA çözülmez; repoda SW kaydı yok) AppShell bootstrap'ını kilitliyor, createWS hiç
// çağrılmıyor, canlı mesaj gelmiyordu. Bu testler fonksiyonun ASLA asılmadığını sınar.

type PermissionState = "default" | "granted" | "denied";

interface Env {
  requestPermission: ReturnType<typeof vi.fn>;
  getRegistration: ReturnType<typeof vi.fn>;
  subscribe: ReturnType<typeof vi.fn>;
  readyAccess: ReturnType<typeof vi.fn>;
}

function setupEnv(permission: PermissionState, opts: { registration?: "none" | "hang" | "ok" } = {}): Env {
  const requestPermission = vi.fn(async () => permission);
  const subscribe = vi.fn(async () => ({ endpoint: "https://push.example/abc" }));
  const readyAccess = vi.fn();
  const registration = opts.registration ?? "none";
  const getRegistration = vi.fn(() => {
    if (registration === "hang") return new Promise(() => {}); // asla çözülmez
    if (registration === "ok") return Promise.resolve({ pushManager: { subscribe } });
    return Promise.resolve(undefined);
  });

  const notification = { permission, requestPermission };
  const serviceWorker = {
    getRegistration,
    // eski kod .ready'e dokunuyordu; artık hiç erişilmemeli
    get ready() {
      readyAccess();
      return new Promise(() => {});
    },
  };
  vi.stubGlobal("window", { Notification: notification });
  vi.stubGlobal("Notification", notification);
  vi.stubGlobal("navigator", { serviceWorker });
  return { requestPermission, getRegistration, subscribe, readyAccess };
}

async function load() {
  vi.resetModules();
  return import("./tauri");
}

describe("requestWebPushPermission — bootstrap'ı kilitlemez", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("izin sorulmamışken (default) ve etkileşimsiz: pencere AÇMAZ, hemen null", async () => {
    const env = setupEnv("default");
    const { requestWebPushPermission } = await load();

    await expect(requestWebPushPermission()).resolves.toBeNull();
    expect(env.requestPermission).not.toHaveBeenCalled();
    expect(env.getRegistration).not.toHaveBeenCalled();
  });

  it("etkileşimli çağrıda (interactive=true) izin ister", async () => {
    const env = setupEnv("default", { registration: "none" });
    const { requestWebPushPermission } = await load();

    await expect(requestWebPushPermission(true)).resolves.toBeNull(); // izin verildi ama SW yok
    expect(env.requestPermission).toHaveBeenCalledTimes(1);
  });

  it("izin reddedilmişse null, SW'ye bakmaz", async () => {
    const env = setupEnv("denied");
    const { requestWebPushPermission } = await load();

    await expect(requestWebPushPermission(true)).resolves.toBeNull();
    expect(env.getRegistration).not.toHaveBeenCalled();
  });

  it("izin verilmiş ama kayıtlı SW YOK: serviceWorker.ready'ye dokunmadan hemen null (asılmaz)", async () => {
    const env = setupEnv("granted", { registration: "none" });
    const { requestWebPushPermission } = await load();

    await expect(requestWebPushPermission()).resolves.toBeNull();
    expect(env.readyAccess).not.toHaveBeenCalled();
    expect(env.subscribe).not.toHaveBeenCalled();
  });

  it("SW araması hiç çözülmese bile ~3 sn'de null döner (zaman aşımı, asılmaz)", async () => {
    setupEnv("granted", { registration: "hang" });
    const { requestWebPushPermission } = await load();

    const pending = requestWebPushPermission();
    let settled = false;
    void pending.then(() => { settled = true; });

    await vi.advanceTimersByTimeAsync(2_900);
    expect(settled).toBe(false); // zaman aşımından önce hâlâ bekliyor
    await vi.advanceTimersByTimeAsync(200);
    await expect(pending).resolves.toBeNull();
  });

  it("izin + kayıtlı SW varsa abonelik JSON'u döner", async () => {
    const env = setupEnv("granted", { registration: "ok" });
    const { requestWebPushPermission } = await load();

    const token = await requestWebPushPermission();
    expect(token).toBe(JSON.stringify({ endpoint: "https://push.example/abc" }));
    expect(env.subscribe).toHaveBeenCalledTimes(1);
  });

  it("subscribe hata verirse (VAPID yok vb.) fırlatmaz, null döner", async () => {
    const env = setupEnv("granted", { registration: "ok" });
    env.subscribe.mockRejectedValueOnce(new Error("no active service worker"));
    const { requestWebPushPermission } = await load();

    await expect(requestWebPushPermission()).resolves.toBeNull();
  });
});
