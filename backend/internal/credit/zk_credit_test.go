package credit

import (
	"testing"
)

// TestPointsForProofType — her proof tipi için beklenen puan değerini doğrular.
func TestPointsForProofType(t *testing.T) {
	cases := []struct {
		pt       ProofType
		expected float64
	}{
		{ProofTypeAge, 1.0},
		{ProofTypeActivity, 0.5},
		{ProofTypeNode, 10.0},
		{ProofTypeEndorsement, 1.0},
		{ProofTypeStreak, 0.5},
		{ProofTypeMsgCount, 0.1},
		{ProofType("unknown"), 0.0},
	}

	for _, tc := range cases {
		got := PointsForProofType(tc.pt)
		if got != tc.expected {
			t.Errorf("PointsForProofType(%q) = %v, beklenen %v", tc.pt, got, tc.expected)
		}
	}
}

// TestIsValidProofType — geçerli/geçersiz proof tiplerini doğrular.
func TestIsValidProofType(t *testing.T) {
	valid := []string{"age", "activity", "node", "endorsement", "streak", "msg_count"}
	for _, v := range valid {
		if !IsValidProofType(v) {
			t.Errorf("IsValidProofType(%q) = false, true beklendi", v)
		}
	}

	invalid := []string{"", "unknown", "credit_threshold", "vote", "identity"}
	for _, v := range invalid {
		if IsValidProofType(v) {
			t.Errorf("IsValidProofType(%q) = true, false beklendi", v)
		}
	}
}

// TestCooldownDuration — cooldown sürelerinin beklenen değerlerini doğrular.
func TestCooldownDuration(t *testing.T) {
	cases := []struct {
		pt           ProofType
		expectedDays int
	}{
		{ProofTypeAge, 30},
		{ProofTypeActivity, 1},
		{ProofTypeNode, 30},
		{ProofTypeEndorsement, 7},
		{ProofTypeStreak, 1},
		{ProofTypeMsgCount, 7},
	}

	for _, tc := range cases {
		d := cooldownDuration(tc.pt)
		expectedH := tc.expectedDays * 24
		gotH := int(d.Hours())
		if gotH != expectedH {
			t.Errorf("cooldownDuration(%q) = %dh, beklenen %dh", tc.pt, gotH, expectedH)
		}
	}
}

// TestClaimZKCredit_InvalidProofType — geçersiz proof tipi 400 benzeri hata döndürmeli.
func TestClaimZKCredit_InvalidProofType(t *testing.T) {
	_, err := ClaimZKCredit("did:obs:test", "invalid_type", "{}", "[]")
	if err == nil {
		t.Fatal("geçersiz proof tipi için hata beklendi, nil döndü")
	}
}

// TestClaimZKCredit_UnloadedCircuit — circuit yüklü değilse hata döndürmeli.
// (zk.IsCircuitKnown yüklü olmadığı için false döner — test ortamında vkey yok)
func TestClaimZKCredit_UnloadedCircuit(t *testing.T) {
	// Tüm proof tipleri valid ama test ortamında hiçbir vkey yüklü değil
	_, err := ClaimZKCredit("did:obs:test", ProofTypeAge, "{}", "[]")
	if err == nil {
		t.Fatal("yüklü olmayan circuit için hata beklendi, nil döndü")
	}
}

// TestProofCircuitMapping — tüm ProofType'ların geçerli CircuitID'ye sahip olduğunu doğrular.
func TestProofCircuitMapping(t *testing.T) {
	allTypes := []ProofType{
		ProofTypeAge,
		ProofTypeActivity,
		ProofTypeNode,
		ProofTypeEndorsement,
		ProofTypeStreak,
		ProofTypeMsgCount,
	}
	for _, pt := range allTypes {
		circID, ok := proofCircuit[pt]
		if !ok {
			t.Errorf("proofCircuit[%q] eksik", pt)
			continue
		}
		if circID == "" {
			t.Errorf("proofCircuit[%q] boş CircuitID", pt)
		}
	}
}
