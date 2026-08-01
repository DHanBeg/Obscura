package token_test

// ADIM 7 (ADR-0017, "sonradan-tasdik" deseni) — token.SetOpRecorder'ın
// SADECE başarılı (commit edilmiş) operasyonlarda çağrıldığını, başarısız/
// rollback edilen operasyonlarda ÇAĞRILMADIĞINI doğrular. Bakiye mantığına
// dokunulmadı — bu testler onu bir daha doğrulamıyor (token_test.go zaten
// yapıyor), sadece yeni hook'un davranışını doğruluyor.

import (
	"context"
	"testing"

	"obscura.network/core/internal/token"
)

// withOpRecorder, bir recorder kaydedip test sonunda nil'e resetler —
// paylaşılan package-level state testler arası sızmasın diye.
func withOpRecorder(t *testing.T) *[]string {
	t.Helper()
	var recorded []string
	token.SetOpRecorder(func(txID string) {
		recorded = append(recorded, txID)
	})
	t.Cleanup(func() { token.SetOpRecorder(nil) })
	return &recorded
}

func TestOpRecorder_CalledOnSuccessfulTransfer(t *testing.T) {
	recorded := withOpRecorder(t)
	alice := "did:obs:oprecorder-alice"
	bob := "did:obs:oprecorder-bob"
	fund(t, alice, obs(100))

	txID, err := token.Transfer(context.Background(), alice, bob, obs(10), "test")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}

	if len(*recorded) == 0 {
		t.Fatal("başarılı transfer sonrası opRecorder hiç çağrılmadı")
	}
	last := (*recorded)[len(*recorded)-1]
	if last != txID {
		t.Fatalf("opRecorder'a geçen txID (%s) Transfer'in döndürdüğü txID (%s) ile eşleşmiyor", last, txID)
	}
}

func TestOpRecorder_NotCalledOnFailedTransfer(t *testing.T) {
	recorded := withOpRecorder(t)
	poor := "did:obs:oprecorder-poor"
	rich := "did:obs:oprecorder-rich"
	// poor hesabına HİÇ fon yok — transfer başarısız olmalı.

	before := len(*recorded)
	_, err := token.Transfer(context.Background(), poor, rich, obs(10), "test")
	if err == nil {
		t.Fatal("bakiyesiz transfer başarılı oldu, hata beklenirdi")
	}
	if len(*recorded) != before {
		t.Fatalf("başarısız (rollback edilen) transfer opRecorder'ı çağırmamalıydı, önce=%d sonra=%d", before, len(*recorded))
	}
}

func TestOpRecorder_CalledOnSuccessfulMint(t *testing.T) {
	recorded := withOpRecorder(t)
	target := "did:obs:oprecorder-mint-target"

	txID, err := token.Mint(context.Background(), target, obs(5), "test mint")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	last := (*recorded)[len(*recorded)-1]
	if last != txID {
		t.Fatalf("opRecorder'a geçen txID (%s) Mint'in döndürdüğü txID (%s) ile eşleşmiyor", last, txID)
	}
}

func TestOpRecorder_CalledOnSuccessfulBurn(t *testing.T) {
	recorded := withOpRecorder(t)
	holder := "did:obs:oprecorder-burn-holder"
	fund(t, holder, obs(20))

	txID, err := token.Burn(context.Background(), holder, obs(5), "test burn")
	if err != nil {
		t.Fatalf("burn: %v", err)
	}
	last := (*recorded)[len(*recorded)-1]
	if last != txID {
		t.Fatalf("opRecorder'a geçen txID (%s) Burn'ün döndürdüğü txID (%s) ile eşleşmiyor", last, txID)
	}
}

func TestOpRecorder_NotCalledOnFailedBurn(t *testing.T) {
	recorded := withOpRecorder(t)
	poor := "did:obs:oprecorder-burn-poor"

	before := len(*recorded)
	_, err := token.Burn(context.Background(), poor, obs(999999), "test")
	if err == nil {
		t.Fatal("bakiyesiz burn başarılı oldu, hata beklenirdi")
	}
	if len(*recorded) != before {
		t.Fatalf("başarısız burn opRecorder'ı çağırmamalıydı, önce=%d sonra=%d", before, len(*recorded))
	}
}

func TestOpRecorder_NilRecorderDoesNotPanic(t *testing.T) {
	token.SetOpRecorder(nil)
	alice := "did:obs:oprecorder-nil-alice"
	fund(t, alice, obs(10)) // opRecorder nil iken Mint çağrılıyor — panic etmemeli
}
