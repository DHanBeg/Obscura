// Kaba konum ızgarası — Bölüm 6.1: "Kaba konum: Buluşma/keşif özelliği
// yalnızca semt/1km-grid gösterir. Sokak/nokta konum asla."
//
// Bu modül location.tsx'ten (ZK konum kanıtı ekranı) çıkarıldı, panik butonu
// (Madde 13) da aynı fonksiyonu kullanır — tek kaynak, iki tüketici.

const GRID_DEG = 0.009; // ~1 km per degree

/** 1 km grid cell id (floor division). Used as an anonymous proximity label. */
export function gridCell(lat: number, lon: number): string {
  const latCell = Math.floor(lat / GRID_DEG);
  const lonCell = Math.floor(lon / GRID_DEG);
  return `${latCell}:${lonCell}`;
}
