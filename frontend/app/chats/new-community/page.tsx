"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Users2, ChevronDown } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

const TOPICS = [
  { value: "Teknoloji", label: "Teknoloji" },
  { value: "Spor", label: "Spor" },
  { value: "Oyunlar", label: "Oyunlar" },
  { value: "Sanat", label: "Sanat" },
  { value: "Diğer", label: "Diğer" },
] as const;

type Topic = (typeof TOPICS)[number]["value"];

export default function NewCommunityPage() {
  const router = useRouter();

  const [communityName, setCommunityName] = useState("");
  const [description, setDescription] = useState("");
  const [topic, setTopic] = useState<Topic>("Teknoloji");
  const [topicOpen, setTopicOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit =
    communityName.trim().length > 0 &&
    description.trim().length > 0 &&
    !isSubmitting;

  const handleCreate = useCallback(async () => {
    if (!canSubmit) return;
    setIsSubmitting(true);
    setError(null);
    try {
      // NOT: backend CreateConversationRequest'te "topic" alanı yok (bkz.
      // backend/internal/api/extra_handlers.go:192-206) — mobile new-community.tsx
      // da aynı şekilde topics'i sadece local UI state olarak tutuyor, backend'e
      // hiç göndermiyor. Burada da göndermiyoruz; kategori seçimi şu an sadece
      // ekran-içi, kalıcı değil (pre-existing gap, B7 kapsamı dışı).
      const res = await api.createConversation({
        type: "community",
        name: communityName.trim(),
        description: description.trim(),
        is_public: true, // topluluk her zaman açık — mobile new-community.tsx'te de is_public seçeneği yok
      });
      router.replace(res?.conv_id ? `/chats/${res.conv_id}` : "/chats");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Topluluk oluşturulamadı. Lütfen tekrar deneyin.");
    } finally {
      setIsSubmitting(false);
    }
  }, [canSubmit, communityName, description, topic, router]);

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
    <AppShell showBack title="Yeni Topluluk">
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
            <Users2 size={26} style={{ color: "var(--em)" }} />
          </div>
        </div>

        {/* Community name */}
        <div className="flex flex-col gap-2">
          <label
            htmlFor="community-name"
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Topluluk Adı <span style={{ color: "var(--em)" }}>*</span>
          </label>
          <input
            id="community-name"
            type="text"
            placeholder="Topluluğunuza bir ad verin"
            value={communityName}
            maxLength={80}
            onChange={(e) => setCommunityName(e.target.value)}
            autoFocus
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = "rgba(0,229,160,0.35)"; }}
            onBlur={(e) => { e.currentTarget.style.borderColor = "var(--border-1)"; }}
            aria-required="true"
          />
          <span className="text-right text-[11px]" style={{ color: "var(--text-3)" }}>
            {communityName.length}/80
          </span>
        </div>

        {/* Description */}
        <div className="flex flex-col gap-2">
          <label
            htmlFor="community-desc"
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Açıklama <span style={{ color: "var(--em)" }}>*</span>
          </label>
          <textarea
            id="community-desc"
            placeholder="Topluluğun amacını ve kurallarını kısaca anlatın"
            value={description}
            maxLength={400}
            rows={4}
            onChange={(e) => setDescription(e.target.value)}
            style={{
              ...inputStyle,
              resize: "none",
              lineHeight: 1.6,
            }}
            onFocus={(e) => { e.currentTarget.style.borderColor = "rgba(0,229,160,0.35)"; }}
            onBlur={(e) => { e.currentTarget.style.borderColor = "var(--border-1)"; }}
            aria-required="true"
          />
          <span className="text-right text-[11px]" style={{ color: "var(--text-3)" }}>
            {description.length}/400
          </span>
        </div>

        {/* Topic / Category select */}
        <div className="flex flex-col gap-2">
          <p
            className="text-[11px] font-semibold tracking-wider uppercase"
            style={{ color: "var(--text-3)" }}
          >
            Kategori
          </p>

          {/* Custom dropdown trigger */}
          <div className="relative">
            <button
              type="button"
              onClick={() => setTopicOpen((v) => !v)}
              aria-haspopup="listbox"
              aria-expanded={topicOpen}
              className="w-full flex items-center justify-between px-4 py-4 rounded-2xl text-[15px] font-medium transition-colors"
              style={{
                background: "var(--surface-2)",
                border: topicOpen
                  ? "1px solid rgba(0,229,160,0.35)"
                  : "1px solid var(--border-1)",
                color: "var(--text-1)",
              }}
            >
              <span>{TOPICS.find((t) => t.value === topic)?.label}</span>
              <ChevronDown
                size={16}
                style={{
                  color: "var(--text-3)",
                  transform: topicOpen ? "rotate(180deg)" : "rotate(0deg)",
                  transition: "transform 200ms",
                }}
              />
            </button>

            {/* Dropdown options */}
            {topicOpen && (
              <div
                role="listbox"
                aria-label="Kategori seç"
                className="absolute left-0 right-0 top-full mt-1 z-20 rounded-2xl overflow-hidden"
                style={{
                  background: "var(--surface-2)",
                  border: "1px solid var(--border-2)",
                  boxShadow: "0 8px 32px rgba(0,0,0,0.5)",
                }}
              >
                {TOPICS.map((t, i) => {
                  const selected = t.value === topic;
                  return (
                    <button
                      key={t.value}
                      role="option"
                      aria-selected={selected}
                      onClick={() => {
                        setTopic(t.value);
                        setTopicOpen(false);
                      }}
                      className="w-full flex items-center justify-between px-4 py-3.5 text-[14px] font-medium text-left transition-colors hover:bg-white/[0.04]"
                      style={{
                        color: selected ? "var(--em)" : "var(--text-1)",
                        borderBottom:
                          i < TOPICS.length - 1 ? "1px solid var(--border-1)" : "none",
                      }}
                    >
                      {t.label}
                      {selected && (
                        <svg width="14" height="10" viewBox="0 0 14 10" fill="none">
                          <path
                            d="M1 5l3.5 3.5L13 1"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
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
            "Topluluğu Oluştur"
          )}
        </button>
      </div>
    </AppShell>
  );
}
