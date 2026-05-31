"use client";
import { cn } from "@/lib/cn";
import { initials, tierColor } from "@/lib/format";

interface AvatarProps {
  name: string;
  /** Legacy prop — use avatarUrl instead */
  src?: string;
  /** URL from MinIO/backend */
  avatarUrl?: string;
  tier?: number;
  size?: "xs" | "sm" | "md" | "lg" | "xl";
  online?: boolean;
  className?: string;
}

const sizes = {
  xs: "w-7 h-7 text-xs",
  sm: "w-9 h-9 text-sm",
  md: "w-11 h-11 text-base",
  lg: "w-14 h-14 text-lg",
  xl: "w-20 h-20 text-2xl",
};

const dotSizes = {
  xs: "w-2 h-2 -bottom-0.5 -right-0.5",
  sm: "w-2.5 h-2.5 -bottom-0.5 -right-0.5",
  md: "w-3 h-3 bottom-0 right-0",
  lg: "w-3.5 h-3.5 bottom-0.5 right-0.5",
  xl: "w-4 h-4 bottom-1 right-1",
};

// Deterministic Telegram-style palette from name
function avatarPalette(name: string): { bg: string; color: string } {
  const palettes = [
    { bg: "rgba(139,92,246,0.24)", color: "#A78BFA" },  // violet
    { bg: "rgba(59,130,246,0.24)", color: "#60A5FA" },   // blue
    { bg: "rgba(16,185,129,0.24)", color: "#34D399" },   // emerald
    { bg: "rgba(239,68,68,0.24)",  color: "#F87171" },   // red
    { bg: "rgba(245,158,11,0.24)", color: "#FBBF24" },   // amber
    { bg: "rgba(14,165,233,0.24)", color: "#38BDF8" },   // sky
    { bg: "rgba(236,72,153,0.24)", color: "#F472B6" },   // pink
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  return palettes[Math.abs(hash) % palettes.length];
}

export function Avatar({ name, src, avatarUrl, tier, size = "md", online, className }: AvatarProps) {
  const s = sizes[size];
  const ds = dotSizes[size];
  const imgSrc = avatarUrl || src;
  const palette = avatarPalette(name || "?");

  return (
    <div className={cn("relative flex-shrink-0", className)}>
      <div
        className={cn(
          "relative rounded-full overflow-hidden flex items-center justify-center",
          s
        )}
        style={{ background: palette.bg }}
      >
        {imgSrc ? (
          <img src={imgSrc} alt={name} className="w-full h-full object-cover" />
        ) : (
          <span className="font-semibold no-select" style={{ color: palette.color }}>
            {initials(name || "?")}
          </span>
        )}
        {/* Tier ring */}
        {tier && tier >= 3 && (
          <div
            className={cn(
              "absolute inset-0 rounded-full border",
              tier === 3 && "border-tier3/40",
              tier === 4 && "border-tier4/50",
              tier === 5 && "border-tier5/60"
            )}
          />
        )}
      </div>
      {/* Online dot */}
      {online !== undefined && (
        <span
          className={cn(
            "absolute rounded-full border-2 border-void",
            ds,
            online ? "bg-accent" : "bg-dim"
          )}
        />
      )}
    </div>
  );
}
