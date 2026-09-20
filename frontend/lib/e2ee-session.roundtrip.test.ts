import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  generateIdentity,
  generatePreKeyStore,
  toB64,
  type IdentityKeys,
  type PreKeyBundle,
  type PreKeyStore,
  type RatchetState,
} from "./e2ee";
import {
  setActiveAccountDid,
  initiateSession,
  encryptForSend,
  loadSession,
  decryptIncoming,
  loadPendingX3dhInit,
} from "./e2ee-session";

const CONV_ID = "conv-roundtrip-1";

interface Party {
  identity: IdentityKeys;
  store: PreKeyStore;
}

async function makeParty(): Promise<Party> {
  const identity = await generateIdentity();
  const store = await generatePreKeyStore(identity);
  return { identity, store };
}

// Alice'in sunucudan alacağı bundle'ın yerel karşılığı (ilk OPK dahil).
function bundleOf(p: Party): PreKeyBundle {
  const opk = p.store.oneTimePreKeys[0];
  return {
    identity_key: toB64(p.identity.dhKeyPair.publicKeyBytes),
    signed_prekey: toB64(p.store.signedPreKey.publicKeyBytes),
    signed_prekey_sig: toB64(p.store.signedPreKeySig),
    one_time_prekey: toB64(opk.keyPair.publicKeyBytes),
    one_time_prekey_id: opk.id,
    did: p.identity.did,
  };
}

// Tek localStorage'ı paylaşan iki hesap: aktif hesap DID'i ile ayrışırlar.
function actAs(p: Party): void {
  setActiveAccountDid(p.identity.did);
}

describe("e2ee-session: encrypt → decryptIncoming round-trip (saf kripto, backend yok)", () => {
  let alice: Party;
  let bob: Party;

  beforeEach(async () => {
    localStorage.clear();
    vi.restoreAllMocks();
    alice = await makeParty();
    bob = await makeParty();
  });

  it("ilk mesaj: alıcı X3DH zarfından oturumu kurar ve doğru plaintext'i çözer", async () => {
    actAs(alice);
    const { state, x3dhInit } = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const { ciphertext } = await encryptForSend(state, "merhaba bob", CONV_ID, x3dhInit);

    actAs(bob);
    const onUpdate = vi.fn<(convId: string, s: RatchetState) => void>();
    const plaintext = await decryptIncoming(CONV_ID, ciphertext, bob.identity, bob.store, onUpdate);

    expect(plaintext).toBe("merhaba bob");
    expect(onUpdate).toHaveBeenCalled();
    expect(await loadSession(CONV_ID)).not.toBeNull();
  });

  it("ikinci mesaj (zarfsız) mevcut alıcı oturumuyla çözülür", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "bir", CONV_ID, first.x3dhInit);
    const m2 = await encryptForSend(m1.newState, "iki", CONV_ID);

    actAs(bob);
    const noop = () => {};
    expect(await decryptIncoming(CONV_ID, m1.ciphertext, bob.identity, bob.store, noop)).toBe("bir");
    expect(await decryptIncoming(CONV_ID, m2.ciphertext, bob.identity, bob.store, noop)).toBe("iki");
  });

  it("cevap yönü: Bob'un gönderdiği mesajı Alice çözer (DH ratchet adımı)", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "selam", CONV_ID, first.x3dhInit);

    actAs(bob);
    const noop = () => {};
    await decryptIncoming(CONV_ID, m1.ciphertext, bob.identity, bob.store, noop);
    const bobState = await loadSession(CONV_ID);
    expect(bobState).not.toBeNull();
    const reply = await encryptForSend(bobState!, "cevap", CONV_ID);

    actAs(alice);
    expect(await decryptIncoming(CONV_ID, reply.ciphertext, alice.identity, alice.store, noop)).toBe("cevap");
  });

  it("hesap-bazlı anahtarlama: aynı convId'de iki hesabın oturumu birbirini ezmez", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    expect(loadPendingX3dhInit(CONV_ID)).not.toBeNull();

    actAs(bob);
    expect(await loadSession(CONV_ID)).toBeNull();
    expect(loadPendingX3dhInit(CONV_ID)).toBeNull();

    actAs(alice);
    expect(await loadSession(CONV_ID)).not.toBeNull();
    expect(first.x3dhInit.opkId).toBe(0);
  });

  it("aynı şifreli mesaj ikinci kez gelince önbellekten döner (zincir anahtarı tek kullanımlık)", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "tek sefer", CONV_ID, first.x3dhInit);

    actAs(bob);
    const noop = () => {};
    expect(await decryptIncoming(CONV_ID, m1.ciphertext, bob.identity, bob.store, noop)).toBe("tek sefer");
    expect(await decryptIncoming(CONV_ID, m1.ciphertext, bob.identity, bob.store, noop)).toBe("tek sefer");
  });

  it("oturum yok + X3DH zarfı yok: ham ciphertext sızmaz, güvenli placeholder döner", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "gizli", CONV_ID); // zarfsız

    actAs(bob);
    const out = await decryptIncoming(CONV_ID, m1.ciphertext, bob.identity, bob.store, () => {});
    expect(out).not.toContain("gizli");
    expect(out).not.toBe(m1.ciphertext);
    expect(out).toContain("oturum kurulamadi");
  });

  it("anahtarlar hazır değilken (identity/prekeyStore null) placeholder döner, plaintext üretmez", async () => {
    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "gizli", CONV_ID, first.x3dhInit);

    actAs(bob);
    const out = await decryptIncoming(CONV_ID, m1.ciphertext, null, null, () => {});
    expect(out).toContain("anahtarlar hazir degil");
  });

  it("yanlış alıcı (başka prekey deposu) mesajı çözemez, plaintext sızmaz", async () => {
    const mallory = await makeParty();
    vi.spyOn(console, "error").mockImplementation(() => {});

    actAs(alice);
    const first = await initiateSession(alice.identity, bundleOf(bob), CONV_ID);
    const m1 = await encryptForSend(first.state, "sadece bob", CONV_ID, first.x3dhInit);

    actAs(mallory);
    const out = await decryptIncoming(CONV_ID, m1.ciphertext, mallory.identity, mallory.store, () => {});
    expect(out).not.toContain("sadece bob");
    expect(out).toContain("cozulemedi");
  });
});
