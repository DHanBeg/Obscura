// signal_handlers.go — HTTP handlers for Signal Protocol session state and
// X3DH prekey distribution.
//
// Routes (all protected by AuthMiddleware unless noted):
//   POST   /v1/signal/session         — save / update session state blob
//   GET    /v1/signal/session/{did}   — retrieve session state blob for peer DID
//   DELETE /v1/signal/session/{did}   — delete session
//   GET    /v1/signal/sessions        — list all sessions for the authenticated user
//   GET    /v1/signal/prekey/{did}    — X3DH prekey bundle (claims one OPK)
//
// Security contract:
//   The backend is E2EE-blind. It stores ciphertext and opaque state blobs;
//   it never decrypts, never derives session keys, never inspects Double Ratchet
//   internals (Spec Bölüm 4.5 rule 2 — "şifreleme client'ta").
package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/signal"
)

// signalStore is the package-level singleton; initialised by InitSignalStore.
var signalStore *signal.SessionStore

// InitSignalStore must be called once from main after db.Init.
func InitSignalStore() {
	signalStore = signal.NewSessionStore(db.DB)
}

// ─── POST /v1/signal/session ─────────────────────────────────────────────────

// HandleSignalSaveSession stores or updates an opaque Double Ratchet state
// blob for the authenticated user ↔ peer pair.
func HandleSignalSaveSession(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req struct {
		PeerDID      string `json:"peer_did"`
		SessionState string `json:"session_state"` // base64-encoded, client-encrypted blob
	}
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.PeerDID == "" {
		respond(w, 400, nil, "peer_did zorunlu")
		return
	}
	if req.SessionState == "" {
		respond(w, 400, nil, "session_state zorunlu")
		return
	}

	if err := signalStore.SaveSession(user.DID, req.PeerDID, req.SessionState); err != nil {
		log.Printf("signal.SaveSession hata (owner=%s peer=%s): %v", user.DID, req.PeerDID, err)
		respond(w, 500, nil, "Session state kaydedilemedi")
		return
	}

	respond(w, 200, map[string]string{"status": "saved"}, "")
}

// ─── GET /v1/signal/session/{did} ────────────────────────────────────────────

// HandleSignalGetSession retrieves the opaque state blob for the session
// between the authenticated user and the given peer DID.
func HandleSignalGetSession(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	peerDID := mux.Vars(r)["did"]
	if peerDID == "" {
		respond(w, 400, nil, "peer DID eksik")
		return
	}

	stateB64, err := signalStore.GetSession(user.DID, peerDID)
	if err != nil {
		if err.Error() == "signal.GetSession: sql: no rows in result set" {
			respond(w, 404, nil, "Session bulunamadı")
			return
		}
		// unwrapped ErrNoRows check
		if isNoRows(err) {
			respond(w, 404, nil, "Session bulunamadı")
			return
		}
		log.Printf("signal.GetSession hata (owner=%s peer=%s): %v", user.DID, peerDID, err)
		respond(w, 500, nil, "Session alınamadı")
		return
	}

	respond(w, 200, map[string]string{
		"peer_did":      peerDID,
		"session_state": stateB64,
	}, "")
}

// ─── DELETE /v1/signal/session/{did} ─────────────────────────────────────────

// HandleSignalDeleteSession removes a session.  Idempotent.
func HandleSignalDeleteSession(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	peerDID := mux.Vars(r)["did"]
	if peerDID == "" {
		respond(w, 400, nil, "peer DID eksik")
		return
	}

	if err := signalStore.DeleteSession(user.DID, peerDID); err != nil {
		log.Printf("signal.DeleteSession hata (owner=%s peer=%s): %v", user.DID, peerDID, err)
		respond(w, 500, nil, "Session silinemedi")
		return
	}

	respond(w, 200, map[string]string{"status": "deleted"}, "")
}

// ─── GET /v1/signal/sessions ─────────────────────────────────────────────────

// HandleSignalListSessions lists all session metadata (no state blobs) for
// the authenticated user.
func HandleSignalListSessions(w http.ResponseWriter, r *http.Request) {
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	sessions, err := signalStore.ListSessions(user.DID)
	if err != nil {
		log.Printf("signal.ListSessions hata (owner=%s): %v", user.DID, err)
		respond(w, 500, nil, "Session listesi alınamadı")
		return
	}

	respond(w, 200, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}, "")
}

// ─── GET /v1/signal/prekey/{did} ─────────────────────────────────────────────

// HandleSignalGetPrekey returns the X3DH prekey bundle for a given DID,
// atomically claiming one OPK from the pool.
func HandleSignalGetPrekey(w http.ResponseWriter, r *http.Request) {
	// Auth required: prevents anonymous enumeration / OPK exhaustion attacks.
	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	targetDID := mux.Vars(r)["did"]
	if targetDID == "" {
		respond(w, 400, nil, "DID eksik")
		return
	}

	bundle, err := signalStore.GetPrekeyBundle(targetDID)
	if err != nil {
		// GetPrekeyBundle wraps sql.ErrNoRows with context text.
		if isNoRows(err) {
			respond(w, 404, nil, "Bu DID için prekey bundle bulunamadı")
			return
		}
		log.Printf("signal.GetPrekeyBundle hata (did=%s): %v", targetDID, err)
		respond(w, 500, nil, "Prekey bundle alınamadı")
		return
	}

	respond(w, 200, bundle, "")
}

// isNoRows returns true if err wraps or equals sql.ErrNoRows.
func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	if err == sql.ErrNoRows {
		return true
	}
	// errors.Is traversal for wrapped errors.
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return isNoRows(u.Unwrap())
	}
	return false
}
