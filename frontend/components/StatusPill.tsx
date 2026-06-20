"use client";

import { cn } from "@/lib/cn";

// ── Types ──────────────────────────────────────────────────────────────────────

export type ConnectionStatus = "secure" | "warning" | "offline";

interface Props {
  status?: ConnectionStatus;
  className?: string;
}

const CONFIG = {
  secure: {
    dot: "var(--accent)",
    dotGlow: "rgba(0,229,160,0.4)",
    label: "Şifreli bağlantı",
    labelColor: "var(--accent)",
    bg: "rgba(0,229,160,0.06)",
    border: "rgba(0,229,160,0.15)",
  },
  warning: {
    dot: "#FFAA00",
    dotGlow: "rgba(255,170,0,0.4)",
    label: "Bağlantı kuruluyor",
    labelColor: "#FFAA00",
    bg: "rgba(255,170,0,0.06)",
    border: "rgba(255,170,0,0.15)",
  },
  offline: {
    dot: "#FF4444",
    dotGlow: "rgba(255,68,68,0.4)",
    label: "Bağlantı yok",
    labelColor: "#FF4444",
    bg: "rgba(255,68,68,0.06)",
    border: "rgba(255,68,68,0.15)",
  },
} as const;

// ── Component ──────────────────────────────────────────────────────────────────

export function StatusPill({ status = "secure", className }: Props) {
  const cfg = CONFIG[status];

  return (
    <div
      role="status"
      aria-label={cfg.label}
      className={cn(
        "inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full",
        className,
      )}
      style={{
        background: cfg.bg,
        border: `1px solid ${cfg.border}`,
      }}
    >
      {/* Animated dot */}
      <span
        className="relative flex-shrink-0"
        aria-hidden="true"
        style={{ width: 7, height: 7 }}
      >
        {/* Pulse ring — only on secure/warning */}
        {status !== "offline" && (
          <span
            className="absolute inset-0 rounded-full animate-ping opacity-60"
            style={{
              background: cfg.dot,
              animationDuration: status === "secure" ? "2.5s" : "1s",
            }}
          />
        )}
        <span
          className="absolute inset-0 rounded-full"
          style={{ background: cfg.dot }}
        />
      </span>

      <span
        className="text-[11px] font-semibold leading-none"
        style={{ color: cfg.labelColor, letterSpacing: "0.01em" }}
      >
        {cfg.label}
      </span>
    </div>
  );
}
