"use client";

import { useState, useCallback } from "react";
import {
  RefreshCw, Wifi, Server, Clock, HardDrive, Zap,
  Battery, ChevronDown, Trash2, AlertCircle,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Gelişmiş",
    back: "Ayarlar",
    syncSection: "Senkronizasyon",
    bgSync: "Arka Plan Senkronizasyonu",
    bgSyncDesc: "Uygulama kapalıyken mesajları güncelle",
    syncInterval: "Senkronizasyon Aralığı",
    syncIntervals: ["30 sn", "1 dk", "5 dk", "Manuel"],
    offlineMode: "Çevrimdışı Mod",
    offlineModeDesc: "İnternet yokken de uygulamayı kullan",
    connSection: "Bağlantı",
    activeNode: "Aktif Node",
    nodes: ["Otomatik", "node-1", "node-2", "node-3", "node-4", "node-5"],
    p2pMode: "P2P Modu",
    p2pOptions: ["HTTP Gossip", "libp2p (Deneysel)"],
    timeout: "Zaman Aşımı",
    timeouts: ["10 sn", "30 sn", "60 sn"],
    cacheSection: "Önbellek",
    clearMessages: "Mesaj Önbelleğini Temizle",
    clearMedia: "Medya Önbelleğini Temizle",
    clearAll: "Tüm Önbelleği Temizle",
    perfSection: "Performans",
    hwAccel: "Donanım Hızlandırma",
    hwAccelDesc: "GPU ile arayüzü hızlandır",
    lowPower: "Düşük Güç Modu",
    lowPowerDesc: "Pil tasarrufu için arka plan görevlerini azalt",
    confirm: "Temizle",
    cancel: "İptal",
    cleared: "Temizlendi",
    msgCacheSize: "~8.4 MB",
    mediaCacheSize: "~142 MB",
    totalCacheSize: "~150 MB",
    clearWarning: "Bu işlem geri alınamaz.",
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

// ── Helpers ───────────────────────────────────────────────────────────────────

function useSetting<T>(key: string, initial: T): [T, (v: T) => void] {
  const storageKey = `obscura_settings_advanced_${key}`;
  const [value, setValue] = useState<T>(() => {
    if (typeof window === "undefined") return initial;
    try {
      const s = localStorage.getItem(storageKey);
      return s !== null ? JSON.parse(s) : initial;
    } catch { return initial; }
  });
  const set = useCallback((v: T) => {
    setValue(v);
    try { localStorage.setItem(storageKey, JSON.stringify(v)); } catch {}
  }, [storageKey]);
  return [value, set];
}

// ── Toggle ────────────────────────────────────────────────────────────────────

function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <div
      onClick={() => onChange(!on)}
      role="switch"
      aria-checked={on}
      tabIndex={0}
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onChange(!on)}
      className="relative w-11 h-6 rounded-full cursor-pointer flex-shrink-0 transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      style={{ background: on ? "var(--accent)" : "var(--surface-3)", border: on ? "none" : "1px solid var(--border-2)" }}
    >
      <div
        className="absolute top-0.5 w-5 h-5 rounded-full transition-all duration-200"
        style={{ background: "white", boxShadow: "0 1px 4px rgba(0,0,0,0.4)", left: on ? "22px" : "2px" }}
      />
    </div>
  );
}

function SettingCard({ children }: { children: React.ReactNode }) {
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

function ToggleRow({
  icon, iconBg, iconColor, label, desc, value, onChange, last,
}: {
  icon: React.ReactNode; iconBg: string; iconColor: string;
  label: string; desc?: string; value: boolean; onChange: (v: boolean) => void; last?: boolean;
}) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <button
        onClick={() => onChange(!value)}
        className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none"
        role="switch"
        aria-checked={value}
      >
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{label}</p>
          {desc && <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>{desc}</p>}
        </div>
        <Toggle on={value} onChange={onChange} />
      </button>
    </div>
  );
}

