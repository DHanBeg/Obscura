import { u8ToBase64 } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

jest.mock("../api", () => ({
  api: {
    getPreKeyBundle: jest.fn(),
  },
}));

import { api } from "../api";
import {
  sealAndEncryptMessage,
  receiveMessage,
  sealReadReceipt,
  parseReadReceiptPayload,
  getOrCreateKeyPair,
} from "../e2e";
import { didFromDhPublic } from "../sealed-sender";
import { getSigningPublicKeyBase64 } from "../identity";
import { getSignedPreKeyBundle } from "../prekeys";
import { generateBatch } from "../opk";
import { KeyValueStore } from "../keyValueStore";

// Madde 15, Adım 6b — okundu-bilgisi sealed mesajlarda sunucu-taraflı
// SendReadReceipt yerine ALICININ GÖNDERDİĞİ ayrı bir sealed mesaj
// (type: "read_receipt") olarak modelleniyor. Bu dosya mobile tarafını
// (sealReadReceipt/parseReadReceiptPayload) sealed-e2e.test.ts'in İKİ CİHAZ
// desenini kullanarak sınıyor — backend tarafı sealed_read_receipt_test.go'da.

const mockedApi = api as unknown as { getPreKeyBundle: jest.Mock };

interface Device {
  did: string;
  stores: { secure: KeyValueStore; async: KeyValueStore };
}

async function makeDevice(): Promise<Device> {
  const stores = { secure: createMemoryStore(), async: createMemoryStore() };
  const identity = await getOrCreateKeyPair(stores.secure);
  return { did: didFromDhPublic(identity.publicKey), stores };
}

async function buildBundleResponse(device: Device) {
  const identity = await getOrCreateKeyPair(device.stores.secure);
  const signingKey = await getSigningPublicKeyBase64(device.stores.secure);
  const spk = await getSignedPreKeyBundle(device.stores.secure);
  const batch = await generateBatch(1, device.stores.secure);
  return {
    did: device.did,
    identity_key: u8ToBase64(identity.publicKey),
    signing_key: signingKey,
    signed_prekey: spk.signedPrekey,
    signed_prekey_sig: spk.signedPrekeySig,
    one_time_prekey: batch[0].publicKey,
    one_time_prekey_id: batch[0].id,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("sealReadReceipt / parseReadReceiptPayload (Adım 6b)", () => {
  test("Bob, Alice'in mesajını okuyunca Alice'e sealed read_receipt gönderir; Alice onu doğru msgId ile çözer", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));

    // 1) Alice → Bob normal sealed mesaj (oturum kurulur, peer IK kalıcılaşır).
    const sealedMsg = await sealAndEncryptMessage("merhaba Bob", bob.did, alice.stores);
    const plain = await receiveMessage(sealedMsg, alice.did, bob.stores, "msg-1");
    expect(plain).toBe("merhaba Bob");

    // 2) Bob → Alice sealed read_receipt — Bob'un Alice'e zaten kurulu oturumu
    // var (adım 1'de kuruldu), bu yüzden sealReadReceipt (sealAndEncryptMessage
    // sarmalayıcısı) sorunsuz çalışmalı.
    const receiptEnvelope = await sealReadReceipt("msg-1", alice.did, bob.stores);
    expect(() => JSON.parse(receiptEnvelope)).toThrow(); // sealed — JSON değil

    // 3) Alice tarafında receiveMessage ile aç, payload'ı parse et.
    const receiptPlain = await receiveMessage(receiptEnvelope, bob.did, alice.stores);
    const parsed = parseReadReceiptPayload(receiptPlain);

    expect(parsed).not.toBeNull();
    expect(parsed?.msgId).toBe("msg-1");
  });

  test("parseReadReceiptPayload: geçersiz/alakasız JSON için null döner", () => {
    expect(parseReadReceiptPayload("bu bir mesaj metni")).toBeNull();
    expect(parseReadReceiptPayload(JSON.stringify({ foo: "bar" }))).toBeNull();
    expect(parseReadReceiptPayload("{bozuk json")).toBeNull();
  });
});
