import * as SecureStore from "expo-secure-store";
import {
  generateX25519KeyPair,
  x25519PublicKey,
  encryptForRecipient,
  decryptFromEnvelope,
  parseEnvelope,
  u8ToBase64,
  base64ToU8,
  hexToU8,
  u8ToHex,
} from "./crypto";

const KEY_PRIV = "obscura_x25519_private";
const KEY_PUB = "obscura_x25519_public";

export async function getOrCreateKeyPair(): Promise<{ privateKey: Uint8Array; publicKey: Uint8Array }> {
  const privHex = await SecureStore.getItemAsync(KEY_PRIV);
  if (privHex) {
    const privateKey = hexToU8(privHex);
    return { privateKey, publicKey: x25519PublicKey(privateKey) };
  }
  const { privateKey, publicKey } = generateX25519KeyPair();
  await SecureStore.setItemAsync(KEY_PRIV, u8ToHex(privateKey));
  await SecureStore.setItemAsync(KEY_PUB, u8ToHex(publicKey));
  return { privateKey, publicKey };
}

export async function getIdentityPublicKeyBase64(): Promise<string> {
  const { publicKey } = await getOrCreateKeyPair();
  return u8ToBase64(publicKey);
}

export async function encryptMessage(plaintext: string, recipientPublicKeyBase64: string): Promise<string> {
  const recipientPub = base64ToU8(recipientPublicKeyBase64);
  const envelope = encryptForRecipient(plaintext, recipientPub);
  return JSON.stringify(envelope);
}

export async function decryptMessage(raw: string): Promise<string> {
  const envelope = parseEnvelope(raw);
  if (!envelope) return raw; // plaintext fallback for legacy messages
  try {
    const { privateKey } = await getOrCreateKeyPair();
    return decryptFromEnvelope(envelope, privateKey);
  } catch {
    return "🔒 Şifresi çözülemeyen mesaj";
  }
}
