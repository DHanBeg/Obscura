package api_test

// message_delivery_status testleri — grup mesajında per-recipient teslimat
// kaydı (messages.status/delivered_at tek-sütun sınırlamasına ek).

import (
	"encoding/json"
	"testing"
	"time"

	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
)

type deliveryStatusRow struct {
	RecipientDID string
	DeliveredAt  string
}

func queryDeliveryStatusRows(t *testing.T, msgID string) []deliveryStatusRow {
	t.Helper()
	rows, err := db.DB.Query(
		"SELECT recipient_did, delivered_at FROM message_delivery_status WHERE message_id = ? ORDER BY recipient_did",
		msgID,
	)
	if err != nil {
		t.Fatalf("message_delivery_status sorgusu: %v", err)
	}
	defer rows.Close()
	var out []deliveryStatusRow
	for rows.Next() {
		var r deliveryStatusRow
		if err := rows.Scan(&r.RecipientDID, &r.DeliveredAt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// TestGroupMessage_DeliveryStatus_OneRowPerOnlineMember — 3 üyeli grup
// (kurucu + 2 üye, ikisi de online), kurucu mesaj gönderiyor. Her üye kendi
// (message_id, recipient_did) satırını almalı — biri diğerini ezmemeli.
func TestGroupMessage_DeliveryStatus_OneRowPerOnlineMember(t *testing.T) {
	creatorPhone := "+905559996001"
	creatorToken := loginAndRegister(t, creatorPhone, "mds_creator")
	setUserCreditScore(t, creatorPhone, 65, 2) // Gümüş — grup açabilir

	memberAToken := loginAndRegister(t, "+905559996002", "mds_member_a")
	memberADID := currentUserDID(t, memberAToken)
	memberBToken := loginAndRegister(t, "+905559996003", "mds_member_b")
	memberBDID := currentUserDID(t, memberBToken)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Delivery Status Grubu",
		"members": []string{memberADID, memberBDID},
	}, creatorToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)

	// Her ikisini de online yap.
	registerFakeWSClient(t, memberADID)
	registerFakeWSClient(t, memberBDID)

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      convData.ConvID,
		"ciphertext": "delivery_status_payload",
		"type":       "text",
		"is_group":   true,
	}, creatorToken)
	if sendCode != 201 || !sendResp.Success {
		t.Fatalf("grup mesajı gönderilemedi: %d %s", sendCode, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	if sendData.ID == "" {
		t.Fatal("mesaj id boş")
	}

	rows := queryDeliveryStatusRows(t, sendData.ID)
	if len(rows) != 2 {
		t.Fatalf("2 ayrı teslimat satırı bekleniyordu (üye başına bir), alınan=%d (%+v)", len(rows), rows)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.DeliveredAt == "" {
			t.Errorf("delivered_at boş: %+v", r)
		}
		seen[r.RecipientDID] = true
	}
	if !seen[memberADID] || !seen[memberBDID] {
		t.Fatalf("beklenen iki alıcı da satırlarda yok: %+v (beklenen %s, %s)", rows, memberADID, memberBDID)
	}

	// messages.status/delivered_at (eski tek-sütun) hâlâ 'delivered' —
	// geriye uyumluluk, dokunulmadı.
	var oldStatus, oldDeliveredAt string
	db.DB.QueryRow("SELECT status, COALESCE(delivered_at,'') FROM messages WHERE id = ?", sendData.ID).
		Scan(&oldStatus, &oldDeliveredAt)
	if oldStatus != "delivered" || oldDeliveredAt == "" {
		t.Errorf("eski messages.status/delivered_at bozulmuş: status=%q delivered_at=%q", oldStatus, oldDeliveredAt)
	}
}

// TestDirectMessage_DeliveryStatus_StillWritesOneRow — 1-1 mesajlaşma
// davranışı grup fix'inden sonra DEĞİŞMEMELİ: tam olarak 1 satır, doğru
// alıcı, eski messages.status/delivered_at de aynı şekilde set olmalı.
func TestDirectMessage_DeliveryStatus_StillWritesOneRow(t *testing.T) {
	senderToken := loginAndRegister(t, "+905559996004", "mds_dm_sender")
	receiverToken := loginAndRegister(t, "+905559996005", "mds_dm_receiver")
	receiverDID := currentUserDID(t, receiverToken)

	registerFakeWSClient(t, receiverDID)

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "dm_delivery_status_payload",
		"type":       "text",
	}, senderToken)
	if sendCode != 201 || !sendResp.Success {
		t.Fatalf("1-1 mesaj gönderilemedi: %d %s", sendCode, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	rows := queryDeliveryStatusRows(t, sendData.ID)
	if len(rows) != 1 {
		t.Fatalf("1-1 mesajda tam 1 teslimat satırı bekleniyordu, alınan=%d (%+v)", len(rows), rows)
	}
	if rows[0].RecipientDID != receiverDID {
		t.Errorf("yanlış alıcı: %q, beklenen %q", rows[0].RecipientDID, receiverDID)
	}

	var oldStatus, oldDeliveredAt string
	db.DB.QueryRow("SELECT status, COALESCE(delivered_at,'') FROM messages WHERE id = ?", sendData.ID).
		Scan(&oldStatus, &oldDeliveredAt)
	if oldStatus != "delivered" || oldDeliveredAt == "" {
		t.Errorf("eski messages.status/delivered_at (1-1, regresyon kontrolü) bozulmuş: status=%q delivered_at=%q", oldStatus, oldDeliveredAt)
	}
}

// TestDirectMessage_SendToFails_NotMarkedDelivered — alıcı IsOnline()=true
// ama Send kanalı dolu (yavaş tüketici / backpressure): SendTo false döner.
// Bug regresyonu: eskiden SendTo'nun dönüşü kontrol edilmeden 'delivered'
// yazılıyordu — mesaj kuyruğa hiç girmemiş olsa bile gönderene "delivered"
// deniyordu. Fix sonrası: SendTo false ise messages.status 'sent' kalmalı,
// message_delivery_status'a hiç satır yazılmamalı.
func TestDirectMessage_SendToFails_NotMarkedDelivered(t *testing.T) {
	senderToken := loginAndRegister(t, "+905559996006", "mds_full_sender")
	receiverToken := loginAndRegister(t, "+905559996007", "mds_full_receiver")
	receiverDID := currentUserDID(t, receiverToken)

	// Kapasite 0, hiç okunmuyor — ilk SendTo çağrısı non-blocking select'te
	// default'a düşer (false döner), tıpkı dolu bir kanal gibi.
	client := &messaging.Client{DID: receiverDID, Send: make(chan []byte, 0)}
	messaging.GlobalHub.Register <- client
	time.Sleep(20 * time.Millisecond)
	t.Cleanup(func() {
		messaging.GlobalHub.Unregister <- client
	})

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "sendto_fail_payload",
		"type":       "text",
	}, senderToken)
	if sendCode != 201 || !sendResp.Success {
		t.Fatalf("mesaj gönderilemedi: %d %s", sendCode, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	var status, deliveredAt string
	db.DB.QueryRow("SELECT status, COALESCE(delivered_at,'') FROM messages WHERE id = ?", sendData.ID).
		Scan(&status, &deliveredAt)
	if status == "delivered" || deliveredAt != "" {
		t.Errorf("SendTo false döndüğü halde 'delivered' işaretlenmiş: status=%q delivered_at=%q", status, deliveredAt)
	}

	rows := queryDeliveryStatusRows(t, sendData.ID)
	if len(rows) != 0 {
		t.Errorf("SendTo false döndüğü halde message_delivery_status satırı yazılmış: %+v", rows)
	}
}
