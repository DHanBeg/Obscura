package api_test

// #23 — DID şema doğrulaması: binding_rotation.go ve cross_signing.go
// regex'leri eskiden 64-hex bekliyordu, kanonik DID formatı (auth.GenerateDID
// / mobile sealed-sender.ts:didFromDhPublic) ise "did:obs:" + 32 hex
// (SHA256(identity_key)[:16 byte]). Gerçek kayıt DID'i hiçbir zaman bu
// regex'lerden geçmiyordu — rotation-confirm ve pairing target_did_hint
// gerçek kullanıcılar için her zaman reddediliyordu. Fix: regex 32-hex'e
// hizalandı (prefix/anchor aynı kaldı, sadece {64}→{32}).

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestRotationRequest_AcceptsCanonicalDID — gerçek kayıt DID'i (auth.GenerateDID
// üretimi, 32 hex) rotation isteğinde new_did olarak kabul edilmeli.
func TestRotationRequest_AcceptsCanonicalDID(t *testing.T) {
	oldToken := loginAndRegister(t, "+905559995001", "did23_old1")
	newDID := currentUserDID(t, loginAndRegister(t, "+905559995002", "did23_new1"))

	resp, code := post(t, "/v1/identity/rotation/request", map[string]interface{}{
		"new_did": newDID,
	}, oldToken)
	if code != 200 || !resp.Success {
		t.Fatalf("gerçek kanonik DID rotation isteğinde reddedildi: %d %s", code, resp.Error)
	}
	var data struct {
		RotationID string `json:"rotation_id"`
		NewDID     string `json:"new_did"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.RotationID == "" {
		t.Error("rotation_id boş döndü")
	}
	if data.NewDID != newDID {
		t.Errorf("new_did beklenen %q, alınan %q", newDID, data.NewDID)
	}
}

// TestRotationRequest_RejectsNon32HexDID — sahte/yanlış-uzunluk DID hâlâ
// reddedilmeli (64-hex, eski yanlış varsayım dahil — bu değer kanona uymuyor).
func TestRotationRequest_RejectsNon32HexDID(t *testing.T) {
	token := loginAndRegister(t, "+905559995003", "did23_old2")

	fake64Hex := "did:obs:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"
	resp, code := post(t, "/v1/identity/rotation/request", map[string]interface{}{
		"new_did": fake64Hex,
	}, token)
	if code != 400 {
		t.Errorf("64-hex (kanon olmayan) DID: beklenen 400, alınan %d (%s)", code, resp.Error)
	}

	garbage := "did:obs:not-hex-at-all"
	resp, code = post(t, "/v1/identity/rotation/request", map[string]interface{}{
		"new_did": garbage,
	}, token)
	if code != 400 {
		t.Errorf("bozuk format: beklenen 400, alınan %d (%s)", code, resp.Error)
	}
}

// genEd25519PubB64 — HandlePairStart'ın zorunlu new_device_pubkey_b64 alanı için geçerli anahtar üretir.
func genEd25519PubB64(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 anahtar üretimi: %v", err)
	}
	return base64.StdEncoding.EncodeToString(pub)
}

// TestPairStart_AcceptsCanonicalTargetDIDHint — gerçek kayıt DID'i
// target_did_hint olarak kabul edilmeli (public endpoint, auth yok).
func TestPairStart_AcceptsCanonicalTargetDIDHint(t *testing.T) {
	hintDID := currentUserDID(t, loginAndRegister(t, "+905559995004", "did23_hint1"))

	resp, code := post(t, "/v1/devices/pair/start", map[string]interface{}{
		"new_device_pubkey_b64": genEd25519PubB64(t),
		"new_device_name":       "test-device",
		"target_did_hint":       hintDID,
	}, "")
	if code != 200 || !resp.Success {
		t.Fatalf("gerçek kanonik DID target_did_hint'te reddedildi: %d %s", code, resp.Error)
	}
}

// TestPairStart_RejectsNon32HexTargetDIDHint — 64-hex (eski yanlış varsayım)
// target_did_hint hâlâ reddedilmeli.
func TestPairStart_RejectsNon32HexTargetDIDHint(t *testing.T) {
	fake64Hex := "did:obs:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"

	resp, code := post(t, "/v1/devices/pair/start", map[string]interface{}{
		"new_device_pubkey_b64": genEd25519PubB64(t),
		"new_device_name":       "test-device",
		"target_did_hint":       fake64Hex,
	}, "")
	if code != 400 {
		t.Errorf("64-hex (kanon olmayan) target_did_hint: beklenen 400, alınan %d (%s)", code, resp.Error)
	}
}
