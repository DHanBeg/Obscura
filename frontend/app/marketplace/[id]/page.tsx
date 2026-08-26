"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter, useParams } from "next/navigation";
import { ChevronLeft, Loader2, AlertCircle, UserCircle, Pencil, Trash2 } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { useStore } from "@/lib/store";
import { getListing, purchaseListing, deleteListing, type Listing } from "@/lib/marketplace-api";

const STATUS_LABELS: Record<string, string> = {
  active: "Satışta", pending_purchase: "İşlemde", sold: "Satıldı", removed: "Kaldırıldı", flagged: "İncelemede",
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

export default function MarketplaceListingPage() {
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const { user } = useStore();
  const [listing, setListing] = useState<Listing | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [purchasing, setPurchasing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!params.id) return;
    setLoading(true);
    try {
      const l = await getListing(params.id);
      setListing(l);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [params.id]);

  useEffect(() => { load(); }, [load]);

  const isOwnListing = listing?.seller_did === user?.did;
  const canPurchase = listing?.status === "active" && !isOwnListing;

  const handlePurchase = useCallback(async () => {
    if (!listing) return;
    setPurchasing(true);
    setError("");
    try {
      const result = await purchaseListing(listing.id);
      router.replace(`/marketplace/orders/${result.transaction_id}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Satın alma başarısız.");
      setPurchasing(false);
    }
  }, [listing, router]);

  const handleDelete = useCallback(async () => {
    if (!listing) return;
    if (!confirm("İlanı kaldırmak istediğinize emin misiniz? Bu geri alınamaz.")) return;
    setDeleting(true);
    setError("");
    try {
      await deleteListing(listing.id);
      router.replace("/marketplace");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "İlan silinemedi.");
      setDeleting(false);
    }
  }, [listing, router]);

  if (loading) {
    return (
      <AppShell showBack title="İlan">
        <div className="flex-1 flex items-center justify-center h-full">
          <Loader2 size={28} className="animate-spin" style={{ color: "var(--color-accent)" }} />
        </div>
      </AppShell>
    );
  }

  if (loadError || !listing) {
    return (
      <AppShell showBack title="İlan">
        <div className="flex-1 flex flex-col items-center justify-center gap-3 h-full">
          <AlertCircle size={36} style={{ color: "var(--text-3)" }} />
          <p style={{ color: "var(--text-3)" }}>İlan bulunamadı</p>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="İlan">
      <div className="flex flex-col h-full">
        <div className="flex-1 overflow-y-auto scroll-area p-5">
          <div className="flex items-center gap-2 mb-4">
            <span
              className="badge"
              style={{
                background: listing.status === "active" ? "var(--color-accent-deep)" : "var(--surface-3)",
                color: listing.status === "active" ? "var(--color-accent)" : "var(--text-2)",
              }}
            >
              {STATUS_LABELS[listing.status] || listing.status}
            </span>
            <span className="badge badge-neutral">{listing.category}</span>
          </div>

          <h1 className="text-[24px] font-bold mb-1" style={{ color: "var(--text-1)", fontFamily: "var(--font-display)" }}>
            {listing.title}
          </h1>
          <p className="text-[20px] font-bold mb-4" style={{ color: "var(--color-accent)" }}>
            {fmtPrice(listing.price)} OBS
          </p>
          <p className="text-[15px] leading-relaxed mb-6" style={{ color: "var(--text-2)" }}>
            {listing.description}
          </p>

          <div className="flex items-center gap-2">
            <UserCircle size={18} style={{ color: "var(--text-3)" }} />
            <span className="text-[12px] truncate" style={{ color: "var(--text-3)" }}>{listing.seller_did}</span>
          </div>

          {error && (
            <div className="mt-4 badge badge-error" style={{ height: "auto", padding: "8px 12px" }}>
              {error}
            </div>
          )}
        </div>

        <div className="flex-shrink-0 p-4" style={{ borderTop: "1px solid var(--border-1)" }}>
          {isOwnListing ? (
            <div className="flex gap-2">
              <button
                onClick={() => router.push(`/marketplace/${listing.id}/edit`)}
                disabled={listing.status === "removed"}
                className="flex-1 h-13 rounded-full font-bold text-[15px] flex items-center justify-center gap-2"
                style={{
                  height: 52,
                  border: "1px solid var(--border-2)",
                  color: listing.status === "removed" ? "var(--text-3)" : "var(--text-1)",
                  opacity: listing.status === "removed" ? 0.5 : 1,
                }}
              >
                <Pencil size={16} /> Düzenle
              </button>
              <button
                onClick={handleDelete}
                disabled={deleting || listing.status === "removed"}
                className="flex-1 h-13 rounded-full font-bold text-[15px] flex items-center justify-center gap-2"
                style={{
                  height: 52,
                  border: "1px solid var(--error)",
                  color: "var(--error)",
                  opacity: listing.status === "removed" ? 0.5 : 1,
                }}
              >
                {deleting ? <Loader2 size={18} className="animate-spin" /> : <><Trash2 size={16} /> Kaldır</>}
              </button>
            </div>
          ) : (
            <button
              onClick={handlePurchase}
              disabled={!canPurchase || purchasing}
              className="w-full h-13 rounded-full font-bold text-[15px] flex items-center justify-center transition-all duration-150 active:scale-[0.98]"
              style={{
                height: 52,
                background: canPurchase ? "var(--color-accent)" : "var(--surface-3)",
                color: canPurchase ? "var(--color-void)" : "var(--text-3)",
              }}
            >
              {purchasing ? <Loader2 size={20} className="animate-spin" /> : (canPurchase ? `Satın Al · ${fmtPrice(listing.price)} OBS` : "Satışta Değil")}
            </button>
          )}
        </div>
      </div>
    </AppShell>
  );
}
