"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, ChevronLeft, Lock, ShieldCheck } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { proveIdentity } from "@/lib/zk";
import {
  generateIdentity,
  generatePreKeyStore,
  bundleToUpload,
  saveIdentity,
  loadIdentity,
} from "@/lib/e2ee";
import {
  generateMnemonic12,
  deriveSecretFromMnemonic,
  isMnemonicBackedUp,
} from "@/lib/mnemonic";
import MnemonicBackup from "@/components/MnemonicBackup";

type Step = "phone" | "otp" | "username" | "mnemonic";
const OTP_LENGTH = 6;

/* ── Universal phone formatter ──────────────────────────────── */

function formatPhoneDisplay(digits: string): string {
  if (!digits) return "";
  // Group digits: first 2-3 as country code, then 3+3+4 pattern
  // Universal: just insert spaces every 3 digits after first 2
  const clean = digits.replace(/\D/g, "");
  if (clean.length <= 2) return clean;
  const parts: string[] = [];
  let i = 0;
  // First chunk: 2 digits (country code area)
  parts.push(clean.slice(0, 2));
  i = 2;
  // Remaining: groups of 3
  while (i < clean.length) {
    parts.push(clean.slice(i, i + 3));
    i += 3;
  }
  return parts.join(" ");
}

/* ── Country code detection ─────────────────────────────────── */

interface CountryInfo {
  name: string;
  flag: string;
  code: string;
  format: string; // placeholder for digits after country code
}

