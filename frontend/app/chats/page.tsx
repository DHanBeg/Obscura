"use client";

import { useEffect, useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { Search, MessageCircle } from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { ObscuraWordmark } from "@/components/ui/ObscuraLogo";
import { EncryptionBadge } from "@/components/ui/EncryptionBadge";
import { Skeleton, ConvSkeleton } from "@/components/ui/Skeleton";
import { formatTime, truncate } from "@/lib/format";

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
    } catch {} finally { setLoading(false); }
  }, [setConversations]);

  useEffect(() => { load(); }, [load]);

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
        {/* Header */}
        <div className="flex items-center justify-between px-5 pt-5 pb-3 flex-shrink-0">
          <ObscuraWordmark />
          <div className="flex items-center gap-2">
            <EncryptionBadge showLabel />
            <button
              onClick={() => setSearchOpen((v) => !v)}
              className="btn-icon"
            >
              <Search size={18} />
            </button>
          </div>
        </div>

        {/* Search bar */}
        {searchOpen && (
          <div className="px-4 pb-3 animate-slide-down flex-shrink-0">
            <input
              autoFocus
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Sohbet ara..."
              className="field"
            />
          </div>
        )}

        {/* Conversation list */}
        <div className="flex-1 scroll-area pb-28">
          {loading && (
            <div className="space-y-0.5 px-2">
              {[0,1,2,3,4].map(i => <ConvSkeleton key={i} />)}
            </div>
          )}

          {!loading && filtered.length === 0 && (
            <div className="flex flex-col items-center justify-center h-full pb-32 text-center px-6">
              <div className="w-20 h-20 rounded-full bg-raised border border-border flex items-center justify-center mb-4">
                <MessageCircle size={32} className="text-dim" />
              </div>
              <p className="text-body font-medium mb-1">
                {searchQuery ? "Sonuç bulunamadı" : "Henüz sohbet yok"}
              </p>
              <p className="text-dim text-sm">
                {searchQuery ? "Farklı bir arama deneyin" : "Gravity well'e basın ve yeni sohbet başlatın"}
              </p>
            </div>
          )}

          {!loading && filtered.length > 0 && (
            <div className="px-2 space-y-0.5">
              {filtered.map((conv, idx) => {
                const isOnline = conv.peer_did ? onlineUsers.has(conv.peer_did) : false;
                const name = conv.name || conv.peer_name || "Bilinmiyor";

                return (
                  <button
                    key={conv.id}
                    onClick={() => router.push(`/chats/${conv.id}`)}
                    className="conv-card w-full group"
                    style={{ animationDelay: `${idx * 30}ms` }}
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
                        <span className={cn(
                          "font-medium text-sm truncate max-w-[180px]",
                          conv.unread_count > 0 ? "text-head" : "text-body"
                        )}>
                          {name}
                        </span>
                        <span className="text-[11px] text-dim flex-shrink-0 ml-2">
                          {formatTime(conv.last_msg_at)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between">
                        <p className={cn(
                          "text-xs truncate max-w-[200px]",
                          conv.unread_count > 0 ? "text-sub" : "text-dim"
                        )}>
                          {conv.last_msg_text
                            ? truncate(conv.last_msg_text.replace("__init__", ""), 48) || "Sohbet başladı"
                            : "Mesaj yok"}
                        </p>
                        {conv.unread_count > 0 && (
                          <span className="flex-shrink-0 ml-2 min-w-[20px] h-5 px-1.5 rounded-full bg-accent text-void text-[11px] font-semibold flex items-center justify-center">
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
