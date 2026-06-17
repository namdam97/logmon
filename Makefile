# LogMon — root orchestration cho local dev + test.
# `make help` để xem tất cả target. Stack nặng (ES/Grafana/Prom/OTel) nằm sau
# profile `observability`; demo workload sau profile `demo`.

COMPOSE           ?= docker compose -f $(CURDIR)/infra/docker/docker-compose.yml
POSTGRES_PASSWORD ?= logmon
# URL trong network compose (service host = postgres) — cho migrate container.
MIGRATE_DB        := postgres://logmon:$(POSTGRES_PASSWORD)@postgres:5432/logmon?sslmode=disable
# URL từ host (port-forward 5432) — cho seeder/go test chạy trên máy.
DB_URL_HOST       := postgres://logmon:$(POSTGRES_PASSWORD)@localhost:5432/logmon?sslmode=disable

.DEFAULT_GOAL := help
.PHONY: help doctor up up-full up-demo down down-v logs ps db \
        migrate migrate-down seed dev dev-be dev-fe \
        test test-be test-fe test-integration e2e ci-local fmt lint build clean

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

# ── Stack ───────────────────────────────────────────────────────────────────
up: ## Dựng stack nhẹ: postgres + migrate + userservice
	$(COMPOSE) up -d --build

up-full: ## Dựng full: thêm observability (ES/Grafana/Prometheus/Alertmanager/OTel)
	$(COMPOSE) --profile observability up -d --build

up-demo: ## Dựng full + demo workload (demo-order + loadgen)
	$(COMPOSE) --profile observability --profile demo up -d --build

down: ## Dừng + xoá container (giữ volume/data)
	$(COMPOSE) --profile observability --profile demo down

down-v: ## Dừng + xoá cả volume (reset sạch DB/ES/Grafana)
	$(COMPOSE) --profile observability --profile demo down -v

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

build: ## Build binary BE + image userservice
	cd backend && go build ./...
	$(COMPOSE) build userservice

clean: ## Xoá artifact build (bin, tmp)
	cd backend && rm -rf bin tmp && go clean
