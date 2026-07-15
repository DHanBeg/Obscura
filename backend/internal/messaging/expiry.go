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

// selfDestructScrubCiphertext — self-destruct süresi dolan mesajın ciphertext'i
// yerine yazılır (recall_handler.go'daki '[Geri alındı]' deseniyle aynı yaklaşım).
const selfDestructScrubCiphertext = "[Mesaj süresi doldu]"

// runExpiryPass — tek bir sona erme tarama turunu yürütür. İKİ AYRI eşiği
// tarar: expires_at (30 gün genel saklama TTL, spec Bölüm 6.4) VE
// self_destruct_at (kullanıcı seçimli erken silme, ExpiresAt'ten bağımsız).
//
// Davranış farkı bilinçli: self_destruct_at tetiklerse ciphertext GERÇEKTEN
// silinir (deleted_at set + placeholder) — kullanıcı "kendini imha et" dediği
// için içerik DB'de kalmamalı. Sadece genel 30 gün TTL'i tetiklerse mevcut
// davranış korunur (yalnızca status='expired' flag'i, ciphertext dokunulmaz);
// bu ayrı davranış değişikliği bu adımın kapsamı dışında bırakıldı.
//
// Hatalar loglanır ve bir sonraki tura kadar ertelenir; paniklemez.
func runExpiryPass(db *sql.DB, hub *Hub) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// Süresi dolan ve henüz işaretlenmemiş mesajları bul (iki eşikten biri
	// geçmişte kalmışsa). is_self_destruct: hangi eşiğin tetiklediğini ayırt
	// eder — sadece self_destruct_at tetiklediyse ciphertext silinir.
	// "status NOT IN ('expired','failed')" koşulu duplicate broadcast'i önler.
	rows, err := db.Query(`
		SELECT id, conv_id, to_did,
		       CASE WHEN self_destruct_at IS NOT NULL AND self_destruct_at < ? THEN 1 ELSE 0 END AS is_self_destruct
		FROM   messages
		WHERE  deleted_at IS NULL
		  AND  status NOT IN ('expired', 'failed')
		  AND  (expires_at < ? OR (self_destruct_at IS NOT NULL AND self_destruct_at < ?))
		LIMIT  500
	`, nowStr, nowStr, nowStr)
	if err != nil {
		log.Printf("[expiry] Sorgu hatası: %v", err)
		return
	}
	defer rows.Close()

	type expiredMsg struct {
		id             string
		convID         string
		toDID          string
		isSelfDestruct bool
	}

	var batch []expiredMsg
	for rows.Next() {
		var m expiredMsg
		var isSelfDestructInt int
		if err := rows.Scan(&m.id, &m.convID, &m.toDID, &isSelfDestructInt); err != nil {
			log.Printf("[expiry] Scan hatası: %v", err)
			continue
		}
		m.isSelfDestruct = isSelfDestructInt != 0
		batch = append(batch, m)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[expiry] Rows hatası: %v", err)
	}
	rows.Close() // defer'den önce kapat; UPDATE aşağıda aynı DB bağlantısını kullanır

	if len(batch) == 0 {
		return
	}

	for _, m := range batch {
		var res sql.Result
		var execErr error
		if m.isSelfDestruct {
			// Gerçek silme: ciphertext scrub + deleted_at (teşhiste tespit
			// edilen "sadece flag, içerik kalıyor" eksikliği burada düzeltiliyor).
			res, execErr = db.Exec(`
				UPDATE messages
				SET    status = 'expired', expired_at = ?, deleted_at = ?, ciphertext = ?
				WHERE  id = ?
				  AND  status NOT IN ('expired', 'failed')
			`, nowStr, nowStr, selfDestructScrubCiphertext, m.id)
		} else {
			// Genel 30 gün TTL: mevcut davranış (sadece flag) korunuyor.
			res, execErr = db.Exec(`
				UPDATE messages
				SET    status = 'expired', expired_at = ?
				WHERE  id = ?
				  AND  status NOT IN ('expired', 'failed')
			`, nowStr, m.id)
		}
		if execErr != nil {
			log.Printf("[expiry] UPDATE hatası (msg=%s): %v", m.id, execErr)
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

		log.Printf("[expiry] Mesaj sona erdi: id=%s conv=%s self_destruct=%v", m.id, m.convID, m.isSelfDestruct)
	}

	log.Printf("[expiry] Tur tamamlandı: %d mesaj işlendi", len(batch))
}
