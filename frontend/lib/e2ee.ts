/**
 * Obscura E2EE — Tarayıcı taraflı uçtan uca şifreleme
 *
 * X25519/Ed25519/AES-GCM: @noble/curves + @noble/ciphers (mobile lib/crypto.ts,
 * lib/identity.ts, lib/ratchet.ts ile AYNI kütüphane, AYNI sürüm — 2026-09-17
 * öncesi burada WebCrypto ECDH/ECDSA P-256 kullanılıyordu: fonksiyon adları
 * "X25519"/"Ed25519" yazıyordu ama GERÇEK eğri P-256'ydı, mobile ile X3DH
 * matematiksel olarak KURULAMIYORDU (farklı eğri) ve backend'in 32-byte
 * identity_key/signed_prekey kontrolü web'in 65-byte P-256 raw export'unu her
 * zaman reddediyordu (prekey upload hiç varmıyordu). Bu geçişle ikisi de
 * kapandı — tek eğri, mobile ile byte-uyumlu.
 *   - X25519: Anahtar anlaşması (DH)
 *   - Ed25519: Kimlik imzalama
 *   - AES-256-GCM: Mesaj şifreleme (ham byte key/nonce/aad, CryptoKey köprüsü yok)
 *   - HKDF-SHA256 / HMAC-SHA256 / SHA-256 / PBKDF2: DEĞİŞMEDİ — eğriden
 *     bağımsız, WebCrypto'da kalıyor (doğrulandı).
 *
 * Signal Protocol (X3DH + Double Ratchet).
 */
import { x25519, ed25519 } from "@noble/curves/ed25519.js";
import { gcm } from "@noble/ciphers/aes.js";

// Browser-only: SSR sırasında import edilmez (dynamic import kullanılır)
const subtle = typeof window !== "undefined" ? window.crypto.subtle : (crypto as any).subtle;

// ─── Yardımcı Fonksiyonlar ────────────────────────────────────────────────────

function randomBytes(n: number): Uint8Array {
    const arr = new Uint8Array(n);
    (typeof window !== "undefined" ? window.crypto : crypto).getRandomValues(arr);
    return arr;
}

function hex(buf: ArrayBuffer | Uint8Array): string {
    return Array.from(new Uint8Array(buf))
        .map(b => b.toString(16).padStart(2, '0'))
        .join('');
}

function fromHex(h: string): Uint8Array {
    const arr = new Uint8Array(h.length / 2);
    for (let i = 0; i < h.length; i += 2) {
        arr[i / 2] = parseInt(h.slice(i, i + 2), 16);
    }
    return arr;
}

function toB64(buf: ArrayBuffer | Uint8Array): string {
    const arr = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
    return btoa(String.fromCharCode(...arr));
}

function fromB64(b64: string): Uint8Array {
    return Uint8Array.from(atob(b64), c => c.charCodeAt(0));
}

function concat(...arrays: Uint8Array[]): Uint8Array {
    const total = arrays.reduce((s, a) => s + a.length, 0);
    const out = new Uint8Array(total);
    let offset = 0;
    for (const a of arrays) { out.set(a, offset); offset += a.length; }
    return out;
}

// ─── SHA-256 ──────────────────────────────────────────────────────────────────

async function sha256(data: Uint8Array): Promise<Uint8Array> {
    const hash = await subtle.digest('SHA-256', data);
    return new Uint8Array(hash);
}

// ─── HKDF-SHA256 ─────────────────────────────────────────────────────────────

async function hkdf(
    ikm: Uint8Array,
    salt: Uint8Array | null,
    info: Uint8Array,
    length: number
): Promise<Uint8Array> {
    const keyMaterial = await subtle.importKey(
        'raw', ikm, { name: 'HKDF' }, false, ['deriveBits']
    );
    const bits = await subtle.deriveBits(
        {
            name: 'HKDF',
            hash: 'SHA-256',
            salt: salt ?? new Uint8Array(32),
            info,
        },
        keyMaterial,
        length * 8
    );
    return new Uint8Array(bits);
}

