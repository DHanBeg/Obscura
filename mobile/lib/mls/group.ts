// L2 Tuğla 1 — 2-üye MLS grubu + tek uygulama mesajı, ts-mls ile.
// Bkz. docs/adr/0019-mobile-mls-ts-port.md (Decision, Implementation notes,
// Scope → "İlk kapanabilir dilim"). Genel N-üyeli grup API'si DEĞİL — bu
// dilim kasıtlı olarak statik 2-üye + tek mesaja sınırlı.
//
// ts-mls API sürtünmeleri (spike'ta bulundu, ADR'ye not düşüldü):
// - getCiphersuiteImpl bare id değil getCiphersuiteFromName(...) çıktısı ister.
// - X25519 suite'i SADECE nobleCryptoProvider ile çalışır (defaultCryptoProvider
//   WebCrypto tabanlı, X25519'u desteklemiyor, "kdf undefined" ile patlar).
// - defaultCapabilities BİR FONKSİYON, çağrılmalı (defaultCapabilities()).
// - emptyPskIndex BİR SABİT, fonksiyon değil.
import {
  getCiphersuiteFromName,
  getCiphersuiteImpl,
  nobleCryptoProvider,
  generateKeyPackage,
  createGroup,
  createCommit,
  joinGroup,
  createApplicationMessage,
  processMessage,
  acceptAll,
  emptyPskIndex,
  defaultCapabilities,
  defaultLifetime,
  encodeMlsMessage,
  decodeMlsMessage,
  type MLSMessage,
  type KeyPackage,
  type PrivateKeyPackage,
  type ClientState,
  type CiphersuiteImpl,
} from "ts-mls";
import { mlsClientConfig } from "./mls-store";

const CIPHERSUITE_NAME = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519";

// Tuğla 5a — createCommit/processMessage her ikisi de kullanımdan çıkan
// epoch'un ham secret byte'larını `consumed` dizisinde geri verir (eski
// epoch'un secretTree yaprakları + initSecret — ts-mls'in kendi API
// sözleşmesi, bkz. docs/adr notu). newState zaten bunları TAŞIMIYOR (state
// içinde referansları yok) — bu sadece JS bellekte bir süre daha yaşayacak
// olan ayrık kopyaları temizler (defense-in-depth, disk değil RAM için).
export function zeroizeConsumed(consumed: Uint8Array[]): void {
  for (const buf of consumed) buf.fill(0);
}

/** Bu modülün her yerinde kullanılan ciphersuite impl'i — nobleCryptoProvider
 * ZORUNLU (X25519, defaultCryptoProvider'da desteklenmiyor). */
export async function getMlsCiphersuiteImpl(): Promise<CiphersuiteImpl> {
  return getCiphersuiteImpl(getCiphersuiteFromName(CIPHERSUITE_NAME), nobleCryptoProvider);
}

export interface TwoPartyMlsRun {
  aliceKeyPackageWireB64: string;
  bobKeyPackageWireB64: string;
  welcomeWireB64: string;
  applicationMessageWireB64: string;
  plaintext: string;
  decrypted: string;
}

function encodeWire(msg: MLSMessage): string {
  return Buffer.from(encodeMlsMessage(msg)).toString("base64");
}

function decodeWire(b64: string): MLSMessage {
  const bytes = new Uint8Array(Buffer.from(b64, "base64"));
  const decoded = decodeMlsMessage(bytes, 0)?.[0];
  if (!decoded) throw new Error("decodeWire: geçersiz MLS wire mesajı");
  return decoded;
}

/**
 * Alice grup kurar, Bob'u KeyPackage'ıyla ekler, Alice tek bir application
 * mesajı şifreler, Bob Welcome'ı işleyip mesajı çözer. Her adımın gerçek
 * TLS-codec wire byte'ları (base64) sonuçta dönüyor — backend interop
 * testleri için.
 */
