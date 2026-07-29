// Sohbet fotoğrafları hiç resize edilmeden base64 inline gönderiliyordu
// (E2EE gerekçesiyle upload+URL değil, ciphertext'e gömülü) — modern telefon
// kamerası 4000×3000+ üretir, bu ham haliyle şifrelenip mesaj olarak
// gönderilince gönderim yavaşlıyor, veri kullanımı artıyor, düşük uçlu
// cihazlarda decode OOM riski oluşuyordu. 1280px pratik chat-uygulaması
// standardı (WhatsApp/Telegram/Signal'in "normal" foto gönderiminde
// kullandığı yaygın üst sınırla aynı mertebede).
export const MAX_PHOTO_EDGE = 1280;

// resizeActionFor — expo-image-manipulator'ın `resize` action'ı için
// hazır bir eylem üretir (uzun kenar zaten sınırın altında/eşitse null,
// resize gereksiz). SADECE kısıtlayıcı kenarı (width VEYA height) set
// eder — kütüphane diğer kenarı en-boy oranını koruyarak kendi hesaplar,
// burada ekstra yuvarlama hatası riski taşınmaz.
export function resizeActionFor(
  width: number,
  height: number,
  maxEdge: number = MAX_PHOTO_EDGE
): { resize: { width?: number; height?: number } } | null {
  const longEdge = Math.max(width, height);
  if (longEdge <= maxEdge) return null;
  return width >= height ? { resize: { width: maxEdge } } : { resize: { height: maxEdge } };
}
