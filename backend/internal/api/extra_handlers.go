package api

// Ek HTTP handler'lar — FAZ 3
//
// PATCH /v1/users/me          → Profil güncelle
// POST  /v1/conversations     → Yeni grup konuşması başlat
// DELETE /v1/messages/{id}    → Mesaj sil (soft delete)
// GET   /v1/credit/history    → Kredi geçmişi
// POST  /v1/media/upload      → Medya yükle (MinIO)
// POST  /v1/devices/register  → FCM/APNs token kaydet (push bildirim)

import (
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/media"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/zk"
)

// hexDecode — encoding/hex.DecodeString wrapper (compile-time symbol gereksinimi için)
func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// ─── PATCH /v1/users/me ──────────────────────────────────────────────────────

type UpdateMeRequest struct {
	DisplayName     string `json:"display_name"`
	Username        string `json:"username"`
	AvatarURL       string `json:"avatar_url"`
	Bio             string `json:"bio"`
	DilithiumPubKey string `json:"dilithium_pub_key,omitempty"` // hex; Dilithium3 genel anahtar kaydı
}

func HandleUpdateMe(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req UpdateMeRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	// En az bir alan güncellenmeli
	if req.DisplayName == "" && req.Username == "" && req.AvatarURL == "" && req.Bio == "" && req.DilithiumPubKey == "" {
		respond(w, 400, nil, "Güncellenecek alan bulunamadı")
		return
	}

	// Kullanıcı adı benzersizlik kontrolü
	if req.Username != "" && req.Username != user.Username {
		var exists int
		db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? AND id != ?",
			req.Username, user.ID).Scan(&exists)
		if exists > 0 {
			respond(w, 409, nil, "Bu kullanıcı adı zaten kullanımda")
			return
		}
		// Kullanıcı adı format kontrolü
		if len(req.Username) < 3 || len(req.Username) > 32 {
			respond(w, 400, nil, "Kullanıcı adı 3-32 karakter arasında olmalı")
			return
		}
	}

	// Dinamik UPDATE sorgusu oluştur
	setClauses := []string{}
	args := []interface{}{}

	if req.DisplayName != "" {
		setClauses = append(setClauses, "display_name = ?")
		args = append(args, req.DisplayName)
	}
	if req.Username != "" {
		setClauses = append(setClauses, "username = ?")
		args = append(args, req.Username)
	}
	if req.AvatarURL != "" {
		setClauses = append(setClauses, "avatar_url = ?")
		args = append(args, req.AvatarURL)
	}
	if req.DilithiumPubKey != "" {
		// Geçerli hex ve doğru boyut kontrolü
		// Dilithium3 (mode3) public key: 1952 byte → 3904 hex karakter
		if len(req.DilithiumPubKey) != 3904 {
			respond(w, 400, nil, fmt.Sprintf("Geçersiz dilithium_pub_key: 3904 hex karakter bekleniyor, %d geldi", len(req.DilithiumPubKey)))
			return
		}
		if _, hexErr := hexDecode(req.DilithiumPubKey); hexErr != nil {
			respond(w, 400, nil, "Geçersiz dilithium_pub_key: geçerli hex değil")
			return
		}
		setClauses = append(setClauses, "dilithium_pub_key = ?")
		args = append(args, req.DilithiumPubKey)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, user.ID)

	query := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = ?",
		strings.Join(setClauses, ", "),
	)

	if _, err := db.DB.Exec(query, args...); err != nil {
		respond(w, 500, nil, "Profil güncellenemedi")
		return
	}

	// Güncel kullanıcıyı getir
	var updated models.User
	db.DB.QueryRow(`
		SELECT id, phone, username, display_name, did, identity_key, avatar_url,
		       tier, credit_score, is_active, is_banned, node_id
		FROM users WHERE id = ?`, user.ID,
	).Scan(&updated.ID, &updated.Phone, &updated.Username, &updated.DisplayName,
		&updated.DID, &updated.IdentityKey, &updated.AvatarURL,
		&updated.Tier, &updated.CreditScore, &updated.IsActive, &updated.IsBanned, &updated.NodeID)

	respond(w, 200, updated, "")
}

// ─── POST /v1/conversations ───────────────────────────────────────────────────

type CreateConversationRequest struct {
	// 1-1 konuşma için peer DID
	PeerDID string `json:"peer_did,omitempty"`
	// Grup konuşması için
	IsGroup bool     `json:"is_group,omitempty"`
	Name    string   `json:"name,omitempty"`
	Members []string `json:"members,omitempty"` // DID listesi
}

