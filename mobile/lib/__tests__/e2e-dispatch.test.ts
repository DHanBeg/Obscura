import { u8ToBase64, encryptForRecipient } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

jest.mock("../api", () => ({
  api: {
    getPreKeyBundle: jest.fn(),
  },
}));

import { api } from "../api";
import { encryptMessage, decryptMessage, getOrCreateKeyPair } from "../e2e";
import { generateSigningKeyPair, getSigningPublicKeyBase64 } from "../identity";
import { getSignedPreKeyBundle } from "../prekeys";
import { generateBatch, getOneTimePreKeyPrivate } from "../opk";
import { loadSession } from "../session-store";
import { KeyValueStore } from "../keyValueStore";

const mockedApi = api as unknown as { getPreKeyBundle: jest.Mock };

const FALLBACK = "🔒 Şifresi çözülemeyen mesaj";

// İki simüle cihaz — her birinin kendi izole secure/async store'u var.
interface Device {
  did: string;
  stores: { secure: KeyValueStore; async: KeyValueStore };
}

let deviceCounter = 0;
function makeDevice(name: string): Device {
  // DID'ler test başına benzersiz — e2e.ts'in per-DID kilit haritası modül
  // seviyesinde yaşadığı için testler arası state karışmasın.
  deviceCounter++;
  return {
    did: `did:key:${name}-${deviceCounter}`,
    stores: { secure: createMemoryStore(), async: createMemoryStore() },
  };
}

