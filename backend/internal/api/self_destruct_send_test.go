package api_test

// ADIM 2 — Gönderme yolu testleri (Madde 5: self-destruct).
//
// loginAndRegister() (integration_test.go) gerçek OTP akışından geçiyor ve
// bu akış bu depoda önceden bozuk (dev_otp alanı handler'da artık yok —
// self-destruct çalışmasından bağımsız, pre-existing). Bu testler o akışa
// bağımlı kalmamak için kullanıcıyı doğrudan DB'ye yazıp auth.GenerateToken
// ile gerçek bir JWT üretir — router/middleware/handler zinciri hâlâ gerçek,
// sadece OTP adımı atlanıyor.
import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/identity"
	"obscura.network/core/internal/models"
)

// registerUserDirect — kullanıcıyı doğrudan DB'ye yazar ve geçerli bir JWT döner.
func registerUserDirect(t *testing.T, phone, username string) (did, token string) {
	t.Helper()
	id := uuid.New().String()
	did = "did:obscura:" + uuid.New().String()
	now := time.Now().Format(time.RFC3339)

	// identity_key NULL bırakılırsa middleware'in Scan(&user.IdentityKey) (plain
	// string, sql.NullString değil) hatası sessizce yutuluyor (err != nil ama
	// sql.ErrNoRows değil) — zero-value user (is_active=false) → 403 "askıya
	// alınmış" gibi görünüyor. Boş string ile bu tuzağı atla.
	// odi burada da hesaplanır — gerçek register akışıyla (HandleVerifyOTP)
	// aynı davranış, by-odi endpoint testleri bu satıra bağımlı.
	_, err := db.DB.Exec(`
		INSERT INTO users (id, phone, username, display_name, did, identity_key, odi, created_at, updated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, phone, username, username, did, "", identity.DeriveODI(did), now, now, now,
	)
	if err != nil {
		t.Fatalf("kullanıcı insert hatası: %v", err)
	}

	tok, err := auth.GenerateToken(&models.User{ID: id, DID: did, Tier: 1})
	if err != nil {
		t.Fatalf("token üretilemedi: %v", err)
	}
	return did, tok
}

func queryMessageSelfDestruct(t *testing.T, msgID string) (seconds *int, at *string) {
	t.Helper()
	err := db.DB.QueryRow(`SELECT self_destruct_seconds, self_destruct_at FROM messages WHERE id = ?`, msgID).
		Scan(&seconds, &at)
	if err != nil {
		t.Fatalf("mesaj sorgu hatası: %v", err)
	}
	return
}

// TestSendMessageWithFixedSelfDestruct — self_destruct_seconds=60 gönderilince
// backend self_destruct_at'i gönderimden ~60sn sonrasına hesaplamalı.
func TestSendMessageWithFixedSelfDestruct(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990001", "sd_sender_001")
	receiverDID, _ := registerUserDirect(t, "+905559990002", "sd_receiver_001")

	before := time.Now()
	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "self_destruct_payload_60",
		"type":                  "text",
		"self_destruct_seconds": 60,
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)
	if sendData.ID == "" {
		t.Fatal("Mesaj ID boş")
	}

	seconds, at := queryMessageSelfDestruct(t, sendData.ID)
	if seconds == nil || *seconds != 60 {
		t.Fatalf("self_destruct_seconds = %v, beklenen 60", seconds)
	}
	if at == nil {
		t.Fatal("self_destruct_at NULL — sabit süreli modda gönderimde hesaplanmalıydı")
	}
	parsedAt, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		t.Fatalf("self_destruct_at parse edilemedi: %v", err)
	}
	expected := before.Add(60 * time.Second)
	diff := parsedAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("self_destruct_at = %v, beklenen ~%v (±5sn)", parsedAt, expected)
	}
}

// TestSendMessageSelfDestructOnRead — self_destruct_seconds=0 ("okununca")
// gönderimde self_destruct_at'i NULL bırakmalı; okuma anında set edilecek
// (o kısım ADIM 3/4 kapsamında, burada sadece gönderim davranışı doğrulanır).
func TestSendMessageSelfDestructOnRead(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990003", "sd_sender_002")
	receiverDID, _ := registerUserDirect(t, "+905559990004", "sd_receiver_002")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "self_destruct_payload_onread",
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

	seconds, at := queryMessageSelfDestruct(t, sendData.ID)
	if seconds == nil || *seconds != 0 {
		t.Fatalf("self_destruct_seconds = %v, beklenen 0 (okununca)", seconds)
	}
	if at != nil {
		t.Errorf("self_destruct_at = %v, beklenen NULL (henüz okunmadı)", *at)
	}
}

// TestSendMessageWithoutSelfDestruct — alan hiç gönderilmezse iki kolon da
// NULL kalmalı (geriye dönük uyumluluk, yanlışlıkla self-destruct olmamalı).
func TestSendMessageWithoutSelfDestruct(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990005", "sd_sender_003")
	receiverDID, _ := registerUserDirect(t, "+905559990006", "sd_receiver_003")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "normal_payload",
		"type":       "text",
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID string `json:"id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	seconds, at := queryMessageSelfDestruct(t, sendData.ID)
	if seconds != nil {
		t.Errorf("self_destruct_seconds = %v, beklenen NULL", *seconds)
	}
	if at != nil {
		t.Errorf("self_destruct_at = %v, beklenen NULL", *at)
	}
}

// TestSendMessageInvalidSelfDestructSeconds — beyaz listede olmayan değer (ör.
// 42) 400 dönmeli; sistem sınırında doğrulama yapılmalı.
func TestSendMessageInvalidSelfDestructSeconds(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990007", "sd_sender_004")
	receiverDID, _ := registerUserDirect(t, "+905559990008", "sd_receiver_004")

	_, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "invalid_payload",
		"type":                  "text",
		"self_destruct_seconds": 42,
	}, senderToken)
	if code != 400 {
		t.Errorf("Beklenen HTTP 400 (geçersiz self_destruct_seconds), alınan %d", code)
	}
}

