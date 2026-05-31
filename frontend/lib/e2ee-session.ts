/**
 * Obscura E2EE Session Manager
 * Konuşma başına Double Ratchet oturumu yönetir.
 * Ratchet state'ini localStorage'da seri hale getirir.
 */
"use client";

import {
  x3dhInitiate,
  ratchetInitSender,
  ratchetInitReceiver,
  ratchetEncrypt,
  ratchetDecrypt,
  generateX25519,
  x25519DH,
  toB64,
  fromB64,
  type IdentityKeys,
  type RatchetState,
  type PreKeyBundle,
  type RatchetMessage,
} from "./e2ee";

const SESSION_PREFIX = "obscura_session_v1_";

// ── Serialization ──────────────────────────────────────────────────────────

/** CryptoKey export helpers */
async function exportPrivKey(key: CryptoKey): Promise<string> {
  try {
    const raw = await window.crypto.subtle.exportKey("pkcs8", key);
    return toB64(new Uint8Array(raw));
  } catch {
    return "";
  }
}

async function exportPubBytes(key: CryptoKey): Promise<Uint8Array> {
  const raw = await window.crypto.subtle.exportKey("raw", key);
  return new Uint8Array(raw);
}

/** Serialize RatchetState to JSON-safe object (without CryptoKey) */
async function serializeRatchet(state: RatchetState): Promise<object> {
  const dhsPrivB64 = await exportPrivKey(state.dhsPriv);
  const skippedObj: Record<string, string> = {};
  state.mkSkipped.forEach((v, k) => { skippedObj[k] = toB64(v); });

  return {
    dhsPub: toB64(state.dhsPub),
    dhsPriv: dhsPrivB64,
    dhr: state.dhr ? toB64(state.dhr) : null,
    rk: toB64(state.rk),
    cks: state.cks ? toB64(state.cks) : null,
    ckr: state.ckr ? toB64(state.ckr) : null,
    ns: state.ns,
    nr: state.nr,
    pn: state.pn,
    mkSkipped: skippedObj,
  };
}

/** Deserialize RatchetState from JSON */
async function deserializeRatchet(data: any): Promise<RatchetState | null> {
  try {
    const subtle = window.crypto.subtle;

    // Import DH private key (P-256 — X25519 Web Crypto desteği tarayıcıya göre değişir)
    const dhsPrivRaw = fromB64(data.dhsPriv);
    const dhsPriv = await subtle.importKey(
      "pkcs8", dhsPrivRaw as unknown as ArrayBuffer,
      { name: "ECDH", namedCurve: "P-256" },
      true, ["deriveBits"]
    );

    const skipped = new Map<string, Uint8Array>();
    if (data.mkSkipped) {
      Object.entries(data.mkSkipped as Record<string, string>).forEach(([k, v]) => {
        skipped.set(k, fromB64(v));
      });
    }

    return {
      dhsPub: fromB64(data.dhsPub),
      dhsPriv,
      dhr: data.dhr ? fromB64(data.dhr) : undefined,
      rk: fromB64(data.rk),
      cks: data.cks ? fromB64(data.cks) : undefined,
      ckr: data.ckr ? fromB64(data.ckr) : undefined,
      ns: data.ns,
      nr: data.nr,
      pn: data.pn,
      mkSkipped: skipped,
    };
  } catch {
    return null;
  }
}

// ── Session persistence ────────────────────────────────────────────────────

export async function saveSession(convId: string, state: RatchetState): Promise<void> {
  try {
    const serialized = await serializeRatchet(state);
    localStorage.setItem(SESSION_PREFIX + convId, JSON.stringify(serialized));
  } catch {}
}

export async function loadSession(convId: string): Promise<RatchetState | null> {
  try {
    const raw = localStorage.getItem(SESSION_PREFIX + convId);
    if (!raw) return null;
    return deserializeRatchet(JSON.parse(raw));
  } catch {
    return null;
  }
}

// ── Session initialization ─────────────────────────────────────────────────

/**
 * Alice olarak yeni bir X3DH + Ratchet oturumu başlatır.
 * Sunucudan Bob'un prekey bundle'ını alır.
 */
export async function initiateSession(
  myIdentity: IdentityKeys,
  bundle: PreKeyBundle,
  convId: string
): Promise<RatchetState> {
  const { sharedKey, ephemeralPublicBytes } = await x3dhInitiate(myIdentity, bundle);

  // Bob'un SPK'sı ratchet starting point olarak kullanılır
  const bobRatchetPub = fromB64(bundle.signed_prekey);
  const state = await ratchetInitSender(sharedKey, bobRatchetPub);

  await saveSession(convId, state);
  return state;
}

// ── Encrypt/Decrypt helpers ────────────────────────────────────────────────

const AD = new TextEncoder().encode("Obscura-E2EE-v1");

export async function encryptForSend(
  state: RatchetState,
  plaintext: string,
  convId: string
): Promise<{ ciphertext: string; newState: RatchetState }> {
  const pt = new TextEncoder().encode(plaintext);
  const { message, newState } = await ratchetEncrypt(state, pt, AD);
  await saveSession(convId, newState);
  return { ciphertext: JSON.stringify(message), newState };
}

export async function decryptReceived(
  state: RatchetState,
  ciphertext: string,
  convId: string
): Promise<{ plaintext: string; newState: RatchetState }> {
  const message: RatchetMessage = JSON.parse(ciphertext);
  const { plaintext: pt, newState } = await ratchetDecrypt(state, message, AD);
  await saveSession(convId, newState);
  return { plaintext: new TextDecoder().decode(pt), newState };
}

// ── Is encrypted payload? ──────────────────────────────────────────────────
export function isEncryptedPayload(s: string): boolean {
  try {
    const obj = JSON.parse(s);
    return typeof obj === "object" && "dhPub" in obj && "ciphertext" in obj;
  } catch {
    return false;
  }
}