export async function runTwoPartyMlsFlow(plaintext: string): Promise<TwoPartyMlsRun> {
  const ciphersuite = getCiphersuiteFromName(CIPHERSUITE_NAME);
  const aliceImpl = await getCiphersuiteImpl(ciphersuite, nobleCryptoProvider);
  const bobImpl = await getCiphersuiteImpl(ciphersuite, nobleCryptoProvider);

  const aliceCred = { credentialType: "basic" as const, identity: new TextEncoder().encode("did:obs:alice") };
  const bobCred = { credentialType: "basic" as const, identity: new TextEncoder().encode("did:obs:bob") };

  const aliceKp = await generateKeyPackage(aliceCred, defaultCapabilities(), defaultLifetime, [], aliceImpl);
  const bobKp = await generateKeyPackage(bobCred, defaultCapabilities(), defaultLifetime, [], bobImpl);

  let aliceGroup = await createGroup(
    new TextEncoder().encode("obscura-two-party-group"),
    aliceKp.publicPackage,
    aliceKp.privatePackage,
    [],
    aliceImpl,
    mlsClientConfig()
  );

  const addProposal = { proposalType: "add" as const, add: { keyPackage: bobKp.publicPackage } };
  const commitResult = await createCommit(
    { state: aliceGroup, cipherSuite: aliceImpl },
    // ratchetTreeExtension:true — Welcome ağacı kendi içinde taşır, yeni üye
    // katılmak için gönderenin canlı state'ine (aliceGroup.ratchetTree)
    // ihtiyaç duymaz. Golden fixture'ın (joinFromWelcomeWire) çalışması için
    // zorunlu; bu akış için de zararsız (Bob zaten burada aliceGroup.ratchetTree'yi
    // doğrudan alıyor, aşağıda değişmedi).
    { extraProposals: [addProposal], ratchetTreeExtension: true }
  );
  aliceGroup = commitResult.newState;
  if (!commitResult.welcome) throw new Error("runTwoPartyMlsFlow: Commit bir Welcome üretmedi");
  zeroizeConsumed(commitResult.consumed);

  const welcomeMsg: MLSMessage = { version: "mls10", wireformat: "mls_welcome", welcome: commitResult.welcome };
  const welcomeWireB64 = encodeWire(welcomeMsg);
  const decodedWelcome = decodeWire(welcomeWireB64);
  if (decodedWelcome.wireformat !== "mls_welcome") throw new Error("runTwoPartyMlsFlow: decode edilen mesaj Welcome değil");

  let bobGroup = await joinGroup(
    decodedWelcome.welcome,
    bobKp.publicPackage,
    bobKp.privatePackage,
    emptyPskIndex,
    bobImpl,
    aliceGroup.ratchetTree,
    undefined,
    mlsClientConfig()
  );

  const msgResult = await createApplicationMessage(aliceGroup, new TextEncoder().encode(plaintext), aliceImpl);
  aliceGroup = msgResult.newState;

  const appMsg: MLSMessage = { version: "mls10", wireformat: "mls_private_message", privateMessage: msgResult.privateMessage };
  const applicationMessageWireB64 = encodeWire(appMsg);
  const decodedApp = decodeWire(applicationMessageWireB64);
  if (decodedApp.wireformat !== "mls_private_message" && decodedApp.wireformat !== "mls_public_message") {
    throw new Error(`runTwoPartyMlsFlow: decode edilen mesaj private/public message değil (${decodedApp.wireformat})`);
  }

  const processResult = await processMessage(decodedApp, bobGroup, emptyPskIndex, acceptAll, bobImpl);
  if (processResult.kind !== "applicationMessage") {
    throw new Error(`runTwoPartyMlsFlow: Bob mesajı application-message olarak işleyemedi (kind=${processResult.kind})`);
  }
  zeroizeConsumed(processResult.consumed);

  return {
    aliceKeyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: aliceKp.publicPackage }),
    bobKeyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: bobKp.publicPackage }),
    welcomeWireB64,
    applicationMessageWireB64,
    plaintext,
    decrypted: new TextDecoder().decode(processResult.message),
  };
}

// ─── Tuğla 4a — API-hazır parçalanmış fonksiyonlar ─────────────────────────
// runTwoPartyMlsFlow monolitinden çıkarıldı. Kripto AYNI (createGroup/
// createCommit/createApplicationMessage çağrıları değişmedi) — sadece adımlar
// bağımsız, tiplenmiş fonksiyonlara ayrıldı ki backend API katmanı her adımı
// ayrı ayrı tetikleyebilsin.

