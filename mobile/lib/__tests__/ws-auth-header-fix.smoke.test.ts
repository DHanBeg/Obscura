// FAZ A CANLI SMOKE — WS auth regresyon fix'i (lib/api.ts:361-386, commit
// 1873709'dan beri 2 aydır kırıktı: token WS mesajı olarak gönderiliyordu,
// backend hiç okumuyordu → her bağlantı 401/1006, onopen hiç ateşlenmiyordu,
// 1:1/grup/call/presence real-time push'un TAMAMI ölüydü). Fix: token artık
// Authorization header'ında (RN + Node WebSocket native destekli, backend
// zaten destekliyordu — cmd/node/main.go:569-572).
//
// Bu test 1:1 "new_message" push'unu (handlers.go:987) kanıtlar — grup
// tarafı (mls_message) ayrı dosyada: mls-b6-realtime-push.smoke.test.ts.
// İki iddia:
//   1. createWS() (DEĞİŞTİRİLMEDİ, gerçek fonksiyon) artık gerçek backend'e
//      header'la bağlanıp OPEN oluyor, 1:1 mesaj push'u geliyor.
//   2. Reconnect (kasıtlı kopma sonrası createWS'in kendi 3sn mekanizması)
//      da AYNI header'ı taşıyor — sadece ilk bağlantı değil.
//
// ÇALIŞTIRMA:
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 OBSCURA_WS_BASE=ws://localhost:8099 \
//        npx jest ws-auth-header-fix.smoke --watchAll=false --forceExit
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

import { apiFetch, createWS, api } from "../api";

const REAL_BACKEND = process.env.OBSCURA_API_BASE;
const maybeTest = REAL_BACKEND ? test : test.skip;

function randomPhone(): string {
  const n = Math.floor(1000000 + Math.random() * 8999999);
  return `+1557${n}`;
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

function waitFor(cond: () => boolean, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const iv = setInterval(() => {
      if (cond()) { clearInterval(iv); resolve(); return; }
      if (Date.now() - start > timeoutMs) { clearInterval(iv); reject(new Error("waitFor timeout")); }
    }, 50);
  });
}

describe("FAZ A CANLI SMOKE — WS header auth fix", () => {
  maybeTest(
    "1:1 new_message push geliyor (2 aydır ölüydü) + reconnect de header'lı auth ediyor",
    async () => {
      const log = (s: string) => console.log(`[WS-FIX SMOKE] ${Date.now() % 100000}ms: ${s}`);

      const alice = await registerRealUser(`wsfix_alice_${Date.now()}`);
      const bob = await registerRealUser(`wsfix_bob_${Date.now()}`);
      log("alice+bob kayıtlı");

      let openCount = 0, closeCount = 0;
      let liveWs: WebSocket | null = null;
      const newMessageEvents: any[] = [];
      mockCurrentToken = bob.token;
      const initialWs = await new Promise<WebSocket>((resolve) => {
        const ws = createWS(
          bob.token,
          (msg) => { if (msg.type === "new_message") newMessageEvents.push(msg.payload ?? msg.data); },
          () => { openCount++; liveWs = ws; log(`WS open #${openCount} (header auth)`); if (openCount === 1) resolve(ws); },
          () => { closeCount++; log(`WS close #${closeCount}`); },
        );
      });
      liveWs = initialWs;

      try {
        // 1. ANLIK 1:1 PUSH — Alice Bob'a mesaj gönderir (raw REST /v1/messages,
        // dev'de sealed-sender required kapalı — InitSealedSenderPolicyFromEnv,
        // env yoksa varsayılan kapalı), Bob'un WS'i "new_message" alır.
        const t0 = Date.now();
        mockCurrentToken = alice.token;
        await api.sendMessage({ to_id: bob.did, type: "text", ciphertext: "wsfix-anlik-mesaj" });
        log("alice mesaj gönderdi, WS event bekleniyor");

        await waitFor(() => newMessageEvents.length > 0, 3000);
        const deliveryMs = Date.now() - t0;
        expect(newMessageEvents.length).toBeGreaterThan(0);
        log(`new_message WS push gecikmesi: ${deliveryMs}ms — 2 AYDIR HİÇ GELMİYORDU, artık geliyor`);

        // 2. RECONNECT DE HEADER TAŞIYOR — kasıtlı kopar, reconnect sonrası
        // yeni bir mesajla push'un YİNE çalıştığını doğrula (sadece ilk
        // bağlantı değil, createWS'in setTimeout ile kurduğu YENİ bağlantı da).
        newMessageEvents.length = 0;
        log("WS kasıtlı kapatılıyor");
        liveWs!.close();
        await waitFor(() => closeCount >= 1, 2000);

        await waitFor(() => openCount >= 2, 8000);
        log("reconnect tamamlandı (header ile yeniden auth oldu)");

        const t1 = Date.now();
        mockCurrentToken = alice.token;
        await api.sendMessage({ to_id: bob.did, type: "text", ciphertext: "wsfix-reconnect-sonrasi-mesaj" });
        await waitFor(() => newMessageEvents.length > 0, 3000);
        log(`reconnect SONRASI push gecikmesi: ${Date.now() - t1}ms — header reconnect'te de çalışıyor`);
      } finally {
        if (liveWs) { (liveWs as any).onclose = null; liveWs.close(); }
      }
    },
    30000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[WS-FIX SMOKE] OBSCURA_API_BASE set edilmedi — gerçek backend smoke ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8099 OBSCURA_WS_BASE=ws://localhost:8099 " +
        "npx jest ws-auth-header-fix.smoke --watchAll=false --forceExit"
      );
    });
  }
});
