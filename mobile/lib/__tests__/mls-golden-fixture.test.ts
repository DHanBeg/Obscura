// L2 Tuğla 2 (Kısım A) — golden fixture: bir kez üretilip DONDURULMUŞ, her
// koşuda AYNI kalan MLS wire byte seti. Tuğla 1'in fixture'ından farkı budur —
// o taze rastgele anahtarla her koşuda değişiyordu, interop testi için
// tekrarlanamazdı. Bu dosya (two_party_golden.json) SADECE okunur, bu test
// tarafından asla yeniden üretilmez.
import * as fs from "fs";
import * as path from "path";
import { joinFromWelcomeWire, decryptApplicationMessageWire, getMlsCiphersuiteImpl } from "../mls/group";

const GOLDEN_PATH = path.join(__dirname, "../mls/fixtures/two_party_golden.json");

describe("two_party_golden.json — dondurulmuş wire byte seti hâlâ geçerli mi", () => {
  test("Bob, SADECE golden dosyadaki byte'lardan gruba katılıp sabit plaintext'i çözebiliyor", async () => {
    const golden = JSON.parse(fs.readFileSync(GOLDEN_PATH, "utf-8"));
    const cs = await getMlsCiphersuiteImpl();

    const bobState = await joinFromWelcomeWire(
      golden.welcome_wire_b64,
      golden.bob_key_package_wire_b64,
      {
        initPrivateKeyB64: golden.bob_private_key_package.init_private_key_b64,
        hpkePrivateKeyB64: golden.bob_private_key_package.hpke_private_key_b64,
        signaturePrivateKeyB64: golden.bob_private_key_package.signature_private_key_b64,
      },
      cs
    );

    const decrypted = await decryptApplicationMessageWire(bobState, golden.application_message_wire_b64, cs);

    expect(decrypted).toBe(golden.plaintext);
  });
});
