import { describe, it, expect } from "vitest";
import { sentAtToMs, sentAtToIso } from "./sent-at";

const SECONDS = 1_790_000_000; // ~2026, Unix saniyesi
const ISO = new Date(SECONDS * 1000).toISOString();

describe("sent-at: sentAtToMs", () => {
  it("Unix saniyesini (WS yükü) ms'ye çevirir — 1970'e düşmez", () => {
    expect(sentAtToMs(SECONDS)).toBe(SECONDS * 1000);
    expect(new Date(sentAtToMs(SECONDS)).getUTCFullYear()).toBeGreaterThanOrEqual(2026);
  });

  it("zaten ms olan sayıyı olduğu gibi bırakır", () => {
    expect(sentAtToMs(SECONDS * 1000)).toBe(SECONDS * 1000);
  });

  it("RFC3339 string'i (REST yanıtı) ayrıştırır", () => {
    expect(sentAtToMs(ISO)).toBe(SECONDS * 1000);
  });

  it("sayısal string'i saniye gibi ele alır", () => {
    expect(sentAtToMs(String(SECONDS))).toBe(SECONDS * 1000);
  });

  it("geçersiz/eksik değer için 0 döner, fırlatmaz", () => {
    for (const bad of [undefined, null, "", "yarin", NaN, Infinity, -5, 0, {}, []]) {
      expect(sentAtToMs(bad)).toBe(0);
    }
  });
});

describe("sent-at: sentAtToIso", () => {
  it("saniye → ISO", () => {
    expect(sentAtToIso(SECONDS)).toBe(ISO);
  });

  it("ISO string'i aynı anı verir", () => {
    expect(sentAtToIso(ISO)).toBe(ISO);
  });

  it("çözülemeyen zamanda şimdiki ana düşer (1970 değil)", () => {
    const iso = sentAtToIso(undefined);
    expect(new Date(iso).getUTCFullYear()).toBeGreaterThanOrEqual(2026);
    expect(Math.abs(Date.now() - Date.parse(iso))).toBeLessThan(5_000);
  });
});