function toRawPrivateKeyPackage(priv: PrivateKeyPackage): RawPrivateKeyPackage {
  return {
    initPrivateKeyB64: Buffer.from(priv.initPrivateKey).toString("base64"),
    hpkePrivateKeyB64: Buffer.from(priv.hpkePrivateKey).toString("base64"),
    signaturePrivateKeyB64: Buffer.from(priv.signaturePrivateKey).toString("base64"),
  };
}

export interface OwnKeyPackage {
  keyPackageWireB64: string;
  privateKeyPackage: RawPrivateKeyPackage;
}

/** Kendi KeyPackage'ını üretir (public wire + local'de saklanacak private — bkz #10 mimari notu). */
export async function createOwnKeyPackage(did: string, cs: CiphersuiteImpl): Promise<OwnKeyPackage> {
  const credential = { credentialType: "basic" as const, identity: new TextEncoder().encode(did) };
  const kp = await generateKeyPackage(credential, defaultCapabilities(), defaultLifetime, [], cs);
  return {
    keyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: kp.publicPackage }),
    privateKeyPackage: toRawPrivateKeyPackage(kp.privatePackage),
  };
}

export interface GroupCreationResult {
  newState: ClientState;
  commitWireB64: string;
  welcomeWireB64: string;
  newEpoch: number;
}

/** Kendi KeyPackage'ından grup kurar, verilen üyeyi Add proposal ile ekler, tek Commit'te kapatır. */
export async function createGroupWithMember(
  ownKeyPackage: OwnKeyPackage,
  groupId: Uint8Array,
  memberKeyPackageWireB64: string,
  cs: CiphersuiteImpl
): Promise<GroupCreationResult> {
  const ownPublicPackage = decodeKeyPackageWire(ownKeyPackage.keyPackageWireB64);
  const ownPrivatePackage = toPrivateKeyPackage(ownKeyPackage.privateKeyPackage);

  let state = await createGroup(groupId, ownPublicPackage, ownPrivatePackage, [], cs, mlsClientConfig());

  const memberKeyPackage = decodeKeyPackageWire(memberKeyPackageWireB64);
  const addProposal = { proposalType: "add" as const, add: { keyPackage: memberKeyPackage } };
  const commitResult = await createCommit(
    { state, cipherSuite: cs },
    // ratchetTreeExtension:true — bkz. joinFromWelcomeWire'daki not, Welcome
    // kendi içinde ağaç taşımalı ki alıcı canlı state'e ihtiyaç duymasın.
    { extraProposals: [addProposal], ratchetTreeExtension: true }
  );
  state = commitResult.newState;
  if (!commitResult.welcome) throw new Error("createGroupWithMember: Commit bir Welcome üretmedi");
  zeroizeConsumed(commitResult.consumed);

  return {
    newState: state,
    commitWireB64: encodeWire(commitResult.commit),
    welcomeWireB64: encodeWire({ version: "mls10", wireformat: "mls_welcome", welcome: commitResult.welcome }),
    newEpoch: Number(state.groupContext.epoch),
  };
}

export interface EncryptedGroupMessage {
  ciphertextWireB64: string;
  epoch: number;
}

/** Grup state'inde tek bir application mesajı şifreler. Çağıran yeni state'i kendi saklamalı (epoch ilerlemedi ise mesaj tekrar şifrelenemez). */
export async function encryptGroupMessage(
  state: ClientState,
  plaintext: string,
  cs: CiphersuiteImpl
): Promise<EncryptedGroupMessage> {
  const msgResult = await createApplicationMessage(state, new TextEncoder().encode(plaintext), cs);
  const appMsg: MLSMessage = { version: "mls10", wireformat: "mls_private_message", privateMessage: msgResult.privateMessage };
  return {
    ciphertextWireB64: encodeWire(appMsg),
    epoch: Number(msgResult.newState.groupContext.epoch),
  };
}

