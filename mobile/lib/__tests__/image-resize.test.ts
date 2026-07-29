import { resizeActionFor, MAX_PHOTO_EDGE } from "../image-resize";

describe("resizeActionFor — chat foto gönderiminde piksel/boyut sınırlama", () => {
  test("uzun kenar sınırın altındaysa resize gerekmez (null)", () => {
    expect(resizeActionFor(800, 600)).toBeNull();
  });

  test("uzun kenar TAM sınırdaysa resize gerekmez (null, sınır dahil)", () => {
    expect(resizeActionFor(MAX_PHOTO_EDGE, 900)).toBeNull();
  });

  test("landscape (genişlik uzun kenar) — width kısıtlanır, height otomatik", () => {
    expect(resizeActionFor(4000, 3000)).toEqual({ resize: { width: MAX_PHOTO_EDGE } });
  });

  test("portrait (yükseklik uzun kenar) — height kısıtlanır, width otomatik", () => {
    expect(resizeActionFor(3000, 4000)).toEqual({ resize: { height: MAX_PHOTO_EDGE } });
  });

  test("kare (width===height) — width kısıtlanır (genişlik >= yükseklik dalı)", () => {
    expect(resizeActionFor(4000, 4000)).toEqual({ resize: { width: MAX_PHOTO_EDGE } });
  });

  test("özel maxEdge parametresi kullanılabilir", () => {
    expect(resizeActionFor(2000, 1000, 1600)).toEqual({ resize: { width: 1600 } });
    expect(resizeActionFor(1000, 500, 1600)).toBeNull();
  });
});
