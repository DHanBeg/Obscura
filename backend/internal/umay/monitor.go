package umay

// monitor.go — kamusal içerik akışını dinler (spec Bölüm 1.4, "monitor").
// Kapsam sınırı burada uygulanır: yalnızca conversations.is_public = 1.
// internal/scanner'a hiç dokunmaz, hiç import etmez — iki paket bağımsız
// çalışır, aynı messages tablosunu farklı WHERE koşullarıyla okur.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// scanInterval — scanner.go'nun en sık tarayıcısından (30s) daha seyrek:
// her taranan mesaj bir LLM çağrısına dönüşebilir, ucuz değil.
const scanInterval = 60 * time.Second

// Monitor polls public messages and routes each through brain (Classify)
// and notify (Handle).
type Monitor struct {
	db *sql.DB
}

func NewMonitor(db *sql.DB) *Monitor {
	return &Monitor{db: db}
}

// Start launches the scan loop in its own goroutine. ctx cancellation stops
// it cleanly — same shutdown contract as scanner.Scanner.Start.
func (m *Monitor) Start(ctx context.Context) {
	log.Println("[Umay] kamusal içerik tarama motoru aktif")
	go m.runLoop(ctx)
}

func (m *Monitor) runLoop(ctx context.Context) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[Umay] monitor durduruldu")
			return
		case <-ticker.C:
			if err := m.scan(ctx); err != nil {
				log.Printf("[Umay] tarama hatası: %v", err)
			}
		}
	}
}

type publicMessage struct {
	id      string
	fromDID string
	content string
}

// scan reads recently-sent public-conversation messages (Bölüm 1.1 boundary:
// is_public=1; sealed-sender excluded same as scanner.go — from_did is
// empty for sealed rows, a classified verdict would have nowhere to route
// for a category that needs a target), classifies each, and hands the
// result to notify.Handle.
func (m *Monitor) scan(ctx context.Context) error {
	cutoff := time.Now().Add(-scanInterval).UTC().Format(time.RFC3339)
	rows, err := m.db.QueryContext(ctx, `
		SELECT msg.id, msg.from_did, msg.ciphertext
		FROM messages msg
		JOIN conversations c ON c.id = msg.conv_id
		WHERE c.is_public = 1
		  AND msg.encryption_type != 'sealed'
		  AND msg.sent_at > ?
		  AND msg.deleted_at IS NULL
		LIMIT 500`, cutoff)
	if err != nil {
		return fmt.Errorf("umay monitor sorgu: %w", err)
	}

	var batch []publicMessage
	for rows.Next() {
		var pm publicMessage
		if err := rows.Scan(&pm.id, &pm.fromDID, &pm.content); err != nil {
			continue
		}
		batch = append(batch, pm)
	}
	rowsErr := rows.Err()
	rows.Close() // Exec (Handle içinde) rows kapatıldıktan sonra çağrılmalı — scanner.go'daki deadlock notuyla aynı gerekçe.

	for _, pm := range batch {
		verdict, classifyErr := Classify(ctx, pm.content)
		if err := Handle(ctx, m.db, pm.id, pm.fromDID, verdict, classifyErr); err != nil {
			log.Printf("[Umay] notify hatası msg=%s...: %v", truncate(pm.id, 8), err)
		}
	}
	return rowsErr
}
