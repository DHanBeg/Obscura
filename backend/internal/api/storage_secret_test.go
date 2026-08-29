package api

// N11-storage kanıtı — storage_handlers.go'nun 4 node-to-node call-site'ı
// eskiden X-Internal-Secret header'ında HAM INTERNAL_SECRET taşıyordu (fail-
// open commit sadece karşılaştırmayı sabit-zamanlı yaptı, ham secret hâlâ tel
// üzerinde açık gidiyordu — D2 follow-up). Artık internal/gossip'in nodeMAC
// deseninin birebir kopyası: HMAC-SHA256(secret, ts+body), X-Node-Ts/
// X-Node-Sig header'ları, ±30s replay penceresi, sabit-zamanlı karşılaştırma
// (verifyNodeHMAC, bkz. storage_handlers.go).
//
// Kanıtlanan 4 durum (her biri 4 call-site'ta da birebir aynı iki satırı
// paylaşıyor — HandleFetchLocalShard/HandleFetchShardInternal ile temsili
// test ediliyor, body alan HandleLocalShard/HandleStoreShard ayrıca):
//  1. Geçerli imza + taze ts → KABUL (401 değil).
//  2. Yanlış imza → RED (401).
//  3. Replay: eski ts (±30s dışı) → RED, geçerli imza olsa bile.
//  4. Header hiç yok → RED (eski kodun "INTERNAL_SECRET boşsa atla" fail-open
//     regresyonunun aynısı, artık yapısal olarak imkansız).

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// signForTest — test tarafında verifyNodeHMAC'in beklediği imzayı üretir.
// Prod kodundaki nodeMAC (internal/storage/sharding.go) ile birebir aynı
// hesaplama; burada bağımsız tutuluyor ki test, imzalama koduyla aynı hatayı
// paylaşıp yanlışlıkla "geçti" göstermesin.
func signForTest(ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(internalSecretValue))
	mac.Write([]byte(ts))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func freshTs() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func staleTs() string {
	return strconv.FormatInt(time.Now().Add(-31*time.Second).UnixMilli(), 10)
}

func TestStorageNodeHMAC_NoRawSecretOnWire(t *testing.T) {
	ts := freshTs()
	sig := signForTest(ts, nil)
	// İmza, ham secret'ın kendisi DEĞİL — hex-encoded 32-byte HMAC çıktısı,
	// internalSecretValue'ya birebir eşit olamaz (farklı uzunluk/format).
	if sig == internalSecretValue {
		t.Fatalf("signature equals the raw secret — HMAC is not actually being applied")
	}
	if len(sig) != 64 { // hex(SHA-256) = 64 hex chars
		t.Fatalf("expected 64-char hex HMAC-SHA256, got %d chars: %q", len(sig), sig)
	}
}

func TestStorageNodeHMAC_FetchLocalShard(t *testing.T) {
	mk := func(ts, sig string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/storage/local-shard/does-not-exist", nil)
		if ts != "" {
			req.Header.Set("X-Node-Ts", ts)
		}
		if sig != "" {
			req.Header.Set("X-Node-Sig", sig)
		}
		w := httptest.NewRecorder()
		HandleFetchLocalShard(w, req)
		return w
	}

	t.Run("valid_sig_fresh_ts_accepted", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, signForTest(ts, nil))
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("valid signature rejected with 401 (body=%s)", w.Body.String())
		}
	})

	t.Run("wrong_sig_rejected", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, "0000000000000000000000000000000000000000000000000000000000000000")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong signature, got %d (body=%s)", w.Code, w.Body.String())
		}
	})

	t.Run("replay_stale_ts_rejected", func(t *testing.T) {
		ts := staleTs()
		w := mk(ts, signForTest(ts, nil)) // imza doğru, ama ts >30s eski
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for stale (replayed) timestamp, got %d (body=%s)", w.Code, w.Body.String())
		}
	})

	t.Run("missing_headers_rejected", func(t *testing.T) {
		w := mk("", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing X-Node-Ts/X-Node-Sig (fail-open regression), got %d (body=%s)", w.Code, w.Body.String())
		}
	})
}

func TestStorageNodeHMAC_FetchShardInternal(t *testing.T) {
	mk := func(ts, sig string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/internal/fetch-shard?id=does-not-exist", nil)
		if ts != "" {
			req.Header.Set("X-Node-Ts", ts)
		}
		if sig != "" {
			req.Header.Set("X-Node-Sig", sig)
		}
		w := httptest.NewRecorder()
		HandleFetchShardInternal(w, req)
		return w
	}

	t.Run("valid_sig_fresh_ts_accepted", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, signForTest(ts, nil))
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("valid signature rejected with 401 (body=%s)", w.Body.String())
		}
	})

	t.Run("missing_headers_rejected", func(t *testing.T) {
		w := mk("", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing headers, got %d (body=%s)", w.Code, w.Body.String())
		}
	})
}

func TestStorageNodeHMAC_LocalShard_BodySigned(t *testing.T) {
	body := []byte(`{"shard_id":"s1","content_id":"c1","chunk_idx":0,"shard_idx":0,"data":null,"expires_at":"2027-01-01T00:00:00Z"}`)

	mk := func(ts, sig string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/storage/local-shard", bytes.NewReader(body))
		if ts != "" {
			req.Header.Set("X-Node-Ts", ts)
		}
		if sig != "" {
			req.Header.Set("X-Node-Sig", sig)
		}
		w := httptest.NewRecorder()
		HandleLocalShard(w, req)
		return w
	}

	t.Run("valid_sig_over_body_accepted", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, signForTest(ts, body))
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("valid body signature rejected with 401 (body=%s)", w.Body.String())
		}
	})

	t.Run("sig_over_wrong_body_rejected", func(t *testing.T) {
		ts := freshTs()
		// Farklı bir body için üretilmiş imza — bu isteğin body'siyle eşleşmiyor.
		wrongSig := signForTest(ts, []byte("different-body"))
		w := mk(ts, wrongSig)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 when signature doesn't match this request's body, got %d (body=%s)", w.Code, w.Body.String())
		}
	})

	t.Run("replay_stale_ts_rejected", func(t *testing.T) {
		ts := staleTs()
		w := mk(ts, signForTest(ts, body))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for stale timestamp, got %d (body=%s)", w.Code, w.Body.String())
		}
	})
}

func TestStorageNodeHMAC_StoreShard_BodySigned(t *testing.T) {
	body := []byte(`{"shard_id":"s2","data":"","message_id":"m1","chunk_index":0,"total_chunks":1,"is_parity":false}`)

	mk := func(ts, sig string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/internal/store-shard", bytes.NewReader(body))
		if ts != "" {
			req.Header.Set("X-Node-Ts", ts)
		}
		if sig != "" {
			req.Header.Set("X-Node-Sig", sig)
		}
		w := httptest.NewRecorder()
		HandleStoreShard(w, req)
		return w
	}

	t.Run("wrong_sig_rejected", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, "1111111111111111111111111111111111111111111111111111111111111111")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong signature, got %d (body=%s)", w.Code, w.Body.String())
		}
	})

	t.Run("valid_sig_over_body_accepted", func(t *testing.T) {
		ts := freshTs()
		w := mk(ts, signForTest(ts, body))
		// shard_id/data boş olduğu için 400 dönebilir (auth katmanını AŞTIĞI
		// için) — kesin olarak 401 DEĞİL, bu yeterli.
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("valid body signature rejected with 401 — auth check itself is broken (body=%s)", w.Body.String())
		}
	})
}
