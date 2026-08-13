// L2 Tuğla 1 — 2-üye grup + tek mesaj, ts-mls ile (bkz. docs/adr/0019-mobile-mls-ts-port.md,
// "İlk kapanabilir dilim"). Alice grup kurar, Bob'u ekler, Alice şifreler, Bob çözer.
//
// Interop kancası: her adımın GERÇEK TLS-codec wire byte'ları (KeyPackage, Welcome,
// application-message) fixture'a yazılıyor — sonraki tuğlada backend openmls'in bunları
// çözebildiğini doğrulamak için. MLS her çalıştırmada taze rastgele anahtar üretir,
// bu yüzden fixture İÇERİĞİ her test koşusunda DEĞİŞİR — fixture'ın işi "sabit beklenen
// değer" değil, "şu an geçerli, gerçek bir MLS wire mesajı örneği" sağlamak.
import * as fs from "fs";
import * as path from "path";
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

    // Interop kancası: gerçek wire byte'larını fixture'a yaz — sonraki tuğla
    // (backend openmls decrypt testi) bu dosyayı okuyacak.
    const fixtureDir = path.join(__dirname, "../mls/fixtures");
    fs.mkdirSync(fixtureDir, { recursive: true });
    fs.writeFileSync(
      path.join(fixtureDir, "two_party.json"),
      JSON.stringify(
        {
          ciphersuite: "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
          plaintext,
          alice_key_package_wire_b64: result.aliceKeyPackageWireB64,
          bob_key_package_wire_b64: result.bobKeyPackageWireB64,
          welcome_wire_b64: result.welcomeWireB64,
          application_message_wire_b64: result.applicationMessageWireB64,
        },
        null,
        2
      )
    );
  });
});
