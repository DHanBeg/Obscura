package marketplace_test

// Tests for marketplace.Release — escrow release to seller (#31, vault
// Phase-Status.md 2026-08-11, plan commit 28b1527, Adım 4). Money code:
// every test asserts exact balances and exact status transitions, not just
// "no error" / "error". Double-release is the named risk this step exists
// to defend against, so it gets the most scrutiny.
//
// Shares TestMain (db.Init) and the obs()/fund()/makeUser()/mustBalance()/
// mustTxStatus() helpers with marketplace_test.go in this same package.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/marketplace"
	"obscura.network/core/internal/token"
)

var releaseSeq int64

// purchaseHeld creates a listing, funds a buyer, and purchases it, leaving
// a "held" marketplace_transactions row ready for Release. Returns the
// transaction id.
func purchaseHeld(t *testing.T, seller, buyer string, price *big.Int) string {
	t.Helper()
	releaseSeq++
	id, err := marketplace.CreateListing(context.Background(), seller,
		fmt.Sprintf("Item %d", releaseSeq), "d", price.String(), "misc")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	result, err := marketplace.Purchase(context.Background(), id, buyer)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if got := mustTxStatus(t, result.TransactionID); got != marketplace.TransactionStatusHeld {
		t.Fatalf("purchaseHeld: tx status = %q, want held", got)
	}
	return result.TransactionID
}

func TestRelease_HappyPath(t *testing.T) {
	seller := "did:obs:rel-happy-seller"
	buyer := "did:obs:rel-happy-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(15)
	txID := purchaseHeld(t, seller, buyer, price)

	sellerBefore := mustBalance(t, seller)
	escrowBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	result, err := marketplace.Release(context.Background(), txID, buyer)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if result.TokenTxID == "" || result.TransactionID != txID {
		t.Fatalf("Release result = %+v", result)
	}

	sellerAfter := mustBalance(t, seller)
	wantSeller := new(big.Int).Add(sellerBefore, price)
	if sellerAfter.Cmp(wantSeller) != 0 {
		t.Fatalf("seller balance = %s, want %s", sellerAfter, wantSeller)
	}

	escrowAfter := mustBalance(t, marketplace.MarketplaceEscrowDID)
	wantEscrow := new(big.Int).Sub(escrowBefore, price)
	if escrowAfter.Cmp(wantEscrow) != 0 {
		t.Fatalf("escrow balance = %s, want %s", escrowAfter, wantEscrow)
	}

	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusReleased {
		t.Fatalf("tx status = %q, want %q", got, marketplace.TransactionStatusReleased)
	}
}

// TestRelease_Concurrent_OnlyOneMovesMoney is the double-release test. N
// goroutines all call Release on the SAME held transaction at once; the
// state-flip (UPDATE ... WHERE status = 'held') must let exactly one of
// them through to token.InternalMove. If the flip-before-money ordering (or
// the RowsAffected check) were broken, more than one goroutine could pass
// and the seller would be paid N times instead of once.
func TestRelease_Concurrent_OnlyOneMovesMoney(t *testing.T) {
	const attempts = 8

	seller := "did:obs:rel-race-seller"
	buyer := "did:obs:rel-race-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(9)
	txID := purchaseHeld(t, seller, buyer, price)
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
			_, err := marketplace.Release(context.Background(), txID, buyer)
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
		t.Fatalf("%d/%d Release attempts succeeded, want exactly 1 (double-release defense broken)", successCount, attempts)
	}

	// Seller paid EXACTLY once — the core double-release assertion.
	sellerAfter := mustBalance(t, seller)
	wantSeller := new(big.Int).Add(sellerBefore, price) // +price, NOT +2*price / +N*price
	if sellerAfter.Cmp(wantSeller) != 0 {
		t.Fatalf("seller balance = %s, want %s (paid exactly once, not %d times)", sellerAfter, wantSeller, successCount)
	}

	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusReleased {
		t.Fatalf("tx status = %q, want %q", got, marketplace.TransactionStatusReleased)
	}
}

func TestRelease_NotBuyer_Rejected(t *testing.T) {
	seller := "did:obs:rel-notbuyer-seller"
	buyer := "did:obs:rel-notbuyer-buyer"
	stranger := "did:obs:rel-notbuyer-stranger"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(3)
	txID := purchaseHeld(t, seller, buyer, price)
	sellerBefore := mustBalance(t, seller)
	escrowBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	if _, err := marketplace.Release(context.Background(), txID, stranger); !errors.Is(err, marketplace.ErrNotBuyer) {
		t.Fatalf("Release(stranger) err = %v, want ErrNotBuyer", err)
	}
	// Seller not paid either — stranger's own account is irrelevant here, but
	// also not the seller's, in case a bug credited the wrong account.
	if strings.Contains(stranger, seller) {
		t.Fatal("test setup bug: stranger equals seller")
	}

	if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
		t.Fatalf("seller balance changed on rejected Release: %s", got)
	}
	if got := mustBalance(t, marketplace.MarketplaceEscrowDID); got.Cmp(escrowBefore) != 0 {
		t.Fatalf("escrow balance changed on rejected Release: %s", got)
	}
	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusHeld {
		t.Fatalf("tx status = %q, want still %q", got, marketplace.TransactionStatusHeld)
	}
}

