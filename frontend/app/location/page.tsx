"use client";

import { useState } from "react";
import { MapPin, Shield, CheckCircle, XCircle, Loader2, Navigation } from "lucide-react";
import { api } from "@/lib/api";
import { proveLocation } from "@/lib/zk";
import { AppShell } from "@/components/AppShell";

interface GPSCoords {
  lat: number;
  lon: number;
  accuracy: number;
}

interface VerifyResult {
  verified: boolean;
  in_region: boolean;
}

export default function LocationPage() {
  const [coords, setCoords] = useState<GPSCoords | null>(null);
  const [status, setStatus] = useState<"idle" | "gps" | "proving" | "verifying" | "done" | "error">("idle");
  const [result, setResult] = useState<VerifyResult | null>(null);
  const [error, setError] = useState("");

  // Bölge: kullanıcının konumunun ±0.5 derece yarıçapı (yaklaşık 55km)
  const REGION_RADIUS = 0.5;

  async function handleCheckIn() {
    setError("");
    setResult(null);

    // 1. GPS al
    setStatus("gps");
    let position: GeolocationPosition;
    try {
      position = await new Promise<GeolocationPosition>((resolve, reject) =>
        navigator.geolocation.getCurrentPosition(resolve, reject, {
          enableHighAccuracy: true,
          timeout: 15000,
          maximumAge: 0,
        })
      );
    } catch (e: any) {
      setError("GPS erişimi reddedildi veya zaman aşımı: " + (e.message || e.code));
      setStatus("error");
      return;
    }

    const lat = position.coords.latitude;
    const lon = position.coords.longitude;
    const accuracy = position.coords.accuracy;
    setCoords({ lat, lon, accuracy });

    // 2. ZK kanıtı üret
    setStatus("proving");
    const latRaw = Math.round(lat * 1e6);
    const lonRaw = Math.round(lon * 1e6);
    const regionLatMin = Math.round((lat - REGION_RADIUS) * 1e6);
    const regionLatMax = Math.round((lat + REGION_RADIUS) * 1e6);
    const regionLonMin = Math.round((lon - REGION_RADIUS) * 1e6);
    const regionLonMax = Math.round((lon + REGION_RADIUS) * 1e6);
    // Cihaz gizli anahtarı — localStorage'dan al veya yeni üret
    const existingSecret = localStorage.getItem("obscura_device_secret");
    const secretBytes = existingSecret
      ? BigInt("0x" + existingSecret)
      : (() => {
          const b = crypto.getRandomValues(new Uint8Array(31));
          const hex = Array.from(b).map(x => x.toString(16).padStart(2, "0")).join("");
          localStorage.setItem("obscura_device_secret", hex);
          return BigInt("0x" + hex);
        })();

    let proof: any;
    try {
      proof = await proveLocation({
        latRaw,
        lonRaw,
        regionLatMin,
        regionLatMax,
        regionLonMin,
        regionLonMax,
        deviceSecret: secretBytes,
      });
    } catch (e: any) {
      setError("ZK kanıtı üretilemedi: " + e.message);
      setStatus("error");
      return;
    }

    // 3. Backend'e gönder
    setStatus("verifying");
    try {
      const res = await api.locationVerify({
        proof_json: JSON.stringify(proof.proof),
        public_inputs: proof.publicSignals,
      });
      setResult(res as VerifyResult);
      setStatus("done");
    } catch (e: any) {
      setError("Doğrulama başarısız: " + e.message);
      setStatus("error");
    }
  }

  const isLoading = ["gps", "proving", "verifying"].includes(status);

  return (
    <AppShell>
    <div className="flex flex-col h-full scroll-area pb-24">

      {/* ── Header ── */}
      <div className="page-header px-5">
        <div className="flex items-center gap-3">
          <div
            className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0"
            style={{ background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.2)" }}
          >
            <MapPin size={18} style={{ color: "var(--accent)" }} />
          </div>
          <div>
            <h1 className="page-title">Konum Kanıtı</h1>
            <p className="text-xs" style={{ color: "var(--text-3)" }}>GPS · ZK Proof · Anonim check-in</p>
          </div>
        </div>
      </div>

      <div className="px-5 space-y-4">

        {/* Açıklama */}
        <div className="rounded-2xl p-4 space-y-2"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
          <div className="flex items-start gap-2">
            <Shield size={15} className="flex-shrink-0 mt-0.5" style={{ color: "var(--accent)" }} />
            <p className="text-sm" style={{ color: "var(--text-2)" }}>
              Tam konumun açıklanmadan "belirli bir bölgedesin" kanıtı üretilir.
              Koordinatlar yalnızca cihazında kalır.
            </p>
          </div>
        </div>

        {/* GPS Koordinatları */}
        {coords && (
          <div className="rounded-2xl p-4"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
            <p className="section-label mb-3">Tespit Edilen Konum</p>
            <div className="grid grid-cols-2 gap-3">
              {[
                { label: "Enlem",          val: coords.lat.toFixed(6) },
                { label: "Boylam",         val: coords.lon.toFixed(6) },
                { label: "Doğruluk",       val: `±${Math.round(coords.accuracy)}m` },
                { label: "Bölge Yarıçapı", val: "~55km" },
              ].map(({ label, val }) => (
                <div key={label}>
                  <p className="text-[10px] uppercase tracking-wide mb-0.5" style={{ color: "var(--text-3)" }}>{label}</p>
                  <p className="font-mono text-sm font-semibold" style={{ color: "var(--text-1)" }}>{val}</p>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Durum adımları */}
        {status !== "idle" && (
          <div className="space-y-2">
            {[
              { key: "gps",       label: "GPS konumu alınıyor" },
              { key: "proving",   label: "ZK kanıtı üretiliyor (Groth16)" },
              { key: "verifying", label: "Backend doğrulaması" },
            ].map(({ key, label }) => {
              const steps = ["gps", "proving", "verifying", "done", "error"];
              const currentIdx = steps.indexOf(status);
              const stepIdx = steps.indexOf(key);
              const isDone = currentIdx > stepIdx && status !== "error";
              const isActive = status === key;
              const isFailed = status === "error" && isActive;

              return (
                <div key={key} className="flex items-center gap-3 text-sm">
                  {isDone ? (
                    <CheckCircle size={16} className="flex-shrink-0" style={{ color: "var(--accent)" }} />
                  ) : isActive ? (
                    <Loader2 size={16} className="animate-spin flex-shrink-0" style={{ color: "var(--accent)" }} />
                  ) : isFailed ? (
                    <XCircle size={16} className="flex-shrink-0" style={{ color: "var(--error)" }} />
                  ) : (
                    <div className="w-4 h-4 rounded-full flex-shrink-0" style={{ border: "1px solid var(--border-2)" }} />
                  )}
                  <span style={{ color: isActive ? "var(--text-1)" : "var(--text-3)" }}>
                    {label}
                  </span>
                </div>
              );
            })}
          </div>
        )}

        {/* Sonuç */}
        {status === "done" && result && (
          <div
            className="rounded-2xl p-5 flex items-center gap-4"
            style={result.in_region
              ? { background: "var(--accent-muted)", border: "1px solid rgba(0,229,160,0.25)" }
              : { background: "rgba(255,170,0,0.08)", border: "1px solid rgba(255,170,0,0.25)" }}
          >
            {result.in_region ? (
              <CheckCircle size={28} style={{ color: "var(--accent)", flexShrink: 0 }} />
            ) : (
              <XCircle size={28} style={{ color: "var(--warning)", flexShrink: 0 }} />
            )}
            <div>
              <p className="font-semibold text-sm" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
                {result.in_region ? "Bölge Doğrulandı" : "Bölge Dışı"}
              </p>
              <p className="text-xs mt-0.5" style={{ color: "var(--text-2)" }}>
                {result.in_region
                  ? "ZK kanıtı doğrulandı. Hedef bölgedesin."
                  : "Konumun belirtilen bölge dışında."}
              </p>
            </div>
          </div>
        )}

        {/* Hata */}
        {status === "error" && error && (
          <div className="rounded-2xl px-4 py-3 text-sm"
            style={{ background: "rgba(255,64,88,0.06)", border: "1px solid rgba(255,64,88,0.18)", color: "var(--error)" }}>
            {error}
          </div>
        )}

        {/* Check-in butonu */}
        <button
          onClick={handleCheckIn}
          disabled={isLoading}
          className="btn-primary w-full"
        >
          {isLoading ? (
            <Loader2 size={18} className="animate-spin" />
          ) : (
            <Navigation size={18} />
          )}
          {isLoading ? "İşleniyor..." : "ZK Check-in Yap"}
        </button>

        {status === "done" && (
          <button
            onClick={() => { setStatus("idle"); setCoords(null); setResult(null); }}
            className="btn-secondary w-full"
          >
            Yeniden Dene
          </button>
        )}
      </div>
    </div>
    </AppShell>
  );
}
