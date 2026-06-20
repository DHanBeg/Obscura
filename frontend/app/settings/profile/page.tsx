"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Camera, Loader2, Check, Copy, ShieldCheck,
  KeyRound, Clock, CalendarDays, Phone, AtSign, FileText, User,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { useStore } from "@/lib/store";
import { AppShell } from "@/components/AppShell";
import { Avatar } from "@/components/ui/Avatar";
import { maskPhone } from "@/lib/format";
import { api } from "@/lib/api";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Profil",
    back: "Ayarlar",
    displayName: "Görünen Ad",
    username: "Kullanıcı Adı",
    bio: "Hakkımda",
    bioPlaceholder: "Kısa bir tanıtım yazısı…",
    bioCounter: (n: number) => `${n}/70`,
    status: "Profil Durumu",
    statuses: ["Çevrimiçi", "Rahatsız Etme", "Gizli"],
    readonlySection: "Hesap Bilgileri",
    did: "DID",
    copyDid: "DID kopyala",
    phone: "Telefon",
    accountAge: "Hesap Yaşı",
    createdAt: "Oluşturulma",
    zkSection: "ZK Kimlik",
    zkVerified: "ZK-ID Doğrulandı",
    zkNotVerified: "ZK-ID Doğrulanmadı",
    zkVerify: "Doğrula",
    dilithium: "Dilithium3 İmza",
    dilithiumActive: "Etkin",
    dilithiumInactive: "Devre Dışı",
    save: "Değişiklikleri Kaydet",
    saving: "Kaydediliyor…",
    saved: "Kaydedildi",
    avatarHint: "JPEG, PNG veya WebP · Maks 5MB",
    uploadErr: "Yükleme başarısız",
    saveErr: "Kaydetme başarısız",
    days: "gün",
  },
  en: {
    title: "Profile",
    back: "Settings",
    displayName: "Display Name",
    username: "Username",
    bio: "About",
    bioPlaceholder: "A short introduction…",
    bioCounter: (n: number) => `${n}/70`,
    status: "Profile Status",
    statuses: ["Online", "Do Not Disturb", "Hidden"],
    readonlySection: "Account Info",
    did: "DID",
    copyDid: "Copy DID",
    phone: "Phone",
    accountAge: "Account Age",
    createdAt: "Created",
    zkSection: "ZK Identity",
    zkVerified: "ZK-ID Verified",
    zkNotVerified: "ZK-ID Not Verified",
    zkVerify: "Verify",
    dilithium: "Dilithium3 Signature",
    dilithiumActive: "Active",
    dilithiumInactive: "Inactive",
    save: "Save Changes",
    saving: "Saving…",
    saved: "Saved",
    avatarHint: "JPEG, PNG or WebP · Max 5MB",
    uploadErr: "Upload failed",
    saveErr: "Save failed",
    days: "days",
  },
};

// ── Status indicator ──────────────────────────────────────────────────────────

const STATUS_COLORS = ["#00e5a0", "#ff4058", "var(--text-3)"];
const STATUS_BG = [
  "rgba(0,229,160,0.1)",
  "rgba(255,64,88,0.1)",
  "rgba(255,255,255,0.04)",
];

// ── ReadOnly row ──────────────────────────────────────────────────────────────

function InfoRow({
  icon,
  label,
  value,
  mono,
  action,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  mono?: boolean;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-3 px-4 py-3.5" style={{ borderBottom: "1px solid var(--border-1)" }}>
      <div
        className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
        style={{ background: "var(--surface-3)", color: "var(--text-3)" }}
      >
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-[11px] font-semibold mb-0.5" style={{ color: "var(--text-3)", letterSpacing: "0.05em", textTransform: "uppercase" }}>
          {label}
        </p>
        <p
          className={cn("text-sm truncate", mono && "font-mono text-[12px] tracking-wide")}
          style={{ color: "var(--text-2)" }}
        >
          {value}
        </p>
      </div>
      {action}
    </div>
  );
}

