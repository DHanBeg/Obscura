package models

import (
	"encoding/json"
	"time"
)

// ToJSON — herhangi bir değeri JSON string'e dönüştür (güvenli, hata durumunda "null")
func ToJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// ─── KULLANICI ───────────────────────────────────────────────────────────────

type User struct {
	ID              string     `json:"id" db:"id"`
	Phone           string     `json:"-" db:"phone"`
	Username        string     `json:"username" db:"username"`
	DisplayName     string     `json:"display_name" db:"display_name"`
	DID             string     `json:"did" db:"did"`
	IdentityKey     string     `json:"identity_key" db:"identity_key"`
	AvatarURL       string     `json:"avatar_url" db:"avatar_url"`
	Bio             string     `json:"bio" db:"bio"`
	Tier            int        `json:"tier" db:"tier"`
	CreditScore     float64    `json:"credit_score" db:"credit_score"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	HideOnline      bool       `json:"hide_online" db:"hide_online"`
	IsBanned        bool       `json:"is_banned" db:"is_banned"`
	BanExpiresAt    *time.Time `json:"ban_expires_at,omitempty" db:"ban_expires_at"`
	NodeID          string     `json:"node_id" db:"node_id"`
	DilithiumPubKey string     `json:"dilithium_pub_key,omitempty" db:"dilithium_pub_key"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	LastSeenAt      time.Time  `json:"last_seen_at" db:"last_seen_at"`
}

// ─── MESAJ ───────────────────────────────────────────────────────────────────

type MessageType string

const (
	MsgText        MessageType = "text"
	MsgImage       MessageType = "image"
	MsgVoice       MessageType = "voice"
	MsgFile        MessageType = "file"
	MsgLocation    MessageType = "location"
	MsgCallInvite  MessageType = "call_invite"
	MsgCallAccept  MessageType = "call_accept"
	MsgCallEnd     MessageType = "call_end"
	MsgGroupInvite MessageType = "group_invite"
	MsgZKProof     MessageType = "zk_proof"
	// MsgPanicAlert — Madde 13 (Bölüm 6): panik butonu mesajı. Ciphertext
	// içinde yalnızca kaba grid_id taşır (asla ham lat/lon) — sunucu type/
	// from_did/to_did/sent_at'i normal mesajlarda olduğu gibi düz görür
	// (sealed-sender bağlı değil, bkz. proje notu), ama içeriği/konumu
	// asla göremez. Bilinçli kabul edilen sınır — UI metni buna göre yazılır.
	MsgPanicAlert MessageType = "panic_alert"
	// MsgImSafe — Madde 13 Adım 7: "Buluştum, iyiyim" onayı. Panik butonunun
	// tersi yönü — konum İÇERMEZ (yalnızca sent_at), sadece güven kişisine
	// giden bir güvendeyim sinyali. Normal push önceliğiyle iletilir (panik
	// kadar acil değil), operatörde ayrıca loglanmaz (diğer her mesaj gibi).
	MsgImSafe MessageType = "im_safe"
	// MsgReadReceipt — Madde 15, Adım 6b: sealed mesajlar için okundu-bilgisi.
	// HTTP POST /v1/messages/{id}/read (extra_handlers.go) sunucu-taraflı
	// SendReadReceipt'e dayanır — bu, plaintext from_did'i DB'den okumayı
	// gerektirir, sealed mesajlarda (from_did="") ÇALIŞMAZ (sunucu kime
	// göndereceğini bilemez). Çözüm: okundu-bilgisi Signal'ın yaptığı gibi
	// NORMAL bir sealed mesaj olarak alıcıdan göndericiye gider — sunucu
	// yalnızca to_did'i görür (zaten her mesajda gördüğü şey), from_did'i
	// yine görmez. HandleSendMessage bu tip için unread_count/konuşma
	// önizlemesini GÜNCELLEMEZ (bkz. o dosyadaki özel-durum kontrolü) —
	// bu bir "gerçek mesaj" değil, meta-sinyal. Eski (zarfsız) mesajlarda
	// mevcut HTTP+SendReadReceipt yolu AYNEN çalışmaya devam eder — bu tip
	// SADECE sealed mesajlar için ek bir yoldur, eskisinin yerini almaz.
	MsgReadReceipt MessageType = "read_receipt"
)

type MessageStatus string

const (
	StatusSending   MessageStatus = "sending"
	StatusSent      MessageStatus = "sent"      // Sunucuya ulaştı (1 pençe)
	StatusDelivered MessageStatus = "delivered" // Cihaza düştü (2 pençe)
	StatusRead      MessageStatus = "read"      // Görüldü (3 pençe - yeşil)
	StatusFailed    MessageStatus = "failed"
	StatusExpired   MessageStatus = "expired"
)