// ─── HMAC-SHA256 ─────────────────────────────────────────────────────────────

async function hmacSha256(key: Uint8Array, data: Uint8Array): Promise<Uint8Array> {
    const k = await subtle.importKey(
        'raw', key, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']
    );
    const sig = await subtle.sign('HMAC', k, data);
    return new Uint8Array(sig);
}

// ─── AES-256-GCM ─────────────────────────────────────────────────────────────
// @noble/ciphers — mobile lib/crypto.ts/ratchet.ts ile aynı desen: ham
// Uint8Array key/nonce/aad, CryptoKey/importKey köprüsü yok.

export async function aesEncrypt(key: Uint8Array, plaintext: Uint8Array, aad?: Uint8Array): Promise<Uint8Array> {
    const iv = randomBytes(12);
    const ct = gcm(key, iv, aad).encrypt(plaintext);
    return concat(iv, ct);
}

export async function aesDecrypt(key: Uint8Array, data: Uint8Array, aad?: Uint8Array): Promise<Uint8Array> {
    if (data.length < 28) throw new Error('Şifreli veri çok kısa');
    const iv = data.slice(0, 12);
    const ct = data.slice(12);
    return gcm(key, iv, aad).decrypt(ct);
}

// ─── X25519 ───────────────────────────────────────────────────────────────────

export interface X25519KeyPair {
    privateKey: Uint8Array;    // 32 byte raw
    publicKeyBytes: Uint8Array; // 32 byte raw
}

export const X25519_KEY_LEN = 32;

export async function generateX25519(): Promise<X25519KeyPair> {
    const { secretKey, publicKey } = x25519.keygen();
    return { privateKey: secretKey, publicKeyBytes: publicKey };
}

export async function x25519DH(myPrivate: Uint8Array, theirPublicBytes: Uint8Array): Promise<Uint8Array> {
    return x25519.getSharedSecret(myPrivate, theirPublicBytes);
}

// ─── Ed25519 İmzalama ────────────────────────────────────────────────────────

export interface Ed25519KeyPair {
    privateKey: Uint8Array;    // 32 byte raw
    publicKeyBytes: Uint8Array; // 32 byte raw
}

export async function generateEd25519(): Promise<Ed25519KeyPair> {
    const { secretKey, publicKey } = ed25519.keygen();
    return { privateKey: secretKey, publicKeyBytes: publicKey };
}

export async function ed25519Sign(privateKey: Uint8Array, message: Uint8Array): Promise<Uint8Array> {
    return ed25519.sign(message, privateKey);
}

export async function ed25519Verify(
    publicKeyBytes: Uint8Array,
    message: Uint8Array,
    signature: Uint8Array
): Promise<boolean> {
    return ed25519.verify(signature, message, publicKeyBytes);
}

// ─── Kimlik Yönetimi ──────────────────────────────────────────────────────────

export interface IdentityKeys {
    dhKeyPair: X25519KeyPair;
    signingKeyPair: Ed25519KeyPair;
    did: string;
}

// DID türet: did:obs:<sha256(dhPublicKey)[0:16]>
export async function deriveDID(dhPublicBytes: Uint8Array): Promise<string> {
    const hash = await sha256(dhPublicBytes);
    return 'did:obs:' + hex(hash.slice(0, 16));
}

export async function generateIdentity(): Promise<IdentityKeys> {
    const dhKeyPair = await generateX25519();
    const signingKeyPair = await generateEd25519();
    const did = await deriveDID(dhKeyPair.publicKeyBytes);
    return { dhKeyPair, signingKeyPair, did };
}

// ─── PreKey Bundle ────────────────────────────────────────────────────────────

export interface OPK {
    id: number;
    keyPair: X25519KeyPair;
}

export interface PreKeyStore {
    identity: IdentityKeys;
    signedPreKey: X25519KeyPair;
    signedPreKeySig: Uint8Array;
    oneTimePreKeys: OPK[];
}

