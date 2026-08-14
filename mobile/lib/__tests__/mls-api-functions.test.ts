// L2 Tuğla 4a — group.ts'in monolit runTwoPartyMlsFlow'dan çıkarılan API-hazır
// fonksiyonları. Kripto DEĞİŞMEDİ, sadece parçalandı. Bu test izole çağrıyla
// (Alice/Bob ayrı, sırayla) TAM round-trip'i kanıtlıyor — createOwnKeyPackage'ın
// ürettiği wire format, createGroupWithMember/encryptGroupMessage'ın çıktısı,
// joinFromWelcomeWire/decryptApplicationMessageWire'ın (Tuğla 2A'dan beri var,
// golden fixture'ı okuyan) beklediği formatla TUTARLI mı — yani yeni parçalar
// birbirine ve eski parçalara gerçekten "API-uyumlu" mı.
import {
  getMlsCiphersuiteImpl,
  createOwnKeyPackage,
  createGroupWithMember,
  encryptGroupMessage,
  joinFromWelcomeWire,
  decryptApplicationMessageWire,
} from "../mls/group";

describe("group.ts — parçalanmış API fonksiyonları", () => {
  test("createOwnKeyPackage → createGroupWithMember → encryptGroupMessage → joinFromWelcomeWire → decryptApplicationMessageWire (tam round-trip)", async () => {
    const cs = await getMlsCiphersuiteImpl();
    const plaintext = "L2 Tuğla 4a — parçalanmış API round-trip";

    const aliceOwn = await createOwnKeyPackage("did:obs:alice-4a", cs);
    const bobOwn = await createOwnKeyPackage("did:obs:bob-4a", cs);
    expect(aliceOwn.keyPackageWireB64.length).toBeGreaterThan(0);
    expect(bobOwn.privateKeyPackage.initPrivateKeyB64.length).toBeGreaterThan(0);

    const groupId = new TextEncoder().encode("obscura-tugla4a-group");
    const created = await createGroupWithMember(aliceOwn, groupId, bobOwn.keyPackageWireB64, cs);
    expect(created.commitWireB64.length).toBeGreaterThan(0);
    expect(created.welcomeWireB64.length).toBeGreaterThan(0);
    expect(created.newEpoch).toBe(1);

    const bobState = await joinFromWelcomeWire(
      created.welcomeWireB64,
      bobOwn.keyPackageWireB64,
      bobOwn.privateKeyPackage,
      cs
    );

    const encrypted = await encryptGroupMessage(created.newState, plaintext, cs);
    expect(encrypted.ciphertextWireB64.length).toBeGreaterThan(0);
    expect(encrypted.epoch).toBe(1);

    const decrypted = await decryptApplicationMessageWire(bobState, encrypted.ciphertextWireB64, cs);
    expect(decrypted).toBe(plaintext);
  });
});
