"use client";

import { useState, useEffect, useCallback } from "react";
import { Layers, RefreshCw, Loader2, CheckCircle2, Clock, Hash, Cpu, Zap, BarChart2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

interface SequencerCandidate {
  node_id: string;
  did: string;
  stake: number;
  peer_addr?: string;
  registered_at: string;
}

interface Batch {
  id: string;
  sequencer_id: string;
  state_root: string;
  tx_count: number;
  submitted_at: string;
}

function timeSince(iso: string): string {
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 60) return `${sec}s önce`;
  if (sec < 3600) return `${Math.floor(sec / 60)}dk önce`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}sa önce`;
  return `${Math.floor(sec / 86400)}g önce`;
}

export default function SequencerPage() {
  const [candidates, setCandidates] = useState<SequencerCandidate[]>([]);
  const [active, setActive]         = useState<SequencerCandidate | null>(null);
  const [batches, setBatches]       = useState<Batch[]>([]);
  const [loading, setLoading]       = useState(true);
  const [tab, setTab]               = useState<"candidates" | "batches">("candidates");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [seqData, batchData] = await Promise.all([
        api.sequencerList(),
        api.sequencerBatches(),
      ]);
      setCandidates(seqData?.candidates || []);
      setActive(seqData?.active || null);
      setBatches(batchData?.batches || []);
    } catch {}
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const totalStake = candidates.reduce((s, c) => s + (c.stake || 0), 0);

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Header ── */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Sequencer</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>VRF stake-ağırlıklı rotasyon · 4s epochlar</p>
          </div>
          <button
            onClick={load}
            disabled={loading}
            aria-label="Yenile"
            className="btn-icon"
          >
            <RefreshCw size={15} className={cn(loading && "animate-spin")} />
          </button>
        </div>

        {/* ── Active sequencer card ── */}
        {active && (
          <div className="mx-5 mb-4 rounded-2xl p-4"
            style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}
          >
            <div className="flex items-center gap-2 mb-2">
              <span
                className="w-2 h-2 rounded-full flex-shrink-0"
                style={{ background: "var(--accent)", animation: "pulse 2s ease-in-out infinite" }}
              />
              <span
                className="text-xs font-bold uppercase tracking-wider"
                style={{ color: "var(--accent)" }}
              >
                Aktif Sequencer
              </span>
            </div>
            <p
              className="text-sm font-semibold font-mono"
              style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}
            >
              {active.node_id}
            </p>
            <p className="text-xs font-mono truncate mt-0.5" style={{ color: "var(--text-3)" }}>
              {active.did}
            </p>
            <div className="flex gap-4 mt-3">
              <div>
                <p className="section-label mb-0.5">Stake</p>
                <p className="text-sm font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                  {active.stake.toLocaleString()} OBS
                </p>
              </div>
              <div>
                <p className="section-label mb-0.5">Kayıt</p>
                <p className="text-sm font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                  {timeSince(active.registered_at)}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* ── Stats row ── */}
        <div className="grid grid-cols-3 gap-2 px-5 mb-4">
          {[
            { label: "Aday",         value: candidates.length.toString(),                           icon: Cpu },
            { label: "Toplam Stake", value: `${(totalStake / 1000).toFixed(1)}k OBS`,               icon: Zap },
            { label: "Son Batch",    value: batches.length > 0 ? timeSince(batches[0].submitted_at) : "—", icon: BarChart2 },
          ].map(({ label, value, icon: Icon }) => (
            <div key={label} className="stat-box">
              <Icon size={13} style={{ color: "var(--text-3)" }} />
              <span className="stat-value">{value}</span>
              <span className="stat-label">{label}</span>
            </div>
          ))}
        </div>

        {/* ── Tabs ── */}
        <div className="mx-5 mb-4 flex rounded-[16px] p-1"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          {(["candidates", "batches"] as const).map(t => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={cn(
                "flex-1 py-2 px-3 rounded-[12px] text-sm font-semibold transition-all duration-150",
                tab === t
                  ? "text-[var(--accent)]"
                  : "text-[var(--text-3)] hover:text-[var(--text-2)]"
              )}
              style={{
                fontFamily: "var(--font-display)",
                background: tab === t ? "var(--accent-muted)" : undefined,
              }}
            >
              {t === "candidates" ? "Adaylar" : "Batch'ler"}
            </button>
          ))}
        </div>

        {/* ── Content ── */}
        {loading ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 size={28} className="animate-spin" style={{ color: "var(--accent)" }} />
          </div>
        ) : tab === "candidates" ? (
          candidates.length === 0 ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
              <div className="w-14 h-14 rounded-3xl flex items-center justify-center"
                style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
                <Layers size={24} style={{ color: "var(--text-3)" }} />
              </div>
              <p className="text-sm" style={{ color: "var(--text-3)" }}>Henüz aday yok</p>
            </div>
          ) : (
            <div className="px-5 space-y-2">
              {candidates.map((c, i) => {
                const isActive = active?.node_id === c.node_id;
                const stakePct = totalStake > 0 ? Math.round((c.stake / totalStake) * 100) : 0;
                return (
                  <div
                    key={c.node_id}
                    className="rounded-2xl p-3"
                    style={{
                      background: "var(--surface-2)",
                      border: `1px solid ${isActive ? "rgba(0,229,160,0.3)" : "var(--border-1)"}`,
                    }}
                  >
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className="text-xs" style={{ color: "var(--text-3)" }}>#{i + 1}</span>
                        <span
                          className="text-sm font-semibold font-mono"
                          style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}
                        >
                          {c.node_id}
                        </span>
                        {isActive && (
                          <span
                            className="badge badge-accent text-[10px]"
                            style={{ fontFamily: "var(--font-display)" }}
                          >
                            Aktif
                          </span>
                        )}
                      </div>
                      <span
                        className="text-sm font-semibold"
                        style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}
                      >
                        {c.stake.toLocaleString()} OBS
                      </span>
                    </div>

                    {/* Stake weight bar */}
                    <div
                      className="h-1.5 rounded-full overflow-hidden"
                      style={{ background: "var(--surface-3)" }}
                    >
                      <div
                        className="h-full rounded-full transition-all duration-500"
                        style={{
                          width: `${stakePct}%`,
                          background: isActive ? "var(--accent)" : "rgba(0,229,160,0.4)",
                        }}
                      />
                    </div>

                    <div className="flex justify-between mt-1.5">
                      <span className="text-[10px] font-mono truncate max-w-[60%]" style={{ color: "var(--text-3)" }}>
                        {c.peer_addr || "—"}
                      </span>
                      <span className="text-[10px]" style={{ color: "var(--text-3)" }}>%{stakePct} ağırlık</span>
                    </div>
                  </div>
                );
              })}
            </div>
          )
        ) : (
          batches.length === 0 ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
              <div className="w-14 h-14 rounded-3xl flex items-center justify-center"
                style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
                <Hash size={24} style={{ color: "var(--text-3)" }} />
              </div>
              <p className="text-sm" style={{ color: "var(--text-3)" }}>Henüz batch yok</p>
            </div>
          ) : (
            <div className="px-5 space-y-2">
              {batches.map(b => (
                <div
                  key={b.id}
                  className="rounded-2xl px-4 py-3"
                  style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
                >
                  <div className="flex items-center justify-between mb-1">
                    <div className="flex items-center gap-2">
                      <CheckCircle2 size={14} style={{ color: "var(--accent)", flexShrink: 0 }} />
                      <span className="text-xs font-mono" style={{ color: "var(--text-2)" }}>
                        seq: {b.sequencer_id}
                      </span>
                    </div>
                    <div className="flex items-center gap-1 text-xs" style={{ color: "var(--text-3)" }}>
                      <Clock size={11} />
                      <span>{timeSince(b.submitted_at)}</span>
                    </div>
                  </div>
                  <p className="text-xs font-mono truncate" style={{ color: "var(--text-1)" }}>
                    {b.state_root}
                  </p>
                  <p className="text-[10px] mt-0.5" style={{ color: "var(--text-3)" }}>
                    {b.tx_count} işlem
                  </p>
                </div>
              ))}
            </div>
          )
        )}
      </div>
    </AppShell>
  );
}
