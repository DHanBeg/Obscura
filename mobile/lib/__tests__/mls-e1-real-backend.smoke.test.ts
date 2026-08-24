// E1 CANLI SMOKE (ADR-0019 kapanış kriteri) — jest-mock relay testleri
// (mls-e2e-mock-relay.test.ts) bu SMOKE'un YERİNİ TUTMAZ. Burada:
//   - GERÇEK HTTP (Node http modülü üzerinden — XMLHttpRequest Jest'in node
//     testEnvironment'ında hiç yok, bkz. test-utils/nodeXhrPolyfill.ts),
//   - GERÇEK /v1/auth/* (OTP + JWT, backend'in kendi auth.GenerateToken'ı),
//   - GERÇEK /v1/mls/* relay (mock DEĞİL, gerçek mls_handlers.go),
//   - chat/[id].tsx'in FİİLEN ÇAĞIRDIĞI groupChat.ts fonksiyonları
//     (sendGroupTextMessage/fetchAndDecryptGroupMessages) — el ile
//     encryptGroupMessage/sendGroupMessage tekrar yazılmıyor.
// birlikte kanıtlanıyor.
//
// ÇALIŞTIRMA (2 terminal):
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 npx jest mls-e1-real-backend.smoke --watchAll=false
//
// OBSCURA_API_BASE set değilse test SKIP edilir (jest çıktısında görünür
// "skipped" — kalem 0'ın bulduğu sessiz-CI-skip hatasına burada DÜŞMÜYORUZ,
// varsayılan `npm test` bu dosyayı sessizce YEŞİL yapmaz, açıkça atlar).
let mockCurrentToken: string | null = null;
jest.mock("expo-secure-store", () => ({
  // api.ts:getToken() gerçek SecureStore'u okur; Jest'in node testEnvironment'ında
  // native köprü yok (requireNativeModule sessizce no-op döner, bkz.
  // test-utils/memoryStore.ts yorumu) — bu yüzden auth token'ı BURADA, testin
  // kontrol ettiği bir değişkenden veriyoruz (iki aktörü tek process'te
  // simüle etmenin auth-katmanı karşılığı, bkz. mls-e2e-mock-relay.test.ts'in
  // relay.setActor'ı — burada actor = hangi token aktif).
  getItemAsync: jest.fn(() => Promise.resolve(mockCurrentToken)),
  setItemAsync: jest.fn(() => Promise.resolve()),
  deleteItemAsync: jest.fn(() => Promise.resolve()),
}));

import { installNodeXhrPolyfill } from "../../test-utils/nodeXhrPolyfill";
installNodeXhrPolyfill();

import { apiFetch } from "../api";
import { getMlsCiphersuiteImpl, createOwnKeyPackage, createGroupWithMember, joinFromWelcomeWire } from "../mls/group";
import {
  uploadKeyPackage,
  getKeyPackage,
  createGroup as createGroupOnServer,
  addMember as addMemberOnServer,
  getWelcomes,
} from "../mls/mlsApi";
import { saveGroupState } from "../mls/mls-store";
import { sendGroupTextMessage, fetchAndDecryptGroupMessages } from "../mls/groupChat";
import { createMemoryStore } from "../../test-utils/memoryStore";

const REAL_BACKEND = process.env.OBSCURA_API_BASE;
const maybeTest = REAL_BACKEND ? test : test.skip;

function randomPhone(): string {
  const n = Math.floor(1000000 + Math.random() * 8999999);
  return `+1555${n}`;
}

function randomIdentityKeyB64(): string {
  const bytes = new Uint8Array(32);
  for (let i = 0; i < 32; i++) bytes[i] = Math.floor(Math.random() * 256);
  return Buffer.from(bytes).toString("base64");
}

// loginAndRegister — backend/internal/api/integration_test.go'daki Go
// yardımcısıyla AYNI sözleşme (dev/otp + verify-otp tek adımda username+
// identity_key vererek yeni kullanıcı akışını atlıyor, bkz. handlers.go:186).
async function registerRealUser(username: string): Promise<{ token: string; did: string }> {
  const phone = randomPhone();
  await apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) });
  const dev = await apiFetch(`/v1/dev/otp?phone=${encodeURIComponent(phone)}`);
  const otp = dev.otp;
  if (!otp) throw new Error("registerRealUser: dev/otp boş döndü — OBSCURA_ENV=development set mi?");
  const res = await apiFetch("/v1/auth/verify-otp", {
    method: "POST",
    body: JSON.stringify({ phone, otp, username, identity_key: randomIdentityKeyB64() }),
  });
  if (!res.token) throw new Error("registerRealUser: token dönmedi");
  return { token: res.token, did: res.user.did };
}

