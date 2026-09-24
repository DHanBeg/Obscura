import { describe, it, expect, vi, beforeEach } from "vitest";
import { createTabNotifier, notificationBody, type TabNotifyEnv } from "./tab-notify";

interface FakeEnv extends TabNotifyEnv {
  title: string;
  background: boolean;
  badges: Array<number | null>;
  sounds: number;
  os: Array<{ title: string; body: string }>;
}

function fakeEnv(over: Partial<TabNotifyEnv> = {}): FakeEnv {
  const env: FakeEnv = {
    title: "Obscura",
    background: true,
    badges: [],
    sounds: 0,
    os: [],
    isBackground: () => env.background,
    getTitle: () => env.title,
    setTitle: (t) => {
      env.title = t;
    },
    setFaviconBadge: (c) => {
      env.badges.push(c);
    },
    playSound: () => {
      env.sounds += 1;
    },
    showOsNotification: (title, body) => {
      env.os.push({ title, body });
    },
    ...over,
  };
  return env;
}

const msg = (id: string, extra: Record<string, unknown> = {}) => ({
  id,
  type: "text",
  fromDid: "did:peer",
  ownDid: "did:me",
  plaintext: "selam",
  ...extra,
});

describe("tab-notify: arka plandayken bildirir", () => {
  let env: FakeEnv;
  beforeEach(() => {
    env = fakeEnv();
  });

  it("başlık sayacı, favicon rozeti, ses ve OS bildirimi tetiklenir", () => {
    const n = createTabNotifier(env);
    expect(n.onIncoming(msg("m1"))).toBe(true);
    expect(env.title).toBe("(1) Obscura");
    expect(env.badges.at(-1)).toBe(1);
    expect(env.sounds).toBe(1);
    expect(env.os).toEqual([{ title: "Yeni mesaj", body: "selam" }]);
  });

  it("birden çok mesajda sayaç artar; 99'dan sonra 99+", () => {
    const n = createTabNotifier(env);
    n.onIncoming(msg("a"));
    n.onIncoming(msg("b"));
    expect(env.title).toBe("(2) Obscura");
    for (let i = 0; i < 100; i++) n.onIncoming(msg(`x${i}`));
    expect(n.unread).toBe(102);
    expect(env.title).toBe("(99+) Obscura");
  });

  it("öne gelince (reset) başlık ve favicon geri gelir", () => {
    const n = createTabNotifier(env);
    n.onIncoming(msg("a"));
    n.onIncoming(msg("b"));
    n.reset();
    expect(env.title).toBe("Obscura");
    expect(env.badges.at(-1)).toBeNull();
    expect(n.unread).toBe(0);
  });

  it("reset sonrası yeni mesaj 1'den başlar", () => {
    const n = createTabNotifier(env);
    n.onIncoming(msg("a"));
    n.reset();
    n.onIncoming(msg("b"));
    expect(env.title).toBe("(1) Obscura");
  });

  it("sayfa geçişi başlığı değiştirince önek tekrarlanmadan yeniden uygulanır", () => {
    const n = createTabNotifier(env);
    n.onIncoming(msg("a"));
    n.onIncoming(msg("b"));
    env.title = "Sohbetler"; // Next.js başlığı değiştirdi, önek gitti
    n.refreshTitle();
    expect(env.title).toBe("(2) Sohbetler");
    n.refreshTitle();
    expect(env.title).toBe("(2) Sohbetler"); // çift önek yok
  });
});

describe("tab-notify: bildirmemesi gerekenler", () => {
  it("ön plandayken (sekme görünür + odakta) hiçbir şey yapmaz", () => {
    const env = fakeEnv({ isBackground: () => false });
    const n = createTabNotifier(env);
    expect(n.onIncoming(msg("m1"))).toBe(false);
    expect(env.title).toBe("Obscura");
    expect(env.sounds).toBe(0);
    expect(env.os).toEqual([]);
    expect(env.badges).toEqual([]);
  });

  it("kendi mesajını, sistem mesajını ve okundu bilgisini saymaz", () => {
    const env = fakeEnv();
    const n = createTabNotifier(env);
    expect(n.onIncoming(msg("a", { fromDid: "did:me" }))).toBe(false);
    expect(n.onIncoming(msg("b", { type: "system", plaintext: "__init__" }))).toBe(false);
    expect(n.onIncoming(msg("c", { type: "read_receipt" }))).toBe(false);
    expect(n.unread).toBe(0);
    expect(env.title).toBe("Obscura");
  });

  it("aynı mesaj kimliği iki kez gelirse (yeniden bağlanma/çift WS) bir kez sayılır", () => {
    const env = fakeEnv();
    const n = createTabNotifier(env);
    expect(n.onIncoming(msg("dup"))).toBe(true);
    expect(n.onIncoming(msg("dup"))).toBe(false);
    expect(n.unread).toBe(1);
  });

  it("gönderen bilinmeyen (sealed-sender) mesajı sayılır", () => {
    const n = createTabNotifier(fakeEnv());
    expect(n.onIncoming({ id: "s1", type: "text", ownDid: "did:me", plaintext: "x" })).toBe(true);
  });
});

