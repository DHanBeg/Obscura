"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import {
  MessageCircle, Phone, Wallet, TrendingUp, Vote, Layers, Globe,
  ArrowRightLeft, Scale, Cpu, MapPin, Settings, ArrowLeft,
  Users, Search, Camera, Plus, Code2, Calendar,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";

/* ── Types ─────────────────────────────────────────────────── */

interface NavItem {
  id: string;
  icon: React.ReactNode;
  href: string;
  tooltip: string;
  badge?: number;
}

interface GravityWellProps {
  onNewChat?: () => void;
  onSearch?: () => void;
  showBack?: boolean;
  onBack?: () => void;
  title?: string;
}

/* ── ObscuraOrb — app logo ──────────────────────────────────── */

function ObscuraOrb({ size = 28 }: { size?: number }) {
  return (
    <div
      style={{
        width: size,
        height: size,
        borderRadius: "50%",
        overflow: "hidden",
        flexShrink: 0,
        boxShadow: "0 0 8px rgba(0,229,160,0.25)",
        border: "1px solid rgba(0,229,160,0.2)",
      }}
      aria-hidden="true"
    >
      <img
        src="/logo.jpeg"
        alt=""
        style={{ width: "100%", height: "100%", objectFit: "cover", mixBlendMode: "screen" }}
        draggable={false}
      />
    </div>
  );
}

/* ── Tooltip ────────────────────────────────────────────────── */

function Tooltip({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="relative group/tip flex items-center justify-center">
      {children}
      <span
        className={cn(
          "absolute bottom-[calc(100%+8px)] left-1/2 -translate-x-1/2",
          "px-2.5 py-1 rounded-lg text-[11px] font-semibold tracking-wide whitespace-nowrap",
          "bg-[var(--surface-3)] border border-[var(--border-2)] text-[var(--text-1)]",
          "pointer-events-none opacity-0 group-hover/tip:opacity-100",
          "translate-y-1 group-hover/tip:translate-y-0",
          "transition-all duration-150 shadow-lg z-50",
        )}
      >
        {label}
      </span>
    </div>
  );
}

/* ── Main component ─────────────────────────────────────────── */

