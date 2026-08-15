// L2 Tuğla 5b-2 Parça B/C — createMlsGroupConversation: sıra + local-önce +
// kısmi-başarısızlık sözleşmesi. Tüm ağ/kripto bağımlılıkları mock —
// bu dosya SADECE koordinasyon mantığını (sıra, ne zaman ne çağrılır, hata
// durumunda ne çağrılMAZ) test eder, gerçek MLS/HTTP burada yok (onlar
// group.ts/mlsApi.ts/mls-store.ts'in kendi testlerinde).
jest.mock("../mls/group");
jest.mock("../mls/mls-store");
jest.mock("../mls/mlsApi");
jest.mock("../api");

import { createMlsGroupConversation } from "../mls/createGroupFlow";
import * as group from "../mls/group";
import * as mlsStore from "../mls/mls-store";
import * as mlsApi from "../mls/mlsApi";
import { api } from "../api";

const FAKE_CS = { fake: "ciphersuite-impl" } as any;
const FAKE_OWN_KP = { keyPackageWireB64: "own-kp-wire", privateKeyPackage: { initPrivateKeyB64: "a", hpkePrivateKeyB64: "b", signaturePrivateKeyB64: "c" } };

function makeState(epoch: number) {
  return { groupContext: { epoch: BigInt(epoch) }, marker: `state-epoch-${epoch}` } as any;
}

let callOrder: string[];

beforeEach(() => {
  jest.clearAllMocks();
  callOrder = [];

  (group.getMlsCiphersuiteImpl as jest.Mock).mockResolvedValue(FAKE_CS);
  (group.createOwnKeyPackage as jest.Mock).mockResolvedValue(FAKE_OWN_KP);
  (mlsStore.loadOwnKeyPackage as jest.Mock).mockResolvedValue(FAKE_OWN_KP); // varsayılan: zaten var
  (mlsStore.saveOwnKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("saveOwnKeyPackage");
  });
  (mlsStore.saveGroupState as jest.Mock).mockImplementation(async () => {
    callOrder.push("saveGroupState");
  });

  (group.createGroupWithMember as jest.Mock).mockImplementation(async (_ownKp: any, _groupId: any, memberKpWire: string) => {
    callOrder.push(`createGroupWithMember(${memberKpWire})`);
    return { newState: makeState(1), commitWireB64: `commit-${memberKpWire}`, welcomeWireB64: `welcome-${memberKpWire}`, newEpoch: 1 };
  });
  (group.addMemberToGroup as jest.Mock).mockImplementation(async (state: any, memberKpWire: string) => {
    const nextEpoch = Number(state.groupContext.epoch) + 1;
    callOrder.push(`addMemberToGroup(${memberKpWire})`);
    return { newState: makeState(nextEpoch), commitWireB64: `commit-${memberKpWire}`, welcomeWireB64: `welcome-${memberKpWire}`, newEpoch: nextEpoch };
  });

  (mlsApi.getKeyPackage as jest.Mock).mockImplementation(async (did: string) => {
    callOrder.push(`getKeyPackage(${did})`);
    return { key_package_b64: `kp-wire-${did}`, target_did: did };
  });
  (mlsApi.createGroup as jest.Mock).mockImplementation(async (groupId: string) => {
    callOrder.push(`createGroupOnServer(${groupId})`);
    return { group_id: groupId, creator_did: "did:obs:me", epoch: 0 };
  });
  (mlsApi.addMember as jest.Mock).mockImplementation(async (groupId: string, memberDid: string) => {
    callOrder.push(`addMemberOnServer(${memberDid})`);
    return { group_id: groupId, new_epoch: 1, welcomed: memberDid, broadcast: 0 };
  });
  (api.createConversation as jest.Mock).mockImplementation(async () => {
    callOrder.push("createConversation");
    return { conv_id: "conv-123" };
  });
});

