"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Scale, Clock, CheckCircle2, XCircle, Loader2, RefreshCw, Shield, Plus, X, Users, FileText, Coins, Zap, AlertTriangle, Timer } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

interface DAOProposal {
  id: string;
  title: string;
  description: string;
  category: string;
  proposer_did: string;
  status: string;
  yes_weight: number;
  no_weight: number;
  abstain_weight: number;
  veto_count: number;
  voting_ends_at: string;
  executable_at?: string;
  executed_at?: string;
}

const STATUS_STYLES: Record<string, string> = {
  active:     "badge-warning",
  passed:     "badge-signal",
  rejected:   "badge-error",
  timelocked: "badge-signal",
  executed:   "badge-success",
  vetoed:     "badge-error",
  draft:      "badge-neutral",
};

const STATUS_LABELS: Record<string, string> = {
  active:     "Oylamada",
  passed:     "Kabul",
  rejected:   "Red",
  timelocked: "Timelock",
  executed:   "Uygulandı",
  vetoed:     "Veto",
  draft:      "Taslak",
};

const CATEGORY_META: Record<string, { label: string; icon: typeof Scale; color: string }> = {
  param:     { label: "Parametre",  icon: FileText,      color: "#4da8ff" },
  protocol:  { label: "Protokol",   icon: Zap,           color: "#a78bfa" },
  treasury:  { label: "Hazine",     icon: Coins,         color: "#facc15" },
  guardian:  { label: "Guardian",   icon: Shield,        color: "#fb923c" },
  emergency: { label: "Acil",       icon: AlertTriangle, color: "#ff4058" },
};

