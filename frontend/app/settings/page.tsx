"use client";

import { useState, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import {
  ChevronRight, LogOut, Shield, Key, Eye,
  Bell, Smartphone, Info, Copy, Check,
  User, Lock, Fingerprint, Database, Trash2,
  ShieldCheck, AlertCircle, ChevronDown,
  Camera, Loader2, X,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { tierLabel, tierColor, maskPhone } from "@/lib/format";
import { api } from "@/lib/api";
import { deleteToken, getAppVersion } from "@/lib/tauri";

/* ── Section ──────────────────────────────────────────────────────── */
function Section({ title, children }: { title?: string; children: React.ReactNode }) {
  return (
    <div className="mb-2">
      {title && (
        <p className="px-5 py-2 text-[11px] font-semibold text-dim uppercase tracking-widest">
          {title}
        </p>
      )}
      <div className="mx-3 rounded-3xl border border-border overflow-hidden bg-surface">
        {children}
      </div>
    </div>
  );
}

/* ── Row ──────────────────────────────────────────────────────────── */
interface RowProps {
  icon: React.ReactNode;
  iconColor?: string;
  label: string;
  sublabel?: string;
  value?: React.ReactNode;
  chevron?: boolean;
  danger?: boolean;
  onClick?: () => void;
  toggle?: { on: boolean; onToggle: () => void };
  divider?: boolean;
  disabled?: boolean;
}

function Row({ icon, iconColor, label, sublabel, value, chevron, danger, onClick, toggle, divider = true, disabled }: RowProps) {
  return (
    <button
      onClick={disabled ? undefined : (onClick ?? toggle?.onToggle)}
      disabled={disabled}
      className={cn(
        "w-full flex items-center gap-3 px-4 py-3.5 text-left",
        "transition-colors duration-150 relative",
        onClick || toggle ? "hover:bg-raised active:bg-muted cursor-pointer" : "cursor-default",
        disabled && "opacity-40 cursor-not-allowed"
      )}
    >
      <div className={cn("w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0", iconColor ?? "bg-muted text-sub")}>
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className={cn("text-sm font-medium", danger ? "text-red" : "text-body")}>{label}</p>
        {sublabel && <p className="text-xs text-dim mt-0.5 truncate">{sublabel}</p>}
      </div>
      {toggle && (
        <div className={cn("relative w-11 h-6 rounded-full transition-all duration-200 flex-shrink-0", toggle.on ? "bg-accent" : "bg-muted border border-border")}>
          <div className={cn("absolute top-0.5 w-5 h-5 rounded-full bg-white shadow-void-sm transition-all duration-200", toggle.on ? "left-5" : "left-0.5")} />
        </div>
      )}
      {value && !toggle && <span className="text-xs text-dim flex-shrink-0 ml-2">{value}</span>}
      {chevron && <ChevronRight size={14} className="text-dim flex-shrink-0 ml-1" />}
      {divider && <span className="absolute bottom-0 left-16 right-0 h-px bg-border/60" />}
    </button>
  );
}

/* ── Copy badge ───────────────────────────────────────────────────── */
function CopyValue({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(value).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };
  return (
    <button onClick={copy} className="flex items-center gap-1.5 text-dim hover:text-sub transition-colors text-xs">
      <span className="font-mono text-[11px]">
        {value.length > 20 ? `${value.slice(0, 10)}…${value.slice(-8)}` : value}
      </span>
      {copied ? <Check size={12} className="text-accent" /> : <Copy size={11} />}
    </button>
  );
}

/* ── Profil Düzenleme Modal ───────────────────────────────────────── */
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
    } catch (e: any) {
      setError(e.message);
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
    } catch (e: any) {
      setError(e.message);
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div className="relative w-full bg-surface rounded-t-3xl border-t border-border px-5 pt-5 pb-10 animate-slide-up">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-head">Profili Düzenle</h2>
          <button onClick={onClose} className="w-8 h-8 rounded-full bg-muted flex items-center justify-center">
            <X size={16} className="text-sub" />
          </button>
        </div>

        {/* Avatar */}
        <div className="flex flex-col items-center mb-6">
          <div className="relative">
            <Avatar name={user?.display_name || user?.username || "?"} size="xl" tier={user?.tier} avatarUrl={user?.avatar_url} />
            <button
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
              className="absolute bottom-0 right-0 w-8 h-8 rounded-full bg-accent flex items-center justify-center shadow-void"
            >
              {uploading ? <Loader2 size={14} className="animate-spin text-white" /> : <Camera size={14} className="text-white" />}
            </button>
            <input
              ref={fileRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={(e) => e.target.files?.[0] && handleAvatarChange(e.target.files[0])}
            />
          </div>
          <p className="text-xs text-dim mt-2">JPEG, PNG veya WebP • Maks 5MB</p>
        </div>

        {/* Form */}
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-dim mb-1.5">Görünen Ad</label>
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Adınız"
              maxLength={50}
              className="w-full bg-raised border border-border rounded-2xl px-4 py-3 text-sm text-body placeholder:text-muted outline-none focus:border-accent transition-colors"
            />
          </div>
          <div>
            <label className="block text-xs text-dim mb-1.5">Kullanıcı Adı</label>
            <div className="relative">
              <span className="absolute left-4 top-1/2 -translate-y-1/2 text-dim text-sm">@</span>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
                placeholder="kullaniciadi"
                maxLength={32}
                className="w-full bg-raised border border-border rounded-2xl px-4 pl-8 py-3 text-sm text-body placeholder:text-muted outline-none focus:border-accent transition-colors"
              />
            </div>
          </div>
          {error && <p className="text-xs text-red bg-red/10 rounded-xl px-3 py-2">{error}</p>}
        </div>

        {/* Save */}
        <button
          onClick={save}
          disabled={saving}
          className="mt-6 w-full h-12 rounded-2xl bg-accent text-white font-medium text-sm flex items-center justify-center gap-2 disabled:opacity-50"
        >
          {saving ? <><Loader2 size={16} className="animate-spin" /> Kaydediliyor...</> : "Kaydet"}
        </button>
      </div>
    </div>
  );
}

/* ── Main Page ────────────────────────────────────────────────────── */
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
    setUser(null as any);
    router.replace("/login");
  }, [router, setUser]);

  const fingerprint = user?.did
    ? user.did.replace("did:obs:", "").toUpperCase().match(/.{1,4}/g)?.join(" ") ?? "—"
    : "—";

  const tierName = user ? tierLabel(user.tier) : "—";
  const tierCls = user ? tierColor(user.tier) : "text-sub";

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">
        {/* Header */}
        <div className="px-5 pt-5 pb-4 flex-shrink-0">
          <h1 className="text-2xl font-semibold text-head">Ayarlar</h1>
        </div>

        {/* Profile Card */}
        <div className="mx-3 mb-2 rounded-3xl border border-border bg-surface overflow-hidden">
          <div className="px-4 pt-5 pb-4 flex items-center gap-4">
            <div className="relative">
              <Avatar
                name={user?.display_name || user?.username || "?"}
                size="lg"
                tier={user?.tier}
                avatarUrl={(user as any)?.avatar_url}
              />
              {user?.credit_score !== undefined && (
                <div className="absolute -bottom-1 -right-1 w-6 h-6 rounded-full bg-surface border border-border flex items-center justify-center">
                  <span className="text-[9px] font-bold text-accent">{Math.round(user.credit_score)}</span>
                </div>
              )}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-head font-semibold text-base truncate">
                {user?.display_name || user?.username || "Yükleniyor..."}
              </p>
              {user?.username && <p className="text-dim text-sm">@{user.username}</p>}
              <div className="flex items-center gap-2 mt-1">
                <span className={cn("text-xs font-medium", tierCls)}>{tierName}</span>
                {user?.credit_score !== undefined && (
                  <>
                    <span className="text-dim text-xs">·</span>
                    <span className="text-dim text-xs">{user.credit_score.toFixed(0)} puan</span>
                  </>
                )}
              </div>
            </div>
            <button onClick={() => setEditOpen(true)} className="btn-ghost text-xs px-3 h-8 rounded-xl">
              Düzenle
            </button>
          </div>

          {/* DID / Fingerprint */}
          <div className="border-t border-border/60 px-4 py-3">
            <button onClick={() => setShowDID((v) => !v)} className="w-full flex items-center justify-between text-left">
              <div className="flex items-center gap-2">
                <Fingerprint size={14} className="text-dim" />
                <span className="text-xs text-dim">Kimlik parmak izi</span>
              </div>
              <ChevronDown size={13} className={cn("text-dim transition-transform duration-200", showDID && "rotate-180")} />
            </button>
            {showDID && (
              <div className="mt-2 pt-2 border-t border-border/40 animate-fade-in">
                <p className="font-mono text-[11px] text-sub tracking-wider break-all leading-relaxed">{user?.did || "—"}</p>
                <div className="mt-2 flex items-center justify-between">
                  <span className="text-[10px] text-dim">Parmak izi: {fingerprint}</span>
                  <CopyValue value={user?.did || "—"} />
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Güvenlik */}
        <Section title="Güvenlik">
          <Row icon={<ShieldCheck size={16} />} iconColor="bg-accent/15 text-accent" label="Uçtan Uca Şifreleme"
            sublabel="Signal protokolü aktif (X3DH + Double Ratchet)"
            value={<span className="text-accent text-xs font-medium">Aktif ✓</span>} divider />
          <Row icon={<Key size={16} />} iconColor="bg-blue-900/40 text-tier2" label="Şifreleme Anahtarları"
            sublabel="İmzalı PreKey'ler ve tek kullanımlık anahtarlar"
            chevron onClick={() => router.push("/keys")} divider />
          <Row icon={<Lock size={16} />} iconColor="bg-purple-900/40 text-tier3" label="ZK Kimlik Kanıtı"
            sublabel="Sıfır bilgi kanıtı ile anonim doğrulama"
            chevron onClick={() => {}} divider={false} />
        </Section>

        {/* Gizlilik */}
        <Section title="Gizlilik">
          <Row icon={<Check size={16} />} iconColor="bg-muted text-sub" label="Okundu Bildirimleri"
            sublabel="Mesajlarınızı okuduğunuzda karşı tarafa bildirilir"
            toggle={{ on: readReceipts, onToggle: () => setReadReceipts((v) => !v) }} divider />
          <Row icon={<Eye size={16} />} iconColor="bg-muted text-sub" label="Çevrimiçi Durumu"
            sublabel="Bağlandığınızda bağlantı listenize görünür"
            toggle={{ on: onlineStatus, onToggle: () => setOnlineStatus((v) => !v) }} divider />
          <Row icon={<User size={16} />} iconColor="bg-muted text-sub" label="Son Görülme"
            sublabel="Kimler ne zaman aktif olduğunuzu görebilir"
            value="Bağlantılar" chevron onClick={() => {}} divider={false} />
        </Section>

        {/* Bildirimler */}
        <Section title="Bildirimler">
          <Row icon={<Bell size={16} />} iconColor="bg-amber/15 text-amber" label="Push Bildirimleri"
            sublabel="Yeni mesaj ve arama bildirimleri"
            toggle={{ on: notifications, onToggle: () => setNotifications((v) => !v) }} divider />
          <Row icon={<Bell size={16} />} iconColor="bg-muted text-sub" label="Bildirim Sesi"
            sublabel="Mesaj geldiğinde ses çıkar"
            toggle={{ on: notifSound, onToggle: () => setNotifSound((v) => !v) }} divider={false} />
        </Section>

        {/* Depolama */}
        <Section title="Depolama & Veri">
          <Row icon={<Database size={16} />} iconColor="bg-muted text-sub" label="Yerel Depolama"
            sublabel="Şifreli anahtar deposu (localStorage)"
            value="~0.1 MB" divider />
          <Row icon={<Smartphone size={16} />} iconColor="bg-muted text-sub" label="Oturum Yönetimi"
            sublabel="Aktif cihaz ve oturumlarınız"
            chevron onClick={() => {}} divider={false} />
        </Section>

        {/* Kredi */}
        <Section title="Kredi & Tier">
          <Row icon={<ShieldCheck size={16} />} iconColor="bg-accent/15 text-accent" label="Kredi Puanım"
            sublabel="Ağ güven puanınız ve tier seviyeniz"
            value={<span className={cn("text-xs font-medium", tierCls)}>{user?.credit_score?.toFixed(0) ?? "—"} / 100</span>}
            chevron onClick={() => router.push("/credit")} divider={false} />
        </Section>

        {/* Hakkında */}
        <Section title="Hakkında">
          <Row icon={<Info size={16} />} iconColor="bg-muted text-sub" label="Sürüm"
            value={`Obscura v${appVersion}`} divider />
          <Row icon={<Shield size={16} />} iconColor="bg-muted text-sub" label="Gizlilik Politikası"
            chevron onClick={() => {}} divider />
          <Row icon={<Info size={16} />} iconColor="bg-muted text-sub" label="Açık Kaynak Lisanslar"
            chevron onClick={() => {}} divider={false} />
        </Section>

        {/* Çıkış */}
        <div className="mx-3 mb-2 mt-2 rounded-3xl border border-red/20 overflow-hidden bg-surface">
          {!showLogoutConfirm ? (
            <button onClick={() => setShowLogoutConfirm(true)}
              className="w-full flex items-center gap-3 px-4 py-4 hover:bg-red/5 transition-colors">
              <div className="w-9 h-9 rounded-xl bg-red/15 flex items-center justify-center">
                <LogOut size={16} className="text-red" />
              </div>
              <span className="text-sm font-medium text-red flex-1 text-left">Çıkış Yap</span>
            </button>
          ) : (
            <div className="px-4 py-4 animate-fade-in">
              <div className="flex items-start gap-3 mb-4">
                <AlertCircle size={18} className="text-red flex-shrink-0 mt-0.5" />
                <div>
                  <p className="text-sm font-medium text-head">Çıkış yapmak istediğinizden emin misiniz?</p>
                  <p className="text-xs text-dim mt-1">
                    Yerel şifreleme anahtarlarınız cihazda kalır. Tekrar giriş yapabilirsiniz.
                  </p>
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => setShowLogoutConfirm(false)}
                  className="flex-1 h-10 rounded-2xl border border-border text-sub text-sm hover:text-body transition-colors">
                  İptal
                </button>
                <button onClick={logout}
                  className="flex-1 h-10 rounded-2xl bg-red text-white text-sm font-medium hover:bg-red/90 transition-colors">
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
