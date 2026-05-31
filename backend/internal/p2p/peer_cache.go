// Package p2p — son bağlanılan peer'ların SQLite cache'i
//
// AddPeerCacheMigration() migration'ı idempotent olarak db package'ının
// runMigrations() listesine eklenmesi yerine bu paketten çağrılır;
// böylece db ↔ p2p arası döngüsel import oluşmaz.
// discovery.go içinde DiscoverBootstrapPeers çağrıldığında bu migration
// otomatik olarak tetiklenir.
package p2p

import (
	"database/sql"
	"log"
	"time"
)

const peerCacheSchema = `
CREATE TABLE IF NOT EXISTS peer_cache (
  peer_id                TEXT PRIMARY KEY,
  multiaddr              TEXT NOT NULL,
  last_seen              INTEGER NOT NULL,
  successful_connections INTEGER DEFAULT 0
)`

// AddPeerCacheMigration peer_cache tablosunu idempotent olarak oluşturur.
// db.Init() çağrısından sonra, p2p.Start()'tan önce çağrılmalıdır.
//
// database.go'yu doğrudan düzenlemek yerine bu yol seçildi; böylece
// p2p paketi migration sistemine bağımlılık yaratmadan kendi şemasını yönetir.
func AddPeerCacheMigration(database *sql.DB) error {
	_, err := database.Exec(peerCacheSchema)
	if err != nil {
		return err
	}
	log.Println("P2P: peer_cache tablosu hazır")
	return nil
}

// SavePeer bir peer'ın multiaddr'ını ve son görülme zamanını kaydeder.
// Peer zaten varsa last_seen güncellenir, successful_connections artırılır.
func SavePeer(database *sql.DB, peerID, multiaddrStr string) {
	now := time.Now().Unix()
	_, err := database.Exec(`
		INSERT INTO peer_cache (peer_id, multiaddr, last_seen, successful_connections)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(peer_id) DO UPDATE SET
			multiaddr              = excluded.multiaddr,
			last_seen              = excluded.last_seen,
			successful_connections = successful_connections + 1
	`, peerID, multiaddrStr, now)
	if err != nil {
		log.Printf("P2P: peer_cache kayıt hatası (%s): %v", peerID, err)
	}
}

// LoadPeers peer_cache'den multiaddr listesi döner.
// Son 72 saat içinde görülen, en az bir başarılı bağlantısı olan peer'lar
// döndürülür; en son görülenden en eskiye doğru sıralanır.
// P2P node başlatılmadan önce bootstrap adayı olarak kullanılır.
func LoadPeers(database *sql.DB) []string {
	cutoff := time.Now().Add(-72 * time.Hour).Unix()
	rows, err := database.Query(`
		SELECT multiaddr FROM peer_cache
		WHERE last_seen >= ? AND successful_connections >= 1
		ORDER BY last_seen DESC
		LIMIT 50
	`, cutoff)
	if err != nil {
		log.Printf("P2P: peer_cache okuma hatası: %v", err)
		return nil
	}
	defer rows.Close()

	var addrs []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}
