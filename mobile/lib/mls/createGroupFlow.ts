// L2 Tuğla 5b-2 Parça B — new-group.tsx submit'inin çağırdığı, saf (UI'sız)
// grup-kurma akışı. Sıra KRİTİK, local-önce:
//
//   1. group_id üret (lokal)
//   2. kendi KeyPackage yoksa üret + sakla (#10: local-only)
//   3-4. her üyenin KeyPackage'ını çek, SIRAYLA local commit zinciri kur
//   5. saveGroupState — AĞDAN ÖNCE. ts-mls state kullanıcının TEK
//      kurtarılamaz kopyası (backend group secret TUTMUYOR); backend
//      adımları (6/7/8) tekrar denenebilir, bu denenemez.
//   6. backend MLS grup kaydı
//   7. her üyeye kendi commit+welcome'ı (backend'in epoch-sıra beklentisiyle
//      uyumlu SIRA, bkz. Tuğla 4e)
//   8. conversation + mls_group_id link (5b-1)
//
// MÜHÜR: 6/7/8'den biri başarısız olursa saveGroupState'te yazılan state
// SİLİNMEZ (kullanıcı aynı group_id ile devam edebilsin diye — bkz. modül
// üstü not) ve plaintext-fallback YOK — hata çağırana fırlatılır, grup ya
// MLS ile kurulur ya hiç kurulmaz.
import type { ClientState } from "ts-mls";
import { getMlsCiphersuiteImpl, createOwnKeyPackage, createGroupWithMember, addMemberToGroup, type OwnKeyPackage } from "./group";
import { saveGroupState, loadOwnKeyPackage, saveOwnKeyPackage } from "./mls-store";
import { generateGroupId } from "./groupId";
import { getKeyPackage, createGroup as createGroupOnServer, addMember as addMemberOnServer } from "./mlsApi";
import { api } from "../api";

export interface CreateMlsGroupConversationParams {
  ownDid: string;
  name: string;
  memberDids: string[];
  description?: string;
  isPublic?: boolean;
}

export interface CreateMlsGroupConversationResult {
  convId: string;
  groupId: string;
}

export async function createMlsGroupConversation(
  params: CreateMlsGroupConversationParams
): Promise<CreateMlsGroupConversationResult> {
  const { ownDid, name, memberDids, description, isPublic } = params;
  if (memberDids.length === 0) {
    throw new Error("createMlsGroupConversation: en az 1 üye gerekli");
  }

  // 1. group_id üret — lokal, ağ yok.
  const groupId = generateGroupId();

  // 2. kendi KeyPackage'ı yoksa üret + sakla.
  const cs = await getMlsCiphersuiteImpl();
  let ownKp: OwnKeyPackage | null = await loadOwnKeyPackage(ownDid);
  if (!ownKp) {
    ownKp = await createOwnKeyPackage(ownDid, cs);
    await saveOwnKeyPackage(ownDid, ownKp);
  }

  // 3-4. üyelerin KeyPackage'larını çek + local commit zinciri — SIRALI
  // (MLS commit'leri sıkı sıralı, paralel eklenemez; her adım bir öncekinin
  // newState'ini girdi alır).
  const perMember: { memberDid: string; commitWireB64: string; welcomeWireB64: string; newEpoch: number }[] = [];
  let state: ClientState | undefined;
  for (const memberDid of memberDids) {
    const memberKp = await getKeyPackage(memberDid);
    const result = state
      ? await addMemberToGroup(state, memberKp.key_package_b64, cs)
      : await createGroupWithMember(ownKp, groupId.bytes, memberKp.key_package_b64, cs);
    state = result.newState;
    perMember.push({
      memberDid,
      commitWireB64: result.commitWireB64,
      welcomeWireB64: result.welcomeWireB64,
      newEpoch: result.newEpoch,
    });
  }

  // 5. LOCAL ÖNCE — ağ adımlarından ÖNCE kaydet (bkz. modül üstü not).
  await saveGroupState(groupId.b64, state!);

  // 6. backend MLS grup kaydı.
  await createGroupOnServer(groupId.b64, name);

  // 7. her üyeye kendi commit+welcome'ı — SIRAYLA.
  for (const m of perMember) {
    await addMemberOnServer(groupId.b64, m.memberDid, m.commitWireB64, m.welcomeWireB64, m.newEpoch);
  }

  // 8. conversation + link. MÜHÜR: plaintext-fallback yok, burası MLS
  // dışına asla düşmez.
  const convResult: { conv_id?: string } = await api.createConversation({
    type: "group",
    name,
    members: memberDids,
    ...(description !== undefined ? { description } : {}),
    ...(isPublic !== undefined ? { is_public: isPublic } : {}),
    mls_group_id: groupId.b64,
  });
  if (!convResult.conv_id) {
    throw new Error("createMlsGroupConversation: conversation oluşturuldu ama conv_id dönmedi");
  }

  return { convId: convResult.conv_id, groupId: groupId.b64 };
}
