package token_test

// Tests for token.InternalMove — the escrow-only, fee-free internal shift
// primitive (#31, vault Phase-Status.md 2026-08-11, plan commit 28b1527,
// Adım 2). Money code: every test below asserts exact balances, not just
// "no error" — a silently-wrong debit/credit is worse than a loud failure.
//
// Shares TestMain (db.Init against a temp on-disk SQLite DB) and the
// obs()/fund()/mustBalance()/mustSupply() helpers with token_test.go in this
// same package.

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"obscura.network/core/internal/token"
)

func TestInternalMove_HappyPath_NoFee(t *testing.T) {
	alice := "did:obs:im-happy-alice"
	bob := "did:obs:im-happy-bob"
	fund(t, alice, obs(100))

	amount := obs(10)
	txID, err := token.InternalMove(context.Background(), alice, bob, amount, "escrow_hold")
	if err != nil {
		t.Fatalf("internalMove: %v", err)
	}
	if txID == "" {
		t.Fatal("empty tx id")
	}

	// Alice: -amount EXACTLY (no fee subtracted, unlike Transfer).
	wantAlice := new(big.Int).Sub(obs(100), amount)
	if got := mustBalance(t, alice); got.Cmp(wantAlice) != 0 {
		t.Errorf("alice balance = %s, want %s", got, wantAlice)
	}
	// Bob: +amount EXACTLY — this is the "fee YOK" assertion: Transfer would
	// have credited bob only amount (fee is charged to the sender on top of
	// amount, so bob's credit is unaffected there too) but alice would have
	// been debited amount+fee. Here alice is debited exactly amount.
	if got := mustBalance(t, bob); got.Cmp(amount) != 0 {
		t.Errorf("bob balance = %s, want %s (full amount, no fee skimmed)", got, amount)
	}

	// tx_type recorded as given ("escrow_hold"), fee column is "0".
	hist, err := token.History(alice, 50)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	found := false
	for _, h := range hist {
		if h.ID == txID {
			found = true
			if h.TxType != "escrow_hold" {
				t.Errorf("tx_type = %q, want %q", h.TxType, "escrow_hold")
			}
			if h.Fee != "0" {
				t.Errorf("fee = %q, want \"0\" — internalMove charges no fee", h.Fee)
			}
			if h.Amount != amount.String() {
				t.Errorf("amount = %q, want %q", h.Amount, amount.String())
			}
		}
	}
	if !found {
		t.Fatal("internalMove not found in alice history")
	}
}

// TestInternalMove_NoFee_DiffersFromTransfer is the explicit side-by-side
// "fee YOK" proof the plan calls for: send the same amount via Transfer and
// via InternalMove from equally-funded senders and show the recipients end
// up with the same credit (amount, no fee touches the recipient either way)
// while the SENDERS diverge by exactly TransferFee() — Transfer's sender
// pays amount+fee, InternalMove's sender pays amount only.
func TestInternalMove_NoFee_DiffersFromTransfer(t *testing.T) {
	transferSender := "did:obs:im-cmp-transfer-sender"
	moveSender := "did:obs:im-cmp-move-sender"
	transferRecipient := "did:obs:im-cmp-transfer-recipient"
	moveRecipient := "did:obs:im-cmp-move-recipient"

	amount := obs(100)
	fund(t, transferSender, obs(1000))
	fund(t, moveSender, obs(1000))

	if _, err := token.Transfer(context.Background(), transferSender, transferRecipient, amount, "compare"); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if _, err := token.InternalMove(context.Background(), moveSender, moveRecipient, amount, "escrow_hold"); err != nil {
		t.Fatalf("internalMove: %v", err)
	}

	// Recipients: both get exactly `amount` — no divergence on the credit side.
	transferRecvBal := mustBalance(t, transferRecipient)
	moveRecvBal := mustBalance(t, moveRecipient)
	if transferRecvBal.Cmp(amount) != 0 {
		t.Errorf("transfer recipient balance = %s, want %s", transferRecvBal, amount)
	}
	if moveRecvBal.Cmp(amount) != 0 {
		t.Errorf("internalMove recipient balance = %s, want %s", moveRecvBal, amount)
	}

	// Senders: Transfer's sender paid amount+fee; InternalMove's sender paid
	// only amount. The difference between what each sender has left is
	// exactly TransferFee().
	transferSenderBal := mustBalance(t, transferSender)
	moveSenderBal := mustBalance(t, moveSender)
	fee := token.TransferFee()
	gotDiff := new(big.Int).Sub(moveSenderBal, transferSenderBal)
	if gotDiff.Cmp(fee) != 0 {
		t.Errorf("moveSender - transferSender = %s, want exactly TransferFee() = %s (internalMove must not charge the fee Transfer charges)",
			gotDiff, fee)
	}

	// Fee pool must not have grown from the internalMove side at all — only
	// Transfer's poolPart should have landed there. Isolate by diffing
	// against a fresh baseline captured after Transfer but that's already
	// folded into feepool's running total across the whole test binary, so
	// instead assert the exact poolPart delta by re-deriving it: run a second
	// InternalMove of the same amount and confirm the fee pool balance is
	// unchanged across that call.
	poolBefore := mustBalance(t, token.FeePoolDID)
	fund(t, moveSender, amount) // top up so the second move has funds
	if _, err := token.InternalMove(context.Background(), moveSender, moveRecipient, amount, "escrow_release"); err != nil {
		t.Fatalf("second internalMove: %v", err)
	}
	poolAfter := mustBalance(t, token.FeePoolDID)
	if poolAfter.Cmp(poolBefore) != 0 {
		t.Errorf("fee pool balance changed by internalMove: before=%s after=%s (internalMove must never touch FeePoolDID)",
			poolBefore, poolAfter)
	}
}

