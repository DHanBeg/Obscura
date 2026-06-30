package api

// Kişi rehberi — GET/POST/DELETE /v1/contacts
//
// Bir kullanıcı başka bir kullanıcıyı rehberine ekleyebilir.
// Rehber tamamen kişisel; karşı tarafın haberi olmaz.

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
)

type contactRow struct {
	DID         string `json:"did"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	Tier        int    `json:"tier"`
	Nickname    string `json:"nickname"`
	AddedAt     string `json:"added_at"`
}

// GET /v1/contacts — kişi listesi
func HandleGetContacts(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	rows, err := db.DB.Query(`
		SELECT u.did, COALESCE(u.display_name,''), COALESCE(u.username,''),
		       COALESCE(u.avatar_url,''), u.tier, COALESCE(c.nickname,''), c.created_at
		FROM contacts c
		JOIN users u ON u.did = c.contact_did
		WHERE c.owner_did = ?
		ORDER BY c.created_at DESC
	`, user.DID)
	if err != nil {
		respond(w, 500, nil, "Kişiler alınamadı")
		return
	}
	defer rows.Close()

	contacts := []contactRow{}
	for rows.Next() {
		var cr contactRow
		if err := rows.Scan(&cr.DID, &cr.DisplayName, &cr.Username,
			&cr.AvatarURL, &cr.Tier, &cr.Nickname, &cr.AddedAt); err == nil {
			contacts = append(contacts, cr)
		}
	}

	respond(w, 200, map[string]interface{}{"contacts": contacts}, "")
}

type addContactRequest struct {
	DID      string `json:"did"`
	Nickname string `json:"nickname"`
}

// POST /v1/contacts — kişi ekle
func HandleAddContact(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req addContactRequest
	if err := decodeBody(r, &req); err != nil || req.DID == "" {
		respond(w, 400, nil, "did zorunlu")
		return
	}
	if req.DID == user.DID {
		respond(w, 400, nil, "Kendinizi ekleyemezsiniz")
		return
	}

	// Hedef kullanıcı var mı?
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE did = ? AND is_active = 1 AND is_banned = 0", req.DID).Scan(&exists)
	if exists == 0 {
		respond(w, 404, nil, "Kullanıcı bulunamadı")
		return
	}

	id := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	_, err := db.DB.Exec(`
		INSERT OR IGNORE INTO contacts (id, owner_did, contact_did, nickname, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, user.DID, req.DID, req.Nickname, now,
	)
	if err != nil {
		respond(w, 500, nil, "Kişi eklenemedi")
		return
	}

	respond(w, 201, map[string]string{"id": id, "did": req.DID}, "")
}

// DELETE /v1/contacts/{did} — kişi sil
func HandleRemoveContact(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	contactDID := mux.Vars(r)["did"]
	if contactDID == "" {
		respond(w, 400, nil, "did zorunlu")
		return
	}

	db.DB.Exec("DELETE FROM contacts WHERE owner_did = ? AND contact_did = ?", user.DID, contactDID)
	respond(w, 200, map[string]bool{"removed": true}, "")
}
