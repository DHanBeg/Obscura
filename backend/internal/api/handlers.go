package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/gossip"
	"obscura.network/core/internal/identity"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/models"
	"obscura.network/core/internal/moderation"
	"obscura.network/core/internal/p2p"
	"obscura.network/core/internal/pqcrypto"
	"obscura.network/core/internal/push"
	"obscura.network/core/internal/zk"
)

// ─── YARDIMCI ─────────────────────────────────────────────────────────────────

func respond(w http.ResponseWriter, code int, data interface{}, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(models.APIResponse{
		Success: errMsg == "",
		Data:    data,
		Error:   errMsg,
		Code:    code,
	})
}

func decodeBody(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func getUser(r *http.Request) *models.User {
	user, _ := r.Context().Value("user").(*models.User)
	return user
}

// ─── AUTH ─────────────────────────────────────────────────────────────────────

// otpRateLimiter — IP başına dakikada 5 OTP isteğiyle sınırlandırır
var otpRateLimiter = struct {
	sync.Mutex
	hits map[string][]time.Time
}{hits: make(map[string][]time.Time)}

// clientIPFromRequest — proxy arkasında (Railway) gerçek istemci IP'sini bulur.
// X-Forwarded-For / X-Real-IP yoksa RemoteAddr'a düşer. Rate limiting ve
// loglama aynı IP değerini kullanmalı, yoksa limiter proxy'nin sabit IP'sine
// göre çalışıp ya işe yaramaz ya da tüm kullanıcıları tek limite sokar.
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	return ip
}

func checkOTPRateLimit(ip string) bool {
	otpRateLimiter.Lock()
	defer otpRateLimiter.Unlock()
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	prev := otpRateLimiter.hits[ip]
	var recent []time.Time
	for _, t := range prev {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 5 {
		otpRateLimiter.hits[ip] = recent
		return false
	}
	otpRateLimiter.hits[ip] = append(recent, now)
	return true
}

// POST /v1/auth/request-otp
func HandleRequestOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req models.LoginRequest
	if err := decodeBody(r, &req); err != nil || req.Phone == "" {
		respond(w, 400, nil, "Geçerli bir telefon numarası giriniz")
		return
	}

	// E.164 format kontrolü: + ile başlamalı, min 10 karakter
	if !strings.HasPrefix(req.Phone, "+") || len(req.Phone) < 10 || len(req.Phone) > 16 {
		respond(w, 400, nil, "Telefon numarası uluslararası formatta olmalı (+XXXXXXXXXXX)")
		return
	}

	clientIP := clientIPFromRequest(r)

	// IP-based rate limiting: dakikada en fazla 5 OTP isteği
	if !checkOTPRateLimit(clientIP) {
		respond(w, 429, nil, "Çok fazla istek. 1 dakika bekleyin.")
		return
	}

	code, err := auth.GenerateOTP(req.Phone)
	if err != nil {
		if err == auth.ErrOTPLockedOut {
			respond(w, 429, nil, "Çok fazla hatalı deneme yapıldı. 15 dakika sonra tekrar deneyin.")
			return
		}
		log.Printf("GenerateOTP hatası (phone=%s): %v", req.Phone, err)
		respond(w, 500, nil, "OTP oluşturulamadı")
		return
	}

	// Kod SADECE loglara yazılır — SMS_PROVIDER=log modunda operatör railway
	// logs üzerinden takip eder. API response'unda ASLA dönmez: request-otp
	// public/unauthenticated bir endpoint, kodu response'a koymak herhangi
	// birinin herhangi bir telefon numarasını anında ele geçirmesi demektir.
	log.Printf("🔐 [OTP-MONITOR] IP:%s | Tel:%s | Kod:%s", clientIP, req.Phone, code)

	respond(w, 200, map[string]interface{}{"message": "OTP gönderildi"}, "")
}

