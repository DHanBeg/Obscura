"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Search, Plus, Receipt, StoreIcon, WifiOff } from "lucide-react";
import { cn } from "@/lib/cn";
import { AppShell } from "@/components/AppShell";
import { listListings, type Listing } from "@/lib/marketplace-api";

const CATEGORIES = ["all", "goods", "services", "digital", "misc"] as const;
const CATEGORY_LABELS: Record<string, string> = {
  all: "Tümü", goods: "Ürün", services: "Hizmet", digital: "Dijital", misc: "Diğer",
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

function SkeletonCard() {
  return (
    <div className="card p-4 mb-3">
      <div className="h-4 w-32 rounded shimmer mb-2" />
      <div className="h-3 w-full rounded shimmer mb-1" />
      <div className="h-3 w-2/3 rounded shimmer" />
    </div>
  );
}

export default function MarketplacePage() {
  const router = useRouter();
  const [listings, setListings] = useState<Listing[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<string>("all");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listListings({ status: "active" });
      setListings(res.listings || []);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const filtered = listings.filter((l) => {
    if (category !== "all" && l.category !== category) return false;
    if (!search) return true;
    return (l.title + l.description).toLowerCase().includes(search.toLowerCase());
  });

  return (
    <AppShell>
      <div className="flex flex-col h-full scroll-area">
        {/* ── Page Header ── */}
        <div className="page-header">
          <h1 className="page-title">Pazar</h1>
          <div className="flex items-center gap-1">
            <button
              onClick={() => router.push("/marketplace/orders")}
              aria-label="Siparişlerim"
              className="btn-icon"
            >
              <Receipt size={18} />
            </button>
            <button
              onClick={() => router.push("/marketplace/new")}
              aria-label="Yeni ilan"
              className="btn-icon"
              style={{ color: "var(--color-accent)" }}
            >
              <Plus size={20} />
            </button>
          </div>
        </div>

        {/* ── Search ── */}
        <div className="px-4 pt-3">
          <div
            className="flex items-center gap-2 h-11 px-3 rounded-2xl"
            style={{ background: "var(--surface-2)", border: "1px solid var(--border-1)" }}
          >
            <Search size={16} style={{ color: "var(--text-3)" }} />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="İlan ara..."
              className="flex-1 bg-transparent outline-none text-sm"
              style={{ color: "var(--text-1)" }}
            />
          </div>
        </div>

        {/* ── Category chips ── */}
        <div className="flex items-center gap-1.5 px-4 pt-3 pb-2 overflow-x-auto scrollbar-none">
          {CATEGORIES.map((c) => {
            const active = category === c;
            return (
              <button
                key={c}
                onClick={() => setCategory(c)}
                className="flex-shrink-0 px-3 py-1.5 rounded-full text-[12px] font-semibold transition-all duration-150"
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

        {/* ── List ── */}
        <div className="flex-1 overflow-y-auto px-4 pb-6">
          {loading ? (
            <>
              <SkeletonCard /><SkeletonCard /><SkeletonCard />
            </>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-2 pt-20">
              {loadError ? <WifiOff size={36} style={{ color: "var(--text-3)" }} /> : <StoreIcon size={36} style={{ color: "var(--text-3)" }} />}
              <p style={{ color: "var(--text-3)", fontSize: 14 }}>
                {loadError ? "İlanlar yüklenemedi" : "Henüz ilan yok"}
              </p>
            </div>
          ) : (
            filtered.map((l) => (
              <button
                key={l.id}
                onClick={() => router.push(`/marketplace/${l.id}`)}
                className="card-interactive w-full text-left p-4 mb-3 block"
              >
                <div className="flex items-start justify-between gap-2 mb-1">
                  <span className="font-semibold text-[15px] truncate" style={{ color: "var(--text-1)" }}>{l.title}</span>
                  <span className="font-bold text-[14px] flex-shrink-0" style={{ color: "var(--color-accent)" }}>
                    {fmtPrice(l.price)} OBS
                  </span>
                </div>
                <p className="text-[13px] line-clamp-2" style={{ color: "var(--text-3)" }}>{l.description}</p>
                <div className="mt-2">
                  <span className="badge badge-neutral">{CATEGORY_LABELS[l.category] || l.category}</span>
                </div>
              </button>
            ))
          )}
        </div>
      </div>
    </AppShell>
  );
}
