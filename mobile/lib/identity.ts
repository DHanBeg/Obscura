import "react-native-get-random-values";
import { ed25519 } from "@noble/curves/ed25519.js";
import { u8ToBase64, u8ToHex, hexToU8 } from "./crypto";
import { secureStore, KeyValueStore } from "./keyValueStore";

// Ed25519 signing keypair — e2e.ts'teki X25519 DH identity key'den AYRI.
// X25519 anahtar sadece key-agreement (X3DH DH1-DH4) için; bu Ed25519
// anahtar SPK'ları imzalamak için (Signal X3DH spec: IK_ed25519 → Sig(SPK)).
const KEY_SIGNING_PRIV = "obscura_ed25519_signing_private";
const KEY_SIGNING_PUB = "obscura_ed25519_signing_public";

export function generateSigningKeyPair(): { privateKey: Uint8Array; publicKey: Uint8Array } {
  const { secretKey: privateKey, publicKey } = ed25519.keygen();
  return { privateKey, publicKey };
}

export function signingPublicKey(privateKey: Uint8Array): Uint8Array {
  return ed25519.getPublicKey(privateKey);
}

export function signWithKey(privateKey: Uint8Array, message: Uint8Array): Uint8Array {
  return ed25519.sign(message, privateKey);
}

export function verifyWithKey(publicKey: Uint8Array, message: Uint8Array, signature: Uint8Array): boolean {
  return ed25519.verify(signature, message, publicKey);
}

export async function getOrCreateSigningKeyPair(
  store: KeyValueStore = secureStore
): Promise<{ privateKey: Uint8Array; publicKey: Uint8Array }> {
  const privHex = await store.getItem(KEY_SIGNING_PRIV);
  if (privHex) {
    const privateKey = hexToU8(privHex);
    return { privateKey, publicKey: signingPublicKey(privateKey) };
  }
  const { privateKey, publicKey } = generateSigningKeyPair();
  await store.setItem(KEY_SIGNING_PRIV, u8ToHex(privateKey));
  await store.setItem(KEY_SIGNING_PUB, u8ToHex(publicKey));
  return { privateKey, publicKey };
}

export async function getSigningPublicKeyBase64(store: KeyValueStore = secureStore): Promise<string> {
  const { publicKey } = await getOrCreateSigningKeyPair(store);
  return u8ToBase64(publicKey);
}

export async function signBytes(message: Uint8Array, store: KeyValueStore = secureStore): Promise<Uint8Array> {
  const { privateKey } = await getOrCreateSigningKeyPair(store);
  return signWithKey(privateKey, message);
}
