package api

// MLS (RFC 9420) HTTP endpoints — group encryption per spec Bölüm 6.3.
//
// All operations route through the Rust mls-cli subprocess via internal/mls/client.go.
// Server holds NO group secrets; only encrypted/wire blobs.
//
// Endpoints (per spec Bölüm 17 EK B):
//   POST /v1/mls/key-package        upload self KeyPackage
//   GET  /v1/mls/key-package/{did}  consume one peer KeyPackage
//   POST /v1/mls/group              create new group
//   POST /v1/mls/group/{id}/add     add member (sender supplies Welcome+Commit)
//   POST /v1/mls/group/{id}/message broadcast encrypted message
//   GET  /v1/mls/group/{id}         group info + member list
//   GET  /v1/mls/welcomes           pending welcomes for caller
//   POST /v1/mls/welcomes/ack       mark welcome delivered

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/credit"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	mlspkg "obscura.network/core/internal/mls"
	"obscura.network/core/internal/moderation"
)

// ─── Request types ───────────────────────────────────────────────────────────

type MLSUploadKeyPackageRequest struct {
	KeyPackageB64 string `json:"key_package_b64"`
	// Optional explicit TTL; default 90 days per spec Bölüm 4.2
	TTLDays int `json:"ttl_days,omitempty"`
}

type MLSCreateGroupRequest struct {
	Name        string `json:"name,omitempty"`
	Ciphersuite string `json:"ciphersuite,omitempty"` // default: MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519
	GroupID     string `json:"group_id"`              // client-generated, base64 of openmls group_id
	// Initial members are added via subsequent POST /v1/mls/group/{id}/add calls
}

type MLSAddMemberRequest struct {
	NewMemberDID string `json:"new_member_did"`
	// The sender (already in group) computed these client-side via mls-cli:
	CommitB64  string `json:"commit_b64"`
	WelcomeB64 string `json:"welcome_b64"`
	NewEpoch   uint64 `json:"new_epoch"`
}

type MLSGroupMessageRequest struct {
	CiphertextB64 string `json:"ciphertext_b64"`
	Epoch         uint64 `json:"epoch"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// POST /v1/mls/key-package
func HandleMLSUploadKeyPackage(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req MLSUploadKeyPackageRequest
	if err := decodeBody(r, &req); err != nil || req.KeyPackageB64 == "" {
		respond(w, 400, nil, "key_package_b64 zorunlu")
		return
	}
	ttlDays := req.TTLDays
	if ttlDays <= 0 || ttlDays > 365 {
		ttlDays = 90 // spec Bölüm 4.2
	}

	now := time.Now().UTC()
	expires := now.AddDate(0, 0, ttlDays)
	id := uuid.New().String()

	_, err := db.DB.Exec(`
		INSERT INTO mls_key_packages
			(id, user_did, key_package_b64, ciphersuite, used, created_at, expires_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		id, user.DID, req.KeyPackageB64,
		"MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
		now.Format(time.RFC3339), expires.Format(time.RFC3339),
	)
	if err != nil {
		respond(w, 500, nil, "KeyPackage kaydedilemedi: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"id":         id,
		"expires_at": expires.Format(time.RFC3339),
	}, "")
}

