// Package media — MinIO S3-compatible medya depolama
//
// Env değişkenleri:
//   MINIO_ENDPOINT    — MinIO endpoint (varsayılan: localhost:9000)
//   MINIO_ACCESS_KEY  — Erişim anahtarı
//   MINIO_SECRET_KEY  — Gizli anahtar
//   MINIO_BUCKET      — Bucket adı (varsayılan: obscura-media)
//   MINIO_USE_SSL     — TLS kullan (varsayılan: false dev'de)
//
// Harici SDK yok — standart HTTP ile S3 REST API kullanılır.
// Production'da minio/minio-go tercih edilebilir; bu sürüm sıfır-bağımlılıklı.
package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─── Yapılandırma ─────────────────────────────────────────────────────────────

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	BaseURL   string // public URL prefix
}

var cfg Config

func init() {
	cfg = Config{
		Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey: getEnv("MINIO_ACCESS_KEY", "obscura-admin"),
		SecretKey: getEnv("MINIO_SECRET_KEY", "obscura-secret"),
		Bucket:    getEnv("MINIO_BUCKET", "obscura-media"),
		UseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
	}

	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	cfg.BaseURL = fmt.Sprintf("%s://%s/%s", scheme, cfg.Endpoint, cfg.Bucket)
}

// ─── Upload ────────────────────────────────────────────────────────────────────

// Upload — nesneyi MinIO'ya yükler, public URL döner
func Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s/%s/%s", scheme, cfg.Endpoint, cfg.Bucket, key)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, body)
	if err != nil {
		return "", fmt.Errorf("MinIO PUT isteği: %w", err)
	}

	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-amz-acl", "public-read")

	// AWS Signature v4 imzala
	now := time.Now().UTC()
	signRequest(req, now, cfg.AccessKey, cfg.SecretKey, cfg.Bucket, key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("MinIO HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MinIO hata %d: %s", resp.StatusCode, string(body))
	}

	publicURL := fmt.Sprintf("%s/%s", cfg.BaseURL, key)
	log.Printf("📦 Medya yüklendi: %s (%d bytes)", key, size)
	return publicURL, nil
}

// Delete — nesneyi MinIO'dan sil
func Delete(ctx context.Context, key string) error {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s/%s/%s", scheme, cfg.Endpoint, cfg.Bucket, key)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("MinIO DELETE isteği: %w", err)
	}

	now := time.Now().UTC()
	signRequest(req, now, cfg.AccessKey, cfg.SecretKey, cfg.Bucket, key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode != 404 {
		return fmt.Errorf("MinIO DELETE hata %d", resp.StatusCode)
	}
	return nil
}

// Download — nesneyi MinIO'dan çeker, içeriği []byte olarak döner.
// 404 için (nil, nil) döner; diğer hatalar için hata döner.
func Download(ctx context.Context, key string) ([]byte, error) {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	objectURL := fmt.Sprintf("%s://%s/%s/%s", scheme, cfg.Endpoint, cfg.Bucket, key)

	req, err := http.NewRequestWithContext(ctx, "GET", objectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("MinIO GET isteği: %w", err)
	}

	now := time.Now().UTC()
	signRequestGet(req, now, cfg.AccessKey, cfg.SecretKey, cfg.Bucket, key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MinIO HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MinIO GET hata %d: %s", resp.StatusCode, string(b))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("MinIO okuma: %w", err)
	}
	return data, nil
}

// ─── AWS Signature v4 ────────────────────────────────────────────────────────
// MinIO, AWS S3 Signature v4'ü destekler.
// Bu implementasyon PUT ve DELETE için yeterlidir.

func signRequest(req *http.Request, t time.Time, accessKey, secretKey, bucket, key string) {
	datestamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")
	region := "us-east-1" // MinIO varsayılan region

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")

	// Canonical request
	signedHeaders := "content-type;host;x-amz-acl;x-amz-content-sha256;x-amz-date"
	if req.Method == "DELETE" {
		signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	}

	host := req.URL.Host
	canonicalRequest := strings.Join([]string{
		req.Method,
		"/" + bucket + "/" + key,
		"",
		"host:" + host + "\n" +
			"x-amz-content-sha256:UNSIGNED-PAYLOAD\n" +
			"x-amz-date:" + amzDate + "\n",
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	// String to sign
	credentialScope := datestamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" +
		hex.EncodeToString(hashSHA256([]byte(canonicalRequest)))

	// Signing key
	signingKey := hmacSHA256(
		hmacSHA256(
			hmacSHA256(
				hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp)),
				[]byte(region),
			),
			[]byte("s3"),
		),
		[]byte("aws4_request"),
	)

	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		accessKey, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)
}

// signRequestGet — GET için Signature v4 (signed headers: host + amz-date + amz-content-sha256)
func signRequestGet(req *http.Request, t time.Time, accessKey, secretKey, bucket, key string) {
	datestamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")
	region := "us-east-1"

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", "UNSIGNED-PAYLOAD")

	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		"GET",
		"/" + bucket + "/" + key,
		"",
		"host:" + req.URL.Host,
		"x-amz-content-sha256:UNSIGNED-PAYLOAD",
		"x-amz-date:" + amzDate,
		"",
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	credentialScope := datestamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" +
		hex.EncodeToString(hashSHA256([]byte(canonicalRequest)))

	signingKey := hmacSHA256(
		hmacSHA256(
			hmacSHA256(
				hmacSHA256([]byte("AWS4"+secretKey), []byte(datestamp)),
				[]byte(region),
			),
			[]byte("s3"),
		),
		[]byte("aws4_request"),
	)

	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		accessKey, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func hashSHA256(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
