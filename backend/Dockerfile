# ─── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# modernc.org/sqlite is pure Go — no CGO, no sqlite-dev needed
RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o obscura-node ./cmd/node/

# ─── Runtime Stage ────────────────────────────────────────────────────────────
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata wget su-exec

WORKDIR /app

COPY --from=builder /app/obscura-node .
COPY docker-entrypoint.sh /docker-entrypoint.sh

RUN mkdir -p /app/data && \
    addgroup -S obscura && \
    adduser -S -G obscura obscura && \
    chown -R obscura:obscura /app && \
    chmod +x /docker-entrypoint.sh

# NOT: USER obscura BİLEREK kaldırıldı — konteyner root olarak başlıyor ki
# entrypoint, Railway'in runtime'da mount ettiği volume'u (build-time chown
# ona uygulanmadığı için) yeniden chown'layıp SONRA obscura'ya düşebilsin
# (bkz. docker-entrypoint.sh). Volume yoksa (DATA_DIR boş/./data) entrypoint
# hiçbir şey yapmadan direkt privilege-drop eder — davranış değişmez.

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:${PORT:-8080}/v1/node/status || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["./obscura-node"]
