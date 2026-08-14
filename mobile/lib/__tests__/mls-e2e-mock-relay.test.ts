// L2 Tuğla 4c — E1 uçtan-uca kanıt: 4a (kripto) + 4b (HTTP client) ilk kez
// BİRLİKTE. Mock relay, backend'in mls_handlers.go davranışını taklit eder
// (opak blob sakla/ver, içerik parse etmez) — gerçek backend'e bağlanmaz
// (o ayrı smoke), AsyncStorage/store persistence yok (ayrı tuğla), UI yok
// (Tuğla 5). Sadece: kripto → client → mock-relay → çözüm.
jest.mock("../api", () => ({ apiFetch: jest.fn() }));

import { apiFetch } from "../api";
import {
  getMlsCiphersuiteImpl,
  createOwnKeyPackage,
  createGroupWithMember,
  encryptGroupMessage,
  joinFromWelcomeWire,
  decryptApplicationMessageWire,
} from "../mls/group";
import {
  uploadKeyPackage,
  getKeyPackage,
  addMember,
  getWelcomes,
  sendGroupMessage,
  getGroupMessages,
  type PendingWelcome,
  type GroupMessageEntry,
} from "../mls/mlsApi";

const mockedApiFetch = apiFetch as unknown as jest.Mock;

/**
 * İki cihazlı (Alice/Bob) tek-process test için minimal relay taklidi.
 * Backend gibi: KeyPackage tek-kullanımlık (fetch = tüket), Welcome hedef
 * DID'e kuyruklanır, mesajlar group_id altında biriktirilir — içerik hiç
 * parse edilmez, sadece opak base64 blob olarak taşınır. `setActor` gerçek
 * cihazın kendi Bearer token'ıyla kimliğini taşımasının test-scope yerine
 * geçer (mlsApi.ts'te YENİ bir auth mekanizması YOK — bu sadece iki cihazı
 * tek process'te ayırt etmenin test-only yolu).
 */
function createMockRelay() {
  const keyPackages = new Map<string, { wire: string; used: boolean }>();
  const welcomeQueue = new Map<string, PendingWelcome[]>();
  const messages = new Map<string, GroupMessageEntry[]>();
  let actor = "";
  let seq = 0;
  const nextId = (prefix: string) => `${prefix}-${++seq}`;

  const handle = async (path: string, opts?: RequestInit): Promise<any> => {
    const method = (opts?.method as string) || "GET";
    const body = opts?.body ? JSON.parse(opts.body as string) : undefined;

    if (method === "POST" && path === "/v1/mls/key-package") {
      keyPackages.set(actor, { wire: body.key_package_b64, used: false });
      return { id: nextId("kp"), expires_at: "2099-01-01T00:00:00Z" };
    }

    let m = path.match(/^\/v1\/mls\/key-package\/([^/]+)$/);
    if (method === "GET" && m) {
      const targetDid = decodeURIComponent(m[1]);
      const kp = keyPackages.get(targetDid);
      if (!kp || kp.used) throw new Error(`mock relay: KeyPackage bulunamadı (${targetDid})`);
      kp.used = true;
      return { key_package_b64: kp.wire, target_did: targetDid };
    }

    m = path.match(/^\/v1\/mls\/group\/([^/]+)\/add$/);
    if (method === "POST" && m) {
      const groupId = decodeURIComponent(m[1]);
      const q = welcomeQueue.get(body.new_member_did) ?? [];
      q.push({ id: nextId("w"), group_id: groupId, welcome_b64: body.welcome_b64, created_at: "2099-01-01T00:00:00Z" });
      welcomeQueue.set(body.new_member_did, q);
      return { group_id: groupId, new_epoch: body.new_epoch, welcomed: body.new_member_did, broadcast: 0 };
    }

    if (method === "GET" && path === "/v1/mls/welcomes") {
      return welcomeQueue.get(actor) ?? [];
    }

    m = path.match(/^\/v1\/mls\/group\/([^/]+)\/message$/);
    if (method === "POST" && m) {
      const groupId = decodeURIComponent(m[1]);
      const list = messages.get(groupId) ?? [];
      const id = nextId("m");
      list.push({ id, sender_did: actor, ciphertext_b64: body.ciphertext_b64, epoch: body.epoch, created_at: "2099-01-01T00:00:00Z" });
      messages.set(groupId, list);
      return { id, created_at: "2099-01-01T00:00:00Z", delivered: 0, queued: 0 };
    }

    m = path.match(/^\/v1\/mls\/group\/([^/]+)\/messages(?:\?since_epoch=(\d+))?$/);
    if (method === "GET" && m) {
      const groupId = decodeURIComponent(m[1]);
      const since = m[2] ? Number(m[2]) : 0;
      const list = (messages.get(groupId) ?? []).filter((msg) => msg.epoch >= since);
      return { group_id: groupId, messages: list };
    }

    throw new Error(`mock relay: eşleşmeyen istek ${method} ${path}`);
  };

  return { handle, setActor: (did: string) => { actor = did; } };
}

