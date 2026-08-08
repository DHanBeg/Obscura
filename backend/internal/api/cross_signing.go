package api

// Cross-signing — çoklu cihaz onayı (spec Bölüm 5.4).
//
// Akış:
//   1) Yeni cihaz: POST /v1/devices/pair/start
//      → server bir challenge + pairing_id üretir
//      → server cevap döner: { pairing_id, challenge, qr_payload }
//      → yeni cihaz QR kod gösterir (qr_payload içeriği)
//   2) Birincil cihaz QR'ı okur, içindeki challenge + new_device_pubkey'i imzalar
//   3) Birincil: POST /v1/devices/pair/approve { pairing_id, signature_b64 }
//      → server imzayı birincil cihazın anahtarıyla doğrular
//      → yeni cihaz devices tablosuna eklenir, pairing_request status=approved
//   4) Yeni cihaz: GET /v1/devices/pair/status/{id}
//      → status pending|approved|expired
//      → approved ise yeni cihaz auth token alabilir
//
// Spec'e göre kurtarma cihazı için 7 gün timelock + email — bu FAZ 2'ye bırakıldı.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
)

// ─── N9: IP-based rate limiting for the unauthenticated HandlePairStart ───────
// Max 5 pairing starts per IP per minute prevents DB flooding → disk fill → crash.

var (
	pairRateMu  sync.Mutex
	pairRateMap = make(map[string]*pairRateEntry)
)

type pairRateEntry struct {
	count     int
	windowEnd time.Time
}

const (
	pairRateLimit  = 5
	pairRateWindow = time.Minute
)

func pairRateAllow(ip string) bool {
	pairRateMu.Lock()
	defer pairRateMu.Unlock()
	now := time.Now()
	e, ok := pairRateMap[ip]
	if !ok || now.After(e.windowEnd) {
		pairRateMap[ip] = &pairRateEntry{count: 1, windowEnd: now.Add(pairRateWindow)}
		return true
	}
	if e.count >= pairRateLimit {
		return false
	}
	e.count++
	return true
}

// N8: DID format validation — did:obs: + 32 hex chars, kanonik format
// (auth.GenerateDID/mobile sealed-sender.ts:didFromDhPublic ile aynı,
// SHA256(identity_key)[:16 byte] hex — bkz. #23 DID şema doğrulaması).
var obsDidRegex = regexp.MustCompile(`^did:obs:[0-9a-f]{32}$`)

// PairSigDomain is the domain separator bound into pairing signatures.
// Prevents cross-protocol signature reuse and binds the signed message to
// this specific pairing flow + version.
const PairSigDomain = "obscura-pair-v1"

type PairStartRequest struct {
	NewDevicePubkeyB64 string `json:"new_device_pubkey_b64"` // ed25519 public, 32 bytes
	NewDeviceName      string `json:"new_device_name"`
	// TargetDIDHint is informational only — displayed in the QR/UX so the
	// primary device can confirm "you are adding device X to YOUR account".
	// Server does NOT trust this for authorization; the authoritative DID
	// is taken from the JWT-authenticated caller in HandlePairApprove.
	TargetDIDHint string `json:"target_did_hint,omitempty"`
}

type PairApproveRequest struct {
	PairingID    string `json:"pairing_id"`
	SignatureB64 string `json:"signature_b64"` // signed by primary device's identity key
}

