package db

import "fmt"

// BackfillUserDisplayName, display_name (nickname) kolonu boş olan users
// satırları için deterministik bir varsayılan yazar. Kayıt akışı
// (HandleVerifyOTP) her zaman display_name=username atadığından bu satırın
// pratikte hiç tetiklenmemesi beklenir — ODI backfill'deki gibi bir
// güvenlik ağı. Idempotent — her boot'ta çağrılması güvenli.
//
// Öncelik: username varsa onu kullan (registration invariant'ıyla tutarlı),
// yoksa odi (zaten benzersiz+deterministik), o da yoksa sabit bir varsayılan.
func BackfillUserDisplayName() error {
	rows, err := DB.Query(`SELECT id, COALESCE(username,''), COALESCE(odi,'') FROM users WHERE display_name IS NULL OR display_name = ''`)
	if err != nil {
		return fmt.Errorf("nickname backfill select: %w", err)
	}
	type pending struct{ id, username, odi string }
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.username, &p.odi); err != nil {
			_ = rows.Close()
			return fmt.Errorf("nickname backfill scan: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("nickname backfill rows: %w", err)
	}
	_ = rows.Close()

	for _, p := range todo {
		name := p.username
		if name == "" {
			name = p.odi
		}
		if name == "" {
			name = "Kullanıcı"
		}
		if _, err := DB.Exec(`UPDATE users SET display_name = ? WHERE id = ?`, name, p.id); err != nil {
			return fmt.Errorf("nickname backfill update (id=%s): %w", p.id, err)
		}
	}
	return nil
}
