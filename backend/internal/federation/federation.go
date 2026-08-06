// Package federation — permissionless node registration & peer directory (FAZ 3, spec 12.3)
package federation

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	dbpkg "obscura.network/core/internal/db"
	"obscura.network/core/internal/dbi"
)

// NodeRecord — kayıtlı node bilgisi
type NodeRecord struct {
	NodeID      string    `json:"node_id"`
	PeerAddr    string    `json:"peer_addr"`    // libp2p multiaddr
	HTTPURL     string    `json:"http_url"`     // opsiyonel HTTP fallback
	Pubkey      string    `json:"pubkey"`       // node imza anahtarı (Ed25519 hex)
	Version     string    `json:"version"`
	Region      string    `json:"region"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen    time.Time  `json:"last_seen"`
	Status      string    `json:"status"` // active, inactive, banned
	LatencyMs   int       `json:"latency_ms"` // son ölçülen round-trip (ms), hiç probe edilmediyse 0
	VRFPubkey   string    `json:"vrf_pubkey,omitempty"` // sequencer VRF public key (sıkıştırılmış P-256 nokta, hex) — ADR-0017 adım 5
}

// RegisterRequest — POST /v1/nodes/register body
type RegisterRequest struct {
	NodeID    string `json:"node_id"`
	PeerAddr  string `json:"peer_addr"`
	HTTPURL   string `json:"http_url"`
	Pubkey    string `json:"pubkey"`
	Version   string `json:"version"`
	Region    string `json:"region"`
	// Sig — Ed25519 imzası, SignaturePayload(req) üzerinden, Pubkey'nin
	// karşılık geldiği private key ile üretilir (hex). YUMUŞAK GEÇİŞ: boş
	// bırakılabilir (eski client'lar/manuel kayıtlar için, bkz. Register) —
	// ama doluysa MUTLAKA geçerli olmalı, geçersiz imza reddedilir.
	Sig string `json:"sig,omitempty"`
	// Timestamp — Unix saniye, Sig dolu olduğunda ZORUNLU (replay guard,
	// bkz. verifyRegistrationSignature). Sig boşsa kullanılmaz.
	Timestamp int64  `json:"timestamp,omitempty"`
	VRFPubkey string `json:"vrf_pubkey,omitempty"` // sequencer.Sequencer.VRFPublicKeyHex() — VRF proof doğrulama için
}

// SignaturePayload, RegisterRequest için imzalanacak/doğrulanacak kanonik
// byte dizisini üretir — imzalayan (bkz. cmd/node self-register akışı) ve
// doğrulayan (verifyRegistrationSignature) AYNI fonksiyonu kullanmalı, aksi
// halde format kayması sessizce her imzayı geçersiz kılar.
//
// VRFPubkey KASITLI OLARAK imzaya dahil: dahil edilmezse, biri BAŞKA bir
// node'un geçerli (node_id, peer_addr, pubkey) imzasını alıp üzerine
// KENDİ vrf_pubkey'ini ekleyerek o node_id adına sahte VRF proof'larını
// "meşrulaştırabilir" — sequencer paketindeki proof-doğrulama zincirini
// (handleIncomingVRFProof, gerçek pubkey'e güvenir) federation seviyesinden
// dolaylı atlatmanın yolu bu olurdu (bkz. ADR-0017 adım 5 adım-3).
// Timestamp de dahil: aksi halde eski geçerli bir kayıt sonsuza dek replay
// edilebilir.
func SignaturePayload(req RegisterRequest) []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|%d",
		req.NodeID, req.PeerAddr, req.Pubkey, req.VRFPubkey, req.Timestamp))
}

// maxSignatureAgeSeconds — Sig dolu olduğunda Timestamp'in kabul edilen
// azami yaşı (replay guard). Register seyrek çağrıldığı için peer_auth.go'daki
// 30sn'lik sıkı pencereden daha geniş (5dk) — hem ağ gecikmesi hem makul
// clock skew'i tolere eder.
const maxSignatureAgeSeconds = 5 * 60

// verifyRegistrationSignature, req.Sig'in req.Pubkey'e (SignaturePayload
// üzerinden) ve Timestamp'in kabul edilebilir bir pencerede olduğunu
// doğrular. SADECE req.Sig doluyken çağrılmalı (boş Sig = yumuşak geçiş,
// Register'da ayrı ele alınır).
func verifyRegistrationSignature(req RegisterRequest) error {
	if req.Timestamp == 0 {
		return fmt.Errorf("sig verilmiş ama timestamp eksik")
	}
	age := time.Now().UTC().Unix() - req.Timestamp
	if age < -maxSignatureAgeSeconds || age > maxSignatureAgeSeconds {
		return fmt.Errorf("timestamp kabul penceresi dışında (yaş=%ds, izin verilen=±%ds — replay veya clock skew)",
			age, maxSignatureAgeSeconds)
	}

	pubBytes, err := hex.DecodeString(req.Pubkey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("pubkey geçersiz format (Ed25519, %d byte hex bekleniyor)", ed25519.PublicKeySize)
	}
	sigBytes, err := hex.DecodeString(req.Sig)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("sig geçersiz format (Ed25519, %d byte hex bekleniyor)", ed25519.SignatureSize)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubBytes), SignaturePayload(req), sigBytes) {
		return fmt.Errorf("imza doğrulanamadı")
	}
	return nil
}

var (
	db   dbi.Querier
	mu   sync.RWMutex
)

// Init — federation'u başlat, DB bağlantısını kaydet
func Init(database dbi.Querier) error {
	db = database
	return migrate()
}

func migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS federation_nodes (
			node_id      TEXT PRIMARY KEY,
			peer_addr    TEXT NOT NULL,
			http_url     TEXT NOT NULL DEFAULT '',
			pubkey       TEXT NOT NULL,
			version      TEXT NOT NULL DEFAULT '',
			region       TEXT NOT NULL DEFAULT '',
			registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			status       TEXT NOT NULL DEFAULT 'active'
		)
	`)
	if err != nil {
		return err
	}
	// Idempotent ALTER: federation_nodes önceden bu kolon olmadan yaratılmış olabilir.
	// federation paketi kendi _migrations izleme tablosunu kullanmıyor (bkz. db.runMigrations),
	// bu yüzden "already exists" hatası burada tolere edilir — driver-bazı
	// (SQLSTATE Postgres'te, mesaj metni SQLite'ta), bkz. dbpkg.IsBenignSchemaError.
	driver := dbi.DriverFromEnv()
	_, err = db.Exec(`ALTER TABLE federation_nodes ADD COLUMN last_latency_ms INTEGER DEFAULT 0`)
	if err != nil && !dbpkg.IsBenignSchemaError(driver, err) {
		return err
	}
	_, err = db.Exec(`ALTER TABLE federation_nodes ADD COLUMN vrf_pubkey TEXT NOT NULL DEFAULT ''`)
	if err != nil && !dbpkg.IsBenignSchemaError(driver, err) {
		return err
	}
	return nil
}

