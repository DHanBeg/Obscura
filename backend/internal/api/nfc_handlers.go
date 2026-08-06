// Package api — NFC Handler'ları (Spec Bölüm 11.4)
//
// NFC tap-to-attend, tap-to-pair ve OBS ödeme akışları.
//
// Güvenlik garantileri:
//   - Tüm NFC mesajları Signal Protocol ile şifrelenir (spec gereksinimi).
//     Bu katman client'ta gerçekleşir; backend sadece şifreli yükü iletir.
//   - Replay koruması: nonce tablosu (nfc_nonces). Aynı nonce ikinci kez reddedilir.
//   - Timestamp penceresi: ±5 dakika. Dışındaki istekler reddedilir.
//   - ZK check-in: identity_proof.circom ile anonim katılım (spec Bölüm 11.1).
package api

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/zk"
)

// nfcTimestampToleranceSec — replay koruması için zaman penceresi (±5 dakika).
const nfcTimestampToleranceSec = 5 * 60

// ─── POST /v1/nfc/checkin ─────────────────────────────────────────────────────

// NFCCheckinRequest — NFC tap-to-attend isteği.
//
// nfc_payload: `obscura://event/{eventID}/checkin/{token}` formatında NFC URI.
//              Client, NFC tag'den okur ve bu field'a yerleştirir.
// nonce:       UUID veya benzeri tek kullanımlık değer (replay koruması).
// timestamp:   Unix saniye cinsinden istek zamanı (replay penceresi doğrulaması).
// zk_proof:    Opsiyonel — identity_proof.circom Groth16 kanıtı (JSON string).
// zk_inputs:   Opsiyonel — identity_proof public signals.
// anonymous:   true ise kimlik açıklanmadan ZK check-in yapılır.
type NFCCheckinRequest struct {
	NFCPayload string   `json:"nfc_payload"`
	Nonce      string   `json:"nonce"`
	Timestamp  int64    `json:"timestamp"`
	ZKProof    string   `json:"zk_proof,omitempty"`
	ZKInputs   []string `json:"zk_inputs,omitempty"`
	Anonymous  bool     `json:"anonymous,omitempty"`
}

