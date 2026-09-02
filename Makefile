# ─── Obscura Network — Geliştirme Araçları ───────────────────────────────────

.PHONY: help dev dev-web dev-mobile dev-backend dev-desktop \
        build build-backend build-frontend build-desktop \
        docker-up docker-down docker-logs deploy-prod \
        test test-backend lint \
        circuits-build clean

# Varsayılan hedef
help:
	@echo ""
	@echo "  ╔═══════════════════════════════════════╗"
	@echo "  ║  🦅  Obscura Network — Makefile        ║"
	@echo "  ╚═══════════════════════════════════════╝"
	@echo ""
	@echo "  Geliştirme:"
	@echo "    make dev           Tüm servisleri başlat (Docker)"
	@echo "    make dev-backend   Sadece backend (hot reload)"
	@echo "    make dev-web       Sadece Next.js frontend"
	@echo "    make dev-mobile    Expo mobil uygulama"
	@echo "    make dev-desktop   Tauri masaüstü"
	@echo ""
	@echo "  Build:"
	@echo "    make build         Tümünü derle"
	@echo "    make build-backend Backend binary"
	@echo "    make build-frontend Next.js production"
	@echo "    make build-desktop Tauri installer"
	@echo ""
	@echo "  Docker:"
	@echo "    make docker-up     Docker Compose başlat"
	@echo "    make docker-down   Docker Compose durdur"
	@echo "    make docker-logs   Log akışı"
	@echo ""
	@echo "  Test & Kalite:"
	@echo "    make test          Tüm testler"
	@echo "    make test-backend  Backend Go testleri"
	@echo "    make lint          Kod kalite kontrolü"
	@echo ""
	@echo "  ZK Devreleri:"
	@echo "    make circuits-build  Circom devrelerini derle"
	@echo ""

# ─── Geliştirme ───────────────────────────────────────────────────────────────

dev: docker-up
	@echo "✅ Tüm servisler başlatıldı"
	@echo "   Web   → http://localhost:3000"
	@echo "   API   → http://localhost:8080"
	@echo "   MinIO → http://localhost:9001"

dev-backend:
	@echo "🚀 Backend başlatılıyor (hot reload)..."
	cd backend && \
		$(shell which air || echo "go run") ./cmd/node/

dev-web:
	@echo "🌐 Next.js başlatılıyor..."
	cd frontend && npm run dev

dev-mobile:
	@echo "📱 Expo başlatılıyor..."
	cd mobile && npm start

dev-desktop:
	@echo "🖥️  Tauri başlatılıyor..."
	cd desktop && npm run dev

# ─── Build ────────────────────────────────────────────────────────────────────

build: build-backend build-frontend
	@echo "✅ Build tamamlandı"

build-backend:
	@echo "🔨 Backend derleniyor..."
	cd backend && CGO_ENABLED=0 go build -ldflags="-w -s" -o ./dist/obscura-node ./cmd/node/
	@echo "✅ Backend: backend/dist/obscura-node"

build-frontend:
	@echo "🔨 Frontend derleniyor..."
	cd frontend && npm run build
	@echo "✅ Frontend: frontend/.next"

build-desktop:
	@echo "🔨 Tauri installer derleniyor..."
	cd desktop && npm run build
	@echo "✅ Installer: desktop/src-tauri/target/release/bundle/"

# ─── Docker ───────────────────────────────────────────────────────────────────

docker-up:
	@echo "🐳 Docker Compose başlatılıyor..."
	docker compose up -d --build

docker-down:
	@echo "🛑 Docker Compose durduruluyor..."
	docker compose down

docker-logs:
	docker compose logs -f --tail=50

docker-clean:
	docker compose down -v
	docker system prune -f

# ─── Deploy (production) ───────────────────────────────────────────────────────
# docker-up (yukarıda) yereldir, .env placeholder'la meşru çalışır — DOKUNULMADI.
# deploy-prod go-live komutu: placeholder sağ kalırsa build/up'a hiç geçmez.

GENERATE ?= ./scripts/generate-secrets.sh
COMPOSE  ?= docker compose

deploy-prod:
	@echo "🔐 Secret'lar üretiliyor (placeholder varsa, idempotent)..."
	$(GENERATE) .env
	@echo "🛡️  Pre-deploy secret kontrolü (placeholder kalırsa DURUR)..."
	./scripts/check-secrets.sh .
	@echo "🔨 Build..."
	$(COMPOSE) build
	@echo "🚀 Up..."
	$(COMPOSE) up -d

# ─── Test ─────────────────────────────────────────────────────────────────────

test: test-backend
	@echo "✅ Tüm testler tamamlandı"

test-backend:
	@echo "🧪 Backend testleri çalıştırılıyor..."
	# CI ile aynı opt-in (bkz. .github/workflows/ci.yml) — secrets.Require
	# (internal/secrets) OBSCURA_ENV açıkça dev değilse prod sayar ve eksik
	# secret'ta FATAL olur (C10). Lokal test runner'ın gerçek secret'ı yok,
	# bu yüzden CI'daki gibi dev opt-in gerekli; prod-fatal davranışı bu
	# değişkenle BOZULMAZ — sadece açıkça set ediliyor.
	# 30s sabit timeout kaldırıldı: boot-fatal altında testler <1s'de
	# ölüyordu, bu yüzden bu süre hiç gerçek bir test çalışmasıyla
	# doğrulanmamıştı. db (~24s) / mls (~75s) meşru yavaş test süreleri,
	# takılma değil (bkz. commit'in kanıt bölümü). CI (ci.yml) de hiç
	# -timeout kullanmıyor, Go varsayılanı (600s) geçerli — burada da öyle.
	cd backend && OBSCURA_ENV=development go test ./... -v

# ─── Lint ─────────────────────────────────────────────────────────────────────

lint:
	@echo "🔍 Kod kalite kontrolü..."
	cd backend && go vet ./...
	cd frontend && npm run lint 2>/dev/null || true
	@echo "✅ Lint tamamlandı"

# ─── ZK Devreleri ─────────────────────────────────────────────────────────────

circuits-build:
	@echo "🔐 ZK devreleri derleniyor..."
	chmod +x circuits/build.sh
	cd circuits && ./build.sh
	@echo "✅ ZK devreleri hazır: circuits/build/"

# ─── Temizlik ─────────────────────────────────────────────────────────────────

clean:
	rm -f backend/dist/obscura-node
	rm -rf frontend/.next
	rm -rf circuits/build
	find . -name "node_modules" -prune -o -name "*.log" -print -exec rm {} \;
	@echo "✅ Temizlendi"

# ─── Kurulum ──────────────────────────────────────────────────────────────────

setup:
	@echo "📦 Bağımlılıklar kuruluyor..."
	cp -n .env.example .env || true
	cd frontend && npm install
	cd mobile && npm install
	cd desktop && npm install
	cd backend && go mod download
	@echo "✅ Kurulum tamamlandı — .env dosyasını düzenleyin"
