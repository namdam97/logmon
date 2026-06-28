# LogMon — root orchestration cho local dev + test.
# `make help` để xem tất cả target. Stack nặng (ES/Grafana/Prom/OTel) nằm sau
# profile `observability`; demo workload sau profile `demo`.

COMPOSE           ?= docker compose -f $(CURDIR)/infra/docker/docker-compose.yml
# Production overlay (ADR-040): base + prod + .env.prod (gitignored). Network
# segmentation + nginx/TLS + frontend. Xem infra/docker/.env.prod.example.
COMPOSE_PROD      ?= docker compose -f $(CURDIR)/infra/docker/docker-compose.yml -f $(CURDIR)/infra/docker/docker-compose.prod.yml --env-file $(CURDIR)/infra/docker/.env.prod
# Mode B overlay (profile scale, ADR-027/011): Kafka buffer + Thanos + SeaweedFS
# + ILM snapshot. Xem infra/docker/docker-compose.scale.yml.
COMPOSE_SCALE     ?= docker compose -f $(CURDIR)/infra/docker/docker-compose.yml -f $(CURDIR)/infra/docker/docker-compose.scale.yml
POSTGRES_PASSWORD ?= logmon
# URL trong network compose (service host = postgres) — cho migrate container.
MIGRATE_DB        := postgres://logmon:$(POSTGRES_PASSWORD)@postgres:5432/logmon?sslmode=disable
# URL từ host (port-forward 5432) — cho seeder/go test chạy trên máy.
DB_URL_HOST       := postgres://logmon:$(POSTGRES_PASSWORD)@localhost:5432/logmon?sslmode=disable

.DEFAULT_GOAL := help
.PHONY: help doctor hooks up up-full up-demo down down-v logs ps db \
        migrate migrate-down seed dev dev-be dev-fe \
        test test-be test-fe test-integration e2e ci-local fmt lint vuln build clean \
        tls-cert prod-up prod-down prod-logs prod-verify prod-backup \
        scale-up scale-down scale-logs

