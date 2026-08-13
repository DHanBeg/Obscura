// L2 Tuğla 1 — 2-üye grup + tek mesaj, ts-mls ile (bkz. docs/adr/0019-mobile-mls-ts-port.md,
// "İlk kapanabilir dilim"). Alice grup kurar, Bob'u ekler, Alice şifreler, Bob çözer.
//
// Not: bu test artık disk'e yazmıyor. Wire byte'ların DONDURULMUŞ (deterministik)
// kanıtı mls/fixtures/two_party_golden.json'da (Tuğla 2A) — o dosya interop
// testlerinin (Tuğla 3, backend openmls) okuduğu tek kaynak. Bu test'in kendi
// çıktısı taze-random (her koşuda değişir), bu yüzden hiçbir yerde saklanmaz —
// sadece bu koşumun içinde doğrulanır.
import { runTwoPartyMlsFlow } from "../mls/group";

describe("runTwoPartyMlsFlow", () => {
  test("Alice grup kurar, Bob'u ekler, Alice şifreler, Bob aynı plaintext'i çözer", async () => {
    const plaintext = "hello bob, this is end-to-end encrypted via MLS";
    const result = await runTwoPartyMlsFlow(plaintext);

    expect(result.decrypted).toBe(plaintext);
    expect(result.aliceKeyPackageWireB64.length).toBeGreaterThan(0);
    expect(result.bobKeyPackageWireB64.length).toBeGreaterThan(0);
    expect(result.welcomeWireB64.length).toBeGreaterThan(0);
    expect(result.applicationMessageWireB64.length).toBeGreaterThan(0);
  });
});
