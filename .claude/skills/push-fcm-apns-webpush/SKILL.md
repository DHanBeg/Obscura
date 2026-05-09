---
name: push-fcm-apns-webpush
description: Push notifications via FCM HTTP v1, APNs token-based auth, Web Push API with VAPID. Use for backend/internal/push/ and any client push registration flow.
---

# Push Notifications (FCM + APNs + Web Push)

## Privacy rule (Obscura mandate)

**Push payloads NEVER contain message plaintext.** Only:
- "Yeni şifreli mesaj"
- "Yeni arama"
- conversation_id (for routing on tap)
- timestamp

Real content stays encrypted on device, decrypted by client.

## Provider abstraction (Go)

```go
// backend/internal/push/push.go
package push

import "context"

type Message struct {
    Title    string
    Body     string
    Data     map[string]string  // routing data, never content
    Priority Priority
}

type Priority int
const (
    Normal Priority = iota
    High
    VoIP  // calls
)

type Provider interface {
    Send(ctx context.Context, deviceToken string, msg Message) error
}

var Default Provider = &LogProvider{}  // dev stub

type LogProvider struct{}
func (l *LogProvider) Send(ctx context.Context, token string, msg Message) error {
    log.Printf("[PUSH STUB] token=%s title=%s", token[:8], msg.Title)
    return nil
}
```

## FCM HTTP v1 (Android)

Authentication: OAuth 2.0 with service account JSON.

```go
type FCMProvider struct {
    projectID    string
    accessToken  string
    expiresAt    time.Time
    sa           ServiceAccount
}

type ServiceAccount struct {
    ProjectID   string `json:"project_id"`
    PrivateKey  string `json:"private_key"`
    ClientEmail string `json:"client_email"`
    TokenURI    string `json:"token_uri"`
}

func (f *FCMProvider) refreshToken() error {
    if time.Now().Before(f.expiresAt) { return nil }

    // Build JWT
    now := time.Now()
    claims := jwt.MapClaims{
        "iss":   f.sa.ClientEmail,
        "scope": "https://www.googleapis.com/auth/firebase.messaging",
        "aud":   f.sa.TokenURI,
        "exp":   now.Add(time.Hour).Unix(),
        "iat":   now.Unix(),
    }
    block, _ := pem.Decode([]byte(f.sa.PrivateKey))
    privKey, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
    token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
    signed, _ := token.SignedString(privKey)

    // Exchange for access token
    resp, _ := http.PostForm(f.sa.TokenURI, url.Values{
        "grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
        "assertion":  {signed},
    })
    var body struct {
        AccessToken string `json:"access_token"`
        ExpiresIn   int    `json:"expires_in"`
    }
    json.NewDecoder(resp.Body).Decode(&body)
    f.accessToken = body.AccessToken
    f.expiresAt = time.Now().Add(time.Duration(body.ExpiresIn-60) * time.Second)
    return nil
}

func (f *FCMProvider) Send(ctx context.Context, token string, msg Message) error {
    if err := f.refreshToken(); err != nil { return err }

    payload := map[string]any{
        "message": map[string]any{
            "token": token,
            "notification": map[string]any{
                "title": msg.Title,
                "body":  msg.Body,
            },
            "data":     msg.Data,
            "android":  map[string]any{"priority": "high"},
        },
    }
    body, _ := json.Marshal(payload)

    url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", f.projectID)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+f.accessToken)
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("fcm: %d %s", resp.StatusCode, b)
    }
    return nil
}
```

## APNs (iOS) — token-based JWT