func HandleCreateConversation(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req CreateConversationRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	now := time.Now()

	if !req.IsGroup {
		// 1-1 konuşma — findOrCreate kullan
		if req.PeerDID == "" {
			respond(w, 400, nil, "peer_did zorunlu (1-1 konuşma için)")
			return
		}
		convID, err := findOrCreateConversation(user.DID, req.PeerDID, false)
		if err != nil {
			respond(w, 500, nil, "Konuşma oluşturulamadı")
			return
		}
		respond(w, 201, map[string]string{"conv_id": convID}, "")
		return
	}

	// Grup konuşması oluştur
	if req.Name == "" {
		respond(w, 400, nil, "Grup adı zorunlu")
		return
	}
	if len(req.Members) < 2 {
		respond(w, 400, nil, "En az 2 üye gerekli")
		return
	}

	convID := uuid.New().String()
	_, err := db.DB.Exec(`
		INSERT INTO conversations (id, is_group, name, created_at, updated_at)
		VALUES (?, 1, ?, ?, ?)`,
		convID, req.Name, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "Grup oluşturulamadı")
		return
	}

	// Üyeleri ekle (kurucu dahil)
	allMembers := append([]string{user.DID}, req.Members...)
	for _, did := range allMembers {
		db.DB.Exec(`
			INSERT OR IGNORE INTO conv_members (conv_id, user_did, joined_at, unread_count)
			VALUES (?, ?, ?, 0)`,
			convID, did, now.Format(time.RFC3339),
		)
	}

	// Üyelere WebSocket bildirimi gönder
	for _, did := range allMembers {
		messaging.GlobalHub.SendTo(did, "group_created", map[string]interface{}{
			"conv_id":    convID,
			"name":       req.Name,
			"created_by": user.DID,
			"members":    allMembers,
		})
	}

	respond(w, 201, map[string]interface{}{
		"conv_id": convID,
		"name":    req.Name,
		"members": len(allMembers),
	}, "")
}

// ─── DELETE /v1/messages/{id} ─────────────────────────────────────────────────

func HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	vars := mux.Vars(r)
	msgID := vars["id"]

	// Mesaj sahibi mi?
	var fromDID, toID, convID string
	err := db.DB.QueryRow("SELECT from_did, to_did, conv_id FROM messages WHERE id = ? AND deleted_at IS NULL",
		msgID).Scan(&fromDID, &toID, &convID)
	if err != nil {
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	if fromDID != user.DID {
		respond(w, 403, nil, "Bu mesajı silemezsiniz")
		return
	}

	// Soft delete
	now := time.Now()
	db.DB.Exec("UPDATE messages SET deleted_at = ?, ciphertext = '[Mesaj silindi]' WHERE id = ?",
		now.Format(time.RFC3339), msgID)

	// Alıcıya bildir
	messaging.GlobalHub.SendTo(toID, "message_deleted", map[string]interface{}{
		"msg_id":  msgID,
		"conv_id": convID,
	})

	respond(w, 200, map[string]string{"status": "deleted"}, "")
}

// ─── GET /v1/credit/history ───────────────────────────────────────────────────

func HandleGetCreditHistory(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	// limit query param
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit > 200 {
			limit = 200
		}
		if limit < 1 {
			limit = 1
		}
	}

	history, err := credit.GetHistory(user.DID, limit)
	if err != nil {
		respond(w, 500, nil, "Geçmiş alınamadı")
		return
	}

	if history == nil {
		history = []models.CreditEvent{}
	}

	respond(w, 200, map[string]interface{}{
		"history": history,
		"count":   len(history),
		"score":   user.CreditScore,
		"tier":    user.Tier,
	}, "")
}

// ─── POST /v1/media/upload ────────────────────────────────────────────────────

