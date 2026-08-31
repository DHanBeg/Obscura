// B10.2 Tuğla 2 — mobile/lib/mls/groupId.ts'nin web portu. Byte üretimi
// BİREBİR AYNI (32 byte, kriptografik RNG, aynı URL-safe base64 kodlaması)
// — çapraz-uyum için: web'de kurulan bir gruba mobile client'ı (veya tersi)
// katılabilmeli, ikisi de AYNI group_id şeklini üretip/okuyabilmeli.
//
// Mobile'daki "react-native-get-random-values" polyfill'i BURADA YOK —
// o polyfill RN'in eksik global crypto.getRandomValues'ını doldurmak için
// var; web (tarayıcı) ortamında Web Crypto API zaten native (bkz.
// tsconfig.json "lib": ["dom", ...], crypto.getRandomValues DOM tipinde
// tanımlı). Ekstra polyfill import'u web'de gereksiz/yanlış paket.
"use client";

const GROUP_ID_BYTES = 32;

function base64UrlEncode(bytes: Uint8Array): string {
  const std = Buffer.from(bytes).toString("base64");
  return std.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export interface GeneratedGroupId {
  /** ts-mls createGroup'un beklediği ham byte'lar. */
  bytes: Uint8Array;
  /** Backend'e (mls_group_id, URL path segmenti) ve api.mls* çağrılarına giden kimlik. */
  b64: string;
}

/** Rastgele, benzersiz bir MLS group_id üretir (32 byte, kriptografik RNG). */
export function generateGroupId(): GeneratedGroupId {
  const bytes = new Uint8Array(GROUP_ID_BYTES);
  crypto.getRandomValues(bytes);
  return { bytes, b64: base64UrlEncode(bytes) };
}
