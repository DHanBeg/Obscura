jest.mock("../api", () => ({
  api: { sendMessage: jest.fn() },
}));
jest.mock("../e2e", () => ({
  encryptMessage: jest.fn(),
}));

import { api } from "../api";
import { encryptMessage } from "../e2e";
import { sendPanicAlert, sendImSafe, PANIC_MESSAGE_TYPE, IM_SAFE_MESSAGE_TYPE, PANIC_PRIVACY_NOTE } from "../panic";

const mockEncrypt = encryptMessage as jest.Mock;
const mockSend = api.sendMessage as jest.Mock;

describe("sendPanicAlert — Madde 13 panik mesajı", () => {
  beforeEach(() => {
    mockEncrypt.mockReset();
    mockSend.mockReset();
  });

  test("kaba grid_id dışında hiçbir ham koordinat payload'a girmez", async () => {
    mockEncrypt.mockImplementation(async (plaintext: string) => `CIPHER(${plaintext})`);
    mockSend.mockResolvedValue({ id: "m1", conv_id: "c1" });

    await sendPanicAlert("did:obscura:trusted", { latitude: 41.015137, longitude: 29.023056 });

    expect(mockEncrypt).toHaveBeenCalledTimes(1);
    const plaintextArg = mockEncrypt.mock.calls[0][0] as string;
    const payload = JSON.parse(plaintextArg);

    // Ham lat/lon plaintext payload'da bulunmamalı — yalnızca kaba grid_id.
    expect(payload).toHaveProperty("grid_id");
    expect(payload.grid_id).toBe("4557:3224");
    expect(payload).not.toHaveProperty("latitude");
    expect(payload).not.toHaveProperty("longitude");
    expect(payload).not.toHaveProperty("lat");
    expect(payload).not.toHaveProperty("lon");
  });

  test("şifreleme güven kişisinin DID'ine karşı yapılır", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m1", conv_id: "c1" });

    await sendPanicAlert("did:obscura:trusted", { latitude: 41.0, longitude: 29.0 });

    expect(mockEncrypt.mock.calls[0][1]).toBe("did:obscura:trusted");
  });

  test("gönderim PANIC_MESSAGE_TYPE tipiyle, şifreli ciphertext ile yapılır", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m1", conv_id: "c1" });

    await sendPanicAlert("did:obscura:trusted", { latitude: 41.0, longitude: 29.0 });

    expect(mockSend).toHaveBeenCalledWith({
      to_id: "did:obscura:trusted",
      ciphertext: "CIPHERTEXT",
      type: PANIC_MESSAGE_TYPE,
    });
  });

  test("PANIC_MESSAGE_TYPE backend'in panic_alert enum değeriyle eşleşir", () => {
    expect(PANIC_MESSAGE_TYPE).toBe("panic_alert");
  });

  test("gönderim sonucu (id/conv_id) çağırana döner", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m1", conv_id: "c1" });

    const result = await sendPanicAlert("did:obscura:trusted", { latitude: 41.0, longitude: 29.0 });
    expect(result).toEqual({ id: "m1", conv_id: "c1" });
  });

  test("PANIC_PRIVACY_NOTE dürüst metni taşır — 'sunucu hiçbir şey bilmez' DEMEZ", () => {
    expect(PANIC_PRIVACY_NOTE).toMatch(/sunucu konumu göremez/i);
    expect(PANIC_PRIVACY_NOTE).toMatch(/kime gönderdiğini sunucu görür/i);
    expect(PANIC_PRIVACY_NOTE.toLowerCase()).not.toContain("hiçbir şey bilmez");
    expect(PANIC_PRIVACY_NOTE.toLowerCase()).not.toContain("hiçbir şey göremez");
  });
});

describe("sendImSafe — Madde 13 Adım 7 (\"Buluştum, iyiyim\" onayı)", () => {
  beforeEach(() => {
    mockEncrypt.mockReset();
    mockSend.mockReset();
  });

  test("payload'da konum YOK — ne grid_id ne lat/lon, yalnızca sent_at", async () => {
    mockEncrypt.mockImplementation(async (plaintext: string) => `CIPHER(${plaintext})`);
    mockSend.mockResolvedValue({ id: "m2", conv_id: "c1" });

    await sendImSafe("did:obscura:trusted");

    const plaintextArg = mockEncrypt.mock.calls[0][0] as string;
    const payload = JSON.parse(plaintextArg);

    expect(payload).toHaveProperty("sent_at");
    expect(payload).not.toHaveProperty("grid_id");
    expect(payload).not.toHaveProperty("latitude");
    expect(payload).not.toHaveProperty("longitude");
    expect(payload).not.toHaveProperty("lat");
    expect(payload).not.toHaveProperty("lon");
    // "iyiyim" konum İSTEMEZ — fonksiyon imzası zaten coords parametresi almıyor.
    expect(Object.keys(payload)).toEqual(["sent_at"]);
  });

  test("şifreleme güven kişisinin DID'ine karşı yapılır", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m2", conv_id: "c1" });

    await sendImSafe("did:obscura:trusted");

    expect(mockEncrypt.mock.calls[0][1]).toBe("did:obscura:trusted");
  });

  test("gönderim IM_SAFE_MESSAGE_TYPE tipiyle yapılır", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m2", conv_id: "c1" });

    await sendImSafe("did:obscura:trusted");

    expect(mockSend).toHaveBeenCalledWith({
      to_id: "did:obscura:trusted",
      ciphertext: "CIPHERTEXT",
      type: IM_SAFE_MESSAGE_TYPE,
    });
  });

  test("IM_SAFE_MESSAGE_TYPE backend'in im_safe enum değeriyle eşleşir", () => {
    expect(IM_SAFE_MESSAGE_TYPE).toBe("im_safe");
  });

  test("gönderim sonucu (id/conv_id) çağırana döner", async () => {
    mockEncrypt.mockResolvedValue("CIPHERTEXT");
    mockSend.mockResolvedValue({ id: "m2", conv_id: "c1" });

    const result = await sendImSafe("did:obscura:trusted");
    expect(result).toEqual({ id: "m2", conv_id: "c1" });
  });
});
