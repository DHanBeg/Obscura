"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Search, X, Users, ChevronRight, Camera, Info } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";

// B10 Faz 1 KAPSAM SINIRI: web'den grup KURULAMAZ (MLS grup-kurma kriptosu
// B10.2'ye kadar web'e taşınmadı — bkz. B10 Faz 0.5 kararı). Bu sayfa üye
// seçimini bırakmıyor (kullanışlı, zararsız) ama Adım 2'deki "Grubu Oluştur"
// devre dışı — açık uyarı, sahte/kırık bir grup oluşturmasına İZİN VERİLMİYOR.
const WEB_GROUP_CREATE_DISABLED = true;

interface UserResult {
  did: string;
  display_name?: string;
  username?: string;
}

// ── Step Indicator ────────────────────────────────────────────────────────────
function StepDots({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center justify-center gap-2 py-4">
      {Array.from({ length: total }).map((_, i) => (
        <span
          key={i}
          style={{
            width: i === current ? 20 : 7,
            height: 7,
            borderRadius: 999,
            background: i === current ? "var(--em)" : "var(--surface-3)",
            transition: "all 200ms cubic-bezier(0.4,0,0.2,1)",
            display: "inline-block",
          }}
        />
      ))}
    </div>
  );
}

// ── Member Chip ───────────────────────────────────────────────────────────────
function MemberChip({ name, onRemove }: { name: string; onRemove: () => void }) {
  return (
    <span
      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[13px] font-medium"
      style={{
        background: "rgba(0,229,160,0.1)",
        border: "1px solid rgba(0,229,160,0.2)",
        color: "var(--text-1)",
      }}
    >
      {name}
      <button
        onClick={onRemove}
        aria-label={`${name} kişisini kaldır`}
        className="flex items-center justify-center rounded-full"
        style={{ color: "var(--text-3)", width: 16, height: 16 }}
      >
        <X size={11} />
      </button>
    </span>
  );
}

