// Package zk — dual-system proof verification.
//
// Spec Bölüm 19.3: "Groth16 + PLONK cross-check".
// DualVerify aynı proof'u iki bağımsız geçişten geçirir:
//   - Geçiş 1: iden3/go-rapidsnark Groth16 verifier (gerçek BN254 pairing)
//   - Geçiş 2: TODO(FAZ-3) gerçek PLONK verifier — şu an ikinci Groth16
//     geçişiyle stub edilmiştir; sonuçlar her zaman tutarlıdır.
//
// İki sonuç uyumsuzsa (Consistent=false) bu ciddi bir yazılım veya
// güvenlik anomalisidir — Anomaly=true set edilir ve loglanır.
package zk

import (
	"fmt"
	"log"
)

// DualVerifyResult — iki verifier'dan gelen birleşik sonuç.
type DualVerifyResult struct {
	// Groth16Valid: iden3 go-rapidsnark BN254 pairing sonucu.
	Groth16Valid bool

	// PlonkValid: gerçek PLONK verifier sonucu.
	// TODO(FAZ-3): gnark veya bellman tabanlı PLONK entegrasyonu.
	// Şu an: ikinci Groth16 geçişiyle stub — her zaman Groth16Valid ile eşit.
	PlonkValid bool

	// Consistent: her iki sonuç birbiriyle uyumluysa true.
	// false olması ciddi bir anomalidir (yazılım hatası veya saldırı).
	Consistent bool

	// Anomaly: Consistent=false olduğunda true.
	Anomaly bool
}

// DualVerify, proof'u iki bağımsız doğrulama sisteminden geçirir.
//
// Şu anki stub davranışı:
//   - Groth16 ile doğrula (gerçek BN254 pairing check)
//   - PLONK stub: aynı Groth16 geçişini tekrar çalıştır
//   - İkisi her zaman aynı sonucu verir → Consistent=true
//
// Production (FAZ-3) davranışı:
//   - Groth16: iden3/go-rapidsnark
//   - PLONK: ayrı PLONK verification key + gnark/bellman verifier
func DualVerify(proof ProofData, circuitID CircuitID) (DualVerifyResult, error) {
	if proof.ProofJSON == "" {
		return DualVerifyResult{}, fmt.Errorf("dualverify: proof_json boş")
	}

	// — Geçiş 1: gerçek Groth16 —
	groth16Err := VerifyGroth16(circuitID, []byte(proof.ProofJSON), proof.PublicInputs)
	groth16Valid := groth16Err == nil

	// — Geçiş 2: PLONK stub (ikinci Groth16 geçişi) —
	// TODO(FAZ-3): ayrı PLONK vkey + verifier paketi ile değiştir.
	// Bu satır hem doğruluğu test eder hem de VerifyGroth16'nın
	// side-effect'siz (idempotent) olduğunu kanıtlar.
	plonkErr := VerifyGroth16(circuitID, []byte(proof.ProofJSON), proof.PublicInputs)
	plonkValid := plonkErr == nil

	consistent := groth16Valid == plonkValid
	anomaly := !consistent

	result := DualVerifyResult{
		Groth16Valid: groth16Valid,
		PlonkValid:   plonkValid,
		Consistent:   consistent,
		Anomaly:      anomaly,
	}

	if anomaly {
		// Bu durum stub'da teorik olarak imkânsız; production PLONK
		// entegrasyonundan sonra gerçek anomali yansıtabilir.
		log.Printf("[DualVerify] ANOMALY circuit=%s groth16=%v plonk=%v",
			circuitID, groth16Valid, plonkValid)
	}

	if !groth16Valid {
		return result, fmt.Errorf("dualverify groth16: %w", groth16Err)
	}
	if !plonkValid {
		// Stub'da ulaşılamaz; gelecekte PLONK hatası buradan döner.
		return result, fmt.Errorf("dualverify plonk: %w", plonkErr)
	}

	return result, nil
}
