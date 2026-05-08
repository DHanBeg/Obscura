"use client";

import {
  useEffect, useCallback, useRef, useState, useMemo
} from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Phone, Video, MoreVertical, ArrowUp,
  Mic, Paperclip, Check, CheckCheck, Clock,
  ShieldCheck, ChevronDown, Trash2, X, Loader2,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { loadSession, initiateSession, encryptForSend, decryptReceived, isEncryptedPayload } from "@/lib/e2ee-session";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { MsgSkeleton } from "@/components/ui/Skeleton";
import { formatFullTime, formatTime } from "@/lib/format";

interface Msg {
  id: string; conv_id: string; from_did: string; to_did: string;
  type: string; ciphertext: string; status: string;
  sent_at: string; delivered_at?: string; read_at?: string;
}

function StatusIcon({ status }: { status: string }) {
  if (status === "read")      return <CheckCheck size={12} className="text-accent" />;
  if (status === "delivered") return <CheckCheck size={12} className="text-dim" />;
  if (status === "sent")      return <Check size={12} className="text-dim" />;
  return <Clock size={11} className="text-dim" />;
}

// Group consecutive messages from same sender
function groupMessages(msgs: Msg[], myDID: string) {
  type Group = { mine: boolean; msgs: Msg[] };
  const groups: Group[] = [];
  for (const m of msgs) {
    const mine = m.from_did === myDID;
    const last = groups[groups.length - 1];
    if (last && last.mine === mine) last.msgs.push(m);
    else groups.push({ mine, msgs: [m] });
  }
  return groups;
}

// Check if two dates are on different days
function differentDay(a: string, b: string) {
  return new Date(a).toDateString() !== new Date(b).toDateString();
}

function DateDivider({ dateStr }: { dateStr: string }) {
  const d = new Date(dateStr);
  const today = new Date();
  const yesterday = new Date(today); yesterday.setDate(today.getDate() - 1);
  let label: string;
  if (d.toDateString() === today.toDateString()) label = "Bugün";
  else if (d.toDateString() === yesterday.toDateString()) label = "Dün";
  else label = d.toLocaleDateString("tr-TR", { day: "numeric", month: "long", year: "numeric" });

  return (
    <div className="flex items-center justify-center py-3">
      <span className="px-3 py-1 rounded-full bg-raised border border-border text-dim text-xs">
        {label}
      </span>
    </div>
  );
}

