"use client";

import { useState, useEffect, useCallback } from "react";
import { Zap, TrendingUp, AlertTriangle, Loader2, RefreshCw, Lock, Unlock, ChevronRight } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface StakingPosition {
  id: string;
  amount: string;
  staked_at: string;
  unlock_at?: string;
  status: string;
  reward_accrued?: string;
}

interface SlashEvent {
  id: string;
  position_id: string;
  slash_amount: string;
  reason: string;
  created_at: string;
  reviewed: boolean;
}

const DURATIONS = [
  { label: "30 Gün", value: "30", apy: "8.2%" },
  { label: "90 Gün", value: "90", apy: "14.5%" },
  { label: "365 Gün", value: "365", apy: "24.0%" },
];

/* ── Skeleton ── */
function SkeletonRow() {
  return (
    <div className="flex items-center gap-3 px-4 py-3.5 border-b border-[var(--border-1)]">
      <div className="w-10 h-10 rounded-full shimmer flex-shrink-0" />
      <div className="flex-1 space-y-1.5">
        <div className="h-3 w-28 rounded shimmer" />
        <div className="h-2.5 w-20 rounded shimmer" />
      </div>
      <div className="w-14 h-6 rounded-full shimmer" />
    </div>
  );
}

export default function StakingPage() {
  const [positions, setPositions] = useState<StakingPosition[]>([]);
  const [slashes, setSlashes] = useState<SlashEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<"stake" | "positions" | "slashes">("stake");

  const [stakeAmount, setStakeAmount] = useState("");
  const [stakeDuration, setStakeDuration] = useState("90");
  const [unstakeAmount, setUnstakeAmount] = useState("");
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [pos, sl] = await Promise.all([
        api.stakingPositions(),
        api.stakingSlashes(),
      ]);
      setPositions(pos?.positions || []);
      setSlashes(sl?.slashes || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const doAction = async (action: () => Promise<void>, key: string) => {
    setActionLoading(key);
    setError("");
    setSuccess("");
    try {
      await action();
      setSuccess("İşlem başarılı!");
      await load();
      setTimeout(() => setSuccess(""), 3000);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setActionLoading(null);
    }
  };

  const activePositions = positions.filter(p => p.status === "active");
  const totalStaked = activePositions
    .reduce((sum, p) => sum + parseFloat(p.amount || "0"), 0)
    .toFixed(4);

  const selectedDuration = DURATIONS.find(d => d.value === stakeDuration) ?? DURATIONS[1];

  const statusBadge = (status: string) => {
    if (status === "active") return <span className="badge badge-accent">Aktif</span>;
    if (status === "unbonding") return <span className="badge badge-warning">Bekliyor</span>;
    return <span className="badge badge-neutral">{status}</span>;
  };

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Page Header ── */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">Staking</h1>
          <button onClick={load} aria-label="Yenile" className="btn-icon">
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>

        {/* ── Stats Row ── */}
        <div className="grid grid-cols-3 gap-2 mx-4 mb-5">
          <div className="stat-box">
            <span className="stat-value">{loading ? "—" : totalStaked}</span>
            <span className="stat-label">Toplam Stake</span>
          </div>
          <div className="stat-box">
            <span className="stat-value" style={{ color: "var(--accent)" }}>
              {selectedDuration.apy}
            </span>
            <span className="stat-label">APY</span>
          </div>
          <div className="stat-box">
            <span className="stat-value" style={{ color: slashes.length > 0 ? "var(--error)" : "var(--text-1)" }}>
              {loading ? "—" : slashes.length}
            </span>
            <span className="stat-label">Kesinti</span>
          </div>
        </div>

        {/* ── Tab Bar ── */}
        <div className="mx-4 mb-4 flex rounded-[16px] p-1"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          {(["stake", "positions", "slashes"] as const).map(t => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={cn(
                "flex-1 py-2 px-3 rounded-[12px] text-sm font-semibold transition-all duration-150 relative",
                tab === t
                  ? "text-[var(--accent)]"
                  : "text-[var(--text-3)] hover:text-[var(--text-2)]"
              )}
              style={tab === t ? { background: "var(--accent-muted)", fontFamily: "var(--font-display)" } : { fontFamily: "var(--font-display)" }}
            >
              {t === "stake" ? "Stake Et" : t === "positions" ? `Pozisyonlar${activePositions.length ? ` (${activePositions.length})` : ""}` : `Kesintiler${slashes.length ? ` (${slashes.length})` : ""}`}
              {/* Active underline */}
              {tab === t && (
                <span
                  className="absolute bottom-0 left-1/2 -translate-x-1/2 w-6 h-0.5 rounded-full"
                  style={{ background: "var(--accent)" }}
                />
              )}
            </button>
          ))}
        </div>

        {/* ── Tab Content ── */}

        {tab === "stake" && (
          <div className="mx-4 mb-4 space-y-4">
            {/* Duration selector */}
            <div>
              <p className="section-label mb-2">Kilitleme Süresi</p>
              <div className="grid grid-cols-3 gap-2">
                {DURATIONS.map(d => (
                  <button
                    key={d.value}
                    onClick={() => setStakeDuration(d.value)}
                    className={cn(
                      "rounded-[14px] px-3 py-3 text-center transition-all duration-150 border",
                      stakeDuration === d.value
                        ? "border-[rgba(0,229,160,0.3)]"
                        : "border-[var(--border-1)] hover:border-[var(--border-2)]"
                    )}
                    style={{
                      background: stakeDuration === d.value ? "var(--accent-muted)" : "var(--surface-2)",
                    }}
                  >
                    <p className="text-sm font-semibold" style={{
                      color: stakeDuration === d.value ? "var(--accent)" : "var(--text-1)",
                      fontFamily: "var(--font-display)",
                    }}>
                      {d.label}
                    </p>
                    <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>APY {d.apy}</p>
                  </button>
                ))}
              </div>
            </div>

            {/* Stake input */}
            <div className="card p-4 space-y-3">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                OBS Kilitle
              </p>
              <div className="flex gap-2">
                <input
                  value={stakeAmount}
                  onChange={e => setStakeAmount(e.target.value)}
                  placeholder="Miktar (OBS)"
                  type="number"
                  min="0"
                  step="0.0001"
                  className="field flex-1"
                  onKeyDown={e => e.key === "Enter" && stakeAmount && doAction(() => api.stakingStake({ amount: stakeAmount }), "stake")}
                />
                <button
                  onClick={() => doAction(() => api.stakingStake({ amount: stakeAmount }), "stake")}
                  disabled={actionLoading === "stake" || !stakeAmount}
                  className="btn-primary px-5 gap-2"
                >
                  {actionLoading === "stake" ? <Loader2 size={16} className="animate-spin" /> : <Lock size={16} />}
                  Kilitle
                </button>
              </div>

              {/* Unstake */}
              <div className="flex gap-2">
                <input
                  value={unstakeAmount}
                  onChange={e => setUnstakeAmount(e.target.value)}
                  placeholder="Serbest bırakılacak miktar"
                  type="number"
                  min="0"
                  step="0.0001"
                  className="field flex-1"
                />
                <button
                  onClick={() => doAction(() => api.stakingUnstake({ amount: unstakeAmount }), "unstake")}
                  disabled={actionLoading === "unstake" || !unstakeAmount}
                  className="btn-secondary px-5 gap-2"
                >
                  {actionLoading === "unstake" ? <Loader2 size={16} className="animate-spin" /> : <Unlock size={16} />}
                  Unstake
                </button>
              </div>

              {/* Withdraw */}
              <button
                onClick={() => doAction(() => api.stakingWithdraw(), "withdraw")}
                disabled={actionLoading === "withdraw"}
                className="w-full h-11 rounded-[14px] text-sm font-semibold flex items-center justify-center gap-2 transition-all"
                style={{
                  background: "rgba(0,229,160,0.06)",
                  border: "1px solid rgba(0,229,160,0.15)",
                  color: "var(--accent)",
                }}
              >
                {actionLoading === "withdraw" ? <Loader2 size={15} className="animate-spin" /> : <TrendingUp size={15} />}
                Çek (Kilidi Açılanları)
              </button>

              {error && (
                <div className="rounded-2xl px-4 py-2.5 text-sm"
                  style={{ color: "var(--error)", background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)" }}>
                  {error}
                </div>
              )}
              {success && (
                <div className="rounded-2xl px-4 py-2.5 text-sm"
                  style={{ color: "var(--accent)", background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
                  {success}
                </div>
              )}
            </div>
          </div>
        )}

        {tab === "positions" && (
          <div className="mx-4 rounded-[20px] overflow-hidden mb-32"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
          >
            {loading ? (
              <><SkeletonRow /><SkeletonRow /></>
            ) : positions.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 gap-3">
                <Zap size={28} style={{ color: "var(--text-3)" }} />
                <p className="text-sm" style={{ color: "var(--text-3)" }}>Henüz stake pozisyonu yok</p>
                <button onClick={() => setTab("stake")} className="btn-accent-ghost text-xs h-8 px-4">
                  Stake Et <ChevronRight size={12} />
                </button>
              </div>
            ) : (
              positions.map((pos, i) => (
                <div
                  key={pos.id}
                  className={cn(
                    "flex items-center gap-3 px-4 py-3.5",
                    i < positions.length - 1 && "border-b border-[var(--border-1)]",
                  )}
                >
                  <div
                    className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                    style={{
                      background: pos.status === "active" ? "rgba(255,170,0,0.10)" : "var(--surface-3)",
                      border: `1px solid ${pos.status === "active" ? "rgba(255,170,0,0.2)" : "var(--border-1)"}`,
                    }}
                  >
                    <Zap size={16} style={{ color: pos.status === "active" ? "var(--warning)" : "var(--text-3)" }} />
                  </div>

                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                      {parseFloat(pos.amount).toFixed(4)} OBS
                    </p>
                    <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
                      {formatTime(pos.staked_at)}
                      {pos.unlock_at && ` · Kilit: ${formatTime(pos.unlock_at)}`}
                    </p>
                  </div>

                  <div className="flex flex-col items-end gap-1.5">
                    {statusBadge(pos.status)}
                    {pos.reward_accrued && parseFloat(pos.reward_accrued) > 0 && (
                      <span className="text-[11px]" style={{ color: "var(--accent)" }}>
                        +{parseFloat(pos.reward_accrued).toFixed(4)}
                      </span>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {tab === "slashes" && (
          <div className="mx-4 rounded-[20px] overflow-hidden mb-32"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
          >
            {loading ? (
              <><SkeletonRow /><SkeletonRow /></>
            ) : slashes.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 gap-3">
                <AlertTriangle size={28} style={{ color: "var(--text-3)" }} />
                <p className="text-sm" style={{ color: "var(--text-3)" }}>Kesinti olayı yok</p>
              </div>
            ) : (
              slashes.map((sl, i) => (
                <div
                  key={sl.id}
                  className={cn(
                    "flex items-center gap-3 px-4 py-3.5",
                    i < slashes.length - 1 && "border-b border-[var(--border-1)]",
                  )}
                >
                  <div
                    className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                    style={{ background: "rgba(255,64,88,0.10)", border: "1px solid rgba(255,64,88,0.2)" }}
                  >
                    <AlertTriangle size={16} style={{ color: "var(--error)" }} />
                  </div>

                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold" style={{ color: "var(--error)", fontFamily: "var(--font-display)" }}>
                      −{parseFloat(sl.slash_amount).toFixed(4)} OBS
                    </p>
                    <p className="text-xs mt-0.5 truncate" style={{ color: "var(--text-3)" }}>{sl.reason}</p>
                  </div>

                  <div className="flex flex-col items-end gap-1.5 flex-shrink-0">
                    <span className="text-[11px]" style={{ color: "var(--text-3)" }}>{formatTime(sl.created_at)}</span>
                    <span className={cn("badge", sl.reviewed ? "badge-neutral" : "badge-error")}>
                      {sl.reviewed ? "İncelendi" : "Bekliyor"}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

      </div>
    </AppShell>
  );
}
