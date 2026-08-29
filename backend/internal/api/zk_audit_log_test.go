package api

// C10 fail-open kökü kanıtı, #6 — HandleZKAuditLog eskiden
// `secret != expected` (değişken-zamanlı) karşılaştırması + kendi
// os.Getenv("INTERNAL_SECRET")+""-kontrolü kullanıyordu. Artık paketin tek
// INTERNAL_SECRET kaynağı olan internalSecretValue (event_handlers.go,
// secrets.Require) + secrets.RequireEqual (hmac.Equal, sabit-zamanlı).
// Davranış: doğru header → 200, yanlış header → 403, header hiç yok → 403.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleZKAuditLog_CorrectSecret_Allowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/zk/audit", nil)
	req.Header.Set("X-Internal-Secret", internalSecretValue)
	rec := httptest.NewRecorder()

	HandleZKAuditLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("correct secret: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleZKAuditLog_WrongSecret_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/zk/audit", nil)
	req.Header.Set("X-Internal-Secret", internalSecretValue+"-wrong")
	rec := httptest.NewRecorder()

	HandleZKAuditLog(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong secret: expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleZKAuditLog_MissingHeader_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/zk/audit", nil)
	rec := httptest.NewRecorder()

	HandleZKAuditLog(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing header: expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
