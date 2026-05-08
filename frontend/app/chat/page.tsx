"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { api, createWS } from "@/lib/api";
import { useStore } from "@/lib/store";

/* ─── TİER RENK/İSİM ───────────────────────────────────────────────────── */
const TIER = {
  1: { name: "Bronz",  color: "#cd7f32", dot: "🟤" },
  2: { name: "Gümüş", color: "#c0c0c0", dot: "⚪" },
  3: { name: "Altın",  color: "#ffd700", dot: "🟡" },
  4: { name: "Platin", color: "#e5e4e2", dot: "◈"  },
  5: { name: "Elmas",  color: "#b9f2ff", dot: "💠" },
} as const;

/* ─── MESAJ DURUMU İKONU (pençe izi) ────────────────────────────────────── */
function StatusIcon({ status }: { status: string }) {
  if (status === "sending") return <span style={{ color: "#444", fontSize: 10 }}>⏳</span>;
  if (status === "sent")
    return (
      <svg viewBox="0 0 24 13" width="18" height="10">
        <polygon points="4,1 6,0 19,12 17,13" fill="#3a5c3a"/>
      </svg>
    );
  if (status === "delivered")
    return (
      <svg viewBox="0 0 32 13" width="22" height="10">
        <polygon points="2,1 4,0 15,12 13,13" fill="#3a5c3a"/>
        <polygon points="10,1 12,0 25,12 23,13" fill="#3a5c3a"/>
      </svg>
    );
  if (status === "read")
    return (
      <svg viewBox="0 0 38 13" width="26" height="10">
        <polygon points="2,1 4,0 15,12 13,13" fill="#257830"/>
        <polygon points="10,1 12,0 25,12 23,13" fill="#257830"/>
        <polygon points="18,1 20,0 33,12 31,13" fill="#5ec46e"/>
      </svg>
    );
  return null;
}

/* ─── LOGO ────────────────────────────────────────────────────────────────── */
function ObscuraLogo({ size = 32 }: { size?: number }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 500" width={size} height={size * 1.25}>
      <defs>
        <clipPath id="ec-chat">
          <path d="M 192,48 L 228,76 L 264,62 L 285,102 L 277,150 L 260,183 L 234,208 L 214,235 L 226,272 L 248,312 L 265,354 L 272,388 L 250,370 L 224,344 L 204,368 L 192,394 L 174,374 L 175,344 L 155,361 L 143,388 L 123,368 L 127,335 L 106,352 L 93,376 L 78,356 L 84,319 L 79,289 L 75,260 L 65,242 L 52,225 L 68,208 L 93,197 L 116,182 L 136,163 L 149,140 L 160,115 L 169,89 L 177,67 Z"/>
        </clipPath>
      </defs>
      <path d="M 192,48 L 228,76 L 264,62 L 285,102 L 277,150 L 260,183 L 234,208 L 214,235 L 226,272 L 248,312 L 265,354 L 272,388 L 250,370 L 224,344 L 204,368 L 192,394 L 174,374 L 175,344 L 155,361 L 143,388 L 123,368 L 127,335 L 106,352 L 93,376 L 78,356 L 84,319 L 79,289 L 75,260 L 65,242 L 52,225 L 68,208 L 93,197 L 116,182 L 136,163 L 149,140 L 160,115 L 169,89 L 177,67 Z" fill="#257830"/>
      <g clipPath="url(#ec-chat)">
        <g transform="rotate(-38, 175, 370)">
          <rect x="-60" y="238" width="480" height="20" fill="#0b2210"/>
          <rect x="-60" y="268" width="480" height="20" fill="#0b2210"/>
          <rect x="-60" y="298" width="480" height="20" fill="#0b2210"/>
          <rect x="-60" y="328" width="480" height="20" fill="#0b2210"/>
        </g>
      </g>
      <polygon points="168,106 202,120 186,150 153,136" fill="#5ec46e"/>
    </svg>
  );
}

