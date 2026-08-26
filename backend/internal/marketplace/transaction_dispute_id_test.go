package marketplace_test

// #30 B9 parça 1 — TransactionInfo.DisputeID regresyon testleri.
// TransactionInfo.DisputeID'nin varlık nedeni: web tarafında transaction→
// dispute ID eşlemesi eskiden sadece localStorage'daydı (cihaz değişince
// kaybolur) — artık marketplace_transactions okunduğunda query-time
// subquery ile geliyor, migration/backfill yok.

import (
	"context"
	"testing"

	"obscura.network/core/internal/marketplace"
)

func TestTransactionInfo_DisputeID_EmptyBeforeDispute(t *testing.T) {
	seller := "did:obs:disp-id-nodispute-seller"
	buyer := "did:obs:disp-id-nodispute-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))

	txn, err := marketplace.GetTransactionForCaller(txID, buyer)
	if err != nil {
		t.Fatalf("GetTransactionForCaller: %v", err)
	}
	if txn.DisputeID != "" {
		t.Fatalf("dispute açılmadan DisputeID boş bekleniyordu, alınan=%q", txn.DisputeID)
	}
}

func TestTransactionInfo_DisputeID_SetAfterOpenDispute(t *testing.T) {
	seller := "did:obs:disp-id-open-seller"
	buyer := "did:obs:disp-id-open-buyer"
	makeUser(t, seller, 5)
	makeUser(t, buyer, 1)
	fund(t, buyer, obs(100))

	txID := purchaseHeld(t, seller, buyer, obs(5))

	dispute, err := marketplace.OpenDispute(context.Background(), txID, buyer, "hiç gelmedi")
	if err != nil {
		t.Fatalf("OpenDispute: %v", err)
	}

	// GetTransactionForCaller (buyer VE seller — dispute_id ödeme yapan
	// tarafın DID'ine göre gizlenmiyor, transaction'ın kendisi kadar görünür).
	for _, caller := range []string{buyer, seller} {
		txn, err := marketplace.GetTransactionForCaller(txID, caller)
		if err != nil {
			t.Fatalf("GetTransactionForCaller(%s): %v", caller, err)
		}
		if txn.DisputeID != dispute.ID {
			t.Fatalf("caller=%s: DisputeID = %q, want %q", caller, txn.DisputeID, dispute.ID)
		}
	}

	// ListTransactionsForUser — "Siparişlerim" listesinin aynı alanı taşıdığını
	// doğrula (loadTransaction ile ayrı bir sorgu yolu).
	list, err := marketplace.ListTransactionsForUser(buyer)
	if err != nil {
		t.Fatalf("ListTransactionsForUser: %v", err)
	}
	found := false
	for _, t2 := range list {
		if t2.ID == txID {
			found = true
			if t2.DisputeID != dispute.ID {
				t.Fatalf("list: DisputeID = %q, want %q", t2.DisputeID, dispute.ID)
			}
		}
	}
	if !found {
		t.Fatalf("ListTransactionsForUser sonucunda txID=%s bulunamadı", txID)
	}
}
