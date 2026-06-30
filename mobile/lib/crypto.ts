import "react-native-get-random-values";
import { x25519 } from "@noble/curves/ed25519";
import { gcm } from "@noble/ciphers/aes";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha256";

const HKDF_SALT = new TextEncoder().encode("obscura_v1_salt");
const HKDF_INFO = new TextEncoder().encode("obscura_v1_msg");

export interface EncryptedEnvelope {
  v: 1;
  eph: string; // base64 ephemeral X25519 public key
  n: string;   // base64 AES-GCM nonce (12 bytes)
  ct: string;  // base64 ciphertext + 16-byte auth tag
}

export function generateX25519KeyPair(): { privateKey: Uint8Array; publicKey: Uint8Array } {
  const { secretKey: privateKey, publicKey } = x25519.keygen();
  return { privateKey, publicKey };
}

export function x25519PublicKey(privateKey: Uint8Array): Uint8Array {
  return x25519.getPublicKey(privateKey);
}

export function encryptForRecipient(plaintext: string, recipientPublicKey: Uint8Array): EncryptedEnvelope {
  const { secretKey: ephPriv, publicKey: ephPub } = x25519.keygen();
  const shared = x25519.getSharedSecret(ephPriv, recipientPublicKey);
  const key = hkdf(sha256, shared, HKDF_SALT, HKDF_INFO, 32);
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(key, nonce).encrypt(new TextEncoder().encode(plaintext));
  return {
    v: 1,
    eph: u8ToBase64(ephPub),
    n: u8ToBase64(nonce),
    ct: u8ToBase64(ct),
  };
}

export function decryptFromEnvelope(envelope: EncryptedEnvelope, recipientPrivateKey: Uint8Array): string {
  const ephPub = base64ToU8(envelope.eph);
  const shared = x25519.getSharedSecret(recipientPrivateKey, ephPub);
  const key = hkdf(sha256, shared, HKDF_SALT, HKDF_INFO, 32);
  const nonce = base64ToU8(envelope.n);
  const ct = base64ToU8(envelope.ct);
  const plain = gcm(key, nonce).decrypt(ct);
  return new TextDecoder().decode(plain);
}

export function parseEnvelope(raw: string): EncryptedEnvelope | null {
  try {
    const obj = JSON.parse(raw);
    if (obj?.v === 1 && typeof obj.eph === "string" && typeof obj.n === "string" && typeof obj.ct === "string") {
      return obj as EncryptedEnvelope;
    }
    return null;
  } catch {
    return null;
  }
}

export function u8ToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

export function base64ToU8(b64: string): Uint8Array {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

export function hexToU8(hex: string): Uint8Array {
  const arr = new Uint8Array(hex.length / 2);
  for (let i = 0; i < arr.length; i++) arr[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return arr;
}

export function u8ToHex(bytes: Uint8Array): string {
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, "0")).join("");
}