// ── Label ─────────────────────────────────────────────────────────────────────

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label
      className="block text-[11px] font-semibold mb-1.5"
      style={{ color: "var(--text-3)", letterSpacing: "0.06em", textTransform: "uppercase" }}
    >
      {children}
    </label>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function ProfilePage() {
  const router = useRouter();
  const { user, setUser } = useStore();

  const locale = "tr";
  const t = TEXTS[locale];

  const [displayName, setDisplayName] = useState(user?.display_name || "");
  const [username, setUsername] = useState(user?.username || "");
  const [bio, setBio] = useState("");
  const [statusIdx, setStatusIdx] = useState(0);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);
  const [copiedDid, setCopiedDid] = useState(false);

  const fileRef = useRef<HTMLInputElement>(null);

  // Sync user changes to form
  useEffect(() => {
    if (user) {
      setDisplayName(user.display_name || "");
      setUsername(user.username || "");
    }
  }, [user]);

  const copyDid = () => {
    if (!user?.did) return;
    navigator.clipboard.writeText(user.did).catch(() => {});
    setCopiedDid(true);
    setTimeout(() => setCopiedDid(false), 1800);
  };

  const handleAvatarChange = async (file: File) => {
    setUploading(true);
    setError("");
    try {
      const result = await api.uploadMedia(file, "avatar");
      const updated = await api.updateMe({ avatar_url: result.url });
      setUser(updated);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t.uploadErr);
    } finally {
      setUploading(false);
    }
  };

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      const updated = await api.updateMe({
        display_name: displayName.trim() || undefined,
        username: username.trim() || undefined,
      });
      setUser(updated);
      setSaved(true);
      setTimeout(() => setSaved(false), 2200);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t.saveErr);
    } finally {
      setSaving(false);
    }
  };

  const accountAge = user
    ? Math.floor((Date.now() - new Date(2024, 0, 1).getTime()) / 86400000)
    : 0;

  // Dirty check
  const isDirty =
    displayName !== (user?.display_name || "") ||
    username !== (user?.username || "");

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        {/* ── Avatar area ────────────────────────────────────────────────────── */}
        <div className="flex flex-col items-center pt-8 pb-6">
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
              className="absolute bottom-0 right-0 w-9 h-9 rounded-full flex items-center justify-center transition-all duration-150 active:scale-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
              style={{ background: "var(--accent)", color: "#020208", boxShadow: "0 2px 10px rgba(0,0,0,0.5)" }}
              aria-label={uploading ? t.saving : "Fotoğraf değiştir"}
            >
              {uploading
                ? <Loader2 size={15} className="animate-spin" />
                : <Camera size={15} />
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
          <p className="text-xs mt-3" style={{ color: "var(--text-3)" }}>{t.avatarHint}</p>
        </div>

        {/* ── Editable fields ─────────────────────────────────────────────────── */}
        <div className="px-3 mb-4">
          <div
            className="overflow-hidden"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
          >
            {/* Display Name */}
            <div className="px-4 pt-4 pb-3" style={{ borderBottom: "1px solid var(--border-1)" }}>
              <FieldLabel>{t.displayName}</FieldLabel>
              <div className="relative">
                <User size={14} className="absolute left-3.5 top-1/2 -translate-y-1/2" style={{ color: "var(--text-3)" }} />
                <input
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Adınız"
                  maxLength={50}
                  className="field pl-9"
                  aria-label={t.displayName}
                />
              </div>
            </div>

            {/* Username */}
            <div className="px-4 pt-3 pb-3" style={{ borderBottom: "1px solid var(--border-1)" }}>
              <FieldLabel>{t.username}</FieldLabel>
              <div className="relative">
                <AtSign size={14} className="absolute left-3.5 top-1/2 -translate-y-1/2" style={{ color: "var(--text-3)" }} />
                <input
                  value={username}
                  onChange={(e) => setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
                  placeholder="kullaniciadi"
                  maxLength={32}
                  className="field pl-9"
                  aria-label={t.username}
                  autoCapitalize="none"
                  autoCorrect="off"
                />
              </div>
            </div>

            {/* Bio */}
            <div className="px-4 pt-3 pb-3" style={{ borderBottom: "1px solid var(--border-1)" }}>
              <div className="flex items-center justify-between mb-1.5">
                <FieldLabel>{t.bio}</FieldLabel>
                <span className="text-[10px]" style={{ color: bio.length > 60 ? "var(--warning)" : "var(--text-3)" }}>
                  {t.bioCounter(bio.length)}
                </span>
              </div>
              <div className="relative">
                <FileText size={14} className="absolute left-3.5 top-3.5" style={{ color: "var(--text-3)" }} />
                <textarea
                  value={bio}
                  onChange={(e) => setBio(e.target.value.slice(0, 70))}
                  placeholder={t.bioPlaceholder}
                  rows={3}
                  className="field pl-9 resize-none leading-relaxed"
                  aria-label={t.bio}
                />
              </div>
            </div>

            {/* Status */}
            <div className="px-4 pt-3 pb-4">
              <FieldLabel>{t.status}</FieldLabel>
              <div className="flex gap-2">
                {t.statuses.map((s, i) => (
                  <button
                    key={i}
                    onClick={() => setStatusIdx(i)}
                    className={cn(
                      "flex-1 h-9 rounded-xl text-xs font-semibold flex items-center justify-center gap-1.5 transition-all duration-150",
                      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                    )}
                    style={{
                      background: statusIdx === i ? STATUS_BG[i] : "var(--surface-3)",
                      border: statusIdx === i ? `1px solid ${STATUS_COLORS[i]}33` : "1px solid transparent",
                      color: statusIdx === i ? STATUS_COLORS[i] : "var(--text-3)",
                    }}
                    aria-pressed={statusIdx === i}
                  >
                    <span
                      className="w-1.5 h-1.5 rounded-full"
                      style={{ background: STATUS_COLORS[i] }}
                    />
                    {s}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* ── Read-only info ──────────────────────────────────────────────────── */}
        <div className="px-3 mb-1">
          <span className="section-label px-1">{t.readonlySection}</span>
        </div>
        <div
          className="mx-3 mb-4 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          <InfoRow
            icon={<span className="font-mono text-[9px] font-bold tracking-widest" style={{ color: "var(--text-3)" }}>DID</span>}
            label={t.did}
            value={user?.did ? `${user.did.slice(0, 18)}…` : "—"}
            mono
            action={
              <button
                onClick={copyDid}
                className="w-8 h-8 rounded-xl flex items-center justify-center transition-colors duration-100 hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--accent)]"
                style={{ color: copiedDid ? "var(--accent)" : "var(--text-3)" }}
                aria-label={t.copyDid}
              >
                {copiedDid ? <Check size={13} /> : <Copy size={13} />}
              </button>
            }
          />
          {user?.phone && (
            <InfoRow
              icon={<Phone size={14} />}
              label={t.phone}
              value={maskPhone(user.phone)}
            />
          )}
          <InfoRow
            icon={<Clock size={14} />}
            label={t.accountAge}
            value={`${accountAge} ${t.days}`}
          />
          <InfoRow
            icon={<CalendarDays size={14} />}
            label={t.createdAt}
            value="Oca 2024"
          />
        </div>

        {/* ── ZK Identity ────────────────────────────────────────────────────── */}
        <div className="px-3 mb-1">
          <span className="section-label px-1">{t.zkSection}</span>
        </div>
        <div
          className="mx-3 mb-6 overflow-hidden"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
        >
          {/* ZK-ID */}
          <div className="flex items-center gap-3 px-4 py-3.5" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <div
              className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(0,229,160,0.08)", color: "var(--accent)" }}
            >
              <ShieldCheck size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>
                {t.zkVerified}
              </p>
              <p className="text-xs" style={{ color: "var(--accent)" }}>
                Aktif · Groth16/BN254
              </p>
            </div>
            <span className="badge badge-success text-[10px]">
              <Check size={9} />
              Aktif
            </span>
          </div>

          {/* Dilithium */}
          <div className="flex items-center gap-3 px-4 py-3.5">
            <div
              className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
              style={{ background: "rgba(77,168,255,0.08)", color: "var(--signal)" }}
            >
              <KeyRound size={15} />
            </div>
            <div className="flex-1">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>
                {t.dilithium}
              </p>
              <p className="text-xs" style={{ color: "var(--signal)" }}>
                NIST ML-DSA mode3
              </p>
            </div>
            <span className="badge badge-signal text-[10px]">
              {t.dilithiumActive}
            </span>
          </div>
        </div>

        {/* Error */}
        {error && (
          <div
            className="mx-3 mb-4 px-4 py-3 rounded-2xl text-sm"
            style={{ background: "rgba(255,64,88,0.08)", color: "var(--error)", border: "1px solid rgba(255,64,88,0.15)" }}
          >
            {error}
          </div>
        )}

        {/* Save */}
        <div className="px-3 mb-4">
          <button
            onClick={save}
            disabled={saving || !isDirty}
            className="btn-primary w-full"
            aria-label={saving ? t.saving : t.save}
          >
            {saving ? (
              <><Loader2 size={16} className="animate-spin" />{t.saving}</>
            ) : saved ? (
              <><Check size={16} />{t.saved}</>
            ) : (
              t.save
            )}
          </button>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