describe("E1 CANLI SMOKE — gerçek backend /v1/mls/*", () => {
  maybeTest(
    "Alice grup kurar, Bob katılır, iki yönlü ardışık mesajlaşma gerçek backend'de çalışır (generation-persistence dahil)",
    async () => {
      const cs = await getMlsCiphersuiteImpl();
      const aliceStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const aliceCache = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobCache = { secure: createMemoryStore(), async: createMemoryStore() };

      // 1. Alice + Bob gerçek OTP akışıyla kayıt olur, gerçek JWT alır.
      const alice = await registerRealUser(`alice_${Date.now()}`);
      const bob = await registerRealUser(`bob_${Date.now()}`);

      // 2. Her ikisi kendi KeyPackage'ını üretir + backend'e yükler (gerçek
      // /v1/mls/key-package, Bearer token gerçek — auth'un fiilen geçtiğinin
      // kanıtı: token yanlış/eksik olsa 401 fırlatırdı, aşağıdaki adımlar 401
      // fırlatmadan geçiyorsa auth doğru işliyor demektir).
      const aliceOwn = await createOwnKeyPackage(alice.did, cs);
      mockCurrentToken = alice.token;
      await uploadKeyPackage(aliceOwn.keyPackageWireB64);

      const bobOwn = await createOwnKeyPackage(bob.did, cs);
      mockCurrentToken = bob.token;
      await uploadKeyPackage(bobOwn.keyPackageWireB64);

      // 3. Alice Bob'un KP'sini çeker, grup kurar + Bob'u ekler (local crypto).
      mockCurrentToken = alice.token;
      const bobKp = await getKeyPackage(bob.did);
      const groupIdBytes = new TextEncoder().encode(`e1-smoke-${Date.now()}`);
      const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
      const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");

      // 4. LOCAL ÖNCE (createGroupFlow.ts ile AYNI ilke) — sonra backend'e bildir.
      await saveGroupState(groupIdB64, created.newState, aliceStores);
      await createGroupOnServer(groupIdB64, "E1 smoke grubu");
      await addMemberOnServer(groupIdB64, bob.did, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

      // 5. Bob welcome'ı gerçek backend'den çeker, katılır, kaydeder.
      mockCurrentToken = bob.token;
      const welcomes = await getWelcomes();
      const welcome = welcomes.find((w) => w.group_id === groupIdB64);
      if (!welcome) throw new Error("Bob'un welcome kuyruğunda bu grup yok");
      const bobState = await joinFromWelcomeWire(welcome.welcome_b64, bobOwn.keyPackageWireB64, bobOwn.privateKeyPackage, cs);
      await saveGroupState(groupIdB64, bobState, bobStores);

      // 6. Alice → Bob: chat/[id].tsx'in ÇAĞIRDIĞI fonksiyon, gerçek backend'e.
      mockCurrentToken = alice.token;
      await sendGroupTextMessage(groupIdB64, "merhaba bob (mesaj 1)", aliceStores);

      // 7. Bob gerçek backend'den çekip ts-mls ile çözer (ADR-0019 kapanış cümlesi
      // birebir: "başka bir üyenin bunu ts-mls ile çözdüğü an kapanmış sayılır").
      mockCurrentToken = bob.token;
      const bobInbox1 = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
      expect(bobInbox1.map((m) => m.plaintext)).toContain("merhaba bob (mesaj 1)");
      expect(bobInbox1.find((m) => m.plaintext === "merhaba bob (mesaj 1)")?.sender_did).toBe(alice.did);

      // 8. Alice İKİNCİ bir mesaj gönderir — group.ts:EncryptedGroupMessage.newState
      // fix'inin kanıtı: newState persist edilmeseydi bu ikinci şifreleme
      // BİRİNCİYLE AYNI ratchet generation'ı kullanır (nonce/key reuse), Bob
      // ya çözemez ya da (daha kötü) sessizce yanlış/eski anahtarla "çözer".
      mockCurrentToken = alice.token;
      await sendGroupTextMessage(groupIdB64, "merhaba bob (mesaj 2 — generation ilerlemis mi?)", aliceStores);

      mockCurrentToken = bob.token;
      const bobInbox2 = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
      expect(bobInbox2.map((m) => m.plaintext)).toContain("merhaba bob (mesaj 2 — generation ilerlemis mi?)");
      expect(bobInbox2.length).toBe(2); // ilk mesaj da hâlâ çözülebiliyor (fetch tekrar tüm geçmişi çeker)

      // 9. Bob → Alice: ters yön, aynı grup state'iyle.
      mockCurrentToken = bob.token;
      await sendGroupTextMessage(groupIdB64, "selam alice, ben de yaziyorum", bobStores);

      mockCurrentToken = alice.token;
      const aliceInbox = await fetchAndDecryptGroupMessages(groupIdB64, undefined, aliceStores, aliceCache);
      const fromBob = aliceInbox.find((m) => m.sender_did === bob.did);
      expect(fromBob?.plaintext).toBe("selam alice, ben de yaziyorum");
    },
    60000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[E1 SMOKE] OBSCURA_API_BASE set edilmedi — gerçek backend smoke ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8099 npx jest mls-e1-real-backend.smoke"
      );
    });
  }
});
