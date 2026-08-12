package marketplace_test

// Tests for marketplace.OpenDispute / marketplace.ResolveDispute — the
// final escrow layer (#31, vault Phase-Status.md 2026-08-11, plan commit
// 28b1527, Adım 5). Money code: exact balances, exact status transitions.
// Double-resolve gets the same scrutiny double-release got in Adım 4 — it's
// the same resolveHeld atomic core, but this is the test that proves the
// sharing actually holds for the dispute path too.
//
// Shares TestMain (db.Init) and the obs()/fund()/makeUser()/mustBalance()/
// mustTxStatus() helpers with marketplace_test.go, and purchaseHeld() with
// release_test.go, in this same package.

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"

	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/token"
)

func TestOpenDispute_HappyPath(t *testing.T) {
	seller := "did:obs:disp-open-seller"
	buyer := "did:obs:disp-open-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))

	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "item never arrived")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}
	if dispute.ID == "" || dispute.TransactionID != txID || dispute.OpenerDID != buyer {
		t.Fatalf("dispute = %+v", dispute)
	}
	if dispute.Status != marketplace.DisputeStatusOpen {
		t.Fatalf("status = %q, want %q", dispute.Status, marketplace.DisputeStatusOpen)
	}

	// Opening a dispute must not itself move money or change the
	// transaction's status — it's still held until an admin resolves it.
	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusHeld {
		t.Fatalf("tx status = %q, want still %q", got, marketplace.TransactionStatusHeld)
	}
}

func TestOpenDispute_NotBuyer_Rejected(t *testing.T) {
	seller := "did:obs:disp-notbuyer-seller"
	buyer := "did:obs:disp-notbuyer-buyer"
	stranger := "did:obs:disp-notbuyer-stranger"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))

	if _, err := marketplace.OpenDispute(context.Background(), txID, stranger, "not mine"); !errors.Is(err, marketplace.ErrNotBuyer) {
		t.Fatalf("OpenDispute(stranger) err = %v, want ErrNotBuyer", err)
	}
}

func TestOpenDispute_NotHeld_Rejected(t *testing.T) {
	seller := "did:obs:disp-notheld-seller"
	buyer := "did:obs:disp-notheld-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))
	if _, err := marketplace.Release(context.Background(), txID, buyer); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if _, err := marketplace.OpenDispute(context.Background(), txID, buyer, "too late"); !errors.Is(err, marketplace.ErrAlreadyResolved) {
		t.Fatalf("OpenDispute(released tx) err = %v, want ErrAlreadyResolved", err)
	}
}

func TestOpenDispute_DuplicateOpen_Rejected(t *testing.T) {
	seller := "did:obs:disp-dup-seller"
	buyer := "did:obs:disp-dup-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))
	if _, err := marketplace.OpenDispute(context.Background(), txID, buyer, "first"); err != nil {
		t.Fatalf("first OpenDispute: %v", err)
	}
	if _, err := marketplace.OpenDispute(context.Background(), txID, buyer, "second"); !errors.Is(err, marketplace.ErrDisputeAlreadyOpen) {
		t.Fatalf("second OpenDispute err = %v, want ErrDisputeAlreadyOpen", err)
	}
}

