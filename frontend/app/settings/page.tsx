"use client";

import { useState, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import {
  ChevronRight, LogOut, Shield, Key, Eye,
  Bell, Smartphone, Info, Copy, Check,
  User, Lock, Fingerprint, Database, Trash2,
  ShieldCheck, AlertCircle, ChevronDown,
  Camera, Loader2, X, Globe, Layers,
  BookKey, Cpu, Zap, ScanLine, Languages,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { tierLabel, tierColor, maskPhone } from "@/lib/format";
import { api } from "@/lib/api";
import { deleteToken, getAppVersion } from "@/lib/tauri";

// ── Setting Group ─────────────────────────────────────────────────────────────
function SettingGroup({
  title,
  children,
}: {
  title?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="mb-3">
      {title && (
        <div className="px-5 pb-2 pt-1">
          <span className="section-label">{title}</span>
        </div>
      )}
      <div
        className="mx-3 overflow-hidden"
        style={{
          background: "var(--surface-2)",
          border: "1px solid var(--border-1)",
          borderRadius: "20px",
        }}
      >
        {children}
      </div>
    </div>
  );
}

// ── Setting Row ───────────────────────────────────────────────────────────────
interface RowProps {
  icon: React.ReactNode;
  iconBg?: string;
  iconColor?: string;
  label: string;
  sublabel?: string;
  value?: React.ReactNode;
  chevron?: boolean;
  danger?: boolean;
  onClick?: () => void;
  toggle?: { on: boolean; onToggle: () => void };
  last?: boolean;
  disabled?: boolean;
}

function Row({
  icon,
  iconBg,
  iconColor,
  label,
  sublabel,
  value,
  chevron,
  danger,
  onClick,
  toggle,
  last,
  disabled,
}: RowProps) {
  const isClickable = !!(onClick || toggle);

  return (
    <div
      className="relative"
      style={
        !last
          ? { borderBottom: "1px solid var(--border-1)" }
          : undefined
      }
    >
      <button
        onClick={disabled ? undefined : (onClick ?? toggle?.onToggle)}
        disabled={disabled}
        className={cn(
          "input-group-row w-full text-left transition-colors duration-100",
          isClickable && "hover:bg-white/[0.025] active:bg-white/[0.04] cursor-pointer",
          !isClickable && "cursor-default",
          disabled && "opacity-40 cursor-not-allowed"
        )}
      >
        {/* Icon box */}
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
          style={{
            background: iconBg ?? "var(--surface-3)",
            color: iconColor ?? "var(--text-3)",
          }}
        >
          {icon}
        </div>

        {/* Text */}
        <div className="flex-1 min-w-0">
          <p
            className="text-sm font-medium"
            style={{ color: danger ? "var(--error)" : "var(--text-1)" }}
          >
            {label}
          </p>
          {sublabel && (
            <p className="text-xs mt-0.5 truncate" style={{ color: "var(--text-3)" }}>
              {sublabel}
            </p>
          )}
        </div>

        {/* Right side */}
        {toggle && (
          <div
            className="relative w-11 h-6 rounded-full transition-all duration-200 flex-shrink-0"
            style={{
              background: toggle.on ? "var(--accent)" : "var(--surface-3)",
              border: toggle.on ? "none" : "1px solid var(--border-2)",
            }}
            role="switch"
            aria-checked={toggle.on}
          >
            <div
              className="absolute top-0.5 w-5 h-5 rounded-full transition-all duration-200"
              style={{
                background: "white",
                boxShadow: "0 1px 4px rgba(0,0,0,0.4)",
                left: toggle.on ? "22px" : "2px",
              }}
            />
          </div>
        )}
        {value && !toggle && (
          <span className="text-xs flex-shrink-0 ml-2" style={{ color: "var(--text-3)" }}>
            {value}
          </span>
        )}
        {chevron && (
          <ChevronRight size={14} className="flex-shrink-0 ml-1" style={{ color: "var(--text-3)" }} />
        )}
      </button>
    </div>
  );
}

// ── Copy DID Button ───────────────────────────────────────────────────────────
function CopyDID({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };

  return (
    <button
      onClick={copy}
      className="flex items-center gap-1.5 transition-colors duration-100"
      style={{ color: copied ? "var(--accent)" : "var(--text-3)" }}
      aria-label="DID kopyala"
    >
      <span className="font-mono text-[11px]">
        {value.length > 22 ? `${value.slice(0, 12)}…${value.slice(-8)}` : value}
      </span>
      {copied
        ? <Check size={12} style={{ color: "var(--accent)" }} />
        : <Copy size={11} />
      }
    </button>
  );
}

// ── Edit Profile Modal ────────────────────────────────────────────────────────
function EditProfileModal({ onClose }: { onClose: () => void }) {
  const { user, setUser } = useStore();
  const [displayName, setDisplayName] = useState(user?.display_name || "");
  const [username, setUsername] = useState(user?.username || "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const save = async () => {
    if (!displayName.trim() && !username.trim()) return;
    setSaving(true);
    setError("");
    try {
      const updated = await api.updateMe({
        display_name: displayName.trim() || undefined,
        username: username.trim() || undefined,
      });
      setUser(updated);
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Kaydetme başarısız");
    } finally {
      setSaving(false);
    }
  };

  const handleAvatarChange = async (file: File) => {
    setUploading(true);
    try {
      const result = await api.uploadMedia(file, "avatar");
      const updated = await api.updateMe({ avatar_url: result.url });
      setUser(updated);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Yükleme başarısız");
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end" role="dialog" aria-modal="true" aria-label="Profili düzenle">
      <div
        className="absolute inset-0 bg-black/60"
        style={{ backdropFilter: "blur(8px)" }}
        onClick={onClose}
      />
      <div
        className="relative w-full px-5 pt-6 pb-10 animate-slide-up"
        style={{
          background: "var(--surface)",
          borderTop: "1px solid var(--border-2)",
          borderRadius: "28px 28px 0 0",
        }}
      >
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <h2
            className="text-lg font-bold"
            style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.02em" }}
          >
            Profili Düzenle
          </h2>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full flex items-center justify-center"
            style={{ background: "var(--surface-3)", color: "var(--text-2)" }}
            aria-label="Kapat"
          >
            <X size={15} />
          </button>
        </div>

        {/* Avatar picker */}
        <div className="flex flex-col items-center mb-6">
          <div className="relative">
            <Avatar
              name={user?.display_name || user?.username || "?"}
              size="xl"
              tier={user?.tier}
              avatarUrl={(user as unknown as { avatar_url?: string })?.avatar_url}
            />
            <button
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
              className="absolute bottom-0 right-0 w-8 h-8 rounded-full flex items-center justify-center transition-all duration-150 active:scale-90"
              style={{ background: "var(--accent)", color: "var(--void)", boxShadow: "0 2px 8px rgba(0,0,0,0.4)" }}
              aria-label="Fotoğraf değiştir"
            >
              {uploading
                ? <Loader2 size={14} className="animate-spin" />
                : <Camera size={14} />
              }
            </button>
            <input
              ref={fileRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={(e) => e.target.files?.[0] && handleAvatarChange(e.target.files[0])}
              aria-hidden="true"
            />
          </div>
          <p className="text-xs mt-2" style={{ color: "var(--text-3)" }}>
            JPEG, PNG veya WebP · Maks 5MB
          </p>
        </div>

        {/* Fields */}
        <div className="space-y-4">
          <div>
            <label
              className="block text-xs font-semibold mb-1.5"
              style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}
            >
              Görünen Ad
            </label>
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Adınız"
              maxLength={50}
              className="field"
            />
          </div>
          <div>
            <label
              className="block text-xs font-semibold mb-1.5"
              style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}
            >
              Kullanıcı Adı
            </label>
            <div className="relative">
              <span
                className="absolute left-4 top-1/2 -translate-y-1/2 text-sm font-medium"
                style={{ color: "var(--text-3)" }}
              >
                @
              </span>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
                placeholder="kullaniciadi"
                maxLength={32}
                className="field pl-8"
              />
            </div>
          </div>

          {error && (
            <p
              className="text-xs rounded-xl px-3 py-2.5"
              style={{ background: "rgba(255,64,88,0.08)", color: "var(--error)", border: "1px solid rgba(255,64,88,0.15)" }}
            >
              {error}
            </p>
          )}
        </div>

        {/* Save */}
        <button
          onClick={save}
          disabled={saving}
          className="btn-primary w-full mt-6"
        >
          {saving ? (
            <>
              <Loader2 size={16} className="animate-spin" />
              Kaydediliyor...
            </>
          ) : (
            "Kaydet"
          )}
        </button>
      </div>
    </div>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────
export default function SettingsPage() {
  const router = useRouter();
  const { user, setUser } = useStore();

  const [readReceipts, setReadReceipts] = useState(true);
  const [onlineStatus, setOnlineStatus] = useState(true);
  const [notifications, setNotifications] = useState(true);
  const [notifSound, setNotifSound] = useState(true);
  const [showDID, setShowDID] = useState(false);
  const [showLogoutConfirm, setShowLogoutConfirm] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [appVersion, setAppVersion] = useState("1.0.0");

  // Uygulama sürümünü al
  useState(() => {
    getAppVersion().then(setAppVersion).catch(() => {});
  });

  const logout = useCallback(async () => {
    await deleteToken().catch(() => {});
    localStorage.clear();
    setUser(null as unknown as typeof user);
    router.replace("/login");
  }, [router, setUser]);

  const tierName = user ? tierLabel(user.tier) : "—";
  const tierCls = user ? tierColor(user.tier) : "text-sub";

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">

        {/* ── Page Header ───────────────────────────────────────────────── */}
        <div className="page-header flex-shrink-0 px-4">
          <h1 className="page-title">Ayarlar</h1>
        </div>

        {/* ── Profile Card ──────────────────────────────────────────────── */}
        <div
          className="mx-3 mb-5 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-1)",
            borderRadius: "24px",
          }}
        >
          {/* Identity row */}
          <div className="flex items-center gap-4 px-4 pt-5 pb-4">
            {/* Avatar — 64px */}
            <div className="relative flex-shrink-0">
              <div className="w-16 h-16">
                <Avatar
                  name={user?.display_name || user?.username || "?"}
                  size="lg"
                  tier={user?.tier}
                  avatarUrl={(user as unknown as { avatar_url?: string })?.avatar_url}
                />
              </div>
              {user?.credit_score !== undefined && (
                <div
                  className="absolute -bottom-1 -right-1 w-6 h-6 rounded-full flex items-center justify-center"
                  style={{
                    background: "var(--surface-2)",
                    border: "1px solid var(--border-2)",
                  }}
                >
                  <span
                    className="text-[9px] font-bold"
                    style={{ fontFamily: "var(--font-display)", color: "var(--accent)" }}
                  >
                    {Math.round(user.credit_score)}
                  </span>
                </div>
              )}
            </div>

            {/* Name & DID */}
            <div className="flex-1 min-w-0">
              <p
                className="text-[17px] font-bold truncate mb-0.5"
                style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.02em" }}
              >
                {user?.display_name || user?.username || "Yükleniyor..."}
              </p>
              {user?.username && (
                <p className="text-sm" style={{ color: "var(--text-3)" }}>
                  @{user.username}
                </p>
              )}
              <div className="flex items-center gap-2 mt-1">
                <span
                  className={cn("text-xs font-semibold", tierCls)}
                  style={{ fontFamily: "var(--font-display)" }}
                >
                  {tierName}
                </span>
                {user?.credit_score !== undefined && (
                  <>
                    <span style={{ color: "var(--text-3)" }}>·</span>
                    <span className="text-xs" style={{ color: "var(--text-3)" }}>
                      {user.credit_score.toFixed(0)} puan
                    </span>
                  </>
                )}
              </div>
            </div>

            <button
              onClick={() => setEditOpen(true)}
              className="btn-ghost text-xs px-3 h-8 rounded-xl flex-shrink-0"
            >
              Düzenle
            </button>
          </div>

          {/* DID accordion */}
          <div style={{ borderTop: "1px solid var(--border-1)" }}>
            <button
              onClick={() => setShowDID((v) => !v)}
              className="w-full flex items-center justify-between px-4 py-3 transition-colors duration-100 hover:bg-white/[0.02]"
              aria-expanded={showDID}
            >
              <div className="flex items-center gap-2">
                <Fingerprint size={13} style={{ color: "var(--text-3)" }} />
                <span className="text-xs font-medium" style={{ color: "var(--text-3)" }}>
                  Kimlik parmak izi
                </span>
              </div>
              <ChevronDown
                size={13}
                style={{
                  color: "var(--text-3)",
                  transform: showDID ? "rotate(180deg)" : "rotate(0deg)",
                  transition: "transform 200ms",
                }}
              />
            </button>

            {showDID && (
              <div
                className="px-4 pb-4 animate-slide-down"
                style={{ borderTop: "1px solid var(--border-1)" }}
              >
                <p
                  className="font-mono text-[11px] tracking-wider break-all leading-relaxed mt-3"
                  style={{ color: "var(--text-2)" }}
                >
                  {user?.did || "—"}
                </p>
                <div className="mt-2 flex items-center justify-end">
                  <CopyDID value={user?.did || "—"} />
                </div>
              </div>
            )}
          </div>
        </div>

        {/* ── Hesap ─────────────────────────────────────────────────────── */}
        <SettingGroup title="Hesap">
          <Row
            icon={<User size={15} />}
            iconBg="rgba(77,168,255,0.1)"
            iconColor="var(--signal)"
            label="Profil Düzenle"
            sublabel="Ad, kullanıcı adı, fotoğraf"
            chevron
            onClick={() => setEditOpen(true)}
          />
          <Row
            icon={<Smartphone size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Telefon"
            value={user?.phone ? maskPhone(user.phone) : "—"}
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<Globe size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="E-posta"
            sublabel="Kurtarma için isteğe bağlı"
            chevron
            onClick={() => {}}
            last
          />
        </SettingGroup>

        {/* ── Güvenlik ──────────────────────────────────────────────────── */}
        <SettingGroup title="Güvenlik">
          <Row
            icon={<Smartphone size={15} />}
            iconBg="rgba(0,229,160,0.08)"
            iconColor="var(--accent)"
            label="Cihazlar"
            sublabel="Aktif oturumlar ve cihaz yönetimi"
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<ScanLine size={15} />}
            iconBg="rgba(0,229,160,0.08)"
            iconColor="var(--accent)"
            label="Cross-signing"
            sublabel="QR kod ile çok cihazlı doğrulama"
            chevron
            onClick={() => router.push("/settings/cross-signing")}
          />
          <Row
            icon={<Lock size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Şifre Değiştir"
            chevron
            onClick={() => {}}
            last
          />
        </SettingGroup>

        {/* ── Kripto ────────────────────────────────────────────────────── */}
        <SettingGroup title="Kripto">
          <Row
            icon={<ShieldCheck size={15} />}
            iconBg="rgba(0,229,160,0.08)"
            iconColor="var(--accent)"
            label="ZK Kanıtları"
            sublabel="Sıfır bilgi kanıtı yönetimi"
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<Layers size={15} />}
            iconBg="rgba(77,168,255,0.1)"
            iconColor="var(--signal)"
            label="MLS Grubu"
            sublabel="RFC 9420 grup şifreleme durumu"
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<BookKey size={15} />}
            iconBg="rgba(255,170,0,0.08)"
            iconColor="#ffaa00"
            label="Mnemonic Yedekle"
            sublabel="12 kelime BIP39 yedek ifadesi"
            chevron
            onClick={() => {}}
            last
          />
        </SettingGroup>

        {/* ── Gizlilik ──────────────────────────────────────────────────── */}
        <SettingGroup title="Gizlilik">
          <Row
            icon={<Check size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Okundu Bildirimleri"
            sublabel="Mesaj okunduğunda karşı tarafa bildir"
            toggle={{ on: readReceipts, onToggle: () => setReadReceipts((v) => !v) }}
          />
          <Row
            icon={<Eye size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Çevrimiçi Durumu"
            sublabel="Bağlantılarınıza görünür olun"
            toggle={{ on: onlineStatus, onToggle: () => setOnlineStatus((v) => !v) }}
            last
          />
        </SettingGroup>

        {/* ── Bildirimler ───────────────────────────────────────────────── */}
        <SettingGroup title="Bildirimler">
          <Row
            icon={<Bell size={15} />}
            iconBg="rgba(255,170,0,0.08)"
            iconColor="#ffaa00"
            label="Push Bildirimleri"
            sublabel="Mesaj ve arama bildirimleri"
            toggle={{ on: notifications, onToggle: () => setNotifications((v) => !v) }}
          />
          <Row
            icon={<Cpu size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Bildirim Sesi"
            sublabel="Yeni mesaj geldiğinde ses çal"
            toggle={{ on: notifSound, onToggle: () => setNotifSound((v) => !v) }}
            last
          />
        </SettingGroup>

        {/* ── Uygulama ──────────────────────────────────────────────────── */}
        <SettingGroup title="Uygulama">
          <Row
            icon={<Languages size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Dil"
            value="Türkçe"
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<Zap size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Tema"
            value="Void Cipher"
            chevron
            onClick={() => {}}
          />
          <Row
            icon={<Info size={15} />}
            iconBg="var(--surface-3)"
            iconColor="var(--text-3)"
            label="Hakkında"
            value={`v${appVersion}`}
            chevron
            onClick={() => {}}
            last
          />
        </SettingGroup>

        {/* ── Tehlikeli Bölge ───────────────────────────────────────────── */}
        <SettingGroup title="Tehlikeli Bölge">
          <Row
            icon={<Trash2 size={15} />}
            iconBg="rgba(255,64,88,0.10)"
            iconColor="var(--error)"
            label="Hesabı Sil"
            sublabel="Tüm veriler kalıcı olarak silinir"
            danger
            chevron
            onClick={() => {}}
            last
          />
        </SettingGroup>

        {/* ── Logout ───────────────────────────────────────────────────── */}
        <div
          className="mx-3 mb-3 mt-1 overflow-hidden"
          style={{
            background: "var(--surface-2)",
            border: "1px solid rgba(255,64,88,0.12)",
            borderRadius: "20px",
          }}
        >
          {!showLogoutConfirm ? (
            <button
              onClick={() => setShowLogoutConfirm(true)}
              className="w-full flex items-center gap-3 px-4 py-4 transition-colors duration-100 hover:bg-red-500/5"
            >
              <div
                className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
                style={{ background: "rgba(255,64,88,0.10)", color: "var(--error)" }}
              >
                <LogOut size={15} />
              </div>
              <span className="text-sm font-semibold flex-1 text-left" style={{ color: "var(--error)" }}>
                Çıkış Yap
              </span>
              <ChevronRight size={14} style={{ color: "rgba(255,64,88,0.4)" }} />
            </button>
          ) : (
            <div className="px-4 py-4 animate-fade-in">
              <div className="flex items-start gap-3 mb-4">
                <AlertCircle size={17} className="flex-shrink-0 mt-0.5" style={{ color: "var(--error)" }} />
                <div>
                  <p className="text-sm font-semibold mb-1" style={{ color: "var(--text-1)" }}>
                    Çıkış yapmak istediğinizden emin misiniz?
                  </p>
                  <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
                    Yerel şifreleme anahtarlarınız cihazda kalır. Tekrar giriş yapabilirsiniz.
                  </p>
                </div>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => setShowLogoutConfirm(false)}
                  className="btn-secondary flex-1 h-10 text-sm"
                  aria-label="İptal"
                >
                  İptal
                </button>
                <button
                  onClick={logout}
                  className="flex-1 h-10 rounded-full text-sm font-semibold transition-all duration-150 active:scale-[0.97]"
                  style={{ background: "var(--error)", color: "white" }}
                  aria-label="Çıkış yap"
                >
                  Çıkış Yap
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>

      {/* Edit Profile Modal */}
      {editOpen && <EditProfileModal onClose={() => setEditOpen(false)} />}
    </AppShell>
  );
}
