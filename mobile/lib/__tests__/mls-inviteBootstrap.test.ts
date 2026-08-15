// L2 Tuğla 5c bölüm 2 — ensureInvitable: Bob'un davet-EDİLEBİLİR olması
// (kendi KeyPackage'ını backend'e yükleme). 5b-2/5c'nin diğer flow
// testleriyle AYNI desen — mock, sadece koordinasyon mantığı.
jest.mock("../mls/group");
jest.mock("../mls/mls-store");
jest.mock("../mls/mlsApi");

import { ensureInvitable } from "../mls/inviteBootstrap";
import * as group from "../mls/group";
import * as mlsStore from "../mls/mls-store";
import * as mlsApi from "../mls/mlsApi";

const FAKE_CS = { fake: "ciphersuite-impl" } as any;
const FAKE_OWN_KP = {
  keyPackageWireB64: "own-kp-wire",
  privateKeyPackage: { initPrivateKeyB64: "a", hpkePrivateKeyB64: "b", signaturePrivateKeyB64: "c" },
};

let callOrder: string[];

beforeEach(() => {
  jest.clearAllMocks();
  callOrder = [];

  (group.getMlsCiphersuiteImpl as jest.Mock).mockResolvedValue(FAKE_CS);
  (group.createOwnKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("createOwnKeyPackage");
    return FAKE_OWN_KP;
  });
  (mlsStore.loadOwnKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("loadOwnKeyPackage");
    return null; // varsayılan: yok
  });
  (mlsStore.saveOwnKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("saveOwnKeyPackage");
  });
  (mlsStore.hasUploadedKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("hasUploadedKeyPackage");
    return false; // varsayılan: yüklenmemiş
  });
  (mlsStore.markKeyPackageUploaded as jest.Mock).mockImplementation(async () => {
    callOrder.push("markKeyPackageUploaded");
  });
  (mlsApi.uploadKeyPackage as jest.Mock).mockImplementation(async () => {
    callOrder.push("uploadKeyPackage");
    return { id: "kp-id", expires_at: "2099-01-01T00:00:00Z" };
  });
});

describe("ensureInvitable — kendi KeyPackage'ı hiç yoksa", () => {
  test("üretir + lokale kaydeder + backend'e yükler + bayrağı işaretler (doğru sıra)", async () => {
    await ensureInvitable("did:obs:bob");

    expect(callOrder).toEqual([
      "loadOwnKeyPackage",
      "createOwnKeyPackage",
      "saveOwnKeyPackage",
      "uploadKeyPackage",
      "markKeyPackageUploaded",
    ]);
    expect(mlsApi.uploadKeyPackage).toHaveBeenCalledWith(FAKE_OWN_KP.keyPackageWireB64);
  });
});

describe("ensureInvitable — kendi KeyPackage'ı var", () => {
  test("zaten yüklenmişse (bayrak true): upload ATLANIR", async () => {
    (mlsStore.loadOwnKeyPackage as jest.Mock).mockResolvedValue(FAKE_OWN_KP);
    (mlsStore.hasUploadedKeyPackage as jest.Mock).mockResolvedValue(true);

    await ensureInvitable("did:obs:bob");

    expect(group.createOwnKeyPackage).not.toHaveBeenCalled();
    expect(mlsApi.uploadKeyPackage).not.toHaveBeenCalled();
    expect(mlsStore.markKeyPackageUploaded).not.toHaveBeenCalled();
  });

  test("KRİTİK: lokalde var ama HİÇ YÜKLENMEMİŞSE (bayrak false) — yine de yükler", async () => {
    // Bu tam olarak 5b-2'nin createGroupFlow.ts'inin bıraktığı durum: kurucu
    // kendi KeyPackage'ını üretip SADECE lokale kaydediyor, backend'e hiç
    // göndermiyor. ensureInvitable bunu YAKALAMALI, "zaten var" diye atlamamalı.
    (mlsStore.loadOwnKeyPackage as jest.Mock).mockResolvedValue(FAKE_OWN_KP);
    (mlsStore.hasUploadedKeyPackage as jest.Mock).mockResolvedValue(false);

    await ensureInvitable("did:obs:bob");

    expect(group.createOwnKeyPackage).not.toHaveBeenCalled(); // var olanı YENİDEN üretmez
    expect(mlsApi.uploadKeyPackage).toHaveBeenCalledWith(FAKE_OWN_KP.keyPackageWireB64);
    expect(mlsStore.markKeyPackageUploaded).toHaveBeenCalledWith("did:obs:bob");
  });
});

describe("ensureInvitable — upload başarısız", () => {
  test("hata yükselir, bayrak İŞARETLENMEZ (bir sonraki çağrı tekrar dener)", async () => {
    (mlsApi.uploadKeyPackage as jest.Mock).mockImplementation(async () => {
      throw new Error("backend 500");
    });

    await expect(ensureInvitable("did:obs:bob")).rejects.toThrow("backend 500");

    expect(mlsStore.markKeyPackageUploaded).not.toHaveBeenCalled();
  });
});
