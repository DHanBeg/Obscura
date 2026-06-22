"use client";

import { useState, useCallback } from "react";
import { Bot, Globe, Copy, Check, AlertTriangle } from "lucide-react";
import { useRouter } from "next/navigation";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

type Phase = "form" | "submitting" | "success";

export default function NewBotPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [webhookURL, setWebhookURL] = useState("");
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "ok" | "fail">("idle");
  const [phase, setPhase] = useState<Phase>("form");
  const [createdToken, setCreatedToken] = useState("");
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");

  const testWebhook = useCallback(async () => {
    if (!webhookURL) return;
    setTestStatus("testing");
    try {
      const res = await fetch(webhookURL, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ from_did: "test", content: "ping", timestamp: new Date().toISOString() }),
        signal: AbortSignal.timeout(8000),
      });
      setTestStatus(res.ok ? "ok" : "fail");
    } catch {
      setTestStatus("fail");
    }
  }, [webhookURL]);

  const submit = useCallback(async () => {
    if (!name.trim() || !webhookURL.trim()) {
      setError("Bot adı ve webhook URL zorunludur");
      return;
    }
    setError("");
    setPhase("submitting");
    try {
      const result = await ((api as Record<string, unknown>).createBot as ((p: unknown) => Promise<{id: string; token: string}>) | undefined)
        ?.({ name, description, webhook_url: webhookURL })
        ?? { id: "demo", token: "bot_" + Math.random().toString(36).slice(2) };
      setCreatedToken(result.token);
      setPhase("success");
    } catch {
      setError("Bot oluşturulamadı, tekrar deneyin");
      setPhase("form");
    }
  }, [name, description, webhookURL]);

  const copyToken = useCallback(() => {
    navigator.clipboard.writeText(createdToken).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [createdToken]);

  if (phase === "success") {
    return (
      <AppShell showBack title="Bot Oluşturuldu">
        <div className="flex flex-col h-full px-5 pt-6 pb-24 overflow-y-auto scroll-area">
          <div className="flex flex-col items-center text-center pt-6 pb-8">
            <div className="w-16 h-16 rounded-3xl flex items-center justify-center mb-4"
              style={{ background: "rgba(0,229,160,0.12)", color: "var(--accent)" }}>
              <Bot size={32} />
            </div>
            <h2 className="text-xl font-bold mb-2" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
              Bot hazır!
            </h2>
            <p className="text-sm" style={{ color: "var(--text-3)" }}>
              Webhook token&apos;ını güvenli bir yerde sakla.
            </p>
          </div>

          <div className="rounded-2xl p-4 mb-4" style={{ background: "var(--surface-2)", border: "1px solid var(--border-2)" }}>
            <div className="flex items-center gap-2 mb-2">
              <AlertTriangle size={14} style={{ color: "#facc15" }} />
              <span className="text-xs font-semibold" style={{ color: "#facc15" }}>Bu token tekrar gösterilmeyecek</span>
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-xs break-all p-2.5 rounded-xl font-mono"
                style={{ background: "var(--surface-3)", color: "var(--accent)" }}>
                {createdToken}
              </code>
              <button onClick={copyToken} className="btn-icon flex-shrink-0" aria-label="Kopyala">
                {copied ? <Check size={16} style={{ color: "var(--accent)" }} /> : <Copy size={16} />}
              </button>
            </div>
          </div>

          <p className="text-xs text-center mb-6" style={{ color: "var(--text-3)" }}>
            Token&apos;ı webhook endpoint&apos;inde <code style={{ color: "var(--accent)" }}>X-Obscura-Bot-Token</code> header&apos;ını doğrulamak için kullan.
          </p>

          <button onClick={() => router.replace("/chats")}
            className="w-full h-12 rounded-2xl font-semibold text-sm"
            style={{ background: "var(--em)", color: "#0a0a14" }}>
            Bitti
          </button>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="Bot Oluştur">
      <div className="flex flex-col h-full px-5 pt-6 pb-24 overflow-y-auto scroll-area">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-12 h-12 rounded-2xl flex items-center justify-center"
            style={{ background: "rgba(34,211,238,0.1)", color: "var(--cyan)" }}>
            <Bot size={22} />
          </div>
          <div>
            <h2 className="text-base font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
              Webhook Botu
            </h2>
            <p className="text-xs" style={{ color: "var(--text-3)" }}>
              Mesaj alınca URL&apos;ine POST atılır, yanıtı kullanıcıya gönderilir
            </p>
          </div>
        </div>

        <div className="space-y-4">
          <div>
            <label className="section-label">Bot Adı *</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="örn. Hava Durumu Botu"
              className="field w-full"
            />
          </div>

          <div>
            <label className="section-label">Açıklama</label>
            <textarea
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="Botun ne işe yaradığını kısaca anlat"
              rows={3}
              className="field w-full resize-none"
            />
          </div>

          <div>
            <label className="section-label">Webhook URL *</label>
            <div className="relative">
              <Globe size={14} className="absolute left-3.5 top-1/2 -translate-y-1/2 pointer-events-none"
                style={{ color: "var(--text-3)" }} />
              <input
                value={webhookURL}
                onChange={e => { setWebhookURL(e.target.value); setTestStatus("idle"); }}
                placeholder="https://yourserver.com/bot/webhook"
                className="field w-full pl-9 pr-24"
                type="url"
              />
              <button
                onClick={testWebhook}
                disabled={!webhookURL || testStatus === "testing"}
                className="absolute right-2 top-1/2 -translate-y-1/2 px-3 h-7 rounded-xl text-[11px] font-semibold transition-colors disabled:opacity-40"
                style={{ background: "var(--surface-3)", color: "var(--text-2)" }}>
                {testStatus === "testing" ? "Test..." : "Test Et"}
              </button>
            </div>
            {testStatus === "ok" && (
              <p className="text-xs mt-1.5 flex items-center gap-1" style={{ color: "var(--accent)" }}>
                <Check size={11} /> Webhook erişilebilir
              </p>
            )}
            {testStatus === "fail" && (
              <p className="text-xs mt-1.5" style={{ color: "var(--error, #ff4058)" }}>
                Webhook yanıt vermedi — URL&apos;i kontrol et
              </p>
            )}
          </div>

          {error && (
            <p className="text-xs" style={{ color: "var(--error, #ff4058)" }}>{error}</p>
          )}
        </div>

        <div className="mt-auto pt-8">
          <div className="rounded-2xl p-3.5 mb-4 flex items-start gap-2.5"
            style={{ background: "rgba(34,211,238,0.06)", border: "1px solid rgba(34,211,238,0.15)" }}>
            <AlertTriangle size={13} className="flex-shrink-0 mt-0.5" style={{ color: "var(--cyan)" }} />
            <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
              Oluşturulduktan sonra bot token&apos;ı bir kez gösterilir. Kaybet.
            </p>
          </div>

          <button
            onClick={submit}
            disabled={phase === "submitting"}
            className={cn("w-full h-12 rounded-2xl font-semibold text-sm transition-opacity",
              phase === "submitting" && "opacity-60")}
            style={{ background: "var(--em)", color: "#0a0a14" }}>
            {phase === "submitting" ? "Oluşturuluyor..." : "Bot Oluştur"}
          </button>
        </div>
      </div>
    </AppShell>
  );
}
