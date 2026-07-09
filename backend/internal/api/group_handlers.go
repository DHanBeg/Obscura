package api

// Grup/kanal/topluluk profil-yönetim handler'ları.
//
// GET    /v1/conversations/{id}                  → konuşma detayı (tip, açıklama, üye sayısı, kendi rolüm)
// GET    /v1/conversations/{id}/members           → üye listesi (rol, tier, katılma tarihi)
// PATCH  /v1/conversations/{id}                   → ad/açıklama/avatar/görünürlük güncelle (admin)
// POST   /v1/conversations/{id}/leave             → konuşmadan ayrıl
// DELETE /v1/conversations/{id}/members/{did}     → üyeyi at (admin)
// PATCH  /v1/conversations/{id}/members/{did}     → rol değiştir (admin)

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
)

func isConvMember(convID, did string) (bool, string) {
	var role string
	err := db.DB.QueryRow(
		"SELECT role FROM conv_members WHERE conv_id = ? AND user_did = ?",
		convID, did,
	).Scan(&role)
	if err != nil {
		return false, ""
	}
	return true, role
}

// ─── GET /v1/conversations/{id} ────────────────────────────────────────────

func HandleGetConversationDetail(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	convID := mux.Vars(r)["id"]

	isMember, role := isConvMember(convID, user.DID)
	if !isMember {
		respond(w, 403, nil, "Bu konuşmaya erişiminiz yok")
		return
	}

	var name, convType, description, avatarURL string
	var isGroup, isPublic bool
	var createdAt string
	err := db.DB.QueryRow(`
		SELECT name, conv_type, description, COALESCE(avatar_url,''), is_group, is_public, created_at
		FROM conversations WHERE id = ?`, convID,
	).Scan(&name, &convType, &description, &avatarURL, &isGroup, &isPublic, &createdAt)
	if err != nil {
		respond(w, 404, nil, "Konuşma bulunamadı")
		return
	}

	var memberCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM conv_members WHERE conv_id = ?", convID).Scan(&memberCount)

	respond(w, 200, map[string]interface{}{
		"id":           convID,
		"name":         name,
		"conv_type":    convType,
		"description":  description,
		"avatar_url":   avatarURL,
		"is_group":     isGroup,
		"is_public":    isPublic,
		"member_count": memberCount,
		"my_role":      role,
		"created_at":   createdAt,
	}, "")
}

// ─── GET /v1/conversations/{id}/members ────────────────────────────────────

type ConvMemberInfo struct {
	DID         string `json:"did"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Tier        int    `json:"tier"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at"`
}

func HandleGetConversationMembers(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	convID := mux.Vars(r)["id"]

	isMember, _ := isConvMember(convID, user.DID)
	if !isMember {
		respond(w, 403, nil, "Bu konuşmaya erişiminiz yok")
		return
	}

	rows, err := db.DB.Query(`
		SELECT u.did, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), COALESCE(u.tier,0),
		       cm.role, cm.joined_at
		FROM conv_members cm
		JOIN users u ON u.did = cm.user_did
		WHERE cm.conv_id = ?
		ORDER BY CASE cm.role WHEN 'admin' THEN 0 ELSE 1 END, cm.joined_at ASC`, convID,
	)
	if err != nil {
		respond(w, 500, nil, "Üyeler alınamadı")
		return
	}
	defer rows.Close()

	members := []ConvMemberInfo{}
	for rows.Next() {
		var m ConvMemberInfo
		if err := rows.Scan(&m.DID, &m.DisplayName, &m.AvatarURL, &m.Tier, &m.Role, &m.JoinedAt); err != nil {
			continue
		}
		members = append(members, m)
	}

	respond(w, 200, members, "")
}

// ─── PATCH /v1/conversations/{id} ──────────────────────────────────────────

func HandleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	convID := mux.Vars(r)["id"]

	isMember, role := isConvMember(convID, user.DID)
	if !isMember {
		respond(w, 403, nil, "Bu konuşmaya erişiminiz yok")
		return
	}
	if role != "admin" {
		respond(w, 403, nil, "Sadece yöneticiler düzenleyebilir")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		AvatarURL   *string `json:"avatar_url"`
		IsPublic    *bool   `json:"is_public"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	now := time.Now().Format(time.RFC3339)
	if req.Name != nil {
		if *req.Name == "" {
			respond(w, 400, nil, "Ad boş olamaz")
			return
		}
		db.DB.Exec("UPDATE conversations SET name = ?, updated_at = ? WHERE id = ?", *req.Name, now, convID)
	}
	if req.Description != nil {
		db.DB.Exec("UPDATE conversations SET description = ?, updated_at = ? WHERE id = ?", *req.Description, now, convID)
	}
	if req.AvatarURL != nil {
		db.DB.Exec("UPDATE conversations SET avatar_url = ?, updated_at = ? WHERE id = ?", *req.AvatarURL, now, convID)
	}
	if req.IsPublic != nil {
		isPublicInt := 0
		if *req.IsPublic {
			isPublicInt = 1
		}
		db.DB.Exec("UPDATE conversations SET is_public = ?, updated_at = ? WHERE id = ?", isPublicInt, now, convID)
	}

	rows, _ := db.DB.Query("SELECT user_did FROM conv_members WHERE conv_id = ?", convID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var did string
			if rows.Scan(&did) == nil {
				messaging.GlobalHub.SendTo(did, "group_updated", map[string]interface{}{"conv_id": convID})
			}
		}
	}

	respond(w, 200, map[string]string{"status": "updated"}, "")
}

// ─── POST /v1/conversations/{id}/leave ─────────────────────────────────────

func HandleLeaveConversation(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	convID := mux.Vars(r)["id"]

	isMember, role := isConvMember(convID, user.DID)
	if !isMember {
		respond(w, 403, nil, "Bu konuşmanın üyesi değilsiniz")
		return
	}

	db.DB.Exec("DELETE FROM conv_members WHERE conv_id = ? AND user_did = ?", convID, user.DID)

	// Son admin ayrıldıysa, en eski kalan üyeyi otomatik admin yap — grup sahipsiz kalmasın.
	if role == "admin" {
		var remainingAdmins int
		db.DB.QueryRow("SELECT COUNT(*) FROM conv_members WHERE conv_id = ? AND role = 'admin'", convID).Scan(&remainingAdmins)
		if remainingAdmins == 0 {
			var nextDID string
			err := db.DB.QueryRow(
				"SELECT user_did FROM conv_members WHERE conv_id = ? ORDER BY joined_at ASC LIMIT 1", convID,
			).Scan(&nextDID)
			if err == nil {
				db.DB.Exec("UPDATE conv_members SET role = 'admin' WHERE conv_id = ? AND user_did = ?", convID, nextDID)
				messaging.GlobalHub.SendTo(nextDID, "promoted_to_admin", map[string]interface{}{"conv_id": convID})
			}
		}
	}

	messaging.GlobalHub.SendTo(user.DID, "left_conversation", map[string]interface{}{"conv_id": convID})
	respond(w, 200, map[string]string{"status": "left"}, "")
}

// ─── DELETE /v1/conversations/{id}/members/{did} ───────────────────────────

func HandleRemoveConvMember(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	convID := vars["id"]
	targetDID := vars["did"]

	if targetDID == user.DID {
		respond(w, 400, nil, "Kendinizi atamazsınız, konuşmadan ayrılın")
		return
	}

	isMember, role := isConvMember(convID, user.DID)
	if !isMember || role != "admin" {
		respond(w, 403, nil, "Sadece yöneticiler üye atabilir")
		return
	}

	targetIsMember, _ := isConvMember(convID, targetDID)
	if !targetIsMember {
		respond(w, 404, nil, "Üye bulunamadı")
		return
	}

	db.DB.Exec("DELETE FROM conv_members WHERE conv_id = ? AND user_did = ?", convID, targetDID)
	messaging.GlobalHub.SendTo(targetDID, "removed_from_conversation", map[string]interface{}{"conv_id": convID})
	respond(w, 200, map[string]string{"status": "removed"}, "")
}

// ─── PATCH /v1/conversations/{id}/members/{did} ────────────────────────────

func HandleUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	convID := vars["id"]
	targetDID := vars["did"]

	isMember, role := isConvMember(convID, user.DID)
	if !isMember || role != "admin" {
		respond(w, 403, nil, "Sadece yöneticiler rol değiştirebilir")
		return
	}

	var req struct {
		Role string `json:"role"` // "admin" | "member"
	}
	if err := decodeBody(r, &req); err != nil || (req.Role != "admin" && req.Role != "member") {
		respond(w, 400, nil, "role \"admin\" veya \"member\" olmalı")
		return
	}

	targetIsMember, _ := isConvMember(convID, targetDID)
	if !targetIsMember {
		respond(w, 404, nil, "Üye bulunamadı")
		return
	}

	// Kendini tek admin iken düşürmeyi engelle — grup sahipsiz kalmasın.
	if targetDID == user.DID && req.Role == "member" {
		var adminCount int
		db.DB.QueryRow("SELECT COUNT(*) FROM conv_members WHERE conv_id = ? AND role = 'admin'", convID).Scan(&adminCount)
		if adminCount <= 1 {
			respond(w, 400, nil, "Tek yönetici kendini düşüremez, önce başka birini yönetici yapın")
			return
		}
	}

	db.DB.Exec("UPDATE conv_members SET role = ? WHERE conv_id = ? AND user_did = ?", req.Role, convID, targetDID)
	messaging.GlobalHub.SendTo(targetDID, "role_changed", map[string]interface{}{"conv_id": convID, "role": req.Role})
	respond(w, 200, map[string]string{"status": "updated"}, "")
}
