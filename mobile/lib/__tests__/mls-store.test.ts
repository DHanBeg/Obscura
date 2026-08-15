// L2 Tuğla 5a — mls-store.ts: grup state kalıcılığı (#10 mimari notu).
import {
  getCiphersuiteFromName,
  getCiphersuiteImpl,
  nobleCryptoProvider,
  createCommit,
  processMessage,
  emptyPskIndex,
  acceptAll,
  encodeGroupState,
  type ClientState,
} from "ts-mls";
import {
  createOwnKeyPackage,
  createGroupWithMember,
  joinFromWelcomeWire,
  encryptGroupMessage,
  decryptApplicationMessageWire,
} from "../mls/group";
import {
  saveGroupState,
  loadGroupState,
  deleteGroupState,
  saveOwnKeyPackage,
  loadOwnKeyPackage,
  deleteOwnKeyPackage,
  MLS_RETAIN_EPOCHS,
  type MlsStores,
} from "../mls/mls-store";
import { createMemoryStore } from "../../test-utils/memoryStore";

const CIPHERSUITE_NAME = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519";
async function cs() {
  return getCiphersuiteImpl(getCiphersuiteFromName(CIPHERSUITE_NAME), nobleCryptoProvider);
}

function stores(): MlsStores {
  return { secure: createMemoryStore(), async: createMemoryStore() };
}

async function makeAliceBobGroup() {
  const impl = await cs();
  const aliceKp = await createOwnKeyPackage("did:obs:mls-store-alice", impl);
  const bobKp = await createOwnKeyPackage("did:obs:mls-store-bob", impl);
  const groupId = new TextEncoder().encode("mls-store-test-group");
  const created = await createGroupWithMember(aliceKp, groupId, bobKp.keyPackageWireB64, impl);
  const bobState = await joinFromWelcomeWire(created.welcomeWireB64, bobKp.keyPackageWireB64, bobKp.privateKeyPackage, impl);
  return { impl, aliceKp, bobKp, aliceState: created.newState, bobState };
}

describe("mls-store.ts — grup state kaydet → yükle", () => {
  test("round-trip byte-eşit: yüklenen state yeniden encode edilince orijinalle birebir aynı", async () => {
    const st = stores();
    const { aliceState } = await makeAliceBobGroup();

    await saveGroupState("g-roundtrip", aliceState, st);
    const loaded = await loadGroupState("g-roundtrip", st);
    expect(loaded).not.toBeNull();

    const originalBytes = encodeGroupState(aliceState);
    const loadedBytes = encodeGroupState(loaded!);
    expect(Buffer.from(loadedBytes).equals(Buffer.from(originalBytes))).toBe(true);
    expect(Number(loaded!.groupContext.epoch)).toBe(Number(aliceState.groupContext.epoch));
  });

  test("kayıt yoksa loadGroupState null döner", async () => {
    const st = stores();
    expect(await loadGroupState("never-saved-group", st)).toBeNull();
  });

  test("deleteGroupState sonrası loadGroupState null döner", async () => {
    const st = stores();
    const { aliceState } = await makeAliceBobGroup();
    await saveGroupState("g-delete", aliceState, st);
    expect(await loadGroupState("g-delete", st)).not.toBeNull();

    await deleteGroupState("g-delete", st);
    expect(await loadGroupState("g-delete", st)).toBeNull();
  });

  test("diskteki ham blob şifreli — plaintext state verisi İÇERMİYOR", async () => {
    const st = stores();
    const { aliceState } = await makeAliceBobGroup();
    await saveGroupState("g-secrecy", aliceState, st);

    const rawHex = await st.async!.getItem("obscura_mls_group_g-secrecy");
    expect(rawHex).not.toBeNull();

    // Ham blob JSON DEĞİL (TLS-codec + AES-GCM binary) — JSON.parse denemesi patlamalı.
    expect(() => JSON.parse(rawHex!)).toThrow();

    // Açık state'teki tanınabilir byte dizileri (signature private key, groupId)
    // hex blob içinde düz metin olarak görünmemeli.
    const groupIdHex = Buffer.from("mls-store-test-group").toString("hex");
    expect(rawHex!.toLowerCase()).not.toContain(groupIdHex);
    const sigKeyHex = Buffer.from(aliceState.signaturePrivateKey).toString("hex");
    expect(rawHex!.toLowerCase()).not.toContain(sigKeyHex);
  });

  test("yanlış master key ile loadGroupState reddeder (bütünlük)", async () => {
    const stA = stores();
    const stB: MlsStores = { secure: createMemoryStore(), async: stA.async }; // farklı secure store → farklı master key, AYNI blob
    const { aliceState } = await makeAliceBobGroup();

    await saveGroupState("g-wrongkey", aliceState, stA);
    await expect(loadGroupState("g-wrongkey", stB)).rejects.toThrow();
  });
});

