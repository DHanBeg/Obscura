"use client";

import { useState, useRef, useEffect } from "react";
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

const STATUS_COLORS = ["#00e5a0", "#ff4058", "var(--text-3)"];
const STATUS_BG = [
  "rgba(0,229,160,0.1)",
  "rgba(255,64,88,0.1)",
  "rgba(255,255,255,0.04)",
];
const STATUSES = ["Çevrimiçi", "Rahatsız Etme", "Gizli"];

/* ── Floating-label input row (Telegram-style) ── */
function EditRow({
  icon,
  label,
  last,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  last?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div
      className="flex items-start gap-4 px-5 py-4"
      style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}
    >
      <div className="mt-3 flex-shrink-0" style={{ color: "var(--accent)", width: 22 }}>
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p
          className="text-[11px] font-semibold mb-1"
          style={{ color: "var(--accent)", letterSpacing: "0.04em" }}
        >
          {label}
        </p>
        {children}
      </div>
    </div>
  );
}

/* ── Borderless input ── */
const inputCls = [
  "w-full bg-transparent border-none outline-none",
  "text-sm leading-snug",
  "placeholder:text-[var(--text-3)]",
].join(" ");

/* ── Read-only info row ── */
function InfoRow({
  icon, label, value, mono, action, last,
}: {
  icon: React.ReactNode; label: string; value: string;
  mono?: boolean; action?: React.ReactNode; last?: boolean;
}) {
  return (
    <div
      className="flex items-center gap-4 px-5 py-4"
      style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}
    >
      <div className="flex-shrink-0" style={{ color: "var(--text-3)", width: 22 }}>{icon}</div>
      <div className="flex-1 min-w-0">
        <p className="text-[11px] mb-0.5" style={{ color: "var(--text-3)" }}>{label}</p>
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

/* ── Section label ── */
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p
      className="px-5 pt-5 pb-1 text-[11px] font-semibold tracking-widest uppercase"
      style={{ color: "var(--text-3)" }}
    >
      {children}
    </p>
  );
}

/* ── Card wrapper ── */
function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div
      className={cn("mx-4 overflow-hidden", className)}
      style={{
        background: "var(--surface-2)",
        border: "1px solid var(--border-1)",
        borderRadius: "18px",
      }}
    >
      {children}
    </div>
  );
}

/* ══════════════════════════════════════════════════════════ */