// Register — yeni node kaydet veya güncelle (permissionless)
//
// İmza politikası — YUMUŞAK GEÇİŞ: Sig doluysa MUTLAKA geçerli olmalı
// (geçersizse kayıt reddedilir). Sig boşsa kayıt eski (imzasız) davranışla
// kabul edilir ama uyarı loglanır — mevcut kayıtlı node'lar (frontend'deki
// manuel kayıt formu, hiç Sig göndermiyor) ve henüz self-register akışına
// geçmemiş client'lar için geriye dönük uyumluluk. Bu, Sig'in zorunlu
// kılınacağı bir sonraki aşamaya kadar geçici bir durum.
func Register(req RegisterRequest) (*NodeRecord, error) {
	if req.NodeID == "" || req.PeerAddr == "" || req.Pubkey == "" {
		return nil, fmt.Errorf("node_id, peer_addr ve pubkey zorunlu")
	}

	if req.Sig != "" {
		if err := verifyRegistrationSignature(req); err != nil {
			return nil, fmt.Errorf("imza doğrulanamadı: %w", err)
		}
	} else {
		log.Printf("⚠️  imzasız node kaydı (deprecated — yakında zorunlu olacak): node_id=%s", req.NodeID)
	}

	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO federation_nodes (node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status, vrf_pubkey)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?)
		ON CONFLICT(node_id) DO UPDATE SET
			peer_addr = excluded.peer_addr,
			http_url  = excluded.http_url,
			version   = excluded.version,
			region    = excluded.region,
			last_seen = excluded.last_seen,
			status    = 'active',
			vrf_pubkey = COALESCE(NULLIF(excluded.vrf_pubkey, ''), federation_nodes.vrf_pubkey)
	`, req.NodeID, req.PeerAddr, req.HTTPURL, req.Pubkey, req.Version, req.Region, now, now, req.VRFPubkey)
	if err != nil {
		return nil, fmt.Errorf("node kaydı: %w", err)
	}

	return &NodeRecord{
		NodeID:      req.NodeID,
		PeerAddr:    req.PeerAddr,
		HTTPURL:     req.HTTPURL,
		Pubkey:      req.Pubkey,
		Version:     req.Version,
		Region:      req.Region,
		RegisteredAt: now,
		LastSeen:    now,
		Status:      "active",
		VRFPubkey:   req.VRFPubkey,
	}, nil
}

// List — aktif node'ları listele
func List() ([]NodeRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	rows, err := db.Query(`
		SELECT node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status, last_latency_ms, vrf_pubkey
		FROM federation_nodes
		WHERE status = 'active'
		ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeRecord
	for rows.Next() {
		var n NodeRecord
		if err := rows.Scan(&n.NodeID, &n.PeerAddr, &n.HTTPURL, &n.Pubkey,
			&n.Version, &n.Region, &n.RegisteredAt, &n.LastSeen, &n.Status, &n.LatencyMs, &n.VRFPubkey); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// Get — tek node
func Get(nodeID string) (*NodeRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	var n NodeRecord
	err := db.QueryRow(`
		SELECT node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status, last_latency_ms, vrf_pubkey
		FROM federation_nodes WHERE node_id = ?
	`, nodeID).Scan(&n.NodeID, &n.PeerAddr, &n.HTTPURL, &n.Pubkey,
		&n.Version, &n.Region, &n.RegisteredAt, &n.LastSeen, &n.Status, &n.LatencyMs, &n.VRFPubkey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

// UpdateLatency — bir node'un son ölçülen round-trip latency'sini (ms) kaydet.
func UpdateLatency(nodeID string, latencyMs int) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := db.Exec(`UPDATE federation_nodes SET last_latency_ms = ? WHERE node_id = ?`, latencyMs, nodeID)
	return err
}

// Heartbeat — node'un last_seen zamanını güncelle
func Heartbeat(nodeID string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := db.Exec(`UPDATE federation_nodes SET last_seen = ? WHERE node_id = ?`,
		time.Now().UTC(), nodeID)
	return err
}

// PruneInactive — 10 dakika görünmeyenleri inactive yap
func PruneInactive() {
	threshold := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := db.Exec(`UPDATE federation_nodes SET status = 'inactive' WHERE last_seen < ? AND status = 'active'`, threshold); err != nil {
		log.Printf("⚠️  Federation prune: %v", err)
	}
}

// StartPruner — arka planda her 5dk prune çalıştır
func StartPruner() {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		for range t.C {
			PruneInactive()
		}
	}()
}

// ProbeHealth — node'un HTTP endpoint'ini kontrol et, round-trip latency'yi ölçer.
// healthy=false durumunda latencyMs anlamsızdır (timeout/hata, ölçüm yapılmadı).
func ProbeHealth(n NodeRecord) (healthy bool, latencyMs int) {
	if n.HTTPURL == "" {
		return false, 0
	}
	client := &http.Client{Timeout: 3 * time.Second}
	start := time.Now()
	resp, err := client.Get(n.HTTPURL + "/v1/node/status")
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return false, 0
	}
	defer resp.Body.Close()
	var r struct{ Success bool `json:"success"` }
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return false, 0
	}
	return r.Success, elapsed
}

// StartHealthProbe — arka planda her 30sn aktif node'ları probe eder,
// ölçülen round-trip latency'yi federation_nodes.last_latency_ms'e yazar.
func StartHealthProbe(ctx context.Context) {
	go func() {
		for {
			nodes, err := List()
			if err == nil {
				for _, n := range nodes {
					go func(node NodeRecord) {
						if healthy, latencyMs := ProbeHealth(node); healthy {
							if err := UpdateLatency(node.NodeID, latencyMs); err != nil {
								log.Printf("⚠️  Latency güncellenemedi (%s): %v", node.NodeID, err)
							}
						}
					}(n)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
}