// POST /v1/devices/pair/start
// Yeni cihaz çağırır (henüz auth yok — public endpoint).
// Cevap: pairing_id + challenge + qr_payload
func HandlePairStart(w http.ResponseWriter, r *http.Request) {
	// N9: IP-based rate limit — prevents DB flooding from unauthenticated callers.
	ip := r.RemoteAddr
	if idx := len(ip) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if ip[i] == ':' {
				ip = ip[:i]
				break
			}
		}
	}
	if !pairRateAllow(ip) {
		respond(w, 429, nil, "Çok fazla pairing isteği — 1 dakika sonra tekrar deneyin")
		return
	}

	var req PairStartRequest
	if err := decodeBody(r, &req); err != nil || req.NewDevicePubkeyB64 == "" {
		respond(w, 400, nil, "new_device_pubkey_b64 zorunlu")
		return
	}

	// N8: Validate optional target_did_hint format before inserting into QR payload.
	if req.TargetDIDHint != "" && !obsDidRegex.MatchString(req.TargetDIDHint) {
		respond(w, 400, nil, "Geçersiz target_did_hint formatı (did:obs:<32 hex> bekleniyor)")
		return
	}

	// Pubkey validate
	pubkey, err := base64.StdEncoding.DecodeString(req.NewDevicePubkeyB64)
	if err != nil || len(pubkey) != ed25519.PublicKeySize {
		respond(w, 400, nil, "Geçersiz ed25519 public key (32 byte base64)")
		return
	}

	// Challenge: 32 byte random
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		respond(w, 500, nil, "Challenge üretilemedi")
		return
	}
	challengeB64 := base64.StdEncoding.EncodeToString(challenge)

	pairingID := uuid.New().String()
	now := time.Now().UTC()
	expires := now.Add(10 * time.Minute) // QR + onay için kısa pencere

	_, err = db.DB.Exec(`
		INSERT INTO pairing_requests
			(id, user_did, challenge, new_device_pubkey, new_device_name, status, created_at, expires_at)
		VALUES (?, '', ?, ?, ?, 'pending', ?, ?)`,
		pairingID, challengeB64, req.NewDevicePubkeyB64, req.NewDeviceName,
		now.Format(time.RFC3339), expires.Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Pairing oluşturulamadı: "+err.Error())
		return
	}

	// QR payload: birincil cihaz okur.
	// target_did=<hint> primary cihazın "kendi hesabına" eklediğini doğrulaması için
	// gösterilir (informational; server JWT'den gelen DID'i kullanır).
	qrPayload := fmt.Sprintf("obscura://device-pair/%s/%s/%s?target_did=%s",
		pairingID, challengeB64, req.NewDevicePubkeyB64, req.TargetDIDHint)

	respond(w, 200, map[string]any{
		"pairing_id":  pairingID,
		"challenge":   challengeB64,
		"qr_payload":  qrPayload,
		"expires_at":  expires.Format(time.RFC3339),
	}, "")
}

// POST /v1/devices/pair/approve  (auth gerektirir — birincil cihaz çağırır)
// Birincil cihaz, challenge + new_device_pubkey üzerinde ed25519 imzasını sunar.
// Server imzayı user'ın identity_key'i ile doğrular.
func HandlePairApprove(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req PairApproveRequest
	if err := decodeBody(r, &req); err != nil ||
		req.PairingID == "" || req.SignatureB64 == "" {
		respond(w, 400, nil, "pairing_id ve signature_b64 zorunlu")
		return
	}

	var challenge, newDevicePubkey, status, expiresAt string
	err := db.DB.QueryRow(`
		SELECT challenge, new_device_pubkey, status, expires_at
		FROM pairing_requests WHERE id = ?`,
		req.PairingID).Scan(&challenge, &newDevicePubkey, &status, &expiresAt)
	if err != nil {
		respond(w, 404, nil, "Pairing isteği bulunamadı")
		return
	}
	if status != "pending" {
		respond(w, 400, nil, "Pairing zaten "+status)
		return
	}

	exp, _ := time.Parse(time.RFC3339, expiresAt)
	if time.Now().UTC().After(exp) {
		db.DB.Exec(`UPDATE pairing_requests SET status = 'expired' WHERE id = ?`, req.PairingID)
		respond(w, 400, nil, "Pairing isteği zaman aşımına uğradı")
		return
	}

	// İmza doğrulama: msg = challenge || new_device_pubkey
	identityPubkey, err := base64.StdEncoding.DecodeString(user.IdentityKey)
	if err != nil || len(identityPubkey) != ed25519.PublicKeySize {
		respond(w, 400, nil, "Birincil cihaz identity_key geçersiz")
		return
	}

	// SECURITY (C5 fix): Bind caller DID + domain separator into signed message.
	// Eski format: challenge || new_device_pubkey  →  saldırgan kurbanı kandırarak
	// kendi pubkey'ini kurban hesabına imzalatabilirdi. Yeni format DID'i de
	// zorunlu kılar; server JWT'den gelen güvenilir DID ile yeniden inşa eder.
	//   sig_msg = "obscura-pair-v1" + ":" + base64(challenge) + ":" +
	//             base64(new_device_pubkey) + ":" + caller_did
	if _, err := base64.StdEncoding.DecodeString(challenge); err != nil {
		respond(w, 500, nil, "Stored challenge geçersiz")
		return
	}
	if _, err := base64.StdEncoding.DecodeString(newDevicePubkey); err != nil {
		respond(w, 500, nil, "Stored new_device_pubkey geçersiz")
		return
	}
	sigMsg := []byte(PairSigDomain + ":" + challenge + ":" + newDevicePubkey + ":" + user.DID)

	sigBytes, err := base64.StdEncoding.DecodeString(req.SignatureB64)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		respond(w, 400, nil, "İmza geçersiz formatta (64 byte base64)")
		return
	}

	if !ed25519.Verify(identityPubkey, sigMsg, sigBytes) {
		respond(w, 403, nil, "İmza doğrulanamadı (birincil cihaz değil veya yanlış imza)")
		return
	}

	// Approved — yeni cihazı kaydet
	now := time.Now().UTC().Format(time.RFC3339)
	deviceID := uuid.New().String()

	_, err = db.DB.Exec(`
		INSERT INTO devices
			(id, user_did, device_pubkey, device_name, device_type, signed_by, signature_b64, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'secondary', ?, ?, ?, ?)`,
		deviceID, user.DID, newDevicePubkey, "", user.IdentityKey, req.SignatureB64, now, now,
	)
	if err != nil {
		respond(w, 500, nil, "Cihaz kaydedilemedi: "+err.Error())
		return
	}

	_, _ = db.DB.Exec(`
		UPDATE pairing_requests
		SET status = 'approved', approved_at = ?, signature_b64 = ?, user_did = ?
		WHERE id = ?`,
		now, req.SignatureB64, user.DID, req.PairingID,
	)

	respond(w, 200, map[string]any{
		"approved":  true,
		"device_id": deviceID,
		"user_did":  user.DID,
	}, "")
}

