import {
  triggerPanicAlert,
  PanicSendDeps,
  triggerImSafeAlert,
  ImSafeSendDeps,
  canShowImSafeButton,
  PanicSendOutcome,
} from "../panic-flow";

function makeDeps(overrides: Partial<PanicSendDeps> = {}): PanicSendDeps {
  return {
    requestPermission: jest.fn().mockResolvedValue("granted"),
    getCurrentPosition: jest.fn().mockResolvedValue({ latitude: 41.0, longitude: 29.0 }),
    send: jest.fn().mockResolvedValue({ id: "m1", conv_id: "c1" }),
    ...overrides,
  };
}

describe("triggerPanicAlert — Madde 13 Adım 5 orkestrasyon", () => {
  test("izin reddedilirse (yeniden sorulabilir) permission_denied döner, konum/gönderim hiç çağrılmaz", async () => {
    const deps = makeDeps({ requestPermission: jest.fn().mockResolvedValue("denied") });

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "permission_denied" });
    expect(deps.getCurrentPosition).not.toHaveBeenCalled();
    expect(deps.send).not.toHaveBeenCalled();
  });

  test("izin kalıcı engellenmişse (canAskAgain=false) permission_blocked döner, sessizce patlamaz", async () => {
    const deps = makeDeps({ requestPermission: jest.fn().mockResolvedValue("blocked") });

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "permission_blocked" });
    expect(deps.getCurrentPosition).not.toHaveBeenCalled();
    expect(deps.send).not.toHaveBeenCalled();
  });

  test("izin verilirse konum alınır, seçilen güven kişisine gönderilir, sent döner", async () => {
    const deps = makeDeps();

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(deps.getCurrentPosition).toHaveBeenCalledTimes(1);
    expect(deps.send).toHaveBeenCalledWith("did:obscura:trusted", { latitude: 41.0, longitude: 29.0 });
    expect(outcome).toEqual({ status: "sent", id: "m1", conv_id: "c1" });
  });

  test("konum alma hata verirse (ör. GPS kapalı) error döner, gönderim hiç çağrılmaz, çökmez", async () => {
    const deps = makeDeps({
      getCurrentPosition: jest.fn().mockRejectedValue(new Error("Konum servisleri kapalı")),
    });

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "error", message: "Konum servisleri kapalı" });
    expect(deps.send).not.toHaveBeenCalled();
  });

  test("gönderim hata verirse (ör. ağ hatası) error döner, çökmez", async () => {
    const deps = makeDeps({
      send: jest.fn().mockRejectedValue(new Error("Ağ hatası")),
    });

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "error", message: "Ağ hatası" });
  });

  test("beklenmeyen (message'sız) hata bile çökmeden 'Bilinmeyen hata' ile döner", async () => {
    const deps = makeDeps({
      getCurrentPosition: jest.fn().mockRejectedValue({}),
    });

    const outcome = await triggerPanicAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "error", message: "Bilinmeyen hata" });
  });
});

describe("triggerImSafeAlert — Madde 13 Adım 7 orkestrasyon", () => {
  test("izin/konum adımı YOK — doğrudan gönderilir, sent döner", async () => {
    const send = jest.fn().mockResolvedValue({ id: "m2", conv_id: "c1" });
    const deps: ImSafeSendDeps = { send };

    const outcome = await triggerImSafeAlert("did:obscura:trusted", deps);

    expect(send).toHaveBeenCalledWith("did:obscura:trusted");
    expect(outcome).toEqual({ status: "sent", id: "m2", conv_id: "c1" });
  });

  test("gönderim hata verirse error döner, çökmez", async () => {
    const send = jest.fn().mockRejectedValue(new Error("Ağ hatası"));
    const deps: ImSafeSendDeps = { send };

    const outcome = await triggerImSafeAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "error", message: "Ağ hatası" });
  });

  test("beklenmeyen (message'sız) hata bile çökmeden 'Bilinmeyen hata' ile döner", async () => {
    const send = jest.fn().mockRejectedValue({});
    const deps: ImSafeSendDeps = { send };

    const outcome = await triggerImSafeAlert("did:obscura:trusted", deps);

    expect(outcome).toEqual({ status: "error", message: "Bilinmeyen hata" });
  });
});

describe("canShowImSafeButton — panik-sonrası akış zemini (panik → sonra iyiyim)", () => {
  test("henüz panik gönderilmemişse (null) İyiyim butonu gösterilmez", () => {
    expect(canShowImSafeButton(null)).toBe(false);
  });

  test("panik başarıyla gönderildiyse (sent) İyiyim butonu gösterilir", () => {
    const outcome: PanicSendOutcome = { status: "sent", id: "m1", conv_id: "c1" };
    expect(canShowImSafeButton(outcome)).toBe(true);
  });

  test("izin reddi/blok/hata durumlarında İyiyim butonu gösterilmez", () => {
    expect(canShowImSafeButton({ status: "permission_denied" })).toBe(false);
    expect(canShowImSafeButton({ status: "permission_blocked" })).toBe(false);
    expect(canShowImSafeButton({ status: "error", message: "x" })).toBe(false);
  });
});
