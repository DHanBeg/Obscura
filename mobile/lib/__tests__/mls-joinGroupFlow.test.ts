// L2 Tuğla 5c — acceptMlsWelcome: sıra + local-önce + kısmi-başarısızlık.
// 5b-2'nin mls-createGroupFlow.test.ts'iyle AYNI desen: tüm ağ/kripto
// bağımlılıkları mock, sadece koordinasyon mantığı test ediliyor.
jest.mock("../mls/group");
jest.mock("../mls/mls-store");
jest.mock("../mls/mlsApi");

import { acceptMlsWelcome } from "../mls/joinGroupFlow";
import * as group from "../mls/group";
import * as mlsStore from "../mls/mls-store";
import * as mlsApi from "../mls/mlsApi";
import type { PendingWelcome } from "../mls/mlsApi";

const FAKE_CS = { fake: "ciphersuite-impl" } as any;
const FAKE_OWN_KP = {
  keyPackageWireB64: "own-kp-wire",
  privateKeyPackage: { initPrivateKeyB64: "a", hpkePrivateKeyB64: "b", signaturePrivateKeyB64: "c" },
};
const FAKE_STATE = { groupContext: { epoch: 1n }, marker: "joined-state" } as any;

const WELCOME: PendingWelcome = {
  id: "welcome-1",
  group_id: "group-abc",
  welcome_b64: "welcome-wire-bytes",
  created_at: "2026-08-15T00:00:00Z",
};

let callOrder: string[];

beforeEach(() => {
  jest.clearAllMocks();
  callOrder = [];

  (group.getMlsCiphersuiteImpl as jest.Mock).mockResolvedValue(FAKE_CS);
  (mlsStore.loadOwnKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("loadOwnKeyPackage");
    return FAKE_OWN_KP;
  });
  (group.joinFromWelcomeWire as jest.Mock).mockImplementation(async () => {
    callOrder.push("joinFromWelcomeWire");
    return FAKE_STATE;
  });
  (mlsStore.saveGroupState as jest.Mock).mockImplementation(async () => {
    callOrder.push("saveGroupState");
  });
  (mlsApi.joinGroup as jest.Mock).mockImplementation(async (groupId: string, welcomeId: string | undefined, epoch: number) => {
    callOrder.push("joinGroup");
    return { group_id: groupId, role: "member", name: "G", ciphersuite: "cs", epoch, member_count: 2 };
  });
});

describe("acceptMlsWelcome — sıra ve local-önce", () => {
  test("doğru sıra: loadOwnKeyPackage → joinFromWelcomeWire → saveGroupState → joinGroup(ağ)", async () => {
    const result = await acceptMlsWelcome({ ownDid: "did:obs:bob", welcome: WELCOME });

    expect(result).toEqual({ groupId: "group-abc", epoch: 1 });
    expect(callOrder).toEqual(["loadOwnKeyPackage", "joinFromWelcomeWire", "saveGroupState", "joinGroup"]);

    // local-önce mühürü: saveGroupState, AĞ çağrısından (joinGroup) önce.
    expect(callOrder.indexOf("saveGroupState")).toBeLessThan(callOrder.indexOf("joinGroup"));
  });

  test("doğru argümanlar: welcome_b64 + kendi KeyPackage'ı join'e, group_id + welcome.id + epoch mlsApi.joinGroup'a", async () => {
    await acceptMlsWelcome({ ownDid: "did:obs:bob", welcome: WELCOME });

    expect(group.joinFromWelcomeWire).toHaveBeenCalledWith(
      WELCOME.welcome_b64,
      FAKE_OWN_KP.keyPackageWireB64,
      FAKE_OWN_KP.privateKeyPackage,
      FAKE_CS
    );
    expect(mlsStore.saveGroupState).toHaveBeenCalledWith(WELCOME.group_id, FAKE_STATE);
    expect(mlsApi.joinGroup).toHaveBeenCalledWith(WELCOME.group_id, WELCOME.id, 1);
  });
});

describe("acceptMlsWelcome — kendi KeyPackage'ı yoksa", () => {
  test("YENİ bir tane üretmez — hata fırlatır (Welcome, Alice'in fetch ettiği ESKİ private key ile eşleşmeli)", async () => {
    (mlsStore.loadOwnKeyPackage as jest.Mock).mockResolvedValue(null);

    await expect(acceptMlsWelcome({ ownDid: "did:obs:bob", welcome: WELCOME })).rejects.toThrow(/KeyPackage/);

    expect(group.createOwnKeyPackage).not.toHaveBeenCalled();
    expect(group.joinFromWelcomeWire).not.toHaveBeenCalled();
    expect(mlsStore.saveGroupState).not.toHaveBeenCalled();
  });
});

describe("acceptMlsWelcome — kısmi başarısızlık", () => {
  test("join başarılı ama mlsApi.joinGroup (ağ) başarısız: local state ZATEN kaydedilmiş (Bob fiilen grupta), retry edilebilir", async () => {
    (mlsApi.joinGroup as jest.Mock).mockImplementation(async () => {
      callOrder.push("joinGroup:FAIL");
      throw new Error("network drop");
    });

    await expect(acceptMlsWelcome({ ownDid: "did:obs:bob", welcome: WELCOME })).rejects.toThrow("network drop");

    expect(mlsStore.saveGroupState).toHaveBeenCalledTimes(1);
    expect(mlsStore.deleteGroupState).not.toHaveBeenCalled();
  });

  test("local adımlardan biri (joinFromWelcomeWire) başarısız: saveGroupState HİÇ çağrılmadı, ağa hiç çıkılmadı", async () => {
    (group.joinFromWelcomeWire as jest.Mock).mockImplementation(async () => {
      throw new Error("welcome wire bozuk");
    });

    await expect(acceptMlsWelcome({ ownDid: "did:obs:bob", welcome: WELCOME })).rejects.toThrow("welcome wire bozuk");

    expect(mlsStore.saveGroupState).not.toHaveBeenCalled();
    expect(mlsApi.joinGroup).not.toHaveBeenCalled();
  });
});
