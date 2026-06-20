"use client";

import { useState, useCallback } from "react";
import {
  Eye, EyeOff, Phone, Users, MessageSquare, Shield, ShieldOff,
  PhoneCall, UserPlus, Fingerprint, Lock, RefreshCw, Ban,
  Radio, Wifi, Clock, Trash2, MonitorOff, ScanEye,
  ChevronRight, FileText,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";

// ── i18n ─────────────────────────────────────────────────────────────────────

const TEXTS = {
  tr: {
    title: "Gizlilik ve Güvenlik",
    back: "Ayarlar",
    savedLocal: "Kaydedildi (yerel)",
    sections: {
      identity: "Kimlik Gizliliği",
      communication: "İletişim Kontrolü",
      security: "Hesap Güvenliği",
      messaging: "Mesaj Güvenliği",
      zk: "Gelişmiş Güvenlik",
      spam: "Spam Koruması",
      network: "Ağ Gizliliği",
    },
    visibility: {
      everyone: "Herkes",
      contacts: "Kişilerim",
      nobody: "Hiç Kimse",
      required: "Onayım Gerekli",
    },
    phone: "Telefon Görünürlüğü",
      phoneDesc: "Telefon numaranı kim görebilir",
    findByPhone: "Numarayla Bulunma",
      findByPhoneDesc: "Telefonla aranabilirliğin",
    lastSeen: "Son Görülme",
      lastSeenDesc: "Son çevrimiçi zamanı göster",
    profilePhoto: "Profil Fotoğrafı",
      profilePhotoDesc: "Avatarını kim görebilir",
    bioVisibility: "Bio Görünürlüğü",
      bioVisibilityDesc: "Hakkımda kısmını kim görebilir",
    whoCanCall: "Arama",
      whoCanCallDesc: "Kim arayabilir",
    whoCanGroup: "Gruba Ekleme",
      whoCanGroupDesc: "Kim bir gruba ekleyebilir",
    forwardVisibility: "Yönlendirme",
      forwardVisibilityDesc: "İletilen mesajlarda hesabı gizle",
    twoFactor: "İki Aşamalı Doğrulama",
      twoFactorDesc: "OTP ile ek giriş güvenliği",
    activeSessions: "Aktif Oturumlar",
      activeSessionsDesc: "Bağlı cihazları görüntüle ve yönet",
    autoDelete: "Hesap Silme Zamanlayıcısı",
      autoDeleteDesc: "Aktif olmazsam hesabı sil",
    loginNotif: "Giriş Bildirimleri",
      loginNotifDesc: "Yeni giriş yapıldığında bildir",
    e2eeEnforced: "Zorunlu E2EE",
      e2eeEnforcedDesc: "Tüm sohbetlerde uçtan uca şifreleme",
    msgAutoDelete: "Varsayılan Mesaj Silme",
      msgAutoDeleteDesc: "Yeni sohbetlerde otomatik silme",
    screenshotBlock: "Ekran Görüntüsü Engeli",
      screenshotBlockDesc: "Gizli sohbetlerde ekran görüntüsünü engelle",
    zkProof: "Gelişmiş Kimlik Doğrulama",
      zkProofDesc: "Kimliğinizi kanıtlanabilir şekilde doğrulayın",
    sealedSender: "Gönderen Gizleme",
      sealedSenderDesc: "Mesajlarda gönderen bilginizi gizle",
    zkCredit: "Güven Puanı Paylaşımı",
      zkCreditDesc: "Güven puanınızı anonim olarak kanıtlayın",
    zkAudit: "Güvenlik Günlüğü",
    autoBlock: "Otomatik Engelleme",
      autoBlockDesc: "Spam raporu sonrası otomatik engelle",
    blockedUsers: "Engellenen Kullanıcılar",
      blockedUsersDesc: "Engellenen kişileri görüntüle",
    callRelay: "Güvenli Arama Yönlendirmesi",
      callRelayDesc: "Aramalarda konumunuzu gizleyin",
    ipHide: "Bağlantı Gizleme",
      ipHideDesc: "Ağ bağlantınızı anonim tutun",
    manage: "Yönet",
    setup: "Ayarla",
    autoDeleteOptions: ["Kapalı", "1 Gün", "1 Hafta", "1 Ay"],
    msgDeleteOptions: ["Kapalı", "24 Saat", "7 Gün", "30 Gün"],
  },
};

type TLocale = keyof typeof TEXTS;
const locale: TLocale = "tr";
const t = TEXTS[locale];

// ── Persistence helpers ───────────────────────────────────────────────────────

function useSetting<T>(key: string, initial: T): [T, (v: T) => void] {
  const storageKey = `obscura_settings_privacy_${key}`;
  const [value, setValue] = useState<T>(() => {
    if (typeof window === "undefined") return initial;
    try {
      const stored = localStorage.getItem(storageKey);
      return stored !== null ? JSON.parse(stored) : initial;
    } catch {
      return initial;
    }
  });
  const set = useCallback((v: T) => {
    setValue(v);
    try { localStorage.setItem(storageKey, JSON.stringify(v)); } catch {}
  }, [storageKey]);
  return [value, set];
}

// ── Sub-components ────────────────────────────────────────────────────────────

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-4 pt-5 pb-2">
      <span className="section-label">{children}</span>
    </div>
  );
}

