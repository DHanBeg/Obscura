"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import {
  Vote, ThumbsUp, ThumbsDown, Minus, AlertTriangle,
  Loader2, CheckCircle2, Clock, ShieldCheck, Zap,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface Tally {
  yes: number;
  no: number;
  abstain: number;
  veto: number;
  total: number;
}

interface ProposalDetail {
  id: string;
  title: string;
  description: string;
  proposal_type: string;
  status: string;
  created_at: string;
  voting_ends_at?: string;
  execution_at?: string;
  proposer_did: string;
  execution_payload?: string;
}

const CHOICES = [
  { value: 0, label: "Evet", Icon: ThumbsUp, accent: "var(--accent)", bg: "rgba(0,229,160,0.08)", border: "rgba(0,229,160,0.25)" },
  { value: 1, label: "Hayır", Icon: ThumbsDown, accent: "var(--error)", bg: "rgba(255,64,88,0.08)", border: "rgba(255,64,88,0.25)" },
  { value: 2, label: "Çekimser", Icon: Minus, accent: "var(--text-2)", bg: "var(--surface-3)", border: "var(--border-2)" },
  { value: 3, label: "Veto", Icon: AlertTriangle, accent: "var(--warning)", bg: "rgba(255,170,0,0.08)", border: "rgba(255,170,0,0.25)" },
];

