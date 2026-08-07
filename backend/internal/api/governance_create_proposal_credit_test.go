package api_test

// community kredi kablolaması testi (Madde 4, community event) —
// HandleGovernanceCreateProposal başarılı governance.CreateProposal
// çağrısından sonra credit.AddEvent(EventCommunity) tetikliyor.
//
// governance.CreateProposal her çağrıda uuid.New() ile YENİ bir proposal id
// üretir ve 100 OBS burn eder — group_created'daki ON CONFLICT DO NOTHING
// no-op senaryosu burada mümkün değil, bu yüzden ayrı bir idempotency guard'ı
// gerekmiyor (raporda bu ayrıca not edildi).

import (
	"math/big"
	"testing"
	"time"

	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/governance"
	"obscura.network/core/internal/token"
)

// obsAmount returns n whole OBS as a smallest-unit *big.Int (n * 10^decimals).
func obsAmount(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(token.Decimals), nil))
}

// giveOBSBalance credits a DID a transparent OBS balance directly (bypasses
// transfer fee) and bumps obs_supply.circulating so token.Burn's invariant
// check passes — same pattern as internal/governance/governance_test.go's
// unexported `credit` helper, reimplemented here since it's a different
// package (can't reuse an unexported func across packages) and this file
// avoids the name `credit` for a local func since it would shadow the
// imported internal/credit package.
func giveOBSBalance(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	var curBal string
	err := db.DB.QueryRow(
		`SELECT transparent_balance FROM obs_accounts WHERE user_did = ?`, did).Scan(&curBal)
	cur := big.NewInt(0)
	if err == nil {
		if v, ok := new(big.Int).SetString(curBal, 10); ok {
			cur = v
		}
	}
	newBal := new(big.Int).Add(cur, amount)
	if _, err := db.DB.Exec(`
		INSERT INTO obs_accounts (user_did, transparent_balance, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			transparent_balance = excluded.transparent_balance,
			updated_at = excluded.updated_at`,
		did, newBal.String(), now); err != nil {
		t.Fatalf("giveOBSBalance %s: %v", did, err)
	}

	var supCirc string
	if err := db.DB.QueryRow(`SELECT circulating FROM obs_supply WHERE id = 1`).Scan(&supCirc); err != nil {
		t.Fatalf("obs_supply okunamadı: %v", err)
	}
	circ, ok := new(big.Int).SetString(supCirc, 10)
	if !ok {
		circ = big.NewInt(0)
	}
	newCirc := new(big.Int).Add(circ, amount)
	if _, err := db.DB.Exec(
		`UPDATE obs_supply SET circulating = ?, last_updated = ? WHERE id = 1`,
		newCirc.String(), now); err != nil {
		t.Fatalf("obs_supply güncellenemedi: %v", err)
	}
}

