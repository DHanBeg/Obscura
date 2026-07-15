package api_test

// ADIM 3 — Okuma-tetikli self-destruct testleri (Madde 5).
//
// "okununca" modu (self_destruct_seconds=0): gönderimde self_destruct_at NULL
// bırakılır (ADIM 2), alıcı mesajı HandleMarkMessageRead ile okundu
// işaretleyince self_destruct_at set edilmeli — o andan sonra expiry.go
// scheduler'ı (ADIM 3) ciphertext'i gerçekten siler.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestMarkMessageReadSetsSelfDestructAtForOnReadMode — okununca modundaki
// mesaj okunduğunda self_destruct_at NULL'dan dolu hale gelmeli (~okuma anı).
func TestMarkMessageReadSetsSelfDestructAtForOnReadMode(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990011", "sd_read_sender_001")
	receiverDID, receiverToken := registerUserDirect(t, "+905559990012", "sd_read_receiver_001")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "onread_payload",
		"type":                  "text",
		"self_destruct_seconds": 0,
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	// Gönderimde self_destruct_at NULL olmalı (ADIM 2 davranışı, regresyon guard'ı)
	if _, at := queryMessageSelfDestruct(t, sendData.ID); at != nil {
		t.Fatalf("self_destruct_at gönderimde zaten dolu: %v (okununca modunda NULL beklenirdi)", *at)
	}

	before := time.Now()
	readResp, readCode := post(t, fmt.Sprintf("/v1/messages/%s/read", sendData.ID), nil, receiverToken)
	if readCode != 200 || !readResp.Success {
		t.Fatalf("Okundu işaretlenemedi (code=%d): %s", readCode, readResp.Error)
	}

	seconds, at := queryMessageSelfDestruct(t, sendData.ID)
	if seconds == nil || *seconds != 0 {
		t.Fatalf("self_destruct_seconds = %v, beklenen 0", seconds)
	}
	if at == nil {
		t.Fatal("self_destruct_at okuma sonrası hâlâ NULL — okununca modu tetiklenmedi")
	}
	parsedAt, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		t.Fatalf("self_destruct_at parse edilemedi: %v", err)
	}
	diff := parsedAt.Sub(before)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("self_destruct_at = %v, beklenen ~okuma anı (%v) ±5sn", parsedAt, before)
	}
}

// TestMarkMessageReadDoesNotOverwriteFixedSelfDestructAt — sabit süreli
// (>0) mesajda self_destruct_at zaten gönderimde hesaplanmıştı; okuma bunu
// EZMEMELİ (aksi halde "5 dakika sonra sil" seçimi okuma anına sıfırlanır).
func TestMarkMessageReadDoesNotOverwriteFixedSelfDestructAt(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990013", "sd_read_sender_002")
	receiverDID, receiverToken := registerUserDirect(t, "+905559990014", "sd_read_receiver_002")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "fixed_payload",
		"type":                  "text",
		"self_destruct_seconds": 3600,
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	_, atBeforeRead := queryMessageSelfDestruct(t, sendData.ID)
	if atBeforeRead == nil {
		t.Fatal("self_destruct_at gönderimde set edilmemiş (sabit süreli mod)")
	}

	readResp, readCode := post(t, fmt.Sprintf("/v1/messages/%s/read", sendData.ID), nil, receiverToken)
	if readCode != 200 || !readResp.Success {
		t.Fatalf("Okundu işaretlenemedi (code=%d): %s", readCode, readResp.Error)
	}

	_, atAfterRead := queryMessageSelfDestruct(t, sendData.ID)
	if atAfterRead == nil || *atAfterRead != *atBeforeRead {
		t.Errorf("self_destruct_at okuma ile değişti: önce=%v sonra=%v — sabit süreli modda EZİLMEMELİYDİ",
			*atBeforeRead, atAfterRead)
	}
}

// TestMarkMessageReadDoesNotSetSelfDestructAtForNormalMessage — self-destruct
// hiç seçilmemiş normal mesajda okuma self_destruct_at'i set ETMEMELİ.
func TestMarkMessageReadDoesNotSetSelfDestructAtForNormalMessage(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990015", "sd_read_sender_003")
	receiverDID, receiverToken := registerUserDirect(t, "+905559990016", "sd_read_receiver_003")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "plain_payload",
		"type":       "text",
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	readResp, readCode := post(t, fmt.Sprintf("/v1/messages/%s/read", sendData.ID), nil, receiverToken)
	if readCode != 200 || !readResp.Success {
		t.Fatalf("Okundu işaretlenemedi (code=%d): %s", readCode, readResp.Error)
	}

	seconds, at := queryMessageSelfDestruct(t, sendData.ID)
	if seconds != nil {
		t.Errorf("self_destruct_seconds = %v, beklenen NULL (normal mesaj)", *seconds)
	}
	if at != nil {
		t.Errorf("self_destruct_at = %v, beklenen NULL (normal mesaj, self-destruct seçilmedi)", *at)
	}
}
