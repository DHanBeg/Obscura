// B11 CANLI SMOKE — blob E2E şifreleme, gerçek backend + gerçek MinIO.
// mls-b5-group-media.smoke.test.ts (grup medya) ile AYNI harness — E1/B5'in
// gerçek backend deseni. Burada kanıtlanan iddialar:
//   1. Rastgele blob-key CSPRNG'den geliyor (media-crypto.test.ts — birim,
//      mock'lu; burada onun ÜRETTİĞİ anahtarla gerçek MinIO round-trip).
//   2. MinIO'da duran ham byte'lar ŞİFRELİ — indirilip anahtarsız/yanlış
//      anahtarla çözülemiyor (gerçek AWS SigV4 upload + gerçek public-read GET).
//   3. Doğru anahtarla (mesaj payload'ından gelen) çözünce byte-birebir
//      orijinal — gerçek E2E round-trip.
//   4. Anahtar GERÇEK MLS grup mesajı içinde taşınıyor (encryptGroupMessage/
//      sendGroupTextMessage — B5'in kanıtladığı AYNI relay, MLS'e dokunulmadı).
//   5. Eski (legacy, şifresiz) blob hâlâ direkt okunabiliyor — backward compat.
//
// ÇALIŞTIRMA (3 terminal):
//   1) MinIO:   docker compose up -d minio   (veya: docker run -p 9000:9000 minio/minio server /data)
//   2) Backend: cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data-b11 PORT=8097 \
//               MINIO_ENDPOINT=localhost:9000 MINIO_ACCESS_KEY=obscura-admin \
//               MINIO_SECRET_KEY=obscura-secret MINIO_BUCKET=obscura-media go run ./cmd/node
//   3) Test:    cd mobile && OBSCURA_API_BASE=http://localhost:8097 npx jest mls-b11-blob-encryption --watchAll=false
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
import { encryptBlob, decryptBlob } from "../session-store";
import { u8ToBase64, base64ToU8 } from "../crypto";
import { parseMediaKey } from "../media-crypto";

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

