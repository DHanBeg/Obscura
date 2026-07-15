import "react-native-get-random-values";
import { gcm } from "@noble/ciphers/aes.js";
import { secureStore, asyncStore, KeyValueStore } from "./keyValueStore";
import { hexToU8, u8ToHex } from "./crypto";

// Çözülmüş mesaj düz metinlerinin ŞİFRELİ önbelleği.
//
// Double Ratchet mesaj anahtarları TEK KULLANIMLIK (forward secrecy): bir v2
// mesaj bir kez çözüldükten sonra aynı ciphertext bir daha çözülemez. Chat
// ekranı her açılışta geçmişi sunucudan çekip yeniden çözmeye çalıştığı için
// v2 mesajlar ilk başarılı çözümden sonra buradan okunur — bu önbellek
// olmasaydı tüm eski v2 mesajlar kalıcı olarak "🔒" görünürdü. v1 (ECIES)
// mesajlar stateless olduğundan önbelleğe ihtiyaç duymaz ve buraya yazılmaz
// (v1 davranışı birebir korunuyor).
//
// Düz metinler hassas veri: master key (32 byte, SecureStore/keychain) ile
// AES-256-GCM şifrelenip AsyncStorage'a yazılır — session-store.ts ile aynı
// desen (SecureStore'un ~2KB/item sınırı yüzünden büyük blob'lar AsyncStorage'da).

const MASTER_KEY_STORE_KEY = "obscura_msgcache_master_key";
const CACHE_KEY_PREFIX = "obscura_msgcache_";

export interface CacheStores {
  secure?: KeyValueStore;
  async?: KeyValueStore;
}

function cacheStoreKey(messageId: string): string {
  return `${CACHE_KEY_PREFIX}${messageId}`;
}

async function getOrCreateMasterKey(secStore: KeyValueStore): Promise<Uint8Array> {
  const hex = await secStore.getItem(MASTER_KEY_STORE_KEY);
  if (hex) return hexToU8(hex);
  const key = new Uint8Array(32);
  crypto.getRandomValues(key);
  await secStore.setItem(MASTER_KEY_STORE_KEY, u8ToHex(key));
  return key;
}

export async function cachePlaintext(
  messageId: string,
  plaintext: string,
  stores: CacheStores = {}
): Promise<void> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const masterKey = await getOrCreateMasterKey(secStore);
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const ct = gcm(masterKey, nonce).encrypt(new TextEncoder().encode(plaintext));
  const blob = new Uint8Array(12 + ct.length);
  blob.set(nonce, 0);
  blob.set(ct, 12);

  await asyStore.setItem(cacheStoreKey(messageId), u8ToHex(blob));
}

// Kayıt yoksa VEYA çözülemiyorsa (bozulmuş blob, yanlış master key) null döner
// — fail-open: çağıran taraf normal decrypt akışını dener, hata yutulmaz ama
// önbellek sorunları mesajlaşmayı kilitlemez.
export async function getCachedPlaintext(
  messageId: string,
  stores: CacheStores = {}
): Promise<string | null> {
  const secStore = stores.secure ?? secureStore;
  const asyStore = stores.async ?? asyncStore;

  const hex = await asyStore.getItem(cacheStoreKey(messageId));
  if (!hex) return null;

  try {
    const masterKey = await getOrCreateMasterKey(secStore);
    const blob = hexToU8(hex);
    if (blob.length < 28) return null;
    const plain = gcm(masterKey, blob.slice(0, 12)).decrypt(blob.slice(12));
    return new TextDecoder().decode(plain);
  } catch {
    return null;
  }
}

// Self-destruct/expiry: mesaj sona erdiğinde çağrılır (bkz. app/_layout.tsx
// "message_expired" WS handler'ı). Backend ciphertext'i silse bile bu
// önbellekte çözülmüş düz metin kalırsa self-destruct SAHTE olur — bu yüzden
// expire event'inde purge ZORUNLU. Kayıt yoksa sessizce no-op.
export async function purgePlaintext(
  messageId: string,
  stores: CacheStores = {}
): Promise<void> {
  const asyStore = stores.async ?? asyncStore;
  await asyStore.deleteItem(cacheStoreKey(messageId));
}