// ─── Golden-fixture okuma yüzeyi ────────────────────────────────────────────
// Aşağıdaki fonksiyonlar runTwoPartyMlsFlow'un aksine CANLI bir gönderen
// state'ine (aliceGroup) ihtiyaç duymaz — sadece wire byte'ları + Bob'un kendi
// özel anahtar malzemesinden gruba katılıp mesaj çözebilir. Bu, gerçek cihaz
// senaryosuna (Bob, Alice'in bellek içi nesnesine değil sadece sunucudan gelen
// byte'lara erişir) daha yakın — ve golden fixture testinin ihtiyacı tam bu.

/** Bob'un ÖZEL anahtar malzemesi — wire formatı YOK (hiç ağa çıkmaz), fixture'da
 * ayrı base64 alanları olarak saklanır. */
export interface RawPrivateKeyPackage {
  initPrivateKeyB64: string;
  hpkePrivateKeyB64: string;
  signaturePrivateKeyB64: string;
}

function toPrivateKeyPackage(raw: RawPrivateKeyPackage): PrivateKeyPackage {
  return {
    initPrivateKey: new Uint8Array(Buffer.from(raw.initPrivateKeyB64, "base64")),
    hpkePrivateKey: new Uint8Array(Buffer.from(raw.hpkePrivateKeyB64, "base64")),
    signaturePrivateKey: new Uint8Array(Buffer.from(raw.signaturePrivateKeyB64, "base64")),
  };
}

function decodeKeyPackageWire(wireB64: string): KeyPackage {
  const decoded = decodeWire(wireB64);
  if (decoded.wireformat !== "mls_key_package") {
    throw new Error(`decodeKeyPackageWire: decode edilen mesaj KeyPackage değil (${decoded.wireformat})`);
  }
  return decoded.keyPackage;
}

/**
 * Bob'u SADECE wire byte'lardan (Welcome + kendi public KeyPackage'ı) + kendi
 * özel anahtarından gruba katılmış hale getirir. Welcome'ın
 * ratchetTreeExtension:true ile üretilmiş olması ZORUNLU — aksi halde ağaç
 * bilgisi eksik kalır, join başarısız olur.
 */
export async function joinFromWelcomeWire(
  welcomeWireB64: string,
  bobKeyPackageWireB64: string,
  bobPrivateKeyPackage: RawPrivateKeyPackage,
  cs: CiphersuiteImpl
): Promise<ClientState> {
  const decodedWelcome = decodeWire(welcomeWireB64);
  if (decodedWelcome.wireformat !== "mls_welcome") {
    throw new Error(`joinFromWelcomeWire: decode edilen mesaj Welcome değil (${decodedWelcome.wireformat})`);
  }
  const bobKeyPackage = decodeKeyPackageWire(bobKeyPackageWireB64);
  return joinGroup(
    decodedWelcome.welcome,
    bobKeyPackage,
    toPrivateKeyPackage(bobPrivateKeyPackage),
    emptyPskIndex,
    cs,
    // ratchetTree parametresi bilerek verilmiyor — Welcome'ın kendi içindeki
    // ratchet_tree extension'ından (ratchetTreeExtension:true) türetiliyor.
    undefined,
    undefined,
    mlsClientConfig()
  );
}

/** Bir state'e sahip üyenin (Bob) wire-encoded bir application mesajını çözer. */
export async function decryptApplicationMessageWire(
  state: ClientState,
  applicationMessageWireB64: string,
  cs: CiphersuiteImpl
): Promise<string> {
  const decoded = decodeWire(applicationMessageWireB64);
  if (decoded.wireformat !== "mls_private_message" && decoded.wireformat !== "mls_public_message") {
    throw new Error(`decryptApplicationMessageWire: decode edilen mesaj private/public message değil (${decoded.wireformat})`);
  }
  const result = await processMessage(decoded, state, emptyPskIndex, acceptAll, cs);
  if (result.kind !== "applicationMessage") {
    throw new Error(`decryptApplicationMessageWire: application-message olarak işlenemedi (kind=${result.kind})`);
  }
  zeroizeConsumed(result.consumed);
  return new TextDecoder().decode(result.message);
}
