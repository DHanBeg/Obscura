// L2 Tuğla 5b-2 Parça A — generateGroupId.
import { generateGroupId } from "../mls/groupId";

describe("mls/groupId.ts — generateGroupId", () => {
  test("her çağrı benzersiz byte'lar ve benzersiz b64 üretir", () => {
    const a = generateGroupId();
    const b = generateGroupId();
    expect(Buffer.from(a.bytes).equals(Buffer.from(b.bytes))).toBe(false);
    expect(a.b64).not.toBe(b.b64);
  });

  test("bytes 32 byte, b64 URL-safe (/, +, = yok)", () => {
    const { bytes, b64 } = generateGroupId();
    expect(bytes.length).toBe(32);
    expect(b64).not.toMatch(/[/+=]/);
  });

  test("çok sayıda üretimde çakışma yok (istatistiksel benzersizlik)", () => {
    const seen = new Set<string>();
    for (let i = 0; i < 500; i++) {
      const { b64 } = generateGroupId();
      expect(seen.has(b64)).toBe(false);
      seen.add(b64);
    }
  });

  test("b64, mls_relay_golden_test.go:175'teki kısıt gibi bir URL path segmentine güvenle gömülebilir", () => {
    const { b64 } = generateGroupId();
    const encoded = encodeURIComponent(b64);
    // URL-safe base64 zaten path-safe karakterlerden oluşur — encode etmek
    // onu DEĞİŞTİRMEMELİ (encodeURIComponent '-' ve '_'e dokunmaz).
    expect(encoded).toBe(b64);
  });
});
