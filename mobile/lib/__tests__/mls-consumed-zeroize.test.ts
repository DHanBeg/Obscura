// L2 Tuğla 5a bölüm C — createCommit/processMessage'ın döndürdüğü `consumed`
// (kullanımdan çıkan epoch'un ham secret byte'ları) artık group.ts'in 4
// çağrı noktasında zeroizeConsumed() ile RAM'den temizleniyor (defense-in-depth).
// Bu test zeroizeConsumed'ı GERÇEK ts-mls consumed byte'larıyla doğrular —
// group.ts'in private scope'undaki çağrıları doğrudan gözlemleyemeyiz, ama
// aynı fonksiyonu aynı şekilde üretilmiş veriyle çalıştırıp (1) önce sıfır
// olmadığını (gerçek secret, çöp veri değil), (2) sonra tamamen sıfır
// olduğunu, (3) zeroize'ın newState'i BOZMADIĞINI (aynı obje değil, ayrı
// kopya) kanıtlar.
import {
  getCiphersuiteFromName,
  getCiphersuiteImpl,
  nobleCryptoProvider,
  createCommit,
  processMessage,
  emptyPskIndex,
  acceptAll,
} from "ts-mls";
import {
  createOwnKeyPackage,
  createGroupWithMember,
  joinFromWelcomeWire,
  encryptGroupMessage,
  decryptApplicationMessageWire,
  zeroizeConsumed,
} from "../mls/group";

const CIPHERSUITE_NAME = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519";
async function cs() {
  return getCiphersuiteImpl(getCiphersuiteFromName(CIPHERSUITE_NAME), nobleCryptoProvider);
}

function isAllZero(buf: Uint8Array): boolean {
  return buf.every((b) => b === 0);
}

describe("group.ts — zeroizeConsumed (Tuğla 5a bölüm C)", () => {
  test("createCommit'in consumed byte'ları önce GERÇEK (sıfır olmayan) secret, zeroize sonrası tamamen sıfır", async () => {
    const impl = await cs();
    const aliceKp = await createOwnKeyPackage("did:obs:zeroize-alice", impl);
    const bobKp = await createOwnKeyPackage("did:obs:zeroize-bob", impl);
    const groupId = new TextEncoder().encode("zeroize-test-group");

    const created = await createGroupWithMember(aliceKp, groupId, bobKp.keyPackageWireB64, impl);

    // Bir sonraki commit'in consumed'ını doğrudan gözlemlemek için burada
    // AYNI ts-mls çağrısını group.ts'in yaptığı gibi tekrar ediyoruz (group.ts
    // zaten kendi 4 noktasında zeroize edip döndürmüyor — bu, gözlemlenebilir
    // bir consumed elde etmenin tek yolu).
    const commitResult = await createCommit({ state: created.newState, cipherSuite: impl });
    expect(commitResult.consumed.length).toBeGreaterThan(0);

    // Zeroize'dan ÖNCE: en az bir consumed byte dizisi gerçek secret veri
    // taşımalı (hepsi tesadüfen sıfırsa bu test bir şey kanıtlamaz).
    const anyNonZeroBefore = commitResult.consumed.some((buf) => buf.length > 0 && !isAllZero(buf));
    expect(anyNonZeroBefore).toBe(true);

    zeroizeConsumed(commitResult.consumed);

    for (const buf of commitResult.consumed) {
      expect(isAllZero(buf)).toBe(true);
    }

    // newState zeroize'dan ETKİLENMEMİŞ olmalı — ayrı obje/buffer'lar,
    // grup akışı zeroize sonrası da sorunsuz devam edebilmeli.
    const enc = await encryptGroupMessage(commitResult.newState, "zeroize sonrası mesaj hâlâ çalışıyor", impl);
    expect(enc.ciphertextWireB64.length).toBeGreaterThan(0);
  });

  test("processMessage'ın consumed byte'ları da aynı şekilde zeroize edilebiliyor, newState etkilenmiyor", async () => {
    const impl = await cs();
    const aliceKp = await createOwnKeyPackage("did:obs:zeroize-alice2", impl);
    const bobKp = await createOwnKeyPackage("did:obs:zeroize-bob2", impl);
    const groupId = new TextEncoder().encode("zeroize-test-group-2");

    const created = await createGroupWithMember(aliceKp, groupId, bobKp.keyPackageWireB64, impl);
    const bobState = await joinFromWelcomeWire(created.welcomeWireB64, bobKp.keyPackageWireB64, bobKp.privateKeyPackage, impl);

    const commitResult = await createCommit({ state: created.newState, cipherSuite: impl });
    zeroizeConsumed(commitResult.consumed); // group.ts deseniyle aynı — ara adım temiz

    const processResult = await processMessage(commitResult.commit as any, bobState, emptyPskIndex, acceptAll, impl);
    if (processResult.kind !== "newState") throw new Error("beklenmeyen kind: " + processResult.kind);
    expect(processResult.consumed.length).toBeGreaterThan(0);

    const anyNonZeroBefore = processResult.consumed.some((buf) => buf.length > 0 && !isAllZero(buf));
    expect(anyNonZeroBefore).toBe(true);

    zeroizeConsumed(processResult.consumed);
    for (const buf of processResult.consumed) {
      expect(isAllZero(buf)).toBe(true);
    }

    // Bob'un yeni state'i hâlâ fonksiyonel: Alice'ten gelecek bir mesajı çözebilmeli.
    const enc = await encryptGroupMessage(commitResult.newState, "bob zeroize sonrası da çözebiliyor", impl);
    const dec = await decryptApplicationMessageWire(processResult.newState, enc.ciphertextWireB64, impl);
    expect(dec).toBe("bob zeroize sonrası da çözebiliyor");
  });
});
