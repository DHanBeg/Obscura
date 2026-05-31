"use client";

/**
 * MnemonicBackup — BIP39 12 kelime yedekleme modal akışı
 *
 * State akışı:
 *   show → verify → done
 *
 * Spec Bölüm 5.2 Adım 7:
 *   1. 12 kelimeyi göster, not aldım onayı al
 *   2. 3 rastgele kelime doğrulama
 *   3. Onay
 */

import { useState, useCallback, useEffect, useRef } from "react";
import { Copy, Check, ShieldCheck, AlertCircle, ArrowRight } from "lucide-react";
import { cn } from "@/lib/cn";
import {
  getMnemonicWords,
  pickVerificationWords,
  markMnemonicBackedUp,
} from "@/lib/mnemonic";

// ─── Tipler ───────────────────────────────────────────────────────────────────

export interface MnemonicBackupProps {
  mnemonic: string;
  onComplete: () => void;
  onSkip?: () => void;
}

type Stage = "show" | "verify" | "done";

interface VerifySlot {
  index: number;
  word: string;
  value: string;
  status: "idle" | "correct" | "wrong";
}

// ─── Sub-component: Kelime kartı ──────────────────────────────────────────────

function WordCard({ index, word }: { index: number; word: string }) {
  return (
    <div
      className="flex items-center gap-2 rounded-xl px-3 py-2.5"
      style={{
        background: "rgba(255,255,255,0.03)",
        border: "1px solid rgba(255,255,255,0.07)",
      }}
    >
      <span
        className="text-[10px] font-mono w-4 text-right shrink-0"
        style={{ color: "var(--text-3)" }}
      >
        {index + 1}
      </span>
      <span className="text-sm font-medium" style={{ color: "var(--text-1)" }}>
        {word}
      </span>
    </div>
  );
}

// ─── Stage: Göster ────────────────────────────────────────────────────────────

function ShowStage({
  mnemonic,
  onConfirm,
}: {
  mnemonic: string;
  onConfirm: () => void;
}) {
  const words = getMnemonicWords(mnemonic);
  const [copied, setCopied] = useState(false);
  const [checked, setChecked] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(mnemonic);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API mevcut değilse sessizce geç
    }
  }, [mnemonic]);

  return (
    <div className="space-y-5">
      {/* Başlık */}
      <div className="text-center space-y-1.5">
        <div
          className="w-12 h-12 rounded-full flex items-center justify-center mx-auto mb-3"
          style={{
            background: "rgba(0,229,160,0.08)",
            border: "1px solid rgba(0,229,160,0.2)",
          }}
        >
          <ShieldCheck size={22} style={{ color: "var(--accent)" }} />
        </div>
        <h2
          className="text-lg font-semibold"
          style={{ color: "var(--text-1)", letterSpacing: "-0.02em" }}
        >
          Kurtarma İfadenizi Kaydedin
        </h2>
        <p className="text-xs" style={{ color: "var(--text-2)" }}>
          Bu 12 kelime hesabınızın tek yedeğidir. Güvenli bir yere not alın.
        </p>
      </div>

      {/* Uyarı bandı */}
      <div
        className="flex items-start gap-2.5 rounded-xl px-3 py-2.5 text-xs"
        style={{
          background: "rgba(245,158,11,0.06)",
          border: "1px solid rgba(245,158,11,0.18)",
          color: "#f59e0b",
        }}
      >
        <AlertCircle size={13} className="mt-0.5 shrink-0" />
        <span>
          Bu kelimeleri kimseyle paylaşmayın. Obscura bunları hiçbir zaman sormaz.
        </span>
      </div>

      {/* 3×4 Grid */}
      <div className="grid grid-cols-3 gap-2" aria-label="Kurtarma kelimeleri">
        {words.map((word, i) => (
          <WordCard key={i} index={i} word={word} />
        ))}
      </div>

      {/* Kopyala butonu */}
      <button
        onClick={handleCopy}
        className={cn(
          "w-full flex items-center justify-center gap-2 py-2.5 rounded-xl text-xs font-medium",
          "transition-all duration-150"
        )}
        style={{
          background: "rgba(255,255,255,0.04)",
          border: "1px solid rgba(255,255,255,0.08)",
          color: copied ? "var(--accent)" : "var(--text-2)",
        }}
        aria-label={copied ? "Kopyalandı" : "Kelimeleri kopyala"}
      >
        {copied ? (
          <>
            <Check size={13} />
            <span>Kopyalandı</span>
          </>
        ) : (
          <>
            <Copy size={13} />
            <span>Kelimeleri kopyala</span>
          </>
        )}
      </button>

      {/* Not aldım onay kutusu */}
      <label
        className="flex items-start gap-3 cursor-pointer select-none"
        htmlFor="mnemonic-confirm"
      >
        <div className="relative mt-0.5">
          <input
            id="mnemonic-confirm"
            type="checkbox"
            checked={checked}
            onChange={(e) => setChecked(e.target.checked)}
            className="sr-only"
          />
          <div
            className={cn(
              "w-5 h-5 rounded-md flex items-center justify-center transition-all duration-150",
              "border"
            )}
            style={{
              background: checked ? "var(--accent)" : "transparent",
              borderColor: checked ? "var(--accent)" : "rgba(255,255,255,0.15)",
            }}
          >
            {checked && <Check size={12} color="#020208" strokeWidth={3} />}
          </div>
        </div>
        <span className="text-xs leading-relaxed" style={{ color: "var(--text-2)" }}>
          12 kelimeyi güvenli bir yere not aldım. Bu ekranı kapattıktan sonra
          bu kelimelere erişemeyeceğimi anlıyorum.
        </span>
      </label>

      {/* İleri */}
      <button
        onClick={onConfirm}
        disabled={!checked}
        className="btn-primary w-full"
        aria-disabled={!checked}
      >
        <span>Devam et</span>
        <ArrowRight size={15} />
      </button>
    </div>
  );
}

