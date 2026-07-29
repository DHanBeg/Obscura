package api

import (
	"context"
	"database/sql"
	"net/http"
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
			SELECT id, phone, username, display_name, did, COALESCE(odi,''), identity_key, avatar_url,
			       COALESCE(bio,''), tier, credit_score, is_active, COALESCE(hide_online,0), is_banned, node_id
			FROM users WHERE id = ?`, claims.UserID,
		).Scan(&user.ID, &user.Phone, &user.Username, &user.DisplayName,
			&user.DID, &user.Odi, &user.IdentityKey, &user.AvatarURL, &user.Bio,
			&user.Tier, &user.CreditScore, &user.IsActive, &user.HideOnline, &user.IsBanned, &user.NodeID)

		if err == sql.ErrNoRows {
			respond(w, 401, nil, "Kullanıcı bulunamadı")
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