// TestGetMessagesIncludesSelfDestruct — sohbet geçmişi yeniden çekildiğinde
// (uygulama yeniden açılışı) self_destruct_seconds/at alanları JSON'da
// dönmeli, yoksa ikon reopen sonrası yanlış (gri/yeşil) görünür.
func TestGetMessagesIncludesSelfDestruct(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905559990009", "sd_sender_005")
	receiverDID, _ := registerUserDirect(t, "+905559990010", "sd_receiver_005")

	sendResp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":                 receiverDID,
		"ciphertext":            "history_reload_payload",
		"type":                  "text",
		"self_destruct_seconds": 300,
	}, senderToken)
	if code != 201 || !sendResp.Success {
		t.Fatalf("Mesaj gönderilemedi (code=%d): %s", code, sendResp.Error)
	}
	var sendData struct {
		ID     string `json:"id"`
		ConvID string `json:"conv_id"`
	}
	json.Unmarshal(sendResp.Data, &sendData)

	histResp, histCode := get(t, "/v1/conversations/"+sendData.ConvID+"/messages", senderToken)
	if histCode != 200 || !histResp.Success {
		t.Fatalf("Geçmiş alınamadı (code=%d): %s", histCode, histResp.Error)
	}
	var msgs []struct {
		ID                  string  `json:"id"`
		SelfDestructSeconds *int    `json:"self_destruct_seconds"`
		SelfDestructAt      *string `json:"self_destruct_at"`
	}
	json.Unmarshal(histResp.Data, &msgs)

	var found *struct {
		ID                  string  `json:"id"`
		SelfDestructSeconds *int    `json:"self_destruct_seconds"`
		SelfDestructAt      *string `json:"self_destruct_at"`
	}
	for i := range msgs {
		if msgs[i].ID == sendData.ID {
			found = &msgs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Gönderilen mesaj geçmişte bulunamadı")
	}
	if found.SelfDestructSeconds == nil || *found.SelfDestructSeconds != 300 {
		t.Errorf("self_destruct_seconds = %v, beklenen 300", found.SelfDestructSeconds)
	}
	if found.SelfDestructAt == nil {
		t.Error("self_destruct_at nil, beklenen dolu (sabit süreli mod)")
	}
}
