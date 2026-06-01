package messaging

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"obscura.network/core/internal/models"
)

// ─── WEBSOCKEt MESAJ TİPLERİ (Spec Bölüm 6.4) ───────────────────────────────

const (
	// MsgTypeDeliveryAck — node gönderene iletir: alıcıya ulaştı (delivered)
	MsgTypeDeliveryAck = "delivery_ack"
	// MsgTypeReadReceipt — alıcıdan gönderene: mesaj okundu (read)
	MsgTypeReadReceipt = "read_receipt"
	// MsgTypeStatusUpdate — genel durum güncellemesi (failed, expired)
	MsgTypeStatusUpdate = "status_update"
)

// ─── WEBSOCKEt UPGRADER ──────────────────────────────────────────────────────

var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Prod'da domain kontrolü yap
	},
}

// ─── CLIENT ───────────────────────────────────────────────────────────────────

type Client struct {
	DID      string
	UserID   string
	Tier     int
	Conn     *websocket.Conn
	Send     chan []byte
	Hub      *Hub
	LastPing time.Time
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(64 * 1024) // 64KB
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.LastPing = time.Now()
		return nil
	})

	for {
		_, raw, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS hata [%s]: %v", c.DID, err)
			}
			break
		}

		var wsMsg models.WSMessage
		if err := json.Unmarshal(raw, &wsMsg); err != nil {
			log.Printf("WS JSON parse hatası: %v", err)
			continue
		}

		c.Hub.HandleMessage(c, &wsMsg)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.Conn.WriteMessage(websocket.TextMessage, msg)

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ─── HUB ──────────────────────────────────────────────────────────────────────

type Hub struct {
	clients    map[string]*Client // DID → Client
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *BroadcastMsg
	mu         sync.RWMutex
}

// shortDID returns first n characters of a DID safely (panic-safe truncation).
func shortDID(did string, n int) string {
	if len(did) <= n {
		return did
	}
	return did[:n]
}

type BroadcastMsg struct {
	ToDID   string
	Payload []byte
}

var GlobalHub = NewHub()

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		Register:   make(chan *Client, 100),
		Unregister: make(chan *Client, 100),
		Broadcast:  make(chan *BroadcastMsg, 1000),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client.DID] = client
			h.mu.Unlock()
			log.Printf("🟢 Bağlandı: %s (Tier %d)", shortDID(client.DID, 12), client.Tier)

			// Bağlantı bildirimi
			h.sendSystemMsg(client, "connected", map[string]interface{}{
				"did":   client.DID,
				"tier":  client.Tier,
				"time":  time.Now().Unix(),
			})

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.DID]; ok {
				delete(h.clients, client.DID)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("🔴 Ayrıldı: %s", shortDID(client.DID, 12))

		case msg := <-h.Broadcast:
			h.mu.RLock()
			target, ok := h.clients[msg.ToDID]
			h.mu.RUnlock()
			if ok {
				select {
				case target.Send <- msg.Payload:
				default:
					h.mu.Lock()
					delete(h.clients, target.DID)
					close(target.Send)
					h.mu.Unlock()
				}
			}
		}
	}
}

func (h *Hub) IsOnline(did string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[did]
	return ok
}

func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) SendTo(did string, msgType string, payload interface{}) bool {
	h.mu.RLock()
	client, ok := h.clients[did]
	h.mu.RUnlock()

	if !ok {
		return false
	}

	data, _ := json.Marshal(models.WSMessage{
		Type:    msgType,
		Payload: payload,
	})

	select {
	case client.Send <- data:
		return true
	default:
		return false
	}
}

func (h *Hub) HandleMessage(client *Client, msg *models.WSMessage) {
	switch msg.Type {
	case "ping":
		h.sendSystemMsg(client, "pong", map[string]interface{}{"time": time.Now().Unix()})

	case "typing":
		// Yazıyor bildirimi
		payload, _ := json.Marshal(msg.Payload)
		var data map[string]interface{}
		json.Unmarshal(payload, &data)

		if toDID, ok := data["to_did"].(string); ok {
			h.SendTo(toDID, "typing", map[string]interface{}{
				"from_did": client.DID,
				"conv_id":  data["conv_id"],
			})
		}

	case "read_receipt":
		// Okundu bildirimi
		payload, _ := json.Marshal(msg.Payload)
		var data map[string]interface{}
		json.Unmarshal(payload, &data)

		if fromDID, ok := data["from_did"].(string); ok {
			h.SendTo(fromDID, "read_receipt", map[string]interface{}{
				"msg_id":  data["msg_id"],
				"read_by": client.DID,
				"time":    time.Now().Unix(),
			})
		}

	// ─── WebRTC Sinyal (1-1) ─────────────────────────────────────────────────
	// call_invite / call_accept / call_end: doğrudan alıcı DID'ye ilet.
	// from_did gönderici olarak zorla (client'ın kendi DID'i).
	case MsgTypeCallInvite:
		h.routeCallSignal(client, msg, MsgTypeCallInvite)

	case MsgTypeCallAccept:
		h.routeCallSignal(client, msg, MsgTypeCallAccept)

	case MsgTypeCallEnd:
		h.routeCallSignal(client, msg, MsgTypeCallEnd)

	// ─── Grup Daveti ─────────────────────────────────────────────────────────
	// group_invite: grup üyelerinin tamamına broadcast.
	// Payload: GroupInvitePayload — member_dids listesi üzerinden her üyeye ilet.
	case MsgTypeGroupInvite:
		h.routeGroupInvite(client, msg)

	// ─── Mesaj Geri Alma ─────────────────────────────────────────────────────
	// recall_request: HTTP handler zaten DB + gossip işini yapar.
	// WS üzerinden gelen recall_request'leri sadece log'la; işlem HTTP API'ye aittir.
	case MsgTypeRecallRequest:
		log.Printf("[hub] recall_request WS üzerinden geldi (did=%s) — HTTP API kullanın", shortDID(client.DID, 12))
		h.sendSystemMsg(client, "error", map[string]interface{}{
			"code":    "use_http",
			"message": "Geri alma işlemi POST /v1/messages/{id}/recall ile yapılır",
		})
	}
}