type Message struct {
	ID           string        `json:"id" db:"id"`
	ConvID       string        `json:"conv_id" db:"conv_id"` // Conversation ID
	FromDID      string        `json:"from_did" db:"from_did"`
	ToDID        string        `json:"to_did" db:"to_did"` // Grup ise group_id
	Type         MessageType   `json:"type" db:"type"`
	Ciphertext   string        `json:"ciphertext" db:"ciphertext"` // Base64 E2EE
	MediaURL     string        `json:"media_url,omitempty" db:"media_url"`
	Status       MessageStatus `json:"status" db:"status"`
	IsGroup      bool          `json:"is_group" db:"is_group"`
	ReplyToID    string        `json:"reply_to_id,omitempty" db:"reply_to_id"`
	SentAt       time.Time     `json:"sent_at" db:"sent_at"`
	DeliveredAt  *time.Time    `json:"delivered_at,omitempty" db:"delivered_at"`
	ReadAt       *time.Time    `json:"read_at,omitempty" db:"read_at"`
	ExpiresAt    time.Time     `json:"expires_at" db:"expires_at"` // 30 gün TTL
	DeletedAt    *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
	DilithiumSig string        `json:"dilithium_sig,omitempty" db:"dilithium_sig"` // hex; optional PQ imzası
	// SelfDestructSeconds/SelfDestructAt — ExpiresAt'ten (30 gün genel TTL) AYRI,
	// kullanıcı seçimli erken silme. Seconds: nil=kapalı, 0="okununca", >0=gönderimden
	// N sn sonra. At: hesaplanmış mutlak silme zamanı (okununca modunda okunana kadar nil).
	SelfDestructSeconds *int       `json:"self_destruct_seconds,omitempty" db:"self_destruct_seconds"`
	SelfDestructAt      *time.Time `json:"self_destruct_at,omitempty" db:"self_destruct_at"`
}

// ─── KONUŞMA ─────────────────────────────────────────────────────────────────

