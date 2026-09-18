import { describe, it, expect, vi, beforeEach } from "vitest";
import { generateIdentity } from "./e2ee";
import { api } from "./api";
import { ensurePreKeysUploaded, SIGNED_PREKEY_STORAGE_KEY } from "./prekeys-sync";

describe("web prekey yüklemesi — tek kaynak + idempotent (login/page.tsx ile AppShell ortak yol)", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("ardışık iki çağrı: SPK bir kez üretilip yüklenir, ikincisinde SPK'ya dokunulmadan sadece eksik OPK tamamlanır", async () => {
    const identity = await generateIdentity();

    const uploadSpy = vi
      .spyOn(api, "uploadPrekeys")
      .mockResolvedValue({ uploaded: true, opk_count: 100 });
    const replenishSpy = vi
      .spyOn(api, "replenishOPK")
      .mockResolvedValue({ added: 30, remaining: 35 });
    vi.spyOn(api, "getOPKCount")
      .mockRejectedValueOnce(new Error("henüz bundle yok"))
      .mockResolvedValueOnce({ count: 5, low: true, critical: false });

    const first = await ensurePreKeysUploaded(identity);
    const second = await ensurePreKeysUploaded(identity);

    // SPK içeren tam bundle SADECE ilk çağrıda gider — ikinci çağrı SPK'yı
    // yeniden üretip üstüne yazmaz, sadece eksik OPK'yı tamamlar.
    expect(uploadSpy).toHaveBeenCalledTimes(1);
    expect(replenishSpy).toHaveBeenCalledTimes(1);
    expect(first.reason).toBe("initial");
    expect(second.reason).toBe("replenished");

    const uploadedBody = uploadSpy.mock.calls[0][0] as { signed_prekey: string };
    const registry = JSON.parse(localStorage.getItem(SIGNED_PREKEY_STORAGE_KEY)!) as {
      pub: string;
      retiredAt: number | null;
    }[];
    expect(registry).toHaveLength(1);
    expect(registry[0].retiredAt).toBeNull();
    expect(registry[0].pub).toBe(uploadedBody.signed_prekey);
  });

  it("SPK zaten yüklenmişken OPK seti watermark üstündeyse çağrı hiçbir şey üretmez/yüklemez", async () => {
    const identity = await generateIdentity();
    const uploadSpy = vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
    const replenishSpy = vi.spyOn(api, "replenishOPK").mockResolvedValue({});
    const countSpy = vi
      .spyOn(api, "getOPKCount")
      .mockResolvedValueOnce({ count: 0, low: true, critical: true })
      .mockResolvedValue({ count: 50, low: false, critical: false });

    await ensurePreKeysUploaded(identity); // ilk yükleme: SPK yüklendi olarak işaretlenir
    uploadSpy.mockClear();
    replenishSpy.mockClear();

    const result = await ensurePreKeysUploaded(identity);

    expect(countSpy).toHaveBeenCalledTimes(2);
    expect(uploadSpy).not.toHaveBeenCalled();
    expect(replenishSpy).not.toHaveBeenCalled();
    expect(result.reason).toBe("sufficient");
  });

  // Gerçek backend bundle'ı hiç olmayan kullanıcıda getOPKCount'a HATA değil
  // 200 + count:0 döner (keys.go HandleGetOPKCount). Eskiden bu "bundle var,
  // OPK azalmış" sanılıp sadece replenish çağrılıyor, SPK/identity hiç
  // yazılmıyordu (gerçek-backend testi yakaladı).
  it("count:0 (hata değil) + SPK hiç yüklenmemiş → tam bundle yüklenir, replenish değil", async () => {
    const identity = await generateIdentity();
    const uploadSpy = vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
    const replenishSpy = vi.spyOn(api, "replenishOPK").mockResolvedValue({});
    vi.spyOn(api, "getOPKCount").mockResolvedValue({ count: 0, low: true, critical: true });

    const result = await ensurePreKeysUploaded(identity);

    expect(result.reason).toBe("initial");
    expect(uploadSpy).toHaveBeenCalledTimes(1);
    expect(replenishSpy).not.toHaveBeenCalled();
  });

  it("bozuk aktif SPK kaydı SİLİNMEZ — retiredAt alıp registry'de kalır, yeni SPK ayrı kayıt olarak eklenir", async () => {
    const identity = await generateIdentity();
    vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
    vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("henüz bundle yok"));

    localStorage.setItem(
      SIGNED_PREKEY_STORAGE_KEY,
      JSON.stringify([
        {
          pub: "corrupt-pub",
          priv: "not-valid-pkcs8-base64!!",
          sig: "corrupt-sig",
          createdAt: Date.now() - 1000,
          retiredAt: null,
        },
      ])
    );

    await ensurePreKeysUploaded(identity);

    const registry = JSON.parse(localStorage.getItem(SIGNED_PREKEY_STORAGE_KEY)!) as {
      pub: string;
      retiredAt: number | null;
    }[];
    expect(registry).toHaveLength(2);
    const corrupted = registry.find((e) => e.pub === "corrupt-pub")!;
    expect(corrupted.retiredAt).not.toBeNull();
    const active = registry.find((e) => e.retiredAt === null)!;
    expect(active).toBeDefined();
    expect(active.pub).not.toBe("corrupt-pub");
  });

  it("30 günden eski retired SPK kaydı budanır (mobile RETENTION_MS ile aynı eşik)", async () => {
    const identity = await generateIdentity();
    vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
    // Aktif kayıt geçerli olduğu için getOPKCount'un sonucu bu testte önemsiz —
    // yeterli görünsün diye sufficient veriyoruz, odak sadece prune.
    vi.spyOn(api, "getOPKCount").mockResolvedValue({ count: 50, low: false, critical: false });

    // Önce gerçek bir aktif SPK üret (fresh keypair, import edilebilir).
    await ensurePreKeysUploaded(identity);
    const afterFirst = JSON.parse(localStorage.getItem(SIGNED_PREKEY_STORAGE_KEY)!) as Array<
      Record<string, unknown>
    >;

    // 31 gün önce retire olmuş EK bir kayıt enjekte et.
    const THIRTY_ONE_DAYS_MS = 31 * 24 * 60 * 60 * 1000;
    afterFirst.push({
      pub: "expired-retired-pub",
      priv: "irrelevant",
      sig: "irrelevant",
      createdAt: Date.now() - THIRTY_ONE_DAYS_MS - 1000,
      retiredAt: Date.now() - THIRTY_ONE_DAYS_MS,
    });
    localStorage.setItem(SIGNED_PREKEY_STORAGE_KEY, JSON.stringify(afterFirst));

    await ensurePreKeysUploaded(identity);

    const finalRegistry = JSON.parse(localStorage.getItem(SIGNED_PREKEY_STORAGE_KEY)!) as {
      pub: string;
      retiredAt: number | null;
    }[];
    expect(finalRegistry.find((e) => e.pub === "expired-retired-pub")).toBeUndefined();
  });
});
