import { create } from "zustand";
import type { IdentityKeys, RatchetState } from "./e2ee";

export interface User {
  id: string;
  did: string;
  username: string;
  display_name: string;
  avatar_url: string;
  phone?: string;
  tier: number;
  credit_score: number;
  identity_key?: string;
  node_id?: string;
}

export interface Message {
  id: string; conv_id: string; from_did: string; to_did: string;
  type: string; ciphertext: string; status: string;
  sent_at: string; delivered_at?: string; read_at?: string;
  media_url?: string; reply_to_id?: string;
}

export interface Conversation {
  id: string; is_group: boolean; name: string; avatar_url: string;
  last_msg_text: string; last_msg_at?: string; unread_count: number;
  peer_did?: string; peer_name?: string; peer_tier?: number;
  // B7 — backend HandleGetConversations (handlers.go:524-537) bunları zaten
  // döndürüyordu, web tipi eksikti (runtime'da veri vardı, TS görmüyordu).
  conv_type?: "direct" | "group" | "channel" | "community";
  is_public?: boolean;
  my_role?: "admin" | "member";
}

interface State {
  user: User | null;
  conversations: Conversation[];
  activeConvId: string | null;
  messages: Record<string, Message[]>;
  onlineUsers: Set<string>;
  ws: WebSocket | null;
  // E2EE
  identity: IdentityKeys | null;
  ratchets: Record<string, RatchetState>;  // convId → ratchet

  setUser: (u: User | null) => void;
  setConversations: (c: Conversation[]) => void;
  setActiveConv: (id: string | null) => void;
  addMessages: (convId: string, msgs: Message[]) => void;
  addMessage: (msg: Message) => void;
  updateMsgStatus: (msgId: string, status: string, extra?: { delivered_at?: string; read_at?: string }) => void;
  setOnline: (did: string, online: boolean) => void;
  setWS: (ws: WebSocket | null) => void;
  updateUnread: (convId: string, count: number) => void;
  // E2EE
  setIdentity: (identity: IdentityKeys | null) => void;
  setRatchet: (convId: string, state: RatchetState) => void;
}

export const useStore = create<State>((set, get) => ({
  user: null,
  conversations: [],
  activeConvId: null,
  messages: {},
  onlineUsers: new Set(),
  ws: null,
  identity: null,
  ratchets: {},

  setUser: (user) => set({ user }),
  setConversations: (conversations) => set({ conversations }),
  setActiveConv: (id) => set({ activeConvId: id }),

  addMessages: (convId, msgs) => set((s) => ({
    messages: { ...s.messages, [convId]: msgs },
  })),

  addMessage: (msg) => set((s) => {
    const existing = s.messages[msg.conv_id] || [];
    const updated = [...existing.filter((m) => m.id !== msg.id), msg];
    return { messages: { ...s.messages, [msg.conv_id]: updated } };
  }),

  updateMsgStatus: (msgId, status, extra) => set((s) => {
    const updated = { ...s.messages };
    for (const convId in updated) {
      updated[convId] = updated[convId].map((m) =>
        m.id === msgId ? { ...m, status, ...extra } : m
      );
    }
    return { messages: updated };
  }),

  setOnline: (did, online) => set((s) => {
    const next = new Set(s.onlineUsers);
    online ? next.add(did) : next.delete(did);
    return { onlineUsers: next };
  }),

  setWS: (ws) => set({ ws }),

  updateUnread: (convId, count) => set((s) => ({
    conversations: s.conversations.map((c) =>
      c.id === convId ? { ...c, unread_count: count } : c
    ),
  })),

  setIdentity: (identity) => set({ identity }),
  setRatchet: (convId, state) => set((s) => ({
    ratchets: { ...s.ratchets, [convId]: state },
  })),
}));
