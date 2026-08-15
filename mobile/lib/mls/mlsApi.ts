// L2 Tuğla 4b — MLS API client. Saf transport: 8 endpoint HTTP wrapper.
// ts-mls'e DOKUNMAZ (group.ts kriptosu burada yok) — sadece wire byte'ları
// backend'e taşır. Auth mevcut apiFetch (lib/api.ts) üzerinden — Bearer
// token kaynağı/envelope soyma burada tekrar yazılmıyor.
import { apiFetch } from "../api";

export interface UploadKeyPackageResult {
  id: string;
  expires_at: string;
}

/** POST /v1/mls/key-package — kendi KeyPackage'ını yükler (bkz. createOwnKeyPackage, Tuğla 4a). */
export function uploadKeyPackage(keyPackageWireB64: string, ttlDays?: number): Promise<UploadKeyPackageResult> {
  return apiFetch("/v1/mls/key-package", {
    method: "POST",
    body: JSON.stringify({
      key_package_b64: keyPackageWireB64,
      ...(ttlDays !== undefined ? { ttl_days: ttlDays } : {}),
    }),
  });
}

export interface GetKeyPackageResult {
  key_package_b64: string;
  target_did: string;
}

/** GET /v1/mls/key-package/{did} — hedefin tek-kullanımlık KeyPackage'ını tüketir. */
export function getKeyPackage(did: string): Promise<GetKeyPackageResult> {
  return apiFetch(`/v1/mls/key-package/${encodeURIComponent(did)}`);
}

export interface CreateGroupResult {
  group_id: string;
  creator_did: string;
  epoch: number;
}

/** POST /v1/mls/group — client-üretilmiş group_id'yi sunucuya bildirir (bkz. createGroupWithMember, Tuğla 4a). */
export function createGroup(groupId: string, name?: string, ciphersuite?: string): Promise<CreateGroupResult> {
  return apiFetch("/v1/mls/group", {
    method: "POST",
    body: JSON.stringify({
      group_id: groupId,
      ...(name !== undefined ? { name } : {}),
      ...(ciphersuite !== undefined ? { ciphersuite } : {}),
    }),
  });
}

export interface AddMemberResult {
  group_id: string;
  new_epoch: number;
  welcomed: string;
  broadcast: number;
}

/** POST /v1/mls/group/{id}/add — client-hesaplanmış commit+welcome'ı broadcast için sunucuya yollar. */
export function addMember(
  groupId: string,
  newMemberDid: string,
  commitB64: string,
  welcomeB64: string,
  newEpoch: number
): Promise<AddMemberResult> {
  return apiFetch(`/v1/mls/group/${encodeURIComponent(groupId)}/add`, {
    method: "POST",
    body: JSON.stringify({
      new_member_did: newMemberDid,
      commit_b64: commitB64,
      welcome_b64: welcomeB64,
      new_epoch: newEpoch,
    }),
  });
}

export interface PendingWelcome {
  id: string;
  group_id: string;
  welcome_b64: string;
  created_at: string;
}

/** GET /v1/mls/welcomes — çağıran için bekleyen Welcome'lar. */
export function getWelcomes(): Promise<PendingWelcome[]> {
  return apiFetch("/v1/mls/welcomes");
}

export interface JoinGroupResult {
  group_id: string;
  role: string;
  name: string;
  ciphersuite: string;
  epoch: number;
  member_count: number;
}

/** POST /v1/mls/group/{id}/join — Welcome'ı client'ta işledikten sonra üyeliği teyit eder. */
export function joinGroup(groupId: string, welcomeId: string | undefined, epoch: number): Promise<JoinGroupResult> {
  return apiFetch(`/v1/mls/group/${encodeURIComponent(groupId)}/join`, {
    method: "POST",
    body: JSON.stringify({
      ...(welcomeId !== undefined ? { welcome_id: welcomeId } : {}),
      epoch,
    }),
  });
}

export interface SendGroupMessageResult {
  id: string;
  created_at: string;
  delivered: number;
  queued: number;
}

/** POST /v1/mls/group/{id}/message — şifreli application mesajını broadcast için yollar (bkz. encryptGroupMessage, Tuğla 4a). */
export function sendGroupMessage(groupId: string, ciphertextB64: string, epoch: number): Promise<SendGroupMessageResult> {
  return apiFetch(`/v1/mls/group/${encodeURIComponent(groupId)}/message`, {
    method: "POST",
    body: JSON.stringify({ ciphertext_b64: ciphertextB64, epoch }),
  });
}

export interface GroupMessageEntry {
  id: string;
  sender_did: string;
  ciphertext_b64: string;
  /** Aynı kuyrukta iki tür kayıt var (Tuğla 4e): "application" = şifreli
   * kullanıcı mesajı, "commit" = epoch geçişi. Commit'ler bir application
   * mesajı gibi çözülemez; çağıran bu alana bakarak ayırmak ZORUNDA. */
  content_type: "application" | "commit";
  epoch: number;
  created_at: string;
}

export interface GetGroupMessagesResult {
  group_id: string;
  messages: GroupMessageEntry[];
}

/** GET /v1/mls/group/{id}/messages — sinceEpoch verilirse yalnızca o epoch'tan itibaren. */
export function getGroupMessages(groupId: string, sinceEpoch?: number): Promise<GetGroupMessagesResult> {
  const qs = sinceEpoch !== undefined ? `?since_epoch=${sinceEpoch}` : "";
  return apiFetch(`/v1/mls/group/${encodeURIComponent(groupId)}/messages${qs}`);
}