func TestResolveDispute_UpheldFalse_SellerPaid(t *testing.T) {
	seller := "did:obs:disp-sellerwins-seller"
	buyer := "did:obs:disp-sellerwins-buyer"
	admin := "did:obs:disp-sellerwins-admin"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(6)
	txID := purchaseHeld(t, seller, buyer, price)
	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "wrong item")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	sellerBefore := mustBalance(t, seller)
	buyerBefore := mustBalance(t, buyer)
	escrowBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	result, err := marketplace.ResolveDispute(context.Background(), dispute.ID, admin, false)
	if err != nil {
		t.Fatalf("ResolveDispute(upheld=false): %v", err)
	}
	if result.Upheld {
		t.Fatal("result.Upheld = true, want false")
	}
	if result.PaidTo != seller {
		t.Fatalf("PaidTo = %q, want seller %q", result.PaidTo, seller)
	}

	sellerAfter := mustBalance(t, seller)
	wantSeller := new(big.Int).Add(sellerBefore, price)
	if sellerAfter.Cmp(wantSeller) != 0 {
		t.Fatalf("seller balance = %s, want %s", sellerAfter, wantSeller)
	}
	// Buyer gets nothing back — seller won the dispute.
	if got := mustBalance(t, buyer); got.Cmp(buyerBefore) != 0 {
		t.Fatalf("buyer balance changed despite losing dispute: before=%s after=%s", buyerBefore, got)
	}
	escrowAfter := mustBalance(t, marketplace.MarketplaceEscrowDID)
	wantEscrow := new(big.Int).Sub(escrowBefore, price)
	if escrowAfter.Cmp(wantEscrow) != 0 {
		t.Fatalf("escrow balance = %s, want %s", escrowAfter, wantEscrow)
	}

	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusReleased {
		t.Fatalf("tx status = %q, want %q", got, marketplace.TransactionStatusReleased)
	}
	gotDispute, err := marketplace.GetDispute(dispute.ID)
	if err != nil {
		t.Fatalf("GetDispute: %v", err)
	}
	if gotDispute.Status != marketplace.DisputeStatusResolved || gotDispute.ResolvedBy != admin {
		t.Fatalf("dispute = %+v, want status=resolved resolved_by=%s", gotDispute, admin)
	}
}

// TestResolveDispute_UpheldTrue_BuyerRefundedPriceNotFee is the explicit
// "refund is price, not price+fee" proof the plan calls for. The buyer's
// TOTAL round-trip must be: paid (price+fee) at Purchase, refunded (price)
// at ResolveDispute — net cost to the buyer is exactly TransferFee(), never
// zero and never negative (which would mean the fee came back too).
func TestResolveDispute_UpheldTrue_BuyerRefundedPriceNotFee(t *testing.T) {
	seller := "did:obs:disp-buyerwins-seller"
	buyer := "did:obs:disp-buyerwins-buyer"
	admin := "did:obs:disp-buyerwins-admin"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(8)
	buyerBeforePurchase := mustBalance(t, buyer)

	txID := purchaseHeld(t, seller, buyer, price)
	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "never delivered")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	sellerBefore := mustBalance(t, seller)
	escrowBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	result, err := marketplace.ResolveDispute(context.Background(), dispute.ID, admin, true)
	if err != nil {
		t.Fatalf("ResolveDispute(upheld=true): %v", err)
	}
	if !result.Upheld {
		t.Fatal("result.Upheld = false, want true")
	}
	if result.PaidTo != buyer {
		t.Fatalf("PaidTo = %q, want buyer %q", result.PaidTo, buyer)
	}
	if result.Amount != price.String() {
		t.Fatalf("Amount = %q, want %q (price, not price+fee)", result.Amount, price.String())
	}

	// Seller gets nothing — buyer won the dispute.
	if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
		t.Fatalf("seller balance changed despite losing dispute: before=%s after=%s", sellerBefore, got)
	}

	// Escrow paid out exactly `price` (it never held the fee to begin with).
	escrowAfter := mustBalance(t, marketplace.MarketplaceEscrowDID)
	wantEscrow := new(big.Int).Sub(escrowBefore, price)
	if escrowAfter.Cmp(wantEscrow) != 0 {
		t.Fatalf("escrow balance = %s, want %s", escrowAfter, wantEscrow)
	}

	// THE core assertion: buyer's balance after refund is price+fee below
	// where they started before Purchase — i.e. they got `price` back, NOT
	// `price+fee`. If the refund wrongly included the fee, this diff would
	// be exactly TransferFee() smaller (buyer would be made whole again).
	buyerAfterRefund := mustBalance(t, buyer)
	netCost := new(big.Int).Sub(buyerBeforePurchase, buyerAfterRefund)
	wantNetCost := token.TransferFee()
	if netCost.Cmp(wantNetCost) != 0 {
		t.Fatalf("buyer net cost (paid - refunded) = %s, want exactly TransferFee() = %s (refund must be price only, fee is non-refundable by design)",
			netCost, wantNetCost)
	}

	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusRefunded {
		t.Fatalf("tx status = %q, want %q", got, marketplace.TransactionStatusRefunded)
	}
}

