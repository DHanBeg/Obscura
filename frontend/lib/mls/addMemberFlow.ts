// B10.2 Tuğla 3 — web'den mevcut (kendi kurduğu) bir gruba İLK üyeyi ekler +
// backend'in welcome-kuyruğuna yazdırır + conversation satırını kurar.
// Mobile'da doğrudan karşılığı YOK (createGroupFlow.ts:39-103 create+add+
// conversation'ı tek akışta yapar, Tuğla 2 create'i ayırdığı için burası onun
// "add" yarısı + conversation kapanışı).
//
// SIRA (Karar B'nin addMember için TERSİ — bkz. group.ts:addMemberToOwnGroup
// üstü not): Tuğla 2'de "local'e ÖNCE yaz, ağ SONRA" ilkesi buraya UYMAZ.
// addMember'ın speküle ettiği newState, backend CAS'i (Tuğla 1,
// advanceGroupEpoch) 409 dönerse GEÇERSİZDİR — epoch yarışını kaybetmiş
// demektir. Bu yüzden: backend'den 200 gelmeden saveGroupState ÇAĞRILMAZ.
//
// 409 RE-SYNC + RETRY (mobile'da YOK — CAS'ten önce yazıldı, bu davranış
// PORT değil, Tuğla 1/3 ile gelen YENİ bir sözleşme):
//   1. speküle commit üret (local, ağ yok)
//   2. POST commit+welcome → backend
//   3a. 200 → şimdi persist et, bitti.
//   3b. 409 → speküle newState AT (hiç kaydedilmedi) → GET .../messages ile
//       kazanan commit(ler)i çek → sırayla işleyip local baseline'ı ilerlet →
//       o yeni baseline'dan YENİDEN commit üret → 2'ye dön (maxRetries'e kadar).
// Başka HİÇBİR hata (403/404/500/ağ hatası) için retry YOK — fail-loud
// (bkz. addMemberToOwnGroupFlow, keypackage fetch noktası).
//
// SELF-COLLISION (B10.2 Tuğla 3 kanıt turu, canlı backend + gerçek ts-mls ile
// bulundu — mock DEĞİL): applyCommitWire yalnızca BAŞKA bir üyenin (farklı
// MLS leaf) commit'ini işleyebilir — bir kontrol testiyle doğrulandı (Alice
// commit atar, Bob FARKLI leaf, applyCommitWire ile epoch ilerletir: BAŞARILI).
// Ama kazanan commit'in göndereni BİZZAT BİZSEK (aynı kullanıcının başka
// sekmesi/cihazı — backend addMember yetkisi role'süz, herhangi bir mevcut
// üye ekleyebiliyor, bkz. mls_handlers.go:300-307 — bu yüzden aynı kullanıcı
// iki cihazdan/sekmeden eş-zamanlı add DENEYEBİLİR), processMessage kendi
// commit'ini "alıcı" gibi çözemiyor — ts-mls "aes/gcm: invalid ghash tag" ile
// patlıyor (ham, tanı konulamaz bir hata). Kazanan commit'in sender_did'i
// KENDİ DID'imizle eşleşiyorsa applyCommitWire'a HİÇ girilmez — tanımlı,
// net bir hata fırlatılır (fail-loud, local state kirlenmez, orphan yok).
// FARKLI-leaf durumu (gerçek 2. bir üyenin commit'i) DOKUNULMADI, retry AYNEN
// çalışıyor (bkz. joinGroupFlow.ts:acceptMlsWelcome — web'de gerçek non-creator
// üyelik VAR, backend'in rol-serbest addMember yetkisiyle birleşince bu yol
// gerçek dünyada tetiklenebilir).
"use client";
import { getMlsCiphersuiteImpl, addMemberToOwnGroup, applyCommitWire } from "./group";
import { saveGroupState, loadGroupState, type MlsStores } from "./mls-store";
import { api, ApiError } from "../api";

