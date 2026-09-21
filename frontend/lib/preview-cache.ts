// Sohbet listesi mesaj önizlemesi — YEREL önbellek.
//
// Sunucu kör-depolama: conversations.last_msg_text sabit "🔒 Şifreli" (içerik
// yok). Listede ratchet'ten decrypt YASAK (OPK tüketir, session state'ini
// ilerletir, decrypt kuyruğuyla yarışır). Bu yüzden düz metni GÖRDÜĞÜMÜZ ANDA
// (gönderirken, WS'ten gelip çözülünce, sohbet açılıp çözülünce) burada tutar,
// liste SENKRON okur: ağ yok, ratchet yok, unread_count'a dokunulmaz.
//
// Tehdit modeli mevcut gönderilen/alınan düz metin önbelleğiyle (e2ee-session.ts
// sent/recv cache) AYNI: yerel localStorage, ek şifreleme yok, sunucu görmez.
// Sohbet başına TEK kayıt (son mesaj), en fazla PREVIEW_MAX_CHARS karakter.
//
// Hesap-bazlı anahtar: aynı tarayıcıda iki hesap birbirinin önizlemesini görmez.
// Tüm localStorage erişimi try/catch içinde — hata halinde önizleme yok sayılır
// (liste "🔒 Şifreli" kalır), asla fırlatmaz.

const KEY_PREFIX = "obscura_preview_v1:";

export const PREVIEW_MAX_CHARS = 50;
export const PREVIEW_UPDATED_EVENT = "obscura:preview-updated";

export interface PreviewEntry {
  text: string; // PREVIEW_MAX_CHARS'a kesilmiş düz metin
  msgId: string; // sunucudaki mesaj id'si ("" = bilinmiyor); last_msg_id ile eşleştirilir
  at: number; // yazıldığı an (istemci saati, ms)
  from?: string; // grup önizlemesinde gönderen etiketi
}

export type PreviewMap = Record<string, PreviewEntry>;

export interface PreviewInput {
  text: string;
  msgId?: string;
  from?: string;
}

// decryptIncoming başarısızlıkta "🔒 Şifreli mesaj (…)" yer tutucusu döner;
// bu, önizleme olarak ÖNBELLEĞE YAZILMAZ.
const PLACEHOLDER_PREFIX = "\u{1F512}";

/** Boşlukları tek boşluğa indirir, code-point sınırında (emoji ortadan kesilmez) keser. */
export function makePreviewText(raw: string, max: number = PREVIEW_MAX_CHARS): string {
  const flat = raw.replace(/\s+/g, " ").trim();
  const chars = Array.from(flat);
  return chars.length <= max ? flat : chars.slice(0, max).join("") + "…";
}

function storageKey(accountDid: string): string {
  return KEY_PREFIX + accountDid;
}

function readMap(accountDid: string): PreviewMap {
  try {
    const raw = localStorage.getItem(storageKey(accountDid));
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    return parsed as PreviewMap;
  } catch {
    return {};
  }
}

export function readPreviews(accountDid: string | null | undefined): PreviewMap {
  if (!accountDid) return {};
  return readMap(accountDid);
}

/** Sohbetin son mesaj önizlemesini yazar. Boş/yer tutucu metin yazılmaz. */
export function writePreview(
  accountDid: string | null | undefined,
  convId: string,
  input: PreviewInput
): void {
  if (!accountDid || !convId) return;
  if (input.text.startsWith(PLACEHOLDER_PREFIX)) return;
  const text = makePreviewText(input.text);
  if (!text) return;

  try {
    const entry: PreviewEntry = {
      text,
      msgId: input.msgId ?? "",
      at: Date.now(),
      ...(input.from ? { from: input.from } : {}),
    };
    const next: PreviewMap = { ...readMap(accountDid), [convId]: entry };
    localStorage.setItem(storageKey(accountDid), JSON.stringify(next));
    if (typeof window !== "undefined" && typeof window.dispatchEvent === "function") {
      window.dispatchEvent(new Event(PREVIEW_UPDATED_EVENT));
    }
  } catch {
    // localStorage dolu/erişilemez: önizleme yok sayılır, liste "🔒 Şifreli" kalır.
  }
}

/**
 * Listede gösterilecek önizleme metni; güvenilir değilse null ("🔒 Şifreli" kalır).
 *
 * Bir kayıt YALNIZ şu durumlarda güncel sayılır:
 *  - sunucu son mesaj id'si vermiyor (grup: MLS gönderimleri conversations.last_msg_*
 *    alanlarını güncellemez → doğrulanamaz, bu tarayıcının gördüğü son mesaj), ya da
 *  - kaydın msgId'si sunucunun last_msg_id'siyle eşleşiyor, ya da
 *  - kayıt, listenin sunucu verisini çektiği andan SONRA yazıldı (aynı istemci saati,
 *    saat kayması yok): liste verisi bu mesajdan eski.
 * Aksi halde sunucuda görmediğimiz daha yeni bir mesaj var (çevrimdışı/başka cihaz):
 * eski önizlemeyi göstermek yanlış olur.
 */
export function selectPreview(
  entry: PreviewEntry | undefined,
  conv: { last_msg_id?: string },
  listLoadedAt: number
): string | null {
  if (!entry || typeof entry.text !== "string" || entry.text === "") return null;
  const serverLast = conv.last_msg_id;
  const isFresh = !serverLast || entry.msgId === serverLast || entry.at > listLoadedAt;
  if (!isFresh) return null;
  return entry.from ? `${entry.from}: ${entry.text}` : entry.text;
}
