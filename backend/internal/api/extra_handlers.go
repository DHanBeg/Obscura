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
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/media"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/moderation"
	"obscura.network/core/internal/zk"
)

// hexDecode — encoding/hex.DecodeString wrapper (compile-time symbol gereksinimi için)
func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// ─── PATCH /v1/users/me ──────────────────────────────────────────────────────

// bidiControlChars — RTL/LTR override + isolate + zero-width karakterler.
// Mobile client (profile.tsx) bunları girişte zaten filtreliyordu ama backend
// hiç uygulamıyordu — doğrudan API çağrısıyla (mobile dışı) bir kullanıcı bu
// karakterlerle görünen adını spoof edebilirdi (bidi-override kimlik sahteciliği,
// bkz. "Trojan Source" sınıfı saldırılar). Mobile'daki filtreyle birebir aynı
// aralık, sunucu tarafında da uygulanıyor.
var bidiControlChars = regexp.MustCompile("[​-‏‪-‮⁦-⁩]")

// sanitizeDisplayName — bidi/zero-width kontrol karakterlerini siler, baştaki/
// sondaki boşluğu kırpar.
func sanitizeDisplayName(s string) string {
	return strings.TrimSpace(bidiControlChars.ReplaceAllString(s, ""))
}

type UpdateMeRequest struct {
	DisplayName     *string `json:"display_name,omitempty"` // nickname, zorunlu (boş gönderilemez)
	Username        string  `json:"username"`
	AvatarURL       string  `json:"avatar_url"`
	Bio             string  `json:"bio"`
	HideOnline      *int    `json:"hide_online,omitempty"`   // 1=gizli görün
	PhoneVisible    *int    `json:"phone_visible,omitempty"` // 1=telefon profilde görünür
	DilithiumPubKey string  `json:"dilithium_pub_key,omitempty"`
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
	if req.DisplayName == nil && req.Username == "" && req.AvatarURL == "" && req.Bio == "" && req.DilithiumPubKey == "" && req.HideOnline == nil && req.PhoneVisible == nil {
		respond(w, 400, nil, "Güncellenecek alan bulunamadı")
		return
	}

	// nickname (display_name) zorunlu alan — gönderildiyse boş/sadece-boşluk/
	// sadece-bidi-kontrol-karakteri OLAMAZ. Alan hiç gönderilmediyse (nil)
	// dokunulmaz, mevcut değer korunur.
	var sanitizedDisplayName string
	if req.DisplayName != nil {
		sanitizedDisplayName = sanitizeDisplayName(*req.DisplayName)
		if sanitizedDisplayName == "" {
			respond(w, 400, nil, "Görünen ad (nickname) boş olamaz")
			return
		}
		if len([]rune(sanitizedDisplayName)) > 50 {
			respond(w, 400, nil, "Görünen ad en fazla 50 karakter olabilir")
			return
		}
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

	if req.DisplayName != nil {
		setClauses = append(setClauses, "display_name = ?")
		args = append(args, sanitizedDisplayName)
	}
	if req.Username != "" {
		setClauses = append(setClauses, "username = ?")
		args = append(args, req.Username)
	}
	if req.AvatarURL != "" {
		setClauses = append(setClauses, "avatar_url = ?")
		args = append(args, req.AvatarURL)
	}
	if req.Bio != "" {
		setClauses = append(setClauses, "bio = ?")
		args = append(args, req.Bio)
	}
	if req.HideOnline != nil {
		setClauses = append(setClauses, "hide_online = ?")
		args = append(args, *req.HideOnline)
	}
	if req.PhoneVisible != nil {
		setClauses = append(setClauses, "phone_visible = ?")
		args = append(args, *req.PhoneVisible)
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
		SELECT id, phone, username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
		       COALESCE(bio,''), tier, credit_score, is_active, COALESCE(hide_online,0), COALESCE(phone_visible,0), is_banned, node_id
		FROM users WHERE id = ?`, user.ID,
	).Scan(&updated.ID, &updated.Phone, &updated.Username, &updated.DisplayName,
		&updated.DID, &updated.Odi, &updated.IdentityKey, &updated.AvatarURL, &updated.Bio,
		&updated.Tier, &updated.CreditScore, &updated.IsActive, &updated.HideOnline, &updated.PhoneVisible, &updated.IsBanned, &updated.NodeID)

	respond(w, 200, updated, "")
}

// ─── POST /v1/conversations ───────────────────────────────────────────────────

// GroupCreateAccessLevel is the spec Bölüm 5.2 access level required to
// create a group/channel/community conversation (Katman 2 — "sağlıklı
// kullanıcı"). Bronz (credit tier 1) kullanıcılar grup açamaz.
const GroupCreateAccessLevel = 2

type CreateConversationRequest struct {
	PeerDID      string   `json:"peer_did,omitempty"`
	IsGroup      bool     `json:"is_group,omitempty"`
	Type         string   `json:"type,omitempty"` // "direct" | "group" | "channel" | "community"
	Name         string   `json:"name,omitempty"`
	Description  string   `json:"description,omitempty"`
	IsPublic     bool     `json:"is_public,omitempty"`
	Members      []string `json:"members,omitempty"`      // DID listesi
	Participants []string `json:"participants,omitempty"` // alias for Members
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

	// Participants alias'ını Members'a birleştir
	if len(req.Participants) > 0 && len(req.Members) == 0 {
		req.Members = req.Participants
	}

	// Type alanından IsGroup çıkar
	convType := req.Type
	if convType == "" {
		convType = "direct"
	}
	if convType == "group" || convType == "channel" || convType == "community" {
		req.IsGroup = true
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

	// Grup / kanal / topluluk oluştur
	// Erişim kapısı: spec Bölüm 5.2 Katman 2 ("sağlıklı kullanıcı") gerekli —
	// Katman 1/Bronz (credit tier 1, puan <60) grup açamaz (spec 7.2: Bronz'da
	// "Grup yok"). models.TierToAccessLevel(tier) tier>=2'de zaten 2 döner.
	if models.TierToAccessLevel(user.Tier) < GroupCreateAccessLevel {
		respond(w, 403, nil, "Grup oluşturmak için en az Gümüş katman (kredi puanı 60+) gerekli")
		return
	}

	if req.Name == "" {
		respond(w, 400, nil, "Ad zorunlu")
		return
	}

	// Grup için minimum üye kontrolü; kanal/topluluk tek kişiyle oluşturulabilir
	if convType == "group" && len(req.Members) < 1 {
		respond(w, 400, nil, "Gruba en az 1 üye ekle")
		return
	}

	// Grup büyüklük limiti (spec Bölüm 7.2)
	totalMembers := len(req.Members) + 1
	maxAllowed := moderation.MaxGroupSize(user.CreditScore)
	if totalMembers > maxAllowed {
		respond(w, 403, nil, fmt.Sprintf(
			"Üye limitine ulaşıldı. (Mevcut limit: %d üye)", maxAllowed,
		))
		return
	}

	convID := uuid.New().String()
	isPublicInt := 0
	if req.IsPublic {
		isPublicInt = 1
	}
	_, err := db.DB.Exec(`
		INSERT INTO conversations (id, is_group, name, conv_type, description, is_public, created_at, updated_at)
		VALUES (?, 1, ?, ?, ?, ?, ?, ?)`,
		convID, req.Name, convType, req.Description, isPublicInt,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		// conv_type/description/is_public kolonları henüz yoksa eski şemaya fallback
		_, err = db.DB.Exec(`
			INSERT INTO conversations (id, is_group, name, created_at, updated_at)
			VALUES (?, 1, ?, ?, ?)`,
			convID, req.Name, now.Format(time.RFC3339), now.Format(time.RFC3339),
		)
		if err != nil {
			respond(w, 500, nil, "Konuşma oluşturulamadı")
			return
		}
	}

	// Üyeleri ekle (kurucu admin, diğerleri member)
	allMembers := append([]string{user.DID}, req.Members...)
	for _, did := range allMembers {
		memberRole := "member"
		if did == user.DID {
			memberRole = "admin"
		}
		db.DB.Exec(`
			INSERT INTO conv_members (conv_id, user_did, role, joined_at, unread_count)
			VALUES (?, ?, ?, ?, 0)
			ON CONFLICT DO NOTHING`,
			convID, did, memberRole, now.Format(time.RFC3339),
		)
	}

	// Üyelere WebSocket bildirimi gönder
	for _, did := range allMembers {
		messaging.GlobalHub.SendTo(did, "group_created", map[string]interface{}{
			"conv_id":    convID,
			"name":       req.Name,
			"conv_type":  convType,
			"created_by": user.DID,
			"members":    allMembers,
		})
	}

	respond(w, 201, map[string]interface{}{
		"conv_id":     convID,
		"name":        req.Name,
		"conv_type":   convType,
		"description": req.Description,
		"members":     len(allMembers),
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
	var fromDID, toID, convID, ownerHash string
	err := db.DB.QueryRow("SELECT from_did, to_did, conv_id, owner_hash FROM messages WHERE id = ? AND deleted_at IS NULL",
		msgID).Scan(&fromDID, &toID, &convID, &ownerHash)
	if err != nil {
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	// Sealed mesajda from_did boş (bkz. ADR-0016) — owner_hash ile doğrula.
	// Eski/zarfsız mesajda mevcut plaintext karşılaştırma AYNEN kalır.
	if fromDID == "" {
		if !ownerHashMatches(user.DID, msgID, ownerHash) {
			respond(w, 403, nil, "Bu mesajı silemezsiniz")
			return
		}
	} else if fromDID != user.DID {
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

	// MinIO'ya yükle.
	// DID formatı "did:obs:<hex>" — literal "did:obs:" tam 8 karakter olduğundan
	// user.DID[:8] her kullanıcı için aynı sabit dizeye çözülüyordu. Gerçek
	// kullanıcıya özgü hex ön ekini kullan (kısa DID'lerde panic'i önlemek için
	// uzunluğu koru).
	didSuffix := strings.TrimPrefix(user.DID, "did:obs:")
	if len(didSuffix) > 8 {
		didSuffix = didSuffix[:8]
	}
	objectKey := fmt.Sprintf("%s/%s/%s", mediaType, didSuffix, uuid.New().String())
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
	// self_destruct_at: SADECE "okununca" modunda (self_destruct_seconds=0 VE
	// henüz set edilmemiş) okuma anına set edilir — UTC zorunlu, expiry.go
	// scheduler'ı time.Now().UTC() ile karşılaştırıyor (bkz. HandleSendMessage
	// yorumu). Sabit süreli (>0) mesajlarda self_destruct_at gönderimde zaten
	// hesaplanmıştı; CASE koşulu (self_destruct_at IS NULL) onu EZMEMEYİ garantiler.
	nowUTC := now.UTC().Format(time.RFC3339)
	_, dbErr := db.DB.Exec(`
		UPDATE messages
		SET    status = 'read',
		       read_at = ?,
		       self_destruct_at = CASE
		           WHEN self_destruct_seconds = 0 AND self_destruct_at IS NULL THEN ?
		           ELSE self_destruct_at
		       END
		WHERE  id = ?`,
		now.Format(time.RFC3339), nowUTC, msgID,
	)
	if dbErr != nil {
		log.Printf("HandleMarkMessageRead DB hatası (msg=%s): %v", msgID, dbErr)
		respond(w, 500, nil, "Durum güncellenemedi")
		return
	}

	// Gönderene WebSocket read_receipt ilet — sealed mesajlarda from_did opak
	// ("", Adım 5) olduğu için sunucu kime göndereceğini bilemez; bu durumda
	// SendReadReceipt'i hiç çağırma (boş DID'e SendTo zaten no-op dönerdi
	// ama niyeti kod okuyucusuna açıkça belirtmek için burada durduruluyor).
	// Sealed mesajlarda gerçek okundu-bilgisi Adım 6b'nin yeni mekanizmasıyla
	// gider: alıcı, MsgReadReceipt tipinde AYRI bir sealed mesaj gönderir
	// (mobile/lib/e2e.ts sealReadReceipt) — sunucu orada da from_did'i
	// öğrenmez, sadece to_did'i (zaten normalde gördüğü şey) görür.
	if fromDID != "" {
		messaging.GlobalHub.SendReadReceipt(fromDID, msgID, user.DID)
	}

	respond(w, 200, map[string]interface{}{
		"msg_id":  msgID,
		"status":  "read",
		"read_at": now.Format(time.RFC3339),
	}, "")
}

// ─── POST /v1/groups/{id}/report ─────────────────────────────────────────────
//
// Kimliği doğrulanmış kullanıcı bir grubu raporlar.
// 24 saat içinde 3+ rapor → grup otomatik incelemeye alınır.
// Geçerli sebepler: spam | inappropriate | scam | other

func HandleReportGroup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkilendirme gerekli")
		return
	}

	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "Geçersiz grup kimliği")
		return
	}

	var body struct {
		Reason string `json:"reason"` // spam | inappropriate | scam | other
	}
	if err := decodeBody(r, &body); err != nil {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}

	validReasons := map[string]bool{
		"spam":          true,
		"inappropriate": true,
		"scam":          true,
		"other":         true,
	}
	if !validReasons[body.Reason] {
		respond(w, 400, nil, "Geçersiz rapor sebebi. Seçenekler: spam, inappropriate, scam, other")
		return
	}

	if err := moderation.ReportGroup(db.DB, groupID, user.DID, body.Reason); err != nil {
		log.Printf("HandleReportGroup hata (group=%s, reporter=%s): %v", groupID, user.DID, err)
		respond(w, 500, nil, "Rapor gönderilemedi")
		return
	}

	log.Printf("[MODERASYON] Grup raporu alındı — group=%s reporter=%s reason=%s", groupID, user.DID, body.Reason)
	respond(w, 200, map[string]string{"message": "Raporunuz alındı. İncelenecek."}, "")
}

// ─── GET /v1/conversations/discover?q=... ────────────────────────────────────
//
// Public konuşmaları (grup, kanal, topluluk) listele.
// İsteğe bağlı ?q= parametresi ile ada göre filtrele.

func HandleDiscoverConversations(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	q := r.URL.Query().Get("q")

	type DiscoverItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"conv_type"`
		MemberCount int    `json:"member_count"`
	}

	baseSQL := `
		SELECT c.id, c.name, c.conv_type,
		       (SELECT COUNT(*) FROM conv_members WHERE conv_id = c.id) AS member_count
		FROM conversations c
		WHERE c.is_public = 1`

	var queryStr string
	var args []interface{}
	if q == "" {
		queryStr = baseSQL + " ORDER BY member_count DESC LIMIT 50"
	} else {
		queryStr = baseSQL + " AND c.name LIKE ? ORDER BY member_count DESC LIMIT 50"
		args = []interface{}{"%" + q + "%"}
	}

	rows, err := db.DB.Query(queryStr, args...)
	if err != nil {
		respond(w, 500, nil, "DB hatası")
		return
	}
	defer rows.Close()

	items := []DiscoverItem{}
	for rows.Next() {
		var item DiscoverItem
		rows.Scan(&item.ID, &item.Name, &item.Type, &item.MemberCount)
		items = append(items, item)
	}
	respond(w, 200, items, "")
}

// ─── POST /v1/conversations/{id}/invite/create ───────────────────────────────

type CreateInviteRequest struct {
	Slug         string `json:"slug"`
	MaxUses      int    `json:"max_uses"`
	MaxMembers   int    `json:"max_members"`
	ExpiresHours int    `json:"expires_hours"`
}

func HandleCreateConvInvite(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	convId := mux.Vars(r)["id"]

	var req CreateInviteRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM conversations WHERE id=?", convId).Scan(&exists)
	if exists == 0 {
		respond(w, 404, nil, "Sohbet bulunamadı")
		return
	}

	id := uuid.New().String()
	token := uuid.New().String()

	var slug interface{} = nil
	if req.Slug != "" {
		if len(req.Slug) < 3 || len(req.Slug) > 50 {
			respond(w, 400, nil, "Slug 3-50 karakter olmalı")
			return
		}
		slug = req.Slug
	}

	var expiresAt interface{} = nil
	if req.ExpiresHours > 0 {
		expiresAt = time.Now().Add(time.Duration(req.ExpiresHours) * time.Hour).Format(time.RFC3339)
	}

	_, err := db.DB.Exec(
		`INSERT INTO invite_links (id, conv_id, token, slug, max_uses, used_count, max_members, expires_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		id, convId, token, slug, req.MaxUses, req.MaxMembers, expiresAt, user.DID, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		log.Printf("HandleCreateConvInvite davet linki oluşturulamadı (conv=%s): %v", convId, err)
		respond(w, 400, nil, "Davet linki oluşturulamadı. Slug zaten kullanımda olabilir.")
		return
	}

	identifier := token
	if req.Slug != "" {
		identifier = req.Slug
	}

	respond(w, 200, map[string]interface{}{
		"token":       token,
		"slug":        req.Slug,
		"invite_url":  "obscura://join/" + identifier,
		"max_uses":    req.MaxUses,
		"max_members": req.MaxMembers,
	}, "")
}

// ─── POST /v1/conversations/join ─────────────────────────────────────────────

func HandleJoinViaInvite(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req struct {
		Identifier string `json:"identifier"`
	}
	if err := decodeBody(r, &req); err != nil || req.Identifier == "" {
		respond(w, 400, nil, "identifier gerekli")
		return
	}

	var (
		id, convId, token              string
		maxUses, usedCount, maxMembers int
		expiresAt                      sql.NullString
	)
	err := db.DB.QueryRow(
		`SELECT id, conv_id, token, max_uses, used_count, max_members, expires_at
		FROM invite_links WHERE token=? OR slug=?`,
		req.Identifier, req.Identifier,
	).Scan(&id, &convId, &token, &maxUses, &usedCount, &maxMembers, &expiresAt)
	if err != nil {
		respond(w, 404, nil, "Davet linki bulunamadı")
		return
	}

	if expiresAt.Valid {
		exp, _ := time.Parse(time.RFC3339, expiresAt.String)
		if time.Now().After(exp) {
			respond(w, 410, nil, "Davet linki süresi dolmuş")
			return
		}
	}
	if maxUses > 0 && usedCount >= maxUses {
		respond(w, 410, nil, "Davet linki maksimum kullanım sayısına ulaştı")
		return
	}
	if maxMembers > 0 {
		var memberCount int
		db.DB.QueryRow("SELECT COUNT(*) FROM conv_members WHERE conv_id=?", convId).Scan(&memberCount)
		if memberCount >= maxMembers {
			respond(w, 410, nil, "Sohbet dolu")
			return
		}
	}

	db.DB.Exec(
		"INSERT INTO conv_members (conv_id, user_did, joined_at, unread_count) VALUES (?, ?, ?, 0) ON CONFLICT DO NOTHING",
		convId, user.DID, time.Now().Format(time.RFC3339),
	)
	db.DB.Exec("UPDATE invite_links SET used_count=used_count+1 WHERE id=?", id)

	var convName string
	db.DB.QueryRow("SELECT name FROM conversations WHERE id=?", convId).Scan(&convName)
	respond(w, 200, map[string]string{"conv_id": convId, "conv_name": convName}, "")
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

	var fromDID, toDID, status, ownerHash string

	// deliveredAt ve readAt nullable
	var deliveredAtSQL, readAtSQL interface{}

	err := db.DB.QueryRow(
		"SELECT from_did, to_did, status, delivered_at, read_at, owner_hash FROM messages WHERE id = ? AND deleted_at IS NULL",
		msgID,
	).Scan(&fromDID, &toDID, &status, &deliveredAtSQL, &readAtSQL, &ownerHash)
	if err != nil {
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	// Sadece gönderen veya alıcı sorgulayabilir. Sealed mesajda from_did boş
	// (bkz. ADR-0016) — göndereni owner_hash ile doğrula. Eski/zarfsız
	// mesajda mevcut plaintext karşılaştırma AYNEN kalır.
	var isSender bool
	if fromDID == "" {
		isSender = ownerHashMatches(user.DID, msgID, ownerHash)
	} else {
		isSender = fromDID == user.DID
	}
	if !isSender && user.DID != toDID {
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
