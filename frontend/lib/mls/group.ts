// B10 Faz 1 — mobile/lib/mls/group.ts'nin web alt-kümesi. KAPSAM SINIRI
// (B10.2 Tuğla 2'den itibaren): grup CREATE-half burada VAR (createOwnGroup,
// aşağıda) — ts-mls createGroup ile epoch-0, tek-yapraklı (yalnız creator)
// state üretir. ADD-half (createCommit/addMember, üye ekleme) HÂLÂ YOK —
// Tuğla 3'e kadar web'den kurulan bir gruba üye eklenemez, yalnız var olan
// bir gruba (mobil daveti üzerinden) katılıp mesaj gönderip/alabilir. Kripto
// AYNI (ts-mls, X25519 suite, nobleCryptoProvider ZORUNLU — WebCrypto X25519
// desteklemiyor, Faz 0.5 spike'ta doğrulandı).
"use client";
import {
  getCiphersuiteFromName,
  getCiphersuiteImpl,
  nobleCryptoProvider,
  generateKeyPackage,
  createGroup,
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

export function zeroizeConsumed(consumed: Uint8Array[]): void {
  for (const buf of consumed) buf.fill(0);
}

/** mobile/lib/mls/group.ts:getMlsCiphersuiteImpl ile AYNI — nobleCryptoProvider
 * ZORUNLU (X25519, defaultCryptoProvider'da desteklenmiyor, WebCrypto tabanlı). */
export async function getMlsCiphersuiteImpl(): Promise<CiphersuiteImpl> {
  return getCiphersuiteImpl(getCiphersuiteFromName(CIPHERSUITE_NAME), nobleCryptoProvider);
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

function toRawPrivateKeyPackage(priv: PrivateKeyPackage): RawPrivateKeyPackage {
  return {
    initPrivateKeyB64: Buffer.from(priv.initPrivateKey).toString("base64"),
    hpkePrivateKeyB64: Buffer.from(priv.hpkePrivateKey).toString("base64"),
    signaturePrivateKeyB64: Buffer.from(priv.signaturePrivateKey).toString("base64"),
  };
}

function decodeKeyPackageWire(wireB64: string): KeyPackage {
  const decoded = decodeWire(wireB64);
  if (decoded.wireformat !== "mls_key_package") {
    throw new Error(`decodeKeyPackageWire: decode edilen mesaj KeyPackage değil (${decoded.wireformat})`);
  }
  return decoded.keyPackage;
}

export interface OwnKeyPackage {
  keyPackageWireB64: string;
  privateKeyPackage: RawPrivateKeyPackage;
}

/** Kendi KeyPackage'ını üretir — mobile'ın davet-edilebilirlik akışıyla aynı
 * (bkz. inviteBootstrap.ts): web kullanıcısı bunu backend'e yükler ki mobil
 * onu bir gruba EKLEYEBİLSİN (web kendi grup kuramaz ama davet edilebilir). */
export async function createOwnKeyPackage(did: string, cs: CiphersuiteImpl): Promise<OwnKeyPackage> {
  const credential = { credentialType: "basic" as const, identity: new TextEncoder().encode(did) };
  const kp = await generateKeyPackage(credential, defaultCapabilities(), defaultLifetime, [], cs);
  return {
    keyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: kp.publicPackage }),
    privateKeyPackage: toRawPrivateKeyPackage(kp.privatePackage),
  };
}

/** CREATE-half — mobile/lib/mls/group.ts:createGroupWithMember'ın İLK
 * yarısıyla AYNI ts-mls çağrısı (o fonksiyonun addMemberToGroup'a devrettiği
 * kısım burada YOK — bkz. dosya üstü not, B10.2 Tuğla 2 kapsam sınırı).
 * Boş üye listesiyle ([]) epoch-0, tek-yapraklı (yalnız creator) bir grup
 * state'i üretir. addMember/Welcome/commit ÇAĞIRMAZ — Tuğla 3'e kadar bu
 * state'e kimse eklenemez. */
export async function createOwnGroup(
  groupIdBytes: Uint8Array,
  ownKeyPackage: OwnKeyPackage,
  cs: CiphersuiteImpl
): Promise<ClientState> {
  const ownPublicPackage = decodeKeyPackageWire(ownKeyPackage.keyPackageWireB64);
  const ownPrivatePackage = toPrivateKeyPackage(ownKeyPackage.privateKeyPackage);
  return createGroup(groupIdBytes, ownPublicPackage, ownPrivatePackage, [], cs, mlsClientConfig());
}

/** Bir Welcome'dan (SADECE wire byte'lardan) + kendi özel anahtarından gruba
 * katılır — mobile/lib/mls/group.ts:joinFromWelcomeWire ile AYNI. */
export async function joinFromWelcomeWire(
  welcomeWireB64: string,
  ownKeyPackageWireB64: string,
  ownPrivateKeyPackage: RawPrivateKeyPackage,
  cs: CiphersuiteImpl
): Promise<ClientState> {
  const decodedWelcome = decodeWire(welcomeWireB64);
  if (decodedWelcome.wireformat !== "mls_welcome") {
    throw new Error(`joinFromWelcomeWire: decode edilen mesaj Welcome değil (${decodedWelcome.wireformat})`);
  }
  const ownKeyPackage = decodeKeyPackageWire(ownKeyPackageWireB64);
  return joinGroup(
    decodedWelcome.welcome,
    ownKeyPackage,
    toPrivateKeyPackage(ownPrivateKeyPackage),
    emptyPskIndex,
    cs,
    // ratchetTree parametresi bilerek verilmiyor — Welcome'ın kendi içindeki
    // ratchet_tree extension'ından türetiliyor (mobile ile aynı).
    undefined,
    undefined,
    mlsClientConfig()
  );
}

export interface EncryptedGroupMessage {
  ciphertextWireB64: string;
  epoch: number;
  // newState kaydedilmezse bir sonraki encryptGroupMessage AYNI ratchet
  // generation'dan şifreler → nonce/key reuse (mobile group.ts'teki aynı not).
  newState: ClientState;
}

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
    newState: msgResult.newState,
  };
}

export interface DecryptedApplicationMessage {
  plaintext: string;
  newState: ClientState;
}

/** processMessage state referansını YERİNDE mutasyona uğratıyor — aynı state
 * referansı ikinci mesaj için tekrar kullanılamaz (mobile'da bulunan aynı
 * kısıt). Çağıran newState'i açıkça ileri taşımalı (bkz. groupChat.ts). */
export async function decryptApplicationMessageWireWithState(
  state: ClientState,
  applicationMessageWireB64: string,
  cs: CiphersuiteImpl
): Promise<DecryptedApplicationMessage> {
  const decoded = decodeWire(applicationMessageWireB64);
  if (decoded.wireformat !== "mls_private_message" && decoded.wireformat !== "mls_public_message") {
    throw new Error(`decryptApplicationMessageWireWithState: decode edilen mesaj private/public message değil (${decoded.wireformat})`);
  }
  const result = await processMessage(decoded, state, emptyPskIndex, acceptAll, cs);
  if (result.kind !== "applicationMessage") {
    throw new Error(`decryptApplicationMessageWireWithState: application-message olarak işlenemedi (kind=${result.kind})`);
  }
  zeroizeConsumed(result.consumed);
  return { plaintext: new TextDecoder().decode(result.message), newState: result.newState };
}