// ─── Stage: Doğrula ───────────────────────────────────────────────────────────

function VerifyStage({
  mnemonic,
  onVerified,
}: {
  mnemonic: string;
  onVerified: () => void;
}) {
  const [slots, setSlots] = useState<VerifySlot[]>(() =>
    pickVerificationWords(mnemonic).map(({ index, word }) => ({
      index,
      word,
      value: "",
      status: "idle",
    }))
  );
  const [globalError, setGlobalError] = useState("");
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  // İlk input'a focus
  useEffect(() => {
    inputRefs.current[0]?.focus();
  }, []);

  const handleChange = (slotIdx: number, val: string) => {
    setGlobalError("");
    const clean = val.toLowerCase().replace(/[^a-z]/g, "");
    setSlots((prev) =>
      prev.map((s, i) =>
        i === slotIdx ? { ...s, value: clean, status: "idle" } : s
      )
    );
  };

  const handleVerify = useCallback(() => {
    let allCorrect = true;
    const updated = slots.map((s) => {
      const correct = s.value.trim() === s.word;
      if (!correct) allCorrect = false;
      return { ...s, status: correct ? ("correct" as const) : ("wrong" as const) };
    });
    setSlots(updated);

    if (allCorrect) {
      markMnemonicBackedUp();
      setTimeout(onVerified, 400);
    } else {
      setGlobalError("Bazı kelimeler yanlış. Lütfen tekrar kontrol edin.");
    }
  }, [slots, onVerified]);

  const handleKeyDown = (
    slotIdx: number,
    e: React.KeyboardEvent<HTMLInputElement>
  ) => {
    if (e.key === "Enter") {
      if (slotIdx < slots.length - 1) {
        inputRefs.current[slotIdx + 1]?.focus();
      } else {
        handleVerify();
      }
    }
  };

  return (
    <div className="space-y-5">
      {/* Başlık */}
      <div className="text-center space-y-1.5">
        <h2
          className="text-lg font-semibold"
          style={{ color: "var(--text-1)", letterSpacing: "-0.02em" }}
        >
          Doğrulama
        </h2>
        <p className="text-xs" style={{ color: "var(--text-2)" }}>
          Not aldığınızı doğrulamak için aşağıdaki kelimeleri girin.
        </p>
      </div>

      {/* Doğrulama alanları */}
      <div className="space-y-3" role="group" aria-label="Kelime doğrulama">
        {slots.map((slot, i) => (
          <div key={slot.index} className="space-y-1.5">
            <label
              className="text-xs font-medium"
              style={{ color: "var(--text-3)" }}
              htmlFor={`verify-slot-${i}`}
            >
              {slot.index + 1}. kelime
            </label>
            <input
              id={`verify-slot-${i}`}
              ref={(el) => { inputRefs.current[i] = el; }}
              type="text"
              value={slot.value}
              onChange={(e) => handleChange(i, e.target.value)}
              onKeyDown={(e) => handleKeyDown(i, e)}
              autoComplete="off"
              autoCorrect="off"
              spellCheck={false}
              aria-invalid={slot.status === "wrong"}
              className="field w-full font-mono"
              style={{
                borderColor:
                  slot.status === "correct"
                    ? "rgba(0,229,160,0.5)"
                    : slot.status === "wrong"
                    ? "var(--error)"
                    : undefined,
                background:
                  slot.status === "correct"
                    ? "rgba(0,229,160,0.04)"
                    : slot.status === "wrong"
                    ? "rgba(239,68,68,0.04)"
                    : undefined,
              }}
              placeholder={`${slot.index + 1}. kelimeyi girin`}
            />
            {slot.status === "wrong" && (
              <p role="alert" className="text-[10px]" style={{ color: "var(--error)" }}>
                Yanlış kelime
              </p>
            )}
          </div>
        ))}
      </div>

      {globalError && (
        <p role="alert" className="text-xs text-center" style={{ color: "var(--error)" }}>
          {globalError}
        </p>
      )}

      <button
        onClick={handleVerify}
        disabled={slots.some((s) => !s.value.trim())}
        className="btn-primary w-full"
      >
        Doğrula
      </button>
    </div>
  );
}