// TestRelease_TerminalStatus_Rejected covers both terminal predecessors the
// plan calls out: releasing an already-released transaction, and releasing
// one that was refunded (simulated directly via SQL since Adım 5's refund
// path doesn't exist yet) — both must be rejected as ErrAlreadyResolved,
// with zero further money movement.
func TestRelease_TerminalStatus_Rejected(t *testing.T) {
	t.Run("already released", func(t *testing.T) {
		seller := "did:obs:rel-term-rel-seller"
		buyer := "did:obs:rel-term-rel-buyer"
		makeUser(t, seller, 5)
		makeUser(t, buyer, 1)
		fund(t, buyer, obs(100))

		price := obs(4)
		txID := purchaseHeld(t, seller, buyer, price)
		if _, err := marketplace.Release(context.Background(), txID, buyer); err != nil {
			t.Fatalf("first Release: %v", err)
		}
		sellerAfterFirst := mustBalance(t, seller)

		if _, err := marketplace.Release(context.Background(), txID, buyer); !errors.Is(err, marketplace.ErrAlreadyResolved) {
			t.Fatalf("second Release err = %v, want ErrAlreadyResolved", err)
		}
		if got := mustBalance(t, seller); got.Cmp(sellerAfterFirst) != 0 {
			t.Fatalf("seller balance changed on second Release: before=%s after=%s", sellerAfterFirst, got)
		}
	})

	t.Run("refunded", func(t *testing.T) {
		seller := "did:obs:rel-term-ref-seller"
		buyer := "did:obs:rel-term-ref-buyer"
		makeUser(t, seller, 5)
		makeUser(t, buyer, 1)
		fund(t, buyer, obs(100))

		price := obs(6)
		txID := purchaseHeld(t, seller, buyer, price)
		// Adım 5 (refund) doesn't exist yet — simulate its end state directly.
		if _, err := db.DB.Exec(`UPDATE marketplace_transactions SET status = ? WHERE id = ?`,
			marketplace.TransactionStatusRefunded, txID); err != nil {
			t.Fatalf("simulate refunded status: %v", err)
		}
		sellerBefore := mustBalance(t, seller)

		if _, err := marketplace.Release(context.Background(), txID, buyer); !errors.Is(err, marketplace.ErrAlreadyResolved) {
			t.Fatalf("Release(refunded) err = %v, want ErrAlreadyResolved", err)
		}
		if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
			t.Fatalf("seller balance changed on Release of refunded tx: before=%s after=%s", sellerBefore, got)
		}
	})
}

// TestRelease_CertainFailure_RevertsToHeld forces releaseMove to fail with a
// CERTAIN error (anything but token.ErrCommitUncertain) and checks the plan's
// "para donması" contract: the state-flip must be reverted back to "held" so
// the release is retryable, and no money should have moved.
func TestRelease_CertainFailure_RevertsToHeld(t *testing.T) {
	seller := "did:obs:rel-certain-seller"
	buyer := "did:obs:rel-certain-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(8)
	txID := purchaseHeld(t, seller, buyer, price)
	sellerBefore := mustBalance(t, seller)
	escrowBefore := mustBalance(t, marketplace.MarketplaceEscrowDID)

	wantErr := fmt.Errorf("simulated certain failure: %w", token.ErrInsufficientBalance)
	prev := marketplace.SetEscrowMoveForTest(func(ctx context.Context, from, to string, amount *big.Int, txType string) (string, error) {
		return "", wantErr
	})
	defer marketplace.SetEscrowMoveForTest(prev)

	_, err := marketplace.Release(context.Background(), txID, buyer)
	if err == nil {
		t.Fatal("Release: want error, got nil")
	}
	if errors.Is(err, marketplace.ErrAlreadyResolved) {
		t.Fatal("Release: got ErrAlreadyResolved, want the wrapped certain failure")
	}

	// Reverted to held — retryable, no money moved.
	if got := mustTxStatus(t, txID); got != marketplace.TransactionStatusHeld {
		t.Fatalf("tx status = %q, want reverted to %q", got, marketplace.TransactionStatusHeld)
	}
	if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
		t.Fatalf("seller balance changed despite certain failure: before=%s after=%s", sellerBefore, got)
	}
	if got := mustBalance(t, marketplace.MarketplaceEscrowDID); got.Cmp(escrowBefore) != 0 {
		t.Fatalf("escrow balance changed despite certain failure: before=%s after=%s", escrowBefore, got)
	}

	// resolved_at/resolved_by must be cleared back too — a stale resolver on
	// a "held" row would be a lie about who resolved it (nobody did).
	txn, err := marketplace.GetTransaction(txID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.ResolvedAt != "" || txn.ResolvedBy != "" {
		t.Fatalf("reverted tx still has resolved_at=%q resolved_by=%q, want both cleared", txn.ResolvedAt, txn.ResolvedBy)
	}

	// And it's retryable: swap back to the real mover and confirm a second
	// Release call now succeeds normally.
	marketplace.SetEscrowMoveForTest(prev)
	if _, err := marketplace.Release(context.Background(), txID, buyer); err != nil {
		t.Fatalf("retry Release after revert: %v", err)
	}
	wantSeller := new(big.Int).Add(sellerBefore, price)
	if got := mustBalance(t, seller); got.Cmp(wantSeller) != 0 {
		t.Fatalf("seller balance after retry = %s, want %s", got, wantSeller)
	}
}

