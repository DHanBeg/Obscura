"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Shield, Zap, Globe, ArrowRight } from "lucide-react";

const FEATURES = [
  {
    icon: Shield,
    title: "Uçtan uca şifreli",
    sub: "Signal Protokolü — her mesaj kilitli",
  },
  {
    icon: Zap,
    title: "Sıfır bilgi kanıtı",
    sub: "Kimliğin doğrulanır, paylaşılmaz",
  },
  {
    icon: Globe,
    title: "Merkezi olmayan",
    sub: "P2P ağ — hiçbir sunucu izlemez",
  },
];

export default function WelcomePage() {
  const router = useRouter();
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const token = localStorage.getItem("obscura_token");
    if (token) {
      router.replace("/chats");
    } else {
      setReady(true);
    }
  }, [router]);

  if (!ready) {
    return (
      <div className="fixed inset-0 flex items-center justify-center void-bg">
        <div
          className="w-2 h-2 rounded-full animate-pulse"
          style={{ background: "var(--accent)", opacity: 0.6 }}
        />
      </div>
    );
  }

  return (
    <div
      className="fixed inset-0 overflow-y-auto"
      style={{ background: "var(--void)" }}
    >
      {/* Ambient glow */}
      <div
        className="fixed top-0 left-1/2 -translate-x-1/2 w-[500px] h-[500px] pointer-events-none"
        style={{
          background:
            "radial-gradient(ellipse at top, rgba(0,229,160,0.07) 0%, transparent 65%)",
        }}
      />

      <div className="flex flex-col items-center min-h-full px-6 pb-10">

        {/* ── Hero ─────────────────────────────────────────────────────── */}
        <div className="flex flex-col items-center pt-20 pb-10">
          {/* Logo with rotating rings */}
          <div
            className="relative flex items-center justify-center mb-8"
            style={{ width: 200, height: 200 }}
          >
            {/* Rings */}
            <svg
              width="200"
              height="200"
              viewBox="0 0 200 200"
              fill="none"
              className="absolute inset-0 pointer-events-none"
              aria-hidden="true"
            >
              <circle
                cx="100" cy="100" r="90"
                stroke="rgba(0,229,160,0.10)"
                strokeWidth="1"
                strokeDasharray="10 8"
                style={{
                  transformOrigin: "100px 100px",
                  animation: "obs-spin 22s linear infinite",
                }}
              />
              <circle
                cx="100" cy="100" r="76"
                stroke="rgba(0,229,160,0.07)"
                strokeWidth="0.75"
                strokeDasharray="5 14"
                style={{
                  transformOrigin: "100px 100px",
                  animation: "obs-spin 16s linear infinite reverse",
                }}
              />
              <circle
                cx="100" cy="100" r="60"
                stroke="rgba(0,229,160,0.04)"
                strokeWidth="0.5"
                strokeDasharray="3 18"
                style={{
                  transformOrigin: "100px 100px",
                  animation: "obs-spin 30s linear infinite",
                }}
              />
            </svg>

            {/* Eagle logo */}
            <div
              className="relative z-10 rounded-full overflow-hidden"
              style={{
                width: 120,
                height: 120,
                boxShadow:
                  "0 0 48px rgba(0,229,160,0.20), 0 0 96px rgba(0,229,160,0.08)",
                border: "1.5px solid rgba(0,229,160,0.22)",
              }}
            >
              <img
                src="/logo.jpeg"
                alt="Obscura"
                style={{ width: "100%", height: "100%", objectFit: "cover" }}
                draggable={false}
              />
            </div>
          </div>

          {/* Title + tagline */}
          <h1
            style={{
              fontFamily: "var(--font-display)",
              fontSize: 42,
              fontWeight: 800,
              color: "var(--text-1)",
              letterSpacing: "-0.04em",
              lineHeight: 1,
              textAlign: "center",
              marginBottom: 14,
            }}
          >
            OBSCURA
          </h1>
          <p
            style={{
              fontSize: 15,
              color: "var(--text-2)",
              lineHeight: 1.6,
              textAlign: "center",
              maxWidth: 260,
            }}
          >
            Gizlilik bir seçim değil,
            <br />
            yaşam tarzıdır.
          </p>
        </div>

        {/* ── Features ─────────────────────────────────────────────────── */}
        <div
          className="flex flex-col gap-2.5 w-full mb-10"
          style={{ maxWidth: 360 }}
        >
          {FEATURES.map(({ icon: Icon, title, sub }, i) => (
            <div
              key={title}
              className="flex items-center gap-4 px-4 py-3.5 rounded-2xl animate-in"
              style={{
                background: "var(--surface-2)",
                border: "1px solid var(--border-1)",
                animationDelay: `${i * 60}ms`,
              }}
            >
              <div
                className="w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0"
                style={{ background: "var(--accent-muted)" }}
              >
                <Icon size={18} style={{ color: "var(--accent)" }} />
              </div>
              <div>
                <p
                  style={{
                    fontSize: 14,
                    fontWeight: 600,
                    color: "var(--text-1)",
                    fontFamily: "var(--font-display)",
                    lineHeight: 1.3,
                  }}
                >
                  {title}
                </p>
                <p
                  style={{
                    fontSize: 12,
                    color: "var(--text-3)",
                    marginTop: 2,
                    lineHeight: 1.4,
                  }}
                >
                  {sub}
                </p>
              </div>
            </div>
          ))}
        </div>

        {/* ── CTA ──────────────────────────────────────────────────────── */}
        <div
          className="flex flex-col items-center gap-4 w-full"
          style={{ maxWidth: 360 }}
        >
          <button
            onClick={() => router.push("/login")}
            className="w-full flex items-center justify-center gap-2.5"
            style={{
              height: 56,
              borderRadius: 18,
              background: "var(--accent)",
              color: "var(--void)",
              fontFamily: "var(--font-display)",
              fontSize: 16,
              fontWeight: 700,
              letterSpacing: "-0.01em",
              boxShadow:
                "0 0 24px rgba(0,229,160,0.28), 0 4px 12px rgba(0,0,0,0.4)",
              border: "none",
              cursor: "pointer",
            }}
          >
            Başla
            <ArrowRight size={18} />
          </button>

          {/* ToS note */}
          <p
            style={{
              fontSize: 11,
              color: "var(--text-4)",
              textAlign: "center",
              lineHeight: 1.6,
              maxWidth: 280,
            }}
          >
            Devam ederek{" "}
            <span style={{ color: "var(--text-3)", textDecoration: "underline", cursor: "pointer" }}>
              Gizlilik Politikası
            </span>
            'nı ve{" "}
            <span style={{ color: "var(--text-3)", textDecoration: "underline", cursor: "pointer" }}>
              Kullanım Şartları
            </span>
            'nı kabul etmiş olursunuz.
          </p>
        </div>
      </div>

      <style>{`
        @keyframes obs-spin {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}
