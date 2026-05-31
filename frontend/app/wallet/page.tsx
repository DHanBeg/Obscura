"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowUpRight, ArrowDownLeft, ArrowRightLeft,
  Eye, EyeOff, Copy, Check, Loader2, ShieldCheck, Clock,
  RefreshCw,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface WalletBalance {
  transparent_balance: string;
  currency: string;
  decimals: number;
}

interface WalletTx {
  id: string;
  from_did: string;
  to_did: string;
  amount: string;
  fee: string;
  memo?: string;
  status: string;
  created_at: string;
  tx_type: string;
}

interface Supply {
  total: string;
  circulating: string;
  burned: string;
  currency: string;
}

/* ── Skeleton ── */
function SkeletonRow() {
  return (
    <div className="flex items-center gap-3 px-4 py-3.5 border-b border-[var(--border-1)]">
      <div className="w-10 h-10 rounded-full shimmer flex-shrink-0" />
      <div className="flex-1 space-y-1.5">
        <div className="h-3 w-32 rounded shimmer" />
        <div className="h-2.5 w-24 rounded shimmer" />
      </div>
      <div className="space-y-1.5 text-right">
        <div className="h-3 w-20 rounded shimmer ml-auto" />
        <div className="h-2.5 w-12 rounded shimmer ml-auto" />
      </div>
    </div>
  );
}

