---
name: go-backend-patterns
description: Obscura-specific Go backend patterns — handler structure, middleware, DB access, WebSocket hub, gossip relay, response envelope, error handling. Use when writing any backend/internal/ code.
---

# Obscura Go Backend Patterns

## Handler structure (canonical)

```go
package api

import (
    "encoding/json"
    "net/http"
    "github.com/gorilla/mux"
    "obscura/internal/db"
    "obscura/internal/auth"
)

func HandleSomething(w http.ResponseWriter, r *http.Request) {
    // 1. Limit request body
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

    // 2. Get authed user (auth middleware ran)
    userDID := auth.UserDID(r)
    if userDID == "" {
        respond(w, false, nil, "unauthorized")
        return
    }

    // 3. Decode + validate body
    var req struct {
        Field string `json:"field"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respond(w, false, nil, "invalid json")
        return
    }
    if req.Field == "" {
        respond(w, false, nil, "field required")
        return
    }

    // 4. DB call (parameterized!)
    var result string
    err := db.DB.QueryRowContext(r.Context(),
        "SELECT something FROM table WHERE did = ? AND field = ?",
        userDID, req.Field,
    ).Scan(&result)
    if err != nil {
        respond(w, false, nil, "not found")
        return
    }

    // 5. Respond
    respond(w, true, map[string]any{"result": result}, "")
}
```

## Response envelope (standard)

```go
func respond(w http.ResponseWriter, success bool, data any, errMsg string) {
    w.Header().Set("Content-Type", "application/json")
    payload := map[string]any{"success": success}
    if success {
        payload["data"] = data
    } else {
        payload["error"] = errMsg
        w.WriteHeader(http.StatusBadRequest)
    }
    _ = json.NewEncoder(w).Encode(payload)
}
```

Always: `{"success": bool, "data" | "error": ...}`. Frontend `apiFetch` expects this shape.

## Route registration (cmd/node/main.go)

```go
r := mux.NewRouter()
r.Use(middleware.Logger)
r.Use(middleware.RateLimit)

public := r.PathPrefix("/v1").Subrouter()
public.HandleFunc("/auth/request-otp", api.HandleRequestOTP).Methods("POST")
public.HandleFunc("/auth/verify-otp", api.HandleVerifyOTP).Methods("POST")

authed := r.PathPrefix("/v1").Subrouter()
authed.Use(middleware.RequireAuth)
authed.HandleFunc("/users/me", api.HandleGetMe).Methods("GET")
authed.HandleFunc("/users/me", api.HandleUpdateMe).Methods("PATCH")
// ... rest

internal := r.PathPrefix("/v1/internal").Subrouter()
internal.Use(middleware.RequireInternalSecret)
internal.HandleFunc("/relay", gossip.BuildRelayHandler(onRelay)).Methods("POST")
```

## DB access rules

```go
// ✓ CORRECT — parameterized
db.DB.QueryRow("SELECT * FROM users WHERE did = ?", did)

// ✗ WRONG — SQL injection
db.DB.QueryRow(fmt.Sprintf("SELECT * FROM users WHERE did = '%s'", did))

// ✗ WRONG — string concat
db.DB.Query("SELECT * FROM users WHERE did = '" + did + "'")
```

## Migration pattern

```go
// internal/db/database.go
func runMigrations() error {
    _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
        id TEXT PRIMARY KEY,
        applied_at INTEGER NOT NULL DEFAULT (unixepoch())
    )`)
    if err != nil { return err }

    migrations := []struct{ id, sql string }{
        {"001_add_fcm_token", "ALTER TABLE users ADD COLUMN fcm_token TEXT DEFAULT ''"},
        {"002_add_apns_token", "ALTER TABLE users ADD COLUMN apns_token TEXT DEFAULT ''"},
    }
    for _, m := range migrations {
        var exists int
        DB.QueryRow("SELECT COUNT(*) FROM _migrations WHERE id = ?", m.id).Scan(&exists)
        if exists > 0 { continue }
        if _, err := DB.Exec(m.sql); err != nil {
            // Idempotent: ignore "duplicate column" errors from previous partial run
            if !strings.Contains(err.Error(), "duplicate column") {
                return fmt.Errorf("migration %s: %w", m.id, err)
            }
        }
        DB.Exec("INSERT INTO _migrations(id) VALUES (?)", m.id)
    }
    return nil
}
```

## WebSocket hub

```go
// internal/messaging/hub.go
var GlobalHub = &Hub{
    clients: make(map[string]map[*Client]bool), // did → connections
    register: make(chan *Client),
    unregister: make(chan *Client),
    broadcast: make(chan BroadcastMsg),
}

type BroadcastMsg struct {
    ToDID string
    Type  string
    Data  any
}

func (h *Hub) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done(): return
        case c := <-h.register:
            h.mu.Lock()
            if h.clients[c.did] == nil { h.clients[c.did] = map[*Client]bool{} }
            h.clients[c.did][c] = true
            h.mu.Unlock()
        case c := <-h.unregister:
            h.mu.Lock()
            delete(h.clients[c.did], c)
            close(c.send)
            h.mu.Unlock()
        case m := <-h.broadcast:
            h.mu.RLock()
            for c := range h.clients[m.ToDID] {
                select {
                case c.send <- m:
                default: // client buffer full, skip
                }
            }
            h.mu.RUnlock()
        }
    }
}
```

## Gossip relay (loop prevention)

```go
// internal/gossip/gossip.go
func RelayToPeers(targetDID, msgType string, payload any) {
    peers := os.Getenv("NODE_PEERS")
    if peers == "" { return }
    body, _ := json.Marshal(map[string]any{
        "origin_node_id": os.Getenv("NODE_ID"),
        "target_did": targetDID,
        "type": msgType,
        "payload": payload,
        "timestamp": time.Now().Unix(),
    })

    for _, peer := range strings.Split(peers, ",") {
        peer := peer
        go func() {
            req, _ := http.NewRequest("POST", "http://"+peer+"/v1/internal/relay", bytes.NewReader(body))
            req.Header.Set("X-Internal-Secret", os.Getenv("INTERNAL_SECRET"))
            req.Header.Set("Content-Type", "application/json")
            client := &http.Client{Timeout: 3 * time.Second}
            resp, _ := client.Do(req)
            if resp != nil { resp.Body.Close() }
        }()
    }
}

func BuildRelayHandler(onRelay func(target, msgType string, payload any)) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var msg struct {
            OriginNodeID string `json:"origin_node_id"`
            TargetDID    string `json:"target_did"`
            Type         string `json:"type"`
            Payload      any    `json:"payload"`
        }
        if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
            w.WriteHeader(400); return
        }
        // Loop prevention
        if msg.OriginNodeID == os.Getenv("NODE_ID") {
            w.WriteHeader(204); return
        }
        onRelay(msg.TargetDID, msg.Type, msg.Payload)
        w.WriteHeader(200)
    }
}
```

## Build commands

```bash
cd D:/obscura/backend
CGO_ENABLED=0 go build -ldflags="-w -s" -o obscura-node.exe ./cmd/node/
go test ./...
go vet ./...
```

## Hard rules (Obscura)

- `CGO_ENABLED=0` always (modernc.org/sqlite is pure Go)
- No external SDK (Twilio/FCM/MinIO are raw HTTP)
- All SQL parameterized (`?`)
- Response envelope: `{"success": bool, "data" | "error": ...}`
- Auth via `middleware.RequireAuth`, internal via `middleware.RequireInternalSecret`
- Push payloads NEVER contain message plaintext
- No panic outside main()
- Errors wrapped: `fmt.Errorf("context: %w", err)`
