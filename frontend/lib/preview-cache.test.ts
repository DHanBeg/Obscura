import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  makePreviewText,
  writePreview,
  readPreviews,
  selectPreview,
  PREVIEW_MAX_CHARS,
  type PreviewEntry,
} from "./preview-cache";

const ACCT = "did:test:alice";

describe("preview-cache: makePreviewText", () => {
  it("kısa metni olduğu gibi bırakır, boşlukları tekleştirir", () => {
    expect(makePreviewText("  merhaba \n\n  dünya  ")).toBe("merhaba dünya");
  });

  it(`${PREVIEW_MAX_CHARS} karakterden uzun metni keser ve … ekler`, () => {
    const out = makePreviewText("a".repeat(200));
    expect(Array.from(out)).toHaveLength(PREVIEW_MAX_CHARS + 1);
    expect(out.endsWith("…")).toBe(true);
  });

  it("tam sınırdaki metni kesmez", () => {
    const exact = "b".repeat(PREVIEW_MAX_CHARS);
    expect(makePreviewText(exact)).toBe(exact);
  });

  it("emoji (surrogate pair) ortadan kesilmez", () => {
    const out = makePreviewText("😀".repeat(80));
    expect(Array.from(out).slice(0, PREVIEW_MAX_CHARS).every((c) => c === "😀")).toBe(true);
    expect(out).not.toMatch(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])/); // yetim yüksek surrogate yok
  });
});

describe("preview-cache: writePreview / readPreviews", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("yazılan önizleme okunur; sohbet başına tek (son) kayıt", () => {
    writePreview(ACCT, "c1", { text: "ilk", msgId: "m1" });
    writePreview(ACCT, "c1", { text: "ikinci", msgId: "m2" });
    const map = readPreviews(ACCT);
    expect(Object.keys(map)).toEqual(["c1"]);
    expect(map.c1.text).toBe("ikinci");
    expect(map.c1.msgId).toBe("m2");
  });

  it("uzun metin kesilerek saklanır (düz metin tam saklanmaz)", () => {
    writePreview(ACCT, "c1", { text: "x".repeat(500), msgId: "m1" });
    expect(Array.from(readPreviews(ACCT).c1.text).length).toBeLessThanOrEqual(PREVIEW_MAX_CHARS + 1);
  });

  it("hesap-bazlı: bir hesabın önizlemesini diğeri görmez", () => {
    writePreview(ACCT, "c1", { text: "alice'in" });
    expect(readPreviews("did:test:bob")).toEqual({});
  });

  it("decrypt yer tutucusu ve boş metin önbelleğe YAZILMAZ", () => {
    writePreview(ACCT, "c1", { text: "\u{1F512} Şifreli mesaj (cozulemedi)" });
    writePreview(ACCT, "c2", { text: "   \n  " });
    expect(readPreviews(ACCT)).toEqual({});
  });

  it("hesap DID'i ya da convId yoksa sessizce hiçbir şey yazmaz", () => {
    writePreview(undefined, "c1", { text: "x" });
    writePreview(ACCT, "", { text: "x" });
    expect(readPreviews(ACCT)).toEqual({});
  });

  it("grup önizlemesi gönderen etiketini saklar", () => {
    writePreview(ACCT, "g1", { text: "selam", msgId: "m9", from: "Sen" });
    expect(readPreviews(ACCT).g1.from).toBe("Sen");
  });

  it("bozuk JSON okunurken çökmez, boş harita döner", () => {
    localStorage.setItem(`obscura_preview_v1:${ACCT}`, "{bozuk json");
    expect(readPreviews(ACCT)).toEqual({});
    writePreview(ACCT, "c1", { text: "kurtarıldı" }); // bozuk kaydın üstüne yazabilir
    expect(readPreviews(ACCT).c1.text).toBe("kurtarıldı");
  });

  it("dizi/ilkel değer saklanmışsa boş harita döner", () => {
    localStorage.setItem(`obscura_preview_v1:${ACCT}`, "[1,2,3]");
    expect(readPreviews(ACCT)).toEqual({});
  });

  it("localStorage yazma hatası fırlatmaz (kota dolu)", () => {
    vi.spyOn(localStorage, "setItem").mockImplementation(() => {
      throw new Error("kota doldu");
    });
    expect(() => writePreview(ACCT, "c1", { text: "x" })).not.toThrow();
  });
});

describe("preview-cache: selectPreview (güncellik doğrulaması)", () => {
  const LOADED_AT = 1_000_000;
  const entry = (over: Partial<PreviewEntry> = {}): PreviewEntry => ({
    text: "selam",
    msgId: "m1",
    at: LOADED_AT - 5_000,
    ...over,
  });

  it("kayıt yoksa null (liste '🔒 Şifreli' kalır)", () => {
    expect(selectPreview(undefined, { last_msg_id: "m1" }, LOADED_AT)).toBeNull();
  });

  it("sunucunun son mesaj id'siyle eşleşirse gösterir", () => {
    expect(selectPreview(entry(), { last_msg_id: "m1" }, LOADED_AT)).toBe("selam");
  });

  it("id eşleşmiyor ve kayıt liste yüklenmeden ÖNCE yazıldıysa null (daha yeni, görmediğimiz mesaj var)", () => {
    expect(selectPreview(entry(), { last_msg_id: "m2" }, LOADED_AT)).toBeNull();
  });

  it("id eşleşmiyor ama kayıt liste yüklendikten SONRA yazıldıysa gösterir (canlı mesaj)", () => {
    expect(selectPreview(entry({ msgId: "m3", at: LOADED_AT + 10 }), { last_msg_id: "m2" }, LOADED_AT)).toBe("selam");
  });

  it("sunucu last_msg_id vermiyorsa (grup) kaydı gösterir", () => {
    expect(selectPreview(entry({ msgId: "" }), {}, LOADED_AT)).toBe("selam");
  });

  it("grup önizlemesine gönderen adı önek olur, 1:1'de yalnız metin", () => {
    expect(selectPreview(entry({ from: "Sen" }), {}, LOADED_AT)).toBe("Sen: selam");
    expect(selectPreview(entry(), { last_msg_id: "m1" }, LOADED_AT)).toBe("selam");
  });

  it("bozuk kayıt (metin yok/tip yanlış) çökertmez, null döner", () => {
    expect(selectPreview({ text: 5 } as unknown as PreviewEntry, {}, LOADED_AT)).toBeNull();
    expect(selectPreview({ text: "", msgId: "m1", at: 1 }, {}, LOADED_AT)).toBeNull();
  });
});
