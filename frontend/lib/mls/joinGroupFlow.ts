// B10 Faz 1 — mobile/lib/mls/joinGroupFlow.ts'nin portu. Sıra KRİTİK,
// local-önce (mobile ile AYNI ilke — bkz. mobile dosyasındaki not):
//   1. kendi KeyPackage'ını YÜKLE (loadOwnKeyPackage) — YENİ ÜRETME.
//   2. joinFromWelcomeWire (local, ağ yok).
//   3. saveGroupState — AĞDAN ÖNCE (ts-mls state kullanıcının tek
//      kurtarılamaz kopyası).
//   4. api.mlsJoinGroup(groupId, welcomeId, epoch) — üyelik teyidi + welcome
//      ack TEK ÇAĞRIDA (backend HandleMLSJoinGroup, mobile ile aynı).
// Adım 4 başarısız olursa local state SİLİNMEZ — kullanıcı fiilen zaten
// grupta, sadece backend'e "geldim" bildirimi eksik kalmış (retry edilebilir,
// idempotent).
"use client";
import { getMlsCiphersuiteImpl, joinFromWelcomeWire } from "./group";
import { saveGroupState, loadOwnKeyPackage, type MlsStores } from "./mls-store";
import { api } from "../api";

export interface PendingWelcome {
  id: string;
  group_id: string;
  welcome_b64: string;
  created_at: string;
}

export interface AcceptMlsWelcomeParams {
  ownDid: string;
  welcome: PendingWelcome;
  stores?: MlsStores;
}

export interface AcceptMlsWelcomeResult {
  groupId: string;
  epoch: number;
}

export async function acceptMlsWelcome(params: AcceptMlsWelcomeParams): Promise<AcceptMlsWelcomeResult> {
  const { ownDid, welcome, stores = {} } = params;

  const cs = await getMlsCiphersuiteImpl();
  const ownKp = await loadOwnKeyPackage(ownDid, stores);
  if (!ownKp) {
    throw new Error(
      "acceptMlsWelcome: kendi KeyPackage'ı bulunamadı — bu Welcome ile join edilemez (davet edenin fetch ettiği eski private key gerekli)"
    );
  }

  const newState = await joinFromWelcomeWire(welcome.welcome_b64, ownKp.keyPackageWireB64, ownKp.privateKeyPackage, cs);

  await saveGroupState(welcome.group_id, newState, stores);
  const epoch = Number(newState.groupContext.epoch);

  await api.mlsJoinGroup(welcome.group_id, welcome.id, epoch);

  return { groupId: welcome.group_id, epoch };
}
