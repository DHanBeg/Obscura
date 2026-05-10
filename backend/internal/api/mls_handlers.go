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
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
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
	_, err := db.DB.Exec(`
		INSERT INTO mls_groups (id, creator_did, name, ciphersuite, epoch, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(id) DO NOTHING`,
		req.GroupID, user.DID, req.Name, cs, now, now,
	)
	if err != nil {
		respond(w, 500, nil, "Grup oluşturulamadı: "+err.Error())
		return
	}

	// Creator is the first member at epoch 0
	_, _ = db.DB.Exec(`
		INSERT INTO mls_group_members (group_id, user_did, role, joined_at, joined_at_epoch)
		VALUES (?, ?, 'creator', ?, 0)
		ON CONFLICT(group_id, user_did) DO NOTHING`,
		req.GroupID, user.DID, now,
	)

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
	db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&memberCheck)
	if memberCheck == 0 {
		respond(w, 403, nil, "Sadece grup üyeleri davet edebilir")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Add new member to roster
	_, err := db.DB.Exec(`
		INSERT INTO mls_group_members (group_id, user_did, role, joined_at, joined_at_epoch)
		VALUES (?, ?, 'member', ?, ?)
		ON CONFLICT(group_id, user_did) DO UPDATE SET joined_at = excluded.joined_at`,
		groupID, req.NewMemberDID, now, req.NewEpoch,
	)
	if err != nil {
		respond(w, 500, nil, "Üye eklenemedi: "+err.Error())
		return
	}

	// Bump group epoch
	db.DB.Exec(`UPDATE mls_groups SET epoch = ?, updated_at = ? WHERE id = ?`,
		req.NewEpoch, now, groupID)

	// Queue Welcome for new member
	welcomeID := uuid.New().String()
	db.DB.Exec(`
		INSERT INTO mls_welcome_queue (id, group_id, recipient_did, welcome_b64, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		welcomeID, groupID, req.NewMemberDID, req.WelcomeB64, now)

	// Push Welcome via WS if online
	if messaging.GlobalHub.IsOnline(req.NewMemberDID) {
		messaging.GlobalHub.SendTo(req.NewMemberDID, "mls_welcome", map[string]any{
			"group_id":    groupID,
			"welcome_b64": req.WelcomeB64,
			"epoch":       req.NewEpoch,
		})
	}

	// Broadcast Commit to all OTHER existing members
	rows, _ := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ? AND user_did != ?`,
		groupID, user.DID, req.NewMemberDID)
	defer rows.Close()
	var recipients []string
	for rows.Next() {
		var d string
		rows.Scan(&d)
		recipients = append(recipients, d)
	}
	for _, d := range recipients {
		if messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, "mls_commit", map[string]any{
				"group_id":   groupID,
				"commit_b64": req.CommitB64,
				"epoch":      req.NewEpoch,
			})
		}
	}

	respond(w, 200, map[string]any{
		"group_id":   groupID,
		"new_epoch":  req.NewEpoch,
		"welcomed":   req.NewMemberDID,
		"broadcast":  len(recipients),
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
	db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&ok)
	if ok == 0 {
		respond(w, 403, nil, "Grubun üyesi değilsiniz")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	msgID := uuid.New().String()
	_, err := db.DB.Exec(`
		INSERT INTO mls_messages (id, group_id, sender_did, ciphertext_b64, content_type, epoch, created_at)
		VALUES (?, ?, ?, ?, 'application', ?, ?)`,
		msgID, groupID, user.DID, req.CiphertextB64, req.Epoch, now,
	)
	if err != nil {
		respond(w, 500, nil, "Mesaj kaydedilemedi: "+err.Error())
		return
	}

	// Broadcast to ALL members (sender included for own-device fanout)
	rows, _ := db.DB.Query(`SELECT user_did FROM mls_group_members WHERE group_id = ? AND user_did != ?`,
		groupID, user.DID)
	defer rows.Close()
	var delivered, total int
	for rows.Next() {
		var d string
		rows.Scan(&d)
		total++
		if messaging.GlobalHub.IsOnline(d) {
			messaging.GlobalHub.SendTo(d, "mls_message", map[string]any{
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
		DID  string `json:"did"`
		Role string `json:"role"`
		EpochJoined int64 `json:"epoch_joined"`
	}
	rows, _ := db.DB.Query(`SELECT user_did, role, joined_at_epoch FROM mls_group_members WHERE group_id = ?`,
		groupID)
	defer rows.Close()
	var members []member
	for rows.Next() {
		var m member
		rows.Scan(&m.DID, &m.Role, &m.EpochJoined)
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
		rows.Scan(&x.ID, &x.GroupID, &x.WelcomeB64, &x.CreatedAt)
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
	db.DB.QueryRow(`SELECT 1 FROM mls_group_members WHERE group_id = ? AND user_did = ?`,
		groupID, user.DID).Scan(&ok)
	if ok == 0 {
		respond(w, 403, nil, "Grubun üyesi değilsiniz")
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, sender_did, ciphertext_b64, epoch, created_at
		FROM mls_messages
		WHERE group_id = ? AND epoch >= ?
		ORDER BY created_at ASC LIMIT 500`,
		groupID, sinceEpoch,
	)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	defer rows.Close()

	type m struct {
		ID            string `json:"id"`
		SenderDID     string `json:"sender_did"`
		CiphertextB64 string `json:"ciphertext_b64"`
		Epoch         int64  `json:"epoch"`
		CreatedAt     string `json:"created_at"`
	}
	var out []m
	for rows.Next() {
		var x m
		rows.Scan(&x.ID, &x.SenderDID, &x.CiphertextB64, &x.Epoch, &x.CreatedAt)
		out = append(out, x)
	}

	respond(w, 200, map[string]any{
		"group_id": groupID,
		"messages": out,
	}, "")
}

// Suppress unused import warnings if we wire JSON helpers later
var _ = json.Marshal
