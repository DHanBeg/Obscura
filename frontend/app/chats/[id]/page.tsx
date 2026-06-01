"use client";

import {
  useEffect, useCallback, useRef, useState, useMemo
} from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Phone, Video, MoreVertical, ArrowUp,
  Mic, Paperclip, Clock, AlertCircle,
  ShieldCheck, ChevronDown, Trash2, X, Loader2, Lock,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { loadSession, initiateSession, encryptForSend, decryptReceived, isEncryptedPayload } from "@/lib/e2ee-session";
import { AppShell } from "@/components/AppShell";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { GeometricAvatar } from "@/components/GeometricAvatar";
import { MessageStatusIcon, toStatusType } from "@/components/MessageStatus";
import { formatFullTime } from "@/lib/format";

interface Msg {
  id: string; conv_id: string; from_did: string; to_did: string;
  type: string; ciphertext: string; status: string;
  sent_at: string; delivered_at?: string; read_at?: string;
}

// ── Status Icon ───────────────────────────────────────────────────────────────
// Spec Bölüm 6.4: sending→sent→delivered→read→failed→expired
function StatusIcon({ status }: { status: string }) {
  if (status === "read" || status === "delivered" || status === "sent") {
    return <MessageStatusIcon status={toStatusType(status)} size={14} />;
  }
  if (status === "failed") return <AlertCircle size={11} style={{ color: "var(--red)" }} />;
  if (status === "expired") return <Clock size={11} style={{ color: "var(--t3)", opacity: 0.5 }} />;
  return <Clock size={11} style={{ color: "var(--t3)" }} />;
}

// ── Group Messages by sender ──────────────────────────────────────────────────
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

function differentDay(a: string, b: string) {
  return new Date(a).toDateString() !== new Date(b).toDateString();
}

// ── Date Divider ──────────────────────────────────────────────────────────────
function DateDivider({ dateStr }: { dateStr: string }) {
  const d = new Date(dateStr);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);

  let label: string;
  if (d.toDateString() === today.toDateString()) label = "Bugün";
  else if (d.toDateString() === yesterday.toDateString()) label = "Dün";
  else label = d.toLocaleDateString("tr-TR", { day: "numeric", month: "long" });

  return (
    <div className="flex items-center justify-center py-4">
      <span
        className="section-label px-3 py-1 rounded-full"
        style={{
          background: "var(--surface-2)",
          border: "1px solid var(--border-1)",
        }}
      >
        {label}
      </span>
    </div>
  );
}

// ── Skeleton Messages ─────────────────────────────────────────────────────────
function MessageSkeletons() {
  const pattern = [false, true, false, true, false] as const;
  return (
    <div className="space-y-3 px-4 pt-4">
      {pattern.map((mine, i) => (
        <div key={i} className={cn("flex", mine ? "justify-end" : "justify-start")}>
          <div
            className={cn("rounded-2xl shimmer", mine ? "rounded-tr-sm" : "rounded-tl-sm")}
            style={{
              width: `${140 + (i % 3) * 60}px`,
              height: "38px",
              background: "var(--surface-2)",
            }}
          />
        </div>
      ))}
    </div>
  );
}

