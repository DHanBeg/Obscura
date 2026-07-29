"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import {
  Bot, Plus, MoreVertical, Trash2, ExternalLink, Copy, Check,
  Globe, Loader2, RefreshCw, Zap, X,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

interface BotRecord {
  id: string;
  name: string;
  description?: string;
  webhook_url: string;
  created_at: string;
  message_count?: number;
  active?: boolean;
}

function BotCard({
  bot,
  onDelete,
}: {
  bot: BotRecord;
  onDelete: (id: string) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const copyWebhook = () => {
    navigator.clipboard.writeText(bot.webhook_url).catch(() => {});
    setCopied(true);
    if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
    copiedTimerRef.current = setTimeout(() => setCopied(false), 2000);
  };

  useEffect(() => () => { if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current); }, []);

  const handleDelete = async () => {
    setDeleting(true);
    setMenuOpen(false);
    await new Promise((r) => setTimeout(r, 600));
    onDelete(bot.id);
  };

  return (
    <div
      className="card p-4"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
    >
      <div className="flex items-start gap-3">
        <div
          className="w-11 h-11 rounded-2xl flex items-center justify-center flex-shrink-0"
          style={{ background: "rgba(34,211,238,0.1)", color: "var(--cyan)" }}
        >
          <Bot size={20} />
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <p className="text-sm font-bold truncate" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
              {bot.name}
            </p>
            <span
              className="flex-shrink-0 inline-flex items-center gap-1 h-5 px-2 rounded-full text-[9px] font-bold"
              style={
                bot.active
                  ? { background: "var(--accent-muted)", color: "var(--accent)" }
                  : { background: "var(--surface-3)", color: "var(--text-3)" }
              }
            >
              <span
                className="w-1.5 h-1.5 rounded-full"
                style={{ background: bot.active ? "var(--accent)" : "var(--text-3)" }}
              />
              {bot.active ? "Aktif" : "Pasif"}
            </span>
          </div>

          {bot.description && (
            <p className="text-xs mb-2 line-clamp-1" style={{ color: "var(--text-3)" }}>
              {bot.description}
            </p>
          )}

          {/* Webhook URL */}
          <button
            onClick={copyWebhook}
            className="flex items-center gap-1.5 max-w-full transition-colors"
            aria-label="Webhook URL kopyala"
          >
            <Globe size={10} style={{ color: "var(--text-3)", flexShrink: 0 }} />
            <span
              className="text-[10px] font-mono truncate"
              style={{ color: "var(--text-3)", maxWidth: 180 }}
            >
              {bot.webhook_url}
            </span>
            {copied
              ? <Check size={10} style={{ color: "var(--accent)", flexShrink: 0 }} />
              : <Copy size={10} style={{ color: "var(--text-3)", flexShrink: 0 }} />
            }
          </button>

          {/* Stats */}
          {bot.message_count !== undefined && (
            <p className="text-[10px] mt-1" style={{ color: "var(--text-3)" }}>
              <Zap size={9} className="inline mr-0.5" />
              {bot.message_count} mesaj işlendi
            </p>
          )}
        </div>

        {/* Menu */}
        <div className="relative flex-shrink-0">
          <button
            onClick={() => setMenuOpen((v) => !v)}
            className="btn-icon w-8 h-8"
            aria-label="Bot seçenekleri"
          >
            <MoreVertical size={15} />
          </button>

          {menuOpen && (
            <>
              <div
                className="fixed inset-0 z-30"
                onClick={() => setMenuOpen(false)}
              />
              <div
                className="absolute right-0 top-9 z-40 rounded-2xl overflow-hidden"
                style={{
                  background: "var(--surface-2)",
                  border: "1px solid var(--border-2)",
                  minWidth: 160,
                  boxShadow: "0 8px 24px rgba(0,0,0,0.6)",
                  animation: "fadeIn 120ms ease both",
                }}
              >
                <button
                  onClick={copyWebhook}
                  className="flex items-center gap-2.5 px-3.5 py-2.5 text-xs font-medium w-full text-left hover:bg-white/[0.04] transition-colors"
                  style={{ borderBottom: "1px solid var(--border-1)", color: "var(--text-1)" }}
                >
                  <Copy size={13} style={{ color: "var(--text-3)" }} />
                  Webhook URL kopyala
                </button>
                <button
                  onClick={handleDelete}
                  disabled={deleting}
                  className="flex items-center gap-2.5 px-3.5 py-2.5 text-xs font-medium w-full text-left hover:bg-red-500/[0.06] transition-colors"
                  style={{ color: "var(--error)" }}
                >
                  {deleting
                    ? <Loader2 size={13} className="animate-spin" style={{ color: "var(--error)" }} />
                    : <Trash2 size={13} />
                  }
                  Botu Sil
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default function BotsPage() {
  const router = useRouter();
  const [bots, setBots] = useState<BotRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.listBots();
      setBots(Array.isArray(data) ? data : []);
    } catch {
      setBots([]);
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = (id: string) => {
    setBots((prev) => prev.filter((b) => b.id !== id));
  };

  return (
    <AppShell showBack title="Uygulamalar">
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* Header */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Botlarım</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
              Webhook tabanlı botlar
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={load} className="btn-icon" aria-label="Yenile">
              <RefreshCw size={15} className={cn(loading && "animate-spin")} />
            </button>
            <button
              onClick={() => router.push("/bots/new")}
              className="btn-accent-ghost flex items-center gap-1.5 h-8 px-3 rounded-xl text-xs font-semibold"
              aria-label="Bot oluştur"
            >
              <Plus size={14} />
              Yeni Bot
            </button>
          </div>
        </div>

        {/* SDK info card */}
        <div
          className="mx-5 mb-4 rounded-2xl p-4"
          style={{ background: "rgba(34,211,238,0.05)", border: "1px solid rgba(34,211,238,0.12)" }}
        >
          <p className="text-xs font-semibold mb-1" style={{ color: "var(--cyan)" }}>
            Bot nasıl çalışır?
          </p>
          <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
            Bir mesaj botuna ulaştığında sunucundaki webhook URL'ine <code style={{ color: "var(--accent)" }}>POST</code> atılır.
            Sunucu yanıtındaki <code style={{ color: "var(--accent)" }}>reply</code> alanı kullanıcıya gönderilir.
          </p>
          <div className="mt-3 rounded-xl p-2.5 text-[10px] font-mono overflow-x-auto"
            style={{ background: "var(--surface-3)", color: "var(--text-2)", whiteSpace: "pre" }}>
            {"// Webhook request body\n{ from_did, content, timestamp }\n\n// Expected response\n{ reply: \"Merhaba!\" }"}
          </div>
        </div>

        {loading ? (
          <div className="px-5 space-y-3">
            {[0, 1].map((i) => (
              <div key={i} className="rounded-2xl h-24 shimmer" style={{ background: "var(--surface-2)" }} />
            ))}
          </div>
        ) : bots.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div
              className="w-16 h-16 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
            >
              <Bot size={28} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>
                Henüz bot yok
              </p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>
                İlk botunu oluştur — webhook URL yeterli
              </p>
            </div>
            <button
              onClick={() => router.push("/bots/new")}
              className="btn-primary h-10 px-5 text-sm"
            >
              <Bot size={14} />
              Bot Oluştur
            </button>
          </div>
        ) : (
          <div className="px-5 space-y-3">
            {bots.map((bot) => (
              <BotCard key={bot.id} bot={bot} onDelete={handleDelete} />
            ))}
          </div>
        )}
      </div>
    </AppShell>
  );
}