function useCountdown(endIso?: string) {
  const [remaining, setRemaining] = useState("");
  useEffect(() => {
    if (!endIso) return;
    const tick = () => {
      const diff = new Date(endIso).getTime() - Date.now();
      if (diff <= 0) { setRemaining("Sona erdi"); return; }
      const h = Math.floor(diff / 3600000);
      const m = Math.floor((diff % 3600000) / 60000);
      const s = Math.floor((diff % 60000) / 1000);
      setRemaining(`${h > 0 ? h + "s " : ""}${m}d ${s}sn`);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [endIso]);
  return remaining;
}

function ProposalCard({
  p,
  actionBusy,
  onFinalize,
  onExecute,
  onVeto,
}: {
  p: DAOProposal;
  actionBusy: string | null;
  onFinalize: () => void;
  onExecute: () => void;
  onVeto: () => void;
}) {
  const countdown = useCountdown(p.status === "active" ? p.voting_ends_at : p.executable_at);
  const catMeta = CATEGORY_META[p.category] ?? CATEGORY_META.param;
  const CatIcon = catMeta.icon;

  const total = p.yes_weight + p.no_weight + p.abstain_weight;
  const yesPct = total > 0 ? Math.round((p.yes_weight / total) * 100) : 0;
  const noPct  = total > 0 ? Math.round((p.no_weight  / total) * 100) : 0;
  const absPct = total > 0 ? 100 - yesPct - noPct : 0;

  const busyId = p.id;

  return (
    <div
      className="card"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
    >
      <div className="p-4">
        {/* ── Top row ── */}
        <div className="flex items-start gap-3 mb-3">
          {/* Category icon */}
          <div
            className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5"
            style={{ background: catMeta.color + "15", border: `1px solid ${catMeta.color}30` }}
          >
            <CatIcon size={15} style={{ color: catMeta.color }} />
          </div>

          <div className="flex-1 min-w-0">
            <div className="flex items-start gap-2 flex-wrap mb-1">
              <span
                className="font-bold text-sm leading-snug"
                style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
              >
                {p.title}
              </span>
            </div>
            <div className="flex items-center gap-2 flex-wrap">
              <span className={cn("badge", STATUS_STYLES[p.status] ?? "badge-neutral")}>
                {STATUS_LABELS[p.status] ?? p.status}
              </span>
              <span
                className="badge badge-neutral"
                style={{ color: catMeta.color, borderColor: catMeta.color + "30", background: catMeta.color + "10" }}
              >
                {catMeta.label}
              </span>
            </div>
          </div>

          {/* Veto indicator */}
          {p.veto_count > 0 && (
            <div
              className="flex items-center gap-1 text-[10px] font-semibold flex-shrink-0"
              style={{ color: "var(--error)" }}
            >
              <Shield size={11} />
              Veto ×{p.veto_count}
            </div>
          )}
        </div>

        {/* Description */}
        <p className="text-xs leading-relaxed mb-3 line-clamp-2" style={{ color: "var(--text-2)" }}>
          {p.description}
        </p>

        {/* ── Tally bar ── */}
        {total > 0 && (
          <div className="mb-3">
            <div className="tally mb-1.5 rounded-full overflow-hidden" style={{ height: "6px" }}>
              <div className="tally-yes h-full transition-all duration-700" style={{ width: `${yesPct}%` }} />
              <div className="tally-no  h-full transition-all duration-700" style={{ width: `${noPct}%`  }} />
              <div style={{ flex: 1, background: "var(--surface-3)" }} />
            </div>
            <div className="flex gap-4 text-[10px]" style={{ color: "var(--text-3)" }}>
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full" style={{ background: "var(--accent)" }} />
                Evet %{yesPct}
              </span>
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full" style={{ background: "var(--error)" }} />
                Hayır %{noPct}
              </span>
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full" style={{ background: "var(--surface-3)" }} />
                Çekimser %{absPct}
              </span>
            </div>
          </div>
        )}

        {/* ── Timelock / countdown ── */}
        {(p.status === "active" || p.status === "timelocked") && countdown && (
          <div
            className="flex items-center gap-2 rounded-xl px-3 py-2 mb-3 text-xs"
            style={p.status === "timelocked"
              ? { background: "rgba(77,168,255,0.07)", border: "1px solid rgba(77,168,255,0.15)", color: "var(--signal)" }
              : { background: "var(--surface-3)", border: "1px solid var(--border-1)", color: "var(--text-2)" }}
          >
            <Timer size={12} className="flex-shrink-0" />
            <span>
              {p.status === "timelocked" ? "Timelock kalan: " : "Oylama bitiş: "}
              <strong>{countdown}</strong>
            </span>
          </div>
        )}

        {/* ── Guardian veto warning ── */}
        {p.status === "timelocked" && (
          <div
            className="flex items-center gap-2 rounded-xl px-3 py-2 mb-3 text-xs"
            style={{ background: "rgba(251,146,60,0.07)", border: "1px solid rgba(251,146,60,0.15)", color: "#fb923c" }}
          >
            <Shield size={12} className="flex-shrink-0" />
            <span>Guardian 24 saat içinde veto edebilir</span>
          </div>
        )}

        {/* ── Actions ── */}
        {p.status === "active" && (
          <button
            onClick={onFinalize}
            disabled={actionBusy === busyId + "fin"}
            className="w-full h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
            style={{ background: "var(--surface-3)", border: "1px solid var(--border-2)", color: "var(--text-1)" }}
          >
            {actionBusy === busyId + "fin" ? <Loader2 size={12} className="animate-spin" /> : <Clock size={12} />}
            Sonuçlandır
          </button>
        )}

        {p.status === "timelocked" && (
          <div className="flex gap-2">
            <button
              onClick={onExecute}
              disabled={!!actionBusy}
              className="flex-1 h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
              style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)", color: "var(--accent)" }}
            >
              {actionBusy === busyId + "exe" ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />}
              Uygula
            </button>
            <button
              onClick={onVeto}
              disabled={!!actionBusy}
              className="flex-1 h-9 rounded-xl flex items-center justify-center gap-1.5 text-xs font-semibold transition-all active:scale-[0.98]"
              style={{ background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.2)", color: "var(--error)" }}
            >
              {actionBusy === busyId + "veto" ? <Loader2 size={12} className="animate-spin" /> : <XCircle size={12} />}
              Veto
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

export default function DAOPage() {
  const [proposals, setProposals] = useState<DAOProposal[]>([]);
  const [loading, setLoading]     = useState(true);
  const [actionBusy, setActionBusy] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm]           = useState({ title: "", description: "", category: "param" });
  const [creating, setCreating]   = useState(false);
  const [error, setError]         = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.daoListProposals();
      setProposals(res?.proposals || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const doAction = async (id: string, suffix: string, action: () => Promise<any>, label: string) => {
    setActionBusy(id + suffix);
    setError("");
    try {
      await action();
      await load();
    } catch (e: any) {
      setError(`${label}: ${e.message}`);
    } finally {
      setActionBusy(null);
    }
  };

  const handleCreate = async () => {
    if (!form.title || !form.description) return;
    setCreating(true);
    setError("");
    try {
      await api.daoCreateProposal(form);
      setForm({ title: "", description: "", category: "param" });
      setShowCreate(false);
      await load();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setCreating(false);
    }
  };

  const activeCount   = proposals.filter(p => p.status === "active").length;
  const executedCount = proposals.filter(p => p.status === "executed").length;
  const memberCount   = 847; // stub

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Header ── */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">DAO Yönetimi</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>Timelock · Guardian veto · Süper çoğunluk %67</p>
          </div>
          <div className="flex gap-2">
            <button onClick={load} aria-label="Yenile" className="btn-icon">
              <RefreshCw size={14} />
            </button>
            <button
              onClick={() => setShowCreate(v => !v)}
              aria-label="Yeni teklif"
              className="btn-accent-ghost"
            >
              <Plus size={14} />
            </button>
          </div>
        </div>

        {/* ── DAO stats ── */}
        <div className="grid grid-cols-3 gap-2 px-5 mb-4">
          {[
            { label: "Üye",       value: memberCount.toLocaleString(), icon: Users },
            { label: "Öneri",     value: proposals.length.toString(),  icon: FileText },
            { label: "Uygulandı", value: executedCount.toString(),     icon: CheckCircle2 },
          ].map(({ label, value, icon: Icon }) => (
            <div key={label} className="stat-box">
              <Icon size={13} style={{ color: "var(--text-3)" }} />
              <span className="stat-value text-lg">{value}</span>
              <span className="stat-label" style={{ fontSize: "9px" }}>{label}</span>
            </div>
          ))}
        </div>

        {/* ── Create form ── */}
        {showCreate && (
          <div
            className="mx-5 mb-4 rounded-2xl p-4 space-y-3 animate-slide-down"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
          >
            <div className="flex items-center justify-between">
              <p className="section-label">Yeni Öneri</p>
              <button onClick={() => setShowCreate(false)} aria-label="Kapat" className="btn-icon w-7 h-7">
                <X size={13} />
              </button>
            </div>
            <input
              value={form.title}
              onChange={e => setForm(f => ({ ...f, title: e.target.value }))}
              placeholder="Başlık"
              aria-label="Teklif başlığı"
              className="field h-10 text-sm"
            />
            <textarea
              value={form.description}
              onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
              placeholder="Açıklama..."
              aria-label="Teklif açıklaması"
              rows={3}
              className="field resize-none text-sm"
            />
            <select
              value={form.category}
              onChange={e => setForm(f => ({ ...f, category: e.target.value }))}
              aria-label="Kategori"
              className="field h-10 text-sm"
            >
              {Object.entries(CATEGORY_META).map(([k, v]) => (
                <option key={k} value={k}>{v.label}</option>
              ))}
            </select>
            <button
              onClick={handleCreate}
              disabled={creating || !form.title || !form.description}
              className="btn-primary w-full"
            >
              {creating ? <Loader2 size={14} className="animate-spin" /> : <Scale size={14} />}
              {creating ? "Gönderiliyor..." : "Öneri Gönder"}
            </button>
          </div>
        )}

        {/* ── Error ── */}
        {error && (
          <div className="mx-5 mb-4 rounded-2xl px-4 py-3 text-sm animate-slide-down"
            style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}>
            {error}
          </div>
        )}

        {/* ── Proposals ── */}
        {loading ? (
          <div className="px-5 space-y-3">
            {[0,1,2].map(i => (
              <div key={i} className="rounded-2xl h-40 shimmer" style={{ background: "var(--surface-2)" }} />
            ))}
          </div>
        ) : proposals.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div className="w-14 h-14 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
              <Scale size={24} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>Henüz öneri yok</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>
                İlk DAO önerisini oluşturmak için <strong>+</strong> butonuna tıkla.
              </p>
            </div>
            <button onClick={() => setShowCreate(true)} className="btn-primary h-10 px-5 text-sm">
              <Plus size={14} /> Öneri Oluştur
            </button>
          </div>
        ) : (
          <div className="px-5 space-y-3">
            {activeCount > 0 && (
              <p className="section-label">{activeCount} Aktif Oylama</p>
            )}
            {proposals.map(p => (
              <ProposalCard
                key={p.id}
                p={p}
                actionBusy={actionBusy}
                onFinalize={() => doAction(p.id, "fin", () => api.daoFinalize(p.id), "Finalize")}
                onExecute={() => doAction(p.id, "exe", () => api.daoExecute(p.id), "Execute")}
                onVeto={() => doAction(p.id, "veto", () => api.daoVeto(p.id, "Guardian veto"), "Veto")}
              />
            ))}
          </div>
        )}
      </div>
    </AppShell>
  );
}
