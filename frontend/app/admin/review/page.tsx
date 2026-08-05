"use client";

import { useState, useEffect, useCallback } from "react";
import { ShieldAlert, Flag, Bot, CheckCircle2, AlertTriangle, Loader2, RefreshCw, ChevronLeft, ChevronRight, Ban } from "lucide-react";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

// İlke 5 (docs/spec/obscura_denetim_topluluk_katmani.md Bölüm 0): sistem
// ön eleyici, ciddi kararı insan verir. Bu sayfa o kararın verildiği yer.

interface ReviewQueueItem {
  id: string;
  report_id?: string;
  reason: string;
  status: string;
  source: "user_report" | "auto_scan";
  target_type?: string;
  target_id?: string;
  created_at: string;
  reporter_did?: string;
  reported_did?: string;
  category?: string;
  evidence_verified?: boolean;
}

const PAGE_SIZE = 20;

const SOURCE_META: Record<string, { label: string; icon: typeof Flag; color: string }> = {
  user_report: { label: "User Report", icon: Flag, color: "#facc15" },
  auto_scan: { label: "Auto Scan", icon: Bot, color: "#a78bfa" },
};

const CATEGORY_LABELS: Record<string, string> = {
  spam: "Spam",
  scam: "Dolandırıcılık",
  harassment: "Taciz",
  copyright: "Telif",
  illegal_sale: "Yasadışı Satış",
  child_safety: "Çocuk Güvenliği",
};

