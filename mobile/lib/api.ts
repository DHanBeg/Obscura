import * as SecureStore from "expo-secure-store";
import Constants from "expo-constants";

// OBSCURA_API_BASE/OBSCURA_WS_BASE: sadece Node test/proof script'lerinin
// gerçek local backend'e (go run) karşı sendSealedMessage/message-send.ts'i
// çağırabilmesi için opsiyonel override (Madde 15, Adım 11a.5). RN
// runtime'ında process.env bu isimle asla set edilmez (Metro sadece
// EXPO_PUBLIC_ önekli değişkenleri inline eder) — üretim davranışı DEĞİŞMEDİ.
const BASE = (typeof process !== "undefined" && process.env?.OBSCURA_API_BASE) || "https://obscura-backend-production-1827.up.railway.app";
const WS_BASE = (typeof process !== "undefined" && process.env?.OBSCURA_WS_BASE) || "wss://obscura-backend-production-1827.up.railway.app";

export { WS_BASE };

async function getToken(): Promise<string | null> {
  return SecureStore.getItemAsync("obscura_token");
}

function xhrFetch(url: string, method: string, headers: Record<string, string>, body?: string): Promise<any> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(method, url);
    xhr.timeout = 30000;
    Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));
    xhr.onload = () => {
      try { resolve(JSON.parse(xhr.responseText)); }
      catch (e) { reject(new Error(`JSON parse error: ${xhr.responseText?.slice(0, 100)}`)); }
    };
    xhr.onerror = () => reject(new TypeError(`XHR network error: ${xhr.status}`));
    xhr.ontimeout = () => reject(new TypeError(`XHR timeout after 30s`));
    xhr.send(body);
  });
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<any> {
  const token = await getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "Connection": "close",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...((opts.headers as Record<string, string>) || {}),
  };
  const data = await xhrFetch(`${BASE}${path}`, (opts.method as string) || "GET", headers, opts.body as string | undefined);
  if (!data.success) throw new Error(data.error || "Bir hata oluştu");
  return data.data;
}

