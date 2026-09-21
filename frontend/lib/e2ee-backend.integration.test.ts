import { describe, it, expect, beforeAll } from "vitest";

// GERÇEK backend'e gider (mock YOK), iki hesap arası 1:1 uçtan uca şifreli akış.
// Önkoşul: backend OBSCURA_ENV=development ile BACKEND_URL'de (varsayılan
// http://localhost:8099) çalışıyor. Erişilemezse test KIRMIZI olur — atlanmaz
// (bkz. prekeys-backend.integration.test.ts, aynı gerekçe).
//
// Regresyon: aynı saniyede giden iki mesaj (X3DH zarfı taşıyan ilk + ikinci)
// sunucudan GÖNDERİM SIRASIYLA dönmeli. Eskiden ORDER BY sent_at ASC eşitlik
// bozucusuz olduğundan ikinci mesaj önce gelip "oturum kurulamadi" alıyordu ve
// yanıttaki sent_at hep 0001-01-01T00:00:00Z idi.
const BACKEND_URL = process.env.BACKEND_URL ?? "http://localhost:8099";

async function post(path: string, body: unknown, token?: string) {
  const res = await fetch(`${BACKEND_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: JSON.stringify(body),
  });
  return { status: res.status, json: (await res.json()) as any };
}

interface Acct {
  identity: any;
  token: string;
  did: string;
  pass: string;
  store?: any;
}

describe("iki hesap 1:1 — gerçek backend, uçtan uca şifreli, gönderim sırası korunur", () => {
  let e2ee: typeof import("./e2ee");
  let sess: typeof import("./e2ee-session");
  let pk: typeof import("./prekeys-sync");
  let api: typeof import("./api").api;
  let alice: Acct;
  let bob: Acct;

  // Tarayıcı oturumu: api.ts token'ı localStorage'dan okur; AppShell aktif DID'i kurar.
  const as = (a: Acct) => {
    localStorage.setItem("obscura_token", a.token);
    sess.setActiveAccountDid(a.identity.did);
  };

  async function register(username: string): Promise<Acct> {
    const identity = await e2ee.generateIdentity();
    const phone = "+90555" + Math.floor(1000000 + Math.random() * 8999999);
    // Backend IP başına dakikada 5 OTP isteği sınırlıyor: iki hesap = 2 istek.
    const r1 = await post("/v1/auth/request-otp", { phone });
    expect(r1.json.success).toBe(true);
    const dev = await fetch(`${BACKEND_URL}/v1/dev/otp?phone=${encodeURIComponent(phone)}`);
    const otp = ((await dev.json()) as any).data.otp as string;
    const r2 = await post("/v1/auth/verify-otp", {
      phone,
      otp,
      username,
      identity_key: e2ee.toB64(identity.dhKeyPair.publicKeyBytes),
    });
    expect(r2.json.success).toBe(true);
    const did = r2.json.data.user?.did ?? r2.json.data.did;
    expect(did).toBeTruthy();
    return { identity, token: r2.json.data.token, did, pass: `obscura_${did}_v1` };
  }

  beforeAll(async () => {
    // api.ts token'ı yalnız `typeof window !== "undefined"` iken localStorage'dan okur.
    (globalThis as any).window = globalThis;
    process.env.NEXT_PUBLIC_API_URL = BACKEND_URL;
    e2ee = await import("./e2ee");
    sess = await import("./e2ee-session");
    pk = await import("./prekeys-sync");
    ({ api } = await import("./api"));
    alice = await register("al" + Math.random().toString(36).slice(2, 8));
    bob = await register("bo" + Math.random().toString(36).slice(2, 8));
  });

  it("Alice iki hızlı mesaj → sunucu gönderim sırasıyla döner, sent_at gerçek, Bob ikisini de çözer; Bob→Alice cevap", async () => {
    // AppShell bootstrap: her iki taraf prekey yükler + PreKeyStore alır
    as(bob);
    bob.store = await pk.ensurePreKeysUploaded(bob.identity, bob.pass);
    as(alice);
    alice.store = await pk.ensurePreKeysUploaded(alice.identity, alice.pass);

    // Alice: Bob'un bundle'ı (sunucu bir OPK verir → 4-DH yolu), konuşma, oturum
    const bundle = await api.getPreKeyBundle(bob.did);
    expect(bundle.one_time_prekey).toBeTruthy();
    const conv = await api.createConversation({ peer_did: bob.did });
    const convId = conv.conv_id;
    expect(convId).toBeTruthy();
    const init = await sess.initiateSession(alice.identity, bundle, convId);

    // İki mesaj ARKA ARKAYA (aynı saniye): ilki X3DH zarfı taşır, ikincisi taşımaz
    const secret1 = "ilk gizli mesaj " + Math.random().toString(36).slice(2);
    const secret2 = "ikinci gizli mesaj " + Math.random().toString(36).slice(2);
    const m1 = await sess.encryptForSend(init.state, secret1, convId, init.x3dhInit);
    const m2 = await sess.encryptForSend(m1.newState, secret2, convId);
    expect(m1.ciphertext).not.toContain(secret1);
    expect(m1.ciphertext).toContain("x3dhEphemeral");
    expect(m2.ciphertext).not.toContain("x3dhEphemeral");
    await api.sendMessage({ to_id: bob.did, ciphertext: m1.ciphertext, type: "text" });
    await api.sendMessage({ to_id: bob.did, ciphertext: m2.ciphertext, type: "text" });

    // Bob: sunucunun döndürdüğü sırayı OLDUĞU GİBİ kullanır (UI böyle yapıyor)
    as(bob);
    const bobConv = (await api.getConversations()).find((c: any) => c.peer_did === alice.did);
    expect(bobConv).toBeTruthy();
    expect(bobConv.id).toBe(convId);
    const msgs = (await api.getMessages(bobConv.id)) as any[];
    expect(msgs).toHaveLength(2);

    // 1) gönderim sırası: X3DH zarflı ilk mesaj önce
    expect(msgs[0].ciphertext).toContain("x3dhEphemeral");
    expect(msgs[1].ciphertext).not.toContain("x3dhEphemeral");
    // 2) sent_at gerçek (sıfır zaman değil)
    for (const m of msgs) {
      const t = new Date(m.sent_at).getTime();
      expect(t).toBeGreaterThan(Date.UTC(2020, 0, 1));
      expect(Math.abs(Date.now() - t)).toBeLessThan(5 * 60 * 1000);
    }
    // 3) sunucu yalnız ham blob görür
    for (const m of msgs) {
      expect(m.ciphertext).not.toContain(secret1);
      expect(m.ciphertext).not.toContain(secret2);
    }
    // 4) sıralı decrypt: ikisi de doğru plaintext
    const noop = () => {};
    expect(await sess.decryptIncoming(bobConv.id, msgs[0].ciphertext, bob.identity, bob.store, noop)).toBe(secret1);
    expect(await sess.decryptIncoming(bobConv.id, msgs[1].ciphertext, bob.identity, bob.store, noop)).toBe(secret2);

    // Bob → Alice cevap
    const bobState = await sess.loadSession(bobConv.id);
    expect(bobState).not.toBeNull();
    const secret3 = "cevap " + Math.random().toString(36).slice(2);
    const reply = await sess.encryptForSend(bobState!, secret3, bobConv.id);
    await api.sendMessage({ to_id: alice.did, ciphertext: reply.ciphertext, type: "text" });

    as(alice);
    const aliceMsgs = (await api.getMessages(convId)) as any[];
    const fromBob = aliceMsgs.find((m: any) => m.from_did === bob.did);
    expect(fromBob).toBeTruthy();
    expect(fromBob.ciphertext).not.toContain(secret3);
    expect(await sess.decryptIncoming(convId, fromBob.ciphertext, alice.identity, alice.store, noop)).toBe(secret3);
  });
});
