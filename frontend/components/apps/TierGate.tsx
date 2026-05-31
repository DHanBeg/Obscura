"use client";

import { Lock } from "lucide-react";
import { cn } from "@/lib/cn";
import { tierLabel } from "@/lib/format";

interface TierGateProps {
  /** Kullanıcının mevcut tier'ı */
  userTier: number;
  /** Erişim için gerekli minimum tier */
  requiredTier: number;
  /** İçerik gösterilsin mi yoksa kapı engelleme mi? */
  children: React.ReactNode;
  /** Engellendiğinde özel mesaj */
  message?: string;
  /** Compact varyant — küçük rozet */
  variant?: "default" | "overlay" | "inline";
  className?: string;
}

/**
 * Tier kapısı — yetersiz seviyeli kullanıcıyı engeller.
 * `overlay`: çocukları grilstirir ve üstüne kilit kartı koyar (kartlar için).
 * `inline`: tek satır rozet (linkler için).
 * `default`: tam kart blok mesaj.
 */
export function TierGate({
  userTier,
  requiredTier,
  children,
  message,
  variant = "default",
  className,
}: TierGateProps) {
  const allowed = userTier >= requiredTier;
  if (allowed) return <>{children}</>;

  const label = tierLabel(requiredTier);
  const msg = message || `${label} tier gerekli`;

  if (variant === "inline") {
    return (
      <span
        className={cn(
          "inline-flex items-center gap-1 px-2 py-0.5 rounded-full",
          "bg-muted/60 border border-border text-[10px] font-medium text-dim",
          className
        )}
      >
        <Lock size={9} />
        {msg}
      </span>
    );
  }

  if (variant === "overlay") {
    return (
      <div className={cn("relative", className)}>
        <div className="opacity-30 pointer-events-none select-none filter grayscale">
          {children}
        </div>
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-full glass border border-border shadow-void-sm">
            <Lock size={11} className="text-amber" />
            <span className="text-[11px] font-semibold text-body">{msg}</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "card p-6 flex flex-col items-center text-center animate-fade-in",
        className
      )}
    >
      <div className="w-12 h-12 rounded-2xl bg-amber/15 flex items-center justify-center mb-3">
        <Lock size={20} className="text-amber" />
      </div>
      <p className="text-head font-semibold text-sm">{msg}</p>
      <p className="text-dim text-xs mt-1 max-w-[260px]">
        Tier seviyenizi yükseltmek için kredi puanınızı artırın.
      </p>
    </div>
  );
}
