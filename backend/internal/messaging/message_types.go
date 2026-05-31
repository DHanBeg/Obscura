// Package messaging — WebSocket mesaj tipleri (Spec Bölüm 6.1, 6.4, 6.6)
//
// Tüm WS type sabitlerinin tek kaynağı burasıdır. hub.go ve MLS tipleri
// (mls_types.go) bu paketteki isimlendirme sözleşmesini takip eder.
//
// Routing kuralları:
//   - call_invite / call_accept / call_end : 1-1 WebRTC sinyal kanalı
//   - group_invite                         : grup_id → tüm üyelere broadcast
//   - zk_proof                             : alıcı DID'ye doğrudan
//   - message_expired                      : scheduler → etkilenen tüm alıcılara
//   - recall_request / recall_confirm      : göndericiden → ilgili DID'lere
package messaging

// ─── ÇAĞRI (WebRTC sinyal) ────────────────────────────────────────────────────

const (
	MsgTypeCallInvite = "call_invite"
	MsgTypeCallAccept = "call_accept"
	MsgTypeCallEnd    = "call_end"
)

// CallInvitePayload — arayan tarafından oluşturulur; SDP offer + TURN bilgisi içerir.
type CallInvitePayload struct {
	// FromDID: arayan kullanıcının DID'i
	FromDID string `json:"from_did"`
	// ToDID: aranan kullanıcının DID'i
	ToDID string `json:"to_did"`
	// ConvID: varsa mevcut konuşma ID'si
	ConvID string `json:"conv_id,omitempty"`
	// SDPOffer: WebRTC SDP offer (base64)
	SDPOffer string `json:"sdp_offer"`
	// CallType: "audio" | "video"
	CallType string `json:"call_type"`
	// ICEServers: TURN server bilgisi (ttl, credential)
	ICEServers []ICEServer `json:"ice_servers,omitempty"`
	// Timestamp: Unix milli
	Timestamp int64 `json:"timestamp"`
}

// CallAcceptPayload — aranan tarafından oluşturulur; SDP answer içerir.
type CallAcceptPayload struct {
	FromDID   string      `json:"from_did"`
	ToDID     string      `json:"to_did"`
	SDPAnswer string      `json:"sdp_answer"`
	ICECands  []string    `json:"ice_candidates,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// CallEndPayload — her iki taraf da gönderebilir; sebep opsiyonel.
type CallEndPayload struct {
	FromDID  string `json:"from_did"`
	ToDID    string `json:"to_did"`
	// Reason: "hangup" | "busy" | "missed" | "error"
	Reason   string `json:"reason"`
	Duration int64  `json:"duration_sec,omitempty"` // Aramanın süresi (sn)
	Timestamp int64 `json:"timestamp"`
}

// ICEServer — WebRTC TURN/STUN sunucu tanımı
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ─── GRUP DAVET ───────────────────────────────────────────────────────────────

const MsgTypeGroupInvite = "group_invite"

// GroupInvitePayload — yeni üye ekleme isteği; admin tarafından oluşturulur.
// Grup üyelerinin tamamına broadcast edilir; yeni üyeye MLS welcome ayrıca iletilir.
type GroupInvitePayload struct {
	// GroupID: MLS group ID (conversations tablosundaki konuşma ID'si)
	GroupID string `json:"group_id"`
	// GroupName: kullanıcıya gösterilecek isim
	GroupName string `json:"group_name"`
	// InviterDID: daveti gönderen kullanıcının DID'i
	InviterDID string `json:"inviter_did"`
	// InviteeDID: davet edilen kullanıcının DID'i
	InviteeDID string `json:"invitee_did"`
	// MemberDIDs: mevcut tüm grup üyelerinin DID listesi
	MemberDIDs []string `json:"member_dids"`
	// Timestamp: Unix milli
	Timestamp int64 `json:"timestamp"`
}

// ─── ZK PROOF (eşler arası) ───────────────────────────────────────────────────

const MsgTypeZKProof = "zk_proof"

// ZKProofPayload — bir kullanıcıdan diğerine ZK kanıt gönderimi.
// Sunucu sadece iletir; kanıt içeriğini yorumlamaz.
type ZKProofPayload struct {
	FromDID     string      `json:"from_did"`
	ToDID       string      `json:"to_did"`
	CircuitID   string      `json:"circuit_id"`
	ProofB64    string      `json:"proof_b64"`
	PublicInputs interface{} `json:"public_inputs"`
	Timestamp   int64       `json:"timestamp"`
}

// ─── MESAJ SONA ERME ──────────────────────────────────────────────────────────

const MsgTypeMessageExpired = "message_expired"

// MessageExpiredPayload — scheduler tarafından gönderilir;
// client'ın UI'ında mesajı "sona erdi" olarak işaretlemesi gerekir.
type MessageExpiredPayload struct {
	MessageID string `json:"message_id"`
	ConvID    string `json:"conv_id"`
	// ExpiredAt: Unix timestamp (sona erme zamanı)
	ExpiredAt int64 `json:"expired_at"`
}

// ─── MESAJ GERİ ALMA (Recall) ─────────────────────────────────────────────────

const (
	MsgTypeRecallRequest = "recall_request"
	MsgTypeRecallConfirm = "recall_confirm"
)

// RecallRequestPayload — gönderici, mesajı geri almak istediğinde hub'a iletir.
// ZKProof opsiyoneldir; varsa gossip relay üzerinden diğer node'lara da gönderilir.
type RecallRequestPayload struct {
	MessageID string      `json:"message_id"`
	ConvID    string      `json:"conv_id"`
	// ZKProof: silme kanıtı (isteğe bağlı — spec Bölüm 6.6)
	ZKProof   interface{} `json:"zk_proof,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// RecallConfirmPayload — hub, geri alma onayını tüm alıcılara iletir.
type RecallConfirmPayload struct {
	MessageID  string `json:"message_id"`
	ConvID     string `json:"conv_id"`
	RecalledBy string `json:"recalled_by"` // Gönderici DID
	// RecallProofHash: ZK proof'un SHA256 hash'i (doğrulama için — boş olabilir)
	RecallProofHash string `json:"recall_proof_hash,omitempty"`
	Timestamp       int64  `json:"timestamp"`
}