// TestRelease_UncertainFailure_StaysReleased_NoRevert forces releaseMove to
// fail with token.ErrCommitUncertain (the "did it actually commit?" case)
// and checks the OPPOSITE contract from the certain-failure test: status
// must stay "released" (NOT reverted to "held") — reverting here would let
// a retry succeed even if the original InternalMove actually landed,
// double-paying the seller. This is proven by retrying with the real mover
// restored and confirming Release refuses (ErrAlreadyResolved, since the
// status is still "released") rather than paying the seller.
func TestRelease_UncertainFailure_StaysReleased_NoRevert(t *testing.T) {
	seller := "did:obs:rel-uncertain-seller"
	buyer := "did:obs:rel-uncertain-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	price := obs(11)
	txID := purchaseHeld(t, seller, buyer, price)
	sellerBefore := mustBalance(t, seller)

	wantErr := fmt.Errorf("%w: simulated commit failure", token.ErrCommitUncertain)
	prev := marketplace.SetEscrowMoveForTest(func(ctx context.Context, from, to string, amount *big.Int, txType string) (string, error) {
		return "", wantErr
	})
	defer marketplace.SetEscrowMoveForTest(prev)

	_, err := marketplace.Release(context.Background(), txID, buyer)
	if err == nil {
		t.Fatal("Release: want error, got nil")
	}
	if !errors.Is(err, token.ErrCommitUncertain) {
		t.Fatalf("Release err = %v, want wrapping token.ErrCommitUncertain", err)
	}

	// NOT reverted — still "released", resolved_by/resolved_at still the
	// buyer's original flip (nothing about the flip itself is undone).
	txn, err := marketplace.GetTransaction(txID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if txn.Status != marketplace.TransactionStatusReleased {
		t.Fatalf("tx status = %q, want left as %q (not reverted)", txn.Status, marketplace.TransactionStatusReleased)
	}
	if txn.ResolvedBy != buyer {
		t.Fatalf("resolved_by = %q, want %q (flip itself untouched)", txn.ResolvedBy, buyer)
	}

	// Money did not actually move in THIS test (the mover was faked out
	// before touching any balance) — confirm that too, so this test isn't
	// just checking the status field in isolation.
	if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
		t.Fatalf("seller balance changed despite faked-uncertain failure: before=%s after=%s", sellerBefore, got)
	}

	// The double-release guard: restore the REAL mover and retry. Because
	// status is still "released" (not reverted), this must be refused —
	// exactly the behavior that prevents an uncertain failure from turning
	// into a double-pay if the original InternalMove actually landed.
	marketplace.SetEscrowMoveForTest(prev)
	if _, err := marketplace.Release(context.Background(), txID, buyer); !errors.Is(err, marketplace.ErrAlreadyResolved) {
		t.Fatalf("retry after uncertain failure: err = %v, want ErrAlreadyResolved (must not double-pay)", err)
	}
	if got := mustBalance(t, seller); got.Cmp(sellerBefore) != 0 {
		t.Fatalf("seller balance changed on retry after uncertain failure: before=%s after=%s (would be a double-pay)", sellerBefore, got)
	}
}

func TestRelease_UnknownTransaction_NotFound(t *testing.T) {
	buyer := "did:obs:rel-unknown-buyer"
	makeUser(t, buyer, 1)
	if _, err := marketplace.Release(context.Background(), "no-such-transaction", buyer); !errors.Is(err, marketplace.ErrTransactionNotFound) {
		t.Fatalf("Release(unknown) err = %v, want ErrTransactionNotFound", err)
	}
}
