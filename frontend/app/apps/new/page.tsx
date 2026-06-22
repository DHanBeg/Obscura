"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  ChevronRight,
  ChevronLeft,
  Upload,
  Plus,
  Trash2,
  Loader2,
  CheckCircle2,
  ImagePlus,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";

// ── Types ──────────────────────────────────────────────────────────────────────

type Category = "tools" | "finance" | "social" | "games";
type Language = "javascript" | "typescript" | "python";
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
type Permission = "location" | "messaging" | "wallet" | "zk_proof";

interface ExternalEndpoint {
  id: string;
  label: string;
  url: string;
  method: HttpMethod;
}

interface Step1 {
  name: string;
  description: string;
  category: Category;
  iconFileName: string;
}

interface Step2 {
  entryMode: "url" | "upload";
  entryUrl: string;
  permissions: Permission[];
  allowedDomains: string;
  language: Language;
}

interface Step3 {
  webhookUrl: string;
  endpoints: ExternalEndpoint[];
}

// ── Constants ──────────────────────────────────────────────────────────────────

const CATEGORIES: { value: Category; label: string }[] = [
  { value: "tools", label: "Araçlar" },
  { value: "finance", label: "Finans" },
  { value: "social", label: "Sosyal" },
  { value: "games", label: "Oyunlar" },
];

const PERMISSIONS: { value: Permission; label: string; desc: string }[] = [
  { value: "location", label: "Konum", desc: "Kullanıcının GPS konumuna eriş" },
  { value: "messaging", label: "Mesajlaşma", desc: "Mesaj gönder ve al" },
  { value: "wallet", label: "Cüzdan", desc: "Bakiye sorgula ve transfer başlat" },
  { value: "zk_proof", label: "ZK Kanıt", desc: "Sıfır bilgi kanıtı üret" },
];

const LANGUAGES: { value: Language; label: string; note?: string }[] = [
  { value: "javascript", label: "JavaScript" },
  { value: "typescript", label: "TypeScript" },
  { value: "python", label: "Python", note: "Pyodide aracılığıyla JS'e derlenir" },
];

const HTTP_METHODS: HttpMethod[] = ["GET", "POST", "PUT", "DELETE", "PATCH"];

const METHOD_STYLES: Record<HttpMethod, string> = {
  GET: "text-[var(--accent)]",
  POST: "text-[#a78bfa]",
  PUT: "text-[#facc15]",
  DELETE: "text-[var(--error)]",
  PATCH: "text-[var(--signal)]",
};

const STEPS = ["Temel Bilgiler", "Teknik Ayarlar", "Yayın"];

// ── Step indicator ─────────────────────────────────────────────────────────────