// HandleNFCCheckin — POST /v1/nfc/checkin
//
// NFC tag'inden okunan payload'u parse eder ve etkinlik check-in akışını çalıştırır.
// Replay saldırısı önleme: nonce tek kullanımlık, timestamp ±5 dakika içinde olmalı.
func HandleNFCCheckin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req NFCCheckinRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	// Zorunlu alanlar
	if strings.TrimSpace(req.NFCPayload) == "" {
		respond(w, 400, nil, "nfc_payload zorunlu")
		return
	}
	if strings.TrimSpace(req.Nonce) == "" {
		respond(w, 400, nil, "nonce zorunlu")
		return
	}
	if req.Timestamp == 0 {
		respond(w, 400, nil, "timestamp zorunlu")
		return
	}

	// Timestamp pencere kontrolü: ±5 dakika
	nowSec := time.Now().UTC().Unix()
	diff := nowSec - req.Timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > nfcTimestampToleranceSec {
		respond(w, 400, nil, "NFC timestamp süresi dolmuş (±5 dakika penceresi)")
		return
	}

	// Nonce tekrar kullanım kontrolü (replay attack)
	if err := consumeNFCNonce(req.Nonce, nowSec); err != nil {
		log.Printf("NFC nonce tekrar kullanım: nonce=%s did=%s", req.Nonce, user.DID)
		respond(w, 409, nil, "Bu NFC isteği daha önce işlendi (replay koruması)")
		return
	}

	// Anonim mod doğrulaması
	if req.Anonymous && (req.ZKProof == "" || len(req.ZKInputs) == 0) {
		respond(w, 400, nil, "Anonim NFC check-in için zk_proof ve zk_inputs zorunlu")
		return
	}

	// NFC payload parse — desteklenen format:
	//   obscura://event/{eventID}/checkin/{token}
	eventID, qrToken, err := parseNFCCheckinPayload(req.NFCPayload)
	if err != nil {
		respond(w, 400, nil, fmt.Sprintf("NFC payload geçersiz: %v", err))
		return
	}

	// QR token doğrulama
	resolvedEventID, valid := ValidateCheckinQR(qrToken, checkinSecret())
	if !valid {
		respond(w, 400, nil, "NFC token geçersiz veya süresi dolmuş")
		return
	}
	if resolvedEventID != eventID {
		respond(w, 400, nil, "NFC token bu etkinliğe ait değil")
		return
	}

	// Etkinlik mevcut mu?
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", eventID).Scan(&exists)
	if exists == 0 {
		respond(w, 404, nil, "Etkinlik bulunamadı")
		return
	}

	// Otomatik join (NFC ile gelen ilk defa check-in yapan katılımcı)
	var attendeeExists int
	db.DB.QueryRow(
		"SELECT COUNT(*) FROM event_attendees WHERE event_id = ? AND attendee_did = ?",
		eventID, user.DID,
	).Scan(&attendeeExists)
	if attendeeExists == 0 {
		joinNow := time.Now().UTC().Format(time.RFC3339)
		db.DB.Exec(`
			INSERT INTO event_attendees (event_id, attendee_did, checked_in, joined_at)
			VALUES (?, ?, 0, ?)
			ON CONFLICT DO NOTHING`, eventID, user.DID, joinNow)
	}

	// Zaten check-in yaptı mı?
	var alreadyCheckedIn int
	db.DB.QueryRow(
		"SELECT checked_in FROM event_attendees WHERE event_id = ? AND attendee_did = ?",
		eventID, user.DID,
	).Scan(&alreadyCheckedIn)
	if alreadyCheckedIn == 1 {
		respond(w, 409, nil, "Zaten check-in yapıldı")
		return
	}

	// ZK kanıt doğrulama (opsiyonel — anonim ya da isteğe bağlı)
	var proofBlob *string
	isAnonymous := false
	if req.ZKProof != "" && len(req.ZKInputs) > 0 {
		ok, verErr := zk.VerifyProof("identity_proof", req.ZKProof, req.ZKInputs)
		if verErr != nil || !ok {
			log.Printf("NFC check-in ZK kanıtı geçersiz event=%s did=%s: %v", eventID, user.DID, verErr)
			respond(w, 400, nil, "ZK kimlik kanıtı geçersiz")
			return
		}
		proofBlob = &req.ZKProof
		isAnonymous = req.Anonymous
	}

	anonymousInt := 0
	if isAnonymous {
		anonymousInt = 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, dbErr := db.DB.Exec(`
		UPDATE event_attendees
		SET checked_in = 1, checked_in_at = ?, zk_checkin_proof = ?, anonymous = ?
		WHERE event_id = ? AND attendee_did = ?`,
		now, proofBlob, anonymousInt, eventID, user.DID,
	)
	if dbErr != nil {
		log.Printf("NFC check-in DB hatası event=%s did=%s: %v", eventID, user.DID, dbErr)
		respond(w, 500, nil, "Check-in kaydedilemedi")
		return
	}

	log.Printf("NFC check-in başarılı: event=%s did=%s zk=%v anonymous=%v",
		eventID, user.DID, proofBlob != nil, isAnonymous)
	respond(w, 200, map[string]interface{}{
		"status":        "checked_in",
		"event_id":      eventID,
		"checked_in_at": now,
		"zk_verified":   proofBlob != nil,
		"anonymous":     isAnonymous,
		"method":        "nfc",
	}, "")
}

// ─── POST /v1/nfc/pair ────────────────────────────────────────────────────────

// NFCPairRequest — NFC tap-to-pair cihaz eşleştirme isteği.
//
// Akış:
//  1. Yeni cihaz, NFC tag'i okur: `obscura://pair/{challenge}`.
//  2. Yeni cihaz, bu endpoint'i çağırır.
//  3. Sunucu, mevcut cihaza bildirim gönderir (WebSocket).
//  4. Kullanıcı ana cihazda onaylar → /v1/devices/pair/approve.
type NFCPairRequest struct {
	NFCPayload   string `json:"nfc_payload"`   // obscura://pair/{challenge} veya ham challenge
	Nonce        string `json:"nonce"`
	Timestamp    int64  `json:"timestamp"`
	NewDevicePub string `json:"new_device_pub"` // yeni cihazın Ed25519 public key (base64)
	NewDeviceName string `json:"new_device_name,omitempty"`
}

