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

// Deterministic color from name
function avatarColor(name: string): string {
  const colors = [
    "from-purple-900/60 to-purple-800/40",
    "from-blue-900/60 to-blue-800/40",
    "from-emerald-900/60 to-emerald-800/40",
    "from-rose-900/60 to-rose-800/40",
    "from-amber-900/60 to-amber-800/40",
    "from-cyan-900/60 to-cyan-800/40",
    "from-indigo-900/60 to-indigo-800/40",
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  return colors[Math.abs(hash) % colors.length];
}

export function Avatar({ name, src, avatarUrl, tier, size = "md", online, className }: AvatarProps) {
  const s = sizes[size];
  const ds = dotSizes[size];
  const imgSrc = avatarUrl || src;

  return (
    <div className={cn("relative flex-shrink-0", className)}>
      <div
        className={cn(
          "relative rounded-full overflow-hidden flex items-center justify-center",
          `bg-gradient-to-br ${avatarColor(name || "?")}`,
          s
        )}
      >
        {imgSrc ? (
          <img src={imgSrc} alt={name} className="w-full h-full object-cover" />
        ) : (
          <span className="font-semibold text-head/80 no-select">
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
