// chat/[id].tsx'in 21 çağrı noktası (fetch guard + 6 gönderim çağrısı + 7
// gönderim guard'ı) `conv?.peer_did`'i her zaman "gönderim hedefi" olarak
// varsayıyordu — grup konuşmalarında peer_did asla dolmuyor (backend
// handlers.go:533, is_group=0'la sınırlı JOIN, kasıtlı), bu yüzden grup
// ekranı geçmişini bile ÇEKMİYORDU (sessiz no-op). Bu tek saf fonksiyon o
// 21 noktanın hedef-çözme kararını merkezileştirir.
//
// Grup gönderimi burada bir FLAG'le engellenmiyor (bkz. ADR-0019, Ders-4:
// flag ikinci/bypass-edilebilir bir savunma yaratır — istenmiyor). Güvence
// flag'den gelmiyor, chat/[id].tsx'te grup için plaintext gönderebilen bir
// kod yolunun HİÇ YAZILMAMASINDAN geliyor — L2 gerçek MLS-encrypt çağrısını
// yazana kadar grup gönderim call site'ları bu fonksiyonu SADECE geçmiş
// okumak/görüntülemek için kullanır, gönderim için değil.
export interface SendGateConversation {
  id: string;
  is_group: boolean | undefined;
  peer_did?: string;
}

// classifyConv — is_group'un TEK yorumlandığı yer. resolveSendTarget ve
// isGroupSendBlocked kendi is_group kontrolü YAPMAZ, ikisi de bu sonuçtan
// türer — iki bağımsız is_group yorumu olursa (biri throw eşiği farklı
// tutarsa) sessizce ayrışabilirler, tek primitif bunu yapısal olarak engeller.
export type ConvKind = "group" | "direct" | "unknown";

export function classifyConv(conv: SendGateConversation | undefined): ConvKind {
  if (!conv) return "unknown";
  if (conv.is_group === true) return "group";
  if (conv.is_group === false) return "direct";
  return "unknown";
}

// resolveSendTarget — grup konuşmasında hedef conv.id (backend
// findOrCreateConversation, handlers.go:1276-1277, grup için to_id=conv_id
// bekliyor); 1:1'de hedef peer_did (X25519 sealed-sender tek-alıcı DH'ı).
// is_group belirsizse (undefined/null) 1:1 mi grup mü TAHMİN ETMEZ, hata
// fırlatır — fail-closed: belirsiz durumda sessizce bir hedefe düşüp onu
// "güvenli" sanmak, tam da düz-metin sızıntısının doğduğu türden bir
// varsayım olurdu.
export function resolveSendTarget(conv: SendGateConversation | undefined): string | null {
  // conv henüz store'da yok (ekran daha yüklüyor) — bu is_group yorumu
  // DEĞİL, ayrı bir "veri yok" durumu, classifyConv'a hiç sorulmadan null.
  if (!conv) return null;
  const kind = classifyConv(conv);
  if (kind === "group") return conv.id;
  if (kind === "direct") return conv.peer_did || null;
  throw new Error(
    `resolveSendTarget: is_group belirsiz (${String(conv.is_group)}) — 1:1/grup ayrımı yapılamadan hedef çözülemez`
  );
}

// isGroupSendBlocked — chat/[id].tsx'in 6 gönderim akışı için tek gate.
// classifyConv'dan türer: SADECE 'direct' gönderime izin verir — 'group' VE
// 'unknown' ikisi de bloklanır (fail-closed: belirsiz durumda göndermeye
// izin vermek güvenlik açığı, resolveSendTarget'ın throw'uyla aynı ilke,
// burada throw yerine boolean çünkü UI thread'i çökertmemesi gerekiyor).
export function isGroupSendBlocked(conv: SendGateConversation | undefined): boolean {
  return classifyConv(conv) !== "direct";
}

// shouldFetchConversationHistory — chat/[id].tsx:139 fetch guard'ı. Mesaj
// geçmişi GET /v1/conversations/{convId}/messages convId ile çekiliyor
// (backend handlers.go:592-599, sadece conv_members üyeliğine bakıyor,
// peer_did'e ihtiyacı yok) — bu yüzden fetch'in koşulu peer_did DEĞİL,
// sadece "convId var mı ve conv store'da yüklendi mi" olmalı.
export function shouldFetchConversationHistory(
  convId: string | undefined,
  conv: SendGateConversation | undefined
): boolean {
  return !!convId && !!conv;
}
