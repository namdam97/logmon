# Claude Code Project Structure — Hướng Dẫn Áp Dụng

> **Dự án:** `wild-workouts-go-ddd-example` — Go DDD Microservices
> **Ngày cập nhật:** 2026-03-17
> **Tham khảo:** [paperclipai/paperclip](https://github.com/paperclipai/paperclip), Claude Code Best Practices

---

## Tổng Quan Nhận Định

### Tại sao cấu trúc này quan trọng?

Nhận định về Claude Code Project Structure giải quyết 3 vấn đề cốt lõi khi làm việc với AI-assisted development:

| Vấn đề | Triệu chứng | Giải pháp |
|--------|-------------|-----------|
| **Context drift** | AI không nhớ architecture, lặp lại cùng mistake | CLAUDE.md per-module |
| **Workflow không tái dụng** | Mỗi lần review/refactor phải prompt lại từ đầu | `.claude/skills/` |
| **Không có guardrails** | AI có thể modify sai layer, vi phạm DDD boundaries | `.claude/hooks/` |
| **Chaos khi scale** | 10+ agent tabs, không biết ai làm gì | Paperclip orchestration |

### Đánh giá tính phù hợp với project này

Project `wild-workouts-go-ddd-example` là **ideal candidate** vì:
- ✅ 3 microservices độc lập (`trainer`, `trainings`, `users`) → mỗi service cần context riêng
- ✅ Kiến trúc DDD nghiêm ngặt → cần hooks bảo vệ layer boundaries
- ✅ Pattern lặp lại (CQRS, adapters, ports) → skills tái sử dụng có giá trị cao
- ✅ Multi-module Go workspace → cần CLAUDE.md per-module rõ ràng

---

## Cấu Trúc Đề Xuất

```
wild-workouts-go-ddd-example/
├── CLAUDE.md                          # Root context: tổng quan project, DDD rules
├── docs/
│   ├── architecture.md               # Kiến trúc tổng thể (đã có)
│   ├── decisions/                    # Architecture Decision Records (ADRs)
│   │   ├── 001-ddd-boundaries.md
│   │   ├── 002-grpc-vs-http.md
│   │   └── 003-firestore-choice.md
│   └── runbooks/                     # Quy trình vận hành
│       ├── local-dev.md
│       ├── deploy.md
│       └── troubleshooting.md
├── .claude/
│   ├── settings.json                 # Permissions, hooks config
│   ├── hooks/
│   │   ├── pre-edit-check.sh         # Kiểm tra DDD layer violations
│   │   ├── post-edit-lint.sh         # Auto lint sau khi edit
│   │   └── security-check.sh        # Chặn hardcode secrets
│   └── skills/
│       ├── code-review/
│       │   └── SKILL.md
│       ├── add-feature/
│       │   └── SKILL.md
│       ├── refactor/
│       │   └── SKILL.md
│       └── release/
│           └── SKILL.md
├── internal/
│   ├── common/
│   │   └── CLAUDE.md                 # Context: shared utilities, auth, errors
│   ├── trainer/
│   │   └── CLAUDE.md                 # Context: trainer domain, schedule logic
│   ├── trainings/
│   │   └── CLAUDE.md                 # Context: training CQRS, Firestore adapters
│   └── users/
│       └── CLAUDE.md                 # Context: user auth, Firebase integration
└── web/
    └── CLAUDE.md                     # Context: Vue.js frontend, API clients
```

---

## Bước 1: Tạo Root CLAUDE.md

**File:** `/CLAUDE.md`

```markdown
# Wild Workouts — AI Context

## Project Overview
Go DDD microservices example. 3 services: trainer, trainings, users.
Frontend: Vue.js 2. Backend: Go 1.25 + gRPC + Firebase Firestore.

## Architecture Rules (MUST FOLLOW)
- **DDD Layer Order (strict):** domain → app → ports → adapters
- **No import upward:** adapters CANNOT import from domain directly (use ports)
- **No cross-service imports:** services communicate ONLY via gRPC or HTTP
- `internal/common` is the ONLY shared package

## Go Module Structure
- `go.work` workspace with 5 modules
- Each service: `internal/{service}/go.mod`
- Shared: `internal/common/go.mod`

## DDD Layers per Service
| Layer | Package | Responsibility |
|-------|---------|----------------|
| domain/ | business logic | entities, value objects, domain errors |
| app/ | use cases | commands, queries (CQRS) |
| ports/ | contracts | interfaces for adapters |
| adapters/ | implementations | HTTP, gRPC, Firestore, MySQL |
| service/ | wiring | dependency injection, server setup |

## Key Commands
- `make openapi` — regenerate OpenAPI clients
- `make proto` — compile protobuf
- `make lint` — run go-cleanarch + goimports
- `docker-compose up` — local dev environment
- `make test` — run all tests

## Style Guide
- Error handling: always wrap with context (`fmt.Errorf("doing X: %w", err)`)
- Logging: use `logrus` with structured fields
- Tests: table-driven, use `testify/require` not `assert`
- No global state, use dependency injection

## What NOT to do
- Do NOT add business logic in adapters layer
- Do NOT use `interface{}` — use typed structs
- Do NOT commit `.env` files
- Do NOT skip error handling
```

---

## Bước 2: Tạo CLAUDE.md per Service

### `internal/trainer/CLAUDE.md`

```markdown
# Trainer Service — AI Context

## Domain Purpose
Quản lý lịch trống (availability) của trainer. Trainer set giờ available,
trainings service book các giờ đó.

## Key Domain Concepts
- `TrainingHour`: value object đại diện 1 khung giờ (date + hour)
- `Availability`: aggregate root quản lý tập hợp TrainingHours
- Business rule: trainer chỉ có thể set giờ trong tương lai

## CQRS Split
- **Commands:** `ScheduleTraining`, `CancelTraining`, `MakeHoursAvailable`
- **Queries:** `HourAvailability`

## External Dependencies
- Firestore: lưu training hours
- MySQL: optional alternative storage
- gRPC server: port 3010 (nhận requests từ trainings service)
- HTTP server: port 3000 (nhận requests từ web frontend)

## Important Files
- `domain/training_hour.go` — core domain logic
- `adapters/trainings_firestore_repo.go` — Firestore implementation
- `ports/trainer_repository.go` — interface định nghĩa

## Test Pattern
Integration tests trong `adapters/` dùng Firestore emulator (port 8787).
Unit tests trong `domain/` không cần external dependencies.
```

### `internal/trainings/CLAUDE.md`

```markdown
# Trainings Service — AI Context

## Domain Purpose
Quản lý việc đặt lịch training. User (attendee) tạo training,
trainer accept/reject, trainings service giao tiếp với trainer service qua gRPC.

## Key Domain Concepts
- `Training`: aggregate root (có ID, user, trainer, time, status)
- `User`: value object (attendee info)
- Status flow: proposed → approved/rejected → cancelled/completed

## Cross-Service Communication
- Calls `trainer` service via gRPC để check availability & update schedule
- gRPC client: `adapters/trainer_grpc.go`

## CQRS Split
- **Commands:** `ScheduleTraining`, `CancelTraining`, `ApproveTrainingReschedule`
- **Queries:** `AllTrainings`

## HTTP Port
- Port 3001, OpenAPI spec: `api/openapi/trainings.yml`

## Important Files
- `domain/training.go` — Training aggregate
- `adapters/trainings_firestore_repo.go` — persistence
- `adapters/trainer_grpc.go` — gRPC client to trainer service
```

### `internal/users/CLAUDE.md`

```markdown
# Users Service — AI Context

## Domain Purpose
User management và authentication với Firebase Auth.
Phân biệt 2 roles: `attendee` (đặt training) và `trainer` (cung cấp training).

## Authentication Flow
1. Frontend đăng nhập qua Firebase Auth → nhận JWT token
2. JWT được gửi kèm mọi HTTP request
3. Middleware `internal/common/auth/` verify JWT

## Key Files
- `domain/user.go` — User entity + role logic
- `adapters/user_firestore_repo.go` — persistence
- `internal/common/auth/` — JWT middleware (shared)

## gRPC Server
- Port 3020
- Proto: `api/protobuf/users.proto`
- Được trainer service dùng để lấy user info

## Important Business Rules
- User chỉ có 1 role tại một thời điểm (attendee XOR trainer)
- Role được set khi đăng ký, không thay đổi được
```

### `internal/common/CLAUDE.md`

```markdown
# Common Package — AI Context

## Purpose
Shared utilities dùng bởi tất cả services. KHÔNG chứa business logic.

## Packages
| Package | Mô tả |
|---------|-------|
| auth/ | Firebase JWT verification middleware |
| client/ | HTTP client với retry logic |
| decorator/ | Command/Query decorators (logging, metrics, auth) |
| errors/ | Typed application errors |
| logs/ | Logrus setup + request logging |
| metrics/ | Prometheus metrics helpers |
| server/ | HTTP server boilerplate |
| tests/ | Test helpers, Firestore test setup |

## Golden Rule
Common package chỉ được chứa infrastructure concerns.
Nếu muốn thêm code mới vào common, hỏi: "Cái này có specific cho 1 service không?"
Nếu có → đặt vào service đó. Nếu không → mới đặt vào common.
```

---

## Bước 3: Tạo `.claude/settings.json`

**File:** `.claude/settings.json`

```json
{
  "permissions": {
    "allow": [
      "Bash(make:*)",
      "Bash(go:*)",
      "Bash(docker-compose:*)",
      "Bash(git diff:*)",
      "Bash(git log:*)",
      "Bash(git status:*)"
    ],
    "deny": [
      "Bash(rm -rf:*)",
      "Bash(git push --force:*)",
      "Bash(git reset --hard:*)"
    ]
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/pre-edit-check.sh"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/post-edit-lint.sh"
          }
        ]
      }
    ]
  }
}
```

---

## Bước 4: Tạo Hooks

### `.claude/hooks/pre-edit-check.sh`

Hook này kiểm tra DDD layer violations trước khi AI edit file.

```bash
#!/bin/bash
# Pre-edit hook: kiểm tra DDD boundary violations
# Input (env): CLAUDE_TOOL_INPUT_FILE_PATH

FILE_PATH="${CLAUDE_TOOL_INPUT_FILE_PATH:-}"

if [[ -z "$FILE_PATH" ]]; then
  exit 0
fi

# Cảnh báo nếu AI cố gắng thêm Firestore import vào domain layer
if [[ "$FILE_PATH" =~ internal/.*/domain/.*\.go ]]; then
  # Kiểm tra nội dung sẽ được viết (từ stdin)
  CONTENT=$(cat)
  if echo "$CONTENT" | grep -q "cloud.google.com/go/firestore"; then
    echo "ERROR: Domain layer CANNOT import Firestore directly." >&2
    echo "Use the repository interface in ports/ instead." >&2
    echo "$CONTENT"  # pass through để không block hoàn toàn
    exit 0
  fi
  echo "$CONTENT"
  exit 0
fi

# Pass through nếu không có violation
cat
exit 0
```

### `.claude/hooks/post-edit-lint.sh`

```bash
#!/bin/bash
# Post-edit hook: auto format và lint sau khi edit Go files

FILE_PATH="${CLAUDE_TOOL_RESULT_FILE_PATH:-}"

if [[ -z "$FILE_PATH" ]]; then
  exit 0
fi

# Chỉ lint Go files
if [[ "$FILE_PATH" =~ \.go$ ]]; then
  # Tìm module directory chứa file này
  MODULE_DIR=$(dirname "$FILE_PATH")
  while [[ "$MODULE_DIR" != "/" ]]; do
    if [[ -f "$MODULE_DIR/go.mod" ]]; then
      break
    fi
    MODULE_DIR=$(dirname "$MODULE_DIR")
  done

  if [[ -f "$MODULE_DIR/go.mod" ]]; then
    # Format imports
    cd "$MODULE_DIR" && goimports -w "$FILE_PATH" 2>/dev/null || true
    # Vet
    go vet ./... 2>/dev/null | head -20 || true
  fi
fi

exit 0
```

---

## Bước 5: Tạo Skills

### `.claude/skills/code-review/SKILL.md`

```markdown
---
name: code-review
description: >
  Review Go code changes theo DDD principles và Go best practices.
  Kiểm tra layer boundaries, error handling, test coverage, naming conventions.
---

# Code Review Skill — Wild Workouts

## Checklist Tự Động

Khi review code trong project này, kiểm tra theo thứ tự:

### 1. DDD Layer Boundaries
- [ ] domain/ không import từ adapters/ hoặc external packages (Firestore, gRPC)
- [ ] adapters/ implement interfaces từ ports/ (không tự tạo interface)
- [ ] app/ (commands/queries) chỉ dùng interfaces từ ports/
- [ ] Không có business logic trong adapters/

### 2. Error Handling
- [ ] Errors được wrap với context: `fmt.Errorf("context: %w", err)`
- [ ] Không có `_ = err` hoặc error bị bỏ qua
- [ ] Domain errors dùng types từ `internal/common/errors/`
- [ ] HTTP errors return đúng status code

### 3. Testing
- [ ] Unit tests cho domain logic (không cần external)
- [ ] Integration tests cho adapters dùng emulator
- [ ] Table-driven tests với tên case mô tả rõ ràng
- [ ] `require.NoError` thay vì `assert.NoError` cho setup steps

### 4. Go Idioms
- [ ] Context propagation: mọi function có side effect nhận `context.Context`
- [ ] No global state
- [ ] Interface định nghĩa ở nơi sử dụng (ports/), không ở nơi implement
- [ ] Pointer receiver nhất quán trong 1 type

### 5. Security
- [ ] Không hardcode credentials
- [ ] User input được validate trước khi dùng
- [ ] Authorization check trước khi access resource

## Output Format
Trả về review dưới dạng:
```
## Code Review

### Critical Issues (phải fix trước merge)
- ...

### Suggestions (nên fix)
- ...

### Positives
- ...
```
```

### `.claude/skills/add-feature/SKILL.md`

```markdown
---
name: add-feature
description: >
  Thêm feature mới vào 1 service theo DDD pattern.
  Hướng dẫn từng bước: domain entity → port interface → adapter → app layer → HTTP handler.
---

# Add Feature Skill — Wild Workouts DDD Pattern

## Khi nhận yêu cầu thêm feature, làm theo thứ tự này:

### Step 1: Define Domain (domain/)
```go
// Tạo/update entity hoặc value object
// File: internal/{service}/domain/{entity}.go
type NewEntity struct {
    // fields
}

func NewNewEntity(/* params */) (*NewEntity, error) {
    // validate business rules
    // return domain error nếu vi phạm
}
```

### Step 2: Define Port Interface (ports/)
```go
// File: internal/{service}/ports/{repository}.go
type NewEntityRepository interface {
    GetByID(ctx context.Context, id string) (*domain.NewEntity, error)
    Save(ctx context.Context, entity *domain.NewEntity) error
}
```

### Step 3: Implement Adapter (adapters/)
```go
// File: internal/{service}/adapters/{impl}_repo.go
type FirestoreNewEntityRepo struct {
    firestoreClient *firestore.Client
}

func (r *FirestoreNewEntityRepo) GetByID(ctx context.Context, id string) (*domain.NewEntity, error) {
    // implementation
}
```

### Step 4: Add Command/Query (app/)
```go
// Command file: internal/{service}/app/command/new_action.go
type NewActionHandler struct {
    repo ports.NewEntityRepository
}

func (h NewActionHandler) Handle(ctx context.Context, cmd NewAction) error {
    // business use case
}
```

### Step 5: Wire in Service (service/)
Inject new repository và handler vào Application struct.

### Step 6: Add HTTP Handler (ports/)
Implement OpenAPI generated interface, delegate to app layer.

### Step 7: Update OpenAPI Spec
Thêm endpoint vào `api/openapi/{service}.yml`, chạy `make openapi`.

### Step 8: Write Tests
- Unit test cho domain logic
- Integration test cho adapter với Firestore emulator
```

### `.claude/skills/refactor/SKILL.md`

```markdown
---
name: refactor
description: >
  Refactor code trong wild-workouts project. Đảm bảo không vi phạm DDD boundaries,
  không break existing tests, và follow Go best practices.
---

# Refactor Skill — Wild Workouts

## Pre-Refactor Checklist
1. Đọc file cần refactor và hiểu context đầy đủ
2. Chạy tests hiện tại để xác nhận chúng pass: `cd internal/{service} && go test ./...`
3. Identify mục tiêu refactor (performance? readability? DDD compliance?)

## Refactor Patterns Thường Gặp

### Extract Repository Interface
Khi adapter bị gọi trực tiếp từ app layer:
```go
// TRƯỚC (sai)
type Handler struct {
    firestoreRepo *adapters.FirestoreRepo  // concrete type
}

// SAU (đúng)
type Handler struct {
    repo ports.Repository  // interface
}
```

### Replace Domain Primitive với Value Object
```go
// TRƯỚC
type Training struct {
    UserID string  // primitive
}

// SAU
type Training struct {
    UserID UserID  // value object với validation
}
type UserID struct {
    value string
}
```

## Post-Refactor Verification
1. `go build ./...` — không có compile error
2. `go test ./...` — tất cả tests pass
3. `make lint` — không có lint violations
4. Manual check: DDD layer boundaries không bị vi phạm
```

### `.claude/skills/release/SKILL.md`

```markdown
---
name: release
description: >
  Chuẩn bị và thực hiện release cho wild-workouts services.
  Bao gồm: build Docker images, update configs, deploy với Terraform.
---

# Release Skill — Wild Workouts

## Pre-Release Checklist
- [ ] `make lint` passes
- [ ] `make test` passes (tất cả integration tests)
- [ ] OpenAPI specs up to date (`make openapi`)
- [ ] Proto files compiled (`make proto`)
- [ ] Docker images build thành công

## Build Docker Images
```bash
# Build tất cả services
./scripts/build-docker.sh

# Hoặc individual service
docker build -f docker/app/Dockerfile \
  --build-arg service=trainer \
  -t wild-workouts-trainer:$(git rev-parse --short HEAD) .
```

## Local Validation
```bash
# Start toàn bộ stack
docker-compose up --build

# Verify services healthy
curl http://localhost:3000/health  # trainer
curl http://localhost:3001/health  # trainings
curl http://localhost:3002/health  # users
```

## Deploy to Google Cloud
```bash
cd terraform/service
terraform plan -var="image_tag=$(git rev-parse --short HEAD)"
terraform apply  # cần confirm
```

## Rollback
```bash
terraform apply -var="image_tag={previous_tag}"
```
```

---

## Bước 6: Tạo Architecture Decision Records

### `docs/decisions/001-ddd-layer-boundaries.md`

```markdown
# ADR 001: DDD Layer Boundary Enforcement

**Date:** 2026-03-17
**Status:** Accepted

## Context
Project dùng DDD với 4 layers: domain, app, ports, adapters.
Cần rule rõ ràng về import direction.

## Decision
Import direction (một chiều):
```
adapters → ports ← app → domain
```
- `domain` không import gì ngoài standard library
- `app` chỉ import `domain` và `ports`
- `adapters` implement interfaces từ `ports`
- Enforce bằng `go-cleanarch` trong `make lint`

## Consequences
- Domain logic hoàn toàn testable không cần infrastructure
- Có thể swap Firestore → MySQL mà không sửa business logic
- Overhead: cần viết interface cho mọi external dependency
```

---

## Bước 7: Tích Hợp Paperclip (Optional — cho Multi-Agent Scale)

### Khi nào nên dùng Paperclip?

Paperclip phù hợp khi:
- Bạn cần **nhiều Claude Code agents** làm việc song song (trainer service + trainings service + users service cùng lúc)
- Cần **24/7 autonomous operation** (CI fixes, dependency updates, monitoring)
- Cần **cost tracking** cho từng service/task
- Cần **governance** — approve major refactors trước khi AI apply

### Setup Paperclip cho Wild Workouts

```bash
# Cài đặt Paperclip local
npx paperclipai onboard --yes
# Paperclip UI: http://localhost:3100

# Hoặc manual
git clone https://github.com/paperclipai/paperclip.git
cd paperclip && pnpm install && pnpm dev
```

### Org Chart gợi ý

```
CEO Agent (goal: maintain wild-workouts codebase quality)
├── Trainer Service Agent
│   └── Skills: code-review, add-feature, refactor
├── Trainings Service Agent
│   └── Skills: code-review, add-feature, refactor
├── Users Service Agent
│   └── Skills: code-review, add-feature, refactor
└── DevOps Agent
    └── Skills: release, infrastructure
```

### Inject Skills vào Agent

Paperclip hỗ trợ **runtime skill injection** — agent nhận skills khi được assign task,
không cần retrain. Skills từ `.claude/skills/` có thể được load trực tiếp:

```bash
# Agent được configure với PROJECT_DIR pointing to wild-workouts
# Skills được inject qua PAPERCLIP env vars
PAPERCLIP_SKILLS_DIR=./skills  # pointing to .claude/skills/
```

---

## Quick Start Checklist

```
□ Tạo /CLAUDE.md (root context)
□ Tạo internal/trainer/CLAUDE.md
□ Tạo internal/trainings/CLAUDE.md
□ Tạo internal/users/CLAUDE.md
□ Tạo internal/common/CLAUDE.md
□ Tạo web/CLAUDE.md
□ Tạo .claude/settings.json
□ Tạo .claude/hooks/pre-edit-check.sh (chmod +x)
□ Tạo .claude/hooks/post-edit-lint.sh (chmod +x)
□ Tạo .claude/skills/code-review/SKILL.md
□ Tạo .claude/skills/add-feature/SKILL.md
□ Tạo .claude/skills/refactor/SKILL.md
□ Tạo .claude/skills/release/SKILL.md
□ Tạo docs/decisions/ directory với ADRs
□ (Optional) Setup Paperclip cho multi-agent orchestration
```

---

## Kết Quả Kỳ Vọng

| Trước | Sau |
|-------|-----|
| AI không biết DDD rules, suggest sai architecture | CLAUDE.md guides AI đúng layer boundaries |
| Mỗi PR review phải prompt lại từ đầu | `/code-review` skill tái dụng nhất quán |
| AI có thể edit bất kỳ file nào theo cách sai | Hooks cảnh báo layer violations realtime |
| Không rõ "tại sao code lại viết vậy" | ADRs document decisions rõ ràng |
| 1 agent, tuần tự | N agents song song với Paperclip |

---

*Tài liệu này được tạo dựa trên cấu trúc thực tế của project và best practices từ Claude Code Project Structure + Paperclip orchestration platform.*
