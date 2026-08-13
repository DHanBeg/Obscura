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
} from "ts-mls";

const CIPHERSUITE_NAME = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519";

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
    aliceImpl
  );

  const addProposal = { proposalType: "add" as const, add: { keyPackage: bobKp.publicPackage } };
  const commitResult = await createCommit(
    { state: aliceGroup, cipherSuite: aliceImpl },
    { extraProposals: [addProposal] }
  );
  aliceGroup = commitResult.newState;
  if (!commitResult.welcome) throw new Error("runTwoPartyMlsFlow: Commit bir Welcome üretmedi");

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
    aliceGroup.ratchetTree
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

  return {
    aliceKeyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: aliceKp.publicPackage }),
    bobKeyPackageWireB64: encodeWire({ version: "mls10", wireformat: "mls_key_package", keyPackage: bobKp.publicPackage }),
    welcomeWireB64,
    applicationMessageWireB64,
    plaintext,
    decrypted: new TextDecoder().decode(processResult.message),
  };
}
