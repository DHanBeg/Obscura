// B10 Faz 1 — mobile/lib/session-store.ts + mobile/lib/crypto.ts'nin
// mls-store.ts'in kullandığı 3 saf fonksiyonunun (RN'e bağımlı OLMAYAN)
// birebir portu: getOrCreateMasterKey/encryptBlob/decryptBlob + hex helper'lar.
// KRİPTO/MANTIK DEĞİŞMEDİ — sadece "react-native-get-random-values" polyfill
// import'u atlandı (tarayıcı native crypto.getRandomValues zaten sağlıyor,
// bu polyfill sadece eski RN JSC motorları için gerekliydi).
"use client";
import { gcm } from "@noble/ciphers/aes.js";
import type { KeyValueStore } from "./webStore";

export function hexToU8(hex: string): Uint8Array {
  const arr = new Uint8Array(hex.length / 2);
  for (let i = 0; i < arr.length; i++) arr[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return arr;
}

export function u8ToHex(bytes: Uint8Array): string {
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, "0")).join("");
}

// storeKey parametreli: mls-store.ts KENDİ master key namespace'iyle
// (mobile ile aynı ilke — bkz. mobile/lib/session-store.ts:37-40) yeniden
// kullanır, biri sızarsa diğeri etkilenmez.
export async function getOrCreateMasterKey(secStore: KeyValueStore, storeKey: string): Promise<Uint8Array> {
  const hex = await secStore.getItem(storeKey);
  if (hex) return hexToU8(hex);
  const key = new Uint8Array(32);
  crypto.getRandomValues(key);
  await secStore.setItem(storeKey, u8ToHex(key));
  return key;
}

export function encryptBlob(masterKey: Uint8Array, plaintext: Uint8Array): Uint8Array {
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(masterKey, nonce).encrypt(plaintext);
  const out = new Uint8Array(12 + ct.length);
  out.set(nonce, 0);
  out.set(ct, 12);
  return out;
}

export function decryptBlob(masterKey: Uint8Array, blob: Uint8Array): Uint8Array {
  if (blob.length < 28) throw new Error("Grup verisi bozuk (çok kısa)");
  const nonce = blob.slice(0, 12);
  const ct = blob.slice(12);
  try {
    return gcm(masterKey, nonce).decrypt(ct);
  } catch {
    throw new Error("Grup verisi şifresi çözülemedi — bütünlük bozulmuş veya master key yanlış");
  }
}
