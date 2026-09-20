import { describe, it, expect, beforeAll, beforeEach } from "vitest";

// GERÇEK backend'e gider (mock YOK). Önkoşul: backend OBSCURA_ENV=development
// ile BACKEND_URL'de (varsayılan http://localhost:8099) çalışıyor. Erişilemezse
// test KIRMIZI olur — atlanmaz, çünkü tur boyunca mock'un gerçek defect'i
// (web P-256 65 byte ≠ backend 32 byte) sakladığı görüldü.
const BACKEND_URL = process.env.BACKEND_URL ?? "http://localhost:8099";

async function post(path: string, body: unknown, token?: string) {
  const res = await fetch(`${BACKEND_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: JSON.stringify(body),
  });
  return { status: res.status, json: (await res.json()) as any };
}

async function loginWith(identityKeyB64: string, phone: string): Promise<string> {
  const r1 = await post("/v1/auth/request-otp", { phone });
  expect(r1.json.success).toBe(true);
  const dev = await fetch(`${BACKEND_URL}/v1/dev/otp?phone=${encodeURIComponent(phone)}`);
  const otp = ((await dev.json()) as any).data.otp as string;
  const r2 = await post("/v1/auth/verify-otp", {
    phone,
    otp,
    username: "u" + Math.random().toString(36).slice(2, 10),
    identity_key: identityKeyB64,
  });
  expect(r2.json.success).toBe(true);
  return r2.json.data.token as string;
}

describe("web prekey yükleme — GERÇEK backend byte-validasyonu (mock yok)", () => {
  let generateIdentity: typeof import("./e2ee").generateIdentity;
  let toB64: typeof import("./e2ee").toB64;
  let syncPreKeys: typeof import("./prekeys-sync").syncPreKeys;
  let api: typeof import("./api").api;
  // Backend IP başına dakikada 5 OTP isteği sınırlıyor (handlers.go checkOTPRateLimit):
  // tüm dosya için TEK login, iki test aynı kimlik/token'ı paylaşır.
  let identity: Awaited<ReturnType<typeof import("./e2ee").generateIdentity>>;
  let token: string;

  beforeAll(async () => {
    // api.ts token'ı yalnız `typeof window !== "undefined"` iken localStorage'dan
    // okur; Node'da window yok — tarayıcı ortamını taklit et (fetch/crypto gerçek).
    (globalThis as any).window = globalThis;
    process.env.NEXT_PUBLIC_API_URL = BACKEND_URL;
    ({ generateIdentity, toB64 } = await import("./e2ee"));
    ({ syncPreKeys } = await import("./prekeys-sync"));
    ({ api } = await import("./api"));

    identity = await generateIdentity();
    const phone = "+90555" + Math.floor(1000000 + Math.random() * 8999999);
    token = await loginWith(toB64(identity.dhKeyPair.publicKeyBytes), phone);
  });

  beforeEach(() => {
    localStorage.clear();
  });

  it("ensurePreKeysUploaded gerçek /v1/keys/upload'a gider: identity_key + signed_prekey 32-byte kontrolünden geçer (200)", async () => {
    localStorage.setItem("obscura_token", token);

    // ensurePreKeysUploaded içinde uploadPrekeys hatası fırlatırsa (backend 400) test düşer.
    const result = await syncPreKeys(identity, "obscura_did:obs:integ_v1");
    expect(result.reason).toBe("initial");
    expect(result.uploaded).toBe(true);

    // Sunucu tarafında gerçekten yazıldığını bağımsız doğrula.
    const count = await api.getOPKCount();
    expect(count.count).toBe(100);

    // İkinci çağrı (idempotent): sunucuda yeterli set var → hiçbir şey yüklenmez.
    const second = await syncPreKeys(identity, "obscura_did:obs:integ_v1");
    expect(second.reason).toBe("sufficient");
    expect((await api.getOPKCount()).count).toBe(100);
  });

  it("negatif kontrol: 65 byte (eski P-256 boyutu) identity_key backend tarafından 400 ile reddedilir", async () => {
    const p256Sized = toB64(crypto.getRandomValues(new Uint8Array(65)));
    const res = await post(
      "/v1/keys/upload",
      {
        identity_key: p256Sized,
        signing_key: toB64(identity.signingKeyPair.publicKeyBytes),
        signed_prekey: toB64(identity.dhKeyPair.publicKeyBytes),
        signed_prekey_sig: toB64(crypto.getRandomValues(new Uint8Array(64))),
        signed_prekey_id: 0,
        one_time_prekeys: [],
      },
      token
    );
    expect(res.status).toBe(400);
    expect(res.json.error).toContain("32 byte");
  });
});
