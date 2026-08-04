package umay

// brain.go — sınıflandırma katmanı (spec Bölüm 1.4, "brain"). Yerel model
// birincil, cloud yalnızca kararsızlıkta (maliyet ilkesi). Kategori sözlüğü
// internal/moderation'ın kapalı Bölüm 1.2 listesiyle aynı — burada
// tekrarlanmaz.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"obscura.network/core/internal/moderation"
)

// Kararsızlık bandı (spec 1.4): bu aralıkta yerel sonuca güvenilmez, cloud
// fallback devreye girer. Sabit — config'e taşımak şu an gerekli değil,
// ihtiyaç çıkarsa env var eklenir.
const (
	uncertainConfidenceLow  = 0.4
	uncertainConfidenceHigh = 0.7
)

func classifyTimeout() time.Duration {
	raw := os.Getenv("OBSCURA_UMAY_CLASSIFY_TIMEOUT_MS")
	if raw == "" {
		return 5 * time.Second
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 5 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

// ─── LocalOllamaClassifier ─────────────────────────────────────────────────

// LocalOllamaClassifier calls a local Ollama server's /api/generate with
// format:"json" so the model returns a parseable {"category","confidence"}
// object instead of free text. Fields are exported/constructible so tests
// can point BaseURL at an httptest.Server without touching env vars.
type LocalOllamaClassifier struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewLocalOllamaClassifier builds a classifier from
// OBSCURA_UMAY_OLLAMA_URL / OBSCURA_UMAY_OLLAMA_MODEL, defaulting to the
// standard local Ollama port and "mistral" (spec 1.3: "Yerel model
// (Ollama/Mistral)").
func NewLocalOllamaClassifier() *LocalOllamaClassifier {
	baseURL := os.Getenv("OBSCURA_UMAY_OLLAMA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := os.Getenv("OBSCURA_UMAY_OLLAMA_MODEL")
	if model == "" {
		model = "mistral"
	}
	return &LocalOllamaClassifier{
		BaseURL:    baseURL,
		Model:      model,
		HTTPClient: &http.Client{},
		Timeout:    classifyTimeout(),
	}
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Format string `json:"format"`
	Stream bool   `json:"stream"`
}

// ollamaGenerateResponse is Ollama's envelope; Response holds the model's
// raw output — since Format:"json" was requested, Response is itself a JSON
// string that decodes into classifyResult.
type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

type classifyResult struct {
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
}

func classifyPrompt(content string) string {
	return fmt.Sprintf(`Aşağıdaki kamusal içeriği şu kategorilerden birine sınıflandır:
spam, scam, harassment, copyright, illegal_sale, child_safety, none (ihlal yok).
Yalnızca şu JSON şemasıyla yanıt ver: {"category":"<kategori>","confidence":<0..1>}

İçerik: %q`, content)
}

func (c *LocalOllamaClassifier) Classify(ctx context.Context, content string) (Verdict, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	reqBody, err := json.Marshal(ollamaGenerateRequest{
		Model:  c.Model,
		Prompt: classifyPrompt(content),
		Format: "json",
		Stream: false,
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: ollama istek gövdesi: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: ollama istek oluşturma: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: ollama istek hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Verdict{}, fmt.Errorf("umay: ollama beklenmeyen durum kodu: %d", resp.StatusCode)
	}

	var envelope ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Verdict{}, fmt.Errorf("umay: ollama gövde çözümleme: %w", err)
	}

	var result classifyResult
	if err := json.Unmarshal([]byte(envelope.Response), &result); err != nil {
		return Verdict{}, fmt.Errorf("umay: sınıflandırma sonucu çözümleme: %w", err)
	}
	if result.Category != CategoryNone && !moderation.IsKnownCategory(result.Category) {
		return Verdict{}, fmt.Errorf("umay: bilinmeyen kategori: %q", result.Category)
	}
	return Verdict{Category: result.Category, Confidence: result.Confidence}, nil
}

// ─── CloudFallbackClassifier (Groq) ────────────────────────────────────────

// CloudFallbackClassifier calls Groq's OpenAI-compatible chat completions
// API. Only invoked by DualClassifier when the local verdict's confidence
// falls in the uncertain band — spec 1.4's cost principle: cloud is not the
// primary path.
type CloudFallbackClassifier struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewGroqFallbackClassifier builds a classifier from
// OBSCURA_UMAY_GROQ_API_KEY / OBSCURA_UMAY_GROQ_MODEL. If no API key is
// configured, Classify always errors — DualClassifier treats that as "no
// fallback available" and keeps the local (uncertain) verdict rather than
// failing the whole scan.
func NewGroqFallbackClassifier() *CloudFallbackClassifier {
	model := os.Getenv("OBSCURA_UMAY_GROQ_MODEL")
	if model == "" {
		model = "llama-3.1-8b-instant"
	}
	return &CloudFallbackClassifier{
		BaseURL:    "https://api.groq.com/openai/v1",
		Model:      model,
		APIKey:     os.Getenv("OBSCURA_UMAY_GROQ_API_KEY"),
		HTTPClient: &http.Client{},
		Timeout:    classifyTimeout(),
	}
}

type groqChatRequest struct {
	Model          string            `json:"model"`
	Messages       []groqChatMessage `json:"messages"`
	ResponseFormat groqResponseFmt   `json:"response_format"`
}

type groqChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponseFmt struct {
	Type string `json:"type"`
}

type groqChatResponse struct {
	Choices []struct {
		Message groqChatMessage `json:"message"`
	} `json:"choices"`
}

func (c *CloudFallbackClassifier) Classify(ctx context.Context, content string) (Verdict, error) {
	if c.APIKey == "" {
		return Verdict{}, fmt.Errorf("umay: groq api anahtarı ayarlanmamış (OBSCURA_UMAY_GROQ_API_KEY)")
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	reqBody, err := json.Marshal(groqChatRequest{
		Model: c.Model,
		Messages: []groqChatMessage{
			{Role: "user", Content: classifyPrompt(content)},
		},
		ResponseFormat: groqResponseFmt{Type: "json_object"},
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: groq istek gövdesi: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: groq istek oluşturma: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: groq istek hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Verdict{}, fmt.Errorf("umay: groq beklenmeyen durum kodu: %d", resp.StatusCode)
	}

	var envelope groqChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Verdict{}, fmt.Errorf("umay: groq gövde çözümleme: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return Verdict{}, fmt.Errorf("umay: groq yanıtı boş")
	}

	var result classifyResult
	if err := json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &result); err != nil {
		return Verdict{}, fmt.Errorf("umay: groq sınıflandırma sonucu çözümleme: %w", err)
	}
	if result.Category != CategoryNone && !moderation.IsKnownCategory(result.Category) {
		return Verdict{}, fmt.Errorf("umay: groq bilinmeyen kategori: %q", result.Category)
	}
	return Verdict{Category: result.Category, Confidence: result.Confidence}, nil
}

// ─── Circuit breaker ────────────────────────────────────────────────────────

// circuitBreakerFailThreshold ardışık yerel-sınıflandırma hatasından sonra
// devre açılır; circuitBreakerCooldown boyunca yerel çağrı hiç denenmez.
// Amaç: Ollama çökerse monitor'un scan goroutine'i her mesaj için ayrı ayrı
// timeout beklemek zorunda kalmasın (scanner.go'nun "biri hata verirse
// diğerleri etkilenmez" ilkesiyle aynı ruhta — burada "diğerleri" aynı
// goroutine'deki sıradaki mesajlar).
const (
	circuitBreakerFailThreshold = 3
	circuitBreakerCooldown      = 2 * time.Minute
)

type circuitBreaker struct {
	mu              sync.Mutex
	consecutiveFail int
	openUntil       time.Time
}

func (cb *circuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.openUntil.IsZero() || time.Now().After(cb.openUntil)
}

func (cb *circuitBreaker) RecordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err == nil {
		cb.consecutiveFail = 0
		cb.openUntil = time.Time{}
		return
	}
	cb.consecutiveFail++
	if cb.consecutiveFail >= circuitBreakerFailThreshold {
		cb.openUntil = time.Now().Add(circuitBreakerCooldown)
	}
}

// ─── DualClassifier ─────────────────────────────────────────────────────────

// DualClassifier is the production Classifier: local first, cloud fallback
// only on uncertain confidence, circuit breaker over the local leg.
type DualClassifier struct {
	Local    Classifier
	Fallback Classifier
	breaker  *circuitBreaker
}

func NewDualClassifier(local, fallback Classifier) *DualClassifier {
	return &DualClassifier{Local: local, Fallback: fallback, breaker: &circuitBreaker{}}
}

func (d *DualClassifier) Classify(ctx context.Context, content string) (Verdict, error) {
	if !d.breaker.Allow() {
		return Verdict{}, fmt.Errorf("umay: yerel sınıflandırıcı devre-kesici açık (ardışık hata eşiği aşıldı)")
	}

	v, err := d.Local.Classify(ctx, content)
	d.breaker.RecordResult(err)
	if err != nil {
		return Verdict{}, fmt.Errorf("umay: yerel sınıflandırma hatası: %w", err)
	}

	if v.Confidence >= uncertainConfidenceLow && v.Confidence <= uncertainConfidenceHigh && d.Fallback != nil {
		fv, ferr := d.Fallback.Classify(ctx, content)
		if ferr == nil {
			return fv, nil
		}
		log.Printf("[Umay] cloud fallback başarısız, yerel (kararsız) sonuç kullanılıyor: %v", ferr)
	}

	return v, nil
}
