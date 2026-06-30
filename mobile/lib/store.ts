import { create } from "zustand";

export interface User {
  id: string; did: string; username: string;
  display_name: string; avatar_url: string;
  tier: number; credit_score: number; phone?: string;
}

export interface Message {
  id: string; conv_id: string; from_did: string; to_did: string;
  type: string; ciphertext: string;
  status: "pending" | "sent" | "delivered" | "read" | "failed";
  sent_at: string; delivered_at?: string; read_at?: string;
  reply_to_id?: string;
}

export interface Conversation {
  id: string; is_group: boolean; name: string;
  last_msg_text: string; last_msg_at?: string;
  unread_count: number; peer_did?: string;
  peer_name?: string; peer_tier?: number;
}

interface State {
  user: User | null;
  conversations: Conversation[];
  messages: Record<string, Message[]>;
  onlineUsers: Set<string>;
  typingUsers: Record<string, string[]>;
  ws: WebSocket | null;
  networkStatus: "online" | "offline" | "connecting";
  wsConnected: boolean;
  incomingCall: IncomingCall | null;
  pendingCallAnswer: string | null;
  activeCallId: string | null;

  setUser: (u: User | null) => void;
  setConversations: (c: Conversation[]) => void;
  setMessages: (convId: string, msgs: Message[]) => void;
  addMessage: (msg: Message) => void;
  updateMsgStatus: (msgId: string, status: Message["status"]) => void;
  setOnline: (did: string, online: boolean) => void;
  setTyping: (convId: string, did: string, typing: boolean) => void;
  removeMessage: (convId: string, msgId: string) => void;
  setWS: (ws: WebSocket | null) => void;
  setNetworkStatus: (s: "online" | "offline" | "connecting") => void;
  setWsConnected: (v: boolean) => void;
  incomingCall: IncomingCall | null;
  pendingCallAnswer: string | null;
  activeCallId: string | null;
  setIncomingCall: (c: IncomingCall | null) => void;
  setPendingCallAnswer: (sdp: string | null) => void;
  setActiveCallId: (id: string | null) => void;
  fontSize: number;
  setFontSize: (size: number) => void;
  reset: () => void;
}

export interface IncomingCall {
  callId: string;
  callerDid: string;
  callerName: string;
  sdpOffer: string;
  callType: "audio" | "video";
}

export const useStore = create<State>((set) => ({
  user: null,
  conversations: [],
  messages: {},
  onlineUsers: new Set(),
  typingUsers: {},
  ws: null,
  networkStatus: "connecting",
  wsConnected: false,
  incomingCall: null,
  pendingCallAnswer: null,
  activeCallId: null,
  fontSize: 15,

  setUser: (user) => set({ user }),
  setConversations: (conversations) => set({ conversations }),

  setMessages: (convId, msgs) => set((s) => ({
    messages: { ...s.messages, [convId]: msgs },
  })),

  addMessage: (msg) => set((s) => {
    const existing = s.messages[msg.conv_id] || [];
    const updated = [...existing.filter((m) => m.id !== msg.id), msg]
      .sort((a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime());
    const conversations = s.conversations.map((c) =>
      c.id === msg.conv_id
        ? { ...c, last_msg_text: msg.ciphertext, last_msg_at: msg.sent_at }
        : c
    );
    return { messages: { ...s.messages, [msg.conv_id]: updated }, conversations };
  }),

  updateMsgStatus: (msgId, status) => set((s) => {
    const updated = { ...s.messages };
    for (const k in updated) {
      updated[k] = updated[k].map((m) => m.id === msgId ? { ...m, status } : m);
    }
    return { messages: updated };
  }),

  setOnline: (did, online) => set((s) => {
    const next = new Set(s.onlineUsers);
    online ? next.add(did) : next.delete(did);
    return { onlineUsers: next };
  }),

  setTyping: (convId, did, typing) => set((s) => {
    const cur = s.typingUsers[convId] || [];
    return {
      typingUsers: {
        ...s.typingUsers,
        [convId]: typing ? [...new Set([...cur, did])] : cur.filter((d) => d !== did),
      },
    };
  }),

  removeMessage: (convId, msgId) => set((s) => ({
    messages: {
      ...s.messages,
      [convId]: (s.messages[convId] || []).filter((m) => m.id !== msgId),
    },
  })),

  setWS: (ws) => set({ ws }),
  setNetworkStatus: (networkStatus) => set({ networkStatus }),
  setWsConnected: (wsConnected) => set({ wsConnected }),
  incomingCall: null,
  pendingCallAnswer: null,
  activeCallId: null,
  setIncomingCall: (incomingCall) => set({ incomingCall }),
  setPendingCallAnswer: (pendingCallAnswer) => set({ pendingCallAnswer }),
  setActiveCallId: (activeCallId) => set({ activeCallId }),
  setFontSize: (fontSize) => set({ fontSize }),
  reset: () => set((state) => {
    if (state.ws) {
      state.ws.onclose = null;
      state.ws.close(1000, "logout");
    }
    return { user: null, conversations: [], messages: {}, ws: null, wsConnected: false };
  }),
}));
