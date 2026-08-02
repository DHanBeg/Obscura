"use client";

import { useState, useEffect, useCallback } from "react";
import { Globe, Server, RefreshCw, Loader2, Wifi, WifiOff, Activity, ChevronDown, ChevronUp, Plus, Radio, Timer, Info } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { formatTime } from "@/lib/format";

interface FederationNode {
  node_id: string;
  peer_addr: string;
  http_url: string;
  version: string;
  region: string;
  registered_at: string;
  last_seen: string;
  status: string;
  latency_ms: number;
}

type Filter = "active" | "all";

function timeSince(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}dk`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}sa`;
  return `${Math.floor(secs / 86400)}g`;
}

// Node ilk kayıttan beri geçen süre — gerçek uptime (yüzde değil, süre).
function uptimeDuration(registeredAt: string): string {
  const diff = Date.now() - new Date(registeredAt).getTime();
  const secs = Math.floor(diff / 1000);
  const days = Math.floor(secs / 86400);
  const hours = Math.floor((secs % 86400) / 3600);
  const mins = Math.floor((secs % 3600) / 60);
  if (days > 0) return `${days}g ${hours}s`;
  if (hours > 0) return `${hours}s ${mins}dk`;
  if (mins > 0) return `${mins}dk`;
  return `${secs}sn`;
}

const STATUS_DOT: Record<string, string> = {
  active:   "bg-[var(--accent)]",
  inactive: "bg-[var(--text-3)]",
  banned:   "bg-[var(--error)]",
};

const FILTER_LABELS: Record<Filter, string> = {
  active: "Aktif",
  all:    "Tümü",
};

