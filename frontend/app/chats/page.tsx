"use client";

import { useEffect, useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { Search, MessageCircle, X, Archive, Users, User, Plus, UserPlus } from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { ObscuraWordmark } from "@/components/ui/ObscuraLogo";
import { StatusPill } from "@/components/StatusPill";
import { GeometricAvatar } from "@/components/GeometricAvatar";
import { MessageStatusIcon, toStatusType } from "@/components/MessageStatus";
import { formatTime, truncate } from "@/lib/format";

// ── Folder Tabs ───────────────────────────────────────────────────────────────

type Folder = "all" | "personal" | "groups" | "archive";

const FOLDERS: { id: Folder; label: string; icon: React.ReactNode }[] = [
  { id: "all",      label: "Tümü",    icon: <MessageCircle size={12} strokeWidth={2} /> },
  { id: "personal", label: "Kişisel", icon: <User          size={12} strokeWidth={2} /> },
  { id: "groups",   label: "Gruplar", icon: <Users         size={12} strokeWidth={2} /> },
  { id: "archive",  label: "Arşiv",   icon: <Archive       size={12} strokeWidth={2} /> },
];

function FolderTabs({
  active,
  onChange,
  unreadTotal,
}: {
  active: Folder;
  onChange: (f: Folder) => void;
  unreadTotal: number;
}) {
  return (
    <div
      className="flex items-center gap-1 px-3 pb-2 flex-shrink-0 overflow-x-auto"
      style={{ scrollbarWidth: "none" }}
      role="tablist"
      aria-label="Sohbet klasörleri"
    >
      {FOLDERS.map((f) => {
        const isActive = f.id === active;
        return (
          <button
            key={f.id}
            role="tab"
            aria-selected={isActive}
            onClick={() => onChange(f.id)}
            className={cn(
              "inline-flex items-center gap-1.5 h-7 px-3 rounded-full text-[12px] font-semibold",
              "flex-shrink-0 transition-all duration-150",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--em)]",
            )}
            style={{
              background: isActive ? "var(--em)" : "var(--bg2)",
              color: isActive ? "var(--bg)" : "var(--t3)",
              border: `1px solid ${isActive ? "transparent" : "var(--border)"}`,
            }}
          >
            <span aria-hidden="true">{f.icon}</span>
            {f.label}
            {f.id === "all" && unreadTotal > 0 && (
              <span
                className="inline-flex items-center justify-center w-4 h-4 rounded-full text-[10px] font-bold"
                style={{
                  background: isActive ? "rgba(0,0,0,0.25)" : "var(--em)",
                  color: "var(--bg)",
                }}
              >
                {unreadTotal > 9 ? "9+" : unreadTotal}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}

// ── Skeleton Rows ─────────────────────────────────────────────────────────────

function SkeletonRows() {
  return (
    <div aria-busy="true" aria-label="Yükleniyor">
      {[0, 1, 2, 3, 4].map((i) => (
        <div
          key={i}
          className="flex items-center gap-3 px-4"
          style={{ minHeight: 72, borderBottom: "1px solid var(--border)" }}
        >
          <div className="w-11 h-11 rounded-full flex-shrink-0 shimmer" style={{ background: "var(--bg2)" }} />
          <div className="flex-1 space-y-2">
            <div className="h-3 rounded-full shimmer" style={{ width: `${55 + (i % 3) * 15}%`, background: "var(--bg2)" }} />
            <div className="h-2.5 rounded-full shimmer" style={{ width: `${40 + (i % 4) * 10}%`, background: "var(--bg2)" }} />
          </div>
          <div className="w-8 h-2 rounded-full shimmer flex-shrink-0" style={{ background: "var(--bg2)" }} />
        </div>
      ))}
    </div>
  );
}

// ── Empty State ───────────────────────────────────────────────────────────────

function EmptyState({ searching, onNewChat }: { searching: boolean; onNewChat?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center h-full pb-32 text-center px-6">
      <div
        className="w-20 h-20 rounded-full flex items-center justify-center mb-5"
        style={{ background: "var(--bg2)", border: "1px solid var(--border)" }}
      >
        <MessageCircle size={30} style={{ color: "var(--t3)" }} />
      </div>
      <p className="font-semibold text-base mb-2" style={{ color: "var(--t1)" }}>
        {searching ? "Sonuç bulunamadı" : "Henüz sohbet yok"}
      </p>
      <p className="text-sm max-w-[220px] leading-relaxed mb-5" style={{ color: "var(--t2)" }}>
        {searching
          ? "Farklı bir arama deneyin"
          : "Yeni bir sohbet başlatmak için aşağıdaki butona dokunun"}
      </p>
      {!searching && onNewChat && (
        <button
          onClick={onNewChat}
          className="btn-primary"
          aria-label="Yeni sohbet başlat"
        >
          <Plus size={16} />
          Yeni Sohbet
        </button>
      )}
    </div>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export default function ChatsPage() {
  const router = useRouter();
  const { conversations, setConversations, onlineUsers } = useStore();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [folder, setFolder] = useState<Folder>("all");

  const load = useCallback(async () => {
    setError(false);
    try {
      const data = await api.getConversations();
      setConversations(data || []);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [setConversations]);

  useEffect(() => { load(); }, [load]);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && searchOpen) {
        setSearchOpen(false);
        setSearchQuery("");
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [searchOpen]);

  const totalUnread = conversations.reduce((a, c) => a + c.unread_count, 0);

  const filtered = conversations.filter((c) => {
    if (folder === "groups" && !c.is_group) return false;
    if (folder === "personal" && c.is_group) return false;
    if (folder === "archive") return false;
    if (!searchQuery) return true;
    return (
      c.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (c.peer_name || "").toLowerCase().includes(searchQuery.toLowerCase())
    );
  });

  return (
    <AppShell>
      <div className="flex flex-col h-full">

        {/* ── Page Header ─────────────────────────────────────────────── */}
        <div className="flex-shrink-0 px-4 pt-3 pb-2 flex items-start justify-between"
          style={{ borderBottom: "1px solid var(--border)" }}>
          <div>
            <ObscuraWordmark />
          </div>
          <div className="flex items-center gap-2">
            <StatusPill />
            <button
              onClick={() => router.push("/chats/invites")}
              className="btn-icon"
              aria-label="Grup davetleri"
              title="Grup davetleri (yalnızca mobil'den kurulan gruplara katılma)"
            >
              <UserPlus size={18} />
            </button>
            <button
              onClick={() => setSearchOpen((v) => !v)}
              className="btn-icon"
              aria-label="Sohbet ara"
              aria-expanded={searchOpen}
            >
              <Search size={18} />
            </button>
          </div>
        </div>

        {/* ── Search Bar ──────────────────────────────────────────────── */}
        {searchOpen && (
          <div
            className="px-3 pb-3 pt-2 animate-slide-down flex-shrink-0"
            style={{ borderBottom: "1px solid var(--border)" }}
          >
            <div className="relative">
              <Search
                size={15}
                className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none"
                style={{ color: "var(--t3)" }}
                aria-hidden="true"
              />
              <input
                autoFocus
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Sohbet ara..."
                className="field pl-9 pr-9"
                aria-label="Sohbet ara"
              />
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 w-5 h-5 rounded-full flex items-center justify-center"
                  style={{ background: "var(--bg3)", color: "var(--t3)" }}
                  aria-label="Aramayı temizle"
                >
                  <X size={11} />
                </button>
              )}
            </div>
          </div>
        )}

        {/* ── Folder Tabs ─────────────────────────────────────────────── */}
        {!searchOpen && (
          <div className="pt-1 flex-shrink-0">
            <FolderTabs active={folder} onChange={setFolder} unreadTotal={totalUnread} />
          </div>
        )}

        {/* ── Error State ─────────────────────────────────────────────── */}
        {error && !loading && (
          <div className="mx-4 mt-4 flex-shrink-0">
            <div
              className="flex items-center justify-between gap-3 px-4 py-3 rounded-2xl"
              style={{ background: "var(--red-d)", border: "1px solid rgba(239,68,68,0.2)" }}
            >
              <p className="text-sm" style={{ color: "var(--red)" }}>
                Sohbetler yüklenemedi
              </p>
              <button
                onClick={load}
                className="text-xs font-semibold px-3 h-7 rounded-xl flex-shrink-0 transition-colors duration-100"
                style={{ background: "var(--red)", color: "white" }}
                aria-label="Yeniden dene"
              >
                Yeniden dene
              </button>
            </div>
          </div>
        )}

        {/* ── Conversation List ───────────────────────────────────────── */}
        <div className="flex-1 scroll-area" style={{ paddingBottom: "120px" }}>
          {loading && <SkeletonRows />}

          {!loading && folder === "archive" && (
            <div className="flex flex-col items-center justify-center h-full pb-32 text-center px-6">
              <Archive size={30} className="mb-4" style={{ color: "var(--t3)" }} />
              <p className="text-sm font-medium" style={{ color: "var(--t2)" }}>Arşiv boş</p>
              <p className="text-xs mt-1" style={{ color: "var(--t3)" }}>
                Arşivlenen sohbetler burada görünür
              </p>
            </div>
          )}

          {!loading && folder !== "archive" && filtered.length === 0 && (
            <EmptyState searching={!!searchQuery} />
          )}

          {!loading && folder !== "archive" && filtered.length > 0 && (
            <div>
              {filtered.map((conv, idx) => {
                const isOnline = conv.peer_did ? onlineUsers.has(conv.peer_did) : false;
                const name = conv.name || conv.peer_name || "Bilinmiyor";
                const hasUnread = conv.unread_count > 0;
                const isVerified = !conv.is_group;
                // Determine last message status icon
                const lastStatus = (conv as { last_msg_status?: string }).last_msg_status;
                const showStatus = lastStatus && !hasUnread;

                return (
                  <button
                    key={conv.id}
                    onClick={() => router.push(`/chats/${conv.id}`)}
                    className="w-full animate-in"
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "12px",
                      padding: "11px 18px",
                      borderBottom: "1px solid var(--border)",
                      cursor: "pointer",
                      background: "transparent",
                      transition: "background 0.12s",
                      animationDelay: `${idx * 25}ms`,
                      textAlign: "left",
                    }}
                    onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "rgba(255,255,255,0.02)"; }}
                    onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "transparent"; }}
                    aria-label={`${name} sohbeti — ${conv.last_msg_text ? truncate(conv.last_msg_text.replace("__init__", ""), 48) : "Mesaj yok"}`}
                  >
                    {/* Avatar with lock badge */}
                    <div className="relative flex-shrink-0">
                      <GeometricAvatar
                        username={name}
                        size={46}
                        online={conv.is_group ? undefined : isOnline}
                        showLock={false}
                      />
                      {/* Online dot — status role (C13-a FIX 2a) */}
                      {conv.peer_did && onlineUsers.has(conv.peer_did) && (
                        <span
                          aria-hidden="true"
                          style={{
                            position: "absolute",
                            bottom: 1,
                            right: 1,
                            width: 10,
                            height: 10,
                            borderRadius: "50%",
                            background: "var(--status)",
                            border: "1.5px solid var(--bg)",
                          }}
                        />
                      )}
                    </div>

                    {/* Info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center justify-between mb-0.5">
                        {/* Name + shield */}
                        <span
                          className="flex items-center gap-1 text-[14px] truncate max-w-[160px]"
                          style={{
                            fontWeight: hasUnread ? 600 : 500,
                            color: "var(--t1)",
                          }}
                        >
                          {name}
                        </span>
                        <span
                          className="text-[10px] flex-shrink-0 ml-2"
                          style={{ color: "var(--t3)" }}
                        >
                          {formatTime(conv.last_msg_at)}
                        </span>
                      </div>

                      {/* Preview row */}
                      <div className="flex items-center gap-1 min-w-0">
                        {showStatus && lastStatus && (
                          <span className="flex-shrink-0 flex items-center">
                            <MessageStatusIcon
                              status={toStatusType(lastStatus)}
                              size={13}
                            />
                          </span>
                        )}
                        <p
                          className="text-[12px] truncate flex-1"
                          style={{
                            color: hasUnread ? "var(--t2)" : "var(--t3)",
                            fontWeight: hasUnread ? 500 : 400,
                          }}
                        >
                          {conv.last_msg_text
                            ? truncate(conv.last_msg_text.replace("__init__", ""), 50) || "Sohbet başladı"
                            : "Mesaj yok"}
                        </p>
                      </div>

                      {/* Self-destruct tag (optional — show if conv has ttl) */}
                      {(conv as { ttl_seconds?: number }).ttl_seconds && (
                        <div className="mt-1">
                          <span className="destruct-tag">
                            ⏱ {(conv as { ttl_seconds?: number }).ttl_seconds}s
                          </span>
                        </div>
                      )}
                    </div>

                    {/* Right meta */}
                    <div className="flex flex-col items-end gap-1.5 flex-shrink-0">
                      {hasUnread && (
                        <span
                          className="min-w-[20px] h-5 px-1.5 rounded-full text-[11px] font-bold flex items-center justify-center"
                          style={{
                            background: "var(--em)",
                            color: "#000",
                          }}
                          aria-label={`${conv.unread_count} okunmamış mesaj`}
                        >
                          {conv.unread_count > 99 ? "99+" : conv.unread_count}
                        </span>
                      )}
                      {conv.is_group && (
                        <span
                          className="text-[8px] px-1.5 py-0.5 rounded-full font-bold"
                          style={{ background: "var(--em-d)", color: "var(--em)", border: "1px solid color-mix(in srgb, var(--em) 20%, transparent)" }}
                        >
                          GRUP
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </div>

      </div>
    </AppShell>
  );
}