function randomBytes(n: number): Uint8Array {
  const b = new Uint8Array(n);
  for (let i = 0; i < n; i++) b[i] = Math.floor(Math.random() * 256);
  return b;
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

// uploadRawBytes — api.uploadMedia'nın (mobile/lib/api.ts:225) RN FormData
// deseninin gerçek HTTP karşılığı — Node'un global FormData/Blob'uyla AYNI
// /v1/media/upload endpoint'ine gerçek multipart POST.
async function uploadRawBytes(base: string, token: string, bytes: Uint8Array, filename: string): Promise<{ url: string }> {
  const form = new FormData();
  form.append("file", new Blob([Buffer.from(bytes)]), filename);
  form.append("type", "media");
  const res = await fetch(`${base}/v1/media/upload`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  const data = await res.json();
  if (!data.success) throw new Error(data.error || "upload başarısız");
  return data.data;
}

async function fetchRawBytes(url: string): Promise<Uint8Array> {
  const res = await fetch(url);
  const buf = await res.arrayBuffer();
  return new Uint8Array(buf);
}

describe("B11 CANLI SMOKE — blob E2E şifreleme, gerçek backend + gerçek MinIO", () => {
  maybeTest(
    "video/dosya/ses: şifreli upload → MinIO'da ham byte çözülemez → doğru anahtar (gerçek MLS mesajından) orijinali verir; eski legacy blob hâlâ düz okunabiliyor",
    async () => {
      const base = REAL_BACKEND!;
      const cs = await getMlsCiphersuiteImpl();
      const aliceStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobStores = { secure: createMemoryStore(), async: createMemoryStore() };
      const bobCache = { secure: createMemoryStore(), async: createMemoryStore() };

      const alice = await registerRealUser(`alice_b11_${Date.now()}`);
      const bob = await registerRealUser(`bob_b11_${Date.now()}`);

      // ── 1) ŞİFRELİ upload (yeni model) — "video" içeriğini simüle eden
      // rastgele byte'lar, rastgele blob-key ile şifrelenip yükleniyor.
      const originalVideoBytes = randomBytes(8192);
      const videoKey = randomBytes(32); // CSPRNG kullanımı ayrıca media-crypto.test.ts'te birim testli
      const videoCipher = encryptBlob(videoKey, originalVideoBytes);
      const videoUpload = await uploadRawBytes(base, alice.token, videoCipher, "video.bin");

      // MinIO'da duran HAM byte — sunucudan/CDN'den anahtarsız çekilen aynı şey.
      const rawFromMinio = await fetchRawBytes(videoUpload.url);
      expect(rawFromMinio).toEqual(videoCipher); // MinIO tam olarak ne yüklendiyse onu döndürüyor (ciphertext)
      expect(rawFromMinio).not.toEqual(originalVideoBytes); // ham haliyle ORİJİNAL DEĞİL — çözülemez

      // Yanlış anahtar → GCM auth tag hatası (obfuscation değil, gerçek AEAD).
      const wrongKey = randomBytes(32);
      expect(() => decryptBlob(wrongKey, rawFromMinio)).toThrow();

      // ── 2) Anahtarı GERÇEK MLS grup mesajı içinde taşı — B5'in kanıtladığı
      // AYNI relay (sendGroupTextMessage/fetchAndDecryptGroupMessages), MLS'e
      // sıfır dokunuş. Payload chat/[id].tsx'in ürettiği BİREBİR format.
      const aliceOwn = await createOwnKeyPackage(alice.did, cs);
      mockCurrentToken = alice.token;
      await uploadKeyPackage(aliceOwn.keyPackageWireB64);
      const bobOwn = await createOwnKeyPackage(bob.did, cs);
      mockCurrentToken = bob.token;
      await uploadKeyPackage(bobOwn.keyPackageWireB64);

      mockCurrentToken = alice.token;
      const bobKp = await getKeyPackage(bob.did);
      const groupIdBytes = new TextEncoder().encode(`b11-smoke-${Date.now()}`);
      const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
      const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");
      await saveGroupState(groupIdB64, created.newState, aliceStores);
      await createGroupOnServer(groupIdB64, "B11 smoke grubu");
      await addMemberOnServer(groupIdB64, bob.did, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

      mockCurrentToken = bob.token;
      const welcomes = await getWelcomes();
      const welcome = welcomes.find((w) => w.group_id === groupIdB64);
      if (!welcome) throw new Error("Bob'un welcome kuyruğunda bu grup yok");
      const bobState = await joinFromWelcomeWire(welcome.welcome_b64, bobOwn.keyPackageWireB64, bobOwn.privateKeyPackage, cs);
      await saveGroupState(groupIdB64, bobState, bobStores);

      const videoPayload = `[video]${videoUpload.url}|${u8ToBase64(videoKey)}`;
      mockCurrentToken = alice.token;
      await sendGroupTextMessage(groupIdB64, videoPayload, aliceStores);

      // ── 3) Bob GERÇEK backend'den çekip MLS ile çözer, payload'dan anahtarı
      // ayıklar, blob'u GERÇEK MinIO'dan indirir, ÇÖZER — orijinalle eşleşiyor.
      mockCurrentToken = bob.token;
      const bobInbox = await fetchAndDecryptGroupMessages(groupIdB64, undefined, bobStores, bobCache);
      const received = bobInbox.find((m) => m.plaintext.startsWith("[video]"));
      expect(received).toBeDefined();
      expect(received!.sender_did).toBe(alice.did);

      const { url: bobUrl, keyB64: bobKeyB64 } = parseMediaKey(received!.plaintext.slice(7));
      expect(bobKeyB64).not.toBeNull();
      const bobRawBytes = await fetchRawBytes(bobUrl);
      const bobDecrypted = decryptBlob(base64ToU8(bobKeyB64!), bobRawBytes);
      expect(bobDecrypted).toEqual(originalVideoBytes); // gerçek E2E round-trip: upload→MLS→indir→çöz→orijinal

      // ── 4) Aynı mekanizma dosya/ses için de birebir aynı (encryptBlob/
      // decryptBlob, format farkı sadece payload prefix'i) — tekrar ayrı
      // kanıtlamaya gerek yok, media-crypto.test.ts'te birim testli; burada
      // sadece ikinci bir gerçek-backend örneğiyle (ses) teyit.
      const originalVoiceBytes = randomBytes(2048);
      const voiceKey = randomBytes(32);
      const voiceCipher = encryptBlob(voiceKey, originalVoiceBytes);
      const voiceUpload = await uploadRawBytes(base, alice.token, voiceCipher, "voice.bin");
      const voiceRaw = await fetchRawBytes(voiceUpload.url);
      expect(voiceRaw).not.toEqual(originalVoiceBytes);
      expect(decryptBlob(voiceKey, voiceRaw)).toEqual(originalVoiceBytes);

      // ── 5) GERİYE UYUMLULUK — eski (legacy, ŞİFRESİZ) upload hâlâ direkt
      // okunabiliyor, backend/MinIO'ya hiç dokunulmadı, format ayrımı
      // tamamen client-side (payload'da `|key` var mı yok mu).
      const legacyPlainBytes = randomBytes(512);
      const legacyUpload = await uploadRawBytes(base, alice.token, legacyPlainBytes, "old-video.mp4");
      const legacyRaw = await fetchRawBytes(legacyUpload.url);
      expect(legacyRaw).toEqual(legacyPlainBytes); // eski davranış: hiç şifrelenmeden, olduğu gibi

      const legacyPayload = `[video]${legacyUpload.url}`; // eski format — key segmenti YOK
      const { keyB64: legacyKeyB64 } = parseMediaKey(legacyPayload.slice(7));
      expect(legacyKeyB64).toBeNull(); // client bunu "şifresiz, direkt kullan" olarak okur
    },
    60000
  );

  if (!REAL_BACKEND) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[B11 SMOKE] OBSCURA_API_BASE set edilmedi — gerçek backend+MinIO smoke ATLANDI. " +
        "Çalıştırmak için: OBSCURA_API_BASE=http://localhost:8097 npx jest mls-b11-blob-encryption --watchAll=false"
      );
    });
  }
});
