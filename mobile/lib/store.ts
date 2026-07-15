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
  // expires_at (30 gün genel saklama TTL) İLE KARIŞTIRMA — bu, kullanıcının
  // bilinçli seçtiği erken silme: seconds null=kapalı, 0="okununca",
  // >0=gönderimden N sn sonra (10/60/300/3600). self_destruct_at backend'in
  // hesapladığı mutlak silme zamanı ("okununca" modunda okunana kadar null).
  self_destruct_seconds?: number | null;
  self_destruct_at?: string | null;
}

export interface Conversation {
  id: string; is_group: boolean; name: string;
  last_msg_text: string; last_msg_at?: string;
  unread_count: number; peer_did?: string;
  peer_name?: string; peer_tier?: number;
  conv_type?: "direct" | "group" | "channel" | "community";
  description?: string; is_public?: boolean;
  avatar_url?: string; my_role?: "admin" | "member";
}

interface State {
  user: User | null;
  conversations: Conversation[];
  messages: Record<string, Message[]>;
  // WS status events (delivery_ack/read_receipt) for a real server id can
  // arrive before the optimistic temp→real swap runs for that message, so
  // updateMsgStatus has nothing to match yet. Buffer those here, keyed by
  // real msg id, and replaceMessage consumes/clears the entry on swap.
  pendingStatus: Record<string, Message["status"]>;
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
  replaceMessage: (convId: string, tempId: string, msg: Message) => void;
  updateMsgStatus: (msgId: string, status: Message["status"]) => void;
  setOnline: (did: string, online: boolean) => void;
  setTyping: (convId: string, did: string, typing: boolean) => void;
  removeMessage: (convId: string, msgId: string) => void;
  setWS: (ws: WebSocket | null) => void;
  setNetworkStatus: (s: "online" | "offline" | "connecting") => void;
  setWsConnected: (v: boolean) => void;
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
  pendingStatus: {},
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

  replaceMessage: (convId, tempId, msg) => set((s) => {
    const existing = s.messages[convId] || [];
    // A delivery_ack/read_receipt for msg.id may have arrived (and been
    // buffered into pendingStatus, see updateMsgStatus) before this swap ran.
    const buffered = s.pendingStatus[msg.id];
    const rank: Record<Message["status"], number> = { pending: 0, sent: 1, delivered: 2, read: 3, failed: 0 };
    const status = buffered && rank[buffered] > rank[msg.status] ? buffered : msg.status;
    const updated = [...existing.filter((m) => m.id !== tempId && m.id !== msg.id), { ...msg, status }]
      .sort((a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime());
    const pendingStatus = { ...s.pendingStatus };
    delete pendingStatus[msg.id];
    return { messages: { ...s.messages, [convId]: updated }, pendingStatus };
  }),

  updateMsgStatus: (msgId, status) => set((s) => {
    let matched = false;
    const updated = { ...s.messages };
    for (const k in updated) {
      updated[k] = updated[k].map((m) => {
        if (m.id !== msgId) return m;
        matched = true;
        return { ...m, status };
      });
    }
    // Not found yet (e.g. optimistic temp→real swap hasn't run) — buffer it
    // so replaceMessage can apply it once the real id lands in the store.
    const pendingStatus = matched ? s.pendingStatus : { ...s.pendingStatus, [msgId]: status };
    return { messages: updated, pendingStatus };
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
  setIncomingCall: (incomingCall) => set({ incomingCall }),
  setPendingCallAnswer: (pendingCallAnswer) => set({ pendingCallAnswer }),
  setActiveCallId: (activeCallId) => set({ activeCallId }),
  setFontSize: (fontSize) => set({ fontSize }),
  reset: () => set((state) => {
    if (state.ws) {
      state.ws.onclose = null;
      state.ws.close(1000, "logout");
    }
    return { user: null, conversations: [], messages: {}, pendingStatus: {}, ws: null, wsConnected: false };
  }),
}));