```go
type APNSProvider struct {
    keyID    string  // 10-char from Apple Developer
    teamID   string  // 10-char team ID
    bundleID string  // com.obscura.app
    privKey  *ecdsa.PrivateKey
    token    string
    issuedAt time.Time
}

func (a *APNSProvider) jwt() string {
    if time.Since(a.issuedAt) < 50*time.Minute { return a.token }
    claims := jwt.MapClaims{
        "iss": a.teamID,
        "iat": time.Now().Unix(),
    }
    tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
    tok.Header["kid"] = a.keyID
    signed, _ := tok.SignedString(a.privKey)
    a.token = signed
    a.issuedAt = time.Now()
    return signed
}

func (a *APNSProvider) Send(ctx context.Context, deviceToken string, msg Message) error {
    payload := map[string]any{
        "aps": map[string]any{
            "alert": map[string]any{"title": msg.Title, "body": msg.Body},
            "sound": "default",
            "badge": 1,
        },
    }
    for k, v := range msg.Data { payload[k] = v }
    body, _ := json.Marshal(payload)

    // Production: api.push.apple.com:443; Sandbox: api.sandbox.push.apple.com:443
    url := fmt.Sprintf("https://api.push.apple.com/3/device/%s", deviceToken)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Authorization", "bearer "+a.jwt())
    req.Header.Set("apns-topic", a.bundleID)
    req.Header.Set("apns-push-type", "alert")
    if msg.Priority == VoIP {
        req.Header.Set("apns-push-type", "voip")
        req.Header.Set("apns-priority", "10")
        req.Header.Set("apns-topic", a.bundleID+".voip")
    }

    // APNs requires HTTP/2
    transport := &http2.Transport{}
    client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("apns: %d %s", resp.StatusCode, b)
    }
    return nil
}
```

## Web Push (browser)

VAPID keys (one-time generation):
```bash
npx web-push generate-vapid-keys
# Save VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY to .env
```

Frontend (subscribe):
```ts
// frontend/lib/tauri.ts requestWebPushPermission
export async function requestWebPushPermission(): Promise<string | null> {
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) return null;
    const reg = await navigator.serviceWorker.register("/sw.js");
    await Notification.requestPermission();
    const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(VAPID_PUBLIC_KEY),
    });
    return JSON.stringify(sub);
}
```

Service Worker (`frontend/public/sw.js`):
```js
self.addEventListener("push", (event) => {
    const data = event.data.json();
    self.registration.showNotification(data.title, {
        body: data.body,
        icon: "/icon.png",
        data: { conversationId: data.conversation_id },
    });
});

self.addEventListener("notificationclick", (event) => {
    event.notification.close();
    const url = `/chats/${event.notification.data.conversationId}`;
    event.waitUntil(clients.openWindow(url));
});
```

Backend (send):
```go
// Use github.com/SherClockHolmes/webpush-go
import webpush "github.com/SherClockHolmes/webpush-go"

func sendWebPush(subscriptionJSON string, msg Message) error {
    var sub webpush.Subscription
    json.Unmarshal([]byte(subscriptionJSON), &sub)

    payload, _ := json.Marshal(map[string]string{
        "title": msg.Title, "body": msg.Body,
        "conversation_id": msg.Data["conversation_id"],
    })

    _, err := webpush.SendNotification(payload, &sub, &webpush.Options{
        VAPIDPublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
        VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
        Subscriber:      "mailto:ops@obscura.network",
        TTL:             30,
    })
    return err
}
```

## Token registration endpoint

```go
// POST /v1/devices/register
// Body: { "platform": "fcm" | "apns" | "webpush", "token": "..." }
func HandleRegisterDevice(w http.ResponseWriter, r *http.Request) {
    userDID := auth.UserDID(r)
    var req struct {
        Platform string `json:"platform"`
        Token    string `json:"token"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    var col string
    switch req.Platform {
    case "fcm":     col = "fcm_token"
    case "apns":    col = "apns_token"
    case "webpush": col = "webpush_subscription"
    default:
        respond(w, false, nil, "invalid platform"); return
    }

    _, err := db.DB.Exec(
        fmt.Sprintf("UPDATE users SET %s = ? WHERE did = ?", col),
        req.Token, userDID,
    )
    if err != nil { respond(w, false, nil, "db error"); return }
    respond(w, true, nil, "")
}
```

NOTE: Above uses `Sprintf` for column name only (whitelist-validated above) — never with user data.

## Token rotation on logout

Always clear push token on logout to prevent leaking notifications to logged-out user.

```go
// On logout
db.DB.Exec("UPDATE users SET fcm_token='', apns_token='', webpush_subscription='' WHERE did = ?", userDID)
```

## Hard rules

- Payload: title + body + routing data only — NO message content
- Token rotation on logout
- Token storage encrypted at rest (SQLCipher or app-level)
- Failed delivery: cleanup invalid tokens (FCM 404, APNs 410)
