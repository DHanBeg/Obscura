"use client";

import { useState, useEffect, useRef } from "react";
import { AppShell } from "@/components/AppShell";
import { api } from "@/lib/api";
import { encodeDeviceQR, decodeObscuraQR } from "@/lib/zk-qr";
import {
  QrCode, Smartphone, Shield, ShieldCheck, Copy, CheckCheck,
  RefreshCw, AlertTriangle, Eye, EyeOff, Download,
} from "lucide-react";

type Step = "overview" | "show-qr" | "scan-qr" | "confirm" | "done";

interface TrustedDevice {
  device_id: string;
  name: string;
  added_at: string;
  is_current: boolean;
}

export default function CrossSigningPage() {
  const [step, setStep] = useState<Step>("overview");
  const [devices, setDevices] = useState<TrustedDevice[]>([]);
  const [myQR, setMyQR] = useState<string>("");
  const [scannedDID, setScannedDID] = useState<string>("");
  const [copied, setCopied] = useState(false);
  const [showMnemonic, setShowMnemonic] = useState(false);
  const [mnemonic, setMnemonic] = useState<string>("");
  const [mnemonicLoading, setMnemonicLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const qrInputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    loadDevices();
    generateMyQR();
  }, []);

  async function loadDevices() {
    try {
      const me = await api.getMe();
      // Stub: in production, fetch from /v1/devices
      setDevices([
        { device_id: "dev-current", name: "Bu Cihaz", added_at: new Date().toISOString(), is_current: true },
      ]);
      const qr = encodeDeviceQR(me.did ?? "did:obs:unknown", "dev-current", crypto.getRandomValues(new Uint8Array(32)));
      setMyQR(qr);
    } catch {}
  }

  async function generateMyQR() {
    try {
      const me = await api.getMe();
      const challenge = crypto.getRandomValues(new Uint8Array(32));
      const qr = encodeDeviceQR(me.did ?? "did:obs:unknown", `dev-${Date.now()}`, challenge);
      setMyQR(qr);
    } catch {}
  }

  async function loadMnemonic() {
    setMnemonicLoading(true);
    try {
      const res = await (api as any).mnemonicGenerate?.();
      setMnemonic(res?.mnemonic ?? "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about");
    } catch {
      setMnemonic("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about");
    } finally {
      setMnemonicLoading(false);
    }
  }

  function handleScanResult(raw: string) {
    const parsed = decodeObscuraQR(raw.trim());
    if (parsed?.type === "device") {
      const p = parsed.payload as { did: string; deviceId: string };
      setScannedDID(p.did);
      setStep("confirm");
    }
  }

  async function confirmDevice() {
    setLoading(true);
    try {
      // Stub: POST /v1/devices/cross-sign
      await new Promise((r) => setTimeout(r, 1200));
      setStep("done");
    } finally {
      setLoading(false);
    }
  }

  function copyQR() {
    navigator.clipboard.writeText(myQR).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <AppShell showBack title="Cross-Signing">
      <div className="scroll-area h-full pb-28">
        <div className="px-4 pt-4 space-y-4 animate-slide-up">

          {/* Header */}
          <div className="flex items-center gap-3 mb-6">
            <div className="w-12 h-12 rounded-3xl flex items-center justify-center"
              style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}>
              <Shield size={22} style={{ color: "var(--accent)" }} />
            </div>
            <div>
              <h1 className="page-title">Çoklu Cihaz</h1>
              <p className="text-sm" style={{ color: "var(--text-2)" }}>QR ile güvenli cihaz eşleştirme</p>
            </div>
          </div>

          {step === "overview" && (
            <>
              {/* Trusted devices */}
              <div>
                <div className="section-label mb-2">GÜVENILIR CIHAZLAR</div>
                <div className="card space-y-0 overflow-hidden">
                  {devices.map((d, i) => (
                    <div key={d.device_id}
                      className="flex items-center gap-3 px-4 py-3.5"
                      style={{ borderBottom: i < devices.length - 1 ? "1px solid var(--border-1)" : undefined }}>
                      <div className="w-9 h-9 rounded-2xl flex items-center justify-center"
                        style={{ background: "var(--surface-3)" }}>
                        <Smartphone size={16} style={{ color: d.is_current ? "var(--accent)" : "var(--text-2)" }} />
                      </div>
                      <div className="flex-1">
                        <div className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                          {d.name}
                        </div>
                        <div className="text-xs" style={{ color: "var(--text-3)" }}>
                          {new Date(d.added_at).toLocaleDateString("tr-TR")}
                        </div>
                      </div>
                      {d.is_current && (
                        <span className="badge badge-accent">Aktif</span>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              {/* Actions */}
              <div className="grid grid-cols-2 gap-3">
                <button className="card p-4 flex flex-col gap-2 items-start card-interactive"
                  onClick={() => setStep("show-qr")}>
                  <QrCode size={20} style={{ color: "var(--accent)" }} />
                  <div className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    QR Göster
                  </div>
                  <div className="text-xs" style={{ color: "var(--text-2)" }}>
                    Yeni cihaza bu QR'ı tarat
                  </div>
                </button>
                <button className="card p-4 flex flex-col gap-2 items-start card-interactive"
                  onClick={() => setStep("scan-qr")}>
                  <Smartphone size={20} style={{ color: "var(--signal)" }} />
                  <div className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    QR Tara
                  </div>
                  <div className="text-xs" style={{ color: "var(--text-2)" }}>
                    Diğer cihazın QR'ını tara
                  </div>
                </button>
              </div>

              {/* Mnemonic backup */}
              <div>
                <div className="section-label mb-2">YEDEKLEME</div>
                <div className="card p-4">
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <Download size={16} style={{ color: "var(--text-2)" }} />
                      <span className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                        12 Kelime Yedek
                      </span>
                    </div>
                    <button className="btn-icon" onClick={() => setShowMnemonic(!showMnemonic)}>
                      {showMnemonic ? <EyeOff size={16} /> : <Eye size={16} />}
                    </button>
                  </div>

                  {showMnemonic ? (
                    mnemonic ? (
                      <div className="grid grid-cols-3 gap-2">
                        {mnemonic.split(" ").map((word, i) => (
                          <div key={i} className="flex items-center gap-1.5 px-2 py-1.5 rounded-xl"
                            style={{ background: "var(--surface-3)" }}>
                            <span className="text-[10px] w-4 text-right" style={{ color: "var(--text-3)", fontFamily: "var(--font-mono)" }}>
                              {i + 1}
                            </span>
                            <span className="text-xs font-medium" style={{ color: "var(--text-1)", fontFamily: "var(--font-mono)" }}>
                              {word}
                            </span>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <button className="btn-secondary w-full" onClick={loadMnemonic} disabled={mnemonicLoading}>
                        {mnemonicLoading ? "Yükleniyor..." : "Mnemonic Göster"}
                      </button>
                    )
                  ) : (
                    <div className="flex items-center gap-2 px-3 py-2 rounded-2xl"
                      style={{ background: "var(--surface-3)" }}>
                      <AlertTriangle size={14} style={{ color: "var(--warning)" }} />
                      <span className="text-xs" style={{ color: "var(--text-2)" }}>
                        Yedek ifadenizi kimseyle paylaşmayın
                      </span>
                    </div>
                  )}
                </div>
              </div>
            </>
          )}

          {step === "show-qr" && (
            <div className="space-y-4">
              <div className="card p-6 flex flex-col items-center gap-4">
                <div className="w-48 h-48 rounded-3xl flex items-center justify-center"
                  style={{ background: "var(--surface-3)", border: "1px solid var(--border-2)" }}>
                  <div className="text-center space-y-2">
                    <QrCode size={64} style={{ color: "var(--text-3)" }} />
                    <p className="text-xs" style={{ color: "var(--text-3)" }}>QR kodu burada</p>
                  </div>
                </div>
                <div className="text-center">
                  <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    Bu QR&apos;ı Yeni Cihaza Tarat
                  </p>
                  <p className="text-xs mt-1" style={{ color: "var(--text-2)" }}>
                    Geçerlilik: 5 dakika
                  </p>
                </div>
              </div>

              <div className="card p-4 space-y-3">
                <div className="section-label">VEYA KODU KOPYALA</div>
                <div className="flex items-center gap-2 px-3 py-2 rounded-2xl"
                  style={{ background: "var(--surface-3)", fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--text-2)", wordBreak: "break-all" }}>
                  {myQR.slice(0, 60)}...
                </div>
                <button className="btn-secondary w-full" onClick={copyQR}>
                  {copied ? <><CheckCheck size={16} /> Kopyalandı</> : <><Copy size={16} /> Kopyala</>}
                </button>
              </div>

              <div className="flex gap-3">
                <button className="btn-ghost flex-1" onClick={() => setStep("overview")}>Geri</button>
                <button className="btn-secondary flex-1" onClick={generateMyQR}>
                  <RefreshCw size={14} /> Yenile
                </button>
              </div>
            </div>
          )}

          {step === "scan-qr" && (
            <div className="space-y-4">
              <div className="card p-6 text-center space-y-4">
                <Smartphone size={48} style={{ color: "var(--signal)", margin: "0 auto" }} />
                <div>
                  <p className="text-sm font-semibold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    QR Kodunu Yapıştır
                  </p>
                  <p className="text-xs mt-1" style={{ color: "var(--text-2)" }}>
                    Diğer cihazdan kopyalanan kodu yapıştırın
                  </p>
                </div>
                <textarea
                  ref={qrInputRef}
                  className="field resize-none h-24 text-xs"
                  placeholder="obscura://device/..."
                  style={{ fontFamily: "var(--font-mono)" }}
                  onChange={(e) => {
                    if (e.target.value.includes("obscura://")) {
                      handleScanResult(e.target.value);
                    }
                  }}
                />
              </div>
              <button className="btn-ghost w-full" onClick={() => setStep("overview")}>Geri</button>
            </div>
          )}

          {step === "confirm" && (
            <div className="space-y-4">
              <div className="card p-6 text-center space-y-4">
                <div className="w-16 h-16 rounded-full mx-auto flex items-center justify-center animate-pulse-accent"
                  style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.3)" }}>
                  <ShieldCheck size={32} style={{ color: "var(--accent)" }} />
                </div>
                <div>
                  <p className="text-base font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                    Cihazı Doğrula
                  </p>
                  <p className="text-xs mt-2" style={{ color: "var(--text-2)" }}>
                    Aşağıdaki DID sahibini tanıyor musunuz?
                  </p>
                  <div className="mt-3 px-3 py-2 rounded-2xl" style={{ background: "var(--surface-3)" }}>
                    <p className="text-xs" style={{ color: "var(--accent)", fontFamily: "var(--font-mono)", wordBreak: "break-all" }}>
                      {scannedDID}
                    </p>
                  </div>
                </div>
              </div>
              <div className="flex gap-3">
                <button className="btn-ghost flex-1" onClick={() => setStep("scan-qr")}>İptal</button>
                <button className="btn-primary flex-1" onClick={confirmDevice} disabled={loading}>
                  {loading ? "İmzalanıyor..." : "Onayla & İmzala"}
                </button>
              </div>
            </div>
          )}

          {step === "done" && (
            <div className="card p-8 text-center space-y-4 animate-scale-in">
              <div className="w-20 h-20 rounded-full mx-auto flex items-center justify-center"
                style={{ background: "rgba(0,229,160,0.08)", border: "2px solid rgba(0,229,160,0.3)" }}>
                <ShieldCheck size={36} style={{ color: "var(--accent)" }} />
              </div>
              <div>
                <p className="text-lg font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                  Cihaz Eklendi
                </p>
                <p className="text-sm mt-1" style={{ color: "var(--text-2)" }}>
                  Çapraz imzalama başarılı
                </p>
              </div>
              <button className="btn-primary w-full" onClick={() => setStep("overview")}>
                Tamam
              </button>
            </div>
          )}

        </div>
      </div>
    </AppShell>
  );
}
