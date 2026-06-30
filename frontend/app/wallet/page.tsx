"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowUpRight, ArrowDownLeft, ArrowRightLeft,
  Eye, EyeOff, Copy, Check, Loader2, ShieldCheck, Clock,
  RefreshCw, Lock, TrendingUp, Scale,
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

  const financeLinks = [
    { label: "Cüzdan", href: "/wallet", icon: <Lock size={12} /> },
    { label: "Staking", href: "/staking", icon: <TrendingUp size={12} /> },
    { label: "Bridge", href: "/bridge", icon: <ArrowRightLeft size={12} /> },
    { label: "DAO", href: "/dao", icon: <Scale size={12} /> },
  ];

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Finance Tab Bar ── */}
        <div
          className="flex-shrink-0 flex items-center gap-1.5 px-4 pt-3 pb-2 overflow-x-auto scrollbar-none"
        >
          {financeLinks.map((tab) => {
            const isActive = tab.href === "/wallet";
            return (
              <a
                key={tab.href}
                href={tab.href}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[11px] font-semibold flex-shrink-0 transition-all duration-150"
                style={{
                  background: isActive ? "rgba(0,229,160,0.12)" : "var(--surface-2)",
                  border: `1px solid ${isActive ? "rgba(0,229,160,0.25)" : "var(--border-1)"}`,
                  color: isActive ? "var(--accent)" : "var(--text-3)",
                }}
              >
                {tab.icon}
                {tab.label}
              </a>
            );
          })}
        </div>

        {/* ── Page Header ── */}
        <div
          className="flex-shrink-0 flex items-start justify-between px-5 pt-1 pb-2"
          style={{ borderBottom: "1px solid var(--border)" }}
        >
          <div>
            <h1
              className="text-[22px] font-bold"
              style={{ letterSpacing: "-0.02em", color: "var(--t1)", fontFamily: "var(--font-display)" }}
            >
              Gizli Cüzdan
            </h1>
            <div className="flex items-center gap-1.5 mt-1">
              <span style={{ fontSize: 11 }}>🛡</span>
              <span
                className="text-[10px] font-bold tracking-widest uppercase"
                style={{ fontFamily: "var(--font-mono)", color: "var(--em)" }}
              >
                Privacy Mode
              </span>
            </div>
          </div>
          <button
            onClick={load}
            aria-label="Yenile"
            className="btn-icon"
          >
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>

        {/* ── Balance Hero Card (dot grid pattern) ── */}
        <div
          className="bal-card mx-4 mb-4"
          style={{ boxShadow: "0 0 40px rgba(74,222,128,0.05), 0 8px 32px rgba(0,0,0,0.6)" }}
        >
          <div className="relative p-5" style={{ zIndex: 1 }}>
            {/* Top row: label + lock icon + eye toggle */}
            <div className="flex items-start justify-between mb-4">
              <div className="flex items-center gap-2">
                <div
                  className="w-7 h-7 rounded-lg flex items-center justify-center"
                  style={{ background: "var(--em-d)", border: "1px solid rgba(74,222,128,0.2)", fontSize: 13 }}
                  aria-hidden="true"
                >
                  🔒
                </div>
                <div>
                  <div className="text-[12px] font-medium" style={{ color: "var(--t3)" }}>
                    Toplam Bakiye
                  </div>
                  <div
                    className="text-[8px] font-mono font-bold px-1.5 py-0.5 rounded"
                    style={{ background: "var(--em-d)", color: "var(--em)" }}
                  >
                    GİZLİ
                  </div>
                </div>
              </div>
              <button
                onClick={() => setHideBalance(v => !v)}
                aria-label={hideBalance ? "Bakiyeyi göster" : "Bakiyeyi gizle"}
                className="w-7 h-7 rounded-lg flex items-center justify-center"
                style={{ background: "rgba(255,255,255,0.05)", border: "1px solid var(--border)" }}
              >
                {hideBalance ? <EyeOff size={13} style={{ color: "var(--t2)" }} /> : <Eye size={13} style={{ color: "var(--t2)" }} />}
              </button>
            </div>

            {/* Big balance number */}
            <div
              className="font-bold mb-2"
              style={{
                fontSize: "clamp(28px, 9vw, 36px)",
                letterSpacing: "-0.03em",
                color: "var(--t1)",
                fontFamily: "var(--font-display)",
                fontWeight: 800,
              }}
            >
              {loading
                ? <span className="inline-block w-36 h-9 rounded shimmer align-middle" />
                : hideBalance
                  ? "••••••"
                  : formatBalance(balance?.transparent_balance || "0") + " OBS"
              }
            </div>

            {/* Change indicator */}
            <div
              className="flex items-center gap-1 mb-4 text-[12px] font-medium"
              style={{ color: "var(--em)" }}
            >
              <span aria-hidden="true">▲</span>
              {hideBalance ? "•••••" : usdValue + " ≈ bugün"}
            </div>

            {/* DID address row */}
            <div
              className="flex items-center gap-2 px-3 py-2 rounded-lg"
              style={{ background: "rgba(0,0,0,0.25)", border: "1px solid var(--border)" }}
            >
              <span
                className="text-[10px] flex-1 truncate"
                style={{ fontFamily: "var(--font-mono)", color: "var(--t3)" }}
              >
                {user?.did ? `${user.did.slice(0, 12)}...${user.did.slice(-8)}` : "0x7F4a…c8D2"}
              </span>
              <button
                onClick={copyDID}
                aria-label="DID adresini kopyala"
                className="flex-shrink-0 text-[9px] font-mono font-bold"
                style={{ color: "var(--em)" }}
              >
                {copied ? "Kopyalandı" : "Kopyala"}
              </button>
            </div>
          </div>
        </div>

        {/* ── Quick Actions (4 buttons) ── */}
        <div className="grid grid-cols-4 gap-2 mx-4 mb-5">
          {[
            {
              label: "Gönder",
              icon: <ArrowUpRight size={17} style={{ color: "#4a9eff" }} />,
              action: () => setTransferOpen(true),
              ariaLabel: "OBS gönder",
              bg: "rgba(74,158,255,0.12)",
            },
            {
              label: "Al",
              icon: <ArrowDownLeft size={17} style={{ color: "var(--em)" }} />,
              action: () => {},
              ariaLabel: "OBS al",
              bg: "var(--em-d)",
            },
            {
              label: "Gizli",
              icon: <ShieldCheck size={17} style={{ color: "#a855f7" }} />,
              action: () => router.push("/wallet/shielded"),
              ariaLabel: "Gizli havuza aktar",
              bg: "rgba(168,85,247,0.12)",
            },
            {
              label: "Köprü",
              icon: <ArrowRightLeft size={17} style={{ color: "var(--amber)" }} />,
              action: () => router.push("/bridge"),
              ariaLabel: "Zincirlerarası köprü",
              bg: "var(--amb-d)",
            },
          ].map(({ label, icon, action, ariaLabel, bg }) => (
            <button
              key={label}
              onClick={action}
              aria-label={ariaLabel}
              className="flex flex-col items-center gap-1.5 py-3 rounded-xl transition-all duration-150 active:scale-95"
              style={{ background: "var(--bg2)", border: "1px solid var(--border)", fontSize: 11, fontWeight: 500, color: "var(--t1)" }}
            >
              <div
                className="w-8 h-8 rounded-full flex items-center justify-center"
                style={{ background: bg }}
                aria-hidden="true"
              >
                {icon}
              </div>
              {label}
            </button>
          ))}
        </div>

        {/* ── Asset rows — token list ── */}
        <div className="px-4 mb-2">
          <span
            className="text-[11px] font-bold tracking-widest uppercase"
            style={{ color: "var(--t3)", fontFamily: "var(--font-display)" }}
          >
            Varlıklar
          </span>
        </div>
        <div
          className="mx-4 mb-5 rounded-[20px] overflow-hidden"
          style={{ background: "var(--bg2)", border: "1px solid var(--border)" }}
        >
          {/* OBS */}
          <div
            className="flex items-center gap-3 px-4 py-3"
            style={{ borderBottom: "1px solid var(--border)" }}
          >
            <div
              className="w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 text-base"
              style={{ background: "rgba(74,222,128,0.1)", border: "1px solid var(--border)" }}
            >
              ⬡
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-[14px] font-semibold" style={{ color: "var(--t1)" }}>OBS Token</div>
              <div className="text-[10px] font-mono" style={{ color: "var(--t3)" }}>OBS</div>
            </div>
            <div className="text-right">
              <div className="text-[13px] font-semibold font-mono" style={{ color: "var(--t1)" }}>
                {loading ? <span className="inline-block w-16 h-3 rounded shimmer" /> : hideBalance ? "••••" : `${parseFloat(balance?.transparent_balance || "0").toLocaleString("tr-TR", { maximumFractionDigits: 2 })} OBS`}
              </div>
              <div className="flex items-center justify-end gap-1">
                <div className="text-[11px]" style={{ color: "var(--t3)" }}>{hideBalance ? "$ ••••" : usdValue}</div>
                <span className="text-[10px] font-mono font-semibold" style={{ color: "var(--em)" }}>+3.1%</span>
              </div>
            </div>
          </div>

          {/* Gizli */}
          <div
            className="flex items-center gap-3 px-4 py-3 cursor-pointer transition-colors"
            style={{ borderBottom: "1px solid var(--border)" }}
            onClick={() => router.push("/wallet/shielded")}
            role="button"
            tabIndex={0}
            aria-label="Gizli bakiye"
            onKeyDown={(e) => e.key === "Enter" && router.push("/wallet/shielded")}
            onMouseEnter={(e) => { (e.currentTarget as HTMLDivElement).style.background = "rgba(255,255,255,0.02)"; }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLDivElement).style.background = "transparent"; }}
          >
            <div
              className="w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(77,168,255,0.08)", border: "1px solid rgba(77,168,255,0.15)" }}
            >
              <ShieldCheck size={15} style={{ color: "var(--cyan)" }} />
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-[14px] font-semibold" style={{ color: "var(--t1)" }}>Gizli Bakiye</div>
              <div className="text-[10px]" style={{ color: "var(--t3)" }}>Miktar ve alıcı gizli</div>
            </div>
            <div className="text-right">
              <div className="text-[13px] font-semibold" style={{ color: "var(--t3)", letterSpacing: "0.08em" }}>••••••</div>
            </div>
          </div>

          {/* Kilitli OBS */}
          <div className="flex items-center gap-3 px-4 py-3">
            <div
              className="w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(245,158,11,0.08)", border: "1px solid rgba(245,158,11,0.15)" }}
            >
              <Lock size={15} style={{ color: "var(--amber)" }} />
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-[14px] font-semibold" style={{ color: "var(--t1)" }}>Kilitli OBS</div>
              <div className="text-[10px]" style={{ color: "var(--t3)" }}>Staking pozisyonları</div>
            </div>
            <div className="text-right">
              <div className="text-[13px] font-semibold font-mono" style={{ color: "var(--t1)" }}>— OBS</div>
            </div>
          </div>
        </div>

        {/* ── Supply stats ── */}
        {supply && (
          <div className="mx-4 mb-5 grid grid-cols-3 gap-2">
            {[
              { label: "Toplam Arz", val: formatBalance(supply.total) },
              { label: "Dolaşım", val: formatBalance(supply.circulating) },
              { label: "Yakılan", val: formatBalance(supply.burned) },
            ].map(({ label, val }) => (
              <div
                key={label}
                className="text-center rounded-2xl p-4"
                style={{ background: "var(--bg2)", border: "1px solid var(--border)" }}
              >
                <div
                  className="text-[16px] font-bold font-mono"
                  style={{ color: "var(--t1)", letterSpacing: "-0.04em" }}
                >
                  {val}
                </div>
                <div
                  className="text-[10px] font-bold tracking-wider uppercase mt-1"
                  style={{ color: "var(--t3)" }}
                >
                  {label}
                </div>
              </div>
            ))}
          </div>
        )}

        {/* ── Transactions ── */}
        <div className="px-4 mb-2.5">
          <span
            className="text-[11px] font-bold tracking-widest uppercase"
            style={{ color: "var(--t3)", fontFamily: "var(--font-display)" }}
          >
            Son İşlemler
          </span>
        </div>

        <div
          className="mx-4 rounded-[20px] overflow-hidden mb-32"
          style={{ background: "var(--bg2)", border: "1px solid var(--border)" }}
        >
          {loading ? (
            <>
              <SkeletonRow />
              <SkeletonRow />
              <SkeletonRow />
            </>
          ) : txs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 gap-3">
              <ArrowDownLeft size={28} style={{ color: "var(--t3)" }} />
              <p className="text-sm" style={{ color: "var(--t3)" }}>Henüz işlem yok</p>
            </div>
          ) : (
            txs.map((tx, i) => {
              const out = isOutgoing(tx);
              const isPending = tx.status === "pending";
              // Icon styles matching reference
              const iconBg = isPending
                ? "var(--amb-d)"
                : out
                  ? "var(--red-d)"
                  : "var(--em-d)";
              const iconColor = isPending
                ? "var(--amber)"
                : out
                  ? "var(--red)"
                  : "var(--em)";
              const Icon = isPending ? Clock : out ? ArrowUpRight : ArrowDownLeft;
              const amountColor = isPending
                ? "var(--amber)"
                : out
                  ? "var(--red)"
                  : "var(--em)";

              return (
                <div
                  key={tx.id}
                  className={cn(
                    "flex items-center gap-3 px-4 py-3 transition-colors",
                    i < txs.length - 1 && "border-b border-[var(--border)]",
                  )}
                >
                  {/* Icon circle */}
                  <div
                    className="w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0"
                    style={{ background: iconBg }}
                    aria-hidden="true"
                  >
                    <Icon size={15} style={{ color: iconColor }} />
                  </div>

                  {/* Description */}
                  <div className="flex-1 min-w-0">
                    <p className="text-[13px] font-semibold truncate" style={{ color: "var(--t1)" }}>
                      {out ? "Gönderildi" : "Alındı"}
                      {tx.memo && (
                        <span className="font-normal ml-1.5" style={{ color: "var(--t3)" }}>· {tx.memo}</span>
                      )}
                    </p>
                    <p className="text-[10px] font-mono truncate mt-0.5" style={{ color: "var(--t3)" }}>
                      {out ? tx.to_did?.slice(0, 16) + "…" : tx.from_did?.slice(0, 16) + "…"}
                    </p>
                    <div className="flex items-center gap-1 mt-0.5">
                      <span style={{ fontSize: 9 }}>🔒</span>
                      <span className="text-[8px] font-mono" style={{ color: "var(--t4)" }}>Özel</span>
                    </div>
                  </div>

                  {/* Amount + time */}
                  <div className="text-right flex-shrink-0">
                    <p
                      className="text-[13px] font-semibold font-mono"
                      style={{ color: amountColor }}
                    >
                      {out ? "−" : "+"}{formatBalance(tx.amount)} OBS
                    </p>
                    <p className="text-[11px] mt-0.5" style={{ color: "var(--t3)" }}>
                      {formatTime(tx.created_at)}
                    </p>
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