// GET /v1/devices/pair/status/{id}  (PUBLIC — yeni cihaz polling yapar)
func HandlePairStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pairingID := vars["id"]

	var status, userDID, expiresAt, approvedAt string
	err := db.DB.QueryRow(`
		SELECT status, COALESCE(user_did, ''), expires_at, COALESCE(approved_at, '')
		FROM pairing_requests WHERE id = ?`,
		pairingID).Scan(&status, &userDID, &expiresAt, &approvedAt)
	if err != nil {
		respond(w, 404, nil, "Pairing bulunamadı")
		return
	}

	exp, _ := time.Parse(time.RFC3339, expiresAt)
	if status == "pending" && time.Now().UTC().After(exp) {
		status = "expired"
		db.DB.Exec(`UPDATE pairing_requests SET status = 'expired' WHERE id = ?`, pairingID)
	}

	resp := map[string]any{
		"status":     status,
		"expires_at": expiresAt,
	}
	if status == "approved" {
		resp["user_did"] = userDID
		resp["approved_at"] = approvedAt
	}
	respond(w, 200, resp, "")
}

// GET /v1/devices  — kullanıcının cihaz listesi (auth)
func HandleListDevices(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	rows, err := db.DB.Query(`
		SELECT id, device_pubkey, device_name, device_type, created_at,
		       COALESCE(last_seen_at, ''), COALESCE(revoked_at, '')
		FROM devices WHERE user_did = ?
		ORDER BY created_at DESC`,
		user.DID)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	defer rows.Close()

	type d struct {
		ID         string `json:"id"`
		Pubkey     string `json:"pubkey"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		CreatedAt  string `json:"created_at"`
		LastSeenAt string `json:"last_seen_at,omitempty"`
		RevokedAt  string `json:"revoked_at,omitempty"`
	}
	var out []d
	for rows.Next() {
		var x d
		rows.Scan(&x.ID, &x.Pubkey, &x.Name, &x.Type, &x.CreatedAt, &x.LastSeenAt, &x.RevokedAt)
		out = append(out, x)
	}
	respond(w, 200, out, "")
}

// DELETE /v1/devices/{id}  — cihaz iptali
func HandleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	deviceID := vars["id"]

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.DB.Exec(`
		UPDATE devices SET revoked_at = ?
		WHERE id = ? AND user_did = ? AND revoked_at IS NULL`,
		now, deviceID, user.DID)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		respond(w, 404, nil, "Cihaz bulunamadı veya zaten iptal edilmiş")
		return
	}
	respond(w, 200, map[string]any{"revoked": true}, "")
}

var _ = json.Marshal