// Cihazın backend'e yükleyeceği bundle'ın GET /v1/keys/{did} yanıtı biçiminde
// (snake_case, ham obje) karşılığını üretir — backend OPK'yı bir kez dağıtır,
// testte de bundle tek bir OPK taşır.
async function buildBundleResponse(device: Device, opts: { withOpk?: boolean } = {}) {
  const identity = await getOrCreateKeyPair(device.stores.secure);
  const signingKey = await getSigningPublicKeyBase64(device.stores.secure);
  const spk = await getSignedPreKeyBundle(device.stores.secure);
  let opk: { id: number; publicKey: string } | null = null;
  if (opts.withOpk !== false) {
    const batch = await generateBatch(1, device.stores.secure);
    opk = batch[0];
  }
  return {
    did: device.did,
    identity_key: u8ToBase64(identity.publicKey),
    signing_key: signingKey,
    signed_prekey: spk.signedPrekey,
    signed_prekey_sig: spk.signedPrekeySig,
    one_time_prekey: opk ? opk.publicKey : null,
    one_time_prekey_id: opk ? opk.id : null,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("e2e.ts v2 — X3DH + Double Ratchet uçtan uca (iki simüle cihaz)", () => {
  test("(a) oturum yokken ilk mesaj: Alice bundle üzerinden oturum kurar, Bob x3dhAccept ile çözer", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    const bundle = await buildBundleResponse(bob);
    mockedApi.getPreKeyBundle.mockResolvedValue(bundle);

    const raw = await encryptMessage("merhaba Bob 🌙 — ilk temas", bob.did, alice.stores);

    // Zarf şeması: v2 + dolu x3dh (bootstrap), imza alanı YOK (deniability)
    const env = JSON.parse(raw);
    expect(env.v).toBe(2);
    expect(typeof env.msg).toBe("string");
    expect(typeof env.x3dh.ik).toBe("string");
    expect(typeof env.x3dh.ek).toBe("string");
    expect(env.x3dh.spk).toBe(bundle.signed_prekey);
    expect(env.x3dh.opkId).toBe(bundle.one_time_prekey_id);
    expect(env.x3dh.sig).toBeUndefined();
    expect(mockedApi.getPreKeyBundle).toHaveBeenCalledWith(bob.did);

    // Bob ilk kez alıyor — kendi SPK/OPK private'larıyla çözer
    const plain = await decryptMessage(raw, alice.did, bob.stores);
    expect(plain).toBe("merhaba Bob 🌙 — ilk temas");

    // İki tarafta da oturum kalıcılaşmış olmalı
    expect(await loadSession(bob.did, alice.stores)).not.toBeNull();
    expect(await loadSession(alice.did, bob.stores)).not.toBeNull();
  });

  test("(b) Bob cevap verir: ekstra adım gerekmeden ratchet iki yönde kurulu, Alice çözer", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const m1 = await encryptMessage("selam", bob.did, alice.stores);
    expect(await decryptMessage(m1, alice.did, bob.stores)).toBe("selam");

    // Bob'un cevabı: oturum zaten var → bundle FETCH EDİLMEZ, x3dh alanı YOK
    const reply = await encryptMessage("selam Alice, geldim", alice.did, bob.stores);
    expect(JSON.parse(reply).x3dh).toBeUndefined();
    expect(mockedApi.getPreKeyBundle).toHaveBeenCalledTimes(1);

    expect(await decryptMessage(reply, bob.did, alice.stores)).toBe("selam Alice, geldim");
  });

  test("(c) çok turlu iki yönlü konuşma — DH ratchet ileri sarar, her mesaj doğru çözülür", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    for (let round = 0; round < 4; round++) {
      const aMsg = `alice → bob, tur ${round}`;
      expect(
        await decryptMessage(await encryptMessage(aMsg, bob.did, alice.stores), alice.did, bob.stores)
      ).toBe(aMsg);

      const bMsg = `bob → alice, tur ${round}`;
      expect(
        await decryptMessage(await encryptMessage(bMsg, alice.did, bob.stores), bob.did, alice.stores)
      ).toBe(bMsg);
    }
    // Tüm konuşma tek bundle fetch'iyle döndü (oturum kalıcı)
    expect(mockedApi.getPreKeyBundle).toHaveBeenCalledTimes(1);
  });

  test("bootstrap, ilk ratchet cevabı alınana kadar HER mesajda taşınır; cevap gelince düşer", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const m1 = await encryptMessage("bir", bob.did, alice.stores);
    const m2 = await encryptMessage("iki", bob.did, alice.stores);
    // Cevap gelmeden ikinci mesaj da AYNI bootstrap'ı taşımalı (ilk mesaj
    // kaybolursa m2 tek başına oturumu kurabilsin — Signal PreKey davranışı)
    expect(JSON.parse(m2).x3dh).toEqual(JSON.parse(m1).x3dh);

    // Bob SADECE m2'yi alır (m1 kayıp/gecikmiş) — bootstrap'tan oturumu kurar
    expect(await decryptMessage(m2, alice.did, bob.stores)).toBe("iki");
    // m1 geç gelir — skipped-key ile yine çözülür
    expect(await decryptMessage(m1, alice.did, bob.stores)).toBe("bir");

    // Bob cevap verir, Alice çözer → Alice'in pending bootstrap'ı temizlenir
    const reply = await encryptMessage("üç", alice.did, bob.stores);
    expect(await decryptMessage(reply, bob.did, alice.stores)).toBe("üç");
    const m4 = await encryptMessage("dört", bob.did, alice.stores);
    expect(JSON.parse(m4).x3dh).toBeUndefined();
    expect(await decryptMessage(m4, alice.did, bob.stores)).toBe("dört");
  });

  test("(d) SPK imzası yanlış signing_key ile doğrulanamıyorsa oturum KURULMAZ, hata fırlatılır", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    const bundle = await buildBundleResponse(bob);
    const mallory = generateSigningKeyPair();
    mockedApi.getPreKeyBundle.mockResolvedValue({
      ...bundle,
      signing_key: u8ToBase64(mallory.publicKey), // MITM: imza artık doğrulanamaz
    });

    await expect(encryptMessage("tuzağa düşme", bob.did, alice.stores)).rejects.toThrow(
      /doğrulanamadı/
    );
    // Sessizce devam edilmedi: oturum yok, sonraki deneme yine bundle'a gider
    expect(await loadSession(bob.did, alice.stores)).toBeNull();
  });

  test("(d2) signing_key alanı hiç yoksa da oturum kurulmaz (imzasız bundle kabul edilmez)", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    const bundle = await buildBundleResponse(bob);
    mockedApi.getPreKeyBundle.mockResolvedValue({ ...bundle, signing_key: undefined });

    await expect(encryptMessage("x", bob.did, alice.stores)).rejects.toThrow(/doğrulanamadı/);
    expect(await loadSession(bob.did, alice.stores)).toBeNull();
  });

  test("OPK'sız bundle (one_time_prekey: null) ile de oturum kurulur — DH4 atlanır", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob, { withOpk: false }));

    const raw = await encryptMessage("opk yok ama olsun", bob.did, alice.stores);
    expect(JSON.parse(raw).x3dh.opkId).toBeUndefined();
    expect(await decryptMessage(raw, alice.did, bob.stores)).toBe("opk yok ama olsun");
  });

  test("tüketilen OPK private'ı başarılı accept sonrası silinir (one-time garantisi)", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    const bundle = await buildBundleResponse(bob);
    mockedApi.getPreKeyBundle.mockResolvedValue(bundle);

    const raw = await encryptMessage("tek kullanımlık", bob.did, alice.stores);
    expect(await getOneTimePreKeyPrivate(bundle.one_time_prekey_id!, bob.stores.secure)).not.toBeNull();
    await decryptMessage(raw, alice.did, bob.stores);
    expect(await getOneTimePreKeyPrivate(bundle.one_time_prekey_id!, bob.stores.secure)).toBeNull();
  });

  test("AD gerçekten AEAD'e bağlı: saklanan peer identity key bozulursa decrypt başarısız olur", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    // Oturum kur
    const m1 = await encryptMessage("kur", bob.did, alice.stores);
    await decryptMessage(m1, alice.did, bob.stores);
    const reply1 = await encryptMessage("cevap 1", alice.did, bob.stores);
    expect(await decryptMessage(reply1, bob.did, alice.stores)).toBe("cevap 1");

    // Alice'in sakladığı Bob IK'sını boz → AD değişir → AEAD tag tutmaz
    const wrongIk = u8ToBase64(new Uint8Array(32).fill(7));
    await alice.stores.async.setItem(`obscura_e2e_peer_ik_${bob.did}`, wrongIk);
    const reply2 = await encryptMessage("cevap 2", alice.did, bob.stores);
    expect(await decryptMessage(reply2, bob.did, alice.stores)).toBe(FALLBACK);
  });

  test("v2 zarf: oturum da bootstrap da yoksa çözülemez fallback döner", async () => {
    const stranger = makeDevice("stranger");
    const orphan = JSON.stringify({ v: 2, msg: u8ToBase64(new Uint8Array(80).fill(1)) });
    expect(await decryptMessage(orphan, "did:key:unknown-peer", stranger.stores)).toBe(FALLBACK);
  });

  test("forward secrecy + önbellek: aynı v2 ciphertext ikinci kez çözülemez, messageId verilirse önbellekten döner", async () => {
    const alice = makeDevice("alice");
    const bob = makeDevice("bob");
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const raw = await encryptMessage("bir kere okunur", bob.did, alice.stores);

    // messageId İLE: ilk çözüm gerçek, ikincisi önbellekten — ikisi de düz metin
    expect(await decryptMessage(raw, alice.did, bob.stores, "msg-1")).toBe("bir kere okunur");
    expect(await decryptMessage(raw, alice.did, bob.stores, "msg-1")).toBe("bir kere okunur");

    // messageId OLMADAN üçüncü deneme: mesaj anahtarı tüketildi → fallback
    // (bu, ratchet anahtarlarının tek kullanımlık olduğunun — forward
    // secrecy'nin — ve önbelleğin neden zorunlu olduğunun kanıtı)
    expect(await decryptMessage(raw, alice.did, bob.stores)).toBe(FALLBACK);
  });
});

