"use client";

import { useState, useEffect, useCallback } from "react";
import { ShieldAlert, Loader2, CheckCircle2, Info } from "lucide-react";
import { api } from "@/lib/api";
import { AppShell } from "@/components/AppShell";
import { resolveMarketplaceDispute, type ResolveDisputeResult } from "@/lib/marketplace-api";

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

// B9 parça 3 — admin marketplace dispute çözüm ekranı.
//
// Admin tespiti: /v1/users/me hiçbir zaman is_admin döndürmüyor (backend
// isAdminDID sadece OBSCURA_ADMIN_DIDS env'ini kontrol ediyor, HTTP
// katmanında yalnızca AdminMiddleware'de) — yeni bir endpoint eklemeden
// (guardrail) admin durumu, zaten admin-gated olan mevcut
// api.adminListReviewQueue ile SESSİZ PROBE edilir. Başarısızsa (403) form
// hiç render edilmez — admin/review/page.tsx'in "backend 403'ü göster"
// deseninden daha katı, bu ekranın kendi görevi (para hareketi) gereği.
//
// Önizleme YOK: GetDispute buyer/seller-only (marketplace_handlers.go:289,
// ErrNotParticipant) — admin bu id'yi görüntüleyemez, yeni bir
// admin-görüntüleme endpoint'i kapsam dışı. Admin dispute ID'yi (ve
// bağlamı) dış kanaldan bilir, bu ekran sadece kararı uygular.
export default function AdminMarketplaceDisputesPage() {
  const [adminCheck, setAdminCheck] = useState<"checking" | "admin" | "denied">("checking");
  const [disputeId, setDisputeId] = useState("");
  const [upheld, setUpheld] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<ResolveDisputeResult | null>(null);

  useEffect(() => {
    api.adminListReviewQueue({ limit: 1 })
      .then(() => setAdminCheck("admin"))
      .catch(() => setAdminCheck("denied"));
  }, []);

  const canSubmit = disputeId.trim().length > 0 && upheld !== null && !busy;

  const handleResolve = useCallback(async () => {
    if (!canSubmit || upheld === null) return;
    setBusy(true);
    setError("");
    setResult(null);
    try {
      const r = await resolveMarketplaceDispute(disputeId.trim(), upheld);
      setResult(r);
      setDisputeId("");
      setUpheld(null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Dispute çözülemedi");
    } finally {
      setBusy(false);
    }
  }, [canSubmit, disputeId, upheld]);

  if (adminCheck === "checking") {
    return (
      <AppShell showBack title="Dispute Çözümü">
        <div className="flex-1 flex items-center justify-center h-full">
          <Loader2 size={28} className="animate-spin" style={{ color: "var(--color-accent)" }} />
        </div>
      </AppShell>
    );
  }

  if (adminCheck === "denied") {
    return (
      <AppShell showBack title="Dispute Çözümü">
        <div className="flex-1 flex flex-col items-center justify-center gap-3 h-full px-6 text-center">
          <ShieldAlert size={36} style={{ color: "var(--text-3)" }} />
          <p style={{ color: "var(--text-3)" }}>Bu ekran için yönetici yetkisi gerekli</p>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="Dispute Çözümü">
      <div className="flex-1 overflow-y-auto scroll-area p-5">
        <div className="flex items-center gap-2 mb-2">
          <ShieldAlert size={18} style={{ color: "var(--color-accent)" }} />
          <h1 className="text-[17px] font-bold" style={{ color: "var(--text-1)" }}>Marketplace Dispute Çöz</h1>
        </div>

        <div className="flex items-start gap-2 mb-5 p-3 rounded-2xl" style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}>
          <Info size={14} style={{ color: "var(--text-3)", marginTop: 2, flexShrink: 0 }} />
          <p className="text-[11px] leading-relaxed" style={{ color: "var(--text-3)" }}>
            Bu ekran dispute'u ÖNİZLEYEMEZ (buyer/seller-only bir uç noktaya bağlı) —
            dispute ID'yi ve bağlamı dış kanaldan (destek talebi vb.) bilmeniz gerekir.
          </p>
        </div>

        <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Dispute ID</label>
        <input
          value={disputeId}
          onChange={(e) => setDisputeId(e.target.value)}
          placeholder="uuid..."
          className="w-full h-12 mt-1.5 mb-4 px-4 rounded-2xl text-[14px] font-mono outline-none"
          style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", color: "var(--text-1)" }}
        />

        <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Karar</label>
        <div className="flex gap-2 mt-1.5 mb-5">
          <button
            onClick={() => setUpheld(true)}
            className="flex-1 py-3 rounded-2xl text-[13px] font-semibold"
            style={{
              background: upheld === true ? "rgba(239,68,68,0.1)" : "var(--surface-2)",
              border: `1.5px solid ${upheld === true ? "var(--error)" : "var(--border-1)"}`,
              color: upheld === true ? "var(--error)" : "var(--text-2)",
            }}
          >
            Alıcı haklı — iade et
          </button>
          <button
            onClick={() => setUpheld(false)}
            className="flex-1 py-3 rounded-2xl text-[13px] font-semibold"
            style={{
              background: upheld === false ? "var(--color-accent-deep)" : "var(--surface-2)",
              border: `1.5px solid ${upheld === false ? "var(--color-accent-dim)" : "var(--border-1)"}`,
              color: upheld === false ? "var(--color-accent)" : "var(--text-2)",
            }}
          >
            Satış geçerli — satıcıya öde
          </button>
        </div>

        {error && (
          <div className="mb-4 badge badge-error" style={{ height: "auto", padding: "8px 12px" }}>{error}</div>
        )}

        {result && (
          <div className="mb-4 p-4 rounded-2xl flex items-start gap-2" style={{ background: "rgba(0,229,160,0.06)", border: "1px solid var(--color-accent-dim)" }}>
            <CheckCircle2 size={16} style={{ color: "var(--color-accent)", marginTop: 1, flexShrink: 0 }} />
            <div className="text-[12px] leading-relaxed" style={{ color: "var(--text-2)" }}>
              <p className="font-semibold mb-1" style={{ color: "var(--color-accent)" }}>
                Çözüldü — {result.upheld ? "alıcıya iade edildi" : "satıcıya ödendi"}
              </p>
              <p>Tutar: {fmtPrice(result.amount)} OBS · Alan: <code>{result.paid_to.slice(0, 20)}…</code></p>
              <p>Transaction: <code>{result.transaction_id}</code></p>
            </div>
          </div>
        )}

        <button
          onClick={handleResolve}
          disabled={!canSubmit}
          className="w-full h-12 rounded-full font-bold text-[14px] flex items-center justify-center"
          style={{
            background: canSubmit ? "var(--color-accent)" : "var(--surface-3)",
            color: canSubmit ? "var(--color-void)" : "var(--text-3)",
          }}
        >
          {busy ? <Loader2 size={18} className="animate-spin" /> : "Kararı Uygula"}
        </button>
      </div>
    </AppShell>
  );
}