// ── Main Page ─────────────────────────────────────────────────────────────────
export default function NewGroupPage() {
  const router = useRouter();

  const [step, setStep] = useState(0);
  const [groupName, setGroupName] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<UserResult[]>([]);
  const [selectedMembers, setSelectedMembers] = useState<UserResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSearch = useCallback(async (q: string) => {
    setSearchQuery(q);
    if (!q.trim()) {
      setSearchResults([]);
      return;
    }
    setIsSearching(true);
    try {
      const results = await api.searchUsers(q).catch(() => []);
      setSearchResults(Array.isArray(results) ? results : []);
    } catch {
      setSearchResults([]);
    } finally {
      setIsSearching(false);
    }
  }, []);

  const toggleMember = useCallback((user: UserResult) => {
    setSelectedMembers((prev) => {
      const exists = prev.find((m) => m.did === user.did);
      return exists ? prev.filter((m) => m.did !== user.did) : [...prev, user];
    });
  }, []);

  const handleCreate = useCallback(async () => {
    if (!groupName.trim() || isSubmitting) return;
    setIsSubmitting(true);
    setError(null);
    try {
      const res = await api.createConversation({
        type: "group",
        name: groupName.trim(),
        members: selectedMembers.map((m) => m.did),
      });
      router.replace(res?.conv_id ? `/chats/${res.conv_id}` : "/chats");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Grup oluşturulamadı. Lütfen tekrar deneyin.");
    } finally {
      setIsSubmitting(false);
    }
  }, [groupName, selectedMembers, isSubmitting, router]);

  const memberName = (u: UserResult) => u.display_name || u.username || u.did.slice(0, 10);
  const isMemberSelected = (did: string) => selectedMembers.some((m) => m.did === did);

  return (
    <AppShell showBack title="Yeni Grup">
      <div className="flex flex-col h-full" style={{ background: "var(--bg)" }}>

        {/* Step indicator */}
        <StepDots current={step} total={3} />

        {/* Dürüstlük notu — B10 Faz 1 kapsam sınırı, en baştan görünür */}
        <div
          className="mx-5 mb-1 flex items-start gap-2 px-3 py-2.5 rounded-2xl flex-shrink-0"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
        >
          <Info size={14} style={{ color: "var(--text-3)", marginTop: 2, flexShrink: 0 }} />
          <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
            Web&apos;den grup oluşturma şu an desteklenmiyor. Grubu mobil uygulamadan kurup web&apos;den davetle katılabilirsin (bkz. Grup Davetleri).
          </p>
        </div>

        {/* ── Step 0: Name + Avatar ─────────────────────────────────── */}
        {step === 0 && (
          <div className="flex-1 flex flex-col px-5 pt-2 pb-8 gap-6">
            {/* Avatar placeholder */}
            <div className="flex justify-center">
              <button
                aria-label="Grup fotoğrafı ekle"
                className="flex flex-col items-center justify-center gap-2 rounded-full"
                style={{
                  width: 88,
                  height: 88,
                  background: "var(--surface-2)",
                  border: "2px dashed var(--border-2)",
                  color: "var(--text-3)",
                }}
              >
                <Camera size={22} />
                <span className="text-[10px]" style={{ color: "var(--text-3)" }}>Fotoğraf</span>
              </button>
            </div>

            {/* Group name input */}
            <div className="flex flex-col gap-2">
              <label
                htmlFor="group-name"
                className="text-[11px] font-semibold tracking-wider uppercase"
                style={{ color: "var(--text-3)" }}
              >
                Grup Adı
              </label>
              <input
                id="group-name"
                className="field"
                type="text"
                placeholder="Grubunuza bir ad verin"
                value={groupName}
                maxLength={60}
                onChange={(e) => setGroupName(e.target.value)}
                autoFocus
                style={{
                  background: "var(--surface-2)",
                  border: "1px solid var(--border-1)",
                  borderRadius: 14,
                  padding: "14px 16px",
                  color: "var(--text-1)",
                  fontSize: 15,
                  outline: "none",
                  width: "100%",
                }}
                onFocus={(e) => {
                  e.currentTarget.style.borderColor = "rgba(0,229,160,0.35)";
                }}
                onBlur={(e) => {
                  e.currentTarget.style.borderColor = "var(--border-1)";
                }}
              />
              <span
                className="text-right text-[11px]"
                style={{ color: "var(--text-3)" }}
              >
                {groupName.length}/60
              </span>
            </div>

            <div className="flex-1" />

            <button
              disabled={!groupName.trim()}
              onClick={() => setStep(1)}
              className="flex items-center justify-center gap-2 w-full rounded-2xl py-4 text-[15px] font-semibold transition-all active:scale-[0.98]"
              style={{
                background: groupName.trim() ? "var(--em)" : "var(--surface-3)",
                color: groupName.trim() ? "#0a0a14" : "var(--text-3)",
                cursor: groupName.trim() ? "pointer" : "not-allowed",
              }}
            >
              Devam Et
              <ChevronRight size={17} strokeWidth={2.5} />
            </button>
          </div>
        )}

        {/* ── Step 1: Member Search ─────────────────────────────────── */}
        {step === 1 && (
          <div className="flex-1 flex flex-col px-5 pt-2 pb-8 gap-4 overflow-hidden">
            {/* Search bar */}
            <div
              className="flex items-center gap-3 rounded-2xl px-4"
              style={{
                background: "var(--surface-2)",
                border: "1px solid var(--border-1)",
                height: 48,
              }}
            >
              <Search size={16} style={{ color: "var(--text-3)", flexShrink: 0 }} />
              <input
                type="search"
                placeholder="Kullanıcı ara..."
                value={searchQuery}
                onChange={(e) => handleSearch(e.target.value)}
                autoFocus
                style={{
                  flex: 1,
                  background: "transparent",
                  border: "none",
                  outline: "none",
                  color: "var(--text-1)",
                  fontSize: 14,
                }}
                aria-label="Üye ara"
              />
              {isSearching && (
                <span
                  className="w-4 h-4 rounded-full border-2 animate-spin flex-shrink-0"
                  style={{ borderColor: "var(--border-2)", borderTopColor: "var(--em)" }}
                />
              )}
            </div>

            {/* Selected member chips */}
            {selectedMembers.length > 0 && (
              <div className="flex flex-wrap gap-2">
                {selectedMembers.map((m) => (
                  <MemberChip
                    key={m.did}
                    name={memberName(m)}
                    onRemove={() => toggleMember(m)}
                  />
                ))}
              </div>
            )}

            {/* Results list */}
            <div className="flex-1 overflow-y-auto -mx-5 px-5 scroll-area">
              {searchResults.length === 0 && searchQuery.trim() && !isSearching && (
                <div
                  className="flex flex-col items-center justify-center py-12 gap-2"
                  style={{ color: "var(--text-3)" }}
                >
                  <Users size={28} />
                  <p className="text-[13px]">Kullanıcı bulunamadı</p>
                </div>
              )}
              {searchResults.map((user) => {
                const selected = isMemberSelected(user.did);
                return (
                  <button
                    key={user.did}
                    onClick={() => toggleMember(user)}
                    className="w-full flex items-center gap-3 px-3 py-3 rounded-2xl transition-colors"
                    style={{
                      background: selected ? "rgba(0,229,160,0.08)" : "transparent",
                      marginBottom: 2,
                    }}
                  >
                    {/* Avatar placeholder */}
                    <div
                      className="flex-shrink-0 flex items-center justify-center rounded-full text-[13px] font-bold"
                      style={{
                        width: 40,
                        height: 40,
                        background: "var(--surface-3)",
                        color: "var(--accent)",
                      }}
                    >
                      {memberName(user).charAt(0).toUpperCase()}
                    </div>
                    <div className="flex-1 min-w-0 text-left">
                      <p
                        className="text-[14px] font-medium truncate"
                        style={{ color: "var(--text-1)" }}
                      >
                        {memberName(user)}
                      </p>
                      {user.username && (
                        <p
                          className="text-[12px] truncate"
                          style={{ color: "var(--text-3)" }}
                        >
                          @{user.username}
                        </p>
                      )}
                    </div>
                    <span
                      className="flex-shrink-0 w-5 h-5 rounded-full flex items-center justify-center transition-all"
                      style={{
                        border: selected
                          ? "2px solid var(--em)"
                          : "2px solid var(--border-2)",
                        background: selected ? "var(--em)" : "transparent",
                      }}
                    >
                      {selected && (
                        <svg width="10" height="8" viewBox="0 0 10 8" fill="none">
                          <path d="M1 4l2.5 2.5L9 1" stroke="#0a0a14" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>
                      )}
                    </span>
                  </button>
                );
              })}
            </div>

            <button
              onClick={() => setStep(2)}
              className="flex items-center justify-center gap-2 w-full rounded-2xl py-4 text-[15px] font-semibold transition-all active:scale-[0.98]"
              style={{
                background: "var(--em)",
                color: "#0a0a14",
              }}
            >
              Devam Et
              {selectedMembers.length > 0 && (
                <span
                  className="flex items-center justify-center rounded-full text-[11px] font-bold"
                  style={{
                    width: 20,
                    height: 20,
                    background: "rgba(0,0,0,0.2)",
                    color: "#0a0a14",
                  }}
                >
                  {selectedMembers.length}
                </span>
              )}
            </button>
          </div>
        )}

        {/* ── Step 2: Confirm + Create ──────────────────────────────── */}
        {step === 2 && (
          <div className="flex-1 flex flex-col px-5 pt-2 pb-8 gap-5">
            {/* Summary card */}
            <div
              className="rounded-2xl p-5 flex flex-col gap-4"
              style={{
                background: "var(--surface-2)",
                border: "1px solid var(--border-1)",
              }}
            >
              <div className="flex items-center gap-4">
                <div
                  className="flex items-center justify-center rounded-full flex-shrink-0"
                  style={{
                    width: 52,
                    height: 52,
                    background: "rgba(0,229,160,0.1)",
                    border: "1px solid rgba(0,229,160,0.2)",
                  }}
                >
                  <Users size={22} style={{ color: "var(--em)" }} />
                </div>
                <div>
                  <p
                    className="text-[16px] font-bold"
                    style={{ color: "var(--text-1)" }}
                  >
                    {groupName}
                  </p>
                  <p className="text-[12px] mt-0.5" style={{ color: "var(--text-3)" }}>
                    {selectedMembers.length > 0
                      ? `${selectedMembers.length} üye`
                      : "Sadece siz"}
                  </p>
                </div>
              </div>

              {selectedMembers.length > 0 && (
                <div className="flex flex-wrap gap-2 pt-1">
                  {selectedMembers.map((m) => (
                    <span
                      key={m.did}
                      className="text-[12px] px-2.5 py-1 rounded-full"
                      style={{
                        background: "var(--surface-3)",
                        color: "var(--text-2)",
                      }}
                    >
                      {memberName(m)}
                    </span>
                  ))}
                </div>
              )}
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
              onClick={() => setStep(1)}
              className="w-full rounded-2xl py-3.5 text-[14px] font-medium transition-all active:scale-[0.98]"
              style={{
                background: "var(--surface-2)",
                border: "1px solid var(--border-1)",
                color: "var(--text-2)",
              }}
            >
              Geri Dön
            </button>

            <button
              disabled={isSubmitting || WEB_GROUP_CREATE_DISABLED}
              onClick={handleCreate}
              className="flex items-center justify-center gap-2 w-full rounded-2xl py-4 text-[15px] font-semibold transition-all active:scale-[0.98]"
              style={{
                background: isSubmitting || WEB_GROUP_CREATE_DISABLED ? "var(--surface-3)" : "var(--em)",
                color: isSubmitting || WEB_GROUP_CREATE_DISABLED ? "var(--text-3)" : "#0a0a14",
                cursor: WEB_GROUP_CREATE_DISABLED ? "not-allowed" : undefined,
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
              ) : WEB_GROUP_CREATE_DISABLED ? (
                "Web'den grup oluşturulamıyor"
              ) : (
                "Grubu Oluştur"
              )}
            </button>
          </div>
        )}
      </div>
    </AppShell>
  );
}