describe("tab-notify: hata izolasyonu (fail-safe)", () => {
  it("ses ve OS bildirimi patlasa da başlık/favicon çalışır ve fırlatmaz", () => {
    const env = fakeEnv({
      playSound: () => {
        throw new Error("AudioContext yok");
      },
      showOsNotification: () => {
        throw new Error("izin yok");
      },
    });
    const n = createTabNotifier(env);
    expect(() => n.onIncoming(msg("m1"))).not.toThrow();
    expect(env.title).toBe("(1) Obscura");
    expect(env.badges.at(-1)).toBe(1);
  });

  it("favicon patlasa da başlık sayacı ve ses çalışır", () => {
    const env = fakeEnv({
      setFaviconBadge: () => {
        throw new Error("canvas yok");
      },
    });
    const n = createTabNotifier(env);
    expect(() => n.onIncoming(msg("m1"))).not.toThrow();
    expect(env.title).toBe("(1) Obscura");
    expect(env.sounds).toBe(1);
  });

  it("isBackground patlarsa bildirmez, fırlatmaz", () => {
    const env = fakeEnv({
      isBackground: () => {
        throw new Error("document yok");
      },
    });
    const n = createTabNotifier(env);
    expect(() => n.onIncoming(msg("m1"))).not.toThrow();
    expect(n.unread).toBe(0);
  });
});

describe("tab-notify: notificationBody (sızıntı önleme)", () => {
  it("metin mesajı 60 karaktere kesilir", () => {
    const out = notificationBody("text", "a".repeat(200));
    expect(Array.from(out)).toHaveLength(61);
    expect(out.endsWith("…")).toBe(true);
  });

  it("decrypt yer tutucusu düz metin gibi gösterilmez", () => {
    expect(notificationBody("text", "\u{1F512} Şifreli mesaj (cozulemedi)")).toBe("Yeni şifreli mesaj");
  });

  it("medya/konum gibi metin olmayan tiplerde URL/JSON gösterilmez", () => {
    expect(notificationBody("image", "https://sunucu/medya/abc.jpg")).toBe("Yeni mesaj");
    expect(notificationBody("location", '{"lat":41,"lng":29}')).toBe("Yeni mesaj");
  });

  it("boş metin için genel şifreli mesaj metni", () => {
    expect(notificationBody("text", "  \n ")).toBe("Yeni şifreli mesaj");
    expect(notificationBody("text", undefined)).toBe("Yeni şifreli mesaj");
  });
});

// OS bildirimi (tauri.ts showNotification): izin yoksa sessizce atlar, çökmez.
describe("tauri.showNotification: izin durumuna göre", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllGlobals();
  });

  async function loadWith(permission: string) {
    const ctor = vi.fn();
    const Notif = Object.assign(ctor, { permission });
    vi.stubGlobal("window", { Notification: Notif });
    vi.stubGlobal("Notification", Notif);
    const mod = await import("./tauri");
    return { ctor, mod };
  }

  it("izin verilmemişse (default) Notification oluşturmaz ve fırlatmaz", async () => {
    const { ctor, mod } = await loadWith("default");
    await expect(mod.showNotification("Yeni mesaj", "x")).resolves.toBeUndefined();
    expect(ctor).not.toHaveBeenCalled();
  });

  it("izin reddedilmişse oluşturmaz ve fırlatmaz", async () => {
    const { ctor, mod } = await loadWith("denied");
    await expect(mod.showNotification("Yeni mesaj", "x")).resolves.toBeUndefined();
    expect(ctor).not.toHaveBeenCalled();
  });

  it("izin verilmişse bildirimi var olan site ikonuyla gösterir", async () => {
    const { ctor, mod } = await loadWith("granted");
    await mod.showNotification("Yeni mesaj", "selam");
    expect(ctor).toHaveBeenCalledWith("Yeni mesaj", { body: "selam", icon: "/logo.jpeg" });
  });
});
