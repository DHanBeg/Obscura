"use client";
import { Shield, ShieldCheck } from "lucide-react";
import { cn } from "@/lib/cn";

interface Props {
  verified?: boolean;
  className?: string;
  showLabel?: boolean;
}

export function EncryptionBadge({ verified = true, className, showLabel }: Props) {
  return (
    <div
      className={cn(
        "inline-flex items-center gap-1.5",
        verified ? "text-accent" : "text-amber",
        className
      )}
    >
      {verified
        ? <ShieldCheck size={13} strokeWidth={2.5} />
        : <Shield size={13} strokeWidth={2.5} />
      }
      {showLabel && (
        <span className="text-xs font-medium">
          {verified ? "Uçtan uca şifreli" : "Doğrulama bekleniyor"}
        </span>
      )}
    </div>
  );
}
