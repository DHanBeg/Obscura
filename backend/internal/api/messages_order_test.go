package api_test

// HandleGetMessages sıralama regresyonu.
//
// messages.sent_at RFC3339 (saniye çözünürlüğü) ile yazılıyor; aynı saniyedeki
// mesajlar ORDER BY sent_at ASC altında eşitlik bozucu olmadan tanımsız sırada
// dönüyordu (SQLite idx_msg_conv(conv_id, sent_at DESC) üzerinde geri tarayarak
// eşitleri TERS rowid sırasıyla verdi). İstemci decrypt'i sıralı ilerlediği için
// X3DH zarfı taşıyan ilk mesaj sonra gelirse ikinci mesaj "oturum kurulamadi"
// alıyordu. Ayrıca sent_at taranıp atıldığından yanıtta hep sıfır zaman
// (0001-01-01T00:00:00Z) dönüyordu.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"obscura.network/core/internal/db"
)

type orderedMsg struct {
	ID         string    `json:"id"`
	Ciphertext string    `json:"ciphertext"`
	SentAt     time.Time `json:"sent_at"`
}

// sendN, sender'dan receiver'a n mesaj yollar ve (conv_id, gönderim sırasıyla id'ler) döner.
// conv_id HTTP yanıtında yok; ilk mesajın satırından DB'den okunur.
func sendN(t *testing.T, senderToken, receiverDID string, n int, prefix string) (convID string, ids []string) {
	t.Helper()
	for i := 0; i < n; i++ {
		resp, code := post(t, "/v1/messages", map[string]interface{}{
			"to_id":      receiverDID,
			"ciphertext": fmt.Sprintf("%s-%d", prefix, i),
			"type":       "text",
		}, senderToken)
		if code != 201 || !resp.Success {
			t.Fatalf("mesaj %d gönderilemedi (code=%d): %s", i, code, resp.Error)
		}
		var d struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp.Data, &d); err != nil || d.ID == "" {
			t.Fatalf("mesaj %d yanıtı çözülemedi: %v (%s)", i, err, string(resp.Data))
		}
		ids = append(ids, d.ID)
	}
	if err := db.DB.QueryRow(`SELECT conv_id FROM messages WHERE id = ?`, ids[0]).Scan(&convID); err != nil {
		t.Fatalf("conv_id okunamadı: %v", err)
	}
	return convID, ids
}

func fetchMessages(t *testing.T, convID, token string) []orderedMsg {
	t.Helper()
	resp, code := get(t, "/v1/conversations/"+convID+"/messages", token)
	if code != 200 || !resp.Success {
		t.Fatalf("mesajlar alınamadı (code=%d): %s", code, resp.Error)
	}
	var msgs []orderedMsg
	if err := json.Unmarshal(resp.Data, &msgs); err != nil {
		t.Fatalf("mesaj listesi çözülemedi: %v", err)
	}
	return msgs
}

// TestGetMessagesSameSecondKeepsSendOrder — eşit sent_at'li mesajlar gönderim
// (ekleme) sırasıyla dönmeli. Eşitlik saniye-sınırına bırakılmaz: tüm satırların
// sent_at'i aynı değere sabitlenir, böylece test deterministik.
func TestGetMessagesSameSecondKeepsSendOrder(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559988101", "ord_sender_001")
	receiverDID, receiverToken := registerUserDirect(t, "+905559988102", "ord_receiver_001")

	const n = 8
	convID, ids := sendN(t, senderToken, receiverDID, n, "seq")

	tie := "2026-01-01T00:00:00Z"
	if _, err := db.DB.Exec(`UPDATE messages SET sent_at = ? WHERE conv_id = ?`, tie, convID); err != nil {
		t.Fatalf("sent_at sabitlenemedi: %v", err)
	}

	msgs := fetchMessages(t, convID, receiverToken)
	if len(msgs) != n {
		t.Fatalf("beklenen %d mesaj, gelen %d", n, len(msgs))
	}
	for i, m := range msgs {
		if m.ID != ids[i] {
			t.Fatalf("sıra bozuk: konum %d beklenen id=%s (%s-%d), gelen id=%s (%s)",
				i, ids[i], "seq", i, m.ID, m.Ciphertext)
		}
	}
}

// TestGetMessagesReturnsRealSentAt — yanıttaki sent_at DB'deki değerdir, sıfır zaman değil.
func TestGetMessagesReturnsRealSentAt(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559988103", "ord_sender_002")
	receiverDID, receiverToken := registerUserDirect(t, "+905559988104", "ord_receiver_002")

	before := time.Now().UTC().Add(-2 * time.Second)
	convID, _ := sendN(t, senderToken, receiverDID, 1, "ts")
	after := time.Now().UTC().Add(2 * time.Second)

	msgs := fetchMessages(t, convID, receiverToken)
	if len(msgs) != 1 {
		t.Fatalf("beklenen 1 mesaj, gelen %d", len(msgs))
	}
	got := msgs[0].SentAt
	if got.IsZero() || got.Year() < 2000 {
		t.Fatalf("sent_at sıfır/geçersiz döndü: %v", got)
	}
	if got.Before(before) || got.After(after) {
		t.Fatalf("sent_at gönderim anına yakın olmalı: got=%v, aralık=[%v, %v]", got, before, after)
	}
}
