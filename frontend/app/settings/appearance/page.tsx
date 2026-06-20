"use client";

import { useState, useCallback, useEffect } from "react";
import {
  Sun, MessageSquare, Clock, Image, Wifi, Zap,
  Minus, Plus,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Görünüm",
    back: "Ayarlar",
    themeSection: "Tema",
    theme: "Void Cipher Dark",
    themeDesc: "Obscura yalnızca karanlık temayı destekler",
    accentSection: "Vurgu Rengi",
    accents: [
      { name: "Mint",     value: "#00e5a0" },
      { name: "Safir",    value: "#4da8ff" },
      { name: "Ametist",  value: "#a78bfa" },
      { name: "Altın",    value: "#ffaa00" },
      { name: "Kırmızı",  value: "#ff4058" },
      { name: "Beyaz",    value: "#eef0ff" },
    ],
    fontSection: "Yazı Tipi",
    fontSize: "Yazı Boyutu",
    fontSizes: ["Küçük", "Normal", "Büyük", "Çok Büyük"],
    systemFont: "Sistem Yazı Tipi",
    systemFontDesc: "Özel yazı tipi yerine sistem yazı tipini kullan",
    chatSection: "Sohbet Görünümü",
    bubbleCorners: "Balon Köşeleri",
    bubbleOptions: ["Keskin", "Hafif", "Yuvarlak"],
    msgGrouping: "Mesaj Gruplama",
    msgGroupingDesc: "Ardışık mesajları grupla",
    timestamp: "Zaman Damgası",
    timestampOptions: ["Her Mesajda", "Grupta Bir", "Hover'da"],
    linkPreview: "Bağlantı Önizleme",
    linkPreviewDesc: "URL'leri kart olarak göster",
    motionSection: "Animasyonlar",
    animations: "Animasyonları Etkinleştir",
    animationsDesc: "Genel arayüz geçişleri",
    emojiAnim: "Emoji Animasyonları",
    emojiAnimDesc: "Animasyonlu emoji ve sticker",
    sendAnim: "Gönderme Animasyonu",
    sendAnimDesc: "Mesaj gönderilince balonu canlandır",
    reducedMotion: "Azaltılmış Hareket",
    reducedMotionDesc: "Erişilebilirlik için animasyonları azalt",
    mediaSection: "Medya ve Depolama",
    autoSave: "Galeriye Otomatik Kaydet",
    autoSaveDesc: "İndirilen medyaları galeriye ekle",
    cacheSize: "Önbellek Boyutu",
    cacheSizes: ["1 GB", "2 GB", "5 GB", "Sınırsız"],
    lowData: "Düşük Veri Modu",
    lowDataDesc: "Otomatik medya indirmeyi devre dışı bırak",
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

// ── Helpers ───────────────────────────────────────────────────────────────────

function useSetting<T>(key: string, initial: T): [T, (v: T) => void] {
  const storageKey = `obscura_settings_appearance_${key}`;
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

// ── SettingCard container ─────────────────────────────────────────────────────

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

// Row
function ToggleRow({
  icon, iconBg, iconColor, label, desc, value, onChange, last,
}: {
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc?: string;
  value: boolean;
  onChange: (v: boolean) => void;
  last?: boolean;
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
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  value: string;
  options: string[];
  onChange: (v: string) => void;
  last?: boolean;
}) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined} className="px-4 pt-3.5 pb-3">
      <div className="flex items-center gap-3.5 mb-2.5">
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{label}</p>
      </div>
      <div className="flex flex-wrap gap-2 pl-12">
        {options.map((opt) => (
          <button
            key={opt}
            onClick={() => onChange(opt)}
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
  );
}

// ── Font size slider ──────────────────────────────────────────────────────────

function FontSizeRow({
  value, options, onChange,
}: {
  value: string;
  options: string[];
  onChange: (v: string) => void;
}) {
  const idx = options.indexOf(value);
  const pct = (idx / (options.length - 1)) * 100;

  const prev = () => idx > 0 && onChange(options[idx - 1]);
  const next = () => idx < options.length - 1 && onChange(options[idx + 1]);

  return (
    <div className="px-4 pt-3.5 pb-4" style={{ borderBottom: "1px solid var(--border-1)" }}>
      <div className="flex items-center gap-3.5 mb-4">
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "rgba(77,168,255,0.08)", color: "var(--signal)" }}>
          <Sun size={15} />
        </div>
        <div className="flex-1">
          <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.fontSize}</p>
          <p className="text-xs" style={{ color: "var(--accent)", fontFamily: "var(--font-display)", fontWeight: 600 }}>{value}</p>
        </div>
      </div>

      {/* Preview text */}
      <div
        className="mx-1 px-3 py-2.5 rounded-xl mb-4"
        style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}
      >
        <p
          style={{
            color: "var(--text-2)",
            fontSize: idx === 0 ? "11px" : idx === 1 ? "14px" : idx === 2 ? "17px" : "20px",
            lineHeight: 1.5,
            transition: "font-size 200ms",
          }}
        >
          Güvenli ve özgür bir iletişim.
        </p>
      </div>

      {/* Slider row */}
      <div className="flex items-center gap-3">
        <button
          onClick={prev}
          disabled={idx === 0}
          className="w-8 h-8 rounded-xl flex items-center justify-center flex-shrink-0 disabled:opacity-30 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          style={{ border: "1px solid var(--border-2)", color: "var(--text-2)" }}
          aria-label="Küçült"
        >
          <Minus size={13} />
        </button>

        <div className="flex-1 relative h-6 flex items-center">
          {/* Track */}
          <div className="w-full h-1 rounded-full" style={{ background: "var(--surface-3)" }}>
            <div
              className="h-full rounded-full transition-all duration-200"
              style={{ width: `${pct}%`, background: "var(--accent)" }}
            />
          </div>
          {/* Thumb */}
          <div
            className="absolute w-4 h-4 rounded-full -translate-x-1/2"
            style={{
              left: `${pct}%`,
              background: "var(--accent)",
              boxShadow: "0 0 8px rgba(0,229,160,0.4)",
              transition: "left 200ms var(--ease-out-expo)",
            }}
          />
          {/* Click zones */}
          {options.map((_, i) => (
            <button
              key={i}
              onClick={() => onChange(options[i])}
              className="absolute h-full w-1/4 focus-visible:outline-none"
              style={{ left: `${(i / (options.length - 1)) * 100}%`, transform: "translateX(-50%)" }}
              aria-label={`${options[i]} yazı boyutu`}
            />
          ))}
        </div>

        <button
          onClick={next}
          disabled={idx === options.length - 1}
          className="w-8 h-8 rounded-xl flex items-center justify-center flex-shrink-0 disabled:opacity-30 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          style={{ border: "1px solid var(--border-2)", color: "var(--text-2)" }}
          aria-label="Büyüt"
        >
          <Plus size={13} />
        </button>
      </div>
    </div>
  );
}