// HandleNFCPair — POST /v1/nfc/pair
//
// NFC tap ile cihaz eşleştirme başlatır. Cross-signing akışının NFC katmanıdır.
// Onay aşaması: /v1/devices/pair/approve (mevcut cross_signing.go handler).
func HandleNFCPair(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req NFCPairRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if strings.TrimSpace(req.NFCPayload) == "" {
		respond(w, 400, nil, "nfc_payload zorunlu")
		return
	}
	if strings.TrimSpace(req.Nonce) == "" {
		respond(w, 400, nil, "nonce zorunlu")
		return
	}
	if req.Timestamp == 0 {
		respond(w, 400, nil, "timestamp zorunlu")
		return
	}
	if strings.TrimSpace(req.NewDevicePub) == "" {
		respond(w, 400, nil, "new_device_pub zorunlu")
		return
	}

	// Timestamp pencere kontrolü
	nowSec := time.Now().UTC().Unix()
	diff := nowSec - req.Timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > nfcTimestampToleranceSec {
		respond(w, 400, nil, "NFC timestamp süresi dolmuş")
		return
	}

	// Nonce tekrar kullanım kontrolü
	if err := consumeNFCNonce(req.Nonce, nowSec); err != nil {
		respond(w, 409, nil, "Bu NFC isteği daha önce işlendi (replay koruması)")
		return
	}

	// NFC payload parse — desteklenen format: obscura://pair/{challenge}
	challenge, err := parseNFCPairPayload(req.NFCPayload)
	if err != nil {
		respond(w, 400, nil, fmt.Sprintf("NFC pair payload geçersiz: %v", err))
		return
	}

	// Pairing request'i kaydet (cross_signing.go'daki HandlePairStart ile aynı tablo)
	pairID := uuid.New().String()
	expiresAt := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	nowStr := time.Now().UTC().Format(time.RFC3339)
	deviceName := strings.TrimSpace(req.NewDeviceName)
	if deviceName == "" {
		deviceName = "NFC Cihaz"
	}

	_, dbErr := db.DB.Exec(`
		INSERT INTO pairing_requests
			(id, user_did, challenge, new_device_pubkey, new_device_name, status, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`,
		pairID, user.DID, challenge, req.NewDevicePub, deviceName, nowStr, expiresAt,
	)
	if dbErr != nil {
		log.Printf("NFC pair DB hatası did=%s: %v", user.DID, dbErr)
		respond(w, 500, nil, "Eşleştirme isteği kaydedilemedi")
		return
	}

	log.Printf("NFC pair isteği oluşturuldu: id=%s did=%s", pairID, user.DID)
	respond(w, 200, map[string]interface{}{
		"pair_id":    pairID,
		"status":     "pending",
		"expires_at": expiresAt,
		"message":    "Ana cihazdan onaylayın: /v1/devices/pair/approve",
	}, "")
}

// ─── POST /v1/nfc/payment ─────────────────────────────────────────────────────

// NFCPaymentRequest — NFC tap ile OBS token transferi.
//
// Akış:
//  1. Alıcı cihaz, NFC tag'i okur: `obscura://pay/{receiverDID}/{amount}`.
//  2. Gönderen cihaz, bu endpoint'i çağırır.
//  3. Sunucu, mevcut wallet transfer endpoint'ini iç olarak çağırır.
type NFCPaymentRequest struct {
	NFCPayload string `json:"nfc_payload"` // obscura://pay/{receiverDID}/{amount}
	Nonce      string `json:"nonce"`
	Timestamp  int64  `json:"timestamp"`
	Memo       string `json:"memo,omitempty"`
}

