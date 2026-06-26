"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Package,
  Plus,
  Pencil,
  Users,
  Loader2,
  RefreshCw,
  Zap,
  Shield,
  Wallet,
  MessageSquare,
  BarChart2,
  Star,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api, type MiniApp } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

// ── Types ──────────────────────────────────────────────────────────────────────

type AppStatus = "active" | "review" | "draft";

// ── Constants ──────────────────────────────────────────────────────────────────

const STATUS_STYLES: Record<AppStatus, { label: string; cls: string }> = {
  active: {
    label: "Aktif",
    cls: "badge-success",
  },
  review: {
    label: "İncelemede",
    cls: "badge-warning",
  },
  draft: {
    label: "Taslak",
    cls: "badge-neutral",
  },
};

const APP_ICON_COLORS = [
  "from-[#00e5a0]/30 to-[#4da8ff]/20",
  "from-[#a78bfa]/30 to-[#4da8ff]/20",
  "from-[#facc15]/25 to-[#ff6b35]/20",
  "from-[#4da8ff]/30 to-[#00e5a0]/20",
  "from-[#ff4058]/20 to-[#a78bfa]/20",
];

const PLACEHOLDER_ICONS = [Zap, Shield, Wallet, MessageSquare, BarChart2, Star];

// ── Helpers ────────────────────────────────────────────────────────────────────

function deriveStatus(app: MiniApp): AppStatus {
  // Derive status from available fields since MiniApp doesn't carry a status field.
  // installed acts as a proxy for active; otherwise treat as review if has install_count, else draft.
  if (app.installed) return "active";
  if ((app.install_count ?? 0) > 0) return "review";
  return "draft";
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

// ── App card ───────────────────────────────────────────────────────────────────

function MyAppCard({
  app,
  index,
  onEdit,
}: {
  app: MiniApp;
  index: number;
  onEdit: (id: string) => void;
}) {
  const status = deriveStatus(app);
  const { label: statusLabel, cls: statusCls } = STATUS_STYLES[status];
  const colorCls = APP_ICON_COLORS[index % APP_ICON_COLORS.length];
  const PlaceholderIcon = PLACEHOLDER_ICONS[index % PLACEHOLDER_ICONS.length];
  const dailyUsers = app.install_count ?? 0;

  return (
    <div
      className="card-interactive flex items-center gap-4 p-4"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
    >
      {/* Icon */}
      <div
        className={cn(
          "w-12 h-12 rounded-2xl flex items-center justify-center flex-shrink-0 bg-gradient-to-br text-xl",
          colorCls
        )}
        style={{ border: "1px solid var(--border-2)" }}
      >
        {app.manifest?.icon ? (
          <span>{app.manifest.icon}</span>
        ) : (
          <PlaceholderIcon size={20} style={{ color: "var(--accent)" }} />
        )}
      </div>

      {/* Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5 flex-wrap">
          <p
            className="text-sm font-bold truncate"
            style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
          >
            {app.manifest?.name || "Uygulama"}
          </p>
          <span className={cn("badge text-[10px]", statusCls)}>{statusLabel}</span>
        </div>
        <p
          className="text-xs line-clamp-1 mb-1.5"
          style={{ color: "var(--text-3)" }}
        >
          {app.manifest?.description || "Açıklama yok"}
        </p>
        <div className="flex items-center gap-1" style={{ color: "var(--text-3)" }}>
          <Users size={11} />
          <span className="text-[11px]">{formatCount(dailyUsers)} günlük kullanıcı</span>
        </div>
      </div>

      {/* Edit button */}
      <button
        onClick={() => onEdit(app.id)}
        className="btn-icon flex-shrink-0"
        aria-label="Düzenle"
        title="Düzenle"
      >
        <Pencil size={15} />
      </button>
    </div>
  );
}

// ── Empty state ────────────────────────────────────────────────────────────────

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <div className="flex-1 flex flex-col items-center justify-center gap-5 px-8 text-center py-16">
      <div
        className="w-16 h-16 rounded-3xl flex items-center justify-center"
        style={{
          background: "var(--surface-2)",
          border: "1px solid var(--border-1)",
        }}
      >
        <Package size={28} style={{ color: "var(--text-3)" }} />
      </div>
      <div>
        <p
          className="font-semibold text-sm mb-1"
          style={{ color: "var(--text-1)" }}
        >
          Henüz uygulama oluşturmadınız
        </p>
        <p className="text-xs" style={{ color: "var(--text-3)" }}>
          İlk mini uygulamanızı oluşturun ve mağazada yayınlayın.
        </p>
      </div>
      <button onClick={onNew} className="btn-primary h-11 px-6 text-sm flex items-center gap-2">
        <Plus size={16} />
        Yeni Uygulama Oluştur
      </button>
    </div>
  );
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function MyAppsPage() {
  const router = useRouter();
  const [apps, setApps] = useState<MiniApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const result = await (((api as Record<string, unknown>).getMyApps as (() => Promise<MiniApp[]>) | undefined)?.()
        ?? Promise.resolve<MiniApp[]>([]));
      setApps(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Uygulamalar yüklenemedi");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleEdit = (id: string) => {
    router.push(`/apps/${id}/edit`);
  };

  const handleNew = () => {
    router.push("/apps/new");
  };

  return (
    <AppShell showBack title="Uygulamalarım">
      <div className="flex flex-col h-full scroll-area pb-6">
        {/* Header */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Uygulamalarım</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
              Geliştirdiğin mini uygulamalar
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={load}
              aria-label="Yenile"
              className="btn-icon"
              disabled={loading}
            >
              <RefreshCw size={15} className={cn(loading && "animate-spin")} />
            </button>
            <button
              onClick={handleNew}
              aria-label="Yeni uygulama oluştur"
              className="btn-icon"
            >
              <Plus size={17} />
            </button>
          </div>
        </div>

        {/* Error */}
        {error && (
          <div
            className="mx-5 mb-4 rounded-2xl border px-4 py-3 text-sm"
            style={{
              background: "rgba(255,64,88,0.06)",
              borderColor: "rgba(255,64,88,0.18)",
              color: "var(--error)",
            }}
          >
            {error}
          </div>
        )}

        {/* Loading skeleton */}
        {loading ? (
          <div className="px-5 space-y-3">
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                className="rounded-2xl h-20 shimmer"
                style={{ background: "var(--surface-2)" }}
              />
            ))}
          </div>
        ) : apps.length === 0 ? (
          <EmptyState onNew={handleNew} />
        ) : (
          <div className="px-5 space-y-2.5">
            <p className="section-label px-1 mb-3">
              {apps.length} uygulama
            </p>
            {apps.map((app, i) => (
              <MyAppCard
                key={app.id}
                app={app}
                index={i}
                onEdit={handleEdit}
              />
            ))}

            {/* CTA at bottom */}
            <div className="pt-2 pb-4">
              <button
                onClick={handleNew}
                className="w-full h-11 rounded-2xl flex items-center justify-center gap-2 text-sm font-semibold transition-all hover:opacity-80"
                style={{
                  background: "var(--surface-2)",
                  border: "1px dashed var(--border-2)",
                  color: "var(--text-3)",
                }}
              >
                <Plus size={15} />
                Yeni Uygulama Ekle
              </button>
            </div>
          </div>
        )}

        {/* Floating loading overlay when refreshing non-empty list */}
        {loading && apps.length > 0 && (
          <div className="flex justify-center py-4">
            <Loader2 size={18} className="animate-spin" style={{ color: "var(--text-3)" }} />
          </div>
        )}
      </div>
    </AppShell>
  );
}
