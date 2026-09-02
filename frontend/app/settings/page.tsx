"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ChevronRight, LogOut, Trash2, AlertCircle,
  Shield, Bell, Palette, FlaskConical,
  Terminal, Info, Copy, Check, Fingerprint,
  ChevronDown, Loader2, Zap, Globe, Cpu, Wifi,
  FileCode2, Calendar,
} from "lucide-react";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { StatusPill } from "@/components/StatusPill";
import { maskPhone } from "@/lib/format";
import { deleteToken } from "@/lib/tauri";

// ── CopyDID ───────────────────────────────────────────────────────────────────

function CopyDID({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };
  return (
    <button
      onClick={copy}
      className="flex items-center gap-1.5 transition-colors duration-100 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--accent)] rounded"
      style={{ color: copied ? "var(--status)" : "var(--text-3)" }}
      aria-label={label}
    >
      <span className="font-mono text-[11px]">
        {value.length > 24 ? `${value.slice(0, 13)}…${value.slice(-9)}` : value}
      </span>
      {copied
        ? <Check size={12} style={{ color: "var(--status)" }} className="check-appear" />
        : <Copy size={11} />
      }
    </button>
  );
}

// ── Category Row ──────────────────────────────────────────────────────────────

interface CategoryProps {
  href: string;
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc: string;
  last?: boolean;
  badge?: string;
}

function CategoryRow({ href, icon, iconBg, iconColor, label, desc, last, badge }: CategoryProps) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <Link
        href={href}
        className="flex items-center gap-3.5 px-4 py-3.5 transition-colors duration-100 hover:bg-white/[0.025] active:bg-white/[0.04] focus-visible:outline-none focus-visible:bg-white/[0.03]"
      >
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
          style={{ background: iconBg, color: iconColor }}
        >
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold leading-tight" style={{ color: "var(--text-1)" }}>
            {label}
          </p>
          <p className="text-xs mt-0.5 truncate" style={{ color: "var(--text-3)" }}>
            {desc}
          </p>
        </div>
        {badge && (
          <span className="badge badge-warning text-[10px] mr-1 flex-shrink-0">
            {badge}
          </span>
        )}
        <ChevronRight size={14} className="flex-shrink-0" style={{ color: "var(--text-3)" }} />
      </Link>
    </div>
  );
}

// ── Developer Toggle ──────────────────────────────────────────────────────────

function DevToggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!value)}
      role="switch"
      aria-checked={value}
      aria-label="Geliştirici modunu aç/kapat"
      className="flex items-center gap-3.5 px-4 py-3.5 w-full text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none focus-visible:bg-white/[0.02]"
    >
      <div
        className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
        style={{ background: "rgba(255,255,255,0.04)", color: "var(--text-2)" }}
      >
        <Terminal size={16} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold leading-tight" style={{ color: "var(--text-1)" }}>
          Geliştirici Modu
        </p>
        <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
          Teknik bilgileri göster
        </p>
      </div>
      <div
        className="relative w-11 h-6 rounded-full flex-shrink-0 transition-colors duration-200"
        style={{ background: value ? "var(--accent)" : "var(--surface-3)", border: value ? "none" : "1px solid var(--border-2)" }}
      >
        <div
          className="absolute top-0.5 w-5 h-5 rounded-full transition-all duration-200"
          style={{ background: "white", boxShadow: "0 1px 4px rgba(0,0,0,0.4)", left: value ? "22px" : "2px" }}
        />
      </div>
    </button>
  );
}

// ── Developer Info Section ────────────────────────────────────────────────────

