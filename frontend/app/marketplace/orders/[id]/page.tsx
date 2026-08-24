"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import { Loader2, AlertCircle, AlertTriangle } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { useStore } from "@/lib/store";
import { getTransaction, releaseTransaction, openDispute, getDispute, type Transaction, type Dispute } from "@/lib/marketplace-api";

const STATUS_LABELS: Record<string, string> = {
  held: "Beklemede (escrow)", released: "Tamamlandı", refunded: "İade edildi", completed: "Tamamlandı",
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

// #30 — GET /v1/marketplace/disputes/{id} sadece dispute ID biliniyorsa
// çalışır (mobile marketplace-order/[id].tsx ile AYNI gerekçe: backend'de
// transaction→dispute listesi yok, bkz. Faz 1 envanteri). openDispute()
// başarılı olduğunda döndürdüğü id localStorage'a yazılır.
function disputeCacheKey(transactionId: string): string {
  return `obscura_mp_dispute_for_tx_${transactionId}`;
}

export default function MarketplaceOrderDetailPage() {
  const params = useParams<{ id: string }>();
  const { user } = useStore();
  const [txn, setTxn] = useState<Transaction | null>(null);
  const [dispute, setDispute] = useState<Dispute | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [disputeOpen, setDisputeOpen] = useState(false);
  const [disputeReason, setDisputeReason] = useState("");

  const load = useCallback(async () => {
    if (!params.id) return;
    setLoading(true);
    try {
      const t = await getTransaction(params.id);
      setTxn(t);
      setLoadError(false);

      const cachedId = typeof window !== "undefined" ? localStorage.getItem(disputeCacheKey(params.id)) : null;
      if (cachedId) {
        try {
          const d = await getDispute(cachedId);
          setDispute(d);
        } catch { /* stale id — sessizce yok say */ }
      }
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [params.id]);

  useEffect(() => { load(); }, [load]);

  const isBuyer = txn?.buyer_did === user?.did;
  const canAct = txn?.status === "held" && isBuyer && !dispute;

  const handleRelease = useCallback(async () => {
    if (!txn) return;
    if (!confirm("Ödeme satıcıya geçecek. Emin misiniz?")) return;
    setBusy(true);
    setError("");
    try {
      await releaseTransaction(txn.id);
      await load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "İşlem başarısız.");
    } finally {
      setBusy(false);
    }
  }, [txn, load]);

  const handleOpenDispute = useCallback(async () => {
    if (!txn || !disputeReason.trim()) return;
    setBusy(true);
    setError("");
    try {
      const d = await openDispute(txn.id, disputeReason.trim());
      if (typeof window !== "undefined") localStorage.setItem(disputeCacheKey(txn.id), d.id);
      setDispute(d);
      setDisputeOpen(false);
      setDisputeReason("");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Dispute açılamadı.");
    } finally {
      setBusy(false);
    }
  }, [txn, disputeReason]);

  if (loading) {
    return (
      <AppShell showBack title="Sipariş Detayı">
        <div className="flex-1 flex items-center justify-center h-full">
          <Loader2 size={28} className="animate-spin" style={{ color: "var(--color-accent)" }} />
        </div>
      </AppShell>
    );
  }

  if (loadError || !txn) {
    return (
      <AppShell showBack title="Sipariş Detayı">
        <div className="flex-1 flex flex-col items-center justify-center gap-3 h-full">
          <AlertCircle size={36} style={{ color: "var(--text-3)" }} />
          <p style={{ color: "var(--text-3)" }}>Sipariş bulunamadı</p>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="Sipariş Detayı">
      <div className="flex flex-col h-full">
        <div className="flex-1 overflow-y-auto scroll-area p-5">
          <div className="text-center mb-1">
            <span className="text-[28px] font-bold" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
              {fmtPrice(txn.amount)} OBS
            </span>
          </div>
          <p className="text-center text-[13px] font-semibold mb-5" style={{ color: "var(--amber)" }}>
            {STATUS_LABELS[txn.status] || txn.status}
          </p>

          <div className="card p-4 space-y-2">
            <div className="flex justify-between text-[13px]">
              <span style={{ color: "var(--text-3)" }}>Rol</span>
              <span style={{ color: "var(--text-1)" }}>{isBuyer ? "Alıcı" : "Satıcı"}</span>
            </div>
            <div className="flex justify-between text-[13px] gap-3">
              <span style={{ color: "var(--text-3)" }} className="flex-shrink-0">{isBuyer ? "Satıcı" : "Alıcı"}</span>
              <span style={{ color: "var(--text-1)" }} className="truncate">{isBuyer ? txn.seller_did : txn.buyer_did}</span>
            </div>
            <div className="flex justify-between text-[13px]">
              <span style={{ color: "var(--text-3)" }}>Tarih</span>
              <span style={{ color: "var(--text-1)" }}>{new Date(txn.created_at).toLocaleString("tr-TR")}</span>
            </div>
          </div>

          {dispute && (
            <div className="mt-4 p-4 rounded-2xl" style={{ background: "rgba(255,170,0,0.08)", border: "1px solid rgba(255,170,0,0.25)" }}>
              <div className="flex items-center gap-1.5 mb-1">
                <AlertTriangle size={14} style={{ color: "var(--warning)" }} />
                <span className="text-[13px] font-bold" style={{ color: "var(--warning)" }}>
                  Dispute {dispute.status === "open" ? "açık" : "çözüldü"}
                </span>
              </div>
              <p className="text-[13px]" style={{ color: "var(--text-2)" }}>{dispute.reason}</p>
            </div>
          )}

          {error && (
            <div className="mt-4 badge badge-error" style={{ height: "auto", padding: "8px 12px" }}>{error}</div>
          )}

          {disputeOpen && (
            <div className="mt-4 card p-4">
              <textarea
                value={disputeReason}
                onChange={(e) => setDisputeReason(e.target.value)}
                placeholder="Sorunu açıklayın..."
                className="w-full h-24 bg-transparent outline-none text-sm resize-none"
                style={{ color: "var(--text-1)" }}
              />
              <div className="flex gap-2 mt-2">
                <button
                  onClick={() => setDisputeOpen(false)}
                  className="flex-1 h-10 rounded-full text-[13px] font-semibold"
                  style={{ border: "1px solid var(--border-2)", color: "var(--text-3)" }}
                >
                  İptal
                </button>
                <button
                  onClick={handleOpenDispute}
                  disabled={!disputeReason.trim() || busy}
                  className="flex-1 h-10 rounded-full text-[13px] font-bold"
                  style={{
                    background: disputeReason.trim() ? "var(--color-accent)" : "var(--surface-3)",
                    color: disputeReason.trim() ? "var(--color-void)" : "var(--text-3)",
                  }}
                >
                  {busy ? "..." : "Gönder"}
                </button>
              </div>
            </div>
          )}
        </div>

        {canAct && !disputeOpen && (
          <div className="flex-shrink-0 flex gap-2 p-4" style={{ borderTop: "1px solid var(--border-1)" }}>
            <button
              onClick={() => setDisputeOpen(true)}
              disabled={busy}
              className="flex-1 h-12 rounded-full text-[13px] font-bold"
              style={{ border: "1px solid var(--error)", color: "var(--error)" }}
            >
              Dispute Aç
            </button>
            <button
              onClick={handleRelease}
              disabled={busy}
              className="flex-[2] h-12 rounded-full text-[13px] font-bold flex items-center justify-center"
              style={{ background: "var(--color-accent)", color: "var(--color-void)" }}
            >
              {busy ? <Loader2 size={18} className="animate-spin" /> : "Teslimatı Onayla"}
            </button>
          </div>
        )}
      </div>
    </AppShell>
  );
}
