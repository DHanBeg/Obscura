package api_test

// AdminMiddleware testleri — İlke 5 (admin inceleme arayüzü) için erişim
// kapısı. Bu adımda middleware henüz hiçbir route'a bağlı değil (adım 3'te
// GET /v1/admin/review-queue ile birlikte bağlanacak) — burada doğrudan
// http.Handler zinciri kurup test ediyoruz.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"obscura.network/core/internal/api"
)

// withAdminEnv, OBSCURA_ADMIN_DIDS'i test süresince val'e ayarlar ve testten
// sonra önceki değere geri döner (test izolasyonu — env process-global).
func withAdminEnv(t *testing.T, val string) {
	t.Helper()
	prev, had := os.LookupEnv("OBSCURA_ADMIN_DIDS")
	os.Setenv("OBSCURA_ADMIN_DIDS", val)
	t.Cleanup(func() {
		if had {
			os.Setenv("OBSCURA_ADMIN_DIDS", prev)
		} else {
			os.Unsetenv("OBSCURA_ADMIN_DIDS")
		}
	})
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
}

// callAuthed, api.AuthMiddleware(api.AdminMiddleware(okHandler)) zincirini
// gerçek bir Bearer token ile çağırır (Authorization header üzerinden —
// AuthMiddleware zaten bunu bekliyor, gerçek zincir aynen üretimdeki gibi).
func callAuthed(token string) int {
	handler := api.AuthMiddleware(api.AdminMiddleware(okHandler()))
	req := httptest.NewRequest("GET", "/v1/admin/review-queue", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

// TestAdminMiddleware_DIDInSet_Passes — DID OBSCURA_ADMIN_DIDS'te → geçer (200).
func TestAdminMiddleware_DIDInSet_Passes(t *testing.T) {
	token := loginAndRegister(t, "+905559993001", "admin_mw_pass")
	did := currentUserDID(t, token)

	withAdminEnv(t, did)

	if code := callAuthed(token); code != 200 {
		t.Errorf("DID sette iken 200 bekleniyordu, alınan=%d", code)
	}
}

// TestAdminMiddleware_DIDNotInSet_Returns403 — DID başka DID'lerin
// listesinde ama kendisi yok → 403.
func TestAdminMiddleware_DIDNotInSet_Returns403(t *testing.T) {
	token := loginAndRegister(t, "+905559993002", "admin_mw_notin")
	_ = currentUserDID(t, token)

	withAdminEnv(t, "did:obs:someoneelse1,did:obs:someoneelse2")

	if code := callAuthed(token); code != 403 {
		t.Errorf("DID sette değilken 403 bekleniyordu, alınan=%d", code)
	}
}

// TestAdminMiddleware_EmptyEnv_FailsClosed — OBSCURA_ADMIN_DIDS boş/tanımsız
// → HERKES 403 (fail-closed, fail-open DEĞİL). Gerçek, kayıtlı bir kullanıcı
// bile geçemez.
func TestAdminMiddleware_EmptyEnv_FailsClosed(t *testing.T) {
	token := loginAndRegister(t, "+905559993003", "admin_mw_emptyenv")
	_ = currentUserDID(t, token)

	withAdminEnv(t, "")

	if code := callAuthed(token); code != 403 {
		t.Errorf("env boşken 403 (fail-closed) bekleniyordu, alınan=%d", code)
	}
}

// TestAdminMiddleware_EmptyEnv_Unset_FailsClosed — env hiç set edilmemişse
// de (LookupEnv false) aynı fail-closed davranış — os.Getenv("") ile
// os.Unsetenv arasında fark olmamalı.
func TestAdminMiddleware_EmptyEnv_Unset_FailsClosed(t *testing.T) {
	token := loginAndRegister(t, "+905559993004", "admin_mw_unsetenv")
	_ = currentUserDID(t, token)

	prev, had := os.LookupEnv("OBSCURA_ADMIN_DIDS")
	os.Unsetenv("OBSCURA_ADMIN_DIDS")
	t.Cleanup(func() {
		if had {
			os.Setenv("OBSCURA_ADMIN_DIDS", prev)
		}
	})

	if code := callAuthed(token); code != 403 {
		t.Errorf("env tanımsızken 403 (fail-closed) bekleniyordu, alınan=%d", code)
	}
}

// TestAdminMiddleware_NoAuthContext_Returns401 — AuthMiddleware zincire hiç
// girmemiş (context'te "user" yok) → AdminMiddleware tek başına 401 döner
// (403 değil — bu "yetkisiz admin" değil, "kimliği doğrulanmamış" durumu).
func TestAdminMiddleware_NoAuthContext_Returns401(t *testing.T) {
	withAdminEnv(t, "did:obs:whatever")

	handler := api.AdminMiddleware(okHandler()) // AuthMiddleware YOK
	req := httptest.NewRequest("GET", "/v1/admin/review-queue", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("auth context yokken 401 bekleniyordu, alınan=%d", rec.Code)
	}
}