// ─── Stage: Tamamlandı ────────────────────────────────────────────────────────

function DoneStage({ onComplete }: { onComplete: () => void }) {
  // onComplete'i otomatik çağır
  useEffect(() => {
    const t = setTimeout(onComplete, 1800);
    return () => clearTimeout(t);
  }, [onComplete]);

  return (
    <div className="flex flex-col items-center justify-center space-y-5 py-6 text-center">
      <div
        className="w-16 h-16 rounded-full flex items-center justify-center"
        style={{
          background: "rgba(0,229,160,0.1)",
          border: "1px solid rgba(0,229,160,0.25)",
        }}
      >
        <ShieldCheck size={28} style={{ color: "var(--accent)" }} />
      </div>
      <div className="space-y-1">
        <h2
          className="text-lg font-semibold"
          style={{ color: "var(--text-1)", letterSpacing: "-0.02em" }}
        >
          Yedeğiniz Güvende
        </h2>
        <p className="text-xs" style={{ color: "var(--text-2)" }}>
          Hesabınız artık kurtarılabilir. Yönlendiriliyor…
        </p>
      </div>
    </div>
  );
}

// ─── Ana Component ────────────────────────────────────────────────────────────

export default function MnemonicBackup({
  mnemonic,
  onComplete,
  onSkip,
}: MnemonicBackupProps) {
  const [stage, setStage] = useState<Stage>("show");

  // ESC ile atlama (onSkip varsa)
  useEffect(() => {
    if (!onSkip) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onSkip();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onSkip]);

  return (
    /* Tam ekran overlay */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ background: "rgba(2,2,8,0.92)", backdropFilter: "blur(16px)" }}
      role="dialog"
      aria-modal="true"
      aria-label="Kurtarma ifadesini yedekle"
    >
      {/* İçerik kartı */}
      <div
        className="w-full max-w-sm rounded-2xl overflow-hidden animate-in"
        style={{
          background: "var(--surface-1)",
          border: "1px solid var(--border-1)",
          maxHeight: "90vh",
          overflowY: "auto",
        }}
      >
        {/* Üst bar */}
        <div
          className="flex items-center justify-between px-5 py-3.5"
          style={{ borderBottom: "1px solid var(--border-1)" }}
        >
          {/* Adım göstergesi */}
          <div className="flex items-center gap-1.5">
            {(["show", "verify", "done"] as Stage[]).map((s, i) => (
              <div
                key={s}
                className="h-1 rounded-full transition-all duration-300"
                style={{
                  width:
                    stage === s
                      ? 20
                      : ["show", "verify", "done"].indexOf(stage) > i
                      ? 8
                      : 8,
                  background:
                    stage === s
                      ? "var(--accent)"
                      : ["show", "verify", "done"].indexOf(stage) > i
                      ? "rgba(0,229,160,0.4)"
                      : "rgba(255,255,255,0.1)",
                }}
              />
            ))}
          </div>

          {/* Atla butonu */}
          {onSkip && stage !== "done" && (
            <button
              onClick={onSkip}
              className="text-[11px] transition-opacity hover:opacity-70"
              style={{ color: "var(--text-3)" }}
              aria-label="Daha sonra yedekle"
            >
              Sonra
            </button>
          )}
        </div>

        {/* Stage içeriği */}
        <div className="px-5 py-5">
          {stage === "show" && (
            <ShowStage
              mnemonic={mnemonic}
              onConfirm={() => setStage("verify")}
            />
          )}
          {stage === "verify" && (
            <VerifyStage
              mnemonic={mnemonic}
              onVerified={() => setStage("done")}
            />
          )}
          {stage === "done" && <DoneStage onComplete={onComplete} />}
        </div>
      </div>
    </div>
  );
}
