// B10 Faz 1 — mobile/lib/mls/inviteBootstrap.ts'nin portu. Web kullanıcısını
// davet-EDİLEBİLİR yapar (kendi KeyPackage'ını üretip backend'e yükler) —
// web'in KENDİSİ grup kuramaz ama mobil onu bir gruba EKLEYEBİLSİN diye bu
// gerekli. Aynı tek-slot/tek-kullanımlık KeyPackage sınırı mobile ile aynı
// (bkz. mobile dosyasındaki not — havuz değil, tek KeyPackage).
"use client";
import { getMlsCiphersuiteImpl, createOwnKeyPackage } from "./group";
import { saveOwnKeyPackage, loadOwnKeyPackage, hasUploadedKeyPackage, markKeyPackageUploaded, type MlsStores } from "./mls-store";
import { api } from "../api";

export async function ensureInvitable(ownDid: string, stores: MlsStores = {}): Promise<void> {
  let ownKp = await loadOwnKeyPackage(ownDid, stores);
  if (!ownKp) {
    const cs = await getMlsCiphersuiteImpl();
    ownKp = await createOwnKeyPackage(ownDid, cs);
    await saveOwnKeyPackage(ownDid, ownKp, stores);
  } else if (await hasUploadedKeyPackage(ownDid, stores)) {
    return;
  }

  await api.mlsUploadKeyPackage(ownKp.keyPackageWireB64);
  await markKeyPackageUploaded(ownDid, stores);
}
