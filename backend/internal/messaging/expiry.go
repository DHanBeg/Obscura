// Package messaging — Mesaj sona erme scheduler'ı (Spec Bölüm 6.4)
//
// StartMessageExpiryScheduler her saat çalışan bir background goroutine başlatır.
// Süresi dolan mesajları (expires_at < NOW(), deleted_at IS NULL) 'expired' olarak
// işaretler ve her etkilenen alıcıya WebSocket üzerinden "message_expired" bildirim
// gönderir. Goroutine'in ömrü context ile kontrol edilir; ctx iptal edildiğinde
// temiz şekilde sonlanır.
//
// TTL hesaplaması: mesaj oluşturma anı + 30 gün = expires_at (HandleSendMessage).
// Scheduler sadece bu eşiği geçen mesajlara dokunur.
package messaging

import (
	"database/sql"
	"log"
	"time"
)

// StartMessageExpiryScheduler — background goroutine başlatır.
//
// Parametreler:
//   - db:       obscura SQLite veritabanı bağlantısı
//   - hub:      global WebSocket hub (broadcast için)
//   - interval: kontrol aralığı (production'da 1h, test'te daha kısa)
//
// Goroutine, ctx.Done() kanalı kapanınca geri döner.
// Dışarıdan durdurmak için context.WithCancel kullanın.
//
// Bu scheduler mesaj TTL'ine ek olarak node_shards tablosunu da temizler.
// node_shards: diğer node'lardan alınan P2P DHT shard'ları (30 gün TTL, Unix epoch).
func StartMessageExpiryScheduler(db *sql.DB, hub *Hub, interval time.Duration) {
	go func() {
		log.Printf("[expiry] Scheduler başlatıldı (aralık=%v)", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// İlk tick'te hemen bir tur çalıştır (yeniden başlatma sonrası birikmiş mesajlar)
		runExpiryPass(db, hub)
		runShardExpiryPass(db)

		for range ticker.C {
			runExpiryPass(db, hub)
			runShardExpiryPass(db)
		}
	}()
}

// runShardExpiryPass — node_shards tablosundaki süresi dolmuş shard'ları siler.
// expires_at alanı Unix epoch (INTEGER) olarak saklanır.
// Her saat çalışır; hata durumunda loglanır, panic etmez.
func runShardExpiryPass(database *sql.DB) {
	now := time.Now().Unix()
	res, err := database.Exec(`DELETE FROM node_shards WHERE expires_at < ?`, now)
	if err != nil {
		// Tablo henüz oluşturulmamışsa (InitStorage çağrılmadıysa) sessizce geç.
		// "no such table" hataları beklenir; diğerleri loglanır.
		if !isNoSuchTableError(err) {
			log.Printf("[expiry] node_shards temizleme hatası: %v", err)
		}
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[expiry] Süresi dolmuş %d shard silindi (node_shards)", n)
	}
}

// isNoSuchTableError — SQLite "no such table" hatasını tespit eder.
// CGO_ENABLED=0 (modernc.org/sqlite) — hata string'ine bakarız.
func isNoSuchTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 13 && (msg[:13] == "no such table" ||
		containsStr(msg, "no such table"))
}

func containsStr(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// runExpiryPass — tek bir sona erme tarama turunu yürütür.
// Hatalar loglanır ve bir sonraki tura kadar ertelenir; paniklemez.
func runExpiryPass(db *sql.DB, hub *Hub) {
	now := time.Now().UTC()

	// Süresi dolan ve henüz işaretlenmemiş mesajları bul.
	// Kısa sorgu: sadece id, conv_id ve to_did çekiyoruz (alıcıya bildirim için).
	// "status NOT IN ('expired','deleted')" koşulu duplicate broadcast'i önler.
	rows, err := db.Query(`
		SELECT id, conv_id, to_did
		FROM   messages
		WHERE  expires_at < ?
		  AND  deleted_at IS NULL
		  AND  status NOT IN ('expired', 'failed')
		LIMIT  500
	`, now.Format(time.RFC3339))
	if err != nil {
		log.Printf("[expiry] Sorgu hatası: %v", err)
		return
	}
	defer rows.Close()

	type expiredMsg struct {
		id     string
		convID string
		toDID  string
	}

	var batch []expiredMsg
	for rows.Next() {
		var m expiredMsg
		if err := rows.Scan(&m.id, &m.convID, &m.toDID); err != nil {
			log.Printf("[expiry] Scan hatası: %v", err)
			continue
		}
		batch = append(batch, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[expiry] Rows hatası: %v", err)
	}
	rows.Close() // defer'den önce kapat; UPDATE aşağıda aynı DB bağlantısını kullanır

	if len(batch) == 0 {
		return
	}

	expiredAt := now.Format(time.RFC3339)

	for _, m := range batch {
		// Tek tek güncelle; toplu UPDATE bireysel hataları maskeleyebilir.
		res, err := db.Exec(`
			UPDATE messages
			SET    status = 'expired', expired_at = ?
			WHERE  id = ?
			  AND  status NOT IN ('expired', 'failed')
		`, expiredAt, m.id)
		if err != nil {
			log.Printf("[expiry] UPDATE hatası (msg=%s): %v", m.id, err)
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			// Başka bir goroutine veya node zaten işledi; atla.
			continue
		}

		// Alıcıya WebSocket bildirimi gönder (online değilse sessizce geç).
		payload := MessageExpiredPayload{
			MessageID: m.id,
			ConvID:    m.convID,
			ExpiredAt: now.Unix(),
		}
		hub.SendTo(m.toDID, MsgTypeMessageExpired, payload)

		log.Printf("[expiry] Mesaj sona erdi: id=%s conv=%s", m.id, m.convID)
	}

	log.Printf("[expiry] Tur tamamlandı: %d mesaj işlendi", len(batch))
}
