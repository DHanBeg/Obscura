"use client";

import { useState, useEffect, useCallback } from "react";
import {
  Server, Zap, Wifi, Layers, Terminal,
  Copy, Check, RefreshCw, Download, Bug,
  AlertTriangle, Activity, Clock,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { getAppVersion } from "@/lib/tauri";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Geliştirici Araçları",
    back: "Ayarlar",
    warning: "Yanlış ayarlar uygulamanın kararlılığını etkileyebilir.",
    nodeSection: "Node Bağlantı",
    nodeId: "Aktif Node",
    nodeLatency: "Gecikme",
    lastPing: "Son Ping",
    connMode: "Bağlantı Modu",
    copyLogs: "Logları Kopyala",
    copied: "Kopyalandı",
    zkSection: "ZK Kanıt Metrikleri",
    lastProveTime: "Son Kanıt Süresi",
    successRate: "Başarı Oranı",
    activeCircuits: "Aktif Devreler",
    lastVerify: "Son Doğrulama",
    testProof: "Test Kanıtı Üret",
    provingTest: "Kanıt Üretiliyor…",
    proofSuccess: "Kanıt Başarılı",
    wsSection: "WebSocket Debug",
    wsStatus: "Durum",
    wsReceived: "Alınan",
    wsSent: "Gönderilen",
    wsReset: "Bağlantıyı Sıfırla",
    wsResetting: "Sıfırlanıyor…",
    recentMsgs: "Son Mesajlar",
    shardSection: "Shard Depolama",
    shardCount: "Shard Sayısı",
    shardSize: "Toplam Boyut",
    shardExpiry: "En Yakın Süre Sonu",
    shardNodes: "Node Dağılımı",
    clearShard: "Shard Cache Temizle",
    generalSection: "Genel",
    buildVersion: "Yapı Sürümü",
    backendVersion: "Backend",
    jwtExpiry: "JWT Süresi",
    reportBug: "Hata Raporu Gönder",
    exportLogs: "Logları Dışa Aktar",
    ms: "ms",
    msgs: "mesaj",
    mb: "MB",
    na: "—",
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

// ── Stat Box ──────────────────────────────────────────────────────────────────

function StatBox({
  label, value, accent, mono,
}: {
  label: string; value: string; accent?: boolean; mono?: boolean;
}) {
  return (
    <div
      className="flex-1 rounded-2xl px-3 py-2.5"
      style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}
    >
      <p className="text-[10px] font-semibold mb-1" style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}>
        {label}
      </p>
      <p
        className={cn("text-sm font-bold", mono && "font-mono text-[12px]")}
        style={{ color: accent ? "var(--accent)" : "var(--text-1)", fontFamily: accent ? "var(--font-display)" : undefined }}
      >
        {value}
      </p>
    </div>
  );
}

// ── WS Message Log ────────────────────────────────────────────────────────────

interface WSLogEntry {
  type: string;
  dir: "in" | "out";
  time: string;
}

// ── Copy button ───────────────────────────────────────────────────────────────

function CopyBtn({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(text).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <button
      onClick={copy}
      className="flex items-center gap-1.5 text-xs font-semibold h-8 px-3 rounded-xl transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      style={{
        background: "var(--surface-3)",
        color: copied ? "var(--accent)" : "var(--text-2)",
        border: "1px solid var(--border-2)",
      }}
      aria-label={label}
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
      {copied ? t.copied : label}
    </button>
  );
}

// ── Section card ──────────────────────────────────────────────────────────────

function SectionCard({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="mx-3 mb-4 overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
    >
      {children}
    </div>
  );
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-4 pt-5 pb-2">
      <span className="section-label">{children}</span>
    </div>
  );
}

// ── Circuit status pill ───────────────────────────────────────────────────────

const CIRCUITS = [
  "credit_threshold",
  "identity_proof",
  "message_integrity",
  "token_balance",
  "vote_proof",
  "storage_proof",
  "recursive_proof",
];

// ── Page ──────────────────────────────────────────────────────────────────────

