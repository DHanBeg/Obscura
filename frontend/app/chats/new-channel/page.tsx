"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Radio, Lock, Globe } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

export default function NewChannelPage() {
  const router = useRouter();

  const [channelName, setChannelName] = useState("");
  const [description, setDescription] = useState("");
  const [isPublic, setIsPublic] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = channelName.trim().length > 0 && !isSubmitting;

  const handleCreate = useCallback(async () => {
    if (!canSubmit) return;
    setIsSubmitting(true);
    setError(null);
    try {
      const res = await api.createConversation({
        type: "channel",
        name: channelName.trim(),
        description: description.trim() || undefined,
        is_public: isPublic,
      });
      router.replace(res?.conv_id ? `/chats/${res.conv_id}` : "/chats");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Kanal oluşturulamadı. Lütfen tekrar deneyin.");
    } finally {
      setIsSubmitting(false);
    }
  }, [canSubmit, channelName, description, isPublic, router]);

  const inputStyle: React.CSSProperties = {
    background: "var(--surface-2)",
    border: "1px solid var(--border-1)",
    borderRadius: 14,
    padding: "14px 16px",
    color: "var(--text-1)",
    fontSize: 15,
    outline: "none",
    width: "100%",
    transition: "border-color 150ms",
  };

  return (
    <AppShell showBack title="Yeni Kanal">
      <div
        className="flex flex-col h-full px-5 pt-4 pb-8 gap-5"
        style={{ background: "var(--bg)" }}
      >
        {/* Header icon */}
        <div className="flex justify-center py-2">
          <div
            className="flex items-center justify-center rounded-full"
            style={{
              width: 64,
              height: 64,
              background: "rgba(0,229,160,0.08)",
              border: "1px solid rgba(0,229,160,0.2)",
            }}
          >
            <Radio size={26} style={{ color: "var(--em)" }} />
          </div>
        </div>

        {/* Channel name */}
        <div className="flex flex-col gap-2">
          <label
            htmlFor="channel-name"
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Kanal Adı <span style={{ color: "var(--em)" }}>*</span>
          </label>
          <input
            id="channel-name"
            type="text"
            placeholder="Kanalınıza bir ad verin"
            value={channelName}
            maxLength={80}
            onChange={(e) => setChannelName(e.target.value)}
            autoFocus
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = "rgba(0,229,160,0.35)"; }}
            onBlur={(e) => { e.currentTarget.style.borderColor = "var(--border-1)"; }}
            aria-required="true"
          />
          <span className="text-right text-[11px]" style={{ color: "var(--text-3)" }}>
            {channelName.length}/80
          </span>
        </div>

        {/* Description */}
        <div className="flex flex-col gap-2">
          <label
            htmlFor="channel-desc"
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Açıklama <span style={{ color: "var(--text-3)", fontWeight: 400 }}>(isteğe bağlı)</span>
          </label>
          <textarea
            id="channel-desc"
            placeholder="Kanalın amacını kısaca anlatın"
            value={description}
            maxLength={280}
            rows={3}
            onChange={(e) => setDescription(e.target.value)}
            style={{
              ...inputStyle,
              resize: "none",
              lineHeight: 1.6,
            }}
            onFocus={(e) => { e.currentTarget.style.borderColor = "rgba(0,229,160,0.35)"; }}
            onBlur={(e) => { e.currentTarget.style.borderColor = "var(--border-1)"; }}
          />
          <span className="text-right text-[11px]" style={{ color: "var(--text-3)" }}>
            {description.length}/280
          </span>
        </div>

        {/* Public / Private toggle */}
        <div className="flex flex-col gap-3">
          <p
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Gizlilik
          </p>

          <div className="flex gap-3">
            {/* Public option */}
            <button
              onClick={() => setIsPublic(true)}
              aria-pressed={isPublic}
              className="flex-1 flex flex-col items-center gap-2 py-4 rounded-2xl transition-all active:scale-[0.97]"
              style={{
                background: isPublic ? "rgba(0,229,160,0.08)" : "var(--surface-2)",
                border: isPublic
                  ? "1.5px solid rgba(0,229,160,0.35)"
                  : "1.5px solid var(--border-1)",
              }}
            >
              <Globe
                size={20}
                style={{ color: isPublic ? "var(--em)" : "var(--text-3)" }}
              />
              <span
                className="text-[13px] font-semibold"
                style={{ color: isPublic ? "var(--em)" : "var(--text-2)" }}
              >
                Herkese Açık
              </span>
              <span
                className="text-[11px] text-center px-2"
                style={{ color: "var(--text-3)" }}
              >
                Herkes katılabilir
              </span>
            </button>

            {/* Private option */}
            <button
              onClick={() => setIsPublic(false)}
              aria-pressed={!isPublic}
              className="flex-1 flex flex-col items-center gap-2 py-4 rounded-2xl transition-all active:scale-[0.97]"
              style={{
                background: !isPublic ? "rgba(0,229,160,0.08)" : "var(--surface-2)",
                border: !isPublic
                  ? "1.5px solid rgba(0,229,160,0.35)"
                  : "1.5px solid var(--border-1)",
              }}
            >
              <Lock
                size={20}
                style={{ color: !isPublic ? "var(--em)" : "var(--text-3)" }}
              />
              <span
                className="text-[13px] font-semibold"
                style={{ color: !isPublic ? "var(--em)" : "var(--text-2)" }}
              >
                Özel
              </span>
              <span
                className="text-[11px] text-center px-2"
                style={{ color: "var(--text-3)" }}
              >
                Davetli üyeler
              </span>
            </button>
          </div>
        </div>

        {error && (
          <p
            className="text-[13px] text-center rounded-xl px-4 py-3"
            style={{
              color: "var(--error, #ef4444)",
              background: "rgba(239,68,68,0.08)",
              border: "1px solid rgba(239,68,68,0.15)",
            }}
          >
            {error}
          </p>
        )}

        <div className="flex-1" />

        <button
          disabled={!canSubmit}
          onClick={handleCreate}
          className="flex items-center justify-center gap-2 w-full rounded-2xl py-4 text-[15px] font-semibold transition-all active:scale-[0.98]"
          style={{
            background: canSubmit ? "var(--em)" : "var(--surface-3)",
            color: canSubmit ? "#0a0a14" : "var(--text-3)",
            cursor: canSubmit ? "pointer" : "not-allowed",
          }}
        >
          {isSubmitting ? (
            <>
              <span
                className="w-4 h-4 rounded-full border-2 animate-spin"
                style={{ borderColor: "rgba(0,0,0,0.15)", borderTopColor: "#0a0a14" }}
              />
              Oluşturuluyor...
            </>
          ) : (
            "Kanalı Oluştur"
          )}
        </button>
      </div>
    </AppShell>
  );
}
