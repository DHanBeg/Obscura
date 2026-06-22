package bots

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

func respond(w http.ResponseWriter, code int, data interface{}, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	body := map[string]interface{}{"success": errMsg == ""}
	if errMsg != "" {
		body["error"] = errMsg
	} else {
		body["data"] = data
	}
	json.NewEncoder(w).Encode(body) //nolint:errcheck
}

func callerDID(r *http.Request) string {
	if u, ok := r.Context().Value("user").(*models.User); ok {
		return u.DID
	}
	return ""
}

// HandleCreateBot — POST /api/v1/bots
func HandleCreateBot(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		AvatarURL   string `json:"avatar_url"`
		WebhookURL  string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.Name == "" || req.WebhookURL == "" {
		respond(w, 400, nil, "name ve webhook_url zorunludur")
		return
	}

	bot, err := CreateBot(r.Context(), db.DB, callerDID(r),
		req.Name, req.Description, req.AvatarURL, req.WebhookURL)
	if err != nil {
		log.Printf("bots: create: %v", err)
		respond(w, 500, nil, "Bot oluşturulamadı")
		return
	}
	respond(w, 201, bot, "")
}

// HandleListBots — GET /api/v1/bots (caller's bots)
func HandleListBots(w http.ResponseWriter, r *http.Request) {
	bots, err := ListUserBots(r.Context(), db.DB, callerDID(r))
	if err != nil {
		log.Printf("bots: list: %v", err)
		respond(w, 500, nil, "Botlar listelenemedi")
		return
	}
	if bots == nil {
		bots = []Bot{}
	}
	// Token'ı gizle listede
	for i := range bots {
		bots[i].Token = ""
	}
	respond(w, 200, bots, "")
}

// HandleGetBot — GET /api/v1/bots/{id}
func HandleGetBot(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	bot, err := GetBot(r.Context(), db.DB, id)
	if err != nil {
		log.Printf("bots: get: %v", err)
		respond(w, 500, nil, "Bot alınamadı")
		return
	}
	if bot == nil {
		respond(w, 404, nil, "Bot bulunamadı")
		return
	}
	// Sadece sahibi token'ı görebilir
	if bot.OwnerDID != callerDID(r) {
		bot.Token = ""
	}
	respond(w, 200, bot, "")
}

// HandleUpdateBot — PUT /api/v1/bots/{id}
func HandleUpdateBot(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1<<20)
	id := mux.Vars(r)["id"]
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		WebhookURL  string `json:"webhook_url"`
		Active      bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, nil, "Geçersiz istek gövdesi")
		return
	}
	if req.Name == "" || req.WebhookURL == "" {
		respond(w, 400, nil, "name ve webhook_url zorunludur")
		return
	}
	if err := UpdateBot(r.Context(), db.DB, id, callerDID(r),
		req.Name, req.Description, req.WebhookURL, req.Active); err != nil {
		log.Printf("bots: update: %v", err)
		respond(w, 500, nil, "Bot güncellenemedi")
		return
	}
	respond(w, 200, map[string]bool{"updated": true}, "")
}

// HandleDeleteBot — DELETE /api/v1/bots/{id}
func HandleDeleteBot(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := DeleteBot(r.Context(), db.DB, id, callerDID(r)); err != nil {
		log.Printf("bots: delete: %v", err)
		respond(w, 500, nil, "Bot silinemedi")
		return
	}
	respond(w, 200, map[string]bool{"deleted": true}, "")
}

// HandleRegenerateToken — POST /api/v1/bots/{id}/regenerate-token
func HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	token, err := RegenerateToken(r.Context(), db.DB, id, callerDID(r))
	if err != nil {
		log.Printf("bots: regen-token: %v", err)
		respond(w, 500, nil, "Token yenilenemedi")
		return
	}
	respond(w, 200, map[string]string{"token": token}, "")
}

// HandleDiscoverBots — GET /api/v1/bots/discover?limit=20&offset=0
func HandleDiscoverBots(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	bots, err := ListPublicBots(r.Context(), db.DB, limit, offset)
	if err != nil {
		log.Printf("bots: discover: %v", err)
		respond(w, 500, nil, "Botlar alınamadı")
		return
	}
	if bots == nil {
		bots = []Bot{}
	}
	respond(w, 200, bots, "")
}

// WebhookDispatch — bot'a mesaj geldiğinde webhook URL'ine async POST atar,
// yanıtı gelen kullanıcıya geri gönderir. Fire-and-forget goroutine.
func WebhookDispatch(bot *Bot, fromDID, content string, sendReply func(toDID, text string)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		payload, err := json.Marshal(WebhookPayload{
			FromDID:   fromDID,
			Content:   content,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			log.Printf("bots: webhook marshal: %v", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", bot.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			log.Printf("bots: webhook req: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Obscura-Bot-Token", bot.Token)

		// Gelen mesajı logla
		_ = LogBotMessage(ctx, db.DB, bot.ID, fromDID, bot.OwnerDID, content, "inbound")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("bots: webhook call failed (bot=%s): %v", bot.ID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Printf("bots: webhook non-2xx (bot=%s status=%d)", bot.ID, resp.StatusCode)
			return
		}

		var reply WebhookReply
		if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil || reply.Content == "" {
			return
		}

		// Yanıtı gönder
		_ = LogBotMessage(ctx, db.DB, bot.ID, bot.OwnerDID, fromDID, reply.Content, "outbound")
		sendReply(fromDID, reply.Content)
		log.Printf("bots: webhook reply sent (bot=%s to=%s)", bot.ID, fromDID)
	}()
}

// RegisterRoutes — mux subrouter'a bot rotalarını ekler.
func RegisterRoutes(priv *mux.Router) {
	priv.HandleFunc("/bots/discover", HandleDiscoverBots).Methods("GET", "OPTIONS")
	priv.HandleFunc("/bots", HandleListBots).Methods("GET", "OPTIONS")
	priv.HandleFunc("/bots", HandleCreateBot).Methods("POST", "OPTIONS")
	priv.HandleFunc("/bots/{id}", HandleGetBot).Methods("GET", "OPTIONS")
	priv.HandleFunc("/bots/{id}", HandleUpdateBot).Methods("PUT", "OPTIONS")
	priv.HandleFunc("/bots/{id}", HandleDeleteBot).Methods("DELETE", "OPTIONS")
	priv.HandleFunc("/bots/{id}/regenerate-token", HandleRegenerateToken).Methods("POST", "OPTIONS")
	fmt.Println("🤖 Bot rotaları kayıtlı")
}
