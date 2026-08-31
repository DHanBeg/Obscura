// B10.2 Tuğla 2 — mobile/lib/mls/createGroupFlow.ts'nin create-half alt-kümesi.
// KAPSAM SINIRI: yalnız boş (creator-only) grup kurma. Mobile'ın orijinali
// (createMlsGroupConversation) üye ekleme + Welcome dağıtımı + conversation
// satırı kurmayı da tek akışta yapıyor (createGroupFlow.ts:39-103, adım 3-4/
// 7/8) — o adımlar BURADA YOK, Tuğla 3'e kadar taşınmadı (bkz. B10.2 Faz 0.5
// kararı: backend'in conversations kapısı grup tipi için >=1 üye zorunlu
// kılıyor — extra_handlers.go:267 — bu yüzden conversation satırı, ilk
// addMember ile AYNI adımda kurulmalı, create'ten ayrılamaz).
//
// Sıra KRİTİK, local-önce (joinGroupFlow.ts'teki AYNI ilke, bkz. o dosyanın
// üstü notu):
//   1. kendi KeyPackage'ı yoksa üret + sakla (mobile createGroupFlow.ts:50-56
//      ile AYNI — sonraki bir davet için kullanılabilir olsun diye, bu akışın
//      kendisi tüketmiyor).
//   2. group_id üret (lokal, ağ yok).
//   3. createOwnGroup (local, ağ yok) — epoch-0, tek-yapraklı state.
//   4. saveGroupState — AĞDAN ÖNCE. ts-mls state kullanıcının TEK
//      kurtarılamaz kopyası (backend group secret TUTMUYOR).
//   5. api.mlsCreateGroup — backend ack. BAŞARISIZ olursa local state
//      SİLİNMEZ (kullanıcı aynı group_id ile retry edebilsin diye) — tıpkı
//      joinGroupFlow.ts'nin adım 4 notu gibi, burada da idempotent: backend
//      HandleMLSCreateGroup aynı group_id'yi "ON CONFLICT DO NOTHING" ile
//      karşılıyor (mls_handlers.go:171-176).
"use client";
import { getMlsCiphersuiteImpl, createOwnGroup, createOwnKeyPackage, type OwnKeyPackage } from "./group";
import { saveGroupState, loadOwnKeyPackage, saveOwnKeyPackage, type MlsStores } from "./mls-store";
import { generateGroupId } from "./groupId";
import { api } from "../api";

export interface CreateMlsOwnGroupParams {
  ownDid: string;
  name?: string;
  stores?: MlsStores;
}

export interface CreateMlsOwnGroupResult {
  groupId: string;
  epoch: number;
}

export async function createMlsOwnGroup(params: CreateMlsOwnGroupParams): Promise<CreateMlsOwnGroupResult> {
  const { ownDid, name, stores = {} } = params;

  const cs = await getMlsCiphersuiteImpl();

  // 1. kendi KeyPackage'ı yoksa üret + sakla.
  let ownKp: OwnKeyPackage | null = await loadOwnKeyPackage(ownDid, stores);
  if (!ownKp) {
    ownKp = await createOwnKeyPackage(ownDid, cs);
    await saveOwnKeyPackage(ownDid, ownKp, stores);
  }

  // 2. group_id üret — lokal, ağ yok.
  const groupId = generateGroupId();

  // 3. CREATE-half — local, ağ yok. addMember YOK (Tuğla 3).
  const state = await createOwnGroup(groupId.bytes, ownKp, cs);

  // 4. LOCAL ÖNCE — ağ adımından ÖNCE kaydet (Karar B).
  await saveGroupState(groupId.b64, state, stores);
  const epoch = Number(state.groupContext.epoch);

  // 5. backend MLS grup kaydı (ack). Hata fırlarsa local state (adım 4'te
  // yazılmış) SİLİNMEZ — çağıran aynı groupId.b64 ile api.mlsCreateGroup'u
  // retry edebilir, HandleMLSCreateGroup idempotent (ON CONFLICT DO NOTHING).
  await api.mlsCreateGroup(groupId.b64, name);

  return { groupId: groupId.b64, epoch };
}
