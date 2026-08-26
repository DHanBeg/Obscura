package api_test

// AuthMiddleware — migrate.go:105 (subscriber.MigratePhoneToSubscriberStore)
// tarafından phone=NULL yapılan, henüz migrate edilmemiş kullanıcıların
// middleware.go:34 eski COALESCE'siz SELECT yüzünden yanlışlıkla "hesabınız
// askıya alınmıştır" (403) almasının regresyon testi. İki yönlü kanıt — auth
// kodu olduğu için tek yön yeterli değil:
//   a) phone NULL + is_active=1 → artık 401/403 DEĞİL, zincir geçiyor (200).
//   b) phone dolu + is_active=0 (gerçek suspension) → HÂLÂ 403 (fix suspension'ı delmiyor).
//
// internal/api, internal/subscriber'ı import EDEMEZ (layer_boundary_test.go,
// "message plane must stay isolated from identity store") — bu yüzden gerçek
// subscriber.MigratePhoneToSubscriberStore çağrılmıyor. Onun yerine aynı iki
// adım burada bağımsızca tekrarlanıyor: (1) subscriber/migrate.go'daki
// ensurePhoneNullable ile birebir aynı, şemadan-bağımsız NOT NULL gevşetme
// (sqlite_master'dan canlı CREATE TABLE okunup regex'le NOT NULL düşürülüyor
// — kolon listesi elle kopyalanmıyor), (2) migrate.go:104-105'teki UPDATE ile
// birebir aynı `UPDATE users SET phone = NULL, phone_migrated = 1`.

import (
	"database/sql"
	"net/http/httptest"
	"regexp"
	"testing"

	"obscura.network/core/internal/api"
	"obscura.network/core/internal/db"
)

var (
	phoneNotNullRe     = regexp.MustCompile(`(?i)(phone\s+TEXT[^,\n]*?)\s+NOT NULL`)
	createUsersTableRe = regexp.MustCompile(`(?i)CREATE TABLE (IF NOT EXISTS )?["']?users["']?`)
)

// relaxPhoneNotNullForTest, subscriber/migrate.go:118 ensurePhoneNullable
// ile birebir aynı SQLite table-rebuild'i internal/api içinde tekrarlar
// (import sınırı yüzünden — yukarıdaki not). Zaten nullable ise no-op.
func relaxPhoneNotNullForTest(t *testing.T) {
	t.Helper()

	rows, err := db.DB.Query(`PRAGMA table_info(users)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	notNull := false
	for rows.Next() {
		var cid, nn, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &nn, &dflt, &pk); err != nil {
			rows.Close()
			t.Fatalf("table_info scan: %v", err)
		}
		if name == "phone" && nn == 1 {
			notNull = true
		}
	}
	rows.Close()
	if !notNull {
		return
	}

	var createSQL string
	if err := db.DB.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`,
	).Scan(&createSQL); err != nil {
		t.Fatalf("read users schema: %v", err)
	}
	if !phoneNotNullRe.MatchString(createSQL) {
		t.Fatalf("users.phone NOT NULL ama beklenen desenle eşleşmedi, rebuild SQL tahmin edilemiyor")
	}

	newSQL := createUsersTableRe.ReplaceAllString(createSQL, "CREATE TABLE users_new")
	newSQL = phoneNotNullRe.ReplaceAllString(newSQL, "$1")

	tx, err := db.DB.Begin()
	if err != nil {
		t.Fatalf("rebuild begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range []string{
		newSQL,
		`INSERT INTO users_new SELECT * FROM users`,
		`DROP TABLE users`,
		`ALTER TABLE users_new RENAME TO users`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			t.Fatalf("rebuild step failed (%.40q): %v", stmt, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("rebuild commit: %v", err)
	}
}

func TestAuthMiddleware_NullPhone_NotFalselySuspended(t *testing.T) {
	token := loginAndRegister(t, "+905559997101", "auth_mw_nullphone")

	relaxPhoneNotNullForTest(t)
	if _, err := db.DB.Exec(
		`UPDATE users SET phone = NULL, phone_migrated = 1 WHERE username = ?`, "auth_mw_nullphone",
	); err != nil {
		t.Fatalf("phone NULL'a çevrilemedi: %v", err)
	}

	var phone sql.NullString
	if err := db.DB.QueryRow(`SELECT phone FROM users WHERE username = ?`, "auth_mw_nullphone").Scan(&phone); err != nil {
		t.Fatalf("post-migration okunamadı: %v", err)
	}
	if phone.Valid {
		t.Fatalf("migration sonrası phone NULL bekleniyordu, alınan=%q", phone.String)
	}

	handler := api.AuthMiddleware(okHandler())
	req := httptest.NewRequest("GET", "/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("phone NULL + is_active=1 iken 200 bekleniyordu, alınan=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthMiddleware_RealSuspension_StillBlocked(t *testing.T) {
	token := loginAndRegister(t, "+905559997102", "auth_mw_realsuspend")

	if _, err := db.DB.Exec(`UPDATE users SET is_active = 0 WHERE username = ?`, "auth_mw_realsuspend"); err != nil {
		t.Fatalf("is_active=0 yapılamadı: %v", err)
	}

	handler := api.AuthMiddleware(okHandler())
	req := httptest.NewRequest("GET", "/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Errorf("gerçek suspension (is_active=0) iken 403 bekleniyordu, alınan=%d body=%s", rec.Code, rec.Body.String())
	}
}