export async function generatePreKeyStore(identity: IdentityKeys): Promise<PreKeyStore> {
    // İmzalı PreKey
    const signedPreKey = await generateX25519();
    const signedPreKeySig = await ed25519Sign(
        identity.signingKeyPair.privateKey,
        signedPreKey.publicKeyBytes
    );

    // 100 OPK
    const oneTimePreKeys: OPK[] = [];
    for (let i = 0; i < 100; i++) {
        oneTimePreKeys.push({ id: i, keyPair: await generateX25519() });
    }

    return { identity, signedPreKey, signedPreKeySig, oneTimePreKeys };
}

// PreKey bundle'ı sunucuya yükleme formatı
// signing_key (Ed25519 pub) eklendi — backend SPK imzasını doğrulamak için kullanır.
export function bundleToUpload(store: PreKeyStore) {
    return {
        identity_key: toB64(store.identity.dhKeyPair.publicKeyBytes),
        signing_key: toB64(store.identity.signingKeyPair.publicKeyBytes),
        signed_prekey: toB64(store.signedPreKey.publicKeyBytes),
        signed_prekey_sig: toB64(store.signedPreKeySig),
        signed_prekey_id: 0,
        one_time_prekeys: store.oneTimePreKeys.slice(0, 100).map(opk => ({
            id: opk.id,
            public_key: toB64(opk.keyPair.publicKeyBytes),
        })),
    };
}

// ─── X3DH Anahtar Anlaşması ───────────────────────────────────────────────────

// Signal standardı: IKM = 0xFF×32 || DH1 || DH2 || DH3 [|| DH4]
async function deriveSharedKey(dhOutputs: Uint8Array[]): Promise<Uint8Array> {
    const prefix = new Uint8Array(32).fill(0xFF);
    const ikm = concat(prefix, ...dhOutputs);
    return hkdf(ikm, null, new TextEncoder().encode('ObscuraX3DH'), 32);
}

export interface X3DHInitResult {
    sharedKey: Uint8Array;           // 32 byte
    ephemeralPublicBytes: Uint8Array; // Karşı tarafa gönderilir
    opkId?: number;
}

// Alice — X3DH başlat (Bob'un bundle'ından)
export interface PreKeyBundle {
    identity_key: string;       // Base64
    signed_prekey: string;      // Base64
    signed_prekey_sig: string;  // Base64
    one_time_prekey?: string;   // Base64
    one_time_prekey_id?: number;
    did: string;
}

export async function x3dhInitiate(
    aliceIdentity: IdentityKeys,
    bundle: PreKeyBundle
): Promise<X3DHInitResult> {
    const ikB = fromB64(bundle.identity_key);   // Bob IK pub
    const spkB = fromB64(bundle.signed_prekey); // Bob SPK pub

    // SPK imzasını doğrula (Ed25519)
    // Not: bundle.identity_key X25519 key, imza için ayrı Ed25519 key lazım
    // Şu an atlıyoruz — tam implementasyonda bundle'da signing_public da olacak

    // Alice efemeral anahtar (tüm DH'lerde reuse)
    const ekA = await generateX25519();

    // DH1 = DH(IK_A, SPK_B)
    const dh1 = await x25519DH(aliceIdentity.dhKeyPair.privateKey, spkB);
    // DH2 = DH(EK_A, IK_B)
    const dh2 = await x25519DH(ekA.privateKey, ikB);
    // DH3 = DH(EK_A, SPK_B)
    const dh3 = await x25519DH(ekA.privateKey, spkB);

    let opkId: number | undefined;

    if (bundle.one_time_prekey && bundle.one_time_prekey_id !== undefined) {
        const opkB = fromB64(bundle.one_time_prekey);
        // DH4 = DH(EK_A, OPK_B)
        const dh4 = await x25519DH(ekA.privateKey, opkB);
        const sharedKey = await deriveSharedKey([dh1, dh2, dh3, dh4]);
        opkId = bundle.one_time_prekey_id;
        return { sharedKey, ephemeralPublicBytes: ekA.publicKeyBytes, opkId };
    } else {
        const sharedKey = await deriveSharedKey([dh1, dh2, dh3]);
        return { sharedKey, ephemeralPublicBytes: ekA.publicKeyBytes };
    }
}