describe("e2e.ts — v1 GERİYE UYUMLULUK (bu testlerin FAIL olması kabul edilemez)", () => {
  test("(e) elle üretilmiş eski v1 ECIES zarfı hâlâ kendi identity anahtarıyla çözülüyor", async () => {
    const dave = makeDevice("dave");
    const { publicKey } = await getOrCreateKeyPair(dave.stores.secure);

    // Eski istemcinin encryptForRecipient ile ürettiği v1 zarf
    const legacyEnvelope = encryptForRecipient("eski dünyadan bir mesaj", publicKey);
    expect(legacyEnvelope.v).toBe(1);

    const plain = await decryptMessage(JSON.stringify(legacyEnvelope), "did:key:old-friend", dave.stores);
    expect(plain).toBe("eski dünyadan bir mesaj");
  });

  test("parse edilemeyen içerik plaintext fallback sayılır (legacy davranış korunuyor)", async () => {
    const dave = makeDevice("dave");
    expect(await decryptMessage("düz metin — zarf değil", "did:key:x", dave.stores)).toBe(
      "düz metin — zarf değil"
    );
    expect(await decryptMessage('{"hello":"world"}', "did:key:x", dave.stores)).toBe(
      '{"hello":"world"}'
    );
  });

  test("başkasının anahtarına şifrelenmiş v1 zarf çözülemez fallback'e düşer", async () => {
    const dave = makeDevice("dave");
    const eve = makeDevice("eve");
    const { publicKey: evePub } = await getOrCreateKeyPair(eve.stores.secure);

    const notForDave = encryptForRecipient("dave okuyamaz", evePub);
    expect(await decryptMessage(JSON.stringify(notForDave), "did:key:eve", dave.stores)).toBe(FALLBACK);
  });
});
