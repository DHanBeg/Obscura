"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { ArrowDownCircle, ArrowUpCircle, Receipt, WifiOff } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { useStore } from "@/lib/store";
import { listMyTransactions, type Transaction } from "@/lib/marketplace-api";

const STATUS_LABELS: Record<string, string> = {
  held: "Beklemede (escrow)", released: "Tamamlandı", refunded: "İade edildi", completed: "Tamamlandı",
};
const STATUS_CLASS: Record<string, string> = {
  held: "badge-warning", released: "badge-success", refunded: "badge-error", completed: "badge-success",
};

function fmtPrice(raw: string): string {
  try {
    return (BigInt(raw) / 1000000000000000000n).toString();
  } catch {
    return raw;
  }
}

function SkeletonRow() {
  return (
    <div className="card p-4 mb-3">
      <div className="h-3 w-24 rounded shimmer mb-2" />
      <div className="h-5 w-20 rounded shimmer" />
    </div>
  );
}

export default function MarketplaceOrdersPage() {
  const router = useRouter();
  const { user } = useStore();
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listMyTransactions();
      setTransactions(res.transactions || []);
      setLoadError(false);
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <AppShell showBack title="Siparişlerim">
      <div className="flex-1 overflow-y-auto scroll-area px-4 pt-4 pb-6">
        {loading ? (
          <><SkeletonRow /><SkeletonRow /><SkeletonRow /></>
        ) : transactions.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 pt-20">
            {loadError ? <WifiOff size={36} style={{ color: "var(--text-3)" }} /> : <Receipt size={36} style={{ color: "var(--text-3)" }} />}
            <p style={{ color: "var(--text-3)", fontSize: 14 }}>{loadError ? "Siparişler yüklenemedi" : "Henüz sipariş yok"}</p>
          </div>
        ) : (
          transactions.map((t) => {
            const isBuyer = t.buyer_did === user?.did;
            return (
              <button
                key={t.id}
                onClick={() => router.push(`/marketplace/orders/${t.id}`)}
                className="card-interactive w-full text-left p-4 mb-3 block"
              >
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-1.5" style={{ color: "var(--text-3)" }}>
                    {isBuyer ? <ArrowDownCircle size={14} /> : <ArrowUpCircle size={14} />}
                    <span className="text-[11px] font-semibold">{isBuyer ? "Satın alım" : "Satış"}</span>
                  </div>
                  <span className={`badge ${STATUS_CLASS[t.status] || "badge-neutral"}`}>
                    {STATUS_LABELS[t.status] || t.status}
                  </span>
                </div>
                <span className="text-[18px] font-bold" style={{ color: "var(--text-1)" }}>{fmtPrice(t.amount)} OBS</span>
                <div className="text-[11px] mt-1" style={{ color: "var(--text-3)" }}>
                  {new Date(t.created_at).toLocaleDateString("tr-TR")}
                </div>
              </button>
            );
          })
        )}
      </div>
    </AppShell>
  );
}