const COUNTRY_CODES: Record<string, CountryInfo> = {
  "1":   { name: "ABD / Kanada",  flag: "🇺🇸", code: "1",   format: "XXX XXX XXXX" },
  "7":   { name: "Rusya",         flag: "🇷🇺", code: "7",   format: "XXX XXX XXXX" },
  "20":  { name: "Mısır",         flag: "🇪🇬", code: "20",  format: "XX XXXX XXXX" },
  "27":  { name: "Güney Afrika",  flag: "🇿🇦", code: "27",  format: "XX XXX XXXX" },
  "30":  { name: "Yunanistan",    flag: "🇬🇷", code: "30",  format: "XXX XXX XXXX" },
  "31":  { name: "Hollanda",      flag: "🇳🇱", code: "31",  format: "X XXXX XXXX" },
  "32":  { name: "Belçika",       flag: "🇧🇪", code: "32",  format: "XXX XX XX XX" },
  "33":  { name: "Fransa",        flag: "🇫🇷", code: "33",  format: "X XX XX XX XX" },
  "34":  { name: "İspanya",       flag: "🇪🇸", code: "34",  format: "XXX XXX XXX" },
  "36":  { name: "Macaristan",    flag: "🇭🇺", code: "36",  format: "XX XXX XXXX" },
  "39":  { name: "İtalya",        flag: "🇮🇹", code: "39",  format: "XXX XXXX XXXX" },
  "40":  { name: "Romanya",       flag: "🇷🇴", code: "40",  format: "XXX XXX XXX" },
  "41":  { name: "İsviçre",       flag: "🇨🇭", code: "41",  format: "XX XXX XXXX" },
  "43":  { name: "Avusturya",     flag: "🇦🇹", code: "43",  format: "XXX XXXXXXX" },
  "44":  { name: "İngiltere",     flag: "🇬🇧", code: "44",  format: "XXXX XXXXXX" },
  "45":  { name: "Danimarka",     flag: "🇩🇰", code: "45",  format: "XXXX XXXX" },
  "46":  { name: "İsveç",         flag: "🇸🇪", code: "46",  format: "XX XXX XXXX" },
  "47":  { name: "Norveç",        flag: "🇳🇴", code: "47",  format: "XXX XX XXX" },
  "48":  { name: "Polonya",       flag: "🇵🇱", code: "48",  format: "XXX XXX XXX" },
  "49":  { name: "Almanya",       flag: "🇩🇪", code: "49",  format: "XXXX XXXXXXX" },
  "51":  { name: "Peru",          flag: "🇵🇪", code: "51",  format: "XXX XXX XXX" },
  "52":  { name: "Meksika",       flag: "🇲🇽", code: "52",  format: "XX XXXX XXXX" },
  "54":  { name: "Arjantin",      flag: "🇦🇷", code: "54",  format: "XX XXXX XXXX" },
  "55":  { name: "Brezilya",      flag: "🇧🇷", code: "55",  format: "XX XXXXX XXXX" },
  "56":  { name: "Şili",          flag: "🇨🇱", code: "56",  format: "X XXXX XXXX" },
  "57":  { name: "Kolombiya",     flag: "🇨🇴", code: "57",  format: "XXX XXX XXXX" },
  "58":  { name: "Venezuela",     flag: "🇻🇪", code: "58",  format: "XXX XXX XXXX" },
  "60":  { name: "Malezya",       flag: "🇲🇾", code: "60",  format: "XX XXXX XXXX" },
  "61":  { name: "Avustralya",    flag: "🇦🇺", code: "61",  format: "X XXXX XXXX" },
  "62":  { name: "Endonezya",     flag: "🇮🇩", code: "62",  format: "XXX XXXX XXXX" },
  "63":  { name: "Filipinler",    flag: "🇵🇭", code: "63",  format: "XXX XXX XXXX" },
  "64":  { name: "Yeni Zelanda",  flag: "🇳🇿", code: "64",  format: "XX XXX XXXX" },
  "65":  { name: "Singapur",      flag: "🇸🇬", code: "65",  format: "XXXX XXXX" },
  "66":  { name: "Tayland",       flag: "🇹🇭", code: "66",  format: "X XXXX XXXX" },
  "81":  { name: "Japonya",       flag: "🇯🇵", code: "81",  format: "XX XXXX XXXX" },
  "82":  { name: "Güney Kore",    flag: "🇰🇷", code: "82",  format: "XX XXXX XXXX" },
  "84":  { name: "Vietnam",       flag: "🇻🇳", code: "84",  format: "XXX XXX XXXX" },
  "86":  { name: "Çin",           flag: "🇨🇳", code: "86",  format: "XXX XXXX XXXX" },
  "90":  { name: "Türkiye",       flag: "🇹🇷", code: "90",  format: "5XX XXX XX XX" },
  "91":  { name: "Hindistan",     flag: "🇮🇳", code: "91",  format: "XXXXX XXXXX" },
  "92":  { name: "Pakistan",      flag: "🇵🇰", code: "92",  format: "XXX XXXXXXX" },
  "93":  { name: "Afganistan",    flag: "🇦🇫", code: "93",  format: "XX XXX XXXX" },
  "94":  { name: "Sri Lanka",     flag: "🇱🇰", code: "94",  format: "XX XXX XXXX" },
  "95":  { name: "Myanmar",       flag: "🇲🇲", code: "95",  format: "XXX XXX XXXX" },
  "98":  { name: "İran",          flag: "🇮🇷", code: "98",  format: "XXX XXX XXXX" },
  "212": { name: "Fas",           flag: "🇲🇦", code: "212", format: "XXX XXXXXX" },
  "213": { name: "Cezayir",       flag: "🇩🇿", code: "213", format: "XXX XX XX XX" },
  "216": { name: "Tunus",         flag: "🇹🇳", code: "216", format: "XX XXX XXX" },
  "218": { name: "Libya",         flag: "🇱🇾", code: "218", format: "XX XXXXXXX" },
  "234": { name: "Nijerya",       flag: "🇳🇬", code: "234", format: "XXX XXX XXXX" },
  "254": { name: "Kenya",         flag: "🇰🇪", code: "254", format: "XXX XXXXXX" },
  "353": { name: "İrlanda",       flag: "🇮🇪", code: "353", format: "XX XXX XXXX" },
  "380": { name: "Ukrayna",       flag: "🇺🇦", code: "380", format: "XX XXX XXXX" },
  "381": { name: "Sırbistan",     flag: "🇷🇸", code: "381", format: "XX XXX XXXX" },
  "385": { name: "Hırvatistan",   flag: "🇭🇷", code: "385", format: "XX XXX XXXX" },
  "420": { name: "Çek Cumh.",     flag: "🇨🇿", code: "420", format: "XXX XXX XXX" },
  "421": { name: "Slovakya",      flag: "🇸🇰", code: "421", format: "XXX XXX XXX" },
  "994": { name: "Azerbaycan",    flag: "🇦🇿", code: "994", format: "XX XXX XXXX" },
  "995": { name: "Gürcistan",     flag: "🇬🇪", code: "995", format: "XXX XXX XXX" },
  "996": { name: "Kırgızistan",   flag: "🇰🇬", code: "996", format: "XXX XXX XXX" },
  "998": { name: "Özbekistan",    flag: "🇺🇿", code: "998", format: "XX XXX XXXX" },
};

