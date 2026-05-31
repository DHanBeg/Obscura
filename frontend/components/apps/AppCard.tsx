"use client";

import { useRouter } from "next/navigation";
import { Download, Check, Star, Lock } from "lucide-react";
import { cn } from "@/lib/cn";
import type { MiniApp } from "@/lib/api";
import { tierLabel, tierColor } from "@/lib/format";

interface AppCardProps {
  app: MiniApp;
  userTier: number;
  /** Liste içindeki sırada animasyon gecikmesi için */
  index?: number;
  variant?: "default" | "compact";
}

export function AppCard({ app, userTier, index = 0, variant = "default" }: AppCardProps) {
  const router = useRouter();
  const minTier = app.manifest.minTier ?? 1;
  const locked = userTier < minTier;
  const installed = !!app.installed;
  const compact = variant === "compact";

  return (
    <button
      onClick={() => router.push(`/apps/${app.id}`)}
      style={{ animationDelay: `${index * 30}ms` }}
      className={cn(
        "group relative w-full text-left animate-slide-up",
        "card p-3.5 flex items-center gap-3",
        "hover:border-accent/30 hover:bg-raised/60",
        "transition-all duration-200 ease-spring",
        "active:scale-[0.985]",
        locked && "opacity-70"
      )}
    >
      {/* Icon */}
      <div
        className={cn(
          "flex-shrink-0 flex items-center justify-center rounded-2xl",
          "bg-gradient-to-br from-raised to-muted border border-border",
          "text-head font-bold select-none",
          compact ? "w-11 h-11 text-base" : "w-14 h-14 text-xl",
          locked && "filter grayscale"
        )}
      >
        {app.manifest.icon ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={app.manifest.icon} alt="" className="w-full h-full rounded-2xl object-cover" />
        ) : (
          app.manifest.name?.[0]?.toUpperCase() ?? "?"
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <p className={cn("font-semibold truncate", compact ? "text-sm text-body" : "text-sm text-head")}>
            {app.manifest.name}
          </p>
          {app.featured && (
            <Star size={11} className="text-amber flex-shrink-0" fill="currentColor" />
          )}
        </div>
        {app.manifest.description && (
          <p className="text-xs text-dim mt-0.5 line-clamp-1">
            {app.manifest.description}
          </p>
        )}
        <div className="flex items-center gap-2 mt-1.5 text-[10px]">
          <span className={cn("font-semibold", tierColor(minTier))}>
            {tierLabel(minTier)}+
          </span>
          {app.install_count !== undefined && (
            <>
              <span className="text-dim">·</span>
              <span className="text-dim">{formatInstalls(app.install_count)} kurulum</span>
            </>
          )}
        </div>
      </div>

      {/* Action */}
      <div className="flex-shrink-0">
        {locked ? (
          <div className="flex items-center gap-1 px-2.5 h-7 rounded-full bg-muted/60 border border-border text-[10px] font-medium text-dim">
            <Lock size={10} />
            <span>Kilitli</span>
          </div>
        ) : installed ? (
          <div className="flex items-center gap-1 px-2.5 h-7 rounded-full bg-accent/15 border border-accent/30 text-[10px] font-semibold text-accent">
            <Check size={11} />
            <span>Kurulu</span>
          </div>
        ) : (
          <div className="flex items-center gap-1 px-2.5 h-7 rounded-full bg-raised border border-border text-[10px] font-medium text-body group-hover:border-accent/40 group-hover:text-accent transition-colors">
            <Download size={11} />
            <span>Kur</span>
          </div>
        )}
      </div>
    </button>
  );
}

function formatInstalls(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}
