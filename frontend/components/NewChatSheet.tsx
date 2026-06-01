"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search, X, UserPlus, QrCode, Share2, Hash,
  ChevronRight, Users,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { Avatar } from "./ui/Avatar";
import { Skeleton } from "./ui/Skeleton";
import { useStore } from "@/lib/store";

interface User {
  id: string; did: string; username: string;
  display_name: string; tier: number; credit_score: number;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

type SheetTab = "search" | "qr" | "code";

// ── QR placeholder ─────────────────────────────────────────────────────────────

function QRSection({ did }: { did?: string }) {
  const [copied, setCopied] = useState(false);
  const share = () => {
    if (!did) return;
    if (navigator.share) {
      navigator.share({ title: "Obscura Bağlantım", text: did }).catch(() => {});
    } else {
      navigator.clipboard.writeText(did).catch(() => {});
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    }
  };

  return (
    <div className="flex flex-col items-center px-6 py-6 gap-5">
      {/* QR code placeholder */}
      <div
        className="w-48 h-48 rounded-2xl flex items-center justify-center relative overflow-hidden"
        style={{ background: "white" }}
        role="img"
        aria-label="QR kod — yakında"
      >
        {/* Pattern */}
        <div className="grid grid-cols-8 gap-1 p-4 opacity-20">
          {Array.from({ length: 64 }).map((_, i) => (
            <div
              key={i}
              className="w-3.5 h-3.5 rounded-[2px]"
              style={{ background: (i * 13 + 7) % 3 === 0 ? "#000" : "transparent" }}
            />
          ))}
        </div>
        <div
          className="absolute inset-0 flex flex-col items-center justify-center"
          style={{ background: "rgba(255,255,255,0.92)" }}
        >
          <QrCode size={36} style={{ color: "#000", opacity: 0.4 }} />
          <p className="text-[11px] mt-2 font-medium text-black/40">Yakında</p>
        </div>
      </div>

      <div className="text-center">
        <p className="text-sm font-semibold mb-1" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
          Bağlantı Kodunuzu Paylaşın
        </p>
        <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
          Arkadaşınızın kamerasını QR koduna tutması yeterli
        </p>
      </div>

      <button
        onClick={share}
        className="btn-primary gap-2"
        aria-label="Bağlantı kodunu paylaş"
      >
        <Share2 size={15} />
        {copied ? "Kopyalandı!" : "Kodumu Paylaş"}
      </button>
    </div>
  );
}

// ── Code entry ─────────────────────────────────────────────────────────────────

function CodeSection({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    if (!code.trim()) return;
    setLoading(true);
    setError("");
    try {
      const data = await api.searchUsers(code.trim());
      const user = (data || [])[0];
      if (user) {
        await api.sendMessage({ to_id: user.did, ciphertext: "__init__", type: "system" });
        const convs = await api.getConversations();
        const conv = (convs || []).find((c: { peer_did?: string }) => c.peer_did === user.did);
        onClose();
        if (conv) router.push(`/chats/${(conv as { id: string }).id}`);
      } else {
        setError("Kullanıcı bulunamadı");
      }
    } catch {
      setError("Bir hata oluştu");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="px-5 py-5">
      <p className="text-sm font-semibold mb-1" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
        Kullanıcı adı veya kod girin
      </p>
      <p className="text-xs mb-4" style={{ color: "var(--text-3)" }}>
        Karşı tarafın paylaştığı kullanıcı adını veya bağlantı kodunu girin
      </p>

      <div className="flex gap-2">
        <input
          value={code}
          onChange={(e) => { setCode(e.target.value); setError(""); }}
          placeholder="@kullanici veya kod"
          className="field flex-1"
          autoFocus
          onKeyDown={(e) => e.key === "Enter" && !loading && submit()}
          aria-label="Kullanıcı adı veya bağlantı kodu"
        />
        <button
          onClick={submit}
          disabled={loading || !code.trim()}
          className="btn-primary px-5 gap-2 flex-shrink-0"
          aria-label="Sohbet başlat"
        >
          {loading
            ? <span className="w-4 h-4 rounded-full border-2 border-void border-t-transparent animate-spin" aria-hidden="true" />
            : <ChevronRight size={15} />
          }
        </button>
      </div>

      {error && (
        <p className="mt-3 text-sm px-3 py-2 rounded-2xl"
          style={{ color: "var(--error)", background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)" }}>
          {error}
        </p>
      )}
    </div>
  );
}

// ── Recent contacts ─────────────────────────────────────────────────────────────

function RecentContacts({ onSelect }: { onSelect: (did: string) => void }) {
  const conversations = useStore((s) => s.conversations);
  const recent = conversations
    .filter((c) => !c.is_group && c.peer_did)
    .slice(0, 4);

  if (recent.length === 0) return null;

  return (
    <div className="px-5 pb-3">
      <p className="section-label mb-3">Son Konuşulanlar</p>
      <div className="flex gap-4 overflow-x-auto" style={{ scrollbarWidth: "none" }}>
        {recent.map((c) => {
          const name = c.peer_name || c.name || "?";
          return (
            <button
              key={c.id}
              onClick={() => c.peer_did && onSelect(c.peer_did)}
              className="flex flex-col items-center gap-1.5 flex-shrink-0"
              aria-label={`${name} ile sohbet başlat`}
            >
              <Avatar name={name} tier={c.peer_tier} size="md" />
              <span className="text-[11px] font-medium max-w-[52px] truncate" style={{ color: "var(--text-2)" }}>
                {name.split(" ")[0]}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── Main component ─────────────────────────────────────────────────────────────

export function NewChatSheet({ open, onClose }: Props) {
  const router = useRouter();
  const { user } = useStore();
  const [tab, setTab] = useState<SheetTab>("search");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<User[]>([]);
  const [loading, setLoading] = useState(false);
  const [empty, setEmpty] = useState(false);

  // Reset on close
  useEffect(() => {
    if (!open) { setQuery(""); setResults([]); setEmpty(false); setTab("search"); }
  }, [open]);

  // ESC to close
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

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

  const startChat = useCallback(async (userOrDid: User | string) => {
    const did = typeof userOrDid === "string" ? userOrDid : userOrDid.did;
    try {
      await api.sendMessage({ to_id: did, ciphertext: "__init__", type: "system" });
      const convs = await api.getConversations();
      const conv = (convs || []).find((c: { peer_did?: string }) => c.peer_did === did);
      onClose();
      if (conv) router.push(`/chats/${(conv as { id: string }).id}`);
    } catch {
      onClose();
      router.push("/chats");
    }
  }, [router, onClose]);

  if (!open) return null;

  const TABS = [
    { id: "search" as SheetTab, label: "Ara",       icon: <Search  size={14} /> },
    { id: "qr"     as SheetTab, label: "QR Tara",   icon: <QrCode  size={14} /> },
    { id: "code"   as SheetTab, label: "Kodu Gir",  icon: <Hash    size={14} /> },
  ];

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 z-40 animate-fade-in"
        style={{ background: "rgba(2,2,8,0.7)", backdropFilter: "blur(8px)" }}
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Sheet */}
      <div
        className="fixed inset-x-0 bottom-0 z-50 animate-slide-up"
        role="dialog"
        aria-modal="true"
        aria-label="Yeni sohbet başlat"
        style={{ maxHeight: "85vh" }}
      >
        <div
          className="flex flex-col overflow-hidden"
          style={{
            maxHeight: "85vh",
            background: "var(--surface)",
            borderTop: "1px solid var(--border-2)",
            borderRadius: "28px 28px 0 0",
            backdropFilter: "blur(40px)",
          }}
        >
          {/* Handle */}
          <div className="flex justify-center pt-3 pb-1 flex-shrink-0" aria-hidden="true">
            <div className="w-10 h-1 rounded-full" style={{ background: "var(--border-2)" }} />
          </div>

          {/* Header */}
          <div className="flex items-center justify-between px-5 pb-3 flex-shrink-0">
            <h2 className="text-lg font-bold" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>
              Yeni Sohbet
            </h2>
            <button
              onClick={onClose}
              className="btn-icon"
              aria-label="Kapat"
            >
              <X size={18} />
            </button>
          </div>

          {/* Tab bar */}
          <div className="flex gap-2 px-5 pb-3 flex-shrink-0">
            {TABS.map((t) => {
              const isActive = tab === t.id;
              return (
                <button
                  key={t.id}
                  onClick={() => setTab(t.id)}
                  role="tab"
                  aria-selected={isActive}
                  className={cn(
                    "flex items-center gap-1.5 h-8 px-3 rounded-xl text-xs font-semibold",
                    "transition-all duration-150 flex-shrink-0",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                  )}
                  style={{
                    background: isActive ? "var(--accent-muted)" : "var(--surface-2)",
                    color: isActive ? "var(--accent)" : "var(--text-3)",
                    border: isActive ? "1px solid rgba(0,229,160,0.2)" : "1px solid var(--border-1)",
                  }}
                >
                  <span aria-hidden="true">{t.icon}</span>
                  {t.label}
                </button>
              );
            })}
          </div>

          {/* Content */}
          <div className="flex-1 scroll-area pb-safe">

            {/* ── Search tab ── */}
            {tab === "search" && (
              <>
                <div className="px-4 pb-3 flex-shrink-0">
                  <div className="relative">
                    <Search
                      size={15}
                      className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none"
                      style={{ color: "var(--text-3)" }}
                      aria-hidden="true"
                    />
                    <input
                      autoFocus
                      type="text"
                      value={query}
                      onChange={(e) => search(e.target.value)}
                      placeholder="Kullanıcı ara..."
                      className="field pl-9"
                      aria-label="Kullanıcı ara"
                    />
                  </div>
                </div>

                {/* Recent contacts — shown when no search query */}
                {!query && (
                  <RecentContacts onSelect={(did) => startChat(did)} />
                )}

                {/* New group button */}
                {!query && (
                  <div className="px-4 pb-4">
                    <button
                      className="w-full flex items-center gap-3 px-4 py-3 rounded-2xl transition-colors duration-100 hover:bg-white/[0.025]"
                      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
                      aria-label="Yeni grup oluştur"
                    >
                      <div className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                        style={{ background: "rgba(0,229,160,0.08)", border: "1px solid rgba(0,229,160,0.15)" }}>
                        <Users size={16} style={{ color: "var(--accent)" }} />
                      </div>
                      <div className="flex-1 text-left">
                        <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                          Yeni Grup Oluştur
                        </p>
                        <p className="text-xs" style={{ color: "var(--text-3)" }}>Kişileri grup sohbetine ekleyin</p>
                      </div>
                      <ChevronRight size={14} style={{ color: "var(--text-3)" }} />
                    </button>
                  </div>
                )}

                {/* Search results */}
                {loading && (
                  <div className="space-y-1 px-3">
                    {[0, 1, 2].map((i) => (
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
                  <div className="flex flex-col items-center justify-center py-10 text-center px-6">
                    <UserPlus size={30} className="mb-3" style={{ color: "var(--text-3)" }} />
                    <p className="text-sm font-medium mb-1" style={{ color: "var(--text-2)" }}>Kullanıcı bulunamadı</p>
                    <p className="text-xs" style={{ color: "var(--text-3)" }}>Tam kullanıcı adını deneyin</p>
                  </div>
                )}

                {!loading && !empty && query.length >= 2 && results.map((u) => (
                  <button
                    key={u.did}
                    onClick={() => startChat(u)}
                    className="w-full flex items-center gap-3 px-5 py-3 transition-colors duration-100 hover:bg-white/[0.025] focus-visible:outline-none focus-visible:bg-white/[0.03]"
                    aria-label={`${u.display_name || u.username} ile sohbet başlat`}
                  >
                    <Avatar name={u.display_name || u.username} tier={u.tier} size="md" />
                    <div className="flex-1 min-w-0 text-left">
                      <p className="text-sm font-semibold truncate" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                        {u.display_name || u.username}
                      </p>
                      <p className="text-xs" style={{ color: "var(--text-3)" }}>@{u.username}</p>
                    </div>
                    <span className="text-xs font-semibold flex-shrink-0" style={{ color: "var(--accent)" }}>
                      Sohbet başlat
                    </span>
                  </button>
                ))}
              </>
            )}

            {/* ── QR tab ── */}
            {tab === "qr" && <QRSection did={user?.did} />}

            {/* ── Code tab ── */}
            {tab === "code" && <CodeSection onClose={onClose} />}
          </div>
        </div>
      </div>
    </>
  );
}