export function GravityWell({ onNewChat, onSearch, showBack, onBack, title }: GravityWellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(false);
  const [radialOpen, setRadialOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const holdTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const conversations = useStore((s) => s.conversations);

  useEffect(() => { setMounted(true); }, []);

  const totalUnread = conversations.reduce((a, c) => a + c.unread_count, 0);

  const navItems: NavItem[] = [
    { id: "chats",     icon: <MessageCircle size={20} strokeWidth={1.75} />, href: "/chats",     tooltip: "Sohbetler", badge: totalUnread },
    { id: "calls",     icon: <Phone         size={20} strokeWidth={1.75} />, href: "/calls",     tooltip: "Aramalar" },
    { id: "wallet",    icon: <Wallet        size={20} strokeWidth={1.75} />, href: "/wallet",    tooltip: "Cüzdan" },
    { id: "staking",   icon: <TrendingUp    size={20} strokeWidth={1.75} />, href: "/staking",   tooltip: "Staking" },
    { id: "governance",icon: <Vote          size={20} strokeWidth={1.75} />, href: "/governance",tooltip: "Yönetim" },
    { id: "apps",      icon: <Layers        size={20} strokeWidth={1.75} />, href: "/apps",      tooltip: "Uygulamalar" },
    { id: "nodes",     icon: <Globe         size={20} strokeWidth={1.75} />, href: "/nodes",     tooltip: "Node'lar" },
    { id: "bridge",    icon: <ArrowRightLeft size={20} strokeWidth={1.75} />,href: "/bridge",    tooltip: "Bridge" },
    { id: "dao",       icon: <Scale         size={20} strokeWidth={1.75} />, href: "/dao",       tooltip: "DAO" },
    { id: "sequencer", icon: <Cpu           size={20} strokeWidth={1.75} />, href: "/sequencer", tooltip: "Sequencer" },
    { id: "events",     icon: <Calendar      size={20} strokeWidth={1.75} />, href: "/events",     tooltip: "Etkinlikler" },
    { id: "location",   icon: <MapPin        size={20} strokeWidth={1.75} />, href: "/location",   tooltip: "Konum Kanıtı" },
    { id: "dev-tools",  icon: <Code2         size={20} strokeWidth={1.75} />, href: "/dev-tools",  tooltip: "Geliştirici Araçları" },
    { id: "settings",   icon: <Settings      size={20} strokeWidth={1.75} />, href: "/settings",   tooltip: "Ayarlar" },
  ];

  const activeId = navItems.find((n) => pathname?.startsWith(n.href))?.id ?? "chats";

  /* Long-press → radial */
  const handlePointerDown = useCallback(() => {
    holdTimer.current = setTimeout(() => setRadialOpen(true), 420);
  }, []);

  const handlePointerUp = useCallback(() => {
    if (holdTimer.current) { clearTimeout(holdTimer.current); holdTimer.current = null; }
  }, []);

  const handleOrbTap = useCallback(() => {
    if (!radialOpen) setExpanded((v) => !v);
  }, [radialOpen]);

  /* Close expanded on route change */
  useEffect(() => { setExpanded(false); }, [pathname]);

  const radialActions = [
    { icon: <MessageCircle size={16} />, label: "Yeni sohbet", action: () => { onNewChat?.(); setRadialOpen(false); } },
    { icon: <Users         size={16} />, label: "Grup oluştur", action: () => { setRadialOpen(false); } },
    { icon: <Search        size={16} />, label: "Ara",          action: () => { onSearch?.();  setRadialOpen(false); } },
    { icon: <Camera        size={16} />, label: "Durum",        action: () => { setRadialOpen(false); } },
  ];

  /* ── Back mode ── */
  if (showBack) {
    return (
      <div
        aria-label="Geri"
        className="fixed bottom-0 left-0 right-0 z-50 flex items-end justify-center pointer-events-none"
        style={{ paddingBottom: "calc(24px + env(safe-area-inset-bottom, 0px))" }}
      >
        <div className="pointer-events-auto animate-in stagger-1">
          <button
            onClick={onBack ?? (() => router.back())}
            aria-label={title ? `Geri: ${title}` : "Geri"}
            className={cn(
              "flex items-center gap-2 px-5 h-12 rounded-full",
              "glass border border-[var(--border-2)]",
              "text-[var(--text-2)] text-sm font-semibold",
              "hover:text-[var(--text-1)] hover:border-[rgba(0,229,160,0.25)]",
              "transition-all duration-200 shadow-lg",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-0",
            )}
          >
            <ArrowLeft size={16} strokeWidth={2.5} />
            {title && <span>{title}</span>}
          </button>
        </div>
      </div>
    );
  }

  if (!mounted) return null;

  return (
    <>
      {/* ── Radial overlay ── */}
      {radialOpen && (
        <div
          className="fixed inset-0 z-40"
          onClick={() => setRadialOpen(false)}
          aria-label="Menüyü kapat"
          role="button"
          tabIndex={-1}
          onKeyDown={(e) => e.key === "Escape" && setRadialOpen(false)}
        >
          {/* Scrim */}
          <div className="absolute inset-0 bg-[var(--void)]/70 backdrop-blur-sm" />

          {/* Actions — stacked vertically above orb */}
          <div
            className="absolute left-1/2 -translate-x-1/2 flex flex-col-reverse items-center gap-2.5"
            style={{ bottom: "calc(100px + env(safe-area-inset-bottom, 0px))" }}
          >
            {radialActions.map((action, i) => (
              <button
                key={i}
                onClick={(e) => { e.stopPropagation(); action.action(); }}
                className={cn(
                  "flex items-center gap-3 pl-3.5 pr-5 h-11 rounded-full",
                  "glass border border-[var(--border-2)]",
                  "text-[var(--text-1)] text-sm font-semibold",
                  "hover:border-[rgba(0,229,160,0.3)] hover:text-[var(--accent)]",
                  "shadow-lg transition-all duration-150",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                )}
                style={{
                  animation: `slideUp 280ms cubic-bezier(0.16,1,0.3,1) both`,
                  animationDelay: `${i * 40}ms`,
                }}
              >
                <span className="text-[var(--accent)]">{action.icon}</span>
                <span>{action.label}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ── Main gravity well ── */}
      <div
        className="fixed left-0 right-0 z-50 flex items-end justify-center pointer-events-none"
        style={{ bottom: 0, paddingBottom: "calc(24px + env(safe-area-inset-bottom, 0px))" }}
      >
        {/* Pill / orb wrapper */}
        <div
          className={cn(
            "pointer-events-auto relative flex items-center",
            "glass border shadow-xl",
            "transition-all duration-[320ms] ease-[cubic-bezier(0.16,1,0.3,1)]",
            expanded
              ? "rounded-[24px] border-[var(--border-2)] h-[60px] px-2 gap-0.5"
              : "rounded-full border-[rgba(0,229,160,0.2)] h-[60px] w-[60px] justify-center",
          )}
          style={
            expanded
              ? {
                  width: Math.min(window.innerWidth - 32, 520),
                  boxShadow: "0 8px 32px rgba(0,0,0,0.8), 0 0 0 1px rgba(0,229,160,0.08)",
                }
              : {
                  boxShadow: "0 0 24px rgba(0,229,160,0.1), 0 8px 24px rgba(0,0,0,0.7)",
                }
          }
        >
          {/* ── Collapsed: Orb ── */}
          {!expanded && (
            <button
              className="w-full h-full flex items-center justify-center relative focus-visible:outline-none"
              onPointerDown={handlePointerDown}
              onPointerUp={handlePointerUp}
              onPointerLeave={handlePointerUp}
              onClick={handleOrbTap}
              aria-label="Navigasyon menüsü"
              aria-expanded={false}
            >
              <ObscuraOrb size={30} />
              {totalUnread > 0 && (
                <span
                  aria-label={`${totalUnread} okunmamış`}
                  className={cn(
                    "absolute top-[11px] right-[11px]",
                    "w-[9px] h-[9px] rounded-full",
                    "bg-[var(--accent)]",
                    "ring-2 ring-[var(--void)]",
                  )}
                />
              )}
            </button>
          )}

          {/* ── Expanded: Nav items ── */}
          {expanded && (
            <>
              {/* Orb — shifted to left end */}
              <button
                className={cn(
                  "relative flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center",
                  "transition-all duration-200",
                  "hover:bg-[rgba(0,229,160,0.08)]",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                )}
                onClick={() => setExpanded(false)}
                aria-label="Menüyü kapat"
              >
                <ObscuraOrb size={22} />
              </button>

              {/* Divider */}
              <div className="w-px h-5 bg-[var(--border-2)] mx-1 flex-shrink-0" />

              {/* Nav items — scrollable if many */}
              <div className="flex items-center gap-0.5 flex-1 overflow-x-auto scrollbar-none">
                {navItems.map((item, i) => {
                  const isActive = item.id === activeId;
                  return (
                    <Tooltip key={item.id} label={item.tooltip}>
                      <button
                        onClick={() => { router.push(item.href); setExpanded(false); }}
                        aria-label={item.tooltip}
                        aria-current={isActive ? "page" : undefined}
                        className={cn(
                          "relative flex items-center justify-center w-10 h-10 rounded-2xl flex-shrink-0",
                          "transition-all duration-150 no-select",
                          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
                          isActive
                            ? "text-[var(--accent)] bg-[rgba(0,229,160,0.1)]"
                            : "text-[var(--text-2)] hover:text-[var(--text-1)] hover:bg-[var(--surface-2)]",
                        )}
                        style={{
                          animation: expanded ? `slideRight 260ms cubic-bezier(0.16,1,0.3,1) both` : undefined,
                          animationDelay: expanded ? `${i * 25}ms` : undefined,
                          ...(isActive ? { boxShadow: "0 0 12px rgba(0,229,160,0.12)" } : {}),
                        }}
                      >
                        {item.icon}
                        {item.badge ? (
                          <span className="absolute top-1 right-1 w-[7px] h-[7px] rounded-full bg-[var(--accent)] ring-2 ring-[var(--void)]" />
                        ) : null}
                      </button>
                    </Tooltip>
                  );
                })}
              </div>
            </>
          )}
        </div>

        {/* New chat FAB — chats page, collapsed */}
        {!expanded && !radialOpen && activeId === "chats" && onNewChat && (
          <button
            onClick={onNewChat}
            aria-label="Yeni sohbet"
            className={cn(
              "pointer-events-auto absolute right-6 bottom-0",
              "mb-[calc(24px+env(safe-area-inset-bottom,0px))]",
              "w-11 h-11 rounded-full",
              "bg-[rgba(0,229,160,0.08)] border border-[rgba(0,229,160,0.2)]",
              "flex items-center justify-center text-[var(--accent)]",
              "hover:bg-[rgba(0,229,160,0.16)] transition-colors duration-200 shadow-lg",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]",
            )}
            style={{ animation: "scaleIn 200ms cubic-bezier(0.16,1,0.3,1) both" }}
          >
            <Plus size={18} strokeWidth={2.5} />
          </button>
        )}
      </div>

      {/* Keyframes for expanded stagger */}
      <style>{`
        @keyframes slideRight {
          from { opacity: 0; transform: translateX(-8px) scale(0.9); }
          to   { opacity: 1; transform: translateX(0)    scale(1);   }
        }
        @keyframes scaleIn {
          from { opacity: 0; transform: scale(0.75); }
          to   { opacity: 1; transform: scale(1);    }
        }
        .scrollbar-none { scrollbar-width: none; }
        .scrollbar-none::-webkit-scrollbar { display: none; }
      `}</style>
    </>
  );
}
