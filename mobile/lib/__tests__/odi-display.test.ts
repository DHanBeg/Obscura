import { displayIdentifier, sanitizeNickname, isValidNickname, deriveOdiFromDid } from "../odi-display";

describe("displayIdentifier — DID hiçbir ekranda ham gösterilmez, ODI tercih edilir", () => {
  test("odi varsa odi döner", () => {
    expect(displayIdentifier({ odi: "ODI-ABCD-EFGH-JKMN", did: "did:obs:xyz" })).toBe(
      "ODI-ABCD-EFGH-JKMN"
    );
  });

  test("odi boşsa did'e düşer", () => {
    expect(displayIdentifier({ odi: "", did: "did:obs:xyz" })).toBe("did:obs:xyz");
  });

  test("odi yoksa (undefined) did'e düşer", () => {
    expect(displayIdentifier({ did: "did:obs:xyz" })).toBe("did:obs:xyz");
  });

  test("ikisi de yoksa boş string döner", () => {
    expect(displayIdentifier({})).toBe("");
    expect(displayIdentifier(null)).toBe("");
    expect(displayIdentifier(undefined)).toBe("");
  });
});

describe("sanitizeNickname / isValidNickname — backend sanitizeDisplayName ile aynı kural", () => {
  test("baştaki/sondaki boşluğu kırpar", () => {
    expect(sanitizeNickname("  Ada Lovelace  ")).toBe("Ada Lovelace");
  });

  test("bidi/zero-width kontrol karakterlerini siler", () => {
    expect(sanitizeNickname("‮nasil")).toBe("nasil");
  });

  test("boş string geçersiz", () => {
    expect(isValidNickname("")).toBe(false);
  });

  test("sadece boşluk geçersiz", () => {
    expect(isValidNickname("   ")).toBe(false);
  });

  test("sadece bidi/zero-width karakter geçersiz", () => {
    expect(isValidNickname("‮​")).toBe(false);
  });

  test("normal isim geçerli", () => {
    expect(isValidNickname("Ada")).toBe(true);
  });
});

// deriveOdiFromDid — backend internal/identity.DeriveODI ÇALIŞTIRILARAK
// üretilmiş gerçek golden vector'lar (bkz. odi-display.ts yorumu). Herhangi
// bir vektör tutmazsa NFC ekranındaki ODI, backend'in hesaplayacağı ODI'den
// FARKLI olur — sessiz bir tutarsızlık, bu yüzden kritik.
describe("deriveOdiFromDid — Go internal/identity.DeriveODI ile vector cross-check", () => {
  const VECTORS: Array<[string, string]> = [
    ["did:obs:0000000000000000000000000000000000000000", "ODI-7JEJ-A46K-S5EX"],
    ["did:obs:abcdef0123456789abcdef0123456789abcdef01", "ODI-0GJE-E7HA-2DFQ"],
    ["", "ODI-WERC-8GMR-ZGE1"],
    ["did:obs:nfc-pairing-cross-check-vector-001", "ODI-Z367-A36S-GQ3D"],
  ];

  test.each(VECTORS)("DeriveODI(%s) === %s (Go ile bire bir)", (did, want) => {
    expect(deriveOdiFromDid(did)).toBe(want);
  });

  test("deterministik — aynı DID her zaman aynı ODI'yi üretir", () => {
    const did = "did:obs:determinism-check";
    expect(deriveOdiFromDid(did)).toBe(deriveOdiFromDid(did));
  });
});
