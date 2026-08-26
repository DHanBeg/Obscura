"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter, useParams } from "next/navigation";
import { Loader2, AlertCircle } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { useStore } from "@/lib/store";
import { getListing, updateListing, type Listing } from "@/lib/marketplace-api";

const CATEGORIES = ["goods", "services", "digital", "misc"] as const;
const CATEGORY_LABELS: Record<string, string> = {
  goods: "Ürün", services: "Hizmet", digital: "Dijital", misc: "Diğer",
};

// B9 parça 2 — düzenleme formu new/page.tsx ile AYNI alan/ölçek/stil.
// price backend'de en küçük birim (18 ondalık) string olarak duruyor —
// forma insan-okur OBS değeri olarak geri çevrilip, gönderilirken tekrar
// en küçük birime çevrilir (create formundakiyle simetrik).
export default function MarketplaceEditListingPage() {
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const { user } = useStore();

  const [listing, setListing] = useState<Listing | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [forbidden, setForbidden] = useState(false);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [category, setCategory] = useState<string>("goods");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!params.id) return;
    (async () => {
      setLoading(true);
      try {
        const l = await getListing(params.id);
        setListing(l);
        setTitle(l.title);
        setDescription(l.description);
        setPrice((Number(BigInt(l.price)) / 1e18).toString());
        setCategory(l.category);
        setLoadError(false);
      } catch {
        setLoadError(true);
      } finally {
        setLoading(false);
      }
    })();
  }, [params.id]);

  // Sahiplik doğrulaması: getListing herkese açık döner (satıcı-özel bir
  // "kendi ilanlarım" uç noktası yok) — bu yüzden client tarafında da
  // kontrol edilir. Asıl güvence backend'de (marketplace.UpdateListing
  // ErrNotSeller → 403, PATCH gönderilse bile) — bu sadece formu erken
  // gizleyip gereksiz denemeyi önlüyor.
  useEffect(() => {
    if (listing && user && listing.seller_did !== user.did) setForbidden(true);
  }, [listing, user]);

  const canSubmit = title.trim().length > 0 && description.trim().length > 0 && /^\d+(\.\d+)?$/.test(price.trim());

  const handleSubmit = useCallback(async () => {
    if (!canSubmit || !listing) return;
    setSubmitting(true);
    setError("");
    try {
      const priceSmallestUnit = (BigInt(Math.round(parseFloat(price) * 1e6)) * 1000000000000n).toString();
      await updateListing(listing.id, {
        title: title.trim(),
        description: description.trim(),
        price: priceSmallestUnit,
        category,
      });
      router.replace(`/marketplace/${listing.id}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "İlan güncellenemedi.");
      setSubmitting(false);
    }
  }, [canSubmit, listing, title, description, price, category, router]);

  if (loading) {
    return (
      <AppShell showBack title="İlanı Düzenle">
        <div className="flex-1 flex items-center justify-center h-full">
          <Loader2 size={28} className="animate-spin" style={{ color: "var(--color-accent)" }} />
        </div>
      </AppShell>
    );
  }

  if (loadError || !listing || forbidden) {
    return (
      <AppShell showBack title="İlanı Düzenle">
        <div className="flex-1 flex flex-col items-center justify-center gap-3 h-full">
          <AlertCircle size={36} style={{ color: "var(--text-3)" }} />
          <p style={{ color: "var(--text-3)" }}>{forbidden ? "Bu ilanı düzenleme yetkiniz yok" : "İlan bulunamadı"}</p>
        </div>
      </AppShell>
    );
  }

  return (
    <AppShell showBack title="İlanı Düzenle">
      <div className="flex flex-col h-full">
        <div className="flex-1 overflow-y-auto scroll-area p-5">
          <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Başlık</label>
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Ne satıyorsunuz?"
            className="w-full h-12 mt-1.5 mb-4 px-4 rounded-2xl text-[15px] outline-none"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", color: "var(--text-1)" }}
          />

          <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Açıklama</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Detayları yazın..."
            className="w-full h-24 mt-1.5 mb-4 p-4 rounded-2xl text-[15px] outline-none resize-none"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", color: "var(--text-1)" }}
          />

          <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Fiyat (OBS)</label>
          <input
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            placeholder="0"
            inputMode="decimal"
            className="w-full h-12 mt-1.5 mb-4 px-4 rounded-2xl text-[15px] outline-none"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)", color: "var(--text-1)" }}
          />

          <label className="text-[11px] font-semibold uppercase tracking-wide" style={{ color: "var(--text-3)" }}>Kategori</label>
          <div className="flex flex-wrap gap-1.5 mt-1.5">
            {CATEGORIES.map((c) => {
              const active = category === c;
              return (
                <button
                  key={c}
                  onClick={() => setCategory(c)}
                  className="px-3 py-1.5 rounded-full text-[12px] font-semibold"
                  style={{
                    background: active ? "var(--color-accent-deep)" : "var(--surface-2)",
                    border: `1px solid ${active ? "var(--color-accent-dim)" : "var(--border-1)"}`,
                    color: active ? "var(--color-accent)" : "var(--text-3)",
                  }}
                >
                  {CATEGORY_LABELS[c]}
                </button>
              );
            })}
          </div>

          {error && (
            <div className="mt-4 badge badge-error" style={{ height: "auto", padding: "8px 12px" }}>{error}</div>
          )}
        </div>

        <div className="flex-shrink-0 p-4" style={{ borderTop: "1px solid var(--border-1)" }}>
          <button
            onClick={handleSubmit}
            disabled={!canSubmit || submitting}
            className="w-full rounded-full font-bold text-[15px] flex items-center justify-center"
            style={{
              height: 52,
              background: canSubmit ? "var(--color-accent)" : "var(--surface-3)",
              color: canSubmit ? "var(--color-void)" : "var(--text-3)",
            }}
          >
            {submitting ? <Loader2 size={20} className="animate-spin" /> : "Değişiklikleri Kaydet"}
          </button>
        </div>
      </div>
    </AppShell>
  );
}
