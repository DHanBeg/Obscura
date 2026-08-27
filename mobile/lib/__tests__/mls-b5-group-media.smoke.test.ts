// B5 CANLI SMOKE — grup medyası (resim/video/dosya/konum/ses) gerçek backend'de.
// mls-e1-real-backend.smoke.test.ts'in AYNI harness'i (gerçek HTTP, gerçek
// /v1/auth/*, gerçek /v1/mls/* relay) — E1 sadece plaintext:string kapsıyordu,
// bu test chat/[id].tsx'in sendGroupMedia'sının ürettiği 5 payload formatını
// ([img]/[video]/[file]/[location]/[voice]) sendGroupTextMessage/
// fetchAndDecryptGroupMessages üzerinden AYNI yoldan gönderip çözüyor —
// kripto tarafı text'ten FARKSIZ olduğu iddiasının (bkz. B5 Faz 0 keşfi)
// canlı kanıtı.
//
// ÇALIŞTIRMA (2 terminal):
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 npx jest mls-b5-group-media --watchAll=false
let mockCurrentToken: string | null = null;
jest.mock("expo-secure-store", () => ({
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

describe("B5 CANLI SMOKE — grup medyası gerçek backend /v1/mls/*", () => {
  maybeTest(
    "Alice 5 medya tipini (resim/video/dosya/konum/ses payload formatı) gruba gönderir, Bob hepsini çözer",
    async () => {
      const cs = await getMlsCiphersuiteImpl();
      const aliceStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobCache = { secure: createMemoryStore(), async: createMemoryStore() };

      const alice = await registerRealUser(`alice_b5_${Date.now()}`);
      const bob = await registerRealUser(`bob_b5_${Date.now()}`);

      const aliceOwn = await createOwnKeyPackage(alice.did, cs);
      mockCurrentToken = alice.token;
      await uploadKeyPackage(aliceOwn.keyPackageWireB64);

      const bobOwn = await createOwnKeyPackage(bob.did, cs);
      mockCurrentToken = bob.token;
      await uploadKeyPackage(bobOwn.keyPackageWireB64);

      mockCurrentToken = alice.token;
      const bobKp = await getKeyPackage(bob.did);
      const groupIdBytes = new TextEncoder().encode(`b5-smoke-${Date.now()}`);
      const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
      const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");

      await saveGroupState(groupIdB64, created.newState, aliceStores);
      await createGroupOnServer(groupIdB64, "B5 smoke grubu");
      await addMemberOnServer(groupIdB64, bob.did, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

      mockCurrentToken = bob.token;
      const welcomes = await getWelcomes();
      const welcome = welcomes.find((w) => w.group_id === groupIdB64);
      if (!welcome) throw new Error("Bob'un welcome kuyruğunda bu grup yok");
      const bobState = await joinFromWelcomeWire(welcome.welcome_b64, bobOwn.keyPackageWireB64, bobOwn.privateKeyPackage, cs);
      await saveGroupState(groupIdB64, bobState, bobStores);

      // chat/[id].tsx sendGroupMedia'sının ürettiği 5 payload formatı, birebir
      // — image küçük bir sahte base64 (gerçek boyut ratchet/MLS'i etkilemez,
      // kripto tarafı text'ten farksız olduğu zaten Faz 0'da doğrulandı; media
      // upload/MinIO bu testin kapsamı dışı, sadece MLS relay yolu kanıtlanıyor).
      const payloads = {
        img: "[img]/9j/4AAQSkZJRgABAQAAAQABAAD_2wBDAAoHBwgHBgoICAgLCgoLDhgQDg0NDh0VFhEYIx8lJCIfIiEmKzcvJik0",
        video: "[video]https://media.obscura.test/b5-smoke/video.mp4",
        file: "[file]rapor.pdf|https://media.obscura.test/b5-smoke/rapor.pdf",
        location: "[location]41.0082,28.9784",
        voice: "[voice]https://media.obscura.test/b5-smoke/voice.m4a",
      } as const;

      mockCurrentToken = alice.token;
      for (const payload of Object.values(payloads)) {
        await sendGroupTextMessage(groupIdB64, payload, aliceStores);
      }

      mockCurrentToken = bob.token;
      const bobInbox = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
      const plaintexts = bobInbox.map((m) => m.plaintext);

      for (const [kind, payload] of Object.entries(payloads)) {
        expect(plaintexts).toContain(payload);
        const found = bobInbox.find((m) => m.plaintext === payload);
        expect(found?.sender_did).toBe(alice.did);
        // render katmanının (chat/[id].tsx renderMsgContent) prefix ayrıştırması
        // için gerçek koşul — çözülen düz metin beklenen prefix'le başlıyor mu.
        expect(payload.startsWith(`[${kind}]`)).toBe(true);
      }
      expect(bobInbox.length).toBe(Object.keys(payloads).length);
    },
    60000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[B5 SMOKE] OBSCURA_API_BASE set edilmedi — gerçek backend smoke ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8099 npx jest mls-b5-group-media --watchAll=false"
      );
    });
  }
});