export interface AddMemberToOwnGroupParams {
  /** Self-collision tespiti için gerekli (bkz. dosya üstü not) — kazanan
   * commit'in sender_did'i bununla eşleşirse applyCommitWire'a hiç girilmez. */
  ownDid: string;
  groupId: string;
  targetDid: string;
  stores?: MlsStores;
  /** Eş-zamanlı epoch yarışı için üst sınır — tükenirse net hata (sonsuz döngü yok). */
  maxRetries?: number;
}

export interface AddMemberToOwnGroupResult {
  epoch: number;
  /** Kaç denemede kazanıldığı — 1 = ilk denemede (çakışma yok), >1 = 409 sonrası re-sync ile kazanıldı. */
  attempts: number;
}

const DEFAULT_MAX_RETRIES = 3;

interface GroupMessageRow {
  content_type: "application" | "commit";
  ciphertext_b64: string;
  epoch: number;
  /** mls_handlers.go:1052/1068 — GET .../messages zaten döndürüyor, backend
   * DEĞİŞMEDİ. Self-collision tespiti bunu kullanıyor. */
  sender_did: string;
}

/** create-half (Tuğla 2) tarafından kurulmuş, henüz hiç üyesi olmayan bir
 * gruba İLK üyeyi ekler. Yalnız crypto+network — conversation satırına
 * dokunmaz (bkz. addMemberAndLinkConversation, aşağıda). */
export async function addMemberToOwnGroupFlow(params: AddMemberToOwnGroupParams): Promise<AddMemberToOwnGroupResult> {
  const { ownDid, groupId, targetDid, stores = {}, maxRetries = DEFAULT_MAX_RETRIES } = params;
  const cs = await getMlsCiphersuiteImpl();

  // TEK SEFER, döngü DIŞINDA (bkz. kanıt turu bulgusu): backend'in KeyPackage
  // fetch'i TEK KULLANIMLIK (mls_handlers.go:184-195, GET = tüket). Döngü
  // İÇİNDE her attempt'te tekrar fetch edilirse, 409 sonrası retry (attempt 2)
  // aynı hedef için backend'de artık "used" olan KeyPackage'ı ARAR ve 404
  // alır — GERÇEK farklı-leaf race testinde canlı yakalandı (retry kendi
  // hedefinin KeyPackage'ını kendi ilk denemesinde tüketiyordu). Public
  // KeyPackage materyali attempt'ler arası SABİT (yalnız gönderenin local
  // baseline'ı değişiyor) — tek fetch + tekrar kullanım DOĞRU ve GÜVENLİ.
  // FAIL-LOUD: hedefin KeyPackage'ı yok/tükenmişse backend 404 döner, apiFetch
  // bunu fırlatır — burada YUTULMAZ, sessiz-ekleme YOK.
  const targetKp: { key_package_b64: string; target_did: string } = await api.mlsGetKeyPackage(targetDid);

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    const baseline = await loadGroupState(groupId, stores);
    if (!baseline) throw new Error("addMemberToOwnGroupFlow: local grup state'i bulunamadı (önce createOwnGroup çalışmalı)");

    const result = await addMemberToOwnGroup(baseline, targetKp.key_package_b64, cs);

    try {
      await api.mlsAddMember(groupId, targetDid, result.commitWireB64, result.welcomeWireB64, result.newEpoch);
      // 200 — kazandı. ANCAK ŞİMDİ persist et (bkz. dosya üstü Karar B notu).
      await saveGroupState(groupId, result.newState, stores);
      return { epoch: result.newEpoch, attempts: attempt };
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // Kaybettik — result.newState/commit hiç kaydedilmedi, hiç bir yere
        // sızmadı (yalnız yerel değişkendi). Kazanan commit(ler)i çekip
        // baseline'ı ilerlet, sonra yeniden dene.
        const sinceEpoch = Number(baseline.groupContext.epoch) + 1;
        const messagesResp: { group_id: string; messages: GroupMessageRow[] } = await api.mlsGetGroupMessages(groupId, sinceEpoch);
        const winningCommits = messagesResp.messages.filter((m) => m.content_type === "commit");
        if (winningCommits.length === 0) {
          throw new Error("addMemberToOwnGroupFlow: 409 alındı ama kazanan commit bulunamadı — re-sync imkansız");
        }
        // PRİSTİN kopyadan başla — `baseline`'ı DEĞİL (bkz. dosya üstü not,
        // kanıtla bulundu): addMemberToOwnGroup(baseline,...) YUKARIDA zaten
        // çağrıldı ve ts-mls'in createCommit'i input state'i İN-PLACE mutasyona
        // uğratıyor (secretTree ratchet ileri gidiyor — kontrol testiyle
        // doğrulandı: mutasyona uğramış state, GERÇEK farklı bir üyenin
        // GERÇEK kazanan commit'ini bile işleyemiyor, "Could not verify
        // confirmation tag"). `baseline` hiçbir zaman storage'a YAZILMADI
        // (yalnız kaybeden speküle commit için okundu) — storage'daki kopya
        // hâlâ pristine, o yüzden tekrar loadGroupState ile GÜVENLİ.
        let resynced = await loadGroupState(groupId, stores);
        if (!resynced) throw new Error("addMemberToOwnGroupFlow: 409 sonrası pristine state yeniden yüklenemedi");
        for (const c of winningCommits) {
          // SELF-COLLISION (bkz. dosya üstü not, kanıtla bulundu): kazanan
          // commit BİZİM gönderdiğimizse (başka sekme/cihaz, aynı kullanıcı)
          // applyCommitWire'ı HİÇ ÇAĞIRMA — processMessage kendi commit'ini
          // çözemiyor (ham "aes/gcm: invalid ghash tag"). Tanımlı hata fırlat,
          // local state (baseline) DOKUNULMADAN kalır — kirlenme/orphan yok.
          if (c.sender_did === ownDid) {
            throw new Error(
              `addMemberToOwnGroupFlow: bu ekleme başka bir sekmede/cihazda (aynı kullanıcı, ${ownDid}) yarışı kaybetti — bu cihazdaki grup durumu artık geride kalmış olabilir çözülemez (self-collision, kripto ile kurtarılamaz). Sayfayı yenileyin ya da tekrar deneyin.`
            );
          }
          resynced = await applyCommitWire(resynced, c.ciphertext_b64, cs);
        }
        await saveGroupState(groupId, resynced, stores);
        continue; // maxRetries'e kadar yeni baseline'dan tekrar dene.
      }
      throw err; // 409 DIŞINDA her şey fail-loud, retry YOK.
    }
  }
  throw new Error(`addMemberToOwnGroupFlow: ${maxRetries} denemede epoch yarışı kazanılamadı`);
}

