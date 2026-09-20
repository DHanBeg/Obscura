/**
 * Obscura E2EE Session Manager
 * Konuşma başına Double Ratchet oturumu yönetir.
 * Ratchet state'ini localStorage'da seri hale getirir.
 */
"use client";

import {
  x3dhInitiate,
  x3dhAccept,
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
  type PreKeyStore,
  type RatchetMessage,
} from "./e2ee";

const SESSION_PREFIX = "obscura_session_v1_";
const ACTIVE_DID_KEY = "obscura_active_did";

// Aktif hesabin DID'i (public, gizli degil) -- oturum/pending-x3dh/sent-cache
// anahtarlarini hesaba gore ayristirmak icin. AppShell kimlik yuklenince cagirir.
export function setActiveAccountDid(did: string): void {
  try {
    localStorage.setItem(ACTIVE_DID_KEY, did);
  } catch {}
}

function activeAccountPrefix(): string {
  try {
    return localStorage.getItem(ACTIVE_DID_KEY) || "unknown";
  } catch {
    return "unknown";
  }
}

// ── Serialization ──────────────────────────────────────────────────────────
// dhsPriv ham X25519 byte (@noble/curves, bkz. e2ee.ts P-256→X25519 geçişi
// 2026-09-17) — pkcs8/CryptoKey köprüsüne gerek yok, direkt base64.

