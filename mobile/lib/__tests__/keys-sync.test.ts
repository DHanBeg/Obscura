import { base64ToU8 } from "../crypto";
import { verifyWithKey } from "../identity";
import { createMemoryStore } from "../../test-utils/memoryStore";

jest.mock("../api", () => ({
  api: {
    getOPKCount: jest.fn(),
    uploadPrekeys: jest.fn(),
    replenishOPK: jest.fn(),
  },
}));

import { api } from "../api";
import { ensureKeyBundleUploaded, replenishOPKsIfLow } from "../keys-sync";

const mockedApi = api as unknown as {
  getOPKCount: jest.Mock;
  uploadPrekeys: jest.Mock;
  replenishOPK: jest.Mock;
};

beforeEach(() => {
  jest.clearAllMocks();
});

describe("keys-sync.ts — ensureKeyBundleUploaded", () => {
  test("ilk yükleme: getOPKCount hata verirse yine de tekil-alan-adlı, gerçek imzalı bundle + OPK batch yükler", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockRejectedValue(new Error("henüz bundle yok — 404 benzeri"));

    await ensureKeyBundleUploaded(store);

    expect(mockedApi.uploadPrekeys).toHaveBeenCalledTimes(1);
    const payload = mockedApi.uploadPrekeys.mock.calls[0][0];

    // Backend PreKeyUploadRequest alan adlarıyla birebir (çoğul "signed_prekeys" DEĞİL)
    expect(typeof payload.identity_key).toBe("string");
    expect(typeof payload.signing_key).toBe("string");
    expect(typeof payload.signed_prekey).toBe("string");
    expect(typeof payload.signed_prekey_id).toBe("number");
    expect(typeof payload.signed_prekey_sig).toBe("string");
    expect(Array.isArray(payload.one_time_prekeys)).toBe(true);
    expect(payload.one_time_prekeys.length).toBe(30);
    expect(payload.one_time_prekeys[0]).toHaveProperty("id");
    expect(payload.one_time_prekeys[0]).toHaveProperty("public_key");

    // signed_prekey_sig, signing_key ile gerçekten doğrulanabiliyor mu
    // (sahte "imza=pubkey" değil, gerçek Ed25519 imza)
    const signingPub = base64ToU8(payload.signing_key);
    const spkPub = base64ToU8(payload.signed_prekey);
    const sig = base64ToU8(payload.signed_prekey_sig);
    expect(verifyWithKey(signingPub, spkPub, sig)).toBe(true);

    // identity_key X25519 (32 byte) — signing_key Ed25519 (32 byte) ile AYNI değer OLMAMALI
    expect(payload.identity_key).not.toBe(payload.signing_key);
  });

  test("OPK sayısı eşiğin üstündeyse yeni batch üretmez, boş one_time_prekeys ile yükler", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockResolvedValue({ count: 50, low: false, critical: false });

    await ensureKeyBundleUploaded(store);

    const payload = mockedApi.uploadPrekeys.mock.calls[0][0];
    expect(payload.one_time_prekeys).toEqual([]);
  });

  test("OPK sayısı eşiğin altındaysa (low) yeni batch üretip yükler", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockResolvedValue({ count: 5, low: true, critical: false });

    await ensureKeyBundleUploaded(store);

    const payload = mockedApi.uploadPrekeys.mock.calls[0][0];
    expect(payload.one_time_prekeys.length).toBe(30);
  });
});

describe("keys-sync.ts — replenishOPKsIfLow", () => {
  test("count düşükse replenish endpoint'ini çağırır", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockResolvedValue({ count: 3, low: true, critical: true });

    await replenishOPKsIfLow(store);

    expect(mockedApi.replenishOPK).toHaveBeenCalledTimes(1);
    const payload = mockedApi.replenishOPK.mock.calls[0][0];
    expect(payload.one_time_prekeys.length).toBe(30);
  });

  test("count yeterliyse replenish'i hiç çağırmaz", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockResolvedValue({ count: 40, low: false, critical: false });

    await replenishOPKsIfLow(store);

    expect(mockedApi.replenishOPK).not.toHaveBeenCalled();
  });

  test("aynı store'da art arda batch'ler OPK id'lerini ÇAKIŞTIRMAZ (SPK ile bağımsız sayaç)", async () => {
    const store = createMemoryStore();
    mockedApi.getOPKCount.mockResolvedValue({ count: 0, low: true, critical: true });

    await replenishOPKsIfLow(store);
    await replenishOPKsIfLow(store);

    const firstIds = mockedApi.replenishOPK.mock.calls[0][0].one_time_prekeys.map((o: any) => o.id);
    const secondIds = mockedApi.replenishOPK.mock.calls[1][0].one_time_prekeys.map((o: any) => o.id);
    const overlap = firstIds.filter((id: number) => secondIds.includes(id));
    expect(overlap.length).toBe(0);
  });
});
