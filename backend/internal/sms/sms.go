// Package sms — Takılır SMS adaptörü
//
// Sıfır dış bağımlılık stratejisi:
//   - Development: OTP konsola / log'a yazılır, response'da döner
//   - Production:  Pluggable provider (herhangi bir HTTP SMS API'si)
//
// Desteklenen provider'lar (dış SDK yok — saf HTTP):
//   - netgsm   : Türkiye için (GSM ile anlaşma)
//   - vodafone : Türkiye için (Vodafone Business API)
//   - twilio   : Uluslararası (en son çare — bağımlılık sayılmaz, sadece HTTP)
//   - custom   : Kendi HTTP endpoint'inizi yazın
//   - log      : Development — sadece logla
//
// Yapılandırma (env variables):
//   SMS_PROVIDER=netgsm|vodafone|custom|log
//   SMS_API_URL=https://...
//   SMS_API_KEY=...
//   SMS_FROM=ObscuraApp
package sms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ─── Provider Arayüzü ─────────────────────────────────────────────────────────

// Provider — SMS gönderme arayüzü
type Provider interface {
	SendOTP(phone, otp string) error
	Name() string
}

// ─── Global Provider ──────────────────────────────────────────────────────────

var defaultProvider Provider

func init() {
	defaultProvider = NewProvider()
}

// NewProvider — ortam değişkenlerine göre provider seç
func NewProvider() Provider {
	prov := strings.ToLower(os.Getenv("SMS_PROVIDER"))
	switch prov {
	case "netgsm":
		return &NetGSMProvider{}
	case "vodafone":
		return &VodafoneProvider{}
	case "custom":
		return &CustomHTTPProvider{
			APIURL: os.Getenv("SMS_API_URL"),
			APIKey: os.Getenv("SMS_API_KEY"),
		}
	default:
		// Development: logla
		return &LogProvider{}
	}
}

// SendOTP — OTP mesajı gönder
func SendOTP(phone, otp string) error {
	return defaultProvider.SendOTP(phone, otp)
}

// ─── LOG PROVIDER (Development) ───────────────────────────────────────────────

type LogProvider struct{}

func (p *LogProvider) Name() string { return "log" }

func (p *LogProvider) SendOTP(phone, otp string) error {
	log.Printf(
		"🔐 [SMS-DEV] OTP → %s | Kod: %s | Zaman: %s",
		phone, otp, time.Now().Format("15:04:05"),
	)
	return nil
}

// ─── NETGSM PROVIDER ─────────────────────────────────────────────────────────
//
// Netgsm Türkiye SMS API:
//   https://www.netgsm.com.tr/dokuman/
//
// Gerekli env:
//   SMS_NETGSM_USER=kullanici_kodu
//   SMS_NETGSM_PASS=sifre
//   SMS_NETGSM_HEADER=ObscuraTR

type NetGSMProvider struct{}

func (p *NetGSMProvider) Name() string { return "netgsm" }