// GET /v1/mls/key-package/{did}
// Tek kullanımlık: fetch + mark used. Caller bu KeyPackage'ı add_member için kullanır.
func HandleMLSGetKeyPackage(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	targetDID := vars["did"]
	if targetDID == "" {
		respond(w, 400, nil, "did gerekli")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var id, kpB64 string
	err := db.DB.QueryRow(`
		SELECT id, key_package_b64 FROM mls_key_packages
		WHERE user_did = ? AND used = 0 AND expires_at > ?
		ORDER BY created_at ASC LIMIT 1`,
		targetDID, now,
	).Scan(&id, &kpB64)
	if err != nil {
		respond(w, 404, nil, "KeyPackage bulunamadı (kullanıcı KP yüklememiş veya tükenmiş)")
		return
	}

	// Mark used (one-time per spec)
	_, _ = db.DB.Exec(`UPDATE mls_key_packages SET used = 1, used_at = ? WHERE id = ?`,
		now, id)

	respond(w, 200, map[string]any{
		"key_package_b64": kpB64,
		"target_did":      targetDID,
	}, "")
}

// POST /v1/mls/group
// Kayıt: client mls-cli ile group oluşturur, group_id'yi server'a bildirir.
func HandleMLSCreateGroup(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req MLSCreateGroupRequest
	if err := decodeBody(r, &req); err != nil || req.GroupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}

	cs := req.Ciphersuite
	if cs == "" {
		cs = "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519"
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	groupRes, err := tx.Exec(`
		INSERT INTO mls_groups (id, creator_did, name, ciphersuite, epoch, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		req.GroupID, user.DID, req.Name, cs, now, now,
	)
	if err != nil {
		respond(w, 500, nil, "Grup oluşturulamadı: "+err.Error())
		return
	}
	// ON CONFLICT DO NOTHING sessizce no-op olabilir (aynı group_id tekrar
	// çağrılırsa) — kredi sadece GERÇEKTEN yeni bir grup insert edildiyse
	// verilir, aksi halde aynı group_id'yi tekrar POST etmek sınırsız
	// group_created kredisi çiftlemeye (cap dolana kadar) yol açardı.
	isNewGroup := false
	if n, raErr := groupRes.RowsAffected(); raErr == nil && n > 0 {
		isNewGroup = true
	}

	// Creator is the first member at epoch 0
	if _, err := tx.Exec(`
		INSERT INTO mls_group_members (group_id, user_did, role, joined_at, joined_at_epoch)
		VALUES (?, ?, 'creator', ?, 0)
		ON CONFLICT(group_id, user_did) DO NOTHING`,
		req.GroupID, user.DID, now,
	); err != nil {
		respond(w, 500, nil, "Üye kaydedilemedi: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	if isNewGroup {
		if err := credit.AddEvent(user.DID, credit.EventGroupCreated, "Grup oluşturuldu: "+req.GroupID); err != nil {
			log.Printf("⚠️ group_created kredi olayı başarısız (did=%s, group=%s): %v", user.DID, req.GroupID, err)
		}
	}

	respond(w, 200, map[string]any{
		"group_id":    req.GroupID,
		"creator_did": user.DID,
		"epoch":       0,
	}, "")
}

// POST /v1/mls/group/{id}/add
// Sender (existing member) computes commit + welcome client-side, server broadcasts.
func HandleMLSAddMember(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}
	var req MLSAddMemberRequest
	if err := decodeBody(r, &req); err != nil ||
		req.NewMemberDID == "" || req.CommitB64 == "" || req.WelcomeB64 == "" {
		respond(w, 400, nil, "new_member_did, commit_b64, welcome_b64 zorunlu")
		return
	}

	// Authorization: sender must be a current member
	var memberCheck int
	err := db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&memberCheck)
	if err == sql.ErrNoRows || memberCheck == 0 {
		respond(w, 403, nil, "Sadece grup üyeleri davet edebilir")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	// ── Grup büyüklük limiti — kurucu/davet eden kullanıcının kredi skoruna göre ─
	// Grubun yaratıcısının kredi skorunu DB'den çek; limit buna göre uygulanır.
	{
		var creatorDID string
		var creatorScore float64
		db.DB.QueryRow(`SELECT creator_did FROM mls_groups WHERE id = ?`, groupID).Scan(&creatorDID)
		db.DB.QueryRow(`SELECT credit_score FROM users WHERE did = ?`, creatorDID).Scan(&creatorScore)

		var currentCount int
		db.DB.QueryRow(`SELECT COUNT(*) FROM mls_group_members WHERE group_id = ?`, groupID).Scan(&currentCount)

		maxAllowed := moderation.MaxGroupSize(creatorScore)
		if currentCount >= maxAllowed {
			respond(w, 403, nil, fmt.Sprintf(
				"Grup üye limitine ulaşıldı. Hesabınızın daha aktif olması gerekiyor. (Mevcut limit: %d üye)", maxAllowed,
			))
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	// Add new member to roster
	if _, err := tx.Exec(`
		INSERT INTO mls_group_members (group_id, user_did, role, joined_at, joined_at_epoch)
		VALUES (?, ?, 'member', ?, ?)
		ON CONFLICT(group_id, user_did) DO UPDATE SET joined_at = excluded.joined_at`,
		groupID, req.NewMemberDID, now, req.NewEpoch,
	); err != nil {
		respond(w, 500, nil, "Üye eklenemedi: "+err.Error())
		return
	}

	// Bump group epoch
	if _, err := tx.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		req.NewEpoch, now, groupID); err != nil {
		respond(w, 500, nil, "Epoch güncellenemedi: "+err.Error())
		return
	}

	// Queue Welcome for new member
	welcomeID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO mls_welcome_queue (id, group_id, recipient_did, welcome_b64, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		welcomeID, groupID, req.NewMemberDID, req.WelcomeB64, now); err != nil {
		respond(w, 500, nil, "Welcome kaydedilemedi: "+err.Error())
		return
	}

	// Persist the Commit for EXISTING members (Tuğla 4e).
	// Welcome yalnızca YENİ üyeyi besler; mevcut üyeler epoch'u ancak Commit'i
	// işleyerek ilerletir. Aşağıdaki WS yayını yalnızca o an ONLINE olanlara
	// ulaşır — çevrimdışı bir mevcut üye commit'i kaçırırsa epoch atlar ve o
	// epoch'tan sonraki hiçbir mesajı çözemez. Bu yüzden commit, Welcome ile
	// AYNI transaction içinde kalıcılaştırılır; üye sonradan
	// GET /v1/mls/group/{id}/messages ile content_type='commit' satırlarını
	// çeker. Yeni tablo yok — mls_messages.content_type zaten bu ayrım için var.
	if _, err := tx.Exec(`
		INSERT INTO mls_messages (id, group_id, sender_did, ciphertext_b64, content_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'commit', ?, ?)`,
		uuid.New().String(), groupID, user.DID, req.CommitB64, req.NewEpoch, now); err != nil {
		respond(w, 500, nil, "Commit kaydedilemedi: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	// Push Welcome via WS if online (sadece hedef DID — mls_welcome point-to-point)
	if messaging.GlobalHub.IsOnline(req.NewMemberDID) {
		messaging.GlobalHub.SendTo(req.NewMemberDID, messaging.MsgTypeMlsWelcome, map[string]any{
			"group_id":    groupID,
			"welcome_b64": req.WelcomeB64,
			"epoch":       req.NewEpoch,
		})
	}

	// Broadcast Commit to all OTHER existing members (out-of-band: DB commit already succeeded).
	// N6 fix: broadcast errors are logged, not returned as 500 — the commit is already persisted.
	var broadcastCount int
	broadcastRows, bErr := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ? AND user_did != ?`,
		groupID, user.DID, req.NewMemberDID)
	if bErr != nil {
		fmt.Printf("⚠️  MLS AddMember broadcast sorgusu başarısız: %v\n", bErr)
	} else {
		defer broadcastRows.Close()
		var recipients []string
		for broadcastRows.Next() {
			var d string
			if err := broadcastRows.Scan(&d); err != nil {
				fmt.Printf("⚠️  MLS AddMember broadcast scan hatası: %v\n", err)
				continue
			}
			recipients = append(recipients, d)
		}
		for _, d := range recipients {
			if messaging.GlobalHub.IsOnline(d) {
				messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsCommit, map[string]any{
					"group_id":   groupID,
					"commit_b64": req.CommitB64,
					"epoch":      req.NewEpoch,
				})
				broadcastCount++
			}
		}
	}

	respond(w, 200, map[string]any{
		"group_id":  groupID,
		"new_epoch": req.NewEpoch,
		"welcomed":  req.NewMemberDID,
		"broadcast": broadcastCount,
	}, "")
}