export interface AddMemberAndLinkConversationParams extends AddMemberToOwnGroupParams {
  name: string;
  description?: string;
  isPublic?: boolean;
}

export interface AddMemberAndLinkConversationResult extends AddMemberToOwnGroupResult {
  convId: string;
}

/** addMemberToOwnGroupFlow BAŞARILI olduktan SONRA conversation satırını
 * kurar (mobile/lib/mls/createGroupFlow.ts:90-97 ile AYNI body şekli —
 * type:"group", members, mls_group_id). addMember BAŞARISIZ olursa
 * (fail-loud veya retry tükenmesi) bu satıra hiç gelinmez — conversation
 * KURULMAZ, orphan yok. */
export async function addMemberAndLinkConversation(
  params: AddMemberAndLinkConversationParams
): Promise<AddMemberAndLinkConversationResult> {
  const { ownDid, groupId, targetDid, name, description, isPublic, stores, maxRetries } = params;

  const addResult = await addMemberToOwnGroupFlow({ ownDid, groupId, targetDid, stores, maxRetries });

  const convResult = await api.createConversation({
    type: "group",
    name,
    members: [targetDid],
    ...(description !== undefined ? { description } : {}),
    ...(isPublic !== undefined ? { is_public: isPublic } : {}),
    mls_group_id: groupId,
  });
  if (!convResult.conv_id) {
    throw new Error("addMemberAndLinkConversation: conversation oluşturuldu ama conv_id dönmedi");
  }

  return { ...addResult, convId: convResult.conv_id };
}
