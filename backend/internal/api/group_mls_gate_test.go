package api_test

// Commit 0 — grup mesajları artık encryption_type:"mls" etiketi olmadan
// sunucuya kabul edilmiyor. Öncesi: sealedSenderRequiredForRequest (bkz.
// sealed_policy.go) grup mesajlarını her politika kontrolünden muaf
// tutuyordu — ciphertext alanına HERHANGİ bir değer (düz metin dahil)
// yazılıp is_group:true ile gönderilebiliyordu (bkz. bridge.go, mobil
// chat/[id].tsx grup transport planı). Bu test o boşluğu kapatan gate'i
// doğruluyor.
//
// NOT (dosya:satır kanıtlı, ADR-0019'a düşülecek): bu gate CONTENT
// DOĞRULAMASI değil — encryption_type client'ın JSON body'sinde kendi
// beyan ettiği bir alan (models.go:219, decodeBody ile doğrudan client
// body'sinden okunuyor, sunucu hesaplamıyor). Gate yalnızca "mls" etiketini
// zorunlu kılıyor, ciphertext'in gerçekten MLS wire-format'ında olduğunu
// doğrulamıyor — L2 gerçek MLS entegrasyonu gelene kadar bu, honesty
// contract'tır, kriptografik kanıt değil.
import (
	"testing"
)

func TestSendMessage_RejectsNonMLSGroup_NoEncryptionType(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905558887001", "mls_gate_sender_a")

	resp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      "some-group-conv-id",
		"ciphertext": "plaintext_group_payload",
		"type":       "text",
		"is_group":   true,
	}, senderToken)

	if code != 400 {
		t.Fatalf("encryption_type olmayan grup mesajı 400 dönmeliydi, alınan: %d %s", code, resp.Error)
	}
}

func TestSendMessage_RejectsNonMLSGroup_SignalEncryptionType(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905558887002", "mls_gate_sender_b")

	resp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":           "some-group-conv-id",
		"ciphertext":      "plaintext_group_payload",
		"type":            "text",
		"is_group":        true,
		"encryption_type": "signal",
	}, senderToken)

	if code != 400 {
		t.Fatalf("encryption_type:signal olan grup mesajı 400 dönmeliydi, alınan: %d %s", code, resp.Error)
	}
}

func TestSendMessage_AllowsDirectMessageWithoutMLSTag(t *testing.T) {
	_, senderToken := registerUserDirect(t, "+905558887003", "mls_gate_sender_c")
	receiverDID, _ := registerUserDirect(t, "+905558887004", "mls_gate_receiver_c")

	resp, code := post(t, "/v1/messages", map[string]interface{}{
		"to_id":      receiverDID,
		"ciphertext": "direct_payload",
		"type":       "text",
	}, senderToken)

	if code != 201 || !resp.Success {
		t.Fatalf("1:1 mesaj (grup DEĞİL) mls gate'inden etkilenmemeli, alınan: %d %s", code, resp.Error)
	}
}