function detectCountry(digits: string): CountryInfo | null {
  if (!digits) return null;
  for (const len of [3, 2, 1]) {
    const prefix = digits.slice(0, len);
    if (COUNTRY_CODES[prefix]) return COUNTRY_CODES[prefix];
  }
  return null;
}

/* ── Iris animation ─────────────────────────────────────────── */

function IrisOrb() {
  return (
    <div className="relative flex items-center justify-center" style={{ width: 200, height: 200 }}>
      {/* Outer ambient glow */}
      <div
        className="absolute inset-0 rounded-full pointer-events-none"
        style={{
          background: "radial-gradient(circle, rgba(0,229,160,0.04) 0%, transparent 70%)",
        }}
      />

      {/* SVG iris rings */}
      <svg
        width="200"
        height="200"
        viewBox="0 0 200 200"
        fill="none"
        aria-hidden="true"
        className="absolute inset-0"
      >
        {/* Outermost ring — slow clockwise */}
        <circle
          cx="100"
          cy="100"
          r="92"
          stroke="rgba(0,229,160,0.12)"
          strokeWidth="1"
          strokeDasharray="12 8"
          style={{ transformOrigin: "100px 100px", animation: "irisOuter 18s linear infinite" }}
        />
        {/* Second ring — counter-clockwise */}
        <circle
          cx="100"
          cy="100"
          r="76"
          stroke="rgba(0,229,160,0.18)"
          strokeWidth="1"
          strokeDasharray="6 10"
          style={{ transformOrigin: "100px 100px", animation: "irisMiddle 12s linear infinite reverse" }}
        />
        {/* Third ring — slow clockwise */}
        <circle
          cx="100"
          cy="100"
          r="60"
          stroke="rgba(0,229,160,0.10)"
          strokeWidth="0.75"
          strokeDasharray="3 14"
          style={{ transformOrigin: "100px 100px", animation: "irisOuter 28s linear infinite" }}
        />
        {/* Static core ring */}
        <circle
          cx="100"
          cy="100"
          r="44"
          stroke="rgba(0,229,160,0.08)"
          strokeWidth="1"
        />
      </svg>

      {/* Core glass orb */}
      <div
        className="relative rounded-full overflow-hidden border"
        style={{
          width: 96,
          height: 96,
          borderColor: "rgba(0,229,160,0.28)",
          boxShadow: "0 0 40px rgba(0,229,160,0.20), 0 0 80px rgba(0,229,160,0.08)",
          flexShrink: 0,
        }}
      >
        {/* Logo fills container */}
        <img
          src="/logo.jpeg"
          alt="Obscura"
          style={{ width: "100%", height: "100%", objectFit: "cover", display: "block", mixBlendMode: "screen" }}
        />
        {/* Subtle highlight overlay */}
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: "radial-gradient(circle at 38% 32%, rgba(0,229,160,0.10) 0%, transparent 60%)",
          }}
        />
      </div>

      {/* Keyframes injected inline */}
      <style>{`
        @keyframes irisOuter {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
        @keyframes irisMiddle {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}

/* ── Loading dots ───────────────────────────────────────────── */

function LoadingDots() {
  return (
    <span className="flex gap-1 items-center">
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className="typing-dot w-1.5 h-1.5 rounded-full bg-[#020208]"
          style={{ animationDelay: `${i * 120}ms` }}
        />
      ))}
    </span>
  );
}

/* ── Page ───────────────────────────────────────────────────── */

export default function LoginPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("phone");
  const [phone, setPhone] = useState("+");
  const [otp, setOtp] = useState<string[]>(Array(OTP_LENGTH).fill(""));
  const [username, setUsername] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [countdown, setCountdown] = useState(0);
  const [zkStatus, setZkStatus] = useState<"idle" | "proving" | "verified" | "failed">("idle");
  const [pendingMnemonic, setPendingMnemonic] = useState<string | null>(null);
  const otpRefs = useRef<(HTMLInputElement | null)[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  /* Countdown timer */
  useEffect(() => {
    if (countdown <= 0) { if (timerRef.current) clearInterval(timerRef.current); return; }
    timerRef.current = setInterval(() => setCountdown((c) => c - 1), 1000);
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, [countdown]);

  /* ESC → go back a step (not during mnemonic — it is mandatory) */
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && step !== "phone" && step !== "mnemonic") {
        setStep(step === "otp" ? "phone" : "otp");
        setError("");
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [step]);

  const doVerify = useCallback(async (otpCode: string, uname?: string) => {
    setError(""); setLoading(true);
    try {
      let identityKey: string;
      const passphrase = `obscura_${phone}_v1`;
      let existingIdentity = await loadIdentity(passphrase).catch(() => null);

      if (!existingIdentity) {
        existingIdentity = await generateIdentity();
        await saveIdentity(existingIdentity, passphrase);
      }
      identityKey = btoa(String.fromCharCode(...Array.from(existingIdentity.dhKeyPair.publicKeyBytes)));

      const data = await api.verifyOTP({
        phone, otp: otpCode,
        username: uname || undefined,
        identity_key: identityKey,
      });
      if ((data.is_new || data.status === "new_user") && !uname) {
        setStep("username");
        setLoading(false);
        return;
      }
      if (!data.token) {
        throw new Error("Sunucu geçerli bir oturum döndürmedi");
      }
      localStorage.setItem("obscura_token", data.token);

      try {
        const preKeyStore = await generatePreKeyStore(existingIdentity);
        const bundle = bundleToUpload(preKeyStore);
        await api.uploadPrekeys(bundle).catch(() => {});
        localStorage.setItem("obscura_prekey_store", JSON.stringify({
          signedPreKey: btoa(String.fromCharCode(...Array.from(preKeyStore.signedPreKey.publicKeyBytes))),
        }));
      } catch {}

      // ZK-ID kanıtı oluştur ve gönder (spec Bölüm 5.2-5.3)
      // Secret client'ta üretilir ve saklanır — backend hiçbir zaman görmez.
      // Yeni kullanıcılar için: mnemonic'ten secret türet (Spec Bölüm 5.2 Adım 7).
      if (!data.zk_id_verified) {
        setZkStatus("proving");
        setLoading(false); // prekey bitti, ZK bağımsız
        try {
          // Yeni kullanıcı ise mnemonic üret ve secret türet.
          // Mevcut kullanıcıda yedekleme zaten yapılmış; eski secret'i koru.
          let secret = localStorage.getItem("obscura_zk_secret");
          const isNew = data.is_new === true;
          let newMnemonic: string | null = null;

          if (!secret) {
            // İlk kayıt: mnemonic üret, secret türet
            newMnemonic = generateMnemonic12();
            secret = await deriveSecretFromMnemonic(newMnemonic);
            localStorage.setItem("obscura_zk_secret", secret);
            localStorage.setItem("obscura_mnemonic_backed_up", "false");
          } else if (isNew && !isMnemonicBackedUp()) {
            // Token yenileme ama yedekleme henüz yapılmamış — mnemonic göster
            newMnemonic = generateMnemonic12();
            const rederived = await deriveSecretFromMnemonic(newMnemonic);
            localStorage.setItem("obscura_zk_secret", rederived);
            localStorage.setItem("obscura_mnemonic_backed_up", "false");
            secret = rederived;
          }

          const did = data.user?.did ?? "";
          // DID string'ini hex'e çevir (identity_proof circuit: did_hash BigInt bekler)
          const didHashHex = Array.from(new TextEncoder().encode(did))
            .map((b) => b.toString(16).padStart(2, "0"))
            .join("")
            .slice(0, 64)
            .padStart(64, "0");
          const zkProof = await proveIdentity({
            privateKeyHex: secret.slice(0, 64).padStart(64, "0"),
            didHashHex,
            userDID: did,
          });
          await api.updateZkId(zkProof.proof, zkProof.publicSignals);
          setZkStatus("verified");

          // Yeni kullanıcı: mnemonic yedekleme ekranını göster
          if (newMnemonic) {
            setPendingMnemonic(newMnemonic);
            setStep("mnemonic");
            return;
          }
        } catch (zkErr) {
          // ZK kanıt hatası login'i engellememeli — sadece bilgilendirme
          console.warn("ZK-ID kanıtı gönderilemedi:", zkErr);
          setZkStatus("failed");
        }
        // ZK bitti — chats'e git
        router.replace("/chats");
        return;
      }

      // Mevcut kullanıcı ama mnemonic hiç yedeklenmemiş — üret ve göster
      if (!isMnemonicBackedUp() && !localStorage.getItem("obscura_zk_secret")) {
        const newMnemonic = generateMnemonic12();
        const secret = await deriveSecretFromMnemonic(newMnemonic);
        localStorage.setItem("obscura_zk_secret", secret);
        localStorage.setItem("obscura_mnemonic_backed_up", "false");
        setPendingMnemonic(newMnemonic);
        setStep("mnemonic");
        return;
      }

      router.replace("/chats");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Bir hata oluştu");
      if (step === "otp") {
        setOtp(Array(OTP_LENGTH).fill(""));
        setTimeout(() => otpRefs.current[0]?.focus(), 50);
      }
    } finally { setLoading(false); }
  }, [phone, step, router]);

  const sendOTP = useCallback(async () => {
    if (phone.length < 8) { setError("Geçerli bir telefon numarası girin"); return; }
    setError(""); setLoading(true);
    try {
      const res = await api.requestOTP(phone);
      setStep("otp");
      setCountdown(60);
      if (res?.dev_otp && typeof res.dev_otp === "string" && res.dev_otp.length === OTP_LENGTH) {
        setOtp(res.dev_otp.split(""));
        setTimeout(() => doVerify(res.dev_otp), 300);
      } else {
        setTimeout(() => otpRefs.current[0]?.focus(), 100);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Bir hata oluştu");
    } finally { setLoading(false); }
  }, [phone, doVerify]);

  const verifyOTP = useCallback(() => {
    const code = otp.join("");
    if (code.length !== OTP_LENGTH) { setError("Kodu eksiksiz girin"); return; }
    doVerify(code);
  }, [otp, doVerify]);

  const finalizeUsername = useCallback(() => {
    if (username.length < 3) { setError("En az 3 karakter"); return; }
    doVerify(otp.join(""), username);
  }, [username, otp, doVerify]);

  const handleOTPInput = (idx: number, val: string) => {
    const digit = val.replace(/\D/g, "").slice(-1);
    const next = [...otp]; next[idx] = digit; setOtp(next);
    if (digit && idx < OTP_LENGTH - 1) otpRefs.current[idx + 1]?.focus();
    if (next.every((d) => d) && idx === OTP_LENGTH - 1) {
      setTimeout(() => doVerify(next.join("")), 80);
    }
  };

  const handleOTPKey = (idx: number, e: React.KeyboardEvent) => {
    if (e.key === "Backspace" && !otp[idx] && idx > 0) otpRefs.current[idx - 1]?.focus();
    if (e.key === "Enter" && otp.every((d) => d)) verifyOTP();
  };

  const handleOTPPaste = (e: React.ClipboardEvent) => {
    const paste = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, OTP_LENGTH);
    if (paste.length === OTP_LENGTH) {
      setOtp(paste.split(""));
      otpRefs.current[OTP_LENGTH - 1]?.focus();
    }
  };

  return (
    <div className="fixed inset-0 void-bg">
      {/* Background ambient — full viewport */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          background: "radial-gradient(ellipse 80% 60% at 50% -10%, rgba(0,229,160,0.04) 0%, transparent 70%)",
        }}
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute bottom-0 left-0 right-0 h-64"
        style={{
          background: "radial-gradient(ellipse 100% 100% at 50% 100%, rgba(77,168,255,0.03) 0%, transparent 70%)",
        }}
      />

      {/* Centered column — margin auto, classic centering */}
      <div
        className="relative flex flex-col overflow-hidden"
        style={{ height: "100%", width: "100%", maxWidth: 480, marginLeft: "auto", marginRight: "auto" }}
      >

      {/* Back button — mnemonic adımında gösterme (zorunlu akış) */}
      {step !== "phone" && step !== "mnemonic" && (
        <button
          onClick={() => { setStep(step === "otp" ? "phone" : "otp"); setError(""); }}
          aria-label="Geri"
          className={cn(
            "absolute top-safe left-4 z-10 btn-icon",
            "animate-in stagger-1",
          )}
          style={{ marginTop: "calc(max(env(safe-area-inset-top), 12px) + 8px)" }}
        >
          <ChevronLeft size={20} />
        </button>
      )}

      {/* Mnemonic yedekleme overlay — yeni kullanıcılar için zorunlu */}
      {step === "mnemonic" && pendingMnemonic && (
        <MnemonicBackup
          mnemonic={pendingMnemonic}
          onComplete={() => {
            setPendingMnemonic(null);
            router.replace("/chats");
          }}
        />
      )}

      {/* ── Hero region (top ~45%) ── */}
      <div style={{
        display: "flex", flexDirection: "column",
        alignItems: "center", justifyContent: "flex-end",
        flex: "0 0 45%", paddingBottom: 24, paddingLeft: 24, paddingRight: 24,
      }}>
        <IrisOrb />

        {/* Wordmark */}
        <div style={{ marginTop: 16, textAlign: "center" }}>
          <h1 style={{
            fontFamily: "var(--font-display)", fontSize: 32, fontWeight: 800,
            letterSpacing: "-0.04em", color: "var(--text-1)", marginBottom: 6,
          }}>
            OBSCURA
          </h1>
          <p style={{ color: "var(--text-2)", fontSize: 14 }}>
            Sıfır bilgi. Tam gizlilik.
          </p>
        </div>
      </div>

      {/* ── Form sheet (bottom ~55%) ── */}
      <div style={{
        display: "flex", flexDirection: "column",
        flex: 1, paddingLeft: 24, paddingRight: 24,
        paddingTop: 32, overflowY: "auto",
      }}>
        <div style={{ width: "100%", maxWidth: 360, marginLeft: "auto", marginRight: "auto" }}>

          {/* ── Phone step ── */}
          {step === "phone" && (
            <div className="animate-in stagger-1 space-y-4">
              <div className="text-center mb-6">
                <h2
                  className="font-semibold text-[var(--text-1)] mb-1"
                  style={{ fontFamily: "var(--font-display)", fontSize: 20, letterSpacing: "-0.02em" }}
                >
                  Giriş yap
                </h2>
                <p className="text-[13px]" style={{ color: "var(--text-2)" }}>
                  Numara yalnızca doğrulama için kullanılır
                </p>
              </div>

              {/* Phone input */}
              {(() => {
                const digits = phone.replace(/^\+/, "");
                const country = detectCountry(digits);
                const displayValue = formatPhoneDisplay(digits);
                return (
                  <div className="space-y-3">
                    <div className="relative">
                      {/* "+" prefix */}
                      <div
                        className="absolute left-4 top-1/2 -translate-y-1/2 text-sm font-bold select-none pointer-events-none"
                        style={{ color: "var(--accent)", fontFamily: "var(--font-mono)" }}
                      >
                        +
                      </div>
                      <input
                        type="tel"
                        value={displayValue}
                        onChange={(e) => {
                          setError("");
                          const raw = e.target.value.replace(/\D/g, "");
                          setPhone("+" + raw);
                        }}
                        onKeyDown={(e) => e.key === "Enter" && sendOTP()}
                        placeholder=""
                        aria-label="Telefon numarası"
                        className="field pl-8 text-base tracking-wider h-14"
                        autoFocus
                        autoComplete="tel"
                        inputMode="tel"
                      />
                    </div>

                    {/* Country badge + format — centered below input */}
                    {country && (
                      <div className="flex flex-col items-center gap-1.5 animate-in">
                        <div
                          className="flex items-center gap-1.5 px-3 py-1 rounded-full"
                          style={{
                            background: "rgba(0,229,160,0.08)",
                            border: "1px solid rgba(0,229,160,0.18)",
                          }}
                        >
                          <span style={{ fontSize: 15, lineHeight: 1 }}>{country.flag}</span>
                          <span
                            className="text-[11px] font-semibold"
                            style={{ color: "var(--accent)", fontFamily: "var(--font-display)" }}
                          >
                            {country.name}
                          </span>
                        </div>
                        <p
                          className="text-[11px]"
                          style={{ color: "var(--text-3)", fontFamily: "var(--font-mono)", letterSpacing: "0.05em" }}
                        >
                          +{country.code} {country.format}
                        </p>
                      </div>
                    )}
                  </div>
                );
              })()}

              {error && (
                <p role="alert" className="text-[var(--error)] text-xs text-center animate-in">
                  {error}
                </p>
              )}

              <button
                onClick={sendOTP}
                disabled={loading || phone.length < 8}
                className="btn-primary w-full"
              >
                {loading ? <LoadingDots /> : <><span>Kod gönder</span><ArrowRight size={16} /></>}
              </button>

              {/* Security note */}
              <div
                className="flex items-center justify-center gap-2 mt-4 text-xs"
                style={{ color: "var(--text-3)" }}
              >
                <Lock size={11} />
                <span>Uçtan Uca Şifreli</span>
              </div>

              {/* ZK-ID durum göstergesi */}
              {zkStatus === "proving" && (
                <div
                  className="flex items-center justify-center gap-2 mt-2 text-xs animate-in"
                  style={{ color: "var(--accent)" }}
                >
                  <LoadingDots />
                  <span>ZK kimliği doğrulanıyor…</span>
                </div>
              )}
              {zkStatus === "verified" && (
                <div
                  className="flex items-center justify-center gap-2 mt-2 text-xs animate-in"
                  style={{ color: "var(--status)" }}
                >
                  <ShieldCheck size={12} />
                  <span>ZK kimliği doğrulandı</span>
                </div>
              )}
              {zkStatus === "failed" && (
                <div
                  className="flex items-center justify-center gap-2 mt-2 text-xs animate-in"
                  style={{ color: "var(--text-3)" }}
                >
                  <span>ZK kimliği daha sonra doğrulanabilir</span>
                </div>
              )}
            </div>
          )}

          {/* ── OTP step ── */}
          {step === "otp" && (
            <div className="animate-in stagger-1 space-y-4">
              <div className="text-center mb-6">
                <h2
                  className="font-semibold text-[var(--text-1)] mb-1"
                  style={{ fontFamily: "var(--font-display)", fontSize: 20, letterSpacing: "-0.02em" }}
                >
                  Kodu girin
                </h2>
                <p className="text-[13px]" style={{ color: "var(--text-2)" }}>
                  <span style={{ color: "var(--text-1)" }}>{phone}</span> numarasına SMS gönderildi
                </p>
              </div>

              {/* OTP boxes */}
              <div
                className="flex gap-2.5 justify-center"
                onPaste={handleOTPPaste}
                role="group"
                aria-label="Doğrulama kodu"
              >
                {otp.map((digit, idx) => (
                  <input
                    key={idx}
                    ref={(el) => { otpRefs.current[idx] = el; }}
                    type="text"
                    inputMode="numeric"
                    maxLength={1}
                    value={digit}
                    onChange={(e) => handleOTPInput(idx, e.target.value)}
                    onKeyDown={(e) => handleOTPKey(idx, e)}
                    aria-label={`Hane ${idx + 1}`}
                    className={cn(
                      "w-12 h-14 text-center text-xl font-semibold rounded-2xl",
                      "focus:outline-none transition-all duration-200",
                      "no-select",
                    )}
                    style={{
                      background: digit ? "rgba(0,229,160,0.06)" : "var(--surface-2)",
                      border: `1px solid ${digit ? "rgba(0,229,160,0.4)" : "var(--border-1)"}`,
                      color: "var(--text-1)",
                      boxShadow: digit ? "0 0 0 3px rgba(0,229,160,0.06)" : undefined,
                      fontFamily: "var(--font-mono)",
                    }}
                  />
                ))}
              </div>

              {error && (
                <p role="alert" className="text-[var(--error)] text-xs text-center animate-in">
                  {error}
                </p>
              )}

              <button
                onClick={verifyOTP}
                disabled={loading || otp.some((d) => !d)}
                className="btn-primary w-full"
              >
                {loading ? <LoadingDots /> : "Doğrula"}
              </button>

              <div className="text-center text-xs">
                {countdown > 0 ? (
                  <p style={{ color: "var(--text-3)" }}>
                    Tekrar gönder:{" "}
                    <span
                      style={{ color: "var(--text-2)", fontFamily: "var(--font-mono)" }}
                    >
                      {countdown}s
                    </span>
                  </p>
                ) : (
                  <button
                    onClick={sendOTP}
                    className="transition-colors duration-150 hover:opacity-80"
                    style={{ color: "var(--accent)" }}
                  >
                    Kodu tekrar gönder
                  </button>
                )}
              </div>
            </div>
          )}

          {/* ── Username step ── */}
          {step === "username" && (
            <div className="animate-in stagger-1 space-y-4">
              <div className="text-center mb-6">
                {/* Accent circle */}
                <div
                  className="w-14 h-14 rounded-full flex items-center justify-center mx-auto mb-4"
                  style={{
                    background: "rgba(0,229,160,0.08)",
                    border: "1px solid rgba(0,229,160,0.2)",
                  }}
                >
                  <span
                    style={{ fontSize: 26, fontFamily: "var(--font-display)", color: "var(--accent)" }}
                  >
                    @
                  </span>
                </div>
                <h2
                  className="font-semibold text-[var(--text-1)] mb-1"
                  style={{ fontFamily: "var(--font-display)", fontSize: 20, letterSpacing: "-0.02em" }}
                >
                  Kullanıcı adı seç
                </h2>
                <p className="text-[13px]" style={{ color: "var(--text-2)" }}>
                  Başkalarının sizi bulacağı isim
                </p>
              </div>

              <div className="relative">
                <span
                  className="absolute left-4 top-1/2 -translate-y-1/2 text-sm pointer-events-none select-none"
                  style={{ color: "var(--text-3)", fontFamily: "var(--font-mono)" }}
                >
                  @
                </span>
                <input
                  type="text"
                  value={username}
                  onChange={(e) => {
                    setError("");
                    setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, ""));
                  }}
                  onKeyDown={(e) => e.key === "Enter" && finalizeUsername()}
                  placeholder="kullanici_adi"
                  aria-label="Kullanıcı adı"
                  className="field pl-8"
                  autoFocus
                  minLength={3}
                  maxLength={32}
                />
              </div>

              {error && (
                <p role="alert" className="text-[var(--error)] text-xs animate-in">
                  {error}
                </p>
              )}

              <button
                onClick={finalizeUsername}
                disabled={loading || username.length < 3}
                className="btn-primary w-full"
              >
                {loading ? <LoadingDots /> : <><span>Hesabı oluştur</span><ArrowRight size={16} /></>}
              </button>
            </div>
          )}

        </div>

        {/* Bottom spacer for safe area */}
        <div style={{ paddingBottom: "calc(max(env(safe-area-inset-bottom), 16px) + 16px)" }} />
      </div>

      </div>{/* end centered container */}
    </div>
  );
}