// POST /v1/auth/verify-otp
func HandleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req models.VerifyOTPRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}

	// Önce doğrula (tüketme) — yeni kullanıcı akışında OTP yeniden kullanılabilir olmalı
	if err := auth.ValidateOTP(req.Phone, req.OTP); err != nil {
		if err == auth.ErrOTPLockedOut {
			respond(w, 429, nil, err.Error())
			return
		}
		respond(w, 401, nil, err.Error())
		return
	}

	// Kullanıcı var mı?
	var user models.User
	var zkIDVerified int
	err := db.DB.QueryRow(`
		SELECT id, phone, username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
		       tier, credit_score, is_active, is_banned, node_id,
		       created_at, updated_at, last_seen_at,
		       COALESCE(zk_id_verified, 0) AS zk_id_verified
		FROM users WHERE phone = ?`, req.Phone,
	).Scan(&user.ID, &user.Phone, &user.Username, &user.DisplayName, &user.DID, &user.Odi,
		&user.IdentityKey, &user.AvatarURL, &user.Tier, &user.CreditScore,
		&user.IsActive, &user.IsBanned, &user.NodeID,
		new(string), new(string), new(string),
		&zkIDVerified)

	if err == sql.ErrNoRows {
		// Yeni kayıt
		if req.Username == "" || req.IdentityKey == "" {
			// OTP tüketilmedi — kullanıcı username adımından sonra tekrar gönderecek
			respond(w, 200, map[string]interface{}{
				"is_new":  true,
				"status":  "new_user",
				"message": "Kullanıcı adı ve kimlik anahtarı gerekli",
			}, "")
			return
		}

		// ZK-ID proof varsa kayıt öncesi doğrula (spec Bölüm 5.2)
		// Doğrulama başarısız → 400; gönderilmemişse sessizce geç (backward compat)
		var zkVerifiedFlag int
		var zkProofToSave, zkPublicToSave string
		if req.ZKIDProof != "" {
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
			if verErr := zk.VerifyGroth16(zk.CircuitIdentityProof, proofBytes, pubSignals); verErr != nil {
				log.Printf("ZK-ID doğrulama başarısız (yeni kullanıcı, phone=%s): %v", req.Phone, verErr)
				respond(w, 400, nil, "ZK kimlik kanıtı geçersiz")
				return
			}
			zkVerifiedFlag = 1
			zkProofToSave = req.ZKIDProof
			zkPublicToSave = req.ZKIDPublic
		}

		// DID oluştur
		did := auth.GenerateDID(req.IdentityKey)
		if req.DID != "" {
			did = req.DID
		}

		// Kullanıcı oluştur
		now := time.Now()
		newUser := models.User{
			ID:          uuid.New().String(),
			Phone:       req.Phone,
			Username:    req.Username,
			DisplayName: req.Username,
			DID:         did,
			Odi:         identity.DeriveODI(did),
			IdentityKey: req.IdentityKey,
			Tier:        1,
			CreditScore: credit.InitialScore(),
			IsActive:    true,
			CreatedAt:   now,
			UpdatedAt:   now,
			LastSeenAt:  now,
		}
		newUser.Tier = models.ScoreToTier(newUser.CreditScore)

		_, err = db.DB.Exec(`
			INSERT INTO users (id, phone, username, display_name, did, odi, identity_key, avatar_url,
			                   tier, credit_score, is_active, is_banned, node_id,
			                   created_at, updated_at, last_seen_at,
			                   zk_id_proof_b64, zk_id_public_params, zk_id_verified)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?, 1, 0, '',
			        ?, ?, ?,
			        ?, ?, ?)`,
			newUser.ID, newUser.Phone, newUser.Username, newUser.DisplayName,
			newUser.DID, newUser.Odi, newUser.IdentityKey, newUser.Tier, newUser.CreditScore,
			now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
			nullableString(zkProofToSave), nullableString(zkPublicToSave), zkVerifiedFlag,
		)
		if err != nil {
			log.Printf("Kullanıcı oluşturulamadı (phone=%s): %v", req.Phone, err)
			respond(w, 500, nil, "İşlem başarısız")
			return
		}

		// ZK-ID proof ayrıca zk_proofs tablosuna da kaydedilir (spec Ek B — ZK proof index).
		// Bu sayede /v1/zk/verify ve kredi sistemi aynı kaynaktan sorgulayabilir.
		if zkVerifiedFlag == 1 && zkProofToSave != "" {
			zkProofID := uuid.New().String()
			db.DB.Exec(`
				INSERT INTO zk_proofs (id, user_did, circuit_id, proof_data, public_inputs, verified, created_at, expires_at)
				VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
				zkProofID, newUser.DID, string(zk.CircuitIdentityProof),
				zkProofToSave, nullableString(zkPublicToSave),
				now.Format(time.RFC3339),
				now.Add(365*24*time.Hour).Format(time.RFC3339), // ZK-ID: 1 yıl geçerli
			)
		}

		zkIDVerified = zkVerifiedFlag
		user = newUser
	} else if err != nil {
		respond(w, 500, nil, "Veritabanı hatası")
		return
	}

	// Banlı mı?
	if user.IsBanned {
		respond(w, 403, nil, "Hesabınız askıya alınmıştır")
		return
	}

	// Son görülme güncelle
	db.DB.Exec("UPDATE users SET last_seen_at = ? WHERE id = ?",
		time.Now().Format(time.RFC3339), user.ID)

	// Günlük giriş kredisi
	go credit.TrackDailyLogin(user.DID)

	// OTP tüket — başarılı giriş/kayıt onaylandı
	auth.ConsumeOTP(req.Phone)

	// JWT üret
	token, err := auth.GenerateToken(&user)
	if err != nil {
		respond(w, 500, nil, "Token oluşturulamadı")
		return
	}

	respond(w, 200, map[string]interface{}{
		"token":          token,
		"user":           user,
		"zk_id_verified": zkIDVerified == 1,
		"zk_id_required": zkIDVerified == 0, // bilgilendirme — zorlama değil
	}, "")
}

// ─── ZK-ID YARDIMCILARI ───────────────────────────────────────────────────────

// decodeZKIDFields — base64 proof + public params alanlarını decode eder.
// Alanlar base64 değilse düz JSON string olarak da kabul edilir (development kolaylığı).
func decodeZKIDFields(proofB64, publicB64 string) (proofJSON, publicJSON []byte, err error) {
	proofJSON, err = base64OrRaw(proofB64)
	if err != nil {
		return nil, nil, fmt.Errorf("zk_id_proof decode: %w", err)
	}
	publicJSON, err = base64OrRaw(publicB64)
	if err != nil {
		return nil, nil, fmt.Errorf("zk_id_public decode: %w", err)
	}
	return proofJSON, publicJSON, nil
}

// base64OrRaw — önce base64 decode dener; başarısız olursa raw bytes döndürür.
func base64OrRaw(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("alan boş")
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Raw JSON string olabilir
		b = []byte(s)
	}
	return b, nil
}

// parsePublicSignals — JSON byte dizisini []string'e çevirir.
// Hem ["123","456"] hem de [123,456] formatını destekler.
func parsePublicSignals(data []byte) ([]string, error) {
	// Önce string dizisi olarak dene
	var signals []string
	if err := json.Unmarshal(data, &signals); err == nil {
		return signals, nil
	}
	// Sayısal dizi olarak dene (snarkjs bazen number üretir)
	var nums []interface{}
	if err := json.Unmarshal(data, &nums); err != nil {
		return nil, fmt.Errorf("public signals JSON: %w", err)
	}
	out := make([]string, len(nums))
	for i, v := range nums {
		switch n := v.(type) {
		case string:
			out[i] = n
		case float64:
			out[i] = fmt.Sprintf("%.0f", n)
		default:
			return nil, fmt.Errorf("public signals[%d] beklenmedik tip: %T", i, v)
		}
	}
	return out, nil
}

// nullableString — boş string'i SQL NULL'a dönüştürür.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// shortDIDStr — DID'in ilk 12 karakterini döndürür (log için güvenli kırpma).
// Panik önler: DID 12 karakterden kısaysa tamamını döner.
func shortDIDStr(did string) string {
	if len(did) <= 12 {
		return did
	}
	return did[:12]
}

// ─── KULLANICI ─────────────────────────────────────────────────────────────────

// GET /v1/users/me
func HandleGetMe(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	respond(w, 200, user, "")
}

// publicUserResponse — profil görüntüleme endpoint'leri (HandleGetUser,
// HandleGetUserByODI) için sarmalayıcı. phone alanı sadece hedef kullanıcı
// phone_visible=1 seçtiyse doldurulur, yoksa omitempty ile hiç dönmez.
type publicUserResponse struct {
	models.User
	Phone string `json:"phone,omitempty"`
}

func respondPublicUser(w http.ResponseWriter, user models.User, phone string, phoneVisible bool) {
	resp := publicUserResponse{User: user}
	if phoneVisible {
		resp.Phone = phone
	}
	respond(w, 200, resp, "")
}

// GET /v1/users/{did}
func HandleGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetDID := vars["did"]

	var user models.User
	var phone string
	var phoneVisible bool
	err := db.DB.QueryRow(`
		SELECT id, username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
		       tier, credit_score, is_active, created_at, COALESCE(phone,''), COALESCE(phone_visible,0)
		FROM users WHERE did = ? AND is_active = 1`, targetDID,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.DID, &user.Odi,
		&user.IdentityKey, &user.AvatarURL, &user.Tier, &user.CreditScore,
		&user.IsActive, new(string), &phone, &phoneVisible)

	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Kullanıcı bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Veritabanı hatası")
		return
	}

	respondPublicUser(w, user, phone, phoneVisible)
}

// GET /v1/users/by-odi/{odi}
//
// ODI kullanıcıya gösterilen kimlik kodudur (bkz. internal/identity/odi.go),
// DID'in tek yönlü hash'inden türetilir. Bu endpoint ODI ile kullanıcı
// bulma/ekleme akışı için DID çözümü yapar — ODI kendisi ters çevrilemez,
// sadece bu indexli DB araması üzerinden eşlenir.
func HandleGetUserByODI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetODI := vars["odi"]

	var user models.User
	var phone string
	var phoneVisible bool
	err := db.DB.QueryRow(`
		SELECT id, username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
		       tier, credit_score, is_active, created_at, COALESCE(phone,''), COALESCE(phone_visible,0)
		FROM users WHERE odi = ? AND is_active = 1`, targetODI,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.DID, &user.Odi,
		&user.IdentityKey, &user.AvatarURL, &user.Tier, &user.CreditScore,
		&user.IsActive, new(string), &phone, &phoneVisible)

	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Kullanıcı bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Veritabanı hatası")
		return
	}

	respondPublicUser(w, user, phone, phoneVisible)
}

// GET /v1/users/search?q=username
func HandleSearchUser(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		respond(w, 400, nil, "En az 2 karakter giriniz")
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, username, display_name, did, COALESCE(odi,''), avatar_url, tier, credit_score
		FROM users
		WHERE (username LIKE ? OR display_name LIKE ?) AND is_active = 1
		LIMIT 20`, "%"+q+"%", "%"+q+"%",
	)
	if err != nil {
		respond(w, 500, nil, "Arama hatası")
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.DID, &u.Odi, &u.AvatarURL, &u.Tier, &u.CreditScore); err != nil {
			log.Printf("HandleSearchUser scan hatası: %v", err)
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		respond(w, 500, nil, "Arama hatası")
		return
	}

	respond(w, 200, map[string]interface{}{"users": users}, "")
}

// ─── MESAJLAŞMA ───────────────────────────────────────────────────────────────

// GET /v1/conversations
func HandleGetConversations(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	// Tek sorgu ile konuşmalar + peer bilgisi (nested query deadlock önleme)
	rows, err := db.DB.Query(`
		SELECT c.id, c.is_group, c.name, c.avatar_url,
		       COALESCE(c.conv_type, 'direct'), COALESCE(c.description, ''), COALESCE(c.is_public, 0),
		       c.mls_group_id,
		       c.last_msg_text, c.last_msg_at,
		       cm.unread_count, cm.role,
		       COALESCE(peer.did, '')        AS peer_did,
		       COALESCE(peer.display_name, '') AS peer_name,
		       COALESCE(peer.tier, 0)        AS peer_tier
		FROM conversations c
		JOIN conv_members cm ON c.id = cm.conv_id AND cm.user_did = ?
		LEFT JOIN conv_members cm2 ON c.id = cm2.conv_id AND cm2.user_did != ? AND c.is_group = 0
		LEFT JOIN users peer ON cm2.user_did = peer.did
		ORDER BY c.last_msg_at DESC
		LIMIT 50`, user.DID, user.DID,
	)
	if err != nil {
		respond(w, 500, nil, "Konuşmalar alınamadı")
		return
	}
	defer rows.Close()

	type ConvWithPeer struct {
		models.Conversation
		MyRole   string `json:"my_role,omitempty"`
		PeerDID  string `json:"peer_did,omitempty"`
		PeerName string `json:"peer_name,omitempty"`
		PeerTier int    `json:"peer_tier,omitempty"`
	}

	var convs []ConvWithPeer
	for rows.Next() {
		var c ConvWithPeer
		var lastMsgAt, mlsGroupID sql.NullString
		if err := rows.Scan(&c.ID, &c.IsGroup, &c.Name, &c.AvatarURL,
			&c.ConvType, &c.Description, &c.IsPublic, &mlsGroupID,
			&c.LastMsgText, &lastMsgAt, &c.UnreadCount, &c.MyRole,
			&c.PeerDID, &c.PeerName, &c.PeerTier); err != nil {
			log.Printf("HandleGetConversations scan hatası: %v", err)
			continue
		}
		if mlsGroupID.Valid {
			c.MLSGroupID = &mlsGroupID.String
		}

		if lastMsgAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastMsgAt.String)
			c.LastMsgAt = &t
		}
		if !c.IsGroup && c.Name == "" {
			c.Name = c.PeerName
		}

		convs = append(convs, c)
	}
	if err := rows.Err(); err != nil {
		respond(w, 500, nil, "Konuşmalar alınamadı")
		return
	}

	if convs == nil {
		convs = []ConvWithPeer{}
	}

	respond(w, 200, convs, "")
}