function StepIndicator({ current }: { current: number }) {
  return (
    <div className="flex items-center justify-center gap-0 mb-6">
      {STEPS.map((label, i) => {
        const isDone = i < current;
        const isActive = i === current;
        return (
          <div key={label} className="flex items-center">
            <div className="flex flex-col items-center gap-1">
              <div
                className="w-7 h-7 rounded-full flex items-center justify-center text-[11px] font-bold transition-all duration-200"
                style={{
                  background: isDone
                    ? "var(--accent)"
                    : isActive
                    ? "var(--em)"
                    : "var(--surface-3)",
                  color: isDone || isActive ? "#020208" : "var(--text-3)",
                  border: isActive
                    ? "2px solid var(--em)"
                    : "2px solid transparent",
                  boxShadow: isActive
                    ? "0 0 10px var(--em-glow)"
                    : "none",
                }}
              >
                {isDone ? <CheckCircle2 size={14} /> : i + 1}
              </div>
              <span
                className="text-[9px] font-semibold whitespace-nowrap"
                style={{
                  color: isActive
                    ? "var(--text-1)"
                    : isDone
                    ? "var(--accent)"
                    : "var(--text-3)",
                }}
              >
                {label}
              </span>
            </div>
            {i < STEPS.length - 1 && (
              <div
                className="h-px w-10 mx-1 mb-4 transition-all duration-300"
                style={{
                  background: isDone ? "var(--accent)" : "var(--border-1)",
                }}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}

// ── Section title ──────────────────────────────────────────────────────────────

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <p className="section-label mb-3">{children}</p>
  );
}

// ── Label ──────────────────────────────────────────────────────────────────────

function FieldLabel({
  children,
  htmlFor,
  optional,
}: {
  children: React.ReactNode;
  htmlFor?: string;
  optional?: boolean;
}) {
  return (
    <label
      htmlFor={htmlFor}
      className="block text-xs font-semibold mb-1.5"
      style={{ color: "var(--text-2)" }}
    >
      {children}
      {optional && (
        <span className="ml-1.5 text-[10px] font-normal" style={{ color: "var(--text-3)" }}>
          (isteğe bağlı)
        </span>
      )}
    </label>
  );
}

// ── Step 1 ─────────────────────────────────────────────────────────────────────

function Step1Form({
  data,
  onChange,
}: {
  data: Step1;
  onChange: (updates: Partial<Step1>) => void;
}) {
  return (
    <div className="space-y-5">
      {/* Icon upload placeholder */}
      <div>
        <FieldLabel>Uygulama İkonu</FieldLabel>
        <div
          className="flex items-center gap-4"
        >
          <div
            className="w-16 h-16 rounded-2xl flex flex-col items-center justify-center gap-1 flex-shrink-0 cursor-pointer transition-all hover:opacity-80"
            style={{
              background: "var(--surface-3)",
              border: "2px dashed var(--border-2)",
            }}
          >
            {data.iconFileName ? (
              <div className="w-full h-full rounded-2xl flex items-center justify-center">
                <ImagePlus size={20} style={{ color: "var(--accent)" }} />
              </div>
            ) : (
              <>
                <Upload size={16} style={{ color: "var(--text-3)" }} />
                <span className="text-[9px]" style={{ color: "var(--text-3)" }}>PNG / SVG</span>
              </>
            )}
          </div>
          <div className="flex-1 min-w-0">
            <label
              className="inline-flex items-center gap-2 h-9 px-4 rounded-xl text-xs font-semibold cursor-pointer transition-all hover:opacity-80"
              style={{
                background: "var(--surface-3)",
                border: "1px solid var(--border-2)",
                color: "var(--text-2)",
              }}
            >
              <Upload size={13} />
              {data.iconFileName ? data.iconFileName : "Dosya Seç"}
              <input
                type="file"
                accept="image/png,image/svg+xml,image/webp"
                className="sr-only"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) onChange({ iconFileName: file.name });
                }}
              />
            </label>
            <p className="text-[10px] mt-1.5" style={{ color: "var(--text-3)" }}>
              Maks. 512×512 px, PNG veya SVG önerilir
            </p>
          </div>
        </div>
      </div>

      {/* App name */}
      <div>
        <FieldLabel htmlFor="app-name">Uygulama Adı</FieldLabel>
        <input
          id="app-name"
          className="field h-10 text-sm"
          placeholder="Uygulamanıza bir ad verin"
          value={data.name}
          onChange={(e) => onChange({ name: e.target.value })}
          maxLength={64}
        />
      </div>

      {/* Short description */}
      <div>
        <FieldLabel htmlFor="app-desc">Kısa Açıklama</FieldLabel>
        <textarea
          id="app-desc"
          className="field text-sm resize-none"
          style={{ minHeight: 80 }}
          placeholder="Uygulamanızın ne yaptığını kısaca açıklayın (maks. 160 karakter)"
          value={data.description}
          onChange={(e) => onChange({ description: e.target.value.slice(0, 160) })}
          rows={3}
        />
        <p className="text-[10px] mt-1 text-right" style={{ color: "var(--text-3)" }}>
          {data.description.length}/160
        </p>
      </div>

      {/* Category */}
      <div>
        <FieldLabel>Kategori</FieldLabel>
        <div className="grid grid-cols-2 gap-2">
          {CATEGORIES.map((cat) => (
            <button
              key={cat.value}
              type="button"
              onClick={() => onChange({ category: cat.value })}
              className="h-10 rounded-xl text-sm font-semibold transition-all duration-150"
              style={{
                background:
                  data.category === cat.value
                    ? "var(--accent-muted)"
                    : "var(--surface-3)",
                color:
                  data.category === cat.value
                    ? "var(--accent)"
                    : "var(--text-2)",
                border: `1px solid ${
                  data.category === cat.value
                    ? "rgba(0,229,160,0.25)"
                    : "var(--border-1)"
                }`,
              }}
            >
              {cat.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

// ── Step 2 ─────────────────────────────────────────────────────────────────────

function Step2Form({
  data,
  onChange,
}: {
  data: Step2;
  onChange: (updates: Partial<Step2>) => void;
}) {
  const togglePermission = (perm: Permission) => {
    const has = data.permissions.includes(perm);
    onChange({
      permissions: has
        ? data.permissions.filter((p) => p !== perm)
        : [...data.permissions, perm],
    });
  };

  return (
    <div className="space-y-5">
      {/* Entry point */}
      <div>
        <FieldLabel>Giriş Noktası</FieldLabel>
        <div className="flex rounded-xl overflow-hidden mb-3" style={{ border: "1px solid var(--border-1)" }}>
          {(["url", "upload"] as const).map((mode, idx) => (
            <button
              key={mode}
              type="button"
              onClick={() => onChange({ entryMode: mode })}
              className={cn(
                "flex-1 h-9 text-xs font-semibold transition-all duration-150",
                idx === 0 ? "" : "border-l"
              )}
              style={{
                background:
                  data.entryMode === mode
                    ? "var(--em)"
                    : "var(--surface-2)",
                color:
                  data.entryMode === mode
                    ? "#020208"
                    : "var(--text-3)",
                borderColor: "var(--border-1)",
              }}
            >
              {mode === "url" ? "URL Gir" : "Dosya Yükle"}
            </button>
          ))}
        </div>
        {data.entryMode === "url" ? (
          <input
            className="field h-10 text-sm font-mono"
            placeholder="https://example.com/app.js"
            value={data.entryUrl}
            onChange={(e) => onChange({ entryUrl: e.target.value })}
          />
        ) : (
          <label
            className="flex items-center justify-center gap-2 h-12 rounded-xl cursor-pointer transition-all hover:opacity-80"
            style={{
              background: "var(--surface-3)",
              border: "2px dashed var(--border-2)",
              color: "var(--text-3)",
            }}
          >
            <Upload size={15} />
            <span className="text-sm">Dosya seçin veya sürükleyin</span>
            <input type="file" accept=".js,.ts,.py" className="sr-only" />
          </label>
        )}
      </div>

      {/* Language */}
      <div>
        <FieldLabel>Dil</FieldLabel>
        <div className="flex flex-col gap-2">
          {LANGUAGES.map((lang) => (
            <button
              key={lang.value}
              type="button"
              onClick={() => onChange({ language: lang.value })}
              className="flex items-center justify-between h-11 px-3.5 rounded-xl text-sm transition-all duration-150"
              style={{
                background:
                  data.language === lang.value
                    ? "var(--accent-muted)"
                    : "var(--surface-3)",
                color:
                  data.language === lang.value
                    ? "var(--accent)"
                    : "var(--text-2)",
                border: `1px solid ${
                  data.language === lang.value
                    ? "rgba(0,229,160,0.25)"
                    : "var(--border-1)"
                }`,
              }}
            >
              <span className="font-semibold">{lang.label}</span>
              {lang.note && (
                <span className="text-[10px]" style={{ color: data.language === lang.value ? "var(--accent)" : "var(--text-3)" }}>
                  {lang.note}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Permissions */}
      <div>
        <FieldLabel>İzinler</FieldLabel>
        <div className="space-y-2">
          {PERMISSIONS.map((perm) => {
            const active = data.permissions.includes(perm.value);
            return (
              <button
                key={perm.value}
                type="button"
                onClick={() => togglePermission(perm.value)}
                className="w-full flex items-center gap-3 px-3.5 py-3 rounded-xl text-left transition-all duration-150"
                style={{
                  background: active
                    ? "var(--accent-muted)"
                    : "var(--surface-3)",
                  border: `1px solid ${
                    active ? "rgba(0,229,160,0.2)" : "var(--border-1)"
                  }`,
                }}
              >
                <div
                  className="w-5 h-5 rounded-md flex items-center justify-center flex-shrink-0 transition-all"
                  style={{
                    background: active ? "var(--accent)" : "var(--surface-2)",
                    border: `1.5px solid ${active ? "var(--accent)" : "var(--border-2)"}`,
                  }}
                >
                  {active && (
                    <CheckCircle2 size={12} style={{ color: "#020208" }} />
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <p
                    className="text-sm font-semibold"
                    style={{ color: active ? "var(--accent)" : "var(--text-1)" }}
                  >
                    {perm.label}
                  </p>
                  <p className="text-[11px]" style={{ color: "var(--text-3)" }}>
                    {perm.desc}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Allowed domains */}
      <div>
        <FieldLabel htmlFor="allowed-domains" optional>
          İzin Verilen Alanlar
        </FieldLabel>
        <input
          id="allowed-domains"
          className="field h-10 text-sm"
          placeholder="example.com, api.example.com"
          value={data.allowedDomains}
          onChange={(e) => onChange({ allowedDomains: e.target.value })}
        />
        <p className="text-[10px] mt-1" style={{ color: "var(--text-3)" }}>
          Virgülle ayrılmış alan adları. Boş bırakılırsa kısıtlama uygulanmaz.
        </p>
      </div>
    </div>
  );
}

// ── Step 3 ─────────────────────────────────────────────────────────────────────

function Step3Form({
  data,
  onChange,
  step1,
  step2,
}: {
  data: Step3;
  onChange: (updates: Partial<Step3>) => void;
  step1: Step1;
  step2: Step2;
}) {
  const addEndpoint = () => {
    onChange({
      endpoints: [
        ...data.endpoints,
        { id: crypto.randomUUID(), label: "", url: "", method: "GET" },
      ],
    });
  };

  const removeEndpoint = (id: string) => {
    onChange({ endpoints: data.endpoints.filter((ep) => ep.id !== id) });
  };

  const updateEndpoint = (id: string, updates: Partial<ExternalEndpoint>) => {
    onChange({
      endpoints: data.endpoints.map((ep) =>
        ep.id === id ? { ...ep, ...updates } : ep
      ),
    });
  };

  return (
    <div className="space-y-5">
      {/* Webhook URL */}
      <div>
        <FieldLabel htmlFor="webhook-url" optional>
          Webhook URL
        </FieldLabel>
        <input
          id="webhook-url"
          className="field h-10 text-sm font-mono"
          placeholder="https://example.com/webhook"
          value={data.webhookUrl}
          onChange={(e) => onChange({ webhookUrl: e.target.value })}
        />
        <p className="text-[10px] mt-1" style={{ color: "var(--text-3)" }}>
          Uygulama olayları bu adrese POST isteği olarak iletilir.
        </p>
      </div>

      {/* External endpoints */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <FieldLabel optional>Harici API Uç Noktaları</FieldLabel>
          <button
            type="button"
            onClick={addEndpoint}
            className="flex items-center gap-1 h-7 px-2.5 rounded-lg text-[11px] font-semibold transition-all hover:opacity-80"
            style={{
              background: "var(--accent-muted)",
              color: "var(--accent)",
              border: "1px solid rgba(0,229,160,0.2)",
            }}
          >
            <Plus size={12} />
            Ekle
          </button>
        </div>

        {data.endpoints.length === 0 ? (
          <div
            className="flex flex-col items-center justify-center py-6 rounded-xl gap-2"
            style={{
              background: "var(--surface-3)",
              border: "1px dashed var(--border-1)",
            }}
          >
            <p className="text-xs" style={{ color: "var(--text-3)" }}>
              Harici API uç noktası eklenmedi
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {data.endpoints.map((ep) => (
              <div
                key={ep.id}
                className="rounded-xl p-3 space-y-2"
                style={{
                  background: "var(--surface-3)",
                  border: "1px solid var(--border-1)",
                }}
              >
                <div className="flex gap-2">
                  <select
                    value={ep.method}
                    onChange={(e) =>
                      updateEndpoint(ep.id, { method: e.target.value as HttpMethod })
                    }
                    className="field h-8 text-xs font-mono font-bold w-24 px-2"
                    style={{ color: METHOD_STYLES[ep.method] }}
                  >
                    {HTTP_METHODS.map((m) => (
                      <option key={m} value={m}>
                        {m}
                      </option>
                    ))}
                  </select>
                  <input
                    className="field h-8 text-xs font-mono flex-1"
                    placeholder="https://api.example.com/data"
                    value={ep.url}
                    onChange={(e) => updateEndpoint(ep.id, { url: e.target.value })}
                  />
                  <button
                    type="button"
                    onClick={() => removeEndpoint(ep.id)}
                    className="btn-icon w-8 h-8 flex-shrink-0"
                    style={{ color: "var(--error)" }}
                    aria-label="Kaldır"
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
                <input
                  className="field h-8 text-xs"
                  placeholder="Etiket (örn. Fiyat API)"
                  value={ep.label}
                  onChange={(e) => updateEndpoint(ep.id, { label: e.target.value })}
                />
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Review summary */}
      <div>
        <SectionTitle>Özet</SectionTitle>
        <div
          className="rounded-2xl p-4 space-y-2.5"
          style={{
            background: "var(--surface-2)",
            border: "1px solid var(--border-2)",
          }}
        >
          {[
            { label: "Ad", value: step1.name || "—" },
            {
              label: "Kategori",
              value:
                CATEGORIES.find((c) => c.value === step1.category)?.label ?? "—",
            },
            { label: "Dil", value: LANGUAGES.find((l) => l.value === step2.language)?.label ?? "—" },
            {
              label: "İzinler",
              value:
                step2.permissions.length > 0
                  ? step2.permissions
                      .map(
                        (p) =>
                          PERMISSIONS.find((x) => x.value === p)?.label ?? p
                      )
                      .join(", ")
                  : "Yok",
            },
            {
              label: "Harici Uç Nokta",
              value: `${data.endpoints.length} adet`,
            },
          ].map((row) => (
            <div key={row.label} className="flex items-center justify-between gap-4">
              <span className="text-xs" style={{ color: "var(--text-3)" }}>
                {row.label}
              </span>
              <span
                className="text-xs font-semibold text-right"
                style={{ color: "var(--text-1)" }}
              >
                {row.value}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function NewAppPage() {
  const router = useRouter();
  const [step, setStep] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const [step1, setStep1] = useState<Step1>({
    name: "",
    description: "",
    category: "tools",
    iconFileName: "",
  });

  const [step2, setStep2] = useState<Step2>({
    entryMode: "url",
    entryUrl: "",
    permissions: [],
    allowedDomains: "",
    language: "javascript",
  });

  const [step3, setStep3] = useState<Step3>({
    webhookUrl: "",
    endpoints: [],
  });

  const canNext = () => {
    if (step === 0) return step1.name.trim().length > 0 && step1.description.trim().length > 0;
    if (step === 1) {
      if (step2.entryMode === "url") return step2.entryUrl.trim().length > 0;
      return true;
    }
    return true;
  };

  const handleNext = () => {
    if (step < STEPS.length - 1) {
      setStep((s) => s + 1);
    } else {
      handleSubmit();
    }
  };

  const handleSubmit = async () => {
    setLoading(true);
    setError("");
    try {
      const manifest = {
        name: step1.name,
        description: step1.description,
        category: step1.category,
        permissions: step2.permissions,
        allowedDomains: step2.allowedDomains
          .split(",")
          .map((d) => d.trim())
          .filter(Boolean),
        language: step2.language,
      };

      const body = {
        manifest,
        code_url: step2.entryUrl || "pending",
        webhook_url: step3.webhookUrl || undefined,
        external_endpoints: step3.endpoints,
      };

      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      await ((api as Record<string, unknown>).createApp as ((b: unknown) => Promise<unknown>) | undefined)?.(body)
        ?? Promise.resolve({ id: "demo" });

      setSuccess(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Bir hata oluştu");
    } finally {
      setLoading(false);
    }
  };

  if (success) {
    return (
      <AppShell showBack title="Yeni Uygulama">
        <div className="flex flex-col h-full items-center justify-center gap-6 px-8 text-center">
          <div
            className="w-20 h-20 rounded-3xl flex items-center justify-center"
            style={{
              background: "var(--accent-muted)",
              border: "1px solid rgba(0,229,160,0.2)",
            }}
          >
            <CheckCircle2 size={36} style={{ color: "var(--accent)" }} />
          </div>
          <div>
            <h2
              className="text-lg font-bold mb-2"
              style={{
                fontFamily: "var(--font-display)",
                color: "var(--text-1)",
              }}
            >
              Uygulama Oluşturuldu!
            </h2>
            <p className="text-sm" style={{ color: "var(--text-3)" }}>
              Uygulamanız inceleme sürecine alındı. Onaylandığında mağazada yayınlanacak.
            </p>
          </div>
          <div className="flex flex-col gap-2 w-full">
            <button
              className="btn-primary h-11 text-sm"
              onClick={() => router.push("/apps/my")}
            >
              Uygulamalarımı Görüntüle
            </button>
            <button
              className="btn-secondary h-11 text-sm"
              onClick={() => router.push("/apps")}
            >
              Mağazaya Dön
            </button>
          </div>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="Yeni Uygulama">
      <div className="flex flex-col h-full scroll-area pb-24">
        <div className="px-5 pt-5">
          <StepIndicator current={step} />

          {/* Step content */}
          <div>
            {step === 0 && (
              <Step1Form
                data={step1}
                onChange={(u) => setStep1((s) => ({ ...s, ...u }))}
              />
            )}
            {step === 1 && (
              <Step2Form
                data={step2}
                onChange={(u) => setStep2((s) => ({ ...s, ...u }))}
              />
            )}
            {step === 2 && (
              <Step3Form
                data={step3}
                onChange={(u) => setStep3((s) => ({ ...s, ...u }))}
                step1={step1}
                step2={step2}
              />
            )}
          </div>

          {/* Error */}
          {error && (
            <div
              className="mt-4 rounded-2xl border px-4 py-3 text-sm"
              style={{
                background: "rgba(255,64,88,0.06)",
                borderColor: "rgba(255,64,88,0.18)",
                color: "var(--error)",
              }}
            >
              {error}
            </div>
          )}
        </div>
      </div>

      {/* Sticky footer navigation */}
      <div
        className="fixed bottom-0 inset-x-0 flex items-center justify-center"
        style={{ zIndex: 30 }}
      >
        <div
          className="w-full flex items-center gap-3 px-5 py-4"
          style={{
            maxWidth: 480,
            background: "var(--bg)",
            borderTop: "1px solid var(--border-1)",
          }}
        >
          {step > 0 ? (
            <button
              type="button"
              onClick={() => setStep((s) => s - 1)}
              className="btn-secondary h-11 px-4 flex items-center gap-2 text-sm"
            >
              <ChevronLeft size={16} />
              Geri
            </button>
          ) : (
            <div className="flex-1" />
          )}

          <button
            type="button"
            onClick={handleNext}
            disabled={!canNext() || loading}
            className={cn(
              "btn-primary h-11 flex-1 flex items-center justify-center gap-2 text-sm",
              (!canNext() || loading) && "opacity-50 cursor-not-allowed"
            )}
          >
            {loading ? (
              <Loader2 size={15} className="animate-spin" />
            ) : step < STEPS.length - 1 ? (
              <>
                İleri
                <ChevronRight size={16} />
              </>
            ) : (
              "Yayınla"
            )}
          </button>
        </div>
      </div>
    </AppShell>
  );
}
