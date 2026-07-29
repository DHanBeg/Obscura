package db

import (
	"strings"
	"testing"
)

// TestConvMembersUserDidIndexExists — conv_members(conv_id, user_did) composite
// PK'si tek başına user_did filtreleyen sorgular için kullanılamaz (user_did
// PK'nin İKİNCİ kolonu). GET /v1/conversations (en sık çağrılan endpoint) bu
// yüzden `cm.user_did = ?` filtresiyle tüm conv_members'ı taramak zorunda
// kalıyordu. Ayrı bir user_did index'i bunu düzeltir.
func TestConvMembersUserDidIndexExists(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`PRAGMA index_list(conv_members)`)
	if err != nil {
		t.Fatalf("index_list sorgu hatası: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var seq int
		var name, origin string
		var unique, partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		if name == "idx_conv_members_user_did" {
			found = true
		}
	}
	if !found {
		t.Fatal("idx_conv_members_user_did index'i yok")
	}
}

// TestConvMembersUserDidQueryUsesIndex — HandleGetConversations'ın gerçek
// sorgu şeklinin (JOIN + WHERE cm.user_did = ?) planlayıcı tarafından
// idx_conv_members_user_did ile çözüldüğünü doğrular — index var olması
// yetmez, SQLite'ın onu GERÇEKTEN seçtiğini de görmek gerekir.
func TestConvMembersUserDidQueryUsesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("db.Init hatası: %v", err)
	}
	defer DB.Close()

	rows, err := DB.Query(`
		EXPLAIN QUERY PLAN
		SELECT c.id FROM conversations c
		JOIN conv_members cm ON c.id = cm.conv_id AND cm.user_did = ?
		ORDER BY c.last_msg_at DESC
		LIMIT 50`, "did:obs:plan-check")
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN hatası: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan hatası: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}

	planStr := plan.String()
	if !strings.Contains(planStr, "idx_conv_members_user_did") {
		t.Fatalf("sorgu planı idx_conv_members_user_did kullanmıyor:\n%s", planStr)
	}
}