// Bob — X3DH kabul (Alice'den gelen ephemeral public ile)
export async function x3dhAccept(
    bobStore: PreKeyStore,
    aliceIdentityPub: Uint8Array,
    aliceEphemeralPub: Uint8Array,
    usedOpkId?: number
): Promise<Uint8Array> {
    const spkB = bobStore.signedPreKey;
    const ikB = bobStore.identity.dhKeyPair;

    // DH1 = DH(SPK_B, IK_A)
    const dh1 = await x25519DH(spkB.privateKey, aliceIdentityPub);
    // DH2 = DH(IK_B, EK_A)
    const dh2 = await x25519DH(ikB.privateKey, aliceEphemeralPub);
    // DH3 = DH(SPK_B, EK_A)
    const dh3 = await x25519DH(spkB.privateKey, aliceEphemeralPub);

    if (usedOpkId !== undefined) {
        const opk = bobStore.oneTimePreKeys.find(o => o.id === usedOpkId);
        if (opk) {
            const dh4 = await x25519DH(opk.keyPair.privateKey, aliceEphemeralPub);
            return deriveSharedKey([dh1, dh2, dh3, dh4]);
        }
    }
    return deriveSharedKey([dh1, dh2, dh3]);
}

// ─── Double Ratchet ───────────────────────────────────────────────────────────

async function kdfRK(rk: Uint8Array, dhOut: Uint8Array): Promise<[Uint8Array, Uint8Array]> {
    const out = await hkdf(dhOut, rk, new TextEncoder().encode('ObscuraRatchetRK'), 64);
    return [out.slice(0, 32), out.slice(32, 64)];
}

async function kdfCK(ck: Uint8Array): Promise<[Uint8Array, Uint8Array]> {
    const mk = await hmacSha256(ck, new Uint8Array([0x01]));
    const newCk = await hmacSha256(ck, new Uint8Array([0x02]));
    return [newCk, mk];
}

export interface RatchetState {
    dhsPub: Uint8Array;   // Bizim DH ratchet pub
    dhsPriv: Uint8Array;  // Bizim DH ratchet priv (32 byte raw X25519)
    dhr?: Uint8Array;     // Karşı taraf DH pub
    rk: Uint8Array;       // Root key
    cks?: Uint8Array;     // Sending chain key
    ckr?: Uint8Array;     // Receiving chain key
    ns: number;           // Sending counter
    nr: number;           // Receiving counter
    pn: number;           // Previous sending chain count
    mkSkipped: Map<string, Uint8Array>; // Skipped message keys
}

export async function ratchetInitSender(
    sharedKey: Uint8Array,
    bobRatchetPubBytes: Uint8Array
): Promise<RatchetState> {
    const dhs = await generateX25519();
    const dhOut = await x25519DH(dhs.privateKey, bobRatchetPubBytes);
    const [newRk, cks] = await kdfRK(sharedKey, dhOut);
    return {
        dhsPub: dhs.publicKeyBytes,
        dhsPriv: dhs.privateKey,
        dhr: bobRatchetPubBytes,
        rk: newRk,
        cks,
        ckr: undefined,
        ns: 0, nr: 0, pn: 0,
        mkSkipped: new Map(),
    };
}

export async function ratchetInitReceiver(
    sharedKey: Uint8Array,
    bobRatchetKeyPair: X25519KeyPair
): Promise<RatchetState> {
    return {
        dhsPub: bobRatchetKeyPair.publicKeyBytes,
        dhsPriv: bobRatchetKeyPair.privateKey,
        dhr: undefined,
        rk: sharedKey,
        cks: undefined,
        ckr: undefined,
        ns: 0, nr: 0, pn: 0,
        mkSkipped: new Map(),
    };
}

