// B10 Faz 1 CROSS-PLATFORM SMOKE — mls-e1-real-backend.smoke.test.ts ile AYNI
// desen (GERÇEK HTTP, GERÇEK /v1/auth/*, GERÇEK /v1/mls/* relay), ama Bob
// artık mobile DEĞİL — frontend/lib/mls/* (bu turda web'e taşınan, YENİ)
// dosyaları çalıştıran ayrı bir process ("web aktörü", frontend/_b10_web_actor.ts,
// npx tsx ile). Mobile tarafı (Alice) GERÇEK, DEĞİŞMEMİŞ mobile/lib/mls/*.
// İkisi SADECE gerçek backend üzerinden konuşuyor — process'ler arası hiçbir
// doğrudan bağlantı yok (mobile'ın kendi smoke test'inin actor-simülasyonu
// gibi ama artık gerçek iki farklı kod tabanı).
//
// KANIT ETMEK İSTEDİĞİ: mobile MLS grubu kurar → web (YENİ kod) JOIN eder →
// web mesaj gönderir → mobile çözer; SONRA mobile gönderir → web çözer.
// B10 Faz 1'in "bitti"si tam bu — web ve mobile AYNI MLS grubunda buluşuyor.
//
// ÇALIŞTIRMA (2 terminal):
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data-b10 PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 npx jest mls-b10-web-cross-platform --watchAll=false
//
// GEÇİCİ dosyalar (frontend/_b10_web_actor.ts, frontend/_b10_file_store.ts,
// bu test dosyası) — kanıt raporlandıktan sonra silinecek, commit'e girmeyecek.
let mockCurrentToken: string | null = null;
jest.mock("expo-secure-store", () => ({
  getItemAsync: jest.fn(() => Promise.resolve(mockCurrentToken)),
  setItemAsync: jest.fn(() => Promise.resolve()),
  deleteItemAsync: jest.fn(() => Promise.resolve()),
}));

import { installNodeXhrPolyfill } from "../../test-utils/nodeXhrPolyfill";
installNodeXhrPolyfill();

import { execFileSync } from "child_process";
import * as path from "path";
import * as fs from "fs";
import { apiFetch } from "../api";
import { getMlsCiphersuiteImpl, createOwnKeyPackage, createGroupWithMember } from "../mls/group";
import { uploadKeyPackage, getKeyPackage, createGroup as createGroupOnServer, addMember as addMemberOnServer } from "../mls/mlsApi";
import { saveGroupState } from "../mls/mls-store";
import { sendGroupTextMessage, fetchAndDecryptGroupMessages } from "../mls/groupChat";
import { createMemoryStore } from "../../test-utils/memoryStore";

const REAL_BACKEND = process.env.OBSCURA_API_BASE;
const maybeTest = REAL_BACKEND ? test : test.skip;

const FRONTEND_DIR = path.resolve(__dirname, "../../../frontend");
const WEB_ACTOR = path.resolve(FRONTEND_DIR, "_b10_web_actor.ts");

function runWebActor(args: string[], extraEnv: Record<string, string> = {}): string {
  const stateFile = path.resolve(FRONTEND_DIR, "_b10_web_actor_state.json");
  const kvFile = path.resolve(FRONTEND_DIR, "_b10_web_actor_kv.json");
  // npx .cmd (Windows) — shell:false EINVAL veriyor, shell:false+npx.cmd de
  // EINVAL. shell:true GEREKLİ (Windows'ta npx bir batch script). Serbest
  // metin (mesaj gövdesi) argv'ye KOYULMUYOR — BULUNDU: shell:true args'ı
  // TEK STRING'e birleştirip kabuğa veriyor (kaçışsız), tek tırnak içeren
  // mesaj argümanı kesiliyordu. argv'de SADECE güvenli token'lar (phase,
  // base64 groupId); serbest metin env var ile taşınıyor (env kabuk
  // tokenize'ından geçmiyor).
  return execFileSync("npx", ["tsx", WEB_ACTOR, ...args], {
    cwd: FRONTEND_DIR,
    env: {
      ...process.env,
      NEXT_PUBLIC_API_URL: REAL_BACKEND,
      B10_ACTOR_STATE: stateFile,
      B10_ACTOR_KV: kvFile,
      ...extraEnv,
    },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    shell: true,
  }).trim();
}

