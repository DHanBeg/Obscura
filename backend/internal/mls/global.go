package mls

import (
	"log"
	"sync"
)

var (
	globalClient *Client
	globalMu     sync.RWMutex
)

// SetGlobal registers the package-level MLS client started by main.
func SetGlobal(c *Client) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalClient = c
}

// Global returns the package-level MLS client, or nil if not started.
// Handlers must check for nil before use.
func Global() *Client {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalClient
}

// InitFromPath creates and registers a global client from a binary path.
// Returns nil without error if binPath is empty (MLS_CLI_PATH unset).
func InitFromPath(binPath string) *Client {
	if binPath == "" {
		return nil
	}
	c, err := NewClient(binPath)
	if err != nil {
		log.Printf("⚠️  MLS CLI başlatılamadı (%s): %v — MLS grup şifrelemesi devre dışı", binPath, err)
		return nil
	}
	SetGlobal(c)
	log.Printf("🔒 MLS CLI aktif: %s", binPath)
	return c
}
