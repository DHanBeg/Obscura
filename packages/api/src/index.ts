/**
 * @obscura/api — Paylaşımlı API katmanı
 * Web, Mobil ve Desktop için ortak HTTP + WebSocket istemcisi
 */

export interface ApiConfig {
  baseUrl: string;
  wsUrl: string;
  getToken: () => string | null;
  onUnauthorized?: () => void;
}

let _config: ApiConfig = {
  baseUrl: "http://localhost:8080",
  wsUrl: "ws://localhost:8080",
  getToken: () => null,
};

export function configureApi(config: Partial<ApiConfig>) {
  _config = { ..._config, ...config };
}

// ── Core fetch ──────────────────────────────────────────────────────────────

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<any> {
  const token = _config.getToken();
  const res = await fetch(`${_config.baseUrl}${path}`, {
    ...opts,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(opts.headers || {}),
    },
  });
  const data = await res.json();
  if (!data.success) {
    if (res.status === 401) _config.onUnauthorized?.();
    throw new Error(data.error || "Bir hata oluştu");
  }
  return data.data;
}

// ── Auth ────────────────────────────────────────────────────────────────────

export const authApi = {
  requestOTP: (phone: string) =>
    apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) }),

  verifyOTP: (body: {
    phone: string;
    otp: string;
    username?: string;
    identity_key: string;
  }) => apiFetch("/v1/auth/verify-otp", { method: "POST", body: JSON.stringify(body) }),
};

// ── Users ───────────────────────────────────────────────────────────────────

export const usersApi = {
  getMe: () => apiFetch("/v1/users/me"),
  getUser: (did: string) => apiFetch(`/v1/users/${did}`),
  searchUsers: (q: string) => apiFetch(`/v1/users/search?q=${encodeURIComponent(q)}`),
  updateProfile: (body: { display_name?: string; avatar_url?: string }) =>
    apiFetch("/v1/users/me", { method: "PATCH", body: JSON.stringify(body) }),
};

// ── Conversations ───────────────────────────────────────────────────────────

export const conversationsApi = {
  getAll: () => apiFetch("/v1/conversations"),
  getMessages: (convId: string, limit = 50, before?: string) =>
    apiFetch(`/v1/conversations/${convId}/messages?limit=${limit}${before ? `&before=${before}` : ""}`),
  markRead: (convId: string) =>
    apiFetch(`/v1/conversations/${convId}/read`, { method: "POST" }),
};

// ── Messages ────────────────────────────────────────────────────────────────

export const messagesApi = {
  send: (body: { to_id: string; ciphertext: string; type: string; reply_to_id?: string }) =>
    apiFetch("/v1/messages", { method: "POST", body: JSON.stringify(body) }),
  delete: (msgId: string) =>
    apiFetch(`/v1/messages/${msgId}`, { method: "DELETE" }),
};

// ── Keys (E2EE) ─────────────────────────────────────────────────────────────

export const keysApi = {
  upload: (bundle: object) =>
    apiFetch("/v1/keys/upload", { method: "POST", body: JSON.stringify(bundle) }),
  getBundle: (did: string) =>
    apiFetch(`/v1/keys/bundle/${did}`),
};

// ── Calls / WebRTC ──────────────────────────────────────────────────────────

export const callsApi = {
  getTurnCredentials: () => apiFetch("/v1/webrtc/turn-credentials"),
  initiateCall: (body: { to_did: string; type: "audio" | "video" }) =>
    apiFetch("/v1/calls/initiate", { method: "POST", body: JSON.stringify(body) }),
};

// ── Credit / Spam ───────────────────────────────────────────────────────────

export const creditApi = {
  getScore: () => apiFetch("/v1/credit/score"),
  reportSpam: (body: { reported_did: string; reason: string }) =>
    apiFetch("/v1/spam/report", { method: "POST", body: JSON.stringify(body) }),
};

// ── Node ────────────────────────────────────────────────────────────────────

export const nodeApi = {
  status: () => apiFetch("/v1/node/status"),
};

// ── Unified API object ──────────────────────────────────────────────────────

export const api = {
  ...authApi,
  ...usersApi,
  getConversations: conversationsApi.getAll,
  getMessages: conversationsApi.getMessages,
  markRead: conversationsApi.markRead,
  sendMessage: messagesApi.send,
  deleteMessage: messagesApi.delete,
  uploadPrekeys: keysApi.upload,
  getPreKeyBundle: keysApi.getBundle,
  getTurnCredentials: callsApi.getTurnCredentials,
  getCreditScore: creditApi.getScore,
  reportSpam: creditApi.reportSpam,
  nodeStatus: nodeApi.status,
};

// ── WebSocket ───────────────────────────────────────────────────────────────

export type WSMessage =
  | { type: "new_message"; data: any }
  | { type: "message_delivered"; data: { msg_id: string } }
  | { type: "message_read"; data: { msg_id: string } }
  | { type: "user_online"; data: { did: string } }
  | { type: "user_offline"; data: { did: string } }
  | { type: "call_incoming"; data: any }
  | { type: "call_ended"; data: any }
  | { type: "typing"; data: { conv_id: string; did: string } };

export function createWebSocket(
  token: string,
  onMessage: (msg: WSMessage) => void,
  onConnect?: () => void,
  onDisconnect?: () => void
): WebSocket {
  const url = `${_config.wsUrl}/v1/stream?token=${token}`;
  const ws = new WebSocket(url);

  ws.onopen = () => onConnect?.();
  ws.onclose = () => {
    onDisconnect?.();
    // Otomatik yeniden bağlan
    setTimeout(() => createWebSocket(token, onMessage, onConnect, onDisconnect), 3000);
  };
  ws.onerror = () => {};
  ws.onmessage = (e) => {
    try { onMessage(JSON.parse(e.data)); } catch {}
  };

  return ws;
}

// ── Types ────────────────────────────────────────────────────────────────────

export interface User {
  id: string;
  did: string;
  username: string;
  display_name: string;
  phone?: string;
  avatar_url: string;
  tier: number;
  credit_score: number;
  identity_key?: string;
}

export interface Message {
  id: string;
  conv_id: string;
  from_did: string;
  to_did: string;
  type: string;
  ciphertext: string;
  status: "pending" | "sent" | "delivered" | "read" | "failed";
  sent_at: string;
  delivered_at?: string;
  read_at?: string;
  media_url?: string;
  reply_to_id?: string;
}

export interface Conversation {
  id: string;
  is_group: boolean;
  name: string;
  avatar_url: string;
  last_msg_text: string;
  last_msg_at?: string;
  unread_count: number;
  peer_did?: string;
  peer_name?: string;
  peer_tier?: number;
}
