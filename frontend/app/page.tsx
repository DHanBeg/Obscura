"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ObscuraLogo } from "@/components/ui/ObscuraLogo";

export default function Home() {
  const router = useRouter();

  useEffect(() => {
    const token = localStorage.getItem("obscura_token");
    // Brief splash then redirect
    const t = setTimeout(() => {
      router.replace(token ? "/chats" : "/login");
    }, 800);
    return () => clearTimeout(t);
  }, [router]);

  return (
    <div className="fixed inset-0 flex flex-col items-center justify-center void-bg">
      {/* Ambient glow */}
      <div className="absolute w-64 h-64 rounded-full bg-accent/5 blur-3xl animate-pulse-soft" />

      <div className="relative flex flex-col items-center gap-4 animate-float">
        <ObscuraLogo size={64} />
        <div className="flex flex-col items-center gap-1">
          <h1 className="text-2xl font-semibold tracking-tight text-head">obscura</h1>
          <p className="text-xs text-dim tracking-widest uppercase">Zero-Knowledge Messaging</p>
        </div>
      </div>

      {/* Loading dots */}
      <div className="absolute bottom-16 flex gap-1.5">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="typing-dot w-1.5 h-1.5 rounded-full bg-accent/40"
          />
        ))}
      </div>
    </div>
  );
}
