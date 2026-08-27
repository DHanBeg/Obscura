// B11 — media-crypto.ts birim testleri (gerçek backend/native olmadan).
// expo-file-system'i in-memory sahte bir dosya sistemiyle mock'luyoruz
// (expo-secure-store'un memoryStore mock deseniyle aynı ilke, bkz.
// mls-e1-real-backend.smoke.test.ts) — crypto (@noble/ciphers) GERÇEK,
// sadece disk I/O sahte.
const mockFakeFS = new Map<string, string>();
jest.mock("expo-file-system", () => ({
  cacheDirectory: "file:///fake-cache/",
  EncodingType: { Base64: "base64" },
  readAsStringAsync: jest.fn((uri: string) => Promise.resolve(mockFakeFS.get(uri) ?? "")),
  writeAsStringAsync: jest.fn((uri: string, contents: string) => {
    mockFakeFS.set(uri, contents);
    return Promise.resolve();
  }),
  deleteAsync: jest.fn((uri: string) => {
    mockFakeFS.delete(uri);
    return Promise.resolve();
  }),
}));

import { encryptFileForUpload, decryptDownloadedBlob, decryptToTempFile, parseMediaKey } from "../media-crypto";
import { decryptBlob, encryptBlob } from "../session-store";
import { u8ToBase64, base64ToU8 } from "../crypto";

function randomBytes(n: number): Uint8Array {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return b;
}

describe("media-crypto — B11 blob şifreleme", () => {
  beforeEach(() => {
    mockFakeFS.clear();
    jest.clearAllMocks();
  });

  test("encryptFileForUpload: CSPRNG kullanır (Math.random DEĞİL) — crypto.getRandomValues fiilen çağrılıyor", async () => {
    const spy = jest.spyOn(crypto, "getRandomValues");
    const sourceUri = "file:///picker/video.mp4";
    mockFakeFS.set(sourceUri, u8ToBase64(randomBytes(1024)));

    await encryptFileForUpload(sourceUri);

    // encryptFileForUpload içinde HEM anahtar (32B) HEM nonce (12B, encryptBlob
    // içinde) için crypto.getRandomValues çağrılmış olmalı — en az 2 çağrı.
    expect(spy.mock.calls.length).toBeGreaterThanOrEqual(2);
    spy.mockRestore();
  });

  test("iki ayrı upload FARKLI anahtar üretir (rastgelelik gerçek, sabit/deterministik değil)", async () => {
    const sourceUri = "file:///picker/voice.m4a";
    mockFakeFS.set(sourceUri, u8ToBase64(randomBytes(256)));

    const a = await encryptFileForUpload(sourceUri);
    const b = await encryptFileForUpload(sourceUri);

    expect(a.keyB64).not.toBe(b.keyB64);
  });

  test("round-trip: encryptFileForUpload → (yüklenmiş byte'lar ham çözülemez) → doğru anahtarla decryptBlob orijinali verir", async () => {
    const sourceUri = "file:///picker/document.pdf";
    const original = randomBytes(4096);
    mockFakeFS.set(sourceUri, u8ToBase64(original));

    const { tempUri, keyB64 } = await encryptFileForUpload(sourceUri);

    // "Upload edilecek" byte'lar — MinIO'ya giden TAM OLARAK budur.
    const uploadedBytes = base64ToU8(mockFakeFS.get(tempUri)!);
    expect(uploadedBytes).not.toEqual(original); // ham byte orijinalle eşleşmiyor — şifreli

    // Yanlış anahtarla çözmeye çalışmak (GCM auth tag) başarısız olmalı —
    // sadece obfuscation değil, gerçek authenticated encryption.
    const wrongKey = randomBytes(32);
    expect(() => decryptBlob(wrongKey, uploadedBytes)).toThrow();

    // Doğru anahtar — byte-birebir orijinali verir.
    const key = base64ToU8(keyB64);
    const decrypted = decryptBlob(key, uploadedBytes);
    expect(decrypted).toEqual(original);
  });

  test("decryptDownloadedBlob + decryptToTempFile: fetch edilen ciphertext'i çözüp geçici dosyaya yazar", async () => {
    const original = randomBytes(2048);
    const key = randomBytes(32);
    // encryptFileForUpload'ın kullandığı AYNI encryptBlob ile "MinIO'da duran" ciphertext'i üret.
    const cipherBytes = encryptBlob(key, original);

    global.fetch = jest.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        arrayBuffer: () => Promise.resolve(cipherBytes.buffer.slice(cipherBytes.byteOffset, cipherBytes.byteOffset + cipherBytes.byteLength)),
      })
    ) as any;

    const keyB64 = u8ToBase64(key);
    const plain = await decryptDownloadedBlob("https://fake-minio.test/obj", keyB64);
    expect(plain).toEqual(original);

    const tempUri = await decryptToTempFile("https://fake-minio.test/obj", keyB64, "bin");
    expect(base64ToU8(mockFakeFS.get(tempUri)!)).toEqual(original);
  });

  test("parseMediaKey: yeni format (url|keyB64) ayrıştırılır, eski format (sadece url) legacy olarak keyB64:null döner", () => {
    const fresh = parseMediaKey("https://media.test/x.mp4|c29tZWtleQ==");
    expect(fresh.url).toBe("https://media.test/x.mp4");
    expect(fresh.keyB64).toBe("c29tZWtleQ==");

    const legacy = parseMediaKey("https://media.test/old-video.mp4");
    expect(legacy.url).toBe("https://media.test/old-video.mp4");
    expect(legacy.keyB64).toBeNull();
  });

  test("[file] payload şekli: <name>|<url>|<keyB64> üç segmente ayrılır (chat/[id].tsx'in split(\"|\") deseniyle aynı)", () => {
    const payload = "rapor.pdf|https://media.test/rapor.bin|c29tZWtleQ==";
    const parts = payload.split("|");
    expect(parts[0]).toBe("rapor.pdf");
    expect(parts[1]).toBe("https://media.test/rapor.bin");
    expect(parts[2]).toBe("c29tZWtleQ==");

    // legacy [file] (key segmenti yok) — parts[2] undefined, çağıran taraf
    // bunu "şifresiz" olarak okumalı (chat/[id].tsx ile aynı sözleşme).
    const legacyPayload = "eski-dosya.pdf|https://media.test/eski.pdf";
    const legacyParts = legacyPayload.split("|");
    expect(legacyParts[2]).toBeUndefined();
  });
});
