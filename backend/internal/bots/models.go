package bots

// Bot — webhook tabanlı otomatik ajan. Mesaj geldiğinde kayıtlı URL'ye POST atar.
type Bot struct {
	ID          string `json:"id"`
	OwnerDID    string `json:"owner_did"`
	Name        string `json:"name"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
	WebhookURL  string `json:"webhook_url"`
	Token       string `json:"token,omitempty"` // webhook auth secret — sadece owner görür
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at"`
}

// BotMessage — bot gelen/giden mesaj kaydı.
type BotMessage struct {
	ID        string `json:"id"`
	BotID     string `json:"bot_id"`
	FromDID   string `json:"from_did"`
	ToDID     string `json:"to_did"`
	Content   string `json:"content"`
	Direction string `json:"direction"` // "inbound" | "outbound"
	CreatedAt string `json:"created_at"`
}

// WebhookPayload — bot webhook URL'ine POST edilen JSON.
type WebhookPayload struct {
	FromDID   string `json:"from_did"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// WebhookReply — bot webhook'undan beklenen JSON yanıt.
type WebhookReply struct {
	Content string `json:"content"`
}
