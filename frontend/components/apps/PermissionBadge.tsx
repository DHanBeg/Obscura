"use client";

import {
  MessageCircle, Wallet, Shield, Globe, Database,
  Image as ImageIcon, Bell, Users, Lock, Zap,
} from "lucide-react";
import { cn } from "@/lib/cn";

const PERMISSION_META: Record<
  string,
  { label: string; icon: React.ReactNode; tone: "default" | "zk" | "danger" }
> = {
  messaging:           { label: "Mesajlaşma",       icon: <MessageCircle size={13} />, tone: "default" },
  "messaging.send":    { label: "Mesaj gönderme",   icon: <MessageCircle size={13} />, tone: "default" },
  wallet:              { label: "Cüzdan",           icon: <Wallet size={13} />,        tone: "default" },
  "wallet.read":       { label: "Bakiye okuma",     icon: <Wallet size={13} />,        tone: "default" },
  "wallet.pay":        { label: "Ödeme",            icon: <Wallet size={13} />,        tone: "danger" },
  zk:                  { label: "ZK kanıt",         icon: <Shield size={13} />,        tone: "zk" },
  "zk.generateProof":  { label: "ZK kanıt üret",    icon: <Shield size={13} />,        tone: "zk" },
  "zk.verifyProof":    { label: "ZK kanıt doğrula", icon: <Shield size={13} />,        tone: "zk" },
  "zk.creditScore":    { label: "Kredi puanı oku",  icon: <Zap size={13} />,           tone: "zk" },
  network:             { label: "Ağ erişimi",       icon: <Globe size={13} />,         tone: "default" },
  storage:             { label: "Yerel depolama",   icon: <Database size={13} />,      tone: "default" },
  media:               { label: "Medya",            icon: <ImageIcon size={13} />,     tone: "default" },
  notifications:       { label: "Bildirim",         icon: <Bell size={13} />,          tone: "default" },
  identity:            { label: "Kimlik",           icon: <Users size={13} />,         tone: "default" },
};

interface PermissionBadgeProps {
  permission: string;
  /** ZK izinleri özel olarak işaretle */
  isZk?: boolean;
  className?: string;
}

export function PermissionBadge({ permission, isZk, className }: PermissionBadgeProps) {
  const meta = PERMISSION_META[permission] ?? {
    label: permission,
    icon: <Lock size={13} />,
    tone: "default" as const,
  };
  const tone = isZk ? "zk" : meta.tone;

  const toneCls =
    tone === "zk"
      ? "bg-purple-900/30 border-tier3/30 text-tier3"
      : tone === "danger"
      ? "bg-amber/10 border-amber/30 text-amber"
      : "bg-raised border-border text-body";

  return (
    <div
      className={cn(
        "inline-flex items-center gap-1.5 h-7 px-2.5 rounded-full border text-xs font-medium",
        toneCls,
        className
      )}
    >
      {meta.icon}
      <span>{meta.label}</span>
      {isZk && (
        <span className="ml-0.5 px-1 rounded-sm bg-tier3/20 text-[9px] uppercase tracking-wide">
          ZK
        </span>
      )}
    </div>
  );
}
