package db

import (
	"fmt"

	"obscura.network/core/internal/identity"
)

// BackfillUserODI, odi kolonu boş olan users satırları için DID'den
// deterministik ODI hesaplayıp yazar. Idempotent — her boot'ta çağrılması
// güvenli (WHERE odi IS NULL OR odi = '' filtresi zaten hesaplanmış
// satırlara dokunmaz).
func BackfillUserODI() error {
	rows, err := DB.Query(`SELECT id, did FROM users WHERE odi IS NULL OR odi = ''`)
	if err != nil {
		return fmt.Errorf("odi backfill select: %w", err)
	}
	type pending struct{ id, did string }
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.did); err != nil {
			_ = rows.Close()
			return fmt.Errorf("odi backfill scan: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("odi backfill rows: %w", err)
	}
	_ = rows.Close()

	for _, p := range todo {
		odi := identity.DeriveODI(p.did)
		if _, err := DB.Exec(`UPDATE users SET odi = ? WHERE id = ?`, odi, p.id); err != nil {
			return fmt.Errorf("odi backfill update (id=%s): %w", p.id, err)
		}
	}
	return nil
}