// routeCallSignal — call_invite / call_accept / call_end mesajlarını to_did'e iletir.
// Gönderen DID'i payload'a zorunlu olarak ekler (sahteciliği önler).
func (h *Hub) routeCallSignal(client *Client, msg *models.WSMessage, msgType string) {
	raw, _ := json.Marshal(msg.Payload)
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		h.sendSystemMsg(client, "error", map[string]interface{}{
			"code": "bad_payload", "message": "Geçersiz sinyal payload",
		})
		return
	}

	toDID, _ := data["to_did"].(string)
	if toDID == "" {
		h.sendSystemMsg(client, "error", map[string]interface{}{
			"code": "missing_to_did", "message": "to_did zorunlu",
		})
		return
	}

	// Gönderici DID'i server tarafında set et (override güvenliği)
	data["from_did"] = client.DID
	data["timestamp"] = time.Now().UnixMilli()

	sent := h.SendTo(toDID, msgType, data)
	if !sent {
		// Alıcı offline — çağrıyı bilgilendir
		h.sendSystemMsg(client, MsgTypeCallEnd, map[string]interface{}{
			"from_did":  toDID,
			"to_did":    client.DID,
			"reason":    "offline",
			"timestamp": time.Now().UnixMilli(),
		})
		log.Printf("[hub] %s iletilmedi (alıcı offline): %s→%s", msgType, shortDID(client.DID, 12), shortDID(toDID, 12))
	}
}

// routeGroupInvite — GroupInvitePayload.MemberDIDs listesindeki her üyeye iletir.
// Payload içindeki from_did sunucu tarafında client.DID ile ezilir.
func (h *Hub) routeGroupInvite(client *Client, msg *models.WSMessage) {
	raw, _ := json.Marshal(msg.Payload)
	var payload GroupInvitePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		h.sendSystemMsg(client, "error", map[string]interface{}{
			"code": "bad_payload", "message": "Geçersiz GroupInvitePayload",
		})
		return
	}

	if payload.GroupID == "" {
		h.sendSystemMsg(client, "error", map[string]interface{}{
			"code": "missing_group_id", "message": "group_id zorunlu",
		})
		return
	}

	// Sunucu gönderici DID'ini zorla
	payload.InviterDID = client.DID
	payload.Timestamp = time.Now().UnixMilli()

	delivered := 0
	for _, memberDID := range payload.MemberDIDs {
		if memberDID == client.DID {
			continue // Gönderici kendine bildirim almaz
		}
		if h.SendTo(memberDID, MsgTypeGroupInvite, payload) {
			delivered++
		}
	}
	log.Printf("[hub] group_invite yayınlandı: group=%s iletilen=%d/%d",
		payload.GroupID, delivered, len(payload.MemberDIDs))
}

func (h *Hub) sendSystemMsg(client *Client, msgType string, payload interface{}) {
	data, _ := json.Marshal(models.WSMessage{
		Type:    msgType,
		Payload: payload,
	})
	select {
	case client.Send <- data:
	default:
	}
}

// SendDeliveryAck — gönderene delivery_ack iletir.
// msgID: mesaj kimliği, status: "delivered" veya "sent".
// Gönderen online değilse false döner (sessizce geçilir).
func (h *Hub) SendDeliveryAck(senderDID, msgID, status string) bool {
	return h.SendTo(senderDID, MsgTypeDeliveryAck, map[string]interface{}{
		"msg_id": msgID,
		"status": status,
		"time":   time.Now().Unix(),
	})
}

// SendReadReceipt — alıcıdan gönderene okundu bildirimini iletir.
// senderDID: mesajı gönderen kişi, msgID: mesaj kimliği, readerDID: okuyan kişi.
func (h *Hub) SendReadReceipt(senderDID, msgID, readerDID string) bool {
	return h.SendTo(senderDID, MsgTypeReadReceipt, map[string]interface{}{
		"msg_id":   msgID,
		"read_by":  readerDID,
		"time":     time.Now().Unix(),
	})
}