function DevSection() {
  const { user } = useStore();
  return (
    <div className="mx-3 mb-4 overflow-hidden animate-slide-down" style={{
      background: "var(--surface-2)",
      border: "1px solid rgba(99,102,241,0.2)",
      borderRadius: "20px",
    }}>
      <div className="px-4 py-3 flex items-center gap-2" style={{ borderBottom: "1px solid var(--border-1)" }}>
        <FileCode2 size={13} style={{ color: "var(--indigo)", flexShrink: 0 }} />
        <span className="section-label" style={{ color: "var(--indigo)" }}>Geliştirici Bilgileri</span>
      </div>
      {[
        { label: "Node Seçimi",     icon: <Globe size={14} />,   value: user?.node_id || "node-1 · 12ms" },
        { label: "ZK Metrikleri",   icon: <Cpu size={14} />,     value: "7 circuit" },
        { label: "WebSocket Debug", icon: <Wifi size={14} />,    value: "Bağlı" },
        { label: "Sürüm",          icon: <Zap size={14} />,     value: "v3.0.0" },
      ].map((row, i, arr) => (
        <div
          key={row.label}
          className="flex items-center gap-3 px-4 py-3"
          style={i < arr.length - 1 ? { borderBottom: "1px solid var(--border-1)" } : undefined}
        >
          <span style={{ color: "var(--text-3)", flexShrink: 0 }}>{row.icon}</span>
          <span className="text-sm flex-1" style={{ color: "var(--text-2)" }}>{row.label}</span>
          <span className="font-mono text-xs" style={{ color: "var(--text-3)" }}>{row.value}</span>
        </div>
      ))}
      {/* DID row */}
      {user?.did && (
        <div className="flex items-center gap-3 px-4 py-3" style={{ borderTop: "1px solid var(--border-1)" }}>
          <Fingerprint size={14} style={{ color: "var(--text-3)", flexShrink: 0 }} />
          <span className="text-sm flex-1" style={{ color: "var(--text-2)" }}>DID</span>
          <CopyDID value={user.did} label="DID kopyala" />
        </div>
      )}
    </div>
  );
}

// ── Delete Account Modal ──────────────────────────────────────────────────────