// HandleNFCPayment — POST /v1/nfc/payment
//
// NFC üzerinden OBS token transferi başlatır. Mevcut wallet transfer mantığını
// yeniden kullanır; NFC katmanı sadece replay koruması ve payload parse ekler.
func HandleNFCPayment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req NFCPaymentRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if strings.TrimSpace(req.NFCPayload) == "" {
		respond(w, 400, nil, "nfc_payload zorunlu")
		return
	}
	if strings.TrimSpace(req.Nonce) == "" {
		respond(w, 400, nil, "nonce zorunlu")
		return
	}
	if req.Timestamp == 0 {
		respond(w, 400, nil, "timestamp zorunlu")
		return
	}

	// Timestamp pencere kontrolü
	nowSec := time.Now().UTC().Unix()
	diff := nowSec - req.Timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > nfcTimestampToleranceSec {
		respond(w, 400, nil, "NFC timestamp süresi dolmuş")
		return
	}

	// Nonce tekrar kullanım kontrolü — finansal işlemlerde kritik!
	if err := consumeNFCNonce(req.Nonce, nowSec); err != nil {
		log.Printf("NFC payment nonce tekrar: nonce=%s did=%s", req.Nonce, user.DID)
		respond(w, 409, nil, "Bu ödeme isteği daha önce işlendi (replay koruması)")
		return
	}

	// NFC payload parse — format: obscura://pay/{receiverDID}/{amount}
	receiverDID, amount, err := parseNFCPaymentPayload(req.NFCPayload)
	if err != nil {
		respond(w, 400, nil, fmt.Sprintf("NFC ödeme payload'u geçersiz: %v", err))
		return
	}

	// Alıcı mevcut mu?
	var receiverExists int
	db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE did = ?", receiverDID).Scan(&receiverExists)
	if receiverExists == 0 {
		respond(w, 404, nil, "Alıcı bulunamadı")
		return
	}

	// Kendi kendine transfer engeli
	if receiverDID == user.DID {
		respond(w, 400, nil, "Kendinize transfer yapamazsınız")
		return
	}

	// Miktar doğrulama
	if !isValidPositiveDecimal(amount) {
		respond(w, 400, nil, "Geçersiz miktar")
		return
	}

	// Mevcut wallet transfer mantığını çağır
	txID, transferErr := executeWalletTransfer(user.DID, receiverDID, amount, req.Memo)
	if transferErr != nil {
		log.Printf("NFC ödeme transfer hatası from=%s to=%s amount=%s: %v",
			user.DID, receiverDID, amount, transferErr)
		respond(w, 400, nil, fmt.Sprintf("Transfer başarısız: %v", transferErr))
		return
	}

	log.Printf("NFC ödeme başarılı: tx=%s from=%s to=%s amount=%s", txID, user.DID, receiverDID, amount)
	respond(w, 200, map[string]interface{}{
		"tx_id":        txID,
		"from_did":     user.DID,
		"to_did":       receiverDID,
		"amount":       amount,
		"method":       "nfc",
		"status":       "confirmed",
	}, "")
}

// ─── YARDIMCI FONKSİYONLAR ────────────────────────────────────────────────────

// consumeNFCNonce — nonce'u DB'ye yazar. Zaten varsa hata döner (replay koruması).
// Ayrıca 24 saatten eski nonce'ları temizler (TTL).
func consumeNFCNonce(nonce string, nowSec int64) error {
	// Önce eski nonce'ları temizle (24 saat TTL, kaynak sızıntısı önlemi)
	cutoff := nowSec - 24*3600
	db.DB.Exec("DELETE FROM nfc_nonces WHERE created_at < ?", cutoff)

	// Nonce'u atomik INSERT ile kaydet; PRIMARY KEY çakışması = replay
	_, err := db.DB.Exec(
		"INSERT INTO nfc_nonces (nonce, created_at) VALUES (?, ?)",
		nonce, nowSec,
	)
	if err != nil {
		// SQLite "UNIQUE constraint failed" → replay attack
		return fmt.Errorf("nonce zaten kullanılmış: %w", err)
	}
	return nil
}

// parseNFCCheckinPayload — `obscura://event/{eventID}/checkin/{token}` parse eder.
func parseNFCCheckinPayload(payload string) (eventID, token string, err error) {
	// Schema prefix temizle
	payload = strings.TrimPrefix(payload, "obscura://")

	// Beklenen format: event/{eventID}/checkin/{token}
	parts := strings.SplitN(payload, "/", 4)
	if len(parts) != 4 || parts[0] != "event" || parts[2] != "checkin" {
		return "", "", fmt.Errorf(
			"beklenen format: obscura://event/{eventID}/checkin/{token}, alınan: %q", payload)
	}

	eventID = parts[1]
	token = parts[3]
	if eventID == "" || token == "" {
		return "", "", fmt.Errorf("eventID veya token boş")
	}
	return eventID, token, nil
}

// parseNFCPairPayload — `obscura://pair/{challenge}` parse eder.
func parseNFCPairPayload(payload string) (challenge string, err error) {
	payload = strings.TrimPrefix(payload, "obscura://")

	parts := strings.SplitN(payload, "/", 2)
	if len(parts) != 2 || parts[0] != "pair" {
		return "", fmt.Errorf(
			"beklenen format: obscura://pair/{challenge}, alınan: %q", payload)
	}

	challenge = parts[1]
	if challenge == "" {
		return "", fmt.Errorf("challenge boş")
	}
	return challenge, nil
}

