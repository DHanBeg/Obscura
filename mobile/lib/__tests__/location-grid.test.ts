import { gridCell } from "../location-grid";

describe("gridCell — 1km kaba konum ızgarası (Madde 13, Bölüm 6.1)", () => {
  test("location.tsx'teki eski davranışla birebir eşleşir: floor(lat/0.009):floor(lon/0.009)", () => {
    // Bu değerler refactor ÖNCESİ location.tsx:50-55'teki mantıkla elle
    // hesaplandı — davranış birebir korunmalı, sadece taşındı.
    expect(gridCell(41.015137, 29.023056)).toBe("4557:3224");
  });

  test("negatif enlem/boylam da doğru ızgaraya düşer (floor, truncate değil)", () => {
    expect(gridCell(-33.868_820, 151.209_296)).toBe(
      `${Math.floor(-33.86882 / 0.009)}:${Math.floor(151.209296 / 0.009)}`
    );
  });

  test("aynı ~1km hücredeki iki farklı nokta aynı grid_id'yi üretir (anonimleştirme çalışıyor)", () => {
    const a = gridCell(41.0151, 29.0231);
    const b = gridCell(41.0153, 29.0233);
    expect(a).toBe(b);
  });

  test("1km'den uzak iki nokta farklı grid_id üretir", () => {
    const a = gridCell(41.0151, 29.0231);
    const b = gridCell(41.05, 29.0231);
    expect(a).not.toBe(b);
  });

  test("sıfır koordinat (0,0) çökmeden çalışır", () => {
    expect(gridCell(0, 0)).toBe("0:0");
  });
});
