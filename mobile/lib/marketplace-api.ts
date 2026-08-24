// #30 — marketplace HTTP client. Saf transport, gerçek backend'e
// (backend/internal/api/marketplace_handlers.go) bağlı — mock/placeholder
// data YOK. Auth mevcut apiFetch (lib/api.ts) üzerinden.
import { apiFetch } from "./api";

export interface Listing {
  id: string;
  seller_did: string;
  title: string;
  description: string;
  price: string;
  currency: string;
  category: string;
  status: "active" | "pending_purchase" | "sold" | "removed" | "flagged";
  created_at: string;
  updated_at: string;
}

export interface Transaction {
  id: string;
  listing_id: string;
  buyer_did: string;
  seller_did: string;
  amount: string;
  token_tx_id: string;
  status: "completed" | "held" | "released" | "refunded";
  resolved_at?: string;
  resolved_by?: string;
  created_at: string;
}

export interface Dispute {
  id: string;
  transaction_id: string;
  opener_did: string;
  reason: string;
  status: "open" | "resolved";
  resolved_by?: string;
  resolved_at?: string;
  created_at: string;
}

/** POST /v1/marketplace/listings — marketplace_handlers.go:61 (satıcı erişim seviyesi ≥3 gerekir). */
export function createListing(title: string, description: string, price: string, category: string) {
  return apiFetch("/v1/marketplace/listings", {
    method: "POST",
    body: JSON.stringify({ title, description, price, category }),
  }) as Promise<{ listing_id: string; listing?: Listing }>;
}

/** GET /v1/marketplace/listings?status=&limit=&offset= — marketplace_handlers.go:98. */
export function listListings(opts: { status?: string; limit?: number; offset?: number } = {}) {
  const qs = new URLSearchParams();
  if (opts.status) qs.set("status", opts.status);
  if (opts.limit !== undefined) qs.set("limit", String(opts.limit));
  if (opts.offset !== undefined) qs.set("offset", String(opts.offset));
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  return apiFetch(`/v1/marketplace/listings${suffix}`) as Promise<{ listings: Listing[]; count: number }>;
}

/** GET /v1/marketplace/listings/{id} — marketplace_handlers.go:121. */
export function getListing(id: string) {
  return apiFetch(`/v1/marketplace/listings/${encodeURIComponent(id)}`) as Promise<Listing>;
}

/** PATCH /v1/marketplace/listings/{id} — marketplace_handlers.go:146 (sadece satıcı). */
export function updateListing(id: string, patch: Partial<Pick<Listing, "title" | "description" | "price" | "category" | "status">>) {
  return apiFetch(`/v1/marketplace/listings/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  }) as Promise<Listing>;
}

/** DELETE /v1/marketplace/listings/{id} — marketplace_handlers.go:195 (soft-remove, sadece satıcı). */
export function deleteListing(id: string) {
  return apiFetch(`/v1/marketplace/listings/${encodeURIComponent(id)}`, { method: "DELETE" }) as Promise<{ listing_id: string; status: string }>;
}

/** POST /v1/marketplace/listings/{id}/purchase — marketplace_handlers.go:218, escrow hold tetikler. */
export function purchaseListing(id: string) {
  return apiFetch(`/v1/marketplace/listings/${encodeURIComponent(id)}/purchase`, { method: "POST" }) as Promise<{
    transaction_id: string; token_tx_id: string; amount: string;
  }>;
}

/** POST /v1/marketplace/listings/{id}/report — marketplace_handlers.go:366. */
export function reportListing(id: string, reason: string, category: string) {
  return apiFetch(`/v1/marketplace/listings/${encodeURIComponent(id)}/report`, {
    method: "POST",
    body: JSON.stringify({ reason, category }),
  }) as Promise<{ status: string; report_id: string }>;
}

/** GET /v1/marketplace/transactions — marketplace_handlers.go (#30) — caller'ın buyer VEYA seller olduğu tüm işlemler. */
export function listMyTransactions() {
  return apiFetch("/v1/marketplace/transactions") as Promise<{ transactions: Transaction[]; count: number }>;
}

/** GET /v1/marketplace/transactions/{id} — marketplace_handlers.go (#30), sadece buyer/seller görebilir. */
export function getTransaction(id: string) {
  return apiFetch(`/v1/marketplace/transactions/${encodeURIComponent(id)}`) as Promise<Transaction>;
}

/** POST /v1/marketplace/transactions/{id}/release — marketplace_handlers.go:245, alıcı teslim onaylar, escrow satıcıya öder. */
export function releaseTransaction(id: string) {
  return apiFetch(`/v1/marketplace/transactions/${encodeURIComponent(id)}/release`, { method: "POST" }) as Promise<{
    transaction_id: string; token_tx_id: string; amount: string;
  }>;
}

/** POST /v1/marketplace/transactions/{id}/dispute — marketplace_handlers.go:273, sadece alıcı, sadece "held". */
export function openDispute(transactionId: string, reason: string) {
  return apiFetch(`/v1/marketplace/transactions/${encodeURIComponent(transactionId)}/dispute`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  }) as Promise<Dispute>;
}

/** GET /v1/marketplace/disputes/{id} — marketplace_handlers.go (#30), sadece buyer/seller görebilir. */
export function getDispute(id: string) {
  return apiFetch(`/v1/marketplace/disputes/${encodeURIComponent(id)}`) as Promise<Dispute>;
}
