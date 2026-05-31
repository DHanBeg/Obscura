"use client";

import { useEffect, useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { Search, MessageCircle, Lock, Plus, X } from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { ObscuraWordmark } from "@/components/ui/ObscuraLogo";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { Skeleton, ConvSkeleton } from "@/components/ui/Skeleton";
import { formatTime, truncate } from "@/lib/format";

// ── Active Users Strip ────────────────────────────────────────────────────────
function ActiveUsersStrip({ conversations, onlineUsers, onSelect }: {
  conversations: Array<{ id: string; peer_did?: string; peer_name?: string; name?: string; peer_tier?: number }>;
  onlineUsers: Set<string>;
  onSelect: (id: string) => void;
}) {
  const online = conversations.filter((c) => c.peer_did && onlineUsers.has(c.peer_did));
  if (online.length === 0) return null;

  return (
    <div className="flex-shrink-0 px-4 pb-3">
      <div
        className="flex gap-4 overflow-x-auto pb-1"
        style={{ scrollbarWidth: "none" }}
      >
        {online.map((c) => {
          const name = c.peer_name || c.name || "?";
          return (
            <button
              key={c.id}
              onClick={() => onSelect(c.id)}
              className="flex flex-col items-center gap-1.5 flex-shrink-0 group"
            >
              {/* Avatar with accent ring */}
              <div className="relative">
                <div
                  className="w-12 h-12 rounded-full p-[2px]"
                  style={{
                    background: "linear-gradient(135deg, var(--accent) 0%, var(--signal) 100%)",
                  }}
                >
                  <div className="w-full h-full rounded-full overflow-hidden" style={{ background: "var(--surface-3)" }}>
                    <Avatar name={name} tier={c.peer_tier} size="sm" className="w-full h-full" />
                  </div>
                </div>
                {/* Online pulse */}
                <span
                  className="absolute bottom-0 right-0 w-3 h-3 rounded-full border-2"
                  style={{
                    background: "var(--accent)",
                    borderColor: "var(--void)",
                  }}
                />
              </div>
              <span
                className="text-[10px] font-medium max-w-[48px] truncate"
                style={{ color: "var(--text-2)" }}
              >
                {name.split(" ")[0]}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── Skeleton Rows ─────────────────────────────────────────────────────────────
function SkeletonRows() {
  return (
    <div>
      {[0, 1, 2, 3, 4].map((i) => (
        <div
          key={i}
          className="flex items-center gap-3 px-4"
          style={{ minHeight: 72, borderBottom: "1px solid var(--border-1)", animationDelay: `${i * 60}ms` }}
        >
          {/* Avatar shimmer */}
          <div
            className="w-11 h-11 rounded-full flex-shrink-0 shimmer"
            style={{ background: "var(--surface-2)" }}
          />
          <div className="flex-1 space-y-2">
            <div
              className="h-3 rounded-full shimmer"
              style={{ width: `${55 + (i % 3) * 15}%`, background: "var(--surface-2)" }}
            />
            <div
              className="h-2.5 rounded-full shimmer"
              style={{ width: `${40 + (i % 4) * 10}%`, background: "var(--surface-2)" }}
            />
          </div>
          <div
            className="w-8 h-2 rounded-full shimmer flex-shrink-0"
            style={{ background: "var(--surface-2)" }}
          />
        </div>
      ))}
    </div>
  );
}

// ── Empty State ───────────────────────────────────────────────────────────────
function EmptyState({ searching }: { searching: boolean }) {
  return (
    <div className="flex flex-col items-center justify-center h-full pb-32 text-center px-6">
      <div
        className="w-20 h-20 rounded-full flex items-center justify-center mb-5"
        style={{
          background: "var(--surface-2)",
          border: "1px solid var(--border-1)",
        }}
      >
        <MessageCircle size={30} style={{ color: "var(--text-3)" }} />
      </div>
      <p
        className="font-semibold text-base mb-2"
        style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
      >
        {searching ? "Sonuç bulunamadı" : "Henüz sohbet yok"}
      </p>
      <p
        className="text-sm max-w-[220px] leading-relaxed"
        style={{ color: "var(--text-2)" }}
      >
        {searching
          ? "Farklı bir arama deneyin"
          : "Gravity well'e basın ve yeni bir sohbet başlatın"}
      </p>
    </div>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────
export default function ChatsPage() {
  const router = useRouter();
  const { conversations, setConversations, onlineUsers } = useStore();
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api.getConversations();
      setConversations(data || []);
    } catch {
      // silently fail — error boundary handles persistent failures
    } finally {
      setLoading(false);
    }
  }, [setConversations]);

  useEffect(() => {
    load();
  }, [load]);

  // Close search on Escape
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

  const filtered = conversations.filter((c) => {
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
        <div className="page-header flex-shrink-0 px-4">
          <ObscuraWordmark />
          <div className="flex items-center gap-1.5">
            <EncryptionBadge showLabel />
            <button
              onClick={() => setSearchOpen((v) => !v)}
              className="btn-icon"
              aria-label="Sohbet ara"
            >
              <Search size={18} />
            </button>
          </div>
        </div>

        {/* ── Search Bar ──────────────────────────────────────────────── */}
        {searchOpen && (
          <div className="px-3 pb-3 animate-slide-down flex-shrink-0" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div className="relative">
              <Search
                size={15}
                className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none"
                style={{ color: "var(--text-3)" }}
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
                  style={{ background: "var(--surface-3)", color: "var(--text-3)" }}
                  aria-label="Aramayı temizle"
                >
                  <X size={11} />
                </button>
              )}
            </div>
          </div>
        )}

        {/* ── Active Users Strip ──────────────────────────────────────── */}
        {!loading && !searchQuery && (
          <ActiveUsersStrip
            conversations={conversations}
            onlineUsers={onlineUsers}
            onSelect={(id) => router.push(`/chats/${id}`)}
          />
        )}

        {/* ── Conversation List ───────────────────────────────────────── */}
        <div className="flex-1 scroll-area pb-28">
          {loading && <SkeletonRows />}

          {!loading && filtered.length === 0 && (
            <EmptyState searching={!!searchQuery} />
          )}

          {!loading && filtered.length > 0 && (
            <div>
              {filtered.map((conv, idx) => {
                const isOnline = conv.peer_did ? onlineUsers.has(conv.peer_did) : false;
                const name = conv.name || conv.peer_name || "Bilinmiyor";
                const hasUnread = conv.unread_count > 0;

                return (
                  <button
                    key={conv.id}
                    onClick={() => router.push(`/chats/${conv.id}`)}
                    className="conv-row w-full group animate-in"
                    style={{ animationDelay: `${idx * 25}ms` }}
                    aria-label={`${name} sohbeti — ${conv.last_msg_text ? truncate(conv.last_msg_text.replace("__init__", ""), 48) : "Mesaj yok"}`}
                  >
                    {/* Avatar */}
                    <Avatar
                      name={name}
                      tier={conv.peer_tier}
                      online={conv.is_group ? undefined : isOnline}
                      size="md"
                    />

                    {/* Content */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-baseline justify-between mb-0.5">
                        <span
                          className={cn(
                            "text-[15px] truncate max-w-[170px]",
                            hasUnread ? "font-semibold" : "font-medium"
                          )}
                          style={{
                            fontFamily: "var(--font-display)",
                            color: hasUnread ? "var(--text-1)" : "var(--text-1)",
                          }}
                        >
                          {name}
                        </span>
                        <span
                          className="text-[11px] flex-shrink-0 ml-2"
                          style={{ color: "var(--text-3)" }}
                        >
                          {formatTime(conv.last_msg_at)}
                        </span>
                      </div>

                      <div className="flex items-center justify-between gap-2">
                        {/* Lock icon + preview */}
                        <div className="flex items-center gap-1.5 min-w-0">
                          <Lock
                            size={10}
                            className="flex-shrink-0"
                            style={{ color: "var(--text-3)" }}
                            aria-label="Uçtan uca şifreli"
                          />
                          <p
                            className={cn(
                              "text-[13px] truncate",
                              hasUnread ? "font-medium" : "font-normal"
                            )}
                            style={{ color: hasUnread ? "var(--text-2)" : "var(--text-3)" }}
                          >
                            {conv.last_msg_text
                              ? truncate(conv.last_msg_text.replace("__init__", ""), 50) || "Sohbet başladı"
                              : "Mesaj yok"}
                          </p>
                        </div>

                        {/* Unread badge */}
                        {hasUnread && (
                          <span
                            className="flex-shrink-0 min-w-[20px] h-5 px-1.5 rounded-full text-[11px] font-bold flex items-center justify-center"
                            style={{
                              background: "var(--accent)",
                              color: "var(--void)",
                              fontFamily: "var(--font-display)",
                            }}
                            aria-label={`${conv.unread_count} okunmamış mesaj`}
                          >
                            {conv.unread_count > 99 ? "99+" : conv.unread_count}
                          </span>
                        )}
                      </div>
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