export interface RatchetMessage {
    dhPub: string;   // Base64
    pn: number;
    n: number;
    ciphertext: string; // Base64
}

export async function ratchetEncrypt(
    state: RatchetState,
    plaintext: Uint8Array,
    ad: Uint8Array
): Promise<{ message: RatchetMessage; newState: RatchetState }> {
    if (!state.cks) throw new Error('Gönderme zincir anahtarı yok');

    const [newCks, mk] = await kdfCK(state.cks);
    const n = state.ns;
    const headerBytes = makeHeader(state.dhsPub, state.pn, n);
    const aad = concat(ad, headerBytes);
    const ct = await aesEncrypt(mk, plaintext, aad);

    return {
        message: {
            dhPub: toB64(state.dhsPub),
            pn: state.pn,
            n,
            ciphertext: toB64(ct),
        },
        newState: { ...state, cks: newCks, ns: state.ns + 1 },
    };
}

export async function ratchetDecrypt(
    state: RatchetState,
    message: RatchetMessage,
    ad: Uint8Array
): Promise<{ plaintext: Uint8Array; newState: RatchetState }> {
    const dhPub = fromB64(message.dhPub);
    const ct = fromB64(message.ciphertext);

    // Skipped key kontrolü
    const skippedKey = `${message.dhPub}:${message.n}`;
    if (state.mkSkipped.has(skippedKey)) {
        const mk = state.mkSkipped.get(skippedKey)!;
        const newSkipped = new Map(state.mkSkipped);
        newSkipped.delete(skippedKey);
        const headerBytes = makeHeader(dhPub, message.pn, message.n);
        const aad = concat(ad, headerBytes);
        const pt = await aesDecrypt(mk, ct, aad);
        return { plaintext: pt, newState: { ...state, mkSkipped: newSkipped } };
    }

    let newState = { ...state };

    // DH ratchet gerekiyor mu?
    const isDifferentDH = !state.dhr ||
        hex(dhPub) !== hex(state.dhr);

    if (isDifferentDH) {
        // Mevcut zinciri kaydırarak bitir
        newState = await skipMessageKeys(newState, message.pn);
        newState = await dhRatchetStep(newState, dhPub);
    }

    // Alma zincirini ilerlet
    newState = await skipMessageKeys(newState, message.n);

    if (!newState.ckr) throw new Error('Alma zinciri başlatılmamış');
    const [newCkr, mk] = await kdfCK(newState.ckr);

    const headerBytes = makeHeader(dhPub, message.pn, message.n);
    const aad = concat(ad, headerBytes);
    const pt = await aesDecrypt(mk, ct, aad);

    return {
        plaintext: pt,
        newState: { ...newState, ckr: newCkr, nr: newState.nr + 1 },
    };
}

function makeHeader(dhPub: Uint8Array, pn: number, n: number): Uint8Array {
    const out = new Uint8Array(40);
    out.set(dhPub, 0);
    new DataView(out.buffer).setUint32(32, pn, false);
    new DataView(out.buffer).setUint32(36, n, false);
    return out;
}

async function skipMessageKeys(state: RatchetState, until: number): Promise<RatchetState> {
    if (!state.ckr || state.nr >= until) return state;
    if (state.nr + 1000 < until) throw new Error('Çok fazla mesaj atlandı');

    const newSkipped = new Map(state.mkSkipped);
    let ckr = state.ckr;
    let nr = state.nr;
    const dhrHex = state.dhr ? toB64(state.dhr) : '';

    while (nr < until) {
        const [newCkr, mk] = await kdfCK(ckr);
        newSkipped.set(`${dhrHex}:${nr}`, mk);
        ckr = newCkr;
        nr++;
    }

    return { ...state, ckr, nr, mkSkipped: newSkipped };
}

