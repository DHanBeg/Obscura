// Package api — WebRTC call signalling handlers.
//
// POST /v1/call/invite  — WebRTC arama daveti gönder
// POST /v1/call/answer  — Aramayı kabul et veya reddet
// POST /v1/call/end     — Aramayı sonlandır
//
// Spec Bölüm 9 (P2P VoIP) — SDP + ICE paylaşımı sunucu üzerinden yapılır;
// medya akışı doğrudan WebRTC peer-to-peer taşınır.
//
// Güvenlik notu: Bu endpoint'ler JWT auth middleware tarafından korunur.
// call_id UUID sunucu tarafında üretilir; client tarafından kabul edilmez.
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/messaging"
	"obscura.network/core/internal/push"
)

// ─── İSTEK / YANITLAR ────────────────────────────────────────────────────────

type callInviteRequest struct {
	ToDID         string   `json:"to_did"`
	SDPOffer      string   `json:"sdp_offer"`
	ICECandidates []string `json:"ice_candidates"`
	CallType      string   `json:"call_type"` // "audio" | "video"
}

type callAnswerRequest struct {
	CallID        string   `json:"call_id"`
	SDPAnswer     string   `json:"sdp_answer"`
	ICECandidates []string `json:"ice_candidates"`
	Accepted      bool     `json:"accepted"`
}

type callEndRequest struct {
	CallID string `json:"call_id"`
	Reason string `json:"reason"` // "hung_up" | "busy" | "no_answer" | "network_error"
}

// ─── POST /v1/call/invite ─────────────────────────────────────────────────────

// HandleCallInvite — arayan taraf SDP offer + ICE adayları ile davet gönderir.
// Alıcı online ise WebSocket call_invite mesajı; offline ise FCM VoIP push.
func HandleCallInvite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req callInviteRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if req.ToDID == "" {
		respond(w, 400, nil, "to_did zorunlu")
		return
	}
	if req.SDPOffer == "" {
		respond(w, 400, nil, "sdp_offer zorunlu")
		return
	}
	if req.ToDID == user.DID {
		respond(w, 400, nil, "Kendinizi arayamazsınız")
		return
	}

	// Alıcının sistemde kayıtlı olup olmadığını kontrol et
	var recipientExists int
	db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE did = ? AND is_active = 1 AND is_banned = 0",
		req.ToDID).Scan(&recipientExists)
	if recipientExists == 0 {
		respond(w, 404, nil, "Kullanıcı bulunamadı")
		return
	}

	callID := uuid.New().String()
	now := time.Now()

	// Aktif aramayı DB'ye kaydet (durum takibi için)
	_, dbErr := db.DB.Exec(`
		INSERT OR IGNORE INTO active_calls
		    (id, caller_did, callee_did, status, sdp_offer, created_at, updated_at)
		VALUES (?, ?, ?, 'ringing', ?, ?, ?)`,
		callID, user.DID, req.ToDID, req.SDPOffer,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if dbErr != nil {
		// active_calls tablosu yoksa (migration eksikse) sessizce devam et —
		// WebSocket sinyalizasyonu yine de çalışır.
		log.Printf("call/invite: active_calls insert atlandı: %v", dbErr)
	}

	callType := req.CallType
	if callType == "" {
		callType = "audio"
	}

	callPayload := map[string]interface{}{
		"call_id":        callID,
		"caller_did":     user.DID,
		"caller_name":    user.DisplayName,
		"sdp_offer":      req.SDPOffer,
		"ice_candidates": req.ICECandidates,
		"call_type":      callType,
		"created_at":     now.Unix(),
	}

	// Alıcı online mu? → WebSocket call_invite
	if messaging.GlobalHub.IsOnline(req.ToDID) {
		messaging.GlobalHub.SendTo(req.ToDID, "call_invite", callPayload)
		log.Printf("call/invite: WebSocket iletildi call_id=%s → %s", callID, shortDIDStr(req.ToDID))
	} else {
		// Offline → FCM VoIP push
		go func() {
			var fcmToken string
			db.DB.QueryRow("SELECT COALESCE(fcm_token,'') FROM users WHERE did = ?", req.ToDID).
				Scan(&fcmToken)
			if fcmToken == "" {
				return
			}
			callerName := user.DisplayName
			if callerName == "" {
				callerName = user.Username
			}
			pushMsg := push.IncomingCall(callerName, callID)
			if err := push.Default.Send(context.Background(), fcmToken, pushMsg); err != nil {
				log.Printf("call/invite: push hatası call_id=%s: %v", callID, err)
			}
		}()
		log.Printf("call/invite: push gönderildi call_id=%s → %s (offline)", callID, shortDIDStr(req.ToDID))
	}

	respond(w, 200, map[string]interface{}{
		"call_id": callID,
		"status":  "ringing",
	}, "")
}

// ─── POST /v1/call/answer ─────────────────────────────────────────────────────

