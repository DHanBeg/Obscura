const BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export class AuthError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "AuthError";
    this.status = status;
  }
}

export async function apiFetch(path: string, opts: RequestInit = {}) {
  const token = typeof window !== "undefined" ? localStorage.getItem("obscura_token") : null;
  const res = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers || {}),
    },
  });
  const data = await res.json();
  if (!data.success) {
    if (res.status === 401) throw new AuthError(data.error || "Oturum süresi doldu", 401);
    throw new Error(data.error || "Bir hata oluştu");
  }
  return data.data;
}

export const api = {
  // ── Auth ───────────────────────────────────────────────────────────
  requestOTP: (phone: string) =>
    apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) }),
  verifyOTP: (body: object) =>
    apiFetch("/v1/auth/verify-otp", { method: "POST", body: JSON.stringify(body) }),

  // ── Kullanıcı ──────────────────────────────────────────────────────
  getMe: () => apiFetch("/v1/users/me"),
  getUser: (did: string) => apiFetch(`/v1/users/${did}`),
  searchUsers: (q: string) => apiFetch(`/v1/users/search?q=${encodeURIComponent(q)}`),
  updateMe: (body: { display_name?: string; username?: string; avatar_url?: string }) =>
    apiFetch("/v1/users/me", { method: "PATCH", body: JSON.stringify(body) }),

  // ── Mesajlaşma ─────────────────────────────────────────────────────
  getConversations: () => apiFetch("/v1/conversations"),
  getMessages: (convId: string) => apiFetch(`/v1/conversations/${convId}/messages`),
  sendMessage: (body: object) =>
    apiFetch("/v1/messages", { method: "POST", body: JSON.stringify(body) }),
  createConversation: (body: {
    peer_did?: string;
    is_group?: boolean;
    type?: "direct" | "group" | "channel" | "community";
    name?: string;
    description?: string;
    is_public?: boolean;
    members?: string[];
  }): Promise<{ conv_id: string; name?: string; conv_type?: string; description?: string; members?: number }> =>
    apiFetch("/v1/conversations", { method: "POST", body: JSON.stringify(body) }),
  deleteMessage: (id: string) =>
    apiFetch(`/v1/messages/${id}`, { method: "DELETE" }),
  // B7 Faz 1'de kurulan yetki yüzeyi (group_handlers.go, extra_handlers.go):
  // davet linki (grup/kanal admin-only, topluluk herhangi üye — backend
  // kararı, burada sadece çağrılıyor).
  createConvInvite: (convId: string): Promise<{ token: string; slug?: string; invite_url: string }> =>
    apiFetch(`/v1/conversations/${convId}/invite/create`, { method: "POST", body: JSON.stringify({}) }),
  // is_public=1 grup/kanal/topluluk keşfi ve invite'sız katılma
  // (extra_handlers.go HandleDiscoverConversations, group_handlers.go
  // HandleSelfJoinConversation). selfJoin yanıtı mls_synced:false döner —
  // SADECE HTTP/SQL üyeliği verir, MLS grup üyeliği ayrı senkronize edilmez.
  discoverConversations: (q?: string): Promise<Array<{ id: string; name: string; conv_type: string; member_count: number }>> =>
    apiFetch(`/v1/conversations/discover${q ? `?q=${encodeURIComponent(q)}` : ""}`),
  selfJoinConversation: (convId: string): Promise<{ conv_id: string; conv_name: string; conv_type: string; mls_synced: boolean; status: "joined" | "already_member" }> =>
    apiFetch(`/v1/conversations/${convId}/join`, { method: "POST", body: JSON.stringify({}) }),
  // Mesaj durum sistemi (Spec Bölüm 6.4)
  markMessageRead: (id: string): Promise<void> =>
    apiFetch(`/v1/messages/${id}/read`, { method: "POST", body: JSON.stringify({}) }),
  getMessageStatus: (id: string): Promise<{ msg_id: string; status: string; delivered_at?: string; read_at?: string }> =>
    apiFetch(`/v1/messages/${id}/status`),

  // ── Kredi ──────────────────────────────────────────────────────────
  getCreditScore: () => apiFetch("/v1/credit/score"),
  getCreditHistory: (limit = 50) => apiFetch(`/v1/credit/history?limit=${limit}`),
  reportSpam: (body: object) =>
    apiFetch("/v1/spam/report", { method: "POST", body: JSON.stringify(body) }),
  /**
   * ZK proof tabanlı kredi claim (Spec Bölüm 5.5).
   * @param proofType - "age" | "activity" | "node" | "endorsement" | "streak" | "msg_count"
   * @param proof     - snarkjs Groth16 proof nesnesi (groth16.fullProve'dan dönen proof alanı)
   * @param publicSignals - ZK kanıtının genel sinyalleri dizisi
   * @returns { points_awarded, new_score, next_claim_at }
   */
  claimZKCredit: (
    proofType: "age" | "activity" | "node" | "endorsement" | "streak" | "msg_count",
    proof: object,
    publicSignals: string[]
  ): Promise<{ points_awarded: number; new_score: number; next_claim_at: string }> =>
    apiFetch("/v1/credit/claim-zk", {
      method: "POST",
      body: JSON.stringify({ proof_type: proofType, proof, public_signals: publicSignals }),
    }),

  // ── Medya ──────────────────────────────────────────────────────────
  uploadMedia: async (file: File, type: "avatar" | "media" | "voice" = "media") => {
    const token = typeof window !== "undefined" ? localStorage.getItem("obscura_token") : null;
    const formData = new FormData();
    formData.append("file", file);
    formData.append("type", type);
    const res = await fetch(`${BASE}/v1/media/upload`, {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: formData,
    });
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Yükleme başarısız");
    return data.data as { url: string; key: string; type: string };
  },

  // ── E2EE Prekey ────────────────────────────────────────────────────
  uploadPrekeys: (body: object) =>
    apiFetch("/v1/keys/upload", { method: "POST", body: JSON.stringify(body) }),
  getPreKeyBundle: (did: string) => apiFetch(`/v1/keys/${did}`),

  // ── ZK ─────────────────────────────────────────────────────────────
  verifyZKProof: (body: { proof_json: string; circuit_id: string; public_inputs: string[] }) =>
    apiFetch("/v1/zk/verify", { method: "POST", body: JSON.stringify(body) }),

  // ── ZK-ID Kimlik Sistemi (Spec Bölüm 5.2-5.3) ─────────────────────
  // proof ve publicParams base64 veya JSON string olarak gönderilebilir.
  // Secret asla backend'e gönderilmez.
  updateZkId: (proof: object, publicSignals: string[]): Promise<{ zk_id_verified: boolean; message: string }> =>
    apiFetch("/v1/auth/zk-id-update", {
      method: "POST",
      body: JSON.stringify({
        zk_id_proof: btoa(JSON.stringify(proof)),
        zk_id_public: btoa(JSON.stringify(publicSignals)),
      }),
    }),

  // ── Cihaz ──────────────────────────────────────────────────────────
  registerDevice: (platform: "fcm" | "apns", token: string) =>
    apiFetch("/v1/devices/register", { method: "POST", body: JSON.stringify({ platform, token }) }),

  // ── WebRTC ─────────────────────────────────────────────────────────
  getTurnCredentials: () => apiFetch("/v1/rtc/turn-credentials"),

  // ── Node ───────────────────────────────────────────────────────────
  nodeStatus: () => apiFetch("/v1/node/status"),

  // ── MLS (RFC 9420) ─────────────────────────────────────────────────
  mlsUploadKeyPackage: (body: object) =>
    apiFetch("/v1/mls/key-package", { method: "POST", body: JSON.stringify(body) }),
  mlsGetKeyPackage: (did: string) => apiFetch(`/v1/mls/key-package/${did}`),
  mlsCreateGroup: (body: { name: string; member_dids: string[] }) =>
    apiFetch("/v1/mls/group", { method: "POST", body: JSON.stringify(body) }),
  mlsGroupInfo: (id: string) => apiFetch(`/v1/mls/group/${id}`),
  mlsAddMember: (groupId: string, body: { member_did: string }) =>
    apiFetch(`/v1/mls/group/${groupId}/add`, { method: "POST", body: JSON.stringify(body) }),
  mlsSendGroupMessage: (groupId: string, body: { ciphertext: string }) =>
    apiFetch(`/v1/mls/group/${groupId}/message`, { method: "POST", body: JSON.stringify(body) }),
  mlsGetGroupMessages: (groupId: string) => apiFetch(`/v1/mls/group/${groupId}/messages`),
  mlsGetPendingWelcomes: () => apiFetch("/v1/mls/welcomes"),
  mlsAckWelcome: (body: { group_id: string }) =>
    apiFetch("/v1/mls/welcomes/ack", { method: "POST", body: JSON.stringify(body) }),

  // ── OBS Cüzdan ─────────────────────────────────────────────────────
  walletBalance: () => apiFetch("/v1/wallet/balance"),
  walletTransfer: (body: { to_did: string; amount: string; memo?: string }) =>
    apiFetch("/v1/wallet/transfer", { method: "POST", body: JSON.stringify(body) }),
  walletTransactions: (limit = 50) => apiFetch(`/v1/wallet/transactions?limit=${limit}`),
  walletSupply: () => apiFetch("/v1/wallet/supply"),
  walletShield: (body: { amount: string; commitment: string }) =>
    apiFetch("/v1/wallet/shield", { method: "POST", body: JSON.stringify(body) }),
  walletShieldedTransfer: (body: { proof_json: string; public_inputs: string[]; recipient_commitment: string; amount_commitment: string }) =>
    apiFetch("/v1/wallet/shielded-transfer", { method: "POST", body: JSON.stringify(body) }),
  walletUnshield: (body: { proof_json: string; public_inputs: string[]; nullifier: string; amount: string }) =>
    apiFetch("/v1/wallet/unshield", { method: "POST", body: JSON.stringify(body) }),
  walletShieldedRoot: () => apiFetch("/v1/wallet/shielded/root"),
  walletShieldedNotes: () => apiFetch("/v1/wallet/shielded/notes"),
  walletMerkleProof: (leafIndex: number): Promise<{ root: string; leaf_index: number; path_elements: string[]; path_indices: number[] }> =>
    apiFetch(`/v1/wallet/shielded/proof/${leafIndex}`),

  // ── Staking ────────────────────────────────────────────────────────
  stakingStake: (body: { amount: string }) =>
    apiFetch("/v1/staking/stake", { method: "POST", body: JSON.stringify(body) }),
  stakingUnstake: (body: { amount: string }) =>
    apiFetch("/v1/staking/unstake", { method: "POST", body: JSON.stringify(body) }),
  stakingWithdraw: () =>
    apiFetch("/v1/staking/withdraw", { method: "POST", body: JSON.stringify({}) }),
  stakingPositions: () => apiFetch("/v1/staking/positions"),
  stakingSlashes: () => apiFetch("/v1/staking/slashes"),
  stakingSlashReview: (body: { slash_id: string; accept: boolean }) =>
    apiFetch("/v1/staking/slash/review", { method: "POST", body: JSON.stringify(body) }),

  // ── Airdrop ────────────────────────────────────────────────────────
  airdropListCampaigns: () => apiFetch("/v1/airdrop/campaigns"),
  airdropGetCampaign: (id: string) => apiFetch(`/v1/airdrop/campaigns/${id}`),
  airdropClaim: (id: string, body: { proof_json: string; public_inputs: string[] }) =>
    apiFetch(`/v1/airdrop/campaigns/${id}/claim`, { method: "POST", body: JSON.stringify(body) }),
  airdropCreateCampaign: (body: { title: string; total_amount: string; per_claim: string; min_tier: number; ends_at: string }) =>
    apiFetch("/v1/airdrop/campaigns", { method: "POST", body: JSON.stringify(body) }),
  airdropEndCampaign: (id: string) =>
    apiFetch(`/v1/airdrop/campaigns/${id}/end`, { method: "POST", body: JSON.stringify({}) }),

  // ── Governance ─────────────────────────────────────────────────────
  governanceCreateProposal: (body: { title: string; description: string; proposal_type: string; execution_payload?: string }) =>
    apiFetch("/v1/governance/proposals", { method: "POST", body: JSON.stringify(body) }),
  governanceListProposals: () => apiFetch("/v1/governance/proposals"),
  governanceGetProposal: (id: string) => apiFetch(`/v1/governance/proposals/${id}`),
  governanceVoterRoot: (id: string) => apiFetch(`/v1/governance/proposals/${id}/voter-root`),
  governanceVote: (id: string, body: { proof_json: string; public_inputs: string[]; choice: number }) =>
    apiFetch(`/v1/governance/proposals/${id}/vote`, { method: "POST", body: JSON.stringify(body) }),
  governanceFinalize: (id: string) =>
    apiFetch(`/v1/governance/proposals/${id}/finalize`, { method: "POST", body: JSON.stringify({}) }),
  governanceExecute: (id: string) =>
    apiFetch(`/v1/governance/proposals/${id}/execute`, { method: "POST", body: JSON.stringify({}) }),

  // ── Mini App Store ─────────────────────────────────────────────────
  listApps: () => apiFetch("/v1/apps"),
  getApp: (id: string) => apiFetch(`/v1/apps/${id}`),
  publishApp: (body: { manifest: object; code_url: string }) =>
    apiFetch("/v1/apps", { method: "POST", body: JSON.stringify(body) }),
  installApp: (id: string) =>
    apiFetch(`/v1/apps/${id}/install`, { method: "POST", body: JSON.stringify({}) }),
  uninstallApp: (id: string) =>
    apiFetch(`/v1/apps/${id}/install`, { method: "DELETE" }),
  runApp: async (id: string, payload?: object) => {
    const token = typeof window !== "undefined" ? localStorage.getItem("obscura_token") : null;
    const res = await fetch(`${BASE}/v1/apps/${id}/run`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(payload || {}),
    });
    if (res.status === 501) return { status: "coming_soon", stdout: "" };
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Çalıştırma başarısız");
    return data.data;
  },

  // ── FAZ 3 — Federation (permissionless node registry) ─────────────
  nodeRegister: (body: { node_id: string; peer_addr: string; pubkey: string; version?: string; region?: string; http_url?: string; sig?: string }) =>
    apiFetch("/v1/nodes/register", { method: "POST", body: JSON.stringify(body) }),
  nodeList: () => apiFetch("/v1/nodes"),
  nodeGet: (id: string) => apiFetch(`/v1/nodes/${id}`),
  nodeHeartbeat: (id: string) =>
    apiFetch(`/v1/nodes/${id}/heartbeat`, { method: "POST", body: JSON.stringify({}) }),

  // ── FAZ 3 — Cross-chain Bridge ─────────────────────────────────────
  bridgeStatus: () => apiFetch("/v1/bridge/status"),
  bridgeLock: (body: { from_chain: string; to_chain: string; amount: string; sender_addr: string; recipient_addr: string }) =>
    apiFetch("/v1/bridge/lock", { method: "POST", body: JSON.stringify(body) }),

  // ── FAZ 3 — Post-quantum key preparation ───────────────────────────
  pqKeygen: () => apiFetch("/v1/pq/keygen", { method: "POST", body: JSON.stringify({}) }),

  // ── Identity + BIP39 + Sosyal Kurtarma ───────────────────────────────────
  generateMnemonic: () =>
    apiFetch("/v1/identity/mnemonic/generate", { method: "POST", body: JSON.stringify({}) }),
  validateMnemonic: (mnemonic: string) =>
    apiFetch("/v1/identity/mnemonic/validate", { method: "POST", body: JSON.stringify({ mnemonic }) }),
  deriveIdentity: (mnemonic: string, passphrase = "") =>
    apiFetch("/v1/identity/derive", { method: "POST", body: JSON.stringify({ mnemonic, passphrase }) }),
  shamirSplit: (mnemonic: string, k = 3, n = 5) =>
    apiFetch("/v1/identity/shamir/split", { method: "POST", body: JSON.stringify({ mnemonic, k, n }) }),
  shamirCombine: (shares: Array<{ index: number; value: number[] }>) =>
    apiFetch("/v1/identity/shamir/combine", { method: "POST", body: JSON.stringify({ shares }) }),

  // ── Shard Storage ─────────────────────────────────────────────────────────
  shardStats: () => apiFetch("/v1/storage/stats"),
  shardDelete: (contentId: string) =>
    apiFetch(`/v1/storage/shard/${encodeURIComponent(contentId)}`, { method: "DELETE" }),

  // ── FAZ 4 — DAO (tam yönetim) ──────────────────────────────────────
  daoCreateProposal: (body: { title: string; description: string; category?: string }) =>
    apiFetch("/v1/dao/proposals", { method: "POST", body: JSON.stringify(body) }),
  daoListProposals: (status?: string) =>
    apiFetch(`/v1/dao/proposals${status ? `?status=${status}` : ""}`),
  daoFinalize: (id: string) =>
    apiFetch(`/v1/dao/proposals/${id}/finalize`, { method: "POST", body: JSON.stringify({}) }),
  daoExecute: (id: string) =>
    apiFetch(`/v1/dao/proposals/${id}/execute`, { method: "POST", body: JSON.stringify({}) }),
  daoVeto: (id: string, reason: string) =>
    apiFetch(`/v1/dao/proposals/${id}/veto`, { method: "POST", body: JSON.stringify({ reason }) }),

  // ── FAZ 4 — Post-quantum (Dilithium3) ──────────────────────────────
  dilithiumKeygen: () => apiFetch("/v1/pq/dilithium/keygen", { method: "POST", body: JSON.stringify({}) }),

  // ── FAZ 4 — AI optimizer ───────────────────────────────────────────
  aiMetrics: () => apiFetch("/v1/ai/metrics"),
  aiPeers: (n = 5) => apiFetch(`/v1/ai/peers?n=${n}`),

  // ── FAZ 4 — Sequencer ──────────────────────────────────────────────
  sequencerRegister: (body: { node_id: string; stake: number; peer_addr?: string }) =>
    apiFetch("/v1/sequencer/register", { method: "POST", body: JSON.stringify(body) }),
  sequencerList: () => apiFetch("/v1/sequencer/candidates"),
  sequencerBatches: () => apiFetch("/v1/sequencer/batches"),
  sequencerSubmitBatch: (body: { state_root: string; tx_count: number }) =>
    apiFetch("/v1/sequencer/batch", { method: "POST", body: JSON.stringify(body) }),

  // ── FAZ 4 — GPS + ZK location proof ───────────────────────────────
  locationVerify: (body: { proof_json: string; public_inputs: string[] }) =>
    apiFetch("/v1/location/verify", { method: "POST", body: JSON.stringify(body) }),

  // ── Fiziksel Etkinlik Entegrasyonu (Spec Bölüm 11) ────────────────
  createEvent: (body: { title: string; description?: string; location?: string; lat?: number; lon?: number; starts_at: string; ends_at: string; capacity?: number; min_credit_tier?: number }) =>
    apiFetch("/v1/events", { method: "POST", body: JSON.stringify(body) }),
  listEvents: () => apiFetch("/v1/events"),
  nearbyEvents: (lat: number, lon: number) => apiFetch(`/v1/events/nearby?lat=${lat}&lon=${lon}`),
  getEvent: (id: string) => apiFetch(`/v1/events/${id}`),
  joinEvent: (id: string, body?: { proof_json?: string; public_inputs?: string[] }) =>
    apiFetch(`/v1/events/${id}/join`, { method: "POST", body: JSON.stringify(body || {}) }),
  checkIn: (id: string, body: { token: string; lat?: number; lon?: number; proof_json?: string; public_inputs?: string[] }) =>
    apiFetch(`/v1/events/${id}/checkin`, { method: "POST", body: JSON.stringify(body) }),
  listAttendees: (id: string) => apiFetch(`/v1/events/${id}/attendees`),
  getCheckinQR: (id: string) => apiFetch(`/v1/events/${id}/qr`),
  leaveEvent: (id: string) => apiFetch(`/v1/events/${id}/join`, { method: "DELETE" }),

  // ── Bot Ekosistemi ─────────────────────────────────────────────────
  listBots: () => apiFetch("/v1/bots"),

  // ── Post-quantum Dilithium3 ──────────────────────────────────────
  dilithiumSetup: () =>
    apiFetch("/v1/pq/dilithium/keygen", { method: "POST", body: JSON.stringify({}) }),
  dilithiumVerify: (body: { public_key: string; message: string; signature: string }) =>
    apiFetch("/v1/pq/dilithium/verify", { method: "POST", body: JSON.stringify(body) }),

  // ── Admin — İlke 5 inceleme kuyruğu ───────────────────────────────
  adminListReviewQueue: (params?: { status?: string; source?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams();
    if (params?.status) q.set("status", params.status);
    if (params?.source) q.set("source", params.source);
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.offset) q.set("offset", String(params.offset));
    const qs = q.toString();
    return apiFetch(`/v1/admin/review-queue${qs ? `?${qs}` : ""}`);
  },
  adminResolveReviewQueue: (id: string, action: "dismiss" | "confirm_remove" | "confirm_warn", note?: string) =>
    apiFetch(`/v1/admin/review-queue/${id}/resolve`, {
      method: "POST",
      body: JSON.stringify({ action, note: note || "" }),
    }),
};

export interface MiniAppManifest {
  name: string;
  description?: string;
  author?: string;
  version?: string;
  icon?: string;
  permissions?: string[];
  zkPermissions?: string[];
  allowedDomains?: string[];
  maxMemory?: number;
  maxCpu?: number;
  minTier?: number;
}

export interface MiniApp {
  id: string;
  manifest: MiniAppManifest;
  code_url: string;
  publisher_did?: string;
  install_count?: number;
  installed?: boolean;
  featured?: boolean;
  created_at?: string;
}


export function createWS(token: string, onMsg: (msg: any) => void) {
  const wsUrl = (process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080") + `/v1/stream?token=${token}`;
  const ws = new WebSocket(wsUrl);
  ws.onmessage = (e) => { try { onMsg(JSON.parse(e.data)); } catch {} };
  ws.onclose = () => setTimeout(() => createWS(token, onMsg), 3000);
  return ws;
}