export const api = {
  requestOTP: (phone: string) =>
    apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) }),
  verifyOTP: (body: object) =>
    apiFetch("/v1/auth/verify-otp", { method: "POST", body: JSON.stringify(body) }),
  getMe: () => apiFetch("/v1/users/me"),
  searchUsers: (q: string) => apiFetch(`/v1/users/search?q=${encodeURIComponent(q)}`),
  getConversations: () => apiFetch("/v1/conversations"),
  getMessages: (convId: string) => apiFetch(`/v1/conversations/${convId}/messages`),
  sendMessage: (body: object) =>
    apiFetch("/v1/messages", { method: "POST", body: JSON.stringify(body) }),
  getPreKeyBundle: (did: string) => apiFetch(`/v1/keys/${did}`),
  uploadPrekeys: (body: object) =>
    apiFetch("/v1/keys/upload", { method: "POST", body: JSON.stringify(body) }),
  getOPKCount: () => apiFetch("/v1/keys/opk/count"),
  replenishOPK: (body: object) =>
    apiFetch("/v1/keys/opk/replenish", { method: "POST", body: JSON.stringify(body) }),
  getTurnCredentials: () => apiFetch("/v1/rtc/turn-credentials"),
  callInvite: (body: { to_did: string; sdp_offer: string; ice_candidates?: string[]; call_type?: string }) =>
    apiFetch("/v1/call/invite", { method: "POST", body: JSON.stringify(body) }),
  callAnswer: (body: { call_id: string; sdp_answer: string; ice_candidates?: string[]; accepted: boolean }) =>
    apiFetch("/v1/call/answer", { method: "POST", body: JSON.stringify(body) }),
  callEnd: (body: { call_id: string; reason?: string }) =>
    apiFetch("/v1/call/end", { method: "POST", body: JSON.stringify(body) }),

  // FAZ 3 — yeni endpoint'ler
  updateMe: (body: { display_name?: string; username?: string; avatar_url?: string; bio?: string; hide_online?: number; phone_visible?: number }) =>
    apiFetch("/v1/users/me", { method: "PATCH", body: JSON.stringify(body) }),
  createConversation: (body: object) =>
    apiFetch("/v1/conversations", { method: "POST", body: JSON.stringify(body) }),
  deleteMessage: (id: string) =>
    apiFetch(`/v1/messages/${id}`, { method: "DELETE" }),
  getCreditHistory: (limit = 50) =>
    apiFetch(`/v1/credit/history?limit=${limit}`),
  getCreditScore: () => apiFetch("/v1/credit/score"),
  reportSpam: (body: object) =>
    apiFetch("/v1/spam/report", { method: "POST", body: JSON.stringify(body) }),
  getUser: (did: string) => apiFetch(`/v1/users/${did}`),
  getUserByOdi: (odi: string) => apiFetch(`/v1/users/by-odi/${odi}`),

  // ── Kişi Rehberi ──────────────────────────────────────────────────────
  getContacts: () => apiFetch("/v1/contacts"),
  addContact: (did: string, nickname?: string) =>
    apiFetch("/v1/contacts", { method: "POST", body: JSON.stringify({ did, nickname: nickname ?? "" }) }),
  removeContact: (did: string) =>
    apiFetch(`/v1/contacts/${did}`, { method: "DELETE" }),
  updateContactTrust: (did: string, isTrusted: boolean) =>
    apiFetch(`/v1/contacts/${did}`, { method: "PATCH", body: JSON.stringify({ is_trusted: isTrusted }) }),

  registerDevice: (platform: string, token: string) =>
    apiFetch("/v1/devices/register", { method: "POST", body: JSON.stringify({ platform, token }) }),

  // ── OBS Cüzdan ─────────────────────────────────────────────────────
  walletBalance: () => apiFetch("/v1/wallet/balance"),
  walletTransfer: (body: { to_did: string; amount: string; memo?: string }) =>
    apiFetch("/v1/wallet/transfer", { method: "POST", body: JSON.stringify(body) }),
  walletTransactions: (limit = 20) => apiFetch(`/v1/wallet/transactions?limit=${limit}`),
  walletSupply: () => apiFetch("/v1/wallet/supply"),
  walletShield: (body: { amount: string; commitment: string }) =>
    apiFetch("/v1/wallet/shield", { method: "POST", body: JSON.stringify(body) }),
  walletShieldedTransfer: (body: { proof_json: string; public_inputs: string[] }) =>
    apiFetch("/v1/wallet/shielded-transfer", { method: "POST", body: JSON.stringify(body) }),
  walletUnshield: (body: { proof_json: string; public_inputs: string[]; recipient_did: string; amount: string }) =>
    apiFetch("/v1/wallet/unshield", { method: "POST", body: JSON.stringify(body) }),
  walletShieldedRoot: () => apiFetch("/v1/wallet/shielded/root"),
  walletShieldedNotes: () => apiFetch("/v1/wallet/shielded/notes"),
  walletMerkleProof: (leafIndex: number) => apiFetch(`/v1/wallet/shielded/proof/${leafIndex}`),

  // ── Staking ────────────────────────────────────────────────────────
  stakingStake: (body: { amount: string }) =>
    apiFetch("/v1/staking/stake", { method: "POST", body: JSON.stringify(body) }),
  stakingUnstake: (body: { amount: string }) =>
    apiFetch("/v1/staking/unstake", { method: "POST", body: JSON.stringify(body) }),
  stakingWithdraw: () =>
    apiFetch("/v1/staking/withdraw", { method: "POST", body: JSON.stringify({}) }),
  stakingPositions: () => apiFetch("/v1/staking/positions"),
  stakingSlashes: () => apiFetch("/v1/staking/slashes"),

  // ── Governance ─────────────────────────────────────────────────────
  governanceListProposals: () => apiFetch("/v1/governance/proposals"),
  governanceGetProposal: (id: string) => apiFetch(`/v1/governance/proposals/${id}`),
  governanceVote: (id: string, body: { proof_json: string; public_inputs: string[]; choice: number }) =>
    apiFetch(`/v1/governance/proposals/${id}/vote`, { method: "POST", body: JSON.stringify(body) }),
  governanceVoterRoot: (id: string) => apiFetch(`/v1/governance/proposals/${id}/voter-root`),
  governanceFinalize: (id: string) =>
    apiFetch(`/v1/governance/proposals/${id}/finalize`, { method: "POST", body: JSON.stringify({}) }),
  governanceExecute: (id: string) =>
    apiFetch(`/v1/governance/proposals/${id}/execute`, { method: "POST", body: JSON.stringify({}) }),

  // ── Airdrop ────────────────────────────────────────────────────────
  airdropListCampaigns: () => apiFetch("/v1/airdrop/campaigns"),
  airdropClaim: (id: string, body: { proof_json: string; public_inputs: string[] }) =>
    apiFetch(`/v1/airdrop/campaigns/${id}/claim`, { method: "POST", body: JSON.stringify(body) }),

  // ── Mini Apps ──────────────────────────────────────────────────────
  listApps: () => apiFetch("/v1/apps"),
  getApp: (id: string) => apiFetch(`/v1/apps/${id}`),
  publishApp: (body: { manifest: object; code_url: string }) =>
    apiFetch("/v1/apps", { method: "POST", body: JSON.stringify(body) }),
  installApp: (id: string) =>
    apiFetch(`/v1/apps/${id}/install`, { method: "POST", body: JSON.stringify({}) }),
  uninstallApp: (id: string) =>
    apiFetch(`/v1/apps/${id}/uninstall`, { method: "DELETE" }),

  // ── FAZ 3 — Federation ────────────────────────────────────────────────
  nodeList: () => apiFetch("/v1/nodes"),
  nodeGet: (id: string) => apiFetch(`/v1/nodes/${id}`),
  nodeHeartbeat: (id: string) =>
    apiFetch(`/v1/nodes/${id}/heartbeat`, { method: "POST", body: JSON.stringify({}) }),
  bridgeStatus: () => apiFetch("/v1/bridge/status"),
  bridgeLock: (body: { from_chain: string; to_chain: string; amount: string; sender_addr: string; recipient_addr: string }) =>
    apiFetch("/v1/bridge/lock", { method: "POST", body: JSON.stringify(body) }),
  pqKeygen: () => apiFetch("/v1/pq/keygen", { method: "POST", body: JSON.stringify({}) }),

  // ── Identity + Sosyal Kurtarma ───────────────────────────────────
  shamirSplit: (mnemonic: string, k = 3, n = 5) =>
    apiFetch("/v1/identity/shamir/split", { method: "POST", body: JSON.stringify({ mnemonic, k, n }) }),
  shamirCombine: (shares: Array<{ index: number; value: number[] }>) =>
    apiFetch("/v1/identity/shamir/combine", { method: "POST", body: JSON.stringify({ shares }) }),

  // ── FAZ 4 — DAO ───────────────────────────────────────────────────────
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

  // ── FAZ 4 — Post-quantum ──────────────────────────────────────────────
  dilithiumKeygen: () =>
    apiFetch("/v1/pq/dilithium/keygen", { method: "POST", body: JSON.stringify({}) }),

  // ── FAZ 4 — AI optimizer ─────────────────────────────────────────────
  aiMetrics: () => apiFetch("/v1/ai/metrics"),
  aiPeers: (n = 5) => apiFetch(`/v1/ai/peers?n=${n}`),

  // ── FAZ 4 — Sequencer ─────────────────────────────────────────────────
  sequencerList: () => apiFetch("/v1/sequencer/candidates"),
  sequencerBatches: () => apiFetch("/v1/sequencer/batches"),
  sequencerRegister: (body: { node_id: string; stake: number; peer_addr?: string }) =>
    apiFetch("/v1/sequencer/register", { method: "POST", body: JSON.stringify(body) }),

  // ── Fiziksel Etkinlik Entegrasyonu (Spec Bölüm 11) ────────────────────────
  createEvent: (body: { title: string; description?: string; location?: string; lat?: number; lon?: number; starts_at: string; ends_at: string; capacity?: number; min_credit_tier?: number }) =>
    apiFetch("/v1/events", { method: "POST", body: JSON.stringify(body) }),
  listEvents: () => apiFetch("/v1/events"),
  nearbyEvents: (lat: number, lon: number) => apiFetch(`/v1/events/nearby?lat=${lat}&lon=${lon}`),
  getEvent: (id: string) => apiFetch(`/v1/events/${id}`),
  joinEvent: (id: string) =>
    apiFetch(`/v1/events/${id}/join`, { method: "POST", body: JSON.stringify({}) }),
  checkIn: (id: string, body: { token: string; lat?: number; lon?: number }) =>
    apiFetch(`/v1/events/${id}/checkin`, { method: "POST", body: JSON.stringify(body) }),
  listAttendees: (id: string) => apiFetch(`/v1/events/${id}/attendees`),
  getCheckinQR: (id: string) => apiFetch(`/v1/events/${id}/qr`),
  leaveEvent: (id: string) => apiFetch(`/v1/events/${id}/join`, { method: "DELETE" }),

  // ── NFC ───────────────────────────────────────────────────────────────────
  nfcCheckin: (eventId: string) =>
    apiFetch(`/v1/events/${eventId}/checkin`, { method: "POST", body: JSON.stringify({}) }),

  // ── Konum / ZK Location Proof (raw — sadece dev ortamında)
  locationVerifyRaw: (lat: number, lon: number) => {
    if (!__DEV__) throw new Error("Raw location only in dev mode");
    return apiFetch("/v1/location/verify", {
      method: "POST",
      body: JSON.stringify({
        lat_raw: Math.round(lat * 1_000_000),
        lon_raw: Math.round(lon * 1_000_000),
        proof: "pending",
      }),
    });
  },

  // Medya yükleme (multipart)
  uploadMedia: async (file: { uri: string; name: string; type: string }, mediaType = "media"): Promise<{ url: string; key: string }> => {
    const token = await getToken();
    const form = new FormData();
    form.append("file", { uri: file.uri, name: file.name, type: file.type } as any);
    form.append("type", mediaType);
    const res = await fetch(`${BASE}/v1/media/upload`, {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    });
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Yükleme başarısız");
    return data.data;
  },

  // ── Post-quantum Dilithium3 ────────────────────────────────────────────────
  dilithiumSetup: () =>
    apiFetch("/v1/pq/dilithium/keygen", { method: "POST", body: JSON.stringify({}) }),
  dilithiumVerify: (body: { public_key: string; message: string; signature: string }) =>
    apiFetch("/v1/pq/dilithium/verify", { method: "POST", body: JSON.stringify(body) }),

  // ── GPS + ZK Konum Kanıtı — ZK proof ile (locationVerify üzerine yazar)
  locationVerify: (body: { proof_json: string; public_inputs: string[] }) =>
    apiFetch("/v1/location/verify", { method: "POST", body: JSON.stringify(body) }),

  // ── Kimlik / ZK / Audit ──────────────────────────────────────────────────
  nodeStatus: () => apiFetch("/v1/node/status"),
  zkAuditLog: () => apiFetch("/v1/zk/audit"),

  // ── Davet Linki ──────────────────────────────────────────────────────────────
  createConvInvite: async (
    convId: string,
    opts: { slug?: string; max_uses?: number; max_members?: number; expires_hours?: number },
  ): Promise<{ token: string; slug?: string; invite_url: string }> => {
    const token = await getToken();
    const res = await fetch(`${BASE}/v1/conversations/${convId}/invite/create`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify(opts),
    });
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Hata");
    return data.data;
  },
  joinViaInvite: async (identifier: string): Promise<{ conv_id: string; conv_name: string }> => {
    const token = await getToken();
    const res = await fetch(`${BASE}/v1/conversations/join`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ identifier }),
    });
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Katılım başarısız");
    return data.data;
  },

  // ── Bot Ekosistemi ────────────────────────────────────────────────────────
  createBot: (body: { name: string; description: string; webhook_url: string }) =>
    apiFetch("/v1/bots", { method: "POST", body: JSON.stringify(body) }),
  listMyBots: () => apiFetch("/v1/bots/my"),
  listPublicBots: () => apiFetch("/v1/bots"),
  deleteBot: (id: string) => apiFetch(`/v1/bots/${id}`, { method: "DELETE" }),
  regenerateBotToken: (id: string) =>
    apiFetch(`/v1/bots/${id}/token`, { method: "POST" }),

  // ── Keşfet — public gruplar/kanallar/topluluklar ─────────────────────────
  discoverConversations: (q = ""): Promise<Array<{ id: string; name: string; conv_type: string; member_count: number }>> =>
    apiFetch(`/v1/conversations/discover?q=${encodeURIComponent(q)}`),
  joinConversation: (convId: string): Promise<void> =>
    apiFetch(`/v1/conversations/${convId}/members`, {
      method: "POST",
      body: JSON.stringify({ conv_id: convId }),
    }),

  // ── Grup/Kanal/Topluluk Profil-Yönetimi ──────────────────────────────────
  getConversation: (convId: string): Promise<{
    id: string; name: string; conv_type: string; description: string;
    avatar_url: string; is_group: boolean; is_public: boolean;
    member_count: number; my_role: "admin" | "member"; created_at: string;
  }> => apiFetch(`/v1/conversations/${convId}`),
  getConversationMembers: (convId: string): Promise<Array<{
    did: string; odi?: string; display_name: string; avatar_url: string;
    tier: number; role: "admin" | "member"; joined_at: string;
  }>> => apiFetch(`/v1/conversations/${convId}/members`),
  updateConversation: (convId: string, body: { name?: string; description?: string; avatar_url?: string; is_public?: boolean }) =>
    apiFetch(`/v1/conversations/${convId}`, { method: "PATCH", body: JSON.stringify(body) }),
  leaveConversation: (convId: string) =>
    apiFetch(`/v1/conversations/${convId}/leave`, { method: "POST" }),
  removeConvMember: (convId: string, did: string) =>
    apiFetch(`/v1/conversations/${convId}/members/${did}`, { method: "DELETE" }),
  updateMemberRole: (convId: string, did: string, role: "admin" | "member") =>
    apiFetch(`/v1/conversations/${convId}/members/${did}`, { method: "PATCH", body: JSON.stringify({ role }) }),

  // ── OBS Topluluk (Governance Community Chat) ─────────────────────────────
  joinOBSCommunity: async (): Promise<{ conv_id: string }> => {
    const token = await getToken();
    const res = await fetch(`${BASE}/v1/governance/community/join`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token ?? ""}` },
    });
    const data = await res.json();
    if (!data.success) throw new Error(data.error || "Katılım başarısız");
    return data.data;
  },

  // ── Admin — İlke 5 inceleme kuyruğu (backend fail-closed korur, bkz.
  // AdminMiddleware/OBSCURA_ADMIN_DIDS) ─────────────────────────────────────
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

export async function deriveIdentity(mnemonic: string): Promise<{ identity_secret_hex: string }> {
  const { mnemonicToSeedSync, validateMnemonic } = await import("@scure/bip39");
  const { wordlist } = await import("@scure/bip39/wordlists/english");
  if (!validateMnemonic(mnemonic.trim(), wordlist)) {
    throw new Error("Geçersiz anımsatıcı ifade (mnemonic)");
  }
  const seed = mnemonicToSeedSync(mnemonic.trim());
  const identity_secret_hex = Array.from(seed.slice(0, 32))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return { identity_secret_hex };
}

export function createWS(
  token: string,
  onMsg: (msg: any) => void,
  onOpen?: () => void,
  onClose?: () => void,
): WebSocket {
  // Auth regresyonu fix'i (1873709, 2026-06-23 — 2 aydır kırıktı): token'ı
  // handshake'ten SONRA bir WS mesajı olarak göndermek backend'de hiç
  // OKUNMUYORDU (internal/messaging/hub.go ReadPump → HandleMessage'da
  // "auth" case'i yok) — her bağlantı 401'e düşüp CLOSE 1006 alıyor,
  // onopen HİÇ ateşlenmiyor, wsConnected asla true olmuyordu (1:1/grup/
  // call/presence real-time push'un TAMAMI ölüydü). Backend zaten
  // Authorization header'ı destekliyor (cmd/node/main.go:569-572,
  // query param'dan SONRA denenen fallback) — query param'ı DEĞİL header'ı
  // seçtik: nginx $request log format'ı (nginx/nginx.conf:17) query
  // string'i plaintext JWT olarak access.log'a yazar, header'ları yazmaz.
  // RN'nin WebSocket'i (Libraries/WebSocket/WebSocket.js:98-148) bu üçüncü
  // {headers} argümanını native seviyede destekliyor — JS-only hack değil.
  // lib.dom.d.ts'in WebSocket ctor tipi (url, protocols?) — RN'nin native
  // üçüncü {headers} argümanını (yukarıdaki yorum) modellemiyor, bu yüzden
  // ctor referansı any'e düşürülüyor (tip güvenliği burada geçerli değil,
  // RN runtime'ının gerçek imzası TS lib'inden geniş).
  const WSCtor = WebSocket as any;
  const ws: WebSocket = new WSCtor(`${WS_BASE}/v1/stream`, undefined, {
    headers: { Authorization: `Bearer ${token}` },
  });
  ws.onopen = () => { onOpen?.(); };
  ws.onmessage = (e) => { try { onMsg(JSON.parse(e.data)); } catch {} };
  ws.onclose = () => {
    onClose?.();
    setTimeout(() => createWS(token, onMsg, onOpen, onClose), 3000);
  };
  return ws;
}