// GET /v1/conversations/{id}/messages
func HandleGetMessages(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	vars := mux.Vars(r)
	convID := vars["id"]

	// Üye mi?
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM conv_members WHERE conv_id = ? AND user_did = ?",
		convID, user.DID).Scan(&count)
	if count == 0 {
		respond(w, 403, nil, "Bu konuşmaya erişiminiz yok")
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, conv_id, from_did, to_did, type, ciphertext, media_url,
		       status, is_group, reply_to_id, sent_at, delivered_at, read_at,
		       COALESCE(dilithium_sig, '') AS dilithium_sig,
		       self_destruct_seconds, self_destruct_at
		FROM messages
		WHERE conv_id = ? AND deleted_at IS NULL
		ORDER BY sent_at ASC
		LIMIT 100`, convID,
	)
	if err != nil {
		respond(w, 500, nil, "Mesajlar alınamadı")
		return
	}
	defer rows.Close()

	var msgs []models.Message
	for rows.Next() {
		var m models.Message
		var deliveredAt, readAt, selfDestructAt sql.NullString
		var selfDestructSeconds sql.NullInt64
		if err := rows.Scan(&m.ID, &m.ConvID, &m.FromDID, &m.ToDID, &m.Type,
			&m.Ciphertext, &m.MediaURL, &m.Status, &m.IsGroup,
			&m.ReplyToID, new(string), &deliveredAt, &readAt, &m.DilithiumSig,
			&selfDestructSeconds, &selfDestructAt); err != nil {
			log.Printf("HandleGetMessages scan hatası: %v", err)
			continue
		}

		if deliveredAt.Valid {
			t, _ := time.Parse(time.RFC3339, deliveredAt.String)
			m.DeliveredAt = &t
		}
		if readAt.Valid {
			t, _ := time.Parse(time.RFC3339, readAt.String)
			m.ReadAt = &t
		}
		if selfDestructSeconds.Valid {
			s := int(selfDestructSeconds.Int64)
			m.SelfDestructSeconds = &s
		}
		if selfDestructAt.Valid {
			t, _ := time.Parse(time.RFC3339, selfDestructAt.String)
			m.SelfDestructAt = &t
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		respond(w, 500, nil, "Mesajlar alınamadı")
		return
	}

	// Okunmamışları temizle
	db.DB.Exec("UPDATE conv_members SET unread_count = 0 WHERE conv_id = ? AND user_did = ?",
		convID, user.DID)

	if msgs == nil {
		msgs = []models.Message{}
	}

	respond(w, 200, msgs, "")
}

// POST /v1/messages
func HandleSendMessage(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	var req models.SendMessageRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz mesaj gövdesi")
		return
	}
	if req.ToID == "" {
		respond(w, 400, nil, "to_id zorunlu")
		return
	}
	// Normalize: prefer "ciphertext" but fall back to legacy "content" field.
	if req.EffectiveCiphertext() == "" {
		respond(w, 400, nil, "ciphertext zorunlu (E2EE: plaintext kabul edilmez)")
		return
	}
	// Write canonical values back so the rest of the handler uses a single field.
	req.Ciphertext = req.EffectiveCiphertext()

	// Commit 0 (mobil grup transport planı, ADR-0019 öncesi): grup mesajları
	// artık encryption_type:"mls" etiketi olmadan kabul edilmiyor. Öncesi:
	// sealedSenderRequiredForRequest grup mesajlarını HER politika
	// kontrolünden muaf tutuyordu (bkz. sealed_policy.go) — ciphertext
	// alanına düz metin dahil herhangi bir değer yazılıp is_group:true ile
	// gönderilebiliyordu (bkz. bridge.go sendGroupMessage, Commit 0b).
	//
	// NOT (honesty contract, kriptografik kanıt DEĞİL): encryption_type
	// client'ın kendi beyan ettiği bir alan (models.go:219, decodeBody ile
	// doğrudan client body'sinden okunuyor) — bu gate ciphertext'in GERÇEKTEN
	// MLS wire-format'ında olduğunu doğrulamıyor, yalnızca etiketi zorunlu
	// kılıyor. L2 gerçek MLS entegrasyonuna kadar E1 bu yüzden KAPANMIŞ
	// SAYILMAZ — yapısal doğrulama L2/backend'in görevi.
	if req.IsGroup && req.EffectiveEncryptionType() != "mls" {
		respond(w, 400, nil, "Grup mesajlaşma MLS (encryption_type:\"mls\") olmadan henüz desteklenmiyor")
		return
	}

	// Adım 10: kademeli geçiş anahtarı (bkz. sealed_policy.go) — VARSAYILAN
	// KAPALI. Açıldığında grup DIŞI mesajlarda zarfsız (eski) format
	// reddedilir; grup mesajları her zaman muaf (sealed tek-alıcı, gruba
	// mimari olarak uygulanamaz).
	if sealedSenderRequiredForRequest(&req) && !req.IsSealedSender() {
		respond(w, 400, nil, "Sunucu artık yalnızca sealed-sender mesajlarını kabul ediyor — uygulamanızı güncelleyin")
		return
	}

	// ── Self-destruct (ExpiresAt/30 gün genel TTL'den AYRI) ──────────────────
	// nil=kapalı, 0="okununca" (self_destruct_at okuma anında set edilir —
	// bkz. HandleMarkMessageRead), >0=gönderimden N sn sonra (mutlak zaman
	// burada hesaplanır).
	var selfDestructAt *time.Time
	if req.SelfDestructSeconds != nil {
		if !models.SelfDestructAllowedSeconds[*req.SelfDestructSeconds] {
			respond(w, 400, nil, "Geçersiz self_destruct_seconds")
			return
		}
		if *req.SelfDestructSeconds > 0 {
			// UTC ZORUNLU: expiry.go scheduler'ı self_destruct_at'i time.Now().UTC()
			// ile karşılaştırıyor. Yerel TZ (örn. +03:00) ile yazılırsa RFC3339
			// string karşılaştırması offset farkında yanlış sıralanabilir — kısa
			// süreli (10sn) self-destruct'ta bu saatlerce gecikme/erken tetikleme demek.
			t := time.Now().UTC().Add(time.Duration(*req.SelfDestructSeconds) * time.Second)
			selfDestructAt = &t
		}
	}

	// ── Dilithium3 otomatik imzalama ─────────────────────────────────────────
	// Kullanıcının private key'i DB'de kayıtlıysa ciphertext'i otomatik imzala.
	// Client dilithium_sig göndermişse otomatik imza atlanır (client imzası önceliklidir).
	if req.DilithiumSig == "" {
		var storedPriv string
		db.DB.QueryRow("SELECT COALESCE(dilithium_priv_key,'') FROM users WHERE did = ?", user.DID).
			Scan(&storedPriv)
		if storedPriv != "" {
			msgBytes, hexErr := hex.DecodeString(req.Ciphertext)
			if hexErr != nil {
				msgBytes = []byte(req.Ciphertext)
			}
			msgHex := hex.EncodeToString(msgBytes)
			if sig, sigErr := pqcrypto.DilithiumSign(storedPriv, msgHex); sigErr == nil {
				req.DilithiumSig = sig.Signature
			}
		}
	}

	// ── Dilithium3 imza doğrulaması (isteğe bağlı — backward compatible) ──────
	// Gönderenin dilithium_pub_key'i varsa ve istek dilithium_sig içeriyorsa doğrula.
	// İmza yoksa veya public key kayıtlı değilse sessizce geç.
	if req.DilithiumSig != "" {
		var senderPubKeyHex string
		db.DB.QueryRow("SELECT COALESCE(dilithium_pub_key,'') FROM users WHERE did = ?", user.DID).
			Scan(&senderPubKeyHex)

		if senderPubKeyHex != "" {
			// İmzalanan mesaj: ciphertext'in ham byte'ları (hex decode)
			msgBytes, hexErr := hex.DecodeString(req.Ciphertext)
			if hexErr != nil {
				// Ciphertext base64 veya başka format olabilir; raw bytes kullan
				msgBytes = []byte(req.Ciphertext)
			}
			msgHex := hex.EncodeToString(msgBytes)

			ok, verErr := pqcrypto.DilithiumVerify(senderPubKeyHex, msgHex, req.DilithiumSig)
			if verErr != nil {
				log.Printf("dilithium doğrulama hatası (did=%s): %v", user.DID, verErr)
				respond(w, 400, nil, "Dilithium imza doğrulama hatası")
				return
			}
			if !ok {
				respond(w, 400, nil, "Geçersiz Dilithium3 imzası")
				return
			}
		}
		// public key kayıtlı değilse imzayı yine de saklıyoruz (anahtar sonradan yüklenebilir)
	}

	// Konuşma bul veya oluştur
	convID, err := findOrCreateConversation(user.DID, req.ToID, req.IsGroup)
	if err != nil {
		respond(w, 500, nil, "Konuşma oluşturulamadı")
		return
	}

	// Grup/kanal/topluluk konuşmalarında findOrCreateConversation, isGroup=true
	// ise req.ToID'yi doğrudan convID kabul eder (bkz. yukarısı, satır ~1298) —
	// üyelik kontrolü yapmaz. Bu yüzden gate BURADA: üye olmayan bir kullanıcı
	// bildiği herhangi bir grup conv_id'sine mesaj yazamasın (önceki davranış:
	// hiçbir kontrol yoktu, herkes yazabiliyordu).
	if req.IsGroup {
		if isMember, _ := isConvMember(convID, user.DID); !isMember {
			respond(w, 403, nil, "Bu konuşmanın üyesi değilsiniz")
			return
		}
	}

	// Mesaj kaydet
	// UTC ZORUNLU: expiry.go scheduler'ı expires_at'i time.Now().UTC() ile
	// karşılaştırıyor (SQLite TEXT `<`, gerçek datetime parse değil). Yerel
	// TZ ile yazılırsa (örn. +03:00) bu karşılaştırma sunucu TZ'si UTC
	// olmayan ortamlarda saatlerce yanlış sıralanabilir — self_destruct_at
	// için aynı gerekçeyle biraz yukarıda zaten .UTC() kullanılıyor.
	now := time.Now().UTC()
	msgID := uuid.New().String()
	expires := now.Add(30 * 24 * time.Hour)

	var selfDestructAtStr interface{}
	if selfDestructAt != nil {
		selfDestructAtStr = selfDestructAt.Format(time.RFC3339)
	}

	// Madde 15 (sealed-sender): ciphertext bir sealed-sender zarfıysa from_did
	// PLAINTEXT KALICI SAKLANMAZ (bkz. ADR-0016 — sunucu göndereni istek anında
	// JWT'den zaten biliyor, sealed-sender bunu sunucudan gizlemez, yalnızca
	// DB'ye yazılmasını ve alıcıya/3. taraflara iletilmesini engeller). Boş
	// string (SQL NULL DEĞİL) tercih edildi: from_did NOT NULL kısıtı bozulmaz
	// ve moderation/scanner/recall gibi mevcut `Scan(&fromDID)` (plain string,
	// sql.NullString değil) çağrıları NULL'da hata FIRLATMAZ.
	//
	// owner_hash = HMAC-SHA256(pepper, DID+":"+msgID) — Adım 7: sealed
	// mesajlarda from_did boş olduğundan yetki kontrolleri (sil/geri al/durum
	// sorgusu) plaintext karşılaştırma yapamaz; bu alan karşılaştırılabilir
	// ama tersine çevrilemez bir yetkilendirme kanıtı sağlar (owner_hash.go,
	// ADR-0016 "residual risk").
	storedFromDID := user.DID
	ownerHash := ""
	if req.IsSealedSender() {
		storedFromDID = ""
		ownerHash = computeOwnerHash(user.DID, msgID)
	}

	_, err = db.DB.Exec(`
		INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext, media_url,
		                      status, is_group, reply_to_id, sent_at, expires_at, dilithium_sig,
		                      is_encrypted, encryption_type, self_destruct_seconds, self_destruct_at,
		                      owner_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sent', ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
		msgID, convID, storedFromDID, req.ToID, req.Type, req.Ciphertext,
		req.MediaURL, req.IsGroup, req.ReplyToID,
		now.Format(time.RFC3339), expires.Format(time.RFC3339),
		req.DilithiumSig, req.EffectiveEncryptionType(),
		req.SelfDestructSeconds, selfDestructAtStr,
		ownerHash,
	)
	if err != nil {
		respond(w, 500, nil, "Mesaj kaydedilemedi")
		return
	}

	// Konuşmayı güncelle — read_receipt bir meta-sinyal, "gerçek mesaj" değil
	// (Adım 6b): önizlemeyi/okunmamış sayacını EZMEMELİ.
	if req.Type != models.MsgReadReceipt {
		previewText := "[Şifreli mesaj]"
		if req.Type == models.MsgText {
			previewText = "🔒 Şifreli"
		}
		db.DB.Exec(`
			UPDATE conversations SET
				last_msg_id = ?, last_msg_text = ?, last_msg_at = ?, updated_at = ?
			WHERE id = ?`,
			msgID, previewText, now.Format(time.RFC3339), now.Format(time.RFC3339), convID,
		)

		// Alıcının okunmamış sayısını artır
		db.DB.Exec(`
			UPDATE conv_members SET unread_count = unread_count + 1
			WHERE conv_id = ? AND user_did != ?`, convID, user.DID)
	}

	// Gerçek zamanlı iletim (WebSocket)
	msgPayload := map[string]interface{}{
		"id":                    msgID,
		"conv_id":               convID,
		"ciphertext":            req.Ciphertext,
		"type":                  req.Type,
		"sent_at":               now.Unix(),
		"dilithium_sig":         req.DilithiumSig, // boşsa JSON'da "" olarak görünür
		"self_destruct_seconds": req.SelfDestructSeconds,
		"self_destruct_at":      selfDestructAtStr,
	}
	// Madde 15 (sealed-sender): sealed mesajlarda WS payload'ına from_did HİÇ
	// eklenmez (boş string bile değil — alan tamamen yok, "sunucu bilmiyor"
	// dürüstçe yansır). Client zaten gönderen kimliğini kendi Unseal()'ından
	// çıkarıyor (mobile/lib/e2e.ts receiveMessage), server'ın buradaki
	// beyanına ihtiyaç duymuyor. NOT: hub.go'nun SendTo/broadcast içindeki
	// AYRI bir from_did zorlaması (Adım 6 kapsamı) bu alanı hâlâ ekleyebilir —
	// bu satır sadece BU çağrı sitesindeki forcing'i kaldırıyor.
	if !req.IsSealedSender() {
		msgPayload["from_did"] = user.DID
	}

	// Alıcı listesi: 1-1'de tek eleman (req.ToID). Grupta req.ToID zaten
	// conv_id'nin kendisi (bkz. findOrCreateConversation: isGroup ise toDID
	// aynen döner) — teslimat mantığı bunu tek bir alıcı DID'iymiş gibi
	// kullanamaz. conv_members'tan gönderen hariç tüm üyeleri çekip her
	// birine ayrı teslimat uygula (extra_handlers.go:316'daki group_created
	// bildirim deseniyle aynı yaklaşım).
	recipients := []string{req.ToID}
	if req.IsGroup {
		recipients = groupRecipientDIDs(convID, user.DID)
	}

	senderName := user.DisplayName
	if senderName == "" {
		senderName = user.Username
	}

	for _, recipientDID := range recipients {
		deliverMessageToRecipient(recipientDID, msgID, req.Type, user.DID, senderName, convID, msgPayload)
	}

	// Panik sinyali (Madde 13, Bölüm 6) — çevrimiçi/çevrimdışı fark etmeksizin
	// HER ZAMAN yüksek öncelikli push tetiklenir. WS "online" olması alıcının
	// telefonunun kilitli/sessiz olmadığı anlamına gelmez; güvenlik-kritik
	// sinyal bu varsayıma güvenemez. Grup panik mesajında tüm üyelere gider.
	if req.Type == models.MsgPanicAlert {
		for _, recipientDID := range recipients {
			go sendPanicPush(recipientDID, convID, senderName)
		}
	}

	// Kredi
	go credit.TrackMessageSent(user.DID)

	respond(w, 201, map[string]interface{}{
		"id":      msgID,
		"conv_id": convID,
		"status":  "sent",
	}, "")
}