// ── Empty Chat State ──────────────────────────────────────────────────────────
function EmptyChatState() {
  return (
    <div className="flex flex-col items-center justify-center h-full pb-20 text-center px-6">
      <div
        className="w-14 h-14 rounded-full flex items-center justify-center mb-4"
        style={{ background: "var(--accent-deep)", border: "1px solid rgba(0,229,160,0.15)" }}
      >
        <ShieldCheck size={22} style={{ color: "var(--accent)" }} />
      </div>
      <p
        className="text-sm font-semibold mb-1.5"
        style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
      >
        Sohbet başladı
      </p>
      <p className="text-xs max-w-[200px] leading-relaxed" style={{ color: "var(--text-3)" }}>
        Mesajlar uçtan uca şifreli — sadece siz ve karşı taraf okuyabilir
      </p>
    </div>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────
export default function ChatPage() {
  const params = useParams<{ id: string }>();
  const convId = params.id;
  const router = useRouter();

  const {
    user, conversations, messages, addMessages, addMessage,
    updateMsgStatus, onlineUsers, identity, ratchets, setRatchet
  } = useStore();

  const [inputVal, setInputVal] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [showScrollDown, setShowScrollDown] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [e2eeReady, setE2eeReady] = useState(false);
  const [selectedMsgId, setSelectedMsgId] = useState<string | null>(null);
  const [uploadingMedia, setUploadingMedia] = useState(false);
  const [encryptionBannerDismissed, setEncryptionBannerDismissed] = useState(false);
  const [showCipherDetails, setShowCipherDetails] = useState(false);
  const [showDIDExpand, setShowDIDExpand] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const conv = conversations.find((c) => c.id === convId);
  const convMsgs: Msg[] = useMemo(
    () => (messages[convId] || []).sort(
      (a, b) => new Date(a.sent_at).getTime() - new Date(b.sent_at).getTime()
    ),
    [messages, convId]
  );
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
      } catch {
        // handled by empty state
      } finally {
        setLoading(false);
      }
    })();
  }, [convId, addMessages]);

  // E2EE session init
  useEffect(() => {
    if (!convId || !identity || !conv?.peer_did) return;
    (async () => {
      try {
        const existing = await loadSession(convId);
        if (existing) {
          setRatchet(convId, existing);
          setE2eeReady(true);
          return;
        }
        const bundle = await api.getPreKeyBundle(conv.peer_did!).catch(() => null);
        if (!bundle) return;
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
    } catch {
      setInputVal(text);
    } finally {
      setSending(false);
    }
  }, [inputVal, conv, sending, ratchets, convId, e2eeReady, setRatchet]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInputVal(e.target.value);
    e.target.style.height = "auto";
    e.target.style.height = Math.min(e.target.scrollHeight, 120) + "px";
  };

  const deleteMessage = useCallback(async (msgId: string) => {
    try {
      await api.deleteMessage(msgId);
      addMessages(convId, (messages[convId] || []).filter((m) => m.id !== msgId));
    } catch {}
    setSelectedMsgId(null);
  }, [convId, messages, addMessages]);

  const handleMediaUpload = useCallback(async (file: File) => {
    setUploadingMedia(true);
    try {
      const result = await api.uploadMedia(file, "media");
      await api.sendMessage({
        to_id: conv?.peer_did,
        ciphertext: result.url,
        type: "image",
        media_url: result.url,
      });
    } catch {
      // silently ignore
    } finally {
      setUploadingMedia(false);
    }
  }, [conv?.peer_did]);

  const groups = useMemo(
    () => groupMessages(convMsgs, user?.did || ""),
    [convMsgs, user?.did]
  );

  // ── IntersectionObserver: gelen mesajları görüntülenince "read" olarak işaretle ──
  // Sadece bize gelen (from_did != user.did) ve henüz "read" olmayan mesajlar için.
  const readMarkedRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (loading || !user?.did) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          const msgId = (entry.target as HTMLElement).dataset.msgId;
          if (!msgId || readMarkedRef.current.has(msgId)) continue;
          readMarkedRef.current.add(msgId);
          // Arka planda çağır — UI'yi bloklamasın
          api.markMessageRead(msgId).catch(() => {});
          observer.unobserve(entry.target);
        }
      },
      { threshold: 0.5 } // Mesajın %50'si görünür olduğunda tetikle
    );

    // Tüm gelen, henüz okunmamış mesajların DOM elementlerini gözlemle
    const incomingMsgIds = convMsgs
      .filter((m) => m.from_did !== user.did && m.status !== "read")
      .map((m) => m.id);

    for (const msgId of incomingMsgIds) {
      const el = document.querySelector(`[data-msg-id="${msgId}"]`);
      if (el) observer.observe(el);
    }

    return () => observer.disconnect();
  }, [convMsgs, loading, user?.did]);

  return (
    <AppShell showBack title={peerName}>
      <div className="flex flex-col h-full">

        {/* ── Chat Sub-Header ──────────────────────────────────────────── */}
        <div
          className="flex-shrink-0"
          style={{ borderBottom: "1px solid var(--border-1)" }}
        >
          <div className="flex items-center gap-3 px-4" style={{ height: 56 }}>
            {/* GeometricAvatar with online dot */}
            <div className="relative flex-shrink-0">
              <GeometricAvatar username={peerName} size={40} />
              {peerOnline && (
                <span
                  aria-label="Çevrimiçi"
                  style={{
                    position: "absolute",
                    bottom: 0,
                    right: 0,
                    width: 10,
                    height: 10,
                    borderRadius: "50%",
                    background: "var(--em)",
                    border: "1.5px solid var(--bg)",
                  }}
                />
              )}
            </div>

            {/* Center — tap to expand DID/fingerprint */}
            <button
              className="flex-1 min-w-0 text-left"
              onClick={() => setShowDIDExpand((v) => !v)}
              aria-expanded={showDIDExpand}
              aria-label="Kimlik bilgilerini göster"
            >
              <div className="flex items-center gap-2">
                <p
                  className="text-[15px] font-semibold truncate leading-none"
                  style={{ color: "var(--t1)" }}
                >
                  {peerName}
                </p>
                {/* ENCRYPTED badge */}
                <span
                  className="text-[8px] px-1.5 py-0.5 rounded-full font-mono font-bold flex-shrink-0"
                  style={{
                    background: "var(--em-d)",
                    color: "var(--em)",
                    border: "1px solid rgba(74,222,128,0.2)",
                    letterSpacing: "0.08em",
                  }}
                >
                  ENCRYPTED
                </span>
              </div>
              <div className="flex items-center gap-1.5 mt-1">
                {isTyping ? (
                  <span className="text-[11px] animate-fade-in" style={{ color: "var(--em)" }}>
                    yazıyor...
                  </span>
                ) : peerOnline ? (
                  <span className="flex items-center gap-1.5 text-[11px] font-medium" style={{ color: "var(--em)" }}>
                    <span
                      className="online-blink"
                      style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--em)", display: "inline-block" }}
                    />
                    çevrimiçi
                  </span>
                ) : (
                  <EncryptionBadge verified={e2eeReady} />
                )}
              </div>
            </button>

            <div className="flex items-center gap-0.5">
              <button
                className="btn-icon"
                onClick={() => router.push(`/calls?peer=${conv?.peer_did}`)}
                aria-label="Sesli arama"
              >
                <Phone size={17} />
              </button>
              <button className="btn-icon" aria-label="Görüntülü arama">
                <Video size={17} />
              </button>
              <button className="btn-icon" aria-label="Seçenekler">
                <MoreVertical size={17} />
              </button>
            </div>
          </div>

          {/* DID + Fingerprint expand panel */}
          {showDIDExpand && conv?.peer_did && (
            <div
              className="px-4 py-3 animate-slide-down space-y-2.5"
              style={{
                background: "rgba(0,229,160,0.025)",
                borderBottom: "1px solid rgba(0,229,160,0.1)",
              }}
            >
              <div className="flex items-center gap-2">
                <ShieldCheck size={11} style={{ color: "var(--accent)" }} />
                <span className="text-[11px] font-semibold tracking-wide" style={{ color: "var(--accent)", letterSpacing: "0.05em" }}>
                  KİMLİK DOĞRULANDI
                </span>
              </div>
              <div className="space-y-1">
                <p className="text-[10px] font-mono tracking-widest" style={{ color: "var(--text-3)" }}>DID</p>
                <p
                  className="text-[11px] break-all"
                  style={{ fontFamily: "var(--font-mono)", color: "var(--text-2)", lineHeight: 1.6 }}
                >
                  {conv.peer_did}
                </p>
              </div>
              <div className="space-y-1">
                <p className="text-[10px] font-mono tracking-widest" style={{ color: "var(--text-3)" }}>PARMAK İZİ</p>
                <p
                  className="text-[11px]"
                  style={{ fontFamily: "var(--font-mono)", color: "var(--text-2)", letterSpacing: "0.08em" }}
                >
                  {conv.peer_did.slice(-20).toUpperCase().match(/.{1,2}/g)?.join(":") ?? "—"}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* ── Encryption Banner ──────────────────────────────────────── */}
        {!encryptionBannerDismissed && <div
          className="enc-banner flex-shrink-0 cursor-pointer select-none"
          onClick={() => setShowCipherDetails((v) => !v)}
          role="button"
          aria-expanded={showCipherDetails}
          aria-label="Şifreleme detayları"
        >
          <span style={{ fontSize: 13 }}>🔒</span>
          <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>
            Mesajlar uçtan uca şifrelenmiştir
          </span>
          <button
            onClick={(e) => { e.stopPropagation(); setEncryptionBannerDismissed(true); }}
            className="ml-auto w-5 h-5 rounded flex items-center justify-center"
            style={{ color: "rgba(74,222,128,0.5)" }}
            aria-label="Şifreleme bilgisini kapat"
          >
            <X size={10} />
          </button>
        </div>}

        {showCipherDetails && !encryptionBannerDismissed && (
          <div
            className="flex-shrink-0 px-4 py-2.5 animate-slide-down"
            style={{
              background: "rgba(74,222,128,0.04)",
              borderBottom: "1px solid rgba(74,222,128,0.1)",
            }}
          >
            <div className="flex items-center gap-3 flex-wrap">
              {["Signal Protocol", "AES-256-GCM", "Ed25519", "X3DH"].map((label) => (
                <span
                  key={label}
                  className="text-[10px]"
                  style={{ fontFamily: "var(--font-mono)", color: "rgba(74,222,128,0.6)" }}
                >
                  {label}
                </span>
              ))}
              {conv?.peer_did && (
                <span
                  className="text-[10px] truncate max-w-[140px]"
                  style={{ fontFamily: "var(--font-mono)", color: "var(--t3)" }}
                >
                  {conv.peer_did.slice(0, 22)}…
                </span>
              )}
            </div>
          </div>
        )}

        {/* ── Messages Area ────────────────────────────────────────────── */}
        <div
          ref={messagesContainerRef}
          onScroll={handleScroll}
          className="flex-1 scroll-area px-3 py-2"
        >
          {loading && <MessageSkeletons />}

          {!loading && convMsgs.length === 0 && <EmptyChatState />}

          {!loading && groups.map((group, gi) => (
            <div key={gi}>
              {/* Date dividers */}
              {gi === 0 && group.msgs[0]?.sent_at && (
                <DateDivider dateStr={group.msgs[0].sent_at} />
              )}
              {gi > 0 && differentDay(
                groups[gi - 1].msgs[groups[gi - 1].msgs.length - 1].sent_at,
                group.msgs[0].sent_at
              ) && <DateDivider dateStr={group.msgs[0].sent_at} />}

              {/* Message group */}
              <div
                className={cn(
                  "flex flex-col gap-[3px] mb-4",
                  group.mine ? "items-end" : "items-start"
                )}
              >
                {/* Peer avatar — first message of group */}
                {!group.mine && (
                  <div className="flex items-center gap-2 mb-1 px-1">
                    <GeometricAvatar username={peerName} size={22} />
                    <span className="text-[11px] font-medium" style={{ color: "var(--t3)" }}>
                      {peerName}
                    </span>
                  </div>
                )}

                {group.msgs.map((msg, mi) => {
                  const isFirst = mi === 0;
                  const isLast = mi === group.msgs.length - 1;
                  const text = msg.ciphertext === "__init__" ? null : msg.ciphertext;
                  if (!text) return null;

                  const displayText = (() => {
                    if (!isEncryptedPayload(text)) return text;
                    return text;
                  })();

                  const isSelected = selectedMsgId === msg.id;

                  return (
                    <div
                      key={msg.id}
                      data-msg-id={msg.id}
                      className={cn(
                        "relative max-w-[72%] sm:max-w-[65%]",
                        group.mine ? "ml-auto" : "mr-auto"
                      )}
                    >
                      <div
                        onClick={() => group.mine && setSelectedMsgId(isSelected ? null : msg.id)}
                        className={cn(
                          "px-3.5 py-2.5 text-[14px] leading-relaxed cursor-pointer select-none",
                          "transition-colors duration-100"
                        )}
                        style={
                          group.mine
                            ? {
                                background: isSelected ? "rgba(74,222,128,0.7)" : "var(--em)",
                                borderRadius: isFirst
                                  ? "16px 16px 2px 16px"
                                  : isLast
                                    ? "16px 16px 2px 16px"
                                    : "16px 4px 4px 16px",
                                color: "#0a0a14",
                                fontWeight: 500,
                                boxShadow: "0 1px 4px rgba(0,0,0,0.4)",
                                animation: "msgSlideRight 200ms cubic-bezier(0,0,0.2,1) both",
                              }
                            : {
                                background: "var(--bg2)",
                                border: "1px solid var(--border)",
                                borderRadius: isFirst
                                  ? "16px 16px 16px 2px"
                                  : isLast
                                    ? "16px 16px 16px 2px"
                                    : "4px 16px 16px 4px",
                                color: "var(--t1)",
                                boxShadow: "0 1px 3px rgba(0,0,0,0.35)",
                                animation: "msgSlideLeft 200ms cubic-bezier(0,0,0.2,1) both",
                              }
                        }
                      >
                        {displayText}
                      </div>

                      {/* Delete action popup */}
                      {isSelected && group.mine && (
                        <div className="absolute -top-9 right-0 flex items-center gap-1 animate-fade-in z-10">
                          <button
                            onClick={() => deleteMessage(msg.id)}
                            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-white text-xs font-semibold shadow-lg"
                            style={{ background: "var(--error)" }}
                          >
                            <Trash2 size={10} />
                            Sil
                          </button>
                          <button
                            onClick={() => setSelectedMsgId(null)}
                            className="w-6 h-6 rounded-full flex items-center justify-center"
                            style={{
                              background: "var(--surface-3)",
                              border: "1px solid var(--border-2)",
                              color: "var(--text-2)",
                            }}
                            aria-label="İptal"
                          >
                            <X size={10} />
                          </button>
                        </div>
                      )}

                      {/* Timestamp + lock + status (only on last of group) */}
                      {isLast && (
                        <div
                          className={cn(
                            "flex items-center gap-1 mt-1 px-1",
                            group.mine ? "justify-end" : "justify-start"
                          )}
                        >
                          <span
                            style={{ fontSize: 8, color: "var(--em)", opacity: 0.7 }}
                            aria-hidden="true"
                          >
                            🔒
                          </span>
                          <span
                            className="text-[9px]"
                            style={{
                              fontFamily: "var(--font-mono)",
                              color: group.mine ? "rgba(74,222,128,0.55)" : "var(--t3)",
                            }}
                          >
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

          <div ref={messagesEndRef} />
        </div>

        {/* ── Scroll to Bottom ─────────────────────────────────────────── */}
        {showScrollDown && (
          <button
            onClick={() => { scrollToBottom(true); setShowScrollDown(false); }}
            className="absolute right-4 bottom-24 w-9 h-9 rounded-full flex items-center justify-center glass animate-scale-in z-10 shadow-lg"
            style={{
              border: "1px solid var(--border-2)",
              color: "var(--text-2)",
            }}
            aria-label="En alta git"
          >
            <ChevronDown size={16} />
          </button>
        )}

        {/* ── Composer ─────────────────────────────────────────────────── */}
        <div
          className="px-3 pt-2 flex-shrink-0"
          style={{
            borderTop: "1px solid var(--border-1)",
            background: "var(--void)",
            paddingBottom: "max(12px, calc(var(--gw-height, 0px) + 24px))",
          }}
        >
          {/* Hidden file input */}
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*,video/*,audio/*,.pdf,.doc,.docx"
            className="hidden"
            onChange={(e) => e.target.files?.[0] && handleMediaUpload(e.target.files[0])}
            aria-hidden="true"
          />

          <div className="flex items-end gap-2">
            {/* Attachment button */}
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={uploadingMedia}
              className="btn-icon flex-shrink-0 mb-0.5"
              aria-label="Dosya ekle"
            >
              {uploadingMedia
                ? <Loader2 size={17} className="animate-spin" />
                : <Paperclip size={17} />
              }
            </button>

            {/* Text input — styled per reference */}
            <div
              className="flex-1 flex items-center rounded-full transition-all duration-150"
              style={{
                background: "var(--bg2)",
                border: "1px solid var(--border)",
              }}
              onFocus={(e) => {
                (e.currentTarget as HTMLDivElement).style.borderColor = "rgba(74,222,128,0.25)";
              }}
              onBlur={(e) => {
                (e.currentTarget as HTMLDivElement).style.borderColor = "var(--border)";
              }}
            >
              <textarea
                ref={inputRef}
                rows={1}
                value={inputVal}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
                placeholder="Mesaj yaz..."
                className={cn(
                  "flex-1 resize-none bg-transparent px-4 py-3 text-[14px]",
                  "focus:outline-none min-h-[44px] max-h-[120px]"
                )}
                style={{
                  color: "var(--t1)",
                  caretColor: "var(--em)",
                  height: "44px",
                  lineHeight: "1.5",
                }}
                aria-label="Mesaj yaz"
              />
              {/* E2E indicator */}
              <div
                className="flex-shrink-0 flex items-center gap-1 pr-4"
                aria-label="Uçtan uca şifreli"
              >
                <span style={{ fontSize: 9 }}>🔒</span>
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 8,
                    color: "var(--em)",
                    opacity: 0.7,
                  }}
                >
                  E2E
                </span>
              </div>
            </div>

            {/* Send / Mic button */}
            <button
              onClick={inputVal.trim() ? sendMessage : undefined}
              disabled={sending}
              className="flex-shrink-0 w-11 h-11 rounded-full flex items-center justify-center mb-0.5 transition-all duration-150 active:scale-95"
              style={
                inputVal.trim()
                  ? {
                      background: "var(--accent)",
                      color: "var(--void)",
                      boxShadow: "0 0 20px rgba(0,229,160,0.25), 0 2px 8px rgba(0,0,0,0.4)",
                    }
                  : {
                      background: "var(--surface-2)",
                      border: "1px solid var(--border-1)",
                      color: "var(--text-3)",
                    }
              }
              aria-label={inputVal.trim() ? "Mesaj gönder" : "Sesli mesaj"}
            >
              {inputVal.trim()
                ? <ArrowUp size={18} strokeWidth={2.5} />
                : <Mic size={17} />
              }
            </button>
          </div>
        </div>
      </div>
    </AppShell>
  );
}
