---
name: minio-aws-sigv4
description: Implement MinIO/S3 client from scratch using AWS Signature v4, no external SDK. Use when working with backend/internal/media/ or any S3-compatible storage in Obscura.
---

# MinIO + AWS Signature v4 (No SDK)

## Why no SDK

Obscura principle: zero external service dependencies. The aws-sdk-go pulls in 50+ transitive deps. We implement Sig v4 directly in ~200 lines.

## Config

```go
type Config struct {
    Endpoint   string // "minio:9000"
    Region     string // "us-east-1" (MinIO default)
    AccessKey  string
    SecretKey  string
    Bucket     string
    UseSSL     bool
    PublicURL  string // optional CDN-fronted URL
}

func LoadConfig() *Config {
    return &Config{
        Endpoint:  getenv("MINIO_ENDPOINT", "minio:9000"),
        Region:    getenv("MINIO_REGION", "us-east-1"),
        AccessKey: os.Getenv("MINIO_ACCESS_KEY"),
        SecretKey: os.Getenv("MINIO_SECRET_KEY"),
        Bucket:    getenv("MINIO_BUCKET", "obscura-media"),
        UseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
        PublicURL: os.Getenv("MINIO_PUBLIC_URL"),
    }
}
```

## Sign request (Sig v4)

```go
func sign(req *http.Request, body []byte, cfg *Config) {
    now := time.Now().UTC()
    amzDate := now.Format("20060102T150405Z")
    dateStamp := now.Format("20060102")

    // Hash body
    bodyHash := sha256Hex(body)
    req.Header.Set("X-Amz-Content-Sha256", bodyHash)
    req.Header.Set("X-Amz-Date", amzDate)
    req.Header.Set("Host", req.Host)

    // Canonical request
    canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
        req.Host, bodyHash, amzDate)
    signedHeaders := "host;x-amz-content-sha256;x-amz-date"
    canonicalQuery := req.URL.RawQuery

    canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
        req.Method, req.URL.Path, canonicalQuery,
        canonicalHeaders, signedHeaders, bodyHash)

    // String to sign
    scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, cfg.Region)
    stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
        amzDate, scope, sha256Hex([]byte(canonicalRequest)))

    // Derive signing key
    kDate := hmacSHA256([]byte("AWS4"+cfg.SecretKey), []byte(dateStamp))
    kRegion := hmacSHA256(kDate, []byte(cfg.Region))
    kService := hmacSHA256(kRegion, []byte("s3"))
    kSigning := hmacSHA256(kService, []byte("aws4_request"))

    signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

    // Authorization header
    auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
        cfg.AccessKey, scope, signedHeaders, signature)
    req.Header.Set("Authorization", auth)
}

func sha256Hex(b []byte) string {
    h := sha256.Sum256(b)
    return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
    h := hmac.New(sha256.New, key)
    h.Write(data)
    return h.Sum(nil)
}
```

## Upload

```go
func Upload(ctx context.Context, key string, body []byte, contentType string) (string, error) {
    cfg := LoadConfig()
    scheme := "http"
    if cfg.UseSSL { scheme = "https" }
    url := fmt.Sprintf("%s://%s/%s/%s", scheme, cfg.Endpoint, cfg.Bucket, key)

    req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
    if err != nil { return "", err }
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Content-Length", strconv.Itoa(len(body)))

    sign(req, body, cfg)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()

    if resp.StatusCode >= 300 {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("upload failed: %d %s", resp.StatusCode, body)
    }

    publicURL := url
    if cfg.PublicURL != "" {
        publicURL = fmt.Sprintf("%s/%s/%s", cfg.PublicURL, cfg.Bucket, key)
    }
    return publicURL, nil
}
```

## Delete

```go
func Delete(ctx context.Context, key string) error {
    cfg := LoadConfig()
    scheme := "http"
    if cfg.UseSSL { scheme = "https" }
    url := fmt.Sprintf("%s://%s/%s/%s", scheme, cfg.Endpoint, cfg.Bucket, key)

    req, _ := http.NewRequestWithContext(ctx, "DELETE", url, nil)
    sign(req, nil, cfg)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        return fmt.Errorf("delete failed: %d", resp.StatusCode)
    }
    return nil
}
```

## Bucket setup (one-time)

```bash
# Via mc (MinIO client)
docker run --rm -it minio/mc:latest \
  alias set obs http://minio:9000 obscura "$MINIO_SECRET_KEY"

docker run --rm -it minio/mc:latest mb obs/obscura-media
docker run --rm -it minio/mc:latest anonymous set download obs/obscura-media
```

## Tier-based size limits (Obscura)

```go
func MaxUploadSize(tier int) int64 {
    switch tier {
    case 1: return 0           // Bronze: no media
    case 2: return 5 << 20     // Silver: 5MB
    case 3: return 50 << 20    // Gold: 50MB
    case 4: return 100 << 20   // Platinum
    case 5: return 100 << 20   // Diamond
    default: return 1 << 20
    }
}
```

## Common errors

- `SignatureDoesNotMatch` — clock skew (>5 min off NTP), or query params not in canonical request
- `403 Forbidden` — bucket policy blocks anonymous access; use `mc anonymous set download`
- `Connection refused` — MINIO_ENDPOINT wrong (use service name in Docker, not localhost)
- Missing `Host` header — Go's http.Request.Host vs Header.Host inconsistency; set both
