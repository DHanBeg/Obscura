"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Shield, ChevronLeft } from "lucide-react";
import { cn } from "@/lib/cn";
import { api } from "@/lib/api";
import { ObscuraLogo } from "@/components/ui/ObscuraLogo";
import {
  generateIdentity,
  generatePreKeyStore,
  bundleToUpload,
  saveIdentity,
  loadIdentity,
} from "@/lib/e2ee";

type Step = "phone" | "otp" | "username";
const OTP_LENGTH = 6;

export default function LoginPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("phone");
  const [phone, setPhone] = useState("+90");
  const [otp, setOtp] = useState<string[]>(Array(OTP_LENGTH).fill(""));
  const [username, setUsername] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [countdown, setCountdown] = useState(0);
  const [isNewUser, setIsNewUser] = useState(false);
  const otpRefs = useRef<(HTMLInputElement | null)[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (countdown <= 0) { if (timerRef.current) clearInterval(timerRef.current); return; }
    timerRef.current = setInterval(() => setCountdown((c) => c - 1), 1000);
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, [countdown]);

  const sendOTP = useCallback(async () => {
    if (phone.length < 10) { setError("Geçerli bir numara girin"); return; }
    setError(""); setLoading(true);
    try {
      await api.requestOTP(phone);
      setStep("otp");
      setCountdown(60);
      setTimeout(() => otpRefs.current[0]?.focus(), 100);
    } catch (e: any) {
      setError(e.message);
    } finally { setLoading(false); }
  }, [phone]);

  const doVerify = useCallback(async (otpCode: string, uname?: string) => {
    setError(""); setLoading(true);
    try {
      // E2EE: Kimlik anahtarı yükle veya oluştur
      let identityKey: string;
      const passphrase = `obscura_${phone}_v1`;
      let existingIdentity = await loadIdentity(passphrase).catch(() => null);

      if (!existingIdentity) {
        // Yeni kimlik oluştur
        existingIdentity = await generateIdentity();
        await saveIdentity(existingIdentity, passphrase);
        // PreKey store oluştur ve sunucuya yükle (giriş sonrası)
      }
      // Base64 encoded identity public key
      identityKey = btoa(String.fromCharCode(...existingIdentity.dhKeyPair.publicKeyBytes));

      const data = await api.verifyOTP({
        phone, otp: otpCode,
        username: uname || undefined,
        identity_key: identityKey,
      });
      if (data.is_new && !uname) {
        setIsNewUser(true);
        setStep("username");
        setLoading(false);
        return;
      }
      localStorage.setItem("obscura_token", data.token);

      // Arka planda prekey store oluştur ve yükle
      try {
        const preKeyStore = await generatePreKeyStore(existingIdentity);
        const bundle = bundleToUpload(preKeyStore);
        await api.uploadPrekeys(bundle).catch(() => {}); // hata olursa devam et
        // OPK'ları localStorage'a kaydet
        localStorage.setItem("obscura_prekey_store", JSON.stringify({
          signedPreKey: btoa(String.fromCharCode(...preKeyStore.signedPreKey.publicKeyBytes)),
        }));
      } catch {}

      router.replace("/chats");
    } catch (e: any) {
      setError(e.message);
      if (step === "otp") { setOtp(Array(OTP_LENGTH).fill("")); setTimeout(() => otpRefs.current[0]?.focus(), 50); }
    } finally { setLoading(false); }
  }, [phone, step, router]);

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
    if (paste.length === OTP_LENGTH) { setOtp(paste.split("")); otpRefs.current[OTP_LENGTH - 1]?.focus(); }
  };

  const LoadingDots = () => (
    <span className="flex gap-1 items-center">
      {[0,1,2].map(i => <span key={i} className="typing-dot w-1.5 h-1.5 rounded-full bg-void" />)}
    </span>
  );

  return (
    <div className="fixed inset-0 flex flex-col void-bg overflow-hidden">
      {/* Ambient glow */}
      <div className="pointer-events-none absolute top-0 left-1/2 -translate-x-1/2 w-96 h-96 rounded-full bg-accent/6 blur-3xl" />

      {/* Back button */}
      {step !== "phone" && (
        <button
          onClick={() => { setStep(step === "otp" ? "phone" : "otp"); setError(""); }}
          className="absolute top-6 left-6 btn-icon z-10 animate-fade-in"
        >
          <ChevronLeft size={20} />
        </button>
      )}

      <div className="flex flex-col items-center justify-center flex-1 px-6">
        {/* Logo */}
        <div className="mb-10 animate-slide-up">
          <ObscuraLogo size={52} animated />
        </div>

        {/* ── Step: Phone ── */}
        {step === "phone" && (
          <div className="w-full max-w-sm animate-slide-up" style={{ animationDelay: "60ms" }}>
            <div className="mb-8 text-center">
              <h1 className="text-2xl font-semibold text-head mb-2">Hoş geldiniz</h1>
              <p className="text-sm text-sub">Numara yalnızca doğrulama için kullanılır</p>
            </div>
            <div className="space-y-3">
              <input
                type="tel"
                value={phone}
                onChange={(e) => { setError(""); let v = e.target.value; if (!v.startsWith("+")) v = "+" + v; setPhone(v); }}
                onKeyDown={(e) => e.key === "Enter" && sendOTP()}
                placeholder="+90 555 000 00 00"
                className="field text-center text-lg tracking-widest h-14"
                autoFocus autoComplete="tel" inputMode="tel"
              />
              {error && <p className="text-red text-xs text-center animate-fade-in">{error}</p>}
              <button onClick={sendOTP} disabled={loading || phone.length < 10} className="btn-primary w-full">
                {loading ? <LoadingDots /> : <><span>Kod gönder</span><ArrowRight size={16} /></>}
              </button>
            </div>
            <div className="mt-8 flex items-center justify-center gap-2 text-dim text-xs">
              <Shield size={12} /><span>Uçtan uca şifreli iletişim</span>
            </div>
          </div>
        )}

        {/* ── Step: OTP ── */}
        {step === "otp" && (
          <div className="w-full max-w-sm animate-slide-up" style={{ animationDelay: "60ms" }}>
            <div className="mb-8 text-center">
              <h1 className="text-2xl font-semibold text-head mb-2">Kodu girin</h1>
              <p className="text-sm text-sub"><span className="text-body">{phone}</span> numarasına gönderildi</p>
            </div>
            <div className="flex gap-2.5 justify-center mb-4" onPaste={handleOTPPaste}>
              {otp.map((digit, idx) => (
                <input
                  key={idx}
                  ref={(el) => { otpRefs.current[idx] = el; }}
                  type="text" inputMode="numeric" maxLength={1} value={digit}
                  onChange={(e) => handleOTPInput(idx, e.target.value)}
                  onKeyDown={(e) => handleOTPKey(idx, e)}
                  className={cn(
                    "w-12 h-14 text-center text-xl font-semibold rounded-2xl bg-raised border text-head",
                    "focus:outline-none transition-all duration-200",
                    digit ? "border-accent/50 bg-accent/5" : "border-border focus:border-accent/40"
                  )}
                />
              ))}
            </div>
            {error && <p className="text-red text-xs text-center mb-3 animate-fade-in">{error}</p>}
            <button onClick={verifyOTP} disabled={loading || otp.some((d) => !d)} className="btn-primary w-full mb-4">
              {loading ? <LoadingDots /> : "Doğrula"}
            </button>
            <div className="text-center">
              {countdown > 0
                ? <p className="text-dim text-xs">Tekrar gönder: <span className="text-sub">{countdown}s</span></p>
                : <button onClick={sendOTP} className="text-accent text-xs hover:text-accent/80 transition-colors">Kodu tekrar gönder</button>
              }
            </div>
          </div>
        )}

        {/* ── Step: Username ── */}
        {step === "username" && (
          <div className="w-full max-w-sm animate-slide-up" style={{ animationDelay: "60ms" }}>
            <div className="mb-8 text-center">
              <div className="w-16 h-16 rounded-full bg-accent/10 border border-accent/20 flex items-center justify-center mx-auto mb-4">
                <span className="text-2xl">👋</span>
              </div>
              <h1 className="text-2xl font-semibold text-head mb-2">Kullanıcı adı seç</h1>
              <p className="text-sm text-sub">Başkalarının sizi bulacağı isim</p>
            </div>
            <div className="space-y-3">
              <div className="relative">
                <span className="absolute left-4 top-1/2 -translate-y-1/2 text-dim text-sm">@</span>
                <input
                  type="text" value={username}
                  onChange={(e) => { setError(""); setUsername(e.target.value.toLowerCase().replace(/[^a-z0-9_.]/g, "")); }}
                  onKeyDown={(e) => e.key === "Enter" && finalizeUsername()}
                  placeholder="kullanici_adi"
                  className="field pl-8" autoFocus minLength={3} maxLength={32}
                />
              </div>
              {error && <p className="text-red text-xs animate-fade-in">{error}</p>}
              <button onClick={finalizeUsername} disabled={loading || username.length < 3} className="btn-primary w-full">
                {loading ? <LoadingDots /> : "Hesabı oluştur →"}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
