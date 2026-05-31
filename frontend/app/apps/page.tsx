"use client";

import { useState, useEffect, useCallback } from "react";
import { Layers, Download, Play, Trash2, Loader2, RefreshCw, Lock, Search, Star, Zap, Shield, Wallet, MessageSquare, BarChart2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { api, type MiniApp } from "@/lib/api";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";

const TIER_STYLES: Record<number, { label: string; cls: string }> = {
  0: { label: "Herkes",   cls: "bg-[var(--surface-3)] text-[var(--text-3)] border border-[var(--border-2)]" },
  1: { label: "Temel",    cls: "bg-[var(--signal-muted)] text-[var(--signal)] border border-[rgba(77,168,255,0.2)]" },
  2: { label: "Güvenilir",cls: "bg-[rgba(167,139,250,0.1)] text-[#a78bfa] border border-[rgba(167,139,250,0.2)]" },
  3: { label: "Elit",     cls: "bg-[rgba(250,204,21,0.1)] text-[#facc15] border border-[rgba(250,204,21,0.2)]" },
  4: { label: "Elmas",    cls: "bg-[var(--accent-muted)] text-[var(--accent)] border border-[rgba(0,229,160,0.2)]" },
};

const APP_ICON_COLORS = [
  "from-[#00e5a0]/30 to-[#4da8ff]/20",
  "from-[#a78bfa]/30 to-[#4da8ff]/20",
  "from-[#facc15]/25 to-[#ff6b35]/20",
  "from-[#4da8ff]/30 to-[#00e5a0]/20",
  "from-[#ff4058]/20 to-[#a78bfa]/20",
];

const PLACEHOLDER_ICONS = [Zap, Shield, Wallet, MessageSquare, BarChart2, Star];

export default function AppsPage() {
  const { user } = useStore();
  const [apps, setApps] = useState<MiniApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [runResult, setRunResult] = useState<{ appId: string; output: string } | null>(null);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.listApps();
      setApps(res?.apps || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleInstall = async (app: MiniApp) => {
    setActionLoading(app.id);
    setError("");
    try {
      await api.installApp(app.id);
      await load();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setActionLoading(null);
    }
  };

  const handleUninstall = async (app: MiniApp) => {
    setActionLoading(app.id + "_uninstall");
    try {
      await api.uninstallApp(app.id);
      await load();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setActionLoading(null);
    }
  };

  const handleRun = async (app: MiniApp) => {
    setActionLoading(app.id + "_run");
    setRunResult(null);
    try {
      const result = await api.runApp(app.id);
      setRunResult({ appId: app.id, output: result.stdout || result.status || "Çalıştırıldı" });
    } catch (e: any) {
      setError(e.message);
    } finally {
      setActionLoading(null);
    }
  };

  const userTier = user?.tier || 0;

  const filtered = apps.filter(a => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      (a.manifest?.name || "").toLowerCase().includes(q) ||
      (a.manifest?.description || "").toLowerCase().includes(q)
    );
  });

  const featured = filtered.find(a => a.featured);
  const rest = filtered.filter(a => !a.featured);

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Header ── */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Mini Uygulamalar</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>Web3 uygulama mağazası</p>
          </div>
          <div className="flex items-center gap-2">
            {/* Tier badge */}
            <span className={cn("badge text-[10px] px-2.5", TIER_STYLES[userTier]?.cls ?? TIER_STYLES[0].cls)}>
              {TIER_STYLES[userTier]?.label ?? "Temel"}
            </span>
            <button
              onClick={load}
              aria-label="Yenile"
              className="btn-icon"
            >
              <RefreshCw size={15} className={cn(loading && "animate-spin")} />
            </button>
          </div>
        </div>

        {/* ── Search ── */}
        <div className="px-5 mb-4">
          <div className="relative">
            <Search size={14} className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none" style={{ color: "var(--text-3)" }} />
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Uygulama ara..."
              aria-label="Uygulama ara"
              className="field pl-9 h-10 text-sm"
            />
          </div>
        </div>

        {/* ── Error ── */}
        {error && (
          <div className="mx-5 mb-4 rounded-2xl border px-4 py-3 text-sm animate-slide-down"
            style={{ background: "rgba(255,64,88,0.06)", borderColor: "rgba(255,64,88,0.18)", color: "var(--error)" }}>
            {error}
          </div>
        )}

        {/* ── Loading skeleton ── */}
        {loading ? (
          <div className="px-5 space-y-3">
            <div className="rounded-3xl h-40 shimmer" style={{ background: "var(--surface-2)" }} />
            <div className="grid grid-cols-2 gap-3">
              {[0,1,2,3].map(i => (
                <div key={i} className="rounded-2xl h-36 shimmer" style={{ background: "var(--surface-2)" }} />
              ))}
            </div>
          </div>
        ) : filtered.length === 0 ? (
          /* ── Empty state ── */
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-16">
            <div className="w-16 h-16 rounded-3xl flex items-center justify-center" style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
              <Layers size={28} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>
                {search ? "Sonuç bulunamadı" : "Henüz uygulama yok"}
              </p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>
                {search ? `"${search}" için eşleşme yok` : "İlk mini uygulamayı yayınla."}
              </p>
            </div>
          </div>
        ) : (
          <div className="px-5 space-y-4">

            {/* ── Featured card ── */}
            {featured && (() => {
              const app = featured;
              const minTier = app.manifest?.minTier || 1;
              const canInstall = userTier >= Math.max(2, minTier);
              const isInstalling = actionLoading === app.id;
              const isRunning = actionLoading === app.id + "_run";
              const isUninstalling = actionLoading === app.id + "_uninstall";
              const colorIdx = 0;
              const PlaceholderIcon = PLACEHOLDER_ICONS[0];

              return (
                <div
                  className="relative rounded-3xl overflow-hidden"
                  style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
                >
                  {/* Gradient backdrop */}
                  <div className={cn("absolute inset-0 bg-gradient-to-br opacity-30", APP_ICON_COLORS[colorIdx])} />
                  <div className="relative p-5">
                    <div className="flex items-start gap-4">
                      {/* Icon */}
                      <div className={cn("w-14 h-14 rounded-2xl flex items-center justify-center flex-shrink-0 text-2xl bg-gradient-to-br", APP_ICON_COLORS[colorIdx])}
                        style={{ border: "1px solid var(--border-2)" }}>
                        {app.manifest?.icon ? (
                          <span>{app.manifest.icon}</span>
                        ) : (
                          <PlaceholderIcon size={22} style={{ color: "var(--accent)" }} />
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap mb-1">
                          <span className="font-display font-bold text-base" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>
                            {app.manifest?.name || "Uygulama"}
                          </span>
                          <span className="badge badge-accent text-[9px] tracking-wider">ÖNE ÇIKAN</span>
                        </div>
                        <p className="text-xs line-clamp-2 mb-3" style={{ color: "var(--text-2)" }}>
                          {app.manifest?.description || ""}
                        </p>
                        <div className="flex items-center gap-2 flex-wrap">
                          {app.manifest?.permissions?.slice(0, 2).map(p => (
                            <span key={p} className="text-[10px] rounded-full px-2 py-0.5" style={{ background: "var(--surface-3)", color: "var(--text-3)", border: "1px solid var(--border-1)" }}>{p}</span>
                          ))}
                          <span className={cn("badge text-[10px]", TIER_STYLES[minTier]?.cls ?? TIER_STYLES[0].cls)}>
                            {TIER_STYLES[minTier]?.label ?? `Tier ${minTier}`}
                          </span>
                        </div>
                      </div>
                    </div>
                    <div className="mt-4 flex items-center gap-2">
                      {app.installed ? (
                        <>
                          <button
                            onClick={() => handleRun(app)}
                            disabled={isRunning}
                            className="btn-primary h-10 px-5 text-xs flex-1"
                          >
                            {isRunning ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}
                            Çalıştır
                          </button>
                          <button
                            onClick={() => handleUninstall(app)}
                            disabled={isUninstalling}
                            className="btn-secondary h-10 px-4 text-xs"
                            style={{ color: "var(--error)", borderColor: "rgba(255,64,88,0.25)" }}
                          >
                            {isUninstalling ? <Loader2 size={12} className="animate-spin" /> : <Trash2 size={12} />}
                          </button>
                        </>
                      ) : (
                        <button
                          onClick={() => canInstall && handleInstall(app)}
                          disabled={!canInstall || isInstalling}
                          className={cn("h-10 px-5 text-xs flex-1 inline-flex items-center justify-center gap-2 rounded-full font-semibold transition-all duration-150",
                            canInstall
                              ? "btn-primary"
                              : "cursor-not-allowed opacity-50")}
                          style={!canInstall ? { background: "var(--surface-3)", color: "var(--text-3)" } : undefined}
                        >
                          {isInstalling ? <Loader2 size={12} className="animate-spin" /> : <Download size={12} />}
                          {canInstall ? "Yükle" : `Tier ${minTier} Gerekli`}
                        </button>
                      )}
                      <span className="text-[10px] ml-auto" style={{ color: "var(--text-3)" }}>{app.install_count || 0} kurulum</span>
                    </div>
                    {runResult?.appId === app.id && (
                      <div className="mt-3 rounded-2xl px-3 py-2.5 text-xs font-mono" style={{ background: "var(--surface-3)", color: "var(--text-1)", border: "1px solid var(--border-1)" }}>
                        {runResult.output}
                      </div>
                    )}
                  </div>
                </div>
              );
            })()}

            {/* ── Section label ── */}
            {rest.length > 0 && (
              <p className="section-label px-1">Tüm Uygulamalar</p>
            )}

            {/* ── App grid ── */}
            <div className="grid grid-cols-2 gap-3 pb-2">
              {rest.map((app, i) => {
                const minTier = app.manifest?.minTier || 1;
                const canInstall = userTier >= Math.max(2, minTier);
                const isInstalling = actionLoading === app.id;
                const isRunning = actionLoading === app.id + "_run";
                const isUninstalling = actionLoading === app.id + "_uninstall";
                const colorIdx = i % APP_ICON_COLORS.length;
                const PlaceholderIcon = PLACEHOLDER_ICONS[i % PLACEHOLDER_ICONS.length];

                return (
                  <div
                    key={app.id}
                    className="card-interactive flex flex-col p-4 gap-3"
                  >
                    {/* Icon + tier */}
                    <div className="flex items-start justify-between">
                      <div className={cn("w-12 h-12 rounded-2xl flex items-center justify-center text-xl bg-gradient-to-br flex-shrink-0", APP_ICON_COLORS[colorIdx])}
                        style={{ border: "1px solid var(--border-2)" }}>
                        {app.manifest?.icon ? (
                          <span>{app.manifest.icon}</span>
                        ) : (
                          <PlaceholderIcon size={20} style={{ color: "var(--accent)" }} />
                        )}
                      </div>
                      {minTier > 1 && (
                        <span className={cn("badge text-[9px]", TIER_STYLES[minTier]?.cls ?? TIER_STYLES[0].cls)}>
                          <Lock size={8} />
                          T{minTier}
                        </span>
                      )}
                    </div>

                    {/* Name + description */}
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-bold leading-tight mb-1 truncate" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>
                        {app.manifest?.name || "Uygulama"}
                      </p>
                      <p className="text-[11px] line-clamp-2 leading-relaxed" style={{ color: "var(--text-2)" }}>
                        {app.manifest?.description || ""}
                      </p>
                    </div>

                    {/* Action */}
                    {app.installed ? (
                      <div className="flex gap-1.5">
                        <button
                          onClick={() => handleRun(app)}
                          disabled={isRunning}
                          className="flex-1 h-8 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-95"
                          style={{ background: "var(--accent-muted)", color: "var(--accent)", border: "1px solid rgba(0,229,160,0.15)" }}
                        >
                          {isRunning ? <Loader2 size={11} className="animate-spin" /> : <Play size={11} />}
                          Çalıştır
                        </button>
                        <button
                          onClick={() => handleUninstall(app)}
                          disabled={isUninstalling}
                          className="w-8 h-8 rounded-xl flex items-center justify-center transition-all active:scale-95"
                          style={{ background: "rgba(255,64,88,0.08)", color: "var(--error)", border: "1px solid rgba(255,64,88,0.15)" }}
                          aria-label="Kaldır"
                        >
                          {isUninstalling ? <Loader2 size={11} className="animate-spin" /> : <Trash2 size={11} />}
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => canInstall && handleInstall(app)}
                        disabled={!canInstall || isInstalling}
                        className={cn(
                          "h-8 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-95",
                          canInstall
                            ? "text-[#020208]"
                            : "cursor-not-allowed"
                        )}
                        style={canInstall
                          ? { background: "var(--accent)" }
                          : { background: "var(--surface-3)", color: "var(--text-3)" }}
                      >
                        {isInstalling ? <Loader2 size={11} className="animate-spin" /> : <Download size={11} />}
                        {canInstall ? "Kur" : `T${minTier}+`}
                      </button>
                    )}

                    {runResult?.appId === app.id && (
                      <div className="rounded-xl px-2.5 py-1.5 text-[10px] font-mono" style={{ background: "var(--surface-3)", color: "var(--text-1)" }}>
                        {runResult.output}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </AppShell>
  );
}