export default function NodesPage() {
  const [nodes, setNodes]       = useState<FederationNode[]>([]);
  const [loading, setLoading]   = useState(true);
  const [error, setError]       = useState("");
  const [filter, setFilter]     = useState<Filter>("active");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [showRegister, setShowRegister] = useState(false);
  const [regForm, setRegForm]   = useState({ node_id: "", peer_addr: "", pubkey: "", http_url: "", region: "", version: "" });
  const [registering, setRegistering] = useState(false);
  const [regError, setRegError] = useState("");
  const [regSuccess, setRegSuccess] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await api.nodeList();
      setNodes(res?.nodes || []);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const visible = filter === "all"
    ? nodes
    : nodes.filter(n => n.status === "active");

  const activeCount  = nodes.filter(n => n.status === "active").length;
  const totalPeers   = nodes.length;
  const avgLatency   = nodes.length > 0
    ? Math.round(nodes.reduce((s, n) => s + n.latency_ms, 0) / nodes.length)
    : 0;
  const healthPct    = nodes.length > 0 ? Math.round((activeCount / nodes.length) * 100) : 0;

  const handleRegister = async () => {
    if (!regForm.node_id || !regForm.peer_addr) { setRegError("Node ID ve Peer Adresi zorunlu"); return; }
    setRegistering(true);
    setRegError("");
    try {
      await api.nodeRegister(regForm);
      setRegSuccess(true);
      setRegForm({ node_id: "", peer_addr: "", pubkey: "", http_url: "", region: "", version: "" });
      await load();
    } catch (e: any) {
      setRegError(e.message);
    } finally {
      setRegistering(false);
    }
  };

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area pb-24">

        {/* ── Header ── */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">Node Explorer</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>Permissionless federe ağ</p>
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

        {/* ── Stat bar ── */}
        <div className="grid grid-cols-4 gap-2 px-5 mb-4">
          {[
            { label: "Aktif Node",      value: activeCount.toString(),     icon: Wifi },
            { label: "Toplam Peer",     value: totalPeers.toString(),      icon: Radio },
            { label: "Ağ Sağlığı",      value: `%${healthPct}`,            icon: Activity },
            { label: "Ort. Gecikme",    value: avgLatency > 0 ? `${avgLatency}ms` : "—", icon: Timer },
          ].map(({ label, value, icon: Icon }) => (
            <div key={label} className="stat-box">
              <Icon size={13} style={{ color: "var(--text-3)" }} />
              <span className="stat-value text-lg">{value}</span>
              <span className="stat-label" style={{ fontSize: "9px" }}>{label}</span>
            </div>
          ))}
        </div>

        {/* ── Filters ── */}
        <div className="flex gap-2 px-5 mb-4">
          {(["active", "all"] as Filter[]).map(f => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={cn(
                "h-8 px-3 rounded-full text-xs font-semibold border transition-all duration-150",
                filter === f
                  ? "text-[var(--accent)]"
                  : "text-[var(--text-2)]"
              )}
              style={filter === f
                ? { background: "var(--accent-muted)", borderColor: "rgba(0,229,160,0.25)" }
                : { background: "var(--surface-2)", borderColor: "var(--border-1)" }}
            >
              {FILTER_LABELS[f]}
            </button>
          ))}
        </div>

        {/* ── Error ── */}
        {error && (
          <div className="mx-5 mb-4 rounded-2xl px-4 py-3 text-sm animate-slide-down"
            style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}>
            {error}
            <button onClick={load} className="ml-2 underline text-xs">Tekrar dene</button>
          </div>
        )}

        {/* ── Node list ── */}
        {loading ? (
          <div className="px-5 space-y-3">
            {[0,1,2,3].map(i => (
              <div key={i} className="rounded-2xl h-20 shimmer" style={{ background: "var(--surface-2)" }} />
            ))}
          </div>
        ) : visible.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-4 px-8 text-center py-12">
            <div className="w-14 h-14 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
              <Server size={24} style={{ color: "var(--text-3)" }} />
            </div>
            <div>
              <p className="font-semibold text-sm mb-1" style={{ color: "var(--text-1)" }}>Node bulunamadı</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>Ağa ilk node'u eklemek için aşağıdan kayıt ol.</p>
            </div>
          </div>
        ) : (
          <div className="px-5 space-y-2">
            {visible.map(node => {
              const isExpanded = expanded === node.node_id;
              const latency = node.latency_ms;
              const uptime  = uptimeDuration(node.registered_at);

              return (
                <div
                  key={node.node_id}
                  className="card-interactive"
                  role="button"
                  tabIndex={0}
                  aria-expanded={isExpanded}
                  onClick={() => setExpanded(isExpanded ? null : node.node_id)}
                  onKeyDown={e => e.key === "Enter" && setExpanded(isExpanded ? null : node.node_id)}
                >
                  {/* ── Collapsed row ── */}
                  <div className="flex items-center gap-3 px-4 py-3.5">
                    {/* Status dot */}
                    <div className={cn("w-2 h-2 rounded-full flex-shrink-0", STATUS_DOT[node.status] ?? STATUS_DOT.inactive)} />

                    {/* Node ID */}
                    <div className="flex-1 min-w-0">
                      <span className="text-xs font-mono" style={{ color: "var(--text-1)" }}>
                        {node.node_id.slice(0, 18)}&hellip;
                      </span>
                      {node.version && (
                        <span className="text-[10px] ml-1.5" style={{ color: "var(--text-3)" }}>v{node.version}</span>
                      )}
                    </div>

                    {/* Region */}
                    <div className="hidden sm:flex items-center gap-1 text-xs" style={{ color: "var(--text-2)" }}>
                      <Globe size={11} />
                      <span>{node.region || "—"}</span>
                    </div>

                    {/* Latency badge — henüz probe edilmemişse (0) nötr göster */}
                    <span
                      className="text-[10px] font-semibold px-2 py-0.5 rounded-full flex-shrink-0"
                      style={latency <= 0
                        ? { background: "var(--surface-2)", color: "var(--text-3)", border: "1px solid var(--border-1)" }
                        : latency < 50
                        ? { background: "rgba(0,229,160,0.1)", color: "var(--accent)", border: "1px solid rgba(0,229,160,0.2)" }
                        : latency < 100
                        ? { background: "rgba(255,170,0,0.1)", color: "var(--warning)", border: "1px solid rgba(255,170,0,0.2)" }
                        : { background: "rgba(255,64,88,0.1)", color: "var(--error)", border: "1px solid rgba(255,64,88,0.2)" }}
                    >
                      {latency > 0 ? `${latency}ms` : "—"}
                    </span>

                    {/* Uptime — kayıttan beri geçen süre */}
                    <span className="text-[10px] hidden sm:block" style={{ color: "var(--text-3)" }}>
                      {uptime}
                    </span>

                    <ChevronDown
                      size={14}
                      className={cn("flex-shrink-0 transition-transform duration-150", isExpanded && "rotate-180")}
                      style={{ color: "var(--text-3)" }}
                    />
                  </div>

                  {/* ── Expanded detail ── */}
                  {isExpanded && (
                    <div
                      className="px-4 pb-4 space-y-2 border-t animate-slide-down"
                      style={{ borderColor: "var(--border-1)" }}
                    >
                      <div className="grid grid-cols-2 gap-2 pt-3">
                        <div>
                          <p className="section-label mb-1">Region</p>
                          <div className="flex items-center gap-1 text-xs" style={{ color: "var(--text-1)" }}>
                            <Globe size={11} />
                            {node.region || "Bilinmiyor"}
                          </div>
                        </div>
                        <div>
                          <p className="section-label mb-1">Son Heartbeat</p>
                          <p className="text-xs" style={{ color: "var(--text-1)" }}>
                            {timeSince(node.last_seen)} önce
                          </p>
                        </div>
                      </div>
                      {node.peer_addr && (
                        <div>
                          <p className="section-label mb-1">libp2p Multiaddr</p>
                          <p className="text-xs font-mono truncate" style={{ color: "var(--text-2)" }}>
                            {node.peer_addr}
                          </p>
                        </div>
                      )}
                      {node.http_url && (
                        <div>
                          <p className="section-label mb-1">HTTP URL</p>
                          <p className="text-xs font-mono truncate" style={{ color: "var(--text-2)" }}>
                            {node.http_url}
                          </p>
                        </div>
                      )}
                      <div>
                        <p className="section-label mb-1">Kayıt Tarihi</p>
                        <p className="text-xs" style={{ color: "var(--text-2)" }}>{formatTime(node.registered_at)}</p>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* ── Register new node ── */}
        <div className="px-5 mt-4">
          <button
            onClick={() => { setShowRegister(v => !v); setRegSuccess(false); setRegError(""); }}
            className={cn(
              "w-full h-11 rounded-2xl flex items-center justify-center gap-2 text-sm font-semibold transition-all duration-150 border",
              showRegister
                ? "text-[var(--text-2)]"
                : "text-[var(--text-1)]"
            )}
            style={showRegister
              ? { background: "var(--surface-2)", borderColor: "var(--border-1)" }
              : { background: "var(--surface-2)", borderColor: "var(--border-2)" }}
          >
            {showRegister ? <ChevronUp size={15} /> : <Plus size={15} />}
            {showRegister ? "Gizle" : "Yeni Node Kaydet"}
          </button>

          {showRegister && (
            <div
              className="mt-3 rounded-2xl p-4 space-y-3 animate-slide-down"
              style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}
            >
              <p className="section-label">Node Kaydı</p>

              <div className="rounded-xl px-3 py-2.5 text-xs flex items-start gap-2"
                style={{ background: "var(--surface-3)", color: "var(--text-3)", border: "1px solid var(--border-1)" }}>
                <Info size={12} className="mt-0.5 shrink-0" />
                <span>
                  Node&apos;lar artık başlangıçta kendi P2P anahtarlarıyla imzalı şekilde
                  otomatik kaydoluyor. Bu form yalnızca imzasız/manuel (legacy) kayıt
                  içindir — doğrulanmış node&apos;lardan güven derecesi daha düşüktür ve
                  bu yol ileride kapatılabilir.
                </span>
              </div>

              {regSuccess && (
                <div className="rounded-xl px-3 py-2.5 text-xs flex items-center gap-2"
                  style={{ background: "var(--accent-muted)", color: "var(--accent)", border: "1px solid rgba(0,229,160,0.2)" }}>
                  <Wifi size={12} /> Node başarıyla kaydedildi!
                </div>
              )}

              {regError && (
                <p className="text-xs" style={{ color: "var(--error)" }}>{regError}</p>
              )}

              {[
                { key: "node_id",   placeholder: "node-xyz",               label: "Node ID *" },
                { key: "peer_addr", placeholder: "/ip4/1.2.3.4/tcp/9001",  label: "Peer Adresi *" },
                { key: "pubkey",    placeholder: "0x04...",                 label: "Public Key *" },
                { key: "http_url",  placeholder: "https://node.example.com", label: "HTTP URL" },
                { key: "region",    placeholder: "EU-West",                label: "Region" },
                { key: "version",   placeholder: "1.0.0",                  label: "Versiyon" },
              ].map(({ key, placeholder, label }) => (
                <div key={key}>
                  <label className="block text-[11px] mb-1" style={{ color: "var(--text-3)" }}>{label}</label>
                  <input
                    value={(regForm as any)[key]}
                    onChange={e => setRegForm(f => ({ ...f, [key]: e.target.value }))}
                    placeholder={placeholder}
                    className="field text-xs h-10"
                    aria-label={label}
                  />
                </div>
              ))}

              <button
                onClick={handleRegister}
                disabled={registering || !regForm.node_id || !regForm.peer_addr || !regForm.pubkey}
                className="btn-primary w-full"
              >
                {registering ? <Loader2 size={14} className="animate-spin" /> : <Server size={14} />}
                {registering ? "Kaydediliyor..." : "Node Kaydet"}
              </button>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
