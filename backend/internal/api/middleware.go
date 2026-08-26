package api

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	"obscura.network/core/internal/auth"
	"obscura.network/core/internal/db"
	"obscura.network/core/internal/models"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respond(w, 401, nil, "Yetkilendirme başlığı eksik")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateToken(tokenStr)
		if err != nil {
			respond(w, 401, nil, "Geçersiz token")
			return
		}

		// Kullanıcıyı DB'den çek
		var user models.User
		err = db.DB.QueryRow(`
			SELECT id, COALESCE(phone,''), username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
			       COALESCE(bio,''), tier, credit_score, is_active, COALESCE(hide_online,0), COALESCE(phone_visible,0), is_banned, node_id
			FROM users WHERE id = ?`, claims.UserID,
		).Scan(&user.ID, &user.Phone, &user.Username, &user.DisplayName,
			&user.DID, &user.Odi, &user.IdentityKey, &user.AvatarURL, &user.Bio,
			&user.Tier, &user.CreditScore, &user.IsActive, &user.HideOnline, &user.PhoneVisible, &user.IsBanned, &user.NodeID)

		if err == sql.ErrNoRows {
			respond(w, 401, nil, "Kullanıcı bulunamadı")
			return
		}
		// migrate.go:105 (MigratePhoneToSubscriberStore) NULL'a çevirdiği
		// phone kolonu COALESCE olmadan bir Scan tip hatası üretiyordu; o hata
		// ErrNoRows olmadığı için burada sessizce yutulup user sıfır-değer
		// struct'ta (IsActive=false) kalıyor ve aşağıdaki kontrol yanlışlıkla
		// "askıya alınmış" 403 döndürüyordu. Artık her Scan hatası (ErrNoRows
		// dışı) 500'e düşüyor — sıfır-değer struct'la devam yok.
		if err != nil {
			respond(w, 500, nil, "Kullanıcı bilgisi okunamadı")
			return
		}

		if !user.IsActive || user.IsBanned {
			respond(w, 403, nil, "Hesabınız askıya alınmıştır")
			return
		}

		// Last seen güncelle
		go db.DB.Exec("UPDATE users SET last_seen_at = ? WHERE id = ?",
			time.Now().Format(time.RFC3339), user.ID)

		ctx := context.WithValue(r.Context(), "user", &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isAdminDID reports whether did is listed in OBSCURA_ADMIN_DIDS (comma-
// separated). Read fresh on every call — NOT cached at package-init like
// jwtKey/turnSharedSecret/internalSecret, deliberately: those are hot-path
// secrets read once at boot in real deployment; this is a low-traffic
// admin-only check where per-call os.Getenv cost is negligible, and reading
// fresh keeps the middleware testable across "empty env" vs "DID present"
// vs "DID absent" scenarios within the same test binary (a package-init-once
// var would freeze the first value for the whole process, making that
// impossible to exercise in one `go test` run).
func isAdminDID(did string) bool {
	if did == "" {
		return false
	}
	raw := os.Getenv("OBSCURA_ADMIN_DIDS")
	if raw == "" {
		return false // fail-closed: env tanımsız/boşsa hiç kimse admin değil
	}
	for _, d := range strings.Split(raw, ",") {
		if strings.TrimSpace(d) == did {
			return true
		}
	}
	return false
}

// AdminMiddleware — İlke 5 (docs/spec/obscura_denetim_topluluk_katmani.md
// Bölüm 0: "ciddi cezalarda kararı insan verir") gereği admin-only
// endpoint'leri korur (review-queue listeleme/karar). AuthMiddleware'DEN
// SONRA zincirlenmeli — context'te "user" (DID) bekler.
//
// Fail-closed: OBSCURA_ADMIN_DIDS tanımsız/boşsa (isAdminDID her zaman
// false döner) HİÇBİR istek geçmez. "env unutuldu" senaryosunda sessizce
// herkese açılmak (fail-open) yerine sessizce kapalı kalır — güvenlik
// kritik bir kapı için doğru varsayılan budur.
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getUser(r)
		if user == nil {
			respond(w, 401, nil, "Yetkilendirme gerekli")
			return
		}
		if !isAdminDID(user.DID) {
			respond(w, 403, nil, "Bu işlem için yönetici yetkisi gerekli")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		_ = start
		// log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}