beforeEach(() => {
  jest.clearAllMocks();
});

describe("E1 uçtan-uca — 4a kripto + 4b client + mock relay", () => {
  test("Alice grup kurar, Bob'u ekler, Alice şifreler, Bob mock-relay üzerinden çözer", async () => {
    const relay = createMockRelay();
    mockedApiFetch.mockImplementation(relay.handle);

    const cs = await getMlsCiphersuiteImpl();
    const aliceDid = "did:obs:alice-4c";
    const bobDid = "did:obs:bob-4c";
    const plaintext = "L2 Tuğla 4c — uçtan-uca mock relay";

    // 1. Alice + Bob KeyPackage üretir, relay'e yükler
    const aliceOwn = await createOwnKeyPackage(aliceDid, cs);
    relay.setActor(aliceDid);
    await uploadKeyPackage(aliceOwn.keyPackageWireB64);

    const bobOwn = await createOwnKeyPackage(bobDid, cs);
    relay.setActor(bobDid);
    await uploadKeyPackage(bobOwn.keyPackageWireB64);

    // 2. Alice Bob'un KP'sini çeker
    relay.setActor(aliceDid);
    const bobKp = await getKeyPackage(bobDid);

    // 3. Alice grup kurar + Bob'u ekler (local crypto, 4a)
    const groupIdBytes = new TextEncoder().encode("obscura-tugla4c-group");
    const created = await createGroupWithMember(aliceOwn, groupIdBytes, bobKp.key_package_b64, cs);
    expect(created.newEpoch).toBe(1);

    // 4. Alice commit+welcome'ı relay'e yollar (backend group_id: base64 of openmls group_id)
    const groupIdB64 = Buffer.from(groupIdBytes).toString("base64");
    await addMember(groupIdB64, bobDid, created.commitWireB64, created.welcomeWireB64, created.newEpoch);

    // 5. Bob welcome'ı çeker
    relay.setActor(bobDid);
    const welcomes = await getWelcomes();
    expect(welcomes.length).toBe(1);
    expect(welcomes[0].group_id).toBe(groupIdB64);

    // 6. Bob welcome'ı işler, gruba katılır (local crypto, 4a)
    const bobState = await joinFromWelcomeWire(welcomes[0].welcome_b64, bobOwn.keyPackageWireB64, bobOwn.privateKeyPackage, cs);

    // 7. Alice mesajı şifreler (local crypto, 4a)
    const encrypted = await encryptGroupMessage(created.newState, plaintext, cs);

    // 8. Alice ciphertext'i relay'e yollar
    relay.setActor(aliceDid);
    await sendGroupMessage(groupIdB64, encrypted.ciphertextWireB64, encrypted.epoch);

    // 9. Bob mesajları çeker
    relay.setActor(bobDid);
    const fetched = await getGroupMessages(groupIdB64);
    expect(fetched.messages.length).toBe(1);

    // 10. Bob çözer (local crypto, 4a) — plaintext Alice'in gönderdiğiyle eşleşmeli
    const decrypted = await decryptApplicationMessageWire(bobState, fetched.messages[0].ciphertext_b64, cs);
    expect(decrypted).toBe(plaintext);
  });
});