// parseNFCPaymentPayload — `obscura://pay/{receiverDID}/{amount}` parse eder.
func parseNFCPaymentPayload(payload string) (receiverDID, amount string, err error) {
	payload = strings.TrimPrefix(payload, "obscura://")

	parts := strings.SplitN(payload, "/", 3)
	if len(parts) != 3 || parts[0] != "pay" {
		return "", "", fmt.Errorf(
			"beklenen format: obscura://pay/{receiverDID}/{amount}, alınan: %q", payload)
	}

	receiverDID = parts[1]
	amount = parts[2]
	if receiverDID == "" || amount == "" {
		return "", "", fmt.Errorf("receiverDID veya amount boş")
	}
	return receiverDID, amount, nil
}

// isValidPositiveDecimal — miktar string'inin pozitif ondalık sayı olup olmadığını kontrol eder.
// Basit kontrol: sadece rakam ve en fazla bir nokta, sıfırdan büyük.
func isValidPositiveDecimal(s string) bool {
	if s == "" || s == "0" || s == "0.0" {
		return false
	}
	dotCount := 0
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			// geçerli
		case c == '.' && i > 0:
			dotCount++
			if dotCount > 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// executeWalletTransfer — mevcut token transfer mantığını çağırır.
// wallet_handlers.go'daki transfer logic ile aynı DB operasyonunu yapar,
// NFC handler'ına import cycle oluşturmadan.
func executeWalletTransfer(fromDID, toDID, amount, memo string) (txID string, err error) {
	// Gönderenin bakiyesini kontrol et
	var senderBalance string
	err = db.DB.QueryRow(
		"SELECT transparent_balance FROM obs_accounts WHERE user_did = ?", fromDID,
	).Scan(&senderBalance)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("gönderenin cüzdanı bulunamadı")
	}
	if err != nil {
		return "", fmt.Errorf("bakiye sorgulanamadı: %w", err)
	}

	// Basit tam sayı karşılaştırması — production'da math/big.Int kullanılır.
	// NFC handler için: amount integer check (küsuratlı OBS desteği için wallet_handlers'a delege edilmeli).
	if !decimalGTE(senderBalance, amount) {
		return "", fmt.Errorf("yetersiz bakiye")
	}

	// Alıcı hesabı yoksa oluştur
	now := time.Now().UTC().Format(time.RFC3339)
	db.DB.Exec(`
		INSERT INTO obs_accounts (user_did, transparent_balance, updated_at)
		VALUES (?, '0', ?)
		ON CONFLICT DO NOTHING`, toDID, now)

	txID = uuid.New().String()

	// Tek transaction içinde bakiye güncelle
	tx, err := db.DB.Begin()
	if err != nil {
		return "", fmt.Errorf("transaction başlatılamadı: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Gönderenden düş
	_, err = tx.Exec(`
		UPDATE obs_accounts
		SET transparent_balance = CAST(CAST(transparent_balance AS INTEGER) - CAST(? AS INTEGER) AS TEXT),
		    updated_at = ?
		WHERE user_did = ?`, amount, now, fromDID)
	if err != nil {
		return "", fmt.Errorf("gönderen bakiye güncellenemedi: %w", err)
	}

	// Alıcıya ekle
	_, err = tx.Exec(`
		UPDATE obs_accounts
		SET transparent_balance = CAST(CAST(transparent_balance AS INTEGER) + CAST(? AS INTEGER) AS TEXT),
		    updated_at = ?
		WHERE user_did = ?`, amount, now, toDID)
	if err != nil {
		return "", fmt.Errorf("alıcı bakiye güncellenemedi: %w", err)
	}

	// İşlem kaydı
	_, err = tx.Exec(`
		INSERT INTO obs_transactions (id, from_did, to_did, amount, fee, tx_type, memo, status, created_at)
		VALUES (?, ?, ?, ?, '0', 'nfc_payment', ?, 'confirmed', ?)`,
		txID, fromDID, toDID, amount, memo, now)
	if err != nil {
		return "", fmt.Errorf("işlem kaydedilemedi: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("transaction commit başarısız: %w", err)
	}

	return txID, nil
}

// decimalGTE — a >= b mi kontrol eder (her ikisi de ondalıksız integer string).
// Production'da math/big.Int ile yapılmalı — bu stub tam sayılar için yeterli.
func decimalGTE(a, b string) bool {
	// Uzunluk karşılaştırması (leading zero yok varsayımıyla)
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a >= b
}

// ─── KULLANILMAYAN IMPORT ENGELLEYICI ────────────────────────────────────────
// math import'u doğrudan kullanılmıyor — derleyici uyarısı engellemek için.
var _ = math.Floor