// POST /v1/mls/group/{id}/message
// Encrypted application message — server stores ciphertext + broadcasts to members.
func HandleMLSGroupMessage(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	var req MLSGroupMessageRequest
	if err := decodeBody(r, &req); err != nil || req.CiphertextB64 == "" {
		respond(w, 400, nil, "ciphertext_b64 zorunlu")
		return
	}

	// Sender must be a member
	var ok int
	err := db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&ok)
	if err == sql.ErrNoRows || ok == 0 {
		respond(w, 403, nil, "Grubun üyesi değilsiniz")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	msgID := uuid.New().String()
	if _, err := db.DB.Exec(`
		INSERT INTO mls_messages (id, group_id, sender_did, ciphertext_b64, content_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'application', ?, ?)`,
		msgID, groupID, user.DID, req.CiphertextB64, req.Epoch, now,
	); err != nil {
		respond(w, 500, nil, "Mesaj kaydedilemedi: "+err.Error())
		return
	}

	// Broadcast to ALL members — out-of-band after successful INSERT.
	// N6 fix: broadcast errors logged, not returned as 500.
	var delivered, total int
	msgRows, bErr := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ?`,
		groupID, user.DID)
	if bErr != nil {
		fmt.Printf("⚠️  MLS GroupMessage broadcast sorgusu başarısız: %v\n", bErr)
	} else {
		defer msgRows.Close()
		for msgRows.Next() {
			var d string
			if err := msgRows.Scan(&d); err != nil {
				fmt.Printf("⚠️  MLS GroupMessage broadcast scan hatası: %v\n", err)
				continue
			}
			total++
			if messaging.GlobalHub.IsOnline(d) {
				messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsMessage, map[string]any{
					"id":             msgID,
					"group_id":       groupID,
					"sender_did":     user.DID,
					"ciphertext_b64": req.CiphertextB64,
					"epoch":          req.Epoch,
					"created_at":     now,
				})
				delivered++
			}
		}
	}

	respond(w, 200, map[string]any{
		"id":         msgID,
		"created_at": now,
		"delivered":  delivered,
		"queued":     total - delivered,
	}, "")
}

// GET /v1/mls/group/{id}
func HandleMLSGroupInfo(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]

	// Membership check
	var role string
	err := db.DB.QueryRow(`SELECT role FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&role)
	if err != nil {
		respond(w, 403, nil, "Grubun üyesi değilsiniz")
		return
	}

	var name, cs, createdAt string
	var epoch int64
	err = db.DB.QueryRow(`SELECT name, ciphersuite, epoch, created_at FROM mls_groups WHERE id = ?`,
		groupID).Scan(&name, &cs, &epoch, &createdAt)
	if err != nil {
		respond(w, 404, nil, "Grup bulunamadı")
		return
	}

	// Member list
	type member struct {
		DID         string `json:"did"`
		Role        string `json:"role"`
		EpochJoined int64  `json:"epoch_joined"`
	}
	rows, err := db.DB.Query(`SELECT user_did, role, joined_at_epoch FROM mls_group_members WHERE group_id = ?`,
		groupID)
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer rows.Close()
	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.DID, &m.Role, &m.EpochJoined); err != nil {
			respond(w, 500, nil, "DB hatası: "+err.Error())
			return
		}
		members = append(members, m)
	}

	respond(w, 200, map[string]any{
		"id":          groupID,
		"name":        name,
		"ciphersuite": cs,
		"epoch":       epoch,
		"created_at":  createdAt,
		"role":        role,
		"members":     members,
	}, "")
}

