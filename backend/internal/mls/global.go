package mls

import (
	"log"
	"sync"
)

var (
	globalClient  *Client
	globalMu      sync.RWMutex
	globalBinPath string
)

// SetGlobal registers the package-level MLS client started by main.
func SetGlobal(c *Client) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalClient = c
}

// Global returns the package-level MLS client.
// If the subprocess died (closed.Load() == true), it is restarted automatically (N5 fix).
// Returns nil if MLS_CLI_PATH was not set or restart fails.
func Global() *Client {
	globalMu.RLock()
	c := globalClient
	dead := c != nil && c.closed.Load()
	globalMu.RUnlock()

	if c == nil {
		return nil
	}
	if !dead {
		return c
	}

	// Subprocess exited — restart once under write lock.
	globalMu.Lock()
	defer globalMu.Unlock()
	// Double-check: another goroutine may have already restarted between RUnlock and Lock.
	if globalClient == nil || !globalClient.closed.Load() {
		return globalClient
	}
	if globalBinPath == "" {
		return nil
	}
	newC, err := NewClient(globalBinPath)
	if err != nil {
		log.Printf("⚠️  MLS CLI yeniden başlatılamadı: %v", err)
		return nil
	}
	log.Printf("🔄 MLS CLI yeniden başlatıldı: %s", globalBinPath)
	globalClient = newC
	return newC
}

// InitFromPath creates and registers a global client from a binary path.
// Returns nil without error if binPath is empty (MLS_CLI_PATH unset).
func InitFromPath(binPath string) *Client {
	if binPath == "" {
		return nil
	}
	globalMu.Lock()
	globalBinPath = binPath
	globalMu.Unlock()

	c, err := NewClient(binPath)
	if err != nil {
		log.Printf("⚠️  MLS CLI başlatılamadı (%s): %v — MLS grup şifrelemesi devre dışı", binPath, err)
		return nil
	}
	SetGlobal(c)
	log.Printf("🔒 MLS CLI aktif: %s", binPath)
	return c
}