// groupRecipientDIDs, bir grup konuşmasındaki gönderen hariç tüm üyelerin
// DID'lerini döner (HandleSendMessage'ın grup teslimat döngüsü için).
func groupRecipientDIDs(convID, senderDID string) []string {
	rows, err := db.DB.Query(
		"SELECT user_did FROM conv_members WHERE conv_id = ? AND user_did != ?",
		convID, senderDID,
	)
	if err != nil {
		log.Printf("groupRecipientDIDs sorgu hatası (conv=%s): %v", convID, err)
		return nil
	}
	defer rows.Close()

	var dids []string
	for rows.Next() {
		var did string
		if rows.Scan(&did) == nil {
			dids = append(dids, did)
		}
	}
	return dids
}

// deliverMessageToRecipient — tek bir alıcıya WS/P2P/gossip/FCM teslimat
// mantığı. Eskiden HandleSendMessage içinde req.ToID'ye özel inline
// yazılıydı (1-1 varsayımıyla); artık hem 1-1 hem grup-üye-başına çağrı
// için ortak — HandleSendMessage bunu recipients listesindeki her DID için
// ayrı ayrı çağırır.
func deliverMessageToRecipient(recipientDID, msgID string, msgType models.MessageType, senderDID, senderName, convID string, msgPayload map[string]interface{}) {
	if messaging.GlobalHub.IsOnline(recipientDID) {
		// Online → ilet. SendTo'nun dönüş değeri kontrol edilmeli: alıcının
		// Send kanalı doluysa (yavaş tüketici / backpressure) mesaj kuyruğa
		// hiç girmez ve SendTo false döner — bu durumda 'delivered'
		// işaretlemek yanlış pozitif olur (mesaj asla iletilmeyecek ama
		// gönderene "delivered" denir, alıcı hiçbir fallback yoluyla mesajı
		// almaz). Sadece SendTo gerçekten kuyruğa yazdıysa delivered say.
		if messaging.GlobalHub.SendTo(recipientDID, "new_message", msgPayload) {
			deliveredAt := time.Now()
			// messages.status/delivered_at: tek sütun, 1-1'de doğru — grup
			// mesajında birden fazla üye "delivered" olursa son yazan kazanır.
			// GERİYE UYUMLULUK İÇİN DOKUNULMADI (mevcut GET /messages/{id}/status
			// hâlâ bunu okuyor). message_delivery_status: EK, per-recipient bilgi
			// — her üye kendi (message_id, recipient_did) satırını yazar, üye
			// sayısı kadar ayrı satır birikir, birbirini ezmez.
			db.DB.Exec("UPDATE messages SET status = 'delivered', delivered_at = ? WHERE id = ?",
				deliveredAt.Format(time.RFC3339), msgID)
			db.DB.Exec(`
				INSERT INTO message_delivery_status (message_id, recipient_did, delivered_at)
				VALUES (?, ?, ?)
				ON CONFLICT(message_id, recipient_did) DO UPDATE SET delivered_at = excluded.delivered_at`,
				msgID, recipientDID, deliveredAt.Format(time.RFC3339))
			// Gönderene delivery_ack gönder
			messaging.GlobalHub.SendDeliveryAck(senderDID, msgID, string(models.StatusDelivered))
			return
		}
		// SendTo false: kanal doluydu, mesaj kuyruğa girmedi — offline yoluna
		// düş (aşağıdaki P2P/gossip/push fallback).
	}

	// Offline → P2P GossipSub ile yay; başarısız olursa HTTP gossip fallback
	go func() {
		if err := p2p.PublishMessage(recipientDID, "new_message", msgPayload); err != nil {
			gossip.RelayToPeers(recipientDID, "new_message", msgPayload)
		}
	}()

	// Push bildirim (FCM/APNs) — alıcının FCM token'ı varsa gönder.
	// Panik mesajı burada ATLANIR (msgType != MsgPanicAlert şartı) — onun
	// için HandleSendMessage'da ayrı, ÇEVRİMİÇİ/ÇEVRİMDIŞI FARK ETMEKSİZİN
	// HER ZAMAN tetiklenen bir çağrı var; aynı mesaj için iki bildirim gitmesin.
	// read_receipt de atlanır (Adım 6b) — meta-sinyal için "yeni mesaj"
	// push'u kullanıcıyı yanlış bilgilendirir (aslında yeni mesaj yok).
	if msgType != models.MsgPanicAlert && msgType != models.MsgReadReceipt {
		go func() {
			var fcmToken string
			db.DB.QueryRow("SELECT fcm_token FROM users WHERE did = ?", recipientDID).Scan(&fcmToken)
			if fcmToken != "" {
				pushMsg := push.NewMessage(senderName, "🔒 Yeni şifreli mesaj", convID)
				if err := push.Default.Send(context.Background(), fcmToken, pushMsg); err != nil {
					// Push hatası mesaj gönderimini etkilemez
				}
			}
		}()
	}
}