// stakeOBS inserts an active stake row so governance.StakedBalanceFn sees it.
func stakeOBS(t *testing.T, did string, amount *big.Int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	lockedUntil := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT INTO stakes (id, user_did, amount, stake_type, locked_until, apy_bps, status, created_at)
		VALUES (?, ?, ?, 'user', ?, 1000, 'active', ?)`,
		"stake-"+did, did, amount.String(), lockedUntil, now)
	if err != nil {
		t.Fatalf("stakeOBS %s: %v", did, err)
	}
}

// makeGovernanceEligible sets tier=Platinum + 5000 OBS staked + enough
// spendable balance to cover the 100 OBS proposal-creation burn.
func makeGovernanceEligible(t *testing.T, phone, did string) {
	t.Helper()
	setUserCreditScore(t, phone, 90, governance.MinTierToParticipate)
	stakeOBS(t, did, obsAmount(5000))
	giveOBSBalance(t, did, obsAmount(200)) // 100 OBS cost + headroom
}

func TestGovernanceCreateProposal_AwardsCommunityCredit(t *testing.T) {
	phone := "+905559990907"
	authToken := loginAndRegister(t, phone, "gov_proposal_credit_001")
	proposerDID := currentUserDID(t, authToken)
	makeGovernanceEligible(t, phone, proposerDID)

	resp, code := post(t, "/v1/governance/proposals", map[string]interface{}{
		"title":         "Kredi testi önerisi",
		"description":   "community event kablolaması doğrulaması",
		"proposal_type": "param",
	}, authToken)
	if code != 200 || !resp.Success {
		t.Fatalf("proposal oluşturulamadı (code=%d): %s", code, resp.Error)
	}

	var delta float64
	err := db.DB.QueryRow(
		`SELECT delta FROM credit_events WHERE user_did = ? AND event_type = ?`,
		proposerDID, credit.EventCommunity,
	).Scan(&delta)
	if err != nil {
		t.Fatalf("community credit_events satırı bulunamadı: %v", err)
	}
	if delta != credit.EventDeltas[credit.EventCommunity] {
		t.Errorf("delta = %v, beklenen %v", delta, credit.EventDeltas[credit.EventCommunity])
	}
}

// TestGovernanceCreateProposal_RepeatedProposalsClampAtGlobalCeiling —
// applyCategoryCap'in gerçek HTTP akışında (bypass edilmeden) devrede
// olduğunu kanıtlar: her proposal credit_events'e HAM delta'sıyla loglanır
// (audit tam), ama credit_score global [-20,100] tavanını aşamaz.
//
// NOT: CategoryCommunity cap'i (25) burada İZOLE test edilemiyor — governance
// eligibility tier>=4 (Platinum) gerektiriyor, ki bu credit_score>=80 demek
// (ScoreToTier), ve her credit.AddEvent çağrısı users.tier'ı
// ScoreToTier(credit_score)'a göre YENİDEN yazıyor. 80+25(cap)=105>100,
// yani global tavan kategori tavanından HER ZAMAN önce devreye giriyor bu
// akışta — kategori tavanının kendisi izole olarak zaten
// internal/credit/category_cap_test.go'da (5 test, plain+zk karışık dahil)
// kanıtlandı. Burada amaç sadece: gerçek proposal akışının o kod yolunu
// (AddEvent→AddCustomEvent→applyCategoryCap) BYPASS ETMEDİĞİNİ göstermek.
func TestGovernanceCreateProposal_RepeatedProposalsClampAtGlobalCeiling(t *testing.T) {
	phone := "+905559990908"
	tok := loginAndRegister(t, phone, "gov_proposal_credit_002")
	proposerDID := currentUserDID(t, tok)
	setUserCreditScore(t, phone, 90, governance.MinTierToParticipate)
	stakeOBS(t, proposerDID, obsAmount(5000))
	giveOBSBalance(t, proposerDID, obsAmount(1000))

	// 90 + 5 + 5 = 100 tam global tavanda — 3. proposal effectiveDelta=0
	// vermeli (skor zaten 100), ama yine de credit_events'e loglanmalı.
	for i := 0; i < 3; i++ {
		resp, code := post(t, "/v1/governance/proposals", map[string]interface{}{
			"title":         "Tavan testi önerisi",
			"description":   "global clamp doğrulaması",
			"proposal_type": "param",
		}, tok)
		if code != 200 || !resp.Success {
			t.Fatalf("proposal %d oluşturulamadı (code=%d): %s", i, code, resp.Error)
		}
	}

	var n int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM credit_events WHERE user_did = ? AND event_type = ?`,
		proposerDID, credit.EventCommunity,
	).Scan(&n); err != nil {
		t.Fatalf("credit_events sayımı: %v", err)
	}
	if n != 3 {
		t.Errorf("credit_events satır sayısı = %d, beklenen 3 (hepsi loglanmalı, clamp'e rağmen)", n)
	}

	var score float64
	if err := db.DB.QueryRow("SELECT credit_score FROM users WHERE did = ?", proposerDID).Scan(&score); err != nil {
		t.Fatalf("credit_score okunamadı: %v", err)
	}
	if score != 100.0 {
		t.Errorf("credit_score = %v, beklenen 100.0 (global tavanda kırpılmış)", score)
	}
}
