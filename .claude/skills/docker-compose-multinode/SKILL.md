---
name: docker-compose-multinode
description: Obscura's 5-node Docker Compose setup with nginx LB, MinIO, coturn, Prometheus stack. Use when modifying docker-compose.yml or adding services.
---

# Docker Compose 5-Node Setup

## Architecture

```
       ┌─────────────────────────┐
       │   nginx (LB + TLS)      │
       │   :80, :443             │
       └────┬────┬────┬────┬─────┘
            │    │    │    │
       ┌────▼┐  ┌▼┐  ┌▼┐  ┌▼┐
       │node1│ │n2│ │n3│ │n4│ │n5
       └──┬──┘ └─┘ └─┘ └─┘ └─┘
          │ HTTP gossip relay
          ▼
   ┌──────────────┐
   │  MinIO       │ media storage
   │  coturn      │ TURN server
   │  Prometheus  │ metrics
   │  Grafana     │ dashboards
   │  Alertmanager│ alerts
   │  Loki        │ logs
   └──────────────┘
```

## docker-compose.yml structure

```yaml
version: "3.9"

networks:
  frontend:
    driver: bridge
  backend:
    driver: bridge
  internal:
    driver: bridge
    internal: true   # no external access

volumes:
  minio_data:
  prometheus_data:
  grafana_data:
  node1_data:
  node2_data:
  node3_data:
  node4_data:
  node5_data:

services:
  nginx:
    image: nginx:1.27-alpine
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
    depends_on:
      node1: { condition: service_healthy }
      node2: { condition: service_healthy }
      node3: { condition: service_healthy }
      node4: { condition: service_healthy }
      node5: { condition: service_healthy }
    networks: [frontend, backend]

  node1: &node-base
    build: ./backend
    restart: unless-stopped
    environment:
      PORT: "8080"
      NODE_ID: "node-1"
      NODE_PEERS: "node2:8080,node3:8080,node4:8080,node5:8080"
      JWT_SECRET: ${JWT_SECRET}
      INTERNAL_SECRET: ${INTERNAL_SECRET}
      MINIO_ENDPOINT: "minio:9000"
      MINIO_ACCESS_KEY: ${MINIO_ACCESS_KEY}
      MINIO_SECRET_KEY: ${MINIO_SECRET_KEY}
      MINIO_BUCKET: "obscura-media"
      TURN_SECRET: ${TURN_SECRET}
      TURN_HOST: "turn.obscura.network"
    volumes:
      - node1_data:/app/data
    networks: [backend, internal]
    depends_on:
      minio: { condition: service_healthy }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/v1/node/status"]
      interval: 10s
      timeout: 3s
      retries: 5
      start_period: 10s

  node2:
    <<: *node-base
    environment:
      PORT: "8080"
      NODE_ID: "node-2"
      NODE_PEERS: "node1:8080,node3:8080,node4:8080,node5:8080"
      # ... rest same
    volumes: [node2_data:/app/data]

  # node3, node4, node5 similar with rotating peer lists

  minio:
    image: minio/minio:latest
    restart: unless-stopped
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ACCESS_KEY}
      MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY}
    volumes: [minio_data:/data]
    ports: ["9001:9001"]   # console only; data via internal
    networks: [internal]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s

  coturn:
    image: coturn/coturn:latest
    restart: unless-stopped
    network_mode: host  # required for TURN port range
    volumes:
      - ./coturn/turnserver.conf:/etc/turnserver.conf:ro
      - ./coturn/ssl:/etc/coturn/ssl:ro

  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./prometheus/alerts.yml:/etc/prometheus/alerts.yml:ro
      - prometheus_data:/prometheus
    ports: ["9090:9090"]
    networks: [backend]

  grafana:
    image: grafana/grafana:latest
    restart: unless-stopped
    environment:
      GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_PASSWORD}
    volumes: [grafana_data:/var/lib/grafana]
    ports: ["3000:3000"]
    networks: [backend]
    depends_on: [prometheus]

  alertmanager:
    image: prom/alertmanager:latest
    restart: unless-stopped
    volumes: [./prometheus/alertmanager.yml:/etc/alertmanager/config.yml:ro]
    ports: ["9093:9093"]
    networks: [backend]
```

## .env file (gitignored)

```bash
JWT_SECRET=$(openssl rand -hex 32)
INTERNAL_SECRET=$(openssl rand -hex 32)
MINIO_ACCESS_KEY=obscura
MINIO_SECRET_KEY=$(openssl rand -hex 24)
TURN_SECRET=$(openssl rand -hex 32)
GRAFANA_PASSWORD=$(openssl rand -hex 16)
```

## Operations

```bash
# Generate .env from .env.example
cp .env.example .env && nano .env

# Start
docker compose up -d

# Tail one node
docker compose logs -f node1

# Restart one node
docker compose restart node1

# Scale (caution — change peers env if scaling)
docker compose up -d --scale node1=2

# Stop
docker compose down

# Wipe everything (destructive)
docker compose down -v
```

## Health verification

```bash
# All nodes responding
for i in 1 2 3 4 5; do
  curl -s http://localhost/v1/node/status | jq .
done

# nginx routing across nodes
for i in {1..10}; do
  curl -s http://localhost/v1/node/status | jq -r .data.node_id
done | sort | uniq -c
# Should show distribution across all 5 nodes
```

## Rules

- All sensitive env in `.env` (never committed)
- All services have `restart: unless-stopped`
- All services have healthchecks
- Volume names explicit (not anonymous)
- Networks segmented: frontend (public), backend (internal LB), internal (DB only)
- Image pinned to digest for prod (`@sha256:...`)
- HEALTHCHECK in every Dockerfile + condition: service_healthy in compose
