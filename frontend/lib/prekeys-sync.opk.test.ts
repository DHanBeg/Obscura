import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  generateIdentity,
  loadPreKeyStore,
  toB64,
  type IdentityKeys,
  type PreKeyBundle,
} from "./e2ee";
import { api } from "./api";
import {
  ensurePreKeysUploaded,
  syncPreKeys,
  consumeOneTimePreKey,
  signedPreKeyStorageKey,
} from "./prekeys-sync";
import { setActiveAccountDid, initiateSession, encryptForSend, decryptIncoming } from "./e2ee-session";

interface UploadBody {
  identity_key: string;
  signed_prekey: string;
  signed_prekey_sig: string;
  one_time_prekeys: { id: number; public_key: string }[];
}

const passOf = (id: IdentityKeys) => `obscura_${id.did}_v1`;

function mockServerEmpty() {
  const upload = vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
  const replenish = vi.spyOn(api, "replenishOPK").mockResolvedValue({});
  const count = vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("henüz bundle yok"));
  return { upload, replenish, count };
}

describe("prekeys-sync: OPK private saklama + PreKeyStore döndürme", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("PreKeyStore döner; OPK private'ları 32 byte, yüklenen public'lere karşılık gelir ve kalıcı depoda durur", async () => {
    const identity = await generateIdentity();
    const { upload } = mockServerEmpty();

    const store = await ensurePreKeysUploaded(identity, passOf(identity));

    expect(store.identity.did).toBe(identity.did);
    expect(store.oneTimePreKeys).toHaveLength(100);
    for (const opk of store.oneTimePreKeys) expect(opk.keyPair.privateKey).toHaveLength(32);

    const body = upload.mock.calls[0][0] as UploadBody;
    expect(body.signed_prekey).toBe(toB64(store.signedPreKey.publicKeyBytes));
    const uploaded = new Map(body.one_time_prekeys.map((o) => [o.id, o.public_key]));
    for (const opk of store.oneTimePreKeys) {
      expect(uploaded.get(opk.id)).toBe(toB64(opk.keyPair.publicKeyBytes));
    }

    const persisted = await loadPreKeyStore(identity, passOf(identity));
    expect(persisted).not.toBeNull();
    expect(persisted!.oneTimePreKeys.map((o) => o.id).sort()).toEqual(
      store.oneTimePreKeys.map((o) => o.id).sort()
    );
  });

  it("OPK id'leri rastgele 31-bit ve benzersiz (sabit 0-99 değil)", async () => {
    const identity = await generateIdentity();
    mockServerEmpty();

    const store = await ensurePreKeysUploaded(identity, passOf(identity));
    const ids = store.oneTimePreKeys.map((o) => o.id);

    expect(new Set(ids).size).toBe(ids.length);
    expect(ids.some((id) => id > 99)).toBe(true);
    expect(ids.every((id) => id >= 0 && id < 2 ** 31)).toBe(true);
  });

  it("private'lar public yüklemeden ÖNCE kalıcı yazılır: yükleme düşse bile depoda durur", async () => {
    const identity = await generateIdentity();
    vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("henüz bundle yok"));
    vi.spyOn(api, "uploadPrekeys").mockRejectedValue(new Error("ağ hatası"));

    await expect(syncPreKeys(identity, passOf(identity))).rejects.toThrow("ağ hatası");

    const persisted = await loadPreKeyStore(identity, passOf(identity));
    expect(persisted?.oneTimePreKeys).toHaveLength(100);
  });

  it("replenish: mevcut OPK private'ları korunur, yenileri eklenir", async () => {
    const identity = await generateIdentity();
    const pass = passOf(identity);
    mockServerEmpty();
    const first = await ensurePreKeysUploaded(identity, pass);

    vi.spyOn(api, "getOPKCount").mockResolvedValue({ count: 5, low: true, critical: false });
    const replenish = vi.spyOn(api, "replenishOPK").mockResolvedValue({});
    const second = await syncPreKeys(identity, pass);

    expect(second.reason).toBe("replenished");
    expect(replenish).toHaveBeenCalledTimes(1);
    expect(second.store.oneTimePreKeys).toHaveLength(200);
    const firstIds = new Set(first.oneTimePreKeys.map((o) => o.id));
    expect(second.store.oneTimePreKeys.filter((o) => firstIds.has(o.id))).toHaveLength(100);
    // yeni batch'in id'leri eskilerle çakışmaz
    expect(new Set(second.store.oneTimePreKeys.map((o) => o.id)).size).toBe(200);
  });
});

