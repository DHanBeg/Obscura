import { asyncStore, KeyValueStore } from "./keyValueStore";

// Unseal() sırasında sertifikadan çıkan, kriptografik olarak KANITLANMIŞ
// gönderen kimliği — madde 4 (Bölüm 2.3 TIER A kanıt doğrulama, adım 8)
// için gerekli ham malzeme: bir şikayet anında bu kayıt sunucuya "bu mesajı
// gerçekten bu DID gönderdi" iddiasının imzalı kanıtı olarak sunulur.
//
// Şifrelenmiş SAKLANMIYOR (plaintext-cache.ts'in aksine): senderDid zaten
// server'ın kendi verdiği from_did ile aynı bilgiyi taşır (yeni bir gizlilik
// kaybı yok) ve sertifika imzası da alıcının cihazında zaten AÇIK biçimde
// var olan bir veri (Unseal sonucu) — cihaz ele geçirilirse zaten her şey
// görünür. Buradaki tek amaç KALICILIK: adım 8'de "bu şikayet anında kanıt
// sun" akışı çalışabilsin diye, tek seferlik Unseal çıktısını unutmadan
// saklamak (mesaj listesi yeniden yüklendiğinde ikinci kez Unseal
// ÇAĞRILMAYABİLİR — ör. ratchet mesaj anahtarı tükenmişse iç payload ikinci
// kez çözülemez, bkz. plaintext-cache.ts; sertifika de aynı sebeple kaybolmasın).
const CACHE_KEY_PREFIX = "obscura_sender_attestation_";

// Adım 8: `UnsealedMessage` artık sertifikanın ham Ed25519 imzasını ve
// expiresAt'ını da dışa veriyor (sealed-sender.ts + sealed_sender.rs) — bu
// yüzden burada gerçek "imzalı kanıt" saklanabiliyor. Bir şikayet anında bu
// dört alan (identityDhPubHex/signingPubHex/expiresAt/signatureHex) sunucuya
// gönderilir; sunucu (backend/internal/moderation/evidence.go) signing_bytes'ı
// bağımsızca yeniden hesaplayıp imzayı doğrular — plaintext from_did'e
// ihtiyaç duymadan.
export interface SenderAttestation {
  senderDid: string;
  senderIdentityDhPubHex: string;
  senderSigningPubHex: string;
  senderCertSignatureHex: string;
  senderCertExpiresAt: number;
}

export interface AttestationStores {
  async?: KeyValueStore;
}

function attestationStoreKey(messageId: string): string {
  return `${CACHE_KEY_PREFIX}${messageId}`;
}

export async function cacheSenderAttestation(
  messageId: string,
  attestation: SenderAttestation,
  stores: AttestationStores = {}
): Promise<void> {
  const asyStore = stores.async ?? asyncStore;
  await asyStore.setItem(attestationStoreKey(messageId), JSON.stringify(attestation));
}

// Kayıt yoksa VEYA bozuksa null döner — fail-open (bkz. plaintext-cache.ts
// getCachedPlaintext aynı gerekçe): eksik kanıt mesajlaşmayı KİLİTLEMEMELİ,
// sadece o mesaj için ileride şikayet kanıtı sunulamaz.
export async function getSenderAttestation(
  messageId: string,
  stores: AttestationStores = {}
): Promise<SenderAttestation | null> {
  const asyStore = stores.async ?? asyncStore;
  const raw = await asyStore.getItem(attestationStoreKey(messageId));
  if (!raw) return null;
  try {
    return JSON.parse(raw) as SenderAttestation;
  } catch {
    return null;
  }
}
