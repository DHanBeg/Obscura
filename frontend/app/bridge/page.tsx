"use client";

import { useState, useEffect, useCallback } from "react";
import { ArrowUpDown, Lock, Loader2, RefreshCw, AlertTriangle, CheckCircle2, ExternalLink, Clock } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

type Chain = "ethereum" | "polkadot";

const CHAIN_META: Record<Chain, { name: string; symbol: string; hex: string; abbr: string }> = {
  ethereum: { name: "Ethereum",  symbol: "ETH", hex: "#627EEA", abbr: "ETH" },
  polkadot: { name: "Polkadot",  symbol: "DOT", hex: "#E6007A", abbr: "DOT" },
};

const STATUS_BADGE: Record<string, { label: string; cls: string }> = {
  pending:   { label: "Bekliyor",  cls: "badge-warning" },
  confirmed: { label: "Onaylandı", cls: "badge-success" },
  failed:    { label: "Başarısız", cls: "badge-error"   },
  completed: { label: "Tamamlandı",cls: "badge-success" },
};

function ChainPill({ chain }: { chain: Chain }) {
  const meta = CHAIN_META[chain];
  return (
    <div className="flex items-center gap-2">
      <div
        className="w-5 h-5 rounded-full flex-shrink-0"
        style={{ background: meta.hex + "30", border: `1.5px solid ${meta.hex}60` }}
      />
      <span className="font-bold text-sm" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
        {meta.name}
      </span>
      <span className="text-xs px-1.5 py-0.5 rounded-full" style={{ background: meta.hex + "15", color: meta.hex }}>
        {meta.abbr}
      </span>
    </div>
  );
}