async function dhRatchetStep(state: RatchetState, newDhr: Uint8Array): Promise<RatchetState> {
    const pn = state.ns;
    // Alma zinciri
    const dhOut1 = await x25519DH(state.dhsPriv, newDhr);
    const [rk1, ckr] = await kdfRK(state.rk, dhOut1);
    // Yeni DH key pair
    const newDhs = await generateX25519();
    // Gönderme zinciri
    const dhOut2 = await x25519DH(newDhs.privateKey, newDhr);
    const [rk2, cks] = await kdfRK(rk1, dhOut2);

    return {
        ...state,
        dhsPub: newDhs.publicKeyBytes,
        dhsPriv: newDhs.privateKey,
        dhr: newDhr,
        rk: rk2,
        cks,
        ckr,
        ns: 0,
        nr: 0,
        pn,
    };
}

// ─── LocalStorage Kalıcılık ───────────────────────────────────────────────────

const IDENTITY_KEY = 'obscura_identity_v1';
const PREKEY_STORE_KEY = 'obscura_prekey_store_v1';

// Format 2 = X25519/Ed25519 ham byte (2026-09-17 P-256→X25519 geçişi).
// Format 1 (marker YOK, alan hiç yazılmazdı) = eski WebCrypto P-256 kaydı:
// dhPriv/sigPriv pkcs8-DER (CryptoKey export), dhPub/sigPub 65 byte
// uncompressed P-256 idi. İkisi AYNI JSON alan adlarını (dhPriv/dhPub/
// sigPriv/sigPub) paylaşıyor — marker'sız yüklemede eski byte'lar sessizce
// "32 byte X25519 anahtarı" sanılıp yanlış bir kimlik üretilebilirdi. Bu
// yüzden format kontrolü ZORUNLU ve açık bir reddetme dalı.
const IDENTITY_FORMAT_VERSION = 2;

// Kimlik anahtarlarını şifreli localStorage'a kaydet.
export async function saveIdentity(identity: IdentityKeys, passphrase: string): Promise<void> {
    // Paroladan şifreleme anahtarı türet — PBKDF2 eğriden bağımsız, WebCrypto'da kalıyor.
    const enc = new TextEncoder();
    const keyMaterial = await subtle.importKey('raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveBits']);
    const salt = randomBytes(16);
    const keyBits = await subtle.deriveBits(
        { name: 'PBKDF2', hash: 'SHA-256', salt, iterations: 100000 },
        keyMaterial,
        256
    );
    const aesKey = new Uint8Array(keyBits);

    const data = JSON.stringify({
        format: IDENTITY_FORMAT_VERSION,
        dhPriv: toB64(identity.dhKeyPair.privateKey),
        dhPub: toB64(identity.dhKeyPair.publicKeyBytes),
        sigPriv: toB64(identity.signingKeyPair.privateKey),
        sigPub: toB64(identity.signingKeyPair.publicKeyBytes),
        did: identity.did,
    });

    const iv = randomBytes(12);
    const ct = gcm(aesKey, iv).encrypt(enc.encode(data));

    const stored = JSON.stringify({
        salt: toB64(salt),
        iv: toB64(iv),
        ct: toB64(ct),
    });
    localStorage.setItem(IDENTITY_KEY, stored);
}

