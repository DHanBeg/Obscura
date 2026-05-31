/**
 * Obscura E2EE — Tarayıcı taraflı uçtan uca şifreleme
 *
 * WebCrypto API kullanır (dış bağımlılık yok):
 *   - X25519 ECDH: Anahtar anlaşması
 *   - Ed25519: Kimlik imzalama
 *   - AES-256-GCM: Mesaj şifreleme
 *   - HKDF-SHA256: Anahtar türetme
 *   - HMAC-SHA256: Ratchet zincir anahtarı
 *
 * Signal Protocol (X3DH + Double Ratchet) tarayıcı-native implementasyonu.
 * Node.js çalışma zamanı veya WASM gerektirmez.
 */

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

export async function aesEncrypt(key: Uint8Array, plaintext: Uint8Array, aad?: Uint8Array): Promise<Uint8Array> {
    const iv = randomBytes(12);
    const k = await subtle.importKey('raw', key, 'AES-GCM', false, ['encrypt']);
    const ct = await subtle.encrypt({ name: 'AES-GCM', iv, additionalData: aad }, k, plaintext);
    return concat(iv, new Uint8Array(ct));
}

export async function aesDecrypt(key: Uint8Array, data: Uint8Array, aad?: Uint8Array): Promise<Uint8Array> {
    if (data.length < 28) throw new Error('Şifreli veri çok kısa');
    const iv = data.slice(0, 12);
    const ct = data.slice(12);
    const k = await subtle.importKey('raw', key, 'AES-GCM', false, ['decrypt']);
    const pt = await subtle.decrypt({ name: 'AES-GCM', iv, additionalData: aad }, k, ct);
    return new Uint8Array(pt);
}

// ─── X25519 ECDH ──────────────────────────────────────────────────────────────

export interface X25519KeyPair {
    privateKey: CryptoKey;
    publicKey: CryptoKey;
    publicKeyBytes: Uint8Array; // 32 byte raw
}

// P-256 is universally supported (Chrome 37+, Firefox 34+, Safari 7+).
// X25519 requires Chrome 133+ / Firefox 130+ / Safari 17.4+ and fails on older builds.
const ECDH_CURVE = 'P-256';

export async function generateX25519(): Promise<X25519KeyPair> {
    const pair = await subtle.generateKey(
        { name: 'ECDH', namedCurve: ECDH_CURVE },
        true,
        ['deriveBits']
    );
    const pubRaw = await subtle.exportKey('raw', pair.publicKey);
    return {
        privateKey: pair.privateKey,
        publicKey: pair.publicKey,
        publicKeyBytes: new Uint8Array(pubRaw),
    };
}

export async function x25519DH(myPrivate: CryptoKey, theirPublicBytes: Uint8Array): Promise<Uint8Array> {
    const theirPublic = await subtle.importKey(
        'raw', theirPublicBytes,
        { name: 'ECDH', namedCurve: ECDH_CURVE },
        true, []
    );
    const bits = await subtle.deriveBits(
        { name: 'ECDH', public: theirPublic } as any,
        myPrivate,
        256
    );
    return new Uint8Array(bits);
}

// ─── Ed25519 İmzalama ────────────────────────────────────────────────────────

export interface Ed25519KeyPair {
    privateKey: CryptoKey;
    publicKey: CryptoKey;
    publicKeyBytes: Uint8Array; // 32 byte raw
}

// ECDSA P-256 instead of Ed25519 — universally supported, same security level.
const SIGN_ALGO = { name: 'ECDSA', namedCurve: 'P-256' } as const;
const SIGN_PARAMS = { name: 'ECDSA', hash: 'SHA-256' } as const;

export async function generateEd25519(): Promise<Ed25519KeyPair> {
    const pair = await subtle.generateKey(SIGN_ALGO, true, ['sign', 'verify']);
    const pubRaw = await subtle.exportKey('raw', pair.publicKey);
    return {
        privateKey: pair.privateKey,
        publicKey: pair.publicKey,
        publicKeyBytes: new Uint8Array(pubRaw),
    };
}