// GET /v1/mls/welcomes — pending welcomes for caller
func HandleMLSPendingWelcomes(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	rows, err := db.DB.Query(`
		SELECT id, group_id, welcome_b64, created_at
		FROM mls_welcome_queue
		WHERE recipient_did = ? AND delivered_at IS NULL
		ORDER BY created_at ASC LIMIT 100`,
		user.DID,
	)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	defer rows.Close()

	type w_ struct {
		ID         string `json:"id"`
		GroupID    string `json:"group_id"`
		WelcomeB64 string `json:"welcome_b64"`
		CreatedAt  string `json:"created_at"`
	}
	var out []w_
	for rows.Next() {
		var x w_
		if err := rows.Scan(&x.ID, &x.GroupID, &x.WelcomeB64, &x.CreatedAt); err != nil {
			respond(w, 500, nil, err.Error())
			return
		}
		out = append(out, x)
	}
	respond(w, 200, out, "")
}

// POST /v1/mls/welcomes/ack
func HandleMLSAckWelcome(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &req); err != nil || req.ID == "" {
		respond(w, 400, nil, "id zorunlu")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.DB.Exec(`UPDATE mls_welcome_queue SET delivered_at = ? WHERE id = ? AND recipient_did = ?`,
		now, req.ID, user.DID)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		respond(w, 404, nil, "Welcome bulunamadı veya size ait değil")
		return
	}
	respond(w, 200, map[string]any{"acked": true}, "")
}

// ─── EK İŞLEMLER (join / commit / remove / state) ────────────────────────────
//
// Bu handler'lar spec Bölüm 17 EK B'deki tam endpoint setini tamamlar:
//   POST   /v1/mls/group/{id}/join            — alıcı welcome'ı işledikten sonra üyelik teyit eder
//   POST   /v1/mls/group/{id}/commit          — genel commit broadcast (update / remove proposal)
//   DELETE /v1/mls/group/{id}/member/{did}    — üye çıkar (commit ile)
//   GET    /v1/mls/group/{id}/state           — grup state alias'ı (HandleMLSGroupInfo'ya yönlenir)

// MLSJoinRequest — alıcı welcome'ı işledi, üyelik teyidi.
// Sunucu zaten /add sırasında üyeyi tabloya yazıyor; bu endpoint kullanıcının
// gerçekten welcome'ı uygulayıp ratchet'e girdiğini onaylar (state senkronu).
type MLSJoinRequest struct {
	WelcomeID string `json:"welcome_id,omitempty"` // mls_welcome_queue.id (varsa ack ile birlikte)
	Epoch     uint64 `json:"epoch"`                // alıcının ulaştığı epoch
}

// POST /v1/mls/group/{id}/join
// Welcome'ı işlemiş yeni üye buraya çağrı yapar. Sunucu:
//  1. Üyeliği doğrular (zaten add sırasında eklenmiş olmalı)
//  2. welcome_id verildiyse mls_welcome_queue.delivered_at işaretler
//  3. Grup state özetini döner
func HandleMLSJoinGroup(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}
	var req MLSJoinRequest
	_ = decodeBody(r, &req) // body opsiyonel

	// Üyelik teyidi
	var role string
	err := db.DB.QueryRow(`SELECT role FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&role)
	if err == sql.ErrNoRows {
		respond(w, 403, nil, "Bu gruba üye değilsiniz (önce welcome işlenmeli)")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	// Welcome ack (varsa)
	now := time.Now().UTC().Format(time.RFC3339)
	if req.WelcomeID != "" {
		_, _ = db.DB.Exec(`UPDATE mls_welcome_queue SET delivered_at = ? WHERE id = ? AND recipient_did = ?`,
			now, req.WelcomeID, user.DID)
	}

	// Üye sayısı
	var memberCount int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM mls_group_members WHERE group_id = ?`, groupID).Scan(&memberCount)

	var name, cs string
	var epoch int64
	_ = db.DB.QueryRow(`SELECT name, ciphersuite, epoch FROM mls_groups WHERE id = ?`,
		groupID).Scan(&name, &cs, &epoch)

	respond(w, 200, map[string]any{
		"group_id":     groupID,
		"role":         role,
		"name":         name,
		"ciphersuite":  cs,
		"epoch":        epoch,
		"member_count": memberCount,
	}, "")
}

