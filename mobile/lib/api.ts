import * as SecureStore from "expo-secure-store";
import Constants from "expo-constants";

const BASE = Constants.expoConfig?.extra?.apiUrl ?? "http://localhost:8080";
const WS_BASE = Constants.expoConfig?.extra?.wsUrl ?? "ws://localhost:8080";

export { WS_BASE };

async function getToken(): Promise<string | null> {
  return SecureStore.getItemAsync("obscura_token");
}

export async function apiFetch(path: string, opts: RequestInit = {}): Promise<any> {
  const token = await getToken();
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
  searchUsers: (q: string) => apiFetch(`/v1/users/search?q=${encodeURIComponent(q)}`),
  getConversations: () => apiFetch("/v1/conversations"),
  getMessages: (convId: string) => apiFetch(`/v1/conversations/${convId}/messages`),
  sendMessage: (body: object) =>
    apiFetch("/v1/messages", { method: "POST", body: JSON.stringify(body) }),
  getPreKeyBundle: (did: string) => apiFetch(`/v1/keys/${did}`),
  uploadPrekeys: (body: object) =>
    apiFetch("/v1/keys/upload", { method: "POST", body: JSON.stringify(body) }),
  getTurnCredentials: () => apiFetch("/v1/rtc/turn-credentials"),

  // FAZ 3 — yeni endpoint'ler
  updateMe: (body: { display_name?: string; username?: string; avatar_url?: string }) =>
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
  registerDevice: (platform: string, token: string) =>
    apiFetch("/v1/devices/register", { method: "POST", body: JSON.stringify({ platform, token }) }),

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
};

export function createWS(token: string, onMsg: (msg: any) => void): WebSocket {
  const ws = new WebSocket(`${WS_BASE}/v1/stream?token=${token}`);
  ws.onmessage = (e) => { try { onMsg(JSON.parse(e.data)); } catch {} };
  ws.onclose = () => setTimeout(() => createWS(token, onMsg), 3000);
  return ws;
}