// TestInternalMove_NoSupplyChange confirms internalMove never touches
// circulating/burned — only Transfer's fee-burn does.
func TestInternalMove_NoSupplyChange(t *testing.T) {
	sender := "did:obs:im-supply-sender"
	recipient := "did:obs:im-supply-recipient"
	fund(t, sender, obs(50))

	supBefore := mustSupply(t)
	if _, err := token.InternalMove(context.Background(), sender, recipient, obs(10), "escrow_hold"); err != nil {
		t.Fatalf("internalMove: %v", err)
	}
	supAfter := mustSupply(t)

	if supAfter.Circulating != supBefore.Circulating {
		t.Errorf("circulating changed: before=%s after=%s", supBefore.Circulating, supAfter.Circulating)
	}
	if supAfter.Burned != supBefore.Burned {
		t.Errorf("burned changed: before=%s after=%s", supBefore.Burned, supAfter.Burned)
	}
}

func TestInternalMove_InsufficientBalance(t *testing.T) {
	alice := "did:obs:im-poor-alice"
	bob := "did:obs:im-poor-bob"
	fund(t, alice, obs(50))

	_, err := token.InternalMove(context.Background(), alice, bob, obs(100), "escrow_hold")
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}

	// No partial movement: alice untouched, bob untouched.
	if got := mustBalance(t, alice); got.Cmp(obs(50)) != 0 {
		t.Errorf("alice balance changed on failed internalMove: %s", got)
	}
	if got := mustBalance(t, bob); got.Sign() != 0 {
		t.Errorf("bob balance changed on failed internalMove: %s", got)
	}

	// Edge case: balance exactly equal to amount must succeed (no fee to
	// leave room for, unlike Transfer's amount+fee requirement).
	carol := "did:obs:im-exact-carol"
	dave := "did:obs:im-exact-dave"
	fund(t, carol, obs(10))
	if _, err := token.InternalMove(context.Background(), carol, dave, obs(10), "escrow_hold"); err != nil {
		t.Errorf("expected exact-balance internalMove to succeed (no fee headroom needed), got: %v", err)
	}
	if got := mustBalance(t, carol); got.Sign() != 0 {
		t.Errorf("carol balance = %s, want 0", got)
	}
	if got := mustBalance(t, dave); got.Cmp(obs(10)) != 0 {
		t.Errorf("dave balance = %s, want %s", got, obs(10))
	}
}

// TestInternalMove_Atomicity mirrors TestTransfer_Atomicity: a failed
// internalMove must leave zero partial state anywhere it touches.
func TestInternalMove_Atomicity(t *testing.T) {
	alice := "did:obs:im-atomic-alice"
	bob := "did:obs:im-atomic-bob"
	fund(t, alice, obs(1)) // far short of the 50 OBS move below

	aliceBefore := mustBalance(t, alice)
	bobBefore := mustBalance(t, bob)
	supBefore := mustSupply(t)

	if _, err := token.InternalMove(context.Background(), alice, bob, obs(50), "escrow_hold"); err == nil {
		t.Fatal("expected failure")
	}

	if mustBalance(t, alice).Cmp(aliceBefore) != 0 {
		t.Error("alice balance mutated by failed internalMove")
	}
	if mustBalance(t, bob).Cmp(bobBefore) != 0 {
		t.Error("bob balance mutated by failed internalMove")
	}
	supAfter := mustSupply(t)
	if supAfter.Circulating != supBefore.Circulating || supAfter.Burned != supBefore.Burned {
		t.Errorf("supply mutated by failed internalMove: before=%+v after=%+v", supBefore, supAfter)
	}

	hist, _ := token.History(alice, 50)
	for _, h := range hist {
		if h.TxType == "escrow_hold" {
			t.Errorf("failed internalMove left a tx row: %+v", h)
		}
	}
}