function SettingCard({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="mx-3 mb-2 overflow-hidden"
      style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", borderRadius: "20px" }}
    >
      {children}
    </div>
  );
}

// Toggle row
function ToggleRow({
  icon, iconBg, iconColor, label, desc, value, onChange, last,
}: {
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc?: string;
  value: boolean;
  onChange: (v: boolean) => void;
  last?: boolean;
}) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <button
        onClick={() => onChange(!value)}
        className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none focus-visible:bg-white/[0.02]"
        role="switch"
        aria-checked={value}
        aria-label={label}
      >
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-tight" style={{ color: "var(--text-1)" }}>{label}</p>
          {desc && <p className="text-xs mt-0.5 leading-snug" style={{ color: "var(--text-3)" }}>{desc}</p>}
        </div>
        {/* Toggle pill */}
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
    </div>
  );
}

// Visibility selector row
function VisibilityRow({
  icon, iconBg, iconColor, label, desc, value, options, onChange, last,
}: {
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc?: string;
  value: string;
  options: string[];
  onChange: (v: string) => void;
  last?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none focus-visible:bg-white/[0.02]"
        aria-expanded={open}
        aria-label={label}
      >
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-tight" style={{ color: "var(--text-1)" }}>{label}</p>
          {desc && <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>{desc}</p>}
        </div>
        <span className="text-xs font-semibold flex-shrink-0 mr-1" style={{ color: "var(--accent)" }}>{value}</span>
        <ChevronRight
          size={14}
          className="flex-shrink-0"
          style={{ color: "var(--text-3)", transform: open ? "rotate(90deg)" : "rotate(0deg)", transition: "transform 150ms" }}
        />
      </button>
      {open && (
        <div className="pb-2 px-4 animate-slide-down flex gap-2 flex-wrap" style={{ borderTop: "1px solid var(--border-1)" }}>
          <div className="pt-2 flex gap-2 flex-wrap w-full">
            {options.map((opt) => (
              <button
                key={opt}
                onClick={() => { onChange(opt); setOpen(false); }}
                className={cn(
                  "h-8 px-3.5 rounded-xl text-xs font-semibold transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                )}
                style={{
                  background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
                  color: value === opt ? "var(--accent)" : "var(--text-2)",
                  border: value === opt ? "1px solid rgba(0,229,160,0.25)" : "1px solid transparent",
                }}
                aria-pressed={value === opt}
              >
                {opt}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Link row
function LinkRow({
  icon, iconBg, iconColor, label, desc, linkLabel, href, last,
}: {
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc?: string;
  linkLabel: string;
  href?: string;
  last?: boolean;
}) {
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <div className="flex items-center gap-3.5 px-4 py-3.5">
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-tight" style={{ color: "var(--text-1)" }}>{label}</p>
          {desc && <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>{desc}</p>}
        </div>
        <button
          className="text-xs font-semibold px-3 h-7 rounded-xl flex-shrink-0 transition-colors duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          style={{ background: "var(--surface-3)", color: "var(--text-2)", border: "1px solid var(--border-2)" }}
          onClick={() => href && (window.location.href = href)}
        >
          {linkLabel}
        </button>
      </div>
    </div>
  );
}

// Select row (dropdown chips for auto-delete options, etc.)
function SelectRow({
  icon, iconBg, iconColor, label, desc, value, options, onChange, last,
}: {
  icon: React.ReactNode;
  iconBg: string;
  iconColor: string;
  label: string;
  desc?: string;
  value: string;
  options: string[];
  onChange: (v: string) => void;
  last?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={!last ? { borderBottom: "1px solid var(--border-1)" } : undefined}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-colors duration-100 hover:bg-white/[0.02] focus-visible:outline-none focus-visible:bg-white/[0.02]"
        aria-expanded={open}
        aria-label={label}
      >
        <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0" style={{ background: iconBg, color: iconColor }}>
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium leading-tight" style={{ color: "var(--text-1)" }}>{label}</p>
          {desc && <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>{desc}</p>}
        </div>
        <span className="text-xs font-semibold flex-shrink-0 mr-1" style={{ color: "var(--text-2)" }}>{value}</span>
        <ChevronRight
          size={14}
          className="flex-shrink-0"
          style={{ color: "var(--text-3)", transform: open ? "rotate(90deg)" : "rotate(0deg)", transition: "transform 150ms" }}
        />
      </button>
      {open && (
        <div className="pb-3 px-4 flex flex-wrap gap-2 animate-slide-down" style={{ borderTop: "1px solid var(--border-1)" }}>
          <div className="pt-2 flex gap-2 flex-wrap w-full">
            {options.map((opt) => (
              <button
                key={opt}
                onClick={() => { onChange(opt); setOpen(false); }}
                className={cn(
                  "h-8 px-3.5 rounded-xl text-xs font-semibold transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                )}
                style={{
                  background: value === opt ? "var(--accent-muted)" : "var(--surface-3)",
                  color: value === opt ? "var(--accent)" : "var(--text-2)",
                  border: value === opt ? "1px solid rgba(0,229,160,0.25)" : "1px solid transparent",
                }}
                aria-pressed={value === opt}
              >
                {opt}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function PrivacyPage() {
  const vis = t.visibility;

  // Kimlik
  const [phoneVis, setPhoneVis] = useSetting("phoneVis", vis.contacts);
  const [findByPhone, setFindByPhone] = useSetting("findByPhone", vis.contacts);
  const [lastSeenVis, setLastSeenVis] = useSetting("lastSeenVis", vis.contacts);
  const [photoVis, setPhotoVis] = useSetting("photoVis", vis.everyone);
  const [bioVis, setBioVis] = useSetting("bioVis", vis.everyone);

  // İletişim
  const [callVis, setCallVis] = useSetting("callVis", vis.contacts);
  const [groupAdd, setGroupAdd] = useSetting("groupAdd", vis.contacts);
  const [forwardHide, setForwardHide] = useSetting("forwardHide", true);

  // Güvenlik
  const [twoFactor, setTwoFactor] = useSetting("twoFactor", false);
  const [autoDeleteAccount, setAutoDeleteAccount] = useSetting("autoDeleteAccount", t.autoDeleteOptions[0]);
  const [loginNotif, setLoginNotif] = useSetting("loginNotif", true);

  // Mesaj
  const [e2eeEnforced, setE2eeEnforced] = useSetting("e2eeEnforced", true);
  const [msgAutoDelete, setMsgAutoDelete] = useSetting("msgAutoDelete", t.msgDeleteOptions[0]);
  const [screenshotBlock, setScreenshotBlock] = useSetting("screenshotBlock", true);

  // ZK
  const [zkProof, setZkProof] = useSetting("zkProof", true);
  const [sealedSender, setSealedSender] = useSetting("sealedSender", false);
  const [zkCredit, setZkCredit] = useSetting("zkCredit", false);

  // Spam
  const [autoBlock, setAutoBlock] = useSetting("autoBlock", true);

  // Ağ
  const [callRelay, setCallRelay] = useSetting("callRelay", true);
  const [ipHide, setIpHide] = useSetting("ipHide", false);

  const visOptions = [vis.everyone, vis.contacts, vis.nobody];
  const callOptions = [vis.everyone, vis.contacts, vis.nobody];
  const groupOptions = [vis.everyone, vis.contacts, vis.required];

  return (
    <AppShell showBack title={t.back}>
      <div className="flex flex-col h-full scroll-area">

        {/* Page Header */}
        <div className="page-header flex-shrink-0 px-5">
          <h1 className="page-title">{t.title}</h1>
        </div>

        {/* Notice */}
        <div
          className="mx-3 mt-4 mb-2 px-4 py-3 rounded-2xl flex items-start gap-2.5"
          style={{ background: "rgba(77,168,255,0.06)", border: "1px solid rgba(77,168,255,0.12)" }}
        >
          <Shield size={14} className="flex-shrink-0 mt-0.5" style={{ color: "var(--signal)" }} />
          <p className="text-xs leading-relaxed" style={{ color: "var(--text-3)" }}>
            Gizlilik ayarları yerel olarak kaydedilir. Sunucu senkronizasyonu yakında aktif olacak.
          </p>
        </div>

        {/* ── Kimlik Gizliliği ────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.identity}</SectionHeader>
        <SettingCard>
          <VisibilityRow icon={<Phone size={15}/>} iconBg="rgba(77,168,255,0.08)" iconColor="var(--signal)" label={t.phone} desc={t.phoneDesc} value={phoneVis} options={visOptions} onChange={setPhoneVis} />
          <VisibilityRow icon={<Eye size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.findByPhone} desc={t.findByPhoneDesc} value={findByPhone} options={visOptions} onChange={setFindByPhone} />
          <VisibilityRow icon={<Clock size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.lastSeen} desc={t.lastSeenDesc} value={lastSeenVis} options={visOptions} onChange={setLastSeenVis} />
          <VisibilityRow icon={<Users size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.profilePhoto} desc={t.profilePhotoDesc} value={photoVis} options={visOptions} onChange={setPhotoVis} />
          <VisibilityRow icon={<FileText size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.bioVisibility} desc={t.bioVisibilityDesc} value={bioVis} options={visOptions} onChange={setBioVis} last />
        </SettingCard>

        {/* ── İletişim Kontrolü ───────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.communication}</SectionHeader>
        <SettingCard>
          <VisibilityRow icon={<PhoneCall size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.whoCanCall} desc={t.whoCanCallDesc} value={callVis} options={callOptions} onChange={setCallVis} />
          <VisibilityRow icon={<UserPlus size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.whoCanGroup} desc={t.whoCanGroupDesc} value={groupAdd} options={groupOptions} onChange={setGroupAdd} />
          <ToggleRow icon={<EyeOff size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.forwardVisibility} desc={t.forwardVisibilityDesc} value={forwardHide} onChange={setForwardHide} last />
        </SettingCard>

        {/* ── Hesap Güvenliği ─────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.security}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Lock size={15}/>} iconBg="rgba(255,170,0,0.08)" iconColor="#ffaa00" label={t.twoFactor} desc={t.twoFactorDesc} value={twoFactor} onChange={setTwoFactor} />
          <LinkRow icon={<Fingerprint size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.activeSessions} desc={t.activeSessionsDesc} linkLabel={t.manage} href="/settings/sessions" />
          <SelectRow icon={<Trash2 size={15}/>} iconBg="rgba(255,64,88,0.06)" iconColor="rgba(255,64,88,0.7)" label={t.autoDelete} desc={t.autoDeleteDesc} value={autoDeleteAccount} options={t.autoDeleteOptions} onChange={setAutoDeleteAccount} />
          <ToggleRow icon={<Shield size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.loginNotif} desc={t.loginNotifDesc} value={loginNotif} onChange={setLoginNotif} last />
        </SettingCard>

        {/* ── Mesaj Güvenliği ─────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.messaging}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<MessageSquare size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.e2eeEnforced} desc={t.e2eeEnforcedDesc} value={e2eeEnforced} onChange={setE2eeEnforced} />
          <SelectRow icon={<Clock size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.msgAutoDelete} desc={t.msgAutoDeleteDesc} value={msgAutoDelete} options={t.msgDeleteOptions} onChange={setMsgAutoDelete} />
          <ToggleRow icon={<MonitorOff size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.screenshotBlock} desc={t.screenshotBlockDesc} value={screenshotBlock} onChange={setScreenshotBlock} last />
        </SettingCard>

        {/* ── ZK Güvenlik ─────────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.zk}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<ScanEye size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.zkProof} desc={t.zkProofDesc} value={zkProof} onChange={setZkProof} />
          <ToggleRow icon={<ShieldOff size={15}/>} iconBg="rgba(77,168,255,0.08)" iconColor="var(--signal)" label={t.sealedSender} desc={t.sealedSenderDesc} value={sealedSender} onChange={setSealedSender} />
          <ToggleRow icon={<Fingerprint size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.zkCredit} desc={t.zkCreditDesc} value={zkCredit} onChange={setZkCredit} />
          <LinkRow icon={<RefreshCw size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.zkAudit} linkLabel="Görüntüle" href="/settings/zk-audit" last />
        </SettingCard>

        {/* ── Spam Koruması ───────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.spam}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Ban size={15}/>} iconBg="rgba(255,170,0,0.08)" iconColor="#ffaa00" label={t.autoBlock} desc={t.autoBlockDesc} value={autoBlock} onChange={setAutoBlock} />
          <LinkRow icon={<Users size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.blockedUsers} desc={t.blockedUsersDesc} linkLabel={t.manage} last />
        </SettingCard>

        {/* ── Ağ Gizliliği ────────────────────────────────────────────────────── */}
        <SectionHeader>{t.sections.network}</SectionHeader>
        <SettingCard>
          <ToggleRow icon={<Radio size={15}/>} iconBg="rgba(0,229,160,0.08)" iconColor="var(--accent)" label={t.callRelay} desc={t.callRelayDesc} value={callRelay} onChange={setCallRelay} />
          <ToggleRow icon={<Wifi size={15}/>} iconBg="var(--surface-3)" iconColor="var(--text-3)" label={t.ipHide} desc={t.ipHideDesc} value={ipHide} onChange={setIpHide} last />
        </SettingCard>

        <div className="h-32 flex-shrink-0" />
      </div>
    </AppShell>
  );
}
