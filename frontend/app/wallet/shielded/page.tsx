"use client";

import { useState, useEffect, useCallback } from "react";
import { ShieldCheck, Lock, Loader2, Eye, EyeOff, Unlock } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { createShieldNote } from "@/lib/zk";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface ShieldedNote {
  nullifier: string;
  commitment: string;
  amount: string;
  created_at: string;
  spent: boolean;
}

/* ── Skeleton ── */
function SkeletonNote() {
  return (
    <div className="flex items-center gap-3 px-4 py-3.5 border-b border-[var(--border-1)]">
      <div className="w-10 h-10 rounded-full shimmer flex-shrink-0" />
      <div className="flex-1 space-y-1.5">
        <div className="h-3 w-28 rounded shimmer" />
        <div className="h-2.5 w-36 rounded shimmer" />
      </div>
      <div className="w-14 h-7 rounded-full shimmer" />
    </div>
  );
}

export default function ShieldedWalletPage() {
  const [notes, setNotes] = useState<ShieldedNote[]>([]);
  const [root, setRoot] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [shieldAmount, setShieldAmount] = useState("");
  const [shielding, setShielding] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [balanceVisible, setBalanceVisible] = useState(false);
  const [provingState, setProvingState] = useState<"idle" | "proving" | "done">("idle");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [notesRes, rootRes] = await Promise.all([
        api.walletShieldedNotes(),
        api.walletShieldedRoot(),
      ]);
      setNotes(notesRes?.notes || []);
      setRoot(rootRes?.merkle_root || "");
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleShield = async () => {
    if (!shieldAmount.trim()) return;
    setShielding(true);
    setProvingState("proving");
    setError("");
    setSuccess("");
    try {
      // Compute Poseidon commitment client-side; secret+salt never leave browser
      const { commitment, secret, salt } = await createShieldNote(shieldAmount);

      const result = await api.walletShield({ amount: shieldAmount, commitment });

      // Persist note locally so user can spend it later
      const noteKey = `obscura_note_${result?.leaf_index ?? Date.now()}`;
      localStorage.setItem(noteKey, JSON.stringify({
        leaf_index: result?.leaf_index,
        amount: shieldAmount,
        commitment,
        secret,
        salt,
        created_at: new Date().toISOString(),
      }));

      setProvingState("done");
      setSuccess("Başarıyla gizlendi! Shielded pool'a aktarıldı.");
      setShieldAmount("");
      await load();
    } catch (e: unknown) {
      setProvingState("idle");
      setError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally {
      setShielding(false);
      setTimeout(() => setProvingState("idle"), 2000);
    }
  };

  const unspentBalance = notes
    .filter(n => !n.spent)
    .reduce((sum, n) => sum + parseFloat(n.amount || "0"), 0)
    .toFixed(4);

  const unspentCount = notes.filter(n => !n.spent).length;

  return (
    <AppShell showBack title="Gizli Havuz">
      <div className="flex flex-col h-full scroll-area">

        {/* ── Hero ── */}
        <div className="mx-4 mt-4 mb-5 rounded-[24px] overflow-hidden relative"
          style={{
            background: "linear-gradient(145deg, rgba(77,168,255,0.08) 0%, rgba(0,229,160,0.04) 60%, rgba(7,7,26,0) 100%)",
            border: "1px solid rgba(77,168,255,0.14)",
            boxShadow: "0 0 60px rgba(77,168,255,0.06), 0 8px 32px rgba(0,0,0,0.7)",
          }}
        >
          {/* Iris ring decoration */}
          <div className="iris-bg absolute inset-0 pointer-events-none opacity-60" />

          {/* Ambient glow */}
          <div className="absolute -top-10 left-1/2 -translate-x-1/2 w-40 h-40 rounded-full pointer-events-none"
            style={{ background: "radial-gradient(circle, rgba(77,168,255,0.12) 0%, transparent 70%)" }} />

          <div className="relative flex flex-col items-center text-center px-6 py-8">
            {/* Shield icon */}
            <div className="mb-4 relative">
              <div className="w-16 h-16 rounded-full flex items-center justify-center"
                style={{ background: "rgba(0,229,160,0.06)", border: "1px solid rgba(0,229,160,0.18)" }}
              >
                <ShieldCheck size={32} style={{ color: "var(--accent)" }} />
              </div>
              {/* Pulse ring */}
              <div className="absolute inset-0 rounded-full animate-pulse-accent" />
            </div>

            <h1 className="font-display font-bold mb-1" style={{ fontSize: 24, color: "var(--text-1)", letterSpacing: "-0.025em" }}>
              Gizli Havuz
            </h1>
            <p className="text-sm mb-6" style={{ color: "var(--text-2)" }}>Miktarınız ve alıcınız tamamen gizli kalır</p>

            {/* Hidden balance display */}
            <div className="flex items-baseline gap-2 mb-2">
              <span
                className="font-display font-extrabold"
                style={{ fontSize: 38, color: "var(--text-1)", letterSpacing: "-0.04em" }}
              >
                {loading
                  ? <span className="inline-block w-28 h-9 rounded shimmer align-middle" />
                  : balanceVisible
                    ? unspentBalance
                    : <span style={{ letterSpacing: "0.12em" }}>{"⬤ ⬤ ⬤ ⬤ ⬤"}</span>
                }
              </span>
              {balanceVisible && (
                <span className="font-display text-base" style={{ color: "var(--text-3)", fontWeight: 600 }}>OBS</span>
              )}
            </div>
            <p className="text-xs mb-4" style={{ color: "var(--text-3)" }}>
              {balanceVisible ? `${unspentCount} aktif not` : "Bakiyeyi görmek için kilidi aç"}
            </p>

            <button
              onClick={() => setBalanceVisible(v => !v)}
              aria-label={balanceVisible ? "Bakiyeyi gizle" : "Bakiyeyi göster"}
              className="btn-ghost h-8 px-4 text-xs gap-1.5"
            >
              {balanceVisible ? <><EyeOff size={13} /> Gizle</> : <><Eye size={13} /> Göster</>}
            </button>

            {/* Merkle root */}
            {root && (
              <p className="font-mono text-[10px] mt-3 px-3 py-1.5 rounded-full"
                style={{ color: "var(--text-3)", background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
                Root: {root.slice(0, 20)}…{root.slice(-8)}
              </p>
            )}
          </div>
        </div>

        {/* ── Shield Action ── */}
        <div className="mx-4 mb-4 card p-4">
          <p className="text-sm font-semibold mb-1" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
            Gizli Havuza Aktar
          </p>
          <p className="text-xs mb-3" style={{ color: "var(--text-3)" }}>
            Miktar ve alıcı gizli kalır. İşlem izlenebilir değil.
          </p>

          {/* Quick amount buttons */}
          <div className="flex gap-2 mb-3">
            {["10", "50", "100", "500"].map((v) => (
              <button
                key={v}
                onClick={() => setShieldAmount(v)}
                className={cn(
                  "flex-1 h-8 rounded-xl text-xs font-semibold transition-all duration-150",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                )}
                style={{
                  background: shieldAmount === v ? "var(--accent-muted)" : "var(--surface-3)",
                  color: shieldAmount === v ? "var(--accent)" : "var(--text-3)",
                  border: shieldAmount === v ? "1px solid rgba(0,229,160,0.25)" : "1px solid transparent",
                }}
                aria-pressed={shieldAmount === v}
              >
                {v}
              </button>
            ))}
          </div>

          <div
            className="field mb-3 text-display"
            style={{ fontSize: 24, textAlign: "center", fontWeight: 800, letterSpacing: "-0.03em", height: 60, display: "flex", alignItems: "center", justifyContent: "center", padding: 0 }}
          >
            <input
              value={shieldAmount}
              onChange={e => setShieldAmount(e.target.value)}
              placeholder="0"
              type="number"
              min="0"
              step="0.0001"
              className="bg-transparent text-center w-full focus:outline-none"
              style={{ fontSize: 24, fontWeight: 800, letterSpacing: "-0.03em", color: "var(--text-1)", fontFamily: "var(--font-display)" }}
              aria-label="Miktar (OBS)"
              onKeyDown={e => e.key === "Enter" && !shielding && shieldAmount && handleShield()}
            />
          </div>

          <button
            onClick={handleShield}
            disabled={shielding || !shieldAmount}
            className="btn-primary w-full mb-3"
          >
            {shielding ? <Loader2 size={16} className="animate-spin" /> : <ShieldCheck size={16} />}
            Gizli Havuza Gönder
          </button>

          {/* ZK proof state indicator */}
          {provingState !== "idle" && (
            <div className={cn(
              "flex items-center gap-2 px-3 py-2.5 rounded-2xl text-sm",
              provingState === "proving"
                ? "bg-[rgba(77,168,255,0.08)] border border-[rgba(77,168,255,0.15)] text-[var(--signal)]"
                : "bg-[var(--accent-muted)] border border-[rgba(0,229,160,0.2)] text-[var(--accent)]"
            )}>
              {provingState === "proving"
                ? <><Loader2 size={14} className="animate-spin flex-shrink-0" /> Kanıt üretiliyor…</>
                : <><ShieldCheck size={14} className="flex-shrink-0" /> Kanıt doğrulandı</>
              }
            </div>
          )}

          {error && (
            <p className="text-sm mt-2 px-3 py-2 rounded-2xl"
              style={{ color: "var(--error)", background: "rgba(255,64,88,0.08)", border: "1px solid rgba(255,64,88,0.15)" }}>
              {error}
            </p>
          )}
          {success && !provingState && (
            <p className="text-sm mt-2 px-3 py-2 rounded-2xl"
              style={{ color: "var(--accent)", background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
              {success}
            </p>
          )}
        </div>

        {/* ── Notes List ── */}
        <div className="px-4 mb-2.5">
          <span className="section-label">Gizli Notlar</span>
        </div>

        <div className="mx-4 rounded-[20px] overflow-hidden mb-32"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          {loading ? (
            <>
              <SkeletonNote />
              <SkeletonNote />
            </>
          ) : notes.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 gap-3">
              <Lock size={28} style={{ color: "var(--text-3)" }} />
              <p className="text-sm" style={{ color: "var(--text-3)" }}>Henüz gizli not yok</p>
              <p className="text-xs" style={{ color: "var(--text-4)" }}>İlk transferini yukarıdan gönder</p>
            </div>
          ) : (
            notes.map((note, i) => (
              <div
                key={note.nullifier}
                className={cn(
                  "flex items-center gap-3 px-4 py-3.5",
                  i < notes.length - 1 && "border-b border-[var(--border-1)]",
                  note.spent && "opacity-50",
                )}
              >
                {/* Icon */}
                <div
                  className="w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0"
                  style={{
                    background: note.spent ? "var(--surface-3)" : "rgba(77,168,255,0.10)",
                    border: `1px solid ${note.spent ? "var(--border-1)" : "rgba(77,168,255,0.2)"}`,
                  }}
                >
                  {note.spent
                    ? <Unlock size={15} style={{ color: "var(--text-3)" }} />
                    : <Lock size={15} style={{ color: "var(--signal)" }} />
                  }
                </div>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    {note.spent ? "••••" : parseFloat(note.amount).toFixed(4)} OBS
                  </p>
                  <p className="text-[11px] font-mono truncate mt-0.5" style={{ color: "var(--text-3)" }}>
                    {note.commitment.slice(0, 18)}…
                  </p>
                </div>

                {/* Right side */}
                <div className="flex flex-col items-end gap-1 flex-shrink-0">
                  <span className="text-[11px]" style={{ color: "var(--text-3)" }}>{formatTime(note.created_at)}</span>
                  {note.spent ? (
                    <span className="badge badge-neutral">Harcandı</span>
                  ) : (
                    <button className="btn-ghost h-6 px-2.5 text-[11px]" style={{ color: "var(--accent)" }}>
                      Çıkar
                    </button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </AppShell>
  );
}
