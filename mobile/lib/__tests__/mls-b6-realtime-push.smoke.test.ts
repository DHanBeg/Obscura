// B6 CANLI SMOKE — grup mesajlaşmada 4sn polling yerine WS "mls_message"
// push'u (mls_handlers.go:435). E1 smoke ile AYNI desen (mls-e1-real-backend.
// smoke.test.ts): gerçek HTTP + gerçek OTP/JWT + gerçek /v1/mls/* relay,
// chat/[id].tsx'in FİİLEN ÇAĞIRDIĞI fonksiyonlar (createWS, sendGroupTextMessage,
// fetchAndDecryptGroupMessages) — el ile yeniden yazılmıyor, MLS crypto'ya
// dokunulmuyor (guardrail).
//
// İki iddiayı kanıtlar:
//   1. Anlık teslimat: Alice mesaj gönderince Bob'un WS'i "mls_message"
//      event'ini POLLING ARALIĞINI (eski 4000ms) BEKLEMEDEN alır.
//   2. WS-kopma/reconnect: Bob'un WS'i KASITLI koparılır, kopukken Alice
//      ikinci bir mesaj gönderir (kaybolmaz — mls_messages tablosuna WS
//      broadcast'ten ÖNCE yazılır, mls_handlers.go:409-416), reconnect
//      (lib/api.ts:373-376, createWS'in kendi 3sn'lik mekanizması) sonrası
//      chat/[id].tsx'in reconnect-catchup effect'inin yapacağı AYNI çağrı
//      (fetchAndDecryptGroupMessages) kaçan mesajı gerçekten yakalıyor.
//
// ÇALIŞTIRMA (2 terminal):
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 OBSCURA_WS_BASE=ws://localhost:8099 \
//        npx jest mls-b6-realtime-push.smoke --watchAll=false
let mockCurrentToken: string | null = null;
jest.mock("expo-secure-store", () => ({
  getItemAsync: jest.fn(() => Promise.resolve(mockCurrentToken)),
  setItemAsync: jest.fn(() => Promise.resolve()),
  deleteItemAsync: jest.fn(() => Promise.resolve()),
}));

import { installNodeXhrPolyfill } from "../../test-utils/nodeXhrPolyfill";
installNodeXhrPolyfill();
import { installNodeWebSocketHeaderShim } from "../../test-utils/nodeWebSocketShim";
installNodeWebSocketHeaderShim();

import { apiFetch, createWS } from "../api";
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
  return `+1556${n}`;
}

function randomIdentityKeyB64(): string {
  const bytes = new Uint8Array(32);
  for (let i = 0; i < 32; i++) bytes[i] = Math.floor(Math.random() * 256);
  return Buffer.from(bytes).toString("base64");
}

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