export default function ProfilePage() {
  const { user, setUser } = useStore();

  const [displayName, setDisplayName] = useState(user?.display_name || "");
  const [username, setUsername]       = useState(user?.username || "");
  const [bio, setBio]                 = useState("");
  const [statusIdx, setStatusIdx]     = useState(0);
  const [saving, setSaving]           = useState(false);
  const [saved, setSaved]             = useState(false);
  const [error, setError]             = useState("");
  const [uploading, setUploading]     = useState(false);
  const [copiedDid, setCopiedDid]     = useState(false);

  const fileRef = useRef<HTMLInputElement>(null);

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
    setUploading(true); setError("");
    try {
      const result  = await api.uploadMedia(file, "avatar");
      const updated = await api.updateMe({ avatar_url: result.url });
      setUser(updated);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Yükleme başarısız");
    } finally { setUploading(false); }
  };

  const save = async () => {
    setSaving(true); setError("");
    try {
      const updated = await api.updateMe({
        display_name: displayName.trim() || undefined,
        username:     username.trim()     || undefined,
      });
      setUser(updated);
      setSaved(true);
      setTimeout(() => setSaved(false), 2200);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Kaydetme başarısız");
    } finally { setSaving(false); }
  };

  const accountAge = user
    ? Math.floor((Date.now() - new Date(2024, 0, 1).getTime()) / 86400000)
    : 0;

  const isDirty =
    displayName !== (user?.display_name || "") ||
    username    !== (user?.username     || "");

  const avatarUrl = (user as unknown as { avatar_url?: string })?.avatar_url;

  return (
    <AppShell showBack title="Ayarlar">
      <div className="flex flex-col h-full scroll-area">

        {/* ══ HERO ══ */}
        <div
          className="flex-shrink-0 flex flex-col items-center pt-10 pb-6 px-6"
          style={{
            background: "linear-gradient(180deg, rgba(0,229,160,0.08) 0%, transparent 100%)",
            borderBottom: "1px solid var(--border-1)",
          }}
        >
          {/* Avatar + camera */}
          <div className="relative mb-5">
            {/* Outer glow ring */}
            <div
              className="absolute inset-0 rounded-full"
              style={{ boxShadow: "0 0 0 2px rgba(0,229,160,0.25)" }}
            />
            <div className="w-[88px] h-[88px] rounded-full overflow-hidden">
              {avatarUrl ? (
                <img src={avatarUrl} alt="" className="w-full h-full object-cover" />
              ) : (
                <Avatar
                  name={user?.display_name || user?.username || "?"}
                  size="xl"
                  tier={user?.tier}
                />
              )}
            </div>

            {/* Camera button */}
            <button
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
              className="absolute -bottom-1 -right-1 w-8 h-8 rounded-full flex items-center justify-center transition-all duration-150 active:scale-90 focus-visible:outline-none"
              style={{
                background: "var(--accent)",
                color: "#020208",
                boxShadow: "0 0 0 2px var(--void), 0 2px 8px rgba(0,0,0,0.5)",
              }}
              aria-label={uploading ? "Yükleniyor…" : "Fotoğraf değiştir"}
            >
              {uploading
                ? <Loader2 size={13} className="animate-spin" />
                : <Camera size={13} />
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

          {/* Name / skeleton */}
          {user ? (
            <div className="text-center">
              <h2
                className="text-[22px] font-bold leading-tight"
                style={{ fontFamily: "var(--font-display)", color: "var(--text-1)", letterSpacing: "-0.03em" }}
              >
                {user.display_name || user.username || "—"}
              </h2>
              {user.username && (
                <p className="text-[13px] mt-0.5" style={{ color: "var(--accent)" }}>
                  @{user.username}
                </p>
              )}
              {user.phone && (
                <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
                  {maskPhone(user.phone)}
                </p>
              )}
            </div>
          ) : (
            <div className="flex flex-col items-center gap-2">
              <div className="h-6 w-36 rounded-lg shimmer" />
              <div className="h-4 w-24 rounded-lg shimmer" />
            </div>
          )}

          {/* Status badge */}
          <button
            onClick={() => setStatusIdx((i) => (i + 1) % 3)}
            className="flex items-center gap-1.5 mt-3 px-3 py-1 rounded-full transition-colors duration-150 hover:bg-white/[0.04]"
            style={{
              background: STATUS_BG[statusIdx],
              border: `1px solid ${STATUS_COLORS[statusIdx]}33`,
            }}
          >
            <span className="w-1.5 h-1.5 rounded-full" style={{ background: STATUS_COLORS[statusIdx] }} />
            <span className="text-[11px] font-semibold" style={{ color: STATUS_COLORS[statusIdx] }}>
              {STATUSES[statusIdx]}
            </span>
          </button>
        </div>

        {/* ══ EDIT FIELDS ══ */}
        <SectionLabel>Profil Bilgileri</SectionLabel>
        <Card className="mb-4">
          <EditRow icon={<User size={18} />} label="Görünen Ad">
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Adınız"
              maxLength={50}
              className={inputCls}
              style={{ color: "var(--text-1)" }}
              aria-label="Görünen Ad"
            />
          </EditRow>

          <EditRow icon={<AtSign size={18} />} label="Kullanıcı Adı">
            <div className="flex items-center">
              <span className="text-sm mr-0.5" style={{ color: "var(--text-3)" }}>@</span>
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, ""))}
                placeholder="kullaniciadi"
                maxLength={32}
                className={inputCls}
                style={{ color: "var(--text-1)" }}
                aria-label="Kullanıcı Adı"
                autoCapitalize="none"
                autoCorrect="off"
              />
            </div>
            <p className="text-[11px] mt-1" style={{ color: "var(--text-3)" }}>
              obscura.app/{username || "kullaniciadi"}
            </p>
          </EditRow>

          <EditRow icon={<FileText size={18} />} label="Hakkımda" last>
            <div className="relative">
              <textarea
                value={bio}
                onChange={(e) => setBio(e.target.value.slice(0, 70))}
                placeholder="Kendinizden kısaca bahsedin…"
                rows={2}
                className={cn(inputCls, "resize-none leading-relaxed")}
                style={{ color: "var(--text-1)" }}
                aria-label="Hakkımda"
              />
              <span
                className="absolute bottom-0 right-0 text-[10px]"
                style={{ color: bio.length > 60 ? "var(--warning)" : "var(--text-3)" }}
              >
                {bio.length}/70
              </span>
            </div>
          </EditRow>
        </Card>

        {/* ══ ACCOUNT INFO ══ */}
        <SectionLabel>Hesap</SectionLabel>
        <Card className="mb-4">
          {user?.phone && (
            <InfoRow icon={<Phone size={16} />} label="Telefon" value={maskPhone(user.phone)} />
          )}
          <InfoRow
            icon={<span className="font-mono text-[9px] font-black tracking-widest" style={{ color: "var(--text-3)" }}>DID</span>}
            label="Merkeziyetsiz Kimlik"
            value={user?.did ? `${user.did.slice(0, 20)}…` : "—"}
            mono
            action={
              <button
                onClick={copyDid}
                className="w-8 h-8 rounded-xl flex items-center justify-center hover:bg-white/[0.04] transition-colors"
                style={{ color: copiedDid ? "var(--accent)" : "var(--text-3)" }}
                aria-label="DID kopyala"
              >
                {copiedDid ? <Check size={13} /> : <Copy size={13} />}
              </button>
            }
          />
          <InfoRow icon={<Clock size={16} />}       label="Hesap Yaşı"   value={`${accountAge} gün`} />
          <InfoRow icon={<CalendarDays size={16} />} label="Oluşturulma" value="Oca 2024" last />
        </Card>

        {/* ══ ZK IDENTITY ══ */}
        <SectionLabel>Kriptografik Kimlik</SectionLabel>
        <Card className="mb-6">
          <div className="flex items-center gap-4 px-5 py-4" style={{ borderBottom: "1px solid var(--border-1)" }}>
            <ShieldCheck size={18} style={{ color: "var(--accent)", flexShrink: 0 }} />
            <div className="flex-1">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>ZK-ID Doğrulandı</p>
              <p className="text-[11px]" style={{ color: "var(--accent)" }}>Groth16 · BN254 eliptik eğrisi</p>
            </div>
            <span className="badge badge-success text-[10px]"><Check size={9} />Aktif</span>
          </div>
          <div className="flex items-center gap-4 px-5 py-4">
            <KeyRound size={18} style={{ color: "var(--signal)", flexShrink: 0 }} />
            <div className="flex-1">
              <p className="text-sm font-semibold" style={{ color: "var(--text-1)" }}>Dilithium3 İmza</p>
              <p className="text-[11px]" style={{ color: "var(--signal)" }}>NIST ML-DSA mode3 · Post-kuantum</p>
            </div>
            <span className="badge badge-signal text-[10px]">Etkin</span>
          </div>
        </Card>

        {/* Error */}
        {error && (
          <div
            className="mx-4 mb-4 px-4 py-3 rounded-2xl text-sm"
            style={{ background: "rgba(255,64,88,0.08)", color: "var(--error)", border: "1px solid rgba(255,64,88,0.15)" }}
          >
            {error}
          </div>
        )}

        {/* Save */}
        <div className="px-4 mb-4">
          <button
            onClick={save}
            disabled={saving || !isDirty}
            className="btn-primary w-full"
            style={{ opacity: saving || !isDirty ? 0.35 : 1, cursor: saving || !isDirty ? "not-allowed" : "pointer" }}
          >
            {saving ? (
              <><Loader2 size={16} className="animate-spin" />Kaydediliyor…</>
            ) : saved ? (
              <><Check size={16} />Kaydedildi</>
            ) : (
              "Değişiklikleri Kaydet"
            )}
          </button>
        </div>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
