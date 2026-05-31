// Package federation — permissionless node registration & peer directory (FAZ 3, spec 12.3)
package federation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
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
}

// RegisterRequest — POST /v1/nodes/register body
type RegisterRequest struct {
	NodeID   string `json:"node_id"`
	PeerAddr string `json:"peer_addr"`
	HTTPURL  string `json:"http_url"`
	Pubkey   string `json:"pubkey"`
	Version  string `json:"version"`
	Region   string `json:"region"`
	Sig      string `json:"sig"` // Ed25519 signature of node_id+peer_addr (hex)
}

var (
	db   *sql.DB
	mu   sync.RWMutex
)

// Init — federation'u başlat, DB bağlantısını kaydet
func Init(database *sql.DB) error {
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
	return err
}

// Register — yeni node kaydet veya güncelle (permissionless)
func Register(req RegisterRequest) (*NodeRecord, error) {
	if req.NodeID == "" || req.PeerAddr == "" || req.Pubkey == "" {
		return nil, fmt.Errorf("node_id, peer_addr ve pubkey zorunlu")
	}

	mu.Lock()
	defer mu.Unlock()

	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO federation_nodes (node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')
		ON CONFLICT(node_id) DO UPDATE SET
			peer_addr = excluded.peer_addr,
			http_url  = excluded.http_url,
			version   = excluded.version,
			region    = excluded.region,
			last_seen = excluded.last_seen
	`, req.NodeID, req.PeerAddr, req.HTTPURL, req.Pubkey, req.Version, req.Region, now, now)
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
	}, nil
}

// List — aktif node'ları listele
func List() ([]NodeRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	rows, err := db.Query(`
		SELECT node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status
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
			&n.Version, &n.Region, &n.RegisteredAt, &n.LastSeen, &n.Status); err != nil {
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
		SELECT node_id, peer_addr, http_url, pubkey, version, region, registered_at, last_seen, status
		FROM federation_nodes WHERE node_id = ?
	`, nodeID).Scan(&n.NodeID, &n.PeerAddr, &n.HTTPURL, &n.Pubkey,
		&n.Version, &n.Region, &n.RegisteredAt, &n.LastSeen, &n.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
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

// ProbeHealth — node'un HTTP endpoint'ini kontrol et
func ProbeHealth(n NodeRecord) bool {
	if n.HTTPURL == "" {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(n.HTTPURL + "/v1/node/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var r struct{ Success bool `json:"success"` }
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return false
	}
	return r.Success
}