export default function WalletPage() {
  const router = useRouter();
  const { user } = useStore();

  const [balance, setBalance] = useState<WalletBalance | null>(null);
  const [txs, setTxs] = useState<WalletTx[]>([]);
  const [supply, setSupply] = useState<Supply | null>(null);
  const [loading, setLoading] = useState(true);
  const [hideBalance, setHideBalance] = useState(false);
  const [copied, setCopied] = useState(false);

  // Transfer modal
  const [transferOpen, setTransferOpen] = useState(false);
  const [toDID, setToDID] = useState("");
  const [amount, setAmount] = useState("");
  const [memo, setMemo] = useState("");
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState("");
  const [sendSuccess, setSendSuccess] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [bal, history, sup] = await Promise.all([
        api.walletBalance(),
        api.walletTransactions(20),
        api.walletSupply(),
      ]);
      setBalance(bal);
      setTxs(history?.transactions || []);
      setSupply(sup);
    } catch (e: unknown) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const copyDID = () => {
    if (!user?.did) return;
    navigator.clipboard.writeText(user.did).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };

  const handleTransfer = async () => {
    if (!toDID.trim() || !amount.trim()) return;
    setSending(true);
    setSendError("");
    setSendSuccess("");
    try {
      const result = await api.walletTransfer({ to_did: toDID.trim(), amount: amount.trim(), memo: memo.trim() || undefined });
      setSendSuccess(`Transfer başarılı! TX: ${result.tx_id?.slice(0, 12)}...`);
      setToDID("");
      setAmount("");
      setMemo("");
      await load();
      setTimeout(() => { setTransferOpen(false); setSendSuccess(""); }, 2000);
    } catch (e: unknown) {
      setSendError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setSending(false);
    }
  };

  const isOutgoing = (tx: WalletTx) => tx.from_did === user?.did;

  const formatBalance = (bal: string) => {
    const n = parseFloat(bal);
    if (isNaN(n)) return "0";
    return n.toLocaleString("tr-TR", { maximumFractionDigits: 4 });
  };

  /* ── USD placeholder (no price feed) ── */
  const usdValue = balance
    ? (parseFloat(balance.transparent_balance || "0") * 0.42).toLocaleString("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 })
    : "$0.00";

  const txIcon = (tx: WalletTx) => {
    const out = isOutgoing(tx);
    if (tx.status === "pending") return { Icon: Clock, color: "text-[var(--warning)]", bg: "bg-[rgba(255,170,0,0.12)]" };
    return out
      ? { Icon: ArrowUpRight, color: "text-[var(--error)]", bg: "bg-[rgba(255,64,88,0.10)]" }
      : { Icon: ArrowDownLeft, color: "text-[var(--accent)]", bg: "bg-[var(--accent-muted)]" };
  };

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Page Header ── */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">Cüzdan</h1>
          <button
            onClick={load}
            aria-label="Yenile"
            className="btn-icon"
          >
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>

        {/* ── Balance Hero ── */}
        <div className="mx-4 mb-4 rounded-[24px] relative overflow-hidden"
          style={{
            background: "linear-gradient(145deg, rgba(0,229,160,0.08) 0%, rgba(0,229,160,0.03) 50%, rgba(7,7,26,0) 100%)",
            border: "1px solid rgba(0,229,160,0.14)",
            boxShadow: "0 0 40px rgba(0,229,160,0.05), 0 8px 32px rgba(0,0,0,0.6)",
          }}
        >
          {/* Ambient glow blob */}
          <div className="absolute -top-8 -right-8 w-32 h-32 rounded-full"
            style={{ background: "radial-gradient(circle, rgba(0,229,160,0.10) 0%, transparent 70%)", pointerEvents: "none" }} />

          <div className="relative p-5">
            {/* Label row */}
            <div className="flex items-center justify-between mb-1">
              <span className="section-label">OBS Bakiye</span>
              <button
                onClick={() => setHideBalance(v => !v)}
                aria-label={hideBalance ? "Bakiyeyi göster" : "Bakiyeyi gizle"}
                className="btn-icon w-8 h-8"
              >
                {hideBalance ? <EyeOff size={14} /> : <Eye size={14} />}
              </button>
            </div>

            {/* Big balance number */}
            <div className="flex items-baseline gap-2.5 mt-2 mb-1">
              <span
                className="text-display"
                style={{ fontSize: "clamp(36px, 10vw, 48px)", color: "var(--text-1)", letterSpacing: "-0.04em", fontWeight: 800 }}
              >
                {loading
                  ? <span className="inline-block w-36 h-10 rounded shimmer align-middle" />
                  : hideBalance
                    ? "••••••"
                    : formatBalance(balance?.transparent_balance || "0")
                }
              </span>
              <span className="font-display text-base font-600" style={{ color: "var(--text-3)", fontWeight: 600 }}>OBS</span>
            </div>

            {/* USD value */}
            <p className="text-sm mb-4" style={{ color: "var(--text-2)" }}>
              {hideBalance ? "$ •••••" : usdValue}
            </p>

            {/* DID address row */}
            <div className="flex items-center gap-2 pt-4"
              style={{ borderTop: "1px solid var(--border-1)" }}>
              <span className="font-mono text-[11px] flex-1 truncate" style={{ color: "var(--text-3)" }}>
                {user?.did ? `${user.did.slice(0, 22)}...${user.did.slice(-8)}` : "—"}
              </span>
              <button
                onClick={copyDID}
                aria-label="DID adresini kopyala"
                className="btn-accent-ghost h-7 px-3 text-[11px]"
              >
                {copied ? <><Check size={11} /> Kopyalandı</> : <><Copy size={11} /> Kopyala</>}
              </button>
            </div>
          </div>
        </div>

        {/* ── Quick Actions ── */}
        <div className="grid grid-cols-3 gap-2 mx-4 mb-5">
          {[
            { label: "Gönder", Icon: ArrowUpRight, action: () => setTransferOpen(true) },
            { label: "Al", Icon: ArrowDownLeft, action: () => {} },
            { label: "Köprü", Icon: ArrowRightLeft, action: () => router.push("/bridge") },
          ].map(({ label, Icon, action }) => (
            <button
              key={label}
              onClick={action}
              className="btn-secondary flex-col h-16 gap-1.5 rounded-[16px]"
            >
              <Icon size={18} />
              <span className="text-xs">{label}</span>
            </button>
          ))}
        </div>

        {/* ── Shielded Pool link ── */}
        <div className="mx-4 mb-5 flex items-center justify-between rounded-[16px] px-4 py-3"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          <div className="flex items-center gap-2.5">
            <ShieldCheck size={18} style={{ color: "var(--accent)" }} />
            <div>
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>Gizli Havuz</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>ZK-korumalı transferler</p>
            </div>
          </div>
          <button
            onClick={() => router.push("/wallet/shielded")}
            className="btn-accent-ghost text-xs h-8 px-3"
          >
            Gizli Havuz <ArrowUpRight size={11} />
          </button>
        </div>

        {/* ── Supply stats ── */}
        {supply && (
          <div className="mx-4 mb-5 grid grid-cols-3 gap-2">
            {[
              { label: "Toplam Arz", val: formatBalance(supply.total) },
              { label: "Dolaşım", val: formatBalance(supply.circulating) },
              { label: "Yakılan", val: formatBalance(supply.burned) },
            ].map(({ label, val }) => (
              <div key={label} className="stat-box text-center">
                <span className="stat-value" style={{ fontSize: 16 }}>{val}</span>
                <span className="stat-label">{label}</span>
              </div>
            ))}
          </div>
        )}

        {/* ── Transactions ── */}
        <div className="px-4 mb-2.5">
          <span className="section-label">Son İşlemler</span>
        </div>

        <div className="mx-4 rounded-[20px] overflow-hidden mb-32"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          {loading ? (
            <>
              <SkeletonRow />
              <SkeletonRow />
              <SkeletonRow />
            </>
          ) : txs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 gap-3">
              <ArrowDownLeft size={28} style={{ color: "var(--text-3)" }} />
              <p className="text-sm" style={{ color: "var(--text-3)" }}>Henüz işlem yok</p>
            </div>
          ) : (
            txs.map((tx, i) => {
              const out = isOutgoing(tx);
              const { Icon, color, bg } = txIcon(tx);
              return (
                <div
                  key={tx.id}
                  className={cn(
                    "flex items-center gap-3 px-4 py-3.5 transition-colors",
                    i < txs.length - 1 && "border-b border-[var(--border-1)]",
                  )}
                >
                  {/* Icon circle */}
                  <div className={cn("w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0", bg)}>
                    <Icon size={16} className={color} />
                  </div>

                  {/* Description */}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold truncate" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                      {out ? "Gönderildi" : "Alındı"}
                      {tx.memo && (
                        <span className="font-normal ml-1.5" style={{ color: "var(--text-3)" }}>· {tx.memo}</span>
                      )}
                    </p>
                    <p className="text-xs font-mono truncate mt-0.5" style={{ color: "var(--text-3)" }}>
                      {out ? tx.to_did?.slice(0, 18) + "…" : tx.from_did?.slice(0, 18) + "…"}
                    </p>
                  </div>

                  {/* Amount + time */}
                  <div className="text-right flex-shrink-0">
                    <p className={cn("text-sm font-semibold font-display", color)}>
                      {out ? "−" : "+"}{formatBalance(tx.amount)} OBS
                    </p>
                    <p className="text-[11px] mt-0.5" style={{ color: "var(--text-3)" }}>{formatTime(tx.created_at)}</p>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* ── Transfer Modal ── */}
      {transferOpen && (
        <div
          className="fixed inset-0 z-50 flex items-end"
          role="dialog"
          aria-modal="true"
          aria-label="OBS Gönder"
        >
          <div
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            onClick={() => setTransferOpen(false)}
          />
          <div
            className="relative w-full rounded-t-[28px] px-5 pt-5 pb-10 animate-slide-up"
            style={{ background: "var(--surface-2)", borderTop: "1px solid var(--border-2)" }}
          >
            <div className="w-10 h-1 rounded-full mx-auto mb-5" style={{ background: "var(--border-3)" }} />
            <h2 className="text-lg font-display font-bold mb-5" style={{ color: "var(--text-1)" }}>OBS Gönder</h2>

            <div className="space-y-3">
              <div>
                <label className="section-label block mb-2">Alıcı DID</label>
                <input
                  value={toDID}
                  onChange={e => setToDID(e.target.value)}
                  placeholder="did:obs:..."
                  className="field font-mono"
                  autoFocus
                  onKeyDown={e => e.key === "Escape" && setTransferOpen(false)}
                />
              </div>
              <div>
                <label className="section-label block mb-2">Miktar (OBS)</label>
                <input
                  value={amount}
                  onChange={e => setAmount(e.target.value)}
                  placeholder="0.0000"
                  type="number"
                  min="0"
                  step="0.0001"
                  className="field"
                />
              </div>
              <div>
                <label className="section-label block mb-2">Not (isteğe bağlı)</label>
                <input
                  value={memo}
                  onChange={e => setMemo(e.target.value)}
                  placeholder="İşlem notu"
                  maxLength={256}
                  className="field"
                />
              </div>

              {sendError && (
                <div className="rounded-2xl px-4 py-3 text-sm" style={{ background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)", color: "var(--error)" }}>
                  {sendError}
                </div>
              )}
              {sendSuccess && (
                <div className="rounded-2xl px-4 py-3 text-sm" style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)", color: "var(--accent)" }}>
                  {sendSuccess}
                </div>
              )}
            </div>

            <div className="flex gap-2.5 mt-5">
              <button
                onClick={() => setTransferOpen(false)}
                className="btn-secondary flex-1"
              >
                İptal
              </button>
              <button
                onClick={handleTransfer}
                disabled={sending || !toDID.trim() || !amount.trim()}
                className="btn-primary flex-1"
              >
                {sending ? <><Loader2 size={15} className="animate-spin" /> Gönderiliyor…</> : "Gönder"}
              </button>
            </div>
          </div>
        </div>
      )}
    </AppShell>
  );
}