/** Serialize RatchetState to JSON-safe object */
async function serializeRatchet(state: RatchetState): Promise<object> {
  const skippedObj: Record<string, string> = {};
  state.mkSkipped.forEach((v, k) => { skippedObj[k] = toB64(v); });

  return {
    dhsPub: toB64(state.dhsPub),
    dhsPriv: toB64(state.dhsPriv),
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
    const skipped = new Map<string, Uint8Array>();
    if (data.mkSkipped) {
      Object.entries(data.mkSkipped as Record<string, string>).forEach(([k, v]) => {
        skipped.set(k, fromB64(v));
      });
    }

    return {
      dhsPub: fromB64(data.dhsPub),
      dhsPriv: fromB64(data.dhsPriv),
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
    localStorage.setItem(SESSION_PREFIX + activeAccountPrefix() + ":" + convId, JSON.stringify(serialized));
  } catch {}
}

export async function loadSession(convId: string): Promise<RatchetState | null> {
  try {
    const raw = localStorage.getItem(SESSION_PREFIX + activeAccountPrefix() + ":" + convId);
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
export interface X3dhPendingInit {
  ephemeralPub: string;
  identityPub: string;
  opkId?: number;
}

const PENDING_X3DH_PREFIX = "obscura_pending_x3dh_";

// X3DH baslatma verisi (efemeral/identity pub, opk id) -- SIR DEGIL, karsi
// tarafa zaten gonderilecek public malzeme. Sadece bir React ref'te
// tutulursa, ratchet session localStorage'a kalici yazilip da mesaj
// gonderilmeden sayfa yenilenirse (session var ama init verisi hic
// gonderilmemis) bu bilgi kaybolur ve alici oturumu asla kabul edemez.
// Bu yuzden session ile AYNI kalicilikta saklanir.
export function savePendingX3dhInit(convId: string, init: X3dhPendingInit): void {
  try {
    localStorage.setItem(PENDING_X3DH_PREFIX + activeAccountPrefix() + ":" + convId, JSON.stringify(init));
  } catch {}
}

export function loadPendingX3dhInit(convId: string): X3dhPendingInit | null {
  try {
    const raw = localStorage.getItem(PENDING_X3DH_PREFIX + activeAccountPrefix() + ":" + convId);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

export function clearPendingX3dhInit(convId: string): void {
  try {
    localStorage.removeItem(PENDING_X3DH_PREFIX + activeAccountPrefix() + ":" + convId);
  } catch {}
}

export async function initiateSession(
  myIdentity: IdentityKeys,
  bundle: PreKeyBundle,
  convId: string
): Promise<{ state: RatchetState; x3dhInit: X3dhPendingInit }> {
  const { sharedKey, ephemeralPublicBytes, opkId } = await x3dhInitiate(myIdentity, bundle);

  // Bob'un SPK'sı ratchet starting point olarak kullanılır
  const bobRatchetPub = fromB64(bundle.signed_prekey);
  const state = await ratchetInitSender(sharedKey, bobRatchetPub);

  await saveSession(convId, state);
  const x3dhInit: X3dhPendingInit = {
    ephemeralPub: toB64(ephemeralPublicBytes),
    identityPub: toB64(myIdentity.dhKeyPair.publicKeyBytes),
    opkId,
  };
  savePendingX3dhInit(convId, x3dhInit);
  return { state, x3dhInit };
}

// ── Encrypt/Decrypt helpers ────────────────────────────────────────────────

const AD = new TextEncoder().encode("Obscura-E2EE-v1");

export async function encryptForSend(
  state: RatchetState,
  plaintext: string,
  convId: string,
  x3dhInit?: X3dhPendingInit
): Promise<{ ciphertext: string; newState: RatchetState }> {
  const pt = new TextEncoder().encode(plaintext);
  const { message, newState } = await ratchetEncrypt(state, pt, AD);
  await saveSession(convId, newState);
  const envelope: Record<string, unknown> = { ...message };
  if (x3dhInit) {
    envelope.x3dhEphemeral = x3dhInit.ephemeralPub;
    envelope.x3dhIdentityPub = x3dhInit.identityPub;
    if (x3dhInit.opkId !== undefined) envelope.x3dhOpkId = x3dhInit.opkId;
  }
  return { ciphertext: JSON.stringify(envelope), newState };
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

/**
 * Gelen bir mesaji cozer. Oturum yoksa ve mesaj bir X3DH baslatma zarfi
 * tasiyorsa (ilk mesaj), alici tarafi (x3dhAccept + ratchetInitReceiver) burada
 * kurulur. Hicbir durumda ham/cozulmemis ciphertext kullaniciya gosterilmez --
 * basarisizlikta guvenli bir placeholder doner.
 */
export async function decryptIncoming(
  convId: string,
  rawText: string,
  myIdentity: IdentityKeys | null,
  prekeyStore: PreKeyStore | null,
  onSessionUpdate: (convId: string, state: RatchetState) => void
): Promise<string> {
  if (!isEncryptedPayload(rawText)) return rawText;

  const cached = getReceivedPlaintextCache(convId, rawText);
  if (cached !== null) return cached;

  try {
    let state = await loadSession(convId);

    if (!state) {
      const parsed = JSON.parse(rawText);
      if (!parsed.x3dhEphemeral || !parsed.x3dhIdentityPub) {
        return "\u{1F512} \u015Eifreli mesaj (oturum kurulamadi)";
      }
      if (!myIdentity || !prekeyStore) {
        return "\u{1F512} \u015Eifreli mesaj (anahtarlar hazir degil)";
      }
      const sharedKey = await x3dhAccept(
        prekeyStore,
        fromB64(parsed.x3dhIdentityPub),
        fromB64(parsed.x3dhEphemeral),
        parsed.x3dhOpkId
      );
      state = await ratchetInitReceiver(sharedKey, prekeyStore.signedPreKey);
      await saveSession(convId, state);
      onSessionUpdate(convId, state);
    }

    const { plaintext, newState } = await decryptReceived(state, rawText, convId);
    onSessionUpdate(convId, newState);
    cacheReceivedPlaintext(convId, rawText, plaintext);
    return plaintext;
  } catch (e) {
    console.error("decryptIncoming error:", e);
    return "\u{1F512} \u015Eifreli mesaj (cozulemedi)";
  }
}

// ── Is encrypted payload? ──────────────────────────────────────────────────
const SENT_CACHE_KEY = "obscura_sent_cache_v1";

function loadSentCache(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(SENT_CACHE_KEY + ":" + activeAccountPrefix()) || "{}");
  } catch {
    return {};
  }
}

// Kendi gonderdigimiz mesajin duz metnini ciphertext'e gore yerelde saklar.
// Double Ratchet'te gonderen kendi mesajini alici zinciriyle (ckr) cozemez --
// bu protokolun dogasi, bu yuzden kendi mesajlarimizi burada cache'leriz.
export function cacheSentPlaintext(convId: string, ciphertext: string, plaintext: string): void {
  try {
    const cache = loadSentCache();
    cache[`${convId}:${ciphertext}`] = plaintext;
    localStorage.setItem(SENT_CACHE_KEY + ":" + activeAccountPrefix(), JSON.stringify(cache));
  } catch {}
}

export function getSentPlaintextCache(convId: string, ciphertext: string): string | null {
  const cache = loadSentCache();
  return cache[`${convId}:${ciphertext}`] ?? null;
}

const RECV_CACHE_KEY = "obscura_recv_cache_v1";

function loadRecvCache(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(RECV_CACHE_KEY + ":" + activeAccountPrefix()) || "{}");
  } catch {
    return {};
  }
}

// Alinan (decrypt edilmis) mesajin duz metnini ciphertext'e gore yerelde
// saklar. Double Ratchet zincir anahtarlari tek kullanimlik -- bu cache
// olmadan sayfa her yenilendiginde ayni mesaj yeniden decrypt deneniyordu;
// anahtar zaten ilerlemis oldugundan ikinci deneme hep basarisiz oluyordu
// ("cozulemedi"). Kendi mesajlarimiz icin zaten var olan
// cacheSentPlaintext/getSentPlaintextCache ile ayni desen ve ayni depolama
// modeli (yerel localStorage, ek sifreleme yok -- sunucu hicbir zaman
// gormez, tehdit modeli mevcut sent-cache ile birebir ayni).
export function cacheReceivedPlaintext(convId: string, ciphertext: string, plaintext: string): void {
  try {
    const cache = loadRecvCache();
    cache[`${convId}:${ciphertext}`] = plaintext;
    localStorage.setItem(RECV_CACHE_KEY + ":" + activeAccountPrefix(), JSON.stringify(cache));
  } catch {}
}

export function getReceivedPlaintextCache(convId: string, ciphertext: string): string | null {
  const cache = loadRecvCache();
  return cache[`${convId}:${ciphertext}`] ?? null;
}

export function isEncryptedPayload(s: string): boolean {
  try {
    const obj = JSON.parse(s);
    return typeof obj === "object" && "dhPub" in obj && "ciphertext" in obj;
  } catch {
    return false;
  }
}