export async function ed25519Sign(privateKey: CryptoKey, message: Uint8Array): Promise<Uint8Array> {
    const sig = await subtle.sign(SIGN_PARAMS, privateKey, message);
    return new Uint8Array(sig);
}

export async function ed25519Verify(
    publicKeyBytes: Uint8Array,
    message: Uint8Array,
    signature: Uint8Array
): Promise<boolean> {
    const pk = await subtle.importKey('raw', publicKeyBytes, SIGN_ALGO, true, ['verify']);
    return subtle.verify(SIGN_PARAMS, pk, signature, message);
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
    dhsPriv: CryptoKey;   // Bizim DH ratchet priv
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

// Kimlik anahtarlarını şifreli IndexedDB'ye kaydet
// Şu an: basit localStorage (TODO: IndexedDB + device key encrypt)
export async function saveIdentity(identity: IdentityKeys, passphrase: string): Promise<void> {
    // Paroladan şifreleme anahtarı türet
    const enc = new TextEncoder();
    const keyMaterial = await subtle.importKey('raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveKey']);
    const salt = randomBytes(16);
    const aesKey = await subtle.deriveKey(
        { name: 'PBKDF2', hash: 'SHA-256', salt, iterations: 100000 },
        keyMaterial,
        { name: 'AES-GCM', length: 256 },
        false,
        ['encrypt', 'decrypt']
    );

    // DH private key
    const dhPriv = await subtle.exportKey('pkcs8', identity.dhKeyPair.privateKey);
    const sigPriv = await subtle.exportKey('pkcs8', identity.signingKeyPair.privateKey);

    const data = JSON.stringify({
        dhPriv: toB64(new Uint8Array(dhPriv)),
        dhPub: toB64(identity.dhKeyPair.publicKeyBytes),
        sigPriv: toB64(new Uint8Array(sigPriv)),
        sigPub: toB64(identity.signingKeyPair.publicKeyBytes),
        did: identity.did,
    });

    const iv = randomBytes(12);
    const ct = await subtle.encrypt({ name: 'AES-GCM', iv }, aesKey, enc.encode(data));

    const stored = JSON.stringify({
        salt: toB64(salt),
        iv: toB64(iv),
        ct: toB64(new Uint8Array(ct)),
    });
    localStorage.setItem(IDENTITY_KEY, stored);
}

export async function loadIdentity(passphrase: string): Promise<IdentityKeys | null> {
    const raw = localStorage.getItem(IDENTITY_KEY);
    if (!raw) return null;

    try {
        const { salt, iv, ct } = JSON.parse(raw);
        const enc = new TextEncoder();
        const keyMaterial = await subtle.importKey('raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveKey']);
        const aesKey = await subtle.deriveKey(
            { name: 'PBKDF2', hash: 'SHA-256', salt: fromB64(salt), iterations: 100000 },
            keyMaterial,
            { name: 'AES-GCM', length: 256 },
            false,
            ['encrypt', 'decrypt']
        );
        const dec = await subtle.decrypt({ name: 'AES-GCM', iv: fromB64(iv) }, aesKey, fromB64(ct));
        const data = JSON.parse(new TextDecoder().decode(dec));

        const dhPriv = await subtle.importKey('pkcs8', fromB64(data.dhPriv),
            { name: 'ECDH', namedCurve: ECDH_CURVE }, true, ['deriveBits']);
        const dhPub = await subtle.importKey('raw', fromB64(data.dhPub),
            { name: 'ECDH', namedCurve: ECDH_CURVE }, true, []);
        const sigPriv = await subtle.importKey('pkcs8', fromB64(data.sigPriv),
            SIGN_ALGO, true, ['sign']);
        const sigPub = await subtle.importKey('raw', fromB64(data.sigPub),
            SIGN_ALGO, true, ['verify']);

        return {
            dhKeyPair: { privateKey: dhPriv, publicKey: dhPub, publicKeyBytes: fromB64(data.dhPub) },
            signingKeyPair: { privateKey: sigPriv, publicKey: sigPub, publicKeyBytes: fromB64(data.sigPub) },
            did: data.did,
        };
    } catch (e) {
        console.error('Kimlik yükleme hatası:', e);
        return null;
    }
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
