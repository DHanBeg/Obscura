import { describe, it, expect, beforeEach, vi } from "vitest";
import { gcm } from "@noble/ciphers/aes.js";
import {
  generateX25519,
  generateEd25519,
  ed25519Sign,
  ed25519Verify,
  aesEncrypt,
  aesDecrypt,
  getOrCreateIdentity,
  toB64,
} from "./e2ee";

const IDENTITY_KEY = "obscura_identity_v1";

describe("web EC primitifi: P-256 → X25519/Ed25519 (@noble)", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  // Test 1 — turun sebebi: P-256 raw pubkey 65 byte'tı, backend 32 bekliyor.
  it("X25519 raw pubkey 32 byte (P-256'nın 65'i değil)", async () => {
    const pair = await generateX25519();
    expect(pair.publicKeyBytes.length).toBe(32);
    expect(pair.privateKey.length).toBe(32);
  });

  // Test 3 — eski P-256 formatlı kayıt sessizce "geçerli" sanılmamalı.
  // Eski kayıtta "format" alanı yoktu. Length guard'ı ayrıca izole etmek için
  // sahte eski kayıt bilerek 32'şer byte alanlarla kuruluyor: yalnız format
  // marker'ı reddi sağlayabilir.
  it("eski (format marker'sız) kayıt varken getOrCreateIdentity eskiyi reddeder, yeni 32 byte kimlik üretir", async () => {
    const passphrase = "obscura_+905550000000_v1";
    const enc = new TextEncoder();

    const legacyKeys = {
      dhPriv: toB64(crypto.getRandomValues(new Uint8Array(32))),
      dhPub: toB64(crypto.getRandomValues(new Uint8Array(32))),
      sigPriv: toB64(crypto.getRandomValues(new Uint8Array(32))),
      sigPub: toB64(crypto.getRandomValues(new Uint8Array(32))),
      did: "did:obs:legacyp256identity0000000000000000",
    };

    const salt = crypto.getRandomValues(new Uint8Array(16));
    const km = await crypto.subtle.importKey("raw", enc.encode(passphrase), "PBKDF2", false, ["deriveBits"]);
    const bits = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt, iterations: 100000 }, km, 256);
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const ct = gcm(new Uint8Array(bits), iv).encrypt(enc.encode(JSON.stringify(legacyKeys)));
    localStorage.setItem(IDENTITY_KEY, JSON.stringify({ salt: toB64(salt), iv: toB64(iv), ct: toB64(ct) }));

    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const identity = await getOrCreateIdentity(passphrase);

    expect(identity.did).not.toBe(legacyKeys.did);
    expect(identity.dhKeyPair.publicKeyBytes.length).toBe(32);
    expect(toB64(identity.dhKeyPair.publicKeyBytes)).not.toBe(legacyKeys.dhPub);
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("eski formatta"));
  });

  // Test 4 — Ed25519 sign→verify, 32 byte anahtarlar.
  it("Ed25519 sign→verify round-trip (32 byte anahtar), yanlış mesaj reddedilir", async () => {
    const kp = await generateEd25519();
    expect(kp.publicKeyBytes.length).toBe(32);
    const msg = new TextEncoder().encode("spk-public-bytes");
    const sig = await ed25519Sign(kp.privateKey, msg);
    expect(sig.length).toBe(64);
    expect(await ed25519Verify(kp.publicKeyBytes, msg, sig)).toBe(true);
    expect(await ed25519Verify(kp.publicKeyBytes, new TextEncoder().encode("baska"), sig)).toBe(false);
  });

  // Test 5 — AES-GCM ham byte key/nonce/aad (@noble/ciphers), CryptoKey yok.
  it("AES-GCM round-trip (ham 32 byte key, aad), yanlış aad reddedilir", async () => {
    const key = crypto.getRandomValues(new Uint8Array(32));
    const aad = new TextEncoder().encode("ad");
    const pt = new TextEncoder().encode("merhaba x25519");
    const blob = await aesEncrypt(key, pt, aad);
    expect(new TextDecoder().decode(await aesDecrypt(key, blob, aad))).toBe("merhaba x25519");
    await expect(aesDecrypt(key, blob, new TextEncoder().encode("yanlis"))).rejects.toThrow();
  });
});
