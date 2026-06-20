"use client";

import { useState, useCallback } from "react";
import {
  ScanEye, MessageSquare, Sparkles, ShieldCheck,
  Users, MapPin, ArrowRightLeft, AlertTriangle,
  FlaskConical,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Atış Alanı",
    back: "Ayarlar",
    warningTitle: "Deneysel Özellikler",
    warningDesc: "Bu özellikler kararsız olabilir veya beklenmedik şekillerde çalışabilir. Geribildirim için geliştirici kanalından ulaşın.",
    features: [
      {
        key: "zkVisualizer",
        icon: "zk",
        label: "ZK Proof Görselleştirici",
        desc: "Sıfır bilgi kanıtı üretim sürecini adım adım görsel olarak izle",
        badge: "BETA",
        badgeColor: "rgba(0,229,160,0.1)",
        badgeTextColor: "var(--accent)",
        iconBg: "rgba(0,229,160,0.08)",
        iconColor: "var(--accent)",
      },
      {
        key: "newBubbles",
        icon: "bubble",
        label: "Yeni Balonu Tasarımı",
        desc: "Alternatif sohbet balonu stili — daha compact, daha hızlı",
        badge: "BETA",
        badgeColor: "rgba(77,168,255,0.1)",
        badgeTextColor: "var(--signal)",
        iconBg: "rgba(77,168,255,0.08)",
        iconColor: "var(--signal)",
      },
      {
        key: "aiSummary",
        icon: "ai",
        label: "AI Konuşma Özeti",
        desc: "Uzun konuşmaları yerel model ile otomatik özetle",
        badge: "YAKINDA",
        badgeColor: "rgba(139,92,246,0.1)",
        badgeTextColor: "#a78bfa",
        iconBg: "rgba(139,92,246,0.08)",
        iconColor: "#a78bfa",
        disabled: true,
      },
      {
        key: "quantumMode",
        icon: "quantum",
        label: "Kuantum Güvenli Mod",
        desc: "Dilithium3 imza göstergesini etkinleştir, post-quantum hazırlık durumunu göster",
        badge: "DENEYSEL",
        badgeColor: "rgba(255,170,0,0.1)",
        badgeTextColor: "#ffaa00",
        iconBg: "rgba(255,170,0,0.08)",
        iconColor: "#ffaa00",
      },
      {
        key: "multiAccount",
        icon: "accounts",
        label: "Çoklu Hesap",
        desc: "Birden fazla DID hesabını aynı anda yönet",
        badge: "GELİŞTİRİLİYOR",
        badgeColor: "rgba(255,64,88,0.08)",
        badgeTextColor: "rgba(255,64,88,0.7)",
        iconBg: "rgba(255,64,88,0.06)",
        iconColor: "rgba(255,64,88,0.7)",
        disabled: true,
      },
      {
        key: "locationProof",
        icon: "location",
        label: "Coğrafi ZK Kanıtı",
        desc: "GPS konumunu sıfır bilgi kanıtı ile paylaş. Konum verisi cihazdan çıkmaz.",
        badge: "BETA",
        badgeColor: "rgba(0,229,160,0.1)",
        badgeTextColor: "var(--accent)",
        iconBg: "rgba(0,229,160,0.08)",
        iconColor: "var(--accent)",
      },
      {
        key: "bridge",
        icon: "bridge",
        label: "Token Köprüsü Arayüzü",
        desc: "ETH ↔ DOT testnet token köprüsü. Gerçek varlıkları köprüleme — yalnızca test",
        badge: "TESTNET",
        badgeColor: "rgba(255,170,0,0.1)",
        badgeTextColor: "#ffaa00",
        iconBg: "rgba(255,170,0,0.08)",
        iconColor: "#ffaa00",
      },
    ],
    enabled: "Etkin",
    disabled: "Devre Dışı",
    comingSoon: "Çok Yakında",
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

// ── Helpers ───────────────────────────────────────────────────────────────────

function useSetting(key: string): [boolean, (v: boolean) => void] {
  const storageKey = `obscura_settings_labs_${key}`;
  const [value, setValue] = useState(() => {
    if (typeof window === "undefined") return false;
    try { return localStorage.getItem(storageKey) === "true"; }
    catch { return false; }
  });
  const set = useCallback((v: boolean) => {
    setValue(v);
    try { localStorage.setItem(storageKey, String(v)); } catch {}
  }, [storageKey]);
  return [value, set];
}

// ── Feature icon map ──────────────────────────────────────────────────────────

function FeatureIcon({ type, size = 16 }: { type: string; size?: number }) {
  switch (type) {
    case "zk":       return <ScanEye size={size} />;
    case "bubble":   return <MessageSquare size={size} />;
    case "ai":       return <Sparkles size={size} />;
    case "quantum":  return <ShieldCheck size={size} />;
    case "accounts": return <Users size={size} />;
    case "location": return <MapPin size={size} />;
    case "bridge":   return <ArrowRightLeft size={size} />;
    default:         return <FlaskConical size={size} />;
  }
}

// ── Toggle ────────────────────────────────────────────────────────────────────

function Toggle({ on, onChange, disabled }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <div
      onClick={() => !disabled && onChange(!on)}
      role="switch"
      aria-checked={on}
      aria-disabled={disabled}
      tabIndex={disabled ? -1 : 0}
      onKeyDown={(e) => !disabled && (e.key === "Enter" || e.key === " ") && onChange(!on)}
      className={cn(
        "relative w-11 h-6 rounded-full flex-shrink-0 transition-colors duration-200",
        disabled ? "opacity-30 cursor-not-allowed" : "cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      )}
      style={{ background: on && !disabled ? "var(--accent)" : "var(--surface-3)", border: on ? "none" : "1px solid var(--border-2)" }}
    >
      <div
        className="absolute top-0.5 w-5 h-5 rounded-full transition-all duration-200"
        style={{ background: "white", boxShadow: "0 1px 4px rgba(0,0,0,0.4)", left: on ? "22px" : "2px" }}
      />
    </div>
  );
}

// ── Feature Card ──────────────────────────────────────────────────────────────

interface Feature {
  key: string;
  icon: string;
  label: string;
  desc: string;
  badge: string;
  badgeColor: string;
  badgeTextColor: string;
  iconBg: string;
  iconColor: string;
  disabled?: boolean;
}

function FeatureCard({
  feature,
  value,
  onChange,
  last,
}: {
  feature: Feature;
  value: boolean;
  onChange: (v: boolean) => void;
  last?: boolean;
}) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <div
        className={cn(
          "flex items-start gap-3.5 px-4 py-4 transition-colors duration-100",
          !feature.disabled && "hover:bg-white/[0.015]"
        )}
      >
        {/* Icon */}
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5"
          style={{ background: feature.iconBg, color: feature.iconColor }}
        >
          <FeatureIcon type={feature.icon} />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5 flex-wrap">
            <span className="text-sm font-semibold" style={{ color: feature.disabled ? "var(--text-3)" : "var(--text-1)" }}>
              {feature.label}
            </span>
            <span
              className="inline-flex items-center h-4 px-2 rounded-full text-[9px] font-bold tracking-wider flex-shrink-0"
              style={{
                background: feature.badgeColor,
                color: feature.badgeTextColor,
                fontFamily: "var(--font-display)",
                letterSpacing: "0.08em",
              }}
            >
              {feature.badge}
            </span>
          </div>
          <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
            {feature.desc}
          </p>
        </div>

        {/* Toggle */}
        <div className="flex-shrink-0 mt-1">
          <Toggle on={value} onChange={onChange} disabled={feature.disabled} />
        </div>
      </div>
    </div>
  );
}

