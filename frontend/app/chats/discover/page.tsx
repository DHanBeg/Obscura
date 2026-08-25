"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Search, Radio, Users2, Users, ChevronRight, Info } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { useToast } from "@/components/Toast";

interface DiscoverItem {
  id: string;
  name: string;
  conv_type: string;
  member_count: number;
}

const TYPE_META: Record<string, { icon: React.ReactNode; label: string }> = {
  channel: { icon: <Radio size={16} />, label: "Kanal" },
  community: { icon: <Users2 size={16} />, label: "Topluluk" },
  group: { icon: <Users size={16} />, label: "Grup" },
};

export default function DiscoverPage() {
  const router = useRouter();
  const { setConversations } = useStore();
  const { toast } = useToast();

  const [query, setQuery] = useState("");
  const [items, setItems] = useState<DiscoverItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [joiningId, setJoiningId] = useState<string | null>(null);

  const load = useCallback(async (q?: string) => {
    setLoading(true);
    try {
      const data = await api.discoverConversations(q);
      setItems(data || []);
    } catch {
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleJoin = useCallback(async (item: DiscoverItem) => {
    if (joiningId) return;
    setJoiningId(item.id);
    try {
      const res = await api.selfJoinConversation(item.id);
      // Üyelik listesini tazele ki /chats/[id] konuşmayı gerçekten bulabilsin.
      const convs = await api.getConversations().catch(() => null);
      if (convs) setConversations(convs);

      // KRİTİK (B7 Faz 1 guardrail — dürüstlük UI'da da korunmalı): self-join
      // SADECE HTTP/SQL üyeliği verir. MLS grup üyeliği (Welcome-based) ayrı
      // bir sistemdir ve burada senkronize edilmiyor. "Tam katıldın" gibi
      // davranmıyoruz — mls_synced:false'u kullanıcıya açıkça söylüyoruz.
      if (res.status === "already_member") {
        toast("Zaten bu konuşmanın üyesisiniz", "info");
      } else if (res.mls_synced === false) {
        toast("Katıldın — şifreli mesajlar ayrıca senkronize edilecek, hemen görünmeyebilir", "info");
      } else {
        toast("Katıldın", "success");
      }
      router.push(`/chats/${item.id}`);
    } catch (e) {
      toast(e instanceof Error ? e.message : "Katılım başarısız", "error");
    } finally {
      setJoiningId(null);
    }
  }, [joiningId, router, setConversations, toast]);

  return (
    <AppShell showBack title="Keşfet">
      <div className="flex flex-col h-full" style={{ background: "var(--bg)" }}>
        <div className="px-4 pt-3 pb-2 flex-shrink-0">
          <div className="relative">
            <Search
              size={15}
              className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none"
              style={{ color: "var(--text-3)" }}
              aria-hidden="true"
            />
            <input
              type="text"
              value={query}
              onChange={(e) => { setQuery(e.target.value); load(e.target.value || undefined); }}
              placeholder="Herkese açık kanal/topluluk ara..."
              className="field pl-9"
              aria-label="Herkese açık konuşma ara"
            />
          </div>
        </div>

        {/* Dürüstlük notu — sayfa genelinde, tek tek toast'a gerek kalmadan
            önceden görünür olsun diye */}
        <div
          className="mx-4 mb-2 flex items-start gap-2 px-3 py-2.5 rounded-2xl flex-shrink-0"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          <Info size={14} style={{ color: "var(--text-3)", marginTop: 2, flexShrink: 0 }} />
          <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
            Katılma anında üyeliğin eklenir; şifreli mesajlar ayrıca senkronize edilir ve hemen görünmeyebilir.
          </p>
        </div>

        <div className="flex-1 overflow-y-auto px-4 pb-4">
          {loading && (
            <div className="flex flex-col gap-2">
              {[0, 1, 2].map((i) => (
                <div key={i} className="h-16 rounded-2xl animate-pulse" style={{ background: "var(--surface-2)" }} />
              ))}
            </div>
          )}

          {!loading && items.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 text-center px-6">
              <Radio size={28} className="mb-3" style={{ color: "var(--text-3)" }} />
              <p className="text-sm font-medium" style={{ color: "var(--text-2)" }}>
                {query ? "Sonuç bulunamadı" : "Henüz herkese açık konuşma yok"}
              </p>
            </div>
          )}

          <div className="flex flex-col gap-2">
            {items.map((item) => {
              const meta = TYPE_META[item.conv_type] || TYPE_META.group;
              return (
                <button
                  key={item.id}
                  onClick={() => handleJoin(item)}
                  disabled={joiningId === item.id}
                  className="w-full flex items-center gap-3 px-4 py-3 rounded-2xl transition-colors duration-100 hover:bg-white/[0.025] text-left"
                  style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
                >
                  <div
                    className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                    style={{ background: "rgba(0,229,160,0.08)", border: "1px solid rgba(0,229,160,0.15)", color: "var(--em)" }}
                  >
                    {meta.icon}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold truncate" style={{ color: "var(--text-1)" }}>{item.name}</p>
                    <p className="text-xs" style={{ color: "var(--text-3)" }}>
                      {meta.label} · {item.member_count} üye
                    </p>
                  </div>
                  {joiningId === item.id ? (
                    <span
                      className="w-4 h-4 rounded-full border-2 animate-spin flex-shrink-0"
                      style={{ borderColor: "var(--border-1)", borderTopColor: "var(--em)" }}
                    />
                  ) : (
                    <ChevronRight size={14} style={{ color: "var(--text-3)" }} />
                  )}
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </AppShell>
  );
}