// TestResolveDispute_Concurrent_OnlyOneMovesMoney is the double-resolve
// test — same shape as Adım 4's double-release test, proving resolveHeld's
// sharing actually holds: N goroutines call ResolveDispute on the SAME open
// dispute at once, only one may pay out.
func TestResolveDispute_Concurrent_OnlyOneMovesMoney(t *testing.T) {
	const attempts = 8

	seller := "did:obs:disp-race-seller"
	buyer := "did:obs:disp-race-buyer"
	admin := "did:obs:disp-race-admin"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(7)
	txID := purchaseHeld(t, seller, buyer, price)
	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "damaged")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}
	sellerBefore := mustBalance(t, seller)

	succeeded := make([]bool, attempts)
	errs := make([]error, attempts)

	var ready sync.WaitGroup
	var wg sync.WaitGroup
	start := make(chan struct{})

	ready.Add(attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			// All attempts vote the SAME way (upheld=false, seller wins) —
			// double-resolve must hold even when every racer agrees on the
			// outcome, not just when outcomes differ.
			_, err := marketplace.ResolveDispute(context.Background(), dispute.ID, admin, false)
			succeeded[i] = err == nil
			errs[i] = err
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	successCount := 0
	for i, ok := range succeeded {
		if ok {
			successCount++
		} else if !errors.Is(errs[i], marketplace.ErrAlreadyResolved) {
			t.Errorf("attempt %d: err = %v, want nil or ErrAlreadyResolved", i, errs[i])
		}
	}
	if successCount != 1 {
		t.Fatalf("%d/%d ResolveDispute attempts succeeded, want exactly 1 (double-resolve defense broken)", successCount, attempts)
	}

	sellerAfter := mustBalance(t, seller)
	wantSeller := new(big.Int).Add(sellerBefore, price) // +price, NOT +N*price
	if sellerAfter.Cmp(wantSeller) != 0 {
		t.Fatalf("seller balance = %s, want %s (paid exactly once, not %d times)", sellerAfter, wantSeller, successCount)
	}
}

func TestResolveDispute_AlreadyResolved_Rejected(t *testing.T) {
	seller := "did:obs:disp-term-seller"
	buyer := "did:obs:disp-term-buyer"
	admin := "did:obs:disp-term-admin"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))
	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "x")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}
	if _, err := marketplace.ResolveDispute(context.Background(), dispute.ID, admin, false); err != nil {
		t.Fatalf("first ResolveDispute: %v", err)
	}
	sellerAfterFirst := mustBalance(t, seller)

	if _, err := marketplace.ResolveDispute(context.Background(), dispute.ID, admin, false); !errors.Is(err, marketplace.ErrAlreadyResolved) {
		t.Fatalf("second ResolveDispute err = %v, want ErrAlreadyResolved", err)
	}
	if got := mustBalance(t, seller); got.Cmp(sellerAfterFirst) != 0 {
		t.Fatalf("seller balance changed on second resolve: before=%s after=%s", sellerAfterFirst, got)
	}
}

func TestResolveDispute_UnknownDispute_NotFound(t *testing.T) {
	admin := "did:obs:disp-unknown-admin"
	if _, err := marketplace.ResolveDispute(context.Background(), "no-such-dispute", admin, false); !errors.Is(err, marketplace.ErrDisputeNotFound) {
		t.Fatalf("ResolveDispute(unknown) err = %v, want ErrDisputeNotFound", err)
	}
}
