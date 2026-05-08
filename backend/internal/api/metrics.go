package api

// Prometheus-compatible metrics endpoint
// GET /v1/metrics — text/plain Prometheus format
//
// Bağımlılıksız implementasyon — prometheus/client_golang gerekmez.
// Temel sayaçlar runtime.ReadMemStats + hub online sayısı.

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"obscura.network/core/internal/messaging"
)

var (
	startTime     = time.Now()
	requestsTotal int64
	messagesTotal int64
	errorsTotal   int64
)

// IncrRequests — her istekte çağır
func IncrRequests() { requestsTotal++ }

// IncrMessages — her mesaj gönderimde çağır
func IncrMessages() { messagesTotal++ }

// IncrErrors — her hata yanıtında çağır
func IncrErrors() { errorsTotal++ }

// GET /v1/metrics
func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	// Basit IP whitelist — sadece localhost / iç ağ
	// Production'da nginx ile dışarıya kapatın
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	uptime := time.Since(startTime).Seconds()
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-unknown"
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(200)

	fmt.Fprintf(w, `# HELP obscura_uptime_seconds Uptime in seconds
# TYPE obscura_uptime_seconds gauge
obscura_uptime_seconds{node="%s"} %.2f

# HELP obscura_online_users Currently connected WebSocket users
# TYPE obscura_online_users gauge
obscura_online_users{node="%s"} %d

# HELP obscura_requests_total Total HTTP requests handled
# TYPE obscura_requests_total counter
obscura_requests_total{node="%s"} %d

# HELP obscura_messages_total Total messages sent
# TYPE obscura_messages_total counter
obscura_messages_total{node="%s"} %d

# HELP obscura_errors_total Total error responses
# TYPE obscura_errors_total counter
obscura_errors_total{node="%s"} %d

# HELP go_memstats_alloc_bytes Number of bytes allocated and in use
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes{node="%s"} %d

# HELP go_memstats_heap_inuse_bytes Number of heap bytes in use
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes{node="%s"} %d

# HELP go_goroutines Number of goroutines
# TYPE go_goroutines gauge
go_goroutines{node="%s"} %d
`,
		nodeID, uptime,
		nodeID, messaging.GlobalHub.OnlineCount(),
		nodeID, requestsTotal,
		nodeID, messagesTotal,
		nodeID, errorsTotal,
		nodeID, ms.Alloc,
		nodeID, ms.HeapInuse,
		nodeID, runtime.NumGoroutine(),
	)
}