// ── Individual toggles must be hoisted to avoid conditional hooks ─────────────

function LabsContent() {
  const [v0, setV0] = useSetting(t.features[0].key);
  const [v1, setV1] = useSetting(t.features[1].key);
  const [v2, setV2] = useSetting(t.features[2].key);
  const [v3, setV3] = useSetting(t.features[3].key);
  const [v4, setV4] = useSetting(t.features[4].key);
  const [v5, setV5] = useSetting(t.features[5].key);
  const [v6, setV6] = useSetting(t.features[6].key);

  const values = [v0, v1, v2, v3, v4, v5, v6];
  const setters = [setV0, setV1, setV2, setV3, setV4, setV5, setV6];

  const enabledCount = values.filter(Boolean).length;

  return (
    <>
      {/* Warning banner */}
      <div
        className="mx-3 mt-4 mb-4 px-4 py-3.5 rounded-2xl"
        style={{ background: "rgba(255,170,0,0.05)", border: "1px solid rgba(255,170,0,0.15)" }}
      >
        <div className="flex items-start gap-3">
          <AlertTriangle size={16} className="flex-shrink-0 mt-0.5" style={{ color: "#ffaa00" }} />
          <div>
            <p className="text-sm font-semibold mb-1" style={{ color: "#ffaa00" }}>
              {t.warningTitle}
            </p>
            <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
              {t.warningDesc}
            </p>
          </div>
        </div>
      </div>

      {/* Enabled count badge */}
      {enabledCount > 0 && (
        <div className="px-4 pb-2 flex items-center gap-2">
          <span
            className="badge badge-accent text-[10px]"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {enabledCount} özellik etkin
          </span>
        </div>
      )}

      {/* Feature list */}
      <div
        className="mx-3 mb-4 overflow-hidden"
        style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
      >
        {t.features.map((feature, i) => (
          <FeatureCard
            key={feature.key}
            feature={feature}
            value={values[i]}
            onChange={setters[i]}
            last={i === t.features.length - 1}
          />
        ))}
      </div>

      {/* Footer note */}
      <div className="px-4 pb-4">
        <p className="text-[11px] text-center" style={{ color: "var(--text-3)", lineHeight: 1.7 }}>
          Deneysel özelliklerle ilgili sorunları bildirmek için{" "}
          <span style={{ color: "var(--accent)" }}>geliştirici kanalı</span>nı kullanabilirsiniz.
        </p>
      </div>
    </>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function LabsPage() {
  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <div className="flex items-center gap-2.5">
            <FlaskConical size={20} style={{ color: "#a78bfa" }} />
            <h1 className="page-title">{t.title}</h1>
          </div>
        </div>

        <LabsContent />

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
