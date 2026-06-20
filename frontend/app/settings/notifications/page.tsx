"use client";

import { useState, useCallback } from "react";
import {
  MessageCircle, Users, Megaphone, PhoneCall, Vote, Bell,
  Volume2, VolumeX, ChevronDown, UserPlus,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Bildirimler",
    back: "Ayarlar",
    categories: [
      { key: "dm",     icon: "dm",       label: "Özel Mesajlar",      color: "var(--accent)",  bg: "rgba(0,229,160,0.08)" },
      { key: "group",  icon: "group",    label: "Grup Mesajları",      color: "var(--signal)",  bg: "rgba(77,168,255,0.08)" },
      { key: "channel",icon: "channel",  label: "Kanallar",            color: "#ffaa00",         bg: "rgba(255,170,0,0.08)" },
      { key: "call",   icon: "call",     label: "Aramalar",            color: "var(--accent)",  bg: "rgba(0,229,160,0.08)" },
      { key: "vote",   icon: "vote",     label: "Yönetim Oylamaları",  color: "#a78bfa",        bg: "rgba(139,92,246,0.08)" },
      { key: "system", icon: "system",   label: "Sistem Bildirimleri", color: "var(--text-2)",  bg: "var(--surface-3)" },
    ],
    enabled: "Bildirimler",
    sound: "Ses",
    vibrate: "Titreşim",
    preview: "Önizleme",
    lockScreen: "Kilit Ekranı",
    sounds: ["Varsayılan", "Sessiz", "Ping", "Buzz", "Ding"],
    previews: ["Tam İçerik", "Yalnızca İsim", "Hiçbiri"],
    lockOptions: ["Göster", "Gizle"],
    advancedSection: "Gelişmiş",
    inApp: "Uygulama İçinde Ses",
    inAppDesc: "Uygulama açıkken de bildirim sesi",
    unreadCounter: "Okunmamış Sayacı",
    unreadOptions: ["Tüm Mesajlar", "Yalnızca Sesli"],
    silentBadge: "Sessiz Bildirimleri Say",
    silentBadgeDesc: "Sessiz mesajları okunmamış sayacına ekle",
    focusMode: "Odak Modu",
    focusModeDesc: "Yalnızca öncelikli kişilerden bildirim al",
    exceptionsSection: "Kişiye Özel Kurallar",
    addException: "Kişiye Özel Bildirim Ekle",
    noExceptions: "Henüz kural eklenmedi",
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

// ── Helpers ───────────────────────────────────────────────────────────────────

function useSetting<T>(key: string, initial: T): [T, (v: T) => void] {
  const storageKey = `obscura_settings_notif_${key}`;
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

// ── Category icon map ─────────────────────────────────────────────────────────

function CategoryIcon({ type, size = 15 }: { type: string; size?: number }) {
  switch (type) {
    case "dm":      return <MessageCircle size={size} />;
    case "group":   return <Users size={size} />;
    case "channel": return <Megaphone size={size} />;
    case "call":    return <PhoneCall size={size} />;
    case "vote":    return <Vote size={size} />;
    case "system":  return <Bell size={size} />;
    default:        return <Bell size={size} />;
  }
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

// ── Select Chips ──────────────────────────────────────────────────────────────

function SelectChips({
  value, options, onChange,
}: { value: string; options: string[]; onChange: (v: string) => void }) {
  return (
    <div className="flex flex-wrap gap-1.5 pt-0.5">
      {options.map((opt) => (
        <button
          key={opt}
          onClick={() => onChange(opt)}
          className={cn(
            "h-7 px-3 rounded-xl text-xs font-semibold transition-all duration-150",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          )}
          style={{
            background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
            color: value === opt ? "var(--accent)" : "var(--text-3)",
            border: value === opt ? "1px solid rgba(0,229,160,0.2)" : "1px solid transparent",
          }}
          aria-pressed={value === opt}
        >
          {opt}
        </button>
      ))}
    </div>
  );
}

// ── Category Card ─────────────────────────────────────────────────────────────

interface CatSettings {
  enabled: boolean;
  sound: string;
  vibrate: boolean;
  preview: string;
  lock: string;
}

function CategoryCard({
  catKey, icon, label, color, bg, settings, onChange,
}: {
  catKey: string;
  icon: string;
  label: string;
  color: string;
  bg: string;
  settings: CatSettings;
  onChange: (patch: Partial<CatSettings>) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div
      className="mx-3 mb-2 overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
    >
      {/* Header row */}
      <div
        className="flex items-center gap-3.5 px-4 py-3.5 cursor-pointer select-none"
        style={expanded ? { borderBottom: "1px solid var(--border-1)" } : undefined}
        onClick={() => setExpanded((v) => !v)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && setExpanded((v) => !v)}
        aria-expanded={expanded}
        aria-label={label}
      >
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
          style={{ background: bg, color }}
        >
          <CategoryIcon type={icon} />
        </div>
        <div className="flex-1">
          <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>{label}</p>
          {!settings.enabled && (
            <p className="text-xs" style={{ color: "var(--text-3)" }}>Kapalı</p>
          )}
          {settings.enabled && (
            <p className="text-xs" style={{ color: "var(--text-3)" }}>
              {settings.sound === "Sessiz" ? "Sessiz" : "Sesli"} · {settings.preview}
            </p>
          )}
        </div>
        <Toggle on={settings.enabled} onChange={(v) => { onChange({ enabled: v }); }} />
        <button
          onClick={(e) => { e.stopPropagation(); setExpanded((v) => !v); }}
          className="w-7 h-7 rounded-xl flex items-center justify-center ml-1 transition-colors hover:bg-white/[0.04]"
          aria-label="Detayları aç"
        >
          <ChevronDown
            size={14}
            style={{ color: "var(--text-3)", transform: expanded ? "rotate(180deg)" : "rotate(0deg)", transition: "transform 150ms" }}
          />
        </button>
      </div>

      {/* Expanded settings */}
      {expanded && settings.enabled && (
        <div className="px-4 pt-3 pb-4 space-y-4 animate-slide-down">
          {/* Sound */}
          <div>
            <div className="flex items-center gap-2 mb-1.5">
              <Volume2 size={12} style={{ color: "var(--text-3)" }} />
              <span className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--text-3)" }}>{t.sound}</span>
            </div>
            <SelectChips value={settings.sound} options={t.sounds} onChange={(v) => onChange({ sound: v })} />
          </div>

          {/* Preview */}
          <div>
            <div className="flex items-center gap-2 mb-1.5">
              <span className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--text-3)" }}>{t.preview}</span>
            </div>
            <SelectChips value={settings.preview} options={t.previews} onChange={(v) => onChange({ preview: v })} />
          </div>

          {/* Lock screen */}
          <div>
            <div className="flex items-center gap-2 mb-1.5">
              <span className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--text-3)" }}>{t.lockScreen}</span>
            </div>
            <SelectChips value={settings.lock} options={t.lockOptions} onChange={(v) => onChange({ lock: v })} />
          </div>

          {/* Vibrate */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {settings.vibrate
                ? <Volume2 size={13} style={{ color: "var(--text-3)" }} />
                : <VolumeX size={13} style={{ color: "var(--text-3)" }} />
              }
              <span className="text-sm" style={{ color: "var(--text-2)" }}>{t.vibrate}</span>
            </div>
            <Toggle on={settings.vibrate} onChange={(v) => onChange({ vibrate: v })} />
          </div>
        </div>
      )}
    </div>
  );
}

// ── Default category settings factory ────────────────────────────────────────

function defaultCat(): CatSettings {
  return { enabled: true, sound: "Varsayılan", vibrate: true, preview: "Tam İçerik", lock: "Göster" };
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function NotificationsPage() {
  // Per-category settings
  const [catSettings, setCatSettings] = useState<Record<string, CatSettings>>(() => {
    const saved: Record<string, CatSettings> = {};
    t.categories.forEach(({ key }) => {
      try {
        const s = localStorage.getItem(`obscura_settings_notif_cat_${key}`);
        saved[key] = s ? JSON.parse(s) : defaultCat();
      } catch {
        saved[key] = defaultCat();
      }
    });
    return saved;
  });

  const updateCat = (key: string, patch: Partial<CatSettings>) => {
    setCatSettings((prev) => {
      const next = { ...prev, [key]: { ...prev[key], ...patch } };
      try { localStorage.setItem(`obscura_settings_notif_cat_${key}`, JSON.stringify(next[key])); } catch {}
      return next;
    });
  };

  // Advanced
  const [inApp, setInApp] = useSetting("inApp", true);
  const [unreadCounter, setUnreadCounter] = useSetting("unreadCounter", t.unreadOptions[0]);
  const [silentBadge, setSilentBadge] = useSetting("silentBadge", false);
  const [focusMode, setFocusMode] = useSetting("focusMode", false);

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        <div className="h-4 flex-shrink-0" />

        {/* Category cards */}
        {t.categories.map(({ key, icon, label, color, bg }) => (
          <CategoryCard
            key={key}
            catKey={key}
            icon={icon}
            label={label}
            color={color}
            bg={bg}
            settings={catSettings[key] ?? defaultCat()}
            onChange={(patch) => updateCat(key, patch)}
          />
        ))}

        {/* Advanced */}
        <div className="px-4 pt-4 pb-2">
          <span className="section-label">{t.advancedSection}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {/* In-app sound */}
          <div className="flex items-center gap-3.5 px-4 py-3.5" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "rgba(255,170,0,0.08)", color: "#ffaa00" }}>
              <Volume2 size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.inApp}</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>{t.inAppDesc}</p>
            </div>
            <Toggle on={inApp} onChange={setInApp} />
          </div>

          {/* Unread counter */}
          <div className="px-4 pt-3 pb-3" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div className="flex items-center gap-3.5 mb-2">
              <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "var(--surface-3)", color: "var(--text-3)" }}>
                <Bell size={15} />
              </div>
              <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.unreadCounter}</p>
            </div>
            <div className="pl-12">
              <SelectChips value={unreadCounter} options={t.unreadOptions} onChange={setUnreadCounter} />
            </div>
          </div>

          {/* Silent badge */}
          <div className="flex items-center gap-3.5 px-4 py-3.5" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "var(--surface-3)", color: "var(--text-3)" }}>
              <VolumeX size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.silentBadge}</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>{t.silentBadgeDesc}</p>
            </div>
            <Toggle on={silentBadge} onChange={setSilentBadge} />
          </div>

          {/* Focus mode */}
          <div className="flex items-center gap-3.5 px-4 py-3.5">
            <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "rgba(139,92,246,0.08)", color: "#a78bfa" }}>
              <Bell size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.focusMode}</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>{t.focusModeDesc}</p>
            </div>
            <Toggle on={focusMode} onChange={setFocusMode} />
          </div>
        </div>

        {/* Exceptions */}
        <div className="px-4 pb-2">
          <span className="section-label">{t.exceptionsSection}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          <button
            className="w-full flex items-center gap-3.5 px-4 py-3.5 transition-colors duration-100 hover:bg-white/[0.025] focus-visible:outline-none focus-visible:bg-white/[0.025]"
            aria-label={t.addException}
          >
            <div
              className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(0,229,160,0.06)", color: "var(--accent)" }}
            >
              <UserPlus size={15} />
            </div>
            <span className="text-sm font-semibold" style={{ color: "var(--accent)" }}>
              {t.addException}
            </span>
          </button>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