type Conversation struct {
	ID          string     `json:"id" db:"id"`
	IsGroup     bool       `json:"is_group" db:"is_group"`
	Name        string     `json:"name,omitempty" db:"name"`
	AvatarURL   string     `json:"avatar_url,omitempty" db:"avatar_url"`
	ConvType    string     `json:"conv_type,omitempty" db:"conv_type"`
	Description string     `json:"description,omitempty" db:"description"`
	IsPublic    bool       `json:"is_public" db:"is_public"`
	LastMsgID   string     `json:"last_msg_id,omitempty" db:"last_msg_id"`
	LastMsgText string     `json:"last_msg_text,omitempty" db:"last_msg_text"`
	LastMsgAt   *time.Time `json:"last_msg_at,omitempty" db:"last_msg_at"`
	UnreadCount int        `json:"unread_count" db:"unread_count"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

type ConvMember struct {
	ConvID     string     `json:"conv_id" db:"conv_id"`
	UserDID    string     `json:"user_did" db:"user_did"`
	Role       string     `json:"role" db:"role"` // admin, member
	JoinedAt   time.Time  `json:"joined_at" db:"joined_at"`
	MutedUntil *time.Time `json:"muted_until,omitempty" db:"muted_until"`
}

// ─── KREDİ ───────────────────────────────────────────────────────────────────

type CreditEvent struct {
	ID        string    `json:"id" db:"id"`
	UserDID   string    `json:"user_did" db:"user_did"`
	EventType string    `json:"event_type" db:"event_type"`
	Delta     float64   `json:"delta" db:"delta"`
	Reason    string    `json:"reason" db:"reason"`
	NewScore  float64   `json:"new_score" db:"new_score"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─── ZK PROOF ─────────────────────────────────────────────────────────────────

type ZKProof struct {
	ID           string    `json:"id" db:"id"`
	UserDID      string    `json:"user_did" db:"user_did"`
	CircuitID    string    `json:"circuit_id" db:"circuit_id"`
	ProofData    string    `json:"proof_data" db:"proof_data"`       // Base64
	PublicInputs string    `json:"public_inputs" db:"public_inputs"` // JSON
	Verified     bool      `json:"verified" db:"verified"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
}

// ─── OTP ─────────────────────────────────────────────────────────────────────

type OTPRecord struct {
	ID        string    `json:"id" db:"id"`
	Phone     string    `json:"-" db:"phone"`
	Code      string    `json:"-" db:"code"`
	Attempts  int       `json:"attempts" db:"attempts"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	Used      bool      `json:"used" db:"used"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─── API YANIT YAPILARI ───────────────────────────────────────────────────────

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Code    int         `json:"code,omitempty"`
}

type LoginRequest struct {
	Phone string `json:"phone"` // +905551234567
}

type VerifyOTPRequest struct {
	Phone       string `json:"phone"`
	OTP         string `json:"otp"`
	IdentityKey string `json:"identity_key,omitempty"` // Yeni kayıt
	Username    string `json:"username,omitempty"`
	DID         string `json:"did,omitempty"`
	// ZK-ID — identity_proof.circom kanıtı (opsiyonel, spec Bölüm 5.2-5.3)
	// Secret asla backend'e gönderilmez; sadece proof + public params gelir.
	ZKIDProof  string `json:"zk_id_proof,omitempty"`  // base64 Groth16 proof JSON
	ZKIDPublic string `json:"zk_id_public,omitempty"` // base64 publicSignals JSON
}

// ZKIDUpdateRequest — POST /v1/auth/zk-id-update
// Mevcut kullanıcıların sonradan ZK kimlik doğrulaması eklemesi için.
type ZKIDUpdateRequest struct {
	ZKIDProof  string `json:"zk_id_proof"`  // base64 Groth16 proof JSON
	ZKIDPublic string `json:"zk_id_public"` // base64 publicSignals JSON
}

type SendMessageRequest struct {
	ToID           string      `json:"to_id"` // DID veya Group ID
	Type           MessageType `json:"type"`
	Ciphertext     string      `json:"ciphertext"`                // Signal ciphertext (preferred)
	Content        string      `json:"content,omitempty"`         // Backward-compat alias for Ciphertext
	EncryptionType string      `json:"encryption_type,omitempty"` // "signal" | "mls" | "" (defaults to "signal")
	MediaURL       string      `json:"media_url,omitempty"`
	ReplyToID      string      `json:"reply_to_id,omitempty"`
	IsGroup        bool        `json:"is_group"`
	DilithiumSig   string      `json:"dilithium_sig,omitempty"` // hex; optional Dilithium3 imzası
	// SelfDestructSeconds — nil=kapalı, 0="okununca", ya da 10/60/300/3600.
	// ExpiresAt (30 gün genel TTL) İLE KARIŞMAZ, ayrı ve isteğe bağlı.
	SelfDestructSeconds *int `json:"self_destruct_seconds,omitempty"`
}

// SelfDestructAllowedSeconds — desteklenen self-destruct süreleri (sistem
// sınırında doğrulama; UI'daki seçenek listesiyle birebir).
var SelfDestructAllowedSeconds = map[int]bool{
	0:    true, // okununca
	10:   true,
	60:   true,
	300:  true,
	3600: true,
}

// EffectiveCiphertext returns the ciphertext, falling back to the legacy
// Content field so old clients keep working while new ones use Ciphertext.
func (r *SendMessageRequest) EffectiveCiphertext() string {
	if r.Ciphertext != "" {
		return r.Ciphertext
	}
	return r.Content
}

// EffectiveEncryptionType returns "signal" unless explicitly set to another
// recognised type ("mls", "sealed").
//
// "sealed" — Madde 15 (sealed-sender): Ciphertext bir sealed-sender zarfı
// (Signal-tarzı gönderen sertifikası — bkz. crypto/src/sealed_sender.rs,
// mobile/lib/sealed-sender.ts). Sunucu bu durumda from_did'i PLAINTEXT
// SAKLAMAZ (bkz. HandleSendMessage) — kademeli geçiş: eski client'lar bu
// alanı hiç göndermez, "signal" varsayılanına düşer, davranış değişmez.
func (r *SendMessageRequest) EffectiveEncryptionType() string {
	switch r.EncryptionType {
	case "mls", "signal", "sealed":
		return r.EncryptionType
	default:
		return "signal"
	}
}

// IsSealedSender reports whether this message's ciphertext is a
// sealed-sender envelope — the caller (HandleSendMessage) must not persist
// or broadcast a plaintext from_did for it.
func (r *SendMessageRequest) IsSealedSender() bool {
	return r.EffectiveEncryptionType() == "sealed"
}

type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ─── KATMANlar ────────────────────────────────────────────────────────────────

var TierNames = map[int]string{
	1: "Bronz",
	2: "Gümüş",
	3: "Altın",
	4: "Platin",
	5: "Elmas",
}

var TierMinScore = map[int]float64{
	1: -20,
	2: 60,
	3: 70,
	4: 80,
	5: 90,
}

var TierMaxGroupSize = map[int]int{
	1: 0,
	2: 100,
	3: 1000,
	4: 10000,
	5: -1, // limitsiz
}

var TierDailyMsgLimit = map[int]int{
	1: 50,
	2: 200,
	3: 1000,
	4: -1,
	5: -1,
}

func ScoreToTier(score float64) int {
	switch {
	case score >= 90:
		return 5
	case score >= 80:
		return 4
	case score >= 70:
		return 3
	case score >= 60:
		return 2
	default:
		return 1
	}
}