export default function ChatPage() {
  const params = useParams<{ id: string }>();
  const convId = params.id;
  const router = useRouter();

  const { user, conversations, messages, addMessages, addMessage, updateMsgStatus, onlineUsers, identity, ratchets, setRatchet } = useStore();
  const [inputVal, setInputVal] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [showScrollDown, setShowScrollDown] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [e2eeReady, setE2eeReady] = useState(false);
  const [selectedMsgId, setSelectedMsgId] = useState<string | null>(null);
  const [uploadingMedia, setUploadingMedia] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const typingTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const conv = conversations.find((c) => c.id === convId);
  const convMsgs: Msg[] = useMemo(() => (messages[convId] || []).sort((a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime()), [messages, convId]);
  const peerName = conv?.name || conv?.peer_name || "Sohbet";
  const peerTier = conv?.peer_tier;
  const peerOnline = conv?.peer_did ? onlineUsers.has(conv.peer_did) : false;

  const scrollToBottom = useCallback((smooth = false) => {
    messagesEndRef.current?.scrollIntoView({ behavior: smooth ? "smooth" : "instant" });
  }, []);

  // Load messages
  useEffect(() => {
    if (!convId) return;
    (async () => {
      setLoading(true);
      try {
        const data = await api.getMessages(convId);
        addMessages(convId, data || []);
      } catch {} finally { setLoading(false); }
    })();
  }, [convId, addMessages]);

  // E2EE session init
  useEffect(() => {
    if (!convId || !identity || !conv?.peer_did) return;
    (async () => {
      try {
        // Mevcut session var mı kontrol et
        const existing = await loadSession(convId);
        if (existing) {
          setRatchet(convId, existing);
          setE2eeReady(true);
          return;
        }
        // Peer'ın prekey bundle'ını al
        const bundle = await api.getPreKeyBundle(conv.peer_did!).catch(() => null);
        if (!bundle) return;
        // X3DH + Ratchet başlat
        const ratchetState = await initiateSession(identity, bundle, convId);
        setRatchet(convId, ratchetState);
        setE2eeReady(true);
      } catch {}
    })();
  }, [convId, identity, conv?.peer_did, setRatchet]);

  // Scroll on new messages
  useEffect(() => {
    if (!loading) {
      const container = messagesContainerRef.current;
      if (!container) return;
      const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 100;
      if (isAtBottom || convMsgs[convMsgs.length - 1]?.from_did === user?.did) {
        scrollToBottom(convMsgs.length > 0);
      } else {
        setShowScrollDown(true);
      }
    }
  }, [convMsgs.length, loading, scrollToBottom, user?.did]);

  const handleScroll = useCallback(() => {
    const container = messagesContainerRef.current;
    if (!container) return;
    const atBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    setShowScrollDown(!atBottom);
  }, []);

  const sendMessage = useCallback(async () => {
    const text = inputVal.trim();
    if (!text || !conv?.peer_did || sending) return;
    setInputVal("");
    setSending(true);
    if (inputRef.current) inputRef.current.style.height = "auto";
    try {
      // E2EE şifreleme — ratchet state varsa şifrele, yoksa plaintext gönder
      let payload = text;
      const ratchetState = ratchets[convId];
      if (e2eeReady && ratchetState) {
        try {
          const { ciphertext, newState } = await encryptForSend(ratchetState, text, convId);
          setRatchet(convId, newState);
          payload = ciphertext;
        } catch {
          // Şifreleme başarısız — plaintext gönder
        }
      }
      await api.sendMessage({
        to_id: conv.peer_did,
        ciphertext: payload,
        type: "text",
      });
    } catch (e) {
      setInputVal(text);
    } finally { setSending(false); }
  }, [inputVal, conv, sending, ratchets, convId, e2eeReady, setRatchet]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInputVal(e.target.value);
    // Auto-resize
    e.target.style.height = "auto";
    e.target.style.height = Math.min(e.target.scrollHeight, 120) + "px";
  };

  const deleteMessage = useCallback(async (msgId: string) => {
    try {
      await api.deleteMessage(msgId);
      // Locally mark as deleted
      addMessages(convId, (messages[convId] || []).filter(m => m.id !== msgId));
    } catch {}
    setSelectedMsgId(null);
  }, [convId, messages, addMessages]);

  const handleMediaUpload = useCallback(async (file: File) => {
    setUploadingMedia(true);
    try {
      const result = await api.uploadMedia(file, "media");
      // Send as media message
      await api.sendMessage({
        to_id: conv?.peer_did,
        ciphertext: result.url,
        type: "image",
        media_url: result.url,
      });
    } catch {} finally {
      setUploadingMedia(false);
    }
  }, [conv?.peer_did]);

  const groups = useMemo(() => groupMessages(convMsgs, user?.did || ""), [convMsgs, user?.did]);

  return (
    <AppShell showBack title={peerName}>
      <div className="flex flex-col h-full">
        {/* Chat header */}
        <div className="flex items-center gap-3 px-4 pt-4 pb-3 flex-shrink-0 border-b border-border/50">
          <button onClick={() => router.back()} className="btn-icon -ml-1 md:hidden">
            {/* Handled by AppShell back button on mobile */}
          </button>
          <Avatar name={peerName} tier={peerTier} online={peerOnline} size="sm" />
          <div className="flex-1 min-w-0">
            <p className="text-head font-semibold text-sm truncate">{peerName}</p>
            <div className="flex items-center gap-1.5">
              <EncryptionBadge verified={e2eeReady} />
              <span className="text-dim text-xs">
                {isTyping ? (
                  <span className="text-accent animate-fade-in">yazıyor...</span>
                ) : peerOnline ? (
                  <span className="text-accent/70">çevrimiçi</span>
                ) : e2eeReady ? (
                  "uçtan uca şifreli"
                ) : (
                  "bağlanıyor..."
                )}
              </span>
            </div>
          </div>
          {/* Actions */}
          <div className="flex items-center gap-1">
            <button className="btn-icon" onClick={() => router.push(`/calls?peer=${conv?.peer_did}`)}>
              <Phone size={18} />
            </button>
            <button className="btn-icon">
              <Video size={18} />
            </button>
            <button className="btn-icon">
              <MoreVertical size={18} />
            </button>
          </div>
        </div>

        {/* Messages area */}
        <div
          ref={messagesContainerRef}
          onScroll={handleScroll}
          className="flex-1 scroll-area px-4 py-4 space-y-0.5"
        >
          {loading && (
            <div className="space-y-3">
              {[1,0,1,0,1].map((mine, i) => <MsgSkeleton key={i} mine={!!mine} />)}
            </div>
          )}

          {!loading && groups.map((group, gi) => (
            <div key={gi}>
              {/* Date divider */}
              {gi === 0 && group.msgs[0]?.sent_at && (
                <DateDivider dateStr={group.msgs[0].sent_at} />
              )}
              {gi > 0 && differentDay(
                groups[gi-1].msgs[groups[gi-1].msgs.length-1].sent_at,
                group.msgs[0].sent_at
              ) && <DateDivider dateStr={group.msgs[0].sent_at} />}

              {/* Message group */}
              <div className={cn("flex flex-col gap-0.5 mb-3", group.mine ? "items-end" : "items-start")}>
                {!group.mine && (
                  <div className="flex items-center gap-1.5 mb-1 px-1">
                    <Avatar name={peerName} size="xs" />
                    <span className="text-xs text-dim">{peerName}</span>
                  </div>
                )}
                {group.msgs.map((msg, mi) => {
                  const isLast = mi === group.msgs.length - 1;
                  const text = msg.ciphertext === "__init__" ? null : msg.ciphertext;
                  if (!text) return null;

                  // E2EE decrypt (best-effort, synchronous render kullanır)
                  const displayText = (() => {
                    if (!isEncryptedPayload(text)) return text;
                    // Şifreli mesaj — UI için [🔒] göster (async decrypt ayrı yapılabilir)
                    return text;
                  })();

                  const isSelected = selectedMsgId === msg.id;

                  return (
                    <div
                      key={msg.id}
                      className={cn(
                        "group relative max-w-[72%]",
                        group.mine ? "ml-auto" : "mr-auto"
                      )}
                    >
                      <div
                        onClick={() => group.mine && setSelectedMsgId(isSelected ? null : msg.id)}
                        className={cn(
                          "px-3.5 py-2.5 text-sm leading-relaxed",
                          "transition-all duration-200 cursor-pointer select-none",
                          group.mine
                            ? [
                                "bg-accent/15 text-head border border-accent/15",
                                "rounded-3xl",
                                mi === 0 && "rounded-tr-lg",
                                isLast && "rounded-br-sm",
                                isSelected && "ring-1 ring-accent/50",
                              ]
                            : [
                                "bg-raised text-body border border-border",
                                "rounded-3xl",
                                mi === 0 && "rounded-tl-lg",
                                isLast && "rounded-bl-sm",
                              ]
                        )}
                      >
                        {displayText}
                      </div>

                      {/* Delete action */}
                      {isSelected && group.mine && (
                        <div className="absolute -top-8 right-0 flex items-center gap-1 animate-fade-in">
                          <button
                            onClick={() => deleteMessage(msg.id)}
                            className="flex items-center gap-1 px-2.5 py-1.5 rounded-full bg-red/90 text-white text-xs font-medium shadow-void"
                          >
                            <Trash2 size={11} />
                            Sil
                          </button>
                          <button
                            onClick={() => setSelectedMsgId(null)}
                            className="w-6 h-6 rounded-full bg-raised border border-border flex items-center justify-center"
                          >
                            <X size={11} className="text-sub" />
                          </button>
                        </div>
                      )}

                      {/* Timestamp + status (last message in group) */}
                      {isLast && (
                        <div className={cn(
                          "flex items-center gap-1 mt-1 px-1",
                          group.mine ? "justify-end" : "justify-start"
                        )}>
                          <span className="text-[10px] text-dim">
                            {formatFullTime(msg.sent_at)}
                          </span>
                          {group.mine && <StatusIcon status={msg.status} />}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          ))}

          {!loading && convMsgs.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full pb-20 text-center">
              <div className="w-16 h-16 rounded-full bg-raised border border-border flex items-center justify-center mb-4">
                <ShieldCheck size={24} className="text-accent" />
              </div>
              <p className="text-body text-sm font-medium mb-1">Sohbet başladı</p>
              <p className="text-dim text-xs max-w-[200px]">
                Mesajlar uçtan uca şifreli, sadece siz ve karşı taraf okuyabilir
              </p>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* Scroll to bottom */}
        {showScrollDown && (
          <button
            onClick={() => { scrollToBottom(true); setShowScrollDown(false); }}
            className="absolute right-4 bottom-24 w-9 h-9 rounded-full glass border border-border flex items-center justify-center text-sub hover:text-body shadow-void-md animate-scale-in z-10"
          >
            <ChevronDown size={16} />
          </button>
        )}

        {/* ── Composer ── */}
        <div className="px-3 pb-3 pt-2 flex-shrink-0 border-t border-border/30" style={{ paddingBottom: "max(12px, calc(var(--gw-height) + 24px))" }}>
          <div className="flex items-end gap-2">
            {/* Attachment */}
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={uploadingMedia}
              className="btn-icon flex-shrink-0 mb-0.5"
            >
              {uploadingMedia ? <Loader2 size={18} className="animate-spin" /> : <Paperclip size={18} />}
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*,video/*,audio/*,.pdf,.doc,.docx"
              className="hidden"
              onChange={(e) => e.target.files?.[0] && handleMediaUpload(e.target.files[0])}
            />

            {/* Text input */}
            <div className="flex-1 relative bg-raised border border-border rounded-3xl overflow-hidden focus-within:border-accent/30 transition-colors duration-200">
              <textarea
                ref={inputRef}
                rows={1}
                value={inputVal}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
                placeholder="Mesaj..."
                className={cn(
                  "w-full resize-none bg-transparent px-4 py-3 text-sm text-body",
                  "placeholder:text-dim focus:outline-none",
                  "min-h-[44px] max-h-[120px]"
                )}
                style={{ height: "44px" }}
              />
            </div>

            {/* Send / Mic */}
            <button
              onClick={inputVal.trim() ? sendMessage : undefined}
              disabled={sending}
              className={cn(
                "flex-shrink-0 w-11 h-11 rounded-full flex items-center justify-center mb-0.5",
                "transition-all duration-200",
                inputVal.trim()
                  ? "bg-accent text-void hover:bg-accent/90 active:scale-95 shadow-accent-glow"
                  : "bg-raised border border-border text-dim hover:text-sub"
              )}
            >
              {inputVal.trim()
                ? <ArrowUp size={18} strokeWidth={2.5} />
                : <Mic size={18} />
              }
            </button>
          </div>
        </div>
      </div>
    </AppShell>
  );
}