help: ## Hiển thị danh sách target
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-13s\033[0m %s\n", $$1, $$2}'

# ── Toolchain ───────────────────────────────────────────────────────────────
doctor: ## Kiểm tra toolchain local (docker/compose/go/pnpm/chrome)
	@echo "== toolchain =="
	@docker --version 2>/dev/null || echo "MISSING: docker"
	@docker compose version 2>/dev/null || echo "MISSING: docker compose (cần v2.22+)"
	@go version 2>/dev/null || echo "MISSING: go"
	@pnpm --version >/dev/null 2>&1 && echo "pnpm $$(pnpm --version)" || echo "MISSING: pnpm"
	@golangci-lint --version 2>/dev/null | head -1 || echo "WARN: golangci-lint (cần cho make lint)"
	@google-chrome --version 2>/dev/null || echo "WARN: google-chrome (Playwright E2E dùng channel chrome)"

hooks: ## Cài git hooks versioned (.githooks qua core.hooksPath)
	git config core.hooksPath .githooks
	@echo "✓ git hooks đã cài (.githooks). pre-commit: gofmt+gitleaks (staged);"
	@echo "  pre-push: lint+test+govulncheck. Bỏ qua tạm: git commit/push --no-verify."

# ── Stack ───────────────────────────────────────────────────────────────────
up: ## Dựng stack nhẹ: postgres + migrate + userservice
	$(COMPOSE) up -d --build

up-full: ## Dựng full: thêm observability (ES/Grafana/Prometheus/Alertmanager/OTel/Jaeger)
	OTEL_EXPORTER_OTLP_ENDPOINT=otel-agent:4317 ELASTICSEARCH_URL=http://elasticsearch:9200 $(COMPOSE) --profile observability up -d --build

up-demo: ## Dựng full + demo workload (demo-order + loadgen)
	OTEL_EXPORTER_OTLP_ENDPOINT=otel-agent:4317 ELASTICSEARCH_URL=http://elasticsearch:9200 $(COMPOSE) --profile observability --profile demo up -d --build

down: ## Dừng + xoá container (giữ volume/data)
	$(COMPOSE) --profile observability --profile demo down

down-v: ## Dừng + xoá cả volume (reset sạch DB/ES/Grafana)
	$(COMPOSE) --profile observability --profile demo down -v

# ── Production-like (local hoặc VPS) — Mode A: nginx/TLS + network segmentation ─
tls-cert: ## Sinh self-signed cert cho HTTPS local (prod dùng Let's Encrypt)
	./infra/nginx/gen-self-signed.sh $(CN)

prod-up: tls-cert ## Dựng stack production-like (base+prod overlay, cần infra/docker/.env.prod)
	@test -f infra/docker/.env.prod || { echo "THIẾU infra/docker/.env.prod — copy từ .env.prod.example"; exit 1; }
	$(COMPOSE_PROD) --profile observability up -d --build

prod-down: ## Dừng stack production-like (giữ volume)
	$(COMPOSE_PROD) --profile observability down

prod-logs: ## Tail logs prod (make prod-logs ; hoặc S=nginx)
	$(COMPOSE_PROD) logs -f --tail=100 $(S)

prod-verify: ## Smoke post-deploy (make prod-verify ; hoặc URL=https://domain)
	./infra/scripts/verify.sh $(URL)

prod-backup: ## Backup Postgres (pg_dump -Fc → ./backups)
	./infra/scripts/backup.sh

# ── Mode B (production-scale tí hon): Kafka buffer + Thanos + SeaweedFS + ILM ──
scale-up: ## Dựng stack Mode B (base + scale overlay + observability)
	OTEL_EXPORTER_OTLP_ENDPOINT=otel-agent:4317 ELASTICSEARCH_URL=http://elasticsearch:9200 \
	  $(COMPOSE_SCALE) --profile observability --profile scale up -d --build

scale-down: ## Dừng stack Mode B (giữ volume)
	$(COMPOSE_SCALE) --profile observability --profile scale down

scale-logs: ## Tail logs Mode B (make scale-logs ; hoặc S=kafka)
	$(COMPOSE_SCALE) --profile observability --profile scale logs -f --tail=100 $(S)

logs: ## Tail logs (make logs ; hoặc make logs S=userservice)
	$(COMPOSE) logs -f --tail=100 $(S)

ps: ## Liệt kê container đang chạy
	$(COMPOSE) ps

db: ## Chỉ dựng postgres + áp migrations (cho seed/dev nhanh)
	$(COMPOSE) up -d postgres
	$(COMPOSE) run --rm migrate

# ── Migrations & seed ─────────────────────────────────────────────────────────
migrate: ## Áp tất cả migrations (golang-migrate up)
	$(COMPOSE) run --rm migrate

migrate-down: ## Rollback 1 migration gần nhất
	$(COMPOSE) run --rm migrate -path /migrations -database "$(MIGRATE_DB)" down 1

seed: db ## Nạp dữ liệu demo (idempotent) qua cmd/seeder
	cd backend && DATABASE_URL="$(DB_URL_HOST)" go run ./cmd/seeder

# ── Dev (hot reload) ──────────────────────────────────────────────────────────
dev: db ## Dựng DB + hướng dẫn chạy hot-reload BE/FE
	@echo "DB sẵn sàng. Mở 2 terminal:"
	@echo "  make dev-be   # Go API hot reload (air)"
	@echo "  make dev-fe   # Next.js dev server (:3000)"

dev-be: ## Go API hot reload bằng air (không cần cài global)
	cd backend && go run github.com/air-verse/air@latest

dev-fe: ## Next.js dev server
	cd frontend && pnpm install && pnpm dev

# ── Tests ─────────────────────────────────────────────────────────────────────
test: test-be test-fe ## Chạy unit test BE + FE (nhanh, không Docker)

test-be: ## Go test có race + coverage
	cd backend && go test -race -cover ./...

test-fe: ## Frontend unit test (vitest)
	cd frontend && pnpm install --frozen-lockfile && pnpm test

test-integration: db ## Integration test BE (cần Postgres) — go test -tags integration
	# -p 1: chạy tuần tự từng package (các integration test chia sẻ cùng DB).
	cd backend && DATABASE_URL="$(DB_URL_HOST)" go test -tags integration -race -p 1 ./...

e2e: ## Full-stack E2E: tự dựng BE+pg, chạy Playwright, teardown
	AUTH_RATE_PER_MINUTE=100000 AUTH_RATE_BURST=100000 \
	  $(COMPOSE) up -d --build postgres migrate userservice
	@set -e; trap '$(COMPOSE) down' EXIT; \
	  echo "waiting userservice healthz..."; \
	  for i in $$(seq 1 30); do curl -sf http://localhost:8080/healthz >/dev/null 2>&1 && break; sleep 2; done; \
	  cd frontend && pnpm install --frozen-lockfile && pnpm exec next build && pnpm exec playwright test

ci-local: ## Mô phỏng CI: lint + test BE/FE + e2e
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) e2e

# ── Quality ───────────────────────────────────────────────────────────────────
fmt: ## gofmt backend
	cd backend && gofmt -w ./cmd ./internal

lint: ## golangci-lint backend
	cd backend && golangci-lint run

vuln: ## govulncheck backend (allowlist dùng chung CI + git hook)
	./scripts/govulncheck.sh

build: ## Build binary BE + image userservice
	cd backend && go build ./...
	$(COMPOSE) build userservice

clean: ## Xoá artifact build (bin, tmp)
	cd backend && rm -rf bin tmp && go clean
