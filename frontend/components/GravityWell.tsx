"use client";

import { useState, useRef, useCallback } from "react";
import { useRouter, usePathname } from "next/navigation";
import {
  MessageCircle, Phone, Settings, Search,
  Plus, X, Camera, Users, ArrowLeft,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { ObscuraLogo } from "./ui/ObscuraLogo";
import { useStore } from "@/lib/store";

interface NavItem {
  id: string;
  icon: React.ReactNode;
  label: string;
  href: string;
  badge?: number;
}

interface GravityWellProps {
  onNewChat?: () => void;
  onSearch?: () => void;
  showBack?: boolean;
  onBack?: () => void;
  title?: string;
}

export function GravityWell({ onNewChat, onSearch, showBack, onBack, title }: GravityWellProps) {
  const router = useRouter();
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(false);
  const [radialOpen, setRadialOpen] = useState(false);
  const holdTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const conversations = useStore((s) => s.conversations);

  const totalUnread = conversations.reduce((a, c) => a + c.unread_count, 0);

  const navItems: NavItem[] = [
    {
      id: "chats",
      icon: <MessageCircle size={20} strokeWidth={2} />,
      label: "Sohbetler",
      href: "/chats",
      badge: totalUnread,
    },
    {
      id: "calls",
      icon: <Phone size={20} strokeWidth={2} />,
      label: "Aramalar",
      href: "/calls",
    },
    {
      id: "settings",
      icon: <Settings size={20} strokeWidth={2} />,
      label: "Ayarlar",
      href: "/settings",
    },
  ];

  const activeId = navItems.find((n) => pathname.startsWith(n.href))?.id ?? "chats";

  const handlePointerDown = useCallback(() => {
    holdTimer.current = setTimeout(() => {
      setRadialOpen(true);
    }, 400);
  }, []);

  const handlePointerUp = useCallback(() => {
    if (holdTimer.current) {
      clearTimeout(holdTimer.current);
      holdTimer.current = null;
    }
  }, []);

  const handleTap = useCallback(() => {
    if (!radialOpen) {
      setExpanded((v) => !v);
    }
  }, [radialOpen]);

  const radialActions = [
    { icon: <MessageCircle size={18} />, label: "Yeni sohbet", action: () => { onNewChat?.(); setRadialOpen(false); } },
    { icon: <Users size={18} />, label: "Grup oluştur", action: () => { setRadialOpen(false); } },
    { icon: <Search size={18} />, label: "Ara", action: () => { onSearch?.(); setRadialOpen(false); } },
    { icon: <Camera size={18} />, label: "Durum", action: () => { setRadialOpen(false); } },
  ];

  if (showBack) {
    return (
      <div className="fixed bottom-0 left-0 right-0 z-50 flex items-end justify-center pb-6 pb-safe pointer-events-none">
        <div className="pointer-events-auto">
          <button
            onClick={onBack ?? (() => router.back())}
            className={cn(
              "flex items-center gap-2 px-5 h-12 rounded-full glass border border-border",
              "text-body text-sm font-medium hover:text-head hover:border-accent/30",
              "transition-all duration-200 shadow-void-md animate-slide-up"
            )}
          >
            <ArrowLeft size={16} strokeWidth={2.5} />
            {title && <span>{title}</span>}
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Radial overlay */}
      {radialOpen && (
        <div
          className="fixed inset-0 z-40"
          onClick={() => setRadialOpen(false)}
        >
          <div className="absolute inset-0 bg-void/60 backdrop-blur-sm" />
          {/* Radial actions */}
          <div className="absolute bottom-24 left-1/2 -translate-x-1/2 flex flex-col items-center gap-3">
            {radialActions.map((action, i) => (
              <button
                key={i}
                onClick={(e) => { e.stopPropagation(); action.action(); }}
                className={cn(
                  "flex items-center gap-3 px-5 h-12 rounded-full glass border border-border",
                  "text-body text-sm font-medium hover:border-accent/40 hover:text-head",
                  "shadow-void-md transition-all duration-200"
                )}
                style={{
                  animation: `radialIn ${0.1 + i * 0.05}s cubic-bezier(0.16,1,0.3,1) both`,
                  animationDelay: `${i * 40}ms`,
                }}
              >
                <span className="text-accent">{action.icon}</span>
                <span>{action.label}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Main Gravity Well */}
      <div className="fixed bottom-0 left-0 right-0 z-50 flex items-end justify-center pb-6 pb-safe pointer-events-none">
        <div className="pointer-events-auto">
          <div
            className={cn(
              "relative glass border border-border shadow-void-lg rounded-full",
              "transition-all duration-300 ease-spring overflow-hidden",
              expanded ? "rounded-3xl" : "rounded-full"
            )}
            style={{
              width: expanded ? Math.min(320, window?.innerWidth - 32 || 320) : 56,
              height: 56,
            }}
          >
            {/* Collapsed: single orb */}
            {!expanded && (
              <button
                className="w-full h-full flex items-center justify-center relative"
                onPointerDown={handlePointerDown}
                onPointerUp={handlePointerUp}
                onPointerLeave={handlePointerUp}
                onClick={handleTap}
              >
                <ObscuraLogo size={28} />
                {totalUnread > 0 && (
                  <span className="absolute top-1 right-1 w-4 h-4 bg-accent rounded-full text-void text-[9px] font-bold flex items-center justify-center">
                    {totalUnread > 9 ? "9+" : totalUnread}
                  </span>
                )}
              </button>
            )}

            {/* Expanded: nav items */}
            {expanded && (
              <div className="flex items-center h-full px-2 gap-1 animate-fade-in">
                {navItems.map((item) => {
                  const isActive = item.id === activeId;
                  return (
                    <button
                      key={item.id}
                      onClick={() => { router.push(item.href); setExpanded(false); }}
                      className={cn(
                        "relative flex flex-col items-center justify-center flex-1 h-12 rounded-2xl gap-0.5",
                        "transition-all duration-200 no-select",
                        isActive
                          ? "text-accent bg-accent/10"
                          : "text-sub hover:text-body hover:bg-raised"
                      )}
                    >
                      {item.icon}
                      <span className="text-[9px] font-semibold tracking-wide uppercase opacity-80">
                        {item.label}
                      </span>
                      {item.badge ? (
                        <span className="absolute top-1.5 right-3 w-4 h-4 bg-accent rounded-full text-void text-[9px] font-bold flex items-center justify-center">
                          {item.badge > 9 ? "9+" : item.badge}
                        </span>
                      ) : null}
                    </button>
                  );
                })}

                {/* Close */}
                <button
                  onClick={() => setExpanded(false)}
                  className="w-10 h-10 flex items-center justify-center rounded-2xl text-dim hover:text-sub"
                >
                  <X size={16} />
                </button>
              </div>
            )}
          </div>

          {/* New chat FAB (only on chats page, collapsed state) */}
          {!expanded && activeId === "chats" && onNewChat && (
            <button
              onClick={onNewChat}
              className={cn(
                "absolute -top-14 right-0",
                "w-10 h-10 rounded-full bg-accent/10 border border-accent/20",
                "flex items-center justify-center text-accent",
                "hover:bg-accent/20 transition-colors duration-200 shadow-void-sm",
                "animate-scale-in"
              )}
            >
              <Plus size={18} strokeWidth={2.5} />
            </button>
          )}
        </div>
      </div>
    </>
  );
}
