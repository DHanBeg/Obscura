// did-backfill — one-time migration: recompute every user's DID from
// GenerateDID's new formula (SHA256(identity_key) instead of uuid.New()),
// then propagate old-DID -> new-DID across every column in the schema that
// stores a DID value (see the tables slice below — enumerated by hand from
// `PRAGMA table_info` against the live schema, not guessed from naming
// convention alone).
//
// Two things are DELIBERATELY NOT touched, because they cannot be correctly
// migrated:
//   - messages.owner_hash: for sealed-sender rows (from_did == ""), this is
//     a one-way hash of (old_did, msg_id). The original sender's DID isn't
//     recoverable from the row (that's the privacy guarantee), so it cannot
//     be recomputed under the new DID. Any such rows become permanently
//     unverifiable for moderation evidence purposes. The tool only WARNS if
//     any exist; it does not modify or delete them.
//   - binding_rotations.new_did_proof: a signature over the OLD new_did
//     string. Remapping old_did/new_did to their new-format equivalents
//     makes any existing proof cryptographically stale (it was signed over
//     a string that no longer matches). The tool warns if any rows exist.
//
// Usage:
//
//	go run ./cmd/did-backfill -db path/to/obscura.db           # dry-run (default)
//	go run ./cmd/did-backfill -db path/to/obscura.db -commit    # actually writes, in ONE transaction
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/identity"
)

// didColumn — one (table, column) pair known to store a DID value.
// Enumerated by hand from `PRAGMA table_info(<table>)` against every table
// in the live schema (71 tables checked, 47 matched) — see docs/sessions
// entry for this migration for the enumeration transcript.
type didColumn struct {
	table  string
	column string
}

var didColumns = []didColumn{
	{"active_calls", "caller_did"},
	{"active_calls", "callee_did"},
	{"airdrop_claims", "user_did"},
	{"binding_rotations", "old_did"},
	{"binding_rotations", "new_did"},
	{"bot_messages", "from_did"},
	{"bot_messages", "to_did"},
	{"bots", "owner_did"},
	{"complainant_credibility", "user_did"},
	{"contacts", "owner_did"},
	{"contacts", "contact_did"},
	{"conv_members", "user_did"},
	{"credit_events", "user_did"},
	{"credit_proof_claims", "did"},
	{"daily_activity", "user_did"},
	{"dao_guardians", "did"},
	{"dao_proposals", "proposer_did"},
	{"dao_vetoes", "guardian_did"},
	{"devices", "user_did"},
	{"event_attendees", "attendee_did"},
	{"events", "creator_did"},
	{"group_reports", "reporter_did"},
	{"messages", "from_did"},
	{"messages", "to_did"},
	{"mini_app_installs", "user_did"},
	{"mini_app_permissions_log", "user_did"},
	{"mini_app_sessions", "user_did"},
	{"mini_apps", "developer_did"},
	{"mls_group_members", "user_did"},
	{"mls_groups", "creator_did"},
	{"mls_key_packages", "user_did"},
	{"mls_messages", "sender_did"},
	{"mls_pending_proposals", "proposer_did"},
	{"mls_welcome_queue", "recipient_did"},
	{"node_uptime", "user_did"},
	{"obs_accounts", "user_did"},
	{"obs_transactions", "from_did"},
	{"obs_transactions", "to_did"},
	{"one_time_prekeys", "did"},
	{"pairing_requests", "user_did"},
	{"prekey_bundles", "did"},
	{"proof_log", "user_did"},
	{"proposals", "proposer_did"},
	{"scanner_flags", "user_did"},
	{"shard_manifests", "owner_did"},
	{"signal_sessions", "owner_did"},
	{"signal_sessions", "peer_did"},
	{"slash_events", "user_did"},
	{"slash_reviews", "reviewer_did"},
	{"spam_reports", "reporter_did"},
	{"spam_reports", "reported_did"},
	{"stakes", "user_did"},
	{"user_violations", "user_did"},
	{"zk_nullifiers", "user_did"},
	{"zk_proof_log", "user_did"},
	{"zk_proofs", "user_did"},
	// users.did is the primary record — handled separately (also updates odi).
}

type userRow struct {
	id          string
	oldDID      string
	identityKey string
}

