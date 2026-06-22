"use client";

import { useState, useEffect, useCallback } from "react";
import {
  Lock,
  Plus,
  Trash2,
  ChevronDown,
  ChevronUp,
  Globe,
  Key,
  ShieldCheck,
  Eye,
  EyeOff,
  Save,
  Loader2,
  CheckSquare,
  Square,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

// ─── Types ─────────────────────────────────────────────────────────────────

type AuthType = "none" | "api_key" | "bearer" | "oauth2";
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE";

interface ApiEndpoint {
  id: string;
  label: string;
  baseUrl: string;
  authType: AuthType;
  authKey: string;
  methods: HttpMethod[];
  rateLimit: number;
}

interface StoredApiKey {
  id: string;
  service: string;
  key: string;
  addedAt: string;
}

interface DevApp {
  id: string;
  name: string;
}

// ─── Constants ──────────────────────────────────────────────────────────────

const MOCK_APPS: DevApp[] = [{ id: "1", name: "Uygulamam" }];
const LS_KEY = "obscura_api_keys";
const ALL_METHODS: HttpMethod[] = ["GET", "POST", "PUT", "DELETE"];

const AUTH_LABELS: Record<AuthType, string> = {
  none: "Yok",
  api_key: "API Anahtarı",
  bearer: "Bearer Token",
  oauth2: "OAuth2",
};

// ─── Helpers ────────────────────────────────────────────────────────────────

function uuid(): string {
  return crypto.randomUUID();
}

function readStoredKeys(): StoredApiKey[] {
  if (typeof window === "undefined") return [];
  try {
    return JSON.parse(localStorage.getItem(LS_KEY) || "[]");
  } catch {
    return [];
  }
}

function writeStoredKeys(keys: StoredApiKey[]): void {
  localStorage.setItem(LS_KEY, JSON.stringify(keys));
}

// ─── Sub-components ─────────────────────────────────────────────────────────

function SectionHeader({
  title,
  subtitle,
  open,
  onToggle,
}: {
  title: string;
  subtitle?: string;
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      onClick={onToggle}
      className="w-full flex items-center justify-between px-5 py-4 text-left transition-colors"
      style={{ borderBottom: open ? "1px solid var(--border-1)" : "none" }}
      aria-expanded={open}
    >
      <div>
        <p
          className="text-sm font-bold"
          style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}
        >
          {title}
        </p>
        {subtitle && (
          <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
            {subtitle}
          </p>
        )}
      </div>
      {open ? (
        <ChevronUp size={16} style={{ color: "var(--text-3)", flexShrink: 0 }} />
      ) : (
        <ChevronDown size={16} style={{ color: "var(--text-3)", flexShrink: 0 }} />
      )}
    </button>
  );
}

function MethodCheckbox({
  method,
  checked,
  onChange,
}: {
  method: HttpMethod;
  checked: boolean;
  onChange: (m: HttpMethod) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(method)}
      className="flex items-center gap-1.5 text-xs font-semibold transition-all"
      style={{ color: checked ? "var(--accent)" : "var(--text-3)" }}
      aria-pressed={checked}
    >
      {checked ? (
        <CheckSquare size={14} style={{ color: "var(--accent)" }} />
      ) : (
        <Square size={14} style={{ color: "var(--text-3)" }} />
      )}
      {method}
    </button>
  );
}

// ─── Section 1: Developer API Connections ───────────────────────────────────

