---
name: webrtc-coturn-self-hosted
description: Self-hosted coturn TURN/STUN server for Obscura WebRTC voice/video calls. Includes SSL, time-limited credentials, NAT config.
---

# coturn TURN Server (Self-Hosted)

## Why self-host

Spec rule: zero external service dependencies. Twilio TURN costs $0.40/GB. coturn on a $20/mo VPS handles thousands of users.

## Server requirements

- 2 vCPU, 4GB RAM, 100GB SSD
- Ubuntu 22.04 LTS
- Public IP (no NAT, no Cloudflare proxy)
- Open ports: 3478 TCP/UDP, 5349 TCP/UDP, 49152-65535 UDP (relay range)

## Install

```bash
sudo apt update && sudo apt install -y coturn
sudo systemctl enable coturn
```

Or via Docker (Obscura uses this):

```yaml
# docker-compose.yml
coturn:
  image: coturn/coturn:latest
  network_mode: host          # MUST — TURN needs full UDP port range
  restart: unless-stopped
  volumes:
    - ./coturn/turnserver.conf:/etc/turnserver.conf:ro
    - ./coturn/ssl:/etc/coturn/ssl:ro
```

## Config (`coturn/turnserver.conf`)

```conf
# Listening
listening-port=3478
tls-listening-port=5349
listening-ip=0.0.0.0
relay-ip=YOUR_PUBLIC_IP
external-ip=YOUR_PUBLIC_IP

# Auth: REST API mode (time-limited credentials, no static users)
use-auth-secret
static-auth-secret=YOUR_LONG_RANDOM_SECRET    # match TURN_SECRET in backend env
realm=obscura.network
server-name=turn.obscura.network

# TLS
cert=/etc/coturn/ssl/fullchain.pem
pkey=/etc/coturn/ssl/privkey.pem

# Limits
total-quota=100
user-quota=12
max-bps=3000000

# Relay range
min-port=49152
max-port=65535

# Disable old/insecure
no-cli
no-tlsv1
no-tlsv1_1
no-stdout-log
no-multicast-peers
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
denied-peer-ip=169.254.0.0-169.254.255.255

# Logging
log-file=stdout
verbose
```

## Backend issues time-limited credentials

```go
// backend/internal/api/webrtc.go
func HandleTurnCredentials(w http.ResponseWriter, r *http.Request) {
    userDID := auth.UserDID(r)
    if userDID == "" {
        respond(w, false, nil, "unauthorized"); return
    }

    secret := os.Getenv("TURN_SECRET")
    host := getenv("TURN_HOST", "turn.obscura.network")

    // Username = expiry_unix:userDID
    // Password = base64(HMAC-SHA1(secret, username))
    expiry := time.Now().Add(1 * time.Hour).Unix()
    username := fmt.Sprintf("%d:%s", expiry, userDID)

    h := hmac.New(sha1.New, []byte(secret))
    h.Write([]byte(username))
    password := base64.StdEncoding.EncodeToString(h.Sum(nil))

    respond(w, true, map[string]any{
        "iceServers": []map[string]any{
            {"urls": []string{"stun:" + host + ":3478"}},
            {
                "urls": []string{
                    "turn:" + host + ":3478?transport=udp",
                    "turn:" + host + ":3478?transport=tcp",
                    "turns:" + host + ":5349?transport=tcp",
                },
                "username":   username,
                "credential": password,
            },
        },
        "ttl": 3600,
    }, "")
}
```

## Client integration

```ts
// frontend/lib/call.ts
const { iceServers } = await api.getTurnCredentials();
const pc = new RTCPeerConnection({ iceServers });

pc.onicecandidate = (e) => {
    if (e.candidate) sendSignal({ type: "candidate", candidate: e.candidate });
};
```

## Test TURN

Open: https://webrtc.github.io/samples/src/content/peerconnection/trickle-ice/

Add:
- STUN URL: stun:turn.obscura.network:3478
- TURN URL: turn:turn.obscura.network:3478
- Username + password from backend

Expected: at least one `srflx` (server reflexive) and one `relay` candidate.

## SSL cert (Let's Encrypt)

```bash
# DNS-01 challenge (since coturn uses port 80?... actually use HTTP-01 by stopping coturn briefly)
sudo certbot certonly --standalone -d turn.obscura.network --pre-hook "systemctl stop coturn" --post-hook "systemctl start coturn"

# Symlink for coturn
sudo ln -sf /etc/letsencrypt/live/turn.obscura.network/fullchain.pem /etc/coturn/ssl/fullchain.pem
sudo ln -sf /etc/letsencrypt/live/turn.obscura.network/privkey.pem /etc/coturn/ssl/privkey.pem
```

## Monitoring

coturn logs include relay stats. Pipe to Loki/Prometheus.

Metrics endpoint (coturn 4.5.2+):
```conf
prometheus
```

Then scrape from Prometheus:
```yaml
- job_name: coturn
  static_configs:
    - targets: ['turn.obscura.network:9641']
```

## Rules

- Credentials expire in 1 hour (renew on call start)
- TURN over TLS (`turns://`) for clients on networks blocking UDP
- `network_mode: host` for Docker (UDP port range issue)
- Deny private IP ranges (`denied-peer-ip`) to prevent SSRF via TURN relay
- TLS 1.2+ only
