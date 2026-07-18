// Package scanner — 7/24 merkezi tarama motoru.
//
// Her tarayıcı bağımsız goroutine'de çalışır; biri hata verirse diğerleri
// etkilenmez. Tüm hatalar log'a yazılır, sistem durmaz. Panic yok.
package scanner

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// Scanner — merkezi 7/24 tarama motoru
type Scanner struct {
	db *sql.DB
}

// New — yeni Scanner oluşturur.
func New(db *sql.DB) *Scanner {
	return &Scanner{db: db}
}

// Start — tüm tarayıcıları goroutine olarak başlatır.
// ctx iptal edildiğinde tüm goroutine'ler durur.
func (s *Scanner) Start(ctx context.Context) {
	log.Println("[Scanner] 7/24 tarama sistemi aktif")
	go s.runLoop(ctx, "mesaj", 30*time.Second, s.scanMessages)
	go s.runLoop(ctx, "grup", 60*time.Second, s.scanGroups)
	go s.runLoop(ctx, "kullanici", 120*time.Second, s.scanUsers)
	go s.runLoop(ctx, "node", 15*time.Second, s.scanNodes)
	go s.runLoop(ctx, "anomali", 45*time.Second, s.scanAnomalies)
	go s.runLoop(ctx, "spam", 30*time.Second, s.scanSpamReports)
}

// runLoop — verilen fonksiyonu interval aralıklarla çalıştırır.
// ctx.Done() sinyalinde düzgünce durur.
func (s *Scanner) runLoop(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Scanner] %s tarayicisi durduruldu", name)
			return
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				log.Printf("[Scanner] %s hatasi: %v", name, err)
			}
		}
	}
}

// msgFlag — scanMessages tarafından toplanan spam mesajı bilgisi.
type msgFlag struct {
	id      string
	fromDID string
	reason  string
}

// scanMessages — son 30 saniyedeki mesajları spam/içerik açısından tarar.
//
// Tablo: messages
//   - from_did  TEXT — gönderici (sender_did değil)
//   - sent_at   TEXT — ISO-8601 tarih, datetime() ile karşılaştırılır
//   - deleted_at TEXT — NULL ise silinmemiş
//
// Adım 9 — sealed-sender (encryption_type='sealed') mesajlar bilerek DIŞLANIR:
//  1. isSpamContent zaten AES-256-GCM ciphertext üzerinde ÇALIŞMAZ (IND-CPA:
//     şifreli bayt dizisinin düz metin anahtar kelimeleriyle bağı yok — kanıt:
//     scanner_test.go TestIsSpamContent_NeverMatchesRealCiphertext, gerçek
//     "free money" gibi ifadeleri şifreleyip hiçbirinin tetiklenmediğini
//     gösteriyor). Bu, sealed-sender'dan BAĞIMSIZ, önceden de geçerli bir
//     ölü-kod durumu — motorun asıl yeniden tasarımı madde 3'ün işi.
//  2. Sealed'a özgü İKİNCİ bir sorun ÜSTÜNE biner: from_did DB'de boş
//     saklanır (ADR-0016) — flagMessage'ın kredi cezası hedefsiz kalırdı.
//     Gerçek göndereni öğrenmenin tek yolu, sealed-sender'ın tam olarak
//     engellemeye çalıştığı kalıcı sunucu-taraflı bağlantıyı kurmak olurdu
//     (Adım 8'in fanout-atlama kararıyla aynı gerekçe). O yüzden içerik
//     sinyali bir gün gerçek hale gelse bile (madde 3), sealed satırlar
//     BURADAN elenmeye devam etmeli — o zaman ihtiyaç duyulacak şey
//     decrypt DEĞİL, Adım 8'deki gibi rapor/kanıt tabanlı ayrı bir akış.
//
// Rows kapatıldıktan SONRA flagleme yapılır; aksi halde MaxOpenConns=1 altında
// QueryContext bağlantısı Exec'i bloklar (deadlock).
func (s *Scanner) scanMessages(ctx context.Context) error {
	cutoff := time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_did, ciphertext
		FROM messages
		WHERE sent_at > ? AND deleted_at IS NULL AND encryption_type != 'sealed'
		LIMIT 500`, cutoff)
	if err != nil {
		return fmt.Errorf("scanMessages sorgu: %w", err)
	}

	var flagged []msgFlag
	for rows.Next() {
		var id, fromDID, content string
		if err := rows.Scan(&id, &fromDID, &content); err != nil {
			continue
		}
		if isSpamContent(content) {
			flagged = append(flagged, msgFlag{id: id, fromDID: fromDID, reason: "spam_content"})
		}
	}
	rowsErr := rows.Err()
	rows.Close() // rows kapatıldıktan sonra Exec çağırılabilir

	for _, f := range flagged {
		s.flagMessage(f.id, f.fromDID, f.reason)
	}
	if len(flagged) > 0 {
		log.Printf("[Scanner] Mesaj: %d supheyli icerik tespit edildi", len(flagged))
	}
	return rowsErr
}

// groupFlag — scanGroups tarafından toplanan grup bilgisi.
type groupFlag struct {
	groupID string
	cnt     int
}

// scanGroups — 24 saatte 3+ rapor alan grupları incelemeye alır.
//
// Tablo: group_reports
//   - group_id   TEXT
//   - created_at INTEGER (Unix timestamp — migration 087)
func (s *Scanner) scanGroups(ctx context.Context) error {
	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT group_id, COUNT(*) AS cnt FROM group_reports
		WHERE created_at > ? GROUP BY group_id HAVING cnt >= 3`, cutoff)
	if err != nil {
		return fmt.Errorf("scanGroups sorgu: %w", err)
	}

	var groups []groupFlag
	for rows.Next() {
		var groupID string
		var cnt int
		if err := rows.Scan(&groupID, &cnt); err != nil {
			continue
		}
		groups = append(groups, groupFlag{groupID: groupID, cnt: cnt})
	}
	rowsErr := rows.Err()
	rows.Close()

	now := time.Now().Unix()
	for _, g := range groups {
		_, execErr := s.db.ExecContext(ctx, `
			INSERT INTO group_moderation (group_id, is_reviewing, report_count, last_review)
			VALUES (?, 1, ?, ?)
			ON CONFLICT(group_id) DO UPDATE SET
				is_reviewing = 1,
				report_count = excluded.report_count,
				last_review  = excluded.last_review`,
			g.groupID, g.cnt, now)
		if execErr != nil {
			log.Printf("[Scanner] grup moderasyon guncelleme hatasi %s: %v", truncate(g.groupID, 8), execErr)
			continue
		}
		log.Printf("[Scanner] Grup incelemeye alindi: %s... (%d rapor)", truncate(g.groupID, 8), g.cnt)
	}
	return rowsErr
}