// sendPanicPush — tek bir alıcıya panik-alarmı push bildirimi gönderir.
// HandleSendMessage, recipients listesindeki her DID için bunu ayrı çağırır
// (çevrimiçi/çevrimdışı fark etmeksizin her zaman).
func sendPanicPush(recipientDID, convID, senderName string) {
	var fcmToken string
	db.DB.QueryRow("SELECT fcm_token FROM users WHERE did = ?", recipientDID).Scan(&fcmToken)
	if fcmToken == "" {
		return
	}
	pushMsg := push.PanicAlert(senderName, convID)
	if err := push.Default.Send(context.Background(), fcmToken, pushMsg); err != nil {
		// Push hatası mesaj gönderimini etkilemez
	}
}

// ─── KREDİ ────────────────────────────────────────────────────────────────────

// GET /v1/credit/score
func HandleGetCreditScore(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	history, _ := credit.GetHistory(user.DID, 20)

	respond(w, 200, map[string]interface{}{
		"score":           user.CreditScore,
		"tier":            user.Tier,
		"tier_name":       models.TierNames[user.Tier],
		"history":         history,
		"next_tier_score": nextTierScore(user.Tier),
	}, "")
}

func nextTierScore(tier int) float64 {
	if tier >= 5 {
		return 100
	}
	return models.TierMinScore[tier+1]
}

