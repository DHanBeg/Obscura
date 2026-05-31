"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Vote, Plus, Loader2, RefreshCw,
  CheckCircle2, Clock, XCircle, Zap, Users,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface Proposal {
  id: string;
  title: string;
  description: string;
  proposal_type: string;
  status: string;
  created_at: string;
  voting_ends_at?: string;
  proposer_did: string;
  yes_count?: number;
  no_count?: number;
  abstain_count?: number;
  total_votes?: number;
}

type FilterTab = "active" | "passed" | "rejected";

/* ── Skeleton ── */
function SkeletonCard() {
  return (
    <div className="card p-4 space-y-3">
      <div className="flex items-center gap-2">
        <div className="h-5 w-16 rounded-full shimmer" />
        <div className="h-5 w-20 rounded-full shimmer" />
      </div>
      <div className="h-4 w-3/4 rounded shimmer" />
      <div className="space-y-1.5">
        <div className="h-3 w-full rounded shimmer" />
        <div className="h-3 w-4/5 rounded shimmer" />
      </div>
      <div className="h-2 rounded-full shimmer" />
    </div>
  );
}

export default function GovernancePage() {
  const router = useRouter();
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterTab, setFilterTab] = useState<FilterTab>("active");
  const [createOpen, setCreateOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [pType, setPType] = useState("param");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.governanceListProposals();
      setProposals(res?.proposals || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!title.trim()) return;
    setCreating(true);
    setError("");
    try {
      await api.governanceCreateProposal({ title, description, proposal_type: pType });
      setTitle("");
      setDescription("");
      setCreateOpen(false);
      await load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setCreating(false);
    }
  };

  /* ── Derived stats ── */
  const activeCount = proposals.filter(p => p.status === "active").length;
  const totalVotes = proposals.reduce((s, p) => s + (p.total_votes || 0), 0);
  const passedCount = proposals.filter(p => p.status === "passed").length;
  const participation = proposals.length
    ? Math.round((passedCount / proposals.length) * 100)
    : 0;

  /* ── Filter ── */
  const statusMap: Record<FilterTab, string[]> = {
    active: ["active", "pending"],
    passed: ["passed", "executed"],
    rejected: ["rejected"],
  };
  const filtered = proposals.filter(p => statusMap[filterTab].includes(p.status));

  /* ── Status helpers ── */
  const statusLabel = (status: string) =>
    ({ active: "Oylamada", passed: "Kabul", rejected: "Red", executed: "Uygulandı", pending: "Bekliyor" }[status] ?? status);

  const categoryLabel = (type: string) =>
    type === "param" ? "Parametre" : type === "protocol" ? "Protokol" : type;

  const tallyPercent = (count: number, total: number) =>
    total > 0 ? Math.round((count / total) * 100) : 0;

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Page Header ── */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">Yönetişim</h1>
          <button onClick={load} aria-label="Yenile" className="btn-icon">
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>

        {/* ── Stats Row ── */}
        <div className="grid grid-cols-3 gap-2 mx-4 mb-5">
          <div className="stat-box">
            <span className="stat-value" style={{ color: "var(--warning)" }}>{loading ? "—" : activeCount}</span>
            <span className="stat-label">Aktif Öneri</span>
          </div>
          <div className="stat-box">
            <span className="stat-value">{loading ? "—" : totalVotes.toLocaleString()}</span>
            <span className="stat-label">Toplam Oy</span>
          </div>
          <div className="stat-box">
            <span className="stat-value" style={{ color: "var(--accent)" }}>%{loading ? "—" : participation}</span>
            <span className="stat-label">Katılım</span>
          </div>
        </div>

        {/* ── Filter Tabs ── */}
        <div className="mx-4 mb-4 flex rounded-[16px] p-1"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          {(["active", "passed", "rejected"] as const).map(t => {
            const labels = { active: "Aktif", passed: "Onaylandı", rejected: "Reddedildi" };
            return (
              <button
                key={t}
                onClick={() => setFilterTab(t)}
                className={cn(
                  "flex-1 py-2 px-3 rounded-[12px] text-sm font-semibold transition-all duration-150 relative",
                  filterTab === t
                    ? "text-[var(--text-1)]"
                    : "text-[var(--text-3)] hover:text-[var(--text-2)]"
                )}
                style={{
                  fontFamily: "var(--font-display)",
                  background: filterTab === t
                    ? t === "active" ? "rgba(255,170,0,0.10)"
                      : t === "passed" ? "var(--accent-muted)"
                        : "rgba(255,64,88,0.08)"
                    : undefined,
                  color: filterTab === t
                    ? t === "active" ? "var(--warning)"
                      : t === "passed" ? "var(--accent)"
                        : "var(--error)"
                    : undefined,
                }}
              >
                {labels[t]}
              </button>
            );
          })}
        </div>

        {/* ── Proposal Cards ── */}
        <div className="px-4 space-y-3 mb-32">
          {loading ? (
            <>
              <SkeletonCard />
              <SkeletonCard />
              <SkeletonCard />
            </>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 gap-3">
              <Vote size={32} style={{ color: "var(--text-3)" }} />
              <p className="text-sm" style={{ color: "var(--text-3)" }}>
                {filterTab === "active" ? "Aktif öneri yok. İlk öneriyi sen yap!" : "Bu kategoride öneri yok."}
              </p>
            </div>
          ) : (
            filtered.map(p => {
              const total = p.total_votes || 0;
              const yes = tallyPercent(p.yes_count || 0, total);
              const no = tallyPercent(p.no_count || 0, total);

              return (
                <button
                  key={p.id}
                  onClick={() => router.push(`/governance/${p.id}`)}
                  className="card-interactive w-full text-left p-4 space-y-3"
                >
                  {/* Badges row */}
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className={cn("badge",
                      p.proposal_type === "param" ? "badge-signal" :
                      p.proposal_type === "protocol" ? "badge-accent" : "badge-neutral"
                    )}>
                      {categoryLabel(p.proposal_type)}
                    </span>
                    <span className={cn("badge",
                      p.status === "active" ? "badge-warning" :
                      p.status === "passed" ? "badge-success" :
                      p.status === "rejected" ? "badge-error" : "badge-neutral"
                    )}>
                      {p.status === "active" && <Clock size={9} className="mr-0.5" />}
                      {p.status === "passed" && <CheckCircle2 size={9} className="mr-0.5" />}
                      {p.status === "rejected" && <XCircle size={9} className="mr-0.5" />}
                      {statusLabel(p.status)}
                    </span>
                  </div>

                  {/* Title */}
                  <p
                    className="font-semibold leading-snug"
                    style={{ fontSize: 15, color: "var(--text-1)", fontFamily: "var(--font-display)", letterSpacing: "-0.01em" }}
                  >
                    {p.title}
                  </p>

                  {/* Description preview */}
                  <p className="text-sm line-clamp-2" style={{ color: "var(--text-2)" }}>
                    {p.description}
                  </p>

                  {/* Tally bar */}
                  {total > 0 && (
                    <div className="space-y-1.5">
                      <div className="tally">
                        <div className="tally-yes transition-all" style={{ width: `${yes}%` }} />
                        <div className="tally-no transition-all" style={{ width: `${no}%` }} />
                      </div>
                      <div className="flex items-center justify-between">
                        <span className="text-[11px]" style={{ color: "var(--accent)" }}>%{yes} Evet</span>
                        <span className="text-[11px]" style={{ color: "var(--text-3)" }}>{total} oy</span>
                        <span className="text-[11px]" style={{ color: "var(--error)" }}>%{no} Hayır</span>
                      </div>
                    </div>
                  )}

                  {/* Footer */}
                  <div className="flex items-center gap-3 pt-1" style={{ borderTop: "1px solid var(--border-1)" }}>
                    <div className="flex items-center gap-1">
                      <Clock size={11} style={{ color: "var(--text-3)" }} />
                      <span className="text-[11px]" style={{ color: "var(--text-3)" }}>{formatTime(p.created_at)}</span>
                    </div>
                    {p.voting_ends_at && (
                      <div className="flex items-center gap-1">
                        <Zap size={11} style={{ color: "var(--warning)" }} />
                        <span className="text-[11px]" style={{ color: "var(--warning)" }}>Bitiş: {formatTime(p.voting_ends_at)}</span>
                      </div>
                    )}
                    {total > 0 && (
                      <div className="flex items-center gap-1 ml-auto">
                        <Users size={11} style={{ color: "var(--text-3)" }} />
                        <span className="text-[11px]" style={{ color: "var(--text-3)" }}>{total}</span>
                      </div>
                    )}
                  </div>
                </button>
              );
            })
          )}
        </div>

        {/* ── FAB — New Proposal ── */}
        <button
          onClick={() => setCreateOpen(true)}
          aria-label="Yeni öneri oluştur"
          className="fixed bottom-24 right-5 z-40 btn-primary w-14 h-14 rounded-full p-0 shadow-4"
          style={{ boxShadow: "0 0 24px rgba(0,229,160,0.35), 0 8px 24px rgba(0,0,0,0.7)" }}
        >
          <Plus size={22} />
        </button>
      </div>

      {/* ── Create Proposal Modal ── */}
      {createOpen && (
        <div
          className="fixed inset-0 z-50 flex items-end"
          role="dialog"
          aria-modal="true"
          aria-label="Yeni Öneri Oluştur"
        >
          <div
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            onClick={() => setCreateOpen(false)}
          />
          <div
            className="relative w-full rounded-t-[28px] px-5 pt-5 pb-10 animate-slide-up"
            style={{ background: "var(--surface-2)", borderTop: "1px solid var(--border-2)" }}
          >
            <div className="w-10 h-1 rounded-full mx-auto mb-5" style={{ background: "var(--border-3)" }} />
            <h2 className="text-lg font-display font-bold mb-1" style={{ color: "var(--text-1)" }}>Yeni Öneri</h2>
            <p className="text-sm mb-5" style={{ color: "var(--text-2)" }}>Teklif oluşturmak 100 OBS yakar.</p>

            <div className="space-y-3">
              <div>
                <label className="section-label block mb-2">Başlık</label>
                <input
                  value={title}
                  onChange={e => setTitle(e.target.value)}
                  placeholder="Öneri başlığı"
                  className="field"
                  autoFocus
                  onKeyDown={e => e.key === "Escape" && setCreateOpen(false)}
                />
              </div>
              <div>
                <label className="section-label block mb-2">Açıklama</label>
                <textarea
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder="Öneriyi detaylandır…"
                  rows={3}
                  className="field resize-none"
                />
              </div>
              <div>
                <label className="section-label block mb-2">Tür</label>
                <div className="grid grid-cols-2 gap-2">
                  {([ ["param", "Parametre"], ["protocol", "Protokol"] ] as const).map(([val, label]) => (
                    <button
                      key={val}
                      onClick={() => setPType(val)}
                      className={cn(
                        "h-11 rounded-[14px] text-sm font-semibold transition-all border",
                        pType === val
                          ? "border-[rgba(0,229,160,0.3)] text-[var(--accent)]"
                          : "border-[var(--border-1)] text-[var(--text-3)]"
                      )}
                      style={{
                        fontFamily: "var(--font-display)",
                        background: pType === val ? "var(--accent-muted)" : "var(--surface-3)",
                      }}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>

              {error && (
                <div className="rounded-2xl px-4 py-2.5 text-sm"
                  style={{ color: "var(--error)", background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)" }}>
                  {error}
                </div>
              )}
            </div>

            <div className="flex gap-2.5 mt-5">
              <button onClick={() => setCreateOpen(false)} className="btn-secondary flex-1">İptal</button>
              <button
                onClick={handleCreate}
                disabled={creating || !title.trim()}
                className="btn-primary flex-1"
              >
                {creating && <Loader2 size={15} className="animate-spin" />}
                Oluştur
              </button>
            </div>
          </div>
        </div>
      )}
    </AppShell>
  );
}
