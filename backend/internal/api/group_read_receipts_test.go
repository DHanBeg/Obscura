package api_test

// #37 Bug B — per-reader grup okundu-bilgisi. Bug A yetkiyi düzeltti (grup
// üyeliği üzerinden), Bug B mesajın PAYLAŞILAN messages.status/read_at tek
// sütununu (grup içinde son yazan kazanır) message_read_status tablosuyla
// tamamlıyor — her üyenin kendi (message_id, reader_did) satırı olur.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"obscura.network/core/internal/db"
)

// TestGroupReadStatus_ThreeMembersEachGetOwnRow — 3 üye sırayla aynı grup
// mesajını okur → message_read_status'ta 3 AYRI satır (hiçbiri diğerini
// ezmedi), GetMessageStatus'un read_by alanı üçünü de listeliyor.
func TestGroupReadStatus_ThreeMembersEachGetOwnRow(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905561101001", "bugB_creator1")
	setUserCreditScore(t, "+905561101001", 65, 2)

	memberAToken := loginAndRegister(t, "+905561101002", "bugB_memberA1")
	memberADID := currentUserDID(t, memberAToken)
	memberBToken := loginAndRegister(t, "+905561101003", "bugB_memberB1")
	memberBDID := currentUserDID(t, memberBToken)
	memberCToken := loginAndRegister(t, "+905561101004", "bugB_memberC1")
	memberCDID := currentUserDID(t, memberCToken)

	convID := createTestGroup(t, creatorToken, []string{memberADID, memberBDID, memberCDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	for _, tok := range []string{memberAToken, memberBToken, memberCToken} {
		_, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, tok)
		if code != 200 {
			t.Fatalf("okundu işaretleme başarısız: %d", code)
		}
	}

	rows, err := db.DB.Query(
		"SELECT reader_did, read_at FROM message_read_status WHERE message_id = ? ORDER BY reader_did",
		msgID,
	)
	if err != nil {
		t.Fatalf("message_read_status sorgusu: %v", err)
	}
	defer rows.Close()

	seen := map[string]string{}
	for rows.Next() {
		var readerDID, readAt string
		if err := rows.Scan(&readerDID, &readAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[readerDID] = readAt
	}
	if len(seen) != 3 {
		t.Fatalf("beklenen 3 ayrı reader satırı, bulunan %d: %v", len(seen), seen)
	}
	for _, did := range []string{memberADID, memberBDID, memberCDID} {
		if seen[did] == "" {
			t.Errorf("üye %s için message_read_status satırı bulunamadı", did)
		}
	}

	// GetMessageStatus (gönderen sorguluyor) read_by'da üçünü de listelemeli.
	statusResp, code := get(t, fmt.Sprintf("/v1/messages/%s/status", msgID), creatorToken)
	if code != 200 || !statusResp.Success {
		t.Fatalf("GetMessageStatus başarısız: %d %s", code, statusResp.Error)
	}
	var statusData struct {
		ReadBy []struct {
			ReaderDID string `json:"reader_did"`
			ReadAt    string `json:"read_at"`
		} `json:"read_by"`
	}
	json.Unmarshal(statusResp.Data, &statusData)
	if len(statusData.ReadBy) != 3 {
		t.Fatalf("read_by: beklenen 3 okuyucu, alınan %d (%+v)", len(statusData.ReadBy), statusData.ReadBy)
	}
	gotDIDs := map[string]bool{}
	for _, rb := range statusData.ReadBy {
		gotDIDs[rb.ReaderDID] = true
		if rb.ReadAt == "" {
			t.Errorf("read_by satırında read_at boş: %+v", rb)
		}
	}
	for _, did := range []string{memberADID, memberBDID, memberCDID} {
		if !gotDIDs[did] {
			t.Errorf("read_by listesinde %s eksik", did)
		}
	}
}

// TestGroupReadStatus_SameMemberTwiceUpdatesNotDuplicates — aynı üye aynı
// grup mesajını iki kez okundu işaretlerse tek satır kalmalı, read_at
// güncellenmeli (çift satır açılmamalı).
func TestGroupReadStatus_SameMemberTwiceUpdatesNotDuplicates(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905561101005", "bugB_creator2")
	setUserCreditScore(t, "+905561101005", 65, 2)
	memberToken := loginAndRegister(t, "+905561101006", "bugB_member2")
	memberDID := currentUserDID(t, memberToken)

	convID := createTestGroup(t, creatorToken, []string{memberDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	_, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, memberToken)
	if code != 200 {
		t.Fatalf("ilk okundu işaretleme başarısız: %d", code)
	}
	var firstReadAt string
	if err := db.DB.QueryRow(
		"SELECT read_at FROM message_read_status WHERE message_id = ? AND reader_did = ?",
		msgID, memberDID,
	).Scan(&firstReadAt); err != nil {
		t.Fatalf("ilk read_at okunamadı: %v", err)
	}

	// read_at RFC3339 saniye çözünürlüğünde — güncellemenin farkını görmek için bekle.
	time.Sleep(1100 * time.Millisecond)

	_, code = post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, memberToken)
	if code != 200 {
		t.Fatalf("ikinci okundu işaretleme başarısız: %d", code)
	}

	var rowCount int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM message_read_status WHERE message_id = ? AND reader_did = ?",
		msgID, memberDID,
	).Scan(&rowCount); err != nil {
		t.Fatalf("satır sayımı: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("beklenen 1 satır (çift satır YOK), bulunan %d", rowCount)
	}

	var secondReadAt string
	if err := db.DB.QueryRow(
		"SELECT read_at FROM message_read_status WHERE message_id = ? AND reader_did = ?",
		msgID, memberDID,
	).Scan(&secondReadAt); err != nil {
		t.Fatalf("ikinci read_at okunamadı: %v", err)
	}
	if secondReadAt == firstReadAt {
		t.Errorf("read_at güncellenmedi: ilk=%q ikinci=%q", firstReadAt, secondReadAt)
	}
}

// TestGroupReadStatus_MembersDoNotOverwriteEachOther — B üyesi okuduktan
// sonra A üyesi okur; B'nin satırı A'nın okumasıyla EZİLMEMELİ (sıralı
// yazma senaryosu, race değil — açık sıra ile per-reader izolasyonu kanıtlar).
func TestGroupReadStatus_MembersDoNotOverwriteEachOther(t *testing.T) {
	creatorToken := loginAndRegister(t, "+905561101007", "bugB_creator3")
	setUserCreditScore(t, "+905561101007", 65, 2)
	memberAToken := loginAndRegister(t, "+905561101008", "bugB_memberA3")
	memberADID := currentUserDID(t, memberAToken)
	memberBToken := loginAndRegister(t, "+905561101009", "bugB_memberB3")
	memberBDID := currentUserDID(t, memberBToken)

	convID := createTestGroup(t, creatorToken, []string{memberADID, memberBDID})
	msgID := sendTestGroupMessage(t, creatorToken, convID)

	_, code := post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, memberBToken)
	if code != 200 {
		t.Fatalf("B okundu işaretleme başarısız: %d", code)
	}
	_, code = post(t, fmt.Sprintf("/v1/messages/%s/read", msgID), nil, memberAToken)
	if code != 200 {
		t.Fatalf("A okundu işaretleme başarısız: %d", code)
	}

	var bReadAt string
	if err := db.DB.QueryRow(
		"SELECT read_at FROM message_read_status WHERE message_id = ? AND reader_did = ?",
		msgID, memberBDID,
	).Scan(&bReadAt); err != nil {
		t.Fatalf("B'nin satırı bulunamadı — A'nın okuması B'yi EZMİŞ olabilir: %v", err)
	}
	if bReadAt == "" {
		t.Error("B'nin read_at değeri boş")
	}
}