/* ─── AVATAR ──────────────────────────────────────────────────────────────── */
function Avatar({ name, tier, size = 40 }: { name: string; tier?: number; size?: number }) {
  const t = TIER[(tier || 1) as keyof typeof TIER];
  const letter = (name?.[0] || "?").toUpperCase();
  return (
    <div style={{
      width: size, height: size, borderRadius: "50%",
      background: `${t.color}22`, border: `2px solid ${t.color}55`,
      display: "flex", alignItems: "center", justifyContent: "center",
      color: t.color, fontWeight: 700, fontSize: size * 0.4,
      flexShrink: 0, position: "relative",
    }}>
      {letter}
    </div>
  );
}

/* ─── ANA SAYFA ───────────────────────────────────────────────────────────── */
export default function ChatPage() {
  const router = useRouter();
  const { user, setUser, conversations, setConversations,
          activeConvId, setActiveConv, messages, addMessages,
          addMessage, updateMsgStatus, setWS } = useStore();

  const [msgText, setMsgText]     = useState("");
  const [typing, setTyping]       = useState(false);
  const [search, setSearch]       = useState("");
  const [loading, setLoading]     = useState(true);
  const [creditInfo, setCreditInfo] = useState<any>(null);
  const [showProfile, setShowProfile] = useState(false);
  const messagesEndRef            = useRef<HTMLDivElement>(null);

  /* ── BAŞLANGIÇ ─────────────────────────────────────────────────────────── */
  useEffect(() => {
    const token = localStorage.getItem("obscura_token");
    if (!token) { router.replace("/login"); return; }

    (async () => {
      try {
        const [me, convs, credit] = await Promise.all([
          api.getMe(), api.getConversations(), api.getCreditScore(),
        ]);
        setUser(me);
        setConversations(convs || []);
        setCreditInfo(credit);
      } catch { router.replace("/login"); }
      finally { setLoading(false); }
    })();

    const ws = createWS(token, (msg) => {
      if (msg.type === "new_message") addMessage(msg.payload);
      if (msg.type === "message_delivered") updateMsgStatus(msg.payload.msg_id, "delivered");
      if (msg.type === "read_receipt")      updateMsgStatus(msg.payload.msg_id, "read");
    });
    setWS(ws);
    return () => ws.close();
  }, []);

  /* ── AKTİF KONUŞMA MESAJLARI ───────────────────────────────────────────── */
  useEffect(() => {
    if (!activeConvId) return;
    api.getMessages(activeConvId).then((msgs) => addMessages(activeConvId, msgs || []));
  }, [activeConvId]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, activeConvId]);

  /* ── MESAJ GÖNDER ─────────────────────────────────────────────────────── */
  async function sendMsg(e: React.FormEvent) {
    e.preventDefault();
    if (!msgText.trim() || !activeConvId || !user) return;
    const conv = conversations.find((c) => c.id === activeConvId);
    if (!conv) return;

    const tempId = `temp-${Date.now()}`;
    const tempMsg = {
      id: tempId, conv_id: activeConvId, from_did: user.did,
      to_did: conv.peer_did || activeConvId, type: "text",
      ciphertext: msgText, status: "sending",
      sent_at: new Date().toISOString(), is_group: conv.is_group,
    };
    addMessage(tempMsg as any);
    setMsgText("");

    try {
      const res = await api.sendMessage({
        to_id: conv.peer_did || activeConvId,
        type: "text", ciphertext: msgText,
        is_group: conv.is_group,
      });
      updateMsgStatus(tempId, res.status || "sent");
      const updatedConvs = await api.getConversations();
      setConversations(updatedConvs || []);
    } catch { updateMsgStatus(tempId, "failed"); }
  }

  /* ── AKTİF KONUŞMA ────────────────────────────────────────────────────── */
  const activeConv = conversations.find((c) => c.id === activeConvId);
  const activeMessages = activeConvId ? (messages[activeConvId] || []) : [];
  const filteredConvs = conversations.filter((c) =>
    c.name?.toLowerCase().includes(search.toLowerCase())
  );

  if (loading) return (
    <div style={{ display: "flex", height: "100vh", background: "#0e0e0e",
      alignItems: "center", justifyContent: "center" }}>
      <ObscuraLogo size={48} />
    </div>
  );

  return (
    <div style={{ display: "flex", height: "100vh", background: "#0e0e0e", overflow: "hidden" }}>

      {/* ═══════════════════ SOL PANEL ═══════════════════ */}
      <div style={{
        width: 300, background: "#111", borderRight: "1px solid #1e1e1e",
        display: "flex", flexDirection: "column", flexShrink: 0,
      }}>
        {/* Üst bar */}
        <div style={{
          padding: "12px 16px", borderBottom: "1px solid #1e1e1e",
          display: "flex", alignItems: "center", gap: 10,
        }}>
          <ObscuraLogo size={28} />
          <span style={{ color: "#f0f0f0", fontWeight: 700, letterSpacing: 2, fontSize: 13 }}>
            OBSCURA
          </span>
          <div style={{ flex: 1 }} />
          {/* Kredi göstergesi */}
          {creditInfo && (
            <div
              onClick={() => setShowProfile(!showProfile)}
              style={{
                background: "#1a1a1a", border: `1px solid ${TIER[(user?.tier || 1) as keyof typeof TIER].color}44`,
                borderRadius: 8, padding: "3px 8px", cursor: "pointer",
                display: "flex", alignItems: "center", gap: 4,
              }}
              title={`Kredi: ${creditInfo.score} | ${creditInfo.tier_name}`}
            >
              <span style={{ fontSize: 12 }}>{TIER[(user?.tier || 1) as keyof typeof TIER].dot}</span>
              <span style={{ color: TIER[(user?.tier || 1) as keyof typeof TIER].color, fontSize: 11, fontWeight: 600 }}>
                {Math.round(creditInfo.score)}
              </span>
            </div>
          )}
        </div>

        {/* Arama */}
        <div style={{ padding: "8px 12px", borderBottom: "1px solid #1e1e1e" }}>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="🔍 Ara..."
            style={{
              width: "100%", background: "#1a1a1a", border: "none",
              borderRadius: 8, padding: "7px 12px", color: "#f0f0f0",
              fontSize: 13, outline: "none",
            }}
          />
        </div>

        {/* Konuşma listesi */}
        <div style={{ flex: 1, overflowY: "auto" }}>
          {filteredConvs.length === 0 && (
            <div style={{ color: "#333", fontSize: 12, textAlign: "center", padding: 24 }}>
              Henüz konuşma yok
            </div>
          )}
          {filteredConvs.map((conv) => {
            const isActive = conv.id === activeConvId;
            const tierKey = (conv.peer_tier || 1) as keyof typeof TIER;
            return (
              <div
                key={conv.id}
                onClick={() => setActiveConv(conv.id)}
                style={{
                  display: "flex", alignItems: "center", gap: 12,
                  padding: "10px 16px", cursor: "pointer",
                  background: isActive ? "#1a2e1a" : "transparent",
                  borderLeft: isActive ? "3px solid #257830" : "3px solid transparent",
                  transition: "background 0.1s",
                }}
                onMouseEnter={(e) => { if (!isActive) e.currentTarget.style.background = "#161616"; }}
                onMouseLeave={(e) => { if (!isActive) e.currentTarget.style.background = "transparent"; }}
              >
                <Avatar name={conv.name || "?"} tier={conv.peer_tier} size={42} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <span style={{ color: "#f0f0f0", fontWeight: 500, fontSize: 14,
                      overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {conv.name || conv.peer_name || "Bilinmiyor"}
                    </span>
                    {conv.last_msg_at && (
                      <span style={{ color: "#444", fontSize: 10, flexShrink: 0, marginLeft: 4 }}>
                        {formatTime(conv.last_msg_at)}
                      </span>
                    )}
                  </div>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 2 }}>
                    <span style={{ color: "#555", fontSize: 12,
                      overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {conv.last_msg_text || ""}
                    </span>
                    {conv.unread_count > 0 && (
                      <span style={{
                        background: "#257830", color: "#fff", fontSize: 11,
                        borderRadius: 10, padding: "1px 6px", marginLeft: 4, flexShrink: 0,
                      }}>
                        {conv.unread_count}
                      </span>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>

        {/* Alt kullanıcı bilgisi */}
        <div style={{
          padding: "10px 16px", borderTop: "1px solid #1e1e1e",
          display: "flex", alignItems: "center", gap: 10,
        }}>
          <Avatar name={user?.display_name || "?"} tier={user?.tier} size={34} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ color: "#f0f0f0", fontSize: 13, fontWeight: 500,
              overflow: "hidden", textOverflow: "ellipsis" }}>
              {user?.display_name}
            </div>
            <div style={{ color: "#444", fontSize: 11 }}>@{user?.username}</div>
          </div>
          <button
            onClick={() => { localStorage.clear(); router.replace("/login"); }}
            style={{ background: "none", border: "none", color: "#444",
              cursor: "pointer", fontSize: 16, padding: 4 }}
            title="Çıkış"
          >⏻</button>
        </div>
      </div>

      {/* ═══════════════════ SAĞ PANEL ═══════════════════ */}
      {activeConv ? (
        <div style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>

          {/* Sohbet başlığı */}
          <div style={{
            padding: "10px 20px", background: "#111", borderBottom: "1px solid #1e1e1e",
            display: "flex", alignItems: "center", gap: 12,
          }}>
            <Avatar name={activeConv.name || "?"} tier={activeConv.peer_tier} size={36} />
            <div>
              <div style={{ color: "#f0f0f0", fontWeight: 600 }}>
                {activeConv.name || activeConv.peer_name}
              </div>
              <div style={{
                fontSize: 11, color: TIER[(activeConv.peer_tier || 1) as keyof typeof TIER].color
              }}>
                {TIER[(activeConv.peer_tier || 1) as keyof typeof TIER].dot}{" "}
                {TIER[(activeConv.peer_tier || 1) as keyof typeof TIER].name} Üye
              </div>
            </div>
            <div style={{ flex: 1 }} />
            <div style={{ display: "flex", gap: 8 }}>
              <IconBtn title="Sesli Arama">📞</IconBtn>
              <IconBtn title="Görüntülü Arama">📹</IconBtn>
              <IconBtn title="Ara">🔍</IconBtn>
            </div>
          </div>

          {/* Mesajlar */}
          <div style={{
            flex: 1, overflowY: "auto", padding: "16px 20px",
            display: "flex", flexDirection: "column", gap: 4,
          }}>
            {activeMessages.length === 0 && (
              <div style={{ color: "#2a2a2a", fontSize: 13, textAlign: "center", marginTop: 60 }}>
                🔐 Mesajlar uçtan uca şifreli
              </div>
            )}
            {activeMessages.map((msg, i) => {
              const isMine = msg.from_did === user?.did;
              const showDate = i === 0 || !sameDay(activeMessages[i-1]?.sent_at, msg.sent_at);
              return (
                <div key={msg.id}>
                  {showDate && (
                    <div style={{ textAlign: "center", color: "#333", fontSize: 11, margin: "12px 0 4px" }}>
                      {formatDate(msg.sent_at)}
                    </div>
                  )}
                  <div style={{
                    display: "flex",
                    justifyContent: isMine ? "flex-end" : "flex-start",
                    marginBottom: 2,
                  }}>
                    <div style={{
                      maxWidth: "65%", minWidth: 80,
                      background: isMine ? "#1a3d1e" : "#1e1e1e",
                      borderRadius: isMine ? "12px 3px 12px 12px" : "3px 12px 12px 12px",
                      padding: "7px 10px",
                      opacity: msg.status === "sending" ? 0.7 : 1,
                    }}>
                      <div style={{ color: "#f0f0f0", fontSize: 14, lineHeight: 1.4, wordBreak: "break-word" }}>
                        {msg.ciphertext}
                      </div>
                      <div style={{ display: "flex", alignItems: "center", justifyContent: "flex-end",
                        gap: 3, marginTop: 2 }}>
                        <span style={{ color: "#444", fontSize: 10 }}>
                          {formatTime(msg.sent_at)}
                        </span>
                        {isMine && <StatusIcon status={msg.status} />}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
            <div ref={messagesEndRef} />
          </div>

          {/* Mesaj kutusu */}
          <form onSubmit={sendMsg} style={{
            padding: "10px 16px", background: "#111", borderTop: "1px solid #1e1e1e",
            display: "flex", gap: 8, alignItems: "center",
          }}>
            <IconBtn title="Ekle" type="button">📎</IconBtn>
            <input
              value={msgText}
              onChange={(e) => setMsgText(e.target.value)}
              placeholder="Mesaj yaz..."
              style={{
                flex: 1, background: "#1a1a1a", border: "1px solid #2a2a2a",
                borderRadius: 20, padding: "8px 16px", color: "#f0f0f0",
                fontSize: 14, outline: "none",
              }}
            />
            <IconBtn title="Emoji" type="button">😊</IconBtn>
            <button type="submit" disabled={!msgText.trim()} style={{
              background: msgText.trim() ? "#257830" : "#1a1a1a",
              border: "none", borderRadius: "50%", width: 36, height: 36,
              color: "#fff", cursor: msgText.trim() ? "pointer" : "default",
              fontSize: 16, transition: "background 0.15s", flexShrink: 0,
            }}>
              ➤
            </button>
          </form>
        </div>
      ) : (
        /* Boş durum */
        <div style={{
          flex: 1, display: "flex", flexDirection: "column",
          alignItems: "center", justifyContent: "center", gap: 16,
        }}>
          <ObscuraLogo size={80} />
          <div style={{ color: "#f0f0f0", fontSize: 20, fontWeight: 700, letterSpacing: 3 }}>
            OBSCURA
          </div>
          <div style={{ color: "#333", fontSize: 13, textAlign: "center", maxWidth: 280 }}>
            Uçtan uca şifreli mesajlaşma<br />
            Zero-knowledge kimlik doğrulama
          </div>
          {creditInfo && (
            <div style={{
              background: "#161616", border: "1px solid #1e1e1e",
              borderRadius: 12, padding: "12px 20px", textAlign: "center",
            }}>
              <div style={{ color: TIER[(user?.tier || 1) as keyof typeof TIER].color, fontWeight: 700 }}>
                {TIER[(user?.tier || 1) as keyof typeof TIER].dot} {creditInfo.tier_name} Katman
              </div>
              <div style={{ color: "#444", fontSize: 12, marginTop: 4 }}>
                Kredi Puanı: {Math.round(creditInfo.score)} / 100
              </div>
              <div style={{
                marginTop: 8, height: 4, background: "#1a1a1a", borderRadius: 2, width: 160,
              }}>
                <div style={{
                  height: "100%", borderRadius: 2,
                  background: TIER[(user?.tier || 1) as keyof typeof TIER].color,
                  width: `${Math.max(0, Math.min(100, (creditInfo.score + 20) / 1.2))}%`,
                }} />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function IconBtn({ children, title, type = "button" as any, ...props }: any) {
  return (
    <button type={type} title={title} style={{
      background: "none", border: "none", color: "#555", cursor: "pointer",
      fontSize: 18, padding: "4px 6px", borderRadius: 8,
      transition: "color 0.15s", flexShrink: 0,
    }}
    onMouseEnter={(e) => e.currentTarget.style.color = "#f0f0f0"}
    onMouseLeave={(e) => e.currentTarget.style.color = "#555"}
    {...props}>{children}</button>
  );
}

function formatTime(iso: string): string {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  } catch { return ""; }
}

function formatDate(iso: string): string {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    const today = new Date();
    if (d.toDateString() === today.toDateString()) return "Bugün";
    const yesterday = new Date(today);
    yesterday.setDate(today.getDate() - 1);
    if (d.toDateString() === yesterday.toDateString()) return "Dün";
    return d.toLocaleDateString("tr-TR");
  } catch { return ""; }
}

function sameDay(a: string, b: string): boolean {
  if (!a || !b) return false;
  try { return new Date(a).toDateString() === new Date(b).toDateString(); }
  catch { return false; }
}
