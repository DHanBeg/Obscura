"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Search, X, UserPlus } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { Avatar } from "./ui/Avatar";
import { Skeleton } from "./ui/Skeleton";
import { truncate } from "@/lib/format";

interface User {
  id: string; did: string; username: string;
  display_name: string; tier: number; credit_score: number;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

export function NewChatSheet({ open, onClose }: Props) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [empty, setEmpty] = useState(false);

  const search = useCallback(async (q: string) => {
    setQuery(q);
    if (q.length < 2) { setResults([]); setEmpty(false); return; }
    setLoading(true);
    try {
      const data = await api.searchUsers(q);
      setResults(data || []);
      setEmpty((data || []).length === 0);
    } catch {
      setResults([]);
    } finally { setLoading(false); }
  }, []);

  const startChat = useCallback(async (user: User) => {
    try {
      // Send a silent "handshake" message to create conversation
      await api.sendMessage({ to_id: user.did, ciphertext: "__init__", type: "system" });
      const convs = await api.getConversations();
      const conv = convs?.find((c: any) => c.peer_did === user.did);
      onClose();
      if (conv) router.push(`/chats/${conv.id}`);
    } catch (e: any) {
      // Conversation might already exist — just navigate to chats
      onClose();
      router.push("/chats");
    }
  }, [router, onClose]);

  if (!open) return null;

  return (
    <>
      {/* Overlay */}
      <div className="fixed inset-0 z-40 bg-void/60 backdrop-blur-sm animate-fade-in" onClick={onClose} />

      {/* Sheet */}
      <div className="fixed inset-x-0 bottom-0 z-50 animate-slide-up" style={{ maxHeight: "85vh" }}>
        <div className="glass-heavy border-t border-border rounded-t-3xl flex flex-col overflow-hidden" style={{ maxHeight: "85vh" }}>
          {/* Handle */}
          <div className="flex justify-center pt-3 pb-1 flex-shrink-0">
            <div className="w-10 h-1 rounded-full bg-muted" />
          </div>

          {/* Header */}
          <div className="flex items-center justify-between px-5 pb-4 flex-shrink-0">
            <h2 className="text-lg font-semibold text-head">Yeni sohbet</h2>
            <button onClick={onClose} className="btn-icon">
              <X size={18} />
            </button>
          </div>

          {/* Search */}
          <div className="px-4 pb-3 flex-shrink-0">
            <div className="relative">
              <Search size={16} className="absolute left-4 top-1/2 -translate-y-1/2 text-dim pointer-events-none" />
              <input
                autoFocus
                type="text"
                value={query}
                onChange={(e) => search(e.target.value)}
                placeholder="Kullanıcı ara..."
                className="field pl-10"
              />
            </div>
          </div>

          {/* Results */}
          <div className="flex-1 scroll-area px-2 pb-6">
            {loading && (
              <div className="space-y-1 px-2">
                {[0,1,2].map(i => (
                  <div key={i} className="flex items-center gap-3 px-2 py-3">
                    <Skeleton className="w-11 h-11 rounded-full flex-shrink-0" />
                    <div className="flex-1 space-y-2">
                      <Skeleton className="h-3.5 w-1/3" />
                      <Skeleton className="h-3 w-1/4" />
                    </div>
                  </div>
                ))}
              </div>
            )}

            {!loading && empty && (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <UserPlus size={32} className="text-dim mb-3" />
                <p className="text-sub text-sm">Kullanıcı bulunamadı</p>
                <p className="text-dim text-xs mt-1">Tam kullanıcı adını deneyin</p>
              </div>
            )}

            {!loading && !empty && results.length === 0 && query.length < 2 && (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Search size={32} className="text-dim mb-3" />
                <p className="text-sub text-sm">En az 2 karakter girin</p>
              </div>
            )}

            {results.map((user) => (
              <button
                key={user.did}
                onClick={() => startChat(user)}
                className="conv-card w-full text-left"
              >
                <Avatar name={user.display_name || user.username} tier={user.tier} />
                <div className="flex-1 min-w-0">
                  <p className="text-head text-sm font-medium truncate">
                    {user.display_name || user.username}
                  </p>
                  <p className="text-dim text-xs">@{user.username}</p>
                </div>
                <div className="text-xs text-accent font-medium">Sohbet başlat</div>
              </button>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}
