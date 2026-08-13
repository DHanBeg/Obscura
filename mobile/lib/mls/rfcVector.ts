// RFC 9420 resmi test vektörleriyle ts-mls'i doğrulamak için destek —
// SADECE passive-client-welcome kategorisi (bkz. docs/adr/0019-mobile-mls-ts-port.md
// fizibilite notu: key-schedule/treekem gibi diğer kategoriler iç-secret
// istiyor, ts-mls'in published paketiyle okunamaz; passive-client-welcome
// public API'yle — Welcome işle, katıl — çalışan tek kategori).
//
// mlswg/mls-implementations test-vectors.md ("Passive Client Scenarios")
// dokümantasyonu "key_package: serialized KeyPackage" / "welcome: serialized
// MLSMessage (Welcome)" diyor — ikisi de aslında MLSMessage ZARFINA sarılı
// (barrel'ın decodeMlsMessage'ı ile çözülüyor, ts-mls'in KENDİ
// test/test-vectors/passiveClientScenarios.test.ts'i bunu doğruladı). SADECE
// "ratchet_tree" alanı gerçekten ham (wrapper'sız) struct — deep-path
// decodeRatchetTree gerekiyor, barrel bunu dışa vermiyor. Relative path
// kullanılıyor çünkü moduleNameMapper/extraNodeModules SADECE bare "ts-mls"
// specifier'ını eşliyor, deep-path'i değil.
import { decodeRatchetTree } from "../../../vendor/ts-mls/node_modules/ts-mls/dist/src/ratchetTree.js";
import {
  joinGroup,
  makePskIndex,
  bytesToBase64,
  decodeMlsMessage,
  type PrivateKeyPackage,
  type CiphersuiteImpl,
  type ClientState,
  type RatchetTree,
} from "ts-mls";

/** mlswg/mls-implementations `test-vectors/passive-client-welcome.json`
 * satırının şeması (kullandığımız alanlarla sınırlı). */
export interface PassiveClientWelcomeVector {
  cipher_suite: number;
  external_psks: { psk_id: string; psk: string }[];
  key_package: string;
  signature_priv: string;
  encryption_priv: string;
  init_priv: string;
  welcome: string;
  ratchet_tree: string | null;
  initial_epoch_authenticator: string;
}

function hexToBytes(hex: string): Uint8Array {
  return new Uint8Array(Buffer.from(hex, "hex"));
}

/**
 * Vektördeki pasif üyeyi (kendi private key'i + public KeyPackage'ı + Welcome)
 * SADECE vektör byte'larından gruba katılmış hale getirir. `ratchet_tree`
 * alanı null'sa Welcome kendi ağacını taşıyor sayılır (ratchetTree parametresi
 * verilmez); doluysa ayrı decode edilip joinGroup'a açıkça geçilir.
 */
export async function joinFromRfcVector(
  vector: PassiveClientWelcomeVector,
  cs: CiphersuiteImpl
): Promise<ClientState> {
  const kpMsg = decodeMlsMessage(hexToBytes(vector.key_package), 0)?.[0];
  if (!kpMsg || kpMsg.wireformat !== "mls_key_package") {
    throw new Error(`joinFromRfcVector: vektörün key_package'ı beklenen MLSMessage(mls_key_package) değil (${kpMsg?.wireformat})`);
  }
  const keyPackage = kpMsg.keyPackage;

  const welcomeMsg = decodeMlsMessage(hexToBytes(vector.welcome), 0)?.[0];
  if (!welcomeMsg || welcomeMsg.wireformat !== "mls_welcome") {
    throw new Error(`joinFromRfcVector: vektörün welcome'ı beklenen MLSMessage(mls_welcome) değil (${welcomeMsg?.wireformat})`);
  }

  const privateKeyPackage: PrivateKeyPackage = {
    initPrivateKey: hexToBytes(vector.init_priv),
    hpkePrivateKey: hexToBytes(vector.encryption_priv),
    signaturePrivateKey: hexToBytes(vector.signature_priv),
  };

  let ratchetTree: RatchetTree | undefined;
  if (vector.ratchet_tree) {
    const treeDecoded = decodeRatchetTree(hexToBytes(vector.ratchet_tree), 0);
    if (!treeDecoded) throw new Error("joinFromRfcVector: vektörün ratchet_tree'si decode edilemedi");
    ratchetTree = treeDecoded[0];
  }

  // Bazı vektör satırları external PSK proposal'ı içeriyor — makePskIndex'in
  // beklediği key formatı bytesToBase64(pskId) (clientState.js:505), hex DEĞİL.
  const externalPsks: Record<string, Uint8Array> = {};
  for (const { psk_id, psk } of vector.external_psks) {
    externalPsks[bytesToBase64(hexToBytes(psk_id))] = hexToBytes(psk);
  }
  const pskIndex = makePskIndex(undefined, externalPsks);

  return joinGroup(welcomeMsg.welcome, keyPackage, privateKeyPackage, pskIndex, cs, ratchetTree);
}
