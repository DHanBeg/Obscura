"use client";

import { useState, useEffect } from "react";
import {
  Github, ExternalLink, Shield, Lock, Code2,
  Copy, Check, Heart, ChevronRight,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { AppShell } from "@/components/AppShell";
import { getAppVersion } from "@/lib/tauri";

const TEXTS = {
  tr: {
    title: "Hakkında",
    back: "Ayarlar",
    appName: "Obscura",
    tagline: "Sıfır Bilgi. Tam Gizlilik.",
    version: "Sürüm",
    build: "Yapı",
    protocol: "Protokol",
    libs: "Kullanılan Kütüphaneler",
    libsList: [
      { name: "snarkjs", desc: "Groth16 ZK kanıt üretimi" },
      { name: "libsignal-protocol", desc: "X3DH + Double Ratchet E2EE" },
      { name: "Next.js 14", desc: "Web uygulama çerçevesi" },
      { name: "Go 1.22", desc: "Backend node" },
      { name: "Circom 2.1.6", desc: "ZK devre dili" },
      { name: "cloudflare/circl", desc: "Post-kuantum kriptografi" },
    ],
    legal: "Hukuki",
    privacy: "Gizlilik Politikası",
    terms: "Kullanım Koşulları",
    openSource: "Açık Kaynak Lisanslar",
    contact: "İletişim",
    github: "GitHub",
    securityReport: "Güvenlik Raporu",
    copyVersion: "Sürümü kopyala",
    copied: "Kopyalandı",
    madeWith: "ile yapıldı",
    specVersion: "Spec v3.0-FINAL",
    coreCrypto: "Çekirdek Kriptografi",
    cryptoItems: [
      "BN254 eliptik eğrisi (ZK kanıtları)",
      "X25519 ECDH (Diffie-Hellman)",
      "Ed25519 imzalama",
      "AES-256-GCM mesaj şifreleme",
      "SHA-256 / Poseidon hash",
      "Kyber-768 post-kuantum KEM",
      "Dilithium3 post-kuantum imza",
    ],
  },
};

const locale = "tr" as const;
const t = TEXTS[locale];

export default function AboutPage() {
  const router = useRouter();
  const [appVersion, setAppVersion] = useState("web");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    getAppVersion().then(setAppVersion).catch(() => {});
  }, []);

  const copyVersion = () => {
    navigator.clipboard.writeText(`Obscura v${appVersion}`).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        {/* Hero */}
        <div className="flex flex-col items-center pt-10 pb-8 px-6">
          {/* Logo */}
          <div
            className="w-20 h-20 rounded-[24px] overflow-hidden mb-5"
            style={{
              boxShadow: "0 0 40px rgba(0,229,160,0.15), 0 8px 24px rgba(0,0,0,0.6)",
              border: "1px solid rgba(0,229,160,0.15)",
            }}
          >
            <img src="/logo.jpeg" alt="Obscura logo" className="w-full h-full object-cover" style={{ mixBlendMode: "screen" }} />
          </div>

          <h2
            className="text-3xl font-black mb-1"
            style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.04em" }}
          >
            {t.appName}
          </h2>
          <p className="text-sm mb-4" style={{ color: "var(--text-3)" }}>
            {t.tagline}
          </p>

          {/* Version badge */}
          <button
            onClick={copyVersion}
            className="flex items-center gap-2 h-8 px-4 rounded-full transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] hover:bg-white/[0.04]"
            style={{
              background: "var(--surface-2)",
              border: "1px solid var(--border-2)",
              color: copied ? "var(--accent)" : "var(--text-3)",
            }}
            aria-label={t.copyVersion}
          >
            <span className="text-xs font-mono font-semibold">
              v{appVersion}
            </span>
            {copied ? <Check size={11} /> : <Copy size={11} />}
          </button>
        </div>

        {/* Version info */}
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {[
            { label: t.version, value: `v${appVersion}` },
            { label: t.build, value: "315beab" },
            { label: t.protocol, value: t.specVersion },
          ].map(({ label, value }, i, arr) => (
            <div
              key={label}
              className="flex items-center px-4 py-3.5"
              style={i < arr.length - 1 ? { borderBottom: "1px solid var(--border-1)" } : undefined}
            >
              <span className="text-sm flex-1" style={{ color: "var(--text-3)" }}>{label}</span>
              <span className="text-sm font-semibold font-mono" style={{ color: "var(--text-2)" }}>{value}</span>
            </div>
          ))}
        </div>

        {/* Core Crypto */}
        <div className="px-4 pb-2">
          <span className="section-label">{t.coreCrypto}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          <div className="px-4 py-4">
            <div className="flex flex-col gap-2">
              {t.cryptoItems.map((item) => (
                <div key={item} className="flex items-center gap-2">
                  <div
                    className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                    style={{ background: "var(--accent)" }}
                  />
                  <span className="text-xs" style={{ color: "var(--text-2)" }}>{item}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Libraries */}
        <div className="px-4 pb-2">
          <span className="section-label">{t.libs}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {t.libsList.map(({ name, desc }, i) => (
            <div
              key={name}
              className="flex items-center gap-3 px-4 py-3"
              style={i < t.libsList.length - 1 ? { borderBottom: "1px solid var(--border-1)" } : undefined}
            >
              <div
                className="w-7 h-7 rounded-lg flex items-center justify-center flex-shrink-0"
                style={{ background: "var(--surface-3)" }}
              >
                <Code2 size={12} style={{ color: "var(--text-3)" }} />
              </div>
              <div className="flex-1">
                <p className="text-xs font-semibold font-mono" style={{ color: "var(--text-2)" }}>{name}</p>
                <p className="text-[11px]" style={{ color: "var(--text-3)" }}>{desc}</p>
              </div>
            </div>
          ))}
        </div>

        {/* Legal & Contact */}
        <div className="px-4 pb-2">
          <span className="section-label">{t.legal}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {[
            { icon: <Lock size={14} />, label: t.privacy, href: "/privacy" },
            { icon: <Shield size={14} />, label: t.terms, href: "/terms" },
            { icon: <Code2 size={14} />, label: t.openSource, href: null },
          ].map(({ icon, label, href }, i) => (
            <button
              key={label}
              onClick={() => href && router.push(href)}
              className="w-full flex items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-white/[0.025] focus-visible:outline-none focus-visible:bg-white/[0.025]"
              style={i < 2 ? { borderBottom: "1px solid var(--border-1)" } : undefined}
            >
              <div
                className="w-8 h-8 rounded-xl flex items-center justify-center flex-shrink-0"
                style={{ background: "var(--surface-3)", color: "var(--text-3)" }}
              >
                {icon}
              </div>
              <span className="text-sm flex-1" style={{ color: "var(--text-2)" }}>{label}</span>
              <ChevronRight size={14} style={{ color: "var(--text-3)" }} />
            </button>
          ))}
        </div>

        {/* Contact */}
        <div className="px-4 pb-2">
          <span className="section-label">{t.contact}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {[
            { icon: <Github size={14} />, label: t.github, href: "https://github.com/obscura-protocol" },
            { icon: <Shield size={14} />, label: t.securityReport, href: "mailto:security@obscura.app" },
          ].map(({ icon, label, href }, i) => (
            <button
              key={label}
              onClick={() => window.open(href, "_blank", "noopener")}
              className="w-full flex items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-white/[0.025] focus-visible:outline-none focus-visible:bg-white/[0.025]"
              style={i === 0 ? { borderBottom: "1px solid var(--border-1)" } : undefined}
            >
              <div
                className="w-8 h-8 rounded-xl flex items-center justify-center flex-shrink-0"
                style={{ background: "var(--surface-3)", color: "var(--text-3)" }}
              >
                {icon}
              </div>
              <span className="text-sm flex-1" style={{ color: "var(--text-2)" }}>{label}</span>
              <ExternalLink size={12} style={{ color: "var(--text-3)" }} />
            </button>
          ))}
        </div>

        {/* Made with love */}
        <div className="flex items-center justify-center gap-1.5 py-2 mb-4">
          <span className="text-xs" style={{ color: "var(--text-3)" }}>{t.madeWith}</span>
          <Heart size={11} style={{ color: "var(--accent)" }} />
          <span className="text-xs" style={{ color: "var(--text-3)" }}>açık kaynak</span>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
