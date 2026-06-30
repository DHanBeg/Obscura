package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/gossip"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/moderation"
	"obscura.network/core/internal/models"
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

	code, err := auth.GenerateOTP(req.Phone)
	if err != nil {
		respond(w, 500, nil, "OTP oluşturulamadı")
		return
	}

	// IP tespiti — proxy arkasında X-Forwarded-For, yoksa RemoteAddr
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
		if i := strings.LastIndex(clientIP, ":"); i > 0 {
			clientIP = clientIP[:i]
		}
	} else {
		if i := strings.Index(clientIP, ","); i > 0 {
			clientIP = strings.TrimSpace(clientIP[:i])
		}
	}
	log.Printf("🔐 [OTP-MONITOR] IP:%s | Tel:%s | Kod:%s", clientIP, req.Phone, code)

	// dev_otp: development modunda VEYA log provider'da her zaman döner
	data := map[string]interface{}{"message": "OTP gönderildi"}
	provider := os.Getenv("SMS_PROVIDER")
	isDev := os.Getenv("OBSCURA_ENV") == "development"
	if isDev || provider == "" || provider == "log" {
		data["dev_otp"] = code
	}
	respond(w, 200, data, "")
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
		respond(w, 401, nil, err.Error())
		return
	}

	// Kullanıcı var mı?
	var user models.User
	var zkIDVerified int
	err := db.DB.QueryRow(`
		SELECT id, phone, username, display_name, did, identity_key, avatar_url,
		       tier, credit_score, is_active, is_banned, node_id,
		       created_at, updated_at, last_seen_at,
		       COALESCE(zk_id_verified, 0) AS zk_id_verified
		FROM users WHERE phone = ?`, req.Phone,
	).Scan(&user.ID, &user.Phone, &user.Username, &user.DisplayName, &user.DID,
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
			INSERT INTO users (id, phone, username, display_name, did, identity_key, avatar_url,
			                   tier, credit_score, is_active, is_banned, node_id,
			                   created_at, updated_at, last_seen_at,
			                   zk_id_proof_b64, zk_id_public_params, zk_id_verified)
			VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, 1, 0, '',
			        ?, ?, ?,
			        ?, ?, ?)`,
			newUser.ID, newUser.Phone, newUser.Username, newUser.DisplayName,
			newUser.DID, newUser.IdentityKey, newUser.Tier, newUser.CreditScore,
			now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
			nullableString(zkProofToSave), nullableString(zkPublicToSave), zkVerifiedFlag,
		)
		if err != nil {
			respond(w, 500, nil, fmt.Sprintf("Kullanıcı oluşturulamadı: %v", err))
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

// GET /v1/users/{did}
func HandleGetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetDID := vars["did"]

	var user models.User
	err := db.DB.QueryRow(`
		SELECT id, username, display_name, did, identity_key, avatar_url,
		       tier, credit_score, is_active, created_at
		FROM users WHERE did = ? AND is_active = 1`, targetDID,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.DID,
		&user.IdentityKey, &user.AvatarURL, &user.Tier, &user.CreditScore,
		&user.IsActive, new(string))

	if err == sql.ErrNoRows {
		respond(w, 404, nil, "Kullanıcı bulunamadı")
		return
	}
	if err != nil {
		respond(w, 500, nil, "Veritabanı hatası")
		return
	}

	respond(w, 200, user, "")
}

// GET /v1/users/search?q=username
func HandleSearchUser(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		respond(w, 400, nil, "En az 2 karakter giriniz")
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, username, display_name, did, avatar_url, tier, credit_score
		FROM users
		WHERE (username LIKE ? OR display_name LIKE ?) AND is_active = 1
		LIMIT 20`, "%"+q+"%", "%"+q+"%",
	)
	if err != nil {
		respond(w, 500, nil, "Arama hatası")
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.DID, &u.AvatarURL, &u.Tier, &u.CreditScore); err != nil {
			log.Printf("HandleSearchUser scan hatası: %v", err)
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		respond(w, 500, nil, "Arama hatası")
		return
	}

	respond(w, 200, users, "")
}

// ─── MESAJLAŞMA ───────────────────────────────────────────────────────────────

