package api_test

// Grup mesajı gerçek-zamanlı teslimat testleri.
//
// Teşhis (önceki tur): HandleSendMessage'da conv_id (grup mesajlarında
// req.ToID = conv_id, bkz. findOrCreateConversation) tek bir alıcı DID'iymiş
// gibi IsOnline/SendTo/PublishMessage/RelayToPeers/FCM'e geçiriliyordu —
// gruba hiç fanout yapılmıyordu. Fix: groupRecipientDIDs + deliverMessageToRecipient
// (handlers.go) her üyeye ayrı ayrı aynı teslimat mantığını uyguluyor.
//
// Bu testler messaging.GlobalHub'a GERÇEK websocket.Conn olmadan hafif bir
// fake Client kaydederek (Register kanalı + tamponlu Send kanalı — Hub'ın
// kendisi Conn'a dokunmadan IsOnline/SendTo çalıştırabiliyor) gerçek Hub
// kodu üzerinden "kaç alıcıya SendTo tetiklendi" sorusuna cevap veriyor.

import (
	"encoding/json"
	"testing"
	"time"

	"obscura.network/core/internal/messaging"
)

// registerFakeWSClient, GlobalHub'a gerçek soket olmadan hafif bir client
// kaydeder (Conn=nil — Hub'ın Register/IsOnline/SendTo yolu Conn'a dokunmuyor,
// sadece client.Send kanalına yazıyor). t.Cleanup ile unregister edilir.
func registerFakeWSClient(t *testing.T, did string) chan []byte {
	t.Helper()
	send := make(chan []byte, 20)
	client := &messaging.Client{DID: did, Send: send}
	messaging.GlobalHub.Register <- client
	// Register asenkron (channel + Run() goroutine) — IsOnline'ın client'ı
	// görmesini garanti etmek için kısa bir işlem sırası bekle.
	time.Sleep(20 * time.Millisecond)
	t.Cleanup(func() {
		messaging.GlobalHub.Unregister <- client
	})
	return send
}

// drainNewMessageEvents, kanaldaki "new_message" tipli WS event'lerini
// (varsa "connected" gibi diğer sistem mesajlarını atlayarak) toplar.
func drainNewMessageEvents(t *testing.T, ch chan []byte) []map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	for {
		select {
		case raw := <-ch:
			var wsMsg struct {
				Type    string                 `json:"type"`
				Payload map[string]interface{} `json:"payload"`
			}
			if err := json.Unmarshal(raw, &wsMsg); err != nil {
				continue
			}
			if wsMsg.Type == "new_message" {
				events = append(events, wsMsg.Payload)
			}
		case <-time.After(100 * time.Millisecond):
			return events
		}
	}
}

// TestGroupMessage_DeliversToAllMembersExceptSender — 3 üyeli grup (kurucu +
// 2 üye), kurucu mesaj gönderir, HER İKİ üye de "new_message" almalı, kurucu
// KENDİ mesajını almamalı (recipients listesi gönderen hariç).
func TestGroupMessage_DeliversToAllMembersExceptSender(t *testing.T) {
	creatorPhone := "+905559992001"
	creatorToken := loginAndRegister(t, creatorPhone, "gmd_creator")
	setUserCreditScore(t, creatorPhone, 65, 2) // Gümüş — grup açabilir
	creatorDID := currentUserDID(t, creatorToken)

	memberAToken := loginAndRegister(t, "+905559992002", "gmd_member_a")
	memberADID := currentUserDID(t, memberAToken)

	memberBToken := loginAndRegister(t, "+905559992003", "gmd_member_b")
	memberBDID := currentUserDID(t, memberBToken)

	resp, code := post(t, "/v1/conversations", map[string]interface{}{
		"type":    "group",
		"name":    "Delivery Test Grubu",
		"members": []string{memberADID, memberBDID},
	}, creatorToken)
	if code != 201 {
		t.Fatalf("grup oluşturulamadı: %d %s", code, resp.Error)
	}
	var convData struct {
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(resp.Data, &convData)
	convID := convData.ConvID

	// Üçünü de "online" yap (kurucu dahil — kendi mesajını almamalı).
	creatorCh := registerFakeWSClient(t, creatorDID)
	memberACh := registerFakeWSClient(t, memberADID)
	memberBCh := registerFakeWSClient(t, memberBDID)

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      convID,
		"ciphertext": "group_fanout_payload",
		"type":       "text",
		"is_group":   true,
	}, creatorToken)
	if sendCode != 201 || !sendResp.Success {
		t.Fatalf("grup mesajı gönderilemedi: %d %s", sendCode, sendResp.Error)
	}

	aEvents := drainNewMessageEvents(t, memberACh)
	bEvents := drainNewMessageEvents(t, memberBCh)
	creatorEvents := drainNewMessageEvents(t, creatorCh)

	if len(aEvents) != 1 {
		t.Errorf("member A: 1 new_message bekleniyordu, alınan=%d", len(aEvents))
	} else if aEvents[0]["ciphertext"] != "group_fanout_payload" {
		t.Errorf("member A: yanlış ciphertext: %v", aEvents[0]["ciphertext"])
	}
	if len(bEvents) != 1 {
		t.Errorf("member B: 1 new_message bekleniyordu, alınan=%d", len(bEvents))
	} else if bEvents[0]["ciphertext"] != "group_fanout_payload" {
		t.Errorf("member B: yanlış ciphertext: %v", bEvents[0]["ciphertext"])
	}
	if len(creatorEvents) != 0 {
		t.Errorf("kurucu kendi mesajını ALMAMALI, alınan=%d", len(creatorEvents))
	}
}

// TestDirectMessage_StillOnlyDeliversToRecipient — 1-1 mesajlaşma davranışı
// grup fix'inden sonra DEĞİŞMEMELİ: sadece alıcı new_message alır, gönderen
// almaz (regresyon kontrolü).
func TestDirectMessage_StillOnlyDeliversToRecipient(t *testing.T) {
	senderToken := loginAndRegister(t, "+905559992004", "gmd_dm_sender")
	senderDID := currentUserDID(t, senderToken)
	receiverToken := loginAndRegister(t, "+905559992005", "gmd_dm_receiver")
	receiverDID := currentUserDID(t, receiverToken)
	_ = receiverToken

	senderCh := registerFakeWSClient(t, senderDID)
	receiverCh := registerFakeWSClient(t, receiverDID)

	sendResp, sendCode := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "direct_payload",
		"type":       "text",
	}, senderToken)
	if sendCode != 201 || !sendResp.Success {
		t.Fatalf("1-1 mesaj gönderilemedi: %d %s", sendCode, sendResp.Error)
	}

	receiverEvents := drainNewMessageEvents(t, receiverCh)
	senderEvents := drainNewMessageEvents(t, senderCh)

	if len(receiverEvents) != 1 {
		t.Errorf("alıcı: 1 new_message bekleniyordu, alınan=%d", len(receiverEvents))
	} else if receiverEvents[0]["ciphertext"] != "direct_payload" {
		t.Errorf("alıcı: yanlış ciphertext: %v", receiverEvents[0]["ciphertext"])
	}
	if len(senderEvents) != 0 {
		t.Errorf("gönderen new_message ALMAMALI (delivery_ack alır, new_message değil), alınan=%d", len(senderEvents))
	}
}