func (p *NetGSMProvider) SendOTP(phone, otp string) error {
	user := os.Getenv("SMS_NETGSM_USER")
	pass := os.Getenv("SMS_NETGSM_PASS")
	header := os.Getenv("SMS_NETGSM_HEADER")
	if header == "" {
		header = "OBSCURA"
	}

	msg := fmt.Sprintf("Obscura dogrulama kodunuz: %s\n5 dakika gecerlidir.", otp)

	// +90 prefix'ini kaldır — Netgsm 905XXXXXXXXX bekler
	gsm := strings.TrimPrefix(phone, "+")

	apiURL := "https://api.netgsm.com.tr/sms/send/get"
	params := url.Values{
		"usercode": {user},
		"password": {pass},
		"gsmno":    {gsm},
		"message":  {msg},
		"msgheader": {header},
	}

	resp, err := http.Get(apiURL + "?" + params.Encode())
	if err != nil {
		return fmt.Errorf("netgsm HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result := strings.TrimSpace(string(body))

	// Netgsm: "00" veya "01" başarı, diğerleri hata
	if !strings.HasPrefix(result, "00") && !strings.HasPrefix(result, "01") {
		return fmt.Errorf("netgsm gönderim hatası: %s", result)
	}

	log.Printf("✅ SMS gönderildi (netgsm): %s", phone)
	return nil
}

// ─── VODAFONE PROVIDER ────────────────────────────────────────────────────────
//
// Vodafone Business SMS API (Türkiye):
//   https://adesweb.vodafone.com.tr/docs/sms-api/
//
// Gerekli env:
//   SMS_VODAFONE_KEY=api_key
//   SMS_VODAFONE_FROM=ObscuraTR

type VodafoneProvider struct{}

func (p *VodafoneProvider) Name() string { return "vodafone" }

func (p *VodafoneProvider) SendOTP(phone, otp string) error {
	apiKey := os.Getenv("SMS_VODAFONE_KEY")
	from := os.Getenv("SMS_VODAFONE_FROM")
	if from == "" {
		from = "Obscura"
	}

	msg := fmt.Sprintf("Obscura dogrulama kodu: %s (5 dakika gecerli)", otp)

	type SMSRequest struct {
		Messages []struct {
			To   []string `json:"to"`
			Body string   `json:"body"`
		} `json:"messages"`
		Sender string `json:"sender"`
	}

	reqBody := SMSRequest{
		Sender: from,
		Messages: []struct {
			To   []string `json:"to"`
			Body string   `json:"body"`
		}{{To: []string{phone}, Body: msg}},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", "https://api.vodafone.com.tr/sms/v1/send",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("vodafone istek oluşturma hatası: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("vodafone HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vodafone API hatası %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("✅ SMS gönderildi (vodafone): %s", phone)
	return nil
}

// ─── CUSTOM HTTP PROVIDER ─────────────────────────────────────────────────────
//
// Kendi SMS gateway'inizi entegre etmek için.
//
// Gerekli env:
//   SMS_API_URL=https://your-sms-gateway.com/send
//   SMS_API_KEY=your_key
//
// POST isteği gönderir:
//   {"phone": "+905551234567", "message": "Obscura kodu: 123456", "api_key": "..."}

type CustomHTTPProvider struct {
	APIURL string
	APIKey string
}

func (p *CustomHTTPProvider) Name() string { return "custom" }

func (p *CustomHTTPProvider) SendOTP(phone, otp string) error {
	if p.APIURL == "" {
		return fmt.Errorf("SMS_API_URL tanımlı değil")
	}

	msg := fmt.Sprintf("Obscura dogrulama kodunuz: %s (5 dakika gecerli)", otp)
	payload := map[string]string{
		"phone":   phone,
		"message": msg,
		"api_key": p.APIKey,
		"from":    "Obscura",
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", p.APIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("custom SMS istek hatası: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("custom SMS HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("custom SMS hatası %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("✅ SMS gönderildi (custom): %s", phone)
	return nil
}

// ─── OTP Mesaj Şablonu ────────────────────────────────────────────────────────

// OTPMessage — SMS içeriğini dil ve bölgeye göre oluştur
func OTPMessage(otp string, lang string) string {
	switch lang {
	case "tr":
		return fmt.Sprintf("Obscura dogrulama kodunuz: %s\nBu kodu kimseyle paylasmayiniz. 5 dakika gecerlidir.", otp)
	case "de":
		return fmt.Sprintf("Ihr Obscura-Verifizierungscode: %s\nGültig für 5 Minuten.", otp)
	case "ar":
		return fmt.Sprintf("رمز التحقق من Obscura: %s\nصالح لمدة 5 دقائق.", otp)
	default: // en
		return fmt.Sprintf("Your Obscura verification code: %s\nValid for 5 minutes. Do not share.", otp)
	}
}
