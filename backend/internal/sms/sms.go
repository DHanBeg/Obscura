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

	"obscura.network/core/internal/logredact"
	"obscura.network/core/internal/secrets"
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
	// (C10 fail-open kökü kapatıldı — behavioral grep sonrası bulunan 7.
	// kardeş, isimli listede yoktu) Eskiden yalnızca OBSCURA_ENV TAM OLARAK
	// "production" ise fatal oluyordu (opt-out yönü) — env unutulur ya da
	// "staging"/typo olursa SMS_PROVIDER=log sessizce prod'da aktif kalır,
	// OTP kodları gerçek SMS yerine sunucu loglarına düşerdi (log erişimi
	// olan biri için hesap ele geçirme riski). secrets.IsDev() ile D1
	// fail-safe: açık dev opt-in değilse prod sayılır.
	if _, ok := defaultProvider.(*LogProvider); ok && !secrets.IsDev() {
		log.Fatal("SMS_PROVIDER env zorunlu (OBSCURA_ENV açık dev opt-in değilse prod sayılır; netgsm|vodafone|twilio|custom) — şu an 'log' aktif, OTP gönderilmez")
	}
	if _, ok := defaultProvider.(*LogProvider); ok {
		log.Println("⚠️  SMS_PROVIDER=log — OTP sadece loglara yazılır, gerçek SMS gönderilmez (dev modu)")
	} else {
		log.Printf("✅ SMS provider: %s", defaultProvider.Name())
	}
}

// NewProvider — ortam değişkenlerine göre provider seç
func NewProvider() Provider {
	prov := strings.ToLower(os.Getenv("SMS_PROVIDER"))
	switch prov {
	case "netgsm":
		return &NetGSMProvider{}
	case "vodafone":
		return &VodafoneProvider{}
	case "twilio":
		return &TwilioProvider{}
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
		logredact.Phone(phone), otp, time.Now().Format("15:04:05"),
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

	msg := buildOTPMessage(otp)

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

	log.Printf("✅ SMS gönderildi (netgsm): %s", logredact.Phone(phone))
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

	msg := buildOTPMessage(otp)

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

	log.Printf("✅ SMS gönderildi (vodafone): %s", logredact.Phone(phone))
	return nil
}

// ─── TWILIO PROVIDER ─────────────────────────────────────────────────────────
//
// Twilio Programmable Messaging API (sıfır SDK — saf HTTP + Basic Auth).
//
// Gerekli env:
//   TWILIO_ACCOUNT_SID=ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
//   TWILIO_AUTH_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
//   TWILIO_FROM=+19066673349

type TwilioProvider struct{}

func (p *TwilioProvider) Name() string { return "twilio" }

func (p *TwilioProvider) SendOTP(phone, otp string) error {
	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	token := os.Getenv("TWILIO_AUTH_TOKEN")
	from := os.Getenv("TWILIO_FROM")

	if sid == "" || token == "" || from == "" {
		return fmt.Errorf("twilio: TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN ve TWILIO_FROM tanımlı olmalı")
	}

	// TWILIO_SENDER_ID varsa alfanümerik gönderici adı kullan (ücretli hesap + ülke desteği gerekir).
	// Örnek: TWILIO_SENDER_ID=Obscura
	if senderID := os.Getenv("TWILIO_SENDER_ID"); senderID != "" {
		from = senderID
	}

	msg := buildOTPMessage(otp)

	body := url.Values{}
	body.Set("To", phone)
	body.Set("From", from)
	body.Set("Body", msg)

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", sid)
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(body.Encode()))
	if err != nil {
		return fmt.Errorf("twilio istek oluşturma hatası: %w", err)
	}
	req.SetBasicAuth(sid, token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio API hatası %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("✅ SMS gönderildi (twilio): %s", logredact.Phone(phone))
	return nil
}

// buildOTPMessage — standart OTP mesaj formatı
// GSM-7 alfabe ile sinirli tutulur (160 karakter / segment, UCS-2 yerine).
func buildOTPMessage(otp string) string {
	return fmt.Sprintf(
		"Obscura kodunuz: %s\nBu kodu kimseyle paylasmayin. 5 dakika gecerlidir.",
		otp,
	)
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

	msg := buildOTPMessage(otp)
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

	log.Printf("✅ SMS gönderildi (custom): %s", logredact.Phone(phone))
	return nil
}

// ─── OTP Mesaj Şablonu ────────────────────────────────────────────────────────

// OTPMessage — SMS içeriğini dil ve bölgeye göre oluştur
func OTPMessage(otp string, lang string) string {
	return buildOTPMessage(otp)
}
