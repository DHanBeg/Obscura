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
	"fmt"
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
)

// ─── PATCH /v1/users/me ──────────────────────────────────────────────────────

type UpdateMeRequest struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	Bio         string `json:"bio"`
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
	if req.DisplayName == "" && req.Username == "" && req.AvatarURL == "" && req.Bio == "" {
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
