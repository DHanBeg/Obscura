// Package dao — Full DAO governance (FAZ 4, spec 12.4)
//
// FAZ 2'deki governance'ı genişletir:
//   - Çok imzalı (multi-sig) karar onayı
//   - Timelock zinciri (proposal → delay → execute)
//   - Yetkiyi merkezi ekipten DAO'ya devreder (< %10 merkezi kararlar)
//   - ZK vote ile anonim, ağırlıklı oy
//   - Guardian veto (acil güvenlik)
package dao

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/dbi"
)

// DAO parametreleri
const (
	TimelockDuration    = 48 * time.Hour  // Proposal kabul → execution arasında minimum süre
	QuorumPercent       = 10              // Toplam stake'in %10'u minimum katılım
	PassThresholdPct    = 51              // %51 evet → kabul
	SuperMajorityPct    = 67             // %67 → critical değişiklikler için
	GuardianVetoWindow  = 24 * time.Hour // Guardian veto penceresi
	MaxGuardians        = 5              // Maksimum guardian sayısı
)

// ProposalCategory — öneri kategorisi (kritiklik seviyesi belirler)
type ProposalCategory string

const (
	CategoryParam       ProposalCategory = "param"        // Parametre değişikliği — basit çoğunluk
	CategoryProtocol    ProposalCategory = "protocol"     // Protokol değişikliği — süper çoğunluk
	CategoryTreasury    ProposalCategory = "treasury"     // Hazine harcaması — süper çoğunluk
	CategoryGuardian    ProposalCategory = "guardian"     // Guardian değişikliği — süper çoğunluk
	CategoryEmergency   ProposalCategory = "emergency"    // Acil — guardian onayı yeterli
)

// DAOProposal — genişletilmiş öneri
type DAOProposal struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Category     ProposalCategory `json:"category"`
	ProposerDID  string           `json:"proposer_did"`
	Status       string           `json:"status"`    // draft|active|passed|rejected|timelocked|executed|vetoed
	YesWeight    float64          `json:"yes_weight"`
	NoWeight     float64          `json:"no_weight"`
	AbstainWeight float64         `json:"abstain_weight"`
	TotalStake   float64          `json:"total_stake"`   // toplam katılım (staked OBS)
	VetoCount    int              `json:"veto_count"`
	CreatedAt    time.Time        `json:"created_at"`
	VotingEndsAt time.Time        `json:"voting_ends_at"`
	TimelockAt   *time.Time       `json:"timelock_at,omitempty"`
	ExecutableAt *time.Time       `json:"executable_at,omitempty"`
	ExecutedAt   *time.Time       `json:"executed_at,omitempty"`
}

// Guardian — veto yetkisi olan güvenilir adres
type Guardian struct {
	DID       string    `json:"did"`
	AddedAt   time.Time `json:"added_at"`
	AddedBy   string    `json:"added_by"`
}

var (
	db dbi.Querier
	mu sync.RWMutex
)

// Init — DAO'yu başlat
func Init(database dbi.Querier) error {
	db = database
	return migrate()
}

func migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dao_proposals (
			id              TEXT PRIMARY KEY,
			title           TEXT NOT NULL,
			description     TEXT NOT NULL,
			category        TEXT NOT NULL DEFAULT 'param',
			proposer_did    TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'draft',
			yes_weight      REAL NOT NULL DEFAULT 0,
			no_weight       REAL NOT NULL DEFAULT 0,
			abstain_weight  REAL NOT NULL DEFAULT 0,
			total_stake     REAL NOT NULL DEFAULT 0,
			veto_count      INTEGER NOT NULL DEFAULT 0,
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			voting_ends_at  DATETIME NOT NULL,
			timelock_at     DATETIME,
			executable_at   DATETIME,
			executed_at     DATETIME
		);

		CREATE TABLE IF NOT EXISTS dao_guardians (
			did       TEXT PRIMARY KEY,
			added_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			added_by  TEXT NOT NULL DEFAULT 'genesis'
		);

		CREATE TABLE IF NOT EXISTS dao_vetoes (
			id          TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL,
			guardian_did TEXT NOT NULL,
			reason      TEXT NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(proposal_id, guardian_did)
		);
	`)
	return err
}

// CreateProposal — yeni DAO öneri oluştur (48 saat oylama)
func CreateProposal(proposerDID, title, description string, category ProposalCategory) (*DAOProposal, error) {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UTC()
	p := &DAOProposal{
		ID:           uuid.New().String(),
		Title:        title,
		Description:  description,
		Category:     category,
		ProposerDID:  proposerDID,
		Status:       "active",
		CreatedAt:    now,
		VotingEndsAt: now.Add(48 * time.Hour),
	}

	_, err := db.Exec(`
		INSERT INTO dao_proposals (id, title, description, category, proposer_did, status, created_at, voting_ends_at)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?)
	`, p.ID, p.Title, p.Description, string(p.Category), p.ProposerDID, p.CreatedAt, p.VotingEndsAt)
	if err != nil {
		return nil, fmt.Errorf("öneri oluşturma: %w", err)
	}
	return p, nil
}

// Finalize — oylama sonucunu hesapla, timelock başlat
func Finalize(proposalID string) (*DAOProposal, error) {
	mu.Lock()
	defer mu.Unlock()

	p, err := getProposal(proposalID)
	if err != nil || p == nil {
		return nil, fmt.Errorf("öneri bulunamadı")
	}
	if p.Status != "active" {
		return nil, fmt.Errorf("aktif olmayan öneri finalize edilemez")
	}
	if time.Now().UTC().Before(p.VotingEndsAt) {
		return nil, fmt.Errorf("oylama süresi bitmedi")
	}

	// Katılım kotası kontrolü
	participation := (p.YesWeight + p.NoWeight + p.AbstainWeight)
	if p.TotalStake > 0 && (participation/p.TotalStake)*100 < float64(QuorumPercent) {
		p.Status = "rejected"
		_, _ = db.Exec(`UPDATE dao_proposals SET status='rejected' WHERE id=?`, proposalID)
		return p, nil
	}

	// Eşik belirleme
	threshold := float64(PassThresholdPct)
	if p.Category == CategoryProtocol || p.Category == CategoryTreasury ||
		p.Category == CategoryGuardian {
		threshold = float64(SuperMajorityPct)
	}

	yesVotes := p.YesWeight
	totalVoted := p.YesWeight + p.NoWeight
	if totalVoted == 0 {
		p.Status = "rejected"
		_, _ = db.Exec(`UPDATE dao_proposals SET status='rejected' WHERE id=?`, proposalID)
		return p, nil
	}

	yesPct := (yesVotes / totalVoted) * 100
	now := time.Now().UTC()

	if yesPct >= threshold {
		// Kabul → timelock başlat
		timelockAt := now
		execAt := now.Add(TimelockDuration)
		p.Status = "timelocked"
		p.TimelockAt = &timelockAt
		p.ExecutableAt = &execAt
		_, _ = db.Exec(`UPDATE dao_proposals SET status='timelocked', timelock_at=?, executable_at=? WHERE id=?`,
			timelockAt, execAt, proposalID)
	} else {
		p.Status = "rejected"
		_, _ = db.Exec(`UPDATE dao_proposals SET status='rejected' WHERE id=?`, proposalID)
	}
	return p, nil
}

// Execute — timelock süresi dolduysa çalıştır
func Execute(proposalID string) (*DAOProposal, error) {
	mu.Lock()
	defer mu.Unlock()

	p, err := getProposal(proposalID)
	if err != nil || p == nil {
		return nil, fmt.Errorf("öneri bulunamadı")
	}
	if p.Status != "timelocked" {
		return nil, fmt.Errorf("öneri timelocked değil")
	}
	if p.ExecutableAt == nil || time.Now().UTC().Before(*p.ExecutableAt) {
		return nil, fmt.Errorf("timelock süresi bitmedi")
	}
	if p.VetoCount > 0 {
		return nil, fmt.Errorf("öneri guardian tarafından veto edildi")
	}

	now := time.Now().UTC()
	p.Status = "executed"
	p.ExecutedAt = &now
	_, _ = db.Exec(`UPDATE dao_proposals SET status='executed', executed_at=? WHERE id=?`, now, proposalID)
	return p, nil
}

// GuardianVeto — guardian veto uygular
func GuardianVeto(proposalID, guardianDID, reason string) error {
	mu.Lock()
	defer mu.Unlock()

	// Guardian kontrolü
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dao_guardians WHERE did=?`, guardianDID).Scan(&count); err != nil || count == 0 {
		return fmt.Errorf("yetkili guardian değil")
	}

	p, err := getProposal(proposalID)
	if err != nil || p == nil {
		return fmt.Errorf("öneri bulunamadı")
	}
	if p.Status != "timelocked" && p.Status != "active" {
		return fmt.Errorf("bu aşamada veto uygulanamaz")
	}

	// Veto penceresi kontrolü (timelock'tan sonra 24 saat)
	if p.TimelockAt != nil && time.Now().UTC().After(p.TimelockAt.Add(GuardianVetoWindow)) {
		return fmt.Errorf("guardian veto penceresi kapandı")
	}

	_, err = db.Exec(`
		INSERT OR IGNORE INTO dao_vetoes (id, proposal_id, guardian_did, reason, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, uuid.New().String(), proposalID, guardianDID, reason, time.Now().UTC())
	if err != nil {
		return err
	}

	_, _ = db.Exec(`UPDATE dao_proposals SET veto_count = veto_count + 1, status='vetoed' WHERE id=?`, proposalID)
	return nil
}

// ListProposals — tüm öneri listesi
func ListProposals(statusFilter string) ([]DAOProposal, error) {
	mu.RLock()
	defer mu.RUnlock()

	q := `SELECT id, title, description, category, proposer_did, status,
		yes_weight, no_weight, abstain_weight, total_stake, veto_count,
		created_at, voting_ends_at, timelock_at, executable_at, executed_at
		FROM dao_proposals`
	args := []interface{}{}
	if statusFilter != "" {
		q += " WHERE status=?"
		args = append(args, statusFilter)
	}
	q += " ORDER BY created_at DESC LIMIT 100"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proposals []DAOProposal
	for rows.Next() {
		var p DAOProposal
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Category,
			&p.ProposerDID, &p.Status, &p.YesWeight, &p.NoWeight, &p.AbstainWeight,
			&p.TotalStake, &p.VetoCount, &p.CreatedAt, &p.VotingEndsAt,
			&p.TimelockAt, &p.ExecutableAt, &p.ExecutedAt); err != nil {
			continue
		}
		proposals = append(proposals, p)
	}
	return proposals, nil
}

func getProposal(id string) (*DAOProposal, error) {
	var p DAOProposal
	err := db.QueryRow(`SELECT id, title, description, category, proposer_did, status,
		yes_weight, no_weight, abstain_weight, total_stake, veto_count,
		created_at, voting_ends_at, timelock_at, executable_at, executed_at
		FROM dao_proposals WHERE id=?`, id).Scan(
		&p.ID, &p.Title, &p.Description, &p.Category, &p.ProposerDID, &p.Status,
		&p.YesWeight, &p.NoWeight, &p.AbstainWeight, &p.TotalStake, &p.VetoCount,
		&p.CreatedAt, &p.VotingEndsAt, &p.TimelockAt, &p.ExecutableAt, &p.ExecutedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}
