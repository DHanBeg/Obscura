"use client";
import { cn } from "@/lib/cn";

interface LogoProps {
  size?: number;
  className?: string;
  animated?: boolean;
}

export function ObscuraLogo({ size = 40, className, animated }: LogoProps) {
  return (
    <div
      className={cn(animated && "animate-float", className)}
      style={{
        width: size,
        height: size,
        borderRadius: "50%",
        overflow: "hidden",
        flexShrink: 0,
      }}
    >
      <img
        src="/logo.jpeg"
        alt="Obscura"
        style={{ width: "100%", height: "100%", objectFit: "cover", mixBlendMode: "screen" }}
        draggable={false}
      />
    </div>
  );
}

export function ObscuraWordmark({ className }: { className?: string }) {
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <ObscuraLogo size={30} />
      <span
        style={{
          fontFamily: "var(--font-display)",
          fontWeight: 800,
          fontSize: 19,
          color: "var(--text-1)",
          letterSpacing: "-0.04em",
        }}
      >
        obscura
      </span>
    </div>
  );
}