func TestInternalMove_RejectsBadInput(t *testing.T) {
	ctx := context.Background()
	funded := "did:obs:im-badinput-funded"
	fund(t, funded, obs(10))

	cases := []struct {
		name   string
		from   string
		to     string
		amount *big.Int
		txType string
	}{
		{"zero amount", funded, "did:obs:im-badinput-to", big.NewInt(0), "escrow_hold"},
		{"negative amount", funded, "did:obs:im-badinput-to", big.NewInt(-1), "escrow_hold"},
		{"nil amount", funded, "did:obs:im-badinput-to", nil, "escrow_hold"},
		{"empty from", "", "did:obs:im-badinput-to", obs(1), "escrow_hold"},
		{"empty to", funded, "", obs(1), "escrow_hold"},
		{"self move", funded, funded, obs(1), "escrow_hold"},
		{"empty txType", funded, "did:obs:im-badinput-to", obs(1), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := token.InternalMove(ctx, c.from, c.to, c.amount, c.txType); err == nil {
				t.Errorf("expected error for %s", c.name)
			}
		})
	}
}

// TestInternalMove_ConcurrentSpendsFromSameAccount is the race test the plan
// calls for. It runs under `go test -race` (Go's data-race detector, catches
// unsynchronized access to Go-level shared state) and, independently, proves
// the DB-level serialization is correct: fund a sender with exactly enough
// for K of N concurrent InternalMove attempts, release all N at once, and
// assert exactly K succeed with the sender landing at precisely 0 — a lost
// update (two goroutines both reading the pre-debit balance, both passing
// the sufficiency check) would let more than K succeed or leave the sender's
// final balance non-zero. This exercises txBalance's row-lock query, the
// same helper Transfer uses (FOR UPDATE on Postgres; on this suite's SQLite,
// db.DB's MaxOpenConns(1) is the serialization mechanism — see token.go's
// package doc comment — so a bug in the tx boundary itself, not just a
// missing lock, is what this would actually catch here).
func TestInternalMove_ConcurrentSpendsFromSameAccount(t *testing.T) {
	const attempts = 8
	const affordable = 3 // K: exactly this many of `attempts` can succeed

	amount := obs(1)
	sender := "did:obs:im-race-sender"
	fund(t, sender, new(big.Int).Mul(amount, big.NewInt(affordable)))

	recipients := make([]string, attempts)
	succeeded := make([]bool, attempts)

	var ready sync.WaitGroup
	var wg sync.WaitGroup
	start := make(chan struct{})

	ready.Add(attempts)
	for i := 0; i < attempts; i++ {
		recipients[i] = fmt.Sprintf("did:obs:im-race-recipient-%d", i)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			_, err := token.InternalMove(context.Background(), sender, recipients[i], amount, "escrow_hold")
			succeeded[i] = err == nil
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	successCount := 0
	for _, ok := range succeeded {
		if ok {
			successCount++
		}
	}
	if successCount != affordable {
		t.Fatalf("%d/%d internalMove attempts succeeded, want exactly %d", successCount, attempts, affordable)
	}

	finalSenderBal := mustBalance(t, sender)
	if finalSenderBal.Sign() != 0 {
		t.Fatalf("sender balance = %s, want 0 (exactly %d debits of %s each from a balance of exactly %d*%s — a lost update would leave this non-zero)",
			finalSenderBal, affordable, amount, affordable, amount)
	}

	creditedCount := 0
	for i, r := range recipients {
		bal := mustBalance(t, r)
		wantCredited := succeeded[i]
		gotCredited := bal.Cmp(amount) == 0
		if wantCredited != gotCredited {
			t.Fatalf("recipient %d: succeeded=%v but balance=%s (want %s if succeeded, 0 otherwise)",
				i, succeeded[i], bal, amount)
		}
		if gotCredited {
			creditedCount++
		}
	}
	if creditedCount != affordable {
		t.Fatalf("%d recipients credited, want %d", creditedCount, affordable)
	}
}
