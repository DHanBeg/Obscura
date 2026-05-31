/**
 * BIP39 Mnemonic Yedekleme Yardımcıları — Mobile (Expo/React Native)
 *
 * Spec Bölüm 5.2 Adım 7: Kayıt sonrası 12 kelime mnemonic üretimi,
 * ZK-ID secret türetimi, doğrulama.
 *
 * localStorage yerine expo-secure-store kullanılır.
 * Kural: Secret hiçbir zaman network'e gönderilmez.
 */

import { generateMnemonic as _generate, mnemonicToSeed } from "@scure/bip39";
import { wordlist } from "@scure/bip39/wordlists/english";
import * as SecureStore from "expo-secure-store";

// ─── Üretim ───────────────────────────────────────────────────────────────────

/**
 * 12 kelimelik BIP39 mnemonic üret (128-bit entropi).
 */
export function generateMnemonic12(): string {
  return _generate(wordlist, 128);
}

// ─── Türetim ──────────────────────────────────────────────────────────────────

/**
 * Mnemonic'den ZK-ID secret türet.
 * Sonuç: 64 hex karakter (256-bit).
 * Spec: secret client'ta saklanır, backend görmez.
 */
export async function deriveSecretFromMnemonic(mnemonic: string): Promise<string> {
  const seed = await mnemonicToSeed(mnemonic);
  const slice = seed.slice(0, 32);
  const hex: string[] = [];
  for (let i = 0; i < slice.length; i++) {
    hex.push(slice[i].toString(16).padStart(2, "0"));
  }
  return hex.join("");
}

// ─── Yardımcılar ──────────────────────────────────────────────────────────────

/**
 * Mnemonic'i kelime dizisine çevir.
 */
export function getMnemonicWords(mnemonic: string): string[] {
  return mnemonic.trim().split(/\s+/);
}

/**
 * Doğrulama için 3 rastgele kelimeyi sıralı olarak seç.
 * Döner: [{index: 0-based, word: string}] — index'e göre sıralı.
 */
export function pickVerificationWords(
  mnemonic: string
): { index: number; word: string }[] {
  const words = getMnemonicWords(mnemonic);
  const picked = new Set<number>();
  while (picked.size < 3) {
    picked.add(Math.floor(Math.random() * 12));
  }
  return Array.from(picked)
    .sort((a, b) => a - b)
    .map((i) => ({ index: i, word: words[i] }));
}

// ─── SecureStore helpers ──────────────────────────────────────────────────────

export async function saveZkSecret(secret: string): Promise<void> {
  await SecureStore.setItemAsync("obscura_zk_secret", secret);
}

export async function loadZkSecret(): Promise<string | null> {
  return SecureStore.getItemAsync("obscura_zk_secret");
}

export async function markMnemonicBackedUp(): Promise<void> {
  await SecureStore.setItemAsync("obscura_mnemonic_backed_up", "true");
}

export async function isMnemonicBackedUp(): Promise<boolean> {
  const val = await SecureStore.getItemAsync("obscura_mnemonic_backed_up");
  return val === "true";
}