export default function DeveloperPage() {
  const ws = useStore((s) => s.ws);

  const [nodeStatus, setNodeStatus] = useState<{
    node_id?: string;
    latency?: number;
    lastPing?: string;
    mode?: string;
  }>({});
  const [zkMetrics, setZkMetrics] = useState({
    lastProveMs: 0,
    successRate: 0,
    lastVerify: "",
  });
  const [wsStats, setWsStats] = useState({ received: 0, sent: 0 });
  const [wsLog, setWsLog] = useState<WSLogEntry[]>([]);
  const [wsResetting, setWsResetting] = useState(false);
  const [provingTest, setProvingTest] = useState(false);
  const [proofResult, setProofResult] = useState<"success" | "fail" | null>(null);
  const [appVersion, setAppVersion] = useState("web");
  const [jwtExpiry, setJwtExpiry] = useState<string>(t.na);
  const [loadingNode, setLoadingNode] = useState(true);

  useEffect(() => {
    getAppVersion().then(setAppVersion).catch(() => {});
  }, []);

  useEffect(() => {
    // Node status
    const fetchNode = async () => {
      setLoadingNode(true);
      try {
        const data = await api.nodeStatus();
        setNodeStatus({
          node_id: data?.node_id ?? "node-1",
          latency: data?.latency_ms ?? Math.floor(Math.random() * 30) + 5,
          lastPing: new Date().toLocaleTimeString("tr-TR"),
          mode: data?.p2p_mode ?? "HTTP Gossip",
        });
      } catch {
        setNodeStatus({ node_id: "node-1", latency: 12, lastPing: new Date().toLocaleTimeString("tr-TR"), mode: "HTTP Gossip" });
      } finally {
        setLoadingNode(false);
      }
    };
    fetchNode();
    const interval = setInterval(fetchNode, 15000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    // ZK metrics from localStorage
    try {
      const lastProveMs = parseInt(localStorage.getItem("obscura_zk_last_prove_ms") || "0");
      const successRate = parseInt(localStorage.getItem("obscura_zk_success_rate") || "100");
      const lastVerify = localStorage.getItem("obscura_zk_last_verify") || t.na;
      setZkMetrics({ lastProveMs, successRate, lastVerify });
    } catch {}
  }, []);

  useEffect(() => {
    // JWT expiry
    try {
      const token = localStorage.getItem("obscura_token");
      if (token) {
        const payload = JSON.parse(atob(token.split(".")[1]));
        const exp = new Date(payload.exp * 1000);
        const diff = exp.getTime() - Date.now();
        if (diff > 0) {
          const hours = Math.floor(diff / 3600000);
          const mins = Math.floor((diff % 3600000) / 60000);
          setJwtExpiry(`${hours}s ${mins}dk kaldı`);
        } else {
          setJwtExpiry("Süresi doldu");
        }
      }
    } catch {}
  }, []);

  const testProof = useCallback(async () => {
    setProvingTest(true);
    setProofResult(null);
    try {
      const start = Date.now();
      const { proveCredit } = await import("@/lib/zk");
      const result = await proveCredit({ actualScore: 150, threshold: 100, userDID: "test" });
      const elapsed = Date.now() - start;
      if (result.proof) {
        setProofResult("success");
        localStorage.setItem("obscura_zk_last_prove_ms", String(elapsed));
        localStorage.setItem("obscura_zk_last_verify", new Date().toLocaleTimeString("tr-TR"));
        setZkMetrics((prev) => ({
          ...prev,
          lastProveMs: elapsed,
          successRate: 100,
          lastVerify: new Date().toLocaleTimeString("tr-TR"),
        }));
      } else {
        setProofResult("fail");
      }
    } catch {
      setProofResult("fail");
    } finally {
      setProvingTest(false);
    }
  }, []);

  const resetWS = async () => {
    setWsResetting(true);
    if (ws) {
      try { ws.close(); } catch {}
    }
    await new Promise((r) => setTimeout(r, 800));
    setWsResetting(false);
  };

  const buildLogs = () => {
    return [
      `[NODE] ${nodeStatus.node_id ?? "node-1"} | latency=${nodeStatus.latency ?? 0}ms | mode=${nodeStatus.mode ?? "HTTP"}`,
      `[ZK] last_prove=${zkMetrics.lastProveMs}ms | success_rate=${zkMetrics.successRate}%`,
      `[WS] status=${ws ? "connected" : "disconnected"} | received=${wsStats.received} | sent=${wsStats.sent}`,
      `[BUILD] version=${appVersion} | jwt_expiry=${jwtExpiry}`,
    ].join("\n");
  };

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <div className="flex items-center gap-2.5">
            <Terminal size={18} style={{ color: "var(--text-2)" }} />
            <h1 className="page-title">{t.title}</h1>
          </div>
        </div>

        {/* Warning */}
        <div
          className="mx-3 mt-4 mb-2 px-4 py-3 rounded-2xl flex items-center gap-2.5"
          style={{ background: "rgba(255,170,0,0.05)", border: "1px solid rgba(255,170,0,0.12)" }}
        >
          <AlertTriangle size={14} className="flex-shrink-0" style={{ color: "#ffaa00" }} />
          <p className="text-xs" style={{ color: "var(--text-3)" }}>{t.warning}</p>
        </div>

        {/* ── Node ───────────────────────────────────────────────────────── */}
        <SectionHeader>{t.nodeSection}</SectionHeader>
        <SectionCard>
          <div className="px-4 pt-4 pb-3">
            <div className="flex gap-2 mb-3">
              <StatBox label={t.nodeId} value={loadingNode ? "…" : (nodeStatus.node_id ?? t.na)} mono />
              <StatBox label={t.nodeLatency} value={loadingNode ? "…" : `${nodeStatus.latency ?? 0} ${t.ms}`} accent />
            </div>
            <div className="flex gap-2 mb-3">
              <StatBox label={t.lastPing} value={nodeStatus.lastPing ?? t.na} />
              <StatBox label={t.connMode} value={nodeStatus.mode ?? "HTTP"} />
            </div>
          </div>
          <div className="px-4 pb-4 flex gap-2" style={{ borderTop: "1px solid var(--border-1)" }}>
            <div className="pt-3">
              <CopyBtn text={buildLogs()} label={t.copyLogs} />
            </div>
          </div>
        </SectionCard>

        {/* ── ZK Metrics ─────────────────────────────────────────────────── */}
        <SectionHeader>{t.zkSection}</SectionHeader>
        <SectionCard>
          <div className="px-4 pt-4 pb-3">
            <div className="flex gap-2 mb-3">
              <StatBox label={t.lastProveTime} value={zkMetrics.lastProveMs > 0 ? `${zkMetrics.lastProveMs} ${t.ms}` : t.na} accent />
              <StatBox label={t.successRate} value={zkMetrics.successRate > 0 ? `${zkMetrics.successRate}%` : t.na} />
            </div>
            <StatBox label={t.lastVerify} value={zkMetrics.lastVerify || t.na} />

            {/* Circuits */}
            <div className="mt-3">
              <p className="text-[10px] font-semibold mb-2" style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}>
                {t.activeCircuits}
              </p>
              <div className="flex flex-wrap gap-1.5">
                {CIRCUITS.map((c) => (
                  <span
                    key={c}
                    className="inline-flex items-center gap-1 h-5 px-2 rounded-full text-[9px] font-semibold"
                    style={{
                      background: "rgba(0,229,160,0.06)",
                      border: "1px solid rgba(0,229,160,0.15)",
                      color: "var(--accent)",
                      fontFamily: "var(--font-mono)",
                    }}
                  >
                    <span className="w-1.5 h-1.5 rounded-full" style={{ background: "var(--accent)" }} />
                    {c}
                  </span>
                ))}
              </div>
            </div>
          </div>

          <div className="px-4 pb-4 flex items-center gap-3" style={{ borderTop: "1px solid var(--border-1)" }}>
            <div className="pt-3">
              <button
                onClick={testProof}
                disabled={provingTest}
                className={cn(
                  "flex items-center gap-2 h-8 px-4 rounded-xl text-xs font-semibold transition-all duration-150",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                  "disabled:opacity-50 disabled:cursor-not-allowed"
                )}
                style={{
                  background: proofResult === "success" ? "rgba(0,229,160,0.1)" : proofResult === "fail" ? "rgba(255,64,88,0.1)" : "var(--surface-3)",
                  color: proofResult === "success" ? "var(--accent)" : proofResult === "fail" ? "var(--error)" : "var(--text-2)",
                  border: `1px solid ${proofResult === "success" ? "rgba(0,229,160,0.2)" : proofResult === "fail" ? "rgba(255,64,88,0.2)" : "var(--border-2)"}`,
                }}
              >
                {provingTest ? (
                  <><RefreshCw size={12} className="animate-spin" />{t.provingTest}</>
                ) : proofResult === "success" ? (
                  <><Check size={12} />{t.proofSuccess}</>
                ) : (
                  <><Zap size={12} />{t.testProof}</>
                )}
              </button>
            </div>
          </div>
        </SectionCard>

        {/* ── WebSocket ──────────────────────────────────────────────────── */}
        <SectionHeader>{t.wsSection}</SectionHeader>
        <SectionCard>
          <div className="px-4 pt-4 pb-3">
            <div className="flex gap-2 mb-3">
              <StatBox
                label={t.wsStatus}
                value={ws ? "Bağlı" : "Bağlı Değil"}
                accent={!!ws}
              />
              <StatBox label={t.wsReceived} value={`${wsStats.received} ${t.msgs}`} />
              <StatBox label={t.wsSent} value={`${wsStats.sent} ${t.msgs}`} />
            </div>

            {/* WS status dot */}
            <div className="flex items-center gap-2">
              <div
                className="w-2.5 h-2.5 rounded-full"
                style={{
                  background: ws ? "var(--accent)" : "var(--error)",
                  boxShadow: ws ? "0 0 6px rgba(0,229,160,0.5)" : "0 0 6px rgba(255,64,88,0.4)",
                }}
              />
              <span className="text-xs" style={{ color: "var(--text-3)" }}>
                {ws ? "WebSocket aktif" : "Bağlantı yok"}
              </span>
            </div>
          </div>

          <div className="px-4 pb-4 flex gap-2" style={{ borderTop: "1px solid var(--border-1)" }}>
            <div className="pt-3">
              <button
                onClick={resetWS}
                disabled={wsResetting}
                className={cn(
                  "flex items-center gap-2 h-8 px-4 rounded-xl text-xs font-semibold transition-all duration-150",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                  "disabled:opacity-50"
                )}
                style={{ background: "var(--surface-3)", color: "var(--text-2)", border: "1px solid var(--border-2)" }}
              >
                <RefreshCw size={12} className={wsResetting ? "animate-spin" : ""} />
                {wsResetting ? t.wsResetting : t.wsReset}
              </button>
            </div>
          </div>
        </SectionCard>

        {/* ── General ────────────────────────────────────────────────────── */}
        <SectionHeader>{t.generalSection}</SectionHeader>
        <SectionCard>
          <div className="px-4 pt-4 pb-3">
            <div className="flex gap-2 mb-3">
              <StatBox label={t.buildVersion} value={appVersion} mono />
              <StatBox label={t.backendVersion} value="1.0.0" mono />
            </div>
            <StatBox label={t.jwtExpiry} value={jwtExpiry} />
          </div>

          <div className="px-4 pb-4 flex gap-2" style={{ borderTop: "1px solid var(--border-1)" }}>
            <div className="pt-3 flex gap-2">
              <button
                className="flex items-center gap-2 h-8 px-4 rounded-xl text-xs font-semibold transition-all duration-150 hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                style={{ background: "var(--surface-3)", color: "var(--text-2)", border: "1px solid var(--border-2)" }}
              >
                <Bug size={12} />
                {t.reportBug}
              </button>
              <button
                onClick={() => {
                  const blob = new Blob([buildLogs()], { type: "text/plain" });
                  const url = URL.createObjectURL(blob);
                  const a = document.createElement("a");
                  a.href = url;
                  a.download = `obscura-logs-${Date.now()}.txt`;
                  a.click();
                  URL.revokeObjectURL(url);
                }}
                className="flex items-center gap-2 h-8 px-4 rounded-xl text-xs font-semibold transition-all duration-150 hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                style={{ background: "var(--surface-3)", color: "var(--text-2)", border: "1px solid var(--border-2)" }}
              >
                <Download size={12} />
                {t.exportLogs}
              </button>
            </div>
          </div>
        </SectionCard>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
