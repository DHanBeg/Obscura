package umay

// monitor.go — kamusal içerik akışını dinler (spec Bölüm 1.4, "monitor").
// Kapsam sınırı burada uygulanır: yalnızca conversations.is_public = 1.
// internal/scanner'a hiç dokunmaz, hiç import etmez — iki paket bağımsız
// çalışır, aynı messages tablosunu farklı WHERE koşullarıyla okur.

import (
	"context"
	"fmt"
	"log"
	"time"
	"obscura.network/core/internal/dbi"
)

// scanInterval — scanner.go'nun en sık tarayıcısından (30s) daha seyrek:
// her taranan mesaj bir LLM çağrısına dönüşebilir, ucuz değil.
const scanInterval = 60 * time.Second

// Monitor polls public messages and routes each through brain (Classify)
// and notify (Handle).
type Monitor struct {
	db dbi.Querier
}

func NewMonitor(db dbi.Querier) *Monitor {
	return &Monitor{db: db}
}

// Start launches the scan loop in its own goroutine. ctx cancellation stops
// it cleanly — same shutdown contract as scanner.Scanner.Start.
func (m *Monitor) Start(ctx context.Context) {
	log.Println("[Umay] kamusal içerik tarama motoru aktif")
	go m.runLoop(ctx)
}

// runLoop drives BOTH scanners off one ticker — listing volume is a small
// fraction of message volume, so a second goroutine/interval isn't worth the
// complexity (see plan discussion: option A over a separate listing ticker).
func (m *Monitor) runLoop(ctx context.Context) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[Umay] monitor durduruldu")
			return
		case <-ticker.C:
			if err := m.scanMessages(ctx); err != nil {
				log.Printf("[Umay] mesaj tarama hatası: %v", err)
			}
			if err := m.scanListings(ctx); err != nil {
				log.Printf("[Umay] ilan tarama hatası: %v", err)
			}
		}
	}
}

type publicMessage struct {
	id      string
	fromDID string
	content string
}

// scanMessages reads recently-sent public-conversation messages (Bölüm 1.1
// boundary: is_public=1; sealed-sender excluded same as scanner.go —
// from_did is empty for sealed rows, a classified verdict would have
// nowhere to route for a category that needs a target), classifies each,
// and hands the result to notify.Handle.
func (m *Monitor) scanMessages(ctx context.Context) error {
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
		return fmt.Errorf("umay monitor mesaj sorgu: %w", err)
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

type publicListing struct {
	id          string
	sellerDID   string
	title       string
	description string
}

// scanListings reads recently-touched active marketplace listings (Bölüm
// 1.1: marketplace ilanları her zaman kamusal — is_public filtresi yok,
// scope zaten status='active' ile çiziliyor), classifies title+description,
// and hands the result to notify.HandleListing.
func (m *Monitor) scanListings(ctx context.Context) error {
	cutoff := time.Now().Add(-scanInterval).UTC().Format(time.RFC3339)
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, seller_did, title, description
		FROM marketplace_listings
		WHERE status = 'active' AND updated_at > ?
		LIMIT 500`, cutoff)
	if err != nil {
		return fmt.Errorf("umay monitor ilan sorgu: %w", err)
	}

	var batch []publicListing
	for rows.Next() {
		var pl publicListing
		if err := rows.Scan(&pl.id, &pl.sellerDID, &pl.title, &pl.description); err != nil {
			continue
		}
		batch = append(batch, pl)
	}
	rowsErr := rows.Err()
	rows.Close() // scanMessages'daki aynı gerekçe: Exec, rows kapatıldıktan sonra.

	for _, pl := range batch {
		content := pl.title + "\n" + pl.description
		verdict, classifyErr := Classify(ctx, content)
		if err := HandleListing(ctx, m.db, pl.id, pl.sellerDID, verdict, classifyErr); err != nil {
			log.Printf("[Umay] notify hatası listing=%s...: %v", truncate(pl.id, 8), err)
		}
	}
	return rowsErr
}
