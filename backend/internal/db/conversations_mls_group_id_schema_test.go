package db_test

// L2 Tuğla 5b-1 — conversations.mls_group_id kolonu (şema, hareket yok).
// Karar 1 (a): conv.id ↔ mls_group_id link'i conversations tablosunda nullable
// bir kolon olarak yaşıyor (mls_groups'a değil) — conv_type/description/
// is_public'in izlediği aynı yamanmış-kolon deseni (101/102/103), aynı
// escrow "önce şema, hareket yok" adımı (bkz. escrow_schema_test.go).

import (
	"database/sql"
	"testing"

	dbpkg "obscura.network/core/internal/db"
)

// TestConversationsMlsGroupIdSchema_MigrationsIdempotent — escrow_schema_test.go
// ile aynı desen: merkezi migration listesi (createTables + runMigrations,
// RunMigrationsForTest üzerinden — Init()'in çalıştırdığı AYNI kod) taze bir
// SQLite DB'ye iki kez uygulanır. İkinci koşu sessiz no-op olmalı.
func TestConversationsMlsGroupIdSchema_MigrationsIdempotent(t *testing.T) {
	sqlDB := openFreshSQLite(t)

	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("ilk migration koşusu başarısız: %v", err)
	}
	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("ikinci migration koşusu başarısız (idempotent değil): %v", err)
	}
}

// TestConversationsMlsGroupIdSchema_ColumnExistsAndNullable — kolon var ve
// NOT NULL DEĞİL (1:1 konuşmalarda hep NULL kalacak, eski akış bunu hiç
// bilmiyor — zorunlu olursa mevcut INSERT'ler patlardı).
func TestConversationsMlsGroupIdSchema_ColumnExistsAndNullable(t *testing.T) {
	sqlDB := openFreshSQLite(t)
	if err := dbpkg.RunMigrationsForTest(sqlDB, dbpkg.DriverSQLite); err != nil {
		t.Fatalf("migration koşusu başarısız: %v", err)
	}

	cols := tableColumns(t, sqlDB, "conversations")
	if !cols["mls_group_id"] {
		t.Fatal("conversations.mls_group_id kolonu yok")
	}

	rows, err := sqlDB.Query("PRAGMA table_info(conversations)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if name == "mls_group_id" {
			found = true
			if notNull != 0 {
				t.Errorf("mls_group_id NOT NULL olarak işaretli — nullable olmalı (1:1 konuşmalar hiç doldurmaz)")
			}
		}
	}
	if !found {
		t.Fatal("mls_group_id table_info'da bulunamadı")
	}

	// Boş konuşma tablosunda NULL bir mls_group_id ile INSERT çakılmamalı —
	// nullability iddiasının pratik kanıtı, sadece PRAGMA okuması değil.
	if _, err := sqlDB.Exec(`INSERT INTO conversations (id, is_group, name, created_at, updated_at)
		VALUES ('schema-check-conv', 0, 'x', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("mls_group_id belirtilmeden INSERT başarısız olmamalıydı: %v", err)
	}
	var got sql.NullString
	if err := sqlDB.QueryRow("SELECT mls_group_id FROM conversations WHERE id = 'schema-check-conv'").Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if got.Valid {
		t.Errorf("mls_group_id belirtilmeden INSERT sonrası beklenen NULL, alınan %q", got.String)
	}
}