export default function ProposalDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [proposal, setProposal] = useState<ProposalDetail | null>(null);
  const [tally, setTally] = useState<Tally | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedChoice, setSelectedChoice] = useState<number | null>(null);
  const [voting, setVoting] = useState(false);
  const [voteError, setVoteError] = useState("");
  const [voteSuccess, setVoteSuccess] = useState(false);
  const [finalizing, setFinalizing] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const res = await api.governanceGetProposal(id);
      setProposal(res?.proposal || null);
      setTally(res?.tally || null);
    } catch {}
    finally { setLoading(false); }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const handleVote = async () => {
    if (selectedChoice === null) return;
    setVoting(true);
    setVoteError("");
    try {
      const rootData = await api.governanceVoterRoot(id);
      const pollId: string = rootData?.poll_id ?? "0";
      const voterRoot: string = rootData?.merkle_root ?? "0";

      const { buildVoteProof } = await import("@/lib/zkproof");
      const { proof_json, public_inputs } = await buildVoteProof(pollId, voterRoot, selectedChoice);

      await api.governanceVote(id, { proof_json, public_inputs, choice: selectedChoice });
      setVoteSuccess(true);
      await load();
    } catch (e: unknown) {
      setVoteError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setVoting(false);
    }
  };

  const handleFinalize = async () => {
    setFinalizing(true);
    try {
      await api.governanceFinalize(id);
      await load();
    } catch (e: unknown) {
      setVoteError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setFinalizing(false);
    }
  };

  const handleExecute = async () => {
    setFinalizing(true);
    try {
      await api.governanceExecute(id);
      await load();
    } catch (e: unknown) {
      setVoteError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setFinalizing(false);
    }
  };

  const tallyPercent = (count: number) => {
    if (!tally?.total) return 0;
    return Math.round((count / tally.total) * 100);
  };

  /* ── Timelock helper ── */
  const timelockHours = proposal?.execution_at
    ? Math.max(0, Math.round((new Date(proposal.execution_at).getTime() - Date.now()) / 3_600_000))
    : null;

  if (loading) {
    return (
      <AppShell showBack>
        <div className="flex items-center justify-center h-full">
          <Loader2 size={28} className="animate-spin" style={{ color: "var(--accent)" }} />
        </div>
      </AppShell>
    );
  }

  if (!proposal) {
    return (
      <AppShell showBack>
        <div className="flex items-center justify-center h-full">
          <p className="text-sm" style={{ color: "var(--text-3)" }}>Öneri bulunamadı</p>
        </div>
      </AppShell>
    );
  }

  const isActive = proposal.status === "active";
  const isPassed = proposal.status === "passed";
  const categoryLabel = proposal.proposal_type === "param" ? "Parametre" : "Protokol";

  return (
    <AppShell showBack>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Header ── */}
        <div className="px-5 pt-5 pb-4 flex-shrink-0 animate-in">
          {/* Badge row */}
          <div className="flex items-center gap-2 flex-wrap mb-4">
            <span className={cn("badge",
              proposal.proposal_type === "param" ? "badge-signal" : "badge-accent"
            )}>
              {categoryLabel}
            </span>
            <span className={cn("badge",
              isActive ? "badge-warning" :
              isPassed ? "badge-success" :
              proposal.status === "rejected" ? "badge-error" : "badge-neutral"
            )}
              style={{ height: 24, fontSize: 12 }}
            >
              {isActive && <Clock size={11} className="mr-1" />}
              {isPassed && <CheckCircle2 size={11} className="mr-1" />}
              {isActive ? "Oylamada" : isPassed ? "Kabul Edildi" : proposal.status === "rejected" ? "Reddedildi" : proposal.status}
            </span>
          </div>

          {/* Title */}
          <h1
            className="font-display font-bold leading-tight mb-2"
            style={{ fontSize: "clamp(20px, 5vw, 24px)", color: "var(--text-1)", letterSpacing: "-0.025em" }}
          >
            {proposal.title}
          </h1>

          {/* Meta */}
          <p className="text-xs" style={{ color: "var(--text-3)" }}>
            {formatTime(proposal.created_at)} · {proposal.proposer_did?.slice(0, 18)}…
          </p>
        </div>

        {/* ── Description ── */}
        <div className="mx-4 mb-4 card p-4 animate-in stagger-1">
          <p className="text-sm leading-relaxed" style={{ color: "var(--text-2)", lineHeight: 1.7 }}>
            {proposal.description}
          </p>
        </div>

        {/* ── Big Tally Bar ── */}
        {tally && (
          <div className="mx-4 mb-4 card p-4 animate-in stagger-2">
            <div className="flex items-center justify-between mb-3">
              <p className="section-label">Oylama Sonuçları</p>
              <span className="text-xs" style={{ color: "var(--text-3)" }}>{tally.total} toplam oy</span>
            </div>

            {/* Segmented tally bar */}
            <div className="flex rounded-full overflow-hidden h-5 mb-4"
              style={{ background: "var(--surface-3)" }}
            >
              <div
                className="transition-all duration-500 tally-yes"
                style={{ width: `${tallyPercent(tally.yes)}%` }}
                title={`Evet: ${tallyPercent(tally.yes)}%`}
              />
              <div
                className="transition-all duration-500 tally-no"
                style={{ width: `${tallyPercent(tally.no)}%` }}
                title={`Hayır: ${tallyPercent(tally.no)}%`}
              />
              <div
                className="transition-all duration-500"
                style={{ width: `${tallyPercent(tally.abstain)}%`, background: "var(--surface-3)" }}
                title={`Çekimser: ${tallyPercent(tally.abstain)}%`}
              />
              <div
                className="transition-all duration-500"
                style={{ width: `${tallyPercent(tally.veto)}%`, background: "var(--warning)" }}
                title={`Veto: ${tallyPercent(tally.veto)}%`}
              />
            </div>

            {/* Vote counts grid */}
            <div className="grid grid-cols-4 gap-2">
              {[
                { label: "Evet", count: tally.yes, color: "var(--accent)" },
                { label: "Hayır", count: tally.no, color: "var(--error)" },
                { label: "Çekimser", count: tally.abstain, color: "var(--text-2)" },
                { label: "Veto", count: tally.veto, color: "var(--warning)" },
              ].map(({ label, count, color }) => (
                <div key={label} className="text-center rounded-[12px] py-2.5"
                  style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}>
                  <p className="font-display font-bold" style={{ fontSize: 20, color, letterSpacing: "-0.03em" }}>
                    {count}
                  </p>
                  <p className="text-[10px] mt-0.5" style={{ color: "var(--text-3)" }}>{label}</p>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ── ZK Vote Panel ── */}
        {isActive && !voteSuccess && (
          <div className="mx-4 mb-4 card p-4 animate-in stagger-3">
            <div className="flex items-center gap-2 mb-4">
              <p className="section-label">Oyunu Kullan</p>
              <span className="zk-badge">
                <ShieldCheck size={10} />
                ZK Anonim
              </span>
            </div>

            {/* Choice buttons */}
            <div className="grid grid-cols-2 gap-2 mb-4">
              {CHOICES.map(({ value, label, Icon, accent, bg, border }) => (
                <button
                  key={value}
                  onClick={() => setSelectedChoice(value)}
                  className="flex items-center gap-2.5 h-12 px-4 rounded-[14px] border transition-all duration-150 active:scale-[0.97]"
                  style={{
                    background: selectedChoice === value ? bg : "var(--surface-3)",
                    borderColor: selectedChoice === value ? border : "var(--border-1)",
                    color: selectedChoice === value ? accent : "var(--text-2)",
                  }}
                  aria-pressed={selectedChoice === value}
                >
                  <Icon size={14} />
                  <span className="text-sm font-semibold" style={{ fontFamily: "var(--font-display)" }}>{label}</span>
                </button>
              ))}
            </div>

            {voteError && (
              <div className="rounded-2xl px-4 py-2.5 text-sm mb-3"
                style={{ color: "var(--error)", background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)" }}>
                {voteError}
              </div>
            )}

            <button
              onClick={handleVote}
              disabled={voting || selectedChoice === null}
              className="btn-primary w-full"
              aria-label="ZK kanıtı ile oy ver"
            >
              {voting
                ? <><Loader2 size={15} className="animate-spin" /> ZK Kanıtı Oluşturuluyor…</>
                : <><Vote size={15} /> ZK Kanıtı ile Oy Ver</>
              }
            </button>
          </div>
        )}

        {/* ── Vote Success ── */}
        {voteSuccess && (
          <div className="mx-4 mb-4 rounded-[20px] flex items-center gap-3 px-4 py-4 animate-in stagger-3"
            style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}
          >
            <CheckCircle2 size={22} style={{ color: "var(--accent)", flexShrink: 0 }} />
            <div>
              <p className="text-sm font-semibold" style={{ color: "var(--accent)", fontFamily: "var(--font-display)" }}>
                Oyunuz kaydedildi
              </p>
              <p className="text-xs mt-0.5" style={{ color: "var(--text-2)" }}>
                ZK kanıtı ile anonim olarak işleme alındı.
              </p>
            </div>
          </div>
        )}

        {/* ── Timelock info ── */}
        {timelockHours !== null && isPassed && (
          <div className="mx-4 mb-4 rounded-[16px] flex items-center gap-2.5 px-4 py-3"
            style={{ background: "rgba(255,170,0,0.06)", border: "1px solid rgba(255,170,0,0.15)" }}
          >
            <Zap size={15} style={{ color: "var(--warning)", flexShrink: 0 }} />
            <p className="text-sm" style={{ color: "var(--warning)" }}>
              Yürütme: <strong>{timelockHours} saat sonra</strong> (48s timelock)
            </p>
          </div>
        )}

        {/* ── Admin Actions ── */}
        {(isActive || isPassed) && (
          <div className="mx-4 mb-32 flex gap-2.5">
            {isActive && (
              <button
                onClick={handleFinalize}
                disabled={finalizing}
                className="btn-secondary flex-1 gap-2"
              >
                {finalizing ? <Loader2 size={15} className="animate-spin" /> : null}
                Sonuçlandır
              </button>
            )}
            {isPassed && (
              <button
                onClick={handleExecute}
                disabled={finalizing}
                className="btn-primary flex-1 gap-2"
              >
                {finalizing ? <Loader2 size={15} className="animate-spin" /> : null}
                Uygula
              </button>
            )}
          </div>
        )}

      </div>
    </AppShell>
  );
}