function SelectRow({
  icon, iconBg, iconColor, label, value, options, onChange, last,
}: {
  icon: React.ReactNode; iconBg: string; iconColor: string;
  label: string; value: string; options: string[]; onChange: (v: string) => void; last?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none"
        aria-expanded={open}
      >
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1">
          <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{label}</p>
        </div>
        <span className="text-xs font-semibold mr-1" style={{ color: "var(--text-2)" }}>{value}</span>
        <ChevronDown
          size={14}
          style={{ color: "var(--text-3)", transform: open ? "rotate(180deg)" : "rotate(0deg)", transition: "transform 150ms" }}
        />
      </button>
      {open && (
        <div className="pb-3 px-4 flex flex-wrap gap-2 animate-slide-down" style={{ borderTop: "1px solid var(--border-1)" }}>
          <div className="pt-2 flex flex-wrap gap-2 w-full">
            {options.map((opt) => (
              <button
                key={opt}
                onClick={() => { onChange(opt); setOpen(false); }}
                className={cn(
                  "h-8 px-3.5 rounded-xl text-xs font-semibold transition-all duration-150",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                )}
                style={{
                  background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
                  color: value === opt ? "var(--accent)" : "var(--text-2)",
                  border: value === opt ? "1px solid rgba(0,229,160,0.2)" : "1px solid transparent",
                }}
                aria-pressed={value === opt}
              >
                {opt}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ── Cache Row ─────────────────────────────────────────────────────────────────

function CacheRow({
  icon, iconBg, iconColor, label, size, onClear, last,
}: {
  icon: React.ReactNode; iconBg: string; iconColor: string;
  label: string; size: string; onClear: () => void; last?: boolean;
}) {
  const [confirm, setConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [cleared, setCleared] = useState(false);

  const clear = async () => {
    setClearing(true);
    await new Promise((r) => setTimeout(r, 600));
    setClearing(false);
    setCleared(true);
    setConfirm(false);
    setTimeout(() => setCleared(false), 2000);
    onClear();
  };

  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <div className="flex items-center gap-3.5 px-4 py-3.5">
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1">
          <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{label}</p>
          <p className="text-xs" style={{ color: cleared ? "var(--accent)" : "var(--text-3)" }}>
            {cleared ? t.cleared : size}
          </p>
        </div>
        {!confirm ? (
          <button
            onClick={() => setConfirm(true)}
            className="text-xs font-semibold h-7 px-3 rounded-xl flex-shrink-0 transition-colors duration-100 hover:bg-red-500/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--error)]"
            style={{ color: "var(--error)", border: "1px solid rgba(255,64,88,0.2)" }}
          >
            <Trash2 size={12} className="inline mr-1" />
            {t.confirm}
          </button>
        ) : (
          <div className="flex gap-1.5 flex-shrink-0">
            <button
              onClick={() => setConfirm(false)}
              className="text-xs font-semibold h-7 px-2.5 rounded-xl transition-colors"
              style={{ color: "var(--text-3)", background: "var(--surface-3)" }}
            >
              {t.cancel}
            </button>
            <button
              onClick={clear}
              disabled={clearing}
              className="text-xs font-semibold h-7 px-2.5 rounded-xl transition-colors"
              style={{ background: "var(--error)", color: "white" }}
            >
              {clearing ? <RefreshCw size={11} className="animate-spin" /> : t.confirm}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AdvancedPage() {
  const [bgSync, setBgSync] = useSetting("bgSync", true);
  const [syncInterval, setSyncInterval] = useSetting("syncInterval", t.syncIntervals[1]);
  const [offlineMode, setOfflineMode] = useSetting("offlineMode", false);
  const [activeNode, setActiveNode] = useSetting("activeNode", t.nodes[0]);
  const [p2pMode, setP2pMode] = useSetting("p2pMode", t.p2pOptions[0]);
  const [timeout, setTimeout_] = useSetting("timeout", t.timeouts[1]);
  const [hwAccel, setHwAccel] = useSetting("hwAccel", true);
  const [lowPower, setLowPower] = useSetting("lowPower", false);

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        {/* ── Sync ───────────────────────────────────────────────────────── */}
        <SectionHeader>{t.syncSection}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<RefreshCw size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.bgSync} desc={t.bgSyncDesc} value={bgSync} onChange={setBgSync} />
          <SelectRow icon={<Clock size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.syncInterval} value={syncInterval} options={t.syncIntervals} onChange={setSyncInterval} />
          <ToggleRow icon={<Wifi size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.offlineMode} desc={t.offlineModeDesc} value={offlineMode} onChange={setOfflineMode} last />
        </SettingCard>

        {/* ── Connection ─────────────────────────────────────────────────── */}
        <SectionHeader>{t.connSection}</SectionHeader>
        <SettingCard>
          <SelectRow icon={<Server size={15}/>} iconBg="rgba(77,168,255,0.08)" iconColor="var(--signal)" label={t.activeNode} value={activeNode} options={t.nodes} onChange={setActiveNode} />
          <SelectRow icon={<Wifi size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.p2pMode} value={p2pMode} options={t.p2pOptions} onChange={setP2pMode} />
          <SelectRow icon={<Clock size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.timeout} value={timeout} options={t.timeouts} onChange={setTimeout_} last />
        </SettingCard>

        {/* ── Cache ──────────────────────────────────────────────────────── */}
        <SectionHeader>{t.cacheSection}</SectionHeader>
        <SettingCard>
          <CacheRow icon={<HardDrive size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.clearMessages} size={t.msgCacheSize} onClear={() => {}} />
          <CacheRow icon={<HardDrive size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.clearMedia} size={t.mediaCacheSize} onClear={() => {}} />
          <CacheRow icon={<Trash2 size={15}/>} iconBg="rgba(255,64,88,0.06)" iconColor="rgba(255,64,88,0.7)" label={t.clearAll} size={t.totalCacheSize} onClear={() => {}} last />
        </SettingCard>

        {/* ── Performance ────────────────────────────────────────────────── */}
        <SectionHeader>{t.perfSection}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Zap size={15}/>} iconBg="rgba(255,170,0,0.08)" iconColor="#ffaa00" label={t.hwAccel} desc={t.hwAccelDesc} value={hwAccel} onChange={setHwAccel} />
          <ToggleRow icon={<Battery size={15}/>} iconBg="rgba(0,229,160,0.06)" iconColor="var(--accent)" label={t.lowPower} desc={t.lowPowerDesc} value={lowPower} onChange={setLowPower} last />
        </SettingCard>

        {/* Notice */}
        <div
          className="mx-3 mb-4 px-4 py-3 rounded-2xl flex items-start gap-2.5"
          style={{ background: "rgba(255,170,0,0.05)", border: "1px solid rgba(255,170,0,0.12)" }}
        >
          <AlertCircle size={14} className="flex-shrink-0 mt-0.5" style={{ color: "#ffaa00" }} />
          <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
            Bağlantı ve senkronizasyon ayarları değiştirildikten sonra uygulama yeniden başlatılmalıdır.
          </p>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
