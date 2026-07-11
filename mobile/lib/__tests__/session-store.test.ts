import { x25519 } from "@noble/curves/ed25519.js";
import { RatchetState } from "../ratchet";
import { saveSession, loadSession, deleteSession } from "../session-store";
import { createMemoryStore } from "../../test-utils/memoryStore";

function makeSession() {
  const sharedKey = new Uint8Array(32).fill(0x55);
  const bobRatchet = x25519.keygen();
  const alice = RatchetState.initSender(sharedKey, bobRatchet.publicKey);
  const bob = RatchetState.initReceiver(sharedKey, bobRatchet.secretKey);
  return { alice, bob };
}

describe("session-store.ts — kaydet → yükle → devam et", () => {
  test("yüklenen state'ten devam edilen mesajlar gerçek bir Bob peer'ı tarafından kesintisiz çözülüyor", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("persist-conv");

    // Alice 2 mesaj gönderir, Bob ikisini de alır (gerçek zamanlı)
    const m1 = alice.encrypt(new TextEncoder().encode("once upon a time"), ad);
    expect(new TextDecoder().decode(bob.decrypt(m1, ad))).toBe("once upon a time");
    const m2 = alice.encrypt(new TextEncoder().encode("chapter two"), ad);
    expect(new TextDecoder().decode(bob.decrypt(m2, ad))).toBe("chapter two");

    // Alice'in oturumu kaydedilir, in-memory referans "kaybedilir" (uygulama kapanmış gibi)
    await saveSession("peer-bob", alice, stores);

    // Taze bir RatchetState yüklenir — orijinal `alice` objesi bir daha KULLANILMAZ
    const aliceReloaded = await loadSession("peer-bob", stores);
    expect(aliceReloaded).not.toBeNull();

    // Yüklenen state'ten devam: yeni mesaj gönder
    const m3 = aliceReloaded!.encrypt(new TextEncoder().encode("chapter three"), ad);
    // Bob (hiç kesintiye uğramamış gerçek peer) bunu SORUNSUZ çözebilmeli —
    // bu, yüklenen state'in ns/cks'inin tam doğru yerden devam ettiğinin kanıtı
    expect(new TextDecoder().decode(bob.decrypt(m3, ad))).toBe("chapter three");
    expect(m3.header.n).toBe(2); // m1=n0, m2=n1, m3=n2 — sayaç kesintisiz
  });

  test("DH ratchet SONRASI kaydedilen state doğru yükleniyor (yeni dhs_pub/dhs_priv korunuyor)", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("dh-persist-conv");

    const a0 = alice.encrypt(new TextEncoder().encode("a0"), ad);
    bob.decrypt(a0, ad);
    const b0 = bob.encrypt(new TextEncoder().encode("b0"), ad); // Bob cevap verir → Alice'te DH ratchet tetiklenecek
    alice.decrypt(b0, ad);

    await saveSession("peer-bob-dh", alice, stores);
    const aliceReloaded = await loadSession("peer-bob-dh", stores);

    const a1 = aliceReloaded!.encrypt(new TextEncoder().encode("a1-after-reload"), ad);
    expect(new TextDecoder().decode(bob.decrypt(a1, ad))).toBe("a1-after-reload");
  });

  test("skipped (out-of-order) anahtarlar da korunuyor — yüklendikten sonra geç gelen mesaj çözülebiliyor", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("skip-persist-conv");

    const m0 = alice.encrypt(new TextEncoder().encode("m0"), ad);
    const m1 = alice.encrypt(new TextEncoder().encode("m1"), ad);

    // Bob m1'i alır, m0 atlanır (skipped buffer'a düşer)
    bob.decrypt(m1, ad);
    expect(bob.skippedCount).toBe(1);

    await saveSession("peer-bob-skip", bob, stores);
    const bobReloaded = await loadSession("peer-bob-skip", stores);
    expect(bobReloaded!.skippedCount).toBe(1);

    // m0 GEÇ gelir — yüklenen state'in skipped buffer'ından çözülebilmeli
    expect(new TextDecoder().decode(bobReloaded!.decrypt(m0, ad))).toBe("m0");
  });
});

