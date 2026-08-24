// #30 CANLI SMOKE — gerçek backend'e karşı marketplace akışı. Aynı altyapı
// (nodeXhrPolyfill + expo-secure-store mock) mls-e1-real-backend.smoke.test.ts
// ile — bkz. o dosyanın üst yorumu için gerekçe. OBSCURA_API_BASE set
// değilse dürüst test.skip.
//
// ÇALIŞTIRMA:
//   1) cd backend && OBSCURA_ENV=development DATA_DIR=./smoke-data PORT=8099 go run ./cmd/node
//   2) cd mobile  && OBSCURA_API_BASE=http://localhost:8099 OBSCURA_MP_DATA_DIR=<1'deki DATA_DIR> npx jest marketplace-real-backend.smoke
import { execFileSync } from "child_process";
import path from "path";

let mockCurrentToken: string | null = null;
jest.mock("expo-secure-store", () => ({
  getItemAsync: jest.fn(() => Promise.resolve(mockCurrentToken)),
  setItemAsync: jest.fn(() => Promise.resolve()),
  deleteItemAsync: jest.fn(() => Promise.resolve()),
}));

import { installNodeXhrPolyfill } from "../../test-utils/nodeXhrPolyfill";
installNodeXhrPolyfill();

import { apiFetch } from "../api";
import {
  createListing, listListings, getListing, purchaseListing,
  listMyTransactions, getTransaction, releaseTransaction,
} from "../marketplace-api";

const REAL_BACKEND = process.env.OBSCURA_API_BASE;
const DATA_DIR = process.env.OBSCURA_MP_DATA_DIR;
const maybeTest = REAL_BACKEND && DATA_DIR ? test : test.skip;

function randomPhone(): string {
  return `+1555${Math.floor(1000000 + Math.random() * 8999999)}`;
}
function randomIdentityKeyB64(): string {
  const bytes = new Uint8Array(32);
  for (let i = 0; i < 32; i++) bytes[i] = Math.floor(Math.random() * 256);
  return Buffer.from(bytes).toString("base64");
}
async function registerRealUser(username: string): Promise<{ token: string; did: string }> {
  const phone = randomPhone();
  await apiFetch("/v1/auth/request-otp", { method: "POST", body: JSON.stringify({ phone }) });
  const dev = await apiFetch(`/v1/dev/otp?phone=${encodeURIComponent(phone)}`);
  const res = await apiFetch("/v1/auth/verify-otp", {
    method: "POST",
    body: JSON.stringify({ phone, otp: dev.otp, username, identity_key: randomIdentityKeyB64() }),
  });
  return { token: res.token, did: res.user.did };
}

// Backend'in kendi test suite'inin fundMarketplaceUser helper'ıyla AYNI
// mekanizma (token.Mint), HTTP dışından — marketplace'te satın alma parası
// gerçek zincir-üstü bakiye ister, ZK-proof'suz bir faucet endpoint'i yok
// (airdrop claim ZK proof istiyor, bu smoke'un kapsamı değil). Backend'in
// AYNI DATA_DIR'ine WAL modunda ikinci bir yazar olarak bağlanıyor.
function fundAndSetTier(did: string, wholeOBS: number, tier?: number) {
  const cwd = path.resolve(__dirname, "../../../backend");
  const args = ["run", "./cmd/smoke-fund", DATA_DIR!, did, String(wholeOBS)];
  if (tier !== undefined) args.push(String(tier));
  execFileSync("go", args, { cwd, stdio: "pipe" });
}

describe("#30 CANLI SMOKE — gerçek backend marketplace akışı", () => {
  maybeTest(
    "satıcı ilan açar, gerçek backend'de görünür, alıcı satın alır (escrow hold), teslimat onaylanır",
    async () => {
      const seller = await registerRealUser(`mp-seller-${Date.now()}`);
      const buyer = await registerRealUser(`mp-buyer-${Date.now()}`);

      fundAndSetTier(buyer.did, 100);
      fundAndSetTier(seller.did, 1, 5); // marketplace.SellerAccessLevel=3 gerekli

      // 1. Satıcı ilan açar — gerçek POST /v1/marketplace/listings.
      mockCurrentToken = seller.token;
      const created = await createListing("Smoke Test Item", "canlı smoke aciklama", "5000000000000000000", "misc");
      expect(created.listing_id).toBeTruthy();

      // 2. İlan gerçek backend'den GEZİNME listesinde görünüyor mu (mobile
      // marketplace.tsx'in çağırdığı AYNI fonksiyon).
      const listRes = await listListings({ status: "active" });
      expect(listRes.listings.some((l) => l.id === created.listing_id)).toBe(true);

      // 3. Detay ekranının çağırdığı AYNI fonksiyon.
      const detail = await getListing(created.listing_id);
      expect(detail.title).toBe("Smoke Test Item");
      expect(detail.status).toBe("active");

      // 4. Alıcı satın alır — gerçek escrow hold (mobile marketplace/[id].tsx'in
      // çağırdığı AYNI fonksiyon).
      mockCurrentToken = buyer.token;
      const purchase = await purchaseListing(created.listing_id);
      expect(purchase.transaction_id).toBeTruthy();

      // 5. İlan artık "sold" — escrow gerçekten tetiklendi.
      const afterPurchase = await getListing(created.listing_id);
      expect(afterPurchase.status).toBe("sold");

      // 6. Alıcının "Siparişlerim" listesinde görünüyor mu (mobile
      // marketplace-orders.tsx'in çağırdığı AYNI fonksiyon), status="held".
      const myTx = await listMyTransactions();
      const found = myTx.transactions.find((t) => t.id === purchase.transaction_id);
      expect(found?.status).toBe("held");

      // 7. Tek işlem detayı (mobile marketplace-order/[id].tsx).
      const txDetail = await getTransaction(purchase.transaction_id);
      expect(txDetail.status).toBe("held");
      expect(txDetail.buyer_did).toBe(buyer.did);
      expect(txDetail.seller_did).toBe(seller.did);

      // 8. Alıcı teslimatı onaylar — escrow satıcıya öder.
      const release = await releaseTransaction(purchase.transaction_id);
      expect(release.transaction_id).toBe(purchase.transaction_id);

      const afterRelease = await getTransaction(purchase.transaction_id);
      expect(afterRelease.status).toBe("released");
    },
    60000
  );

  if (!(REAL_BACKEND && DATA_DIR)) {
    test("SKIP nedeni görünür olsun", () => {
      console.log(
        "[#30 SMOKE] OBSCURA_API_BASE/OBSCURA_MP_DATA_DIR set edilmedi — gerçek backend smoke ATLANDI."
      );
    });
  }
});
