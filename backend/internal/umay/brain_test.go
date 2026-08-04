package umay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"obscura.network/core/internal/moderation"
)

// fakeOllama returns an httptest.Server mimicking /api/generate: it wraps
// the given classifyResult in Ollama's {"response": "<json-string>"} envelope.
func fakeOllama(t *testing.T, result classifyResult) *httptest.Server {
	t.Helper()
	inner, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal inner result: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{Response: string(inner)})
	}))
}

func TestLocalOllamaClassifier_Success(t *testing.T) {
	srv := fakeOllama(t, classifyResult{Category: moderation.CategorySpam, Confidence: 0.95})
	defer srv.Close()

	c := &LocalOllamaClassifier{
		BaseURL:    srv.URL,
		Model:      "test-model",
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	}

	v, err := c.Classify(context.Background(), "free money click here")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v.Category != moderation.CategorySpam || v.Confidence != 0.95 {
		t.Fatalf("Classify = %+v, want {spam 0.95}", v)
	}
}

func TestLocalOllamaClassifier_None(t *testing.T) {
	srv := fakeOllama(t, classifyResult{Category: CategoryNone, Confidence: 0.99})
	defer srv.Close()

	c := &LocalOllamaClassifier{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client(), Timeout: 2 * time.Second}
	v, err := c.Classify(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v.Category != CategoryNone {
		t.Fatalf("Category = %q, want %q", v.Category, CategoryNone)
	}
}

func TestLocalOllamaClassifier_UnknownCategory(t *testing.T) {
	srv := fakeOllama(t, classifyResult{Category: "not_a_real_category", Confidence: 0.5})
	defer srv.Close()

	c := &LocalOllamaClassifier{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client(), Timeout: 2 * time.Second}
	if _, err := c.Classify(context.Background(), "x"); err == nil {
		t.Fatal("Classify: want error for unknown category, got nil")
	}
}

func TestLocalOllamaClassifier_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &LocalOllamaClassifier{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client(), Timeout: 2 * time.Second}
	if _, err := c.Classify(context.Background(), "x"); err == nil {
		t.Fatal("Classify: want error on HTTP 500, got nil")
	}
}

func TestLocalOllamaClassifier_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{Response: `{"category":"none","confidence":1}`})
	}))
	defer srv.Close()

	c := &LocalOllamaClassifier{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client(), Timeout: 5 * time.Millisecond}
	if _, err := c.Classify(context.Background(), "x"); err == nil {
		t.Fatal("Classify: want timeout error, got nil")
	}
}

// mockClassifier — brain'in orkestrasyon mantığını (DualClassifier, circuit
// breaker) gerçek ağ çağrısı olmadan test etmek için.
type mockClassifier struct {
	verdict     Verdict
	err         error
	calls       int
	lastContent string
}

func (m *mockClassifier) Classify(ctx context.Context, content string) (Verdict, error) {
	m.calls++
	m.lastContent = content
	return m.verdict, m.err
}

func TestDualClassifier_ConfidentLocal_NoFallbackCall(t *testing.T) {
	local := &mockClassifier{verdict: Verdict{Category: moderation.CategorySpam, Confidence: 0.95}}
	fallback := &mockClassifier{verdict: Verdict{Category: moderation.CategorySpam, Confidence: 0.99}}
	d := NewDualClassifier(local, fallback)

	v, err := d.Classify(context.Background(), "x")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v.Confidence != 0.95 {
		t.Fatalf("Confidence = %v, want local's 0.95 (fallback should not have been used)", v.Confidence)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback.calls = %d, want 0 (confident local verdict)", fallback.calls)
	}
}

func TestDualClassifier_UncertainLocal_CallsFallback(t *testing.T) {
	local := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.5}} // in [0.4,0.7]
	fallback := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.85}}
	d := NewDualClassifier(local, fallback)

	v, err := d.Classify(context.Background(), "x")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback.calls = %d, want 1 (uncertain local verdict)", fallback.calls)
	}
	if v.Confidence != 0.85 {
		t.Fatalf("Confidence = %v, want fallback's 0.85", v.Confidence)
	}
}

func TestDualClassifier_UncertainLocal_FallbackErrors_KeepsLocal(t *testing.T) {
	local := &mockClassifier{verdict: Verdict{Category: moderation.CategoryScam, Confidence: 0.6}}
	fallback := &mockClassifier{err: fmt.Errorf("groq unavailable")}
	d := NewDualClassifier(local, fallback)

	v, err := d.Classify(context.Background(), "x")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v.Confidence != 0.6 {
		t.Fatalf("Confidence = %v, want local's 0.6 kept on fallback error", v.Confidence)
	}
}

func TestDualClassifier_LocalError_Propagates(t *testing.T) {
	local := &mockClassifier{err: fmt.Errorf("ollama down")}
	d := NewDualClassifier(local, &mockClassifier{})

	if _, err := d.Classify(context.Background(), "x"); err == nil {
		t.Fatal("Classify: want error when local fails, got nil")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	local := &mockClassifier{err: fmt.Errorf("ollama down")}
	d := NewDualClassifier(local, &mockClassifier{})
	ctx := context.Background()

	for i := 0; i < circuitBreakerFailThreshold; i++ {
		if _, err := d.Classify(ctx, "x"); err == nil {
			t.Fatalf("call %d: want error, got nil", i)
		}
	}
	if local.calls != circuitBreakerFailThreshold {
		t.Fatalf("local.calls = %d, want %d before breaker opens", local.calls, circuitBreakerFailThreshold)
	}

	// Breaker now open: local must NOT be called again.
	if _, err := d.Classify(ctx, "x"); err == nil {
		t.Fatal("Classify: want circuit-open error, got nil")
	}
	if local.calls != circuitBreakerFailThreshold {
		t.Fatalf("local.calls = %d after breaker open, want unchanged %d", local.calls, circuitBreakerFailThreshold)
	}
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := &circuitBreaker{}
	cb.RecordResult(fmt.Errorf("e1"))
	cb.RecordResult(fmt.Errorf("e2"))
	cb.RecordResult(nil) // success resets the streak
	cb.RecordResult(fmt.Errorf("e3"))
	if !cb.Allow() {
		t.Fatal("Allow() = false, want true (streak reset by success, only 1 failure since)")
	}
}