export default function BridgePage() {
  const [status, setStatus]       = useState<any>(null);
  const [loading, setLoading]     = useState(true);
  const [fromChain, setFrom]      = useState<Chain>("ethereum");
  const [toChain, setTo]          = useState<Chain>("polkadot");
  const [amount, setAmount]       = useState("");
  const [senderAddr, setSender]   = useState("");
  const [recipientAddr, setRecip] = useState("");
  const [locking, setLocking]     = useState(false);
  const [result, setResult]       = useState<{ txId: string; status: string } | null>(null);
  const [error, setError]         = useState("");
  const [swapRotate, setSwapRotate] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api.bridgeStatus();
      setStatus(res);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const swap = () => {
    setSwapRotate(r => !r);
    setFrom(toChain);
    setTo(fromChain);
    setResult(null);
    setError("");
  };

  const handleLock = async () => {
    if (!amount || !senderAddr || !recipientAddr) {
      setError("Tüm alanları doldurun");
      return;
    }
    setLocking(true);
    setError("");
    setResult(null);
    try {
      const res = await api.bridgeLock({
        from_chain: fromChain,
        to_chain: toChain,
        amount: amount.trim(),
        sender_addr: senderAddr.trim(),
        recipient_addr: recipientAddr.trim(),
      });
      setResult({ txId: res?.tx_id || "stub", status: res?.status || "pending" });
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLocking(false);
    }
  };

  const canSubmit = !!amount && !!senderAddr && !!recipientAddr && !locking;

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Header ── */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Köprü</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>Zincirler arası OBS transferi</p>
          </div>
          <button onClick={load} aria-label="Yenile" className="btn-icon">
            <RefreshCw size={15} />
          </button>
        </div>

        {/* ── Chain status badges ── */}
        {!loading && status && (
          <div className="flex gap-2 px-5 mb-4">
            {(status.chains || ["ethereum", "polkadot"]).map((c: string) => (
              <div
                key={c}
                className="flex items-center gap-1.5 h-7 px-3 rounded-full text-[11px] font-semibold"
                style={{ background: "var(--accent-muted)", color: "var(--accent)", border: "1px solid rgba(0,229,160,0.2)" }}
              >
                <span className="w-1.5 h-1.5 rounded-full bg-[var(--accent)] animate-pulse-accent" />
                {c.charAt(0).toUpperCase() + c.slice(1)} Online
              </div>
            ))}
          </div>
        )}

        <div className="px-5 space-y-4">

          {/* ── Swap interface ── */}
          <div
            className="rounded-3xl p-5 space-y-1"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
          >
            {/* FROM */}
            <div
              className="rounded-2xl px-4 py-3.5"
              style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}
            >
              <p className="section-label mb-2">Kaynak</p>
              <div className="flex items-center justify-between">
                <ChainPill chain={fromChain} />
                <div className="text-right">
                  <p className="text-[10px] mb-1" style={{ color: "var(--text-3)" }}>Bakiye</p>
                  <p className="text-xs font-bold" style={{ color: "var(--text-2)" }}>— OBS</p>
                </div>
              </div>
              <input
                value={amount}
                onChange={e => setAmount(e.target.value)}
                placeholder="0.0000"
                aria-label="Transfer miktarı"
                className="mt-3 w-full bg-transparent border-none outline-none text-2xl font-black placeholder:text-[var(--text-3)]"
                style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}
              />
            </div>

            {/* SWAP button */}
            <div className="flex justify-center py-1">
              <button
                onClick={swap}
                aria-label="Zincirleri değiştir"
                className="w-10 h-10 rounded-xl flex items-center justify-center transition-all duration-300 active:scale-90"
                style={{ background: "var(--overlay)", border: "1px solid var(--border-2)" }}
              >
                <ArrowUpDown
                  size={16}
                  className={cn("transition-transform duration-300", swapRotate && "rotate-180")}
                  style={{ color: "var(--accent)" }}
                />
              </button>
            </div>

            {/* TO */}
            <div
              className="rounded-2xl px-4 py-3.5"
              style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}
            >
              <p className="section-label mb-2">Hedef</p>
              <div className="flex items-center justify-between">
                <ChainPill chain={toChain} />
                <div className="text-right">
                  <p className="text-[10px] mb-1" style={{ color: "var(--text-3)" }}>Alınacak</p>
                  <p className="text-xs font-bold" style={{ color: "var(--accent)" }}>
                    {amount ? `≈ ${amount} OBS` : "—"}
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* ── Address fields ── */}
          <div
            className="rounded-2xl p-4 space-y-3"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
          >
            <p className="section-label">Adresler</p>
            <div>
              <label className="block text-[11px] mb-1.5" style={{ color: "var(--text-3)" }}>
                Gönderici — {CHAIN_META[fromChain].name}
              </label>
              <input
                value={senderAddr}
                onChange={e => setSender(e.target.value)}
                placeholder="0x..."
                aria-label="Gönderici adresi"
                className="field text-xs h-10 font-mono"
              />
            </div>
            <div>
              <label className="block text-[11px] mb-1.5" style={{ color: "var(--text-3)" }}>
                Alıcı — {CHAIN_META[toChain].name}
              </label>
              <input
                value={recipientAddr}
                onChange={e => setRecip(e.target.value)}
                placeholder="0x..."
                aria-label="Alıcı adresi"
                className="field text-xs h-10 font-mono"
              />
            </div>
          </div>

          {/* ── Notice ── */}
          <div
            className="flex items-start gap-2 rounded-2xl p-3 text-xs"
            style={{ background: "rgba(255,170,0,0.05)", border: "1px solid rgba(255,170,0,0.15)", color: "var(--warning)" }}
          >
            <AlertTriangle size={13} className="flex-shrink-0 mt-0.5" />
            <span style={{ color: "var(--text-2)" }}>
              FAZ 3 stub — gerçek relayer ve on-chain doğrulama FAZ 4&apos;te aktif olacak.
            </span>
          </div>

          {/* ── Error ── */}
          {error && (
            <div className="rounded-2xl px-4 py-3 text-sm animate-slide-down"
              style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}>
              {error}
            </div>
          )}

          {/* ── Success result ── */}
          {result && (
            <div
              className="rounded-2xl p-4 space-y-2 animate-slide-up"
              style={{ background: "rgba(0,229,160,0.05)", border: "1px solid rgba(0,229,160,0.18)" }}
            >
              <div className="flex items-center gap-2">
                <CheckCircle2 size={15} style={{ color: "var(--accent)" }} />
                <span className="text-sm font-semibold" style={{ color: "var(--accent)" }}>Kilitleme başlatıldı</span>
              </div>
              <div className="flex items-center justify-between">
                <p className="text-xs font-mono truncate flex-1" style={{ color: "var(--text-2)" }}>
                  TX: {result.txId}
                </p>
                <ExternalLink size={11} style={{ color: "var(--text-3)" }} className="flex-shrink-0 ml-2" />
              </div>
              <div className="flex items-center gap-1.5">
                <span className={cn("badge", STATUS_BADGE[result.status]?.cls ?? "badge-neutral")}>
                  {STATUS_BADGE[result.status]?.label ?? result.status}
                </span>
              </div>
            </div>
          )}

          {/* ── Submit ── */}
          <button
            onClick={handleLock}
            disabled={!canSubmit}
            className="btn-primary w-full"
          >
            {locking ? <Loader2 size={16} className="animate-spin" /> : <Lock size={16} />}
            {locking ? "İşleniyor..." : "OBS Kilitle & Köprüle"}
          </button>

          {/* ── Recent transactions ── */}
          {status?.recent_txs && status.recent_txs.length > 0 && (
            <div>
              <p className="section-label mb-3">Son İşlemler</p>
              <div className="space-y-2">
                {status.recent_txs.map((tx: any, i: number) => (
                  <div
                    key={tx.tx_id || i}
                    className="flex items-center gap-3 rounded-2xl px-4 py-3"
                    style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
                  >
                    <span className="text-xs font-mono truncate flex-1" style={{ color: "var(--text-2)" }}>
                      {(tx.tx_id || "").slice(0, 14)}&hellip;
                    </span>
                    <span className={cn("badge", STATUS_BADGE[tx.status]?.cls ?? "badge-neutral")}>
                      {STATUS_BADGE[tx.status]?.label ?? tx.status}
                    </span>
                    {tx.amount && (
                      <span className="text-xs font-bold flex-shrink-0" style={{ color: "var(--text-1)" }}>
                        {tx.amount} OBS
                      </span>
                    )}
                    {tx.created_at && (
                      <span className="text-[10px] flex items-center gap-0.5 flex-shrink-0" style={{ color: "var(--text-3)" }}>
                        <Clock size={9} />
                        {tx.created_at}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