function DeleteAccountModal({ onClose }: { onClose: () => void }) {
  const [confirm, setConfirm] = useState("");
  const [deleting, setDeleting] = useState(false);

  const isReady = confirm.trim().toUpperCase() === "SİL" || confirm.trim().toUpperCase() === "DELETE";

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const handleDelete = async () => {
    if (!isReady) return;
    setDeleting(true);
    await new Promise((r) => setTimeout(r, 1200));
    setDeleting(false);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end" role="dialog" aria-modal="true" aria-label="Hesabı sil">
      <div
        className="absolute inset-0 bg-black/70"
        style={{ backdropFilter: "blur(10px)" }}
        onClick={onClose}
      />
      <div
        className="relative w-full px-5 pt-7 pb-10 animate-slide-up"
        style={{
          background: "var(--surface)",
          borderTop: "1px solid rgba(255,64,88,0.2)",
          borderRadius: "28px 28px 0 0",
        }}
      >
        <div className="w-10 h-1 rounded-full mx-auto mb-6" style={{ background: "var(--border-2)" }} />

        <div className="w-12 h-12 rounded-2xl flex items-center justify-center mx-auto mb-4"
          style={{ background: "rgba(255,64,88,0.1)" }}>
          <Trash2 size={22} style={{ color: "var(--error)" }} />
        </div>

        <h2 className="text-lg font-bold text-center mb-2" style={{ fontFamily: "var(--font-display)", color: "var(--text-1)" }}>
          Hesabınızı silmek istediğinizden emin misiniz?
        </h2>
        <p className="text-sm text-center leading-relaxed mb-6" style={{ color: "var(--text-3)" }}>
          Bu işlem geri alınamaz. Tüm mesajlarınız, bağlantılarınız ve verileriniz kalıcı olarak silinir.
        </p>

        <label className="block text-xs font-semibold mb-1.5" style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}>
          Onaylamak için &ldquo;SİL&rdquo; yazın
        </label>
        <input
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          placeholder="SİL"
          className="field mb-4"
          style={{ borderColor: isReady ? "rgba(255,64,88,0.4)" : undefined }}
          autoFocus
        />

        <div className="flex gap-2">
          <button onClick={onClose} className="btn-secondary flex-1 h-11 text-sm">
            İptal
          </button>
          <button
            onClick={handleDelete}
            disabled={!isReady || deleting}
            className="flex-1 h-11 rounded-full text-sm font-semibold flex items-center justify-center gap-2 transition-all duration-150 active:scale-[0.97] disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ background: "var(--error)", color: "white" }}
          >
            {deleting ? <Loader2 size={16} className="animate-spin" /> : null}
            Hesabı Kalıcı Olarak Sil
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export default function SettingsPage() {
  const router = useRouter();
  const { user, ws, setUser } = useStore();

  const [showDID, setShowDID] = useState(false);
  const [showLogout, setShowLogout] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);

  // Developer mode — persisted in localStorage
  const [devMode, setDevMode] = useState(() => {
    if (typeof window === "undefined") return false;
    return localStorage.getItem("obscura_dev_mode") === "true";
  });

  const toggleDevMode = (v: boolean) => {
    setDevMode(v);
    try { localStorage.setItem("obscura_dev_mode", String(v)); } catch {}
  };

  const logout = useCallback(async () => {
    setLoggingOut(true);
    ws?.close();
    await deleteToken().catch(() => {});
    localStorage.clear();
    setUser(null as unknown as typeof user);
    router.replace("/login");
  }, [router, ws, setUser]);

  const categories: CategoryProps[] = [
    {
      href: "/settings/privacy",
      icon: <Shield size={16} strokeWidth={2} />,
      iconBg: "color-mix(in srgb, var(--em) 8%, transparent)",
      iconColor: "var(--accent)",
      label: "Gizlilik ve Güvenlik",
      desc: "Kim sizi görebilir, aramalar, mesaj koruması",
    },
    {
      href: "/settings/notifications",
      icon: <Bell size={16} strokeWidth={2} />,
      iconBg: "rgba(255,170,0,0.08)",
      iconColor: "#ffaa00",
      label: "Bildirimler",
      desc: "Ses, önizleme ve uyarı ayarları",
    },
    {
      href: "/settings/appearance",
      icon: <Palette size={16} strokeWidth={2} />,
      iconBg: "rgba(77,168,255,0.08)",
      iconColor: "var(--signal)",
      label: "Görünüm",
      desc: "Tema ve sohbet düzeni",
    },
    {
      href: "/events",
      icon: <Calendar size={16} strokeWidth={2} />,
      iconBg: "rgba(251,146,60,0.08)",
      iconColor: "#fb923c",
      label: "Etkinlikler",
      desc: "Topluluk etkinlikleri ve duyurular",
    },
    {
      href: "/settings/about",
      icon: <Info size={16} strokeWidth={2} />,
      iconBg: "rgba(255,255,255,0.04)",
      iconColor: "var(--text-2)",
      label: "Hakkında",
      desc: "Sürüm ve lisans",
      last: true,
    },
  ];

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">Ayarlar</h1>
          <StatusPill />
        </div>

        {/* ── User Card ──────────────────────────────────────────────────────── */}
        <div
          className="mx-3 mt-4 mb-1 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-1)",
            borderRadius: "24px",
          }}
        >
          {/* Identity row */}
          <div className="flex items-center gap-4 px-4 pt-5 pb-4">
            <Link href="/settings/profile" className="relative flex-shrink-0 group" aria-label="Profili düzenle">
              <Avatar
                name={user?.display_name || user?.username || "?"}
                size="lg"
                tier={user?.tier}
                avatarUrl={(user as unknown as { avatar_url?: string })?.avatar_url}
              />
              <div
                className="absolute inset-0 rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity duration-150"
                style={{ background: "rgba(0,0,0,0.55)" }}
                aria-hidden="true"
              >
                <svg width="18" height="18" fill="none" stroke="white" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" viewBox="0 0 24 24">
                  <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/>
                </svg>
              </div>
            </Link>

            <div className="flex-1 min-w-0">
              {user ? (
                <>
                  <p
                    className="text-[17px] font-bold truncate mb-0.5"
                    style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.02em" }}
                  >
                    {user.display_name || user.username}
                  </p>
                  {user.username && (
                    <p className="text-xs" style={{ color: "var(--text-3)" }}>@{user.username}</p>
                  )}
                  {user.phone && (
                    <p className="text-xs" style={{ color: "var(--text-3)" }}>{maskPhone(user.phone)}</p>
                  )}
                </>
              ) : (
                <div className="flex flex-col gap-1.5">
                  <div className="h-4 w-28 rounded shimmer" />
                  <div className="h-3 w-20 rounded shimmer" />
                </div>
              )}
            </div>

            <Link
              href="/settings/profile"
              className="btn-ghost text-xs px-3 h-8 rounded-xl flex-shrink-0"
              aria-label="Profili düzenle"
            >
              Düzenle
            </Link>
          </div>

        </div>

        {/* ── Basic Settings ─────────────────────────────────────────────────── */}
        <div className="px-4 pt-5 pb-2">
          <span className="section-label">Ayarlar</span>
        </div>

        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-1)",
            borderRadius: "20px",
          }}
        >
          {categories.map((cat, i) => (
            <CategoryRow key={cat.href} {...cat} last={i === categories.length - 1} />
          ))}
        </div>

        {/* ── Gelişmiş bölüm ─────────────────────────────────────────────────── */}
        <div className="px-4 pt-1 pb-2">
          <span className="section-label">Gelişmiş</span>
        </div>

        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-1)",
            borderRadius: "20px",
          }}
        >
          {/* Labs */}
          <div style={{ borderBottom: "1px solid var(--border-1)" }}>
            <Link
              href="/settings/labs"
              className="flex items-center gap-3.5 px-4 py-3.5 transition-colors duration-100 hover:bg-white/[0.025] active:bg-white/[0.04] focus-visible:outline-none focus-visible:bg-white/[0.03]"
            >
              <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
                style={{ background: "rgba(139,92,246,0.1)", color: "#a78bfa" }}>
                <FlaskConical size={16} strokeWidth={2} />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold leading-tight" style={{ color: "var(--text-1)" }}>Atış Alanı</p>
                <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>Deneysel özellikler</p>
              </div>
              <span className="badge badge-warning text-[10px] mr-1 flex-shrink-0">BETA</span>
              <ChevronRight size={14} className="flex-shrink-0" style={{ color: "var(--text-3)" }} />
            </Link>
          </div>

          {/* Developer mode toggle */}
          <DevToggle value={devMode} onChange={toggleDevMode} />
        </div>

        {/* ── Developer section (only when dev mode is ON) ───────────────────── */}
        {devMode && <DevSection />}

        {/* ── Danger Zone ───────────────────────────────────────────────────── */}
        <div className="mx-3 mb-2">
          <span className="section-label px-1">Tehlikeli Bölge</span>
        </div>

        <div
          className="mx-3 mb-3 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-1)",
            borderRadius: "20px",
          }}
        >
          {/* Logout */}
          {!showLogout ? (
            <div style={{ borderBottom: "1px solid var(--border-1)" }}>
              <button
                onClick={() => setShowLogout(true)}
                className="w-full flex items-center gap-3.5 px-4 py-3.5 transition-colors duration-100 hover:bg-red-500/[0.04] focus-visible:outline-none focus-visible:bg-red-500/[0.04]"
              >
                <div
                  className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
                  style={{ background: "rgba(255,64,88,0.08)", color: "var(--error)" }}
                >
                  <LogOut size={16} />
                </div>
                <div className="flex-1 text-left">
                  <p className="text-sm font-semibold" style={{ color: "var(--error)" }}>Oturumu Kapat</p>
                  <p className="text-xs" style={{ color: "rgba(255,64,88,0.5)" }}>Bu cihazdan çıkış yap</p>
                </div>
                <ChevronRight size={14} style={{ color: "rgba(255,64,88,0.3)" }} />
              </button>
            </div>
          ) : (
            <div className="px-4 py-4 animate-fade-in" style={{ borderBottom: "1px solid var(--border-1)" }}>
              <div className="flex items-start gap-3 mb-4">
                <AlertCircle size={17} className="flex-shrink-0 mt-0.5" style={{ color: "var(--error)" }} />
                <div>
                  <p className="text-sm font-semibold mb-1" style={{ color: "var(--text-1)" }}>
                    Oturumu kapatmak istediğinizden emin misiniz?
                  </p>
                  <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
                    Yerel şifreleme anahtarlarınız cihazda kalır. Tekrar giriş yapabilirsiniz.
                  </p>
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => setShowLogout(false)} className="btn-secondary flex-1 h-10 text-sm">
                  İptal
                </button>
                <button
                  onClick={logout}
                  disabled={loggingOut}
                  className="flex-1 h-10 rounded-full text-sm font-semibold flex items-center justify-center gap-2 transition-all duration-150 active:scale-[0.97] disabled:opacity-50"
                  style={{ background: "var(--error)", color: "white" }}
                >
                  {loggingOut ? <Loader2 size={14} className="animate-spin" /> : null}
                  Oturumu Kapat
                </button>
              </div>
            </div>
          )}

          {/* Delete Account */}
          <button
            onClick={() => setShowDelete(true)}
            className="w-full flex items-center gap-3.5 px-4 py-3.5 transition-colors duration-100 hover:bg-red-500/[0.04] focus-visible:outline-none focus-visible:bg-red-500/[0.04]"
          >
            <div
              className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(255,64,88,0.06)", color: "rgba(255,64,88,0.7)" }}
            >
              <Trash2 size={16} />
            </div>
            <div className="flex-1 text-left">
              <p className="text-sm font-semibold" style={{ color: "rgba(255,64,88,0.8)" }}>Hesabı Sil</p>
              <p className="text-xs" style={{ color: "rgba(255,64,88,0.4)" }}>
                Tüm veriler kalıcı olarak silinir
              </p>
            </div>
          </button>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>

      {showDelete && <DeleteAccountModal onClose={() => setShowDelete(false)} />}
    </AppShell>
  );
}
