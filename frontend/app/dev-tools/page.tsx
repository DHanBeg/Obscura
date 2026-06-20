"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import {
  Terminal, Zap, Radio, RefreshCw, Loader2,
  ChevronDown, ChevronUp, Trash2, Circle, CheckCircle2, XCircle,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

/* ── Types ──────────────────────────────────────────────────── */

interface NodeLog {
  id: string;
  timestamp: string;
  level: "info" | "warn" | "error";
  node_id: string;
  message: string;
}

interface ZKMetric {
  circuit: string;
  prove_ms: number;
  verify_ms: number;
  proof_size_bytes: number;
  status: "ok" | "fail";
  last_run: string;
}

interface WsMessage {
  id: string;
  direction: "in" | "out";
  type: string;
  payload_size: number;
  timestamp: string;
  raw: string;
}

type Section = "node-logs" | "zk-metrics" | "ws-debug";

/* ── Helpers ────────────────────────────────────────────────── */

function nowIso() {
  return new Date().toISOString();
}

function fmtMs(ms: number) {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms}ms`;
}

function fmtBytes(b: number) {
  if (b < 1024) return `${b}B`;
  return `${(b / 1024).toFixed(1)}KB`;
}

function timePart(iso: string) {
  return iso.split("T")[1]?.slice(0, 12) ?? iso;
}

const LOG_LEVEL_STYLE: Record<NodeLog["level"], { bg: string; text: string; label: string }> = {
  info:  { bg: "rgba(0,229,160,0.08)",  text: "var(--accent)",  label: "INFO" },
  warn:  { bg: "rgba(255,170,0,0.08)",  text: "var(--warning)", label: "WARN" },
  error: { bg: "rgba(255,64,88,0.08)",  text: "var(--error)",   label: "ERR " },
};

/* ── Stub data generators ───────────────────────────────────── */

const NODE_IDS = ["node-1", "node-2", "node-3", "node-4", "node-5"];
const LOG_MESSAGES: [NodeLog["level"], string][] = [
  ["info",  "Peer bağlantısı kuruldu"],
  ["info",  "GossipSub mesajı iletildi"],
  ["info",  "DHT güncellendi (16 peer)"],
  ["warn",  "Heartbeat gecikmesi: 320ms"],
  ["info",  "BFT Prevote tamamlandı"],
  ["error", "Peer bağlantısı zaman aşımına uğradı"],
  ["info",  "Blok onaylandı: epoch #4271"],
  ["warn",  "Gossip fan-out sınırına ulaşıldı"],
  ["info",  "QUIC tüneli açıldı"],
  ["info",  "Yeni node kaydı: node-6"],
];

function generateLogs(n = 12): NodeLog[] {
  return Array.from({ length: n }, (_, i) => {
    const [level, message] = LOG_MESSAGES[i % LOG_MESSAGES.length];
    return {
      id: `log-${Date.now()}-${i}`,
      timestamp: new Date(Date.now() - (n - i) * 4200).toISOString(),
      level,
      node_id: NODE_IDS[i % NODE_IDS.length],
      message,
    };
  });
}

const ZK_CIRCUITS: ZKMetric[] = [
  { circuit: "credit_threshold",  prove_ms: 494,  verify_ms: 9,   proof_size_bytes: 812, status: "ok",   last_run: nowIso() },
  { circuit: "identity_proof",    prove_ms: 511,  verify_ms: 11,  proof_size_bytes: 796, status: "ok",   last_run: nowIso() },
  { circuit: "message_integrity", prove_ms: 638,  verify_ms: 14,  proof_size_bytes: 824, status: "ok",   last_run: nowIso() },
  { circuit: "token_balance",     prove_ms: 872,  verify_ms: 18,  proof_size_bytes: 944, status: "ok",   last_run: nowIso() },
  { circuit: "vote_proof",        prove_ms: 743,  verify_ms: 13,  proof_size_bytes: 733, status: "ok",   last_run: nowIso() },
  { circuit: "storage_proof",     prove_ms: 1120, verify_ms: 22,  proof_size_bytes: 960, status: "ok",   last_run: nowIso() },
  { circuit: "location_proof",    prove_ms: 2210, verify_ms: 31,  proof_size_bytes: 1024, status: "ok",  last_run: nowIso() },
  { circuit: "recursive_proof",   prove_ms: 3450, verify_ms: 42,  proof_size_bytes: 1280, status: "fail", last_run: nowIso() },
];

const WS_TYPES = [
  "new_message", "typing", "read_receipt", "mls_commit",
  "mls_welcome", "gossip_relay", "presence", "zk_verify",
];

function generateWsMessages(n = 10): WsMessage[] {
  return Array.from({ length: n }, (_, i) => {
    const type = WS_TYPES[i % WS_TYPES.length];
    const dir: WsMessage["direction"] = i % 3 === 0 ? "out" : "in";
    return {
      id: `ws-${Date.now()}-${i}`,
      direction: dir,
      type,
      payload_size: 64 + (i * 37) % 512,
      timestamp: new Date(Date.now() - (n - i) * 1800).toISOString(),
      raw: JSON.stringify({ type, seq: 1000 + i, ts: Date.now() }),
    };
  });
}

/* ── Section header ─────────────────────────────────────────── */

function SectionHeader({
  icon,
  title,
  subtitle,
  open,
  onToggle,
  onRefresh,
  refreshing,
}: {
  icon: React.ReactNode;
  title: string;
  subtitle?: string;
  open: boolean;
  onToggle: () => void;
  onRefresh?: () => void;
  refreshing?: boolean;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      aria-expanded={open}
      onClick={onToggle}
      onKeyDown={(e) => e.key === "Enter" && onToggle()}
      className="flex items-center gap-3 px-4 py-3.5 cursor-pointer select-none"
    >
      <span style={{ color: "var(--accent)" }}>{icon}</span>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>{title}</p>
        {subtitle && (
          <p className="text-[11px]" style={{ color: "var(--text-3)" }}>{subtitle}</p>
        )}
      </div>
      {onRefresh && (
        <button
          onClick={(e) => { e.stopPropagation(); onRefresh(); }}
          aria-label="Yenile"
          disabled={refreshing}
          className="btn-icon"
        >
          <RefreshCw size={13} className={cn(refreshing && "animate-spin")} />
        </button>
      )}
      {open ? (
        <ChevronUp size={15} style={{ color: "var(--text-3)" }} />
      ) : (
        <ChevronDown size={15} style={{ color: "var(--text-3)" }} />
      )}
    </div>
  );
}

/* ── Node Bağlantı Logları ──────────────────────────────────── */

function NodeLogs() {
  const [open, setOpen] = useState(true);
  const [logs, setLogs] = useState<NodeLog[]>(() => generateLogs());
  const [refreshing, setRefreshing] = useState(false);
  const [filter, setFilter] = useState<NodeLog["level"] | "all">("all");
  const bottomRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(() => {
    setRefreshing(true);
    setTimeout(() => {
      setLogs(generateLogs());
      setRefreshing(false);
    }, 600);
  }, []);

  useEffect(() => {
    if (open) bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs, open]);

  const visible = filter === "all" ? logs : logs.filter((l) => l.level === filter);

  return (
    <div
      className="rounded-2xl overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
    >
      <SectionHeader
        icon={<Terminal size={16} />}
        title="Node Bağlantı Logları"
        subtitle={`${logs.filter((l) => l.level === "error").length} hata · ${logs.filter((l) => l.level === "warn").length} uyarı`}
        open={open}
        onToggle={() => setOpen((v) => !v)}
        onRefresh={refresh}
        refreshing={refreshing}
      />

      {open && (
        <div className="border-t animate-slide-down" style={{ borderColor: "var(--border-1)" }}>
          {/* Filter bar */}
          <div className="flex items-center gap-1.5 px-4 py-2.5 border-b" style={{ borderColor: "var(--border-1)" }}>
            {(["all", "info", "warn", "error"] as const).map((lvl) => (
              <button
                key={lvl}
                onClick={() => setFilter(lvl)}
                className={cn(
                  "h-6 px-2.5 rounded-full text-[11px] font-semibold border transition-all duration-150",
                )}
                style={
                  filter === lvl
                    ? { background: "var(--accent-muted)", borderColor: "rgba(0,229,160,0.25)", color: "var(--accent)" }
                    : { background: "transparent", borderColor: "var(--border-1)", color: "var(--text-3)" }
                }
              >
                {lvl === "all" ? "Tümü" : lvl.toUpperCase()}
              </button>
            ))}
            <div className="flex-1" />
            <button
              onClick={() => setLogs([])}
              aria-label="Logları temizle"
              className="flex items-center gap-1 text-[11px] px-2 h-6 rounded-lg transition-colors"
              style={{ color: "var(--text-3)" }}
            >
              <Trash2 size={11} /> Temizle
            </button>
          </div>

          {/* Log lines */}
          <div
            className="overflow-y-auto font-mono text-[11px] leading-relaxed"
            style={{ maxHeight: 280, background: "var(--void)" }}
          >
            {visible.length === 0 ? (
              <p className="text-center py-6 text-[11px]" style={{ color: "var(--text-3)" }}>Log yok</p>
            ) : (
              visible.map((log) => {
                const s = LOG_LEVEL_STYLE[log.level];
                return (
                  <div
                    key={log.id}
                    className="flex items-start gap-2 px-3 py-1 border-b"
                    style={{ borderColor: "rgba(255,255,255,0.03)" }}
                  >
                    <span style={{ color: "var(--text-3)", minWidth: 80 }}>{timePart(log.timestamp)}</span>
                    <span
                      className="px-1.5 rounded text-[10px] font-bold flex-shrink-0"
                      style={{ background: s.bg, color: s.text, minWidth: 36, textAlign: "center" }}
                    >
                      {s.label}
                    </span>
                    <span className="flex-shrink-0" style={{ color: "var(--text-3)", minWidth: 56 }}>
                      {log.node_id}
                    </span>
                    <span style={{ color: "var(--text-2)" }}>{log.message}</span>
                  </div>
                );
              })
            )}
            <div ref={bottomRef} />
          </div>
        </div>
      )}
    </div>
  );
}

/* ── ZK Proof Metrikleri ────────────────────────────────────── */

function ZKMetrics() {
  const [open, setOpen] = useState(true);
  const [metrics, setMetrics] = useState<ZKMetric[]>(ZK_CIRCUITS);
  const [refreshing, setRefreshing] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);

  const refresh = useCallback(() => {
    setRefreshing(true);
    setTimeout(() => {
      setMetrics(ZK_CIRCUITS.map((m) => ({
        ...m,
        prove_ms: m.prove_ms + Math.floor(Math.random() * 60 - 30),
        verify_ms: m.verify_ms + Math.floor(Math.random() * 4 - 2),
        last_run: nowIso(),
      })));
      setRefreshing(false);
    }, 700);
  }, []);

  const okCount   = metrics.filter((m) => m.status === "ok").length;
  const failCount = metrics.filter((m) => m.status === "fail").length;

  return (
    <div
      className="rounded-2xl overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
    >
      <SectionHeader
        icon={<Zap size={16} />}
        title="ZK Proof Metrikleri"
        subtitle={`${okCount} devre aktif · ${failCount > 0 ? `${failCount} hata` : "hatasız"}`}
        open={open}
        onToggle={() => setOpen((v) => !v)}
        onRefresh={refresh}
        refreshing={refreshing}
      />

      {open && (
        <div className="border-t animate-slide-down" style={{ borderColor: "var(--border-1)" }}>
          {/* Summary bar */}
          <div
            className="grid grid-cols-3 divide-x px-0"
            style={{ borderBottom: "1px solid var(--border-1)" }}
          >
            {[
              { label: "Toplam Devre",  value: metrics.length.toString() },
              { label: "Ort. Prove",    value: fmtMs(Math.round(metrics.reduce((s, m) => s + m.prove_ms, 0) / metrics.length)) },
              { label: "Ort. Verify",   value: fmtMs(Math.round(metrics.reduce((s, m) => s + m.verify_ms, 0) / metrics.length)) },
            ].map(({ label, value }) => (
              <div key={label} className="flex flex-col items-center py-3 gap-0.5">
                <span className="text-sm font-bold" style={{ color: "var(--text-1)" }}>{value}</span>
                <span className="text-[10px]" style={{ color: "var(--text-3)" }}>{label}</span>
              </div>
            ))}
          </div>

          {/* Circuit rows */}
          <div className="divide-y" style={{ borderColor: "var(--border-1)" }}>
            {metrics.map((m) => {
              const isExp = expanded === m.circuit;
              return (
                <div key={m.circuit}>
                  <div
                    role="button"
                    tabIndex={0}
                    onClick={() => setExpanded(isExp ? null : m.circuit)}
                    onKeyDown={(e) => e.key === "Enter" && setExpanded(isExp ? null : m.circuit)}
                    className="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-[rgba(255,255,255,0.02)] transition-colors"
                  >
                    {m.status === "ok" ? (
                      <CheckCircle2 size={13} style={{ color: "var(--accent)", flexShrink: 0 }} />
                    ) : (
                      <XCircle size={13} style={{ color: "var(--error)", flexShrink: 0 }} />
                    )}
                    <span
                      className="flex-1 text-xs font-mono truncate"
                      style={{ color: "var(--text-1)" }}
                    >
                      {m.circuit}
                    </span>
                    <span className="text-[11px] tabular-nums" style={{ color: "var(--text-2)" }}>
                      {fmtMs(m.prove_ms)}
                    </span>
                    <span
                      className="text-[10px] w-4 text-right"
                      style={{ color: "var(--text-3)" }}
                    >
                      {isExp ? "▲" : "▼"}
                    </span>
                  </div>

                  {isExp && (
                    <div
                      className="px-4 pb-3 grid grid-cols-2 gap-2 animate-slide-down"
                      style={{ background: "rgba(0,0,0,0.15)" }}
                    >
                      {[
                        { label: "Prove süresi",   value: fmtMs(m.prove_ms) },
                        { label: "Verify süresi",  value: fmtMs(m.verify_ms) },
                        { label: "Proof boyutu",   value: fmtBytes(m.proof_size_bytes) },
                        { label: "Son çalışma",    value: timePart(m.last_run) },
                      ].map(({ label, value }) => (
                        <div key={label}>
                          <p className="section-label mb-0.5">{label}</p>
                          <p className="text-xs font-mono" style={{ color: "var(--text-1)" }}>{value}</p>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

/* ── WebSocket Debug ────────────────────────────────────────── */

function WebSocketDebug() {
  const [open, setOpen] = useState(true);
  const [messages, setMessages] = useState<WsMessage[]>(() => generateWsMessages());
  const [connected, setConnected] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(() => {
    setRefreshing(true);
    setTimeout(() => {
      setMessages(generateWsMessages());
      setRefreshing(false);
    }, 500);
  }, []);

  useEffect(() => {
    if (open) bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, open]);

  const inCount  = messages.filter((m) => m.direction === "in").length;
  const outCount = messages.filter((m) => m.direction === "out").length;
  const totalBytes = messages.reduce((s, m) => s + m.payload_size, 0);

  return (
    <div
      className="rounded-2xl overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
    >
      <SectionHeader
        icon={<Radio size={16} />}
        title="WebSocket Debug"
        subtitle={`${inCount} gelen · ${outCount} giden · ${fmtBytes(totalBytes)} toplam`}
        open={open}
        onToggle={() => setOpen((v) => !v)}
        onRefresh={refresh}
        refreshing={refreshing}
      />

      {open && (
        <div className="border-t animate-slide-down" style={{ borderColor: "var(--border-1)" }}>
          {/* Status bar */}
          <div
            className="flex items-center gap-2 px-4 py-2.5 border-b"
            style={{ borderColor: "var(--border-1)" }}
          >
            <Circle
              size={8}
              className={cn("fill-current", connected ? "text-[var(--accent)]" : "text-[var(--error)]")}
            />
            <span className="text-xs" style={{ color: connected ? "var(--accent)" : "var(--error)" }}>
              {connected ? "Bağlı — /v1/stream" : "Bağlantı kesildi"}
            </span>
            <div className="flex-1" />
            <button
              onClick={() => setConnected((v) => !v)}
              className="text-[11px] px-2.5 h-6 rounded-lg border transition-all"
              style={
                connected
                  ? { borderColor: "rgba(255,64,88,0.3)", color: "var(--error)", background: "rgba(255,64,88,0.06)" }
                  : { borderColor: "rgba(0,229,160,0.3)", color: "var(--accent)", background: "var(--accent-muted)" }
              }
            >
              {connected ? "Kes" : "Bağlan"}
            </button>
            <button
              onClick={() => setMessages([])}
              aria-label="Temizle"
              className="text-[11px] px-2 h-6 rounded-lg flex items-center gap-1 transition-colors"
              style={{ color: "var(--text-3)" }}
            >
              <Trash2 size={11} /> Temizle
            </button>
          </div>

          {/* Message stream */}
          <div
            className="overflow-y-auto font-mono text-[11px]"
            style={{ maxHeight: 280, background: "var(--void)" }}
          >
            {messages.length === 0 ? (
              <p className="text-center py-6" style={{ color: "var(--text-3)" }}>Mesaj yok</p>
            ) : (
              messages.map((msg) => {
                const isSel = selectedId === msg.id;
                return (
                  <div key={msg.id}>
                    <div
                      role="button"
                      tabIndex={0}
                      onClick={() => setSelectedId(isSel ? null : msg.id)}
                      onKeyDown={(e) => e.key === "Enter" && setSelectedId(isSel ? null : msg.id)}
                      className={cn(
                        "flex items-center gap-2 px-3 py-1 border-b cursor-pointer",
                        isSel ? "bg-[rgba(0,229,160,0.05)]" : "hover:bg-[rgba(255,255,255,0.02)]",
                      )}
                      style={{ borderColor: "rgba(255,255,255,0.03)" }}
                    >
                      <span
                        className="text-[10px] font-bold px-1.5 rounded flex-shrink-0"
                        style={
                          msg.direction === "in"
                            ? { background: "rgba(0,229,160,0.1)", color: "var(--accent)" }
                            : { background: "rgba(100,149,237,0.12)", color: "#6495ED" }
                        }
                      >
                        {msg.direction === "in" ? "IN " : "OUT"}
                      </span>
                      <span style={{ color: "var(--text-3)", minWidth: 80 }}>{timePart(msg.timestamp)}</span>
                      <span className="flex-1 truncate" style={{ color: "var(--text-2)" }}>{msg.type}</span>
                      <span style={{ color: "var(--text-3)" }}>{fmtBytes(msg.payload_size)}</span>
                    </div>

                    {isSel && (
                      <div
                        className="px-3 py-2 border-b animate-slide-down"
                        style={{ background: "rgba(0,0,0,0.2)", borderColor: "rgba(255,255,255,0.03)" }}
                      >
                        <p className="text-[10px] mb-1" style={{ color: "var(--text-3)" }}>Payload (raw)</p>
                        <pre
                          className="text-[10px] whitespace-pre-wrap break-all"
                          style={{ color: "var(--accent)", opacity: 0.85 }}
                        >
                          {msg.raw}
                        </pre>
                      </div>
                    )}
                  </div>
                );
              })
            )}
            <div ref={bottomRef} />
          </div>
        </div>
      )}
    </div>
  );
}

/* ── Page ───────────────────────────────────────────────────── */

export default function DevToolsPage() {
  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* Header */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Geliştirici Araçları</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
              Node logları · ZK metrikleri · WebSocket debug
            </p>
          </div>
        </div>

        {/* Sections */}
        <div className="px-5 flex flex-col gap-4">
          <NodeLogs />
          <ZKMetrics />
          <WebSocketDebug />
        </div>

      </div>
    </AppShell>
  );
}