describe("mls-store.ts — kalıcı state'ten fonksiyonel devam", () => {
  test("KRİTİK: save→load→createCommit→save→load zinciri — persist edilmiş state gerçekten epoch ilerletebiliyor", async () => {
    const st = stores();
    const { impl, aliceState, bobState } = await makeAliceBobGroup();

    // 1. adım: Alice'in state'i (epoch 1) kaydedilir, in-memory referans "kaybedilir".
    await saveGroupState("g-continue", aliceState, st);
    const aliceReloaded1 = await loadGroupState("g-continue", st);
    expect(aliceReloaded1).not.toBeNull();

    // 2. adım: Yüklenen state'ten devam — Alice bir mesaj şifreler, Bob (hiç
    // kesintiye uğramamış gerçek peer) çözebilmeli.
    const enc1 = await encryptGroupMessage(aliceReloaded1!, "reload sonrası ilk mesaj", impl);
    const dec1 = await decryptApplicationMessageWire(bobState, enc1.ciphertextWireB64, impl);
    expect(dec1).toBe("reload sonrası ilk mesaj");

    // 3. adım: Yüklenen state'ten bir COMMIT üret (self-update, epoch ilerler),
    // tekrar kaydet, tekrar yükle.
    const commitResult = await createCommit({ state: aliceReloaded1!, cipherSuite: impl });
    const aliceAfterCommit = commitResult.newState;
    expect(Number(aliceAfterCommit.groupContext.epoch)).toBe(Number(aliceReloaded1!.groupContext.epoch) + 1);

    await saveGroupState("g-continue", aliceAfterCommit, st);
    const aliceReloaded2 = await loadGroupState("g-continue", st);
    expect(Number(aliceReloaded2!.groupContext.epoch)).toBe(Number(aliceAfterCommit.groupContext.epoch));

    // 4. adım: Bob da aynı commit'i işler (epoch senkron), sonra yüklenen
    // ikinci-nesil state'ten devam eden bir mesaj hâlâ çözülebiliyor mu.
    const bobProcessed = await processMessage(commitResult.commit as any, bobState, emptyPskIndex, acceptAll, impl);
    if (bobProcessed.kind !== "newState") throw new Error("beklenmeyen processMessage sonucu: " + bobProcessed.kind);
    const bobAfterCommit = bobProcessed.newState;

    const enc2 = await encryptGroupMessage(aliceReloaded2!, "iki kez reload sonrası ikinci mesaj", impl);
    const dec2 = await decryptApplicationMessageWire(bobAfterCommit, enc2.ciphertextWireB64, impl);
    expect(dec2).toBe("iki kez reload sonrası ikinci mesaj");
  });
});

describe("mls-store.ts — kendi KeyPackage private-state'i (#10 local-only)", () => {
  test("save → load round-trip, private key material korunuyor", async () => {
    const st = stores();
    const impl = await cs();
    const own = await createOwnKeyPackage("did:obs:mls-store-kp", impl);

    await saveOwnKeyPackage("did:obs:mls-store-kp", own, st);
    const loaded = await loadOwnKeyPackage("did:obs:mls-store-kp", st);

    expect(loaded).not.toBeNull();
    expect(loaded!.keyPackageWireB64).toBe(own.keyPackageWireB64);
    expect(loaded!.privateKeyPackage).toEqual(own.privateKeyPackage);
  });

  test("kayıt yoksa null, delete sonrası null", async () => {
    const st = stores();
    expect(await loadOwnKeyPackage("never-did", st)).toBeNull();

    const impl = await cs();
    const own = await createOwnKeyPackage("did:obs:mls-store-kp2", impl);
    await saveOwnKeyPackage("did:obs:mls-store-kp2", own, st);
    await deleteOwnKeyPackage("did:obs:mls-store-kp2", st);
    expect(await loadOwnKeyPackage("did:obs:mls-store-kp2", st)).toBeNull();
  });

  test("diskteki ham blob plaintext private key içermiyor", async () => {
    const st = stores();
    const impl = await cs();
    const own = await createOwnKeyPackage("did:obs:mls-store-kp3", impl);
    await saveOwnKeyPackage("did:obs:mls-store-kp3", own, st);

    const rawHex = await st.async!.getItem("obscura_mls_keypkg_did:obs:mls-store-kp3");
    expect(rawHex).not.toBeNull();
    expect(() => JSON.parse(rawHex!)).toThrow();
    expect(rawHex).not.toContain(own.privateKeyPackage.hpkePrivateKeyB64);
  });
});

describe("mls-store.ts — PCS kalibrasyonu (retainKeysForEpochs)", () => {
  test("MLS_RETAIN_EPOCHS gerçekten uygulanıyor: yeterli commit sonrası historicalReceiverData penceresi bu sayıya sabitleniyor", async () => {
    const { impl, aliceState } = await makeAliceBobGroup(); // epoch 1
    let state: ClientState = aliceState;

    // Varsayılan (4) ile karışmasın diye yeterince fazla self-update commit
    // (MLS_RETAIN_EPOCHS + 3) — pencere gerçekten 2'de sabitlenmiş mi.
    for (let i = 0; i < MLS_RETAIN_EPOCHS + 3; i++) {
      const commitResult = await createCommit({ state, cipherSuite: impl });
      state = commitResult.newState;
    }

    expect(state.historicalReceiverData.size).toBe(MLS_RETAIN_EPOCHS);
    // Kalan pencere EN YENİ epoch'lar olmalı, epoch 0 (grup kuruluşu) çoktan düşmüş olmalı.
    expect(state.historicalReceiverData.has(0n)).toBe(false);
  });
});
