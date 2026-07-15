import {
  SELF_DESTRUCT_OPTIONS,
  isSelfDestructMessage,
  formatSelfDestructLabel,
} from "../self-destruct";

describe("SELF_DESTRUCT_OPTIONS", () => {
  test("ADIM 1 semantiğiyle birebir eşleşir: kapalı/okununca/10sn/1dk/5dk/1saat", () => {
    expect(SELF_DESTRUCT_OPTIONS.map((o) => o.seconds)).toEqual([null, 0, 10, 60, 300, 3600]);
  });
});

describe("isSelfDestructMessage", () => {
  test("self_destruct_seconds tanımsızsa false döner (normal mesaj)", () => {
    expect(isSelfDestructMessage({})).toBe(false);
  });

  test("self_destruct_seconds null ise false döner", () => {
    expect(isSelfDestructMessage({ self_destruct_seconds: null })).toBe(false);
  });

  test("self_destruct_seconds 0 ise true döner (okununca)", () => {
    expect(isSelfDestructMessage({ self_destruct_seconds: 0 })).toBe(true);
  });

  test("self_destruct_seconds > 0 ise true döner", () => {
    expect(isSelfDestructMessage({ self_destruct_seconds: 60 })).toBe(true);
  });
});

describe("formatSelfDestructLabel", () => {
  test("null → Kapalı", () => {
    expect(formatSelfDestructLabel(null)).toBe("Kapalı");
  });
  test("undefined → Kapalı", () => {
    expect(formatSelfDestructLabel(undefined)).toBe("Kapalı");
  });
  test("0 → Okununca", () => {
    expect(formatSelfDestructLabel(0)).toBe("Okununca");
  });
  test("10 → 10 saniye", () => {
    expect(formatSelfDestructLabel(10)).toBe("10 saniye");
  });
  test("60 → 1 dakika", () => {
    expect(formatSelfDestructLabel(60)).toBe("1 dakika");
  });
  test("300 → 5 dakika", () => {
    expect(formatSelfDestructLabel(300)).toBe("5 dakika");
  });
  test("3600 → 1 saat", () => {
    expect(formatSelfDestructLabel(3600)).toBe("1 saat");
  });
});
