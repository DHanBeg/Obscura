// L2 Tuğla 5c bölüm 2 — Bob'u davet-EDİLEBİLİR yapar.
//
// TEK KeyPackage, HAVUZ DEĞİL: mls-store.ts'in own-keypackage saklama alanı
// (obscura_mls_keypkg_{did}) tek slot — bir havuzdaki N farklı KeyPackage'ın
// HER BİRİNİN kendi private key'ini ayrı saklamak (bir Welcome geldiğinde
// hangi KeyPackage'ı hedeflediğini çözüp doğru private key'i seçmek) bugünkü
// şemayla YAPILAMAZ — bilerek yapılmadı. N'den fazla üretip yalnızca sonuncuyu
// saklamak, önceki N-1'i tüketen davetleri SESSİZCE join-edilemez bırakırdı
// (yanlış private key'le join denemesi başarısız olur) — havuzdan tek'e
// düşmek daha güvenli.
//
// Sonuç: KeyPackage tek kullanımlıktır (backend mls_handlers.go, GET
// /v1/mls/key-package/{did} → used=0 filtresiyle tüketir). Biri Bob'u davet
// edip tüketince, Bob YENİDEN upload edene kadar davet-edilemez olur — VE
// bunu öğrenmesinin proaktif bir yolu yok (mlsApi.ts'in 8 endpoint'i arasında
// "kaç KeyPackage'ım kaldı" diye sorulabilecek bir uç nokta yok; Alice'in
// isteği 404 alır, Bob'a hiçbir sinyal gitmez). Bilinen bir sınır — gerçek
// çözüm (ref-bazlı çoklu-KeyPackage saklama + backend'de "kalan sayı"
// endpoint'i) ayrı bir tuğla.
import { getMlsCiphersuiteImpl, createOwnKeyPackage } from "./group";
import { saveOwnKeyPackage, loadOwnKeyPackage, hasUploadedKeyPackage, markKeyPackageUploaded } from "./mls-store";
import { uploadKeyPackage } from "./mlsApi";

export async function ensureInvitable(ownDid: string): Promise<void> {
  let ownKp = await loadOwnKeyPackage(ownDid);
  if (!ownKp) {
    const cs = await getMlsCiphersuiteImpl();
    ownKp = await createOwnKeyPackage(ownDid, cs);
    await saveOwnKeyPackage(ownDid, ownKp);
  } else if (await hasUploadedKeyPackage(ownDid)) {
    return; // zaten var + zaten yüklü — bkz. modül üstü bilinen sınır
  }

  await uploadKeyPackage(ownKp.keyPackageWireB64);
  await markKeyPackageUploaded(ownDid);
}
