// Package api — Mesaj geri alma handler'ı (Spec Bölüm 6.6)
//
// ROUTE: POST /v1/messages/{id}/recall
//   → HandleGlobalRecall — RequireAuth middleware gerekli
//
// Geri alma akışı:
//  1. JWT auth + istek sahibi kontrolü (sadece mesaj göndericisi geri alabilir)
//  2. İsteğe bağlı ZK silme kanıtı JSON body'den alınır
//  3. DB: deleted_at + recall_proof (hash) set edilir
//  4. Yerel WebSocket hub üzerinden tüm alıcılara "recall_confirm" broadcast
//  5. Gossip relay: tüm peer node'lara "shard-delete" isteği gönderilir
//     (POST /v1/internal/shard-delete — spec Bölüm 6.6)
//
// İstekler şu gövde yapısını bekler:
//
//	{ "zk_proof": { ... } }   // opsiyonel
//
// Yanıt:
//
//	{ "success": true, "data": { "message_id": "...", "status": "recalled" } }
package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/gossip"
	"obscura.network/core/internal/messaging"
)

// recallRequest — POST /v1/messages/{id}/recall gövdesi
type recallRequest struct {
	// ZKProof: gönderici tarafından oluşturulan silme kanıtı (opsiyonel).
	// Sunucu kanıtı doğrulamaz; SHA256 hash'ini recall_proof kolonuna yazar.
	// Alıcılar ve üçüncü taraflar kanıtı bağımsız olarak doğrulayabilir.
	ZKProof interface{} `json:"zk_proof,omitempty"`
}

// HandleGlobalRecall — POST /v1/messages/{id}/recall
//
// Auth: JWT (RequireAuth middleware)
// Body: { "zk_proof": { ... } }  (opsiyonel)
func HandleGlobalRecall(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	user := getUser(r)
	if user == nil {
		respond(w, 401, nil, "Yetkisiz")
		return
	}

	vars := mux.Vars(r)
	msgID := vars["id"]
	if msgID == "" {
		respond(w, 400, nil, "Mesaj ID gerekli")
		return
	}

	// ── Mesaj sahipliği kontrolü ──────────────────────────────────────────────
	// from_did, to_did ve conv_id çekiyoruz; to_did grup ise tüm üyelere bildirim.
	var fromDID, toDID, convID string
	var isGroup bool
	err := db.DB.QueryRow(`
		SELECT from_did, to_did, conv_id, is_group
		FROM   messages
		WHERE  id = ? AND deleted_at IS NULL
	`, msgID).Scan(&fromDID, &toDID, &convID, &isGroup)
	if err != nil {
		// Bulunamadı veya zaten silinmiş
		respond(w, 404, nil, "Mesaj bulunamadı")
		return
	}

	if fromDID != user.DID {
		respond(w, 403, nil, "Yalnızca mesaj sahibi geri alabilir")
		return
	}

	// ── Gövde ayrıştırma (opsiyonel ZK proof) ────────────────────────────────
	var req recallRequest
	// Body boş olabilir; hata yoksay.
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	// ZK proof hash'ini hesapla (audit trail)
	proofHash := ""
	if req.ZKProof != nil {
		proofBytes, marshalErr := json.Marshal(req.ZKProof)
		if marshalErr == nil && len(proofBytes) > 2 {
			sum := sha256.Sum256(proofBytes)
			proofHash = fmt.Sprintf("%x", sum)
		}
	}

	// ── DB güncelle ───────────────────────────────────────────────────────────
	now := time.Now().UTC()
	// status = 'recalled' — geri çağrılan mesaj için doğru durum. Önceden
	// 'failed' yazılıyordu; bu yanlış sinyaldi (mesaj iletilememiş gibi
	// görünüyordu). API yanıtı zaten "recalled" döndürdüğünden DB ile tutarlı
	// olması için 'recalled' kullanıyoruz.
	res, dbErr := db.DB.Exec(`
		UPDATE messages
		SET    deleted_at   = ?,
		       recall_proof = ?,
		       ciphertext   = '[Geri alındı]',
		       status       = 'recalled'
		WHERE  id = ?
		  AND  deleted_at IS NULL
	`, now.Format(time.RFC3339), proofHash, msgID)
	if dbErr != nil {
		log.Printf("[recall] DB güncelleme hatası (msg=%s): %v", msgID, dbErr)
		respond(w, 500, nil, "Geri alma kaydedilemedi")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Eş zamanlı istek mesajı zaten silmiş olabilir
		respond(w, 409, nil, "Mesaj zaten silinmiş veya geri alınmış")
		return
	}

	// ── WebSocket broadcast ───────────────────────────────────────────────────
	confirmPayload := messaging.RecallConfirmPayload{
		MessageID:       msgID,
		ConvID:          convID,
		RecalledBy:      user.DID,
		RecallProofHash: proofHash,
		Timestamp:       now.Unix(),
	}

	if isGroup {
		// Grup: tüm üyelere broadcast
		broadcastRecallToGroup(convID, confirmPayload)
	} else {
		// 1-1: alıcı + gönderici (gönderici online ise kendi UI'ını da günceller)
		messaging.GlobalHub.SendTo(toDID, messaging.MsgTypeRecallConfirm, confirmPayload)
		messaging.GlobalHub.SendTo(fromDID, messaging.MsgTypeRecallConfirm, confirmPayload)
	}

	// ── Gossip relay: shard silme ─────────────────────────────────────────────
	// Spek Bölüm 6.6: diğer node'lardaki shard'ları sil.
	// Hata durumunda recall işlemini geri almıyoruz; eventual consistency yeterli.
	go gossipShardDelete(msgID, convID, user.DID, proofHash)

	log.Printf("[recall] Mesaj geri alındı: id=%s conv=%s kullanici=%s proof=%s",
		msgID, convID, shortDIDStr(user.DID), proofHash)

	respond(w, 200, map[string]interface{}{
		"message_id": msgID,
		"status":     "recalled",
	}, "")
}

// broadcastRecallToGroup — konuşma üyelerini DB'den çekip her birine bildirim gönderir.
func broadcastRecallToGroup(convID string, payload messaging.RecallConfirmPayload) {
	rows, err := db.DB.Query(
		"SELECT user_did FROM conv_members WHERE conv_id = ?", convID,
	)
	if err != nil {
		log.Printf("[recall] Grup üyeleri alınamadı (conv=%s): %v", convID, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var did string
		if err := rows.Scan(&did); err != nil {
			continue
		}
		messaging.GlobalHub.SendTo(did, messaging.MsgTypeRecallConfirm, payload)
	}
}

// gossipShardDelete — tüm peer node'lara /v1/internal/shard-delete isteği gönderir.
// RelayToPeers fire-and-forget'tir; bu fonksiyon bir goroutine içinde çağrılır.
func gossipShardDelete(msgID, convID, callerDID, proofHash string) {
	shardPayload := map[string]interface{}{
		"message_id":        msgID,
		"conv_id":           convID,
		"requested_by":      callerDID,
		"recall_proof_hash": proofHash,
		"requested_at":      time.Now().Unix(),
	}
	gossip.RelayToPeers("*", "shard_delete", shardPayload)
}
