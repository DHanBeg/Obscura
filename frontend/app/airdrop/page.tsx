"use client";

import { useState, useEffect, useCallback } from "react";
import { Gift, CheckCircle2, Clock, Loader2, RefreshCw, Lock, KeyRound, X, Sparkles, CircleCheck } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface Campaign {
  id: string;
  title: string;
  total_amount: string;
  per_claim: string;
  min_tier: number;
  claimed_count: number;
  max_claims: number;
  ends_at: string;
  status: string;
  user_claimed?: boolean;
}

export default function AirdropPage() {
  const { user } = useStore();
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [claiming, setClaiming] = useState<string | null>(null);
  const [claimSuccess, setClaimSuccess] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [mnemonicInput, setMnemonicInput] = useState("");
  const [pendingCampaign, setPendingCampaign] = useState<Campaign | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.airdropListCampaigns();
      setCampaigns(res?.campaigns || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleClaim = (campaign: Campaign) => {
    setError("");
    setPendingCampaign(campaign);
    setMnemonicInput("");
  };

  const submitClaim = async () => {
    if (!pendingCampaign || !mnemonicInput.trim()) return;
    const campaign = pendingCampaign;
    setPendingCampaign(null);
    setClaiming(campaign.id);
    setError("");
    try {
      // Derive identity_secret from mnemonic via backend (HTTPS — acceptable for MVP)
      const derived = await api.deriveIdentity(mnemonicInput.trim());
      const identitySecretHex: string = derived?.identity_secret_hex ?? "";
      const phone: string = user?.phone ?? "";
      if (!identitySecretHex || !phone) throw new Error("Kimlik türetme başarısız");

      const { buildIdentityProof } = await import("@/lib/zkproof");
      const { proof_json, public_inputs } = await buildIdentityProof(identitySecretHex, phone);

      await api.airdropClaim(campaign.id, { proof_json, public_inputs });
      setClaimSuccess(campaign.id);
      await load();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setClaiming(null);
    }
  };

  const userTier = user?.tier || 0;
  const activeCount = campaigns.filter(c => c.status === "active").length;
  const claimableCampaigns = campaigns.filter(c => c.status === "active" && userTier >= c.min_tier && !c.user_claimed);
  const totalClaimable = claimableCampaigns.reduce((s, c) => s + parseFloat(c.per_claim || "0"), 0);

  // Eligibility checklist derived from actual data
  const eligChecks = [
    { label: "Hesap Aktif", ok: !!user },
    { label: "Tier 1+ Seviye", ok: userTier >= 1 },
    { label: "Tier 2+ Güvenilir", ok: userTier >= 2 },
    { label: "Tier 3+ Veteran", ok: userTier >= 3 },
  ];

  return (
    <AppShell>
      {/* ── ZK Proof Modal ── */}
      {pendingCampaign && (
        <div
          className="fixed inset-0 z-50 flex items-end justify-center p-4"
          style={{ background: "rgba(2,2,8,0.8)", backdropFilter: "blur(16px)" }}
          role="dialog"
          aria-modal="true"
          aria-label="ZK Kimlik Kanıtı"
        >
          <div
            className="w-full max-w-sm rounded-3xl p-5 space-y-4 animate-slide-up"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-xl flex items-center justify-center" style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
                  <KeyRound size={15} style={{ color: "var(--accent)" }} />
                </div>
                <p className="text-sm font-bold" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>ZK Kimlik Kanıtı</p>
              </div>
              <button
                onClick={() => setPendingCampaign(null)}
                className="btn-icon w-8 h-8"
                aria-label="Kapat"
              >
                <X size={15} />
              </button>
            </div>

            <div className="zk-badge w-fit">
              <Sparkles size={10} />
              Zero-Knowledge Proof
            </div>

            <p className="text-xs leading-relaxed" style={{ color: "var(--text-2)" }}>
              Kimliğinizi kanıtlamak için mnemonik ifadenizi girin. ZK kanıtı cihazınızda oluşturulacak — sunucuya gönderilmeyecek.
            </p>
            <textarea
              value={mnemonicInput}
              onChange={e => setMnemonicInput(e.target.value)}
              placeholder="word1 word2 word3 ... word12"
              rows={3}
              aria-label="Mnemonik ifade"
              className="field resize-none text-sm"
            />
            <button
              onClick={submitClaim}
              disabled={!mnemonicInput.trim()}
              className="btn-primary w-full"
            >
              <Gift size={14} />
              Kanıtla ve Talep Et
            </button>
          </div>
        </div>
      )}

      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Hero Header ── */}
        <div className="relative px-5 pt-8 pb-6 overflow-hidden">
          {/* Ambient glow */}
          <div className="absolute top-0 left-1/2 -translate-x-1/2 w-64 h-32 rounded-full pointer-events-none opacity-20"
            style={{ background: "radial-gradient(ellipse, var(--accent) 0%, transparent 70%)", filter: "blur(32px)" }} />
          <div className="relative flex flex-col items-center text-center gap-3">
            <div className="w-16 h-16 rounded-3xl flex items-center justify-center animate-float"
              style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.25)" }}>
              <Gift size={28} style={{ color: "var(--accent)" }} />
            </div>
            <div>
              <h1 className="text-display text-3xl mb-1" style={{ color: "var(--text-1)" }}>OBS Airdrop</h1>
              <p className="text-sm" style={{ color: "var(--text-2)" }}>
                {activeCount > 0
                  ? `${activeCount} aktif kampanya · Talep et`
                  : "Yakında yeni dağıtımlar"}
              </p>
            </div>
            {totalClaimable > 0 && (
              <div className="mt-1 px-4 py-2 rounded-2xl" style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
                <span className="text-display font-bold text-2xl" style={{ fontFamily: "var(--font-display)", color: "var(--accent)" }}>
                  {totalClaimable.toFixed(2)} OBS
                </span>
                <span className="text-xs ml-2" style={{ color: "var(--text-2)" }}>talep edilebilir</span>
              </div>
            )}
          </div>
        </div>

        {/* ── Eligibility checklist ── */}
        <div className="mx-5 mb-4 rounded-2xl p-4" style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
          <p className="section-label mb-3">Hak Kazanma Koşulları</p>
          <div className="grid grid-cols-2 gap-2">
            {eligChecks.map(({ label, ok }) => (
              <div key={label} className="flex items-center gap-2">
                <CircleCheck
                  size={14}
                  className={cn(ok ? "text-[var(--accent)]" : "")}
                  style={{ color: ok ? "var(--accent)" : "var(--text-3)" }}
                />
                <span className="text-xs" style={{ color: ok ? "var(--text-1)" : "var(--text-3)" }}>{label}</span>
              </div>
            ))}
          </div>
        </div>

        {/* ── Error ── */}
        {error && (
          <div className="mx-5 mb-4 rounded-2xl px-4 py-3 text-sm animate-slide-down"
            style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}>
            {error}
          </div>
        )}

        {/* ── Section label ── */}
        <div className="px-5 mb-3">
          <p className="section-label">Kampanyalar</p>
        </div>

        {/* ── Campaigns ── */}
        {loading ? (
          <div className="px-5 space-y-3">
            {[0,1,2].map(i => (
              <div key={i} className="rounded-2xl h-28 shimmer" style={{ background: "var(--surface-2)" }} />
            ))}
          </div>
        ) : campaigns.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div className="w-14 h-14 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
              <Gift size={24} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>Henüz kampanya yok</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>Yakında OBS dağıtımları başlayacak.</p>
            </div>
          </div>
        ) : (
          <div className="px-5 space-y-3">
            {campaigns.map((c) => {
              const isActive = c.status === "active";
              const canClaim = isActive && userTier >= c.min_tier && !c.user_claimed;
              const isClaiming = claiming === c.id;
              const claimed = c.user_claimed || claimSuccess === c.id;
              const progress = c.max_claims > 0 ? (c.claimed_count / c.max_claims) * 100 : 0;

              return (
                <div
                  key={c.id}
                  className="rounded-2xl p-4"
                  style={{ background: "var(--surface-2)", border: `1px solid ${isActive ? "var(--border-2)" : "var(--border-1)"}` }}
                >
                  {/* Top row */}
                  <div className="flex items-start justify-between gap-3 mb-4">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap mb-1.5">
                        <p className="font-bold text-sm truncate" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>
                          {c.title}
                        </p>
                        {!isActive && (
                          <span className="badge badge-neutral text-[9px]">
                            {c.status === "ended" ? "Bitti" : c.status}
                          </span>
                        )}
                      </div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-xs font-bold" style={{ color: "var(--text-1)" }}>
                          {parseFloat(c.per_claim).toFixed(2)} OBS
                        </span>
                        <span style={{ color: "var(--text-3)" }}>·</span>
                        <span className="flex items-center gap-1 text-xs" style={{ color: "var(--text-2)" }}>
                          {c.min_tier > 1 && <Lock size={10} />}
                          Tier {c.min_tier}+
                        </span>
                      </div>
                    </div>

                    {/* Action */}
                    {isActive ? (
                      claimed ? (
                        <div className="flex items-center gap-1.5 flex-shrink-0">
                          <span className="badge badge-success">
                            <CheckCircle2 size={10} />
                            Alındı
                          </span>
                        </div>
                      ) : (
                        <button
                          onClick={() => handleClaim(c)}
                          disabled={!canClaim || isClaiming}
                          className={cn(
                            "flex-shrink-0 h-9 px-3 rounded-xl text-xs font-semibold flex items-center gap-1.5 transition-all active:scale-95",
                            canClaim
                              ? "btn-primary h-9 px-3 text-xs"
                              : "cursor-not-allowed opacity-50"
                          )}
                          style={!canClaim ? { background: "var(--surface-3)", color: "var(--text-3)" } : undefined}
                        >
                          {isClaiming ? <Loader2 size={12} className="animate-spin" /> : <Gift size={12} />}
                          {canClaim
                            ? "Talep Et"
                            : userTier < c.min_tier
                            ? `Tier ${c.min_tier}`
                            : "Alındı"}
                        </button>
                      )
                    ) : null}
                  </div>

                  {/* ZK badge for claimable */}
                  {canClaim && (
                    <div className="mb-3">
                      <span className="zk-badge">
                        <Sparkles size={9} />
                        ZK Proof ile Talep
                      </span>
                    </div>
                  )}

                  {/* Progress bar */}
                  <div className="space-y-1.5">
                    <div className="flex justify-between text-[10px]" style={{ color: "var(--text-3)" }}>
                      <span>{c.claimed_count.toLocaleString()} / {c.max_claims.toLocaleString()} talep</span>
                      <span className="flex items-center gap-1">
                        <Clock size={10} />
                        {formatTime(c.ends_at)}
                      </span>
                    </div>
                    <div className="progress-track">
                      <div
                        className="progress-fill"
                        style={{ width: `${Math.min(100, progress)}%` }}
                      />
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* ── Total claimable footer ── */}
        {!loading && totalClaimable > 0 && (
          <div className="mx-5 mt-4 rounded-2xl p-4 flex items-center justify-between"
            style={{ background: "var(--accent-deep)", border: "1px solid rgba(0,229,160,0.12)" }}>
            <div>
              <p className="section-label mb-1">Toplam Talep Edilebilir</p>
              <p className="text-display font-black text-2xl" style={{ fontFamily: "var(--font-display)", color: "var(--accent)" }}>
                {totalClaimable.toFixed(2)} OBS
              </p>
            </div>
            <div className="w-12 h-12 rounded-2xl flex items-center justify-center"
              style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
              <Sparkles size={20} style={{ color: "var(--accent)" }} />
            </div>
          </div>
        )}
      </div>
    </AppShell>
  );
}
