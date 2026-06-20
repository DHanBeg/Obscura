"use client";

import { useState } from "react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { Shield, AlertTriangle, Copy, CheckCheck, Eye, EyeOff, Loader2, CheckCircle } from "lucide-react";

type Tab = "setup" | "recover";

interface Share {
  index: number;
  value: number[];
}

export default function RecoveryPage() {
  const [tab, setTab] = useState<Tab>("setup");

  // Setup flow
  const [guardians, setGuardians] = useState<string[]>(["", "", "", "", ""]);
  const [shares, setShares] = useState<Share[] | null>(null);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);
  const [splitting, setSplitting] = useState(false);
  const [splitError, setSplitError] = useState("");

  // Recover flow
  const [inputShares, setInputShares] = useState<string[]>(["", "", ""]);
  const [recoveredMnemonic, setRecoveredMnemonic] = useState("");
  const [showRecovered, setShowRecovered] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [recoverError, setRecoverError] = useState("");
  const [mnemonicCopied, setMnemonicCopied] = useState(false);

  async function handleSplit() {
    setSplitting(true);
    setSplitError("");
    setShares(null);
    try {
      const res = await api.shamirSplit("", 3, 5);
      if (!res?.shares) throw new Error("Paylaşım oluşturulamadı");
      setShares(res.shares as Share[]);
    } catch (e: unknown) {
      setSplitError(e instanceof Error ? e.message : "Hata oluştu");
    } finally {
      setSplitting(false);
    }
  }

  function copyShare(idx: number) {
    if (!shares) return;
    const share = shares[idx];
    const encoded = JSON.stringify({ index: share.index, value: share.value });
    navigator.clipboard.writeText(encoded).then(() => {
      setCopiedIdx(idx);
      setTimeout(() => setCopiedIdx(null), 2000);
    });
  }

  async function handleCombine() {
    setRecovering(true);
    setRecoverError("");
    setRecoveredMnemonic("");
    try {
      const parsedShares = inputShares
        .filter((s) => s.trim())
        .map((s) => {
          try { return JSON.parse(s.trim()); }
          catch { throw new Error(`Geçersiz paylaşım formatı: ${s.slice(0, 20)}...`); }
        });
      if (parsedShares.length < 3) throw new Error("En az 3 paylaşım gerekli");
      const res = await api.shamirCombine(parsedShares);
      if (!res?.mnemonic) throw new Error("Mnemonic kurtarılamadı");
      setRecoveredMnemonic(res.mnemonic);
    } catch (e: unknown) {
      setRecoverError(e instanceof Error ? e.message : "Kurtarma başarısız");
    } finally {
      setRecovering(false);
    }
  }

  function copyMnemonic() {
    navigator.clipboard.writeText(recoveredMnemonic).then(() => {
      setMnemonicCopied(true);
      setTimeout(() => setMnemonicCopied(false), 2000);
    });
  }

  return (
    <AppShell showBack title="Sosyal Kurtarma">
      <div className="scroll-area h-full pb-28">
        <div className="px-4 pt-4 space-y-4 animate-slide-up">

          <div className="flex items-center gap-3 mb-2">
            <div className="w-12 h-12 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
              <Shield size={22} style={{ color: "var(--accent)" }} />
            </div>
            <div>
              <h1 className="page-title">Sosyal Kurtarma</h1>
              <p className="text-sm" style={{ color: "var(--text-2)" }}>Shamir 3-of-5 paylaşım şeması</p>
            </div>
          </div>

          {/* Tab switcher */}
          <div className="flex rounded-2xl p-1" style={{ background: "var(--surface-2)" }}>
            {(["setup", "recover"] as Tab[]).map((t) => (
              <button key={t} onClick={() => setTab(t)}
                className="flex-1 py-2.5 rounded-xl text-sm font-semibold transition-all"
                style={{
                  background: tab === t ? "var(--surface-3)" : "transparent",
                  color: tab === t ? "var(--text-1)" : "var(--text-3)",
                  fontFamily: "var(--font-display)",
                }}>
                {t === "setup" ? "Kurulum" : "Kurtarma"}
              </button>
            ))}
          </div>

          {/* ── SETUP TAB ── */}
          {tab === "setup" && (
            <div className="space-y-4">
              <div className="px-4 py-3 rounded-2xl"
                style={{ background: "rgba(245,158,11,0.06)", border: "1px solid rgba(245,158,11,0.2)" }}>
                <div className="flex items-start gap-2">
                  <AlertTriangle size={15} className="mt-0.5 shrink-0" style={{ color: "#f59e0b" }} />
                  <p className="text-xs" style={{ color: "var(--text-2)", lineHeight: 1.6 }}>
                    Hesabınız 5 paylaşıma bölünür. Bunlardan herhangi 3&apos;ü birleşince hesabınız kurtarılabilir. Paylaşımları 5 farklı güvendiğiniz kişiye verin.
                  </p>
                </div>
              </div>

              <button onClick={handleSplit} disabled={splitting}
                className="btn-primary w-full">
                {splitting
                  ? <><Loader2 size={15} className="animate-spin" /> Paylaşımlar oluşturuluyor...</>
                  : "5 Paylaşım Oluştur"}
              </button>

              {splitError && (
                <p className="text-xs px-3" style={{ color: "#ef4444" }}>{splitError}</p>
              )}

              {shares && (
                <div className="space-y-2">
                  <div className="section-label">PAYLAŞIMLAR (her birini farklı kişiye ver)</div>
                  {shares.map((share, i) => (
                    <div key={i} className="card p-4 flex items-center gap-3">
                      <div className="w-8 h-8 rounded-xl flex items-center justify-center shrink-0"
                        style={{ background: "var(--accent-muted)" }}>
                        <span className="text-sm font-bold" style={{ color: "var(--accent)", fontFamily: "var(--font-mono)" }}>
                          {i + 1}
                        </span>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="text-xs font-mono truncate" style={{ color: "var(--text-3)" }}>
                          {JSON.stringify({ index: share.index, value: share.value }).slice(0, 40)}…
                        </div>
                      </div>
                      <button className="btn-icon" onClick={() => copyShare(i)}>
                        {copiedIdx === i
                          ? <CheckCheck size={15} style={{ color: "var(--accent)" }} />
                          : <Copy size={15} />}
                      </button>
                    </div>
                  ))}
                  <div className="flex items-center gap-2 px-3 py-2 rounded-2xl mt-2"
                    style={{ background: "rgba(0,229,160,0.04)", border: "1px solid rgba(0,229,160,0.15)" }}>
                    <CheckCircle size={14} style={{ color: "var(--accent)" }} />
                    <p className="text-xs" style={{ color: "var(--text-2)" }}>
                      Tüm paylaşımları kaydettikten sonra orijinal kopyaları silin.
                    </p>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* ── RECOVER TAB ── */}
          {tab === "recover" && (
            <div className="space-y-4">
              <div className="px-4 py-3 rounded-2xl"
                style={{ background: "rgba(239,68,68,0.06)", border: "1px solid rgba(239,68,68,0.2)" }}>
                <div className="flex items-start gap-2">
                  <AlertTriangle size={15} className="mt-0.5 shrink-0" style={{ color: "#ef4444" }} />
                  <p className="text-xs" style={{ color: "var(--text-2)", lineHeight: 1.6 }}>
                    Hesabınızı kurtarmak için güvendiğiniz kişilerden 3 paylaşım isteyin ve aşağıya yapıştırın.
                  </p>
                </div>
              </div>

              {[0, 1, 2].map((i) => (
                <div key={i}>
                  <div className="section-label mb-1">PAYLAŞIM {i + 1}</div>
                  <textarea
                    className="field resize-none h-20 text-xs"
                    style={{ fontFamily: "var(--font-mono)" }}
                    placeholder={`{"index":${i + 1},"value":[...]}`}
                    value={inputShares[i]}
                    onChange={(e) => {
                      const next = [...inputShares];
                      next[i] = e.target.value;
                      setInputShares(next);
                    }}
                  />
                </div>
              ))}

              <button onClick={handleCombine}
                disabled={recovering || inputShares.filter((s) => s.trim()).length < 3}
                className="btn-primary w-full">
                {recovering
                  ? <><Loader2 size={15} className="animate-spin" /> Kurtarılıyor...</>
                  : "Hesabı Kurtar"}
              </button>

              {recoverError && (
                <p className="text-xs px-3" style={{ color: "#ef4444" }}>{recoverError}</p>
              )}

              {recoveredMnemonic && (
                <div className="card p-4 space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                      Kurtarılan Mnemonic
                    </span>
                    <div className="flex gap-2">
                      <button className="btn-icon" onClick={() => setShowRecovered(!showRecovered)}>
                        {showRecovered ? <EyeOff size={15} /> : <Eye size={15} />}
                      </button>
                      <button className="btn-icon" onClick={copyMnemonic}>
                        {mnemonicCopied
                          ? <CheckCheck size={15} style={{ color: "var(--accent)" }} />
                          : <Copy size={15} />}
                      </button>
                    </div>
                  </div>
                  <div className="grid grid-cols-3 gap-2"
                    style={{ filter: showRecovered ? "none" : "blur(8px)", transition: "filter 0.2s" }}>
                    {recoveredMnemonic.split(" ").map((word, i) => (
                      <div key={i} className="flex items-center gap-1.5 px-2 py-1.5 rounded-xl"
                        style={{ background: "var(--surface-3)" }}>
                        <span className="text-[10px] w-4 text-right" style={{ color: "var(--text-3)", fontFamily: "var(--font-mono)" }}>
                          {i + 1}
                        </span>
                        <span className="text-xs font-medium" style={{ color: "var(--text-1)", fontFamily: "var(--font-mono)" }}>
                          {word}
                        </span>
                      </div>
                    ))}
                  </div>
                  <p className="text-xs" style={{ color: "var(--text-3)" }}>
                    Bu mnemonic&apos;i güvenli bir yere kaydedin. Ekrandan çıktığınızda tekrar gösterilmez.
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
