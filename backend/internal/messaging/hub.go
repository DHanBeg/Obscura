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
			log.Printf("🟢 Bağlandı: %s (Tier %d)", client.DID[:12], client.Tier)

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
			log.Printf("🔴 Ayrıldı: %s", client.DID[:12])

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
	}
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
