---
name: nginx-multinode-lb
description: nginx config for Obscura — 5-node load balancing, WebSocket upgrade, TLS termination, sticky sessions. Use when editing nginx/nginx.conf.
---

# nginx Multi-Node LB

## Full config (`nginx/nginx.conf`)

```nginx
worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main escape=json '{"time":"$time_iso8601","remote_addr":"$remote_addr","method":"$request_method","uri":"$request_uri","status":$status,"bytes":$body_bytes_sent,"rt":$request_time,"upstream":"$upstream_addr"}';
    access_log /var/log/nginx/access.log main;
    error_log /var/log/nginx/error.log warn;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    keepalive_requests 1000;
    types_hash_max_size 2048;
    server_tokens off;

    client_max_body_size 100M;        # max upload size (Diamond tier)
    client_body_timeout 60s;
    client_header_timeout 60s;
    send_timeout 60s;

    # Gzip (NOT for WebSocket)
    gzip on;
    gzip_types text/plain application/json application/javascript text/css;
    gzip_vary on;

    # Rate limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;
    limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/s;

    # ─── Upstream pool ────────────────────────────────────────────────
    upstream obscura_backend {
        # ip_hash;  # uncomment for sticky sessions (WS)
        least_conn;
        server node1:8080 max_fails=3 fail_timeout=10s;
        server node2:8080 max_fails=3 fail_timeout=10s;
        server node3:8080 max_fails=3 fail_timeout=10s;
        server node4:8080 max_fails=3 fail_timeout=10s;
        server node5:8080 max_fails=3 fail_timeout=10s;
        keepalive 32;
    }

    # ─── HTTP → HTTPS redirect ────────────────────────────────────────
    server {
        listen 80;
        listen [::]:80;
        server_name api.obscura.network obscura.network;

        location /.well-known/acme-challenge/ {
            root /var/www/certbot;
        }
        location / {
            return 301 https://$host$request_uri;
        }
    }

    # ─── Main HTTPS server ────────────────────────────────────────────
    server {
        listen 443 ssl http2;
        listen [::]:443 ssl http2;
        server_name api.obscura.network;

        # TLS
        ssl_certificate /etc/nginx/ssl/fullchain.pem;
        ssl_certificate_key /etc/nginx/ssl/privkey.pem;
        ssl_protocols TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;
        ssl_prefer_server_ciphers off;
        ssl_session_cache shared:SSL:10m;
        ssl_session_timeout 1d;
        ssl_session_tickets off;
        ssl_stapling on;
        ssl_stapling_verify on;

        # Security headers
        add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
        add_header X-Frame-Options DENY always;
        add_header X-Content-Type-Options nosniff always;
        add_header Referrer-Policy "no-referrer" always;
        add_header Content-Security-Policy "default-src 'self'" always;

        # ── Auth endpoints (stricter rate limit) ────────────────────
        location ~ ^/v1/auth/ {
            limit_req zone=auth burst=10 nodelay;
            proxy_pass http://obscura_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto https;
        }

        # ── WebSocket stream ─────────────────────────────────────────
        location /v1/stream {
            proxy_pass http://obscura_backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_read_timeout 3600s;          # 1 hour for long-lived WS
            proxy_send_timeout 3600s;
            proxy_buffering off;
            gzip off;                           # critical: no gzip on WS
        }

        # ── Internal endpoints (block from public) ──────────────────
        location /v1/internal/ {
            return 403;
        }

        # ── Metrics endpoint (block from public; access via private VPN) ──
        location /v1/metrics {
            return 403;
        }

        # ── All other API ────────────────────────────────────────────
        location /v1/ {
            limit_req zone=api burst=60 nodelay;
            proxy_pass http://obscura_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto https;
            proxy_connect_timeout 5s;
            proxy_send_timeout 30s;
            proxy_read_timeout 30s;
            proxy_next_upstream error timeout http_502 http_503 http_504;
            proxy_next_upstream_tries 3;
        }

        # ── Health (open for ELB / monitoring) ──────────────────────
        location = /health {
            access_log off;
            return 200 'ok';
            add_header Content-Type text/plain;
        }
    }
}
```

## SSL setup (Let's Encrypt)

```bash
# Initial cert
docker run --rm -v /etc/letsencrypt:/etc/letsencrypt \
    -v /var/www/certbot:/var/www/certbot \
    -p 80:80 certbot/certbot certonly --standalone \
    -d api.obscura.network --email ops@obscura.network --agree-tos --no-eff-email

# Auto renewal cron
0 0,12 * * * docker run --rm -v /etc/letsencrypt:/etc/letsencrypt certbot/certbot renew --quiet && docker exec nginx nginx -s reload
```

## Operations

```bash
# Test config without reload
docker exec nginx nginx -t

# Hot reload
docker exec nginx nginx -s reload

# Check upstream health
curl http://localhost/health

# Tail access log
docker logs nginx -f
```

## Rules

- WS endpoints: NO gzip, long timeouts, `Upgrade` headers
- Internal/metrics endpoints: blocked from public (`/v1/internal/`, `/v1/metrics`)
- TLS: 1.3 only, HSTS preload, OCSP stapling
- Rate limits: aggressive on auth (5r/s), normal on api (30r/s)
- `client_max_body_size`: matches max tier upload (100MB)
- `least_conn` over `round_robin` for unequal request handling
- `keepalive 32` in upstream for connection reuse