// HandleCallAnswer — aranan taraf daveti kabul veya reddeder.
// accepted=true → SDP answer arayana iletilir.
// accepted=false → call_end arayana iletilir.
func HandleCallAnswer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req callAnswerRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if req.CallID == "" {
		respond(w, 400, nil, "call_id zorunlu")
		return
	}

	// Arayanın DID'ini al — yalnızca bu çağrıdaki callee cevap verebilir
	var callerDID, calleeDID, currentStatus string
	err := db.DB.QueryRow(
		"SELECT caller_did, callee_did, status FROM active_calls WHERE id = ?",
		req.CallID,
	).Scan(&callerDID, &calleeDID, &currentStatus)

	// Tablo yoksa veya kayıt bulunamazsa WebSocket üzerinden devam et
	callNotFound := err != nil
	if !callNotFound && calleeDID != user.DID {
		respond(w, 403, nil, "Bu aramayı cevaplayamazsınız")
		return
	}
	if !callNotFound && currentStatus != "ringing" {
		respond(w, 409, nil, fmt.Sprintf("Arama zaten sonlandı: %s", currentStatus))
		return
	}

	if !req.Accepted {
		// Ret — arayana call_end bildir
		endPayload := map[string]interface{}{
			"call_id": req.CallID,
			"reason":  "rejected",
			"ended_at": time.Now().Unix(),
		}
		messaging.GlobalHub.SendTo(callerDID, "call_end", endPayload)

		// DB güncelle
		db.DB.Exec("UPDATE active_calls SET status = 'rejected', updated_at = ? WHERE id = ?",
			time.Now().Format(time.RFC3339), req.CallID)

		log.Printf("call/answer: reddedildi call_id=%s callee=%s", req.CallID, shortDIDStr(user.DID))
		respond(w, 200, map[string]interface{}{
			"call_id": req.CallID,
			"status":  "rejected",
		}, "")
		return
	}

	// Kabul — SDP answer + ICE arayana ilet
	if req.SDPAnswer == "" {
		respond(w, 400, nil, "accepted=true iken sdp_answer zorunlu")
		return
	}

	answerPayload := map[string]interface{}{
		"call_id":        req.CallID,
		"callee_did":     user.DID,
		"sdp_answer":     req.SDPAnswer,
		"ice_candidates": req.ICECandidates,
		"answered_at":    time.Now().Unix(),
	}

	messaging.GlobalHub.SendTo(callerDID, "call_answer", answerPayload)

	// DB güncelle
	db.DB.Exec(
		"UPDATE active_calls SET status = 'connected', sdp_answer = ?, updated_at = ? WHERE id = ?",
		req.SDPAnswer, time.Now().Format(time.RFC3339), req.CallID,
	)

	log.Printf("call/answer: kabul edildi call_id=%s callee=%s → caller=%s",
		req.CallID, shortDIDStr(user.DID), shortDIDStr(callerDID))

	respond(w, 200, map[string]interface{}{
		"call_id": req.CallID,
		"status":  "connected",
	}, "")
}

// ─── POST /v1/call/end ───────────────────────────────────────────────────────

// HandleCallEnd — taraflardan biri aramayı sonlandırır.
// Her iki tarafa da call_end WebSocket mesajı gönderilir.
func HandleCallEnd(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	var req callEndRequest
	if err := decodeBody(r, &req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}

	if req.CallID == "" {
		respond(w, 400, nil, "call_id zorunlu")
		return
	}

	reason := req.Reason
	if reason == "" {
		reason = "hung_up"
	}

	// Arama taraflarını bul
	var callerDID, calleeDID string
	db.DB.QueryRow("SELECT caller_did, callee_did FROM active_calls WHERE id = ?",
		req.CallID).Scan(&callerDID, &calleeDID)

	// Arayan veya aranan olmalı (kayıt yoksa yine de WS mesajı gönder)
	if callerDID != "" && calleeDID != "" {
		if user.DID != callerDID && user.DID != calleeDID {
			respond(w, 403, nil, "Bu aramayı sonlandıramazsınız")
			return
		}
	}

	endPayload := map[string]interface{}{
		"call_id":  req.CallID,
		"ended_by": user.DID,
		"reason":   reason,
		"ended_at": time.Now().Unix(),
	}

	// Her iki tarafa ilet (online olmayanlar sessizce atlanır)
	otherDID := callerDID
	if user.DID == callerDID {
		otherDID = calleeDID
	}
	if otherDID != "" && otherDID != user.DID {
		messaging.GlobalHub.SendTo(otherDID, "call_end", endPayload)
	}
	// Kendine de bildir (çok cihaz senaryosu)
	messaging.GlobalHub.SendTo(user.DID, "call_end", endPayload)

	// DB güncelle
	db.DB.Exec("UPDATE active_calls SET status = 'ended', updated_at = ? WHERE id = ?",
		time.Now().Format(time.RFC3339), req.CallID)

	log.Printf("call/end: call_id=%s reason=%s by=%s", req.CallID, reason, shortDIDStr(user.DID))

	respond(w, 200, map[string]interface{}{
		"call_id": req.CallID,
		"status":  "ended",
		"reason":  reason,
	}, "")
}
