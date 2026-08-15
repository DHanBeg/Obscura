// L2 Tuğla 2 (Kısım A) — golden fixture: bir kez üretilip DONDURULMUŞ, her
// koşuda AYNI kalan MLS wire byte seti. Tuğla 1'in fixture'ından farkı budur —
// o taze rastgele anahtarla her koşuda değişiyordu, interop testi için
// tekrarlanamazdı. Bu dosya (two_party_golden.json) SADECE okunur, bu test
// tarafından asla yeniden üretilmez.
import * as fs from "fs";
import * as path from "path";
import { decodeMlsMessage } from "ts-mls";
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

  // L2 Tuğla 4e — commit_b64 alanı fixture'a eklendi (backend artık commit'i
  // kalıcılaştırıyor, bkz. mls_commit_persist_test.go). Bu test alanın
  // ÇÜRÜMESİNİ engeller: rastgele bir blob ya da başka bir koşudan kopyalanmış
  // yabancı bir wire buraya konursa aşağıdaki üç iddia da düşer. MLS
  // PrivateMessage başlığı (group_id, epoch, content_type) şifresizdir —
  // wire'ın kendisi ne olduğunu söyler, biz iddia etmiyoruz.
  test("commit_b64, golden grubun gerçek Commit wire'ı (başlık kendini böyle tanıtıyor)", () => {
    const golden = JSON.parse(fs.readFileSync(GOLDEN_PATH, "utf-8"));

    const decoded = decodeMlsMessage(new Uint8Array(Buffer.from(golden.commit_b64, "base64")), 0)?.[0];
    expect(decoded?.wireformat).toBe("mls_private_message");
    if (decoded?.wireformat !== "mls_private_message") throw new Error("commit wire private message değil");

    expect(new TextDecoder().decode(decoded.privateMessage.groupId)).toBe("obscura-golden-group");
    expect(decoded.privateMessage.contentType).toBe("commit");
    // Commit, golden epoch'ta (1) ÜRETİLDİ; uygulandığında grubu 2'ye taşır.
    expect(Number(decoded.privateMessage.epoch)).toBe(golden.epoch);
  });
});