func main() {
	dbPath := flag.String("db", "data/obscura.db", "SQLite DB path")
	commit := flag.Bool("commit", false, "actually write changes (default: dry-run, prints plan only)")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=10000")
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, did, identity_key FROM users`)
	if err != nil {
		log.Fatalf("users select: %v", err)
	}
	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.id, &u.oldDID, &u.identityKey); err != nil {
			log.Fatalf("users scan: %v", err)
		}
		users = append(users, u)
	}
	rows.Close()

	if len(users) == 0 {
		fmt.Println("0 kullanıcı — yapılacak bir şey yok.")
		return
	}

	type migration struct {
		user   userRow
		newDID string
		newODI string
	}
	var plan []migration
	seenNewDID := map[string]string{} // newDID -> user id, collision check
	for _, u := range users {
		if u.identityKey == "" {
			log.Fatalf("KRİTİK: user id=%s identity_key boş — DID türetilemez, migration DURDURULDU", u.id)
		}
		newDID := auth.GenerateDID(u.identityKey)
		if newDID == u.oldDID {
			fmt.Printf("id=%s zaten yeni şemada (%s) — atlanıyor\n", u.id, u.oldDID)
			continue
		}
		if prevOwner, dup := seenNewDID[newDID]; dup {
			log.Fatalf("KRİTİK: iki kullanıcı aynı yeni DID'e çakışıyor (%s ve %s -> %s) — migration DURDURULDU, identity_key tekrarını kontrol et", prevOwner, u.id, newDID)
		}
		seenNewDID[newDID] = u.id
		plan = append(plan, migration{user: u, newDID: newDID, newODI: identity.DeriveODI(newDID)})
	}

	if len(plan) == 0 {
		fmt.Println("Tüm kullanıcılar zaten yeni şemada — yapılacak bir şey yok.")
		return
	}

	fmt.Printf("=== PLAN: %d kullanıcı migrate edilecek ===\n", len(plan))
	for _, m := range plan {
		fmt.Printf("  %s\n    eski DID: %s\n    yeni DID: %s\n    yeni ODI: %s\n", m.user.id, m.user.oldDID, m.newDID, m.newODI)
	}

	// Sealed-sender owner_hash uyarısı — geri döndürülemez, sadece bilgilendirme.
	var sealedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE owner_hash != ''`).Scan(&sealedCount); err == nil && sealedCount > 0 {
		fmt.Printf("\n⚠️  UYARI: messages tablosunda %d satırda owner_hash dolu (sealed-sender mesajları).\n", sealedCount)
		fmt.Println("   Bu hash'ler eski DID'den türetildi ve GERİ HESAPLANAMAZ (orijinal gönderen")
		fmt.Println("   bilerek saklanmıyor — sealed-sender'ın gizlilik garantisi budur). Bu satırlar")
		fmt.Println("   için moderasyon kanıt doğrulaması migration SONRASI çalışmayacak. Dokunulmuyor.")
	}
	var rotationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM binding_rotations`).Scan(&rotationCount); err == nil && rotationCount > 0 {
		fmt.Printf("\n⚠️  UYARI: binding_rotations tablosunda %d satır var. old_did/new_did güncellenecek\n", rotationCount)
		fmt.Println("   ama new_did_proof bu satırların ESKİ new_did string'i üzerinden imzalanmış —")
		fmt.Println("   remap sonrası bu imza artık doğrulanamaz (stale). Sadece bilgilendirme.")
	}

	if !*commit {
		fmt.Println("\n(dry-run — hiçbir şey yazılmadı. Uygulamak için -commit geçin.)")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("tx begin: %v", err)
	}
	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()

	for _, m := range plan {
		if _, err := tx.Exec(`UPDATE users SET did = ?, odi = ? WHERE id = ?`, m.newDID, m.newODI, m.user.id); err != nil {
			log.Fatalf("users update (id=%s): %v", m.user.id, err)
		}
		for _, dc := range didColumns {
			res, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`, dc.table, dc.column, dc.column),
				m.newDID, m.user.oldDID,
			)
			if err != nil {
				log.Fatalf("%s.%s update (old=%s): %v", dc.table, dc.column, m.user.oldDID, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				fmt.Printf("  %s.%s: %d satır güncellendi (%s -> %s)\n", dc.table, dc.column, n, m.user.oldDID, m.newDID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("tx commit: %v", err)
	}
	rollback = false
	fmt.Println("\n✅ Migration commit edildi.")
	fmt.Println("NOT: Bu migration sonrası çıkan tüm JWT'ler eski DID'i taşıyor — geçersiz sayılmalı, kullanıcılar re-login olmalı.")
	os.Exit(0)
}