// GET /v1/conversations
func HandleGetConversations(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	// Tek sorgu ile konuşmalar + peer bilgisi (nested query deadlock önleme)
	rows, err := db.DB.Query(`
		SELECT c.id, c.is_group, c.name, c.avatar_url,
		       c.last_msg_text, c.last_msg_at,
		       cm.unread_count,
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
		PeerDID  string `json:"peer_did,omitempty"`
		PeerName string `json:"peer_name,omitempty"`
		PeerTier int    `json:"peer_tier,omitempty"`
	}

	var convs []ConvWithPeer
	for rows.Next() {
		var c ConvWithPeer
		var lastMsgAt sql.NullString
		if err := rows.Scan(&c.ID, &c.IsGroup, &c.Name, &c.AvatarURL,
			&c.LastMsgText, &lastMsgAt, &c.UnreadCount,
			&c.PeerDID, &c.PeerName, &c.PeerTier); err != nil {
			log.Printf("HandleGetConversations scan hatası: %v", err)
			continue
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
		       COALESCE(dilithium_sig, '') AS dilithium_sig
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
		var deliveredAt, readAt sql.NullString
		if err := rows.Scan(&m.ID, &m.ConvID, &m.FromDID, &m.ToDID, &m.Type,
			&m.Ciphertext, &m.MediaURL, &m.Status, &m.IsGroup,
			&m.ReplyToID, new(string), &deliveredAt, &readAt, &m.DilithiumSig); err != nil {
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

	// Mesaj kaydet
	now := time.Now()
	msgID := uuid.New().String()
	expires := now.Add(30 * 24 * time.Hour)

	_, err = db.DB.Exec(`
		INSERT INTO messages (id, conv_id, from_did, to_did, type, ciphertext, media_url,
		                      status, is_group, reply_to_id, sent_at, expires_at, dilithium_sig,
		                      is_encrypted, encryption_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sent', ?, ?, ?, ?, ?, 1, ?)`,
		msgID, convID, user.DID, req.ToID, req.Type, req.Ciphertext,
		req.MediaURL, req.IsGroup, req.ReplyToID,
		now.Format(time.RFC3339), expires.Format(time.RFC3339),
		req.DilithiumSig, req.EffectiveEncryptionType(),
	)
	if err != nil {
		respond(w, 500, nil, "Mesaj kaydedilemedi")
		return
	}

	// Konuşmayı güncelle
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

	// Gerçek zamanlı iletim (WebSocket)
	msgPayload := map[string]interface{}{
		"id":            msgID,
		"conv_id":       convID,
		"from_did":      user.DID,
		"ciphertext":    req.Ciphertext,
		"type":          req.Type,
		"sent_at":       now.Unix(),
		"dilithium_sig": req.DilithiumSig, // boşsa JSON'da "" olarak görünür
	}

	if messaging.GlobalHub.IsOnline(req.ToID) {
		// Online → ilet + delivered olarak işaretle
		messaging.GlobalHub.SendTo(req.ToID, "new_message", msgPayload)
		deliveredAt := time.Now()
		db.DB.Exec("UPDATE messages SET status = 'delivered', delivered_at = ? WHERE id = ?",
			deliveredAt.Format(time.RFC3339), msgID)
		// Gönderene delivery_ack gönder
		messaging.GlobalHub.SendDeliveryAck(user.DID, msgID, string(models.StatusDelivered))
	} else {
		// Offline → P2P GossipSub ile yay; başarısız olursa HTTP gossip fallback
		go func() {
			if err := p2p.PublishMessage(req.ToID, "new_message", msgPayload); err != nil {
				gossip.RelayToPeers(req.ToID, "new_message", msgPayload)
			}
		}()

		// Push bildirim (FCM/APNs) — alıcının FCM token'ı varsa gönder
		go func() {
			var fcmToken string
			db.DB.QueryRow("SELECT fcm_token FROM users WHERE did = ?", req.ToID).Scan(&fcmToken)
			if fcmToken != "" {
				senderName := user.DisplayName
				if senderName == "" {
					senderName = user.Username
				}
				pushMsg := push.NewMessage(senderName, "🔒 Yeni şifreli mesaj", convID)
				if err := push.Default.Send(context.Background(), fcmToken, pushMsg); err != nil {
					// Push hatası mesaj gönderimini etkilemez
				}
			}
		}()
	}

	// Kredi
	go credit.TrackMessageSent(user.DID)

	respond(w, 201, map[string]interface{}{
		"id":      msgID,
		"conv_id": convID,
		"status":  "sent",
	}, "")
}

// ─── KREDİ ────────────────────────────────────────────────────────────────────

// GET /v1/credit/score
func HandleGetCreditScore(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)

	history, _ := credit.GetHistory(user.DID, 20)

	respond(w, 200, map[string]interface{}{
		"score":    user.CreditScore,
		"tier":     user.Tier,
		"tier_name": models.TierNames[user.Tier],
		"history":  history,
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
func HandleSpamReport(w http.ResponseWriter, r *http.Request) {
	reporter := getUser(r)

	var req struct {
		ReportedDID string `json:"reported_did"`
		Reason      string `json:"reason"`
	}
	if err := decodeBody(r, &req); err != nil || req.ReportedDID == "" {
		respond(w, 400, nil, "Geçersiz istek")
		return
	}

	// Persist via moderation package (single source of truth for spam_reports
	// writes; see ADR-0013 — keeps the door open for ZK-ML Score() enrichment).
	if err := moderation.Report(r.Context(), db.DB, "", reporter.DID, req.ReportedDID, req.Reason); err != nil {
		respond(w, 500, nil, "Rapor kaydedilemedi")
		return
	}

	// Bildirilen kullanıcının puanını düşür
	credit.AddCustomEvent(req.ReportedDID, "spam_received", -5, "Spam raporu alındı")

	respond(w, 200, map[string]string{"status": "reported"}, "")
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