function ReviewCard({
  item,
  busy,
  onAction,
}: {
  item: ReviewQueueItem;
  busy: string | null;
  onAction: (action: "dismiss" | "confirm_remove" | "confirm_warn") => void;
}) {
  const meta = SOURCE_META[item.source] ?? SOURCE_META.auto_scan;
  const Icon = meta.icon;
  const isBusy = (a: string) => busy === item.id + a;
  const anyBusy = busy !== null && busy.startsWith(item.id);

  return (
    <div className="card" style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
      <div className="p-4">
        <div className="flex items-start gap-3 mb-3">
          <div
            className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5"
            style={{ background: meta.color + "15", border: `1px solid ${meta.color}30` }}
          >
            <Icon size={15} style={{ color: meta.color }} />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap mb-1">
              <span
                className="badge badge-neutral"
                style={{ color: meta.color, borderColor: meta.color + "30", background: meta.color + "10" }}
              >
                {meta.label}
              </span>
              {item.category && (
                <span className="badge badge-neutral">{CATEGORY_LABELS[item.category] ?? item.category}</span>
              )}
              {item.source === "user_report" && item.evidence_verified && (
                <span className="badge badge-success">Kanıt doğrulandı</span>
              )}
            </div>
            <p className="text-xs leading-relaxed" style={{ color: "var(--text-2)" }}>
              {item.reason}
            </p>
          </div>
        </div>

        {item.source === "user_report" ? (
          <div className="text-[11px] space-y-0.5 mb-3" style={{ color: "var(--text-3)" }}>
            <div>Şikayetçi: <code>{item.reporter_did ? item.reporter_did.slice(0, 24) + "…" : "—"}</code></div>
            <div>Hedef: <code>{item.reported_did ? item.reported_did.slice(0, 24) + "…" : "—"}</code></div>
          </div>
        ) : (
          <div className="text-[11px] space-y-0.5 mb-3" style={{ color: "var(--text-3)" }}>
            <div>Hedef tipi: <strong>{item.target_type || "—"}</strong></div>
            <div>Hedef ID: <code>{item.target_id ? item.target_id.slice(0, 24) + "…" : "—"}</code></div>
          </div>
        )}

        <div className="text-[10px] mb-3" style={{ color: "var(--text-3)" }}>
          {new Date(item.created_at).toLocaleString("tr-TR", { dateStyle: "short", timeStyle: "short" })}
        </div>

        <div className="flex gap-2">
          <button
            onClick={() => onAction("confirm_remove")}
            disabled={anyBusy}
            className="flex-1 h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
            style={{ background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.2)", color: "var(--error)" }}
          >
            {isBusy("confirm_remove") ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />}
            Kaldır
          </button>
          {item.source === "user_report" && (
            <button
              onClick={() => onAction("confirm_warn")}
              disabled={anyBusy}
              className="flex-1 h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
              style={{ background: "rgba(251,146,60,0.08)", border: "1px solid rgba(251,146,60,0.2)", color: "#fb923c" }}
            >
              {isBusy("confirm_warn") ? <Loader2 size={12} className="animate-spin" /> : <AlertTriangle size={12} />}
              Uyar
            </button>
          )}
          <button
            onClick={() => onAction("dismiss")}
            disabled={anyBusy}
            className="flex-1 h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
            style={{ background: "var(--surface-3)", border: "1px solid var(--border-2)", color: "var(--text-1)" }}
          >
            {isBusy("dismiss") ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />}
            Reddet
          </button>
        </div>
      </div>
    </div>
  );
}

export default function AdminReviewQueuePage() {
  const [items, setItems] = useState<ReviewQueueItem[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState("");
  const [loadError, setLoadError] = useState("");

  const load = useCallback(async (newOffset: number) => {
    setLoading(true);
    setLoadError("");
    try {
      const res = await api.adminListReviewQueue({ status: "pending", limit: PAGE_SIZE, offset: newOffset });
      setItems(res?.items || []);
      setTotal(res?.total || 0);
      setOffset(newOffset);
    } catch (e: any) {
      // Backend 403'te zaten "Bu işlem için yönetici yetkisi gerekli" gibi
      // net bir mesaj döndürüyor — ayrı bir client-side yetki kontrolü
      // yazmadan, gelen hatayı doğrudan gösteriyoruz.
      setLoadError(e.message || "İnceleme kuyruğu yüklenemedi");
      setItems([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleAction = async (item: ReviewQueueItem, action: "dismiss" | "confirm_remove" | "confirm_warn") => {
    setBusy(item.id + action);
    setActionError("");
    try {
      await api.adminResolveReviewQueue(item.id, action);
      setItems((prev) => prev.filter((i) => i.id !== item.id));
      setTotal((t) => Math.max(0, t - 1));
    } catch (e: any) {
      setActionError(e.message || "İşlem başarısız");
    } finally {
      setBusy(null);
    }
  };

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">İnceleme Kuyruğu</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
              İlke 5 — ciddi kararı insan verir
            </p>
          </div>
          <button onClick={() => load(offset)} aria-label="Yenile" className="btn-icon">
            <RefreshCw size={14} />
          </button>
        </div>

        {!loadError && (
          <div className="grid grid-cols-2 gap-2 px-5 mb-4">
            <div className="stat-box">
              <ShieldAlert size={13} style={{ color: "var(--text-3)" }} />
              <span className="stat-value text-lg">{total}</span>
              <span className="stat-label" style={{ fontSize: "9px" }}>Bekleyen</span>
            </div>
            <div className="stat-box">
              <Flag size={13} style={{ color: "var(--text-3)" }} />
              <span className="stat-value text-lg">{items.length}</span>
              <span className="stat-label" style={{ fontSize: "9px" }}>Bu sayfa</span>
            </div>
          </div>
        )}

        {actionError && (
          <div
            className="mx-5 mb-4 rounded-2xl px-4 py-3 text-sm animate-slide-down"
            style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}
          >
            {actionError}
          </div>
        )}

        {loading ? (
          <div className="px-5 space-y-3">
            {[0, 1, 2].map((i) => (
              <div key={i} className="rounded-2xl h-40 shimmer" style={{ background: "var(--surface-2)" }} />
            ))}
          </div>
        ) : loadError ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div
              className="w-14 h-14 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
            >
              <ShieldAlert size={24} style={{ color: "var(--error)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>
                Yetkiniz yok
              </p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>
                {loadError}
              </p>
            </div>
          </div>
        ) : items.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div
              className="w-14 h-14 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
            >
              <CheckCircle2 size={24} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>
                Bekleyen kayıt yok
              </p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>
                İnceleme kuyruğu temiz.
              </p>
            </div>
          </div>
        ) : (
          <>
            <div className="px-5 space-y-3">
              {items.map((item) => (
                <ReviewCard key={item.id} item={item} busy={busy} onAction={(action) => handleAction(item, action)} />
              ))}
            </div>

            {total > PAGE_SIZE && (
              <div className="flex items-center justify-between px-5 mt-4">
                <button
                  onClick={() => load(Math.max(0, offset - PAGE_SIZE))}
                  disabled={offset === 0 || loading}
                  className="btn-icon"
                  aria-label="Önceki sayfa"
                >
                  <ChevronLeft size={14} />
                </button>
                <span className="text-xs" style={{ color: "var(--text-3)" }}>
                  {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} / {total}
                </span>
                <button
                  onClick={() => load(offset + PAGE_SIZE)}
                  disabled={offset + PAGE_SIZE >= total || loading}
                  className="btn-icon"
                  aria-label="Sonraki sayfa"
                >
                  <ChevronRight size={14} />
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </AppShell>
  );
}