describe("B6 CANLI SMOKE — WS mls_message push + reconnect", () => {
  maybeTest(
    "anlık teslimat (polling beklemeden) + WS kopma sonrası kaçan mesaj reconnect-catchup ile geliyor",
    async () => {
      const cs = await getMlsCiphersuiteImpl();
      const aliceStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobCache = { secure: createMemoryStore(), async: createMemoryStore() };

      const log = (s: string) => console.log(`[B6 SMOKE] ${Date.now() % 100000}ms: ${s}`);

      // 1-5: E1 smoke ile birebir aynı grup bootstrap'ı (Alice kurar, Bob katılır).
      log("register alice");
      const alice = await registerRealUser(`b6alice_${Date.now()}`);
      log("register bob");
      const bob = await registerRealUser(`b6bob_${Date.now()}`);

      log("alice keypackage");
      const aliceOwn = await createOwnKeyPackage(alice.did, cs);
      mockCurrentToken = alice.token;
      await uploadKeyPackage(aliceOwn.keyPackageWireB64);

      log("bob keypackage");
      const bobOwn = await createOwnKeyPackage(bob.did, cs);
      mockCurrentToken = bob.token;
      await uploadKeyPackage(bobOwn.keyPackageWireB64);

      mockCurrentToken = alice.token;
      const bobKp = await getKeyPackage(bob.did);
      const groupIdBytes = new TextEncoder().encode(`b6-smoke-${Date.now()}`);
      const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
      const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");

      log("create group on server");
      await saveGroupState(groupIdB64, created.newState, aliceStores);
      await createGroupOnServer(groupIdB64, "B6 smoke grubu");
      await addMemberOnServer(groupIdB64, bob.did, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

      log("bob joins via welcome");
      mockCurrentToken = bob.token;
      const welcomes = await getWelcomes();
      const welcome = welcomes.find((w) => w.group_id === groupIdB64);
      if (!welcome) throw new Error("Bob'un welcome kuyruğunda bu grup yok");
      const bobState = await joinFromWelcomeWire(welcome.welcome_b64, bobOwn.keyPackageWireB64, bobOwn.privateKeyPackage, cs);
      await saveGroupState(groupIdB64, bobState, bobStores);
      log("bootstrap tamam");

      // 6. Bob'un WS'i — chat/[id].tsx'in dolaylı olarak dayandığı AYNI createWS
      // (lib/api.ts:361), _layout.tsx'teki "mls_message" case'inin göreceği
      // AYNI event'i burada doğrudan yakalıyoruz.
      mockCurrentToken = bob.token;
      let openCount = 0;
      let closeCount = 0;
      let liveWs: WebSocket | null = null;
      const mlsMessageEvents: any[] = [];
      const initialWs = await new Promise<WebSocket>((resolve) => {
        const ws = createWS(
          bob.token,
          (msg) => { if (msg.type === "mls_message") mlsMessageEvents.push(msg.payload); },
          () => { openCount++; liveWs = ws; log(`WS open #${openCount}`); if (openCount === 1) resolve(ws); },
          () => { closeCount++; log(`WS close #${closeCount}`); },
        );
      });
      liveWs = initialWs;

      try {
        // 7. ANLIK TESLİMAT — Alice gönderir, Bob'un WS'i event'i eski 4000ms
        // polling aralığını BEKLEMEDEN alır (aşağıdaki bekleme üst sınırı 2000ms,
        // eski pollling aralığının yarısı — event daha erken gelmezse test FAIL olur).
        const t0 = Date.now();
        mockCurrentToken = alice.token;
        await sendGroupTextMessage(groupIdB64, "b6 anlık mesaj", aliceStores);
        log("alice mesaj 1 gönderdi, WS event bekleniyor");

        await waitFor(() => mlsMessageEvents.some((p) => p.group_id === groupIdB64), 2000);
        const deliveryMs = Date.now() - t0;
        expect(mlsMessageEvents.some((p) => p.group_id === groupIdB64)).toBe(true);
        expect(deliveryMs).toBeLessThan(2000); // eski 4000ms poll aralığının yarısından hızlı — "polling beklemeden" kanıtı
        log(`mls_message WS push gecikmesi: ${deliveryMs}ms (eski polling aralığı: 4000ms)`);

        // Bob decrypt eder (E1'in yolu — chat/[id].tsx'in nudge sonrası çağırdığı
        // AYNI fonksiyon), mesaj görünür olduğunu doğrula.
        mockCurrentToken = bob.token;
        const bobInbox1 = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
        expect(bobInbox1.map((m) => m.plaintext)).toContain("b6 anlık mesaj");
        log("mesaj 1 decrypt edildi");

        // 8. WS-KOPMA — Bob'un bağlantısını KASITLI kapat. createWS'in kendi
        // onclose handler'ı (lib/api.ts:373-376) 3sn sonra otomatik reconnect
        // dener — production'daki AYNI mekanizma, burada mock'lanmıyor.
        mlsMessageEvents.length = 0;
        log("WS kasıtlı kapatılıyor");
        liveWs!.close();
        await waitFor(() => closeCount >= 1, 2000);
        expect(closeCount).toBeGreaterThanOrEqual(1);

        // 9. Bob'un WS'i KOPUKKEN Alice ikinci bir mesaj gönderir — bu, fallback
        // polling kaldırıldıktan sonra "mesaj kaybolur mu" senaryosunun ta kendisi.
        mockCurrentToken = alice.token;
        await sendGroupTextMessage(groupIdB64, "b6 kopukken gönderilen mesaj", aliceStores);
        log("alice mesaj 2'yi Bob'un WS'i kopukken gönderdi, reconnect bekleniyor (~3sn)");

        // 10. RECONNECT — createWS'in otomatik reconnect'i (3sn) gerçekleşene
        // kadar bekle (production'da chat/[id].tsx bu open event'ini
        // store.wsConnected üzerinden görür ve reconnect-catchup fetch'ini tetikler).
        await waitFor(() => openCount >= 2, 8000);
        expect(openCount).toBeGreaterThanOrEqual(2);
        log("reconnect tamamlandı");

        // 11. RECONNECT-CATCHUP — chat/[id].tsx'in wsConnected false→true
        // effect'inin yapacağı BİREBİR AYNI çağrı: fetchAndDecryptGroupMessages.
        // Kopukken gönderilen mesaj burada görünmeli — mesaj kaybolmadı.
        mockCurrentToken = bob.token;
        const bobInbox2 = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
        expect(bobInbox2.map((m) => m.plaintext)).toContain("b6 kopukken gönderilen mesaj");
        log("WS kopması sırasında gönderilen mesaj, reconnect-catchup fetch'iyle yakalandı — kayıp yok.");
      } finally {
        // reconnect zinciri sonsuza kadar denemesin diye store.ts:reset()'teki
        // AYNI idiom: onclose'u önce söndür, sonra kapat.
        if (liveWs) { (liveWs as any).onclose = null; liveWs.close(); }
      }
    },
    60000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[B6 SMOKE] OBSCURA_API_BASE set edilmedi — gerçek backend smoke ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8099 OBSCURA_WS_BASE=ws://localhost:8099 " +
        "npx jest mls-b6-realtime-push.smoke --watchAll=false"
      );
    });
  }
});

function waitFor(cond: () => boolean, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const iv = setInterval(() => {
      if (cond()) { clearInterval(iv); resolve(); return; }
      if (Date.now() - start > timeoutMs) { clearInterval(iv); reject(new Error("waitFor timeout")); }
    }, 50);
  });
}