func HandleMediaUpload(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	// Max 10MB
	r.ParseMultipartForm(10 << 20)

	file, header, err := r.FormFile("file")
	if err != nil {
		respond(w, 400, nil, "Dosya bulunamadı (form field: file)")
		return
	}
	defer file.Close()

	// Tier bazlı boyut limiti
	maxSize := int64(5 << 20) // 5MB default
	if user.Tier >= 3 {
		maxSize = 50 << 20 // 50MB Tier 3+
	} else if user.Tier >= 2 {
		maxSize = 25 << 20 // 25MB Tier 2
	}

	if header.Size > maxSize {
		respond(w, 413, nil, fmt.Sprintf("Dosya boyutu limiti: %dMB", maxSize>>20))
		return
	}

	// Medya tipini tespit et
	mediaType := r.FormValue("type") // avatar, media, voice
	if mediaType == "" {
		mediaType = "media"
	}

	// MinIO'ya yükle
	objectKey := fmt.Sprintf("%s/%s/%s", mediaType, user.DID[:8], uuid.New().String())
	url, err := media.Upload(r.Context(), objectKey, file, header.Size, header.Header.Get("Content-Type"))
	if err != nil {
		respond(w, 500, nil, "Dosya yüklenemedi")
		return
	}

	respond(w, 200, map[string]string{
		"url":  url,
		"key":  objectKey,
		"type": mediaType,
	}, "")
}

// ─── POST /v1/devices/register ───────────────────────────────────────────────
// FCM veya APNs push token'ı kaydet

func HandleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Platform string `json:"platform"` // "fcm" | "apns"
		Token    string `json:"token"`
	}
	if err := decodeBody(r, &req); err != nil || req.Token == "" {
		respond(w, 400, nil, "platform ve token zorunlu")
		return
	}

	now := time.Now().Format(time.RFC3339)
	switch req.Platform {
	case "fcm":
		db.DB.Exec("UPDATE users SET fcm_token = ?, updated_at = ? WHERE id = ?",
			req.Token, now, user.ID)
	case "apns":
		db.DB.Exec("UPDATE users SET apns_token = ?, updated_at = ? WHERE id = ?",
			req.Token, now, user.ID)
	default:
		respond(w, 400, nil, "platform 'fcm' veya 'apns' olmalı")
		return
	}

	respond(w, 200, map[string]string{"status": "registered"}, "")
}

// ─── POST /v1/auth/zk-id-update ──────────────────────────────────────────────
//
// Kimlik doğrulanmış kullanıcıların sonradan ZK-ID kanıtı yüklemesi için.
// Spec Bölüm 5.2: kayıt sırasında kanıt gönderilmemişse buradan gönderilebilir.
// Secret asla backend'e gönderilmez; sadece Groth16 proof + publicSignals gelir.

func HandleZKIDUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req models.ZKIDUpdateRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if req.ZKIDProof == "" || req.ZKIDPublic == "" {
		respond(w, 400, nil, "zk_id_proof ve zk_id_public zorunlu")
		return
	}

	// Zaten doğrulanmış mı?
	var already int
	db.DB.QueryRow("SELECT COALESCE(zk_id_verified, 0) FROM users WHERE id = ?", user.ID).Scan(&already)
	if already == 1 {
		respond(w, 200, map[string]interface{}{
			"zk_id_verified": true,
			"message":        "ZK kimliği zaten doğrulanmış",
		}, "")
		return
	}

	// base64 / raw JSON decode
	proofBytes, pubBytes, decErr := decodeZKIDFields(req.ZKIDProof, req.ZKIDPublic)
	if decErr != nil {
		respond(w, 400, nil, fmt.Sprintf("ZK-ID alanları geçersiz: %v", decErr))
		return
	}

	pubSignals, parseErr := parsePublicSignals(pubBytes)
	if parseErr != nil {
		respond(w, 400, nil, fmt.Sprintf("ZK-ID public params geçersiz: %v", parseErr))
		return
	}

	// Groth16 doğrulama
	if verErr := zk.VerifyGroth16(zk.CircuitIdentityProof, proofBytes, pubSignals); verErr != nil {
		log.Printf("ZK-ID güncelleme doğrulama başarısız (did=%s): %v", user.DID, verErr)
		respond(w, 400, nil, "ZK kimlik kanıtı geçersiz")
		return
	}

	// Kaydet — users tablosu + zk_proofs indeksi (spec Ek B)
	now := time.Now()
	_, dbErr := db.DB.Exec(`
		UPDATE users
		SET zk_id_proof_b64 = ?, zk_id_public_params = ?, zk_id_verified = 1, updated_at = ?
		WHERE id = ?`,
		req.ZKIDProof, req.ZKIDPublic, now.Format(time.RFC3339), user.ID,
	)
	if dbErr != nil {
		log.Printf("ZK-ID kayıt hatası (did=%s): %v", user.DID, dbErr)
		respond(w, 500, nil, "ZK kimliği kaydedilemedi")
		return
	}

	// zk_proofs tablosuna da ekle — kredi/airdrop sistemleri bu tabloyu sorgular
	zkProofID := uuid.New().String()
	if _, zkErr := db.DB.Exec(`
		INSERT INTO zk_proofs (id, user_did, circuit_id, proof_data, public_inputs, verified, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		zkProofID, user.DID, string(zk.CircuitIdentityProof),
		req.ZKIDProof, req.ZKIDPublic,
		now.Format(time.RFC3339),
		now.Add(365*24*time.Hour).Format(time.RFC3339),
	); zkErr != nil {
		// Non-fatal: users tablosu başarıyla güncellendi; sadece log at
		log.Printf("ZK-ID zk_proofs kayıt uyarısı (did=%s): %v", user.DID, zkErr)
	}

	log.Printf("ZK-ID doğrulandı ve kaydedildi (did=%s)", user.DID)
	respond(w, 200, map[string]interface{}{
		"zk_id_verified": true,
		"message":        "ZK kimliği doğrulandı",
	}, "")
}

// ─── POST /v1/messages/{id}/read ─────────────────────────────────────────────
//
// Alıcı mesajı okuduğunda çağırır. Sadece alıcı (to_did) çağırabilir —
// gönderen kendi mesajını "okundu" olarak işaretleyemez (spec Bölüm 6.4).
// Başarı sonrası gönderene WebSocket read_receipt iletilir.

func HandleMarkMessageRead(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	vars := mux.Vars(r)
	msgID := vars["id"]
	if msgID == "" {
		respond(w, 400, nil, "msg_id zorunlu")
		return
	}

	// Mesajı getir — alıcı DID ve mevcut durum kontrolü için
	var fromDID, toDID, currentStatus string
	err := db.DB.QueryRow(
		"SELECT from_did, to_did, status FROM messages WHERE id = ? AND deleted_at IS NULL",
		msgID,
	).Scan(&fromDID, &toDID, &currentStatus)
	if err != nil {
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	// Sadece alıcı okundu işaretleyebilir
	if toDID != user.DID {
		respond(w, 403, nil, "Bu mesajı okundu olarak işaretleyemezsiniz")
		return
	}

	// Zaten okunmuşsa tekrar işlem yapma
	if currentStatus == string(models.StatusRead) {
		respond(w, 200, map[string]string{"status": "already_read"}, "")
		return
	}

	now := time.Now()
	_, dbErr := db.DB.Exec(
		"UPDATE messages SET status = 'read', read_at = ? WHERE id = ?",
		now.Format(time.RFC3339), msgID,
	)
	if dbErr != nil {
		log.Printf("HandleMarkMessageRead DB hatası (msg=%s): %v", msgID, dbErr)
		respond(w, 500, nil, "Durum güncellenemedi")
		return
	}

	// Gönderene WebSocket read_receipt ilet
	messaging.GlobalHub.SendReadReceipt(fromDID, msgID, user.DID)

	respond(w, 200, map[string]interface{}{
		"msg_id":  msgID,
		"status":  "read",
		"read_at": now.Format(time.RFC3339),
	}, "")
}

// ─── GET /v1/messages/{id}/status ────────────────────────────────────────────
//
// Mesajın güncel durumunu döndürür. Mesajın göndereni veya alıcısı sorgulayabilir.

func HandleGetMessageStatus(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	vars := mux.Vars(r)
	msgID := vars["id"]
	if msgID == "" {
		respond(w, 400, nil, "msg_id zorunlu")
		return
	}

	var fromDID, toDID, status string

	// deliveredAt ve readAt nullable
	var deliveredAtSQL, readAtSQL interface{}

	err := db.DB.QueryRow(
		"SELECT from_did, to_did, status, delivered_at, read_at FROM messages WHERE id = ? AND deleted_at IS NULL",
		msgID,
	).Scan(&fromDID, &toDID, &status, &deliveredAtSQL, &readAtSQL)
	if err != nil {
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	// Sadece gönderen veya alıcı sorgulayabilir
	if user.DID != fromDID && user.DID != toDID {
		respond(w, 403, nil, "Bu mesajın durumunu sorgulama yetkiniz yok")
		return
	}

	out := map[string]interface{}{
		"msg_id": msgID,
		"status": status,
	}
	if deliveredAtSQL != nil {
		out["delivered_at"] = deliveredAtSQL
	}
	if readAtSQL != nil {
		out["read_at"] = readAtSQL
	}

	respond(w, 200, out, "")
}