// POST /v1/spam/report
//
// Bölüm 2.2 (docs/spec/obscura_denetim_topluluk_katmani.md): kanıtsız şikayet
// işleme alınmaz. message_id + evidence_ciphertext_hash zorunlu; VerifyEvidence
// bunun gerçekten bu mesaj, gerçekten reported_did'den, gerçekten şikayetçiye
// gönderildiğini doğrular (Bölüm 2.3 TIER A — içerik hiç okunmaz/çözülmez).
// Bariz spam ise (İlke 5 carve-out) otomatik işlenir; değilse insan inceleme
// kuyruğuna düşer. Brigading tespit edilirse (Bölüm 4) otomatik ceza YOK.
func HandleSpamReport(w http.ResponseWriter, r *http.Request) {
	reporter := getUser(r)

	var req struct {
		ReportedDID            string `json:"reported_did"`
		Reason                 string `json:"reason"`
		Category               string `json:"category"`
		MessageID              string `json:"message_id"`
		EvidenceScreenshotURL  string `json:"evidence_screenshot_url"`
		EvidenceCiphertextHash string `json:"evidence_ciphertext_hash"`
		// Adım 8 — sealed-sender mesajları için imza-tabanlı kanıt (bkz.
		// moderation.VerifySealedEvidence, mobile/lib/sender-attestation-cache.ts
		// SenderAttestation). Şikayet edilen mesaj sealed DEĞİLSE bu alanlar
		// boş bırakılır ve eski (from_did karşılaştırmalı) yol kullanılır.
		SealedCertIdentityDHPubHex string `json:"sealed_cert_identity_dh_pub_hex,omitempty"`
		SealedCertSigningPubHex    string `json:"sealed_cert_signing_pub_hex,omitempty"`
		SealedCertExpiresAt        uint64 `json:"sealed_cert_expires_at,omitempty"`
		SealedCertSignatureHex     string `json:"sealed_cert_signature_hex,omitempty"`
	}
	if err := decodeBody(r, &req); err != nil || req.ReportedDID == "" {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}
	if !moderation.IsKnownCategory(req.Category) {
		respond(w, 400, nil, "Geçersiz kategori")
		return
	}
	if req.MessageID == "" || req.EvidenceCiphertextHash == "" {
		respond(w, 400, nil, "Kanıt zorunlu: message_id ve evidence_ciphertext_hash")
		return
	}

	// Adım 8: sealed mesajlarda DB'deki from_did boş olduğundan (ADR-0016)
	// eski karşılaştırmalı yol anlamsız kalır — client sealed kanıt alanlarını
	// gönderdiyse (yalnızca gerçekten sealed bir mesajı şikayet ederken
	// gönderir, bkz. mobile SenderAttestation) imza-tabanlı yola geçilir.
	isSealedComplaint := req.SealedCertSignatureHex != ""
	var verified bool
	var verErr error
	if isSealedComplaint {
		verified, verErr = moderation.VerifySealedEvidence(r.Context(), db.DB, req.MessageID, req.ReportedDID, reporter.DID, req.EvidenceCiphertextHash,
			moderation.SealedCertEvidence{
				IdentityDHPubHex: req.SealedCertIdentityDHPubHex,
				SigningPubHex:    req.SealedCertSigningPubHex,
				ExpiresAt:        req.SealedCertExpiresAt,
				SignatureHex:     req.SealedCertSignatureHex,
			}, uint64(time.Now().Unix()))
	} else {
		verified, verErr = moderation.VerifyEvidence(r.Context(), db.DB, req.MessageID, req.ReportedDID, reporter.DID, req.EvidenceCiphertextHash)
	}
	if verErr != nil && !errors.Is(verErr, moderation.ErrEvidenceMismatch) {
		respond(w, 500, nil, "Kanıt doğrulanamadı")
		return
	}
	if !verified {
		respond(w, 400, nil, "Kanıt sunucu kaydıyla eşleşmiyor")
		return
	}

	reportID := uuid.New().String()
	err := moderation.Report(r.Context(), db.DB, moderation.ReportInput{
		ID:                     reportID,
		MessageID:              req.MessageID,
		ReporterDID:            reporter.DID,
		ReportedDID:            req.ReportedDID,
		Reason:                 req.Reason,
		Category:               req.Category,
		EvidenceScreenshotURL:  req.EvidenceScreenshotURL,
		EvidenceCiphertextHash: req.EvidenceCiphertextHash,
		EvidenceVerified:       true,
	})
	if err != nil {
		respond(w, 500, nil, "Rapor kaydedilemedi")
		return
	}

	// Brigading: aynı hedefe kısa sürede toplu şikayet → otomatik ceza yok,
	// elle/kurul incelemesine düşer (Bölüm 4).
	brigading, _ := moderation.IsBrigading(r.Context(), db.DB, req.ReportedDID)
	if brigading {
		_ = moderation.EnqueueReview(r.Context(), db.DB, reportID, "brigading")
		respond(w, 200, map[string]string{"status": "queued_for_review", "report_id": reportID}, "")
		return
	}

	// Bariz spam ise (İlke 5: yalnızca bariz spam otomatik işlenir) hemen
	// karara bağlanır; değilse insan incelemesine düşer. Kronik asılsız
	// şikayetçi (Bölüm 4: weight zamanla düşer) bu hızlı-yoldan hiç
	// geçemez — weight'in gerçek karar noktası burası, düşükse skor ne
	// olursa olsun insan incelemesine düşer.
	autoProcessed := false
	credWeight, credErr := moderation.GetCredibilityWeight(r.Context(), db.DB, reporter.DID)
	lowCredibility := credErr == nil && credWeight < moderation.LowCredibilityWeightThreshold
	// Adım 8: sealed şikayetlerde bu hızlı-yol ATLANIR — recentFanout sorgusu
	// (aşağıda) from_did = reported_did'e dayanıyor, ama sealed mesajlarda
	// from_did DB'de her zaman "" (ADR-0016): sorgu reported_did'e özgü
	// fanout DEĞİL, TÜM sealed gönderenlerin toplam alıcı çeşitliliğini
	// döndürür — yanlış sinyal. Doğru fanout'u hesaplamanın tek yolu
	// göndereni server-side kalıcı/aranabilir bir kimlikle etiketlemek
	// olurdu ki bu tam olarak sealed-sender'ın (Adım 6/7) engellemeye
	// çalıştığı şey — o yüzden düzeltmek yerine BİLİNÇLİ atlanıyor. Zaten
	// zararsız: recentFanout burada hesaplanamayınca sabit 1'e düşerdi, ve
	// yukarıdaki yorumun kendisi bunun skoru tam sınırda (0.7) tutup ASLA
	// auto-flag tetiklemediğini söylüyor — yani atlamak, "her zaman insan
	// incelemesine düşür" ile matematiksel olarak eşdeğer, davranış kaybı yok.
	if req.Category == moderation.CategorySpam && !lowCredibility && !isSealedComplaint {
		var ciphertext string
		_ = db.DB.QueryRowContext(r.Context(), `SELECT ciphertext FROM messages WHERE id = ?`, req.MessageID).Scan(&ciphertext)

		// Score's fanout signal needs the sender's REAL recent recipient
		// spread, not a hardcoded 1 — otherwise a single-recipient complaint
		// can never mathematically clear the spam threshold (len+entropy+
		// repetition alone cap out at exactly 0.7, the boundary itself).
		var recentFanout int
		_ = db.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(DISTINCT to_did) FROM messages WHERE from_did = ? AND sent_at >= ?`,
			req.ReportedDID, time.Now().UTC().Add(-1*time.Hour).Format(time.RFC3339),
		).Scan(&recentFanout)
		if recentFanout < 1 {
			recentFanout = 1
		}

		score, scoreErr := moderation.Score(r.Context(), []byte(ciphertext), moderation.Metadata{SenderDID: req.ReportedDID, RecipientCount: recentFanout})
		if scoreErr == nil && moderation.IsSpam(score) {
			if err := moderation.RecordComplaintVerdict(r.Context(), db.DB, reportID, true); err != nil {
				respond(w, 500, nil, "Ceza uygulanamadı")
				return
			}
			autoProcessed = true
		}
	}
	if !autoProcessed {
		_ = moderation.EnqueueReview(r.Context(), db.DB, reportID, "insan incelemesi bekliyor")
	}

	respond(w, 200, map[string]string{"status": "reported", "report_id": reportID}, "")
}

// POST /v1/complaints/{id}/verdict — operatör/kurul kararı (İlke 5: sistem
// ön eleyicidir, ciddi cezalarda karar insana aittir). upheld=true ise
// şikayet edilen kademeli yaptırıma girer; false ise şikayetçi girer
// (Bölüm 4: yalan şikayet ihlaldir).
func HandleComplaintVerdict(w http.ResponseWriter, r *http.Request) {
	caller := getUser(r)
	// PLACEHOLDER: gerçek bir kurul/operatör rol sistemi gelene kadar "admin" ==
	// Diamond tier (5) — internal/airdrop.AdminMinTier ile aynı desen.
	const adminMinTier = 5
	if caller.Tier < adminMinTier {
		respond(w, 403, nil, "yetkiniz yok (kurul/operatör gerekli)")
		return
	}

	reportID := mux.Vars(r)["id"]
	if reportID == "" {
		respond(w, 400, nil, "report id zorunlu")
		return
	}

	var req struct {
		Upheld bool `json:"upheld"`
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}

	if err := moderation.RecordComplaintVerdict(r.Context(), db.DB, reportID, req.Upheld); err != nil {
		respond(w, 500, nil, "Karar uygulanamadı")
		return
	}

	respond(w, 200, map[string]string{"status": "verdict_recorded"}, "")
}

// GET /v1/moderation/my-status — şeffaflık (İlke 3): kullanıcı hangi
// kademede/neden olduğunu görebilir. Gizli ceza/puanlama yoktur.
func HandleModerationMyStatus(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	rows, err := db.DB.QueryContext(r.Context(), `
		SELECT category, tier, action, created_at, expires_at
		FROM user_violations WHERE user_did = ? ORDER BY created_at DESC`, user.DID)
	if err != nil {
		respond(w, 500, nil, "Durum okunamadı")
		return
	}
	defer rows.Close()

	type violation struct {
		Category  string  `json:"category"`
		Tier      int     `json:"tier"`
		Action    string  `json:"action"`
		CreatedAt string  `json:"created_at"`
		ExpiresAt *string `json:"expires_at,omitempty"`
	}
	var violations []violation
	for rows.Next() {
		var v violation
		var expiresAt sql.NullString
		if err := rows.Scan(&v.Category, &v.Tier, &v.Action, &v.CreatedAt, &expiresAt); err != nil {
			respond(w, 500, nil, "Durum okunamadı")
			return
		}
		if expiresAt.Valid {
			v.ExpiresAt = &expiresAt.String
		}
		violations = append(violations, v)
	}

	respond(w, 200, map[string]interface{}{
		"is_banned":      user.IsBanned,
		"ban_expires_at": user.BanExpiresAt,
		"violations":     violations,
	}, "")
}

// ─── NODE BİLGİSİ ─────────────────────────────────────────────────────────────

// GET /v1/node/status
func HandleNodeStatus(w http.ResponseWriter, r *http.Request) {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-unknown"
	}
	respond(w, 200, map[string]interface{}{
		"node_id":      nodeID,
		"node_type":    "bootstrap",
		"version":      "3.0.0",
		"online_users": messaging.GlobalHub.OnlineCount(),
		"uptime":       time.Now().Unix(),
		"status":       "healthy",
	}, "")
}

// ─── YARDIMCI ─────────────────────────────────────────────────────────────────

func findOrCreateConversation(fromDID, toDID string, isGroup bool) (string, error) {
	if isGroup {
		return toDID, nil // Grup konuşması zaten var
	}

	// Mevcut 1-1 konuşmayı bul
	var convID string
	err := db.DB.QueryRow(`
		SELECT cm1.conv_id FROM conv_members cm1
		JOIN conv_members cm2 ON cm1.conv_id = cm2.conv_id
		JOIN conversations c ON c.id = cm1.conv_id
		WHERE cm1.user_did = ? AND cm2.user_did = ? AND c.is_group = 0
		LIMIT 1`, fromDID, toDID,
	).Scan(&convID)

	if err == nil {
		return convID, nil
	}

	// Yeni konuşma oluştur
	convID = uuid.New().String()
	now := time.Now()

	_, err = db.DB.Exec(`
		INSERT INTO conversations (id, is_group, created_at, updated_at)
		VALUES (?, 0, ?, ?)`,
		convID, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return "", err
	}

	// Her iki kullanıcıyı ekle
	for _, did := range []string{fromDID, toDID} {
		db.DB.Exec(`
			INSERT INTO conv_members (conv_id, user_did, role, joined_at)
			VALUES (?, ?, 'member', ?)`,
			convID, did, now.Format(time.RFC3339),
		)
	}

	return convID, nil
}
