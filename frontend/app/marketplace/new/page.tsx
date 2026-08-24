"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { createListing } from "@/lib/marketplace-api";

const CATEGORIES = ["goods", "services", "digital", "misc"] as const;
const CATEGORY_LABELS: Record<string, string> = {
  goods: "Ürün", services: "Hizmet", digital: "Dijital", misc: "Diğer",
};

export default function MarketplaceNewListingPage() {
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [price, setPrice] = useState("");
  const [category, setCategory] = useState<string>("goods");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const canSubmit = title.trim().length > 0 && description.trim().length > 0 && /^\d+(\.\d+)?$/.test(price.trim());

  const handleSubmit = useCallback(async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    setError("");
    try {
      // price backend'e obs_token'ın en küçük birimi (18 ondalık) string
      // olarak gidiyor — mobile marketplace-new-listing.tsx ile AYNI ölçek.
      const priceSmallestUnit = (BigInt(Math.round(parseFloat(price) * 1e6)) * 1000000000000n).toString();
      const res = await createListing(title.trim(), description.trim(), priceSmallestUnit, category);
      router.replace(`/marketplace/${res.listing_id}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "İlan oluşturulamadı.");
      setSubmitting(false);
    }
  }, [canSubmit, title, description, price, category, router]);

  return (
    <AppShell showBack title="Yeni İlan">
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
            {submitting ? <Loader2 size={20} className="animate-spin" /> : "İlanı Yayınla"}
          </button>
        </div>
      </div>
    </AppShell>
  );
}
