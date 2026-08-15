// L2 Tuğla 5c — bekleyen bir MLS Welcome'ı kabul etme akışı. Sıra KRİTİK,
// local-önce (5b-2'nin createGroupFlow.ts'iyle AYNI ilke):
//
//   1. kendi KeyPackage'ını YÜKLE (loadOwnKeyPackage) — YENİ ÜRETME. Welcome,
//      Alice'in backend'den fetch ettiği ESKİ public KeyPackage'a karşılık
//      gelen private key ile şifrelendi; farklı/yeni bir key ile join
//      kriptografik olarak yanlış olur (yanlış anahtar = başarısız decrypt,
//      "sessiz" bir hata değil ama YİNE DE yanlış — bu yüzden burada
//      createOwnKeyPackage'a asla düşülmez, eksikse hata fırlatılır).
//   2. joinFromWelcomeWire (4a, ts-mls) — local, ağ yok.
//   3. saveGroupState — AĞDAN ÖNCE (5a: ts-mls state kullanıcının tek
//      kurtarılamaz kopyası; backend'e "geldim" bildirimi (adım 4) tekrar
//      denenebilir, bu denenemez).
//   4. mlsApi.joinGroup(groupId, welcomeId, epoch) — ÜYELİK TEYİDİ + WELCOME
//      ACK TEK ÇAĞRIDA (backend HandleMLSJoinGroup, welcome_id verilirse
//      mls_welcome_queue.delivered_at'i de işaretliyor — ayrı bir ack
//      endpoint'i/wrapper'ı YOK, mlsApi.ts'te böyle bir fonksiyon zaten yok).
//
// MÜHÜR (5b-2 ile aynı): adım 4 başarısız olursa local state SİLİNMEZ —
// Bob fiilen zaten grupta (state fonksiyonel), sadece backend'e "geldim"
// bildirimi eksik kalmış, retry edilebilir (join çağrısı best-effort/idempotent
// bir UPDATE'e dayanıyor, tekrar çağırmak zararsız).
import { getMlsCiphersuiteImpl, joinFromWelcomeWire } from "./group";
import { saveGroupState, loadOwnKeyPackage } from "./mls-store";
import { joinGroup as joinGroupOnServer, type PendingWelcome } from "./mlsApi";

export interface AcceptMlsWelcomeParams {
  ownDid: string;
  welcome: PendingWelcome;
}

export interface AcceptMlsWelcomeResult {
  groupId: string;
  epoch: number;
}

export async function acceptMlsWelcome(params: AcceptMlsWelcomeParams): Promise<AcceptMlsWelcomeResult> {
  const { ownDid, welcome } = params;

  // 1. kendi KeyPackage'ını yükle — eksikse hata (bkz. modül üstü not).
  const cs = await getMlsCiphersuiteImpl();
  const ownKp = await loadOwnKeyPackage(ownDid);
  if (!ownKp) {
    throw new Error(
      "acceptMlsWelcome: kendi KeyPackage'ı bulunamadı — bu Welcome ile join edilemez (Alice'in fetch ettiği eski private key gerekli)"
    );
  }

  // 2. local join (ts-mls, ağ yok).
  const newState = await joinFromWelcomeWire(welcome.welcome_b64, ownKp.keyPackageWireB64, ownKp.privateKeyPackage, cs);

  // 3. LOCAL ÖNCE — ağdan önce kaydet.
  await saveGroupState(welcome.group_id, newState);
  const epoch = Number(newState.groupContext.epoch);

  // 4. üyelik teyidi + welcome ack (tek çağrı).
  await joinGroupOnServer(welcome.group_id, welcome.id, epoch);

  return { groupId: welcome.group_id, epoch };
}