describe("prekeys-sync: hesap-bazlı anahtarlama + in-flight kilidi + ağsız yedek", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("aynı tarayıcıda iki hesap: SPK registry ve OPK deposu birbirini ezmez", async () => {
    const a = await generateIdentity();
    const b = await generateIdentity();
    mockServerEmpty();

    const storeA = await ensurePreKeysUploaded(a, passOf(a));
    const registryABefore = localStorage.getItem(signedPreKeyStorageKey(passOf(a)));
    const storeB = await ensurePreKeysUploaded(b, passOf(b));

    expect(toB64(storeB.signedPreKey.publicKeyBytes)).not.toBe(toB64(storeA.signedPreKey.publicKeyBytes));
    expect(localStorage.getItem(signedPreKeyStorageKey(passOf(a)))).toBe(registryABefore);
    expect(localStorage.getItem(signedPreKeyStorageKey(passOf(b)))).not.toBeNull();

    const persistedA = await loadPreKeyStore(a, passOf(a));
    const persistedB = await loadPreKeyStore(b, passOf(b));
    const idsA = new Set(persistedA!.oneTimePreKeys.map((o) => o.id));
    expect(persistedB!.oneTimePreKeys.some((o) => idsA.has(o.id))).toBe(false);
  });

  it("eşzamanlı iki çağrı (StrictMode/remount) tek yükleme yapar, aynı store'u döner", async () => {
    const identity = await generateIdentity();
    const { upload } = mockServerEmpty();
    const pass = passOf(identity);

    const [s1, s2] = await Promise.all([
      ensurePreKeysUploaded(identity, pass),
      ensurePreKeysUploaded(identity, pass),
    ]);

    expect(upload).toHaveBeenCalledTimes(1);
    expect(toB64(s1.signedPreKey.publicKeyBytes)).toBe(toB64(s2.signedPreKey.publicKeyBytes));
    expect(s1.oneTimePreKeys.map((o) => o.id)).toEqual(s2.oneTimePreKeys.map((o) => o.id));
  });

  it("sunucu senkronu düşerse yerelde depo varsa o döner (decrypt ağa bağımlı değil)", async () => {
    const identity = await generateIdentity();
    const pass = passOf(identity);
    mockServerEmpty();
    const first = await ensurePreKeysUploaded(identity, pass);

    vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("çevrimdışı"));
    vi.spyOn(api, "uploadPrekeys").mockRejectedValue(new Error("çevrimdışı"));
    vi.spyOn(console, "warn").mockImplementation(() => {});
    const offline = await ensurePreKeysUploaded(identity, pass);

    expect(toB64(offline.signedPreKey.publicKeyBytes)).toBe(toB64(first.signedPreKey.publicKeyBytes));
    const offlineIds = new Set(offline.oneTimePreKeys.map((o) => o.id));
    for (const opk of first.oneTimePreKeys) expect(offlineIds.has(opk.id)).toBe(true);
  });

  it("yerel depo yazılamıyorsa (kota) ve kullanılabilir yerel depo yoksa hata yukarı fırlar (sessiz null yok)", async () => {
    const identity = await generateIdentity();
    vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("çevrimdışı"));
    vi.spyOn(api, "uploadPrekeys").mockResolvedValue({});
    vi.spyOn(localStorage, "setItem").mockImplementation(() => {
      throw new Error("kota doldu");
    });
    vi.spyOn(console, "warn").mockImplementation(() => {});

    await expect(ensurePreKeysUploaded(identity, passOf(identity))).rejects.toThrow("kota doldu");
  });

  it("sunucu yokken tekrarlanan açılışlar yerel OPK private'larını sınırsız biriktirmez", async () => {
    const identity = await generateIdentity();
    const pass = passOf(identity);
    vi.spyOn(api, "getOPKCount").mockRejectedValue(new Error("çevrimdışı"));
    vi.spyOn(api, "uploadPrekeys").mockRejectedValue(new Error("çevrimdışı"));
    vi.spyOn(console, "warn").mockImplementation(() => {});

    for (let i = 0; i < 3; i++) await ensurePreKeysUploaded(identity, pass);

    expect((await loadPreKeyStore(identity, pass))!.oneTimePreKeys).toHaveLength(100);
  });
});