// userRateFlag — scanUsers tarafından toplanan kullanıcı bilgisi.
type userRateFlag struct {
	did string
	cnt int
}

// scanUsers — 60 saniyede 100'den fazla mesaj gönderen kullanıcıları tespit eder.
//
// Tablo: messages
//   - from_did TEXT — gönderici
//   - sent_at  TEXT — ISO-8601
//
// Adım 9 — sealed mesajlar dışlanır: from_did hepsinde "" olduğundan
// GROUP BY from_did bunları TEK bir sahte "kullanıcı" (did="") altında
// toplar — hem yanlış (kimseye ait olmayan bir bayrak/log) hem kör (gerçek
// tek-kullanıcı taşkını diğer sealed göndericilerin trafiğine karışıp
// asla ayırt edilemez). Gerçek per-gönderen hız sınırı sealed'da sunucu
// tarafında MÜMKÜN DEĞİL — göndereni tekilleştirecek kalıcı bir kimlik
// üretmek sealed-sender'ın (Adım 6/7) engellediği şeyin ta kendisi olurdu.
func (s *Scanner) scanUsers(ctx context.Context) error {
	cutoff := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT from_did, COUNT(*) AS cnt FROM messages
		WHERE sent_at > ? AND encryption_type != 'sealed' GROUP BY from_did HAVING cnt > 100`, cutoff)
	if err != nil {
		return fmt.Errorf("scanUsers sorgu: %w", err)
	}

	var flagged []userRateFlag
	for rows.Next() {
		var did string
		var cnt int
		if err := rows.Scan(&did, &cnt); err != nil {
			continue
		}
		flagged = append(flagged, userRateFlag{did: did, cnt: cnt})
	}
	rowsErr := rows.Err()
	rows.Close()

	for _, f := range flagged {
		log.Printf("[Scanner] Yuksek mesaj hizi: %s... (%d/dk)", truncate(f.did, 12), f.cnt)
		s.flagUser(ctx, f.did, "rate_limit_exceeded")
	}
	return rowsErr
}

// scanNodes — aktif node'ların son görülme zamanını kontrol eder.
// 5 dakikadır haber alınamayan node'lar 'inactive' olarak işaretlenir.
//
// Tablo: federation_nodes
//   - last_seen DATETIME (TEXT, ISO-8601 / SQLite datetime formatı)
//   - status    TEXT 'active' | 'inactive' | 'banned'
func (s *Scanner) scanNodes(ctx context.Context) error {
	threshold := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE federation_nodes
		SET status = 'inactive'
		WHERE status = 'active' AND last_seen < ?`, threshold)
	if err != nil {
		return fmt.Errorf("scanNodes guncelleme: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[Scanner] Node: %d pasif node isaretlendi", n)
	}
	return nil
}