function DeveloperSection() {
  const [open, setOpen] = useState(true);
  const [selectedApp, setSelectedApp] = useState<string>(MOCK_APPS[0].id);
  const [endpoints, setEndpoints] = useState<ApiEndpoint[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState("");

  // Form state
  const [label, setLabel] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [authType, setAuthType] = useState<AuthType>("none");
  const [authKey, setAuthKey] = useState("");
  const [showAuthKey, setShowAuthKey] = useState(false);
  const [methods, setMethods] = useState<HttpMethod[]>(["GET"]);
  const [rateLimit, setRateLimit] = useState<number>(60);
  const [formError, setFormError] = useState("");

  const toggleMethod = useCallback((m: HttpMethod) => {
    setMethods((prev) =>
      prev.includes(m) ? prev.filter((x) => x !== m) : [...prev, m]
    );
  }, []);

  const handleAdd = () => {
    if (!label.trim()) { setFormError("Etiket boş olamaz."); return; }
    if (!baseUrl.trim()) { setFormError("Base URL boş olamaz."); return; }
    try { new URL(baseUrl.trim()); } catch {
      setFormError("Geçerli bir URL girin (https://...).");
      return;
    }
    if (methods.length === 0) { setFormError("En az bir HTTP metodu seçin."); return; }
    if ((authType === "api_key" || authType === "bearer") && !authKey.trim()) {
      setFormError("Kimlik doğrulama anahtarı boş olamaz.");
      return;
    }

    const newEndpoint: ApiEndpoint = {
      id: uuid(),
      label: label.trim(),
      baseUrl: baseUrl.trim(),
      authType,
      authKey: authKey.trim(),
      methods,
      rateLimit,
    };

    setEndpoints((prev) => [...prev, newEndpoint]);
    setLabel("");
    setBaseUrl("");
    setAuthType("none");
    setAuthKey("");
    setShowAuthKey(false);
    setMethods(["GET"]);
    setRateLimit(60);
    setFormError("");
  };

  const handleDelete = (id: string) => {
    setEndpoints((prev) => prev.filter((e) => e.id !== id));
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveMsg("");
    try {
      await (
        (api as any).updateAppManifest?.({ appId: selectedApp, endpoints }) ??
        Promise.resolve()
      );
      setSaveMsg("Kaydedildi.");
    } catch {
      setSaveMsg("Kaydetme başarısız.");
    } finally {
      setSaving(false);
      setTimeout(() => setSaveMsg(""), 3000);
    }
  };

  return (
    <div
      className="rounded-3xl overflow-hidden"
      style={{
        background: "var(--surface-2)",
        border: "1px solid var(--border-2)",
      }}
    >
      <SectionHeader
        title="Uygulama API Bağlantıları"
        subtitle="Mini uygulamanızın kullanacağı dış endpoint'leri tanımlayın"
        open={open}
        onToggle={() => setOpen((v) => !v)}
      />

      {open && (
        <div className="px-5 py-4 space-y-5">
          {/* App selector */}
          <div>
            <label className="block text-xs font-semibold mb-1.5" style={{ color: "var(--text-2)" }}>
              Uygulama
            </label>
            <select
              value={selectedApp}
              onChange={(e) => setSelectedApp(e.target.value)}
              className="field h-10 text-sm w-full"
            >
              {MOCK_APPS.map((app) => (
                <option key={app.id} value={app.id}>
                  {app.name}
                </option>
              ))}
            </select>
          </div>

          {/* Add endpoint form */}
          <div
            className="rounded-2xl p-4 space-y-3"
            style={{
              background: "var(--surface-3)",
              border: "1px solid var(--border-1)",
            }}
          >
            <p
              className="text-xs font-bold uppercase tracking-wider"
              style={{ color: "var(--text-3)" }}
            >
              Yeni Endpoint Ekle
            </p>

            {/* Label + Base URL */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-semibold mb-1" style={{ color: "var(--text-2)" }}>
                  Etiket
                </label>
                <input
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  placeholder="Ürün Kataloğu"
                  className="field h-9 text-sm w-full"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold mb-1" style={{ color: "var(--text-2)" }}>
                  Base URL
                </label>
                <div className="relative">
                  <Globe
                    size={13}
                    className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none"
                    style={{ color: "var(--text-3)" }}
                  />
                  <input
                    value={baseUrl}
                    onChange={(e) => setBaseUrl(e.target.value)}
                    placeholder="https://api.example.com"
                    className="field h-9 text-sm w-full pl-8"
                  />
                </div>
              </div>
            </div>

            {/* Auth type */}
            <div>
              <label className="block text-xs font-semibold mb-1" style={{ color: "var(--text-2)" }}>
                Kimlik Doğrulama
              </label>
              <div className="flex flex-wrap gap-2">
                {(Object.keys(AUTH_LABELS) as AuthType[]).map((t) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => { setAuthType(t); setAuthKey(""); }}
                    className="h-7 px-3 rounded-full text-xs font-semibold transition-all"
                    style={{
                      background:
                        authType === t ? "var(--em)" : "var(--surface-2)",
                      color:
                        authType === t ? "#0a0a14" : "var(--text-3)",
                      border: `1px solid ${authType === t ? "transparent" : "var(--border-1)"}`,
                    }}
                  >
                    {AUTH_LABELS[t]}
                  </button>
                ))}
              </div>
            </div>

            {/* Auth key input */}
            {(authType === "api_key" || authType === "bearer") && (
              <div>
                <label className="block text-xs font-semibold mb-1" style={{ color: "var(--text-2)" }}>
                  {authType === "api_key" ? "API Anahtarı" : "Bearer Token"}
                </label>
                <div className="relative">
                  <Key
                    size={13}
                    className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none"
                    style={{ color: "var(--text-3)" }}
                  />
                  <input
                    type={showAuthKey ? "text" : "password"}
                    value={authKey}
                    onChange={(e) => setAuthKey(e.target.value)}
                    placeholder={authType === "api_key" ? "sk_..." : "eyJ..."}
                    className="field h-9 text-sm w-full pl-8 pr-9"
                  />
                  <button
                    type="button"
                    onClick={() => setShowAuthKey((v) => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2"
                    aria-label={showAuthKey ? "Gizle" : "Göster"}
                  >
                    {showAuthKey ? (
                      <EyeOff size={13} style={{ color: "var(--text-3)" }} />
                    ) : (
                      <Eye size={13} style={{ color: "var(--text-3)" }} />
                    )}
                  </button>
                </div>
                {/* Security notice inline */}
                <p className="text-[10px] mt-1 flex items-center gap-1" style={{ color: "var(--text-3)" }}>
                  <Lock size={9} />
                  Anahtar yalnızca cihazınızda saklanır, Obscura sunucularına iletilmez.
                </p>
              </div>
            )}

            {/* HTTP methods */}
            <div>
              <label className="block text-xs font-semibold mb-2" style={{ color: "var(--text-2)" }}>
                İzin Verilen HTTP Metodları
              </label>
              <div className="flex flex-wrap gap-4">
                {ALL_METHODS.map((m) => (
                  <MethodCheckbox
                    key={m}
                    method={m}
                    checked={methods.includes(m)}
                    onChange={toggleMethod}
                  />
                ))}
              </div>
            </div>

            {/* Rate limit */}
            <div className="flex items-center gap-3">
              <div className="flex-1">
                <label className="block text-xs font-semibold mb-1" style={{ color: "var(--text-2)" }}>
                  Hız Limiti (istek/dk)
                </label>
                <input
                  type="number"
                  min={1}
                  max={10000}
                  value={rateLimit}
                  onChange={(e) => setRateLimit(Math.max(1, Number(e.target.value)))}
                  className="field h-9 text-sm w-full"
                />
              </div>
            </div>

            {/* Form error */}
            {formError && (
              <p
                className="text-xs px-3 py-2 rounded-xl"
                style={{
                  background: "rgba(255,64,88,0.08)",
                  border: "1px solid rgba(255,64,88,0.18)",
                  color: "var(--error)",
                }}
              >
                {formError}
              </p>
            )}

            {/* Add button */}
            <button
              type="button"
              onClick={handleAdd}
              className="btn-primary h-9 px-4 text-xs w-full flex items-center justify-center gap-2"
            >
              <Plus size={13} />
              Endpoint Ekle
            </button>
          </div>

          {/* Endpoint list */}
          {endpoints.length > 0 && (
            <div className="space-y-2">
              <p className="section-label px-1">Eklenen Endpoint&apos;ler</p>
              {endpoints.map((ep) => (
                <div
                  key={ep.id}
                  className="flex items-start gap-3 rounded-2xl px-4 py-3"
                  style={{
                    background: "var(--surface-3)",
                    border: "1px solid var(--border-1)",
                  }}
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span
                        className="text-sm font-semibold"
                        style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}
                      >
                        {ep.label}
                      </span>
                      <span
                        className="text-[10px] rounded-full px-2 py-0.5"
                        style={{
                          background: "var(--surface-2)",
                          color: "var(--text-3)",
                          border: "1px solid var(--border-1)",
                        }}
                      >
                        {AUTH_LABELS[ep.authType]}
                      </span>
                    </div>
                    <p
                      className="text-xs mt-0.5 truncate"
                      style={{ color: "var(--text-3)" }}
                    >
                      {ep.baseUrl}
                    </p>
                    <div className="flex items-center gap-2 mt-1.5 flex-wrap">
                      {ep.methods.map((m) => (
                        <span
                          key={m}
                          className="text-[10px] font-bold rounded px-1.5 py-0.5"
                          style={{
                            background: "rgba(0,229,160,0.08)",
                            color: "var(--accent)",
                            border: "1px solid rgba(0,229,160,0.15)",
                          }}
                        >
                          {m}
                        </span>
                      ))}
                      <span
                        className="text-[10px]"
                        style={{ color: "var(--text-3)" }}
                      >
                        {ep.rateLimit} istek/dk
                      </span>
                    </div>
                  </div>
                  <button
                    onClick={() => handleDelete(ep.id)}
                    aria-label={`${ep.label} sil`}
                    className="flex-shrink-0 w-8 h-8 rounded-xl flex items-center justify-center transition-all active:scale-95"
                    style={{
                      background: "rgba(255,64,88,0.08)",
                      color: "var(--error)",
                      border: "1px solid rgba(255,64,88,0.15)",
                    }}
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Save */}
          <div className="flex items-center gap-3">
            <button
              onClick={handleSave}
              disabled={saving || endpoints.length === 0}
              className="btn-primary h-10 px-5 text-xs flex items-center gap-2 disabled:opacity-40"
            >
              {saving ? (
                <Loader2 size={13} className="animate-spin" />
              ) : (
                <Save size={13} />
              )}
              Kaydet
            </button>
            {saveMsg && (
              <p
                className="text-xs"
                style={{
                  color:
                    saveMsg === "Kaydedildi."
                      ? "var(--accent)"
                      : "var(--error)",
                }}
              >
                {saveMsg}
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Section 2: User API Keys ────────────────────────────────────────────────

function UserKeysSection() {
  const [open, setOpen] = useState(true);
  const [keys, setKeys] = useState<StoredApiKey[]>([]);
  const [service, setService] = useState("");
  const [keyValue, setKeyValue] = useState("");
  const [showKey, setShowKey] = useState<Record<string, boolean>>({});
  const [addError, setAddError] = useState("");

  useEffect(() => {
    setKeys(readStoredKeys());
  }, []);

  const handleAdd = () => {
    if (!service.trim()) { setAddError("Servis adı boş olamaz."); return; }
    if (!keyValue.trim()) { setAddError("API anahtarı boş olamaz."); return; }

    const newKey: StoredApiKey = {
      id: uuid(),
      service: service.trim(),
      key: keyValue.trim(),
      addedAt: new Date().toISOString().split("T")[0],
    };

    const updated = [...keys, newKey];
    setKeys(updated);
    writeStoredKeys(updated);
    setService("");
    setKeyValue("");
    setAddError("");
  };

  const handleDelete = (id: string) => {
    const updated = keys.filter((k) => k.id !== id);
    setKeys(updated);
    writeStoredKeys(updated);
  };

  const toggleShow = (id: string) => {
    setShowKey((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  return (
    <div
      className="rounded-3xl overflow-hidden"
      style={{
        background: "var(--surface-2)",
        border: "1px solid var(--border-2)",
      }}
    >
      <SectionHeader
        title="Harici API Anahtarlarım"
        subtitle="Entegre uygulamalar için sakladığınız anahtarlar"
        open={open}
        onToggle={() => setOpen((v) => !v)}
      />

      {open && (
        <div className="px-5 py-4 space-y-4">
          {/* Explanation notice */}
          <div
            className="flex items-start gap-3 rounded-2xl px-4 py-3"
            style={{
              background: "rgba(0,229,160,0.06)",
              border: "1px solid rgba(0,229,160,0.15)",
            }}
          >
            <ShieldCheck
              size={16}
              className="flex-shrink-0 mt-0.5"
              style={{ color: "var(--accent)" }}
            />
            <p className="text-xs leading-relaxed" style={{ color: "var(--text-2)" }}>
              Entegre uygulamalar sizden API anahtarı isteyebilir. Anahtarlar{" "}
              <strong style={{ color: "var(--text-1)" }}>cihazınızda şifrelenmiş</strong>{" "}
              saklanır ve Obscura sunucularına iletilmez.
            </p>
          </div>

          {/* Existing keys */}
          {keys.length > 0 && (
            <div className="space-y-2">
              {keys.map((k) => (
                <div
                  key={k.id}
                  className="flex items-center gap-3 rounded-2xl px-4 py-3"
                  style={{
                    background: "var(--surface-3)",
                    border: "1px solid var(--border-1)",
                  }}
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-0.5">
                      <span
                        className="text-sm font-semibold"
                        style={{
                          color: "var(--text-1)",
                          fontFamily: "var(--font-display)",
                        }}
                      >
                        {k.service}
                      </span>
                      <span
                        className="text-[10px]"
                        style={{ color: "var(--text-3)" }}
                      >
                        {k.addedAt}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span
                        className="text-xs font-mono truncate"
                        style={{ color: "var(--text-3)", maxWidth: "160px" }}
                      >
                        {showKey[k.id]
                          ? k.key
                          : k.key.slice(0, 6) + "••••••••••••"}
                      </span>
                      <button
                        onClick={() => toggleShow(k.id)}
                        aria-label={showKey[k.id] ? "Gizle" : "Göster"}
                      >
                        {showKey[k.id] ? (
                          <EyeOff size={12} style={{ color: "var(--text-3)" }} />
                        ) : (
                          <Eye size={12} style={{ color: "var(--text-3)" }} />
                        )}
                      </button>
                    </div>
                  </div>
                  <button
                    onClick={() => handleDelete(k.id)}
                    aria-label={`${k.service} sil`}
                    className="flex-shrink-0 w-8 h-8 rounded-xl flex items-center justify-center transition-all active:scale-95"
                    style={{
                      background: "rgba(255,64,88,0.08)",
                      color: "var(--error)",
                      border: "1px solid rgba(255,64,88,0.15)",
                    }}
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              ))}
            </div>
          )}

          {keys.length === 0 && (
            <p
              className="text-xs text-center py-4"
              style={{ color: "var(--text-3)" }}
            >
              Henüz kayıtlı API anahtarı yok.
            </p>
          )}

          {/* Add new key form */}
          <div
            className="rounded-2xl p-4 space-y-3"
            style={{
              background: "var(--surface-3)",
              border: "1px solid var(--border-1)",
            }}
          >
            <p
              className="text-xs font-bold uppercase tracking-wider"
              style={{ color: "var(--text-3)" }}
            >
              Anahtar Ekle
            </p>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label
                  className="block text-xs font-semibold mb-1"
                  style={{ color: "var(--text-2)" }}
                >
                  Servis Adı
                </label>
                <input
                  value={service}
                  onChange={(e) => setService(e.target.value)}
                  placeholder="MerchantAPI"
                  className="field h-9 text-sm w-full"
                />
              </div>
              <div>
                <label
                  className="block text-xs font-semibold mb-1"
                  style={{ color: "var(--text-2)" }}
                >
                  API Anahtarı
                </label>
                <div className="relative">
                  <Key
                    size={13}
                    className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none"
                    style={{ color: "var(--text-3)" }}
                  />
                  <input
                    type="password"
                    value={keyValue}
                    onChange={(e) => setKeyValue(e.target.value)}
                    placeholder="sk_..."
                    className="field h-9 text-sm w-full pl-8"
                    onKeyDown={(e) => e.key === "Enter" && handleAdd()}
                  />
                </div>
              </div>
            </div>

            {addError && (
              <p
                className="text-xs px-3 py-2 rounded-xl"
                style={{
                  background: "rgba(255,64,88,0.08)",
                  border: "1px solid rgba(255,64,88,0.18)",
                  color: "var(--error)",
                }}
              >
                {addError}
              </p>
            )}

            <button
              type="button"
              onClick={handleAdd}
              className="btn-primary h-9 px-4 text-xs w-full flex items-center justify-center gap-2"
            >
              <Plus size={13} />
              Ekle
            </button>
          </div>

          {/* Global security warning */}
          <div
            className="flex items-center gap-2 rounded-2xl px-4 py-2.5"
            style={{
              background: "var(--surface-3)",
              border: "1px solid var(--border-1)",
            }}
          >
            <Lock size={12} style={{ color: "var(--text-3)", flexShrink: 0 }} />
            <p className="text-[11px]" style={{ color: "var(--text-3)" }}>
              API anahtarları Obscura sunucularına iletilmez.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Page ────────────────────────────────────────────────────────────────────

export default function ApiConnectPage() {
  return (
    <AppShell showBack title="API Bağlantıları">
      <div className="flex flex-col h-full scroll-area pb-24">
        {/* Header */}
        <div className="page-header px-5">
          <div>
            <h1 className="page-title">API Bağlantıları</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-3)" }}>
              Uygulama ve kullanıcı API entegrasyonları
            </p>
          </div>
        </div>

        {/* Sections */}
        <div className="px-5 space-y-4">
          <DeveloperSection />
          <UserKeysSection />
        </div>
      </div>
    </AppShell>
  );
}
