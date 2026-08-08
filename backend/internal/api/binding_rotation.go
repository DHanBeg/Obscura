package api

// Binding rotation — timelocked DID migration (ADR-0009 N3).
//
// Prevents permanent account loss when a mnemonic is compromised: attacker with
// temporary access cannot immediately lock out the original user because there is
// a mandatory 7-day window during which the original user can cancel.
//
// Flow:
//   POST /v1/identity/rotation/request  — initiates rotation (auth: old DID)
//   DELETE /v1/identity/rotation/{id}   — cancels pending rotation (auth: old DID)
//   POST /v1/identity/rotation/{id}/confirm — applies rotation after timelock (auth: old DID)

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
)

const rotationTimelockDays = 7

// rotationDIDRegex matches did:obs:<32 hex chars> — kanonik format,
// auth.GenerateDID/mobile sealed-sender.ts:didFromDhPublic ile aynı
// (SHA256(identity_key)[:16 byte] hex, bkz. #23 DID şema doğrulaması).
var rotationDIDRegex = regexp.MustCompile(`^did:obs:[0-9a-f]{32}$`)

type RotationRequestBody struct {
	NewDID      string `json:"new_did"`
	NewDIDProof string `json:"new_did_proof_b64,omitempty"` // ZK proof of new identity (optional for now)
}

// POST /v1/identity/rotation/request
func HandleRotationRequest(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req RotationRequestBody
	if err := decodeBody(r, &req); err != nil || req.NewDID == "" {
		respond(w, 400, nil, "new_did zorunlu")
		return
	}
	if !rotationDIDRegex.MatchString(req.NewDID) {
		respond(w, 400, nil, "Geçersiz new_did formatı (did:obs:<32 hex> bekleniyor)")
		return
	}
	if req.NewDID == user.DID {
		respond(w, 400, nil, "new_did mevcut DID'den farklı olmalı")
		return
	}

	// Only one pending rotation per user at a time.
	var existingID string
	_ = db.DB.QueryRow(`SELECT id FROM binding_rotations WHERE old_did = ? AND status = 'pending'`, user.DID).Scan(&existingID)
	if existingID != "" {
		respond(w, 409, map[string]any{"existing_rotation_id": existingID}, "Bekleyen bir rotasyon zaten mevcut")
		return
	}

	rotationID := uuid.New().String()
	now := time.Now().UTC()
	unlockAt := now.Add(rotationTimelockDays * 24 * time.Hour)

	if _, err := db.DB.Exec(`
		INSERT INTO binding_rotations (id, old_did, new_did, new_did_proof, requested_at, unlock_at, status)
		VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
		rotationID, user.DID, req.NewDID, req.NewDIDProof,
		now.Format(time.RFC3339), unlockAt.Format(time.RFC3339),
	); err != nil {
		respond(w, 500, nil, "Rotasyon oluşturulamadı: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"rotation_id": rotationID,
		"old_did":     user.DID,
		"new_did":     req.NewDID,
		"unlock_at":   unlockAt.Format(time.RFC3339),
		"status":      "pending",
		"note":        fmt.Sprintf("%d günlük bekleme süresi dolmadan önce /rotation/%s ile iptal edebilirsiniz.", rotationTimelockDays, rotationID),
	}, "")
}

// DELETE /v1/identity/rotation/{id}
// Cancels a pending rotation within the timelock window.
func HandleRotationCancel(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	rotationID := mux.Vars(r)["id"]
	if rotationID == "" {
		respond(w, 400, nil, "rotation_id zorunlu")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.DB.Exec(`
		UPDATE binding_rotations SET status = 'cancelled', cancelled_at = ?
		WHERE id = ? AND old_did = ? AND status = 'pending'`,
		now, rotationID, user.DID,
	)
	if err != nil {
		respond(w, 500, nil, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		respond(w, 404, nil, "Rotasyon bulunamadı veya iptal edilemez durumda")
		return
	}
	respond(w, 200, map[string]any{"cancelled": true, "rotation_id": rotationID}, "")
}

// POST /v1/identity/rotation/{id}/confirm
// Applies the rotation after the timelock has passed.
// The user must still authenticate as the OLD DID (they still control that mnemonic).
// Server marks the rotation as applied; the new DID can then be registered separately.
func HandleRotationConfirm(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}
	rotationID := mux.Vars(r)["id"]
	if rotationID == "" {
		respond(w, 400, nil, "rotation_id zorunlu")
		return
	}

	var oldDID, newDID, unlockAtStr, status string
	err := db.DB.QueryRow(`
		SELECT old_did, new_did, unlock_at, status FROM binding_rotations WHERE id = ?`, rotationID,
	).Scan(&oldDID, &newDID, &unlockAtStr, &status)
	if err != nil {
		respond(w, 404, nil, "Rotasyon bulunamadı")
		return
	}
	if oldDID != user.DID {
		respond(w, 403, nil, "Bu rotasyon başka bir kullanıcıya ait")
		return
	}
	if status != "pending" {
		respond(w, 400, nil, "Rotasyon durumu: "+status+" (sadece 'pending' onaylanabilir)")
		return
	}

	unlockAt, err := time.Parse(time.RFC3339, unlockAtStr)
	if err != nil {
		respond(w, 500, nil, "Timelock tarihi okunamadı")
		return
	}
	if time.Now().UTC().Before(unlockAt) {
		remaining := time.Until(unlockAt).Round(time.Hour)
		respond(w, 400, nil, fmt.Sprintf("Timelock henüz dolmadı — yaklaşık %s kaldı (unlock: %s)", remaining, unlockAt.Format(time.RFC3339)))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.Exec(`
		UPDATE binding_rotations SET status = 'applied', applied_at = ? WHERE id = ?`, now, rotationID,
	); err != nil {
		respond(w, 500, nil, "Rotasyon uygulanamadı: "+err.Error())
		return
	}

	respond(w, 200, map[string]any{
		"applied":     true,
		"rotation_id": rotationID,
		"old_did":     oldDID,
		"new_did":     newDID,
		"applied_at":  now,
		"note":        "Eski DID korunmaktadır. Yeni DID ile kayıt olabilirsiniz.",
	}, "")
}