function randomPhone(): string {
  const n = Math.floor(1000000 + Math.random() * 8999999);
  return `+1555${n}`;
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

describe("B10 Faz 1 CROSS-PLATFORM SMOKE — mobile (gerçek) ↔ web (YENİ, gerçek) aynı MLS grubunda", () => {
  maybeTest(
    "mobile grup kurar, web JOIN eder + gönderir (mobile çözer), mobile gönderir (web çözer)",
    async () => {
      const cs = await getMlsCiphersuiteImpl();
      const aliceStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const aliceCache = { secure: createMemoryStore(), async: createMemoryStore() };

      // 1. Alice (mobile, GERÇEK kod) kayıt olur.
      const alice = await registerRealUser(`b10_alice_${Date.now()}`);

      // 2. Bob (WEB, YENİ frontend/lib/mls/* kodu, ayrı process) kayıt olur +
      // kendi KeyPackage'ını üretip backend'e yükler.
      const bobJson = runWebActor(["register"]);
      const bob = JSON.parse(bobJson) as { did: string; token: string };
      expect(bob.did).toMatch(/^did:obs:/);

      // 3. Alice (mobile) Bob'un KP'sini çeker, grup kurar + Bob'u ekler.
      mockCurrentToken = alice.token;
      const aliceOwn = await createOwnKeyPackage(alice.did, cs);
      await uploadKeyPackage(aliceOwn.keyPackageWireB64);
      const bobKp = await getKeyPackage(bob.did);
      const groupIdBytes = new TextEncoder().encode(`b10-cross-${Date.now()}`);
      const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
      const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");

      await saveGroupState(groupIdB64, created.newState, aliceStores);
      await createGroupOnServer(groupIdB64, "B10 cross-platform proof");
      await addMemberOnServer(groupIdB64, bob.did, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

      // 4. Bob (WEB) welcome'ı çeker, JOIN eder (frontend/lib/mls/joinGroupFlow.ts,
      // YENİ kod), sonra mesaj 1'i gönderir (frontend/lib/mls/groupChat.ts, YENİ kod).
      const msg1 = "web'den mobil grubuna JOIN + mesaj 1 (B10 Faz1 kanıtı)";
      const joinRes = runWebActor(["join-and-send", groupIdB64], { B10_MSG_TEXT: msg1 });
      expect(JSON.parse(joinRes).joined).toBe(true);

      // 5. Alice (mobile) çeker + çözer — WEB'İN GÖNDERDİĞİ mesajı GERÇEK MLS ile.
      mockCurrentToken = alice.token;
      const aliceInbox1 = await fetchAndDecryptGroupMessages(groupIdB64, undefined, aliceStores, aliceCache);
      const fromWeb = aliceInbox1.find((m) => m.plaintext === msg1);
      expect(fromWeb).toBeDefined();
      expect(fromWeb?.sender_did).toBe(bob.did);

      // 6. Alice (mobile) mesaj 2'yi gönderir — TERS YÖN.
      const msg2 = "mobil'den web'e mesaj 2 (B10 Faz1 kanıtı, ters yön)";
      await sendGroupTextMessage(groupIdB64, msg2, aliceStores);

      // 7. Bob (WEB) çeker + çözer — MOBILE'İN GÖNDERDİĞİ mesajı YENİ web kodu ile.
      const bobInboxJson = runWebActor(["receive", groupIdB64]);
      const bobInbox = JSON.parse(bobInboxJson) as Array<{ plaintext: string; sender_did: string }>;
      const fromMobile = bobInbox.find((m) => m.plaintext === msg2);
      expect(fromMobile).toBeDefined();
      expect(fromMobile?.sender_did).toBe(alice.did);
    },
    120000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[B10 CROSS-PLATFORM SMOKE] OBSCURA_API_BASE set edilmedi — ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8099 npx jest mls-b10-web-cross-platform"
      );
    });
  }
});