// zkAnomalyFlag — scanAnomalies tarafından toplanan ZK anomali bilgisi.
type zkAnomalyFlag struct {
	did string
	cnt int
}

// scanAnomalies — kısa sürede çok fazla ZK proof gönderen kullanıcıları tespit eder.
// 45 saniyede 20'den fazla proof → zk_proof_flood bayrağı.
//
// Tablo: zk_proof_log
//   - user_did    TEXT
//   - verified_at INTEGER (Unix timestamp)
func (s *Scanner) scanAnomalies(ctx context.Context) error {
	cutoff := time.Now().Add(-45 * time.Second).Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_did, COUNT(*) AS cnt FROM zk_proof_log
		WHERE verified_at > ? GROUP BY user_did HAVING cnt > 20`, cutoff)
	if err != nil {
		return fmt.Errorf("scanAnomalies sorgu: %w", err)
	}

	var flagged []zkAnomalyFlag
	for rows.Next() {
		var did string
		var cnt int
		if err := rows.Scan(&did, &cnt); err != nil {
			continue
		}
		flagged = append(flagged, zkAnomalyFlag{did: did, cnt: cnt})
	}
	rowsErr := rows.Err()
	rows.Close()

	for _, f := range flagged {
		log.Printf("[Scanner] ZK Anomali: %s... %d proof/45sn", truncate(f.did, 12), f.cnt)
		s.flagUser(ctx, f.did, "zk_proof_flood")
	}
	return rowsErr
}

// spammerFlag — scanSpamReports tarafından toplanan spam fail bilgisi.
type spammerFlag struct {
	did string
	cnt int
}

// scanSpamReports — 24 saatte 5+ spam raporu alan kullanıcıların kredi puanını düşürür.
//
// Tablo: spam_reports
//   - reported_did TEXT
//   - created_at   TEXT (ISO-8601)
func (s *Scanner) scanSpamReports(ctx context.Context) error {
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT reported_did, COUNT(*) AS cnt FROM spam_reports
		WHERE created_at > ? GROUP BY reported_did HAVING cnt >= 5`, cutoff)
	if err != nil {
		return fmt.Errorf("scanSpamReports sorgu: %w", err)
	}

	var spammers []spammerFlag
	for rows.Next() {
		var did string
		var cnt int
		if err := rows.Scan(&did, &cnt); err != nil {
			continue
		}
		spammers = append(spammers, spammerFlag{did: did, cnt: cnt})
	}
	rowsErr := rows.Err()
	rows.Close()

	for _, sp := range spammers {
		if _, execErr := s.db.ExecContext(ctx,
			`UPDATE users SET credit_score = MAX(0, credit_score - 5) WHERE did = ?`, sp.did); execErr != nil {
			log.Printf("[Scanner] kredi dusurme hatasi %s: %v", truncate(sp.did, 12), execErr)
			continue
		}
		log.Printf("[Scanner] Spam: %s... %d rapor, puan dusuruldu", truncate(sp.did, 12), sp.cnt)
	}
	return rowsErr
}

// flagMessage — şüpheli mesajı is_flagged=1 olarak işaretler ve gönderenin
// kredi puanını 2 puan düşürür.
func (s *Scanner) flagMessage(msgID, fromDID, reason string) {
	if _, err := s.db.Exec(
		`UPDATE messages SET is_flagged = 1, flag_reason = ? WHERE id = ?`, reason, msgID); err != nil {
		log.Printf("[Scanner] flagMessage guncelleme hatasi %s: %v", truncate(msgID, 8), err)
	}
	if _, err := s.db.Exec(
		`UPDATE users SET credit_score = MAX(0, credit_score - 2) WHERE did = ?`, fromDID); err != nil {
		log.Printf("[Scanner] flagMessage kredi hatasi %s: %v", truncate(fromDID, 12), err)
	}
}

// flagUser — kullanıcıyı scanner_flags tablosuna ekler / günceller.
func (s *Scanner) flagUser(ctx context.Context, did, reason string) {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO scanner_flags (user_did, reason, flagged_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_did) DO UPDATE SET
			reason     = excluded.reason,
			flagged_at = excluded.flagged_at`,
		did, reason, time.Now().Unix()); err != nil {
		log.Printf("[Scanner] flagUser hatasi %s: %v", truncate(did, 12), err)
	}
}

// isSpamContent — basit anahtar-kelime bazlı spam tespiti.
// Mesaj içeriği şifreli (ciphertext) olduğu için bu kontrol yalnızca
// şifrelenmemiş/meta içerikler veya cleartext test mesajları için anlamlıdır.
func isSpamContent(content string) bool {
	if len(content) == 0 {
		return false
	}
	keywords := []string{
		"spam", "scam", "hack", "phishing",
		"click here", "free money", "crypto giveaway",
	}
	lower := strings.ToLower(content)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// truncate — string'i güvenli şekilde kısaltır (log için).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