// ── Bubble corner preview ─────────────────────────────────────────────────────

const BUBBLE_RADII = { Keskin: "4px", Hafif: "12px", Yuvarlak: "20px" } as const;

function BubbleCornerRow({
  value, onChange,
}: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="px-4 pt-3.5 pb-4" style={{ borderBottom: "1px solid var(--border-1)" }}>
      <div className="flex items-center gap-3.5 mb-3">
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "var(--surface-3)", color: "var(--text-3)" }}>
          <SquareRoundCorner size={15} />
        </div>
        <p className="text-sm font-medium" style={{ color: "var(--text-1)" }}>{t.bubbleCorners}</p>
      </div>
      <div className="flex gap-2 pl-12">
        {t.bubbleOptions.map((opt) => {
          const r = BUBBLE_RADII[opt as keyof typeof BUBBLE_RADII] || "8px";
          return (
            <button
              key={opt}
              onClick={() => onChange(opt)}
              className={cn(
                "flex-1 flex flex-col items-center gap-1.5 py-2 px-1 text-xs font-semibold transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
              )}
              style={{
                background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
                color: value === opt ? "var(--accent)" : "var(--text-3)",
                borderRadius: "12px",
                border: value === opt ? "1px solid rgba(0,229,160,0.2)" : "1px solid transparent",
              }}
              aria-pressed={value === opt}
            >
              <div
                style={{
                  width: 32,
                  height: 20,
                  background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
                  border: `1px solid ${value === opt ? "rgba(0,229,160,0.4)" : "var(--border-2)"}`,
                  borderRadius: r,
                }}
              />
              {opt}
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── Accent Color Preview ──────────────────────────────────────────────────────

function AccentPreview({ color }: { color: string }) {
  return (
    <div className="px-4 pt-3 pb-4">
      <div
        className="rounded-2xl p-3 flex gap-3"
        style={{ background: "var(--surface-3)", border: "1px solid var(--border-1)" }}
      >
        {/* Incoming message */}
        <div className="flex-1">
          <div
            className="inline-block px-3 py-2 text-xs rounded-2xl rounded-tl-sm mb-1"
            style={{ background: `${color}14`, border: `1px solid ${color}22`, color: "var(--text-2)", maxWidth: "80%" }}
          >
            Merhaba! Nasılsın?
          </div>
        </div>
        {/* Outgoing */}
        <div className="flex-none flex flex-col items-end gap-1">
          <div
            className="inline-block px-3 py-2 text-xs rounded-2xl rounded-tr-sm"
            style={{ background: `${color}22`, border: `1px solid ${color}33`, color }}
          >
            İyiyim, teşekkürler!
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AppearancePage() {
  const [accent, setAccent] = useSetting("accent", "#00e5a0");
  const [fontSize, setFontSize] = useSetting("fontSize", t.fontSizes[1]);
  const [systemFont, setSystemFont] = useSetting("systemFont", false);
  const [bubbleCorners, setBubbleCorners] = useSetting("bubbleCorners", t.bubbleOptions[2]);
  const [msgGrouping, setMsgGrouping] = useSetting("msgGrouping", true);
  const [timestamp, setTimestamp] = useSetting("timestamp", t.timestampOptions[0]);
  const [linkPreview, setLinkPreview] = useSetting("linkPreview", true);
  const [animations, setAnimations] = useSetting("animations", true);
  const [emojiAnim, setEmojiAnim] = useSetting("emojiAnim", true);
  const [sendAnim, setSendAnim] = useSetting("sendAnim", true);
  const [reducedMotion, setReducedMotion] = useSetting("reducedMotion", false);
  const [autoSave, setAutoSave] = useSetting("autoSave", false);
  const [cacheSize, setCacheSize] = useSetting("cacheSize", t.cacheSizes[1]);
  const [lowData, setLowData] = useSetting("lowData", false);

  // Apply accent to CSS var dynamically for preview
  useEffect(() => {
    if (typeof document !== "undefined") {
      document.documentElement.style.setProperty("--accent", accent);
    }
  }, [accent]);

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        {/* ── Theme ──────────────────────────────────────────────────────── */}
        <SectionHeader>{t.themeSection}</SectionHeader>
        <SettingCard>
          <div className="flex items-center gap-3.5 px-4 py-3.5">
            <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: "rgba(255,255,255,0.04)", color: "var(--text-2)" }}>
              <Sun size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>{t.theme}</p>
              <p className="text-xs" style={{ color: "var(--text-3)" }}>{t.themeDesc}</p>
            </div>
            <div
              className="w-8 h-8 rounded-xl flex items-center justify-center flex-shrink-0"
              style={{ background: "#020208", border: "2px solid var(--accent)", boxShadow: "0 0 8px rgba(0,229,160,0.2)" }}
              aria-hidden="true"
            >
              <div className="w-3 h-3 rounded-full" style={{ background: "var(--accent)" }} />
            </div>
          </div>
        </SettingCard>

        {/* ── Accent Color ────────────────────────────────────────────────── */}
        <SectionHeader>{t.accentSection}</SectionHeader>
        <SettingCard>
          <div className="px-4 pt-4 pb-3" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div className="flex gap-3 justify-between">
              {t.accents.map(({ name, value }) => (
                <button
                  key={value}
                  onClick={() => setAccent(value)}
                  className="flex flex-col items-center gap-1.5 focus-visible:outline-none"
                  aria-label={`${name} vurgu rengi`}
                  aria-pressed={accent === value}
                >
                  <div
                    className="w-9 h-9 rounded-full transition-all duration-200"
                    style={{
                      background: value,
                      boxShadow: accent === value ? `0 0 0 3px var(--surface-2), 0 0 0 5px ${value}` : "none",
                      transform: accent === value ? "scale(1.1)" : "scale(1)",
                    }}
                  />
                  <span className="text-[10px] font-medium" style={{ color: accent === value ? value : "var(--text-3)" }}>
                    {name}
                  </span>
                </button>
              ))}
            </div>
          </div>
          <AccentPreview color={accent} />
        </SettingCard>

        {/* ── Font ────────────────────────────────────────────────────────── */}
        <SectionHeader>{t.fontSection}</SectionHeader>
        <SettingCard>
          <FontSizeRow value={fontSize} options={t.fontSizes} onChange={setFontSize} />
          <ToggleRow
            icon={<span className="text-xs font-bold" style={{ fontFamily: "system-ui" }}>Aa</span>}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label={t.systemFont}
            desc={t.systemFontDesc}
            value={systemFont}
            onChange={setSystemFont}
            last
          />
        </SettingCard>

        {/* ── Chat Appearance ─────────────────────────────────────────────── */}
        <SectionHeader>{t.chatSection}</SectionHeader>
        <SettingCard>
          <BubbleCornerRow value={bubbleCorners} onChange={setBubbleCorners} />
          <ToggleRow icon={<MessageSquare size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.msgGrouping} desc={t.msgGroupingDesc} value={msgGrouping} onChange={setMsgGrouping} />
          <SelectRow icon={<Clock size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.timestamp} value={timestamp} options={t.timestampOptions} onChange={setTimestamp} />
          <ToggleRow icon={<Image size={15}/>} iconBg="rgba(77,168,255,0.08)" iconColor="var(--signal)" label={t.linkPreview} desc={t.linkPreviewDesc} value={linkPreview} onChange={setLinkPreview} last />
        </SettingCard>

        {/* ── Animations ──────────────────────────────────────────────────── */}
        <SectionHeader>{t.motionSection}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Zap size={15}/>} iconBg="rgba(255,170,0,0.08)" iconColor="#ffaa00" label={t.animations} desc={t.animationsDesc} value={animations} onChange={setAnimations} />
          <ToggleRow icon={<span className="text-base leading-none" role="img" aria-label="emoji">✨</span>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.emojiAnim} desc={t.emojiAnimDesc} value={emojiAnim} onChange={setEmojiAnim} />
          <ToggleRow icon={<MessageSquare size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.sendAnim} desc={t.sendAnimDesc} value={sendAnim} onChange={setSendAnim} />
          <ToggleRow icon={<Minus size={15}/>} iconBg="rgba(255,64,88,0.06)" iconColor="rgba(255,64,88,0.7)" label={t.reducedMotion} desc={t.reducedMotionDesc} value={reducedMotion} onChange={setReducedMotion} last />
        </SettingCard>

        {/* ── Media ───────────────────────────────────────────────────────── */}
        <SectionHeader>{t.mediaSection}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Image size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.autoSave} desc={t.autoSaveDesc} value={autoSave} onChange={setAutoSave} />
          <SelectRow icon={<Zap size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.cacheSize} value={cacheSize} options={t.cacheSizes} onChange={setCacheSize} />
          <ToggleRow icon={<Wifi size={15}/>} iconBg="rgba(255,170,0,0.08)" iconColor="#ffaa00" label={t.lowData} desc={t.lowDataDesc} value={lowData} onChange={setLowData} last />
        </SettingCard>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}

// SquareRoundCorner icon (lucide doesn't have it, define inline)
function SquareRoundCorner({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 9a6 6 0 0 1 6-6h6a6 6 0 0 1 6 6v6a6 6 0 0 1-6 6H9a6 6 0 0 1-6-6V9Z"/>
    </svg>
  );
}
