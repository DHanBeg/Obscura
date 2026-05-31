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
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /app/obscura-node .

RUN mkdir -p /app/data && \
    addgroup -S obscura && \
    adduser -S -G obscura obscura && \
    chown -R obscura:obscura /app

USER obscura

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:${PORT:-8080}/v1/node/status || exit 1

CMD ["./obscura-node"]