describe("createMlsGroupConversation — sıra ve local-önce", () => {
  test("2 üyeli grup: doğru sıra, saveGroupState AĞDAN (createGroupOnServer) ÖNCE", async () => {
    const result = await createMlsGroupConversation({
      ownDid: "did:obs:me",
      name: "Test Grubu",
      memberDids: ["did:obs:bob", "did:obs:carol"],
    });

    expect(result.convId).toBe("conv-123");
    expect(callOrder).toEqual([
      "getKeyPackage(did:obs:bob)",
      "createGroupWithMember(kp-wire-did:obs:bob)",
      "getKeyPackage(did:obs:carol)",
      "addMemberToGroup(kp-wire-did:obs:carol)",
      "saveGroupState",
      "createGroupOnServer(" + result.groupId + ")",
      "addMemberOnServer(did:obs:bob)",
      "addMemberOnServer(did:obs:carol)",
      "createConversation",
    ]);

    // local-önce mühürü: saveGroupState indexi, HER ağ çağrısından önce.
    const saveIdx = callOrder.indexOf("saveGroupState");
    const firstNetworkIdx = callOrder.findIndex((c) => c.startsWith("createGroupOnServer") || c.startsWith("addMemberOnServer") || c === "createConversation");
    expect(saveIdx).toBeLessThan(firstNetworkIdx);
  });

  test("kendi KeyPackage'ı yoksa üretilir + saklanır (createGroupWithMember'dan ÖNCE)", async () => {
    (mlsStore.loadOwnKeyPackage as jest.Mock).mockResolvedValue(null);

    await createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] });

    expect(group.createOwnKeyPackage).toHaveBeenCalledWith("did:obs:me", FAKE_CS);
    expect(mlsStore.saveOwnKeyPackage).toHaveBeenCalledWith("did:obs:me", FAKE_OWN_KP);
    const saveOwnIdx = callOrder.indexOf("saveOwnKeyPackage");
    const createGroupIdx = callOrder.findIndex((c) => c.startsWith("createGroupWithMember"));
    expect(saveOwnIdx).toBeLessThan(createGroupIdx);
  });

  test("mls_group_id — üretilen groupId createGroup/addMember/createConversation'a AYNI şekilde gidiyor", async () => {
    const result = await createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] });

    expect(mlsApi.createGroup).toHaveBeenCalledWith(result.groupId, "G");
    expect(mlsApi.addMember).toHaveBeenCalledWith(result.groupId, "did:obs:bob", "commit-kp-wire-did:obs:bob", "welcome-kp-wire-did:obs:bob", 1);
    expect(api.createConversation).toHaveBeenCalledWith(
      expect.objectContaining({ type: "group", name: "G", members: ["did:obs:bob"], mls_group_id: result.groupId })
    );
  });
});

describe("createMlsGroupConversation — kısmi başarısızlık (Parça C)", () => {
  test("adım 6 (createGroupOnServer) başarısız: local state ZATEN kaydedilmiş, sonraki ağ adımları hiç çağrılmadı", async () => {
    (mlsApi.createGroup as jest.Mock).mockImplementation(async () => {
      callOrder.push("createGroupOnServer:FAIL");
      throw new Error("backend 500");
    });

    await expect(
      createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] })
    ).rejects.toThrow("backend 500");

    expect(mlsStore.saveGroupState).toHaveBeenCalledTimes(1); // local state YAZILDI, silinmedi
    expect(mlsApi.addMember).not.toHaveBeenCalled();
    expect(api.createConversation).not.toHaveBeenCalled();
    expect(mlsStore.deleteGroupState).not.toHaveBeenCalled();
  });

  test("adım 7 (addMember) başarısız: createGroupOnServer zaten çağrıldı, createConversation hiç çağrılmadı", async () => {
    (mlsApi.addMember as jest.Mock).mockImplementation(async () => {
      callOrder.push("addMemberOnServer:FAIL");
      throw new Error("network drop");
    });

    await expect(
      createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] })
    ).rejects.toThrow("network drop");

    expect(mlsStore.saveGroupState).toHaveBeenCalledTimes(1);
    expect(mlsApi.createGroup).toHaveBeenCalledTimes(1);
    expect(api.createConversation).not.toHaveBeenCalled();
    expect(mlsStore.deleteGroupState).not.toHaveBeenCalled();
  });

  test("adım 8 (createConversation) başarısız: 6/7 zaten tamamlandı, yine de local state SİLİNMEDİ (deleteGroupState çağrılabilir bir fonksiyon olarak mock'ta yok/çağrılmadı)", async () => {
    (api.createConversation as jest.Mock).mockImplementation(async () => {
      callOrder.push("createConversation:FAIL");
      throw new Error("conv insert failed");
    });

    await expect(
      createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] })
    ).rejects.toThrow("conv insert failed");

    expect(mlsStore.saveGroupState).toHaveBeenCalledTimes(1);
    expect(mlsApi.addMember).toHaveBeenCalledTimes(1);
    // Karar (Parça C): 6/7/8'den HİÇBİRİNİN başarısızlığında local state
    // silinmez — deleteGroupState var olan bir fonksiyon (5a'dan) ama bu
    // akış onu hiçbir dalda ÇAĞIRMAZ.
    expect(mlsStore.deleteGroupState).not.toHaveBeenCalled();
  });

  test("adım 1-4 (local, ağdan önce) başarısız: saveGroupState HİÇ çağrılmadı — temiz başarısızlık", async () => {
    (mlsApi.getKeyPackage as jest.Mock).mockImplementation(async () => {
      throw new Error("key package bulunamadı");
    });

    await expect(
      createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: ["did:obs:bob"] })
    ).rejects.toThrow("key package bulunamadı");

    expect(mlsStore.saveGroupState).not.toHaveBeenCalled();
    expect(mlsApi.createGroup).not.toHaveBeenCalled();
    expect(api.createConversation).not.toHaveBeenCalled();
  });

  test("üye listesi boş: hemen hata, hiçbir yan etki yok", async () => {
    await expect(
      createMlsGroupConversation({ ownDid: "did:obs:me", name: "G", memberDids: [] })
    ).rejects.toThrow(/en az 1 üye/);

    expect(mlsApi.getKeyPackage).not.toHaveBeenCalled();
    expect(mlsStore.saveGroupState).not.toHaveBeenCalled();
  });
});