describe("prekeys-sync: consumeOneTimePreKey (removeUsedOneTimePreKey bağlantısı)", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("kullanılan OPK kalıcı depodan silinir; bayat bellek-içi store eskiyi geri yazmaz", async () => {
    const identity = await generateIdentity();
    const pass = passOf(identity);
    mockServerEmpty();
    const stale = await ensurePreKeysUploaded(identity, pass);
    const [a, b] = [stale.oneTimePreKeys[0].id, stale.oneTimePreKeys[1].id];

    await consumeOneTimePreKey(stale, a);
    await consumeOneTimePreKey(stale, b); // stale hâlâ a'yı içeriyor

    const ids = (await loadPreKeyStore(identity, pass))!.oneTimePreKeys.map((o) => o.id);
    expect(ids).not.toContain(a);
    expect(ids).not.toContain(b);
    expect(ids).toHaveLength(98);
  });

  it("uçtan uca: Bob'un OPK'si ile başlayan ilk mesaj çözülür, OPK tüketilir; OPK private'ı yoksa çözülemez", async () => {
    const alice = await generateIdentity();
    const bob = await generateIdentity();
    const bobPass = passOf(bob);
    const { upload } = mockServerEmpty();
    const bobStore = await ensurePreKeysUploaded(bob, bobPass);

    // Alice sunucudan Bob'un bundle'ını (bir OPK ile) alır.
    const body = upload.mock.calls[0][0] as UploadBody;
    const opk = body.one_time_prekeys[0];
    const bundle: PreKeyBundle = {
      identity_key: body.identity_key,
      signed_prekey: body.signed_prekey,
      signed_prekey_sig: body.signed_prekey_sig,
      one_time_prekey: opk.public_key,
      one_time_prekey_id: opk.id,
      did: bob.did,
    };
    setActiveAccountDid(alice.did);
    const init = await initiateSession(alice, bundle, "conv-opk");
    const msg = await encryptForSend(init.state, "opk ile ilk mesaj", "conv-opk", init.x3dhInit);
    expect(init.x3dhInit.opkId).toBe(opk.id);

    // Kontrol: OPK private'ı olmayan bir store (eski davranış) 3-DH'ye düşer, çözemez.
    setActiveAccountDid(bob.did);
    vi.spyOn(console, "error").mockImplementation(() => {});
    const withoutOpk = { ...bobStore, oneTimePreKeys: [] };
    const failed = await decryptIncoming("conv-opk", msg.ciphertext, bob, withoutOpk, () => {});
    expect(failed).toContain("cozulemedi");

    // Asıl yol: saklanan OPK private'lı store ile çözülür ve OPK tüketilir.
    localStorage.removeItem(`obscura_session_v1_${bob.did}:conv-opk`);
    const ok = await decryptIncoming("conv-opk", msg.ciphertext, bob, bobStore, () => {});
    expect(ok).toBe("opk ile ilk mesaj");

    const remaining = (await loadPreKeyStore(bob, bobPass))!.oneTimePreKeys.map((o) => o.id);
    expect(remaining).not.toContain(opk.id);
    expect(remaining).toHaveLength(99);
  });
});
