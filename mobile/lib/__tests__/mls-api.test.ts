// L2 Tuğla 4b — mlsApi.ts: 8 endpoint HTTP wrapper. Saf transport, ts-mls'e
// dokunmaz. Her fonksiyonun DOĞRU path + method + body kurduğunu ve
// apiFetch'in döndürdüğü (envelope zaten soyulmuş) veriyi olduğu gibi
// ilettiğini doğruluyor. Gerçek backend'e bağlanma — o Tuğla 4c.
jest.mock("../api", () => ({
  apiFetch: jest.fn(),
}));

import { apiFetch } from "../api";
import {
  uploadKeyPackage,
  getKeyPackage,
  createGroup,
  addMember,
  getWelcomes,
  joinGroup,
  sendGroupMessage,
  getGroupMessages,
} from "../mls/mlsApi";

const mockedApiFetch = apiFetch as unknown as jest.Mock;

beforeEach(() => {
  jest.clearAllMocks();
});

describe("mlsApi.ts — 8 endpoint HTTP wrapper", () => {
  test("uploadKeyPackage → POST /v1/mls/key-package, ttl_days opsiyonel", async () => {
    mockedApiFetch.mockResolvedValue({ id: "kp-1", expires_at: "2026-11-14T00:00:00Z" });

    const result = await uploadKeyPackage("a2V5cGFja2FnZQ==", 30);

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/key-package", {
      method: "POST",
      body: JSON.stringify({ key_package_b64: "a2V5cGFja2FnZQ==", ttl_days: 30 }),
    });
    expect(result).toEqual({ id: "kp-1", expires_at: "2026-11-14T00:00:00Z" });
  });

  test("uploadKeyPackage → ttlDays verilmezse body'de ttl_days alanı yok", async () => {
    mockedApiFetch.mockResolvedValue({ id: "kp-2", expires_at: "2026-11-14T00:00:00Z" });

    await uploadKeyPackage("a2V5cGFja2FnZQ==");

    const body = JSON.parse(mockedApiFetch.mock.calls[0][1].body);
    expect(body).toEqual({ key_package_b64: "a2V5cGFja2FnZQ==" });
  });

  test("getKeyPackage → GET /v1/mls/key-package/{did}", async () => {
    mockedApiFetch.mockResolvedValue({ key_package_b64: "d2lyZQ==", target_did: "did:obs:bob" });

    const result = await getKeyPackage("did:obs:bob");

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/key-package/did%3Aobs%3Abob");
    expect(result).toEqual({ key_package_b64: "d2lyZQ==", target_did: "did:obs:bob" });
  });

  test("createGroup → POST /v1/mls/group, name/ciphersuite opsiyonel", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", creator_did: "did:obs:alice", epoch: 0 });

    const result = await createGroup("g1", "Ekip", "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519");

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group", {
      method: "POST",
      body: JSON.stringify({
        group_id: "g1",
        name: "Ekip",
        ciphersuite: "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
      }),
    });
    expect(result).toEqual({ group_id: "g1", creator_did: "did:obs:alice", epoch: 0 });
  });

  test("createGroup → name/ciphersuite verilmezse body'de yok", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", creator_did: "did:obs:alice", epoch: 0 });

    await createGroup("g1");

    const body = JSON.parse(mockedApiFetch.mock.calls[0][1].body);
    expect(body).toEqual({ group_id: "g1" });
  });

  test("addMember → POST /v1/mls/group/{id}/add", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", new_epoch: 1, welcomed: "did:obs:bob", broadcast: 0 });

    const result = await addMember("g1", "did:obs:bob", "Y29tbWl0", "d2VsY29tZQ==", 1);

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group/g1/add", {
      method: "POST",
      body: JSON.stringify({
        new_member_did: "did:obs:bob",
        commit_b64: "Y29tbWl0",
        welcome_b64: "d2VsY29tZQ==",
        new_epoch: 1,
      }),
    });
    expect(result).toEqual({ group_id: "g1", new_epoch: 1, welcomed: "did:obs:bob", broadcast: 0 });
  });

  test("getWelcomes → GET /v1/mls/welcomes", async () => {
    const welcomes = [{ id: "w1", group_id: "g1", welcome_b64: "d2VsY29tZQ==", created_at: "2026-08-14T00:00:00Z" }];
    mockedApiFetch.mockResolvedValue(welcomes);

    const result = await getWelcomes();

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/welcomes");
    expect(result).toEqual(welcomes);
  });

  test("joinGroup → POST /v1/mls/group/{id}/join, welcomeId opsiyonel", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", role: "member", name: "Ekip", ciphersuite: "cs", epoch: 1, member_count: 2 });

    const result = await joinGroup("g1", "w1", 1);

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group/g1/join", {
      method: "POST",
      body: JSON.stringify({ welcome_id: "w1", epoch: 1 }),
    });
    expect(result).toEqual({ group_id: "g1", role: "member", name: "Ekip", ciphersuite: "cs", epoch: 1, member_count: 2 });
  });

  test("joinGroup → welcomeId verilmezse body'de welcome_id alanı yok", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", role: "member", name: "Ekip", ciphersuite: "cs", epoch: 1, member_count: 2 });

    await joinGroup("g1", undefined, 1);

    const body = JSON.parse(mockedApiFetch.mock.calls[0][1].body);
    expect(body).toEqual({ epoch: 1 });
  });

  test("sendGroupMessage → POST /v1/mls/group/{id}/message", async () => {
    mockedApiFetch.mockResolvedValue({ id: "m1", created_at: "2026-08-14T00:00:00Z", delivered: 1, queued: 0 });

    const result = await sendGroupMessage("g1", "Y2lwaGVy", 1);

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group/g1/message", {
      method: "POST",
      body: JSON.stringify({ ciphertext_b64: "Y2lwaGVy", epoch: 1 }),
    });
    expect(result).toEqual({ id: "m1", created_at: "2026-08-14T00:00:00Z", delivered: 1, queued: 0 });
  });

  test("getGroupMessages → GET /v1/mls/group/{id}/messages, sinceEpoch opsiyonel", async () => {
    const payload = { group_id: "g1", messages: [] };
    mockedApiFetch.mockResolvedValue(payload);

    const result = await getGroupMessages("g1", 5);

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group/g1/messages?since_epoch=5");
    expect(result).toEqual(payload);
  });

  test("getGroupMessages → sinceEpoch verilmezse query string yok", async () => {
    mockedApiFetch.mockResolvedValue({ group_id: "g1", messages: [] });

    await getGroupMessages("g1");

    expect(mockedApiFetch).toHaveBeenCalledWith("/v1/mls/group/g1/messages");
  });
});
