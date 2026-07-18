import { u8ToBase64 } from "../crypto";
import { createMemoryStore } from "../../test-utils/memoryStore";

jest.mock("../api", () => ({
  api: {
    getPreKeyBundle: jest.fn(),
    sendMessage: jest.fn(),
  },
}));

import { api } from "../api";
import { getOrCreateKeyPair, receiveMessage } from "../e2e";
import { didFromDhPublic } from "../sealed-sender";
import { getSigningPublicKeyBase64 } from "../identity";
import { getSignedPreKeyBundle } from "../prekeys";
import { generateBatch } from "../opk";
import { getSenderAttestation } from "../sender-attestation-cache";
import { KeyValueStore } from "../keyValueStore";
import { sendSealedMessage } from "../message-send";

// 11a.1 — message-send.ts henüz yazılmadı (11a.2). Bu dosya, chat/[id].tsx'in
// 6 gönderim call site'ının (text/image/video/file/location/voice) hepsinin
// kullanacağı TEK karar noktasını (sendSealedMessage) test ediyor:
// sealAndEncryptMessage ile zarfla + api.sendMessage'ı encryption_type:"sealed"
// ile çağır. sealed-e2e.test.ts'teki iki-cihaz desenini tekrarlıyor, ama
// e2e.ts'in sarmalayıcılarını değil, message-send.ts'in ürettiği gerçek HTTP
// body'sini ve receiveMessage roundtrip'ini doğruluyor.
const mockedApi = api as unknown as { getPreKeyBundle: jest.Mock; sendMessage: jest.Mock };

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
  const opk = batch[0];
  return {
    did: device.did,
    identity_key: u8ToBase64(identity.publicKey),
    signing_key: signingKey,
    signed_prekey: spk.signedPrekey,
    signed_prekey_sig: spk.signedPrekeySig,
    one_time_prekey: opk.publicKey,
    one_time_prekey_id: opk.id,
  };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("message-send.ts — chat ekranının kullanacağı tek karar noktası", () => {
  test("sendSealedMessage: api.sendMessage'a encryption_type:sealed + base64 (JSON DEĞİL) ciphertext ile çağırır", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));
    mockedApi.sendMessage.mockResolvedValue({ id: "srv-1", conv_id: "conv-1", status: "sent" });

    await sendSealedMessage(bob.did, "merhaba Bob — gerçek chat yolu", "text", alice.stores);

    expect(mockedApi.sendMessage).toHaveBeenCalledTimes(1);
    const body = mockedApi.sendMessage.mock.calls[0][0];
    expect(body.to_id).toBe(bob.did);
    expect(body.type).toBe("text");
    expect(body.encryption_type).toBe("sealed");
    expect(() => JSON.parse(body.ciphertext)).toThrow(); // sealed zarf JSON değil, base64 ikili veri
  });

  test("alice→bob gerçek roundtrip: Bob receiveMessage ile doğru plaintext'i alır ve kriptografik olarak kanıtlanmış senderDid + sertifika imzası saklanır", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));
    mockedApi.sendMessage.mockImplementation(async (body: any) => ({
      id: "srv-2", conv_id: "conv-1", status: "sent", ciphertext: body.ciphertext,
    }));

    await sendSealedMessage(bob.did, "gerçek yoldan giden gizli mesaj", "text", alice.stores);
    const sentBody = mockedApi.sendMessage.mock.calls[0][0];

    // Sunucunun WS/HTTP'de gönderdiği from_did — sealed mesajlarda boş olur
    // (bkz. ADR-0016) — client bunu ASLA kullanmamalı, receiveMessage
    // kendi kriptografik kanıtından çıkarır. Burada boş string simüle ediliyor.
    const plain = await receiveMessage(sentBody.ciphertext, "", bob.stores, "srv-2");
    expect(plain).toBe("gerçek yoldan giden gizli mesaj");

    const attestation = await getSenderAttestation("srv-2", { async: bob.stores.async });
    expect(attestation?.senderDid).toBe(alice.did);
    expect(attestation?.senderCertSignatureHex).toBeTruthy();
  });

  test("extra alanlar (reply_to_id, self_destruct_seconds) api.sendMessage body'sine geçer", async () => {
    const alice = await makeDevice();
    const bob = await makeDevice();
    mockedApi.getPreKeyBundle.mockResolvedValue(await buildBundleResponse(bob));
    mockedApi.sendMessage.mockResolvedValue({ id: "srv-3", conv_id: "conv-1", status: "sent" });

    await sendSealedMessage(bob.did, "yanıt", "text", alice.stores, {
      reply_to_id: "msg-0", self_destruct_seconds: 30,
    });

    const body = mockedApi.sendMessage.mock.calls[0][0];
    expect(body.reply_to_id).toBe("msg-0");
    expect(body.self_destruct_seconds).toBe(30);
  });
});
