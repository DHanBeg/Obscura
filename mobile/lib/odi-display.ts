// ODI (Obscura Display Identifier) gösterim yardımcıları.
//
// DID hiçbir ekranda ham gösterilmez (backend internal/identity/odi.go,
// aynı ilke). Ağ/API çağrıları (mesaj gönderme, arama, sohbet oluşturma vb.)
// DID kullanmaya devam eder — bu modül SADECE gösterim katmanı içindir.

import { sha256 } from "@noble/hashes/sha256";

const CROCKFORD_ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

// deriveOdiFromDid — backend internal/identity/odi.go DeriveODI'nin bire bir
// JS portu (SHA-256 → ilk 60 bit → Crockford Base32, ODI-XXXX-XXXX-XXXX).
// NFC eşleştirme gibi offline akışlarda (backend'e sormadan) sadece EKRANDA
// göstermek için kullanılır — payload/tag/deep-link hâlâ DID taşır, bu
// fonksiyon o veriyi değiştirmez, sadece görüntüleme metnini üretir.
// Golden vector'lar gerçek Go DeriveODI() çalıştırılarak üretildi (bkz.
// odi-display.test.ts) — x3dh/sealed-sender vector-crosscheck testleriyle
// aynı desen.
export function deriveOdiFromDid(did: string): string {
  const sum = sha256(new TextEncoder().encode(did));
  let symbols = "";
  let bitBuf = 0;
  let bitCount = 0;
  for (let i = 0; i < sum.length && symbols.length < 12; i++) {
    bitBuf = (bitBuf << 8) | sum[i];
    bitCount += 8;
    while (bitCount >= 5 && symbols.length < 12) {
      bitCount -= 5;
      const idx = (bitBuf >>> bitCount) & 0x1f;
      symbols += CROCKFORD_ALPHABET[idx];
    }
  }
  return `ODI-${symbols.slice(0, 4)}-${symbols.slice(4, 8)}-${symbols.slice(8, 12)}`;
}

// backend internal/api/extra_handlers.go sanitizeDisplayName ile birebir
// aynı karakter aralığı (RTL/LTR override + isolate + zero-width).
const BIDI_CONTROL_CHARS = /[​-‏‪-‮⁦-⁩]/g;

export function displayIdentifier(
  entity?: { odi?: string; did?: string } | null
): string {
  return entity?.odi || entity?.did || "";
}

export function sanitizeNickname(raw: string): string {
  return raw.replace(BIDI_CONTROL_CHARS, "").trim();
}

export function isValidNickname(raw: string): boolean {
  return sanitizeNickname(raw).length > 0;
}
