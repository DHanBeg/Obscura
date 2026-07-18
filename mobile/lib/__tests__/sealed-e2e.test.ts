import { u8ToBase64 } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

jest.mock("../api", () => ({
  api: {
    getPreKeyBundle: jest.fn(),
  },
}));

import { api } from "../api";
import { encryptMessage, decryptMessage, sealAndEncryptMessage, receiveMessage, getOrCreateKeyPair } from "../e2e";
import { didFromDhPublic } from "../sealed-sender";
import { getSigningPublicKeyBase64 } from "../identity";
import { getSignedPreKeyBundle } from "../prekeys";
import { generateBatch } from "../opk";
import { getSenderAttestation } from "../sender-attestation-cache";
import { KeyValueStore } from "../keyValueStore";

// Madde 15, Adım 4 — sealed-sender'ın X3DH/Ratchet üstüne oturması. Bu dosya
// e2e-dispatch.test.ts'in İKİ CİHAZ desenini birebir tekrarlıyor (o dosyaya
// DOKUNULMADI — X3DH/ratchet regresyon kilidi olarak öylece kalıyor), sadece
// yeni sealAndEncryptMessage/receiveMessage sarmalayıcılarını sınıyor.
//
// ÖNEMLİ FARK: e2e-dispatch.test.ts'teki `did:key:isim-N` gibi RASTGELE
// placeholder DID'ler burada KULLANILAMAZ — receiveMessage() içeride
// decryptMessage()'ı server'ın söylediği DEĞİL, Unseal()'ın sertifikadan
// kriptografik olarak türettiği DID ile çağırıyor (opened.senderDid), ve bu
// DID gerçek sistemde HER ZAMAN didFromDhPublic(identityPublicKey) ile
// eşleşir (bkz. identity.rs::did() ile aynı şema). Test cihazlarının DID'i
// de bu yüzden gerçek türetimden gelmeli — aksi halde peerIkStoreKey/
// session-store anahtarları (senderDid'e göre) tutarsızlaşır.
const mockedApi = api as unknown as { getPreKeyBundle: jest.Mock };
const FALLBACK = "🔒 Şifresi çözülemeyen mesaj";

interface Device {
  did: string;
  stores: { secure: KeyValueStore; async: KeyValueStore };
}

async function makeDevice(): Promise<Device> {
  const stores = { secure: createMemoryStore(), async: createMemoryStore() };
  const identity = await getOrCreateKeyPair(stores.secure);
  return { did: didFromDhPublic(identity.publicKey), stores };
}

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

describe("sealed-sender + X3DH/ratchet uçtan uca (iki simüle cihaz)", () => {
  test("ilk temas: Alice sealAndEncryptMessage ile zarflar, Bob receiveMessage ile açar ve doğru düz metni alır", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const sealed = await sealAndEncryptMessage("merhaba Bob 🌙 — sealed ilk temas", bob.did, alice.stores);

    // Zarf JSON DEĞİL (base64 ikili veri) — legacy dispatch'e YANLIŞLIKLA düşmediğini doğrula.
    expect(() => JSON.parse(sealed)).toThrow();

    const plain = await receiveMessage(sealed, alice.did, bob.stores, "msg-1");
    expect(plain).toBe("merhaba Bob 🌙 — sealed ilk temas");

    // Kriptografik olarak kanıtlanmış gönderen kimliği saklanmış olmalı (madde 4/8 için).
    const attestation = await getSenderAttestation("msg-1", { async: bob.stores.async });
    expect(attestation?.senderDid).toBe(alice.did);
  });

  test("Bob cevap verir (oturum kurulu): sealed roundtrip iki yönde de çalışıyor", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const m1 = await sealAndEncryptMessage("selam", bob.did, alice.stores);
    expect(await receiveMessage(m1, alice.did, bob.stores)).toBe("selam");

    const reply = await sealAndEncryptMessage("selam Alice, geldim", alice.did, bob.stores);
    expect(await receiveMessage(reply, bob.did, alice.stores)).toBe("selam Alice, geldim");
  });

  test("geriye uyumluluk: ESKİ (zarfsız) encryptMessage çıktısı receiveMessage ile HÂLÂ açılıyor", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    // Eski akış: sealAndEncryptMessage YOK, düz encryptMessage — sealed-sender
    // rollout ÖNCESİ gönderilmiş bir mesajı simüle eder.
    const legacyRaw = await encryptMessage("eski (zarfsız) mesaj", bob.did, alice.stores);
    expect(() => JSON.parse(legacyRaw)).not.toThrow(); // hâlâ düz JSON

    // receiveMessage, JSON'ı tanıyıp Unseal'a hiç girmeden decryptMessage'a düşmeli.
    const plain = await receiveMessage(legacyRaw, alice.did, bob.stores);
    expect(plain).toBe("eski (zarfsız) mesaj");
  });

  test("geriye uyumluluk kontrolü: AYNI eski zarf, receiveMessage ve decryptMessage'dan AYNI sonucu veriyor", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    // messageId ile: ilk çağrı (receiveMessage → decryptMessage) ratchet
    // anahtarını tüketir ve plaintext-cache'e yazar; ikinci çağrı (decryptMessage
    // doğrudan) AYNI messageId ile önbellekten okur — tek kullanımlık ratchet
    // anahtarı ikinci kez tüketilmeye ÇALIŞILMAZ (bkz. plaintext-cache.ts).
    const legacyRaw = await encryptMessage("kontrol mesajı", bob.did, alice.stores);
    const viaReceive = await receiveMessage(legacyRaw, alice.did, bob.stores, "ctrl-1");
    const viaDecrypt = await decryptMessage(legacyRaw, alice.did, bob.stores, "ctrl-1");

    expect(viaReceive).toBe("kontrol mesajı");
    expect(viaReceive).toBe(viaDecrypt);
  });

  test("YANLIŞ alıcıda receiveMessage sealed zarfı açamaz — fallback döner, throw ETMEZ", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    const eve = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const sealed = await sealAndEncryptMessage("gizli mesaj", bob.did, alice.stores);

    const result = await receiveMessage(sealed, alice.did, eve.stores);
    expect(result).toBe(FALLBACK);
  });

  test("bozulmuş sealed zarf → fallback döner, throw ETMEZ", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    const sealed = await sealAndEncryptMessage("mesaj", bob.did, alice.stores);
    const bytes = Buffer.from(sealed, "base64");
    bytes[0] = 99; // versiyon bozuldu
    const tampered = bytes.toString("base64");

    expect(await receiveMessage(tampered, alice.did, bob.stores)).toBe(FALLBACK);
  });
});
