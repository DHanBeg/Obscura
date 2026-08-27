// B11 — video/dosya/ses blob'larını yükleme öncesi şifreler. Resme (inline
// base64, tam E2E) ve konuma (blob yok) DOKUNULMAZ — bkz. B5 Faz 0.
//
// Anahtar MLS/ratchet'ten TÜRETİLMEZ: her upload için rastgele, tek-kullanımlık
// bir blob-key üretilir (crypto.getRandomValues — CSPRNG, react-native-get-
// random-values polyfill'i session-store.ts'te zaten aynı şekilde kullanılıyor,
// bkz. o dosyanın import'u). Şifreleme primitifi de YENİDEN YAZILMADI —
// session-store.ts'in ratchet-state-at-rest için kullandığı AYNI encryptBlob/
// decryptBlob (AES-256-GCM, nonce||ciphertext), burada persistent master key
// yerine medyaya özel rastgele anahtarla çağrılıyor.
//
// Anahtar mevcut E2E mesaj payload'ında taşınır (1:1=ratchet, grup=MLS) —
// chat/[id].tsx payload'a `|<keyB64>` olarak ekler. MLS/ratchet yapısına
// SIFIR dokunuş: taşınan string'in İÇERİĞİ değişiyor, taşıma mekanizması değil.
//
// Geriye uyumluluk: eski mesajlarda bu segment YOK — çağıran taraf (chat/[id].tsx)
// key parse edilemezse (undefined) blob'u şifresiz/legacy kabul eder, decrypt
// denemez (bkz. parseMediaKey).
import * as FileSystem from "expo-file-system";
import { encryptBlob, decryptBlob } from "./session-store";
import { u8ToBase64, base64ToU8 } from "./crypto";

function randomBlobKey(): Uint8Array {
  const key = new Uint8Array(32);
  crypto.getRandomValues(key); // CSPRNG — react-native-get-random-values polyfill (bkz. session-store.ts:1)
  return key;
}

// parseMediaKey — payload'ın `<url>|<keyB64>` kısmını ayırır. keyB64 yoksa
// (eski mesaj, tek segment) legacy olarak `key: null` döner — çağıran taraf
// bunu "şifresiz, direkt kullan" sinyali olarak okumalı.
export function parseMediaKey(rest: string): { url: string; keyB64: string | null } {
  const idx = rest.indexOf("|");
  if (idx === -1) return { url: rest, keyB64: null };
  return { url: rest.slice(0, idx), keyB64: rest.slice(idx + 1) };
}

// encryptFileForUpload — kaynak dosyayı (picker/recording uri) okur, rastgele
// bir blob-key ile şifreler, şifreli byte'ları YENİ bir geçici dosyaya yazar.
// Dönen tempUri mevcut api.uploadMedia() akışına (multipart, değişmedi)
// AYNEN verilir — upload endpoint'i hangi byte gelirse onu yazıyor, backend'e
// dokunulmadı.
export async function encryptFileForUpload(sourceUri: string): Promise<{ tempUri: string; keyB64: string }> {
  const plainB64 = await FileSystem.readAsStringAsync(sourceUri, { encoding: FileSystem.EncodingType.Base64 });
  const plainBytes = base64ToU8(plainB64);
  const key = randomBlobKey();
  const cipherBytes = encryptBlob(key, plainBytes);
  const cipherB64 = u8ToBase64(cipherBytes);
  const tempUri = `${FileSystem.cacheDirectory}obscura-enc-${Date.now()}-${Math.random().toString(36).slice(2, 10)}.bin`;
  await FileSystem.writeAsStringAsync(tempUri, cipherB64, { encoding: FileSystem.EncodingType.Base64 });
  return { tempUri, keyB64: u8ToBase64(key) };
}

// decryptDownloadedBlob — herkese-açık MinIO URL'inden (backend'e uğramadan,
// mevcut davranış — bkz. B11 Faz 0) ham şifreli byte'ları çeker, verilen
// anahtarla çözer. Anahtar mesaj payload'ından (E2E kanaldan) geldiği için
// bu round-trip gerçek E2E — sunucu hiçbir noktada anahtarı görmüyor.
export async function decryptDownloadedBlob(url: string, keyB64: string): Promise<Uint8Array> {
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`Medya indirilemedi: ${resp.status}`);
  const buf = await resp.arrayBuffer();
  const cipherBytes = new Uint8Array(buf);
  const key = base64ToU8(keyB64);
  return decryptBlob(key, cipherBytes);
}

// decryptToTempFile — çözülen byte'ları oynatılabilir/açılabilir bir yerel
// dosyaya yazar (expo-av gibi native player'lar data: URI değil gerçek
// dosya/URL bekler — bkz. B11 Faz 0, data-URI güvenilirliği bu kod
// tabanında hiç doğrulanmamıştı, gerçek geçici dosya tercih edildi).
export async function decryptToTempFile(url: string, keyB64: string, extension: string): Promise<string> {
  const plainBytes = await decryptDownloadedBlob(url, keyB64);
  const tempUri = `${FileSystem.cacheDirectory}obscura-dec-${Date.now()}-${Math.random().toString(36).slice(2, 10)}.${extension}`;
  await FileSystem.writeAsStringAsync(tempUri, u8ToBase64(plainBytes), { encoding: FileSystem.EncodingType.Base64 });
  return tempUri;
}
