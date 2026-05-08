const BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

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
  if (!data.success) throw new Error(data.error || "Bir hata oluştu");
  return data.data;
}

export const api = {
  requestOTP: (phone: string) =>
    apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) }),

  verifyOTP: (body: object) =>
    apiFetch("/v1/auth/verify-otp", { method: "POST", body: JSON.stringify(body) }),

  getMe: () => apiFetch("/v1/users/me"),
  getUser: (did: string) => apiFetch(`/v1/users/${did}`),
  searchUsers: (q: string) => apiFetch(`/v1/users/search?q=${q}`),

  getConversations: () => apiFetch("/v1/conversations"),
  getMessages: (convId: string) => apiFetch(`/v1/conversations/${convId}/messages`),
  sendMessage: (body: object) =>
    apiFetch("/v1/messages", { method: "POST", body: JSON.stringify(body) }),

  getCreditScore: () => apiFetch("/v1/credit/score"),
  reportSpam: (body: object) =>
    apiFetch("/v1/spam/report", { method: "POST", body: JSON.stringify(body) }),

  nodeStatus: () => apiFetch("/v1/node/status"),

  // E2EE prekey endpoints
  uploadPrekeys: (body: object) =>
    apiFetch("/v1/keys/upload", { method: "POST", body: JSON.stringify(body) }),
  getPreKeyBundle: (did: string) =>
    apiFetch(`/v1/keys/${did}`),

  // Profil güncelleme
  updateMe: (body: { display_name?: string; username?: string; avatar_url?: string }) =>
    apiFetch("/v1/users/me", { method: "PATCH", body: JSON.stringify(body) }),

  // Konuşma oluşturma
  createConversation: (body: { peer_did?: string; is_group?: boolean; name?: string; members?: string[] }) =>
    apiFetch("/v1/conversations", { method: "POST", body: JSON.stringify(body) }),

  // Mesaj silme
  deleteMessage: (id: string) =>
    apiFetch(`/v1/messages/${id}`, { method: "DELETE" }),

  // Kredi geçmişi
  getCreditHistory: (limit = 50) =>
    apiFetch(`/v1/credit/history?limit=${limit}`),

  // Medya yükleme
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

  // Push token kaydı
  registerDevice: (platform: "fcm" | "apns", token: string) =>
    apiFetch("/v1/devices/register", { method: "POST", body: JSON.stringify({ platform, token }) }),

  // ZK kanıt doğrulama
  verifyZKProof: (body: { proof_json: string; circuit_id: string; public_inputs: string[] }) =>
    apiFetch("/v1/zk/verify", { method: "POST", body: JSON.stringify(body) }),

  // TURN credentials
  getTurnCredentials: () => apiFetch("/v1/rtc/turn-credentials"),

  // Node durumu
  nodeStatus: () => apiFetch("/v1/node/status"),
};

export function createWS(token: string, onMsg: (msg: any) => void) {
  const wsUrl = (process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080") + `/v1/stream?token=${token}`;
  const ws = new WebSocket(wsUrl);
  ws.onmessage = (e) => { try { onMsg(JSON.parse(e.data)); } catch {} };
  ws.onclose = () => setTimeout(() => createWS(token, onMsg), 3000);
  return ws;
}
