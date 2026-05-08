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
	ID           string    `json:"id" db:"id"`                     // UUID
	Phone        string    `json:"-" db:"phone"`                   // +905551234567 (gizli)
	Username     string    `json:"username" db:"username"`          // @kullanici
	DisplayName  string    `json:"display_name" db:"display_name"`
	DID          string    `json:"did" db:"did"`                   // did:obs:hash
	IdentityKey  string    `json:"identity_key" db:"identity_key"` // Ed25519 pubkey (base64)
	AvatarURL    string    `json:"avatar_url" db:"avatar_url"`
	Tier         int       `json:"tier" db:"tier"`                 // 1=Bronz 2=Gümüş 3=Altın 4=Platin 5=Elmas
	CreditScore  float64   `json:"credit_score" db:"credit_score"` // -20 ile 100 arası
	IsActive     bool      `json:"is_active" db:"is_active"`
	IsBanned     bool      `json:"is_banned" db:"is_banned"`
	BanExpiresAt *time.Time `json:"ban_expires_at,omitempty" db:"ban_expires_at"`
	NodeID       string    `json:"node_id" db:"node_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	LastSeenAt   time.Time `json:"last_seen_at" db:"last_seen_at"`
}

// ─── MESAJ ───────────────────────────────────────────────────────────────────

type MessageType string

const (
	MsgText      MessageType = "text"
	MsgImage     MessageType = "image"
	MsgVoice     MessageType = "voice"
	MsgFile      MessageType = "file"
	MsgLocation  MessageType = "location"
	MsgCallInvite MessageType = "call_invite"
	MsgCallAccept MessageType = "call_accept"
	MsgCallEnd   MessageType = "call_end"
	MsgGroupInvite MessageType = "group_invite"
	MsgZKProof   MessageType = "zk_proof"
)

type MessageStatus string

const (
	StatusSending   MessageStatus = "sending"
	StatusSent      MessageStatus = "sent"       // Sunucuya ulaştı (1 pençe)
	StatusDelivered MessageStatus = "delivered"  // Cihaza düştü (2 pençe)
	StatusRead      MessageStatus = "read"       // Görüldü (3 pençe - yeşil)
	StatusFailed    MessageStatus = "failed"
	StatusExpired   MessageStatus = "expired"
)

type Message struct {
	ID          string        `json:"id" db:"id"`
	ConvID      string        `json:"conv_id" db:"conv_id"`       // Conversation ID
	FromDID     string        `json:"from_did" db:"from_did"`
	ToDID       string        `json:"to_did" db:"to_did"`         // Grup ise group_id
	Type        MessageType   `json:"type" db:"type"`
	Ciphertext  string        `json:"ciphertext" db:"ciphertext"` // Base64 E2EE
	MediaURL    string        `json:"media_url,omitempty" db:"media_url"`
	Status      MessageStatus `json:"status" db:"status"`
	IsGroup     bool          `json:"is_group" db:"is_group"`
	ReplyToID   string        `json:"reply_to_id,omitempty" db:"reply_to_id"`
	SentAt      time.Time     `json:"sent_at" db:"sent_at"`
	DeliveredAt *time.Time    `json:"delivered_at,omitempty" db:"delivered_at"`
	ReadAt      *time.Time    `json:"read_at,omitempty" db:"read_at"`
	ExpiresAt   time.Time     `json:"expires_at" db:"expires_at"` // 30 gün TTL
	DeletedAt   *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
}

// ─── KONUŞMA ─────────────────────────────────────────────────────────────────

type Conversation struct {
	ID           string    `json:"id" db:"id"`
	IsGroup      bool      `json:"is_group" db:"is_group"`
	Name         string    `json:"name,omitempty" db:"name"`
	AvatarURL    string    `json:"avatar_url,omitempty" db:"avatar_url"`
	LastMsgID    string    `json:"last_msg_id,omitempty" db:"last_msg_id"`
	LastMsgText  string    `json:"last_msg_text,omitempty" db:"last_msg_text"`
	LastMsgAt    *time.Time `json:"last_msg_at,omitempty" db:"last_msg_at"`
	UnreadCount  int       `json:"unread_count" db:"unread_count"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type ConvMember struct {
	ConvID    string    `json:"conv_id" db:"conv_id"`
	UserDID   string    `json:"user_did" db:"user_did"`
	Role      string    `json:"role" db:"role"` // admin, member
	JoinedAt  time.Time `json:"joined_at" db:"joined_at"`
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
	ID          string    `json:"id" db:"id"`
	UserDID     string    `json:"user_did" db:"user_did"`
	CircuitID   string    `json:"circuit_id" db:"circuit_id"`
	ProofData   string    `json:"proof_data" db:"proof_data"`     // Base64
	PublicInputs string   `json:"public_inputs" db:"public_inputs"` // JSON
	Verified    bool      `json:"verified" db:"verified"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	ExpiresAt   time.Time `json:"expires_at" db:"expires_at"`
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
}

type SendMessageRequest struct {
	ToID       string      `json:"to_id"`       // DID veya Group ID
	Type       MessageType `json:"type"`
	Ciphertext string      `json:"ciphertext"`
	MediaURL   string      `json:"media_url,omitempty"`
	ReplyToID  string      `json:"reply_to_id,omitempty"`
	IsGroup    bool        `json:"is_group"`
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