export async function loadIdentity(passphrase: string): Promise<IdentityKeys | null> {
    const raw = localStorage.getItem(IDENTITY_KEY);
    if (!raw) return null;

    try {
        const { salt, iv, ct } = JSON.parse(raw);
        const enc = new TextEncoder();
        const keyMaterial = await subtle.importKey('raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveBits']);
        const keyBits = await subtle.deriveBits(
            { name: 'PBKDF2', hash: 'SHA-256', salt: fromB64(salt), iterations: 100000 },
            keyMaterial,
            256
        );
        const aesKey = new Uint8Array(keyBits);
        const dec = gcm(aesKey, fromB64(iv)).decrypt(fromB64(ct));
        const data = JSON.parse(new TextDecoder().decode(dec));

        if (data.format !== IDENTITY_FORMAT_VERSION) {
            console.warn(
                `Kimlik kaydı eski formatta (format=${data.format ?? 'yok'}, beklenen ${IDENTITY_FORMAT_VERSION}) ` +
                `— P-256'dan X25519'a geçiş (2026-09-17). Eski kayıt GEÇERSİZ sayılıyor, ` +
                `eski byte'lar X25519 anahtarı olarak yorumlanmayacak. Yeni kimlik üretilecek.`
            );
            return null;
        }

        const dhPriv = fromB64(data.dhPriv);
        const dhPub = fromB64(data.dhPub);
        const sigPriv = fromB64(data.sigPriv);
        const sigPub = fromB64(data.sigPub);

        if (dhPriv.length !== X25519_KEY_LEN || dhPub.length !== X25519_KEY_LEN ||
            sigPriv.length !== X25519_KEY_LEN || sigPub.length !== X25519_KEY_LEN) {
            console.warn('Kimlik kaydı beklenmeyen byte uzunluğunda (format doğru ama boyut yanlış) — geçersiz sayılıyor.');
            return null;
        }

        return {
            dhKeyPair: { privateKey: dhPriv, publicKeyBytes: dhPub },
            signingKeyPair: { privateKey: sigPriv, publicKeyBytes: sigPub },
            did: data.did,
        };
    } catch (e) {
        console.error('Kimlik yükleme hatası:', e);
        return null;
    }
}

// ─── Kimlik: getOrCreate (idempotent) ─────────────────────────────────────────
//
// doVerify() öncesi check-then-generate-then-save ayrı adımlardı, çağıran
// tarafta kilit yoktu — iki eşzamanlı çağrı ikisi de "yok" görüp FARKLI
// rastgele X25519 kimliği üretip birbirinin üzerine yazabiliyordu. Bu
// fonksiyon üç adımı tek atomik birim haline getirir (mobile lib/e2e.ts
// getOrCreateKeyPair ile aynı desen): varsa döndür, yoksa üret+kaydet.
export async function getOrCreateIdentity(passphrase: string): Promise<IdentityKeys> {
    const existing = await loadIdentity(passphrase);
    if (existing) return existing;

    const identity = await generateIdentity();
    await saveIdentity(identity, passphrase);
    return identity;
}

// ─── Tek-uçuş kilit (race guard) ───────────────────────────────────────────────
//
// mobile app/(auth)/login.tsx (verifyingRef) ile aynı desen, framework'ten
// bağımsız. Kilit sink'te durur (doVerify'in gövdesinde) — tetikleyici
// (manuel buton / oto-80ms / dev-otp) sayısı önemli değil, hangisi önce
// girerse kilidi alır, diğerleri sessizce dışarıda kalır.
export function createRunOnceGuard(): { tryEnter: () => boolean; exit: () => void } {
    let locked = false;
    return {
        tryEnter: () => {
            if (locked) return false;
            locked = true;
            return true;
        },
        exit: () => {
            locked = false;
        },
    };
}

// ─── Konuşma Şifreleme API'si ─────────────────────────────────────────────────

export interface ConversationCrypto {
    state: RatchetState;
    ad: Uint8Array; // Conversation ID (additional data)
}

export async function encryptMessage(
    conv: ConversationCrypto,
    plaintext: string
): Promise<{ ciphertext: string; newState: RatchetState }> {
    const pt = new TextEncoder().encode(plaintext);
    const { message, newState } = await ratchetEncrypt(conv.state, pt, conv.ad);
    return {
        ciphertext: JSON.stringify(message),
        newState,
    };
}

export async function decryptMessage(
    conv: ConversationCrypto,
    ciphertext: string
): Promise<{ plaintext: string; newState: RatchetState }> {
    const message: RatchetMessage = JSON.parse(ciphertext);
    const { plaintext: pt, newState } = await ratchetDecrypt(conv.state, message, conv.ad);
    return {
        plaintext: new TextDecoder().decode(pt),
        newState,
    };
}

// ─── Tip ihracı ───────────────────────────────────────────────────────────────

export { toB64, fromB64, hex, fromHex, randomBytes };