describe("session-store.ts — skip buffer disk kırpma (~2KB)", () => {
  test("çok sayıda skipped key varsa saklanan blob küçük tutuluyor, en YENİ kayıtlar korunuyor", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice, bob } = makeSession();
    const ad = new TextEncoder().encode("trim-conv");

    // 50 mesaj üret, Bob hiçbirini almasın (hepsi "skipped" olacak son mesaj gelince)
    const msgs = [];
    for (let i = 0; i < 50; i++) {
      msgs.push(alice.encrypt(new TextEncoder().encode(`t${i}`), ad));
    }
    // Sadece SONUNCUYU teslim et — Bob 0..48'i skipped buffer'a yazar (49 tanesi)
    bob.decrypt(msgs[49], ad);
    expect(bob.skippedCount).toBe(49);

    await saveSession("peer-bob-trim", bob, stores);

    // Ham (şifreli) blob boyutu makul kalmalı — 49 tam kayıt (~150 byte/kayıt
    // ~7350 byte) DEĞİL, ~2KB sınırına yakın olmalı (hex-encoded + AES overhead)
    const rawBlobHex = await stores.async.getItem("obscura_ratchet_session_peer-bob-trim");
    expect(rawBlobHex).not.toBeNull();
    const blobByteLen = rawBlobHex!.length / 2;
    expect(blobByteLen).toBeLessThan(3000); // 2KB skip payload + header/overhead için gevşek üst sınır

    // En YENİ skipped mesaj (n=48, en olası geç-gelecek olan) hâlâ kurtarılabilir olmalı
    const bobReloaded = await loadSession("peer-bob-trim", stores);
    expect(new TextDecoder().decode(bobReloaded!.decrypt(msgs[48], ad))).toBe("t48");
  });
});

describe("session-store.ts — master key ve şifreleme", () => {
  test("aynı store çifti iki save çağrısında AYNI master key'i yeniden kullanıyor", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice: a1 } = makeSession();
    const { alice: a2 } = makeSession();

    await saveSession("s1", a1, stores);
    const masterKeyAfterFirst = await stores.secure.getItem("obscura_ratchet_master_key");
    await saveSession("s2", a2, stores);
    const masterKeyAfterSecond = await stores.secure.getItem("obscura_ratchet_master_key");

    expect(masterKeyAfterFirst).not.toBeNull();
    expect(masterKeyAfterSecond).toBe(masterKeyAfterFirst);
  });

  test("AsyncStorage'daki ham blob plaintext session verisi İÇERMİYOR (şifreli)", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice } = makeSession();
    await saveSession("secrecy-check", alice, stores);

    const rawHex = await stores.async.getItem("obscura_ratchet_session_secrecy-check");
    expect(rawHex).not.toBeNull();

    // Serialize edilmiş state'in rk/cks hex'lerinin AÇIK haliyle blob'da
    // görünmemesi gerekir — şifreliyse rastgele baytlar olur, tanınabilir alt-dize kalmaz.
    const serialized = alice.serialize();
    expect(rawHex).not.toContain(serialized.rk);
    if (serialized.cks) expect(rawHex).not.toContain(serialized.cks);
    expect(rawHex).not.toContain(JSON.stringify(serialized).slice(2, 20)); // JSON parça arama
  });

  test("yanlış master key ile decrypt başarısız olur (bütünlük)", async () => {
    const storesA = { secure: createMemoryStore(), async: createMemoryStore() };
    const storesB = { secure: createMemoryStore(), async: storesA.async }; // farklı secure store → farklı master key, AYNI blob

    const { alice } = makeSession();
    await saveSession("wrong-key-conv", alice, storesA);

    await expect(loadSession("wrong-key-conv", storesB)).rejects.toThrow();
  });
});

describe("session-store.ts — silme ve eksik kayıt", () => {
  test("var olmayan sessionId için loadSession null döner", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const result = await loadSession("never-saved", stores);
    expect(result).toBeNull();
  });

  test("deleteSession sonrası loadSession null döner", async () => {
    const stores = { secure: createMemoryStore(), async: createMemoryStore() };
    const { alice } = makeSession();
    await saveSession("to-delete", alice, stores);
    expect(await loadSession("to-delete", stores)).not.toBeNull();

    await deleteSession("to-delete", stores);
    expect(await loadSession("to-delete", stores)).toBeNull();
  });
});
