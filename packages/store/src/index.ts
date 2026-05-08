/**
 * @obscura/store — Paylaşımlı global state
 * Zustand — Web, Mobil ve Desktop'ta aynı store
 */

import { create } from "zustand";
import type { User, Message, Conversation } from "@obscura/api";

export type { User, Message, Conversation };

export interface AppState {
  // Auth
  token: string | null;
  user: User | null;

  // Conversations & messages
  conversations: Conversation[];
  messages: Record<string, Message[]>;
  activeConvId: string | null;

  // Realtime
  onlineUsers: Set<string>;
  typingUsers: Record<string, string[]>; // convId → [did]
  ws: WebSocket | null;

  // E2EE (platform-specific implementation inject edilir)
  identityKey: string | null;   // Base64 public key
  ratchets: Record<string, any>; // convId → RatchetState

  // UI state
  networkStatus: "online" | "offline" | "connecting";

  // Actions — Auth
  setToken: (token: string | null) => void;
  setUser: (user: User | null) => void;
  logout: () => void;

  // Actions — Conversations
  setConversations: (convs: Conversation[]) => void;
  updateConversation: (convId: string, patch: Partial<Conversation>) => void;

  // Actions — Messages
  setMessages: (convId: string, msgs: Message[]) => void;
  addMessage: (msg: Message) => void;
  updateMessageStatus: (msgId: string, status: Message["status"]) => void;
  prependMessages: (convId: string, msgs: Message[]) => void;

  // Actions — Realtime
  setOnline: (did: string, online: boolean) => void;
  setTyping: (convId: string, did: string, isTyping: boolean) => void;
  setWS: (ws: WebSocket | null) => void;

  // Actions — E2EE
  setIdentityKey: (key: string | null) => void;
  setRatchet: (convId: string, state: any) => void;

  // Actions — UI
  setActiveConv: (id: string | null) => void;
  setNetworkStatus: (status: "online" | "offline" | "connecting") => void;
}

export const useStore = create<AppState>((set, get) => ({
  // Initial state
  token: null,
  user: null,
  conversations: [],
  messages: {},
  activeConvId: null,
  onlineUsers: new Set(),
  typingUsers: {},
  ws: null,
  identityKey: null,
  ratchets: {},
  networkStatus: "connecting",

  // Auth
  setToken: (token) => set({ token }),
  setUser: (user) => set({ user }),
  logout: () => set({
    token: null,
    user: null,
    conversations: [],
    messages: {},
    onlineUsers: new Set(),
    ws: null,
    identityKey: null,
    ratchets: {},
  }),

  // Conversations
  setConversations: (conversations) => set({ conversations }),
  updateConversation: (convId, patch) => set((s) => ({
    conversations: s.conversations.map((c) =>
      c.id === convId ? { ...c, ...patch } : c
    ),
  })),

  // Messages
  setMessages: (convId, msgs) => set((s) => ({
    messages: { ...s.messages, [convId]: msgs },
  })),

  addMessage: (msg) => set((s) => {
    const existing = s.messages[msg.conv_id] || [];
    const deduped = existing.filter((m) => m.id !== msg.id);
    const updated = [...deduped, msg].sort(
      (a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime()
    );
    // Son mesajı conversation'da güncelle
    const conversations = s.conversations.map((c) =>
      c.id === msg.conv_id
        ? { ...c, last_msg_text: msg.ciphertext, last_msg_at: msg.sent_at }
        : c
    );
    return { messages: { ...s.messages, [msg.conv_id]: updated }, conversations };
  }),

  updateMessageStatus: (msgId, status) => set((s) => {
    const updated = { ...s.messages };
    for (const convId in updated) {
      updated[convId] = updated[convId].map((m) =>
        m.id === msgId ? { ...m, status } : m
      );
    }
    return { messages: updated };
  }),

  prependMessages: (convId, msgs) => set((s) => {
    const existing = s.messages[convId] || [];
    const ids = new Set(existing.map((m) => m.id));
    const newMsgs = msgs.filter((m) => !ids.has(m.id));
    return {
      messages: {
        ...s.messages,
        [convId]: [...newMsgs, ...existing].sort(
          (a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime()
        ),
      },
    };
  }),

  // Realtime
  setOnline: (did, online) => set((s) => {
    const next = new Set(s.onlineUsers);
    online ? next.add(did) : next.delete(did);
    return { onlineUsers: next };
  }),

  setTyping: (convId, did, isTyping) => set((s) => {
    const current = s.typingUsers[convId] || [];
    const updated = isTyping
      ? [...new Set([...current, did])]
      : current.filter((d) => d !== did);
    return { typingUsers: { ...s.typingUsers, [convId]: updated } };
  }),

  setWS: (ws) => set({ ws }),

  // E2EE
  setIdentityKey: (identityKey) => set({ identityKey }),
  setRatchet: (convId, state) => set((s) => ({
    ratchets: { ...s.ratchets, [convId]: state },
  })),

  // UI
  setActiveConv: (activeConvId) => set({ activeConvId }),
  setNetworkStatus: (networkStatus) => set({ networkStatus }),
}));

// ── Selectors ────────────────────────────────────────────────────────────────

export const selectConversation = (id: string) => (s: AppState) =>
  s.conversations.find((c) => c.id === id);

export const selectMessages = (convId: string) => (s: AppState) =>
  s.messages[convId] || [];

export const selectUnreadTotal = (s: AppState) =>
  s.conversations.reduce((sum, c) => sum + c.unread_count, 0);

export const selectIsOnline = (did: string) => (s: AppState) =>
  s.onlineUsers.has(did);

export const selectIsTyping = (convId: string) => (s: AppState) =>
  (s.typingUsers[convId] || []).length > 0;