// MLSCommitRequest — genel commit (update / remove proposal).
// Add işlemleri için /add endpoint'i welcome ile birlikte kullanılır.
// Bu endpoint:
//   - leaf key update (forward secrecy rotasyonu)
//   - remove proposal commit'i
//   - external (PSK / reinit) commit'leri
//
// için tüm üyelere broadcast yapar.
type MLSCommitRequest struct {
	CommitB64    string `json:"commit_b64"`
	NewEpoch     uint64 `json:"new_epoch"`
	ProposalType string `json:"proposal_type,omitempty"` // "update" | "remove" | "external" | "psk"
	RemovedDID   string `json:"removed_did,omitempty"`   // remove ise hangi üye çıkıyor
}

// POST /v1/mls/group/{id}/commit
func HandleMLSCommitProposal(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}
	var req MLSCommitRequest
	if err := decodeBody(r, &req); err != nil || req.CommitB64 == "" {
		respond(w, 400, nil, "commit_b64 zorunlu")
		return
	}

	// Yetki: gönderen üye olmalı
	var memberCheck int
	err := db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&memberCheck)
	if err == sql.ErrNoRows || memberCheck == 0 {
		respond(w, 403, nil, "Sadece grup üyeleri commit gönderebilir")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	// Commit'i pending proposal olarak kaydet (audit + replay için)
	proposalID := uuid.New().String()
	proposalType := req.ProposalType
	if proposalType == "" {
		proposalType = "update"
	}
	if _, err := tx.Exec(`
		INSERT INTO mls_pending_proposals
			(id, group_id, proposer_did, proposal_b64, proposal_type, epoch, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		proposalID, groupID, user.DID, req.CommitB64, proposalType, req.NewEpoch, now,
	); err != nil {
		respond(w, 500, nil, "Proposal kaydedilemedi: "+err.Error())
		return
	}

	// Remove ise üyeyi de düş
	if proposalType == "remove" && req.RemovedDID != "" {
		if _, err := tx.Exec(`DELETE FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
			groupID, req.RemovedDID); err != nil {
			respond(w, 500, nil, "Üye silinemedi: "+err.Error())
			return
		}
	}

	// Epoch ilerlet
	if _, err := tx.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		req.NewEpoch, now, groupID); err != nil {
		respond(w, 500, nil, "Epoch güncellenemedi: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	// Tüm güncel üyelere broadcast (gönderen hariç — kendi state'i zaten ileri)
	rows, err := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ?`,
		groupID, user.DID)
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer rows.Close()
	var recipients []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			respond(w, 500, nil, "DB hatası: "+err.Error())
			return
		}
		recipients = append(recipients, d)
	}
	for _, d := range recipients {
		if messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsCommit, map[string]any{
				"group_id":      groupID,
				"commit_b64":    req.CommitB64,
				"epoch":         req.NewEpoch,
				"proposal_type": proposalType,
				"proposer_did":  user.DID,
			})
		}
	}

	respond(w, 200, map[string]any{
		"proposal_id":   proposalID,
		"group_id":      groupID,
		"new_epoch":     req.NewEpoch,
		"proposal_type": proposalType,
		"broadcast":     len(recipients),
	}, "")
}

// MLSRemoveMemberRequest — remove için commit gerekli (post-compromise security).
type MLSRemoveMemberRequest struct {
	CommitB64 string `json:"commit_b64"`
	NewEpoch  uint64 `json:"new_epoch"`
}

// DELETE /v1/mls/group/{id}/member/{did}
// Sadece grup üyesi başka bir üyeyi çıkarabilir. Gönderen client tarafında
// remove proposal + commit üretmeli (mls-cli ile), bu endpoint sunucu state'ini
// günceller ve diğer üyelere broadcast eder.
//
// Post-compromise security (spec): üye çıkarıldıktan sonraki tüm epoch'lar
// onun erişemeyeceği yeni ratchet anahtarlarını türetir.
func HandleMLSRemoveMember(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	targetDID := vars["did"]
	if groupID == "" || targetDID == "" {
		respond(w, 400, nil, "group_id ve did zorunlu")
		return
	}
	var req MLSRemoveMemberRequest
	if err := decodeBody(r, &req); err != nil || req.CommitB64 == "" {
		respond(w, 400, nil, "commit_b64 zorunlu (post-compromise security için commit gerekli)")
		return
	}

	// Yetki: gönderen üye olmalı + hedef de üye olmalı
	var senderRole string
	err := db.DB.QueryRow(`SELECT role FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&senderRole)
	if err == sql.ErrNoRows {
		respond(w, 403, nil, "Sadece grup üyeleri remove yapabilir")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	// Kendini çıkarma (leave): izinli ama epoch yine de ilerlemeli
	// Başkasını çıkarma: creator rolü zorunlu (basit policy — DAO ile genişletilebilir)
	if user.DID != targetDID && senderRole != "creator" {
		respond(w, 403, nil, "Başka üyeyi çıkarmak için creator yetkisi gerekli")
		return
	}

	var targetExists int
	err = db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, targetDID).Scan(&targetExists)
	if err == sql.ErrNoRows || targetExists == 0 {
		respond(w, 404, nil, "Hedef üye grupta değil")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	// Üyeyi sil
	if _, err := tx.Exec(`DELETE FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, targetDID); err != nil {
		respond(w, 500, nil, "Üye silinemedi: "+err.Error())
		return
	}

	// Commit'i kaydet
	proposalID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO mls_pending_proposals
			(id, group_id, proposer_did, proposal_b64, proposal_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'remove', ?, ?)`,
		proposalID, groupID, user.DID, req.CommitB64, req.NewEpoch, now,
	); err != nil {
		respond(w, 500, nil, "Proposal kaydedilemedi: "+err.Error())
		return
	}

	// Epoch ilerlet
	if _, err := tx.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		req.NewEpoch, now, groupID); err != nil {
		respond(w, 500, nil, "Epoch güncellenemedi: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	// Kalan üyelere commit broadcast (çıkarılan üye dahil değil — onun ratchet'i zaten geçersiz)
	rows, err := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ?`,
		groupID, user.DID)
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer rows.Close()
	var recipients []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			respond(w, 500, nil, "DB hatası: "+err.Error())
			return
		}
		recipients = append(recipients, d)
	}
	for _, d := range recipients {
		if messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsCommit, map[string]any{
				"group_id":      groupID,
				"commit_b64":    req.CommitB64,
				"epoch":         req.NewEpoch,
				"proposal_type": "remove",
				"removed_did":   targetDID,
				"proposer_did":  user.DID,
			})
		}
	}

	// Çıkarılan üyeye bildirim (isteğe bağlı — kendisi de bilsin)
	if messaging.GlobalHub.IsOnline(targetDID) {
		messaging.GlobalHub.SendTo(targetDID, messaging.MsgTypeMlsRemoved, map[string]any{
			"group_id":   groupID,
			"epoch":      req.NewEpoch,
			"removed_by": user.DID,
		})
	}

	respond(w, 200, map[string]any{
		"group_id":    groupID,
		"removed_did": targetDID,
		"new_epoch":   req.NewEpoch,
		"broadcast":   len(recipients),
	}, "")
}

// GET /v1/mls/group/{id}/state — HandleMLSGroupInfo'nun alias'ı.
// Spec Bölüm 17 EK B'de bu endpoint adı geçer; UI bunu çağırır.
func HandleMLSGroupState(w http.ResponseWriter, r *http.Request) {
	HandleMLSGroupInfo(w, r)
}

// GET /v1/mls/group/{id}/messages — fetch missed messages since epoch
func HandleMLSGroupMessages(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	sinceEpoch := r.URL.Query().Get("since_epoch")
	if sinceEpoch == "" {
		sinceEpoch = "0"
	}

	// Membership check
	var ok int
	err := db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&ok)
	if err == sql.ErrNoRows || ok == 0 {
		respond(w, 403, nil, "Grubun üyesi değilsiniz")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	// ORDER BY epoch ASC — MLS'te commit'ler sıkı sıralıdır (epoch N+1
	// uygulanmadan N+2 işlenemez). created_at tek başına yetmez: düğümler
	// arası saat kayması veya aynı saniyeye düşen kayıtlar sırayı bozar.
	// created_at yalnızca aynı epoch içindeki eşitlik bozucudur.
	rows, err := db.DB.Query(`
		SELECT id, sender_did, ciphertext_b64, content_type, epoch, created_at
		FROM mls_messages
		WHERE group_id = ? AND epoch >= ?
		ORDER BY epoch ASC, created_at ASC LIMIT 500`,
		groupID, sinceEpoch,
	)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	defer rows.Close()

	// content_type: 'application' (şifreli kullanıcı mesajı) veya 'commit'
	// (epoch geçişi). Çağıran ikisini ayırt edemeden aynı kuyruktan besleyemez.
	type m struct {
		ID            string `json:"id"`
		SenderDID     string `json:"sender_did"`
		CiphertextB64 string `json:"ciphertext_b64"`
		ContentType   string `json:"content_type"`
		Epoch         int64  `json:"epoch"`
		CreatedAt     string `json:"created_at"`
	}
	var out []m
	for rows.Next() {
		var x m
		if err := rows.Scan(&x.ID, &x.SenderDID, &x.CiphertextB64, &x.ContentType, &x.Epoch, &x.CreatedAt); err != nil {
			respond(w, 500, nil, err.Error())
			return
		}
		out = append(out, x)
	}

	respond(w, 200, map[string]any{
		"group_id": groupID,
		"messages": out,
	}, "")
}

// ─── Görev 2: Server-orchestrated remove + key update ────────────────────────
//
// Mevcut HandleMLSRemoveMember (DELETE /member/{did}) client'tan hazır commit
// ister. Aşağıdaki iki handler ise MLS_CLI_PATH yapılandırıldığında sunucunun
// kendi mls-cli üzerinden commit üretmesini sağlar — server-managed group
// senaryoları için (örn. moderation action: spam üyeyi kick'le).
//
// MLS_CLI_PATH yoksa fallback olarak client tarafından sağlanan commit_b64'ü
// kullanır — bu sayede her iki dağıtım modu da çalışır.

// MLSRemoveMemberV2Request — POST /v1/mls/group/{id}/remove
type MLSRemoveMemberV2Request struct {
	TargetDID string `json:"target_did"`
	// Opsiyonel: sunucuda mls-cli yoksa client kendi commit'ini gönderebilir.
	CommitB64 string `json:"commit_b64,omitempty"`
	NewEpoch  uint64 `json:"new_epoch,omitempty"`
	// Sunucunun mls-cli ile commit üretmesi için sender identity_id'si gerek.
	SenderIdentityID string `json:"sender_identity_id,omitempty"`
}

// POST /v1/mls/group/{id}/remove
// Sender grubun mevcut üyesi olmalı + (yönetici veya kendini çıkaran olmalı).
func HandleMlsRemoveMember(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}
	var req MLSRemoveMemberV2Request
	if err := decodeBody(r, &req); err != nil || req.TargetDID == "" {
		respond(w, 400, nil, "target_did zorunlu")
		return
	}

	// Yetki: sender üye olmalı
	var senderRole string
	err := db.DB.QueryRow(`SELECT role FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&senderRole)
	if err == sql.ErrNoRows {
		respond(w, 403, nil, "Sadece grup üyeleri remove yapabilir")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	// Kendini çıkarma serbest; başkasını creator çıkarabilir
	if user.DID != req.TargetDID && senderRole != "creator" {
		respond(w, 403, nil, "Başka üyeyi çıkarmak için creator yetkisi gerekli")
		return
	}

	// Hedef üye var mı?
	var exists int
	err = db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, req.TargetDID).Scan(&exists)
	if err == sql.ErrNoRows || exists == 0 {
		respond(w, 404, nil, "Hedef üye grupta değil")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	// Mevcut epoch'u oku
	var curEpoch int64
	if err := db.DB.QueryRow(`SELECT epoch FROM mls_groups WHERE id = ?`, groupID).Scan(&curEpoch); err != nil {
		respond(w, 500, nil, "Grup okunamadı: "+err.Error())
		return
	}

	commitB64 := req.CommitB64
	newEpoch := req.NewEpoch

	// MLS CLI varsa ve client commit göndermediyse server-side üret
	if commitB64 == "" {
		cli := mlspkg.Global()
		if cli == nil {
			respond(w, 400, nil, "commit_b64 zorunlu (MLS CLI sunucuda aktif değil)")
			return
		}
		if req.SenderIdentityID == "" {
			respond(w, 400, nil, "sender_identity_id zorunlu (server-side commit üretimi için)")
			return
		}
		callCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		out, err := cli.RemoveMember(callCtx, groupID, req.SenderIdentityID, req.TargetDID)
		if err != nil {
			respond(w, 500, nil, "MLS remove commit üretilemedi: "+err.Error())
			return
		}
		commitB64 = out.CommitB64
		newEpoch = out.Epoch
	}
	if newEpoch == 0 {
		newEpoch = uint64(curEpoch) + 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, req.TargetDID); err != nil {
		respond(w, 500, nil, "Üye silinemedi: "+err.Error())
		return
	}
	proposalID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO mls_pending_proposals
			(id, group_id, proposer_did, proposal_b64, proposal_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'remove', ?, ?)`,
		proposalID, groupID, user.DID, commitB64, newEpoch, now,
	); err != nil {
		respond(w, 500, nil, "Proposal kaydedilemedi: "+err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		newEpoch, now, groupID); err != nil {
		respond(w, 500, nil, "Epoch güncellenemedi: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	// Kalan üyelere broadcast
	recipients := groupMemberDIDs(groupID, user.DID)
	for _, d := range recipients {
		if messaging.GlobalHub != nil && messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsCommit, map[string]any{
				"group_id":      groupID,
				"commit_b64":    commitB64,
				"epoch":         newEpoch,
				"proposal_type": "remove",
				"removed_did":   req.TargetDID,
				"proposer_did":  user.DID,
			})
		}
	}
	// Çıkarılana ayrı bilgilendirme
	if messaging.GlobalHub != nil && messaging.GlobalHub.IsOnline(req.TargetDID) {
		messaging.GlobalHub.SendTo(req.TargetDID, messaging.MsgTypeMlsRemoved, map[string]any{
			"group_id":   groupID,
			"epoch":      newEpoch,
			"removed_by": user.DID,
		})
	}

	respond(w, 200, map[string]any{
		"group_id":    groupID,
		"removed_did": req.TargetDID,
		"new_epoch":   newEpoch,
		"proposal_id": proposalID,
		"broadcast":   len(recipients),
	}, "")
}

// MLSUpdateKeyRequest — POST /v1/mls/group/{id}/update-key
// Forward secrecy rotasyonu: leaf HPKE anahtarını yeniler.
type MLSUpdateKeyRequest struct {
	// Server-side mls-cli ile üretim için identity_id; aksi halde client
	// kendi commit'ini gönderebilir.
	IdentityID string `json:"identity_id,omitempty"`
	CommitB64  string `json:"commit_b64,omitempty"`
	NewEpoch   uint64 `json:"new_epoch,omitempty"`
}

// POST /v1/mls/group/{id}/update-key
func HandleMlsUpdateKey(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	vars := mux.Vars(r)
	groupID := vars["id"]
	if groupID == "" {
		respond(w, 400, nil, "group_id zorunlu")
		return
	}
	var req MLSUpdateKeyRequest
	_ = decodeBody(r, &req)

	// Yetki: sender üye olmalı
	var ok int
	err := db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&ok)
	if err == sql.ErrNoRows || ok == 0 {
		respond(w, 403, nil, "Sadece grup üyeleri key update yapabilir")
		return
	}
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}

	var curEpoch int64
	if err := db.DB.QueryRow(`SELECT epoch FROM mls_groups WHERE id = ?`, groupID).Scan(&curEpoch); err != nil {
		respond(w, 500, nil, "Grup okunamadı: "+err.Error())
		return
	}

	commitB64 := req.CommitB64
	newEpoch := req.NewEpoch

	if commitB64 == "" {
		cli := mlspkg.Global()
		if cli == nil {
			respond(w, 400, nil, "commit_b64 zorunlu (MLS CLI sunucuda aktif değil)")
			return
		}
		if req.IdentityID == "" {
			respond(w, 400, nil, "identity_id zorunlu (server-side commit üretimi için)")
			return
		}
		callCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		out, err := cli.UpdateKey(callCtx, groupID, req.IdentityID)
		if err != nil {
			respond(w, 500, nil, "MLS update commit üretilemedi: "+err.Error())
			return
		}
		commitB64 = out.CommitB64
		newEpoch = out.Epoch
	}
	if newEpoch == 0 {
		newEpoch = uint64(curEpoch) + 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.DB.Begin()
	if err != nil {
		respond(w, 500, nil, "DB hatası: "+err.Error())
		return
	}
	defer tx.Rollback()

	proposalID := uuid.New().String()
	if _, err := tx.Exec(`
		INSERT INTO mls_pending_proposals
			(id, group_id, proposer_did, proposal_b64, proposal_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'update', ?, ?)`,
		proposalID, groupID, user.DID, commitB64, newEpoch, now,
	); err != nil {
		respond(w, 500, nil, "Proposal kaydedilemedi: "+err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		newEpoch, now, groupID); err != nil {
		respond(w, 500, nil, "Epoch güncellenemedi: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		respond(w, 500, nil, "DB commit hatası: "+err.Error())
		return
	}

	// Tüm diğer üyelere broadcast
	recipients := groupMemberDIDs(groupID, user.DID)
	for _, d := range recipients {
		if messaging.GlobalHub != nil && messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, messaging.MsgTypeMlsCommit, map[string]any{
				"group_id":      groupID,
				"commit_b64":    commitB64,
				"epoch":         newEpoch,
				"proposal_type": "update",
				"proposer_did":  user.DID,
			})
		}
	}

	respond(w, 200, map[string]any{
		"group_id":      groupID,
		"new_epoch":     newEpoch,
		"proposal_id":   proposalID,
		"proposal_type": "update",
		"broadcast":     len(recipients),
	}, "")
}

// groupMemberDIDs — yardımcı: belirli grubun, gönderen hariç, tüm üye DID'leri.
// Hata sessizce yutar (broadcast best-effort).
func groupMemberDIDs(groupID, excludeDID string) []string {
	rows, err := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ?`,
		groupID, excludeDID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil {
			out = append(out, d)
		}
	}
	return out
}
